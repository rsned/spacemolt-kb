package jumpmap

import "testing"

func TestDisplayBlockedPct(t *testing.T) {
	cases := []struct {
		blocked float64
		gaps    int
		want    float64
	}{
		{100.0, 0, 100.0},    // truly sealed -> 100 is honest
		{99.98689, 1, 99.9},  // Alpha Centauri: would round to 100.0, cap it
		{99.95, 1, 99.9},     // rounds to 100.0 without the cap
		{99.9, 1, 99.9},      // already at the cap
		{50.0, 3, 50.0},      // well below the cap, unchanged
		{99.4, 2, 99.4},      // below cap, unchanged
	}
	for _, c := range cases {
		if got := DisplayBlockedPct(c.blocked, c.gaps); got != c.want {
			t.Errorf("DisplayBlockedPct(%v, %d) = %v, want %v", c.blocked, c.gaps, got, c.want)
		}
	}
}
