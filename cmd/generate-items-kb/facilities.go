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

	"github.com/rsned/spacemolt-kb/pkg/bom"
)

// Facility represents a station-based building entity.
type Facility struct {
	// Identity
	ID          string
	Name        string
	Description string
	Category    string // production, service, faction, infrastructure, personal

	// Build requirements
	Level            int
	BuildCost        int
	BuildTime        int
	LaborCost        int
	RentPerCycle     int
	RecipeMultiplier float64

	// Upgrade chain
	UpgradesFrom     *string // facility ID
	UpgradesFromName *string
	UpgradesTo       *string // facility ID (derived from the inverse of UpgradesFrom)
	UpgradesToName   *string
	UpgradeChain     *UpgradeChain // Computed full upgrade path

	// Embedded data
	BuildMaterials      []MaterialRef
	MaintenancePerCycle []MaterialRef
	RecipeID            string // resolved into Recipe from the recipe catalog
	Recipe              *RecipeSummary
	BoM                 *bom.BoMResult

	// Operational stats (from the unified facility catalog, server v0.418.0+)
	AlwaysOn          bool
	PowerDraw         int
	PowerSupply       int
	LifeSupportDraw   int
	LifeSupportSupply int
	FuelCapacity      int
	FuelOutput        bool
	BatteryCapacity   int

	// Role / classification
	Empire              string
	Unique              bool
	ServiceType         string
	FactionServiceType  string
	PersonalServiceType string
	RequiresServiceType string
	FactionCap          int
	PersonalBonusType   string
	PersonalBonusValue  int
	ScanPower           int
	ScanFalloff         int
	FleetUpkeep         bool
	AllowsContraband    bool
	PirateBaseOnly      bool
	StationOrFactionOnly bool
	ExpansionOf         string
	ExpansionScale      float64
	Lore                string

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
	Category string `json:"-"` // populated from items data for linking
}

// UpgradeChainLink represents a single link in the upgrade chain.
type UpgradeChainLink struct {
	ID        string
	Name      string
	IsCurrent bool
}

// UpgradeChain represents the full upgrade path.
type UpgradeChain struct {
	Links []UpgradeChainLink
}

// RecipeSummary is a simplified recipe representation embedded in a facility.
type RecipeSummary struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Category     string        `json:"category"` // Required for linking to recipe pages
	CraftingTime float64       `json:"crafting_time"`
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

// facilityCatalogJSON is the unified facility catalog (catalog_facilities.json,
// server v0.418.0+): a single {"items":[...]} array of full facility records
// replacing the legacy per-facility facility_details/*.json dumps.
type facilityCatalogJSON struct {
	Items []facilityCatalogItem `json:"items"`
}

// facilityCatalogItem is one entry of the unified facility catalog. Unlike the
// legacy detail files it carries no embedded recipe (only recipe_id) and no
// upgrades_to (only upgrades_from); both are reconstructed downstream.
type facilityCatalogItem struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	Category          string        `json:"category"`
	Level             int           `json:"level"`
	BuildCost         int           `json:"build_cost"`
	BuildTime         int           `json:"build_time"`
	LaborCost         int           `json:"labor_cost"`
	BuildMaterials    []MaterialRef  `json:"build_materials"`
	MaintenanceInputs []MaterialRef  `json:"maintenance_inputs"`
	RecipeID          string         `json:"recipe_id"`
	UpgradesFrom      string         `json:"upgrades_from"`

	AlwaysOn          bool `json:"always_on"`
	PowerDraw         int  `json:"power_draw"`
	PowerSupply       int  `json:"power_supply"`
	LifeSupportDraw   int  `json:"life_support_draw"`
	LifeSupportSupply int  `json:"life_support_supply"`
	FuelCapacity      int  `json:"fuel_capacity"`
	FuelOutput        bool `json:"fuel_output"`
	BatteryCapacity   int  `json:"battery_capacity"`

	Empire               string  `json:"empire"`
	Unique               bool    `json:"unique"`
	ServiceType          string  `json:"service_type"`
	FactionServiceType   string  `json:"faction_service_type"`
	PersonalServiceType  string  `json:"personal_service_type"`
	RequiresServiceType  string  `json:"requires_service_type"`
	FactionCap           int     `json:"faction_cap"`
	PersonalBonusType    string  `json:"personal_bonus_type"`
	PersonalBonusValue   int     `json:"personal_bonus_value"`
	ScanPower            int     `json:"scan_power"`
	ScanFalloff          int     `json:"scan_falloff"`
	FleetUpkeep          bool    `json:"fleet_upkeep"`
	AllowsContraband     bool    `json:"allows_contraband"`
	PirateBaseOnly       bool    `json:"pirate_base_only"`
	StationOrFactionOnly bool    `json:"station_or_faction_only"`
	ExpansionOf          string  `json:"expansion_of"`
	ExpansionScale       float64 `json:"expansion_scale"`
	Lore                 string  `json:"lore"`

	SatisfiedDescription string `json:"satisfied_description"`
	DegradedDescription  string `json:"degraded_description"`
}

// loadFacilities loads the facility catalog for a snapshot directory, preferring
// the unified catalog_facilities.json (server v0.418.0+) and falling back to the
// legacy facility_details/ directory for older snapshots.
func loadFacilities(catalogDir string) (map[string]*Facility, error) {
	catalogPath := filepath.Join(catalogDir, "catalog_facilities.json")
	if data, err := os.ReadFile(catalogPath); err == nil {
		return parseFacilityCatalog(data)
	}
	return loadFacilitiesFromJSON(filepath.Join(catalogDir, "facility_details"))
}

// parseFacilityCatalog parses the unified facility catalog blob.
func parseFacilityCatalog(data []byte) (map[string]*Facility, error) {
	var cat facilityCatalogJSON
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("unmarshal facility catalog: %w", err)
	}

	facilities := make(map[string]*Facility, len(cat.Items))
	for i := range cat.Items {
		fac := convertCatalogItemToFacility(&cat.Items[i])
		facilities[fac.ID] = fac
	}
	return facilities, nil
}

// convertCatalogItemToFacility converts a unified-catalog item to a Facility.
func convertCatalogItemToFacility(raw *facilityCatalogItem) *Facility {
	fac := &Facility{
		ID:                  raw.ID,
		Name:                raw.Name,
		Description:         raw.Description,
		Category:            raw.Category,
		Level:               raw.Level,
		BuildCost:           raw.BuildCost,
		BuildTime:           raw.BuildTime,
		LaborCost:           raw.LaborCost,
		BuildMaterials:      raw.BuildMaterials,
		MaintenancePerCycle: raw.MaintenanceInputs,
		RecipeID:            raw.RecipeID,
		AlwaysOn:            raw.AlwaysOn,
		PowerDraw:           raw.PowerDraw,
		PowerSupply:         raw.PowerSupply,
		LifeSupportDraw:     raw.LifeSupportDraw,
		LifeSupportSupply:   raw.LifeSupportSupply,
		FuelCapacity:        raw.FuelCapacity,
		FuelOutput:          raw.FuelOutput,
		BatteryCapacity:     raw.BatteryCapacity,
		Empire:              raw.Empire,
		Unique:              raw.Unique,
		ServiceType:         raw.ServiceType,
		FactionServiceType:  raw.FactionServiceType,
		PersonalServiceType: raw.PersonalServiceType,
		RequiresServiceType: raw.RequiresServiceType,
		FactionCap:          raw.FactionCap,
		PersonalBonusType:   raw.PersonalBonusType,
		PersonalBonusValue:  raw.PersonalBonusValue,
		ScanPower:           raw.ScanPower,
		ScanFalloff:         raw.ScanFalloff,
		FleetUpkeep:         raw.FleetUpkeep,
		AllowsContraband:    raw.AllowsContraband,
		PirateBaseOnly:      raw.PirateBaseOnly,
		StationOrFactionOnly: raw.StationOrFactionOnly,
		ExpansionOf:         raw.ExpansionOf,
		ExpansionScale:      raw.ExpansionScale,
		Lore:                raw.Lore,
	}

	if raw.UpgradesFrom != "" {
		fac.UpgradesFrom = &raw.UpgradesFrom
	}
	if raw.SatisfiedDescription != "" {
		fac.SatisfiedDescription = &raw.SatisfiedDescription
	}
	if raw.DegradedDescription != "" {
		fac.DegradedDescription = &raw.DegradedDescription
	}

	return fac
}

// convertJSONToFacility converts raw JSON to Facility struct.
func convertJSONToFacility(raw *facilityJSON) *Facility {
	fac := &Facility{
		ID:               raw.TypeID,
		Name:             raw.Name,
		Description:      raw.Description,
		Category:         raw.Category,
		Level:            raw.Level,
		BuildCost:        raw.BuildCost,
		BuildTime:        raw.BuildTime,
		LaborCost:        raw.LaborCost,
		RentPerCycle:     raw.RentPerCycle,
		RecipeMultiplier: raw.RecipeMultiplier,
		BuildMaterials:   raw.BuildMaterials,
		MaintenancePerCycle: raw.MaintenancePerCycle,
		RecipeID:         raw.RecipeID,
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
	"production":     "Buildable facilities for crafting items, weapons, and equipment.",
	"service":        "Station facilities providing trade, storage, repair, and services.",
	"faction":        "Faction-specific buildings for diplomacy and warfare.",
	"infrastructure": "Facilities supporting power, fuel, and station operations.",
	"personal":       "Compact facilities for personal crafting and storage.",
}

// htmlFacilitiesTopTemplate is the template for kb/facilities/index.html
var htmlFacilitiesTopTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Facilities - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../facilities/facilities.css">
</head>
<body>
` + siteHeader + `
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
    <title>{{titleCase .Name}} - Facilities - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../facilities/facilities.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Facilities</a> / {{titleCase .Name}}</div>
        <h2>{{titleCase .Name}} <span class="text-muted">{{.Count}} facilities</span></h2>
        <p class="text-muted mt-1">{{.Description}}</p>
        <div class="table-container">
            <table class="sortable">
                <thead>
                    <tr>
                        <th class="sortable">Name</th>
                        <th class="sortable">Level</th>
                        <th class="sortable">Build Cost</th>
                        <th class="sortable">Labor</th>
                        <th class="sortable">Recipe Output</th>
                    </tr>
                </thead>
                <tbody>
{{- range .Facilities}}
                    <tr>
                        <td><a href="{{.ID}}.html">{{.Name}}</a></td>
                        <td data-sort="{{.Level}}">{{.Level}}</td>
                        <td data-sort="{{.BuildCost}}">{{fmtValue .BuildCost}}</td>
                        <td data-sort="{{.LaborCost}}">{{.LaborCost}}</td>
                        <td>
{{- if .Recipe}}
{{- if .Recipe.Category}}
                            <a href="../../recipes/{{dirName .Recipe.Category}}/{{.Recipe.ID}}.html">{{.Recipe.Name}}</a>
{{- else}}
                            {{.Recipe.Name}}
{{- end}}
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

// htmlFacilityDetailTemplate is the template for kb/facilities/{category}/{facility-id}.html
var htmlFacilityDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Facility.Name}} - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../facilities/facilities.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Facilities</a> / <a href="./">{{titleCase .Facility.Category}}</a> / {{.Facility.Name}}</div>

        <h2>{{.Facility.Name}}{{if .Facility.Empire}} <span class="badge {{empireBadgeClass .Facility.Empire}}">{{titleCase .Facility.Empire}}</span>{{end}}{{if .Facility.PirateBaseOnly}} <span class="badge badge-red">Pirate Base</span>{{end}}{{if .Facility.StationOrFactionOnly}} <span class="badge badge-orange">Faction Only</span>{{end}}{{if .Facility.Unique}} <span class="badge badge-rare">Unique</span>{{end}}{{if .Facility.AlwaysOn}} <span class="badge">Always On</span>{{end}}</h2>

        {{if .Facility.Description}}
        <blockquote class="item-desc">{{.Facility.Description}}</blockquote>
        {{end}}

        {{if .Facility.Lore}}
        <blockquote class="item-desc" style="font-style:italic">{{.Facility.Lore}}</blockquote>
        {{end}}

        <div class="card mt-2" style="padding:0">
            <div class="section-label">Stats</div>
            <table>
                <tr><td class="kv-label">Level</td><td>{{.Facility.Level}}</td></tr>
                {{if .Facility.Empire}}<tr><td class="kv-label">Empire</td><td>{{titleCase .Facility.Empire}}</td></tr>{{end}}
                <tr><td class="kv-label">Build Cost</td><td>{{fmtValue .Facility.BuildCost}}</td></tr>
                {{if gt .Facility.BuildTime 0}}<tr><td class="kv-label">Build Time</td><td>{{.Facility.BuildTime}}s</td></tr>{{end}}
                <tr><td class="kv-label">Labor</td><td>{{.Facility.LaborCost}}</td></tr>
                {{if gt .Facility.PowerDraw 0}}<tr><td class="kv-label">Power Draw</td><td>{{.Facility.PowerDraw}}</td></tr>{{end}}
                {{if gt .Facility.PowerSupply 0}}<tr><td class="kv-label">Power Supply</td><td>{{.Facility.PowerSupply}}</td></tr>{{end}}
                {{if gt .Facility.LifeSupportDraw 0}}<tr><td class="kv-label">Life Support Draw</td><td>{{.Facility.LifeSupportDraw}}</td></tr>{{end}}
                {{if gt .Facility.LifeSupportSupply 0}}<tr><td class="kv-label">Life Support Supply</td><td>{{.Facility.LifeSupportSupply}}</td></tr>{{end}}
                {{if gt .Facility.FuelCapacity 0}}<tr><td class="kv-label">Fuel Capacity</td><td>{{.Facility.FuelCapacity}}</td></tr>{{end}}
                {{if .Facility.FuelOutput}}<tr><td class="kv-label">Fuel Output</td><td>Yes</td></tr>{{end}}
                {{if gt .Facility.BatteryCapacity 0}}<tr><td class="kv-label">Battery Capacity</td><td>{{.Facility.BatteryCapacity}}</td></tr>{{end}}
            </table>
        </div>

        {{if facilityHasProperties .Facility}}
        <div class="card" style="padding:0">
            <div class="section-label">Properties</div>
            <table>
                {{if .Facility.ServiceType}}<tr><td class="kv-label">Service</td><td>{{titleCase .Facility.ServiceType}}</td></tr>{{end}}
                {{if .Facility.FactionServiceType}}<tr><td class="kv-label">Faction Service</td><td>{{titleCase .Facility.FactionServiceType}}</td></tr>{{end}}
                {{if .Facility.PersonalServiceType}}<tr><td class="kv-label">Personal Service</td><td>{{titleCase .Facility.PersonalServiceType}}</td></tr>{{end}}
                {{if .Facility.RequiresServiceType}}<tr><td class="kv-label">Requires Service</td><td>{{titleCase .Facility.RequiresServiceType}}</td></tr>{{end}}
                {{if .Facility.PersonalBonusType}}<tr><td class="kv-label">Personal Bonus</td><td>{{titleCase .Facility.PersonalBonusType}}{{if gt .Facility.PersonalBonusValue 0}} (+{{.Facility.PersonalBonusValue}}){{end}}</td></tr>{{end}}
                {{if gt .Facility.FactionCap 0}}<tr><td class="kv-label">Faction Cap</td><td>{{.Facility.FactionCap}}</td></tr>{{end}}
                {{if gt .Facility.ScanPower 0}}<tr><td class="kv-label">Scan Power</td><td>{{.Facility.ScanPower}}</td></tr>{{end}}
                {{if gt .Facility.ScanFalloff 0}}<tr><td class="kv-label">Scan Falloff</td><td>{{.Facility.ScanFalloff}}</td></tr>{{end}}
                {{if .Facility.FleetUpkeep}}<tr><td class="kv-label">Fleet Upkeep</td><td>Yes</td></tr>{{end}}
                {{if .Facility.AllowsContraband}}<tr><td class="kv-label">Allows Contraband</td><td>Yes</td></tr>{{end}}
                {{if .Facility.PirateBaseOnly}}<tr><td class="kv-label">Pirate Base Only</td><td>Yes</td></tr>{{end}}
                {{if .Facility.StationOrFactionOnly}}<tr><td class="kv-label">Station/Faction Only</td><td>Yes</td></tr>{{end}}
                {{if .Facility.ExpansionOf}}<tr><td class="kv-label">Expansion Of</td><td>{{titleCase .Facility.ExpansionOf}}{{if gt .Facility.ExpansionScale 0.0}} (&times;{{printf "%.2f" .Facility.ExpansionScale}}){{end}}</td></tr>{{end}}
            </table>
        </div>
        {{end}}

        {{if .Facility.UpgradeChain}}
        <div class="card" style="padding:0">
            <div class="section-label">Upgrade Path</div>
            <div class="upgrade-diagram" style="margin:0;padding:0.75rem">
            {{- upgradeSVG .Facility.UpgradeChain}}
            </div>
        </div>
        {{end}}

        {{if .Facility.Recipe}}
        <div class="card" style="padding:0">
            <div class="section-label">Recipe</div>
            <table>
                <tr><td class="kv-label">Output</td><td>{{if .Facility.Recipe.Category}}<a href="../../recipes/{{dirName .Facility.Recipe.Category}}/{{.Facility.Recipe.ID}}.html">{{.Facility.Recipe.Name}}</a>{{else}}{{.Facility.Recipe.Name}}{{end}}{{if gt .Facility.RecipeMultiplier 0.0}} <span class="text-muted">&times;{{printf "%.2f" .Facility.RecipeMultiplier}}</span>{{end}}</td></tr>
                <tr><td class="kv-label">Crafting Time</td><td>{{.Facility.Recipe.CraftingTime}}s</td></tr>
            </table>
            {{if .Facility.Recipe.Inputs}}
            <div class="section-label">Recipe Inputs</div>
            <table>
                <thead><tr><th>Item</th><th>Quantity</th></tr></thead>
                <tbody>
                {{- range .Facility.Recipe.Inputs}}
                    <tr><td>{{if .Category}}<a href="../../items/{{.Category}}/{{.ItemID}}.html">{{.Name}}</a>{{else}}{{.Name}}{{end}}</td><td>{{.Quantity}}</td></tr>
                {{- end}}
                </tbody>
            </table>
            {{end}}
            {{if .Facility.Recipe.Outputs}}
            <div class="section-label">Recipe Outputs</div>
            <table>
                <thead><tr><th>Item</th><th>Quantity</th></tr></thead>
                <tbody>
                {{- range .Facility.Recipe.Outputs}}
                    <tr><td>{{if .Category}}<a href="../../items/{{.Category}}/{{.ItemID}}.html">{{.Name}}</a>{{else}}{{.Name}}{{end}}</td><td>{{.Quantity}}</td></tr>
                {{- end}}
                </tbody>
            </table>
            {{end}}
        </div>
        {{end}}

        {{if .Facility.BuildMaterials}}
        <div class="card" style="padding:0">
            <div class="section-label">Build Materials</div>
            <table>
                <thead><tr><th>Material</th><th>Quantity</th></tr></thead>
                <tbody>
                {{- range .Facility.BuildMaterials}}
                    <tr><td>{{if .Category}}<a href="../../items/{{.Category}}/{{.ItemID}}.html">{{.Name}}</a>{{else}}{{.Name}}{{end}}</td><td>{{.Quantity}}</td></tr>
                {{- end}}
                </tbody>
            </table>
        </div>
        {{end}}

        {{if .Facility.MaintenancePerCycle}}
        <div class="card" style="padding:0">
            <div class="section-label">Maintenance per Cycle</div>
            <table>
                <thead><tr><th>Material</th><th>Quantity</th></tr></thead>
                <tbody>
                {{- range .Facility.MaintenancePerCycle}}
                    <tr><td>{{if .Category}}<a href="../../items/{{.Category}}/{{.ItemID}}.html">{{.Name}}</a>{{else}}{{.Name}}{{end}}</td><td>{{.Quantity}}</td></tr>
                {{- end}}
                </tbody>
            </table>
        </div>
        {{end}}

        {{if .Facility.Hint}}
        <div class="card" style="padding:0">
            <div class="section-label">Hint</div>
            <p style="margin:0;padding:0.75rem;color:hsl(var(--muted-foreground))">{{.Facility.Hint}}</p>
        </div>
        {{end}}

        {{if hasBoM .Facility.BoM}}
        {{boMTable .Facility.BoM}}
        <details class="bom-json-details">
          <summary>View JSON Data</summary>
          <pre class="bom-json">{{boMJSON .Facility.BoM}}</pre>
        </details>
        {{end}}
    </main>
` + sortScript + themeScript + `
</body>
</html>
`

// writeFacilityPages generates all facility HTML pages.
func writeFacilityPages(outDir string, facilities map[string]*Facility, items map[string]*Item) error {
	// Build upgrade chains for all facilities
	buildUpgradeChains(facilities)

	// Populate item categories on all MaterialRefs for correct linking
	populateMaterialCategories(facilities, items)

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
		"fmtValue":               fmtValue,
		"titleCase":              titleCase,
		"dirName":                dirName,
		"upgradeSVG":             generateUpgradeSVG,
		"facilityHasProperties":  facilityHasProperties,
		"empireBadgeClass":       empireBadgeClass,
		"hasBoM": func(b *bom.BoMResult) bool {
			return b != nil && len(b.BaseMaterials) > 0
		},
		"boMJSON": func(b *bom.BoMResult) string { return b.JSON() },
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
					sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, mat.ItemID, mat.Quantity))
					continue
				}

				if item.Category == "" {
					sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, item.Name, mat.Quantity))
					continue
				}

				sb.WriteString(fmt.Sprintf(`<tr><td><a href="../../items/%s/%s.html">%s</a></td><td>%d</td></tr>`,
					item.Category, mat.ItemID, item.Name, mat.Quantity))
			}

			sb.WriteString(`</tbody></table></div>`)
			sb.WriteString(`</div>`)
			return htmltpl.HTML(sb.String())
		},
	}

	// Parse templates
	topTmpl := htmltpl.Must(htmltpl.New("top").Funcs(funcs).Parse(htmlFacilitiesTopTemplate))
	catTmpl := htmltpl.Must(htmltpl.New("cat").Funcs(funcs).Parse(htmlFacilitiesCategoryTemplate))
	facTmpl := htmltpl.Must(htmltpl.New("fac").Funcs(funcs).Parse(htmlFacilityDetailTemplate))

	// Create output directory
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Write main index (using writeTemplate helper from skills.go)
	if err := writeTemplate(filepath.Join(outDir, "index.html"), topTmpl, categories); err != nil {
		return err
	}

	// Write category indexes and facility pages
	for _, cat := range categories {
		catDir := filepath.Join(outDir, cat.Name)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			return err
		}

		// Category index
		if err := writeTemplate(filepath.Join(catDir, "index.html"), catTmpl, cat); err != nil {
			return err
		}

		// Individual facility pages
		for _, fac := range cat.Facilities {
			facPath := filepath.Join(catDir, fac.ID+".html")
			// Wrap facility with items for template functions
			type facilityDetailData struct {
				Facility *Facility
				Items    map[string]*Item
			}
			if err := writeTemplate(facPath, facTmpl, facilityDetailData{
				Facility: fac,
				Items:    items,
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

// resolveFacilityRecipes attaches a RecipeSummary to each facility, keyed by
// RecipeID. The unified facility catalog no longer embeds recipe input/output
// detail (only recipe_id), so recipes are resolved from, in order: the crafting
// recipe DB (authoritative, has a recipe page to link to), then the snapshot's
// catalog_recipes.json (fallback used when the separately-maintained crafting DB
// lags the scrape — rendered as plain text since no recipe page exists). Recipe
// IDs absent from both are genuine server-side gaps and warned about. Legacy
// snapshots that still embed a recipe keep it; the category is backfilled.
func resolveFacilityRecipes(facilities map[string]*Facility, recipes map[string]*Recipe, catalogRecipes map[string]*RecipeSummary) {
	for _, fac := range facilities {
		if fac.RecipeID == "" {
			continue
		}
		craft, inCraft := recipes[fac.RecipeID]
		switch {
		case fac.Recipe != nil:
			// Legacy embedded recipe: backfill category for linking if known.
			if inCraft && fac.Recipe.Category == "" {
				fac.Recipe.Category = craft.Category
			}
		case inCraft:
			fac.Recipe = recipeToSummary(craft)
		default:
			if sum, ok := catalogRecipes[fac.RecipeID]; ok {
				fac.Recipe = cloneRecipeSummary(sum)
			} else {
				log.Printf("warning: facility %s references unknown recipe %s", fac.ID, fac.RecipeID)
			}
		}
	}
}

// cloneRecipeSummary returns a deep copy of a RecipeSummary so facilities that
// share a fallback recipe do not alias each other's MaterialRef slices (which
// are mutated in place by populateMaterialCategories).
func cloneRecipeSummary(s *RecipeSummary) *RecipeSummary {
	out := *s
	out.Inputs = append([]MaterialRef(nil), s.Inputs...)
	out.Outputs = append([]MaterialRef(nil), s.Outputs...)
	return &out
}

// catalogRecipeJSON mirrors catalog_recipes.json, used only as a fallback source
// of facility recipe detail. Input/output entries carry item_id + quantity but
// no item name; names are backfilled from the items catalog downstream.
type catalogRecipeJSON struct {
	Items []struct {
		ID           string        `json:"id"`
		Name         string        `json:"name"`
		Category     string        `json:"category"`
		CraftingTime float64       `json:"crafting_time"`
		Inputs       []MaterialRef `json:"inputs"`
		Outputs      []MaterialRef `json:"outputs"`
	} `json:"items"`
}

// loadCatalogRecipeSummaries loads recipe summaries from the snapshot's
// catalog_recipes.json for use as a facility-recipe fallback. Category is left
// blank so callers render the recipe name as plain text rather than linking to a
// recipe page that the crafting DB (the recipe-page source) did not generate.
func loadCatalogRecipeSummaries(path string) (map[string]*RecipeSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cat catalogRecipeJSON
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("unmarshal catalog recipes: %w", err)
	}
	out := make(map[string]*RecipeSummary, len(cat.Items))
	for i := range cat.Items {
		r := &cat.Items[i]
		out[r.ID] = &RecipeSummary{
			ID:           r.ID,
			Name:         r.Name,
			CraftingTime: r.CraftingTime,
			Inputs:       r.Inputs,
			Outputs:      r.Outputs,
		}
	}
	return out, nil
}

// recipeToSummary builds the facility-embedded RecipeSummary view from a full
// recipe catalog entry.
func recipeToSummary(r *Recipe) *RecipeSummary {
	sum := &RecipeSummary{
		ID:           r.ID,
		Name:         r.Name,
		Category:     r.Category,
		CraftingTime: r.CraftingTime,
		Inputs:       make([]MaterialRef, len(r.Inputs)),
		Outputs:      make([]MaterialRef, len(r.Outputs)),
	}
	for i, in := range r.Inputs {
		sum.Inputs[i] = MaterialRef{ItemID: in.ItemID, Name: in.ItemName, Quantity: in.Quantity}
	}
	for i, out := range r.Outputs {
		sum.Outputs[i] = MaterialRef{ItemID: out.ItemID, Name: out.ItemName, Quantity: out.Quantity}
	}
	return sum
}

// facilityHasProperties reports whether a facility has any classification or
// role fields worth rendering in the Properties card.
func facilityHasProperties(f *Facility) bool {
	return f.ServiceType != "" || f.FactionServiceType != "" || f.PersonalServiceType != "" ||
		f.RequiresServiceType != "" || f.PersonalBonusType != "" || f.FactionCap > 0 ||
		f.ScanPower > 0 || f.ScanFalloff > 0 || f.FleetUpkeep || f.AllowsContraband ||
		f.PirateBaseOnly || f.StationOrFactionOnly || f.ExpansionOf != ""
}

// empireBadgeClass maps a facility's owning empire/faction to a badge color
// class from smui.css, so the empire badge reads at a glance. Unknown empires
// fall back to purple. The two spellings outer_rim/outerrim in the data share a
// color. (Distinct from ships.go's factionBadgeClass, which maps ship faction
// tags with a different value set and palette.)
func empireBadgeClass(empire string) string {
	switch empire {
	case "pirates":
		return "badge-red"
	case "crimson_pact":
		return "badge-orange"
	case "solarian":
		return "badge-yellow"
	case "nebula", "nebula_trade":
		return "badge-frost"
	case "outer_rim", "outerrim":
		return "badge-green"
	case "voidborn":
		return "badge-purple"
	default:
		return "badge-purple"
	}
}

// populateBuiltByFacility adds a BuiltBy entry on each item for every facility
// whose embedded recipe produces it as an output.
func populateBuiltByFacility(items map[string]*Item, facilities map[string]*Facility) {
	for _, fac := range facilities {
		if fac.Recipe == nil {
			continue
		}
		for _, out := range fac.Recipe.Outputs {
			it, ok := items[out.ItemID]
			if !ok {
				continue
			}
			it.BuiltBy = append(it.BuiltBy, BuiltByFacility{
				FacilityID:       fac.ID,
				FacilityName:     fac.Name,
				FacilityCategory: fac.Category,
				FacilityLevel:    fac.Level,
				RecipeID:         fac.Recipe.ID,
				RecipeName:       fac.Recipe.Name,
				RecipeCategory:   fac.Recipe.Category,
				Quantity:         out.Quantity,
				CraftingTime:     fac.Recipe.CraftingTime,
				RecipeMultiplier: fac.RecipeMultiplier,
			})
		}
	}
	for _, it := range items {
		if len(it.BuiltBy) == 0 {
			continue
		}
		slices.SortFunc(it.BuiltBy, func(a, b BuiltByFacility) int {
			if c := cmp.Compare(a.FacilityLevel, b.FacilityLevel); c != 0 {
				return c
			}
			return cmp.Compare(a.FacilityName, b.FacilityName)
		})
	}
}

// populateMaterialCategories resolves item categories and names on all
// MaterialRefs for correct HTML linking and display. The unified facility
// catalog omits item names on build materials, maintenance inputs, and recipe
// inputs/outputs, so names are backfilled from the items catalog here.
func populateMaterialCategories(facilities map[string]*Facility, items map[string]*Item) {
	for _, fac := range facilities {
		populateMaterialRefs(fac.BuildMaterials, items)
		populateMaterialRefs(fac.MaintenancePerCycle, items)
		if fac.Recipe != nil {
			populateMaterialRefs(fac.Recipe.Inputs, items)
			populateMaterialRefs(fac.Recipe.Outputs, items)
		}
	}
}

// populateMaterialRefs backfills the Category (for linking) and Name (for
// display) of each MaterialRef from the items catalog, falling back to the item
// ID when the item is unknown so cells are never blank.
func populateMaterialRefs(refs []MaterialRef, items map[string]*Item) {
	for i := range refs {
		if item, ok := items[refs[i].ItemID]; ok {
			refs[i].Category = item.Category
			if refs[i].Name == "" {
				refs[i].Name = item.Name
			}
		}
		if refs[i].Name == "" {
			refs[i].Name = refs[i].ItemID
		}
	}
}

// buildUpgradeChains populates the UpgradeChain field for all facilities.
func buildUpgradeChains(facilities map[string]*Facility) {
	// The unified catalog provides only upgrades_from; reconstruct the forward
	// links (upgrades_to) by inverting it so chains render in both directions.
	for _, fac := range facilities {
		if fac.UpgradesFrom == nil || *fac.UpgradesFrom == "" {
			continue
		}
		if prev, ok := facilities[*fac.UpgradesFrom]; ok {
			id := fac.ID
			prev.UpgradesTo = &id
		}
	}

	for _, fac := range facilities {
		chain := &UpgradeChain{Links: []UpgradeChainLink{}}

		// Walk backwards to find all previous upgrades
		backLinks := []UpgradeChainLink{}
		current := fac
		for current.UpgradesFrom != nil && *current.UpgradesFrom != "" {
			prevFac, exists := facilities[*current.UpgradesFrom]
			if !exists {
				break
			}
			backLinks = append([]UpgradeChainLink{{
				ID:   prevFac.ID,
				Name: prevFac.Name,
			}}, backLinks...)
			current = prevFac
		}

		// Add current facility
		chain.Links = append(chain.Links, backLinks...)
		chain.Links = append(chain.Links, UpgradeChainLink{
			ID:        fac.ID,
			Name:      fac.Name,
			IsCurrent: true,
		})

		// Walk forwards to find all next upgrades
		current = fac
		for current.UpgradesTo != nil && *current.UpgradesTo != "" {
			nextFac, exists := facilities[*current.UpgradesTo]
			if !exists {
				break
			}
			chain.Links = append(chain.Links, UpgradeChainLink{
				ID:   nextFac.ID,
				Name: nextFac.Name,
			})
			current = nextFac
		}

		fac.UpgradeChain = chain
	}
}

// generateUpgradeSVG creates an SVG diagram showing the upgrade path.
func generateUpgradeSVG(chain *UpgradeChain) htmltpl.HTML {
	if len(chain.Links) == 0 {
		return ""
	}

	const (
		boxWidth      = 140
		boxHeight     = 60
		boxSpacing    = 40
		arrowLength   = 30
		startPadding  = 20
		vertCenter    = 45
		fontSize      = 11
		lineHeight    = 14
	)

	numLinks := len(chain.Links)
	totalWidth := startPadding*2 + (numLinks-1)*(boxWidth+boxSpacing+arrowLength) + boxWidth

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg width="100%%" height="%d" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`, boxHeight+20, totalWidth, boxHeight+20))
	sb.WriteString(`<style>`)
	sb.WriteString(`.upgrade-box { fill: transparent; stroke: #999; stroke-width: 1.5; }`)
	sb.WriteString(`.upgrade-box-current { fill: transparent; stroke: #000; stroke-width: 2; }`)
	sb.WriteString(`.upgrade-text { font-family: system-ui, -apple-system, sans-serif; font-size: 12px; fill: #666; text-anchor: middle; }`)
	sb.WriteString(`.upgrade-text-current { font-family: system-ui, -apple-system, sans-serif; font-size: 12px; fill: #000; font-weight: 600; text-anchor: middle; }`)
	sb.WriteString(`.upgrade-link { text-decoration: none; }`)
	sb.WriteString(`.upgrade-arrow { stroke: #999; stroke-width: 1.5; fill: none; }`)
	sb.WriteString(`</style>`)

	for i, link := range chain.Links {
		x := startPadding + i*(boxWidth+boxSpacing+arrowLength)
		y := vertCenter - boxHeight/2

		// Draw arrow from previous box (except for first item)
		if i > 0 {
			prevX := startPadding + (i-1)*(boxWidth+boxSpacing+arrowLength) + boxWidth
			arrowStartX := prevX
			arrowEndX := x - 5
			midY := vertCenter
			sb.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" class="upgrade-arrow"/>`, arrowStartX, midY, arrowEndX, midY))
			// Arrowhead
			sb.WriteString(fmt.Sprintf(`<polygon points="%d,%d %d,%d %d,%d" fill="#999"/>`, arrowEndX+5, midY, arrowEndX, midY-4, arrowEndX, midY+4))
		}

		// Draw box
		boxClass := "upgrade-box"
		textClass := "upgrade-text"
		if link.IsCurrent {
			boxClass = "upgrade-box-current"
			textClass = "upgrade-text-current"
		}

		if link.IsCurrent {
			sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="4" class="%s"/>`, x, y, boxWidth, boxHeight, boxClass))
		} else {
			sb.WriteString(fmt.Sprintf(`<a href="%s.html" class="upgrade-link">`, link.ID))
			sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="4" class="%s"/>`, x, y, boxWidth, boxHeight, boxClass))
			sb.WriteString(`</a>`)
		}

		// Draw text with line wrapping
		words := strings.Fields(link.Name)
		var lines []string
		currentLine := ""
		for _, word := range words {
			testLine := currentLine
			if testLine == "" {
				testLine = word
			} else {
				testLine += " " + word
			}
			// Rough character width estimation
			if len(testLine) > 18 && currentLine != "" {
				lines = append(lines, currentLine)
				currentLine = word
			} else {
				currentLine = testLine
			}
		}
		if currentLine != "" {
			lines = append(lines, currentLine)
		}

		// Calculate vertical position to center text
		totalTextHeight := len(lines) * lineHeight
		startY := vertCenter - totalTextHeight/2 + lineHeight/3

		for lineIdx, line := range lines {
			textY := startY + lineIdx*lineHeight
			if link.IsCurrent {
				sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" class="%s">%s</text>`, x+boxWidth/2, textY, textClass, line))
			} else {
				sb.WriteString(fmt.Sprintf(`<a href="%s.html" class="upgrade-link">`, link.ID))
				sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" class="%s">%s</text>`, x+boxWidth/2, textY, textClass, line))
				sb.WriteString(`</a>`)
			}
		}
	}

	sb.WriteString("</svg>")
	return htmltpl.HTML(sb.String())
}


