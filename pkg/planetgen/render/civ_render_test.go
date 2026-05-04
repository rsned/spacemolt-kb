package render_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// civEnabledTerran returns a clone of the terran archetype with the
// Civ knobs flipped on. The production terran profile currently has
// Civ.Tier == 0 (per-archetype defaults land in P9b Task 6); this
// helper short-cuts that for the render-side wiring tests.
func civEnabledTerran() *types.PlanetProfile {
	prof := *planetgen.Profiles["terran"]
	prof.Civ = types.CivConfig{
		Tier:           0.5,
		SiteMinDistRad: 0.05,
		SiteMaxDistRad: 0.20,
		MaxPopulation:  0.6,
	}
	return &prof
}

// TestRenderNightCubeMapTerranNonNil verifies that an enabled-civ
// terran profile produces a non-nil Black-Marble nightside, and a
// gas-giant profile (default Civ.Tier=0) returns nil.
func TestRenderNightCubeMapTerranNonNil(t *testing.T) {
	cm := render.RenderNightCubeMap(civEnabledTerran(), 42, 48)
	if cm == nil {
		t.Fatal("RenderNightCubeMap returned nil for enabled-civ terran")
	}
	// Sanity: at least some pixels must have been splatted to.
	var nonBlack int
	for face := range cm.Faces {
		for _, p := range cm.Faces[face] {
			if p.R > 0 || p.G > 0 || p.B > 0 {
				nonBlack++
			}
		}
	}
	if nonBlack == 0 {
		t.Fatal("RenderNightCubeMap returned a fully-black cube map")
	}

	if got := render.RenderNightCubeMap(planetgen.Profiles["jovian"], 42, 48); got != nil {
		t.Errorf("RenderNightCubeMap returned non-nil for jovian (Civ.Tier=0)")
	}
}

// TestRenderNightCubeMapDisabledForCivlessArchetype verifies that
// archetypes whose production default leaves Civ.Tier=0 yield nil
// from RenderNightCubeMap. Tundra is one such archetype as of P9b
// Task 6 — only terran and super_terran are civ-enabled by default.
func TestRenderNightCubeMapDisabledForCivlessArchetype(t *testing.T) {
	if got := render.RenderNightCubeMap(planetgen.Profiles["tundra"], 42, 48); got != nil {
		t.Errorf("RenderNightCubeMap returned non-nil for tundra (Civ.Tier=0)")
	}
}

// TestRenderRockyAddsCivStage verifies that an enabled-civ profile
// produces a Civ stage in the debug frame, sandwiched between Ejecta
// (when present) and LUT (when present). The production-default
// archetypes do NOT exercise this path (Civ.Tier=0) so existing
// stage-count tests remain valid.
func TestRenderRockyAddsCivStage(t *testing.T) {
	frame := render.RenderRockyDebug(civEnabledTerran(), 42, 48, nil)
	if frame == nil {
		t.Fatal("RenderRockyDebug returned nil")
	}
	civIdx := -1
	lutIdx := -1
	ejectaIdx := -1
	for i, st := range frame.Stages {
		switch st.Name {
		case "Civ":
			civIdx = i
		case "LUT":
			lutIdx = i
		case "Ejecta":
			ejectaIdx = i
		}
	}
	if civIdx < 0 {
		t.Fatal("expected a Civ stage in RenderRockyDebug frame; not found")
	}
	if ejectaIdx >= 0 && civIdx <= ejectaIdx {
		t.Errorf("Civ stage at %d must come after Ejecta at %d", civIdx, ejectaIdx)
	}
	if lutIdx >= 0 && civIdx >= lutIdx {
		t.Errorf("Civ stage at %d must come before LUT at %d", civIdx, lutIdx)
	}
}

// TestRenderRockyDebugDoesNotAddCivStageWhenDisabled verifies the
// gate: an archetype with Civ.Tier=0 must NOT emit a Civ stage.
// Uses tundra because terran/super_terran are civ-enabled as of
// P9b Task 6.
func TestRenderRockyDebugDoesNotAddCivStageWhenDisabled(t *testing.T) {
	frame := render.RenderRockyDebug(planetgen.Profiles["tundra"], 42, 48, nil)
	for _, st := range frame.Stages {
		if st.Name == "Civ" {
			t.Fatalf("default tundra (Civ.Tier=0) emitted a Civ stage; should be skipped")
		}
	}
}
