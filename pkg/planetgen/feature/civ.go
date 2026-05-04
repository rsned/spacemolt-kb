// Package feature: civilization stage. This file owns the population-
// assignment, render-orchestration, and civ-specific glue logic for
// the Phase 9b civilization-overlay pipeline. The concrete renderable
// artifacts (habitability scalar field, Bridson sites, road network,
// city/agriculture/night-light passes) live in sibling files within
// this package; civ.go is the top-level coordinator.
package feature

import (
	"image/color"
	"math"
	"math/rand/v2"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// zipfAlpha is the Zipf exponent applied to ranked civilization sites.
// 1.07 is the typical empirical value for real-world city-size
// distributions (Gabaix 1999, "Zipf's Law for Cities").
const zipfAlpha = 1.07

// AssignPopulations sorts sites by habitability descending and writes
// Zipfian populations to each site (mutating the slice in place).
// population[rank] = cfg.Tier * (rank+1)^(-zipfAlpha), capped at
// cfg.MaxPopulation when MaxPopulation > 0.
//
// When sites is empty or cfg.Tier <= 0, AssignPopulations is a no-op
// (matching the "Tier == 0 disables civ" idiom).
func AssignPopulations(sites []Site, cfg types.CivConfig) {
	if len(sites) == 0 || cfg.Tier <= 0 {
		return
	}

	// Stable sort so habitability ties resolve by original-slice
	// order — required for deterministic outputs given identical
	// inputs.
	sort.SliceStable(sites, func(i, j int) bool {
		return sites[i].Habitability > sites[j].Habitability
	})

	for i := range sites {
		pop := cfg.Tier * math.Pow(float64(i+1), -zipfAlpha)
		if cfg.MaxPopulation > 0 && pop > cfg.MaxPopulation {
			pop = cfg.MaxPopulation
		}
		sites[i].Population = pop
	}
}

// CivField is the rendered artifact bundle produced by GenerateCiv. It
// carries everything the colorize pipeline needs to overlay civ signs
// onto the daytime image and to ship a separate Black Marble nightside
// cube-map.
//
// Layout:
//   - DayMask is per-pixel "how strongly should civ blend over the
//     biome color here" (0..1+, but typically <= 1). Faces are flat
//     row-major Size*Size slices, matching cubemap.CubeMapF layout.
//   - DayColor is the pre-blended overlay color (city beige +
//     agriculture green-yellow + darker road beige) ready to be alpha-
//     blended into the daytime planet color using DayMask as the alpha.
//   - Roads is a copy of the upstream RoadField.Intensity buffer so
//     downstream consumers (e.g. the slider tool) can sample it
//     without re-running the road pipeline.
//   - Night is the standalone Black Marble nightside RGBA, additively
//     accumulated as Gaussian splats per-site.
type CivField struct {
	Size         int
	Sites        []Site
	Habitability *HabitabilityField
	DayMask      [cubemap.NumFaces][]float64
	DayColor     *cubemap.CubeMap
	Roads        [cubemap.NumFaces][]float64
	Night        *cubemap.CubeMap
}

// Civ overlay color constants. Tuned for stylized readability rather
// than physical realism — at face=64 these read as "lit settled area"
// against the biome palette without dominating it.
var (
	civCityBeige = color.RGBA{R: 178, G: 153, B: 127, A: 255} // ~(0.7, 0.6, 0.5)
	civAgriGreen = color.RGBA{R: 160, G: 178, B: 120, A: 255} // green-yellow farmland
	civRoadBeige = color.RGBA{R: 140, G: 120, B: 95, A: 255}  // darker than city beige
	civNightWarm = color.RGBA{R: 255, G: 200, B: 120, A: 255} // golden-orange Black-Marble splat
)

// civCityRadiusPx returns the city-patch radius in pixels for site
// population pop at face size S. Calibration: at S=64 a unit-population
// site produces ~6 px (the plan's "30 km of patch" eyeballed for the
// rocky archetype). Capped at S/8 to avoid pathological coverage.
func civCityRadiusPx(pop float64, S int) int {
	if pop <= 0 {
		return 0
	}
	r := 6.0 * float64(S) / 64.0 * math.Sqrt(pop)
	// Guard against degenerate sites covering more than 1/8 of a face.
	maxR := float64(S) / 8.0
	if r > maxR {
		r = maxR
	}
	if r < 1 {
		// Sub-pixel sites still get a single-pixel mark so very small
		// populations remain visible at low S.
		return 1
	}
	return int(math.Round(r))
}

// civNightSplatPx returns the Black-Marble splat radius in pixels for
// site population pop at face size S. Slightly wider than the day
// patch so glow bleed past the city boundary.
func civNightSplatPx(pop float64, S int) int {
	if pop <= 0 {
		return 0
	}
	r := 5.0 * float64(S) / 64.0 * math.Sqrt(pop)
	cap := float64(S) / 6.0
	if r > cap {
		r = cap
	}
	if r < 1 {
		return 1
	}
	return int(math.Round(r))
}

// blendDayPixel writes (overlayColor, alpha) into civ's day buffers at
// (face, px, py). Existing zero-alpha pixels (the initial state from
// cubemap.New) get the overlay written directly; subsequent overlapping
// writes blend in OkLab. DayMask records the strongest alpha that has
// touched the pixel so the colorize step knows how strongly to tint
// the underlying biome color.
func (cf *CivField) blendDayPixel(face cubemap.Face, px, py int, overlay color.RGBA, alpha float64) {
	if alpha <= 0 {
		return
	}
	if alpha > 1 {
		alpha = 1
	}
	idx := py*cf.Size + px
	existing := cf.DayColor.Faces[face][idx]
	var c color.RGBA
	if existing.A == 0 {
		// First touch: write the overlay color directly with full
		// opacity so subsequent BlendOkLab calls have a real base.
		c = overlay
		c.A = 255
	} else {
		c = planetcolor.BlendOkLab(existing, overlay, alpha)
	}
	cf.DayColor.Faces[face][idx] = c
	if cur := cf.DayMask[face][idx]; alpha > cur {
		cf.DayMask[face][idx] = alpha
	}
}

// addNightPixel adds a warm-light contribution into the Night cube
// map. Per-channel additive saturation matches the stylized "more
// sites = brighter glow" intent of the Black Marble look.
func (cf *CivField) addNightPixel(face cubemap.Face, px, py int, intensity float64) {
	if intensity <= 0 {
		return
	}
	if intensity > 1 {
		intensity = 1
	}
	idx := py*cf.Size + px
	existing := cf.Night.Faces[face][idx]
	addR := float64(civNightWarm.R) * intensity
	addG := float64(civNightWarm.G) * intensity
	addB := float64(civNightWarm.B) * intensity
	r := float64(existing.R) + addR
	g := float64(existing.G) + addG
	b := float64(existing.B) + addB
	if r > 255 {
		r = 255
	}
	if g > 255 {
		g = 255
	}
	if b > 255 {
		b = 255
	}
	cf.Night.Faces[face][idx] = color.RGBA{
		R: uint8(r),
		G: uint8(g),
		B: uint8(b),
		A: 255,
	}
}

// rasterizeCityPatch paints a circular city patch and a banded
// agriculture pattern onto cf.DayColor / cf.DayMask for site s.
//
// The patch and band use a flat per-face pixel disc (O(rPx²) per site
// rather than O(S²)) walked in pixel-space. Pixels outside the
// face are folded across seams via cubemap.OffsetPixel so seams stay
// continuous.
//
// rng provides the per-site agriculture rotation hash; a fresh draw
// per site keeps the output deterministic given (master, "civ.day").
func (cf *CivField) rasterizeCityPatch(s Site, rng *rand.Rand) {
	S := cf.Size
	rPx := civCityRadiusPx(s.Population, S)
	if rPx <= 0 {
		return
	}
	face, cx, cy := cubemap.DirToFacePixel(s.Dir[0], s.Dir[1], s.Dir[2], S)

	// Per-site agriculture rotation (radians). Stable for a given
	// rng seed stream (which is itself derived from "civ.day").
	agAngle := rng.Float64() * 2 * math.Pi
	cosAg := math.Cos(agAngle)
	sinAg := math.Sin(agAngle)

	// Outer agriculture-band radius. 1.6x city radius matches the
	// plan's "in a band around the patch".
	outerPx := int(math.Ceil(float64(rPx) * 1.6))

	for dy := -outerPx; dy <= outerPx; dy++ {
		for dx := -outerPx; dx <= outerPx; dx++ {
			dist := math.Sqrt(float64(dx*dx + dy*dy))
			if dist > float64(outerPx) {
				continue
			}
			f, nx, ny := cubemap.OffsetPixel(face, cx, cy, dx, dy, S)
			switch {
			case dist <= float64(rPx):
				// City core: linear falloff alpha.
				alpha := (1.0 - dist/float64(rPx)) * s.Population
				cf.blendDayPixel(f, nx, ny, civCityBeige, alpha)
			case dist <= float64(outerPx):
				// Agriculture band: regular dot/checker pattern
				// modulated by the per-site rotation. Project (dx,dy)
				// onto the rotated axis and stride every 3 px so the
				// pattern reads as a coarse field grid.
				u := float64(dx)*cosAg + float64(dy)*sinAg
				v := -float64(dx)*sinAg + float64(dy)*cosAg
				ui := int(math.Round(u))
				vi := int(math.Round(v))
				// "Every 3rd pixel along the rotation axis": both u
				// and v on a 3-pixel stride, with alternating offsets
				// so the pattern looks like a checker rather than a
				// straight grid.
				if (ui%3+3)%3 != 0 || (vi%3+3)%3 != 0 {
					continue
				}
				alpha := 0.5 * s.Population
				cf.blendDayPixel(f, nx, ny, civAgriGreen, alpha)
			}
		}
	}
}

// splatNightLight stamps a Gaussian Black-Marble splat into cf.Night
// for site s. Intensity is pop^1.5 (so a tier-1 metropolis is much
// brighter than a tier-0.3 town). Falloff is exp(-d² / (r²/2)).
func (cf *CivField) splatNightLight(s Site) {
	S := cf.Size
	splatPx := civNightSplatPx(s.Population, S)
	if splatPx <= 0 {
		return
	}
	face, cx, cy := cubemap.DirToFacePixel(s.Dir[0], s.Dir[1], s.Dir[2], S)
	popI := math.Pow(s.Population, 1.5)
	denom := float64(splatPx*splatPx) / 2.0
	if denom <= 0 {
		denom = 1
	}
	for dy := -splatPx; dy <= splatPx; dy++ {
		for dx := -splatPx; dx <= splatPx; dx++ {
			d2 := float64(dx*dx + dy*dy)
			if d2 > float64(splatPx*splatPx) {
				continue
			}
			gauss := math.Exp(-d2 / denom)
			intensity := gauss * popI
			if intensity <= 0 {
				continue
			}
			f, nx, ny := cubemap.OffsetPixel(face, cx, cy, dx, dy, S)
			cf.addNightPixel(f, nx, ny, intensity)
		}
	}
}

// weaveRoads blends darker beige into DayColor at every road-pixel and
// updates DayMask accordingly. Intensity is the per-pixel value from
// RoadField.Intensity (0..roadIntensityCap, typically 0..2).
func (cf *CivField) weaveRoads() {
	for face := range cubemap.Face(cubemap.NumFaces) {
		row := cf.Roads[face]
		if row == nil {
			continue
		}
		S := cf.Size
		for i, v := range row {
			if v <= 0 {
				continue
			}
			alpha := 0.5 * v
			if alpha > 1 {
				alpha = 1
			}
			py := i / S
			px := i - py*S
			cf.blendDayPixel(face, px, py, civRoadBeige, alpha)
		}
	}
}

// GenerateCiv runs the Phase 9b civ pipeline: habitability → Bridson
// sites → Zipfian populations → roads → city/agriculture/night
// rasterization. Returns nil when civ is disabled (profile.Civ.Tier
// <= 0) or any upstream stage produces an empty result (no habitable
// pixels, zero sites, etc.).
//
// Determinism: every per-site RNG draw is sourced from
// seed.Domain(master, "civ.day") and seed.Domain(master, "civ.night").
// The road-network call uses its own internal stream(s) inherited from
// the road generator. Inputs heightmap/tField/mField/plates/flow/
// rainShadow may be nil where the generator gracefully degrades —
// habitability handles missing inputs by zeroing the corresponding
// term.
func GenerateCiv(
	heightmap *cubemap.CubeMapF,
	tField, mField *cubemap.CubeMapF,
	plates *field.PlateField,
	flow *field.FlowField,
	rainShadow *biome.RainShadowField,
	profile *types.PlanetProfile,
	master int64,
	S int,
) *CivField {
	if profile == nil || profile.Civ.Tier <= 0 {
		return nil
	}

	hf := GenerateHabitability(heightmap, tField, mField, plates, flow, rainShadow, profile.Civ, S)
	if hf == nil {
		return nil
	}
	sites := PoissonOnSphere(hf, profile.Civ, master)
	if len(sites) == 0 {
		return nil
	}
	AssignPopulations(sites, profile.Civ)

	roads := GenerateRoads(sites, heightmap, profile.Civ)

	cf := &CivField{
		Size:         S,
		Sites:        sites,
		Habitability: hf,
		DayColor:     cubemap.New(S),
		Night:        cubemap.New(S),
	}
	for face := range cubemap.Face(cubemap.NumFaces) {
		cf.DayMask[face] = make([]float64, S*S)
		cf.Roads[face] = make([]float64, S*S)
	}
	if roads != nil {
		for face := range cubemap.Face(cubemap.NumFaces) {
			if src := roads.Intensity[face]; src != nil {
				copy(cf.Roads[face], src)
			}
		}
	}

	dayRng := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "civ.day")),        //nolint:gosec // intentional reinterpretation
		uint64(seed.Domain(master, "civ.day.stream")), //nolint:gosec // intentional reinterpretation
	))
	// Reserve the night-stream domain without allocating a generator —
	// future jitter on the splat path can pick it up without disturbing
	// other streams' seed allocation order.
	_ = seed.Domain(master, "civ.night")
	_ = seed.Domain(master, "civ.night.stream")

	for _, s := range sites {
		cf.rasterizeCityPatch(s, dayRng)
	}
	cf.weaveRoads()
	for _, s := range sites {
		cf.splatNightLight(s)
	}

	return cf
}

