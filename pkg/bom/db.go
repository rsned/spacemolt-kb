package bom

// RecipeItem represents a single item input or output in a recipe
type RecipeItem struct {
	ItemID   string
	Quantity int
}

// Recipe represents a crafting recipe from the database
type Recipe struct {
	ID       string
	Inputs   []RecipeItem
	Outputs  []RecipeItem
}

// Item represents an item from the database
type Item struct {
	ID           string
	Name         string
	Type         string
	Category     string
	IsBase       bool
}
