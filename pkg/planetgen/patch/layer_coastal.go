package patch

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

// distanceToCoastPatch: two-pass chamfer distance (in pixels) to the
// nearest coastline crossing of threshold, converted to the JFA
// field's [0,1] angular-fraction units.
func distanceToCoastPatch(hm *Grid, threshold float64, pxRad float64) *Grid {
	size := hm.Size
	const inf = math.MaxFloat64
	d := NewGrid(size)
	for i, h := range hm.Data {
		ocean := h < threshold
		// Seed: pixels adjacent to the opposite domain are coast.
		d.Data[i] = inf
		ix, iy := i%size, i/size
		for _, o := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := ix+o[0], iy+o[1]
			if nx < 0 || ny < 0 || nx >= size || ny >= size {
				continue
			}
			if (hm.At(nx, ny) < threshold) != ocean {
				d.Data[i] = 0
				break
			}
		}
	}
	// Chamfer 3-4 forward/backward passes.
	relax := func(i int, ix, iy, ox, oy int, w float64) {
		nx, ny := ix+ox, iy+oy
		if nx < 0 || ny < 0 || nx >= size || ny >= size {
			return
		}
		if v := d.Data[ny*size+nx] + w; v < d.Data[i] {
			d.Data[i] = v
		}
	}
	for iy := range size {
		for ix := range size {
			i := iy*size + ix
			relax(i, ix, iy, -1, 0, 1)
			relax(i, ix, iy, 0, -1, 1)
			relax(i, ix, iy, -1, -1, math.Sqrt2)
			relax(i, ix, iy, 1, -1, math.Sqrt2)
		}
	}
	for iy := size - 1; iy >= 0; iy-- {
		for ix := size - 1; ix >= 0; ix-- {
			i := iy*size + ix
			relax(i, ix, iy, 1, 0, 1)
			relax(i, ix, iy, 0, 1, 1)
			relax(i, ix, iy, 1, 1, math.Sqrt2)
			relax(i, ix, iy, -1, 1, math.Sqrt2)
		}
	}
	// Pixels → angular fraction of π (JFA units).
	pxToFrac := pxRad / math.Pi
	for i, v := range d.Data {
		if v == inf {
			d.Data[i] = 1
		} else {
			d.Data[i] = v * pxToFrac
		}
	}
	return d
}

func coastalEnabled(ctx *Context) bool {
	return ctx.Profile.Coastal.Amp > 0 && ctx.Sphere.SeaLevel0 > 0
}

// applyCoastal (layer 5): production ApplyCoastal per pixel, with the
// patch-local chamfer standing in for the sphere JFA (spec §7).
func applyCoastal(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	p := ctx.Profile
	sea := ctx.Sphere.SeaLevel0
	dist := distanceToCoastPatch(st.Height, sea, w.PxRad())
	gen := noise.NewCoastalGen(seed.Domain(ctx.Master, "coastal"))

	ns := *st
	ns.Height = st.Height.Clone()
	ns.DistCoast = dist
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			dx, dy, dz := w.Dir(ix, iy)
			ns.Height.Data[i] = noise.ApplyCoastal(gen, dx, dy, dz,
				st.Height.Data[i], dist.Data[i], p.Coastal.Amp, p.Coastal.Threshold, p.Coastal.Freq)
		}
	}
	return &ns
}
