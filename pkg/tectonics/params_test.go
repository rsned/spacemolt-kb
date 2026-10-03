// pkg/tectonics/params_test.go
package tectonics

import "testing"

func TestDefaultParamsPerArchetype(t *testing.T) {
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	if p.Face != 256 || p.Steps != 150 || p.MajorMin != 6 || p.MajorMax != 9 || p.TimelineMyr != 800 {
		t.Errorf("terran defaults %+v", p)
	}
	if got := p.MyrPerStep(); got < 5.3 || got > 5.4 {
		t.Errorf("MyrPerStep %g", got)
	}
	if _, err := DefaultParams("jovian"); err == nil {
		t.Error("jovian must be refused")
	}
	if _, err := DefaultParams("nope"); err == nil {
		t.Error("unknown archetype must error")
	}
}

func TestParamsSet(t *testing.T) {
	p, _ := DefaultParams("arid")
	if err := p.Set("Steps", "12"); err != nil || p.Steps != 12 {
		t.Errorf("Set Steps: %v %+v", err, p)
	}
	if err := p.Set("RidgeThickness", "0.2"); err != nil || p.RidgeThickness != 0.2 {
		t.Errorf("Set RidgeThickness: %v", err)
	}
	if err := p.Set("Nope", "1"); err == nil {
		t.Error("unknown knob must error")
	}
	if err := p.Set("Steps", "x"); err == nil {
		t.Error("bad value must error")
	}
}

func TestSeedForPlanetStable(t *testing.T) {
	// The repeated call is intentional: it checks that SeedForPlanet is
	// deterministic (same id -> same seed), not a copy-paste mistake.
	if SeedForPlanet("sol_earth") != SeedForPlanet("sol_earth") || SeedForPlanet("a") == SeedForPlanet("b") { //nolint:staticcheck // SA4000: intentional repeat call checks determinism
		t.Error("seed must be stable and distinct")
	}
	a, b := newRNG(7, "x"), newRNG(7, "x")
	if a.Uint64() != b.Uint64() {
		t.Error("newRNG not deterministic")
	}
}
