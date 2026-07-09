package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		// No bases row for starfall_salvage_station -- the COALESCE falls back to pf.station_id.
		// This tests the fallback path where station_id is a pois.id with no bases entry.
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

	// Row 2: station_id is itself a POI id with no bases row, testing COALESCE fallback.
	f2 := byID["f2"]
	if f2.SystemID != "starfall" {
		t.Errorf("f2.SystemID = %q, want starfall", f2.SystemID)
	}
	// Station name resolved through pois.name via COALESCE fallback.
	if f2.StationName != "Starfall Salvage Station" {
		t.Errorf("f2.StationName = %q, want Starfall Salvage Station (from pois fallback)", f2.StationName)
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

func TestGroupByRecipe(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}
	facs, err := loadPublicFacilities(newKnowledgeFixture(t))
	if err != nil {
		t.Fatalf("loadPublicFacilities: %v", err)
	}

	groups := groupByRecipe(facs, recipes)

	// Two distinct recipe_ids among the 3 public rows: refine_steel, hand_recipe.
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	// Sorted by recipe name: "Hand Recipe" < "Refine Steel".
	if groups[0].RecipeName != "Hand Recipe" || groups[1].RecipeName != "Refine Steel" {
		t.Fatalf("groups not sorted by name: %q, %q", groups[0].RecipeName, groups[1].RecipeName)
	}

	rs := groups[1]
	if len(rs.Facilities) != 2 {
		t.Fatalf("refine_steel has %d facilities, want 2", len(rs.Facilities))
	}
	// Facilities within a group sort by station name: Confederacy < Starfall.
	if rs.Facilities[0].StationName != "Confederacy Central Command" {
		t.Errorf("first facility station = %q, want Confederacy Central Command",
			rs.Facilities[0].StationName)
	}
	if !rs.FacilityOnly {
		t.Error("refine_steel.FacilityOnly = false, want true")
	}
	if rs.RecipeCategory != "Refining" {
		t.Errorf("refine_steel.RecipeCategory = %q, want Refining", rs.RecipeCategory)
	}
	if len(rs.Outputs) != 1 || rs.Outputs[0].ItemID != "steel_plate" {
		t.Errorf("refine_steel outputs = %+v, want one steel_plate", rs.Outputs)
	}
}

func TestGroupByStation(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}
	facs, err := loadPublicFacilities(newKnowledgeFixture(t))
	if err != nil {
		t.Fatalf("loadPublicFacilities: %v", err)
	}

	stations := groupByStation(facs, recipes)

	if len(stations) != 2 {
		t.Fatalf("got %d stations, want 2", len(stations))
	}
	// Ordered by facility count descending: CCC has 2, Starfall has 1.
	if stations[0].StationID != "confederacy_central_command" || stations[0].Count != 2 {
		t.Fatalf("first station = %s (%d facilities), want confederacy_central_command (2)",
			stations[0].StationID, stations[0].Count)
	}
	if stations[1].Count != 1 {
		t.Errorf("second station count = %d, want 1", stations[1].Count)
	}

	// CCC's two facilities are in different recipe categories -> two blocks,
	// sorted by category name: Components < Refining.
	ccc := stations[0]
	if len(ccc.Categories) != 2 {
		t.Fatalf("CCC has %d category blocks, want 2", len(ccc.Categories))
	}
	if ccc.Categories[0].Category != "Components" || ccc.Categories[1].Category != "Refining" {
		t.Errorf("CCC categories = %q, %q; want Components, Refining",
			ccc.Categories[0].Category, ccc.Categories[1].Category)
	}
	// Fee range spans both facilities: 5 (Broken Line) .. 40 (Salvage Smelter).
	if ccc.FeeMin != 5 || ccc.FeeMax != 40 {
		t.Errorf("CCC fee range = %d..%d, want 5..40", ccc.FeeMin, ccc.FeeMax)
	}
	// Recipe metadata is attached to each station facility.
	if ccc.Categories[1].Facilities[0].RecipeName != "Refine Steel" {
		t.Errorf("RecipeName = %q, want Refine Steel",
			ccc.Categories[1].Facilities[0].RecipeName)
	}
}

func TestSplitNoFacilityRecipes(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}
	// refine_steel and hand_recipe have public lines; gap_recipe does not.
	covered := map[string]bool{"refine_steel": true, "hand_recipe": true}

	facilityOnly, noFacilityNeeded := splitNoFacilityRecipes(recipes, covered)

	// gap_recipe is facility_only with no line -> the gap table.
	if len(facilityOnly) != 1 || facilityOnly[0].ID != "gap_recipe" {
		t.Fatalf("facilityOnly = %+v, want [gap_recipe]", facilityOnly)
	}
	if facilityOnly[0].Category != "Weapons" || facilityOnly[0].DirName != "Weapons" {
		t.Errorf("gap_recipe category/dir = %q/%q, want Weapons/Weapons",
			facilityOnly[0].Category, facilityOnly[0].DirName)
	}
	if facilityOnly[0].OutputID != "copper_wire" || facilityOnly[0].OutputQty != 1 {
		t.Errorf("gap_recipe output = %s x%d, want copper_wire x1",
			facilityOnly[0].OutputID, facilityOnly[0].OutputQty)
	}

	// Covered recipes appear in NEITHER table.
	if len(noFacilityNeeded) != 0 {
		t.Fatalf("noFacilityNeeded = %+v, want empty (both non-gap recipes are covered)", noFacilityNeeded)
	}

	// Now uncover hand_recipe: it is not facility_only, so it lands in the
	// second table, never the first.
	facilityOnly, noFacilityNeeded = splitNoFacilityRecipes(recipes, map[string]bool{"refine_steel": true})
	if len(noFacilityNeeded) != 1 || noFacilityNeeded[0].ID != "hand_recipe" {
		t.Fatalf("noFacilityNeeded = %+v, want [hand_recipe]", noFacilityNeeded)
	}
	if len(facilityOnly) != 1 || facilityOnly[0].ID != "gap_recipe" {
		t.Fatalf("facilityOnly = %+v, want [gap_recipe] still", facilityOnly)
	}

	// The two tables must never overlap and never lose a recipe.
	seen := map[string]bool{}
	for _, r := range append(append([]NoFacilityRecipe{}, facilityOnly...), noFacilityNeeded...) {
		if seen[r.ID] {
			t.Errorf("recipe %s appears in both tables", r.ID)
		}
		seen[r.ID] = true
	}
	if len(seen) != len(recipes)-1 { // minus the one covered recipe
		t.Errorf("split covers %d recipes, want %d", len(seen), len(recipes)-1)
	}
}

func TestWriteWherePage(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}
	dir := t.TempDir()
	if err := writeWherePage(dir, newKnowledgeFixture(t), recipes); err != nil {
		t.Fatalf("writeWherePage: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "where.html"))
	if err != nil {
		t.Fatalf("read where.html: %v", err)
	}
	html := string(raw)

	for _, want := range []string{
		`id="by-recipe"`,
		`id="by-station"`,
		`id="r-refine_steel"`,                           // recipe deep-link anchor
		`id="s-confederacy_central_command"`,            // station deep-link anchor
		`../systems/sol/index.html`,                     // station -> system link
		`../facilities/production/salvage_smelter.html`, // facility -> type link
		`Refining/refine_steel.html`,                    // recipe -> recipe page link
		`../items/material/steel_plate.html`,            // output -> item link
		`Gap Recipe`,                                    // facility-only gap table
		`No Known Public Line`,
		`No Facility Required`,
		`fac_unknown_hash`, // unresolved faction renders raw
	} {
		if !strings.Contains(html, want) {
			t.Errorf("where.html missing %q", want)
		}
	}

	// The private (public=0) facility must never reach the page.
	if strings.Contains(html, "Private Line") || strings.Contains(html, "private_lab") {
		t.Error("public=0 facility leaked into rendered HTML")
	}

	// Copy rule from the spec: never claim absolute impossibility.
	for _, banned := range []string{"nowhere", "impossible"} {
		if strings.Contains(strings.ToLower(html), banned) {
			t.Errorf("page contains banned absolute-claim word %q", banned)
		}
	}
}

func TestWriteWherePageEmptyTable(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}
	db := newKnowledgeFixture(t)
	if _, err := db.Exec(`DELETE FROM public_facilities`); err != nil {
		t.Fatalf("clear table: %v", err)
	}

	dir := t.TempDir()
	if err := writeWherePage(dir, db, recipes); err != nil {
		t.Fatalf("writeWherePage on empty table: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "where.html"))
	if err != nil {
		t.Fatalf("read where.html: %v", err)
	}
	// With no lines at all, every recipe falls into one of the two dense tables.
	if !strings.Contains(string(raw), "Hand Recipe") {
		t.Error("empty-table page should still list all recipes as having no public line")
	}
}
