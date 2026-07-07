package main

import (
	"os"
	"path/filepath"
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

func TestLoadFacilityCatalog(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "20260706")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	blob := `{"items":[
	  {"id":"forge","name":"Ghost Forge","category":"production","level":2,"recipe_id":"forge_ghost_rounds",
	   "build_materials":[{"item_id":"hot_cell","quantity":2},{"item_id":"titanium_ingot","quantity":3}]},
	  {"id":"depot","name":"Depot","category":"service","level":1,"build_materials":[]}
	]}`
	if err := os.WriteFile(filepath.Join(snap, "catalog_facilities.json"), []byte(blob), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, err := loadFacilityCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]FacilityRec{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	f := byID["forge"]
	if f.Name != "Ghost Forge" || f.Category != "production" || f.Level != 2 || f.RecipeID != "forge_ghost_rounds" {
		t.Fatalf("forge = %+v", f)
	}
	if len(f.Build) != 2 || f.Build[0].ItemID != "hot_cell" || f.Build[0].Qty != 2 {
		t.Fatalf("forge build = %+v", f.Build)
	}
	if byID["depot"].Category != "service" || len(byID["depot"].Build) != 0 {
		t.Fatalf("depot = %+v", byID["depot"])
	}
}

func TestLoadFacilityBoM(t *testing.T) {
	db := newCraftTestDB(t)
	bom, err := loadFacilityBoM(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bom["widget"]; ok {
		t.Fatal("widget is an item, must not appear in facility BoM")
	}
	reqs := bom["forge"]
	if len(reqs) != 2 {
		t.Fatalf("forge BoM = %+v", reqs)
	}
	got := map[string]float64{}
	for _, r := range reqs {
		got[r.ItemID] = r.Qty
	}
	if got["titanium_ore"] != 8 || got["copper_ore"] != 2 {
		t.Fatalf("forge BoM quantities = %+v", got)
	}
}

func TestLoadRecipeOutputItem(t *testing.T) {
	db := newCraftTestDB(t)
	out, err := loadRecipeOutputItem(db)
	if err != nil {
		t.Fatal(err)
	}
	if out["forge_ghost_rounds"] != "ghost_rounds" {
		t.Fatalf("recipe output = %q, want ghost_rounds", out["forge_ghost_rounds"])
	}
}
