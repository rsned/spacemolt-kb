package tectonics

import (
	"container/heap"
	"math"
	"sort"
)

type Plate struct {
	ID            int32
	Area          float64 // fraction of the sphere
	Major         bool
	MeanThickness float64
	Centroid      [3]float64 // unit vector
	Retired       bool
}

type floodItem struct {
	th    float64
	idx   int32
	label int32
}

// floodHeap pops the thickest pixel first; ties break on the lower index so
// the flood order is a pure function of the field.
type floodHeap []floodItem

func (h floodHeap) Len() int { return len(h) }
func (h floodHeap) Less(i, j int) bool {
	if h[i].th != h[j].th {
		return h[i].th > h[j].th
	}
	return h[i].idx < h[j].idx
}
func (h floodHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *floodHeap) Push(x any)   { *h = append(*h, x.(floodItem)) }
func (h *floodHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// basins priority-floods the field from every local maximum of thickness so
// each pixel joins the catchment of the thick high it drains to.
func basins(th *Grid[float64]) (*Grid[int32], int) {
	nb := Neighbors4(th.S)
	labels := NewGrid[int32](th.S)
	for i := range labels.Cells {
		labels.Cells[i] = -1
	}
	h := &floodHeap{}
	n := int32(0)
	for i := range th.Len() {
		isMax := true
		for _, j := range nb[i] {
			if th.Cells[j] > th.Cells[i] {
				isMax = false
				break
			}
		}
		if isMax {
			labels.Cells[i] = n
			heap.Push(h, floodItem{th.Cells[i], int32(i), n})
			n++
		}
	}
	for h.Len() > 0 {
		it := heap.Pop(h).(floodItem)
		for _, j := range nb[it.idx] {
			if labels.Cells[j] < 0 {
				labels.Cells[j] = it.label
				heap.Push(h, floodItem{th.Cells[j], j, it.label})
			}
		}
	}
	return labels, int(n)
}

type pairKey struct{ a, b int32 }

func key(a, b int32) pairKey {
	if a > b {
		a, b = b, a
	}
	return pairKey{a, b}
}

// find with path halving over a union-find parent slice.
func find(parent []int32, x int32) int32 {
	for parent[x] != x {
		parent[x] = parent[parent[x]]
		x = parent[x]
	}
	return x
}

// BuildPlates segments the field into major+minor plates. Micro-basins are
// merged across their thickest divides until the seeded target count is
// reached, then plates below p.MinPlateArea join their largest neighbour.
// Labels are relabelled dense, largest plate first; the first N are majors.
func BuildPlates(th *Grid[float64], p Params, master int64) (*Grid[int32], []Plate) {
	rng := newRNG(master, "plates.count")
	majors := p.MajorMin + rng.IntN(p.MajorMax-p.MajorMin+1)
	minors := p.MinorMin + rng.IntN(p.MinorMax-p.MinorMin+1)
	target := majors + minors

	labels, n := basins(th)
	nb := Neighbors4(th.S)
	divide := map[pairKey]float64{}
	for i := range th.Len() {
		for _, j := range nb[i] {
			if a, b := labels.Cells[i], labels.Cells[j]; a != b {
				k := key(a, b)
				if v := math.Max(th.Cells[i], th.Cells[j]); v > divide[k] {
					divide[k] = v
				}
			}
		}
	}
	parent := make([]int32, n)
	area := make([]float64, n) // pixel count per root; only meaningful while r is a live root
	for i := range parent {
		parent[i] = int32(i)
	}
	for _, l := range labels.Cells {
		area[l]++
	}
	count := n
	for count > target && len(divide) > 0 {
		// smallest-first absorption: merging the biggest basin into its
		// thickest neighbour every time is rich-get-richer and starves
		// minor plates, so instead find the smallest live plate (ties on
		// lower id) and fold IT into the neighbour across its thickest
		// shared divide (ties on lower neighbour id). Thin divides survive
		// longest, so boundaries still fall on thin crust.
		roots := map[int32]bool{}
		for k := range divide {
			roots[k.a] = true
			roots[k.b] = true
		}
		small := int32(-1)
		smallArea := math.Inf(1)
		for r := range roots {
			if area[r] < smallArea || (area[r] == smallArea && r < small) {
				small, smallArea = r, area[r]
			}
		}
		if small < 0 {
			break
		}
		mergeInto := int32(-1)
		bestV := -1.0
		for k, v := range divide {
			var other int32
			switch {
			case k.a == small:
				other = k.b
			case k.b == small:
				other = k.a
			default:
				continue
			}
			if v > bestV || (v == bestV && other < mergeInto) {
				mergeInto, bestV = other, v
			}
		}
		parent[small] = mergeInto
		area[mergeInto] += area[small]
		count--
		// re-key every divide that touched small onto mergeInto
		next := make(map[pairKey]float64, len(divide))
		for k, v := range divide {
			a, b := find(parent, k.a), find(parent, k.b)
			if a == b {
				continue
			}
			nk := key(a, b)
			if v > next[nk] {
				next[nk] = v
			}
		}
		divide = next
	}
	for i, l := range labels.Cells {
		labels.Cells[i] = find(parent, l)
	}
	labels, plates := compact(th, labels)
	labels, plates = mergeSmall(th, labels, plates, p.MinPlateArea)
	for i := range plates {
		plates[i].Major = i < majors && i < len(plates)
	}
	return labels, plates
}

// compact relabels to dense ids ordered by area (largest first) and returns
// the plate table.
func compact(th *Grid[float64], labels *Grid[int32]) (*Grid[int32], []Plate) {
	area := map[int32]int{}
	for _, l := range labels.Cells {
		area[l]++
	}
	ids := make([]int32, 0, len(area))
	for id := range area {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if area[ids[i]] != area[ids[j]] {
			return area[ids[i]] > area[ids[j]]
		}
		return ids[i] < ids[j]
	})
	remap := make(map[int32]int32, len(ids))
	for i, id := range ids {
		remap[id] = int32(i)
	}
	out := NewGrid[int32](labels.S)
	for i, l := range labels.Cells {
		out.Cells[i] = remap[l]
	}
	return out, PlateStats(th, out, len(ids))
}

// PlateStats recomputes Area, MeanThickness and Centroid for n plates.
func PlateStats(th *Grid[float64], labels *Grid[int32], n int) []Plate {
	plates := make([]Plate, n)
	dirs := Dirs(th.S)
	cnt := make([]int, n)
	for i, l := range labels.Cells {
		pl := &plates[l]
		cnt[l]++
		pl.MeanThickness += th.Cells[i]
		for k := range 3 {
			pl.Centroid[k] += dirs[i][k]
		}
	}
	total := float64(th.Len())
	for i := range plates {
		plates[i].ID = int32(i)
		plates[i].Area = float64(cnt[i]) / total
		if cnt[i] > 0 {
			plates[i].MeanThickness /= float64(cnt[i])
			plates[i].Centroid = unit(plates[i].Centroid)
		} else {
			plates[i].Retired = true
		}
	}
	return plates
}

// mergeSmall folds every plate under minArea into the neighbour it shares the
// most boundary with, then re-compacts.
func mergeSmall(th *Grid[float64], labels *Grid[int32], plates []Plate, minArea float64) (*Grid[int32], []Plate) {
	for {
		small := int32(-1)
		for i := len(plates) - 1; i >= 0; i-- {
			if plates[i].Area < minArea {
				small = int32(i)
				break
			}
		}
		if small < 0 || len(plates) <= 2 {
			return labels, plates
		}
		nb := Neighbors4(th.S)
		shared := map[int32]int{}
		for i, l := range labels.Cells {
			if l != small {
				continue
			}
			for _, j := range nb[i] {
				if m := labels.Cells[j]; m != small {
					shared[m]++
				}
			}
		}
		best, bestN := int32(-1), -1
		for m, c := range shared {
			if c > bestN || (c == bestN && m < best) {
				best, bestN = m, c
			}
		}
		for i, l := range labels.Cells {
			if l == small {
				labels.Cells[i] = best
			}
		}
		labels, plates = compact(th, labels)
	}
}

func unit(v [3]float64) [3]float64 {
	n := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if n == 0 {
		return [3]float64{0, 0, 1}
	}
	return [3]float64{v[0] / n, v[1] / n, v[2] / n}
}
