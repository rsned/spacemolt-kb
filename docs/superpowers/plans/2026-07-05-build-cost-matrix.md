# Build-Cost Matrix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a KB page showing the live-market build cost of every Item and Ship at every Station, for both BoM (raw-ore) and Recipe (sub-assembly) modes, with feasibility and build-vs-buy margins.

**Architecture:** A pure, unit-tested core package `pkg/buildcost` computes per-cell cost/feasibility/margin from in-memory order-book ladders (no DB coupling). A generator `cmd/generate-build-costs` loads the four data sources, runs the core over every (target × station), and renders a filterable landing matrix plus per-target detail pages under `kb/build-costs/`.

**Tech Stack:** Go 1.24+, `modernc.org/sqlite` (pure-Go driver, already used — imported as `_ "modernc.org/sqlite"` with `sql.Open("sqlite", ...)`), `html/template`, vanilla JS embedded in output.

## Global Constraints

- Go module: `github.com/rsned/spacemolt-kb`. Target Go 1.24+; use range-over-int and `b.Loop()` in any benchmarks.
- All new code must pass `golangci-lint` with no new findings. Run `go build ./...` and `go test ./...` before every commit.
- Built binaries go in `bin/` (gitignored), never the repo root. Watch `git add -A`.
- Generated HTML under `kb/` IS committed. Output lives under `kb/build-costs/`.
- Data source paths (resolved relative to `kb/`, matching `cmd/generate-items-kb`):
  - Crafting DB: `../../spacemolt-crafting-server/database/crafting.db`
  - Market DB: `../spacemolt/data/market.db` (this symlink/path resolves to the 6 GB order-book DB; the generator opens it read-only).
  - Knowledge DB: `../spacemolt-knowledge.db`
  - Ships catalog: latest dir under `../spacemolt/data/game-api/` → `catalog_ships.json` (use the existing `findLatestCatalogDir` pattern).
- SQLite opens are read-only where possible (`?mode=ro`); the market DB especially must never be written.
- `market.db` has a 6-hour TTL, so every present row is fresh — no staleness handling.

---

### Task 1: Order-book depth walk (`pkg/buildcost` core primitive)

**Files:**
- Create: `pkg/buildcost/types.go`
- Create: `pkg/buildcost/orderbook.go`
- Test: `pkg/buildcost/orderbook_test.go`

**Interfaces:**
- Consumes: nothing (pure).
- Produces:
  - `type Order struct { Price, Qty float64 }`
  - `type Ladder []Order` (sell ladders sorted ascending by Price)
  - `type Book struct { Sell map[string]Ladder; BestBuy map[string]float64 }`
  - `type WalkResult struct { Cost, Covered, Shortfall float64 }`
  - `func (b *Book) Walk(itemID string, qty float64) WalkResult`

- [ ] **Step 1: Write the failing test**

Create `pkg/buildcost/orderbook_test.go`:

```go
package buildcost

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestWalk_ExactFit(t *testing.T) {
	b := &Book{Sell: map[string]Ladder{"iron": {{Price: 10, Qty: 5}}}}
	got := b.Walk("iron", 5)
	if !approx(got.Cost, 50) || !approx(got.Covered, 5) || got.Shortfall != 0 {
		t.Fatalf("exact fit: got %+v, want cost 50 covered 5 shortfall 0", got)
	}
}

func TestWalk_MultiTierAscending(t *testing.T) {
	// Must consume cheapest first: 3@10 then 2@25 for qty 5 => 30+50=80.
	b := &Book{Sell: map[string]Ladder{"iron": {{Price: 10, Qty: 3}, {Price: 25, Qty: 10}}}}
	got := b.Walk("iron", 5)
	if !approx(got.Cost, 80) || !approx(got.Covered, 5) || got.Shortfall != 0 {
		t.Fatalf("multi tier: got %+v, want cost 80 covered 5", got)
	}
}

func TestWalk_ShortBook(t *testing.T) {
	b := &Book{Sell: map[string]Ladder{"iron": {{Price: 10, Qty: 2}}}}
	got := b.Walk("iron", 5)
	if !approx(got.Cost, 20) || !approx(got.Covered, 2) || !approx(got.Shortfall, 3) {
		t.Fatalf("short book: got %+v, want cost 20 covered 2 shortfall 3", got)
	}
}

func TestWalk_EmptyBook(t *testing.T) {
	b := &Book{Sell: map[string]Ladder{}}
	got := b.Walk("iron", 5)
	if got.Cost != 0 || got.Covered != 0 || !approx(got.Shortfall, 5) {
		t.Fatalf("empty book: got %+v, want cost 0 covered 0 shortfall 5", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/buildcost/ -run TestWalk -v`
Expected: FAIL — `undefined: Book` / package does not compile.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/buildcost/types.go`:

```go
// Package buildcost computes the live-market cost, feasibility, and margin of
// building an item or ship at a station, walking order-book depth. It is pure:
// callers supply in-memory order books; the package touches no database.
package buildcost

// Order is one resting order at a price level.
type Order struct {
	Price float64
	Qty   float64
}

// Ladder is the price-sorted resting depth for one (station, item) sell side.
// Sell ladders are sorted ascending by Price (cheapest first).
type Ladder []Order

// Book is the current order book at a single station.
// Sell maps item_id to its ascending sell ladder.
// BestBuy maps item_id to the highest resting buy price (0 if none).
type Book struct {
	Sell    map[string]Ladder
	BestBuy map[string]float64
}

// WalkResult is the outcome of covering a required quantity from a sell ladder.
// Cost and Covered describe what was actually purchasable; Shortfall is the
// unmet quantity (0 when fully covered).
type WalkResult struct {
	Cost      float64
	Covered   float64
	Shortfall float64
}
```

Create `pkg/buildcost/orderbook.go`:

```go
package buildcost

// Walk covers qty of itemID by consuming the sell ladder cheapest-first.
// It returns the cost of what was covered and any shortfall.
func (b *Book) Walk(itemID string, qty float64) WalkResult {
	remaining := qty
	var cost float64
	for _, o := range b.Sell[itemID] {
		if remaining <= 0 {
			break
		}
		take := o.Qty
		if take > remaining {
			take = remaining
		}
		cost += take * o.Price
		remaining -= take
	}
	covered := qty - remaining
	if remaining < 0 {
		remaining = 0
	}
	return WalkResult{Cost: cost, Covered: covered, Shortfall: remaining}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/buildcost/ -run TestWalk -v`
Expected: PASS (all four subtests).

- [ ] **Step 5: Commit**

```bash
git add pkg/buildcost/types.go pkg/buildcost/orderbook.go pkg/buildcost/orderbook_test.go
git commit -m "feat(buildcost): order-book depth walk primitive"
```

---

### Task 2: Price a requirement set (BoM cost + feasibility)

**Files:**
- Create: `pkg/buildcost/cost.go`
- Test: `pkg/buildcost/cost_test.go`

**Interfaces:**
- Consumes: `Book.Walk` (Task 1).
- Produces:
  - `type Requirement struct { ItemID string; Qty float64 }`
  - `type Shortfall struct { ItemID string; Short float64 }`
  - `type ModeResult struct { Cost float64; Covered, Total int; Feasible bool; Shortfalls []Shortfall; RecipeID string; NA bool; NAReason string }`
  - `func (b *Book) PriceRequirements(reqs []Requirement) ModeResult`

- [ ] **Step 1: Write the failing test**

Create `pkg/buildcost/cost_test.go`:

```go
package buildcost

import "testing"

func bookFixture() *Book {
	return &Book{
		Sell: map[string]Ladder{
			"iron":   {{Price: 10, Qty: 100}},
			"copper": {{Price: 5, Qty: 100}},
			"gold":   {{Price: 50, Qty: 1}}, // thin
		},
		BestBuy: map[string]float64{},
	}
}

func TestPriceRequirements_AllCovered(t *testing.T) {
	b := bookFixture()
	got := b.PriceRequirements([]Requirement{{"iron", 2}, {"copper", 4}})
	if !got.Feasible || got.Covered != 2 || got.Total != 2 {
		t.Fatalf("feasible: got %+v", got)
	}
	if !approx(got.Cost, 2*10+4*5) { // 40
		t.Fatalf("cost: got %v want 40", got.Cost)
	}
	if len(got.Shortfalls) != 0 {
		t.Fatalf("expected no shortfalls, got %+v", got.Shortfalls)
	}
}

func TestPriceRequirements_PartialInfeasible(t *testing.T) {
	b := bookFixture()
	got := b.PriceRequirements([]Requirement{{"iron", 2}, {"gold", 5}})
	if got.Feasible {
		t.Fatalf("expected infeasible, got %+v", got)
	}
	if got.Covered != 1 || got.Total != 2 { // iron covered, gold not
		t.Fatalf("coverage: got covered=%d total=%d", got.Covered, got.Total)
	}
	// Partial cost still reported: iron 20 + gold 1 unit @50 = 70.
	if !approx(got.Cost, 70) {
		t.Fatalf("partial cost: got %v want 70", got.Cost)
	}
	if len(got.Shortfalls) != 1 || got.Shortfalls[0].ItemID != "gold" || !approx(got.Shortfalls[0].Short, 4) {
		t.Fatalf("shortfall: got %+v", got.Shortfalls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/buildcost/ -run TestPriceRequirements -v`
Expected: FAIL — `undefined: Requirement` / `PriceRequirements`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/buildcost/cost.go`:

```go
package buildcost

// Requirement is a quantity of a material needed to build a target.
type Requirement struct {
	ItemID string
	Qty    float64
}

// Shortfall records an unmet material quantity.
type Shortfall struct {
	ItemID string
	Short  float64
}

// ModeResult is a per-station build cost for one mode (BoM or Recipe).
// Cost is the total, or the partial cost of covered materials when infeasible.
// Covered/Total count materials fully satisfiable from depth. RecipeID names the
// chosen recipe (Recipe mode only). NA marks a mode that does not apply to this
// target (e.g. ship Recipe mode); NAReason explains why.
type ModeResult struct {
	Cost       float64
	Covered    int
	Total      int
	Feasible   bool
	Shortfalls []Shortfall
	RecipeID   string
	NA         bool
	NAReason   string
}

// PriceRequirements walks each requirement against the book and aggregates the
// cost, coverage, and shortfalls. Feasible is true only if every requirement is
// fully covered.
func (b *Book) PriceRequirements(reqs []Requirement) ModeResult {
	res := ModeResult{Total: len(reqs), Feasible: true}
	for _, r := range reqs {
		w := b.Walk(r.ItemID, r.Qty)
		res.Cost += w.Cost
		if w.Shortfall <= 0 {
			res.Covered++
		} else {
			res.Feasible = false
			res.Shortfalls = append(res.Shortfalls, Shortfall{ItemID: r.ItemID, Short: w.Shortfall})
		}
	}
	return res
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/buildcost/ -run TestPriceRequirements -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/buildcost/cost.go pkg/buildcost/cost_test.go
git commit -m "feat(buildcost): price requirement sets with feasibility and shortfalls"
```

---

### Task 3: Cheapest-feasible recipe selection

**Files:**
- Modify: `pkg/buildcost/cost.go` (add `Recipe` type and `CheapestRecipe`)
- Test: `pkg/buildcost/recipe_test.go`

**Interfaces:**
- Consumes: `Book.PriceRequirements` (Task 2).
- Produces:
  - `type Recipe struct { ID string; OutputQty float64; Inputs []Requirement }`
  - `func (b *Book) CheapestRecipe(recipes []Recipe) ModeResult` — evaluates all recipes; returns the cheapest **feasible** one (per-unit cost, i.e. cost scaled by 1/OutputQty), setting `RecipeID`. If none is feasible, returns the one with the lowest partial cost, still infeasible. Returns an `NA` result if `recipes` is empty.

- [ ] **Step 1: Write the failing test**

Create `pkg/buildcost/recipe_test.go`:

```go
package buildcost

import "testing"

func TestCheapestRecipe_PicksCheapestFeasible(t *testing.T) {
	b := bookFixture()
	// Recipe A: 5 iron => 50. Recipe B: 4 copper => 20 (cheaper, feasible).
	recipes := []Recipe{
		{ID: "recA", OutputQty: 1, Inputs: []Requirement{{"iron", 5}}},
		{ID: "recB", OutputQty: 1, Inputs: []Requirement{{"copper", 4}}},
	}
	got := b.CheapestRecipe(recipes)
	if !got.Feasible || got.RecipeID != "recB" || !approx(got.Cost, 20) {
		t.Fatalf("cheapest feasible: got %+v want recB cost 20", got)
	}
}

func TestCheapestRecipe_PrefersFeasibleOverCheaperInfeasible(t *testing.T) {
	b := bookFixture()
	// recCheapButShort needs 5 gold (only 1 available) -> infeasible though nominally cheap.
	// recFeasible needs 5 iron -> feasible at 50.
	recipes := []Recipe{
		{ID: "recCheapButShort", OutputQty: 1, Inputs: []Requirement{{"gold", 5}}},
		{ID: "recFeasible", OutputQty: 1, Inputs: []Requirement{{"iron", 5}}},
	}
	got := b.CheapestRecipe(recipes)
	if !got.Feasible || got.RecipeID != "recFeasible" || !approx(got.Cost, 50) {
		t.Fatalf("prefer feasible: got %+v want recFeasible cost 50", got)
	}
}

func TestCheapestRecipe_OutputQtyScales(t *testing.T) {
	b := bookFixture()
	// 4 copper => 20 total, but recipe yields 2 units => per-unit cost 10.
	recipes := []Recipe{{ID: "rec2", OutputQty: 2, Inputs: []Requirement{{"copper", 4}}}}
	got := b.CheapestRecipe(recipes)
	if !approx(got.Cost, 10) {
		t.Fatalf("output scaling: got %v want 10", got.Cost)
	}
}

func TestCheapestRecipe_AllInfeasibleReturnsLowestPartial(t *testing.T) {
	b := bookFixture()
	recipes := []Recipe{
		{ID: "r1", OutputQty: 1, Inputs: []Requirement{{"gold", 5}}}, // partial 50
		{ID: "r2", OutputQty: 1, Inputs: []Requirement{{"gold", 10}}}, // partial 50 too, more short
	}
	got := b.CheapestRecipe(recipes)
	if got.Feasible {
		t.Fatalf("expected infeasible, got %+v", got)
	}
	if got.RecipeID == "" {
		t.Fatalf("expected a chosen recipe id even when infeasible")
	}
}

func TestCheapestRecipe_EmptyIsNA(t *testing.T) {
	b := bookFixture()
	got := b.CheapestRecipe(nil)
	if !got.NA {
		t.Fatalf("empty recipes should be NA, got %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/buildcost/ -run TestCheapestRecipe -v`
Expected: FAIL — `undefined: Recipe` / `CheapestRecipe`.

- [ ] **Step 3: Write minimal implementation**

Append to `pkg/buildcost/cost.go`:

```go
// Recipe is one way to produce a target: its direct inputs and how many units
// of the target it yields (OutputQty, at least 1).
type Recipe struct {
	ID        string
	OutputQty float64
	Inputs    []Requirement
}

// CheapestRecipe prices every recipe and returns the cheapest feasible one
// (per-unit cost = total input cost / OutputQty), tagging RecipeID. When no
// recipe is feasible, it returns the lowest partial-cost result (still
// infeasible). An empty recipe list yields an NA result.
func (b *Book) CheapestRecipe(recipes []Recipe) ModeResult {
	if len(recipes) == 0 {
		return ModeResult{NA: true, NAReason: "no recipe"}
	}
	var best ModeResult
	var haveBest bool
	for _, rec := range recipes {
		r := b.PriceRequirements(rec.Inputs)
		out := rec.OutputQty
		if out < 1 {
			out = 1
		}
		r.Cost /= out
		r.RecipeID = rec.ID
		if !haveBest || better(r, best) {
			best, haveBest = r, true
		}
	}
	return best
}

// better reports whether candidate c should replace the current best: a feasible
// result always beats an infeasible one; among same feasibility, lower cost wins.
func better(c, best ModeResult) bool {
	if c.Feasible != best.Feasible {
		return c.Feasible
	}
	return c.Cost < best.Cost
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/buildcost/ -run TestCheapestRecipe -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/buildcost/cost.go pkg/buildcost/recipe_test.go
git commit -m "feat(buildcost): cheapest-feasible recipe selection"
```

---

### Task 4: Margin math

**Files:**
- Create: `pkg/buildcost/margin.go`
- Test: `pkg/buildcost/margin_test.go`

**Interfaces:**
- Consumes: nothing (pure).
- Produces:
  - `type Margin struct { FinishedAsk, FinishedBid float64; HasAsk, HasBid bool }`
  - `func (m Margin) SavingsVsAsk(cost float64) (float64, bool)` — `(ask-cost, true)` when `HasAsk`, else `(0, false)`.
  - `func (m Margin) ProfitVsBid(cost float64) (float64, bool)` — `(bid-cost, true)` when `HasBid`, else `(0, false)`.

- [ ] **Step 1: Write the failing test**

Create `pkg/buildcost/margin_test.go`:

```go
package buildcost

import "testing"

func TestMargin_SavingsAndProfit(t *testing.T) {
	m := Margin{FinishedAsk: 6000, FinishedBid: 4500, HasAsk: true, HasBid: true}
	if s, ok := m.SavingsVsAsk(1894); !ok || !approx(s, 4106) {
		t.Fatalf("savings: got %v ok=%v want 4106", s, ok)
	}
	if p, ok := m.ProfitVsBid(1894); !ok || !approx(p, 2606) {
		t.Fatalf("profit: got %v ok=%v want 2606", p, ok)
	}
}

func TestMargin_Absent(t *testing.T) {
	m := Margin{}
	if _, ok := m.SavingsVsAsk(100); ok {
		t.Fatalf("expected no ask")
	}
	if _, ok := m.ProfitVsBid(100); ok {
		t.Fatalf("expected no bid")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/buildcost/ -run TestMargin -v`
Expected: FAIL — `undefined: Margin`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/buildcost/margin.go`:

```go
package buildcost

// Margin holds the finished good's own price at a station for build-vs-buy
// comparison. FinishedAsk is what you'd pay to acquire it; FinishedBid is what
// a buyer will pay you. Has* flags distinguish a genuine 0 from "unknown".
type Margin struct {
	FinishedAsk float64
	FinishedBid float64
	HasAsk      bool
	HasBid      bool
}

// SavingsVsAsk is finished ask minus build cost (positive = building is cheaper).
func (m Margin) SavingsVsAsk(cost float64) (float64, bool) {
	if !m.HasAsk {
		return 0, false
	}
	return m.FinishedAsk - cost, true
}

// ProfitVsBid is finished bid minus build cost (positive = craft-and-sell profit).
func (m Margin) ProfitVsBid(cost float64) (float64, bool) {
	if !m.HasBid {
		return 0, false
	}
	return m.FinishedBid - cost, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/buildcost/ -run TestMargin -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/buildcost/margin.go pkg/buildcost/margin_test.go
git commit -m "feat(buildcost): build-vs-buy margin math"
```

---

### Task 5: Cell assembly (combine modes + margin per target×station)

**Files:**
- Create: `pkg/buildcost/cell.go`
- Test: `pkg/buildcost/cell_test.go`

**Interfaces:**
- Consumes: `PriceRequirements`, `CheapestRecipe`, `Margin` (Tasks 2–4).
- Produces:
  - `type Target struct { ID string; Kind string; BoM []Requirement; Recipes []Recipe; RecipeNA string }` where `Kind` is `"item"` or `"ship"`; `RecipeNA` (non-empty) forces Recipe mode NA with that reason (used for ships).
  - `type Cell struct { TargetID, StationID string; BoM, Recipe ModeResult; Margin Margin }`
  - `func BuildCell(t Target, stationID string, b *Book, m Margin) Cell`

- [ ] **Step 1: Write the failing test**

Create `pkg/buildcost/cell_test.go`:

```go
package buildcost

import "testing"

func TestBuildCell_Item_BothModes(t *testing.T) {
	b := bookFixture()
	tgt := Target{
		ID: "widget", Kind: "item",
		BoM:     []Requirement{{"iron", 2}, {"copper", 4}},          // 40, feasible
		Recipes: []Recipe{{ID: "r", OutputQty: 1, Inputs: []Requirement{{"copper", 4}}}}, // 20
	}
	m := Margin{FinishedAsk: 100, HasAsk: true}
	c := BuildCell(tgt, "st1", b, m)
	if c.TargetID != "widget" || c.StationID != "st1" {
		t.Fatalf("ids: got %+v", c)
	}
	if !c.BoM.Feasible || !approx(c.BoM.Cost, 40) {
		t.Fatalf("bom: got %+v", c.BoM)
	}
	if c.Recipe.NA || !c.Recipe.Feasible || c.Recipe.RecipeID != "r" || !approx(c.Recipe.Cost, 20) {
		t.Fatalf("recipe: got %+v", c.Recipe)
	}
}

func TestBuildCell_Ship_RecipeNA(t *testing.T) {
	b := bookFixture()
	tgt := Target{
		ID: "frigate", Kind: "ship",
		BoM:      []Requirement{{"iron", 2}},
		RecipeNA: "sub-assemblies not market-traded",
	}
	c := BuildCell(tgt, "st1", b, Margin{})
	if !c.Recipe.NA || c.Recipe.NAReason != "sub-assemblies not market-traded" {
		t.Fatalf("ship recipe should be NA: got %+v", c.Recipe)
	}
	if !c.BoM.Feasible || !approx(c.BoM.Cost, 20) {
		t.Fatalf("ship bom: got %+v", c.BoM)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/buildcost/ -run TestBuildCell -v`
Expected: FAIL — `undefined: Target` / `BuildCell`.

- [ ] **Step 3: Write minimal implementation**

Create `pkg/buildcost/cell.go`:

```go
package buildcost

// Target is a buildable item or ship: its fully-decomposed BoM requirements and
// (for items) its candidate recipes. RecipeNA, when non-empty, forces Recipe
// mode to NA with that reason (ships, whose sub-assemblies are not market-traded).
type Target struct {
	ID       string
	Kind     string // "item" or "ship"
	BoM      []Requirement
	Recipes  []Recipe
	RecipeNA string
}

// Cell is the computed build cost of one target at one station.
type Cell struct {
	TargetID  string
	StationID string
	BoM       ModeResult
	Recipe    ModeResult
	Margin    Margin
}

// BuildCell computes BoM and Recipe results for target t at a station whose order
// book is b and whose finished-good margin is m.
func BuildCell(t Target, stationID string, b *Book, m Margin) Cell {
	c := Cell{TargetID: t.ID, StationID: stationID, Margin: m}
	c.BoM = b.PriceRequirements(t.BoM)
	if t.RecipeNA != "" {
		c.Recipe = ModeResult{NA: true, NAReason: t.RecipeNA}
	} else {
		c.Recipe = b.CheapestRecipe(t.Recipes)
	}
	return c
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/buildcost/ -run TestBuildCell -v` then `go test ./pkg/buildcost/ -v`
Expected: PASS (whole package green).

- [ ] **Step 5: Commit**

```bash
git add pkg/buildcost/cell.go pkg/buildcost/cell_test.go
git commit -m "feat(buildcost): per-cell assembly of BoM, Recipe, and margin"
```

---

### Task 6: Data loaders (`cmd/generate-build-costs/load.go`)

**Files:**
- Create: `cmd/generate-build-costs/load.go`
- Test: `cmd/generate-build-costs/load_test.go`

**Interfaces:**
- Consumes: `buildcost.{Book, Ladder, Order, Target, Requirement, Recipe, Margin}`.
- Produces (all in `package main`):
  - `type StationMeta struct { ID, Name, System, Empire string }`
  - `func loadBooks(marketDB *sql.DB) (map[string]*buildcost.Book, error)` — latest snapshot per station.
  - `func loadStations(marketDB, knowledgeDB *sql.DB) ([]StationMeta, error)` — station→system→empire.
  - `func loadTargets(craftDB *sql.DB, ships []Ship) ([]buildcost.Target, map[string]string, error)` — returns targets plus an id→display-name map. `Ship` is the struct from `cmd/generate-items-kb/ships.go`; copy the minimal loader (`loadShips`) or a trimmed struct into this package (see Step 3).
  - `func itemMargin(book *buildcost.Book, finishedID string) buildcost.Margin` — ask=cheapest sell, bid=highest buy for the finished item at that station.
  - `func shipMargin(listings map[string]map[string]float64, catalogPrice map[string]int, shipID, stationID string) buildcost.Margin` — per-station listing ask (min) with catalog-price fallback; never a bid.
  - `func loadShipListings(knowledgeDB *sql.DB) (map[string]map[string]float64, error)` — `stationID -> shipKey -> minPrice`, where `shipKey` is the `class_id` normalized (see Step 3).

The **finished-item margin** needs the best buy price too. Extend the book loader to also populate `BestBuy` (highest buy price per item). `itemMargin` reads `book.Sell[id][0].Price` for ask and `book.BestBuy[id]` for bid.

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-build-costs/load_test.go`. This test builds a tiny in-memory SQLite DB for the market side and asserts the loaders assemble books and margins correctly.

```go
package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newMarketTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE market_orders (station_id TEXT, item_id TEXT, side TEXT, price_each REAL, quantity REAL, captured_at TEXT);
CREATE TABLE stations (station_id TEXT PRIMARY KEY, station_name TEXT, system_id TEXT, system_name TEXT);
INSERT INTO stations VALUES ('st1','Station One','sysA','System A');
-- stale snapshot (ignored) then fresh snapshot for st1
INSERT INTO market_orders VALUES ('st1','iron','sell',99,1,'2026-07-05T10:00:00Z');
INSERT INTO market_orders VALUES ('st1','iron','sell',10,5,'2026-07-05T16:00:00Z');
INSERT INTO market_orders VALUES ('st1','iron','sell',12,5,'2026-07-05T16:00:00Z');
INSERT INTO market_orders VALUES ('st1','iron','buy',8,3,'2026-07-05T16:00:00Z');
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadBooks_LatestSnapshotAndSortedLadder(t *testing.T) {
	db := newMarketTestDB(t)
	books, err := loadBooks(db)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := books["st1"]
	if !ok {
		t.Fatal("no book for st1")
	}
	// The stale 99-priced order must be excluded; ladder sorted ascending 10 then 12.
	if len(b.Sell["iron"]) != 2 || b.Sell["iron"][0].Price != 10 || b.Sell["iron"][1].Price != 12 {
		t.Fatalf("ladder: %+v", b.Sell["iron"])
	}
	if b.BestBuy["iron"] != 8 {
		t.Fatalf("best buy: %v want 8", b.BestBuy["iron"])
	}
}

func TestItemMargin(t *testing.T) {
	db := newMarketTestDB(t)
	books, _ := loadBooks(db)
	m := itemMargin(books["st1"], "iron")
	if !m.HasAsk || m.FinishedAsk != 10 || !m.HasBid || m.FinishedBid != 8 {
		t.Fatalf("margin: %+v", m)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run 'TestLoadBooks|TestItemMargin' -v`
Expected: FAIL — `undefined: loadBooks` / `itemMargin`.

- [ ] **Step 3: Write minimal implementation**

Create `cmd/generate-build-costs/load.go`:

```go
package main

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// StationMeta is a station column: its id, display name, system, and empire.
type StationMeta struct {
	ID     string
	Name   string
	System string
	Empire string
}

// loadBooks builds each station's current order book from its most recent
// snapshot (MAX(captured_at) per station). Sell ladders are sorted ascending by
// price; BestBuy holds the highest resting buy price per item.
func loadBooks(marketDB *sql.DB) (map[string]*buildcost.Book, error) {
	rows, err := marketDB.Query(`
WITH latest AS (
  SELECT station_id, MAX(captured_at) AS cap FROM market_orders GROUP BY station_id
)
SELECT o.station_id, o.item_id, o.side, o.price_each, o.quantity
FROM market_orders o
JOIN latest l ON o.station_id = l.station_id AND o.captured_at = l.cap`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books := map[string]*buildcost.Book{}
	for rows.Next() {
		var st, item, side string
		var price, qty float64
		if err := rows.Scan(&st, &item, &side, &price, &qty); err != nil {
			return nil, err
		}
		b := books[st]
		if b == nil {
			b = &buildcost.Book{Sell: map[string]buildcost.Ladder{}, BestBuy: map[string]float64{}}
			books[st] = b
		}
		switch side {
		case "sell":
			b.Sell[item] = append(b.Sell[item], buildcost.Order{Price: price, Qty: qty})
		case "buy":
			if price > b.BestBuy[item] {
				b.BestBuy[item] = price
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, b := range books {
		for item := range b.Sell {
			l := b.Sell[item]
			sort.Slice(l, func(i, j int) bool { return l[i].Price < l[j].Price })
			b.Sell[item] = l
		}
	}
	return books, nil
}

// loadStations returns station columns joined to their empire via the knowledge
// DB systems table (station.system_id -> systems.id -> systems.empire).
func loadStations(marketDB, knowledgeDB *sql.DB) ([]StationMeta, error) {
	empire := map[string]string{}
	erows, err := knowledgeDB.Query(`SELECT id, COALESCE(empire,'') FROM systems`)
	if err != nil {
		return nil, err
	}
	defer erows.Close()
	for erows.Next() {
		var id, emp string
		if err := erows.Scan(&id, &emp); err != nil {
			return nil, err
		}
		empire[id] = emp
	}
	if err := erows.Err(); err != nil {
		return nil, err
	}

	rows, err := marketDB.Query(`SELECT station_id, station_name, system_id, system_name FROM stations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StationMeta
	for rows.Next() {
		var m StationMeta
		var sysName string
		if err := rows.Scan(&m.ID, &m.Name, &m.System, &sysName); err != nil {
			return nil, err
		}
		m.Empire = empire[m.System]
		if m.Empire == "" {
			m.Empire = "Independent"
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// itemMargin derives an item's finished-good margin at a station: ask = cheapest
// sell (first ladder entry), bid = highest buy.
func itemMargin(book *buildcost.Book, finishedID string) buildcost.Margin {
	var m buildcost.Margin
	if book == nil {
		return m
	}
	if l := book.Sell[finishedID]; len(l) > 0 {
		m.FinishedAsk, m.HasAsk = l[0].Price, true
	}
	if bid, ok := book.BestBuy[finishedID]; ok && bid > 0 {
		m.FinishedBid, m.HasBid = bid, true
	}
	return m
}

// normalizeShipKey lowercases and strips a leading empire prefix so a catalog
// ship id (e.g. "outerrim_cobble") and a ship_listings class_id ("cobble") map to
// the same key. Both the full and prefix-stripped forms are indexed by callers.
func normalizeShipKey(s string) string {
	s = strings.ToLower(s)
	if i := strings.Index(s, "_"); i >= 0 {
		return s[i+1:]
	}
	return s
}
```

Add ship listing + margin loaders in the same file:

```go
// loadShipListings returns stationID -> shipKey -> minimum listed price, where
// shipKey is the normalized class_id. Only the cheapest listing per ship/station
// is kept (the ask you'd actually pay).
func loadShipListings(knowledgeDB *sql.DB) (map[string]map[string]float64, error) {
	rows, err := knowledgeDB.Query(`
WITH latest AS (SELECT station_id, class_id, MIN(price) AS p
                FROM ship_listings GROUP BY station_id, class_id)
SELECT station_id, class_id, p FROM latest`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]float64{}
	for rows.Next() {
		var st, classID string
		var price float64
		if err := rows.Scan(&st, &classID, &price); err != nil {
			return nil, err
		}
		if out[st] == nil {
			out[st] = map[string]float64{}
		}
		out[st][normalizeShipKey(classID)] = price
	}
	return out, rows.Err()
}

// shipMargin returns a ship's finished-good margin at a station: ask from the
// per-station listing (matched by normalized ship id) with a catalog-price
// fallback; never a bid.
func shipMargin(listings map[string]map[string]float64, catalogPrice map[string]int, shipID, stationID string) buildcost.Margin {
	var m buildcost.Margin
	key := normalizeShipKey(shipID)
	if st := listings[stationID]; st != nil {
		if p, ok := st[key]; ok {
			m.FinishedAsk, m.HasAsk = p, true
			return m
		}
	}
	if p, ok := catalogPrice[shipID]; ok && p > 0 {
		m.FinishedAsk, m.HasAsk = float64(p), true
	}
	return m
}
```

Add the target loader (BoM + item recipes; ships get `RecipeNA`). `Ship` here is a trimmed local struct decoded from `catalog_ships.json` — do not import `cmd/generate-items-kb` (Go forbids importing `main`). Define the minimal shape:

```go
// Ship is the minimal ship-catalog shape this generator needs.
type Ship struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Class string `json:"class"`
	Price int    `json:"price"`
}

// loadTargets builds the row set: every item and ship in bill_of_materials, with
// BoM requirements from that table and (items only) candidate recipes from
// recipe_inputs/recipe_outputs. The second return maps target id -> display name.
func loadTargets(craftDB *sql.DB, ships []Ship, itemNames map[string]string) ([]buildcost.Target, map[string]string, error) {
	// 1. BoM rows: target -> requirements, split by kind.
	type key struct{ id, kind string }
	bom := map[key][]buildcost.Requirement{}
	brows, err := craftDB.Query(`SELECT target_id, target_type, base_item_id, quantity
	                             FROM bill_of_materials WHERE target_type IN ('item','ship')`)
	if err != nil {
		return nil, nil, err
	}
	for brows.Next() {
		var id, kind, base string
		var qty float64
		if err := brows.Scan(&id, &kind, &base, &qty); err != nil {
			brows.Close()
			return nil, nil, err
		}
		k := key{id, kind}
		bom[k] = append(bom[k], buildcost.Requirement{ItemID: base, Qty: qty})
	}
	brows.Close()
	if err := brows.Err(); err != nil {
		return nil, nil, err
	}

	// 2. Item recipes: output -> []Recipe (inputs + output qty).
	recipes := map[string][]buildcost.Recipe{}
	rrows, err := craftDB.Query(`
SELECT ro.item_id AS output, ri.recipe_id, ri.item_id AS input, ri.quantity, ro.quantity AS out_qty
FROM recipe_inputs ri JOIN recipe_outputs ro ON ri.recipe_id = ro.recipe_id
ORDER BY ro.item_id, ri.recipe_id`)
	if err != nil {
		return nil, nil, err
	}
	// Accumulate inputs per (output, recipe_id).
	type rkey struct{ output, recipe string }
	acc := map[rkey]*buildcost.Recipe{}
	var order []rkey
	for rrows.Next() {
		var output, recipeID, input string
		var qty, outQty float64
		if err := rrows.Scan(&output, &recipeID, &input, &qty, &outQty); err != nil {
			rrows.Close()
			return nil, nil, err
		}
		rk := rkey{output, recipeID}
		r := acc[rk]
		if r == nil {
			r = &buildcost.Recipe{ID: recipeID, OutputQty: outQty}
			acc[rk] = r
			order = append(order, rk)
		}
		r.Inputs = append(r.Inputs, buildcost.Requirement{ItemID: input, Qty: qty})
	}
	rrows.Close()
	if err := rrows.Err(); err != nil {
		return nil, nil, err
	}
	for _, rk := range order {
		recipes[rk.output] = append(recipes[rk.output], *acc[rk])
	}

	// 3. Assemble targets.
	names := map[string]string{}
	var targets []buildcost.Target
	shipSet := map[string]bool{}
	for _, s := range ships {
		shipSet[s.ID] = true
		names[s.ID] = s.Name
	}
	for k, reqs := range bom {
		t := buildcost.Target{ID: k.id, Kind: k.kind, BoM: reqs}
		if k.kind == "item" {
			t.Recipes = recipes[k.id]
			if n, ok := itemNames[k.id]; ok {
				names[k.id] = n
			} else {
				names[k.id] = k.id
			}
		} else { // ship
			t.RecipeNA = "sub-assemblies not market-traded"
			if _, ok := names[k.id]; !ok {
				names[k.id] = k.id
			}
		}
		targets = append(targets, t)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
	return targets, names, nil
}
```

Note: `itemNames` (id→name) and ship `catalogPrice` maps are built in `main.go` (Task 10) from `crafting.db` `items` and the ships catalog respectively, and passed in.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run 'TestLoadBooks|TestItemMargin' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/load.go cmd/generate-build-costs/load_test.go
git commit -m "feat(generate-build-costs): DB loaders for books, stations, targets, margins"
```

---

### Task 7: Matrix model (compute every cell into a render-ready structure)

**Files:**
- Create: `cmd/generate-build-costs/matrix.go`
- Test: `cmd/generate-build-costs/matrix_test.go`

**Interfaces:**
- Consumes: `buildcost.BuildCell`, loaders (Task 6).
- Produces:
  - `type RowCell struct { BoMCost float64; BoMFeasible bool; BoMCovered, BoMTotal int; RecipeCost float64; RecipeFeasible, RecipeNA bool; RecipeID string; SavingsBoM float64; HasSavings bool; ProfitBoM float64; HasProfit bool }`
  - `type MatrixRow struct { ID, Name, Kind, Category string; Cells map[string]RowCell; CheapestStation string; CheapestCost float64; FeasibleCount int }`
  - `type Matrix struct { Stations []StationMeta; Rows []MatrixRow }`
  - `func BuildMatrix(targets []buildcost.Target, books map[string]*buildcost.Book, stations []StationMeta, names, categories map[string]string, listings map[string]map[string]float64, catalogPrice map[string]int) Matrix`

Margin note: the landing matrix shows one margin per cell tied to the **BoM** cost (BoM is the always-available mode; Recipe savings/profit live on the detail page). `SavingsBoM`/`ProfitBoM` are computed from the cell margin against BoM cost.

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-build-costs/matrix_test.go`:

```go
package main

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func TestBuildMatrix_CheapestAndFeasibleCount(t *testing.T) {
	targets := []buildcost.Target{{
		ID: "widget", Kind: "item",
		BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 2}},
	}}
	books := map[string]*buildcost.Book{
		"st1": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 100}}}, BestBuy: map[string]float64{}},
		"st2": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 4, Qty: 100}}}, BestBuy: map[string]float64{}},
		"st3": {Sell: map[string]buildcost.Ladder{}, BestBuy: map[string]float64{}}, // infeasible
	}
	stations := []StationMeta{{ID: "st1"}, {ID: "st2"}, {ID: "st3"}}
	m := BuildMatrix(targets, books, stations, map[string]string{"widget": "Widget"},
		map[string]string{"widget": "Module"}, nil, nil)
	if len(m.Rows) != 1 {
		t.Fatalf("rows: %d", len(m.Rows))
	}
	r := m.Rows[0]
	if r.CheapestStation != "st2" || !approx(r.CheapestCost, 8) {
		t.Fatalf("cheapest: %s %v", r.CheapestStation, r.CheapestCost)
	}
	if r.FeasibleCount != 2 {
		t.Fatalf("feasible count: %d want 2", r.FeasibleCount)
	}
	if r.Name != "Widget" || r.Category != "Module" {
		t.Fatalf("meta: %+v", r)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run TestBuildMatrix -v`
Expected: FAIL — `undefined: BuildMatrix`.

- [ ] **Step 3: Write minimal implementation**

Create `cmd/generate-build-costs/matrix.go`:

```go
package main

import (
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// RowCell is the render-ready per-station result for one target.
type RowCell struct {
	BoMCost        float64
	BoMFeasible    bool
	BoMCovered     int
	BoMTotal       int
	RecipeCost     float64
	RecipeFeasible bool
	RecipeNA       bool
	RecipeID       string
	SavingsBoM     float64
	HasSavings     bool
	ProfitBoM      float64
	HasProfit      bool
}

// MatrixRow is one item/ship across all stations, plus summary columns.
type MatrixRow struct {
	ID              string
	Name            string
	Kind            string
	Category        string
	Cells           map[string]RowCell
	CheapestStation string
	CheapestCost    float64
	FeasibleCount   int
}

// Matrix is the full render model: station columns and target rows.
type Matrix struct {
	Stations []StationMeta
	Rows     []MatrixRow
}

// BuildMatrix computes every target×station cell and the per-row summaries.
func BuildMatrix(targets []buildcost.Target, books map[string]*buildcost.Book, stations []StationMeta,
	names, categories map[string]string, listings map[string]map[string]float64, catalogPrice map[string]int) Matrix {
	m := Matrix{Stations: stations}
	for _, t := range targets {
		row := MatrixRow{ID: t.ID, Name: names[t.ID], Kind: t.Kind, Category: categories[t.ID], Cells: map[string]RowCell{}}
		if row.Name == "" {
			row.Name = t.ID
		}
		haveCheapest := false
		for _, st := range stations {
			book := books[st.ID]
			if book == nil {
				continue
			}
			var margin buildcost.Margin
			if t.Kind == "ship" {
				margin = shipMargin(listings, catalogPrice, t.ID, st.ID)
			} else {
				margin = itemMargin(book, t.ID)
			}
			c := buildcost.BuildCell(t, st.ID, book, margin)
			rc := RowCell{
				BoMCost: c.BoM.Cost, BoMFeasible: c.BoM.Feasible,
				BoMCovered: c.BoM.Covered, BoMTotal: c.BoM.Total,
				RecipeCost: c.Recipe.Cost, RecipeFeasible: c.Recipe.Feasible,
				RecipeNA: c.Recipe.NA, RecipeID: c.Recipe.RecipeID,
			}
			if s, ok := c.Margin.SavingsVsAsk(c.BoM.Cost); ok {
				rc.SavingsBoM, rc.HasSavings = s, true
			}
			if p, ok := c.Margin.ProfitVsBid(c.BoM.Cost); ok {
				rc.ProfitBoM, rc.HasProfit = p, true
			}
			row.Cells[st.ID] = rc
			if c.BoM.Feasible {
				row.FeasibleCount++
				if !haveCheapest || c.BoM.Cost < row.CheapestCost {
					row.CheapestStation, row.CheapestCost, haveCheapest = st.ID, c.BoM.Cost, true
				}
			}
		}
		m.Rows = append(m.Rows, row)
	}
	sort.Slice(m.Rows, func(i, j int) bool { return m.Rows[i].Name < m.Rows[j].Name })
	return m
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run TestBuildMatrix -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/matrix.go cmd/generate-build-costs/matrix_test.go
git commit -m "feat(generate-build-costs): compute full cost matrix model"
```

---

### Task 8: Render the landing matrix page

**Files:**
- Create: `cmd/generate-build-costs/render.go`
- Create: `cmd/generate-build-costs/templates/index.html.tmpl`
- Test: `cmd/generate-build-costs/render_test.go`

**Interfaces:**
- Consumes: `Matrix` (Task 7).
- Produces:
  - `func renderIndex(outDir string, m Matrix) error` — writes `outDir/index.html` with the matrix data embedded as JSON and vanilla-JS filters (Category, substring, Station, Empire, Show-only-feasible default on) + BoM/Recipe + metric toggles.
  - `func matrixJSON(m Matrix) (string, error)` — serializes a compact client model.

The JS is embedded verbatim in the template. Filtering/sorting happen client-side over the embedded JSON. Keep the payload compact: emit arrays, not verbose objects.

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-build-costs/render_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func sampleMatrix() Matrix {
	targets := []buildcost.Target{{ID: "widget", Kind: "item", BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 2}}}}
	books := map[string]*buildcost.Book{"st1": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 100}}}, BestBuy: map[string]float64{}}}
	stations := []StationMeta{{ID: "st1", Name: "Station One", Empire: "Sol"}}
	return BuildMatrix(targets, books, stations, map[string]string{"widget": "Widget"}, map[string]string{"widget": "Module"}, nil, nil)
}

func TestRenderIndex_WritesFileWithData(t *testing.T) {
	dir := t.TempDir()
	if err := renderIndex(dir, sampleMatrix()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Widget", "Station One", "Show only feasible", "BoM", "Recipe"} {
		if !strings.Contains(s, want) {
			t.Fatalf("index.html missing %q", want)
		}
	}
}

func TestMatrixJSON_Valid(t *testing.T) {
	js, err := matrixJSON(sampleMatrix())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, "widget") {
		t.Fatalf("json missing target id: %s", js)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run 'TestRenderIndex|TestMatrixJSON' -v`
Expected: FAIL — `undefined: renderIndex`.

- [ ] **Step 3: Write minimal implementation**

Create `cmd/generate-build-costs/templates/index.html.tmpl`. The Go `render.go` embeds this via `embed`. It renders the KB chrome and a `<script>` block holding the JSON and the client logic. Full template:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Build Cost Matrix — Spacemolt KB</title>
<style>
 body{font-family:system-ui,sans-serif;margin:0;padding:1rem;background:#0d1117;color:#c9d1d9}
 h1{font-size:1.4rem} .controls{display:flex;flex-wrap:wrap;gap:.6rem;align-items:center;margin:.6rem 0}
 .controls input,.controls select{background:#161b22;color:#c9d1d9;border:1px solid #30363d;border-radius:4px;padding:.3rem}
 table{border-collapse:collapse;font-size:.8rem} th,td{border:1px solid #21262d;padding:.25rem .4rem;text-align:right;white-space:nowrap}
 th.rowhdr,td.rowhdr{text-align:left;position:sticky;left:0;background:#0d1117;z-index:1}
 thead th{position:sticky;top:0;background:#161b22}
 td.infeasible{color:#6e7681} td.cheapest{outline:2px solid #2ea043}
 td.pos{color:#3fb950} td.neg{color:#f85149} a{color:#58a6ff}
 .wrap{overflow:auto;max-height:80vh}
</style>
</head>
<body>
<h1>Build Cost Matrix</h1>
<p>Live-market build cost of every item and ship at every station. BoM = from raw ore; Recipe = buy sub-assemblies. Muted = can't complete from current depth.</p>
<div class="controls">
 <label>Mode <select id="mode"><option value="bom">BoM</option><option value="recipe">Recipe</option></select></label>
 <label>Metric <select id="metric"><option value="cost">Cost</option><option value="savings">Savings</option><option value="profit">Profit</option></select></label>
 <label>Category <select id="category"></select></label>
 <label>Empire <select id="empire"></select></label>
 <label>Station <select id="station"></select></label>
 <input id="search" placeholder="search name…">
 <label><input type="checkbox" id="feasible" checked> Show only feasible</label>
</div>
<div class="wrap"><table id="matrix"><thead></thead><tbody></tbody></table></div>
<script id="data" type="application/json">{{.JSON}}</script>
<script>
const M = JSON.parse(document.getElementById('data').textContent);
const $ = id => document.getElementById(id);
function fmt(n){ return n==null? '' : Math.round(n).toLocaleString(); }
function initFilter(sel, vals){ sel.innerHTML='<option value="">All</option>'+[...new Set(vals)].filter(Boolean).sort().map(v=>`<option>${v}</option>`).join(''); }
initFilter($('category'), M.rows.map(r=>r.cat));
initFilter($('empire'), M.stations.map(s=>s.empire));
initFilter($('station'), M.stations.map(s=>s.name));
function cellVal(c, mode, metric){
  if(!c) return {v:null, feas:false, na:false};
  if(mode==='recipe' && c.rna) return {v:null, feas:false, na:true};
  const cost = mode==='bom'? c.bc : c.rc;
  const feas = mode==='bom'? c.bf : c.rf;
  if(metric==='cost') return {v:cost, feas, na:false};
  if(metric==='savings') return {v:c.hs? c.sv : null, feas, na:false};
  return {v:c.hp? c.pf : null, feas, na:false};
}
function render(){
  const mode=$('mode').value, metric=$('metric').value;
  const cat=$('category').value, emp=$('empire').value, stn=$('station').value;
  const q=$('search').value.toLowerCase(), onlyFeas=$('feasible').checked;
  const cols = M.stations.filter(s=> (!emp||s.empire===emp) && (!stn||s.name===stn));
  let head='<tr><th class="rowhdr">Item / Ship</th><th>Cheapest</th>';
  head += cols.map(s=>`<th title="${s.empire}">${s.name}</th>`).join('')+'</tr>';
  $('matrix').tHead.innerHTML=head;
  const rowsHtml=[];
  for(const r of M.rows){
    if(cat && r.cat!==cat) continue;
    if(q && !r.name.toLowerCase().includes(q)) continue;
    if(onlyFeas && r.fc===0) continue;
    let tds=`<td class="rowhdr"><a href="./${r.id}.html">${r.name}</a> <small>${r.kind}</small></td>`;
    tds+=`<td>${r.cs? fmt(r.cc)+' @'+ (M.stations.find(s=>s.id===r.cs)||{}).name : '—'}</td>`;
    for(const s of cols){
      const cv=cellVal(r.cells[s.id], mode, metric);
      let cls=[]; if(!cv.feas) cls.push('infeasible'); if(r.cs===s.id && metric==='cost' && mode==='bom') cls.push('cheapest');
      if(metric!=='cost' && cv.v!=null) cls.push(cv.v>=0?'pos':'neg');
      const txt = cv.na? 'n/a' : (cv.v==null? '' : fmt(cv.v));
      tds+=`<td class="${cls.join(' ')}">${txt}</td>`;
    }
    rowsHtml.push('<tr>'+tds+'</tr>');
  }
  $('matrix').tBodies[0].innerHTML=rowsHtml.join('');
}
['mode','metric','category','empire','station','feasible'].forEach(id=>$(id).addEventListener('change',render));
$('search').addEventListener('input',render);
render();
</script>
</body>
</html>
```

Create `cmd/generate-build-costs/render.go`:

```go
package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
)

//go:embed templates/*.tmpl
var tmplFS embed.FS

// clientCell is the compact per-station cell in the embedded JSON.
type clientCell struct {
	BC float64 `json:"bc"`           // BoM cost
	BF bool    `json:"bf"`           // BoM feasible
	RC float64 `json:"rc"`           // Recipe cost
	RF bool    `json:"rf"`           // Recipe feasible
	RNA bool   `json:"rna"`          // Recipe NA
	SV float64 `json:"sv"`           // savings (BoM)
	HS bool    `json:"hs"`           // has savings
	PF float64 `json:"pf"`           // profit (BoM)
	HP bool    `json:"hp"`           // has profit
}

type clientRow struct {
	ID    string                `json:"id"`
	Name  string                `json:"name"`
	Kind  string                `json:"kind"`
	Cat   string                `json:"cat"`
	CS    string                `json:"cs"` // cheapest station id
	CC    float64               `json:"cc"` // cheapest cost
	FC    int                   `json:"fc"` // feasible count
	Cells map[string]clientCell `json:"cells"`
}

type clientStation struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Empire string `json:"empire"`
}

type clientModel struct {
	Stations []clientStation `json:"stations"`
	Rows     []clientRow     `json:"rows"`
}

func toClientModel(m Matrix) clientModel {
	cm := clientModel{}
	for _, s := range m.Stations {
		cm.Stations = append(cm.Stations, clientStation{ID: s.ID, Name: s.Name, Empire: s.Empire})
	}
	for _, r := range m.Rows {
		cr := clientRow{ID: r.ID, Name: r.Name, Kind: r.Kind, Cat: r.Category, CS: r.CheapestStation, CC: r.CheapestCost, FC: r.FeasibleCount, Cells: map[string]clientCell{}}
		for st, c := range r.Cells {
			cr.Cells[st] = clientCell{BC: c.BoMCost, BF: c.BoMFeasible, RC: c.RecipeCost, RF: c.RecipeFeasible, RNA: c.RecipeNA, SV: c.SavingsBoM, HS: c.HasSavings, PF: c.ProfitBoM, HP: c.HasProfit}
		}
		cm.Rows = append(cm.Rows, cr)
	}
	return cm
}

// matrixJSON serializes the compact client model.
func matrixJSON(m Matrix) (string, error) {
	b, err := json.Marshal(toClientModel(m))
	return string(b), err
}

// renderIndex writes the landing matrix page.
func renderIndex(outDir string, m Matrix) error {
	js, err := matrixJSON(m)
	if err != nil {
		return err
	}
	t, err := template.ParseFS(tmplFS, "templates/index.html.tmpl")
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
	defer f.Close()
	return t.Execute(f, map[string]any{"JSON": template.JS(js)})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run 'TestRenderIndex|TestMatrixJSON' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/render.go cmd/generate-build-costs/render_test.go cmd/generate-build-costs/templates/index.html.tmpl
git commit -m "feat(generate-build-costs): render filterable landing matrix"
```

---

### Task 9: Render per-target detail pages

**Files:**
- Modify: `cmd/generate-build-costs/render.go` (add `renderDetail`)
- Create: `cmd/generate-build-costs/templates/detail.html.tmpl`
- Test: `cmd/generate-build-costs/render_test.go` (add a detail test)

**Interfaces:**
- Consumes: `MatrixRow`, `StationMeta` (Tasks 6–7).
- Produces:
  - `func renderDetail(outDir string, row MatrixRow, stations []StationMeta) error` — writes `outDir/<id>.html` with the full per-station breakdown table (BoM cost, BoM feasible x/N, Recipe cost, Recipe feasible + chosen recipe, savings, profit) and a link back to the KB item/ship page.

- [ ] **Step 1: Write the failing test**

Add to `cmd/generate-build-costs/render_test.go`:

```go
func TestRenderDetail_WritesTable(t *testing.T) {
	dir := t.TempDir()
	m := sampleMatrix()
	if err := renderDetail(dir, m.Rows[0], m.Stations); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "widget.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Widget", "Station One", "BoM", "Recipe", "Feasible"} {
		if !strings.Contains(s, want) {
			t.Fatalf("detail missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run TestRenderDetail -v`
Expected: FAIL — `undefined: renderDetail`.

- [ ] **Step 3: Write minimal implementation**

Create `cmd/generate-build-costs/templates/detail.html.tmpl`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Name}} — Build Cost — Spacemolt KB</title>
<style>
 body{font-family:system-ui,sans-serif;margin:0;padding:1rem;background:#0d1117;color:#c9d1d9}
 table{border-collapse:collapse;font-size:.85rem;margin-top:1rem} th,td{border:1px solid #21262d;padding:.3rem .5rem;text-align:right}
 th:first-child,td:first-child{text-align:left} .infeasible{color:#6e7681} .pos{color:#3fb950} .neg{color:#f85149} a{color:#58a6ff}
</style>
</head>
<body>
<p><a href="../build-costs/">← Build Cost Matrix</a></p>
<h1>{{.Name}} <small>({{.Kind}})</small></h1>
<p>Build cost by station. BoM = from raw ore; Recipe = buy sub-assemblies.</p>
<table>
<thead><tr><th>Station</th><th>Empire</th><th>BoM cost</th><th>BoM feasible</th><th>Recipe cost</th><th>Recipe feasible</th><th>Recipe</th><th>Savings</th><th>Profit</th></tr></thead>
<tbody>
{{range .Lines}}
<tr>
 <td>{{.StationName}}</td><td>{{.Empire}}</td>
 <td class="{{if not .BoMFeasible}}infeasible{{end}}">{{.BoMCostStr}}</td>
 <td>{{.BoMCovered}}/{{.BoMTotal}}</td>
 <td class="{{if .RecipeNA}}infeasible{{else if not .RecipeFeasible}}infeasible{{end}}">{{.RecipeCostStr}}</td>
 <td>{{.RecipeFeasibleStr}}</td>
 <td>{{.RecipeID}}</td>
 <td class="{{.SavingsClass}}">{{.SavingsStr}}</td>
 <td class="{{.ProfitClass}}">{{.ProfitStr}}</td>
</tr>
{{end}}
</tbody>
</table>
</body>
</html>
```

Append to `cmd/generate-build-costs/render.go`:

```go
type detailLine struct {
	StationName, Empire            string
	BoMFeasible                    bool
	BoMCostStr                     string
	BoMCovered, BoMTotal           int
	RecipeNA, RecipeFeasible       bool
	RecipeCostStr, RecipeFeasibleStr, RecipeID string
	SavingsStr, SavingsClass       string
	ProfitStr, ProfitClass         string
}

func money(v float64, ok bool) string {
	if !ok {
		return "—"
	}
	return template.HTMLEscapeString(commaInt(v))
}

func signClass(v float64, ok bool) string {
	if !ok {
		return ""
	}
	if v >= 0 {
		return "pos"
	}
	return "neg"
}

// renderDetail writes the per-target station breakdown page.
func renderDetail(outDir string, row MatrixRow, stations []StationMeta) error {
	t, err := template.ParseFS(tmplFS, "templates/detail.html.tmpl")
	if err != nil {
		return err
	}
	var lines []detailLine
	for _, s := range stations {
		c, ok := row.Cells[s.ID]
		if !ok {
			continue
		}
		ln := detailLine{
			StationName: s.Name, Empire: s.Empire,
			BoMFeasible: c.BoMFeasible, BoMCostStr: commaInt(c.BoMCost),
			BoMCovered: c.BoMCovered, BoMTotal: c.BoMTotal,
			RecipeNA: c.RecipeNA, RecipeFeasible: c.RecipeFeasible, RecipeID: c.RecipeID,
			SavingsStr: money(c.SavingsBoM, c.HasSavings), SavingsClass: signClass(c.SavingsBoM, c.HasSavings),
			ProfitStr: money(c.ProfitBoM, c.HasProfit), ProfitClass: signClass(c.ProfitBoM, c.HasProfit),
		}
		if c.RecipeNA {
			ln.RecipeCostStr, ln.RecipeFeasibleStr = "n/a", "sub-assemblies not traded"
		} else {
			ln.RecipeCostStr = commaInt(c.RecipeCost)
			if c.RecipeFeasible {
				ln.RecipeFeasibleStr = "yes"
			} else {
				ln.RecipeFeasibleStr = "no"
			}
		}
		lines = append(lines, ln)
	}
	f, err := os.Create(filepath.Join(outDir, row.ID+".html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, map[string]any{"Name": row.Name, "Kind": row.Kind, "Lines": lines})
}
```

Add the `commaInt` helper to `render.go` (thousands-separated integer string):

```go
func commaInt(v float64) string {
	n := int64(v + 0.5)
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
```

Add `"strconv"` to the `render.go` import block.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run TestRenderDetail -v` then `go test ./cmd/generate-build-costs/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-build-costs/render.go cmd/generate-build-costs/render_test.go cmd/generate-build-costs/templates/detail.html.tmpl
git commit -m "feat(generate-build-costs): render per-target detail pages"
```

---

### Task 10: Wire up `main.go`, run end-to-end, document

**Files:**
- Create: `cmd/generate-build-costs/main.go`
- Modify: `docs/superpowers/plans/` — none; update the KB regen runbook note (see Step 6).

**Interfaces:**
- Consumes: all loaders/renderers (Tasks 6–9).
- Produces: the `main` entrypoint and the generated site under `kb/build-costs/`.

- [ ] **Step 1: Write `main.go`**

Create `cmd/generate-build-costs/main.go`:

```go
// Command generate-build-costs renders the KB build-cost matrix: the live-market
// cost, feasibility, and margin of building every item and ship at every station.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func openRO(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}

func main() {
	craftPath := flag.String("crafting", "../../spacemolt-crafting-server/database/crafting.db", "crafting DB")
	marketPath := flag.String("market", "../spacemolt/data/market.db", "market DB")
	knowledgePath := flag.String("knowledge", "../spacemolt-knowledge.db", "knowledge DB")
	catalogRoot := flag.String("catalog", "../spacemolt/data/game-api", "game-api catalog root")
	outDir := flag.String("out", "kb/build-costs", "output directory")
	flag.Parse()

	craftDB, err := openRO(*craftPath)
	must(err, "open crafting")
	defer craftDB.Close()
	marketDB, err := openRO(*marketPath)
	must(err, "open market")
	defer marketDB.Close()
	knowledgeDB, err := openRO(*knowledgePath)
	must(err, "open knowledge")
	defer knowledgeDB.Close()

	ships, catalogPrice, err := loadShipCatalog(*catalogRoot)
	must(err, "load ships")
	itemNames, categories, err := loadItemMeta(craftDB)
	must(err, "load item meta")

	books, err := loadBooks(marketDB)
	must(err, "load books")
	stations, err := loadStations(marketDB, knowledgeDB)
	must(err, "load stations")
	listings, err := loadShipListings(knowledgeDB)
	must(err, "load ship listings")
	targets, names, err := loadTargets(craftDB, ships, itemNames)
	must(err, "load targets")

	// Ships also contribute their category for the filter.
	for _, s := range ships {
		if categories[s.ID] == "" {
			categories[s.ID] = s.Class
		}
	}
	// Merge display names (item names + ship names) for rows.
	for id, n := range names {
		if itemNames[id] == "" {
			itemNames[id] = n
		}
	}

	m := BuildMatrix(targets, books, stations, names, categories, listings, catalogPrice)

	must(renderIndex(*outDir, m), "render index")
	for _, row := range m.Rows {
		must(renderDetail(*outDir, row, stations), "render detail "+row.ID)
	}
	log.Printf("build-costs: %d rows × %d stations → %s", len(m.Rows), len(stations), *outDir)
}

func must(err error, ctx string) {
	if err != nil {
		log.Fatalf("%s: %v", ctx, err)
	}
}

// loadItemMeta returns id->name and id->category for catalog items in crafting.db.
func loadItemMeta(craftDB *sql.DB) (names, categories map[string]string, err error) {
	names, categories = map[string]string{}, map[string]string{}
	rows, err := craftDB.Query(`SELECT id, name, COALESCE(category,'') FROM items`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, cat string
		if err := rows.Scan(&id, &name, &cat); err != nil {
			return nil, nil, err
		}
		names[id], categories[id] = name, cat
	}
	return names, categories, rows.Err()
}

// loadShipCatalog reads the latest catalog_ships.json, returning the trimmed ship
// list and an id->price map.
func loadShipCatalog(root string) ([]Ship, map[string]int, error) {
	dir, err := findLatestCatalogDir(root)
	if err != nil {
		return nil, nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "catalog_ships.json"))
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Items []Ship `json:"items"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	price := map[string]int{}
	for _, s := range doc.Items {
		price[s.ID] = s.Price
	}
	return doc.Items, price, nil
}

// findLatestCatalogDir returns the most recently modified subdirectory of root.
func findLatestCatalogDir(root string) (string, error) {
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
```

- [ ] **Step 2: Build and vet**

Run: `go build ./... && go vet ./cmd/generate-build-costs/`
Expected: no errors. Fix any unused/missing imports (`names` vs `itemNames` merge is intentional; confirm `BuildMatrix` is called with `names` for display).

- [ ] **Step 3: Run the generator end-to-end**

Run (from `kb/`):

```bash
mkdir -p bin
go run ./cmd/generate-build-costs
```

Expected: log line like `build-costs: NNN rows × MM stations → kb/build-costs`, and `kb/build-costs/index.html` plus per-target `<id>.html` files exist. Spot-check:

```bash
ls kb/build-costs/ | head
grep -c 'class="rowhdr"' kb/build-costs/index.html   # header present
```

- [ ] **Step 4: Verify in a browser (visual check)**

Open `kb/build-costs/index.html`. Confirm: filters populate (Category/Empire/Station), Show-only-feasible is checked by default, BoM↔Recipe and metric toggles change the numbers, muted cells for infeasible, cheapest highlighted, row links open detail pages. Fix any JS/template issues and re-run.

- [ ] **Step 5: Full test + lint**

Run: `go test ./... && golangci-lint run ./pkg/buildcost/... ./cmd/generate-build-costs/...`
Expected: all pass, no new lint findings.

- [ ] **Step 6: Commit generated site + runbook note**

Update the KB regen runbook (memory `project_kb_regeneration_runbook.md`) with a new generator entry: `go run ./cmd/generate-build-costs` → `kb/build-costs/` (sources: crafting.db BOM/recipes, market.db order book, knowledge.db ship_listings + systems.empire, ships catalog). Then:

```bash
git add cmd/generate-build-costs kb/build-costs
git commit -m "feat: build-cost matrix KB page (generate-build-costs)"
```

Add a link to the new page from the KB landing/nav where the other generated sections are linked (grep the site index in `cmd/generate-items-kb` for how sections are listed; add a "Build Costs" entry pointing to `build-costs/`). Commit that separately if it touches `generate-items-kb`.

---

## Self-Review

**Spec coverage:**
- Reference lookup (both modes) → Tasks 2, 3, 7, 8, 9. ✓
- Build-vs-buy margin (ask+bid, ship listing/catalog) → Tasks 4, 6, 7, 9. ✓
- Feasibility + partial cost + shortfall → Tasks 2, 5, 7, 9. ✓
- Depth walk, independent runs, sell-side, latest snapshot → Tasks 1, 6. ✓
- BoM for items+ships; Recipe items via recipe_inputs; ship Recipe NA → Tasks 5, 6. ✓
- Multi-recipe cheapest-feasible + recipe named → Tasks 3, 9. ✓
- Hybrid layout, filters (Category/search/Station/Empire/Show-only-feasible), toggles → Task 8. ✓
- `pkg/buildcost` pure/tested + `cmd/generate-build-costs`, output `kb/build-costs/` → all. ✓
- 6h TTL / no staleness handling → honored (latest snapshot only; no greying). ✓
- Future +N-hop: not built; the `Book`/`BuildCell` API takes a single station and is unchanged-compatible with a future station-set variant. ✓

**Deferred to execution (flagged, not placeholders):**
- Per-material walk *expandable* detail (spec §6) is described but Task 9 renders the summary table only; the ladder-tier expansion is a follow-up enhancement — note it in the detail page as a TODO comment, do not block the page. *(If desired as part of MVP, add a Task 9b rendering the walk per station; the data is available from `Book.Walk`.)*
- KB nav link wiring (Task 10 Step 6) depends on the existing index structure; the executing engineer greps `generate-items-kb` for the pattern.

**Placeholder scan:** No TBD/TODO in code steps; every code step has complete code. The one explicit follow-up (walk expansion) is called out above, not left as an in-code placeholder.

**Type consistency:** `ModeResult`, `Cell`, `Target`, `RowCell`, `MatrixRow`, `Matrix`, `clientCell/clientRow` field names are consistent across Tasks 1–9; JSON tags (`bc/bf/rc/rf/rna/sv/hs/pf/hp`) match between `render.go` and the template JS.
