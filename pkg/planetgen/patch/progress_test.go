package patch

import (
	"slices"
	"strings"
	"testing"
)

// TestProgressHookSphereSequence asserts ComputeSphere announces its
// stages in pipeline order, and that setting the hook does not change
// any derived scalar (byte-level identity is separately pinned by
// TestPatchLayerGoldens running with a nil hook).
func TestProgressHookSphereSequence(t *testing.T) {
	base := testSphere(t) // nil-hook baseline

	var got []string
	SetProgressHook(func(stage string, i, n int) {
		got = append(got, stage)
		if n != 10 {
			t.Errorf("stage %q: n = %d, want 10", stage, n)
		}
	})
	t.Cleanup(func() { SetProgressHook(nil) })

	hooked, err := ComputeSphere(base.Profile, base.Master, base.STect)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := []string{"sphere:jitter", "sphere:plates", "sphere:crust", "sphere:fx", "sphere:splines", "sphere:tectonic-fx"}
	if len(got) < len(wantPrefix) || !slices.Equal(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("stage sequence = %v, want prefix %v", got, wantPrefix)
	}
	for _, s := range got {
		if !strings.HasPrefix(s, "sphere:") {
			t.Fatalf("unexpected stage key %q", s)
		}
	}
	if hooked.HMin != base.HMin || hooked.HMax != base.HMax ||
		hooked.SeaLevel0 != base.SeaLevel0 || hooked.SeaLevel != base.SeaLevel {
		t.Fatalf("hooked ComputeSphere diverged: %+v vs %+v",
			[4]float64{hooked.HMin, hooked.HMax, hooked.SeaLevel0, hooked.SeaLevel},
			[4]float64{base.HMin, base.HMax, base.SeaLevel0, base.SeaLevel})
	}
}

// TestProgressHookRenderToReportsRunLayersOnly asserts RenderTo
// announces exactly the layers it re-runs (cached layers are silent).
func TestProgressHookRenderToReportsRunLayersOnly(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)

	var got []string
	SetProgressHook(func(stage string, i, n int) {
		if strings.HasPrefix(stage, "layer:") {
			got = append(got, stage)
			if n != 13 {
				t.Errorf("stage %q: n = %d, want 13", stage, n)
			}
		}
	})
	t.Cleanup(func() { SetProgressHook(nil) })

	if _, err := s.RenderTo(3); err != nil {
		t.Fatal(err)
	}
	first := len(got)
	if first == 0 {
		t.Fatal("no layer progress reported on first render")
	}
	got = got[:0]
	if _, err := s.RenderTo(3); err != nil { // fully cached
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("cached re-render reported %v, want none", got)
	}
}
