package bom

// Recipe represents a crafting recipe from the database
type Recipe struct {
	ID           string
	Name         string
	Description  string
	Category     string
	CraftingTime int
	Hidden       bool
	Inputs       []RecipeItem
	Outputs      []RecipeItem
}

// RecipeItem is an item reference within a recipe (input or output)
type RecipeItem struct {
	ItemID       string
	ItemName     string
	ItemCategory string
	Quantity     int
	HasImage     bool
}

// Item represents an item from the database
type Item struct {
	ID          string
	Name        string
	Description string
	Category    string
	Rarity      string
	Size        int
	BaseValue   int
	Stackable   bool
	Tradeable   bool
}
