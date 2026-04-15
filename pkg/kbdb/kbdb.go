// Package kbdb manages KB-specific metadata tables in the knowledge database.
// These tables persist generated/derived metadata for POIs (planets, stars, etc.)
// so that values are stable across KB regenerations and can be queried directly.
package kbdb

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed sql/schema.sql
var schemaSQL string

// Schema returns the embedded SQL schema used to create the KB metadata tables.
// The SQL file at pkg/kbdb/sql/schema.sql is the source of truth.
func Schema() string {
	return schemaSQL
}

// Migrate creates or updates the KB metadata tables by applying the embedded
// schema. The schema must be idempotent (all statements use IF NOT EXISTS).
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply kbdb schema: %w", err)
	}
	return nil
}
