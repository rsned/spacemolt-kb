# Bill of Materials Explorer — Design

**Date:** 2026-08-08
**Status:** Approved for planning

## Goal

Add an interactive Bill of Materials explorer to the Spacemolt KB: pick any
craftable output, set a quantity, choose which recipe to use for each
multi-recipe ingredient, and see the whole production chain as a layered
left-to-right graph plus flat input tables.

The existing per-target pages under `kb/build-costs/` already answer "what
does this cost and where can I build it" against live market depth. This page
answers a different question — "what goes into it, through which recipes" —
and deliberately carries no prices.

## Scope

**In scope:** output selection by autocomplete over every craftable target
(610 items, 335 ships, 2650 facilities = 3595 entries); a quantity spinner
from 1 to 99,999; per-item recipe selection; a multi-tier horizontal graph;
a flattened base-materials table; a direct-inputs table; a surplus table.

**Out of scope:** prices, credits, station availability, feasibility, and
market depth of any kind. Those live on the existing build-cost matrix pages,
which each node links to. Also out of scope: per-site recipe variation — a
recipe choice for an item applies to that item everywhere in the tree
(`circuit_board` uses one recipe throughout, never a different one per
consumer).

## Architecture

One static page, all computation client-side. Nothing is precomputed per
target: with 62 multi-recipe items (`power_cell` alone has 15 recipes) the
combinations cannot be enumerated server-side.

Go owns data generation; hand-maintained page assets own presentation. This
mirrors the existing `kb/warp.js`, which is hand-maintained and emitted by no
generator.

### Files

| File | Role |
|---|---|
| `cmd/generate-bom-explorer/` | Generator: `crafting.db` + newest snapshot catalogs → `recipe-graph.json`. Writes nothing else. |
| `pkg/catalog/` | Ship and facility catalog loaders plus latest-snapshot-dir discovery, extracted from `cmd/generate-build-costs`. |
| `kb/build-costs/explorer.html` | Hand-maintained: markup, CSS, standard KB theme toggle. No logic. |
| `kb/build-costs/bom-explorer.js` | Hand-maintained: graph model, roll-up, layout, SVG rendering, UI wiring. |
| `kb/build-costs/recipe-graph.json` | Generated data (~850 KB raw, ~125 KB gzipped). |
| `tests/js/bom-explorer.test.js` | `node --test` suite over the exported pure functions. |

`cmd/generate-build-costs/render.go` gains one link per target page:
"Explore this BoM interactively →", pointing at
`explorer.html?target=<id>`.

### Extraction of `pkg/catalog`

`cmd/generate-build-costs` currently holds `findLatestCatalogDir`,
`loadShipCatalog` (`main.go`) and `loadFacilityCatalog` (`facilities.go`) in
`package main`, so the new generator cannot reuse them. Move them to
`pkg/catalog` as `FindLatestDir`, `LoadShips`, `LoadFacilities`, with their
tests, and have both generators call the package. `generate-build-costs`
keeps its existing behaviour and its existing tests must still pass.

### Data file format

```json
{
  "items": {
    "steel_plate": {"n": "Steel Plate", "c": "refined"}
  },
  "recipes": {
    "smelt_steel": {
      "n": "Smelt Steel",
      "c": "Refining",
      "i": [["iron_ore", 5]],
      "o": [["steel_plate", 2]]
    }
  },
  "targets": {
    "absence": {
      "n": "Absence",
      "t": "ship",
      "bm": [["silicate_composite", 12], ["steel_plate", 5]]
    }
  },
  "defaults": {
    "circuit_board": "etch_pcb"
  }
}
```

- `items` — all 752 rows of `crafting.db.items`: id → display name, category.
- `recipes` — the 741 rows remaining after `wrap_*` / `unwrap_*` packaging
  recipes are dropped (20 of 761), with inputs (`i`) and outputs (`o`) as
  `[item_id, quantity]` pairs. Dropping them at generation time means the page
  can never surface or select one, so the `X ↔ contained_X` cycles they form
  are structurally unreachable rather than filtered client-side.
- `targets` — ships and facilities only. These are pure *sinks*: they carry a
  `build_materials` list but no recipe id, so they can only ever occupy the
  rightmost column and never appear as an input to anything. Facility
  `build_materials` quantities are `float64` in the catalog JSON and are
  converted to `int` on write. Facility entries without `build_materials`
  (77 of 2727) are omitted.
- `defaults` — an entry for each of the 62 items with more than one
  producing recipe, computed **in Go by the existing `bom.SelectRecipe`**,
  which is called on the *full* recipe set (it applies its own `wrap_*` /
  `unwrap_*` exclusion as filtering layer 1, so the result matches the static
  pages exactly).
  Items with exactly one recipe are omitted; the page falls back to that
  single recipe. This is what makes the explorer open on the same recipe path
  the static per-target pages already display.

The generator opens only `crafting.db` and the newest snapshot directory. It
requires no `market.db`, so the page cannot go stale against market data and
regeneration takes well under a second.

## Page layout

```
Bill of Materials Explorer

  Output [ Power Core                    ⌄ ]   Quantity [    1 ]⇅

  assemble_power_core  —  4 tiers, 12 items          [reset choices]
  ┌────────────────────────────────────────────────────────────────┐
  │  BASE ORES        tier 1         tier 2          OUTPUT        │
  │                                                                 │
  │  ┌──────────┐                                                   │
  │  │Uranium   │──96──┐ ┌────────────┐                             │
  │  │Ore       │      └▶│Refined     │──2──┐ ┌────────────┐        │
  │  └──────────┘        │Uranium     │     └▶│Fusion Fuel │──2─┐   │
  │  ┌──────────┐   ┌───▶│[refine  ▾] │      │Rod         │    │   │
  │  │Deuterium │──8┘    └────────────┘      └────────────┘    │   │
  │  │Ice       │                                         ┌────▼──┐ │
  │  └──────────┘                                         │Power  │ │
  │  ┌──────────┐───────────────2──────────────────────▶  │Core ×1│ │
  │  │Energy    │                                         └───────┘ │
  │  │Crystal   │                                                   │
  │  └──────────┘                        ← horizontally scrollable →│
  └────────────────────────────────────────────────────────────────┘

  BASE MATERIALS (flattened)      DIRECT INPUTS (assemble_power_core)
  Uranium Ore            96       Energy Crystal          2
  Deuterium Ice         152       Fusion Fuel Rod         2
  ...                             Helium-3                1
                                  Power Battery           1

  SURPLUS FROM BATCHING           (rendered only when non-empty)
  Refined Uranium         1
```

### Controls

**Output autocomplete.** A text input filtering the 3595 selectable outputs
on both display name and id, showing a keyboard-navigable result list with
each row's type (item / ship / facility). No dependency.

The selectable set is derived on load, not shipped as a fourth list: every
key of `targets` (335 ships + 2650 facilities), plus every non-terminal item
in `items` that at least one recipe in `recipes` produces (610 — `fuel_reserve`
is produced by 7 recipes but has no `items` row, so it has no name or
category and is skipped). Items that
the explorer treats as terminal — every ore and material, plus anything no
recipe produces — are not selectable as outputs; picking one is only
reachable by hand-editing the URL, which falls through to the "no recipe
produces this" message.

**Quantity.** `<input type="number" min="1" max="99999" step="1">`, default
1. Non-integer, out-of-range, and empty values clamp to the nearest valid
value rather than blanking the graph.

**Recipe selectors.** An item with more than one producing recipe renders a
`▾` selector inside its node box, listing the recipe ids that produce it.
Items with a single recipe render no control, so visual noise tracks actual
ambiguity rather than node count. A `[reset choices]` control returns every
item to its default.

Changing any selector recomputes and re-lays out the entire graph, because a
different recipe for an ingredient can change which tiers exist both above
and below it. There is one render path; no partial updates.

### Node boxes

Each box shows the item's display name (linked to its catalog page under
`kb/items/<category>/<id>.html`), the quantity this build requires, and the
recipe selector when applicable. Leaves get a distinct border colour from
intermediates; the output box is emphasised.

### Tables

- **Base materials** — the flattened leaf totals, the analogue of the
  existing "Bill of Materials" table on the per-target pages.
- **Direct inputs** — the immediate inputs of the top-level selected recipe:
  "what to buy if you buy sub-assemblies".
- **Surplus from batching** — items over-produced by whole-batch rounding.
  Rendered only when non-empty.

### URL state

`explorer.html?target=power_core&qty=5&r=circuit_board:etch_pcb,steel_plate:cast_steel`

Written with `history.replaceState` on every change, and parsed on load. Only
choices that differ from `defaults` are recorded, so the common URL is just
`?target=<id>`. Unknown target ids, unknown recipe ids, and out-of-range
quantities are ignored in favour of the defaults rather than erroring.

### Degenerate cases

These are the common cases and must look deliberate:

- A Refining recipe renders as exactly two boxes and one arrow:
  `[Uranium Ore] ──▶ [Uranium Concentrate]`, recipe name above the chart.
  44 real targets have this shape. (`steel_plate` is not one of them — its
  default recipe takes two inputs.)
- Reaching a terminal item by hand-editing the URL shows a message instead
  of a graph. Ores and materials are described as raw materials the explorer
  deliberately stops at — which stays accurate for the four ores that do have
  recipes; a non-ore item that no recipe produces is described as a drop.
  `decodeState` therefore admits any id that exists, leaving selectability to
  render time, rather than discarding unselectable ids while parsing.
- Selecting a ship or facility shows its `build_materials` expanded normally;
  the target box sits alone in the rightmost column.

## Computation

Given target `T`, quantity `Q`, and a choice map `C` (item id → recipe id):

### 1. Build the DAG

Depth-first from `T`. For item `X`, the active recipe is `C[X]`, else
`defaults[X]`, else its single producing recipe. That recipe's inputs become
`X`'s children. Expansion stops at leaves (below).

**One node per distinct item, not per path.** `steel_plate` feeding three
different parents is one box with three outgoing arrows. The result is a DAG,
not a tree.

### 2. Roll up quantities with whole batches

You cannot craft a partial batch, so quantities round up to whole batches at
every tier. Because items are shared, batch counts cannot be decided
top-down: an item's batch count depends on its *total* demand across every
parent. Process nodes in topological order, output first:

```
demand[T] = Q
for X in topological order (output → leaves):
    y          = the active recipe's output quantity of X
    batches    = ceil(demand[X] / y)
    surplus[X] = batches * y - demand[X]
    for each (input i, quantity q) of the active recipe:
        demand[i] += batches * q
```

Leaves accumulate demand and produce no surplus.

Computing batches per-parent and summing would over-count; this order is what
makes batching correct under sharing.

**This differs from the existing static tables**, which compute a per-unit
cost with a ceiling at each tier and then multiply. For `5 iron_ore → 2
steel_plate`, needing 3 plates: the static table reports 9 ore, this page
reports 10 ore and 1 surplus plate. Neither result dominates the other in
general (per-unit-then-multiply can under- or over-count depending on the
ratio), so the page carries a footnote explaining the difference and linking
to the static page.

### 3. Assign columns

`rank(leaf) = 0`; `rank(X) = 1 + max(rank of the active recipe's inputs)`,
memoized. Column index = rank, drawn left to right.

Because every input has strictly lower rank than its consumer, **all arrows
run strictly left-to-right and no arrow is ever within a column.** This is
the invariant the whole visual depends on. `T` always attains the maximum
rank, so the output is always rightmost.

A base ore consumed directly by the output spans the full width of the chart.
That is expected, not a defect.

### 4. Order within columns

Two barycentre passes: each node sorts to the mean vertical position of its
neighbours in the adjacent column. Standard, roughly twenty lines, and the
difference between readable and unreadable at the `station_core` worst case
(10 tiers, 75 nodes).

### 5. Edges

Elbow polylines routed through inter-column gutters, with an arrowhead at the
consumer. The quantity label sits near the arrowhead so it reads against the
consuming item.

### Leaves

An item is a leaf when its category is `ore` or `material`, **or** when no
recipe produces it. This is exactly the terminal rule in
`pkg/bom/calculator.go`'s `isTerminal`, so this page and the static tables
agree on where a chain stops.

Both halves of that rule are load-bearing. Four items are ores that also have
a crafting recipe — `energy_crystal`, `exotic_crystal`, `void_crystal`,
`hydrogen_gas` — so "has no recipe" alone would expand them, and the base
material totals would stop matching the static pages. It would also make
`circuit_board → power_cell → energy_crystal → circuit_board` a cycle
reachable by ordinary recipe picks. With the category test applied, a
strongly-connected-component analysis over all 615 craftable items finds
**zero** reachable cycles, which is what keeps the cycle backstop below
genuinely unreachable. Terminal items are therefore also excluded from the
selectable-output list: the explorer treats them as raw inputs.

The second kind of leaf is not mineable — `hoarfrost_heartcore` and similar
drops have no recipe and no ore category. The base-materials table labels
these distinctly so they are not read as ores.

### Cycles

`wrap_*` / `unwrap_*` recipes form `X ↔ contained_X` cycles in the source
data. All 20 are omitted from the generated `recipes` map, so they reach
neither `defaults` nor any selector list — the same exclusion layer 1 of the
existing `bom.selectRecipe` applies.

As a backstop, if expansion re-encounters an item already on the current DFS
stack, that node renders as "cycle — not expanded" and expansion stops there.
The page must never hang, whatever combination of choices a user makes.

### Scale

Median item target: 5 tiers, 15 distinct items. Worst case `station_core`: 10
tiers, 75 items. Layout is O(V+E) and completes imperceptibly.

## Testing

### Go

Fixture pattern already used by `cmd/generate-build-costs/facilities_test.go`:
a temp snapshot directory plus an in-memory SQLite database.

- Every craftable item, recipe, ship, and facility with `build_materials`
  reaches the JSON; counts asserted.
- `defaults` agrees with `bom.SelectRecipe` for all multi-recipe items.
- `wrap_*` / `unwrap_*` recipes appear in neither `defaults` nor any item's
  selectable recipe list.
- Facilities lacking `build_materials` are omitted; float quantities convert
  to int.
- `pkg/catalog` carries over the loader tests from `generate-build-costs`,
  and that generator's existing tests still pass against the extracted
  package.

### JavaScript

`node --test` (Node 22 built-in, no dependencies). `bom-explorer.js` ends
with `if (typeof module !== 'undefined') module.exports = {...}` — browsers
skip it, the test runner picks it up.

- **Batch roll-up**: 5 ore → 2 plates, need 3 ⇒ 10 ore, 1 surplus plate.
- **Shared-item aggregation**: an item consumed by two parents batches once
  against summed demand, not twice against each. This is the case that
  separates the correct algorithm from the naive one.
- **Layering invariant**: across a sample of real targets, every edge
  satisfies `rank(input) < rank(consumer)`.
- **Cycle backstop**: a hand-built cyclic choice map terminates and marks the
  node.
- **Leaf classification**: ore vs. no-recipe drop.
- **Default fallback and per-item override**.
- **URL state round-trip**: encode then decode returns the same target,
  quantity, and choice map; defaults are not serialised.

### Cross-check against the existing tables

For targets whose active recipes all yield exactly 1, batching and per-unit
arithmetic are provably identical. A test asserts the flattened base totals
equal `Q ×` the committed `bill_of_materials` rows for a sample of such
targets.

Targets containing a multi-yield recipe are expected to differ — that is the
deliberate change above — so rather than a test, the generator logs a
one-line count of affected targets on each run.

### Visual verification

Before the work is called done, render and eyeball three cases: the two-box
refining degenerate, a median-sized target, and the largest graph
(`overmind`, 10 tiers / 135 nodes).

### Gates

`go build ./...`, `go test ./...`, `node --test`, and `golangci-lint` with no
new findings.

## Regeneration

`cmd/generate-bom-explorer` runs standalone and depends only on `crafting.db`
and the newest `data/snapshots/<date>/` directory. It must be re-run after a
crafting-DB refresh, alongside the existing `generate-items-kb` and
`generate-build-costs` steps in the KB regeneration runbook.
