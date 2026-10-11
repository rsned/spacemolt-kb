// pkg/tectonics/params_test.go
package tectonics

import "testing"

func TestDefaultParamsPerArchetype(t *testing.T) {
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	if p.Face != 256 || p.Steps != 400 || p.KeyframeEvery != 4 || p.MajorMin != 6 || p.MajorMax != 9 || p.TimelineMyr != 800 {
		t.Errorf("terran defaults %+v", p)
	}
	if p.SpeedMinCmYr != 3 || p.SpeedMaxCmYr != 10 || p.DominantMin != 0.20 || p.DominantMax != 0.35 || p.RepoleEveryMyr != 25 || p.ReaimFresh != 0.75 || p.SpeedSkew != 2 {
		t.Errorf("terran tuning defaults %+v", p)
	}
	if p.RiftMinShareMin != 0.04 || p.RiftMinShareMax != 0.08 || p.RiftRestMyrMin != 50 || p.RiftRestMyrMax != 120 ||
		p.RiftChancePerMyr != 0.03 || p.RiftMinChildShare != 0.015 || p.RiftThinPower != 1 || p.RiftMaxPlates != 200 ||
		p.RiftRetries != 3 {
		t.Errorf("rift defaults %+v", p)
	}
	if got := p.MyrPerStep(); got != 2 {
		t.Errorf("MyrPerStep %g, want 2", got)
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
	if err := p.Set("Archetype", "x"); err == nil || p.Archetype == "x" {
		t.Errorf("string knob must error and stay unchanged: %v %q", err, p.Archetype)
	}
}

func TestLandmassDefaultsAndValidation(t *testing.T) {
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	if p.LandShareT0 != 0.30 || p.WaterFactorMin != 0.9 || p.WaterFactorMax != 1.1 {
		t.Fatalf("terran land/water defaults: %g %g %g", p.LandShareT0, p.WaterFactorMin, p.WaterFactorMax)
	}
	if p.ElevPeakKm != 8 || p.RidgeDepthKm != -2.5 || p.ErosionRate != 0.0015 {
		t.Fatalf("elevation/erosion defaults: %g %g %g", p.ElevPeakKm, p.RidgeDepthKm, p.ErosionRate)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	o, _ := DefaultParams("oceanic")
	if o.LandShareT0 != 0.15 || o.WaterFactorMin != 1.3 {
		t.Fatalf("oceanic columns: %g %g", o.LandShareT0, o.WaterFactorMin)
	}
	bad := []struct {
		name string
		mut  func(*Params)
	}{
		{"LandShareT0 0", func(q *Params) { q.LandShareT0 = 0 }},
		{"bands cross threshold", func(q *Params) { q.OceanBandMax = 0.6 }},
		{"LandBandMin below threshold", func(q *Params) { q.LandBandMin = 0.4 }},
		{"ElevPow 0", func(q *Params) { q.ElevPow = 0 }},
		{"shelf above peak", func(q *Params) { q.ElevShelfKm = 9 }},
		{"abyss above ridge", func(q *Params) { q.AbyssKm = -1 }},
		{"ElevBlend 0", func(q *Params) { q.ElevBlend = 0 }},
		{"water range reversed", func(q *Params) { q.WaterFactorMax = 0.5 }},
		{"water min 0", func(q *Params) { q.WaterFactorMin = 0 }},
		{"ErosionMaxPerStep 0.3", func(q *Params) { q.ErosionMaxPerStep = 0.3 }},
		{"SedimentOceanFactor 2", func(q *Params) { q.SedimentOceanFactor = 2 }},
		{"negative erosion", func(q *Params) { q.ErosionRate = -1 }},
	}
	for _, b := range bad {
		q := p
		b.mut(&q)
		if err := Validate(q); err == nil {
			t.Errorf("%s: Validate accepted an invalid value", b.name)
		}
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
