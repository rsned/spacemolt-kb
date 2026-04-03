// Package kbdb manages KB-specific metadata tables in the knowledge database.
// These tables persist generated/derived metadata for POIs (planets, stars, etc.)
// so that values are stable across KB regenerations and can be queried directly.
package kbdb

import (
	"database/sql"
	"fmt"
)

// migrations are the KB metadata table migrations, run in order.
// Each migration is idempotent (CREATE TABLE IF NOT EXISTS).
var migrations = []struct {
	Name string
	SQL  string
}{
	{
		Name: "poi_metadata_planets",
		SQL: `CREATE TABLE IF NOT EXISTS poi_metadata_planets (
			poi_id            TEXT PRIMARY KEY,
			poi_name          TEXT NOT NULL,
			seed              INTEGER NOT NULL,
			planet_class      TEXT NOT NULL,
			type_name         TEXT NOT NULL,
			radius_km         REAL NOT NULL,
			mass_earths       REAL NOT NULL,
			gravity_g         REAL NOT NULL,
			temperature_k     REAL NOT NULL,
			temperature_c     REAL NOT NULL,
			atm_pressure_atm  REAL NOT NULL,
			atm_description   TEXT NOT NULL,
			day_length_hours  REAL NOT NULL,
			orbital_period_days REAL NOT NULL,
			orbital_distance_au REAL NOT NULL
		)`,
	},
	{
		Name: "poi_metadata_stars",
		SQL: `CREATE TABLE IF NOT EXISTS poi_metadata_stars (
			poi_id            TEXT PRIMARY KEY,
			poi_name          TEXT NOT NULL,
			seed              INTEGER NOT NULL,
			star_class        TEXT NOT NULL,
			spectral_type     TEXT NOT NULL,
			spectral_subtype  INTEGER NOT NULL DEFAULT -1,
			luminosity_class  TEXT NOT NULL,
			luminosity_name   TEXT NOT NULL,
			color_hex         TEXT NOT NULL,
			color_name        TEXT NOT NULL,
			temp_range        TEXT NOT NULL,
			size_multiplier   REAL NOT NULL,
			render_size       REAL NOT NULL
		)`,
	},
}

// Migrate creates or updates the KB metadata tables.
func Migrate(db *sql.DB) error {
	for _, m := range migrations {
		if _, err := db.Exec(m.SQL); err != nil {
			return fmt.Errorf("migration %q: %w", m.Name, err)
		}
	}
	return nil
}
