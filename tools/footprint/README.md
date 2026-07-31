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

Batch run: `~/moge-venv/bin/python -m tools.footprint.run --all`, batch alpha
`13.0` (the tightest alpha in `ground.ALPHA_CANDIDATES` that still kept 90% of
every cloud's own points — `run._pick_alpha`/`ground.sweep_alpha`), background
`neutral` (the setting recorded above under Background handling).

**14 of 19 hero images recovered.** All 14 landed as `ok_asymmetric`, not
plain `ok`: `mirror.RESIDUAL_CEILING` (0.013) was calibrated on a two-sided
synthetic cloud, and every real one-view cloud's `mirror_residual` in this
batch falls in 0.022-0.100 — consistent with `mirror.py`'s own documented
measurement that partial one-view coverage alone puts real clouds at or above
that ceiling with no actual asymmetry required. `ok_asymmetric` here is an
artifact of the residual ceiling being tuned for a different regime, not
evidence these 14 hulls are lopsided.

Recovered: comet, excessive_force, ledger, liquidity_event, magnate,
premiere, principia, rapid_smelter, reliquary, smelter, superposition,
thermodynamic_end, war_wagon, yard_sale.

**5 did not recover:**

| id | status | reason |
|----|--------|--------|
| bonanza | `failed_silhouette_gate` | `mirrored_fraction=0.9448` below `gate.MIR_FLOOR=0.96` — the guessed symmetry plane's mirrored half does not land inside bonanza's own silhouette closely enough to trust |
| crowbar | `failed_dimensional_check` | aspect 0.98 below `profile.ASPECT_BOUNDS`'s floor of 1.2 — reads as too stubby to be a hull |
| last_warning | `failed_dimensional_check` | aspect 1.00 below 1.2 — same failure mode as crowbar |
| paradox | `failed_dimensional_check` | aspect 0.51 below 1.2 |
| prayer | `failed_dimensional_check` | aspect 0.59 below 1.2 |

All four dimensional failures score well on every other quality signal
(silhouette IoU 0.98-0.99, mirrored_fraction 0.98-1.00) — the aspect-bound
gate is catching something the silhouette and mirror checks are structurally
blind to (`uv = K.p / p_z` is invariant under any positive per-point depth
scale, so a flattened or foreshortened reconstruction reprojects identically
to a correct one; see `profile.py`'s comment on `ASPECT_BOUNDS`). Whether
these four are genuine reconstruction failures (an over-flattened one-view
cloud) or ships whose true footprint is legitimately closer to square than
`ASPECT_BOUNDS`'s 1.2 floor assumes is not settled by this run — that
determination is for the consuming half of the plan, which has ground-truth
glyphs to check against. `prayer` (outerrim_prayer) is notable here: it is
the ship named throughout `mirror.py`'s and this file's own calibration notes
as a well-behaved case on every other axis, so its dimensional failure is
worth a first look there.

`bonanza`'s failure is the only stage-5 exclusion in this batch; nothing
required the click fallback. `camera_confidence` clears
`camera.CONFIDENCE_FLOOR` (0.05) for only 2 of the 19 (`bonanza` 0.064,
`ledger` 0.057) — the other 17 sit at 0.03 or lower, matching the plan's own
finding that only a small minority of keyable hero images produce a confident
vanishing-point fit. `ground.up_vector`'s Task 9 revision means it did not
matter either way: stage 2 is a cross-check logged for agreement, not a gate,
so no ship in this batch was excluded or overridden on camera confidence —
`bonanza` failed on the silhouette gate, not the camera fit.
