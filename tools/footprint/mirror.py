#!/usr/bin/env python3
"""Stage 4: solve the hull's plane of bilateral symmetry.

A single view recovers only the front surface. For a bilaterally symmetric hull
the rest is recoverable by mirroring, and the mirror is also the measurement:
the plane that minimises the distance between the cloud and its reflection is
the symmetry plane, its residual says whether the hull is symmetric at all, and
its intersection with the ground plane gives the longitudinal axis.

The normal this returns is no longer only a mirroring axis: stage 6 takes it as
the lateral axis of the hull's reference frame (longitudinal = the cloud's
principal axis within the symmetry plane, up = lateral x longitudinal), because
only 1 of 14 hero images supports a trustworthy vanishing-point fit. A wrong
normal here silently rotates every recovered footprint, not just the mirrored
half.

MoGe-2 output is metric, so the affine scale/shift solve is off by default. It
stays available for a fallback to MoGe-1, whose point map is affine-invariant
in both scale and shift; solving only for scale would leave the cloud sheared.

    ~/moge-venv/bin/python -m tools.footprint.mirror <ship_id>
"""

import dataclasses

import numpy as np
from scipy.optimize import minimize
from scipy.spatial import cKDTree

from . import paths, pointmap

# `residual` is chamfer distance normalised by the sample's bounding-box
# diagonal (see `cost()` below), so this ceiling is a fraction of hull size,
# not an absolute distance -- it has to be, since ships range from fighters to
# haulers. But that normalisation divides by the OBSERVED bounding box, not
# the hull's true extent, so residual is partly a function of how much of the
# hull the camera actually saw -- which is exactly why this constant cannot be
# picked from synthetic data alone, however it's assembled.
#
# CALIBRATE ON ONE-VIEW CLOUDS, NEVER TWO-SIDED ONES. A fully two-sided
# synthetic hull (both halves present, as in `_symmetric_hull()`) is not what
# production ever sees, and it understates the residual: isotropic noise up to
# sigma=0.04 on a two-sided cloud tops out at ~0.011. But on `test_mirror.py`'s
# own one-view fixture (`_partially_visible()`: near half plus a sparse,
# noised sample of the far half) -- exactly symmetric by construction, no
# EXTRA noise added -- the residual is already ~0.013-0.014, i.e. AT OR OVER
# this ceiling before any asymmetry or additional cloud noise exists at all.
# That single fact -- not merely "real noise might be higher" -- fully
# explains why 3 real MoGe-2 clouds measured 0.0140-0.0163 with no asymmetry
# required (see the task report): partial coverage alone accounts for it.
#
# A genuinely one-sided (sponson-lopsided) hull sits at ~0.016-0.017 regardless
# of point density, so 0.013 is not defensible as a real/asymmetric separator
# once one-view coverage is accounted for -- the true gap is closer to 5%
# (0.0123 one-view-symmetric vs 0.017 lopsided) than the ~18% synthetic
# two-sided calibration implied. Do not retune this number here: Task 6b (the
# stage 4 rewrite to a silhouette-and-depth objective, since self-chamfer
# cannot find the PLANE on one-sided data at all -- see the design doc) is
# where the ceiling gets re-derived from real clouds, not synthetic ones.
RESIDUAL_CEILING = 0.013
_SUBSAMPLE = 4000


@dataclasses.dataclass
class Symmetry:
    normal: np.ndarray
    offset: float
    scale: float
    shift: float
    residual: float


def reflect(points: np.ndarray, normal: np.ndarray, offset: float) -> np.ndarray:
    n = normal / np.linalg.norm(normal)
    return points - 2.0 * ((points @ n) - offset)[:, None] * n


def _apply_affine(points, scale, shift):
    out = points.copy()
    out[:, 2] = out[:, 2] * scale + shift
    return out


# `shift` is NOT identifiable by self-reflection chamfer, for any normal, and
# no threshold on `sym.shift` can be a real check. Proof: adding shift to z
# translates the whole cloud (and, since it's applied before reflecting, its
# reflection too) by `shift` along z; `off` can absorb that translation
# exactly by moving to `off - shift * n_z`, and chamfer is translation
# invariant, so cost(n, off, shift) == cost(n, off - shift*n_z, 0) for every
# shift. (shift, offset) is a flat valley, not a point. Measured on
# `_rotated_symmetric_hull()` with refine_affine=True: init_shift 0.0 / 0.7 /
# -1.5 returns shift 0.000000 / 0.561740 / -1.205369, all at residual
# ~1e-8-1e-16 -- three different "answers", none of them wrong, because the
# objective cannot tell them apart. `test_affine_refinement_is_a_noop_on_metric_input`
# does not assert on `sym.shift` for this reason; the shift solve belongs to
# stage 5/6b's reprojection objective, which IS sensitive to a z-translation
# (a mirrored point's reprojected depth relative to the visible surface
# changes with shift, where chamfer against its own reflection does not).


def _sph(theta, phi):
    return np.array([np.sin(theta) * np.cos(phi), np.sin(theta) * np.sin(phi), np.cos(theta)])


def _chamfer(a, b):
    d1, _ = cKDTree(a).query(b, k=1)
    d2, _ = cKDTree(b).query(a, k=1)
    return float(np.mean(d1) + np.mean(d2)) / 2.0


def solve(points: np.ndarray, init_scale: float = 1.0, init_shift: float = 0.0,
          refine_affine: bool = False) -> Symmetry:
    rng = np.random.default_rng(0)
    idx = rng.choice(len(points), min(_SUBSAMPLE, len(points)), replace=False)
    sample = points[idx]
    centre = sample.mean(axis=0)
    extent = float(np.linalg.norm(sample.max(axis=0) - sample.min(axis=0)))

    # Initialise from the principal axes: for a bilaterally symmetric hull the
    # symmetry-plane normal is one of them, so try all three and keep the best.
    _, _, vt = np.linalg.svd(sample - centre, full_matrices=False)

    def cost(params):
        # `extent` is computed once, pre-affine, at :103, but divides a
        # POST-affine chamfer here -- the cost is therefore not scale
        # invariant. Recomputing extent post-affine does not fix this and
        # makes the scale-bound-attractor problem above worse (an init_scale
        # of 3.0 on the recovery-test fixture then runs to the 5.0 UPPER
        # bound instead of the correct 1.6667, residual 0.018), so it is left
        # as-is; the non-convexity this whole function works around is
        # intrinsic, not an artifact of this normalisation choice. It does
        # introduce a small monotone downward bias in the recovered scale
        # under cloud noise -- measured on `_rotated_symmetric_hull()` at
        # noise sigma 0.005/0.01/0.02/0.04: scale 0.9993/0.9982/0.9929/0.9553,
        # i.e. still within the no-op test's 0.05 tolerance at these levels,
        # but worth knowing about if a real cloud's noise turns out higher.
        theta, phi, off = params[:3]
        n = _sph(theta, phi)
        pts = _apply_affine(sample, params[3], params[4]) if refine_affine else sample
        return _chamfer(pts, reflect(pts, n, off)) / max(extent, 1e-9)

    # The affine cost is non-convex in scale and has attractors AT the bounds:
    # measured on the recovery test's own fixture (true scale 1/0.6=1.6667),
    # init_scale=0.5 or 4.5 converges to scale=0.2 (the lower bound) with
    # residual 0.0073 -- comfortably under RESIDUAL_CEILING, i.e. a 5x depth
    # collapse reported as trustworthy. This was also the direct cause of a
    # real flake: the recovery test failed once in a full-suite run landing on
    # scale=0.2 with the DEFAULT init_scale=1.0 and zero code changes, then
    # passed on rerun -- consistent with floating-point nondeterminism in the
    # SVD/BLAS path occasionally perturbing which basin a single-start L-BFGS-B
    # trajectory falls into on this non-convex surface. Multi-starting scale
    # over a fixed grid (in addition to the caller's own init_scale) gives the
    # correct basin more chances to win regardless of such perturbation, and a
    # result that terminates ON a bound is discarded outright below rather
    # than trusted as a real local optimum.
    # Verified load-bearing, by disabling the multi-start and running each init
    # alone against this same fixture (true scale 1.6667):
    #     init=0.5 -> scale=0.2000 cost=0.00729  ON BOUND
    #     init=1.0 -> scale=1.6667 cost=0.00000
    #     init=2.0 -> scale=1.6667 cost=0.00000
    #     init=3.0 -> scale=1.6667 cost=0.00000
    #     init=4.5 -> scale=0.2000 cost=0.00729  ON BOUND
    # Two of five single starts land on the bound at a cost under
    # RESIDUAL_CEILING, so the grid plus the bound-discard is what makes this
    # robust rather than either alone.
    #
    # 0.5 is retained deliberately even though it is itself a bound-lander here
    # and is therefore always discarded on THIS fixture: that it always fails is
    # a property of one synthetic hull, not a guarantee, and pruning a start
    # because it never wins on the only fixture we have is how a grid quietly
    # stops covering its range. Pruning it is a real option -- it would cut the
    # refine_affine cost measurably -- but it needs a test that pins which
    # starts matter, so it is deferred to Task 6b, which rewrites this search.
    scale_bounds = (0.2, 5.0)
    scale_starts = sorted({0.5, 1.0, 2.0, init_scale}) if refine_affine else [init_scale]

    best = None
    best_any = None  # fallback if every candidate terminates on a scale bound
    for axis in vt:
        theta = np.arccos(np.clip(axis[2], -1, 1))
        phi = np.arctan2(axis[1], axis[0])
        for scale0 in scale_starts:
            x0 = [theta, phi, float(centre @ axis), scale0, init_shift]
            bounds = [(0, np.pi), (-np.pi, np.pi), (None, None),
                      scale_bounds if refine_affine else (init_scale, init_scale),
                      (-extent, extent) if refine_affine else (init_shift, init_shift)]
            res = minimize(cost, x0, method="L-BFGS-B", bounds=bounds,
                           options={"maxiter": 120})
            if best_any is None or res.fun < best_any.fun:
                best_any = res
            on_bound = refine_affine and (res.x[3] <= scale_bounds[0] + 1e-9
                                           or res.x[3] >= scale_bounds[1] - 1e-9)
            if on_bound:
                continue
            if best is None or res.fun < best.fun:
                best = res
    best = best if best is not None else best_any

    theta, phi, off, scale, shift = best.x
    return Symmetry(normal=_sph(theta, phi), offset=float(off),
                    scale=float(scale), shift=float(shift), residual=float(best.fun))


def complete(points: np.ndarray, sym: Symmetry) -> np.ndarray:
    pts = _apply_affine(points, sym.scale, sym.shift)
    return np.vstack([pts, reflect(pts, sym.normal, sym.offset)])


def run(ship_id: str, cloud: pointmap.Cloud, refine_affine: bool = False) -> Symmetry:
    sym = solve(cloud.points, refine_affine=refine_affine)
    np.savez_compressed(paths.artifact_dir(ship_id) / "cloud_resolved.npz",
                        points=complete(cloud.points, sym),
                        normal=sym.normal, offset=sym.offset,
                        scale=sym.scale, shift=sym.shift, residual=sym.residual)
    return sym


def load(ship_id: str) -> tuple[Symmetry, np.ndarray]:
    """Read back `cloud_resolved.npz`, returning (sym, completed points).

    Without this, stages 5-7 would each hand-roll `np.load` against the six
    keys `run()` writes, unpinned by any test -- a rename here would break
    them silently. Mirrors `pointmap.load()`'s pattern one file over.
    """
    z = np.load(paths.artifact_dir(ship_id) / "cloud_resolved.npz")
    sym = Symmetry(normal=z["normal"], offset=float(z["offset"]),
                   scale=float(z["scale"]), shift=float(z["shift"]),
                   residual=float(z["residual"]))
    return sym, z["points"]
