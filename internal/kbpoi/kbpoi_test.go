package kbpoi

import (
	"database/sql"
	"slices"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLiveHidesOnlySupersededRenames(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	stmts := []string{
		`CREATE TABLE systems (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE pois (id TEXT PRIMARY KEY, system_id TEXT, name TEXT, last_updated_tick INTEGER)`,
		`INSERT INTO systems VALUES ('botein','Botein'), ('hadar','Hadar')`,
		// botein was rescanned after the rename: old belt row left behind
		`INSERT INTO pois VALUES ('alloy_dust_belt_botein','botein','Alloy Dust Belt — Botein',100)`,
		`INSERT INTO pois VALUES ('lampglint_rock','botein','Lampglint Rock',200)`,
		`INSERT INTO pois VALUES ('botein_i','botein','Botein I',200)`,
		// hadar not rescanned: the old name is still the latest data
		`INSERT INTO pois VALUES ('crystalline_field_hadar','hadar','Crystalline Field — Hadar',100)`,
		`INSERT INTO pois VALUES ('hadar_i','hadar','Hadar I',100)`,
		// a dash in a name that does not end with the system name is not a rename
		`INSERT INTO pois VALUES ('relay','botein','Relay — Outpost',50)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	rows, err := db.Query(`SELECT p.id FROM pois p WHERE ` + Live + ` ORDER BY p.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	want := []string{"botein_i", "crystalline_field_hadar", "hadar_i", "lampglint_rock", "relay"}
	if !slices.Equal(got, want) {
		t.Fatalf("live POIs %v, want %v", got, want)
	}
}
