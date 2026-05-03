package render

import (
	"image/color"
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// FlatDebugStage captures one stage's output in the flat pipeline.
type FlatDebugStage struct {
	// Name is the canonical stage identifier (matches bypass keys).
	Name string
	// Kind is "height" or "color".
	Kind string
	// RawDelta is the signed per-pixel delta for height stages (nil for color).
	RawDelta []float64
	// SumAfter is the running heightmap after this height stage (nil for color).
	SumAfter []float64
	// ColorAfter holds the RGBA frame after this color stage (nil for height).
	ColorAfter []color.RGBA
	// Skipped is true when bypass suppressed this stage's mutation.
	Skipped bool
}

// FlatDebugFrame collects all stages produced by RenderFlatDebug.
type FlatDebugFrame struct {
	Size   int
	Stages []FlatDebugStage
}

// FlatDebugBypass is the set of stage names to skip during flat debug capture.
type FlatDebugBypass map[string]bool

// RenderFlatDebug intentionally does NOT use flatUpstreamCache. The debug
// panel needs to capture every stage's actual computation, so caching would
// break the visualization.
//
// RenderFlatDebug mirrors RenderFlat but captures per-stage state into a
// FlatDebugFrame. Stages listed in bypass have their mutation suppressed but
// still emit a Skipped=true entry. Pass nil to run all stages normally.
func RenderFlatDebug(prof *types.PlanetProfile, masterSeed int64, size int, bypass FlatDebugBypass, jitter *noise.JitterField) FlatDebugFrame {
	frame := FlatDebugFrame{Size: size}
	n := size * size

	// snapH returns a copy of the float64 slice.
	snapH := func(hm []float64) []float64 {
		c := make([]float64, n)
		copy(c, hm)
		return c
	}

	// diff returns post[i] - pre[i] for every pixel.
	diff := func(pre, post []float64) []float64 {
		d := make([]float64, n)
		for i := range n {
			d[i] = post[i] - pre[i]
		}
		return d
	}

	// snapC returns a copy of the RGBA buffer.
	snapC := func(buf []color.RGBA) []color.RGBA {
		c := make([]color.RGBA, n)
		copy(c, buf)
		return c
	}

	// === Height stages ===

	// 1. Generate all 5 control fields.
	fields := generateFlatControlFields(prof, masterSeed, size)
	cf := orderedControlFieldsFlat(prof.ControlConfig)
	heightmap := make([]float64, n)

	// Stages: Continentalness, Detail, PeaksValleys (control fields 0-2).
	fieldNames := [3]string{"Continentalness", "Detail", "PeaksValleys"}
	for i := range 3 {
		name := fieldNames[i]
		bypassed := bypass[name]
		pre := snapH(heightmap)
		if !bypassed {
			for py := range size {
				for px := range size {
					idx := py*size + px
					heightmap[idx] += planetcolor.EvalSpline(cf[i].Spline, fields[i][idx])
				}
			}
		}
		post := snapH(heightmap)
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:     name,
			Kind:     "height",
			RawDelta: diff(pre, post),
			SumAfter: post,
			Skipped:  bypassed,
		})
	}

	// Stage: HeightSmooth.
	{
		name := "HeightSmooth"
		bypassed := bypass[name]
		pre := snapH(heightmap)
		if !bypassed && prof.HeightSmoothRadius > 0 {
			heightmap = smoothFlat(heightmap, size, prof.HeightSmoothRadius)
		}
		post := snapH(heightmap)
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:     name,
			Kind:     "height",
			RawDelta: diff(pre, post),
			SumAfter: post,
			Skipped:  bypassed,
		})
	}

	// Stage: Normalize.
	{
		name := "Normalize"
		bypassed := bypass[name]
		pre := snapH(heightmap)
		if !bypassed {
			normalizeFlat(heightmap)
		}
		post := snapH(heightmap)
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:     name,
			Kind:     "height",
			RawDelta: diff(pre, post),
			SumAfter: post,
			Skipped:  bypassed,
		})
	}

	// Stage: Coastal (only emitted when configured).
	if prof.Coastal.Amp > 0 && prof.OceanLevel > 0 {
		name := "Coastal"
		bypassed := bypass[name]
		pre := snapH(heightmap)
		if !bypassed {
			dist := distanceToCoastFlat(heightmap, size, prof.OceanLevel)
			coastGen := noise.NewCoastalGen(seed.Domain(masterSeed, "coastal"))
			for py := range size {
				for px := range size {
					idx := py*size + px
					dx, dy, dz := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
					heightmap[idx] = noise.ApplyCoastal(
						coastGen, dx, dy, dz,
						heightmap[idx], dist[idx],
						prof.Coastal.Amp, prof.Coastal.Threshold, prof.Coastal.Freq)
				}
			}
		}
		post := snapH(heightmap)
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:     name,
			Kind:     "height",
			RawDelta: diff(pre, post),
			SumAfter: post,
			Skipped:  bypassed,
		})
	}

	// Stage: Erosion (only emitted when configured).
	if prof.Erosion.Droplets > 0 {
		name := "Erosion"
		bypassed := bypass[name]
		pre := snapH(heightmap)
		if !bypassed {
			nd := scaleErosionDropletsFlat(prof.Erosion.Droplets, size)
			if nd > 0 {
				cfg := prof.Erosion
				cfg.Droplets = nd
				erodeFlat(masterSeed, heightmap, size, cfg, prof.OceanLevel)
			}
		}
		post := snapH(heightmap)
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:     name,
			Kind:     "height",
			RawDelta: diff(pre, post),
			SumAfter: post,
			Skipped:  bypassed,
		})
	}

	// === Color stages ===
	// Build the color buffer stage-by-stage, mirroring colorizeFlat exactly.

	capNoise := noise.New(masterSeed + 42)
	oceanNoise := noise.New(masterSeed + 77)
	biomeNoise := noise.New(masterSeed + 99)
	warper := noise.NewWarper(masterSeed, prof.Warp)
	hasBiomes := len(prof.EquatorialPalette) > 0 || len(prof.PolarPalette) > 0
	useBiomeTable := len(prof.BiomeTable.Cells) > 0

	buf := make([]color.RGBA, n)

	// Stage: Palette base color.
	{
		bypassed := bypass["Palette"]
		for py := range size {
			for px := range size {
				idx := py*size + px
				h := heightmap[idx]
				rawX, rawY, rawZ := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
				dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
				lat := math.Asin(rawY)
				absLat := math.Abs(lat) / (math.Pi / 2)
				var c color.RGBA
				if useBiomeTable {
					latBias := 0.5 + 0.5*math.Cos(lat)*0.6
					t := fields[3][idx]*0.7 + latBias*0.3
					if t < 0 {
						t = 0
					} else if t > 1 {
						t = 1
					}
					if !bypassed {
						c = biome.LookupColor(prof.BiomeTable, t, fields[4][idx], h)
					}
				} else {
					if !bypassed {
						c = planetcolor.SampleGradientOkLab(prof.Palette, h)
					}
					if hasBiomes {
						sx, sy, sz := dx, dy, dz
						if jitter != nil {
							sx, sy, sz = jitter.TransformPixel(cubemap.FacePosZ, px, py, dx, dy, dz)
						}
						biomeVar := biomeNoise.FractalNoise3D(sx, sy, sz, 3, 2.0, 0.5, 4.0)
						if !bypassed {
							adjustedLat := absLat + (biomeVar-0.5)*0.15
							if len(prof.EquatorialPalette) > 0 && adjustedLat < 0.35 {
								eqColor := planetcolor.SampleGradientOkLab(prof.EquatorialPalette, h)
								eqBlend := 1.0 - adjustedLat/0.35
								eqBlend *= eqBlend
								c = planetcolor.BlendOkLab(c, eqColor, eqBlend*0.8)
							}
							if len(prof.PolarPalette) > 0 && adjustedLat > 0.6 {
								polColor := planetcolor.SampleGradientOkLab(prof.PolarPalette, h)
								polBlend := (adjustedLat - 0.6) / 0.4
								polBlend *= polBlend
								c = planetcolor.BlendOkLab(c, polColor, polBlend*0.7)
							}
						}
					}
				}
				if !bypassed {
					c.A = 255
				}
				buf[idx] = c
			}
		}
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:       "Palette",
			Kind:       "color",
			ColorAfter: snapC(buf),
			Skipped:    bypassed,
		})
	}

	// Stage: Snow.
	if prof.SnowLine > 0 {
		bypassed := bypass["Snow"]
		if !bypassed {
			for py := range size {
				for px := range size {
					idx := py*size + px
					h := heightmap[idx]
					if h > prof.SnowLine {
						rawX, rawY, rawZ := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
						_ = rawX
						_ = rawZ
						lat := math.Asin(rawY)
						absLat := math.Abs(lat) / (math.Pi / 2)
						c := buf[idx]
						t := (h - prof.SnowLine) / (1.0 - prof.SnowLine)
						snowBlend := t * t * (3 - 2*t)
						snowBlend = math.Min(1.0, snowBlend*1.5)
						latBoost := 1.0 + absLat*0.5
						snowBlend = math.Min(1.0, snowBlend*latBoost)
						buf[idx] = planetcolor.BlendOkLab(c, whiteSnow, snowBlend*0.85)
					}
				}
			}
		}
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:       "Snow",
			Kind:       "color",
			ColorAfter: snapC(buf),
			Skipped:    bypassed,
		})
	}

	// Stage: Ocean.
	if prof.OceanLevel > 0 {
		bypassed := bypass["Ocean"]
		for py := range size {
			for px := range size {
				idx := py*size + px
				h := heightmap[idx]
				if h < prof.OceanLevel {
					rawX, rawY, rawZ := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
					dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
					depth := (prof.OceanLevel - h) / prof.OceanLevel
					surfaceVar := oceanNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 6.0)
					if !bypassed {
						var c color.RGBA
						if prof.Type == "lava_world" {
							brightness := 0.7 + depth*0.3
							if depth < 0.2 {
								brightness *= 0.6 + depth*2.0
							}
							brightness += (surfaceVar - 0.5) * 0.25
							brightness = math.Max(0.4, math.Min(1.2, brightness))
							lavaColor := planetcolor.Lerp(
								prof.OceanColor,
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
							c = planetcolor.Brighten(prof.OceanColor, brightness)
						}
						buf[idx] = c
					}
				}
			}
		}
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:       "Ocean",
			Kind:       "color",
			ColorAfter: snapC(buf),
			Skipped:    bypass["Ocean"],
		})
	}

	// Stage: PolarCaps (only when enabled).
	if prof.HasPolarCaps && prof.PolarCapSize > 0 {
		bypassed := bypass["PolarCaps"]
		if !bypassed {
			for py := range size {
				for px := range size {
					rawX, rawY, rawZ := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
					dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
					lat := math.Asin(rawY)
					absLat := math.Abs(lat) / (math.Pi / 2)
					capThreshold := 1.0 - prof.PolarCapSize
					if absLat > capThreshold {
						capEdgeNoise := capNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 8.0)
						noiseAmt := prof.PolarCapNoise
						if noiseAmt == 0 {
							noiseAmt = 0.08
						}
						adjustedThreshold := capThreshold + (capEdgeNoise-0.5)*noiseAmt
						if absLat > adjustedThreshold {
							idx := py*size + px
							c := buf[idx]
							blend := math.Min(1.0, (absLat-adjustedThreshold)*15)
							capColor := planetcolor.Brighten(whiteIce, 0.9+capEdgeNoise*0.2)
							buf[idx] = planetcolor.BlendOkLab(c, capColor, blend)
						}
					}
				}
			}
		}
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:       "PolarCaps",
			Kind:       "color",
			ColorAfter: snapC(buf),
			Skipped:    bypassed,
		})
	}

	// Stage: Shading (only when enabled).
	if prof.ShadingStrength > 0 {
		bypassed := bypass["Shading"]
		if !bypassed {
			exag := prof.ShadingExaggeration
			if exag <= 0 {
				exag = 8.0
			}
			for py := range size {
				for px := range size {
					idx := py*size + px
					buf[idx] = applySlopeShadingFlat(heightmap, size, buf[idx], px, py, prof.ShadingStrength, exag)
				}
			}
		}
		frame.Stages = append(frame.Stages, FlatDebugStage{
			Name:       "Shading",
			Kind:       "color",
			ColorAfter: snapC(buf),
			Skipped:    bypassed,
		})
	}

	return frame
}
