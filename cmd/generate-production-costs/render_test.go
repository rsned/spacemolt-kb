package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/prodcost"
)

func mat(id string, q float64) catalog.Material { return catalog.Material{ItemID: id, Quantity: q} }

// steelWorld produces steel_plate by hand and at an empire L1 refinery.
func steelWorld() prodcost.World {
	return prodcost.World{
		Items: map[string]catalog.Item{
			"iron_ore":    {ID: "iron_ore", Name: "Iron Ore", Category: "ore", ExtractedBy: []string{"mining"}},
			"steel_plate": {ID: "steel_plate", Name: "Steel Plate", Category: "component"},
		},
		Recipes: []catalog.Recipe{
			{ID: "basic_iron_smelting", CraftingTime: 0.27, HandCraftable: true,
				Inputs: []catalog.Material{mat("iron_ore", 3)}, Outputs: []catalog.Material{mat("steel_plate", 1)}},
			{ID: "refine_steel", CraftingTime: 0.54, ProducedBy: []string{"iron_refinery"},
				Inputs: []catalog.Material{mat("iron_ore", 5)}, Outputs: []catalog.Material{mat("steel_plate", 2)}},
		},
		Facilities: map[string]catalog.Facility{
			"iron_refinery": {ID: "iron_refinery", Level: 1, LaborCost: 10},
		},
		Lines: []prodcost.Line{{RecipeID: "refine_steel", Level: 1, Fee: 9, Empire: true}},
	}
}

func renderTo(t *testing.T, w prodcost.World) string {
	t.Helper()
	dir := t.TempDir()
	costs := prodcost.Compute(w, prodcost.Options{})
	if err := renderPublic(dir, w, costs, DataAges{Catalog: "20261101", Facilities: "2026-11-02T10:12:34Z"}); err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	files, _ := filepath.Glob(filepath.Join(dir, "*.html"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	return all.String()
}

func readOut(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRenderPublicIndexHighlights(t *testing.T) {
	w := steelWorld()
	dir := t.TempDir()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	idx := readOut(t, dir, "index.html")
	if !strings.Contains(idx, "empire L1 (labor 10 + fee 9)") {
		t.Errorf("missing empire label")
	}
	if n := strings.Count(idx, `class="hl-cheap"`); n != 1 {
		t.Errorf("hl-cheap count %d", n)
	}
	if n := strings.Count(idx, `class="hl-fast"`); n != 1 {
		t.Errorf("hl-fast count %d", n)
	}
	if strings.Contains(idx, "Ask") || strings.Contains(idx, "Bid") {
		t.Errorf("market column leaked")
	}
}

func TestRenderPublicSingleRouteNoBox(t *testing.T) {
	w := steelWorld()
	w.Lines = nil
	w.Recipes = w.Recipes[:1]
	out := renderTo(t, w)
	if strings.Contains(out, `class="hl-cheap"`) || strings.Contains(out, `class="hl-fast"`) {
		t.Errorf("single-route item was boxed")
	}
}

func plasmaWorld() prodcost.World {
	return prodcost.World{
		Items: map[string]catalog.Item{
			"iron_ore":        {ID: "iron_ore", Category: "ore", ExtractedBy: []string{"mining"}},
			"mystery_part":    {ID: "mystery_part", Name: "Mystery Part", Category: "component"},
			"plasma_injector": {ID: "plasma_injector", Name: "Plasma Injector", Category: "component"},
		},
		Recipes: []catalog.Recipe{
			{ID: "make_plasma_injector", CraftingTime: 1, HandCraftable: true,
				Inputs: []catalog.Material{mat("iron_ore", 2), mat("mystery_part", 1)}, Outputs: []catalog.Material{mat("plasma_injector", 1)}},
			{ID: "make_plasma_injector_alt", CraftingTime: 2, HandCraftable: true,
				Inputs: []catalog.Material{mat("iron_ore", 4), mat("mystery_part", 1)}, Outputs: []catalog.Material{mat("plasma_injector", 1)}},
		},
	}
}

func TestRenderPublicItemPage(t *testing.T) {
	w := plasmaWorld()
	dir := t.TempDir()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	page := readOut(t, dir, "plasma_injector.html")
	for _, want := range []string{"make_plasma_injector", "make_plasma_injector_alt", "has an unpriced input",
		"../items/component/plasma_injector.html"} {
		if !strings.Contains(page, want) {
			t.Errorf("item page missing %q", want)
		}
	}
	// Build-cost pages are deprecated and no longer published (2026-11-10).
	if strings.Contains(page, "build-costs/") {
		t.Error("item page still links to the unpublished build-cost pages")
	}
}

func TestRenderPublicNoPrivateData(t *testing.T) {
	w := steelWorld()
	w.Lines = append(w.Lines, prodcost.Line{RecipeID: "refine_steel", Level: 2, FactionTag: "CRFT",
		StationName: "Conjunction Junction", Kind: prodcost.KindCRFT})
	out := strings.ToLower(renderTo(t, w))
	if strings.Contains(out, "crft") {
		t.Errorf("private crft data in public output")
	}
}

func TestRenderPublicCleanupOnlyRemovesOwnPages(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "foreign.html")
	stale := filepath.Join(dir, "stale.html")
	if err := os.WriteFile(foreign, []byte("<html>someone else</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte(pageMarker+"\n<html>old</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := steelWorld()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("foreign page removed: %v", err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Errorf("stale marked page kept")
	}
	if !strings.Contains(readOut(t, dir, "index.html"), pageMarker) {
		t.Errorf("index lacks marker")
	}
}

// cycleWorld has x and y each made only from the other at an empire L1, so
// both are costed as a cycle: a floor, like a partial cost.
func cycleWorld() prodcost.World {
	return prodcost.World{
		Items: map[string]catalog.Item{
			"x": {ID: "x", Name: "Ex", Category: "component", Size: 1},
			"y": {ID: "y", Name: "Why", Category: "component", Size: 1},
		},
		Recipes: []catalog.Recipe{
			{ID: "make_x", CraftingTime: 1, ProducedBy: []string{"shop"},
				Inputs: []catalog.Material{mat("y", 1)}, Outputs: []catalog.Material{mat("x", 1)}},
			{ID: "make_y", CraftingTime: 1, ProducedBy: []string{"shop"},
				Inputs: []catalog.Material{mat("x", 1)}, Outputs: []catalog.Material{mat("y", 1)}},
		},
		Facilities: map[string]catalog.Facility{"shop": {ID: "shop", Level: 1, LaborCost: 10}},
		Lines: []prodcost.Line{{RecipeID: "make_x", Level: 1, Fee: 5, Empire: true},
			{RecipeID: "make_y", Level: 1, Fee: 5, Empire: true}},
	}
}

func TestRenderPublicCycleIsLowerBound(t *testing.T) {
	w := cycleWorld()
	dir := t.TempDir()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "x.html"} {
		out := readOut(t, dir, f)
		if !strings.Contains(out, "≥ ") || !strings.Contains(out, "cycle") {
			t.Errorf("%s: cycle cost not shown as a lower bound", f)
		}
	}
}

func TestRenderPublicEmpireUniformNote(t *testing.T) {
	w := steelWorld()
	dir := t.TempDir()
	if err := renderPublic(dir, w, prodcost.Compute(w, prodcost.Options{}), DataAges{}); err != nil {
		t.Fatal(err)
	}
	page := readOut(t, dir, "steel_plate.html")
	row := regexp.MustCompile(`<tr><td>empire L1 \(labor 10 \+ fee 9\)[^\n]*`).FindString(page)
	if !strings.Contains(row, "uniform at all empire stations") {
		t.Errorf("empire row lacks the uniform note: %q", row)
	}
	if n := strings.Count(page, "uniform at all empire stations"); n != 1 {
		t.Errorf("uniform note shown %d times, want once (empire row only)", n)
	}
}
