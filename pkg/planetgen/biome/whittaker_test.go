package biome

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestGenerateClimateFieldsShape(t *testing.T) {
	prof := &types.PlanetProfile{
		ControlConfig: types.ControlConfig{
			Temperature: types.ControlField{Amp: 1, Freq: 1, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
			Humidity:    types.ControlField{Amp: 1, Freq: 1, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
		},
	}
	tField, mField := GenerateClimateFields(42, prof, 32)
	if tField.Size != 32 || mField.Size != 32 {
		t.Errorf("expected 32×32 fields, got T=%d M=%d", tField.Size, mField.Size)
	}
}

func TestGenerateClimateFieldsPolarColder(t *testing.T) {
	// Pole pixels (top of +Y face) should have T < equator pixels
	// (center of +X face) on average, after the cos(lat) bias.
	prof := &types.PlanetProfile{
		ControlConfig: types.ControlConfig{
			Temperature: types.ControlField{Amp: 1, Freq: 1, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
			Humidity:    types.ControlField{Amp: 1, Freq: 1, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
		},
	}
	tField, _ := GenerateClimateFields(42, prof, 32)
	poleT := tField.Get(2, 16, 16)   // FacePosY center — north pole
	equatorT := tField.Get(0, 16, 16) // FacePosX center — equator
	if poleT >= equatorT {
		t.Errorf("pole T (%f) should be < equator T (%f)", poleT, equatorT)
	}
}
