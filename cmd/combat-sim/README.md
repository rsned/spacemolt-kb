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

    cmd/combat-sim/loader.go     catalog JSON → typed defs
    cmd/combat-sim/resolver.go   fit + skills → combat stat block
    cmd/combat-sim/engine.go     ResolveVolley: the golden-tested mitigation pipeline
    cmd/combat-sim/battle.go     tick loop, stances, regen, flee, calibration
    cmd/combat-sim/table.go      Monte Carlo runner + stance table
    data/combat-sim/             calibration.json, example fits, vendored catalog
