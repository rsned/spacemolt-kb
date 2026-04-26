package render

import (
	"image/color"
	"testing"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestRenderRockyAllPixelsOpaque(t *testing.T) {
	prof := &types.PlanetProfile{
		Type:             "scorched",
		Renderer:         "rocky",
		NoiseOctaves:     8,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.5,
		CraterCount:      200,
		CraterMinRadius:  0.005,
		CraterMaxRadius:  0.08,
		CraterDepth:      0.25,
		Palette: []planetcolor.ColorStop{
			{0.0, color.RGBA{R: 40, G: 40, B: 40, A: 255}},
		},
	}
	cm := RenderRocky(prof, 1234, 64)
	for face := range len(cm.Faces) {
		for i, c := range cm.Faces[face] {
			if c.A != 255 {
				t.Fatalf("face %d pixel %d alpha=%d", face, i, c.A)
			}
		}
	}
}

func TestRenderRockyDeterministic(t *testing.T) {
	prof := &types.PlanetProfile{
		Type:             "arid",
		Renderer:         "rocky",
		NoiseOctaves:     7,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.5,
		CraterCount:      80,
		CraterMinRadius:  0.005,
		CraterMaxRadius:  0.06,
		CraterDepth:      0.2,
		HasPolarCaps:     true,
		PolarCapSize:     0.18,
		PolarCapNoise:    0.15,
		Palette: []planetcolor.ColorStop{
			{0.0, color.RGBA{R: 100, G: 60, B: 30, A: 255}},
		},
	}
	a := RenderRocky(prof, 7, 32)
	b := RenderRocky(prof, 7, 32)
	for face := range len(a.Faces) {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs across runs", face, i)
			}
		}
	}
}

func TestRenderRockyWithOceanAndSnow(t *testing.T) {
	prof := &types.PlanetProfile{
		Type:             "test_terran",
		Renderer:         "rocky",
		NoiseOctaves:     6,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.0,
		OceanLevel:       0.50,
		OceanColor:       color.RGBA{R: 30, G: 60, B: 140, A: 255},
		SnowLine:         0.80,
		HasPolarCaps:     true,
		PolarCapSize:     0.15,
		Palette: []planetcolor.ColorStop{
			{Position: 0.0, Color: color.RGBA{R: 20, G: 80, B: 30, A: 255}},
			{Position: 1.0, Color: color.RGBA{R: 180, G: 175, B: 160, A: 255}},
		},
		EquatorialPalette: []planetcolor.ColorStop{
			{Position: 0.0, Color: color.RGBA{R: 190, G: 170, B: 120, A: 255}},
			{Position: 1.0, Color: color.RGBA{R: 200, G: 200, B: 195, A: 255}},
		},
	}
	cm := RenderRocky(prof, 42, 32)
	for face := range len(cm.Faces) {
		for i, c := range cm.Faces[face] {
			if c.A != 255 {
				t.Fatalf("face %d pixel %d alpha=%d", face, i, c.A)
			}
		}
	}
}
