# Facility Build-Cost Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a KB section that estimates, for every facility, the market cost of constructing it — a flattened BoM view and a direct-components Recipe view, each priced by market-average and galaxy-wide cheapest depth-walk — grouped like the items/recipes sections.

**Architecture:** Extend the existing `cmd/generate-build-costs` binary (which already loads market order books and sell VWAP). Add pure loaders for the facility catalog and facility BoM, a galaxy-pooled order book, a grouping rule (production → produced-item market category, non-production → facility category), a per-view cost computation, a page-assembly step producing display view-models, and two HTML templates. Wire it into `main.go` behind a flag, reusing the already-loaded DBs and books.

**Tech Stack:** Go 1.24+, `modernc.org/sqlite` (pure-Go SQLite, read-only), `html/template` (embedded via existing `//go:embed templates/*.tmpl`). Standard-library tests with in-memory SQLite; no new dependencies.

## Global Constraints

- Go 1.24+; use `range` over integers and `b.Loop()` in any benchmarks (none required here).
- All new code must pass `golangci-lint` with no new findings.
- After each series of changes run `go build ./...` and `go test ./...`.
- Package is `main` under `cmd/generate-build-costs/`; new code joins that package.
- Reuse existing helpers verbatim — do not duplicate: `commaInt`, `qtyStr`, `emDash`, `findLatestCatalogDir`, `pooledBook`, `loadBooks`, `loadSellVWAP`, `loadItemMeta`, `buildcost.Book.Walk`, `buildcost.Book.PriceRequirements`.
- Read-only DB access only (`?mode=ro` is already applied by `openRO`); never write to game DBs.
- Facility builds are a single unit — show a build-cost total, never a per-unit/units-per-run line.

**Data facts (verified against the live DBs):**
- `crafting.db` `bill_of_materials(target_id, target_type, base_item_id, quantity)` — 2,440 rows with `target_type='facility'` (flattened to ores).
- `crafting.db` `items(id, name, category)`; `recipe_outputs(recipe_id, item_id, quantity)`.
- `catalog_facilities.json` shape: `{"items":[{"id","name","category","level","recipe_id","build_materials":[{"item_id","quantity"}], ...}]}`. Categories seen: `production` (2,365), `service` (68), `infrastructure` (65), `faction` (43), `personal` (13).
- `market.db` `market_ohlcv(item_id, side, vwap, volume)` and `market_orders(...)` — loaded by the existing `loadSellVWAP` / `loadBooks`.
- Produced-item category resolution: `facility.recipe_id` → `recipe_outputs.item_id` (first, ordered) → `items.category`. ~186 facilities have a `recipe_id` with no resolvable output and ~24 resolve to an uncategorized item → these go to group `other`.

## File Structure

- Create: `cmd/generate-build-costs/facilities.go` — loaders, galaxy book, grouping, cost model, page assembly.
- Create: `cmd/generate-build-costs/facilities_test.go` — tests for the above.
- Create: `cmd/generate-build-costs/facilities_render.go` — index + group render functions.
- Create: `cmd/generate-build-costs/facilities_render_test.go` — render tests.
- Create: `cmd/generate-build-costs/templates/facilities-index.html.tmpl` — landing page (auto-included by the existing `//go:embed templates/*.tmpl` in `render.go`).
- Create: `cmd/generate-build-costs/templates/facilities-group.html.tmpl` — per-group page.
- Modify: `cmd/generate-build-costs/main.go` — add `-facilities-out` flag and wiring after the existing steps.

---

### Task 1: Pure helpers — `galaxyBook` and `fmtMoney`

**Files:**
- Create: `cmd/generate-build-costs/facilities.go`
- Test: `cmd/generate-build-costs/facilities_test.go`

**Interfaces:**
- Consumes: `pooledBook(books map[string]*buildcost.Book, members []string) *buildcost.Book` (hops.go); `commaInt(float64) string` (render.go).
- Produces:
  - `galaxyBook(books map[string]*buildcost.Book) *buildcost.Book` — pools every station's sell ladder into one ascending book.
  - `fmtMoney(v float64) string` — thousands-separated 2-decimal money string.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func TestGalaxyBook_PoolsAndSorts(t *testing.T) {
	books := map[string]*buildcost.Book{
		"a": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 12, Qty: 5}}}, BestBuy: map[string]float64{}},
		"b": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 8, Qty: 3}}}, BestBuy: map[string]float64{}},
	}
	gb := galaxyBook(books)
	l := gb.Sell["iron"]
	if len(l) != 2 || l[0].Price != 8 || l[1].Price != 12 {
		t.Fatalf("pooled iron ladder = %+v, want 8 then 12", l)
	}
	// Depth walk across the pool: need 6 → 3@8 + 3@12 = 60.
	w := gb.Walk("iron", 6)
	if w.Cost != 60 || w.Shortfall != 0 {
		t.Fatalf("walk = %+v, want cost 60 shortfall 0", w)
	}
}

func TestFmtMoney(t *testing.T) {
	cases := map[float64]string{
		25.38:    "25.38",
		3579.666: "3,579.67",
		28762.9:  "28,762.90",
		0:        "0.00",
		-4.5:     "-4.50",
	}
	for in, want := range cases {
		if got := fmtMoney(in); got != want {
			t.Fatalf("fmtMoney(%v) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run 'TestGalaxyBook_PoolsAndSorts|TestFmtMoney' -v`
Expected: FAIL — `undefined: galaxyBook`, `undefined: fmtMoney`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package-level file for the facility build-cost section.
package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// galaxyBook pools every station's sell ladder into a single ascending order
// book — the "cheapest sourcing anywhere" reference the galaxy price walks. It
// reuses pooledBook with the full station set. BestBuy is irrelevant here.
func galaxyBook(books map[string]*buildcost.Book) *buildcost.Book {
	ids := make([]string, 0, len(books))
	for id := range books {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return pooledBook(books, ids)
}

// fmtMoney formats a value with thousands separators and two decimals
// (e.g. 28762.9 → "28,762.90"). Reuses commaInt for the integer part.
func fmtMoney(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := math.Floor(v)
	frac := int64(math.Round((v - whole) * 100))
	if frac >= 100 {
		whole++
		frac -= 100
	}
	s := fmt.Sprintf("%s.%02d", commaInt(whole), frac)
	if neg {
		return "-" + s
	}
	return s
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run 'TestGalaxyBook_PoolsAndSorts|TestFmtMoney' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/facilities.go cmd/generate-build-costs/facilities_test.go
git commit -m "feat(build-costs): galaxyBook pooling + fmtMoney helper for facility pages"
```

---

### Task 2: Load the facility catalog

**Files:**
- Modify: `cmd/generate-build-costs/facilities.go`
- Test: `cmd/generate-build-costs/facilities_test.go`

**Interfaces:**
- Consumes: `findLatestCatalogDir(root string) (string, error)` (main.go); `buildcost.Requirement{ItemID string, Qty float64}`.
- Produces:
  - `type FacilityRec struct { ID, Name, Category string; Level int; RecipeID string; Build []buildcost.Requirement }`
  - `loadFacilityCatalog(root string) ([]FacilityRec, error)` — reads `catalog_facilities.json` from the newest snapshot dir under root; `Build` is the direct `build_materials`.

- [ ] **Step 1: Write the failing test**

```go
func TestLoadFacilityCatalog(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "20260706")
	if err := os.MkdirAll(snap, 0o755); err != nil {
		t.Fatal(err)
	}
	blob := `{"items":[
	  {"id":"forge","name":"Ghost Forge","category":"production","level":2,"recipe_id":"forge_ghost_rounds",
	   "build_materials":[{"item_id":"hot_cell","quantity":2},{"item_id":"titanium_ingot","quantity":3}]},
	  {"id":"depot","name":"Depot","category":"service","level":1,"build_materials":[]}
	]}`
	if err := os.WriteFile(filepath.Join(snap, "catalog_facilities.json"), []byte(blob), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, err := loadFacilityCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]FacilityRec{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	f := byID["forge"]
	if f.Name != "Ghost Forge" || f.Category != "production" || f.Level != 2 || f.RecipeID != "forge_ghost_rounds" {
		t.Fatalf("forge = %+v", f)
	}
	if len(f.Build) != 2 || f.Build[0].ItemID != "hot_cell" || f.Build[0].Qty != 2 {
		t.Fatalf("forge build = %+v", f.Build)
	}
	if byID["depot"].Category != "service" || len(byID["depot"].Build) != 0 {
		t.Fatalf("depot = %+v", byID["depot"])
	}
}
```

Add these imports to the top of `facilities_test.go`: `"os"`, `"path/filepath"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run TestLoadFacilityCatalog -v`
Expected: FAIL — `undefined: loadFacilityCatalog`, `undefined: FacilityRec`.

- [ ] **Step 3: Write minimal implementation**

Append to `facilities.go` (add `"encoding/json"`, `"os"`, `"path/filepath"` to its imports):

```go
// FacilityRec is the minimal facility shape the build-cost pages need: identity,
// category, level, its production recipe id (used only for grouping), and the
// direct build_materials that construct it (the Recipe view).
type FacilityRec struct {
	ID       string
	Name     string
	Category string
	Level    int
	RecipeID string
	Build    []buildcost.Requirement
}

// facilityCatDoc / facilityCatItem mirror the fields of catalog_facilities.json
// that the build-cost pages consume.
type facilityCatDoc struct {
	Items []facilityCatItem `json:"items"`
}

type facilityCatItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Level          int    `json:"level"`
	RecipeID       string `json:"recipe_id"`
	BuildMaterials []struct {
		ItemID   string  `json:"item_id"`
		Quantity float64 `json:"quantity"`
	} `json:"build_materials"`
}

// loadFacilityCatalog reads catalog_facilities.json from the newest snapshot dir
// under root and returns the trimmed facility records.
func loadFacilityCatalog(root string) ([]FacilityRec, error) {
	dir, err := findLatestCatalogDir(root)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "catalog_facilities.json"))
	if err != nil {
		return nil, err
	}
	var doc facilityCatDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make([]FacilityRec, 0, len(doc.Items))
	for _, it := range doc.Items {
		rec := FacilityRec{ID: it.ID, Name: it.Name, Category: it.Category, Level: it.Level, RecipeID: it.RecipeID}
		for _, m := range it.BuildMaterials {
			rec.Build = append(rec.Build, buildcost.Requirement{ItemID: m.ItemID, Qty: m.Quantity})
		}
		out = append(out, rec)
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run TestLoadFacilityCatalog -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/facilities.go cmd/generate-build-costs/facilities_test.go
git commit -m "feat(build-costs): load facility catalog (build_materials)"
```

---

### Task 3: Load facility BoM and recipe outputs from the crafting DB

**Files:**
- Modify: `cmd/generate-build-costs/facilities.go`
- Test: `cmd/generate-build-costs/facilities_test.go`

**Interfaces:**
- Consumes: `*sql.DB`; `buildcost.Requirement`.
- Produces:
  - `loadFacilityBoM(craftDB *sql.DB) (map[string][]buildcost.Requirement, error)` — facility id → flattened base-material requirements (from `bill_of_materials` where `target_type='facility'`).
  - `loadRecipeOutputItem(craftDB *sql.DB) (map[string]string, error)` — recipe id → its first output item id.

- [ ] **Step 1: Write the failing test**

```go
func newCraftTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE bill_of_materials (target_id TEXT, target_type TEXT, base_item_id TEXT, quantity REAL);
CREATE TABLE recipe_outputs (recipe_id TEXT, item_id TEXT, quantity REAL);
INSERT INTO bill_of_materials VALUES ('forge','facility','titanium_ore',8);
INSERT INTO bill_of_materials VALUES ('forge','facility','copper_ore',2);
INSERT INTO bill_of_materials VALUES ('widget','item','iron_ore',3);
INSERT INTO recipe_outputs VALUES ('forge_ghost_rounds','ghost_rounds',2);
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadFacilityBoM(t *testing.T) {
	db := newCraftTestDB(t)
	bom, err := loadFacilityBoM(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bom["widget"]; ok {
		t.Fatal("widget is an item, must not appear in facility BoM")
	}
	reqs := bom["forge"]
	if len(reqs) != 2 {
		t.Fatalf("forge BoM = %+v", reqs)
	}
	got := map[string]float64{}
	for _, r := range reqs {
		got[r.ItemID] = r.Qty
	}
	if got["titanium_ore"] != 8 || got["copper_ore"] != 2 {
		t.Fatalf("forge BoM quantities = %+v", got)
	}
}

func TestLoadRecipeOutputItem(t *testing.T) {
	db := newCraftTestDB(t)
	out, err := loadRecipeOutputItem(db)
	if err != nil {
		t.Fatal(err)
	}
	if out["forge_ghost_rounds"] != "ghost_rounds" {
		t.Fatalf("recipe output = %q, want ghost_rounds", out["forge_ghost_rounds"])
	}
}
```

Add these imports to `facilities_test.go`: `"database/sql"` and the blank driver import `_ "modernc.org/sqlite"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run 'TestLoadFacilityBoM|TestLoadRecipeOutputItem' -v`
Expected: FAIL — `undefined: loadFacilityBoM`, `undefined: loadRecipeOutputItem`.

- [ ] **Step 3: Write minimal implementation**

Append to `facilities.go` (add `"database/sql"` to its imports):

```go
// loadFacilityBoM returns facility id -> flattened base-material requirements,
// from bill_of_materials rows with target_type='facility'.
func loadFacilityBoM(craftDB *sql.DB) (map[string][]buildcost.Requirement, error) {
	rows, err := craftDB.Query(`SELECT target_id, base_item_id, quantity
	                            FROM bill_of_materials WHERE target_type='facility'
	                            ORDER BY target_id, base_item_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]buildcost.Requirement{}
	for rows.Next() {
		var id, base string
		var qty float64
		if err := rows.Scan(&id, &base, &qty); err != nil {
			return nil, err
		}
		out[id] = append(out[id], buildcost.Requirement{ItemID: base, Qty: qty})
	}
	return out, rows.Err()
}

// loadRecipeOutputItem returns recipe id -> its first output item id (ordered),
// used to resolve what a production facility makes for grouping.
func loadRecipeOutputItem(craftDB *sql.DB) (map[string]string, error) {
	rows, err := craftDB.Query(`SELECT recipe_id, item_id FROM recipe_outputs ORDER BY recipe_id, item_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var rid, iid string
		if err := rows.Scan(&rid, &iid); err != nil {
			return nil, err
		}
		if _, seen := out[rid]; !seen {
			out[rid] = iid
		}
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run 'TestLoadFacilityBoM|TestLoadRecipeOutputItem' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/facilities.go cmd/generate-build-costs/facilities_test.go
git commit -m "feat(build-costs): load facility BoM + recipe output items"
```

---

### Task 4: Grouping rule

**Files:**
- Modify: `cmd/generate-build-costs/facilities.go`
- Test: `cmd/generate-build-costs/facilities_test.go`

**Interfaces:**
- Consumes: `FacilityRec`; `recipeOut map[string]string` (Task 3); `itemCat map[string]string` (from `loadItemMeta` in main.go).
- Produces: `facilityGroup(f FacilityRec, recipeOut, itemCat map[string]string) string`.

- [ ] **Step 1: Write the failing test**

```go
func TestFacilityGroup(t *testing.T) {
	recipeOut := map[string]string{"build_railgun": "railgun", "mystery": "unknownitem"}
	itemCat := map[string]string{"railgun": "weapon"}
	cases := []struct {
		name string
		f    FacilityRec
		want string
	}{
		{"production resolves to produced-item category",
			FacilityRec{Category: "production", RecipeID: "build_railgun"}, "weapon"},
		{"production with no recipe output -> other",
			FacilityRec{Category: "production", RecipeID: "none"}, "other"},
		{"production with uncategorized output -> other",
			FacilityRec{Category: "production", RecipeID: "mystery"}, "other"},
		{"non-production uses facility category",
			FacilityRec{Category: "service", RecipeID: "build_railgun"}, "service"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := facilityGroup(tc.f, recipeOut, itemCat); got != tc.want {
				t.Fatalf("facilityGroup = %q, want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run TestFacilityGroup -v`
Expected: FAIL — `undefined: facilityGroup`.

- [ ] **Step 3: Write minimal implementation**

Append to `facilities.go`:

```go
// facilityGroup returns the navigation group for a facility: non-production
// facilities group by their own category; production facilities group by the
// market category of the item their recipe produces, falling back to "other"
// when the produced item is unknown or uncategorized.
func facilityGroup(f FacilityRec, recipeOut, itemCat map[string]string) string {
	if f.Category != "production" {
		return f.Category
	}
	out := recipeOut[f.RecipeID]
	if out == "" {
		return "other"
	}
	if cat := itemCat[out]; cat != "" {
		return cat
	}
	return "other"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run TestFacilityGroup -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/facilities.go cmd/generate-build-costs/facilities_test.go
git commit -m "feat(build-costs): facility grouping by produced-item category"
```

---

### Task 5: Per-view cost computation

**Files:**
- Modify: `cmd/generate-build-costs/facilities.go`
- Test: `cmd/generate-build-costs/facilities_test.go`

**Interfaces:**
- Consumes: `buildcost.Requirement`, `buildcost.Book.Walk`, `buildcost.Book.PriceRequirements`; `sellVWAP map[string]float64`; `names, cats map[string]string`.
- Produces:
  - `type FacilityComponentCost struct { ItemID, Name, Href string; Qty float64; MktUnit float64; HasMkt bool; GalUnit float64; GalFull bool }`
  - `type FacilityView struct { Components []FacilityComponentCost; MktTotal float64; MktPriced, MktCount int; GalTotal float64; GalFeasible bool; GalCovered int }`
  - `facItemHref(id string, cats map[string]string) string` — relative link to `kb/items/<cat>/<id>.html` from a group page (three levels up), or `""`.
  - `compName(id string, names map[string]string) string`
  - `buildFacilityView(reqs []buildcost.Requirement, sellVWAP map[string]float64, galaxy *buildcost.Book, names, cats map[string]string) FacilityView`

- [ ] **Step 1: Write the failing test**

```go
func TestBuildFacilityView(t *testing.T) {
	reqs := []buildcost.Requirement{
		{ItemID: "titanium_ore", Qty: 8},
		{ItemID: "copper_ore", Qty: 2},
		{ItemID: "exotic", Qty: 5}, // no VWAP, thin galaxy depth
	}
	sellVWAP := map[string]float64{"titanium_ore": 100, "copper_ore": 25}
	galaxy := &buildcost.Book{Sell: map[string]buildcost.Ladder{
		"titanium_ore": {{Price: 90, Qty: 100}},
		"copper_ore":   {{Price: 20, Qty: 100}},
		"exotic":       {{Price: 5, Qty: 2}}, // only 2 of 5 available
	}, BestBuy: map[string]float64{}}
	names := map[string]string{"titanium_ore": "Titanium Ore", "copper_ore": "Copper Ore", "exotic": "Exotic"}
	cats := map[string]string{"titanium_ore": "ore", "copper_ore": "ore"} // exotic uncategorized

	v := buildFacilityView(reqs, sellVWAP, galaxy, names, cats)

	if v.MktCount != 3 || v.MktPriced != 2 {
		t.Fatalf("mkt count/priced = %d/%d, want 3/2", v.MktCount, v.MktPriced)
	}
	// MKT total over priced components only: 8*100 + 2*25 = 850.
	if v.MktTotal != 850 {
		t.Fatalf("MktTotal = %v, want 850", v.MktTotal)
	}
	// Galaxy is infeasible (exotic short 3), covered 2 of 3 components.
	if v.GalFeasible || v.GalCovered != 2 {
		t.Fatalf("galaxy feasible=%v covered=%d, want false/2", v.GalFeasible, v.GalCovered)
	}
	byID := map[string]FacilityComponentCost{}
	for _, c := range v.Components {
		byID[c.ItemID] = c
	}
	if !byID["titanium_ore"].HasMkt || byID["titanium_ore"].MktUnit != 100 {
		t.Fatalf("titanium mkt = %+v", byID["titanium_ore"])
	}
	if byID["titanium_ore"].Href != "../../../items/ore/titanium_ore.html" {
		t.Fatalf("titanium href = %q", byID["titanium_ore"].Href)
	}
	if !byID["titanium_ore"].GalFull || byID["titanium_ore"].GalUnit != 90 {
		t.Fatalf("titanium galaxy = %+v", byID["titanium_ore"])
	}
	if byID["exotic"].HasMkt || byID["exotic"].GalFull || byID["exotic"].Href != "" {
		t.Fatalf("exotic = %+v (want no mkt, not full, no href)", byID["exotic"])
	}
	if byID["exotic"].Name != "Exotic" {
		t.Fatalf("exotic name = %q", byID["exotic"].Name)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run TestBuildFacilityView -v`
Expected: FAIL — `undefined: buildFacilityView`.

- [ ] **Step 3: Write minimal implementation**

Append to `facilities.go`:

```go
// FacilityComponentCost is one component within a cost view, priced two ways.
type FacilityComponentCost struct {
	ItemID  string
	Name    string
	Href    string // items page link, or "" when uncategorized
	Qty     float64
	MktUnit float64 // sell VWAP per unit
	HasMkt  bool
	GalUnit float64 // average per-unit fill price from the galaxy depth walk
	GalFull bool    // galaxy depth fully covers the required qty
}

// FacilityView is one costing view (BoM or Recipe) of constructing a facility.
type FacilityView struct {
	Components  []FacilityComponentCost
	MktTotal    float64 // sum over priced components
	MktPriced   int     // components with a sell VWAP
	MktCount    int     // total components
	GalTotal    float64 // pooled galaxy cost (partial when infeasible)
	GalFeasible bool    // every component fully covered by galaxy depth
	GalCovered  int     // components fully covered
}

// facItemHref returns the relative link to an item's KB catalog page from a
// facility group page (kb/build-costs/facilities/<group>/index.html → three
// levels up), or "" when the item's category is unknown.
func facItemHref(id string, cats map[string]string) string {
	cat := cats[id]
	if cat == "" {
		return ""
	}
	return "../../../items/" + cat + "/" + id + ".html"
}

// compName returns an item's display name, falling back to its id.
func compName(id string, names map[string]string) string {
	if n := names[id]; n != "" {
		return n
	}
	return id
}

// buildFacilityView prices a requirement set two ways: MKT-AVG (sell VWAP per
// unit, summed over components that have a price) and Galaxy (pooled sell-order
// depth walked cheapest-first). Coverage is reported when depth is short.
func buildFacilityView(reqs []buildcost.Requirement, sellVWAP map[string]float64, galaxy *buildcost.Book, names, cats map[string]string) FacilityView {
	v := FacilityView{MktCount: len(reqs)}
	for _, r := range reqs {
		c := FacilityComponentCost{ItemID: r.ItemID, Qty: r.Qty, Name: compName(r.ItemID, names), Href: facItemHref(r.ItemID, cats)}
		if u, ok := sellVWAP[r.ItemID]; ok && u > 0 {
			c.MktUnit, c.HasMkt = u, true
			v.MktTotal += u * r.Qty
			v.MktPriced++
		}
		w := galaxy.Walk(r.ItemID, r.Qty)
		if w.Shortfall <= 0 && w.Covered > 0 {
			c.GalFull = true
			c.GalUnit = w.Cost / w.Covered
		}
		v.Components = append(v.Components, c)
	}
	gr := galaxy.PriceRequirements(reqs)
	v.GalTotal, v.GalFeasible, v.GalCovered = gr.Cost, gr.Feasible, gr.Covered
	return v
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run TestBuildFacilityView -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/facilities.go cmd/generate-build-costs/facilities_test.go
git commit -m "feat(build-costs): per-view facility cost computation (MKT-AVG + galaxy)"
```

---

### Task 6: Page assembly — groups, sort, TOC, display view-models

**Files:**
- Modify: `cmd/generate-build-costs/facilities.go`
- Test: `cmd/generate-build-costs/facilities_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–5; `qtyStr` and `emDash` (render.go).
- Produces:
  - `type ComponentVM struct { Name, Href, Qty, MktUnit, MktTotal, GalUnit, GalTotal string; GalInfeasible bool }`
  - `type ViewVM struct { Title string; Components []ComponentVM; MktBuildCost, MktNote, GalBuildCost string; GalInfeasible, Empty bool }`
  - `type FacilityEntryVM struct { ID, Name, Href, Produces string; Level int; BoM, Recipe ViewVM }`
  - `type FacilityTOCEntry struct { Group, Href string; Count int; Active bool }`
  - `type FacilityGroupPage struct { Group, Heading string; Facilities []FacilityEntryVM; TOC []FacilityTOCEntry }`
  - `type FacilityGroupSummary struct { Group, Href string; Count int }`
  - `facilityViewVM(title string, v FacilityView) ViewVM`
  - `facDetailHref(f FacilityRec) string` — relative link to the facility's `kb/facilities/<category>/<id>.html` detail page.
  - `buildFacilityPages(recs []FacilityRec, facBoM map[string][]buildcost.Requirement, recipeOut, names, cats map[string]string, sellVWAP map[string]float64, galaxy *buildcost.Book) ([]FacilityGroupPage, []FacilityGroupSummary)`

- [ ] **Step 1: Write the failing test**

```go
func TestFacilityViewVM_CoverageAndPricedNote(t *testing.T) {
	// 3 components, 2 priced, galaxy covers 2 of 3 → infeasible with coverage note.
	v := FacilityView{
		MktCount: 3, MktPriced: 2, MktTotal: 850,
		GalFeasible: false, GalCovered: 2,
		Components: []FacilityComponentCost{
			{Name: "Titanium Ore", Href: "../../../items/ore/titanium_ore.html", Qty: 8, MktUnit: 100, HasMkt: true, GalUnit: 90, GalFull: true},
			{Name: "Exotic", Qty: 5}, // unpriced, not full
		},
	}
	vm := facilityViewVM("BoM (ore)", v)
	if vm.Title != "BoM (ore)" || vm.Empty {
		t.Fatalf("vm header = %+v", vm)
	}
	if vm.MktBuildCost != "850.00" || vm.MktNote != "(2/3 priced)" {
		t.Fatalf("mkt = %q note %q", vm.MktBuildCost, vm.MktNote)
	}
	if !vm.GalInfeasible || vm.GalBuildCost != "2/3 covered" {
		t.Fatalf("gal = %q infeasible=%v", vm.GalBuildCost, vm.GalInfeasible)
	}
	c0 := vm.Components[0]
	if c0.MktUnit != "100.00" || c0.MktTotal != "800.00" || c0.GalUnit != "90.00" || c0.GalTotal != "720.00" || c0.GalInfeasible {
		t.Fatalf("component0 = %+v", c0)
	}
	c1 := vm.Components[1]
	if c1.MktUnit != "—" || c1.GalUnit != "—" || !c1.GalInfeasible {
		t.Fatalf("component1 = %+v", c1)
	}
}

func TestBuildFacilityPages_GroupsSortTOC(t *testing.T) {
	recs := []FacilityRec{
		{ID: "b_forge", Name: "B Forge", Category: "production", RecipeID: "r_wpn", Level: 2,
			Build: []buildcost.Requirement{{ItemID: "iron", Qty: 2}}},
		{ID: "a_forge", Name: "A Forge", Category: "production", RecipeID: "r_wpn", Level: 1,
			Build: []buildcost.Requirement{{ItemID: "iron", Qty: 1}}},
		{ID: "depot", Name: "Depot", Category: "service",
			Build: []buildcost.Requirement{{ItemID: "iron", Qty: 3}}},
	}
	facBoM := map[string][]buildcost.Requirement{
		"a_forge": {{ItemID: "iron_ore", Qty: 2}},
		"b_forge": {{ItemID: "iron_ore", Qty: 4}},
		"depot":   {{ItemID: "iron_ore", Qty: 6}},
	}
	recipeOut := map[string]string{"r_wpn": "railgun"}
	names := map[string]string{"railgun": "Railgun", "iron": "Iron", "iron_ore": "Iron Ore"}
	cats := map[string]string{"railgun": "weapon", "iron": "component", "iron_ore": "ore"}
	sellVWAP := map[string]float64{"iron": 10, "iron_ore": 5}
	galaxy := &buildcost.Book{Sell: map[string]buildcost.Ladder{
		"iron":     {{Price: 9, Qty: 100}},
		"iron_ore": {{Price: 4, Qty: 100}},
	}, BestBuy: map[string]float64{}}

	pages, summaries := buildFacilityPages(recs, facBoM, recipeOut, names, cats, sellVWAP, galaxy)

	// Two groups: weapon (2), service (1); summaries sorted by group name.
	if len(summaries) != 2 || summaries[0].Group != "service" || summaries[1].Group != "weapon" {
		t.Fatalf("summaries = %+v", summaries)
	}
	if summaries[1].Count != 2 || summaries[1].Href != "weapon/" {
		t.Fatalf("weapon summary = %+v", summaries[1])
	}
	var weapon FacilityGroupPage
	for _, p := range pages {
		if p.Group == "weapon" {
			weapon = p
		}
	}
	// Facilities alphabetical within the group.
	if len(weapon.Facilities) != 2 || weapon.Facilities[0].ID != "a_forge" {
		t.Fatalf("weapon facilities = %+v", weapon.Facilities)
	}
	f := weapon.Facilities[0]
	if f.Href != "../../../facilities/production/a_forge.html" {
		t.Fatalf("detail href = %q", f.Href)
	}
	if f.Produces != "Railgun" {
		t.Fatalf("produces = %q", f.Produces)
	}
	if f.BoM.Title != "BoM (ore)" || f.Recipe.Title != "Recipe (components)" {
		t.Fatalf("view titles = %q / %q", f.BoM.Title, f.Recipe.Title)
	}
	// TOC on every page lists all groups with the active one flagged and links to siblings.
	var activeService bool
	for _, e := range weapon.TOC {
		if e.Group == "weapon" && !e.Active {
			t.Fatalf("weapon TOC entry should be active")
		}
		if e.Group == "service" {
			activeService = e.Active
			if e.Href != "../service/" {
				t.Fatalf("service TOC href = %q", e.Href)
			}
		}
	}
	if activeService {
		t.Fatalf("service should not be active on the weapon page")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run 'TestFacilityViewVM_CoverageAndPricedNote|TestBuildFacilityPages_GroupsSortTOC' -v`
Expected: FAIL — `undefined: facilityViewVM`, `undefined: buildFacilityPages`.

- [ ] **Step 3: Write minimal implementation**

Append to `facilities.go` (add `"sort"` is already imported; ensure `"fmt"` is imported — it is):

```go
// ComponentVM is a rendered component row.
type ComponentVM struct {
	Name, Href, Qty              string
	MktUnit, MktTotal            string
	GalUnit, GalTotal            string
	GalInfeasible                bool
}

// ViewVM is a rendered cost view (BoM or Recipe).
type ViewVM struct {
	Title         string
	Components     []ComponentVM
	MktBuildCost   string
	MktNote        string // "(k/N priced)" when some components lack a price
	GalBuildCost   string // money when feasible, else "N/M covered"
	GalInfeasible  bool
	Empty          bool
}

// FacilityEntryVM is one facility section on a group page.
type FacilityEntryVM struct {
	ID, Name, Href, Produces string
	Level                     int
	BoM, Recipe               ViewVM
}

// FacilityTOCEntry is one entry in the horizontal cross-group TOC.
type FacilityTOCEntry struct {
	Group, Href string
	Count       int
	Active      bool
}

// FacilityGroupPage is a rendered per-group page.
type FacilityGroupPage struct {
	Group, Heading string
	Facilities     []FacilityEntryVM
	TOC            []FacilityTOCEntry
}

// FacilityGroupSummary is a landing-page card.
type FacilityGroupSummary struct {
	Group, Href string
	Count       int
}

// facilityViewVM converts a numeric view into rendered strings, applying the
// em-dash for unpriced/uncovered cells, a "k/N priced" note when MKT-AVG is
// partial, and an "N/M covered" galaxy total when depth is short.
func facilityViewVM(title string, v FacilityView) ViewVM {
	vm := ViewVM{Title: title, Empty: len(v.Components) == 0}
	for _, c := range v.Components {
		cvm := ComponentVM{Name: c.Name, Href: c.Href, Qty: qtyStr(c.Qty), MktUnit: emDash, MktTotal: emDash, GalUnit: emDash, GalTotal: emDash}
		if c.HasMkt {
			cvm.MktUnit = fmtMoney(c.MktUnit)
			cvm.MktTotal = fmtMoney(c.MktUnit * c.Qty)
		}
		if c.GalFull {
			cvm.GalUnit = fmtMoney(c.GalUnit)
			cvm.GalTotal = fmtMoney(c.GalUnit * c.Qty)
		} else {
			cvm.GalInfeasible = true
		}
		vm.Components = append(vm.Components, cvm)
	}
	vm.MktBuildCost = fmtMoney(v.MktTotal)
	if v.MktPriced < v.MktCount {
		vm.MktNote = fmt.Sprintf("(%d/%d priced)", v.MktPriced, v.MktCount)
	}
	if v.GalFeasible {
		vm.GalBuildCost = fmtMoney(v.GalTotal)
	} else {
		vm.GalBuildCost = fmt.Sprintf("%d/%d covered", v.GalCovered, v.MktCount)
		vm.GalInfeasible = true
	}
	return vm
}

// facDetailHref links to a facility's existing KB detail page from a group page
// (three levels up to kb/, then facilities/<category>/<id>.html).
func facDetailHref(f FacilityRec) string {
	return "../../../facilities/" + f.Category + "/" + f.ID + ".html"
}

// buildFacilityPages groups the facilities, builds each facility's two cost
// views, and assembles the per-group pages (facilities alphabetical within a
// group) plus the landing summaries. Both outputs are group-name sorted; every
// page carries the full cross-group TOC with its own group flagged active.
func buildFacilityPages(recs []FacilityRec, facBoM map[string][]buildcost.Requirement, recipeOut, names, cats map[string]string, sellVWAP map[string]float64, galaxy *buildcost.Book) ([]FacilityGroupPage, []FacilityGroupSummary) {
	grouped := map[string][]FacilityEntryVM{}
	for _, f := range recs {
		g := facilityGroup(f, recipeOut, itemCatKey(cats, recipeOut, f))
		entry := FacilityEntryVM{ID: f.ID, Name: f.Name, Href: facDetailHref(f), Level: f.Level}
		if out := recipeOut[f.RecipeID]; out != "" && f.Category == "production" {
			entry.Produces = compName(out, names)
		}
		entry.BoM = facilityViewVM("BoM (ore)", buildFacilityView(facBoM[f.ID], sellVWAP, galaxy, names, cats))
		entry.Recipe = facilityViewVM("Recipe (components)", buildFacilityView(f.Build, sellVWAP, galaxy, names, cats))
		grouped[g] = append(grouped[g], entry)
	}

	groupNames := make([]string, 0, len(grouped))
	for g := range grouped {
		groupNames = append(groupNames, g)
	}
	sort.Strings(groupNames)

	summaries := make([]FacilityGroupSummary, 0, len(groupNames))
	for _, g := range groupNames {
		summaries = append(summaries, FacilityGroupSummary{Group: g, Href: g + "/", Count: len(grouped[g])})
	}

	pages := make([]FacilityGroupPage, 0, len(groupNames))
	for _, g := range groupNames {
		facs := grouped[g]
		sort.Slice(facs, func(i, j int) bool { return facs[i].Name < facs[j].Name })
		toc := make([]FacilityTOCEntry, 0, len(groupNames))
		for _, other := range groupNames {
			toc = append(toc, FacilityTOCEntry{Group: other, Href: "../" + other + "/", Count: len(grouped[other]), Active: other == g})
		}
		pages = append(pages, FacilityGroupPage{Group: g, Heading: g, Facilities: facs, TOC: toc})
	}
	return pages, summaries
}

// itemCatKey is a tiny indirection so facilityGroup receives the item-category
// map directly; kept separate to make the grouping call site readable.
func itemCatKey(cats, _ map[string]string, _ FacilityRec) map[string]string { return cats }
```

Note: `itemCatKey` is an unnecessary indirection — replace the grouped line with a direct call `facilityGroup(f, recipeOut, cats)` and delete `itemCatKey`. (Kept out of the final code.)

Corrected grouping line inside `buildFacilityPages`:

```go
		g := facilityGroup(f, recipeOut, cats)
```

Delete the `itemCatKey` function entirely.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run 'TestFacilityViewVM_CoverageAndPricedNote|TestBuildFacilityPages_GroupsSortTOC' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/facilities.go cmd/generate-build-costs/facilities_test.go
git commit -m "feat(build-costs): assemble facility group pages + TOC view-models"
```

---

### Task 7: Templates and render functions

**Files:**
- Create: `cmd/generate-build-costs/templates/facilities-index.html.tmpl`
- Create: `cmd/generate-build-costs/templates/facilities-group.html.tmpl`
- Create: `cmd/generate-build-costs/facilities_render.go`
- Test: `cmd/generate-build-costs/facilities_render_test.go`

**Interfaces:**
- Consumes: `FacilityGroupPage`, `FacilityGroupSummary` (Task 6); `tmplFS` embed (render.go).
- Produces:
  - `renderFacilitiesIndex(outDir string, summaries []FacilityGroupSummary) error` — writes `outDir/index.html`.
  - `renderFacilityGroup(outDir string, page FacilityGroupPage) error` — writes `outDir/<group>/index.html`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderFacilitiesIndex(t *testing.T) {
	dir := t.TempDir()
	summaries := []FacilityGroupSummary{
		{Group: "weapon", Href: "weapon/", Count: 2},
		{Group: "service", Href: "service/", Count: 1},
	}
	if err := renderFacilitiesIndex(dir, summaries); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"Facility Build Costs", `href="weapon/"`, "weapon", "service", ">2<"} {
		if !strings.Contains(s, want) {
			t.Fatalf("index missing %q", want)
		}
	}
}

func TestRenderFacilityGroup(t *testing.T) {
	dir := t.TempDir()
	page := FacilityGroupPage{
		Group: "weapon", Heading: "weapon",
		TOC: []FacilityTOCEntry{
			{Group: "service", Href: "../service/", Count: 1},
			{Group: "weapon", Href: "../weapon/", Count: 1, Active: true},
		},
		Facilities: []FacilityEntryVM{{
			ID: "a_forge", Name: "A Forge", Href: "../../../facilities/production/a_forge.html", Level: 1, Produces: "Railgun",
			BoM: ViewVM{Title: "BoM (ore)", MktBuildCost: "40.00", GalBuildCost: "36.00", Components: []ComponentVM{
				{Name: "Iron Ore", Href: "../../../items/ore/iron_ore.html", Qty: "2", MktUnit: "5.00", MktTotal: "10.00", GalUnit: "4.00", GalTotal: "8.00"},
			}},
			Recipe: ViewVM{Title: "Recipe (components)", MktBuildCost: "10.00", MktNote: "(1/2 priced)", GalBuildCost: "1/2 covered", GalInfeasible: true, Components: []ComponentVM{
				{Name: "Iron", Qty: "1", MktUnit: "—", MktTotal: "—", GalUnit: "—", GalTotal: "—", GalInfeasible: true},
			}},
		}},
	}
	if err := renderFacilityGroup(dir, page); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "weapon", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`id="a_forge"`,
		`href="../../../facilities/production/a_forge.html">A Forge`,
		"produces Railgun",
		"BoM (ore)", "Recipe (components)",
		`href="../../../items/ore/iron_ore.html">Iron Ore`,
		"(1/2 priced)", "1/2 covered",
		`href="../service/"`, // TOC sibling link
		"class=\"active\"",   // active TOC entry
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("group page missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run 'TestRenderFacilitiesIndex|TestRenderFacilityGroup' -v`
Expected: FAIL — `undefined: renderFacilitiesIndex`, `undefined: renderFacilityGroup`.

- [ ] **Step 3a: Create the landing template**

Create `cmd/generate-build-costs/templates/facilities-index.html.tmpl`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Facility Build Costs — Spacemolt KB</title>
<style>
 body{font-family:system-ui,sans-serif;margin:0;padding:1rem;background:#0d1117;color:#c9d1d9}
 a{color:#58a6ff} h1{margin:.3rem 0}
 .legend{font-size:.85rem;color:#8b949e;max-width:80ch}
 .cards{display:flex;flex-wrap:wrap;gap:.6rem;margin-top:1rem}
 .card{display:block;min-width:8rem;background:#161b22;border:1px solid #30363d;border-radius:6px;padding:.7rem .9rem;color:#c9d1d9;text-decoration:none}
 .card:hover{border-color:#58a6ff}
 .card .n{font-size:1.5rem;color:#e3b341;line-height:1}
 .card .g{text-transform:capitalize;margin-top:.2rem}
</style>
</head>
<body>
<p><a href="../">← Build Costs</a></p>
<h1>Facility Build Costs</h1>
<p class="legend">Cost to <strong>construct</strong> each facility, two ways: <strong>BoM (ore)</strong> flattens the build to raw materials; <strong>Recipe (components)</strong> uses the direct build components. Each is priced by <strong>MKT-AVG</strong> (sell-side volume-weighted average) and <strong>Galaxy</strong> (cheapest sourcing, walking pooled sell-order depth across every station). Production facilities are grouped by what they produce; others by facility type.</p>
<div class="cards">
{{range .}}<a class="card" href="{{.Href}}"><div class="n">{{.Count}}</div><div class="g">{{.Group}}</div></a>
{{end}}</div>
</body>
</html>
```

- [ ] **Step 3b: Create the group template**

Create `cmd/generate-build-costs/templates/facilities-group.html.tmpl`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Heading}} — Facility Build Costs — Spacemolt KB</title>
<style>
 body{font-family:system-ui,sans-serif;margin:0;padding:1rem;background:#0d1117;color:#c9d1d9}
 a{color:#58a6ff}
 table{border-collapse:collapse;font-size:.85rem;margin-top:.3rem}
 th,td{border:1px solid #21262d;padding:.3rem .5rem;text-align:right}
 th:first-child,td:first-child{text-align:left}
 .toc{display:flex;flex-wrap:wrap;gap:.35rem;margin:.5rem 0 1rem;font-size:.8rem}
 .toc a{background:#161b22;border:1px solid #30363d;border-radius:4px;padding:.15rem .5rem;text-decoration:none;text-transform:capitalize}
 .toc a.active{border-color:#e3b341;color:#e3b341}
 .fac{border-top:1px solid #30363d;margin-top:1.3rem;padding-top:.4rem;scroll-margin-top:.5rem}
 .fac h2{margin:.2rem 0;font-size:1.05rem} .fac h2 small{color:#8b949e;font-weight:400}
 .view h3{margin:.6rem 0 .1rem;font-size:.9rem;color:#c9d1d9}
 .infeasible{color:#6e7681} .note{color:#8b949e;font-weight:400} .empty{color:#6e7681;font-size:.85rem;margin:.2rem 0}
 h1{margin:.2rem 0;text-transform:capitalize}
</style>
</head>
<body>
<p><a href="../">← Facility Build Costs</a></p>
<div class="toc">
{{range .TOC}}<a href="{{.Href}}"{{if .Active}} class="active"{{end}}>{{.Group}} ({{.Count}})</a>
{{end}}</div>
<h1>{{.Heading}}</h1>
{{range .Facilities}}
<section class="fac" id="{{.ID}}">
 <h2><a href="{{.Href}}">{{.Name}}</a> <small>L{{.Level}}{{if .Produces}} · produces {{.Produces}}{{end}}</small></h2>
 {{template "facview" .BoM}}
 {{template "facview" .Recipe}}
</section>
{{end}}
{{define "facview"}}
<div class="view">
 <h3>{{.Title}}</h3>
 {{if .Empty}}<p class="empty">No components.</p>{{else}}
 <table>
 <thead><tr><th>Component</th><th>Qty</th><th>MKT-AVG</th><th>MKT total</th><th>Galaxy</th><th>Galaxy total</th></tr></thead>
 <tbody>
 {{range .Components}}<tr>
  <td>{{if .Href}}<a href="{{.Href}}">{{.Name}}</a>{{else}}{{.Name}}{{end}}</td>
  <td>{{.Qty}}</td><td>{{.MktUnit}}</td><td>{{.MktTotal}}</td>
  <td class="{{if .GalInfeasible}}infeasible{{end}}">{{.GalUnit}}</td>
  <td class="{{if .GalInfeasible}}infeasible{{end}}">{{.GalTotal}}</td>
 </tr>
 {{end}}
 <tr><td colspan="3"><strong>build cost</strong> <span class="note">{{.MktNote}}</span></td>
     <td><strong>{{.MktBuildCost}}</strong></td>
     <td colspan="2" class="{{if .GalInfeasible}}infeasible{{end}}"><strong>{{.GalBuildCost}}</strong></td></tr>
 </tbody></table>
 {{end}}
</div>
{{end}}
</body>
</html>
```

- [ ] **Step 3c: Create the render functions**

Create `cmd/generate-build-costs/facilities_render.go`:

```go
package main

import (
	"html/template"
	"os"
	"path/filepath"
)

// renderFacilitiesIndex writes the facility build-cost landing page.
func renderFacilitiesIndex(outDir string, summaries []FacilityGroupSummary) error {
	t, err := template.ParseFS(tmplFS, "templates/facilities-index.html.tmpl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, summaries)
}

// renderFacilityGroup writes one group's page to outDir/<group>/index.html.
func renderFacilityGroup(outDir string, page FacilityGroupPage) error {
	t, err := template.ParseFS(tmplFS, "templates/facilities-group.html.tmpl")
	if err != nil {
		return err
	}
	dir := filepath.Join(outDir, page.Group)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, page)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run 'TestRenderFacilitiesIndex|TestRenderFacilityGroup' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/templates/facilities-index.html.tmpl cmd/generate-build-costs/templates/facilities-group.html.tmpl cmd/generate-build-costs/facilities_render.go cmd/generate-build-costs/facilities_render_test.go
git commit -m "feat(build-costs): facility landing + group page templates and renderers"
```

---

### Task 8: Wire into main.go and generate for real

**Files:**
- Modify: `cmd/generate-build-costs/main.go`

**Interfaces:**
- Consumes: everything above; the already-loaded `craftDB`, `books`, `sellVWAP`, `itemNames`, `categories`, and `*catalogRoot` in `main`.
- Produces: the `-facilities-out` flag and generated pages under `kb/build-costs/facilities/`.

- [ ] **Step 1: Add the flag**

In `main.go`, after the existing `stationCoverOut` flag declaration (line ~29), add:

```go
	facilitiesOut := flag.String("facilities-out", "kb/build-costs/facilities", "output dir for facility build-cost pages; empty disables")
```

- [ ] **Step 2: Add the generation block**

In `main.go`, immediately before the final `log.Printf("build-costs: %d rows ...")` line (currently line ~149), insert:

```go
	if *facilitiesOut != "" {
		facRecs, err := loadFacilityCatalog(*catalogRoot)
		must(err, "load facility catalog")
		facBoM, err := loadFacilityBoM(craftDB)
		must(err, "load facility bom")
		recipeOut, err := loadRecipeOutputItem(craftDB)
		must(err, "load recipe outputs")
		gb := galaxyBook(books)
		fPages, fSummaries := buildFacilityPages(facRecs, facBoM, recipeOut, itemNames, categories, sellVWAP, gb)
		must(renderFacilitiesIndex(*facilitiesOut, fSummaries), "render facilities index")
		for _, p := range fPages {
			must(renderFacilityGroup(*facilitiesOut, p), "render facility group "+p.Group)
		}
		log.Printf("facility build-costs: %d facilities across %d groups → %s", len(facRecs), len(fPages), *facilitiesOut)
	}
```

- [ ] **Step 3: Build and run the full test suite**

Run: `go build ./... && go test ./cmd/generate-build-costs/...`
Expected: build clean; all tests PASS.

- [ ] **Step 4: Run the generator against the real DBs**

Run (from the repo root `/home/robert/spacemolt/kb`):

```bash
go run ./cmd/generate-build-costs \
  -crafting ./crafting.db \
  -market ../spacemolt/data/market.db \
  -knowledge ./spacemolt-knowledge.db \
  -catalog ../spacemolt/data/game-api \
  -out kb/build-costs \
  -facilities-out kb/build-costs/facilities \
  -station-cover-out ''
```

Expected: a log line like `facility build-costs: 2554 facilities across 17 groups → kb/build-costs/facilities`.

- [ ] **Step 5: Verify the output**

Run:

```bash
ls kb/build-costs/facilities/
ls kb/build-costs/facilities/weapon/ | head
grep -c 'class="fac"' kb/build-costs/facilities/weapon/index.html
grep -o 'produces [A-Za-z ]*' kb/build-costs/facilities/weapon/index.html | head -3
```

Expected: an `index.html` plus one directory per group (weapon, ammo, component, refined, utility, defense, consumable, contraband, mining, drone, material, ore, other, service, infrastructure, faction, personal — those that are non-empty); the weapon page's `class="fac"` count matches the weapon group count from the log; `produces …` lines present. Open `kb/build-costs/facilities/index.html` in a browser and confirm cards link to group pages, the horizontal TOC switches pages, and both cost tables render with MKT-AVG and Galaxy columns (galaxy showing `N/M covered` where depth is short).

- [ ] **Step 6: Run golangci-lint**

Run: `golangci-lint run ./cmd/generate-build-costs/...`
Expected: no new findings.

- [ ] **Step 7: Commit**

```bash
git add cmd/generate-build-costs/main.go
git commit -m "feat(build-costs): wire facility build-cost page generation into main"
```

---

## Self-Review

**Spec coverage:**
- Two cost views (BoM ore + Recipe components) → Tasks 3 (BoM load), 2 (build_materials load), 5 (both priced), 6 (both view-models).
- MKT-AVG (sell VWAP) pricing → Task 5 (`buildFacilityView`), reuses `loadSellVWAP`.
- Galaxy cheapest depth-walk from sell asks → Task 1 (`galaxyBook`) + Task 5 (`Walk`/`PriceRequirements`).
- Grouping: production by produced-item category, non-production by facility category, `other` fallback → Task 4.
- Pages mirror items/recipes (landing index + per-group pages) with a horizontal cross-group TOC → Tasks 6 (assembly/TOC) + 7 (templates/render).
- Facility name links to its detail page; component names link to item pages → Tasks 5 (`facItemHref`) + 6 (`facDetailHref`).
- Single build-cost total, no per-unit line → Task 6 view-model / Task 7 template (footer row only).
- Built into `cmd/generate-build-costs`, output `kb/build-costs/facilities/`, flag-gated → Task 8.
- Coverage shown when depth is short (`N/M covered`), `k/N priced` note → Tasks 5/6, asserted in tests.

**Placeholder scan:** No TBD/TODO. Every code step shows complete code. The one indirection (`itemCatKey`) is explicitly called out to be removed with the corrected line given.

**Type consistency:** `FacilityRec`, `FacilityView`, `FacilityComponentCost`, `ViewVM`, `ComponentVM`, `FacilityEntryVM`, `FacilityGroupPage`, `FacilityTOCEntry`, `FacilityGroupSummary` are defined once (Tasks 2/5/6) and consumed with matching field names in Tasks 6/7/8. Function signatures (`galaxyBook`, `fmtMoney`, `loadFacilityCatalog`, `loadFacilityBoM`, `loadRecipeOutputItem`, `facilityGroup`, `buildFacilityView`, `facilityViewVM`, `facDetailHref`, `facItemHref`, `compName`, `buildFacilityPages`, `renderFacilitiesIndex`, `renderFacilityGroup`) are consistent across their definition and call sites. Reused helpers (`commaInt`, `qtyStr`, `emDash`, `findLatestCatalogDir`, `pooledBook`, `tmplFS`, `loadSellVWAP`, `loadBooks`, `loadItemMeta`) exist in the current package.
