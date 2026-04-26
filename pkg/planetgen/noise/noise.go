package noise

import opensimplex "github.com/ojrac/opensimplex-go"

// Generator wraps opensimplex with octave-based fractal noise.
// Renamed from "NoiseGenerator" to "Generator" to avoid stutter
// (noise.NoiseGenerator → noise.Generator).
type Generator struct {
	noise opensimplex.Noise
}

// New creates a noise generator with the given seed.
func New(seed int64) *Generator {
	return &Generator{noise: opensimplex.New(seed)}
}

// Sample3D returns a single noise sample in [-1, 1].
func (g *Generator) Sample3D(x, y, z float64) float64 {
	return g.noise.Eval3(x, y, z)
}

// FractalNoise3D returns multi-octave fractal noise normalized to [0, 1].
func (g *Generator) FractalNoise3D(x, y, z float64, octaves int, lacunarity, persistence, scale float64) float64 {
	var value, amplitude, maxAmplitude float64
	amplitude = 1.0
	freq := scale
	for range octaves {
		value += g.noise.Eval3(x*freq, y*freq, z*freq) * amplitude
		maxAmplitude += amplitude
		amplitude *= persistence
		freq *= lacunarity
	}
	return (value/maxAmplitude + 1.0) / 2.0
}
