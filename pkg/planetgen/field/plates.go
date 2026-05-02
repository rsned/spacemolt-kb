// Tectonic plate seed types and Fibonacci-spiral seeding.
// Flood-fill, boundary classification, and SDFs are added in later tasks.
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
// Pixels in a planet with no plates of that boundary type get
// math.MaxFloat64.
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
// Pipeline (Phase 7 — flood-fill only; classification + SDFs in
// later commits):
//  1. seedPlates: N spiral seeds + per-plate motion + oceanic flag.
//  2. floodFillPlates: assign every pixel a plate id by random-walk frontier.
//  3. classifyAndSDF: <added in Task 5/6>.
//
// All RNG draws happen inside the named-domain seeds defined in
// pkg/planetgen/field/plates.go so adding new sub-steps in later
// phases never shifts existing field values.
func GeneratePlates(profile *types.PlanetProfile, master int64, S int) *PlateField {
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
	frontier := make([]frontierItem, 0, 6*S*S)

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
