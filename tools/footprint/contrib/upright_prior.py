#!/usr/bin/env python3
"""Prototype: upright-art prior in the mirror-plane search.

Hypothesis (user-confirmed on symposium/prayer): hero art keeps ships visually
upright, so the TRUE lateral axis is near-horizontal in image y. The plane
search ties on near-square cross-sections and sometimes picks the dorsal
plane (frame rolled 90deg about Y). A soft penalty lam*|n_y| on candidate
normals breaks exactly those ties without touching feasibility.

Runs the modified solve on the 22 plane-swap suspects plus a control sample of
previously-good ships, straight from each ship's stage-3 artifacts (cloud.npz
+ matte.png). Reports: plane change (deg), old/new screen-tilt of the implied
up axis, old/new inside_fraction, and for ships that reach stage 6 logic a
recomputed footprint aspect via the production ground/profile stages.
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib
import sys

import cv2
import numpy as np
from scipy.optimize import minimize
from scipy.spatial import cKDTree

sys.path.insert(0, _REPO)
from tools.footprint import gate, ground, mirror, profile  # noqa: E402

FOOT = pathlib.Path(_REPO + "/data/footprints")
# User bound (2026-08-01): every hero image keeps the ship's +Z within +/-20deg
# of screen-vertical. Lateral is perpendicular to up, so |n_y| <= sin(20deg)
# ~= 0.34 for any admissible plane. Penalty: zero-ish inside the cone (tiny
# linear tie-break), steep outside it — a dorsal-plane swap sits at |n_y|~0.9.
CONE = 0.42         # sin(25deg): the user's 20 plus margin for solve noise
STEEP = 0.5         # frac-units per unit |n_y| beyond the cone
LAM = 0.01          # gentle tie-break inside the cone
ALPHA = 8.0


def upright_penalty(n_y):
    a = abs(float(n_y))
    return LAM * a + STEEP * max(0.0, a - CONE)

SUSPECTS = ["convoy", "pike", "cryogenic_survey", "syndicate", "guillotine", "crucible",
            "portcullis", "blind_spot", "spit_and_prayer", "paramount", "manifest_destiny",
            "start_praying", "munitions_foundry", "symposium", "no_refunds", "buckler",
            "prayer", "absolute_zero", "bad_influence", "confluence", "capacity", "prospect"]
CONTROLS = ["magnate", "comet", "severance_package", "no_appeal", "frostbite",
            "war_wagon", "excessive_force", "reliquary", "ledger", "yard_sale",
            "juggernaut", "taxonomy", "venture", "datum", "four_on_the_floor"]


def solve_with_prior(points, mask, intrinsics, lam=LAM):
    """mirror.solve_from_view with `lam*|n_y|` added to every feasible cost.

    Verbatim copy of the production search (same constants, same seeds, same
    refinement) apart from the two `+ lam * ...` lines; the coarse ranking
    carries the penalty too so upright basins are seeded, not just refined.
    """
    shape = mask.shape
    rng = np.random.default_rng(0)
    idx = rng.choice(len(points), min(mirror._SUBSAMPLE, len(points)), replace=False)
    sample = points[idx]
    extent = float(np.linalg.norm(sample.max(axis=0) - sample.min(axis=0)))
    z_extent = float(points[:, 2].max() - points[:, 2].min())
    floor = gate.MIN_DEPTH_SEPARATION_FRACTION * z_extent
    depth_points = points[rng.choice(len(points), min(mirror._DEPTH_SUBSAMPLE, len(points)),
                                     replace=False)]
    depth = mirror._visible_depth(depth_points, intrinsics, shape)
    tree = cKDTree(sample)

    def score(theta, phi, off, proximity=True):
        n = mirror._sph(theta, phi)
        mirrored = mirror.reflect(sample, n, off)
        sep, _ = mirror._depth_separation(depth, mirrored, intrinsics, shape)
        near = 0.0
        if proximity:
            probe = mirrored[:: max(1, len(mirrored) // mirror._PROXIMITY_SAMPLE)]
            near = float(tree.query(probe)[0].mean()) / max(extent, 1e-9)
        feasible = sep >= floor and near <= mirror._PROXIMITY_CEILING
        return feasible, gate.inside_fraction(mirrored, intrinsics, mask), sep, near

    def cost(x, proximity=True):
        feasible, frac, sep, near = score(*x[:3], proximity=proximity)
        if not feasible:
            return (1.0 + max(0.0, floor - sep) / max(floor, 1e-12)
                    + max(0.0, near - mirror._PROXIMITY_CEILING) / mirror._PROXIMITY_CEILING)
        n = mirror._sph(x[0], x[1])
        return -frac + upright_penalty(n[1])    # <-- the upright prior

    coarse = []
    for n in mirror._hemisphere(mirror._SEARCH_NORMALS):
        theta = np.arccos(np.clip(n[2], -1.0, 1.0))
        phi = np.arctan2(n[1], n[0])
        proj = sample @ n
        pad = 0.1 * (proj.max() - proj.min())
        for off in np.linspace(proj.min() - pad, proj.max() + pad, mirror._SEARCH_OFFSETS):
            x = [theta, phi, float(off)]
            coarse.append((cost(x, proximity=False), n, x))
    coarse.sort(key=lambda row: row[0])

    seeds = []
    for row in coarse:
        if all(mirror._angle_deg(row[1], kept[1]) > mirror._BASIN_SEPARATION_DEG
               for kept in seeds):
            seeds.append(row)
        if len(seeds) == mirror._REFINE_BASINS:
            break

    results = []
    for _, _, (theta, phi, off) in seeds:
        res = minimize(cost, [theta, phi, off], method="Nelder-Mead",
                       options={"xatol": 1e-4, "fatol": 1e-6, "maxiter": 400,
                                "adaptive": True})
        results.append((res.fun, res.x))

    _, x = min(results, key=lambda r: r[0])
    feasible, frac, sep, near = score(*x[:3])
    chamfer = mirror._chamfer(sample, mirror.reflect(sample, mirror._sph(x[0], x[1]), x[2])) \
        / max(extent, 1e-9)
    failure = None
    if not feasible:
        failure = "infeasible"
    elif frac < gate.MIR_FLOOR:
        failure = f"inside_fraction={frac:.4f} below MIR_FLOOR"
    return mirror.Symmetry(normal=mirror._sph(x[0], x[1]), offset=float(x[2]),
                           scale=1.0, shift=0.0, residual=float(chamfer),
                           depth_separation=float(sep), failure=failure), frac


def frame_of(normal, points):
    lat = normal / np.linalg.norm(normal)
    ip = points - np.outer(points @ lat, lat)
    ip -= ip.mean(axis=0)
    _, _, vt = np.linalg.svd(ip[:: max(1, len(ip) // 20000)], full_matrices=False)
    lon = vt[0] / np.linalg.norm(vt[0])
    up = np.cross(lat, lon)
    return lat, lon, up


def up_tilt(up):
    """Screen tilt of the implied up axis, deg from screen-vertical (sign-free)."""
    return float(np.degrees(np.arctan2(abs(up[0]), abs(up[1]))))


def new_aspect(sid, points, normals, sym):
    """Aspect via the production stages 6-7 on the re-solved plane."""
    full = mirror.complete(points, sym)
    up = ground.up_vector(sym, full, np.vstack([normals, normals]))
    xy = ground.project(full, up)
    poly = ground.hull(ground.subsample(xy), ALPHA)
    sym_xy = ground.project(sym.normal[None, :], up)[0]   # production chain, run.py:296
    canon = profile.canonicalise(poly, sym_xy)
    w, _ = profile.sample(canon)
    return float(profile.aspect(w))


rows = []
for group, sids in (("SUSPECT", SUSPECTS), ("control", CONTROLS)):
    for sid in sids:
        npz = FOOT / sid / "cloud.npz"
        mpath = FOOT / sid / "matte.png"
        if not npz.exists() or not mpath.exists():
            print(f"{sid}: missing artifacts, skipped")
            continue
        d = np.load(npz)
        points, normals, K = d["points"], d["normals"], d["intrinsics"]
        mask = (cv2.imread(str(mpath), cv2.IMREAD_GRAYSCALE) > 127).astype(np.uint8)

        old = np.load(FOOT / sid / "cloud_resolved.npz")
        old_n = old["normal"] / np.linalg.norm(old["normal"])
        _, _, old_up = frame_of(old_n, points)

        sym, frac = solve_with_prior(points, mask, K)
        new_n = sym.normal / np.linalg.norm(sym.normal)
        _, _, nup = frame_of(new_n, points)
        dang = float(np.degrees(np.arccos(np.clip(abs(old_n @ new_n), 0, 1))))

        asp = ""
        try:
            asp = f"{new_aspect(sid, points, normals, sym):.2f}"
        except Exception as e:
            asp = f"({type(e).__name__})"
        rows.append((group, sid, dang, up_tilt(old_up), up_tilt(nup), frac, sym.failure, asp,
                     [float(v) for v in sym.normal], float(sym.offset)))
        print(f"{group:8s} {sid:22s} plane_moved={dang:5.1f}deg  "
              f"up_tilt {up_tilt(old_up):4.0f}->{up_tilt(nup):4.0f}  "
              f"frac={frac:.4f}  new_aspect={asp}  {sym.failure or ''}", flush=True)

json.dump([list(r) for r in rows],
          open(pathlib.Path.cwd() / "upright_prior_results.json", "w"), indent=1)
print("\nsaved upright_prior_results.json")
