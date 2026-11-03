package prodcost

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

const hullBaseSchema = `
CREATE TABLE factions (faction_id TEXT PRIMARY KEY, tag TEXT);
CREATE TABLE ship_listings (station_id TEXT, class_id TEXT, price INTEGER, captured_at TEXT);
INSERT INTO factions VALUES ('f-crft', 'CRFT');
`

const hullQuoteSchema = `
CREATE TABLE shipyard_quotes (id INTEGER PRIMARY KEY AUTOINCREMENT, ship_class TEXT NOT NULL, station_id TEXT NOT NULL DEFAULT '', faction_id TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '', mode TEXT NOT NULL DEFAULT 'quote', labor INTEGER NOT NULL DEFAULT 0, material_cost INTEGER NOT NULL DEFAULT 0, provide_materials_total INTEGER NOT NULL DEFAULT 0, yard_margin INTEGER NOT NULL DEFAULT 0, shipyard_tier_here INTEGER NOT NULL DEFAULT 0, royalty INTEGER, tick INTEGER NOT NULL DEFAULT 0, seen_at TEXT NOT NULL, raw TEXT NOT NULL DEFAULT '');
CREATE TABLE ship_licenses (faction_id TEXT NOT NULL, ship_class TEXT NOT NULL, empire TEXT NOT NULL DEFAULT '', agent_id TEXT NOT NULL DEFAULT '', cost_paid INTEGER NOT NULL DEFAULT 0, tick INTEGER NOT NULL DEFAULT 0, seen_at TEXT NOT NULL, PRIMARY KEY (faction_id, ship_class));
`

func openHullDB(t *testing.T, schemas ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, s := range schemas {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestLoadHullDataMissingTables(t *testing.T) {
	db := openHullDB(t, hullBaseSchema)
	hd, err := LoadHullData(db, "CRFT", nil, time.Now())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(hd.Quotes) != 0 || len(hd.Licensed) != 0 {
		t.Errorf("want empty, got %+v", hd)
	}
}

func TestLoadHullDataReadsAll(t *testing.T) {
	db := openHullDB(t, hullBaseSchema, hullQuoteSchema, `
INSERT INTO shipyard_quotes (ship_class, station_id, mode, labor, royalty, tick, seen_at) VALUES
 ('congregation', 'st1', 'quote', 5000, NULL, 100, '2026-11-02T00:00:00Z'),
 ('congregation', 'st2', 'commission', 6000, 723, 200, '2026-11-02T01:00:00Z');
INSERT INTO ship_licenses (faction_id, ship_class, seen_at) VALUES
 ('f-crft', 'congregation', '2026-11-02T00:00:00Z'),
 ('f-other', 'cobble', '2026-11-02T00:00:00Z');
INSERT INTO ship_listings VALUES
 ('stA', 'congregation', 13000, '2026-11-02T18:00:00Z'),
 ('stB', 'congregation', 12202, '2026-11-01T18:00:00Z'),
 ('stC', 'congregation', 9000, '2026-10-20T18:00:00Z');
`)
	now := time.Date(2026, 11, 2, 19, 0, 0, 0, time.UTC)
	hd, err := LoadHullData(db, "CRFT", map[string]int{"congregation": 3}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(hd.Quotes) != 2 {
		t.Fatalf("quotes = %d", len(hd.Quotes))
	}
	q0, q1 := hd.Quotes[0], hd.Quotes[1]
	if q0.Royalty != nil || q0.Tier != 3 || q0.Labor != 5000 || q0.StationID != "st1" || q0.Tick != 100 {
		t.Errorf("q0 = %+v", q0)
	}
	if q1.Royalty == nil || *q1.Royalty != 723 || q1.Mode != "commission" {
		t.Errorf("q1 = %+v", q1)
	}
	if !hd.Licensed["congregation"] || hd.Licensed["cobble"] {
		t.Errorf("licensed = %v", hd.Licensed)
	}
	if got, want := hd.Listings["congregation"], (ListingMin{Price: 12202, Count: 2, StationID: "stB"}); got != want {
		t.Errorf("listing = %+v, want %+v", got, want)
	}
}
