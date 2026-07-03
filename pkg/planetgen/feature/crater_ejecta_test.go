package feature

import (
	"image/color"
	"math"
	"testing"
)

// A young, large crater should brighten pixels just outside its rim.
func TestEjectaBrightensNearFreshCrater(t *testing.T) {
	craters := []Crater{
		{Lat: 0, Lon: 0, Radius: 0.05, Age: 1.0, ParentIdx: -1},
	}
	base := color.RGBA{R: 100, G: 100, B: 100, A: 255}

	// Pixel just outside the rim (at twice the radius) should brighten.
	ang := 0.10
	dx, dy, dz := math.Cos(ang), 0.0, math.Sin(ang)
	got := ApplyEjecta(base, dx, dy, dz, craters)
	if got.R <= base.R {
		t.Errorf("expected brightening near fresh rim; got R=%d, base R=%d", got.R, base.R)
	}
}

// Ejecta should leave pixels far away alone.
func TestEjectaNoOpFarAway(t *testing.T) {
	craters := []Crater{
		{Lat: 0, Lon: 0, Radius: 0.05, Age: 1.0, ParentIdx: -1},
	}
	base := color.RGBA{R: 100, G: 100, B: 100, A: 255}
	got := ApplyEjecta(base, -1, 0, 0, craters)
	if got != base {
		t.Errorf("antipodal pixel should be unchanged; got %v want %v", got, base)
	}
}

// Old craters (low age) should produce far less brightening than young ones.
func TestEjectaAgeAttenuates(t *testing.T) {
	build := func(age float64) uint8 {
		base := color.RGBA{R: 100, G: 100, B: 100, A: 255}
		ang := 0.10
		dx, dy, dz := math.Cos(ang), 0.0, math.Sin(ang)
		return ApplyEjecta(base, dx, dy, dz, []Crater{
			{Lat: 0, Lon: 0, Radius: 0.05, Age: age, ParentIdx: -1},
		}).R
	}
	young := build(1.0)
	old := build(0.4)
	if young <= old {
		t.Errorf("young rays should be brighter; young=%d old=%d", young, old)
	}
}

// Secondaries shouldn't cast rays.
func TestEjectaIgnoresSecondaries(t *testing.T) {
	base := color.RGBA{R: 100, G: 100, B: 100, A: 255}
	ang := 0.10
	dx, dy, dz := math.Cos(ang), 0.0, math.Sin(ang)
	got := ApplyEjecta(base, dx, dy, dz, []Crater{
		{Lat: 0, Lon: 0, Radius: 0.05, Age: 1.0, IsSecondary: true, ParentIdx: 0},
	})
	if got != base {
		t.Errorf("secondaries shouldn't emit rays; got %v want %v", got, base)
	}
}
