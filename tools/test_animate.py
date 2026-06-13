import os
import sys

import numpy as np

sys.path.insert(0, os.path.dirname(__file__))
import animate  # noqa: E402


def _synth(h=64, w=48):
    rng = np.random.default_rng(7)
    return rng.random((h, w, 3), dtype=np.float32)


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
