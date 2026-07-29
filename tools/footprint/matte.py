#!/usr/bin/env python3
"""Stage 1: chroma-key the magenta hero background into a binary matte.

The hero backgrounds are a flat key, so a colour-distance threshold gives an
exact alpha without a segmentation model. The matte doubles as the view-plane
silhouette that stage 5 checks the reconstruction against.

    ~/moge-venv/bin/python -m tools.footprint.matte <image>
"""

import cv2
import numpy as np

from . import paths

TOLERANCE = 60.0
MIN_COMPONENT_FRACTION = 0.01


def background_color(image_rgb: np.ndarray) -> np.ndarray:
    """Median of the four corner patches — the flat key colour."""
    k = 8
    patches = np.concatenate([
        image_rgb[:k, :k].reshape(-1, 3), image_rgb[:k, -k:].reshape(-1, 3),
        image_rgb[-k:, :k].reshape(-1, 3), image_rgb[-k:, -k:].reshape(-1, 3)])
    return np.median(patches, axis=0).astype(image_rgb.dtype)


def extract(image_rgb: np.ndarray, tolerance: float = TOLERANCE):
    """Return a (H,W) uint8 mask in {0,1} and the foreground fraction."""
    bg = background_color(image_rgb).astype(np.float32)
    dist = np.linalg.norm(image_rgb.astype(np.float32) - bg, axis=-1)
    mask = (dist > tolerance).astype(np.uint8)

    kernel = np.ones((5, 5), np.uint8)
    mask = cv2.morphologyEx(mask, cv2.MORPH_OPEN, kernel)
    mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)

    # Keep only the largest connected component: hero art has one subject, and
    # stray specks would otherwise widen the footprint.
    n, labels, stats, _ = cv2.connectedComponentsWithStats(mask, connectivity=8)
    if n > 1:
        areas = stats[1:, cv2.CC_STAT_AREA]
        mask = (labels == 1 + int(np.argmax(areas))).astype(np.uint8)

    # Fill interior holes so subject pixels that happen to match the key colour
    # are not punched out of the middle of the hull.
    filled = mask.copy()
    h, w = mask.shape
    flood = np.zeros((h + 2, w + 2), np.uint8)
    cv2.floodFill(filled, flood, (0, 0), 1)
    mask = (mask | (1 - filled)).astype(np.uint8)

    return mask, float(mask.mean())


def run(ship_id: str, image_rgb: np.ndarray) -> float:
    mask, frac = extract(image_rgb)
    cv2.imwrite(str(paths.artifact_dir(ship_id) / "matte.png"), mask * 255)
    return frac
