#!/usr/bin/env python3
"""Stage 2: fit the camera from vanishing points.

These hulls are Manhattan-world objects with long parallel structural lines, so
three orthogonal vanishing points give rotation and focal length outright and
reveal whether a render is orthographic or mildly perspective. Never assume the
3/4 view: a low-confidence fit routes the ship to the hand-clicked fallback.

Implemented directly on cv2's line segment detector. lu-vp-detect is not used:
it breaks on OpenCV 5.x (it strips a singleton dimension LSD no longer emits)
and never exposed the cluster memberships a confidence measure needs.

The RANSAC search objective and the reported confidence are deliberately two
different numbers (see `_search_score` vs. `_confidence`): a search score that
penalises "too many" inliers on one axis converges on wrong-but-balanced
hypotheses instead of the true camera, and a gate score that divides by the
raw segment count is not comparable across image resolutions. Both defects,
plus a winning hypothesis never being re-estimated from its full inlier set
(`_refit`) and a parallel-line guard that didn't actually fire on nearly
(but not exactly) parallel lines (`_vp_from`/`_intersect`'s VP_REJECT_DIAG_
MULTIPLE check), were caught by review with reproducing evidence — see
tools/footprint/test_camera.py and task-3-report.md for the measurements,
including what's still NOT fully fixed (one adversarial synthetic seed still
produces a false-positive confidence; see the box-with-clutter test).

    ~/moge-venv/bin/python -m tools.footprint.camera <image>
"""

import dataclasses
import json

import cv2
import numpy as np

from . import paths

CONFIDENCE_FLOOR = 0.05        # see calibration note below _confidence
MIN_LENGTH_FRACTION = 0.04     # of the smaller image side
INLIER_ANGLE_TOL_DEG = 2.0
RANSAC_ITERATIONS = 2000
RANSAC_SEED = 0
ORTHO_FOCAL = 1e5              # beyond this the projection is orthographic
MIN_INLIERS_PER_AXIS = 3       # gate-only: below this, "balanced" is noise
VP_REJECT_DIAG_MULTIPLE = 50   # gate-only guard against near-parallel lines
REFERENCE_DIAG = 1500.0        # canonical detection resolution; see _canonicalise


@dataclasses.dataclass
class Fit:
    """R's convention: rows are the fitted world-axis directions in camera
    coordinates (R == scene.R.T for a synth.Scene of known rotation scene.R),
    not scene.R itself — up to an unknown ROW PERMUTATION AND PER-ROW SIGN.
    Which detected vanishing point is world x/y/z, and which way each axis
    points, cannot be recovered from three orthogonal directions alone (e.g.
    on the box fixture, fit.R's row 0 comes back as -1 times s.R.T's row 2,
    not row 0). A consumer that needs a specific axis (e.g. "up") must
    disambiguate against other information — it cannot assume row i of R is
    axis i of anything.

    A vanishing point is where image-projected lines parallel to a world axis
    converge; as a camera-frame ray, that is R_world_to_camera @ world_axis, a
    whole COLUMN of the conventional world-to-camera rotation. Stacking the
    three recovered per-axis directions as rows therefore reconstructs the
    transpose of that rotation, not the rotation itself, and no amount of
    re-deriving from the same three vectors changes that — it is what the data
    is. Confirmed by hand: projecting a known world-X segment through a
    synth.Scene's R/K gives an image-space vanishing direction equal to that
    scene's R column 0, not row 0.

    `focal` and `ortho` are reported independently of `confidence`: a
    rejected (sub-floor) fit still carries whatever focal/ortho the rejected
    candidate measured, so a caller can see *why* it was rejected instead of
    the measurement collapsing into a look-alike "orthographic" result.
    `focal` is None only when no candidate could be formed at all (too few
    segments, or no RANSAC sample survived); `ortho` is True when a measured
    focal exceeded ORTHO_FOCAL. Always check `confidence` before trusting
    either — low confidence means "route to the hand-clicked fallback",
    regardless of what focal/ortho happen to say.
    """
    R: np.ndarray
    focal: float | None
    principal: tuple
    confidence: float
    source: str
    n_segments: int
    inliers: tuple
    ortho: bool

    def to_json(self) -> dict:
        return {"R": self.R.tolist(), "focal": self.focal,
                "principal": list(self.principal), "confidence": self.confidence,
                "source": self.source, "n_segments": self.n_segments,
                "inliers": list(self.inliers), "ortho": self.ortho}


def detect_segments(image_rgb, mask, min_length: float) -> np.ndarray:
    """Line segments inside the subject, as (N, 4) rows of (x1, y1, x2, y2).

    OpenCV 5 returns (N, 4) from LSD; OpenCV 4 returned (N, 1, 4). Both are
    accepted so a future pin change does not silently produce zero segments.
    """
    grey = cv2.cvtColor(image_rgb, cv2.COLOR_RGB2GRAY)
    grey = np.where(mask > 0, grey, 0).astype(np.uint8)

    detected = cv2.createLineSegmentDetector(0).detect(grey)[0]
    if detected is None or len(detected) == 0:
        return np.zeros((0, 4), np.float32)
    seg = np.asarray(detected, dtype=np.float64)
    if seg.ndim == 3:
        seg = seg[:, 0]

    dx, dy = seg[:, 2] - seg[:, 0], seg[:, 3] - seg[:, 1]
    return seg[np.hypot(dx, dy) >= min_length]


def _canonicalise(image_rgb, mask, reference_diag):
    """Resize to a fixed detection resolution before running LSD at all.

    This is the real fix for the confidence gate's resolution dependence,
    not a cleverer formula. LSD's segment count AND total inlier edge-length
    both grow with pixel resolution faster than any fixed geometric reference
    does — measured directly on ledger.webp, per-axis inlier length as a
    fraction of the image diagonal grew 0.71/1.96/2.94 from 0.5x/1x/2x, so no
    ratio computed after the fact can fully cancel it (a same-image null
    baseline gets close but does not eliminate it — see _null_baseline).
    Running the detector on the same pixel grid every time, regardless of
    what resolution the source art arrives at, removes the effect at its
    source instead of trying to normalise it out downstream. REFERENCE_DIAG
    (1500) matches synth.IMAGE_SIZE's own diagonal and the hero art
    convention (ledger.webp's native diagonal is 1497.6), so real art at its
    native resolution is left essentially untouched; only unusually large or
    small source images get resampled.
    Returns (image_rgb, mask, scale) where scale is how much the ORIGINAL
    was enlarged (>1) or shrunk (<1) to reach the canonical size — callers
    must divide any resulting pixel-space measurement (focal, principal) by
    this scale to report it back in the caller's original coordinates.
    """
    h, w = mask.shape
    diag = float(np.hypot(h, w))
    scale = reference_diag / diag
    if abs(scale - 1.0) < 1e-3:
        return image_rgb, mask, 1.0
    size = (max(1, round(w * scale)), max(1, round(h * scale)))
    interp = cv2.INTER_AREA if scale < 1.0 else cv2.INTER_CUBIC
    img_r = cv2.resize(image_rgb, size, interpolation=interp)
    mask_r = cv2.resize(mask, size, interpolation=cv2.INTER_NEAREST)
    return img_r, mask_r, scale


def _segment_lengths(seg: np.ndarray) -> np.ndarray:
    return np.hypot(seg[:, 2] - seg[:, 0], seg[:, 3] - seg[:, 1])


def _homogeneous_lines(seg: np.ndarray) -> np.ndarray:
    p1 = np.column_stack([seg[:, 0], seg[:, 1], np.ones(len(seg))])
    p2 = np.column_stack([seg[:, 2], seg[:, 3], np.ones(len(seg))])
    return np.cross(p1, p2)


def _vp_from(lines, i, j, pp, diag):
    """Vanishing point of two image lines, or None if they don't meaningfully meet.

    The raw cross-product z-component test (`abs(v[2]) < eps`) is not scale
    invariant: v's components run ~1e6-1e10 for typical image coordinates, so
    a fixed epsilon effectively never fires (two 1000px segments differing by
    1e-6 px at one endpoint still produce a "valid" vp at -5e10 px). Reject
    instead by where the vp actually LANDS, relative to the image diagonal —
    a vp that far away means the two lines are parallel in every way that
    matters, regardless of what the raw homogeneous arithmetic says.
    """
    v = np.cross(lines[i], lines[j])
    if abs(v[2]) < 1e-12:
        return None
    vp = v[:2] / v[2]
    if np.linalg.norm(vp - pp) > VP_REJECT_DIAG_MULTIPLE * diag:
        return None
    return vp


def _focal_from_two_vps(v1, v2, pp):
    """Orthocentre relation: for orthogonal directions, f^2 = -(v1-pp).(v2-pp)."""
    d = -float(np.dot(np.asarray(v1) - pp, np.asarray(v2) - pp))
    return np.sqrt(d) if d > 0 else None


def _focal_from_vps(vps, pp):
    """Median of the three pairwise orthocentre focal estimates.

    Shared by the RANSAC refit and fit_from_clicks so both get the same
    robustness: one bad pair (e.g. two nearly-parallel axes) is outvoted by
    the other two instead of being the only estimate used.
    """
    focals = [f for f in (_focal_from_two_vps(vps[i], vps[j], pp)
                          for i, j in ((0, 1), (0, 2), (1, 2)))
              if f is not None and f > 1e-6]
    return float(np.median(focals)) if focals else None


def _directions(vps, focal, pp):
    d = np.array([[v[0] - pp[0], v[1] - pp[1], focal] for v in vps], dtype=float)
    return d / np.linalg.norm(d, axis=1, keepdims=True)


def _third_vp(v1, v2, focal, pp):
    d = _directions([v1, v2], focal, pp)
    d3 = np.cross(d[0], d[1])
    if abs(d3[2]) < 1e-9:
        return None
    return np.array([pp[0] + focal * d3[0] / d3[2], pp[1] + focal * d3[1] / d3[2]])


def _score(seg, vps, tol_deg):
    """Assign each segment to the vanishing point it points at, or to none.

    A segment votes for a vp when the line from its midpoint to that vp is
    parallel to the segment itself. Returns (labels, per-vp counts, and each
    segment's angular error to its assigned vp — inf where unassigned).
    """
    mid = np.column_stack([(seg[:, 0] + seg[:, 2]) / 2, (seg[:, 1] + seg[:, 3]) / 2])
    d = np.column_stack([seg[:, 2] - seg[:, 0], seg[:, 3] - seg[:, 1]])
    d = d / np.maximum(np.linalg.norm(d, axis=1, keepdims=True), 1e-12)

    best = np.full(len(seg), -1)
    best_err = np.full(len(seg), np.inf)
    for k, v in enumerate(vps):
        to_vp = np.asarray(v)[None, :] - mid
        to_vp = to_vp / np.maximum(np.linalg.norm(to_vp, axis=1, keepdims=True), 1e-12)
        cos = np.abs(np.sum(to_vp * d, axis=1))
        err = np.degrees(np.arccos(np.clip(cos, -1, 1)))
        take = err < np.minimum(best_err, tol_deg)
        best[take], best_err[take] = k, err[take]

    counts = tuple(int((best == k).sum()) for k in range(3))
    return best, counts, best_err


def _search_score(seg, lengths, vps, tol_deg):
    """RANSAC objective: total length-weighted evidence. Never penalised by more inliers.

    _confidence (the reported gate) is deliberately NOT used to drive the
    search: it divides by segment count and multiplies by a balance term,
    both of which can go DOWN when a hypothesis explains more real structure
    (e.g. five genuine edges on one axis instead of three). An objective that
    decreases when it explains more data has the wrong shape for a search —
    it converges on a wrong-but-balanced hypothesis over the true camera.
    This score instead sums, over inliers only, each segment's pixel length
    times a linear closeness weight (1 at 0 deg error, 0 at the tolerance
    edge); every additional inlier contributes a non-negative amount, so
    explaining more never scores worse.
    """
    labels, _, err = _score(seg, vps, tol_deg)
    inlier = labels != -1
    if not np.any(inlier):
        return 0.0
    weight = np.clip(1.0 - err[inlier] / tol_deg, 0.0, 1.0)
    return float(np.sum(lengths[inlier] * weight))


def _refit_vp(lines_unit, inlier_mask):
    """Least-squares vanishing point from a set of inlier lines.

    A vp lies on every one of its lines: line . vp_h == 0 for every inlier
    homogeneous line. The least-squares vp_h (unit norm) is the right
    singular vector of the stacked, unit-normalised lines with the smallest
    singular value. Returns None when there are too few inliers to fit, or
    when the fitted vp is at infinity (not representable as a finite image
    point) — callers fall back to the coarse RANSAC estimate in that case.
    """
    idx = np.nonzero(inlier_mask)[0]
    if len(idx) < 2:
        return None
    _, _, vt = np.linalg.svd(lines_unit[idx])
    vp_h = vt[-1]
    if abs(vp_h[2]) < 1e-9:
        return None
    return vp_h[:2] / vp_h[2]


def _refit(seg, lines, vps0, focal0, pp, tol_deg):
    """Re-estimate the winning hypothesis from its whole inlier set, not 4 points.

    The RANSAC sample that wins the search is still just 2-4 raw segments;
    on real art with dozens to hundreds of candidate lines per axis, that is
    a noisy stand-in for the average of everything that actually agrees.
    Refits each vp as the least-squares intersection of its inlier lines,
    then re-derives focal and the final inlier set from the refit vps.
    """
    lines_unit = lines / np.linalg.norm(lines, axis=1, keepdims=True)
    labels, _, _ = _score(seg, vps0, tol_deg)

    refit_vps = []
    for k in range(3):
        vp = _refit_vp(lines_unit, labels == k)
        refit_vps.append(vp if vp is not None else vps0[k])

    focal = _focal_from_vps(refit_vps, pp)
    if focal is None:
        focal = focal0

    labels, counts, err = _score(seg, refit_vps, tol_deg)
    return refit_vps, focal, labels, counts, err


NULL_TRIALS = 300
NULL_SEED = 20260729   # independent of RANSAC_SEED; keeps fit() deterministic


def _length_coverage(seg, lengths, total_length, vps, tol_deg):
    """Fraction of all detected edge-length explained by the WEAKEST of the three axes."""
    labels, counts, _ = _score(seg, vps, tol_deg)
    if total_length <= 0:
        return 0.0, counts
    axis_len = min(float(lengths[labels == k].sum()) for k in range(3))
    return axis_len / total_length, counts


def _null_baseline(seg, lines, lengths, total_length, pp, diag, tol_deg):
    """Median length-coverage of RANDOM (non-optimised) VP triples on this same
    segment population — "how good would a fit look by coincidence alone".

    This is what actually fixes the resolution dependence, not the formula
    for the real fit's score. Both a raw segment count and a raw edge-length
    sum grow with resolution as LSD detects more of the real structure (I
    measured this directly: on ledger.webp, per-axis inlier length as a
    fraction of the image DIAGONAL grew 0.71 -> 1.96 -> 2.94 from 0.5x to 2x,
    i.e. dividing by a geometric constant does not cancel a detector-density
    effect). But random, non-optimised triples drawn from the SAME detected
    population are inflated by exactly the same detector-density effect, so
    comparing the real (RANSAC-optimised, refit) score against this null
    baseline computed on the same image at the same resolution cancels it:
    confidence asks "does this fit beat coincidence here", not "is this fit's
    raw score big".
    """
    rng = np.random.default_rng(NULL_SEED)
    vals = []
    tries = 0
    while len(vals) < NULL_TRIALS and tries < NULL_TRIALS * 20:
        tries += 1
        i, j, k, m = rng.choice(len(seg), 4, replace=False)
        v1, v2 = _vp_from(lines, i, j, pp, diag), _vp_from(lines, k, m, pp, diag)
        if v1 is None or v2 is None:
            continue
        focal = _focal_from_two_vps(v1, v2, pp)
        if focal is None or focal < 1e-6:
            continue
        v3 = _third_vp(v1, v2, focal, pp)
        if v3 is None:
            continue
        cov, _ = _length_coverage(seg, lengths, total_length, [v1, v2, v3], tol_deg)
        vals.append(cov)
    return float(np.median(vals)) if vals else 0.0


def _confidence(counts, coverage, null_baseline, median_residual_deg):
    """Gate score: null-normalised, and floored well before it can reward noise.

    Resolution dependence (review C2) is fixed in TWO layers, not one, and
    the split matters — see task-3-report.md for the full measurement trail
    across three candidate formulas before landing here:

    Layer 1 (does most of the work): `fit()` calls `_canonicalise` before
    doing anything else, so this function never actually sees a range of
    resolutions for the same source image — LSD always runs on the same
    pixel grid. Measured on ledger.webp (native diagonal 1497.6, essentially
    already at REFERENCE_DIAG=1500): native vs. a 2x upsample of it now
    agree to a 0.0002 confidence gap, down from a 0.288 gap (0.245 vs 0.533)
    under the pre-fix coverage*balance formula computed on the raw,
    uncanonicalised resolutions.

    Layer 2 (defence in depth): even at one fixed detection resolution, this
    function must not divide by the raw LSD segment count (`inliers /
    n_segments`, the pre-fix formula) or by a fixed geometric constant
    (image diagonal) as its only normalisation — both were tried and both
    still let a hypothesis's score depend on how much of an image's content
    happens to be detectable structure vs. noise, not on whether the
    hypothesis actually explains it. Comparing against a NULL baseline
    computed on the SAME segment population (see `_null_baseline`) instead
    asks "does this beat coincidence here", which is what actually
    generalises. The min-inliers-per-axis floor and the balance term (both
    kept from the previous version) are unaffected by any of this — they
    were never the resolution-dependent part.

    None of this is a claim that the gate is now perfectly resolution
    invariant. Upsampling a source that is genuinely LOWER resolution than
    REFERENCE_DIAG cannot recover detail that was never captured — measured
    at 0.5x, ledger.webp's confidence (0.032) is honestly lower than at
    native resolution (0.057), and that gap is real information loss, not a
    formula artifact still to be fixed.
    """
    if max(counts) == 0 or min(counts) < MIN_INLIERS_PER_AXIS:
        return 0.0
    balance = min(counts) / max(counts)
    excess = max(0.0, coverage - null_baseline) / max(1.0 - null_baseline, 1e-6)
    residual_quality = max(0.0, 1.0 - median_residual_deg / INLIER_ANGLE_TOL_DEG)
    return float(balance * excess * residual_quality)


# CONFIDENCE_FLOOR calibration (review: "choose that floor on synthetic
# ground truth... never by checking which value lets real hero art
# through"). Measured on a 30-seed sweep of _box_scene_with_clutter (known
# camera, 10 random 40-80px clutter lines per seed; see test_camera.py):
# 24/30 seeds gave an accurate fit (<1.5 deg rotation error), with
# confidence ranging 0.062-0.181; 5/30 were correctly rejected at exactly
# 0.0 (a marginal axis fell below MIN_INLIERS_PER_AXIS); cylinder_scene
# (no Manhattan structure at all) always scores 0.0 for the same reason.
# CONFIDENCE_FLOOR = 0.05 sits below the accurate cluster's measured
# minimum (0.062) with headroom, and above the 0.0 the count floor already
# produces for both failure modes above — so MIN_INLIERS_PER_AXIS, not this
# floor, is what's actually doing the rejecting in the clear-cut cases; this
# floor's job is only to catch a marginal case that clears the count floor
# without being genuinely well-supported. One seed (21) in the same sweep
# is a known, unresolved exception: a coincidental alignment scores 0.092
# (above this floor) at 23.7 deg rotation error. No floor value closes that
# gap without also rejecting the 24 genuinely good fits (their minimum,
# 0.062, is close to seed 21's 0.092) — see task-3-report.md.


def fit(image_rgb: np.ndarray, mask: np.ndarray) -> Fit:
    h0, w0 = mask.shape
    pp0 = np.array([w0 / 2.0, h0 / 2.0])

    image_rgb, mask, scale = _canonicalise(image_rgb, mask, REFERENCE_DIAG)
    h, w = mask.shape
    pp = np.array([w / 2.0, h / 2.0])
    diag = float(np.hypot(h, w))
    seg = detect_segments(image_rgb, mask, min(h, w) * MIN_LENGTH_FRACTION)
    if len(seg) < 6:
        return Fit(np.eye(3), None, tuple(pp0), 0.0, "auto", len(seg), (0, 0, 0), False)

    lengths = _segment_lengths(seg)
    lines = _homogeneous_lines(seg)
    rng = np.random.default_rng(RANSAC_SEED)
    best = None

    for _ in range(RANSAC_ITERATIONS):
        i, j, k, m = rng.choice(len(seg), 4, replace=False)
        v1, v2 = _vp_from(lines, i, j, pp, diag), _vp_from(lines, k, m, pp, diag)
        if v1 is None or v2 is None:
            continue
        focal = _focal_from_two_vps(v1, v2, pp)
        if focal is None or focal < 1e-6:
            continue
        v3 = _third_vp(v1, v2, focal, pp)
        if v3 is None:
            continue
        score = _search_score(seg, lengths, [v1, v2, v3], INLIER_ANGLE_TOL_DEG)
        if best is None or score > best[0]:
            best = (score, [v1, v2, v3], focal)

    if best is None:
        return Fit(np.eye(3), None, tuple(pp0), 0.0, "auto", len(seg), (0, 0, 0), False)

    _, vps0, focal0 = best
    refit_vps, focal, labels, counts, err = _refit(seg, lines, vps0, focal0, pp,
                                                    INLIER_ANGLE_TOL_DEG)

    inlier = labels != -1
    total_length = float(lengths.sum())
    coverage, _ = _length_coverage(seg, lengths, total_length, refit_vps, INLIER_ANGLE_TOL_DEG)
    null_baseline = _null_baseline(seg, lines, lengths, total_length, pp, diag,
                                    INLIER_ANGLE_TOL_DEG)
    median_residual = float(np.median(err[inlier])) if inlier.any() else INLIER_ANGLE_TOL_DEG
    conf = _confidence(counts, coverage, null_baseline, median_residual)

    # Report focal/principal back in the CALLER's original pixel coordinates —
    # everything above ran on the canonicalised (possibly resized) image.
    ortho = focal > ORTHO_FOCAL
    focal_out = None if ortho else float(focal / scale)
    pp_out = tuple(pp0)

    if conf <= CONFIDENCE_FLOOR:
        return Fit(np.eye(3), focal_out, pp_out, conf, "auto", len(seg), counts, ortho)

    R = _orthonormalise(_directions(refit_vps, focal, pp))
    return Fit(R, focal_out, pp_out, conf, "auto", len(seg), counts, ortho)


def _orthonormalise(d: np.ndarray) -> np.ndarray:
    """Nearest rotation matrix to three approximately orthogonal directions."""
    u, _, vt = np.linalg.svd(d)
    R = u @ vt
    if np.linalg.det(R) < 0:
        R[-1] *= -1
    return R


def _intersect(l1, l2, diag):
    """Intersection of two clicked lines, or None if they don't meaningfully meet.

    Same scale-invariance fix as `_vp_from`: reject by how far the
    intersection lands relative to the click cloud's own scale (`diag`,
    the bounding-box diagonal of every clicked point), not by a fixed
    epsilon on the raw determinant.
    """
    (x1, y1), (x2, y2) = l1
    (x3, y3), (x4, y4) = l2
    d = (x1 - x2) * (y3 - y4) - (y1 - y2) * (x3 - x4)
    if abs(d) < 1e-9:
        return None
    a, b = x1 * y2 - y1 * x2, x3 * y4 - y3 * x4
    pt = np.array([(a * (x3 - x4) - (x1 - x2) * b) / d,
                   (a * (y3 - y4) - (y1 - y2) * b) / d])
    ref = np.array([(x1 + x2 + x3 + x4) / 4.0, (y1 + y2 + y3 + y4) / 4.0])
    if np.linalg.norm(pt - ref) > VP_REJECT_DIAG_MULTIPLE * diag:
        return None
    return pt


def _click_cloud_diag(clicks: dict) -> float:
    pts = np.array([p for key in ("axis_x", "axis_y", "axis_z")
                     for line in clicks[key] for p in line], dtype=float)
    span = pts.max(axis=0) - pts.min(axis=0)
    diag = float(np.linalg.norm(span))
    return diag if diag > 1e-9 else 1.0


def fit_from_clicks(clicks: dict, principal=None) -> Fit:
    diag = _click_cloud_diag(clicks)
    vps = []
    for key in ("axis_x", "axis_y", "axis_z"):
        v = _intersect(clicks[key][0], clicks[key][1], diag)
        if v is None:
            raise ValueError(f"{key}: the two clicked lines are parallel in image space")
        vps.append(v)
    pp = np.asarray(principal if principal is not None else np.mean(vps, axis=0), dtype=float)

    focal = _focal_from_vps(vps, pp)
    if focal is None:
        raise ValueError("clicked vanishing points are not mutually orthogonal")

    ortho = focal > ORTHO_FOCAL
    return Fit(_orthonormalise(_directions(vps, focal, pp)),
               None if ortho else focal, tuple(pp),
               1.0, "clicks", 0, (0, 0, 0), ortho)


def clicks_from_scene(scene) -> dict:
    """Synthesise a click file from a known scene, for tests."""
    out = {}
    for name, axis in (("axis_x", 0), ("axis_y", 1), ("axis_z", 2)):
        d = np.zeros(3)
        d[axis] = 1.0
        lines = []
        for offset in (np.zeros(3), np.array([0.3, 0.4, 0.5])):
            pts = np.stack([offset - 2 * d, offset + 2 * d])
            cam = pts @ scene.R.T + scene.t
            uv = (cam @ scene.K.T)[:, :2] / cam[:, 2:3]
            lines.append([uv[0].tolist(), uv[1].tolist()])
        out[name] = lines
    return out


def run(ship_id: str, image_rgb, mask, clicks=None) -> Fit:
    f = fit_from_clicks(clicks) if clicks else fit(image_rgb, mask)
    (paths.artifact_dir(ship_id) / "camera.json").write_text(
        json.dumps(f.to_json(), indent=2))
    return f
