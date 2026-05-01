package render

import (
	"image/color"
	"testing"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// flatSmokeProfile returns a minimal PlanetProfile for flat cache smoke tests.
func flatSmokeProfile() *types.PlanetProfile {
	knots := []planetcolor.SplineKnot{
		{Input: 0, Output: 0},
		{Input: 1, Output: 1},
	}
	field := func(amp, freq float64) types.ControlField {
		return types.ControlField{
			Amp: amp, Freq: freq, Octaves: 3,
			Lacunarity: 2, Persistence: 0.5,
			Spline: planetcolor.Spline{Knots: knots},
		}
	}
	return &types.PlanetProfile{
		Renderer:   "rocky",
		OceanLevel: 0.5,
		ControlConfig: types.ControlConfig{
			Continentalness: field(1, 1),
			Detail:          field(0.3, 4),
			PeaksValleys:    field(0.5, 2),
			Temperature:     field(1, 1),
			Humidity:        field(1, 3),
		},
		Palette: []planetcolor.ColorStop{
			{Position: 0, Color: color.RGBA{R: 50, G: 50, B: 50, A: 255}},
			{Position: 1, Color: color.RGBA{R: 200, G: 200, B: 200, A: 255}},
		},
		OceanColor: color.RGBA{R: 30, G: 60, B: 140, A: 255},
	}
}

func TestFlatCacheHitProducesSameOutput(t *testing.T) {
	prof := flatSmokeProfile()
	// First call: cache miss, computes upstream.
	a := RenderFlat(prof, 7, 64)
	// Second call with identical inputs: cache hit, skips upstream.
	b := RenderFlat(prof, 7, 64)
	if a.Bounds() != b.Bounds() {
		t.Fatalf("bounds differ: %v vs %v", a.Bounds(), b.Bounds())
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			t.Fatalf("byte %d differs on cache hit: got %d want %d", i, b.Pix[i], a.Pix[i])
		}
	}
}

func TestFlatCacheStatsIncrement(t *testing.T) {
	// Reset by using a unique seed so we get a fresh cache slot.
	h0, m0 := FlatCacheStats()
	prof := flatSmokeProfile()
	RenderFlat(prof, 99991, 32) // miss
	RenderFlat(prof, 99991, 32) // hit
	h1, m1 := FlatCacheStats()
	if m1-m0 < 1 {
		t.Fatalf("expected at least 1 miss, got %d", m1-m0)
	}
	if h1-h0 < 1 {
		t.Fatalf("expected at least 1 hit, got %d", h1-h0)
	}
}
