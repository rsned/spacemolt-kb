package cubemap

import "image/color"

// Face identifies one of the six faces of a cube map. Order matches
// the OpenGL GL_TEXTURE_CUBE_MAP_POSITIVE_X family so PNGs written
// by this package can be uploaded to a WebGL cube texture without
// remapping.
type Face int

const (
	FacePosX Face = iota // +X (right)
	FaceNegX             // -X (left)
	FacePosY             // +Y (top)
	FaceNegY             // -Y (bottom)
	FacePosZ             // +Z (front)
	FaceNegZ             // -Z (back)
)

// NumFaces is the number of faces in a cube map.
const NumFaces = 6

// CubeMap stores an RGBA value per pixel across six square faces.
// Each face is a row-major Size×Size grid; total storage is
// 6 * Size * Size pixels.
type CubeMap struct {
	Size  int
	Faces [NumFaces][]color.RGBA
}

// New allocates a CubeMap with all faces initialised to zero RGBA.
func New(size int) *CubeMap {
	cm := &CubeMap{Size: size}
	for i := range cm.Faces {
		cm.Faces[i] = make([]color.RGBA, size*size)
	}
	return cm
}

// Set writes a single pixel on a face. (px, py) origin is the top-
// left of the face cell.
func (cm *CubeMap) Set(face Face, px, py int, c color.RGBA) {
	cm.Faces[face][py*cm.Size+px] = c
}

// Get reads a single pixel on a face.
func (cm *CubeMap) Get(face Face, px, py int) color.RGBA {
	return cm.Faces[face][py*cm.Size+px]
}

// CubeMapF is the float64 equivalent of CubeMap, used for scalar
// fields like heightmaps, temperature, moisture, and distance
// transforms.
type CubeMapF struct {
	Size  int
	Faces [NumFaces][]float64
}

// NewF allocates a CubeMapF with all faces zeroed.
func NewF(size int) *CubeMapF {
	cm := &CubeMapF{Size: size}
	for i := range cm.Faces {
		cm.Faces[i] = make([]float64, size*size)
	}
	return cm
}

// Set writes a single float on a face.
func (cm *CubeMapF) Set(face Face, px, py int, v float64) {
	cm.Faces[face][py*cm.Size+px] = v
}

// Get reads a single float on a face.
func (cm *CubeMapF) Get(face Face, px, py int) float64 {
	return cm.Faces[face][py*cm.Size+px]
}
