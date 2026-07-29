"""Checks for the vanishing-point camera fit.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_camera.py
"""

import importlib
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
camera = _load("camera")


def _axis_angle(a, b):
    """Angle in degrees between two directions, ignoring sign."""
    c = np.clip(abs(float(np.dot(a, b))) / (np.linalg.norm(a) * np.linalg.norm(b)), -1, 1)
    return float(np.degrees(np.arccos(c)))


def _best_permutation_error(R_fit, R_true):
    """Largest axis error under the best matching of fitted axes to true axes.

    The fit recovers three orthogonal directions but not which is which, so a
    row-by-row comparison would fail on a correct answer that came back in a
    different order.
    """
    import itertools
    best = 180.0
    for perm in itertools.permutations(range(3)):
        worst = max(_axis_angle(R_fit[perm[i]], R_true[i]) for i in range(3))
        best = min(best, worst)
    return best


def test_detect_segments_finds_the_box_edges():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    seg = camera.detect_segments(s.image, mask, min_length=30.0)

    assert seg.ndim == 2 and seg.shape[1] == 4, seg.shape
    # A shaded box shows at least its silhouette and the three interior edges
    # meeting at the near corner. Fewer than six means the detector or the
    # masking is broken, not that the scene is simple.
    assert len(seg) >= 6, len(seg)


def test_fit_recovers_the_synthetic_rotation_within_five_degrees():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)

    assert fit.confidence > camera.CONFIDENCE_FLOOR, fit.confidence
    # A vanishing point is where lines PARALLEL TO A WORLD AXIS converge in the
    # image; that image-space ray, as a camera-frame direction, is R @ e_axis —
    # a whole COLUMN of the world-to-camera R, not a row. camera.fit stacks the
    # three recovered directions as rows, so it recovers s.R.T (row-permuted,
    # sign-ambiguous per row, both of which _best_permutation_error tolerates),
    # not s.R. Verified directly: comparing against s.R gives a ~38 deg "best"
    # error (no permutation fixes it — that's a genuine row/column mismatch,
    # not fit noise); comparing against s.R.T gives ~0.3 deg. This was checked
    # by projecting a known world-X-axis segment through s.R/s.K by hand and
    # confirming its image-space vanishing direction equals s.R's column 0,
    # not its row 0 — a brief-independent derivation, so this is a fix to the
    # test's ground truth, not a tuning of the implementation to pass it.
    err = _best_permutation_error(fit.R, s.R.T)
    assert err < 5.0, f"worst axis off by {err:.1f} deg"


def test_fit_recovers_the_synthetic_focal_within_ten_percent():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)

    assert fit.focal is not None, "a perspective render must not be called ortho"
    assert abs(fit.focal - s.K[0, 0]) / s.K[0, 0] < 0.10, (fit.focal, s.K[0, 0])


def test_fit_is_deterministic():
    # The RANSAC is seeded. Two fits of the same image must agree exactly, or
    # every downstream artifact becomes irreproducible.
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    a, b = camera.fit(s.image, mask), camera.fit(s.image, mask)

    assert np.allclose(a.R, b.R), "rotation differs between identical fits"
    assert a.focal == b.focal
    assert a.confidence == b.confidence


def test_clicks_fallback_matches_the_known_camera():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    fit = camera.fit_from_clicks(camera.clicks_from_scene(s),
                                 principal=(s.K[0, 2], s.K[1, 2]))

    # Same row/column convention note as the auto-fit rotation test above:
    # fit_from_clicks shares camera.fit's direction-construction code path, so
    # it recovers s.R.T too. Measured directly: fit.R vs s.R.T is exactly 0.0
    # deg here (clicks give exact vanishing points, no detector noise).
    assert _best_permutation_error(fit.R, s.R.T) < 5.0
    assert fit.source == "clicks"
    assert abs(fit.focal - s.K[0, 0]) / s.K[0, 0] < 0.10


def test_clicks_reject_parallel_lines():
    # Two parallel clicked lines never intersect, so the vanishing point is
    # undefined. That must raise, not silently return a huge coordinate.
    clicks = {"axis_x": [[[0, 0], [100, 0]], [[0, 50], [100, 50]]],
              "axis_y": [[[0, 0], [0, 100]], [[50, 0], [50, 100]]],
              "axis_z": [[[0, 0], [100, 100]], [[10, 0], [110, 100]]]}
    try:
        camera.fit_from_clicks(clicks)
    except ValueError as e:
        assert "parallel" in str(e).lower(), str(e)
    else:
        raise AssertionError("parallel clicked lines must raise")


def test_featureless_subject_reports_low_confidence():
    # A smooth ellipsoid has no straight structural edges, so there is no
    # Manhattan frame to recover. The fit must say so rather than invent one.
    s = synth.cylinder_scene(radius=2.0, height=0.2, azimuth_deg=0, elevation_deg=89)
    mask, _ = matte.extract(s.image)
    fit = camera.fit(s.image, mask)

    assert fit.confidence <= camera.CONFIDENCE_FLOOR, fit.confidence
