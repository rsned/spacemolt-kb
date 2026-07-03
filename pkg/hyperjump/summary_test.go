package hyperjump

import "testing"

func TestSummarize(t *testing.T) {
	// Three collinear systems: X(0) - Y(100) - Z(200) on the +X axis.
	g := []System{
		{ID: "X", Name: "X", Pos: Vec{0, 0}},
		{ID: "Y", Name: "Y", Pos: Vec{100, 0}},
		{ID: "Z", Name: "Z", Pos: Vec{200, 0}},
	}
	s := Summarize(Analyze(g, 100))

	if s.Systems != 3 {
		t.Errorf("Systems = %d, want 3", s.Systems)
	}
	if s.DirectedPairs != 6 {
		t.Errorf("DirectedPairs = %d, want 6", s.DirectedPairs)
	}
	// Reachable: X->Y, Y->X, Y->Z, Z->Y. Blocked: X->Z, Z->X (Y is between).
	if s.Reachable != 4 {
		t.Errorf("Reachable = %d, want 4", s.Reachable)
	}
	if s.Blocked != 2 {
		t.Errorf("Blocked = %d, want 2", s.Blocked)
	}
	// X and Z each have a 180deg void gap; Y is fully enclosed (no gap).
	if s.SystemsWithEscape != 2 {
		t.Errorf("SystemsWithEscape = %d, want 2", s.SystemsWithEscape)
	}
	// Every reachable pair here is unconstrained at distance 100 -> margin 90.
	if !almostEqual(s.MinMargin, 90) || !almostEqual(s.MedianMargin, 90) || !almostEqual(s.MaxMargin, 90) {
		t.Errorf("margins = (min=%v median=%v max=%v), want all 90", s.MinMargin, s.MedianMargin, s.MaxMargin)
	}
}
