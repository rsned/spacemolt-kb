package tectonics

import (
	"container/heap"
	"math"
	"math/rand/v2"
)

// riftParams are the per-planet seeded eligibility thresholds: the share of
// the sphere a plate must exceed and the Myr it must go without a split.
type riftParams struct {
	share, rest float64
}

// drawRiftParams seeds the eligibility thresholds for one planet.
func drawRiftParams(p Params, master int64) riftParams {
	rng := newRNG(master, "rift.params")
	return riftParams{
		share: p.RiftMinShareMin + rng.Float64()*(p.RiftMinShareMax-p.RiftMinShareMin),
		rest:  p.RiftRestMyrMin + rng.Float64()*(p.RiftRestMyrMax-p.RiftRestMyrMin),
	}
}

// boundaryPixels lists plate id's pixels that touch another plate, in index order.
func (s *State) boundaryPixels(id int32) []int32 {
	return s.boundaryPixelsIn(id, nil)
}

// boundaryPixelsIn lists the pixels of plate id that touch another plate, in
// index order, restricted to inSet when it is non-nil.
func (s *State) boundaryPixelsIn(id int32, inSet []bool) []int32 {
	nb := Neighbors4(s.Th.S)
	var out []int32
	for i, l := range s.Labels.Cells {
		if l != id || (inSet != nil && !inSet[i]) {
			continue
		}
		for _, j := range nb[i] {
			if s.Labels.Cells[j] != id {
				out = append(out, int32(i))
				break
			}
		}
	}
	return out
}

// labelComponents labels the 4-connected components of the pixels in inSet,
// skipping any pixel in exclude (nil excludes nothing). Components are
// numbered in order of their lowest pixel index and filled breadth-first, so
// the labelling is deterministic. comp is -1 outside the labelled set.
func labelComponents(S int, inSet, exclude []bool) (comp []int32, sizes []int) {
	nb := Neighbors4(S)
	comp = make([]int32, len(inSet))
	for i := range comp {
		comp[i] = -1
	}
	keep := func(i int32) bool { return inSet[i] && (exclude == nil || !exclude[i]) }
	queue := make([]int32, 0, len(inSet))
	for i := range inSet {
		if !keep(int32(i)) || comp[i] >= 0 {
			continue
		}
		c := int32(len(sizes))
		sizes = append(sizes, 0)
		queue = append(queue[:0], int32(i))
		comp[i] = c
		for len(queue) > 0 {
			u := queue[0]
			queue = queue[1:]
			sizes[c]++
			for _, j := range nb[u] {
				if keep(j) && comp[j] < 0 {
					comp[j] = c
					queue = append(queue, j)
				}
			}
		}
	}
	return comp, sizes
}

// componentOf marks the 4-connected component of plate id that contains start.
func (s *State) componentOf(id, start int32) []bool {
	mask := make([]bool, s.Th.Len())
	for i, l := range s.Labels.Cells {
		mask[i] = l == id
	}
	comp, _ := labelComponents(s.Th.S, mask, nil)
	in := make([]bool, len(mask))
	for i, c := range comp {
		in[i] = c >= 0 && c == comp[start]
	}
	return in
}

type pathItem struct {
	cost float64
	idx  int32
}

type pathHeap []pathItem

func (h pathHeap) Len() int { return len(h) }
func (h pathHeap) Less(i, j int) bool {
	if h[i].cost != h[j].cost {
		return h[i].cost < h[j].cost
	}
	return h[i].idx < h[j].idx
}
func (h pathHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *pathHeap) Push(x any)   { *h = append(*h, x.(pathItem)) }
func (h *pathHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// riftPath is the cheapest 4-connected path from start to end through the
// pixels of inSet (one connected component of a plate), where entering a pixel costs thickness^RiftThinPower (plus a
// tiny constant so the path still prefers short routes through equal crust).
// It returns nil when end is unreachable.
func (s *State) riftPath(inSet []bool, start, end int32) []int32 {
	nb := Neighbors4(s.Th.S)
	N := s.Th.Len()
	dist := make([]float64, N)
	prev := make([]int32, N)
	for i := range dist {
		dist[i] = math.Inf(1)
		prev[i] = -1
	}
	dist[start] = 0
	h := &pathHeap{{0, start}}
	for h.Len() > 0 {
		it := heap.Pop(h).(pathItem)
		if it.cost > dist[it.idx] {
			continue
		}
		if it.idx == end {
			break
		}
		for _, j := range nb[it.idx] {
			if !inSet[j] {
				continue
			}
			c := it.cost + math.Pow(s.Th.Cells[j], s.P.RiftThinPower) + 1e-6
			if c < dist[j] {
				dist[j], prev[j] = c, it.idx
				heap.Push(h, pathItem{c, j})
			}
		}
	}
	if math.IsInf(dist[end], 1) {
		return nil
	}
	var path []int32
	for i := end; i >= 0; i = prev[i] {
		path = append(path, i)
	}
	return path
}

// riftPlate splits plate id along the thinnest path from the boundary pixel
// start to the boundary pixel farthest from it, all within the 4-connected
// component of id that contains start; the plate's other fragments are left
// untouched with the parent. The largest piece of that component keeps id;
// every other piece of it becomes the new child plate. It returns false,
// changing nothing and drawing no random numbers, when the path fails or the
// child would be smaller than RiftMinChildShare.
func (s *State) riftPlate(id int32, start int32, rng *rand.Rand) bool {
	S := s.Th.S
	N := s.Th.Len()
	dirs := Dirs(S)
	nb := Neighbors4(S)
	inComp := s.componentOf(id, start)
	bounds := s.boundaryPixelsIn(id, inComp)
	if len(bounds) < 2 {
		return false
	}
	end, endDot := int32(-1), 2.0
	for _, b := range bounds {
		if d := dot(dirs[start], dirs[b]); d < endDot {
			end, endDot = b, d
		}
	}
	path := s.riftPath(inComp, start, end)
	if path == nil {
		return false
	}
	onPath := make([]bool, N)
	for _, i := range path {
		onPath[i] = true
	}
	// connected components of the start's component minus the path
	comp, sizes := labelComponents(S, inComp, onPath)
	if len(sizes) < 2 {
		return false
	}
	largest := 0
	for c, n := range sizes {
		if n > sizes[largest] {
			largest = c
		}
	}
	childPixels := 0
	for c, n := range sizes {
		if c != largest {
			childPixels += n
		}
	}
	if float64(childPixels)/float64(N) < s.P.RiftMinChildShare {
		return false
	}
	// commit: relabel, then give path pixels to the nearest labelled side
	child := int32(len(s.Plates))
	for i := range N {
		if comp[i] >= 0 && comp[i] != int32(largest) {
			s.Labels.Cells[i] = child
		}
	}
	queue := make([]int32, 0, N)
	for i := range N {
		if comp[i] >= 0 {
			queue = append(queue, int32(i))
		}
	}
	assigned := make([]bool, N)
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, j := range nb[u] {
			if onPath[j] && !assigned[j] {
				assigned[j] = true
				s.Labels.Cells[j] = s.Labels.Cells[u]
				queue = append(queue, j)
			}
		}
	}
	for _, i := range path {
		s.Th.Cells[i] = s.P.RidgeThickness + (rng.Float64()*2-1)*s.P.RidgeJitter
		s.Age.Cells[i] = 0
		s.FeatType.Cells[i] = FeatDivergent
		s.FeatAge.Cells[i] = 0
	}
	// tables
	stats := PlateStats(s.Th, s.Labels, len(s.Plates)+1)
	s.carryPlateFlags(stats)
	stats[child].Born, stats[child].LastRift = s.StepNo, s.StepNo
	stats[id].LastRift = s.StepNo
	s.Plates = stats
	c := s.Plates[child]
	t := pushDirection(s.Th, s.Labels, child, c.Centroid)
	if dot(t, t) == 0 {
		t = unit(tangent([3]float64{0.3, 0.5, 0.8}, c.Centroid))
	}
	t = rotate(t, c.Centroid, (rng.Float64()*2-1)*s.P.PushJitterDeg)
	speed := drawSpeed(rng, s.P)
	s.Motions = append(s.Motions, Motion{Pole: poleFor(c.Centroid, t), SpeedCmYr: speed,
		DegPerStep: DegPerStep(speed, s.P.RadiusKm, s.P.MyrPerStep())})
	reaimPlate(s.Th, s.Labels, s.Plates[id], &s.Motions[id], s.P, rng)
	return true
}

// Rift lets at most one eligible plate split this step: eligible means live,
// above rp.share of the sphere, rp.rest Myr since its last split, and fewer
// than RiftMaxPlates ids in use. Each eligible plate, in id order, rolls
// against RiftChancePerMyr scaled to the step length; the first to pass rifts
// at a seeded boundary pixel. A cancelled attempt (no path, or an undersized
// child) draws a fresh start pixel and tries again, up to RiftRetries extra
// times; the first success ends the step, and if every attempt cancels no
// other plate is tried this step. The retries only add rng.IntN draws on the
// rift stream, so the run still depends only on the seed.
func (s *State) Rift(rng *rand.Rand, rp riftParams) bool {
	if len(s.Plates) >= s.P.RiftMaxPlates {
		return false
	}
	perStep := 1 - math.Pow(1-s.P.RiftChancePerMyr, s.P.MyrPerStep())
	for k, pl := range s.Plates {
		if pl.Retired || pl.Area < rp.share || float64(s.StepNo-pl.LastRift)*s.P.MyrPerStep() < rp.rest {
			continue
		}
		if rng.Float64() >= perStep {
			continue
		}
		bounds := s.boundaryPixels(int32(k))
		if len(bounds) < 2 {
			continue
		}
		for range s.P.RiftRetries + 1 {
			if s.riftPlate(int32(k), bounds[rng.IntN(len(bounds))], rng) {
				return true
			}
		}
		return false
	}
	return false
}
