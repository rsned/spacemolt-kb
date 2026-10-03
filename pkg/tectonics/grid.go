// Package tectonics generates tectonic plates from a smooth crust-thickness
// field on a cube-sphere and drifts them over geologic time. It is the
// first stage of the tectonics-first planet generator (see
// docs/superpowers/specs/2026-10-02-tectonics-first-planet-gen-design.md).
package tectonics

import (
	"math"
	"sync"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Grid stores one value per cube-sphere pixel, all six faces flattened so a
// single int addresses any pixel: i = face*S*S + py*S + px.
type Grid[T any] struct {
	S     int
	Cells []T
}

// NewGrid allocates a Grid with face size S, zero-valued.
func NewGrid[T any](S int) *Grid[T] {
	return &Grid[T]{S: S, Cells: make([]T, cubemap.NumFaces*S*S)}
}

// Len returns the total number of pixels across all faces.
func (g *Grid[T]) Len() int { return len(g.Cells) }

// Index converts a (face, px, py) address to a flat pixel index.
func (g *Grid[T]) Index(face cubemap.Face, px, py int) int {
	return int(face)*g.S*g.S + py*g.S + px
}

// Addr converts a flat pixel index back to a (face, px, py) address.
func (g *Grid[T]) Addr(i int) (cubemap.Face, int, int) {
	ff := g.S * g.S
	face := cubemap.Face(i / ff)
	r := i % ff

	return face, r % g.S, r / g.S
}

// Get returns the value at flat pixel index i.
func (g *Grid[T]) Get(i int) T { return g.Cells[i] }

// Set stores v at flat pixel index i.
func (g *Grid[T]) Set(i int, v T) { g.Cells[i] = v }

// Clone returns a deep copy of the grid.
func (g *Grid[T]) Clone() *Grid[T] {
	out := &Grid[T]{S: g.S, Cells: make([]T, len(g.Cells))}
	copy(out.Cells, g.Cells)

	return out
}

var (
	nbMu    sync.Mutex
	nbCache = map[int][][4]int32{}
	dirMu   sync.Mutex
	dirCach = map[int][][3]float64{}
)

// Neighbors4 returns, for every flat pixel index, the flat indices of its
// four seam-aware neighbours. Cached per face size.
func Neighbors4(S int) [][4]int32 {
	nbMu.Lock()
	defer nbMu.Unlock()
	if nb, ok := nbCache[S]; ok {
		return nb
	}
	g := NewGrid[struct{}](S)
	nb := make([][4]int32, g.Len())
	for i := range g.Len() {
		f, px, py := g.Addr(i)
		for k, a := range cubemap.FacePixelNeighbors4(f, px, py, S) {
			nb[i][k] = int32(g.Index(a.Face, a.PX, a.PY))
		}
	}
	nbCache[S] = nb

	return nb
}

// Dirs returns the unit direction of every pixel centre. Cached per face size.
func Dirs(S int) [][3]float64 {
	dirMu.Lock()
	defer dirMu.Unlock()
	if d, ok := dirCach[S]; ok {
		return d
	}
	g := NewGrid[struct{}](S)
	d := make([][3]float64, g.Len())
	for i := range g.Len() {
		f, px, py := g.Addr(i)
		x, y, z := cubemap.FacePixelToDir(f, px, py, S)
		n := math.Sqrt(x*x + y*y + z*z)
		d[i] = [3]float64{x / n, y / n, z / n}
	}
	dirCach[S] = d

	return d
}
