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
//
// Note: This struct is a placeholder for future implementation.
// The fields are intentionally unused as the calculator functionality
// has not yet been implemented.
type Calculator struct {
	db            *sql.DB              //nolint:unused // Placeholder: database connection
	recipes       map[string]*Recipe   //nolint:unused // Placeholder: recipe cache
	itemToRecipes map[string][]*Recipe //nolint:unused // Placeholder: reverse lookup map
	items         map[string]*Item     //nolint:unused // Placeholder: item cache
	memo          map[string][]MaterialRequirement //nolint:unused // Placeholder: memoization cache
	mu            sync.RWMutex          //nolint:unused // Placeholder: mutex for thread safety
}
