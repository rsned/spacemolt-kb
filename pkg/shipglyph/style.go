package shipglyph

import "hash/fnv"

// Point is a position in glyph space: X runs 0 at the nose to 1 at the tail,
// Y is a signed offset from the centerline.
type Point struct{ X, Y float64 }

// SeedOf returns a stable 64-bit FNV-1a hash of a ship ID, used to derive all
// deterministic visual variation. Matches the convention in
// cmd/generate-factions-kb/silhouette.go.
func SeedOf(id string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return h.Sum64()
}

// rng is a deterministic splitmix64 generator. It avoids math/rand so output
// cannot drift with Go version or global seeding.
type rng struct{ state uint64 }

// newRNG returns a generator seeded from s.
func newRNG(s uint64) *rng { return &rng{state: s} }

// next returns the next value in [0, 1).
func (r *rng) next() float64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return float64(z>>11) / float64(uint64(1)<<53)
}

// Style is a faction's rendering treatment. It controls only how hull control
// points are joined and decorated, never the geometry itself, so the same
// Descriptor renders in any faction's language.
type Style struct {
	// Name is the faction key this style was built for.
	Name string
	// Chamfer is the fraction of a segment cut away at each vertex, giving
	// the angular Crimson look. Zero disables chamfering.
	Chamfer float64
	// Smooth rounds the hull with a Chaikin corner-cutting pass, giving the
	// flowing Nebula and Voidborn forms. Chaikin approximates rather than
	// interpolates, so the smoothed outline sits slightly inside the raw
	// control polygon.
	Smooth bool
	// Skew is the fractional half-width deviation applied per hull panel,
	// independently to each side, producing the Outer Rim's welded-scrap
	// asymmetry. It deviates once per panel joint and runs straight in
	// between: an outline carries a ship's shape, not its surface detail, so
	// nothing here may act at per-sample frequency.
	Skew float64
	// Lobed is the amplitude of the organic bulge wave applied before
	// smoothing, as a fraction of local half-width, producing the Voidborn's
	// flowing forms. Zero disables it.
	Lobed float64
}

// StyleFor returns the rendering treatment for a faction. Unknown or empty
// factions get a neutral style so every ship renders.
func StyleFor(faction string) Style {
	switch faction {
	case "crimson":
		return Style{Name: "crimson", Chamfer: 0.22}
	case "nebula":
		return Style{Name: "nebula", Smooth: true}
	case "solarian":
		return Style{Name: "solarian", Chamfer: 0.14}
	case "outerrim":
		return Style{Name: "outerrim", Skew: 0.10}
	case "voidborn":
		return Style{Name: "voidborn", Smooth: true, Lobed: 0.18}
	case "pirate":
		return Style{Name: "pirate", Skew: 0.12, Chamfer: 0.18}
	default:
		return Style{Name: "neutral", Chamfer: 0.08}
	}
}
