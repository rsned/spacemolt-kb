package bom

import (
	"database/sql"
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
	mu            sync.RWMutex
}
