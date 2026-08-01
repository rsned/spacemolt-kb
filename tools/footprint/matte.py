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
# The shadow-tolerant hue key. The art reserves magenta for the background
# (confirmed by the user on the full 402 drop: no ship carries magenta), so a
# pixel that keeps the key's hue and stays saturated is background even when a
# soft shadow or vignette has dropped its value -- exactly the case the flat
# RGB rules above reject (35/35 of the full drop's "unkeyable" images were
# shadowed/vignetted magenta, not scene renders; measured 2026-08-01).
# cv2 HSV hue is in half-degrees [0,180): the real keys measure 149-154, true
# scene renders 17-103, so (130,170) is a wide magenta-family prior.
KEY_HUE_RANGE = (130.0, 170.0)
HUE_TOLERANCE = 14.0
SAT_FLOOR = 60.0
# True scene renders top out at 0.86 border key-fraction (crowbar 0.47,
# paradox 0.69, principia 0.83); every real key -- flat or shadowed --
# measures 1.000.
BORDER_KEY_FRACTION_THRESHOLD = 0.98


def background_color(image_rgb: np.ndarray) -> np.ndarray:
    """Median of the four corner patches — the flat key colour."""
    k = 8
    patches = np.concatenate([
        image_rgb[:k, :k].reshape(-1, 3), image_rgb[:k, -k:].reshape(-1, 3),
        image_rgb[-k:, :k].reshape(-1, 3), image_rgb[-k:, -k:].reshape(-1, 3)])
    return np.median(patches, axis=0).astype(image_rgb.dtype)


def _key_hue_background(image_rgb: np.ndarray) -> tuple[np.ndarray, float]:
    """Per-pixel shadow-invariant background map + border key-fraction.

    A shadowed key keeps hue and saturation and loses only value, so hue
    distance to the border's median hue is the background test that survives
    shadow. The whole rule is gated on the key actually being magenta-family
    (KEY_HUE_RANGE): the hue prior is only safe because the art reserves
    magenta for the background — without it a uniformly-hued scene (a nebula,
    a lit cavern) could pass. Returns (is_background_map, border_key_fraction);
    both are all-False/0.0 when the border isn't magenta at all.
    """
    hsv = cv2.cvtColor(image_rgb, cv2.COLOR_RGB2HSV).astype(np.float32)
    border = np.concatenate([hsv[0], hsv[-1], hsv[:, 0], hsv[:, -1]], axis=0)
    key_hue = float(np.median(border[:, 0]))
    if not KEY_HUE_RANGE[0] <= key_hue <= KEY_HUE_RANGE[1]:
        return np.zeros(image_rgb.shape[:2], dtype=bool), 0.0
    dh = np.abs(hsv[..., 0] - key_hue)
    dh = np.minimum(dh, 180.0 - dh)  # circular hue distance
    is_bg = (dh <= HUE_TOLERANCE) & (hsv[..., 1] >= SAT_FLOOR)
    border_ok = np.concatenate([is_bg[0], is_bg[-1], is_bg[:, 0], is_bg[:, -1]])
    return is_bg, float(border_ok.mean())


def keyability(image_rgb: np.ndarray) -> tuple[bool, float, float, float]:
    """Report whether an image has a keyable background.

    Two routes in: a flat field (all four corner patches agree and the border
    has low variance) or a shadowed magenta key (the border is ≥98% key-hued
    even though shadow gradients push it past the flatness thresholds — see
    _key_hue_background). Environmental renders — a ship in a hall, a cavern,
    a hangar — fail both. They must be detected, because a colour-distance
    threshold still returns a plausible foreground fraction on them while
    actually segmenting scenery.

    Returns (is_keyable, corner_spread, border_std, border_key_fraction).
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

    _, border_key_fraction = _key_hue_background(image_rgb)
    flat = corner_spread < CORNER_SPREAD_THRESHOLD and border_std < BORDER_STD_THRESHOLD
    shadowed_key = border_key_fraction >= BORDER_KEY_FRACTION_THRESHOLD
    return flat or shadowed_key, corner_spread, border_std, border_key_fraction


def extract(image_rgb: np.ndarray, tolerance: float = TOLERANCE):
    """Return a (H,W) uint8 mask in {0,1} and the foreground fraction."""
    bg = background_color(image_rgb).astype(np.float32)
    dist = np.linalg.norm(image_rgb.astype(np.float32) - bg, axis=-1)
    # A pixel is background if it matches the corner colour in RGB (flat key)
    # OR reads as shadowed key by hue (dark-magenta shadows sit >TOLERANCE
    # from the bright corner colour, so RGB distance alone keeps them as
    # foreground and the shadow blob welds itself onto the hull silhouette).
    hue_bg, _ = _key_hue_background(image_rgb)
    mask = ((dist > tolerance) & ~hue_bg).astype(np.uint8)

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

    is_keyable, corner_spread, border_std, border_key_fraction = keyability(image_rgb)
    (art_dir / "keyability.json").write_text(json.dumps({
        "is_keyable": is_keyable,
        "corner_spread": corner_spread,
        "border_std": border_std,
        "border_key_fraction": border_key_fraction,
    }, indent=2))

    return frac, is_keyable
