package bom

// RecipeResolver handles recipe resolution for BoM calculation
type RecipeResolver struct {
	recipes       map[string]*Recipe
	itemToRecipes map[string][]*Recipe
	items         map[string]*Item
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

// SelectRecipe chooses the optimal recipe for an item
func SelectRecipe(itemToRecipes map[string][]*Recipe, itemID string) *Recipe {
	recipes := itemToRecipes[itemID]
	if len(recipes) == 0 {
		return nil
	}

	// Filter out salvage recipes if alternatives exist
	var candidates []*Recipe
	for _, recipe := range recipes {
		if !UsesSalvage(recipe) {
			candidates = append(candidates, recipe)
		}
	}

	// If all recipes use salvage, fall back to all (alphabetical)
	if len(candidates) == 0 {
		candidates = recipes
	}

	// Select recipe with most outputs
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
