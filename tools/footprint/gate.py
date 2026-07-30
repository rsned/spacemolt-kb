#!/usr/bin/env python3
"""Stage 5: check the reconstruction against the view-plane silhouette.

The stage 1 matte is an exact silhouette, so reprojecting the resolved cloud
through the fitted camera and overlaying is a cheap, decisive consistency
check. A ship below the floor is a reconstruction failure: it is listed and
excluded from every aggregate, never quietly averaged in.

`reproject` also doubles as the inner-loop objective for the stage 4 plane
search (task 6b): that caller invokes it hundreds of times per ship, so it
does no I/O and no logging — only array math — and every call re-derives
nothing that isn't a function of its own arguments.

    ~/moge-venv/bin/python -m tools.footprint.gate <ship_id>
"""

import json

import cv2
import numpy as np

from . import paths

# Calibrated only against a perfect cloud (~0.99 IoU) and a grossly displaced
# one (~0.12), a wide gap with nothing in it -- so its position is otherwise
# arbitrary. Swept against a realistically dense (60k-sample) synthetic cloud
# (see test_gate.py) to find where IoU actually crosses 0.70 under three
# independent degradations: sideways rigid displacement crosses at dx~=0.545
# (box length 4.0, width 2.0 -- ~14% of length, ~27% of width); isotropic
# Gaussian point noise crosses at sigma~=0.253 (~17% of the box's longest
# dimension); random point deletion crosses at ~84% of points removed (16% of
# the cloud kept). So 0.70 admits a reconstruction that is either offset by
# roughly a quarter of the hull's own width, noised by roughly a sixth of its
# longest dimension, or missing all but a sixth of its points -- and rejects
# anything worse on any one of those axes. Not retuned to fit a result: this
# is the reported curve, not a chosen point on it.
IOU_FLOOR = 0.70

# Sensitivity measured on the same 60k-sample dense cloud: IoU is flat above
# dilate=3 (0.985 / 0.991 / 0.991 / 0.991 / 0.991 at 3 / 5 / 9 / 15 / 21) for a
# perfect cloud, and equally flat for a displaced one (0.123 at every kernel
# from 3 to 31) -- the verdict does not hinge on this exact constant once the
# cloud is dense enough to close into a solid region. It IS highly sensitive
# below that (0.547 at dilate=0/1) and on an under-dense cloud (see
# test_gate.py's docstring: 0.053/0.127/0.319/0.499 at 3/5/9/15 on the
# original 2400-point fixture) -- density, not the kernel, is what a future
# regression here would actually be catching.
_DILATE = 5


def reproject(points: np.ndarray, intrinsics: np.ndarray, shape) -> np.ndarray:
    """Project camera-space points through `intrinsics` into a (H,W) {0,1} mask."""
    h, w = shape
    K = intrinsics.copy()
    if K[0, 2] <= 2.0:  # MoGe returns intrinsics normalised to the unit square
        K[0, 0] *= w
        K[1, 1] *= h
        K[0, 2] *= w
        K[1, 2] *= h

    front = points[points[:, 2] > 1e-6]
    uv = (front @ K.T)[:, :2] / front[:, 2:3]
    uv = np.round(uv).astype(int)
    ok = (uv[:, 0] >= 0) & (uv[:, 0] < w) & (uv[:, 1] >= 0) & (uv[:, 1] < h)
    out = np.zeros((h, w), np.uint8)
    out[uv[ok, 1], uv[ok, 0]] = 1
    # The cloud is a point set; close it into a region before comparing areas.
    k = np.ones((_DILATE, _DILATE), np.uint8)
    return cv2.morphologyEx(out, cv2.MORPH_CLOSE, k)


def score(points: np.ndarray, intrinsics: np.ndarray, mask: np.ndarray) -> float:
    """IoU between the reprojected cloud and the stage 1 matte, in [0,1]."""
    pred = reproject(points, intrinsics, mask.shape).astype(bool)
    truth = mask.astype(bool)
    union = (pred | truth).sum()
    return float((pred & truth).sum() / union) if union else 0.0


def run(ship_id: str, points, intrinsics, mask) -> float:
    """Score the gate and record the verdict in quality.json."""
    iou = score(points, intrinsics, mask)
    p = paths.artifact_dir(ship_id) / "quality.json"
    data = json.loads(p.read_text()) if p.exists() else {}
    data["silhouette_iou"] = iou
    data["silhouette_pass"] = iou >= IOU_FLOOR
    p.write_text(json.dumps(data, indent=2))
    return iou
