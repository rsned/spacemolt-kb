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

func TestJumpFloodFromMaskWithValuePropagates(t *testing.T) {
	// Two seeds with distinct values on opposite faces. Every pixel
	// must inherit the value of its nearest seed: the closer half of
	// the cube to each seed reports that seed's value.
	S := 16
	var mask [cubemap.NumFaces][]bool
	var value [cubemap.NumFaces][]float64
	for f := range mask {
		mask[f] = make([]bool, S*S)
		value[f] = make([]float64, S*S)
	}
	idxAt := func(px, py int) int { return py*S + px }
	mask[cubemap.FacePosX][idxAt(S/2, S/2)] = true
	value[cubemap.FacePosX][idxAt(S/2, S/2)] = 0.25
	mask[cubemap.FaceNegX][idxAt(S/2, S/2)] = true
	value[cubemap.FaceNegX][idxAt(S/2, S/2)] = 0.75
	_, mag := JumpFloodFromMaskWithValue(mask, value, S)
	if got := mag.Get(cubemap.FacePosX, S/2, S/2); math.Abs(got-0.25) > 1e-5 {
		t.Errorf("PosX center value: got %f, want 0.25", got)
	}
	if got := mag.Get(cubemap.FaceNegX, S/2, S/2); math.Abs(got-0.75) > 1e-5 {
		t.Errorf("NegX center value: got %f, want 0.75", got)
	}
	// PosY face center sits equidistant from the two seeds; it can
	// pick either deterministically — we only require it to be one
	// of the input values, not midway.
	got := mag.Get(cubemap.FacePosY, S/2, S/2)
	if math.Abs(got-0.25) > 1e-5 && math.Abs(got-0.75) > 1e-5 {
		t.Errorf("PosY center value: got %f, want 0.25 or 0.75", got)
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
