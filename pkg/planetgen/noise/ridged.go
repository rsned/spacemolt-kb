package noise

import "math"

// RidgedFractal3D evaluates a ridged-multifractal at point p. Each octave
// contributes (offset - |noise|)² weighted by the previous octave's signal²
// times gain (clamped to [0,1]). The output is non-negative and produces
// sharp ridge-like features good for mountain belts.
//
// Typical settings: octaves 4-6, lacunarity 2.0, gain 0.5, offset 1.0.
// Higher offset (>1) produces sharper, taller peaks.
func (g *Generator) RidgedFractal3D(x, y, z float64, octaves int, lacunarity, gain, offset float64) float64 {
	if octaves <= 0 {
		return 0
	}
	freq := 1.0
	weight := 1.0
	sum := 0.0
	for range octaves {
		signal := offset - math.Abs(g.noise.Eval3(x*freq, y*freq, z*freq))
		signal *= signal
		signal *= weight
		sum += signal
		weight = signal * gain
		if weight > 1 {
			weight = 1
		}
		if weight < 0 {
			weight = 0
		}
		freq *= lacunarity
	}
	return sum
}
