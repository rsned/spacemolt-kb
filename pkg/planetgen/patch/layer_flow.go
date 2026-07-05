package patch

import (
	"math"
	"sort"
)

// d8off: D8 neighbor offsets, index = direction id stored per pixel.
var d8off = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}}

// planchonDarbouxPatch fills pits on a clone with the window border as
// the outlet: border pixels keep their true height; interior pixels
// start at +inf and relax downward. eps keeps drainage strictly
// monotone.
func planchonDarbouxPatch(hm *Grid) *Grid {
	size := hm.Size
	const eps = 1e-7
	f := NewGrid(size)
	for iy := range size {
		for ix := range size {
			if ix == 0 || iy == 0 || ix == size-1 || iy == size-1 {
				f.Set(ix, iy, hm.At(ix, iy))
			} else {
				f.Set(ix, iy, math.MaxFloat64)
			}
		}
	}
	for changed := true; changed; {
		changed = false
		relax := func(ix, iy int) {
			i := iy*size + ix
			h := hm.Data[i]
			if f.Data[i] <= h {
				return
			}
			for _, o := range d8off {
				nx, ny := ix+o[0], iy+o[1]
				if nx < 0 || ny < 0 || nx >= size || ny >= size {
					continue
				}
				cand := f.At(nx, ny) + eps
				if h >= cand {
					if f.Data[i] != h {
						f.Data[i] = h
						changed = true
					}
					return
				}
				if cand < f.Data[i] {
					f.Data[i] = cand
					changed = true
				}
			}
		}
		for iy := 1; iy < size-1; iy++ {
			for ix := 1; ix < size-1; ix++ {
				relax(ix, iy)
			}
		}
		for iy := size - 2; iy >= 1; iy-- {
			for ix := size - 2; ix >= 1; ix-- {
				relax(ix, iy)
			}
		}
	}
	return f
}

func flowEnabled(ctx *Context) bool { return ctx.Profile.Flow.RiverThreshold > 0 }

// applyFlowRivers (layer 8): D8 + Planchon-Darboux with the patch
// border as drain (spec §4.5), then carve rivers.
func applyFlowRivers(ctx *Context, st *State) *State {
	size := st.Height.Size
	cfg := ctx.Profile.Flow
	filled := planchonDarbouxPatch(st.Height)

	// D8 pointers on the filled surface; border pixels drain out (-1).
	d8 := make([]int8, size*size)
	for iy := range size {
		for ix := range size {
			i := iy*size + ix
			d8[i] = -1
			if ix == 0 || iy == 0 || ix == size-1 || iy == size-1 {
				continue
			}
			best, bestDrop := int8(-1), 0.0
			for k, o := range d8off {
				nx, ny := ix+o[0], iy+o[1]
				drop := (filled.Data[i] - filled.At(nx, ny)) / math.Hypot(float64(o[0]), float64(o[1]))
				if drop > bestDrop {
					bestDrop, best = drop, int8(k)
				}
			}
			d8[i] = best
		}
	}

	// Accumulate downstream in descending fill order.
	order := make([]int, size*size)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return filled.Data[order[a]] > filled.Data[order[b]] })
	accum := NewGrid(size)
	for i := range accum.Data {
		accum.Data[i] = 1
	}
	for _, i := range order {
		k := d8[i]
		if k < 0 {
			continue // border outlet or pit
		}
		ix, iy := i%size, i/size
		j := (iy+d8off[k][1])*size + (ix + d8off[k][0])
		accum.Data[j] += accum.Data[i]
	}

	rivers := make([]bool, size*size)
	ns := *st
	ns.Height = st.Height.Clone()
	ns.Rivers = rivers
	ns.FlowAccum = accum
	for i := range rivers {
		if accum.Data[i] >= cfg.RiverThreshold {
			rivers[i] = true
			ns.Height.Data[i] -= cfg.RiverDepth
		}
	}
	return &ns
}
