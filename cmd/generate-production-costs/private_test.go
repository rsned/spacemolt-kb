package main

import (
	"encoding/json"
	"maps"
	"regexp"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/prodcost"
)

// privateWorld is steelWorld plus harness (from copper_wiring and
// steel_plate), an unpriced-market item, and energy_crystal on both sides of
// a recipe; the market makes buying harness cheaper than making it.
func privateWorld() (prodcost.World, prodcost.Market, prodcost.HaulParams) {
	w := steelWorld()
	for id, it := range map[string]catalog.Item{
		"copper_ore":     {ID: "copper_ore", Name: "Copper Ore", Category: "ore", ExtractedBy: []string{"mining"}, Size: 1},
		"energy_crystal": {ID: "energy_crystal", Name: "Energy Crystal", Category: "ore", ExtractedBy: []string{"mining"}, Size: 1},
		"copper_wiring":  {ID: "copper_wiring", Name: "Copper Wiring", Category: "component", Size: 1},
		"harness":        {ID: "harness", Name: "Harness", Category: "component", Size: 1},
		"circuit_board":  {ID: "circuit_board", Name: "Circuit Board", Category: "component", Size: 1},
	} {
		w.Items[id] = it
	}
	w.Recipes = append(w.Recipes,
		catalog.Recipe{ID: "draw_copper_wiring", CraftingTime: 0.32, HandCraftable: true,
			Inputs: []catalog.Material{mat("copper_ore", 2)}, Outputs: []catalog.Material{mat("copper_wiring", 1)}},
		catalog.Recipe{ID: "assemble_harness", CraftingTime: 1.5, ProducedBy: []string{"iron_refinery"},
			Inputs: []catalog.Material{mat("copper_wiring", 3), mat("steel_plate", 1)}, Outputs: []catalog.Material{mat("harness", 1)}},
		catalog.Recipe{ID: "fabricate_circuit_boards", CraftingTime: 1, HandCraftable: true,
			Inputs: []catalog.Material{mat("energy_crystal", 1)}, Outputs: []catalog.Material{mat("circuit_board", 1)}},
		catalog.Recipe{ID: "synthesise_crystal", CraftingTime: 3, HandCraftable: true,
			Inputs: []catalog.Material{mat("iron_ore", 4)}, Outputs: []catalog.Material{mat("energy_crystal", 1)}},
	)
	w.Lines = []prodcost.Line{
		{StationID: "emp", RecipeID: "refine_steel", Level: 1, Fee: 9, Empire: true},
		{StationID: "emp", RecipeID: "assemble_harness", Level: 1, Fee: 300, Empire: true},
	}
	m := prodcost.Market{
		Asks: map[string][]prodcost.Level{
			"harness":        {{StationID: "near", Price: 50, Qty: 10}},
			"iron_ore":       {{StationID: "near", Price: 2, Qty: 100}},
			"energy_crystal": {{StationID: "near", Price: 70, Qty: 10}},
		},
		Bids: map[string][]prodcost.Level{
			"harness":        {{StationID: "near", Price: 400, Qty: 5}},
			"energy_crystal": {{StationID: "near", Price: 90, Qty: 10}},
		},
		StationSystem: map[string]string{"near": "s1", "emp": "s1"},
	}
	hp := prodcost.HaulParams{Home: "home", JumpTicks: 4, Hold: 2000,
		Hops: map[string]map[string]int{"home": {"s1": 1}}}
	return w, m, hp
}

func TestRenderPrivateSections(t *testing.T) {
	w, m, hp := privateWorld()
	costs := prodcost.Compute(w, prodcost.Options{SkipInputs: map[string]bool{"energy_crystal": true}})
	opps := prodcost.Opportunities(costs, w, m, hp)
	dir := t.TempDir()
	ages := DataAges{Catalog: "20261101", Facilities: "2026-11-02T10:12:34Z", Market: "2026-11-02T10:44:23Z"}
	if err := renderPrivate(dir, w, costs, m, hp, opps, ages); err != nil {
		t.Fatal(err)
	}
	idx := readOut(t, dir, "index.html")
	for _, want := range []string{"Profit/hr", `<span class="pc-flag">buy is cheaper</span>`,
		`<span class="pc-flag">market has no price</span>`, "2026-11-02T10:44:23Z", `id="pc-data"`,
		`src="prodcost.js"`, `id="pc-raw"`, `id="pc-haul-fuel"`, `id="pc-mine-fuel"`, `id="pc-unpriced"`,
		"empire L1 (labor 10 + fee 300)", `<td><a href="../../kb/production-costs/harness.html">Harness</a></td>`} {
		if !strings.Contains(idx, want) {
			t.Errorf("index missing %q", want)
		}
	}
	// One <select> per tree step: harness -> copper_wiring -> copper_ore,
	// steel_plate -> iron_ore.
	harness := regexp.MustCompile(`(?s)<details class="pc-item" data-item="harness">.*?</details>`).FindString(idx)
	if n := strings.Count(harness, `<select class="pc-step"`); n != 5 {
		t.Errorf("harness tree has %d step selects, want 5:\n%s", n, harness)
	}
	if steps, sels := strings.Count(idx, "<li>"), strings.Count(idx, `<select class="pc-step"`); steps == 0 || steps != sels {
		t.Errorf("%d tree steps, %d selects", steps, sels)
	}

	data := readOut(t, dir, "data.json")
	for name, s := range map[string]string{"data.json": data, "index.html": idx} {
		if strings.Contains(s, "energy_crystal") {
			t.Errorf("energy_crystal in %s", name)
		}
	}
	var p prodcost.Page
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Bids) != 1 || p.Items["harness"].Asks[0].Price != 50 {
		t.Errorf("page data %+v", p)
	}
	if js := readOut(t, dir, "prodcost.js"); !strings.Contains(js, "function rank(") {
		t.Error("prodcost.js not written")
	}
}

func TestRenderPrivatePartialAndUnreachable(t *testing.T) {
	w, m, hp := privateWorld()
	m.Bids["harness"] = append(m.Bids["harness"], prodcost.Level{StationID: "island", Price: 300, Qty: 1})
	w.Recipes[3].Inputs = append(w.Recipes[3].Inputs, mat("relic", 1)) // assemble_harness
	costs := prodcost.Compute(w, prodcost.Options{})
	dir := t.TempDir()
	if err := renderPrivate(dir, w, costs, m, hp, prodcost.Opportunities(costs, w, m, hp), DataAges{}); err != nil {
		t.Fatal(err)
	}
	idx := readOut(t, dir, "index.html")
	for _, want := range []string{"partial — unpriced inputs at 0", "<td>unreachable</td>"} {
		if !strings.Contains(idx, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestRenderPrivateBaselineCycleIsLowerBound(t *testing.T) {
	w, m, hp := privateWorld()
	cw := cycleWorld()
	maps.Copy(w.Items, cw.Items)
	w.Recipes = append(w.Recipes, cw.Recipes...)
	w.Facilities["shop"] = cw.Facilities["shop"]
	w.Lines = append(w.Lines, cw.Lines...)
	costs := prodcost.Compute(w, prodcost.Options{})
	dir := t.TempDir()
	if err := renderPrivate(dir, w, costs, m, hp, prodcost.Opportunities(costs, w, m, hp), DataAges{}); err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile(`<tr><td><a href="[^"]*/x\.html">Ex</a></td>[^\n]*`).FindString(readOut(t, dir, "index.html"))
	if !strings.Contains(row, "&ge; ") || !strings.Contains(row, "cycle") {
		t.Errorf("baseline cycle row not a lower bound: %q", row)
	}
}
