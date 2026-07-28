package shipglyph

import (
	"math"
	"strings"
	"testing"
)

func TestHardpointsCountMatchesSlots(t *testing.T) {
	s := Stats{ID: "magnate", Class: "Command", Faction: "solarian", Scale: 4,
		Weapon: 3, Defense: 6, Utility: 5}
	hps := Hardpoints(Infer(s), s)

	if len(hps) != 14 {
		t.Fatalf("len = %d, want 14 (3+6+5)", len(hps))
	}
	var w, d, u int
	for _, h := range hps {
		switch h.Kind {
		case "weapon":
			w++
		case "defense":
			d++
		case "utility":
			u++
		default:
			t.Errorf("unexpected kind %q", h.Kind)
		}
	}
	if w != 3 || d != 6 || u != 5 {
		t.Errorf("counts = %d/%d/%d, want 3/6/5", w, d, u)
	}
}

func TestHardpointIDsAreStableAndUnique(t *testing.T) {
	s := Stats{ID: "crowbar", Class: "Salvager", Faction: "crimson", Scale: 1,
		Weapon: 2, Defense: 1, Utility: 1}
	hps := Hardpoints(Infer(s), s)

	seen := map[string]bool{}
	for _, h := range hps {
		if seen[h.ID] {
			t.Errorf("duplicate hardpoint ID %q", h.ID)
		}
		seen[h.ID] = true
		if !strings.HasPrefix(h.ID, "hp-") {
			t.Errorf("ID %q does not use the hp- prefix", h.ID)
		}
	}
	if !seen["hp-w1"] || !seen["hp-w2"] {
		t.Errorf("weapon IDs not numbered from 1: %v", seen)
	}
}

func TestHardpointsSitInsideTheHull(t *testing.T) {
	s := Stats{ID: "war_wagon", Class: "Bulk Hauler", Faction: "crimson", Scale: 4,
		Weapon: 2, Defense: 2, Utility: 8}
	d := Infer(s)
	for _, h := range Hardpoints(d, s) {
		w := hullHalfWidth(d, h.Pos.X)
		if math.Abs(h.Pos.Y) > w+1e-9 {
			t.Errorf("%s at Y=%v exceeds half-width %v at X=%v", h.ID, h.Pos.Y, w, h.Pos.X)
		}
	}
}

func TestHardpointsZeroSlotsProducesNone(t *testing.T) {
	s := Stats{ID: "prayer", Class: "Freighter", Faction: "outerrim", Scale: 1}
	if hps := Hardpoints(Infer(s), s); len(hps) != 0 {
		t.Errorf("len = %d, want 0 for a ship with no slots", len(hps))
	}
}

func TestHardpointsAreDeterministic(t *testing.T) {
	s := Stats{ID: "superposition", Class: "Drone Carrier", Faction: "voidborn", Scale: 4,
		Weapon: 2, Defense: 5, Utility: 6}
	d := Infer(s)
	a, b := Hardpoints(d, s), Hardpoints(d, s)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("hardpoint %d diverged: %+v vs %+v", i, a[i], b[i])
		}
	}
}
