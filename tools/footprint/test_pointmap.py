"""Checks for the MoGe-2 point map wrapper.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_pointmap.py
"""

import importlib
import pathlib
import sys

import cv2
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
matte = _load("matte")
pointmap = _load("pointmap")

torch = pytest.importorskip("torch")

# pointmap.py imports torch and moge unconditionally at module level, so
# `_load("pointmap")` above already fails collection outright on a box
# without those installed — `pytest.importorskip` above is a courtesy, not
# what actually protects the CUDA-only tests. What protects them is this
# per-function mark on each test that calls pointmap.infer(), not a module-
# wide `pytestmark`: pytest applies a bare module-level `pytestmark`
# assignment to every test in the module regardless of where in the file it
# is written (verified directly — a `test_before()` defined textually above
# a later `pytestmark = pytest.mark.skipif(...)` line still gets skipped),
# so a blanket `pytestmark` would skip the CPU-only composite/load tests
# below too on a machine with no GPU. Decorating only the tests that need
# CUDA is the mechanism that actually gives the composite/load tests
# coverage on a CPU box.
needs_cuda = pytest.mark.skipif(not torch.cuda.is_available(), reason="needs CUDA")


def test_composite_neutral_replaces_only_the_background():
    img = np.zeros((4, 4, 3), dtype=np.uint8)
    img[:] = (255, 0, 255)
    img[1:3, 1:3] = (10, 20, 30)  # a distinct foreground patch
    mask = np.zeros((4, 4), dtype=np.uint8)
    mask[1:3, 1:3] = 1

    out = pointmap._composite(img, mask, "neutral")
    assert np.all(out[mask == 0] == pointmap.NEUTRAL)
    assert np.array_equal(out[mask == 1], img[mask == 1])


def test_composite_raw_is_untouched():
    img = np.random.default_rng(0).integers(0, 255, (4, 4, 3), dtype=np.uint8)
    mask = np.zeros((4, 4), dtype=np.uint8)
    out = pointmap._composite(img, mask, "raw")
    assert np.array_equal(out, img)


def test_composite_rejects_an_unknown_background():
    img = np.zeros((2, 2, 3), dtype=np.uint8)
    mask = np.zeros((2, 2), dtype=np.uint8)
    # Without the ValueError, "raw"/"neutral" typos like "Raw" or "none"
    # would silently fall through to the neutral-composite branch (it's an
    # `if ... == "raw": ... else: composite`), corrupting the input for
    # anything that assumed it asked for "raw". Verified: reverting
    # _composite to the two-branch if/else (no explicit rejection) makes
    # this test fail with `Failed: DID NOT RAISE ValueError` rather than
    # erroring some other way, confirming the check is what's asked for.
    with pytest.raises(ValueError, match="sepia"):
        pointmap._composite(img, mask, "sepia")


def test_load_reads_a_handwritten_npz(tmp_path, monkeypatch):
    # Exercises load() against an npz it did not write itself, so a bug tying
    # load() to exactly what run() happens to produce (field order, dtype)
    # wouldn't be masked by round-tripping through the same code.
    monkeypatch.setattr(pointmap.paths, "FOOTPRINT_ROOT", tmp_path)
    d = pointmap.paths.artifact_dir("handwritten")
    np.savez_compressed(d / "cloud.npz",
                        points=np.array([[1.0, 2.0, 3.0]], dtype=np.float32),
                        pixels=np.array([[10.0, 20.0]], dtype=np.float32),
                        normals=np.array([]),
                        intrinsics=np.eye(3, dtype=np.float32))

    c = pointmap.load("handwritten")
    assert np.allclose(c.points, [[1.0, 2.0, 3.0]])
    assert np.allclose(c.pixels, [[10.0, 20.0]])
    assert c.normals is None
    assert np.allclose(c.intrinsics, np.eye(3))


@pytest.fixture(scope="module")
def scene():
    return synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)


@pytest.fixture(scope="module")
def scene_mask(scene):
    mask, _ = matte.extract(scene.image)
    return mask


@pytest.fixture(scope="module")
def cloud(scene, scene_mask):
    return pointmap.infer(scene.image, scene_mask)


@needs_cuda
def test_cloud_covers_only_the_subject(cloud, scene_mask):
    c = cloud
    # Measured on this fixture: scene_mask.sum() == 75192 foreground pixels,
    # and a correct infer() recovers exactly 75192 points (ratio 1.0 — this
    # solid-shaded synthetic render has no ambiguous silhouette edges for
    # MoGe's own mask to disagree with the matte on). The 0.5x floor from an
    # earlier round of this test was 2x looser than that measurement
    # supports and no mutation reddened it; 0.98x is what the actual ratio
    # backs.
    #
    # The brief's `> 1000` is true of almost anything short of a nearly-empty
    # result and does not scale with image size at all. Measured against a
    # deliberately broken build that drops the `& (mask > 0)` term (uses only
    # MoGe's own mask, forgetting the stage-1 matte entirely): that build
    # still returns 75966 points — `> 1000` stays green either way, so it
    # would not have caught this regression.
    #
    # What DOES catch it is the pre-existing per-pixel membership check two
    # lines down (`scene_mask[px[:, 1], px[:, 0]].all()`): the 774 extra
    # points from the broken build sit on pixels the matte marks background,
    # so that assertion goes red (verified directly: without the matte AND,
    # `scene_mask[...].all()` is False). That check was already in the brief
    # and needed no change — it is the robust guard against this failure
    # mode, at single-pixel precision, not a size threshold.
    #
    # The count assertion below is kept, but re-targeted at a different real
    # regression the membership check does not cover: silently returning far
    # fewer points than the true foreground affords (e.g. an accidental
    # resolution mismatch or an overly aggressive validity threshold), which
    # would still clear `> 1000` trivially on any image above thumbnail size.
    assert len(c.points) <= scene_mask.sum(), (len(c.points), scene_mask.sum())
    assert len(c.points) > 0.98 * scene_mask.sum(), (len(c.points), scene_mask.sum())
    assert c.points.shape[1] == 3
    assert c.pixels.shape == (len(c.points), 2)
    # Every returned point must come from a foreground pixel.
    px = c.pixels.astype(int)
    assert scene_mask[px[:, 1], px[:, 0]].all()


@needs_cuda
def test_moge_validity_mask_is_respected(scene, scene_mask):
    # test_cloud_covers_only_the_subject above only ever tested dropping the
    # *matte* half of `valid = out["mask"] & (mask > 0)` (the brief's own
    # fixture happens to make MoGe's own mask a strict superset of the
    # matte, 75966 superset of 75192, so dropping the MoGe term there changes
    # nothing and would leave this half of the intersection completely
    # untested). This test forces the two terms to disagree: feed infer() a
    # matte deliberately dilated past the true silhouette, so some of the
    # extra "foreground" pixels are ones MoGe's own mask marks invalid
    # (`+inf`, per moge.model.v2.MoGeModel.infer's apply_mask branch) and a
    # build that trusted the matte alone would return non-finite points.
    big = cv2.dilate(scene_mask, np.ones((41, 41), np.uint8))
    assert big.sum() > scene_mask.sum(), "dilation must actually grow the mask"

    c = pointmap.infer(scene.image, big)
    # Measured: correct code returns 103958 finite points out of big.sum()
    # == 104112 dilated-mask pixels. A mutant that computes
    # `valid = (big > 0)` (drops the `out["mask"]` term) returns all 104112,
    # every one of them non-finite — both assertions below go red on that
    # mutant, confirmed directly.
    assert np.isfinite(c.points).all()
    assert len(c.points) < big.sum()


@needs_cuda
def test_cloud_is_finite_and_in_front_of_the_camera(cloud):
    c = cloud
    assert np.isfinite(c.points).all()
    # Documented invariant, NOT a check on our code: MoGe emits camera-space
    # points with strictly positive depth by construction (mask_binary &=
    # points[..., 2] > 0 runs unconditionally in MoGeModel.infer before
    # masking, and invalid pixels are set to +inf rather than a negative or
    # merely-finite value — so `> 0` cannot distinguish a real bug from
    # correct behaviour here; `inf > 0` is True, which is exactly why this
    # line by itself cannot tell "included a masked-out pixel" apart from
    # "included a valid one" — np.isfinite above is the only assertion in
    # this pair that is actually load-bearing). This is recorded here
    # because gate.reproject relies on it when it filters
    # points[:, 2] > 1e-6.
    assert (c.points[:, 2] > 0).all()


@needs_cuda
def test_intrinsics_are_returned_and_plausible(cloud, scene):
    c, s = cloud, scene
    assert c.intrinsics.shape == (3, 3)
    # MoGe returns normalised intrinsics; denormalised focal should be within
    # a factor of two of the true one. A wilder value means the FOV recovery
    # disagrees with the render and stage 2 should be believed instead.
    _, w = s.image.shape[:2]
    focal_px = c.intrinsics[0, 0] * w
    assert 0.5 < focal_px / s.K[0, 0] < 2.0, focal_px


@needs_cuda
def test_normals_are_unit_length_when_present(cloud):
    c = cloud
    if c.normals is None:
        pytest.skip("model did not return normals")
    assert c.normals.shape == c.points.shape
    norms = np.linalg.norm(c.normals, axis=1)
    # Unit normals only, not merely finite: a bug that fed the points array
    # into the normals slot instead would still be finite and correctly
    # shaped but would not cluster around length 1. Measured: real normals
    # have norm in [0.99999988, 1.00000012]; the points-instead-of-normals
    # substitution measures [5.36, 6.84] on this fixture — comfortably
    # outside atol=0.05 either way.
    assert np.allclose(norms, 1.0, atol=0.05), (norms.min(), norms.max())


@needs_cuda
def test_run_writes_cloud_and_load_round_trips(scene, scene_mask, tmp_path, monkeypatch):
    monkeypatch.setattr(pointmap.paths, "FOOTPRINT_ROOT", tmp_path)
    written = pointmap.run("test_ship", scene.image, scene_mask)
    assert (tmp_path / "test_ship" / "cloud.npz").exists()

    loaded = pointmap.load("test_ship")
    assert np.allclose(loaded.points, written.points)
    assert np.allclose(loaded.pixels, written.pixels)
    assert np.allclose(loaded.intrinsics, written.intrinsics)
    if written.normals is None:
        assert loaded.normals is None
    else:
        assert np.allclose(loaded.normals, written.normals)
