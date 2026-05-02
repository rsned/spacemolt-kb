// Package field — Phase 7 Tier B: Voronoi tectonic plates.
//
// GeneratePlates produces a PlateField — per-pixel plate id, per-plate
// motion + oceanic flag, and three boundary distance fields
// (convergent / divergent / transform) in km — for use by Phase 8
// consumers. Phase 7 itself only renders these as debug stages.
//
// Algorithm: Fibonacci-spiral N plate seeds → random flood-fill across
// cube faces with cross-face neighbor walk → boundary classification
// via relative-velocity dot-product → three independent JFA passes for
// the typed SDFs.
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

	const goldenAngle = math.Pi * (3.0 - 2.23606797749979) // π·(3 − √5)
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
