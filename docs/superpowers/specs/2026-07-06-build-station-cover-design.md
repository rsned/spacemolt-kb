# Build Station-Cover Did-You-Know Page — Design

**Date:** 2026-07-06
**Status:** Approved design; ready for implementation plan.
**Extends:** the shipped build-cost matrix (`2026-07-05-build-cost-matrix-design.md`) and its +N-hop follow-up. This is deferred follow-up #2 from `project_build_cost_matrix`.

## Goal

For every buildable target (838 items + ships), answer: **what is the minimum number of distinct market stations you must shop at to source all of its parts, with enough order-book depth for one build?** Present it as a did-you-know page: headline factoids, a vertical bar chart of the distribution (to show skew / tail length), a sortable per-target table, and a separate table of targets that can't be built even by pooling every station.

## Core metric (settled in brainstorming)

- **Metric:** minimum count of distinct stations whose *pooled* sell-side depth covers every required input quantity — a depth-aware weighted set-cover. **Count only**; travel distance between stations is ignored.
- **Two modes, two columns:** **BoM** (target fully decomposed to base ores — universal, works for items *and* ships) and **Recipe** (the target's direct recipe inputs / sub-assemblies). Recipe is **N/A** for ships and for any item whose recipe inputs aren't fully sourceable anywhere.
- **Basis:** depth for **one build** (one unit of the target; for Recipe, one run of the chosen recipe). Additionally compute a **batch = 10** pass and flag targets whose station count *increases* at 10 builds (thin-depth inputs) — surfaced as a factoid and a small badge in the table.
- **Eligibility:** all 29 market stations are eligible for the cover (galaxy-wide). Depth is the current, outlier-capped order book — the same books the matrix uses.
- **"Unbuildable" is keyed off BoM:** if a target's fully-decomposed BoM can't be covered even by pooling all 29 stations, it drops to the bottom table. A Recipe = N/A alone never demotes a target; it just shows N/A in the main table's Recipe column.

## Architecture

Two units, mirroring the matrix feature's pure-core / generator split:

### 1. Pure core — `pkg/buildcost` (new file `stationcover.go`)

No DB, no HTML. Operates on the package's existing `Requirement` type and a supplied per-station depth map.

```go
// StationDepth maps station id -> (item id -> total sell depth available there).
type StationDepth map[string]map[string]float64

// CoverResult is the outcome of a minimum-station cover search.
type CoverResult struct {
    Feasible bool     // requirements met by pooling all stations?
    Count    int      // minimum station count (valid when Feasible)
    Stations []string // one minimal cover, sorted (valid when Feasible)
    Exact    bool     // true = proven minimal; false = greedy upper bound
    Missing  []string // inputs whose total depth across all stations < required (when !Feasible), sorted
}

// MinStationCover finds the fewest stations whose pooled sell depth covers every
// requirement. It proves the minimum for covers up to exactK stations by
// exhaustive search over combinations, then falls back to a greedy upper bound.
// Deterministic: stationIDs are considered in sorted order; the first covering
// combination (lexicographically) is returned; ties break by id.
func MinStationCover(reqs []Requirement, depth StationDepth, stationIDs []string, exactK int) CoverResult
```

**Algorithm:**
1. Aggregate required quantity per input id (summing duplicate `Requirement`s). Empty reqs → `{Feasible:true, Count:0, Exact:true}`.
2. Feasibility gate: for each input, if `sum over all stations of depth[S][input] < requiredQty`, it's unmeetable. Collect all such inputs into `Missing`; if any exist → `{Feasible:false, Missing:...}`.
3. Exact search for `k = 1..exactK`: iterate k-combinations of `stationIDs` in lexicographic order; a combination covers iff for **every** input the summed depth over the combination ≥ requiredQty. First covering combination → `{Feasible:true, Count:k, Stations:combo, Exact:true}`.
4. If nothing covers by `exactK`: greedy — repeatedly add the station that reduces total remaining shortfall (Σ over inputs of `max(0, remaining[input])`) the most; ties break by id. Stop when covered. Return `{Feasible:true, Count:len(greedy), Stations:sortedGreedy, Exact:false}`.

`exactK = 3` in the generator: real covers here are almost always 1–3 stations (iron_ore is everywhere; only titanium_ore / plasma_residue are scarce), so the exhaustive pass yields the true minimum for the overwhelming majority. Cost at 29 stations: C(29,3)=3 654 combinations × ~20 inputs, trivially fast even across 838 targets × 2 modes × 2 batch sizes.

### 2. Generator glue — `cmd/generate-build-costs`

Reuses the existing single data-load pass (BoM, recipes, capped books, 29 stations). After rendering the matrices, it also builds the station-cover page. New file `stationcover.go` (package main) holds the glue + render; `pkg/buildcost` holds the algorithm.

- **Build the depth map** from the loaded books: `depth[S][i] = Σ order.Qty for order in books[S].Sell[i]`. `stationIDs` = sorted ids of the 29 stations.
- **Per target:**
  - **BoM cover:** `MinStationCover(t.BoM, depth, ids, 3)`.
  - **Recipe cover:** if `t.RecipeNA != ""` → N/A (reason carried through). Else compute `MinStationCover(rec.Inputs, depth, ids, 3)` for each `rec` in `t.Recipes`; choose the **feasible** result with the smallest `Count` (tie → the recipe `CheapestRecipe` would pick, i.e. lowest input cost, then recipe id). If no recipe is feasible → Recipe N/A (reason "no recipe sourceable").
  - **Batch pass:** recompute the BoM cover with every `Requirement.Qty` ×10; set `BatchSensitive = count10 > count1` (only meaningful when both feasible).
- **Partition:** BoM-feasible targets → main table; BoM-infeasible → bottom "can't be built anywhere" table (with `Missing` inputs).

### Data flow

```
loadBooks / loadTargets / loadStations        ── existing (reused)
depth[S][i] = Σ Sell[i].Qty                    ── NEW (generator)
for each target:
    bom  = MinStationCover(t.BoM, depth, ids, 3)         ── pkg/buildcost
    rec  = best feasible MinStationCover over t.Recipes  ── pkg/buildcost (or N/A)
    bom10= MinStationCover(t.BoM×10, depth, ids, 3)       ── batch flag
partition → buildable rows / unbuildable rows
factoids + histograms                          ── NEW (generator)
render kb/did_you_know/stations_to_build.html  ── NEW (generator)
```

## The page — `kb/did_you_know/stations_to_build.html`

Self-contained static HTML in the existing did-you-know style (`<link rel="stylesheet" href="../smui.css">`, matching the other pages' structure). Sections top-to-bottom:

1. **Headline factoids** (cards, computed over the **buildable** set, with the buildable/total denominator stated):
   - "N of 838 targets build from a single station."
   - "Hardest: *<target>* needs K stations." (max BoM count; name links to its build-cost detail page `../build-costs/<id>.html`)
   - "Average buildable target: X.X stations." (mean BoM count)
   - "M targets can't be built anywhere — their parts aren't all sold." (the bottom-table size)
   - "P targets need more stations to build 10 at once." (batch-sensitive count)

2. **Distribution bar chart** — pure-CSS vertical bars (a flex row of `div`s whose height ∝ target count; no JS library, no external assets). X-axis = min station count buckets `1,2,3,…,max`; Y = number of targets. Two charts side by side: **BoM** and **Recipe** (Recipe over targets with a Recipe count). Each bar labeled with its count; the "unbuildable" total is called out beside the BoM chart as a separate labeled figure (not a bar, since it has no finite count). This is the "how skewed / how long the tail" view.

3. **Main table** (buildable targets), columns: **Target** (links to `../build-costs/<id>.html`) · **Category** · **Kind** (item/ship) · **BoM stations** · **Recipe stations** (or "N/A") · **Example cover** (the returned BoM `Stations`, mapped to display names, comma-joined). Server-rendered, sorted by BoM count descending then name; lightweight self-contained click-to-sort on headers (same minimal inline JS pattern the KB already uses). A non-exact (greedy) count renders as `K*` with a footnote "≥, heuristic".

4. **Bottom table** (unbuildable targets): **Target** · **Category** · **Kind** · **Missing input(s)** — the `Missing` ids (mapped to display names where available). Short intro line: "These can't be built at any combination of stations right now because one or more inputs have no market depth anywhere."

**Index wiring:** the generator writes **only** this page. The `kb/did_you_know/index.html` factoid-grid card that links to it is added by hand as a one-time plan task (the index is hand-authored with other cards; the generator must not clobber it).

**Output path:** a new flag `-station-cover-out` (default `kb/did_you_know/stations_to_build.html`); `0`/empty disables the extra output. All existing generate-build-costs flags and behavior are unchanged.

## Edge cases

- **Empty BoM / no requirements:** Count 0, feasible — shouldn't occur for real targets; handled defensively.
- **Duplicate `Requirement`s for the same input:** summed before covering.
- **Recipe `OutputQty > 1`:** one recipe run is sourced (full input quantities); the metric is "stations to run the recipe once," consistent with `BuildCell`.
- **Illiquid / sentinel-priced inputs:** depth is taken from the already outlier-capped books, so a `999,999`-priced junk order is dropped at load and does not inflate available depth.
- **Non-exact counts:** flagged (`Exact=false` → `K*`), expected to be rare (deep covers only).
- **Determinism:** sorted station ids + first-lexicographic covering combination + id tie-breaks → regenerated pages diff cleanly (site is committed).

## Testing

**Pure core (`pkg/buildcost/stationcover_test.go`):**
1. Single station covers all inputs → Count 1, Exact, correct station.
2. Two inputs each split across two stations (neither alone has full depth) → Count 2, Exact, both stations.
3. Total depth < required for one input → `!Feasible`, `Missing` lists exactly that input.
4. A cover needing 4 stations with `exactK=2` → `!Exact`, greedy Count returned and it is a valid cover.
5. Determinism: two equal-size covers exist → the lexicographically-first sorted combo is returned.
6. Batch: same reqs ×10 needs more stations than ×1 given thin depth → Count increases.
7. Empty reqs → Count 0, Feasible, Exact.

**Generator (`cmd/generate-build-costs`):**
1. Depth-map construction sums a station's sell-ladder quantities per item.
2. Recipe-mode picks the feasible recipe with the smallest station count; all-infeasible → N/A.
3. Partition: a BoM-infeasible target lands in the unbuildable set with its missing inputs; a feasible one lands in the main set.
4. Factoid stats (single-station count, max/hardest, mean, unbuildable count, batch-sensitive count) match a small hand-built fixture.
5. Rendered page contains: the factoid text, both bar charts, a main-table row with an example cover, and a bottom-table row with a missing input.

## Scope / non-goals

- No travel/jump distance in the metric (count only) — the +N-hop matrix already covers the reachability angle.
- No per-input provenance beyond the example cover and the missing-input list.
- The generator does not edit `index.html` (hand-wired once).
- `exactK` fixed at 3; deeper covers report a greedy upper bound, flagged.

Estimated ~5 implementation tasks.
