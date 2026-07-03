package gamediff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FacilityCatalogFile is the unified facility catalog produced by the game
// server's catalog endpoint (type=facilities), available from server v0.409.0.
// When present in a snapshot it is used directly; older snapshots are assembled
// from the per-category files in FacilityCategoryFiles.
const FacilityCatalogFile = "catalog_facilities.json"

// FacilityCategoryFiles are the per-category facility dumps emitted by scrapes
// that predate the unified facility catalog. Each has a top-level
// {"types":[...]} array; facility IDs are unique across all of them.
var FacilityCategoryFiles = []string{
	"facility_production.json",
	"facility_service.json",
	"facility_faction.json",
	"facility_infrastructure.json",
	"facility_personal.json",
}

// LoadFacilities returns a normalized facility catalog blob ({"items":[...]})
// for snapshot directory dir, suitable for DiffCatalog. It prefers the unified
// FacilityCatalogFile when present, otherwise merges the per-category
// facility_*.json files into a single item array.
func LoadFacilities(dir string) ([]byte, error) {
	if data, err := os.ReadFile(filepath.Join(dir, FacilityCatalogFile)); err == nil {
		return data, nil
	}

	var items []any
	found := false
	for _, name := range FacilityCategoryFiles {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		found = true
		var top map[string]any
		if err := json.Unmarshal(data, &top); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		arr, ok := top["types"].([]any)
		if !ok {
			return nil, fmt.Errorf("%s: field %q is not an array", name, "types")
		}
		items = append(items, arr...)
	}
	if !found {
		return nil, fmt.Errorf("no facility data in %s", dir)
	}

	out, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return nil, err
	}
	return out, nil
}
