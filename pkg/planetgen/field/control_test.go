package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestGenerateControlFieldsShape(t *testing.T) {
	cfg := types.ControlConfig{
		Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Erosion:         types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		PeaksValleys:    types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Temperature:     types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Humidity:        types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
	}
	fields := GenerateControlFields(42, cfg, 32)
	if len(fields) != 5 {
		t.Fatalf("got %d fields, want 5", len(fields))
	}
	for i, f := range fields {
		if f.Size != 32 {
			t.Errorf("field %d size = %d, want 32", i, f.Size)
		}
	}
}

func TestGenerateControlFieldsOrthogonal(t *testing.T) {
	// Two fields with same parameters but different domain seeds should
	// produce different output (otherwise the named-domain mix is broken).
	cfg := types.ControlConfig{
		Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Erosion:         types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		PeaksValleys:    types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Temperature:     types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Humidity:        types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
	}
	fields := GenerateControlFields(42, cfg, 16)
	cont := fields[0].Faces[0]
	eros := fields[1].Faces[0]
	identical := true
	for i := range cont {
		if cont[i] != eros[i] {
			identical = false
			break
		}
	}
	if identical {
		t.Error("Continentalness and Erosion fields are identical; named-domain mix not effective")
	}
}
