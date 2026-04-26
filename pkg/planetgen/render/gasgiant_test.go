package render

import (
	"image/color"
	"testing"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestRenderGasGiantAllPixelsOpaque(t *testing.T) {
	prof := &types.PlanetProfile{
		Type:          "jovian",
		Renderer:      "gas_giant",
		BandCount:     26,
		TurbulenceAmp: 0.012,
		StormCount:    3,
		StormSize:     0.25,
		Palette: []planetcolor.ColorStop{
			{Position: 0.0, Color: color.RGBA{R: 180, G: 140, B: 100, A: 255}},
		},
	}
	cm := RenderGasGiant(prof, 1234, 64)
	for face := range len(cm.Faces) {
		for i, c := range cm.Faces[face] {
			if c.A != 255 {
				t.Fatalf("face %d pixel %d alpha=%d", face, i, c.A)
			}
		}
	}
}

func TestRenderGasGiantDeterministic(t *testing.T) {
	prof := &types.PlanetProfile{
		Type:           "ice_giant",
		Renderer:       "gas_giant",
		BandCount:      22,
		TurbulenceAmp:  0.008,
		BandBlendWidth: 0.45,
		Palette: []planetcolor.ColorStop{
			{Position: 0.0, Color: color.RGBA{R: 100, G: 150, B: 185, A: 255}},
		},
	}
	a := RenderGasGiant(prof, 99, 32)
	b := RenderGasGiant(prof, 99, 32)
	for face := range len(a.Faces) {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs across runs", face, i)
			}
		}
	}
}

func TestRenderGasGiantDeterministicWithStorms(t *testing.T) {
	prof := &types.PlanetProfile{
		Type:          "jovian",
		Renderer:      "gas_giant",
		BandCount:     26,
		TurbulenceAmp: 0.012,
		StormCount:    3,
		StormSize:     0.25,
		Palette: []planetcolor.ColorStop{
			{Position: 0.0, Color: color.RGBA{R: 180, G: 140, B: 100, A: 255}},
		},
	}
	a := RenderGasGiant(prof, 42, 32)
	b := RenderGasGiant(prof, 42, 32)
	for face := range len(a.Faces) {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs across runs", face, i)
			}
		}
	}
}
