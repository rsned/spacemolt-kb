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
