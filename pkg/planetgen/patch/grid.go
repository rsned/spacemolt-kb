// Package patch implements the Phase 13 Patch Lab: sphere-global
// tectonics computed at modest resolution, cropped to a flat window of
// one cube face at virtual production resolution, with all downstream
// layers re-run per-pixel on the window.
package patch

import "math"

// Grid is a Size×Size float raster, row-major — the patch analog of
// cubemap.CubeMapF for a single window.
type Grid struct {
	Size int
	Data []float64
}

func NewGrid(size int) *Grid {
	return &Grid{Size: size, Data: make([]float64, size*size)}
}

func (g *Grid) At(ix, iy int) float64     { return g.Data[iy*g.Size+ix] }
func (g *Grid) Set(ix, iy int, v float64) { g.Data[iy*g.Size+ix] = v }

func (g *Grid) Clone() *Grid {
	c := NewGrid(g.Size)
	copy(c.Data, g.Data)
	return c
}

// Bilinear samples at fractional pixel coordinates (pixel centers at
// integer coords), clamping to the window border. The border clamp is
// the patch edge policy: outside-window reads see the edge value.
func (g *Grid) Bilinear(x, y float64) float64 {
	max := float64(g.Size - 1)
	x = min(max, math.Max(0, x))
	y = min(max, math.Max(0, y))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, g.Size-1), min(y0+1, g.Size-1)
	fx, fy := x-float64(x0), y-float64(y0)
	top := g.At(x0, y0)*(1-fx) + g.At(x1, y0)*fx
	bot := g.At(x0, y1)*(1-fx) + g.At(x1, y1)*fx
	return top*(1-fy) + bot*fy
}
