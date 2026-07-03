package color

import (
	"math"
	"testing"
)

func TestSplineEvalAtKnots(t *testing.T) {
	s := Spline{Knots: []SplineKnot{
		{Input: 0.0, Output: 0.0},
		{Input: 0.5, Output: 0.4},
		{Input: 1.0, Output: 1.0},
	}}
	tests := []struct{ in, want float64 }{
		{0.0, 0.0}, {0.5, 0.4}, {1.0, 1.0},
	}
	for _, tc := range tests {
		got := EvalSpline(s, tc.in)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("EvalSpline(%v) = %f, want %f", tc.in, got, tc.want)
		}
	}
}

func TestSplineEvalClampsDomain(t *testing.T) {
	s := Spline{Knots: []SplineKnot{
		{Input: 0.2, Output: 0.5},
		{Input: 0.8, Output: 0.7},
	}}
	if got := EvalSpline(s, -1); got != 0.5 {
		t.Errorf("EvalSpline(-1) = %f, want 0.5 (clamped to first knot)", got)
	}
	if got := EvalSpline(s, 5); got != 0.7 {
		t.Errorf("EvalSpline(5) = %f, want 0.7 (clamped to last knot)", got)
	}
}

func TestSplineMonotonic(t *testing.T) {
	// Strictly increasing knots → output is non-decreasing across the
	// domain. Fritsch-Carlson guarantees this.
	s := Spline{Knots: []SplineKnot{
		{Input: 0.0, Output: 0.0},
		{Input: 0.3, Output: 0.1},
		{Input: 0.6, Output: 0.2},
		{Input: 1.0, Output: 1.0},
	}}
	prev := EvalSpline(s, 0)
	for i := 1; i <= 100; i++ {
		x := float64(i) / 100
		y := EvalSpline(s, x)
		if y < prev-1e-9 {
			t.Errorf("non-monotonic at x=%f: y=%f < prev=%f", x, y, prev)
		}
		prev = y
	}
}

func TestSplineEmptyKnots(t *testing.T) {
	s := Spline{}
	if got := EvalSpline(s, 0.5); got != 0 {
		t.Errorf("empty spline returned %f, want 0", got)
	}
}
