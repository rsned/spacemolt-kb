package buildcost

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestWalk_ExactFit(t *testing.T) {
	b := &Book{Sell: map[string]Ladder{"iron": {{Price: 10, Qty: 5}}}}
	got := b.Walk("iron", 5)
	if !approx(got.Cost, 50) || !approx(got.Covered, 5) || got.Shortfall != 0 {
		t.Fatalf("exact fit: got %+v, want cost 50 covered 5 shortfall 0", got)
	}
}

func TestWalk_MultiTierAscending(t *testing.T) {
	// Must consume cheapest first: 3@10 then 2@25 for qty 5 => 30+50=80.
	b := &Book{Sell: map[string]Ladder{"iron": {{Price: 10, Qty: 3}, {Price: 25, Qty: 10}}}}
	got := b.Walk("iron", 5)
	if !approx(got.Cost, 80) || !approx(got.Covered, 5) || got.Shortfall != 0 {
		t.Fatalf("multi tier: got %+v, want cost 80 covered 5", got)
	}
}

func TestWalk_ShortBook(t *testing.T) {
	b := &Book{Sell: map[string]Ladder{"iron": {{Price: 10, Qty: 2}}}}
	got := b.Walk("iron", 5)
	if !approx(got.Cost, 20) || !approx(got.Covered, 2) || !approx(got.Shortfall, 3) {
		t.Fatalf("short book: got %+v, want cost 20 covered 2 shortfall 3", got)
	}
}

func TestWalk_EmptyBook(t *testing.T) {
	b := &Book{Sell: map[string]Ladder{}}
	got := b.Walk("iron", 5)
	if got.Cost != 0 || got.Covered != 0 || !approx(got.Shortfall, 5) {
		t.Fatalf("empty book: got %+v, want cost 0 covered 0 shortfall 5", got)
	}
}
