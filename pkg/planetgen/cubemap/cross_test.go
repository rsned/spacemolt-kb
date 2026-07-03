package cubemap

import (
	"image/color"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossRoundtrip(t *testing.T) {
	const S = 32
	cm := New(S)
	rng := rand.New(rand.NewPCG(42, 99))
	for face := range Face(NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = color.RGBA{
				R: uint8(rng.UintN(256)),
				G: uint8(rng.UintN(256)),
				B: uint8(rng.UintN(256)),
				A: 255,
			}
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "cross.png")
	if err := WriteCrossPNG(cm, path); err != nil {
		t.Fatalf("WriteCrossPNG: %v", err)
	}
	cm2, err := ReadCrossPNG(path)
	if err != nil {
		t.Fatalf("ReadCrossPNG: %v", err)
	}
	if cm2.Size != cm.Size {
		t.Fatalf("size mismatch: got %d want %d", cm2.Size, cm.Size)
	}
	for face := range Face(NumFaces) {
		for i := range cm.Faces[face] {
			if cm.Faces[face][i] != cm2.Faces[face][i] {
				t.Fatalf("face %d pixel %d: %v vs %v",
					face, i, cm.Faces[face][i], cm2.Faces[face][i])
			}
		}
	}
}

func TestCrossImageSize(t *testing.T) {
	const S = 16
	cm := New(S)
	dir := t.TempDir()
	path := filepath.Join(dir, "cross.png")
	if err := WriteCrossPNG(cm, path); err != nil {
		t.Fatalf("WriteCrossPNG: %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Size() == 0 {
		t.Fatalf("output file missing or empty")
	}
	// Also re-decode to check dimensions.
	cm2, err := ReadCrossPNG(path)
	if err != nil {
		t.Fatalf("ReadCrossPNG: %v", err)
	}
	if cm2.Size != S {
		t.Fatalf("decoded Size = %d, want %d", cm2.Size, S)
	}
}
