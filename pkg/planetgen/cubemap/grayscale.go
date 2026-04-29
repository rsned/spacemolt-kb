package cubemap

import "image/color"

// GrayscaleFromF returns a CubeMap whose pixels are grayscale renderings
// of the corresponding scalar values in cmF. Values are clamped to [0, 1]
// before scaling to [0, 255].
func GrayscaleFromF(cmF *CubeMapF) *CubeMap {
	out := New(cmF.Size)
	for face := range Face(NumFaces) {
		for py := range cmF.Size {
			for px := range cmF.Size {
				v := cmF.Get(face, px, py)
				if v < 0 {
					v = 0
				}
				if v > 1 {
					v = 1
				}
				g := uint8(v * 255)
				out.Set(face, px, py, color.RGBA{R: g, G: g, B: g, A: 255})
			}
		}
	}
	return out
}
