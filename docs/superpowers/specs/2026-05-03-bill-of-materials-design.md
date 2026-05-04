# Bill of Materials (BoM) Feature Design

## Overview

Add recursive Bill of Materials calculation to the KB generation pipeline that traces item, ship, and facility construction materials down to base components (ore and material categories). Display BoM data as summary tables, expandable trees, and minified JSON on KB pages.

## Requirements

### Functional Requirements
- Calculate BoM for all craftable items, ships, and facilities
- Recursively trace recipes through multiple crafting tiers to base materials
- Stop recursion at items in "ore" and "material" categories (base components)
- Use memoization to cache intermediate calculations and improve performance
- Detect circular dependencies and treat as fatal errors
- Handle items with multiple producing recipes using optimal selection rules
- Display BoM data as: summary table + expandable tree + minified JSON array
- Store BoM data in knowledge database `bill_of_materials` table
- Always recalculate during KB generation (no complex caching/invalidation)

### Recipe Selection Rules
- Prefer recipe with the most outputs (highest total output quantity)
- Exclude recipes requiring salvage components (rare_salvage, salvage_metal, etc.) if alternatives exist
- Fall back to alphabetical selection (first recipe) if all options use salvage components

### Non-Functional Requirements
- Handle edge cases gracefully: not craftable items, missing data, broken recipe references
- Log warnings for non-fatal issues, halt for critical errors
- Maintain existing KB generation workflow (single command to run)
- Performance: memoization should prevent redundant calculations

## Architecture

### Components

#### 1. Calculator (`pkg/bom/calculator.go`)
Core recursive algorithm with memoization for BoM computation.

**Key Methods:**
- `NewCalculator(db, recipes, items)` - Initialize calculator with database and data
- `Calculate(itemID, quantity)` - Recursive calculation returning base materials
- `CalculateAll(items, ships, facilities)` - Batch calculation for all targets
- `ClearDatabase()` - Remove old BoM data before regeneration

**Algorithm:**
```
Calculate(itemID, quantity):
1. Check if itemID is base material (ore/material category)
   - If yes: return [{itemID, quantity}]

2. Check memo cache for itemID
   - If cached: return cached result scaled by quantity

3. Add itemID to visited set (circular dependency detection)
   - If already visited: FATAL ERROR (circular dependency)

4. Select optimal recipe for itemID
   - Get all recipes that produce itemID
   - If none: return [{itemID, quantity}] (not craftable)
   - Filter out recipes using salvage components if alternatives exist
   - Choose recipe with most outputs (largest sum of output quantities)

5. For each input in selected recipe:
   - Recursively: Calculate(input.itemID, input.quantity * quantityMultiplier)
   - Accumulate all returned materials

6. Store result in memo cache
7. Remove itemID from visited set
8. Return accumulated materials
```

#### 2. Recipe Resolver (`pkg/bom/recipes.go`)
Handles recipe selection logic and reverse lookups.

**Key Methods:**
- `BuildRecipeMaps(recipes)` - Create item→recipes reverse lookup
- `SelectRecipe(itemID)` - Choose optimal recipe based on criteria
- `UsesSalvage(recipe)` - Check if recipe requires salvage components

**Data Structures:**
```go
type Recipe struct {
    ID           string
    Name         string
    Inputs       []RecipeItem
    Outputs      []RecipeItem
}

type RecipeItem struct {
    ItemID   string
    Quantity  int
}
```

#### 3. Database Layer (`pkg/bom/db.go`)
Handles persistence of BoM data to knowledge database.

**Key Methods:**
- `Migrate(db)` - Create `bill_of_materials` table
- `ClearBoM(db)` - Remove all existing BoM data
- `WriteBoM(db, result)` - Store single BoM result
- `GetBoM(db, targetID, targetType)` - Retrieve BoM for a target

#### 4. Integration
Modify `cmd/generate-items-kb/main.go` to use the BOM calculator.

## Data Structures

### Core Types

```go
// MaterialRequirement represents a single material in the BoM
type MaterialRequirement struct {
    ItemID   string
    Quantity  int
}

// BoMResult contains the complete breakdown for a target
type BoMResult struct {
    TargetID        string
    TargetName      string
    TargetType      string // "item", "ship", "facility"
    BaseMaterials    []MaterialRequirement
    RecipePath      []string  // IDs of recipes used (for documentation)
    HasAlternatives bool    // true if target had multiple recipe options
}

// Calculator holds state for BoM computation
type Calculator struct {
    db              *sql.DB
    recipes         map[string]*Recipe
    itemToRecipes   map[string][]*Recipe  // reverse lookup
    items           map[string]*Item
    memo            map[string][]MaterialRequirement  // memoization cache
    visited         map[string]struct{}  // circular dependency detection
}
```

### Database Schema

```sql
CREATE TABLE IF NOT EXISTS bill_of_materials (
    target_id      TEXT NOT NULL,
    target_type    TEXT NOT NULL,  -- "item", "ship", "facility"
    base_item_id   TEXT NOT NULL,
    quantity       INTEGER NOT NULL,
    recipe_path    TEXT,  -- JSON array of recipe IDs used
    has_alternatives BOOLEAN DEFAULT 0,
    PRIMARY KEY (target_id, target_type, base_item_id)
);
```

**Schema Notes:**
- `recipe_path` stores JSON array of recipe IDs selected during calculation
- `has_alternatives` indicates if the target had multiple recipe options
- Primary key ensures no duplicate entries for same target/base combination

### Extended Structs

```go
// Add to existing Item struct in generate-items-kb
type Item struct {
    // ... existing fields ...
    BoM *BoMResult // New field, nil if not craftable
}

// Add to existing Ship struct
type Ship struct {
    // ... existing fields ...
    BuildMaterials []ShipBuildMaterial
    BoM *BoMResult // New field
}

// Add to Facility struct
type Facility struct {
    // ... existing fields ...
    BuildMaterials []FacilityMaterial
    BoM *BoMResult // New field
}
```

## Error Handling

### Error Scenarios and Responses

1. **Circular Dependency Detected**
   - Error: `fatal: circular dependency detected in BoM calculation: steel_plate → durasteel_plate → steel_plate`
   - Action: Immediately halt execution with clear error message showing the cycle
   - Severity: FATAL (data integrity issue, must be fixed)

2. **Missing Item/Recipe in Database**
   - Error: `warning: recipe "forge_durasteel" references unknown item "mystery_plate", skipping`
   - Action: Log warning, continue with available data
   - Severity: WARNING (can happen with incomplete/partial database states)

3. **Division by Zero in Recipe Outputs**
   - Error: `warning: recipe "forge_thing" has zero total output, cannot calculate cost multiplier`
   - Action: Log warning, skip this recipe
   - Severity: WARNING (prevents panic, graceful degradation)

4. **Database Write Failures**
   - Error: `error: failed to write BoM for item "steel_plate": database locked`
   - Action: Retry up to 3 times with 1s delay, then fail
   - Severity: ERROR with retry (resilience for concurrent access)

5. **Facility/Ship with No Build Materials**
   - Action: Skip BoM calculation, BoM field remains nil
   - HTML template checks `hasBoM` before rendering Construction section
   - Severity: INFO (some items may not be craftable - drops only)

6. **Salvage Recipe Filtering Edge Cases**
   - Scenario: All recipes for an item use salvage components
   - Action: Fall back to alphabetical selection (first recipe), log warning
   - Severity: WARNING (graceful degradation while maintaining preference)

### Logging Strategy

- **Fatal errors**: Circular dependencies, database initialization failures
- **Warnings**: Missing data, division issues, salvage-only recipes
- **Info**: BoM calculation progress, recipe selection decisions

## HTML Template Integration

### Template Functions

Add to existing template functions map in `cmd/generate-items-kb/main.go`:

```go
"hasBoM": func(bom *BoMResult) bool {
    return bom != nil
},
"boMTable": func(bom *BoMResult) template.HTML {
    // Render summary table of base materials
},
"boMTree": func(bom *BoMResult) template.HTML {
    // Render expandable tree with tier-by-tier breakdown
},
"boMJSON": func(bom *BoMResult) string {
    // Render minified JSON array
},
```

### HTML Output Format

**Construction Section Structure:**

```html
<!-- Summary Table (always visible) -->
<div class="card" style="padding:0">
  <div class="section-label">Construction</div>
  <div class="bom-summary-table">
    <table>
      <thead>
        <tr><th>Base Material</th><th>Quantity</th></tr>
      </thead>
      <tbody>
        {{- range .BoM.BaseMaterials}}
        <tr>
          <td><a href="../../items/ore/{{.ItemID}}.html">{{.ItemID}}</a></td>
          <td>{{.Quantity}}</td>
        </tr>
        {{- end}}
      </tbody>
    </table>
  </div>

  <!-- Expandable Tree (click to expand) -->
  <details class="bom-tree-details">
    <summary>View Full Recipe Tree</summary>
    <div class="bom-tree">
      <!-- Hierarchical visualization of recipe breakdown -->
    </div>
  </details>

  <!-- Minified JSON (for developers) -->
  <details class="bom-json-details">
    <summary>View JSON Data</summary>
    <pre class="bom-json">{{boMJSON .BoM}}</pre>
  </details>
</div>
```

**JSON Output Format:**

```json
[{"item_id":"iron_ore","quantity":18750},{"item_id":"titanium_ore","quantity":4500},...]
```

- Minified format (no whitespace)
- Array of objects for easy downstream consumption
- Sorted by item_id for consistency

## Testing

### Unit Tests (`pkg/bom/calculator_test.go`)

1. **Base Material Detection**
   - Test that ore and material category items stop recursion immediately
   - Verify correct quantity scaling for base materials

2. **Simple Recipe Traversal**
   - Test single-tier recipe (e.g., Steel Plate → Iron Ore)
   - Verify correct quantity calculations (outputs → inputs scaling)

3. **Multi-tier Traversal**
   - Test complex chain (e.g., Durasteel Plate → Steel Plate → Iron Ore)
   - Verify memoization prevents redundant calculations
   - Confirm quantities compound correctly through tiers

4. **Multiple Recipe Selection**
   - Test item with 2+ producing recipes
   - Verify most-outputs recipe is chosen
   - Verify salvage recipes are filtered when alternatives exist

5. **Circular Dependency Detection**
   - Create test data with cycle (A→B→C→A)
   - Verify error is returned with clear cycle message
   - Confirm calculator halts without infinite recursion

6. **Memoization Effectiveness**
   - Calculate BoM for complex item with shared subcomponents
   - Verify memo cache has expected entries
   - Confirm performance improvement vs non-memoized version

### Integration Tests (`cmd/generate-items-kb/main_test.go`)

1. **End-to-End Generation**
   - Run full KB generation with BoM enabled
   - Verify `bill_of_materials` table is populated
   - Check HTML pages include Construction sections

2. **Database Persistence**
   - Calculate BoM, write to DB, read back
   - Verify data integrity (quantities preserved)
   - Test JSON serialization round-trips correctly

3. **Template Rendering**
   - Generate HTML with sample BoM data
   - Verify summary table displays correctly
   - Confirm JSON is minified and valid

### Test Data Strategy

- Use in-memory SQLite for isolation
- Create minimal test datasets (not full production DB)
- Include edge cases: circular deps, salvage-only items, missing data
- Test with realistic recipe data from production database

## Implementation Tasks

1. Create `pkg/bom` package with calculator, resolver, and database modules
2. Implement recursive calculation with memoization
3. Implement recipe selection with salvage filtering
4. Add circular dependency detection
5. Create database schema and migration
6. Integrate calculator into `cmd/generate-items-kb/main.go`
7. Add BoM fields to Item, Ship, Facility structs
8. Update HTML templates with Construction sections
9. Implement template functions for BoM rendering
10. Write unit tests for calculator
11. Write integration tests for full pipeline
12. Test with production database and verify output

## Success Criteria

- All craftable items, ships, and facilities have accurate BoM data
- Circular dependencies are detected and reported as fatal errors
- HTML pages display Construction sections with summary, tree, and JSON
- JSON output is minified and valid
- Performance: memoization reduces redundant calculations for complex items
- Recipe selection follows preference rules (max outputs, no salvage)
- Database stores BoM data efficiently and can be regenerated on demand
