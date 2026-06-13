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
