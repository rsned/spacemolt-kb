package main

import (
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newCraftingFixture builds an in-memory crafting DB with the subset of the
// schema loadRecipes reads.
func newCraftingFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open crafting fixture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE items (id TEXT PRIMARY KEY, name TEXT, category TEXT)`,
		`CREATE TABLE recipes (
			id TEXT PRIMARY KEY, name TEXT, description TEXT, category TEXT,
			crafting_time REAL, facility_only INTEGER DEFAULT 0)`,
		`CREATE TABLE recipe_inputs (recipe_id TEXT, item_id TEXT, quantity INTEGER)`,
		`CREATE TABLE recipe_outputs (recipe_id TEXT, item_id TEXT, quantity INTEGER)`,

		`INSERT INTO items VALUES ('iron_ore','Iron Ore','ore')`,
		`INSERT INTO items VALUES ('steel_plate','Steel Plate','material')`,
		`INSERT INTO items VALUES ('copper_wire','Copper Wire','material')`,

		// refine_steel: facility_only, has a public line (in the facilities fixture).
		`INSERT INTO recipes VALUES ('refine_steel','Refine Steel','','Refining',12.5,1)`,
		`INSERT INTO recipe_inputs VALUES ('refine_steel','iron_ore',5)`,
		`INSERT INTO recipe_outputs VALUES ('refine_steel','steel_plate',2)`,

		// gap_recipe: facility_only, NO public line -> belongs in the gap table.
		`INSERT INTO recipes VALUES ('gap_recipe','Gap Recipe','','Weapons',30,1)`,
		`INSERT INTO recipe_outputs VALUES ('gap_recipe','copper_wire',1)`,

		// hand_recipe: not facility_only, NO public line -> "no facility required".
		`INSERT INTO recipes VALUES ('hand_recipe','Hand Recipe','','Components',3,0)`,
		`INSERT INTO recipe_outputs VALUES ('hand_recipe','copper_wire',4)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	return db
}

func TestLoadRecipesReadsFacilityOnly(t *testing.T) {
	db := newCraftingFixture(t)

	recipes, err := loadRecipes(db)
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}

	// facility_only must come from the DB, without the catalog overlay having run.
	if !recipes["refine_steel"].FacilityOnly {
		t.Error("refine_steel.FacilityOnly = false, want true (from DB column)")
	}
	if !recipes["gap_recipe"].FacilityOnly {
		t.Error("gap_recipe.FacilityOnly = false, want true (from DB column)")
	}
	if recipes["hand_recipe"].FacilityOnly {
		t.Error("hand_recipe.FacilityOnly = true, want false")
	}
}

// newKnowledgeFixture builds an in-memory knowledge DB covering every join
// shape and degradation case loadPublicFacilities must handle.
func newKnowledgeFixture(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open knowledge fixture: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stmts := []string{
		`CREATE TABLE public_facilities (
			station_id TEXT NOT NULL, facility_id TEXT NOT NULL,
			recipe_id TEXT NOT NULL DEFAULT '', facility_name TEXT DEFAULT '',
			category TEXT DEFAULT '', level INTEGER DEFAULT 1,
			rental_fee_per_run INTEGER DEFAULT 0, owner_faction TEXT DEFAULT '',
			public INTEGER DEFAULT 1, details_json TEXT DEFAULT '',
			last_seen_tick INTEGER DEFAULT 0, last_seen_utc TEXT DEFAULT '',
			PRIMARY KEY (station_id, facility_id))`,
		`CREATE TABLE bases (id TEXT PRIMARY KEY, poi_id TEXT, name TEXT)`,
		`CREATE TABLE pois (id TEXT PRIMARY KEY, system_id TEXT, name TEXT, type TEXT)`,
		`CREATE TABLE systems (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE factions (faction_id TEXT PRIMARY KEY, name TEXT, tag TEXT)`,

		`INSERT INTO systems VALUES ('sol','Sol')`,
		`INSERT INTO systems VALUES ('starfall','Starfall')`,
		`INSERT INTO pois VALUES ('sol_central','sol','Sol Central','station')`,
		`INSERT INTO pois VALUES ('starfall_salvage_station','starfall','Starfall Salvage Station','station')`,
		// Base ID differs from POI ID -- the COALESCE(b.poi_id, ...) path.
		`INSERT INTO bases VALUES ('confederacy_central_command','sol_central','Confederacy Central Command')`,
		// Base ID equals POI ID -- the direct path.
		`INSERT INTO bases VALUES ('starfall_salvage_station','starfall_salvage_station','Starfall Salvage Station')`,
		`INSERT INTO factions VALUES ('fac_known','Hex Collective','HEXC')`,

		// Row 1: fully resolvable, base-ID station.
		`INSERT INTO public_facilities VALUES (
			'confederacy_central_command','f1','refine_steel','Salvage Smelter','production',
			2, 40, 'fac_known', 1,
			'{"type":"salvage_smelter","production":{"items_per_hour":7666,"output_per_run":2}}',
			1305967,'')`,
		// Row 2: POI-ID station, unresolved faction hash.
		`INSERT INTO public_facilities VALUES (
			'starfall_salvage_station','f2','refine_steel','Frost Furnace','production',
			1, 22, 'fac_unknown_hash', 1,
			'{"type":"frost_furnace","production":{"items_per_hour":6272,"output_per_run":4}}',
			1305967,'')`,
		// Row 3: malformed details_json -- must degrade, not fail.
		`INSERT INTO public_facilities VALUES (
			'confederacy_central_command','f3','hand_recipe','Broken Line','production',
			1, 5, 'fac_known', 1, 'not-json{{{', 1305966,'')`,
		// Row 4: public = 0 -- must be filtered out entirely.
		`INSERT INTO public_facilities VALUES (
			'confederacy_central_command','f4','gap_recipe','Private Line','production',
			3, 999, 'fac_known', 0,
			'{"type":"private_lab","production":{"items_per_hour":1,"output_per_run":1}}',
			1305967,'')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	return db
}

func TestLoadPublicFacilities(t *testing.T) {
	facs, err := loadPublicFacilities(newKnowledgeFixture(t))
	if err != nil {
		t.Fatalf("loadPublicFacilities: %v", err)
	}

	// Row 4 is public=0 and must not appear.
	if len(facs) != 3 {
		t.Fatalf("got %d facilities, want 3 (public=0 row must be filtered)", len(facs))
	}
	for _, f := range facs {
		if f.FacilityID == "f4" {
			t.Fatal("private facility f4 leaked onto the page")
		}
	}

	byID := map[string]PublicFacility{}
	for _, f := range facs {
		byID[f.FacilityID] = f
	}

	// Row 1: base ID resolves through bases.poi_id to a different POI.
	f1 := byID["f1"]
	if f1.StationName != "Confederacy Central Command" {
		t.Errorf("f1.StationName = %q, want Confederacy Central Command", f1.StationName)
	}
	if f1.SystemID != "sol" || f1.SystemName != "Sol" {
		t.Errorf("f1 system = %q/%q, want sol/Sol", f1.SystemID, f1.SystemName)
	}
	if f1.FacilityType != "salvage_smelter" {
		t.Errorf("f1.FacilityType = %q, want salvage_smelter", f1.FacilityType)
	}
	if f1.ItemsPerHour != 7666 || f1.QtyPerRun != 2 {
		t.Errorf("f1 production = %d/hr qty %d, want 7666/2", f1.ItemsPerHour, f1.QtyPerRun)
	}
	if f1.OwnerName != "Hex Collective" || f1.OwnerTag != "HEXC" {
		t.Errorf("f1 owner = %q/%q, want Hex Collective/HEXC", f1.OwnerName, f1.OwnerTag)
	}

	// Row 2: station_id is itself a POI id.
	f2 := byID["f2"]
	if f2.SystemID != "starfall" {
		t.Errorf("f2.SystemID = %q, want starfall", f2.SystemID)
	}
	// Unresolved faction: OwnerName stays empty, OwnerID is preserved for display.
	if f2.OwnerName != "" {
		t.Errorf("f2.OwnerName = %q, want empty (faction does not resolve)", f2.OwnerName)
	}
	if f2.OwnerID != "fac_unknown_hash" {
		t.Errorf("f2.OwnerID = %q, want fac_unknown_hash", f2.OwnerID)
	}

	// Row 3: malformed JSON degrades to zeroed throughput, row still present.
	f3 := byID["f3"]
	if f3.FacilityType != "" || f3.ItemsPerHour != 0 || f3.QtyPerRun != 0 {
		t.Errorf("f3 should have zeroed production from bad JSON, got type=%q %d/hr qty=%d",
			f3.FacilityType, f3.ItemsPerHour, f3.QtyPerRun)
	}
	if f3.FacilityName != "Broken Line" {
		t.Errorf("f3.FacilityName = %q, want Broken Line (row must survive bad JSON)", f3.FacilityName)
	}
}

func TestLoadPublicFacilitiesMissingTable(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = loadPublicFacilities(db)
	if !errors.Is(err, errNoPublicFacilities) {
		t.Fatalf("err = %v, want errNoPublicFacilities", err)
	}
}
