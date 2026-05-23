package hyperjump

import (
	"math"
	"testing"
)

func TestCoverage_singleSystemOneGap(t *testing.T) {
	// One far system at bearing 0, r=1000 -> arc half-width asin(0.1).
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "P", Name: "P", Pos: Vec{1000, 0}},
	}
	pct, gaps := Coverage(sys(g, "A"), g, 100)

	alpha := deg(math.Asin(0.1))
	wantPct := (2 * alpha) / 360
	if !almostEqual(pct, wantPct) {
		t.Errorf("coverage = %v, want %v", pct, wantPct)
	}
	if len(gaps) != 1 {
		t.Fatalf("got %d gaps, want 1", len(gaps))
	}
	if !almostEqual(gaps[0].WidthDeg, 360-2*alpha) {
		t.Errorf("gap width = %v, want %v", gaps[0].WidthDeg, 360-2*alpha)
	}
	if !almostEqual(gaps[0].StartDeg, alpha) || !almostEqual(gaps[0].EndDeg, 360-alpha) {
		t.Errorf("gap span = [%v,%v], want [%v,%v]", gaps[0].StartDeg, gaps[0].EndDeg, alpha, 360-alpha)
	}
}

func TestCoverage_twoSystemsTwoGaps(t *testing.T) {
	// Systems at bearings 0 and 180, both r=1000 -> two symmetric gaps.
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "P", Name: "P", Pos: Vec{1000, 0}},
		{ID: "Q", Name: "Q", Pos: Vec{-1000, 0}},
	}
	pct, gaps := Coverage(sys(g, "A"), g, 100)
	alpha := deg(math.Asin(0.1))
	if !almostEqual(pct, (4*alpha)/360) {
		t.Errorf("coverage = %v, want %v", pct, (4*alpha)/360)
	}
	if len(gaps) != 2 {
		t.Fatalf("got %d gaps, want 2", len(gaps))
	}
	for _, gp := range gaps {
		if !almostEqual(gp.WidthDeg, 180-2*alpha) {
			t.Errorf("gap width = %v, want %v", gp.WidthDeg, 180-2*alpha)
		}
	}
}

func TestCoverage_fullyEnclosed(t *testing.T) {
	// A system within the margin distance blocks a full hemisphere; two opposed
	// such systems cover the entire circle -> no gaps.
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "P", Name: "P", Pos: Vec{50, 0}},
		{ID: "Q", Name: "Q", Pos: Vec{-50, 0}},
	}
	pct, gaps := Coverage(sys(g, "A"), g, 100)
	if len(gaps) != 0 {
		t.Errorf("got %d gaps, want 0 (fully covered)", len(gaps))
	}
	if !almostEqual(pct, 1.0) {
		t.Errorf("coverage = %v, want 1.0", pct)
	}
}
