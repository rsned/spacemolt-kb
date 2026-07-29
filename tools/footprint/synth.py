#!/usr/bin/env python3
"""Synthetic scenes with known geometry, camera and ground-truth footprint.

Every later pipeline stage is tested against these, so no stage's correctness
depends on trusting a hero image. Renders a magenta-backed 3/4 view matching
the hero art convention.

    from tools.footprint import synth
    scene = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
"""

import dataclasses

import cv2
import numpy as np

MAGENTA = (255, 0, 255)
IMAGE_SIZE = (900, 1200)  # (H, W), the hero art aspect
UP = np.array([0.0, 1.0, 0.0])


@dataclasses.dataclass
class Scene:
    image: np.ndarray
    K: np.ndarray
    R: np.ndarray
    t: np.ndarray
    points: np.ndarray
    footprint_xz: np.ndarray
    up: np.ndarray
    ortho: bool


def _look_at(azimuth_deg, elevation_deg, distance):
    """Camera rotation and translation for an orbit position around the origin."""
    az, el = np.radians(azimuth_deg), np.radians(elevation_deg)
    eye = distance * np.array([np.cos(el) * np.sin(az), np.sin(el), np.cos(el) * np.cos(az)])
    fwd = -eye / np.linalg.norm(eye)
    right = np.cross(fwd, UP)
    right /= np.linalg.norm(right)
    down = np.cross(fwd, right)
    R = np.stack([right, down, fwd])  # world -> camera, OpenCV axes
    return R, -R @ eye


def _box_mesh(length, width, height):
    hx, hy, hz = length / 2, height / 2, width / 2
    v = np.array([[sx * hx, sy * hy, sz * hz]
                  for sx in (-1, 1) for sy in (-1, 1) for sz in (-1, 1)], dtype=float)
    f = [(0, 1, 3, 2), (4, 6, 7, 5), (0, 4, 5, 1),
         (2, 3, 7, 6), (0, 2, 6, 4), (1, 5, 7, 3)]
    return v, f


def _cylinder_mesh(radius, height, segments=48):
    ang = np.linspace(0, 2 * np.pi, segments, endpoint=False)
    ring = np.stack([radius * np.cos(ang), np.zeros_like(ang), radius * np.sin(ang)], axis=1)
    bot, top = ring - [0, height / 2, 0], ring + [0, height / 2, 0]
    v = np.vstack([bot, top])
    f = [(i, (i + 1) % segments, segments + (i + 1) % segments, segments + i)
         for i in range(segments)]
    f.append(tuple(range(segments, 2 * segments)))
    f.append(tuple(reversed(range(segments))))
    return v, f


def _render(v, f, K, R, t, ortho):
    cam = v @ R.T + t
    if ortho:
        scale = K[0, 0] / abs(t[2])
        uv = cam[:, :2] * scale + np.array([K[0, 2], K[1, 2]])
    else:
        uv = (cam @ K.T)[:, :2] / cam[:, 2:3]
    img = np.full((*IMAGE_SIZE, 3), MAGENTA, dtype=np.uint8)
    order = sorted(range(len(f)), key=lambda i: -cam[list(f[i])][:, 2].mean())
    for rank, i in enumerate(order):
        poly = uv[list(f[i])].astype(np.int32)
        shade = 90 + int(120 * (rank + 1) / len(f))
        cv2.fillPoly(img, [poly], (shade, shade, shade))
    return img


def _surface_samples(v, f, per_face=400, seed=0):
    """Sample points across every face, standing in for a dense surface scan."""
    rng = np.random.default_rng(seed)
    out = []
    for face in f:
        p = v[list(face)]
        for _ in range(per_face):
            w = rng.random(len(p))
            out.append((p * (w / w.sum())[:, None]).sum(axis=0))
    return np.array(out)


def _scene(v, f, footprint_xz, azimuth_deg, elevation_deg, ortho):
    H, W = IMAGE_SIZE
    focal = 1400.0
    K = np.array([[focal, 0, W / 2], [0, focal, H / 2], [0, 0, 1]])
    radius = 4.0 * max(v.max(axis=0) - v.min(axis=0))
    R, t = _look_at(azimuth_deg, elevation_deg, radius)
    return Scene(image=_render(v, f, K, R, t, ortho), K=K, R=R, t=t,
                 points=_surface_samples(v, f), footprint_xz=footprint_xz,
                 up=UP.copy(), ortho=ortho)


def box_scene(length, width, height, azimuth_deg, elevation_deg, ortho=False):
    v, f = _box_mesh(length, width, height)
    hx, hz = length / 2, width / 2
    fp = np.array([[-hx, -hz], [hx, -hz], [hx, hz], [-hx, hz]])
    return _scene(v, f, fp, azimuth_deg, elevation_deg, ortho)


def cylinder_scene(radius, height, azimuth_deg, elevation_deg, ortho=False):
    v, f = _cylinder_mesh(radius, height)
    ang = np.linspace(0, 2 * np.pi, 128, endpoint=False)
    fp = np.stack([radius * np.cos(ang), radius * np.sin(ang)], axis=1)
    return _scene(v, f, fp, azimuth_deg, elevation_deg, ortho)
