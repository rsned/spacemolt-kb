# Footprint Fusion — Design

**Date:** 2026-08-07
**Status:** Approved by user (deliverable, coverage, scale policy, and approach
each confirmed in session; design sections approved).
**Predecessor:** 2026-07-29-hero-art-footprint-recovery-design.md (the
measurement half). This spec covers the consuming half: selecting, correcting,
and publishing one canonical footprint per catalog ship.

## Problem

The recovery effort produced multiple candidate geometries per ship that
disagree in known, well-understood ways:

- **Pipeline profile** (`data/footprints/report.json`, embedded `w[96]` +
  `concave[96]`): correct for clean solves; wrecked by degenerate cross-sections
  (round/square hulls) and stern-on receding poses; the w(t) envelope
  structurally cannot represent pod gaps, bow prongs, or fine detail
  (wingtip blasters).
- **Pipeline polygon** (`data/footprints/<ship>/footprint.json` alpha shape):
  the only candidate that can represent forks/pods/detail; small prongs may
  still be bridged by the alpha radius.
- **TripoSR mesh profile** (`data/mesh_bakeoff/out-full/<stem>/profile.json`):
  robust to pose, but systematically over-widens (×0.67 beam correction,
  aspect ÷0.67), rounds blocky sterns (fixed per-ship by
  `profile.snap_plateaus`), and **inverts aspect on flying wings** (its long
  axis is the span). Bow/stern orientation arbitrary.
- **User annotations** (`user_annotations_2026-08-01.json`): 172 best-of-row
  picks (per-panel semantics established 2026-08-06) and 264 bow directions.
- **Eyeball labels** (`eyeball_labels_2026-08-01.json`): ground-truth shape
  bestiary, aspect-floor ruling, receding-pose rosters, prong lists, wing
  family, family-consistency pairs.

Fusion turns these into one canonical, provenance-carrying artifact per ship.

## Decisions (user-confirmed)

1. **Deliverable:** a committed fused dataset (`data/footprints/fused/`);
   KB/glyph integration is a separate follow-up that consumes it.
2. **Coverage:** all 267 ships, tiered by confidence
   (`user_pick` > `rules` > `unresolved`); nothing silently missing.
3. **Scale policy:** shape and aspect are decoupled. The winner contributes
   the shape; the aspect comes from whichever source the taxonomy trusts for
   that ship. Both recorded in provenance (canonical case: close_enough —
   pipeline shape, mesh-informed scale).
4. **Selection mechanism:** deterministic rules distilled from the session
   rulings (approach A). No scored/learned selector. The 172 picks serve as a
   validation set for the rules, not training data.

## Architecture

New `tools/footprint/fuse.py` (library functions + runnable entry point, same
conventions as the existing pipeline stages; runs under `~/moge-venv`).
Purely downstream — never mutates its inputs; regeneration is idempotent.

Inputs:

- `data/footprints/report.json` (embedded profiles + quality + orientation)
- `data/footprints/<ship>/footprint.json` (polygons, where stage 6 ran)
- `data/mesh_bakeoff/out-full/<stem>/{profile,footprint}.json`
  (exact-stem-first id resolution, same as the contact sheet)
- `data/footprints/user_annotations_2026-08-01.json`
- `data/footprints/eyeball_labels_2026-08-01.json`

Roster lifting: ship lists that currently live in prose notes (wing family,
receding cohorts, prong lists, tough-shape examples) are lifted into an
explicit machine-readable `fusion_rosters` block inside the labels file, so
rules read data, not English. The prose notes stay as the human record.

Outputs:

- `data/footprints/fused/<ship>.json` — one canonical entry per ship
- `data/footprints/fused/index.json` — per-ship one-liners (rule, confidence,
  sources, aspect), counts by rule, and the validation reports

## Rule ladder

First match wins; the rule name is recorded in the entry.

1. **`user_pick`** — a best-pick exists. Panel → source mapping:
   `moge` → pipeline profile; `footprint` → pipeline polygon;
   `mesh`/`mesh067` → mesh profile ×0.67 beam;
   `mesh067sq` → mesh ×0.67 + `snap_plateaus`.
   Confidence `user_pick`. (Stale picks referencing withdrawn profiles —
   e.g. interstice — fall through to the ladder with the pick noted.)
2. **`wing_family`** — roster: apeiron, qualia, vigil, voidborn_singularity.
   Pipeline shape — polygon for the open crescents (voidborn_singularity,
   qualia, vigil), profile for the filled wing (apeiron) — and **pipeline
   aspect**; mesh aspect is span-inverted on wings and never used for scale.
3. **`prong_or_pod`** — confirmed prong roster (crucible, annihilator,
   encyclopedia, apeiron, frankenhauler) plus ships matching the bow-concave
   detector (≥4 concave stations in the front third AND ≥60% of all concave
   stations there — the 2026-08-06 criterion) or with ≥25% of all stations
   concave (pod-blob): pipeline **polygon** — detector routes require an ok
   solve status (concave flags from a wrecked solve are artifacts of the same
   wreck); the confirmed roster is ungated.
4. **`wrecked_solve`** — status `failed_dimensional_check` plus the
   receding-cohort casualties: mesh shape ×0.67, mesh-adjusted aspect
   (÷0.67). Squaring applied only if `mesh067sq` was picked for a same-family
   sibling; otherwise plain.
5. **`clean_pipeline`** — status ok, silhouette IoU ≥ 0.97, in no suspect
   roster: pipeline profile + polygon, pipeline aspect (marked lower-bound
   in provenance if the ship is in a receding cohort).
6. **`unresolved`** — everything left (canonically-asymmetric omniscience,
   the low-obliquity trio, tough voidborn organics): `winner: null`, all
   candidates listed, confidence `unresolved`.

Scale decoupling applies within rules 2–5: `aspect_source` may differ from
`shape_source` per the taxonomy.

## Entry schema (`fused/<ship>.json`, schema 1)

- `id`, `rule`, `confidence` (`user_pick` | `rules` | `unresolved`)
- `shape_source`, `aspect_source`
  (`pipeline_profile` | `pipeline_polygon` | `mesh` | `mesh_squared` | null)
- `aspect` (final), and the winning geometry:
  - profile winner: `w[96]` + `concave[96]`
  - polygon winner: `polygon` (rings, canonicalised nose-up), plus the
    envelope `w[96]` as a convenience marked `envelope_lossy: true`
- `orientation`: `bow_t0` carried from the pipeline where known; mesh
  winners derive it by correlating the mesh w(t) against the ship's oriented
  pipeline profile (both directions; adopt the better direction only if
  r ≥ 0.5 with ≥ 0.1 margin) — the user bow click cannot be projected into
  the mesh's frame, which has no image registration; else `unknown`
- `provenance`: candidate aspects from every source, pipeline quality
  metrics, roster memberships, the user pick (if any), and `rulings` —
  pointers to the label notes justifying the applied rule

## Validation (built into every fuse run, reported in index.json)

1. **Rules vs picks:** run the ladder minus rule 1 over the 172 picked ships;
   list every disagreement (ship, rule's answer, user's answer). Disagreements
   are rule bugs or interesting ships; neither blocks the write.
2. **Family consistency:** sibling pairs from the bestiary
   (prayer/start_praying, overdue/overhead, crowbar/crucible,
   aether/close_enough ~2:1 narrow-to-wide, no_refunds/manifest_destiny)
   checked against the eyeballed relationships; mismatches flagged.

The 2026-08-07 first run of this validation caught exactly this class of
error: the aether/close_enough family check failed because 7 wrecked ships
(including aether) had published wrecked-mesh-flavored polygons through the
ungated `prong_or_pod` detectors — fixed by gating the detectors on solve
status (user-approved fix).

## Error handling

- Mesh missing for a mesh-rule ship → fall through to the next applicable
  rule, note the fallback in provenance.
- All sources missing → `unresolved`.
- Fuser is read-only on inputs; safe to re-run any time.

## Testing (TDD, `tools/footprint/test_fuse.py`)

Synthetic-fixture unit tests: one per ladder rung including fall-through;
pick→source mapping; scale decoupling (close_enough-shaped fixture); polygon
winner carries `envelope_lossy`; orientation carry-through and mesh-winner
bow derivation. One integration test over the real data asserting headline
invariants: 267 entries; every picked ship resolves by rule 1 (or documented
stale-pick fall-through); no `unresolved` entry has a live user pick.

## Out of scope (follow-ups)

- KB/glyph integration (`pkg/shipglyph` / `generate-ship-glyphs` consuming
  `fused/`), including glyph grading against fused footprints.
- Bow-region alpha tightening to preserve small prongs.
- The ~135 non-catalog drop images.
