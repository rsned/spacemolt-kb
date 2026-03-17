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

	// Step 1b: Normalize heightmap to use full [0,1] range
	// Simplex noise tends to cluster around 0.5, so stretch to ensure
	// peaks and valleys actually reach the extremes.
	var hMin, hMax float64
	hMin = 1.0
	for y := range height {
		for x := range width {
			if heightmap[y][x] < hMin {
				hMin = heightmap[y][x]
			}
			if heightmap[y][x] > hMax {
				hMax = heightmap[y][x]
			}
		}
	}
	if hMax > hMin {
		hRange := hMax - hMin
		for y := range height {
			for x := range width {
				heightmap[y][x] = (heightmap[y][x] - hMin) / hRange
			}
		}
	}

	// Step 2: Stamp craters
	if profile.CraterCount > 0 {
		craters := GenerateCraters(rng, profile.CraterCount,
			profile.CraterMinRadius, profile.CraterMaxRadius)
		ApplyCraters(heightmap, craters, width, height, profile.CraterDepth)
	}

	// Step 3 + 4 + 5: Colorize with biomes, ocean, snow, and polar caps
	// Pre-compute secondary noise generators
	capNoise := NewNoiseGenerator(seed + 42)
	oceanNoise := NewNoiseGenerator(seed + 77)
	biomeNoise := NewNoiseGenerator(seed + 99)

	hasBiomes := len(profile.EquatorialPalette) > 0 || len(profile.PolarPalette) > 0

	for y := range height {
		lat := math.Pi/2 - float64(y)/float64(height)*math.Pi // [pi/2, -pi/2]
		absLat := math.Abs(lat) / (math.Pi / 2)               // [0, 1] from equator to pole

		for x := range width {
			h := heightmap[y][x]

			// Base terrain color — blend between biome palettes by latitude
			var c = sampleGradient(profile.Palette, h)
			if hasBiomes {
				// Add noise to biome boundaries so they're not perfectly zonal
				sx, sy, sz := SphericalCoords(x, y, width, height)
				biomeVar := biomeNoise.FractalNoise3D(sx, sy, sz, 3, 2.0, 0.5, 4.0)
				adjustedLat := absLat + (biomeVar-0.5)*0.15

				if len(profile.EquatorialPalette) > 0 && adjustedLat < 0.35 {
					// Blend equatorial palette near equator
					eqColor := sampleGradient(profile.EquatorialPalette, h)
					eqBlend := 1.0 - adjustedLat/0.35 // 1.0 at equator, 0.0 at lat 0.35
					eqBlend *= eqBlend                  // smooth curve
					c = blendColor(c, eqColor, eqBlend*0.8)
				}
				if len(profile.PolarPalette) > 0 && adjustedLat > 0.6 {
					// Blend polar palette near poles (before ice caps)
					polColor := sampleGradient(profile.PolarPalette, h)
					polBlend := (adjustedLat - 0.6) / 0.4 // 0.0 at lat 0.6, 1.0 at pole
					polBlend *= polBlend
					c = blendColor(c, polColor, polBlend*0.7)
				}
			}

			// Snow line: high elevation peaks get snow
			if profile.SnowLine > 0 && h > profile.SnowLine {
				snowBlend := (h - profile.SnowLine) / (1.0 - profile.SnowLine)
				snowBlend = math.Min(1.0, snowBlend*1.5)
				// More snow at higher latitudes
				latBoost := 1.0 + absLat*0.5
				snowBlend = math.Min(1.0, snowBlend*latBoost)
				c = blendColor(c, whiteSnow, snowBlend*0.85)
			}

			// Ocean/lava: color below ocean level with depth and surface variation
			if profile.OceanLevel > 0 && h < profile.OceanLevel {
				depth := (profile.OceanLevel - h) / profile.OceanLevel
				sx, sy, sz := SphericalCoords(x, y, width, height)
				surfaceVar := oceanNoise.FractalNoise3D(sx, sy, sz, 4, 2.0, 0.5, 6.0)

				isLava := profile.Type == "lava_world"
				if isLava {
					// Lava: brighter when deeper, with hot shimmer
					brightness := 0.7 + depth*0.3
					// Near-shore cooling effect (darker at edges)
					if depth < 0.2 {
						brightness *= 0.6 + depth*2.0
					}
					// Hot shimmer variation
					brightness += (surfaceVar - 0.5) * 0.25
					brightness = math.Max(0.4, math.Min(1.2, brightness))

					// Vary lava color between orange and bright yellow
					lavaColor := lerpColor(
						profile.OceanColor,
						rgba(255, 160, 20), // bright yellow-orange
						surfaceVar*0.4,
					)
					c = brighten(lavaColor, brightness)
				} else {
					// Water: darker when deeper, lighter near shore
					shallowFactor := 1.0
					if depth < 0.15 {
						shallowFactor = 1.3 - depth*2.0
					}
					brightness := (1.0 - depth*0.5) * shallowFactor
					brightness += (surfaceVar - 0.5) * 0.15
					brightness = math.Max(0.5, math.Min(1.3, brightness))
					c = brighten(profile.OceanColor, brightness)
				}
			}

			// Polar caps
			if profile.HasPolarCaps && profile.PolarCapSize > 0 {
				capThreshold := 1.0 - profile.PolarCapSize
				if absLat > capThreshold {
					// Noisy edge — use profile noise amount or default
					sx, sy, sz := SphericalCoords(x, y, width, height)
					capEdgeNoise := capNoise.FractalNoise3D(sx, sy, sz, 4, 2.0, 0.5, 8.0)
					noiseAmt := profile.PolarCapNoise
					if noiseAmt == 0 {
						noiseAmt = 0.08
					}
					adjustedThreshold := capThreshold + (capEdgeNoise-0.5)*noiseAmt

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
var whiteSnow = rgba(235, 240, 245)
