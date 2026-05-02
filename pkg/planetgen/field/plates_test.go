package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestSeedPlatesCountAndUnit(t *testing.T) {
	profile := &types.PlanetProfile{
		PlateCount:           12,
		OceanicPlateFraction: 0.7,
		PlateConvergentT:     0.75,
	}
	plates := seedPlates(profile, 42)
	if len(plates) != 12 {
		t.Fatalf("got %d plates, want 12", len(plates))
	}
	for i, p := range plates {
		if p.ID != i {
			t.Errorf("plate %d: ID=%d, want %d", i, p.ID, i)
		}
		// Seed and RotAxis must be unit vectors.
		for label, v := range map[string][3]float64{"Seed": p.Seed, "RotAxis": p.RotAxis} {
			mag := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
			if math.Abs(mag-1) > 1e-9 {
				t.Errorf("plate %d %s not unit: |v|=%f", i, label, mag)
			}
		}
		if p.AngSpeed < 0 || p.AngSpeed > 1 {
			t.Errorf("plate %d AngSpeed=%f out of [0,1]", i, p.AngSpeed)
		}
	}
}

func TestSeedPlatesDeterministic(t *testing.T) {
	profile := &types.PlanetProfile{PlateCount: 8, OceanicPlateFraction: 0.5}
	a := seedPlates(profile, 99)
	b := seedPlates(profile, 99)
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("plate %d differs across calls: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestSeedPlatesOceanicFractionInCI(t *testing.T) {
	// With PlateCount=200 and OceanicPlateFraction=0.7, expect ~140 oceanic.
	// Wide window [0.60, 0.80] chosen for CI flake-resistance; the Wilson
	// 95% CI for n=200, p=0.7 is ~[0.635, 0.757].
	profile := &types.PlanetProfile{PlateCount: 200, OceanicPlateFraction: 0.7}
	plates := seedPlates(profile, 7)
	var oceanic int
	for _, p := range plates {
		if p.IsOceanic {
			oceanic++
		}
	}
	frac := float64(oceanic) / float64(len(plates))
	if frac < 0.60 || frac > 0.80 {
		t.Errorf("oceanic fraction %.3f outside expected window [0.60, 0.80]", frac)
	}
}

func TestSeedPlatesZeroCount(t *testing.T) {
	profile := &types.PlanetProfile{PlateCount: 0}
	plates := seedPlates(profile, 1)
	if len(plates) != 0 {
		t.Errorf("PlateCount=0 should yield empty slice, got %d", len(plates))
	}
}

func TestFloodFillSinglePlateCoversAll(t *testing.T) {
	S := 16
	profile := &types.PlanetProfile{PlateCount: 1, OceanicPlateFraction: 0.5}
	pf := GeneratePlates(profile, 1, S)
	if pf == nil {
		t.Fatal("GeneratePlates returned nil")
	}
	for f := range pf.PlateID {
		for i, id := range pf.PlateID[f] {
			if id != 0 {
				t.Fatalf("face %d idx %d: id=%d (want 0)", f, i, id)
			}
		}
	}
}

func TestFloodFillCoverageAndDeterminism(t *testing.T) {
	S := 32
	profile := &types.PlanetProfile{PlateCount: 6, OceanicPlateFraction: 0.5}
	a := GeneratePlates(profile, 11, S)
	b := GeneratePlates(profile, 11, S)
	for f := range a.PlateID {
		for i := range a.PlateID[f] {
			if a.PlateID[f][i] != b.PlateID[f][i] {
				t.Fatalf("non-deterministic at face %d idx %d", f, i)
			}
		}
	}
	counts := make(map[int16]int)
	for f := range a.PlateID {
		for _, id := range a.PlateID[f] {
			if id < 0 {
				t.Fatalf("unfilled pixel: face %d id=%d", f, id)
			}
			counts[id]++
		}
	}
	if len(counts) != 6 {
		t.Errorf("expected 6 distinct ids, got %d (%+v)", len(counts), counts)
	}
	for id, c := range counts {
		if c == 0 {
			t.Errorf("plate %d has 0 pixels", id)
		}
	}
}

func TestFloodFillZeroPlatesNilField(t *testing.T) {
	profile := &types.PlanetProfile{PlateCount: 0}
	pf := GeneratePlates(profile, 0, 16)
	if pf != nil {
		t.Errorf("expected nil PlateField when PlateCount=0, got %+v", pf)
	}
}

func TestGeneratePlatesPanicsOnInt16Overflow(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for PlateCount > int16 max, got none")
		}
	}()
	profile := &types.PlanetProfile{PlateCount: 32768, OceanicPlateFraction: 0.5}
	GeneratePlates(profile, 1, 8)
}
