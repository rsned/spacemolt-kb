package prodcost

import (
	"slices"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

func mat(id string, q float64) catalog.Material { return catalog.Material{ItemID: id, Quantity: q} }

// steelWorld has steel_plate by hand (basic_iron_smelting), at an empire L1
// and on a faction L2 line (refine_steel).
func steelWorld() World {
	return World{
		Items: map[string]catalog.Item{
			"iron_ore":    {ID: "iron_ore", Category: "ore", ExtractedBy: []string{"mining"}},
			"steel_plate": {ID: "steel_plate"},
		},
		Recipes: []catalog.Recipe{
			{ID: "basic_iron_smelting", CraftingTime: 0.27, HandCraftable: true,
				Inputs: []catalog.Material{mat("iron_ore", 3)}, Outputs: []catalog.Material{mat("steel_plate", 1)}},
			{ID: "refine_steel", CraftingTime: 0.54, ProducedBy: []string{"iron_refinery", "industrial_forge"},
				Inputs: []catalog.Material{mat("iron_ore", 5)}, Outputs: []catalog.Material{mat("steel_plate", 2)}},
		},
		Facilities: map[string]catalog.Facility{
			"iron_refinery":    {ID: "iron_refinery", Level: 1, LaborCost: 10},
			"industrial_forge": {ID: "industrial_forge", Level: 2, LaborCost: 20},
		},
		Lines: []Line{
			{RecipeID: "refine_steel", Level: 1, Fee: 9, Empire: true},
			{RecipeID: "refine_steel", Level: 2, Fee: 4, TicksPerRun: 0.083, FactionTag: "ABC", StationName: "Dock", Kind: KindFaction},
		},
	}
}

func routeOf(t *testing.T, ic *ItemCost, recipe string, k RouteKind) RouteCost {
	t.Helper()
	for _, r := range ic.Routes {
		if r.RecipeID == recipe && r.Kind == k {
			return r
		}
	}
	t.Fatalf("no %s/%s route in %+v", recipe, k, ic.Routes)
	return RouteCost{}
}

func TestComputeHandIsFree(t *testing.T) {
	got := Compute(steelWorld(), Options{})
	r := routeOf(t, got["steel_plate"], "basic_iron_smelting", KindHand)
	if r.CreditsPerUnit != 0 || !near(r.TicksPerUnit, 0.27) || !near(r.RawPerUnit, 3) || r.Partial || r.Cycle {
		t.Errorf("got %+v", r)
	}
	if len(r.Inputs) != 1 || r.Inputs[0] != (InputLine{ItemID: "iron_ore", QtyPerUnit: 3, Via: "raw"}) {
		t.Errorf("inputs %+v", r.Inputs)
	}
}

func TestComputeEmpirePerUnit(t *testing.T) {
	got := Compute(steelWorld(), Options{})
	r := routeOf(t, got["steel_plate"], "refine_steel", KindEmpire)
	if !near(r.CreditsPerUnit, 9.5) || !near(r.TicksPerUnit, 0.054) || !near(r.RawPerUnit, 2.5) {
		t.Errorf("got %+v", r)
	}
}

func TestComputeHighlights(t *testing.T) {
	ic := Compute(steelWorld(), Options{})["steel_plate"]
	if ic.NoUsableRoute || ic.Cheapest < 0 || ic.Fastest < 0 {
		t.Fatalf("got %+v", ic)
	}
	if c := ic.Routes[ic.Cheapest]; c.Kind != KindHand {
		t.Errorf("cheapest %+v", c)
	}
	if f := ic.Routes[ic.Fastest]; f.Kind != KindFaction || !near(f.TicksPerUnit, 0.0415) {
		t.Errorf("fastest %+v", f)
	}
	// Sorted credits asc, then ticks asc.
	for i := 1; i < len(ic.Routes); i++ {
		a, b := ic.Routes[i-1], ic.Routes[i]
		if a.CreditsPerUnit > b.CreditsPerUnit || (a.CreditsPerUnit == b.CreditsPerUnit && a.TicksPerUnit > b.TicksPerUnit) {
			t.Errorf("unsorted at %d: %+v", i, ic.Routes)
		}
	}
}

func TestComputeInputsRollUp(t *testing.T) {
	w := World{
		Items: map[string]catalog.Item{"copper_ore": {ID: "copper_ore", ExtractedBy: []string{"mining"}}},
		Recipes: []catalog.Recipe{
			{ID: "draw_copper_wiring", CraftingTime: 0.32, HandCraftable: true,
				Inputs: []catalog.Material{mat("copper_ore", 2)}, Outputs: []catalog.Material{mat("copper_wiring", 1)}},
			{ID: "assemble_harness", CraftingTime: 1.5, ProducedBy: []string{"assembly_bay"},
				Inputs: []catalog.Material{mat("copper_wiring", 3)}, Outputs: []catalog.Material{mat("harness", 1)}},
		},
		Facilities: map[string]catalog.Facility{"assembly_bay": {ID: "assembly_bay", Level: 1, LaborCost: 12}},
		Lines:      []Line{{RecipeID: "assemble_harness", Level: 1, Fee: 325, Empire: true}},
	}
	r := routeOf(t, Compute(w, Options{})["harness"], "assemble_harness", KindEmpire)
	own := 1.5 / 5
	if !near(r.CreditsPerUnit, 337) || !near(r.TicksPerUnit, own+3*0.32) || !near(r.RawPerUnit, 6) {
		t.Errorf("got %+v", r)
	}
	if len(r.Inputs) != 1 || r.Inputs[0] != (InputLine{ItemID: "copper_wiring", QtyPerUnit: 3, Via: "hand"}) {
		t.Errorf("inputs %+v", r.Inputs)
	}
}

func TestComputeNoUsableRoute(t *testing.T) {
	w := World{
		Recipes: []catalog.Recipe{{ID: "forge_hull", CraftingTime: 5, ProducedBy: []string{"shipyard"},
			Outputs: []catalog.Material{mat("hull", 1)}}},
		Facilities: map[string]catalog.Facility{"shipyard": {ID: "shipyard", Level: 1, LaborCost: 10}},
	}
	ic := Compute(w, Options{})["hull"]
	if ic == nil || !ic.NoUsableRoute || ic.Cheapest != -1 || ic.Fastest != -1 || len(ic.Routes) != 1 {
		t.Fatalf("got %+v", ic)
	}
	if r := ic.Routes[0]; r.Kind != KindOwn || !near(r.CreditsPerUnit, 10) || !near(r.TicksPerUnit, 1) {
		t.Errorf("own %+v", r)
	}
}

func TestComputeCycleTerminates(t *testing.T) {
	w := World{Recipes: []catalog.Recipe{
		{ID: "make_a", CraftingTime: 1, HandCraftable: true, Inputs: []catalog.Material{mat("b", 1)}, Outputs: []catalog.Material{mat("a", 1)}},
		{ID: "make_b", CraftingTime: 2, HandCraftable: true, Inputs: []catalog.Material{mat("a", 1)}, Outputs: []catalog.Material{mat("b", 1)}},
	}}
	got := Compute(w, Options{})
	if got["a"] == nil || got["b"] == nil {
		t.Fatalf("got %+v", got)
	}
	cycles := 0
	for _, ic := range got {
		for _, r := range ic.Routes {
			if r.Cycle {
				cycles++
			}
		}
	}
	if cycles == 0 {
		t.Error("no route marked Cycle")
	}
}

func TestComputeUnpricedPropagates(t *testing.T) {
	w := World{Recipes: []catalog.Recipe{
		{ID: "make_y", CraftingTime: 1, HandCraftable: true, Inputs: []catalog.Material{mat("whale_oil", 2)}, Outputs: []catalog.Material{mat("y", 1)}},
		{ID: "make_x", CraftingTime: 1, HandCraftable: true, Inputs: []catalog.Material{mat("y", 1)}, Outputs: []catalog.Material{mat("x", 1)}},
	}}
	got := Compute(w, Options{})
	y := got["y"].Routes[0]
	if !y.Partial || !slices.Equal(y.Unpriced, []string{"whale_oil"}) || y.Inputs[0].Via != "unpriced" {
		t.Errorf("y %+v", y)
	}
	x := got["x"].Routes[0]
	if !x.Partial || !slices.Equal(x.Unpriced, []string{"whale_oil"}) || x.Inputs[0].Via != "hand" {
		t.Errorf("x %+v", x)
	}
	if _, ok := got["whale_oil"]; ok {
		t.Error("unpriced item has an entry")
	}
}

func TestComputeSkipInputs(t *testing.T) {
	w := World{Recipes: []catalog.Recipe{
		{ID: "fabricate_circuit_boards", CraftingTime: 1, HandCraftable: true,
			Inputs: []catalog.Material{mat("energy_crystal", 1)}, Outputs: []catalog.Material{mat("circuit_board", 1)}},
		{ID: "etch_circuit_boards", CraftingTime: 2, HandCraftable: true, Outputs: []catalog.Material{mat("circuit_board", 1)}},
	}}
	ic := Compute(w, Options{})["circuit_board"]
	if len(ic.Routes) != 2 {
		t.Fatalf("unskipped %+v", ic.Routes)
	}
	ic = Compute(w, Options{SkipInputs: map[string]bool{"energy_crystal": true}})["circuit_board"]
	for _, r := range ic.Routes {
		if r.RecipeID == "fabricate_circuit_boards" {
			t.Errorf("skipped recipe present: %+v", ic.Routes)
		}
	}
	if len(ic.Routes) != 1 {
		t.Errorf("got %+v", ic.Routes)
	}
}

func TestComputeByproductPrimaryOnly(t *testing.T) {
	w := World{Recipes: []catalog.Recipe{{ID: "electrolyze_water", CraftingTime: 1, HandCraftable: true,
		Outputs: []catalog.Material{mat("hydrogen_gas", 2), mat("oxygen_gas", 1)}}}}
	got := Compute(w, Options{})
	if h := got["hydrogen_gas"]; h == nil || len(h.Routes) != 1 || h.Routes[0].RecipeID != "electrolyze_water" || !near(h.Routes[0].TicksPerUnit, 0.5) {
		t.Errorf("hydrogen %+v", h)
	}
	if o, ok := got["oxygen_gas"]; ok {
		t.Errorf("oxygen has an entry: %+v", o)
	}
}

// TestComputeCycleNotMemoisedIntoCompleteItem: a has a complete fabricate
// route and an unwrap route from b; b is wrapped from a. b must cost a's
// fabricate route, never the broken cycle, and unwrap_a completes through
// fab_a then wrap_b, whatever the recipe order.
func TestComputeCycleNotMemoisedIntoCompleteItem(t *testing.T) {
	fab := catalog.Recipe{ID: "fab_a", CraftingTime: 3, ProducedBy: []string{"fab"},
		Inputs: []catalog.Material{mat("ore", 2)}, Outputs: []catalog.Material{mat("a", 1)}}
	unwrap := catalog.Recipe{ID: "unwrap_a", CraftingTime: 1, ProducedBy: []string{"wrapper"},
		Inputs: []catalog.Material{mat("b", 1)}, Outputs: []catalog.Material{mat("a", 1)}}
	wrap := catalog.Recipe{ID: "wrap_b", CraftingTime: 1, ProducedBy: []string{"wrapper"},
		Inputs: []catalog.Material{mat("a", 1)}, Outputs: []catalog.Material{mat("b", 1)}}
	orders := [][]catalog.Recipe{
		{fab, unwrap, wrap}, {unwrap, fab, wrap}, {wrap, fab, unwrap},
		{wrap, unwrap, fab}, {unwrap, wrap, fab}, {fab, wrap, unwrap},
	}
	for _, recs := range orders {
		w := World{
			Items:   map[string]catalog.Item{"ore": {ID: "ore", ExtractedBy: []string{"mining"}}},
			Recipes: recs,
			Facilities: map[string]catalog.Facility{
				"fab":     {ID: "fab", Level: 1, LaborCost: 100},
				"wrapper": {ID: "wrapper", Level: 1, LaborCost: 7},
			},
			Lines: []Line{{RecipeID: "fab_a", Level: 1, Empire: true}, {RecipeID: "unwrap_a", Level: 1, Empire: true},
				{RecipeID: "wrap_b", Level: 1, Empire: true}},
		}
		got := Compute(w, Options{})
		name := recs[0].ID + "," + recs[1].ID + "," + recs[2].ID
		a, b := got["a"], got["b"]
		if a == nil || b == nil || a.Cheapest < 0 || b.Cheapest < 0 {
			t.Fatalf("%s: got %+v", name, got)
		}
		if c := a.Routes[a.Cheapest]; c.RecipeID != "fab_a" || c.Cycle || c.Partial || !near(c.CreditsPerUnit, 100) {
			t.Errorf("%s: a cheapest %+v", name, c)
		}
		c := b.Routes[b.Cheapest]
		if c.Cycle || c.Partial || !near(c.CreditsPerUnit, 107) || !near(c.RawPerUnit, 2) {
			t.Errorf("%s: b cheapest %+v", name, c)
		}
		if len(c.Inputs) != 1 || c.Inputs[0].Via == "cycle" {
			t.Errorf("%s: b inputs %+v", name, c.Inputs)
		}
		if u := routeOf(t, a, "unwrap_a", KindEmpire); u.Cycle || u.Partial || !near(u.CreditsPerUnit, 114) {
			t.Errorf("%s: unwrap_a completes via fab_a, wrap_b: %+v", name, u)
		}
	}
}
