package cubemap

import (
	"image/color"
	"math"
)

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

// Sample returns the bilinearly-filtered RGBA at the given 3D
// direction. Coordinates are clamped to face edges; see package
// docs for seam behavior.
func (cm *CubeMap) Sample(x, y, z float64) color.RGBA {
	face, u, v := DirToFaceUV(x, y, z)
	return cm.sampleFaceUV(face, u, v)
}

func (cm *CubeMap) sampleFaceUV(face Face, u, v float64) color.RGBA {
	S := cm.Size
	fx := u*float64(S) - 0.5
	fy := v*float64(S) - 0.5
	fxFloor := math.Floor(fx)
	fyFloor := math.Floor(fy)
	x0 := clampi(int(fxFloor), 0, S-1)
	y0 := clampi(int(fyFloor), 0, S-1)
	x1 := clampi(x0+1, 0, S-1)
	y1 := clampi(y0+1, 0, S-1)
	tx := fx - fxFloor
	ty := fy - fyFloor
	c00 := cm.Get(face, x0, y0)
	c10 := cm.Get(face, x1, y0)
	c01 := cm.Get(face, x0, y1)
	c11 := cm.Get(face, x1, y1)
	return blendRGBA4(c00, c10, c01, c11, tx, ty)
}

// Sample returns the bilinearly-filtered float at the given 3D
// direction. Coordinates are clamped to face edges; see package
// docs for seam behavior.
func (cmf *CubeMapF) Sample(x, y, z float64) float64 {
	face, u, v := DirToFaceUV(x, y, z)
	return cmf.SampleFaceUV(face, u, v)
}

// SampleFaceUV bilinearly samples the given face at UV. UV components
// outside [0, 1] are clamped to the face's edge pixels. Use this when
// you need to force-sample a specific face (e.g. for cross-seam
// matched-pair tests where the natural face classification would
// snap back to the source face).
func (cmf *CubeMapF) SampleFaceUV(face Face, u, v float64) float64 {
	S := cmf.Size
	fx := u*float64(S) - 0.5
	fy := v*float64(S) - 0.5
	fxFloor := math.Floor(fx)
	fyFloor := math.Floor(fy)
	x0 := clampi(int(fxFloor), 0, S-1)
	y0 := clampi(int(fyFloor), 0, S-1)
	x1 := clampi(x0+1, 0, S-1)
	y1 := clampi(y0+1, 0, S-1)
	tx := fx - fxFloor
	ty := fy - fyFloor
	v00 := cmf.Get(face, x0, y0)
	v10 := cmf.Get(face, x1, y0)
	v01 := cmf.Get(face, x0, y1)
	v11 := cmf.Get(face, x1, y1)
	a := v00*(1-tx) + v10*tx
	b := v01*(1-tx) + v11*tx
	return a*(1-ty) + b*ty
}

// ForceFaceUV computes the UV of direction (x, y, z) in the given
// face's frame, without checking which face the direction naturally
// belongs to. Returns UV in [0, 1] if the direction lies in the
// face's hemisphere; outside that range otherwise. Inverse of
// FaceUVToDir for a fixed face.
func ForceFaceUV(face Face, x, y, z float64) (u, v float64) {
	var sc, tc, ma float64
	switch face {
	case FacePosX:
		ma = x
		sc, tc = -z, -y
	case FaceNegX:
		ma = -x
		sc, tc = z, -y
	case FacePosY:
		ma = y
		sc, tc = x, z
	case FaceNegY:
		ma = -y
		sc, tc = x, -z
	case FacePosZ:
		ma = z
		sc, tc = x, -y
	case FaceNegZ:
		ma = -z
		sc, tc = -x, -y
	}
	u = 0.5 * (sc/ma + 1)
	v = 0.5 * (tc/ma + 1)
	return u, v
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func blendRGBA4(c00, c10, c01, c11 color.RGBA, tx, ty float64) color.RGBA {
	rA := float64(c00.R)*(1-tx) + float64(c10.R)*tx
	gA := float64(c00.G)*(1-tx) + float64(c10.G)*tx
	bA := float64(c00.B)*(1-tx) + float64(c10.B)*tx
	aA := float64(c00.A)*(1-tx) + float64(c10.A)*tx
	rB := float64(c01.R)*(1-tx) + float64(c11.R)*tx
	gB := float64(c01.G)*(1-tx) + float64(c11.G)*tx
	bB := float64(c01.B)*(1-tx) + float64(c11.B)*tx
	aB := float64(c01.A)*(1-tx) + float64(c11.A)*tx
	return color.RGBA{
		R: uint8(rA*(1-ty) + rB*ty),
		G: uint8(gA*(1-ty) + gB*ty),
		B: uint8(bA*(1-ty) + bB*ty),
		A: uint8(aA*(1-ty) + aB*ty),
	}
}
