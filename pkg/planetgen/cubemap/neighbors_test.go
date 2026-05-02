package cubemap

import (
	"testing"
)

func TestFacePixelNeighbors4Interior(t *testing.T) {
	S := 16
	nbrs := FacePixelNeighbors4(FacePosX, 5, 5, S)
	if len(nbrs) != 4 {
		t.Fatalf("interior should have 4 neighbors, got %d", len(nbrs))
	}
	for _, n := range nbrs {
		if n.Face != FacePosX {
			t.Errorf("interior neighbor on different face: %+v", n)
		}
		if n.PX < 0 || n.PX >= S || n.PY < 0 || n.PY >= S {
			t.Errorf("neighbor out of [0,S): %+v", n)
		}
	}
}

func TestFacePixelNeighbors4Corner(t *testing.T) {
	S := 16
	// Top-left corner: neighbors at (-1, 0) and (0, -1) cross face boundaries.
	nbrs := FacePixelNeighbors4(FacePosX, 0, 0, S)
	if len(nbrs) != 4 {
		t.Fatalf("corner should have 4 neighbors, got %d", len(nbrs))
	}
	var crossFace int
	for _, n := range nbrs {
		if n.Face != FacePosX {
			crossFace++
		}
		if n.PX < 0 || n.PX >= S || n.PY < 0 || n.PY >= S {
			t.Errorf("corner neighbor out of [0,S): %+v", n)
		}
	}
	if crossFace != 2 {
		t.Errorf("corner should have 2 cross-face neighbors, got %d (%+v)", crossFace, nbrs)
	}
}

func TestFacePixelNeighbors4Symmetry(t *testing.T) {
	// Walking from an edge pixel to a cross-face neighbor and asking
	// for that neighbor's neighbors should produce a pixel within ±1
	// of the start. (Cube-face cross-projection is not pixel-exact;
	// ±1 tolerance covers the rounding.)
	S := 16
	type startPt struct {
		Face   Face
		PX, PY int
	}
	starts := []startPt{
		{FacePosX, 0, S / 2},     // left edge
		{FacePosX, S - 1, S / 2}, // right edge
		{FacePosX, S / 2, 0},     // top edge
		{FacePosX, S / 2, S - 1}, // bottom edge
	}
	for _, s := range starts {
		for _, nbr := range FacePixelNeighbors4(s.Face, s.PX, s.PY, S) {
			if nbr.Face == s.Face {
				continue // not a cross-face hop
			}
			back := FacePixelNeighbors4(nbr.Face, nbr.PX, nbr.PY, S)
			var found bool
			for _, b := range back {
				if b.Face == s.Face && absInt(b.PX-s.PX) <= 1 && absInt(b.PY-s.PY) <= 1 {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no return-walk from face=%v (%d,%d) via face=%v (%d,%d); back=%+v",
					s.Face, s.PX, s.PY, nbr.Face, nbr.PX, nbr.PY, back)
			}
		}
	}
}
