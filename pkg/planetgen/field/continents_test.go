package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestContinentsDeterministic(t *testing.T) {
	cfg := types.ContinentConfig{Seeds: 12, WarpAmp: 0.1, WarpFreq: 1.0, HeightLo: 0.3, HeightHi: 0.7}
	a := GenerateContinents(42, cfg, 32)
	b := GenerateContinents(42, cfg, 32)
	for face := range a.Faces {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatal("non-deterministic continent base height")
			}
		}
	}
}

func TestContinentsCoverEverything(t *testing.T) {
	cfg := types.ContinentConfig{Seeds: 8, WarpAmp: 0, WarpFreq: 1, HeightLo: 0.3, HeightHi: 0.7}
	out := GenerateContinents(7, cfg, 32)
	for face := range out.Faces {
		for i, v := range out.Faces[face] {
			if v < 0.3 || v > 0.7 {
				t.Errorf("face %d idx %d height %f outside [0.3,0.7]", face, i, v)
			}
		}
	}
}
