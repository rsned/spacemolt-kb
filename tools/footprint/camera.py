#!/usr/bin/env python3
"""Stage 2: fit the camera from vanishing points.

These hulls are Manhattan-world objects with long parallel structural lines, so
three orthogonal vanishing points give rotation and focal length outright and
reveal whether a render is orthographic or mildly perspective. Never assume the
3/4 view: a low-confidence fit routes the ship to the hand-clicked fallback.

Implemented directly on cv2's line segment detector. lu-vp-detect is not used:
it breaks on OpenCV 5.x (it strips a singleton dimension LSD no longer emits)
and never exposed the cluster memberships a confidence measure needs.

    ~/moge-venv/bin/python -m tools.footprint.camera <image>
"""

import dataclasses
import json

import cv2
import numpy as np

from . import paths

CONFIDENCE_FLOOR = 0.35
MIN_LENGTH_FRACTION = 0.04     # of the smaller image side
INLIER_ANGLE_TOL_DEG = 2.0
RANSAC_ITERATIONS = 2000
RANSAC_SEED = 0
ORTHO_FOCAL = 1e5              # beyond this the projection is orthographic
MIN_INLIERS_PER_AXIS = 3       # below this, a "balanced" fit is noise, not a frame


@dataclasses.dataclass
class Fit:
    """R's convention: rows are the fitted world-axis directions in camera
    coordinates (R == scene.R.T for a synth.Scene of known rotation scene.R),
    not scene.R itself.

    A vanishing point is where image-projected lines parallel to a world axis
    converge; as a camera-frame ray, that is R_world_to_camera @ world_axis, a
    whole COLUMN of the conventional world-to-camera rotation. Stacking the
    three recovered per-axis directions as rows therefore reconstructs the
    transpose of that rotation, not the rotation itself, and no amount of
    re-deriving from the same three vectors changes that — it is what the data
    is. Confirmed by hand: projecting a known world-X segment through a
    synth.Scene's R/K gives an image-space vanishing direction equal to that
    scene's R column 0, not row 0.
    """
    R: np.ndarray
    focal: float | None
    principal: tuple
    confidence: float
    source: str
    n_segments: int
    inliers: tuple

    def to_json(self) -> dict:
        return {"R": self.R.tolist(), "focal": self.focal,
                "principal": list(self.principal), "confidence": self.confidence,
                "source": self.source, "n_segments": self.n_segments,
                "inliers": list(self.inliers)}


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


def _homogeneous_lines(seg: np.ndarray) -> np.ndarray:
    p1 = np.column_stack([seg[:, 0], seg[:, 1], np.ones(len(seg))])
    p2 = np.column_stack([seg[:, 2], seg[:, 3], np.ones(len(seg))])
    return np.cross(p1, p2)


def _vp_from(lines, i, j):
    v = np.cross(lines[i], lines[j])
    if abs(v[2]) < 1e-9:          # the two lines are parallel in the image
        return None
    return v[:2] / v[2]


def _focal_from_two_vps(v1, v2, pp):
    """Orthocentre relation: for orthogonal directions, f^2 = -(v1-pp).(v2-pp)."""
    d = -float(np.dot(np.asarray(v1) - pp, np.asarray(v2) - pp))
    return np.sqrt(d) if d > 0 else None


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
    parallel to the segment itself. Returns (labels, per-vp counts).
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
    return best, counts


def _confidence(counts, n_segments):
    """Explained fraction, penalised when one direction dominates.

    A fit that assigns every segment to one vanishing point has found a single
    edge direction, not a Manhattan frame, so the balance term must pull it
    down. Both terms are needed: coverage alone rewards a degenerate fit, and
    balance alone rewards a fit that explains three segments out of a thousand.

    Coverage and balance alone are not enough: a nearly circular silhouette
    (e.g. a disc viewed close to top-down) reduces to a handful of long
    LSD segments — chords of the same arc — that can land 2-2-2 across three
    "orthogonal" directions purely by the combinatorics of a low count, giving
    coverage*balance north of 0.5 with no real Manhattan structure behind it.
    A Manhattan corner needs at least a few lines actually converging on each
    vanishing point to trust it; below that, floor the score to zero rather
    than let two coincidentally-aligned segments outvote noise. Confirmed
    with cylinder_scene at several azimuth/elevation combinations: min(counts)
    tops out at 2 there, vs. 3 for box_scene's near-corner view, so the floor
    at 3 separates the two without touching CONFIDENCE_FLOOR itself.
    """
    if n_segments == 0 or max(counts) == 0 or min(counts) < MIN_INLIERS_PER_AXIS:
        return 0.0
    coverage = sum(counts) / n_segments
    balance = min(counts) / max(counts)
    return float(coverage * balance)


def fit(image_rgb: np.ndarray, mask: np.ndarray) -> Fit:
    h, w = mask.shape
    pp = np.array([w / 2.0, h / 2.0])
    seg = detect_segments(image_rgb, mask, min(h, w) * MIN_LENGTH_FRACTION)
    if len(seg) < 6:
        return Fit(np.eye(3), None, tuple(pp), 0.0, "auto", len(seg), (0, 0, 0))

    lines = _homogeneous_lines(seg)
    rng = np.random.default_rng(RANSAC_SEED)
    best = None

    for _ in range(RANSAC_ITERATIONS):
        i, j, k, m = rng.choice(len(seg), 4, replace=False)
        v1, v2 = _vp_from(lines, i, j), _vp_from(lines, k, m)
        if v1 is None or v2 is None:
            continue
        focal = _focal_from_two_vps(v1, v2, pp)
        if focal is None or focal < 1e-6:
            continue
        v3 = _third_vp(v1, v2, focal, pp)
        if v3 is None:
            continue
        _, counts = _score(seg, [v1, v2, v3], INLIER_ANGLE_TOL_DEG)
        conf = _confidence(counts, len(seg))
        if best is None or conf > best[0]:
            best = (conf, [v1, v2, v3], focal, counts)

    if best is None:
        return Fit(np.eye(3), None, tuple(pp), 0.0, "auto", len(seg), (0, 0, 0))

    conf, vps, focal, counts = best
    if conf <= CONFIDENCE_FLOOR:
        return Fit(np.eye(3), None, tuple(pp), conf, "auto", len(seg), counts)

    R = _orthonormalise(_directions(vps, focal, pp))
    return Fit(R, None if focal > ORTHO_FOCAL else float(focal), tuple(pp),
               conf, "auto", len(seg), counts)


def _orthonormalise(d: np.ndarray) -> np.ndarray:
    """Nearest rotation matrix to three approximately orthogonal directions."""
    u, _, vt = np.linalg.svd(d)
    R = u @ vt
    if np.linalg.det(R) < 0:
        R[-1] *= -1
    return R


def _intersect(l1, l2):
    (x1, y1), (x2, y2) = l1
    (x3, y3), (x4, y4) = l2
    d = (x1 - x2) * (y3 - y4) - (y1 - y2) * (x3 - x4)
    if abs(d) < 1e-9:
        return None
    a, b = x1 * y2 - y1 * x2, x3 * y4 - y3 * x4
    return np.array([(a * (x3 - x4) - (x1 - x2) * b) / d,
                     (a * (y3 - y4) - (y1 - y2) * b) / d])


def fit_from_clicks(clicks: dict, principal=None) -> Fit:
    vps = []
    for key in ("axis_x", "axis_y", "axis_z"):
        v = _intersect(clicks[key][0], clicks[key][1])
        if v is None:
            raise ValueError(f"{key}: the two clicked lines are parallel in image space")
        vps.append(v)
    pp = np.asarray(principal if principal is not None else np.mean(vps, axis=0), dtype=float)

    focals = [f for f in (_focal_from_two_vps(vps[i], vps[j], pp)
                          for i, j in ((0, 1), (0, 2), (1, 2))) if f is not None]
    if not focals:
        raise ValueError("clicked vanishing points are not mutually orthogonal")
    focal = float(np.median(focals))

    return Fit(_orthonormalise(_directions(vps, focal, pp)),
               None if focal > ORTHO_FOCAL else focal, tuple(pp),
               1.0, "clicks", 0, (0, 0, 0))


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
