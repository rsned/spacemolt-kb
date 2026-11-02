package prodcost

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

var update = flag.Bool("update", false, "rewrite testdata/js-fixture.json and testdata/js-golden.json")

// goldenWorld joins the steel_plate, copper_wiring and plasma_injector
// fixtures, with a CRFT line, an energy_crystal recipe on each side, and a
// market whose bidders sit 1–3 hops from home (plus one unreachable).
func goldenWorld() (World, Market, HaulParams) {
	w := World{
		Items: map[string]catalog.Item{
			"iron_ore":        {ID: "iron_ore", Name: "Iron Ore", Category: "ore", ExtractedBy: []string{"mining"}, Size: 1},
			"copper_ore":      {ID: "copper_ore", Name: "Copper Ore", Category: "ore", ExtractedBy: []string{"mining"}, Size: 1},
			"energy_crystal":  {ID: "energy_crystal", Name: "Energy Crystal", Category: "ore", ExtractedBy: []string{"mining"}, Size: 1},
			"steel_plate":     {ID: "steel_plate", Name: "Steel Plate", Category: "component", Size: 1},
			"copper_wiring":   {ID: "copper_wiring", Name: "Copper Wiring", Category: "component", Size: 0.5},
			"harness":         {ID: "harness", Name: "Harness", Category: "component", Size: 2},
			"mystery_part":    {ID: "mystery_part", Name: "Mystery Part", Category: "component", Size: 1},
			"plasma_injector": {ID: "plasma_injector", Name: "Plasma Injector", Category: "component", Size: 1},
			"circuit_board":   {ID: "circuit_board", Name: "Circuit Board", Category: "component", Size: 1},
		},
		Recipes: []catalog.Recipe{
			{ID: "basic_iron_smelting", CraftingTime: 0.27, HandCraftable: true,
				Inputs: []catalog.Material{mat("iron_ore", 3)}, Outputs: []catalog.Material{mat("steel_plate", 1)}},
			{ID: "refine_steel", CraftingTime: 0.54, ProducedBy: []string{"iron_refinery", "industrial_forge"},
				Inputs: []catalog.Material{mat("iron_ore", 5)}, Outputs: []catalog.Material{mat("steel_plate", 2)}},
			{ID: "draw_copper_wiring", CraftingTime: 0.32, HandCraftable: true,
				Inputs: []catalog.Material{mat("copper_ore", 16)}, Outputs: []catalog.Material{mat("copper_wiring", 2)}},
			{ID: "assemble_harness", CraftingTime: 1.5, ProducedBy: []string{"assembly_bay"},
				Inputs: []catalog.Material{mat("copper_wiring", 3), mat("steel_plate", 1)}, Outputs: []catalog.Material{mat("harness", 1)}},
			{ID: "make_plasma_injector", CraftingTime: 1, HandCraftable: true,
				Inputs: []catalog.Material{mat("iron_ore", 2), mat("mystery_part", 1)}, Outputs: []catalog.Material{mat("plasma_injector", 1)}},
			{ID: "make_plasma_injector_alt", CraftingTime: 2, HandCraftable: true,
				Inputs: []catalog.Material{mat("iron_ore", 4), mat("mystery_part", 1)}, Outputs: []catalog.Material{mat("plasma_injector", 1)}},
			{ID: "fabricate_circuit_boards", CraftingTime: 1, HandCraftable: true,
				Inputs: []catalog.Material{mat("energy_crystal", 1)}, Outputs: []catalog.Material{mat("circuit_board", 1)}},
			{ID: "etch_circuit_boards", CraftingTime: 2, HandCraftable: true,
				Inputs: []catalog.Material{mat("copper_wiring", 2)}, Outputs: []catalog.Material{mat("circuit_board", 1)}},
			{ID: "synthesise_crystal", CraftingTime: 3, HandCraftable: true,
				Inputs: []catalog.Material{mat("iron_ore", 4)}, Outputs: []catalog.Material{mat("energy_crystal", 1)}},
		},
		Facilities: map[string]catalog.Facility{
			"iron_refinery":    {ID: "iron_refinery", Level: 1, LaborCost: 10},
			"industrial_forge": {ID: "industrial_forge", Level: 2, LaborCost: 20},
			"assembly_bay":     {ID: "assembly_bay", Level: 1, LaborCost: 12},
		},
		Lines: []Line{
			{StationID: "emp_a", RecipeID: "refine_steel", Level: 1, Fee: 9, Empire: true},
			{StationID: "dock", RecipeID: "refine_steel", Level: 2, Fee: 4, TicksPerRun: 0.083, FactionTag: "ABC", StationName: "Dock", Kind: KindFaction},
			{StationID: "emp_a", RecipeID: "assemble_harness", Level: 1, Fee: 325, Empire: true},
			{StationID: "cj", RecipeID: "assemble_harness", Level: 1, FactionTag: "CRFT", StationName: "Conjunction Junction", Kind: KindCRFT},
		},
	}
	m := Market{
		Asks: map[string][]Level{
			"copper_ore":     {{StationID: "near", Price: 3, Qty: 1000}, {StationID: "far", Price: 4, Qty: 1000}},
			"iron_ore":       {{StationID: "far", Price: 2, Qty: 50}, {StationID: "island", Price: 2.5, Qty: 500}, {StationID: "near", Price: 6, Qty: 500}},
			"harness":        {{StationID: "near", Price: 100, Qty: 4}},
			"mystery_part":   {{StationID: "far", Price: 40, Qty: 10}},
			"energy_crystal": {{StationID: "near", Price: 70, Qty: 10}},
		},
		Bids: map[string][]Level{
			"steel_plate":     {{StationID: "bidder", Price: 50, Qty: 100}},
			"copper_wiring":   {{StationID: "bidder", Price: 30, Qty: 40}},
			"harness":         {{StationID: "near", Price: 500, Qty: 5}, {StationID: "island", Price: 450, Qty: 9}},
			"plasma_injector": {{StationID: "near", Price: 9000, Qty: 3}},
			"circuit_board":   {{StationID: "far", Price: 80, Qty: 20}},
			"energy_crystal":  {{StationID: "near", Price: 90, Qty: 10}},
		},
		StationSystem: map[string]string{"near": "s1", "far": "s2", "bidder": "s3", "emp_a": "s1",
			"dock": "s2", "cj": "home", "island": "isolated"},
	}
	hp := HaulParams{Home: "home", JumpTicks: 4, Hold: 100,
		Hops: allHops([2]string{"home", "s1"}, [2]string{"s1", "s2"}, [2]string{"s2", "s3"})}
	return w, m, hp
}

var goldenSkip = map[string]bool{"energy_crystal": true}

// jsGolden is what tests/js/prodcost.test.js checks prodcost.js against.
type jsGolden struct {
	Totals        map[string]goldenTotal `json:"totals"`
	Opportunities []Opportunity          `json:"opportunities"`
}

type goldenTotal struct {
	Credits float64 `json:"credits"`
	Ticks   float64 `json:"ticks"`
	Partial bool    `json:"partial"`
}

func marshalIndent(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// TestWriteJSGolden pins the page data and Go's default-setting totals for
// goldenWorld; the JS test must reproduce the totals from the page data.
func TestWriteJSGolden(t *testing.T) {
	w, m, hp := goldenWorld()
	costs := Compute(w, Options{SkipInputs: goldenSkip})
	g := jsGolden{Totals: map[string]goldenTotal{}, Opportunities: Opportunities(costs, w, m, hp)}
	for id, ic := range costs {
		top := max(ic.Cheapest, 0)
		c, tk, p := CostUnder(costs, w, id, ic.Routes[top].Route, PolicyCheapest)
		g.Totals[id] = goldenTotal{Credits: c, Ticks: tk, Partial: p}
	}
	files := map[string][]byte{
		"js-fixture.json": marshalIndent(t, PageData(w, costs, m, hp)),
		"js-golden.json":  marshalIndent(t, g),
	}
	for name, want := range files {
		path := filepath.Join("testdata", name)
		if *update {
			if err := os.MkdirAll("testdata", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, want, 0o644); err != nil { //nolint:gosec // test fixture
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path) //nolint:gosec // fixed test path
		if err != nil {
			t.Fatalf("%v (run with -update)", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale; run go test ./pkg/prodcost -run TestWriteJSGolden -update", path)
		}
	}
	if strings.Contains(string(files["js-fixture.json"]), "energy_crystal") {
		t.Error("energy_crystal in page data")
	}
	if len(g.Opportunities) == 0 || len(g.Totals) < 5 {
		t.Errorf("fixture too thin: %d opportunities, %d totals", len(g.Opportunities), len(g.Totals))
	}
}

func TestComputeSkipsSkippedItemsOwnRecipes(t *testing.T) {
	w, _, _ := goldenWorld()
	costs := Compute(w, Options{SkipInputs: goldenSkip})
	if _, ok := costs["energy_crystal"]; ok {
		t.Error("energy_crystal was costed")
	}
	if ic := costs["circuit_board"]; ic == nil || len(ic.Routes) != 1 || ic.Routes[0].RecipeID != "etch_circuit_boards" {
		t.Errorf("circuit_board %+v", ic)
	}
}

func TestPageDataShape(t *testing.T) {
	w, m, hp := goldenWorld()
	costs := Compute(w, Options{SkipInputs: goldenSkip})
	p := PageData(w, costs, m, hp)
	if p.Home != "home" || p.JumpTicks != 4 || p.Hold != 100 || p.TickSeconds != 10 {
		t.Errorf("haul params %+v", p)
	}
	ore := p.Items["iron_ore"]
	if !ore.Raw || len(ore.Asks) != 3 || ore.Asks[0] != (PageAsk{Station: "far", Price: 2, Qty: 50, Hops: 2}) || ore.Asks[1].Hops != -1 {
		t.Errorf("iron_ore %+v", ore)
	}
	if mp, ok := p.Items["mystery_part"]; !ok || len(mp.Routes) != 0 || mp.Cheapest != -1 || mp.Name != "Mystery Part" {
		t.Errorf("mystery_part %+v", mp)
	}
	h := p.Items["harness"]
	var crft *PageRoute
	for i, r := range h.Routes {
		if r.Kind == string(KindCRFT) {
			crft = &h.Routes[i]
		}
		if r.Kind == string(KindOwn) && r.Sys != "home" {
			t.Errorf("own route runs at %q, want home", r.Sys)
		}
	}
	if crft == nil || crft.Sys != "home" || !crft.Usable || len(crft.Inputs) != 2 || crft.Inputs[0] != (PageInput{Item: "copper_wiring", Qty: 3}) {
		t.Errorf("harness crft route %+v", crft)
	}
	if got := p.Hops["home"]["s3"] + p.Hops["s1"]["s3"]; got != 5 {
		t.Errorf("hops %+v", p.Hops)
	}
	nBids := 0
	for _, b := range p.Bids {
		if b.Item == "energy_crystal" {
			t.Error("energy_crystal bid in page data")
		}
		nBids++
	}
	if nBids != 6 {
		t.Errorf("bids %+v", p.Bids)
	}
}
