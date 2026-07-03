package feature

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestPowerLawSFD(t *testing.T) {
	prof := types.PlanetProfile{
		CraterCount:     1000,
		CraterMinRadius: 0.005, CraterMaxRadius: 0.05,
		PowerLawAlpha: 2.0,
	}
	craters := GenerateCraters(42, &prof)
	if len(craters) != 1000 {
		t.Fatalf("expected 1000 craters, got %d", len(craters))
	}
	smallCount, largeCount := 0, 0
	for _, c := range craters {
		if c.Radius < 0.015 {
			smallCount++
		}
		if c.Radius > 0.04 {
			largeCount++
		}
	}
	if smallCount < 5*largeCount {
		t.Errorf("expected smalls ≫ larges with α=2; small=%d large=%d", smallCount, largeCount)
	}
}

func TestSecondariesNearParent(t *testing.T) {
	prof := types.PlanetProfile{
		CraterCount:     200,
		CraterMinRadius: 0.005, CraterMaxRadius: 0.05,
		PowerLawAlpha:    2.0,
		SecondaryDensity: 0.5,
	}
	craters := GenerateCraters(7, &prof)
	hasSecondary := false
	for _, c := range craters {
		if c.IsSecondary {
			hasSecondary = true
		}
	}
	if !hasSecondary {
		t.Errorf("expected some secondaries with SecondaryDensity=0.5")
	}
}

func TestMariaMaskReducesDensity(t *testing.T) {
	base := types.PlanetProfile{
		CraterCount:     1000,
		CraterMinRadius: 0.005, CraterMaxRadius: 0.05,
		PowerLawAlpha: 2.0,
	}
	masked := base
	masked.MariaDensityFactor = 0.8

	a := GenerateCraters(11, &base)
	b := GenerateCraters(11, &masked)
	if len(a) != base.CraterCount {
		t.Fatalf("baseline: expected %d craters, got %d", base.CraterCount, len(a))
	}
	if len(b) >= len(a) {
		t.Errorf("maria mask should reduce count: baseline=%d masked=%d", len(a), len(b))
	}
}

func TestLegacyPathBackwardCompatible(t *testing.T) {
	// PowerLawAlpha == 0: should produce the same crater list as the legacy
	// quadratic-bias generator with a freshly-seeded PCG. Important so
	// untouched archetypes' goldens don't shift.
	prof := types.PlanetProfile{
		CraterCount:     50,
		CraterMinRadius: 0.005, CraterMaxRadius: 0.05,
	}
	got := GenerateCraters(13, &prof)
	if len(got) != 50 {
		t.Fatalf("expected 50 craters, got %d", len(got))
	}
	for i, c := range got {
		if c.Age != 1.0 {
			t.Errorf("legacy crater[%d] Age=%f, want 1.0", i, c.Age)
		}
		if c.IsSecondary {
			t.Errorf("legacy crater[%d] IsSecondary=true, want false", i)
		}
	}
}
