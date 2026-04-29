package render

import (
	"image/color"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// bandPalette is the fixed five-color palette for spline-band
// classification. Index 0 is the lowest band; out-of-range pixels
// render black.
var bandPalette = [...]color.RGBA{
	{R: 70, G: 200, B: 220, A: 255},  // cyan
	{R: 60, G: 110, B: 230, A: 255},  // blue
	{R: 80, G: 200, B: 80, A: 255},   // green
	{R: 240, G: 220, B: 60, A: 255},  // yellow
	{R: 230, G: 80, B: 60, A: 255},   // red
}

// ClassifySplineInputBands returns a cube map whose pixels are colored
// by which input-axis knot interval the corresponding scalar value
// lands in. For a spline with N knots, intervals are
// [Knot[0].Input, Knot[1].Input), …, [Knot[N-2].Input, Knot[N-1].Input].
func ClassifySplineInputBands(values *cubemap.CubeMapF, spline planetcolor.Spline, S int) *cubemap.CubeMap {
	out := cubemap.New(S)
	knots := spline.Knots
	if len(knots) < 2 {
		return out
	}
	intervals := len(knots) - 1
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				v := values.Get(face, px, py)
				idx := -1
				for i := 0; i < intervals; i++ {
					if v >= knots[i].Input && v <= knots[i+1].Input {
						idx = i
						break
					}
				}
				if idx < 0 {
					out.Set(face, px, py, color.RGBA{A: 255})
					continue
				}
				out.Set(face, px, py, bandPalette[idx%len(bandPalette)])
			}
		}
	}
	return out
}

// ClassifySplineOutputBands classifies on the spline's output axis.
func ClassifySplineOutputBands(values *cubemap.CubeMapF, spline planetcolor.Spline, S int) *cubemap.CubeMap {
	out := cubemap.New(S)
	knots := spline.Knots
	if len(knots) < 2 {
		return out
	}
	intervals := len(knots) - 1
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				outVal := planetcolor.EvalSpline(spline, values.Get(face, px, py))
				idx := -1
				for i := 0; i < intervals; i++ {
					lo, hi := knots[i].Output, knots[i+1].Output
					if hi < lo {
						lo, hi = hi, lo
					}
					if outVal >= lo && outVal <= hi {
						idx = i
						break
					}
				}
				if idx < 0 {
					out.Set(face, px, py, color.RGBA{A: 255})
					continue
				}
				out.Set(face, px, py, bandPalette[idx%len(bandPalette)])
			}
		}
	}
	return out
}

// SignedToRGBA renders a signed scalar field with grayscale for
// non-negative values and a red-intensity ramp for negative values.
// hi is the absolute scaling factor (values divided by hi before
// mapping; clamped to [-1, 1]).
func SignedToRGBA(field *cubemap.CubeMapF, S int, hi float64) *cubemap.CubeMap {
	if hi <= 0 {
		hi = 1
	}
	out := cubemap.New(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				v := field.Get(face, px, py) / hi
				var c color.RGBA
				if v >= 0 {
					if v > 1 {
						v = 1
					}
					g := uint8(v * 255)
					c = color.RGBA{R: g, G: g, B: g, A: 255}
				} else {
					if v < -1 {
						v = -1
					}
					r := uint8(-v * 255)
					c = color.RGBA{R: r, G: 0, B: 0, A: 255}
				}
				out.Set(face, px, py, c)
			}
		}
	}
	return out
}
