# Resource Galaxy Map Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On the KB Resources page, show a galaxy map that highlights which systems contain the currently selected resource.

**Architecture:** Extract the existing `renderGalaxyMap()` from `cmd/generate-galaxy-map` (package main) into a reusable `pkg/galaxymap` with an `Options` struct. The Resources page renders a second, lighter instance of that map (dots only, no empire blobs, no connection lines) where each system dot carries a CSS class per resource it contains. A `data-active` attribute on the map container plus one generated CSS rule per resource does the highlighting; ~15 lines of vanilla JS keep the attribute in sync with a `<select>` and the page's existing `#anchor` navigation.

**Tech Stack:** Go 1.25.0, `html/template`, `strings.Builder`-generated SVG, `modernc.org/sqlite`. No frontend framework, no bundler, no CDN — matches the existing KB.

**Spec:** `docs/superpowers/specs/2026-07-20-resource-galaxy-map-design.md`

## Global Constraints

- Module is `github.com/rsned/spacemolt-kb`; Go directive `go 1.25.0`. Use modern Go (range-over-int, `b.Loop()` in any benchmark).
- `go build ./...` and `go test ./...` must pass before every commit.
- `golangci-lint` must introduce no new findings.
- No new third-party dependencies. No JS framework, bundler, or CDN reference.
- All generated JS goes inline in the template, matching the existing `themeScript` / `sortScript` pattern in `cmd/generate-items-kb/main.go:2822,2835`.
- Work happens in `/home/robert/spacemolt/kb` on branch `feat/resource-galaxy-map`. Do **not** touch `/home/robert/spacemolt/kb-phase-0-cube-map` (stale 2026-07-03 side branch).
- Generated site output under `kb/` is committed to the repo; regenerating it is expected.

## Deviation from spec

The spec's `Options` struct listed an `IDPrefix` field for namespacing element ids. Dropped as YAGNI: the only generated element id is the `goo-galaxy` filter, which is emitted *only* when `ShowEmpireBlobs` is true, and the Resources variant sets it false. There is no id collision to prevent. Add it later if a page ever renders two blob-bearing maps.

The spec did not mention `LinkPrefix`. It is required: `galaxy-map.html` sits at `kb/` and links to `systems/<id>/`, while the Resources page sits at `kb/resources/` and must link to `../systems/<id>/`.

## File Structure

| File | Responsibility |
|---|---|
| `pkg/galaxymap/galaxymap.go` (create) | `System`, `Connection`, `Options` types and `Render()`. Pure function: data in, SVG string out. No DB, no filesystem. |
| `pkg/galaxymap/galaxymap_test.go` (create) | Unit tests over `Render()` using in-memory fixtures. |
| `cmd/generate-galaxy-map/main.go` (modify) | Becomes a thin caller: load from DB, call `galaxymap.Render`, write the page. Loses ~230 lines. |
| `cmd/generate-items-kb/resources.go` (modify) | Adds resource→system aggregation, the dots-only map render, generated CSS, and the sticky panel markup. |

## Pre-flight: pin a database snapshot

Task 2 proves the extraction is behavior-preserving by diffing generated output. The live DB at `/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db` is written continuously by the agent fleet, so a plain before/after render would differ for unrelated reasons. Every render in Tasks 1–3 must use a frozen copy.

- [ ] **Step 1: Freeze a snapshot**

```bash
mkdir -p /tmp/galaxymap-verify
sqlite3 /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db \
  ".backup /tmp/galaxymap-verify/pinned.db"
ls -la /tmp/galaxymap-verify/pinned.db
```

Use `.backup`, not `cp` — the DB is in WAL mode with active writers, and a raw copy can capture a torn page.

- [ ] **Step 2: Capture the baseline render**

`cmd/generate-galaxy-map/main.go:45` hardcodes the DB path and `:63` hardcodes the output dir `"kb"`. Temporarily point it at the snapshot to capture a baseline, then restore the file:

```bash
cd /home/robert/spacemolt/kb
sed -i 's#knowledgeDBPath := ".*"#knowledgeDBPath := "/tmp/galaxymap-verify/pinned.db"#' cmd/generate-galaxy-map/main.go
go run ./cmd/generate-galaxy-map
cp kb/galaxy-map.html /tmp/galaxymap-verify/baseline.html
git checkout cmd/generate-galaxy-map/main.go kb/galaxy-map.html
wc -c /tmp/galaxymap-verify/baseline.html
```

Expected: a byte count near 228000 plus growth for the 505-system DB (the committed file was rendered in April against 378 systems, so expect roughly 300 KB).

---

### Task 1: Create `pkg/galaxymap`

**Files:**
- Create: `pkg/galaxymap/galaxymap.go`
- Test: `pkg/galaxymap/galaxymap_test.go`

**Interfaces:**
- Consumes: nothing (leaf package).
- Produces:
  - `type System struct { ID, Name string; PositionX, PositionY float64; PoliceLevel int; Empire string; IsStronghold bool; LastUpdatedTick int; Connections []Connection }`
  - `type Connection struct { SystemID, Name string; Distance int }`
  - `type Options struct { ShowEmpireBlobs, ShowConnections bool; HighlightClasses func(systemID string) []string; LinkPrefix string }`
  - `func Render(explored, unexplored []*System, systemMap map[string]*System, opt Options) string`

**Implementation note — move, don't retype.** The body of `Render` is the current `renderGalaxyMap` from `cmd/generate-galaxy-map/main.go:227-452`, moved verbatim, then modified at exactly four points (below). Retyping it from memory will break Task 2's byte-identical check. Copy the existing function, then apply the diffs.

The four modifications:

1. **`blobColor` must escape the blob block.** It is declared at the top of the blobs section (`const blobColor = "#E8E8E8"`) but read later by the dot-color default (`dotColor := blobColor`) and by the unexplored-dot loop. Hoist the `const` above the `if opt.ShowEmpireBlobs {` guard or the gated build fails to compile.

2. **Wrap the blob section** — the `<defs><filter id="goo-galaxy">…</filter></defs>` write, the `<g filter="url(#goo-galaxy)">` group, its thick connection lines, its per-system circles, and the closing `</g>` — in `if opt.ShowEmpireBlobs { … }`.

3. **Wrap both connection-line groups** — the `#63b3ed` explored-to-explored group and the `#a0aec0` dashed unexplored group — in `if opt.ShowConnections { … }`.

4. **Dot rendering** takes the link prefix and highlight classes:

```go
classes := "galaxy-sys-dot"
if opt.HighlightClasses != nil {
    if extra := opt.HighlightClasses(s.ID); len(extra) > 0 {
        classes = classes + " " + strings.Join(extra, " ")
    }
}
b.WriteString(fmt.Sprintf(`<a href="%ssystems/%s/"><circle cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="#000" stroke-width="0.5" class="%s"><title>%s</title></circle>`,
    opt.LinkPrefix, s.ID, sx, sy, dotColor, classes, s.Name))
```

**Preserve the existing double-`</a>` bug.** For capitals and strongholds the current code emits `</a>` inside the label branch *and* again unconditionally afterward — 10 occurrences in the committed output. Keep it exactly as-is here. Task 3 fixes it, after Task 2 has proven the extraction clean. Fixing it now makes Task 2 fail for the right reason at the wrong time.

- [ ] **Step 1: Write the failing test**

Create `pkg/galaxymap/galaxymap_test.go`:

```go
package galaxymap

import (
	"strings"
	"testing"
)

func sampleSystems() ([]*System, map[string]*System) {
	a := &System{
		ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
		Empire: "solarian", LastUpdatedTick: 100,
		Connections: []Connection{{SystemID: "vega", Distance: 10}},
	}
	b := &System{
		ID: "vega", Name: "Vega", PositionX: 100, PositionY: 100,
		Empire: "nebula", LastUpdatedTick: 100,
		Connections: []Connection{{SystemID: "sol", Distance: 10}},
	}
	return []*System{a, b}, map[string]*System{"sol": a, "vega": b}
}

func TestRenderFullVariantHasBlobsAndConnections(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{
		ShowEmpireBlobs: true,
		ShowConnections: true,
	})

	if !strings.Contains(svg, "<svg") {
		t.Fatalf("output is not an SVG:\n%s", svg)
	}
	if !strings.Contains(svg, "feGaussianBlur") {
		t.Errorf("missing metaball blob filter")
	}
	if !strings.Contains(svg, "<line") {
		t.Errorf("missing connection lines")
	}
}

func TestRenderDotsOnlyVariantOmitsBlobsAndConnections(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{
		ShowEmpireBlobs: false,
		ShowConnections: false,
	})

	if strings.Contains(svg, "feGaussianBlur") {
		t.Errorf("blob filter present with ShowEmpireBlobs=false")
	}
	if strings.Contains(svg, "goo-galaxy") {
		t.Errorf("blob filter id present with ShowEmpireBlobs=false")
	}
	if strings.Contains(svg, "<line") {
		t.Errorf("connection lines present with ShowConnections=false")
	}
	// Dots survive.
	if n := strings.Count(svg, "galaxy-sys-dot"); n != 2 {
		t.Errorf("got %d dots, want 2", n)
	}
}

func TestRenderHighlightClassesAppliedPerSystem(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{
		HighlightClasses: func(id string) []string {
			if id == "sol" {
				return []string{"r-iron-ore", "r-copper-ore"}
			}
			return nil
		},
	})

	if !strings.Contains(svg, `class="galaxy-sys-dot r-iron-ore r-copper-ore"`) {
		t.Errorf("sol missing highlight classes:\n%s", svg)
	}
	// vega got none, so it keeps the bare class.
	if !strings.Contains(svg, `class="galaxy-sys-dot"`) {
		t.Errorf("vega should have the bare dot class")
	}
}

func TestRenderNilHighlightClassesIsSafe(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{HighlightClasses: nil})

	if !strings.Contains(svg, `class="galaxy-sys-dot"`) {
		t.Errorf("nil HighlightClasses should still emit the base class")
	}
}

func TestRenderLinkPrefixHonored(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{LinkPrefix: "../"})

	if !strings.Contains(svg, `href="../systems/sol/"`) {
		t.Errorf("LinkPrefix not applied:\n%s", svg)
	}
}

func TestRenderEmptyExploredReturnsPlaceholder(t *testing.T) {
	svg := Render(nil, nil, map[string]*System{}, Options{})

	if strings.Contains(svg, "<svg") {
		t.Errorf("expected placeholder text, got an SVG")
	}
	if !strings.Contains(svg, "No explored systems") {
		t.Errorf("expected the no-systems placeholder, got: %s", svg)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /home/robert/spacemolt/kb && go test ./pkg/galaxymap/ -v
```

Expected: FAIL — the package does not compile, `undefined: Render`, `undefined: System`.

- [ ] **Step 3: Create the package**

Create `pkg/galaxymap/galaxymap.go` with `package galaxymap`, the three type declarations from the Interfaces block above, an `isCapital` helper copied verbatim from `cmd/generate-galaxy-map/main.go:456-465`, and `Render` — the moved `renderGalaxyMap` body with the four modifications listed above. Imports: `fmt`, `strings`.

Add a doc comment on the package and on each exported symbol; `golangci-lint` in this repo flags missing comments on exported identifiers.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd /home/robert/spacemolt/kb && go test ./pkg/galaxymap/ -v
```

Expected: PASS, 6 tests.

- [ ] **Step 5: Lint and build**

```bash
cd /home/robert/spacemolt/kb && go build ./... && golangci-lint run ./pkg/galaxymap/
```

Expected: no output from either.

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/galaxymap/
git commit -m 'feat(galaxymap): extract reusable galaxy map renderer

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 2: Rewire `cmd/generate-galaxy-map`, prove byte-identical

**Files:**
- Modify: `cmd/generate-galaxy-map/main.go` — delete `renderGalaxyMap` (227-452), `isCapital` (456-465), and the `System`/`Connection` type declarations (18-36); import and call `pkg/galaxymap`.

**Interfaces:**
- Consumes: `galaxymap.Render`, `galaxymap.System`, `galaxymap.Connection`, `galaxymap.Options` from Task 1.
- Produces: no new exported API. `kb/galaxy-map.html` output must be unchanged.

This is the safety gate for the whole plan. If output differs, the extraction changed behavior and must be corrected before anything builds on it.

- [ ] **Step 1: Replace local types with the package types**

Delete the local `System` and `Connection` structs (`main.go:18-36`). Keep `POI` (`:38-42`) — it is local to this command and unused by the renderer. Add a type alias so `loadSystems` and `loadPOIs` need no edits:

```go
type System = galaxymap.System
type Connection = galaxymap.Connection
```

Using aliases rather than renaming keeps this task's diff small and reviewable.

- [ ] **Step 2: Call the package renderer**

In `writeGalaxyMapPage` (`main.go:162`), replace the `MapSVG` field assignment:

```go
MapSVG: template.HTML(galaxymap.Render(explored, unexplored, systemMap, galaxymap.Options{
    ShowEmpireBlobs: true,
    ShowConnections: true,
    LinkPrefix:      "",
})),
```

Then delete `renderGalaxyMap` and `isCapital` entirely.

- [ ] **Step 3: Build**

```bash
cd /home/robert/spacemolt/kb && go build ./... && go test ./...
```

Expected: both succeed, no output.

- [ ] **Step 4: Render against the pinned snapshot and diff**

```bash
cd /home/robert/spacemolt/kb
sed -i 's#knowledgeDBPath := ".*"#knowledgeDBPath := "/tmp/galaxymap-verify/pinned.db"#' cmd/generate-galaxy-map/main.go
go run ./cmd/generate-galaxy-map
diff /tmp/galaxymap-verify/baseline.html kb/galaxy-map.html && echo "BYTE-IDENTICAL"
```

Expected: `BYTE-IDENTICAL`.

If diff reports changes, **stop and fix `pkg/galaxymap` before continuing.** Read the diff — the usual causes are a dropped `</a>`, reordered `WriteString` calls, or the `blobColor` hoist changing the emitted dot color. Do not proceed to Task 3 with a non-empty diff.

- [ ] **Step 5: Restore the hardcoded path**

Revert **only** the `knowledgeDBPath` line. Do not run `git checkout` on this file — it would discard Steps 1 and 2 along with the temporary path.

```bash
cd /home/robert/spacemolt/kb
sed -i 's#knowledgeDBPath := "/tmp/galaxymap-verify/pinned.db"#knowledgeDBPath := "/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db"#' cmd/generate-galaxy-map/main.go
git diff cmd/generate-galaxy-map/main.go | grep -c 'pinned.db'
```

Expected: `0`. Then confirm the real edits survived:

```bash
grep -c 'galaxymap.Render' cmd/generate-galaxy-map/main.go
```

Expected: `1`.

- [ ] **Step 6: Regenerate against the live DB and commit**

```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-galaxy-map
git add cmd/generate-galaxy-map/main.go kb/galaxy-map.html
git commit -m 'refactor(galaxy-map): use pkg/galaxymap, output unchanged

Verified byte-identical against a pinned DB snapshot.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

The committed `kb/galaxy-map.html` will differ from its previous version — it was last rendered 2026-04-20 against 378 systems and now reflects 505. That is the intended refresh, and it is separate from the byte-identical check, which used the pinned snapshot on both sides.

---

### Task 3: Fix the doubled `</a>` in system dot markup

**Files:**
- Modify: `pkg/galaxymap/galaxymap.go` — the explored-dot loop.
- Test: `pkg/galaxymap/galaxymap_test.go`

**Interfaces:**
- Consumes: `Render`, `System`, `Options` from Task 1.
- Produces: no API change. Output changes by exactly 10 removed `</a>` tokens.

Pre-existing bug, now safe to fix because Task 2 has locked the extraction. Capitals and strongholds close their anchor twice: once inside the label branch, once unconditionally after.

- [ ] **Step 1: Write the failing test**

Append to `pkg/galaxymap/galaxymap_test.go`:

```go
func TestRenderCapitalDotClosesAnchorExactlyOnce(t *testing.T) {
	sol := &System{
		ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
		Empire: "solarian", LastUpdatedTick: 100,
	}
	other := &System{
		ID: "vega", Name: "Vega", PositionX: 100, PositionY: 100,
		Empire: "nebula", LastUpdatedTick: 100,
	}
	explored := []*System{sol, other}
	m := map[string]*System{"sol": sol, "vega": other}

	svg := Render(explored, nil, m, Options{})

	if strings.Contains(svg, "</a></a>") {
		t.Errorf("doubled anchor close in output:\n%s", svg)
	}
	// One anchor open and one close per system.
	if open, close := strings.Count(svg, "<a href="), strings.Count(svg, "</a>"); open != close {
		t.Errorf("unbalanced anchors: %d open, %d close", open, close)
	}
}
```

`sol` is in the `isCapital` set, so it takes the label branch; `vega` is not, exercising both paths.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /home/robert/spacemolt/kb && go test ./pkg/galaxymap/ -run TestRenderCapitalDotClosesAnchorExactlyOnce -v
```

Expected: FAIL — `doubled anchor close in output` and `unbalanced anchors: 2 open, 3 close`.

- [ ] **Step 3: Fix**

In the explored-dot loop, drop the `</a>` from the end of the label `WriteString` so the unconditional one after the branch is the only close:

```go
// Label for major systems (capitals or strongholds).
if s.IsStronghold || isCapital(s.ID) {
    b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="galaxy-sys-label" fill="#d8dee9" font-size="12" font-weight="bold">%s</text>`,
        sx+8, sy+4, s.Name))
}
b.WriteString(`</a>`)
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd /home/robert/spacemolt/kb && go test ./pkg/galaxymap/ -v
```

Expected: PASS, 7 tests.

- [ ] **Step 5: Regenerate and confirm the change is exactly what was intended**

```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-galaxy-map
grep -c '</a></a>' kb/galaxy-map.html
```

Expected: `0` (grep exits 1 with count 0 — that is success here).

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/galaxymap/ kb/galaxy-map.html
git commit -m 'fix(galaxymap): close system anchor once for capitals and strongholds

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 4: Aggregate resource→system data on the Resources page

**Files:**
- Modify: `cmd/generate-items-kb/resources.go`
- Test: `cmd/generate-items-kb/resources_map_test.go` (create)

**Interfaces:**
- Consumes: `ResourceEntry` (`resources.go:19-33`), `ResourceGroup` (`:36-41`), the existing `anchorID` template func (`:255-258`).
- Produces:
  - `func resourceSlug(resourceName string) string` — the slug shared by the anchor, the CSS class suffix, and the dropdown value.
  - `func systemResourceClasses(groups []ResourceGroup) map[string][]string` — system ID → sorted `r-<slug>` classes.

**Why a shared slug function.** The page's anchors come from `anchorID(ResourceName)` — "Iron Ore" becomes `iron-ore`, *not* the resource_id `iron_ore`. If the CSS classes used resource_id the JS would need a name→id lookup table. Deriving both from one function keeps hash, class, and dropdown value identical and removes the table entirely.

`anchorID` is currently an anonymous closure inside the `funcs` map. Extract it to a package-level `resourceSlug` and have the template func delegate, so Go code and templates cannot drift.

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-items-kb/resources_map_test.go`:

```go
package main

import (
	"reflect"
	"testing"
)

func TestResourceSlugMatchesAnchorFormat(t *testing.T) {
	cases := map[string]string{
		"Iron Ore":       "iron-ore",
		"Water Ice":      "water-ice",
		"Miner's Delight": "miners-delight",
		"Copper":         "copper",
	}
	for in, want := range cases {
		if got := resourceSlug(in); got != want {
			t.Errorf("resourceSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSystemResourceClassesGroupsBySystem(t *testing.T) {
	groups := []ResourceGroup{
		{
			ResourceName: "Iron Ore", ResourceID: "iron_ore",
			Entries: []ResourceEntry{
				{SystemID: "sol"}, {SystemID: "vega"}, {SystemID: "sol"},
			},
		},
		{
			ResourceName: "Water Ice", ResourceID: "water_ice",
			Entries: []ResourceEntry{{SystemID: "sol"}},
		},
	}

	got := systemResourceClasses(groups)

	want := map[string][]string{
		"sol":  {"r-iron-ore", "r-water-ice"},
		"vega": {"r-iron-ore"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSystemResourceClassesDedupesRepeatedSystem(t *testing.T) {
	// Two POIs in one system bearing the same resource must yield one class.
	groups := []ResourceGroup{
		{
			ResourceName: "Iron Ore", ResourceID: "iron_ore",
			Entries: []ResourceEntry{{SystemID: "sol"}, {SystemID: "sol"}},
		},
	}

	got := systemResourceClasses(groups)

	if n := len(got["sol"]); n != 1 {
		t.Errorf("got %d classes for sol, want 1: %v", n, got["sol"])
	}
}

func TestSystemResourceClassesSkipsUndiscovered(t *testing.T) {
	groups := []ResourceGroup{
		{ResourceName: "Unobtainium", ResourceID: "unobtainium", Entries: []ResourceEntry{}},
	}

	got := systemResourceClasses(groups)

	if len(got) != 0 {
		t.Errorf("undiscovered resource produced classes: %v", got)
	}
}
```

The dedupe test is the one that matters most — 107 of 1928 (system, resource) pairs have more than one POI, so a naive append emits duplicate classes on ~5% of dots.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-items-kb/ -run 'TestResourceSlug|TestSystemResourceClasses' -v
```

Expected: FAIL — `undefined: resourceSlug`, `undefined: systemResourceClasses`.

- [ ] **Step 3: Implement**

Add to `cmd/generate-items-kb/resources.go`:

```go
// resourceSlug converts a resource display name into the slug shared by the
// page anchor, the highlight CSS class, and the map dropdown value.
func resourceSlug(name string) string {
	r := strings.NewReplacer(" ", "-", "'", "")
	return strings.ToLower(r.Replace(name))
}

// systemResourceClasses maps each system ID to the sorted set of
// "r-<slug>" classes for the resources it contains. Systems with no
// surveyed deposits are absent from the result.
func systemResourceClasses(groups []ResourceGroup) map[string][]string {
	seen := make(map[string]map[string]bool)
	for _, g := range groups {
		if len(g.Entries) == 0 {
			continue
		}
		class := "r-" + resourceSlug(g.ResourceName)
		for _, e := range g.Entries {
			if seen[e.SystemID] == nil {
				seen[e.SystemID] = make(map[string]bool)
			}
			seen[e.SystemID][class] = true
		}
	}

	out := make(map[string][]string, len(seen))
	for sysID, classSet := range seen {
		classes := make([]string, 0, len(classSet))
		for c := range classSet {
			classes = append(classes, c)
		}
		slices.Sort(classes)
		out[sysID] = classes
	}
	return out
}
```

`slices` is already imported (`resources.go:12`). Sorting makes output deterministic across builds — without it Go's randomized map iteration produces a different class order every run, and the committed HTML churns on every regeneration.

Then change the `anchorID` template func to delegate:

```go
"anchorID": resourceSlug,
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-items-kb/ -v
```

Expected: PASS, 4 new tests, no existing test broken.

- [ ] **Step 5: Confirm the page is unchanged so far**

```bash
cd /home/robert/spacemolt/kb
cp kb/resources/index.html /tmp/galaxymap-verify/resources-before.html
go run ./cmd/generate-items-kb -resources-only
diff /tmp/galaxymap-verify/resources-before.html kb/resources/index.html && echo "UNCHANGED"
```

Expected: `UNCHANGED`. This task is pure refactor plus dead-but-tested helpers; the delegation of `anchorID` to `resourceSlug` must not alter a single anchor.

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-items-kb/resources.go cmd/generate-items-kb/resources_map_test.go
git commit -m 'feat(kb): add resource slug and system-to-resource class helpers

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 5: Render the map and generated CSS into the Resources page

**Files:**
- Modify: `cmd/generate-items-kb/resources.go` — new loader, `writeResourcePages` wiring, `resourceIndexTemplate`.
- Test: `cmd/generate-items-kb/resources_map_test.go`

**Interfaces:**
- Consumes: `systemResourceClasses`, `resourceSlug` (Task 4); `galaxymap.Render`, `galaxymap.System`, `galaxymap.Connection`, `galaxymap.Options` (Tasks 1–3).
- Produces:
  - `func loadSystemsForMap(db *sql.DB) ([]*galaxymap.System, map[string]*galaxymap.System, error)`
  - `func resourceHighlightCSS(groups []ResourceGroup) string`
  - Template fields `MapSVG template.HTML`, `HighlightCSS template.CSS`, `FirstSlug string`.

Note `loadSystemsForStats` (`resources.go:104`) selects only `id, last_updated_tick` — insufficient for rendering. Add a separate loader rather than widening it, so the stats path stays cheap.

- [ ] **Step 1: Write the failing test**

Append to `cmd/generate-items-kb/resources_map_test.go`:

```go
import "strings" // add to the existing import block

func TestResourceHighlightCSSOneRulePerDiscoveredResource(t *testing.T) {
	groups := []ResourceGroup{
		{ResourceName: "Iron Ore", Entries: []ResourceEntry{{SystemID: "sol"}}},
		{ResourceName: "Water Ice", Entries: []ResourceEntry{{SystemID: "vega"}}},
		{ResourceName: "Unobtainium", Entries: []ResourceEntry{}},
	}

	css := resourceHighlightCSS(groups)

	if !strings.Contains(css, `#res-map[data-active="iron-ore"] .r-iron-ore`) {
		t.Errorf("missing iron-ore rule:\n%s", css)
	}
	if !strings.Contains(css, `#res-map[data-active="water-ice"] .r-water-ice`) {
		t.Errorf("missing water-ice rule:\n%s", css)
	}
	if strings.Contains(css, "unobtainium") {
		t.Errorf("undiscovered resource must not get a rule:\n%s", css)
	}
	if n := strings.Count(css, "data-active="); n != 2 {
		t.Errorf("got %d rules, want 2", n)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd /home/robert/spacemolt/kb && go test ./cmd/generate-items-kb/ -run TestResourceHighlightCSS -v
```

Expected: FAIL — `undefined: resourceHighlightCSS`.

- [ ] **Step 3: Implement the CSS generator and the loader**

Add to `resources.go`:

```go
// resourceHighlightCSS emits one highlight rule per discovered resource.
func resourceHighlightCSS(groups []ResourceGroup) string {
	var b strings.Builder
	for _, g := range groups {
		if len(g.Entries) == 0 {
			continue
		}
		slug := resourceSlug(g.ResourceName)
		fmt.Fprintf(&b, "#res-map[data-active=\"%s\"] .r-%s{fill:#ffcc44;r:6;stroke:#7a5c00}\n", slug, slug)
	}
	return b.String()
}

// loadSystemsForMap loads system geometry and jump connections for the
// resource map. Unlike loadSystemsForStats it pulls position and empire.
func loadSystemsForMap(db *sql.DB) ([]*galaxymap.System, map[string]*galaxymap.System, error) {
	rows, err := db.Query(`
		SELECT id, name, position_x, position_y, police_level,
		       COALESCE(empire, ''), is_stronghold, last_updated_tick
		FROM systems ORDER BY name
	`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	var systems []*galaxymap.System
	byID := make(map[string]*galaxymap.System)
	for rows.Next() {
		var s galaxymap.System
		if err := rows.Scan(&s.ID, &s.Name, &s.PositionX, &s.PositionY,
			&s.PoliceLevel, &s.Empire, &s.IsStronghold, &s.LastUpdatedTick); err != nil {
			return nil, nil, err
		}
		if s.ID == "" {
			continue
		}
		systems = append(systems, &s)
		byID[s.ID] = &s
	}
	return systems, byID, rows.Err()
}
```

Add `"github.com/rsned/spacemolt-kb/pkg/galaxymap"` to the imports.

Connections are not loaded — the Resources variant sets `ShowConnections: false`, so `System.Connections` stays nil and is never read.

- [ ] **Step 4: Wire it into `writeResourcePages`**

After the `groups` slice is built and sorted (`resources.go:~215`), before `tmpl.Execute`:

```go
mapSystems, mapByID, err := loadSystemsForMap(db)
if err != nil {
	return fmt.Errorf("load systems for map: %w", err)
}
classes := systemResourceClasses(groups)

var explored, unexplored []*galaxymap.System
for _, s := range mapSystems {
	if s.LastUpdatedTick > 0 {
		explored = append(explored, s)
	} else {
		unexplored = append(unexplored, s)
	}
}

mapSVG := galaxymap.Render(explored, unexplored, mapByID, galaxymap.Options{
	ShowEmpireBlobs:  false,
	ShowConnections:  false,
	LinkPrefix:       "../",
	HighlightClasses: func(id string) []string { return classes[id] },
})

firstSlug := ""
for _, g := range groups {
	if len(g.Entries) > 0 {
		firstSlug = resourceSlug(g.ResourceName)
		break
	}
}
```

Add three fields to the anonymous `data` struct and populate them:

```go
MapSVG       htmltpl.HTML
HighlightCSS htmltpl.CSS
FirstSlug    string
```
```go
MapSVG:       htmltpl.HTML(mapSVG),
HighlightCSS: htmltpl.CSS(resourceHighlightCSS(groups)),
FirstSlug:    firstSlug,
```

`htmltpl` is the existing alias for `html/template` (`resources.go:7`). The `HTML` and `CSS` types are required — plain strings get escaped and the SVG renders as visible markup.

- [ ] **Step 5: Run tests and build**

```bash
cd /home/robert/spacemolt/kb && go build ./... && go test ./cmd/generate-items-kb/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-items-kb/
git commit -m 'feat(kb): render resource-highlight galaxy map data on Resources page

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 6: Sticky panel markup, styles, and hash sync

**Files:**
- Modify: `cmd/generate-items-kb/resources.go` — `resourceIndexTemplate` only.

**Interfaces:**
- Consumes: `.MapSVG`, `.HighlightCSS`, `.FirstSlug`, `.Groups` from Task 5.
- Produces: the rendered page. No Go API.

- [ ] **Step 1: Add styles**

In the template's `<style>` block, after the existing `.undiscovered` rule:

```css
.res-map-wrap { position: sticky; top: 8px; float: right; width: 380px;
    margin: 0 0 16px 24px; background: var(--bg-card);
    border: 1px solid var(--border); border-radius: 8px; padding: 12px; }
.res-map-wrap svg { width: 100%; height: auto; display: block;
    border-radius: 4px; }
.res-map-wrap select { width: 100%; margin-top: 8px; padding: 4px; }
.res-map-empty { display: none; font-size: 0.85em; color: var(--text-muted);
    margin-top: 8px; }
#res-map[data-empty="1"] + .res-map-empty { display: block; }
#res-map .galaxy-sys-dot { fill: #2a3038; r: 3; transition: none; }
{{.HighlightCSS}}
@media (max-width: 1100px) {
    .res-map-wrap { float: none; width: 100%; position: static; margin-left: 0; }
}
```

The generated rules must come *after* the dim default or they lose on equal specificity — `[data-active] .r-x` (0,2,0) beats `.galaxy-sys-dot` (0,1,0) on specificity, but ordering is what makes intent obvious to the next reader.

`transition: none` is deliberate: transitioning fill on up to 505 dots at once causes visible jank on resource switch.

- [ ] **Step 2: Add the panel markup**

Immediately after the `<h2>Resources</h2>` / intro `<p>`, before `<div class="summary-cards">`:

```html
<div class="res-map-wrap">
    <div id="res-map" data-active="{{.FirstSlug}}">{{.MapSVG}}</div>
    <div class="res-map-empty">No systems with this resource have been surveyed yet.</div>
    <select id="res-map-select" aria-label="Highlight resource on map">
{{- range .Groups}}
{{- if gt (len .Entries) 0}}
        <option value="{{anchorID .ResourceName}}">{{.ResourceName}} ({{len .Entries}})</option>
{{- end}}
{{- end}}
    </select>
</div>
```

Only discovered resources become options — an option with no CSS rule would silently do nothing.

- [ ] **Step 3: Add the sync script**

Before the closing `</main>`'s following `sortScript`, append a new script block to the template:

```html
<script>
(function () {
  var map = document.getElementById('res-map');
  var sel = document.getElementById('res-map-select');
  if (!map || !sel) return;

  var valid = {};
  for (var i = 0; i < sel.options.length; i++) valid[sel.options[i].value] = true;

  function apply(slug, updateHash) {
    if (!valid[slug]) {
      map.setAttribute('data-empty', '1');
      map.removeAttribute('data-active');
      return;
    }
    map.removeAttribute('data-empty');
    map.setAttribute('data-active', slug);
    if (sel.value !== slug) sel.value = slug;
    if (updateHash && location.hash.slice(1) !== slug) {
      history.replaceState(null, '', '#' + slug);
    }
  }

  sel.addEventListener('change', function () { apply(sel.value, true); });
  window.addEventListener('hashchange', function () { apply(location.hash.slice(1), false); });

  var initial = location.hash.slice(1);
  apply(valid[initial] ? initial : sel.value, false);
})();
</script>
```

Two things this gets right that are easy to get wrong:

- **No event loop.** `select.change` writes the hash via `history.replaceState`, which does **not** fire `hashchange`. Using `location.hash = slug` instead *would* fire it, re-entering `apply` — harmless here because the second call is idempotent, but `replaceState` also avoids polluting browser history with 52 entries as the user browses.
- **Unknown hash falls back.** The page has 54 anchors but only 52 are selectable; landing on `#unobtainium` (or any non-resource anchor) sets `data-empty` and shows the explanatory line rather than leaving every dot dim with no explanation.

- [ ] **Step 4: Generate and inspect**

```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-items-kb -resources-only
echo "--- dots:"        && grep -o 'galaxy-sys-dot' kb/resources/index.html | wc -l
echo "--- highlight rules:" && grep -o 'data-active="[a-z-]*"] ' kb/resources/index.html | wc -l
echo "--- options:"      && grep -c '<option value=' kb/resources/index.html
echo "--- no connections:" && grep -c '<line' kb/resources/index.html
echo "--- no blobs:"     && grep -c 'feGaussianBlur' kb/resources/index.html
```

Expected: 505 dots; 52 highlight rules; 52 options; 0 lines; 0 blur filters.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-items-kb/resources.go kb/resources/index.html
git commit -m 'feat(kb): sticky resource map panel with hash and dropdown sync

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 7: Full build, size gate, manual verification

**Files:** none modified unless the size gate trips.

**Interfaces:** consumes everything above; produces the verification record.

- [ ] **Step 1: Full build and test**

```bash
cd /home/robert/spacemolt/kb && go build ./... && go test ./... && golangci-lint run
```

Expected: all pass, no new lint findings.

- [ ] **Step 2: Full site regeneration**

```bash
cd /home/robert/spacemolt/kb && go run ./cmd/generate-items-kb
```

Expected: exits 0, logs `Resources: 54 types, 2049 total entries` (exact counts drift with the live DB).

- [ ] **Step 3: Check the size gate**

```bash
cd /home/robert/spacemolt/kb
RAW=$(wc -c < kb/resources/index.html)
echo "raw: $RAW  gz: $(gzip -c kb/resources/index.html | wc -c)"
if [ "$RAW" -gt 2621440 ]; then echo "GATE TRIPPED — fall back to per-resource pages"; else echo "within budget"; fi
```

Expected: roughly 1.65 MB raw, ~105 KB gzipped → `within budget`.

The spec's fallback trigger is 2.5 MB raw (2621440 bytes). If tripped, **stop** and revert to the per-resource-pages design in the spec's Fallback section; `pkg/galaxymap` carries over unchanged.

- [ ] **Step 4: Manual browser check**

Open `kb/resources/index.html` and confirm:

1. The map appears in a sticky panel, right-aligned, and stays put while scrolling.
2. Changing the dropdown re-highlights dots, and the URL hash updates.
3. Clicking a "Jump To Resource" TOC link scrolls to the section **and** updates the map.
4. Loading with `#iron-ore` in the URL highlights iron ore on arrival.
5. Loading with a bogus `#zzz` shows the "No systems…" line instead of a dead map.
6. A dot links through to `../systems/<id>/` and the target page exists.
7. The dark/light theme toggle still works and the map is legible in both.
8. At a narrow window width the panel drops below the content and spans full width.

Item 6 is the one most likely to fail — it depends on `LinkPrefix: "../"` from Task 5.

- [ ] **Step 5: Commit any regenerated output**

```bash
cd /home/robert/spacemolt/kb
git add kb/
git commit -m 'chore(kb): regenerate site with resource galaxy map

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

- [ ] **Step 6: Push**

```bash
cd /home/robert/spacemolt/kb && git push -u origin feat/resource-galaxy-map
```

---

## Self-review notes

Spec coverage checked section by section:

| Spec section | Task |
|---|---|
| `pkg/galaxymap` extraction + `Options` | 1 |
| Byte-identical verification | 2 (pre-flight captures the baseline) |
| Dots only, faint empire tint | 1 (`ShowEmpireBlobs/ShowConnections` false), 5 (call site) |
| Binary highlighting, one rule per resource | 5 |
| Sticky panel, hash + dropdown | 6 |
| Undiscovered-resource edge case | 4 (skipped in classes), 5 (no CSS rule), 6 (`data-empty` copy) |
| Unknown-hash edge case | 6 |
| Systems with no deposits stay dim | 1 (nil-safe `HighlightClasses`), 4 (absent from map) |
| Page-weight gate at 2.5 MB | 7 |
| Regenerated `galaxy-map.html` | 2, 3 |
| Out of scope: per-resource pages, grading, zoom, scroll-tracking | not implemented |

Deviations from the spec are recorded in the "Deviation from spec" section above (`IDPrefix` dropped, `LinkPrefix` added).

Two items not in the spec, discovered while reading the code, both handled:

- The doubled `</a>` on capitals and strongholds — Task 3, deliberately sequenced after the byte-identical gate.
- Map-iteration nondeterminism in class ordering, which would churn the committed HTML on every build — Task 4 Step 3 sorts.
