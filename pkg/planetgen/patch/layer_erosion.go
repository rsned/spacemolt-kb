package patch

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// riverNotchDepth is how far below oceanLevel erosion may carve a coast
// channel mouth. 0.01 = 1% of the normalized [0,1] height range, enough
// to make river mouths visibly cut into the shore.
const riverNotchDepth = 0.01

// patchDroplets scales the canonical droplet budget (calibrated for a
// full 6×1024² sphere) down to the window's share of the sphere.
func patchDroplets(canonical, size, sProd int) int {
	n := canonical * size * size / (6 * sProd * sProd)
	return max(n, 200)
}

func erosionEnabled(ctx *Context) bool { return ctx.Profile.Erosion.Droplets > 0 }

// applyErosion (layer 6): the recovered flat droplet simulator, seeded
// with the production erosion domains, on a clone of the heightmap.
func applyErosion(ctx *Context, st *State) *State {
	cfg := ctx.Profile.Erosion
	cfg.Droplets = patchDroplets(cfg.Droplets, st.Height.Size, ctx.Fields.Window.SProd)
	ns := *st
	ns.Height = st.Height.Clone()
	erodePatch(ctx.Master, ns.Height, cfg, ctx.Sphere.SeaLevel0)
	return &ns
}

// erodePatch runs the flat droplet hydraulic erosion simulator over hm
// in place. Ported from the pre-cubemap flat-path implementation
// (erodeFlat in the deleted pkg/planetgen/render/flat_erosion.go); the
// only behavioral change is the caller-supplied cfg.Droplets, which is
// already patch-share-scaled by applyErosion.
func erodePatch(masterSeed int64, hm *Grid, cfg types.ErosionConfig, oceanLevel float64) {
	if cfg.Droplets <= 0 {
		return
	}
	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(masterSeed, "erosion.seed")),   //nolint:gosec
		uint64(seed.Domain(masterSeed, "erosion.stream")), //nolint:gosec
	))
	inertia := defOr(cfg.Inertia, 0.05)
	capacity := defOr(cfg.Capacity, 4.0)
	erosionRate := defOr(cfg.ErosionRate, 0.3)
	deposition := defOr(cfg.Deposition, 0.3)
	evaporation := defOr(cfg.Evaporation, 0.01)
	minSlope := defOr(cfg.MinSlope, 0.01)
	maxSteps := cfg.MaxStepsPerDrop
	if maxSteps <= 0 {
		maxSteps = 50
	}
	gravity := defOr(cfg.Gravity, 4.0)
	stepLen := cfg.StepLen
	if stepLen <= 0 {
		stepLen = 1.0
	}

	weights := computeFlatBrushWeights(cfg.BrushFalloff)
	for range cfg.Droplets {
		simulateDropletFlat(rng, hm.Data, hm.Size, inertia, capacity, erosionRate,
			deposition, evaporation, minSlope, gravity, stepLen, maxSteps, oceanLevel, weights)
	}
}

func simulateDropletFlat(rng *rand.Rand, hm []float64, size int,
	inertia, capacity, erosionRate, deposition, evaporation, minSlope, gravity, stepLen float64,
	maxSteps int, oceanLevel float64, weights [9]float64) {
	px := rng.Float64() * float64(size-1)
	py := rng.Float64() * float64(size-1)

	// Skip droplets that spawn below the ocean surface.
	if oceanLevel > 0 && bilinFlat(hm, size, px, py) < oceanLevel {
		return
	}

	var vx, vy float64
	water := 1.0
	sediment := 0.0

	for range maxSteps {
		if px < 0 || px >= float64(size) || py < 0 || py >= float64(size) {
			return
		}
		h, gx, gy := sampleWithGradientFlat(hm, size, px, py)
		vx = inertia*vx + (1-inertia)*(-gx)
		vy = inertia*vy + (1-inertia)*(-gy)
		vlen := math.Sqrt(vx*vx + vy*vy)
		if vlen < 1e-9 {
			return
		}
		vxN, vyN := vx/vlen, vy/vlen
		nx := px + vxN*stepLen
		ny := py + vyN*stepLen
		if nx < 0 || nx >= float64(size) || ny < 0 || ny >= float64(size) {
			return
		}
		nh, _, _ := sampleWithGradientFlat(hm, size, nx, ny)

		// Carve a river-mouth notch at the coast; sediment is discarded (washes away).
		if oceanLevel > 0 && nh < oceanLevel {
			ix, iy := int(px), int(py)
			applyErodeFlat(hm, size, ix, iy, 1.0, oceanLevel, weights)
			return
		}

		deltaH := nh - h
		speed := vlen
		slope := -deltaH
		if slope < minSlope {
			slope = minSlope
		}
		cap := slope * speed * water * capacity

		ix, iy := int(px), int(py)
		if sediment > cap || deltaH > 0 {
			amt := (sediment - cap) * deposition
			if amt < 0 {
				amt = 0
			}
			if deltaH > 0 && amt > deltaH {
				amt = deltaH
			}
			sediment -= applyDepositFlat(hm, size, ix, iy, amt, weights)
		} else {
			amt := (cap - sediment) * erosionRate
			sediment += applyErodeFlat(hm, size, ix, iy, amt, oceanLevel, weights)
		}

		px, py = nx, ny
		speedSq := speed*speed + (-deltaH)*gravity
		if speedSq < 0 {
			speedSq = 0
		}
		s2 := math.Sqrt(speedSq)
		vx = vxN * s2
		vy = vyN * s2
		water *= 1 - evaporation
		if water < 0.01 {
			return
		}
	}
}

func sampleWithGradientFlat(hm []float64, size int, x, y float64) (h, gx, gy float64) {
	h = bilinFlat(hm, size, x, y)
	eps := 3.0
	hE := bilinFlat(hm, size, x+eps, y)
	hW := bilinFlat(hm, size, x-eps, y)
	hN := bilinFlat(hm, size, x, y+eps)
	hS := bilinFlat(hm, size, x, y-eps)
	gx = (hE - hW) / (2 * eps)
	gy = (hN - hS) / (2 * eps)
	return
}

func bilinFlat(hm []float64, size int, x, y float64) float64 {
	if x < 0 {
		x = 0
	} else if x > float64(size-1) {
		x = float64(size - 1)
	}
	if y < 0 {
		y = 0
	} else if y > float64(size-1) {
		y = float64(size - 1)
	}
	x0, y0 := int(x), int(y)
	x1, y1 := x0+1, y0+1
	if x1 > size-1 {
		x1 = size - 1
	}
	if y1 > size-1 {
		y1 = size - 1
	}
	tx := x - float64(x0)
	ty := y - float64(y0)
	h00 := hm[y0*size+x0]
	h10 := hm[y0*size+x1]
	h01 := hm[y1*size+x0]
	h11 := hm[y1*size+x1]
	return (1-ty)*((1-tx)*h00+tx*h10) + ty*((1-tx)*h01+tx*h11)
}

// 3×3 brush pixel offsets.
var (
	flatBrushOX = [9]int{-1, 0, 1, -1, 0, 1, -1, 0, 1}
	flatBrushOY = [9]int{-1, -1, -1, 0, 0, 0, 1, 1, 1}
)

// computeFlatBrushWeights builds normalized 3x3 brush weights for falloff k.
// k <= 0 falls back to 1.0 (current default).
func computeFlatBrushWeights(k float64) [9]float64 {
	if k <= 0 {
		k = 1.0
	}
	var raw [9]float64
	sum := 0.0
	for i := range 9 {
		d := math.Sqrt(float64(flatBrushOX[i]*flatBrushOX[i] + flatBrushOY[i]*flatBrushOY[i]))
		raw[i] = 1.0 / math.Pow(1.0+d, k)
		sum += raw[i]
	}
	var w [9]float64
	for i := range 9 {
		w[i] = raw[i] / sum
	}
	return w
}

func applyErodeFlat(hm []float64, size, ix, iy int, amt, oceanLevel float64, weights [9]float64) float64 {
	total := 0.0
	for i := range 9 {
		x, y := ix+flatBrushOX[i], iy+flatBrushOY[i]
		if x < 0 || x >= size || y < 0 || y >= size {
			continue
		}
		amtCell := amt * weights[i]
		cur := hm[y*size+x]
		// Clamp so we never carve below the relaxed river-mouth floor.
		floor := 0.0
		if oceanLevel > 0 {
			floor = oceanLevel - riverNotchDepth
			if floor < 0 {
				floor = 0
			}
		}
		if cur-amtCell < floor {
			amtCell = cur - floor
		}
		if amtCell < 0 {
			amtCell = 0
		}
		hm[y*size+x] = cur - amtCell
		total += amtCell
	}
	return total
}

func applyDepositFlat(hm []float64, size, ix, iy int, amt float64, weights [9]float64) float64 {
	total := 0.0
	for i := range 9 {
		x, y := ix+flatBrushOX[i], iy+flatBrushOY[i]
		if x < 0 || x >= size || y < 0 || y >= size {
			continue
		}
		amtCell := amt * weights[i]
		cur := hm[y*size+x]
		if cur+amtCell > 1 {
			amtCell = 1 - cur
		}
		hm[y*size+x] = cur + amtCell
		total += amtCell
	}
	return total
}

func defOr(v, fallback float64) float64 {
	if v == 0 {
		return fallback
	}
	return v
}
