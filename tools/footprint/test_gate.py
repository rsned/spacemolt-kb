"""Checks for the reprojection silhouette gate.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_gate.py
"""

import importlib
import json
import pathlib
import sys

import numpy as np

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
matte = _load("matte")
gate = _load("gate")


def _dense_surface_cloud(scene, target=60_000, seed=0):
    """Barycentric samples over the scene mesh, at realistic cloud density.

    `Scene.points` is far too sparse to stand in for a MoGe cloud: 2400 samples
    across a mask covering 75192 px is 0.032 points per pixel, where MoGe
    returns roughly ONE point per valid pixel. Reprojecting the sparse set and
    closing it with a 5x5 kernel cannot produce a filled region, so a PERFECT
    cloud scores IoU 0.127 — below IOU_FLOOR. Measured: 0.053 / 0.127 / 0.319 /
    0.499 at dilation 3 / 5 / 9 / 15, i.e. no dilation rescues it.

    That would have made `test_perfect_cloud_scores_near_one` fail on correct
    code, and worse, `test_displaced_cloud_scores_low` would have passed for the
    wrong reason — everything scores low when nothing fills.

    Sampling the mesh at realistic density fixes it at the fixture, not by
    loosening the assertion. Measured perfect / displaced IoU: 0.957 / 0.122 at
    20k samples, 0.991 / 0.123 at 60k, 0.993 / 0.123 at 250k. 60k is the knee.
    """
    verts = scene.vertices
    tris = []
    for face in scene.faces:
        idx = np.asarray(face)
        for i in range(1, len(idx) - 1):          # fan-triangulate each polygon
            tris.append([idx[0], idx[i], idx[i + 1]])
    tris = np.array(tris)

    a, b, c = verts[tris[:, 0]], verts[tris[:, 1]], verts[tris[:, 2]]
    area = 0.5 * np.linalg.norm(np.cross(b - a, c - a), axis=1)

    rng = np.random.default_rng(seed)
    pick = rng.choice(len(tris), size=target, p=area / area.sum())
    u, v = rng.random((target, 1)), rng.random((target, 1))
    fold = (u + v) > 1.0                          # reflect into the triangle
    u[fold], v[fold] = 1.0 - u[fold], 1.0 - v[fold]
    return a[pick] + u * (b[pick] - a[pick]) + v * (c[pick] - a[pick])


def _project_scene(s):
    """A realistically dense perfect cloud, in camera coordinates."""
    return _dense_surface_cloud(s) @ s.R.T + s.t


def test_perfect_cloud_scores_near_one():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    iou = gate.score(_project_scene(s), s.K, mask)
    assert iou > 0.90, iou


def test_displaced_cloud_scores_low():
    """A misaligned reconstruction must fail the gate.

    This assertion is only meaningful BECAUSE test_perfect_cloud_scores_near_one
    passes on the same fixture: without that pairing, "scores low" is satisfied
    by a gate that scores everything low, which is exactly what the original
    sparse fixture did (perfect cloud 0.127, displaced 0.024 — both under the
    floor, test green, gate useless). Measured here: 0.991 perfect vs 0.123
    displaced.
    """
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cam = _project_scene(s)
    cam[:, 0] += 3.0  # slide the whole reconstruction sideways
    iou = gate.score(cam, s.K, mask)
    assert iou < gate.IOU_FLOOR, iou


def test_reprojection_mask_matches_the_image_shape():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    m = gate.reproject(_project_scene(s), s.K, s.image.shape[:2])
    assert m.shape == s.image.shape[:2]
    assert set(np.unique(m)).issubset({0, 1})


def test_score_handles_a_cloud_that_is_entirely_behind_the_camera():
    """No point reprojects; score must return 0.0, not a NaN or a crash.

    `reproject` filters to `z > 1e-6` before projecting, so an empty
    front-facing set is a real code path (a badly failed pose or a degenerate
    reconstruction produces exactly this), not a hypothetical. `score()`
    guards the union==0 case explicitly; deleting that guard (`return
    float((pred & truth).sum() / union)` with no `if union else 0.0`) does NOT
    raise on this fixture — numpy divides int64 0/0 to a silent `nan` with a
    RuntimeWarning, which is worse than a crash, since `json.dumps` happily
    serialises `nan` into quality.json as the non-standard token `NaN`.
    Confirmed by mutation: removing the guard turns this assertion red (it
    compares equal to 0.0, `nan == 0.0` is False) with that RuntimeWarning
    printed; restoring the guard turns it green again.
    """
    behind = np.zeros((10, 3))
    behind[:, 2] = -1.0
    mask = np.zeros((50, 50), np.uint8)
    assert gate.score(behind, np.eye(3), mask) == 0.0


def test_normalised_intrinsics_denormalise_to_the_same_reprojection():
    """`reproject` must handle MoGe's unit-square-normalised intrinsics too.

    Every test above passes `s.K`, which is already pixel-space (K[0,2] =
    W/2 = 600 for this fixture), so `K[0, 2] <= 2.0` is False and the
    denormalisation branch never runs in any of them — none would notice if
    it were deleted. That branch is exactly what production exercises:
    `pointmap.Cloud.intrinsics` is MoGe's own unit-square-normalised matrix
    (fx/fy/cx/cy as fractions of width/height — see pointmap.py), not a
    pixel-space K like this fixture's. Confirmed by mutation: commenting out
    the `if K[0, 2] <= 2.0: ...` block leaves this test's normalised-K score
    at 0.0 (everything reprojects near pixel (0,0) since fx/fy/cx/cy are all
    < 1) while the pixel-space score is unaffected and every test above stays
    green throughout, since none of them ever sends a normalised matrix.
    """
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    h, w = mask.shape
    normalised = s.K.copy().astype(float)
    normalised[0, :] /= w
    normalised[1, :] /= h
    cam = _project_scene(s)
    pixel_space_iou = gate.score(cam, s.K, mask)
    normalised_iou = gate.score(cam, normalised, mask)
    assert abs(normalised_iou - pixel_space_iou) < 1e-9, (pixel_space_iou, normalised_iou)


def test_run_merges_into_an_existing_quality_json_instead_of_clobbering_it(tmp_path, monkeypatch):
    """`run` must ADD its keys to quality.json, not replace the whole file.

    A stub that writes `{"silhouette_iou": iou, "silhouette_pass": ...}` fresh
    each time (ignoring anything already on disk) would satisfy an assertion
    that only checked the two new keys. quality.json is a shared artifact --
    stage 4 or a future stage's confidence numbers land there too -- so this
    seeds a pre-existing key and confirms it survives the write.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    art_dir = gate.paths.artifact_dir("test_ship")
    (art_dir / "quality.json").write_text(json.dumps({"pre_existing_key": 42}))

    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    iou = gate.run("test_ship", _project_scene(s), s.K, mask)

    data = json.loads((art_dir / "quality.json").read_text())
    assert data["pre_existing_key"] == 42
    assert data["silhouette_iou"] == iou
    assert data["silhouette_pass"] == (iou >= gate.IOU_FLOOR)
