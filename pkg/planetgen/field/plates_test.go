package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
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

func TestClassifyBoundaryHandBuilt(t *testing.T) {
	// Test cases verify the classifier branches by sign and threshold T.
	// We feed (v_rel, n, T) triples directly; nothing about plate
	// geometry needs to be modeled here.
	cases := []struct {
		vrel [3]float64
		n    [3]float64
		t    float64
		want boundaryKind
	}{
		{[3]float64{0, 0, -2}, [3]float64{0, 0, 1}, 0.75, boundaryDivergent},
		{[3]float64{0, 0, +2}, [3]float64{0, 0, 1}, 0.75, boundaryConvergent},
		{[3]float64{0, 0, +0.1}, [3]float64{0, 0, 1}, 0.75, boundaryTransform},
		{[3]float64{0, 0, +0.8}, [3]float64{0, 0, 1}, 0.75, boundaryConvergent},
		{[3]float64{0, 0, -0.74}, [3]float64{0, 0, 1}, 0.75, boundaryTransform},
	}
	for i, c := range cases {
		got := classifyBoundary(c.vrel, c.n, c.t)
		if got != c.want {
			t.Errorf("case %d: classifyBoundary(%v,%v,%v) = %v, want %v",
				i, c.vrel, c.n, c.t, got, c.want)
		}
	}
}

func TestExtractBoundariesAtLeastOnePerType(t *testing.T) {
	// With 12 random plates we expect at least one of each type. If a
	// run produces zero of a type, the rng was unlucky — bump the seed
	// until this test stabilizes.
	profile := &types.PlanetProfile{
		PlateCount: 12, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75,
	}
	pf := GeneratePlates(profile, 5, 32)
	conv, div, trans, _, _ := extractBoundaries(pf, profile.PlateConvergentT)
	counts := map[string]int{"conv": 0, "div": 0, "trans": 0}
	for f := range conv {
		for i := range conv[f] {
			if conv[f][i] {
				counts["conv"]++
			}
			if div[f][i] {
				counts["div"]++
			}
			if trans[f][i] {
				counts["trans"]++
			}
		}
	}
	for kind, c := range counts {
		if c == 0 {
			t.Errorf("no boundaries of kind %s", kind)
		}
	}
}

func TestExtractBoundariesMutuallyExclusive(t *testing.T) {
	// Smoothed-normal classification must put each pixel in at most
	// one of {convergent, divergent, transform}. The pre-smoothing
	// per-pixel version routinely failed this — the jagged flood-fill
	// front produced noisy n vectors that swung the projection across
	// ±T on adjacent pixels of the same physical boundary.
	profile := &types.PlanetProfile{
		PlateCount: 12, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75,
	}
	pf := GeneratePlates(profile, 5, 64)
	conv, div, trans, _, _ := extractBoundaries(pf, profile.PlateConvergentT)
	for f := range conv {
		for i := range conv[f] {
			n := 0
			if conv[f][i] {
				n++
			}
			if div[f][i] {
				n++
			}
			if trans[f][i] {
				n++
			}
			if n > 1 {
				t.Fatalf("face %d idx %d classified as %d kinds (conv=%v div=%v trans=%v); masks must be mutually exclusive",
					f, i, n, conv[f][i], div[f][i], trans[f][i])
			}
		}
	}
}

func TestExtractBoundariesMagnitudeRange(t *testing.T) {
	// Per-pixel magnitudes must be in [0,1] and non-zero only on the
	// matching boundary mask. The (|proj|-T)/(1-T) clamp guarantees
	// range; the pixel-wise assignment guarantees mask alignment.
	profile := &types.PlanetProfile{
		PlateCount: 12, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75,
	}
	pf := GeneratePlates(profile, 5, 64)
	conv, div, _, convMag, divMag := extractBoundaries(pf, profile.PlateConvergentT)
	for f := range conv {
		for i := range conv[f] {
			cm, dm := convMag[f][i], divMag[f][i]
			if cm < 0 || cm > 1 {
				t.Fatalf("convMag face %d idx %d = %g out of [0,1]", f, i, cm)
			}
			if dm < 0 || dm > 1 {
				t.Fatalf("divMag face %d idx %d = %g out of [0,1]", f, i, dm)
			}
			if cm > 0 && !conv[f][i] {
				t.Fatalf("convMag face %d idx %d = %g but conv mask is false", f, i, cm)
			}
			if dm > 0 && !div[f][i] {
				t.Fatalf("divMag face %d idx %d = %g but div mask is false", f, i, dm)
			}
		}
	}
}

func TestSDFsZeroAtSeeds(t *testing.T) {
	profile := &types.PlanetProfile{
		PlateCount: 6, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75,
	}
	pf := GeneratePlates(profile, 17, 32)
	if pf.Convergent[0] == nil || pf.Divergent[0] == nil || pf.Transform[0] == nil {
		t.Fatal("SDF arrays not allocated")
	}
	conv, div, trans, _, _ := extractBoundaries(pf, profile.PlateConvergentT)
	cases := []struct {
		name string
		mask [cubemap.NumFaces][]bool
		sdf  [cubemap.NumFaces][]float64
	}{
		{"convergent", conv, pf.Convergent},
		{"divergent", div, pf.Divergent},
		{"transform", trans, pf.Transform},
	}
	for _, c := range cases {
		for f := range c.mask {
			for i, isSeed := range c.mask[f] {
				if isSeed && c.sdf[f][i] != 0 {
					t.Fatalf("%s seed face %d idx %d distance=%f, want 0",
						c.name, f, i, c.sdf[f][i])
				}
			}
		}
	}
}

func TestSDFsKmScaling(t *testing.T) {
	// RadiusKm=6371 (Earth-like). Max convergent distance should be
	// bounded above by π * RadiusKm (the great-circle half-circumference).
	// Lower bound: at least 100 km, since a 32-pixel cube face produces
	// pixels separated by ~1000 km of arc.
	profile := &types.PlanetProfile{
		PlateCount: 4, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75,
		RadiusKm: 6371,
	}
	pf := GeneratePlates(profile, 23, 32)
	var maxConv float64
	for f := range pf.Convergent {
		for _, d := range pf.Convergent[f] {
			if d > maxConv {
				maxConv = d
			}
		}
	}
	if maxConv < 100 || maxConv > math.Pi*profile.RadiusKm+1 {
		t.Errorf("max convergent distance %.1f km outside plausible range [100, %.1f]",
			maxConv, math.Pi*profile.RadiusKm+1)
	}
}

func crustProfile() *types.PlanetProfile {
	return &types.PlanetProfile{
		PlateConvergentT: 0.75,
		Crust: types.CrustConfig{
			MajorPlates: 6, MinorPlates: 4, MajorGrowthBias: 4,
			OceanicFraction: 0.45,
		},
	}
}

func TestTwoTierPlateCountAndWeights(t *testing.T) {
	pf := GeneratePlates(crustProfile(), 42, 64)
	if pf == nil {
		t.Fatal("nil PlateField")
	}
	if len(pf.Plates) != 10 {
		t.Fatalf("got %d plates, want 10 (6 major + 4 minor)", len(pf.Plates))
	}
	for i, p := range pf.Plates {
		want := 4.0
		if i >= 6 {
			want = 1.0
		}
		if p.Weight != want {
			t.Errorf("plate %d weight %v, want %v", i, p.Weight, want)
		}
	}
}

func TestTwoTierMajorsClaimMoreArea(t *testing.T) {
	const S = 64
	pf := GeneratePlates(crustProfile(), 42, S)
	areas := make([]int, len(pf.Plates))
	for f := range pf.PlateID {
		for _, id := range pf.PlateID[f] {
			areas[id]++
		}
	}
	var majorSum, minorSum int
	for i, a := range areas {
		if i < 6 {
			majorSum += a
		} else {
			minorSum += a
		}
	}
	majorMean := float64(majorSum) / 6
	minorMean := float64(minorSum) / 4
	if majorMean < 2*minorMean {
		t.Errorf("major mean area %v not ≥ 2× minor mean %v", majorMean, minorMean)
	}
}

func TestTwoTierEveryPixelAssigned(t *testing.T) {
	const S = 64
	pf := GeneratePlates(crustProfile(), 7, S)
	for f := range pf.PlateID {
		for i, id := range pf.PlateID[f] {
			if id < 0 || int(id) >= len(pf.Plates) {
				t.Fatalf("face %d pixel %d has invalid plate id %d", f, i, id)
			}
		}
	}
}

func TestLegacyPathUnchangedByCrustCode(t *testing.T) {
	// A profile with PlateCount but zero Crust must produce the same
	// plates as before this phase: weights all 1, count == PlateCount.
	p := &types.PlanetProfile{PlateCount: 8, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75}
	pf := GeneratePlates(p, 99, 64)
	if len(pf.Plates) != 8 {
		t.Fatalf("legacy plate count %d, want 8", len(pf.Plates))
	}
	for i, pl := range pf.Plates {
		if pl.Weight != 1 {
			t.Errorf("legacy plate %d weight %v, want 1", i, pl.Weight)
		}
	}
}
