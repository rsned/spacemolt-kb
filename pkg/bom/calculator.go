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

// CalculateAll computes BoM for all items, ships, and facilities
func (c *Calculator) CalculateAll(
	items map[string]*Item,
	ships map[string]*Ship,
	facilities map[string]*Facility,
) error {
	// Clear previous data
	if err := ClearBoM(c.db); err != nil {
		return fmt.Errorf("clear bom database: %w", err)
	}

	log.Printf("Calculating BoM for %d items...", len(items))

	// Calculate for items
	for itemID, item := range items {
		if item.Category == "ore" || item.Category == "material" {
			// Skip base materials
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

		// Select recipe path
		recipe := SelectRecipe(c.itemToRecipes, itemID)
		if recipe != nil {
			result.RecipePath = []string{recipe.ID}
		}

		if err := WriteBoM(c.db, result); err != nil {
			return fmt.Errorf("write BoM for item %s: %w", itemID, err)
		}
	}

	// Calculate for ships
	if ships != nil {
		log.Printf("Calculating BoM for %d ships...", len(ships))

		for shipID, ship := range ships {
			var allMaterials []MaterialRequirement

			for _, mat := range ship.BuildMaterials {
				materials, err := c.Calculate(mat.ItemID, mat.Quantity)
				if err != nil {
					return fmt.Errorf("calculate BoM for ship %s material %s: %w", shipID, mat.ItemID, err)
				}
				allMaterials = append(allMaterials, materials...)
			}

			// Aggregate and sort
			aggregated := c.aggregateMaterials(allMaterials)

			result := &BoMResult{
				TargetID:      shipID,
				TargetName:    ship.Name,
				TargetType:    "ship",
				BaseMaterials: aggregated,
			}

			if err := WriteBoM(c.db, result); err != nil {
				return fmt.Errorf("write BoM for ship %s: %w", shipID, err)
			}
		}
	}

	// Calculate for facilities
	if facilities != nil {
		log.Printf("Calculating BoM for %d facilities...", len(facilities))

		for facID, facility := range facilities {
			var allMaterials []MaterialRequirement

			for _, mat := range facility.BuildMaterials {
				materials, err := c.Calculate(mat.ItemID, mat.Quantity)
				if err != nil {
					return fmt.Errorf("calculate BoM for facility %s material %s: %w", facID, mat.ItemID, err)
				}
				allMaterials = append(allMaterials, materials...)
			}

			// Aggregate and sort
			aggregated := c.aggregateMaterials(allMaterials)

			result := &BoMResult{
				TargetID:      facID,
				TargetName:    facility.Name,
				TargetType:    "facility",
				BaseMaterials: aggregated,
			}

			if err := WriteBoM(c.db, result); err != nil {
				return fmt.Errorf("write BoM for facility %s: %w", facID, err)
			}
		}
	}

	log.Printf("BoM calculation complete")
	return nil
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
