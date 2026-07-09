package main

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newCraftingFixture builds an in-memory crafting DB with the subset of the
// schema loadRecipes reads.
func newCraftingFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open crafting fixture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE items (id TEXT PRIMARY KEY, name TEXT, category TEXT)`,
		`CREATE TABLE recipes (
			id TEXT PRIMARY KEY, name TEXT, description TEXT, category TEXT,
			crafting_time REAL, facility_only INTEGER DEFAULT 0)`,
		`CREATE TABLE recipe_inputs (recipe_id TEXT, item_id TEXT, quantity INTEGER)`,
		`CREATE TABLE recipe_outputs (recipe_id TEXT, item_id TEXT, quantity INTEGER)`,

		`INSERT INTO items VALUES ('iron_ore','Iron Ore','ore')`,
		`INSERT INTO items VALUES ('steel_plate','Steel Plate','material')`,
		`INSERT INTO items VALUES ('copper_wire','Copper Wire','material')`,

		// refine_steel: facility_only, has a public line (in the facilities fixture).
		`INSERT INTO recipes VALUES ('refine_steel','Refine Steel','','Refining',12.5,1)`,
		`INSERT INTO recipe_inputs VALUES ('refine_steel','iron_ore',5)`,
		`INSERT INTO recipe_outputs VALUES ('refine_steel','steel_plate',2)`,

		// gap_recipe: facility_only, NO public line -> belongs in the gap table.
		`INSERT INTO recipes VALUES ('gap_recipe','Gap Recipe','','Weapons',30,1)`,
		`INSERT INTO recipe_outputs VALUES ('gap_recipe','copper_wire',1)`,

		// hand_recipe: not facility_only, NO public line -> "no facility required".
		`INSERT INTO recipes VALUES ('hand_recipe','Hand Recipe','','Components',3,0)`,
		`INSERT INTO recipe_outputs VALUES ('hand_recipe','copper_wire',4)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	return db
}

func TestLoadRecipesReadsFacilityOnly(t *testing.T) {
	db := newCraftingFixture(t)

	recipes, err := loadRecipes(db)
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}

	// facility_only must come from the DB, without the catalog overlay having run.
	if !recipes["refine_steel"].FacilityOnly {
		t.Error("refine_steel.FacilityOnly = false, want true (from DB column)")
	}
	if !recipes["gap_recipe"].FacilityOnly {
		t.Error("gap_recipe.FacilityOnly = false, want true (from DB column)")
	}
	if recipes["hand_recipe"].FacilityOnly {
		t.Error("hand_recipe.FacilityOnly = true, want false")
	}
}
