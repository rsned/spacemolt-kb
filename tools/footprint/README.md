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
truth on the two synthetic scenes, where the true focal length (1400 px) is
known and MoGe's recovered focal is directly checkable:

| scene    | background | depth spread | focal error |
|----------|------------|--------------:|------------:|
| box      | raw        |        0.0349 |       +9.5% |
| box      | neutral    |        0.0479 |      −13.4% |
| cylinder | raw        |        0.0492 |       +4.4% |
| cylinder | neutral    |        0.0480 |       −1.0% |

In both scenes, the setting with the *larger* depth spread has the *worse*
focal accuracy (box: neutral's larger spread pairs with the larger error;
cylinder: raw's larger spread pairs with the larger error) — the opposite of
what "larger spread = less flat = better" would predict. The metric is
anti-correlated with accuracy where accuracy is actually checkable, so it
cannot be used to pick a winner on the real hero images either, where there
is no ground truth to fall back on.

**The experiment as specified could not separate `raw` from `neutral`.**
`neutral` is still the recorded default, but on a different, sounder ground:
`raw` makes the reconstructed cloud depend on the backdrop staying a flat,
uniform key — which is exactly the condition stage 1's `keyability()` gate
exists to check, because it does not always hold (5 of the 19 catalog-matched
hero images are environmental scene renders with no flat key at all).
`neutral` behaves identically regardless of what the backdrop is, which is
the property a batch driver over a full, mixed art drop needs. `run.py`
should pass `background="neutral"` (Task 9).

The truth-referenced discriminator that actually works — per-background
focal error against `synth`'s known camera — is recorded above for anyone
revisiting this decision. Once Tasks 6-8 land, stage 5's reprojection IoU and
stage 7's footprint error against `synth` ground truth are the checkable
signals to use instead of a proxy like depth spread.
