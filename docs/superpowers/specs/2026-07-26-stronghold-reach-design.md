# Stronghold Reach Map — Design

Date: 2026-07-26
Repo: `spacemolt-kb` (`/home/robert/spacemolt/kb`, branch `main`)
Status: Approved, pending implementation plan

## Problem

The KB can already answer "how far is each *empire capital* from a stronghold"
(`kb/did_you_know/capital_stronghold_distances.html`, five capitals plus five
fringe stations). It cannot answer the same question for the galaxy as a whole:
for every one of the 505 systems, how many jumps to the nearest of the nine
pirate strongholds, and what does that reach look like on a map as the radius
grows?

Goal: a Did-You-Know page that renders stronghold reach as red territory blobs
over the galaxy map, with a slider that grows the radius and shows the nine
isolated blobs merging into one.

## Data

Measured against `/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db`
on 2026-07-26.

| Quantity | Value |
|---|---|
| Systems | 505 |
| Connection rows | 2130 (directed; ~1065 undirected) |
| Strongholds (`is_stronghold = 1`) | 9 |
| Graph connected components | 1 (all 505) |
| Unreachable systems | 0 |
| Maximum distance to nearest stronghold | 14 |

The nine strongholds: `algol`, `alhena`, `barnard_44`, `bellatrix`,
`gliese_581`, `gsc_0008`, `sheratan`, `xamidimura`, `zaniah`. All nine have
empty `empire` and `police_level = 0`, consistent with the existing capitals
page describing them as neutral.

### Coverage and blob count by radius

Multi-source BFS from all nine strongholds, connections undirected:

| R | Systems ≤R | % of galaxy | Blobs | Note |
|---|---|---|---|---|
| 1 | 18 | 3.6% | 9 | |
| 2 | 41 | 8.1% | 9 | |
| 3 | 72 | 14.3% | 9 | |
| 4 | 114 | 22.6% | 8 | first merge |
| 5 | 171 | 33.9% | 8 | slider default |
| 6 | 235 | 46.5% | 6 | |
| 7 | 285 | 56.4% | 4 | |
| 8 | 335 | 66.3% | 2 | |
| 9 | 389 | 77.0% | 1 | one galaxy-spanning blob |
| 10 | 430 | 85.1% | 1 | |
| 11 | 461 | 91.3% | 1 | |
| 12 | 486 | 96.2% | 1 | |
| 13 | 500 | 99.0% | 1 | |
| 14 | 505 | 100.0% | 1 | full coverage |

Radius 15 is a duplicate of 14 and is not rendered.

**Every connected component of a reach set contains at least one stronghold.**
Any system within R jumps has a path to a stronghold along which distance
strictly decreases, so every step of that path is also within R. Blob count
therefore starts at 9 and only ever decreases — the merge sequence
9 → 8 → 6 → 4 → 2 → 1 is a property of the data, not of the rendering.

### Nearest-stronghold territory (BFS Voronoi)

Ties broken by ascending system ID for determinism.

| Stronghold | Systems |
|---|---|
| Algol | 142 |
| Alhena | 91 |
| Gliese 581 | 86 |
| GSC-0008 | 64 |
| Zaniah | 47 |
| Barnard 44 | 30 |
| Bellatrix | 16 |
| Xamidimura | 15 |
| Sheratan | 14 |

Algol dominating is consistent with the existing capitals page calling it "the
busiest gateway."

### The last systems to fall

- R = 13 (14 systems): Alzirr, Castor, Clearwater, Frostfeld, Fulu, HD 20794,
  Ironhearth, Matar, Relay Station, Rotanev, Saltwind, Sunridge, Tania,
  The Archive.
- R = 14 (5 systems): 82 Eridani, Blackthorn, GSC-0036, Muscida, Windmere.

### Snapshot staleness

**Corrected 2026-07-26 (after implementation).** This section originally claimed
the checked-in `kb/galaxy-map.html` was stale at 378 systems against a DB of 505.
That was wrong — the figure was carried over from the earlier
`2026-07-20-resource-galaxy-map-design.md` without being re-measured, and the
file already contained all 505 systems.

The real staleness was in the `systems.empire` field's *meaning*, not the system
count. The checked-in map was generated when that field held a **region** label
(every system tagged with its nearest empire, 364 systems colored). The field has
since been corrected to mean **ownership**, of which there are only 70 systems.
Regenerating recolors 294 of 505 dots from empire colors to unclaimed grey:

| | as-committed map | regenerated |
|---|---|---|
| unclaimed | 131 | 425 |
| nebula | 97 | 17 |
| solarian | 87 | 15 |
| voidborn | 75 | 10 |
| crimson | 60 | 16 |
| outerrim | 45 | 12 |

The conclusion still held: this page reads current data, so it disagreed with the
checked-in map until that map was regenerated. `galaxy-map.html` was out of scope
for this work and was regenerated separately in `061b9aab0`, which brought the
two pages into agreement on the same 70 owned systems.

## Decisions

Recorded with rationale so they are not relitigated:

1. **Stronghold set comes from `systems.is_stronghold = 1` only.** The
   station-name heuristic in `cmd/generate-galaxy-map/main.go` (matching
   "stronghold" / "fortress" / " port" in POI names) is deliberately *not*
   used. The flag yields exactly the nine neutral strongholds the existing
   `capital_stronghold_distances.html` page already describes to readers; the
   heuristic would silently add systems and make the two pages disagree.
2. **One map with a radius slider**, not ten stacked maps and not an
   autoplaying animation. Scrubbing lets a reader stop on a specific radius,
   and pre-rendering all frames into one SVG keeps the page near the byte cost
   of a single map.
3. **Slider covers 1–14, defaulting to 5.** The original request was 5–15, but
   15 is a duplicate frame and the strongest fact — nine separate blobs merging
   — is only visible below radius 9. Default stays at 5 to honor the requested
   starting point.
4. **Cumulative reach (≤R), not the exactly-R ring.** The question is coverage.
5. **Layers: dim dots + jump network + red blob.** No grey empire blob
   underneath; two metaball layers competing for the same pixels reads as mud.
6. **CSS-driven reveal, not client-side recomputation.** No graph work in the
   browser.

## Architecture

### `pkg/galaxymap` — one new optional field

`Options` gains a single field. Nil preserves current behavior exactly, so
`cmd/generate-galaxy-map` and the Resources page are unaffected.

```go
// ReachBlob, if non-nil, replaces the grey territory blob with a
// per-radius blob whose geometry carries "rb-<n>" activation classes.
// It is mutually exclusive with ShowEmpireBlobs; if both are set,
// ReachBlob wins.
ReachBlob *ReachBlob

// ReachBlob configures a radius-layered metaball blob.
type ReachBlob struct {
    // Radius returns the activation radius for a system, or -1 if the
    // system is never in reach.
    Radius func(systemID string) int
    // Max is the highest radius frame rendered.
    Max int
    // Color is the blob fill.
    Color string
}
```

When `ReachBlob` is non-nil, the existing blob-emitting code path changes in
two ways:

- Each blob circle is wrapped with `class="rb-<r>"` where `r` is that system's
  activation radius. Systems with radius `-1` or `> Max` emit no blob circle.
- Each blob edge line is wrapped with `class="rb-<r>"` where
  `r = max(radius(u), radius(v))`. Edges touching a never-in-reach system are
  skipped.

The existing `blobColor` constant is read by three things: the blob geometry,
the default dot color, and the unexplored-dot color. Only the first should turn
red. The implementation therefore introduces a *separate* blob-fill variable
that defaults to `blobColor` and is overridden by `ReachBlob.Color`; the two
dot paths keep reading the grey constant unchanged.

**Dots need no new API.** The existing `HighlightClasses` hook already returns
per-system classes and will emit `sr-<r>`, which the generated CSS uses to
brighten in-reach dots.

### `cmd/generate-stronghold-reach` (new)

Follows the `cmd/generate-galaxy-map` shape: load from the knowledge DB, render
via `pkg/galaxymap`, execute an `html/template`, write one file.

Split into files so the compute is testable without a database:

| File | Contents |
|---|---|
| `main.go` | flag parsing, DB open, orchestration, template execution |
| `load.go` | `loadSystems`, `loadConnections` — the only DB-touching code |
| `reach.go` | `ComputeReach(systems, edges, sources) Reach` — pure |
| `stats.go` | per-radius rows, Voronoi territory table, merge events |
| `css.go` | generated reveal CSS |
| `template.go` | page template |

```go
// Reach is the result of a multi-source BFS from the stronghold set.
type Reach struct {
    Dist   map[string]int // system ID -> jumps to nearest stronghold; absent if unreachable
    Owner  map[string]string // system ID -> nearest stronghold ID (ties by ascending ID)
    Max    int
}
```

Flags: `-db` (default the shared knowledge DB path) and `-out` (default
`kb/did_you_know/stronghold_reach.html`), so tests and ad-hoc runs can redirect
both. This is a deliberate improvement over `generate-galaxy-map`'s hardcoded
paths.

### Reveal CSS

Container carries the current radius:

```html
<div id="reach-map" data-r="5"> … svg … </div>
```

Generated rules, ~105 selectors for 14 frames:

```css
#reach-map .rb-1, … , #reach-map .rb-14 { display: none }
#reach-map[data-r="6"] .rb-1, … , #reach-map[data-r="6"] .rb-6 { display: inline }
```

Radius 0 is deliberately absent from both the hide rule and the reveal rules.
The nine strongholds are in reach at every frame, so their `rb-0` blob circles
are never hidden and need no per-frame selector.

An equivalent block over `sr-<n>` brightens in-reach dots. Out-of-reach dots
keep the dim default, so no rule is needed for them.

Total geometry on the page: 505 blob circles + ~1065 blob edges + 505 dots,
i.e. roughly one galaxy map, independent of frame count.

### Page structure

Header and nav copied from the sibling Did-You-Know pages (`../smui.css`,
`../items/items.css`, theme toggle).

1. Hero stat — every one of the 505 known systems lies within 14 jumps of a
   stronghold.
2. Slider (`<input type="range" min=1 max=14 value=5>`) with prev/next buttons
   and a live readout: `≤6 jumps · 235 systems · 46.5% of the galaxy · 6
   separate blobs`.
3. The map.
4. Radius table (all 14 rows, merge events marked).
5. Nearest-stronghold territory table.
6. Analysis notes: the merge sequence, Algol's dominance, the five systems at
   radius 14.
7. Data-source note naming the DB, the `is_stronghold` flag, and the BFS.

### JavaScript

Roughly fifteen lines. An inlined array of per-radius stats, a slider `input`
handler that sets `data-r` and rewrites the readout, and prev/next buttons that
step the slider. No graph computation, no fetch.

## Error handling

- **Zero strongholds found** — fail loudly (`log.Fatalf`). A silent empty map
  would look like a rendering bug.
- **Unreachable systems** — currently none, but the code must not assume that.
  They are rendered as permanently dim dots, excluded from every blob frame,
  and reported in a page footnote plus a generator log line. `Reach.Dist` simply
  omits them.
- **Systems missing coordinates** — same handling as the existing map code;
  no new behavior.
- **DB open/query failure** — `log.Fatalf`, consistent with the other
  generators.

## Testing

`pkg/galaxymap`:

- `ReachBlob` non-nil emits `rb-<n>` classes on blob circles and edges.
- Edge activation radius is `max` of its endpoints.
- Systems beyond `Max` and never-in-reach systems emit no blob geometry.
- `ReachBlob` nil leaves output unchanged — existing tests are the guard.

`cmd/generate-stronghold-reach` (all DB-free, on synthetic graphs):

- Multi-source BFS assigns distance 0 to every source.
- A system equidistant from two strongholds is owned by the
  lower-sorting ID.
- A disconnected node is absent from `Dist` and does not crash stats.
- Blob-count-per-radius is monotonically non-increasing.
- Every reach component contains a stronghold (the invariant above).
- Generated CSS contains one reveal block per radius and no rule for radius 0.

A single acceptance check against the live DB confirms the headline numbers:
505 systems, max radius 14, blob sequence 9/9/9/8/8/6/4/2/1/1/1/1/1/1.

## Out of scope

- Regenerating `kb/galaxy-map.html` for the corrected empire-ownership
  semantics. Was separate work; done in `061b9aab0`.
- Per-stronghold reach pages.
- Weighting by jump distance, fuel cost, or danger — hop count only.
