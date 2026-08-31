# Combat Simulator (cmd/combat-sim) — Design

2026-08-31. Approved in-session; implements the ship-vs-ship phase (phase A)
of the combat-sim effort. Phases B (live calibration duels) and C (wildlife
stat harvesting + wildlife-vs-ship) are out of scope here and noted at the end.

## Goal

A hermetic Go CLI in the KB repo that Monte-Carlo simulates 1v1 combat
between two ship fittings using the empirically verified damage model
("model v2", `data/battles/analysis/verify4.py`..`verify6_percentage_armor.py`,
memory note `project_combat_mechanics_from_logs`), and prints a stance-pair
outcome table.

Anyone with a clone of this repo can run it: inputs are the committed catalog
snapshots (`data/snapshots/latest/catalog_{ships,items,skills}.json`) plus two
small fitting-spec JSON files. No databases, no network, no credentials.

## CLI

    bin/combat-sim --a fits/a.json --b fits/b.json \
        [--runs 10000] [--seed 42] [--catalog data/snapshots/latest] \
        [--calibration data/combat-sim/calibration.json] \
        [--max-ticks 500] [--json out.json]

- `--runs` per stance-pair cell (default 10000; ~±1pp on a proportion).
- Deterministic under `--seed` (one seeded RNG; per-cell sub-seeds derived).
- Output: 4×4 ASCII table (A stance rows × B stance cols: fire/brace/evade/flee),
  each cell an outcome distribution `A-kill / B-kill / A-fled / B-fled /
  stalemate / mutual`, plus dominant outcome %. Cells whose result depends on
  an ASSUMED calibration entry are marked with `*`. `--json` dumps full
  distributions.

## Fitting spec (input file)

```json
{
  "name": "MoltenOne-Broadaxe",
  "hull": "broadaxe",
  "modules": ["autocannon_ii","autocannon_ii","autocannon_ii","flak_cannon_ii","armor_plate_ii"],
  "skills": {"weapons":7,"gunnery":10,"shields":4,"armor":0}
}
```

Unknown module/hull ids are a hard error naming the id. Skills default to 0.

## Components (single package, cmd/combat-sim/*.go)

### loader.go
Reads the three catalog JSONs into typed structs. Snapshot items carry combat
fields flattened at top level (`damage`, `damage_type`, `reach`, `cooldown`,
`magazine_size`, `combat_effects`, `shield_bonus`, `armor_bonus`,
`damage_reduction`, `shield_recharge_bonus`, `slot`). Per project rules: match
actual field names in the JSON, do not assume.

### resolver.go — fit + skills → StatBlock
- maxHull = hull.base_hull; maxShield = base_shield + Σ shield_bonus;
  recharge = base_shield_recharge + Σ shield_recharge_bonus. NO capacity
  skill multipliers: measured across all fixtures, max_hull/max_shield equal
  catalog + module bonuses EXACTLY (broadaxe 28 at Shields 4, survey 400 at
  Shields 1) — capacity skills do not appear in the stat block.
- armorTotal = base_armor + Σ armor_bonus, × (1 + Armor×1% armorEffectiveness)
- flatPct = min(75, Σ damage_reduction) — flat/adaptive bucket, capped 75
  (dev-confirmed; a module's `adaptive_resistance_N` special and its
  `damage_reduction` are ONE number — never sum both)
- typedResists: from hardener modules' resistance_bonus per damage type,
  additive, capped 75 (none in current test fits; include for completeness)
- weapons: []{damage, type, cooldown, magazineSize} from weapon-slot modules
- weaponSkillPct = Weapons + Gunnery (matching-type; the Gunnery key→damage
  type mapping is UNKNOWN (memory), so v1 applies Gunnery to ALL weapon types,
  documented as an approximation; capital-hull double-Gunnery excluded, v1
  refuses capital-class hulls with a clear error)
- critChance = Weapons × 1% (measured law; description text 0.2% is a
  confirmed doc bug)

### engine.go — one simulated battle, tick loop
State per side: hull, shield, ammo per weapon, cooldown timers, stance, fled?.
Each tick, BOTH volleys are computed from start-of-tick state, then applied
simultaneously (matches logs: both attack entries per tick; both may die →
`mutual`).

Volley resolution (attacker → target), the verified pipeline:
1. Weapons off cooldown and with ammo fire; each rolls crit (critChance);
   weapon damage w' = floor(w × 1.5) on crit else w. raw = Σ w'.
2. pre = floor(raw × (1 + weaponSkillPct/100)).
3. stance: pre = floor(pre × stanceInMult(target.stance))
   (brace 0.25 measured; evade = calibration `evade_in_mult` ASSUMED 0.5;
   fire/flee 1.0 measured).
4. One hit roll vs hitChance (calibration; see below). Miss → nothing.
5. If target shield > 0:
   - x1 = floor(pre × (1 − shieldsSkill/100))
   - typed resists: x1 = floor(x1 × (1 − typed/100)) (dev-confirmed bucket
     order: shield skill → typed → flat; with shields down, typed applies to
     pre directly)
   - e by type: energy 0.75, kinetic 1.0, void 0 (skips shields entirely),
     explosive/EM/thermal 1.0 (doc "Full"; unmeasured — flag)
   - drain = floor(floor(x1 × e) × (1 − flatPct/100))
   - if shield ≥ drain: shield −= drain; spill points: each stage above whose
     truncated fraction ≥ 0.5 spills 1 to hull, then × (1 − flatPct/100)
     floored; spill bypasses armor (all measured).
   - else breakthrough: consumed = floor(shield / e) (e>0), shield = 0,
     hullIn = pre − consumed  (NO shields-skill term in this branch — measured)
6b. else hullIn = floor(pre × (1 − typed/100)).
8. Armor on hullIn: counted = armorTotal × typeMult (kinetic/void 1.5,
   explosive/EM 1.0, energy 0.75, thermal 0.25). Reduction law from
   calibration `armor_law`:
   - "auto" (default): percentage `counted/(counted+150)` when counted ≥ 12,
     flat `floor(armorTotal × 0.75)` below (each exact in its measured range;
     crossover flagged OPEN)
   - "pct150" | "flat75": force either.
   hull damage = max(1, floor(hullIn × (1−f))) when hullIn ≥ 1 (min-1,
   dev-stated), capped at remaining hull.
9. Regen at end of tick (after volleys): +recharge if not hit this tick,
   +floor(recharge/3) if hit (measured net-regen law), capped at maxShield.

Flee stance: does not fire (matches fled drones logging no attacks); each tick
after the first, escape roll vs calibration `flee_escape_per_tick` (ASSUMED
0.25); success ends the run as `X-fled`. Speed differential ignored in v1
(flagged). Evade: fires normally, `evade_in_mult` reduction (ASSUMED).
Brace: fires normally (assumption, flagged).

Tick cap (`--max-ticks`, default 500) → `stalemate`.

### calibration.json (data/combat-sim/calibration.json)
Every entry carries `"source": "measured" | "ASSUMED"` and a comment string:
- hit_chance_base: 0.95 (measured cap at engaged; per-pair variance 0.79–0.95
  unexplained — sweepable via flag `--hit-chance a=0.9,b=0.85`)
- brace_in_mult 0.25 measured; evade_in_mult 0.5 ASSUMED;
  flee_escape_per_tick 0.25 ASSUMED; armor_law "auto"; regen_hit_divisor 3
  measured.

### table.go / main.go
Runs 16 cells × runs, prints table + flags, optional JSON.

## Testing (golden replay — the load-bearing tests)
`engine_test.go` replays the three fixtures with FORCED rolls (hit/crit taken
from the logs) and asserts volley-by-volley equality with observed
shield/hull damage:
- 509e1ef4 (raw): all 8 attacks (incl. flat-70, breakthrough, kill cap)
- b7847bbc: 6 MoltenOne volleys + autocannon volley (spill case)
- 7c044558: kinetic drains (net-regen), crit breakthrough, shields-down
Broadaxe energy-hull rows assert within ±1 (documented open alternation).
Plus: resolver unit tests against the known fits' max_shield/max_hull, and a
determinism test (same seed → same table). `go build ./... && go test ./...`
and golangci-lint clean; go 1.24 idioms (range-over-int, b.Loop in benches).

## Known limitations (documented in --help and README)
- Drone repair is invisible in logs → not modeled; drone-fit survival is
  underestimated.
- Boarding stance, zones/movement (fixed at engaged), multi-ship, ammo reload,
  armor-melt debuffs, EM debuff (−speed/−damage), capital hulls: out of scope v1.
- hit_chance formula and evade/flee parameters are calibration inputs, not
  measurements — phase B (scripted stance-pair duels between owned agents,
  ~11 rounds per pair) exists to measure them.
- Phase C: wildlife StatBlock source fitted from harvested logs
  (wildlife_attacks/wildlife_kills; today only 11/42 species have any samples).

## File layout
    cmd/combat-sim/{main,loader,resolver,engine,table}.go + *_test.go
    data/combat-sim/calibration.json
    data/combat-sim/fits/*.json (example fits: the three fixture ships)
