package shipglyph

import "testing"

func TestRegionsCoverAllFiveNames(t *testing.T) {
	d := Infer(Stats{ID: "war_wagon", Class: "Bulk Hauler", Faction: "crimson", Scale: 4})
	got := Regions(d, StyleFor("crimson"), SeedOf("war_wagon"))

	if len(RegionNames) != 5 {
		t.Fatalf("RegionNames has %d entries, want 5", len(RegionNames))
	}
	for _, name := range RegionNames {
		poly, ok := got[name]
		if !ok {
			t.Errorf("missing region %q", name)
			continue
		}
		if len(poly) < 3 {
			t.Errorf("region %q has %d points, want at least 3", name, len(poly))
		}
	}
}

func TestRegionsBowIsForwardAndSternIsAft(t *testing.T) {
	d := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	got := Regions(d, StyleFor("nebula"), SeedOf("comet"))

	for _, p := range got["bow"] {
		if p.X > 0.26 {
			t.Errorf("bow point at X=%v, want <= 0.25", p.X)
		}
	}
	for _, p := range got["stern"] {
		if p.X < 0.74 {
			t.Errorf("stern point at X=%v, want >= 0.75", p.X)
		}
	}
}

func TestRegionsPortAndStarboardAreOnOpposingSides(t *testing.T) {
	d := Infer(Stats{ID: "crowbar", Class: "Salvager", Faction: "crimson", Scale: 1})
	got := Regions(d, StyleFor("crimson"), SeedOf("crowbar"))

	for _, p := range got["star"] {
		if p.Y < -1e-9 {
			t.Errorf("starboard point has negative Y %v", p.Y)
		}
	}
	for _, p := range got["port"] {
		if p.Y > 1e-9 {
			t.Errorf("port point has positive Y %v", p.Y)
		}
	}
}

func TestRegionsAreDeterministic(t *testing.T) {
	d := Infer(Stats{ID: "yard_sale", Class: "Salvager", Faction: "outerrim", Scale: 3})
	st := StyleFor("outerrim")
	a := Regions(d, st, SeedOf("yard_sale"))
	b := Regions(d, st, SeedOf("yard_sale"))
	for _, name := range RegionNames {
		if len(a[name]) != len(b[name]) {
			t.Fatalf("region %q length diverged", name)
		}
		for i := range a[name] {
			if a[name][i] != b[name][i] {
				t.Fatalf("region %q point %d diverged", name, i)
			}
		}
	}
}
