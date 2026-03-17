package planetgen

import (
	"math"

	opensimplex "github.com/ojrac/opensimplex-go"
)

// NoiseGenerator wraps opensimplex with octave-based fractal noise.
type NoiseGenerator struct {
	noise opensimplex.Noise
}

// NewNoiseGenerator creates a noise generator with the given seed.
func NewNoiseGenerator(seed int64) *NoiseGenerator {
	return &NoiseGenerator{
		noise: opensimplex.New(seed),
	}
}

// Sample3D returns a single noise sample in [-1, 1].
func (ng *NoiseGenerator) Sample3D(x, y, z float64) float64 {
	return ng.noise.Eval3(x, y, z)
}

// FractalNoise3D returns multi-octave fractal noise normalized to [0, 1].
func (ng *NoiseGenerator) FractalNoise3D(x, y, z float64, octaves int, lacunarity, persistence, scale float64) float64 {
	var value, amplitude, maxAmplitude float64
	amplitude = 1.0
	freq := scale

	for range octaves {
		value += ng.noise.Eval3(x*freq, y*freq, z*freq) * amplitude
		maxAmplitude += amplitude
		amplitude *= persistence
		freq *= lacunarity
	}

	// Normalize to [0, 1]
	return (value/maxAmplitude + 1.0) / 2.0
}

// SphericalCoords converts pixel coordinates on a 2:1 equirectangular map
// to a 3D point on a unit sphere. This ensures seamless wrapping.
func SphericalCoords(px, py, width, height int) (x, y, z float64) {
	lon := float64(px) / float64(width) * 2 * math.Pi   // [0, 2*pi]
	lat := math.Pi/2 - float64(py)/float64(height)*math.Pi // [pi/2, -pi/2]

	x = math.Cos(lat) * math.Cos(lon)
	y = math.Sin(lat)
	z = math.Cos(lat) * math.Sin(lon)
	return x, y, z
}

// SphericalFractal samples fractal noise at a point on the sphere.
func (ng *NoiseGenerator) SphericalFractal(px, py, width, height int, octaves int, lacunarity, persistence, scale float64) float64 {
	sx, sy, sz := SphericalCoords(px, py, width, height)
	return ng.FractalNoise3D(sx, sy, sz, octaves, lacunarity, persistence, scale)
}
