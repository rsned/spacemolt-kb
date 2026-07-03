package feature

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestGenerateCratersLegacyDeterministic(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 99))
	a := GenerateCratersLegacy(rng, 50, 0.01, 0.05)
	rng = rand.New(rand.NewPCG(42, 99))
	b := GenerateCratersLegacy(rng, 50, 0.01, 0.05)
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
	for face := range cubemap.Face(cubemap.NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = 0.5
		}
	}
	// Single crater at +X axis. Age=1 so depth attenuation is identity.
	craters := []Crater{{Lat: 0, Lon: 0, Radius: 0.2, Age: 1.0, ParentIdx: -1}}
	ApplyCraters(cm, craters, 0.3)

	hCenter := cm.Get(cubemap.FacePosX, 32, 32)
	if hCenter >= 0.35 {
		t.Errorf("crater center h=%f, want < 0.35 (significant deposit)", hCenter)
	}

	hOpposite := cm.Get(cubemap.FaceNegX, 32, 32)
	if math.Abs(hOpposite-0.5) > 1e-9 {
		t.Errorf("opposite-side h=%f, want 0.5", hOpposite)
	}
}

func TestApplyCratersAgeAttenuates(t *testing.T) {
	// Old crater (low age) should leave a much shallower bowl than a young one.
	build := func(age float64) float64 {
		cm := cubemap.NewF(64)
		for face := range cubemap.Face(cubemap.NumFaces) {
			for i := range cm.Faces[face] {
				cm.Faces[face][i] = 0.5
			}
		}
		craters := []Crater{{Lat: 0, Lon: 0, Radius: 0.2, Age: age, ParentIdx: -1}}
		ApplyCraters(cm, craters, 0.3)
		return cm.Get(cubemap.FacePosX, 32, 32)
	}
	young := build(1.0)
	old := build(0.3)
	if young >= old {
		t.Errorf("young h=%f should be lower than old h=%f", young, old)
	}
}

func BenchmarkApplyCraters(b *testing.B) {
	cm := cubemap.NewF(256)
	craters := GenerateCratersLegacy(rand.New(rand.NewPCG(1, 2)), 200, 0.005, 0.08)
	b.ResetTimer()
	for b.Loop() {
		for face := range cubemap.Face(cubemap.NumFaces) {
			for i := range cm.Faces[face] {
				cm.Faces[face][i] = 0.5
			}
		}
		ApplyCraters(cm, craters, 0.2)
	}
}
