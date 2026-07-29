"""Checks for the chroma-key matte.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_matte.py
"""

import importlib.util
import os
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


def test_matte_recovers_the_rendered_subject():
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, frac = matte.extract(s.image)

    assert mask.shape == s.image.shape[:2]
    assert set(np.unique(mask)).issubset({0, 1})
    assert 0.05 < frac < 0.85, frac
    # Corners are background in every hero image.
    for y, x in [(0, 0), (0, -1), (-1, 0), (-1, -1)]:
        assert mask[y, x] == 0


def test_matte_drops_disconnected_specks():
    # The speck must survive the 5x5 opening on its own merit, or this test
    # passes regardless of whether the largest-connected-component filter
    # exists: opening alone erodes anything narrower than the kernel to
    # nothing (verified empirically: a 4x4 speck is erased by opening alone;
    # a 6x6 one survives opening+closing intact and is only removed by the
    # CC filter). A 4x4 speck doesn't discriminate between the two
    # implementations; 6x6 does.
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    img = s.image.copy()
    img[5:11, 5:11] = (20, 20, 20)  # an isolated non-magenta speck
    mask, _ = matte.extract(img)
    assert mask[5:11, 5:11].sum() == 0


def test_matte_is_not_fooled_by_a_magenta_subject_pixel():
    # A block that matches the background exactly, but sits inside the hull,
    # must still be foreground: the mask is filled, not merely thresholded.
    #
    # The block must be a genuine hole, not a single pixel: the 5x5 closing
    # kernel in extract() bridges a 1px gap on its own, so a single corrupted
    # pixel passes whether or not the explicit flood-fill hole-fill exists —
    # it doesn't discriminate between the two implementations. A 7x7 block is
    # small enough to sit inside the subject but wide enough that closing
    # alone cannot bridge it (verified empirically: closing alone leaves a
    # >=5x5 hole unrecovered; only the flood-fill step recovers it).
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    ys, xs = np.nonzero(mask)
    cy, cx = int(ys.mean()), int(xs.mean())
    half = 3
    img = s.image.copy()
    img[cy - half:cy + half + 1, cx - half:cx + half + 1] = matte.background_color(s.image)
    mask2, _ = matte.extract(img)
    assert mask2[cy, cx] == 1
