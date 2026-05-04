package feature

import (
	"math"
	"reflect"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// makeFakeSites builds n sites with linearly decreasing habitability
// from 0.99 down (in 0.005 steps for n=200 → 0.99, 0.985, …, 0.005).
// Dir is a fixed unit vector since AssignPopulations does not touch it.
func makeFakeSites(n int) []Site {
	sites := make([]Site, n)
	for i := range n {
		sites[i] = Site{
			Dir:          [3]float64{1, 0, 0},
			Habitability: 0.99 - 0.005*float64(i),
		}
	}
	return sites
}

// TestPopulationsZipfian verifies that AssignPopulations produces a
// log-log slope of (rank, population) close to the configured Zipf
// exponent (-1.07). With cfg.Tier > 0 and no cap, the closed-form
// population[i] = Tier * (i+1)^(-1.07) means OLS on (log(i+1),log(pop))
// must recover the exponent exactly up to float rounding. The accepted
// band [-1.15, -1.0] is a generous wiggle accounting for FP noise on
// 200 points.
func TestPopulationsZipfian(t *testing.T) {
	sites := makeFakeSites(200)
	AssignPopulations(sites, types.CivConfig{Tier: 0.8})

	// Hand-rolled OLS slope of log(rank+1) against log(population).
	n := float64(len(sites))
	var sumX, sumY, sumXY, sumXX float64
	for i, s := range sites {
		x := math.Log(float64(i + 1))
		y := math.Log(s.Population)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	meanX := sumX / n
	meanY := sumY / n
	slope := (sumXY - n*meanX*meanY) / (sumXX - n*meanX*meanX)

	// Expected slope is exactly -1.07 (the Zipf exponent the
	// implementation uses). Allow [-1.15, -1.0] for FP wiggle.
	if slope < -1.15 || slope > -1.0 {
		t.Fatalf("Zipf log-log slope = %v, want in [-1.15, -1.0] (~ -1.07)", slope)
	}
}

// TestPopulationsDeterministic verifies that AssignPopulations is
// purely deterministic: running it twice on equal inputs yields
// byte-identical Population values across the slice.
func TestPopulationsDeterministic(t *testing.T) {
	a := makeFakeSites(200)
	b := makeFakeSites(200)
	cfg := types.CivConfig{Tier: 0.8}

	AssignPopulations(a, cfg)
	AssignPopulations(b, cfg)

	if !reflect.DeepEqual(a, b) {
		t.Fatalf("AssignPopulations not deterministic: a[0]=%+v b[0]=%+v", a[0], b[0])
	}
}

// TestPopulationsRespectsCapAndZeroTier verifies the two scalar knobs:
//
//   - cfg.Tier == 0 is a no-op: every site keeps its zero-valued
//     Population (matching the "Tier == 0 disables civ" idiom).
//   - cfg.MaxPopulation > 0 caps the largest site exactly at
//     MaxPopulation; smaller sites whose Zipf value already sits
//     below the cap are unaffected.
func TestPopulationsRespectsCapAndZeroTier(t *testing.T) {
	// (a) Zero tier leaves Population untouched.
	zeroSites := makeFakeSites(50)
	AssignPopulations(zeroSites, types.CivConfig{Tier: 0})
	for i, s := range zeroSites {
		if s.Population != 0 {
			t.Fatalf("Tier=0: site[%d].Population = %v, want 0", i, s.Population)
		}
	}

	// (b) Cap clamps the top site.
	capped := makeFakeSites(50)
	cfg := types.CivConfig{Tier: 10.0, MaxPopulation: 0.5}
	AssignPopulations(capped, cfg)
	if capped[0].Population != 0.5 {
		t.Fatalf("MaxPopulation=0.5: site[0].Population = %v, want 0.5", capped[0].Population)
	}
	// Sites further down the rank should drop below the cap eventually.
	// At Tier=10, alpha=1.07, population[i] = 10*(i+1)^(-1.07).
	// (i+1)^1.07 > 20 ⇒ rank > ~17 has uncapped pop < 0.5.
	for i := 25; i < len(capped); i++ {
		if capped[i].Population > 0.5 {
			t.Fatalf("site[%d].Population = %v exceeds cap 0.5", i, capped[i].Population)
		}
	}
}
