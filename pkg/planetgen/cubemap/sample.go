package cubemap

import "math"

// DirToFaceUV maps a 3D direction to its cube-map face plus UV
// coordinates in [0, 1]. The input need not be unit-length; only
// the ratios of components matter. Convention matches OpenGL
// GL_TEXTURE_CUBE_MAP.
func DirToFaceUV(x, y, z float64) (face Face, u, v float64) {
	ax, ay, az := math.Abs(x), math.Abs(y), math.Abs(z)
	var sc, tc, ma float64
	switch {
	case ax >= ay && ax >= az:
		ma = ax
		if x >= 0 {
			face = FacePosX
			sc, tc = -z, -y
		} else {
			face = FaceNegX
			sc, tc = z, -y
		}
	case ay >= az:
		ma = ay
		if y >= 0 {
			face = FacePosY
			sc, tc = x, z
		} else {
			face = FaceNegY
			sc, tc = x, -z
		}
	default:
		ma = az
		if z >= 0 {
			face = FacePosZ
			sc, tc = x, -y
		} else {
			face = FaceNegZ
			sc, tc = -x, -y
		}
	}
	u = 0.5 * (sc/ma + 1)
	v = 0.5 * (tc/ma + 1)
	return face, u, v
}

// FaceUVToDir is the inverse of DirToFaceUV. Returns a unit vector.
func FaceUVToDir(face Face, u, v float64) (x, y, z float64) {
	sc := 2*u - 1
	tc := 2*v - 1
	switch face {
	case FacePosX:
		x, y, z = 1, -tc, -sc
	case FaceNegX:
		x, y, z = -1, -tc, sc
	case FacePosY:
		x, y, z = sc, 1, tc
	case FaceNegY:
		x, y, z = sc, -1, -tc
	case FacePosZ:
		x, y, z = sc, -tc, 1
	case FaceNegZ:
		x, y, z = -sc, -tc, -1
	}
	inv := 1.0 / math.Sqrt(x*x+y*y+z*z)
	return x * inv, y * inv, z * inv
}

// FacePixelToDir returns the unit vector pointing at the center of
// pixel (px, py) on a face of side S.
func FacePixelToDir(face Face, px, py, S int) (x, y, z float64) {
	u := (float64(px) + 0.5) / float64(S)
	v := (float64(py) + 0.5) / float64(S)
	return FaceUVToDir(face, u, v)
}
