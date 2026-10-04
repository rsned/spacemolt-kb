package tectonics

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strconv"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

// Params are every knob of checkpoint 1. Archetype defaults come from
// archetypes; any field can be overridden by name with Set.
type Params struct {
	Archetype     string
	Face          int // cube face size S
	Steps         int
	KeyframeEvery int

	// Thickness field
	NoiseFreq        float64 // base octave frequency (cells across the sphere)
	NoiseOctaves     int
	WarpAmp          float64
	MaxNeighborDelta float64
	RelaxIters       int     // per-step relaxation passes
	CrustBias        float64 // added to the field before normalise: >0 thicker worlds

	// Plates
	MajorMin, MajorMax int
	MinorMin, MinorMax int
	MinPlateArea       float64 // fraction of the sphere
	// DominantMin..DominantMax: seeded target share of the sphere for the
	// largest plate (Earth keeps one plate near 0.3: Panthalassa, then the
	// Pacific). DominantSlack is how many extra plates the first merge phase
	// keeps so growing the dominant plate does not undershoot the count.
	DominantMin, DominantMax float64
	DominantSlack            int

	// Motion
	SpeedMinCmYr, SpeedMaxCmYr float64
	SpeedSkew                  float64 // exponent on the uniform draw; 1 = uniform, 2 = bottom-heavy like Earth
	TimelineMyr                float64
	RadiusKm                   float64
	RepoleEveryMyr             float64 // plates re-aim toward their thinnest seam this often
	ReaimFresh                 float64 // weight of the fresh push direction in a re-aim (0..1)
	PushJitterDeg              float64

	// Interactions
	RidgeThickness       float64
	RidgeJitter          float64
	OceanicThickening    float64
	TransformRatio       float64
	ContinentalThreshold float64
	CollisionUplift      float64
	TrenchDepth          float64
	TrenchWidth          int
	ArcUplift            float64
	ArcOffset            int
	FaultScar            float64

	// Rifting (plate birth), see docs/superpowers/specs/2026-10-03-tectonics-rifting-design.md
	RiftMinShareMin, RiftMinShareMax float64 // seeded share of the sphere a plate must exceed
	RiftRestMyrMin, RiftRestMyrMax   float64 // seeded Myr without a split before eligibility
	RiftChancePerMyr                 float64 // per-Myr probability an eligible plate rifts
	RiftMinChildShare                float64 // smaller half below this cancels the split
	RiftThinPower                    float64 // exponent on thickness in the path cost
	RiftMaxPlates                    int     // no rifts once this many plate ids exist (byte-sized ids)
	RiftRetries                      int     // extra seeded start pixels tried after a cancelled attempt
}

type archetypeRow struct {
	majorMin, majorMax, minorMin, minorMax  int
	speedMin, speedMax, timeline, crustBias float64
}

var archetypes = map[string]archetypeRow{
	"terran":       {6, 9, 10, 16, 3, 10, 800, 0},
	"super_terran": {6, 9, 10, 16, 3, 10, 800, 0},
	"oceanic":      {6, 9, 10, 16, 3, 10, 800, -0.15},
	"arid":         {4, 7, 8, 12, 2, 5, 600, 0},
	"tundra":       {4, 7, 8, 12, 2, 5, 600, 0},
	"glacial":      {4, 7, 8, 12, 2, 5, 600, 0},
	"scorched":     {8, 12, 14, 20, 6, 12, 600, -0.2},
	"lava_world":   {8, 12, 14, 20, 6, 12, 600, -0.2},
	"ice_world":    {2, 4, 2, 6, 0.2, 1, 600, 0.1},
}

var refused = map[string]bool{"jovian": true, "ice_giant": true}

// Archetypes lists the supported archetype names, sorted.
func Archetypes() []string {
	out := make([]string, 0, len(archetypes))
	for k := range archetypes {
		out = append(out, k)
	}
	slices.Sort(out)

	return out
}

// DefaultParams returns the default knob set for a given archetype, or an
// error if the archetype is unknown or has no solid crust.
func DefaultParams(archetype string) (Params, error) {
	if refused[archetype] {
		return Params{}, fmt.Errorf("archetype %q has no solid crust; tectonics not applicable", archetype)
	}
	a, ok := archetypes[archetype]
	if !ok {
		return Params{}, fmt.Errorf("unknown archetype %q", archetype)
	}

	return Params{
		Archetype: archetype, Face: 256, Steps: 400, KeyframeEvery: 4,
		NoiseFreq: 2, NoiseOctaves: 3, WarpAmp: 0.3, MaxNeighborDelta: 0.05, RelaxIters: 2, CrustBias: a.crustBias,
		MajorMin: a.majorMin, MajorMax: a.majorMax, MinorMin: a.minorMin, MinorMax: a.minorMax, MinPlateArea: 0.002,
		DominantMin: 0.20, DominantMax: 0.35, DominantSlack: 6,
		SpeedMinCmYr: a.speedMin, SpeedMaxCmYr: a.speedMax, SpeedSkew: 2, TimelineMyr: a.timeline, RadiusKm: 6371,
		RepoleEveryMyr: 25, ReaimFresh: 0.75, PushJitterDeg: 20,
		RidgeThickness: 0.15, RidgeJitter: 0.03, OceanicThickening: 0.01, TransformRatio: 2, ContinentalThreshold: 0.5,
		CollisionUplift: 0.01, TrenchDepth: 0.03, TrenchWidth: 3, ArcUplift: 0.01, ArcOffset: 4, FaultScar: 0.02,
		RiftMinShareMin: 0.08, RiftMinShareMax: 0.15, RiftRestMyrMin: 50, RiftRestMyrMax: 120,
		RiftChancePerMyr: 0.04, RiftMinChildShare: 0.03, RiftThinPower: 1, RiftMaxPlates: 200,
		RiftRetries: 3,
	}, nil
}

// MyrPerStep returns the number of million years simulated by one step.
func (p Params) MyrPerStep() float64 { return p.TimelineMyr / float64(p.Steps) }

// Set overrides one knob by exact field name ("Steps", "RidgeThickness").
func (p *Params) Set(name, value string) error {
	f := reflect.ValueOf(p).Elem().FieldByName(name)
	if !f.IsValid() {
		return fmt.Errorf("unknown knob %q", name)
	}
	switch f.Kind() {
	case reflect.Int:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("knob %s: %w", name, err)
		}
		f.SetInt(int64(n))
	case reflect.Float64:
		x, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("knob %s: %w", name, err)
		}
		f.SetFloat(x)
	case reflect.String:
		// String knobs (Archetype) identify the run; overriding one would
		// desync the bundle slug and manifest from the knobs actually used.
		return fmt.Errorf("knob %s: string knobs cannot be overridden", name)
	default:
		return fmt.Errorf("knob %s: unsupported kind %s", name, f.Kind())
	}

	return nil
}

// SeedForPlanet maps a planet's game id to the master seed.
func SeedForPlanet(id string) int64 { return seed.Hash(id) }

// newRNG returns the deterministic stream for one stage of one planet.
func newRNG(master int64, domain string) *rand.Rand {
	return rand.New(rand.NewPCG(uint64(seed.Domain(master, domain)), 0x9e3779b97f4a7c15))
}
