package render

import (
	"image/color"
	"testing"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// biomeJitterProfile returns a profile that exercises the legacy-palette
// biome-jitter branch (useBiomeTable=false && hasBiomes=true) with jitter
// enabled so noise.GenerateJitter returns a non-nil field.
func biomeJitterProfile() types.PlanetProfile {
	stop := func(pos float64, r, g, b uint8) planetcolor.ColorStop {
		return planetcolor.ColorStop{Position: pos, Color: color.RGBA{R: r, G: g, B: b, A: 255}}
	}
	return types.PlanetProfile{
		Type:     "test_biome_jitter",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			stop(0.0, 100, 60, 30),
			stop(0.5, 160, 110, 70),
			stop(1.0, 220, 190, 150),
		},
		EquatorialPalette: []planetcolor.ColorStop{
			stop(0.0, 190, 170, 120),
			stop(0.5, 180, 150, 100),
			stop(1.0, 200, 195, 170),
		},
		// BiomeTable deliberately left zero-value so useBiomeTable=false.
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{
				Amp: 1, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5,
				Spline: planetcolor.Spline{
					Knots: []planetcolor.SplineKnot{
						{Input: 0, Output: 0},
						{Input: 0.5, Output: 0.3},
						{Input: 1, Output: 0.5},
					},
				},
			},
		},
		JitterEnabled:   true,
		JitterCellCount: 48,
		JitterRotMax:    0.8,
		JitterOffsetMax: 0.1,
	}
}

// TestBiomeJitterAffectsOutput renders the same planet twice — once with
// nil jitter and once with a populated JitterField — and asserts that at
// least 0.5% of pixels differ. The biome-jitter branch only fires inside
// the equatorial blend region, so the threshold is intentionally low.
func TestBiomeJitterAffectsOutput(t *testing.T) {
	const S = 32
	const seed int64 = 77

	prof := biomeJitterProfile()
	hm, craters := generateRockyHeightmap(&prof, seed, S)

	// Render without jitter.
	noJitter := colorizeRockyDebug(&prof, seed, S, hm, craters, nil, nil, nil)

	// Render with jitter.
	jf := noise.GenerateJitter(&prof, seed, S)
	if jf == nil {
		t.Fatal("GenerateJitter returned nil for an enabled profile")
	}
	withJitter := colorizeRockyDebug(&prof, seed, S, hm, craters, nil, nil, jf)

	// Count differing pixels across all 6 faces.
	total := 0
	diff := 0
	for face := range 6 {
		for py := range S {
			for px := range S {
				total++
				a := noJitter.Faces[face][py*S+px]
				b := withJitter.Faces[face][py*S+px]
				if a != b {
					diff++
				}
			}
		}
	}

	pct := float64(diff) / float64(total) * 100
	t.Logf("biome-jitter diff: %d/%d pixels (%.2f%%)", diff, total, pct)
	if pct < 0.5 {
		t.Errorf("expected ≥0.5%% pixels to differ, got %.2f%%", pct)
	}
}
