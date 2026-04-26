package color

import (
	"image/color"
	"math"
)

// ColorStop represents a color at a specific position in a gradient.
type ColorStop struct {
	Position float64
	Color    color.RGBA
}

// Lerp interpolates between two colors by t [0,1].
func Lerp(a, b color.RGBA, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}

// SampleGradient returns the interpolated color at position t [0,1] in a gradient.
func SampleGradient(stops []ColorStop, t float64) color.RGBA {
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
			return Lerp(stops[i-1].Color, stops[i].Color, localT)
		}
	}
	return stops[len(stops)-1].Color
}

// Blend blends src over dst with the given alpha [0,1].
func Blend(dst, src color.RGBA, alpha float64) color.RGBA {
	alpha = math.Max(0, math.Min(1, alpha))
	return color.RGBA{
		R: uint8(float64(dst.R)*(1-alpha) + float64(src.R)*alpha),
		G: uint8(float64(dst.G)*(1-alpha) + float64(src.G)*alpha),
		B: uint8(float64(dst.B)*(1-alpha) + float64(src.B)*alpha),
		A: 255,
	}
}

// Brighten adjusts a color's brightness by factor (>1 brighter, <1 darker).
func Brighten(c color.RGBA, factor float64) color.RGBA {
	return color.RGBA{
		R: uint8(math.Min(255, float64(c.R)*factor)),
		G: uint8(math.Min(255, float64(c.G)*factor)),
		B: uint8(math.Min(255, float64(c.B)*factor)),
		A: 255,
	}
}
