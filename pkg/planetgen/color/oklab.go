package color

import (
	"image/color"
	"math"

	"github.com/lucasb-eyer/go-colorful"
)

// BlendOkLab interpolates between a and b in the OkLab color space, by t∈[0,1].
// Output alpha is forced to 255 (matches the package's other blend conventions).
func BlendOkLab(a, b color.RGBA, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	if t == 0 {
		return color.RGBA{R: a.R, G: a.G, B: a.B, A: 255}
	}
	if t == 1 {
		return color.RGBA{R: b.R, G: b.G, B: b.B, A: 255}
	}
	ca := colorful.Color{
		R: float64(a.R) / 255,
		G: float64(a.G) / 255,
		B: float64(a.B) / 255,
	}
	cb := colorful.Color{
		R: float64(b.R) / 255,
		G: float64(b.G) / 255,
		B: float64(b.B) / 255,
	}
	mix := ca.BlendLab(cb, t).Clamped()
	return color.RGBA{
		R: uint8(math.Round(mix.R * 255)),
		G: uint8(math.Round(mix.G * 255)),
		B: uint8(math.Round(mix.B * 255)),
		A: 255,
	}
}

// SampleGradientOkLab is SampleGradient using OkLab interpolation between
// neighboring stops.
func SampleGradientOkLab(stops []ColorStop, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	if len(stops) == 0 {
		return color.RGBA{128, 128, 128, 255}
	}
	if len(stops) == 1 || t <= stops[0].Position {
		return stops[0].Color
	}
	if t >= stops[len(stops)-1].Position {
		return stops[len(stops)-1].Color
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].Position {
			localT := (t - stops[i-1].Position) / (stops[i].Position - stops[i-1].Position)
			return BlendOkLab(stops[i-1].Color, stops[i].Color, localT)
		}
	}
	return stops[len(stops)-1].Color
}
