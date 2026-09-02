# combat-sim

Monte-Carlo simulator for 1v1 SpaceMolt ship combat. Give it two ship
fittings; it fights every stance pairing 10,000 times each and prints a 4×4
table of outcome distributions — who kills whom, who escapes, what
stalemates.

The damage model is not guessed: it was reverse-engineered from real
`get_battle_log` exports and is locked in by 14 golden replay tests that
assert, volley by volley, the exact shield/hull numbers the server logged in
three committed battle fixtures (`data/battles/509e1ef4*`, `b7847bbc*`,
`7c044558*`). If a code change breaks the model, the golden tests fail.

Fully hermetic: reads only files committed to this repo (a pinned catalog
under `data/combat-sim/catalog/`, snapshot 20260827). No database, no
network, no credentials, no live agents.

The reusable combat model — catalog loading, fit resolution, the tick
engine, the 1v1 table runner, and the swarm/multi-ship engines below —
lives in `pkg/combatsim`. `cmd/combat-sim` is a thin CLI over that
package; `cmd/generate-last-stand` (see below) is a second, independent
consumer of the same package.

## Quick start

    go build -o bin/combat-sim ./cmd/combat-sim
    bin/combat-sim --a data/combat-sim/fits/molten_broadaxe.json \
                   --b data/combat-sim/fits/artis_survey.json

Output (10,000 runs per cell, deterministic under `--seed`):

    A \ B     fire            brace            evade           flee
    fire      B-kill 100%     stalemate 100%   A-kill 78%      B-fled 100%
    brace     B-kill 94%      stalemate 100%   stalemate 100%  B-fled 100%
    ...
    * cell depends on ASSUMED calibration: regen_from_zero (see calibration.json)

Each cell names the dominant outcome (`A-kill`, `B-kill`, `A-fled`,
`B-fled`, `mutual`, `stalemate`) and its share. Cells marked `*` rest on
calibration entries that are assumptions rather than measurements — see
below. `--json out.json` dumps the full per-cell distributions.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--a`, `--b` | (required) | fitting-spec JSON for each side |
| `--runs` | 10000 | battles per stance-pair cell (~±1pp at 10k) |
| `--seed` | 42 | RNG seed; same seed → identical table |
| `--catalog` | `data/combat-sim/catalog` | catalog snapshot dir |
| `--calibration` | `data/combat-sim/calibration.json` | tunables; missing file → built-in defaults |
| `--max-ticks` | 0 | override the calibration's hard tick cap (0 = keep; the 30-tick no-kill stalemate rule usually ends battles first) |
| `--json` | | write full outcome distributions to this file |
| `--extract-fits` | | battle id: write one fit per participant and exit (see below) |
| `--battles` | `data/battles` | battle fixture dir for `--extract-fits` |
| `--out` | `data/combat-sim/fits` | output dir for `--extract-fits` |
| `--swarm` | | attacker hull id: run swarm-crossover mode against `--vs` and exit (see below) |
| `--vs` | | defender hull id (`--swarm`) |
| `--n-max` | 25000 | largest swarm size probed before reporting the crossover as ∞ (`--swarm`) |
| `--swarm-json` | | write the full crossing (curve included) as JSON to this file (`--swarm`) |

`--runs` is shared between table mode and swarm mode: its flag default is
10000 (table mode), but `--swarm` mode applies its own default of 300 when
`--runs` isn't explicitly passed (a `flag.Visit` check — Go can't register
two flags under one name). Pass `--runs` explicitly to override either
mode's default.

`--swarm` mode ignores `--max-ticks`: it always runs with a fixed
`swarmMaxTicks = 4000` internal cap, generous enough for slow grinds
against tough capital defenders. `--max-ticks` only affects `--a/--b`
table mode.

## Fitting specs

The easiest way to make one: open the KB fitting sheet
(`kb/ships/fitting.html`), build the fit, hit **EXPORT**. It downloads
`fit-<ship>.json` (and copies the same JSON to the clipboard) looking like
this — hull and modules filled in from the sheet, skills zeroed:

```json
{
  "name": "Broadaxe fit",
  "hull": "broadaxe",
  "modules": [
    "autocannon_ii",
    "autocannon_ii",
    "autocannon_ii",
    "flak_cannon_ii",
    "armor_plate_ii"
  ],
  "skills": {
    "weapons": 0,
    "gunnery": 0,
    "shields": 0,
    "armor": 0
  }
}
```

The **only edit you make is the `skills` block** — replace the zeros with
the pilot's actual levels (in-game: `get_skills`). For example, MoltenOne's
Broadaxe from battle 7c044558 flies with:

```json
  "skills": {
    "weapons": 7,
    "gunnery": 10,
    "shields": 4,
    "armor": 0
  }
```

The skills matter: at all-zero skills this Broadaxe's fire-vs-evade cell
is a 79% stalemate (it cannot chew through the survey fit's shields inside
the 30-tick stalemate window); at the real levels it becomes a 78% kill. `weapons` + `gunnery` scale damage
(+1%/level each) and `weapons` sets crit chance (1%/level); `shields` is
the shield-resist and only bites while shields hold; `armor` scales armor
effectiveness (+1%/level). Any key omitted defaults to 0; other keys are
ignored.

The resolver refuses capital hulls (tier 5+), mixed-damage-type weapon
loadouts, and unknown ids — each with an error naming the problem.

### Extracting fits from a battle

Any exported battle in `data/battles/` can be turned into ready-to-run
fits, one file per participant, named `<battle_id>_<player_id>.json`:

    bin/combat-sim --extract-fits 7c044558c0c39e972fe560110f69ea25
    # wrote data/combat-sim/fits/7c044558…_a5092491….json  hull=survey_vessel … skills W3/G3/S1/A0
    # wrote data/combat-sim/fits/7c044558…_b195177b….json  hull=broadaxe … skills W7/G10/S4/A0

Hull and modules come straight from the participant records. **Skills are
inferred from the `.raw.json` battle log** when it sits next to the
fixture: `weapons` from per-weapon `crit_chance` (1%/level, rolled even on
misses so it is always visible), `gunnery` from `weapon_skill_pct` minus
weapons, and `shields` from the `shield_resist_pct` on attacks *against*
that pilot (observable only if they were shot at while their shields
held). `armor` never appears in any battle log and is always written as
0 — edit it in if you know it. Without a raw log every skill falls back to
0 and a warning says so. Participants whose hull is not a catalog ship
(stations, creatures) are skipped with a warning.

The extracted pair above, fed back in as `--a`/`--b`, reproduces the real
battle's outcome (Artis's survey vessel kills MoltenOne's Broadaxe in the
fire/fire cell).

## Swarm mode

`--swarm` answers a different question than the 1v1 table: not "who wins
this fight" but "how many identical, unskilled attackers does it take to
beat this one defender by attrition." It finds the **crossover** — the
smallest swarm size `N` whose simulated win rate exceeds 50% — via
exponential doubling then bisection, so it visits roughly `2·log2(N)`
swarm sizes rather than a linear scan:

    bin/combat-sim --swarm shard --vs opus_magna \
                   --n-max 25000 --runs 300 --swarm-json crossing.json

    shard swarm vs opus_magna: crossover N=32 (P=0.51), opus_magna kills 31

`--swarm`/`--vs` take catalog hull ids resolved to their `default_modules`
stock fitting at zero skills — not FitSpec JSON files. The attacker
resolves through the non-capital path (`ResolveHull(id, cat, false)`); the
defender through the capital-allowed path (`ResolveHull(id, cat, true)`),
since this mode exists specifically to answer "how many starters does it
take to bring down a titan." If no swarm size up to `--n-max` reaches a
majority win rate, the crossover is reported as ∞ (`N=0` in the JSON — see
below). `--swarm-json` writes the full `Crossing` (the winning point plus
every `{n, p_win, median_kills}` probed along the way) so a page can
render the curve without re-running the search.

**Two engines, one behind the CLI:** `RunSwarm` (`pkg/combatsim/swarm.go`)
is the fast path this mode uses — a homogeneous-cohort model that tracks
only a live headcount plus one "focused" attacker taking the defender's
exact per-ship volleys, so a tick costs O(1) regardless of swarm size
(binomial sampling over the cohort, exact Bernoulli trials below 30
attackers, a Poisson or normal approximation above it depending on how
extreme the per-tick hit probability is). `RunMultiShip` is the slower,
exact reference engine — a real `sideState` per ship, everyone targeting
the lowest-index living enemy — used in tests to confirm the cohort model
tracks it (see errata below) and available for small heterogeneous
battles the cohort model can't represent (mixed attacker types).

**The model:** both sides start at ring distance 6 and close one ring per
tick; a weapon only fires once distance is within its `Reach`. Every
combatant fights in `fire` stance for the whole battle — no brace, evade,
or flee AI. Ammo is unlimited but an emptied magazine costs a 1-tick
reload (exactly one idle firing tick — see errata). Weapons fire their
default ammo only (base catalog damage/type). Every ship — attacker or
defender — targets exactly one opponent per tick, so a lone defender can
kill at most one attacker per tick no matter how large the swarm; that
one-kill-per-tick ceiling is what makes the crossover scale roughly with
`√(effective HP / per-ship damage)` rather than with raw HP, which is why
a stock Opus Magna (a titan-class capital) falls to on the order of
30–130 starters rather than thousands — see the worked numbers in
`generate-last-stand` below. Attacker swarms get **no capital weapon
bonus**: the resolver only ever grants that bonus on the defender side,
since the swarm attackers are always non-capital hulls.

## generate-last-stand

`cmd/generate-last-stand` builds the full "swarm threshold matrix": for
every hull in the catalog (as defender), the crossover swarm size against
each of the 5 empire-starter hulls (as attacker), using the same
`pkg/combatsim.Crossover`/`RunSwarm` machinery as `--swarm` mode above.

    go build -o bin/generate-last-stand ./cmd/generate-last-stand
    bin/generate-last-stand --out data/combat-sim/last_stand_matrix.json \
                            --page kb/did_you_know/last_stand.html

| Flag | Default | Meaning |
|---|---|---|
| `--catalog` | `data/combat-sim/catalog` | catalog snapshot dir |
| `--calibration` | `data/combat-sim/calibration.json` | tunables; missing file → built-in defaults |
| `--runs` | 300 | battles per probed swarm size |
| `--n-max` | 25000 | largest swarm size probed before reporting ∞ |
| `--out` | `data/combat-sim/last_stand_matrix.json` | matrix JSON output path |
| `--page` | | matrix HTML page output path (empty = skip) |
| `--limit` | 0 | limit to the first N defenders in catalog-id order (0 = all; for smoke runs) |

The 5 attacker columns are fixed: `shard` (Crimson), `prospect` (Nebula),
`cobble` (Outer-Rim), `theoria` (Solarian), `threshold` (Voidborn) — one
stock starter per empire. (The catalog's `ShipDef` doesn't expose faction
as a field, so the id→empire mapping is a small hardcoded table in the
generator, not derived from catalog data.) Every other hull in the
catalog is a defender row. Defenders resolve through the capital-allowed
path, so a titan like `opus_magna` can appear as a row; attacker columns
always resolve through the non-capital path.

Rows are computed by a worker pool (bounded by `GOMAXPROCS`); each cell's
RNG seed is derived deterministically from the `(defender, attacker)` id
pair (an FNV hash), so the matrix is reproducible regardless of
scheduling order.

**Matrix JSON schema:**

```json
{
  "generated_utc": "2026-09-02T16:00:00Z",
  "assumptions": ["attackers use the stock (default_modules) fitting...", "..."],
  "columns": [{"id": "shard", "name": "...", "empire": "crimson", "weapon": "2× Autocannon I", "damage_type": "kinetic"}, ...],
  "rows": [
    {
      "ship_id": "opus_magna", "name": "...", "tier": 5, "class": "...",
      "cells": {
        "shard": {"n": 32, "p_win": 0.51, "median_kills": 31, "curve": [{"n": 1, "p_win": 0.0, "median_kills": 0}, ...]}
      }
    }
  ],
  "notes": ["skipped defender \"annihilation\": mixed-damage-type fits unsupported in v1 (got energy and em)", "..."]
}
```

A defender row's `cells` map is keyed by attacker column id. **A missing
key means that attacker column failed to resolve entirely** (see
`notes`); it is not a per-cell outcome and consumers should not confuse
it with a measured result. Within a present cell, `n == 0` means the
measured crossover exceeded `--n-max` — treat it as ∞, not "zero
attackers needed." As of the snapshot in this repo, 30 of 335 catalog
hulls are skipped as defenders entirely (not just individual cells),
all for the same reason: the v1 fit resolver doesn't support a hull whose
`default_modules` mix more than one weapon damage type. That's recorded
per-hull in `notes`; the other 305 defenders resolve and get a full row.

## The model (measured)

Per tick, both sides volley simultaneously: one hit roll per volley, one
crit roll per weapon (floor(dmg×1.5)), `pre = floor(raw × (1+weapons+gunnery))`.
Mitigation follows the dev-confirmed order — target's Shields skill, then
the damage-type shield split (energy drains shields at 75% efficiency,
kinetic full, void skips shields entirely), then flat/adaptive reduction
(capped 75%) — with integer truncation at each stage, a measured ±1 "spill"
rule, breakthrough consuming `floor(pool/e)`, saturating percentage armor,
a minimum of 1 hull damage per connecting hit, and damage capped at
remaining hull. Shield regen is `floor(recharge/3)` on hit ticks, full
otherwise, and does not restart from zero. Only the fire stance fires:
braced and fleeing ships are measured silent (513 braced / 1,763 flee
ticks in the Haven fixture, zero shots) and evade is documented "Can
Fire: No" (skill.md stance table; zero evade ticks exist in any export).
Per that table, brace takes 25% damage with 2x shield regen, evade takes
50% with a -20% debuff to the attacker's accuracy, and flee escapes after
3 consecutive flee ticks at equal speed. Phase B measured the full flee
law as `max(1, 3 + 2·(pursuer_speed − fleer_speed))` — pure speed, no
Tactics term — but the v1 engine models only the equal-speed base of 3
(see Measured vs ASSUMED). A battle with no kill by tick 30 is a stalemate
draw (skill.md).

Full derivation: `docs/superpowers/specs/2026-08-31-combat-sim-design.md`
and the analysis scripts in `data/battles/analysis/`. The v0.574.3 docs
(2026-09-01) later confirmed two reverse-engineered pieces officially:
crit is 1%/Weapons-level (the old 0.2% text was a doc bug), and the
Damage Types table now states "Armor x1.5" for kinetic and void — the
exact armor-counted multipliers this engine uses.

## Measured vs ASSUMED

Every tunable lives in `data/combat-sim/calibration.json`, each tagged by
provenance. Phase B (scripted stance-pair duels between owned agents,
2026-09-02) turned almost everything here from assumed/doc-backed into
measured — the raw battle logs carry the server's exact `hit_chance`,
`hit_roll`, and `hit_success` on every shot, so a handful of volleys pins
each value with no statistics.

**Measured (Phase B controlled duels):**
- Hit table by `zone_distance` (equal speed): d0 0.90, d1 0.80*, d2 0.65,
  d3 0.50, d4 0.35, d5 0.22, d6 0.12 — deterministic (d1 inferred; it
  never settled at distance 1). `hit_chance_a/b` are set to the engaged
  (d0) 0.90; the full array is in `hit_chance_by_distance` for the future
  zone engine (the v1 engine still pins both sides at engaged).
- Speed → hit_chance: asymmetric on `attacker_speed − target_speed` at d0
  — +2 → 0.95, 0 → 0.90, −1 → 0.81, −2 → 0.78 (a faster target is harder
  to hit; the slow-attacker penalty is steeper than the fast bonus).
- Brace regen: **2.5× recharge, flat** — S4c fitted a shield booster
  (max shield 50 → 75) and braced un-hit regen stayed ~2.5/tick, ruling
  out a 5%-of-max law (would have been 3.75). Corrected from the doc's 2×.
- Flee: `flee_required = max(1, 3 + 2·(pursuer_speed − fleer_speed))` —
  pure speed, **no Tactics term** (four points across S6a/S6c2/S6d fit
  exactly; a Tactics-2 pilot's escape-in-1 was fully his +1 speed).
- `regen_from_zero` **false** (S5 + 40+ wildlife timelines) — now
  measured, no longer assumed.

**Doc-backed (skill.md stance table):** evade 0.5× incoming + 0.20
accuracy debuff, brace 0.25× incoming, stalemate at 30 kill-less ticks —
all consistent with measurement.

**Still assumed / not modeled:** the v1 engine is a flat-hit,
single-distance model, so `hit_chance_by_distance`, the speed→hit
modifier, and the flee speed law are recorded in calibration.json
(`measured_not_modeled`) but not yet consumed — wiring them in is the v2
zone/speed engine, a design change beyond calibration. Tank note (S8): a
70% DR hull plus shield-type absorption stacks to ~90% effective (pulse
10 raw → 1 net on a 600 shield). Table cells depending on a
not-yet-modeled entry carry `*`.

## Not modeled in v1

This section describes the `--a/--b` 1v1 table engine (`RunBattle`)
specifically — the swarm engines above are a separate code path and do
model zones/reach, ammo reload, and capital hulls (as defenders); see
Swarm mode above for what they model instead.

Drone repair (invisible in battle logs — drone-fit survival is
underestimated), boarding, zones/movement (both sides fixed at engaged),
ammo reload mid-fight, armor-melt and EM debuffs, typed hardener resists,
capital hulls, wildlife (phase C: harvest creature stats from battle logs,
then a species becomes just another stat block). Gunnery is applied to all
damage types (matches the v0.574.3 "all weapon types" description; the
per-type bonus-key mapping remains unconfirmed). Skill caps are not
modeled: v0.574.3 reveals crit chance caps and the weapon-damage skills
share a combined cap, but neither cap value is published — measurements
prove crit cap ≥ 20% and damage cap ≥ +25%, so caps are irrelevant at the
skill levels (≤ 10) these fits use.

## Files

    pkg/combatsim/loader.go       catalog JSON → typed defs
    pkg/combatsim/resolver.go     fit + skills → combat stat block; ResolveHull for --swarm
    pkg/combatsim/engine.go       ResolveVolley: the golden-tested mitigation pipeline
    pkg/combatsim/battle.go       tick loop, stances, regen, flee, calibration
    pkg/combatsim/table.go        Monte Carlo runner + stance table (--a/--b mode)
    pkg/combatsim/swarm.go        RunMultiShip, RunSwarm, Crossover — the swarm engines
    pkg/combatsim/extract.go      --extract-fits: battle log → FitSpec
    cmd/combat-sim/main.go        thin CLI: table mode, --extract-fits, --swarm/--vs
    cmd/generate-last-stand/      matrix builder (main.go) + HTML page renderer (render.go)
    data/combat-sim/              calibration.json, example fits, vendored catalog, last_stand_matrix.json
