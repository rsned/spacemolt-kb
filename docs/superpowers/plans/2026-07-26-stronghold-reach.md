# Stronghold Reach Map Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Did-You-Know page that shows, for every system in the galaxy, how many jumps it lies from the nearest of the nine pirate strongholds — drawn as red territory blobs that grow and merge as a radius slider is dragged from 1 to 14.

**Architecture:** A multi-source breadth-first search from the nine `is_stronghold` systems assigns every system an *activation radius*. Because coverage is monotone (in reach at ≤6 implies in reach at ≤7), the blob geometry is emitted **once** with per-element `rb-<n>` activation classes and revealed by generated CSS keyed on a `data-r` attribute — so the page costs about one galaxy map, not fourteen. `pkg/galaxymap` gains one optional field to emit those classes; everything else lives in a new standalone generator.

**Tech Stack:** Go 1.25, `modernc.org/sqlite`, `html/template`, inline SVG, plain CSS and ~15 lines of vanilla JS. No new dependencies.

## Global Constraints

- Repo is `spacemolt-kb` at `/home/robert/spacemolt/kb`, module `github.com/rsned/spacemolt-kb`. **Not** the `spacemolt` repo.
- The knowledge DB is read-only input at `/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db`.
- Stronghold set comes **only** from `systems.is_stronghold = 1` (9 rows). Never use the station-name heuristic found in `cmd/generate-galaxy-map/main.go`.
- Coverage is cumulative (`≤R`), never the exactly-R ring.
- Slider range is 1–14, default 5. Radius 15 is not rendered.
- `galaxymap.Options` with `ReachBlob == nil` must produce byte-identical output to today. The existing tests in `pkg/galaxymap/galaxymap_test.go` are the regression guard — do not modify them.
- Code must pass `golangci-lint` without adding new findings.
- The repo has a large pre-existing dirty tree under `kb/build-costs/`. **Always `git add` explicit paths. Never `git add -A`.**
- Regenerating `kb/galaxy-map.html` is out of scope.

---

## File Structure

| File | Responsibility |
|---|---|
| `pkg/galaxymap/galaxymap.go` (modify) | Add `ReachBlob` type + `Options.ReachBlob`; emit `rb-<n>` classes on blob geometry |
| `pkg/galaxymap/galaxymap_test.go` (modify) | Add `ReachBlob` tests alongside existing ones |
| `cmd/generate-stronghold-reach/reach.go` | Pure multi-source BFS. No DB, no HTML |
| `cmd/generate-stronghold-reach/stats.go` | Per-radius coverage rows, component counting, territory table |
| `cmd/generate-stronghold-reach/css.go` | Generated reveal CSS |
| `cmd/generate-stronghold-reach/load.go` | The only DB-touching code |
| `cmd/generate-stronghold-reach/template.go` | Page HTML template |
| `cmd/generate-stronghold-reach/main.go` | Flags, orchestration, file write |
| `cmd/generate-stronghold-reach/*_test.go` | Table tests on synthetic graphs; no DB required |
| `kb/did_you_know/stronghold_reach.html` | Generated output |
| `kb/did_you_know/index.html` (modify) | Factoid card |
| `README.md` (modify) | Command table row |

---

### Task 1: `ReachBlob` option in `pkg/galaxymap`

**Files:**
- Modify: `pkg/galaxymap/galaxymap.go` (Options block at :33-47, blob block at :129-168)
- Test: `pkg/galaxymap/galaxymap_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces: `galaxymap.ReachBlob{Radius func(string) int; Max int; Color string}` and `galaxymap.Options.ReachBlob *ReachBlob`. Task 5 constructs this.

Blob elements carry `class="rb-<r>"` where `r` is the activation radius: for a circle, that system's radius; for an edge, `max` of its two endpoints' radii. A radius of `-1` means never in reach and emits nothing; a radius above `Max` emits nothing.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/galaxymap/galaxymap_test.go`:

```go
// reachSample returns a three-system chain sol(0) - vega(1) - rigel(2).
func reachSample() ([]*System, map[string]*System) {
	a := &System{
		ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0, IsStronghold: true,
		Connections: []Connection{{SystemID: "vega", Distance: 10}},
	}
	b := &System{
		ID: "vega", Name: "Vega", PositionX: 100, PositionY: 0,
		Connections: []Connection{{SystemID: "sol", Distance: 10}, {SystemID: "rigel", Distance: 10}},
	}
	c := &System{
		ID: "rigel", Name: "Rigel", PositionX: 200, PositionY: 0,
		Connections: []Connection{{SystemID: "vega", Distance: 10}},
	}
	return []*System{a, b, c}, map[string]*System{"sol": a, "vega": b, "rigel": c}
}

func reachRadius(m map[string]int) func(string) int {
	return func(id string) int {
		if r, ok := m[id]; ok {
			return r
		}
		return -1
	}
}

func TestReachBlobEmitsActivationClassPerSystem(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    2,
			Color:  "#e53e3e",
		},
	})

	if !strings.Contains(svg, "feGaussianBlur") {
		t.Errorf("ReachBlob should emit the metaball filter")
	}
	for _, want := range []string{`class="rb-0"`, `class="rb-1"`, `class="rb-2"`} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %s in:\n%s", want, svg)
		}
	}
	if !strings.Contains(svg, "#e53e3e") {
		t.Errorf("blob fill color not applied")
	}
}

func TestReachBlobEdgeUsesMaxOfEndpoints(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    2,
		},
	})

	// sol(0)-vega(1) activates at 1; vega(1)-rigel(2) activates at 2.
	// Circles contribute rb-0, rb-1, rb-2 once each, so each edge class
	// must appear exactly one time beyond its circle.
	if got := strings.Count(svg, `class="rb-1"`); got != 2 {
		t.Errorf("rb-1 count = %d, want 2 (one circle + one edge)", got)
	}
	if got := strings.Count(svg, `class="rb-2"`); got != 2 {
		t.Errorf("rb-2 count = %d, want 2 (one circle + one edge)", got)
	}
	if got := strings.Count(svg, `class="rb-0"`); got != 1 {
		t.Errorf("rb-0 count = %d, want 1 (circle only)", got)
	}
}

func TestReachBlobOmitsBeyondMax(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    1,
		},
	})

	if strings.Contains(svg, `class="rb-2"`) {
		t.Errorf("radius above Max should emit no blob geometry")
	}
	if !strings.Contains(svg, `class="rb-1"`) {
		t.Errorf("radius at Max should still be drawn")
	}
}

func TestReachBlobOmitsUnreachableSystems(t *testing.T) {
	explored, m := reachSample()
	// rigel is never in reach.
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1}),
			Max:    5,
		},
	})

	// Two circles (rb-0, rb-1) and one edge (rb-1). The vega-rigel edge
	// must be dropped because rigel is unreachable.
	if got := strings.Count(svg, `class="rb-`); got != 3 {
		t.Errorf("blob element count = %d, want 3", got)
	}
}

func TestReachBlobDoesNotRecolorSystemDots(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    2,
			Color:  "#e53e3e",
		},
	})

	// vega has no empire and is not a stronghold, so its dot keeps the
	// grey default rather than picking up the blob color.
	if !strings.Contains(svg, `fill="#E8E8E8"`) {
		t.Errorf("system dots should keep the grey default fill")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/robert/spacemolt/kb && go test ./pkg/galaxymap/ -run TestReachBlob -v
```

Expected: compile failure — `undefined: ReachBlob`.

- [ ] **Step 3: Add the type and the option field**

In `pkg/galaxymap/galaxymap.go`, add after the `Options` struct (which ends at :47):

```go
// ReachBlob configures a radius-layered metaball blob: geometry is emitted
// once, tagged with the radius at which it becomes active, so a page can
// reveal successive frames with CSS instead of re-rendering the map.
type ReachBlob struct {
	// Radius returns the activation radius for a system — the lowest
	// frame at which it is in reach — or -1 if it is never in reach.
	Radius func(systemID string) int
	// Max is the highest radius frame rendered. Geometry whose
	// activation radius exceeds Max is omitted entirely.
	Max int
	// Color is the blob fill. Empty means the default grey.
	Color string
}
```

Inside `Options`, add the field:

```go
	// ReachBlob, if non-nil, replaces the grey territory blob with a
	// per-radius blob whose geometry carries "rb-<n>" activation classes.
	// It takes precedence over ShowEmpireBlobs.
	ReachBlob *ReachBlob
```

- [ ] **Step 4: Emit activation classes in the blob block**

Change the guard at :129 from `if opt.ShowEmpireBlobs {` to:

```go
	if opt.ShowEmpireBlobs || opt.ReachBlob != nil {
```

Immediately after `blobR := 28.0` (:138), insert the three helpers. Note `fill` is a *separate* variable — `blobColor` is still read by the dot-color default and the unexplored-dot loop and must stay grey:

```go
		fill := blobColor
		if opt.ReachBlob != nil && opt.ReachBlob.Color != "" {
			fill = opt.ReachBlob.Color
		}

		// radiusOf reports a system's activation radius, or 0 when no
		// ReachBlob is configured (every element is always active).
		radiusOf := func(id string) int {
			if opt.ReachBlob == nil {
				return 0
			}
			return opt.ReachBlob.Radius(id)
		}

		// blobClass returns the class attribute for a blob element whose
		// endpoints have the given radii, and false if the element must
		// not be drawn at all.
		blobClass := func(radii ...int) (string, bool) {
			if opt.ReachBlob == nil {
				return "", true
			}
			r := 0
			for _, x := range radii {
				if x < 0 {
					return "", false
				}
				if x > r {
					r = x
				}
			}
			if r > opt.ReachBlob.Max {
				return "", false
			}
			return fmt.Sprintf(` class="rb-%d"`, r), true
		}
```

Replace the edge write — the `b.WriteString` inside the "Thick connection lines between explored systems" loop, whose format string starts `<line x1=` and ends `stroke-width="%.0f"/>` — with:

```go
				cls, ok := blobClass(radiusOf(s.ID), radiusOf(target.ID))
				if !ok {
					continue
				}
				b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.0f"%s/>`,
					tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY), fill, blobR*1.2, cls))
```

Replace the entire "Circles at each explored system position" loop (the last three lines before the closing `b.WriteString(`</g>`)`) with:

```go
		// Circles at each explored system position.
		for _, s := range explored {
			cls, ok := blobClass(radiusOf(s.ID))
			if !ok {
				continue
			}
			b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.0f" fill="%s"%s/>`,
				tx(s.PositionX), ty(s.PositionY), blobR, fill, cls))
		}
```

- [ ] **Step 5: Run the full package tests**

```bash
cd /home/robert/spacemolt/kb && go test ./pkg/galaxymap/ -v
```

Expected: PASS, including the pre-existing `TestRenderFullVariantHasBlobsAndConnections` and `TestRenderDotsOnlyVariantOmitsBlobsAndConnections`. If either pre-existing test fails, the nil-path was changed — revert and redo Step 4.

- [ ] **Step 6: Prove the nil path is byte-identical**

```bash
cd /home/robert/spacemolt/kb && go run ./cmd/generate-galaxy-map && git diff --stat kb/galaxy-map.html
```

Expected: **no diff** on `kb/galaxy-map.html`. A non-empty diff means the refactor changed existing behavior; fix before proceeding.

(If the file does show a diff purely because the DB snapshot advanced since it was last generated, `git checkout kb/galaxy-map.html` and instead verify by running the command twice and diffing the two outputs against each other.)

- [ ] **Step 7: Lint and commit**

```bash
cd /home/robert/spacemolt/kb && golangci-lint run ./pkg/galaxymap/
git add pkg/galaxymap/galaxymap.go pkg/galaxymap/galaxymap_test.go
git commit -m "feat(galaxymap): add radius-layered ReachBlob option"
```

---

### Task 2: Multi-source BFS

**Files:**
- Create: `cmd/generate-stronghold-reach/reach.go`
- Test: `cmd/generate-stronghold-reach/reach_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Edge{A, B string}`, `Reach{Dist map[string]int; Owner map[string]string; Max int}`, and `ComputeReach(edges []Edge, sources []string) Reach`. Tasks 3 and 5 consume all four.

Unreachable systems are **absent** from `Dist` and `Owner` — not stored with a sentinel. Ties for `Owner` break toward the lowest-sorting source ID.

- [ ] **Step 1: Write the failing tests**

Create `cmd/generate-stronghold-reach/reach_test.go`:

```go
package main

import "testing"

// chain returns edges for a-b-c-d-e.
func chain() []Edge {
	return []Edge{{"a", "b"}, {"b", "c"}, {"c", "d"}, {"d", "e"}}
}

func TestComputeReachSourcesAreDistanceZero(t *testing.T) {
	r := ComputeReach(chain(), []string{"a", "e"})

	if r.Dist["a"] != 0 || r.Dist["e"] != 0 {
		t.Errorf("sources must be distance 0, got a=%d e=%d", r.Dist["a"], r.Dist["e"])
	}
	if r.Owner["a"] != "a" || r.Owner["e"] != "e" {
		t.Errorf("sources must own themselves, got a=%q e=%q", r.Owner["a"], r.Owner["e"])
	}
}

func TestComputeReachDistancesAlongChain(t *testing.T) {
	r := ComputeReach(chain(), []string{"a"})

	want := map[string]int{"a": 0, "b": 1, "c": 2, "d": 3, "e": 4}
	for id, w := range want {
		if r.Dist[id] != w {
			t.Errorf("dist[%s] = %d, want %d", id, r.Dist[id], w)
		}
	}
	if r.Max != 4 {
		t.Errorf("Max = %d, want 4", r.Max)
	}
}

func TestComputeReachEdgesAreUndirected(t *testing.T) {
	// Only a->b is listed, but reaching a from b must still work.
	r := ComputeReach([]Edge{{"a", "b"}}, []string{"b"})

	if r.Dist["a"] != 1 {
		t.Errorf("dist[a] = %d, want 1 (edges are undirected)", r.Dist["a"])
	}
}

func TestComputeReachTieBrokenByLowestSourceID(t *testing.T) {
	// c is two hops from both a and e.
	r := ComputeReach(chain(), []string{"e", "a"})

	if r.Dist["c"] != 2 {
		t.Fatalf("dist[c] = %d, want 2", r.Dist["c"])
	}
	if r.Owner["c"] != "a" {
		t.Errorf("owner[c] = %q, want %q (ties break to the lowest source ID)", r.Owner["c"], "a")
	}
}

func TestComputeReachDisconnectedNodeIsAbsent(t *testing.T) {
	r := ComputeReach([]Edge{{"a", "b"}, {"y", "z"}}, []string{"a"})

	if _, ok := r.Dist["z"]; ok {
		t.Errorf("unreachable node must be absent from Dist")
	}
	if _, ok := r.Owner["z"]; ok {
		t.Errorf("unreachable node must be absent from Owner")
	}
	if r.Max != 1 {
		t.Errorf("Max = %d, want 1 (unreachable nodes must not inflate Max)", r.Max)
	}
}

func TestComputeReachNoSources(t *testing.T) {
	r := ComputeReach(chain(), nil)

	if len(r.Dist) != 0 {
		t.Errorf("no sources should reach nothing, got %d entries", len(r.Dist))
	}
	if r.Max != 0 {
		t.Errorf("Max = %d, want 0", r.Max)
	}
}

func TestComputeReachDuplicateSourceIsHarmless(t *testing.T) {
	r := ComputeReach(chain(), []string{"a", "a"})

	if r.Dist["b"] != 1 {
		t.Errorf("dist[b] = %d, want 1", r.Dist["b"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-stronghold-reach/ -v
```

Expected: compile failure — `undefined: Edge`, `undefined: ComputeReach`.

- [ ] **Step 3: Write the implementation**

Create `cmd/generate-stronghold-reach/reach.go`:

```go
package main

import "slices"

// Edge is an undirected jump-gate link between two systems.
type Edge struct {
	A, B string
}

// Reach is the result of a multi-source breadth-first search from the
// stronghold set.
type Reach struct {
	// Dist maps a system ID to the number of jumps to the nearest
	// source. Systems that cannot reach any source are absent.
	Dist map[string]int
	// Owner maps a system ID to the ID of its nearest source. Ties break
	// toward the lowest-sorting source ID.
	Owner map[string]string
	// Max is the largest value in Dist, or 0 when Dist is empty.
	Max int
}

// ComputeReach runs a multi-source breadth-first search from sources over
// the undirected graph formed by edges.
//
// Sources are seeded in sorted order and adjacency lists are sorted, so the
// result is deterministic and equidistant systems are owned by the
// lowest-sorting source.
func ComputeReach(edges []Edge, sources []string) Reach {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.A] = append(adj[e.A], e.B)
		adj[e.B] = append(adj[e.B], e.A)
	}
	for id := range adj {
		slices.Sort(adj[id])
		adj[id] = slices.Compact(adj[id])
	}

	r := Reach{Dist: make(map[string]int), Owner: make(map[string]string)}

	srcs := slices.Clone(sources)
	slices.Sort(srcs)
	queue := make([]string, 0, len(srcs))
	for _, s := range srcs {
		if _, seen := r.Dist[s]; seen {
			continue
		}
		r.Dist[s] = 0
		r.Owner[s] = s
		queue = append(queue, s)
	}

	// The queue grows while it is being walked, so this cannot be a range
	// loop: range would fix the bound at the initial length.
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		for _, v := range adj[u] {
			if _, seen := r.Dist[v]; seen {
				continue
			}
			r.Dist[v] = r.Dist[u] + 1
			r.Owner[v] = r.Owner[u]
			queue = append(queue, v)
		}
	}

	for _, d := range r.Dist {
		if d > r.Max {
			r.Max = d
		}
	}
	return r
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-stronghold-reach/ -v
```

Expected: PASS (7 tests).

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-stronghold-reach/reach.go cmd/generate-stronghold-reach/reach_test.go
git commit -m "feat(stronghold-reach): multi-source BFS over the jump network"
```

---

### Task 3: Coverage and territory statistics

**Files:**
- Create: `cmd/generate-stronghold-reach/stats.go`
- Test: `cmd/generate-stronghold-reach/stats_test.go`

**Interfaces:**
- Consumes: `Edge`, `Reach` from Task 2.
- Produces: `RadiusRow{Radius, Systems int; Percent float64; Blobs int; Merged bool}`, `TerritoryRow{SystemID, Name string; Systems int}`, `RadiusRows(r Reach, edges []Edge, total, maxRadius int) []RadiusRow`, `TerritoryRows(r Reach, names map[string]string) []TerritoryRow`, and `componentCount(edges []Edge, inSet map[string]bool) int`. Task 5 consumes the two exported constructors.

`RadiusRows` returns one row per radius from 1 to `maxRadius` inclusive. `Merged` is true when a row's `Blobs` is strictly lower than the previous row's; the first row is never `Merged`.

- [ ] **Step 1: Write the failing tests**

Create `cmd/generate-stronghold-reach/stats_test.go`:

```go
package main

import "testing"

// twoStars returns two disjoint 3-node chains: a-b-c and x-y-z, plus a
// bridge c-z that only closes at higher radius.
//
//	a - b - c - z - y - x
func twoStars() []Edge {
	return []Edge{{"a", "b"}, {"b", "c"}, {"c", "z"}, {"z", "y"}, {"y", "x"}}
}

func TestComponentCountCountsIsolatedNodes(t *testing.T) {
	// Only a and x are in the set, and they share no edge.
	got := componentCount(twoStars(), map[string]bool{"a": true, "x": true})
	if got != 2 {
		t.Errorf("componentCount = %d, want 2", got)
	}
}

func TestComponentCountJoinsAcrossEdges(t *testing.T) {
	got := componentCount(twoStars(), map[string]bool{"a": true, "b": true, "c": true})
	if got != 1 {
		t.Errorf("componentCount = %d, want 1", got)
	}
}

func TestComponentCountIgnoresEdgesLeavingTheSet(t *testing.T) {
	// c-z exists but z is out of the set, so it must not merge anything.
	got := componentCount(twoStars(), map[string]bool{"c": true, "x": true})
	if got != 2 {
		t.Errorf("componentCount = %d, want 2", got)
	}
}

func TestRadiusRowsCoverageAndBlobs(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	// dist: a=0 x=0 b=1 y=1 c=2 z=2
	rows := RadiusRows(r, edges, 6, 3)

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	want := []struct{ systems, blobs int }{
		{4, 2}, // R=1: a,b,x,y
		{6, 1}, // R=2: all six, c-z bridges them
		{6, 1}, // R=3: unchanged
	}
	for i, w := range want {
		if rows[i].Radius != i+1 {
			t.Errorf("rows[%d].Radius = %d, want %d", i, rows[i].Radius, i+1)
		}
		if rows[i].Systems != w.systems {
			t.Errorf("rows[%d].Systems = %d, want %d", i, rows[i].Systems, w.systems)
		}
		if rows[i].Blobs != w.blobs {
			t.Errorf("rows[%d].Blobs = %d, want %d", i, rows[i].Blobs, w.blobs)
		}
	}
}

func TestRadiusRowsPercentUsesTotal(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	rows := RadiusRows(r, edges, 8, 1)

	// 4 of 8 systems in reach at R=1.
	if rows[0].Percent != 50.0 {
		t.Errorf("Percent = %v, want 50", rows[0].Percent)
	}
}

func TestRadiusRowsMergedFlag(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	rows := RadiusRows(r, edges, 6, 3)

	if rows[0].Merged {
		t.Errorf("first row must never be Merged")
	}
	if !rows[1].Merged {
		t.Errorf("row at R=2 should be Merged (2 blobs -> 1)")
	}
	if rows[2].Merged {
		t.Errorf("row at R=3 should not be Merged (blob count unchanged)")
	}
}

func TestRadiusRowsBlobCountNeverIncreases(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	rows := RadiusRows(r, edges, 6, 5)

	for i := 1; i < len(rows); i++ {
		if rows[i].Blobs > rows[i-1].Blobs {
			t.Errorf("blob count rose from %d to %d at R=%d",
				rows[i-1].Blobs, rows[i].Blobs, rows[i].Radius)
		}
	}
}

func TestRadiusRowsZeroTotalDoesNotDivideByZero(t *testing.T) {
	rows := RadiusRows(Reach{Dist: map[string]int{}}, nil, 0, 2)

	for _, row := range rows {
		if row.Percent != 0 {
			t.Errorf("Percent = %v, want 0 when total is 0", row.Percent)
		}
	}
}

func TestTerritoryRowsSortedByCountThenName(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	// a owns a,b,c ; x owns x,y,z
	rows := TerritoryRows(r, map[string]string{"a": "Alpha", "x": "Xray"})

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	// Equal counts (3 each), so name ascending: Alpha before Xray.
	if rows[0].Name != "Alpha" || rows[1].Name != "Xray" {
		t.Errorf("got %q,%q; want Alpha,Xray", rows[0].Name, rows[1].Name)
	}
	if rows[0].Systems != 3 || rows[1].Systems != 3 {
		t.Errorf("counts = %d,%d; want 3,3", rows[0].Systems, rows[1].Systems)
	}
}

func TestTerritoryRowsLargestFirst(t *testing.T) {
	// a reaches a,b,c,z,y,x when it is the only source.
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	// Force an imbalance by making x own nothing but itself.
	r.Owner["y"] = "a"
	r.Owner["z"] = "a"
	rows := TerritoryRows(r, map[string]string{"a": "Alpha", "x": "Xray"})

	if rows[0].Name != "Alpha" || rows[0].Systems != 5 {
		t.Errorf("rows[0] = %+v, want Alpha with 5", rows[0])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-stronghold-reach/ -run 'TestRadiusRows|TestTerritoryRows|TestComponentCount' -v
```

Expected: compile failure — `undefined: componentCount`, `undefined: RadiusRows`, `undefined: TerritoryRows`.

- [ ] **Step 3: Write the implementation**

Create `cmd/generate-stronghold-reach/stats.go`:

```go
package main

import (
	"cmp"
	"slices"
)

// RadiusRow describes the galaxy when stronghold reach is drawn out to
// Radius jumps.
type RadiusRow struct {
	Radius  int
	Systems int
	Percent float64
	Blobs   int
	// Merged is true when this row has strictly fewer blobs than the
	// previous one, i.e. territories joined at this radius.
	Merged bool
}

// TerritoryRow is one row of the nearest-stronghold table.
type TerritoryRow struct {
	SystemID string
	Name     string
	Systems  int
}

// componentCount returns the number of connected components formed by the
// members of inSet, counting a member with no in-set edge as its own
// component.
func componentCount(edges []Edge, inSet map[string]bool) int {
	parent := make(map[string]string, len(inSet))
	for id := range inSet {
		parent[id] = id
	}
	var find func(string) string
	find = func(x string) string {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}

	n := len(inSet)
	for _, e := range edges {
		if !inSet[e.A] || !inSet[e.B] {
			continue
		}
		ra, rb := find(e.A), find(e.B)
		if ra != rb {
			parent[ra] = rb
			n--
		}
	}
	return n
}

// RadiusRows builds one row per radius from 1 to maxRadius inclusive.
func RadiusRows(r Reach, edges []Edge, total, maxRadius int) []RadiusRow {
	rows := make([]RadiusRow, 0, maxRadius)
	prevBlobs := -1
	for radius := 1; radius <= maxRadius; radius++ {
		inSet := make(map[string]bool)
		for id, d := range r.Dist {
			if d <= radius {
				inSet[id] = true
			}
		}
		pct := 0.0
		if total > 0 {
			pct = 100.0 * float64(len(inSet)) / float64(total)
		}
		blobs := componentCount(edges, inSet)
		rows = append(rows, RadiusRow{
			Radius:  radius,
			Systems: len(inSet),
			Percent: pct,
			Blobs:   blobs,
			Merged:  prevBlobs >= 0 && blobs < prevBlobs,
		})
		prevBlobs = blobs
	}
	return rows
}

// TerritoryRows counts how many systems each source is nearest to, largest
// first, ties broken by name.
func TerritoryRows(r Reach, names map[string]string) []TerritoryRow {
	counts := make(map[string]int)
	for _, owner := range r.Owner {
		counts[owner]++
	}

	rows := make([]TerritoryRow, 0, len(counts))
	for id, n := range counts {
		name := names[id]
		if name == "" {
			name = id
		}
		rows = append(rows, TerritoryRow{SystemID: id, Name: name, Systems: n})
	}
	slices.SortFunc(rows, func(a, b TerritoryRow) int {
		if c := cmp.Compare(b.Systems, a.Systems); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return rows
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-stronghold-reach/ -v
```

Expected: PASS (all tests from Tasks 2 and 3).

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-stronghold-reach/stats.go cmd/generate-stronghold-reach/stats_test.go
git commit -m "feat(stronghold-reach): coverage rows, blob counting, territory table"
```

---

### Task 4: Generated reveal CSS

**Files:**
- Create: `cmd/generate-stronghold-reach/css.go`
- Test: `cmd/generate-stronghold-reach/css_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `ReachCSS(maxRadius int) string`. Task 5 injects the result into the page template.

**Radius 0 is deliberately excluded from every per-frame rule.** The nine strongholds are in reach at every frame, so `rb-0` blob geometry is never hidden and `sr-0` dots get a single static rule. Emitting frame rules for radius 0 would be dead CSS.

Dot fills are set by CSS rather than the SVG `fill` attribute on purpose: presentation attributes lose to any CSS rule, so the generated rules reliably override the empire colors `galaxymap` writes inline.

- [ ] **Step 1: Write the failing tests**

Create `cmd/generate-stronghold-reach/css_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestReachCSSHidesEveryFrameByDefault(t *testing.T) {
	css := ReachCSS(3)

	for _, want := range []string{"#reach-map .rb-1", "#reach-map .rb-2", "#reach-map .rb-3"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing base hide selector %q", want)
		}
	}
	if !strings.Contains(css, "display:none") {
		t.Errorf("base rule should hide frames")
	}
}

func TestReachCSSRevealIsCumulative(t *testing.T) {
	css := ReachCSS(4)

	// The data-r="3" block must list rb-1 through rb-3 and stop there.
	for _, want := range []string{
		`#reach-map[data-r="3"] .rb-1`,
		`#reach-map[data-r="3"] .rb-2`,
		`#reach-map[data-r="3"] .rb-3`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("missing cumulative selector %q", want)
		}
	}
	if strings.Contains(css, `#reach-map[data-r="3"] .rb-4`) {
		t.Errorf("frame 3 must not reveal frame 4")
	}
}

func TestReachCSSOmitsRadiusZeroFrameRules(t *testing.T) {
	css := ReachCSS(3)

	if strings.Contains(css, ".rb-0") {
		t.Errorf("radius 0 blob geometry is always visible and needs no rule")
	}
	if strings.Contains(css, `[data-r="0"]`) {
		t.Errorf("there is no radius 0 frame")
	}
}

func TestReachCSSHasStaticStrongholdDotRule(t *testing.T) {
	css := ReachCSS(3)

	if !strings.Contains(css, "#reach-map .sr-0") {
		t.Errorf("strongholds need an always-on dot rule")
	}
	if strings.Contains(css, `[data-r="2"] .sr-0`) {
		t.Errorf("sr-0 must not appear in per-frame rules")
	}
}

func TestReachCSSBrightensInReachDots(t *testing.T) {
	css := ReachCSS(2)

	if !strings.Contains(css, `#reach-map[data-r="2"] .sr-1`) {
		t.Errorf("in-reach dots should be brightened per frame")
	}
}

func TestReachCSSZeroMaxProducesNoFrameRules(t *testing.T) {
	css := ReachCSS(0)

	if strings.Contains(css, "[data-r=") {
		t.Errorf("no frames means no frame rules, got:\n%s", css)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-stronghold-reach/ -run TestReachCSS -v
```

Expected: compile failure — `undefined: ReachCSS`.

- [ ] **Step 3: Write the implementation**

Create `cmd/generate-stronghold-reach/css.go`:

```go
package main

import (
	"fmt"
	"strings"
)

// Reach map palette.
const (
	dotDim         = "#3a4557"
	dotInReach     = "#f0f4f8"
	dotStronghold  = "#ff2d2d"
)

// ReachCSS generates the frame-reveal rules for radii 1..maxRadius.
//
// Radius 0 is excluded on purpose: the strongholds are in reach at every
// frame, so their rb-0 geometry is never hidden and their sr-0 dots get a
// single static rule instead of one per frame.
func ReachCSS(maxRadius int) string {
	var b strings.Builder

	// Dots default to dim; strongholds are always red.
	fmt.Fprintf(&b, "#reach-map .galaxy-sys-dot{fill:%s;}\n", dotDim)
	fmt.Fprintf(&b, "#reach-map .sr-0{fill:%s;}\n", dotStronghold)

	if maxRadius < 1 {
		return b.String()
	}

	// Every frame hidden by default.
	hide := make([]string, 0, maxRadius)
	for r := 1; r <= maxRadius; r++ {
		hide = append(hide, fmt.Sprintf("#reach-map .rb-%d", r))
	}
	fmt.Fprintf(&b, "%s{display:none;}\n", strings.Join(hide, ","))

	// Cumulative reveal per frame.
	for frame := 1; frame <= maxRadius; frame++ {
		blob := make([]string, 0, frame)
		dots := make([]string, 0, frame)
		for r := 1; r <= frame; r++ {
			blob = append(blob, fmt.Sprintf(`#reach-map[data-r="%d"] .rb-%d`, frame, r))
			dots = append(dots, fmt.Sprintf(`#reach-map[data-r="%d"] .sr-%d`, frame, r))
		}
		fmt.Fprintf(&b, "%s{display:inline;}\n", strings.Join(blob, ","))
		fmt.Fprintf(&b, "%s{fill:%s;}\n", strings.Join(dots, ","), dotInReach)
	}

	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-stronghold-reach/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-stronghold-reach/css.go cmd/generate-stronghold-reach/css_test.go
git commit -m "feat(stronghold-reach): generated CSS frame-reveal rules"
```

---

### Task 5: Loader, template, and page generation

**Files:**
- Create: `cmd/generate-stronghold-reach/load.go`
- Create: `cmd/generate-stronghold-reach/template.go`
- Create: `cmd/generate-stronghold-reach/main.go`
- Create: `kb/did_you_know/stronghold_reach.html` (generated output)

**Interfaces:**
- Consumes: `ComputeReach`, `Reach`, `Edge` (Task 2); `RadiusRows`, `TerritoryRows` (Task 3); `ReachCSS` (Task 4); `galaxymap.Render`, `galaxymap.Options`, `galaxymap.ReachBlob` (Task 1).
- Produces: the generated page. Nothing downstream consumes Go symbols from here.

Three things that will silently produce a wrong page if missed:

1. **Pass all systems as `explored`, with `nil` for `unexplored`.** `galaxymap.Render` only draws blob geometry and classed dots for the `explored` slice. Splitting on `LastUpdatedTick > 0` the way `generate-galaxy-map` does would drop ~127 systems from the map.
2. **`LinkPrefix` must be `"../"`.** The page lives in `kb/did_you_know/`, one level below `kb/systems/`.
3. **A stronghold count of zero is fatal**, not an empty map.

- [ ] **Step 1: Write the loader**

Create `cmd/generate-stronghold-reach/load.go`:

```go
package main

import (
	"cmp"
	"database/sql"
	"slices"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// loadGalaxy reads every system, its jump-gate connections, and the
// stronghold set from the knowledge database.
//
// It returns the systems in name order, the undirected edge list, and the
// IDs of systems flagged is_stronghold.
func loadGalaxy(db *sql.DB) ([]*galaxymap.System, []Edge, []string, error) {
	rows, err := db.Query(`SELECT id, name, position_x, position_y, police_level,
		COALESCE(empire,''), is_stronghold, last_updated_tick
		FROM systems ORDER BY name`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	byID := make(map[string]*galaxymap.System)
	var systems []*galaxymap.System
	var sources []string
	for rows.Next() {
		var s galaxymap.System
		if err := rows.Scan(&s.ID, &s.Name, &s.PositionX, &s.PositionY,
			&s.PoliceLevel, &s.Empire, &s.IsStronghold, &s.LastUpdatedTick); err != nil {
			return nil, nil, nil, err
		}
		if s.ID == "" {
			continue
		}
		byID[s.ID] = &s
		systems = append(systems, &s)
		if s.IsStronghold {
			sources = append(sources, s.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}

	connRows, err := db.Query(`SELECT from_system, to_system, distance
		FROM connections ORDER BY from_system, to_system`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = connRows.Close() }()

	var edges []Edge
	seen := make(map[string]bool)
	for connRows.Next() {
		var fromID, toID string
		var distance int
		if err := connRows.Scan(&fromID, &toID, &distance); err != nil {
			return nil, nil, nil, err
		}
		from, okFrom := byID[fromID]
		to, okTo := byID[toID]
		if !okFrom || !okTo {
			continue
		}
		from.Connections = append(from.Connections, galaxymap.Connection{
			SystemID: toID, Name: to.Name, Distance: distance,
		})
		// Collapse the directed rows into one undirected edge.
		key := fromID + "|" + toID
		rev := toID + "|" + fromID
		if !seen[key] && !seen[rev] {
			seen[key] = true
			edges = append(edges, Edge{A: fromID, B: toID})
		}
	}
	if err := connRows.Err(); err != nil {
		return nil, nil, nil, err
	}

	for _, s := range systems {
		slices.SortFunc(s.Connections, func(a, b galaxymap.Connection) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}
	slices.Sort(sources)

	return systems, edges, sources, nil
}
```

- [ ] **Step 2: Write the page template**

Create `cmd/generate-stronghold-reach/template.go`:

```go
package main

const pageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Reach of the Nine Strongholds - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../items/items.css">
    <style>
        #reach-map { background:#0a0e1a; border:1px solid var(--border); border-radius:8px; overflow:hidden; }
        #reach-map .galaxy-map-svg { width:100%; height:auto; display:block; }
        .reach-controls { display:flex; align-items:center; gap:12px; margin:16px 0 8px; flex-wrap:wrap; }
        .reach-controls input[type=range] { flex:1; min-width:240px; }
        .reach-controls button { padding:4px 12px; cursor:pointer; }
        .reach-readout { font-weight:bold; font-size:1.05em; margin:0 0 12px; }
        .stat-hero { background:var(--bg-card); border:1px solid var(--border); border-left:4px solid #e53e3e;
                     border-radius:8px; padding:20px; margin:20px 0; }
        .stat-hero .value { font-size:2em; font-weight:bold; color:#e53e3e; }
        table.reach td.merge { color:#e53e3e; font-weight:bold; }
{{.ReachCSS}}
    </style>
</head>
<body>
    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../">Home</a>
            <a href="../systems/index.html">Systems</a>
            <a href="../items/index.html">Items</a>
            <a href="../recipes/index.html">Recipes</a>
            <a href="../skills/index.html">Skills</a>
            <a href="../ships/index.html">Ships</a>
            <a href="../facilities/index.html">Facilities</a>
            <a href="../resources/index.html">Resources</a>
            <a href="../missions/index.html">Missions</a>
            <a href="./">Did You Know?</a>
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>
        </nav>
    </header>
    <main class="container page-content">
        <h2>Reach of the Nine Strongholds</h2>
        <p class="text-muted mt-1">Nobody in this galaxy lives very far from a pirate stronghold. A multi-source breadth-first search over the jump-gate network measures every system's distance to the nearest of the nine, then grows that reach one jump at a time.</p>

        <div class="stat-hero">
            <div class="value">{{.MaxRadius}} jumps</div>
            <div class="label">is all it takes to reach every one of the {{.TotalSystems}} known systems from the nearest stronghold</div>
        </div>

        <div class="reach-controls">
            <button id="prev" type="button">&larr;</button>
            <input type="range" id="radius" min="1" max="{{.MaxRadius}}" value="{{.DefaultRadius}}" step="1">
            <button id="next" type="button">&rarr;</button>
        </div>
        <p class="reach-readout" id="readout"></p>

        <div id="reach-map" data-r="{{.DefaultRadius}}">{{.MapSVG}}</div>
        <p class="text-muted mt-1">Red territory marks systems within the selected number of jumps. Dim stars are out of reach at this radius. The nine strongholds are the bright red dots.</p>

        <h3 class="mt-3">Coverage at Every Radius</h3>
        <table class="reach">
            <thead><tr><th>Jumps</th><th>Systems in reach</th><th>% of galaxy</th><th>Separate blobs</th><th></th></tr></thead>
            <tbody>
            {{range .Rows}}
                <tr>
                    <td>&le;{{.Radius}}</td>
                    <td>{{.Systems}}</td>
                    <td>{{printf "%.1f" .Percent}}%</td>
                    <td>{{.Blobs}}</td>
                    <td class="merge">{{if .Merged}}merge{{end}}</td>
                </tr>
            {{end}}
            </tbody>
        </table>

        <h3 class="mt-3">Which Stronghold Is Nearest?</h3>
        <p class="text-muted">Assigning every system to the stronghold it can reach in the fewest jumps carves the galaxy into nine territories.</p>
        <table class="reach">
            <thead><tr><th>Stronghold</th><th>Systems nearest to it</th></tr></thead>
            <tbody>
            {{range .Territory}}
                <tr><td><a href="../systems/{{.SystemID}}/">{{.Name}}</a></td><td>{{.Systems}}</td></tr>
            {{end}}
            </tbody>
        </table>

        <h3 class="mt-3">Analysis Notes</h3>
        <ul>
            <li><strong>Nine blobs become one.</strong> Each patch of reach always contains a stronghold, so the count can only fall: {{.MergeStory}}.</li>
            <li><strong>{{.TopTerritory}} dominates.</strong> It is the nearest stronghold for {{.TopTerritoryCount}} systems, far more than any other.</li>
            <li><strong>The last holdouts.</strong> {{.FarthestNames}} sit {{.MaxRadius}} jumps out — the deepest anyone in this galaxy gets from pirate territory.</li>
            {{if .UnreachableCount}}<li><strong>{{.UnreachableCount}} systems have no route to any stronghold</strong> and are drawn permanently dim.</li>{{end}}
        </ul>

        <h3 class="mt-3">Data Source</h3>
        <p class="text-muted">System positions, jump-gate connections, and stronghold flags come from the knowledge database ({{.TotalSystems}} systems, {{.EdgeCount}} jump gates). The nine strongholds are the systems flagged <code>is_stronghold</code>; all nine are neutral, with no empire and no police presence. Distance is the minimum number of jump-gate traversals to the nearest stronghold via a multi-source breadth-first search.</p>

        <p class="text-muted mt-3"><a href="./">&larr; Back to Did You Know?</a></p>
    </main>
    <script>
    (function() {
        var toggle = document.getElementById('theme-toggle');
        var root = document.documentElement;
        var stored = localStorage.getItem('theme');
        if (stored === 'dark') root.classList.add('dark');
        toggle.addEventListener('click', function() {
            root.classList.toggle('dark');
            localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
        });
    })();
    (function() {
        var stats = {{.StatsJSON}};
        var map = document.getElementById('reach-map');
        var slider = document.getElementById('radius');
        var readout = document.getElementById('readout');
        function apply() {
            var r = parseInt(slider.value, 10);
            map.setAttribute('data-r', r);
            var s = stats[r];
            if (!s) { return; }
            readout.textContent = '≤' + r + ' jumps · ' + s.systems +
                ' systems · ' + s.percent + '% of the galaxy · ' +
                s.blobs + (s.blobs === 1 ? ' single blob' : ' separate blobs');
        }
        slider.addEventListener('input', apply);
        document.getElementById('prev').addEventListener('click', function() {
            slider.value = Math.max(parseInt(slider.min, 10), parseInt(slider.value, 10) - 1);
            apply();
        });
        document.getElementById('next').addEventListener('click', function() {
            slider.value = Math.min(parseInt(slider.max, 10), parseInt(slider.value, 10) + 1);
            apply();
        });
        apply();
    })();
    </script>
</body>
</html>
`
```

- [ ] **Step 3: Write main.go**

Create `cmd/generate-stronghold-reach/main.go`:

```go
// Command generate-stronghold-reach renders the "Reach of the Nine
// Strongholds" Did-You-Know page: a galaxy map whose red territory blobs
// grow one jump at a time from the galaxy's pirate strongholds.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// defaultRadius is the frame the page opens on.
const defaultRadius = 5

func main() {
	dbPath := flag.String("db", "/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db",
		"path to the knowledge database")
	out := flag.String("out", "kb/did_you_know/stronghold_reach.html", "output HTML path")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	systems, edges, sources, err := loadGalaxy(db)
	if err != nil {
		log.Fatalf("load galaxy: %v", err)
	}
	if len(sources) == 0 {
		log.Fatalf("no systems flagged is_stronghold; nothing to measure reach from")
	}

	reach := ComputeReach(edges, sources)
	if unreachable := len(systems) - len(reach.Dist); unreachable > 0 {
		log.Printf("warning: %d systems have no route to any stronghold", unreachable)
	}

	names := make(map[string]string, len(systems))
	for _, s := range systems {
		names[s.ID] = s.Name
	}

	rows := RadiusRows(reach, edges, len(systems), reach.Max)
	territory := TerritoryRows(reach, names)

	// All systems are passed as "explored" so every one gets a dot and
	// blob geometry; the reach map has no explored/unexplored split.
	svg := galaxymap.Render(systems, nil, systemIndex(systems), galaxymap.Options{
		ShowConnections: true,
		LinkPrefix:      "../",
		HighlightClasses: func(id string) []string {
			d, ok := reach.Dist[id]
			if !ok {
				return nil
			}
			return []string{fmt.Sprintf("sr-%d", d)}
		},
		ReachBlob: &galaxymap.ReachBlob{
			Radius: func(id string) int {
				if d, ok := reach.Dist[id]; ok {
					return d
				}
				return -1
			},
			Max:   reach.Max,
			Color: "#c53030",
		},
	})

	statsJSON, err := json.Marshal(statsByRadius(rows))
	if err != nil {
		log.Fatalf("marshal stats: %v", err)
	}

	data := struct {
		TotalSystems      int
		EdgeCount         int
		MaxRadius         int
		DefaultRadius     int
		Rows              []RadiusRow
		Territory         []TerritoryRow
		MergeStory        string
		TopTerritory      string
		TopTerritoryCount int
		FarthestNames     string
		UnreachableCount  int
		ReachCSS          template.CSS
		MapSVG            template.HTML
		StatsJSON         template.JS
	}{
		TotalSystems:     len(systems),
		EdgeCount:        len(edges),
		MaxRadius:        reach.Max,
		DefaultRadius:    min(defaultRadius, reach.Max),
		Rows:             rows,
		Territory:        territory,
		MergeStory:       mergeStory(rows),
		FarthestNames:    farthestNames(reach, names),
		UnreachableCount: len(systems) - len(reach.Dist),
		ReachCSS:         template.CSS(ReachCSS(reach.Max)),
		MapSVG:           template.HTML(svg),
		StatsJSON:        template.JS(statsJSON),
	}
	if len(territory) > 0 {
		data.TopTerritory = territory[0].Name
		data.TopTerritoryCount = territory[0].Systems
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create output file: %v", err)
	}
	defer func() { _ = f.Close() }()

	tmpl := template.Must(template.New("stronghold-reach").Parse(pageTemplate))
	if err := tmpl.Execute(f, data); err != nil {
		log.Fatalf("render page: %v", err)
	}

	fmt.Printf("Generated %s (%d systems, max radius %d)\n", *out, len(systems), reach.Max)
}

// systemIndex builds the ID lookup galaxymap.Render needs to resolve
// connection endpoints.
func systemIndex(systems []*galaxymap.System) map[string]*galaxymap.System {
	m := make(map[string]*galaxymap.System, len(systems))
	for _, s := range systems {
		m[s.ID] = s
	}
	return m
}

// radiusStat is the per-frame readout payload handed to the page script.
type radiusStat struct {
	Systems int    `json:"systems"`
	Percent string `json:"percent"`
	Blobs   int    `json:"blobs"`
}

// statsByRadius keys the readout payload by radius for direct lookup in JS.
func statsByRadius(rows []RadiusRow) map[string]radiusStat {
	m := make(map[string]radiusStat, len(rows))
	for _, r := range rows {
		m[fmt.Sprintf("%d", r.Radius)] = radiusStat{
			Systems: r.Systems,
			Percent: fmt.Sprintf("%.1f", r.Percent),
			Blobs:   r.Blobs,
		}
	}
	return m
}

// mergeStory renders the blob-count sequence as prose, e.g.
// "9 at 3 jumps, 8 at 4, 6 at 6, 4 at 7, 2 at 8, and a single blob at 9".
func mergeStory(rows []RadiusRow) string {
	var parts []string
	prev := -1
	for _, r := range rows {
		if r.Blobs == prev {
			continue
		}
		prev = r.Blobs
		if r.Blobs == 1 {
			parts = append(parts, fmt.Sprintf("a single blob at %d", r.Radius))
			break
		}
		parts = append(parts, fmt.Sprintf("%d at %d", r.Blobs, r.Radius))
	}
	return strings.Join(parts, ", ")
}

// farthestNames lists the systems sitting at the maximum reach distance.
func farthestNames(r Reach, names map[string]string) string {
	var out []string
	for id, d := range r.Dist {
		if d == r.Max {
			n := names[id]
			if n == "" {
				n = id
			}
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return strings.Join(out, ", ")
}
```

- [ ] **Step 4: Build and generate the page**

```bash
cd /home/robert/spacemolt/kb
go build ./... && go vet ./cmd/generate-stronghold-reach/
go run ./cmd/generate-stronghold-reach
```

Expected output: `Generated kb/did_you_know/stronghold_reach.html (505 systems, max radius 14)` with **no** unreachable-systems warning.

- [ ] **Step 5: Verify the generated page against the known numbers**

```bash
cd /home/robert/spacemolt/kb
# Coverage table: 14 rows, ending at 505 systems / 100.0%.
grep -o '<td>&le;[0-9]*</td>' kb/did_you_know/stronghold_reach.html | wc -l
# Blob geometry is emitted once, not per frame: ~505 circles + ~1065 edges.
grep -o 'class="rb-[0-9]*"' kb/did_you_know/stronghold_reach.html | wc -l
# All nine strongholds present as sr-0 dots.
grep -o 'sr-0' kb/did_you_know/stronghold_reach.html | wc -l
```

Expected: 14 table rows; roughly 1570 `rb-` elements (well under 2× the system count — if it is ~10×, frames are being duplicated and Task 1 was implemented wrong); at least 9 `sr-0` occurrences (9 dots plus the CSS rules).

Confirm the headline sequence appears in the coverage table: systems 18/41/72/114/171/235/285/335/389/430/461/486/500/505 and blobs 9/9/9/8/8/6/4/2/1/1/1/1/1/1.

- [ ] **Step 6: Eyeball the page in a browser**

Open `file:///home/robert/spacemolt/kb/kb/did_you_know/stronghold_reach.html`. Confirm: the slider opens at 5, dragging it grows red territory smoothly, the readout updates, arrows step, the theme toggle works, and the nine strongholds show as bright red dots.

- [ ] **Step 7: Lint and commit**

```bash
cd /home/robert/spacemolt/kb
golangci-lint run ./cmd/generate-stronghold-reach/
go test ./... 
git add cmd/generate-stronghold-reach/load.go cmd/generate-stronghold-reach/template.go \
        cmd/generate-stronghold-reach/main.go kb/did_you_know/stronghold_reach.html
git commit -m "feat(kb): add the stronghold reach did-you-know page"
```

---

### Task 6: Link the page into the site

**Files:**
- Modify: `kb/did_you_know/index.html` (factoid grid)
- Modify: `README.md` (command table)

**Interfaces:**
- Consumes: `kb/did_you_know/stronghold_reach.html` from Task 5.
- Produces: nothing.

- [ ] **Step 1: Add the factoid card**

In `kb/did_you_know/index.html`, inside `<div class="factoid-grid">`, add as the first card (it is the newest and the most visual):

```html
            <a href="stronghold_reach.html" class="factoid-card">
                <h3>Reach of the Nine Strongholds</h3>
                <p>Every known system lies within 14 jumps of a pirate stronghold. Drag a radius slider and watch nine isolated red territories grow and merge into one galaxy-spanning blob.</p>
                <div class="meta">
                    <span>🏴 Stronghold Analysis</span>
                    <span>🗺️ Interactive Map</span>
                </div>
            </a>
```

- [ ] **Step 2: Add the README row**

In `README.md`, in the command table, add after the `generate-galaxy-map` row:

```markdown
| `generate-stronghold-reach` | Renders the "Reach of the Nine Strongholds" Did-You-Know page: multi-source BFS from the nine pirate strongholds, drawn as radius-layered territory blobs with a slider. | `-db`, `-out` |
```

- [ ] **Step 3: Verify the link works**

Open `file:///home/robert/spacemolt/kb/kb/did_you_know/index.html` and click the new card. It must load the reach page.

- [ ] **Step 4: Commit**

```bash
cd /home/robert/spacemolt/kb
git add kb/did_you_know/index.html README.md
git commit -m "docs(kb): link the stronghold reach page from the index and README"
```

---

## Verification Checklist

Run from `/home/robert/spacemolt/kb`:

- [ ] `go build ./...` clean
- [ ] `go test ./...` passes, including the untouched `pkg/galaxymap` regression tests
- [ ] `golangci-lint run ./pkg/galaxymap/ ./cmd/generate-stronghold-reach/` adds no findings
- [ ] `go run ./cmd/generate-galaxy-map` leaves `kb/galaxy-map.html` byte-identical (Task 1, Step 6)
- [ ] `go run ./cmd/generate-stronghold-reach` reports 505 systems, max radius 14, no unreachable warning
- [ ] Blob-count sequence on the page reads 9/9/9/8/8/6/4/2/1/1/1/1/1/1
- [ ] Slider opens at 5, spans 1–14, and the readout tracks it
- [ ] `git status --short` shows no unintended `kb/build-costs/` files staged
