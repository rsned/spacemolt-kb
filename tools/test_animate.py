import os
import sys

import numpy as np

sys.path.insert(0, os.path.dirname(__file__))
import animate  # noqa: E402


def _synth(h=64, w=48):
    rng = np.random.default_rng(7)
    return rng.random((h, w, 3), dtype=np.float32)


def _loops(fn, imgs, params=None):
    """A frame fn loops if its t=0 and t=1 frames match within rounding."""
    params = params or {}
    f0 = fn(imgs, params, 0.0).astype(np.int16)
    f1 = fn(imgs, params, 1.0).astype(np.int16)
    return int(np.abs(f0 - f1).max())


def test_to_uint8_clamps_and_rounds():
    arr = np.array([[[-0.5, 0.0, 1.0]]], dtype=np.float32)
    out = animate.to_uint8(arr)
    assert out.dtype == np.uint8
    assert out.tolist() == [[[0, 0, 255]]]


def test_ensure_even_pads_odd_dims():
    odd = np.zeros((63, 47, 3), dtype=np.float32)
    even = animate.ensure_even(odd)
    h, w = even.shape[:2]
    assert h % 2 == 0 and w % 2 == 0
    assert (h, w) == (64, 48)


def test_remap_identity_returns_same_image():
    img = _synth()
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    out = animate.remap(img, xs, ys)
    assert np.allclose(out, img, atol=1e-4)


def test_remap_integer_shift():
    img = _synth()
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    # sample from one column to the right -> output shifted left by 1
    out = animate.remap(img, xs + 1.0, ys)
    assert np.allclose(out[:, :-1], img[:, 1:], atol=1e-4)


def test_fold_churn_shape_and_loop():
    img = _synth()
    frame = animate.fold_churn([img], {}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.fold_churn, [img]) <= 1


def test_chromatic_split_shape_and_loop():
    img = _synth()
    frame = animate.chromatic_split([img], {}, 0.5)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.chromatic_split, [img]) <= 1


def test_noise_dissolve_shape_and_loop():
    img = _synth()
    frame = animate.noise_dissolve([img], {"seed": 3}, 0.5)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.noise_dissolve, [img], {"seed": 3}) <= 1


def test_noise_dissolve_midpoint_differs_from_source():
    img = _synth()
    f_mid = animate.noise_dissolve([img], {"seed": 3, "max_noise": 0.85}, 0.5)
    f_start = animate.noise_dissolve([img], {"seed": 3}, 0.0)
    # at the dissolve peak the frame should differ substantially from the source
    assert np.abs(f_mid.astype(np.int16) - f_start.astype(np.int16)).mean() > 5
