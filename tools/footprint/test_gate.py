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


def _reflect(points, normal, offset):
    """Mirror `points` across the plane {p : p . normal = offset}.

    Deliberately NOT imported from mirror.py: task 6's file scope is
    gate.py/test_gate.py only, and this is just the textbook reflection
    formula, needed here purely to build synthetic fold fixtures for
    `run()`'s conjunction logic -- it is not a test of mirror.py.
    """
    n = normal / np.linalg.norm(normal)
    return points - 2.0 * ((points @ n) - offset)[:, None] * n


def _depth_separation(visible_cam, candidate_cam, K, shape):
    """Mean of (candidate z - visible z) at pixels where both reproject.

    Stands in for what task 6b's `solve_from_view` will compute as
    `Symmetry.depth_separation` (see the task 6b brief): a genuine occluded
    half is BEHIND the visible surface (positive), a fold sits at the same
    depth (near zero). Test-only -- `depth_separation` is a value `run()`
    receives from its caller, not something gate.py computes itself.
    """
    h, w = shape
    vis_uv, vis_ok = gate.project(visible_cam, K, (h, w))
    cand_uv, cand_ok = gate.project(candidate_cam, K, (h, w))

    vis_z = np.full((h, w), np.nan)
    vis_z[vis_uv[vis_ok, 1], vis_uv[vis_ok, 0]] = visible_cam[vis_ok, 2]

    cand_pix_z = candidate_cam[cand_ok, 2]
    cand_pix_uv = cand_uv[cand_ok]
    shared_vis_z = vis_z[cand_pix_uv[:, 1], cand_pix_uv[:, 0]]
    valid = ~np.isnan(shared_vis_z)
    if valid.sum() == 0:
        return 0.0
    return float(np.mean(cand_pix_z[valid] - shared_vis_z[valid]))


def _one_sided_hull_and_fold(seed=0):
    """A back-face-culled ("visible-only") synthetic hull, its TRUE bilateral
    mirror, and a FOLD -- a wrong plane whose normal is only 15 degrees off
    the mean view direction, which is the family of wrong planes a
    self-consistency objective actually converges to (see the task 6b
    brief's rationale for why self-chamfer folds on one-sided input).

    EXACT construction, stated precisely enough to reproduce independently
    (this matters: an earlier version of this comment said "15 degrees off
    the mean view direction" without stating the rotation axis, and a
    second, independent measurement on a different hull at the same nominal
    angle got a meaningfully different depth_separation -- see
    MIN_DEPTH_SEPARATION_FRACTION's comment in gate.py for the full
    reconciliation):

      1. `s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)`.
      2. Back-face cull: keep only mesh triangles whose face normal points
         toward the camera (`face_normal . (eye - face_centroid) > 0`, all in
         WORLD coordinates) -- this is what makes the cloud one-sided, unlike
         `_dense_surface_cloud`, which samples the whole closed mesh.
      3. Barycentric-sample 30,000 points over the kept (front-facing)
         triangles only, area-weighted, seeded (`seed=0` by default) --
         `visible_world`, then `visible_cam = visible_world @ s.R.T + s.t`.
      4. TRUE bilateral plane: reflect `visible_world` across the box's own
         real world symmetry plane, normal `[1, 0, 0]`, offset `0` (the box
         is centred on the origin) -- this is the actual correct answer for
         this hull, not an approximation.
      5. FOLD: in CAMERA space (camera is at the camera-space origin, so a
         point's own direction from the origin IS its view direction),
         `centre = visible_cam.mean(axis=0)`, `view_dir = centre /
         |centre|`. Rotation axis: `perp = normalize([1,0,0] - ([1,0,0] .
         view_dir) * view_dir)` -- i.e. the component of the WORLD x-axis
         perpendicular to `view_dir` (an arbitrary but fixed choice; this is
         exactly the free parameter the reconciliation above is about).
         `fold_normal = cos(15deg) * view_dir + sin(15deg) * perp`, offset
         `centre @ fold_normal` (plane passes through the visible cloud's
         own centroid).

    Returns (visible_cam, true_mirror_cam, true_depth_separation, fold_cam,
    fold_depth_separation, z_extent), where `z_extent` is
    `visible_cam[:, 2].max() - visible_cam[:, 2].min()` (3.3875 on this
    fixture). Measured on this exact fixture: true plane inside_fraction
    0.999 / depth_separation +0.1792 (+5.29% of z_extent); fold
    inside_fraction 0.9156 / depth_separation -0.0013 (-0.04% of z_extent)
    -- the fold clears MIR_FLOOR (0.85) on fraction alone but sits at
    essentially zero depth separation (sign flips right around this angle;
    a full 0/10/15/20/30-degree sweep with the shared-pixel counts checked
    is in gate.py's MIN_DEPTH_SEPARATION_FRACTION comment), not behind the
    visible surface.
    """
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    verts = s.vertices
    tris = []
    for face in s.faces:
        idx = np.asarray(face)
        for i in range(1, len(idx) - 1):
            tris.append([idx[0], idx[i], idx[i + 1]])
    tris = np.array(tris)
    a, b, c = verts[tris[:, 0]], verts[tris[:, 1]], verts[tris[:, 2]]
    face_normal = np.cross(b - a, c - a)
    face_centroid = (a + b + c) / 3.0
    eye = -s.R.T @ s.t  # camera position in world coordinates
    view = eye - face_centroid
    front = tris[(face_normal * view).sum(axis=1) > 0]  # back-face culled

    fa, fb, fc = verts[front[:, 0]], verts[front[:, 1]], verts[front[:, 2]]
    area = 0.5 * np.linalg.norm(np.cross(fb - fa, fc - fa), axis=1)
    rng = np.random.default_rng(seed)
    target = 30_000
    pick = rng.choice(len(front), size=target, p=area / area.sum())
    u, v = rng.random((target, 1)), rng.random((target, 1))
    swap = (u + v) > 1.0
    u[swap], v[swap] = 1.0 - u[swap], 1.0 - v[swap]
    visible_world = fa[pick] + u * (fb[pick] - fa[pick]) + v * (fc[pick] - fa[pick])
    visible_cam = visible_world @ s.R.T + s.t
    z_extent = float(visible_cam[:, 2].max() - visible_cam[:, 2].min())

    # TRUE bilateral plane: the box's real world symmetry plane, x=0.
    true_mirror_world = _reflect(visible_world, np.array([1.0, 0.0, 0.0]), 0.0)
    true_mirror_cam = true_mirror_world @ s.R.T + s.t
    true_depth_sep = _depth_separation(visible_cam, true_mirror_cam, s.K, s.image.shape[:2])

    # FOLD: reflect in CAMERA space across a plane through the visible
    # cloud's own centroid, normal tilted 15 degrees (about the WORLD
    # x-axis's component perpendicular to view_dir) off the mean view
    # direction (camera is at the camera-space origin, so a point's own
    # direction from the origin IS its view direction). See the docstring
    # above for exactly why this precision matters.
    centre = visible_cam.mean(axis=0)
    view_dir = centre / np.linalg.norm(centre)
    perp = np.array([1.0, 0.0, 0.0]) - (np.array([1.0, 0.0, 0.0]) @ view_dir) * view_dir
    perp /= np.linalg.norm(perp)
    theta = np.radians(15.0)
    fold_normal = np.cos(theta) * view_dir + np.sin(theta) * perp
    fold_cam = _reflect(visible_cam, fold_normal, float(centre @ fold_normal))
    fold_depth_sep = _depth_separation(visible_cam, fold_cam, s.K, s.image.shape[:2])

    return visible_cam, true_mirror_cam, true_depth_sep, fold_cam, fold_depth_sep, z_extent


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
    `run(ship_id, points, mirrored, intrinsics, mask, depth_separation=None,
    z_extent=None) -> dict`, recording
    `silhouette_iou`/`mirrored_fraction`/`mirrored_pass` (plus
    `depth_separation` and `mirrored_pass_reason` when applicable). This is
    an intentional, three-times-revised interface change (the metric the
    gate verdicts on moved from union IoU to a mirrored-fraction-AND-depth
    conjunction, then the depth term moved from an absolute `> 0` check to
    a fraction of `z_extent` -- see `IOU_FLOOR`'s, `MIR_FLOOR`'s, and
    `MIN_DEPTH_SEPARATION_FRACTION`'s comments in gate.py), not a design
    conflict, so this test is updated to the new contract rather than left
    failing against the old one; the property under test -- merge, don't
    clobber -- is unchanged.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    art_dir = gate.paths.artifact_dir("test_ship")
    (art_dir / "quality.json").write_text(json.dumps({"pre_existing_key": 42}))

    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cam = _project_scene(s)
    result = gate.run("test_ship", cam, cam, s.K, mask, depth_separation=0.5, z_extent=1.0)

    data = json.loads((art_dir / "quality.json").read_text())
    assert data["pre_existing_key"] == 42
    assert data["silhouette_iou"] == result["silhouette_iou"]
    assert data["mirrored_fraction"] == result["mirrored_fraction"]
    assert data["mirrored_pass"] == result["mirrored_pass"]
    assert data["depth_separation"] == 0.5


def test_run_returns_a_dict_not_a_bare_float(tmp_path, monkeypatch):
    """The caller must not have to re-derive the verdict from a constant.

    Returning a bare `iou` float (the original design) forced any caller that
    wanted the verdict to recompute `iou >= gate.IOU_FLOOR` itself -- flagged
    in review as a real gap. `run` now returns the dict it also persists, so
    `mirrored_pass` is read directly, not re-derived. Checked both with and
    without `depth_separation`/`z_extent`, since the key set differs: an
    absent depth term adds `mirrored_pass_reason` and omits
    `depth_separation` entirely (never records `None`), a supplied one does
    the reverse when it passes.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cam = _project_scene(s)

    no_depth = gate.run("test_ship_dict_no_depth", cam, cam, s.K, mask)
    assert isinstance(no_depth, dict)
    assert set(no_depth) == {"silhouette_iou", "mirrored_fraction", "mirrored_pass",
                              "mirrored_pass_reason"}
    assert no_depth["mirrored_pass"] is False

    with_depth = gate.run("test_ship_dict_with_depth", cam, cam, s.K, mask,
                           depth_separation=0.5, z_extent=1.0)
    assert set(with_depth) == {"silhouette_iou", "mirrored_fraction", "mirrored_pass",
                                "depth_separation"}
    assert with_depth["mirrored_pass"] is True


def test_run_fails_without_depth_separation_even_with_a_perfect_mirror(tmp_path, monkeypatch):
    """An absent depth term is an unevaluated gate and must never read as a
    pass, no matter how good `mirrored_fraction` looks.

    Without this, a caller that has not yet computed `depth_separation` (or
    forgets to pass it) would get a silent, unjustified `mirrored_pass: true`
    for a mirrored half that has not actually been checked for being a fold
    -- exactly the gap the fold test below demonstrates concretely.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    visible = _project_scene(s)

    result = gate.run("ship_no_depth_term", visible, visible, s.K, mask)
    assert result["mirrored_fraction"] > gate.MIR_FLOOR, result  # would have passed on fraction alone
    assert result["mirrored_pass"] is False
    assert "depth_separation" not in result
    assert "not supplied" in result["mirrored_pass_reason"]


def test_run_fails_without_z_extent_even_with_a_positive_depth_separation(tmp_path, monkeypatch):
    """Paired with the test above: an absent `z_extent` is EQUALLY an
    unevaluated gate, even when `depth_separation` itself is a plausible
    positive number -- `depth_separation` cannot be judged against
    `MIN_DEPTH_SEPARATION_FRACTION` without knowing what fraction of what,
    so this must not silently fall back to a bare `> 0` check.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    visible = _project_scene(s)

    result = gate.run("ship_no_z_extent", visible, visible, s.K, mask, depth_separation=0.5)
    assert result["mirrored_fraction"] > gate.MIR_FLOOR, result
    assert result["mirrored_pass"] is False
    assert "z_extent" not in result       # z_extent itself is never recorded, only checked
    assert "depth_separation" in result   # but the depth_separation that WAS supplied still is
    assert "z_extent not supplied" in result["mirrored_pass_reason"]


def test_run_flags_a_garbage_mirrored_half_that_union_iou_would_pass(tmp_path, monkeypatch):
    """The regression test for the metric change from fix round 1.

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
    (0.85), correctly failing it. Also checked WITH a plausible positive
    `depth_separation`/`z_extent` supplied, to confirm this one fails on
    `mirrored_fraction` specifically, not merely because the depth term
    happened to be missing.
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    visible = _project_scene(s)

    garbage = visible.copy()
    garbage[:, 0] += 3.0  # same displacement as test_displaced_cloud_scores_low
    rng = np.random.default_rng(2)
    garbage = garbage[rng.choice(len(garbage), 2000, replace=False)]

    result = gate.run("ship_bad_mirror", visible, garbage, s.K, mask,
                       depth_separation=0.5, z_extent=1.0)
    assert result["silhouette_iou"] > gate.IOU_FLOOR, result
    assert result["mirrored_fraction"] < gate.MIR_FLOOR, result
    assert result["mirrored_pass"] is False
    assert "MIR_FLOOR" in result["mirrored_pass_reason"]


def test_run_passes_a_mirrored_half_consistent_with_the_matte_and_a_real_depth_term(tmp_path, monkeypatch):
    """Paired with the tests above: a good mirrored half WITH a genuine,
    positive depth separation (checked against the fixture's own real
    z-extent) must pass -- otherwise `mirrored_pass is False` would be
    satisfied by a verdict that always fails, and the conjunction would
    never actually let a solved ship through.

    Uses the geometrically-real TRUE-plane fixture below (not arbitrary
    placeholder numbers) so this is a real positive case, not just "some
    positive float clears some other positive float".
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    visible_cam, true_mirror_cam, true_depth_sep, _, _, z_extent = _one_sided_hull_and_fold()
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)

    # sanity: this fixture IS the genuine case, clearing the threshold with margin
    assert true_depth_sep >= gate.MIN_DEPTH_SEPARATION_FRACTION * z_extent, (true_depth_sep, z_extent)
    result = gate.run("ship_good_mirror_real_depth", visible_cam, true_mirror_cam, s.K, mask,
                       depth_separation=true_depth_sep, z_extent=z_extent)
    assert result["mirrored_fraction"] > gate.MIR_FLOOR, result
    assert result["mirrored_pass"] is True
    assert "mirrored_pass_reason" not in result


def test_run_rejects_a_fold_that_clears_mirrored_fraction_on_its_own(tmp_path, monkeypatch):
    """The regression test for fix round 2, and the whole reason the
    conjunction exists: a fold can clear `MIR_FLOOR` by itself (a fold
    reflects the visible sheet roughly onto itself, so it reprojects inside
    the matte just as well as the true plane), and `mirrored_fraction` has no
    way to tell the difference. `depth_separation` does, because a fold sits
    at the SAME depth as the visible surface rather than genuinely behind it.

    The fold here is built geometrically, not asserted by fiat: reflect a
    back-face-culled ("visible-only") synthetic hull across a plane through
    its own centroid, with a normal 15 degrees (about a precisely stated
    rotation axis -- see `_one_sided_hull_and_fold`'s docstring; "N degrees
    off the mean view direction" alone underdetermines the plane) off the
    mean view direction. Measured on this exact fixture: `mirrored_fraction`
    (this fold's own `inside_fraction`) is 0.9156 -- ABOVE `MIR_FLOOR`
    (0.85) -- while its `depth_separation` is -0.0013, i.e. essentially zero
    and BELOW `MIN_DEPTH_SEPARATION_FRACTION * z_extent` (a small positive
    number): the mirrored half sits at the same depth as the visible
    surface, the definition of a fold, not meaningfully behind it. The true
    bilateral plane on the SAME visible cloud reads 0.999 / +0.179 (+5.29%
    of z_extent) for comparison (see the paired passing test above; the full
    reconciliation against a second, independently-measured sweep that
    disagreed at the same nominal angle -- because of the free rotation-axis
    parameter -- is in gate.py's `MIN_DEPTH_SEPARATION_FRACTION` comment).

    Confirmed load-bearing by construction: dropping the `depth_separation`
    check (i.e. gating on `mirrored_fraction >= MIR_FLOOR` alone) would flip
    this assertion, since 0.9156 clears 0.85 -- this is exactly the
    `test_run_fails_without_depth_separation_even_with_a_perfect_mirror`
    scenario, but with a geometrically real fold instead of a perfect mirror
    standing in for "no depth term available yet".
    """
    monkeypatch.setattr(gate.paths, "FOOTPRINT_ROOT", tmp_path)
    visible_cam, _, _, fold_cam, fold_depth_sep, z_extent = _one_sided_hull_and_fold()
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)

    frac = gate.inside_fraction(fold_cam, s.K, mask)
    assert frac > gate.MIR_FLOOR, frac  # the fold WOULD pass on fraction alone
    # ...but it is not meaningfully behind the visible surface:
    assert fold_depth_sep < gate.MIN_DEPTH_SEPARATION_FRACTION * z_extent, (fold_depth_sep, z_extent)

    result = gate.run("ship_fold", visible_cam, fold_cam, s.K, mask,
                       depth_separation=fold_depth_sep, z_extent=z_extent)
    assert result["mirrored_fraction"] > gate.MIR_FLOOR, result
    assert result["mirrored_pass"] is False
    assert "MIN_DEPTH_SEPARATION_FRACTION" in result["mirrored_pass_reason"]


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
