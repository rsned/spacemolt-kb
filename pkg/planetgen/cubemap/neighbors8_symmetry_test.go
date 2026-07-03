package cubemap_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// TestFacePixelNeighbors8Symmetric mirrors TestFacePixelNeighbors4Symmetric
// for the 8-connected neighbor helper used by the D8 flow + Planchon-
// Darboux fill. If (F1,x1,y1) lists (F2,x2,y2) as a neighbor, then
// (F2,x2,y2) must list (F1,x1,y1) as a neighbor — without this, D8
// pointer chains can be one-way across face seams and accumulation
// passes produce per-face artefacts at the cube boundaries.
func TestFacePixelNeighbors8Symmetric(t *testing.T) {
	S := 64
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				if px > 0 && px < S-1 && py > 0 && py < S-1 {
					continue // interior — symmetry trivially holds
				}
				nbrs := cubemap.FacePixelNeighbors8(f, px, py, S)
				for _, n := range nbrs {
					if n.Face == f {
						continue
					}
					back := cubemap.FacePixelNeighbors8(n.Face, n.PX, n.PY, S)
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
