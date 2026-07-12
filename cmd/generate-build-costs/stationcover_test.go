package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func TestBuildStationCoverPage_LastUpdatedFlowsToPage(t *testing.T) {
	p := buildStationCoverPage(nil, buildcost.StationDepth{}, nil, nil, nil, nil, "2026-07-12 15:00 UTC")
	if p.LastUpdated != "2026-07-12 15:00 UTC" {
		t.Fatalf("LastUpdated = %q, want %q", p.LastUpdated, "2026-07-12 15:00 UTC")
	}
}

func TestStationDepthFromBooks_SumsLadder(t *testing.T) {
	books := map[string]*buildcost.Book{
		"A": {Sell: map[string]buildcost.Ladder{
			"iron": {{Price: 5, Qty: 3}, {Price: 6, Qty: 4}},
		}},
	}
	depth := stationDepthFromBooks(books)
	if depth["A"]["iron"] != 7 {
		t.Fatalf("depth = %v, want 7", depth["A"]["iron"])
	}
}

func TestBuildStationCoverPage_PartitionAndStats(t *testing.T) {
	targets := []buildcost.Target{
		{ID: "easy", Kind: "item", BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 1}}},
		{ID: "hard", Kind: "item", BoM: []buildcost.Requirement{
			{ItemID: "iron", Qty: 1}, {ItemID: "gold", Qty: 1}}},
		{ID: "nope", Kind: "ship", BoM: []buildcost.Requirement{{ItemID: "exotic", Qty: 1}},
			RecipeNA: "sub-assemblies not market-traded"},
	}
	depth := buildcost.StationDepth{
		"A": {"iron": 10},
		"B": {"gold": 10},
	}
	names := map[string]string{"A": "Alpha", "B": "Bravo"}
	items := map[string]string{"easy": "Easy", "hard": "Hard", "nope": "Nope", "exotic": "Exotic Matter"}
	cats := map[string]string{"easy": "widget", "hard": "gadget", "nope": "frigate"}
	p := buildStationCoverPage(targets, depth, []string{"A", "B"}, names, items, cats, "")

	if p.Total != 3 {
		t.Fatalf("Total = %d, want 3", p.Total)
	}
	if len(p.Buildable) != 2 {
		t.Fatalf("buildable = %d, want 2", len(p.Buildable))
	}
	// Sorted by BoM count desc then name: hard(2) before easy(1).
	if p.Buildable[0].ID != "hard" || p.Buildable[1].ID != "easy" {
		t.Fatalf("order = %s,%s want hard,easy", p.Buildable[0].ID, p.Buildable[1].ID)
	}
	if p.SingleStation != 1 {
		t.Errorf("SingleStation = %d, want 1", p.SingleStation)
	}
	if p.MaxStations != 2 || p.HardestID != "hard" {
		t.Errorf("hardest = %d/%s, want 2/hard", p.MaxStations, p.HardestID)
	}
	if len(p.Unbuildable) != 1 || p.Unbuildable[0].ID != "nope" {
		t.Fatalf("unbuildable = %+v, want [nope]", p.Unbuildable)
	}
	if !reflect.DeepEqual(p.Unbuildable[0].Missing, []string{"Exotic Matter"}) {
		t.Errorf("missing = %v, want [Exotic Matter]", p.Unbuildable[0].Missing)
	}
	// easy's example cover maps station ids to display names.
	if !reflect.DeepEqual(p.Buildable[1].ExampleCover, []string{"Alpha"}) {
		t.Errorf("easy cover = %v, want [Alpha]", p.Buildable[1].ExampleCover)
	}
}

func TestBuildStationCoverPage_RecipeBestFeasible(t *testing.T) {
	// Two recipes: r1 needs a rare input (infeasible), r2 needs a common one.
	targets := []buildcost.Target{{
		ID: "widget", Kind: "item",
		BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 1}},
		Recipes: []buildcost.Recipe{
			{ID: "r1", OutputQty: 1, Inputs: []buildcost.Requirement{{ItemID: "rare", Qty: 1}}},
			{ID: "r2", OutputQty: 1, Inputs: []buildcost.Requirement{{ItemID: "iron", Qty: 1}}},
		},
	}}
	depth := buildcost.StationDepth{"A": {"iron": 10}}
	p := buildStationCoverPage(targets, depth, []string{"A"},
		map[string]string{"A": "Alpha"}, map[string]string{"widget": "Widget"},
		map[string]string{"widget": "widget"}, "")
	e := p.Buildable[0]
	if e.RecipeNA {
		t.Fatalf("recipe should be feasible via r2, got NA")
	}
	if !e.Recipe.Feasible || e.Recipe.Count != 1 {
		t.Fatalf("recipe cover = %+v, want feasible count 1", e.Recipe)
	}
}

func TestRenderStationCover_WritesExpectedContent(t *testing.T) {
	p := stationCoverPage{
		Total: 3, SingleStation: 1, MaxStations: 2, UnbuildableCount: 1,
		BatchSensitiveCount: 1, HardestName: "Hard Widget", HardestID: "hard",
		AvgStations: 1.5,
		Buildable: []coverEntry{
			{ID: "hard", Name: "Hard Widget", Category: "gadget", Kind: "item",
				BoM:    buildcost.CoverResult{Feasible: true, Count: 2, Exact: true},
				Recipe: buildcost.CoverResult{Feasible: true, Count: 1, Exact: true},
				ExampleCover: []string{"Alpha", "Bravo"}},
			{ID: "easy", Name: "Easy Widget", Category: "widget", Kind: "item",
				BoM: buildcost.CoverResult{Feasible: true, Count: 1, Exact: true}, RecipeNA: true,
				ExampleCover: []string{"Alpha"}, BatchSensitive: true},
		},
		Unbuildable: []unbuildableEntry{
			{ID: "nope", Name: "Void Reactor", Category: "reactor", Kind: "ship", Missing: []string{"Exotic Matter"}},
		},
		BoMHistogram:    []histBar{{Stations: 1, Count: 1, HeightPct: 100}, {Stations: 2, Count: 1, HeightPct: 100}},
		RecipeHistogram: []histBar{{Stations: 1, Count: 1, HeightPct: 100}},
	}
	dir := t.TempDir()
	out := dir + "/stations_to_build.html"
	if err := renderStationCover(out, p); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"Hard Widget",         // hardest factoid + main-table row
		"../build-costs/hard.html", // target link
		"Alpha, Bravo",        // example cover
		"N/A",                 // easy's recipe column
		"Void Reactor",        // unbuildable row
		"Exotic Matter",       // missing input
		"chart-bar",           // bar chart present
	} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
}
