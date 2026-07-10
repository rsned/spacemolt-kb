package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestWindowDirMatchesFacePixelToDir(t *testing.T) {
	w := Window{Face: cubemap.FacePosZ, X0: 256, Y0: 128, Size: 512, SProd: 1024}
	for _, p := range [][2]int{{0, 0}, {511, 511}, {17, 300}} {
		gx, gy, gz := w.Dir(p[0], p[1])
		ex, ey, ez := cubemap.FacePixelToDir(w.Face, w.X0+p[0], w.Y0+p[1], w.SProd)
		if gx != ex || gy != ey || gz != ez {
			t.Fatalf("Dir(%d,%d) = (%v,%v,%v), want (%v,%v,%v)", p[0], p[1], gx, gy, gz, ex, ey, ez)
		}
	}
}

func TestWindowValid(t *testing.T) {
	ok := Window{Face: cubemap.FacePosX, X0: 0, Y0: 0, Size: 512, SProd: 1024}
	if err := ok.Valid(); err != nil {
		t.Fatalf("valid window rejected: %v", err)
	}
	bad := []Window{
		{Face: cubemap.FacePosX, X0: 600, Y0: 0, Size: 512, SProd: 1024}, // overflows face
		{Face: cubemap.FacePosX, X0: -1, Y0: 0, Size: 512, SProd: 1024},
		{Face: cubemap.FacePosX, X0: 0, Y0: 0, Size: 0, SProd: 1024},
		{Face: cubemap.Face(9), X0: 0, Y0: 0, Size: 64, SProd: 1024},
	}
	for i, w := range bad {
		if err := w.Valid(); err == nil {
			t.Fatalf("bad window %d accepted", i)
		}
	}
}

func TestWindowSamplerRoundTrip(t *testing.T) {
	// At exact pixel centers the sampler must return the grid value exactly.
	w := Window{Face: cubemap.FaceNegY, X0: 100, Y0: 40, Size: 64, SProd: 256}
	g := NewGrid(64)
	for iy := range 64 {
		for ix := range 64 {
			g.Set(ix, iy, float64(iy*64+ix))
		}
	}
	s := w.Sampler(g)
	for _, p := range [][2]int{{0, 0}, {63, 63}, {5, 40}} {
		x, y, z := w.Dir(p[0], p[1])
		got := s(x, y, z)
		want := g.At(p[0], p[1])
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("sampler at pixel (%d,%d): got %v want %v", p[0], p[1], got, want)
		}
	}
}
