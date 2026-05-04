package bom

import "strings"

// RecipeResolver handles recipe resolution for BoM calculation
//
// Note: This struct is a placeholder for future implementation.
// The fields are intentionally unused as the resolver functionality
// is currently implemented through standalone functions.
type RecipeResolver struct {
	recipes       map[string]*Recipe   //nolint:unused // Placeholder: recipe cache
	itemToRecipes map[string][]*Recipe //nolint:unused // Placeholder: reverse lookup map
	items         map[string]*Item     //nolint:unused // Placeholder: item cache
}

// RecipeOption represents a possible recipe path with its alternatives
type RecipeOption struct {
	RecipeID     string
	Alternatives []string
	IsBase       bool
}

// BuildRecipeMaps creates reverse lookup maps from recipes
func BuildRecipeMaps(recipes map[string]*Recipe) (map[string][]*Recipe, error) {
	itemToRecipes := make(map[string][]*Recipe)

	for _, recipe := range recipes {
		// Track which items this recipe has already been added for
		recipeItems := make(map[string]bool)
		for _, output := range recipe.Outputs {
			if !recipeItems[output.ItemID] {
				itemToRecipes[output.ItemID] = append(itemToRecipes[output.ItemID], recipe)
				recipeItems[output.ItemID] = true
			}
		}
	}

	return itemToRecipes, nil
}

// SelectRecipe chooses the optimal recipe for an item.
//
// Filtering layers, applied in order so that each falls back to the next when
// it would eliminate every candidate:
//  1. drop packaging recipes (wrap_* / unwrap_*) — these form X↔contained_X
//     cycles in the data and are never the right BoM source.
//  2. drop salvage-input recipes — non-primary production paths.
//  3. of what remains, pick the recipe with the largest total output quantity.
//
// If both filters are empty, falls back to the raw recipe list before picking
// by max output.
func SelectRecipe(itemToRecipes map[string][]*Recipe, itemID string) *Recipe {
	recipes := itemToRecipes[itemID]
	if len(recipes) == 0 {
		return nil
	}

	candidates := filterRecipes(recipes, func(r *Recipe) bool { return !IsPackagingRecipe(r) })
	if len(candidates) == 0 {
		candidates = recipes
	}

	nonSalvage := filterRecipes(candidates, func(r *Recipe) bool { return !UsesSalvage(r) })
	if len(nonSalvage) > 0 {
		candidates = nonSalvage
	}

	var bestRecipe *Recipe
	maxOutputs := 0
	for _, recipe := range candidates {
		totalOutputs := 0
		for _, output := range recipe.Outputs {
			totalOutputs += output.Quantity
		}
		if totalOutputs > maxOutputs {
			maxOutputs = totalOutputs
			bestRecipe = recipe
		}
	}

	return bestRecipe
}

func filterRecipes(in []*Recipe, keep func(*Recipe) bool) []*Recipe {
	var out []*Recipe
	for _, r := range in {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// IsPackagingRecipe reports whether a recipe is a wrap/unwrap container
// transformation. The data uses a strict wrap_X / unwrap_X naming convention
// for these — they exist for inventory packaging, not as a primary production
// source, and including them in BoM resolution creates X ↔ contained_X cycles.
func IsPackagingRecipe(r *Recipe) bool {
	return strings.HasPrefix(r.ID, "wrap_") || strings.HasPrefix(r.ID, "unwrap_")
}

// UsesSalvage checks if a recipe requires salvage components
func UsesSalvage(recipe *Recipe) bool {
	salvageIDs := map[string]bool{
		"rare_salvage":   true,
		"salvage_metal":   true,
		"common_salvage": true,
		"rare_component":  true,
	}

	for _, input := range recipe.Inputs {
		if salvageIDs[input.ItemID] {
			return true
		}
	}
	return false
}
