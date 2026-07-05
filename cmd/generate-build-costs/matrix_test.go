package main

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestBuildMatrix_CheapestAndFeasibleCount(t *testing.T) {
	targets := []buildcost.Target{{
		ID: "widget", Kind: "item",
		BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 2}},
	}}
	books := map[string]*buildcost.Book{
		"st1": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 100}}}, BestBuy: map[string]float64{}},
		"st2": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 4, Qty: 100}}}, BestBuy: map[string]float64{}},
		"st3": {Sell: map[string]buildcost.Ladder{}, BestBuy: map[string]float64{}}, // infeasible
	}
	stations := []StationMeta{{ID: "st1"}, {ID: "st2"}, {ID: "st3"}}
	m := BuildMatrix(targets, books, stations, map[string]string{"widget": "Widget"},
		map[string]string{"widget": "Module"}, nil, nil)
	if len(m.Rows) != 1 {
		t.Fatalf("rows: %d", len(m.Rows))
	}
	r := m.Rows[0]
	if r.CheapestStation != "st2" || !approx(r.CheapestCost, 8) {
		t.Fatalf("cheapest: %s %v", r.CheapestStation, r.CheapestCost)
	}
	if r.FeasibleCount != 2 {
		t.Fatalf("feasible count: %d want 2", r.FeasibleCount)
	}
	if r.Name != "Widget" || r.Category != "Module" {
		t.Fatalf("meta: %+v", r)
	}
}
