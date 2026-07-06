# Build-Cost +N-Hop Fulfillment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional +1/+2/+3-hop input sourcing to the build-cost matrix: a build at a home station may pool order-book depth from every station within N system-jumps, expanding feasibility and lowering cost.

**Architecture:** Pooling lives entirely in the generator `cmd/generate-build-costs`. A "hop-N build at station S" is a **pooled `buildcost.Book`** (union of the reachable stations' sell ladders, re-sorted ascending) fed to the *existing, unchanged* `BuildCell`. Four landing matrices are rendered (Local + +1/+2/+3), tab-linked; detail pages gain per-radius cost columns.

**Tech Stack:** Go 1.24+; `modernc.org/sqlite` (pure-Go, read-only); `html/template`; existing `pkg/buildcost` pure core.

**Spec:** `docs/superpowers/specs/2026-07-05-build-cost-hop-fulfillment-design.md`.

## Global Constraints

- **Do NOT modify `pkg/buildcost`** — it stays pure; pooling is a generator concern only.
- **Cost model:** pooled depth is combined free (no transport/fuel cost). Cheapest-first walk across the pooled ladder.
- **Margin rule:** Savings/Profit always use the **home station's own book** (finished ask/bid at S), never the pooled book — only the *cost* book changes with radius.
- **System resolution:** `market.db stations.system_id` is id-first, then `systems.name` fallback, to canonical `systems.id` (used by `connections`). `connections` is symmetric.
- **Max radius = 3.** Pages: `index.html` (radius 0 "Local", unchanged URL/behavior), `hop-1.html`, `hop-2.html`, `hop-3.html`.
- **Determinism:** pool members sorted by station id; pooled ladders re-sorted ascending by price — regenerated site must diff cleanly.
- Go 1.24+ idioms (range-over-int where natural). Must pass `golangci-lint run ./cmd/generate-build-costs/` with **0 new findings**. `errcheck` is on (`_ = rows.Close()` / `defer func(){ _ = rows.Close() }()`; return `rows.Err()`).
- `func main` already exists, so full `go build ./...` + `go test ./...` gate applies to **every** task.

---

## File Structure

- **Create `cmd/generate-build-costs/hops.go`** — topology + pooling: system resolver, connections adjacency, BFS hop distances, pool membership, pooled-book construction.
- **Create `cmd/generate-build-costs/hops_test.go`** — unit tests for the above (synthetic graphs/books, no DB).
- **Modify `cmd/generate-build-costs/matrix.go`** — `BuildMatrix` takes separate cost and margin book maps.
- **Modify `cmd/generate-build-costs/matrix_test.go`** — update call sites + a margin-source test.
- **Modify `cmd/generate-build-costs/render.go`** — `renderIndex` takes a filename + tab list; `renderDetail` takes per-radius rows.
- **Modify `cmd/generate-build-costs/templates/index.html.tmpl`** — tab bar + radius-aware heading/title.
- **Modify `cmd/generate-build-costs/templates/detail.html.tmpl`** — per-radius cost columns.
- **Modify `cmd/generate-build-costs/main.go`** — build hop graph + pooled books, render 4 matrices, pass per-radius rows to detail.

---

## Task 1: Hop graph — system resolution, connections, BFS, pool membership

**Files:**
- Create: `cmd/generate-build-costs/hops.go`
- Test: `cmd/generate-build-costs/hops_test.go`

**Interfaces:**
- Produces:
  - `type systemResolver struct{ ids map[string]bool; byName map[string]string }`
  - `func (r *systemResolver) canon(systemID string) (string, bool)`
  - `func loadSystemResolver(knowledgeDB *sql.DB) (*systemResolver, error)`
  - `func loadConnections(knowledgeDB *sql.DB) (map[string][]string, error)` (symmetric adjacency, keyed by systems.id)
  - `func stationSystems(stations []StationMeta, r *systemResolver) (map[string]string, []string)` (stationID→systemID, plus unresolved station ids)
  - `func bfsHops(adj map[string][]string, src string) map[string]int`
  - `func stationHopDist(adj map[string][]string, stationSys map[string]string) map[string]map[string]int`
  - `func poolMembers(hopDist map[string]map[string]int, home string, radius int) []string`

- [ ] **Step 1: Write failing tests**

Create `cmd/generate-build-costs/hops_test.go`:
```go
package main

import (
	"reflect"
	"testing"
)

func TestSystemResolver_canon(t *testing.T) {
	r := &systemResolver{
		ids:    map[string]bool{"krynn": true, "haven": true},
		byName: map[string]string{"Alpha Centauri": "alpha_centauri", "Krynn": "krynn"},
	}
	cases := []struct {
		in       string
		wantID   string
		wantOK   bool
	}{
		{"krynn", "krynn", true},          // id hit
		{"Alpha Centauri", "alpha_centauri", true}, // name fallback
		{"Nowhere", "", false},            // unresolved
	}
	for _, c := range cases {
		id, ok := r.canon(c.in)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("canon(%q) = (%q,%v), want (%q,%v)", c.in, id, ok, c.wantID, c.wantOK)
		}
	}
}

func TestStationSystems(t *testing.T) {
	r := &systemResolver{
		ids:    map[string]bool{"krynn": true},
		byName: map[string]string{"Alpha Centauri": "alpha_centauri"},
	}
	stations := []StationMeta{
		{ID: "krynn_hub", System: "krynn"},
		{ID: "ac_hub", System: "Alpha Centauri"},
		{ID: "ghost", System: "Nowhere"},
	}
	got, unresolved := stationSystems(stations, r)
	want := map[string]string{"krynn_hub": "krynn", "ac_hub": "alpha_centauri"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stationSystems map = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(unresolved, []string{"ghost"}) {
		t.Errorf("unresolved = %v, want [ghost]", unresolved)
	}
}

func TestBfsHops(t *testing.T) {
	// a - b - c ; a - d ; e isolated
	adj := map[string][]string{
		"a": {"b", "d"}, "b": {"a", "c"}, "c": {"b"}, "d": {"a"}, "e": {},
	}
	got := bfsHops(adj, "a")
	want := map[string]int{"a": 0, "b": 1, "d": 1, "c": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bfsHops(a) = %v, want %v", got, want)
	}
	if _, ok := got["e"]; ok {
		t.Errorf("e should be unreachable from a")
	}
}

func TestStationHopDistAndPool(t *testing.T) {
	// systems: sA-sB-sC chain (sB has no station); station in sA, sC, and isolated sZ
	adj := map[string][]string{"sA": {"sB"}, "sB": {"sA", "sC"}, "sC": {"sB"}, "sZ": {}}
	stationSys := map[string]string{"A": "sA", "C": "sC", "Z": "sZ"}
	hd := stationHopDist(adj, stationSys)
	if hd["A"]["C"] != 2 { // A->sA->sB->sC = 2 jumps through empty sB
		t.Fatalf("hopDist A->C = %d, want 2", hd["A"]["C"])
	}
	if hd["A"]["A"] != 0 {
		t.Fatalf("hopDist A->A = %d, want 0", hd["A"]["A"])
	}
	if _, ok := hd["A"]["Z"]; ok {
		t.Fatalf("Z must be unreachable from A")
	}
	// pool membership (sorted, includes home)
	if got := poolMembers(hd, "A", 1); !reflect.DeepEqual(got, []string{"A"}) {
		t.Errorf("pool(A,1) = %v, want [A]", got) // C is 2 hops away
	}
	if got := poolMembers(hd, "A", 2); !reflect.DeepEqual(got, []string{"A", "C"}) {
		t.Errorf("pool(A,2) = %v, want [A C]", got)
	}
	if got := poolMembers(hd, "Z", 3); !reflect.DeepEqual(got, []string{"Z"}) {
		t.Errorf("pool(Z,3) = %v, want [Z]", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/generate-build-costs/ -run 'TestSystemResolver_canon|TestStationSystems|TestBfsHops|TestStationHopDistAndPool' -v`
Expected: FAIL — `undefined: systemResolver`, `undefined: stationSystems`, etc.

- [ ] **Step 3: Write the implementation**

Create `cmd/generate-build-costs/hops.go`:
```go
package main

import (
	"database/sql"
	"sort"
)

// systemResolver maps a market station's system_id — which may be a systems.id
// slug OR a systems.name display value — to the canonical systems.id used by the
// connections graph.
type systemResolver struct {
	ids    map[string]bool
	byName map[string]string
}

// canon resolves a raw system_id to a canonical systems.id: it prefers a direct
// id match, then falls back to a systems.name match. ok is false if neither hits.
func (r *systemResolver) canon(systemID string) (string, bool) {
	if r.ids[systemID] {
		return systemID, true
	}
	if id, ok := r.byName[systemID]; ok {
		return id, true
	}
	return "", false
}

// loadSystemResolver builds a resolver from the knowledge DB systems table.
func loadSystemResolver(knowledgeDB *sql.DB) (*systemResolver, error) {
	r := &systemResolver{ids: map[string]bool{}, byName: map[string]string{}}
	rows, err := knowledgeDB.Query(`SELECT id, name FROM systems`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		r.ids[id] = true
		r.byName[name] = id
	}
	return r, rows.Err()
}

// loadConnections returns a symmetric adjacency map (systems.id -> neighbor ids)
// from the knowledge DB connections table.
func loadConnections(knowledgeDB *sql.DB) (map[string][]string, error) {
	set := map[string]map[string]bool{}
	add := func(a, b string) {
		if set[a] == nil {
			set[a] = map[string]bool{}
		}
		set[a][b] = true
	}
	rows, err := knowledgeDB.Query(`SELECT from_system, to_system FROM connections`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		add(a, b)
		add(b, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	adj := make(map[string][]string, len(set))
	for a, nbrs := range set {
		list := make([]string, 0, len(nbrs))
		for b := range nbrs {
			list = append(list, b)
		}
		sort.Strings(list)
		adj[a] = list
	}
	return adj, nil
}

// stationSystems maps each station id to its canonical system id, and returns
// the ids of any stations whose system could not be resolved.
func stationSystems(stations []StationMeta, r *systemResolver) (map[string]string, []string) {
	out := map[string]string{}
	var unresolved []string
	for _, s := range stations {
		if id, ok := r.canon(s.System); ok {
			out[s.ID] = id
		} else {
			unresolved = append(unresolved, s.ID)
		}
	}
	return out, unresolved
}

// bfsHops returns the jump distance from src to every reachable system.
func bfsHops(adj map[string][]string, src string) map[string]int {
	dist := map[string]int{src: 0}
	queue := []string{src}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range adj[u] {
			if _, seen := dist[v]; !seen {
				dist[v] = dist[u] + 1
				queue = append(queue, v)
			}
		}
	}
	return dist
}

// stationHopDist returns hopDist[home][other] = jump distance between the two
// stations' systems. Unreachable pairs are absent. Every station maps to itself
// at distance 0 (even if its system has no connections).
func stationHopDist(adj map[string][]string, stationSys map[string]string) map[string]map[string]int {
	// group stations by their system to translate system distances to stations.
	out := map[string]map[string]int{}
	for home, homeSys := range stationSys {
		sysDist := bfsHops(adj, homeSys)
		row := map[string]int{}
		for other, otherSys := range stationSys {
			if home == other {
				row[other] = 0
				continue
			}
			if d, ok := sysDist[otherSys]; ok {
				row[other] = d
			}
		}
		out[home] = row
	}
	return out
}

// poolMembers returns the station ids within radius jumps of home (including
// home), sorted for deterministic output.
func poolMembers(hopDist map[string]map[string]int, home string, radius int) []string {
	var members []string
	for other, d := range hopDist[home] {
		if d <= radius {
			members = append(members, other)
		}
	}
	sort.Strings(members)
	return members
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/generate-build-costs/ -run 'TestSystemResolver_canon|TestStationSystems|TestBfsHops|TestStationHopDistAndPool' -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Full gate + commit**

Run: `go build ./... && go test ./... && golangci-lint run ./cmd/generate-build-costs/`
Expected: build clean, tests pass, `0 issues`.
```bash
git add cmd/generate-build-costs/hops.go cmd/generate-build-costs/hops_test.go
git commit -m "feat(build-costs): hop graph — system resolution, connections BFS, pool membership"
```

---

## Task 2: Pooled book construction

**Files:**
- Modify: `cmd/generate-build-costs/hops.go`
- Modify: `cmd/generate-build-costs/hops_test.go`

**Interfaces:**
- Consumes: `poolMembers`, `stationHopDist` (Task 1); `buildcost.Book`, `buildcost.Order`, `buildcost.Ladder` (existing).
- Produces:
  - `func pooledBook(books map[string]*buildcost.Book, members []string) *buildcost.Book`
  - `func pooledBooksForRadius(books map[string]*buildcost.Book, hopDist map[string]map[string]int, stations []StationMeta, radius int) map[string]*buildcost.Book`

- [ ] **Step 1: Write failing tests**

Add to `cmd/generate-build-costs/hops_test.go` (add `"github.com/rsned/spacemolt-kb/pkg/buildcost"` to its imports):
```go
func TestPooledBook_UnionAndSort(t *testing.T) {
	books := map[string]*buildcost.Book{
		"A": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 5}, {Price: 20, Qty: 5}}}},
		"B": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 15, Qty: 5}}, "gold": {{Price: 99, Qty: 1}}}},
	}
	pb := pooledBook(books, []string{"A", "B"})
	// iron ladder is the union of both, re-sorted ascending by price.
	iron := pb.Sell["iron"]
	wantPrices := []float64{10, 15, 20}
	if len(iron) != 3 {
		t.Fatalf("iron ladder len = %d, want 3", len(iron))
	}
	for i, p := range wantPrices {
		if iron[i].Price != p {
			t.Errorf("iron[%d].Price = %v, want %v", i, iron[i].Price, p)
		}
	}
	// gold only exists at B.
	if len(pb.Sell["gold"]) != 1 || pb.Sell["gold"][0].Price != 99 {
		t.Errorf("gold ladder wrong: %v", pb.Sell["gold"])
	}
	// A cheapest-first walk over the pool picks the globally cheapest depth.
	w := pb.Walk("iron", 6) // 5@10 + 1@15 = 65
	if w.Cost != 65 || w.Shortfall != 0 {
		t.Errorf("pooled walk = %+v, want cost 65 shortfall 0", w)
	}
}

func TestPooledBooksForRadius(t *testing.T) {
	// A and C are 2 hops apart; radius 2 pools them, radius 1 does not.
	hopDist := map[string]map[string]int{
		"A": {"A": 0, "C": 2},
		"C": {"C": 0, "A": 2},
	}
	books := map[string]*buildcost.Book{
		"A": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 20, Qty: 5}}}},
		"C": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 8, Qty: 5}}}},
	}
	stations := []StationMeta{{ID: "A"}, {ID: "C"}}
	r1 := pooledBooksForRadius(books, hopDist, stations, 1)
	if got := r1["A"].Walk("iron", 1).Cost; got != 20 {
		t.Errorf("radius1 A iron = %v, want 20 (local only)", got)
	}
	r2 := pooledBooksForRadius(books, hopDist, stations, 2)
	if got := r2["A"].Walk("iron", 1).Cost; got != 8 {
		t.Errorf("radius2 A iron = %v, want 8 (C is cheaper, within 2 hops)", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/generate-build-costs/ -run 'TestPooledBook_UnionAndSort|TestPooledBooksForRadius' -v`
Expected: FAIL — `undefined: pooledBook`, `undefined: pooledBooksForRadius`.

- [ ] **Step 3: Write the implementation**

Append to `cmd/generate-build-costs/hops.go` (add `"github.com/rsned/spacemolt-kb/pkg/buildcost"` to its import block):
```go
// pooledBook merges the sell ladders of the member stations into a single Book
// with each item's ladder re-sorted ascending by price. BestBuy is left empty:
// margins use the home station's own book, never the pool.
func pooledBook(books map[string]*buildcost.Book, members []string) *buildcost.Book {
	pb := &buildcost.Book{Sell: map[string]buildcost.Ladder{}, BestBuy: map[string]float64{}}
	for _, id := range members {
		b := books[id]
		if b == nil {
			continue
		}
		for item, ladder := range b.Sell {
			pb.Sell[item] = append(pb.Sell[item], ladder...)
		}
	}
	for item, ladder := range pb.Sell {
		sort.Slice(ladder, func(i, j int) bool { return ladder[i].Price < ladder[j].Price })
		pb.Sell[item] = ladder
	}
	return pb
}

// pooledBooksForRadius returns, for each station that has a local book, a pooled
// book combining every station within radius jumps. A station's pool always
// includes itself, so the active-station set is identical across radii.
func pooledBooksForRadius(books map[string]*buildcost.Book, hopDist map[string]map[string]int, stations []StationMeta, radius int) map[string]*buildcost.Book {
	out := make(map[string]*buildcost.Book, len(stations))
	for _, s := range stations {
		if books[s.ID] == nil {
			continue
		}
		out[s.ID] = pooledBook(books, poolMembers(hopDist, s.ID, radius))
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/generate-build-costs/ -run 'TestPooledBook_UnionAndSort|TestPooledBooksForRadius' -v`
Expected: PASS.

- [ ] **Step 5: Full gate + commit**

Run: `go build ./... && go test ./... && golangci-lint run ./cmd/generate-build-costs/`
Expected: all clean.
```bash
git add cmd/generate-build-costs/hops.go cmd/generate-build-costs/hops_test.go
git commit -m "feat(build-costs): pooled book construction across a hop radius"
```

---

## Task 3: Split cost and margin books in BuildMatrix

The current `BuildMatrix` uses `books[st.ID]` for BOTH the cost walk and the margin. Hop pooling needs the cost book to be the pooled book while the margin stays anchored to the home station's local book.

**Files:**
- Modify: `cmd/generate-build-costs/matrix.go:49` (signature + body)
- Modify: `cmd/generate-build-costs/main.go:79` (call site — pass local books twice for hop-0)
- Modify: `cmd/generate-build-costs/matrix_test.go` (existing call sites + new test)

**Interfaces:**
- Produces: `func BuildMatrix(targets []buildcost.Target, costBooks, marginBooks map[string]*buildcost.Book, stations []StationMeta, names, categories map[string]string, listings map[string]map[string]float64, catalogPrice map[string]int) Matrix`
- Consumes: `itemMargin`, `shipMargin`, `buildcost.BuildCell` (unchanged).

- [ ] **Step 1: Write the failing test**

Add to `cmd/generate-build-costs/matrix_test.go`:
```go
func TestBuildMatrix_MarginUsesMarginBook(t *testing.T) {
	// Target 'widget' needs 1 iron. Cost book has cheap iron AND (deliberately)
	// no finished 'widget' ask. Margin book carries the finished 'widget' ask.
	// Savings must come from the margin book, proving the two are separate.
	target := buildcost.Target{
		ID: "widget", Kind: "item",
		BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 1}},
	}
	costBooks := map[string]*buildcost.Book{
		"S": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 1}}}, BestBuy: map[string]float64{}},
	}
	marginBooks := map[string]*buildcost.Book{
		"S": {Sell: map[string]buildcost.Ladder{"widget": {{Price: 30, Qty: 1}}}, BestBuy: map[string]float64{}},
	}
	stations := []StationMeta{{ID: "S", Name: "S", Empire: "Independent"}}
	m := BuildMatrix([]buildcost.Target{target}, costBooks, marginBooks, stations,
		map[string]string{"widget": "Widget"}, map[string]string{"widget": "component"},
		map[string]map[string]float64{}, map[string]int{})
	if len(m.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(m.Rows))
	}
	c := m.Rows[0].Cells["S"]
	if c.BoMCost != 10 || !c.BoMFeasible {
		t.Fatalf("BoM cost/feasible = %v/%v, want 10/true", c.BoMCost, c.BoMFeasible)
	}
	// Savings = finished ask (30, from marginBooks) - cost (10) = 20.
	if !c.HasSavings || c.SavingsBoM != 20 {
		t.Errorf("SavingsBoM = %v (has=%v), want 20 — margin must come from marginBooks", c.SavingsBoM, c.HasSavings)
	}
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `go test ./cmd/generate-build-costs/ -run TestBuildMatrix_MarginUsesMarginBook -v`
Expected: FAIL — `too many arguments in call to BuildMatrix` (signature is still the old 7-arg form).

- [ ] **Step 3: Update BuildMatrix signature + body**

In `cmd/generate-build-costs/matrix.go`, change the function signature (line 49-50) to add `marginBooks`:
```go
func BuildMatrix(targets []buildcost.Target, costBooks, marginBooks map[string]*buildcost.Book, stations []StationMeta,
	names, categories map[string]string, listings map[string]map[string]float64, catalogPrice map[string]int) Matrix {
```
In the body, replace the active-station filter and the per-cell book lookup to use `costBooks`, and the margin lookup to use `marginBooks`. Specifically:
- The `for _, st := range stations { if books[st.ID] != nil` filter becomes `if costBooks[st.ID] != nil`.
- `book := books[st.ID]` becomes `book := costBooks[st.ID]`.
- The item-margin line `margin = itemMargin(book, t.ID)` becomes `margin = itemMargin(marginBooks[st.ID], t.ID)`.
The `shipMargin(...)` branch is unchanged (it does not read a book). `buildcost.BuildCell(t, st.ID, book, margin)` keeps using the cost `book`.

- [ ] **Step 4: Update existing call sites**

In `cmd/generate-build-costs/main.go:79`, change:
```go
	m := BuildMatrix(targets, books, stations, names, categories, listings, catalogPrice)
```
to pass the local books as BOTH cost and margin (hop-0 behavior is unchanged):
```go
	m := BuildMatrix(targets, books, books, stations, names, categories, listings, catalogPrice)
```
In `cmd/generate-build-costs/matrix_test.go`, find every existing `BuildMatrix(` call and insert the same book map as the new second argument (cost and margin identical). For example a call `BuildMatrix(targets, books, stations, ...)` becomes `BuildMatrix(targets, books, books, stations, ...)`. Do this for ALL existing call sites so they compile with unchanged behavior.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/generate-build-costs/ -run 'TestBuildMatrix' -v`
Expected: PASS — the new margin test and all pre-existing `TestBuildMatrix*` tests.

- [ ] **Step 6: Full gate + commit**

Run: `go build ./... && go test ./... && golangci-lint run ./cmd/generate-build-costs/`
Expected: all clean.
```bash
git add cmd/generate-build-costs/matrix.go cmd/generate-build-costs/matrix_test.go cmd/generate-build-costs/main.go
git commit -m "refactor(build-costs): BuildMatrix takes separate cost and margin book maps"
```

---

## Task 4: Radius tabs in renderIndex + template

**Files:**
- Modify: `cmd/generate-build-costs/render.go` (`renderIndex` ~line 84)
- Modify: `cmd/generate-build-costs/templates/index.html.tmpl` (head/title + tab bar + heading)
- Modify: `cmd/generate-build-costs/main.go:81` (call site)

**Interfaces:**
- Produces:
  - `type radiusTab struct{ Label, File string; Active bool }`
  - `func renderIndex(outDir, fileName string, m Matrix, heading string, tabs []radiusTab) error`

- [ ] **Step 1: Update renderIndex signature + body**

In `cmd/generate-build-costs/render.go`, add the tab type just above `renderIndex`:
```go
// radiusTab is one entry in the hop-radius tab bar.
type radiusTab struct {
	Label string
	File  string
	Active bool
}
```
Replace `func renderIndex(outDir string, m Matrix) error { ... }` with:
```go
func renderIndex(outDir, fileName string, m Matrix, heading string, tabs []radiusTab) error {
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
	f, err := os.Create(filepath.Join(outDir, fileName))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, map[string]any{"JSON": template.JS(js), "Heading": heading, "Tabs": tabs})
}
```

- [ ] **Step 2: Update the template**

In `cmd/generate-build-costs/templates/index.html.tmpl`:
- Change the `<title>` line to: `<title>{{.Heading}} — Spacemolt KB</title>`
- Add a tab-bar style inside the `<style>` block (next to the other rules):
```css
 .tabs{margin:.4rem 0;display:flex;gap:.4rem}
 .tabs a{padding:.25rem .6rem;border:1px solid #30363d;border-radius:4px;text-decoration:none}
 .tabs a.active{background:#1f6feb;color:#fff;border-color:#1f6feb}
```
- Replace the `<h1>Build Cost Matrix</h1>` line with the dynamic heading plus a tab bar directly beneath it:
```html
<h1>{{.Heading}}</h1>
<div class="tabs">{{range .Tabs}}<a href="{{.File}}"{{if .Active}} class="active"{{end}}>{{.Label}}</a>{{end}}</div>
```
- Immediately after the existing intro `<p>Live-market build cost…</p>` line, append this sentence to describe pooling (add it as a new `<p>`):
```html
<p>Each tab pools sell-order depth from stations within N jumps of the column's home station; cost is the cheapest sourcing within reach. Savings/Profit always compare against the finished good at the home station.</p>
```

- [ ] **Step 3: Update the call site (temporary single-page wiring)**

In `cmd/generate-build-costs/main.go`, replace line 81:
```go
	must(renderIndex(*outDir, m), "render index")
```
with a hop-0-only call that satisfies the new signature (Task 5 will add the loop):
```go
	tabs := []radiusTab{{Label: "Local", File: "index.html", Active: true}}
	must(renderIndex(*outDir, "index.html", m, "Build Cost Matrix (Local)", tabs), "render index")
```

- [ ] **Step 4: Full gate + commit**

Run: `go build ./... && go test ./... && golangci-lint run ./cmd/generate-build-costs/`
Expected: all clean (no test asserts the tab HTML yet; Task 5 exercises it end-to-end).
```bash
git add cmd/generate-build-costs/render.go cmd/generate-build-costs/templates/index.html.tmpl cmd/generate-build-costs/main.go
git commit -m "feat(build-costs): radius-aware renderIndex with hop tab bar"
```

---

## Task 5: main.go — build hop graph, pooled books, render 4 matrices

**Files:**
- Modify: `cmd/generate-build-costs/main.go`

**Interfaces:**
- Consumes: `loadSystemResolver`, `loadConnections`, `stationSystems`, `stationHopDist`, `pooledBooksForRadius` (Tasks 1-2); `BuildMatrix` (Task 3, cost+margin); `renderIndex` (Task 4).
- Produces: `matrices []Matrix` in main (index 0..3 = radius) used by Task 6.

- [ ] **Step 1: Build the hop graph after loading stations**

In `cmd/generate-build-costs/main.go`, after `stations, err := loadStations(...)` / `must(...)` (around line 54-55), add:
```go
	resolver, err := loadSystemResolver(knowledgeDB)
	must(err, "load system resolver")
	adj, err := loadConnections(knowledgeDB)
	must(err, "load connections")
	stationSys, unresolved := stationSystems(stations, resolver)
	if len(unresolved) > 0 {
		log.Printf("build-costs: %d station(s) had unresolved systems (treated as isolated): %v", len(unresolved), unresolved)
	}
	hopDist := stationHopDist(adj, stationSys)
```

- [ ] **Step 2: Replace the single BuildMatrix + renderIndex with a per-radius loop**

Replace the block that currently reads (the Task-3/Task-4 edited lines):
```go
	m := BuildMatrix(targets, books, books, stations, names, categories, listings, catalogPrice)

	tabs := []radiusTab{{Label: "Local", File: "index.html", Active: true}}
	must(renderIndex(*outDir, "index.html", m, "Build Cost Matrix (Local)", tabs), "render index")
	for _, row := range m.Rows {
		must(renderDetail(*outDir, row, stations, targetByID[row.ID], itemNames, categories), "render detail "+row.ID)
	}
	log.Printf("build-costs: %d rows × %d stations → %s", len(m.Rows), len(stations), *outDir)
```
with:
```go
	// Four hop radii: 0 (local) = index.html, then +1/+2/+3.
	radiusFiles := []string{"index.html", "hop-1.html", "hop-2.html", "hop-3.html"}
	radiusLabels := []string{"Local", "+1", "+2", "+3"}
	headings := []string{"Build Cost Matrix (Local)", "Build Cost Matrix (+1 hop)", "Build Cost Matrix (+2 hops)", "Build Cost Matrix (+3 hops)"}

	matrices := make([]Matrix, 4)
	for radius := range 4 {
		costBooks := books
		if radius > 0 {
			costBooks = pooledBooksForRadius(books, hopDist, stations, radius)
		}
		matrices[radius] = BuildMatrix(targets, costBooks, books, stations, names, categories, listings, catalogPrice)
	}
	for radius := range 4 {
		tabs := make([]radiusTab, 4)
		for j := range 4 {
			tabs[j] = radiusTab{Label: radiusLabels[j], File: radiusFiles[j], Active: j == radius}
		}
		must(renderIndex(*outDir, radiusFiles[radius], matrices[radius], headings[radius], tabs), "render index "+radiusFiles[radius])
	}

	for _, row := range matrices[0].Rows {
		must(renderDetail(*outDir, row, stations, targetByID[row.ID], itemNames, categories), "render detail "+row.ID)
	}
	log.Printf("build-costs: %d rows × %d stations × 4 radii → %s", len(matrices[0].Rows), len(matrices[0].Stations), *outDir)
```

- [ ] **Step 3: Full gate**

Run: `go build ./... && go test ./... && golangci-lint run ./cmd/generate-build-costs/`
Expected: all clean.

- [ ] **Step 4: Smoke-test generation (real data)**

Run (from the repo/worktree root):
```bash
go run ./cmd/generate-build-costs \
  -crafting /home/robert/spacemolt/spacemolt/data/crafting.db \
  -market /home/robert/spacemolt/spacemolt/data/market.db \
  -knowledge /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db \
  -catalog /home/robert/spacemolt/spacemolt/data/game-api \
  -out kb/build-costs
```
Expected log: `… × 4 radii → kb/build-costs`. Then verify the four pages exist and feasibility is monotonically non-decreasing:
```bash
ls kb/build-costs/index.html kb/build-costs/hop-1.html kb/build-costs/hop-2.html kb/build-costs/hop-3.html
grep -c 'class="tabs"' kb/build-costs/hop-2.html   # expect 1
```
Spot-check that a locally-infeasible target gains feasibility at higher radius (open `hop-3.html`, confirm more green/feasible cells than `index.html` for the same row). Do NOT commit the generated site in this task — the controller regenerates and commits it after review.

- [ ] **Step 5: Commit (code only)**
```bash
git add cmd/generate-build-costs/main.go
git commit -m "feat(build-costs): render 4 hop-radius matrices (Local/+1/+2/+3)"
```

---

## Task 6: Detail-page per-radius cost columns

**Files:**
- Modify: `cmd/generate-build-costs/render.go` (`renderDetail` + `detailLine`)
- Modify: `cmd/generate-build-costs/templates/detail.html.tmpl`
- Modify: `cmd/generate-build-costs/main.go` (assemble per-radius rows, pass to renderDetail)
- Modify: `cmd/generate-build-costs/render_test.go` (extend the detail test)

**Interfaces:**
- Produces: `func renderDetail(outDir string, row MatrixRow, stations []StationMeta, tgt buildcost.Target, names, categories map[string]string, hopRows [4]MatrixRow) error`
- Consumes: `matrices []Matrix` (Task 5).

- [ ] **Step 1: Extend detailLine and renderDetail**

In `cmd/generate-build-costs/render.go`, add per-radius fields to `detailLine` (alongside the existing `BoMCostStr`/`RecipeCostStr` fields):
```go
	BoMHop        [4]string // BoM cost per radius 0..3 ("—" when infeasible)
	RecipeHop     [4]string // Recipe cost per radius 0..3 ("—" when infeasible/NA)
```
Change the `renderDetail` signature to accept `hopRows [4]MatrixRow`:
```go
func renderDetail(outDir string, row MatrixRow, stations []StationMeta, tgt buildcost.Target, names, categories map[string]string, hopRows [4]MatrixRow) error {
```
Inside the per-station loop, after constructing `ln`, populate the hop arrays from each radius's cell for this station (add a small helper `hopCostStr` defined below):
```go
		for r := range 4 {
			hc, ok := hopRows[r].Cells[s.ID]
			ln.BoMHop[r] = hopCostStr(hc.BoMCost, hc.BoMFeasible, ok)
			ln.RecipeHop[r] = hopCostStr(hc.RecipeCost, hc.RecipeFeasible && !hc.RecipeNA, ok && !hc.RecipeNA)
		}
```
Add this helper near `commaInt` in `render.go`:
```go
// hopCostStr formats a per-radius cost: the number when feasible, else an
// em-dash (also em-dash when the station has no cell at that radius).
func hopCostStr(cost float64, feasible, present bool) string {
	if !present || !feasible {
		return "—"
	}
	return commaInt(cost)
}
```

- [ ] **Step 2: Update the detail template**

In `cmd/generate-build-costs/templates/detail.html.tmpl`, the existing per-station table currently has a `<thead>` row and a `{{range .Lines}}` body. Wrap the `<table>` in a horizontally-scrollable div and replace the BoM/Recipe cost cells with the four per-radius columns.

Wrap the station table: put `<div class="scrollx">` immediately before the `<table>` that follows `<h2>Build cost by station</h2>` and `</div>` immediately after that `</table>`. Add the style to the `<style>` block:
```css
 .scrollx{overflow-x:auto}
```
Replace the station table's `<thead>` row with:
```html
<thead><tr><th>Station</th><th>Empire</th><th>BoM @0</th><th>BoM +1</th><th>BoM +2</th><th>BoM +3</th><th>Recipe @0</th><th>Recipe +1</th><th>Recipe +2</th><th>Recipe +3</th><th>Recipe used</th><th>Savings</th><th>Profit</th></tr></thead>
```
Replace the `{{range .Lines}}` row body with:
```html
{{range .Lines}}
<tr>
 <td>{{.StationName}}</td><td>{{.Empire}}</td>
 <td class="{{if not .BoMFeasible}}infeasible{{end}}">{{index .BoMHop 0}}</td>
 <td>{{index .BoMHop 1}}</td><td>{{index .BoMHop 2}}</td><td>{{index .BoMHop 3}}</td>
 <td class="{{if .RecipeNA}}infeasible{{else if not .RecipeFeasible}}infeasible{{end}}">{{index .RecipeHop 0}}</td>
 <td>{{index .RecipeHop 1}}</td><td>{{index .RecipeHop 2}}</td><td>{{index .RecipeHop 3}}</td>
 <td>{{.RecipeID}}</td>
 <td class="{{.SavingsClass}}">{{.SavingsStr}}</td>
 <td class="{{.ProfitClass}}">{{.ProfitStr}}</td>
</tr>
{{end}}
```
(The `@0` column shows today's local value; `+1/+2/+3` show pooled costs. The old single `BoMCostStr`/`RecipeCostStr`/`BoMCovered` cells are replaced by these; `.BoMCostStr` and `.RecipeCostStr` are no longer referenced by the template but remain populated on the struct — leave them, they feed nothing else, or you may drop them if the compiler/linter flags them as unused fields (struct fields are not flagged by `unused`, so leaving them is fine).)

Append a line to the existing Savings/Profit legend `<p class="legend">` (from the shipped feature) so the columns are explained:
```html
 <strong>@0</strong> is local; <strong>+1/+2/+3</strong> pool inputs from stations within that many jumps (cheapest sourcing; em-dash = still infeasible).
```

- [ ] **Step 3: Assemble and pass hopRows in main.go**

In `cmd/generate-build-costs/main.go`, replace the detail render loop (from Task 5):
```go
	for _, row := range matrices[0].Rows {
		must(renderDetail(*outDir, row, stations, targetByID[row.ID], itemNames, categories), "render detail "+row.ID)
	}
```
with a version that looks up each target's row at every radius:
```go
	rowByID := make([]map[string]MatrixRow, 4)
	for radius := range 4 {
		idx := make(map[string]MatrixRow, len(matrices[radius].Rows))
		for _, row := range matrices[radius].Rows {
			idx[row.ID] = row
		}
		rowByID[radius] = idx
	}
	for _, row := range matrices[0].Rows {
		var hopRows [4]MatrixRow
		for radius := range 4 {
			hopRows[radius] = rowByID[radius][row.ID]
		}
		must(renderDetail(*outDir, row, stations, targetByID[row.ID], itemNames, categories, hopRows), "render detail "+row.ID)
	}
```

- [ ] **Step 4: Extend the detail test**

In `cmd/generate-build-costs/render_test.go`, the existing `TestRenderDetail_BoMAndRecipeTables` and `TestRenderDetail_ShipNoRecipe` call `renderDetail(...)` with the old signature. Update BOTH call sites to pass a `[4]MatrixRow` whose radius-0 entry carries a feasible cell for the station, and assert the hop columns render. Concretely, for `TestRenderDetail_BoMAndRecipeTables`, before the call build:
```go
	base := MatrixRow{ID: "widget", Name: "Widget", Kind: "item", Cells: map[string]RowCell{
		"st1": {BoMCost: 100, BoMFeasible: true, RecipeCost: 120, RecipeFeasible: true},
	}}
	hopRows := [4]MatrixRow{base, base, base, base}
	stations := []StationMeta{{ID: "st1", Name: "Station One", Empire: "Independent"}}
```
Call `renderDetail(dir, base, stations, tgt, names, cats, hopRows)` (replace the existing `row`/`stations` args accordingly), then add an assertion that the output contains the local BoM value and the `BoM +1` header:
```go
	for _, want := range []string{"BoM +1", "Recipe +1", "100"} {
		if !strings.Contains(html, want) {
			t.Errorf("detail html missing %q", want)
		}
	}
```
For `TestRenderDetail_ShipNoRecipe`, pass `hopRows := [4]MatrixRow{row, row, row, row}` (reusing its existing `row`) as the final argument so it compiles; its existing assertions stand.

- [ ] **Step 5: Run detail tests**

Run: `go test ./cmd/generate-build-costs/ -run TestRenderDetail -v`
Expected: PASS (both detail tests).

- [ ] **Step 6: Full gate + commit**

Run: `go build ./... && go test ./... && golangci-lint run ./cmd/generate-build-costs/`
Expected: all clean.
```bash
git add cmd/generate-build-costs/render.go cmd/generate-build-costs/templates/detail.html.tmpl cmd/generate-build-costs/main.go cmd/generate-build-costs/render_test.go
git commit -m "feat(build-costs): per-radius cost columns on detail pages"
```

- [ ] **Step 7: Final regeneration + spot-check (controller step after review)**

After all task reviews pass, regenerate the full site and verify:
```bash
go run ./cmd/generate-build-costs \
  -crafting /home/robert/spacemolt/spacemolt/data/crafting.db \
  -market /home/robert/spacemolt/spacemolt/data/market.db \
  -knowledge /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db \
  -catalog /home/robert/spacemolt/spacemolt/data/game-api \
  -out kb/build-costs
```
- Confirm `index.html`, `hop-1.html`, `hop-2.html`, `hop-3.html` render with the tab bar.
- Pick a target that is infeasible locally but feasible within reach; confirm its detail page shows `—` at `@0` and a number at `+2` or `+3`.
- Commit the regenerated site:
```bash
git add kb/build-costs
git commit -m "chore(build-costs): regenerate site with +N-hop matrices and detail columns"
```

---

## Notes for the implementer

- The pure core `pkg/buildcost` must not change. Pooling is done by building a `buildcost.Book` and calling the existing `BuildCell`/`Walk`.
- gopls "not in workspace"/"BrokenImport"/"undefined" diagnostics are false positives when working in a git worktree — rely on `go build`/`go test`/`golangci-lint`, not editor diagnostics.
- `range 4` (range-over-int) requires Go 1.22+; the module targets 1.24, so it is fine and preferred over `for i := 0; i < 4; i++`.
