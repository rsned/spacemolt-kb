package bom

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	tmpfile, err := os.CreateTemp("", "bom-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpfile.Close()

	db, err := sql.Open("sqlite3", tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Clean up after test
	t.Cleanup(func() {
		db.Close()
		os.Remove(tmpfile.Name())
	})

	return db
}

func TestMigrate(t *testing.T) {
	db := setupTestDB(t)

	err := Migrate(db)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify table exists
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='bill_of_materials'`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 table, got %d", count)
	}

	// Verify columns
	rows, err := db.Query(`PRAGMA table_info(bill_of_materials)`)
	if err != nil {
		t.Fatalf("failed to get table schema: %v", err)
	}
	defer rows.Close()

	columns := map[string]bool{
		"target_id":        false,
		"target_type":      false,
		"base_item_id":     false,
		"quantity":         false,
		"recipe_path":      false,
		"has_alternatives": false,
	}

	for rows.Next() {
		var cid int
		var colname string
		var coltype string
		var notnull int
		var dfltValue sql.NullString
		var pk int

		if err := rows.Scan(&cid, &colname, &coltype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("failed to scan column: %v", err)
		}

		if _, exists := columns[colname]; !exists {
			t.Errorf("unexpected column: %s", colname)
		} else {
			columns[colname] = true
		}
	}

	if rows.Err() != nil {
		t.Fatalf("error iterating columns: %v", rows.Err())
	}

	for col, found := range columns {
		if !found {
			t.Errorf("missing column: %s", col)
		}
	}
}

func TestClearBoM(t *testing.T) {
	db := setupTestDB(t)

	// First migrate and add some data
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Insert test data
	_, err := db.Exec(`
		INSERT INTO bill_of_materials (target_id, target_type, base_item_id, quantity, recipe_path, has_alternatives)
		VALUES ('test_item', 'ship', 'iron', 100, '[]', 0)
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	// Verify data exists
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM bill_of_materials`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row before clear, got %d", count)
	}

	// Clear the data
	if err := ClearBoM(db); err != nil {
		t.Fatalf("ClearBoM failed: %v", err)
	}

	// Verify data is cleared
	err = db.QueryRow(`SELECT COUNT(*) FROM bill_of_materials`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows after clear: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after clear, got %d", count)
	}
}

func TestWriteBoM(t *testing.T) {
	db := setupTestDB(t)

	// First migrate
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create a test BoM result
	result := &BoMResult{
		TargetID:        "test_ship",
		TargetName:      "Test Ship",
		TargetType:      "ship",
		HasAlternatives: false,
		BaseMaterials: []MaterialRequirement{
			{ItemID: "iron", Quantity: 100},
			{ItemID: "steel", Quantity: 50},
			{ItemID: "electronics", Quantity: 20},
		},
		RecipePath: []string{"recipe1", "recipe2"},
	}

	// Write the BoM
	if err := WriteBoM(db, result); err != nil {
		t.Fatalf("WriteBoM failed: %v", err)
	}

	// Verify the data was written correctly
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM bill_of_materials`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 rows, got %d", count)
	}

	// Verify each material
	rows, err := db.Query(`SELECT target_id, target_type, base_item_id, quantity, recipe_path, has_alternatives FROM bill_of_materials ORDER BY base_item_id`)
	if err != nil {
		t.Fatalf("failed to query bill_of_materials: %v", err)
	}
	defer rows.Close()

	expected := []struct {
		targetID      string
		targetType    string
		baseItemID    string
		quantity      int
		recipePath    string
		hasAlt        bool
	}{
		{"test_ship", "ship", "electronics", 20, `["recipe1","recipe2"]`, false},
		{"test_ship", "ship", "iron", 100, `["recipe1","recipe2"]`, false},
		{"test_ship", "ship", "steel", 50, `["recipe1","recipe2"]`, false},
	}

	idx := 0
	for rows.Next() {
		var targetID, targetType, baseItemID, recipePath string
		var quantity int
		var hasAlt bool

		if err := rows.Scan(&targetID, &targetType, &baseItemID, &quantity, &recipePath, &hasAlt); err != nil {
			t.Fatalf("failed to scan row: %v", err)
		}

		if idx >= len(expected) {
			t.Errorf("too many rows")
			continue
		}

		exp := expected[idx]
		if targetID != exp.targetID {
			t.Errorf("row %d: expected target_id %s, got %s", idx, exp.targetID, targetID)
		}
		if targetType != exp.targetType {
			t.Errorf("row %d: expected target_type %s, got %s", idx, exp.targetType, targetType)
		}
		if baseItemID != exp.baseItemID {
			t.Errorf("row %d: expected base_item_id %s, got %s", idx, exp.baseItemID, baseItemID)
		}
		if quantity != exp.quantity {
			t.Errorf("row %d: expected quantity %d, got %d", idx, exp.quantity, quantity)
		}
		if recipePath != exp.recipePath {
			t.Errorf("row %d: expected recipe_path %s, got %s", idx, exp.recipePath, recipePath)
		}
		if hasAlt != exp.hasAlt {
			t.Errorf("row %d: expected has_alternatives %v, got %v", idx, exp.hasAlt, hasAlt)
		}

		idx++
	}

	if rows.Err() != nil {
		t.Fatalf("error iterating rows: %v", rows.Err())
	}

	if idx != len(expected) {
		t.Errorf("expected %d rows, got %d", len(expected), idx)
	}
}

func TestWriteBoMWithAlternatives(t *testing.T) {
	db := setupTestDB(t)

	// First migrate
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create a test BoM result with alternatives
	result := &BoMResult{
		TargetID:        "complex_item",
		TargetName:      "Complex Item",
		TargetType:      "module",
		HasAlternatives: true,
		BaseMaterials: []MaterialRequirement{
			{ItemID: "rare_metal", Quantity: 5},
		},
		RecipePath: []string{"craft_recipe"},
	}

	// Write the BoM
	if err := WriteBoM(db, result); err != nil {
		t.Fatalf("WriteBoM failed: %v", err)
	}

	// Verify has_alternatives flag
	var hasAlt bool
	err := db.QueryRow(`SELECT has_alternatives FROM bill_of_materials LIMIT 1`).Scan(&hasAlt)
	if err != nil {
		t.Fatalf("failed to query has_alternatives: %v", err)
	}

	if !hasAlt {
		t.Errorf("expected has_alternatives to be true, got false")
	}
}

func TestWriteBoMEmptyMaterials(t *testing.T) {
	db := setupTestDB(t)

	// First migrate
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create a BoM result with no materials
	result := &BoMResult{
		TargetID:        "simple_item",
		TargetName:      "Simple Item",
		TargetType:      "item",
		HasAlternatives: false,
		BaseMaterials:   []MaterialRequirement{},
		RecipePath:      []string{},
	}

	// Write the BoM - should succeed even with no materials
	if err := WriteBoM(db, result); err != nil {
		t.Fatalf("WriteBoM with empty materials failed: %v", err)
	}

	// Verify no rows were written
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM bill_of_materials`).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows for empty materials, got %d", count)
	}
}

func TestSchema(t *testing.T) {
	schema := Schema()
	if schema == "" {
		t.Error("Schema() returned empty string")
	}

	// Verify it contains expected table name
	if !contains(schema, "bill_of_materials") {
		t.Error("Schema does not contain table name 'bill_of_materials'")
	}

	// Verify it contains expected columns
	expectedColumns := []string{"target_id", "target_type", "base_item_id", "quantity", "recipe_path", "has_alternatives"}
	for _, col := range expectedColumns {
		if !contains(schema, col) {
			t.Errorf("Schema does not contain column '%s'", col)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[0:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr))))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
