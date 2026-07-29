"""Ground-truth checks for the synthetic scene generator.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_synth.py
"""

import importlib
import pathlib
import sys

import numpy as np

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
synth = importlib.import_module("tools.footprint.synth")


def _ground_project(vertices, up):
    """Project mesh vertices onto the ground plane, in the same 2D basis
    footprint_xz is expressed in (an arbitrary but consistent (e1, e2) pair
    perpendicular to `up`)."""
    up = up / np.linalg.norm(up)
    v = vertices
    ground = v - np.outer(v @ up, up)

    a = np.array([1.0, 0.0, 0.0])
    if abs(a @ up) > 0.9:
        a = np.array([0.0, 0.0, 1.0])
    e1 = a - (a @ up) * up
    e1 /= np.linalg.norm(e1)
    e2 = np.cross(up, e1)
    return np.stack([ground @ e1, ground @ e2], axis=1)


def test_footprint_matches_the_ground_projection_of_the_rendered_mesh():
    """The claimed footprint must agree with the mesh the renderer draws.

    footprint_xz is built by its own formula in box_scene/cylinder_scene, so
    without this test nothing notices if the mesh and that formula drift apart.
    Every later pipeline stage is validated against footprint_xz; a silent
    desync here would make all of them pass against a wrong answer.
    """
    for name, scene, tol in (
            ("box", synth.box_scene(4.0, 2.0, 1.5, 35, 30), 1e-9),
            ("cylinder", synth.cylinder_scene(1.0, 3.0, 20, 35), 1e-9)):
        xy = _ground_project(scene.vertices, scene.up)

        fp = scene.footprint_xz
        for axis in (0, 1):
            got = xy[:, axis].max() - xy[:, axis].min()
            want = fp[:, axis].max() - fp[:, axis].min()
            assert abs(got - want) < tol, (
                f"{name} axis {axis}: mesh projects to {got:.6f} but "
                f"footprint_xz claims {want:.6f}")


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

    # r.std() < 0.02 alone can't fail here: footprint_xz is built as
    # radius*cos(ang), radius*sin(ang), so its own std is ~1e-16 by
    # construction regardless of what the mesh actually looks like. Check
    # the footprint's radius against the mesh's ground-projected radius
    # instead, so a desync between the two is actually catchable.
    mesh_xy = _ground_project(s.vertices, s.up)
    mesh_r = np.linalg.norm(mesh_xy - mesh_xy.mean(axis=0), axis=1)
    assert abs(mesh_r.mean() - r.mean()) < 1e-6, (mesh_r.mean(), r.mean())
