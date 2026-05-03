package render_test

import (
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

// TestRockyRenderEnablesFlowOnTerran verifies the rocky pipeline
// produces visibly different output when Flow river carving is enabled
// vs. disabled. River carving lowers heightmap values along the river
// mask which propagates through Normalize and the colorizer, so the
// final RGBA output must differ on at least some pixels.
func TestRockyRenderEnablesFlowOnTerran(t *testing.T) {
	profileEnabled := *planetgen.Profiles["terran"]
	profileDisabled := profileEnabled
	profileDisabled.Flow.RiverThreshold = 0
	profileDisabled.Flow.RiverDepth = 0

	a := render.RenderRocky(&profileEnabled, 42, 64)
	b := render.RenderRocky(&profileDisabled, 42, 64)

	var diff int
	for f := range a.Faces {
		for i := range a.Faces[f] {
			if a.Faces[f][i] != b.Faces[f][i] {
				diff++
			}
		}
	}
	if diff == 0 {
		t.Fatalf("flow=enabled produced byte-identical output to flow=disabled (expected river carving to change colors)")
	}
	t.Logf("flow on/off: %d differing pixels", diff)
}

// TestRockyRenderEnablesRainShadowOnTerran verifies that toggling
// RainShadow.WalkSteps changes the rocky output. The multiplier feeds
// the M (humidity) axis of the Whittaker biome lookup, so disabling
// it is expected to shift colors in latitudes where the wind tangent
// differs from the windward/lee bands the multiplier creates.
func TestRockyRenderEnablesRainShadowOnTerran(t *testing.T) {
	profileEnabled := *planetgen.Profiles["terran"]
	profileDisabled := profileEnabled
	profileDisabled.RainShadow.WalkSteps = 0
	// Also disable Flow on both so we isolate the rainshadow effect.
	profileEnabled.Flow.RiverThreshold = 0
	profileDisabled.Flow.RiverThreshold = 0

	a := render.RenderRocky(&profileEnabled, 42, 64)
	b := render.RenderRocky(&profileDisabled, 42, 64)

	var diff int
	for f := range a.Faces {
		for i := range a.Faces[f] {
			if a.Faces[f][i] != b.Faces[f][i] {
				diff++
			}
		}
	}
	if diff == 0 {
		t.Fatalf("rainShadow=enabled produced byte-identical output to rainShadow=disabled")
	}
	t.Logf("rainshadow on/off: %d differing pixels", diff)
}

// TestRockyRenderRoundtripDeterministic verifies that two RenderRocky
// calls with the same profile + seed produce byte-equal output. Catches
// accidental nondeterminism (e.g. iterating maps, unstable sorts)
// introduced by the new flow / rain-shadow stages.
func TestRockyRenderRoundtripDeterministic(t *testing.T) {
	profile := *planetgen.Profiles["terran"]
	a := render.RenderRocky(&profile, 42, 64)
	b := render.RenderRocky(&profile, 42, 64)
	for f := range a.Faces {
		for i := range a.Faces[f] {
			if a.Faces[f][i] != b.Faces[f][i] {
				t.Fatalf("RenderRocky non-deterministic: face %d pixel %d differs (a=%v b=%v)", f, i, a.Faces[f][i], b.Faces[f][i])
			}
		}
	}
}

// TestRenderRockyDebugRegistersFlowAndRainShadow verifies that on a
// terran profile (with Flow.RiverThreshold > 0 and RainShadow.WalkSteps
// > 0), RenderRockyDebug appends the three new debug stages and stores
// the FlowField + RainShadowField in the frame.
func TestRenderRockyDebugRegistersFlowAndRainShadow(t *testing.T) {
	profile := *planetgen.Profiles["terran"]
	frame := render.RenderRockyDebug(&profile, 42, 64, nil)

	var foundRivers, foundAccum, foundRainShadow bool
	for _, st := range frame.Stages {
		switch st.Name {
		case "Flow: rivers":
			foundRivers = true
			if st.BooleanAfter == nil {
				t.Errorf("Flow: rivers stage missing BooleanAfter")
			}
		case "Flow: accum":
			foundAccum = true
			if st.ScalarAfter == nil {
				t.Errorf("Flow: accum stage missing ScalarAfter")
			}
		case "RainShadow":
			foundRainShadow = true
			if st.ScalarAfter == nil {
				t.Errorf("RainShadow stage missing ScalarAfter")
			}
		}
	}
	if !foundRivers {
		t.Errorf("missing 'Flow: rivers' stage")
	}
	if !foundAccum {
		t.Errorf("missing 'Flow: accum' stage")
	}
	if !foundRainShadow {
		t.Errorf("missing 'RainShadow' stage")
	}
	if frame.Flow == nil {
		t.Errorf("frame.Flow must be populated when Flow.RiverThreshold > 0")
	}
	if frame.RainShadow == nil {
		t.Errorf("frame.RainShadow must be populated when RainShadow.WalkSteps > 0")
	}
}

// TestRenderRockyDebugNoFlowOrRainShadowOnGasGiant verifies that
// archetypes with Flow disabled (RiverThreshold == 0) and RainShadow
// disabled (WalkSteps == 0) do NOT register any of the three new
// debug stages, and frame.Flow / frame.RainShadow remain nil.
func TestRenderRockyDebugNoFlowOrRainShadowOnGasGiant(t *testing.T) {
	for _, name := range []string{"jovian", "ice_giant"} {
		t.Run(name, func(t *testing.T) {
			profile, ok := planetgen.Profiles[name]
			if !ok {
				t.Skipf("profile %q not registered", name)
			}
			// Sanity-check the profile config.
			if profile.Flow.RiverThreshold != 0 {
				t.Skipf("profile %q unexpectedly has Flow.RiverThreshold=%v; expected 0",
					name, profile.Flow.RiverThreshold)
			}
			if profile.RainShadow.WalkSteps != 0 {
				t.Skipf("profile %q unexpectedly has RainShadow.WalkSteps=%v; expected 0",
					name, profile.RainShadow.WalkSteps)
			}
			frame := render.RenderRockyDebug(profile, 42, 32, nil)
			for _, st := range frame.Stages {
				if strings.HasPrefix(st.Name, "Flow:") || st.Name == "RainShadow" {
					t.Errorf("unexpected stage %q for profile %q", st.Name, name)
				}
			}
			if frame.Flow != nil {
				t.Errorf("frame.Flow should be nil for profile %q", name)
			}
			if frame.RainShadow != nil {
				t.Errorf("frame.RainShadow should be nil for profile %q", name)
			}
		})
	}
}
