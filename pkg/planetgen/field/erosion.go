package field

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// brushOffsets, brushWeights are a 3×3 erosion brush with 1/(1+r) falloff.
// Weights sum to 1 so total mass eroded/deposited == amt regardless of brush shape.
// Reference weights at r ∈ {0, 1, √2}: 1.0, 0.5, 0.4142; sum = 1 + 4·0.5 + 4·0.4142 ≈ 4.6568.
var (
	brushOffsetX = [9]int{-1, 0, 1, -1, 0, 1, -1, 0, 1}
	brushOffsetY = [9]int{-1, -1, -1, 0, 0, 0, 1, 1, 1}
	brushWeights = [9]float64{} // populated in init()
)

func init() {
	var raw [9]float64
	sum := 0.0
	for i := range 9 {
		r := math.Sqrt(float64(brushOffsetX[i]*brushOffsetX[i] + brushOffsetY[i]*brushOffsetY[i]))
		w := 1.0 / (1.0 + r)
		raw[i] = w
		sum += w
	}
	for i := range 9 {
		brushWeights[i] = raw[i] / sum
	}
}

// applyErode carves amt across a 3x3 brush at (ix, iy), clipping at face
// edges and clamping per-cell so height stays ≥ oceanLevel (or ≥ 0 when
// oceanLevel==0). Returns the total height actually carved.
func applyErode(hm *cubemap.CubeMapF, face cubemap.Face, ix, iy, S int, amt, oceanLevel float64) float64 {
	total := 0.0
	for i := range 9 {
		ix2 := ix + brushOffsetX[i]
		iy2 := iy + brushOffsetY[i]
		if ix2 < 0 || ix2 >= S || iy2 < 0 || iy2 >= S {
			continue
		}
		amtCell := amt * brushWeights[i]
		cur := hm.Get(face, ix2, iy2)
		// Clamp so we never carve below the ocean floor.
		floor := 0.0
		if oceanLevel > 0 {
			floor = oceanLevel
		}
		if cur-amtCell < floor {
			amtCell = cur - floor
		}
		if amtCell < 0 {
			amtCell = 0
		}
		hm.Set(face, ix2, iy2, cur-amtCell)
		total += amtCell
	}
	return total
}

// applyDeposit drops amt across a 3x3 brush at (ix, iy), clipping at
// face edges and clamping per-cell so height stays ≤ 1. Returns the
// total height actually deposited.
func applyDeposit(hm *cubemap.CubeMapF, face cubemap.Face, ix, iy, S int, amt float64) float64 {
	total := 0.0
	for i := range 9 {
		ix2 := ix + brushOffsetX[i]
		iy2 := iy + brushOffsetY[i]
		if ix2 < 0 || ix2 >= S || iy2 < 0 || iy2 >= S {
			continue
		}
		amtCell := amt * brushWeights[i]
		cur := hm.Get(face, ix2, iy2)
		if cur+amtCell > 1 {
			amtCell = 1 - cur
		}
		hm.Set(face, ix2, iy2, cur+amtCell)
		total += amtCell
	}
	return total
}

// Erode runs particle hydraulic erosion on heightmap and returns it modified.
// cfg.Droplets <= 0 is a no-op (returns heightmap unchanged).
//
// When oceanLevel > 0 droplets spawning below it are skipped, droplets that
// flow into ocean water deposit remaining sediment at the coast and stop, and
// erosion never carves below oceanLevel. Set oceanLevel to 0 to disable all
// three guards and reproduce legacy byte-identical behaviour.
//
// Same seed + cfg + oceanLevel + heightmap → same output regardless of CPU.
func Erode(masterSeed int64, heightmap *cubemap.CubeMapF, cfg types.ErosionConfig, oceanLevel float64, S int) *cubemap.CubeMapF {
	if cfg.Droplets <= 0 {
		return heightmap
	}
	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(masterSeed, "erosion.seed")),   //nolint:gosec
		uint64(seed.Domain(masterSeed, "erosion.stream")), //nolint:gosec
	))
	inertia := defaultIfZero(cfg.Inertia, 0.05)
	capacity := defaultIfZero(cfg.Capacity, 4.0)
	erosionRate := defaultIfZero(cfg.ErosionRate, 0.3)
	deposition := defaultIfZero(cfg.Deposition, 0.3)
	evaporation := defaultIfZero(cfg.Evaporation, 0.01)
	minSlope := defaultIfZero(cfg.MinSlope, 0.01)
	maxSteps := cfg.MaxStepsPerDrop
	if maxSteps <= 0 {
		maxSteps = 50
	}
	gravity := defaultIfZero(cfg.Gravity, 4.0)
	stepLen := cfg.StepLen
	if stepLen <= 0 {
		stepLen = 1.0 / float64(2*S)
	}

	for range cfg.Droplets {
		simulateDroplet(rng, heightmap, S, inertia, capacity, erosionRate,
			deposition, evaporation, minSlope, gravity, stepLen, maxSteps, oceanLevel)
	}
	return heightmap
}

func simulateDroplet(rng *rand.Rand, hm *cubemap.CubeMapF, S int,
	inertia, capacity, erosionRate, deposition, evaporation, minSlope, gravity, stepLen float64,
	maxSteps int, oceanLevel float64,
) {
	// Uniform-sphere sampling via rejection-free cylindrical method.
	z := 1 - 2*rng.Float64() //nolint:gosec
	phi := 2 * math.Pi * rng.Float64()
	r := math.Sqrt(1 - z*z)
	px, py, pz := r*math.Cos(phi), z, r*math.Sin(phi)

	// Skip droplets that spawn below the ocean surface.
	if oceanLevel > 0 && hm.Sample(px, py, pz) < oceanLevel {
		return
	}

	var vx, vy, vz float64
	water := 1.0
	sediment := 0.0

	for range maxSteps {
		h, gx, gy, gz := sampleWithGradient(hm, S, px, py, pz)
		// Blend velocity toward negative gradient (downhill direction).
		vx = inertia*vx + (1-inertia)*(-gx)
		vy = inertia*vy + (1-inertia)*(-gy)
		vz = inertia*vz + (1-inertia)*(-gz)
		// Project velocity onto tangent plane to keep droplet on sphere.
		dot := vx*px + vy*py + vz*pz
		vx -= dot * px
		vy -= dot * py
		vz -= dot * pz
		// Normalize and step.
		vlen := math.Sqrt(vx*vx + vy*vy + vz*vz)
		if vlen < 1e-9 {
			return // stalled
		}
		vxN, vyN, vzN := vx/vlen, vy/vlen, vz/vlen
		nx, ny, nz := erosionNormalize(px+vxN*stepLen, py+vyN*stepLen, pz+vzN*stepLen)
		nh, _, _, _ := sampleWithGradient(hm, S, nx, ny, nz)

		// Terminate when flowing into ocean: deposit remaining sediment at coast.
		if oceanLevel > 0 && nh < oceanLevel {
			face, ix, iy := cubemap.DirToFacePixel(px, py, pz, S)
			applyDeposit(hm, face, ix, iy, S, sediment)
			return
		}

		deltaH := nh - h
		speed := vlen
		slope := -deltaH
		if slope < minSlope {
			slope = minSlope
		}
		cap := slope * speed * water * capacity

		face, ix, iy := cubemap.DirToFacePixel(px, py, pz, S)
		if sediment > cap || deltaH > 0 {
			// Deposit excess sediment or fill uphill step.
			amt := (sediment - cap) * deposition
			if amt < 0 {
				amt = 0
			}
			if deltaH > 0 && amt > deltaH {
				amt = deltaH
			}
			actuallyDeposited := applyDeposit(hm, face, ix, iy, S, amt)
			sediment -= actuallyDeposited
		} else {
			// Erode; pick up material up to capacity.
			amt := (cap - sediment) * erosionRate
			actuallyEroded := applyErode(hm, face, ix, iy, S, amt, oceanLevel)
			sediment += actuallyEroded
		}

		// Advance position.
		px, py, pz = nx, ny, nz
		// Update speed via gravity-on-slope; clamp to non-negative before sqrt.
		speedSq := speed*speed + (-deltaH)*gravity
		if speedSq < 0 {
			speedSq = 0
		}
		s2 := math.Sqrt(speedSq)
		vx = vxN * s2
		vy = vyN * s2
		vz = vzN * s2
		water *= 1 - evaporation
		if water < 0.01 {
			return
		}
	}
}

// sampleWithGradient returns (h, ∇hx, ∇hy, ∇hz) where the gradient is a 3D
// vector in the tangent plane at (x, y, z). Stencil width is ~3 pixels so the
// gradient reflects continental-scale slope rather than per-pixel Detail noise.
func sampleWithGradient(hm *cubemap.CubeMapF, S int, x, y, z float64) (float64, float64, float64, float64) {
	h := hm.Sample(x, y, z)
	eps := 3.0 / float64(S)
	ex, ey, ez := tangentEast(x, y, z)
	nx, ny, nz := tangentNorth(x, y, z)
	// Inline four samples to avoid closure boxing.
	hE := hm.Sample(erosionNormalize(x+ex*eps, y+ey*eps, z+ez*eps))
	hW := hm.Sample(erosionNormalize(x-ex*eps, y-ey*eps, z-ez*eps))
	hN := hm.Sample(erosionNormalize(x+nx*eps, y+ny*eps, z+nz*eps))
	hS := hm.Sample(erosionNormalize(x-nx*eps, y-ny*eps, z-nz*eps))
	dE := (hE - hW) / (2 * eps)
	dN := (hN - hS) / (2 * eps)
	gx := dE*ex + dN*nx
	gy := dE*ey + dN*ny
	gz := dE*ez + dN*nz
	return h, gx, gy, gz
}

// tangentEast returns a unit tangent pointing "east" (perpendicular to north
// pole axis and radial direction). Falls back to (1,0,0) at poles.
func tangentEast(x, y, z float64) (float64, float64, float64) {
	// north-pole (0,1,0) × pos gives eastward tangent
	cx, cy, cz := -z, 0.0, x
	n := math.Sqrt(cx*cx + cy*cy + cz*cz)
	if n < 1e-9 {
		return 1, 0, 0
	}
	return cx / n, cy / n, cz / n
}

// tangentNorth returns a unit tangent perpendicular to both east and radial.
func tangentNorth(x, y, z float64) (float64, float64, float64) {
	ex, ey, ez := tangentEast(x, y, z)
	// pos × east gives northward tangent
	tnx := y*ez - z*ey
	tny := z*ex - x*ez
	tnz := x*ey - y*ex
	n := math.Sqrt(tnx*tnx + tny*tny + tnz*tnz)
	if n < 1e-9 {
		return 0, 1, 0
	}
	return tnx / n, tny / n, tnz / n
}

// erosionNormalize projects (x, y, z) back onto the unit sphere.
// Named to avoid collision with any future package-level normalize.
func erosionNormalize(x, y, z float64) (float64, float64, float64) {
	n := math.Sqrt(x*x + y*y + z*z)
	if n == 0 {
		return x, y, z
	}
	return x / n, y / n, z / n
}

// defaultIfZero returns fallback when v is zero; used for default-when-unset config.
func defaultIfZero(v, fallback float64) float64 {
	if v == 0 {
		return fallback
	}
	return v
}
