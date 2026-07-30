"""Checks for the MoGe-2 point map wrapper.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_pointmap.py
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
matte = _load("matte")
pointmap = _load("pointmap")

torch = pytest.importorskip("torch")
pytestmark = pytest.mark.skipif(not torch.cuda.is_available(), reason="needs CUDA")


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


def test_cloud_covers_only_the_subject(cloud, scene_mask):
    c = cloud
    # Measured on this fixture: scene_mask.sum() == 75192 foreground pixels,
    # and a correct infer() recovers exactly 75192 points (ratio 1.0 — this
    # solid-shaded synthetic render has no ambiguous silhouette edges for
    # MoGe's own mask to disagree with the matte on).
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
    assert len(c.points) > 0.5 * scene_mask.sum(), (len(c.points), scene_mask.sum())
    assert c.points.shape[1] == 3
    assert c.pixels.shape == (len(c.points), 2)
    # Every returned point must come from a foreground pixel.
    px = c.pixels.astype(int)
    assert scene_mask[px[:, 1], px[:, 0]].all()


def test_cloud_is_finite_and_in_front_of_the_camera(cloud):
    c = cloud
    assert np.isfinite(c.points).all()
    # MoGe's own mask already guarantees returned points have positive depth
    # (moge.model.v2.MoGeModel.infer runs `mask_binary &= points[..., 2] > 0`
    # unconditionally before masking), so on its own this assertion can't
    # catch a bug in *which* pixels we keep — the isfinite check above and
    # the coverage check in test_cloud_covers_only_the_subject already cover
    # that. It DOES catch a sign/axis-convention bug in how Cloud.points gets
    # assembled: measured directly against a deliberately broken build that
    # negated z (`points[..., 2] *= -1` before returning), np.isfinite stays
    # True (negating a finite number is still finite) while `(z > 0).all()`
    # goes False as expected — so it is not redundant with the isfinite
    # check, it guards a distinct failure mode.
    assert (c.points[:, 2] > 0).all()


def test_intrinsics_are_returned_and_plausible(cloud, scene):
    c, s = cloud, scene
    assert c.intrinsics.shape == (3, 3)
    # MoGe returns normalised intrinsics; denormalised focal should be within
    # a factor of two of the true one. A wilder value means the FOV recovery
    # disagrees with the render and stage 2 should be believed instead.
    h, w = s.image.shape[:2]
    focal_px = c.intrinsics[0, 0] * w
    assert 0.5 < focal_px / s.K[0, 0] < 2.0, focal_px


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
