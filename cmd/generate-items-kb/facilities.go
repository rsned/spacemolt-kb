// Command generate-items-kb facilities support.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

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
	ItemID   string `json:"item_id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

// RecipeSummary is a simplified recipe representation embedded in a facility.
type RecipeSummary struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Category     string        `json:"category"` // Required for linking to recipe pages
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

// facilityCategoryDescriptions maps categories to their descriptions.
var facilityCategoryDescriptions = map[string]string{
	"production":     "428 buildable facilities for crafting items, weapons, and equipment.",
	"service":        "68 station facilities providing trade, storage, repair, and services.",
	"faction":        "24 faction-specific buildings for diplomacy and warfare.",
	"infrastructure": "21 stations supporting power, fuel, and station operations.",
	"personal":       "4 compact facilities for personal crafting and storage.",
}

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

// htmlFacilitiesCategoryTemplate is the template for kb/facilities/{category}/index.html
var htmlFacilitiesCategoryTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{titleCase .Category}} - Facilities - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
</head>
<body>
` + siteHeaderFacilitiesSub + `
    <main class="container page-content">
        <h2>{{titleCase .Category}} <span class="text-muted">{{.Count}} facilities</span></h2>
        <p class="text-muted mt-1">{{.Description}}</p>
        <div class="table-container">
            <table class="sortable">
                <thead>
                    <tr>
                        <th class="sortable">Name</th>
                        <th class="sortable">Level</th>
                        <th class="sortable">Build Cost</th>
                        <th class="sortable">Labor</th>
                        <th class="sortable">Rent</th>
                        <th class="sortable">Recipe Output</th>
                    </tr>
                </thead>
                <tbody>
{{- range .Facilities}}
                    <tr>
                        <td>
                            <a href="{{dirName .ID}}/">{{.Name}}</a>
                            {{if .Buildable}}<span class="badge-buildable">buildable</span>{{end}}
                        </td>
                        <td data-sort="{{.Level}}">{{.Level}}</td>
                        <td data-sort="{{.BuildCost}}">{{fmtValue .BuildCost}}</td>
                        <td data-sort="{{.LaborCost}}">{{.LaborCost}}</td>
                        <td data-sort="{{.RentPerCycle}}">{{fmtValue .RentPerCycle}}</td>
                        <td>
{{- if .Recipe}}
                            <a href="../../recipes/{{dirName .Recipe.Category}}/{{dirName .Recipe.ID}}/">{{.Recipe.Name}}</a>
                            <span class="text-muted">&times;{{printf "%.2f" .RecipeMultiplier}}</span>
{{- else}}
                            <span class="text-muted">none</span>
{{- end}}
                        </td>
                    </tr>
{{- end}}
                </tbody>
            </table>
        </div>
    </main>
` + sortScript + themeScript + `
</body>
</html>
`
