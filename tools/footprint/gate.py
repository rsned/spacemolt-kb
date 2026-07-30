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


# PROVISIONAL, and re-derived once already — see fix round 2 in
# task-6-report.md. `mirrored_fraction` ALONE cannot distinguish a correct
# symmetry plane from a fold: a fold reflects the visible sheet roughly onto
# itself, so it reprojects inside the matte just as well as the true plane
# does. Measured directly on a synthetic back-face-culled sheet, reflecting
# the same visible cloud across various candidate planes through its
# centroid:
#
#   TRUE bilateral plane                inside_fraction 0.998   (real number)
#   fold, normal along mean view dir     inside_fraction 0.750-0.92+
#   fold, random normals                 inside_fraction 0.35-0.95
#
# Independently reproduced in this codebase (see test_gate.py's fold
# fixtures): a fold whose normal is tilted only 15 degrees off the mean view
# direction reads inside_fraction 0.9156 — ABOVE any floor that would still
# separate it from a genuine 0.998-0.999 true plane, so no value of
# MIR_FLOOR alone makes this floor sufficient on its own. That same fold
# reads depth_separation -0.0013 (essentially zero, i.e. the mirrored half
# sits at the SAME depth as the visible surface, the definition of a fold),
# where the true plane reads +0.179 to +0.18 (meaningfully behind it) --
# depth_separation is what actually discriminates, which is why `run()` now
# refuses to pass without one (see below; see MIN_DEPTH_SEPARATION_FRACTION's
# comment for why "15 degrees off the mean view direction" needs the exact
# rotation axis stated to be reproducible, and for a 600-random-plane search
# on a separate sheet that independently corroborates this specific fold as
# consistent with the tangential one -- not a proof that it IS the unique
# minimiser, just that it sits well inside the same degenerate band).
#
# NO value of `MIR_FLOOR` sits above the fold band: folds have been measured
# as high as 0.92+ (this module's own 15-degree case above: 0.9156), which
# is above 0.85 too. This floor is NOT what excludes folds -- that is
# `depth_separation`'s job (see `MIN_DEPTH_SEPARATION_FRACTION` below), and
# `run()` requires both. What this floor actually catches is GROSS spill: a
# plane so wrong its reprojection barely lands on the silhouette at all,
# e.g. a shifted cloud at 0.161 or an arbitrary random-normal plane at 0.35.
# It is synthetic-derived, not measured on real solved clouds (there are
# none yet -- stage 4 currently folds), and Task 6b re-derives it once real
# solved planes exist.
MIR_FLOOR = 0.85

# `depth_separation` is task 6b's discriminating term (mean of mirrored-z
# minus visible-z at shared reprojected pixels), computed by the caller
# (`mirror.solve_from_view`), not by this module -- `run()` only checks it,
# against a FRACTION of the cloud's own z-extent, since ships range from
# fighters to haulers and an absolute distance means nothing across that
# range (the same reason `mirror.RESIDUAL_CEILING` is a fraction, not an
# absolute distance).
#
# This threshold is owned HERE, not in mirror.py, because of the import
# direction: `gate` imports only `paths`, and task 6b makes `mirror` import
# `gate` (`solve_from_view` calls `gate.reproject`). A threshold owned by
# `mirror` but needed by `gate.run` would be an import cycle -- one owner,
# imported downhill, so `run()` takes `z_extent` as a parameter rather than
# computing it (it has no access to the full cloud, only `mirrored`).
#
# 0.01 is provisional, and -- like `MIR_FLOOR` -- only excludes the
# degenerate near-zero-separation fold; it does NOT mean "this plane is
# right". A wrong-but-not-folded plane can have a LARGER depth separation
# than the true bilateral plane (confirmed independently: see below), so
# nothing about clearing this threshold ranks candidates -- maximising
# `inside_fraction` is what the task 6b SEARCH does for that; this gate only
# rejects the one specific failure mode inside_fraction cannot see.
#
# Two independent sweeps of "N degrees off the mean view direction" (this
# module's own back-face-culled box, see test_gate.py's
# `_one_sided_hull_and_fold`; a separate hand-built sheet) disagreed at the
# same nominal angle -- because that phrase names a 1-parameter FAMILY of
# planes, not one plane: the rotation axis was a free parameter neither
# sweep pinned down. Resolved by searching for the actual degenerate
# optimum directly (600 random planes through the visible cloud's centroid,
# keeping the one that MINIMISES |depth_separation| -- that minimum, not any
# fixed angle off the view direction, is what a self-consistency objective
# actually converges to):
#
#   candidate                            depth_sep    sep/zext   shared px
#   TRUE bilateral plane                   0.3531      0.29210      5603
#   TANGENTIAL fold (min |sep| of 600)    -0.0000     -0.00002      3022
#
# The tangential fold is smaller in |depth_separation| than the true plane
# by a factor of ~12315x -- decisive, and 3022 shared pixels rules out a
# small-sample artifact. This module's own -0.0013 (-0.04% of z-extent) at
# its 15-degree case is consistent with (and independently corroborates)
# this tangential fold, not a wrong-but-arbitrary rotated plane -- the two
# sweeps' OTHER angles (0/20/30 degrees on this module's sweep, 0/15/20/30
# on the other) were never folds at all, just ordinary wrong planes, which
# is why they read meaningfully positive (+0.85% to +3.73% here; +9.12% to
# +33.52% on the other sweep) rather than near zero. 0.01 sits comfortably
# between the tangential fold's ~0.00002 and the true plane's ~0.29 (and
# above this module's own -0.0004, used by test_gate.py's fold fixture),
# clearing the actual degenerate case by orders of magnitude while staying a
# minimal, synthetic-derived check; task 6b re-derives it once real solved
# clouds exist.
#
# What clearing this threshold does NOT mean: "this plane is right". A
# wrong-but-not-folded plane can have a LARGER depth separation than the
# true bilateral plane (the 30-degree candidate on the other sweep: 0.335 of
# z-extent, vs the true plane's 0.292) and would clear any fraction
# threshold comfortably. Only `inside_fraction`, maximised by the task 6b
# SEARCH (not this gate), identifies the correct plane -- this constant
# exists solely to exclude the one degenerate optimum inside_fraction cannot
# see, and must never be folded into a weighted sum with it: that would let
# a large depth separation buy a worse silhouette fit, which is exactly what
# the 30-degree candidate would do.
MIN_DEPTH_SEPARATION_FRACTION = 0.01


def run(ship_id: str, points, mirrored, intrinsics, mask,
        depth_separation: float | None = None, z_extent: float | None = None) -> dict:
    """Score the gate and record diagnostics and verdict in quality.json.

    `mirrored_pass` is the verdict now — not `silhouette_iou` (see
    `IOU_FLOOR`). `silhouette_iou` is recorded over the full completed cloud
    (`points` + `mirrored`) purely as a diagnostic; `mirrored_fraction` is
    computed from `mirrored` ALONE, which is what actually carries
    information about whether the guessed symmetry plane is right — the
    visible half's own reprojection is uninformative by construction (see
    `IOU_FLOOR`).

    `mirrored_fraction` alone cannot tell a correct plane from a fold (see
    `MIR_FLOOR`), so `mirrored_pass` requires BOTH `mirrored_fraction` above
    `MIR_FLOOR` AND a `depth_separation` that clears
    `MIN_DEPTH_SEPARATION_FRACTION * z_extent`. `depth_separation` and
    `z_extent` default to `None` because neither exists until task 6b's
    `solve_from_view` computes them -- an ABSENT depth term (or an absent
    `z_extent` to evaluate it against) is an unevaluated gate, and an
    unevaluated gate must never read as a pass, so `mirrored_pass` is
    `False` in either case, with `mirrored_pass_reason` recording why.
    """
    mirrored = np.asarray(mirrored)
    combined = np.vstack([points, mirrored]) if len(mirrored) else np.asarray(points)
    iou = score(combined, intrinsics, mask)
    frac = inside_fraction(mirrored, intrinsics, mask) if len(mirrored) else 0.0

    if depth_separation is None:
        mirrored_pass = False
        reason = ("depth_separation not supplied — mirrored_fraction alone cannot "
                   "distinguish a correct symmetry plane from a fold (see MIR_FLOOR); "
                   "an unevaluated depth term must not read as a pass")
    elif z_extent is None:
        mirrored_pass = False
        reason = ("z_extent not supplied — depth_separation cannot be evaluated against "
                   "MIN_DEPTH_SEPARATION_FRACTION without it; an unevaluated depth term "
                   "must not read as a pass")
    elif depth_separation < MIN_DEPTH_SEPARATION_FRACTION * z_extent:
        mirrored_pass = False
        reason = (f"depth_separation={depth_separation:.4f} is below "
                   f"MIN_DEPTH_SEPARATION_FRACTION * z_extent="
                   f"{MIN_DEPTH_SEPARATION_FRACTION * z_extent:.4f} — the mirrored half "
                   "is not meaningfully behind the visible surface, which is what a "
                   "fold looks like")
    elif frac < MIR_FLOOR:
        mirrored_pass = False
        reason = f"mirrored_fraction={frac:.4f} is below MIR_FLOOR={MIR_FLOOR}"
    else:
        mirrored_pass = True
        reason = None

    result = {
        "silhouette_iou": iou,
        "mirrored_fraction": frac,
        "mirrored_pass": mirrored_pass,
    }
    if depth_separation is not None:
        result["depth_separation"] = depth_separation
    if reason is not None:
        result["mirrored_pass_reason"] = reason

    p = paths.artifact_dir(ship_id) / "quality.json"
    data = json.loads(p.read_text()) if p.exists() else {}
    data.update(result)
    p.write_text(json.dumps(data, indent=2))
    return result
