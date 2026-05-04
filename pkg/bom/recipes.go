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
