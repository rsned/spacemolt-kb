package render

import (
	"image/color"
	"testing"
)

func TestSnowPixelBlends(t *testing.T) {
	base := color.RGBA{50, 90, 40, 255}
	got := SnowPixel(base, 0.9, 0.5, 0.6)
	if got == base {
		t.Fatal("SnowPixel above snowline must lighten the pixel")
	}
	// Higher terrain → more snow (monotone in h).
	hi := SnowPixel(base, 0.98, 0.5, 0.6)
	if int(hi.R)+int(hi.G)+int(hi.B) < int(got.R)+int(got.G)+int(got.B) {
		t.Fatal("snow blend must be monotone in height")
	}
}

func TestOceanPixelDepthDarkens(t *testing.T) {
	oc := color.RGBA{10, 40, 120, 255}
	shallow := OceanPixel(oc, "terran", 0.29, 0.30, 0.5)
	deep := OceanPixel(oc, "terran", 0.02, 0.30, 0.5)
	if int(deep.R)+int(deep.G)+int(deep.B) >= int(shallow.R)+int(shallow.G)+int(shallow.B) {
		t.Fatal("deep ocean must be darker than shallow")
	}
}

func TestSlopeShadeSampledFlatIsNeutralish(t *testing.T) {
	flat := func(x, y, z float64) float64 { return 0.5 }
	base := color.RGBA{100, 100, 100, 255}
	got := SlopeShadeSampled(flat, base, 0, 0, 1, 0.5, 8)
	// Flat terrain: normal == radial; brightness = (1-s) + s*(0.4+0.8*diff).
	if got.R < 80 || got.R > 130 {
		t.Fatalf("flat shading out of neutral band: %v", got)
	}
}
