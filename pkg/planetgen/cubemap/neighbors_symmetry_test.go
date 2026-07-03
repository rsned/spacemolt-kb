package cubemap_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// TestFacePixelNeighbors4Symmetric asserts the contract that the
// flood-fill (and any other adjacency-based traversal across cube
// faces) needs from FacePixelNeighbors4: if (F1,x1,y1) lists
// (F2,x2,y2) as a neighbor, then (F2,x2,y2) must list (F1,x1,y1)
// as a neighbor. Without this, a BFS frontier can claim a
// matched-pair pixel from one face and never even consider it from
// the other side, producing seam-id mismatches.
func TestFacePixelNeighbors4Symmetric(t *testing.T) {
	S := 64
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				if px > 0 && px < S-1 && py > 0 && py < S-1 {
					continue // interior — symmetry trivially holds
				}
				nbrs := cubemap.FacePixelNeighbors4(f, px, py, S)
				for _, n := range nbrs {
					if n.Face == f {
						continue
					}
					back := cubemap.FacePixelNeighbors4(n.Face, n.PX, n.PY, S)
					found := false
					for _, b := range back {
						if b.Face == f && b.PX == px && b.PY == py {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("(%v,%d,%d) → (%v,%d,%d) not symmetric; back=%v",
							f, px, py, n.Face, n.PX, n.PY, back)
					}
				}
			}
		}
	}
}
