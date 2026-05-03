package render

import (
	"image/color"
	"math"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
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
func SignedToRGBA(cm *cubemap.CubeMapF, S int, hi float64) *cubemap.CubeMap {
	if hi <= 0 {
		hi = 1
	}
	out := cubemap.New(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				v := cm.Get(face, px, py) / hi
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

// paintCategoricalCubeMap16 maps each unique int16 id to a distinct
// hue via golden-ratio stepping. Negative ids (e.g. PlateID=-1
// sentinel) map to black.
func paintCategoricalCubeMap16(ids [cubemap.NumFaces][]int16, S int) *cubemap.CubeMap {
	out := cubemap.New(S)
	for f := range ids {
		for i, id := range ids[f] {
			if id < 0 {
				out.Faces[f][i] = color.RGBA{A: 255}
				continue
			}
			out.Faces[f][i] = goldenHue(int(id))
		}
	}
	return out
}

func paintOceanicMask(pf *field.PlateField, S int) *cubemap.CubeMap {
	blue := color.RGBA{R: 80, G: 120, B: 200, A: 255}
	brown := color.RGBA{R: 130, G: 100, B: 70, A: 255}
	out := cubemap.New(S)
	for f := range pf.PlateID {
		for i, id := range pf.PlateID[f] {
			if id < 0 || int(id) >= len(pf.Plates) {
				out.Faces[f][i] = color.RGBA{A: 255}
				continue
			}
			if pf.Plates[id].IsOceanic {
				out.Faces[f][i] = blue
			} else {
				out.Faces[f][i] = brown
			}
		}
	}
	return out
}

func scalarFromKmPerFace(km [cubemap.NumFaces][]float64, S int) *cubemap.CubeMapF {
	out := cubemap.NewF(S)
	for f := range km {
		copy(out.Faces[f], km[f])
	}
	return out
}

func goldenHue(id int) color.RGBA {
	const phi = 0.61803398875
	h := math.Mod(float64(id)*phi, 1.0)
	return hsvToRGB(h, 0.65, 0.85)
}

func hsvToRGB(h, s, v float64) color.RGBA {
	i := int(h * 6)
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}
	return color.RGBA{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255), A: 255}
}
