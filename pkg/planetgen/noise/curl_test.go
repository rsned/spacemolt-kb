package noise

import (
	"math"
	"testing"
)

func TestCurlBackwardTraceIdentityAtZeroAmp(t *testing.T) {
	g := NewCurlGen(123)
	x, y, z := 0.5, 0.5, math.Sqrt(0.5)
	tx, ty, tz := g.BackwardTrace(x, y, z, 0, 8, 0.1, 1.0, 0)
	if tx != x || ty != y || tz != z {
		t.Errorf("Amp=0 should be identity; got (%f,%f,%f) want (%f,%f,%f)",
			tx, ty, tz, x, y, z)
	}
}

func TestCurlDeterministic(t *testing.T) {
	g := NewCurlGen(7)
	a := [3]float64{}
	a[0], a[1], a[2] = g.BackwardTrace(0.3, 0.4, 0.5, 0.1, 10, 0.05, 2.0, 0.1)
	g2 := NewCurlGen(7)
	b := [3]float64{}
	b[0], b[1], b[2] = g2.BackwardTrace(0.3, 0.4, 0.5, 0.1, 10, 0.05, 2.0, 0.1)
	if a != b {
		t.Errorf("non-deterministic; %v vs %v", a, b)
	}
}
