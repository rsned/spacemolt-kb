package shipglyph

import (
	"math"
	"testing"
)

func TestOutlineIsAClosedLoopOfBothSides(t *testing.T) {
	d := Infer(Stats{ID: "crowbar", Class: "Salvager", Faction: "crimson", Scale: 1})
	loop := Outline(d, StyleFor("crimson"), SeedOf("crowbar"))

	if len(loop) < profileSamples {
		t.Fatalf("len = %d, want at least %d", len(loop), profileSamples)
	}
	var sawPositive, sawNegative bool
	for _, p := range loop {
		if p.Y > 1e-9 {
			sawPositive = true
		}
		if p.Y < -1e-9 {
			sawNegative = true
		}
	}
	if !sawPositive || !sawNegative {
		t.Errorf("loop does not span both sides: +%v -%v", sawPositive, sawNegative)
	}
}

func TestOutlineStartsAtNoseAndReachesTail(t *testing.T) {
	d := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	loop := Outline(d, StyleFor("nebula"), SeedOf("comet"))

	var minX, maxX = 1.0, 0.0
	for _, p := range loop {
		minX = math.Min(minX, p.X)
		maxX = math.Max(maxX, p.X)
	}
	if minX > 1e-6 {
		t.Errorf("minX = %v, want ~0 (nose)", minX)
	}
	if maxX < 1-1e-6 {
		t.Errorf("maxX = %v, want ~1 (tail)", maxX)
	}
}

func TestOutlineChamferAddsVertices(t *testing.T) {
	d := Descriptor{
		Hull: []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.3}},
	}
	plain := Outline(d, Style{Name: "flat"}, 7)
	cham := Outline(d, Style{Name: "cham", Chamfer: 0.3}, 7)

	if len(cham) <= len(plain) {
		t.Errorf("chamfered loop has %d points, plain has %d; chamfering should add vertices",
			len(cham), len(plain))
	}
}

func TestChamferCutsAtTheCorrectPositions(t *testing.T) {
	// A unit square chamfered at f=0.25: each corner is replaced by two
	// points, each one quarter of the way along its two adjacent edges.
	// This pins the cut arithmetic itself — a chamfer that merely duplicated
	// each vertex without cutting would pass a point-count check but fail
	// this one.
	square := []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	want := []Point{
		{0, 0.25}, {0.25, 0},
		{0.75, 0}, {1, 0.25},
		{1, 0.75}, {0.75, 1},
		{0.25, 1}, {0, 0.75},
	}

	got := chamfer(square, 0.25)

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(got[i].X-want[i].X) > 1e-9 || math.Abs(got[i].Y-want[i].Y) > 1e-9 {
			t.Errorf("point %d = (%v, %v), want (%v, %v)",
				i, got[i].X, got[i].Y, want[i].X, want[i].Y)
		}
	}
}

func TestOutlineIsDeterministic(t *testing.T) {
	d := Infer(Stats{ID: "excessive_force", Class: "Drone Carrier", Faction: "outerrim", Scale: 4})
	st := StyleFor("outerrim")
	a := Outline(d, st, SeedOf("excessive_force"))
	b := Outline(d, st, SeedOf("excessive_force"))

	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("point %d diverged: %v vs %v", i, a[i], b[i])
		}
	}
}
