package render

import (
	"image/color"
	"math"

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
	heightmap, craters := generateRockyHeightmap(profile, seed, S)
	return colorizeRocky(profile, seed, S, heightmap, craters)
}

// RenderRockyHeightmap returns a grayscale cube map showing the
// normalized heightmap. Useful as a debug view to verify ridges,
// craters, control-field shapes, and other height-affecting algorithms
// before the colorizer obscures them with palette/biome blending.
func RenderRockyHeightmap(profile *types.PlanetProfile, seed int64, S int) *cubemap.CubeMap {
	heightmap, _ := generateRockyHeightmap(profile, seed, S)
	out := cubemap.New(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				h := heightmap.Get(face, px, py)
				if h < 0 {
					h = 0
				}
				if h > 1 {
					h = 1
				}
				v := uint8(h * 255)
				out.Set(face, px, py, color.RGBA{R: v, G: v, B: v, A: 255})
			}
		}
	}
	return out
}

// generateRockyHeightmap runs the heightmap-only portion of the rocky
// pipeline: control-field summation (or legacy fBm), ridged contribution,
// province modulation, normalization, and crater stamping. Returns the
// normalized heightmap so RenderRocky can colorize it and
// RenderRockyHeightmap can render it as grayscale.
func generateRockyHeightmap(profile *types.PlanetProfile, seed int64, S int) (*cubemap.CubeMapF, []feature.Crater) {
	return generateRockyHeightmapDebug(profile, seed, S, nil, nil)
}

// generateRockyHeightmapDebug is the debug-aware variant of
// generateRockyHeightmap. When frame is non-nil, it appends a DebugStage
// after each pipeline stage. When a stage's name is in bypass, its
// contribution is skipped while rng draws are still consumed so downstream
// noise streams remain deterministic.
func generateRockyHeightmapDebug(profile *types.PlanetProfile, seed int64, S int,
	frame *DebugFrame, bypass DebugBypass) (*cubemap.CubeMapF, []feature.Crater) {
	ng := noise.New(seed)
	warper := noise.NewWarper(seed, profile.Warp)

	heightmap := cubemap.NewF(S)
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

		if frame != nil || len(bypass) > 0 {
			// Debug path: accumulate one control field at a time so we can
			// record per-stage snapshots and honour bypass semantics. The
			// ridged contribution is still applied in the final per-pixel
			// pass below to keep the fast path bit-identical.
			fieldNames := [5]string{"Continentalness", "Detail", "PeaksValleys", "Temperature", "Humidity"}
			// Only the first three control fields contribute to elevation;
			// Temperature and Humidity are climate fields that feed the
			// Whittaker biome lookup downstream. Keep iterating all five so
			// the debug frame still captures their fbm + spline rows; the
			// SumAfter for Temperature/Humidity will equal SumAfter for
			// PeaksValleys (correctly showing zero contribution to height).
			const heightContributingFields = 3
			for i := range 5 {
				name := fieldNames[i]
				raw := fields[i]
				bypassed := bypass[name]
				if !bypassed && i < heightContributingFields {
					for face := range cubemap.Face(cubemap.NumFaces) {
						for py := range S {
							for px := range S {
								var freqMod float64 = 1
								var rampMod float64 = 1
								if provRamp != nil {
									rampMod = provRamp.Get(face, px, py)
									freqMod = provRFreq.Get(face, px, py)
								}
								v := raw.Get(face, px, py) * freqMod
								contribution := planetcolor.EvalSpline(cfFields[i].Spline, v) * rampMod
								heightmap.Set(face, px, py, heightmap.Get(face, px, py)+contribution)
							}
						}
					}
				}
				if frame != nil {
					// Skip band classification when the spline has fewer than
					// 3 knots — there's only one interval and the result would
					// be a uniform single color, which is not informative.
					var inputBands, outputBands *cubemap.CubeMap
					if len(cfFields[i].Spline.Knots) >= 3 {
						inputBands = ClassifySplineInputBands(raw, cfFields[i].Spline, S)
						outputBands = ClassifySplineOutputBands(raw, cfFields[i].Spline, S)
					}
					frame.Stages = append(frame.Stages, DebugStage{
						Name:        name,
						Kind:        "height",
						RawFbm:      raw.Clone(),
						InputBands:  inputBands,
						OutputBands: outputBands,
						SumAfter:    heightmap.Clone(),
						Skipped:     bypassed,
					})
				}
			}
			// Apply ridged on top of summed control fields.
			if ridgedGen != nil {
				var hmBefore *cubemap.CubeMapF
				if frame != nil {
					hmBefore = heightmap.Clone()
				}
				bypassed := bypass["Ridged"]
				if !bypassed {
					for face := range cubemap.Face(cubemap.NumFaces) {
						for py := range S {
							for px := range S {
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
									heightmap.Set(face, px, py, heightmap.Get(face, px, py)+profile.Ridged.Amp*mask*r)
								}
							}
						}
					}
				} else {
					// Bypass: still consume all ridgedGen draws for rng stability.
					for face := range cubemap.Face(cubemap.NumFaces) {
						for py := range S {
							for px := range S {
								cont := planetcolor.EvalSpline(cfFields[0].Spline, fields[0].Get(face, px, py))
								mask := smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, cont)
								if mask > 0 {
									dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
									dx, dy, dz = warper.Warp(dx, dy, dz)
									_ = ridgedGen.RidgedFractal3D(
										dx*profile.Ridged.Freq,
										dy*profile.Ridged.Freq,
										dz*profile.Ridged.Freq,
										profile.Ridged.Octaves,
										profile.Ridged.Lacunarity,
										profile.Ridged.Gain,
										profile.Ridged.Offset)
								}
							}
						}
					}
				}
				if frame != nil {
					delta := cubemap.NewF(S)
					for face := range cubemap.Face(cubemap.NumFaces) {
						for i := range heightmap.Faces[face] {
							delta.Faces[face][i] = heightmap.Faces[face][i] - hmBefore.Faces[face][i]
						}
					}
					frame.Stages = append(frame.Stages, DebugStage{
						Name:     "Ridged",
						Kind:     "height",
						RawFbm:   delta,
						SumAfter: heightmap.Clone(),
						Skipped:  bypass["Ridged"],
					})
				}
			}
		} else {
			// Fast path (no debug frame, no bypass): single combined pixel loop,
			// identical to the original implementation.
			for face := range cubemap.Face(cubemap.NumFaces) {
				for py := range S {
					for px := range S {
						var rampMod, freqMod float64 = 1, 1
						if provRamp != nil {
							rampMod = provRamp.Get(face, px, py)
							freqMod = provRFreq.Get(face, px, py)
						}
						var h float64
						// Only Continentalness, Detail, PeaksValleys (i < 3)
						// contribute to elevation. Temperature and Humidity are
						// climate fields consumed by the biome lookup later.
						for i := range 3 {
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
		}
	} else {
		var ridgedGen *noise.Generator
		if profile.Ridged.Amp > 0 && profile.Ridged.Freq > 0 && profile.Ridged.Octaves > 0 {
			ridgedGen = noise.New(pgseed.Domain(seed, "ridged"))
		}
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
					if ridgedGen != nil {
						// No Continentalness available in legacy mode; use the
						// local fBm height itself as the mask input so high
						// areas get ridges and low areas don't.
						mask := smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, h)
						if mask > 0 {
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
	}

	// Phase 4 Task 5: Apply Voronoi continents baseline if enabled.
	if profile.Continents.Seeds > 0 {
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		cont := field.GenerateContinents(seed, profile.Continents, S)
		bypassed := bypass["Continents"]
		if !bypassed {
			for face := range cubemap.Face(cubemap.NumFaces) {
				for py := range S {
					for px := range S {
						h := heightmap.Get(face, px, py)
						base := cont.Get(face, px, py)
						if base > h {
							h = base
						}
						heightmap.Set(face, px, py, h)
					}
				}
			}
		}
		if frame != nil {
			// RawFbm holds the delta between before and after so the
			// viewer can see which pixels were lifted.
			delta := cubemap.NewF(S)
			for face := range cubemap.Face(cubemap.NumFaces) {
				for i := range heightmap.Faces[face] {
					delta.Faces[face][i] = heightmap.Faces[face][i] - hmBefore.Faces[face][i]
				}
			}
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "Continents",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed,
			})
		}
	}

	// Phase 5 follow-up: smooth heightmap to reduce per-pixel popcorn so
	// erosion can form coherent channels.
	if profile.HeightSmoothRadius > 0 {
		bypassed := bypass["HeightSmooth"]
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		if !bypassed {
			heightmap = field.SmoothHeightmap(heightmap, profile.HeightSmoothRadius, S)
		}
		if frame != nil {
			delta := cubemap.NewF(S)
			for face := range cubemap.Face(cubemap.NumFaces) {
				for i := range heightmap.Faces[face] {
					delta.Faces[face][i] = heightmap.Faces[face][i] - hmBefore.Faces[face][i]
				}
			}
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "HeightSmooth",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed,
			})
		}
	}

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
	if frame != nil {
		frame.Stages = append(frame.Stages, DebugStage{
			Name:     "Normalize",
			Kind:     "height",
			SumAfter: heightmap.Clone(),
		})
	}

	// Phase 4 Task 3: Apply coastal noise enhancement if enabled.
	{
		active := profile.Coastal.Amp > 0 && profile.OceanLevel > 0
		bypassed := bypass["Coastal"]
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		if active && !bypassed {
			distField := field.DistanceToCoast(heightmap, profile.OceanLevel, S)
			coastGen := noise.NewCoastalGen(pgseed.Domain(seed, "coastal"))
			for face := range cubemap.Face(cubemap.NumFaces) {
				for py := range S {
					for px := range S {
						dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
						h := heightmap.Get(face, px, py)
						d := distField.Get(face, px, py)
						heightmap.Set(face, px, py,
							noise.ApplyCoastal(coastGen, dx, dy, dz, h, d,
								profile.Coastal.Amp, profile.Coastal.Threshold, profile.Coastal.Freq))
					}
				}
			}
		}
		if frame != nil {
			delta := cubemap.NewF(S)
			for face := range cubemap.Face(cubemap.NumFaces) {
				for i := range heightmap.Faces[face] {
					delta.Faces[face][i] = heightmap.Faces[face][i] - hmBefore.Faces[face][i]
				}
			}
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "Coastal",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed || !active,
			})
		}
	}

	// Phase 5: particle hydraulic erosion. Droplet count auto-scales by
	// face area; floor at 5000 so previews at face=64 still show channels.
	{
		active := profile.Erosion.Droplets > 0
		bypassed := bypass["Erosion"]
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		if active && !bypassed {
			const baseSize = 1024 // planetgen.DefaultFaceSize; hard-coded to avoid import cycle
			scale := float64(S*S) / float64(baseSize*baseSize)
			n := int(float64(profile.Erosion.Droplets) * scale)
			if n < 5000 && profile.Erosion.Droplets >= 5000 {
				n = 5000
			}
			cfg := profile.Erosion
			cfg.Droplets = n
			field.Erode(seed, heightmap, cfg, profile.OceanLevel, S)
		}
		if frame != nil {
			delta := cubemap.NewF(S)
			for face := range cubemap.Face(cubemap.NumFaces) {
				for i := range heightmap.Faces[face] {
					delta.Faces[face][i] = heightmap.Faces[face][i] - hmBefore.Faces[face][i]
				}
			}
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "Erosion",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed || !active,
			})
		}
	}

	var craters []feature.Crater
	if profile.CraterCount > 0 {
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		craters = feature.GenerateCraters(seed, profile)
		bypassed := bypass["Craters"]
		if !bypassed {
			feature.ApplyCraters(heightmap, craters, profile.CraterDepth)
		}
		if frame != nil {
			delta := cubemap.NewF(S)
			for face := range cubemap.Face(cubemap.NumFaces) {
				for i := range heightmap.Faces[face] {
					delta.Faces[face][i] = heightmap.Faces[face][i] - hmBefore.Faces[face][i]
				}
			}
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "Craters",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed,
			})
		}
	}
	return heightmap, craters
}

// colorizeRocky paints the heightmap into a color cube map using the
// profile's palette/biome/ocean/snow/cap/shading/LUT settings.
func colorizeRocky(profile *types.PlanetProfile, seed int64, S int, heightmap *cubemap.CubeMapF, craters []feature.Crater) *cubemap.CubeMap {
	return colorizeRockyDebug(profile, seed, S, heightmap, craters, nil, nil)
}

// colorizeRockyDebug is the debug-aware variant of colorizeRocky. When
// frame is non-nil, it appends a DebugStage after each color pipeline
// stage. When a stage's name is in bypass, the stage is skipped while
// any noise draws are still consumed so downstream streams stay stable.
func colorizeRockyDebug(profile *types.PlanetProfile, seed int64, S int, heightmap *cubemap.CubeMapF, craters []feature.Crater, frame *DebugFrame, bypass DebugBypass) *cubemap.CubeMap {
	capNoise := noise.New(seed + 42)
	oceanNoise := noise.New(seed + 77)
	biomeNoise := noise.New(seed + 99)
	warper := noise.NewWarper(seed, profile.Warp)

	// Step 3+4+5: colorise (biome, ocean, snow, polar caps).
	hasBiomes := len(profile.EquatorialPalette) > 0 || len(profile.PolarPalette) > 0
	useBiomeTable := len(profile.BiomeTable.Cells) > 0
	var tField, mField *cubemap.CubeMapF
	if useBiomeTable {
		tField, mField = biome.GenerateClimateFields(seed, profile, S)
	}
	out := cubemap.New(S)

	// Stage: Palette/Biome base color.
	bypassPalette := bypass["Palette"]
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				rawX, rawY, rawZ := cubemap.FacePixelToDir(face, px, py, S)
				dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
				lat := math.Asin(rawY)
				absLat := math.Abs(lat) / (math.Pi / 2)
				h := heightmap.Get(face, px, py)
				var c color.RGBA
				if useBiomeTable {
					// Consume biomeNoise draws regardless of bypass for rng stability.
					if !bypassPalette {
						c = biome.LookupColor(profile.BiomeTable, tField.Get(face, px, py), mField.Get(face, px, py), h)
					}
				} else {
					if !bypassPalette {
						c = planetcolor.SampleGradientOkLab(profile.Palette, h)
					}
					if hasBiomes {
						biomeVar := biomeNoise.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 4.0)
						if !bypassPalette {
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
				}
				out.Set(face, px, py, c)
			}
		}
	}
	if frame != nil {
		frame.Stages = append(frame.Stages, DebugStage{
			Name:       "Palette",
			Kind:       "color",
			ColorAfter: out.Clone(),
			Skipped:    bypassPalette,
		})
	}

	// Stage: Snow.
	if profile.SnowLine > 0 {
		bypassed := bypass["Snow"]
		if !bypassed {
			for face := range cubemap.Face(cubemap.NumFaces) {
				for py := range S {
					for px := range S {
						rawX, rawY, rawZ := cubemap.FacePixelToDir(face, px, py, S)
						lat := math.Asin(rawY)
						absLat := math.Abs(lat) / (math.Pi / 2)
						_ = rawX
						_ = rawZ
						h := heightmap.Get(face, px, py)
						if h > profile.SnowLine {
							c := out.Get(face, px, py)
							snowBlend := (h - profile.SnowLine) / (1.0 - profile.SnowLine)
							snowBlend = math.Min(1.0, snowBlend*1.5)
							latBoost := 1.0 + absLat*0.5
							snowBlend = math.Min(1.0, snowBlend*latBoost)
							out.Set(face, px, py, planetcolor.BlendOkLab(c, whiteSnow, snowBlend*0.85))
						}
					}
				}
			}
		}
		if frame != nil {
			frame.Stages = append(frame.Stages, DebugStage{
				Name:       "Snow",
				Kind:       "color",
				ColorAfter: out.Clone(),
				Skipped:    bypassed,
			})
		}
	}

	// Stage: Ocean.
	if profile.OceanLevel > 0 {
		bypassed := bypass["Ocean"]
		for face := range cubemap.Face(cubemap.NumFaces) {
			for py := range S {
				for px := range S {
					rawX, rawY, rawZ := cubemap.FacePixelToDir(face, px, py, S)
					dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
					h := heightmap.Get(face, px, py)
					if h < profile.OceanLevel {
						depth := (profile.OceanLevel - h) / profile.OceanLevel
						// Always consume the noise draw for rng stability.
						surfaceVar := oceanNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 6.0)
						if !bypassed {
							var c color.RGBA
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
							out.Set(face, px, py, c)
						}
					}
				}
			}
		}
		if frame != nil {
			frame.Stages = append(frame.Stages, DebugStage{
				Name:       "Ocean",
				Kind:       "color",
				ColorAfter: out.Clone(),
				Skipped:    bypassed,
			})
		}
	}

	// Stage: PolarCaps.
	if profile.HasPolarCaps && profile.PolarCapSize > 0 {
		bypassed := bypass["PolarCaps"]
		for face := range cubemap.Face(cubemap.NumFaces) {
			for py := range S {
				for px := range S {
					rawX, rawY, rawZ := cubemap.FacePixelToDir(face, px, py, S)
					dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
					lat := math.Asin(rawY)
					absLat := math.Abs(lat) / (math.Pi / 2)
					capThreshold := 1.0 - profile.PolarCapSize
					if absLat > capThreshold {
						// Always consume noise draw for rng stability.
						capEdgeNoise := capNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 8.0)
						if !bypassed {
							noiseAmt := profile.PolarCapNoise
							if noiseAmt == 0 {
								noiseAmt = 0.08
							}
							adjustedThreshold := capThreshold + (capEdgeNoise-0.5)*noiseAmt
							if absLat > adjustedThreshold {
								c := out.Get(face, px, py)
								blend := math.Min(1.0, (absLat-adjustedThreshold)*15)
								capColor := planetcolor.Brighten(whiteIce, 0.9+capEdgeNoise*0.2)
								out.Set(face, px, py, planetcolor.BlendOkLab(c, capColor, blend))
							}
						}
					}
				}
			}
		}
		if frame != nil {
			frame.Stages = append(frame.Stages, DebugStage{
				Name:       "PolarCaps",
				Kind:       "color",
				ColorAfter: out.Clone(),
				Skipped:    bypassed,
			})
		}
	}

	// Stage: Shading (slope-based Lambertian).
	if profile.ShadingStrength > 0 {
		bypassed := bypass["Shading"]
		if !bypassed {
			exag := profile.ShadingExaggeration
			if exag <= 0 {
				exag = 8.0
			}
			for face := range cubemap.Face(cubemap.NumFaces) {
				for py := range S {
					for px := range S {
						rawX, rawY, rawZ := cubemap.FacePixelToDir(face, px, py, S)
						c := out.Get(face, px, py)
						out.Set(face, px, py, applySlopeShading(heightmap, c, rawX, rawY, rawZ,
							profile.ShadingStrength, exag))
					}
				}
			}
		}
		if frame != nil {
			frame.Stages = append(frame.Stages, DebugStage{
				Name:       "Shading",
				Kind:       "color",
				ColorAfter: out.Clone(),
				Skipped:    bypassed,
			})
		}
	}

	// Stage: Ejecta.
	if profile.PowerLawAlpha > 0 && len(craters) > 0 {
		bypassed := bypass["Ejecta"]
		if !bypassed {
			for face := range cubemap.Face(cubemap.NumFaces) {
				for py := range S {
					for px := range S {
						rawX, rawY, rawZ := cubemap.FacePixelToDir(face, px, py, S)
						c := out.Get(face, px, py)
						out.Set(face, px, py, feature.ApplyEjecta(c, rawX, rawY, rawZ, craters))
					}
				}
			}
		}
		if frame != nil {
			frame.Stages = append(frame.Stages, DebugStage{
				Name:       "Ejecta",
				Kind:       "color",
				ColorAfter: out.Clone(),
				Skipped:    bypassed,
			})
		}
	}

	// Stage: LUT (final color grade).
	if name := profile.LUT; name != "" {
		if lut := planetcolor.LookupLUT(name); lut != nil {
			bypassed := bypass["LUT"]
			if !bypassed {
				for face := range cubemap.Face(cubemap.NumFaces) {
					for py := range S {
						for px := range S {
							c := out.Get(face, px, py)
							out.Set(face, px, py, planetcolor.ApplyLUT(*lut, c))
						}
					}
				}
			}
			if frame != nil {
				frame.Stages = append(frame.Stages, DebugStage{
					Name:       "LUT",
					Kind:       "color",
					ColorAfter: out.Clone(),
					Skipped:    bypassed,
				})
			}
		}
	}

	return out
}

// orderedControlFields returns the five ControlField values in canonical
// order (Continentalness, Detail, PeaksValleys, Temperature, Humidity)
// matching field.GenerateControlFields' output indexing.
func orderedControlFields(c types.ControlConfig) [5]types.ControlField {
	return [5]types.ControlField{
		c.Continentalness,
		c.Detail,
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

// applySlopeShading modulates the input color by a Lambertian dot
// product against a fixed sun direction. The surface normal is
// reconstructed from heightmap finite differences along two tangents
// orthogonal to the radial direction (rawX, rawY, rawZ).  Sample uses
// the cube-map's seam-aware lookup so face boundaries don't tear.
//
// strength in [0,1]: 0 = unchanged; 1 = full diffuse modulation.
// exaggeration scales the gradient so subtle features still shade.
func applySlopeShading(hm *cubemap.CubeMapF, c color.RGBA, rx, ry, rz, strength, exaggeration float64) color.RGBA {
	const eps = 0.005
	// Tangent basis orthogonal to (rx,ry,rz). Use world-up (0,1,0) unless
	// the radial is too close to it, in which case fall back to world-x.
	upx, upy, upz := 0.0, 1.0, 0.0
	if math.Abs(ry) > 0.95 {
		upx, upy, upz = 1.0, 0.0, 0.0
	}
	// t1 = normalize(cross(up, r))
	t1x := upy*rz - upz*ry
	t1y := upz*rx - upx*rz
	t1z := upx*ry - upy*rx
	t1n := math.Sqrt(t1x*t1x + t1y*t1y + t1z*t1z)
	if t1n == 0 {
		return c
	}
	t1x, t1y, t1z = t1x/t1n, t1y/t1n, t1z/t1n
	// t2 = cross(r, t1)
	t2x := ry*t1z - rz*t1y
	t2y := rz*t1x - rx*t1z
	t2z := rx*t1y - ry*t1x

	hu1 := hm.Sample(rx+eps*t1x, ry+eps*t1y, rz+eps*t1z)
	hu0 := hm.Sample(rx-eps*t1x, ry-eps*t1y, rz-eps*t1z)
	hv1 := hm.Sample(rx+eps*t2x, ry+eps*t2y, rz+eps*t2z)
	hv0 := hm.Sample(rx-eps*t2x, ry-eps*t2y, rz-eps*t2z)
	dHdu := (hu1 - hu0) / (2 * eps) * exaggeration
	dHdv := (hv1 - hv0) / (2 * eps) * exaggeration

	// Approximate world-space normal: in the (t1, t2, r) frame the
	// surface tangents are (1, 0, dHdu) and (0, 1, dHdv); their cross
	// product is (-dHdu, -dHdv, 1).  Express in world coords.
	nx := -dHdu*t1x - dHdv*t2x + rx
	ny := -dHdu*t1y - dHdv*t2y + ry
	nz := -dHdu*t1z - dHdv*t2z + rz
	nn := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if nn == 0 {
		return c
	}
	nx, ny, nz = nx/nn, ny/nn, nz/nn

	// Fixed sun direction: upper-right-front, normalized.
	const lx, ly, lz = 0.6172, 0.6172, 0.4881 // (1, 1, 0.7)/|(1,1,0.7)|
	diff := nx*lx + ny*ly + nz*lz
	if diff < 0 {
		diff = 0
	}
	// Blend ambient and diffuse so flat areas read at neutral brightness.
	bright := (1.0 - strength) + strength*(0.4+0.8*diff)
	return planetcolor.Brighten(c, bright)
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
