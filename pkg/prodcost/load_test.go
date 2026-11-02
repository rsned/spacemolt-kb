package prodcost

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

const loadSchema = `
CREATE TABLE public_facilities (
	station_id     TEXT NOT NULL,
	facility_id    TEXT NOT NULL,
	recipe_id      TEXT NOT NULL DEFAULT '',
	facility_name  TEXT DEFAULT '',
	category       TEXT DEFAULT '',
	level          INTEGER DEFAULT 1,
	rental_fee_per_run     INTEGER DEFAULT 0,
	owner_faction  TEXT DEFAULT '',
	public         INTEGER DEFAULT 1,
	details_json   TEXT DEFAULT '',
	last_seen_tick INTEGER DEFAULT 0,
	last_seen_utc  TEXT DEFAULT '',
	PRIMARY KEY (station_id, facility_id)
);
CREATE TABLE faction_facilities (
	faction_id    TEXT NOT NULL,
	base_id       TEXT NOT NULL,
	facility_id   TEXT NOT NULL,
	facility_type TEXT,
	category      TEXT,
	level         INTEGER NOT NULL DEFAULT 0,
	status        TEXT,
	recipe_id     TEXT,
	details_json  TEXT,
	captured_utc  TEXT NOT NULL,
	PRIMARY KEY (faction_id, base_id, facility_id)
);
CREATE TABLE factions (
	faction_id      TEXT PRIMARY KEY,
	name            TEXT,
	tag             TEXT,
	leader_id       TEXT,
	leader_username TEXT,
	treasury        INTEGER,
	member_count    INTEGER,
	owned_bases     INTEGER,
	description     TEXT,
	charter         TEXT,
	emblem          TEXT,
	primary_color   TEXT,
	secondary_color TEXT,
	founded_utc     TEXT,
	intel_systems   INTEGER,
	intel_trade     INTEGER,
	captured_utc    TEXT NOT NULL
);
CREATE TABLE bases (
	id TEXT PRIMARY KEY,
	poi_id TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT,
	empire TEXT,
	defense_level INTEGER DEFAULT 0,
	has_drones BOOLEAN DEFAULT 0,
	public_access BOOLEAN DEFAULT 1,
	last_updated_tick INTEGER DEFAULT 0, story TEXT DEFAULT '', pirate_rep_required INTEGER DEFAULT 0, condition TEXT DEFAULT '', condition_text TEXT DEFAULT '', satisfaction_pct INTEGER DEFAULT 0, satisfied_count INTEGER DEFAULT 0, total_service_infra INTEGER DEFAULT 0, type TEXT NOT NULL DEFAULT '', wrecked INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (poi_id) REFERENCES pois(id) ON DELETE CASCADE
);
INSERT INTO bases (id, poi_id, name) VALUES ('b_emp', 'p1', 'Sol Central'), ('b_fac', 'p2', 'Haven Dock'), ('b_crft', 'p3', 'Conjunction Junction');
INSERT INTO factions (faction_id, name, tag, captured_utc) VALUES ('f_crft', 'Craft', 'CRFT', 'x'), ('f_other', 'Other', 'OTH', 'x');
INSERT INTO public_facilities (station_id, facility_id, recipe_id, level, rental_fee_per_run, owner_faction, details_json) VALUES
 ('b_emp', 'fa1', 'refine_steel', 1, 9, '', ''),
 ('b_fac', 'fa2', 'refine_steel', 2, 7, 'f_other', '{"production":{"ticks_per_run":0.5}}'),
 ('b_crft', 'fa3', 'refine_steel', 3, 4, 'f_crft', '{"production":{"ticks_per_run":0.1}}');
INSERT INTO faction_facilities (faction_id, base_id, facility_id, category, level, recipe_id, captured_utc) VALUES
 ('f_crft', 'b_crft', 'fa3', 'production', 3, 'refine_steel', 'x'),
 ('f_crft', 'b_crft', 'fa4', 'storage', 1, '', 'x');
`

func writeLoadFixture(t *testing.T) (string, *sql.DB) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "snap")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, items any) {
		raw, err := json.Marshal(map[string]any{"items": items})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("catalog_items.json", []catalog.Item{{ID: "steel_plate"}})
	write("catalog_recipes.json", []catalog.Recipe{{ID: "refine_steel", CraftingTime: 0.54,
		Outputs: []catalog.Material{{ItemID: "steel_plate", Quantity: 2}}}})
	write("catalog_facilities.json", []catalog.Facility{{ID: "iron_refinery", Level: 1, LaborCost: 10}})

	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(loadSchema); err != nil {
		t.Fatal(err)
	}
	return root, db
}

func TestLoadWorldFromSQLite(t *testing.T) {
	root, db := writeLoadFixture(t)

	w, err := LoadWorld(root, db, "CRFT")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Lines) != 3 || len(w.Items) != 1 || len(w.Recipes) != 1 || len(w.Facilities) != 1 {
		t.Fatalf("got %d lines, %d items, %d recipes, %d facs", len(w.Lines), len(w.Items), len(w.Recipes), len(w.Facilities))
	}
	var emp, fac, crft *Line
	for i := range w.Lines {
		switch w.Lines[i].Kind {
		case KindCRFT:
			crft = &w.Lines[i]
		case KindFaction:
			fac = &w.Lines[i]
		default:
			emp = &w.Lines[i]
		}
	}
	if emp == nil || fac == nil || crft == nil {
		t.Fatalf("missing kinds: %+v", w.Lines)
	}
	if !emp.Empire || emp.Fee != 9 || emp.StationName != "Sol Central" {
		t.Errorf("empire %+v", *emp)
	}
	if fac.Empire || fac.FactionTag != "OTH" || fac.TicksPerRun != 0.5 || fac.Fee != 7 || fac.StationName != "Haven Dock" {
		t.Errorf("faction %+v", *fac)
	}
	if crft.FactionTag != "CRFT" || crft.Fee != 0 || crft.TicksPerRun != 0 || crft.Level != 3 || crft.StationName != "Conjunction Junction" {
		t.Errorf("crft %+v", *crft)
	}
	// The CRFT facility is also in public_facilities: it must appear only once, as crft.
	n := 0
	for _, l := range w.Lines {
		if l.StationID == "b_crft" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("CRFT facility appears %d times", n)
	}
	routes := Routes(w.Recipes[0], w.Facilities, w.Lines)
	if _, ok := find(routes, KindCRFT, 3); !ok {
		t.Errorf("no crft route: %+v", routes)
	}

	pub, err := LoadWorld(root, db, "")
	if err != nil {
		t.Fatal(err)
	}
	// Public build: empire + OTH faction + the CRFT-owned facility as a plain faction line.
	if len(pub.Lines) != 3 {
		t.Fatalf("public lines %d", len(pub.Lines))
	}
	for _, l := range pub.Lines {
		if l.Kind == KindCRFT {
			t.Errorf("crft line in public build: %+v", l)
		}
		if l.StationID == "b_crft" && (l.Kind != KindFaction || l.TicksPerRun != 0.1 || l.FactionTag != "CRFT") {
			t.Errorf("crft-owned public line %+v", l)
		}
	}
}

func TestLoadWorldKeepsOrphanCRFTPublicLine(t *testing.T) {
	root, db := writeLoadFixture(t)
	if _, err := db.Exec(`INSERT INTO public_facilities (station_id, facility_id, recipe_id, level, rental_fee_per_run, owner_faction, details_json)
VALUES ('b_fac', 'fa9', 'refine_steel', 1, 5, 'f_crft', '')`); err != nil {
		t.Fatal(err)
	}
	w, err := LoadWorld(root, db, "CRFT")
	if err != nil {
		t.Fatal(err)
	}
	var orphan *Line
	for i := range w.Lines {
		if w.Lines[i].StationID == "b_fac" && w.Lines[i].Fee == 5 {
			orphan = &w.Lines[i]
		}
	}
	if orphan == nil || orphan.Kind != KindFaction || orphan.FactionTag != "CRFT" {
		t.Fatalf("orphan CRFT-owned public line dropped or mis-kinded: %+v", w.Lines)
	}
	if len(w.Lines) != 4 {
		t.Errorf("got %d lines, want 4", len(w.Lines))
	}
}
