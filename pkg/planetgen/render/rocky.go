package render

import (
	"image/color"
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	pgseed "github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

var (
	whiteIce  = color.RGBA{R: 240, G: 245, B: 250, A: 255}
	whiteSnow = color.RGBA{R: 235, G: 240, B: 245, A: 255}
)

// RenderRocky generates a rocky planet cube map.
func RenderRocky(profile *types.PlanetProfile, seed int64, S int) *cubemap.CubeMap {
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed*31+7)))
	ng := noise.New(seed)
	capNoise := noise.New(seed + 42)
	oceanNoise := noise.New(seed + 77)
	biomeNoise := noise.New(seed + 99)
	warper := noise.NewWarper(seed, profile.Warp)

	heightmap := cubemap.NewF(S)

	// Step 1: base fractal heightmap on the unit sphere.
	cfFields := orderedControlFields(profile.ControlConfig)
	useControl := !isZeroControlConfig(cfFields) && hasAnySpline(cfFields)
	if useControl {
		fields := field.GenerateControlFields(seed, profile.ControlConfig, S)
		var ridgedGen *noise.Generator
		if profile.Ridged.Amp > 0 && profile.Ridged.Freq > 0 && profile.Ridged.Octaves > 0 {
			ridgedGen = noise.New(pgseed.Domain(seed, "ridged"))
		}
		var provRamp, provRFreq *cubemap.CubeMapF
		if profile.Provinces.Count > 0 {
			_, provRamp, provRFreq = field.GenerateProvinces(seed, profile.Provinces, S)
		}
		for face := range cubemap.Face(cubemap.NumFaces) {
			for py := range S {
				for px := range S {
					var rampMod, freqMod float64 = 1, 1
					if provRamp != nil {
						rampMod = provRamp.Get(face, px, py)
						freqMod = provRFreq.Get(face, px, py)
					}
					var h float64
					for i := 0; i < 5; i++ {
						v := fields[i].Get(face, px, py) * freqMod
						contribution := planetcolor.EvalSpline(cfFields[i].Spline, v) * rampMod
						h += contribution
					}
					if ridgedGen != nil {
						cont := planetcolor.EvalSpline(cfFields[0].Spline, fields[0].Get(face, px, py))
						mask := smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, cont)
						if mask > 0 {
							dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
							dx, dy, dz = warper.Warp(dx, dy, dz)
							r := ridgedGen.RidgedFractal3D(
								dx*profile.Ridged.Freq,
								dy*profile.Ridged.Freq,
								dz*profile.Ridged.Freq,
								profile.Ridged.Octaves,
								profile.Ridged.Lacunarity,
								profile.Ridged.Gain,
								profile.Ridged.Offset)
							h += profile.Ridged.Amp * mask * r
						}
					}
					heightmap.Set(face, px, py, h)
				}
			}
		}
	} else {
		// Legacy single-FBM path (unchanged from Phase 0)
		for face := range cubemap.Face(cubemap.NumFaces) {
			for py := range S {
				for px := range S {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					dx, dy, dz = warper.Warp(dx, dy, dz)
					h := ng.FractalNoise3D(dx, dy, dz,
						profile.NoiseOctaves,
						profile.NoiseLacunarity,
						profile.NoisePersistence,
						profile.NoiseScale)
					heightmap.Set(face, px, py, h)
				}
			}
		}
	}

	// Step 1b: normalise to [0,1] across all faces.
	hMin, hMax := 1.0, 0.0
	for face := range cubemap.Face(cubemap.NumFaces) {
		for _, h := range heightmap.Faces[face] {
			if h < hMin {
				hMin = h
			}
			if h > hMax {
				hMax = h
			}
		}
	}
	if hMax > hMin {
		hRange := hMax - hMin
		for face := range cubemap.Face(cubemap.NumFaces) {
			for i := range heightmap.Faces[face] {
				heightmap.Faces[face][i] = (heightmap.Faces[face][i] - hMin) / hRange
			}
		}
	}

	// Step 2: craters.
	if profile.CraterCount > 0 {
		craters := feature.GenerateCraters(rng, profile.CraterCount,
			profile.CraterMinRadius, profile.CraterMaxRadius)
		feature.ApplyCraters(heightmap, craters, profile.CraterDepth)
	}

	// Step 3+4+5: colorise (biome, ocean, snow, polar caps).
	hasBiomes := len(profile.EquatorialPalette) > 0 || len(profile.PolarPalette) > 0
	useBiomeTable := len(profile.BiomeTable.Cells) > 0
	var tField, mField *cubemap.CubeMapF
	if useBiomeTable {
		tField, mField = biome.GenerateClimateFields(seed, profile, S)
	}
	out := cubemap.New(S)

	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				rawX, rawY, rawZ := cubemap.FacePixelToDir(face, px, py, S)
				dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
				// Latitude-driven overlays (polar caps + equatorial/polar
				// palette blending) read from the *unwarped* direction so
				// they stay as clean lat bands. Noise sampling below uses
				// the warped (dx, dy, dz) so features still bend.
				lat := math.Asin(rawY)
				absLat := math.Abs(lat) / (math.Pi / 2)

				h := heightmap.Get(face, px, py)
				var c color.RGBA
				if useBiomeTable {
					c = biome.LookupColor(profile.BiomeTable, tField.Get(face, px, py), mField.Get(face, px, py), h)
				} else {
					c = planetcolor.SampleGradientOkLab(profile.Palette, h)

					if hasBiomes {
						biomeVar := biomeNoise.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 4.0)
						adjustedLat := absLat + (biomeVar-0.5)*0.15
						if len(profile.EquatorialPalette) > 0 && adjustedLat < 0.35 {
							eqColor := planetcolor.SampleGradientOkLab(profile.EquatorialPalette, h)
							eqBlend := 1.0 - adjustedLat/0.35
							eqBlend *= eqBlend
							c = planetcolor.BlendOkLab(c, eqColor, eqBlend*0.8)
						}
						if len(profile.PolarPalette) > 0 && adjustedLat > 0.6 {
							polColor := planetcolor.SampleGradientOkLab(profile.PolarPalette, h)
							polBlend := (adjustedLat - 0.6) / 0.4
							polBlend *= polBlend
							c = planetcolor.BlendOkLab(c, polColor, polBlend*0.7)
						}
					}
				}

				if profile.SnowLine > 0 && h > profile.SnowLine {
					snowBlend := (h - profile.SnowLine) / (1.0 - profile.SnowLine)
					snowBlend = math.Min(1.0, snowBlend*1.5)
					latBoost := 1.0 + absLat*0.5
					snowBlend = math.Min(1.0, snowBlend*latBoost)
					c = planetcolor.BlendOkLab(c, whiteSnow, snowBlend*0.85)
				}

				if profile.OceanLevel > 0 && h < profile.OceanLevel {
					depth := (profile.OceanLevel - h) / profile.OceanLevel
					surfaceVar := oceanNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 6.0)
					if profile.Type == "lava_world" {
						brightness := 0.7 + depth*0.3
						if depth < 0.2 {
							brightness *= 0.6 + depth*2.0
						}
						brightness += (surfaceVar - 0.5) * 0.25
						brightness = math.Max(0.4, math.Min(1.2, brightness))
						lavaColor := planetcolor.Lerp(
							profile.OceanColor,
							color.RGBA{R: 255, G: 160, B: 20, A: 255},
							surfaceVar*0.4,
						)
						c = planetcolor.Brighten(lavaColor, brightness)
					} else {
						shallowFactor := 1.0
						if depth < 0.15 {
							shallowFactor = 1.3 - depth*2.0
						}
						brightness := (1.0 - depth*0.5) * shallowFactor
						brightness += (surfaceVar - 0.5) * 0.15
						brightness = math.Max(0.5, math.Min(1.3, brightness))
						c = planetcolor.Brighten(profile.OceanColor, brightness)
					}
				}

				if profile.HasPolarCaps && profile.PolarCapSize > 0 {
					capThreshold := 1.0 - profile.PolarCapSize
					if absLat > capThreshold {
						capEdgeNoise := capNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 8.0)
						noiseAmt := profile.PolarCapNoise
						if noiseAmt == 0 {
							noiseAmt = 0.08
						}
						adjustedThreshold := capThreshold + (capEdgeNoise-0.5)*noiseAmt
						if absLat > adjustedThreshold {
							blend := math.Min(1.0, (absLat-adjustedThreshold)*15)
							capColor := planetcolor.Brighten(whiteIce, 0.9+capEdgeNoise*0.2)
							c = planetcolor.BlendOkLab(c, capColor, blend)
						}
					}
				}

				// Tier-S Phase 1 Task 26: Apply per-archetype LUT as final color grade
				if name := profile.LUT; name != "" {
					if lut := planetcolor.LookupLUT(name); lut != nil {
						c = planetcolor.ApplyLUT(*lut, c)
					}
				}

				out.Set(face, px, py, c)
			}
		}
	}

	return out
}

// orderedControlFields returns the five ControlField values in canonical
// order (Continentalness, Erosion, PeaksValleys, Temperature, Humidity)
// matching field.GenerateControlFields' output indexing.
func orderedControlFields(c types.ControlConfig) [5]types.ControlField {
	return [5]types.ControlField{
		c.Continentalness,
		c.Erosion,
		c.PeaksValleys,
		c.Temperature,
		c.Humidity,
	}
}

func isZeroControlConfig(fields [5]types.ControlField) bool {
	for _, f := range fields {
		if f.Amp != 0 || f.Freq != 0 || f.Octaves != 0 || f.Lacunarity != 0 || f.Persistence != 0 {
			return false
		}
	}
	return true
}

// smoothstep returns 0 when x ≤ lo, 1 when x ≥ hi, and a smooth Hermite
// interpolation 3t²-2t³ between. Used for masking ridged contributions
// by Continentalness output so ridges fade in across the coastline band.
func smoothstep(lo, hi, x float64) float64 {
	if hi <= lo {
		return 0
	}
	t := (x - lo) / (hi - lo)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}

func hasAnySpline(fields [5]types.ControlField) bool {
	for _, f := range fields {
		if len(f.Spline.Knots) > 0 {
			return true
		}
	}
	return false
}
