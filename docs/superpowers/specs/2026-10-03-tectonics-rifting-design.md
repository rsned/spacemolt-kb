# Tectonics — Rifting (plate birth) Design

Date: 2026-10-03
Status: approved in conversation, awaiting written review
Extends: `2026-10-02-tectonics-first-planet-gen-design.md` (checkpoint 1)

## Why

Checkpoint 1 only ever removes plates: subduction consumes thin plates and
`retire` folds the remnants, so a world that starts with 21 plates ends with
11–13 after 800 Myr. The Earth reference bundle (Merdith et al. 2021 via
Müller et al. 2022, see `docs/BIBLIOGRAPHY.md`) shows counts that rise and
fall between 9 and 46 as supercontinents assemble and break up. Rifting is
the missing mechanism: a large, long-intact plate splits along its thinnest
interior crust and the two halves drift apart.

## Decisions taken in brainstorming

| Question | Decision |
| --- | --- |
| Trigger | Size and age, supercontinent style: a plate is eligible when its share of the sphere exceeds a seeded threshold AND it has gone a seeded rest period without a split; eligible plates rift with a small per-Myr probability. |
| Motion after a rift | Both halves re-aim at once with the existing push rule; the rift line is thinned to ridge crust and stamped divergent, so it is each half's thinnest boundary and sends them apart. The child draws a fresh (skewed) speed; the parent keeps its speed. |
| Rift line | Thinnest path across the plate: Dijkstra over the plate's own pixels from a seeded boundary pixel to the boundary pixel farthest across the plate, step cost = crust thickness raised to `RiftThinPower`. |

## Knobs (added to `Params`, archetype-independent defaults)

| Knob | Default | Meaning |
| --- | --- | --- |
| `RiftMinShareMin`, `RiftMinShareMax` | 0.04, 0.08 | per-planet seeded share of the sphere a plate must exceed to be eligible |
| `RiftRestMyrMin`, `RiftRestMyrMax` | 50, 120 | per-planet seeded Myr a plate must go without a split (from birth, or the run's start) before it is eligible |
| `RiftChancePerMyr` | 0.03 | probability per Myr that an eligible plate rifts this step (per step: `1 − (1−c)^MyrPerStep`) |
| `RiftMinChildShare` | 0.015 | a split whose smaller half is below this share is cancelled |
| `RiftThinPower` | 1 | exponent on thickness in the path cost; higher hugs thin crust harder |
| `RiftMaxPlates` | 200 | no rifts once this many plate ids exist (the bundle stores ids in one byte) |

`Validate`: shares in (0, 0.9], `RiftMinShareMin ≤ RiftMinShareMax`, rest range
non-negative and ordered, chance in [0, 1], child share in (0, 0.5),
`RiftThinPower > 0`, `RiftMaxPlates` in 2..255.

Seeds: the per-planet share and rest draws use domain `rift.params`; the
per-step chance, start-pixel choice and child speed use the stream
`sim.rift` (one `*rand.Rand` created in `Run`, like `sim.step`).

## Algorithm

`Rift` runs once per step in `Run`, right after `Step` and before the
`Reaim`/keyframe checks:

1. **Eligibility.** For each live plate `k`: `Area ≥ share` and
   `(StepNo − LastRift[k]) × MyrPerStep ≥ rest` and `len(Plates) < RiftMaxPlates`.
   Draw `u`; rift if `u < 1 − (1 − RiftChancePerMyr)^MyrPerStep`. At most one
   plate rifts per step (the lowest eligible id that passes its draw), so a
   step never produces two interacting rifts.
2. **Endpoints.** Collect the plate's boundary pixels (a 4-neighbour with a
   different label), in index order. Start = the boundary pixel at a seeded
   index. End = the boundary pixel with the largest angular distance from the
   start (ties: lower index).
3. **Path.** Dijkstra from start to end over the plate's pixels only
   (4-neighbour moves, seam-aware), edge cost `th[j]^RiftThinPower + 1e-6`.
   The path is the rift line.
4. **Split.** Temporarily remove the path pixels, flood-fill the remaining
   plate pixels into connected components. The largest component keeps the
   parent's id; all other components together form the child (so a path that
   clips off two lobes still yields two plates). Path pixels join whichever
   side the existing gap rule picks (nearest labelled pixel by BFS).
   If the child's share is below `RiftMinChildShare`, cancel: nothing changes
   and the plate's `LastRift` is NOT updated (it may try again next step).
5. **Crust and features.** Path pixels: thickness = `RidgeThickness ± RidgeJitter`,
   age 0, `FeatDivergent`, feature age 0.
6. **Tables.** Append `Plate{ID: next, Major: false, Born: StepNo}` and a
   `Motion` for the child: pole from `pushDirection` (the rift is now its
   thinnest boundary) with the usual jitter, speed from `drawSpeed`. Parent:
   `LastRift = StepNo`, then re-aimed with `Reaim`'s rule (fresh push weight)
   so it also moves away from the rift. Child `LastRift = StepNo`.
   `PlateStats` is recomputed; `Major` flags are unchanged (the child is a
   minor until a later ranking, which checkpoint 1 does not do).

`Plate` gains `Born int` and `LastRift int` (both 0 at the start of a run);
`PlateRow` gains `born` (JSON `born`). Everything else in the bundle contract
is unchanged, so the viewer, the stats script and later layers keep working;
the stats script's lifetime and birth counts become meaningful.

## Viewer

No required change. Optional: the arrow label shows `id` with a `*` for
plates born after frame 0. The stats script gains a `births` line (ids whose
first frame is after frame 0) and keeps the lifetime line.

## Tests

- `TestRiftSplitsAlongThinTrough`: one-plate world at S=32 with a thin trough
  (thickness 0.1) along the x = 0 great circle and 0.8 elsewhere; make it
  eligible (share 1.0, rest 0, chance 1): after `Rift` there are exactly two
  live plates, the two halves lie on opposite signs of x (≥ 95% of each
  half's pixels), the path pixels carry `FeatDivergent` and ridge thickness,
  and the child's velocity at its centroid points away from the parent's
  centroid.
- `TestRiftEligibility`: a plate below the share, or inside its rest period,
  never rifts even with chance 1; `RiftMinChildShare` cancels a split whose
  trough hugs the boundary.
- `TestRiftKeepsTablesParallel`: after several rifts `len(Plates) == len(Motions)`,
  areas sum to 1, no label is out of range, determinism across two runs.
- Golden rebaked as `terran/2026/64/20/v6`; `TestTimingBudgetFace128` unchanged.
- Stats: `tectonics_stats.py` prints births; the tuning target is plate
  counts that oscillate in the 12–25 band over 800 Myr for terran with the
  largest plate near 25–35% most of the time.

## Out of scope

Continental vs oceanic rifting rules, rift failure (aulacogens), triple
junctions, and any change to subduction or retirement.
