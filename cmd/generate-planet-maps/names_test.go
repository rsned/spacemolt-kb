package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// Planet pages (generate-items-kb) name the texture after the system ID, so the
// batch generator must too: GSC catalog systems have IDs (gsc_0001) that differ
// from their names (GSC-0001).
func TestTextureFilenameUsesSystemID(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, q := range []string{
		`CREATE TABLE systems (id TEXT, name TEXT)`,
		`CREATE TABLE pois (system_id TEXT, name TEXT, type TEXT, class TEXT)`,
		`INSERT INTO systems VALUES ('gsc_0001', 'GSC-0001'), ('sol', 'Sol')`,
		`INSERT INTO pois VALUES ('gsc_0001', 'GSC-0001 I', 'planet', 'terran'),
		                         ('sol', 'Earth', 'planet', 'terran'),
		                         ('sol', 'Earth Station', 'station', '')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	planets, err := loadPlanets(db, "")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range planets {
		got = append(got, textureFilename(p))
	}
	want := []string{"gsc_0001_gsc-0001_i.png", "sol_earth.png"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d = %q, want %q", i, got[i], want[i])
		}
	}

	// -system still selects by the human-readable name.
	one, err := loadPlanets(db, "GSC-0001")
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || textureFilename(one[0]) != "gsc_0001_gsc-0001_i.png" {
		t.Errorf("-system GSC-0001 = %+v", one)
	}
}
