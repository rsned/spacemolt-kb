package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/shipglyph"
)

// catalogShip is the subset of catalog_ships.json needed to build a glyph.
type catalogShip struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Class         string `json:"class"`
	Category      string `json:"category"`
	Faction       string `json:"faction"`
	Tier          int    `json:"tier"`
	Scale         int    `json:"scale"`
	WeaponSlots   int    `json:"weapon_slots"`
	DefenseSlots  int    `json:"defense_slots"`
	UtilitySlots  int    `json:"utility_slots"`
	CargoCapacity int    `json:"cargo_capacity"`
	Lore          string `json:"lore"`
}

// loadShipCatalog reads the {"items": [...]} catalog produced by the game API.
func loadShipCatalog(path string) ([]catalogShip, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ship catalog: %w", err)
	}
	var catalog struct {
		Items []catalogShip `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse ship catalog: %w", err)
	}
	return catalog.Items, nil
}

// validateCatalog checks the catalog for problems that would produce broken
// output: an empty id would write a file literally named ".svg", and two
// ships sharing an id would silently overwrite each other. It reports every
// offending id at once rather than failing on the first.
func validateCatalog(ships []catalogShip) error {
	var empty []string
	byID := make(map[string][]string)
	for _, c := range ships {
		if c.ID == "" {
			empty = append(empty, c.Name)
			continue
		}
		byID[c.ID] = append(byID[c.ID], c.Name)
	}
	if len(empty) > 0 {
		slices.Sort(empty)
		return fmt.Errorf("catalog has %d ship(s) with an empty id: %v", len(empty), empty)
	}

	var dupes []string
	for id, names := range byID {
		if len(names) > 1 {
			slices.Sort(names)
			dupes = append(dupes, fmt.Sprintf("%s %v", id, names))
		}
	}
	if len(dupes) > 0 {
		slices.Sort(dupes)
		return fmt.Errorf("catalog has duplicate ship ids: %s", strings.Join(dupes, ", "))
	}
	return nil
}

// toStats projects a catalog ship onto the shape-inference input.
func toStats(c catalogShip) shipglyph.Stats {
	return shipglyph.Stats{
		ID:       c.ID,
		Name:     c.Name,
		Class:    c.Class,
		Category: c.Category,
		Faction:  c.Faction,
		Tier:     c.Tier,
		Scale:    c.Scale,
		Weapon:   c.WeaponSlots,
		Defense:  c.DefenseSlots,
		Utility:  c.UtilitySlots,
		Cargo:    c.CargoCapacity,
	}
}
