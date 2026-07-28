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

func TestHardpointPlacementPinsInsetAndSpread(t *testing.T) {
	// Two weapon slots on a spine hull: the weapon zone is {0.06, 0.40}, so
	// the pair must land exactly on the zone ends, alternating starboard then
	// port, each inset to 55% of the local half-width. This pins both the
	// spread arithmetic and hardpointInset. Asserting only |Y| <= half-width
	// would hold for any inset in [0,1] and so could never fail.
	s := Stats{ID: "pin", Class: "Cruiser", Faction: "crimson", Scale: 3, Weapon: 2}
	d := Infer(s)
	hps := Hardpoints(d, s)

	if len(hps) != 2 {
		t.Fatalf("len = %d, want 2", len(hps))
	}
	if hullHalfWidth(d, 0.06) <= 0 {
		t.Fatalf("test would be vacuous: half-width at the forward zone end is 0")
	}

	wantX := []float64{0.06, 0.40}
	wantSign := []float64{1, -1}
	for i, h := range hps {
		if math.Abs(h.Pos.X-wantX[i]) > 1e-9 {
			t.Errorf("%s X = %v, want %v", h.ID, h.Pos.X, wantX[i])
		}
		wantY := wantSign[i] * hullHalfWidth(d, wantX[i]) * hardpointInset
		if math.Abs(h.Pos.Y-wantY) > 1e-9 {
			t.Errorf("%s Y = %v, want %v", h.ID, h.Pos.Y, wantY)
		}
	}
}

func TestHardpointsZeroSlotsProducesNone(t *testing.T) {
	s := Stats{ID: "prayer", Class: "Freighter", Faction: "outerrim", Scale: 1}
	if hps := Hardpoints(Infer(s), s); len(hps) != 0 {
		t.Errorf("len = %d, want 0 for a ship with no slots", len(hps))
	}
}

func TestSingleHardpointSitsOnTheCenterline(t *testing.T) {
	// With one marker there is no pair to mirror, so it takes the zone
	// midpoint on the centerline rather than an offset side.
	s := Stats{ID: "solo", Class: "Cruiser", Faction: "crimson", Scale: 3, Weapon: 1}
	hps := Hardpoints(Infer(s), s)

	if len(hps) != 1 {
		t.Fatalf("len = %d, want 1", len(hps))
	}
	if math.Abs(hps[0].Pos.X-0.23) > 1e-9 {
		t.Errorf("X = %v, want 0.23 (midpoint of zone {0.06, 0.40})", hps[0].Pos.X)
	}
	if hps[0].Pos.Y != 0 {
		t.Errorf("Y = %v, want exactly 0", hps[0].Pos.Y)
	}
}
