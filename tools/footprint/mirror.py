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
# haulers. Calibrated on synthetic data (see test_mirror.py): a symmetric hull
# with isotropic Gaussian noise up to sigma=0.04 tops out at residual ~0.011,
# a genuinely one-sided (sponson-lopsided) hull sits at ~0.016-0.017 regardless
# of point density, so 0.013 sits in the gap with margin on both sides.
# CAUTION, measured directly against 3 real MoGe-2 clouds from keyable hero
# images (not yet used to retune this constant -- see test_mirror.py and the
# task report): outerrim_prayer 0.0163, smelter 0.0146, ledger 0.0140 -- all
# ABOVE this ceiling, clustering at or above the synthetic lopsided-hull
# number. Real MoGe noise is evidently higher than the synthetic model here
# assumes; Task 9 must re-validate against real clouds before this ceiling is
# trusted in production.
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


def _sph(theta, phi):
    return np.array([np.sin(theta) * np.cos(phi), np.sin(theta) * np.sin(phi), np.cos(theta)])


def _chamfer(a, b, tree_a=None):
    tree_a = tree_a or cKDTree(a)
    d1, _ = tree_a.query(b, k=1)
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
        theta, phi, off = params[:3]
        n = _sph(theta, phi)
        pts = _apply_affine(sample, params[3], params[4]) if refine_affine else sample
        return _chamfer(pts, reflect(pts, n, off)) / max(extent, 1e-9)

    best = None
    for axis in vt:
        theta = np.arccos(np.clip(axis[2], -1, 1))
        phi = np.arctan2(axis[1], axis[0])
        x0 = [theta, phi, float(centre @ axis), init_scale, init_shift]
        bounds = [(0, np.pi), (-np.pi, np.pi), (None, None),
                  (0.2, 5.0) if refine_affine else (init_scale, init_scale),
                  (-extent, extent) if refine_affine else (init_shift, init_shift)]
        res = minimize(cost, x0, method="L-BFGS-B", bounds=bounds,
                       options={"maxiter": 120})
        if best is None or res.fun < best.fun:
            best = res

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
