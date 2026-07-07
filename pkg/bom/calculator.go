package bom

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
)

// MaterialRequirement represents a single material in the BoM
type MaterialRequirement struct {
	ItemID   string
	Quantity int
}

// BoMResult contains the complete breakdown for a target item
type BoMResult struct {
	TargetID        string
	TargetName      string
	TargetType      string
	HasAlternatives bool
	BaseMaterials   []MaterialRequirement
	RecipePath      []string
}

// Calculator holds state for BoM computation.
//
// memo stores per-1-unit base materials for each item; callers scale on lookup
// via Calculate(). Cycle detection lives on the call stack via the path argument
// to calculatePerUnit so it cannot leak across calls.
type Calculator struct {
	db            *sql.DB
	recipes       map[string]*Recipe
	itemToRecipes map[string][]*Recipe
	items         map[string]*Item
	sourceable    map[string]bool // nil unless a market-available set was supplied
	memo          map[string][]MaterialRequirement
	mu            sync.RWMutex
}

// NewCalculator creates a calculator that selects recipes purely by structure
// (no market awareness). Equivalent to NewCalculatorWithMarket with a nil set.
func NewCalculator(db *sql.DB, recipes map[string]*Recipe, items map[string]*Item) (*Calculator, error) {
	return NewCalculatorWithMarket(db, recipes, items, nil)
}

// NewCalculatorWithMarket creates a calculator that, when marketAvailable is
// non-nil, prefers recipe paths whose inputs can actually be sourced (are
// market-available or craftable from market-available items) over shorter paths
// that terminate in an unbuyable item. Pass nil to disable market awareness.
func NewCalculatorWithMarket(db *sql.DB, recipes map[string]*Recipe, items map[string]*Item, marketAvailable map[string]bool) (*Calculator, error) {
	itemToRecipes, err := BuildRecipeMaps(recipes)
	if err != nil {
		return nil, err
	}

	c := &Calculator{
		db:            db,
		recipes:       recipes,
		itemToRecipes: itemToRecipes,
		items:         items,
		memo:          make(map[string][]MaterialRequirement),
	}
	if marketAvailable != nil {
		c.sourceable = ComputeSourceable(itemToRecipes, marketAvailable, c.isTerminal)
	}
	return c, nil
}

// isTerminal reports whether the flattener stops at itemID without expanding a
// recipe: unknown items, ore/material catalog categories, and items no recipe
// produces. It mirrors the terminal conditions in calculatePerUnit so that
// sourceability and flattening agree.
func (c *Calculator) isTerminal(itemID string) bool {
	item, ok := c.items[itemID]
	if !ok {
		return true
	}
	if item.Category == "ore" || item.Category == "material" {
		return true
	}
	return len(c.itemToRecipes[itemID]) == 0
}

// Calculate returns base materials needed to produce `quantity` units of itemID.
func (c *Calculator) Calculate(itemID string, quantity int) ([]MaterialRequirement, error) {
	perUnit, err := c.calculatePerUnit(itemID, nil)
	if err != nil {
		return nil, err
	}
	return scaleMaterials(perUnit, quantity), nil
}

// calculatePerUnit returns base materials for ONE unit of itemID. The path
// argument is the current recursion stack and is used for cycle detection;
// pass nil at the top level. The slice is treated as immutable below the
// current frame: each recursive call creates a fresh slice header so siblings
// don't see each other's appends.
func (c *Calculator) calculatePerUnit(itemID string, path []string) ([]MaterialRequirement, error) {
	c.mu.RLock()
	item, hasItem := c.items[itemID]
	c.mu.RUnlock()

	if !hasItem {
		return []MaterialRequirement{{ItemID: itemID, Quantity: 1}}, nil
	}
	if item.Category == "ore" || item.Category == "material" {
		return []MaterialRequirement{{ItemID: itemID, Quantity: 1}}, nil
	}

	// Per-unit memo lookup.
	c.mu.RLock()
	if cached, ok := c.memo[itemID]; ok {
		c.mu.RUnlock()
		return cloneMaterials(cached), nil
	}
	c.mu.RUnlock()

	// Cycle detection: walk the stack in order so the error reports the cycle
	// deterministically.
	for i, prev := range path {
		if prev == itemID {
			cycle := append([]string(nil), path[i:]...)
			cycle = append(cycle, itemID)
			return nil, fmt.Errorf("circular dependency detected in BoM calculation: %v", cycle)
		}
	}
	// Fresh slice header so concurrent sibling recursions can't see appends
	// past this frame's length.
	nextPath := append(append([]string(nil), path...), itemID)

	recipe := selectRecipe(c.itemToRecipes, itemID, c.sourceable)
	if recipe == nil {
		return []MaterialRequirement{{ItemID: itemID, Quantity: 1}}, nil
	}

	var outputQty int
	for _, output := range recipe.Outputs {
		if output.ItemID == itemID {
			outputQty = output.Quantity
			break
		}
	}
	if outputQty == 0 {
		log.Printf("warning: recipe %s has zero output for item %s", recipe.ID, itemID)
		return []MaterialRequirement{{ItemID: itemID, Quantity: 1}}, nil
	}

	// Walk inputs at recipe-batch quantity, then divide-with-ceil at the end so
	// per-unit values are correct even when a recipe outputs more than 1.
	var batch []MaterialRequirement
	for _, input := range recipe.Inputs {
		sub, err := c.calculatePerUnit(input.ItemID, nextPath)
		if err != nil {
			return nil, err
		}
		batch = append(batch, scaleMaterials(sub, input.Quantity)...)
	}
	aggregated := aggregateMaterials(batch)

	perUnit := make([]MaterialRequirement, len(aggregated))
	for i, mat := range aggregated {
		perUnit[i] = MaterialRequirement{
			ItemID:   mat.ItemID,
			Quantity: ceilDiv(mat.Quantity, outputQty),
		}
	}

	c.mu.Lock()
	c.memo[itemID] = perUnit
	c.mu.Unlock()

	return cloneMaterials(perUnit), nil
}

// scaleMaterials returns a copy of in with each Quantity multiplied by factor.
func scaleMaterials(in []MaterialRequirement, factor int) []MaterialRequirement {
	out := make([]MaterialRequirement, len(in))
	for i, m := range in {
		out[i] = MaterialRequirement{ItemID: m.ItemID, Quantity: m.Quantity * factor}
	}
	return out
}

// cloneMaterials returns a defensive copy so callers cannot mutate the memo cache.
func cloneMaterials(in []MaterialRequirement) []MaterialRequirement {
	out := make([]MaterialRequirement, len(in))
	copy(out, in)
	return out
}

// ceilDiv returns ceil(a/b). b==0 returns a (caller has already logged).
func ceilDiv(a, b int) int {
	if b == 0 {
		return a
	}
	return (a + b - 1) / b
}

// ShipBuildRef links an item to a ship that requires it
type ShipBuildRef struct {
	ItemID   string
	Quantity int
}

// FacilityMaterial represents a facility build material
type FacilityMaterial struct {
	ItemID   string
	Name     string
	Quantity int
}

// Ship is a minimal ship struct for BoM calculation
type Ship struct {
	ID             string
	Name           string
	BuildMaterials []ShipBuildRef
}

// Facility is a minimal facility struct for BoM calculation
type Facility struct {
	ID             string
	Name           string
	BuildMaterials []FacilityMaterial
}

// CalculateAll computes BoM for all items, ships, and facilities.
//
// All results are buffered in memory and only persisted after every calculation
// succeeds, so a mid-run failure leaves the existing BoM data intact.
func (c *Calculator) CalculateAll(
	items map[string]*Item,
	ships map[string]*Ship,
	facilities map[string]*Facility,
) error {
	var results []*BoMResult

	log.Printf("Calculating BoM for %d items...", len(items))

	for itemID, item := range items {
		if item.Category == "ore" || item.Category == "material" {
			continue
		}

		materials, err := c.Calculate(itemID, 1)
		if err != nil {
			return fmt.Errorf("calculate BoM for item %s: %w", itemID, err)
		}

		result := &BoMResult{
			TargetID:        itemID,
			TargetName:      item.Name,
			TargetType:      "item",
			BaseMaterials:   materials,
			HasAlternatives: len(c.itemToRecipes[itemID]) > 1,
		}

		if recipe := selectRecipe(c.itemToRecipes, itemID, c.sourceable); recipe != nil {
			result.RecipePath = []string{recipe.ID}
		}

		results = append(results, result)
	}

	if ships != nil {
		log.Printf("Calculating BoM for %d ships...", len(ships))

		for shipID, ship := range ships {
			var allMaterials []MaterialRequirement
			hasAlt := false

			for _, mat := range ship.BuildMaterials {
				materials, err := c.Calculate(mat.ItemID, mat.Quantity)
				if err != nil {
					return fmt.Errorf("calculate BoM for ship %s material %s: %w", shipID, mat.ItemID, err)
				}
				if len(c.itemToRecipes[mat.ItemID]) > 1 {
					hasAlt = true
				}
				allMaterials = append(allMaterials, materials...)
			}

			results = append(results, &BoMResult{
				TargetID:        shipID,
				TargetName:      ship.Name,
				TargetType:      "ship",
				BaseMaterials:   aggregateMaterials(allMaterials),
				HasAlternatives: hasAlt,
			})
		}
	}

	if facilities != nil {
		log.Printf("Calculating BoM for %d facilities...", len(facilities))

		for facID, facility := range facilities {
			var allMaterials []MaterialRequirement
			hasAlt := false

			for _, mat := range facility.BuildMaterials {
				materials, err := c.Calculate(mat.ItemID, mat.Quantity)
				if err != nil {
					return fmt.Errorf("calculate BoM for facility %s material %s: %w", facID, mat.ItemID, err)
				}
				if len(c.itemToRecipes[mat.ItemID]) > 1 {
					hasAlt = true
				}
				allMaterials = append(allMaterials, materials...)
			}

			results = append(results, &BoMResult{
				TargetID:        facID,
				TargetName:      facility.Name,
				TargetType:      "facility",
				BaseMaterials:   aggregateMaterials(allMaterials),
				HasAlternatives: hasAlt,
			})
		}
	}

	// All calculations succeeded — clear the old data and persist the new set.
	if err := ClearBoM(c.db); err != nil {
		return fmt.Errorf("clear bom database: %w", err)
	}
	for _, result := range results {
		if err := WriteBoM(c.db, result); err != nil {
			return fmt.Errorf("write BoM for %s/%s: %w", result.TargetType, result.TargetID, err)
		}
	}

	log.Printf("BoM calculation complete: %d targets persisted", len(results))
	return nil
}

// JSON returns a minified JSON array of the BoM's base materials in the form
// [{"item_id":"X","quantity":N},...]. Returns "" for nil or empty results so
// templates can drop the JSON section cleanly.
func (r *BoMResult) JSON() string {
	if r == nil || len(r.BaseMaterials) == 0 {
		return ""
	}
	type entry struct {
		ItemID   string `json:"item_id"`
		Quantity int    `json:"quantity"`
	}
	out := make([]entry, len(r.BaseMaterials))
	for i, mat := range r.BaseMaterials {
		out[i] = entry(mat)
	}
	data, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(data)
}

// aggregateMaterials combines duplicate materials by summing quantities.
func aggregateMaterials(materials []MaterialRequirement) []MaterialRequirement {
	matMap := make(map[string]int)
	for _, mat := range materials {
		matMap[mat.ItemID] += mat.Quantity
	}

	result := make([]MaterialRequirement, 0, len(matMap))
	for itemID, qty := range matMap {
		result = append(result, MaterialRequirement{ItemID: itemID, Quantity: qty})
	}

	sort.Slice(result, func(a, b int) bool {
		return result[a].ItemID < result[b].ItemID
	})

	return result
}
