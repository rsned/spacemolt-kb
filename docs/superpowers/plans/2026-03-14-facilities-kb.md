# Facilities KB Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Facilities section to Spacemolt knowledge base with 541 station buildings, upgrade chains, recipe integration, and cross-references to Items/Recipes KB.

**Architecture:** Create new `facilities.go` following existing `ships.go`/`skills.go` pattern. Load facilities from JSON files with abstract loader interface for future SQLite migration. Generate HTML pages using embedded templates consistent with existing KB sections.

**Tech Stack:** Go 1.24+, `encoding/json`, `html/template`, existing `smui.css` styling

---

## File Structure

**New files:**
- `cmd/generate-items-kb/facilities.go` - All facility types, loaders, templates, generation functions (~800 lines)

**Modified files:**
- `cmd/generate-items-kb/main.go` - Add facilities generation phase after recipes (~30 lines)
- `kb/index.html` - Add Facilities card and navigation link (~15 lines)

---

## Chunk 1: Foundation - Facility Types and JSON Loader

### Task 1: Create facilities.go with type definitions

**Files:**
- Create: `cmd/generate-items-kb/facilities.go`

- [ ] **Step 1: Create file with package declaration and imports**

```go
// Command generate-items-kb facilities support.
package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	 htmltpl "html/template"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
)
```

- [ ] **Step 2: Add Facility struct**

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
```

- [ ] **Step 3: Add MaterialRef and RecipeSummary structs**

```go
// MaterialRef references an item with quantity.
type MaterialRef struct {
	ItemID   string `json:"item_id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

// RecipeSummary is a simplified recipe representation embedded in a facility.
type RecipeSummary struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	CraftingTime int           `json:"crafting_time"`
	Inputs       []MaterialRef `json:"inputs"`
	Outputs      []MaterialRef `json:"outputs"`
}

// FacilityCategoryInfo groups facilities for page generation.
type FacilityCategoryInfo struct {
	Name        string
	Description string
	Count       int
	Facilities  []*Facility
}
```

- [ ] **Step 4: Add JSON loader structs for unmarshaling**

```go
// facilityJSON is the raw JSON structure from facility_details/*.json
type facilityJSON struct {
	Action               string          `json:"action"`
	BuildCost            int             `json:"build_cost"`
	BuildMaterials       []MaterialRef   `json:"build_materials"`
	BuildTime            int             `json:"build_time"`
	Buildable            bool            `json:"buildable"`
	Category             string          `json:"category"`
	Description          string          `json:"description"`
	Hint                 string          `json:"hint"`
	LaborCost            int             `json:"labor_cost"`
	Level                int             `json:"level"`
	MaintenancePerCycle  []MaterialRef   `json:"maintenance_per_cycle"`
	Name                 string          `json:"name"`
	Recipe               *RecipeSummary  `json:"recipe"`
	RecipeID             string          `json:"recipe_id"`
	RecipeMultiplier     float64         `json:"recipe_multiplier"`
	RentPerCycle         int             `json:"rent_per_cycle"`
	TypeID               string          `json:"type_id"`
	UpgradesFrom         string          `json:"upgrades_from"`
	UpgradesFromName     string          `json:"upgrades_from_name"`
	UpgradesTo           string          `json:"upgrades_to"`
	UpgradesToName       string          `json:"upgrades_to_name"`
	SatisfiedDescription string          `json:"satisfied_description"`
	DegradedDescription  string          `json:"degraded_description"`
}
```

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "feat(facilities): add type definitions and JSON structs"
```

### Task 2: Implement JSONFacilityLoader

**Files:**
- Modify: `cmd/generate-items-kb/facilities.go`

- [ ] **Step 1: Add loadFacilitiesFromJSON function**

```go
// loadFacilitiesFromJSON loads all facility JSON files from the given directory.
func loadFacilitiesFromJSON(dir string) (map[string]*Facility, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read facility directory: %w", err)
	}

	facilities := make(map[string]*Facility)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("warning: read facility file %s: %v", entry.Name(), err)
			continue
		}

		var raw facilityJSON
		if err := json.Unmarshal(data, &raw); err != nil {
			log.Printf("warning: unmarshal facility file %s: %v", entry.Name(), err)
			continue
		}

		if raw.Action != "types" {
			log.Printf("warning: facility file %s has unexpected action: %s", entry.Name(), raw.Action)
			continue
		}

		fac := convertJSONToFacility(&raw)
		facilities[fac.ID] = fac
	}

	return facilities, nil
}
```

- [ ] **Step 2: Add convertJSONToFacility helper**

```go
// convertJSONToFacility converts raw JSON to Facility struct.
func convertJSONToFacility(raw *facilityJSON) *Facility {
	fac := &Facility{
		ID:               raw.TypeID,
		Name:             raw.Name,
		Description:      raw.Description,
		Category:         raw.Category,
		Level:            raw.Level,
		Buildable:        raw.Buildable,
		BuildCost:        raw.BuildCost,
		BuildTime:        raw.BuildTime,
		LaborCost:        raw.LaborCost,
		RentPerCycle:     raw.RentPerCycle,
		RecipeMultiplier: raw.RecipeMultiplier,
		BuildMaterials:   raw.BuildMaterials,
		MaintenancePerCycle: raw.MaintenancePerCycle,
		Recipe:           raw.Recipe,
	}

	// Handle optional string pointer fields
	if raw.UpgradesFrom != "" {
		fac.UpgradesFrom = &raw.UpgradesFrom
	}
	if raw.UpgradesFromName != "" {
		fac.UpgradesFromName = &raw.UpgradesFromName
	}
	if raw.UpgradesTo != "" {
		fac.UpgradesTo = &raw.UpgradesTo
	}
	if raw.UpgradesToName != "" {
		fac.UpgradesToName = &raw.UpgradesToName
	}
	if raw.SatisfiedDescription != "" {
		fac.SatisfiedDescription = &raw.SatisfiedDescription
	}
	if raw.DegradedDescription != "" {
		fac.DegradedDescription = &raw.DegradedDescription
	}
	if raw.Hint != "" {
		fac.Hint = &raw.Hint
	}

	return fac
}
```

- [ ] **Step 3: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "feat(facilities): add JSON loader implementation"
```

### Task 3: Add category descriptions map

**Files:**
- Modify: `cmd/generate-items-kb/facilities.go`

- [ ] **Step 1: Add category descriptions**

```go
// facilityCategoryDescriptions maps categories to their descriptions.
var facilityCategoryDescriptions = map[string]string{
	"production":     "428 buildable facilities for crafting items, weapons, and equipment.",
	"service":        "68 station facilities providing trade, storage, repair, and services.",
	"faction":        "24 faction-specific buildings for diplomacy and warfare.",
	"infrastructure": "21 stations supporting power, fuel, and station operations.",
	"personal":       "4 compact facilities for personal crafting and storage.",
}
```

- [ ] **Step 2: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "feat(facilities): add category descriptions map"
```

---

## Chunk 2: HTML Templates

### Task 4: Add main facilities index template

**Files:**
- Modify: `cmd/generate-items-kb/facilities.go`

- [ ] **Step 1: Add facility site headers**

```go
// siteHeaderFacilities is the header for facilities main index.
var siteHeaderFacilities = `    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../">Home</a>
            <a href="../systems/">Systems</a>
            <a href="../items/">Items</a>
            <a href="../recipes/">Recipes</a>
            <a href="../skills/">Skills</a>
            <a href="../ships/">Ships</a>
            <a href="./">Facilities</a>
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>
        </nav>
    </header>`

// siteHeaderFacilitiesSub is the header for category and detail pages.
var siteHeaderFacilitiesSub = `    <header class="site-header">
        <h1><a href="../../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../../">Home</a>
            <a href="../../systems/">Systems</a>
            <a href="../../items/">Items</a>
            <a href="../../recipes/">Recipes</a>
            <a href="../../skills/">Skills</a>
            <a href="../../ships/">Ships</a>
            <a href="../../facilities/">Facilities</a>
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>
        </nav>
    </header>`
```

- [ ] **Step 2: Add main facilities index template**

```go
// htmlFacilitiesTopTemplate is the template for kb/facilities/index.html
var htmlFacilitiesTopTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Facilities - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
</head>
<body>
` + siteHeaderFacilities + `
    <main class="container page-content">
        <h2>Facilities</h2>
        <p class="text-muted mt-1">{{len .}} categories of station buildings for crafting, storage, and services.</p>
        <div class="item-categories">
{{- range .}}
            <a href="{{.Name}}/" class="item-cat-card">
                <div class="cat-count">{{.Count}} facilities</div>
                <div class="cat-name">{{titleCase .Name}}</div>
                <div class="cat-desc">{{.Description}}</div>
            </a>
{{- end}}
        </div>
    </main>
` + themeScript + `
</body>
</html>
`
```

- [ ] **Step 3: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "feat(facilities): add main index template and site headers"
```

### Task 5: Add category index template

**Files:**
- Modify: `cmd/generate-items-kb/facilities.go`

- [ ] **Step 1: Add category index template**

```go
// htmlFacilitiesCategoryTemplate is the template for kb/facilities/{category}/index.html
var htmlFacilitiesCategoryTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{titleCase .Name}} - Facilities - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../items/items.css">
</head>
<body>
` + siteHeaderFacilitiesSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Facilities</a> / {{titleCase .Name}}</div>
        <h2>{{titleCase .Name}}</h2>
        <p class="text-muted mt-1">{{.Description}}</p>
        <div class="card mt-3" style="padding:0">
        <table class="sortable">
        <thead>
        <tr><th class="sortable">Name</th><th class="sortable">Level</th><th class="sortable" style="text-align:right">Build Cost</th><th class="sortable" style="text-align:right">Labor</th><th class="sortable" style="text-align:right">Rent</th><th class="sortable">Recipe Output</th></tr>
        </thead>
        <tbody>
{{- range .Facilities}}
        <tr>
          <td><a href="{{.ID}}.html">{{.Name}}</a>{{if .Buildable}} <span class="badge badge-buildable" title="Buildable">B</span>{{end}}</td>
          <td data-sort="{{.Level}}">{{.Level}}</td>
          <td class="value" data-sort="{{.BuildCost}}">{{fmtValue .BuildCost}}</td>
          <td data-sort="{{.LaborCost}}">{{.LaborCost}}</td>
          <td data-sort="{{.RentPerCycle}}">{{.RentPerCycle}}</td>
          <td>{{if .Recipe}}<a href="../../recipes/{{dirName .Recipe.Category}}/{{.Recipe.ID}}.html">{{.Recipe.Outputs}}</a>{{else}}-{{end}}</td>
        </tr>
{{- end}}
        </tbody>
        </table>
        </div>
    </main>
` + sortScript + `
` + themeScript + `
</body>
</html>
`
```

- [ ] **Step 2: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "feat(facilities): add category index template with sortable table"
```

### Task 6: Add facility detail page template

**Files:**
- Modify: `cmd/generate-items-kb/facilities.go`

- [ ] **Step 1: Add facility detail template**

```go
// htmlFacilityDetailTemplate is the template for kb/facilities/{category}/{facility}.html
var htmlFacilityDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Facilities - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../items/items.css">
</head>
<body>
` + siteHeaderFacilitiesSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Facilities</a> / <a href="../">{{titleCase .Category}}</a> / {{.Name}}</div>
        <h2>{{.Name}}</h2>

        <!-- Header Stats -->
        <div class="card mt-3">
            <div class="stat-row">
                <div class="stat">
                    <span class="stat-label">Level</span>
                    <span class="stat-value">{{.Level}}</span>
                </div>
                <div class="stat">
                    <span class="stat-label">Build Cost</span>
                    <span class="stat-value">{{fmtValue .BuildCost}}</span>
                </div>
                <div class="stat">
                    <span class="stat-label">Labor</span>
                    <span class="stat-value">{{.LaborCost}}</span>
                </div>
                <div class="stat">
                    <span class="stat-label">Rent/Cycle</span>
                    <span class="stat-value">{{.RentPerCycle}}</span>
                </div>
                {{if .Buildable}}
                <div class="stat">
                    <span class="stat-label">Buildable</span>
                    <span class="stat-value">Yes</span>
                </div>
                {{end}}
            </div>
        </div>

        <!-- Description -->
        <p class="mt-3">{{.Description}}</p>

        {{if .UpgradesFromName or .UpgradesToName}}
        <!-- Upgrade Chain -->
        <div class="card mt-3">
            <h3>Upgrade Chain</h3>
            <div class="upgrade-chain">
                {{if .UpgradesFromName}}
                <a href="{{.UpgradesFrom}}.html">{{.UpgradesFromName}}</a>
                <span class="upgrade-arrow">&rarr;</span>
                {{end}}
                <strong>{{.Name}}</strong>
                {{if .UpgradesToName}}
                <span class="upgrade-arrow">&rarr;</span>
                <a href="{{.UpgradesTo}}.html">{{.UpgradesToName}}</a>
                {{end}}
            </div>
        </div>
        {{end}}

        {{if .Recipe}}
        <!-- Recipe Section -->
        <div class="card mt-3">
            <h3>Produces</h3>
            <p><a href="../../recipes/{{dirName .Recipe.Category}}/{{.Recipe.ID}}.html">{{.Recipe.Name}}</a></p>
            <table class="sortable">
                <thead><tr><th>Inputs</th><th>Outputs</th><th>Time</th></tr></thead>
                <tbody>
                    <tr>
                        <td>
                            {{range .Recipe.Inputs}}
                            <a href="../../items/{{.ItemID}}.html">{{.Name}}</a> x{{.Quantity}}<br>
                            {{end}}
                        </td>
                        <td>
                            {{range .Recipe.Outputs}}
                            <a href="../../items/{{.ItemID}}.html">{{.Name}}</a> x{{.Quantity}}<br>
                            {{end}}
                        </td>
                        <td>{{.Recipe.CraftingTime}}s</td>
                    </tr>
                </tbody>
            </table>
            <p class="text-muted mt-1"><a href="../../recipes/{{dirName .Recipe.Category}}/{{.Recipe.ID}}.html">See full recipe details &rarr;</a></p>
        </div>
        {{end}}

        <!-- Build Materials -->
        {{if .BuildMaterials}}
        <div class="card mt-3">
            <h3>Build Materials</h3>
            <table class="sortable">
                <thead><tr><th>Material</th><th style="text-align:right">Quantity</th></tr></thead>
                <tbody>
                    {{range .BuildMaterials}}
                    <tr>
                        <td><a href="../../items/{{.ItemID}}.html">{{.Name}}</a></td>
                        <td style="text-align:right">{{.Quantity}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}

        <!-- Maintenance -->
        {{if .MaintenancePerCycle}}
        <div class="card mt-3">
            <h3>Maintenance Per Cycle</h3>
            <table class="sortable">
                <thead><tr><th>Material</th><th style="text-align:right">Quantity</th></tr></thead>
                <tbody>
                    {{range .MaintenancePerCycle}}
                    <tr>
                        <td><a href="../../items/{{.ItemID}}.html">{{.Name}}</a></td>
                        <td style="text-align:right">{{.Quantity}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{end}}

        {{if .SatisfiedDescription}}
        <!-- Satisfied Description -->
        <div class="card mt-3" style="border-left: 3px solid hsl(var(--success));">
            <h4>Operational</h4>
            <p>{{.SatisfiedDescription}}</p>
        </div>
        {{end}}

        {{if .DegradedDescription}}
        <!-- Degraded Description -->
        <div class="card mt-3" style="border-left: 3px solid hsl(var(--destructive));">
            <h4>Degraded</h4>
            <p>{{.DegradedDescription}}</p>
        </div>
        {{end}}

        {{if .Hint}}
        <!-- Hint -->
        <div class="card mt-3" style="background: hsl(var(--muted)); border: none;">
            <strong>Hint:</strong> {{.Hint}}
        </div>
        {{end}}

    </main>
` + sortScript + `
` + themeScript + `
</body>
</html>
`
```

- [ ] **Step 2: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "feat(facilities): add facility detail page template"
```

---

## Chunk 3: Page Generation Functions

### Task 7: Implement main page generation function

**Files:**
- Modify: `cmd/generate-items-kb/facilities.go`

- [ ] **Step 1: Add writeFacilityPages function**

```go
// writeFacilityPages generates all facility HTML pages.
func writeFacilityPages(outDir string, facilities map[string]*Facility) error {
	// Group facilities by category
	catFacilities := make(map[string][]*Facility)
	for _, fac := range facilities {
		catFacilities[fac.Category] = append(catFacilities[fac.Category], fac)
	}

	// Sort facilities within each category
	for _, facList := range catFacilities {
		slices.SortFunc(facList, func(a, b *Facility) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}

	// Build category info
	categories := make([]FacilityCategoryInfo, 0, len(catFacilities))
	for cat, facList := range catFacilities {
		categories = append(categories, FacilityCategoryInfo{
			Name:        cat,
			Description: facilityCategoryDescriptions[cat],
			Count:       len(facList),
			Facilities:  facList,
		})
	}
	slices.SortFunc(categories, func(a, b FacilityCategoryInfo) int {
		return cmp.Compare(a.Name, b.Name)
	})

	// Create template functions
	funcs := htmltpl.FuncMap{
		"fmtValue":  fmtValue,
		"titleCase": titleCase,
		"dirName":   dirName,
	}

	// Parse templates
	topTmpl := htmltpl.Must(htmltpl.New("top").Funcs(funcs).Parse(htmlFacilitiesTopTemplate))
	catTmpl := htmltpl.Must(htmltpl.New("cat").Funcs(funcs).Parse(htmlFacilitiesCategoryTemplate))
	facTmpl := htmltpl.Must(htmltpl.New("fac").Funcs(funcs).Parse(htmlFacilityDetailTemplate))

	// Create output directory
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Write main index
	topPath := filepath.Join(outDir, "index.html")
	f, err := os.Create(topPath)
	if err != nil {
		return err
	}
	if err := topTmpl.Execute(f, categories); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Write category indexes and facility pages
	for _, cat := range categories {
		catDir := filepath.Join(outDir, cat.Name)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return err
		}

		// Category index
		catPath := filepath.Join(catDir, "index.html")
		f, err := os.Create(catPath)
		if err != nil {
			return err
		}
		if err := catTmpl.Execute(f, cat); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}

		// Individual facility pages
		for _, fac := range cat.Facilities {
			facPath := filepath.Join(catDir, fac.ID+".html")
			f, err := os.Create(facPath)
			if err != nil {
				return err
			}
			if err := facTmpl.Execute(f, fac); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}

	return nil
}
```

- [ ] **Step 2: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "feat(facilities): add page generation function"
```

### Task 8: Add recipe validation function

**Files:**
- Modify: `cmd/generate-items-kb/facilities.go`

- [ ] **Step 1: Add validateFacilityRecipes function**

```go
// validateFacilityRecipes checks facility recipes against the recipe map.
func validateFacilityRecipes(facilities map[string]*Facility, recipes map[string]*Recipe) {
	for _, fac := range facilities {
		if fac.Recipe == nil {
			continue
		}

		recipe, ok := recipes[fac.Recipe.ID]
		if !ok {
			log.Printf("warning: facility %s recipe %s not found in recipe KB", fac.ID, fac.Recipe.ID)
			continue
		}

		// Compare inputs
		if len(fac.Recipe.Inputs) != len(recipe.Inputs) {
			log.Printf("warning: facility %s recipe input count mismatch: facility=%d, recipe=%d",
				fac.ID, len(fac.Recipe.Inputs), len(recipe.Inputs))
		}

		// Compare outputs
		if len(fac.Recipe.Outputs) != len(recipe.Outputs) {
			log.Printf("warning: facility %s recipe output count mismatch: facility=%d, recipe=%d",
				fac.ID, len(fac.Recipe.Outputs), len(recipe.Outputs))
		}
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "feat(facilities): add recipe validation function"
```

---

## Chunk 4: Integration with Main Generator

### Task 9: Add facilities generation phase to main.go

**Files:**
- Modify: `cmd/generate-items-kb/main.go`

- [ ] **Step 1: Locate ship generation phase in main()**

Find the ship generation block around line 456-470:

```go
// --- Ship generation ---
shipCatalog, err := loadShipCatalog(shipCatalogPath)
if err != nil {
    log.Printf("warning: load ship catalog: %v (ship pages will be skipped)", err)
} else {
    // Build recipe name lookup for passive recipe display.
    recipeNames := make(map[string]string)
    for _, r := range recipes {
        recipeNames[r.ID] = r.Name
    }
    if err := writeShipPages("kb/ships", shipCatalog, recipeNames); err != nil {
        log.Fatalf("write ship pages: %v", err)
    }
    fmt.Printf("Generated %d ship entries in kb/ships/\n", len(shipCatalog))
}
```

- [ ] **Step 2: Add facilities generation phase after ship generation**

```go
// --- Facilities generation ---
facilityJSONDir := filepath.Join(catalogDir, "facility_details")
facilityOutDir := "kb/facilities"

facilities, err := loadFacilitiesFromJSON(facilityJSONDir)
if err != nil {
    log.Printf("warning: load facilities: %v (facility pages will be skipped)", err)
} else {
    // Validate facility recipes against loaded recipes
    validateFacilityRecipes(facilities, recipes)

    if err := writeFacilityPages(facilityOutDir, facilities); err != nil {
        log.Fatalf("write facility pages: %v", err)
    }
    fmt.Printf("Generated %d facility pages in kb/facilities/\n", len(facilities))
}
```

- [ ] **Step 3: Commit**

```bash
git add cmd/generate-items-kb/main.go
git commit -m "feat: integrate facilities generation into main"
```

---

## Chunk 5: Main KB Index Updates

### Task 10: Add Facilities navigation link to site headers

**Files:**
- Modify: `cmd/generate-items-kb/main.go`

- [ ] **Step 1: Update siteHeader variable**

Find `siteHeader` around line 1588 and add Facilities link:

Change from:
```go
var siteHeader = `    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../">Home</a>
            <a href="../systems/">Systems</a>
            <a href="../items/">Items</a>
            <a href="../recipes/">Recipes</a>
            <a href="../skills/">Skills</a>
            <a href="../ships/">Ships</a>
```

To:
```go
var siteHeader = `    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../">Home</a>
            <a href="../systems/">Systems</a>
            <a href="../items/">Items</a>
            <a href="../recipes/">Recipes</a>
            <a href="../skills/">Skills</a>
            <a href="../ships/">Ships</a>
            <a href="../facilities/">Facilities</a>
```

- [ ] **Step 2: Update siteHeaderSub variable**

Find `siteHeaderSub` around line 1605 and add Facilities link:

Change from:
```go
var siteHeaderSub = `    <header class="site-header">
        <h1><a href="../../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../../">Home</a>
            <a href="../../systems/">Systems</a>
            <a href="../../items/">Items</a>
            <a href="../../recipes/">Recipes</a>
            <a href="../../skills/">Skills</a>
            <a href="../../ships/">Ships</a>
```

To:
```go
var siteHeaderSub = `    <header class="site-header">
        <h1><a href="../../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../../">Home</a>
            <a href="../../systems/">Systems</a>
            <a href="../../items/">Items</a>
            <a href="../../recipes/">Recipes</a>
            <a href="../../skills/">Skills</a>
            <a href="../../ships/">Ships</a>
            <a href="../../facilities/">Facilities</a>
```

- [ ] **Step 3: Commit**

```bash
git add cmd/generate-items-kb/main.go
git commit -m "feat: add Facilities link to site navigation headers"
```

### Task 11: Add Facilities card to main KB index

**Files:**
- Modify: `kb/index.html`

- [ ] **Step 1: Add Facilities link to navigation**

Find the navigation section around line 200-204 and add Facilities link:

Change from:
```html
<a href="systems/">Systems</a>
<a href="items/">Items</a>
<a href="recipes/">Recipes</a>
<a href="skills/">Skills</a>
<a href="ships/">Ships</a>
```

To:
```html
<a href="systems/">Systems</a>
<a href="items/">Items</a>
<a href="recipes/">Recipes</a>
<a href="skills/">Skills</a>
<a href="ships/">Ships</a>
<a href="facilities/">Facilities</a>
```

- [ ] **Step 2: Remove cat-wide class from Ships card**

Find the Ships card around line 293 and remove the `cat-wide` class:

Change from:
```html
<a href="ships/" class="cat cat-wide" style="--cat-accent: hsl(var(--smui-purple))">
```

To:
```html
<a href="ships/" class="cat" style="--cat-accent: hsl(var(--smui-purple))">
```

- [ ] **Step 3: Add Facilities card after Ships**

Add the Facilities card after the Ships card (before the closing `</div>` of categories):

```html
<a href="facilities/" class="cat" style="--cat-accent: hsl(var(--smui-cyan))">
    <div class="cat-top">
        <span class="cat-label">06 &mdash; Stations</span>
        <span class="cat-stat">541 facilities</span>
    </div>
    <div class="cat-name">
        <span class="cat-glyph">&#x2693;</span>
        Facilities
        <span class="cat-arrow">&rarr;</span>
    </div>
    <p class="cat-desc">Station buildings for crafting, storage, trade, and services. Upgrade chains and recipe integration.</p>
    <div class="cat-tags">
        <span class="cat-tag">Production</span>
        <span class="cat-tag">Service</span>
        <span class="cat-tag">Faction</span>
        <span class="cat-tag">Infrastructure</span>
        <span class="cat-tag">Personal</span>
    </div>
</a>
```

- [ ] **Step 4: Add cyan CSS variable if needed**

Check if `--smui-cyan` exists in `kb/smui.css`. If not, add to the CSS variables section around line 80:

```css
--smui-cyan: #00ced1;
```

- [ ] **Step 5: Commit**

```bash
git add kb/index.html kb/smui.css
git commit -m "feat: add Facilities card to main KB index"
```

---

## Chunk 6: CSS Enhancements

### Task 12: Add CSS for upgrade chain diagram and buildable badge

**Files:**
- Modify: `kb/smui.css`

- [ ] **Step 1: Add upgrade chain styles**

Add to the end of `kb/smui.css`:

```css
/* Upgrade chain diagram */
.upgrade-chain {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    font-size: var(--text-ui);
}

.upgrade-chain a {
    color: hsl(var(--primary));
    text-decoration: none;
}

.upgrade-chain a:hover {
    text-decoration: underline;
}

.upgrade-arrow {
    color: hsl(var(--muted-foreground));
    font-weight: bold;
}

/* Buildable badge */
.badge-buildable {
    background: hsl(var(--success));
    color: hsl(var(--success-foreground));
}
```

- [ ] **Step 2: Commit**

```bash
git add kb/smui.css
git commit -m "style: add upgrade chain and buildable badge styles"
```

---

## Chunk 7: Testing and Validation

### Task 13: Run full KB generation

**Files:**
- Test: Run generator

- [ ] **Step 1: Build the generator**

```bash
go build ./cmd/generate-items-kb
```

Expected: Binary created without errors

- [ ] **Step 2: Run the generator**

```bash
./generate-items-kb \
  ../spacemolt-crafting-server/database/crafting.db \
  kb/items \
  kb/recipes
```

Expected: Output includes "Generated 541 facility pages in kb/facilities/"

- [ ] **Step 3: Verify facilities directory structure**

```bash
ls -la kb/facilities/
```

Expected output:
```
index.html
production/
service/
faction/
infrastructure/
personal/
```

- [ ] **Step 4: Check category counts**

```bash
ls kb/facilities/production/ | wc -l
ls kb/facilities/service/ | wc -l
ls kb/facilities/faction/ | wc -l
ls kb/facilities/infrastructure/ | wc -l
ls kb/facilities/personal/ | wc -l
```

Expected: 429 (428 + index), 69 (68 + index), 25 (24 + index), 22 (21 + index), 5 (4 + index)

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: verify facilities KB generation"
```

### Task 14: Validate page content

**Files:**
- Test: Inspect generated HTML

- [ ] **Step 1: Check main facilities index**

```bash
grep -c "cat-cat-card" kb/facilities/index.html
```

Expected: 5 (one per category)

- [ ] **Step 2: Check category index table headers**

```bash
grep -A 2 "<thead>" kb/facilities/production/index.html | head -5
```

Expected: Table with Name, Level, Build Cost, Labor, Rent, Recipe Output columns

- [ ] **Step 3: Check a facility detail page has upgrade chain**

```bash
grep -c "upgrade-chain" kb/facilities/production/accelerator_assembly_plant.html
```

Expected: 1

- [ ] **Step 4: Check item links are correct**

```bash
grep -o 'href="../../items/[^"]*"' kb/facilities/production/accelerator_assembly_plant.html | head -3
```

Expected: Links like `href="../../items/steel_plate.html"`

- [ ] **Step 5: Check recipe links are correct**

```bash
grep -o 'href="../../recipes/[^"]*"' kb/facilities/production/accelerator_assembly_plant.html
```

Expected: Links like `href="../../recipes/Weapons/build_piercing_railgun_i.html"`

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test: validate facilities page content"
```

### Task 15: Verify navigation consistency

**Files:**
- Test: Check all pages have correct navigation

- [ ] **Step 1: Check main KB index has Facilities link**

```bash
grep "facilities/" kb/index.html
```

Expected: 2 matches (navigation link and Facilities card)

- [ ] **Step 2: Check item pages still have correct navigation**

```bash
grep -c "facilities/" kb/items/ore/index.html
```

Expected: 1 (navigation link)

- [ ] **Step 3: Check facility pages link back to items**

```bash
grep -c '../../items/' kb/facilities/production/index.html | head -1
```

Expected: Multiple item links in table

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: verify navigation consistency across KB"
```

### Task 16: Run golangci-lint

**Files:**
- Test: Code quality check

- [ ] **Step 1: Run linter**

```bash
golangci-lint run ./cmd/generate-items-kb/...
```

Expected: No new findings (existing findings are OK)

- [ ] **Step 2: If there are new findings, fix them**

Address any new lint findings in `facilities.go`

- [ ] **Step 3: Commit**

```bash
git add cmd/generate-items-kb/facilities.go
git commit -m "lint: fix golangci-lint findings"
```

---

## Completion Checklist

- [ ] All 541 facilities load from JSON
- [ ] All 5 category pages generated with correct counts
- [ ] All facility detail pages generated
- [ ] Upgrade chains render with clickable links
- [ ] Recipe sections link to correct recipe pages
- [ ] Item links resolve to correct item pages
- [ ] Main KB index includes Facilities card
- [ ] Navigation headers include Facilities link on all pages
- [ ] CSS styles applied consistently
- [ ] golangci-lint passes without new findings
- [ ] Full KB generation completes successfully

---

**Total estimated time:** 60-90 minutes

**Handoff:** After plan approval, use `superpowers:subagent-driven-development` to execute each task with two-stage review (subagent executes, reviewer validates).
