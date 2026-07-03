package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/bom"
)

// Item is the minimal item record we need: identity, category, and human name.
type Item struct {
	ID       string
	Name     string
	Category string
}

// IsBase reports whether the item is a terminal BoM leaf (raw ore or exotic material).
func (it *Item) IsBase() bool {
	return it.Category == "ore" || it.Category == "material"
}

// Recipe links inputs to outputs.
type Recipe struct {
	ID      string
	Name    string
	Inputs  []RecipeRef
	Outputs []RecipeRef
}

// RecipeRef is a single input or output of a recipe.
type RecipeRef struct {
	ItemID   string
	Quantity int
}

// Ship is the minimal ship record we need for BoM analysis.
type Ship struct {
	ID             string
	Name           string
	Category       string
	BuildMaterials []RecipeRef
}

// Product is anything we compute a BoM for and report on.
type Product struct {
	ID            string
	Name          string
	Kind          string // "item" or "ship"
	Category      string // item category for items, ship category for ships
	BaseMaterials []RecipeRef
	// DirectInputs is set only for ships (their build_materials list, which
	// references intermediate items by ID). Items reach their direct inputs
	// via the recipe lookup, so this stays nil for them.
	DirectInputs []RecipeRef
}

// EmpireResources captures presence and richness per base resource for one empire.
type EmpireResources struct {
	// Present is the set of resource IDs where at least one POI in this empire
	// has any richness.
	Present map[string]struct{}
	// Richness sums POI richness across all this empire's POIs, per resource ID.
	Richness map[string]float64
	// POICount counts distinct POIs per resource ID (informational column).
	POICount map[string]int
}

// findLatestCatalogDir mirrors generate-items-kb's helper: picks the most
// recent YYYYMMDD snapshot directory under base.
func findLatestCatalogDir(base string) string {
	entries, err := os.ReadDir(base)
	if err != nil {
		log.Printf("warning: cannot read catalog base dir %s: %v (using as-is)", base, err)
		return base
	}
	var latest string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) == 8 && name > latest {
			latest = name
		}
	}
	if latest == "" {
		return base
	}
	return filepath.Join(base, latest)
}

// loadItems reads the items table from crafting.db.
func loadItems(db *sql.DB) (map[string]*Item, error) {
	rows, err := db.Query(`SELECT id, name, COALESCE(category,'') FROM items`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]*Item)
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Category); err != nil {
			return nil, err
		}
		out[it.ID] = &it
	}
	return out, rows.Err()
}

// loadRecipes reads recipes plus their inputs and outputs.
func loadRecipes(db *sql.DB) (map[string]*Recipe, error) {
	rows, err := db.Query(`SELECT id, name FROM recipes`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]*Recipe)
	for rows.Next() {
		var r Recipe
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		out[r.ID] = &r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	inRows, err := db.Query(`SELECT recipe_id, item_id, quantity FROM recipe_inputs`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = inRows.Close() }()
	for inRows.Next() {
		var rid string
		var ref RecipeRef
		if err := inRows.Scan(&rid, &ref.ItemID, &ref.Quantity); err != nil {
			return nil, err
		}
		if r, ok := out[rid]; ok {
			r.Inputs = append(r.Inputs, ref)
		}
	}
	if err := inRows.Err(); err != nil {
		return nil, err
	}

	outRows, err := db.Query(`SELECT recipe_id, item_id, quantity FROM recipe_outputs`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = outRows.Close() }()
	for outRows.Next() {
		var rid string
		var ref RecipeRef
		if err := outRows.Scan(&rid, &ref.ItemID, &ref.Quantity); err != nil {
			return nil, err
		}
		if r, ok := out[rid]; ok {
			r.Outputs = append(r.Outputs, ref)
		}
	}
	return out, outRows.Err()
}

// loadShips reads the ship catalog JSON.
func loadShips(path string) ([]*Ship, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ship catalog %s: %w", path, err)
	}
	var catalog struct {
		Items []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Category       string `json:"category"`
			BuildMaterials []struct {
				ItemID   string `json:"item_id"`
				Quantity int    `json:"quantity"`
			} `json:"build_materials"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("unmarshal ship catalog: %w", err)
	}
	out := make([]*Ship, 0, len(catalog.Items))
	for _, s := range catalog.Items {
		ship := &Ship{ID: s.ID, Name: s.Name, Category: s.Category}
		for _, m := range s.BuildMaterials {
			ship.BuildMaterials = append(ship.BuildMaterials, RecipeRef{ItemID: m.ItemID, Quantity: m.Quantity})
		}
		out = append(out, ship)
	}
	return out, nil
}

// loadEmpireResources walks the knowledge DB and returns one EmpireResources
// per empire in the empires list, plus the galaxy-wide total richness per
// resource ID. Unaligned-system POIs are excluded.
func loadEmpireResources(db *sql.DB) (map[string]*EmpireResources, map[string]float64, error) {
	empMap := make(map[string]*EmpireResources, len(empires))
	for _, e := range empires {
		empMap[e] = &EmpireResources{
			Present:  make(map[string]struct{}),
			Richness: make(map[string]float64),
			POICount: make(map[string]int),
		}
	}

	rows, err := db.Query(`
		SELECT s.empire, pr.resource_id, pr.richness
		FROM poi_resources pr
		JOIN pois p ON pr.poi_id = p.id
		JOIN systems s ON p.system_id = s.id
		WHERE s.empire != ''`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	galaxy := make(map[string]float64)
	for rows.Next() {
		var empire, resID string
		var richness float64
		if err := rows.Scan(&empire, &resID, &richness); err != nil {
			return nil, nil, err
		}
		// Galaxy total includes every empire, including those outside our list,
		// so an empire's share is always relative to the full named-empire pool.
		galaxy[resID] += richness

		er, ok := empMap[empire]
		if !ok {
			continue
		}
		if richness > 0 {
			er.Present[resID] = struct{}{}
		}
		er.Richness[resID] += richness
		er.POICount[resID]++
	}
	return empMap, galaxy, rows.Err()
}

// buildBomMaps converts our loaded items and recipes into the shapes the
// pkg/bom calculator expects.
func buildBomMaps(items map[string]*Item, recipes map[string]*Recipe) (map[string]*bom.Item, map[string]*bom.Recipe) {
	bomItems := make(map[string]*bom.Item, len(items))
	for id, it := range items {
		bomItems[id] = &bom.Item{
			ID:       it.ID,
			Name:     it.Name,
			Category: it.Category,
			IsBase:   it.IsBase(),
		}
	}
	bomRecipes := make(map[string]*bom.Recipe, len(recipes))
	for id, r := range recipes {
		in := make([]bom.RecipeItem, len(r.Inputs))
		for i, x := range r.Inputs {
			in[i] = bom.RecipeItem{ItemID: x.ItemID, Quantity: x.Quantity}
		}
		out := make([]bom.RecipeItem, len(r.Outputs))
		for i, x := range r.Outputs {
			out[i] = bom.RecipeItem{ItemID: x.ItemID, Quantity: x.Quantity}
		}
		bomRecipes[id] = &bom.Recipe{ID: r.ID, Inputs: in, Outputs: out}
	}
	return bomItems, bomRecipes
}

// computeProducts builds the Product list by running the BoM calculator on
// every non-base item plus every ship. Items whose BoM produces no base
// materials (no recipe at all) are skipped — they aren't craftable.
func computeProducts(calc *bom.Calculator, items map[string]*Item, ships []*Ship) ([]*Product, error) {
	var products []*Product

	for _, it := range items {
		if it.IsBase() {
			continue
		}
		mats, err := calc.Calculate(it.ID, 1)
		if err != nil {
			return nil, fmt.Errorf("bom for item %s: %w", it.ID, err)
		}
		if !hasOnlyBases(mats, items) {
			// BoM bottomed out on a non-base item with no recipe (e.g. salvage-only
			// drop). Skip — we can't reason about empire sourcing for it.
			continue
		}
		products = append(products, &Product{
			ID:            it.ID,
			Name:          it.Name,
			Kind:          "item",
			Category:      it.Category,
			BaseMaterials: toRefs(mats),
		})
	}

	for _, s := range ships {
		if len(s.BuildMaterials) == 0 {
			continue
		}
		var all []bom.MaterialRequirement
		for _, m := range s.BuildMaterials {
			sub, err := calc.Calculate(m.ItemID, m.Quantity)
			if err != nil {
				return nil, fmt.Errorf("bom for ship %s material %s: %w", s.ID, m.ItemID, err)
			}
			all = append(all, sub...)
		}
		agg := aggregate(all)
		if !hasOnlyBases(agg, items) {
			continue
		}
		products = append(products, &Product{
			ID:            s.ID,
			Name:          s.Name,
			Kind:          "ship",
			Category:      s.Category,
			BaseMaterials: toRefs(agg),
			DirectInputs:  append([]RecipeRef(nil), s.BuildMaterials...),
		})
	}
	return products, nil
}

// hasOnlyBases returns true when every material in mats is a known base item.
// The BoM calculator returns the item itself when no recipe resolves; this
// filter rejects products with un-craftable leaves.
func hasOnlyBases(mats []bom.MaterialRequirement, items map[string]*Item) bool {
	for _, m := range mats {
		it, ok := items[m.ItemID]
		if !ok || !it.IsBase() {
			return false
		}
	}
	return true
}

func toRefs(in []bom.MaterialRequirement) []RecipeRef {
	out := make([]RecipeRef, len(in))
	for i, m := range in {
		out[i] = RecipeRef{ItemID: m.ItemID, Quantity: m.Quantity}
	}
	return out
}

func aggregate(in []bom.MaterialRequirement) []bom.MaterialRequirement {
	totals := make(map[string]int)
	for _, m := range in {
		totals[m.ItemID] += m.Quantity
	}
	out := make([]bom.MaterialRequirement, 0, len(totals))
	for id, qty := range totals {
		out = append(out, bom.MaterialRequirement{ItemID: id, Quantity: qty})
	}
	return out
}
