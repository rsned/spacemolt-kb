package seamtest

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestWalkSeamsVisitsEveryFaceEdge(t *testing.T) {
	S := 16
	var faces [cubemap.NumFaces][]int
	for f := range faces {
		faces[f] = make([]int, S*S)
	}
	visits := 0
	WalkSeams(faces, S, func(_ cubemap.Face, _ Edge, _ int, _, _ int) {
		visits++
	})
	// 6 faces × 4 edges × S pixels — each cube edge visited from both
	// adjoining faces (the helper is intentionally symmetric; consumers
	// are dedup-agnostic).
	if visits != 24*S {
		t.Errorf("WalkSeams visited %d pairs, want %d", visits, 24*S)
	}
}

func TestAssertSeamMatchPasses(t *testing.T) {
	S := 8
	var faces [cubemap.NumFaces][]int16
	for f := range faces {
		faces[f] = make([]int16, S*S)
	}
	// All zeros → every seam pixel matches.
	AssertSeamMatch(t, "all-zeros", faces, S)
}

func TestAssertSeamMatchFailsForMismatch(t *testing.T) {
	S := 8
	var faces [cubemap.NumFaces][]int16
	for f := range faces {
		faces[f] = make([]int16, S*S)
	}
	// Force a mismatch on the +X face top-left edge pixel.
	faces[cubemap.FacePosX][0] = 99
	inner := &mockT{}
	AssertSeamMatch(inner, "synthetic", faces, S)
	if !inner.failed {
		t.Errorf("expected failure on synthetic mismatch")
	}
}

func TestAssertSeamContinuityPasses(t *testing.T) {
	S := 8
	cm := cubemap.NewF(S)
	for f := range cm.Faces {
		for i := range cm.Faces[f] {
			cm.Faces[f][i] = 0.5
		}
	}
	AssertSeamContinuity(t, "constant", cm, 0.01)
}

type mockT struct {
	failed bool
}

func (m *mockT) Errorf(string, ...any) { m.failed = true }
func (m *mockT) Fatalf(string, ...any) { m.failed = true }
func (m *mockT) Helper()               {}
