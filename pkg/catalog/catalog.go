// Package catalog reads the game-API snapshot catalogs (ships, facilities)
// that the KB generators build pages from. Snapshot directories live under a
// root such as data/snapshots/ and are selected by modification time, because
// the root also holds non-dated latest/ and previous/ directories.
package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Material is one entry of a ship's or facility's build_materials list.
// Quantity is float64 because facility quantities are floats in the source
// JSON; callers that need integers convert at their own boundary.
type Material struct {
	ItemID   string  `json:"item_id"`
	Quantity float64 `json:"quantity"`
}

// Ship is the subset of catalog_ships.json the KB generators consume.
type Ship struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Class          string     `json:"class"`
	Price          int        `json:"price"`
	BuildMaterials []Material `json:"build_materials"`
}

// Facility is the subset of catalog_facilities.json the KB generators consume.
type Facility struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Category       string     `json:"category"`
	Level          int        `json:"level"`
	RecipeID       string     `json:"recipe_id"`
	BuildMaterials []Material `json:"build_materials"`

	// Cost, upkeep and station-grid fields, used by the guides' build steps.
	// MaintenanceInputs is consumed every cycle (100 ticks); MaintenanceFuel
	// is fuel consumed per cycle, not an item.
	Description       string     `json:"description"`
	BuildCost         int        `json:"build_cost"`
	BuildTime         int        `json:"build_time"`
	MaintenanceInputs []Material `json:"maintenance_inputs"`
	MaintenanceFuel   int        `json:"maintenance_fuel"`
	PowerSupply       int        `json:"power_supply"`
	PowerDraw         int        `json:"power_draw"`
	LifeSupportSupply int        `json:"life_support_supply"`
	LifeSupportDraw   int        `json:"life_support_draw"`
	ServiceType       string     `json:"service_type"`

	// Service facilities with no maintenance_inputs can still have upkeep: a
	// pool of service units refilled each cycle from ServicePoolSupplyItem,
	// ServicePoolUnitsPerItem units per item.
	ServicePoolCapacity       int    `json:"service_pool_capacity"`
	ServicePoolRefillPerCycle int    `json:"service_pool_refill_per_cycle"`
	ServicePoolSupplyItem     string `json:"service_pool_supply_item"`
	ServicePoolUnitsPerItem   int    `json:"service_pool_units_per_item"`
}

// Item is the subset of catalog_items.json needed to link to an item's KB page.
type Item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// LoadItems reads catalog_items.json from the latest snapshot under root.
func LoadItems(root string) ([]Item, error) {
	var doc struct {
		Items []Item `json:"items"`
	}
	if err := loadCatalog(root, "catalog_items.json", &doc); err != nil {
		return nil, err
	}
	return doc.Items, nil
}

// FindLatestDir returns the most recently modified subdirectory of root.
func FindLatestDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var best string
	var bestMod int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mt := info.ModTime().UnixNano(); mt > bestMod {
			bestMod, best = mt, filepath.Join(root, e.Name())
		}
	}
	if best == "" {
		return "", os.ErrNotExist
	}
	return best, nil
}

// LoadShips reads catalog_ships.json from the newest snapshot under root.
func LoadShips(root string) ([]Ship, error) {
	var doc struct {
		Items []Ship `json:"items"`
	}
	if err := loadCatalog(root, "catalog_ships.json", &doc); err != nil {
		return nil, err
	}
	return doc.Items, nil
}

// LoadFacilities reads catalog_facilities.json from the newest snapshot under root.
func LoadFacilities(root string) ([]Facility, error) {
	var doc struct {
		Items []Facility `json:"items"`
	}
	if err := loadCatalog(root, "catalog_facilities.json", &doc); err != nil {
		return nil, err
	}
	return doc.Items, nil
}

func loadCatalog(root, name string, dst any) error {
	dir, err := FindLatestDir(root)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
