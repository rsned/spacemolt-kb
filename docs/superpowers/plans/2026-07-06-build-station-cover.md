# Build Station-Cover Did-You-Know Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a did-you-know page reporting, for every item and ship, the minimum number of distinct market stations needed to source all its parts (depth-aware set-cover), with factoids, CSS bar-chart distributions, a sortable main table, and a separate table of targets unbuildable anywhere.

**Architecture:** A pure `pkg/buildcost.MinStationCover` set-cover function (exact for small covers, greedy fallback) plus generator glue in `cmd/generate-build-costs` that reuses the existing data load (BoM, recipes, capped books, 29 stations), computes per-target covers + aggregate stats, and renders a self-contained static HTML page into `kb/did_you_know/`.

**Tech Stack:** Go 1.24+, `modernc.org/sqlite` (already wired), `html/template` with `//go:embed`, pure-CSS bar charts (no JS library).

## Global Constraints

- Go 1.24+; use `range over int` and `b.Loop()` where applicable; new code must pass `golangci-lint` with zero new findings.
- `pkg/buildcost` stays **pure**: no DB, no HTML, no I/O. The cover algorithm lives there; all DB/HTML glue lives in `cmd/generate-build-costs` (package main).
- Determinism: station ids sorted; the first lexicographic covering combination is returned; ties break by id — so the committed generated page diffs cleanly across regenerations.
- Metric is **count of distinct stations only** (no travel/jump distance). Depth basis is **one build**; a **×10 batch** pass flags targets whose count increases.
- "Unbuildable" is keyed off **BoM** feasibility. Recipe = N/A never demotes a target.
- `exactK = 3` in the generator (proven minimum up to 3 stations, greedy upper bound beyond, flagged as `Exact=false`).
- Reuse existing symbols verbatim: `buildcost.Requirement{ItemID string; Qty float64}`, `buildcost.Target{ID, Kind string; BoM []Requirement; Recipes []Recipe; RecipeNA string}`, `buildcost.Recipe{ID string; OutputQty float64; Inputs []Requirement}`, `buildcost.Book{Sell map[string]Ladder; BestBuy map[string]float64}`, `buildcost.Ladder = []Order`, `buildcost.Order{Price, Qty float64}`, `StationMeta{ID, Name, System, Empire string}`.
- The generator must **not** edit `kb/did_you_know/index.html` (hand-authored). The index card is a one-time manual edit (Task 5).
- Regenerate with explicit DB paths (defaults in main.go are stale):
  `go run ./cmd/generate-build-costs -crafting /home/robert/spacemolt/spacemolt/data/crafting.db -market /home/robert/spacemolt/spacemolt/data/market.db -knowledge /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db -catalog /home/robert/spacemolt/spacemolt/data/game-api -out kb/build-costs`

---

## File Structure

- `pkg/buildcost/stationcover.go` (new) — `StationDepth`, `CoverResult`, `MinStationCover`. Pure.
- `pkg/buildcost/stationcover_test.go` (new) — pure algorithm tests.
- `cmd/generate-build-costs/stationcover.go` (new) — depth-map builder, per-target cover computation, page model + aggregate stats. Package main.
- `cmd/generate-build-costs/stationcover_render.go` (new) — `renderStationCover` (template execution).
- `cmd/generate-build-costs/stationcover_test.go` (new) — generator glue + render tests.
- `cmd/generate-build-costs/templates/stationcover.html.tmpl` (new) — the page template (picked up by the existing `//go:embed templates/*.tmpl`).
- `cmd/generate-build-costs/main.go` (modify) — add `-station-cover-out` flag; after the matrix render, build the depth map, compute the page model, and render it.
- `kb/did_you_know/index.html` (modify, Task 5 — hand edit) — add one factoid-grid card linking to the new page.

---

## Task 1: Pure `MinStationCover` in `pkg/buildcost`

**Files:**
- Create: `pkg/buildcost/stationcover.go`
- Test: `pkg/buildcost/stationcover_test.go`

**Interfaces:**
- Consumes: `buildcost.Requirement{ItemID string; Qty float64}` (existing, `pkg/buildcost/cost.go`).
- Produces:
  - `type StationDepth map[string]map[string]float64`
  - `type CoverResult struct { Feasible bool; Count int; Stations []string; Exact bool; Missing []string }`
  - `func MinStationCover(reqs []Requirement, depth StationDepth, stationIDs []string, exactK int) CoverResult`

- [ ] **Step 1: Write the failing tests**

Create `pkg/buildcost/stationcover_test.go`:

```go
package buildcost

import (
	"reflect"
	"testing"
)

func TestMinStationCover_SingleStationCoversAll(t *testing.T) {
	reqs := []Requirement{{ItemID: "iron", Qty: 5}, {ItemID: "copper", Qty: 3}}
	depth := StationDepth{
		"A": {"iron": 10, "copper": 10},
		"B": {"iron": 1},
	}
	got := MinStationCover(reqs, depth, []string{"A", "B"}, 3)
	if !got.Feasible || got.Count != 1 || !got.Exact {
		t.Fatalf("got %+v, want feasible count=1 exact", got)
	}
	if !reflect.DeepEqual(got.Stations, []string{"A"}) {
		t.Fatalf("stations = %v, want [A]", got.Stations)
	}
}

func TestMinStationCover_NeedsTwoStations(t *testing.T) {
	// iron only fully covered by A+B together; copper only by C... but B has copper too.
	reqs := []Requirement{{ItemID: "iron", Qty: 10}, {ItemID: "copper", Qty: 4}}
	depth := StationDepth{
		"A": {"iron": 6},
		"B": {"iron": 6, "copper": 4},
	}
	got := MinStationCover(reqs, depth, []string{"A", "B"}, 3)
	if !got.Feasible || got.Count != 2 || !got.Exact {
		t.Fatalf("got %+v, want feasible count=2 exact", got)
	}
	if !reflect.DeepEqual(got.Stations, []string{"A", "B"}) {
		t.Fatalf("stations = %v, want [A B]", got.Stations)
	}
}

func TestMinStationCover_InfeasibleListsMissing(t *testing.T) {
	reqs := []Requirement{{ItemID: "iron", Qty: 10}, {ItemID: "unobtainium", Qty: 1}}
	depth := StationDepth{"A": {"iron": 10}}
	got := MinStationCover(reqs, depth, []string{"A"}, 3)
	if got.Feasible {
		t.Fatalf("expected infeasible, got %+v", got)
	}
	if !reflect.DeepEqual(got.Missing, []string{"unobtainium"}) {
		t.Fatalf("missing = %v, want [unobtainium]", got.Missing)
	}
}

func TestMinStationCover_GreedyFallbackBeyondExactK(t *testing.T) {
	// Four inputs, each only at its own station with exactly enough depth →
	// the unique cover is 4 stations. With exactK=2 the exact search cannot
	// find it, so greedy returns a valid 4-station (non-exact) cover.
	reqs := []Requirement{
		{ItemID: "a", Qty: 1}, {ItemID: "b", Qty: 1},
		{ItemID: "c", Qty: 1}, {ItemID: "d", Qty: 1},
	}
	depth := StationDepth{
		"S1": {"a": 1}, "S2": {"b": 1}, "S3": {"c": 1}, "S4": {"d": 1},
	}
	got := MinStationCover(reqs, depth, []string{"S1", "S2", "S3", "S4"}, 2)
	if !got.Feasible || got.Count != 4 || got.Exact {
		t.Fatalf("got %+v, want feasible count=4 non-exact", got)
	}
	if !reflect.DeepEqual(got.Stations, []string{"S1", "S2", "S3", "S4"}) {
		t.Fatalf("stations = %v, want all four", got.Stations)
	}
}

func TestMinStationCover_DeterministicFirstCombo(t *testing.T) {
	// A alone and B alone both cover → the lexicographically first (A) wins.
	reqs := []Requirement{{ItemID: "iron", Qty: 1}}
	depth := StationDepth{"B": {"iron": 5}, "A": {"iron": 5}}
	got := MinStationCover(reqs, depth, []string{"A", "B"}, 3)
	if got.Count != 1 || !reflect.DeepEqual(got.Stations, []string{"A"}) {
		t.Fatalf("got %+v, want count=1 station [A]", got)
	}
}

func TestMinStationCover_BatchIncreasesCount(t *testing.T) {
	// At qty 1, A alone covers iron. At qty 12, A(10)+B(5) needed.
	depth := StationDepth{"A": {"iron": 10}, "B": {"iron": 5}}
	one := MinStationCover([]Requirement{{ItemID: "iron", Qty: 1}}, depth, []string{"A", "B"}, 3)
	ten := MinStationCover([]Requirement{{ItemID: "iron", Qty: 12}}, depth, []string{"A", "B"}, 3)
	if one.Count != 1 || ten.Count != 2 {
		t.Fatalf("one=%d ten=%d, want 1 and 2", one.Count, ten.Count)
	}
}

func TestMinStationCover_EmptyReqs(t *testing.T) {
	got := MinStationCover(nil, StationDepth{"A": {}}, []string{"A"}, 3)
	if !got.Feasible || got.Count != 0 || !got.Exact {
		t.Fatalf("got %+v, want feasible count=0 exact", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/buildcost/ -run TestMinStationCover -v`
Expected: FAIL — `undefined: MinStationCover` (and `StationDepth`, `CoverResult`).

- [ ] **Step 3: Implement `MinStationCover`**

Create `pkg/buildcost/stationcover.go`:

```go
package buildcost

import "sort"

// StationDepth maps station id -> (item id -> total sell depth available there).
type StationDepth map[string]map[string]float64

// CoverResult is the outcome of a minimum-station cover search.
// Feasible is false when some input's total depth across all stations is below
// the required quantity; Missing then lists those inputs (sorted). When
// Feasible, Count is the number of stations in the returned cover (Stations,
// sorted) and Exact reports whether that count is a proven minimum (true) or a
// greedy upper bound (false, for covers deeper than exactK).
type CoverResult struct {
	Feasible bool
	Count    int
	Stations []string
	Exact    bool
	Missing  []string
}

// MinStationCover finds the fewest stations whose pooled sell depth covers every
// requirement. It proves the minimum for covers up to exactK stations by
// exhaustive search over station combinations (in lexicographic order of sorted
// ids), then falls back to a greedy upper bound. Deterministic.
func MinStationCover(reqs []Requirement, depth StationDepth, stationIDs []string, exactK int) CoverResult {
	// Aggregate required quantity per input (summing duplicates).
	need := map[string]float64{}
	for _, r := range reqs {
		need[r.ItemID] += r.Qty
	}
	if len(need) == 0 {
		return CoverResult{Feasible: true, Count: 0, Stations: []string{}, Exact: true}
	}

	ids := append([]string(nil), stationIDs...)
	sort.Strings(ids)

	// Feasibility gate: any input whose total depth < need is unmeetable.
	var missing []string
	for item, q := range need {
		var total float64
		for _, s := range ids {
			total += depth[s][item]
		}
		if total < q {
			missing = append(missing, item)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return CoverResult{Feasible: false, Missing: missing}
	}

	// covers reports whether the given station subset meets every need.
	covers := func(subset []string) bool {
		for item, q := range need {
			var have float64
			for _, s := range subset {
				have += depth[s][item]
			}
			if have < q {
				return false
			}
		}
		return true
	}

	// Exact search for k = 1..exactK over combinations in lexicographic order.
	for k := 1; k <= exactK && k <= len(ids); k++ {
		combo := make([]string, k)
		var found []string
		var rec func(start, filled int) bool
		rec = func(start, filled int) bool {
			if filled == k {
				if covers(combo) {
					found = append([]string(nil), combo...)
					return true
				}
				return false
			}
			for i := start; i <= len(ids)-(k-filled); i++ {
				combo[filled] = ids[i]
				if rec(i+1, filled+1) {
					return true
				}
			}
			return false
		}
		if rec(0, 0) {
			return CoverResult{Feasible: true, Count: k, Stations: found, Exact: true}
		}
	}

	// Greedy fallback: repeatedly add the station that covers the most remaining
	// shortfall; ties break by id (ids already sorted). Guaranteed to terminate
	// because the feasibility gate proved the full set covers.
	remaining := map[string]float64{}
	for item, q := range need {
		remaining[item] = q
	}
	chosen := map[string]bool{}
	var cover []string
	shortfallLeft := func() bool {
		for _, q := range remaining {
			if q > 0 {
				return true
			}
		}
		return false
	}
	for shortfallLeft() {
		bestID := ""
		bestGain := -1.0
		for _, s := range ids {
			if chosen[s] {
				continue
			}
			var gain float64
			for item, q := range remaining {
				if q <= 0 {
					continue
				}
				d := depth[s][item]
				if d > q {
					d = q
				}
				gain += d
			}
			if gain > bestGain {
				bestGain = gain
				bestID = s
			}
		}
		if bestID == "" { // safety; should not happen given the gate
			break
		}
		chosen[bestID] = true
		cover = append(cover, bestID)
		for item := range remaining {
			remaining[item] -= depth[bestID][item]
		}
	}
	sort.Strings(cover)
	return CoverResult{Feasible: true, Count: len(cover), Stations: cover, Exact: false}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/buildcost/ -run TestMinStationCover -v`
Expected: PASS (all 7 tests).

- [ ] **Step 5: Lint and commit**

Run: `golangci-lint run ./pkg/buildcost/` → Expected: `0 issues.`

```bash
git add pkg/buildcost/stationcover.go pkg/buildcost/stationcover_test.go
git commit -m "feat(buildcost): pure MinStationCover depth-aware set-cover"
```

---

## Task 2: Generator glue — depth map, per-target covers, page model

**Files:**
- Create: `cmd/generate-build-costs/stationcover.go`
- Test: `cmd/generate-build-costs/stationcover_test.go`

**Interfaces:**
- Consumes: `buildcost.MinStationCover`, `buildcost.CoverResult`, `buildcost.StationDepth` (Task 1); existing `buildcost.Book`, `buildcost.Target`, `StationMeta`.
- Produces:
  - `func stationDepthFromBooks(books map[string]*buildcost.Book) buildcost.StationDepth`
  - `type coverEntry struct { ID, Name, Category, Kind string; BoM buildcost.CoverResult; Recipe buildcost.CoverResult; RecipeNA bool; RecipeNAReason string; BatchSensitive bool; ExampleCover []string }`
  - `type unbuildableEntry struct { ID, Name, Category, Kind string; Missing []string }`
  - `type histBar struct { Stations, Count, HeightPct int }`
  - `type stationCoverPage struct { Total, SingleStation, MaxStations, UnbuildableCount, BatchSensitiveCount int; HardestName, HardestID string; AvgStations float64; Buildable []coverEntry; Unbuildable []unbuildableEntry; BoMHistogram, RecipeHistogram []histBar }`
  - `func buildStationCoverPage(targets []buildcost.Target, depth buildcost.StationDepth, stationIDs []string, stationNames, itemNames, categories map[string]string) stationCoverPage`

- [ ] **Step 1: Write the failing tests**

Create `cmd/generate-build-costs/stationcover_test.go`:

```go
package main

import (
	"reflect"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func TestStationDepthFromBooks_SumsLadder(t *testing.T) {
	books := map[string]*buildcost.Book{
		"A": {Sell: map[string]buildcost.Ladder{
			"iron": {{Price: 5, Qty: 3}, {Price: 6, Qty: 4}},
		}},
	}
	depth := stationDepthFromBooks(books)
	if depth["A"]["iron"] != 7 {
		t.Fatalf("depth = %v, want 7", depth["A"]["iron"])
	}
}

func TestBuildStationCoverPage_PartitionAndStats(t *testing.T) {
	targets := []buildcost.Target{
		{ID: "easy", Kind: "item", BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 1}}},
		{ID: "hard", Kind: "item", BoM: []buildcost.Requirement{
			{ItemID: "iron", Qty: 1}, {ItemID: "gold", Qty: 1}}},
		{ID: "nope", Kind: "ship", BoM: []buildcost.Requirement{{ItemID: "exotic", Qty: 1}},
			RecipeNA: "sub-assemblies not market-traded"},
	}
	depth := buildcost.StationDepth{
		"A": {"iron": 10},
		"B": {"gold": 10},
	}
	names := map[string]string{"A": "Alpha", "B": "Bravo"}
	items := map[string]string{"easy": "Easy", "hard": "Hard", "nope": "Nope", "exotic": "Exotic Matter"}
	cats := map[string]string{"easy": "widget", "hard": "gadget", "nope": "frigate"}
	p := buildStationCoverPage(targets, depth, []string{"A", "B"}, names, items, cats)

	if p.Total != 3 {
		t.Fatalf("Total = %d, want 3", p.Total)
	}
	if len(p.Buildable) != 2 {
		t.Fatalf("buildable = %d, want 2", len(p.Buildable))
	}
	// Sorted by BoM count desc then name: hard(2) before easy(1).
	if p.Buildable[0].ID != "hard" || p.Buildable[1].ID != "easy" {
		t.Fatalf("order = %s,%s want hard,easy", p.Buildable[0].ID, p.Buildable[1].ID)
	}
	if p.SingleStation != 1 {
		t.Errorf("SingleStation = %d, want 1", p.SingleStation)
	}
	if p.MaxStations != 2 || p.HardestID != "hard" {
		t.Errorf("hardest = %d/%s, want 2/hard", p.MaxStations, p.HardestID)
	}
	if len(p.Unbuildable) != 1 || p.Unbuildable[0].ID != "nope" {
		t.Fatalf("unbuildable = %+v, want [nope]", p.Unbuildable)
	}
	if !reflect.DeepEqual(p.Unbuildable[0].Missing, []string{"Exotic Matter"}) {
		t.Errorf("missing = %v, want [Exotic Matter]", p.Unbuildable[0].Missing)
	}
	// easy's example cover maps station ids to display names.
	if !reflect.DeepEqual(p.Buildable[1].ExampleCover, []string{"Alpha"}) {
		t.Errorf("easy cover = %v, want [Alpha]", p.Buildable[1].ExampleCover)
	}
}

func TestBuildStationCoverPage_RecipeBestFeasible(t *testing.T) {
	// Two recipes: r1 needs a rare input (infeasible), r2 needs a common one.
	targets := []buildcost.Target{{
		ID: "widget", Kind: "item",
		BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 1}},
		Recipes: []buildcost.Recipe{
			{ID: "r1", OutputQty: 1, Inputs: []buildcost.Requirement{{ItemID: "rare", Qty: 1}}},
			{ID: "r2", OutputQty: 1, Inputs: []buildcost.Requirement{{ItemID: "iron", Qty: 1}}},
		},
	}}
	depth := buildcost.StationDepth{"A": {"iron": 10}}
	p := buildStationCoverPage(targets, depth, []string{"A"},
		map[string]string{"A": "Alpha"}, map[string]string{"widget": "Widget"},
		map[string]string{"widget": "widget"})
	e := p.Buildable[0]
	if e.RecipeNA {
		t.Fatalf("recipe should be feasible via r2, got NA")
	}
	if !e.Recipe.Feasible || e.Recipe.Count != 1 {
		t.Fatalf("recipe cover = %+v, want feasible count 1", e.Recipe)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/generate-build-costs/ -run 'TestStationDepthFromBooks|TestBuildStationCoverPage' -v`
Expected: FAIL — `undefined: stationDepthFromBooks` / `buildStationCoverPage`.

- [ ] **Step 3: Implement the glue**

Create `cmd/generate-build-costs/stationcover.go`:

```go
package main

import (
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

const stationCoverExactK = 3
const stationCoverBatch = 10

type coverEntry struct {
	ID, Name, Category, Kind string
	BoM                      buildcost.CoverResult
	Recipe                   buildcost.CoverResult
	RecipeNA                 bool
	RecipeNAReason           string
	BatchSensitive           bool
	ExampleCover             []string // BoM cover, station display names
}

type unbuildableEntry struct {
	ID, Name, Category, Kind string
	Missing                  []string // input display names
}

type histBar struct {
	Stations, Count, HeightPct int
}

type stationCoverPage struct {
	Total               int
	SingleStation       int
	MaxStations         int
	UnbuildableCount    int
	BatchSensitiveCount int
	HardestName         string
	HardestID           string
	AvgStations         float64
	Buildable           []coverEntry
	Unbuildable         []unbuildableEntry
	BoMHistogram        []histBar
	RecipeHistogram     []histBar
}

// stationDepthFromBooks sums each station's sell-ladder quantities per item.
func stationDepthFromBooks(books map[string]*buildcost.Book) buildcost.StationDepth {
	depth := make(buildcost.StationDepth, len(books))
	for st, b := range books {
		if b == nil {
			continue
		}
		m := map[string]float64{}
		for item, ladder := range b.Sell {
			var sum float64
			for _, o := range ladder {
				sum += o.Qty
			}
			m[item] = sum
		}
		depth[st] = m
	}
	return depth
}

// bestRecipeCover returns the feasible recipe cover with the smallest station
// count (ties: fewer stations already handled; then recipe order as given), and
// whether the target's Recipe mode is N/A.
func bestRecipeCover(t buildcost.Target, depth buildcost.StationDepth, ids []string) (buildcost.CoverResult, bool) {
	if t.RecipeNA != "" || len(t.Recipes) == 0 {
		return buildcost.CoverResult{}, true
	}
	var best buildcost.CoverResult
	found := false
	for _, r := range t.Recipes {
		c := buildcost.MinStationCover(r.Inputs, depth, ids, stationCoverExactK)
		if !c.Feasible {
			continue
		}
		if !found || c.Count < best.Count {
			best = c
			found = true
		}
	}
	if !found {
		return buildcost.CoverResult{}, true // no recipe sourceable
	}
	return best, false
}

func scaleReqs(reqs []buildcost.Requirement, factor float64) []buildcost.Requirement {
	out := make([]buildcost.Requirement, len(reqs))
	for i, r := range reqs {
		out[i] = buildcost.Requirement{ItemID: r.ItemID, Qty: r.Qty * factor}
	}
	return out
}

func displayNames(ids []string, names map[string]string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		if n := names[id]; n != "" {
			out[i] = n
		} else {
			out[i] = id
		}
	}
	return out
}

func buildStationCoverPage(targets []buildcost.Target, depth buildcost.StationDepth, stationIDs []string,
	stationNames, itemNames, categories map[string]string) stationCoverPage {
	p := stationCoverPage{Total: len(targets)}
	var sumStations int

	for _, t := range targets {
		bom := buildcost.MinStationCover(t.BoM, depth, stationIDs, stationCoverExactK)
		name := itemNames[t.ID]
		if name == "" {
			name = t.ID
		}
		if !bom.Feasible {
			p.Unbuildable = append(p.Unbuildable, unbuildableEntry{
				ID: t.ID, Name: name, Category: categories[t.ID], Kind: t.Kind,
				Missing: displayNames(bom.Missing, itemNames),
			})
			continue
		}
		rec, recNA := bestRecipeCover(t, depth, stationIDs)
		batch := buildcost.MinStationCover(scaleReqs(t.BoM, stationCoverBatch), depth, stationIDs, stationCoverExactK)
		e := coverEntry{
			ID: t.ID, Name: name, Category: categories[t.ID], Kind: t.Kind,
			BoM: bom, Recipe: rec, RecipeNA: recNA,
			RecipeNAReason: t.RecipeNA,
			BatchSensitive: batch.Feasible && batch.Count > bom.Count,
			ExampleCover:   displayNames(bom.Stations, stationNames),
		}
		p.Buildable = append(p.Buildable, e)
		sumStations += bom.Count
		if bom.Count == 1 {
			p.SingleStation++
		}
		if bom.Count > p.MaxStations {
			p.MaxStations = bom.Count
			p.HardestID = t.ID
			p.HardestName = name
		}
		if e.BatchSensitive {
			p.BatchSensitiveCount++
		}
	}

	p.UnbuildableCount = len(p.Unbuildable)
	if len(p.Buildable) > 0 {
		p.AvgStations = float64(sumStations) / float64(len(p.Buildable))
	}

	// Sort buildable by BoM count desc, then name; unbuildable by name.
	sort.Slice(p.Buildable, func(i, j int) bool {
		if p.Buildable[i].BoM.Count != p.Buildable[j].BoM.Count {
			return p.Buildable[i].BoM.Count > p.Buildable[j].BoM.Count
		}
		if p.Buildable[i].Name != p.Buildable[j].Name {
			return p.Buildable[i].Name < p.Buildable[j].Name
		}
		return p.Buildable[i].ID < p.Buildable[j].ID
	})
	sort.Slice(p.Unbuildable, func(i, j int) bool {
		if p.Unbuildable[i].Name != p.Unbuildable[j].Name {
			return p.Unbuildable[i].Name < p.Unbuildable[j].Name
		}
		return p.Unbuildable[i].ID < p.Unbuildable[j].ID
	})

	p.BoMHistogram = histogram(p.Buildable, func(e coverEntry) (int, bool) { return e.BoM.Count, true })
	p.RecipeHistogram = histogram(p.Buildable, func(e coverEntry) (int, bool) {
		return e.Recipe.Count, !e.RecipeNA && e.Recipe.Feasible
	})
	return p
}

// histogram buckets buildable entries by a count selector into bars 1..max, with
// zero-count buckets filled in and HeightPct scaled to the tallest bar.
func histogram(entries []coverEntry, sel func(coverEntry) (int, bool)) []histBar {
	counts := map[int]int{}
	maxN := 0
	for _, e := range entries {
		n, ok := sel(e)
		if !ok || n < 1 {
			continue
		}
		counts[n]++
		if n > maxN {
			maxN = n
		}
	}
	if maxN == 0 {
		return nil
	}
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	bars := make([]histBar, 0, maxN)
	for n := 1; n <= maxN; n++ {
		h := 0
		if maxCount > 0 {
			h = int(100 * float64(counts[n]) / float64(maxCount))
		}
		bars = append(bars, histBar{Stations: n, Count: counts[n], HeightPct: h})
	}
	return bars
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/generate-build-costs/ -run 'TestStationDepthFromBooks|TestBuildStationCoverPage' -v`
Expected: PASS (all 3 tests).

- [ ] **Step 5: Lint and commit**

Run: `golangci-lint run ./cmd/generate-build-costs/` → Expected: `0 issues.`

```bash
git add cmd/generate-build-costs/stationcover.go cmd/generate-build-costs/stationcover_test.go
git commit -m "feat(build-costs): compute per-target station-cover page model"
```

---

## Task 3: Page template + render function

**Files:**
- Create: `cmd/generate-build-costs/stationcover_render.go`
- Create: `cmd/generate-build-costs/templates/stationcover.html.tmpl`
- Test: append to `cmd/generate-build-costs/stationcover_test.go`

**Interfaces:**
- Consumes: `stationCoverPage`, `coverEntry`, `unbuildableEntry`, `histBar` (Task 2).
- Produces: `func renderStationCover(outPath string, p stationCoverPage) error`

**Note:** the existing `//go:embed templates/*.tmpl` (in `render.go`) already includes any new `.tmpl` file — no embed directive change needed. `renderStationCover` parses `templates/stationcover.html.tmpl` from `tmplFS`.

- [ ] **Step 1: Write the failing test**

Append to `cmd/generate-build-costs/stationcover_test.go`:

```go
func TestRenderStationCover_WritesExpectedContent(t *testing.T) {
	p := stationCoverPage{
		Total: 3, SingleStation: 1, MaxStations: 2, UnbuildableCount: 1,
		BatchSensitiveCount: 1, HardestName: "Hard Widget", HardestID: "hard",
		AvgStations: 1.5,
		Buildable: []coverEntry{
			{ID: "hard", Name: "Hard Widget", Category: "gadget", Kind: "item",
				BoM:    buildcost.CoverResult{Feasible: true, Count: 2, Exact: true},
				Recipe: buildcost.CoverResult{Feasible: true, Count: 1, Exact: true},
				ExampleCover: []string{"Alpha", "Bravo"}},
			{ID: "easy", Name: "Easy Widget", Category: "widget", Kind: "item",
				BoM: buildcost.CoverResult{Feasible: true, Count: 1, Exact: true}, RecipeNA: true,
				ExampleCover: []string{"Alpha"}, BatchSensitive: true},
		},
		Unbuildable: []unbuildableEntry{
			{ID: "nope", Name: "Void Reactor", Category: "reactor", Kind: "ship", Missing: []string{"Exotic Matter"}},
		},
		BoMHistogram:    []histBar{{Stations: 1, Count: 1, HeightPct: 100}, {Stations: 2, Count: 1, HeightPct: 100}},
		RecipeHistogram: []histBar{{Stations: 1, Count: 1, HeightPct: 100}},
	}
	dir := t.TempDir()
	out := dir + "/stations_to_build.html"
	if err := renderStationCover(out, p); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"Hard Widget",         // hardest factoid + main-table row
		"../build-costs/hard.html", // target link
		"Alpha, Bravo",        // example cover
		"N/A",                 // easy's recipe column
		"Void Reactor",        // unbuildable row
		"Exotic Matter",       // missing input
		"chart-bar",           // bar chart present
	} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered page missing %q", want)
		}
	}
}
```

Add imports `os` and `strings` to the test file's import block if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-build-costs/ -run TestRenderStationCover -v`
Expected: FAIL — `undefined: renderStationCover`.

- [ ] **Step 3a: Create the template**

Create `cmd/generate-build-costs/templates/stationcover.html.tmpl`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Stations Needed to Build — Spacemolt KB</title>
<link rel="stylesheet" href="../smui.css">
<style>
 body{max-width:1100px;margin:0 auto;padding:1rem}
 .factoids{display:grid;grid-template-columns:repeat(auto-fill,minmax(240px,1fr));gap:12px;margin:1rem 0}
 .factoid{background:var(--bg-card,#161b22);border:1px solid var(--border,#30363d);border-left:4px solid #6c5ce7;border-radius:8px;padding:14px}
 .factoid .big{font-size:1.6rem;font-weight:700;color:var(--link,#58a6ff)}
 .factoid .lbl{font-size:.85rem;color:var(--text-muted,#8b949e)}
 .charts{display:flex;flex-wrap:wrap;gap:2rem;margin:1rem 0}
 .chart{flex:1;min-width:280px}
 .chart h3{font-size:1rem;margin:.2rem 0}
 .chart-row{display:flex;align-items:flex-end;gap:6px;height:160px;border-bottom:1px solid var(--border,#30363d);padding-bottom:0}
 .chart-col{display:flex;flex-direction:column;align-items:center;justify-content:flex-end;flex:1}
 .chart-bar{width:100%;background:#6c5ce7;border-radius:3px 3px 0 0;min-height:2px}
 .chart-col .n{font-size:.75rem;color:var(--text-muted,#8b949e)}
 .chart-col .x{font-size:.8rem;margin-top:.2rem}
 .unbuildable-note{font-size:.85rem;color:var(--text-muted,#8b949e);margin-top:.5rem}
 table{border-collapse:collapse;width:100%;font-size:.85rem;margin-top:1rem}
 th,td{border:1px solid var(--border,#21262d);padding:.3rem .5rem;text-align:left}
 th{cursor:pointer;background:var(--bg-card,#161b22)}
 td.num{text-align:right}
 .badge{font-size:.7rem;background:#9a6700;color:#fff;border-radius:3px;padding:0 .3rem;margin-left:.3rem}
 .scrollx{overflow-x:auto}
</style>
</head>
<body>
<p><a href="index.html">← Did You Know?</a></p>
<h1>How Many Stations to Build Anything?</h1>
<p>The fewest distinct market stations you must shop at to source every part for one build, depth-aware, across all {{.Total}} items and ships. Count only — travel between stations is ignored. <b>BoM</b> = from raw ore; <b>Recipe</b> = from sub-assemblies.</p>

<div class="factoids">
 <div class="factoid"><div class="big">{{.SingleStation}}</div><div class="lbl">targets buildable from a single station</div></div>
 <div class="factoid"><div class="big">{{printf "%.1f" .AvgStations}}</div><div class="lbl">average stations per buildable target</div></div>
 <div class="factoid"><div class="big">{{.MaxStations}}</div><div class="lbl">hardest: <a href="../build-costs/{{.HardestID}}.html">{{.HardestName}}</a></div></div>
 <div class="factoid"><div class="big">{{.UnbuildableCount}}</div><div class="lbl">targets can't be built anywhere — parts aren't all sold</div></div>
 <div class="factoid"><div class="big">{{.BatchSensitiveCount}}</div><div class="lbl">need more stations to build 10 at once</div></div>
</div>

<div class="charts">
 <div class="chart">
  <h3>BoM — targets by station count</h3>
  <div class="chart-row">
   {{range .BoMHistogram}}<div class="chart-col"><div class="n">{{.Count}}</div><div class="chart-bar" style="height:{{.HeightPct}}%"></div><div class="x">{{.Stations}}</div></div>{{end}}
  </div>
  <div class="unbuildable-note">+ {{.UnbuildableCount}} unbuildable anywhere</div>
 </div>
 <div class="chart">
  <h3>Recipe — targets by station count</h3>
  <div class="chart-row">
   {{range .RecipeHistogram}}<div class="chart-col"><div class="n">{{.Count}}</div><div class="chart-bar" style="height:{{.HeightPct}}%"></div><div class="x">{{.Stations}}</div></div>{{end}}
  </div>
 </div>
</div>

<h2>Every buildable target</h2>
<div class="scrollx">
<table id="main">
<thead><tr><th>Target</th><th>Category</th><th>Kind</th><th>BoM stations</th><th>Recipe stations</th><th>Example cover</th></tr></thead>
<tbody>
{{range .Buildable}}<tr>
<td><a href="../build-costs/{{.ID}}.html">{{.Name}}</a>{{if .BatchSensitive}}<span class="badge" title="needs more stations to build 10">×10</span>{{end}}</td>
<td>{{.Category}}</td><td>{{.Kind}}</td>
<td class="num" data-sort="{{.BoM.Count}}">{{.BoM.Count}}{{if not .BoM.Exact}}*{{end}}</td>
<td class="num" data-sort="{{if .RecipeNA}}-1{{else}}{{.Recipe.Count}}{{end}}">{{if .RecipeNA}}N/A{{else}}{{.Recipe.Count}}{{if not .Recipe.Exact}}*{{end}}{{end}}</td>
<td>{{range $i, $s := .ExampleCover}}{{if $i}}, {{end}}{{$s}}{{end}}</td>
</tr>{{end}}
</tbody>
</table>
</div>
<p style="font-size:.8rem;color:#8b949e">* = greedy upper bound (a proven minimum wasn't searched beyond 3 stations).</p>

<h2>Can't be built anywhere ({{.UnbuildableCount}})</h2>
<p>These can't be built at any combination of stations right now — one or more inputs have no market depth anywhere.</p>
<div class="scrollx">
<table>
<thead><tr><th>Target</th><th>Category</th><th>Kind</th><th>Missing input(s)</th></tr></thead>
<tbody>
{{range .Unbuildable}}<tr><td>{{.Name}}</td><td>{{.Category}}</td><td>{{.Kind}}</td><td>{{range $i, $m := .Missing}}{{if $i}}, {{end}}{{$m}}{{end}}</td></tr>{{end}}
</tbody>
</table>
</div>

<script>
// Lightweight click-to-sort for the main table.
document.querySelectorAll('#main th').forEach((th, idx) => th.addEventListener('click', () => {
  const tb = th.closest('table').tBodies[0];
  const rows = [...tb.rows];
  const dir = th.dataset.dir === 'asc' ? -1 : 1;
  th.dataset.dir = dir === 1 ? 'asc' : 'desc';
  rows.sort((a, b) => {
    const ca = a.cells[idx], cb = b.cells[idx];
    const va = ca.dataset.sort ?? ca.textContent.trim();
    const vb = cb.dataset.sort ?? cb.textContent.trim();
    const na = parseFloat(va), nb = parseFloat(vb);
    if (!isNaN(na) && !isNaN(nb)) return (na - nb) * dir;
    return va.localeCompare(vb) * dir;
  });
  rows.forEach(r => tb.appendChild(r));
}));
</script>
</body>
</html>
```

- [ ] **Step 3b: Implement `renderStationCover`**

Create `cmd/generate-build-costs/stationcover_render.go`:

```go
package main

import (
	"html/template"
	"os"
	"path/filepath"
)

// renderStationCover writes the station-cover did-you-know page to outPath.
func renderStationCover(outPath string, p stationCoverPage) error {
	t, err := template.ParseFS(tmplFS, "templates/stationcover.html.tmpl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, p)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-build-costs/ -run TestRenderStationCover -v`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

Run: `golangci-lint run ./cmd/generate-build-costs/` → Expected: `0 issues.`

```bash
git add cmd/generate-build-costs/stationcover_render.go cmd/generate-build-costs/templates/stationcover.html.tmpl cmd/generate-build-costs/stationcover_test.go
git commit -m "feat(build-costs): render station-cover did-you-know page"
```

---

## Task 4: Wire into `main.go` and regenerate

**Files:**
- Modify: `cmd/generate-build-costs/main.go`

**Interfaces:**
- Consumes: `stationDepthFromBooks`, `buildStationCoverPage`, `renderStationCover` (Tasks 2–3); existing in-scope vars `books`, `stations`, `targets`, `itemNames`, `categories`.

- [ ] **Step 1: Add the flag**

In `main.go`, in the flag block (after the `outDir` flag at line ~27), add:

```go
	stationCoverOut := flag.String("station-cover-out", "kb/did_you_know/stations_to_build.html", "output path for the station-cover did-you-know page; empty disables it")
```

- [ ] **Step 2: Add the render call**

In `main.go`, immediately before the final `log.Printf("build-costs: %d rows ...` line at the end of `main`, add:

```go
	if *stationCoverOut != "" {
		depth := stationDepthFromBooks(books)
		stationIDs := make([]string, 0, len(stations))
		stationNames := make(map[string]string, len(stations))
		for _, st := range stations {
			stationIDs = append(stationIDs, st.ID)
			stationNames[st.ID] = st.Name
		}
		page := buildStationCoverPage(targets, depth, stationIDs, stationNames, itemNames, categories)
		must(renderStationCover(*stationCoverOut, page), "render station cover")
		log.Printf("station-cover: %d buildable, %d single-station, %d unbuildable, hardest %s (%d) → %s",
			len(page.Buildable), page.SingleStation, page.UnbuildableCount, page.HardestID, page.MaxStations, *stationCoverOut)
	}
```

- [ ] **Step 3: Build and run the full generator**

Run: `go build ./cmd/generate-build-costs/... && go run ./cmd/generate-build-costs -crafting /home/robert/spacemolt/spacemolt/data/crafting.db -market /home/robert/spacemolt/spacemolt/data/market.db -knowledge /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db -catalog /home/robert/spacemolt/spacemolt/data/game-api -out kb/build-costs`

Expected: the existing `build-costs:` lines plus a new `station-cover: N buildable, ... → kb/did_you_know/stations_to_build.html` line, exit 0.

- [ ] **Step 4: Verify the generated page**

Run: `grep -c 'chart-bar' kb/did_you_know/stations_to_build.html && grep -oE '<title>[^<]*</title>' kb/did_you_know/stations_to_build.html && grep -c 'build-costs/' kb/did_you_know/stations_to_build.html`

Expected: a non-zero `chart-bar` count, the `Stations Needed to Build` title, and many `build-costs/` links (one per buildable row + hardest factoid).

- [ ] **Step 5: Run the package tests and lint**

Run: `go test ./cmd/generate-build-costs/... ./pkg/buildcost/... && golangci-lint run ./cmd/generate-build-costs/ ./pkg/buildcost/`
Expected: all `ok`, `0 issues.`

- [ ] **Step 6: Commit source + generated page**

```bash
git add cmd/generate-build-costs/main.go kb/did_you_know/stations_to_build.html
git commit -m "feat(build-costs): wire station-cover page into generator + regenerate"
```

---

## Task 5: Link the page from the did-you-know index

**Files:**
- Modify: `kb/did_you_know/index.html`

**Interfaces:** none (static HTML edit).

- [ ] **Step 1: Add the card after the Capitals & Strongholds card**

In `kb/did_you_know/index.html`, the `.factoid-grid` contains sibling cards of the exact form `<a href="..." class="factoid-card"><h3>…</h3><p>…</p><div class="meta"><span>…</span><span>…</span></div></a>`. Insert this new card immediately after the `capital_stronghold_distances.html` card's closing `</a>` (indentation: 12 spaces on the `<a>`, matching siblings):

```html
            <a href="stations_to_build.html" class="factoid-card">
                <h3>Stations Needed to Build</h3>
                <p>The fewest distinct stations you must shop at to source every part for one build — for every item and ship, depth-aware, with the full distribution and the things that can't be built anywhere.</p>
                <div class="meta">
                    <span>🏭 Build Sourcing</span>
                    <span>📊 Station Set-Cover</span>
                </div>
            </a>
```

- [ ] **Step 2: Verify the link resolves**

Run: `grep -c 'stations_to_build.html' kb/did_you_know/index.html`
Expected: `1` (the new card).

- [ ] **Step 3: Commit**

```bash
git add kb/did_you_know/index.html
git commit -m "docs(did-you-know): add Stations Needed to Build card"
```

---

## Final Verification (after all tasks)

- [ ] `go build ./...` → exit 0
- [ ] `go test ./cmd/generate-build-costs/... ./pkg/buildcost/...` → all `ok`
- [ ] `golangci-lint run ./cmd/generate-build-costs/ ./pkg/buildcost/` → `0 issues.`
- [ ] Open `kb/did_you_know/stations_to_build.html` in a browser: factoids populated, both bar charts render with a visible tail, main table sorts on header click, example-cover names show, bottom table lists unbuildable targets with missing inputs.
- [ ] `kb/did_you_know/index.html` shows the new card and it navigates to the page.
