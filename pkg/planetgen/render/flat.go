// Package render: flat 2D-plane render path. Produces a fast square
// preview that mirrors the rocky cube-sphere pipeline minus the
// sphere-specific stages (continents, voronoi, craters, polar caps).
// For tuning only — not the canonical output.
package render

import (
	"image"
	"image/color"
	"math"
	"sync/atomic"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// flatControlFieldDomains mirrors field.controlFieldDomains so the same
// seed-derivation tree is used for the 2D render path.
var flatControlFieldDomains = [5]string{
	"control.continentalness",
	"control.erosion",
	"control.peaks-valleys",
	"biome.temperature",
	"biome.humidity",
}

// RenderFlat produces a size×size flat preview using the rocky
// pipeline minus sphere-specific stages. Noise sampling uses 3D unit-sphere
// directions of the +Z face so patterns match the cube render.
// Stages 1-4 (control fields → sum → smooth → normalize) are memoised in
// flatUpstreamCache keyed by the upstream-affecting inputs; downstream-only
// tweaks (Coastal, Erosion, Colorize) yield cache hits and skip the slow path.
func RenderFlat(prof *types.PlanetProfile, masterSeed int64, size int) *image.RGBA {
	var fields [5][]float64
	var heightmap []float64

	key := flatUpstreamKey(prof, masterSeed, size)
	if entry, ok := flatUpstreamCache.get(key); ok {
		// Cache hit: restore upstream results without recomputing.
		atomic.AddUint64(&flatCacheHits, 1)
		heightmap = make([]float64, len(entry.Heightmap))
		copy(heightmap, entry.Heightmap)
		fields[3] = make([]float64, len(entry.TempField))
		copy(fields[3], entry.TempField)
		fields[4] = make([]float64, len(entry.HumField))
		copy(fields[4], entry.HumField)
	} else {
		// Cache miss: run stages 1-4 and store the result.
		atomic.AddUint64(&flatCacheMisses, 1)

		// 1. Generate 5 control fields as size×size float grids.
		fields = generateFlatControlFields(prof, masterSeed, size)

		// 2. Sum first 3 fields' splined contributions into heightmap.
		cf := orderedControlFieldsFlat(prof.ControlConfig)
		heightmap = make([]float64, size*size)
		for i := range 3 {
			for py := range size {
				for px := range size {
					v := fields[i][py*size+px]
					heightmap[py*size+px] += planetcolor.EvalSpline(cf[i].Spline, v)
				}
			}
		}

		// 3. HeightSmooth (per-axis disc blur, edge-clipped).
		if prof.HeightSmoothRadius > 0 {
			heightmap = smoothFlat(heightmap, size, prof.HeightSmoothRadius)
		}

		// 4. Normalize to [0,1].
		normalizeFlat(heightmap)

		// Snapshot upstream results for future cache hits.
		hmCopy := make([]float64, len(heightmap))
		copy(hmCopy, heightmap)
		tCopy := make([]float64, len(fields[3]))
		copy(tCopy, fields[3])
		mCopy := make([]float64, len(fields[4]))
		copy(mCopy, fields[4])
		flatUpstreamCache.put(key, flatCacheEntry{Heightmap: hmCopy, TempField: tCopy, HumField: mCopy})
	}

	// 5. Coastal (if enabled).
	if prof.Coastal.Amp > 0 && prof.OceanLevel > 0 {
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

	// 6. Erosion (if enabled). 2D droplet sim adapted from the sphere walk.
	if prof.Erosion.Droplets > 0 {
		n := scaleErosionDropletsFlat(prof.Erosion.Droplets, size)
		if n > 0 {
			cfg := prof.Erosion
			cfg.Droplets = n
			erodeFlat(masterSeed, heightmap, size, cfg, prof.OceanLevel)
		}
	}

	// 7. Colorize.
	return colorizeFlat(prof, masterSeed, fields, heightmap, size, nil)
}

// generateFlatControlFields samples noise at 3D unit-sphere directions of the
// +Z face so patterns match the cube render for the same seed.
func generateFlatControlFields(prof *types.PlanetProfile, masterSeed int64, size int) [5][]float64 {
	cf := orderedControlFieldsFlat(prof.ControlConfig)
	out := [5][]float64{}
	for i := range out {
		ng := noise.New(seed.Domain(masterSeed, flatControlFieldDomains[i]))
		out[i] = make([]float64, size*size)
		for py := range size {
			for px := range size {
				dx, dy, dz := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
				out[i][py*size+px] = ng.FractalNoise3D(
					dx, dy, dz,
					cf[i].Octaves, cf[i].Lacunarity, cf[i].Persistence, cf[i].Freq,
				) * cf[i].Amp
			}
		}
	}
	return out
}

func orderedControlFieldsFlat(c types.ControlConfig) [5]types.ControlField {
	return [5]types.ControlField{c.Continentalness, c.Detail, c.PeaksValleys, c.Temperature, c.Humidity}
}

func smoothFlat(hm []float64, size, r int) []float64 {
	if r <= 0 {
		return hm
	}
	side := 2*r + 1
	weights := make([]float64, side*side)
	sum := 0.0
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			d := math.Sqrt(float64(dx*dx + dy*dy))
			w := 1.0 / (1.0 + d)
			weights[(dy+r)*side+(dx+r)] = w
			sum += w
		}
	}
	for i := range weights {
		weights[i] /= sum
	}
	out := make([]float64, size*size)
	for py := range size {
		for px := range size {
			acc, wsum := 0.0, 0.0
			for dy := -r; dy <= r; dy++ {
				iy := py + dy
				if iy < 0 || iy >= size {
					continue
				}
				for dx := -r; dx <= r; dx++ {
					ix := px + dx
					if ix < 0 || ix >= size {
						continue
					}
					w := weights[(dy+r)*side+(dx+r)]
					acc += hm[iy*size+ix] * w
					wsum += w
				}
			}
			if wsum > 0 {
				out[py*size+px] = acc / wsum
			}
		}
	}
	return out
}

func normalizeFlat(hm []float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range hm {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi-lo < 1e-9 {
		return
	}
	for i := range hm {
		hm[i] = (hm[i] - lo) / (hi - lo)
	}
}

// distanceToCoastFlat computes per-pixel distance to the nearest ocean pixel
// (heightmap < threshold) via forward+backward raster scan, normalized to [0,1].
func distanceToCoastFlat(hm []float64, size int, threshold float64) []float64 {
	const big = 1e30
	d := make([]float64, size*size)
	for i, v := range hm {
		if v < threshold {
			d[i] = 0
		} else {
			d[i] = big
		}
	}
	// Forward pass.
	for py := range size {
		for px := range size {
			idx := py*size + px
			if d[idx] == 0 {
				continue
			}
			best := d[idx]
			if px > 0 {
				if cand := d[idx-1] + 1; cand < best {
					best = cand
				}
			}
			if py > 0 {
				if cand := d[idx-size] + 1; cand < best {
					best = cand
				}
				if px > 0 {
					if cand := d[idx-size-1] + math.Sqrt2; cand < best {
						best = cand
					}
				}
				if px < size-1 {
					if cand := d[idx-size+1] + math.Sqrt2; cand < best {
						best = cand
					}
				}
			}
			d[idx] = best
		}
	}
	// Backward pass.
	for py := size - 1; py >= 0; py-- {
		for px := size - 1; px >= 0; px-- {
			idx := py*size + px
			if d[idx] == 0 {
				continue
			}
			best := d[idx]
			if px < size-1 {
				if cand := d[idx+1] + 1; cand < best {
					best = cand
				}
			}
			if py < size-1 {
				if cand := d[idx+size] + 1; cand < best {
					best = cand
				}
				if px > 0 {
					if cand := d[idx+size-1] + math.Sqrt2; cand < best {
						best = cand
					}
				}
				if px < size-1 {
					if cand := d[idx+size+1] + math.Sqrt2; cand < best {
						best = cand
					}
				}
			}
			d[idx] = best
		}
	}
	// Normalize by diagonal so the result is in [0,1].
	diag := math.Sqrt(float64(size*size + size*size))
	for i := range d {
		d[i] /= diag
		if d[i] > 1 {
			d[i] = 1
		}
	}
	return d
}

func scaleErosionDropletsFlat(canonical, size int) int {
	// Scale by pixel area relative to canonical face size of 1024.
	const baseSize = 1024
	scale := float64(size*size) / float64(baseSize*baseSize)
	n := int(float64(canonical) * scale)
	if n < 5000 && canonical >= 5000 {
		n = 5000
	}
	return n
}

// colorizeFlat mirrors colorizeRockyDebug for the +Z face. Snow, Ocean,
// PolarCaps, and Shading stages are applied in the same order and with the
// same logic; the same noise seeds (masterSeed+42/77/99) guarantee parity.
func colorizeFlat(prof *types.PlanetProfile, masterSeed int64, fields [5][]float64, hm []float64, size int, jitter *noise.JitterField) *image.RGBA {
	capNoise := noise.New(masterSeed + 42)
	oceanNoise := noise.New(masterSeed + 77)
	biomeNoise := noise.New(masterSeed + 99)
	warper := noise.NewWarper(masterSeed, prof.Warp)

	hasBiomes := len(prof.EquatorialPalette) > 0 || len(prof.PolarPalette) > 0
	useBiomeTable := len(prof.BiomeTable.Cells) > 0

	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Stage: Palette/Biome base color.
	for py := range size {
		for px := range size {
			idx := py*size + px
			h := hm[idx]
			rawX, rawY, rawZ := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
			dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
			lat := math.Asin(rawY)
			absLat := math.Abs(lat) / (math.Pi / 2)
			var c color.RGBA
			if useBiomeTable {
				// Replicate GenerateClimateFields lat-bias for temperature.
				latBias := 0.5 + 0.5*math.Cos(lat)*0.6
				t := fields[3][idx]*0.7 + latBias*0.3
				if t < 0 {
					t = 0
				} else if t > 1 {
					t = 1
				}
				c = biome.LookupColor(prof.BiomeTable, t, fields[4][idx], h)
			} else {
				c = planetcolor.SampleGradientOkLab(prof.Palette, h)
				if hasBiomes {
					sx, sy, sz := dx, dy, dz
					if jitter != nil {
						sx, sy, sz = jitter.TransformPixel(cubemap.FacePosZ, px, py, dx, dy, dz)
					}
					biomeVar := biomeNoise.FractalNoise3D(sx, sy, sz, 3, 2.0, 0.5, 4.0)
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
			c.A = 255
			img.SetRGBA(px, py, c)
		}
	}

	// Stage: Snow.
	if prof.SnowLine > 0 {
		for py := range size {
			for px := range size {
				idx := py*size + px
				h := hm[idx]
				if h > prof.SnowLine {
					rawX, rawY, rawZ := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
					_ = rawX
					_ = rawZ
					lat := math.Asin(rawY)
					absLat := math.Abs(lat) / (math.Pi / 2)
					c := img.RGBAAt(px, py)
					t := (h - prof.SnowLine) / (1.0 - prof.SnowLine)
					snowBlend := t * t * (3 - 2*t)
					snowBlend = math.Min(1.0, snowBlend*1.5)
					latBoost := 1.0 + absLat*0.5
					snowBlend = math.Min(1.0, snowBlend*latBoost)
					img.SetRGBA(px, py, planetcolor.BlendOkLab(c, whiteSnow, snowBlend*0.85))
				}
			}
		}
	}

	// Stage: Ocean.
	if prof.OceanLevel > 0 {
		for py := range size {
			for px := range size {
				idx := py*size + px
				h := hm[idx]
				if h < prof.OceanLevel {
					rawX, rawY, rawZ := cubemap.FacePixelToDir(cubemap.FacePosZ, px, py, size)
					dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
					depth := (prof.OceanLevel - h) / prof.OceanLevel
					surfaceVar := oceanNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 6.0)
					_ = idx
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
					img.SetRGBA(px, py, c)
				}
			}
		}
	}

	// Stage: PolarCaps.
	if prof.HasPolarCaps && prof.PolarCapSize > 0 {
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
						c := img.RGBAAt(px, py)
						blend := math.Min(1.0, (absLat-adjustedThreshold)*15)
						capColor := planetcolor.Brighten(whiteIce, 0.9+capEdgeNoise*0.2)
						img.SetRGBA(px, py, planetcolor.BlendOkLab(c, capColor, blend))
					}
				}
			}
		}
	}

	// Stage: Shading (slope-based Lambertian).
	if prof.ShadingStrength > 0 {
		exag := prof.ShadingExaggeration
		if exag <= 0 {
			exag = 8.0
		}
		for py := range size {
			for px := range size {
				c := img.RGBAAt(px, py)
				img.SetRGBA(px, py, applySlopeShadingFlat(hm, size, c, px, py, prof.ShadingStrength, exag))
			}
		}
	}

	return img
}

// applySlopeShadingFlat applies a Lambertian shading pass to a flat heightmap
// pixel. Slope is estimated from 2D finite differences; the same sun direction
// and formula as applySlopeShading are used so the two paths produce
// equivalent output.
func applySlopeShadingFlat(hm []float64, size int, c color.RGBA, ix, iy int, strength, exaggeration float64) color.RGBA {
	// Clamp neighbors to image bounds.
	x0 := ix - 1
	if x0 < 0 {
		x0 = 0
	}
	x1 := ix + 1
	if x1 >= size {
		x1 = size - 1
	}
	y0 := iy - 1
	if y0 < 0 {
		y0 = 0
	}
	y1 := iy + 1
	if y1 >= size {
		y1 = size - 1
	}
	dHdx := (hm[iy*size+x1] - hm[iy*size+x0]) / float64(x1-x0) * exaggeration
	dHdy := (hm[y1*size+ix] - hm[y0*size+ix]) / float64(y1-y0) * exaggeration
	// Normal in the local tangent frame: (-dHdx, -dHdy, 1) normalized.
	nx, ny, nz := -dHdx, -dHdy, 1.0
	nn := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if nn == 0 {
		return c
	}
	nx, ny, nz = nx/nn, ny/nn, nz/nn
	// Fixed sun direction matching applySlopeShading.
	const lx, ly, lz = 0.6172, 0.6172, 0.4881
	diff := nx*lx + ny*ly + nz*lz
	if diff < 0 {
		diff = 0
	}
	bright := (1.0 - strength) + strength*(0.4+0.8*diff)
	return planetcolor.Brighten(c, bright)
}
