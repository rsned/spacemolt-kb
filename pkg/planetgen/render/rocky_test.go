package render_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

// TestRockyRenderJitterShiftsOutput verifies that enabling jitter produces
// visibly different output (≥10% pixel diff) vs disabled jitter.
func TestRockyRenderJitterShiftsOutput(t *testing.T) {
	const S = 64
	const seed int64 = 13

	base := *planetgen.Profiles["terran"]

	noJitter := base
	noJitter.JitterEnabled = false

	withJitter := base
	withJitter.JitterEnabled = true

	cmNo := render.RenderRocky(&noJitter, seed, S)
	cmWith := render.RenderRocky(&withJitter, seed, S)

	total, diff := 0, 0
	for face := range len(cmNo.Faces) {
		for i := range cmNo.Faces[face] {
			total++
			if cmNo.Faces[face][i] != cmWith.Faces[face][i] {
				diff++
			}
		}
	}

	pct := float64(diff) / float64(total) * 100
	t.Logf("jitter shift: %d/%d pixels (%.2f%%)", diff, total, pct)
	if pct < 10 {
		t.Errorf("expected ≥10%% pixels to differ with jitter enabled; got %.2f%%", pct)
	}
}

// TestRockyRenderPlatesNoEffectOnProduction verifies that Phase 7 invariant:
// changing PlateCount must NOT change the rendered output (plates are data-only).
func TestRockyRenderPlatesNoEffectOnProduction(t *testing.T) {
	const S = 64
	const seed int64 = 13

	base := *planetgen.Profiles["terran"]

	noPlates := base
	noPlates.PlateCount = 0

	withPlates := base
	withPlates.PlateCount = 12

	cmNo := render.RenderRocky(&noPlates, seed, S)
	cmWith := render.RenderRocky(&withPlates, seed, S)

	for face := range len(cmNo.Faces) {
		for i := range cmNo.Faces[face] {
			if cmNo.Faces[face][i] != cmWith.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs: PlateCount changed production output", face, i)
			}
		}
	}
}

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
