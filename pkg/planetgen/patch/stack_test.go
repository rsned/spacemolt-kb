package patch

import "testing"

// testStack builds a Stack whose first three layers count their
// invocations, to pin cache/dirty semantics without real layers.
func countingStack(t *testing.T, counts *[13]int) *Stack {
	t.Helper()
	sd := testSphere(t)
	w := Pick(sd, 32, 64, 1)[0].Window
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &Context{Sphere: sd, Fields: f, Profile: sd.Profile, Master: sd.Master}
	s := NewStack(ctx)
	for i := range s.layers {
		idx := i
		inner := s.layers[i].Apply
		s.layers[i].Apply = func(c *Context, st *State) *State {
			counts[idx]++
			return inner(c, st)
		}
	}
	return s
}

func TestStackCachesCleanLayers(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if _, err := s.RenderTo(4); err != nil {
		t.Fatal(err)
	}
	if counts[0] != 1 || counts[4] != 1 {
		t.Fatalf("first render should run each layer once: %v", counts)
	}
	if _, err := s.RenderTo(4); err != nil {
		t.Fatal(err)
	}
	if counts[0] != 1 {
		t.Fatalf("clean re-render must be fully cached: %v", counts)
	}
}

func TestStackDirtyRerunsSuffixOnly(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if _, err := s.RenderTo(6); err != nil {
		t.Fatal(err)
	}
	if needsSphere := s.MarkDirty("Erosion.Droplets"); needsSphere {
		t.Fatal("Erosion is a stack param, not a sphere param")
	}
	if _, err := s.RenderTo(6); err != nil {
		t.Fatal(err)
	}
	if counts[5] != 1 {
		t.Fatalf("layer 5 (coastal) must stay cached, ran %d times", counts[5])
	}
	if counts[6] != 2 {
		t.Fatalf("layer 6 (erosion) must re-run, ran %d times", counts[6])
	}
}

func TestStackSphereParamSignals(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if needsSphere := s.MarkDirty("crust.majorPlates"); !needsSphere {
		t.Fatal("crust seeding params must signal a sphere recompute")
	}
}
