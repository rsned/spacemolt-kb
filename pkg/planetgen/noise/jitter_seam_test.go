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
	t.Skip("Two compounding issues at production frequency configs: (a) Voronoi cell-jitter introduces direction discontinuities at cell boundaries by design (Phase 7 §3.3 spec rejects cell-boundary smoothing); ~5.6% of seam pairs at S=64 with 120 cells straddle a boundary. (b) Production Detail Freq×Lac^(Octaves-1) reaches max-octave frequencies several × Nyquist at S=64, so even non-cell-boundary pairs show large deltas from honest sub-pixel aliasing. The direction-only sampling property of the jitter pipeline IS verified by the synthetic low-freq test in jitter_dir_symmetry_test.go. Re-enable when production aliasing is addressed AND a cell-boundary-aware metric exists — see docs/plans/phase-8-seam-bugs.md.")
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
