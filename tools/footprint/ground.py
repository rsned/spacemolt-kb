#!/usr/bin/env python3
"""Stage 6: project the resolved cloud to the ground plane and outline it.

The footprint is an alpha shape, never a convex hull: these hulls have real
concavities between nacelles and a convex hull erases exactly the features that
tell one ship from another. The alpha radius is pinned once per batch, because
a per-ship alpha is a free parameter that can be turned until any footprint
looks right.

    ~/moge-venv/bin/python -m tools.footprint.ground <ship_id>
"""

import json

import alphashape
import numpy as np
import shapely
from shapely.geometry import MultiPolygon, mapping

from . import paths

ALPHA_CANDIDATES = [0.5, 1.0, 2.0, 3.0, 5.0, 8.0, 13.0]

# alphashape's cost is superlinear and severe: measured on a uniform rectangle,
# 2k points take 0.59s, 8k take 2.66s, 20k take 7.79s and 50k take 21.54s. A
# production MoGe cloud is ~400k points, and sweep_alpha evaluates every
# candidate against every cloud. Extrapolating, a 7-candidate sweep over 335
# ships at full density is hundreds of hours. Subsample before both the sweep
# and the final hull; the footprint outline does not need every point, and using
# the same cap for both keeps the chosen alpha meaningful for the hull it will
# be applied to.
SWEEP_SAMPLE = 20_000


def subsample(xy: np.ndarray, cap: int = SWEEP_SAMPLE, seed: int = 0) -> np.ndarray:
    """Deterministically thin a cloud to at most `cap` points.

    Seeded, because an alpha swept on one random subset and applied to another
    is not the alpha that was validated.
    """
    if len(xy) <= cap:
        return xy
    idx = np.random.default_rng(seed).choice(len(xy), size=cap, replace=False)
    return xy[np.sort(idx)]


def up_vector(sym, points, normals=None, fit=None) -> np.ndarray:
    """World up in camera coordinates, from the hull's own recovered geometry.

    REVISED 2026-07-29. This used to treat a confident vanishing-point fit as
    authoritative and fall back to `normals.mean(axis=0)`. Both were wrong:

    - Only 1 of 14 keyable hero images produces a confident fit at all, so the
      camera path is the exception, not the rule (see the plan's Global
      Constraints).
    - The mean surface normal of a SINGLE-VIEW cloud points roughly at the
      camera, not at the deck, because every visible facet faces the viewer.
      That fallback would have returned a badly wrong up on every ship that
      used it.

    The hull frame is fully determined by geometry already recovered upstream:
    the symmetry-plane normal is the lateral axis, the cloud's principal axis
    within that plane is longitudinal, and up is their cross product.

    `fit` is accepted only to log agreement where stage 2 is confident; it
    never overrides the geometric frame.
    """
    lateral = np.asarray(sym.normal, dtype=float)
    lateral /= np.linalg.norm(lateral)

    # Longitudinal: the dominant direction of the cloud with the lateral
    # component projected out, so it is guaranteed to lie in the symmetry plane.
    centred = points - points.mean(axis=0)
    in_plane = centred - np.outer(centred @ lateral, lateral)
    longitudinal = np.linalg.svd(in_plane, full_matrices=False)[2][0]
    longitudinal /= np.linalg.norm(longitudinal)

    up = np.cross(lateral, longitudinal)
    up /= np.linalg.norm(up)

    # Sign: the hull is seen from above, so the camera lies on the +up side.
    # In camera coordinates the viewer sits at the origin looking down +Z, so
    # the visible surface's normals carry the sign even though their mean is a
    # poor direction estimate. Fall back to "points away from the cloud
    # centroid's viewing direction" when normals are absent.
    if normals is not None and len(normals):
        reference = -np.asarray(normals, dtype=float).mean(axis=0)
    else:
        reference = -points.mean(axis=0)
    if float(up @ reference) < 0:
        up = -up
    return up


def project(points: np.ndarray, up: np.ndarray) -> np.ndarray:
    """Drop the up component, returning ground-plane coordinates."""
    u = up / np.linalg.norm(up)
    a = np.array([1.0, 0.0, 0.0])
    if abs(a @ u) > 0.9:
        a = np.array([0.0, 0.0, 1.0])
    e1 = a - (a @ u) * u
    e1 /= np.linalg.norm(e1)
    e2 = np.cross(e1, u)
    return np.stack([points @ e1, points @ e2], axis=1)


SPECK_AREA_FRACTION = 0.01     # of the largest part; below this it is noise


def hull(xy: np.ndarray, alpha: float):
    """The alpha shape, keeping every substantial part.

    Do NOT reduce a MultiPolygon to `max(geoms, key=area)`. A ship with
    separated nacelles has a genuinely MULTI-PART footprint, and taking the
    largest part reports one nacelle as the whole ship. Measured on the
    two-bar fixture below (bars at y in [-1,-0.4] and [0.4,1.0], true area
    4.800): at alpha 3.0 the alpha shape correctly splits into 2 parts, and
    max-area returns 2.336 — one bar — while keeping both returns 4.663.

    That collapse also corrupted two things downstream, which is why this is
    not a cosmetic choice:
      - `profile.canonicalise` reads `.exterior`, which a MultiPolygon does not
        have, so the collapse was masking an AttributeError rather than
        avoiding one.
      - `sweep_alpha` requires a cloud to retain 90% of its points; a collapsed
        twin-nacelle hull retains about half, so the sweep rejected every alpha
        tight enough to resolve the gap and drove the WHOLE BATCH toward
        convex-hull behaviour.

    Specks are still dropped: parts below SPECK_AREA_FRACTION of the largest
    are outlier noise, not structure.
    """
    poly = alphashape.alphashape(xy, alpha)
    if not isinstance(poly, MultiPolygon):
        return poly
    parts = sorted(poly.geoms, key=lambda g: g.area, reverse=True)
    if not parts:
        return poly
    floor = parts[0].area * SPECK_AREA_FRACTION
    keep = [g for g in parts if g.area >= floor]
    return keep[0] if len(keep) == 1 else MultiPolygon(keep)


def sweep_alpha(clouds_xy, candidates=None) -> float:
    """Pick one alpha for the whole batch: the tightest that keeps its points.

    Larger alpha hugs the points more closely but eventually sheds them; the
    batch value is the largest candidate for which every cloud's alpha shape
    still contains most of its own points.

    The criterion is point retention, NOT connectivity. A footprint that splits
    into two parts is the correct answer for a ship with separated nacelles, so
    requiring a single polygon would reject exactly the alphas that resolve the
    feature this stage exists to capture — and, because the batch shares one
    alpha, one twin-hull ship would drag every other ship toward convex-hull
    behaviour.

    Pass ALREADY-SUBSAMPLED clouds: see SWEEP_SAMPLE and the note on `hull`.
    """
    candidates = candidates or ALPHA_CANDIDATES
    best = float(min(float(c) for c in candidates))
    for a in sorted(float(c) for c in candidates):
        ok = True
        for xy in clouds_xy:
            p = hull(xy, a)
            if p.is_empty or p.area <= 0:
                ok = False
                break
            # shapely.contains_xy, not MultiPoint(...).intersection: measured at
            # 50k points the MultiPoint route takes 1.53s and this takes 0.012s,
            # a 127x difference, and it is called once per candidate per cloud.
            # They also disagree — MultiPoint with buffer(1e-9) counts boundary
            # vertices as covered and reports 50000/50000, while contains_xy
            # reports 49747. The threshold below is calibrated against
            # contains_xy's stricter count.
            kept = int(shapely.contains_xy(p, xy[:, 0], xy[:, 1]).sum())
            if kept < 0.9 * len(xy):
                ok = False
                break
        if ok:
            best = a
    return best


def run(ship_id: str, points, up, alpha: float):
    xy = subsample(project(points, up))
    poly = hull(xy, alpha)
    (paths.artifact_dir(ship_id) / "footprint.json").write_text(
        json.dumps({"alpha": alpha, "polygon": mapping(poly)}, indent=2))
    return poly
