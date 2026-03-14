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
