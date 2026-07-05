package patch

import (
	"image"
	"image/color"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
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
