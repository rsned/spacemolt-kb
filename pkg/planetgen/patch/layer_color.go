package patch

import (
	"image"
	"image/color"
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

// applyBiomeColor (layer 10): Whittaker table lookup with
// rain-shadow-multiplied moisture; palette-gradient fallback for
// archetypes without a biome table. Mirrors the Palette/Biome colorize
// stage in render/rocky.go (~L883-946): the rain-shadow multiplier is
// applied to M — and the result clamped to [0,1] — only when rain
// shadow is enabled (RainShadow.WalkSteps > 0), matching production's
// `if rainShadow != nil { ... }` guard exactly. Equatorial/polar
// palette blends are intentionally not reproduced on the patch (spec
// divergence, since crust archetypes are biome-table based).
func applyBiomeColor(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	p := ctx.Profile
	useTable := len(p.BiomeTable.Cells) > 0

	ns := *st
	ns.Img = image.NewRGBA(image.Rect(0, 0, w.Size, w.Size))
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			h := st.Height.Data[i]
			var c color.RGBA
			if useTable {
				m := st.M.Data[i]
				if p.RainShadow.WalkSteps > 0 {
					m *= st.RainMult.Data[i]
					if m < 0 {
						m = 0
					} else if m > 1 {
						m = 1
					}
				}
				c = biome.LookupColor(p.BiomeTable, st.T.Data[i], m, h)
			} else {
				c = planetcolor.SampleGradientOkLab(p.Palette, h)
			}
			o := ns.Img.PixOffset(ix, iy)
			ns.Img.Pix[o], ns.Img.Pix[o+1], ns.Img.Pix[o+2], ns.Img.Pix[o+3] = c.R, c.G, c.B, 255
		}
	}
	return &ns
}

// applyWaterlines (layer 11): the production colorize tail — Snow,
// Ocean, PolarCaps, Shading, Ejecta, LUT — via the shared per-pixel
// helpers in render/pixelcolor.go, mirroring the Snow/Ocean/PolarCaps/
// Shading/Ejecta/LUT stages in render/rocky.go colorizeRockyDebug
// (~L949-1167) term-for-term, gates included. The one intentional
// divergence: the effective sea level is ctx.seaLevelView() (slider
// override, else the post-flow sphere quantile) rather than
// profile.OceanLevel — production hardcodes profile.OceanLevel, but by
// the time colorizeRockyDebug runs that field has already been
// overwritten with the same post-flow quantile (rocky.go L31-33,
// L113-115, L806-808), so ctx.Sphere.SeaLevel is the equivalent value;
// the slider override is the new capability this layer adds. Snow and
// PolarCaps gate on their own profile params directly, unchanged from
// production.
func applyWaterlines(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	p := ctx.Profile
	sea := ctx.seaLevelView()
	warper := noise.NewWarper(ctx.Master, p.Warp)
	capNoise := noise.New(ctx.Master + 42)
	oceanNoise := noise.New(ctx.Master + 77)
	sampler := w.Sampler(st.Height)
	var lut *planetcolor.LUT
	if p.LUT != "" {
		lut = planetcolor.LookupLUT(p.LUT)
	}

	ns := *st
	ns.Img = image.NewRGBA(image.Rect(0, 0, w.Size, w.Size))
	copy(ns.Img.Pix, st.Img.Pix)
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			o := ns.Img.PixOffset(ix, iy)
			c := color.RGBA{R: ns.Img.Pix[o], G: ns.Img.Pix[o+1], B: ns.Img.Pix[o+2], A: 255}
			rawX, rawY, rawZ := w.Dir(ix, iy)
			lat := math.Asin(rawY)
			absLat := math.Abs(lat) / (math.Pi / 2)
			h := st.Height.Data[i]

			if p.SnowLine > 0 && h > p.SnowLine {
				c = render.SnowPixel(c, h, absLat, p.SnowLine)
			}
			if sea > 0 && h < sea {
				dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
				surfaceVar := oceanNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 6.0)
				c = render.OceanPixel(p.OceanColor, p.Type, h, sea, surfaceVar)
			}
			if p.HasPolarCaps && p.PolarCapSize > 0 && absLat > 1.0-p.PolarCapSize {
				dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
				capEdge := capNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 8.0)
				c = render.PolarCapPixel(c, absLat, p.PolarCapSize, p.PolarCapNoise, capEdge)
			}
			if p.ShadingStrength > 0 {
				exag := p.ShadingExaggeration
				if exag <= 0 {
					exag = 8.0
				}
				c = render.SlopeShadeSampled(sampler, c, rawX, rawY, rawZ, p.ShadingStrength, exag)
			}
			if p.PowerLawAlpha > 0 && len(st.Craters) > 0 {
				c = feature.ApplyEjecta(c, rawX, rawY, rawZ, st.Craters)
			}
			if lut != nil {
				c = planetcolor.ApplyLUT(*lut, c)
			}
			ns.Img.Pix[o], ns.Img.Pix[o+1], ns.Img.Pix[o+2], ns.Img.Pix[o+3] = c.R, c.G, c.B, 255
		}
	}
	return &ns
}
