package bom

import (
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestCalculate_BaseMaterial tests that ore and material items stop recursion
func TestCalculate_BaseMaterial(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create test data
	recipes := map[string]*Recipe{}
	items := map[string]*Item{
		"iron_ore": {ID: "iron_ore", Name: "Iron Ore", Type: "resource", Category: "ore"},
		"copper":   {ID: "copper", Name: "Copper", Type: "material", Category: "material"},
	}

	calc, err := NewCalculator(db, recipes, items)
	if err != nil {
		t.Fatalf("failed to create calculator: %v", err)
	}

	// Test ore item
	materials, err := calc.Calculate("iron_ore", 10)
	if err != nil {
		t.Errorf("Calculate(iron_ore, 10) returned error: %v", err)
	}
	if len(materials) != 1 {
		t.Errorf("Expected 1 material, got %d", len(materials))
	}
	if materials[0].ItemID != "iron_ore" {
		t.Errorf("Expected item iron_ore, got %s", materials[0].ItemID)
	}
	if materials[0].Quantity != 10 {
		t.Errorf("Expected quantity 10, got %d", materials[0].Quantity)
	}

	// Test material item
	materials, err = calc.Calculate("copper", 5)
	if err != nil {
		t.Errorf("Calculate(copper, 5) returned error: %v", err)
	}
	if len(materials) != 1 {
		t.Errorf("Expected 1 material, got %d", len(materials))
	}
	if materials[0].ItemID != "copper" {
		t.Errorf("Expected item copper, got %s", materials[0].ItemID)
	}
	if materials[0].Quantity != 5 {
		t.Errorf("Expected quantity 5, got %d", materials[0].Quantity)
	}
}

// TestCalculate_UnknownItem tests that unknown items are treated as base materials
func TestCalculate_UnknownItem(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	recipes := map[string]*Recipe{}
	items := map[string]*Item{}

	calc, err := NewCalculator(db, recipes, items)
	if err != nil {
		t.Fatalf("failed to create calculator: %v", err)
	}

	materials, err := calc.Calculate("unknown_item", 7)
	if err != nil {
		t.Errorf("Calculate(unknown_item, 7) returned error: %v", err)
	}
	if len(materials) != 1 {
		t.Errorf("Expected 1 material, got %d", len(materials))
	}
	if materials[0].ItemID != "unknown_item" {
		t.Errorf("Expected item unknown_item, got %s", materials[0].ItemID)
	}
	if materials[0].Quantity != 7 {
		t.Errorf("Expected quantity 7, got %d", materials[0].Quantity)
	}
}

// TestCalculate_MultiTier tests complex multi-tier calculation
func TestCalculate_MultiTier(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create test data with a 3-tier recipe:
	// Steel Plate (1) = 2 Iron Ingot
	// Iron Ingot (1) = 5 Iron Ore
	// Iron Ore = base material
	recipes := map[string]*Recipe{
		"iron_ingot": {
			ID: "iron_ingot",
			Inputs: []RecipeItem{
				{ItemID: "iron_ore", Quantity: 5},
			},
			Outputs: []RecipeItem{
				{ItemID: "iron_ingot", Quantity: 1},
			},
		},
		"steel_plate": {
			ID: "steel_plate",
			Inputs: []RecipeItem{
				{ItemID: "iron_ingot", Quantity: 2},
			},
			Outputs: []RecipeItem{
				{ItemID: "steel_plate", Quantity: 1},
			},
		},
	}

	items := map[string]*Item{
		"iron_ore":    {ID: "iron_ore", Name: "Iron Ore", Type: "resource", Category: "ore"},
		"iron_ingot":  {ID: "iron_ingot", Name: "Iron Ingot", Type: "component", Category: "component"},
		"steel_plate": {ID: "steel_plate", Name: "Steel Plate", Type: "component", Category: "component"},
	}

	calc, err := NewCalculator(db, recipes, items)
	if err != nil {
		t.Fatalf("failed to create calculator: %v", err)
	}

	// Test: 1 Steel Plate = 10 Iron Ore
	materials, err := calc.Calculate("steel_plate", 1)
	if err != nil {
		t.Errorf("Calculate(steel_plate, 1) returned error: %v", err)
	}

	// Should have 1 aggregated material
	if len(materials) != 1 {
		t.Errorf("Expected 1 aggregated material, got %d", len(materials))
	}

	if materials[0].ItemID != "iron_ore" {
		t.Errorf("Expected item iron_ore, got %s", materials[0].ItemID)
	}

	// 1 Steel Plate = 2 Iron Ingot = 10 Iron Ore
	if materials[0].Quantity != 10 {
		t.Errorf("Expected quantity 10, got %d", materials[0].Quantity)
	}

	// Test with different quantity: 3 Steel Plates = 30 Iron Ore
	materials, err = calc.Calculate("steel_plate", 3)
	if err != nil {
		t.Errorf("Calculate(steel_plate, 3) returned error: %v", err)
	}

	if len(materials) != 1 {
		t.Errorf("Expected 1 aggregated material, got %d", len(materials))
	}

	if materials[0].Quantity != 30 {
		t.Errorf("Expected quantity 30, got %d", materials[0].Quantity)
	}
}

// TestCalculate_CircularDependency tests cycle detection
func TestCalculate_CircularDependency(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Create circular recipe: A -> B -> A
	recipes := map[string]*Recipe{
		"item_a": {
			ID: "item_a",
			Inputs: []RecipeItem{
				{ItemID: "item_b", Quantity: 1},
			},
			Outputs: []RecipeItem{
				{ItemID: "item_a", Quantity: 1},
			},
		},
		"item_b": {
			ID: "item_b",
			Inputs: []RecipeItem{
				{ItemID: "item_a", Quantity: 1},
			},
			Outputs: []RecipeItem{
				{ItemID: "item_b", Quantity: 1},
			},
		},
	}

	items := map[string]*Item{
		"item_a": {ID: "item_a", Name: "Item A", Type: "component", Category: "component"},
		"item_b": {ID: "item_b", Name: "Item B", Type: "component", Category: "component"},
	}

	calc, err := NewCalculator(db, recipes, items)
	if err != nil {
		t.Fatalf("failed to create calculator: %v", err)
	}

	// Should detect circular dependency
	_, err = calc.Calculate("item_a", 1)
	if err == nil {
		t.Error("Expected circular dependency error, got nil")
	}

	if err != nil && err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}

// TestCalculate_Memoization tests that memoization works correctly
func TestCalculate_Memoization(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Simple recipe: Component = 2 Ore
	recipes := map[string]*Recipe{
		"component": {
			ID: "component",
			Inputs: []RecipeItem{
				{ItemID: "ore", Quantity: 2},
			},
			Outputs: []RecipeItem{
				{ItemID: "component", Quantity: 1},
			},
		},
	}

	items := map[string]*Item{
		"ore":       {ID: "ore", Name: "Ore", Type: "resource", Category: "ore"},
		"component": {ID: "component", Name: "Component", Type: "component", Category: "component"},
	}

	calc, err := NewCalculator(db, recipes, items)
	if err != nil {
		t.Fatalf("failed to create calculator: %v", err)
	}

	// First calculation
	materials1, err := calc.Calculate("component", 1)
	if err != nil {
		t.Errorf("First Calculate returned error: %v", err)
	}

	// Second calculation should use memoization
	materials2, err := calc.Calculate("component", 1)
	if err != nil {
		t.Errorf("Second Calculate returned error: %v", err)
	}

	// Results should be identical
	if len(materials1) != len(materials2) {
		t.Errorf("Memoization mismatch: %d vs %d materials", len(materials1), len(materials2))
	}

	for i := range materials1 {
		if materials1[i].ItemID != materials2[i].ItemID {
			t.Errorf("Memoization mismatch at index %d: %s vs %s", i, materials1[i].ItemID, materials2[i].ItemID)
		}
		if materials1[i].Quantity != materials2[i].Quantity {
			t.Errorf("Memoization mismatch at index %d: %d vs %d", i, materials1[i].Quantity, materials2[i].Quantity)
		}
	}
}

// TestCalculate_MultipleOutputs tests recipes with multiple output items
func TestCalculate_MultipleOutputs(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Recipe with multiple outputs
	recipes := map[string]*Recipe{
		"multi_output": {
			ID: "multi_output",
			Inputs: []RecipeItem{
				{ItemID: "ore", Quantity: 4},
			},
			Outputs: []RecipeItem{
				{ItemID: "item_a", Quantity: 2},
				{ItemID: "item_b", Quantity: 2},
			},
		},
	}

	items := map[string]*Item{
		"ore":        {ID: "ore", Name: "Ore", Type: "resource", Category: "ore"},
		"item_a":     {ID: "item_a", Name: "Item A", Type: "component", Category: "component"},
		"item_b":     {ID: "item_b", Name: "Item B", Type: "component", Category: "component"},
	}

	calc, err := NewCalculator(db, recipes, items)
	if err != nil {
		t.Fatalf("failed to create calculator: %v", err)
	}

	// 4 item_a should require 4 ore (recipe produces 2 per batch)
	materials, err := calc.Calculate("item_a", 4)
	if err != nil {
		t.Errorf("Calculate(item_a, 4) returned error: %v", err)
	}

	if len(materials) != 1 {
		t.Errorf("Expected 1 material, got %d", len(materials))
	}

	if materials[0].ItemID != "ore" {
		t.Errorf("Expected item ore, got %s", materials[0].ItemID)
	}

	// 4 item_a = 2 recipe runs (4/2) = 8 ore
	if materials[0].Quantity != 8 {
		t.Errorf("Expected quantity 8, got %d", materials[0].Quantity)
	}
}

// TestCalculateAll tests batch calculation for items, ships, and facilities
func TestCalculateAll(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	// Migrate the database schema
	if err := Migrate(db); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Create test recipes
	recipes := map[string]*Recipe{
		"iron_ingot": {
			ID: "iron_ingot",
			Inputs: []RecipeItem{
				{ItemID: "iron_ore", Quantity: 5},
			},
			Outputs: []RecipeItem{
				{ItemID: "iron_ingot", Quantity: 1},
			},
		},
		"steel_plate": {
			ID: "steel_plate",
			Inputs: []RecipeItem{
				{ItemID: "iron_ingot", Quantity: 2},
			},
			Outputs: []RecipeItem{
				{ItemID: "steel_plate", Quantity: 1},
			},
		},
	}

	// Create test items
	items := map[string]*Item{
		"iron_ore":    {ID: "iron_ore", Name: "Iron Ore", Type: "resource", Category: "ore"},
		"iron_ingot":  {ID: "iron_ingot", Name: "Iron Ingot", Type: "component", Category: "component"},
		"steel_plate": {ID: "steel_plate", Name: "Steel Plate", Type: "component", Category: "component"},
		"copper":      {ID: "copper", Name: "Copper", Type: "material", Category: "material"},
	}

	// Create test ships
	ships := map[string]*Ship{
		"test_ship": {
			ID:   "test_ship",
			Name: "Test Ship",
			BuildMaterials: []ShipBuildRef{
				{ItemID: "steel_plate", Quantity: 5},
			},
		},
	}

	// Create test facilities
	facilities := map[string]*Facility{
		"test_facility": {
			ID:   "test_facility",
			Name: "Test Facility",
			BuildMaterials: []FacilityMaterial{
				{ItemID: "iron_ingot", Name: "Iron Ingot", Quantity: 10},
			},
		},
	}

	calc, err := NewCalculator(db, recipes, items)
	if err != nil {
		t.Fatalf("failed to create calculator: %v", err)
	}

	// Write some initial BoM data to verify it gets cleared
	initialResult := &BoMResult{
		TargetID:      "old_item",
		TargetName:    "Old Item",
		TargetType:    "item",
		BaseMaterials: []MaterialRequirement{{ItemID: "old_material", Quantity: 1}},
	}
	if err := WriteBoM(db, initialResult); err != nil {
		t.Fatalf("failed to write initial BoM: %v", err)
	}

	// Verify initial data exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM bill_of_materials").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query initial count: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 initial BoM entry, got %d", count)
	}

	// Run CalculateAll
	err = calc.CalculateAll(items, ships, facilities)
	if err != nil {
		t.Fatalf("CalculateAll returned error: %v", err)
	}

	// Verify data was cleared and new data written
	err = db.QueryRow("SELECT COUNT(*) FROM bill_of_materials").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query final count: %v", err)
	}

	// Should have entries for:
	// - iron_ingot (1 material: iron_ore)
	// - steel_plate (1 material: iron_ore)
	// - test_ship (1 material: iron_ore)
	// - test_facility (1 material: iron_ore)
	// Total: 4 entries
	if count != 4 {
		t.Errorf("Expected 4 BoM entries after CalculateAll, got %d", count)
	}

	// Verify item BoM was written correctly
	var itemType string
	var itemMaterial string
	var itemQty int
	err = db.QueryRow(`
		SELECT target_type, base_item_id, quantity
		FROM bill_of_materials
		WHERE target_id = 'steel_plate'
	`).Scan(&itemType, &itemMaterial, &itemQty)
	if err != nil {
		t.Fatalf("failed to query item BoM: %v", err)
	}

	if itemType != "item" {
		t.Errorf("Expected target_type 'item', got '%s'", itemType)
	}
	if itemMaterial != "iron_ore" {
		t.Errorf("Expected base_item_id 'iron_ore', got '%s'", itemMaterial)
	}
	// steel_plate = 2 iron_ingot = 10 iron_ore
	if itemQty != 10 {
		t.Errorf("Expected quantity 10 for steel_plate, got %d", itemQty)
	}

	// Verify ship BoM was written correctly
	var shipType string
	var shipMaterial string
	var shipQty int
	err = db.QueryRow(`
		SELECT target_type, base_item_id, quantity
		FROM bill_of_materials
		WHERE target_id = 'test_ship'
	`).Scan(&shipType, &shipMaterial, &shipQty)
	if err != nil {
		t.Fatalf("failed to query ship BoM: %v", err)
	}

	if shipType != "ship" {
		t.Errorf("Expected target_type 'ship', got '%s'", shipType)
	}
	if shipMaterial != "iron_ore" {
		t.Errorf("Expected base_item_id 'iron_ore', got '%s'", shipMaterial)
	}
	// test_ship = 5 steel_plate = 10 iron_ingot = 50 iron_ore
	if shipQty != 50 {
		t.Errorf("Expected quantity 50 for test_ship, got %d", shipQty)
	}

	// Verify facility BoM was written correctly
	var facType string
	var facMaterial string
	var facQty int
	err = db.QueryRow(`
		SELECT target_type, base_item_id, quantity
		FROM bill_of_materials
		WHERE target_id = 'test_facility'
	`).Scan(&facType, &facMaterial, &facQty)
	if err != nil {
		t.Fatalf("failed to query facility BoM: %v", err)
	}

	if facType != "facility" {
		t.Errorf("Expected target_type 'facility', got '%s'", facType)
	}
	if facMaterial != "iron_ore" {
		t.Errorf("Expected base_item_id 'iron_ore', got '%s'", facMaterial)
	}
	// test_facility = 10 iron_ingot = 50 iron_ore
	if facQty != 50 {
		t.Errorf("Expected quantity 50 for test_facility, got %d", facQty)
	}
}
