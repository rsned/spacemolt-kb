package feature

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func defaultCloudProfile() *types.PlanetProfile {
	return &types.PlanetProfile{
		Cloud: types.CloudConfig{
			Coverage:       0.5,
			BandLatRad:     math.Pi / 12,
			Freq:           4,
			Octaves:        4,
			WarpAmp:        0.4,
			StormCount:     4,
			StormRadiusRad: math.Pi / 16,
			SunDir:         [3]float64{1, 0.3, 0},
			ShadowGain:     0.5,
		},
	}
}

// TestGenerateCloudsDeterministic ensures repeated calls with the same
// profile + seed produce byte-identical density and alpha output.
func TestGenerateCloudsDeterministic(t *testing.T) {
	profile := defaultCloudProfile()
	const S = 32
	const seedVal int64 = 42

	a := GenerateClouds(profile, seedVal, S)
	b := GenerateClouds(profile, seedVal, S)

	if a == nil || b == nil {
		t.Fatalf("GenerateClouds returned nil")
	}
	if a.Size != S || b.Size != S {
		t.Fatalf("Size mismatch: a=%d b=%d", a.Size, b.Size)
	}
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		for i := range a.Density[f] {
			if a.Density[f][i] != b.Density[f][i] {
				t.Fatalf("Density mismatch face=%d i=%d a=%v b=%v",
					f, i, a.Density[f][i], b.Density[f][i])
			}
			if a.Alpha[f][i] != b.Alpha[f][i] {
				t.Fatalf("Alpha mismatch face=%d i=%d a=%v b=%v",
					f, i, a.Alpha[f][i], b.Alpha[f][i])
			}
		}
	}
}

// TestGenerateCloudsDisabled ensures Coverage=0 produces nil.
func TestGenerateCloudsDisabled(t *testing.T) {
	profile := defaultCloudProfile()
	profile.Cloud.Coverage = 0

	cf := GenerateClouds(profile, 1, 32)
	if cf != nil {
		t.Fatalf("expected nil for Coverage=0, got %+v", cf)
	}
}

// TestGenerateCloudsCoverageInRange checks that the mean alpha across
// the cube-map sphere lands within a sane envelope around the
// configured coverage. The empirical mean of cos(lat) over cube-map
// pixels is ≈0.79 (cube-map sampling oversamples the equator); with
// the multiplicative detail term centered on 1.0, expected mean alpha
// is roughly Coverage * 0.79 ≈ 0.4 for Coverage=0.5. We allow generous
// bounds [Coverage * 0.5, Coverage * 1.4] to absorb +/- noise plus
// asymmetric clamping at the [0,1] boundary.
func TestGenerateCloudsCoverageInRange(t *testing.T) {
	profile := defaultCloudProfile()
	profile.Cloud.StormCount = 0 // exclude storms so we measure the band+detail mean
	const S = 64

	cf := GenerateClouds(profile, 7, S)
	if cf == nil {
		t.Fatalf("GenerateClouds returned nil")
	}

	var sum float64
	var n int
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		for _, v := range cf.Alpha[f] {
			sum += v
			n++
		}
	}
	mean := sum / float64(n)
	cov := profile.Cloud.Coverage
	lo := cov * 0.5
	hi := cov * 1.4
	if mean < lo || mean > hi {
		t.Fatalf("mean alpha %.4f outside [%.4f, %.4f] (Coverage=%.4f)",
			mean, lo, hi, cov)
	}
	t.Logf("mean alpha = %.4f (%.2f%% of Coverage=%.4f)",
		mean, 100*mean/cov, cov)
}

// TestGenerateCloudsSeamContinuity verifies cloud density is continuous
// across cube-face seams. Threshold is 10% to absorb the inherent
// pixel-snap floor on direction-pure fields (per P8 Task 4 lesson)
// plus the steep gradient inside the Rankine storm cores (each storm
// is a localized 0.5-amplitude bump over ~π/16 rad — at S=64 that
// produces per-pixel deltas up to ~0.07 near the storm rim, which
// pixel-snap on a seam can exaggerate to ~0.08).
func TestGenerateCloudsSeamContinuity(t *testing.T) {
	profile := defaultCloudProfile()
	const S = 64

	cf := GenerateClouds(profile, 11, S)
	if cf == nil {
		t.Fatalf("GenerateClouds returned nil")
	}

	// Wrap the density into a CubeMapF for seamtest.
	dm := &cubemap.CubeMapF{Size: S}
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		dm.Faces[f] = make([]float64, len(cf.Density[f]))
		copy(dm.Faces[f], cf.Density[f])
	}
	seamtest.AssertSeamContinuity(t, "cloud density", dm, 0.10)
}
