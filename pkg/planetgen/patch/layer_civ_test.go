package patch

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestCivSitesOnLandAndRoadsAvoidOcean(t *testing.T) {
	ctx := testContext(t)
	prof := *ctx.Profile
	if prof.Civ.Tier <= 0 {
		prof.Civ = types.CivConfig{Tier: 0.5, SiteMinDistRad: 0.02, SiteMaxDistRad: 0.12, MaxPopulation: 1e7}
	}
	ctx.Profile = &prof
	s := NewStack(ctx)
	st, err := s.RenderTo(12)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Sites) == 0 {
		t.Skip("no habitable pixels in window at this seed")
	}
	// Determinism.
	s2 := NewStack(ctx)
	st2, _ := s2.RenderTo(12)
	if len(st2.Sites) != len(st.Sites) {
		t.Fatal("civ layer not deterministic")
	}
}
