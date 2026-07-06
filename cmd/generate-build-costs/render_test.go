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
	if err := renderIndex(dir, "index.html", sampleMatrix(), "Build Cost Matrix (Local)", tabs); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Widget", "Station One", "Show only feasible", "BoM", "Recipe"} {
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
	if err := renderDetail(dir, m.Rows[0], m.Stations, tgt, names, cats); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "widget.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Widget", "Station One", "BoM", "Recipe", "Feasible"} {
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
	row := MatrixRow{ID: "widget", Name: "Widget", Kind: "item", Cells: map[string]RowCell{}}
	names := map[string]string{"iron_ore": "Iron Ore", "iron_bar": "Iron Bar", "widget": "Widget"}
	cats := map[string]string{"iron_ore": "ore", "iron_bar": "refined", "widget": "component"}
	dir := t.TempDir()
	if err := renderDetail(dir, row, nil, tgt, names, cats); err != nil {
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
	if err := renderDetail(dir, row, nil, tgt, names, cats); err != nil {
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
