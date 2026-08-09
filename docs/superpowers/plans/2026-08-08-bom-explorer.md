# Bill of Materials Explorer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an interactive Bill of Materials explorer page to the Spacemolt KB where the user picks any craftable output, sets a quantity, chooses a recipe per multi-recipe ingredient, and sees the production chain as a layered left-to-right SVG graph plus flat input tables.

**Architecture:** A Go generator writes one JSON data file holding the whole recipe graph (~850 KB). A hand-maintained static HTML page plus a dependency-free JavaScript file loads that JSON and does all computation client-side: DAG expansion, whole-batch quantity roll-up, longest-path layering, barycentre column ordering, and SVG rendering. No prices, no market data, no build step.

**Tech Stack:** Go 1.25 (`modernc.org/sqlite`, stdlib `html/template`, `encoding/json`), vanilla ES2020 JavaScript with no dependencies, Node 22 built-in test runner (`node --test`), inline SVG.

**Spec:** `docs/superpowers/specs/2026-08-08-bom-explorer-design.md`

## Global Constraints

- Go module path is `github.com/rsned/spacemolt-kb`; the repo root is the working directory for all commands.
- Go 1.25: use `for i := range n` integer ranges and `b.Loop()` in benchmarks, never `for i := 0; i < b.N; i++`.
- `golangci-lint run` must produce no new findings. Run it after each series of Go changes.
- `go build ./...` and `go test ./...` must pass before every commit that touches Go.
- `node --test tests/js/` must pass before every commit that touches JavaScript.
- The explorer shows **quantities only** — no prices, credits, station names, or market data anywhere on the page or in the generated JSON.
- A recipe choice for an item applies to that item **everywhere** in the tree. There is never a per-consumer recipe override.
- `wrap_*` and `unwrap_*` recipes are omitted from the generated `recipes` map entirely (20 of 761 recipes).
- Generated data file path: `kb/build-costs/recipe-graph.json`. Page: `kb/build-costs/explorer.html`. Script: `kb/build-costs/bom-explorer.js`.
- `kb/build-costs/explorer.html` and `kb/build-costs/bom-explorer.js` are **hand-maintained**, exactly like the existing `kb/warp.js`. No generator writes them.
- All KB pages carry the standard theme-toggle boilerplate and CSS custom properties. Copy them verbatim from `cmd/generate-build-costs/templates/detail.html.tmpl` lines 1–24.
- Sort every map iteration before writing JSON output so regeneration is byte-identical run to run.

## Reference Data (verified against `crafting.db` on 2026-08-08)

Use these exact numbers in assertions about the real database:

| Quantity | Value |
|---|---|
| rows in `items` | 752 |
| rows in `recipes` | 761 |
| `wrap_*` / `unwrap_*` recipes | 20 |
| recipes after exclusion | 741 |
| distinct craftable items (after exclusion) | 615 |
| items with >1 recipe (after exclusion) | 62 |
| ships in `catalog_ships.json` | 335 (all have `build_materials`) |
| facilities in `catalog_facilities.json` | 2727 (2650 have `build_materials`) |
| craftable items that are NOT terminal (selectable) | 610 (615 distinct recipe outputs, minus 4 craftable ores, minus `fuel_reserve` which has no `items` row) |
| selectable outputs | 610 + 335 + 2650 = 3595 |
| deepest target | `annihilator`, 10 tiers, 92 distinct items |
| widest target | `overmind`, 10 tiers, 135 distinct items |
| `station_core` (the sample target used in checks) | 10 tiers, 74 distinct items |
| median target | 4 tiers, 11 distinct items |

**Correction (2026-08-08, verified):** an earlier draft of this table listed
`station_core` as 10 tiers / 75 items. That figure came from a probe that
picked each item's lexicographically-first recipe; the implemented code uses
the generated `defaults` (from `bom.SelectRecipe`), and 18 multi-recipe items
resolve differently under the two rules. A second draft then measured them before the terminal-item rule
(craftable ores were still being expanded). The figures above are measured
against the real data through the shipped code as it now stands.

## File Structure

| File | Responsibility |
|---|---|
| `pkg/catalog/catalog.go` | `FindLatestDir`, `LoadShips`, `LoadFacilities` — snapshot catalog access shared by both generators |
| `pkg/catalog/catalog_test.go` | Tests for the above |
| `cmd/generate-bom-explorer/main.go` | Flags, DB open, catalog load, write JSON, log summary |
| `cmd/generate-bom-explorer/build.go` | `BuildDoc` — the pure `crafting.db` rows + catalogs → `Doc` transform |
| `cmd/generate-bom-explorer/build_test.go` | Tests for `BuildDoc` |
| `kb/build-costs/bom-explorer.js` | Graph model, roll-up, layering, layout, URL state, SVG render, UI wiring |
| `kb/build-costs/explorer.html` | Markup, CSS, theme toggle. No logic. |
| `tests/js/bom-explorer.test.js` | `node --test` suite over the exported pure functions |
| `cmd/generate-build-costs/templates/detail.html.tmpl` | Gains the "Explore this BoM interactively →" link |
| `cmd/generate-build-costs/render.go` | Passes `ID` to that template |
| `docs/USAGE.md` | Regeneration step for the new generator |

---

### Task 1: Extract `pkg/catalog` from `generate-build-costs`

The two snapshot-catalog loaders currently live in `package main` inside `cmd/generate-build-costs`, so the new generator cannot reuse them. Move them to a package. Behaviour must not change.

**Files:**
- Create: `pkg/catalog/catalog.go`
- Create: `pkg/catalog/catalog_test.go`
- Modify: `cmd/generate-build-costs/main.go` (delete `loadShipCatalog` and `findLatestCatalogDir`, call the package)
- Modify: `cmd/generate-build-costs/facilities.go` (delete `loadFacilityCatalog` and the `facilityCatDoc` / `facilityCatItem` types, call the package)
- Modify: `cmd/generate-build-costs/load.go:323-328` (the `Ship` struct moves to `pkg/catalog`)

**Interfaces:**
- Produces:
  - `catalog.FindLatestDir(root string) (string, error)`
  - `catalog.Ship` struct with fields `ID, Name, Class string`, `Price int`, `BuildMaterials []catalog.Material`
  - `catalog.Facility` struct with fields `ID, Name, Category, RecipeID string`, `Level int`, `BuildMaterials []catalog.Material`
  - `catalog.Material` struct with fields `ItemID string`, `Quantity float64`
  - `catalog.LoadShips(root string) ([]Ship, error)`
  - `catalog.LoadFacilities(root string) ([]Facility, error)`

**Background:** the catalog JSON files live at `data/snapshots/<YYYYMMDD>/catalog_ships.json` and `catalog_facilities.json`. Both files are objects with an `items` array. `FindLatestDir` picks the most recently *modified* subdirectory of `root` — not the lexically greatest — because `data/snapshots/` also contains non-dated `latest/` and `previous/` directories. Facility `build_materials` quantities are floats in the source JSON; keep them float in the loader and let callers convert.

- [ ] **Step 1: Write the failing test**

Create `pkg/catalog/catalog_test.go`:

```go
package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSnapshot(t *testing.T, root, name, ships, facilities string, mod time.Time) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if ships != "" {
		if err := os.WriteFile(filepath.Join(dir, "catalog_ships.json"), []byte(ships), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if facilities != "" {
		if err := os.WriteFile(filepath.Join(dir, "catalog_facilities.json"), []byte(facilities), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(dir, mod, mod); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFindLatestDirPicksMostRecentlyModified(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	// "20260101" sorts after "previous" lexically but is older by mtime.
	writeSnapshot(t, root, "20260101", `{"items":[]}`, `{"items":[]}`, old)
	want := writeSnapshot(t, root, "previous", `{"items":[]}`, `{"items":[]}`, recent)

	got, err := FindLatestDir(root)
	if err != nil {
		t.Fatalf("FindLatestDir: %v", err)
	}
	if got != want {
		t.Errorf("FindLatestDir = %q, want %q", got, want)
	}
}

func TestFindLatestDirErrorsOnEmptyRoot(t *testing.T) {
	if _, err := FindLatestDir(t.TempDir()); err == nil {
		t.Fatal("FindLatestDir on empty root: want error, got nil")
	}
}

func TestLoadShips(t *testing.T) {
	root := t.TempDir()
	writeSnapshot(t, root, "20260807", `{"items":[
	  {"id":"absence","name":"Absence","class":"frigate","price":900,
	   "build_materials":[{"item_id":"steel_plate","quantity":5}]}
	]}`, `{"items":[]}`, time.Now())

	ships, err := LoadShips(root)
	if err != nil {
		t.Fatalf("LoadShips: %v", err)
	}
	if len(ships) != 1 {
		t.Fatalf("len(ships) = %d, want 1", len(ships))
	}
	s := ships[0]
	if s.ID != "absence" || s.Name != "Absence" || s.Class != "frigate" || s.Price != 900 {
		t.Errorf("ship = %+v, want id/name/class/price absence/Absence/frigate/900", s)
	}
	if len(s.BuildMaterials) != 1 || s.BuildMaterials[0].ItemID != "steel_plate" || s.BuildMaterials[0].Quantity != 5 {
		t.Errorf("build materials = %+v, want [{steel_plate 5}]", s.BuildMaterials)
	}
}

func TestLoadFacilitiesKeepsFloatQuantities(t *testing.T) {
	root := t.TempDir()
	writeSnapshot(t, root, "20260807", `{"items":[]}`, `{"items":[
	  {"id":"depot","name":"Depot","category":"service","level":2,"recipe_id":"build_depot",
	   "build_materials":[{"item_id":"steel_plate","quantity":8150.0},{"item_id":"hot_cell","quantity":2.5}]}
	]}`, time.Now())

	facs, err := LoadFacilities(root)
	if err != nil {
		t.Fatalf("LoadFacilities: %v", err)
	}
	if len(facs) != 1 {
		t.Fatalf("len(facilities) = %d, want 1", len(facs))
	}
	f := facs[0]
	if f.ID != "depot" || f.Name != "Depot" || f.Category != "service" || f.Level != 2 || f.RecipeID != "build_depot" {
		t.Errorf("facility = %+v, want depot/Depot/service/2/build_depot", f)
	}
	if len(f.BuildMaterials) != 2 || f.BuildMaterials[1].Quantity != 2.5 {
		t.Errorf("build materials = %+v, want second quantity 2.5 preserved", f.BuildMaterials)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./pkg/catalog/`
Expected: FAIL — the package does not exist (`no required module provides package`).

- [ ] **Step 3: Write the implementation**

Create `pkg/catalog/catalog.go`:

```go
// Package catalog reads the game-API snapshot catalogs (ships, facilities)
// that the KB generators build pages from. Snapshot directories live under a
// root such as data/snapshots/ and are selected by modification time, because
// the root also holds non-dated latest/ and previous/ directories.
package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Material is one entry of a ship's or facility's build_materials list.
// Quantity is float64 because facility quantities are floats in the source
// JSON; callers that need integers convert at their own boundary.
type Material struct {
	ItemID   string  `json:"item_id"`
	Quantity float64 `json:"quantity"`
}

// Ship is the subset of catalog_ships.json the KB generators consume.
type Ship struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Class          string     `json:"class"`
	Price          int        `json:"price"`
	BuildMaterials []Material `json:"build_materials"`
}

// Facility is the subset of catalog_facilities.json the KB generators consume.
type Facility struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Category       string     `json:"category"`
	Level          int        `json:"level"`
	RecipeID       string     `json:"recipe_id"`
	BuildMaterials []Material `json:"build_materials"`
}

// FindLatestDir returns the most recently modified subdirectory of root.
func FindLatestDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var best string
	var bestMod int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mt := info.ModTime().UnixNano(); mt > bestMod {
			bestMod, best = mt, filepath.Join(root, e.Name())
		}
	}
	if best == "" {
		return "", os.ErrNotExist
	}
	return best, nil
}

// LoadShips reads catalog_ships.json from the newest snapshot under root.
func LoadShips(root string) ([]Ship, error) {
	var doc struct {
		Items []Ship `json:"items"`
	}
	if err := loadCatalog(root, "catalog_ships.json", &doc); err != nil {
		return nil, err
	}
	return doc.Items, nil
}

// LoadFacilities reads catalog_facilities.json from the newest snapshot under root.
func LoadFacilities(root string) ([]Facility, error) {
	var doc struct {
		Items []Facility `json:"items"`
	}
	if err := loadCatalog(root, "catalog_facilities.json", &doc); err != nil {
		return nil, err
	}
	return doc.Items, nil
}

func loadCatalog(root, name string, dst any) error {
	dir, err := FindLatestDir(root)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./pkg/catalog/ -v`
Expected: PASS, all four tests.

- [ ] **Step 5: Rewire `generate-build-costs` to the new package**

In `cmd/generate-build-costs/main.go`: delete `findLatestCatalogDir` and `loadShipCatalog`. Replace the call site in `main` (currently `ships, catalogPrice, err := loadShipCatalog(*catalogRoot)`) with:

```go
	ships, err := catalog.LoadShips(*catalogRoot)
	must(err, "load ships")
	catalogPrice := make(map[string]int, len(ships))
	for _, s := range ships {
		catalogPrice[s.ID] = s.Price
	}
```

Add `"github.com/rsned/spacemolt-kb/pkg/catalog"` to the import block.

In `cmd/generate-build-costs/load.go`: delete the local `Ship` struct (lines 323–328) and change `loadTargets`'s signature from `ships []Ship` to `ships []catalog.Ship`, importing the package.

In `cmd/generate-build-costs/facilities.go`: delete `facilityCatDoc`, `facilityCatItem`, and the body of `loadFacilityCatalog`, replacing it with a thin adapter that keeps `FacilityRec` unchanged:

```go
// loadFacilityCatalog reads the newest facility catalog and returns the
// trimmed records the build-cost pages consume.
func loadFacilityCatalog(root string) ([]FacilityRec, error) {
	facs, err := catalog.LoadFacilities(root)
	if err != nil {
		return nil, err
	}
	out := make([]FacilityRec, 0, len(facs))
	for _, f := range facs {
		rec := FacilityRec{ID: f.ID, Name: f.Name, Category: f.Category, Level: f.Level, RecipeID: f.RecipeID}
		for _, m := range f.BuildMaterials {
			rec.Build = append(rec.Build, buildcost.Requirement{ItemID: m.ItemID, Qty: m.Quantity})
		}
		out = append(out, rec)
	}
	return out, nil
}
```

- [ ] **Step 6: Verify the existing generator's tests still pass**

Run: `go build ./... && go test ./cmd/generate-build-costs/ ./pkg/catalog/`
Expected: PASS. `generate-build-costs` has existing tests covering the facility loader (`facilities_test.go`) and they must pass unchanged — that is the proof the extraction preserved behaviour.

- [ ] **Step 7: Lint**

Run: `golangci-lint run ./pkg/catalog/... ./cmd/generate-build-costs/...`
Expected: no new findings.

- [ ] **Step 8: Commit**

```bash
git add pkg/catalog cmd/generate-build-costs
git commit -m "refactor(catalog): extract snapshot catalog loaders into pkg/catalog"
```

---

### Task 2: `generate-bom-explorer` — build the JSON document

The generator's whole job is one pure transform plus file IO. Keep the transform in `build.go` so it is testable without touching the filesystem.

**Files:**
- Create: `cmd/generate-bom-explorer/build.go`
- Create: `cmd/generate-bom-explorer/build_test.go`
- Create: `cmd/generate-bom-explorer/main.go`

**Interfaces:**
- Consumes: `catalog.LoadShips`, `catalog.LoadFacilities`, `catalog.Ship`, `catalog.Facility`, `catalog.Material` (Task 1); `bom.Recipe`, `bom.RecipeItem`, `bom.BuildRecipeMaps`, `bom.SelectRecipe` (existing `pkg/bom`).
- Produces: `Doc`, `ItemRec`, `RecipeRec`, `TargetRec`, and `BuildDoc(items map[string]ItemRec, recipes map[string]RecipeRec, ships []catalog.Ship, facs []catalog.Facility) Doc`. The JSON field names `items`/`recipes`/`targets`/`defaults` and the short keys `n`/`c`/`i`/`o`/`t`/`bm` are the contract the JavaScript in Tasks 3–8 reads.

**Background — existing `pkg/bom` API you will call:**

```go
type RecipeItem struct { ItemID string; Quantity int }
type Recipe struct { ID string; Inputs []RecipeItem; Outputs []RecipeItem }
func BuildRecipeMaps(recipes map[string]*Recipe) (map[string][]*Recipe, error)
func SelectRecipe(itemToRecipes map[string][]*Recipe, itemID string) *Recipe
```

`SelectRecipe` applies its own filtering layers, the first of which drops `wrap_*` / `unwrap_*`. Call it on the **full** recipe set (packaging recipes included) so the chosen default matches what the static per-target pages already display; only then drop packaging recipes from the emitted `recipes` map.

**Background — target types:** ships and facilities are *sinks*. They carry a `build_materials` list but never appear as an input to anything, so they only ever occupy the rightmost column of the graph.

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-bom-explorer/build_test.go`:

```go
package main

import (
	"encoding/json"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// fixture returns a small but structurally complete world:
//   iron_ore (ore, leaf)
//   steel_plate  <- smelt_steel (5 iron_ore -> 2 steel_plate)
//                <- cast_steel  (6 iron_ore -> 2 steel_plate)   [second recipe]
//   crate        <- wrap_crate (1 steel_plate -> 1 crate)       [packaging, dropped]
func fixture() (map[string]ItemRec, map[string]RecipeRec) {
	items := map[string]ItemRec{
		"iron_ore":    {Name: "Iron Ore", Category: "ore"},
		"steel_plate": {Name: "Steel Plate", Category: "refined"},
		"crate":       {Name: "Crate", Category: "misc"},
	}
	recipes := map[string]RecipeRec{
		"smelt_steel": {
			Name: "Smelt Steel", Category: "Refining",
			Inputs:  [][]any{{"iron_ore", 5}},
			Outputs: [][]any{{"steel_plate", 2}},
		},
		"cast_steel": {
			Name: "Cast Steel", Category: "Refining",
			Inputs:  [][]any{{"iron_ore", 6}},
			Outputs: [][]any{{"steel_plate", 2}},
		},
		"wrap_crate": {
			Name: "Wrap Crate", Category: "Logistics",
			Inputs:  [][]any{{"steel_plate", 1}},
			Outputs: [][]any{{"crate", 1}},
		},
	}
	return items, recipes
}

func TestBuildDocDropsPackagingRecipes(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)

	if _, ok := doc.Recipes["wrap_crate"]; ok {
		t.Error("wrap_crate present in doc.Recipes; packaging recipes must be dropped")
	}
	if _, ok := doc.Recipes["smelt_steel"]; !ok {
		t.Error("smelt_steel missing from doc.Recipes")
	}
	if len(doc.Recipes) != 2 {
		t.Errorf("len(doc.Recipes) = %d, want 2", len(doc.Recipes))
	}
}

func TestBuildDocKeepsAllItems(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)

	// Every item stays, including ones only a dropped packaging recipe made.
	// The page needs their display names for tables and node labels.
	if len(doc.Items) != 3 {
		t.Errorf("len(doc.Items) = %d, want 3", len(doc.Items))
	}
	if doc.Items["iron_ore"].Category != "ore" {
		t.Errorf("iron_ore category = %q, want ore", doc.Items["iron_ore"].Category)
	}
}

func TestBuildDocDefaultsOnlyForMultiRecipeItems(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)

	got, ok := doc.Defaults["steel_plate"]
	if !ok {
		t.Fatal("steel_plate missing from doc.Defaults; it has two recipes")
	}
	if got != "smelt_steel" && got != "cast_steel" {
		t.Errorf("default for steel_plate = %q, want one of smelt_steel/cast_steel", got)
	}
	// crate is produced only by the dropped packaging recipe -> no default.
	if _, ok := doc.Defaults["crate"]; ok {
		t.Error("crate present in doc.Defaults; its only recipe is packaging")
	}
	if len(doc.Defaults) != 1 {
		t.Errorf("len(doc.Defaults) = %d, want 1 (only steel_plate)", len(doc.Defaults))
	}
}

func TestBuildDocTargetsFromShipsAndFacilities(t *testing.T) {
	items, recipes := fixture()
	ships := []catalog.Ship{{
		ID: "absence", Name: "Absence",
		BuildMaterials: []catalog.Material{{ItemID: "steel_plate", Quantity: 5}},
	}}
	facs := []catalog.Facility{
		{ID: "depot", Name: "Depot", BuildMaterials: []catalog.Material{{ItemID: "steel_plate", Quantity: 8150.0}}},
		{ID: "empty_pad", Name: "Empty Pad"}, // no build_materials -> omitted
	}

	doc := BuildDoc(items, recipes, ships, facs)

	if len(doc.Targets) != 2 {
		t.Fatalf("len(doc.Targets) = %d, want 2 (empty_pad has no build_materials)", len(doc.Targets))
	}
	if doc.Targets["absence"].Type != "ship" {
		t.Errorf("absence type = %q, want ship", doc.Targets["absence"].Type)
	}
	if doc.Targets["depot"].Type != "facility" {
		t.Errorf("depot type = %q, want facility", doc.Targets["depot"].Type)
	}
	if _, ok := doc.Targets["empty_pad"]; ok {
		t.Error("empty_pad present; targets without build_materials must be omitted")
	}
}

func TestBuildDocConvertsFloatQuantitiesToInt(t *testing.T) {
	items, recipes := fixture()
	facs := []catalog.Facility{{
		ID: "depot", Name: "Depot",
		BuildMaterials: []catalog.Material{{ItemID: "steel_plate", Quantity: 8150.0}},
	}}

	doc := BuildDoc(items, recipes, nil, facs)

	raw, err := json.Marshal(doc.Targets["depot"].BuildMaterials)
	if err != nil {
		t.Fatal(err)
	}
	// Must serialise as an integer, not 8150.0 — the page does integer math.
	if string(raw) != `[["steel_plate",8150]]` {
		t.Errorf("build materials JSON = %s, want [[\"steel_plate\",8150]]", raw)
	}
}

func TestBuildDocSerialisesShortKeys(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)

	raw, err := json.Marshal(doc.Items["iron_ore"])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"n":"Iron Ore","c":"ore"}` {
		t.Errorf("item JSON = %s, want {\"n\":\"Iron Ore\",\"c\":\"ore\"}", raw)
	}

	raw, err = json.Marshal(doc.Recipes["smelt_steel"])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"n":"Smelt Steel","c":"Refining","i":[["iron_ore",5]],"o":[["steel_plate",2]]}`
	if string(raw) != want {
		t.Errorf("recipe JSON = %s, want %s", raw, want)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/generate-bom-explorer/`
Expected: FAIL — `undefined: BuildDoc`, `undefined: ItemRec`, and so on.

- [ ] **Step 3: Write the implementation**

Create `cmd/generate-bom-explorer/build.go`:

```go
package main

import (
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/bom"
	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// ItemRec is one entry of the "items" map: display name and catalog category.
type ItemRec struct {
	Name     string `json:"n"`
	Category string `json:"c"`
}

// RecipeRec is one entry of the "recipes" map. Inputs and Outputs are
// [item_id, quantity] pairs, kept as [][]any so they serialise as compact
// arrays rather than objects — the file ships ~750 of these.
type RecipeRec struct {
	Name     string  `json:"n"`
	Category string  `json:"c"`
	Inputs   [][]any `json:"i"`
	Outputs  [][]any `json:"o"`
}

// TargetRec is one entry of the "targets" map: a ship or facility. These are
// sinks — they consume items but nothing consumes them — so they only ever
// occupy the rightmost column of the graph.
type TargetRec struct {
	Name           string  `json:"n"`
	Type           string  `json:"t"` // "ship" or "facility"
	BuildMaterials [][]any `json:"bm"`
}

// Doc is the generated recipe-graph.json document.
type Doc struct {
	Items    map[string]ItemRec   `json:"items"`
	Recipes  map[string]RecipeRec `json:"recipes"`
	Targets  map[string]TargetRec `json:"targets"`
	Defaults map[string]string    `json:"defaults"`
}

// isPackaging reports whether a recipe id is one of the wrap_/unwrap_ pairs
// that form X <-> contained_X cycles in the source data. They are never a
// legitimate production path, and dropping them here makes those cycles
// unreachable from the page rather than something it must filter.
func isPackaging(recipeID string) bool {
	return strings.HasPrefix(recipeID, "wrap_") || strings.HasPrefix(recipeID, "unwrap_")
}

// BuildDoc transforms loaded crafting rows and snapshot catalogs into the
// document written to recipe-graph.json.
func BuildDoc(items map[string]ItemRec, recipes map[string]RecipeRec, ships []catalog.Ship, facs []catalog.Facility) Doc {
	doc := Doc{
		Items:    items,
		Recipes:  make(map[string]RecipeRec, len(recipes)),
		Targets:  make(map[string]TargetRec, len(ships)+len(facs)),
		Defaults: make(map[string]string),
	}

	for id, r := range recipes {
		if isPackaging(id) {
			continue
		}
		doc.Recipes[id] = r
	}

	doc.Defaults = computeDefaults(recipes, doc.Recipes)

	for _, s := range ships {
		if len(s.BuildMaterials) == 0 {
			continue
		}
		doc.Targets[s.ID] = TargetRec{Name: s.Name, Type: "ship", BuildMaterials: materialPairs(s.BuildMaterials)}
	}
	for _, f := range facs {
		if len(f.BuildMaterials) == 0 {
			continue
		}
		doc.Targets[f.ID] = TargetRec{Name: f.Name, Type: "facility", BuildMaterials: materialPairs(f.BuildMaterials)}
	}

	return doc
}

// materialPairs converts catalog materials to [item_id, quantity] pairs,
// truncating the float quantities the facility catalog uses to int so the
// page does integer arithmetic throughout. Order follows the catalog.
func materialPairs(ms []catalog.Material) [][]any {
	out := make([][]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, []any{m.ItemID, int(m.Quantity)})
	}
	return out
}

// computeDefaults returns the default recipe for each item that more than one
// non-packaging recipe produces.
//
// Selection runs through bom.SelectRecipe over the FULL recipe set, packaging
// included, because SelectRecipe's own first filtering layer drops packaging.
// Going through it rather than reimplementing the choice is what guarantees
// the explorer opens on the same recipe path the static per-target pages show.
func computeDefaults(all, kept map[string]RecipeRec) map[string]string {
	bomRecipes := make(map[string]*bom.Recipe, len(all))
	for id, r := range all {
		bomRecipes[id] = &bom.Recipe{ID: id, Inputs: toRecipeItems(r.Inputs), Outputs: toRecipeItems(r.Outputs)}
	}
	itemToRecipes, err := bom.BuildRecipeMaps(bomRecipes)
	if err != nil {
		// BuildRecipeMaps only errors on malformed input, which cannot happen
		// for rows read straight out of the schema-constrained tables.
		panic("bom.BuildRecipeMaps: " + err.Error())
	}

	// Count producers among the KEPT recipes: an item made only by packaging
	// has no choice to offer.
	producers := map[string][]string{}
	for id, r := range kept {
		for _, o := range r.Outputs {
			item, _ := o[0].(string)
			producers[item] = append(producers[item], id)
		}
	}

	defaults := make(map[string]string)
	for item, ids := range producers {
		if len(ids) < 2 {
			continue
		}
		chosen := bom.SelectRecipe(itemToRecipes, item)
		if chosen == nil {
			// No structural preference; fall back to the lexically smallest id
			// so regeneration stays deterministic.
			sort.Strings(ids)
			defaults[item] = ids[0]
			continue
		}
		defaults[item] = chosen.ID
	}
	return defaults
}

// toRecipeItems converts [item_id, quantity] pairs to pkg/bom's representation.
func toRecipeItems(pairs [][]any) []bom.RecipeItem {
	out := make([]bom.RecipeItem, 0, len(pairs))
	for _, p := range pairs {
		id, _ := p[0].(string)
		qty, _ := p[1].(int)
		out = append(out, bom.RecipeItem{ItemID: id, Quantity: qty})
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/generate-bom-explorer/ -v`
Expected: PASS, all six tests.

- [ ] **Step 5: Write `main.go`**

Create `cmd/generate-bom-explorer/main.go`:

```go
// Command generate-bom-explorer writes the recipe graph that the interactive
// Bill of Materials explorer page loads. It reads only crafting.db and the
// newest game-API snapshot catalogs — no market data — so the output never
// goes stale against market captures.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
	_ "modernc.org/sqlite"
)

func main() {
	craftPath := flag.String("crafting", "crafting.db", "crafting DB")
	catalogRoot := flag.String("catalog", "data/snapshots", "game-api snapshot catalog root")
	out := flag.String("out", "kb/build-costs/recipe-graph.json", "output JSON path")
	flag.Parse()

	craftDB, err := sql.Open("sqlite", "file:"+*craftPath+"?mode=ro")
	must(err, "open crafting")
	defer func() { _ = craftDB.Close() }()

	items, err := loadItems(craftDB)
	must(err, "load items")
	recipes, err := loadRecipes(craftDB)
	must(err, "load recipes")
	ships, err := catalog.LoadShips(*catalogRoot)
	must(err, "load ships")
	facs, err := catalog.LoadFacilities(*catalogRoot)
	must(err, "load facilities")

	doc := BuildDoc(items, recipes, ships, facs)

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		must(err, "create output dir")
	}
	blob, err := json.Marshal(doc)
	must(err, "marshal")
	must(os.WriteFile(*out, blob, 0o644), "write")

	log.Printf("bom-explorer: %d items, %d recipes, %d targets, %d defaults, %d bytes → %s",
		len(doc.Items), len(doc.Recipes), len(doc.Targets), len(doc.Defaults), len(blob), *out)
}

// loadItems reads every row of the items table.
func loadItems(db *sql.DB) (map[string]ItemRec, error) {
	rows, err := db.Query(`SELECT id, name, COALESCE(category,'') FROM items`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]ItemRec{}
	for rows.Next() {
		var id, name, cat string
		if err := rows.Scan(&id, &name, &cat); err != nil {
			return nil, err
		}
		out[id] = ItemRec{Name: name, Category: cat}
	}
	return out, rows.Err()
}

// loadRecipes reads every recipe with its inputs and outputs. Input and output
// pairs are ordered by item id so regeneration is byte-identical run to run.
func loadRecipes(db *sql.DB) (map[string]RecipeRec, error) {
	rows, err := db.Query(`SELECT id, name, COALESCE(category,'') FROM recipes`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]RecipeRec{}
	for rows.Next() {
		var id, name, cat string
		if err := rows.Scan(&id, &name, &cat); err != nil {
			return nil, err
		}
		out[id] = RecipeRec{Name: name, Category: cat}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := attachPairs(db, `SELECT recipe_id, item_id, quantity FROM recipe_inputs ORDER BY recipe_id, item_id`,
		out, func(r *RecipeRec, p []any) { r.Inputs = append(r.Inputs, p) }); err != nil {
		return nil, err
	}
	return out, attachPairs(db, `SELECT recipe_id, item_id, quantity FROM recipe_outputs ORDER BY recipe_id, item_id`,
		out, func(r *RecipeRec, p []any) { r.Outputs = append(r.Outputs, p) })
}

func attachPairs(db *sql.DB, query string, out map[string]RecipeRec, add func(*RecipeRec, []any)) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rid, iid string
		var qty int
		if err := rows.Scan(&rid, &iid, &qty); err != nil {
			return err
		}
		rec, ok := out[rid]
		if !ok {
			continue
		}
		add(&rec, []any{iid, qty})
		out[rid] = rec
	}
	return rows.Err()
}

func must(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Run the generator against the real database**

Run: `go run ./cmd/generate-bom-explorer`
Expected log line: `bom-explorer: 752 items, 741 recipes, 2985 targets, 62 defaults, <N> bytes → kb/build-costs/recipe-graph.json`

Verify the counts match the Reference Data table: 752 items, 741 recipes (761 − 20 packaging), 2985 targets (335 ships + 2650 facilities), 62 defaults. If any differs, stop and investigate before continuing — every later task builds on this file.

- [ ] **Step 7: Verify regeneration is byte-identical**

Run:

```bash
go run ./cmd/generate-bom-explorer -out /tmp/rg1.json
go run ./cmd/generate-bom-explorer -out /tmp/rg2.json
cmp /tmp/rg1.json /tmp/rg2.json && echo IDENTICAL
```

Expected: `IDENTICAL`. Go maps serialise with sorted keys, and the input/output pair queries are `ORDER BY`-ed, so nothing is left to map iteration order.

- [ ] **Step 8: Lint, build, full test**

Run: `go build ./... && go test ./... && golangci-lint run ./cmd/generate-bom-explorer/...`
Expected: PASS with no new findings.

- [ ] **Step 9: Commit**

```bash
git add cmd/generate-bom-explorer kb/build-costs/recipe-graph.json
git commit -m "feat(bom-explorer): generate the client-side recipe graph JSON"
```

---

### Task 3: JavaScript graph model — expansion and layering

The first half of the client model: turn a target plus a choice map into a DAG, and assign each node a column. No quantities yet.

**Files:**
- Create: `kb/build-costs/bom-explorer.js`
- Create: `tests/js/bom-explorer.test.js`

**Interfaces:**
- Consumes: the `recipe-graph.json` shape from Task 2 — `{items:{id:{n,c}}, recipes:{id:{n,c,i,o}}, targets:{id:{n,t,bm}}, defaults:{itemId:recipeId}}`.
- Produces:
  - `producersOf(data)` → `Map<itemId, recipeId[]>`, recipe ids sorted, built once per data load.
  - `activeRecipeId(data, producers, choices, itemId)` → `recipeId | null`
  - `buildGraph(data, producers, targetId, choices)` → `{targetId, nodes: Map<id, Node>}` where `Node` is `{id, kind, recipeId, yield, inputs: [{id, qty}], leaf, cycle}`
  - `rankNodes(graph)` → `Map<id, number>`

**Background:** the file is loaded by the browser with a plain `<script src>` tag, so it must be a classic script, not an ES module. Exporting for tests is a single guarded line at the end of the file — browsers see `module` as undefined and skip it, Node (which treats `.js` as CommonJS because the repo has no `package.json`) picks it up.

**Node semantics:**
- `kind` is `"item"`, `"ship"`, or `"facility"`. Ships and facilities are sinks: they have `inputs` from their `bm` list, `recipeId: null`, and `yield: 1`.
- `leaf` is true when the item is **terminal**: its category is `ore` or `material`, **or** no recipe in `data.recipes` produces it. Both halves are required. Four items are ores that also have a recipe — `energy_crystal`, `exotic_crystal`, `void_crystal`, `hydrogen_gas` — and the category test is the only thing that stops them being expanded. This mirrors `isTerminal` in `pkg/bom/calculator.go` exactly, which is what makes the explorer's base-materials totals agree with the static build-cost pages. Ores and no-recipe drops are told apart later by `data.items[id].c`.
- Getting this wrong has a second consequence: expanding `energy_crystal` makes `circuit_board → power_cell → energy_crystal → circuit_board` a reachable cycle. With the category test in place there are **zero** reachable cycles anywhere in the graph (verified by strongly-connected-component analysis over all 615 craftable items), which is what keeps the cycle backstop genuinely dead code.
- `cycle` is true when expansion re-encountered an item already on the stack. Packaging recipes are already absent from the data, so this is a backstop, not a routine path — but the page must never hang.

- [ ] **Step 1: Write the failing test**

Create `tests/js/bom-explorer.test.js`:

```js
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const bx = require('../../kb/build-costs/bom-explorer.js');

// A small world exercising every structural case:
//   iron_ore, energy_crystal  - ores (leaves)
//   drop_core                 - no recipe, not an ore (a drop; also a leaf)
//   steel_plate               - two recipes -> a choice, default smelt_steel
//   frame                     - one recipe, consumes steel_plate
//   widget                    - one recipe, consumes frame AND steel_plate (shared)
//   hauler                    - a ship sink consuming widget
function fixture() {
  return {
    items: {
      iron_ore: {n: 'Iron Ore', c: 'ore'},
      energy_crystal: {n: 'Energy Crystal', c: 'ore'},
      drop_core: {n: 'Drop Core', c: 'misc'},
      steel_plate: {n: 'Steel Plate', c: 'refined'},
      frame: {n: 'Frame', c: 'component'},
      widget: {n: 'Widget', c: 'component'},
    },
    recipes: {
      smelt_steel: {n: 'Smelt Steel', c: 'Refining', i: [['iron_ore', 5]], o: [['steel_plate', 2]]},
      cast_steel: {n: 'Cast Steel', c: 'Refining', i: [['iron_ore', 6]], o: [['steel_plate', 2]]},
      weld_frame: {n: 'Weld Frame', c: 'Components', i: [['steel_plate', 3]], o: [['frame', 1]]},
      assemble_widget: {
        n: 'Assemble Widget', c: 'Components',
        i: [['frame', 2], ['steel_plate', 1], ['drop_core', 1]], o: [['widget', 1]],
      },
    },
    targets: {
      hauler: {n: 'Hauler', t: 'ship', bm: [['widget', 4], ['energy_crystal', 2]]},
    },
    defaults: {steel_plate: 'smelt_steel'},
  };
}

test('producersOf maps each item to the recipes that make it, sorted', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.deepStrictEqual(producers.get('steel_plate'), ['cast_steel', 'smelt_steel']);
  assert.deepStrictEqual(producers.get('frame'), ['weld_frame']);
  assert.strictEqual(producers.get('iron_ore'), undefined);
});

test('activeRecipeId prefers an explicit choice over the default', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'steel_plate'), 'smelt_steel');
  assert.strictEqual(
    bx.activeRecipeId(data, producers, {steel_plate: 'cast_steel'}, 'steel_plate'), 'cast_steel');
});

test('activeRecipeId falls back to the sole recipe when there is no default', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'frame'), 'weld_frame');
});

test('activeRecipeId returns null for leaves', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'iron_ore'), null);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'drop_core'), null);
});

test('an ore stays terminal even when a recipe produces it', () => {
  const data = fixture();
  // Real data has four of these: energy_crystal, exotic_crystal, void_crystal,
  // hydrogen_gas. The Go flattener stops at them because of their category, and
  // this page must stop in the same place or its totals stop matching the
  // static build-cost pages.
  data.recipes.synthesise_crystal = {
    n: 'Synthesise Crystal', c: 'Refining',
    i: [['iron_ore', 4]], o: [['energy_crystal', 1]],
  };
  const producers = bx.producersOf(data);

  assert.strictEqual(bx.isTerminalItem(data, producers, 'energy_crystal'), true);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'energy_crystal'), null);

  const g = bx.buildGraph(data, producers, 'hauler', {});
  assert.strictEqual(g.nodes.get('energy_crystal').leaf, true, 'craftable ore must stay a leaf');
  assert.deepStrictEqual(g.nodes.get('energy_crystal').inputs, [], 'and must not be expanded');
});

test('a forced cycle still yields a graph with no backwards edge', () => {
  const data = fixture();
  data.recipes.cycle_steel = {n: 'Cycle Steel', c: 'X', i: [['frame', 1]], o: [['steel_plate', 1]]};
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {steel_plate: 'cycle_steel'});
  const ranks = bx.rankNodes(g);

  // The cycle-closing edge must be gone from the graph entirely, not merely
  // left unrecursed: no ranking of a cyclic graph can satisfy the invariant,
  // so an edge left in place would guarantee a backwards arrow.
  assert.strictEqual(g.nodes.get('steel_plate').cycle, true, 'the cutting node is flagged');
  assert.deepStrictEqual(g.nodes.get('steel_plate').inputs, [],
    'the cycle-closing edge is dropped, not retained');

  const violations = [];
  for (const node of g.nodes.values()) {
    for (const input of node.inputs) {
      if (!(ranks.get(input.id) < ranks.get(node.id))) {
        violations.push(`${input.id}(${ranks.get(input.id)}) -> ${node.id}(${ranks.get(node.id)})`);
      }
    }
  }
  assert.deepStrictEqual(violations, [], 'no edge may point backwards');
});

test('activeRecipeId ignores a choice naming a recipe that does not make the item', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(
    bx.activeRecipeId(data, producers, {steel_plate: 'weld_frame'}, 'steel_plate'), 'smelt_steel');
});

test('buildGraph creates one node per distinct item, not per path', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  // widget, frame, steel_plate, iron_ore, drop_core = 5.
  // steel_plate is reached via frame AND directly from widget, but is one node.
  assert.strictEqual(g.nodes.size, 5);
  assert.strictEqual(g.targetId, 'widget');
});

test('buildGraph marks leaves and records recipe yield', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  assert.strictEqual(g.nodes.get('iron_ore').leaf, true);
  assert.strictEqual(g.nodes.get('drop_core').leaf, true);
  assert.strictEqual(g.nodes.get('steel_plate').leaf, false);
  assert.strictEqual(g.nodes.get('steel_plate').yield, 2);
  assert.strictEqual(g.nodes.get('frame').yield, 1);
});

test('buildGraph expands a ship target from its build materials', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const hauler = g.nodes.get('hauler');
  assert.strictEqual(hauler.kind, 'ship');
  assert.strictEqual(hauler.recipeId, null);
  assert.strictEqual(hauler.yield, 1);
  assert.deepStrictEqual(hauler.inputs, [{id: 'widget', qty: 4}, {id: 'energy_crystal', qty: 2}]);
  assert.ok(g.nodes.has('widget'), 'ship target must expand its materials');
});

test('buildGraph follows an overridden recipe choice', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'steel_plate', {steel_plate: 'cast_steel'});
  assert.strictEqual(g.nodes.get('steel_plate').recipeId, 'cast_steel');
  assert.deepStrictEqual(g.nodes.get('steel_plate').inputs, [{id: 'iron_ore', qty: 6}]);
});

test('buildGraph terminates and marks a node when the choice map forms a cycle', () => {
  const data = fixture();
  // Force a cycle: steel_plate made from a recipe consuming frame, which
  // consumes steel_plate. Not reachable through the UI; the backstop must hold.
  data.recipes.cycle_steel = {n: 'Cycle Steel', c: 'X', i: [['frame', 1]], o: [['steel_plate', 1]]};
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {steel_plate: 'cycle_steel'});
  const revisited = [...g.nodes.values()].filter((n) => n.cycle);
  assert.ok(revisited.length > 0, 'expected at least one node marked cycle');
});

test('rankNodes puts leaves at 0 and the target at the maximum', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  assert.strictEqual(ranks.get('iron_ore'), 0);
  assert.strictEqual(ranks.get('drop_core'), 0);
  assert.strictEqual(ranks.get('steel_plate'), 1);
  assert.strictEqual(ranks.get('frame'), 2);
  assert.strictEqual(ranks.get('widget'), 3);
  assert.strictEqual(Math.max(...ranks.values()), ranks.get(g.targetId));
});

test('every edge runs strictly left to right', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  for (const target of ['widget', 'hauler', 'frame', 'steel_plate']) {
    const g = bx.buildGraph(data, producers, target, {});
    const ranks = bx.rankNodes(g);
    for (const node of g.nodes.values()) {
      for (const input of node.inputs) {
        assert.ok(ranks.get(input.id) < ranks.get(node.id),
          `${target}: rank(${input.id})=${ranks.get(input.id)} must be < rank(${node.id})=${ranks.get(node.id)}`);
      }
    }
  }
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `node --test tests/js/`
Expected: FAIL — `Cannot find module '../../kb/build-costs/bom-explorer.js'`.

- [ ] **Step 3: Write the implementation**

Create `kb/build-costs/bom-explorer.js`:

```js
'use strict';
// Interactive Bill of Materials explorer.
//
// Loaded as a classic script by kb/build-costs/explorer.html and as a
// CommonJS module by tests/js/bom-explorer.test.js. Everything above the
// export guard at the bottom is pure: no DOM, no globals, no fetch.

// ---------------------------------------------------------------------------
// Graph model
// ---------------------------------------------------------------------------

// producersOf indexes which recipes produce each item. Built once per data
// load and threaded through the model functions so no function rebuilds it.
function producersOf(data) {
  const producers = new Map();
  for (const [recipeId, recipe] of Object.entries(data.recipes)) {
    for (const [itemId] of recipe.o) {
      if (!producers.has(itemId)) producers.set(itemId, []);
      producers.get(itemId).push(recipeId);
    }
  }
  for (const ids of producers.values()) ids.sort();
  return producers;
}

// isTerminalItem reports whether expansion stops at itemId: ores and raw
// materials always do, as does anything no recipe produces.
//
// The category test is load-bearing, not belt-and-braces. Four items are ores
// that ALSO have a recipe (energy_crystal, exotic_crystal, void_crystal,
// hydrogen_gas); without it they would be expanded, the base-material totals
// would stop agreeing with the static build-cost pages, and
// circuit_board -> power_cell -> energy_crystal -> circuit_board would become
// a reachable cycle. This mirrors isTerminal in pkg/bom/calculator.go.
function isTerminalItem(data, producers, itemId) {
  const item = data.items[itemId];
  if (item && (item.c === 'ore' || item.c === 'material')) return true;
  const ids = producers.get(itemId);
  return !ids || ids.length === 0;
}

// activeRecipeId resolves which recipe makes itemId under the current choices:
// an explicit choice, else the generated default, else the item's only recipe.
// Returns null for terminal items. A choice naming a recipe that does not
// produce the item is ignored rather than trusted — URLs are user-editable.
function activeRecipeId(data, producers, choices, itemId) {
  if (isTerminalItem(data, producers, itemId)) return null;
  const ids = producers.get(itemId);
  if (!ids || ids.length === 0) return null;
  const chosen = choices[itemId];
  if (chosen && ids.includes(chosen)) return chosen;
  const fallback = data.defaults[itemId];
  if (fallback && ids.includes(fallback)) return fallback;
  return ids[0];
}

// yieldOf returns how many units of itemId one batch of the recipe produces.
function yieldOf(data, recipeId, itemId) {
  for (const [id, qty] of data.recipes[recipeId].o) {
    if (id === itemId) return qty;
  }
  return 1;
}

// buildGraph expands targetId into a DAG of nodes under the given choices.
//
// One node per distinct item, never one per path: an item consumed by three
// parents is a single node with three incoming edges. Ships and facilities are
// sinks — they expand their build-materials list but have no recipe.
function buildGraph(data, producers, targetId, choices) {
  const nodes = new Map();

  function visit(id, stack) {
    if (nodes.has(id)) {
      // Already expanded. Only flag a cycle if it is on the current path.
      if (stack.has(id)) nodes.get(id).cycle = true;
      return;
    }

    const target = data.targets[id];
    if (target) {
      const inputs = target.bm.map(([itemId, qty]) => ({id: itemId, qty}));
      nodes.set(id, {
        id, kind: target.t, recipeId: null, yield: 1, inputs, leaf: false, cycle: false,
      });
      const next = new Set(stack).add(id);
      for (const input of inputs) visit(input.id, next);
      return;
    }

    const recipeId = activeRecipeId(data, producers, choices, id);
    if (recipeId === null) {
      nodes.set(id, {
        id, kind: 'item', recipeId: null, yield: 1, inputs: [], leaf: true, cycle: false,
      });
      return;
    }

    // Build the input list, OMITTING any edge that would close a cycle.
    //
    // Dropping the edge rather than merely declining to recurse is what makes
    // the graph acyclic by construction, and that is the only way the layering
    // invariant can hold: no ranking of a cyclic graph can put every input
    // strictly below its consumer, so leaving the edge in place would
    // guarantee at least one backwards arrow. The node keeps cycle:true so the
    // renderer can say so.
    const next = new Set(stack).add(id);
    const inputs = [];
    let cycle = false;
    for (const [itemId, qty] of data.recipes[recipeId].i) {
      if (next.has(itemId)) {
        cycle = true;
        continue;
      }
      inputs.push({id: itemId, qty});
    }
    nodes.set(id, {
      id, kind: 'item', recipeId, yield: yieldOf(data, recipeId, id), inputs, leaf: false, cycle,
    });

    for (const input of inputs) visit(input.id, next);
  }

  visit(targetId, new Set());
  return {targetId, nodes};
}

// rankNodes assigns each node its column: leaves are 0, and every other node
// is one past the highest-ranked of its inputs.
//
// This is longest-path layering, and it is what guarantees the visual's core
// property: every input has a strictly lower rank than its consumer, so all
// arrows run left to right and none is ever within a column. The target always
// attains the maximum rank, so the output is always rightmost.
function rankNodes(graph) {
  const ranks = new Map();

  function rank(id, stack) {
    if (ranks.has(id)) return ranks.get(id);
    // Defence in depth only: buildGraph already drops cycle-closing edges, so
    // the graph it hands us is acyclic and this branch cannot fire. It stays
    // so a future caller that builds a graph some other way still terminates.
    // Note it cannot repair the invariant on a genuinely cyclic graph — no
    // ranking can — which is why the cycle is broken during construction.
    if (stack.has(id)) return 0;
    const node = graph.nodes.get(id);
    if (!node || node.inputs.length === 0) {
      ranks.set(id, 0);
      return 0;
    }
    const next = new Set(stack).add(id);
    let best = 0;
    for (const input of node.inputs) {
      best = Math.max(best, rank(input.id, next) + 1);
    }
    ranks.set(id, best);
    return best;
  }

  for (const id of graph.nodes.keys()) rank(id, new Set());
  return ranks;
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {producersOf, isTerminalItem, activeRecipeId, yieldOf, buildGraph, rankNodes};
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `node --test tests/js/`
Expected: PASS, all twelve tests.

- [ ] **Step 5: Sanity-check against the real data**

Run:

```bash
node -e '
const bx = require("./kb/build-costs/bom-explorer.js");
const data = require("./kb/build-costs/recipe-graph.json");
const p = bx.producersOf(data);
const g = bx.buildGraph(data, p, "station_core", {});
const r = bx.rankNodes(g);
console.log("nodes:", g.nodes.size, "tiers:", Math.max(...r.values()) + 1);
for (const n of g.nodes.values())
  for (const i of n.inputs)
    if (!(r.get(i.id) < r.get(n.id))) throw new Error("edge not left-to-right: " + i.id + " -> " + n.id);
console.log("layering invariant holds");
'
```

Expected: `nodes: 74 tiers: 10` followed by `layering invariant holds`.

- [ ] **Step 6: Commit**

```bash
git add kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): graph expansion and longest-path layering"
```

---

### Task 4: JavaScript quantity roll-up with whole batches

**Files:**
- Modify: `kb/build-costs/bom-explorer.js` (add `rollUp`, extend the export list)
- Modify: `tests/js/bom-explorer.test.js` (append the roll-up tests)

**Interfaces:**
- Consumes: `buildGraph` and `rankNodes` from Task 3.
- Produces: `rollUp(graph, ranks, quantity)` → `{demand: Map<id, number>, batches: Map<id, number>, surplus: Map<id, number>}`. `demand` is how many units are needed; `batches` how many recipe batches are run, **absent only for leaves** — a ship or facility sink DOES get a `batches` entry (equal to its demand, since its yield is 1), because Task 7 scales the direct-inputs table by `batches.get(target)` and would otherwise show a ship's inputs at quantity 1 no matter what the user asked for; `surplus` holds the over-production from rounding (only non-zero entries).

**Background — why the order matters.** You cannot craft a partial batch, so every tier rounds up to whole batches. Because items are shared between parents, batch counts cannot be decided top-down: an item's batch count depends on its *total* demand across every parent. Computing `ceil` per parent and summing over-counts. Processing nodes in topological order — output first — means an item's demand is final before its batches are computed.

Topological order comes free from the ranks: every edge goes from a higher rank to a strictly lower one, so sorting node ids by **descending rank** is a valid topological order. Nodes of equal rank never have an edge between them, so ties can break any way; break them by id so the result is deterministic.

- [ ] **Step 1: Write the failing test**

Append to `tests/js/bom-explorer.test.js`:

```js
test('rollUp rounds up to whole batches and reports surplus', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  // smelt_steel: 5 iron_ore -> 2 steel_plate. Need 3 plates.
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const {demand, batches, surplus} = bx.rollUp(g, ranks, 3);

  assert.strictEqual(demand.get('steel_plate'), 3);
  assert.strictEqual(batches.get('steel_plate'), 2, 'ceil(3/2) = 2 batches');
  assert.strictEqual(demand.get('iron_ore'), 10, '2 batches x 5 ore');
  assert.strictEqual(surplus.get('steel_plate'), 1, '2 batches x 2 = 4 made, 3 needed');
});

test('rollUp batches a shared item once against summed demand', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  // This is the case that separates the correct algorithm from the naive one,
  // so the numbers must actually diverge. Shrink both plate requirements to 1:
  //   1 widget  -> 1 frame + 1 steel_plate + 1 drop_core
  //   1 frame   -> 1 steel_plate
  //   total steel_plate demand = 1 (direct) + 1 (via frame) = 2
  //   batched ONCE against 2:  ceil(2/2) = 1 batch  -> 5 iron_ore
  //   batched PER PARENT:      ceil(1/2) + ceil(1/2) = 2 batches -> 10 iron_ore
  data.recipes.weld_frame.i = [['steel_plate', 1]];
  data.recipes.assemble_widget.i = [['frame', 1], ['steel_plate', 1], ['drop_core', 1]];

  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const {demand, batches} = bx.rollUp(g, ranks, 1);

  assert.strictEqual(demand.get('frame'), 1);
  assert.strictEqual(demand.get('steel_plate'), 2, 'summed across both parents');
  assert.strictEqual(batches.get('steel_plate'), 1, 'ceil(2/2)=1, not ceil(1/2)+ceil(1/2)=2');
  assert.strictEqual(demand.get('iron_ore'), 5, '1 batch x 5 ore, not 10');
});

test('rollUp scales a ship target by quantity', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const {demand, batches} = bx.rollUp(g, ranks, 3);

  assert.strictEqual(demand.get('hauler'), 3);
  assert.strictEqual(demand.get('widget'), 12, '3 haulers x 4 widgets');
  assert.strictEqual(demand.get('energy_crystal'), 6, '3 haulers x 2 crystals');

  // A sink must carry a batches entry equal to its demand. Task 7 scales the
  // direct-inputs table by batches.get(target), so an absent entry would fall
  // back to 1 and show a ship's inputs at quantity 1 whatever was asked for.
  assert.strictEqual(batches.get('hauler'), 3, 'sinks get a batches entry');
});

test('rollUp reports no surplus when every yield divides evenly', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const {surplus} = bx.rollUp(g, ranks, 4); // 2 batches x 2 = exactly 4
  assert.strictEqual(surplus.size, 0);
});

test('rollUp leaves have demand but no batches', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const {demand, batches} = bx.rollUp(g, ranks, 1);
  assert.strictEqual(demand.get('iron_ore'), 5);
  assert.strictEqual(batches.has('iron_ore'), false);
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `node --test tests/js/`
Expected: FAIL — `bx.rollUp is not a function`.

- [ ] **Step 3: Write the implementation**

Add to `kb/build-costs/bom-explorer.js`, immediately after `rankNodes`:

```js
// topoOrder returns node ids ordered output-first. Every edge runs from a
// higher rank to a strictly lower one, so descending rank is a valid
// topological order; ties break by id so the result is deterministic.
function topoOrder(graph, ranks) {
  return [...graph.nodes.keys()].sort((a, b) => {
    const d = ranks.get(b) - ranks.get(a);
    return d !== 0 ? d : (a < b ? -1 : a > b ? 1 : 0);
  });
}

// rollUp computes how much of each item the build needs, in whole batches.
//
// Batch counts cannot be decided top-down, because a shared item's batch count
// depends on its TOTAL demand across every parent: ceil-ing per parent and
// summing over-counts. Walking output-first in topological order means an
// item's demand is final by the time its batches are computed.
function rollUp(graph, ranks, quantity) {
  const demand = new Map();
  const batches = new Map();
  const surplus = new Map();

  demand.set(graph.targetId, quantity);

  for (const id of topoOrder(graph, ranks)) {
    const node = graph.nodes.get(id);
    const need = demand.get(id) || 0;
    if (need === 0 || node.inputs.length === 0) continue;

    const perBatch = node.yield || 1;
    const runs = Math.ceil(need / perBatch);
    batches.set(id, runs);
    const made = runs * perBatch;
    if (made > need) surplus.set(id, made - need);

    for (const input of node.inputs) {
      demand.set(input.id, (demand.get(input.id) || 0) + runs * input.qty);
    }
  }

  return {demand, batches, surplus};
}
```

Extend the export line at the bottom of the file to:

```js
  module.exports = {producersOf, isTerminalItem, activeRecipeId, yieldOf, buildGraph, rankNodes, topoOrder, rollUp};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `node --test tests/js/`
Expected: PASS, all seventeen tests.

- [ ] **Step 5: Commit**

```bash
git add kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): whole-batch quantity roll-up over the shared DAG"
```

---

### Task 5: JavaScript column ordering and layout geometry

Turn ranks into drawable coordinates.

**Files:**
- Modify: `kb/build-costs/bom-explorer.js` (add `orderColumns` and `layout`, extend exports)
- Modify: `tests/js/bom-explorer.test.js` (append layout tests)

**Interfaces:**
- Consumes: `buildGraph`, `rankNodes` (Task 3), `rollUp` (Task 4).
- Produces:
  - `orderColumns(graph, ranks)` → `string[][]`, index = rank, each entry the ordered node ids of that column.
  - `layout(graph, ranks, columns, producers)` → `{width, height, boxes: Map<id, {x, y, w, h, col, row}>, edges: [{from, to, qty, points: [[x,y],...]}]}`. `producers` is optional: pass it so boxes carrying a recipe selector get the taller height, omit it (as the tests do) for plain geometry.

**Background — barycentre ordering.** Within a column, order nodes by the mean vertical position of their already-placed neighbours in the column to the right (consumers). Start from the rightmost column (the target, a single node) and work leftwards, so every column is ordered against one that is already final.

**One sweep, not two.** A second right-to-left pass cannot change anything — each column depends only on the finalised column to its right — and an alternating right-left-right sweep was measured to give byte-identical orderings and identical crossing counts on the largest real graphs (`overmind`, 135 nodes: 161 crossings under every scheme tried). Write the single sweep and say why in the comment.

**Layout constants** (define at the top of the layout section so they are tunable in one place):

```js
const BOX_W = 150;      // node box width
const BOX_H = 46;       // node box height without a recipe selector
const BOX_H_SEL = 66;   // node box height with a recipe selector
const COL_GAP = 90;     // horizontal gutter between columns
const ROW_GAP = 14;     // vertical gap between boxes in a column
const MARGIN = 20;      // canvas padding
```

Column 0 (leaves) is drawn leftmost, so a node's x is `MARGIN + col * (BOX_W + COL_GAP)`. Boxes in a column stack top to bottom, each column vertically centred against the tallest column so short columns do not hug the top.

Edges are elbow polylines: leave the input box at its right edge, run to the horizontal midpoint of the gutter, turn vertically, then run into the consumer's left edge. A long edge spanning several columns uses the gutter immediately left of its consumer.

- [ ] **Step 1: Write the failing test**

Append to `tests/js/bom-explorer.test.js`:

```js
test('orderColumns indexes columns by rank with the target alone on the right', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const columns = bx.orderColumns(g, ranks);

  assert.strictEqual(columns.length, 4, 'ranks 0..3');
  assert.deepStrictEqual(columns[3], ['widget']);
  assert.deepStrictEqual(columns[2], ['frame']);
  assert.deepStrictEqual(columns[1], ['steel_plate']);
  // Assert exact order, not just membership. These two are NOT tied:
  // iron_ore's consumer (steel_plate) sits in the adjacent column so it gets a
  // real barycentre of 0, while drop_core's only consumer (widget) is three
  // columns away, so it has no barycentre and sorts to the bottom. This pins
  // the distant-consumer behaviour that the sweep documents.
  assert.deepStrictEqual(columns[0], ['iron_ore', 'drop_core']);
});

test('orderColumns places every node exactly once', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const columns = bx.orderColumns(g, ranks);

  const placed = columns.flat();
  assert.strictEqual(placed.length, g.nodes.size);
  assert.strictEqual(new Set(placed).size, g.nodes.size, 'no duplicates');
  for (const id of g.nodes.keys()) assert.ok(placed.includes(id), `${id} missing`);
});

test('orderColumns is deterministic across repeated calls', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  assert.deepStrictEqual(bx.orderColumns(g, ranks), bx.orderColumns(g, ranks));
});

test('layout puts lower columns to the left and never overlaps boxes', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const columns = bx.orderColumns(g, ranks);
  const {boxes, width, height} = bx.layout(g, ranks, columns);

  assert.ok(boxes.get('iron_ore').x < boxes.get('steel_plate').x);
  assert.ok(boxes.get('steel_plate').x < boxes.get('frame').x);
  assert.ok(boxes.get('frame').x < boxes.get('widget').x);

  // No two boxes in the same column overlap vertically.
  for (const column of columns) {
    const sorted = column.map((id) => boxes.get(id)).sort((a, b) => a.y - b.y);
    for (let i = 1; i < sorted.length; i++) {
      assert.ok(sorted[i].y >= sorted[i - 1].y + sorted[i - 1].h,
        'boxes in a column must not overlap');
    }
  }

  // Canvas contains every box.
  for (const b of boxes.values()) {
    assert.ok(b.x >= 0 && b.y >= 0 && b.x + b.w <= width && b.y + b.h <= height);
  }
});

test('layout emits one edge per input with its quantity', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const {edges} = bx.layout(g, ranks, bx.orderColumns(g, ranks));

  let total = 0;
  for (const n of g.nodes.values()) total += n.inputs.length;
  assert.strictEqual(edges.length, total);

  const direct = edges.find((e) => e.from === 'steel_plate' && e.to === 'widget');
  assert.ok(direct, 'expected the direct steel_plate -> widget edge');
  assert.strictEqual(direct.qty, 1);
  assert.ok(direct.points.length >= 2, 'edge must have a polyline');
});

test('layout handles the two-box refining degenerate case', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const {boxes, edges} = bx.layout(g, ranks, bx.orderColumns(g, ranks));

  assert.strictEqual(boxes.size, 2);
  assert.strictEqual(edges.length, 1);
  assert.strictEqual(edges[0].qty, 5);
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `node --test tests/js/`
Expected: FAIL — `bx.orderColumns is not a function`.

- [ ] **Step 3: Write the implementation**

Add to `kb/build-costs/bom-explorer.js`, after `rollUp`:

```js
// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

const BOX_W = 150;
const BOX_H = 46;
const BOX_H_SEL = 66;
const COL_GAP = 90;
const ROW_GAP = 14;
const MARGIN = 20;

// orderColumns groups nodes by rank and orders each column to reduce edge
// crossings: a node sorts to the mean vertical position of its consumers in
// the column to its right. The sweep runs right-to-left from the target (a
// single node), so each column is ordered against a column that is already
// final. Ties break by id, so repeated calls agree and determinism does not
// rely on Array.prototype.sort being stable.
//
// ONE sweep, deliberately. A second right-to-left pass is provably a no-op —
// each column depends only on the already-finalised column to its right, so
// nothing can change on a repeat — and measurement showed an alternating
// right-left-right sweep produces byte-identical orderings and identical
// crossing counts on the largest real graphs (overmind 135 nodes: 161
// crossings under every scheme tried). Extra passes would be cost without
// effect.
function orderColumns(graph, ranks) {
  const maxRank = Math.max(...ranks.values());
  const columns = [];
  for (let i = 0; i <= maxRank; i++) columns.push([]);
  for (const id of [...graph.nodes.keys()].sort()) columns[ranks.get(id)].push(id);

  // consumers[id] = ids of nodes that take id as an input.
  const consumers = new Map();
  for (const node of graph.nodes.values()) {
    for (const input of node.inputs) {
      if (!consumers.has(input.id)) consumers.set(input.id, []);
      consumers.get(input.id).push(node.id);
    }
  }

  for (let col = columns.length - 2; col >= 0; col--) {
    const rightPos = new Map();
    columns[col + 1].forEach((id, i) => rightPos.set(id, i));
    const bary = new Map();
    for (const id of columns[col]) {
      // Only consumers in the immediately adjacent column have a position
      // here, so a node whose only consumer is several columns away (a base
      // ore feeding the output directly) has no barycentre and sorts to the
      // bottom. That is the accepted cost of a barycentre pass with no dummy
      // nodes; adding them would be a larger change than the readability
      // gain justifies.
      const positions = (consumers.get(id) || [])
        .map((c) => rightPos.get(c))
        .filter((p) => p !== undefined);
      bary.set(id, positions.length
        ? positions.reduce((a, b) => a + b, 0) / positions.length
        : Number.MAX_SAFE_INTEGER);
    }
    columns[col].sort((a, b) => {
      const d = bary.get(a) - bary.get(b);
      return d !== 0 ? d : (a < b ? -1 : a > b ? 1 : 0);
    });
  }

  return columns;
}

// boxHeight returns a node's height: taller when it carries a recipe selector.
function boxHeight(producers, id) {
  const ids = producers ? producers.get(id) : null;
  return ids && ids.length > 1 ? BOX_H_SEL : BOX_H;
}

// layout converts ordered columns into drawable geometry. Columns are placed
// left to right by rank, so a base ore consumed directly by the output spans
// the full width — expected, not a defect. Each column is vertically centred
// against the tallest so short columns do not hug the top.
//
// producers is optional; pass it to size boxes that carry a recipe selector.
function layout(graph, ranks, columns, producers) {
  const heights = columns.map((column) =>
    column.reduce((sum, id) => sum + boxHeight(producers, id) + ROW_GAP, -ROW_GAP));
  const tallest = Math.max(0, ...heights);

  const boxes = new Map();
  columns.forEach((column, col) => {
    let y = MARGIN + (tallest - heights[col]) / 2;
    column.forEach((id, row) => {
      const h = boxHeight(producers, id);
      boxes.set(id, {x: MARGIN + col * (BOX_W + COL_GAP), y, w: BOX_W, h, col, row});
      y += h + ROW_GAP;
    });
  });

  const width = MARGIN * 2 + columns.length * BOX_W + Math.max(0, columns.length - 1) * COL_GAP;
  const height = MARGIN * 2 + tallest;

  // Elbow polylines: out of the input's right edge, across to the midpoint of
  // the gutter immediately left of the consumer, vertically, then in.
  const edges = [];
  for (const node of graph.nodes.values()) {
    const to = boxes.get(node.id);
    if (!to) continue;
    for (const input of node.inputs) {
      const from = boxes.get(input.id);
      if (!from) continue;
      const x1 = from.x + from.w;
      const y1 = from.y + from.h / 2;
      const x2 = to.x;
      const y2 = to.y + to.h / 2;
      const mid = x2 - COL_GAP / 2;
      const points = y1 === y2
        ? [[x1, y1], [x2, y2]]
        : [[x1, y1], [mid, y1], [mid, y2], [x2, y2]];
      edges.push({from: input.id, to: node.id, qty: input.qty, points});
    }
  }

  return {width, height, boxes, edges};
}
```

Extend the export line to:

```js
  module.exports = {producersOf, isTerminalItem, activeRecipeId, yieldOf, buildGraph, rankNodes,
    topoOrder, rollUp, orderColumns, layout};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `node --test tests/js/`
Expected: PASS, all twenty-three tests.

- [ ] **Step 5: Check the worst case lays out sanely**

Run:

```bash
node -e '
const bx = require("./kb/build-costs/bom-explorer.js");
const data = require("./kb/build-costs/recipe-graph.json");
const p = bx.producersOf(data);
const g = bx.buildGraph(data, p, "station_core", {});
const r = bx.rankNodes(g);
const cols = bx.orderColumns(g, r);
const l = bx.layout(g, r, cols, p);
console.log("canvas", l.width + "x" + l.height, "columns", cols.map(c => c.length).join(","), "edges", l.edges.length);
'
```

Expected: 10 columns, every node placed (74 for `station_core`), a canvas roughly 2350 px wide. Nothing should be zero or `NaN`.

- [ ] **Step 6: Commit**

```bash
git add kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): barycentre column ordering and elbow edge layout"
```

---

### Task 6: JavaScript URL state and the selectable-output set

**Files:**
- Modify: `kb/build-costs/bom-explorer.js` (add `selectableOutputs`, `encodeState`, `decodeState`, extend exports)
- Modify: `tests/js/bom-explorer.test.js` (append state tests)

**Interfaces:**
- Consumes: `producersOf` (Task 3).
- Produces:
  - `selectableOutputs(data, producers)` → `[{id, name, type}]` sorted by name then id, where `type` is `"item"`, `"ship"`, or `"facility"`.
  - `encodeState(data, producers, state)` → query string **without** a leading `?`, where `state` is `{target, qty, choices}`.
  - `decodeState(data, producers, query)` → `{target, qty, choices}`; `target` is `null` when absent or unknown.

**Background:** only choices that differ from the generated default are serialised, so the common URL is just `target=<id>`. Choice pairs are `item:recipe`, comma-separated, and choice keys are sorted so the same state always produces the same URL. Everything invalid is discarded rather than erroring: URLs are user-editable and a bad one must degrade to the defaults, not a broken page.

Quantity clamps to `[1, 99999]` and truncates to an integer.

- [ ] **Step 1: Write the failing test**

Append to `tests/js/bom-explorer.test.js`:

```js
test('selectableOutputs spans craftable items, ships and facilities', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const out = bx.selectableOutputs(data, producers);
  const ids = out.map((o) => o.id);

  assert.ok(ids.includes('steel_plate'), 'craftable item must be selectable');
  assert.ok(ids.includes('widget'));
  assert.ok(ids.includes('hauler'), 'ship must be selectable');
  assert.ok(!ids.includes('iron_ore'), 'ores are not selectable outputs');
  assert.ok(!ids.includes('drop_core'), 'no-recipe drops are not selectable outputs');

  // A craftable ore is still terminal, so it is still not a selectable output.
  data.recipes.synthesise_crystal = {
    n: 'Synthesise Crystal', c: 'Refining',
    i: [['iron_ore', 4]], o: [['energy_crystal', 1]],
  };
  const withCraftableOre = bx.selectableOutputs(data, bx.producersOf(data)).map((o) => o.id);
  assert.ok(!withCraftableOre.includes('energy_crystal'),
    'an ore with a recipe is still terminal, so still not selectable');

  assert.strictEqual(out.find((o) => o.id === 'hauler').type, 'ship');
  assert.strictEqual(out.find((o) => o.id === 'steel_plate').type, 'item');
});

test('selectableOutputs sorts by display name', () => {
  const data = fixture();
  const out = bx.selectableOutputs(data, bx.producersOf(data));
  const names = out.map((o) => o.name);
  assert.deepStrictEqual(names, [...names].sort());
});

test('encodeState omits defaults and the default quantity', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(
    bx.encodeState(data, producers, {target: 'widget', qty: 1, choices: {}}),
    'target=widget');
  assert.strictEqual(
    bx.encodeState(data, producers, {target: 'widget', qty: 5, choices: {steel_plate: 'smelt_steel'}}),
    'target=widget&qty=5',
    'smelt_steel is the default and must not be serialised');
});

test('encodeState serialises non-default choices sorted by item', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const got = bx.encodeState(data, producers,
    {target: 'widget', qty: 1, choices: {steel_plate: 'cast_steel'}});
  assert.strictEqual(got, 'target=widget&r=steel_plate:cast_steel');
});

test('decodeState round-trips an encoded state', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const state = {target: 'widget', qty: 42, choices: {steel_plate: 'cast_steel'}};
  const decoded = bx.decodeState(data, producers, bx.encodeState(data, producers, state));
  assert.deepStrictEqual(decoded, state);
});

test('decodeState clamps quantity into range', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=0').qty, 1);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=-7').qty, 1);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=999999').qty, 99999);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=3.7').qty, 3);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=abc').qty, 1);
});

test('decodeState discards prototype-chain property names as targets', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  // A bare bracket lookup is truthy for every Object.prototype member, so
  // these would pass validation and then throw in buildGraph. The page must
  // degrade, not break, on any hand-edited URL.
  for (const name of ['__proto__', 'constructor', 'toString', 'hasOwnProperty', 'valueOf']) {
    const state = bx.decodeState(data, producers, 'target=' + name);
    assert.strictEqual(state.target, null, `${name} must not validate as a target`);
  }
});

test('decodeState discards unknown targets and bogus choices', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.decodeState(data, producers, 'target=nonesuch').target, null);
  assert.strictEqual(bx.decodeState(data, producers, '').target, null);
  // weld_frame does not produce steel_plate, so the choice is dropped.
  assert.deepStrictEqual(
    bx.decodeState(data, producers, 'target=widget&r=steel_plate:weld_frame').choices, {});
  assert.deepStrictEqual(
    bx.decodeState(data, producers, 'target=widget&r=garbage').choices, {});
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `node --test tests/js/`
Expected: FAIL — `bx.selectableOutputs is not a function`.

- [ ] **Step 3: Write the implementation**

Add to `kb/build-costs/bom-explorer.js`, after `layout`:

```js
// ---------------------------------------------------------------------------
// Selectable outputs and URL state
// ---------------------------------------------------------------------------

const QTY_MIN = 1;
const QTY_MAX = 99999;

// hasOwn tests real membership of a plain object parsed from JSON.
//
// A bare `data.targets[key]` lookup is truthy for every Object.prototype
// member — `__proto__`, `constructor`, `toString`, `hasOwnProperty` — so a
// hand-edited `?target=__proto__` would pass validation and then throw in
// buildGraph. The URL is the one place untrusted keys enter, so this is
// where it gets checked.
function hasOwn(obj, key) {
  return Object.prototype.hasOwnProperty.call(obj, key);
}

// selectableOutputs is everything the user may pick as an output: every ship
// and facility, plus every non-terminal item some recipe produces. Terminal
// items are excluded — the explorer treats them as raw inputs, so offering one
// as an output would render a single leaf box and no tables. That exclusion
// must use isTerminalItem, not merely "has no recipe": four ores DO have
// recipes (energy_crystal, exotic_crystal, void_crystal, hydrogen_gas) and
// still must not be selectable. Derived rather than shipped as a fourth list.
function selectableOutputs(data, producers) {
  const out = [];
  for (const [id, target] of Object.entries(data.targets)) {
    out.push({id, name: target.n, type: target.t});
  }
  for (const id of producers.keys()) {
    const item = data.items[id];
    if (!item) continue;
    if (isTerminalItem(data, producers, id)) continue;
    out.push({id, name: item.n, type: 'item'});
  }
  out.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1
    : a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  return out;
}

// clampQty truncates to an integer inside [QTY_MIN, QTY_MAX]. Anything
// unparseable becomes QTY_MIN rather than blanking the page.
function clampQty(value) {
  const n = Math.trunc(Number(value));
  if (!Number.isFinite(n)) return QTY_MIN;
  return Math.min(QTY_MAX, Math.max(QTY_MIN, n));
}

// encodeState renders state as a query string with no leading '?'. Choices
// equal to the generated default and the default quantity are omitted, so the
// common URL is just target=<id>. Choice keys are sorted for stability.
function encodeState(data, producers, state) {
  const parts = [];
  if (state.target) parts.push('target=' + encodeURIComponent(state.target));
  const qty = clampQty(state.qty);
  if (qty !== QTY_MIN) parts.push('qty=' + qty);

  const pairs = [];
  for (const item of Object.keys(state.choices || {}).sort()) {
    const recipe = state.choices[item];
    const ids = producers.get(item);
    if (!ids || !ids.includes(recipe)) continue;
    if (recipe === data.defaults[item]) continue;
    if (!data.defaults[item] && ids.length < 2) continue;
    pairs.push(item + ':' + recipe);
  }
  // Item and recipe ids are [a-z0-9_] throughout the crafting data, so the
  // pairs need no escaping. Do NOT run them through encodeURIComponent: it
  // escapes the ':' separator to %3A and makes the URL unreadable.
  if (pairs.length) parts.push('r=' + pairs.join(','));

  return parts.join('&');
}

// decodeState parses a query string back into state. Unknown targets, unknown
// recipe ids, recipes that do not produce their item, and out-of-range
// quantities are all discarded in favour of the defaults — URLs are
// user-editable and a bad one must degrade, not break the page.
function decodeState(data, producers, query) {
  const params = new URLSearchParams(query || '');

  // Admit any id that EXISTS — a target, a catalogued item, or something a
  // recipe produces. Whether it is a sensible output is a render-time
  // question, not a parsing one: the spec wants a hand-edited
  // ?target=iron_ore to reach the "this is a raw material" message rather
  // than be silently discarded. hasOwn (not a bare lookup) still keeps
  // Object.prototype names out.
  let target = params.get('target');
  if (target && !hasOwn(data.targets, target) && !hasOwn(data.items, target) &&
      !producers.has(target)) {
    target = null;
  }

  const qty = params.has('qty') ? clampQty(params.get('qty')) : QTY_MIN;

  const choices = {};
  for (const pair of (params.get('r') || '').split(',')) {
    if (!pair) continue;
    const idx = pair.indexOf(':');
    if (idx < 1) continue;
    const item = pair.slice(0, idx);
    const recipe = pair.slice(idx + 1);
    const ids = producers.get(item);
    if (!ids || !ids.includes(recipe)) continue;
    choices[item] = recipe;
  }

  return {target: target || null, qty, choices};
}
```

Extend the export line to:

```js
  module.exports = {producersOf, isTerminalItem, activeRecipeId, yieldOf, buildGraph, rankNodes,
    topoOrder, rollUp, orderColumns, layout, selectableOutputs, clampQty, hasOwn,
    encodeState, decodeState, QTY_MIN, QTY_MAX};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `node --test tests/js/*.test.js`
Expected: PASS, all thirty-three tests.

- [ ] **Step 5: Verify the real selectable count**

Run:

```bash
node -e '
const bx = require("./kb/build-costs/bom-explorer.js");
const data = require("./kb/build-costs/recipe-graph.json");
const out = bx.selectableOutputs(data, bx.producersOf(data));
const by = {};
for (const o of out) by[o.type] = (by[o.type] || 0) + 1;
console.log(out.length, JSON.stringify(by));
'
```

Expected: `3595 {"facility":2650,"item":610,"ship":335}` — matching the Reference Data table. The item count is 610, not 615, for two reasons: `energy_crystal`, `exotic_crystal`, `void_crystal` and `hydrogen_gas` have recipes but are ores, so they are terminal; and `fuel_reserve` is produced by 7 recipes but has no row in the `items` table, so it has no display name or category and is skipped by the `if (!item) continue;` guard. It is the only such gap in the catalog, and it never appears as a recipe input, so it is inert everywhere else.

- [ ] **Step 6: Commit**

```bash
git add kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): selectable outputs and shareable URL state"
```

---

### Task 7: The page shell — markup, data load, autocomplete, quantity, tables

A working page with no graph yet: pick an output, set a quantity, see the three tables. The graph arrives in Task 8.

**Files:**
- Create: `kb/build-costs/explorer.html`
- Modify: `kb/build-costs/bom-explorer.js` (add the DOM layer below the pure functions)

**Interfaces:**
- Consumes: every pure function from Tasks 3–6.
- Produces: the DOM entry point `initExplorer()`, called on `DOMContentLoaded`. It must be a no-op when `document` is undefined so the Node test suite can require the file.

**Background — page conventions.** Copy the `<head>` block (theme bootstrap script, CSS custom properties for both themes, `.theme-toggle` rules) and the `<button class="theme-toggle">` element verbatim from `cmd/generate-build-costs/templates/detail.html.tmpl` lines 1–24. Every KB page carries them and the toggle persists to `localStorage` under `smkb-theme`.

Item catalog links follow `../items/<category>/<id>.html`, using `data.items[id].c` as the category; an item with an empty category gets no link. Ships link to `../ships/all.html`. Facilities have no catalog page and get no link.

The page fetches `recipe-graph.json` from the same directory. That is the established pattern — `kb/did_you_know/hyperspace_warp.html` already fetches `kb/systems/routes.json`, which is 5.4 MB, so an 850 KB fetch is unremarkable.

**Tables to render:**

1. **Base materials** — every leaf node, name and total demand, sorted by name.
2. **Direct inputs** — the target's own inputs (recipe inputs, or `bm` for a ship/facility), name and quantity scaled by the run count, sorted by name.
3. **Surplus from batching** — every entry in the `surplus` map, sorted by name. The whole section is hidden when the map is empty.

**Degenerate cases to handle explicitly:**
- No target selected: show a short prompt, no tables.
- Target is a terminal item (only reachable by a hand-edited URL, since the autocomplete never offers one): render a message and no tables. Ores and materials say they are a raw material the explorer deliberately stops at — accurate even for the four ores that DO have recipes; a non-ore item no recipe produces says it is a drop.

- [ ] **Step 1: Write `explorer.html`**

Create `kb/build-costs/explorer.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Bill of Materials Explorer — Spacemolt KB</title>
<script>(function(){try{var t=localStorage.getItem('smkb-theme');if(!t){t=window.matchMedia&&window.matchMedia('(prefers-color-scheme: light)').matches?'light':'dark';}document.documentElement.setAttribute('data-theme',t);}catch(e){}})();</script>
<style>
:root{--bg:#0d1117;--panel:#161b22;--panel2:#1c2128;--border:#30363d;--border2:#21262d;--text:#c9d1d9;--muted:#8b949e;--muted2:#6e7681;--dim:#adbac7;--link:#58a6ff;--link2:#1f6feb;--accent:#e3b341;--accent2:#d29922;--accent3:#9a6700;--up:#3fb950;--up2:#2ea043;--down:#f85149;--purple:#6c5ce7;--grid:#6e7681}
:root[data-theme="light"]{--bg:#ffffff;--panel:#f6f8fa;--panel2:#eaeef2;--border:#d0d7de;--border2:#d8dee4;--text:#1f2328;--muted:#656d76;--muted2:#8c959f;--dim:#57606a;--link:#0969da;--link2:#0969da;--accent:#9a6700;--accent2:#9a6700;--accent3:#7d4e00;--up:#1a7f37;--up2:#1a7f37;--down:#cf222e;--purple:#6639ba;--grid:#afb8c1}
.theme-toggle{position:fixed;top:.5rem;right:.5rem;z-index:10;background:var(--panel);color:var(--text);border:1px solid var(--border);border-radius:6px;padding:.3rem .55rem;font-size:1rem;cursor:pointer;line-height:1}
.theme-toggle:hover{border-color:var(--link)}

body{font-family:system-ui,sans-serif;margin:0;padding:1rem;background:var(--bg);color:var(--text)}
a{color:var(--link)}
table{border-collapse:collapse;font-size:.85rem;margin-top:.5rem}
th,td{border:1px solid var(--border2);padding:.3rem .5rem;text-align:right}
th:first-child,td:first-child{text-align:left}

.controls{display:flex;flex-wrap:wrap;gap:1rem;align-items:flex-end;background:var(--panel);border:1px solid var(--border);border-radius:6px;padding:.8rem;margin:1rem 0;max-width:60rem}
.controls label{display:block;font-size:.8rem;color:var(--muted);margin-bottom:.25rem}
.controls input{background:var(--panel2);color:var(--text);border:1px solid var(--border);border-radius:4px;padding:.35rem .5rem;font-size:.9rem;font-family:inherit}
#target-input{width:22rem}
#qty-input{width:7rem}

.combo{position:relative}
.combo-list{position:absolute;z-index:20;top:100%;left:0;right:0;max-height:18rem;overflow-y:auto;background:var(--panel2);border:1px solid var(--border);border-radius:4px;margin:2px 0 0;padding:0;list-style:none;display:none}
.combo-list.open{display:block}
.combo-list li{padding:.3rem .5rem;cursor:pointer;display:flex;justify-content:space-between;gap:1rem;font-size:.85rem}
.combo-list li.active,.combo-list li:hover{background:var(--panel)}
.combo-list .type{color:var(--muted2);font-size:.75rem}

.chart-head{margin:1rem 0 .25rem;font-size:.95rem}
.chart-head .recipe{color:var(--accent)}
.chart-head .meta{color:var(--muted);font-size:.85rem;margin-left:.5rem}
#chart{overflow-x:auto;background:var(--panel);border:1px solid var(--border);border-radius:6px;padding:.5rem;min-height:6rem}

.tables{display:flex;flex-wrap:wrap;gap:2rem;margin-top:1rem}
.tables section{min-width:18rem}
.tables h2{font-size:1rem;margin:0}
.tables h2 small{color:var(--muted);font-weight:normal}
.leafkind{color:var(--muted2);font-size:.75rem;margin-left:.35rem}

.note{font-size:.8rem;color:var(--muted);max-width:70ch;margin-top:1.5rem}
.empty{color:var(--muted);font-style:italic}
</style>
</head>
<body>
<button type="button" class="theme-toggle" title="Toggle light/dark theme" aria-label="Toggle light/dark theme" onclick="(function(){var r=document.documentElement,t=r.getAttribute('data-theme')==='light'?'dark':'light';r.setAttribute('data-theme',t);try{localStorage.setItem('smkb-theme',t);}catch(e){}})()">◐</button>

<p><a href="./">← Build Cost Matrix</a></p>
<h1>Bill of Materials Explorer</h1>

<div class="controls">
  <div class="combo">
    <label for="target-input">Output</label>
    <input type="text" id="target-input" autocomplete="off" spellcheck="false"
           placeholder="Type an item, ship or facility name…"
           role="combobox" aria-expanded="false" aria-controls="target-list" aria-autocomplete="list">
    <ul class="combo-list" id="target-list" role="listbox"></ul>
  </div>
  <div>
    <label for="qty-input">Quantity</label>
    <input type="number" id="qty-input" min="1" max="99999" step="1" value="1">
  </div>
</div>

<div id="status" class="empty">Pick an output to see its bill of materials.</div>

<div id="result" hidden>
  <div class="chart-head">
    <span class="recipe" id="chart-recipe"></span><span class="meta" id="chart-meta"></span>
  </div>
  <div id="chart"></div>

  <div class="tables">
    <section>
      <h2>Base materials <small>(flattened)</small></h2>
      <div id="table-base"></div>
    </section>
    <section>
      <h2>Direct inputs <small id="direct-sub"></small></h2>
      <div id="table-direct"></div>
    </section>
    <section id="surplus-section" hidden>
      <h2>Surplus from batching</h2>
      <div id="table-surplus"></div>
    </section>
  </div>

  <p class="note">
    Quantities round up to whole batches at every tier, so totals can exceed the
    per-unit figures on the static build-cost pages — you cannot craft a partial
    batch. Surplus shows what the rounding over-produces. This page carries no
    prices; follow an item's link for its build cost by station.
  </p>
</div>

<script src="bom-explorer.js"></script>
</body>
</html>
```

- [ ] **Step 2: Add the DOM layer**

Add to `kb/build-costs/bom-explorer.js`, after `decodeState` and **before** the export guard:

```js
// ---------------------------------------------------------------------------
// DOM layer
// ---------------------------------------------------------------------------
// Everything below touches the document. It is skipped entirely under Node,
// which loads this file to test the pure functions above.

const MAX_SUGGESTIONS = 50;

// itemHref returns an item's KB catalog page, or '' when it has no category.
function itemHref(data, id) {
  const item = data.items[id];
  if (!item || !item.c) return '';
  return '../items/' + item.c + '/' + id + '.html';
}

// targetHref returns the catalog page for a graph node of any kind.
function targetHref(data, id) {
  const target = data.targets[id];
  if (target) return target.t === 'ship' ? '../ships/all.html' : '';
  return itemHref(data, id);
}

// displayName falls back to the raw id so an unknown item never renders blank.
function displayName(data, id) {
  if (data.targets[id]) return data.targets[id].n;
  return (data.items[id] && data.items[id].n) || id;
}

// leafKind labels a leaf: ores are mined, anything else with no recipe is a
// drop. Keeping them visually distinct stops a drop reading as mineable.
function leafKind(data, id) {
  const item = data.items[id];
  if (!item) return 'unknown';
  return item.c === 'ore' || item.c === 'material' ? 'ore' : 'drop';
}

function initExplorer() {
  const els = {
    target: document.getElementById('target-input'),
    list: document.getElementById('target-list'),
    qty: document.getElementById('qty-input'),
    status: document.getElementById('status'),
    result: document.getElementById('result'),
    recipe: document.getElementById('chart-recipe'),
    meta: document.getElementById('chart-meta'),
    chart: document.getElementById('chart'),
    base: document.getElementById('table-base'),
    direct: document.getElementById('table-direct'),
    directSub: document.getElementById('direct-sub'),
    surplus: document.getElementById('table-surplus'),
    surplusSection: document.getElementById('surplus-section'),
  };

  let data = null;
  let producers = null;
  let outputs = [];
  let state = {target: null, qty: QTY_MIN, choices: {}};
  let suggestions = [];
  let activeIndex = -1;

  fetch('recipe-graph.json')
    .then((r) => {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    })
    .then((doc) => {
      data = doc;
      producers = producersOf(data);
      outputs = selectableOutputs(data, producers);
      state = decodeState(data, producers, window.location.search.slice(1));
      if (state.target) els.target.value = displayName(data, state.target);
      els.qty.value = state.qty;
      render();
    })
    .catch((err) => {
      els.status.textContent = 'Could not load recipe-graph.json: ' + err.message;
    });

  // -- autocomplete ---------------------------------------------------------

  function matches(query) {
    const q = query.trim().toLowerCase();
    if (!q) return outputs.slice(0, MAX_SUGGESTIONS);
    const hits = [];
    for (const o of outputs) {
      if (o.name.toLowerCase().includes(q) || o.id.includes(q)) {
        hits.push(o);
        if (hits.length >= MAX_SUGGESTIONS) break;
      }
    }
    return hits;
  }

  function closeList() {
    els.list.classList.remove('open');
    els.target.setAttribute('aria-expanded', 'false');
    activeIndex = -1;
  }

  function openList(query) {
    suggestions = matches(query);
    els.list.innerHTML = '';
    for (const [i, o] of suggestions.entries()) {
      const li = document.createElement('li');
      li.setAttribute('role', 'option');
      const name = document.createElement('span');
      name.textContent = o.name;
      const type = document.createElement('span');
      type.className = 'type';
      type.textContent = o.type;
      li.append(name, type);
      li.addEventListener('mousedown', (e) => {
        e.preventDefault();
        choose(i);
      });
      els.list.append(li);
    }
    els.list.classList.toggle('open', suggestions.length > 0);
    els.target.setAttribute('aria-expanded', String(suggestions.length > 0));
    activeIndex = -1;
  }

  function highlight(delta) {
    if (!suggestions.length) return;
    activeIndex = (activeIndex + delta + suggestions.length) % suggestions.length;
    [...els.list.children].forEach((li, i) => li.classList.toggle('active', i === activeIndex));
    els.list.children[activeIndex].scrollIntoView({block: 'nearest'});
  }

  function choose(index) {
    const picked = suggestions[index];
    if (!picked) return;
    els.target.value = picked.name;
    // A new output invalidates the old choice map: its items may not appear.
    state = {target: picked.id, qty: state.qty, choices: {}};
    closeList();
    render();
  }

  els.target.addEventListener('input', () => openList(els.target.value));
  els.target.addEventListener('focus', () => openList(els.target.value));
  els.target.addEventListener('blur', closeList);
  els.target.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); highlight(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); highlight(-1); }
    else if (e.key === 'Enter') { e.preventDefault(); choose(activeIndex >= 0 ? activeIndex : 0); }
    else if (e.key === 'Escape') closeList();
  });

  els.qty.addEventListener('change', () => {
    state.qty = clampQty(els.qty.value);
    els.qty.value = state.qty;
    render();
  });

  // -- rendering ------------------------------------------------------------

  function table(rows, headers) {
    if (!rows.length) return '<p class="empty">None.</p>';
    const head = '<thead><tr><th>' + headers.join('</th><th>') + '</th></tr></thead>';
    const body = rows.map((r) => '<tr><td>' + r[0] + '</td><td>' + r[1] + '</td></tr>').join('');
    return '<table>' + head + '<tbody>' + body + '</tbody></table>';
  }

  function nameCell(id, suffix) {
    const href = targetHref(data, id);
    const name = escapeHTML(displayName(data, id));
    const label = href ? '<a href="' + href + '">' + name + '</a>' : name;
    return suffix ? label + '<span class="leafkind">' + suffix + '</span>' : label;
  }

  function byName(a, b) {
    return displayName(data, a) < displayName(data, b) ? -1
      : displayName(data, a) > displayName(data, b) ? 1 : 0;
  }

  function render() {
    if (!data) return;

    const url = encodeState(data, producers, state);
    window.history.replaceState(null, '', url ? '?' + url : window.location.pathname);

    if (!state.target) {
      els.status.hidden = false;
      els.status.textContent = 'Pick an output to see its bill of materials.';
      els.result.hidden = true;
      return;
    }

    // Terminal items are raw inputs to this page, so there is nothing to
    // expand. Say which kind, accurately: four ores DO have recipes
    // (energy_crystal, exotic_crystal, void_crystal, hydrogen_gas) and are
    // terminal by category, so "no recipe produces this" would be false for
    // them. Only a non-ore terminal item is genuinely recipe-less.
    const isTarget = hasOwn(data.targets, state.target);
    if (!isTarget && isTerminalItem(data, producers, state.target)) {
      els.status.hidden = false;
      els.status.textContent = leafKind(data, state.target) === 'ore'
        ? displayName(data, state.target) +
          ' is a raw material — the explorer stops at ores and raw materials, ' +
          'the same place the static build-cost pages stop, so there is nothing to expand.'
        : displayName(data, state.target) +
          ' — no recipe produces this; it is a drop.';
      els.result.hidden = true;
      return;
    }

    els.status.hidden = true;
    els.result.hidden = false;

    const graph = buildGraph(data, producers, state.target, state.choices);
    const ranks = rankNodes(graph);
    const columns = orderColumns(graph, ranks);
    const totals = rollUp(graph, ranks, state.qty);
    const node = graph.nodes.get(state.target);

    els.recipe.textContent = node.recipeId
      ? node.recipeId
      : displayName(data, state.target) + ' (' + node.kind + ' build materials)';
    els.meta.textContent = columns.length + ' tiers, ' + graph.nodes.size + ' items';

    // Base materials: every leaf, by total demand.
    const leaves = [...graph.nodes.values()].filter((n) => n.leaf).map((n) => n.id).sort(byName);
    els.base.innerHTML = table(
      leaves.map((id) => [
        nameCell(id, leafKind(data, id) === 'drop' ? 'drop' : ''),
        (totals.demand.get(id) || 0).toLocaleString(),
      ]),
      ['Material', 'Qty']);

    // Direct inputs: the target's own inputs, scaled by its batch count.
    const runs = totals.batches.get(state.target) || 1;
    const direct = [...node.inputs].sort((a, b) => byName(a.id, b.id));
    els.directSub.textContent = node.recipeId ? '(' + node.recipeId + ')' : '';
    els.direct.innerHTML = table(
      direct.map((i) => [nameCell(i.id, ''), (i.qty * runs).toLocaleString()]),
      ['Input', 'Qty']);

    // Surplus: only when the batch rounding over-produced something.
    const spare = [...totals.surplus.keys()].sort(byName);
    els.surplusSection.hidden = spare.length === 0;
    els.surplus.innerHTML = table(
      spare.map((id) => [nameCell(id, ''), totals.surplus.get(id).toLocaleString()]),
      ['Item', 'Qty']);

    renderChart(els.chart, data, producers, graph, ranks, columns, totals, onChoice);
  }

  function onChoice(itemId, recipeId) {
    state.choices = Object.assign({}, state.choices, {[itemId]: recipeId});
    render();
  }
}

// escapeHTML makes a value safe to interpolate into innerHTML. Item names come
// from game data, not user input, but the tables build markup as strings.
function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]));
}

// renderChart is defined in the SVG section (Task 8). Until then it draws
// nothing so the page works with tables alone.
function renderChart() {}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', initExplorer);
}
```

Extend the export line to add the new pure helpers:

```js
  module.exports = {producersOf, isTerminalItem, activeRecipeId, yieldOf, buildGraph, rankNodes,
    topoOrder, rollUp, orderColumns, layout, selectableOutputs, clampQty, hasOwn,
    encodeState, decodeState, itemHref, leafKind, escapeHTML, QTY_MIN, QTY_MAX};
```

- [ ] **Step 3: Write tests for the new pure helpers**

Append to `tests/js/bom-explorer.test.js`:

```js
test('itemHref points at the catalog page and is empty without a category', () => {
  const data = fixture();
  assert.strictEqual(bx.itemHref(data, 'steel_plate'), '../items/refined/steel_plate.html');
  data.items.mystery = {n: 'Mystery', c: ''};
  assert.strictEqual(bx.itemHref(data, 'mystery'), '');
  assert.strictEqual(bx.itemHref(data, 'nonesuch'), '');
});

test('leafKind separates mined ores from no-recipe drops', () => {
  const data = fixture();
  assert.strictEqual(bx.leafKind(data, 'iron_ore'), 'ore');
  assert.strictEqual(bx.leafKind(data, 'drop_core'), 'drop');
});

test('escapeHTML neutralises markup', () => {
  assert.strictEqual(bx.escapeHTML('<a href="x">&</a>'),
    '&lt;a href=&quot;x&quot;&gt;&amp;&lt;/a&gt;');
});
```

- [ ] **Step 4: Run the tests**

Run: `node --test tests/js/`
Expected: PASS, all thirty-three tests. The DOM layer is inert under Node because `document` is undefined.

- [ ] **Step 5: Verify the page in a browser**

Run: `python3 -m http.server 8765 --directory . > /dev/null 2>&1 &` then open `http://localhost:8765/kb/build-costs/explorer.html`.

Check: typing "power" suggests Power Core and friends; picking one shows the three tables; changing the quantity rescales them; the URL updates to `?target=power_core`; reloading that URL restores the selection; `?target=iron_ore` shows the "no recipe produces this" message. Stop the server when done (`kill %1`).

- [ ] **Step 6: Commit**

```bash
git add kb/build-costs/explorer.html kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): page shell with autocomplete, quantity and input tables"
```

---

### Task 8: SVG graph rendering with in-node recipe selectors

**Files:**
- Modify: `kb/build-costs/bom-explorer.js` (replace the `renderChart` stub with the real implementation)

**Interfaces:**
- Consumes: `layout` (Task 5), the `graph`/`ranks`/`columns`/`totals` values assembled in `render` (Task 7), and the `onChoice(itemId, recipeId)` callback.
- Produces: `renderChart(container, data, producers, graph, ranks, columns, totals, onChoice)` — replaces the container's contents with one `<svg>` element.

**Background — why `foreignObject`.** Node boxes need a real `<select>` for the recipe choice, and SVG has no native form controls. Wrapping the box contents in `<foreignObject>` gives normal HTML inside the SVG coordinate system, which every current browser supports. Boxes without a selector use the same structure for consistency.

**Visual requirements:**
- Leaf boxes take a distinct border colour (`--muted2`); the target box is emphasised with `--accent`; intermediates use `--border`.
- Each box shows the display name (linked via `targetHref` when there is a page) and the quantity from `totals.demand`.
- A box whose item has more than one recipe gets a `<select>` listing those recipe ids, with the active one selected. Changing it calls `onChoice`.
- A node flagged `cycle` shows the text "cycle — not expanded" in place of its quantity.
- Edges draw as `<polyline>` with an arrowhead marker at the consumer end and the input quantity as a `<text>` label near the arrowhead.
- The `<svg>` gets explicit `width`/`height` from `layout` so the container's `overflow-x:auto` scrolls it rather than squashing it.

- [ ] **Step 1: Write the implementation**

Replace the `renderChart` stub in `kb/build-costs/bom-explorer.js` with:

```js
const SVG_NS = 'http://www.w3.org/2000/svg';
const XHTML_NS = 'http://www.w3.org/1999/xhtml';

function svgEl(name, attrs) {
  const el = document.createElementNS(SVG_NS, name);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, String(v));
  return el;
}

// renderChart draws the layered graph. Columns run left to right by rank, so
// every arrow points rightwards and a base ore feeding the output directly
// spans the full width.
function renderChart(container, data, producers, graph, ranks, columns, totals, onChoice) {
  container.innerHTML = '';
  const {width, height, boxes, edges} = layout(graph, ranks, columns, producers);

  const svg = svgEl('svg', {
    width, height, viewBox: '0 0 ' + width + ' ' + height,
    xmlns: SVG_NS, role: 'img', 'aria-label': 'Production chain',
  });

  const defs = svgEl('defs');
  const marker = svgEl('marker', {
    id: 'bx-arrow', viewBox: '0 0 10 10', refX: 9, refY: 5,
    markerWidth: 6, markerHeight: 6, orient: 'auto-start-reverse',
  });
  marker.append(svgEl('path', {d: 'M 0 0 L 10 5 L 0 10 z', fill: 'var(--muted)'}));
  defs.append(marker);
  svg.append(defs);

  // Edges first so boxes paint over them.
  for (const edge of edges) {
    svg.append(svgEl('polyline', {
      points: edge.points.map((p) => p.join(',')).join(' '),
      fill: 'none', stroke: 'var(--muted)', 'stroke-width': 1.5,
      'marker-end': 'url(#bx-arrow)',
    }));
    const [ex, ey] = edge.points[edge.points.length - 1];
    const label = svgEl('text', {
      x: ex - 8, y: ey - 5, 'text-anchor': 'end',
      fill: 'var(--dim)', 'font-size': 11, 'font-family': 'system-ui,sans-serif',
    });
    label.textContent = edge.qty.toLocaleString();
    svg.append(label);
  }

  for (const [id, box] of boxes) {
    const node = graph.nodes.get(id);
    const stroke = id === graph.targetId ? 'var(--accent)'
      : node.leaf ? 'var(--muted2)' : 'var(--border)';
    svg.append(svgEl('rect', {
      x: box.x, y: box.y, width: box.w, height: box.h, rx: 5,
      fill: 'var(--panel2)', stroke, 'stroke-width': id === graph.targetId ? 2 : 1,
    }));

    const fo = svgEl('foreignObject', {x: box.x, y: box.y, width: box.w, height: box.h});
    const div = document.createElementNS(XHTML_NS, 'div');
    div.setAttribute('style',
      'height:100%;box-sizing:border-box;padding:4px 6px;font:12px system-ui,sans-serif;' +
      'color:var(--text);display:flex;flex-direction:column;gap:2px;overflow:hidden');

    const nameEl = document.createElementNS(XHTML_NS, 'div');
    const href = targetHref(data, id);
    if (href) {
      const a = document.createElementNS(XHTML_NS, 'a');
      a.setAttribute('href', href);
      a.setAttribute('style', 'color:var(--link);text-decoration:none');
      a.textContent = displayName(data, id);
      nameEl.append(a);
    } else {
      nameEl.textContent = displayName(data, id);
    }
    nameEl.setAttribute('style', 'font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis');
    div.append(nameEl);

    const qtyEl = document.createElementNS(XHTML_NS, 'div');
    qtyEl.setAttribute('style', 'color:var(--muted);font-size:11px');
    qtyEl.textContent = node.cycle
      ? 'cycle — not expanded'
      : '×' + (totals.demand.get(id) || 0).toLocaleString();
    div.append(qtyEl);

    const recipeIds = producers.get(id);
    if (recipeIds && recipeIds.length > 1) {
      const select = document.createElementNS(XHTML_NS, 'select');
      select.setAttribute('style',
        'width:100%;font:11px system-ui,sans-serif;background:var(--panel);color:var(--text);' +
        'border:1px solid var(--border);border-radius:3px;padding:1px 2px');
      for (const rid of recipeIds) {
        const option = document.createElementNS(XHTML_NS, 'option');
        option.setAttribute('value', rid);
        if (rid === node.recipeId) option.setAttribute('selected', 'selected');
        option.textContent = rid;
        select.append(option);
      }
      select.addEventListener('change', (e) => onChoice(id, e.target.value));
      div.append(select);
    }

    fo.append(div);
    svg.append(fo);
  }

  container.append(svg);
}
```

- [ ] **Step 2: Confirm the pure-function tests still pass**

Run: `node --test tests/js/`
Expected: PASS, all thirty-three tests unchanged. `renderChart` touches the DOM and is not exported, so nothing in the suite calls it.

- [ ] **Step 3: Visual verification — the three required cases**

Serve the site (`python3 -m http.server 8765 --directory . > /dev/null 2>&1 &`) and check each:

1. `explorer.html?target=steel_plate` — the degenerate refining case. Expect exactly two boxes and one labelled arrow, with the recipe id above the chart.
2. `explorer.html?target=power_core` — a median case. Expect roughly 4 tiers and 12 boxes, arrows all pointing right, quantity labels legible.
3. `explorer.html?target=overmind` — the worst case (10 tiers, 135 boxes). Expect the chart to scroll horizontally inside its container while the page itself does not scroll sideways.

Then exercise the interaction: on `?target=power_core`, change a `▾` selector inside a node box and confirm the graph and all three tables update, and that the URL gains an `r=` parameter. Reload that URL and confirm the choice is restored. Stop the server (`kill %1`).

- [ ] **Step 4: Commit**

```bash
git add kb/build-costs/bom-explorer.js
git commit -m "feat(bom-explorer): SVG chain rendering with in-node recipe selectors"
```

---

### Task 9: Link the explorer from every per-target build-costs page

**Files:**
- Modify: `cmd/generate-build-costs/templates/detail.html.tmpl:28`
- Modify: `cmd/generate-build-costs/render.go:301-305`
- Modify: `cmd/generate-build-costs/render_test.go`

**Interfaces:**
- Consumes: `explorer.html?target=<id>` from Task 6's `decodeState`.
- Produces: nothing other tasks depend on.

**Background:** `renderDetail` executes `templates/detail.html.tmpl` with a `map[string]any`. The map currently carries `Name`, `Kind`, `Lines`, `SelfHref`, `BoM`, `Recipes`, `RecipeNA`, `ShowBanner`, `Cover`, `LastUpdated`. The target's id is available in the function as `row.ID` but is not passed to the template yet. Detail pages sit in `kb/build-costs/`, the same directory as `explorer.html`, so the link is relative with no path prefix.

- [ ] **Step 1: Write the failing test**

Append to `cmd/generate-build-costs/render_test.go`. Model the setup on the existing `renderDetail` test in that file — reuse its fixture construction rather than inventing a new one, and only assert the new link:

```go
func TestRenderDetailLinksToExplorer(t *testing.T) {
	dir := t.TempDir()
	row := MatrixRow{ID: "power_core", Name: "Power Core", Kind: "item", Cells: map[string]Cell{}}
	tgt := buildcost.Target{
		ID:   "power_core",
		BoM:  []buildcost.Requirement{{ItemID: "iron_bar", Qty: 2}},
		Recipes: []buildcost.Recipe{{ID: "assemble_power_core", OutputQty: 1,
			Inputs: []buildcost.Requirement{{ItemID: "iron_bar", Qty: 2}}}},
	}
	names := map[string]string{"iron_bar": "Iron Bar", "power_core": "Power Core"}
	categories := map[string]string{"iron_bar": "refined", "power_core": "component"}

	if err := renderDetail(dir, row, nil, tgt, names, categories, [4]MatrixRow{}, galaxyCover{}); err != nil {
		t.Fatalf("renderDetail: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "power_core.html"))
	if err != nil {
		t.Fatal(err)
	}
	want := `href="explorer.html?target=power_core"`
	if !strings.Contains(string(raw), want) {
		t.Errorf("detail page missing explorer link %s", want)
	}
	if !strings.Contains(string(raw), "Explore this BoM interactively") {
		t.Error("detail page missing the explorer link text")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run TestRenderDetailLinksToExplorer -v`
Expected: FAIL — `detail page missing explorer link href="explorer.html?target=power_core"`.

- [ ] **Step 3: Pass the id to the template**

In `cmd/generate-build-costs/render.go`, change the `t.Execute` map at the end of `renderDetail` to include the id:

```go
	return t.Execute(f, map[string]any{
		"ID": row.ID, "Name": row.Name, "Kind": row.Kind, "Lines": lines,
		"SelfHref": selfHref, "BoM": bom, "Recipes": recipes, "RecipeNA": tgt.RecipeNA,
		"ShowBanner": !localAnyFeasible, "Cover": cover, "LastUpdated": lastMarketUpdate,
	})
```

- [ ] **Step 4: Add the link to the template**

In `cmd/generate-build-costs/templates/detail.html.tmpl`, immediately after line 28 (the existing `SelfHref` paragraph), add:

```html
<p><a href="explorer.html?target={{.ID}}">Explore this BoM interactively →</a></p>
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -v`
Expected: PASS, the new test and every existing one.

- [ ] **Step 6: Lint and commit**

Run: `go build ./... && golangci-lint run ./cmd/generate-build-costs/...`

```bash
git add cmd/generate-build-costs
git commit -m "feat(build-costs): link each target page to the interactive BoM explorer"
```

---

### Task 10: Cross-check against the committed BoM table, and document regeneration

Close the loop between the two views and record how to regenerate.

**Files:**
- Create: `cmd/generate-bom-explorer/crosscheck_test.go`
- Modify: `cmd/generate-bom-explorer/main.go` (add the divergence count to the log line)
- Modify: `docs/USAGE.md`

**Interfaces:**
- Consumes: `BuildDoc` (Task 2), the `bill_of_materials` table in `crafting.db`.
- Produces: nothing other tasks depend on.

**Background — what can and cannot be asserted.** The explorer rounds up to whole batches at every tier; the committed `bill_of_materials` table computes a per-unit cost with a ceiling at each tier and multiplies. For a chain whose recipes **all yield exactly 1**, the two are provably identical, because `ceil(n/1) == n` at every step — that is the case worth asserting. Where a multi-yield recipe appears, the two legitimately differ in **either** direction (per-unit-then-multiply can under- or over-count depending on the ratio), so there is nothing to assert; the generator logs how many targets are affected instead.

This test reads the real `crafting.db`. Skip it when the file is absent so the suite still passes in a bare checkout.

- [ ] **Step 1: Write the test**

Create `cmd/generate-bom-explorer/crosscheck_test.go`:

```go
package main

import (
	"database/sql"
	"os"
	"sort"
	"testing"

	_ "modernc.org/sqlite"
)

const craftingDBPath = "../../crafting.db"

// TestSingleYieldTargetsMatchCommittedBoM checks the explorer's arithmetic
// against the committed bill_of_materials table for every target whose chain
// uses only recipes that yield exactly 1. For those, whole-batch rounding and
// per-unit-then-multiply are provably identical, so any difference is a real
// defect rather than the expected batching divergence.
func TestSingleYieldTargetsMatchCommittedBoM(t *testing.T) {
	if _, err := os.Stat(craftingDBPath); err != nil {
		t.Skip("crafting.db not present; skipping cross-check")
	}
	db, err := sql.Open("sqlite", "file:"+craftingDBPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	items, err := loadItems(db)
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := loadRecipes(db)
	if err != nil {
		t.Fatal(err)
	}
	doc := BuildDoc(items, recipes, nil, nil)

	committed, err := loadCommittedBoM(db)
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for target := range committed {
		flat, allSingleYield, ok := flattenSingleYield(doc, target)
		if !ok || !allSingleYield {
			continue
		}
		checked++
		want := committed[target]
		if len(flat) != len(want) {
			t.Errorf("%s: %d base materials, committed table has %d", target, len(flat), len(want))
			continue
		}
		keys := make([]string, 0, len(flat))
		for k := range flat {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if flat[k] != want[k] {
				t.Errorf("%s: %s = %d, committed table has %d", target, k, flat[k], want[k])
			}
		}
	}

	if checked == 0 {
		t.Fatal("no single-yield targets found to cross-check; the filter is wrong")
	}
	t.Logf("cross-checked %d single-yield targets against bill_of_materials", checked)
}

// loadCommittedBoM reads the item rows of the bill_of_materials table.
func loadCommittedBoM(db *sql.DB) (map[string]map[string]int, error) {
	rows, err := db.Query(`SELECT target_id, base_item_id, quantity
	                       FROM bill_of_materials WHERE target_type='item'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]int{}
	for rows.Next() {
		var target, base string
		var qty int
		if err := rows.Scan(&target, &base, &qty); err != nil {
			return nil, err
		}
		if out[target] == nil {
			out[target] = map[string]int{}
		}
		out[target][base] = qty
	}
	return out, rows.Err()
}

// flattenSingleYield expands one unit of target through the default recipe
// choices, accumulating leaf quantities. The second return reports whether
// every recipe on the chain yields exactly 1; the third is false when the
// target has no recipe or the chain hits a cycle.
func flattenSingleYield(doc Doc, target string) (map[string]int, bool, bool) {
	producers := map[string][]string{}
	for id, r := range doc.Recipes {
		for _, o := range r.Outputs {
			item, _ := o[0].(string)
			producers[item] = append(producers[item], id)
		}
	}
	for _, ids := range producers {
		sort.Strings(ids)
	}

	flat := map[string]int{}
	allSingle := true
	ok := true

	var walk func(id string, mult int, depth int, stack map[string]bool)
	walk = func(id string, mult int, depth int, stack map[string]bool) {
		if depth > 32 || stack[id] {
			ok = false
			return
		}
		item, known := doc.Items[id]
		if known && (item.Category == "ore" || item.Category == "material") {
			flat[id] += mult
			return
		}
		ids := producers[id]
		if len(ids) == 0 {
			flat[id] += mult
			return
		}
		chosen := doc.Defaults[id]
		if chosen == "" {
			chosen = ids[0]
		}
		recipe := doc.Recipes[chosen]
		yield := 0
		for _, o := range recipe.Outputs {
			if oid, _ := o[0].(string); oid == id {
				yield, _ = o[1].(int)
			}
		}
		if yield != 1 {
			allSingle = false
		}
		if yield == 0 {
			ok = false
			return
		}
		next := map[string]bool{id: true}
		for k := range stack {
			next[k] = true
		}
		for _, in := range recipe.Inputs {
			iid, _ := in[0].(string)
			qty, _ := in[1].(int)
			walk(iid, mult*qty, depth+1, next)
		}
	}

	if len(producers[target]) == 0 {
		return nil, false, false
	}
	walk(target, 1, 0, map[string]bool{})
	return flat, allSingle, ok
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./cmd/generate-bom-explorer/ -run TestSingleYieldTargetsMatchCommittedBoM -v`
Expected: PASS, with a log line reporting a non-zero count of cross-checked targets.

If it FAILS, that is a genuine finding, not a test to relax: the explorer and the committed table disagree on a chain where they are provably supposed to agree. Investigate the divergence before continuing.

- [ ] **Step 3: Log the divergence count from the generator**

In `cmd/generate-bom-explorer/main.go`, add a count of targets whose chain contains a multi-yield recipe, and extend the log line. Add this helper to `build.go`:

```go
// MultiYieldItems returns the items whose default recipe yields more than one
// unit per batch. Any chain containing one of these produces different totals
// under whole-batch rounding than the per-unit arithmetic the static
// bill_of_materials table uses — expected, and worth reporting on each run so
// the divergence is never silent.
func MultiYieldItems(doc Doc) []string {
	var out []string
	for item, recipeID := range doc.Defaults {
		for _, o := range doc.Recipes[recipeID].Outputs {
			id, _ := o[0].(string)
			qty, _ := o[1].(int)
			if id == item && qty > 1 {
				out = append(out, item)
			}
		}
	}
	sort.Strings(out)
	return out
}
```

And in `main.go`, before the existing log call:

```go
	multi := MultiYieldItems(doc)
	log.Printf("bom-explorer: %d items have a multi-yield default recipe; chains through them "+
		"total differently than the static bill_of_materials table (whole batches vs per-unit)", len(multi))
```

- [ ] **Step 4: Add a test for the helper**

Append to `cmd/generate-bom-explorer/build_test.go`:

```go
func TestMultiYieldItems(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)
	// smelt_steel and cast_steel both yield 2 steel_plate.
	got := MultiYieldItems(doc)
	if len(got) != 1 || got[0] != "steel_plate" {
		t.Errorf("MultiYieldItems = %v, want [steel_plate]", got)
	}
}
```

- [ ] **Step 5: Run everything**

Run: `go build ./... && go test ./... && node --test tests/js/ && golangci-lint run`
Expected: all PASS, no new lint findings.

- [ ] **Step 6: Regenerate and confirm the committed data is current**

Run:

```bash
go run ./cmd/generate-bom-explorer
git diff --stat kb/build-costs/recipe-graph.json
```

Expected: either no diff (the file committed in Task 2 is still current) or a diff you can explain from a crafting-DB change.

- [ ] **Step 7: Document regeneration**

Add to `docs/USAGE.md`, in whichever section lists the KB generators (match the surrounding format; if the file has no such section, append one titled `## BoM Explorer`):

```markdown
### BoM Explorer data

The interactive Bill of Materials explorer (`kb/build-costs/explorer.html`)
reads `kb/build-costs/recipe-graph.json`. Regenerate it after any crafting-DB
refresh:

    go run ./cmd/generate-bom-explorer

It needs only `crafting.db` and the newest `data/snapshots/<date>/` catalogs —
no market DB. `explorer.html` and `bom-explorer.js` are hand-maintained and are
not written by any generator.
```

Check `.gitignore` does not exclude the file before committing — `docs/*.md` has broad ignore patterns in this repo, so add a negation rule if `git check-ignore -v docs/USAGE.md` reports a match.

- [ ] **Step 8: Commit**

```bash
git add cmd/generate-bom-explorer docs/USAGE.md kb/build-costs/recipe-graph.json
git commit -m "test(bom-explorer): cross-check single-yield chains against the committed BoM"
```

---

## Done

At completion the KB has a working interactive BoM explorer at
`kb/build-costs/explorer.html`, reachable from all 1033 per-target build-cost
pages, with its data regenerated by `go run ./cmd/generate-bom-explorer`.
