package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func terranCrustCfg() types.CrustConfig {
	return types.CrustConfig{
		MajorPlates: 7, MinorPlates: 4, MajorGrowthBias: 4,
		OceanicFraction: 0.45,
		Assembly:        -1, AssemblyWeights: [3]float64{25, 65, 10},
		TargetLandFraction: -1, LandFracLo: 0.22, LandFracHi: 0.38,
		TectonicAge: -1, AgeLo: 0.25, AgeHi: 0.75,
		CratonsMax: 8, ShelfWidthRad: 0.05,
		EdgeNoiseAmp: 0.45, EdgeNoiseFreq: 2.2, EdgeNoiseOctaves: 4,
		PlatformHeight: 0.62, OceanFloorHeight: 0.25,
	}
}

func TestResolveCrustParamsDeterministic(t *testing.T) {
	cfg := terranCrustCfg()
	a1, l1, g1 := ResolveCrustParams(cfg, 12345)
	a2, l2, g2 := ResolveCrustParams(cfg, 12345)
	if a1 != a2 || l1 != l2 || g1 != g2 {
		t.Fatalf("not deterministic: (%v,%v,%v) vs (%v,%v,%v)", a1, l1, g1, a2, l2, g2)
	}
}

func TestResolveCrustParamsRanges(t *testing.T) {
	cfg := terranCrustCfg()
	for master := range int64(200) {
		a, l, g := ResolveCrustParams(cfg, master)
		if a < 0 || a > 1 {
			t.Fatalf("seed %d: assembly %v out of [0,1]", master, a)
		}
		if l < cfg.LandFracLo || l > cfg.LandFracHi {
			t.Fatalf("seed %d: landFrac %v out of [%v,%v]", master, l, cfg.LandFracLo, cfg.LandFracHi)
		}
		if g < cfg.AgeLo || g > cfg.AgeHi {
			t.Fatalf("seed %d: age %v out of [%v,%v]", master, g, cfg.AgeLo, cfg.AgeHi)
		}
	}
}

func TestResolveCrustParamsPinned(t *testing.T) {
	cfg := terranCrustCfg()
	cfg.Assembly = 0.0 // pinned supercontinent — 0 is a valid pin, not "unset"
	cfg.TargetLandFraction = 0.3
	cfg.TectonicAge = 0.9
	a, l, g := ResolveCrustParams(cfg, 777)
	if a != 0.0 || l != 0.3 || g != 0.9 {
		t.Fatalf("pin ignored: got (%v,%v,%v)", a, l, g)
	}
}

func TestResolveCrustParamsZeroWeightsFallback(t *testing.T) {
	cfg := terranCrustCfg()
	cfg.AssemblyWeights = [3]float64{} // all zero → default weights {25, 65, 10}
	for master := range int64(50) {
		a, _, _ := ResolveCrustParams(cfg, master)
		if a < 0 || a > 1 {
			t.Fatalf("seed %d: assembly %v out of range", master, a)
		}
	}
}

func TestResolveCrustParamsAssemblyDistribution(t *testing.T) {
	// Weights 25/65/10 → over many seeds the band shares should be
	// roughly proportional (loose tolerance: ±10 points).
	cfg := terranCrustCfg()
	var bands [3]int
	const n = 2000
	for master := range int64(n) {
		a, _, _ := ResolveCrustParams(cfg, master)
		switch {
		case a < 0.33:
			bands[0]++
		case a < 0.67:
			bands[1]++
		default:
			bands[2]++
		}
	}
	want := [3]float64{0.25, 0.65, 0.10}
	for i := range bands {
		got := float64(bands[i]) / n
		if math.Abs(got-want[i]) > 0.10 {
			t.Errorf("band %d share %v, want %v ± 0.10", i, got, want[i])
		}
	}
}

func crustTestProfile() *types.PlanetProfile {
	return &types.PlanetProfile{
		PlateConvergentT: 0.75,
		Crust:            terranCrustCfg(),
	}
}

func TestPlaceCratonsOnCarrierPlates(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	cratons := PlaceCratons(p.Crust, 42, pf, 0.5, 0.3, S)
	if len(cratons) < 2 {
		t.Fatalf("got %d cratons, want ≥ 2", len(cratons))
	}
	for i, c := range cratons {
		f, px, py := cubemap.DirToFacePixel(c.Center[0], c.Center[1], c.Center[2], S)
		if got := int(pf.PlateID[f][py*S+px]); got != c.PlateID {
			t.Errorf("craton %d center sits on plate %d, recorded PlateID %d", i, got, c.PlateID)
		}
		if pf.Plates[c.PlateID].IsOceanic {
			t.Errorf("craton %d placed on oceanic plate %d", i, c.PlateID)
		}
		if c.Radius <= 0 || c.Radius > 1.3 {
			t.Errorf("craton %d radius %v out of (0, 1.3]", i, c.Radius)
		}
	}
}

func TestPlaceCratonsAreaBudget(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	for _, landFrac := range []float64{0.1, 0.3, 0.5} {
		cratons := PlaceCratons(p.Crust, 42, pf, 0.5, landFrac, S)
		var capSum float64 // Σ cap areas as fraction of sphere: (1-cos r)/2 each
		for _, c := range cratons {
			capSum += (1 - math.Cos(c.Radius)) / 2
		}
		// Budget includes a 1.15 overlap fudge; allow generous bounds —
		// exactness comes from the sea-level quantile, not from here.
		if capSum < landFrac*0.8 || capSum > landFrac*1.8 {
			t.Errorf("landFrac %v: cap-area sum %v outside [%v, %v]",
				landFrac, capSum, landFrac*0.8, landFrac*1.8)
		}
	}
}

func TestPlaceCratonsAssemblyClusters(t *testing.T) {
	// Supercontinent (assembly 0) cratons must be mutually closer than
	// fragmented (assembly 1) cratons for the same seed.
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	meanPairDot := func(cs []Craton) float64 {
		var sum float64
		var n int
		for i := range cs {
			for j := i + 1; j < len(cs); j++ {
				sum += dot3(cs[i].Center, cs[j].Center)
				n++
			}
		}
		if n == 0 {
			return 1
		}
		return sum / float64(n)
	}
	clustered := meanPairDot(PlaceCratons(p.Crust, 42, pf, 0.0, 0.3, S))
	scattered := meanPairDot(PlaceCratons(p.Crust, 42, pf, 1.0, 0.3, S))
	if clustered <= scattered {
		t.Errorf("assembly=0 mean pair dot %v not greater than assembly=1 %v", clustered, scattered)
	}
}

func TestGenerateCrustBasics(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	if crust == nil {
		t.Fatal("nil CrustField")
	}
	if crust.ContinentalMask == nil || crust.BaseHeight == nil {
		t.Fatal("nil mask or base height")
	}
	for f := range crust.ContinentalMask.Faces {
		for i, m := range crust.ContinentalMask.Faces[f] {
			if m < 0 || m > 1 || math.IsNaN(m) {
				t.Fatalf("mask face %d idx %d = %v out of [0,1]", f, i, m)
			}
			h := crust.BaseHeight.Faces[f][i]
			if h < 0 || h > 1 || math.IsNaN(h) {
				t.Fatalf("base height face %d idx %d = %v out of [0,1]", f, i, h)
			}
		}
	}
	if crust.LandFraction < 0.22 || crust.LandFraction > 0.38 {
		t.Errorf("resolved land fraction %v outside terran range", crust.LandFraction)
	}
}

func TestGenerateCrustLandAreaNearBudget(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	p.Crust.TargetLandFraction = 0.3 // pin for a deterministic assertion
	p.Crust.Assembly = 0.5
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	var land, total int
	for f := range crust.ContinentalMask.Faces {
		for _, m := range crust.ContinentalMask.Faces[f] {
			if m > 0.5 {
				land++
			}
			total++
		}
	}
	frac := float64(land) / float64(total)
	// Pre-sea-level mask area is approximate (overlap, edge noise);
	// the quantile stage enforces exactness. ±0.12 here.
	if math.Abs(frac-0.3) > 0.12 {
		t.Errorf("mask land fraction %v, want 0.3 ± 0.12", frac)
	}
}

func TestGenerateCrustDeterministic(t *testing.T) {
	const S = 32
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	a := GenerateCrust(p, 42, S, pf)
	b := GenerateCrust(p, 42, S, pf)
	for f := range a.BaseHeight.Faces {
		for i := range a.BaseHeight.Faces[f] {
			if a.BaseHeight.Faces[f][i] != b.BaseHeight.Faces[f][i] {
				t.Fatalf("nondeterministic at face %d idx %d", f, i)
			}
		}
	}
}
