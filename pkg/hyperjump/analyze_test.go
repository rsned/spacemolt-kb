package hyperjump

import (
	"math"
	"reflect"
	"testing"
)

func deg(rad float64) float64 { return rad * 180 / math.Pi }

// A small hand-built galaxy laid out on the +X axis from origin A.
//
//	A(0,0)  E(120,50)  ... B(200,0), with C(100,0) on the line and D(100,150) far off.
func testGalaxy() []System {
	return []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "B", Name: "B", Pos: Vec{200, 0}},
		{ID: "C", Name: "C", Pos: Vec{100, 0}},   // exactly on A->B line
		{ID: "D", Name: "D", Pos: Vec{100, 150}},  // perp 150 > margin
		{ID: "E", Name: "E", Pos: Vec{120, 50}},   // proj 120, perp 50 <= margin
	}
}

func sys(g []System, id string) System {
	for _, s := range g {
		if s.ID == id {
			return s
		}
	}
	panic("no system " + id)
}

func TestInterrupters_blockedPath(t *testing.T) {
	g := testGalaxy()
	// A -> B: C (proj 100) and E (proj 120) both within margin and closer than B.
	got := Interrupters(sys(g, "A"), sys(g, "B"), g, 100)
	want := []string{"C", "E"} // nearest-first by proj
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Interrupters(A->B) = %v, want %v", got, want)
	}
}

func TestInterrupters_cleanPath(t *testing.T) {
	g := testGalaxy()
	// A -> C: nothing is both in-corridor and closer than C.
	got := Interrupters(sys(g, "A"), sys(g, "C"), g, 100)
	if len(got) != 0 {
		t.Errorf("Interrupters(A->C) = %v, want none", got)
	}
}

func TestInterrupters_marginBoundaryInclusive(t *testing.T) {
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "B", Name: "B", Pos: Vec{200, 0}},
		{ID: "edge", Name: "edge", Pos: Vec{100, 100}},  // perp exactly 100 -> lands
		{ID: "miss", Name: "miss", Pos: Vec{100, 100.1}}, // perp 100.1 -> misses
	}
	got := Interrupters(sys(g, "A"), sys(g, "B"), g, 100)
	want := []string{"edge"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("boundary: got %v, want %v", got, want)
	}
}

func TestAngularMargin_destinationOnly(t *testing.T) {
	// Only A and B(1000,0): margin is bounded solely by B leaving its corridor.
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "B", Name: "B", Pos: Vec{1000, 0}},
	}
	m, left, right := AngularMargin(sys(g, "A"), sys(g, "B"), g, 100)
	want := deg(math.Asin(100.0 / 1000.0)) // ~5.739 deg
	if !almostEqual(m, want) || !almostEqual(left, want) || !almostEqual(right, want) {
		t.Errorf("AngularMargin = (m=%v,left=%v,right=%v), want all %v", m, left, right, want)
	}
}

func TestAngularMargin_constrainedBySide(t *testing.T) {
	// B due east at 1000. A closer system C sits so its corridor near-edge is
	// exactly 2deg CCW (left) of the A->B bearing, tightening that side to 2deg.
	alpha := deg(math.Asin(100.0 / 500.0)) // C's arc half-width at r=500
	phiC := (2.0 + alpha) * math.Pi / 180
	cx, cy := 500*math.Cos(phiC), 500*math.Sin(phiC)
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "B", Name: "B", Pos: Vec{1000, 0}},
		{ID: "C", Name: "C", Pos: Vec{cx, cy}},
	}
	m, left, right := AngularMargin(sys(g, "A"), sys(g, "B"), g, 100)
	wantCCW := 2.0
	wantCW := deg(math.Asin(100.0 / 1000.0)) // unconstrained on the CW side -> dest limit
	if !almostEqual(right, wantCCW) {
		t.Errorf("CCW margin (right) = %v, want %v", right, wantCCW)
	}
	if !almostEqual(left, wantCW) {
		t.Errorf("CW margin (left) = %v, want %v", left, wantCW)
	}
	if !almostEqual(m, wantCCW) {
		t.Errorf("AngularMargin = %v, want %v", m, wantCCW)
	}
}

func TestClearance(t *testing.T) {
	alpha := deg(math.Asin(100.0 / 500.0))
	phiC := (2.0 + alpha) * math.Pi / 180
	cx, cy := 500*math.Cos(phiC), 500*math.Sin(phiC)
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "B", Name: "B", Pos: Vec{1000, 0}},
		{ID: "C", Name: "C", Pos: Vec{cx, cy}},
	}
	got := Clearance(sys(g, "A"), sys(g, "B"), g)
	want := 500 * math.Sin(phiC) // perpendicular distance of C from the A->B ray
	if !almostEqual(got, want) {
		t.Errorf("Clearance = %v, want %v", got, want)
	}
}
