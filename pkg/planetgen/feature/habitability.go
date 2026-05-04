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
func GenerateHabitability(
	heightmap *cubemap.CubeMapF,
	tField, mField *cubemap.CubeMapF,
	plates *field.PlateField,
	flow *field.FlowField,
	rainShadow *biome.RainShadowField,
	cfg types.CivConfig,
	S int,
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

			// Height: bonus for lowland/mid, penalty for peaks.
			score := habWeightHeightLow * smoothstep(habHeightLowA, habHeightLowB, h)
			score -= habWeightHeightHigh * smoothstep(habHeightHighA, habHeightHighB, h)

			// Temperature: gaussian centered on temperate.
			score += habWeightTemp * gaussian(t, habTempMu, habTempSigma)

			// Moisture × rain shadow.
			rs := 1.0
			if rsSlice != nil {
				rs = rsSlice[i]
			}
			score += habWeightMoist * gaussian(m*rs, habMoistMu, habMoistSigma)

			// River boost: binary "is river?" → riverDistRecip = 1.0
			// saturates the smoothstep, else 0 below the low end.
			if rivers != nil && rivers[i] {
				score += habWeightRiver * smoothstep(0.0, 0.05, 1.0)
			}

			// Convergent-boundary volcanism penalty: binary cutoff at
			// habConvergeKm. plates.Convergent is a per-pixel
			// distance-to-nearest-convergent-boundary in km.
			if convergent != nil && convergent[i] < habConvergeKm {
				score -= habWeightConvergent
			}

			// Clamp to [0, 1].
			if score < 0 {
				score = 0
			} else if score > 1 {
				score = 1
			}
			out[i] = score
		}
	}
	return hf
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
