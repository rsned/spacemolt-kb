package shipglyph

import "testing"

func TestArchetypeOfKnownAndUnknownClasses(t *testing.T) {
	cases := map[string]string{
		"Liner":        "needle",
		"Courier":      "needle",
		"Dreadnought":  "spine",
		"Freighter":    "slab",
		"Bulk Hauler":  "slab",
		"Tanker":       "drum",
		"Gas Harvester": "drum",
		"Miner":        "rig",
		"Salvager":     "rig",
		"Fleet Carrier": "rack",
		"Drone Carrier": "rack",
		"Fighter":      "dart",
		"Research":     "pod",
		"Nonsense":     "spine",
	}
	for class, want := range cases {
		if got := archetypeOf(class); got != want {
			t.Errorf("archetypeOf(%q) = %q, want %q", class, got, want)
		}
	}
}

func TestInferAlwaysProducesUsableGeometry(t *testing.T) {
	s := Stats{ID: "prayer", Class: "Freighter", Faction: "outerrim", Scale: 1, Utility: 0}
	d := Infer(s)

	if d.ID != "prayer" {
		t.Errorf("ID = %q, want prayer", d.ID)
	}
	if d.Aspect <= 0 {
		t.Errorf("Aspect = %v, want positive", d.Aspect)
	}
	if len(d.Hull) == 0 {
		t.Fatalf("Hull is empty; every ship must get geometry")
	}
	for i, p := range d.Hull {
		if p.Span[1] <= p.Span[0] {
			t.Errorf("Hull[%d].Span = %v, want end > start", i, p.Span)
		}
	}
}

func TestInferNeedleIsNarrowerThanSlab(t *testing.T) {
	needle := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	slab := Infer(Stats{ID: "ledger", Class: "Freighter", Faction: "nebula", Scale: 4})

	if needle.Aspect <= slab.Aspect {
		t.Errorf("needle Aspect %v should exceed slab Aspect %v", needle.Aspect, slab.Aspect)
	}
}

func TestInferMountZonesAlwaysPresent(t *testing.T) {
	d := Infer(Stats{ID: "magnate", Class: "Command", Faction: "solarian", Scale: 4,
		Weapon: 3, Defense: 6, Utility: 5})

	if len(d.MountZones.Weapon) == 0 {
		t.Errorf("Weapon zones empty")
	}
	if len(d.MountZones.Defense) == 0 {
		t.Errorf("Defense zones empty")
	}
	if len(d.MountZones.Utility) == 0 {
		t.Errorf("Utility zones empty")
	}
}

func TestInferIsDeterministic(t *testing.T) {
	s := Stats{ID: "crowbar", Class: "Salvager", Faction: "crimson", Scale: 1, Weapon: 2}
	a, b := Infer(s), Infer(s)
	if a.Aspect != b.Aspect || len(a.Hull) != len(b.Hull) {
		t.Errorf("Infer is not deterministic: %+v vs %+v", a, b)
	}
}
