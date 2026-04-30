package cubemap

import "math"

// DirToFacePixel returns the (face, px, py) of the cube-map cell that the
// unit-sphere direction (x, y, z) projects onto for a face of side S.
// Inverse of FacePixelToDir; round-trips within ±1 pixel.
func DirToFacePixel(x, y, z float64, S int) (Face, int, int) {
	face, u, v := DirToFaceUV(x, y, z)
	px := int(math.Floor(u * float64(S)))
	py := int(math.Floor(v * float64(S)))
	if px < 0 {
		px = 0
	}
	if px >= S {
		px = S - 1
	}
	if py < 0 {
		py = 0
	}
	if py >= S {
		py = S - 1
	}
	return face, px, py
}
