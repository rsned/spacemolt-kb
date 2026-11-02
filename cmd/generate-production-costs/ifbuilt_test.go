package main

import (
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/prodcost"
)

// shieldWorld makes a shield only at facilities: a NOIR public line (fee 5000)
// is the one usable route, and two unbuilt own levels beat it.
func shieldWorld() prodcost.World {
	w := steelWorld()
	w.Items["shield"] = catalog.Item{ID: "shield", Name: "Shield", Category: "component"}
	w.Recipes = append(w.Recipes, catalog.Recipe{ID: "build_shield", CraftingTime: 100,
		ProducedBy: []string{"shield_assembly", "shield_lab"},
		Inputs:     []catalog.Material{mat("iron_ore", 1)}, Outputs: []catalog.Material{mat("shield", 1)}})
	w.Facilities["shield_assembly"] = catalog.Facility{ID: "shield_assembly", Level: 1, LaborCost: 13,
		BuildCost: 1857000, BuildMaterials: []catalog.Material{mat("steel_plate", 10)}}
	w.Facilities["shield_lab"] = catalog.Facility{ID: "shield_lab", Level: 2, LaborCost: 26,
		BuildCost: 5536000, UpgradesFrom: "shield_assembly"}
	w.Lines = append(w.Lines, prodcost.Line{RecipeID: "build_shield", Level: 1, Fee: 5000, TicksPerRun: 15,
		StationName: "Nova Terra Central", FactionTag: "NOIR", Kind: prodcost.KindFaction})
	return w
}

func TestRenderPublicIfBuiltRoutes(t *testing.T) {
	w := shieldWorld()
	dir := t.TempDir()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	page := readOut(t, dir, "shield.html")
	for _, want := range []string{
		"if built",
		"Build cost", "Pays back after",
		// L1: 1,857,000 + 10 steel_plate at 0 (hand) = 1,857,000; saves 5013-13 = 5000/u.
		"1857000", ">372<",
		// L2 is cumulative from L1: 1,857,000 + 5,536,000; saves 5013-26 = 4987/u.
		"7393000", ">1483<",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("item page missing %q", want)
		}
	}
	for _, pg := range []string{page, readOut(t, dir, "index.html")} {
		if n := strings.Count(pg, `class="hl-cheap-if"`); n != 1 {
			t.Errorf("hl-cheap-if count %d, want 1 (own L1 is the cheapest if built)", n)
		}
		if n := strings.Count(pg, `class="hl-fast-if"`); n != 1 {
			t.Errorf("hl-fast-if count %d, want 1 (own L2 is the fastest if built)", n)
		}
	}
	// The usable NOIR line keeps the solid boxes.
	if !strings.Contains(page, `class="hl-cheap"`) {
		t.Errorf("usable route lost its cheapest box")
	}
}

func TestRenderPublicIfBuiltRowsOutsideDetails(t *testing.T) {
	w := shieldWorld()
	// A pricier, slower usable route is unremarkable and must stay collapsed.
	w.Lines = append(w.Lines, prodcost.Line{RecipeID: "build_shield", Level: 1, Fee: 6000, Empire: true})
	dir := t.TempDir()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	idx := readOut(t, dir, "index.html")
	start := strings.Index(idx, `href="shield.html"`)
	if start < 0 {
		t.Fatal("shield row missing")
	}
	body := idx[start:]
	body = body[:strings.Index(body, "</tbody>")]
	before, _, found := strings.Cut(body, "<details>")
	if !found {
		t.Fatal("expected a collapsed section for the remaining route")
	}
	if !strings.Contains(before, "own L1 (labor 13)") || !strings.Contains(before, "own L2 (labor 26)") {
		t.Errorf("if-built rows not shown outside the collapsed section")
	}
	if strings.Contains(before, "fee 6000") {
		t.Errorf("unremarkable route should stay collapsed")
	}
}

func TestRenderPublicExpandAll(t *testing.T) {
	w := steelWorld()
	dir := t.TempDir()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	idx := readOut(t, dir, "index.html")
	if !strings.Contains(idx, `id="pc-expand-all"`) || !strings.Contains(idx, "Expand all") {
		t.Errorf("index has no expand-all control")
	}
}

// interleavedWorld sorts its routes so recipe A's routes come before and after
// recipe B's, which must not lose any of them on the item page.
func TestRenderPublicItemPageKeepsInterleavedRoutes(t *testing.T) {
	w := prodcost.World{
		Items: map[string]catalog.Item{
			"ore":  {ID: "ore", Category: "ore", ExtractedBy: []string{"mining"}},
			"gear": {ID: "gear", Name: "Gear", Category: "component"},
		},
		Recipes: []catalog.Recipe{
			{ID: "gear_a", CraftingTime: 1, HandCraftable: true, ProducedBy: []string{"fa"},
				Inputs: []catalog.Material{mat("ore", 1)}, Outputs: []catalog.Material{mat("gear", 2)}},
			{ID: "gear_b", CraftingTime: 1, ProducedBy: []string{"fb"},
				Inputs: []catalog.Material{mat("ore", 1)}, Outputs: []catalog.Material{mat("gear", 1)}},
		},
		Facilities: map[string]catalog.Facility{
			"fa": {ID: "fa", Level: 1, LaborCost: 4},
			"fb": {ID: "fb", Level: 1, LaborCost: 1},
		},
		Lines: []prodcost.Line{
			{RecipeID: "gear_a", Level: 1, Fee: 0, Empire: true},
			{RecipeID: "gear_b", Level: 1, Fee: 0, Empire: true},
		},
	}
	dir := t.TempDir()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	page := readOut(t, dir, "gear.html")
	for _, want := range []string{"empire L1 (labor 4 + fee 0)", "empire L1 (labor 1 + fee 0)", "own L1 (labor 4)", "own L1 (labor 1)"} {
		if !strings.Contains(page, want) {
			t.Errorf("item page lost route %q", want)
		}
	}
}
