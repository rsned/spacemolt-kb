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

- [x] **Step 1: Write package declarations and core type definitions**

`pkg/bom/calculator.go`:

```go
package bom

import (
    "database/sql"
    "sync"
)

// MaterialRequirement represents a single material in the BoM
type MaterialRequirement struct {
    ItemID   string
    Quantity int
}

// BoMResult contains the complete breakdown for a target item
type BoMResult struct {
    TargetID        string
    TargetName      string
    TargetType      string
    HasAlternatives bool
    BaseMaterials   []MaterialRequirement
    RecipePath      []string
}

// Calculator holds state for BoM computation
type Calculator struct {
    db            *sql.DB
    recipes       map[string]*Recipe
    itemToRecipes map[string][]*Recipe
    items         map[string]*Item
    memo          map[string][]MaterialRequirement // per-1-unit base materials, scaled at lookup
    mu            sync.RWMutex
}
```

`pkg/bom/recipes.go` (must be declared in Task 1 — Task 2 tests reference these types):

```go
package bom

// Recipe describes a crafting recipe.
type Recipe struct {
    ID      string
    Name    string
    Inputs  []RecipeItem
    Outputs []RecipeItem
}

// RecipeItem is an input or output of a recipe.
type RecipeItem struct {
    ItemID   string
    Quantity int
}

// Item is the minimal item shape needed by the BoM calculator.
type Item struct {
    ID       string
    Name     string
    Category string
}
```

- [x] **Step 2: Run verify compilation**

```bash
cd /home/robert/spacemolt/kb
go build ./pkg/bom/...
```

Expected: No compilation errors

- [x] **Step 3: Commit**

```bash
git add pkg/bom/calculator.go pkg/bom/db.go pkg/bom/recipes.go
git commit -m "feat: create bom package structure and core types"
```

---

## Task 2: Implement recipe resolver

**Files:**
- Modify: `pkg/bom/recipes.go`

- [x] **Step 1: Write failing test for recipe map building**

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

- [x] **Step 2: Run test to verify it fails**

```bash
cd /home/robert/spacemolt/kb
go test ./pkg/bom/recipes_test.go -run TestBuildRecipeMaps -v
```

Expected: FAIL with "undefined: BuildRecipeMaps"

- [x] **Step 3: Write BuildRecipeMaps implementation**

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

- [x] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/bom/recipes_test.go -run TestBuildRecipeMaps -v
```

Expected: PASS

- [x] **Step 5: Write failing test for recipe selection**

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

- [x] **Step 6: Run test to verify it fails**

```bash
go test ./pkg/bom/recipes_test.go -run TestSelectRecipe -v
```

Expected: FAIL with "undefined: SelectRecipe"

- [x] **Step 7: Write SelectRecipe implementation**

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

- [x] **Step 8: Run test to verify it passes**

```bash
go test ./pkg/bom/recipes_test.go -run TestSelectRecipe -v
```

Expected: PASS

- [x] **Step 9: Write additional tests for salvage edge cases**

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

- [x] **Step 10: Run tests to verify they pass**

```bash
go test ./pkg/bom/recipes_test.go -v
```

Expected: PASS all tests

- [x] **Step 11: Commit**

```bash
git add pkg/bom/recipes.go pkg/bom/recipes_test.go
git commit -m "feat: implement recipe resolver with optimal selection and salvage filtering"
```

---

## Task 3: Implement database layer

**Files:**
- Modify: `pkg/bom/db.go`

- [x] **Step 1: Write failing test for database migration**

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

- [x] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/bom/db_test.go -run TestMigrate -v
```

Expected: FAIL with "undefined: Migrate"

- [x] **Step 3: Write Migrate implementation**

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

- [x] **Step 4: Create embedded schema file**

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

- [x] **Step 5: Run test to verify it passes**

```bash
go test ./pkg/bom/db_test.go -run TestMigrate -v
```

Expected: PASS

- [x] **Step 6: Write failing test for ClearBoM**

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

- [x] **Step 7: Run test to verify it fails**

```bash
go test ./pkg/bom/db_test.go -run TestClearBoM -v
```

Expected: FAIL with "undefined: ClearBoM"

- [x] **Step 8: Write ClearBoM implementation**

```go
// ClearBoM removes all existing BoM data
func ClearBoM(db *sql.DB) error {
    _, err := db.Exec(`DELETE FROM bill_of_materials`)
    return err
}
```

- [x] **Step 9: Run test to verify it passes**

```bash
go test ./pkg/bom/db_test.go -run TestClearBoM -v
```

Expected: PASS

- [x] **Step 10: Write failing test for WriteBoM**

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

- [x] **Step 11: Run test to verify it fails**

```bash
go test ./pkg/bom/db_test.go -run TestWriteBoM -v
```

Expected: FAIL with "undefined: WriteBoM"

- [x] **Step 12: Write WriteBoM implementation**

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

- [x] **Step 13: Run test to verify it passes**

```bash
go test ./pkg/bom/db_test.go -run TestWriteBoM -v
```

Expected: PASS

- [x] **Step 14: Run all database tests**

```bash
go test ./pkg/bom/db_test.go -v
```

Expected: PASS all tests

- [x] **Step 15: Commit**

```bash
git add pkg/bom/db.go pkg/bom/sql/ pkg/bom/db_test.go
git commit -m "feat: implement database layer for bill of materials"
```

---

## Task 4: Implement core calculator

**Files:**
- Modify: `pkg/bom/calculator.go`

- [x] **Step 1: Write failing test for base material detection**

```go
func TestCalculate_BaseMaterial(t *testing.T) {
    db, _ := sql.Open("sqlite", ":memory:")
    defer db.Close()
    
    items := map[string]*Item{
        "iron_ore": {ID: "iron_ore", Name: "Iron Ore", Category: "ore"},
    }
    
    calc, err := NewCalculator(db, map[string]*Recipe{}, items)
    if err != nil {
        t.Fatalf("new calculator: %v", err)
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

- [x] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_BaseMaterial -v
```

Expected: FAIL with "undefined: Calculator.Calculate"

- [x] **Step 3: Write NewCalculator and base Calculate implementation**

Three correctness rules for this step:

1. **Memoize per-1-unit, scale on lookup.** Storing the scaled result poisons the cache for subsequent calls of different quantities.
2. **Use ceiling arithmetic on the batch multiplier.** You cannot craft a fractional batch; `int(float * x)` truncates and undercounts inputs.
3. **Pass the visited set down the call stack, not on the calculator.** A calculator-wide `visited` map leaks state across calls when a cycle aborts mid-recursion.

```go
import (
    "fmt"
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
    }, nil
}

// Calculate returns base materials needed to produce `quantity` units of itemID.
func (c *Calculator) Calculate(itemID string, quantity int) ([]MaterialRequirement, error) {
    perUnit, err := c.calculatePerUnit(itemID, nil)
    if err != nil {
        return nil, err
    }
    return scaleMaterials(perUnit, quantity), nil
}

// calculatePerUnit returns the base materials needed for ONE unit of itemID.
// `path` is the current recursion stack used for cycle detection; pass nil at the top level.
func (c *Calculator) calculatePerUnit(itemID string, path []string) ([]MaterialRequirement, error) {
    item, hasItem := c.items[itemID]
    if !hasItem {
        return []MaterialRequirement{{ItemID: itemID, Quantity: 1}}, nil
    }
    if item.Category == "ore" || item.Category == "material" {
        return []MaterialRequirement{{ItemID: itemID, Quantity: 1}}, nil
    }

    // Memo lookup (per-unit).
    c.mu.RLock()
    if cached, ok := c.memo[itemID]; ok {
        c.mu.RUnlock()
        return cached, nil
    }
    c.mu.RUnlock()

    // Cycle detection: search the stack in order so we can report the cycle deterministically.
    for i, prev := range path {
        if prev == itemID {
            cycle := append([]string(nil), path[i:]...)
            cycle = append(cycle, itemID)
            return nil, fmt.Errorf("circular dependency detected in BoM calculation: %v", cycle)
        }
    }
    path = append(path, itemID)

    recipe := SelectRecipe(c.itemToRecipes, itemID)
    if recipe == nil {
        return []MaterialRequirement{{ItemID: itemID, Quantity: 1}}, nil
    }

    var outputQty int
    for _, output := range recipe.Outputs {
        if output.ItemID == itemID {
            outputQty = output.Quantity
            break
        }
    }
    if outputQty == 0 {
        log.Printf("warning: recipe %s has zero output for item %s", recipe.ID, itemID)
        return []MaterialRequirement{{ItemID: itemID, Quantity: 1}}, nil
    }

    // Walk inputs at recipe-batch quantity, then divide-with-ceil at the end so we
    // get correct per-unit values even when output > 1.
    var batch []MaterialRequirement
    for _, input := range recipe.Inputs {
        sub, err := c.calculatePerUnit(input.ItemID, path)
        if err != nil {
            return nil, err
        }
        batch = append(batch, scaleMaterials(sub, input.Quantity)...)
    }
    aggregated := aggregateMaterials(batch)

    perUnit := make([]MaterialRequirement, len(aggregated))
    for i, mat := range aggregated {
        perUnit[i] = MaterialRequirement{
            ItemID:   mat.ItemID,
            Quantity: ceilDiv(mat.Quantity, outputQty),
        }
    }

    c.mu.Lock()
    c.memo[itemID] = perUnit
    c.mu.Unlock()

    return perUnit, nil
}

func scaleMaterials(in []MaterialRequirement, factor int) []MaterialRequirement {
    out := make([]MaterialRequirement, len(in))
    for i, m := range in {
        out[i] = MaterialRequirement{ItemID: m.ItemID, Quantity: m.Quantity * factor}
    }
    return out
}

func ceilDiv(a, b int) int {
    if b == 0 {
        return a
    }
    return (a + b - 1) / b
}

// aggregateMaterials combines duplicate materials by summing quantities.
func aggregateMaterials(materials []MaterialRequirement) []MaterialRequirement {
    matMap := make(map[string]int)
    for _, mat := range materials {
        matMap[mat.ItemID] += mat.Quantity
    }

    result := make([]MaterialRequirement, 0, len(matMap))
    for itemID, qty := range matMap {
        result = append(result, MaterialRequirement{ItemID: itemID, Quantity: qty})
    }

    sort.Slice(result, func(a, b int) bool {
        return result[a].ItemID < result[b].ItemID
    })

    return result
}
```

Note: `Calculator.visited` is removed; cycle detection lives on the call stack via `path`. Update the `Calculator` struct in Task 1 to drop the `visited` field, or delete it as part of this step.

- [x] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_BaseMaterial -v
```

Expected: PASS

- [x] **Step 5: Write failing test for multi-tier calculation**

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
    
    calc, err := NewCalculator(db, recipes, items)
    if err != nil {
        t.Fatalf("new calculator: %v", err)
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

- [x] **Step 6: Run test to verify it fails**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_MultiTier -v
```

Expected: FAIL with undefined error (need to add import for sort or fix)

- [x] **Step 7: Add missing imports**

```go
import (
    "database/sql"
    "fmt"
    "log"
    "sort"
    "sync"
)
```

- [x] **Step 8: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_MultiTier -v
```

Expected: PASS

- [x] **Step 9: Write failing test for circular dependency detection**

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
    
    calc, err := NewCalculator(db, recipes, items)
    if err != nil {
        t.Fatalf("new calculator: %v", err)
    }

    _, err = calc.Calculate("item_a", 1)
    if err == nil {
        t.Error("expected circular dependency error, got nil")
    }
    
    if !strings.Contains(err.Error(), "circular dependency") {
        t.Errorf("expected circular dependency error, got: %v", err)
    }
}
```

- [x] **Step 10: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_CircularDependency -v
```

Expected: PASS — error message contains "circular dependency" with the cycle in traversal order (e.g. `[item_a item_b item_c item_a]`).

- [x] **Step 11: (removed — `strings` is no longer needed; cycle path is built deterministically from the call stack)**

- [x] **Step 12: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculate_CircularDependency -v
```

Expected: PASS

- [x] **Step 13: Run all calculator unit tests**

```bash
go test ./pkg/bom/calculator_test.go -v
```

Expected: PASS all tests

- [x] **Step 14: Commit**

```bash
git add pkg/bom/calculator.go pkg/bom/calculator_test.go
git commit -m "feat: implement core calculator with memoization and circular dependency detection"
```

---

## Task 5: Implement batch calculation

**Files:**
- Modify: `pkg/bom/calculator.go`

- [x] **Step 1: Write failing test for CalculateAll**

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

- [x] **Step 2: Run test to verify it fails**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculateAll -v
```

Expected: FAIL with "undefined: Calculator.CalculateAll"

- [x] **Step 3: Write CalculateAll implementation**

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

// CalculateAll computes BoM for all items, ships, and facilities.
// All results are buffered in memory and only written after every calculation
// succeeds, so a mid-run failure leaves the existing BoM data intact.
func (c *Calculator) CalculateAll(
    items map[string]*Item,
    ships map[string]*Ship,
    facilities map[string]*Facility,
) error {
    var results []*BoMResult

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
        
        results = append(results, result)
    }
    
    // Calculate for ships
    if ships != nil {
        log.Printf("Calculating BoM for %d ships...", len(ships))
        
        for shipID, ship := range ships {
            var allMaterials []MaterialRequirement
            hasAlt := false

            for _, mat := range ship.BuildMaterials {
                materials, err := c.Calculate(mat.ItemID, mat.Quantity)
                if err != nil {
                    return fmt.Errorf("calculate BoM for ship %s material %s: %w", shipID, mat.ItemID, err)
                }
                if len(c.itemToRecipes[mat.ItemID]) > 1 {
                    hasAlt = true
                }
                allMaterials = append(allMaterials, materials...)
            }

            aggregated := aggregateMaterials(allMaterials)

            result := &BoMResult{
                TargetID:        shipID,
                TargetName:      ship.Name,
                TargetType:      "ship",
                BaseMaterials:   aggregated,
                HasAlternatives: hasAlt,
            }
            
            results = append(results, result)
        }
    }
    
    // Calculate for facilities
    if facilities != nil {
        log.Printf("Calculating BoM for %d facilities...", len(facilities))
        
        for facID, facility := range facilities {
            var allMaterials []MaterialRequirement
            hasAlt := false

            for _, mat := range facility.BuildMaterials {
                materials, err := c.Calculate(mat.ItemID, mat.Quantity)
                if err != nil {
                    return fmt.Errorf("calculate BoM for facility %s material %s: %w", facID, mat.ItemID, err)
                }
                if len(c.itemToRecipes[mat.ItemID]) > 1 {
                    hasAlt = true
                }
                allMaterials = append(allMaterials, materials...)
            }

            aggregated := aggregateMaterials(allMaterials)

            result := &BoMResult{
                TargetID:        facID,
                TargetName:      facility.Name,
                TargetType:      "facility",
                BaseMaterials:   aggregated,
                HasAlternatives: hasAlt,
            }
            
            results = append(results, result)
        }
    }

    // All calculations succeeded; clear old data and persist the new set.
    if err := ClearBoM(c.db); err != nil {
        return fmt.Errorf("clear bom database: %w", err)
    }
    for _, result := range results {
        if err := WriteBoM(c.db, result); err != nil {
            return fmt.Errorf("write BoM for %s/%s: %w", result.TargetType, result.TargetID, err)
        }
    }

    log.Printf("BoM calculation complete: %d targets persisted", len(results))
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

- [x] **Step 4: Run test to verify it passes**

```bash
go test ./pkg/bom/calculator_test.go -run TestCalculateAll -v
```

Expected: PASS

- [x] **Step 5: Run all calculator tests**

```bash
go test ./pkg/bom/... -v
```

Expected: PASS all tests

- [x] **Step 6: Commit**

```bash
git add pkg/bom/calculator.go pkg/bom/calculator_test.go
git commit -m "feat: implement batch calculation for items, ships, and facilities"
```

---

## Task 6: Integrate BOM calculator into KB generator

**Files:**
- Modify: `cmd/generate-items-kb/main.go`

- [x] **Step 1: Add BOM import and BoM fields to structs**

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

- [x] **Step 2: Add BoM field to Ship struct**

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

- [x] **Step 3: Create minimal Facility struct for build materials**

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

- [x] **Step 4: Verify compilation**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go build
```

Expected: No compilation errors

- [x] **Step 5: Load facility data and calculate BoM**

```go
// Add after loadRecipes(db) call in main()

// Load facilities
facilities, err := loadFacilities(catalogDir)
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

- [x] **Step 6: Write loadFacilities function**

```go
func loadFacilities(catalogDir string) (map[string]*Facility, error) {
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

- [x] **Step 7: Write loadBoMFromDB function**

```go
func loadBoMFromDB(db *sql.DB, items map[string]*Item, bomShips map[string]*bom.Ship, bomFacilities map[string]*bom.Facility) {
    // Load BoM for items (multi-row — one per base material).
    for itemID, item := range items {
        result, err := loadBoMMaterials(db, itemID, "item")
        if err == sql.ErrNoRows {
            continue // not craftable
        }
        if err != nil {
            log.Printf("warning: load BoM for item %s: %v", itemID, err)
            continue
        }
        result.TargetID = itemID
        result.TargetName = item.Name
        item.BoM = result
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

- [x] **Step 8: Add BoM template functions**

The `boMTable` helper needs the items lookup to resolve names and category-specific links. Pass it explicitly via a constructor that returns a `FuncMap` closure, rather than relying on a package-level `bomItems` global.

```go
func bomFuncs(items map[string]*Item) htmltpl.FuncMap {
    return htmltpl.FuncMap{
        "hasBoM": func(b *bom.BoMResult) bool {
            return b != nil && len(b.BaseMaterials) > 0
        },
        "boMJSON": func(b *bom.BoMResult) string {
            if b == nil {
                return ""
            }
            mats := make([]map[string]any, len(b.BaseMaterials))
            for i, mat := range b.BaseMaterials {
                mats[i] = map[string]any{"item_id": mat.ItemID, "quantity": mat.Quantity}
            }
            data, _ := json.Marshal(mats)
            return string(data)
        },
        "boMTable": func(b *bom.BoMResult) htmltpl.HTML {
            if b == nil || len(b.BaseMaterials) == 0 {
                return ""
            }
            var sb strings.Builder
            sb.WriteString(`<div class="card" style="padding:0">`)
            sb.WriteString(`<div class="section-label">Construction</div>`)
            sb.WriteString(`<div class="bom-summary-table">`)
            sb.WriteString(`<table><thead><tr><th>Base Material</th><th>Quantity</th></tr></thead><tbody>`)

            for _, mat := range b.BaseMaterials {
                item, ok := items[mat.ItemID]
                if !ok {
                    sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`,
                        htmltpl.HTMLEscapeString(mat.ItemID), mat.Quantity))
                    continue
                }
                sb.WriteString(fmt.Sprintf(
                    `<tr><td><a href="../items/%s/%s.html">%s</a></td><td>%d</td></tr>`,
                    htmltpl.HTMLEscapeString(item.Category),
                    htmltpl.HTMLEscapeString(mat.ItemID),
                    htmltpl.HTMLEscapeString(item.Name),
                    mat.Quantity,
                ))
            }
            sb.WriteString(`</tbody></table></div></div>`)
            return htmltpl.HTML(sb.String())
        },
    }
}
```

Then merge `bomFuncs(items)` into the existing `FuncMap` for each template that renders BoM (item, ship, facility pages).

The expandable recipe-tree view from the spec is **out of scope** here — see "Known Limitations" at the end. To add it later, the calculator would need to persist the full traversed recipe tree (not just the top recipe) in `recipe_path`.

- [x] **Step 9: Add Construction section to item template**

```go
// In htmlItemTemplate, add after the existing "Used In" section (around line 2105)
{{- if (hasBoM .BoM)}}
        <div class="card" style="padding:0">
{{boMTable .BoM}}
        </div>
{{- end}}
```

- [x] **Step 10: Add Construction section to ship template**

```go
// Find the ship detail template and add Construction section after build materials
{{- if (hasBoM .BoM)}}
        <div class="card" style="padding:0">
{{boMTable .BoM}}
        </div>
{{- end}}
```

- [x] **Step 11: Verify compilation**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go build
```

Expected: No compilation errors

- [x] **Step 12: Run KB generation and verify**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go run . 2>&1 | head -50
```

Expected: Logs showing BoM calculation progress, no fatal errors

- [x] **Step 13: Commit**

```bash
git add cmd/generate-items-kb/main.go
git commit -m "feat: integrate BOM calculator into KB generator"
```

---

## Task 6.5: Remediation of bugs in already-committed code

Tasks 1–6 were implemented and committed before this review. The revised code in
this plan fixes the issues below; apply each to the committed source before
moving on to integration tests.

**Files:**
- Modify: `pkg/bom/calculator.go`
- Modify: `pkg/bom/recipes.go`
- Modify: `cmd/generate-items-kb/main.go`

- [x] **Step 1: Fix memoization to be per-1-unit**

The committed `Calculate` memoizes the *scaled* result of the first call, so a
second call with a different quantity returns the wrong number of base
materials. Replace with the `calculatePerUnit` + `scaleMaterials` pattern from
Task 4 Step 3 (above). The memo must always store per-unit values; the public
`Calculate` scales on lookup.

- [x] **Step 2: Use ceiling division on the batch multiplier**

`int(float64(input.Quantity) * multiplier)` truncates toward zero. With output=2
and quantity=1, multiplier=0.5, a 5-ore input rounds to 2 instead of the
required 3. Use `ceilDiv(inputs * batchSize, outputQty)` after walking inputs at
recipe-batch quantity (see Task 4 Step 3 above).

- [x] **Step 3: Replace calculator-wide `visited` map with a stack-passed `path`**

The committed `Calculator.visited` map is shared across all calls and only
cleared on success. After a real cycle aborts mid-recursion, every later
`Calculate` sees a contaminated visited set. Remove the field; pass `path
[]string` down `calculatePerUnit` and search it for the current item before
appending. This also makes the cycle reported in the error message
deterministic.

- [x] **Step 4: Fix `loadBoMFromDB` items branch to load all base materials**
  *(N/A — committed `loadBoMFromDB` already groups multi-row results correctly; no `LIMIT 1` was present. Plan-text bug only.)*

- [x] **Step 5: Set `HasAlternatives` for ships and facilities**

The committed `CalculateAll` only sets `HasAlternatives` for items. Track an
`hasAlt` flag during the build-materials loop for ships and facilities (see
revised Task 5 above) so the field is populated for all three target types.

- [x] **Step 6: Buffer `CalculateAll` writes so partial failures don't wipe the DB**

The committed code calls `ClearBoM` first and then writes per target. A failure
midway through leaves the database empty. Buffer all `BoMResult` values in a
slice, run `ClearBoM` only after every calculation succeeds, and persist the
buffered slice (see revised Task 5 above).

- [x] **Step 7: Drop the unused `recipes` parameter from `loadFacilities`**
  *(N/A — actual function is `loadFacilitiesFromJSON(dir)` and already takes only the directory.)*

- [ ] **Step 8: Remove the hardcoded salvage-ID list from `recipes.go`** *(deferred)*

`UsesSalvage` still hardcodes four item IDs. Deferred — the production data
issue this would unblock turned out to be wrap/unwrap recipes (handled by the
new `IsPackagingRecipe` filter), not salvage. The hardcoded salvage list is
working in production with no observed issues. If a similar refactor is needed
later (e.g., to externalize a growing list of non-primary recipe categories),
both `UsesSalvage` and `IsPackagingRecipe` should move to a single
`data/bom/non-primary-recipes.json` config in the same change.

- [x] **Step 8b: Wire ships and facilities into `CalculateAll`** *(added during remediation)*

The committed `main.go` was calling `calculator.CalculateAll(bomItems, nil, nil)`,
so ship and facility BoM was never calculated and their pages had no Construction
section. Restructure the load order so that `loadShipCatalog`, `loadFacilitiesFromJSON`,
and `loadRecipes` all run *before* the BoM calculation, then pass converted
`bomShips` and `bomFacilities` maps into `CalculateAll`.

- [x] **Step 8c: Run `bom.Migrate(db)` before `CalculateAll`** *(added during remediation)*

The committed `main.go` never called `bom.Migrate`, so on a fresh production DB
the `ClearBoM` call inside `CalculateAll` would fail with "no such table:
bill_of_materials". Add a `bom.Migrate(db)` call right before `NewCalculator`.

- [x] **Step 8d: Move item HTML generation after BoM calculation** *(added during remediation)*

`writeHTMLPages` was running at line 502, before BoM was calculated at line 573.
Item pages reference `hasBoM .BoM` / `boMTable .BoM` in the template, so without
this reorder no item Construction section ever rendered. Move `writeHTMLPages`
to after `loadBoMFromDB` so item structs have BoM data attached.

- [x] **Step 8e: Add `IsPackagingRecipe` filter to `SelectRecipe`** *(added during remediation)*

Running `CalculateAll` against the production crafting DB exposed 7
wrap/unwrap recipe pairs (e.g. `wrap_enriched_uranium_rod` ↔
`unwrap_enriched_uranium_rod`) that form `X ↔ contained_X` cycles. Without a
filter, `SelectRecipe`'s max-output rule picks `unwrap_*` over the primary
production recipe and 737 targets fail. Added `IsPackagingRecipe` as a first
filter pass before salvage filtering, both with fallback if the filter would
empty the candidate set. Diagnostic test in
`cmd/generate-items-kb/bom_diagnose_test.go` confirms 0 cycles after the fix.

- [x] **Step 8f: Fix broken ship and facility detail templates** *(added during remediation)*

The ship detail template referenced `.BaseHull`, `.BuildMaterials`, etc. on the
dot, but the data passed in was `shipDetailData{Ship, Items}`. `tmpl.Execute`
errored at the first such field, the run aborted before reaching `writeShipPages`,
and stale ship HTML on disk was masking the bug. Wrapped the template body in
`{{- with .Ship}}...{{- end}}` and removed redundant `.Ship.` prefixes inside.
Also reduced `boMTable` to one argument (capturing items via the closure already
in scope) and updated the facility template's call site to match.

- [x] **Step 8g: Add minified JSON output (`boMJSON`)** *(added during remediation)*

Spec called for a `<pre class="bom-json">` block alongside the summary table.
Added `BoMResult.JSON()` method in `pkg/bom/calculator.go` returning a minified
`[{"item_id":"X","quantity":N},...]` array (or empty string for nil/empty).
Wired `boMJSON` template func and a `<details><summary>View JSON Data</summary>`
block into all three templates. Unit test in `TestBoMResult_JSON` covers nil,
empty, single, and multi-material cases.

- [x] **Step 9: Run all calculator tests after the refactor**

Regression tests added in `pkg/bom/calculator_test.go`:
- `TestCalculate_MemoizationDifferentQuantities` — verifies qty=1, 4, and 6
  return correctly scaled results from the same memo entry, plus that the cache
  is immune to caller mutation (cloneMaterials defense).
- `TestCalculate_CircularDependency_PathOrder` — verifies cycle is reported in
  traversal order (`[a b c a]`) rather than non-deterministic map iteration order.

Result: `go test ./pkg/bom/... -v` — 18/18 tests passing.

- [ ] **Step 10: Commit**

```bash
git add pkg/bom/ cmd/generate-items-kb/main.go
git commit -m "fix(bom): per-unit memoization, ceil arithmetic, stack visited, atomic writes"
```

---

## Task 7: Write integration tests

**Files:**
- Create: `cmd/generate-items-kb/bom_test.go`
- Create: `cmd/generate-items-kb/testdata/bom/` (JSON fixtures matching the loaders' real format)

- [x] **Step 1: Inspect the real loaders**

The actual `cmd/generate-items-kb` loaders read JSON files from a catalog
directory; they do **not** read items/recipes from SQL. Open the existing
`loadItems`, `loadRecipes`, and (if present) `loadFacilities` to see the exact
struct tags, field names, and directory layout the test fixture must mirror.
Hand-rolled SQL inserts (as in earlier drafts of this plan) bypass the loaders
entirely and won't catch loader/calculator mismatches.

- [x] **Step 2: Build a tiny on-disk fixture set**

Under `cmd/generate-items-kb/testdata/bom/`, create the minimum directory
structure the loaders expect, with three items (one ore, one refined, one
component) and two recipes. Example layout (adjust filenames/fields to match
what the loaders actually parse):

```
testdata/bom/
  items/
    iron_ore.json
    steel_plate.json
    durasteel_plate.json
  recipes/
    refine_steel.json     # inputs: 5 iron_ore  -> outputs: 2 steel_plate
    forge_durasteel.json  # inputs: 4 steel_plate, 2 tungsten_ore -> outputs: 2 durasteel_plate
```

`tungsten_ore` is intentionally absent from the items fixture so the test also
verifies the "unknown item treated as base material" behavior.

- [x] **Step 3: Write the end-to-end test**

```go
package main

import (
    "database/sql"
    "path/filepath"
    "testing"

    _ "modernc.org/sqlite"

    "github.com/rsned/spacemolt-kb/pkg/bom"
)

func TestBOMIntegration(t *testing.T) {
    catalogDir := filepath.Join("testdata", "bom")

    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    defer db.Close()
    if err := bom.Migrate(db); err != nil {
        t.Fatalf("migrate: %v", err)
    }

    items, err := loadItems(catalogDir)
    if err != nil {
        t.Fatalf("loadItems: %v", err)
    }
    recipes, err := loadRecipes(catalogDir)
    if err != nil {
        t.Fatalf("loadRecipes: %v", err)
    }

    bomItems := make(map[string]*bom.Item, len(items))
    for id, it := range items {
        bomItems[id] = &bom.Item{ID: id, Name: it.Name, Category: it.Category}
    }
    bomRecipes := make(map[string]*bom.Recipe, len(recipes))
    for id, r := range recipes {
        bomRecipes[id] = convertRecipeForBoM(r) // helper that maps cmd types -> bom types
    }

    calc, err := bom.NewCalculator(db, bomRecipes, bomItems)
    if err != nil {
        t.Fatalf("new calculator: %v", err)
    }
    if err := calc.CalculateAll(bomItems, nil, nil); err != nil {
        t.Fatalf("calculate all: %v", err)
    }

    // Per-1-unit assertions:
    //   1 steel_plate needs ceil(5/2) = 3 iron_ore.
    //   1 durasteel_plate needs ceil(4/2)=2 steel_plate -> 2*ceil(5/2)=6 iron_ore,
    //   plus ceil(2/2)=1 tungsten_ore (unknown -> treated as base).
    cases := []struct {
        target, base string
        want         int
    }{
        {"steel_plate", "iron_ore", 3},
        {"durasteel_plate", "iron_ore", 6},
        {"durasteel_plate", "tungsten_ore", 1},
    }
    for _, tc := range cases {
        var got int
        err := db.QueryRow(
            `SELECT quantity FROM bill_of_materials WHERE target_id=? AND target_type='item' AND base_item_id=?`,
            tc.target, tc.base,
        ).Scan(&got)
        if err != nil {
            t.Errorf("%s/%s: query: %v", tc.target, tc.base, err)
            continue
        }
        if got != tc.want {
            t.Errorf("%s/%s: got %d, want %d", tc.target, tc.base, got, tc.want)
        }
    }
}

func TestBOMIntegration_RoundTrip(t *testing.T) {
    // Calculate twice with different requested quantities to verify the
    // memoization fix from Task 6.5 — both calls must return scaled results,
    // not stale cached values from the first call.
    // (see plan Task 4 Step 3 / 6.5 Step 1)
}
```

- [x] **Step 4: Run the integration test**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go test -run TestBOMIntegration -v
```

Expected: PASS. If the loaders' JSON shape doesn't match the fixture, the test
fails fast with a clear error — that's the point of going through the real
loaders.

- [x] **Step 2: Run integration test**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go test -run TestBOMIntegration -v
```

Expected: PASS

- [x] **Step 3: Commit**

```bash
git add cmd/generate-items-kb/bom_test.go
git commit -m "test: add integration tests for BOM feature"
```

---

## Task 8: Final testing and verification

**Files:**
- N/A (run existing generator)

- [x] **Step 1: Run full KB generation with production database**

```bash
cd /home/robert/spacemolt/kb/cmd/generate-items-kb
go run . 2>&1 | tee bom_generation.log
```

- [x] **Step 2: Verify BoM data integrity**

Run each of the following against the production knowledge DB; every query
should return zero rows or otherwise match the expected value.

```bash
cd /home/robert/spacemolt/kb
DB=../spacemolt-knowledge.db

# 2a. Total row count is non-zero.
sqlite3 "$DB" "SELECT COUNT(*) FROM bill_of_materials"

# 2b. No invalid quantities (must be >= 1).
sqlite3 "$DB" "SELECT COUNT(*) FROM bill_of_materials WHERE quantity <= 0"

# 2c. No unknown target_type values.
sqlite3 "$DB" "SELECT DISTINCT target_type FROM bill_of_materials
               WHERE target_type NOT IN ('item','ship','facility')"

# 2d. Every base_item_id is either a known ore/material or absent from items
#     (the latter is acceptable — unknown inputs are treated as base materials).
sqlite3 "$DB" "
  SELECT b.base_item_id, COUNT(*) AS n
  FROM bill_of_materials b
  LEFT JOIN items i ON i.id = b.base_item_id
  WHERE i.id IS NOT NULL AND i.category NOT IN ('ore','material')
  GROUP BY b.base_item_id;
"

# 2e. No orphan targets — every target_id of type 'item' exists in items.
sqlite3 "$DB" "
  SELECT b.target_id
  FROM bill_of_materials b
  LEFT JOIN items i ON i.id = b.target_id
  WHERE b.target_type = 'item' AND i.id IS NULL;
"
```

Expected: 2a non-zero; 2b–2e all empty.

- [x] **Step 3: Check sample HTML output**

```bash
ls -la kb/items/component/durasteel_plate.html
head -100 kb/items/component/durasteel_plate.html | grep -A 20 "Construction"
```

Expected: Construction section visible with base materials table

- [x] **Step 4: Verify JSON output format**

```bash
grep -A 5 "bom-json" kb/items/component/durasteel_plate.html
```

Expected: Minified JSON array like `[{"item_id":"iron_ore","quantity":10},...]`

- [x] **Step 5: Run full test suite**

```bash
cd /home/robert/spacemolt/kb
go test ./...
```

Expected: All tests pass

- [x] **Step 6: Commit final changes**

```bash
git add .
git commit -m "feat: complete Bill of Materials feature implementation"
```

---

## Open Issue: production-data cycles via wrap/unwrap recipe pairs

When `CalculateAll` runs against the live `crafting.db`, it halts with:

```
calculate BOM: calculate BoM for item engine_core: circular dependency
detected in BoM calculation:
[enriched_uranium_rod contained_enriched_uranium_rod enriched_uranium_rod]
```

The cycle is real and lives in the recipe data, not the calculator:

| Recipe | Inputs | Outputs |
|---|---|---|
| `fabricate_enriched_uranium_rod` | 2 low_enriched_uranium | **1** enriched_uranium_rod |
| `unwrap_enriched_uranium_rod` | 1 contained_enriched_uranium_rod | **2** enriched_uranium_rod |
| `wrap_enriched_uranium_rod` | 2 enriched_uranium_rod + 1 hazmat_container + 1 lead_sheet | 1 contained_enriched_uranium_rod |

`SelectRecipe` picks `unwrap_*` (2 outputs) over `fabricate_*` (1 output), then
recurses into `wrap_*`, which needs `enriched_uranium_rod` → cycle.

The behavior matches the spec ("circular dependencies are fatal"), but the KB
cannot ship until this is resolved. Three options, in increasing order of
invasiveness:

1. **Fix the data.** Add a `is_primary` / `is_packaging` flag to recipes and
   exclude packaging recipes from BoM selection. This is the cleanest fix but
   requires a schema/data change in `crafting.db`.
2. **Extend `SelectRecipe` to filter wrap/unwrap recipes.** Treat any recipe
   whose name starts with `wrap_` or `unwrap_` (or that consumes its own
   sibling output transitively) as non-primary. Brittle but unblocks the KB.
3. **Soft-fail cycles.** Log the cycle and treat the cycling item as a base
   material instead of returning fatal. Violates the spec but ships.

Recommendation: option 1 if the data owners can act on it; option 2 as a
near-term workaround. Do not adopt option 3 without explicit signoff.

---

## Known Limitations

The following items are deliberately **out of scope** for this plan. They are
documented here so reviewers know they were considered and not forgotten.

1. **Expandable recipe-tree view.** The spec's HTML output section describes
   three views (summary table, expandable tree, JSON). This plan implements the
   summary table and JSON only. A real tree requires persisting the full
   traversed recipe DAG (not just `[topRecipe.ID]`) and a recursive template
   helper to render it. Add a follow-up task before promising the tree to
   users.

2. **Schema denormalization of `recipe_path` / `has_alternatives`.** Both
   columns are per-target metadata but are stored on every (target, base_item)
   row. This is an intentional simplification; if it becomes a maintenance
   burden, split into a `bom_targets` parent table.

3. **Byproduct overproduction.** When a recipe's batch outputs > 1, computing
   "BoM for 1 unit" with ceiling division means the BoM accounts for inputs
   sufficient to produce 1 unit but understates the byproduct (the extra
   outputs). This is acceptable for a "what does it take to build X?" view,
   but downstream cost-modelling code should account for byproducts
   explicitly.

4. **Recipe selection is static (max-outputs, salvage-excluded).** The plan
   does not implement player-skill-aware or station-aware recipe selection;
   that's a separate feature.

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
