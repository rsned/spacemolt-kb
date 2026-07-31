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


# PROVISIONAL, and re-derived TWICE on synthetic evidence already (fix
# rounds 2-3) before this, its third derivation: Task 6b has now produced
# real solved planes, which is exactly what those earlier derivations said
# was missing.
#
# `mirrored_fraction` ALONE cannot distinguish a correct symmetry plane from
# a fold: a fold reflects the visible sheet roughly onto itself, so it
# reprojects inside the matte just as well as the true plane does. Measured
# directly on a synthetic back-face-culled sheet, reflecting the same
# visible cloud across various candidate planes through its centroid:
#
#   TRUE bilateral plane                inside_fraction 0.998   (real number)
#   fold, normal along mean view dir     inside_fraction 0.750-0.92+
#   fold, random normals                 inside_fraction 0.35-0.95
#
# Independently reproduced in this codebase (see test_gate.py's fold
# fixtures): a plane whose normal is tilted only 15 degrees off the mean
# view direction reads inside_fraction 0.9156. (This specific candidate was
# later shown, by directly searching for the true degenerate optimum --
# see `MIN_DEPTH_SEPARATION_FRACTION` below -- to NOT be the tangential
# fold itself, just an intermediate wrong plane; the genuine tangential
# fold reads inside_fraction 0.5698, well below any reasonable floor. The
# 0.9156 number is kept here as a real, reproducible data point, not as a
# claim about what it is.) NO value of `MIR_FLOOR` sits above this
# synthetic fold band (up to 0.92+): this floor is NOT designed to exclude
# folds -- that is `depth_separation`'s job (see
# `MIN_DEPTH_SEPARATION_FRACTION` below), and `run()` requires both, even
# though in every case measured so far `MIR_FLOOR` alone already rejects
# every fold too (see that constant's comment for the honest accounting of
# why it is still kept regardless).
#
# Measured on FIVE REAL MoGe clouds (Task 6b's `solve_from_view` output),
# which is the new evidence this revision rests on:
#
#   solved planes (5 real ships)      mirrored_fraction 0.9890 - 0.9994
#   deliberately wrong planes         mirrored_fraction up to 0.9300
#                                      (`smelter` at offset+1), another at 0.89
#
# The two populations do not overlap, but the margin is THIN: 0.96 sits only
# ~1.03x above the worst wrong plane (0.96/0.9300) and ~1.03x below the
# weakest solved plane (0.9890/0.96) -- state that honestly rather than
# presenting 0.96 as well-separated. It also rests on a solver that is right
# on synthetic fixtures and NOT YET VERIFIED on real art: the recovered
# normals across those same five ships are still mutually inconsistent (see
# mirror.py). This is five real ships of evidence, not zero, but still five.
#
# This floor is one of THREE checks, and none is sufficient alone:
#
#   1. `MIR_FLOOR` (here) -- catches gross spill (a shifted cloud at 0.161,
#      an arbitrary random-normal plane at 0.35), a MODERATELY wrong real
#      plane the depth term happens not to catch (offset+1 at 0.93), and --
#      on every case measured so far -- the tangential fold too (0.57, see
#      `MIN_DEPTH_SEPARATION_FRACTION` below). Does NOT catch the
#      receded-plane degeneracy below (that clears fraction at 1.0000).
#   2. `depth_separation` / `MIN_DEPTH_SEPARATION_FRACTION` -- exists to
#      catch the tangential fold independently of fraction, but no measured
#      candidate to date is admitted by check #1 and only rejected by this
#      one; see its own comment for why it is defensive rather than
#      demonstrated. Does NOT catch the receded-plane degeneracy either --
#      a receded plane's depth separation is large and positive (it is
#      genuinely far from the visible surface, just in the wrong place),
#      not near zero, so it clears this threshold easily too.
#   3. A proximity ceiling in mirror.py (mean mirrored-to-visible distance
#      <= 0.10 of extent, NOT enforced here) -- catches the RECEDED-PLANE
#      degeneracy Task 6b found in the specified objective: a plane normal
#      to the view axis, placed behind the hull, recedes the reflection
#      until `inside_fraction` is EXACTLY 1.0000 -- ABOVE the true plane's
#      0.998 -- while clearing the depth floor by three orders of
#      magnitude. This candidate clears BOTH of this module's own checks;
#      no value of `MIR_FLOOR` rejects a score of 1.0, and no depth
#      threshold rejects a large, genuinely-positive separation. Only
#      `mirror.py`'s proximity ceiling catches it -- this module's gate is
#      not a complete defence by itself.
#
# Task 6b re-derives all three once more real solved clouds exist.
MIR_FLOOR = 0.96

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
# "N degrees off the mean view direction" (an earlier way of trying to
# construct a fold, see test_gate.py's `_one_sided_hull_and_fold`) names a
# 1-parameter FAMILY of planes, not one plane, and does not reliably locate
# the actual degenerate optimum: this module's own 15-degree case
# (depth_sep -0.0013, -0.04% of z-extent) turned out NOT to be the
# tangential fold -- just an intermediate wrong plane that happens to also
# have a small depth separation. The actual tangential fold was found by
# searching 800 random planes through a sheet's own centroid for the one
# that MINIMISES `|depth_separation|` (that minimum, not any fixed angle
# off the view direction, is the true degenerate optimum):
#
#   candidate                inside_fraction   depth_sep    sep/zext
#   TRUE bilateral plane           0.9988         0.27349      0.27349
#   TANGENTIAL fold (min of 800)   0.5698        -0.00007     -0.00007
#
# SAY THIS PLAINLY: `MIN_DEPTH_SEPARATION_FRACTION` is DEFENSIVE, not
# demonstrated. The tangential fold's own `inside_fraction` (0.5698) fails
# `MIR_FLOOR` (0.96) on fraction alone -- so does every other fold or wrong
# plane measured to date, real or synthetic (real wrong planes up to 0.93,
# this module's own 15-degree case at 0.9156). No measured candidate is
# currently admitted by `mirrored_fraction >= MIR_FLOOR` and THEN caught by
# this check -- there is no observed case this constant is the thing doing
# the rejecting for. It is kept because it is cheap, it is correct on the
# one case it was designed for (a ~4000x margin below this threshold on the
# sheet above), and the fold population sampled to date is small -- not
# because it has caught anything in practice. Nobody should later cite it
# as proven protection against a case that has actually been observed
# passing fraction and only fraction.
#
# It also does NOT catch the receded-plane degeneracy (see `MIR_FLOOR`'s
# comment): that candidate's depth separation is large and positive --
# genuinely far from the visible surface, just in the wrong place, not a
# fold -- so it clears this threshold easily too; only `mirror.py`'s
# proximity ceiling rejects it.
#
# Clearing this threshold does NOT mean "this plane is right", regardless
# of the above: a wrong-but-not-folded plane can have a LARGER depth
# separation than the true bilateral plane (a 30-degree-off-view-direction
# candidate on an earlier sweep: 0.335 of z-extent, vs. the true plane's
# 0.292) and would clear any fraction threshold comfortably. Only
# `inside_fraction`, maximised by the task 6b SEARCH (not this gate),
# identifies the correct plane -- this constant exists solely to exclude
# the one degenerate optimum inside_fraction cannot see, and must never be
# folded into a weighted sum with it: that would let a large depth
# separation buy a worse silhouette fit, which is exactly what the
# 30-degree candidate would do.
#
# 0.01 sits far below the tangential fold's ~0.00007 in relative terms
# while remaining a minimal, synthetic-derived check; task 6b re-derives it
# once more real solved clouds exist.
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
