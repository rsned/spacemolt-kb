package cubemap

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

// crossCells lists each face's (col, row) in the 4×3 cross grid.
var crossCells = [NumFaces]struct{ col, row int }{
	FacePosX: {2, 1},
	FaceNegX: {0, 1},
	FacePosY: {1, 0},
	FaceNegY: {1, 2},
	FacePosZ: {1, 1},
	FaceNegZ: {3, 1},
}

// WriteCrossPNG saves cm as a 4S × 3S horizontal-cross PNG with
// empty cells transparent.
func WriteCrossPNG(cm *CubeMap, path string) error {
	S := cm.Size
	img := image.NewRGBA(image.Rect(0, 0, 4*S, 3*S))
	// Empty cells are zero-RGBA (alpha=0) by default.
	for face := range Face(NumFaces) {
		cell := crossCells[face]
		ox, oy := cell.col*S, cell.row*S
		for py := range S {
			for px := range S {
				img.SetRGBA(ox+px, oy+py, cm.Get(face, px, py))
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

// ReadCrossPNG loads a 4×3 horizontal-cross PNG produced by
// WriteCrossPNG. The image dimensions must be 4S × 3S; face size
// is inferred from width / 4.
func ReadCrossPNG(path string) (*CubeMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w%4 != 0 || h%3 != 0 || w/4 != h/3 {
		return nil, fmt.Errorf("cross image dims %dx%d not 4S×3S", w, h)
	}
	S := w / 4
	cm := New(S)
	for face := range Face(NumFaces) {
		cell := crossCells[face]
		ox, oy := cell.col*S, cell.row*S
		for py := range S {
			for px := range S {
				r, g, bl, a := img.At(ox+px, oy+py).RGBA()
				cm.Set(face, px, py, color.RGBA{
					R: uint8(r >> 8), G: uint8(g >> 8),
					B: uint8(bl >> 8), A: uint8(a >> 8),
				})
			}
		}
	}
	return cm, nil
}
