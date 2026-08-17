#!/usr/bin/env python3
"""Colour a ship's sampled point cloud from its hero image.

No texture model involved: find the hero camera by silhouette match, then
project. The generator emits meshes in a canonical frame roughly aligned
with the conditioning image, so an orthographic camera search over
azimuth/elevation (roll assumed ~0 for upright hero art), scored by
silhouette IoU against the chroma-key alpha, lands on the hero viewpoint.
Points visible from that camera sample their pixel; occluded points take
the colour of their nearest visible neighbour -- which leans on ships being
roughly symmetric and consistently painted, and is exactly the kind of
plausible guess the whole mesh already is on its hidden side.

    ~/hy3d-venv/bin/python colorize_cloud.py <sweep_dir> <stem> <out.js> [n_points]

Writes a JS data file: N_PTS, CLOUD_B64 (int16 xyz ×3N, int8 normal ×3N,
uint8 rgb ×3N) and CAM (the fitted view, so a page can *start* from the
hero's own angle).
"""

import base64
import sys
from pathlib import Path

import numpy as np
import trimesh
from PIL import Image
from scipy.spatial import cKDTree

N_SAMPLES = 25000
FIT_RES = 128
VIS_RES = 512


def look(elev, azim):
    e, a = np.radians(elev), np.radians(azim)
    fwd = np.array([np.cos(e) * np.sin(a), np.sin(e), np.cos(e) * np.cos(a)])
    right = np.cross([0.0, 1.0, 0.0], fwd)
    n = np.linalg.norm(right)
    right = np.array([1.0, 0.0, 0.0]) if n < 1e-6 else right / n
    return right, np.cross(fwd, right), fwd


def occupancy(xy, res):
    """Rasterise projected points into a res×res boolean grid, normalised so
    the max bbox dimension fills ~90% of the grid (uniform scale, centred)."""
    lo, hi = xy.min(axis=0), xy.max(axis=0)
    s = (res * 0.9) / max((hi - lo).max(), 1e-9)
    px = ((xy - (lo + hi) / 2) * s + res / 2).astype(int)
    np.clip(px, 0, res - 1, out=px)
    g = np.zeros((res, res), bool)
    g[px[:, 1], px[:, 0]] = True
    # cheap dilation to close sampling gaps
    for sh in ((1, 0), (-1, 0), (0, 1), (0, -1)):
        g |= np.roll(g, sh, axis=(0, 1))
    return g


def mask_grid(alpha, res):
    im = Image.fromarray((alpha * 255).astype(np.uint8))
    # same normalisation as occupancy(): crop to content, fit max dim to 90%
    ys, xs = np.nonzero(alpha > 0.5)
    im = im.crop((xs.min(), ys.min(), xs.max() + 1, ys.max() + 1))
    w, h = im.size
    s = (res * 0.9) / max(w, h)
    im = im.resize((max(1, int(w * s)), max(1, int(h * s))))
    g = np.zeros((res, res), bool)
    a = np.asarray(im) > 128
    y0, x0 = (res - a.shape[0]) // 2, (res - a.shape[1]) // 2
    g[y0:y0 + a.shape[0], x0:x0 + a.shape[1]] = a
    return g


def iou(a, b):
    return (a & b).sum() / max((a | b).sum(), 1)


def fit_camera(pts, mask):
    m = mask_grid(mask, FIT_RES)

    def score(elev, azim):
        r, u, f = look(elev, azim)
        xy = np.column_stack([pts @ r, -(pts @ u)])  # image y grows downward
        return iou(occupancy(xy, FIT_RES), m)

    best = (-1.0, 0.0, 0.0)
    for elev in range(-40, 61, 10):
        for azim in range(-180, 180, 10):
            s = score(elev, azim)
            if s > best[0]:
                best = (s, elev, azim)
    # refine
    for step in (5, 2):
        s0, e0, a0 = best
        for elev in range(int(e0) - step * 2, int(e0) + step * 2 + 1, step):
            for azim in range(int(a0) - step * 2, int(a0) + step * 2 + 1, step):
                s = score(elev, azim)
                if s > best[0]:
                    best = (s, elev, azim)
    return best


def main() -> int:
    sweep, stem, out_js = Path(sys.argv[1]), sys.argv[2], Path(sys.argv[3])
    n_samples = int(sys.argv[4]) if len(sys.argv) > 4 else N_SAMPLES
    d = sweep / stem

    mesh = trimesh.load(d / "mesh.obj", force="mesh")
    pts, fi = trimesh.sample.sample_surface(mesh, n_samples)
    nrm = mesh.face_normals[fi]
    pts = np.asarray(pts) - np.asarray(pts).mean(axis=0)

    img = np.asarray(Image.open(d / "keyed.png").convert("RGBA")).astype(float)
    alpha = img[..., 3] / 255.0

    score, elev, azim = fit_camera(pts, alpha)
    print(f"{stem}: hero camera elev {elev} azim {azim}  (IoU {score:.3f})")

    # project at the fitted view; map points into the hero image's content box
    r, u, f = look(elev, azim)
    xy = np.column_stack([pts @ r, -(pts @ u)])
    depth = pts @ f
    ys, xs = np.nonzero(alpha > 0.5)
    ix0, ix1, iy0, iy1 = xs.min(), xs.max(), ys.min(), ys.max()
    lo, hi = xy.min(axis=0), xy.max(axis=0)
    span = hi - lo
    s = min((ix1 - ix0) / max(span[0], 1e-9), (iy1 - iy0) / max(span[1], 1e-9))
    px = (xy - (lo + hi) / 2) * s + [(ix0 + ix1) / 2, (iy0 + iy1) / 2]
    pxi = np.clip(px.astype(int), [0, 0], [img.shape[1] - 1, img.shape[0] - 1])

    # visibility via a coarse z-buffer over projected pixels
    gx = (px[:, 0] * VIS_RES / img.shape[1]).astype(int).clip(0, VIS_RES - 1)
    gy = (px[:, 1] * VIS_RES / img.shape[0]).astype(int).clip(0, VIS_RES - 1)
    cell = gy * VIS_RES + gx
    zbuf = np.full(VIS_RES * VIS_RES, -np.inf)
    np.maximum.at(zbuf, cell, depth)
    eps = 0.03 * (depth.max() - depth.min())
    visible = depth >= zbuf[cell] - eps
    # a "visible" point projecting onto transparent background saw nothing
    visible &= alpha[pxi[:, 1], pxi[:, 0]] > 0.5

    rgb = np.zeros((len(pts), 3))
    rgb[visible] = img[pxi[visible, 1], pxi[visible, 0], :3]
    if visible.any() and (~visible).any():
        tree = cKDTree(pts[visible])
        _, nn = tree.query(pts[~visible])
        rgb[~visible] = rgb[visible][nn]
    print(f"  visible {visible.mean() * 100:.0f}%, borrowed {(~visible).mean() * 100:.0f}%")

    scale = 32000.0 / np.abs(pts).max()
    q = np.clip(pts * scale, -32767, 32767).astype("<i2")
    n8 = np.clip(nrm * 127, -127, 127).astype("i1")
    c8 = rgb.clip(0, 255).astype(np.uint8)
    b64 = base64.b64encode(q.tobytes() + n8.tobytes() + c8.tobytes()).decode()
    out_js.write_text(
        f'const N_PTS={len(q)};const CAM={{elev:{elev},azim:{azim}}};'
        f'const CLOUD_B64="{b64}";')
    print(f"  wrote {out_js} ({out_js.stat().st_size // 1024} KB)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
