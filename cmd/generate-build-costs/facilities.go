// Package-level file for the facility build-cost section.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// galaxyBook pools every station's sell ladder into a single ascending order
// book — the "cheapest sourcing anywhere" reference the galaxy price walks. It
// reuses pooledBook with the full station set. BestBuy is irrelevant here.
func galaxyBook(books map[string]*buildcost.Book) *buildcost.Book {
	ids := make([]string, 0, len(books))
	for id := range books {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return pooledBook(books, ids)
}

// fmtMoney formats a value with thousands separators and two decimals
// (e.g. 28762.9 → "28,762.90"). Reuses commaInt for the integer part.
func fmtMoney(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := math.Floor(v)
	frac := int64(math.Round((v - whole) * 100))
	if frac >= 100 {
		whole++
		frac -= 100
	}
	s := fmt.Sprintf("%s.%02d", commaInt(whole), frac)
	if neg {
		return "-" + s
	}
	return s
}

// FacilityRec is the minimal facility shape the build-cost pages need: identity,
// category, level, its production recipe id (used only for grouping), and the
// direct build_materials that construct it (the Recipe view).
type FacilityRec struct {
	ID       string
	Name     string
	Category string
	Level    int
	RecipeID string
	Build    []buildcost.Requirement
}

// facilityCatDoc / facilityCatItem mirror the fields of catalog_facilities.json
// that the build-cost pages consume.
type facilityCatDoc struct {
	Items []facilityCatItem `json:"items"`
}

type facilityCatItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Level          int    `json:"level"`
	RecipeID       string `json:"recipe_id"`
	BuildMaterials []struct {
		ItemID   string  `json:"item_id"`
		Quantity float64 `json:"quantity"`
	} `json:"build_materials"`
}

// loadFacilityCatalog reads catalog_facilities.json from the newest snapshot dir
// under root and returns the trimmed facility records.
func loadFacilityCatalog(root string) ([]FacilityRec, error) {
	dir, err := findLatestCatalogDir(root)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "catalog_facilities.json"))
	if err != nil {
		return nil, err
	}
	var doc facilityCatDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make([]FacilityRec, 0, len(doc.Items))
	for _, it := range doc.Items {
		rec := FacilityRec{ID: it.ID, Name: it.Name, Category: it.Category, Level: it.Level, RecipeID: it.RecipeID}
		for _, m := range it.BuildMaterials {
			rec.Build = append(rec.Build, buildcost.Requirement{ItemID: m.ItemID, Qty: m.Quantity})
		}
		out = append(out, rec)
	}
	return out, nil
}
