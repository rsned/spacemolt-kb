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
