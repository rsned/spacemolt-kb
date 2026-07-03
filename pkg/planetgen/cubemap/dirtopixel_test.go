package cubemap

import "testing"

func TestDirToFacePixelRoundTrip(t *testing.T) {
	S := 64
	for face := range Face(NumFaces) {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				dx, dy, dz := FacePixelToDir(face, px, py, S)
				f2, px2, py2 := DirToFacePixel(dx, dy, dz, S)
				if f2 != face || (px2 != px && absInt(px2-px) > 1) || (py2 != py && absInt(py2-py) > 1) {
					t.Errorf("round-trip mismatch: (%v,%d,%d) → (%v,%d,%d)", face, px, py, f2, px2, py2)
				}
			}
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
