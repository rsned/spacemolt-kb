package bom

import (
	"testing"
)

func TestBuildRecipeMaps(t *testing.T) {
	recipes := map[string]*Recipe{
		"recipe_1": {
			ID: "recipe_1",
			Inputs: []RecipeItem{
				{ItemID: "iron_ore", Quantity: 2},
			},
			Outputs: []RecipeItem{
				{ItemID: "iron_plate", Quantity: 1},
			},
		},
		"recipe_2": {
			ID: "recipe_2",
			Inputs: []RecipeItem{
				{ItemID: "iron_ore", Quantity: 1},
				{ItemID: "coal", Quantity: 1},
			},
			Outputs: []RecipeItem{
				{ItemID: "steel_ingot", Quantity: 1},
			},
		},
		"recipe_3": {
			ID: "recipe_3",
			Inputs: []RecipeItem{
				{ItemID: "steel_ingot", Quantity: 2},
			},
			Outputs: []RecipeItem{
				{ItemID: "steel_plate", Quantity: 1},
				{ItemID: "steel_plate", Quantity: 2}, // Same item, multiple outputs
			},
		},
	}

	itemToRecipes, err := BuildRecipeMaps(recipes)
	if err != nil {
		t.Fatalf("BuildRecipeMaps() error = %v", err)
	}

	// Check iron_plate has 1 recipe
	if len(itemToRecipes["iron_plate"]) != 1 {
		t.Errorf("Expected 1 recipe for iron_plate, got %d", len(itemToRecipes["iron_plate"]))
	}

	// Check steel_ingot has 1 recipe
	if len(itemToRecipes["steel_ingot"]) != 1 {
		t.Errorf("Expected 1 recipe for steel_ingot, got %d", len(itemToRecipes["steel_ingot"]))
	}

	// Check steel_plate has 1 recipe (with multiple outputs for same item)
	if len(itemToRecipes["steel_plate"]) != 1 {
		t.Errorf("Expected 1 recipe for steel_plate, got %d", len(itemToRecipes["steel_plate"]))
	}

	// Check iron_ore has 0 recipes (it's an input, not output)
	if len(itemToRecipes["iron_ore"]) != 0 {
		t.Errorf("Expected 0 recipes for iron_ore, got %d", len(itemToRecipes["iron_ore"]))
	}

	// Check steel_plate recipe is recipe_3
	if itemToRecipes["steel_plate"][0].ID != "recipe_3" {
		t.Errorf("Expected recipe_3 for steel_plate, got %s", itemToRecipes["steel_plate"][0].ID)
	}
}

func TestSelectRecipe(t *testing.T) {
	// Create multiple recipes for the same item
	salvageRecipe := &Recipe{
		ID: "salvage_recipe",
		Inputs: []RecipeItem{
			{ItemID: "rare_salvage", Quantity: 1},
		},
		Outputs: []RecipeItem{
			{ItemID: "advanced_chip", Quantity: 1},
		},
	}

	normalRecipe1 := &Recipe{
		ID: "normal_recipe_1",
		Inputs: []RecipeItem{
			{ItemID: "silicon", Quantity: 1},
		},
		Outputs: []RecipeItem{
			{ItemID: "advanced_chip", Quantity: 1},
		},
	}

	normalRecipe2 := &Recipe{
		ID: "normal_recipe_2",
		Inputs: []RecipeItem{
			{ItemID: "silicon", Quantity: 2},
		},
		Outputs: []RecipeItem{
			{ItemID: "advanced_chip", Quantity: 2}, // More outputs
		},
	}

	itemToRecipes := map[string][]*Recipe{
		"advanced_chip": {salvageRecipe, normalRecipe1, normalRecipe2},
	}

	// Should select normal_recipe_2 (no salvage, most outputs)
	selected := SelectRecipe(itemToRecipes, "advanced_chip")
	if selected == nil {
		t.Fatal("SelectRecipe() returned nil")
	}

	if selected.ID != "normal_recipe_2" {
		t.Errorf("Expected normal_recipe_2, got %s", selected.ID)
	}
}

func TestSelectRecipe_SalvageOnly(t *testing.T) {
	// All recipes use salvage - should still select one
	salvageRecipe1 := &Recipe{
		ID: "salvage_recipe_1",
		Inputs: []RecipeItem{
			{ItemID: "rare_salvage", Quantity: 1},
		},
		Outputs: []RecipeItem{
			{ItemID: "ancient_relic", Quantity: 1},
		},
	}

	salvageRecipe2 := &Recipe{
		ID: "salvage_recipe_2",
		Inputs: []RecipeItem{
			{ItemID: "salvage_metal", Quantity: 2},
		},
		Outputs: []RecipeItem{
			{ItemID: "ancient_relic", Quantity: 2},
		},
	}

	itemToRecipes := map[string][]*Recipe{
		"ancient_relic": {salvageRecipe1, salvageRecipe2},
	}

	// Should select salvage_recipe_2 (more outputs, alphabetical tiebreak)
	selected := SelectRecipe(itemToRecipes, "ancient_relic")
	if selected == nil {
		t.Fatal("SelectRecipe() returned nil")
	}

	if selected.ID != "salvage_recipe_2" {
		t.Errorf("Expected salvage_recipe_2, got %s", selected.ID)
	}
}

func TestSelectRecipe_NoRecipes(t *testing.T) {
	itemToRecipes := map[string][]*Recipe{}

	selected := SelectRecipe(itemToRecipes, "nonexistent_item")
	if selected != nil {
		t.Errorf("Expected nil for nonexistent item, got %v", selected)
	}
}

// TestSelectRecipe_PackagingFilter verifies that wrap_/unwrap_ recipes are
// excluded when an alternative exists. Without this filter the production data
// hits cycles like enriched_uranium_rod ↔ contained_enriched_uranium_rod.
func TestSelectRecipe_PackagingFilter(t *testing.T) {
	recipes := map[string]*Recipe{
		"fabricate_enriched_uranium_rod": {
			ID:      "fabricate_enriched_uranium_rod",
			Inputs:  []RecipeItem{{ItemID: "low_enriched_uranium", Quantity: 2}},
			Outputs: []RecipeItem{{ItemID: "enriched_uranium_rod", Quantity: 1}},
		},
		"unwrap_enriched_uranium_rod": {
			ID:      "unwrap_enriched_uranium_rod",
			Inputs:  []RecipeItem{{ItemID: "contained_enriched_uranium_rod", Quantity: 1}},
			Outputs: []RecipeItem{{ItemID: "enriched_uranium_rod", Quantity: 2}},
		},
	}

	itemToRecipes, _ := BuildRecipeMaps(recipes)
	selected := SelectRecipe(itemToRecipes, "enriched_uranium_rod")

	if selected == nil || selected.ID != "fabricate_enriched_uranium_rod" {
		t.Errorf("expected fabricate_enriched_uranium_rod (max-output non-packaging), got %v", selected)
	}
}

// TestSelectRecipe_PackagingOnlyFallback verifies that if every candidate is a
// packaging recipe, SelectRecipe still returns one (rather than nil) so the
// caller can resolve the item rather than treating it as base.
func TestSelectRecipe_PackagingOnlyFallback(t *testing.T) {
	recipes := map[string]*Recipe{
		"unwrap_widget": {
			ID:      "unwrap_widget",
			Inputs:  []RecipeItem{{ItemID: "contained_widget", Quantity: 1}},
			Outputs: []RecipeItem{{ItemID: "widget", Quantity: 1}},
		},
	}

	itemToRecipes, _ := BuildRecipeMaps(recipes)
	selected := SelectRecipe(itemToRecipes, "widget")

	if selected == nil || selected.ID != "unwrap_widget" {
		t.Errorf("expected unwrap_widget as last-resort fallback, got %v", selected)
	}
}

// TestSelectRecipe_PrefersSourceable reproduces the control_node regression:
// three recipes produce the item, two of them tie on max output quantity, but
// one of those ties (superfluid_winding) requires an input with no market
// supply. With a sourceability set, the buyable path must win even though it is
// not the lexicographically- or iteration-first max-output recipe.
func TestSelectRecipe_PrefersSourceable(t *testing.T) {
	recipes := map[string]*Recipe{
		// Unsourceable: input has no market supply. Ties on output qty (2).
		"superfluid_winding": {
			ID:      "superfluid_winding",
			Inputs:  []RecipeItem{{ItemID: "superfluid_vial", Quantity: 2}},
			Outputs: []RecipeItem{{ItemID: "control_node", Quantity: 2}},
		},
		// Unsourceable: input has no market supply. Fewer outputs (1).
		"silica_lens_fabrication": {
			ID:      "silica_lens_fabrication",
			Inputs:  []RecipeItem{{ItemID: "silica_lens", Quantity: 2}},
			Outputs: []RecipeItem{{ItemID: "control_node", Quantity: 1}},
		},
		// Sourceable: every input is market-available. Ties on output qty (2).
		"assemble_control_node": {
			ID:      "assemble_control_node",
			Inputs:  []RecipeItem{{ItemID: "circuit_board", Quantity: 2}, {ItemID: "copper_wiring", Quantity: 4}},
			Outputs: []RecipeItem{{ItemID: "control_node", Quantity: 2}},
		},
	}
	itemToRecipes, _ := BuildRecipeMaps(recipes)
	sourceable := map[string]bool{"circuit_board": true, "copper_wiring": true}

	got := selectRecipe(itemToRecipes, "control_node", sourceable)
	if got == nil || got.ID != "assemble_control_node" {
		t.Errorf("with sourceability, expected assemble_control_node, got %v", got)
	}

	// Without a sourceability set, selection is structure-only: a max-output
	// recipe wins (either 2-output tie), never the 1-output silica recipe.
	structOnly := SelectRecipe(itemToRecipes, "control_node")
	if structOnly == nil || structOnly.Outputs[0].Quantity != 2 {
		t.Errorf("structure-only selection should pick a max-output (2) recipe, got %v", structOnly)
	}
}

// TestSelectRecipe_SourceableFallback verifies that when NO recipe is fully
// sourceable, the layer is skipped and the item still resolves (to the
// max-output recipe) rather than returning nil.
func TestSelectRecipe_SourceableFallback(t *testing.T) {
	recipes := map[string]*Recipe{
		"make_exotic_a": {
			ID:      "make_exotic_a",
			Inputs:  []RecipeItem{{ItemID: "unobtainium", Quantity: 1}},
			Outputs: []RecipeItem{{ItemID: "exotic", Quantity: 1}},
		},
		"make_exotic_b": {
			ID:      "make_exotic_b",
			Inputs:  []RecipeItem{{ItemID: "unobtainium", Quantity: 2}},
			Outputs: []RecipeItem{{ItemID: "exotic", Quantity: 3}},
		},
	}
	itemToRecipes, _ := BuildRecipeMaps(recipes)
	got := selectRecipe(itemToRecipes, "exotic", map[string]bool{}) // nothing sourceable
	if got == nil || got.ID != "make_exotic_b" {
		t.Errorf("with no sourceable path, expected fallback to max-output make_exotic_b, got %v", got)
	}
}

// TestComputeSourceable exercises the fixpoint: market-available base items seed
// the set; an intermediate becomes sourceable once all its inputs are; a chain
// that bottoms out in an unbuyable base item stays unsourceable; and a terminal
// item is sourceable only when market-available (never via a recipe).
func TestComputeSourceable(t *testing.T) {
	recipes := map[string]*Recipe{
		// widget <- gear (craftable) + iron_ore (market base)
		"make_widget": {
			ID:      "make_widget",
			Inputs:  []RecipeItem{{ItemID: "gear", Quantity: 1}, {ItemID: "iron_ore", Quantity: 2}},
			Outputs: []RecipeItem{{ItemID: "widget", Quantity: 1}},
		},
		// gear <- copper_ore (market base)
		"make_gear": {
			ID:      "make_gear",
			Inputs:  []RecipeItem{{ItemID: "copper_ore", Quantity: 1}},
			Outputs: []RecipeItem{{ItemID: "gear", Quantity: 1}},
		},
		// relic <- superfluid_vial (unbuyable base, no recipe, not market)
		"make_relic": {
			ID:      "make_relic",
			Inputs:  []RecipeItem{{ItemID: "superfluid_vial", Quantity: 1}},
			Outputs: []RecipeItem{{ItemID: "relic", Quantity: 1}},
		},
	}
	itemToRecipes, _ := BuildRecipeMaps(recipes)
	market := map[string]bool{"iron_ore": true, "copper_ore": true}
	// terminal: items with no producing recipe are base materials here.
	isTerminal := func(id string) bool { return len(itemToRecipes[id]) == 0 }

	src := ComputeSourceable(itemToRecipes, market, isTerminal)

	for _, id := range []string{"iron_ore", "copper_ore", "gear", "widget"} {
		if !src[id] {
			t.Errorf("expected %s to be sourceable", id)
		}
	}
	for _, id := range []string{"superfluid_vial", "relic"} {
		if src[id] {
			t.Errorf("expected %s to be UNsourceable", id)
		}
	}
}

// TestSelectRecipe_DeterministicTieBreak verifies that when candidates tie on
// total output, the lexicographically smallest recipe ID wins — so the result
// does not depend on map-iteration order. Runs many times to defeat the random
// seed of Go map iteration.
func TestSelectRecipe_DeterministicTieBreak(t *testing.T) {
	recipes := map[string]*Recipe{
		"zzz_make_gizmo": {
			ID:      "zzz_make_gizmo",
			Inputs:  []RecipeItem{{ItemID: "iron_ore", Quantity: 1}},
			Outputs: []RecipeItem{{ItemID: "gizmo", Quantity: 2}},
		},
		"aaa_make_gizmo": {
			ID:      "aaa_make_gizmo",
			Inputs:  []RecipeItem{{ItemID: "copper_ore", Quantity: 1}},
			Outputs: []RecipeItem{{ItemID: "gizmo", Quantity: 2}},
		},
		"mmm_make_gizmo": {
			ID:      "mmm_make_gizmo",
			Inputs:  []RecipeItem{{ItemID: "silicon_ore", Quantity: 1}},
			Outputs: []RecipeItem{{ItemID: "gizmo", Quantity: 2}},
		},
	}
	for i := 0; i < 50; i++ {
		itemToRecipes, _ := BuildRecipeMaps(recipes)
		got := SelectRecipe(itemToRecipes, "gizmo")
		if got == nil || got.ID != "aaa_make_gizmo" {
			t.Fatalf("iteration %d: expected aaa_make_gizmo (smallest ID among output ties), got %v", i, got)
		}
	}
}
