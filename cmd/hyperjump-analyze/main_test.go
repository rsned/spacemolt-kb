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
