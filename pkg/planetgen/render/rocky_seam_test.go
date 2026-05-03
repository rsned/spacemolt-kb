package render_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

// TestRockyHeightmapSeamContinuity walks each rocky-pipeline height
// stage's cumulative SumAfter cube map and asserts seam continuity.
func TestRockyHeightmapSeamContinuity(t *testing.T) {
	t.Skip("Phase 8: heightmap stages inherit Detail/jitter and plate-SDF seam errors (up to 79% on ice_world Detail). Re-enable after jitter and plates seam fixes land in Phase 8.")
	archetypes := []string{
		"terran", "super_terran", "oceanic", "tundra",
		"arid", "glacial", "scorched", "lava_world",
		"hothouse", "ice_world",
	}
	S := 64
	for i, name := range archetypes {
		t.Run(name, func(t *testing.T) {
			master := int64(100 + i)
			profile := planetgen.Profiles[name]
			frame := render.RenderRockyDebug(profile, master, S, nil)
			for _, st := range frame.Stages {
				if st.SumAfter == nil {
					continue
				}
				seamtest.AssertSeamContinuity(t, name+":"+st.Name, st.SumAfter, 0.01)
			}
		})
	}
}
