package render_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

func TestRenderGasGiantAllPixelsOpaque(t *testing.T) {
	prof := planetgen.Profiles["jovian"]
	cm := render.RenderGasGiant(prof, 1234, 64)
	for face := range len(cm.Faces) {
		for i, c := range cm.Faces[face] {
			if c.A != 255 {
				t.Fatalf("face %d pixel %d alpha=%d", face, i, c.A)
			}
		}
	}
}

func TestRenderGasGiantDeterministic(t *testing.T) {
	prof := planetgen.Profiles["ice_giant"]
	a := render.RenderGasGiant(prof, 99, 32)
	b := render.RenderGasGiant(prof, 99, 32)
	for face := range len(a.Faces) {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs across runs", face, i)
			}
		}
	}
}

func TestRenderGasGiantDeterministicWithStorms(t *testing.T) {
	prof := planetgen.Profiles["jovian"] // StormCount: 3
	a := render.RenderGasGiant(prof, 42, 32)
	b := render.RenderGasGiant(prof, 42, 32)
	for face := range len(a.Faces) {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs across runs", face, i)
			}
		}
	}
}
