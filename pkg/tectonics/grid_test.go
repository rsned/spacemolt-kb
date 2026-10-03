// pkg/tectonics/grid_test.go
package tectonics

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestGridIndexRoundTrip(t *testing.T) {
	g := NewGrid[int32](8)
	if g.Len() != 6*64 {
		t.Fatalf("len %d", g.Len())
	}
	for i := range g.Len() {
		f, px, py := g.Addr(i)
		if j := g.Index(f, px, py); j != i {
			t.Fatalf("index %d -> (%d,%d,%d) -> %d", i, f, px, py, j)
		}
	}
}

func TestNeighbors4MatchCubemapAndAreSymmetric(t *testing.T) {
	const S = 8
	nb := Neighbors4(S)
	g := NewGrid[int32](S)
	for i := range g.Len() {
		f, px, py := g.Addr(i)
		want := cubemap.FacePixelNeighbors4(f, px, py, S)
		for k := range 4 {
			if int(nb[i][k]) != g.Index(want[k].Face, want[k].PX, want[k].PY) {
				t.Fatalf("pixel %d neighbor %d mismatch", i, k)
			}
			// symmetry: i must appear among its neighbour's neighbours
			found := false
			for _, back := range nb[nb[i][k]] {
				if int(back) == i {
					found = true
				}
			}
			if !found {
				t.Fatalf("pixel %d not a neighbour of its neighbour %d", i, nb[i][k])
			}
		}
	}
}

func TestDirsAreUnit(t *testing.T) {
	for _, d := range Dirs(4) {
		if n := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2]); math.Abs(n-1) > 1e-9 {
			t.Fatalf("dir %v has length %g", d, n)
		}
	}
}
