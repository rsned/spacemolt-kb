# Build-Cost +N-Hop Fulfillment — Design

**Date:** 2026-07-05
**Status:** Approved design; ready for implementation plan.
**Extends:** `2026-07-05-build-cost-matrix-design.md` (the shipped build-cost matrix). This feature adds an optional "source inputs from neighboring stations within N jumps" dimension.

## Goal

Let a build at a home station source its inputs not only locally but from any station within N system-jumps (N ∈ {1, 2, 3}), pooling their order-book depth. This expands feasibility and lowers cost for players willing to travel. The shipped matrix assumes each cell sources everything at one station; ~87% of rows are locally infeasible, so pooling nearby depth is high-value.

## Validated data facts (2026-07-05; re-check as data drifts)

- **Topology:** `spacemolt-knowledge.db` `connections(from_system, to_system, distance)` — 2130 edges over 505 systems, **symmetric** (0 missing reverse edges → treat as undirected). Index on `from_system`/`to_system`.
- **Market stations:** 29 stations with order-book data, each in a **distinct** system (29 stations → 29 systems, one each) — so hops are pure station-to-station system-jumps; no intra-system pooling.
- **System id resolution (critical):** `market.db` `stations.system_id` is a **mix** — 5 values match `systems.id` (slugs, e.g. `krynn`), 24 match `systems.name` (display names, e.g. `Alpha Centauri`, `Trader's Rest`). All 29 resolve via **id-first, then name fallback** → canonical `systems.id`, which is what `connections` uses. 0 unresolved.
- **Reachability** (BFS over the full graph, counting *other* market stations reachable): +1 hop → avg 1.6 (pool ~2.6 incl. home); +2 → avg 3.1 (pool ~4.1); +3 → avg 4.3 (pool ~5.3). **No station is isolated at +3.** Pools stay small (≤~5 stations) → compute is cheap.

## Core insight: the pure core (`pkg/buildcost`) needs ZERO changes

A "hop-N build at home station S" equals: construct a **pooled `Book`** = the union of the sell ladders of every station in `pool(S, N)`, re-sorted ascending by price, then feed it to the *existing* `BuildCell`. The cheapest-first `Walk` automatically consumes the cheapest depth anywhere in radius. All pooling lives in the **generator** (`cmd/generate-build-costs`); `Walk` / `PriceRequirements` / `CheapestRecipe` / `BuildCell` / `Margin` are untouched.

**Invariant (test hook):** for a fixed cell and mode, as N increases the pooled cost is **monotonically non-increasing** and feasibility **monotonically non-decreasing** (a superset pool only adds cheaper options; it never removes depth).

## Components

### Hop graph (new generator piece)
1. Load `connections` into an in-memory symmetric adjacency map keyed by `systems.id`.
2. Resolve each market station's system: `canon(system_id) = system_id if in systems.id else name→id`. Map `stationID → canonical systemID`.
3. BFS from each market station's system over the full graph; record `hopDist[S][T]` (in jumps) for all market-station pairs (∞ if unreachable).
4. `pool(S, N) = { T : hopDist[S][T] ≤ N }` (always includes S at distance 0).

### Pooled books (new generator piece)
For each home station S and radius N ∈ {1, 2, 3}: build `pooledBook(S, N)` = a `buildcost.Book` whose `Sell[item]` is the concatenation of `books[T].Sell[item]` for all `T ∈ pool(S, N)`, re-sorted ascending by price. `BestBuy` is **not** needed on the pooled book (margins use the local book — see below). N=0 reuses the existing per-station book directly.

### Landing pages (4, tab-linked)
- `index.html` — hop-0 ("Local"), **unchanged URL and behavior**.
- `hop-1.html`, `hop-2.html`, `hop-3.html` — same layout, filters, modes (BoM/Recipe), columns, and "Show only feasible" as hop-0, each computed with per-cell pooled books.
- A tab bar on all four pages links between radii (e.g. `Local | +1 | +2 | +3`).
- Each page runs `BuildMatrix` with **pooled book for cost** and the **home station's own book for margin** (`BuildCell(t, stationID, pooledBook, itemMargin(localBook, t.ID))`).

### Margin rule
Savings/Profit always compare against the finished good's ask/bid **at the home station S** (that is where you build and would alternatively buy/sell it), regardless of where inputs are pooled from. The margin source is unchanged from the shipped feature — only the *cost* book changes with radius.

### Detail pages
Each per-target detail page's per-station table gains **hop cost columns** so you can see, for this target, how radius changes the build at each home station. Concretely the per-station row becomes:

`Station | Empire | BoM@0 | BoM@+1 | BoM@+2 | BoM@+3 | Recipe@0 | Recipe@+1 | Recipe@+2 | Recipe@+3 | Savings | Profit`

Each `@N` cell is the pooled cost (from `BuildCell` with `pooledBook(S,N)`); infeasible values are styled grey / shown as an em-dash exactly as the existing `@0` columns are. The `@0` columns are today's Local values (unchanged). The table is wrapped in a horizontally-scrollable container (matching the site convention for wide tables). Savings/Profit remain hop-0/local and BoM-based (unchanged). The existing BoM/recipe input tables, catalog links, and Savings/Profit legend are retained; the legend gains one line noting `+N` = cheapest cost sourcing inputs within N jumps. Per-input provenance ("input X came from station Y at hop 2") is **deferred** as a nice-to-have — not in this scope.

## Data flow

```
loadBooks (per-station, capped)             ── existing
loadStations (station→system, empire)       ── existing (reuse; add canonical systemID)
loadConnections + BFS  → hopDist, pool(S,N) ── NEW
for N in {0,1,2,3}:
    pooledBooks[N][S] = union of books over pool(S,N), re-sorted   ── NEW (N=0 = books[S])
    matrix[N] = BuildMatrix(targets, pooledBooks[N], stations, …, margin from books[S])
    renderIndex(matrix[N] → index.html or hop-N.html)
renderDetail(target, per-station × per-radius cost/feasibility)    ── extended
```

## Error handling / edge cases

- **Unreachable pair** → `hopDist = ∞` → never pooled; pool silently excludes it. If some market system fails to resolve to a graph node (should be 0 per validation), log a warning and treat it as an isolated node (pool = {itself}).
- **Empty pooled ladder** for an input → shortfall as today → cell infeasible. Handled by existing `Walk`.
- **Determinism:** pool member order and ladder concatenation must be deterministic (sort pool members by stationID; ladder re-sort is stable/ascending) so regenerated sites diff cleanly.

## Testing

- Pure core: unchanged; existing tests stand.
- New generator tests:
  1. **System resolution:** id-hit, name-fallback, and (synthetic) unresolved cases map correctly.
  2. **Hop graph BFS:** on a small synthetic graph, `hopDist`/`pool(S,N)` are correct, including a 2-hop-through-empty-system path and an unreachable node.
  3. **Pooled book:** union of two stations' ladders for an item is concatenated and re-sorted ascending; a cheapest-first walk over the pool picks the globally cheapest depth.
  4. **Monotonicity:** for a constructed target/pool, cost is non-increasing and feasible-count non-decreasing from N=0→3.
  5. **Detail hop-columns:** rendered page contains the four per-radius cost/feasibility values for a station.

## Scope / non-goals

- No transport/fuel cost (pooled depth is free to combine) — chosen model.
- No per-input hop provenance on detail pages (deferred).
- Max radius fixed at 3.
- The related "min stations to build every item" did-you-know metric is a **separate** future feature, not part of this spec.

Estimated ~6 implementation tasks.
