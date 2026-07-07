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

func TestFacilityGroup(t *testing.T) {
	recipeOut := map[string]string{"build_railgun": "railgun", "mystery": "unknownitem"}
	itemCat := map[string]string{"railgun": "weapon"}
	cases := []struct {
		name string
		f    FacilityRec
		want string
	}{
		{"production resolves to produced-item category",
			FacilityRec{Category: "production", RecipeID: "build_railgun"}, "weapon"},
		{"production with no recipe output -> other",
			FacilityRec{Category: "production", RecipeID: "none"}, "other"},
		{"production with uncategorized output -> other",
			FacilityRec{Category: "production", RecipeID: "mystery"}, "other"},
		{"non-production uses facility category",
			FacilityRec{Category: "service", RecipeID: "build_railgun"}, "service"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := facilityGroup(tc.f, recipeOut, itemCat); got != tc.want {
				t.Fatalf("facilityGroup = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildFacilityView(t *testing.T) {
	reqs := []buildcost.Requirement{
		{ItemID: "titanium_ore", Qty: 8},
		{ItemID: "copper_ore", Qty: 2},
		{ItemID: "exotic", Qty: 5}, // no VWAP, thin galaxy depth
	}
	sellVWAP := map[string]float64{"titanium_ore": 100, "copper_ore": 25}
	galaxy := &buildcost.Book{Sell: map[string]buildcost.Ladder{
		"titanium_ore": {{Price: 90, Qty: 100}},
		"copper_ore":   {{Price: 20, Qty: 100}},
		"exotic":       {{Price: 5, Qty: 2}}, // only 2 of 5 available
	}, BestBuy: map[string]float64{}}
	names := map[string]string{"titanium_ore": "Titanium Ore", "copper_ore": "Copper Ore", "exotic": "Exotic"}
	cats := map[string]string{"titanium_ore": "ore", "copper_ore": "ore"} // exotic uncategorized

	v := buildFacilityView(reqs, sellVWAP, galaxy, names, cats)

	if v.MktCount != 3 || v.MktPriced != 2 {
		t.Fatalf("mkt count/priced = %d/%d, want 3/2", v.MktCount, v.MktPriced)
	}
	// MKT total over priced components only: 8*100 + 2*25 = 850.
	if v.MktTotal != 850 {
		t.Fatalf("MktTotal = %v, want 850", v.MktTotal)
	}
	// Galaxy is infeasible (exotic short 3), covered 2 of 3 components.
	if v.GalFeasible || v.GalCovered != 2 {
		t.Fatalf("galaxy feasible=%v covered=%d, want false/2", v.GalFeasible, v.GalCovered)
	}
	byID := map[string]FacilityComponentCost{}
	for _, c := range v.Components {
		byID[c.ItemID] = c
	}
	if !byID["titanium_ore"].HasMkt || byID["titanium_ore"].MktUnit != 100 {
		t.Fatalf("titanium mkt = %+v", byID["titanium_ore"])
	}
	if byID["titanium_ore"].Href != "../../../items/ore/titanium_ore.html" {
		t.Fatalf("titanium href = %q", byID["titanium_ore"].Href)
	}
	if !byID["titanium_ore"].GalFull || byID["titanium_ore"].GalUnit != 90 {
		t.Fatalf("titanium galaxy = %+v", byID["titanium_ore"])
	}
	if byID["exotic"].HasMkt || byID["exotic"].GalFull || byID["exotic"].Href != "" {
		t.Fatalf("exotic = %+v (want no mkt, not full, no href)", byID["exotic"])
	}
	if byID["exotic"].Name != "Exotic" {
		t.Fatalf("exotic name = %q", byID["exotic"].Name)
	}
}

func TestFacilityViewVM_CoverageAndPricedNote(t *testing.T) {
	// 3 components, 2 priced, galaxy covers 2 of 3 → infeasible with coverage note.
	v := FacilityView{
		MktCount: 3, MktPriced: 2, MktTotal: 850,
		GalFeasible: false, GalCovered: 2,
		Components: []FacilityComponentCost{
			{Name: "Titanium Ore", Href: "../../../items/ore/titanium_ore.html", Qty: 8, MktUnit: 100, HasMkt: true, GalUnit: 90, GalFull: true},
			{Name: "Exotic", Qty: 5}, // unpriced, not full
		},
	}
	vm := facilityViewVM("BoM (ore)", v)
	if vm.Title != "BoM (ore)" || vm.Empty {
		t.Fatalf("vm header = %+v", vm)
	}
	if vm.MktBuildCost != "850.00" || vm.MktNote != "(2/3 priced)" {
		t.Fatalf("mkt = %q note %q", vm.MktBuildCost, vm.MktNote)
	}
	if !vm.GalInfeasible || vm.GalBuildCost != "2/3 covered" {
		t.Fatalf("gal = %q infeasible=%v", vm.GalBuildCost, vm.GalInfeasible)
	}
	c0 := vm.Components[0]
	if c0.MktUnit != "100.00" || c0.MktTotal != "800.00" || c0.GalUnit != "90.00" || c0.GalTotal != "720.00" || c0.GalInfeasible {
		t.Fatalf("component0 = %+v", c0)
	}
	c1 := vm.Components[1]
	if c1.MktUnit != "—" || c1.GalUnit != "—" || !c1.GalInfeasible {
		t.Fatalf("component1 = %+v", c1)
	}
}

func TestBuildFacilityPages_GroupsSortTOC(t *testing.T) {
	recs := []FacilityRec{
		{ID: "b_forge", Name: "B Forge", Category: "production", RecipeID: "r_wpn", Level: 2,
			Build: []buildcost.Requirement{{ItemID: "iron", Qty: 2}}},
		{ID: "a_forge", Name: "A Forge", Category: "production", RecipeID: "r_wpn", Level: 1,
			Build: []buildcost.Requirement{{ItemID: "iron", Qty: 1}}},
		{ID: "depot", Name: "Depot", Category: "service",
			Build: []buildcost.Requirement{{ItemID: "iron", Qty: 3}}},
	}
	facBoM := map[string][]buildcost.Requirement{
		"a_forge": {{ItemID: "iron_ore", Qty: 2}},
		"b_forge": {{ItemID: "iron_ore", Qty: 4}},
		"depot":   {{ItemID: "iron_ore", Qty: 6}},
	}
	recipeOut := map[string]string{"r_wpn": "railgun"}
	names := map[string]string{"railgun": "Railgun", "iron": "Iron", "iron_ore": "Iron Ore"}
	cats := map[string]string{"railgun": "weapon", "iron": "component", "iron_ore": "ore"}
	sellVWAP := map[string]float64{"iron": 10, "iron_ore": 5}
	galaxy := &buildcost.Book{Sell: map[string]buildcost.Ladder{
		"iron":     {{Price: 9, Qty: 100}},
		"iron_ore": {{Price: 4, Qty: 100}},
	}, BestBuy: map[string]float64{}}

	pages, summaries, stats := buildFacilityPages(recs, facBoM, recipeOut, names, cats, sellVWAP, galaxy)

	// Two groups: weapon (2), service (1); summaries sorted by group name.
	if len(summaries) != 2 || summaries[0].Group != "service" || summaries[1].Group != "weapon" {
		t.Fatalf("summaries = %+v", summaries)
	}

	// Stats are group-sorted like summaries, with per-level rows ascending.
	if len(stats) != 2 || stats[0].Group != "service" || stats[1].Group != "weapon" {
		t.Fatalf("stats groups = %+v", stats)
	}
	ws := stats[1] // weapon
	if ws.Count != 2 || len(ws.Levels) != 2 {
		t.Fatalf("weapon stats = %+v", ws)
	}
	if ws.Levels[0].Level != 1 || ws.Levels[1].Level != 2 {
		t.Fatalf("weapon levels not ascending: %+v", ws.Levels)
	}
	// a_forge (L1): BoM iron_ore×2 @ VWAP 5 = 10; Recipe iron×1 @ 10 = 10; both
	// fully priced and galaxy-feasible (ample depth) → single-sample means, buildable.
	l1 := ws.Levels[0]
	if l1.Count != 1 || l1.BoM != "10" || l1.Recipe != "10" || l1.Buildable != 1 {
		t.Fatalf("weapon L1 stat = %+v", l1)
	}
	if summaries[1].Count != 2 || summaries[1].Href != "weapon/" {
		t.Fatalf("weapon summary = %+v", summaries[1])
	}
	var weapon FacilityGroupPage
	for _, p := range pages {
		if p.Group == "weapon" {
			weapon = p
		}
	}
	// Facilities alphabetical within the group.
	if len(weapon.Facilities) != 2 || weapon.Facilities[0].ID != "a_forge" {
		t.Fatalf("weapon facilities = %+v", weapon.Facilities)
	}
	f := weapon.Facilities[0]
	if f.Href != "../../../facilities/production/a_forge.html" {
		t.Fatalf("detail href = %q", f.Href)
	}
	if f.Produces != "Railgun" {
		t.Fatalf("produces = %q", f.Produces)
	}
	if f.BoM.Title != "BoM (ore)" || f.Recipe.Title != "Recipe (components)" {
		t.Fatalf("view titles = %q / %q", f.BoM.Title, f.Recipe.Title)
	}
	// TOC on every page lists all groups with the active one flagged and links to siblings.
	var activeService bool
	for _, e := range weapon.TOC {
		if e.Group == "weapon" && !e.Active {
			t.Fatalf("weapon TOC entry should be active")
		}
		if e.Group == "service" {
			activeService = e.Active
			if e.Href != "../service/" {
				t.Fatalf("service TOC href = %q", e.Href)
			}
		}
	}
	if activeService {
		t.Fatalf("service should not be active on the weapon page")
	}
}

func TestFmtCompact(t *testing.T) {
	cases := map[float64]string{
		4_100_000:     "4.1M",
		900_000:       "900K",
		250_000:       "250K",
		2_000_000_000: "2.0B",
		250:           "250",
		0:             "0",
		-1_500_000:    "-1.5M",
		// Boundary cases: must promote to the next unit, not render "1000K"/"1000.0M".
		999_999:     "1.0M",
		999_999_999: "1.0B",
		999:         "999",
	}
	for in, want := range cases {
		if got := fmtCompact(in); got != want {
			t.Fatalf("fmtCompact(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestMktStatStr(t *testing.T) {
	if got := mktStatStr(nil); got != "—" {
		t.Fatalf("empty sample = %q, want em-dash", got)
	}
	if got := mktStatStr([]float64{4_100_000}); got != "4.1M" {
		t.Fatalf("single sample = %q, want 4.1M (no range)", got)
	}
	// median of {2M, 6M} = 4M (mean of the two middle values); min 2M, max 6M.
	if got := mktStatStr([]float64{2_000_000, 6_000_000}); got != "4.0M (2.0M–6.0M)" {
		t.Fatalf("two samples = %q, want '4.0M (2.0M–6.0M)'", got)
	}
	// Odd count, right-skewed: sorted {2M, 4M, 60M} → median 4M, min 2M, max 60M.
	// The outlier lifts max far above the median but does not move it (unlike a mean).
	if got := mktStatStr([]float64{60_000_000, 2_000_000, 4_000_000}); got != "4.0M (2.0M–60.0M)" {
		t.Fatalf("skewed sample = %q, want '4.0M (2.0M–60.0M)'", got)
	}
}

func TestBuildFacilityPages_StatsExcludePartialAndCountBuildable(t *testing.T) {
	// Two weapon-L1 facilities, both with no Recipe (empty Build) so the Recipe
	// stat has no samples. `full`'s BoM is fully priced and galaxy-feasible;
	// `partial`'s BoM component is unpriced and unstocked, so it is excluded from
	// the cost mean and is not buildable by any route.
	recs := []FacilityRec{
		{ID: "full", Name: "Full", Category: "production", RecipeID: "r", Level: 1},
		{ID: "partial", Name: "Partial", Category: "production", RecipeID: "r", Level: 1},
	}
	facBoM := map[string][]buildcost.Requirement{
		"full":    {{ItemID: "iron_ore", Qty: 4}},  // priced + stocked
		"partial": {{ItemID: "rare", Qty: 5}},       // no VWAP, no galaxy depth
	}
	recipeOut := map[string]string{"r": "railgun"}
	names := map[string]string{"railgun": "Railgun"}
	cats := map[string]string{"railgun": "weapon"}
	sellVWAP := map[string]float64{"iron": 10, "iron_ore": 5}
	galaxy := &buildcost.Book{Sell: map[string]buildcost.Ladder{
		"iron":     {{Price: 9, Qty: 100}},
		"iron_ore": {{Price: 4, Qty: 100}},
	}, BestBuy: map[string]float64{}} // no "rare" ladder → partial is unbuildable

	_, _, stats := buildFacilityPages(recs, facBoM, recipeOut, names, cats, sellVWAP, galaxy)
	if len(stats) != 1 || stats[0].Group != "weapon" {
		t.Fatalf("stats = %+v", stats)
	}
	l1 := stats[0].Levels[0]
	if l1.Count != 2 {
		t.Fatalf("level count = %d, want 2 (both facilities)", l1.Count)
	}
	// BoM mean over `full` only (iron_ore×4 @ 5 = 20); `partial` excluded (unpriced).
	if l1.BoM != "20" {
		t.Fatalf("BoM stat = %q, want 20 (partial excluded)", l1.BoM)
	}
	// Neither facility has a fully-priced non-empty Recipe view.
	if l1.Recipe != "—" {
		t.Fatalf("Recipe stat = %q, want em-dash", l1.Recipe)
	}
	// Only `full` is buildable; `partial`'s BoM lacks depth and it has no Recipe route.
	if l1.Buildable != 1 {
		t.Fatalf("buildable = %d, want 1", l1.Buildable)
	}
}
