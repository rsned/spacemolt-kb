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


def _mini_tree(tmp_path):
    """Two-ship synthetic data tree: 'plain' (full candidates) and 'bare'
    (report record only)."""
    foot = tmp_path / "footprints"
    bakeoff = tmp_path / "bakeoff"
    w = [0.2] * 96
    report = {"alpha": 8.0, "background": "x", "results": [
        {"id": "plain", "status": "ok_asymmetric", "aspect": 2.5, "w": w,
         "concave": [False] * 96, "orientation": "bow_t0",
         "orientation_source": {"flipped_from_stored": False},
         "quality": {"silhouette_iou": 0.99}},
        {"id": "bare", "status": "failed_dimensional_check", "aspect": 0.5,
         "w": w, "concave": [False] * 96, "orientation": "unknown",
         "quality": {"silhouette_iou": 0.98}},
    ]}
    foot.mkdir()
    (foot / "report.json").write_text(json.dumps(report))
    (foot / "plain").mkdir()
    (foot / "plain" / "footprint.json").write_text(json.dumps({
        "alpha": 8.0,
        "polygon": {"type": "Polygon", "coordinates":
                    [[[0, -0.2], [1, -0.2], [1, 0.2], [0, 0.2], [0, -0.2]]]}}))
    # mesh stored under a faction prefix — exercises the prefix-stripped pass
    (bakeoff / "outerrim_plain").mkdir(parents=True)
    (bakeoff / "outerrim_plain" / "profile.json").write_text(json.dumps(
        {"w": [0.3] * 96, "aspect": 1.8}))
    return foot, bakeoff


def test_load_candidates_assembles_all_sources(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    cands = fuse.load_candidates(foot, bakeoff)
    c = cands["plain"]
    assert c.pipe["aspect"] == 2.5
    assert c.polygon["polygon"]["type"] == "Polygon"
    assert c.mesh_w is not None and len(c.mesh_w) == 96
    assert c.mesh_aspect == 1.8


def test_load_candidates_tolerates_missing_sources(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    c = fuse.load_candidates(foot, bakeoff)["bare"]
    assert c.pipe["status"] == "failed_dimensional_check"
    assert c.polygon is None and c.mesh_w is None and c.mesh_aspect is None


def test_mesh_resolution_prefers_the_exact_stem(tmp_path):
    foot, bakeoff = _mini_tree(tmp_path)
    # an exact-stem dir must beat the prefixed one, matching the contact sheet
    (bakeoff / "plain").mkdir()
    (bakeoff / "plain" / "profile.json").write_text(json.dumps(
        {"w": [0.5] * 96, "aspect": 9.9}))
    c = fuse.load_candidates(foot, bakeoff)["plain"]
    assert c.mesh_aspect == 9.9
