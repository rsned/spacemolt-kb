#!/usr/bin/env python3
"""Quick shaded previews of a generated mesh (no GL, no display).

Surface-samples the mesh and splats the points through a z-buffer with
Lambert shading. Point splatting rather than triangle rasterisation keeps
this vectorised in numpy -- fast enough to eyeball a batch, and accurate
enough to answer the only question being asked of it: does the mesh look
like the ship, and what does its top-down silhouette actually look like.

    ~/hy3d-venv/bin/python render_mesh.py <mesh.obj> <out_prefix>
"""

import sys
from pathlib import Path

import numpy as np
import trimesh
from PIL import Image

RES = 700
N_SAMPLES = 900_000
BG = 32


def look_at(elev_deg: float, azim_deg: float) -> np.ndarray:
    """World->camera rotation for an orthographic view."""
    e, a = np.radians(elev_deg), np.radians(azim_deg)
    ca, sa, ce, se = np.cos(a), np.sin(a), np.cos(e), np.sin(e)
    fwd = np.array([ce * sa, se, ce * ca])
    up_w = np.array([0.0, 1.0, 0.0])
    right = np.cross(up_w, fwd)
    if np.linalg.norm(right) < 1e-6:  # looking straight down: pick any right
        right = np.array([1.0, 0.0, 0.0])
    right /= np.linalg.norm(right)
    up = np.cross(fwd, right)
    return np.stack([right, up, fwd])


def render(pts: np.ndarray, nrm: np.ndarray, R: np.ndarray) -> Image.Image:
    cam = pts @ R.T
    nc = nrm @ R.T

    xy = cam[:, :2]
    lo, hi = xy.min(axis=0), xy.max(axis=0)
    scale = (RES * 0.88) / max(hi - lo)
    px = ((xy - (lo + hi) / 2) * scale + RES / 2).astype(np.int32)
    px[:, 1] = RES - 1 - px[:, 1]
    np.clip(px, 0, RES - 1, out=px)

    depth = cam[:, 2]
    flat = px[:, 1] * RES + px[:, 0]

    # z-buffer: nearest sample wins each pixel
    order = np.argsort(-depth)  # far -> near, so nearest overwrites last
    buf_shade = np.full(RES * RES, -1.0)
    lightdir = np.array([0.35, 0.55, -0.75])
    lightdir /= np.linalg.norm(lightdir)
    lam = np.clip(nc @ lightdir, 0, 1)
    shade = 40 + 205 * (0.25 + 0.75 * lam)
    buf_shade[flat[order]] = shade[order]

    img = np.where(buf_shade < 0, BG, buf_shade).reshape(RES, RES)
    return Image.fromarray(img.astype(np.uint8), mode="L")


def main() -> int:
    mesh_path, prefix = sys.argv[1], sys.argv[2]
    mesh = trimesh.load(mesh_path, force="mesh")
    pts, face_idx = trimesh.sample.sample_surface(mesh, N_SAMPLES)
    nrm = mesh.face_normals[face_idx]

    pts = np.asarray(pts) - np.asarray(pts).mean(axis=0)

    views = {"threequarter": (18, 35), "side": (0, 90), "topdown": (89.9, 0)}
    for name, (elev, azim) in views.items():
        out = f"{prefix}_{name}.png"
        render(pts, nrm, look_at(elev, azim)).save(out)
        print(f"  wrote {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
