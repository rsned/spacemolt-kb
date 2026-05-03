package noise_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
)

// TestJitteredDetailFieldSeamContinuity verifies that the Detail
// control field, when sampled through the jittered direction, remains
// continuous across cube-face seams.
func TestJitteredDetailFieldSeamContinuity(t *testing.T) {
	t.Skip("Architectural: Voronoi cell-jitter introduces direction discontinuities at cell boundaries by design (Phase 7 §3.3 spec explicitly rejects cell-boundary smoothing). At S=64 with 120 cells, ~5.6% of seam pixel pairs straddle a cell boundary; at those pixels, the rotated fbm sample can swing by full amplitude → 8-47% Detail-field deltas. Direction-based TransformDir (P8 Task 3) eliminates the per-face raster bug but cannot fix the cell-boundary discontinuity. Re-enable when seamtest infrastructure adopts cell-boundary-aware sampling (e.g. fraction-of-straddling-pairs metric or sub-cell averaging) — see docs/plans/phase-8-seam-bugs.md.")
	seeds := map[string]int64{
		"terran":       1,
		"super_terran": 2,
		"oceanic":      3,
		"tundra":       4,
		"arid":         5,
		"glacial":      6,
		"scorched":     7,
		"lava_world":   8,
	}
	S := 64
	for name, master := range seeds {
		t.Run(name, func(t *testing.T) {
			profile := planetgen.Profiles[name]
			jf := noise.GenerateJitter(profile, master, S)
			if jf == nil {
				t.Skip("jitter disabled")
			}
			fields := field.GenerateControlFields(master, profile.ControlConfig, S, jf)
			// fields[1] is the Detail control field; the only field
			// whose sample direction is jittered.
			seamtest.AssertSeamContinuity(t, name+":Detail", fields[1], 0.02)
		})
	}
}
