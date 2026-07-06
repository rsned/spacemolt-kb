package buildcost

import (
	"reflect"
	"testing"
)

func TestMinStationCover_SingleStationCoversAll(t *testing.T) {
	reqs := []Requirement{{ItemID: "iron", Qty: 5}, {ItemID: "copper", Qty: 3}}
	depth := StationDepth{
		"A": {"iron": 10, "copper": 10},
		"B": {"iron": 1},
	}
	got := MinStationCover(reqs, depth, []string{"A", "B"}, 3)
	if !got.Feasible || got.Count != 1 || !got.Exact {
		t.Fatalf("got %+v, want feasible count=1 exact", got)
	}
	if !reflect.DeepEqual(got.Stations, []string{"A"}) {
		t.Fatalf("stations = %v, want [A]", got.Stations)
	}
}

func TestMinStationCover_NeedsTwoStations(t *testing.T) {
	// iron only fully covered by A+B together; copper only by C... but B has copper too.
	reqs := []Requirement{{ItemID: "iron", Qty: 10}, {ItemID: "copper", Qty: 4}}
	depth := StationDepth{
		"A": {"iron": 6},
		"B": {"iron": 6, "copper": 4},
	}
	got := MinStationCover(reqs, depth, []string{"A", "B"}, 3)
	if !got.Feasible || got.Count != 2 || !got.Exact {
		t.Fatalf("got %+v, want feasible count=2 exact", got)
	}
	if !reflect.DeepEqual(got.Stations, []string{"A", "B"}) {
		t.Fatalf("stations = %v, want [A B]", got.Stations)
	}
}

func TestMinStationCover_InfeasibleListsMissing(t *testing.T) {
	reqs := []Requirement{{ItemID: "iron", Qty: 10}, {ItemID: "unobtainium", Qty: 1}}
	depth := StationDepth{"A": {"iron": 10}}
	got := MinStationCover(reqs, depth, []string{"A"}, 3)
	if got.Feasible {
		t.Fatalf("expected infeasible, got %+v", got)
	}
	if !reflect.DeepEqual(got.Missing, []string{"unobtainium"}) {
		t.Fatalf("missing = %v, want [unobtainium]", got.Missing)
	}
}

func TestMinStationCover_GreedyFallbackBeyondExactK(t *testing.T) {
	// Four inputs, each only at its own station with exactly enough depth →
	// the unique cover is 4 stations. With exactK=2 the exact search cannot
	// find it, so greedy returns a valid 4-station (non-exact) cover.
	reqs := []Requirement{
		{ItemID: "a", Qty: 1}, {ItemID: "b", Qty: 1},
		{ItemID: "c", Qty: 1}, {ItemID: "d", Qty: 1},
	}
	depth := StationDepth{
		"S1": {"a": 1}, "S2": {"b": 1}, "S3": {"c": 1}, "S4": {"d": 1},
	}
	got := MinStationCover(reqs, depth, []string{"S1", "S2", "S3", "S4"}, 2)
	if !got.Feasible || got.Count != 4 || got.Exact {
		t.Fatalf("got %+v, want feasible count=4 non-exact", got)
	}
	if !reflect.DeepEqual(got.Stations, []string{"S1", "S2", "S3", "S4"}) {
		t.Fatalf("stations = %v, want all four", got.Stations)
	}
}

func TestMinStationCover_DeterministicFirstCombo(t *testing.T) {
	// A alone and B alone both cover → the lexicographically first (A) wins.
	reqs := []Requirement{{ItemID: "iron", Qty: 1}}
	depth := StationDepth{"B": {"iron": 5}, "A": {"iron": 5}}
	got := MinStationCover(reqs, depth, []string{"A", "B"}, 3)
	if got.Count != 1 || !reflect.DeepEqual(got.Stations, []string{"A"}) {
		t.Fatalf("got %+v, want count=1 station [A]", got)
	}
}

func TestMinStationCover_BatchIncreasesCount(t *testing.T) {
	// At qty 1, A alone covers iron. At qty 12, A(10)+B(5) needed.
	depth := StationDepth{"A": {"iron": 10}, "B": {"iron": 5}}
	one := MinStationCover([]Requirement{{ItemID: "iron", Qty: 1}}, depth, []string{"A", "B"}, 3)
	ten := MinStationCover([]Requirement{{ItemID: "iron", Qty: 12}}, depth, []string{"A", "B"}, 3)
	if one.Count != 1 || ten.Count != 2 {
		t.Fatalf("one=%d ten=%d, want 1 and 2", one.Count, ten.Count)
	}
}

func TestMinStationCover_EmptyReqs(t *testing.T) {
	got := MinStationCover(nil, StationDepth{"A": {}}, []string{"A"}, 3)
	if !got.Feasible || got.Count != 0 || !got.Exact {
		t.Fatalf("got %+v, want feasible count=0 exact", got)
	}
}
