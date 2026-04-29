package render

import (
	"image/color"
	"testing"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// minimalRockyProfile builds a PlanetProfile that exercises only the
// Continentalness control field. All other pipeline stages are disabled
// so the test is fast and deterministic.
func minimalRockyProfile() types.PlanetProfile {
	return types.PlanetProfile{
		Type:     "test",
		Renderer: "rocky",
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{
				Amp:         1,
				Freq:        1,
				Octaves:     2,
				Lacunarity:  2,
				Persistence: 0.5,
				Spline: planetcolor.Spline{
					Knots: []planetcolor.SplineKnot{
						{Input: 0, Output: 0},
						{Input: 1, Output: 0.5},
					},
				},
			},
		},
	}
}

// TestDebugFrameStageOrder verifies that generateRockyHeightmapDebug
// appends at least one stage and that the first stage is named
// "Continentalness" when only the Continentalness field is configured.
func TestDebugFrameStageOrder(t *testing.T) {
	prof := minimalRockyProfile()
	frame := &DebugFrame{}
	_, _ = generateRockyHeightmapDebug(&prof, 42, 32, frame, nil)
	if len(frame.Stages) == 0 {
		t.Fatal("expected at least one stage")
	}
	if frame.Stages[0].Name != "Continentalness" {
		t.Errorf("first stage should be Continentalness; got %s", frame.Stages[0].Name)
	}
}

// TestDebugFrameStageKind verifies that all stages appended by the
// heightmap path carry Kind == "height".
func TestDebugFrameStageKind(t *testing.T) {
	prof := minimalRockyProfile()
	frame := &DebugFrame{}
	_, _ = generateRockyHeightmapDebug(&prof, 42, 16, frame, nil)
	for _, s := range frame.Stages {
		if s.Kind != "height" {
			t.Errorf("stage %q has Kind %q; want \"height\"", s.Name, s.Kind)
		}
	}
}

// TestDebugFrameStageHasRawFbm verifies that the Continentalness stage
// populates RawFbm and InputBands/OutputBands.
func TestDebugFrameStageHasRawFbm(t *testing.T) {
	prof := minimalRockyProfile()
	frame := &DebugFrame{}
	_, _ = generateRockyHeightmapDebug(&prof, 42, 16, frame, nil)
	if len(frame.Stages) == 0 {
		t.Fatal("no stages recorded")
	}
	s := frame.Stages[0]
	if s.RawFbm == nil {
		t.Error("Continentalness stage: RawFbm should be non-nil")
	}
	if s.SumAfter == nil {
		t.Error("Continentalness stage: SumAfter should be non-nil")
	}
	if s.InputBands == nil {
		t.Error("Continentalness stage: InputBands should be non-nil")
	}
	if s.OutputBands == nil {
		t.Error("Continentalness stage: OutputBands should be non-nil")
	}
}

// TestDebugFrameBypassSkipFlag verifies that when a stage is bypassed
// its DebugStage.Skipped flag is true.
func TestDebugFrameBypassSkipFlag(t *testing.T) {
	prof := minimalRockyProfile()
	frame := &DebugFrame{}
	bypass := DebugBypass{"Continentalness": true}
	_, _ = generateRockyHeightmapDebug(&prof, 42, 16, frame, bypass)
	if len(frame.Stages) == 0 {
		t.Fatal("no stages recorded")
	}
	s := frame.Stages[0]
	if s.Name != "Continentalness" {
		t.Fatalf("expected first stage to be Continentalness; got %s", s.Name)
	}
	if !s.Skipped {
		t.Error("Continentalness stage should have Skipped=true when bypassed")
	}
}

// TestDebugFrameBypassParity verifies that bypassing the only active
// height stage produces a flat (all-equal) heightmap.
func TestDebugFrameBypassParity(t *testing.T) {
	prof := minimalRockyProfile()
	bypass := DebugBypass{"Continentalness": true}
	hm, _ := generateRockyHeightmapDebug(&prof, 42, 16, nil, bypass)

	// With the only stage bypassed the pre-normalize heightmap is all
	// zeros.  After normalization a zero-range map is left as-is, so all
	// values remain zero.  If the normalizer produces a constant (e.g.
	// 0.5 for zero-range), all values will still be equal — assert that.
	first := hm.Faces[0][0]
	for face := range hm.Faces {
		for _, v := range hm.Faces[face] {
			if v != first {
				t.Errorf("with all stages bypassed heightmap should be flat; got differing value %v (expected %v)", v, first)
				return
			}
		}
	}
}

// TestDebugFrameNilFrameNoPanic ensures that nil frame and nil bypass
// work without panicking (the fast path). This guards the existing
// call sites and the RenderRocky fast path.
func TestDebugFrameNilFrameNoPanic(t *testing.T) {
	prof := minimalRockyProfile()
	hm, _ := generateRockyHeightmapDebug(&prof, 42, 16, nil, nil)
	if hm == nil {
		t.Fatal("expected non-nil heightmap")
	}
}

// TestDebugFrameColorStages verifies that colorizeRockyDebug appends
// at least a Palette stage when frame is non-nil.
func TestDebugFrameColorStages(t *testing.T) {
	prof := minimalRockyProfile()
	// Add a minimal palette so the Palette stage fires.
	prof.Palette = []planetcolor.ColorStop{
		{Position: 0, Color: color.RGBA{R: 50, G: 50, B: 50, A: 255}},
		{Position: 1, Color: color.RGBA{R: 200, G: 200, B: 200, A: 255}},
	}
	hm, craters := generateRockyHeightmapDebug(&prof, 42, 16, nil, nil)
	frame := &DebugFrame{}
	out := colorizeRockyDebug(&prof, 42, 16, hm, craters, frame, nil)
	if out == nil {
		t.Fatal("expected non-nil color output")
	}
	if len(frame.Stages) == 0 {
		t.Fatal("expected at least one color stage")
	}
	if frame.Stages[0].Name != "Palette" {
		t.Errorf("first color stage should be Palette; got %s", frame.Stages[0].Name)
	}
	for _, s := range frame.Stages {
		if s.Kind != "color" {
			t.Errorf("color stage %q has Kind %q; want \"color\"", s.Name, s.Kind)
		}
		if s.ColorAfter == nil {
			t.Errorf("color stage %q: ColorAfter should be non-nil", s.Name)
		}
	}
}
