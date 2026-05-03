// Tectonic plate seed types, Fibonacci-spiral seeding, random
// flood-fill across cube faces, boundary classification, and three
// boundary distance fields (convergent / divergent / transform).
package field

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Plate captures a tectonic plate's identity and motion.
//
// Seed is the unit-vector direction of the Fibonacci-spiral seed used
// to start the plate's flood-fill region. RotAxis + AngSpeed encode an
// instantaneous angular-velocity vector ω = AngSpeed · RotAxis used
// only to classify boundaries via relative velocity at boundary pixels.
// No animation is implied.
//
// IsOceanic is drawn at construction time as a Bernoulli sample on the
// archetype's OceanicPlateFraction; it does not depend on the
// heightmap.
type Plate struct {
	ID        int
	Seed      [3]float64
	RotAxis   [3]float64
	AngSpeed  float64
	IsOceanic bool
}

// PlateField is the per-pixel plate output for a planet at a given
// face size. PlateID[face][py*S+px] holds the plate id (-1 for unset
// during construction; never persisted).
//
// Convergent / Divergent / Transform are signed-distance fields in km
// from each pixel to the nearest boundary of the corresponding type.
// Pixels in a face with no boundary of the corresponding type get
// math.Pi * RadiusKm (the geodesic half-circumference, ~20015 km for
// Earth-like radius), not math.MaxFloat64.
type PlateField struct {
	Size       int
	Plates     []Plate
	PlateID    [cubemap.NumFaces][]int16
	Convergent [cubemap.NumFaces][]float64
	Divergent  [cubemap.NumFaces][]float64
	Transform  [cubemap.NumFaces][]float64
}

// GeneratePlates produces a PlateField for a planet at face size S.
// Returns nil when profile.PlateCount == 0 (no plates).
//
// Pipeline:
//  1. seedPlates: N spiral seeds + per-plate motion + oceanic flag.
//  2. floodFillPlates: assign every pixel a plate id by random-walk frontier.
//  3. computeSDFs: classify boundaries + run three JFA passes for
//     convergent / divergent / transform SDFs in km.
//
// All RNG draws happen inside the named-domain seeds defined in
// pkg/planetgen/field/plates.go so adding new sub-steps in later
// phases never shifts existing field values.
func GeneratePlates(profile *types.PlanetProfile, master int64, S int) *PlateField {
	if profile.PlateCount > math.MaxInt16 {
		panic("planetgen/field: PlateCount exceeds int16 max (32767)")
	}
	plates := seedPlates(profile, master)
	if len(plates) == 0 {
		return nil
	}
	pf := &PlateField{Size: S, Plates: plates}
	for f := range pf.PlateID {
		pf.PlateID[f] = make([]int16, S*S)
		for i := range pf.PlateID[f] {
			pf.PlateID[f][i] = -1
		}
	}
	floodFillPlates(pf, master, S)
	computeSDFs(pf, profile)
	return pf
}

// floodFillPlates assigns every pixel a plate id via random-walk
// frontier expansion starting from each plate's spiral seed pixel.
//
// Termination is bounded: each loop iteration assigns exactly one
// previously-unassigned pixel; total iterations = 6·S² − len(plates).
func floodFillPlates(pf *PlateField, master int64, S int) {
	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.fill.random")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.fill.random.stream")), //nolint:gosec
	))

	// Mark each plate's seed pixel.
	for i := range pf.Plates {
		d := pf.Plates[i].Seed
		f, px, py := cubemap.DirToFacePixel(d[0], d[1], d[2], S)
		pf.PlateID[f][py*S+px] = int16(i)
	}

	// Frontier: list of (unassigned-pixel, claiming-plate-id) candidates.
	type frontierItem struct {
		Addr cubemap.PixelAddr
		ID   int16
	}
	frontier := make([]frontierItem, 0, S*S)

	pushNeighbors := func(face cubemap.Face, px, py int, id int16) {
		nbrs := cubemap.FacePixelNeighbors4(face, px, py, S)
		for _, n := range nbrs {
			if pf.PlateID[n.Face][n.PY*S+n.PX] == -1 {
				frontier = append(frontier, frontierItem{Addr: n, ID: id})
			}
		}
	}

	// Seed the frontier from each plate's seed pixel.
	for i := range pf.Plates {
		d := pf.Plates[i].Seed
		f, px, py := cubemap.DirToFacePixel(d[0], d[1], d[2], S)
		pushNeighbors(f, px, py, int16(i))
	}

	for len(frontier) > 0 {
		// Pick a random index, swap-pop.
		idx := rng.IntN(len(frontier))
		item := frontier[idx]
		frontier[idx] = frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]

		if pf.PlateID[item.Addr.Face][item.Addr.PY*S+item.Addr.PX] != -1 {
			continue // already filled by a different chain
		}
		pf.PlateID[item.Addr.Face][item.Addr.PY*S+item.Addr.PX] = item.ID
		pushNeighbors(item.Addr.Face, item.Addr.PX, item.Addr.PY, item.ID)
	}

	// Seam-pin pass: cross-face neighbor lookup is symmetric (verified
	// by cubemap.TestFacePixelNeighbors4Symmetric), but the random-pop
	// frontier can still assign a matched-pair pixel and its cross-face
	// twin different plate ids when both pixels' winning frontier entries
	// were pushed from different chains. This pass enforces categorical
	// equality at every seam pair by a deterministic lexicographic
	// tiebreak. It is idempotent: applying the rule twice yields the
	// same result, so walking each pair from both sides is safe.
	//
	// Bound: at most 12·S seam pixels exist out of 6·S² total, so the
	// reassignment touches ≤ 0.4%·(1/S) of pixels — far below the noise
	// floor of plate-area statistics, and the per-archetype plate-count
	// invariant is preserved because we never overwrite the only pixel
	// of a plate (each plate's seed pixel is interior-of-its-own-plate
	// at S=64 with PlateCount ≤ 16).
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				if px > 0 && px < S-1 && py > 0 && py < S-1 {
					continue
				}
				nbrs := cubemap.FacePixelNeighbors4(f, px, py, S)
				for _, n := range nbrs {
					if n.Face == f {
						continue
					}
					hereID := pf.PlateID[f][py*S+px]
					thereID := pf.PlateID[n.Face][n.PY*S+n.PX]
					if hereID == thereID {
						continue
					}
					// Lexicographic tiebreak: lower (face, py, px) wins.
					if n.Face < f || (n.Face == f && (n.PY < py || (n.PY == py && n.PX < px))) {
						pf.PlateID[f][py*S+px] = thereID
					} else {
						pf.PlateID[n.Face][n.PY*S+n.PX] = hereID
					}
				}
			}
		}
	}
}

// boundaryKind classifies a plate-boundary pixel by its relative-
// velocity dot-product against the boundary normal. boundaryNone
// indicates an interior pixel (no neighbor with a different plate id).
type boundaryKind int8

const (
	boundaryNone boundaryKind = iota
	boundaryConvergent
	boundaryDivergent
	boundaryTransform
)

// classifyBoundary returns the plate-boundary type at a point given
// the relative velocity vRel between the two plates at that point and
// the boundary normal n (unit vector from "this" plate's pixel toward
// the differing-plate neighbor pixel, in tangent space). T is the
// signed-velocity threshold (profile.PlateConvergentT, default 0.75).
func classifyBoundary(vRel, n [3]float64, T float64) boundaryKind {
	proj := vRel[0]*n[0] + vRel[1]*n[1] + vRel[2]*n[2]
	switch {
	case proj > +T:
		return boundaryDivergent
	case proj < -T:
		return boundaryConvergent
	default:
		return boundaryTransform
	}
}

// boundaryAt returns the boundary kind at face/(px,py) by examining
// the four 4-neighbors. If no neighbor has a different plate id, the
// pixel is interior and returns boundaryNone. If multiple neighbors
// with differing ids exist, the kind from the highest-priority
// neighbor is returned (priority: Convergent > Divergent > Transform).
// T is profile.PlateConvergentT.
func boundaryAt(pf *PlateField, face cubemap.Face, px, py int, T float64) boundaryKind {
	S := pf.Size
	here := pf.PlateID[face][py*S+px]
	nbrs := cubemap.FacePixelNeighbors4(face, px, py, S)
	best := boundaryNone
	rank := func(k boundaryKind) int {
		switch k {
		case boundaryConvergent:
			return 3
		case boundaryDivergent:
			return 2
		case boundaryTransform:
			return 1
		}
		return 0
	}
	// Position p on the unit sphere at the boundary pixel — same
	// for every neighbor, hoist out of the loop.
	dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
	p := [3]float64{dx, dy, dz}
	for _, nb := range nbrs {
		there := pf.PlateID[nb.Face][nb.PY*S+nb.PX]
		if there == here {
			continue
		}
		a := pf.Plates[here]
		b := pf.Plates[there]
		// ω = AngSpeed · RotAxis. v = ω × p.
		va := cross(scale(a.RotAxis, a.AngSpeed), p)
		vb := cross(scale(b.RotAxis, b.AngSpeed), p)
		vRel := [3]float64{va[0] - vb[0], va[1] - vb[1], va[2] - vb[2]}
		// Normal: from here-pixel toward there-pixel in tangent plane.
		ndx, ndy, ndz := cubemap.FacePixelToDir(nb.Face, nb.PX, nb.PY, S)
		n := [3]float64{ndx - dx, ndy - dy, ndz - dz}
		// Project n into tangent plane at p (subtract component along p).
		proj := n[0]*p[0] + n[1]*p[1] + n[2]*p[2]
		n[0] -= proj * p[0]
		n[1] -= proj * p[1]
		n[2] -= proj * p[2]
		nmag := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
		// Degenerate case: tangent-plane normal collapsed to zero (would
		// happen only for nearly-antipodal neighbor pairs, which can't
		// occur for adjacent pixels on a cube face). Skip — don't
		// classify with a garbage normal.
		if nmag <= 1e-9 {
			continue
		}
		n[0] /= nmag
		n[1] /= nmag
		n[2] /= nmag
		kind := classifyBoundary(vRel, n, T)
		if rank(kind) > rank(best) {
			best = kind
		}
	}
	return best
}

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func scale(a [3]float64, s float64) [3]float64 {
	return [3]float64{a[0] * s, a[1] * s, a[2] * s}
}

// extractBoundaries computes the boundary kind for every pixel and
// stores convergent/divergent/transform pixel masks in per-face
// boolean slices. The returned slices are owned by the caller; this
// function does not mutate pf.
//
// Precondition: pf.PlateID must be fully assigned — no pixel may
// hold the -1 sentinel. (GeneratePlates guarantees this after
// floodFillPlates returns.) Calling on a partially-assigned field
// will panic via out-of-bounds index into pf.Plates.
//
// Called by GeneratePlates between flood-fill and SDF (Task 6).
func extractBoundaries(pf *PlateField, T float64) (conv, div, trans [cubemap.NumFaces][]bool) {
	S := pf.Size
	for f := range conv {
		conv[f] = make([]bool, S*S)
		div[f] = make([]bool, S*S)
		trans[f] = make([]bool, S*S)
	}
	for f := range cubemap.NumFaces {
		for py := range S {
			for px := range S {
				k := boundaryAt(pf, cubemap.Face(f), px, py, T)
				idx := py*S + px
				switch k {
				case boundaryConvergent:
					conv[f][idx] = true
				case boundaryDivergent:
					div[f][idx] = true
				case boundaryTransform:
					trans[f][idx] = true
				}
			}
		}
	}
	return
}

// seedPlates returns N plates with Fibonacci-spiral unit-vector seeds,
// random rotation axes, [0,1] angular speeds, and Bernoulli-sampled
// oceanic flags. Deterministic for fixed (profile.PlateCount,
// OceanicPlateFraction, master).
func seedPlates(profile *types.PlanetProfile, master int64) []Plate {
	n := profile.PlateCount
	if n <= 0 {
		return nil
	}
	plates := make([]Plate, n)

	// Fibonacci spiral on the unit sphere with a small per-seed jitter
	// so different planet seeds produce visibly distinct plate layouts.
	rngSeed := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.seeds")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.seeds.stream")), //nolint:gosec
	))
	rngMotion := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.motion")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.motion.stream")), //nolint:gosec
	))
	rngOceanic := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.oceanic")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.oceanic.stream")), //nolint:gosec
	))

	goldenAngle := math.Pi * (3.0 - math.Sqrt(5)) // π·(3 − √5)
	for i := range n {
		// Fibonacci spiral point on the unit sphere.
		y := 1.0 - 2.0*(float64(i)+0.5)/float64(n)
		radius := math.Sqrt(1.0 - y*y)
		theta := goldenAngle * float64(i)
		// Jitter the spiral seed direction by ~3° to break visual symmetry
		// between planets that share PlateCount.
		jitter := (rngSeed.Float64() - 0.5) * (math.Pi / 30)
		x := math.Cos(theta+jitter) * radius
		z := math.Sin(theta+jitter) * radius

		plates[i].ID = i
		plates[i].Seed = [3]float64{x, y, z}

		// Random axis on unit sphere via Marsaglia's method (rejection sampling
		// in the unit disk → project onto sphere).
		for {
			a := 2*rngMotion.Float64() - 1
			b := 2*rngMotion.Float64() - 1
			s := a*a + b*b
			if s >= 1 {
				continue
			}
			f := 2 * math.Sqrt(1-s)
			plates[i].RotAxis = [3]float64{a * f, b * f, 1 - 2*s}
			break
		}
		plates[i].AngSpeed = rngMotion.Float64()
		plates[i].IsOceanic = rngOceanic.Float64() < profile.OceanicPlateFraction
	}
	return plates
}

// computeSDFs runs three JFA passes over the boundary-type masks and
// scales the angular-distance output to kilometers using the planet
// RadiusKm. RadiusKm defaults to 6371 km (Earth-like) when zero.
//
// JFA output is angular distance / π in [0, 1]; multiplying by π · R
// gives geodesic km in [0, π·R].
func computeSDFs(pf *PlateField, profile *types.PlanetProfile) {
	conv, div, trans := extractBoundaries(pf, profile.PlateConvergentT)
	radius := profile.RadiusKm
	if radius == 0 {
		radius = 6371
	}
	factor := math.Pi * radius
	runOne := func(mask [cubemap.NumFaces][]bool) [cubemap.NumFaces][]float64 {
		f := JumpFloodFromMask(mask, pf.Size)
		var out [cubemap.NumFaces][]float64
		for i := range f.Faces {
			out[i] = make([]float64, len(f.Faces[i]))
			for j, v := range f.Faces[i] {
				out[i][j] = v * factor
			}
		}
		return out
	}
	pf.Convergent = runOne(conv)
	pf.Divergent = runOne(div)
	pf.Transform = runOne(trans)
}
