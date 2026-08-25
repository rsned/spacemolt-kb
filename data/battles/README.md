# Battle replay fixtures

Exported by `bin/battle-export` from the `spacemolt` repo, which holds the
credentials — battle reads require a logged-in session, so a static page
cannot fetch these for itself.

| File | Battle | Ticks | Participants | Why it is here |
|---|---|---|---|---|
| `a2619bbe….json` | Node Beta | 30 | 42 | Primary acceptance artifact. Two sides, 14 kills, a station with an empty `ship_class`, and two classes with no art (`anamnesis`, `silent_tide`) — every draw path in one frame. |
| `b131fd5a….json` | Kitalpha | 158 | 5 | Four sides at bearings 82/121/152/271°. Proves the radial layout generalises past two sides. |
| `2a76e1a1….json` | Haven | 4430 | 13 | A player (AetherWraith, Opus Magna) solo-destroys the Grand Exchange Station: 17,146 shots, 14 kills, 1.48M damage. Kept as the mechanics-analysis corpus — the volley count is what makes the damage multiplier and crit rate measurable. |

Re-export with:

    bin/battle-export --agent craftsman-boss --battle <id> --out data/battles/<id>.json

Any logged-in agent can read a battle log — you do not need to have been in
the battle — but a login collides with (and kills) an agent that is already
connected elsewhere, failing with `session_replaced` and, on a second
collision within 30s, the exporter's own contention guard. There is no fixed
list of "idle" agents that stays true: `explorer-7`, `databot`, and
`craftsman-boss` are commonly free, but on 2026-08-19 `explorer-7` had a live
`mission-learn` fleet worker and `databot` had an interactive `play_as`
session, and only `craftsman-boss` was actually free. Before picking an
agent, check the process table for a `bin/worker --agent <name>` or a
`play_as <name>` process; if one is running, pick a different agent. Leave
35 s between exports — the exporter aborts if two connections die within
30 s of each other.

## Measured: does x/y drift within a zone?

```
engaged   n=  190 mean=0.578 min=0.087 max=1.479 spread=1.392
inner     n=   98 mean=0.647 min=0.182 max=1.371 spread=1.188
mid       n=   85 mean=0.803 min=0.477 max=1.431 spread=0.955
outer     n=  467 mean=1.078 min=0.778 max=1.630 spread=0.852
```

Radius is not a function of zone: every zone has a wide spread (0.85–1.39,
comparable to the mean radius itself) and the min/max ranges of adjacent
zones overlap heavily (e.g. `engaged` reaches 1.479 while `outer` starts as
low as 0.778), so x/y drifts continuously within a zone rather than sitting
on a fixed ring — P1b should interpolate positions linearly, not ease
between discrete zone radii.

## Reading the shot records

Three things that will produce wrong numbers if assumed otherwise, measured on
the 2a76e1a1 battle:

- `shots[].damage` is the **volley total for the tick**, duplicated onto every
  shot record in that volley. Summing the column overcounts by the number of
  weapons that fired — 8x on an Opus Magna.
- `hit` is per-volley, not per-weapon: one roll, shared by every shot.
- `weapon_damage` is that shot's damage **including its critical-hit roll**
  (a Void Laser reads 65 or 97 = floor(65 x 1.5)). Crits are rolled before the
  hit check, so they appear on misses too.

`damage` is pre-mitigation; `shield_damage + hull_damage` is what landed.
Resolution is `floor(sum(weapon_damage) x multiplier)`.
