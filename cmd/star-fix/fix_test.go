package main

import (
	"math"
	"testing"
)

func TestBearing(t *testing.T) {
	// 0° is east (+x); angles grow toward +y, which is down on the map.
	cases := []struct {
		dx, dy, want float64
	}{
		{1, 0, 0}, {0, 1, 90}, {-1, 0, 180}, {0, -1, 270}, {1, -1, 315},
	}
	for _, c := range cases {
		if got := bearing(Point{0, 0}, Point{c.dx, c.dy}); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("bearing to (%v,%v) = %v, want %v", c.dx, c.dy, got, c.want)
		}
	}
}

func TestAngleDiff(t *testing.T) {
	if d := angleDiff(359, 2); math.Abs(d-3) > 1e-9 {
		t.Errorf("angleDiff(359,2) = %v, want 3", d)
	}
}

func TestParseTempRange(t *testing.T) {
	cases := []struct {
		in     string
		lo, hi float64
	}{
		{"7,500–10,000 K", 7500, 10000},
		{">30,000 K", 30000, math.Inf(1)},
		{"Singularity", 0, 0},
	}
	for _, c := range cases {
		lo, hi := parseTempRange(c.in)
		if lo != c.lo || hi != c.hi {
			t.Errorf("parseTempRange(%q) = %v,%v, want %v,%v", c.in, lo, hi, c.lo, c.hi)
		}
	}
}

func TestMatchSighting(t *testing.T) {
	obs := Point{0, 0}
	stars := []Star{
		{System: "near_m", At: Point{100, 0}, Lo: 2400, Hi: 3700},
		{System: "far_m", At: Point{500, 2}, Lo: 2400, Hi: 3700},
		{System: "hot", At: Point{0, 300}, Lo: 20000, Hi: 210000},
	}
	m, ok := matchSighting(obs, "here", Sighting{Bearing: 1, Temperature: 3000}, stars, 5)
	if !ok || m.Star.System != "near_m" {
		t.Fatalf("east 3000 K = %+v %v, want near_m", m, ok)
	}
	if _, ok := matchSighting(obs, "here", Sighting{Bearing: 90, Temperature: 3000}, stars, 5); ok {
		t.Error("south 3000 K matched the hot star; temperature must rule it out")
	}
	if m, ok := matchSighting(obs, "here", Sighting{Bearing: 92, Temperature: 60000}, stars, 5); !ok || m.Star.System != "hot" {
		t.Errorf("south 60000 K = %+v %v, want hot", m, ok)
	}
}

func TestIntersect(t *testing.T) {
	// From (0,0) heading 45° and from (10,0) heading 135°, the rays cross at (5,5).
	p, ok := intersect(Point{0, 0}, 45, Point{10, 0}, 135)
	if !ok || math.Abs(p.X-5) > 1e-9 || math.Abs(p.Y-5) > 1e-9 {
		t.Fatalf("intersect = %+v %v, want (5,5)", p, ok)
	}
	// Rays pointing away from each other never meet ahead of both observers.
	if _, ok := intersect(Point{0, 0}, 225, Point{10, 0}, 315); ok {
		t.Error("diverging rays reported a crossing")
	}
	if _, ok := intersect(Point{0, 0}, 0, Point{0, 5}, 0); ok {
		t.Error("parallel rays reported a crossing")
	}
}
