package patch

import "testing"

func TestErosionDeterministicAndBounded(t *testing.T) {
	ctx := testContext(t)
	if ctx.Profile.Erosion.Droplets <= 0 {
		t.Fatal("terran profile is expected to have erosion enabled")
	}
	s1 := NewStack(ctx)
	a, err := s1.RenderTo(6)
	if err != nil {
		t.Fatal(err)
	}
	s2 := NewStack(ctx)
	b, _ := s2.RenderTo(6)
	for i := range a.Height.Data {
		if a.Height.Data[i] != b.Height.Data[i] {
			t.Fatal("erosion is not deterministic")
		}
	}
	st5, _ := s1.RenderTo(5)
	diff := 0
	for i := range a.Height.Data {
		v := a.Height.Data[i]
		if v != st5.Height.Data[i] {
			diff++
		}
		if v != v { // NaN
			t.Fatal("erosion produced NaN")
		}
	}
	if diff == 0 {
		t.Fatal("erosion changed nothing")
	}
}

func TestPatchDropletsScaling(t *testing.T) {
	if got := patchDroplets(250000, 512, 1024); got < 9000 || got > 12000 {
		t.Fatalf("512²@1024 share of 250k should be ~10.4k, got %d", got)
	}
	if got := patchDroplets(10, 32, 1024); got != 200 {
		t.Fatalf("floor must be 200, got %d", got)
	}
}
