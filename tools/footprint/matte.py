#!/usr/bin/env python3
"""Stage 1: chroma-key the magenta hero background into a binary matte.

The hero backgrounds are a flat key, so a colour-distance threshold gives an
exact alpha without a segmentation model. The matte doubles as the view-plane
silhouette that stage 5 checks the reconstruction against.

    ~/moge-venv/bin/python -m tools.footprint.matte <image>
"""

import json

import cv2
import numpy as np

from . import paths

TOLERANCE = 60.0
MIN_COMPONENT_FRACTION = 0.01
CORNER_SPREAD_THRESHOLD = 10.0
BORDER_STD_THRESHOLD = 10.0


def background_color(image_rgb: np.ndarray) -> np.ndarray:
    """Median of the four corner patches — the flat key colour."""
    k = 8
    patches = np.concatenate([
        image_rgb[:k, :k].reshape(-1, 3), image_rgb[:k, -k:].reshape(-1, 3),
        image_rgb[-k:, :k].reshape(-1, 3), image_rgb[-k:, -k:].reshape(-1, 3)])
    return np.median(patches, axis=0).astype(image_rgb.dtype)


def keyability(image_rgb: np.ndarray) -> tuple[bool, float, float]:
    """Report whether an image has a flat, keyable background.

    A chroma-keyed subject sits on a uniform field: all four corner patches
    agree with each other and the border has low variance. Environmental
    renders — a ship in a hall, a cavern, a hangar — fail both. They must be
    detected, because a colour-distance threshold still returns a plausible
    foreground fraction on them while actually segmenting scenery.

    Returns (is_keyable, corner_spread, border_std).
    """
    k = 24
    patches = [image_rgb[:k, :k], image_rgb[:k, -k:], image_rgb[-k:, :k], image_rgb[-k:, -k:]]
    corner_means = np.array([p.reshape(-1, 3).mean(axis=0) for p in patches], dtype=np.float64)
    overall = corner_means.mean(axis=0)
    corner_spread = float(np.linalg.norm(corner_means - overall, axis=1).max())

    top, bottom = image_rgb[0, :, :], image_rgb[-1, :, :]
    left, right = image_rgb[:, 0, :], image_rgb[:, -1, :]
    border = np.concatenate([top, bottom, left, right], axis=0).astype(np.float64)
    border_std = float(border.std(axis=0).mean())

    is_keyable = corner_spread < CORNER_SPREAD_THRESHOLD and border_std < BORDER_STD_THRESHOLD
    return is_keyable, corner_spread, border_std


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


def run(ship_id: str, image_rgb: np.ndarray) -> tuple[float, bool]:
    """Write matte.png and a keyability record; return (frac, is_keyable).

    extract() still runs and matte.png still gets written even when the
    background isn't flat — stage 1 doesn't refuse to produce a matte. The
    keyability verdict is recorded alongside it as information for whatever
    consumes these artifacts (the batch driver) to act on, not enforced here.
    """
    mask, frac = extract(image_rgb)
    art_dir = paths.artifact_dir(ship_id)
    cv2.imwrite(str(art_dir / "matte.png"), mask * 255)

    is_keyable, corner_spread, border_std = keyability(image_rgb)
    (art_dir / "keyability.json").write_text(json.dumps({
        "is_keyable": is_keyable,
        "corner_spread": corner_spread,
        "border_std": border_std,
    }, indent=2))

    return frac, is_keyable
