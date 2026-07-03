package noise

import "math"

// CurlGen wraps a noise.Generator for curl-noise sampling.
type CurlGen struct{ g *Generator }

// NewCurlGen constructs a new CurlGen with the given seed.
func NewCurlGen(seed int64) *CurlGen {
	return &CurlGen{g: New(seed)}
}

// curl3D returns the curl of a vector field (Φx, Φy, Φz) where each
// component is fbm. Approximated via central differences.
func (cg *CurlGen) curl3D(x, y, z, freq float64) (float64, float64, float64) {
	const eps = 1e-3
	px := func(a, b, c float64) float64 {
		return cg.g.FractalNoise3D(a, b+1.7, c+5.3, 3, 2.0, 0.5, freq)
	}
	py := func(a, b, c float64) float64 {
		return cg.g.FractalNoise3D(a+5.3, b, c+1.7, 3, 2.0, 0.5, freq)
	}
	pz := func(a, b, c float64) float64 {
		return cg.g.FractalNoise3D(a+1.7, b+5.3, c, 3, 2.0, 0.5, freq)
	}
	dpzdy := (pz(x, y+eps, z) - pz(x, y-eps, z)) / (2 * eps)
	dpydz := (py(x, y, z+eps) - py(x, y, z-eps)) / (2 * eps)
	dpxdz := (px(x, y, z+eps) - px(x, y, z-eps)) / (2 * eps)
	dpzdx := (pz(x+eps, y, z) - pz(x-eps, y, z)) / (2 * eps)
	dpydx := (py(x+eps, y, z) - py(x-eps, y, z)) / (2 * eps)
	dpxdy := (px(x, y+eps, z) - px(x, y-eps, z)) / (2 * eps)
	return dpzdy - dpydz, dpxdz - dpzdx, dpydx - dpxdy
}

// BackwardTrace integrates p backward by `iters` semi-Lagrangian steps.
// Each step subtracts dt·(jetAmp·zonal + amp·curlNoise) from the
// position, normalising back onto the unit sphere.
func (cg *CurlGen) BackwardTrace(x, y, z, amp float64, iters int, dt, freq, jetAmp float64) (float64, float64, float64) {
	if amp == 0 && jetAmp == 0 {
		return x, y, z
	}
	for range iters {
		// Zonal jet: tangent vector (eastward) at this latitude.
		cx, cy, cz := -z, 0.0, x
		n := math.Sqrt(cx*cx + cy*cy + cz*cz)
		if n > 1e-9 {
			cx, cy, cz = cx/n, cy/n, cz/n
		}
		// Sinusoidal jet by latitude (∝ cos(3·lat)).
		lat := math.Asin(y)
		jet := math.Cos(3 * lat)
		cux, cuy, cuz := cg.curl3D(x, y, z, freq)
		x -= dt * (jetAmp*jet*cx + amp*cux)
		y -= dt * (jetAmp*jet*cy + amp*cuy)
		z -= dt * (jetAmp*jet*cz + amp*cuz)
		n = math.Sqrt(x*x + y*y + z*z)
		if n > 0 {
			x, y, z = x/n, y/n, z/n
		}
	}
	return x, y, z
}
