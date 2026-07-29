#!/usr/bin/env python3
"""Artifact layout for the footprint recovery pipeline.

The single place that knows where per-ship intermediates live, so pointing the
batch at a different art drop is a path change here and nowhere else.

    from tools.footprint import paths
"""

import os
import pathlib

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
FOOTPRINT_ROOT = REPO_ROOT / "data" / "footprints"

# Hero art location. Override with SMKB_HERO_DIR when a full art drop arrives.
HERO_DIR = pathlib.Path(os.environ.get("SMKB_HERO_DIR", pathlib.Path.home() / "Downloads"))
HERO_GLOB = "*.webp"


def artifact_dir(ship_id: str) -> pathlib.Path:
    """Return the per-ship artifact directory, creating it if absent."""
    d = FOOTPRINT_ROOT / ship_id
    d.mkdir(parents=True, exist_ok=True)
    return d
