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

func TestBuildMatrix_RecipeFeasibleCount(t *testing.T) {
	// gadget's BoM needs cobalt, which no station stocks (BoM infeasible
	// everywhere). Its recipe needs copper, which only st1 stocks, so Recipe
	// mode is feasible at exactly one station.
	targets := []buildcost.Target{{
		ID:   "gadget",
		Kind: "item",
		BoM:  []buildcost.Requirement{{ItemID: "cobalt", Qty: 1}},
		Recipes: []buildcost.Recipe{
			{ID: "recG", OutputQty: 1, Inputs: []buildcost.Requirement{{ItemID: "copper", Qty: 2}}},
		},
	}}
	books := map[string]*buildcost.Book{
		"st1": {Sell: map[string]buildcost.Ladder{"copper": {{Price: 5, Qty: 50}}}, BestBuy: map[string]float64{}},
		"st2": {Sell: map[string]buildcost.Ladder{}, BestBuy: map[string]float64{}},
	}
	stations := []StationMeta{{ID: "st1"}, {ID: "st2"}}
	m := BuildMatrix(targets, books, stations, map[string]string{"gadget": "Gadget"},
		map[string]string{"gadget": "Module"}, nil, nil)
	if len(m.Rows) != 1 {
		t.Fatalf("rows: %d", len(m.Rows))
	}
	r := m.Rows[0]
	if r.FeasibleCount != 0 {
		t.Fatalf("feasible count: %d want 0", r.FeasibleCount)
	}
	if r.RecipeFeasibleCount != 1 {
		t.Fatalf("recipe feasible count: %d want 1", r.RecipeFeasibleCount)
	}
}
