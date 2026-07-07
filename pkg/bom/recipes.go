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

// SelectRecipe chooses the optimal recipe for an item using only its structure
// (no market awareness). See selectRecipe for the filtering layers.
func SelectRecipe(itemToRecipes map[string][]*Recipe, itemID string) *Recipe {
	return selectRecipe(itemToRecipes, itemID, nil)
}

// selectRecipe chooses the optimal recipe for an item.
//
// Filtering layers, applied in order so that each falls back to the next when
// it would eliminate every candidate:
//  1. drop packaging recipes (wrap_* / unwrap_*) — these form X↔contained_X
//     cycles in the data and are never the right BoM source.
//  2. drop salvage-input recipes — non-primary production paths.
//  3. when sourceable is non-nil, drop recipes with an input that cannot be
//     obtained (not market-available and not itself craftable-from-sourceable).
//     This stops a shorter recipe path that terminates in an unbuyable item
//     (e.g. control_node via superfluid_vial, which has no market supply) from
//     being chosen over a longer path whose inputs can actually be sourced.
//  4. of what remains, pick the recipe with the largest total output quantity.
//
// Each layer is a fallback: if it would eliminate every candidate it is
// skipped, so a fully-unsourceable item still resolves to some recipe.
func selectRecipe(itemToRecipes map[string][]*Recipe, itemID string, sourceable map[string]bool) *Recipe {
	recipes := itemToRecipes[itemID]
	if len(recipes) == 0 {
		return nil
	}

	candidates := filterRecipes(recipes, func(r *Recipe) bool { return !IsPackagingRecipe(r) })
	if len(candidates) == 0 {
		candidates = recipes
	}

	if nonSalvage := filterRecipes(candidates, func(r *Recipe) bool { return !UsesSalvage(r) }); len(nonSalvage) > 0 {
		candidates = nonSalvage
	}

	if sourceable != nil {
		if srcOK := filterRecipes(candidates, func(r *Recipe) bool { return allInputsSourceable(r, sourceable) }); len(srcOK) > 0 {
			candidates = srcOK
		}
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

// allInputsSourceable reports whether every input of r is in the sourceable set.
func allInputsSourceable(r *Recipe, sourceable map[string]bool) bool {
	for _, in := range r.Inputs {
		if !sourceable[in.ItemID] {
			return false
		}
	}
	return true
}

// ComputeSourceable returns the set of item IDs that can actually be obtained.
//
// An item is sourceable if it is market-available on its own, or a
// non-packaging recipe produces it from inputs that are all (recursively)
// sourceable. isTerminal reports whether the BoM flattener treats an item as a
// base material (no recipe expansion); a terminal item is sourceable only when
// market-available, mirroring the flattener so selection and pricing agree.
// isTerminal may be nil, in which case every item is allowed a recipe path.
//
// The result is a least-fixpoint, computed by iterating to convergence: each
// pass can only add items, and there are finitely many, so it terminates.
func ComputeSourceable(itemToRecipes map[string][]*Recipe, marketAvailable map[string]bool, isTerminal func(string) bool) map[string]bool {
	sourceable := make(map[string]bool, len(marketAvailable))
	for id, ok := range marketAvailable {
		if ok {
			sourceable[id] = true
		}
	}
	for {
		changed := false
		for item, recipes := range itemToRecipes {
			if sourceable[item] || (isTerminal != nil && isTerminal(item)) {
				continue
			}
			for _, r := range recipes {
				if IsPackagingRecipe(r) {
					continue
				}
				if allInputsSourceable(r, sourceable) {
					sourceable[item] = true
					changed = true
					break
				}
			}
		}
		if !changed {
			break
		}
	}
	return sourceable
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
