# Bill of Materials (BoM) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add recursive Bill of Materials calculation to KB generation pipeline that traces item, ship, and facility construction materials down to base components (ore and material categories) with summary tables, expandable trees, and minified JSON output.

**Architecture:** New `pkg/bom` package with calculator, resolver, and database modules integrated into existing `cmd/generate-items-kb` pipeline. Uses memoization for performance, circular dependency detection for data integrity, and optimal recipe selection (max outputs, no salvage preference).

**Tech Stack:** Go 1.24+, SQLite (existing kbdb patterns), html/template (existing templates)

---

## File Structure

**New Files:**
- `pkg/bom/calculator.go` - Core recursive algorithm with memoization
- `pkg/bom/db.go` - Database persistence layer
- `pkg/bom/recipes.go` - Recipe selection and reverse lookup
- `pkg/bom/calculator_test.go` - Unit tests for calculator
- `pkg/bom/db_test.go` - Unit tests for database layer
- `pkg/bom/recipes_test.go` - Unit tests for recipe resolver
- `cmd/generate-items-kb/bom_test.go` - Integration tests

**Modified Files:**
- `cmd/generate-items-kb/main.go` - Integration with BOM calculator, HTML templates
- `cmd/generate-items-kb/main_test.go` - Integration tests

---

## Task 1: Create pkg/bom package structure

**Files:**
- Create: `pkg/bom/calculator.go` (package declaration, empty structs)
- Create: `pkg/bom/db.go` (package declaration, empty structs)
- Create: `pkg/bom/recipes.go` (package declaration, empty structs)

- [ ] **Step 1: Write package declarations and core type definitions**

```go
package bom

import (
    "database/sql"
    "sync"
)

// MaterialRequirement represents a single material in the BoM
type MaterialRequirement struct {
    ItemID   string
    Quantity  int
}

// BoMResult contains the complete breakdown for a target item
type BoMResult struct {
    TargetID        string
    TargetName      string
    TargetType      string
    HasAlternatives bool
    BaseMaterials    []MaterialRequirement
    RecipePath      []string
}

// Calculator holds state for BoM computation
type Calculator struct {
    db              *sql.DB
    recipes         map[string]*Recipe
    itemToRecipes   map[string][]*Recipe
    items           map[string]*Item
    memo            map[string][]MaterialRequirement
    mu              sync.RWMutex
}
```

- [ ] **Step 2: Run verify compilation**

```bash
cd /home/robert/spacemolt/kb
go build ./pkg/bom/...
```

Expected: No compilation errors

- [ ] **Step 3: Commit**

```bash
git add pkg/bom/calculator.go pkg/bom/db.go pkg/bom/recipes.go
git commit -m "feat: create bom package structure and core types"
```

---

## Task 2: Implement recipe resolver

**Files:**
- Modify: `pkg/bom/recipes.go`

- [ ] **Step 1: Write failing test for recipe map building**

```go
package bom

import "testing"

func TestBuildRecipeMaps(t *testing.T) {
    recipes := map[string]*Recipe{
        "recipe_1": {
            ID:      "recipe_1",
            Outputs: []RecipeItem{{ItemID: "steel_plate", Quantity: 2}},
        },
        "recipe_2": {
            ID:      "recipe_2",
            Outputs: []RecipeItem{{ItemID: "steel_plate", Quantity: 1}},
        },
    }

    itemToRecipes, _ := BuildRecipeMaps(recipes)
    
    // Verify steel_plate has both recipes
    recipes, ok := itemToRecipes["steel_plate"]
    if !ok || len(recipes) != 2 {
        t.Errorf("expected 2 recipes for steel_plate, got %d", len(recipes))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/robert/spacemolt/kb
go test ./pkg/bom/recipes_test.go -run TestBuildRecipeMaps -v
```

Expected: FAIL with "undefined: BuildRecipeMaps"

- [ ] **Step 3: Write BuildRecipeMaps implementation**

```go
// BuildRecipeMaps creates reverse lookup maps from recipes
func BuildRecipeMaps(recipes map[string]*Recipe) (map[string][]*Recipe, error) {
    itemToRecipes := make(map[string][]*Recipe)
    
    for _, recipe := range recipes {
        for _, output := range recipe.Outputs {
            itemToRecipes[output.ItemID] = append(itemToRecipes[output.ItemID], recipe)
        }
    }
    
    return itemToRecipes, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/bom/recipes_test.go -run TestBuildRecipeMaps -v
```

Expected: PASS

- [ ] **Step 5: Write failing test for recipe selection**

```go
func TestSelectRecipe(t *testing.T) {
    recipes := map[string]*Recipe{
        "recipe_1": {
            ID:      "recipe_1",
            Outputs: []RecipeItem{{ItemID: "steel_plate", Quantity: 2}}, // Most outputs
        },
        "recipe_2": {
            ID:      "recipe_2",
            Outputs: []RecipeItem{{ItemID: "steel_plate", Quantity: 1}},
        },
    }
    
    itemToRecipes, _ := BuildRecipeMaps(recipes)
    
    selected := SelectRecipe(itemToRecipes, "steel_plate")
    if selected == nil || selected.ID != "recipe_1" {
        t.Errorf("expected recipe_1 (most outputs), got %v", selected)
    }
}
```

- [ ] **Step 6: Run test to verify it fails**

```bash
go test ./pkg/bom/recipes_test.go -run TestSelectRecipe -v
```

Expected: FAIL with "undefined: SelectRecipe"

- [ ] **Step 7: Write SelectRecipe implementation**

```go
// SelectRecipe chooses the optimal recipe for an item
func SelectRecipe(itemToRecipes map[string][]*Recipe, itemID string) *Recipe {
    recipes := itemToRecipes[itemID]
    if len(recipes) == 0 {
        return nil
    }
    
    // Filter out salvage recipes if alternatives exist
    var candidates []*Recipe
    var hasSalvage bool
    for _, recipe := range recipes {
        if UsesSalvage(recipe) {
            hasSalvage = true
        } else {
            candidates = append(candidates, recipe)
        }
    }
    
    // If all recipes use salvage, fall back to all (alphabetical)
    if len(candidates) == 0 {
        candidates = recipes
    }
    
    // Select recipe with most outputs
    var bestRecipe *Recipe
    maxOutputs := 0
    for _, recipe := range candidates {
        totalOutputs := 0
        for _, output := range recipe.Outputs {
            totalOutputs += output.Quantity
        }
        if totalOutputs > maxOutputs {
            maxOutputs = totalOutputs
            bestRecipe = recipe
        }
    }
    
    return bestRecipe
}

// UsesSalvage checks if a recipe requires salvage components
func UsesSalvage(recipe *Recipe) bool {
    salvageIDs := map[string]bool{
        "rare_salvage":   true,
        "salvage_metal":   true,
        "common_salvage": true,
        "rare_component":  true,
    }
    
    for _, input := range recipe.Inputs {
        if salvageIDs[input.ItemID] {
            return true
        }
    }
    return false
}
```

- [ ] **Step 8: Run test to verify it passes**

```bash
go test ./pkg/bom/recipes_test.go -run TestSelectRecipe -v
```

Expected: PASS

- [ ] **Step 9: Write additional tests for salvage edge cases**

```go
func TestSelectRecipe_SalvageOnly(t *testing.T) {
    // All recipes use salvage - should fall back to alphabetical
    recipes := map[string]*Recipe{
        "b_recipe": {
            ID:      "b_recipe", // Alphabetical first
            Inputs:  []RecipeItem{{ItemID: "rare_salvage", Quantity: 1}},
            Outputs: []RecipeItem{{ItemID: "item_x", Quantity: 1}},
        },
        "a_recipe": {
            ID:      "a_recipe",
            Inputs:  []RecipeItem{{ItemID: "salvage_metal", Quantity: 2}},
            Outputs: []RecipeItem{{ItemID: "item_x", Quantity: 1}},
        },
    }
    
    itemToRecipes, _ := BuildRecipeMaps(recipes)
    selected := SelectRecipe(itemToRecipes, "item_x")
    
    if selected == nil || selected.ID != "a_recipe" {
        t.Errorf("expected a_recipe (alphabetical fallback), got %v", selected)
    }
}
```

- [ ] **Step 10: Run tests to verify they pass**

```bash
go test ./pkg/bom/recipes_test.go -v
```

Expected: PASS all tests

- [ ] **Step 11: Commit**

```bash
git add pkg/bom/recipes.go pkg/bom/recipes_test.go
git commit -m "feat: implement recipe resolver with optimal selection and salvage filtering"
```

---

## Task 3: Implement database layer

**Files:**
- Modify: `pkg/bom/db.go`

- [ ] **Step 1: Write failing test for database migration**

```go
package bom

import (
    "database/sql"
    "testing"
    _ "modernc.org/sqlite"
)

func TestMigrate(t *testing.T) {
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    defer db.Close()
    
    err = Migrate(db)
    if err != nil {
        t.Fatalf("migrate: %v", err)
    }
    
    // Verify table exists
    var tableName string
    err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='bill_of_materials'").Scan(&tableName)
    if err != nil || tableName != "bill_of_materials" {
        t.Errorf("table not created: %v", err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/bom/db_test.go -run TestMigrate -v
```

Expected: FAIL with "undefined: Migrate"

- [ ] **Step 3: Write Migrate implementation**

```go
//go:embed sql/schema.sql
var schemaSQL string

// Schema returns the BoM table schema
func Schema() string {
    return schemaSQL
}

// Migrate creates or updates the bill_of_materials table
func Migrate(db *sql.DB) error {
    _, err := db.Exec(schemaSQL)
    return err
}
```

- [ ] **Step 4: Create embedded schema file**

```bash
mkdir -p pkg/bom/sql
cat > pkg/bom/sql/schema.sql << 'EOF'
CREATE TABLE IF NOT EXISTS bill_of_materials (
    target_id      TEXT NOT NULL,
    target_type    TEXT NOT NULL,
    base_item_id   TEXT NOT NULL,
    quantity       INTEGER NOT NULL,
    recipe_path    TEXT,
    has_alternatives BOOLEAN DEFAULT 0,
    PRIMARY KEY (target_id, target_type, base_item_id)
);
EOF
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./pkg/bom/db_test.go -run TestMigrate -v
```

Expected: PASS

- [ ] **Step 6: Write failing test for ClearBoM**

```go
func TestClearBoM(t *testing.T) {
    db, _ := sql.Open("sqlite", ":memory:")
    defer db.Close()
    
    Migrate(db)
    
    // Insert test data
    db.Exec(`INSERT INTO bill_of_materials (target_id, target_type, base_item_id, quantity) VALUES (?, ?, ?, ?)`,
        "test_item", "item", "iron_ore", 100)
    
    // Clear
    ClearBoM(db)
    
    // Verify cleared
    var count int
    db.QueryRow("SELECT COUNT(*) FROM bill_of_materials").Scan(&count)
    if count != 0 {
        t.Errorf("expected 0 rows after clear, got %d", count)
    }
}
```

- [ ] **Step 7: Run test to verify it fails**

```bash
go test ./pkg/bom/db_test.go -run TestClearBoM -v
```

Expected: FAIL with "undefined: ClearBoM"

- [ ] **Step 8: Write ClearBoM implementation**

```go
// ClearBoM removes all existing BoM data
func ClearBoM(db *sql.DB) error {
    _, err := db.Exec(`DELETE FROM bill_of_materials`)
    return err
}
```

- [ ] **Step 9: Run test to verify it passes**

```bash
go test ./pkg/bom/db_test.go -run TestClearBoM -v
```

Expected: PASS

- [ ] **Step 10: Write failing test for WriteBoM**

```go
func TestWriteBoM(t *testing.T) {
    db, _ := sql.Open("sqlite", ":memory:")
    defer db.Close()
    
    Migrate(db)
    
    result := &BoMResult{
        TargetID:     "steel_plate",
        TargetName:   "Steel Plate",
        TargetType:   "item",
        BaseMaterials: []MaterialRequirement{
            {ItemID: "iron_ore", Quantity: 5},
        },
        RecipePath:   []string{"refine_steel"},
    }
    
    err := WriteBoM(db, result)
    if err != nil {
        t.Fatalf("write bom: %v", err)
    }
    
    // Verify written
    var baseItemID string
    var quantity int
    err = db.QueryRow(`SELECT base_item_id, quantity FROM bill_of_materials WHERE target_id=? AND target_type=?`,
        "steel_plate", "item").Scan(&baseItemID, &quantity)
    if err != nil || baseItemID != "iron_ore" || quantity != 5 {
        t.Errorf("data not written correctly: got %s, %d", baseItemID, quantity)
    }
}
```

- [ ] **Step 11: Run test to verify it fails**

```bash
go test ./pkg/bom/db_test.go -run TestWriteBoM -v
```

Expected: FAIL with "undefined: WriteBoM"

- [ ] **Step 12: Write WriteBoM implementation**

```go
import (
    "encoding/json"
    "log"
)

// WriteBoM stores a BoM result in the database
func WriteBoM(db *sql.DB, result *BoMResult) error {
    recipePathJSON, err := json.Marshal(result.RecipePath)
    if err != nil {
        return err
    }
    
    tx, err := db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()
    
    stmt, err := tx.Prepare(`
        INSERT INTO bill_of_materials (target_id, target_type, base_item_id, quantity, recipe_path, has_alternatives)
        VALUES (?, ?, ?, ?, ?, ?)
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()
    
    for _, mat := range result.BaseMaterials {
        _, err := stmt.Exec(result.TargetID, result.TargetType, mat.ItemID, mat.Quantity, string(recipePathJSON), result.HasAlternatives)
        if err != nil {
            log.Printf("warning: failed to write BoM entry for %s: %v", mat.ItemID, err)
        }
    }
    
    return tx.Commit()
}
```

- [ ] **Step 13: Run test to verify it passes**

```bash
go test ./pkg/bom/db_test.go -run TestWriteBoM -v
```

Expected: PASS

- [ ] **Step 14: Run all database tests**

```bash
go test ./pkg/bom/db_test.go -v
```

Expected: PASS all tests

- [ ] **Step 15: Commit**

```bash
git add pkg/bom/db.go pkg/bom/sql/ pkg/bom/db_test.go
git commit -m "feat: implement database layer for bill of materials"
```

---

## Task 4: Implement core calculator

**Files:**
- Modify: `pkg/bom/calculator.go`

- [ ] **Step 1: Write failing test for base material detection**

```go
func TestCalculate_BaseMaterial(t *testing.T) {
    db, _ := sql.Open("sqlite", ":memory:")
    defer db.Close()
    
    items := map[string]*Item{
        "iron_ore": {ID: "iron_ore", Name: "Iron Ore", Category: "ore"},
    }
    
    calc := &Calculator{
        db:    db,
        items:  items,
        recipes: make(map[string]*Recipe),
        memo:   make(map[string][]MaterialRequirement),
    }
    
    materials, err := calc.Calculate("iron_ore", 10)
    if err != nil {
        t.Fatalf("calculate: %v", err)
    }
    
    if len(materials) != 1 || materials[0].ItemID != "iron_ore" || materials[0].Quantity != 10 {
        t.Errorf("expected iron_ore x10, got %v", materials)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_BaseMaterial -v
```

Expected: FAIL with "undefined: Calculator.Calculate"

- [ ] **Step 3: Write NewCalculator and base Calculate implementation**

```go
import (
    "log"
)

// NewCalculator creates a new calculator with database and data
func NewCalculator(db *sql.DB, recipes map[string]*Recipe, items map[string]*Item) (*Calculator, error) {
    itemToRecipes, err := BuildRecipeMaps(recipes)
    if err != nil {
        return nil, err
    }
    
    return &Calculator{
        db:            db,
        recipes:       recipes,
        itemToRecipes: itemToRecipes,
        items:         items,
        memo:          make(map[string][]MaterialRequirement),
        visited:       make(map[string]struct{}),
    }, nil
}

// Calculate recursively computes base materials for an item
func (c *Calculator) Calculate(itemID string, quantity int) ([]MaterialRequirement, error) {
    c.mu.RLock()
    item, hasItem := c.items[itemID]
    c.mu.RUnlock()
    
    if !hasItem {
        // Unknown item - treat as base material
        return []MaterialRequirement{{ItemID: itemID, Quantity: quantity}}, nil
    }
    
    // Check if base material (ore or material category)
    if item.Category == "ore" || item.Category == "material" {
        return []MaterialRequirement{{ItemID: itemID, Quantity: quantity}}, nil
    }
    
    // Check memo cache
    c.mu.RLock()
    if cached, ok := c.memo[itemID]; ok {
        c.mu.RUnlock()
        // Scale cached result by requested quantity
        scaled := make([]MaterialRequirement, len(cached))
        for i, mat := range cached {
            scaled[i] = MaterialRequirement{
                ItemID:  mat.ItemID,
                Quantity: mat.Quantity * quantity,
            }
        }
        return scaled, nil
    }
    c.mu.RUnlock()
    
    // Check for circular dependency
    c.mu.Lock()
    _, inPath := c.visited[itemID]
    if inPath {
        c.mu.Unlock()
        // Build cycle message for error
        var cycle []string
        for id := range c.visited {
            cycle = append(cycle, id)
        }
        cycle = append(cycle, itemID)
        return nil, fmt.Errorf("circular dependency detected in BoM calculation: %v", cycle)
    }
    c.visited[itemID] = struct{}{}
    c.mu.Unlock()
    
    // Select recipe
    recipe := SelectRecipe(c.itemToRecipes, itemID)
    if recipe == nil {
        // Not craftable - treat as base material
        return []MaterialRequirement{{ItemID: itemID, Quantity: quantity}}, nil
    }
    
    // Calculate output quantity for scaling
    var outputQty int
    for _, output := range recipe.Outputs {
        if output.ItemID == itemID {
            outputQty = output.Quantity
            break
        }
    }
    
    if outputQty == 0 {
        log.Printf("warning: recipe %s has zero output for item %s", recipe.ID, itemID)
        return []MaterialRequirement{{ItemID: itemID, Quantity: quantity}}, nil
    }
    
    // Calculate multiplier
    multiplier := float64(quantity) / float64(outputQty)
    
    // Recursively calculate inputs
    var allMaterials []MaterialRequirement
    for _, input := range recipe.Inputs {
        inputQty := int(float64(input.Quantity) * multiplier)
        
        subMaterials, err := c.Calculate(input.ItemID, inputQty)
        if err != nil {
            return nil, err
        }
        
        allMaterials = append(allMaterials, subMaterials...)
    }
    
    // Remove from visited set
    c.mu.Lock()
    delete(c.visited, itemID)
    c.mu.Unlock()
    
    // Aggregate materials
    aggregated := c.aggregateMaterials(allMaterials)
    
    // Memoize result
    c.mu.Lock()
    c.memo[itemID] = aggregated
    c.mu.Unlock()
    
    return aggregated, nil
}

// aggregateMaterials combines duplicate materials by summing quantities
func (c *Calculator) aggregateMaterials(materials []MaterialRequirement) []MaterialRequirement {
    matMap := make(map[string]int)
    for _, mat := range materials {
        matMap[mat.ItemID] += mat.Quantity
    }
    
    result := make([]MaterialRequirement, 0, len(matMap))
    i := 0
    for itemID, qty := range matMap {
        result[i] = MaterialRequirement{ItemID: itemID, Quantity: qty}
        i++
    }
    
    // Sort for consistent output
    sort.Slice(result, func(a, b int) bool {
        return result[a].ItemID < result[b].ItemID
    })
    
    return result
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_BaseMaterial -v
```

Expected: PASS

- [ ] **Step 5: Write failing test for multi-tier calculation**

```go
func TestCalculate_MultiTier(t *testing.T) {
    db, _ := sql.Open("sqlite", ":memory:")
    defer db.Close()
    
    items := map[string]*Item{
        "iron_ore":   {ID: "iron_ore", Category: "ore"},
        "steel_plate": {ID: "steel_plate", Category: "refined"},
    }
    
    recipes := map[string]*Recipe{
        "refine_steel": {
            ID: "refine_steel",
            Inputs:  []RecipeItem{{ItemID: "iron_ore", Quantity: 5}},
            Outputs: []RecipeItem{{ItemID: "steel_plate", Quantity: 2}},
        },
    }
    
    itemToRecipes, _ := BuildRecipeMaps(recipes)
    
    calc := &Calculator{
        db:            db,
        recipes:       recipes,
        itemToRecipes: itemToRecipes,
        items:         items,
        memo:          make(map[string][]MaterialRequirement),
        visited:       make(map[string]struct{}),
    }
    
    materials, err := calc.Calculate("steel_plate", 4)
    if err != nil {
        t.Fatalf("calculate: %v", err)
    }
    
    // 4 steel_plate = 2 batches of 2 = 10 iron ore needed
    if len(materials) != 1 || materials[0].ItemID != "iron_ore" || materials[0].Quantity != 10 {
        t.Errorf("expected iron_ore x10, got %v", materials)
    }
}
```

- [ ] **Step 6: Run test to verify it fails**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_MultiTier -v
```

Expected: FAIL with undefined error (need to add import for sort or fix)

- [ ] **Step 7: Add missing sort import**

```go
import (
    "database/sql"
    "log"
    "sort"
    "sync"
)
```

- [ ] **Step 8: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_MultiTier -v
```

Expected: PASS

- [ ] **Step 9: Write failing test for circular dependency detection**

```go
func TestCalculate_CircularDependency(t *testing.T) {
    db, _ := sql.Open("sqlite", ":memory:")
    defer db.Close()
    
    items := map[string]*Item{
        "item_a": {ID: "item_a", Category: "component"},
        "item_b": {ID: "item_b", Category: "component"},
        "item_c": {ID: "item_c", Category: "component"},
    }
    
    recipes := map[string]*Recipe{
        "make_a": {
            ID: "make_a",
            Inputs:  []RecipeItem{{ItemID: "item_b", Quantity: 1}},
            Outputs: []RecipeItem{{ItemID: "item_a", Quantity: 1}},
        },
        "make_b": {
            ID: "make_b",
            Inputs:  []RecipeItem{{ItemID: "item_c", Quantity: 1}},
            Outputs: []RecipeItem{{ItemID: "item_b", Quantity: 1}},
        },
        "make_c": {
            ID: "make_c",
            Inputs:  []RecipeItem{{ItemID: "item_a", Quantity: 1}},
            Outputs: []RecipeItem{{ItemID: "item_c", Quantity: 1}},
        },
    }
    
    itemToRecipes, _ := BuildRecipeMaps(recipes)
    
    calc := &Calculator{
        db:            db,
        recipes:       recipes,
        itemToRecipes: itemToRecipes,
        items:         items,
        memo:          make(map[string][]MaterialRequirement),
        visited:       make(map[string]struct{}),
    }
    
    _, err := calc.Calculate("item_a", 1)
    if err == nil {
        t.Error("expected circular dependency error, got nil")
    }
    
    if !strings.Contains(err.Error(), "circular dependency") {
        t.Errorf("expected circular dependency error, got: %v", err)
    }
}
```

- [ ] **Step 10: Run test to verify it fails**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_CircularDependency -v
```

Expected: FAIL with "undefined: strings"

- [ ] **Step 11: Add strings import**

```go
import (
    "database/sql"
    "fmt"
    "log"
    "sort"
    "strings"
    "sync"
)
```

- [ ] **Step 12: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_CircularDependency -v
```

Expected: PASS

- [ ] **Step 13: Run all calculator unit tests**

```bash
go test ./pkg/bom/calculator_test.go -v
```

Expected: PASS all tests

- [ ] **Step 14: Commit**

```bash
git add pkg/bom/calculator.go pkg/bom/calculator_test.go
git commit -m "feat: implement core calculator with memoization and circular dependency detection"
```

---

## Task 5: Implement batch calculation

**Files:**
- Modify: `pkg/bom/calculator.go`

- [ ] **Step 1: Write failing test for CalculateAll**

```go
func TestCalculateAll(t *testing.T) {
    db, _ := sql.Open("sqlite", ":memory:")
    defer db.Close()
    
    Migrate(db)
    
    items := map[string]*Item{
        "steel_plate": {ID: "steel_plate", Name: "Steel Plate", Category: "refined"},
        "iron_ore":   {ID: "iron_ore", Category: "ore"},
    }
    
    recipes := map[string]*Recipe{
        "refine_steel": {
            ID: "refine_steel",
            Inputs:  []RecipeItem{{ItemID: "iron_ore", Quantity: 5}},
            Outputs: []RecipeItem{{ItemID: "steel_plate", Quantity: 2}},
        },
    }
    
    calc, _ := NewCalculator(db, recipes, items)
    
    err := calc.CalculateAll(items, nil, nil)
    if err != nil {
        t.Fatalf("calculate all: %v", err)
    }
    
    // Verify database contains entries
    var count int
    db.QueryRow("SELECT COUNT(*) FROM bill_of_materials").Scan(&count)
    if count == 0 {
        t.Error("expected BoM data in database, got 0")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculateAll -v
```

Expected: FAIL with "undefined: Calculator.CalculateAll"

- [ ] **Step 3: Write CalculateAll implementation**

```go
import (
    "encoding/json"
    "fmt"
    "log"
)

// ShipBuildRef links an item to a ship that requires it
type ShipBuildRef struct {
    ItemID   string
    Quantity  int
}

// FacilityMaterial represents a facility build material
type FacilityMaterial struct {
    ItemID   string
    Quantity  int
}

// CalculateAll computes BoM for all items, ships, and facilities
func (c *Calculator) CalculateAll(
    items map[string]*Item,
    ships map[string]*Ship,
    facilities map[string]*Facility,
) error {
    // Clear previous data
    if err := ClearBoM(c.db); err != nil {
        return fmt.Errorf("clear bom database: %w", err)
    }
    
    log.Printf("Calculating BoM for %d items...", len(items))
    
    // Calculate for items
    for itemID, item := range items {
        if item.Category == "ore" || item.Category == "material" {
            // Skip base materials
            continue
        }
        
        materials, err := c.Calculate(itemID, 1)
        if err != nil {
            return fmt.Errorf("calculate BoM for item %s: %w", itemID, err)
        }
        
        result := &BoMResult{
            TargetID:     itemID,
            TargetName:   item.Name,
            TargetType:   "item",
            BaseMaterials: materials,
            HasAlternatives: len(c.itemToRecipes[itemID]) > 1,
        }
        
        // Select recipe path
        recipe := SelectRecipe(c.itemToRecipes, itemID)
        if recipe != nil {
            result.RecipePath = []string{recipe.ID}
        }
        
        if err := WriteBoM(c.db, result); err != nil {
            return fmt.Errorf("write BoM for item %s: %w", itemID, err)
        }
    }
    
    // Calculate for ships
    if ships != nil {
        log.Printf("Calculating BoM for %d ships...", len(ships))
        
        for shipID, ship := range ships {
            var allMaterials []MaterialRequirement
            
            for _, mat := range ship.BuildMaterials {
                materials, err := c.Calculate(mat.ItemID, mat.Quantity)
                if err != nil {
                    return fmt.Errorf("calculate BoM for ship %s material %s: %w", shipID, mat.ItemID, err)
                }
                allMaterials = append(allMaterials, materials...)
            }
            
            // Aggregate and sort
            aggregated := c.aggregateMaterials(allMaterials)
            
            result := &BoMResult{
                TargetID:     shipID,
                TargetName:   ship.Name,
                TargetType:   "ship",
                BaseMaterials: aggregated,
            }
            
            if err := WriteBoM(c.db, result); err != nil {
                return fmt.Errorf("write BoM for ship %s: %w", shipID, err)
            }
        }
    }
    
    // Calculate for facilities
    if facilities != nil {
        log.Printf("Calculating BoM for %d facilities...", len(facilities))
        
        for facID, facility := range facilities {
            var allMaterials []MaterialRequirement
            
            for _, mat := range facility.BuildMaterials {
                materials, err := c.Calculate(mat.ItemID, mat.Quantity)
                if err != nil {
                    return fmt.Errorf("calculate BoM for facility %s material %s: %w", facID, mat.ItemID, err)
                }
                allMaterials = append(allMaterials, materials...)
            }
            
            // Aggregate and sort
            aggregated := c.aggregateMaterials(allMaterials)
            
            result := &BoMResult{
                TargetID:     facID,
                TargetName:   facility.Name,
                TargetType:   "facility",
                BaseMaterials: aggregated,
            }
            
            if err := WriteBoM(c.db, result); err != nil {
                return fmt.Errorf("write BoM for facility %s: %w", facID, err)
            }
        }
    }
    
    log.Printf("BoM calculation complete")
    return nil
}

// Ship is a minimal ship struct for BoM calculation
type Ship struct {
    ID            string
    Name          string
    BuildMaterials []ShipBuildRef
}

// Facility is a minimal facility struct for BoM calculation
type Facility struct {
    ID            string
    Name          string
    BuildMaterials []FacilityMaterial
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculateAll -v
```

Expected: PASS

- [ ] **Step 5: Run all calculator tests**

```bash
go test ./pkg/bom/... -v
```

Expected: PASS all tests

- [ ] **Step 6: Commit**

```bash
git add pkg/bom/calculator.go pkg/bom/calculator_test.go
git commit -m "feat: implement batch calculation for items, ships, and facilities"
```

---

## Task 6: Integrate BOM calculator into KB generator

**Files:**
- Modify: `cmd/generate-items-kb/main.go`

- [ ] **Step 1: Add BOM import and BoM fields to structs**

```go
// Add import at top
"github.com/rsned/spacemolt-kb/pkg/bom"

// Modify Item struct
type Item struct {
    ID          string
    Name        string
    Description string
    Category    string
    Rarity      string
    Size        int
    BaseValue   int
    Stackable   bool
    Tradeable   bool
    PowerBonus int
    Hazardous  bool
    Hidden     bool
    HasImage    bool
    ProducedBy  []ProducedBy
    UsedIn      []UsedIn
    UsedInShips []ShipBuildRef
    
    BoM *bom.BoMResult // NEW: Bill of Materials data
}
```

- [ ] **Step 2: Add BoM field to Ship struct**

```go
// Add to Ship struct (around line 150 or so)
type Ship struct {
    ID             string
    Name           string
    Category       string
    Class          string
    BuildMaterials []ShipBuildMaterial
    BoM *bom.BoMResult // NEW: Bill of Materials data
}
```

- [ ] **Step 3: Create minimal Facility struct for build materials**

```go
// Add after Ship struct definition
type Facility struct {
    ID            string
    Name          string
    Category       string
    BuildMaterials []FacilityMaterial
    BoM *bom.BoMResult // NEW: Bill of Materials data
}

type FacilityMaterial struct {
    ItemID   string
    Name     string
    Quantity  int
}
```

- [ ] **Step 4: Verify compilation**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go build
```

Expected: No compilation errors

- [ ] **Step 5: Load facility data and calculate BoM**

```go
// Add after loadRecipes(db) call in main()

// Load facilities
facilities, err := loadFacilities(catalogDir, recipes)
if err != nil {
    log.Printf("warning: load facilities: %v (BoM will not be calculated for facilities)", err)
} else {
    // Initialize BOM calculator
    calc, err := bom.NewCalculator(db, recipes, items)
    if err != nil {
        log.Fatalf("init BOM calculator: %v", err)
    }
    
    // Convert to bom.Ship format
    bomShips := make(map[string]*bom.Ship)
    for id, ship := range shipCatalog {
        buildMaterials := make([]bom.ShipBuildRef, len(ship.BuildMaterials))
        for i, mat := range ship.BuildMaterials {
            buildMaterials[i] = bom.ShipBuildRef{ItemID: mat.ItemID, Quantity: mat.Quantity}
        }
        bomShips[id] = &bom.Ship{
            ID:            id,
            Name:          ship.Name,
            BuildMaterials: buildMaterials,
        }
    }
    
    // Convert to bom.Facility format
    bomFacilities := make(map[string]*bom.Facility)
    for id, facility := range facilities {
        buildMaterials := make([]bom.FacilityMaterial, len(facility.BuildMaterials))
        for i, mat := range facility.BuildMaterials {
            buildMaterials[i] = bom.FacilityMaterial{ItemID: mat.ItemID, Quantity: mat.Quantity}
        }
        bomFacilities[id] = &bom.Facility{
            ID:            id,
            Name:          facility.Name,
            BuildMaterials: buildMaterials,
        }
    }
    
    // Calculate all BoM data
    if err := calc.CalculateAll(items, bomShips, bomFacilities); err != nil {
        log.Fatalf("calculate BoM: %v", err)
    }
    
    // Load BoM results back into items, ships, facilities
    loadBoMFromDB(db, items, bomShips, bomFacilities)
}
```

- [ ] **Step 6: Write loadFacilities function**

```go
func loadFacilities(catalogDir string, recipes map[string]*Recipe) (map[string]*Facility, error) {
    facilityJSONDir := filepath.Join(catalogDir, "facility_details")
    
    facilities := make(map[string]*Facility)
    
    entries, err := os.ReadDir(facilityJSONDir)
    if err != nil {
        return nil, err
    }
    
    for _, entry := range entries {
        if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
            continue
        }
        
        data, err := os.ReadFile(filepath.Join(facilityJSONDir, entry.Name()))
        if err != nil {
            log.Printf("warning: read facility file %s: %v", entry.Name(), err)
            continue
        }
        
        var fac struct {
            ID            string         `json:"type_id"`
            Name          string         `json:"name"`
            Category       string         `json:"category"`
            BuildMaterials []FacilityMaterial `json:"build_materials"`
        }
        
        if err := json.Unmarshal(data, &fac); err != nil {
            log.Printf("warning: parse facility file %s: %v", entry.Name(), err)
            continue
        }
        
        facilities[fac.ID] = &Facility{
            ID:            fac.ID,
            Name:          fac.Name,
            Category:       fac.Category,
            BuildMaterials: fac.BuildMaterials,
        }
    }
    
    return facilities, nil
}
```

- [ ] **Step 7: Write loadBoMFromDB function**

```go
func loadBoMFromDB(db *sql.DB, items map[string]*Item, bomShips map[string]*bom.Ship, bomFacilities map[string]*bom.Facility) {
    // Load BoM for items
    for itemID, item := range items {
        var baseItemID string
        var quantity int
        var recipePathJSON string
        var hasAlternatives bool
        
        row := db.QueryRow(`
            SELECT base_item_id, quantity, recipe_path, has_alternatives
            FROM bill_of_materials
            WHERE target_id=? AND target_type='item'
            LIMIT 1
        `, itemID)
        
        err := row.Scan(&baseItemID, &quantity, &recipePathJSON, &hasAlternatives)
        if err == sql.ErrNoRows {
            // No BoM data - item might not be craftable
            continue
        }
        if err != nil {
            log.Printf("warning: load BoM for item %s: %v", itemID, err)
            continue
        }
        
        var recipePath []string
        if recipePathJSON != "" {
            json.Unmarshal([]byte(recipePathJSON), &recipePath)
        }
        
        item.BoM = &bom.BoMResult{
            TargetID:     itemID,
            TargetName:   item.Name,
            TargetType:   "item",
            BaseMaterials: []bom.MaterialRequirement{{ItemID: baseItemID, Quantity: quantity}},
            RecipePath:   recipePath,
            HasAlternatives: hasAlternatives,
        }
    }
    
    // Load BoM for ships
    if bomShips != nil {
        for shipID, ship := range bomShips {
            materials, err := loadBoMMaterials(db, shipID, "ship")
            if err != nil && err != sql.ErrNoRows {
                log.Printf("warning: load BoM for ship %s: %v", shipID, err)
                continue
            }
            ship.BoM = materials
        }
    }
    
    // Load BoM for facilities
    if bomFacilities != nil {
        for facID, facility := range bomFacilities {
            materials, err := loadBoMMaterials(db, facID, "facility")
            if err != nil && err != sql.ErrNoRows {
                log.Printf("warning: load BoM for facility %s: %v", facID, err)
                continue
            }
            facility.BoM = materials
        }
    }
}

func loadBoMMaterials(db *sql.DB, targetID, targetType string) (*bom.BoMResult, error) {
    rows, err := db.Query(`
        SELECT base_item_id, quantity, recipe_path, has_alternatives
        FROM bill_of_materials
        WHERE target_id=? AND target_type=?
        ORDER BY base_item_id
    `, targetID, targetType)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var materials []bom.MaterialRequirement
    var recipePathJSON string
    var hasAlternatives bool
    first := true
    
    for rows.Next() {
        var itemID string
        var quantity int
        if err := rows.Scan(&itemID, &quantity, &recipePathJSON, &hasAlternatives); err != nil {
            return nil, err
        }
        materials = append(materials, bom.MaterialRequirement{ItemID: itemID, Quantity: quantity})
        first = false
    }
    
    if first {
        return nil, sql.ErrNoRows
    }
    
    var recipePath []string
    if recipePathJSON != "" {
        json.Unmarshal([]byte(recipePathJSON), &recipePath)
    }
    
    return &bom.BoMResult{
        TargetType:   targetType,
        BaseMaterials: materials,
        RecipePath:   recipePath,
        HasAlternatives: hasAlternatives,
    }, nil
}
```

- [ ] **Step 8: Add BoM template functions**

```go
// Add to template funcs map in writeHTMLPages, writeShipPages, etc.
funcs := htmltpl.FuncMap{
    // ... existing funcs ...
    "hasBoM": func(bom *bom.BoMResult) bool {
        return bom != nil && len(bom.BaseMaterials) > 0
    },
    "boMJSON": func(bom *bom.BoMResult) string {
        if bom == nil {
            return ""
        }
        materials := make([]map[string]any, len(bom.BaseMaterials))
        for i, mat := range bom.BaseMaterials {
            materials[i] = map[string]any{
                "item_id": mat.ItemID,
                "quantity": mat.Quantity,
            }
        }
        data, _ := json.Marshal(materials)
        return string(data)
    },
    "boMTable": func(bom *bom.BoMResult) template.HTML {
        if bom == nil || len(bom.BaseMaterials) == 0 {
            return ""
        }
        
        var sb strings.Builder
        sb.WriteString(`<div class="card" style="padding:0">`)
        sb.WriteString(`<div class="section-label">Construction</div>`)
        sb.WriteString(`<div class="bom-summary-table">`)
        sb.WriteString(`<table><thead><tr><th>Base Material</th><th>Quantity</th></tr></thead><tbody>`)
        
        for _, mat := range bom.BaseMaterials {
            item, ok := bomItems[mat.ItemID]
            if !ok {
                sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, mat.ItemID, mat.Quantity))
                continue
            }
            
            var categoryLink string
            if item.Category == "ore" {
                categoryLink = "ore"
            } else if item.Category == "material" {
                categoryLink = "material"
            } else {
                categoryLink = item.Category
            }
            
            sb.WriteString(fmt.Sprintf(`<tr><td><a href="../items/%s/%s.html">%s</a></td><td>%d</td></tr>`,
                categoryLink, mat.ItemID, item.Name, mat.Quantity))
        }
        
        sb.WriteString(`</tbody></table></div>`)
        sb.WriteString(`</div>`)
        return htmltpl.HTML(sb.String())
    },
}
```

Note: The `bomItems` map needs to be accessible in template functions. You may need to pass it as a closure variable or add it to the FuncMap properly.

- [ ] **Step 9: Add Construction section to item template**

```go
// In htmlItemTemplate, add after the existing "Used In" section (around line 2105)
{{- if (hasBoM .BoM)}}
        <div class="card" style="padding:0">
{{boMTable .BoM}}
        </div>
{{- end}}
```

- [ ] **Step 10: Add Construction section to ship template**

```go
// Find the ship detail template and add Construction section after build materials
{{- if (hasBoM .BoM)}}
        <div class="card" style="padding:0">
{{boMTable .BoM}}
        </div>
{{- end}}
```

- [ ] **Step 11: Verify compilation**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go build
```

Expected: No compilation errors

- [ ] **Step 12: Run KB generation and verify**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go run . 2>&1 | head -50
```

Expected: Logs showing BoM calculation progress, no fatal errors

- [ ] **Step 13: Commit**

```bash
git add cmd/generate-items-kb/main.go
git commit -m "feat: integrate BOM calculator into KB generator"
```

---

## Task 7: Write integration tests

**Files:**
- Create: `cmd/generate-items-kb/bom_test.go`

- [ ] **Step 1: Write end-to-end integration test**

```go
package main

import (
    "database/sql"
    "os"
    "path/filepath"
    "testing"
    _ "modernc.org/sqlite"
)

func TestBOMIntegration(t *testing.T) {
    // Create temporary database
    tmpDir := t.TempDir()
    dbPath := filepath.Join(tmpDir, "test.db")
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    defer db.Close()
    
    // Setup test schema
    db.Exec(`
        CREATE TABLE IF NOT EXISTS items (
            id TEXT PRIMARY KEY, name TEXT, category TEXT
        );
        CREATE TABLE IF NOT EXISTS recipes (
            id TEXT PRIMARY KEY
        );
        CREATE TABLE IF NOT EXISTS recipe_inputs (
            recipe_id TEXT, item_id TEXT, quantity INTEGER
        );
        CREATE TABLE IF NOT EXISTS recipe_outputs (
            recipe_id TEXT, item_id TEXT, quantity INTEGER
        );
    `)
    
    // Insert test data
    db.Exec(`INSERT INTO items VALUES ('iron_ore', 'Iron Ore', 'ore')`)
    db.Exec(`INSERT INTO items VALUES ('steel_plate', 'Steel Plate', 'refined')`)
    db.Exec(`INSERT INTO items VALUES ('durasteel_plate', 'Durasteel Plate', 'component')`)
    db.Exec(`INSERT INTO recipes VALUES ('refine_steel', 'Refine Steel', 10)`)
    db.Exec(`INSERT INTO recipes VALUES ('forge_durasteel', 'Forge Durasteel', 15)`)
    db.Exec(`INSERT INTO recipe_inputs VALUES ('refine_steel', 'iron_ore', 5)`)
    db.Exec(`INSERT INTO recipe_outputs VALUES ('refine_steel', 'steel_plate', 2)`)
    db.Exec(`INSERT INTO recipe_inputs VALUES ('forge_durasteel', 'steel_plate', 4)`)
    db.Exec(`INSERT INTO recipe_inputs VALUES ('forge_durasteel', 'tungsten_ore', 2)`)
    db.Exec(`INSERT INTO recipe_outputs VALUES ('forge_durasteel', 'durasteel_plate', 2)`)
    
    // Run BOM calculation
    recipes, _ := loadRecipes(db)
    items, _ := loadItems(db)
    
    calc, err := bom.NewCalculator(db, recipes, items)
    if err != nil {
        t.Fatalf("new calculator: %v", err)
    }
    
    err = calc.CalculateAll(items, nil, nil)
    if err != nil {
        t.Fatalf("calculate all: %v", err)
    }
    
    // Verify BoM table was populated
    var count int
    db.QueryRow("SELECT COUNT(*) FROM bill_of_materials").Scan(&count)
    if count == 0 {
        t.Error("expected BoM data to be written")
    }
    
    // Verify specific calculations
    var steelPlateCount int
    db.QueryRow(`SELECT quantity FROM bill_of_materials WHERE target_id='steel_plate' AND base_item_id='iron_ore'`).Scan(&steelPlateCount)
    if steelPlateCount != 5 {
        t.Errorf("expected iron_ore x5 for steel_plate, got %d", steelPlateCount)
    }
}
```

- [ ] **Step 2: Run integration test**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go test -run TestBOMIntegration -v
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/generate-items-kb/bom_test.go
git commit -m "test: add integration tests for BOM feature"
```

---

## Task 8: Final testing and verification

**Files:**
- N/A (run existing generator)

- [ ] **Step 1: Run full KB generation with production database**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go run . 2>&1 | tee bom_generation.log
```

- [ ] **Step 2: Verify BoM data in database**

```bash
cd /home/robert/spacemolt/kb
sqlite3 ../spacemolt-knowledge.db "SELECT COUNT(*) FROM bill_of_materials"
```

Expected: Non-zero count showing BoM data was calculated

- [ ] **Step 3: Check sample HTML output**

```bash
ls -la kb/items/component/durasteel_plate.html
head -100 kb/items/component/durasteel_plate.html | grep -A 20 "Construction"
```

Expected: Construction section visible with base materials table

- [ ] **Step 4: Verify JSON output format**

```bash
grep -A 5 "bom-json" kb/items/component/durasteel_plate.html
```

Expected: Minified JSON array like `[{"item_id":"iron_ore","quantity":10},...]`

- [ ] **Step 5: Run full test suite**

```bash
cd /home/robert/spacemolt/kb
go test ./...
```

Expected: All tests pass

- [ ] **Step 6: Commit final changes**

```bash
git add .
git commit -m "feat: complete Bill of Materials feature implementation"
```

---

## Summary

This implementation plan creates a complete Bill of Materials calculation system integrated into the KB generation pipeline with:

1. **Core calculator** (`pkg/bom/calculator.go`) - Recursive algorithm with memoization and circular dependency detection
2. **Recipe resolver** (`pkg/bom/recipes.go`) - Optimal recipe selection with salvage filtering
3. **Database layer** (`pkg/bom/db.go`) - BoM data persistence
4. **Integration** (`cmd/generate-items-kb/main.go`) - BoM calculation during KB generation
5. **HTML templates** - Construction sections with summary tables and JSON output
6. **Comprehensive tests** - Unit tests for all components + integration tests

The system calculates BoM for items, ships, and facilities by tracing recipes through multiple crafting tiers down to base materials (ore and material categories), with performance optimization through memoization and data integrity through circular dependency detection.
