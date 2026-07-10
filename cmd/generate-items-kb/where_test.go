package main

import (
	"bytes"
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

// TestSourceCellStationOwned pins how the Source column renders each of the
// three owner shapes. Station-owned facilities carry no faction_id, so the
// scraper writes an empty owner_faction; they must render as a "Station" badge
// rather than an empty <code> box.
//
// This is forward-compatible coverage: today public_facilities holds only
// faction-owned rented lines, but the scraper is being fixed to also capture
// the station_facilities section of the `facility list` response, where every
// row has an empty owner.
func TestSourceCellStationOwned(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}

	facs := []PublicFacility{
		// Station-owned: no owner at all.
		{StationID: "grand_exchange_station", StationName: "Grand Exchange Station",
			SystemID: "haven", SystemName: "Haven", FacilityID: "f750", FacilityName: "Iron Refinery",
			FacilityType: "iron_refinery", RecipeID: "refine_steel", Level: 1,
			FeePerRun: 9, QtyPerRun: 2, ItemsPerHour: 6666},
		// Faction-owned, resolved.
		{StationID: "grand_exchange_station", StationName: "Grand Exchange Station",
			SystemID: "haven", SystemName: "Haven", FacilityID: "fx", FacilityName: "Salvage Smelter",
			FacilityType: "salvage_smelter", RecipeID: "refine_steel", Level: 2,
			FeePerRun: 40, OwnerID: "fac_known", OwnerName: "Hex Collective", OwnerTag: "HEXC"},
		// Faction-owned, unresolved hash.
		{StationID: "grand_exchange_station", StationName: "Grand Exchange Station",
			SystemID: "haven", SystemName: "Haven", FacilityID: "fy", FacilityName: "Frost Furnace",
			FacilityType: "frost_furnace", RecipeID: "hand_recipe", Level: 1,
			FeePerRun: 22, OwnerID: "8d6cdcb7026e799a00fea973d56d8ada"},
	}

	covered := map[string]bool{"refine_steel": true, "hand_recipe": true}
	facilityOnly, noFacility := splitNoFacilityRecipes(recipes, covered)
	data := wherePageData{
		StationCount: 1, FacilityCount: len(facs), RecipesCovered: 2,
		FacilityOnlyGap: len(facilityOnly), LastSeenTick: 1306917,
		RecipeGroups:     groupByRecipe(facs, recipes),
		StationGroups:    groupByStation(facs, recipes),
		FacilityOnlyNone: facilityOnly, NoFacilityNeeded: noFacility,
	}

	var buf strings.Builder
	if err := renderWherePage(&buf, data); err != nil {
		t.Fatalf("renderWherePage: %v", err)
	}
	html := buf.String()

	// A station-owned row must say "Station", not render an empty code box.
	if strings.Contains(html, `<code title=""></code>`) {
		t.Error("station-owned facility rendered an empty <code> box in the Source column")
	}
	if want := `<span class="badge">Station Facility</span>`; !strings.Contains(html, want) {
		t.Errorf("missing Station Facility badge for owner-less facility; want %q", want)
	}
	// Every facility table carries the renamed header. One table renders per
	// recipe group and per station category block, so the count tracks the
	// fixture's shape; what matters is that some render and none say "Owner".
	if n := strings.Count(html, "<th class=\"sortable\">Source</th>"); n < 2 {
		t.Errorf("Source column header appears %d times, want at least 2 (by-recipe + by-station)", n)
	}
	if strings.Contains(html, "<th class=\"sortable\">Owner</th>") {
		t.Error("stale Owner column header still present")
	}
	// Resolved and unresolved faction rendering both survive.
	if !strings.Contains(html, `../factions/hexc/index.html`) {
		t.Error("resolved faction should still link to its lowercased-tag page")
	}
	if !strings.Contains(html, `8d6cdcb7`) {
		t.Error("unresolved faction should still render its truncated hash")
	}
	if strings.Contains(html, `href=""`) {
		t.Error("page emitted an empty href")
	}
}

// TestGroupByRecipeFacilityOrder pins the ordering of facilities within a
// recipe section: station name, then cheapest fee, then highest throughput,
// then faction name. StationID/FacilityID close the sort so it stays
// deterministic -- the rendered HTML is committed to git.
func TestGroupByRecipeFacilityOrder(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}

	mk := func(id, station string, fee, iph int, owner string) PublicFacility {
		return PublicFacility{
			StationID: station, StationName: station, FacilityID: id,
			RecipeID: "refine_steel", FeePerRun: fee, ItemsPerHour: iph,
			OwnerName: owner, OwnerID: owner,
		}
	}
	// Deliberately shuffled input.
	facs := []PublicFacility{
		mk("f6", "bravo", 5, 100, "Zeta"),
		mk("f2", "alpha", 10, 500, "Acme"),
		mk("f5", "alpha", 10, 500, "Acme"), // full tie with f2 -> FacilityID decides
		mk("f1", "alpha", 5, 100, "Acme"),
		mk("f4", "alpha", 10, 900, "Acme"), // same fee as f2, faster -> ranks before it
		mk("f3", "alpha", 10, 500, "Aardvark"),
	}

	groups := groupByRecipe(facs, recipes)
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	var got []string
	for _, f := range groups[0].Facilities {
		got = append(got, f.FacilityID)
	}

	// alpha before bravo (station name).
	// Within alpha: fee 5 (f1) first; then fee 10 sorted by items/hr desc ->
	// f4 (900), then the 500s by faction name: Aardvark (f3) before Acme
	// (f2, f5), and f2 before f5 by facility ID.
	want := []string{"f1", "f4", "f3", "f2", "f5", "f6"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// TestSummarizeOwners pins the faction/owner rollup shown above the tabs:
// one row per owning faction, plus one row for station-owned lines, sorted by
// facility count descending.
func TestSummarizeOwners(t *testing.T) {
	mk := func(id, station, recipe, owner, name, tag string, fee int) PublicFacility {
		return PublicFacility{
			StationID: station, StationName: station, FacilityID: id, RecipeID: recipe,
			FeePerRun: fee, OwnerID: owner, OwnerName: name, OwnerTag: tag,
		}
	}
	facs := []PublicFacility{
		// Hex: 3 facilities, 2 stations, 2 distinct recipes, fees 5..40
		mk("a", "alpha", "refine_steel", "hex", "Hex Collective", "HEXC", 40),
		mk("b", "bravo", "refine_steel", "hex", "Hex Collective", "HEXC", 5),
		mk("c", "bravo", "hand_recipe", "hex", "Hex Collective", "HEXC", 12),
		// Station-owned: 2 facilities, 1 station, fees 9..50
		mk("d", "voss", "refine_steel", "", "", "", 9),
		mk("e", "voss", "gap_recipe", "", "", "", 50),
		// Unresolved hash: 1 facility
		mk("f", "alpha", "hand_recipe", "8d6cdcb7026e799a00fea973d56d8ada", "", "", 22),
	}

	got := summarizeOwners(facs)
	if len(got) != 3 {
		t.Fatalf("got %d owner rows, want 3", len(got))
	}

	// Sorted by facility count desc: Hex (3), station-owned (2), unresolved (1).
	if got[0].OwnerName != "Hex Collective" || got[0].Facilities != 3 {
		t.Errorf("row 0 = %+v, want Hex Collective with 3 facilities", got[0])
	}
	if got[0].Stations != 2 || got[0].Recipes != 2 {
		t.Errorf("Hex stations/recipes = %d/%d, want 2/2", got[0].Stations, got[0].Recipes)
	}
	if got[0].FeeMin != 5 || got[0].FeeMax != 40 {
		t.Errorf("Hex fee range = %d..%d, want 5..40", got[0].FeeMin, got[0].FeeMax)
	}

	if !got[1].StationOwned || got[1].Facilities != 2 {
		t.Errorf("row 1 = %+v, want station-owned with 2 facilities", got[1])
	}
	if got[1].FeeMin != 9 || got[1].FeeMax != 50 {
		t.Errorf("station-owned fee range = %d..%d, want 9..50", got[1].FeeMin, got[1].FeeMax)
	}

	// Unresolved faction keeps its raw ID so the template can show a short hash.
	if got[2].OwnerID != "8d6cdcb7026e799a00fea973d56d8ada" || got[2].OwnerName != "" {
		t.Errorf("row 2 = %+v, want unresolved hash with empty name", got[2])
	}
	if got[2].StationOwned {
		t.Error("unresolved faction must not be marked station-owned")
	}
}

// markPriceExtremes tags the cheapest and costliest lines of a comparison set,
// ranked on cost per output unit. These cases pin the rules that keep the
// colouring meaningful: it takes at least two differently-priced lines to have
// a "best" and a "worst".
func TestMarkPriceExtremes(t *testing.T) {
	mk := func(id string, fee, qty int) PublicFacility {
		return PublicFacility{FacilityID: id, FeePerRun: fee, QtyPerRun: qty}
	}
	ranks := func(rows []PublicFacility) map[string]string {
		ptrs := make([]*PublicFacility, len(rows))
		for i := range rows {
			ptrs[i] = &rows[i]
		}
		markPriceExtremes(ptrs)
		got := make(map[string]string, len(rows))
		for _, r := range rows {
			got[r.FacilityID] = r.PriceRank
		}
		return got
	}

	tests := []struct {
		name string
		rows []PublicFacility
		want map[string]string
	}{{
		name: "ranks on fee per unit, not fee per run",
		// b is dearer per run but yields 10x, so it is the cheaper unit.
		rows: []PublicFacility{mk("a", 100, 1), mk("b", 200, 10)},
		want: map[string]string{"a": priceWorst, "b": priceBest},
	}, {
		name: "single row stays plain",
		rows: []PublicFacility{mk("a", 100, 1)},
		want: map[string]string{"a": ""},
	}, {
		name: "every row priced alike stays plain",
		rows: []PublicFacility{mk("a", 18, 3), mk("b", 18, 3), mk("c", 18, 3)},
		want: map[string]string{"a": "", "b": "", "c": ""},
	}, {
		name: "tied extremes all colour",
		rows: []PublicFacility{mk("a", 1, 1), mk("b", 9, 1), mk("c", 1, 1), mk("d", 9, 1)},
		want: map[string]string{"a": priceBest, "c": priceBest, "b": priceWorst, "d": priceWorst},
	}, {
		name: "middle rows stay plain",
		rows: []PublicFacility{mk("a", 1, 1), mk("b", 5, 1), mk("c", 9, 1)},
		want: map[string]string{"a": priceBest, "b": "", "c": priceWorst},
	}, {
		name: "unpriceable rows are skipped, not ranked",
		// qty 0 cannot yield a per-unit cost. One priced row remains -> no colour.
		rows: []PublicFacility{mk("a", 100, 0), mk("b", 50, 1)},
		want: map[string]string{"a": "", "b": ""},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ranks(tc.rows)
			for id, want := range tc.want {
				if got[id] != want {
					t.Errorf("row %s: PriceRank = %q, want %q", id, got[id], want)
				}
			}
		})
	}
}

// A station block lists many different recipes. Comparing a bolt against a
// warship module is meaningless, so extremes are found per recipe, not across
// the whole block.
func TestGroupByStationPriceRankIsPerRecipe(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}
	mk := func(id, recipe string, fee, qty int) PublicFacility {
		return PublicFacility{
			StationID: "ccc", StationName: "CCC", FacilityID: id,
			RecipeID: recipe, FeePerRun: fee, QtyPerRun: qty,
		}
	}
	// Two duplicate lines per recipe at one station. refine_steel is dear in
	// absolute terms; if extremes were taken across the block, its cheap line
	// (50/unit) would never be "best" and gap_recipe's dear line (2/unit)
	// would never be "worst".
	facs := []PublicFacility{
		mk("s1", "refine_steel", 50, 1),
		mk("s2", "refine_steel", 90, 1),
		mk("g1", "gap_recipe", 1, 1),
		mk("g2", "gap_recipe", 2, 1),
	}

	stations := groupByStation(facs, recipes)
	if len(stations) != 1 {
		t.Fatalf("got %d stations, want 1", len(stations))
	}
	got := map[string]string{}
	for _, cat := range stations[0].Categories {
		for _, f := range cat.Facilities {
			got[f.FacilityID] = f.PriceRank
		}
	}
	want := map[string]string{
		"s1": priceBest, "s2": priceWorst,
		"g1": priceBest, "g2": priceWorst,
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("row %s: PriceRank = %q, want %q", id, got[id], w)
		}
	}
}

// A recipe section lists the same recipe at many stations, so the whole table
// is one comparison set.
func TestGroupByRecipePriceRank(t *testing.T) {
	recipes, err := loadRecipes(newCraftingFixture(t))
	if err != nil {
		t.Fatalf("loadRecipes: %v", err)
	}
	mk := func(id, station string, fee, qty int) PublicFacility {
		return PublicFacility{
			StationID: station, StationName: station, FacilityID: id,
			RecipeID: "refine_steel", FeePerRun: fee, QtyPerRun: qty,
		}
	}
	facs := []PublicFacility{
		mk("f1", "alpha", 2, 2),   // 1.0/unit -> best
		mk("f2", "alpha", 200, 2), // 100.0/unit
		mk("f3", "bravo", 1000, 2),
	} // 500.0/unit -> worst

	groups := groupByRecipe(facs, recipes)
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	want := map[string]string{"f1": priceBest, "f2": "", "f3": priceWorst}
	for _, f := range groups[0].Facilities {
		if f.PriceRank != want[f.FacilityID] {
			t.Errorf("row %s: PriceRank = %q, want %q", f.FacilityID, f.PriceRank, want[f.FacilityID])
		}
	}
}

// The colour must reach the page as a class on the fee cell.
func TestRenderWherePagePriceClasses(t *testing.T) {
	data := wherePageData{
		StationCount: 1,
		RecipeGroups: []WhereRecipeGroup{{
			RecipeID: "refine_steel", RecipeName: "Refine Steel", RecipeCategory: "Refining",
			RecipeDirName: "Refining",
			Facilities: []PublicFacility{
				{StationID: "a", StationName: "Alpha", FacilityID: "f1", FeePerRun: 2, QtyPerRun: 2, PriceRank: priceBest},
				{StationID: "b", StationName: "Bravo", FacilityID: "f2", FeePerRun: 1000, QtyPerRun: 2, PriceRank: priceWorst},
				{StationID: "c", StationName: "Cesium", FacilityID: "f3", FeePerRun: 200, QtyPerRun: 2},
			},
		}},
	}
	var buf bytes.Buffer
	if err := renderWherePage(&buf, data); err != nil {
		t.Fatalf("renderWherePage: %v", err)
	}
	got := buf.String()
	// Count the rendered cells, not the stylesheet rule that also names them.
	if n := strings.Count(got, `class="num-cell fee-best"`); n != 1 {
		t.Errorf("fee-best cells = %d, want 1", n)
	}
	if n := strings.Count(got, `class="num-cell fee-worst"`); n != 1 {
		t.Errorf("fee-worst cells = %d, want 1", n)
	}
	// The unranked middle row must carry no price class.
	if n := strings.Count(got, `class="num-cell"`); n == 0 {
		t.Error("no plain num-cell rendered; the unranked row lost its class")
	}
	// The stylesheet must define both, or the classes are inert.
	for _, sel := range []string{".fee-best", ".fee-worst"} {
		if !strings.Contains(got, sel) {
			t.Errorf("stylesheet does not define %s", sel)
		}
	}
}
