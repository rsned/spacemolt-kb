package field_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// TestControlFieldsDirectionSymmetry verifies that GenerateControlFields
// is a function of pixel direction only — i.e. two pixels on opposite
// faces of a cube edge whose centers point at nearly-the-same 3D
// direction produce nearly-the-same value.
//
// Production profiles have fbm configs at or above Nyquist for any
// practical face size, so this test deliberately uses a synthetic
// low-frequency, low-octave config that stays well below Nyquist at
// S=64. The seam-continuity check is therefore meaningful (bilinear
// interpolation on the neighbor face can recover the source pixel's
// value to within a tight tolerance) and exercises the direction-only
// sampling property of pkg/planetgen/field/control.go without being
// confounded by sub-pixel aliasing.
func TestControlFieldsDirectionSymmetry(t *testing.T) {
	// Freq=1, Octaves=1: noise gradient ~1, bilinear second-order
	// error floor at S=128 (pixel ~0.025 rad) is around 1.5-2%. Set
	// tol=3% with margin. If the production code regressed to
	// per-face raster sampling, seam delta would blow past 10%.
	low := types.ControlField{Amp: 1.0, Freq: 1, Octaves: 1, Lacunarity: 2, Persistence: 0.5}
	cfg := types.ControlConfig{
		Continentalness: low,
		Detail:          low,
		PeaksValleys:    low,
		Temperature:     low,
		Humidity:        low,
	}
	S := 128
	fields := field.GenerateControlFields(42, cfg, S, nil)
	names := [5]string{"Continentalness", "Detail", "PeaksValleys", "Temperature", "Humidity"}
	for i, f := range fields {
		seamtest.AssertSeamContinuityBilinear(t, names[i], f, 0.03)
	}
}
