package cubemap

// PixelAddr identifies a pixel on a cube map by face and (px, py).
type PixelAddr struct {
	Face   Face
	PX, PY int
}

// FacePixelNeighbors4 returns the four 4-connected neighbors of (face,
// px, py) at face size S. In-face neighbors are returned with the
// same Face. Off-edge neighbors are remapped to the adjacent face by
// projecting a unit half-pixel beyond the edge through FaceUVToDir
// and re-projecting via DirToFacePixel.
//
// Returned addresses always lie within [0, S) on both axes; clamping
// is delegated to DirToFacePixel. Cross-face neighbors carry the
// adjacent face id. Order is: +x, -x, +y, -y.
func FacePixelNeighbors4(face Face, px, py, S int) [4]PixelAddr {
	deltas := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	var out [4]PixelAddr
	for i, d := range deltas {
		nx, ny := px+d[0], py+d[1]
		if nx >= 0 && nx < S && ny >= 0 && ny < S {
			out[i] = PixelAddr{Face: face, PX: nx, PY: ny}
			continue
		}
		// Off-edge: project a half-pixel beyond the edge through the
		// face's UV space, take the resulting unit direction, and
		// re-project to (face, px, py) via DirToFacePixel.
		u := (float64(px) + 0.5 + float64(d[0])) / float64(S)
		v := (float64(py) + 0.5 + float64(d[1])) / float64(S)
		dx, dy, dz := FaceUVToDir(face, u, v)
		nf, npx, npy := DirToFacePixel(dx, dy, dz, S)
		out[i] = PixelAddr{Face: nf, PX: npx, PY: npy}
	}
	return out
}

// OffsetPixel returns the pixel at (px+dx, py+dy) on face, projecting
// across face boundaries when the offset leaves the face bounds. Uses
// half-pixel-centered direction snap. dx and dy can be any int.
//
// In-face offsets are returned as-is with the same Face. Off-edge
// offsets are remapped by computing the unit-direction at the offset
// UV and re-projecting via DirToFacePixel; the returned (Face, px, py)
// always lies in [0, S) on both axes.
func OffsetPixel(face Face, px, py, dx, dy, S int) (Face, int, int) {
	nx, ny := px+dx, py+dy
	if nx >= 0 && nx < S && ny >= 0 && ny < S {
		return face, nx, ny
	}
	u := (float64(px) + 0.5 + float64(dx)) / float64(S)
	v := (float64(py) + 0.5 + float64(dy)) / float64(S)
	x, y, z := FaceUVToDir(face, u, v)
	return DirToFacePixel(x, y, z, S)
}
