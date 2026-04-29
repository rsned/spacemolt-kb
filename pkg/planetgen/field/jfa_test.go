package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestJFAOneSeed(t *testing.T) {
	// Single ocean pixel at (+X face, S/2, S/2); everywhere else is land.
	S := 32
	hm := cubemap.NewF(S)
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		for i := range hm.Faces[face] {
			hm.Faces[face][i] = 1.0 // all land
		}
	}
	hm.Set(cubemap.FacePosX, S/2, S/2, 0.0) // single ocean pixel

	dist := DistanceToCoast(hm, 0.5, S)

	// Pixel at the seed should be ~0.
	if got := dist.Get(cubemap.FacePosX, S/2, S/2); got > 0.05 {
		t.Errorf("seed distance: got %f, want ~0", got)
	}
	// Antipodal pixel should be ~π/π == 1.
	if got := dist.Get(cubemap.FaceNegX, S/2, S/2); math.Abs(got-1) > 0.1 {
		t.Errorf("antipodal distance: got %f, want ~1", got)
	}
}

func TestJFANoOcean(t *testing.T) {
	S := 16
	hm := cubemap.NewF(S)
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		for i := range hm.Faces[face] {
			hm.Faces[face][i] = 1.0
		}
	}
	dist := DistanceToCoast(hm, 0.5, S)
	// No ocean → every pixel reports max distance (1.0).
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		for i, v := range dist.Faces[face] {
			if v < 0.999 {
				t.Errorf("face %v idx %d: got %f, want ~1.0 (no ocean seeds)", face, i, v)
			}
		}
	}
}
