package main

import (
	"math"
	"testing"
)

// ray makes an unclear reading from obs pointing at target, with the bearing
// nudged by off degrees.
func ray(obs string, from, target Point, temp, off float64) Reading {
	return Reading{Obs: obs, From: from, S: Sighting{Bearing: bearing(from, target) + off, Temperature: temp}, Status: "unknown"}
}

func TestGroupStars(t *testing.T) {
	star := Point{1000, 500}
	rs := []Reading{
		ray("a", Point{0, 0}, star, 60000, 1),
		ray("b", Point{0, 1000}, star, 60000, -1),
		ray("c", Point{2000, 0}, star, 60000, 0.5),
		ray("d", Point{2000, 1000}, star, 7000, 0), // other temperature: not this star
		ray("e", Point{3000, 3000}, Point{5000, 5000}, 60000, 0),
	}
	fits := groupStars(rs, Limits{FitErr: 3.5, Sigma: 2})
	if len(fits) != 1 {
		t.Fatalf("%d fits, want 1: %+v", len(fits), fits)
	}
	f := fits[0]
	if len(f.Members) != 3 || f.Temp != 60000 {
		t.Errorf("fit has %d members at %v K, want 3 at 60000", len(f.Members), f.Temp)
	}
	if d := math.Hypot(f.At.X-star.X, f.At.Y-star.Y); d > 40 {
		t.Errorf("fit at %+v, %.0f from the true star", f.At, d)
	}
	if f.MaxErr > 3.5 || f.Major <= 0 || f.Minor <= 0 || f.Major < f.Minor {
		t.Errorf("fit errors: max %.1f°, axes %.0f × %.0f", f.MaxErr, f.Major, f.Minor)
	}
}

func TestGroupStarsNearlyParallel(t *testing.T) {
	// Observers close together looking at a far star: the lines agree but
	// the distance is poorly known, so the ellipse is long along the line.
	star := Point{0, -6000}
	rs := []Reading{
		ray("a", Point{0, 0}, star, 60000, 0.5),
		ray("b", Point{400, 0}, star, 60000, -0.5),
		ray("c", Point{800, 100}, star, 60000, 0),
	}
	fits := groupStars(rs, Limits{FitErr: 3.5, Sigma: 2})
	if len(fits) != 1 {
		t.Fatalf("%d fits, want 1", len(fits))
	}
	if f := fits[0]; f.Major < 5*f.Minor {
		t.Errorf("ellipse %.0f × %.0f: want it much longer than wide", f.Major, f.Minor)
	}
}

func TestGroupStarsOnePerObserver(t *testing.T) {
	star := Point{1000, 0}
	rs := []Reading{
		ray("a", Point{0, 0}, star, 60000, 0),
		ray("a", Point{0, 0}, star, 60000, 2), // same observer twice
	}
	if fits := groupStars(rs, Limits{FitErr: 3.5, Sigma: 2}); len(fits) != 0 {
		t.Errorf("one observer alone made %d fits, want 0", len(fits))
	}
}

func TestGroupStarsVisibility(t *testing.T) {
	lim := Limits{FitErr: 3.5, Sigma: 2, MinCross: 10, ClearDist: 2000}
	// A cool star 5000 away from both observers could not have been seen.
	far := Point{0, 5000}
	rs := []Reading{ray("a", Point{-2000, 0}, far, 5000, 0), ray("b", Point{2000, 0}, far, 5000, 0)}
	if fits := groupStars(rs, lim); len(fits) != 0 {
		t.Errorf("cool star 5000 away: %d fits, want 0", len(fits))
	}
	// The same geometry for a hot star is fine.
	rs = []Reading{ray("a", Point{-2000, 0}, far, 60000, 0), ray("b", Point{2000, 0}, far, 60000, 0)}
	if fits := groupStars(rs, lim); len(fits) != 1 {
		t.Errorf("hot star 5000 away: %d fits, want 1", len(fits))
	}
	// A point on top of an observing system is that system, not a new star.
	rs = []Reading{
		ray("a", Point{0, 0}, Point{1005, 0}, 4000, 0),
		{Obs: "b", From: Point{1005, 3}, S: Sighting{Bearing: 270, Temperature: 4000}, Status: "unknown"},
	}
	if fits := groupStars(rs, lim); len(fits) != 0 {
		t.Errorf("star on top of an observer: %d fits, want 0", len(fits))
	}
}
