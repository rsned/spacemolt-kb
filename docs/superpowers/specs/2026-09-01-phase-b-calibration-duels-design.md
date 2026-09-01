# Phase B: Calibration Duels — Design

2026-09-01. Approved in-session. Measures the remaining combat-model
unknowns with scripted duels between two dedicated owned agents, and
lands the results in cmd/combat-sim's calibration, fixtures, and tests.

## Goal

Every calibration entry in `data/combat-sim/calibration.json` either
carries a measured value with a fixture behind it, or is explicitly
deferred with a reason. Concretely:

1. **Hit-chance table by ring** (base per zone_distance 0–6) — replaces
   the guessed flat 0.95; feeds the future v2 zones engine.
2. **Speed modifier on hit chance** — docs claim ±5 speed shifts up to
   ±30%; measure the actual function.
3. **Evade** — verify the −0.20 accuracy debuff, the 0.5× damage taken,
   the 5 fuel/tick drain, and no-fire.
4. **Brace** — verify 2× shield regen and its truncation order against
   the hit-tick divisor; re-confirm 0.25× damage taken.
5. **regen_from_zero** — currently ASSUMED false; measure directly.
6. **Flee** — confirm base 3 consecutive ticks at Outer; resolve the
   doc-vs-bot contradiction (hull speed vs Tactics skill) with
   discriminating duels; verify counter reset on stance change.
7. **Low-armor law** — exact damage law for armor 0–30 (flat
   floor(a×0.75) vs saturating a/(a+150); today's crossover 12 is a fit,
   not a law).
8. Free riders from every duel: ring-delta arithmetic, `pulled_closer`
   label sync, odd zone_distances, `weapon_skill_pct` = 0 confirmation
   for zero-skill pilots.

Out of scope: capital-hull law (Bug Bot question — hypothesis: capital
pct = Gunnery's capitalWeaponDamage only), tackle modules
(webifier/disruptor; optional stretch S9 only if the campaign finishes
under budget), wildlife (phase C), any combat-sim v2 zones work (this
phase produces its inputs, not its code).

## Agents, arena, economics

- **Duelists**: `battle_bot1` and `battle_bot2` — fresh twin accounts
  (250 credits, stock Prospect: 95 hull / 50 shield / 1 weapon /
  2 utility slots, speed 1, Mining Laser I). Zero skills = clean
  baselines: crit 0, weapon_skill_pct 0, shields 0, armor 0, tactics 0.
  Step 0 of the campaign verifies this with `get_skills` on both.
- **Guest**: `craftsman-1` (Artis, Tactics 2, known full skill sheet)
  appears for exactly the flee-Tactics duels (S6c). Logging him in
  kills any live worker session — check `ps aux | grep bin/worker`
  first, 36s between logins.
- **Arena**: a lawless system (police 0, non-stronghold) one jump from
  the staging station. Staging = the station in `cargo_lanes` or
  `treasure_cache` (owner pre-positions the bots); bots set their home
  base there so destruction respawns them at staging **with a free
  replacement starter ship** — attrition is fitted modules only.
- **Funding**: ferried from a main-account agent (owner's choice) to
  the bots at staging. Budget ceiling ~100k credits; expected spend is
  a fraction (module list below).
- **Shopping list** (catalog-verified, no skill requirements):
  `pulse_laser_i` ×4 spares (2,500 ea; reach 3, 10 energy, cd 1),
  `autocannon_i` ×2 (710 ea; reach 2, kinetic, needs kinetic ammo),
  `missile_launcher_i` ×2 + explosive ammo stacks (1,800 ea; reach 6,
  cd 3, 4-round magazine — the only way to sample far-ring hit
  chances, since reach-gating suppresses the attack record entirely),
  `armor_plate_i` ×4+ (490 ea; +5 armor, defense slot). Plus, chosen at
  plan time from the catalog: one cheap hull with ≥3 defense slots for
  the armor ladder (the Prospect has none), and one cheap fast hull
  (max base_speed at tier ≤2) for the speed tests.

## Architecture

Three pieces; scenarios are data, not code.

### duel-runner (spacemolt repo: cmd/tools/duel-runner)

Sibling of battle-export, same client plumbing. One process holds two
persistent logged-in `game.Client` sessions (different agents — no
session contention). It executes a **campaign file** and appends to a
**manifest**; both live in the KB repo and are passed by path.

Per duel it runs a fixed lifecycle:

1. **Preflight**: both bots dock at staging if needed, refit to the
   scenario's fit (buy anything missing), `get_ship` verifies the fit
   matches the scenario spec exactly (abort the duel on mismatch),
   undock, jump to the arena, confirm system police 0 and no hostile
   battle in progress.
2. **Start**: attacker issues `attack(target=<other bot>)` once.
3. **Control loop**: driven by `battle_update` pushes (not polling).
   Each tick, apply the scenario's stance script — a list of
   `{from_tick, stance_a, stance_b, move_a, move_b}` phases (moves are
   advance/retreat/hold; battle actions are free and queue for the
   next tick). Ring holds work by issuing advance/retreat to pin the
   shared ring at the scenario's target and correcting drift.
4. **Terminal**: `battle_ended` (kill, escape, 30-tick stalemate) or
   the scenario's own `max_ticks` (then the loser flees out
   deliberately). Record `{scenario_id, repeat, battle_id, started,
   ended, outcome, void:false}` to the manifest (JSONL, append-only).
5. **Recover**: destroyed bot respawns at staging with a free starter
   hull; survivor jumps back; both refit for the next duel.

Interference rule: any third participant joining the battle (lawless =
pirate country) marks the duel `void:true` in the manifest, both bots
disengage (flee), and the scenario repeat is re-queued. Resume rule:
on start, the runner reads the manifest and skips completed
(scenario_id, repeat) pairs — a killed session loses nothing.

### Campaign file (KB repo: data/battles/duels/campaign.json)

```json
{
  "arena_system": "<lawless id>",
  "staging_station": "<poi id>",
  "duels": [
    {
      "id": "S1-ring2-both-hold",
      "purpose": "hit table @ zone_distance 4",
      "attacker": "battle_bot1",
      "fit_a": {"hull": "prospect", "modules": ["missile_launcher_i"]},
      "fit_b": {"hull": "prospect", "modules": ["pulse_laser_i"]},
      "script": [{"from_tick": 1, "stance_a": "fire", "stance_b": "fire",
                  "hold_ring": 2}],
      "max_ticks": 25,
      "repeats": 2
    }
  ]
}
```

### Analysis (KB repo)

`bin/battle-export --out-dir data/battles/duels` with the manifest's
battle ids (batch mode, one craftsman-boss login) pulls every log.
New scripts `data/battles/analysis/phaseb_hit_table.py`,
`phaseb_stances.py`, `phaseb_flee.py`, `phaseb_armor.py` each read the
duel fixtures and print the fitted value next to every raw
observation, so a reviewer can check the fit against the data. Their
outputs drive the `calibration.json` update by hand (values are few;
no codegen).

## Scenario matrix

Repeats are per-duel battle counts; volleys accumulate within a duel,
so most measurements need few duels. All duels are 1v1 bot-vs-bot
unless marked.

- **S0 probe** (1 duel, ~5 ticks, then mutual flee): verifies attack
  between own accounts in lawless space draws no police/rep response,
  free battle actions work as scripted, and the manifest/export loop
  round-trips. Nothing else proceeds until S0 is reviewed.
- **S1 hit table** (~6 duels): both fire. Both-hold at ring 3/2/1/0 →
  zone_distance 6/4/2/0; one-side-hold transitions sample odd 5/3/1.
  At rings beyond reach a weapon simply produces no attack record, so
  BOTH bots fit missile_launcher_i (reach 6) for the far-ring duels
  (rings 2–3) and swap to pulse_laser_i for rings 0–1. ≥20 attack
  records per distance per direction. Read `hit_chance` directly (it is printed,
  not inferred). Zero-skill pilots + identical speeds isolate the base
  table.
- **S2 speed modifier** (~4 duels): repeat S1 at rings 0 and 2 with
  bot2 in the fast hull (speed delta ≥3), both directions. hit_chance
  delta vs S1 at the same ring = the speed term. If the function looks
  nonlinear, add one intermediate-speed hull duel.
- **S3 evade** (2 duels, ~40 ticks): ring 0, A fire vs B evade.
  Reads: A's logged hit_chance (expect S1 ring-0 value − 0.20), B's
  landed-damage ratio vs S1 (expect 0.5×), B's fuel column (expect
  −5/tick), B fires nothing, and B's `weapon_skill_pct`/crit stay 0.
- **S4 brace** (2 duels): A single autocannon_i (8 kinetic — small,
  steady drain) vs B brace. Reads: B's shield delta and `regen[]`
  entries per tick → brace regen multiplier and its truncation order
  against floor(recharge/3) on hit ticks; landed damage ratio → 0.25×.
- **S5 regen_from_zero** (1–2 duels): A fires until B's shield reads
  exactly 0 — breakthrough empties the pool to 0 whatever the weapon —
  then switches to brace (stops firing). Watch B's shield for ≥10
  ticks: regen restart or not. Also confirms full recharge on un-hit
  ticks.
- **S6 flee series** (5–7 duels):
  (a) baseline: B flees from tick 1, A fires. `flee[]` events give
  counter/required for a zero-Tactics, speed-1 pilot — expect 3.
  (b) reset: B flees 2 ticks, stances to fire for 1, flees again —
  counter must restart from 0 (expect flee events to show it).
  (c) guest: craftsman-1 (Tactics 2, same-speed hull) flees from A —
  required 2 confirms the Tactics theory.
  (d) speed: B in the fast hull flees speed-1 A; then A (speed 1)
  flees the fast hull. Different required values confirm the doc's
  speed theory. (c) and (d) together resolve the contradiction; both
  may be true (two modifiers).
  (e) rings: confirm the counter only accrues at Outer and that flee
  auto-retreats (`flee_retreat` zone_moves), by fleeing from ring 0.
- **S7 armor ladder** (~5 duels): B in the defense-slot hull with
  armor_plate_i steps — armor totals 0, 5, 10, 15, 20 (+hull base).
  A fires an energy weapon (×0.75 armor counting) sized so per-volley
  hull damage stays well clear of the min-1 floor across the whole
  ladder — a 10-damage laser bottoms out against armor 15+, so the
  plan picks a ~30–65 damage option with no or trainable skill
  requirements. A fires until shields break, then ≥20 hull-landing
  volleys per step. Fit per-volley hull
  damage against both candidate laws; find the exact law and
  crossover. One kinetic-weapon repeat at one step cross-checks the
  ×1.5 counting.
- **S9 (stretch, only under budget)**: one stasis webifier duel — its
  escape-slowdown % vs S6a.

Wall-clock: ~25–30 duels × ~10s/tick ≈ 2 hours plus logistics,
resumable at any point via the manifest.

## Deliverables

1. `cmd/tools/duel-runner` (spacemolt repo) with tests for the
   scenario parser and script scheduler (no-network unit tests; the
   client layer is exercised by S0).
2. `data/battles/duels/`: campaign.json, manifest.jsonl, and all
   exported fixtures (committed; short 1v1s are small).
3. `data/battles/analysis/phaseb_*.py` — one per measurement family.
4. `data/combat-sim/calibration.json` updated: measured values move
   out of the assumed list with provenance in `_comment`; a new
   `hit_chance_by_distance` array (0–6) lands as measured data for the
   v2 zones engine (v1 keeps flat hit_chance_a/b, now set from the
   ring-0 measurement).
5. Golden tests in cmd/combat-sim replaying a flee duel and a brace
   duel fixture tick-for-tick (same style as the existing 14).
6. Memory + this spec's errata updated with every measured value;
   README's Measured-vs-ASSUMED section rewritten.

## Risks

- **Pirate interference** in the lawless arena — mitigated by the void
  rule, short duels, and picking the quietest adjacent lawless system
  (fewest wildlife/pirate battles in the bulk feed for that system).
- **Friendly-fire consequences unknown** — S0 exists to find out
  before anything is at stake; if attacking an own account draws rep
  damage or faction flags, stop and reassess with the owner.
- **Prospect has no defense slots** — the armor ladder needs the
  bought hull from day one; if defense-slot hulls under ~10k don't
  exist, S7 shrinks to the slots the cheapest option carries.
- **Starter-ship respawn details** — if respawn relocates bots or the
  free replacement differs from expectations, the recover step
  re-ferries them; the manifest keeps the campaign consistent.
- **Battle-action queue semantics** — actions apply at the next tick;
  the script scheduler must issue orders one tick ahead. S0 validates
  the offset empirically before any measurement relies on it.

## Success criteria

- calibration.json's assumed list is empty or every remaining entry
  names a deferral reason.
- The flee contradiction has a measured answer.
- The low-armor law is exact on every S7 volley.
- Two new golden duel-replay tests pass.
- A re-run of the full combat-sim suite is green and the stance table
  in the README reflects any changed numbers.
