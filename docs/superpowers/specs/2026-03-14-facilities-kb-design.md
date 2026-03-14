# Facilities KB Feature Design

**Date:** 2026-03-14
**Author:** Design specification for adding Facilities section to the knowledge base
**Status:** Approved

## Overview

Add a new first-class "Facilities" section to the Spacemolt knowledge base static site generator. Facilities are station-based building entities that can be built/upgraded and used for crafting, storage, trade, and other services. The feature displays upgrade chains, recipe integration, and cross-references with existing Items and Recipes KB sections.

## Requirements

### Functional Requirements

1. **Data Loading**: Load facility data from JSON files in `data/game-api/prophet-2/facility_details/`
2. **Categorization**: Support 5 facility categories (production, service, faction, infrastructure, personal)
3. **Page Generation**: Generate HTML pages for:
   - Main facilities index with category cards
   - Per-category index pages with sortable tables
   - Individual facility detail pages
4. **Interlinking**: Link facilities to Items and Recipes KB sections
5. **Upgrade Chains**: Visualize upgrade progression between facilities
6. **Recipe Integration**: Display embedded recipe data with links to full recipe pages
7. **Data Validation**: Flag mismatches between facility recipes and Recipes KB

### Non-Functional Requirements

1. **Extensibility**: Design data loader interface to support future SQLite migration
2. **Consistency**: Follow existing KB patterns (items, recipes, skills, ships)
3. **Maintainability**: Keep facility code in separate file (`facilities.go`)
4. **Performance**: Efficient cross-referencing with existing KB data

## Data Model

### Core Types

```go
// Facility represents a station-based building entity.
type Facility struct {
    // Identity
    ID          string
    Name        string
    Description string
    Category    string // production, service, faction, infrastructure, personal

    // Build requirements
    Level      int
    Buildable  bool
    BuildCost  int
    BuildTime  int
    LaborCost  int
    RentPerCycle int
    RecipeMultiplier float64

    // Upgrade chain
    UpgradesFrom     *string // facility ID
    UpgradesFromName *string
    UpgradesTo       *string // facility ID
    UpgradesToName   *string

    // Embedded data
    BuildMaterials      []MaterialRef
    MaintenancePerCycle []MaterialRef
    Recipe              *RecipeSummary

    // Optional descriptive fields
    SatisfiedDescription *string
    DegradedDescription  *string
    Hint                 *string
}

// MaterialRef references an item with quantity.
type MaterialRef struct {
    ItemID   string
    Name     string
    Quantity int
}

// RecipeSummary is a simplified recipe representation embedded in a facility.
type RecipeSummary struct {
    ID           string
    Name         string
    CraftingTime int
    Inputs       []MaterialRef
    Outputs      []MaterialRef
}

// FacilityCategoryInfo groups facilities for page generation.
type FacilityCategoryInfo struct {
    Name        string
    Description string
    Count       int
    Facilities  []*Facility
}
```

### Data Source Interface

```go
// FacilityLoader abstracts the data source (JSON or SQLite).
type FacilityLoader interface {
    LoadFacilities() (map[string]*Facility, error)
}

// JSONFacilityLoader loads facilities from individual JSON files.
type JSONFacilityLoader struct {
    directory string // path to facility_details/*.json
}

func (l *JSONFacilityLoader) LoadFacilities() (map[string]*Facility, error)

// SQLiteFacilityLoader loads facilities from a database table (future).
type SQLiteFacilityLoader struct {
    db *sql.DB
}

func (l *SQLiteFacilityLoader) LoadFacilities() (map[string]*Facility, error)
```

## Output Structure

```
kb/facilities/
├── index.html                    # Main landing with 5 category cards
├── production/
│   ├── index.html                # Sortable table of all production facilities
│   ├── accelerator_assembly_plant.html
│   ├── acid_bath_vat.html
│   └── ... (428 files)
├── service/
│   ├── index.html                # Sortable table of all service facilities
│   ├── warehouse.html
│   └── ... (68 files)
├── faction/
│   ├── index.html                # Sortable table of all faction facilities
│   └── ... (24 files)
├── infrastructure/
│   ├── index.html                # Sortable table of all infrastructure facilities
│   └── ... (21 files)
└── personal/
    ├── index.html                # Sortable table of all personal facilities
    └── ... (4 files)
```

## Page Templates

### Main Index Template (`kb/facilities/index.html`)

**Layout:**
- Header: "Facilities" with subtitle
- 5 category cards in responsive grid
- Each card shows:
  - Category icon (emoji or SVG)
  - Category name (title case)
  - Facility count
  - Description

**Category Descriptions:**
- Production: "428 buildable facilities for crafting items, weapons, and equipment."
- Service: "68 station facilities providing trade, storage, repair, and services."
- Faction: "24 faction-specific buildings for diplomacy and warfare."
- Infrastructure: "21 stations supporting power, fuel, and station operations."
- Personal: "4 compact facilities for personal crafting and storage."

### Category Index Template (`kb/facilities/{category}/index.html`)

**Layout:**
- Breadcrumb: `<a href="../">Facilities</a> / {Category}`
- Header: Category name + description
- Sortable table with columns:
  1. Name (link to detail page)
  2. Level (badge)
  3. Build Cost (formatted)
  4. Labor Cost
  5. Rent Per Cycle
  6. Recipe Output (if applicable, else "-")

**Sorting:** All columns sortable by clicking headers

### Facility Detail Template (`kb/facilities/{category}/{facility_id}.html`)

**Layout Sections:**

1. **Header**
   - Facility name (H1)
   - Level badge (color-coded by tier)
   - Build cost, labor, rent in compact row

2. **Description**
   - Main description text

3. **Upgrade Chain Diagram**
   - Horizontal flow: `[Upgrades From] → [Current Facility] → [Upgrades To]`
   - Each facility name is a clickable link
   - Arrow separator between facilities
   - Handles start/end of chain gracefully

4. **Recipe Section** (if present)
   - Header: "Produces" + link to full recipe page
   - Recipe summary table:
     - Inputs (with item links)
     - Outputs (with item links)
     - Crafting time
   - Link: "See full recipe details" → `../recipes/{category}/{recipe_id}.html`

5. **Build Materials**
   - Table of required materials with quantities
   - Item names link to Items KB

6. **Maintenance Per Cycle**
   - Table of consumables required per cycle
   - Item names link to Items KB

7. **Satisfied/Degraded Descriptions** (if available)
   - Callout boxes with operational state descriptions

8. **Hint** (if available)
   - Info callout with build/upgrade hint text

## Cross-Referencing

### Item Links
All `MaterialRef` items link to Items KB:
```
../items/{item_id}.html
```

### Recipe Links
Facility recipe section links to Recipes KB:
```
../recipes/{category}/{recipe_id}.html
```

### Upgrade Chain Links
Each facility name in upgrade chain is clickable:
```
production/accelerator_assembly_plant.html
```

## Data Validation

During KB generation, validate facility recipes against Recipes KB:

1. Load all recipes from existing recipe data
2. For each facility with a recipe:
   - Check recipe ID exists in recipe map
   - Compare input items (count and IDs)
   - Compare output items (count and IDs)
3. Log warnings for mismatches:
   ```
   WARNING: facility {id} recipe mismatch:
     recipe_id: {id}
     facility inputs: {count} items, recipe inputs: {count} items
     missing in recipe: {item_ids}
     extra in recipe: {item_ids}
   ```
4. Generate page with mismatched data flagged visually

## Integration with Main KB

### Update Main Index (`kb/index.html`)

Add Facilities card to main landing page between Recipes and Skills:

```html
<a href="facilities/" class="section-card">
    <div class="section-icon">🏭</div>
    <div class="section-name">Facilities</div>
    <div class="section-count">541</div>
    <div class="section-desc">Station buildings for crafting, storage, and trade</div>
</a>
```

### Update Navigation Headers

Add Facilities link to all site headers:

**In `siteHeader` (top-level pages):**
```html
<a href="../facilities/">Facilities</a>
```

**In `siteHeaderSub` (nested pages):**
```html
<a href="../../facilities/">Facilities</a>
```

## Implementation Files

### New File: `cmd/generate-items-kb/facilities.go`

Contains all facility-related code:
- Facility type definitions
- FacilityLoader interface and implementations
- Template constants (HTML)
- Page generation functions
- Category descriptions map

### Modified File: `cmd/generate-items-kb/main.go`

Add facilities generation phase in `main()`:

```go
// --- Facility generation ---
facilityJSONDir := "../spacemolt/data/game-api/prophet-2/facility_details"
facilityOutDir := "kb/facilities"

facilities, err := loadFacilitiesFromJSON(facilityJSONDir)
if err != nil {
    log.Printf("warning: load facilities: %v (facility pages will be skipped)", err)
} else {
    // Validate facility recipes against loaded recipes
    validateFacilityRecipes(facilities, recipes)

    if err := writeFacilityPages(facilityOutDir, facilities, recipes); err != nil {
        log.Fatalf("write facility pages: %v", err)
    }
    fmt.Printf("Generated %d facility pages in kb/facilities/\n", len(facilities))
}
```

### Modified File: `kb/index.html`

Add Facilities card to main template (in `writeMainIndex` function).

## CSS Requirements

Reuse existing `smui.css` classes. No new CSS file needed for facilities.

Potential additions for upgrade chain diagram:
```css
.upgrade-chain {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
}
.upgrade-arrow {
    color: var(--text-muted);
}
```

## Error Handling

1. **Missing JSON files**: Log warning, skip facility
2. **Invalid JSON**: Log error, skip facility
3. **Missing item references**: Render as plain text (no link)
4. **Recipe mismatches**: Log warning, render with visual flag
5. **Missing upgrade targets**: Render link as plain text

## Future Enhancements

1. **SQLite Migration**: Implement `SQLiteFacilityLoader` when facilities table exists
2. **Facility Images**: Add image support if facility icons become available
3. **Advanced Filtering**: Add filter controls to category index pages
4. **Upgrade Chain Visualization**: Render full chain diagram with all intermediate facilities
5. **Faciciency Calculator**: Add tool to compare facility efficiency within upgrade chains

## Acceptance Criteria

- [ ] All 541 facilities load from JSON files
- [ ] 5 category index pages generated with correct facility counts
- [ ] Main facilities index displays all 5 category cards
- [ ] Individual facility pages show all data fields
- [ ] Upgrade chains render correctly with clickable links
- [ ] Recipe sections link to correct recipe pages
- [ ] Item links resolve to correct item pages
- [ ] Recipe mismatches are logged during generation
- [ ] Main KB index includes Facilities card
- [ ] Navigation headers include Facilities link
- [ ] All pages use consistent styling with existing KB
- [ ] golangci-lint passes without new findings

## Success Metrics

1. **Completeness**: 100% of facilities in JSON files have KB pages
2. **Accuracy**: All upgrade chains link to valid facility pages
3. **Interlinking**: All item/recipe references resolve correctly
4. **Validation**: All recipe mismatches identified and logged
5. **Performance**: Generation completes in < 5 seconds for 541 facilities
