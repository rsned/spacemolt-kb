package field

import (
	"math"
	"testing"

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
