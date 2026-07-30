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
the returned point cloud — a collapsed spread means MoGe read the image as
flat):

| ship            | background | n points | depth spread | raw vs neutral |
|-----------------|------------|---------:|--------------:|----------------|
| outerrim_prayer | raw        |  461,637 |         0.081 | +2.5%  (tie, <5%) |
| outerrim_prayer | neutral    |  466,501 |         0.079 | |
| magnate         | raw        |  356,799 |         0.137 | +0.0%  (tie, <5%) |
| magnate         | neutral    |  356,100 |         0.137 | |
| comet           | raw        |  115,621 |         0.131 | +6.5%  (raw ahead) |
| comet           | neutral    |  115,380 |         0.123 | |

Two of the three ships are within 5% of each other (raw fractionally ahead
both times, but not meaningfully so); `comet` is the one case where raw
pulls ahead by more than 5%. Per the brief's tie-break rule (within 5%, keep
`neutral`), and since that's the majority outcome here, **`neutral` is the
default** (`run.py` should pass `background="neutral"` — set in Task 9).
Flat saturated magenta is out of distribution for a model trained on
photographs, and empirically it does not reliably do better even where it
is in-distribution enough to produce a sane point cloud at all — `comet` is
the only one of the three where it measurably helps, and not by much.
