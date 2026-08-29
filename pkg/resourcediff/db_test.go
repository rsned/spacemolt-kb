package resourcediff

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE systems (id TEXT PRIMARY KEY, name TEXT, last_updated_tick INTEGER DEFAULT 0)`,
		`CREATE TABLE pois (id TEXT PRIMARY KEY, system_id TEXT, name TEXT, type TEXT, hidden BOOLEAN NOT NULL DEFAULT 0)`,
		`CREATE TABLE items (id TEXT PRIMARY KEY, name TEXT, category TEXT)`,
		`CREATE TABLE poi_resources (poi_id TEXT, resource_id TEXT, richness REAL, remaining REAL, last_updated_tick INTEGER DEFAULT 0, max_remaining REAL NOT NULL DEFAULT 0)`,
		`INSERT INTO systems VALUES ('sol','Sol',100), ('haven','Haven',0), ('dark','Dark',5)`,
		`INSERT INTO pois VALUES ('sol_station','sol','Sol Station','station',0), ('sol_belt','sol','Belt','asteroid_belt',0), ('dark_core','dark','Core','asteroid_belt',1)`,
		`INSERT INTO items VALUES ('iron_ore','Iron Ore','ore'), ('void_crystal','Void Crystal','material'), ('laser','Laser','weapon')`,
		`INSERT INTO poi_resources VALUES ('sol_belt','iron_ore',15.6,1250.9,603343,50000), ('dark_core','iron_ore',3.2,10,12,0), ('dark_core','mystery_ore',1,1,1,0)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}

func TestFromDB(t *testing.T) {
	snap, err := FromDB(openTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Source != SourceDB {
		t.Errorf("source = %q", snap.Source)
	}
	// 3 systems, 2 explored; 3 deposits; types = ore/material items (2) plus
	// the undeclared mystery_ore seen in a deposit (3).
	want := Summary{Types: 3, Deposits: 3, Systems: 3, Explored: 2}
	if snap.Summary != want {
		t.Errorf("summary = %+v, want %+v", snap.Summary, want)
	}
	if len(snap.Types) != 3 || snap.Types[0].ID != "iron_ore" || snap.Types[1].ID != "mystery_ore" || snap.Types[2].ID != "void_crystal" {
		t.Errorf("types = %+v", snap.Types)
	}
	if snap.Types[1].Name != "mystery_ore" || snap.Types[1].Category != "" {
		t.Errorf("undeclared type should fall back to its id: %+v", snap.Types[1])
	}
	if len(snap.Deposits) != 3 {
		t.Fatalf("deposits = %+v", snap.Deposits)
	}
	// Values are rounded the way the page prints them: richness %.0f,
	// remaining truncated to an integer.
	d := snap.Deposits[1] // iron_ore/sol/sol_belt sorts after iron_ore/dark
	if d.SystemID != "sol" || d.Richness != 16 || d.Remaining != 1250 || !d.Station || d.Hidden || d.LastTick != 603343 || d.MaxRemaining != 50000 {
		t.Errorf("sol deposit = %+v", d)
	}
	if d.SupportedPower() != 62 || d.MaxSupportedPower() != 2500 {
		t.Errorf("power = %d / %d, want 62 / 2500", d.SupportedPower(), d.MaxSupportedPower())
	}
	if p := snap.Deposits[0]; p.MaxRemaining != 0 || p.MaxSupportedPower() != 0 || p.SupportedPower() != 0 {
		t.Errorf("unknown capacity deposit = %+v (power %d)", p, p.SupportedPower())
	}
	d = snap.Deposits[0]
	if d.SystemID != "dark" || !d.Hidden || d.Station || d.Richness != 3 {
		t.Errorf("dark deposit = %+v", d)
	}
}
