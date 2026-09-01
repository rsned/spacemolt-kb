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

## Quick start

    go build -o bin/combat-sim ./cmd/combat-sim
    bin/combat-sim --a data/combat-sim/fits/molten_broadaxe.json \
                   --b data/combat-sim/fits/artis_survey.json

Output (10,000 runs per cell, deterministic under `--seed`):

    A \ B     fire            brace            evade           flee
    fire      B-kill 100%     A-kill 100%      B-kill 100%*    B-fled 91%*
    brace     B-kill 100%     stalemate 100%   B-kill 100%*    B-fled 100%*
    ...
    * cell depends on ASSUMED calibration: evade_in_mult, flee_escape_per_tick, ...

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
| `--max-ticks` | 0 | override the calibration's stalemate cap (0 = keep) |
| `--json` | | write full outcome distributions to this file |

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

The skills matter: this fit's evade-vs-fire cell swings from ~67% at all
zeros to ~98% at the real levels. `weapons` + `gunnery` scale damage
(+1%/level each) and `weapons` sets crit chance (1%/level); `shields` is
the shield-resist and only bites while shields hold; `armor` scales armor
effectiveness (+1%/level). Any key omitted defaults to 0; other keys are
ignored.

The resolver refuses capital hulls (tier 5+), mixed-damage-type weapon
loadouts, and unknown ids — each with an error naming the problem.

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
otherwise, and does not restart from zero. Braced and fleeing ships do not
fire (measured: 513 braced ticks / 1,763 flee ticks in the Haven fixture,
zero shots).

Full derivation: `docs/superpowers/specs/2026-08-31-combat-sim-design.md`
and the analysis scripts in `data/battles/analysis/`.

## Measured vs ASSUMED

Every tunable lives in `data/combat-sim/calibration.json`, each tagged by
provenance. Measured: brace 0.25× incoming, regen divisor 3, armor
constants. ASSUMED until calibrated: `evade_in_mult` (0.5),
`flee_escape_per_tick` (0.25), `regen_from_zero` (false), and per-pair hit
chances beyond the measured 0.79–0.95 envelope. Table cells depending on an
assumed entry carry `*`. Phase B (scripted stance-pair duels between owned
agents) exists to measure them.

## Not modeled in v1

Drone repair (invisible in battle logs — drone-fit survival is
underestimated), boarding, zones/movement (both sides fixed at engaged),
ammo reload mid-fight, armor-melt and EM debuffs, typed hardener resists,
capital hulls, wildlife (phase C: harvest creature stats from battle logs,
then a species becomes just another stat block). Gunnery is applied to all
damage types (the real per-type mapping is unknown).

## Files

    cmd/combat-sim/loader.go     catalog JSON → typed defs
    cmd/combat-sim/resolver.go   fit + skills → combat stat block
    cmd/combat-sim/engine.go     ResolveVolley: the golden-tested mitigation pipeline
    cmd/combat-sim/battle.go     tick loop, stances, regen, flee, calibration
    cmd/combat-sim/table.go      Monte Carlo runner + stance table
    data/combat-sim/             calibration.json, example fits, vendored catalog
