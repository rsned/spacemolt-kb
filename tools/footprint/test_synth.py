"""Ground-truth checks for the synthetic scene generator.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_synth.py
"""

import importlib
import pathlib
import sys

import numpy as np

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
synth = importlib.import_module("tools.footprint.synth")


def test_box_scene_footprint_is_a_rectangle_of_the_right_aspect():
    s = synth.box_scene(length=4.0, width=2.0, height=1.5,
                        azimuth_deg=35.0, elevation_deg=30.0)
    fp = s.footprint_xz
    extent_x = fp[:, 0].max() - fp[:, 0].min()
    extent_z = fp[:, 1].max() - fp[:, 1].min()
    assert abs(extent_x - 4.0) < 1e-6, extent_x
    assert abs(extent_z - 2.0) < 1e-6, extent_z


def test_box_scene_renders_a_subject_on_magenta():
    s = synth.box_scene(length=4.0, width=2.0, height=1.5,
                        azimuth_deg=35.0, elevation_deg=30.0)
    assert s.image.shape[2] == 3
    corner = s.image[0, 0]
    assert corner[0] > 200 and corner[2] > 200 and corner[1] < 60, corner
    # The subject must actually cover a sensible slice of frame.
    bg = np.all(np.abs(s.image.astype(int) - corner.astype(int)) < 30, axis=-1)
    covered = 1.0 - bg.mean()
    assert 0.05 < covered < 0.85, covered


def test_cylinder_scene_footprint_is_circular():
    s = synth.cylinder_scene(radius=1.0, height=3.0,
                             azimuth_deg=20.0, elevation_deg=35.0)
    fp = s.footprint_xz
    r = np.linalg.norm(fp - fp.mean(axis=0), axis=1)
    assert abs(r.mean() - 1.0) < 0.02, r.mean()
    assert r.std() < 0.02, r.std()
