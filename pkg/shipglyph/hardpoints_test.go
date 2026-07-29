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

func TestHardpointPairMirrorsAtOneStation(t *testing.T) {
	// Two weapon slots on a spine hull: the weapon zone is {0.08, 0.32}, and
	// two markers make a single station, so both land on the zone midpoint —
	// one starboard, one port — each inset to 55% of the local half-width.
	// This pins both the station arithmetic and hardpointInset. Asserting only
	// |Y| <= half-width would hold for any inset in [0,1] and so could never
	// fail.
	s := Stats{ID: "pin", Class: "Cruiser", Faction: "crimson", Scale: 3, Weapon: 2}
	d := Infer(s)
	hps := Hardpoints(d, s)

	if len(hps) != 2 {
		t.Fatalf("len = %d, want 2", len(hps))
	}
	const wantX = 0.20 // midpoint of {0.08, 0.32}
	if hullHalfWidth(d, wantX) <= 0 {
		t.Fatalf("test would be vacuous: half-width at the station is 0")
	}

	wantSign := []float64{1, -1}
	for i, h := range hps {
		if math.Abs(h.Pos.X-wantX) > 1e-9 {
			t.Errorf("%s X = %v, want %v", h.ID, h.Pos.X, wantX)
		}
		wantY := wantSign[i] * hullHalfWidth(d, wantX) * hardpointInset
		if math.Abs(h.Pos.Y-wantY) > 1e-9 {
			t.Errorf("%s Y = %v, want %v", h.ID, h.Pos.Y, wantY)
		}
	}
}

func TestHardpointStationsSpreadAcrossTheZone(t *testing.T) {
	// Five weapon slots make three stations across the zone {0.08, 0.32}:
	// the first two are full mirrored pairs at the zone ends' inner spread,
	// and the odd fifth marker takes the last station on the centerline.
	s := Stats{ID: "spread", Class: "Cruiser", Faction: "crimson", Scale: 3, Weapon: 5}
	d := Infer(s)
	hps := Hardpoints(d, s)

	if len(hps) != 5 {
		t.Fatalf("len = %d, want 5", len(hps))
	}
	wantX := []float64{0.08, 0.08, 0.20, 0.20, 0.32}
	for i, h := range hps {
		if math.Abs(h.Pos.X-wantX[i]) > 1e-9 {
			t.Errorf("%s X = %v, want %v", h.ID, h.Pos.X, wantX[i])
		}
	}
	// Each pair mirrors exactly; the leftover sits on the centerline.
	for _, pair := range [][2]int{{0, 1}, {2, 3}} {
		a, b := hps[pair[0]], hps[pair[1]]
		if a.Pos.Y <= 0 || b.Pos.Y >= 0 || math.Abs(a.Pos.Y+b.Pos.Y) > 1e-9 {
			t.Errorf("%s/%s Y = %v/%v, want equal and opposite", a.ID, b.ID, a.Pos.Y, b.Pos.Y)
		}
	}
	if hps[4].Pos.Y != 0 {
		t.Errorf("%s Y = %v, want exactly 0 for the unpaired marker", hps[4].ID, hps[4].Pos.Y)
	}
}

func TestHardpointKindsDoNotShareHull(t *testing.T) {
	// Mount zones for the three kinds must be disjoint: markers carry no
	// per-kind position meaning, so an overlap renders as scatter. Check the
	// spans actually used, across every shape family.
	for _, fam := range []string{"needle", "dart", "spine", "slab", "drum", "rig", "rack", "pod"} {
		z := archetypeZones(fam)
		bands := [][2]float64{z.Weapon[0], z.Defense[0], z.Utility[0]}
		names := []string{"weapon", "defense", "utility"}
		for i := 1; i < len(bands); i++ {
			if bands[i][0] <= bands[i-1][1] {
				t.Errorf("%s: %s zone starts at %v, inside the %s zone ending at %v",
					fam, names[i], bands[i][0], names[i-1], bands[i-1][1])
			}
		}
		if bands[2][1] > 0.9 {
			t.Errorf("%s: utility zone ends at %v, too far into the stern taper", fam, bands[2][1])
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
	if math.Abs(hps[0].Pos.X-0.20) > 1e-9 {
		t.Errorf("X = %v, want 0.20 (midpoint of zone {0.08, 0.32})", hps[0].Pos.X)
	}
	if hps[0].Pos.Y != 0 {
		t.Errorf("Y = %v, want exactly 0", hps[0].Pos.Y)
	}
}
