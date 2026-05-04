package bom

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"sync"
)

// MaterialRequirement represents a single material in the BoM
type MaterialRequirement struct {
	ItemID  string
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

// Calculator holds state for BoM computation
type Calculator struct {
	db            *sql.DB
	recipes       map[string]*Recipe
	itemToRecipes map[string][]*Recipe
	items         map[string]*Item
	memo          map[string][]MaterialRequirement
	visited       map[string]struct{}
	mu            sync.RWMutex
}

// NewCalculator creates a new calculator with database and data
func NewCalculator(db *sql.DB, recipes map[string]*Recipe, items map[string]*Item) (*Calculator, error) {
	itemToRecipes, err := BuildRecipeMaps(recipes)
	if err != nil {
		return nil, err
	}

	return &Calculator{
		db:            db,
		recipes:       recipes,
		itemToRecipes: itemToRecipes,
		items:         items,
		memo:          make(map[string][]MaterialRequirement),
		visited:       make(map[string]struct{}),
	}, nil
}

// Calculate recursively computes base materials for an item
func (c *Calculator) Calculate(itemID string, quantity int) ([]MaterialRequirement, error) {
	c.mu.RLock()
	item, hasItem := c.items[itemID]
	c.mu.RUnlock()

	if !hasItem {
		// Unknown item - treat as base material
		return []MaterialRequirement{{ItemID: itemID, Quantity: quantity}}, nil
	}

	// Check if base material (ore or material category)
	if item.Category == "ore" || item.Category == "material" {
		return []MaterialRequirement{{ItemID: itemID, Quantity: quantity}}, nil
	}

	// Check memo cache
	c.mu.RLock()
	if cached, ok := c.memo[itemID]; ok {
		c.mu.RUnlock()
		// Scale cached result by requested quantity
		scaled := make([]MaterialRequirement, len(cached))
		for i, mat := range cached {
			scaled[i] = MaterialRequirement{
				ItemID:  mat.ItemID,
				Quantity: mat.Quantity * quantity,
			}
		}
		return scaled, nil
	}
	c.mu.RUnlock()

	// Check for circular dependency
	c.mu.Lock()
	_, inPath := c.visited[itemID]
	if inPath {
		c.mu.Unlock()
		// Build cycle message for error
		var cycle []string
		for id := range c.visited {
			cycle = append(cycle, id)
		}
		cycle = append(cycle, itemID)
		return nil, fmt.Errorf("circular dependency detected in BoM calculation: %v", cycle)
	}
	c.visited[itemID] = struct{}{}
	c.mu.Unlock()

	// Select recipe
	recipe := SelectRecipe(c.itemToRecipes, itemID)
	if recipe == nil {
		// Not craftable - treat as base material
		return []MaterialRequirement{{ItemID: itemID, Quantity: quantity}}, nil
	}

	// Calculate output quantity for scaling
	var outputQty int
	for _, output := range recipe.Outputs {
		if output.ItemID == itemID {
			outputQty = output.Quantity
			break
		}
	}

	if outputQty == 0 {
		log.Printf("warning: recipe %s has zero output for item %s", recipe.ID, itemID)
		return []MaterialRequirement{{ItemID: itemID, Quantity: quantity}}, nil
	}

	// Calculate multiplier
	multiplier := float64(quantity) / float64(outputQty)

	// Recursively calculate inputs
	var allMaterials []MaterialRequirement
	for _, input := range recipe.Inputs {
		inputQty := int(float64(input.Quantity) * multiplier)

		subMaterials, err := c.Calculate(input.ItemID, inputQty)
		if err != nil {
			return nil, err
		}

		allMaterials = append(allMaterials, subMaterials...)
	}

	// Remove from visited set
	c.mu.Lock()
	delete(c.visited, itemID)
	c.mu.Unlock()

	// Aggregate materials
	aggregated := c.aggregateMaterials(allMaterials)

	// Memoize result
	c.mu.Lock()
	c.memo[itemID] = aggregated
	c.mu.Unlock()

	return aggregated, nil
}

// aggregateMaterials combines duplicate materials by summing quantities
func (c *Calculator) aggregateMaterials(materials []MaterialRequirement) []MaterialRequirement {
	matMap := make(map[string]int)
	for _, mat := range materials {
		matMap[mat.ItemID] += mat.Quantity
	}

	result := make([]MaterialRequirement, 0, len(matMap))
	for itemID, qty := range matMap {
		result = append(result, MaterialRequirement{ItemID: itemID, Quantity: qty})
	}

	// Sort for consistent output
	sort.Slice(result, func(a, b int) bool {
		return result[a].ItemID < result[b].ItemID
	})

	return result
}
