package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestGenerateControlFieldsShape(t *testing.T) {
	cfg := types.ControlConfig{
		Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Detail:          types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		PeaksValleys:    types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Temperature:     types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Humidity:        types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
	}
	fields := GenerateControlFields(42, cfg, 32, nil)
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
		Detail:          types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		PeaksValleys:    types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Temperature:     types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Humidity:        types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
	}
	fields := GenerateControlFields(42, cfg, 16, nil)
	cont := fields[0].Faces[0]
	detail := fields[1].Faces[0]
	identical := true
	for i := range cont {
		if cont[i] != detail[i] {
			identical = false
			break
		}
	}
	if identical {
		t.Error("Continentalness and Detail fields are identical; named-domain mix not effective")
	}
}

func TestDetailFieldJitterDifferentFromUnjittered(t *testing.T) {
	profile := &types.PlanetProfile{
		ControlConfig: types.ControlConfig{
			Detail: types.ControlField{Amp: 1.0, Freq: 4.0, Octaves: 4, Lacunarity: 2.0, Persistence: 0.5},
			// Other fields can be zero — only Detail (index 1) is jittered.
		},
		JitterEnabled:   true,
		JitterCellCount: 32,
		JitterRotMax:    math.Pi / 4,
		JitterOffsetMax: 0.1,
	}
	S := 32
	fieldsNoJitter := GenerateControlFields(11, profile.ControlConfig, S, nil)
	jf := noise.GenerateJitter(profile, 11, S)
	fieldsJitter := GenerateControlFields(11, profile.ControlConfig, S, jf)
	detailNo, detailY := fieldsNoJitter[1], fieldsJitter[1]
	var differ, total int
	for face := range detailNo.Faces {
		for i := range detailNo.Faces[face] {
			total++
			if math.Abs(detailNo.Faces[face][i]-detailY.Faces[face][i]) > 1e-6 {
				differ++
			}
		}
	}
	if float64(differ)/float64(total) < 0.05 {
		t.Errorf("jitter changed only %d/%d (%.1f%%) of detail pixels — want ≥5%%", differ, total, 100*float64(differ)/float64(total))
	}
}
