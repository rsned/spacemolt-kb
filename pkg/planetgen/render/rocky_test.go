package render_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

func TestRenderRockyAllPixelsOpaque(t *testing.T) {
	prof := planetgen.Profiles["scorched"]
	cm := render.RenderRocky(prof, 1234, 64)
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
	a := render.RenderRocky(prof, 7, 32)
	b := render.RenderRocky(prof, 7, 32)
	for face := range len(a.Faces) {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs across runs", face, i)
			}
		}
	}
}

func TestRenderRockyWithOceanAndSnow(t *testing.T) {
	// Use the real "terran" profile, which exercises ocean, snow, polar caps,
	// equatorial palette, and polar palette code paths.
	prof := planetgen.Profiles["terran"]
	cm := render.RenderRocky(prof, 42, 32)
	for face := range len(cm.Faces) {
		for i, c := range cm.Faces[face] {
			if c.A != 255 {
				t.Fatalf("face %d pixel %d alpha=%d", face, i, c.A)
			}
		}
	}
}
