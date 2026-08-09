package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func sampleMatrix() Matrix {
	targets := []buildcost.Target{{ID: "widget", Kind: "item", BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 2}}}}
	books := map[string]*buildcost.Book{"st1": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 100}}}, BestBuy: map[string]float64{}}}
	stations := []StationMeta{{ID: "st1", Name: "Station One", Empire: "Sol"}}
	return BuildMatrix(targets, books, books, stations, map[string]string{"widget": "Widget"}, map[string]string{"widget": "Module"}, nil, nil)
}

func TestRenderIndex_WritesFileWithData(t *testing.T) {
	dir := t.TempDir()
	tabs := []radiusTab{{Label: "Local", File: "index.html", Active: true}}
	summary := "Of 1 targets, 100% are buildable as BoM, 0% as recipe, 100% by either — sourcing inputs within Local."
	if err := renderIndex(dir, "index.html", sampleMatrix(), "Build Cost Matrix (Local)", summary, tabs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Widget", "Station One", "Show only feasible", "BoM", "Recipe", "buildable as BoM", "facilities/", "Facility build costs"} {
		if !strings.Contains(s, want) {
			t.Fatalf("index.html missing %q", want)
		}
	}
}

func TestMatrixJSON_Valid(t *testing.T) {
	js, err := matrixJSON(sampleMatrix())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, "widget") {
		t.Fatalf("json missing target id: %s", js)
	}
}

func TestCommaInt(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want string
	}{
		{"positive with grouping", 1894, "1,894"},
		{"zero", 0, "0"},
		{"three digits no grouping", 999, "999"},
		{"millions with grouping", 1000000, "1,000,000"},
		{"negative with grouping", -4106, "-4,106"},
		{"negative single digit", -1, "-1"},
		{"negative near million", -999999, "-999,999"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := commaInt(tc.in); got != tc.want {
				t.Fatalf("commaInt(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRenderDetail_WritesTable(t *testing.T) {
	dir := t.TempDir()
	m := sampleMatrix()
	tgt := buildcost.Target{ID: "widget", Kind: "item", BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 2}}}
	names := map[string]string{"widget": "Widget"}
	cats := map[string]string{"widget": "Module"}
	hopRows := [4]MatrixRow{m.Rows[0], m.Rows[0], m.Rows[0], m.Rows[0]}
	if err := renderDetail(dir, m.Rows[0], m.Stations, tgt, names, cats, hopRows, galaxyCover{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "widget.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Widget", "Station One", "BoM", "Recipe", "Recipe used"} {
		if !strings.Contains(s, want) {
			t.Fatalf("detail missing %q", want)
		}
	}
}

func TestRenderDetail_BoMAndRecipeTables(t *testing.T) {
	tgt := buildcost.Target{
		ID: "widget", Kind: "item",
		BoM:     []buildcost.Requirement{{ItemID: "iron_ore", Qty: 3}},
		Recipes: []buildcost.Recipe{{ID: "build_widget", OutputQty: 1, Inputs: []buildcost.Requirement{{ItemID: "iron_bar", Qty: 2}}}},
	}
	base := MatrixRow{ID: "widget", Name: "Widget", Kind: "item", Cells: map[string]RowCell{
		"st1": {BoMCost: 100, BoMFeasible: true, RecipeCost: 120, RecipeFeasible: true},
	}}
	hopRows := [4]MatrixRow{base, base, base, base}
	stations := []StationMeta{{ID: "st1", Name: "Station One", Empire: "Independent"}}
	names := map[string]string{"iron_ore": "Iron Ore", "iron_bar": "Iron Bar", "widget": "Widget"}
	cats := map[string]string{"iron_ore": "ore", "iron_bar": "refined", "widget": "component"}
	dir := t.TempDir()
	if err := renderDetail(dir, base, stations, tgt, names, cats, hopRows, galaxyCover{}); err != nil {
		t.Fatalf("renderDetail: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "widget.html"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	html := string(b)
	for _, want := range []string{
		`href="../items/ore/iron_ore.html">Iron Ore`,     // BoM input linked
		`href="../items/refined/iron_bar.html">Iron Bar`, // recipe input linked
		`href="../items/component/widget.html"`,          // self link
		"build_widget",                                   // recipe id heading
		"Bill of Materials",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("detail html missing %q", want)
		}
	}
	for _, want := range []string{"BoM +1", "Recipe +1", "100"} {
		if !strings.Contains(html, want) {
			t.Errorf("detail html missing %q", want)
		}
	}
}

func TestRenderDetail_GalaxyBannerWhenAllInfeasible(t *testing.T) {
	// One station, infeasible for BoM and Recipe at every radius → the table is
	// all em-dashes, so the galaxy-wide banner must appear with the cover.
	tgt := buildcost.Target{ID: "gizmo", Kind: "item", BoM: []buildcost.Requirement{{ItemID: "rare", Qty: 5}}}
	// Infeasible, but 2 of 5 BoM inputs and 1 of 3 recipe inputs are sourceable →
	// cells show coverage fractions rather than a bare em-dash.
	cell := RowCell{BoMFeasible: false, BoMCovered: 2, BoMTotal: 5, RecipeFeasible: false, RecipeCovered: 1, RecipeTotal: 3}
	row := MatrixRow{ID: "gizmo", Name: "Gizmo", Kind: "item", Cells: map[string]RowCell{"S1": cell}}
	stations := []StationMeta{{ID: "S1", Name: "Station One", Empire: "Independent"}}
	names := map[string]string{"gizmo": "Gizmo", "rare": "Rare Ore"}
	cats := map[string]string{"gizmo": "component"}
	hopRows := [4]MatrixRow{row, row, row, row}
	cover := galaxyCover{Feasible: true, Count: 2, Exact: true, Stations: []string{"Alpha", "Bravo"}}
	dir := t.TempDir()
	if err := renderDetail(dir, row, stations, tgt, names, cats, hopRows, cover); err != nil {
		t.Fatalf("renderDetail: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "gizmo.html"))
	html := string(b)
	for _, want := range []string{
		"Not buildable within 3 jumps", "2 station", "Alpha", "Bravo",
		"../did_you_know/stations_to_build.html",
		"2/5", // BoM coverage fraction in the infeasible cell
		"1/3", // Recipe coverage fraction
	} {
		if !strings.Contains(html, want) {
			t.Errorf("banner missing %q", want)
		}
	}
}

func TestRenderDetail_NoBannerWhenLocallyFeasible(t *testing.T) {
	// A station where BoM is feasible at radius 0 → no banner.
	tgt := buildcost.Target{ID: "gizmo", Kind: "item", BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 1}}}
	cell := RowCell{BoMFeasible: true, BoMCost: 10}
	row := MatrixRow{ID: "gizmo", Name: "Gizmo", Kind: "item", Cells: map[string]RowCell{"S1": cell}}
	stations := []StationMeta{{ID: "S1", Name: "Station One", Empire: "Independent"}}
	names := map[string]string{"gizmo": "Gizmo", "iron": "Iron Ore"}
	cats := map[string]string{"gizmo": "component"}
	hopRows := [4]MatrixRow{row, row, row, row}
	cover := galaxyCover{Feasible: true, Count: 1, Exact: true, Stations: []string{"Station One"}}
	dir := t.TempDir()
	if err := renderDetail(dir, row, stations, tgt, names, cats, hopRows, cover); err != nil {
		t.Fatalf("renderDetail: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "gizmo.html"))
	if strings.Contains(string(b), "Not buildable within 3 jumps") {
		t.Errorf("banner should not appear when the target is locally feasible")
	}
}

func TestRenderDetail_ShipNoRecipe(t *testing.T) {
	tgt := buildcost.Target{
		ID: "cobble", Kind: "ship",
		BoM:      []buildcost.Requirement{{ItemID: "refined_alloy", Qty: 10}},
		RecipeNA: "sub-assemblies not market-traded",
	}
	row := MatrixRow{ID: "cobble", Name: "Cobble", Kind: "ship", Cells: map[string]RowCell{}}
	names := map[string]string{"refined_alloy": "Refined Alloy", "cobble": "Cobble"}
	cats := map[string]string{"refined_alloy": "refined"}
	dir := t.TempDir()
	hopRows := [4]MatrixRow{row, row, row, row}
	if err := renderDetail(dir, row, nil, tgt, names, cats, hopRows, galaxyCover{}); err != nil {
		t.Fatalf("renderDetail: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "cobble.html"))
	html := string(b)
	if !strings.Contains(html, "sub-assemblies not market-traded") {
		t.Errorf("ship detail missing RecipeNA message")
	}
	if !strings.Contains(html, `../ships/all.html`) {
		t.Errorf("ship detail missing ship-index self link")
	}
	if strings.Contains(html, "<h3>") {
		t.Errorf("ship detail should have no recipe (<h3>) tables")
	}
}

func TestExplorerLinkable(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		id         string
		categories map[string]string
		recipes    []buildcost.Recipe
		want       bool
	}{
		{
			name:       "terminal ore item is not linkable",
			kind:       "item",
			id:         "iron_ore",
			categories: map[string]string{"iron_ore": "ore"},
			want:       false,
		},
		{
			// contained_* items are produced only by wrap_* packaging recipes,
			// which the explorer deliberately drops — so despite having a
			// producing recipe here, the explorer treats this as a drop.
			name:       "contained_* item made only by a wrap_ recipe is not linkable",
			kind:       "item",
			id:         "contained_liquid_tritium",
			categories: map[string]string{"contained_liquid_tritium": "refined"},
			recipes:    []buildcost.Recipe{{ID: "wrap_liquid_tritium", OutputQty: 1}},
			want:       false,
		},
		{
			name:       "normal item with a non-packaging recipe is linkable",
			kind:       "item",
			id:         "power_core",
			categories: map[string]string{"power_core": "component"},
			recipes:    []buildcost.Recipe{{ID: "assemble_power_core", OutputQty: 1}},
			want:       true,
		},
		{
			name: "ship is always linkable",
			kind: "ship",
			id:   "cobble",
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := explorerLinkable(tc.kind, tc.id, tc.categories, tc.recipes); got != tc.want {
				t.Errorf("explorerLinkable(%q, %q) = %v, want %v", tc.kind, tc.id, got, tc.want)
			}
		})
	}
}

func TestRenderDetailLinksToExplorer(t *testing.T) {
	dir := t.TempDir()
	row := MatrixRow{ID: "power_core", Name: "Power Core", Kind: "item", Cells: map[string]RowCell{}}
	tgt := buildcost.Target{
		ID:   "power_core",
		BoM:  []buildcost.Requirement{{ItemID: "iron_bar", Qty: 2}},
		Recipes: []buildcost.Recipe{{ID: "assemble_power_core", OutputQty: 1,
			Inputs: []buildcost.Requirement{{ItemID: "iron_bar", Qty: 2}}}},
	}
	names := map[string]string{"iron_bar": "Iron Bar", "power_core": "Power Core"}
	categories := map[string]string{"iron_bar": "refined", "power_core": "component"}

	if err := renderDetail(dir, row, nil, tgt, names, categories, [4]MatrixRow{}, galaxyCover{}); err != nil {
		t.Fatalf("renderDetail: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "power_core.html"))
	if err != nil {
		t.Fatal(err)
	}
	want := `href="explorer.html?target=power_core"`
	if !strings.Contains(string(raw), want) {
		t.Errorf("detail page missing explorer link %s", want)
	}
	if !strings.Contains(string(raw), "Explore this BoM interactively") {
		t.Error("detail page missing the explorer link text")
	}
}
