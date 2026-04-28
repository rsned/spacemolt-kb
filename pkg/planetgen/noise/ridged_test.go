package noise

import (
	"math"
	"testing"
)

func TestRidgedRange(t *testing.T) {
	g := New(42)
	min, max := math.Inf(1), math.Inf(-1)
	const N = 1024
	for i := range N {
		theta := float64(i) * 2 * math.Pi / float64(N)
		v := g.RidgedFractal3D(math.Cos(theta), math.Sin(theta), 0, 4, 2.0, 0.5, 1.0)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite ridged value at i=%d: %v", i, v)
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	if min < 0 {
		t.Errorf("ridged should be non-negative, got min=%v", min)
	}
	// Four octaves with gain=0.5, offset=1 has theoretical peak around 1.875.
	// Real samples don't all hit the |noise|=0 ridge so we expect the peak
	// to be at least 0.3 across 1024 samples on a great circle.
	if max < 0.3 {
		t.Errorf("ridged peak suspiciously low: %v", max)
	}
}

func TestRidgedDeterministic(t *testing.T) {
	a := New(1).RidgedFractal3D(0.3, 0.7, 0.1, 5, 2.0, 0.5, 1.0)
	b := New(1).RidgedFractal3D(0.3, 0.7, 0.1, 5, 2.0, 0.5, 1.0)
	if a != b {
		t.Errorf("ridged not deterministic: %v vs %v", a, b)
	}
}

func TestRidgedZeroOctaves(t *testing.T) {
	got := New(1).RidgedFractal3D(0, 0, 0, 0, 2.0, 0.5, 1.0)
	if got != 0 {
		t.Errorf("octaves=0 should return 0, got %v", got)
	}
}
