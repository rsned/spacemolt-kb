"""Checks for canonicalisation and station sampling.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_profile.py
"""

import importlib
import pathlib
import sys

import numpy as np
from shapely.geometry import Polygon

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

profile = _load("profile")


def test_canonicalise_puts_the_long_axis_on_x_and_scales_to_unit_length():
    # A 4 x 1 rectangle lying along Y, so canonicalisation must rotate it.
    poly = Polygon([(-0.5, -2), (0.5, -2), (0.5, 2), (-0.5, 2)])
    out = profile.canonicalise(poly, sym_normal_xy=np.array([1.0, 0.0]))
    xs, ys = np.array(out.exterior.coords).T
    assert abs((xs.max() - xs.min()) - 1.0) < 1e-6
    assert abs((ys.max() - ys.min()) - 0.25) < 1e-6


def test_sample_returns_half_widths_of_a_unit_rectangle():
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, concave = profile.sample(poly)
    assert len(w) == profile.STATIONS
    assert len(concave) == profile.STATIONS
    interior = w[5:-5]
    assert np.allclose(interior, 0.1, atol=1e-6), interior
    assert not concave.any()


def test_sample_flags_a_split_cross_section():
    # Two bars with a gap between them: the station cuts two intervals, which
    # a single half-width per station cannot represent.
    poly = Polygon([(0, -0.3), (1, -0.3), (1, -0.1), (0, -0.1)]).union(
        Polygon([(0, 0.1), (1, 0.1), (1, 0.3), (0, 0.3)]))
    w, concave = profile.sample(poly)
    mid = profile.STATIONS // 2      # NOT a hardcoded 48: STATIONS is 96 today,
                                     # and a literal index silently tests the
                                     # wrong station if that constant changes.
    assert concave[mid], "a split cross-section must be flagged"
    assert abs(w[mid] - 0.3) < 1e-6, w[mid]


def test_aspect_matches_length_over_maximum_beam():
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, _ = profile.sample(poly)
    assert abs(profile.aspect(w) - 5.0) < 1e-6, profile.aspect(w)


def test_canonicalise_accepts_a_multi_part_footprint():
    """A twin-nacelle footprint is a MultiPolygon, which has no `.exterior`.

    Reading `poly.exterior` raises AttributeError on exactly the shape this
    pipeline exists to represent. The defect was previously masked by
    `ground.hull` collapsing to its largest part, which silently discarded the
    other nacelle — so this test and Task 7's `test_hull_of_two_bars_stays_multi_part`
    guard the two halves of one bug. It lives here rather than in Task 7 because
    it needs both modules, and `profile.py` does not exist at Task 7 time.
    """
    ground = _load("ground")
    rng = np.random.default_rng(0)
    xy = np.vstack([rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2)),
                    rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))])
    poly = ground.hull(xy, alpha=3.0)
    assert poly.geom_type == "MultiPolygon", poly.geom_type

    canon = profile.canonicalise(poly, np.array([0.0, 1.0]))
    xs = np.concatenate([np.array(g.exterior.coords)[:, 0] for g in canon.geoms])
    assert abs((xs.max() - xs.min()) - 1.0) < 1e-6, xs.max() - xs.min()


def test_sample_of_a_multi_part_footprint_does_not_crash():
    """`sample` must handle the MultiPolygon that `canonicalise` now returns.

    A station cut across two nacelles yields a MultiLineString; a station in the
    gap between them (if the parts do not span the full length) yields nothing.
    Both paths must produce a finite half-width array of the right length.
    """
    ground = _load("ground")
    rng = np.random.default_rng(0)
    xy = np.vstack([rng.uniform([-2, -1.0], [2, -0.4], size=(3000, 2)),
                    rng.uniform([-2, 0.4], [2, 1.0], size=(3000, 2))])
    canon = profile.canonicalise(ground.hull(xy, alpha=3.0), np.array([0.0, 1.0]))
    w, concave = profile.sample(canon)
    assert len(w) == profile.STATIONS
    assert np.isfinite(w).all()
    assert concave[profile.STATIONS // 2], "a two-nacelle cut must flag as split"


def test_a_plausible_hull_passes_the_dimensional_check():
    """A 5:1 hull with real depth must not be rejected.

    The bounds exist to catch pancakes and slivers, so they must not fire on an
    ordinary hull — a check that rejects everything is as useless as one that
    rejects nothing.
    """
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, _ = profile.sample(poly)
    assert abs(profile.aspect(w) - 5.0) < 1e-6, profile.aspect(w)
    # beam = 0.2, so depth 0.08 is depth/beam = 0.40, well above the 0.15 floor
    assert profile.implausible(w, depth_extent=0.08) is None


def test_a_flattened_reconstruction_is_rejected_as_a_pancake():
    """The case stage 5 provably cannot catch.

    A cloud flattened along the viewing rays reprojects to the SAME silhouette —
    measured IoU 0.9910 either way — so it arrives here with silhouette_pass
    true. If this check does not fire, nothing in the pipeline does.
    """
    poly = Polygon([(0, -0.1), (1, -0.1), (1, 0.1), (0, 0.1)])
    w, _ = profile.sample(poly)
    reason = profile.implausible(w, depth_extent=0.01)   # depth/beam = 0.05
    assert reason is not None, "a flat card must not be publishable"
    assert "flat card" in reason, reason


def test_an_implausible_aspect_is_rejected_and_says_why():
    """Both ends of the aspect band, and the reason string is part of the API.

    The batch report names why each ship was excluded, so an empty or generic
    reason would make a failure unactionable.
    """
    stubby = Polygon([(0, -0.6), (1, -0.6), (1, 0.6), (0, 0.6)])     # aspect 0.83
    w, _ = profile.sample(stubby)
    r = profile.implausible(w, depth_extent=1.0)
    assert r is not None and "stubby" in r, r

    sliver = Polygon([(0, -0.02), (1, -0.02), (1, 0.02), (0, 0.02)])  # aspect 25
    w, _ = profile.sample(sliver)
    r = profile.implausible(w, depth_extent=1.0)
    assert r is not None and "sliver" in r, r
