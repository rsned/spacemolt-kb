package shipglyph

import "testing"

func TestSeedOfPinsTheHashingScheme(t *testing.T) {
	// Golden FNV-1a values. Every ship's visual jitter derives from SeedOf,
	// so a change here silently re-rolls all 335 glyphs. Pin it so that
	// change fails loudly instead.
	cases := map[string]uint64{
		"prayer":    0x874bdcd93ae8b92a,
		"comet":     0x6d628c8e1de4078f,
		"war_wagon": 0xa398ce717915ef06,
	}
	for id, want := range cases {
		if got := SeedOf(id); got != want {
			t.Errorf("SeedOf(%q) = %#x, want %#x", id, got, want)
		}
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

func TestRNGPinsTheGeneratorAlgorithm(t *testing.T) {
	// Golden splitmix64 output for seed 42. Guards against swapping in a
	// different self-consistent generator, which would re-roll the jitter
	// on every ship without failing the self-consistency test above.
	want := []float64{
		0.74156487877182331,
		0.15991039287692010,
		0.27860113025513866,
	}
	r := newRNG(42)
	for i, w := range want {
		if got := r.next(); got != w {
			t.Errorf("next() call %d = %.17g, want %.17g", i+1, got, w)
		}
	}
}
