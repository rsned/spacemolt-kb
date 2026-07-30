#!/usr/bin/env python3
"""Stage 3: recover a point map with MoGe-2.

MoGe-2 emits a metric-scale point map, a validity mask, per-pixel normals and
normalised intrinsics from a single image, so a point cloud comes out without
asserting a camera. Points outside the stage 1 matte are discarded: the
background is not part of the ship.

    ~/moge-venv/bin/python -m tools.footprint.pointmap <image>
"""

import dataclasses
import functools

import numpy as np
import torch
from moge.model.v2 import MoGeModel

from . import paths

MODEL_ID = "Ruicheng/moge-2-vitl-normal"
NEUTRAL = (128, 128, 128)


@dataclasses.dataclass
class Cloud:
    points: np.ndarray
    pixels: np.ndarray
    normals: np.ndarray | None
    intrinsics: np.ndarray


@functools.lru_cache(maxsize=1)
def _model():
    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    return MoGeModel.from_pretrained(MODEL_ID).to(device).eval(), device


def _composite(image_rgb, mask, background):
    """Replace the chroma key with a neutral field, or leave it alone."""
    if background == "raw":
        return image_rgb
    out = image_rgb.copy()
    out[mask == 0] = NEUTRAL
    return out


def infer(image_rgb: np.ndarray, mask: np.ndarray, background: str = "neutral") -> Cloud:
    model, device = _model()
    img = _composite(image_rgb, mask, background)
    t = torch.tensor(img / 255.0, dtype=torch.float32, device=device).permute(2, 0, 1)
    with torch.no_grad():
        out = model.infer(t)

    points = out["points"].cpu().numpy()
    # Intersect MoGe's own validity mask (it already excludes non-finite /
    # non-positive-depth pixels — see MoGeModel.infer) with the stage 1
    # matte: the background is never part of the ship even where MoGe
    # happily reconstructs geometry for it (e.g. a "raw" magenta backdrop it
    # reads as a flat wall).
    valid = out["mask"].cpu().numpy().astype(bool) & (mask > 0)
    normals = out["normal"].cpu().numpy() if "normal" in out else None

    ys, xs = np.nonzero(valid)
    return Cloud(points=points[ys, xs],
                 pixels=np.stack([xs, ys], axis=1).astype(np.float32),
                 normals=None if normals is None else normals[ys, xs],
                 intrinsics=out["intrinsics"].cpu().numpy())


def run(ship_id: str, image_rgb, mask, background: str = "neutral") -> Cloud:
    c = infer(image_rgb, mask, background)
    np.savez_compressed(paths.artifact_dir(ship_id) / "cloud.npz",
                        points=c.points, pixels=c.pixels,
                        normals=np.array([]) if c.normals is None else c.normals,
                        intrinsics=c.intrinsics)
    return c


def load(ship_id: str) -> Cloud:
    z = np.load(paths.artifact_dir(ship_id) / "cloud.npz")
    n = z["normals"]
    return Cloud(points=z["points"], pixels=z["pixels"],
                 normals=None if n.size == 0 else n, intrinsics=z["intrinsics"])
