"""Checks for ground projection and the alpha-shape footprint.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_ground.py
"""

import importlib
import logging
import pathlib
import sys
import types

import numpy as np

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

ground = _load("ground")
camera = _load("camera")


def test_projection_drops_the_up_axis():
    pts = np.array([[1.0, 5.0, 2.0], [3.0, -7.0, 4.0]])
    xy = ground.project(pts, up=np.array([0.0, 1.0, 0.0]))
    assert np.allclose(xy, [[1.0, 2.0], [3.0, 4.0]])


def _hull_cloud(n=4000, seed=0):
    """A hull whose three axes have DISTINCT extents, in a rotated frame.

    Extents must differ: measured 3.999 long, 1.599 wide, 0.800 tall. If two
    were equal the principal axis would be ambiguous and the test could pass on
    a solver that picked either. The rotation angle (1.15 rad) is chosen so a
    stub returning a fixed coordinate axis fails with margin. Measured |dot|
    against the true up, all three against the 0.98 bar:

        fixed +Z axis                       0.798
        centroid direction (the old         0.167
          normals-mean fallback's shape)
        longitudinal instead of up          0.001
        the intended derivation             0.999999

    (An earlier draft of this docstring recorded 0.798 for the centroid stub as
    well. Measured, it is 0.167 — the stub fails harder than claimed, not less
    hard, but the number was wrong and is corrected here.)
    """
    rng = np.random.default_rng(seed)
    local = rng.uniform([-2.0, -0.8, -0.4], [2.0, 0.8, 0.4], size=(n, 3))
    axis = np.array([0.3, 0.5, 0.81])
    axis /= np.linalg.norm(axis)
    c, s = np.cos(1.15), np.sin(1.15)
    K = np.array([[0, -axis[2], axis[1]], [axis[2], 0, -axis[0]], [-axis[1], axis[0], 0]])
    R = np.eye(3) * c + s * K + (1 - c) * np.outer(axis, axis)
    # local x = longitudinal, y = lateral, z = up
    return local @ R.T, R[:, 1], R[:, 2]


def test_up_vector_comes_from_the_hull_geometry():
    """Recover a known up from the symmetry plane and the cloud's own axes.

    The cloud is deliberately rotated out of axis alignment and given three
    distinct extents, so this fails for a stub returning a fixed axis, for one
    returning the mean normal, and for one that confuses the longitudinal and
    up axes. `fit=None` throughout: the geometric path is the primary one, not
    a fallback.
    """
    pts, lateral, up_true = _hull_cloud()
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    up = ground.up_vector(sym, pts, normals=None, fit=None)
    assert abs(abs(float(up @ up_true)) - 1.0) < 0.02, up @ up_true


def test_up_vector_ignores_a_confident_but_wrong_camera_fit():
    """A confident camera fit must not override the geometric frame.

    Stage 2 is a cross-check (see Global Constraints). One of fourteen real
    images produces a confident fit at all, and on a synthetic oracle roughly
    one seed in thirty produces a confident fit that is 24 degrees wrong, so a
    code path that deferred to `fit` would import that error. Passing a
    deliberately garbage R with high confidence must change nothing.
    """
    pts, lateral, up_true = _hull_cloud()
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    baseline = ground.up_vector(sym, pts, normals=None, fit=None)
    bogus = types.SimpleNamespace(R=np.eye(3), confidence=0.99, source="auto")
    assert np.allclose(ground.up_vector(sym, pts, normals=None, fit=bogus), baseline)


def test_up_vector_sign_follows_the_viewer():
    """Up must point toward the camera side, not away from it.

    Sign is the one thing geometry alone cannot settle — lateral x longitudinal
    is up or down depending on handedness — so it is resolved from the normals.
    Flipping every normal must flip the returned up, or the sign logic is dead
    code and the footprint could come out mirrored.
    """
    pts, lateral, _ = _hull_cloud()
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    normals = np.tile(np.array([0.0, 0.0, -1.0]), (len(pts), 1))
    a = ground.up_vector(sym, pts, normals=normals, fit=None)
    b = ground.up_vector(sym, pts, normals=-normals, fit=None)
    assert float(a @ b) < -0.98, (a, b)


def test_up_vector_logs_agreement_with_a_confident_fit(caplog):
    """The plan requires a confident stage-2 fit's agreement with the
    geometric frame to be LOGGED (never used to steer the answer -- see
    `up_vector`'s docstring and the module docstring on `mirror.Symmetry`).
    `test_up_vector_ignores_a_confident_but_wrong_camera_fit` only proves
    `fit` doesn't override the result, which is trivially true if `fit` is
    simply unused -- it does not prove the logging side of the contract is
    implemented at all. This does.
    """
    pts, lateral, _ = _hull_cloud()
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    confident = types.SimpleNamespace(
        R=np.eye(3), confidence=camera.CONFIDENCE_FLOOR + 0.01, source="auto")
    with caplog.at_level(logging.INFO, logger="tools.footprint.ground"):
        ground.up_vector(sym, pts, normals=None, fit=confident)
    assert any("agreement" in r.getMessage().lower() for r in caplog.records), caplog.records


def test_up_vector_is_silent_on_an_unconfident_fit(caplog):
    """Below stage 2's own confidence floor, `fit.R` is `np.eye(3)` --
    unrotated filler, not a real measurement (see `camera.fit`'s own floor
    check). There is nothing worth comparing, so nothing should be logged.
    """
    pts, lateral, _ = _hull_cloud()
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    unconfident = types.SimpleNamespace(
        R=np.eye(3), confidence=camera.CONFIDENCE_FLOOR - 0.01, source="auto")
    with caplog.at_level(logging.INFO, logger="tools.footprint.ground"):
        ground.up_vector(sym, pts, normals=None, fit=unconfident)
    assert not caplog.records, caplog.records


def test_up_vector_sign_agrees_between_normals_and_fallback_paths():
    """Both sign-resolution paths describe the SAME physical convention --
    "which side is the toward-camera side" -- and must resolve the same sign
    on a scene that satisfies both: a cloud offset away from a camera at the
    origin, with normals pointing back toward it (viewer-facing, as any real
    visible surface's normals do). Real ships always supply normals, so if
    the two paths disagreed, this fixture's own no-normals fallback path
    (exercised by the other tests in this file) would be silently inverted
    relative to what real footprints actually get.
    """
    pts, lateral, _ = _hull_cloud()
    pts = pts + np.array([0.0, 0.0, 20.0])  # offset away from a camera at the origin
    sym = types.SimpleNamespace(normal=lateral, offset=0.0)
    direction = pts.mean(axis=0)
    direction /= np.linalg.norm(direction)
    normals = np.tile(-direction, (len(pts), 1))  # viewer-facing: back toward the camera
    with_normals = ground.up_vector(sym, pts, normals=normals, fit=None)
    without_normals = ground.up_vector(sym, pts, normals=None, fit=None)
    assert np.allclose(with_normals, without_normals), (with_normals, without_normals)


def test_alpha_shape_keeps_a_concavity_a_convex_hull_would_erase():
    """Two parallel bars with a gap: the nacelle case.

    Measured on this exact fixture (true two-bar area 4.800, convex hull 7.987):
    the alpha shape is a single gap-filled blob up to alpha 2.4 (area 7.85-7.88)
    and correctly splits into 2 parts from alpha 2.5 up. alpha=3.0 therefore
    sits above the transition with margin.

    The area assertion is what makes this test bite. Without it, the two
    `contains` checks pass on an implementation that keeps only the LARGEST
    part — that part still holds one bar's interior point, and the gap is
    trivially absent from it. Measured: max-area returns 2.336 (one bar),
    keeping both returns 4.663. So the area bound below is the assertion that
    fails for `max(geoms, key=area)`, and it is the bug this fixture exists to
    catch.
    """
    rng = np.random.default_rng(0)
    left = rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2))
    right = rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))
    xy = np.vstack([left, right])

    poly = ground.hull(xy, alpha=3.0)
    assert not poly.contains_properly(_point(0.0, 0.0)), "gap was filled in"
    assert poly.contains(_point(0.0, -0.7)), "lost the lower bar"
    assert poly.contains(_point(0.0, 0.7)), "lost the upper bar"
    assert poly.area > 4.0, f"only kept part of the hull: area {poly.area:.3f}"
    assert poly.area < 6.0, f"gap partially filled: area {poly.area:.3f}"


def test_hull_of_two_bars_stays_multi_part():
    """The twin-nacelle footprint must survive as a MultiPolygon.

    This is the `hull` half of the pair guarded here; the `profile.canonicalise`
    half lives in Task 8's test file, because `profile.py` does not exist yet at
    Task 7 time and importing it here raises ModuleNotFoundError at collection.
    """
    rng = np.random.default_rng(0)
    xy = np.vstack([rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2)),
                    rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))])
    poly = ground.hull(xy, alpha=3.0)
    assert poly.geom_type == "MultiPolygon", poly.geom_type


def _point(x, y):
    from shapely.geometry import Point
    return Point(x, y)


def test_alpha_shape_of_a_solid_rectangle_has_the_right_area():
    rng = np.random.default_rng(1)
    xy = rng.uniform([-2, -1], [2, 1], size=(6000, 2))
    poly = ground.hull(xy, alpha=3.0)
    assert abs(poly.area - 8.0) / 8.0 < 0.15, poly.area


def test_sweep_picks_the_tightest_alpha_that_keeps_its_points():
    """Assert the VALUE, not merely membership in the candidate list.

    `assert alpha in candidates` cannot fail: sweep_alpha returns a candidate by
    construction, so a stub returning `candidates[0]` (0.5) and a stub returning
    `max(candidates)` (20.0) both pass it. Both are wrong.

    Measured on these three clouds: alpha 0.5 keeps 2000/2000 points (area
    7.849, 1 part), alpha 3.0 keeps 2000/2000 (area 7.764, 1 part), alpha 20.0
    fragments into 3 parts and keeps only 1644/2000 = 82.2%, failing the 90%
    retention bar. So 3.0 is the tightest passing candidate and the only correct
    answer.
    """
    rng = np.random.default_rng(2)
    clouds = [rng.uniform([-2, -1], [2, 1], size=(2000, 2)) for _ in range(3)]
    alpha = ground.sweep_alpha(clouds, candidates=[0.5, 3.0, 20.0])
    assert alpha == 3.0, alpha
    assert isinstance(alpha, float)
