package feature

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// HabitabilityField holds the per-pixel scalar habitability score in
// [0,1] used by the Phase 9b civilization-overlay pipeline. Score is
// flat row-major Size*Size per face; downstream Bridson site placement
// (Task 2) samples this directly via cubemap.FacePixelToDir.
type HabitabilityField struct {
	Size  int
	Score [cubemap.NumFaces][]float64
}

// Default habitability weights (P9b plan §1, lines 57-66). Tuned so
// the ideal temperate-lowland pixel scores ~0.5 and ocean/peak/desert
// pixels stay below ~0.1 under the synthetic fixtures used in
// habitability_test.go.
const (
	habWeightHeightLow  = 0.35 // bonus for low-to-mid elevation (smoothstep 0.30..0.55)
	habWeightHeightHigh = 0.20 // penalty for high elevation (smoothstep 0.55..0.85)
	habWeightTemp       = 0.20 // gaussian on temperature, μ=0.55, σ=0.15
	habWeightMoist      = 0.20 // gaussian on humidity*rainshadow, μ=0.60, σ=0.20
	habWeightRiver      = 0.10 // bonus for river-adjacent pixels
	habWeightConvergent = 0.15 // penalty along convergent plate boundaries (volcanism)

	habTempMu      = 0.55
	habTempSigma   = 0.15
	habMoistMu     = 0.60
	habMoistSigma  = 0.20
	habConvergeKm  = 100.0 // distance-to-convergent-boundary cutoff in km
	habHeightLowA  = 0.30
	habHeightLowB  = 0.55
	habHeightHighA = 0.55
	habHeightHighB = 0.85

	// habOceanRampWidth is the smoothstep upper-anchor offset above
	// oceanLevel for the lowland bonus. A shallow continent at
	// h = oceanLevel + habOceanRampWidth earns the saturated bonus.
	habOceanRampWidth = 0.05
)

// GenerateHabitability composes the Phase 9b habitability scalar field
// from the rocky-pipeline outputs.
//
// Returns nil when cfg.Tier <= 0 (zero-value disables, matching the
// "Cloud.Coverage == 0 → off" idiom). Inputs plates, flow, and
// rainShadow may be nil for archetypes without those features —
// habitability falls back to height + climate only when they are
// missing (the corresponding terms drop to zero).
//
// The river term uses a binary "is river?" boost rather than a true
// signed-distance field: when flow != nil and flow.Rivers[face][idx]
// is set the riverDistRecip input is 1.0 (giving the smoothstep
// upper-saturation value); else 0. This is the simpler interpretation
// permitted by the plan (§1) and avoids a second JFA pass.
//
// oceanLevel is the archetype-specific sea-level threshold (typically
// profile.OceanLevel). The lowland-bonus smoothstep is anchored to
// oceanLevel, and a hard guard zeroes the score for any pixel below
// oceanLevel so ocean pixels can't slip past PoissonOnSphere's 0.05
// floor via temperature/moisture contributions. When oceanLevel <= 0
// (archetypes without an ocean) both anchors revert to the legacy
// behavior — bonus smoothstep at (0.30, 0.55) and no hard guard.
func GenerateHabitability(
	heightmap *cubemap.CubeMapF,
	tField, mField *cubemap.CubeMapF,
	plates *field.PlateField,
	flow *field.FlowField,
	rainShadow *biome.RainShadowField,
	cfg types.CivConfig,
	S int,
	oceanLevel float64,
) *HabitabilityField {
	if cfg.Tier <= 0 {
		return nil
	}
	if heightmap == nil || tField == nil || mField == nil {
		return nil
	}

	hf := &HabitabilityField{Size: S}
	for f := range hf.Score {
		hf.Score[f] = make([]float64, S*S)
	}

	for face := range cubemap.Face(cubemap.NumFaces) {
		hSlice := heightmap.Faces[face]
		tSlice := tField.Faces[face]
		mSlice := mField.Faces[face]

		var rsSlice []float64
		if rainShadow != nil {
			rsSlice = rainShadow.Multiplier[face]
		}
		var rivers []bool
		if flow != nil {
			rivers = flow.Rivers[face]
		}
		var convergent []float64
		if plates != nil {
			convergent = plates.Convergent[face]
		}

		out := hf.Score[face]
		for i := range out {
			h := hSlice[i]
			t := tSlice[i]
			m := mSlice[i]

			rs := 1.0
			if rsSlice != nil {
				rs = rsSlice[i]
			}
			onRiver := rivers != nil && rivers[i]
			convKm := math.Inf(1)
			if convergent != nil {
				convKm = convergent[i]
			}

			out[i] = HabitabilityScoreAt(h, t, m, rs, onRiver, convKm, oceanLevel)
		}
	}
	return hf
}

// HabitabilityScoreAt computes the per-pixel habitability score in
// [0,1] from height, temperature, moisture, rain-shadow multiplier,
// river adjacency, distance-to-convergent-boundary, and the
// archetype's ocean level. Callers without plate data should pass
// convergentKm = math.Inf(1); callers without a rain-shadow field
// should pass rainMult = 1.
//
// Extracted verbatim from GenerateHabitability's per-pixel loop body
// so the flat-patch renderer can reuse the exact formula.
func HabitabilityScoreAt(h, t, m, rainMult float64, onRiver bool, convergentKm, oceanLevel float64) float64 {
	// Lowland-bonus smoothstep anchors. When oceanLevel > 0, the bonus
	// only starts ramping above sea level so ocean pixels score 0 in the
	// height term. The +0.05 upper anchor lets a shallow continent at
	// h=oceanLevel+0.05 still earn most of the bonus.
	lowA := habHeightLowA
	lowB := habHeightLowB
	if oceanLevel > 0 {
		lowA = oceanLevel
		lowB = oceanLevel + habOceanRampWidth
	}

	// Height: bonus for lowland/mid (anchored to oceanLevel
	// when present), penalty for peaks (mountain penalty is
	// independent of OceanLevel).
	score := habWeightHeightLow * smoothstep(lowA, lowB, h)
	score -= habWeightHeightHigh * smoothstep(habHeightHighA, habHeightHighB, h)

	// Temperature: gaussian centered on temperate.
	score += habWeightTemp * gaussian(t, habTempMu, habTempSigma)

	// Moisture × rain shadow.
	score += habWeightMoist * gaussian(m*rainMult, habMoistMu, habMoistSigma)

	// River boost: binary "is river?" — the plan formula uses
	// smoothstep(0, 0.05, riverDistRecip) but with no per-pixel
	// river-distance SDF available we collapse to a saturated
	// 1.0 when the pixel is on a river, 0 otherwise.
	if onRiver {
		score += habWeightRiver
	}

	// Convergent-boundary volcanism penalty: binary cutoff at
	// habConvergeKm. convergentKm is the per-pixel
	// distance-to-nearest-convergent-boundary in km.
	if convergentKm < habConvergeKm {
		score -= habWeightConvergent
	}

	// Clamp to [0, 1].
	if score < 0 {
		score = 0
	} else if score > 1 {
		score = 1
	}
	// Hard ocean guard: pixels below sea level are not habitable
	// regardless of climate. Skip when oceanLevel <= 0 (rare
	// archetype with no ocean). Strict <: pixels at exactly
	// h == oceanLevel are treated as shoreline, not ocean.
	if oceanLevel > 0 && h < oceanLevel {
		score = 0
	}
	return score
}

// smoothstep is the canonical Hermite blend on (a, b). Below a
// returns 0; above b returns 1; linearly-interpolated and smoothed in
// between. a == b returns 0/1 step at a.
func smoothstep(a, b, x float64) float64 {
	if b == a {
		if x < a {
			return 0
		}
		return 1
	}
	t := (x - a) / (b - a)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}

// gaussian returns exp(-((x-μ)² / (2σ²))). σ must be > 0.
func gaussian(x, mu, sigma float64) float64 {
	d := x - mu
	return math.Exp(-(d * d) / (2 * sigma * sigma))
}
