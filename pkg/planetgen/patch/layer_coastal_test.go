package patch

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestCoastalOnlyTouchesNearCoast(t *testing.T) {
	ctx := testContext(t)
	// No built-in test profile sets Coastal.Amp (terranProfile leaves it
	// at the zero value), so copy the profile and pin a real coastal
	// config to actually exercise the enabled path.
	prof := *ctx.Profile
	prof.Coastal = types.CoastalConfig{Amp: 0.05, Threshold: 0.5, Freq: 8}
	ctx.Profile = &prof

	s := NewStack(ctx)
	st4, _ := s.RenderTo(4)
	st5, err := s.RenderTo(5)
	if err != nil {
		t.Fatal(err)
	}
	if st5.DistCoast == nil {
		t.Fatal("coastal layer must publish DistCoast")
	}
	changed, far := 0, 0
	for i := range st5.Height.Data {
		if st5.Height.Data[i] != st4.Height.Data[i] {
			changed++
			if st5.DistCoast.Data[i] > 0.2 {
				far++
			}
		}
	}
	if changed == 0 {
		t.Skip("window has no coast; acceptable for some seeds")
	}
	if far > changed/10 {
		t.Fatalf("coastal noise reached far inland/offshore: %d of %d changed pixels", far, changed)
	}
}

func TestCoastalDisabledIsIdentity(t *testing.T) {
	ctx := testContext(t)
	prof := *ctx.Profile
	prof.Coastal.Amp = 0
	ctx.Profile = &prof
	s := NewStack(ctx)
	st4, _ := s.RenderTo(4)
	st5, _ := s.RenderTo(5)
	if st5 != st4 {
		t.Fatal("disabled coastal layer must pass the state through untouched")
	}
}
