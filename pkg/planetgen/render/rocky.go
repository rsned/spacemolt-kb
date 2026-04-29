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

	var craters []feature.Crater
	if profile.CraterCount > 0 {
		craters = feature.GenerateCraters(seed, profile)
		feature.ApplyCraters(heightmap, craters, profile.CraterDepth)
	}
	return heightmap, craters
}

// colorizeRocky paints the heightmap into a color cube map using the
// profile's palette/biome/ocean/snow/cap/shading/LUT settings.
func colorizeRocky(profile *types.PlanetProfile, seed int64, S int, heightmap *cubemap.CubeMapF, craters []feature.Crater) *cubemap.CubeMap {
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

				// Phase 3 polish: slope-based Lambertian shading.  Computes a
				// world-space normal from the heightmap gradient via finite
				// differences against two tangents (seam-aware via
				// CubeMapF.Sample) and modulates color by light·normal.
				if profile.ShadingStrength > 0 {
					exag := profile.ShadingExaggeration
					if exag <= 0 {
						exag = 8.0
					}
					c = applySlopeShading(heightmap, c, rawX, rawY, rawZ,
						profile.ShadingStrength, exag)
				}

				// Phase 3 T8: ejecta-ray albedo overlay around fresh, large
				// craters. Applied before the LUT so the per-archetype grade
				// pulls the rays into the planet's color family. Uses the
				// unwarped direction so rays anchor to the crater on the
				// stored geometry, matching where the bowl actually sits.
				// Only fires when the new crater pipeline is in use; legacy
				// profiles set every crater Age=1.0 and would otherwise gain
				// bright rays everywhere.
				if profile.PowerLawAlpha > 0 && len(craters) > 0 {
					c = feature.ApplyEjecta(c, rawX, rawY, rawZ, craters)
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
