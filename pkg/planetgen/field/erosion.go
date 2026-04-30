package field

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Erode runs particle hydraulic erosion on heightmap and returns it modified.
// cfg.Droplets <= 0 is a no-op (returns heightmap unchanged).
//
// Droplets walk in 3D unit-sphere coordinates. Each droplet samples height +
// gradient at its current position, updates a tangent-space velocity, steps to
// a new position, and erodes or deposits at the previous pixel.
//
// Same seed + cfg + heightmap → same output regardless of CPU.
func Erode(masterSeed int64, heightmap *cubemap.CubeMapF, cfg types.ErosionConfig, S int) *cubemap.CubeMapF {
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
			deposition, evaporation, minSlope, gravity, stepLen, maxSteps)
	}
	return heightmap
}

func simulateDroplet(rng *rand.Rand, hm *cubemap.CubeMapF, S int,
	inertia, capacity, erosionRate, deposition, evaporation, minSlope, gravity, stepLen float64,
	maxSteps int,
) {
	// Uniform-sphere sampling via rejection-free cylindrical method.
	z := 1 - 2*rng.Float64() //nolint:gosec
	phi := 2 * math.Pi * rng.Float64()
	r := math.Sqrt(1 - z*z)
	px, py, pz := r*math.Cos(phi), z, r*math.Sin(phi)

	var vx, vy, vz float64
	water := 1.0
	sediment := 0.0

	for range maxSteps {
		h, gx, gy, gz := sampleWithGradient(hm, px, py, pz)
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
		nh, _, _, _ := sampleWithGradient(hm, nx, ny, nz)

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
			cur := hm.Get(face, ix, iy)
			if cur+amt > 1 {
				amt = 1 - cur
			}
			hm.Set(face, ix, iy, cur+amt)
			sediment -= amt
		} else {
			// Erode; pick up material up to capacity.
			amt := (cap - sediment) * erosionRate
			if amt > -deltaH {
				amt = -deltaH
			}
			cur := hm.Get(face, ix, iy)
			if cur-amt < 0 {
				amt = cur
			}
			hm.Set(face, ix, iy, cur-amt)
			sediment += amt
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
// vector in the tangent plane at (x, y, z). Four offset samples along the
// local east and north tangents are used for central-difference estimation.
func sampleWithGradient(hm *cubemap.CubeMapF, x, y, z float64) (float64, float64, float64, float64) {
	h := hm.Sample(x, y, z)
	const eps = 1e-3
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
