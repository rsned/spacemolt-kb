package feature

import (
	"math"
	"reflect"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// hfUniform builds a HabitabilityField of size S filled with v on every
// face/pixel.
func hfUniform(S int, v float64) *HabitabilityField {
	hf := &HabitabilityField{Size: S}
	for f := range cubemap.Face(cubemap.NumFaces) {
		hf.Score[f] = make([]float64, S*S)
		for i := range hf.Score[f] {
			hf.Score[f][i] = v
		}
	}
	return hf
}

// hfPlusXHotspot builds a HabitabilityField where the +X face is fully
// habitable (0.9) and all other faces are zero. Used to verify cluster
// placement biases toward habitable regions.
func hfPlusXHotspot(S int) *HabitabilityField {
	hf := &HabitabilityField{Size: S}
	for f := range cubemap.Face(cubemap.NumFaces) {
		hf.Score[f] = make([]float64, S*S)
	}
	for i := range hf.Score[cubemap.FacePosX] {
		hf.Score[cubemap.FacePosX][i] = 0.9
	}
	return hf
}

func defaultCivCfg() types.CivConfig {
	return types.CivConfig{
		Tier:           0.5,
		SiteMinDistRad: 0.05,
		SiteMaxDistRad: 0.20,
	}
}

// arcDist returns the great-circle distance (radians) between two unit vectors.
func arcDist(a, b [3]float64) float64 {
	d := a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
	if d > 1 {
		d = 1
	} else if d < -1 {
		d = -1
	}
	return math.Acos(d)
}

// TestPoissonNoTwoSitesCloserThanMin verifies the Bridson minimum-distance
// invariant: no two sites are closer than cfg.SiteMinDistRad on the sphere.
// We use a 1e-6 tolerance to account for floating-point rounding when
// per-pixel radii hit the floor at habitability boundaries.
func TestPoissonNoTwoSitesCloserThanMin(t *testing.T) {
	hf := hfUniform(32, 0.7)
	cfg := defaultCivCfg()
	sites := PoissonOnSphere(hf, cfg, 1234)
	if len(sites) < 2 {
		t.Fatalf("expected >= 2 sites for uniform habitable field, got %d", len(sites))
	}
	const tol = 1e-6
	for i := 0; i < len(sites); i++ {
		for j := i + 1; j < len(sites); j++ {
			d := arcDist(sites[i].Dir, sites[j].Dir)
			if d < cfg.SiteMinDistRad-tol {
				t.Errorf("sites %d and %d arc distance %.6f < SiteMinDistRad %.6f", i, j, d, cfg.SiteMinDistRad)
			}
		}
	}
	t.Logf("uniform-field site count: %d", len(sites))
}

// TestPoissonDeterministic verifies that two calls with the same master
// seed and identical inputs produce byte-identical site slices.
func TestPoissonDeterministic(t *testing.T) {
	hf := hfUniform(32, 0.7)
	cfg := defaultCivCfg()
	a := PoissonOnSphere(hf, cfg, 9876)
	b := PoissonOnSphere(hf, cfg, 9876)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("non-deterministic output: len(a)=%d len(b)=%d", len(a), len(b))
	}
}

// TestPoissonSitesClusterInHabitableRegions verifies that sites cluster in
// regions of high habitability: the mean habitability of returned sites
// should exceed 0.6 when the input field has a single high-habitability
// region (0.9 on +X face) and zero elsewhere.
func TestPoissonSitesClusterInHabitableRegions(t *testing.T) {
	hf := hfPlusXHotspot(32)
	cfg := defaultCivCfg()
	sites := PoissonOnSphere(hf, cfg, 4242)
	if len(sites) == 0 {
		t.Fatalf("expected sites in the +X hotspot, got 0")
	}
	var sum float64
	for _, s := range sites {
		sum += s.Habitability
	}
	mean := sum / float64(len(sites))
	if mean <= 0.6 {
		t.Errorf("mean habitability of %d sites = %.3f; want > 0.6", len(sites), mean)
	}
	t.Logf("hotspot site count: %d, mean habitability: %.3f", len(sites), mean)
}

// TestPoissonNoSitesInOcean verifies that an all-zero (sub-floor)
// habitability field yields no sites.
func TestPoissonNoSitesInOcean(t *testing.T) {
	hf := hfUniform(32, 0.0)
	cfg := defaultCivCfg()
	sites := PoissonOnSphere(hf, cfg, 7)
	if len(sites) != 0 {
		t.Errorf("expected 0 sites in ocean-only field, got %d", len(sites))
	}
}

// TestPoissonReturnsZeroSitesForCivTierZero verifies the Tier<=0 gate.
func TestPoissonReturnsZeroSitesForCivTierZero(t *testing.T) {
	hf := hfUniform(32, 0.7)
	cfg := types.CivConfig{Tier: 0, SiteMinDistRad: 0.05, SiteMaxDistRad: 0.2}
	sites := PoissonOnSphere(hf, cfg, 1)
	if sites != nil {
		t.Errorf("expected nil for Tier=0, got len=%d", len(sites))
	}
}

// TestPoissonNilHabitabilityField verifies the nil-input gate.
func TestPoissonNilHabitabilityField(t *testing.T) {
	cfg := defaultCivCfg()
	sites := PoissonOnSphere(nil, cfg, 1)
	if sites != nil {
		t.Errorf("expected nil for nil hf, got len=%d", len(sites))
	}
}
