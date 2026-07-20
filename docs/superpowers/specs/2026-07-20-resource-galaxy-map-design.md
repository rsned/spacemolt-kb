# Resource Galaxy Map — Design

Date: 2026-07-20
Repo: `spacemolt-kb` (`/home/robert/spacemolt/kb`, branch `main`)
Status: Approved, pending implementation plan

## Problem

The Resources page (`kb/resources/index.html`) lists every known deposit as 52
anchored sections of tables. It answers "what deposits exist" but not "where in
the galaxy is this resource concentrated." The galaxy already has a rendered map
(`kb/galaxy-map.html`), but it is a standalone page with no resource awareness.

Goal: for any selected resource, show which systems contain it, on a galaxy map,
from the Resources page.

## Data

Measured against `/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db`
on 2026-07-20:

| Quantity | Value |
|---|---|
| Systems | 505 |
| Connection rows | 2130 (directed; ~1065 undirected) |
| Distinct resources in `poi_resources` | 52 |
| `items` with category ore/material | 54 |
| Sections rendered on the page | 54 (52 discovered + 2 undiscovered) |
| `poi_resources` rows | 2049 |
| POIs bearing resources | 674 |
| Distinct (system, resource) pairs | 1928 |
| Coverage range | `exotic_matter` 1 system … `iron_ore` 235 systems |

Every `resource_id` in `poi_resources` resolves to an ore/material item, so the
54 sections are a strict superset of the 52 highlightable resources.

A (system, resource) pair maps to exactly one POI in 1821 of 1928 cases, so
there is a single natural marker per system per resource.

Positions are strictly 2D (`systems.position_x`, `position_y`, roughly ±7000).
There is no `position_z`; the `Z` field in the Go wire structs is reserved and
always zero.

### Staleness note

The checked-in `kb/galaxy-map.html` was generated 2026-04-20 and contains 378
system dots (its 881 `<circle>` elements are 378 dots plus metaball blob
circles). 505 is the correct current count. Regenerating the map is therefore an
intended side effect of this work, not a regression.

## Decisions

Recorded with rationale so they are not relitigated:

1. **One map on the Resources index, not per-resource pages.** Per-resource
   pages remain the documented fallback if this underperforms (see Fallback).
2. **Binary presence highlighting**, not graded by richness or remaining.
   Smallest generated CSS and the simplest thing to read. Richness grading was
   considered and deferred.
3. **Sticky panel, hash-driven.** Reuses the existing "Jump To Resource" TOC
   anchors rather than introducing a second navigation model.
4. **Dots only, faint empire tint.** No metaball blobs, no connection lines in
   this variant. Blobs and 2130 lines are the bulk of the bytes and are noise
   for a presence question.
5. **No scroll-tracking in v1.** Hash and dropdown only.

## Architecture

### `pkg/galaxymap` (new)

`renderGalaxyMap()` currently lives in `cmd/generate-galaxy-map/main.go` as
`package main` and is unreachable from `cmd/generate-items-kb`. Extract it:

```go
package galaxymap

type Options struct {
    HighlightClasses func(systemID string) []string // extra classes per dot
    ShowEmpireBlobs  bool
    ShowConnections  bool
    IDPrefix         string // namespaces element ids for multi-map pages
}

func Render(systems []*System, conns []Conn, opt Options) string
```

`cmd/generate-galaxy-map` becomes a thin caller passing
`{ShowEmpireBlobs: true, ShowConnections: true}`, preserving its current output.

This mirrors the repo's existing shared-package pattern (`pkg/systemmap`,
`pkg/jumpmap`, `pkg/marketmeta`).

**Extraction is verified by byte-identical output**: render
`kb/galaxy-map.html` against a pinned DB copy before and after the refactor and
diff. Any difference means the extraction changed behavior.

### Resources page integration

`cmd/generate-items-kb/resources.go` gains:

- A load step for (system → resource-set), derived from the `ResourceEntry`
  slice already loaded by `loadResourceEntries` — no new query needed.
- A `galaxymap.Render` call with `ShowEmpireBlobs: false`,
  `ShowConnections: false`, and `HighlightClasses` returning `r-<resource_id>`
  for each resource present in that system.
- Generated CSS: one rule per *discovered* resource (52). The 2 undiscovered
  sections get a dropdown entry but no rule — see Edge cases.
- A sticky panel in `resourceIndexTemplate` holding the SVG and a `<select>`.

## Highlight mechanism

```html
<div id="res-map" data-active="iron_ore">
  <svg ...>
    <circle class="gsd r-iron_ore r-copper_ore" cx="..." cy="..."/>
  </svg>
</div>
```

```css
#res-map .gsd { fill: var(--map-dim); r: 3; }
#res-map[data-active="iron_ore"] .r-iron_ore { fill: var(--map-hot); r: 6; }
/* one such rule per resource, 52 total */
```

Switching resources is a single attribute write. The browser restyles the
matching dots; nothing rebuilds, and the 2049 table rows never reflow.

Cost: ~2049 class tokens (~25 KB raw, compresses well) plus 52 rules.

## Behavior

- On load: read `location.hash`; if it matches a resource anchor, activate it.
  Otherwise activate the first resource.
- `select.onchange`: set `data-active`, update `location.hash`.
- `hashchange`: set `data-active` from the anchor. This makes the existing TOC
  drive the map with no extra markup.

Roughly 15 lines of vanilla JS, consistent with the KB's existing inline
`themeScript` / `sortScript` approach. No framework, no bundler, no CDN.

### Edge cases

- **Undiscovered resources** (2 of the 54 sections have 0 deposits): no CSS rule
  is generated, so every dot stays dim. The panel must state that no systems are
  known rather than looking broken or unresponsive.
- **Unknown hash** (stale or hand-edited): fall back to the first resource
  rather than leaving `data-active` unset with every dot dim.
- **Systems absent from `poi_resources`** render as dim dots always. This is
  correct — they are real systems with no surveyed deposits.

## Page weight

| Artifact | Current | Note |
|---|---|---|
| `kb/resources/index.html` | 1.57 MB raw / 96 KB gz | dominated by 2049 table rows |
| `kb/galaxy-map.html` | 228 KB raw / 30 KB gz | full variant, blobs + lines |
| Projected addition | ~60 KB raw | dots-only variant + 52 CSS rules |

The SVG is not the expensive part of this page; the deposit tables are. The
dots-only variant keeps the addition to roughly 4% of current raw size.

## Fallback

If the built page exceeds **~2.5 MB raw**, or resource-switch latency is
visibly janky in a browser, revert to per-resource detail pages: split the
index into `kb/resources/<resource_id>/index.html`, each with its own smaller
pre-rendered map and that resource's table. `pkg/galaxymap` is reusable
unchanged under that plan, so the extraction work is not wasted either way.

## Scope

**In:** `pkg/galaxymap` extraction with byte-identical verification; sticky map
panel on the Resources page; binary highlighting; generated per-resource CSS;
hash and dropdown sync; regenerated `kb/galaxy-map.html`.

**Out:** per-resource pages (the fallback); richness or remaining grading; pan
and zoom; scroll-tracking; any change to the deposit tables or their queries;
`kb-phase-0-cube-map` (a stale 2026-07-03 side branch — leave it alone).

## Verification

1. `go build ./... && go test ./...`
2. `golangci-lint` introduces no new findings.
3. `go run ./cmd/generate-galaxy-map` output diffs byte-identical against a
   pre-refactor render on a pinned DB.
4. `go run ./cmd/generate-items-kb -resources-only` succeeds; built page size
   recorded and checked against the 2.5 MB fallback trigger.
5. Manual browser check: dropdown switches highlighting; TOC anchor clicks
   drive the map; an undiscovered resource renders sensibly; theme toggle still
   works.
