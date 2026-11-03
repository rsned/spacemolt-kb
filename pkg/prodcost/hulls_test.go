package prodcost

import (
	"math"
	"slices"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

func hullWorld() World {
	return World{
		Items: map[string]catalog.Item{
			"bauxite":  {ID: "bauxite", ExtractedBy: []string{"mining"}},
			"iron_ore": {ID: "iron_ore", ExtractedBy: []string{"mining"}},
			"ti_ore":   {ID: "ti_ore", ExtractedBy: []string{"mining"}},
		},
		Recipes: []catalog.Recipe{
			{ID: "make_sheet", CraftingTime: 1, ProducedBy: []string{"press"},
				Inputs: []catalog.Material{mat("bauxite", 2)}, Outputs: []catalog.Material{mat("aluminum_sheet", 2)}},
			{ID: "make_polymer", HandCraftable: true, Inputs: []catalog.Material{mat("iron_ore", 1)}, Outputs: []catalog.Material{mat("flex_polymer", 1)}},
			{ID: "make_steel", HandCraftable: true, Inputs: []catalog.Material{mat("iron_ore", 3)}, Outputs: []catalog.Material{mat("steel_plate", 1)}},
			{ID: "make_ti", Inputs: []catalog.Material{mat("ti_ore", 2)}, Outputs: []catalog.Material{mat("titanium_alloy", 1)}},
			{ID: "make_container", HandCraftable: true, Inputs: []catalog.Material{mat("aluminum_sheet", 3), mat("steel_plate", 5)}, Outputs: []catalog.Material{mat("cargo_container", 1)}},
			{ID: "make_tank", HandCraftable: true, Inputs: []catalog.Material{mat("titanium_alloy", 1)}, Outputs: []catalog.Material{mat("fuel_tank", 1)}},
			{ID: "make_plating", HandCraftable: true, Inputs: []catalog.Material{mat("steel_plate", 4), mat("titanium_alloy", 1)}, Outputs: []catalog.Material{mat("hull_plating", 1)}},
		},
		Facilities: map[string]catalog.Facility{"press": {ID: "press", Level: 1, LaborCost: 10, Empire: "solarian"}},
		Lines: []Line{
			{RecipeID: "make_sheet", Level: 1, Fee: 6, Empire: true},
			{RecipeID: "make_ti", Level: 1, Fee: 37, FactionTag: "ABC", StationName: "Dock", Kind: KindFaction},
		},
	}
}

func congregation() catalog.Ship {
	return catalog.Ship{ID: "congregation_hull", Class: "congregation", Tier: 1, Faction: "crft", BuildMaterials: []catalog.Material{
		mat("aluminum_sheet", 50), mat("flex_polymer", 20), mat("cargo_container", 12), mat("fuel_tank", 5), mat("hull_plating", 3),
	}}
}

func costOne(t *testing.T, w World, s catalog.Ship, d HullData) HullCost {
	t.Helper()
	got := CostHulls([]catalog.Ship{s}, w, Compute(w, Options{UnbuiltL1: true}), Market{}, d)
	if len(got) != 1 {
		t.Fatalf("got %d hulls", len(got))
	}
	return got[0]
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestCostHullsCongregationWorkedExample(t *testing.T) {
	r := 723
	d := HullData{Quotes: []Quote{{ShipClass: "congregation", Tier: 1, Labor: 1000, Royalty: &r, Tick: 5}},
		Licensed: map[string]bool{"congregation": true}}
	hc := costOne(t, hullWorld(), congregation(), d)
	if !approx(hc.MaterialCredits, 984) {
		t.Errorf("MaterialCredits %v", hc.MaterialCredits)
	}
	var credits []float64
	for _, m := range hc.Materials {
		credits = append(credits, m.Credits)
	}
	if !slices.EqualFunc(credits, []float64{400, 0, 288, 185, 111}, approx) {
		t.Errorf("credits %v", credits)
	}
	if !approx(hc.Royalty, 98.4) || hc.Labor == nil || *hc.Labor != 1000 || hc.LaborNote != "quoted" || !approx(hc.MakeTotal, 2082.4) {
		t.Errorf("royalty/labor/total %+v", hc)
	}
	if hc.QuotedRoyalty == nil || *hc.QuotedRoyalty != 723 {
		t.Errorf("quoted royalty %v", hc.QuotedRoyalty)
	}
	if hc.License != "owned" || hc.LicenseCost != 1_000_000 {
		t.Errorf("license %v %d", hc.License, hc.LicenseCost)
	}
	if hc.Partial || len(hc.Flags) != 0 {
		t.Errorf("partial %v flags %v", hc.Partial, hc.Flags)
	}
}

func TestCostHullsFlagsCrystalAndUnbuilt(t *testing.T) {
	w := World{
		Items:      map[string]catalog.Item{"energy_crystal": {ID: "energy_crystal", ExtractedBy: []string{"mining"}}},
		Recipes:    []catalog.Recipe{{ID: "make_core", ProducedBy: []string{"lab"}, Inputs: []catalog.Material{mat("energy_crystal", 1)}, Outputs: []catalog.Material{mat("core", 1)}}},
		Facilities: map[string]catalog.Facility{"lab": {ID: "lab", Level: 1, LaborCost: 30}},
	}
	s := catalog.Ship{Class: "x", Tier: 1, BuildMaterials: []catalog.Material{mat("core", 2)}}
	hc := costOne(t, w, s, HullData{})
	if !slices.Contains(hc.Materials[0].Flags, FlagCrystal) || !slices.Contains(hc.Materials[0].Flags, FlagUnbuilt) {
		t.Errorf("flags %v", hc.Materials[0].Flags)
	}
	if !approx(hc.Materials[0].Credits, 60) {
		t.Errorf("credits %v", hc.Materials[0].Credits)
	}
}

func TestCostHullsSalvageIsFlaggedFloor(t *testing.T) {
	s := catalog.Ship{Class: "x", BuildMaterials: []catalog.Material{mat("salvage_metal", 200), mat("flex_polymer", 1)}}
	hc := costOne(t, hullWorld(), s, HullData{})
	if !hc.Partial || !slices.Contains(hc.Flags, FlagSalvage) || hc.Materials[0].Credits != 0 || hc.Materials[0].Route != "unpriced" {
		t.Errorf("got %+v", hc)
	}
}

func TestCostHullsUnpricedNonSalvage(t *testing.T) {
	s := catalog.Ship{Class: "x", BuildMaterials: []catalog.Material{mat("mystery_part", 1)}}
	hc := costOne(t, hullWorld(), s, HullData{})
	if !slices.Contains(hc.Flags, FlagUnpriced) || slices.Contains(hc.Flags, FlagSalvage) || !hc.Partial {
		t.Errorf("flags %v partial %v", hc.Flags, hc.Partial)
	}
}

func TestCostHullsLaborTierRange(t *testing.T) {
	d := HullData{Quotes: []Quote{{ShipClass: "a", Tier: 2, Labor: 2000, Tick: 1}, {ShipClass: "b", Tier: 2, Labor: 2400, Tick: 2}}}
	got := CostHulls([]catalog.Ship{{Class: "c", Tier: 2}, {Class: "a", Tier: 2}}, hullWorld(), nil, Market{}, d)
	if got[0].Labor == nil || *got[0].Labor != 2000 || got[0].LaborNote != "tier range 2,000–2,400" {
		t.Errorf("c: %v %q", got[0].Labor, got[0].LaborNote)
	}
	if got[1].Labor == nil || *got[1].Labor != 2000 || got[1].LaborNote != "quoted" {
		t.Errorf("a: %v %q", got[1].Labor, got[1].LaborNote)
	}
}

func TestCostHullsLaborUnknown(t *testing.T) {
	d := HullData{Quotes: []Quote{{ShipClass: "a", Tier: 2, Labor: 2000}}}
	hc := CostHulls([]catalog.Ship{{Class: "z", Tier: 3}}, hullWorld(), nil, Market{}, d)[0]
	if hc.Labor != nil || !slices.Contains(hc.Flags, FlagLaborUnknown) || hc.Best {
		t.Errorf("got %+v", hc)
	}
}

func TestCostHullsLicenseStates(t *testing.T) {
	ships := []catalog.Ship{
		{Class: "r", CapturePolicy: "reclaim_only", Faction: "f", Tier: 2},
		{Class: "p", PrestigeLock: "x", Faction: "f", Tier: 2},
		{Class: "b", NPCRole: "boss_flagship", Faction: "f", Tier: 2},
		{Class: "pirate", Faction: "pirate", Tier: 3},
	}
	got := CostHulls(ships, hullWorld(), nil, Market{}, HullData{})
	want := []LicenseState{"not buildable", "not licensable", "not licensable", "not owned"}
	for i, w := range want {
		if got[i].License != w {
			t.Errorf("%s: %q want %q", ships[i].Class, got[i].License, w)
		}
	}
	if got[3].LicenseCost != LicenseCost(3) {
		t.Errorf("cost %d", got[3].LicenseCost)
	}
	if got[0].Labor != nil || slices.Contains(got[0].Flags, FlagLaborUnknown) {
		t.Errorf("reclaim_only: %+v", got[0])
	}
}

func TestLicenseCost(t *testing.T) {
	for tier, want := range map[int]int{0: 0, 1: 1_000_000, 2: 5_000_000, 3: 10_000_000, 4: 20_000_000, 5: 50_000_000, 6: 0} {
		if got := LicenseCost(tier); got != want {
			t.Errorf("tier %d: %d want %d", tier, got, want)
		}
	}
}
