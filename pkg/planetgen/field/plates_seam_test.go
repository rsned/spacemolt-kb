package field_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
)

// TestPlateFieldSeamMatch verifies that the plate-id field matches
// exactly across cube-face seams (categorical) and that the three
// boundary SDFs are continuous within a tight tolerance.
func TestPlateFieldSeamMatch(t *testing.T) {
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
			pf := field.GeneratePlates(profile, master, S)
			if pf == nil {
				t.Skip("PlateCount=0 for this archetype")
			}
			seamtest.AssertSeamMatch(t, name+":plate-id", pf.PlateID, S)
			for kind, slc := range map[string][cubemap.NumFaces][]float64{
				"convergent": pf.Convergent,
				"divergent":  pf.Divergent,
				"transform":  pf.Transform,
			} {
				cm := &cubemap.CubeMapF{Size: S}
				for i := range cm.Faces {
					cm.Faces[i] = slc[i]
				}
				// 5% threshold: seamtest.WalkSeams pixel-snap (one-pixel
				// step beyond the edge) costs ~100 km on the SDF's range
				// of ~3300 km on Transform, an inherent ~3% floor that
				// is not a flood-fill or JFA bug. Convergent/divergent
				// pass within 2% but Transform routinely lands at 2.2-
				// 3.1%; widen uniformly for simplicity.
				seamtest.AssertSeamContinuity(t, name+":"+kind, cm, 0.05)
			}
		})
	}
}
