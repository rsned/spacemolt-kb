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
