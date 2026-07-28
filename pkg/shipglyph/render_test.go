package shipglyph

import (
	"strings"
	"testing"
)

func renderFixture(t *testing.T, s Stats) string {
	t.Helper()
	d := Infer(s)
	return Render(d, s, Options{Size: 200, ShowHardpoints: true, Title: s.Name})
}

func TestRenderProducesWellFormedSVG(t *testing.T) {
	out := renderFixture(t, Stats{ID: "war_wagon", Name: "War Wagon",
		Class: "Bulk Hauler", Faction: "crimson", Scale: 4, Weapon: 2, Defense: 2, Utility: 8})

	if !strings.HasPrefix(out, "<svg") {
		t.Errorf("output does not start with <svg>:\n%s", out[:min(120, len(out))])
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "</svg>") {
		t.Errorf("output does not end with </svg>")
	}
	if !strings.Contains(out, `viewBox="0 0 200 200"`) {
		t.Errorf("missing or wrong viewBox")
	}
	if !strings.Contains(out, "<title>War Wagon</title>") {
		t.Errorf("missing accessible title")
	}
}

func TestRenderEmitsAllStableRegionIDs(t *testing.T) {
	out := renderFixture(t, Stats{ID: "comet", Name: "Comet",
		Class: "Liner", Faction: "nebula", Scale: 4, Defense: 4, Utility: 5})

	for _, name := range RegionNames {
		want := `id="region-` + name + `"`
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
	if !strings.Contains(out, `id="hull"`) {
		t.Errorf(`missing id="hull" group`)
	}
	if !strings.Contains(out, `id="hardpoints"`) {
		t.Errorf(`missing id="hardpoints" group`)
	}
}

func TestRenderDrawsAppendages(t *testing.T) {
	// A Nebula liner is inferred with swept wings; they must reach the SVG.
	out := renderFixture(t, Stats{ID: "comet", Name: "Comet",
		Class: "Liner", Faction: "nebula", Scale: 4, Defense: 4, Utility: 5})

	if !strings.Contains(out, `id="appendages"`) {
		t.Errorf(`missing id="appendages" group`)
	}
	if !strings.Contains(out, `id="ap-wing-1s"`) || !strings.Contains(out, `id="ap-wing-1p"`) {
		t.Errorf("missing per-side wing IDs")
	}
}

func TestRenderOmitsAppendageGroupWhenThereAreNone(t *testing.T) {
	d := Descriptor{
		Aspect: 3,
		Hull:   []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
	}
	out := Render(d, Stats{ID: "bare", Name: "Bare", Faction: "crimson"}, Options{Size: 200})

	if strings.Contains(out, `id="appendages"`) {
		t.Errorf("emitted an empty appendages group")
	}
}

func TestRenderEmitsHardpointIDs(t *testing.T) {
	out := renderFixture(t, Stats{ID: "magnate", Name: "Magnate",
		Class: "Command", Faction: "solarian", Scale: 4, Weapon: 3, Defense: 6, Utility: 5})

	for _, id := range []string{"hp-w1", "hp-w3", "hp-d6", "hp-u5"} {
		if !strings.Contains(out, `id="`+id+`"`) {
			t.Errorf("missing hardpoint %s", id)
		}
	}
}

func TestRenderUsesCurrentColorNotHardcodedColors(t *testing.T) {
	out := renderFixture(t, Stats{ID: "paradox", Name: "Paradox",
		Class: "Fighter", Faction: "voidborn", Scale: 1, Weapon: 2, Defense: 2, Utility: 1})

	if !strings.Contains(out, "currentColor") {
		t.Errorf("expected currentColor strokes for theme compatibility")
	}
	if strings.Contains(out, "#") || strings.Contains(out, "hsl(") {
		t.Errorf("glyph hardcodes a color; it must inherit from CSS:\n%s", out)
	}
}

func TestRenderIsByteIdenticalAcrossRuns(t *testing.T) {
	s := Stats{ID: "yard_sale", Name: "Yard Sale",
		Class: "Salvager", Faction: "outerrim", Scale: 3, Defense: 1, Utility: 4}
	first := renderFixture(t, s)
	second := renderFixture(t, s)
	if first != second {
		t.Errorf("Render is not deterministic")
	}
}

func TestRenderHandlesZeroSlotShip(t *testing.T) {
	out := renderFixture(t, Stats{ID: "prayer", Name: "Prayer",
		Class: "Freighter", Faction: "outerrim", Scale: 1})

	if !strings.Contains(out, `id="hull"`) {
		t.Errorf("hull missing for a zero-slot ship")
	}
	if strings.Contains(out, `id="hp-`) {
		t.Errorf("zero-slot ship should have no hardpoint markers")
	}
}

func TestRenderEveryFactionProducesOutput(t *testing.T) {
	for _, f := range []string{"crimson", "nebula", "solarian", "outerrim", "voidborn", "pirate", ""} {
		s := Stats{ID: "x_" + f, Name: "X", Class: "Cruiser", Faction: f, Scale: 3, Weapon: 2}
		out := renderFixture(t, s)
		if !strings.Contains(out, "<path") {
			t.Errorf("faction %q produced no paths", f)
		}
	}
}
