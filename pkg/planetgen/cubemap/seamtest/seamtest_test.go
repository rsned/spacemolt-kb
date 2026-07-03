package seamtest

import (
	"math"
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

// TestAdjacentEdgePixelMatchedPair walks every cube-edge pixel and
// verifies that adjacentEdgePixel returns the *matched-pair* pixel
// on the neighbor face — i.e. the pixel whose 3D direction is closest
// to the source pixel's direction. Acceptance: the angle between the
// source and matched directions must be ≤ one pixel diagonal
// (sqrt(2)/S radians). The legacy one-pixel-step implementation gives
// ~3×/S separation and fails this test.
func TestAdjacentEdgePixelMatchedPair(t *testing.T) {
	S := 64
	pixelDiag := math.Sqrt(2) / float64(S) // worst-case half-pixel snap on each side, summed
	edges := [4]Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight}
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		for _, e := range edges {
			for i := 0; i < S; i++ {
				px, py := edgePixel(e, i, S)
				hx, hy, hz := cubemap.FacePixelToDir(f, px, py, S)
				tFace, tPx, tPy := adjacentEdgePixel(f, e, i, S)
				if tFace == f {
					t.Errorf("adjacentEdgePixel(%v, %v, %d) returned same face — eps step did not cross edge",
						f, e, i)
					continue
				}
				tx, ty, tz := cubemap.FacePixelToDir(tFace, tPx, tPy, S)
				// Angle between unit vectors: acos(h·t). Use chord
				// distance as a tighter, numerically-stabler bound.
				dx, dy, dz := hx-tx, hy-ty, hz-tz
				chord := math.Sqrt(dx*dx + dy*dy + dz*dz)
				if chord > pixelDiag {
					t.Errorf("face=%v edge=%v idx=%d: matched-pair chord %.4f exceeds one-pixel-diagonal %.4f",
						f, e, i, chord, pixelDiag)
				}
			}
		}
	}
}

type mockT struct {
	failed bool
}

func (m *mockT) Errorf(string, ...any) { m.failed = true }
func (m *mockT) Fatalf(string, ...any) { m.failed = true }
func (m *mockT) Helper()               {}
