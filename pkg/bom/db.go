package bom

// Recipe represents a crafting recipe from the database
type Recipe struct {
	ID          string
	OutputID    string
	OutputName  string
	OutputQty   int
	Alternative bool
}

// Item represents an item from the database
type Item struct {
	ID           string
	Name         string
	Type         string
	Category     string
	IsBase       bool
}
