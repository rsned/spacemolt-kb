package shipglyph

import "testing"

func TestSeedOfIsStableAndDistinct(t *testing.T) {
	if SeedOf("prayer") != SeedOf("prayer") { //nolint:staticcheck
		t.Errorf("SeedOf is not stable")
	}
	if SeedOf("prayer") == SeedOf("comet") {
		t.Errorf("SeedOf collided for two different ids")
	}
}

func TestStyleForKnownFactions(t *testing.T) {
	if StyleFor("crimson").Chamfer <= 0 {
		t.Errorf("crimson should chamfer")
	}
	if !StyleFor("nebula").Smooth {
		t.Errorf("nebula should be smooth")
	}
	if !StyleFor("solarian").Flute {
		t.Errorf("solarian should flute")
	}
	if StyleFor("outerrim").Jitter <= 0 {
		t.Errorf("outerrim should jitter")
	}
	if !StyleFor("voidborn").Lobed {
		t.Errorf("voidborn should be lobed")
	}
	p := StyleFor("pirate")
	if p.Jitter <= 0 || p.Chamfer <= 0 {
		t.Errorf("pirate should both jitter and chamfer, got %+v", p)
	}
}

func TestStyleForUnknownFactionFallsBack(t *testing.T) {
	s := StyleFor("")
	if s.Name == "" {
		t.Errorf("unknown faction produced an unnamed style")
	}
}

func TestRNGIsDeterministicAndBounded(t *testing.T) {
	a, b := newRNG(42), newRNG(42)
	for range 100 {
		x, y := a.next(), b.next()
		if x != y {
			t.Fatalf("rng diverged: %v vs %v", x, y)
		}
		if x < 0 || x >= 1 {
			t.Fatalf("rng out of [0,1): %v", x)
		}
	}
	c := newRNG(43)
	if c.next() == newRNG(42).next() {
		t.Errorf("different seeds produced the same first value")
	}
}
