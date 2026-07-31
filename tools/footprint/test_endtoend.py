"""End-to-end checks for the footprint pipeline.

Stages 4-7 are asserted exactly against ground truth. The MoGe stage is
reported, not asserted: it is a network being shown a synthetic render, and a
hard assertion there would fail for reasons unrelated to this code.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_endtoend.py -v -s
"""

import importlib
import pathlib
import sys

import numpy as np
import pytest

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
mirror = _load("mirror")
ground = _load("ground")
profile = _load("profile")

# Match test_pointmap.py exactly. Do NOT write the guard as
# `pytest.mark.skipif(not __import__("importlib").util.find_spec("torch"), ...)`:
# `importlib.util` is a SUBMODULE and is not bound by `import importlib`, so that
# expression raises `AttributeError: module 'importlib' has no attribute 'util'`
# in plain Python. Verified — it only appears to work under pytest, because
# pytest imports importlib.util itself as a side effect. Relying on another
# library's imports to make your guard evaluable is a latent collection failure.
#
# Also per-function marks, never a module-level `pytestmark`: pytest applies a
# bare `pytestmark` to every test in the module regardless of where the line sits
# in the file, which would skip the two CPU-only ground-truth tests on a box with
# no GPU — and those are the tests that assert our own arithmetic.
torch = pytest.importorskip("torch")
needs_cuda = pytest.mark.skipif(not torch.cuda.is_available(), reason="needs CUDA")


def _ground_truth_chain(scene):
    """Stages 4-7 on a perfect cloud: our arithmetic, no network involved."""
    cam = scene.points @ scene.R.T + scene.t
    # `solve`, not `solve_from_view`: scene.points is a TWO-SIDED volume sampled
    # over the whole mesh, which is the regime self-chamfer handles correctly.
    # This chain exists to validate our arithmetic, so it must not depend on the
    # silhouette machinery the real path uses.
    sym = mirror.solve(cam)
    full = mirror.complete(cam, sym)
    up = scene.up @ scene.R.T
    xy = ground.project(full, up)
    poly = ground.hull(xy, alpha=3.0)
    sym_xy = ground.project(sym.normal[None, :], up)[0]
    return profile.sample(profile.canonicalise(poly, sym_xy))


def test_box_recovers_a_rectangular_profile():
    s = synth.box_scene(length=4.0, width=2.0, height=1.5,
                        azimuth_deg=35, elevation_deg=30)
    w, concave = _ground_truth_chain(s)

    interior = w[8:-8]
    assert interior.std() / interior.mean() < 0.10, "a box must have constant beam"
    assert abs(profile.aspect(w) - 2.0) / 2.0 < 0.15, profile.aspect(w)
    assert not concave.any()


def test_vertical_cylinder_recovers_a_circular_profile():
    s = synth.cylinder_scene(radius=1.0, height=3.0,
                             azimuth_deg=20, elevation_deg=35)
    w, _ = _ground_truth_chain(s)

    # A circle of unit length has half-width sqrt(t - t^2) / 2 ... check the
    # midships beam equals the length, i.e. aspect 1.
    assert abs(profile.aspect(w) - 1.0) < 0.20, profile.aspect(w)
    mid, near_end = profile.STATIONS // 2, profile.STATIONS // 12
    assert w[mid] > w[near_end] and w[mid] > w[-near_end], \
        "a circle must be widest amidships"


@needs_cuda
def test_moge_chain_is_reported_not_asserted(capsys):
    pointmap = _load("pointmap")
    matte = _load("matte")
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cloud = pointmap.infer(s.image, mask)
    # A MoGe cloud is one-sided even on a synthetic render, so this is the
    # view-based solve, not the two-sided one.
    sym = mirror.solve_from_view(cloud.points, mask, cloud.intrinsics)
    with capsys.disabled():
        print(f"\n  MoGe on synthetic box: n={len(cloud.points)} "
              f"mirror residual={sym.residual:.4f} "
              f"(ceiling {mirror.RESIDUAL_CEILING})")
    assert len(cloud.points) > 0


@needs_cuda
def test_moge_clears_a_floor_on_real_hero_art():
    """The gate on what we actually depend on.

    The synthetic check above is reporting-only because a flat-shaded render is
    out of distribution for MoGe. A hero image is not: it is a rendered object
    on a plain ground, which is what the model was trained on. So this one
    asserts. Skips when the art is absent rather than failing in a clean
    checkout.
    """
    import cv2
    pointmap = _load("pointmap")
    matte = _load("matte")
    paths = _load("paths")

    hero = paths.HERO_DIR / "outerrim_prayer.webp"
    if not hero.exists():
        pytest.skip(f"no hero art at {hero}")

    img = cv2.cvtColor(cv2.imread(str(hero)), cv2.COLOR_BGR2RGB)
    mask, _ = matte.extract(img)
    cloud = pointmap.infer(img, mask)

    assert len(cloud.points) > 10_000, "point map is implausibly sparse"
    assert np.isfinite(cloud.points).all()
    # Documented invariant, NOT a test of our code: MoGe emits camera-space
    # points with strictly positive depth by construction (measured z in
    # [23.9, 137] even on pure random-noise input), so this cannot fail unless
    # a later change starts transforming the cloud. It is here because
    # gate.reproject relies on it when it filters points[:, 2] > 1e-6.
    assert (cloud.points[:, 2] > 0).all(), "points behind the camera"

    # A hull seen in 3/4 must have real depth relief. A collapsed depth range
    # means the model read the image as a flat card, which would make every
    # downstream footprint a silhouette rather than a shape.
    d = cloud.points[:, 2]
    assert d.std() / d.mean() > 0.02, f"depth relief {d.std() / d.mean():.4f} too flat"

    sym = mirror.solve_from_view(cloud.points, mask, cloud.intrinsics)
    assert sym.residual < 0.25, f"Prayer is a boxy hull; residual {sym.residual:.3f}"
    # The fold check, which is the failure this stage actually had: a folded
    # plane leaves the completed cloud barely larger than the visible one
    # (measured 1.035x with the superseded self-chamfer solve). A genuine
    # occluded half must add real width.
    full = mirror.complete(cloud.points, sym)
    ext = float(np.linalg.norm(cloud.points.max(0) - cloud.points.min(0)))
    gain = float(np.linalg.norm(full.max(0) - full.min(0))) / ext
    assert gain > 1.15, f"completion added almost nothing ({gain:.3f}x): plane folded"
