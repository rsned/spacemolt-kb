package hyperjump

import (
	"reflect"
	"testing"
)

func findReport(reports []OriginReport, id string) OriginReport {
	for _, r := range reports {
		if r.System == id {
			return r
		}
	}
	panic("no report for " + id)
}

func findPair(r OriginReport, to string) Pair {
	for _, p := range r.Pairs {
		if p.To == to {
			return p
		}
	}
	panic("no pair to " + to)
}

func TestAnalyze(t *testing.T) {
	g := testGalaxy()
	reports := Analyze(g, 100)

	if len(reports) != len(g) {
		t.Fatalf("got %d reports, want %d", len(reports), len(g))
	}

	ra := findReport(reports, "A")
	if len(ra.Pairs) != len(g)-1 {
		t.Errorf("origin A has %d pairs, want %d", len(ra.Pairs), len(g)-1)
	}

	// A -> B is blocked; lands at C; interrupters nearest-first [C, E].
	ab := findPair(ra, "B")
	if ab.Reachable {
		t.Errorf("A->B should be blocked")
	}
	if ab.LandsAt != "C" {
		t.Errorf("A->B LandsAt = %q, want C", ab.LandsAt)
	}
	if !reflect.DeepEqual(ab.Interrupters, []string{"C", "E"}) {
		t.Errorf("A->B interrupters = %v, want [C E]", ab.Interrupters)
	}

	// A -> C is clean; lands at C; positive margin.
	ac := findPair(ra, "C")
	if !ac.Reachable {
		t.Errorf("A->C should be reachable")
	}
	if ac.LandsAt != "C" {
		t.Errorf("A->C LandsAt = %q, want C", ac.LandsAt)
	}
	if ac.AngularMargin <= 0 {
		t.Errorf("A->C AngularMargin = %v, want > 0", ac.AngularMargin)
	}
	if !almostEqual(ac.Bearing, 0) {
		t.Errorf("A->C bearing = %v, want 0", ac.Bearing)
	}
}

func TestAnalyze_destHasStation(t *testing.T) {
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "B", Name: "B", Pos: Vec{200, 0}, HasStation: true},
		{ID: "C", Name: "C", Pos: Vec{100, 0}}, // no station
	}
	reports := Analyze(g, 100)
	ra := findReport(reports, "A")
	if !findPair(ra, "B").DestHasStation {
		t.Errorf("A->B DestHasStation = false, want true")
	}
	if findPair(ra, "C").DestHasStation {
		t.Errorf("A->C DestHasStation = true, want false")
	}
}

func TestFilterStationDestinations(t *testing.T) {
	g := []System{
		{ID: "A", Name: "A", Pos: Vec{0, 0}},
		{ID: "B", Name: "B", Pos: Vec{200, 0}, HasStation: true}, // only station
		{ID: "C", Name: "C", Pos: Vec{100, 0}},
	}
	filtered := FilterStationDestinations(Analyze(g, 100))

	// Only origins with at least one station destination are kept: A and C
	// (each has one pair, to B). B itself has no station destinations.
	if len(filtered) != 2 {
		t.Fatalf("got %d filtered reports, want 2", len(filtered))
	}
	for _, r := range filtered {
		if r.System == "B" {
			t.Errorf("B should be dropped (no station destinations)")
		}
		if len(r.Pairs) != 1 {
			t.Errorf("origin %s has %d pairs, want 1", r.System, len(r.Pairs))
		}
		for _, p := range r.Pairs {
			if !p.DestHasStation {
				t.Errorf("origin %s pair to %s is not a station destination", r.System, p.To)
			}
		}
	}
}
