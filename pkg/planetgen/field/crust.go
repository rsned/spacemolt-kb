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

func dot3(a, b [3]float64) float64 { //nolint:unused // consumed by Phase 12 Tasks 4-8
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func norm3(a [3]float64) [3]float64 { //nolint:unused // consumed by Phase 12 Tasks 4-8
	m := math.Sqrt(dot3(a, a))
	if m < 1e-12 {
		return [3]float64{1, 0, 0}
	}
	return [3]float64{a[0] / m, a[1] / m, a[2] / m}
}

// slerp3 spherically interpolates between unit vectors a and b.
// Callers must ensure dot(a,b) > −1+ε; exactly antipodal inputs are
// geometrically ambiguous and produce an arbitrary direction at t≈0.5.
func slerp3(a, b [3]float64, t float64) [3]float64 { //nolint:unused // consumed by Phase 12 Tasks 4-8
	d := dot3(a, b)
	if d > 1 {
		d = 1
	}
	if d < -1 {
		d = -1
	}
	th := math.Acos(d)
	if th < 1e-9 {
		return a
	}
	sa := math.Sin((1-t)*th) / math.Sin(th)
	sb := math.Sin(t*th) / math.Sin(th)
	return norm3([3]float64{
		a[0]*sa + b[0]*sb,
		a[1]*sa + b[1]*sb,
		a[2]*sa + b[2]*sb,
	})
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
