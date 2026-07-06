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
	m := BuildMatrix(targets, books, books, stations, map[string]string{"widget": "Widget"},
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

// TestBuildMatrix_MarginGatedOnBoMFeasibility proves that no savings/profit is
// advertised on an infeasible BoM cell, even when a finished-good ask/bid
// exists. "widget"'s BoM needs "unobtainium", which no station stocks, so the
// BoM cost is only a partial sum (0, since nothing was purchasable) — a
// savings/profit figure computed off that partial cost would be misleading.
func TestBuildMatrix_MarginGatedOnBoMFeasibility(t *testing.T) {
	targets := []buildcost.Target{{
		ID: "widget", Kind: "item",
		BoM: []buildcost.Requirement{{ItemID: "unobtainium", Qty: 1}},
	}}
	books := map[string]*buildcost.Book{
		"st1": {
			Sell:    map[string]buildcost.Ladder{"widget": {{Price: 50, Qty: 100}}},
			BestBuy: map[string]float64{"widget": 40},
		},
	}
	stations := []StationMeta{{ID: "st1"}}
	m := BuildMatrix(targets, books, books, stations, map[string]string{"widget": "Widget"},
		map[string]string{"widget": "Module"}, nil, nil)
	if len(m.Rows) != 1 {
		t.Fatalf("rows: %d", len(m.Rows))
	}
	c, ok := m.Rows[0].Cells["st1"]
	if !ok {
		t.Fatalf("missing cell for st1")
	}
	if c.BoMFeasible {
		t.Fatalf("expected BoM infeasible (unobtainium stocked nowhere), got feasible")
	}
	if !approx(c.BoMCost, 0) {
		t.Fatalf("BoM cost: %v want 0 (nothing purchasable)", c.BoMCost)
	}
	// A finished ask (50) and bid (40) both exist, so without the feasibility
	// gate SavingsVsAsk(0)=50/HasSavings and ProfitVsBid(0)=40/HasProfit would
	// both come back true — a misleading margin on an uncompletable build.
	if c.HasSavings {
		t.Fatalf("HasSavings: got true, want false (BoM infeasible, cost is only a partial sum)")
	}
	if c.SavingsBoM != 0 {
		t.Fatalf("SavingsBoM: got %v, want 0 (zero value, unset)", c.SavingsBoM)
	}
	if c.HasProfit {
		t.Fatalf("HasProfit: got true, want false (BoM infeasible, cost is only a partial sum)")
	}
	if c.ProfitBoM != 0 {
		t.Fatalf("ProfitBoM: got %v, want 0 (zero value, unset)", c.ProfitBoM)
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
	m := BuildMatrix(targets, books, books, stations, map[string]string{"gadget": "Gadget"},
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

func TestBuildMatrix_MarginUsesMarginBook(t *testing.T) {
	// Target 'widget' needs 1 iron. Cost book has cheap iron AND (deliberately)
	// no finished 'widget' ask. Margin book carries the finished 'widget' ask.
	// Savings must come from the margin book, proving the two are separate.
	target := buildcost.Target{
		ID: "widget", Kind: "item",
		BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 1}},
	}
	costBooks := map[string]*buildcost.Book{
		"S": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 1}}}, BestBuy: map[string]float64{}},
	}
	marginBooks := map[string]*buildcost.Book{
		"S": {Sell: map[string]buildcost.Ladder{"widget": {{Price: 30, Qty: 1}}}, BestBuy: map[string]float64{}},
	}
	stations := []StationMeta{{ID: "S", Name: "S", Empire: "Independent"}}
	m := BuildMatrix([]buildcost.Target{target}, costBooks, marginBooks, stations,
		map[string]string{"widget": "Widget"}, map[string]string{"widget": "component"},
		map[string]map[string]float64{}, map[string]int{})
	if len(m.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(m.Rows))
	}
	c := m.Rows[0].Cells["S"]
	if c.BoMCost != 10 || !c.BoMFeasible {
		t.Fatalf("BoM cost/feasible = %v/%v, want 10/true", c.BoMCost, c.BoMFeasible)
	}
	// Savings = finished ask (30, from marginBooks) - cost (10) = 20.
	if !c.HasSavings || c.SavingsBoM != 20 {
		t.Errorf("SavingsBoM = %v (has=%v), want 20 — margin must come from marginBooks", c.SavingsBoM, c.HasSavings)
	}
}
