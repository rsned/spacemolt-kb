# Build-Cost Matrix — Design

**Date:** 2026-07-05
**Status:** Approved design, pending implementation plan
**Component:** New KB page — build cost of every Item + Ship at every Station, at live market prices, for both BoM and Recipe modes.

## 1. Purpose

Add a KB page that answers, for any craftable **Item** or **Ship** at any **Station**: *what would it actually cost to build here right now, at current market depth, and is that even possible?* Three reader goals drive the design:

1. **Reference lookup** — an accurate per-station build cost for both the fully-decomposed **BoM** (buy raw ore) and the one-level **Recipe** (buy sub-assemblies) paths.
2. **Build-vs-buy margin** — compare build cost against the finished good's own price at that station.
3. **Feasibility** — whether the build is completable from that station's current supply depth, and if not, how short it falls.

## 2. Data sources (verified 2026-07-05)

All paths are absolute; the generator resolves them relative to `kb/` where noted in the runbook.

| Data | Source | Key tables / fields |
|---|---|---|
| **Order book (items)** | `market.db` — `/home/robert/spacemolt/spacemolt/data/market.db` (~6 GB) | `market_orders(station_id, item_id, side['buy'|'sell'], price_each, quantity, captured_at, bucket_utc)`. 36 stations (29 active in latest hourly bucket), 599 items, ~9.8M orders. True depth: multiple price levels per (station,item,side). |
| **Fully-decomposed BoM** | `crafting.db` — `/home/robert/spacemolt-crafting-server/database/crafting.db` | `bill_of_materials(target_id, target_type['item'|'ship'|'facility'], base_item_id, quantity, recipe_path, has_alternatives)`. Decomposes to **61 base raw materials, all of which are market-priced**. Covers 507 items, 331 ships. |
| **One-level recipes (items)** | `crafting.db` | `recipes(id,name,category,...)`, `recipe_inputs(recipe_id,item_id,quantity)`, `recipe_outputs(recipe_id,item_id,quantity)`. 258 distinct input items, **257 market-priced**. 46 items have >1 recipe. |
| **One-level build inputs (ships)** | `catalog_ships.json` (latest under `/home/robert/spacemolt/data/game-api/<snapshot>/`) | Per-ship `build_materials:[{item_id,quantity}]`, `build_time`, `price`, `class`, `id`. **Ship build_materials are `comp_*`/`refined_*` sub-assemblies that are NOT market-traded (0/54) and not in the crafting DB** — see §4. |
| **Finished ship price (per station)** | knowledge DB — `/home/robert/spacemolt/spacemolt-knowledge.db` (symlink → `.../spacemolt/data/spacemolt-knowledge.db`) | `ship_listings(system_id, station_id, ship_id, class_id, ship_name, category, tier, price, captured_at, ...)`. Per-station shipyard asks, 15-min fresh, sparse (~3–6 ship types/station). |
| **Station → Empire** | knowledge DB | systems/empire mapping (as used elsewhere in the KB) for the Empire filter. Station→system via `market.db.stations(system_id,system_name)` or knowledge DB `pois`. |

**Snapshot grain:** "current" order book = each station's **most recent `captured_at`** (stations snapshot on their own cadence; there can be several `captured_at` values within one hourly `bucket_utc`). Do **not** mix captured_at times across a station.

## 3. Cost model (the compute core)

For each **(row = item/ship, column = station)** compute up to two independent results using that station's latest snapshot.

### Depth walk (shared primitive)

To price a required quantity `q` of material `m` at station `s`: take all `side='sell'` orders for `(s,m)` at the latest snapshot, **sort ascending by `price_each`**, and consume tiers (price × available qty) until `q` is met.

- **Covered**: `cost = Σ(price_each × qty_taken)`.
- **Short**: if the book is exhausted before `q` is met, record `cost_partial` (what was coverable) and `shortfall = q − covered_qty`.

Buying always hits **sell** orders (you pay the ask). Each cell is an **independent run** — it consumes only its own depth; no cross-cell/global supply contention.

### BoM mode (items and ships)

Decompose the target via `bill_of_materials` to base ores, depth-walk each. Feasible iff **every** base material is fully covered. Cost = Σ material costs. Infeasible cells still show the **partial cost + shortfall summary** ("6/7 covered; iron_ore short 400u").

### Recipe mode

- **Items:** direct `recipe_inputs` for the chosen recipe (see §3.1). Depth-walk each input (buy the sub-assembly). Feasible iff all inputs covered at that station.
- **Ships:** `build_materials` are `comp_*`/`refined_*` items that are **not market-traded**, so ship Recipe cost is unpriceable. Render **"sub-assemblies not market-traded"** (n/a + reason), not a number. Ship BoM (from ore) is the usable ship number.

### 3.1 Multi-recipe selection (46 items) — MVP

For a multi-recipe output, evaluate **every** recipe at that station and let the **cheapest feasible** one win the cell. The **chosen recipe id/name is surfaced in the cell and named on the detail page**, because the same item legitimately resolves to different recipes at different stations (empire ore disparity — one recipe uses ores common in Empire A's space, another uses Empire B's). If no recipe is feasible, pick the cheapest by partial cost and flag infeasible. *Future:* richer per-recipe/per-empire distinction; MVP is cheapest-wins with the recipe named.

## 4. Margins (finished-good comparison)

At the same station, compare build cost against the finished good's own price:

- **Items** — from `market_orders`: **savings vs ask** = cheapest sell − build cost; **profit vs bid** = best buy − build cost.
- **Ships** — from `ship_listings.price` at that station (per-station shipyard ask, 15-min fresh) → **savings vs ask**; fall back to catalog `price` where the ship isn't listed at that station. **Profit vs bid is n/a for ships** (no buy book). Mapping to resolve in planning: catalog ship `id`/`class` ↔ `ship_listings.class_id`/`ship_id`.

Margin cells are **n/a** whenever the finished good isn't priced at that station (common for ships and thin items).

## 5. Landing matrix page — `kb/build-costs/index.html`

Rows × station-columns, **one number + color per cell**, static HTML with the matrix data embedded as compact JSON and vanilla JS handling filter/toggle/sort (consistent with the existing static KB — no framework).

**Controls (top bar):**
- **Mode toggle:** BoM ↔ Recipe.
- **Metric toggle:** cost / savings / profit.
- **Filters:** Category · substring search (name/id) · Station · Empire (station→system→empire) · **☑ Show only feasible** (checkbox, **default on**).

**Cell:** the chosen metric value; **color** encodes feasibility (infeasible = muted/greyed) and margin sign; the **cheapest station in each row** is marked (`*`). For Recipe mode, the cell notes the chosen recipe (compact tag / on hover). Pinned left-hand summary columns: **cheapest station + its cost**, **best margin**, **feasible-station count**. Each row links to its detail page.

**Freshness:** stations whose latest snapshot exceeds a staleness threshold are greyed/flagged; unpopulated stations are dropped from the default view (and "Show only feasible" hides them anyway).

## 6. Detail page — `kb/build-costs/<id>.html`

One per item/ship. Full **station-breakdown table**: BoM cost, BoM feasible (x/N), Recipe cost, Recipe feasible (y/M) + **chosen recipe name**, finished ask, savings, finished bid, profit, snapshot age. The **cheapest feasible station is highlighted**.

Below the table, an **expandable per-material walk** for a selected station: each material's required qty, the ladder tiers consumed (price × qty), subtotal, and any shortfall. Links back to the existing KB item/ship page and to the recipe(s) used.

## 7. Architecture / generation

- **`pkg/buildcost`** — pure, unit-testable core. Loads each station's latest snapshot into sorted per-(station,item) sell/buy price ladders once, then exposes the depth-walk and per-cell cost/feasibility/margin calc. No rendering, no direct DB coupling beyond an injected data-loading boundary. The order-book walk is the primary unit-test target (covered/short/empty-book/multi-tier cases). Scale (~838 rows × ~29 stations × 2 modes) is trivial in one in-memory pass.
- **`cmd/generate-build-costs`** — reads the four sources (§2), computes all cells, renders `kb/build-costs/index.html` + `kb/build-costs/<id>.html`. Run on-demand now; add to the KB regen runbook; cron-able later. Follow existing `cmd/generate-items-kb` conventions.
- Binaries go in `bin/` (never repo root; gitignored).

## 8. Testing

- `pkg/buildcost` unit tests: depth-walk (exact fit, multi-tier, short book, empty book), BoM cost sum, Recipe cheapest-feasible selection across multiple recipes, margin math (ask/bid, ship listing vs catalog fallback), feasibility counts, shortfall reporting.
- A small fixture DB / in-memory ladder set drives tests (no dependence on the 6 GB live market.db).
- `go build ./...`, `go test ./...`, and `golangci-lint` clean before commit.

## 9. Future work (explicitly out of scope for MVP)

- **+1/+2/+3-hop fulfillment** — source materials from neighboring stations. The depth-walk already takes a station set, so this is: expand the set with graph neighbors (systems adjacency exists in the knowledge DB) and attribute cost/hops per material.
- **Richer multi-recipe / per-empire recipe distinction** beyond cheapest-wins.
- **Recursive pricing of ship sub-assemblies** if/when a component build tree becomes available as a data source.
- **Scheduled regeneration** (cron) once the on-demand generator is proven.

## 10. Key decisions (traceability)

| Decision | Choice |
|---|---|
| Page structure | Hybrid: filterable landing matrix → per-row detail pages |
| Filters | Category, substring, Station, Empire, Show-only-feasible (default on); mode + metric toggles |
| Buy side | Materials priced off **sell** book, ascending, walking depth |
| Independence | Each cell an independent run; no cross-cell supply contention |
| BoM mode | Base-ore decomposition; works for items **and** ships |
| Recipe mode | Items: `recipe_inputs`; Ships: `build_materials` → n/a "not market-traded" |
| Shortfall | Infeasible **+ partial cost + shortfall note** |
| Margins | Items: savings vs ask + profit vs bid; Ships: savings vs `ship_listings` price (catalog fallback), profit n/a |
| Multi-recipe | Cheapest feasible recipe per station wins; chosen recipe named in cell/detail |
| Freshness | Per-station latest `captured_at`; stale greyed, empty dropped from default |
| Architecture | `pkg/buildcost` (pure, tested) + `cmd/generate-build-costs`; outputs under `kb/build-costs/` |
