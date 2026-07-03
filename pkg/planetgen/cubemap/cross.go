package cubemap

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
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
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return WriteCrossPNGTo(cm, f)
}

// WriteCrossPNGTo encodes cm as a 4S × 3S horizontal-cross PNG to w.
// Same format as WriteCrossPNG; convenient for callers that don't have
// a filesystem (e.g. wasm).
func WriteCrossPNGTo(cm *CubeMap, w io.Writer) error {
	S := cm.Size
	img := image.NewRGBA(image.Rect(0, 0, 4*S, 3*S))
	for face := range Face(NumFaces) {
		cell := crossCells[face]
		ox, oy := cell.col*S, cell.row*S
		for py := range S {
			for px := range S {
				img.SetRGBA(ox+px, oy+py, cm.Get(face, px, py))
			}
		}
	}
	return png.Encode(w, img)
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
	return ReadCrossPNGFrom(f)
}

// ReadCrossPNGFrom is the io.Reader-driven counterpart of ReadCrossPNG.
func ReadCrossPNGFrom(r io.Reader) (*CubeMap, error) {
	img, err := png.Decode(r)
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
				r2, g, bl, a := img.At(ox+px, oy+py).RGBA()
				cm.Set(face, px, py, color.RGBA{
					R: uint8(r2 >> 8), G: uint8(g >> 8),
					B: uint8(bl >> 8), A: uint8(a >> 8),
				})
			}
		}
	}
	return cm, nil
}
