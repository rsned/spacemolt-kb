package planetgen

import (
	"image"
	"math"
	"math/rand/v2"
)

// RenderRocky generates a rocky planet surface map.
func RenderRocky(profile *PlanetProfile, seed int64, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed*31+7)))
	ng := NewNoiseGenerator(seed)

	// Step 1: Generate base heightmap
	heightmap := make([][]float64, height)
	for y := range height {
		heightmap[y] = make([]float64, width)
		for x := range width {
			heightmap[y][x] = ng.SphericalFractal(
				x, y, width, height,
				profile.NoiseOctaves,
				profile.NoiseLacunarity,
				profile.NoisePersistence,
				profile.NoiseScale,
			)
		}
	}

	// Step 2: Stamp craters
	if profile.CraterCount > 0 {
		craters := GenerateCraters(rng, profile.CraterCount,
			profile.CraterMinRadius, profile.CraterMaxRadius)
		ApplyCraters(heightmap, craters, width, height, profile.CraterDepth)
	}

	// Step 3 + 4 + 5: Colorize with ocean and polar caps
	// Pre-compute secondary noise generators
	capNoise := NewNoiseGenerator(seed + 42)
	oceanNoise := NewNoiseGenerator(seed + 77)

	for y := range height {
		lat := math.Pi/2 - float64(y)/float64(height)*math.Pi // [pi/2, -pi/2]
		absLat := math.Abs(lat) / (math.Pi / 2)               // [0, 1] from equator to pole

		for x := range width {
			h := heightmap[y][x]
			var c = sampleGradient(profile.Palette, h)

			// Ocean: color below ocean level with depth and surface variation
			if profile.OceanLevel > 0 && h < profile.OceanLevel {
				depth := (profile.OceanLevel - h) / profile.OceanLevel

				// Surface variation: currents and waves
				sx, sy, sz := SphericalCoords(x, y, width, height)
				surfaceVar := oceanNoise.FractalNoise3D(sx, sy, sz, 4, 2.0, 0.5, 6.0)

				// Shallow water near coastlines is lighter
				shallowFactor := 1.0
				if depth < 0.15 {
					shallowFactor = 1.3 - depth*2.0
				}

				// Combine depth darkening with surface variation
				brightness := (1.0 - depth*0.5) * shallowFactor
				brightness += (surfaceVar - 0.5) * 0.15
				brightness = math.Max(0.5, math.Min(1.3, brightness))

				c = brighten(profile.OceanColor, brightness)
			}

			// Polar caps
			if profile.HasPolarCaps && profile.PolarCapSize > 0 {
				capThreshold := 1.0 - profile.PolarCapSize
				if absLat > capThreshold {
					// Noisy edge
					sx, sy, sz := SphericalCoords(x, y, width, height)
					capEdgeNoise := capNoise.FractalNoise3D(sx, sy, sz, 4, 2.0, 0.5, 8.0)
					adjustedThreshold := capThreshold + (capEdgeNoise-0.5)*0.08

					if absLat > adjustedThreshold {
						blend := math.Min(1.0, (absLat-adjustedThreshold)*15)
						capColor := brighten(whiteIce, 0.9+capEdgeNoise*0.2)
						c = blendColor(c, capColor, blend)
					}
				}
			}

			img.SetRGBA(x, y, c)
		}
	}

	return img
}

var whiteIce = rgba(240, 245, 250)
