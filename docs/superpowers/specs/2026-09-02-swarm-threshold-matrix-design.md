# Swarm Threshold Matrix — Design

**Date:** 2026-09-02
**Status:** Draft for owner review
**Repo:** `/home/robert/spacemolt/kb` (KB repo only — no `spacemolt` changes)
**Builds on:** the Phase-B–calibrated combat model in `cmd/combat-sim`
(`data/combat-sim/calibration.json`), the catalog snapshot in
`data/combat-sim/catalog/`, and the exported battle fixtures in
`data/battles/`.

## 1. The Question

Inspired by *"you vs 1000 fifth-graders"* (r/whowouldwin): **how many
identical starter ships does it take to reliably destroy a given warship,
and how many of the swarm does that warship kill on the way down?**

The lone giant is deadly but can only shoot **one target per round**, so a
big enough swarm of underpowered attackers wins by attrition — each taking
mosquito bites while the giant can only swat one per tick. There is a real
**floor** (too few attackers can't out-damage shield regen — the giant is
effectively immortal) and a real **crossover** (enough attackers flip it to
a reliable win).

The headline example is a stock **Opus Magna** dreadnought (hull 3000,
shield 2400 @ 25/tick regen, armor 155, 35% flat DR, 8 all-energy mounts) vs
a swarm of **Prospects** (95 hull / 50 shield, one `pulse_laser_i`). But that
is just **one row** of the deliverable.

## 2. Deliverable

A **~375 × 5 matrix**:

- **Rows:** every hull in the ship catalog (`catalog_ships.json`, ~375
  entries) as the lone **defender**, fitted with its `default_modules`.
- **Columns:** the **5 empire starter hulls** as the **swarm attacker**,
  each fitted with its `default_modules`:

  | Empire | Starter | Weapons (default_modules) | Type | Speed |
  |---|---|---|---|---|
  | Crimson | `shard` | 2× autocannon_i (16/tick) | kinetic | 1 |
  | Nebula | `prospect` | pulse_laser_i (10/tick) | energy | 1 |
  | Outer Rim | `cobble` | autocannon_i (8/tick) | kinetic | 2 |
  | Solarian | `theoria` | pulse_laser_i (10/tick) | energy | 1 |
  | Voidborn | `threshold` | em_disruptor_i (5/tick) | em | 1 |

- **Cell value:** the **smallest swarm size `N` for which the swarm wins a
  majority of simulated battles** (`P(swarm win) > 0.50` over `R`
  Monte-Carlo runs). Not a single lucky all-crit outcome — a majority across
  runs. Cells that never cross by `n-max` render as `∞` (unbeatable within
  the cap).

Damage-type interaction is the punchline: kinetic (Crimson/Outer Rim) hits
shields at full effectiveness and armor at ×1.5, energy at 0.75/0.75, em at
1.0/1.0 — so the same-tier starters differ by empire, sometimes by an order
of magnitude, against the same target.

The matrix ships three ways:
1. A committed **swarm simulation mode** in `cmd/combat-sim`.
2. A **generated interactive KB page** at `kb/did_you_know/last_stand.html`
   presenting the full sortable matrix, with the Opus Magna / capital rows as
   the featured "did you know," plus per-cell drill-down (crossover curve +
   "the giant killed N of you").
3. A **real-battle validation** table (small non-wildlife 2v1 / 5v1 battles
   the engine reproduces), shown on the page and asserted in a golden test.

## 3. Global Constraints

- Go 1.24+; `b.Loop()` in benchmarks; `golangci-lint` clean; `go test ./...`
  green.
- Sleeps (none expected here) use `pkg/game/constants.go`.
- Hermetic: the sim reads only committed catalog snapshots and small JSON
  inputs — no network, no live server.
- **Stated modeling assumption, surfaced on the page:** *no special capital
  weapon bonus is applied or exists* — a capital mount does exactly its
  catalog `damage`. (This is moot for the headline case anyway: anything that
  one-shots a 145-HP starter kills exactly one per tick regardless of the
  exact number.)
- Binaries go in `bin/` (gitignored); never `git add -A` blindly.
- Reuse the existing measured model in `calibration.json`; do not re-fit
  combat constants.

## 4. Combat Model Recap (already measured, reused verbatim)

From `data/combat-sim/calibration.json` and the Phase-B work:

- **Hit chance by distance** (`hit_chance_by_distance`, index = zone_distance
  d0..d6): `[0.90, 0.80, 0.65, 0.50, 0.35, 0.22, 0.12]`. Deterministic at
  equal speed. The v1 1v1 `RunBattle` uses a **flat** 0.90; the swarm engine
  is the **zone/approach engine** the calibration comment anticipated (it
  consumes the distance array).
- **Damage pipeline** (`ResolveVolley`, unchanged): shield stage uses
  `shieldEff{energy .75, kinetic 1, void 0, explosive/em/thermal 1}`; hull
  stage uses `armorMult{energy .75, kinetic 1.5, void 1.5, explosive/em/thermal
  …}` with the saturating armor law `1 − counted/(counted+150)` above the
  crossover; flat DR capped 75%; crit = Weapons%×1 chance for ×1.5; brace
  0.25×, evade 0.5× / −0.20 hit; min-1 on any hull-reaching hit.
- **Shield regen** (`regen`): recharge/tick, ×2.5 while bracing, ÷3 on a
  tick you were hit, **no regen from zero** (`regen_from_zero=false`).
- **Ammo & reload (this analysis's assumption):** every ship carries a full
  hold and can reload indefinitely, so ammo never runs out. A magazine
  weapon fires its `magazine` shots (one per cooldown), then spends **one
  idle tick to reload** (refill the whole magazine), then resumes. Beam
  weapons (`magazine` = 0/−1) never reload. This is a **swarm-engine**
  addition; the existing flat 1v1 `RunBattle` (and its golden duel tests)
  keep their current no-reload, fire-until-empty behavior unchanged.
- **Stances:** only `fire` fires; brace/evade/flee measured. The swarm uses
  `fire` for both sides (the interesting question is raw attrition; stance
  strategy is out of scope for v1).

## 5. Engine Design

### 5.1 Weapon reach + distance (new)

The current `Weapon` struct drops `reach`; the flat engine ignores distance.
The swarm engine needs both:

- Add `Reach int` to `Weapon` (populate from `ItemDef.Reach` in `Resolve`).
- Battles open at **Outer = d6** and close **one ring/tick** toward **d0**,
  holding at d0. (Both headline sides are speed ~1; closing rate is a fixed
  1 ring/tick in v1 — see §9 for the faster-starter refinement.)
- A weapon may fire on a tick only when `currentDistance <= weapon.Reach`.
  Hit chance for that tick = `hit_chance_by_distance[currentDistance]`.

This reproduces the measured opening: the Opus Magna's reach-3/4 mounts open
first (d4), an autocannon swarm (reach 2) can't answer until d2 — the giant
gets free early kills.

### 5.2 Targeting — one target per tick

Confirmed from real battle frames (each ship carries a single `target_id`
per tick):

- **Swarm → giant:** all living attackers focus the one defender every tick.
- **Giant → swarm:** the defender holds one target; every ready mount fires
  at that one target. When it dies, the defender retargets next tick.
  Net effect: **the defender removes at most one attacker per tick** (fewer
  if its volley can't finish the target in one tick — which only helps the
  swarm).

### 5.3 Cohort model (the tractable core)

375 × 5 ≈ 1,875 cells, `N` up to tens of thousands: simulating `N`
individual structs is far too slow. All swarm ships are **identical**, share
one stance, one distance, and one target, so their only per-ship state is
alive/dead and (for the one under fire) current HP. Model the swarm as a
**homogeneous cohort**:

```
SwarmState {
    Healthy   int        // undamaged attackers (all identical)
    Focused   *SideState // the single attacker currently under the giant's guns (nil if none)
    Distance  int        // shared ring, d6..d0
}
DefenderState = *SideState  // the lone giant
```

Per tick:

1. Close distance (`d = max(d-1, 0)`).
2. **Swarm volley → defender:** every living attacker whose weapon is in
   reach fires. Aggregate damage is sampled as the sum of `Healthy` (+1 if
   `Focused` alive and in reach) independent per-ship volleys — hits drawn
   `Binomial(count, hitChance[d])`, crits drawn per landed hit — then run
   through `ResolveVolley` against the defender. (Sampling, not expectation,
   so `P(win)` has real variance near the boundary. For cd>1 weapons, the
   steady-state staggered firing fraction `1/cooldown` scales the firing
   count — see §5.4.)
3. **Defender volley → focused attacker:** the defender's ready, in-reach
   mounts fire at `Focused` (spawn one from `Healthy` if `Focused == nil`
   and `Healthy > 0`). Apply via `ResolveVolley`. If `Focused` dies, it is
   gone (not returned to `Healthy`); next tick a new one is drawn.
4. Regen both sides (`regen`).
5. Decrement cooldowns.
6. Terminal checks: defender hull ≤ 0 → **swarm win**; `Healthy == 0 &&
   Focused` dead/nil → **swarm loss**; `tick >= stalemate/max` → **loss**
   (giant survives = swarm failed to kill it).

This is **O(1) per tick regardless of N**. `N` enters only as the initial
`Healthy` count and the per-tick firing-count in step 2.

**Reference path:** a straightforward `RunSwarmDiscrete` that simulates `N`
real `SideState` attackers (capped to small `N`) exists for tests, and must
agree with the cohort model in distribution for small `N`, and with the real
battle fixtures.

### 5.4 Cohort cooldowns & reload

A weapon with cooldown `c` and magazine `m` runs a cycle of `m` shots
(one every `c` ticks) then 1 reload tick, so its **steady-state firing
fraction** is `m / (m·c + 1)` shots/tick (staggered across the cohort); a
beam weapon (`m` ≤ 0) never reloads and fires `1/c`. The cohort scales its
per-tick firing count by this fraction. The discrete reference path uses
true per-ship cooldown + reload-timer counters; the two agree in
expectation. This staggered fraction is the cohort's one documented
approximation. (Starter weapons are mostly cd-1: autocannon_i, pulse_laser_i;
em_disruptor_i is cd-2.)

### 5.5 Capital / any-tier defender resolve

`Resolve` currently rejects `tier >= 5` ("capital weapon bonus unmodeled").
Add `ResolveDefender` (or a `capitalOK bool` param) that:

- Accepts any tier.
- Reads the hull's `default_modules` as the fit (weapons + defense modules),
  exactly as the attacker starters do.
- Applies **no capital weapon bonus** (the stated assumption). Otherwise
  identical to `Resolve`.

Every catalog hull is resolvable as a defender this way; the 5 starters use
the normal `Resolve` path (all tier 0). A handful of hulls have no default
weapons — still valid defenders (they simply never fire back; the swarm wins
at N=1 as long as its own weapon is in reach).

### 5.6 Crossover search

`P(swarm win)` is monotonic non-decreasing in `N` (more identical attackers
is never worse: strictly more incoming damage and more bodies to absorb the
1-kill/tick attrition). So per cell, find the crossover with an **exponential (galloping) search
then bisect** — no fixed +1 stepping, and no need to probe the whole
`[1, n-max]` range:

1. **Double up:** probe `N = 1, 2, 4, 8, …`, estimating `P(win)` from `R`
   runs at each, until a probe dominates (`P(win) > 0.50`) or `N` exceeds
   `n-max`. This costs `~log2(N*)` probes where `N*` is the true crossover —
   cheap when the crossover is small (most cells), and it never assumes an
   upper bound beyond the `n-max` safety cap.
2. **Bisect:** binary-search the last doubling interval `[N/2, N]` (the last
   loss below, first dominant above) for the smallest `N` with
   `P(win) > 0.50`.

A light monotonicity guard re-probes if run-to-run noise inverts two
adjacent points. Total ≈ 2·log2(N*) probes/cell.
- `n-max` default 25,000 (configurable). Doubling passes it with no
  domination → cell = `∞`.
- Record, for the crossover `N`: `P(win)`, and the **median giant kill-count**
  (how many attackers it destroyed) — the "fifth-graders defeated" number.
- Also record a small **curve** (a handful of `N` around the crossover:
  win% and median kill-count) for the page's drill-down, cheaply reused from
  the binary-search probes.

## 6. CLI

Extend `cmd/combat-sim/main.go` with a swarm mode (new flags, mutually
exclusive with the existing `--a/--b` table mode and `--extract-fits`):

```
combat-sim --swarm <attacker-hull-id> --vs <defender-hull-id> \
           [--n-max 25000] [--runs 300] [--seed 42] [--json out.json]
```

- `--swarm`/`--vs` take catalog hull ids (default_modules fits) or a FitSpec
  path (reuse `LoadFit`).
- Prints a one-line result: crossover `N`, `P(win)` there, median kill-count,
  or `∞`.
- `--json` writes the full result incl. the drill-down curve.

A separate matrix driver (below) calls the same library functions in a loop;
the CLI single-cell mode is for spot checks and validation.

## 7. Matrix Generator + KB Page

New `cmd/generate-last-stand`:

1. Loads the catalog + calibration.
2. For each of ~375 defenders × 5 starter columns, runs the crossover search
   (§5.6), writing a matrix JSON to `data/combat-sim/last_stand_matrix.json`
   (committed): per cell `{ n, p_win, median_kills }` or `∞`, plus per-column
   starter metadata and the model/assumptions block.
3. Renders a self-contained interactive `kb/did_you_know/last_stand.html`
   (inline JS, no external libs — matching `warp_simulator.html`): the full
   sortable/filterable ~375×5 table (sort by any column, filter by
   tier/class/empire, search by ship name), capital rows highlighted, a
   featured **Opus Magna** callout, a per-cell drill-down (crossover curve
   canvas + kill-count), a damage-type explainer, and an **Assumptions** box
   (no capital weapon bonus; one target/tick; d6→d0 approach; `fire` stance;
   default_modules fits; cohort cooldown approximation).
4. Adds the card to `kb/did_you_know/index.html`.

Runtime budget: 1,875 cells × ~15–30 probes (galloping + bisect) × `R` runs, each battle O(ticks)
with the O(1)/tick cohort. With `R=300` and the tick cap, this is a few
minutes single-threaded; parallelize cells across goroutines if needed. The
generator is run on demand (like the other KB generators), not in CI.

## 8. Validation

- Pull real **non-wildlife 2v1 / 5v1** battles from the current bulk-feed
  month (`assets.spacemolt.com/public/v1/battles/YYYY-MM.ndjson.gz`), favor
  **short** ones, and `battle-export` a small set (`bin/battle-export --agent
  craftsman-boss --battle <ids>`; mind the 36s session-contention window).
- Resolve each participant's fit from the exported log, run them through the
  discrete multi-ship engine (not the identical-cohort path, since real
  battles are heterogeneous), and assert the predicted winning side matches
  and duration is in a sane band. Ship as a golden test
  (`swarm_golden_test.go`) and a small table on the page.
- The existing local fixtures (3/4/5/13/42-participant battles in
  `data/battles/`) seed the first tests before any new export.

## 9. Testing

- `Resolve`/`ResolveDefender`: capital hull resolves from default_modules;
  reach populated on weapons; no capital bonus applied.
- Cohort engine: N=1 reduces to a single-attacker battle equal (in
  distribution) to the discrete path; focus-fire attrition removes ≤1/tick;
  the immortality floor holds (a swarm whose in-reach chip < shield regen
  never wins → `∞`); `P(win)` monotonic in `N`.
- Approach: no side fires above its reach; giant opens before an autocannon
  swarm; hit chance tracks the distance array.
- Crossover search: binary search returns the true smallest majority-win `N`
  on a constructed monotonic fixture; `∞` when never crossing.
- Golden real-battle replays (§8).
- Generator smoke test (tiny catalog subset → well-formed matrix JSON +
  HTML).
- `go test ./...` and `golangci-lint` clean.

## 10. Assumptions & Limitations (v1)

- No capital weapon bonus (stated).
- Unlimited ammo with a 1-tick reload per emptied magazine (stated); no
  ammo run-out.
- **Default ammo only** — each weapon does its base catalog damage/type; no
  exotic rounds (AP/frag/void-antimatter etc.) or their damage modifiers
  (same ban as Phase-B calibration).
- `fire` stance both sides; no brace/evade/flee AI (the swarm doesn't kite
  or the giant doesn't brace) — raw attrition only.
- Approach closes a fixed 1 ring/tick; the measured **speed→hit modifier**
  and **faster-starter kiting** (e.g. Cobble speed 2, or tier-1 speed-6
  fighters) are **not** modeled in v1 (noted for v2 — the calibration already
  carries the speed modifier).
- Cohort cd>1 uses a staggered-fraction approximation (validated against the
  discrete path).
- Homogeneous swarm only (one starter model per column). Heterogeneous
  swarms are out of scope.

## 11. Future Extensions (designed-for, not built)

- Whole-catalog **any-attacker × any-defender** matrix (the engine already
  takes arbitrary StatBlocks; swap the 5-column set for all hulls).
- Speed/kite model (v2 zone+speed engine) and stance AI.
- "Credit-efficiency" view: cheapest swarm (by build cost) to kill each hull,
  joining the existing build-cost data.

## 12. File Map

- `cmd/combat-sim/swarm.go` — cohort + discrete multi-ship engines, crossover
  search.
- `cmd/combat-sim/resolver.go` — add `Reach` to `Weapon`; `ResolveDefender`
  (any-tier, default_modules, no capital bonus).
- `cmd/combat-sim/main.go` — `--swarm/--vs` mode.
- `cmd/combat-sim/swarm_test.go`, `swarm_golden_test.go` — tests.
- `cmd/generate-last-stand/main.go` (+ templates) — matrix JSON + page.
- `data/combat-sim/last_stand_matrix.json` — committed matrix output.
- `kb/did_you_know/last_stand.html`, `kb/did_you_know/index.html` — page +
  card.
- `data/battles/` — a few new small-battle validation fixtures.
- `cmd/combat-sim/README.md` — swarm mode + assumptions.

## 13. Errata / as-built

Deviations from this spec discovered during implementation. The `.superpowers/sdd/2026-09-02-swarm-threshold-matrix/progress.md` ledger is the authoritative record; this is a summary.

- **`pkg/combatsim` extraction (structural, not in §12's File Map).** §12 lists
  `cmd/combat-sim/swarm.go`, `resolver.go`, etc. — but `cmd/generate-last-stand`
  is a separate `package main` and cannot import another `package main`. Task 7
  extracted the entire reusable model (loader, resolver, engine, battle, table,
  swarm) into `pkg/combatsim`; `cmd/combat-sim` became a thin CLI wrapper. Every
  file this spec names under `cmd/combat-sim/*.go` (other than `main.go` itself)
  now lives under `pkg/combatsim/`. Gated by an unchanged-behavior `go test
  ./...` before and after the move; no behavior change.
- **Reload timer value.** §5-adjacent reload text (and the implementer's first
  pass) set the reload timer to 1 tick on an emptied magazine. With
  `tickWeapons` decrementing and refilling at the *end* of the tick, timer=1 is
  consumed the same tick it's set, yielding **zero** idle firing ticks. The
  spec's own acceptance test (`TestReloadCycle`, expecting exactly one idle
  tick: `[fire, fire, idle, fire]`) is binding, so the timer is set to **2** on
  empty — end-of-tick decrement then leaves exactly one idle tick before the
  weapon fires again.
- **`binomial` sampler gate.** The plan's reference design gated the normal
  approximation on a raw trial-count threshold (`count > 30`). That gate is
  biased in the low-`np`/low-`n(1-p)` corner reachable by long-range shots
  (e.g. `count=31, p=0.02`: normal approximation overstated the mean by ~13%
  and diverged sharply on `P(k=0)`). The as-built gate instead checks
  `np`/`n(1-p)` against a De Moivre–Laplace threshold of 9: below it, a Poisson
  (Knuth) sample is used instead of the normal approximation. Both remain O(1)
  per call. Related, not a deviation from any explicit spec text but worth
  recording: the attacker's `WeaponSkillPct` is folded into the cohort's
  per-volley expected-damage profile, and defender regen-on-hit is applied in
  the bulk-damage path — both required for `TestSwarmAgreesWithMultiShip` to
  pass.
- **`Crossover` gallop-then-bisect edge case.** The plan's reference gallop
  (double until the probe exceeds `n-max`, then declare ∞) misses a real
  crossover strictly between the largest probed power of two and a non-power-
  of-two `n-max` — and the shipped default `--n-max` is 25000, not a power of
  two. As-built, on first overflow the search additionally probes `n-max`
  itself before concluding ∞; if `n-max` dominates, it bisects
  `(largest non-dominant power of two, n-max]` instead. Regression test:
  `TestCrossoverNMaxNotPowerOfTwo`.
- **`--runs` flag reuse, not a new flag.** §9's CLI extension implied a new
  flag for swarm-mode run count; `--runs` already exists for `--a/--b` table
  mode (default 10000) and Go's `flag` package can't register two flags under
  one name, and changing the shared default would alter table-mode behavior.
  As-built: `--swarm` mode reuses `--runs`, applying its own default of 300
  only when the user didn't pass `--runs` explicitly (checked via
  `flag.Visit`).
- **`BuildMatrix` parameter order and missing-cell representation.** The
  as-built signature is `BuildMatrix(cat, cal, attackerIDs, defenderIDs, ...)`
  — attackers first, defenders second (opposite of how §7/§8 prose describes
  the loop order). An attacker or defender hull that fails to resolve is
  **omitted** from the matrix (no key/row) rather than emitted with an
  explicit `null` — a consumer must treat a missing per-attacker cell as
  unresolvable/∞, not distinguish it from a present `n:0` (∞ within `--n-max`)
  cell by key absence alone. Also: `starter → empire` is a small hardcoded
  table in the generator, not read from `ShipDef.Faction` — the catalog's
  `ShipDef` doesn't expose an empire/faction field.
- **`swarmMaxTicks = 4000`.** Both `--swarm` CLI mode and
  `generate-last-stand` cap swarm battles at a fixed 4000 ticks (independent
  of `--max-ticks`, which only applies to `--a/--b` table mode) — generous
  headroom for slow grinds against tough capital defenders.
- **Validation coverage (§9/Task 9), disclosed not deviated:** 3 real battle
  fixtures validate winner-match (no wrong winners across all runs); 3 further
  NPC fixtures were unresolvable and are skipped. No fixture exercises N>2
  ships per side, so multi-ship focus-fire dynamics rest on the cohort model's
  self-consistency against `RunMultiShip` (the `TestSwarmAgreesWithMultiShip`
  family, ±0.15 win-rate tolerance) rather than a real multi-ship battle log;
  likewise the 30/335 mixed-damage-type hulls skipped as defenders (see the
  README) are unrated. Both are recorded as v2 follow-ups, not silently
  dropped.
