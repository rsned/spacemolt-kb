"""Tests for the fusion selector (tools/footprint/fuse.py).

Unit tests run on synthetic fixtures; the final integration test reads the
real data tree and asserts headline invariants only.
"""
import json
import pathlib
import sys

import numpy as np

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
from tools.footprint import fuse  # noqa: E402

LABELS = json.loads(
    (pathlib.Path(__file__).resolve().parents[2]
     / "data/footprints/eyeball_labels_2026-08-01.json").read_text())


def test_load_rosters_reads_the_session_rulings():
    r = fuse.load_rosters(LABELS)
    assert "apeiron" in r.wing_filled
    assert {"voidborn_singularity", "qualia", "vigil"} <= r.wing_crescent
    assert "frankenhauler" in r.prong_confirmed
    assert "no_refunds" in r.receding and "long_haul" in r.receding
    assert any(a == "aether" and b == "close_enough"
               for a, b, _, _ in r.family_pairs)
