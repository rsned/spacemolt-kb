package main

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func TestGalaxyBook_PoolsAndSorts(t *testing.T) {
	books := map[string]*buildcost.Book{
		"a": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 12, Qty: 5}}}, BestBuy: map[string]float64{}},
		"b": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 8, Qty: 3}}}, BestBuy: map[string]float64{}},
	}
	gb := galaxyBook(books)
	l := gb.Sell["iron"]
	if len(l) != 2 || l[0].Price != 8 || l[1].Price != 12 {
		t.Fatalf("pooled iron ladder = %+v, want 8 then 12", l)
	}
	// Depth walk across the pool: need 6 → 3@8 + 3@12 = 60.
	w := gb.Walk("iron", 6)
	if w.Cost != 60 || w.Shortfall != 0 {
		t.Fatalf("walk = %+v, want cost 60 shortfall 0", w)
	}
}

func TestFmtMoney(t *testing.T) {
	cases := map[float64]string{
		25.38:    "25.38",
		3579.666: "3,579.67",
		28762.9:  "28,762.90",
		0:        "0.00",
		-4.5:     "-4.50",
	}
	for in, want := range cases {
		if got := fmtMoney(in); got != want {
			t.Fatalf("fmtMoney(%v) = %q, want %q", in, got, want)
		}
	}
}
