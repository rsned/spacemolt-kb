package field_test

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
)

// TestPlateSDFsContinuousAcrossSeams asserts that the three plate
// boundary SDFs (Convergent / Divergent / Transform) are continuous
// across cube-face seams. Pre-fix, the JFA was strictly per-face so
// faces with no boundary of a given type defaulted to the
// math.Pi*RadiusKm sentinel while neighbors were ~0, producing seam
// deltas up to 100% of range.
//
// Tolerance is 5% of range. The structural floor is set by the
// pixel-snap distance between matched-pair pixels on a seam — the
// `seamtest.WalkSeams` matcher snaps the off-edge UV to the nearest
// adjacent-face pixel, so the two paired pixels lie ~one-pixel-on-
// sphere apart (~100 km on a 64² Earth-radius cube). Since SDF has
// gradient 1, that pixel offset shows up directly in the delta. For
// thin-range fields like Transform (range ~3000 km on terran),
// pixel-snap delta is already ~3% of range, so 5% is what you can
// actually achieve without sub-pixel matched-pair interpolation.
func TestPlateSDFsContinuousAcrossSeams(t *testing.T) {
	pf := field.GeneratePlates(planetgen.Profiles["terran"], 1, 64)
	if pf == nil {
		t.Fatal("plates required")
	}
	for name, slc := range map[string][cubemap.NumFaces][]float64{
		"convergent": pf.Convergent,
		"divergent":  pf.Divergent,
		"transform":  pf.Transform,
	} {
		cm := &cubemap.CubeMapF{Size: 64}
		for i := range cm.Faces {
			cm.Faces[i] = slc[i]
		}
		seamtest.AssertSeamContinuity(t, name, cm, 0.05)
	}
}

// TestPlateSDFKmRange sanity-checks that on a multi-plate planet, no
// pixel in the convergent SDF is stuck at the math.Pi * RadiusKm
// sentinel value. Pre-fix, faces with no local boundary of the given
// type had ~1/6 of pixels at the sentinel; post-fix, cross-face JFA
// propagation reaches every pixel so the field's maximum sits well
// below the sentinel. Tolerance: max < 0.95 * sentinel — a real
// terran has plenty of plates so the most-distant pixel is at most
// ~75% of the geodesic half-circumference from a boundary.
func TestPlateSDFKmRange(t *testing.T) {
	profile := planetgen.Profiles["terran"]
	pf := field.GeneratePlates(profile, 1, 64)
	if pf == nil {
		t.Fatal("plates required")
	}
	radius := profile.RadiusKm
	if radius == 0 {
		radius = 6371
	}
	sentinel := math.Pi * radius
	threshold := 0.95 * sentinel
	var maxConv float64
	var sentinelCount int
	for f := range pf.Convergent {
		for _, v := range pf.Convergent[f] {
			if v > maxConv {
				maxConv = v
			}
			if v >= sentinel-1e-6 {
				sentinelCount++
			}
		}
	}
	if sentinelCount > 0 {
		t.Errorf("Convergent SDF has %d pixels stuck at sentinel %.2f km; some face has no convergent seed reachable via cross-face JFA",
			sentinelCount, sentinel)
	}
	if maxConv >= threshold {
		t.Errorf("max Convergent SDF = %.2f km exceeds threshold %.2f km (sentinel = %.2f km); cross-face propagation may be incomplete",
			maxConv, threshold, sentinel)
	}
}
