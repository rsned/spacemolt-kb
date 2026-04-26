package render

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

func TestRenderRockyAllPixelsOpaque(t *testing.T) {
	prof := planetgen.Profiles["scorched"]
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
	prof := planetgen.Profiles["arid"]
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
