package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLoadSystems(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE TABLE systems (
		id TEXT PRIMARY KEY, name TEXT NOT NULL,
		position_x REAL NOT NULL, position_y REAL NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE pois (
		id TEXT PRIMARY KEY, system_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
		position_x REAL NOT NULL, position_y REAL NOT NULL)`); err != nil {
		t.Fatalf("create pois: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO systems (id,name,position_x,position_y) VALUES
		('sol','Sol',0,0), ('alpha','Alpha',100,200)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := loadSystems(db)
	if err != nil {
		t.Fatalf("loadSystems: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d systems, want 2", len(got))
	}
	// Ordered by id: alpha then sol.
	if got[0].ID != "alpha" || got[0].Pos.X != 100 || got[0].Pos.Y != 200 {
		t.Errorf("got[0] = %+v, want alpha at (100,200)", got[0])
	}
	if got[1].ID != "sol" || got[1].Name != "Sol" {
		t.Errorf("got[1] = %+v, want sol/Sol", got[1])
	}
}

func TestLoadSystems_hasStation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	mustExec(t, db, `CREATE TABLE systems (
		id TEXT PRIMARY KEY, name TEXT NOT NULL,
		position_x REAL NOT NULL, position_y REAL NOT NULL)`)
	mustExec(t, db, `CREATE TABLE pois (
		id TEXT PRIMARY KEY, system_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
		position_x REAL NOT NULL, position_y REAL NOT NULL)`)
	mustExec(t, db, `INSERT INTO systems (id,name,position_x,position_y) VALUES
		('withstation','With',0,0), ('nostation','Without',100,0)`)
	mustExec(t, db, `INSERT INTO pois (id,system_id,name,type,position_x,position_y) VALUES
		('p1','withstation','Dock','station',0,0),
		('p2','nostation','Rock','asteroid_belt',0,0)`)

	got, err := loadSystems(db)
	if err != nil {
		t.Fatalf("loadSystems: %v", err)
	}
	byID := map[string]bool{}
	for _, s := range got {
		byID[s.ID] = s.HasStation
	}
	if !byID["withstation"] {
		t.Errorf("withstation HasStation = false, want true")
	}
	if byID["nostation"] {
		t.Errorf("nostation HasStation = true, want false")
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}
