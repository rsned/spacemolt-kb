// Crust-raft stage (Phase 12): cratons of continental crust riding on
// tectonic plates produce a ContinentalMask and BaseHeight that decide
// land vs ocean, replacing noise-threshold continents on the crust path.
package field

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Craton is one raft of continental crust riding a plate.
type Craton struct {
	Center  [3]float64 // unit direction
	Radius  float64    // angular radius (radians) before edge noise
	PlateID int
}

// CrustField is the per-pixel output of the crust stage plus the
// resolved per-planet tectonic parameters.
type CrustField struct {
	Size            int
	ContinentalMask *cubemap.CubeMapF // 0..1 continental-crust fraction
	BaseHeight      *cubemap.CubeMapF // isostatic base height
	Cratons         []Craton
	Assembly        float64
	LandFraction    float64
	TectonicAge     float64
}

// ResolveCrustParams resolves the three sampled-or-pinned tectonic
// parameters. A -1 sentinel samples deterministically from the
// configured range/weights using named seed domains; any other value
// pins. All draws happen on the "crust.params" domain, in a fixed
// order (assembly, landFrac, age), and pinned parameters still consume
// their draws so pinning one parameter never shifts the others.
// Any negative value is treated as the −1 sentinel.
func ResolveCrustParams(cfg types.CrustConfig, master int64) (assembly, landFrac, age float64) {
	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "crust.params")),        //nolint:gosec
		uint64(seed.Domain(master, "crust.params.stream")), //nolint:gosec
	))

	// Assembly: weighted band, then uniform within the band.
	w := cfg.AssemblyWeights
	for i := range w {
		if w[i] < 0 {
			w[i] = 0
		}
	}
	if w[0]+w[1]+w[2] <= 0 {
		w = [3]float64{25, 65, 10}
	}
	u := rng.Float64() * (w[0] + w[1] + w[2])
	v := rng.Float64()
	var sampledAssembly float64
	switch {
	case u < w[0]:
		sampledAssembly = v * 0.33
	case u < w[0]+w[1]:
		sampledAssembly = 0.33 + v*0.34
	default:
		sampledAssembly = 0.67 + v*0.33
	}
	assembly = cfg.Assembly
	if assembly < 0 {
		assembly = sampledAssembly
	}

	lo, hi := cfg.LandFracLo, cfg.LandFracHi
	if hi <= lo {
		lo, hi = 0.22, 0.38
	}
	sampledLand := lo + rng.Float64()*(hi-lo)
	landFrac = cfg.TargetLandFraction
	if landFrac < 0 {
		landFrac = sampledLand
	}

	alo, ahi := cfg.AgeLo, cfg.AgeHi
	if ahi <= alo {
		alo, ahi = 0.2, 0.8
	}
	sampledAge := alo + rng.Float64()*(ahi-alo)
	age = cfg.TectonicAge
	if age < 0 {
		age = sampledAge
	}
	return assembly, landFrac, age
}

func dot3(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func norm3(a [3]float64) [3]float64 {
	m := math.Sqrt(dot3(a, a))
	if m < 1e-12 {
		return [3]float64{1, 0, 0}
	}
	return [3]float64{a[0] / m, a[1] / m, a[2] / m}
}

// slerp3 spherically interpolates between unit vectors a and b.
//
// Degenerate inputs are handled so the result stays continuous and all
// callers are safe regardless of geometry:
//   - When a and b are nearly parallel (sin(omega)→0) or nearly
//     antipodal (dot(a,b)→−1), slerp falls back to a normalized linear
//     interpolation, which agrees with true slerp in the small-angle
//     limit and never divides by a vanishing sin(omega).
//   - At t == 0 the result is exactly a (early return — the general
//     formula would re-normalize a and could perturb its last ulp), so
//     callers that pass t = 0 to "stay put" get a bit-identical.
func slerp3(a, b [3]float64, t float64) [3]float64 {
	if t == 0 {
		return a
	}
	d := dot3(a, b)
	if d > 1 {
		d = 1
	}
	if d < -1 {
		d = -1
	}
	th := math.Acos(d)
	sinTh := math.Sin(th)
	// Near-parallel or near-antipodal: sin(omega) is too small to divide
	// by. Fall back to normalized linear interpolation, which is
	// continuous with the slerp branch and well-defined at any t.
	if sinTh < 1e-9 {
		return norm3([3]float64{
			a[0]*(1-t) + b[0]*t,
			a[1]*(1-t) + b[1]*t,
			a[2]*(1-t) + b[2]*t,
		})
	}
	sa := math.Sin((1-t)*th) / sinTh
	sb := math.Sin(t*th) / sinTh
	return norm3([3]float64{
		a[0]*sa + b[0]*sb,
		a[1]*sa + b[1]*sb,
		a[2]*sa + b[2]*sb,
	})
}

// PlaceCratons places continental-crust rafts on the carrier
// (non-oceanic) plates. Craton count grows with assembly; centers are
// pulled toward a deterministic "assembly focus" (the midpoint of the
// two closest carrier-plate seeds) by (1−assembly)·0.75 so low
// assembly forms one merged landmass abutting at a shared boundary.
// Radii are budgeted so total cap area ≈ landFrac · sphere · 1.15
// (overlap fudge); the sea-level quantile trues up exactness later.
func PlaceCratons(cfg types.CrustConfig, master int64, pf *PlateField, assembly, landFrac float64, S int) []Craton {
	var carriers []int
	for i, p := range pf.Plates {
		if !p.IsOceanic {
			carriers = append(carriers, i)
		}
	}
	if len(carriers) == 0 {
		carriers = []int{0} // degenerate config: force one carrier
	}

	maxC := cfg.CratonsMax
	if maxC < 2 {
		maxC = 8
	}
	k := 2 + int(math.Round(assembly*float64(maxC-2)))

	// Assembly focus: midpoint of the two closest carrier seeds (or the
	// single carrier's seed). Deterministic — no rng.
	focus := pf.Plates[carriers[0]].Seed
	if len(carriers) >= 2 {
		bi, bj, best := 0, 1, -2.0
		for i := range carriers {
			for j := i + 1; j < len(carriers); j++ {
				d := dot3(pf.Plates[carriers[i]].Seed, pf.Plates[carriers[j]].Seed)
				if d > best {
					best, bi, bj = d, i, j
				}
			}
		}
		focus = norm3([3]float64{
			pf.Plates[carriers[bi]].Seed[0] + pf.Plates[carriers[bj]].Seed[0],
			pf.Plates[carriers[bi]].Seed[1] + pf.Plates[carriers[bj]].Seed[1],
			pf.Plates[carriers[bi]].Seed[2] + pf.Plates[carriers[bj]].Seed[2],
		})
	}
	pull := (1 - assembly) * 0.75

	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "crust.cratons")),        //nolint:gosec
		uint64(seed.Domain(master, "crust.cratons.stream")), //nolint:gosec
	))

	// Radius budget: per-craton weight 1/(1+i) (first craton biggest);
	// cap-area fraction f_i = landFrac·1.15·w_i/Σw; r = acos(1 − 2·f).
	weights := make([]float64, k)
	var wSum float64
	for i := range k {
		weights[i] = 1.0 / float64(1+i)
		wSum += weights[i]
	}

	cratons := make([]Craton, 0, k)
	for i := range k {
		plateIdx := carriers[i%len(carriers)]
		base := pf.Plates[plateIdx].Seed
		// Small jitter off the plate seed so repeat cratons on the same
		// plate don't stack exactly. Always draw 3 Float64s here so the
		// RNG stream and draw order are independent of geometry.
		jx := (rng.Float64() - 0.5) * 0.3
		jy := (rng.Float64() - 0.5) * 0.3
		jz := (rng.Float64() - 0.5) * 0.3
		jittered := norm3([3]float64{base[0] + jx, base[1] + jy, base[2] + jz})

		// onPlate reports whether a unit direction rasterizes to plateIdx.
		onPlate := func(c [3]float64) bool {
			f, px, py := cubemap.DirToFacePixel(c[0], c[1], c[2], S)
			return int(pf.PlateID[f][py*S+px]) == plateIdx
		}

		// Choose the starting point on the home plate: prefer the
		// jittered seed, but if jitter pushed it onto a neighbor plate
		// fall back to the unjittered seed (which is on its own plate by
		// construction).
		start := jittered
		if !onPlate(start) {
			start = base
		}

		// Pull toward focus, shrinking t until the center stays on its
		// home plate (cratons never straddle boundaries).
		center := start
		t := pull
		for range 8 {
			c := slerp3(start, focus, t)
			if onPlate(c) {
				center = c
				break
			}
			t *= 0.5
		}
		// Robust fallback: if every pull attempt left the home plate,
		// keep the on-plate start (or the unjittered seed if even that
		// drifted off, e.g. tiny single-pixel plates).
		if !onPlate(center) {
			if onPlate(start) {
				center = start
			} else {
				center = base
			}
		}

		frac := landFrac * 1.15 * weights[i] / wSum
		if frac > 0.45 {
			frac = 0.45
		}
		r := math.Acos(1 - 2*frac)
		cratons = append(cratons, Craton{Center: center, Radius: r, PlateID: plateIdx})
	}
	return cratons
}

func clamp01(x float64) float64 { //nolint:unused // consumed by Phase 12 Tasks 4-8
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func smoothstep(lo, hi, x float64) float64 { //nolint:unused // consumed by Phase 12 Tasks 4-8
	if hi <= lo {
		if x < lo {
			return 0
		}
		return 1
	}
	t := clamp01((x - lo) / (hi - lo))
	return t * t * (3 - 2*t)
}
