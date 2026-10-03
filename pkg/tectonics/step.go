package tectonics

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Boundary feature types stamped by Step, stored in the top two bits of
// FeatureByte.
const (
	FeatNone uint8 = iota
	FeatDivergent
	FeatConvergent
	FeatTransform
)

// featAgeMax is the saturation value of a feature's age (six bits).
const featAgeMax = 63

// State is the full simulation state between steps. Every per-pixel grid,
// including the boundary features, advects with the plates.
type State struct {
	P        Params
	Th       *Grid[float64]
	Labels   *Grid[int32]
	Age      *Grid[uint16] // steps since the crust formed
	FeatType *Grid[uint8]
	FeatAge  *Grid[uint8] // steps since the feature was stamped, saturating at 63
	Plates   []Plate
	Motions  []Motion
	StepNo   int
}

// NewState wraps an initial thickness field, plate labelling and motions as
// step 0, with zero crust ages and no features.
func NewState(p Params, th *Grid[float64], labels *Grid[int32], plates []Plate, motions []Motion) *State {
	S := th.S
	return &State{P: p, Th: th, Labels: labels, Age: NewGrid[uint16](S),
		FeatType: NewGrid[uint8](S), FeatAge: NewGrid[uint8](S), Plates: plates, Motions: motions}
}

// FeatureByte packs pixel i's feature as type<<6 | min(age, 63).
func (s *State) FeatureByte(i int) uint8 {
	return s.FeatType.Cells[i]<<6 | min(s.FeatAge.Cells[i], featAgeMax)
}

// claim is one plate's bid for a destination pixel: the state it carries
// over from its source pixel.
type claim struct {
	plate int32
	th    float64
	age   uint16
	ft    uint8
	fa    uint8
}

// capCos returns, per plate, the cosine of the angular radius of its
// bounding cap around its centroid, widened by a two-pixel margin and the
// larger of the plate's nominal and applied step rotation.
func (s *State) capCos(applied []float64) []float64 {
	dirs := Dirs(s.Th.S)
	minCos := make([]float64, len(s.Plates))
	for i := range minCos {
		minCos[i] = 1
	}
	for i, l := range s.Labels.Cells {
		c := dot(dirs[i], s.Plates[l].Centroid)
		if c < minCos[l] {
			minCos[l] = c
		}
	}
	margin := 2.0 * (90.0 / float64(s.Th.S)) * math.Pi / 180
	for i, c := range minCos {
		rot := math.Max(math.Abs(s.Motions[i].DegPerStep), math.Abs(applied[i]))
		ang := math.Acos(math.Max(-1, math.Min(1, c))) + margin + rot*math.Pi/180
		minCos[i] = math.Cos(math.Min(ang, math.Pi))
	}
	return minCos
}

type trenchSeed struct {
	idx           int32
	loser, winner int32
}

// Step advances the state by one geologic step.
func (s *State) Step(rng *rand.Rand) {
	S := s.Th.S
	N := s.Th.Len()
	dirs := Dirs(S)
	nb := Neighbors4(S)
	prevTh, prevL, prevAge := s.Th, s.Labels, s.Age
	prevFT, prevFA := s.FeatType, s.FeatAge
	th, labels, age := NewGrid[float64](S), NewGrid[int32](S), NewGrid[uint16](S)
	featType, featAge := NewGrid[uint8](S), NewGrid[uint8](S)
	stamped := make([]bool, N)

	// 0. whole-pixel rotation owed this step (see Motion.Pending)
	pixelDeg := 90.0 / float64(S)
	applied := make([]float64, len(s.Plates))
	for k, pl := range s.Plates {
		if pl.Retired {
			continue
		}
		m := &s.Motions[k]
		m.Pending += m.DegPerStep
		applied[k] = math.Round(m.Pending/pixelDeg) * pixelDeg
		m.Pending -= applied[k]
	}
	capc := s.capCos(applied)

	// 1. claims
	claims := make([][]claim, N)
	for k, pl := range s.Plates {
		if pl.Retired {
			continue
		}
		m := s.Motions[k]
		for i := range N {
			if dot(dirs[i], pl.Centroid) < capc[k] {
				continue
			}
			j := i // applied 0: identity claim, no drift and no gaps
			if applied[k] != 0 {
				src := rotate(dirs[i], m.Pole, -applied[k])
				f, px, py := cubemap.DirToFacePixel(src[0], src[1], src[2], S)
				j = prevL.Index(f, px, py)
			}
			if prevL.Cells[j] == int32(k) {
				claims[i] = append(claims[i], claim{int32(k), prevTh.Cells[j], prevAge.Cells[j], prevFT.Cells[j], prevFA.Cells[j]})
			}
		}
	}

	// 2–4. resolve
	var trenches []trenchSeed
	var gaps []int32
	for i := range N {
		labels.Cells[i] = -1
		switch c := claims[i]; len(c) {
		case 0:
			gaps = append(gaps, int32(i))
		case 1:
			labels.Cells[i], th.Cells[i], age.Cells[i] = c[0].plate, c[0].th, c[0].age+1
			featType.Cells[i], featAge.Cells[i] = c[0].ft, c[0].fa
			if c[0].th < s.P.ContinentalThreshold {
				a := float64(c[0].age)
				th.Cells[i] += s.P.OceanicThickening * (math.Sqrt(a+1) - math.Sqrt(a))
			}
		default:
			sort.Slice(c, func(x, y int) bool {
				if c[x].th != c[y].th {
					return c[x].th > c[y].th
				}
				return c[x].plate < c[y].plate
			})
			a, b := c[0], c[1]
			labels.Cells[i], th.Cells[i], age.Cells[i] = a.plate, a.th, a.age+1
			featType.Cells[i], featAge.Cells[i] = a.ft, a.fa
			va := velocityAt(s.Motions[a.plate], dirs[i])
			vb := velocityAt(s.Motions[b.plate], dirs[i])
			rel := add(va, scale(vb, -1))
			n := unit(tangent(add(s.Plates[a.plate].Centroid, scale(s.Plates[b.plate].Centroid, -1)), dirs[i]))
			normal := math.Abs(dot(rel, n))
			tang := math.Sqrt(math.Max(0, dot(rel, rel)-normal*normal))
			switch {
			case tang > s.P.TransformRatio*normal:
				if a.ft != FeatTransform { // scar only when the fault first forms
					th.Cells[i] = math.Max(0, th.Cells[i]-s.P.FaultScar)
				}
				featType.Cells[i] = FeatTransform
			case a.th >= s.P.ContinentalThreshold && b.th >= s.P.ContinentalThreshold:
				featType.Cells[i] = FeatConvergent
				th.Cells[i] = math.Min(1, th.Cells[i]+s.P.CollisionUplift)
			default:
				featType.Cells[i] = FeatConvergent
				trenches = append(trenches, trenchSeed{int32(i), b.plate, a.plate})
			}
			featAge.Cells[i] = 0
			stamped[i] = true
		}
	}

	// 2. gaps: nearest labelled pixel by BFS, ridge crust
	if len(gaps) > 0 {
		queue := make([]int32, 0, N)
		for i := range N {
			if labels.Cells[i] >= 0 {
				queue = append(queue, int32(i))
			}
		}
		for len(queue) > 0 {
			i := queue[0]
			queue = queue[1:]
			for _, j := range nb[i] {
				if labels.Cells[j] < 0 {
					labels.Cells[j] = labels.Cells[i]
					queue = append(queue, j)
				}
			}
		}
		for _, i := range gaps {
			th.Cells[i] = s.P.RidgeThickness + (rng.Float64()*2-1)*s.P.RidgeJitter
			age.Cells[i] = 0
			featType.Cells[i] = FeatDivergent
			featAge.Cells[i] = 0
			stamped[i] = true
		}
	}

	// 4b. boundary shear: seams where plates slide past without overlapping
	s.shearPass(th, labels, featType, featAge, stamped)

	// 5. trench and arc
	if len(trenches) > 0 {
		s.trenchAndArc(th, labels, trenches)
	}

	// 6. ageing
	for i := range N {
		if featType.Cells[i] == FeatNone || stamped[i] {
			continue
		}
		if featAge.Cells[i] < featAgeMax {
			featAge.Cells[i]++
		} else if featType.Cells[i] != FeatConvergent {
			featType.Cells[i] = FeatNone
			featAge.Cells[i] = 0
		}
	}

	s.Th, s.Labels, s.Age = th, labels, age
	s.FeatType, s.FeatAge = featType, featAge
	s.StepNo++

	// 7. retire
	s.Plates = s.retire(PlateStats(s.Th, s.Labels, len(s.Plates)))

	// 8. smooth
	if s.P.RelaxIters > 0 {
		Relax(s.Th, s.P.MaxNeighborDelta, s.P.RelaxIters)
	}
}

// shearPass stamps FeatTransform on not-yet-stamped boundary pixels whose
// relative motion against a neighbouring live plate is mostly tangential to
// the seam. Overlap-free shear never produces a multi-claim pixel, so rule 4
// alone cannot see it.
func (s *State) shearPass(th *Grid[float64], labels *Grid[int32], featType, featAge *Grid[uint8], stamped []bool) {
	dirs := Dirs(th.S)
	nb := Neighbors4(th.S)
	for i, a := range labels.Cells {
		if stamped[i] || s.Plates[a].Retired {
			continue
		}
		for _, j := range nb[i] {
			b := labels.Cells[j]
			if b == a || s.Plates[b].Retired {
				continue
			}
			rel := add(velocityAt(s.Motions[a], dirs[i]), scale(velocityAt(s.Motions[b], dirs[i]), -1))
			n := unit(tangent(add(s.Plates[a].Centroid, scale(s.Plates[b].Centroid, -1)), dirs[i]))
			rr := dot(rel, rel)
			normal := math.Abs(dot(rel, n))
			tang := math.Sqrt(math.Max(0, rr-normal*normal))
			if rr > 1e-18 && tang > s.P.TransformRatio*normal {
				if featType.Cells[i] != FeatTransform { // carried feature: scar only on first stamping
					th.Cells[i] = math.Max(0, th.Cells[i]-s.P.FaultScar)
				}
				featType.Cells[i] = FeatTransform
				featAge.Cells[i] = 0
				stamped[i] = true
				break
			}
		}
	}
}

// trenchAndArc BFSes out from every subduction pixel: pixels of the losing
// plate within TrenchWidth steps are deepened (tapering with distance), and
// pixels of the winning plate at exactly ArcOffset steps are uplifted.
func (s *State) trenchAndArc(th *Grid[float64], labels *Grid[int32], trenches []trenchSeed) {
	nb := Neighbors4(th.S)
	dist := make([]int16, th.Len())
	for i := range dist {
		dist[i] = -1
	}
	type qi struct {
		idx           int32
		d             int16
		loser, winner int32
	}
	queue := make([]qi, 0, len(trenches))
	for _, t := range trenches {
		dist[t.idx] = 0
		queue = append(queue, qi{t.idx, 0, t.loser, t.winner})
	}
	maxD := int16(max(s.P.TrenchWidth, s.P.ArcOffset))
	for len(queue) > 0 {
		q := queue[0]
		queue = queue[1:]
		if q.d >= maxD {
			continue
		}
		for _, j := range nb[q.idx] {
			if dist[j] >= 0 {
				continue
			}
			l := labels.Cells[j]
			if l != q.loser && l != q.winner {
				continue
			}
			dist[j] = q.d + 1
			queue = append(queue, qi{j, q.d + 1, q.loser, q.winner})
			if l == q.loser && int(q.d+1) <= s.P.TrenchWidth {
				th.Cells[j] = math.Max(0, th.Cells[j]-s.P.TrenchDepth*(1-float64(q.d+1)/float64(s.P.TrenchWidth+1)))
			}
			if l == q.winner && int(q.d+1) == s.P.ArcOffset {
				th.Cells[j] = math.Min(1, th.Cells[j]+s.P.ArcUplift)
			}
		}
	}
}

// retire folds plates under MinPlateArea into their most-shared neighbour and
// carries the Retired and Major flags forward, since PlateStats rebuilds the
// plate table without them.
func (s *State) retire(stats []Plate) []Plate {
	for i := range stats {
		stats[i].Retired = stats[i].Retired || s.Plates[i].Retired
		stats[i].Major = s.Plates[i].Major
	}
	nb := Neighbors4(s.Th.S)
	for i := range stats {
		if stats[i].Retired || stats[i].Area >= s.P.MinPlateArea {
			continue
		}
		shared := map[int32]int{}
		for j, l := range s.Labels.Cells {
			if l != int32(i) {
				continue
			}
			for _, k := range nb[j] {
				if m := s.Labels.Cells[k]; m != int32(i) && !stats[m].Retired {
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
		if best < 0 {
			continue
		}
		for j, l := range s.Labels.Cells {
			if l == int32(i) {
				s.Labels.Cells[j] = best
			}
		}
		stats = PlateStats(s.Th, s.Labels, len(stats))
		for k := range stats {
			stats[k].Retired = stats[k].Retired || s.Plates[k].Retired
			stats[k].Major = s.Plates[k].Major
		}
		stats[i].Retired = true
	}
	return stats
}
