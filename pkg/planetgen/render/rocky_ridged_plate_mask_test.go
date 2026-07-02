package render_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// TestRidgedMaskUsesPlateConvergent verifies that with
// PlateConvergentScaleKm > 0, the ridged mountain mask is sourced from
// the plate convergent SDF: pixels near a convergent boundary should
// see significantly more height lift (higher mean and higher variance)
// than pixels far from one, when compared against a control render that
// disables the plate-driven mask. This is the structural invariant
// behind Phase 8 item 6 — mountains form along convergent plate
// boundaries.
func TestRidgedMaskUsesPlateConvergent(t *testing.T) {
	const S = 64
	const seedVal int64 = 42

	pPlate := *planetgen.Profiles["terran"]
	// Phase 12: terran now defaults to the crust path, which disables
	// legacy Ridged entirely. This test verifies the legacy plate-mask
	// behavior, so force the legacy path.
	pPlate.Crust = types.CrustConfig{}
	pPlate.TectonicFX = types.TectonicFXConfig{}
	if pPlate.Ridged.PlateConvergentScaleKm <= 0 {
		t.Fatalf("terran profile must have PlateConvergentScaleKm > 0; got %v", pPlate.Ridged.PlateConvergentScaleKm)
	}
	pLegacy := pPlate
	pLegacy.Ridged.PlateConvergentScaleKm = 0 // legacy Continentalness mask

	cmPlate := render.RenderRockyHeightmap(&pPlate, seedVal, S)
	cmLegacy := render.RenderRockyHeightmap(&pLegacy, seedVal, S)
	pf := field.GeneratePlates(&pPlate, seedVal, S)
	if pf == nil {
		t.Fatal("expected non-nil PlateField for terran profile")
	}

	// Classify pixels: "near" = within scale/4 km of a convergent
	// boundary (strongest plate-driven mask). "far" = beyond 2× the
	// scale (plate-driven mask exactly zero past 1× scale).
	scale := pPlate.Ridged.PlateConvergentScaleKm
	nearMaxKm := scale * 0.25
	farMinKm := scale * 2.0

	// Per-pixel difference plate-mode minus legacy-mode: the ridged
	// rewrite isolates the plate mask's contribution from the rest of
	// the pipeline, removing baseline height variance from the
	// statistic.
	var nearDeltas, farDeltas []float64
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				distKm := pf.Convergent[face][py*S+px]
				delta := float64(cmPlate.Get(face, px, py).R) - float64(cmLegacy.Get(face, px, py).R)
				switch {
				case distKm <= nearMaxKm:
					nearDeltas = append(nearDeltas, delta)
				case distKm >= farMinKm:
					farDeltas = append(farDeltas, delta)
				}
			}
		}
	}

	if len(nearDeltas) < 100 || len(farDeltas) < 100 {
		t.Fatalf("not enough samples: near=%d far=%d", len(nearDeltas), len(farDeltas))
	}

	nearMean, nearVar := meanVar(nearDeltas)
	farMean, farVar := meanVar(farDeltas)

	t.Logf("near (≤%.0f km, n=%d): mean Δ=%.3f var Δ=%.4f", nearMaxKm, len(nearDeltas), nearMean, nearVar)
	t.Logf("far  (≥%.0f km, n=%d): mean Δ=%.3f var Δ=%.4f", farMinKm, len(farDeltas), farMean, farVar)

	if !(nearVar > 1.5*farVar) {
		t.Errorf("expected variance(plate-vs-legacy delta near) > 1.5 * variance far: got near=%.4f far=%.4f", nearVar, farVar)
	}
	if !(nearMean > farMean) {
		t.Errorf("expected mean(plate-vs-legacy delta near) > mean far: got near=%.3f far=%.3f", nearMean, farMean)
	}
}

// TestRidgedMaskLegacyPathPreserved confirms that for an archetype with
// PlateConvergentScaleKm == 0 (back-compat fallback to the legacy
// Continentalness mask), repeated renders with the same seed produce
// byte-equal output — i.e. the legacy path is deterministic and not
// silently picking up plate state.
func TestRidgedMaskLegacyPathPreserved(t *testing.T) {
	for _, name := range []string{"ice_world", "hothouse"} {
		p := planetgen.Profiles[name]
		if p.Ridged.PlateConvergentScaleKm != 0 {
			t.Errorf("%s profile expected PlateConvergentScaleKm == 0; got %v", name, p.Ridged.PlateConvergentScaleKm)
			continue
		}
		a := render.RenderRocky(p, 42, 64)
		b := render.RenderRocky(p, 42, 64)
		for face := range len(a.Faces) {
			for i := range a.Faces[face] {
				if a.Faces[face][i] != b.Faces[face][i] {
					t.Fatalf("%s face %d pixel %d differs across runs (legacy path nondeterminism)", name, face, i)
				}
			}
		}
	}
}

func meanVar(xs []float64) (mean, variance float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean = sum / float64(len(xs))
	var ssq float64
	for _, x := range xs {
		d := x - mean
		ssq += d * d
	}
	variance = ssq / float64(len(xs))
	return
}
