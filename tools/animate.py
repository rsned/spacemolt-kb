#!/usr/bin/env python3
"""animate — procedurally animate Voidborn keyframes into looping MP4 clips.

Pure-CPU numpy/Pillow compositing (no generative model). Each effect is a
function of a loop phase t in [0,1) so frame 0 and frame N match: the MP4
loops seamlessly with <video loop>. See
docs/superpowers/specs/2026-06-13-voidborn-procedural-animation-design.md.
"""
import numpy as np
from PIL import Image


def load_image(path):
    """Load an image as float32 RGB array in [0,1], shape (H, W, 3)."""
    img = Image.open(path).convert("RGB")
    return np.asarray(img, dtype=np.float32) / 255.0


def to_uint8(arr):
    """Clamp a float array to [0,1] and convert to uint8 with rounding."""
    return (np.clip(arr, 0.0, 1.0) * 255.0 + 0.5).astype(np.uint8)


def ensure_even(arr):
    """Pad bottom/right by one pixel (edge replicate) so H and W are even.

    yuv420p requires even dimensions; we pad ourselves and use
    macro_block_size=1 so imageio-ffmpeg never silently resizes.
    """
    h, w = arr.shape[:2]
    ph, pw = h % 2, w % 2
    if ph or pw:
        arr = np.pad(arr, ((0, ph), (0, pw), (0, 0)), mode="edge")
    return arr


def remap(img, map_x, map_y):
    """Bilinear-sample img at floating source coords (map_x, map_y).

    map_x/map_y are (H, W) arrays giving, for each output pixel, the source
    column/row to read. Coordinates are clamped to the image edges.
    """
    h, w = img.shape[:2]
    x = np.clip(map_x, 0, w - 1)
    y = np.clip(map_y, 0, h - 1)
    x0 = np.floor(x).astype(np.int64)
    y0 = np.floor(y).astype(np.int64)
    x1 = np.minimum(x0 + 1, w - 1)
    y1 = np.minimum(y0 + 1, h - 1)
    wx = (x - x0)[..., None]
    wy = (y - y0)[..., None]
    ia = img[y0, x0]
    ib = img[y0, x1]
    ic = img[y1, x0]
    idd = img[y1, x1]
    top = ia * (1 - wx) + ib * wx
    bot = ic * (1 - wx) + idd * wx
    return top * (1 - wy) + bot * wy


def fold_churn(imgs, params, t):
    """Animated sinusoidal displacement field — non-euclidean folds churning.

    The field phase advances by 2*pi*cycles over the loop; with integer
    `cycles` the t=0 and t=1 fields coincide, so the clip loops seamlessly.
    """
    img = imgs[0]
    amp = float(params.get("amp_px", 12.0))
    freq = float(params.get("freq", 2.0))
    cycles = float(params.get("cycles", 1.0))
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    phase = 2.0 * np.pi * t * cycles
    kx = 2.0 * np.pi * freq / w
    ky = 2.0 * np.pi * freq / h
    dx = amp * np.sin(ky * ys + phase)
    dy = amp * np.cos(kx * xs + phase)
    return to_uint8(remap(img, xs + dx, ys + dy))


def chromatic_split(imgs, params, t):
    """RGB channels drift apart off-axis then re-merge (unify -> split -> unify).

    Split distance d(t) = max_px * 0.5*(1-cos(2*pi*t)) goes 0 -> max -> 0, and a
    shared whole-frame off-axis wobble uses sin(2*pi*t); both vanish at t=0 and
    t=1, so the loop is seamless.
    """
    img = imgs[0]
    max_px = float(params.get("max_px", 18.0))
    angle = np.deg2rad(float(params.get("angle_deg", 20.0)))
    drift_px = float(params.get("drift_px", 6.0))
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    d = max_px * 0.5 * (1.0 - np.cos(2.0 * np.pi * t))
    drift = drift_px * np.sin(2.0 * np.pi * t)
    ox, oy = np.cos(angle), np.sin(angle)
    r = remap(img, xs + d * ox + drift * oy, ys + d * oy - drift * ox)[..., 0]
    g = remap(img, xs + drift * oy, ys - drift * ox)[..., 1]
    b = remap(img, xs - d * ox + drift * oy, ys - d * oy - drift * ox)[..., 2]
    return to_uint8(np.stack([r, g, b], axis=-1))
