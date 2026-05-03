package render_test

import (
	"testing"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

// TestRenderRockyS64WallClock is a sanity-check timer for the Phase 8
// rocky pipeline: ensures a single S=64 RenderRocky call completes in
// well under 5 seconds even after the new flow + rain shadow stages
// land. Failing this is a sign that one of the new passes regressed
// (e.g. accumulation iteration order, walk-step explosion). Skipped
// in -short mode so the standard -count=1 sweep stays fast.
func TestRenderRockyS64WallClock(t *testing.T) {
	if testing.Short() {
		t.Skip("perf check skipped under -short")
	}
	profile := planetgen.Profiles["terran"]
	start := time.Now()
	cm := render.RenderRocky(profile, 42, 64)
	elapsed := time.Since(start)
	if cm == nil {
		t.Fatal("RenderRocky returned nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("RenderRocky S=64 took %v, exceeds 5s budget", elapsed)
	}
	t.Logf("RenderRocky terran S=64: %v", elapsed)
}
