"""Checks for the reprojection silhouette gate.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_gate.py
"""

import importlib
import json
import pathlib
import sys
import warnings

import cv2
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
    """`reproject` must return a (H,W) {0,1} mask that actually has hits in it.

    `set(np.unique(m)).issubset({0, 1})` is an accidental-superset check: a
    stub `reproject` that returns `np.zeros(shape, np.uint8)` satisfies both
    the shape assertion and this one (`{0}.issubset({0, 1})` is True), so the
    original two asserts pass on a mask with nothing projected into it at
    all. Confirmed red against exactly that stub before adding `m.sum() > 0`
    (see task-6-report.md); green again once the stub is removed.
    """
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    m = gate.reproject(_project_scene(s), s.K, s.image.shape[:2])
    assert m.shape == s.image.shape[:2]
    assert set(np.unique(m)).issubset({0, 1})
    assert m.sum() > 0


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

    The union==0 guard is exercised here because this fixture's matte is ALSO
    empty (a 50x50 zeros array, not a real silhouette): with a real non-empty
    matte, a behind-camera cloud gives `union = |truth| > 0` and the
    *unguarded* expression `(pred & truth).sum() / union` already returns a
    correct 0.0 with no special-casing needed — it is specifically the
    empty-matte case that makes the guard load-bearing, not the behind-camera
    cloud by itself.
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

    A stub that writes the new keys fresh each time (ignoring anything
    already on disk) would satisfy an assertion that only checked those keys.
    quality.json is a shared artifact -- stage 4 or a future stage's
    confidence numbers land there too -- so this seeds a pre-existing key and
    confirms it survives the write.

    NOTE: `run`'s signature and return type changed from the original
    3-argument, float-returning version (`run(ship_id, points, intrinsics,
    mask) -> float`, recording `silhouette_iou`/`silhouette_pass`) to
    `run(ship_id, points, mirrored, intrinsics, mask) -> dict`, recording
    `silhouette_iou`/`mirrored_fraction`/`mirrored_pass`. This is an
    intentional interface change (the metric the gate verdicts on moved from
    union IoU to the mirrored half's own inside-fraction -- see `IOU_FLOOR`'s
    comment in gate.py), not a design conflict, so this test is updated to
    the new contract rather than left failing against the old one; the
    property under test -- merge, don't clobber -- is unchanged.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    art_dir = gate.paths.artifact_dir("test_ship")
    (art_dir / "quality.json").write_text(json.dumps({"pre_existing_key": 42}))

    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cam = _project_scene(s)
    result = gate.run("test_ship", cam, cam, s.K, mask)

    data = json.loads((art_dir / "quality.json").read_text())
    assert data["pre_existing_key"] == 42
    assert data["silhouette_iou"] == result["silhouette_iou"]
    assert data["mirrored_fraction"] == result["mirrored_fraction"]
    assert data["mirrored_pass"] == result["mirrored_pass"]


def test_run_returns_a_dict_not_a_bare_float(tmp_path, monkeypatch):
    """The caller must not have to re-derive the verdict from a constant.

    Returning a bare `iou` float (the original design) forced any caller that
    wanted the verdict to recompute `iou >= gate.IOU_FLOOR` itself -- flagged
    in review as a real gap. `run` now returns the dict it also persists, so
    `mirrored_pass` is read directly, not re-derived.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cam = _project_scene(s)
    result = gate.run("test_ship_dict", cam, cam, s.K, mask)
    assert isinstance(result, dict)
    assert set(result) == {"silhouette_iou", "mirrored_fraction", "mirrored_pass"}


def test_run_flags_a_garbage_mirrored_half_that_union_iou_would_pass(tmp_path, monkeypatch):
    """The regression test for the whole metric change.

    The visible half alone already covers nearly the entire silhouette (see
    `IOU_FLOOR`'s calibration comment), so a UNION-IoU verdict over
    `points + mirrored` is dominated by the visible half and barely moves
    even when the mirrored half is badly wrong -- UNLESS the mirrored half is
    dense enough to add a lot of its own spurious area, which a real
    mirrored half (roughly matching the visible half's own density) is not
    guaranteed to be. `mirrored_fraction`, computed from `mirrored` alone, is
    not fooled by this because it does not depend on how much area the
    mirrored half's own points cover.

    Measured on this fixture (60k-point visible cloud, a 2000-point mirrored
    candidate displaced sideways by the full box length): combined
    `silhouette_iou` is 0.9451 -- comfortably above the diagnostic
    `IOU_FLOOR` of 0.70, i.e. the OLD verdict would have called this ship
    solved -- while `mirrored_fraction` is 0.2325, well below `MIR_FLOOR`
    (0.35), correctly failing it.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    visible = _project_scene(s)

    garbage = visible.copy()
    garbage[:, 0] += 3.0  # same displacement as test_displaced_cloud_scores_low
    rng = np.random.default_rng(2)
    garbage = garbage[rng.choice(len(garbage), 2000, replace=False)]

    result = gate.run("ship_bad_mirror", visible, garbage, s.K, mask)
    assert result["silhouette_iou"] > gate.IOU_FLOOR, result
    assert result["mirrored_fraction"] < gate.MIR_FLOOR, result
    assert result["mirrored_pass"] is False


def test_run_passes_a_mirrored_half_consistent_with_the_matte(tmp_path, monkeypatch):
    """Paired with the test above: a good mirrored half must pass, not just
    fail badly ones -- otherwise `mirrored_pass is False` would be satisfied
    by a verdict that always fails.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    visible = _project_scene(s)

    result = gate.run("ship_good_mirror", visible, visible, s.K, mask)
    assert result["mirrored_fraction"] > gate.MIR_FLOOR, result
    assert result["mirrored_pass"] is True


def test_project_returns_arrays_aligned_with_the_input_points():
    """`project`'s (uv, ok) must be length-N and index-aligned with `points`,
    so `reproject` and `inside_fraction` (and task 6b's depth-separation
    term, which needs to pair a mirrored point's uv with its own z) can build
    on it directly.
    """
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    cam = _project_scene(s)
    uv, ok = gate.project(cam, s.K, s.image.shape[:2])
    assert uv.shape == (len(cam), 2)
    assert ok.shape == (len(cam),)
    assert ok.sum() > 0


def test_reproject_is_unchanged_behaviour_built_on_project():
    """Task 6 extracted `project` out of `reproject` with, per the brief, ZERO
    behaviour change. Confirm `reproject`'s output is exactly the closed mask
    of the hits `project` reports -- not merely that the pre-existing tests
    still pass (they do, unchanged), but that the two functions agree
    mechanically.
    """
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    cam = _project_scene(s)
    h, w = s.image.shape[:2]
    uv, ok = gate.project(cam, s.K, (h, w))
    manual = np.zeros((h, w), np.uint8)
    manual[uv[ok, 1], uv[ok, 0]] = 1
    k = np.ones((gate._CLOSE, gate._CLOSE), np.uint8)
    expected = cv2.morphologyEx(manual, cv2.MORPH_CLOSE, k)
    assert np.array_equal(expected, gate.reproject(cam, s.K, (h, w)))


def test_project_filters_non_finite_points_without_a_runtime_warning():
    """MoGe writes +inf at invalid pixels (see test_pointmap.py's
    test_cloud_is_finite_and_in_front_of_the_camera). `points[:, 2] > 1e-6`
    ALONE admits +inf (`inf > 1e-6` is True) and does not catch a non-finite
    x or y either, producing nan/inf in `uv`; `np.round(nan).astype(int)` is
    documented-undefined and is only safe on this platform because the cast
    saturates to INT_MIN, which the bounds check then happens to reject.
    Confirmed directly: reproducing the pre-fix filter on this exact input
    raises `RuntimeWarning: invalid value encountered in matmul` and `in
    cast`, and produces uv values of -9223372036854775808 (INT_MIN) rather
    than anything a future platform is guaranteed to also reject. `project`
    must filter with `np.isfinite(points).all(axis=1)` first so this never
    happens, regardless of platform.
    """
    points = np.array([
        [0.0, 0.0, 1.0],      # ordinary front point
        [np.inf, np.inf, np.inf],  # MoGe's invalid-pixel sentinel
        [np.nan, 0.0, 1.0],   # non-finite in x, finite (positive) z
    ])
    K = np.array([[500.0, 0, 300], [0, 500.0, 300], [0, 0, 1]])  # pixel-space
    with warnings.catch_warnings():
        warnings.simplefilter("error", RuntimeWarning)
        uv, ok = gate.project(points, K, (600, 600))
    assert ok[0]
    assert not ok[1]
    assert not ok[2]


def test_inside_fraction_is_density_invariant_where_score_is_not():
    """The reason `inside_fraction` exists: `score`'s morphological closing
    needs a minimum point density to fill a region, so it is not comparable
    across differently-sized candidate clouds (task 6b evaluates hundreds of
    candidates and cannot afford full density each time). `inside_fraction`
    has no closing, so it does not have this problem.

    Measured on this fixture at target 2400 / 4000 / 20000 / 60000
    barycentric samples of the SAME perfect cloud: `score` reads
    0.084 / 0.20 / 0.957 / 0.991 (collapses at low density), while
    `inside_fraction` reads ~0.998 at every density tried.
    """
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)

    fractions, scores = [], []
    for target in (2400, 4000, 20000, 60000):
        cloud = _dense_surface_cloud(s, target=target) @ s.R.T + s.t
        scores.append(gate.score(cloud, s.K, mask))
        fractions.append(gate.inside_fraction(cloud, s.K, mask))

    assert max(fractions) - min(fractions) < 0.02, fractions
    assert scores[0] < 0.5 < scores[-1], scores  # score collapses at low density


def test_inside_fraction_denominator_is_points_in_front_not_all_points():
    """Points behind the camera must not dilute the denominator: a mirrored
    half is entirely in front by construction once reflected, but a
    degenerate candidate could still put points behind the camera, and those
    should be excluded from the denominator the same way `project`/`reproject`
    already exclude them from projection.
    """
    behind = np.zeros((5, 3))
    behind[:, 2] = -1.0
    mask = np.ones((50, 50), np.uint8)
    assert gate.inside_fraction(behind, np.eye(3), mask) == 0.0

    mixed = np.vstack([behind, np.array([[0.0, 0.0, 1.0]])])
    K = np.array([[500.0, 0, 25], [0, 500.0, 25], [0, 0, 1]])
    frac = gate.inside_fraction(mixed, K, mask)
    assert frac == 1.0, frac  # the single front point lands inside a full mask
