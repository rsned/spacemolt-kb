package bom

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"log"
)

//go:embed sql/schema.sql
var schemaSQL string

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
	ID       string
	Name     string
	Type     string
	Category string
	IsBase   bool
}

// Schema returns the BoM table schema
func Schema() string {
	return schemaSQL
}

// Migrate creates or updates the bill_of_materials table
func Migrate(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	return err
}

// ClearBoM removes all existing BoM data
func ClearBoM(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM bill_of_materials`)
	return err
}

// WriteBoM stores a BoM result in the database
func WriteBoM(db *sql.DB, result *BoMResult) error {
	recipePathJSON, err := json.Marshal(result.RecipePath)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO bill_of_materials (target_id, target_type, base_item_id, quantity, recipe_path, has_alternatives)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, mat := range result.BaseMaterials {
		_, err := stmt.Exec(result.TargetID, result.TargetType, mat.ItemID, mat.Quantity, string(recipePathJSON), result.HasAlternatives)
		if err != nil {
			log.Printf("warning: failed to write BoM entry for %s: %v", mat.ItemID, err)
		}
	}

	return tx.Commit()
}
