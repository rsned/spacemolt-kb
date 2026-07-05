package buildcost

import "testing"

func TestCheapestRecipe_PicksCheapestFeasible(t *testing.T) {
	b := bookFixture()
	// Recipe A: 5 iron => 50. Recipe B: 4 copper => 20 (cheaper, feasible).
	recipes := []Recipe{
		{ID: "recA", OutputQty: 1, Inputs: []Requirement{{"iron", 5}}},
		{ID: "recB", OutputQty: 1, Inputs: []Requirement{{"copper", 4}}},
	}
	got := b.CheapestRecipe(recipes)
	if !got.Feasible || got.RecipeID != "recB" || !approx(got.Cost, 20) {
		t.Fatalf("cheapest feasible: got %+v want recB cost 20", got)
	}
}

func TestCheapestRecipe_PrefersFeasibleOverCheaperInfeasible(t *testing.T) {
	b := bookFixture()
	// recCheapButShort needs 5 gold (only 1 available) -> infeasible though nominally cheap.
	// recFeasible needs 5 iron -> feasible at 50.
	recipes := []Recipe{
		{ID: "recCheapButShort", OutputQty: 1, Inputs: []Requirement{{"gold", 5}}},
		{ID: "recFeasible", OutputQty: 1, Inputs: []Requirement{{"iron", 5}}},
	}
	got := b.CheapestRecipe(recipes)
	if !got.Feasible || got.RecipeID != "recFeasible" || !approx(got.Cost, 50) {
		t.Fatalf("prefer feasible: got %+v want recFeasible cost 50", got)
	}
}

func TestCheapestRecipe_OutputQtyScales(t *testing.T) {
	b := bookFixture()
	// 4 copper => 20 total, but recipe yields 2 units => per-unit cost 10.
	recipes := []Recipe{{ID: "rec2", OutputQty: 2, Inputs: []Requirement{{"copper", 4}}}}
	got := b.CheapestRecipe(recipes)
	if !approx(got.Cost, 10) {
		t.Fatalf("output scaling: got %v want 10", got.Cost)
	}
}

func TestCheapestRecipe_AllInfeasibleReturnsLowestPartial(t *testing.T) {
	b := bookFixture()
	recipes := []Recipe{
		{ID: "r1", OutputQty: 1, Inputs: []Requirement{{"gold", 5}}}, // partial 50
		{ID: "r2", OutputQty: 1, Inputs: []Requirement{{"gold", 10}}}, // partial 50 too, more short
	}
	got := b.CheapestRecipe(recipes)
	if got.Feasible {
		t.Fatalf("expected infeasible, got %+v", got)
	}
	if got.RecipeID == "" {
		t.Fatalf("expected a chosen recipe id even when infeasible")
	}
}

func TestCheapestRecipe_EmptyIsNA(t *testing.T) {
	b := bookFixture()
	got := b.CheapestRecipe(nil)
	if !got.NA {
		t.Fatalf("empty recipes should be NA, got %+v", got)
	}
}
