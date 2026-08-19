#!/usr/bin/env python3
"""Estimate the octree resolution needed to capture a hero image's detail.

Marching cubes keeps a feature only if it spans ~2 voxels, so with the
object spanning S pixels and a structural element f pixels thick, the
needed grid is R = 2*S/f. The features octree resolution actually kills
are thin silhouette limbs (antennas, pylons, trusses) — interior surface
detail never becomes geometry anyway — so f is measured as medial-axis
limb thickness of the chroma-key alpha: p5 (the thinnest spars) and p25
(typical fine structure). Suggested R is reported for both.

Caveat printed with the numbers: past roughly R=512 the shape VAE's
latent, not the grid, is the detail ceiling — a finer grid then only
sharpens a smooth field.

    ~/sf3d-venv/bin/python estimate_octree.py <keyed.png|dir> [...]
"""

import sys
from pathlib import Path

import numpy as np
from PIL import Image
from skimage.morphology import medial_axis


def analyze(path: Path):
    img = np.asarray(Image.open(path).convert("RGBA"))
    a = img[..., 3] > 128
    if a.sum() < 500:
        # magenta-keyed file without alpha: key on colour distance
        rgb = img[..., :3].astype(float)
        a = np.abs(rgb - [255, 0, 255]).sum(axis=2) > 120
    ys, xs = np.nonzero(a)
    span = max(xs.max() - xs.min(), ys.max() - ys.min())
    skel, dist = medial_axis(a, return_distance=True)
    widths = 2 * dist[skel]                     # limb thickness in px
    widths = widths[widths > 1]                 # drop single-px noise
    p5, p25 = np.percentile(widths, [5, 25])
    return span, p5, p25


def main() -> int:
    paths = []
    for arg in sys.argv[1:]:
        p = Path(arg)
        paths += sorted(p.glob("*/keyed.png")) if p.is_dir() else [p]
    print(f"{'image':44} {'span':>5} {'p5':>5} {'p25':>5} {'R(p5)':>6} {'R(p25)':>6}")
    for p in paths:
        span, p5, p25 = analyze(p)
        r5, r25 = 2 * span / p5, 2 * span / p25
        name = p.parent.name if p.name == "keyed.png" else p.stem
        print(f"{name:44} {span:5d} {p5:5.1f} {p25:5.1f} {r5:6.0f} {r25:6.0f}")
    print("\nR = 2*span/width (feature needs ~2 voxels to survive). "
          "Above ~512 the shape VAE latent, not the grid, limits detail.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
