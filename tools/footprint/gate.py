#!/usr/bin/env python3
"""Stage 5: check the reconstruction against the view-plane silhouette.

The stage 1 matte is an exact silhouette, so reprojecting the resolved cloud
through the fitted camera and overlaying is a cheap, decisive consistency
check. A ship below the floor is a reconstruction failure: it is listed and
excluded from every aggregate, never quietly averaged in.

`project` also doubles as the inner-loop primitive for the stage 4 plane
search (task 6b): that caller invokes it hundreds of times per ship, so it
does no I/O and no logging — only array math — and every call re-derives
nothing that isn't a function of its own arguments.

    ~/moge-venv/bin/python -m tools.footprint.gate <ship_id>
"""

import json

import cv2
import numpy as np

from . import paths

# DIAGNOSTIC ONLY, not the verdict — see `run()` and `MIR_FLOOR` below for
# what actually gates a ship now.
#
# This constant cannot separate a correctly-guessed mirrored half from a
# wrong one, for two reasons that are algebra, not measurement:
#
# 1. `pointmap.infer` keeps only the points already inside the stage 1 matte,
#    so the visible half's OWN reprojection lands back on the very pixels it
#    was read from. That alone drives union IoU to ~0.993 regardless of
#    whether the guessed symmetry plane is right, because
#    IoU = |truth| / (|truth| + |spill|): with |truth| ~= |pred| already from
#    the visible half alone, the 0.70 floor only fires once the MIRRORED half
#    spills more than 43% of the silhouette's own area on top of that — a lot
#    of wrongness to hide behind an already-satisfied visible half.
# 2. `uv = K.p / p_z` is exactly invariant under `p -> lambda(p)*p` for any
#    positive per-point lambda, so IoU cannot see ANY motion along the
#    viewing rays — including a global depth scale. Measured: a cloud
#    flattened to 10% of its depth extent scores 0.9910, identical to four
#    decimals to the unflattened cloud; a 5x global scale is likewise
#    invariant; the real `outerrim_prayer` cloud at 2x scale reads 0.9938,
#    the same as at 1x.
#
# Both predictions hold on real clouds: of 12 deliberately wrong symmetry
# planes tried against real ships, 9 pass this floor. The known-broken fold
# (Task 5's self-chamfer solver folding the visible sheet onto itself) scores
# 0.89-0.95; arbitrary axis-aligned normals score 0.65-0.81 — the "garbage"
# population straddles the floor instead of sitting under it, so no value of
# IOU_FLOOR separates correct from wrong. It is not a tuning problem.
#
# Random point deletion is also not a real error mode to calibrate against:
# the morphological close in `reproject` erases it (see `_CLOSE` below), so a
# deletion sweep measures density recovery, not geometry. The achievable
# failure is CONTIGUOUS loss — dropping the front of the hull along its long
# axis, as an occlusion or a bad crop would — and that DOES move this floor:
# 0.918 / 0.844 / 0.805 / 0.766 / 0.686 IoU at 10 / 20 / 25 / 30 / 40% of the
# hull's length removed. This floor admits a reconstruction missing roughly a
# third of the ship's own length.
#
# Kept as a recorded diagnostic (`silhouette_iou` in quality.json) because it
# is still informative about gross reprojection sanity — just not sufficient
# on its own, and no longer what `run()` gates on.
IOU_FLOOR = 0.70

# Sensitivity measured on a 60k-sample dense cloud: IoU is flat above
# kernel=3 (0.985 / 0.991 / 0.991 / 0.991 / 0.991 at kernel 3 / 5 / 9 / 15 /
# 21) for a perfect cloud, and equally flat for a displaced one (0.123 at
# every kernel from 3 to 31) — the verdict does not hinge on this exact
# constant once the cloud is dense enough to close into a solid region. It IS
# highly sensitive below that (0.547 at kernel=0/1) and on an under-dense
# cloud (see test_gate.py's docstring: 0.053/0.127/0.319/0.499 at kernel
# 3/5/9/15 on a 2400-point fixture) — so this constant becomes load-bearing
# again for any SUBSAMPLED use of `reproject`/`score` (e.g. an inner-loop
# search over a subsampled cloud), even though it is inert at full MoGe
# density.
_CLOSE = 5


def _finite_front(points: np.ndarray) -> np.ndarray:
    """Points that are finite and in front of the camera (z > 1e-6).

    `points[:, 2] > 1e-6` alone admits +inf, which MoGe writes at invalid
    pixels (see test_pointmap.py's `test_cloud_is_finite_and_in_front_of_the_
    camera`): +inf survives that test and produces nan in the projected uv,
    and `np.round(nan).astype(int)` is documented-undefined — safe here only
    because this platform saturates it to INT_MIN, which the bounds check
    then rejects. Filtering non-finite points out before the divide avoids
    relying on that platform accident (and silences two RuntimeWarnings from
    numpy dividing by / rounding inf and nan).
    """
    return np.isfinite(points).all(axis=1) & (points[:, 2] > 1e-6)


def project(points: np.ndarray, intrinsics: np.ndarray, shape) -> tuple[np.ndarray, np.ndarray]:
    """Project camera-space points through `intrinsics` into pixel coordinates.

    Returns `(uv, ok)`, both length N and aligned with `points`: `uv[i]` is
    point i's rounded (x, y) pixel — meaningful only where `ok[i]` is True.
    `ok` means finite, in front of the camera, and inside `shape`'s bounds; a
    point failing any of those is spill, not a silhouette hit, so it counts
    as outside everywhere a caller uses `ok`.

    `reproject` and `inside_fraction` both build on this, so there is exactly
    one copy of the unit-square-intrinsics sniff and the in-front/in-bounds
    rule.
    """
    h, w = shape
    K = intrinsics.copy().astype(float)
    if K[0, 2] <= 2.0:  # MoGe returns intrinsics normalised to the unit square
        # Scales four entries elementwise rather than scaling rows, so a K
        # with skew (K[0, 1] != 0) would denormalise incorrectly. MoGe's K
        # has no skew, so this is latent, not a live bug.
        K[0, 0] *= w
        K[1, 1] *= h
        K[0, 2] *= w
        K[1, 2] *= h

    n = len(points)
    uv = np.zeros((n, 2), dtype=int)
    ok = np.zeros(n, dtype=bool)

    front = _finite_front(points)
    if front.any():
        proj = (points[front] @ K.T)[:, :2] / points[front, 2:3]
        proj = np.round(proj).astype(int)
        uv[front] = proj
        in_bounds = (proj[:, 0] >= 0) & (proj[:, 0] < w) & (proj[:, 1] >= 0) & (proj[:, 1] < h)
        ok[front] = in_bounds
    return uv, ok


def reproject(points: np.ndarray, intrinsics: np.ndarray, shape) -> np.ndarray:
    """Project camera-space points through `intrinsics` into a (H,W) {0,1} mask."""
    h, w = shape
    uv, ok = project(points, intrinsics, shape)
    out = np.zeros((h, w), np.uint8)
    out[uv[ok, 1], uv[ok, 0]] = 1
    # The cloud is a point set; close it into a region before comparing areas.
    k = np.ones((_CLOSE, _CLOSE), np.uint8)
    return cv2.morphologyEx(out, cv2.MORPH_CLOSE, k)


def score(points: np.ndarray, intrinsics: np.ndarray, mask: np.ndarray) -> float:
    """IoU between the reprojected cloud and the stage 1 matte, in [0,1].

    DIAGNOSTIC ONLY — see the comment on `IOU_FLOOR` above for why this
    cannot be the verdict. `run()` gates on `mirrored_fraction` instead.
    """
    pred = reproject(points, intrinsics, mask.shape).astype(bool)
    truth = mask.astype(bool)
    union = (pred | truth).sum()
    return float((pred & truth).sum() / union) if union else 0.0


def inside_fraction(points: np.ndarray, intrinsics: np.ndarray, mask: np.ndarray) -> float:
    """Fraction of `points` whose projection lands inside the matte.

    Unlike `score`, there is NO morphological closing here, so this is
    density-invariant: subsampling the same cloud barely moves it, where
    `score` collapses under subsampling (measured in test_gate.py: `score`
    reads 0.20 on a 4000-point subsample of the same cloud that reads 0.991
    at 60k, while `inside_fraction` reads ~0.998 at both — a closed region
    needs a minimum point density to fill, an inside/outside count of
    individual points does not). That is why a subsampled IoU is comparable
    neither to `IOU_FLOOR` nor across candidate planes, while `inside_fraction`
    is — which is what the stage 4 plane
    search (task 6b) needs, since it evaluates hundreds of candidates per
    ship and cannot afford full cloud density each time.

    The denominator is the points in front of the camera (finite, z > 1e-6);
    a point that reprojects out of frame counts as OUTSIDE the numerator,
    because landing out of frame is spill just as much as landing on the
    wrong pixel is.
    """
    uv, ok = project(points, intrinsics, mask.shape)
    front = _finite_front(points)
    denom = int(front.sum())
    if denom == 0:
        return 0.0
    truth = mask.astype(bool)
    hits = np.zeros(len(points), dtype=bool)
    hits[ok] = truth[uv[ok, 1], uv[ok, 0]]
    return float(hits.sum() / denom)


# PROVISIONAL — do not treat this as tuned. `mirrored_fraction` is only
# meaningful once stage 4 returns a genuine symmetry plane rather than a
# fold, and stage 4 currently folds (see mirror.py's RESIDUAL_CEILING comment
# and the task 6b brief), so there is no population of correctly-solved real
# ships to calibrate a separator against yet.
#
# The one concrete real-cloud number that exists: a deliberately wrong plane
# (`outerrim_prayer` with `offset + 1`) reads `mirrored_fraction` 0.267 where
# the OLD union-IoU verdict passed it at 0.734 — exactly the blind spot this
# replaces. Across the broader solved-and-garbage sweep, `mirrored_fraction`
# spans 0.078-0.933, so that single known-bad point does not by itself locate
# a clean separator between solved and garbage. Set just above the one known-
# bad number pending task 6b's real calibration, which will have both actual
# solved clouds and the depth-separation term to draw the line properly.
MIR_FLOOR = 0.35


def run(ship_id: str, points, mirrored, intrinsics, mask) -> dict:
    """Score the gate and record diagnostics and verdict in quality.json.

    `mirrored_pass` is the verdict now — not `silhouette_iou` (see
    `IOU_FLOOR`). `silhouette_iou` is recorded over the full completed cloud
    (`points` + `mirrored`) purely as a diagnostic; `mirrored_fraction` is
    computed from `mirrored` ALONE, which is what actually carries
    information about whether the guessed symmetry plane is right — the
    visible half's own reprojection is uninformative by construction (see
    `IOU_FLOOR`).
    """
    mirrored = np.asarray(mirrored)
    combined = np.vstack([points, mirrored]) if len(mirrored) else np.asarray(points)
    iou = score(combined, intrinsics, mask)
    frac = inside_fraction(mirrored, intrinsics, mask) if len(mirrored) else 0.0
    result = {
        "silhouette_iou": iou,
        "mirrored_fraction": frac,
        "mirrored_pass": frac >= MIR_FLOOR,
    }
    p = paths.artifact_dir(ship_id) / "quality.json"
    data = json.loads(p.read_text()) if p.exists() else {}
    data.update(result)
    p.write_text(json.dumps(data, indent=2))
    return result
