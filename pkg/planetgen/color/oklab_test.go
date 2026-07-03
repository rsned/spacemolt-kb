package color

import (
	"image/color"
	"testing"
)

func TestBlendOkLabEndpoints(t *testing.T) {
	a := color.RGBA{R: 255, A: 255}
	b := color.RGBA{B: 255, A: 255}
	if got := BlendOkLab(a, b, 0); got != a {
		t.Errorf("t=0 returned %v, want %v", got, a)
	}
	if got := BlendOkLab(a, b, 1); got != b {
		t.Errorf("t=1 returned %v, want %v", got, b)
	}
}

func TestBlendOkLabAvoidsMuddyMidpoint(t *testing.T) {
	// Red→Blue midpoint in RGB is muddy gray-purple; in OkLab it's
	// closer to a neutral magenta. We don't pin the exact pixel, but
	// we can assert the midpoint has more total saturation than a
	// pure RGB lerp would produce.
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	mid := BlendOkLab(red, blue, 0.5)
	rgbMid := Blend(red, blue, 0.5)
	maxOk := max3(int(mid.R), int(mid.G), int(mid.B))
	minOk := min3(int(mid.R), int(mid.G), int(mid.B))
	maxRGB := max3(int(rgbMid.R), int(rgbMid.G), int(rgbMid.B))
	minRGB := min3(int(rgbMid.R), int(rgbMid.G), int(rgbMid.B))
	if (maxOk - minOk) <= (maxRGB - minRGB) {
		t.Errorf("OkLab midpoint saturation (%d) should exceed RGB midpoint (%d)",
			maxOk-minOk, maxRGB-minRGB)
	}
}

func TestSampleGradientOkLabRetainsEndpoints(t *testing.T) {
	stops := []ColorStop{
		{Position: 0.0, Color: color.RGBA{R: 200, G: 80, B: 30, A: 255}},
		{Position: 1.0, Color: color.RGBA{R: 30, G: 60, B: 200, A: 255}},
	}
	if got := SampleGradientOkLab(stops, 0.0); got != stops[0].Color {
		t.Errorf("t=0 returned %v, want %v", got, stops[0].Color)
	}
	if got := SampleGradientOkLab(stops, 1.0); got != stops[1].Color {
		t.Errorf("t=1 returned %v, want %v", got, stops[1].Color)
	}
}

func max3(a, b, c int) int {
	if a > b && a > c {
		return a
	}
	if b > c {
		return b
	}
	return c
}

func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}
