package main

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// Player stations appear in bases but not pois; their names must still
// resolve instead of falling back to the 32-hex id.
func TestEnrichMissionLocationNamesUsesBases(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, q := range []string{
		`CREATE TABLE pois (id TEXT, name TEXT, system_id TEXT)`,
		`CREATE TABLE systems (id TEXT, name TEXT)`,
		`CREATE TABLE bases (id TEXT PRIMARY KEY, poi_id TEXT NOT NULL, name TEXT NOT NULL)`,
		`INSERT INTO pois VALUES ('grand_exchange_station', 'Grand Exchange Station', 'nexus')`,
		`INSERT INTO bases VALUES ('b495c6003fc83e18f6d8cecbe6929133', 'p1', 'Hex Star')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	m := &Mission{
		ExclusiveBaseID: "b495c6003fc83e18f6d8cecbe6929133",
		Locations: []MissionLocation{
			{BaseID: "b495c6003fc83e18f6d8cecbe6929133"},
			{BaseID: "grand_exchange_station"},
		},
	}
	if err := enrichMissionLocationNames(db, []*Mission{m}); err != nil {
		t.Fatal(err)
	}
	if m.Locations[0].BaseName != "Hex Star" || m.Locations[1].BaseName != "Grand Exchange Station" || m.ExclusiveBaseName != "Hex Star" {
		t.Fatalf("names = %q, %q, exclusive %q", m.Locations[0].BaseName, m.Locations[1].BaseName, m.ExclusiveBaseName)
	}
}
