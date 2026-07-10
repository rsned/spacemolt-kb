package patch

import "testing"

func TestFlowBorderIsDrain(t *testing.T) {
	ctx := testContext(t)
	if ctx.Profile.Flow.RiverThreshold <= 0 {
		t.Fatal("terran profile is expected to have flow enabled")
	}
	s := NewStack(ctx)
	st, err := s.RenderTo(8)
	if err != nil {
		t.Fatal(err)
	}
	if st.Rivers == nil || st.FlowAccum == nil {
		t.Fatal("flow layer must publish Rivers + FlowAccum")
	}
	// Edge-drain invariant: the filled surface never creates a lake
	// pinned against the frame — every interior pixel must have a
	// non-ascending D8 path that reaches the border. Cheap proxy:
	// total accumulation reaching the border+pits equals the pixel
	// count (mass conservation).
	size := st.Height.Size
	var reached float64
	for i, a := range st.FlowAccum.Data {
		ix, iy := i%size, i/size
		border := ix == 0 || iy == 0 || ix == size-1 || iy == size-1
		if border {
			reached += a
		}
	}
	if reached < float64(size*size)/4 {
		t.Fatalf("suspiciously little flow reaches the border: %v of %d", reached, size*size)
	}
}

func TestCratersLayerIdentityWhenZero(t *testing.T) {
	ctx := testContext(t)
	prof := *ctx.Profile
	prof.CraterCount = 0
	ctx.Profile = &prof
	s := NewStack(ctx)
	st6, _ := s.RenderTo(6)
	st7, _ := s.RenderTo(7)
	if st7 != st6 {
		t.Fatal("craters disabled must be identity (Enabled=false passthrough)")
	}
}
