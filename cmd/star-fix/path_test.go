package main

import "testing"

func TestPathLanding(t *testing.T) {
	systems := []System{
		{ID: "o", At: Point{0, 0}},
		{ID: "near", At: Point{500, 60}}, // 60 off the 0° line: inside the 100 margin
		{ID: "far", At: Point{900, 0}},
		{ID: "wide", At: Point{300, 300}}, // well off the line
	}
	if id, d := pathLanding(Point{0, 0}, "o", 0, systems); id != "near" || d != 500 {
		t.Errorf("0° lands at %q after %.0f, want near after 500", id, d)
	}
	if id, _ := pathLanding(Point{0, 0}, "o", 180, systems); id != "" {
		t.Errorf("180° lands at %q, want the void", id)
	}
}

func TestJumpCheck(t *testing.T) {
	systems := []System{{ID: "o", At: Point{0, 0}}, {ID: "block", At: Point{400, 0}}}
	f := Fit{At: Point{1000, 0}, Members: []Reading{{Obs: "o", From: Point{0, 0}}}}
	js := jumpChecks(f, systems)
	if len(js) != 1 || js[0].Blocker != "block" || js[0].Clear {
		t.Fatalf("got %+v, want blocked by block", js)
	}
	f.At = Point{0, 1000}
	js = jumpChecks(f, systems)
	if !js[0].Clear || js[0].Heading != 90 {
		t.Errorf("got %+v, want a clear jump at 90°", js[0])
	}
	if js[0].Tol < 5.6 || js[0].Tol > 5.8 { // atan(100/1000)
		t.Errorf("tolerance %.2f°, want about 5.7°", js[0].Tol)
	}
}
