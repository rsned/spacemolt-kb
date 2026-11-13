package main

import (
	"math"
	"strconv"
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

func TestAnalyze(t *testing.T) {
	pos := map[string]Point{"a": {0, 0}, "b": {1000, 0}}
	stars := []Star{
		{System: "a", At: Point{0, 0}, Lo: 2400, Hi: 3700},
		{System: "b", At: Point{1000, 0}, Lo: 2400, Hi: 3700},
		{System: "far", At: Point{0, -9000}, Lo: 5200, Hi: 6000},
		{System: "hot", Class: "WR", At: Point{0, 9000}, Lo: 20000, Hi: 210000},
	}
	obs := []Observation{
		{System: "a", Stars: []Sighting{
			{Bearing: 0, Temperature: 3000},   // b, clear
			{Bearing: 45, Temperature: 7000},  // nothing: unknown
			{Bearing: 270, Temperature: 6000}, // an ordinary star 9000 away: doubtful
			{Bearing: 90, Temperature: 60000}, // a Wolf-Rayet 9000 away: fine
		}},
		{System: "b", Stars: []Sighting{{Bearing: 135, Temperature: 7000}}},
		{System: "nowhere", Stars: []Sighting{{Bearing: 0, Temperature: 3000}}},
	}
	rs, xs := analyze(obs, pos, stars, Limits{Tol: 5, ClearErr: 3, ClearDist: 5000, FitErr: 3.5, Sigma: 2, MinCross: 10})
	got := map[string]string{}
	for _, r := range rs {
		got[r.Obs+"/"+fmtF(r.S.Bearing)] = r.Status
	}
	want := map[string]string{"a/0": "match", "a/45": "unknown", "a/270": "doubtful", "a/90": "match", "b/135": "unknown"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s status = %q, want %q", k, got[k], v)
		}
	}
	if len(rs) != 5 {
		t.Errorf("%d readings, want 5 (the system with no position is skipped)", len(rs))
	}
	if len(xs) != 1 || math.Abs(xs[0].At.X-500) > 1e-6 || math.Abs(xs[0].At.Y-500) > 1e-6 {
		t.Fatalf("fits = %+v, want one at (500,500)", xs)
	}
	if xs[0].Major <= 0 {
		t.Errorf("fit has no error ellipse: %+v", xs[0])
	}
}

func fmtF(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func TestAnalyzeOneStarPerSighting(t *testing.T) {
	pos := map[string]Point{"o": {0, 0}}
	stars := []Star{{System: "s", At: Point{1000, 140}, Lo: 20000, Hi: 210000}} // bearing ≈ 8°
	obs := []Observation{{System: "o", Stars: []Sighting{{Bearing: 10, Temperature: 60000}, {Bearing: 5, Temperature: 60000}}}}
	rs, _ := analyze(obs, pos, stars, Limits{Tol: 6, ClearErr: 3})
	matched := 0
	for _, r := range rs {
		if r.Match != nil {
			matched++
		}
	}
	if matched != 1 {
		t.Errorf("%d sightings matched the one star, want 1", matched)
	}
	if rs[0].Status != "match" || rs[1].Status != "unknown" {
		t.Errorf("statuses = %s, %s; want the closer bearing (10°, 2° off) to win", rs[0].Status, rs[1].Status)
	}
}

func TestAnalyzePrefersPlausible(t *testing.T) {
	pos := map[string]Point{"o": {0, 0}}
	stars := []Star{
		{System: "near", At: Point{300, 10}, Lo: 6000, Hi: 7500}, // ≈1.9° off, 300 away
		{System: "far", At: Point{9000, 0}, Lo: 6000, Hi: 7500},  // 0° off, too far for a cool star
	}
	obs := []Observation{{System: "o", Stars: []Sighting{{Bearing: 0, Temperature: 6000}}}}
	rs, _ := analyze(obs, pos, stars, Limits{Tol: 6, ClearErr: 3, ClearDist: 2000})
	if rs[0].Match == nil || rs[0].Match.Star.System != "near" || rs[0].Status != "match" {
		t.Errorf("got %+v (%s), want a clear match to near", rs[0].Match, rs[0].Status)
	}
}

func TestTooFar(t *testing.T) {
	lim := Limits{ClearDist: 2000, DwarfDist: 2500}
	cases := []struct {
		class string
		d     float64
		want  bool
	}{
		{"WR", 9000, false}, {"O7I", 9000, false},
		{"DA", 2000, false}, {"DA", 8000, true},
		{"G2V", 1500, false}, {"G2V", 8000, true},
	}
	for _, c := range cases {
		if got := lim.tooFar(Match{Star: Star{Class: c.class}, Distance: c.d}); got != c.want {
			t.Errorf("tooFar(%s at %v) = %v, want %v", c.class, c.d, got, c.want)
		}
	}
}
