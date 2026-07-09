# "Where Can I Make This" — Design

**Date:** 2026-07-09
**Status:** Approved, ready for implementation plan
**Output:** `kb/recipes/where.html` (single page, two tabs)

## Purpose

The knowledge DB has a new `public_facilities` table recording every public
production line the survey bots have seen across the galaxy. This page answers
two questions from that data:

1. **By Recipe** — "I want to make X. Which stations have a public facility for
   it, and what does it cost?"
2. **By Station** — "I'm docked at Y. What can I make here?"

It is a side page hanging off the Recipes section, not a new top-level nav
entry.

## Source data

`public_facilities` in `../spacemolt-knowledge.db`:

```sql
CREATE TABLE public_facilities (
  station_id     TEXT NOT NULL,
  facility_id    TEXT NOT NULL,
  recipe_id      TEXT NOT NULL DEFAULT '',
  facility_name  TEXT DEFAULT '',
  category       TEXT DEFAULT '',
  level          INTEGER DEFAULT 1,
  rental_fee_per_run INTEGER DEFAULT 0,
  owner_faction  TEXT DEFAULT '',
  public         INTEGER DEFAULT 1,
  details_json   TEXT DEFAULT '',
  last_seen_tick INTEGER DEFAULT 0,
  last_seen_utc  TEXT DEFAULT '',
  PRIMARY KEY (station_id, facility_id)
);
```

Shape as of 2026-07-09: **247 rows, all `category='production'`, all
`public=1`**, covering **149 distinct recipes** across **6 stations**. The
distribution is extremely lopsided — Confederacy Central Command holds 219 of
the 247; the other five hold 2, 5, 6, 7, and 8. The table is new and the other
stations are expected to fill out over time, so the layout must not be tuned to
today's skew.

Facility data is surveyed by per-station bots on a roughly hourly cadence.

### Joins (all verified to resolve)

- `station_id` → `bases.id` → `bases.poi_id` → `pois.id` → `systems.id`.
  Three of the six station IDs are *base* IDs (`confederacy_central_command` →
  POI `sol_central`); three are *POI* IDs directly. Today all six appear in
  `bases`, but the query must tolerate either shape.
- `recipe_id` → `crafting.db recipes.id`. All 149 resolve, spanning 14
  categories. Recipe pages live at `kb/recipes/<Category_Dir>/<id>.html`.
- `details_json.type` → `kb/facilities/production/<type>.html`. All 178 distinct
  types have a rendered page.
- `owner_faction` → `factions.faction_id`. **5 of 8 resolve**; three are
  unknown hashes.
- Recipe outputs: 147 recipes have one output, 2 have two. None have zero.

### Fields used

Columns: `station_id`, `facility_id`, `recipe_id`, `facility_name`, `level`,
`rental_fee_per_run`, `owner_faction`, `last_seen_tick`.

From `details_json` (the only three values not available as columns):
`type`, `production.items_per_hour`, `production.output_per_run`.

### Fields deliberately excluded

- **`queued_runs`, `backlog_ticks`, `queued_items`** — a snapshot of the hopper
  at scrape time. On a statically generated page they would read as
  authoritative while being stale on arrival, and one snapshot cannot
  distinguish a permanent backlog (worth avoiding) from a transient one.
- **`ticks_per_run`** — redundant with `items_per_hour`.
- **`last_seen_utc`** — empty on all 247 rows. Only `last_seen_tick` is
  populated (1305966/1305967, i.e. a single survey pass). Freshness is
  therefore rendered once at page level, not per row; the Resources page's
  per-row staleness treatment does not apply.

## Recipe coverage and the facility-only split

`crafting.db` holds **666** recipes. 149 have a known public facility; **517 do
not**. Those 517 divide along `recipes.facility_only`:

| Group | Count | Meaning |
|---|---|---|
| `facility_only = 1`, no public line | 236 | **A real gap.** Cannot be crafted at a bare station. |
| `facility_only = 0`, no public line | 281 | Craftable at any station regardless. |

(For reference, 81 of the 149 recipes that *do* have a public line are
`facility_only` — public lines are disproportionately how facility-only recipes
get made.)

These two groups answer different questions and are rendered as two separate
dense tables with different framing. They are **not** given a section each: 517
per-recipe placeholder cards would bury the 149 useful ones. This is where the
design diverges from the Resources page, which can afford a card per
undiscovered ore because undiscovered ores are the minority there. Here the
ratio inverts.

Both groups are shown rather than dropping the 281, so that looking up an
arbitrary recipe always yields a definite answer.

### Wording constraint

`public_facilities` records what survey bots observed at stations they visited.
Private and faction-owned facilities never appear in it. Copy must therefore
say **"no known public line"**, never "nowhere" or "impossible". Likewise the
281 are framed as **"no facility required"**, not "facilities are useless here"
— a public line can still out-throughput hand-crafting.

## Architecture

New file `cmd/generate-items-kb/where.go`, mirroring `resources.go`. Single
entry point:

```go
func writeWherePage(outDir string, knowledgeDB *sql.DB, recipes []Recipe, items map[string]*Item) error
```

Called from `main()` after the recipe and facility pages are written, through a
`generateWherePage(recipes, items)` wrapper that opens
`../spacemolt-knowledge.db` itself and logs a **warning, not a fatal**, if the
DB or the `public_facilities` table is missing. This mirrors `generateAllMissions`
and matters specifically because `public_facilities` is new: a bare
`go run ./cmd/generate-items-kb` against an older knowledge DB must keep working.

A `-where-only` flag opens both DBs and regenerates just this page, for fast
iteration (cf. `-resources-only`).

### Query

```sql
SELECT pf.station_id, pf.facility_id, pf.recipe_id, pf.facility_name,
       pf.level, pf.rental_fee_per_run, pf.owner_faction,
       pf.details_json, pf.last_seen_tick,
       b.name, p.id, s.id, s.name,
       COALESCE(f.name, ''), COALESCE(f.tag, '')
FROM public_facilities pf
LEFT JOIN bases    b ON b.id = pf.station_id
LEFT JOIN pois     p ON p.id = COALESCE(b.poi_id, pf.station_id)
LEFT JOIN systems  s ON s.id = p.system_id
LEFT JOIN factions f ON f.faction_id = pf.owner_faction
WHERE pf.public = 1
```

Three deliberate choices:

- **`WHERE public = 1`** — every row is currently public, but the column exists
  and the page promises public facilities. Filtering now means a future private
  row cannot silently leak onto the page.
- **`COALESCE(b.poi_id, pf.station_id)`** — resolves `station_id` whether it is
  a base ID or a POI ID.
- **`LEFT JOIN factions`** — the three unresolved owner hashes must render as a
  truncated `<code>` hash, not a broken link.

Recipe and item metadata are joined **in memory** against the `recipes` slice
and `items` map already loaded from `crafting.db` by `main()`. No cross-database
query.

### Degradation

| Condition | Behavior |
|---|---|
| Knowledge DB absent / unopenable | Warn, skip page, continue site generation |
| `public_facilities` table absent | Warn, skip page, continue site generation |
| Malformed `details_json` on a row | Blank throughput cells for that row only |
| Unresolved `owner_faction` | Truncated `<code>` hash, no link |
| Unresolved system | Plain-text station name, no link |

Station display name falls back `bases.name` → `pois.name` → raw `station_id`.

## Page structure

Single file `kb/recipes/where.html`. Linked from `kb/recipes/index.html` as a
callout above the category cards, in the same slot as the Items page's "Module
Tier Comparison Charts →" link. Not added to the global nav.

### Tabs

Two tabs, `#by-recipe` (default) and `#by-station`, driven off the URL hash.
Deep-link anchors are prefixed — `#r-refine_steel`, `#s-confederacy_central_command`
— and a small inline script selects the tab from the prefix on load, so an
external link to a recipe opens the correct tab *and* scrolls to it. Clicking a
tab writes back to the hash. No framework; roughly 15 lines, in the spirit of
the existing `themeScript` / `sortScript`.

### Header

Four summary cards:

| Card | Value (today) |
|---|---|
| Stations With Public Lines | 6 |
| Public Facilities | 247 |
| Recipes Covered | 149 |
| Facility-Only, No Public Line | 236 |

Followed by a freshness line: *"Facility data as of tick 1,305,967. Station
bots survey hourly."*

### Tab 1 — By Recipe

A 3-column jump-to TOC (as on Resources), then one section per recipe,
alphabetical, 149 sections. Each heading carries:

- Recipe name, linking to `../recipes/<Category_Dir>/<id>.html`
- An `N stations` badge
- The output item, linking to its item page
- A `Facility-Only` badge where `recipes.facility_only = 1`
- A `[top]` back-link

Table columns:

**Station · System · Facility · Level · Fee/run · Qty/run · Items/hr · Owner**

Station links to `../systems/<id>/index.html`. Facility name links to
`../facilities/production/<type>.html`. Owner links to the faction page where it
resolves. Sections are 1–4 rows. Tables use the existing `sortable` class,
chiefly so a multi-station recipe can be sorted by fee.

### Tab 1 bottom — the two dense tables

Below all 149 sections:

1. **"Facility-Only — No Known Public Line (236)"** — warning-toned callout.
   These require a facility and no public one is known; not craftable at a bare
   station.
2. **"No Facility Required (281)"** — neutral callout. Craftable at any station;
   a public line would only add throughput.

Both are 5-column dense tables, ~0.85em with tight cell padding, sortable:

**Recipe · Category · Output · Qty · Craft Time**

Recipe links to its recipe page, Output to its item page, Category to the recipe
category index. No `Facility-Only` badge column — the table split already
carries that information.

### Tab 2 — By Station

A station summary card, then one section per station ordered by facility count
descending (CCC first). Within a station, facilities are grouped under recipe
**category** subheadings (Refining, Components, Weapons, …), turning CCC's 219
rows into 14 scannable blocks and giving the sparse stations the same structure
they will grow into.

Per-block table columns:

**Recipe · Output · Facility · Level · Fee/run · Qty/run · Items/hr · Owner**

`Owner` is retained despite being repetitive within a station: CCC hosts
facilities from multiple factions.

### Column rationale

A separate `Type` column is omitted from both views. `facility_name` is the
title-cased `type` ("Salvage Smelter" / `salvage_smelter`), so a single column
carrying the name and linking to the type page conveys what two would.

`output_per_run` is labelled **Qty/run** rather than "Output/run" to avoid
colliding with the `Output` (item) column.

## Testing

Following the pattern of the existing `*_test.go` files in
`cmd/generate-items-kb/`:

- **Row loading** — an in-memory SQLite fixture covering: a base-ID station, a
  POI-ID station, an unresolved faction, a malformed `details_json`, and a
  `public = 0` row that must be filtered out.
- **Grouping** — by-recipe grouping produces one group per distinct
  `recipe_id`; by-station grouping produces category subgroups in stable order.
- **The 236/281 split** — given a fixture recipe set, recipes with a facility
  are excluded from both dense tables; the remainder partitions on
  `facility_only` with no overlap and no loss.
- **Ordering determinism** — station order (count desc, then name) and recipe
  order (name asc) are stable across runs, so regeneration produces no spurious
  git diff.
- **Template execution** — the page renders without error on the fixture and on
  an empty `public_facilities` table.

Per project convention, `go build ./...`, `go test ./...`, and `golangci-lint`
must all pass before commit.

## Out of scope

- Private and faction-owned facilities (`base_facilities`, `faction_facilities`
  are separate tables, not surveyed the same way).
- Non-production facility categories. Every row today is `category='production'`;
  if service/infrastructure rows appear later, they need their own design pass —
  "what can I craft here" does not describe them.
- Cost modelling. Rental fee is displayed raw. Combining it with BoM input costs
  from `market.db` to produce a true per-unit build cost is what
  `kb/build-costs/` already does; cross-linking the two is a possible follow-up.
- Queue/backlog reporting, which needs a time series rather than a snapshot.
