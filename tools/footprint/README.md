# Footprint recovery

Recovers a measured top-down hull footprint from a ship hero image.
Design: `docs/superpowers/specs/2026-07-29-hero-art-footprint-recovery-design.md`

## Setup

    python3 -m venv ~/moge-venv
    ~/moge-venv/bin/pip install torch --index-url https://download.pytorch.org/whl/cu121
    ~/moge-venv/bin/pip install -r tools/footprint/requirements.txt

torch comes from the CUDA index first; the rest resolve from PyPI. To
reproduce an exact environment instead, use `requirements.lock.txt`.

This venv is deliberately separate from `~/sd-venv`; do not merge them.

## Run

    ~/moge-venv/bin/python tools/footprint/run.py --all

Artifacts land in `data/footprints/<id>/`. Only `profile.json` is committed.
Point at a different art drop with `SMKB_HERO_DIR=/path/to/art`.

## Tests

    ~/moge-venv/bin/python -m pytest tools/footprint/

## Background handling

Stage 3 (`pointmap.py`) can feed MoGe-2 either the raw chroma-keyed image
(flat saturated magenta background) or the matted subject composited onto a
neutral 128-gray field. Measured on the three keyable hero images named in
the stage 3 brief, comparing relative depth spread (`std(z) / mean(z)` over
the returned point cloud), from unrounded values:

| ship            | background | n points | mean z  | depth spread | raw vs neutral |
|-----------------|------------|---------:|--------:|--------------:|----------------|
| outerrim_prayer | raw        |  461,637 | 19.616  |       0.08090 | +2.8% |
| outerrim_prayer | neutral    |  466,501 | 17.988  |       0.07870 | |
| magnate         | raw        |  356,799 | 17.481  |       0.13661 | −0.5% (neutral ahead) |
| magnate         | neutral    |  356,100 | 16.552  |       0.13736 | |
| comet           | raw        |  115,621 | 13.309  |       0.13058 | +5.8% |
| comet           | neutral    |  115,380 | 14.345  |       0.12348 | |

Inference is bit-deterministic here (repeat runs agree to 5 decimal places),
so this spread is a real, small effect, not noise — but it does not settle
the question. The result is one raw win, one neutral win, one near-exact tie
— not a majority for either setting. Note the split cannot be waved away as
"raw fractionally ahead both times": it isn't.

More importantly, `std(z) / mean(z)` turns out not to be a valid
discriminator at all. Its own inputs shift ~10% with the background — MoGe's
recovered metric scale is itself free, and `mean z` moves by that much
between `raw` and `neutral` for the same subject above — so a 3-6% spread
difference can't be trusted to mean anything. Checked this against ground
truth on two synthetic scenes, where the true focal length (1400 px) is
known and MoGe's recovered focal is directly checkable:

| scene                        | background | depth spread | focal error |
|-------------------------------|------------|--------------:|------------:|
| box(4,2,1.5, az35,el30)       | raw        |        0.0349 |       +9.5% |
| box(4,2,1.5, az35,el30)       | neutral    |        0.0479 |      −13.4% |
| cylinder(1.2,3, az40,el25)    | raw        |        0.0380 |      +16.4% |
| cylinder(1.2,3, az40,el25)    | neutral    |        0.0409 |       +1.8% |

The two scenes disagree on which *direction* the relationship even runs. On
the box, the larger-spread setting (`neutral`, 0.0479) has the *worse* focal
error (−13.4% vs. +9.5%). On the cylinder, the larger-spread setting (also
`neutral`, 0.0409) has the *better* focal error (+1.8% vs. +16.4%) — the
opposite pairing. (An earlier draft of this section claimed the box's
pairing held "in both scenes"; a different cylinder fixture happened to
agree with the box by chance, and citing it as a second confirming data
point was wrong — this table replaces it with a cylinder fixture that
disagrees.) Since the sign of the depth-spread/accuracy relationship isn't
stable across fixtures, the metric carries no reliable signal about
reconstruction quality at all — not merely "anti-correlated," which would
itself imply a stable, exploitable (if inverted) relationship that isn't
there either.

**The experiment as specified could not separate `raw` from `neutral`, and
focal error doesn't settle it either**: `neutral` is markedly better on the
cylinder (+1.8% vs. +16.4%) and markedly worse on the box (−13.4% vs.
+9.5%). `neutral` is still the recorded default, but on the one argument
that doesn't depend on a measurement that disagrees with itself: `raw` makes
the reconstructed cloud depend on the backdrop staying a flat, uniform key —
which is exactly the condition stage 1's `keyability()` gate exists to
check, because it does not always hold (5 of the 19 catalog-matched hero
images are environmental scene renders with no flat key at all). `neutral`
behaves identically regardless of what the backdrop is, which is the
property a batch driver over a full, mixed art drop needs. `run.py` should
pass `background="neutral"` (Task 9).

Neither depth spread nor focal error is a usable discriminator from this
experiment; both are recorded above for anyone revisiting this decision, not
as evidence for either setting. Once Tasks 6-8 land, stage 5's reprojection
IoU and stage 7's footprint error against `synth` ground truth are the
checkable signals to use instead.

## Results

**Superseded once, by the final-review fix wave (task-9 final review, 2026-07-31)
— this section reflects the RE-RUN, not the original batch.** The first run
(batch alpha 13.0, 14/19 recovered) predates four correctness fixes to
`run.py` and is kept here only as history for what changed and why:

- **C1 — keyability was computed but never consulted.** `matte.keyability`
  existed and `matte.run` wrote `keyability.json`, but nothing in `run.py`
  ever called either; `process` went straight from `matte.extract` to
  camera/MoGe/mirror. Five of the 19 hero images are environmental scene
  renders (a ship in a hall, a cavern, a hangar) rather than a flat chroma
  key, and a colour-distance threshold still segments something plausible-
  looking off them — `matte.extract` was quietly matting *scenery*, and one
  of those five (`principia`) reconstructed well enough on every other
  signal to publish as `ok_asymmetric` in the first run. `process` now
  checks keyability first, before spending any MoGe inference, and reports
  `failed_unkeyable` with the measured `corner_spread`/`border_std`.
- **C2 — no obliquity floor.** `mirror.py` has always documented that below
  roughly obliquity 0.3 the depth-separation term cannot distinguish a fold
  from a genuine occluded half, but nothing in `run.py` read
  `sym.obliquity`. `mirror.OBLIQUITY_FLOOR = 0.3` is now a real gate
  (`failed_low_obliquity`), and `obliquity` is recorded in `quality` for
  every ship that reaches stage 4, not only the failures.
- **C3 — `sym.failure` was computed and never read.** `mirror.Symmetry`'s own
  docstring says a non-`None` `failure` means "do not trust this result,"
  but `process` never checked it, so a proximity-infeasible solve could
  still clear `gate.run`'s `mirrored_pass` (which has no proximity check of
  its own) and publish. Now checked immediately after `mirror.run`
  (`failed_symmetry_solve`).
- **C4 — the alpha sweep violated `ground.sweep_alpha`'s own contract.**
  `ground.py` documents "pass ALREADY-SUBSAMPLED clouds" because
  `alphashape`'s cost is superlinear; `_ship_ground_xy` was passing full
  ~800k-point completed clouds, and a single un-subsampled `hull()` call did
  not finish in 5 minutes. It also meant the alpha chosen on full-density
  clouds was being applied (via `ground.run`'s own `subsample`) to a 20k
  subsample — not the density it was validated on. `_ship_ground_xy` now
  subsamples before the sweep, the same way `ground.run` already does.

Batch run: `~/moge-venv/bin/python -m tools.footprint.run --all`, batch alpha
now **`3.0`** (down from 13.0 — expected per C4: a value that keeps 90% of an
800k-point cloud is not the value that keeps 90% of the 20k-point subsample
the sweep now correctly evaluates). Background `neutral`, unchanged (see
Background handling above).

**13 of 19 hero images recovered**, all as `ok_asymmetric` for the same
reason recorded before: `mirror.RESIDUAL_CEILING` (0.013) was calibrated on a
two-sided synthetic cloud, and every real one-view cloud's `mirror_residual`
in this batch (0.022-0.100) sits at or above it — see `mirror.py`'s own
documented measurement that partial one-view coverage alone accounts for
this, no actual hull asymmetry required. Every recovered ship's `obliquity`
falls in 0.557-0.851, comfortably inside `mirror.OBLIQUITY_FLOOR`'s 0.3 floor
and the ~0.9 upper edge past which `gate.MIR_FLOOR` stops separating —
C2's new gate did not exclude anyone in this batch, but exists for the
335-ship scale-up. Likewise C3: no recovered or excluded ship in this batch
carried a non-`None` `sym.failure`.

Recovered: comet, excessive_force, ledger, liquidity_event, magnate,
premiere, rapid_smelter, reliquary, smelter, superposition, thermodynamic_end,
war_wagon, yard_sale.

**6 did not recover:**

| id | status | reason |
|----|--------|--------|
| bonanza | `failed_unkeyable` | corner_spread=17.11 / border_std=24.73, both above the floor of 10.0 — this is the ship that failed the silhouette gate in the first run; the unkeyable background was the actual root cause, the silhouette failure was a downstream symptom |
| crowbar | `failed_unkeyable` | corner_spread=5.26 (passes) / border_std=38.97 (fails) — the four corner patches agree, but the border varies too much: a scene render whose corners happen to be visually flat |
| last_warning | `failed_unkeyable` | corner_spread=7.40 (passes) / border_std=39.87 (fails) — same border-only failure mode as crowbar |
| paradox | `failed_unkeyable` | corner_spread=1.12 (passes) / border_std=47.32 (fails) — same border-only failure mode |
| principia | `failed_unkeyable` | corner_spread=50.88 / border_std=67.21, both well above the floor — the cavern scene render that published as `ok_asymmetric` in the first run; this is the case C1 exists to catch |
| prayer | `failed_dimensional_check` | aspect 0.56 below `profile.ASPECT_BOUNDS`'s floor of 1.2 — reads as too stubby to be a hull |

**The I6 correction:** the first run reported four `failed_dimensional_check`
ships and framed all four as an open flattening-vs-stubby question. Three of
those four (`crowbar`, `last_warning`, `paradox`) are now caught upstream as
`failed_unkeyable` — they never had a trustworthy reconstruction to be
"stubby" or "flattened" in the first place, since the mask that fed every
later stage was segmenting scenery, not hull. `prayer` is the one ship that
is genuinely keyable (`corner_spread`/`border_std` both comfortably under the
floor, `foreground_fraction` 0.44) and still fails the dimensional check —
scoring well on every other signal (silhouette IoU 0.993, mirrored_fraction
0.998, obliquity 0.857) exactly as before. Whether `prayer`'s low aspect
(0.56) is a genuine reconstruction flattening or a hull that really is closer
to square than `ASPECT_BOUNDS`'s 1.2 floor assumes is still not settled by
this run — that determination is for the consuming half of the plan, which
has ground-truth glyphs to check against. `prayer` (outerrim_prayer) remains
notable as the ship named throughout `mirror.py`'s own calibration notes as
well-behaved on every other axis, so it is worth a first look there — but the
question is now narrowed to one ship, not four.

`camera_confidence` clears `camera.CONFIDENCE_FLOOR` (0.05) for only 1 of the
14 ships that reach stage 2 in this run (`ledger`, 0.057; `bonanza`, the
other ship that used to clear it, is now excluded before camera confidence
is even relevant) — consistent with the plan's own finding that only a small
minority of keyable hero images produce a confident vanishing-point fit.
`ground.up_vector`'s Task 9 revision means it did not matter either way:
stage 2 is a cross-check logged for agreement, not a gate, so no ship in this
batch was excluded or overridden on camera confidence.
