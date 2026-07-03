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
	jitter := noise.GenerateJitter(profile, seed, S)
	plates := field.GeneratePlates(profile, seed, S)
	heightmap, craters, oceanLevel := generateRockyHeightmapWithJitter(profile, seed, S, jitter, plates)
	// Phase 12: on the crust path the ocean level is derived from the
	// height histogram rather than fixed by the profile. Copy-on-differ
	// keeps the caller's shared default profile immutable; everything
	// below (flow gen and colorizeRocky) reads the patched copy.
	if oceanLevel != profile.OceanLevel {
		prof := *profile
		prof.OceanLevel = oceanLevel
		profile = &prof
	}
	// Recompute the flow field for the civ overlay's habitability
	// (the heightmap pipeline runs flow internally to carve rivers
	// but does not return the FlowField in the non-debug path).
	var flow *field.FlowField
	if profile.Flow.RiverThreshold > 0 {
		flow = field.GenerateFlow(heightmap, profile.Flow)
	}
	return colorizeRocky(profile, seed, S, heightmap, craters, jitter, plates, flow)
}

// RenderRockyHeightmap returns a grayscale cube map showing the
// normalized heightmap. Useful as a debug view to verify ridges,
// craters, control-field shapes, and other height-affecting algorithms
// before the colorizer obscures them with palette/biome blending.
func RenderRockyHeightmap(profile *types.PlanetProfile, seed int64, S int) *cubemap.CubeMap {
	jitter := noise.GenerateJitter(profile, seed, S)
	plates := field.GeneratePlates(profile, seed, S)
	heightmap, _, _ := generateRockyHeightmapWithJitter(profile, seed, S, jitter, plates)
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

// RenderRockyHeightmapWithOceanLevel exposes the normalized heightmap
// plus the effective ocean level — profile.OceanLevel on the legacy
// path, the Phase 12 quantile-derived level on the crust path — for
// statistical invariants and tools. Mirrors RenderRockyHeightmap.
func RenderRockyHeightmapWithOceanLevel(profile *types.PlanetProfile, seed int64, S int) (*cubemap.CubeMapF, float64) {
	jitter := noise.GenerateJitter(profile, seed, S)
	plates := field.GeneratePlates(profile, seed, S)
	hm, _, lvl := generateRockyHeightmapWithJitter(profile, seed, S, jitter, plates)
	return hm, lvl
}

// RenderNightCubeMap returns the Phase 9b Black-Marble nightside as a
// standalone cube-map. Returns nil for archetypes with profile.Civ.Tier
// == 0 (all current production archetypes — per-archetype civ defaults
// land in P9b Task 6).
//
// Internally this duplicates the rocky heightmap generation up to the
// point where GenerateCiv runs. We don't refactor RenderRocky to share
// the work because the inputs (heightmap, climate fields, plates,
// flow, rain shadow) consume seed streams in a specific order; pulling
// them into a shared helper would force a change to RenderCloudCubeMap
// too, and the savings (~1 heightmap pass per civ-enabled planet) are
// not worth the regression risk against existing goldens. Documented
// as a deliberate inline copy.
//
// SEED-STREAM-MIRROR: this body must call the same generators in the
// same order as colorizeRockyDebug, up to feature.GenerateCiv. If a
// new noise stream is added there ahead of GenerateCiv, mirror it
// here too — otherwise night-side seeds will silently diverge from
// day-side. Grep for "SEED-STREAM-MIRROR" to find the paired site.
func RenderNightCubeMap(profile *types.PlanetProfile, masterSeed int64, S int) *cubemap.CubeMap {
	if profile == nil || profile.Civ.Tier <= 0 {
		return nil
	}
	jitter := noise.GenerateJitter(profile, masterSeed, S)
	plates := field.GeneratePlates(profile, masterSeed, S)
	heightmap, _, oceanLevel := generateRockyHeightmapWithJitter(profile, masterSeed, S, jitter, plates)
	// Phase 12: patch the derived ocean level in BEFORE the climate /
	// rainshadow / civ calls so civ habitability sees the same sea
	// level the dayside render used.
	if oceanLevel != profile.OceanLevel {
		prof := *profile
		prof.OceanLevel = oceanLevel
		profile = &prof
	}

	useBiomeTable := len(profile.BiomeTable.Cells) > 0
	var tField, mField *cubemap.CubeMapF
	if useBiomeTable {
		tField, mField = biome.GenerateClimateFields(masterSeed, profile, S)
	}
	var flow *field.FlowField
	if profile.Flow.RiverThreshold > 0 {
		flow = field.GenerateFlow(heightmap, profile.Flow)
	}
	var rainShadow *biome.RainShadowField
	if useBiomeTable && profile.RainShadow.WalkSteps > 0 {
		rainShadow = biome.GenerateRainShadow(heightmap, profile.RainShadow)
	}
	civ := feature.GenerateCiv(heightmap, tField, mField, plates, flow, rainShadow, profile, masterSeed, S)
	if civ == nil {
		return nil
	}
	return civ.Night
}

// RenderCloudCubeMap returns the cloud overlay as a separate cube-map
// suitable for compositing over the base planet color cube-map.
// Returns nil for archetypes with profile.Cloud.Coverage == 0
// (gas giants, vacuum worlds, ice worlds without atmosphere).
//
// Pixels are rendered as a partially-transparent white-ish overlay:
//
//	RGB = uint8(255 * Density)  -- post-shadow brightness
//	A   = uint8(255 * Alpha)    -- raw cloud fraction
//
// Compositing this over the base planet cube-map at standard alpha
// blending yields the rendered planet with cloud cover.
func RenderCloudCubeMap(profile *types.PlanetProfile, masterSeed int64, S int) *cubemap.CubeMap {
	cf := feature.GenerateClouds(profile, masterSeed, S)
	if cf == nil {
		return nil
	}
	out := cubemap.New(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				idx := py*S + px
				d := cf.Density[face][idx]
				a := cf.Alpha[face][idx]
				v := uint8(0)
				if d > 0 {
					if d >= 1 {
						v = 255
					} else {
						v = uint8(255 * d)
					}
				}
				aa := uint8(0)
				if a > 0 {
					if a >= 1 {
						aa = 255
					} else {
						aa = uint8(255 * a)
					}
				}
				out.Set(face, px, py, color.RGBA{R: v, G: v, B: v, A: aa})
			}
		}
	}
	return out
}

// generateRockyHeightmapWithJitter runs the heightmap pipeline; pass
// nil for jitter to skip the Detail-field cell transform. plates may be
// nil; when non-nil and Ridged.PlateConvergentScaleKm > 0, the ridged
// mountain mask is sourced from plates.Convergent instead of
// Continentalness. The returned float64 is the effective ocean level:
// profile.OceanLevel on the legacy path, or the Phase 12 quantile-
// derived level on the crust path (Crust.MajorPlates > 0).
func generateRockyHeightmapWithJitter(profile *types.PlanetProfile, seed int64, S int, jitter *noise.JitterField, plates *field.PlateField) (*cubemap.CubeMapF, []feature.Crater, float64) {
	return generateRockyHeightmapDebug(profile, seed, S, nil, nil, jitter, plates)
}

// generateRockyHeightmapDebug is the debug-aware variant of
// generateRockyHeightmap. When frame is non-nil, it appends a DebugStage
// after each pipeline stage. When a stage's name is in bypass, its
// contribution is skipped while rng draws are still consumed so downstream
// noise streams remain deterministic.
func generateRockyHeightmapDebug(profile *types.PlanetProfile, seed int64, S int,
	frame *DebugFrame, bypass DebugBypass, jitter *noise.JitterField, plates *field.PlateField) (*cubemap.CubeMapF, []feature.Crater, float64) {
	ng := noise.New(seed)
	warper := noise.NewWarper(seed, profile.Warp)
	if bypass["Warp"] {
		warper.SetBypassed(true)
	}

	heightmap := cubemap.NewF(S)

	// Phase 12: crust-raft stage. On the crust path the heightmap is
	// initialized from the crust BaseHeight (cratons riding plates)
	// and Continentalness / legacy Ridged / Basin / Continents are
	// skipped — land vs ocean comes from the crust, boundary relief
	// from TectonicFX below. The "Crust" debug stage cannot be
	// meaningfully bypassed (every later stage builds on the base
	// height); the honest off-switch is zeroing Crust.MajorPlates,
	// which the explorer panel wires up. The Skipped flag here is
	// cosmetic only.
	crustOn := plates != nil && profile.Crust.MajorPlates > 0
	var crust *field.CrustField
	if crustOn {
		crust = field.GenerateCrust(profile, seed, S, plates)
		for face := range cubemap.Face(cubemap.NumFaces) {
			copy(heightmap.Faces[face], crust.BaseHeight.Faces[face])
		}
		if frame != nil {
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "Crust",
				Kind:     "height",
				RawFbm:   crust.ContinentalMask.Clone(),
				SumAfter: heightmap.Clone(),
				Skipped:  bypass["Crust"],
			})
		}
	}

	cfFields := orderedControlFields(profile.ControlConfig)
	useControl := !isZeroControlConfig(cfFields) && hasAnySpline(cfFields)
	if useControl {
		fields := field.GenerateControlFields(seed, profile.ControlConfig, S, jitter)
		var ridgedGen *noise.Generator
		if !crustOn && profile.Ridged.Amp > 0 && profile.Ridged.Freq > 0 && profile.Ridged.Octaves > 0 {
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
				if !bypassed && i < heightContributingFields && (!crustOn || i != 0) {
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
				usePlateMask := plates != nil && profile.Ridged.PlateConvergentScaleKm > 0
				if !bypassed {
					for face := range cubemap.Face(cubemap.NumFaces) {
						for py := range S {
							for px := range S {
								var mask float64
								if usePlateMask {
									distKm := plates.Convergent[face][py*S+px]
									norm := 1.0 - clamp01(distKm/profile.Ridged.PlateConvergentScaleKm)
									mask = smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, norm)
								} else {
									cont := planetcolor.EvalSpline(cfFields[0].Spline, fields[0].Get(face, px, py))
									mask = smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, cont)
								}
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
									// Modulate amplitude by collision magnitude when the
									// mask is plate-driven: violent convergences produce
									// the full Himalaya-scale Amp, gentle convergences a
									// 0.4·Amp floor so weak boundaries still register.
									ampScale := 1.0
									if usePlateMask {
										ampScale = 0.4 + 0.6*plates.ConvergentMag[face][py*S+px]
									}
									heightmap.Set(face, px, py, heightmap.Get(face, px, py)+profile.Ridged.Amp*mask*r*ampScale)
								}
							}
						}
					}
				} else {
					// Bypass: still consume all ridgedGen draws for rng stability.
					for face := range cubemap.Face(cubemap.NumFaces) {
						for py := range S {
							for px := range S {
								var mask float64
								if usePlateMask {
									distKm := plates.Convergent[face][py*S+px]
									norm := 1.0 - clamp01(distKm/profile.Ridged.PlateConvergentScaleKm)
									mask = smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, norm)
								} else {
									cont := planetcolor.EvalSpline(cfFields[0].Spline, fields[0].Get(face, px, py))
									mask = smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, cont)
								}
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
			usePlateMask := plates != nil && profile.Ridged.PlateConvergentScaleKm > 0
			for face := range cubemap.Face(cubemap.NumFaces) {
				for py := range S {
					for px := range S {
						var rampMod, freqMod float64 = 1, 1
						if provRamp != nil {
							rampMod = provRamp.Get(face, px, py)
							freqMod = provRFreq.Get(face, px, py)
						}
						// Start from the existing value: zero on the legacy
						// path (bit-identical to the old `var h float64`),
						// the crust BaseHeight on the crust path.
						h := heightmap.Get(face, px, py)
						// Only Continentalness, Detail, PeaksValleys (i < 3)
						// contribute to elevation. Temperature and Humidity are
						// climate fields consumed by the biome lookup later.
						for i := range 3 {
							if crustOn && i == 0 {
								continue // crust path: Continentalness no longer drives height
							}
							v := fields[i].Get(face, px, py) * freqMod
							contribution := planetcolor.EvalSpline(cfFields[i].Spline, v) * rampMod
							h += contribution
						}
						if ridgedGen != nil {
							var mask float64
							if usePlateMask {
								distKm := plates.Convergent[face][py*S+px]
								norm := 1.0 - clamp01(distKm/profile.Ridged.PlateConvergentScaleKm)
								mask = smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, norm)
							} else {
								cont := planetcolor.EvalSpline(cfFields[0].Spline, fields[0].Get(face, px, py))
								mask = smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, cont)
							}
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
								ampScale := 1.0
								if usePlateMask {
									ampScale = 0.4 + 0.6*plates.ConvergentMag[face][py*S+px]
								}
								h += profile.Ridged.Amp * mask * r * ampScale
							}
						}
						heightmap.Set(face, px, py, h)
					}
				}
			}
		}
	} else {
		var ridgedGen *noise.Generator
		if !crustOn && profile.Ridged.Amp > 0 && profile.Ridged.Freq > 0 && profile.Ridged.Octaves > 0 {
			ridgedGen = noise.New(pgseed.Domain(seed, "ridged"))
		}
		for face := range cubemap.Face(cubemap.NumFaces) {
			for py := range S {
				for px := range S {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					dx, dy, dz = warper.Warp(dx, dy, dz)
					// Accumulate onto the existing value (zero on the
					// legacy path, crust BaseHeight on the crust path).
					h := heightmap.Get(face, px, py) + ng.FractalNoise3D(dx, dy, dz,
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

	// Phase 12: crust-aware boundary effects (belt/cordillera/trench/
	// arc/ridge/rift/fault), replacing legacy Ridged+Basin on the
	// crust path.
	if crustOn {
		bypassed := bypass["TectonicFX"]
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		if !bypassed {
			fx := field.ClassifyTectonics(plates, crust, profile.RadiusKm)
			field.ApplyTectonicFX(heightmap, fx, crust, plates, profile.TectonicFX, seed, S)
		}
		if frame != nil {
			delta := cubemap.NewF(S)
			for face := range cubemap.Face(cubemap.NumFaces) {
				for i := range heightmap.Faces[face] {
					delta.Faces[face][i] = heightmap.Faces[face][i] - hmBefore.Faces[face][i]
				}
			}
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "TectonicFX",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed,
			})
		}
	}

	// Phase 11: Basin — depress elevation along divergent plate
	// boundaries, scaled by the per-pixel spreading magnitude
	// propagated through JFA. Models mid-ocean-ridge spreading
	// creating ocean basins. Disabled when Basin.Depth == 0 or there
	// are no plates. The crust path replaces it with TectonicFX above.
	if plates != nil && !crustOn && profile.Basin.Depth > 0 && profile.Basin.PlateDivergentScaleKm > 0 {
		bypassed := bypass["Basin"]
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		if !bypassed {
			for face := range cubemap.Face(cubemap.NumFaces) {
				for py := range S {
					for px := range S {
						distKm := plates.Divergent[face][py*S+px]
						falloff := 1.0 - clamp01(distKm/profile.Basin.PlateDivergentScaleKm)
						if falloff <= 0 {
							continue
						}
						mag := plates.DivergentMag[face][py*S+px]
						heightmap.Set(face, px, py, heightmap.Get(face, px, py)-profile.Basin.Depth*falloff*mag)
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
				Name:     "Basin",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed,
			})
		}
	}

	// Phase 4 Task 5: Apply Voronoi continents baseline if enabled.
	// The crust path supersedes it — landmasses come from cratons.
	if profile.Continents.Seeds > 0 && !crustOn {
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		// Phase 8 item 10: anchor continents to continental-plate
		// centroids (non-oceanic plates). Falls back to legacy
		// random Fibonacci-spiral seeding when plates is nil or
		// has no continental plates.
		var contSeeds [][3]float64
		if plates != nil {
			for _, p := range plates.Plates {
				if !p.IsOceanic {
					contSeeds = append(contSeeds, p.Seed)
				}
			}
		}
		cont := field.GenerateContinentsFromSeeds(seed, profile.Continents, S, contSeeds)
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

	// Effective ocean level: the profile's fixed value on the legacy
	// path, or (Phase 12) a histogram quantile of the normalized
	// heightmap hitting the crust's resolved target land fraction.
	oceanLevel := profile.OceanLevel

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
	if crustOn {
		oceanLevel = field.QuantileSeaLevel(heightmap, 1-crust.LandFraction)
	}

	// Phase 4 Task 3: Apply coastal noise enhancement if enabled.
	{
		active := profile.Coastal.Amp > 0 && oceanLevel > 0
		bypassed := bypass["Coastal"]
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		if active && !bypassed {
			distField := field.DistanceToCoast(heightmap, oceanLevel, S)
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
			field.Erode(seed, heightmap, cfg, oceanLevel, S)
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

	// Phase 8 item 15: D8 flow + Planchon-Darboux fill + river carving.
	// Runs after Erosion (which has already smoothed the surface) so rivers
	// cut through the eroded terrain. Disabled when RiverThreshold==0.
	{
		active := profile.Flow.RiverThreshold > 0
		bypassed := bypass["Flow"]
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		if active && !bypassed {
			ff := field.GenerateFlow(heightmap, profile.Flow)
			if ff != nil {
				field.CarveRivers(heightmap, ff, profile.Flow)
				if frame != nil {
					frame.Flow = ff
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
				Name:     "Flow",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed || !active,
			})
		}
	}

	// Phase 12: recompute the quantile once more after Flow — erosion
	// and river carving shift the height histogram slightly, and the
	// land-fraction invariant measures the final heightmap. The earlier
	// computation still matters: Coastal and Erode consumed it
	// mid-pipeline.
	if crustOn {
		oceanLevel = field.QuantileSeaLevel(heightmap, 1-crust.LandFraction)
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
	return heightmap, craters, oceanLevel
}

// colorizeRocky paints the heightmap into a color cube map using the
// profile's palette/biome/ocean/snow/cap/shading/LUT settings.
func colorizeRocky(profile *types.PlanetProfile, seed int64, S int, heightmap *cubemap.CubeMapF, craters []feature.Crater, jitter *noise.JitterField, plates *field.PlateField, flow *field.FlowField) *cubemap.CubeMap {
	return colorizeRockyDebug(profile, seed, S, heightmap, craters, nil, nil, jitter, plates, flow)
}

// colorizeRockyDebug is the debug-aware variant of colorizeRocky. When
// frame is non-nil, it appends a DebugStage after each color pipeline
// stage. When a stage's name is in bypass, the stage is skipped while
// any noise draws are still consumed so downstream streams stay stable.
//
// plates and flow feed the Phase 9b Civ overlay (between Ejecta and
// LUT). Either may be nil — the civ pipeline tolerates missing inputs
// by zeroing the corresponding habitability term.
func colorizeRockyDebug(profile *types.PlanetProfile, seed int64, S int, heightmap *cubemap.CubeMapF, craters []feature.Crater, frame *DebugFrame, bypass DebugBypass, jitter *noise.JitterField, plates *field.PlateField, flow *field.FlowField) *cubemap.CubeMap {
	capNoise := noise.New(seed + 42)
	oceanNoise := noise.New(seed + 77)
	biomeNoise := noise.New(seed + 99)
	warper := noise.NewWarper(seed, profile.Warp)
	if bypass["Warp"] {
		warper.SetBypassed(true)
	}

	// Step 3+4+5: colorise (biome, ocean, snow, polar caps).
	hasBiomes := len(profile.EquatorialPalette) > 0 || len(profile.PolarPalette) > 0
	useBiomeTable := len(profile.BiomeTable.Cells) > 0
	var tField, mField *cubemap.CubeMapF
	if useBiomeTable {
		tField, mField = biome.GenerateClimateFields(seed, profile, S)
	}
	// Phase 8 item 16: orographic rain-shadow multiplier on the humidity
	// (M) input to the Whittaker biome lookup. Disabled when WalkSteps==0.
	var rainShadow *biome.RainShadowField
	if useBiomeTable && profile.RainShadow.WalkSteps > 0 {
		rainShadow = biome.GenerateRainShadow(heightmap, profile.RainShadow)
		if frame != nil {
			frame.RainShadow = rainShadow
		}
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
						m := mField.Get(face, px, py)
						if rainShadow != nil {
							m *= rainShadow.Multiplier[face][py*S+px]
							if m < 0 {
								m = 0
							}
							if m > 1 {
								m = 1
							}
						}
						c = biome.LookupColor(profile.BiomeTable, tField.Get(face, px, py), m, h)
					}
				} else {
					if !bypassPalette {
						c = planetcolor.SampleGradientOkLab(profile.Palette, h)
					}
					if hasBiomes {
						sx, sy, sz := dx, dy, dz
						if jitter != nil {
							// Direction-based lookup: seam-symmetric.
							sx, sy, sz = jitter.Transform(dx, dy, dz)
						}
						biomeVar := biomeNoise.FractalNoise3D(sx, sy, sz, 3, 2.0, 0.5, 4.0)
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
							t := (h - profile.SnowLine) / (1.0 - profile.SnowLine)
							snowBlend := t * t * (3 - 2*t)
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

	// Stage: Civ (Phase 9b item 18). Generates city patches +
	// agriculture + roads as a daytime overlay; the night-side
	// Black-Marble cube map is built but exposed separately via
	// RenderNightCubeMap rather than blended here.
	//
	// Skipped silently when profile.Civ.Tier == 0 — no stage is
	// appended in that case so the production-default archetypes
	// (all currently Tier=0) keep their existing stage counts.
	//
	// SEED-STREAM-MIRROR: any noise stream added before this call
	// must be mirrored in RenderNightCubeMap so its civ output
	// matches what gets blended into the day cube-map. Grep for
	// "SEED-STREAM-MIRROR" to find the paired site.
	if profile.Civ.Tier > 0 {
		civ := feature.GenerateCiv(heightmap, tField, mField, plates, flow, rainShadow, profile, seed, S)
		if civ != nil {
			if frame != nil {
				frame.Civ = civ
			}
			bypassed := bypass["Civ"]
			if !bypassed {
				for face := range cubemap.Face(cubemap.NumFaces) {
					for py := range S {
						for px := range S {
							idx := py*S + px
							a := civ.DayMask[face][idx]
							if a <= 0 {
								continue
							}
							if a > 1 {
								a = 1
							}
							overlay := civ.DayColor.Get(face, px, py)
							c := out.Get(face, px, py)
							out.Set(face, px, py, planetcolor.BlendOkLab(c, overlay, a))
						}
					}
				}
			}
			if frame != nil {
				frame.Stages = append(frame.Stages, DebugStage{
					Name:       "Civ",
					Kind:       "color",
					ColorAfter: out.Clone(),
					Skipped:    bypassed,
				})
			}
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

// clamp01 clamps x to [0,1].
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
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
