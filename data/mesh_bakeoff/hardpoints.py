#!/usr/bin/env python3
"""Heuristic engine + weapon-mount guesses for cinema-frame hulls
(+X bow, +Y dorsal, length 1). Deliberately rough: the future placement
tool overwrites the sidecar with source "placed"."""
import numpy as np
from scipy.cluster.hierarchy import fclusterdata

REAR_SLAB = 0.10         # rear fraction of length searched for nozzles
FACING = 0.7             # normal . (-X) for a nozzle face
LINK = 0.03              # single-linkage distance (length units) in YZ
MIN_AREA_FRAC = 0.02     # of all rear-facing area
WALL_FRAC = 0.4          # cluster spread > this * hull beam/height = a wall, not a nozzle
MAX_CLUSTER_PTS = 4000
R_MIN, R_MAX = 0.015, 0.12


def _faces(v, f):
    tri = v[f]
    cross = np.cross(tri[:, 1] - tri[:, 0], tri[:, 2] - tri[:, 0])
    area = np.linalg.norm(cross, axis=1) / 2
    normal = cross / np.maximum(2 * area[:, None], 1e-12)
    return tri.mean(axis=1), normal, area


def _r(x):
    return [round(float(c), 4) for c in x]


def guess_engines(verts, faces, max_engines=6):
    v, f = np.asarray(verts, float), np.asarray(faces, np.int64)
    cent, normal, area = _faces(v, f)
    xmin = v[:, 0].min()
    span_yz = np.ptp(v[:, 1:], axis=0).max()
    sel = (cent[:, 0] < xmin + REAR_SLAB) & (normal[:, 0] < -FACING) & (area > 0)
    engines = []
    if sel.sum() >= 2:
        idx = np.flatnonzero(sel)
        if len(idx) > MAX_CLUSTER_PTS:
            idx = np.random.default_rng(0).choice(idx, MAX_CLUSTER_PTS, replace=False)
        labels = fclusterdata(cent[idx][:, 1:], t=LINK, criterion="distance", method="single")
        total = area[idx].sum()
        for lab in np.unique(labels):
            m = idx[labels == lab]
            a = area[m]
            if a.sum() < MIN_AREA_FRAC * total:
                continue
            spread = np.ptp(v[f[m]].reshape(-1, 3)[:, 1:], axis=0).max()
            if spread > WALL_FRAC * span_yz:
                continue
            yz = np.average(cent[m][:, 1:], axis=0, weights=a)
            x = v[f[m]].reshape(-1, 3)[:, 0].min()
            engines.append((a.sum(), {"pos": _r([x, yz[0], yz[1]]),
                                      "radius": round(float(np.clip(spread / 2, R_MIN, R_MAX)), 4)}))
    engines = [e for _, e in sorted(engines, key=lambda t: -t[0])][:max_engines]
    if not engines:
        stern = v[v[:, 0] < xmin + 0.02]
        engines = [{"pos": _r([xmin, float(np.median(stern[:, 1])), 0.0]), "radius": 0.05}]
    return engines


def guess_mounts(verts, faces, max_mounts=12, pair_tol=0.04):
    v, f = np.asarray(verts, float), np.asarray(faces, np.int64)
    cent, normal, area = _faces(v, f)
    ok = (cent[:, 0] > 0) & ((normal[:, 0] > 0.5) | (normal[:, 1] > 0.6)) & (area > 0)
    cand = np.flatnonzero(ok)
    if not len(cand):
        return []
    seeds_pool = cand[cent[cand, 2] >= -1e-6]         # starboard + centreline seeds
    if not len(seeds_pool):
        seeds_pool = cand
    first = seeds_pool[np.argmax(cent[seeds_pool, 1])]  # highest dorsal point
    chosen = [first]
    dist = np.linalg.norm(cent[seeds_pool] - cent[first], axis=1)
    out = []

    def emit(i):
        out.append({"pivot": _r(cent[i]), "normal": _r(normal[i] / np.linalg.norm(normal[i]))})

    def mirror_of(i):
        target = cent[i] * np.array([1, 1, -1])
        d = np.linalg.norm(cent[cand] - target, axis=1)
        j = cand[np.argmin(d)]
        return j if d.min() < pair_tol else None

    while len(out) < max_mounts:
        i = chosen[-1]
        emit(i)
        if abs(cent[i, 2]) > 0.02 and len(out) < max_mounts:
            j = mirror_of(i)
            if j is not None:
                emit(j)
        k = int(np.argmax(dist))
        if dist[k] < 1e-6:
            break
        chosen.append(seeds_pool[k])
        dist = np.minimum(dist, np.linalg.norm(cent[seeds_pool] - cent[seeds_pool[k]], axis=1))
    return out[:max_mounts]
