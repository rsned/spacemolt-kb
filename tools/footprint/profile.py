#!/usr/bin/env python3
"""Stage 7: canonicalise the footprint into glyph space and sample it.

Glyph space runs X along the hull's long axis from 0 to 1 with Y a signed
half-width, so a canonicalised footprint sampled at the same 96 stations
pkg/shipglyph uses is directly comparable to an inferred profile. Stations
where the cross-section splits are flagged: a single half-width per station
cannot express the gap between two nacelles, and that limitation is a finding,
not something to average away.

WHICH END IS X=0 (bow vs stern) IS NOT RESOLVED HERE. `canonicalise` rotates
using the symmetry-plane normal's in-ground direction (`axis = [-n[1], n[0]]`
below), and that direction's SIGN is arbitrary: `mirror.reflect` depends only
on the plane, not which way its normal points, so `mirror.solve`/
`solve_from_view` never fix a sign -- `(n, off)` and `(-n, -off)` are the same
plane to every stage upstream of this one. Measured directly on the Task 9
batch: `ledger`'s canonicalised profile tapers toward x=0 and `yard_sale`'s
tapers toward x=1, with nothing in the code that would explain a consistent
choice between them (see task-9 final review, finding I2). `run()` below
therefore records `"orientation": "unknown"` in profile.json rather than a
value nothing in this pipeline actually determined. Disambiguating bow from
stern is left to the consuming half, which has glyph ground truth to check a
candidate orientation against -- inventing a heuristic here (longest taper,
narrowest end, etc.) would just be a guess wearing a schema field.

    ~/moge-venv/bin/python -m tools.footprint.profile <ship_id>
"""

import json

import numpy as np
from shapely import affinity
from shapely.geometry import LineString

from . import paths

STATIONS = 96


def canonicalise(poly, sym_normal_xy: np.ndarray):
    """Rotate so the longitudinal axis is +X, translate to x=0, scale length to 1."""
    n = np.asarray(sym_normal_xy, dtype=float)
    n /= np.linalg.norm(n)
    axis = np.array([-n[1], n[0]])  # the symmetry plane's in-ground direction
    angle = np.degrees(np.arctan2(axis[1], axis[0]))
    out = affinity.rotate(poly, -angle, origin="centroid", use_radians=False)

    # Every part's coords, not `out.exterior`: a twin-nacelle footprint is a
    # MultiPolygon, which has no `.exterior` at all (AttributeError). This was
    # previously hidden by ground.hull collapsing to its largest part, which
    # silently discarded the other nacelle.
    geoms = list(out.geoms) if hasattr(out, "geoms") else [out]
    xs, ys = np.concatenate([np.array(g.exterior.coords) for g in geoms]).T
    length = xs.max() - xs.min()
    if length <= 0:
        raise ValueError("degenerate footprint: zero length along the hull axis")
    out = affinity.translate(out, xoff=-xs.min(), yoff=-(ys.min() + ys.max()) / 2.0)
    return affinity.scale(out, xfact=1.0 / length, yfact=1.0 / length, origin=(0, 0))


def sample(poly, stations: int = STATIONS):
    """Half-width and split flag at each station along the hull."""
    miny, maxy = poly.bounds[1], poly.bounds[3]
    pad = max(abs(miny), abs(maxy)) + 1.0
    w = np.zeros(stations)
    concave = np.zeros(stations, dtype=bool)

    for i in range(stations):
        t = i / (stations - 1)
        cut = poly.intersection(LineString([(t, -pad), (t, pad)]))
        if cut.is_empty:
            continue
        parts = list(cut.geoms) if hasattr(cut, "geoms") else [cut]
        parts = [p for p in parts if p.length > 0]
        if not parts:
            continue
        ys = np.concatenate([np.array(p.coords)[:, 1] for p in parts])
        w[i] = float(np.abs(ys).max())
        concave[i] = len(parts) > 1
    return w, concave


def aspect(w: np.ndarray) -> float:
    """Length over maximum beam — exactly Descriptor.Aspect."""
    m = float(np.max(w))
    return float("inf") if m <= 0 else 1.0 / (2.0 * m)


# Stage 7 is the ONLY place a scale or depth error can be caught. The stage 5
# silhouette gate is provably blind to both: `uv = K.p / p_z` is exactly
# invariant under `p -> lambda(p) . p` for any positive per-point lambda, so a
# uniformly scaled cloud and a cloud flattened along the viewing rays reproject
# to the identical silhouette. Measured in the Task 6 review: flattening a cloud
# to 10% of its depth extent leaves IoU at 0.9910, unchanged to four decimals
# from the unflattened cloud, and x5 global scale likewise. A hull reconstructed
# as a billboard arrives here with `silhouette_pass: true`.
#
# So these bounds are not defensive boilerplate — they are the pipeline's only
# check on the dimension it exists to publish.
ASPECT_BOUNDS = (1.2, 12.0)      # derived from the catalog's own ship dimensions
MIN_DEPTH_TO_BEAM = 0.15         # below this the reconstruction is a pancake


def implausible(w: np.ndarray, depth_extent: float) -> str | None:
    """Why this profile must not be published, or None if it is plausible.

    Returns a reason string rather than a bool so the batch report can say what
    was wrong with each excluded ship instead of just counting failures.
    """
    a = aspect(w)
    lo, hi = ASPECT_BOUNDS
    if not np.isfinite(a):
        return "degenerate footprint: zero maximum beam"
    if a < lo:
        return f"aspect {a:.2f} below {lo}: too stubby to be a hull"
    if a > hi:
        return f"aspect {a:.2f} above {hi}: a sliver, not a hull"
    beam = 2.0 * float(np.max(w))
    if depth_extent / beam < MIN_DEPTH_TO_BEAM:
        return (f"depth/beam {depth_extent / beam:.3f} below "
                f"{MIN_DEPTH_TO_BEAM}: reconstructed as a flat card")
    return None


# Bumped whenever a field is added, removed, or changes meaning -- the Go
# reader (not built yet) needs a version to gate on rather than guessing from
# key presence. Added ahead of that reader existing (task-9 final review
# M12) specifically so "schema": 1 is already the FIRST profile.json any
# consumer ever sees, not something retrofitted onto files it must special-
# case.
SCHEMA_VERSION = 1


def run(ship_id: str, poly, sym_normal_xy, quality: dict,
        depth_extent: float | None = None) -> dict:
    canon = canonicalise(poly, sym_normal_xy)
    w, concave = sample(canon)
    reason = None if depth_extent is None else implausible(w, depth_extent)
    data = {"schema": SCHEMA_VERSION, "id": ship_id, "stations": STATIONS,
            "w": w.tolist(), "concave": concave.tolist(), "aspect": aspect(w),
            # See this module's docstring: canonicalise's X=0 end (bow vs
            # stern) is not determined by anything in this pipeline. Recorded
            # explicitly, not omitted, so a consumer must handle it rather
            # than silently assuming a convention that was never established.
            "orientation": "unknown",
            "quality": quality,
            "dimensional_pass": reason is None, "dimensional_reason": reason}
    (paths.artifact_dir(ship_id) / "profile.json").write_text(json.dumps(data, indent=2))
    return data
