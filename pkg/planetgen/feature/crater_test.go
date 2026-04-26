package feature

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestGenerateCratersDeterministic(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 99))
	a := GenerateCraters(rng, 50, 0.01, 0.05)
	rng = rand.New(rand.NewPCG(42, 99))
	b := GenerateCraters(rng, 50, 0.01, 0.05)
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("crater[%d] mismatch", i)
		}
	}
}

func TestApplyCratersStampSingle(t *testing.T) {
	cm := cubemap.NewF(64)
	// Pre-fill heightmap to 0.5 everywhere.
	for face := range cubemap.Face(cubemap.NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = 0.5
		}
	}
	// Single crater at +X axis, angular radius 0.2 rad, depth 0.3.
	craters := []Crater{{Lat: 0, Lon: 0, Radius: 0.2}}
	ApplyCraters(cm, craters, 0.3)

	// Pixel at +X face center (px=S/2, py=S/2) is at direction ~(1,0,0),
	// inside the crater. Bowl formula at t≈0 gives mod≈-0.3, so the
	// pre-filled 0.5 should drop to ~0.2; require a meaningful drop
	// to catch wrong-sign or wrong-formula bugs.
	hCenter := cm.Get(cubemap.FacePosX, 32, 32)
	if hCenter >= 0.35 {
		t.Errorf("crater center h=%f, want < 0.35 (significant deposit)", hCenter)
	}

	// Pixel at -X face center is at direction (-1,0,0), arc π away —
	// untouched.
	hOpposite := cm.Get(cubemap.FaceNegX, 32, 32)
	if math.Abs(hOpposite-0.5) > 1e-9 {
		t.Errorf("opposite-side h=%f, want 0.5", hOpposite)
	}
}
