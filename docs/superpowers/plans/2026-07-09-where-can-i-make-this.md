# "Where Can I Make This" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `kb/recipes/where.html`, a two-tab side page answering "which stations have a public facility for recipe X" and "what can I craft at station Y", from the new `public_facilities` table.

**Architecture:** A new `cmd/generate-items-kb/where.go` mirroring the existing `resources.go`: load rows from the knowledge DB, join them in memory against the `map[string]*Recipe` that `main()` already loaded from the crafting DB, group them two ways, render one static HTML file. Wired into `main()` through a warning-not-fatal wrapper, plus a `-where-only` flag for iteration.

**Tech Stack:** Go 1.24+, `database/sql` + `modernc.org/sqlite` (production) / `github.com/mattn/go-sqlite3` (tests), `html/template`, `github.com/dustin/go-humanize`.

**Spec:** `docs/superpowers/specs/2026-07-09-where-can-i-make-this-design.md`

## Global Constraints

- All commands run from `/home/robert/spacemolt/kb`.
- `go build ./...`, `go test ./...`, and `golangci-lint run` must all pass before every commit. No new lint findings.
- Go 1.24 idioms: `for i := range n` over integer ranges; `b.Loop()` in benchmarks.
- **Copy rule (from spec):** never say "nowhere" or "impossible". The phrase is **"no known public line"**. The 281 hand-craftable recipes are framed **"no facility required"**, never "facilities are useless here".
- **Ordering must be deterministic.** This page is committed to git; a nondeterministic sort produces a spurious diff on every regen. Stations sort by facility count descending, then name ascending. Recipes sort by name ascending. Ties always break on a stable secondary key.
- **`where.html` must be written after `writeRecipePages`.** `writeRecipePages` calls `cleanGeneratedFiles("kb/recipes")`, which deletes every `.html` in that directory. Writing `where.html` before it means writing a file that is immediately deleted.
- Page is **not** added to the global nav (`internal/kbnav`). It is linked from the recipes index only.

## File Structure

| File | Responsibility |
|---|---|
| `cmd/generate-items-kb/where.go` (create) | Row loading, grouping, split, template, `writeWherePage` |
| `cmd/generate-items-kb/where_test.go` (create) | Fixture DB, loader, grouping, split, render tests |
| `cmd/generate-items-kb/main.go` (modify) | `loadRecipes` reads `facility_only`; `generateWherePage` wrapper; `-where-only` flag; recipes-index callout link |

## Background: verified facts about the data

Do not re-derive these; they were confirmed against the live DBs on 2026-07-09.

- `public_facilities` has 247 rows, all `category='production'`, all `public=1`. 149 distinct `recipe_id`, 6 distinct `station_id`.
- All 6 `station_id` values resolve through `bases`. Three are base IDs whose `poi_id` differs from the base ID (`confederacy_central_command` → POI `sol_central`); three are POI IDs that also happen to be base IDs.
- All 149 `recipe_id` values exist in `crafting.db`.
- All 178 distinct `details_json.type` values have a page at `kb/facilities/production/<type>.html`.
- 5 of 8 `owner_faction` values resolve in `factions`. Three do not.
- 147 recipes have exactly one output row, 2 have two. None have zero.
- `crafting.db` has 666 recipes; 317 are `facility_only`. `catalog_recipes.json` agrees on all 666 with zero disagreements.
- 666 − 149 = 517 recipes have no public line; of those, 236 are `facility_only` and 281 are not.
- `last_seen_utc` is empty on all rows. `last_seen_tick` is 1305966 or 1305967.

---

### Task 1: Populate `Recipe.FacilityOnly` from the crafting DB

The entire 236/281 split depends on `Recipe.FacilityOnly`. Today that field is set **only** by `loadRecipeOverlay` reading `catalog_recipes.json`, and that call logs a warning and continues on failure (`main.go:674`). If the catalog is missing, every recipe silently becomes `FacilityOnly=false` and the new page would report a 0/517 split as if it were fact. The crafting DB has the same column with identical values, so read it there as the base and let the overlay continue to override.

**Files:**
- Modify: `cmd/generate-items-kb/main.go` (the `loadRecipes` query, ~line 2452)
- Test: `cmd/generate-items-kb/where_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `loadRecipes(db *sql.DB) (map[string]*Recipe, error)` — unchanged signature, but the returned `*Recipe` values now have `FacilityOnly` set from the DB.

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-items-kb/where_test.go`:

```go
package main

import (
	"database/sql"
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/generate-items-kb/ -run TestLoadRecipesReadsFacilityOnly -v`
Expected: FAIL — `refine_steel.FacilityOnly = false, want true (from DB column)`. It compiles (the field exists) but is never populated.

- [ ] **Step 3: Make it pass**

In `cmd/generate-items-kb/main.go`, in `loadRecipes`, change the first query and its scan. Find:

```go
	rows, err := db.Query(`SELECT id, name, COALESCE(description,''), COALESCE(category,''), crafting_time FROM recipes ORDER BY id`)
```

Replace with:

```go
	rows, err := db.Query(`SELECT id, name, COALESCE(description,''), COALESCE(category,''), crafting_time, COALESCE(facility_only, 0) FROM recipes ORDER BY id`)
```

Find:

```go
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Category, &r.CraftingTime); err != nil {
```

Replace with:

```go
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Category, &r.CraftingTime, &r.FacilityOnly); err != nil {
```

Then update the comment above the `loadRecipeOverlay` call at `main.go:672` so the next reader knows the overlay is now a refinement, not the sole source. Find:

```go
	// Overlay hidden flag from catalog JSON.
	recipeCatalogPath := filepath.Join(catalogDir, "catalog_recipes.json")
	if err := loadRecipeOverlay(recipeCatalogPath, recipes); err != nil {
		log.Printf("warning: load recipe overlay: %v (hidden flag will be omitted)", err)
	}
```

Replace with:

```go
	// Overlay hidden/no_recycle/fuel_output from catalog JSON. facility_only is
	// already loaded from the crafting DB (both sources agree); the overlay
	// re-sets it, so a missing catalog degrades the hidden flag but never
	// silently flattens facility_only, which the Where-To-Craft page splits on.
	recipeCatalogPath := filepath.Join(catalogDir, "catalog_recipes.json")
	if err := loadRecipeOverlay(recipeCatalogPath, recipes); err != nil {
		log.Printf("warning: load recipe overlay: %v (hidden flag will be omitted)", err)
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/generate-items-kb/ -run TestLoadRecipesReadsFacilityOnly -v`
Expected: PASS

- [ ] **Step 5: Verify no regression in the existing recipe pages**

Run: `go build ./... && go test ./cmd/generate-items-kb/ && golangci-lint run ./cmd/generate-items-kb/`
Expected: all pass, no new findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-items-kb/main.go cmd/generate-items-kb/where_test.go
git commit -m "fix(kb): load recipe facility_only from crafting DB, not just catalog overlay"
```

---

### Task 2: Load public facility rows

**Files:**
- Create: `cmd/generate-items-kb/where.go`
- Test: `cmd/generate-items-kb/where_test.go` (extend)

**Interfaces:**
- Consumes: `loadRecipes` from Task 1.
- Produces:
  - `type PublicFacility struct` (fields below)
  - `func loadPublicFacilities(db *sql.DB) ([]PublicFacility, error)`
  - `var errNoPublicFacilities error` — sentinel returned when the table does not exist

- [ ] **Step 1: Write the failing test**

Append to `cmd/generate-items-kb/where_test.go`:

```go
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
```

Add `"errors"` to the test file's import block.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/generate-items-kb/ -run TestLoadPublicFacilities -v`
Expected: FAIL to compile — `undefined: loadPublicFacilities`, `undefined: PublicFacility`, `undefined: errNoPublicFacilities`.

- [ ] **Step 3: Write the implementation**

Create `cmd/generate-items-kb/where.go`:

```go
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// errNoPublicFacilities is returned when the knowledge DB predates the
// public_facilities table. Callers treat this as "skip the page", not "fail
// the build" -- the table is new and older snapshots will not have it.
var errNoPublicFacilities = errors.New("public_facilities table not present")

// PublicFacility is one public production line at one station, joined to its
// station, system, and owning faction.
type PublicFacility struct {
	StationID   string
	StationName string
	SystemID    string
	SystemName  string

	FacilityID   string
	FacilityName string
	FacilityType string // details_json.type; links to kb/facilities/production/<type>.html

	RecipeID string
	Level    int

	FeePerRun    int
	QtyPerRun    int // details_json.production.output_per_run
	ItemsPerHour int // details_json.production.items_per_hour

	OwnerID   string // raw faction hash; always set
	OwnerName string // empty when the faction does not resolve
	OwnerTag  string

	LastSeenTick int
}

// facilityDetails is the narrow slice of details_json we consume. The three
// values here are the only ones not available as table columns.
//
// The numbers are decoded as float64 rather than int: the server has emitted
// fractional throughput before (ticks_per_run is fractional), and a float in
// items_per_hour would otherwise fail the whole row's unmarshal.
type facilityDetails struct {
	Type       string `json:"type"`
	Production struct {
		ItemsPerHour float64 `json:"items_per_hour"`
		OutputPerRun float64 `json:"output_per_run"`
	} `json:"production"`
}

// hasPublicFacilities reports whether the knowledge DB has the table at all.
func hasPublicFacilities(db *sql.DB) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'public_facilities'`,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// loadPublicFacilities reads every public production line, resolving each to
// its station, system, and owning faction.
//
// station_id is a bases.id for some stations and a pois.id for others, so the
// POI join goes through COALESCE(b.poi_id, pf.station_id) to accept either.
// The faction join is a LEFT JOIN because roughly a third of owner hashes do
// not resolve; those render as a bare hash rather than a broken link.
func loadPublicFacilities(db *sql.DB) ([]PublicFacility, error) {
	ok, err := hasPublicFacilities(db)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNoPublicFacilities
	}

	rows, err := db.Query(`
		SELECT pf.station_id, pf.facility_id, pf.recipe_id, pf.facility_name,
		       pf.level, pf.rental_fee_per_run, pf.owner_faction,
		       pf.details_json, pf.last_seen_tick,
		       COALESCE(b.name, ''), COALESCE(p.name, ''),
		       COALESCE(s.id, ''), COALESCE(s.name, ''),
		       COALESCE(f.name, ''), COALESCE(f.tag, '')
		FROM public_facilities pf
		LEFT JOIN bases    b ON b.id = pf.station_id
		LEFT JOIN pois     p ON p.id = COALESCE(b.poi_id, pf.station_id)
		LEFT JOIN systems  s ON s.id = p.system_id
		LEFT JOIN factions f ON f.faction_id = pf.owner_faction
		WHERE pf.public = 1
		ORDER BY pf.station_id, pf.facility_id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []PublicFacility
	for rows.Next() {
		var (
			f            PublicFacility
			detailsJSON  string
			baseName     string
			poiName      string
		)
		if err := rows.Scan(
			&f.StationID, &f.FacilityID, &f.RecipeID, &f.FacilityName,
			&f.Level, &f.FeePerRun, &f.OwnerID,
			&detailsJSON, &f.LastSeenTick,
			&baseName, &poiName,
			&f.SystemID, &f.SystemName,
			&f.OwnerName, &f.OwnerTag,
		); err != nil {
			return nil, err
		}

		// Station display name: bases.name -> pois.name -> raw station_id.
		switch {
		case baseName != "":
			f.StationName = baseName
		case poiName != "":
			f.StationName = poiName
		default:
			f.StationName = f.StationID
		}

		// A malformed details_json degrades this row's throughput cells to
		// blank. It never fails the page.
		var d facilityDetails
		if err := json.Unmarshal([]byte(detailsJSON), &d); err == nil {
			f.FacilityType = d.Type
			f.ItemsPerHour = int(d.Production.ItemsPerHour)
			f.QtyPerRun = int(d.Production.OutputPerRun)
		}

		out = append(out, f)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/generate-items-kb/ -run TestLoadPublicFacilities -v`
Expected: PASS (both `TestLoadPublicFacilities` and `TestLoadPublicFacilitiesMissingTable`)

- [ ] **Step 5: Lint**

Run: `golangci-lint run ./cmd/generate-items-kb/`
Expected: no findings. (If it flags the unused `poiName`/`baseName` alignment in the `var` block, collapse to individual declarations.)

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-items-kb/where.go cmd/generate-items-kb/where_test.go
git commit -m "feat(kb): load public_facilities rows with station/system/faction joins"
```

---

### Task 3: Group by recipe, group by station, split the rest

**Files:**
- Modify: `cmd/generate-items-kb/where.go`
- Test: `cmd/generate-items-kb/where_test.go` (extend)

**Interfaces:**
- Consumes: `PublicFacility` and `loadPublicFacilities` (Task 2); `Recipe`, `RecipeItem`, `dirName` (existing, `main.go`).
- Produces:
  - `type WhereRecipeGroup struct`, `type WhereStationFacility struct`, `type WhereStationCategory struct`, `type WhereStationGroup struct`, `type NoFacilityRecipe struct`
  - `func groupByRecipe(facs []PublicFacility, recipes map[string]*Recipe) []WhereRecipeGroup`
  - `func groupByStation(facs []PublicFacility, recipes map[string]*Recipe) []WhereStationGroup`
  - `func splitNoFacilityRecipes(recipes map[string]*Recipe, covered map[string]bool) (facilityOnly, noFacilityNeeded []NoFacilityRecipe)`

- [ ] **Step 1: Write the failing test**

Append to `cmd/generate-items-kb/where_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/generate-items-kb/ -run 'TestGroupBy|TestSplitNoFacility' -v`
Expected: FAIL to compile — `undefined: groupByRecipe`, `undefined: groupByStation`, `undefined: splitNoFacilityRecipes`.

- [ ] **Step 3: Write the implementation**

Append to `cmd/generate-items-kb/where.go` (and add `"cmp"` and `"slices"` to its import block):

```go
// WhereRecipeGroup is one recipe and every public line that runs it.
type WhereRecipeGroup struct {
	RecipeID       string
	RecipeName     string
	RecipeCategory string
	RecipeDirName  string // category with spaces -> underscores, for the URL path
	FacilityOnly   bool
	Outputs        []RecipeItem
	Facilities     []PublicFacility
}

// WhereStationFacility is a public line with its recipe metadata attached, for
// rendering inside a station's section.
type WhereStationFacility struct {
	PublicFacility
	RecipeName    string
	RecipeDirName string
	Outputs       []RecipeItem
}

// WhereStationCategory is one recipe-category block within a station.
type WhereStationCategory struct {
	Category   string
	Facilities []WhereStationFacility
}

// WhereStationGroup is one station and everything craftable there, bucketed by
// recipe category.
type WhereStationGroup struct {
	StationID   string
	StationName string
	SystemID    string
	SystemName  string
	Count       int
	FeeMin      int
	FeeMax      int
	Categories  []WhereStationCategory
}

// NoFacilityRecipe is a recipe with no known public line, for the two dense
// tables at the bottom of the by-recipe tab.
type NoFacilityRecipe struct {
	ID             string
	Name           string
	Category       string
	DirName        string
	OutputID       string
	OutputName     string
	OutputCategory string
	OutputQty      int
	CraftingTime   float64
}

// groupByRecipe buckets public lines by the recipe they run, sorted by recipe
// name; lines within a group sort by station name. Facilities whose recipe is
// absent from the crafting DB are dropped -- every ID resolves today, and a
// group with no name or category would render as a dead link.
func groupByRecipe(facs []PublicFacility, recipes map[string]*Recipe) []WhereRecipeGroup {
	byRecipe := make(map[string][]PublicFacility)
	for _, f := range facs {
		if _, ok := recipes[f.RecipeID]; !ok {
			continue
		}
		byRecipe[f.RecipeID] = append(byRecipe[f.RecipeID], f)
	}

	groups := make([]WhereRecipeGroup, 0, len(byRecipe))
	for id, lines := range byRecipe {
		r := recipes[id]
		slices.SortFunc(lines, func(a, b PublicFacility) int {
			if c := cmp.Compare(a.StationName, b.StationName); c != 0 {
				return c
			}
			return cmp.Compare(a.FacilityID, b.FacilityID)
		})
		groups = append(groups, WhereRecipeGroup{
			RecipeID:       r.ID,
			RecipeName:     r.Name,
			RecipeCategory: r.Category,
			RecipeDirName:  dirName(r.Category),
			FacilityOnly:   r.FacilityOnly,
			Outputs:        r.Outputs,
			Facilities:     lines,
		})
	}
	slices.SortFunc(groups, func(a, b WhereRecipeGroup) int {
		if c := cmp.Compare(a.RecipeName, b.RecipeName); c != 0 {
			return c
		}
		return cmp.Compare(a.RecipeID, b.RecipeID)
	})
	return groups
}

// groupByStation buckets public lines by station (count descending, then name)
// and, within each station, by recipe category (name ascending). The category
// grouping is what keeps a 219-line station scannable.
func groupByStation(facs []PublicFacility, recipes map[string]*Recipe) []WhereStationGroup {
	type stationAcc struct {
		g      WhereStationGroup
		byCat  map[string][]WhereStationFacility
	}
	acc := make(map[string]*stationAcc)

	for _, f := range facs {
		r, ok := recipes[f.RecipeID]
		if !ok {
			continue
		}
		a, ok := acc[f.StationID]
		if !ok {
			a = &stationAcc{
				g: WhereStationGroup{
					StationID:   f.StationID,
					StationName: f.StationName,
					SystemID:    f.SystemID,
					SystemName:  f.SystemName,
					FeeMin:      f.FeePerRun,
					FeeMax:      f.FeePerRun,
				},
				byCat: make(map[string][]WhereStationFacility),
			}
			acc[f.StationID] = a
		}
		a.g.Count++
		a.g.FeeMin = min(a.g.FeeMin, f.FeePerRun)
		a.g.FeeMax = max(a.g.FeeMax, f.FeePerRun)
		a.byCat[r.Category] = append(a.byCat[r.Category], WhereStationFacility{
			PublicFacility: f,
			RecipeName:     r.Name,
			RecipeDirName:  dirName(r.Category),
			Outputs:        r.Outputs,
		})
	}

	stations := make([]WhereStationGroup, 0, len(acc))
	for _, a := range acc {
		cats := make([]WhereStationCategory, 0, len(a.byCat))
		for name, lines := range a.byCat {
			slices.SortFunc(lines, func(x, y WhereStationFacility) int {
				if c := cmp.Compare(x.RecipeName, y.RecipeName); c != 0 {
					return c
				}
				return cmp.Compare(x.FacilityID, y.FacilityID)
			})
			cats = append(cats, WhereStationCategory{Category: name, Facilities: lines})
		}
		slices.SortFunc(cats, func(x, y WhereStationCategory) int {
			return cmp.Compare(x.Category, y.Category)
		})
		a.g.Categories = cats
		stations = append(stations, a.g)
	}

	slices.SortFunc(stations, func(a, b WhereStationGroup) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 { // count descending
			return c
		}
		if c := cmp.Compare(a.StationName, b.StationName); c != 0 {
			return c
		}
		return cmp.Compare(a.StationID, b.StationID)
	})
	return stations
}

// splitNoFacilityRecipes partitions the recipes with no known public line into
// the two dense tables.
//
// The split is on facility_only, and the distinction is the whole point: a
// facility_only recipe with no public line genuinely cannot be crafted at a
// bare station, while a non-facility_only one can be crafted anywhere, so the
// absence of a public line barely matters. Recipes that DO have a public line
// appear in neither table.
func splitNoFacilityRecipes(recipes map[string]*Recipe, covered map[string]bool) (facilityOnly, noFacilityNeeded []NoFacilityRecipe) {
	for id, r := range recipes {
		if covered[id] {
			continue
		}
		e := NoFacilityRecipe{
			ID:           r.ID,
			Name:         r.Name,
			Category:     r.Category,
			DirName:      dirName(r.Category),
			CraftingTime: r.CraftingTime,
		}
		if len(r.Outputs) > 0 {
			o := r.Outputs[0]
			e.OutputID, e.OutputName, e.OutputCategory, e.OutputQty =
				o.ItemID, o.ItemName, o.ItemCategory, o.Quantity
		}
		if r.FacilityOnly {
			facilityOnly = append(facilityOnly, e)
		} else {
			noFacilityNeeded = append(noFacilityNeeded, e)
		}
	}

	byName := func(a, b NoFacilityRecipe) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	}
	slices.SortFunc(facilityOnly, byName)
	slices.SortFunc(noFacilityNeeded, byName)
	return facilityOnly, noFacilityNeeded
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/generate-items-kb/ -run 'TestGroupBy|TestSplitNoFacility' -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Lint and full test**

Run: `go build ./... && go test ./cmd/generate-items-kb/ && golangci-lint run ./cmd/generate-items-kb/`
Expected: all pass, no new findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-items-kb/where.go cmd/generate-items-kb/where_test.go
git commit -m "feat(kb): group public facilities by recipe and station, split uncovered recipes"
```

---

### Task 4: Render the page

**Files:**
- Modify: `cmd/generate-items-kb/where.go`
- Test: `cmd/generate-items-kb/where_test.go` (extend)

**Interfaces:**
- Consumes: everything from Tasks 2 and 3; `siteHeader`, `sortScript`, `themeScript` (existing `main.go` vars); `humanize.Comma`.
- Produces: `func writeWherePage(outDir string, knowledgeDB *sql.DB, recipes map[string]*Recipe) error`, writing `<outDir>/where.html`.

Note: the spec's signature took an `items map[string]*Item`. It is not needed — `RecipeItem` already carries `ItemName` and `ItemCategory`, which is everything the output-item links require. Dropping the parameter.

- [ ] **Step 1: Write the failing test**

Append to `cmd/generate-items-kb/where_test.go`:

```go
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
		`id="r-refine_steel"`,                              // recipe deep-link anchor
		`id="s-confederacy_central_command"`,               // station deep-link anchor
		`../systems/sol/index.html`,                        // station -> system link
		`../facilities/production/salvage_smelter.html`,    // facility -> type link
		`Refining/refine_steel.html`,                       // recipe -> recipe page link
		`../items/material/steel_plate.html`,               // output -> item link
		`Gap Recipe`,                                       // facility-only gap table
		`No Known Public Line`,
		`No Facility Required`,
		`fac_unknown_hash`,                                 // unresolved faction renders raw
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
```

Add `"os"`, `"path/filepath"`, and `"strings"` to the test file's import block.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/generate-items-kb/ -run TestWriteWherePage -v`
Expected: FAIL to compile — `undefined: writeWherePage`.

- [ ] **Step 3: Write the implementation**

Append to `cmd/generate-items-kb/where.go` (add `"fmt"`, `htmltpl "html/template"`, `"log"`, `"os"`, `"path/filepath"`, `"strings"`, and `humanize "github.com/dustin/go-humanize"` to its import block):

```go
// wherePageData is the root template context for where.html.
type wherePageData struct {
	StationCount     int
	FacilityCount    int
	RecipesCovered   int
	FacilityOnlyGap  int
	LastSeenTick     int
	RecipeGroups     []WhereRecipeGroup
	StationGroups    []WhereStationGroup
	FacilityOnlyNone []NoFacilityRecipe
	NoFacilityNeeded []NoFacilityRecipe
}

// writeWherePage renders kb/recipes/where.html.
//
// MUST be called after writeRecipePages: that function calls
// cleanGeneratedFiles on the same directory, which deletes every .html in it.
func writeWherePage(outDir string, knowledgeDB *sql.DB, recipes map[string]*Recipe) error {
	facs, err := loadPublicFacilities(knowledgeDB)
	if err != nil {
		return fmt.Errorf("load public facilities: %w", err)
	}

	covered := make(map[string]bool, len(facs))
	maxTick := 0
	for _, f := range facs {
		covered[f.RecipeID] = true
		maxTick = max(maxTick, f.LastSeenTick)
	}

	recipeGroups := groupByRecipe(facs, recipes)
	stationGroups := groupByStation(facs, recipes)
	facilityOnlyNone, noFacilityNeeded := splitNoFacilityRecipes(recipes, covered)

	data := wherePageData{
		StationCount:     len(stationGroups),
		FacilityCount:    len(facs),
		RecipesCovered:   len(recipeGroups),
		FacilityOnlyGap:  len(facilityOnlyNone),
		LastSeenTick:     maxTick,
		RecipeGroups:     recipeGroups,
		StationGroups:    stationGroups,
		FacilityOnlyNone: facilityOnlyNone,
		NoFacilityNeeded: noFacilityNeeded,
	}

	funcs := htmltpl.FuncMap{
		"comma": func(n int) string { return humanize.Comma(int64(n)) },
		"lower": strings.ToLower, // faction dirs are the lowercased tag: kb/factions/hexc/
		"fmtTime": func(f float64) string {
			if f == 0 {
				return "-"
			}
			return fmt.Sprintf("%.1fs", f)
		},
		"itemURL": func(category, id string) string {
			if category == "" {
				return ""
			}
			return fmt.Sprintf("../items/%s/%s.html", category, id)
		},
		"shortHash": func(s string) string {
			if len(s) > 8 {
				return s[:8]
			}
			return s
		},
	}

	tmpl := htmltpl.Must(htmltpl.New("where").Funcs(funcs).Parse(whereTemplate))

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	outPath := filepath.Join(outDir, "where.html")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	log.Printf("Where-To-Craft: %d public lines, %d recipes covered, %d stations, %d facility-only gaps",
		len(facs), len(recipeGroups), len(stationGroups), len(facilityOnlyNone))
	return nil
}

// whereTabScript selects the tab from the URL hash: #s-<station> opens the
// by-station tab, anything else opens by-recipe, so external deep links land
// on the right view and scroll to their anchor.
var whereTabScript = `    <script>
    (function() {
      var buttons = document.querySelectorAll(".tab-btn");
      function show(id) {
        document.querySelectorAll(".tab-panel").forEach(function(p) { p.hidden = (p.id !== id); });
        buttons.forEach(function(b) { b.classList.toggle("active", b.dataset.tab === id); });
      }
      var hash = location.hash.slice(1);
      var initial = (hash === "by-station" || hash.indexOf("s-") === 0) ? "by-station" : "by-recipe";
      show(initial);
      if (hash && hash !== "by-recipe" && hash !== "by-station") {
        var el = document.getElementById(hash);
        if (el) el.scrollIntoView();
      }
      buttons.forEach(function(b) {
        b.addEventListener("click", function() {
          show(b.dataset.tab);
          history.replaceState(null, "", "#" + b.dataset.tab);
        });
      });
    })();
    </script>`

var whereTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Where Can I Make This - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
    <style>
        .tabs { display: flex; gap: 4px; margin: 20px 0 8px; border-bottom: 2px solid var(--border); }
        .tab-btn { background: none; border: none; border-bottom: 2px solid transparent; margin-bottom: -2px;
                   padding: 10px 18px; font-size: 1em; cursor: pointer; color: var(--text-muted); }
        .tab-btn:hover { color: var(--link); }
        .tab-btn.active { color: var(--link); border-bottom-color: var(--link); font-weight: 600; }
        .summary-cards { display: flex; gap: 16px; margin: 16px 0; flex-wrap: wrap; }
        .summary-card { background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; padding: 12px 20px; text-align: center; }
        .summary-card .num { font-size: 1.8em; font-weight: 700; }
        .summary-card .label { font-size: 0.8em; color: var(--text-muted); text-transform: uppercase; }
        .freshness { font-size: 0.85em; color: var(--text-muted); margin-bottom: 8px; }
        .toc { columns: 3; column-gap: 24px; margin: 16px 0 32px; }
        .toc a { display: block; padding: 2px 0; color: var(--link); text-decoration: none; font-size: 0.95em; }
        .toc a:hover { text-decoration: underline; }
        .where-section { margin-top: 32px; scroll-margin-top: 16px; }
        .where-section h3 { margin-bottom: 8px; border-bottom: 1px solid var(--border); padding-bottom: 4px; }
        .where-section table { width: 100%; font-size: 0.9em; }
        .where-section th { text-align: left; cursor: pointer; user-select: none; white-space: nowrap; }
        .where-section th:hover { color: var(--link); }
        .where-section td { padding: 4px 8px; }
        .where-section tr:hover { background: var(--bg-hover, rgba(128,128,128,0.08)); }
        .cat-block { margin: 16px 0 24px; }
        .cat-block h4 { margin: 0 0 4px; font-size: 0.95em; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.04em; }
        .dense { font-size: 0.85em; width: 100%; }
        .dense td, .dense th { padding: 2px 8px; }
        .callout { border: 1px solid var(--border); border-left: 4px solid #999; padding: 12px 16px; margin: 24px 0 8px; border-radius: 4px; background: var(--bg-card); }
        .callout.warn { border-left-color: #d08040; }
        .callout h3 { margin: 0 0 4px; }
        .callout p { margin: 0; color: var(--text-muted); font-size: 0.9em; }
        .back-top { font-size: 0.8em; margin-left: 8px; color: var(--text-muted); }
        .num-cell { text-align: right; font-variant-numeric: tabular-nums; }
        @media (max-width: 768px) { .toc { columns: 2; } }
        @media (max-width: 480px) { .toc { columns: 1; } }
    </style>
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Where Can I Make This</h2>
        <p>Public production facilities across the galaxy: which stations rent a line for a given recipe, and what each station can produce.</p>

        <div class="summary-cards">
            <div class="summary-card"><div class="num">{{.StationCount}}</div><div class="label">Stations With Public Lines</div></div>
            <div class="summary-card"><div class="num">{{.FacilityCount}}</div><div class="label">Public Facilities</div></div>
            <div class="summary-card"><div class="num">{{.RecipesCovered}}</div><div class="label">Recipes Covered</div></div>
            <div class="summary-card"><div class="num">{{.FacilityOnlyGap}}</div><div class="label">Facility-Only, No Public Line</div></div>
        </div>
        <p class="freshness">Facility data as of tick {{comma .LastSeenTick}}. Station survey bots report roughly hourly. Private and faction-owned facilities are not listed here.</p>

        <div class="tabs">
            <button class="tab-btn" data-tab="by-recipe">By Recipe</button>
            <button class="tab-btn" data-tab="by-station">By Station</button>
        </div>

        <section class="tab-panel" id="by-recipe">
            <div class="card" style="padding: 12px 16px">
                <div class="section-label">Jump To Recipe</div>
                <div class="toc">
{{- range .RecipeGroups}}
                    <a href="#r-{{.RecipeID}}">{{.RecipeName}} ({{len .Facilities}})</a>
{{- end}}
                </div>
            </div>

{{- range .RecipeGroups}}
            <div id="r-{{.RecipeID}}" class="where-section">
                <h3>
                    <a href="{{.RecipeDirName}}/{{.RecipeID}}.html">{{.RecipeName}}</a>
                    <span class="badge" style="font-size:0.7em; vertical-align:middle;">{{len .Facilities}} station{{if ne (len .Facilities) 1}}s{{end}}</span>
{{- if .FacilityOnly}}
                    <span class="badge badge-frost" style="font-size:0.7em; vertical-align:middle;" title="Requires a production facility">Facility Only</span>
{{- end}}
{{- range .Outputs}}
                    <small style="font-size:0.75em; font-weight:normal;">&rarr; <a href="{{itemURL .ItemCategory .ItemID}}">{{.ItemName}}</a> &times;{{.Quantity}}</small>
{{- end}}
                    <a href="#" class="back-top">[top]</a>
                </h3>
                <table class="sortable">
                    <thead>
                        <tr>
                            <th class="sortable">Station</th>
                            <th class="sortable">System</th>
                            <th class="sortable">Facility</th>
                            <th class="sortable">Level</th>
                            <th class="sortable">Fee/run</th>
                            <th class="sortable">Qty/run</th>
                            <th class="sortable">Items/hr</th>
                            <th class="sortable">Owner</th>
                        </tr>
                    </thead>
                    <tbody>
{{- range .Facilities}}
                        <tr>
                            <td><a href="#s-{{.StationID}}">{{.StationName}}</a></td>
                            <td>{{if .SystemID}}<a href="../systems/{{.SystemID}}/index.html">{{.SystemName}}</a>{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                            <td>{{if .FacilityType}}<a href="../facilities/production/{{.FacilityType}}.html">{{.FacilityName}}</a>{{else}}{{.FacilityName}}{{end}}</td>
                            <td class="num-cell" data-sort="{{.Level}}">{{.Level}}</td>
                            <td class="num-cell" data-sort="{{.FeePerRun}}">{{comma .FeePerRun}}</td>
                            <td class="num-cell" data-sort="{{.QtyPerRun}}">{{if .QtyPerRun}}{{.QtyPerRun}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                            <td class="num-cell" data-sort="{{.ItemsPerHour}}">{{if .ItemsPerHour}}{{comma .ItemsPerHour}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                            <td>{{if .OwnerName}}<a href="../factions/{{lower .OwnerTag}}/index.html">{{.OwnerName}}</a>{{else}}<code title="{{.OwnerID}}">{{shortHash .OwnerID}}</code>{{end}}</td>
                        </tr>
{{- end}}
                    </tbody>
                </table>
            </div>
{{- end}}

            <div class="callout warn">
                <h3>Facility-Only &mdash; No Known Public Line ({{len .FacilityOnlyNone}})</h3>
                <p>These recipes require a production facility, and no public line for them has been surveyed. They cannot be crafted at a bare station. A private or faction-owned facility may still run them &mdash; those never appear in this data.</p>
            </div>
            <table class="dense sortable">
                <thead>
                    <tr>
                        <th class="sortable">Recipe</th>
                        <th class="sortable">Category</th>
                        <th class="sortable">Output</th>
                        <th class="sortable">Qty</th>
                        <th class="sortable">Craft Time</th>
                    </tr>
                </thead>
                <tbody>
{{- range .FacilityOnlyNone}}
                    <tr>
                        <td><a href="{{.DirName}}/{{.ID}}.html">{{.Name}}</a></td>
                        <td><a href="{{.DirName}}/">{{.Category}}</a></td>
                        <td>{{if .OutputCategory}}<a href="{{itemURL .OutputCategory .OutputID}}">{{.OutputName}}</a>{{else}}{{.OutputName}}{{end}}</td>
                        <td class="num-cell" data-sort="{{.OutputQty}}">{{.OutputQty}}</td>
                        <td class="num-cell" data-sort="{{.CraftingTime}}">{{fmtTime .CraftingTime}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>

            <div class="callout">
                <h3>No Facility Required ({{len .NoFacilityNeeded}})</h3>
                <p>No public line has been surveyed for these, but none is needed &mdash; they can be crafted at any station. A public facility would only add throughput.</p>
            </div>
            <table class="dense sortable">
                <thead>
                    <tr>
                        <th class="sortable">Recipe</th>
                        <th class="sortable">Category</th>
                        <th class="sortable">Output</th>
                        <th class="sortable">Qty</th>
                        <th class="sortable">Craft Time</th>
                    </tr>
                </thead>
                <tbody>
{{- range .NoFacilityNeeded}}
                    <tr>
                        <td><a href="{{.DirName}}/{{.ID}}.html">{{.Name}}</a></td>
                        <td><a href="{{.DirName}}/">{{.Category}}</a></td>
                        <td>{{if .OutputCategory}}<a href="{{itemURL .OutputCategory .OutputID}}">{{.OutputName}}</a>{{else}}{{.OutputName}}{{end}}</td>
                        <td class="num-cell" data-sort="{{.OutputQty}}">{{.OutputQty}}</td>
                        <td class="num-cell" data-sort="{{.CraftingTime}}">{{fmtTime .CraftingTime}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
        </section>

        <section class="tab-panel" id="by-station" hidden>
            <div class="card" style="padding: 12px 16px">
                <div class="section-label">Jump To Station</div>
                <div class="toc">
{{- range .StationGroups}}
                    <a href="#s-{{.StationID}}">{{.StationName}} ({{.Count}})</a>
{{- end}}
                </div>
            </div>

{{- range .StationGroups}}
            <div id="s-{{.StationID}}" class="where-section">
                <h3>
                    {{.StationName}}
                    <span class="badge" style="font-size:0.7em; vertical-align:middle;">{{.Count}} facilit{{if eq .Count 1}}y{{else}}ies{{end}}</span>
                    {{if .SystemID}}<small style="font-size:0.75em; font-weight:normal;">in <a href="../systems/{{.SystemID}}/index.html">{{.SystemName}}</a></small>{{end}}
                    <small style="font-size:0.75em; font-weight:normal;" class="text-muted">fees {{comma .FeeMin}}&ndash;{{comma .FeeMax}}/run</small>
                    <a href="#" class="back-top">[top]</a>
                </h3>
{{- range .Categories}}
                <div class="cat-block">
                    <h4>{{.Category}}</h4>
                    <table class="sortable">
                        <thead>
                            <tr>
                                <th class="sortable">Recipe</th>
                                <th class="sortable">Output</th>
                                <th class="sortable">Facility</th>
                                <th class="sortable">Level</th>
                                <th class="sortable">Fee/run</th>
                                <th class="sortable">Qty/run</th>
                                <th class="sortable">Items/hr</th>
                                <th class="sortable">Owner</th>
                            </tr>
                        </thead>
                        <tbody>
{{- range .Facilities}}
                            <tr>
                                <td><a href="{{.RecipeDirName}}/{{.RecipeID}}.html">{{.RecipeName}}</a></td>
                                <td>{{range .Outputs}}<a href="{{itemURL .ItemCategory .ItemID}}">{{.ItemName}}</a> &times;{{.Quantity}} {{end}}</td>
                                <td>{{if .FacilityType}}<a href="../facilities/production/{{.FacilityType}}.html">{{.FacilityName}}</a>{{else}}{{.FacilityName}}{{end}}</td>
                                <td class="num-cell" data-sort="{{.Level}}">{{.Level}}</td>
                                <td class="num-cell" data-sort="{{.FeePerRun}}">{{comma .FeePerRun}}</td>
                                <td class="num-cell" data-sort="{{.QtyPerRun}}">{{if .QtyPerRun}}{{.QtyPerRun}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                                <td class="num-cell" data-sort="{{.ItemsPerHour}}">{{if .ItemsPerHour}}{{comma .ItemsPerHour}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                                <td>{{if .OwnerName}}<a href="../factions/{{lower .OwnerTag}}/index.html">{{.OwnerName}}</a>{{else}}<code title="{{.OwnerID}}">{{shortHash .OwnerID}}</code>{{end}}</td>
                            </tr>
{{- end}}
                        </tbody>
                    </table>
                </div>
{{- end}}
            </div>
{{- end}}
        </section>
    </main>
` + sortScript + `
` + whereTabScript + `
` + themeScript + `
</body>
</html>
`
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/generate-items-kb/ -run TestWriteWherePage -v`
Expected: PASS (both tests)

Faction page URLs are keyed by the **lowercased tag** (`kb/factions/hexc/index.html`, verified 2026-07-09), which is why both templates use `{{lower .OwnerTag}}` rather than the raw tag.

- [ ] **Step 5: Lint and full test**

Run: `go build ./... && go test ./cmd/generate-items-kb/ && golangci-lint run ./cmd/generate-items-kb/`
Expected: all pass, no new findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-items-kb/where.go cmd/generate-items-kb/where_test.go
git commit -m "feat(kb): render Where Can I Make This page with by-recipe and by-station tabs"
```

---

### Task 5: Wire into `main()`, add `-where-only`, link from the recipes index

**Files:**
- Modify: `cmd/generate-items-kb/main.go` (flag block ~line 555; recipes-index template ~line 2978; new wrapper func near `generateAllMissions` ~line 857)

**Interfaces:**
- Consumes: `writeWherePage` (Task 4), `loadRecipes` (Task 1).
- Produces: `func generateWherePage(recipes map[string]*Recipe)`, `func generateWhereOnly()`.

- [ ] **Step 1: Add the wrapper and the where-only mode**

In `cmd/generate-items-kb/main.go`, immediately after the `generateAllMissions` function, add:

```go
// generateWherePage renders the Where-To-Craft side page from the knowledge
// DB's public_facilities table.
//
// Every failure here is a warning, not a fatal: public_facilities is a new
// table, and a knowledge DB from before it landed must not break a full site
// regeneration.
func generateWherePage(recipes map[string]*Recipe) {
	knowledgeDBPath := "../spacemolt-knowledge.db"

	knowledgeDB, err := sql.Open("sqlite", knowledgeDBPath)
	if err != nil {
		log.Printf("warning: open knowledge database for where-page: %v (page will be skipped)", err)
		return
	}
	defer func() { _ = knowledgeDB.Close() }()

	if err := writeWherePage("kb/recipes", knowledgeDB, recipes); err != nil {
		if errors.Is(err, errNoPublicFacilities) {
			log.Printf("note: knowledge DB has no public_facilities table; skipping where-page")
			return
		}
		log.Printf("warning: write where-page: %v (page will be skipped)", err)
		return
	}
	fmt.Println("Generated where-to-craft page in kb/recipes/where.html")
}

// generateWhereOnly regenerates just the where-to-craft page. It needs the
// crafting DB for recipe names/categories/facility_only and the knowledge DB
// for the facility rows.
func generateWhereOnly(craftingDBPath string) {
	db, err := sql.Open("sqlite", craftingDBPath)
	if err != nil {
		log.Fatalf("open crafting database: %v", err)
	}
	defer func() { _ = db.Close() }()

	recipes, err := loadRecipes(db)
	if err != nil {
		log.Fatalf("load recipes: %v", err)
	}
	generateWherePage(recipes)
}
```

Add `"errors"` to `main.go`'s import block if it is not already there.

- [ ] **Step 2: Register the flag**

In `main()`, find:

```go
	resourcesOnly := flag.Bool("resources-only", false, "regenerate only the resources index page")
```

Add below it:

```go
	whereOnly := flag.Bool("where-only", false, "regenerate only the where-to-craft page (kb/recipes/where.html)")
```

Then find the resources-only early-return block:

```go
	// --- Resources-only mode (just the resources index) ---
	if *resourcesOnly {
		generateResourcesOnly()
		return
	}
```

Add below it:

```go
	// --- Where-only mode (just the where-to-craft page) ---
	if *whereOnly {
		generateWhereOnly(dbPath)
		return
	}
```

Note: `dbPath` is assigned above the flag-mode blocks but the positional-arg override happens before them, so `dbPath` is correct at this point. Verify by reading `main()` top-to-bottom before editing.

- [ ] **Step 3: Call it from the full-generation path**

In `main()`, find the last line of the function:

```go
	// --- Missions generation ---
	generateAllMissions(items)
}
```

Replace with:

```go
	// --- Missions generation ---
	generateAllMissions(items)

	// --- Where-to-craft page ---
	// MUST run after writeRecipePages: that call cleans every .html out of
	// kb/recipes/ before writing, which would delete where.html.
	generateWherePage(recipes)
}
```

- [ ] **Step 4: Link the page from the recipes index**

Find the `recipeTopTemplate` body:

```go
    <main class="container page-content">
        <h2>Recipes</h2>
        <p class="text-muted mt-1">{{len .}} categories of crafting recipes.</p>
        <div class="item-categories">
```

Replace with:

```go
    <main class="container page-content">
        <h2>Recipes</h2>
        <p class="text-muted mt-1">{{len .}} categories of crafting recipes.</p>
        <p class="mt-2"><a href="where.html">&#x1F3ED; Where Can I Make This? &mdash; public facilities by recipe and by station &rarr;</a></p>
        <div class="item-categories">
```

- [ ] **Step 5: Build, test, lint**

Run: `go build ./... && go test ./cmd/generate-items-kb/ && golangci-lint run ./cmd/generate-items-kb/`
Expected: all pass, no new findings.

- [ ] **Step 6: Generate the page against the real DBs and verify the counts**

Run: `go run ./cmd/generate-items-kb -where-only`

Expected log line, matching the spec's verified numbers exactly:

```
Where-To-Craft: 247 public lines, 149 recipes covered, 6 stations, 236 facility-only gaps
Generated where-to-craft page in kb/recipes/where.html
```

If any number differs, **stop** — either the DB changed since 2026-07-09 or the grouping is wrong. Re-derive before proceeding:

```bash
sqlite3 ../spacemolt-knowledge.db \
  "SELECT COUNT(*), COUNT(DISTINCT recipe_id), COUNT(DISTINCT station_id) FROM public_facilities WHERE public = 1;"
```

- [ ] **Step 7: Spot-check the rendered page**

```bash
grep -c 'class="where-section"' kb/recipes/where.html   # expect 155 (149 recipes + 6 stations)
grep -c '<tr>' kb/recipes/where.html                    # expect ~1000 (247*2 + 517 dense rows)
grep -o 'id="s-[a-z_]*"' kb/recipes/where.html | sort -u # expect the 6 station anchors
grep -c 'No Known Public Line' kb/recipes/where.html    # expect 1
grep -io 'nowhere\|impossible' kb/recipes/where.html    # expect NO output (copy rule)
```

Confirm the "Facility-Only — No Known Public Line" heading reads `(236)` and "No Facility Required" reads `(281)`.

- [ ] **Step 8: Verify links resolve**

```bash
python3 - <<'EOF'
import re, os
html = open('kb/recipes/where.html').read()
missing = set()
for href in set(re.findall(r'href="([^"#]+\.html)"', html)):
    if not os.path.exists(os.path.join('kb/recipes', href)):
        missing.add(href)
print("broken links:", len(missing))
for m in sorted(missing)[:20]:
    print(" ", m)
EOF
```

Expected: `broken links: 0`. Note this only checks same-directory-relative `.html` targets that exist on disk; the three unresolved owner factions render as `<code>` hashes, not links, so they cannot appear here.

- [ ] **Step 9: Commit**

```bash
git add cmd/generate-items-kb/main.go kb/recipes/where.html kb/recipes/index.html
git commit -m "feat(kb): wire where-to-craft page into generator, add -where-only flag"
```

---

### Task 6: Full regeneration and final verification

The site is committed to git, so this task exists to confirm the change produces exactly the diff it should and nothing else. A bare generator run regenerates the whole site and will surface unrelated data drift (see the KB regeneration runbook); that drift must be reviewed, not blindly committed.

**Files:** none created; `kb/**` regenerated.

- [ ] **Step 1: Confirm a clean tree before regenerating**

Run: `git status --short`
Expected: clean, apart from files you intend to change. If `kb/` already has uncommitted drift, stash or commit it first — otherwise you cannot tell your diff from the drift.

- [ ] **Step 2: Full regeneration**

Run: `go run ./cmd/generate-items-kb`
Expected: completes without `FATAL`. The final two lines are the where-page log lines from Task 5 Step 6.

- [ ] **Step 3: Inspect the diff surface**

Run: `git status --short kb/ | head -40` and `git diff --stat kb/ | tail -5`

Expected to change: `kb/recipes/where.html` (new), `kb/recipes/index.html` (the callout link).

Anything else is unrelated data drift from a newer scrape. Per the runbook, **scope the commit deliberately** — do not `git add -A`. If unrelated files changed, either revert them (`git checkout -- <path>`) or commit them separately with their own message.

- [ ] **Step 4: Determinism check — regenerating twice must produce no diff**

```bash
go run ./cmd/generate-items-kb -where-only
git diff --exit-code kb/recipes/where.html && echo "DETERMINISTIC"
```

Expected: `DETERMINISTIC`. A non-empty diff means a map iteration is leaking into output order; find the missing tiebreak in `groupByRecipe` / `groupByStation` / `splitNoFacilityRecipes`.

- [ ] **Step 5: Open the page and check both tabs**

```bash
python3 -m http.server 8765 --directory kb &
echo "open http://localhost:8765/recipes/where.html"
```

Confirm by eye:
1. Default view is **By Recipe**; clicking **By Station** swaps panels and sets `#by-station`.
2. Loading `.../where.html#s-confederacy_central_command` directly opens the **By Station** tab and scrolls to CCC.
3. Loading `.../where.html#r-refine_steel` opens the **By Recipe** tab at that recipe.
4. CCC's section shows category subheadings, not one flat 219-row table.
5. Clicking a column header sorts; clicking again reverses.
6. Theme toggle still works (the `themeScript` needs `#theme-toggle` from `siteHeader`).

Kill the server when done: `kill %1`

- [ ] **Step 6: Final gate**

Run: `go build ./... && go test ./... && golangci-lint run`
Expected: all pass, no new findings.

- [ ] **Step 7: Commit**

```bash
git add kb/recipes/where.html kb/recipes/index.html
git commit -m "feat(kb): add Where Can I Make This page to recipes section"
```

---

## Self-Review

**Spec coverage:**

| Spec requirement | Task |
|---|---|
| `where.go` mirroring `resources.go`, `writeWherePage` entry point | 2, 3, 4 |
| Warning-not-fatal wrapper; `errNoPublicFacilities` | 2, 5 |
| `-where-only` flag | 5 |
| Single file `kb/recipes/where.html`, linked from recipes index, not in global nav | 4, 5 |
| Query with `WHERE public = 1`, `COALESCE(b.poi_id, ...)`, `LEFT JOIN factions` | 2 |
| `details_json` → type, items_per_hour, output_per_run | 2 |
| Excluded: queue depth, ticks_per_run, last_seen_utc | 2 (never scanned) |
| Degradation table (5 rows) | 2 (loader), 4 (render), 5 (wrapper) |
| Station name fallback chain | 2 |
| Hash-driven tabs, `#r-` / `#s-` anchors | 4 |
| Four summary cards + freshness line | 4 |
| By-recipe: TOC, 8 columns, badges, links | 4 |
| By-station: count-desc order, category subheadings, 8 columns | 3, 4 |
| Two dense 5-column tables split on `facility_only` | 3, 4 |
| Copy rule ("no known public line", "no facility required") | 4 (asserted by test) |
| `Qty/run` naming; no separate `Type` column | 4 |
| Deterministic ordering | 3 (tiebreaks), 6 Step 4 (verified) |
| Tests: loader, grouping, split, ordering, render, empty table | 2, 3, 4 |
| `go build` / `go test` / `golangci-lint` gate | every task |

Two deviations from the spec, both deliberate and noted inline:
1. `writeWherePage` drops the `items map[string]*Item` parameter — `RecipeItem` already carries `ItemName`/`ItemCategory`.
2. Task 1 is new, not in the spec. It hardens `Recipe.FacilityOnly` to load from the crafting DB rather than depending solely on a catalog overlay that fails soft. Without it, a missing `catalog_recipes.json` would render the 236/281 split as 0/517 with no error — the page would state a falsehood confidently.

**Placeholder scan:** no TBDs, no "add error handling", no "similar to Task N". Every code step carries complete code.

**Type consistency:** `PublicFacility` field names are used identically in Tasks 2, 3, and 4 (`FeePerRun`, `QtyPerRun`, `ItemsPerHour`, `OwnerID`/`OwnerName`/`OwnerTag`, `FacilityType`). `WhereStationFacility` embeds `PublicFacility`, so the by-station template reaches `.Level`, `.FeePerRun`, etc. through promotion — consistent with the by-recipe template. `dirName` (existing) is used for both `RecipeDirName` and `NoFacilityRecipe.DirName`. `loadRecipes` returns `map[string]*Recipe` (not `[]Recipe`) everywhere.
