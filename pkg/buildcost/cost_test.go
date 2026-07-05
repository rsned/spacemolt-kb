package buildcost

import "testing"

func bookFixture() *Book {
	return &Book{
		Sell: map[string]Ladder{
			"iron":   {{Price: 10, Qty: 100}},
			"copper": {{Price: 5, Qty: 100}},
			"gold":   {{Price: 50, Qty: 1}}, // thin
		},
		BestBuy: map[string]float64{},
	}
}

func TestPriceRequirements_AllCovered(t *testing.T) {
	b := bookFixture()
	got := b.PriceRequirements([]Requirement{{"iron", 2}, {"copper", 4}})
	if !got.Feasible || got.Covered != 2 || got.Total != 2 {
		t.Fatalf("feasible: got %+v", got)
	}
	if !approx(got.Cost, 2*10+4*5) { // 40
		t.Fatalf("cost: got %v want 40", got.Cost)
	}
	if len(got.Shortfalls) != 0 {
		t.Fatalf("expected no shortfalls, got %+v", got.Shortfalls)
	}
}

func TestPriceRequirements_PartialInfeasible(t *testing.T) {
	b := bookFixture()
	got := b.PriceRequirements([]Requirement{{"iron", 2}, {"gold", 5}})
	if got.Feasible {
		t.Fatalf("expected infeasible, got %+v", got)
	}
	if got.Covered != 1 || got.Total != 2 { // iron covered, gold not
		t.Fatalf("coverage: got covered=%d total=%d", got.Covered, got.Total)
	}
	// Partial cost still reported: iron 20 + gold 1 unit @50 = 70.
	if !approx(got.Cost, 70) {
		t.Fatalf("partial cost: got %v want 70", got.Cost)
	}
	if len(got.Shortfalls) != 1 || got.Shortfalls[0].ItemID != "gold" || !approx(got.Shortfalls[0].Short, 4) {
		t.Fatalf("shortfall: got %+v", got.Shortfalls)
	}
}
