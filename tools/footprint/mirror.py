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
It exists on `solve` (two-sided input) only -- `solve_from_view` refuses it, see
that function's docstring.

`solve_from_view` is the one-view entry point, and how far to trust it is worth
stating precisely.

Silhouette agreement subject to a depth-separation floor -- the objective this
stage was originally specified with -- is DEGENERATE on its own: agreement
saturates at exactly 1.0000 for any plane that recedes the mirrored half clear
of the hull, above the true plane's 0.9969-0.9997, and the depth floor cannot
exclude it because such a plane has MORE separation than the truth. What makes
the search tractable is the second constraint, `_PROXIMITY_CEILING` -- the
mirrored half must land NEAR the visible surface, not merely behind it. Its
comment carries the calibration, the cheaper substitutes that were measured to
fail, and the obliquity range over which it holds.

With that constraint, axis error across the synthetic sweep is 0.18 / 1.74 /
8.19 / 3.40 degrees at obliquity 0.500 / 0.707 / 0.866 / 0.966. Three of those
four are sound and track their true obliquity closely (recovered 0.502 / 0.686 /
0.949). The 0.866 case does NOT: it returns a receded plane at obliquity 0.928,
tilted toward the camera, so it is a wrong kind of answer rather than a
near-miss, and `test_solve_from_view_recovers_the_plane_a_self_chamfer_solve_folds`
is a strict xfail recording exactly that. Both constraints also lose their
margins outside roughly 0.3 <= obliquity <= 0.9.

NO REPLACEMENT DISCRIMINATOR IS CURRENTLY KNOWN -- four were measured and
rejected, and the task report lists them with their numbers. Anything that
rewards the mirrored half for being entirely BEHIND the visible surface will be
won by the degeneracy, which is what it does best, and any candidate has to be
swept across obliquity rather than measured at one angle.

REAL-ART ACCURACY IS UNPROVEN. The five hero clouds measured in task 6b all
solve without reporting failure, at obliquities 0.574-0.856 that sit inside the
usable band, but the recovered normals are NOT yet mutually consistent across
ships. There is no ground truth on real art to settle it, and that inconsistency
is the strongest evidence available that the synthetic result does not
straightforwardly transfer. Treat a real solve as a measurement to be checked
downstream, not as an answer.

    ~/moge-venv/bin/python -m tools.footprint.mirror <ship_id>
"""

import dataclasses
import json

import numpy as np
from scipy.optimize import minimize
from scipy.spatial import cKDTree

from . import gate, paths, pointmap

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
# two-sided calibration implied.
#
# TASK 6B RE-DERIVED IT AND FOUND THAT ONE CONSTANT CANNOT SERVE BOTH SOLVERS,
# so this one is left at 0.013 for `solve` and `solve_from_view` is NOT gated on
# it. On a one-view sheet the residual at the TRUE plane is a steep function of
# obliquity, because the further the symmetry plane is turned toward the camera
# the less of the mirrored half has any visible surface to be near. Measured on
# test_mirror.py's `_one_sided_view` fixture -- exactly symmetric, zero noise,
# the correct plane supplied analytically -- residual at obliquity 0.500 / 0.707 /
# 0.866 / 0.966 is 0.0117 / 0.0282 / 0.0592 / 0.1111. The lowest of those is
# already at this ceiling and the highest is 8.5x over it, with no asymmetry
# anywhere.
#
# Five real MoGe-2 clouds measured 0.0344 / 0.0609 / 0.0903 / 0.0966 / 0.1007
# (comet / magnate / ledger / outerrim_prayer / smelter) at obliquity 0.839 /
# 0.574 / 0.587 / 0.856 / 0.635 -- every one of them inside the range a PERFECTLY
# symmetric hull produces at those angles, so they are not evidence of asymmetry
# and a ceiling drawn above them would separate nothing. A single number that
# admitted them (0.13, clearing the worst by 1.3x) would also admit the
# synthetic lopsided hull at 0.017 by 7.6x, i.e. it would stop being an asymmetry
# check at all -- and it would break
# `test_solve_reports_a_high_residual_on_an_asymmetric_hull`, which is a real
# test of a real property on the fixture it was calibrated for.
#
# The recommendation, NOT implemented here because it needs a working plane solve
# to calibrate against: gate the one-view path on residual measured against the
# obliquity-dependent baseline above (a ratio to the expected value at that
# obliquity), or drop residual as a gate for one-view clouds and rely on
# `gate.mirrored_pass`. Retuning this number was declined rather than done.
RESIDUAL_CEILING = 0.013
_SUBSAMPLE = 4000

# A floor on `Symmetry.obliquity`, gating the batch driver's stage-4 verdict
# (`run.process`'s `failed_low_obliquity`), not calibrated fresh here: it is
# the same 0.3 already documented on `Symmetry.obliquity` above ("below
# roughly 0.3 the depth term cannot distinguish a fold at all") and in
# `solve_from_view`'s own module docstring ("Both constraints also lose
# their margins outside roughly 0.3 <= obliquity <= 0.9"). A near-bow-on view
# can still clear every check `solve_from_view` runs on itself
# (`gate.MIR_FLOOR`, the depth floor, the proximity ceiling) while sitting in
# a regime those checks are independently documented not to discriminate in
# -- `sym.failure is None` is not the same claim as "trustworthy at this
# obliquity", and this constant is the difference between the two. Only a
# floor, not the 0.9 ceiling from the same comment: the populations above
# ~0.93 overlap on `mirrored_fraction` too (see `gate.MIR_FLOOR`'s comment),
# but that is the receded-plane degeneracy `_PROXIMITY_CEILING` exists to
# reject, not a property of high obliquity on its own, and the plan's own
# language ("a low-obliquity ship is a reconstruction failure") only names
# the low end.
OBLIQUITY_FLOOR = 0.3

# Scale multi-starts for `solve`'s affine refinement. A module constant only so
# a test can construct the all-bounds case (see the guard at the end of
# `solve`); it is not a knob.
_SCALE_STARTS = (0.5, 1.0, 2.0)

# --- `solve_from_view`'s search budget -------------------------------------
#
# Priced at `gate.inside_fraction`'s measured 0.333ms per call at _SUBSAMPLE
# points, plus one `gate.project` of the same sample for the depth term:
# _SEARCH_NORMALS * _SEARCH_OFFSETS = 3000 coarse evaluations is ~2s per ship,
# and the refinement adds ~0.6s. Speed is not the binding constraint here.
#
# _SEARCH_NORMALS sets the coarse angular resolution: 200 points on a
# hemisphere is ~11 degrees apart, which is coarser than the 8-degree accuracy
# the recovery test demands -- the coarse stage only has to land in the right
# BASIN and the Nelder-Mead refinement closes the rest. Verified that it does:
# instrumenting the seed list on the obliquity-0.866 fixture, the best of the
# 3000 coarse candidates is 2.4 degrees from the true plane and refines to 2.4
# degrees. The search still returns a wrong answer there, but not for want of
# resolution -- a rival basin wins the objective (see `_PROXIMITY_CEILING`), so
# raising these numbers does not help.
_SEARCH_NORMALS = 200
_SEARCH_OFFSETS = 15
# Refine the best candidate from each of this many DISTINCT basins, where
# distinct means normals at least _BASIN_SEPARATION_DEG apart. Refining the top N
# raw candidates instead refines N neighbours of ONE peak and never visits the
# competing peaks -- measured on the obliquity-0.500 and 0.966 fixtures, ranking
# the coarse pass without this de-duplication filled every slot with a variant of
# the same receded plane and never refined the true basin at all (axis error 68.6
# and 32.4 degrees).
_REFINE_BASINS = 6
_BASIN_SEPARATION_DEG = 15.0
_DEPTH_SUBSAMPLE = 40_000

# --- What the search maximises, and the two constraints it needs -----------
#
# "Maximise inside_fraction subject to the depth constraint" -- the objective
# this task was specified with -- is DEGENERATE on its own, measured. The
# published evidence for it compared candidate planes at a FIXED offset through
# the cloud's centroid, where the true plane reads 0.998 and wrong planes
# 0.75-0.93. Let the offset move and that separation evaporates: because
# `uv = K p / p_z`, moving the plane away from the camera makes the reflection
# RECEDE, which SHRINKS its projection toward the principal point until every
# mirrored point lands strictly inside the matte and `inside_fraction` is exactly
# 1.0000 -- above the true plane, which grazes the silhouette rim and measures
# 0.9969-0.9997.
#
# It is not one rival plane but a whole family: any normal with a z-component can
# recede. Running a local maximisation of `inside_fraction` (with the depth
# constraint active, no proximity constraint) from seeds 5/10/15/20 degrees off
# the true plane at view obliquity 0.866, every one reaches inside_fraction
# 1.0000 and STOPS 3.5 / 8.8 / 13.1 / 18.0 degrees away from the truth. The depth
# term cannot exclude any of it: a receded plane has MORE depth separation than
# the true plane (measured 1.14 of z-extent against 0.357), which is the same
# trap the brief's own 30-degrees-off candidate sets.
#
# Two other objectives were measured and are worse, so this is not a matter of
# picking a different single score:
#   - MINIMISE chamfer (`solve`'s objective): its optimum is the tangential fold,
#     0.0048 of extent against the true plane's 0.0146-0.1137. That is this
#     task's premise.
#   - MAXIMISE the SHARPNESS of the silhouette peak (agreement at the plane minus
#     the mean over six 5-degree/0.1-z-extent perturbations of it), which does
#     separate the true plane from a receded one by 5-60x. Implemented and swept:
#     axis error 6.1 / 21.4 / 21.0 / 32.5 degrees at view 30 / 45 / 60 / 75,
#     because folds are sharp too -- an exact symmetry of the visible sheet is
#     the sharpest optimum there is -- and the depth floor, calibrated to reject
#     only an EXACT fold, admits near-folds that then win on sharpness.
#
# What the receded family cannot fake is PROXIMITY. A reflection across a genuine
# symmetry plane maps the hull onto itself, so mirrored points must land ON the
# hull the visible surface belongs to -- roughly a beam's width behind it. A
# receded plane translates the whole sheet clear of the hull and lands nowhere
# near it. So the search is
#
#     MAXIMISE inside_fraction
#     SUBJECT TO depth_separation >= MIN_DEPTH_SEPARATION_FRACTION * z_extent
#            AND mean distance from a mirrored point to the nearest visible
#                point <= _PROXIMITY_CEILING * extent
#
# and deliberately NOT a weighted sum of the two: a weighted sum lets a large
# depth separation buy a worse silhouette fit, which is exactly what a receded
# plane would do.
#
# The proximity quantity is the one-sided form of the chamfer already reported as
# `residual`, not a new metric.
#
# CHEAPER SUBSTITUTES WERE TRIED AND MEASURED WRONG. Bounding depth_separation
# from ABOVE (<= 0.70 of z-extent) is free -- the depth term already computes it
# -- and it does exclude the classic receded plane, whose separation is 1.11-1.16
# of z-extent. It admits a different wrong family: swept over views 30/45/60/75
# it returned planes 68.6 / 66.0 / 65.8 / 3.4 degrees off, at separations of
# 0.50-0.60 of z-extent -- inside the band, because those planes displace the
# sheet LATERALLY as much as in depth, which a depth difference cannot see and a
# 3D distance can (the 65.8-degree winner measures 0.115 of extent against the
# true plane's 0.061, so the proximity ceiling rejects it while the depth ceiling
# does not). Requiring the plane to CUT the sheet (>= 15% of points on its
# smaller side, true plane 0.23-0.49 against a receded plane's 0.05-0.10) is
# likewise free and likewise insufficient: those same winners cut it (0.22-0.36).
# Both are kept out of the code rather than kept as harmless extra constraints,
# because a constraint that never changes an outcome is a constant nobody can
# calibrate.
#
# CALIBRATION, and its limit. Measured on the one-view fixture, mean
# mirrored-to-visible distance / extent, for the true plane against the receded
# family (offset a fraction q of the way through the cloud's own span along the
# mean view direction):
#
#   view obliquity      0.500   0.707   0.866   0.966   0.996
#   TRUE plane         0.0146  0.0315  0.0610  0.1137  0.1446
#   tangential fold    0.0048  0.0050  0.0048  0.0048  0.0049
#   receded q=0.90     0.1590  0.1467  0.1457  0.1258  0.1038
#   receded q=0.95     0.1794  0.1663  0.1675  0.1465  0.1232
#   receded q=1.05     0.2238  0.2116  0.2157  0.1929  0.1670
#
# 0.10 clears the worst true plane it must admit (0.0610 at obliquity 0.866) by
# 1.6x and the closest receded plane it must reject (0.1258) by 1.3x. The
# tangential fold is far INSIDE the ceiling -- it is the depth floor's job, not
# this one's.
#
# THE ANSWER IS SENSITIVE TO THIS CONSTANT IN A WAY THAT SHOULD NOT BE TUNED
# AWAY, and that is the clearest single sign the objective is wrong rather than
# merely mis-parameterised. Sweeping it against the recovery fixture, axis error
# at view 30 / 45 / 60 / 75 degrees:
#
#   ceiling 0.07    0.18   8.74   4.74  30.78
#   ceiling 0.08    0.18  13.87   6.58  30.78
#   ceiling 0.09    0.18   1.74   7.78   3.41
#   ceiling 0.10    0.18   1.74   8.19   3.40
#
# 0.09 puts all four views under the recovery test's 8-degree threshold and would
# turn that test green. It was NOT adopted: the value the calibration table above
# supports is 0.10, the 0.09 result at view 60 is still a receded plane (obliquity
# 0.926 against a true 0.866, depth separation 0.50 of z-extent against 0.357),
# and the 30-degree swings between adjacent ceiling values are noise from which
# rival basin happens to win, not a trend. A constant chosen because it makes one
# assertion pass is not calibrated.
#
# THE POPULATIONS OVERLAP ABOVE OBLIQUITY ~0.93 and this ceiling cannot separate
# them there: at obliquity 0.966 the true plane reads 0.1137 against a receded
# plane's 0.1258 (1.1x), and at 0.996 the order INVERTS (0.1446 against 0.1038).
# That is not a number to split. A near-broadside view sees one flank only, and a
# plane just behind that flank IS very nearly the right answer, so the two
# hypotheses genuinely converge. It is the mirror image of the depth FLOOR's limit
# at low obliquity, and it is why `obliquity` is reported per ship: outside
# roughly 0.3 <= obliquity <= 0.9 this solve is not trustworthy, and the fix is a
# different view, not a looser constant.
#
# `inside_fraction` itself is also checked once, at the end, against
# `gate.MIR_FLOOR`: a plane inside both bounds whose mirrored half still does not
# land in the silhouette is not a symmetry plane. That is a validity check on the
# answer, not part of what the search maximises.
_PROXIMITY_CEILING = 0.10
# Mirrored points queried against the visible sample's KD-tree per proximity
# evaluation. The full 4000 costs ~37ms, which at 4000 candidates is 2.5 minutes
# per ship; 500 is ~5ms and the quantity is a MEAN, so 500 samples estimate it to
# well inside the 1.3x margin above. The coarse pass skips proximity entirely
# (see the call site) and only the refinement pays for it.
_PROXIMITY_SAMPLE = 500


@dataclasses.dataclass
class Symmetry:
    normal: np.ndarray
    offset: float
    scale: float
    shift: float
    residual: float
    # Mean of (mirrored z - visible z) at pixels where both reproject, in cloud
    # units. A genuine occluded half sits BEHIND the visible surface (positive);
    # a fold -- a plane that maps the one-view sheet onto itself -- sits at the
    # same depth. `solve_from_view` measures it; `solve` cannot (it never sees a
    # camera), so it leaves the 0.0 default, which `gate.run` reads as an
    # unevaluated gate rather than a pass.
    depth_separation: float = 0.0
    # Non-None means DO NOT TRUST this result: the solve ran to completion but
    # its own diagnostics say the answer is not usable. Surfaced to
    # `run.process` and recorded in quality.json by `run()` below. A field
    # rather than an exception because the measured numbers are worth recording
    # for the failing ship, not just the fact of failure.
    failure: str | None = None

    @property
    def obliquity(self) -> float:
        """`abs(normal . [0, 0, 1])` -- how much the lateral axis points at the camera.

        NOT decoration: the achievable `depth_separation` is a steep function of
        it, so `depth_separation` is uninterpretable without it. Measured on a
        back-face-culled sheet, true-plane depth_separation / z-extent against
        obliquity: 0.045 at 0.342, 0.094 at 0.500, 0.204 at 0.707, 0.357 at
        0.866, 0.654 at 0.966, 0.971 at 0.996. At obliquity 0 the symmetry plane
        contains the view direction, reflection moves nothing in depth, and the
        separation is EXACTLY zero -- so below roughly 0.3 the depth term cannot
        distinguish a fold at all, and that is a statement about which ships
        this pipeline can handle, not a threshold to lower.

        A read-only property, not a stored field, so it can never drift out of
        agreement with `normal` (e.g. after `dataclasses.replace`).
        """
        n = np.asarray(self.normal, dtype=float)
        return float(abs(n[2] / max(np.linalg.norm(n), 1e-12)))


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
    scale_starts = sorted({*_SCALE_STARTS, init_scale}) if refine_affine else [init_scale]

    best = None
    best_any = None  # only ever returned WITH a `failure` set -- see below
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

    # An all-bounds outcome is a REPORTED FAILURE, not a silent fallback. The
    # previous `best = best if best is not None else best_any` handed back
    # exactly what the bound-discard above exists to reject: measured on
    # `_rotated_symmetric_hull` squashed by 0.6 (true scale 1.6667), with 0.5 as
    # the only scale start, all three SVD axes terminate on the 0.2 lower bound
    # and the fallback returns residual 0.0073 -- UNDER RESIDUAL_CEILING, i.e. a
    # 5x depth collapse reported as trustworthy. The result is still returned
    # (its numbers are worth recording) but carries `failure`, so no caller can
    # read it as a pass. See
    # `test_solve_reports_failure_when_every_scale_lands_on_a_bound`.
    failure = None
    if best is None:
        best = best_any
        failure = (f"every candidate terminated on a scale bound "
                   f"{scale_bounds}: scale={best.x[3]:.6f} at residual "
                   f"{best.fun:.5f} is a bound artifact, not a local optimum, "
                   f"and the residual understates the error")

    theta, phi, off, scale, shift = best.x
    return Symmetry(normal=_sph(theta, phi), offset=float(off),
                    scale=float(scale), shift=float(shift), residual=float(best.fun),
                    failure=failure)


def _angle_deg(a: np.ndarray, b: np.ndarray) -> float:
    """Unsigned angle between two plane normals, in [0, 90] degrees."""
    return float(np.degrees(np.arccos(min(1.0, abs(float(a @ b))))))


def _hemisphere(n: int) -> np.ndarray:
    """`n` roughly equidistant directions on the upper hemisphere.

    A hemisphere rather than a sphere because `reflect` depends only on the
    PLANE: `(n, off)` and `(-n, -off)` give the same reflection, so the lower
    half would only duplicate work.
    """
    i = np.arange(n) + 0.5
    z = i / n
    r = np.sqrt(np.maximum(0.0, 1.0 - z * z))
    ga = np.pi * (3.0 - np.sqrt(5.0))            # golden-angle spiral
    return np.column_stack([r * np.cos(ga * i), r * np.sin(ga * i), z])


def _visible_depth(points: np.ndarray, intrinsics: np.ndarray, shape) -> np.ndarray:
    """Per-pixel depth of the visible surface, nan where nothing projects.

    MEAN z of the points landing on a pixel, not the front-most and not
    last-write-wins. The choice is not cosmetic -- it is what the fold's
    signature depends on. Measured on the one-view fixture at obliquity 0.866,
    tangential fold vs true plane as a fraction of z-extent:
    mean 0.00015 / 0.331, last-write 0.00005 / 0.332, front-most (min z)
    0.00824 / 0.341. Front-most inflates the fold to within 1.2x of
    `gate.MIN_DEPTH_SEPARATION_FRACTION` -- it biases the comparison positive,
    because a mirrored point is scored against the NEAREST visible point at its
    pixel rather than the one it came from. Last-write-wins matches
    test_gate.py's helper and separates as well, but depends on point order.
    Mean is order-independent and unbiased, so it is what this uses.
    """
    h, w = shape
    uv, ok = gate.project(points, intrinsics, shape)
    flat = uv[ok, 1] * w + uv[ok, 0]
    total = np.bincount(flat, weights=points[ok, 2], minlength=h * w)
    count = np.bincount(flat, minlength=h * w)
    with np.errstate(invalid="ignore"):
        depth = np.where(count > 0, total / np.maximum(count, 1), np.nan)
    return depth.reshape(h, w)


def _depth_separation(depth: np.ndarray, mirrored: np.ndarray,
                      intrinsics: np.ndarray, shape) -> tuple[float, int]:
    """Mean of (mirrored z - visible z) over pixels where both land.

    Returns `(separation, shared_pixels)`. Zero separation on no overlap: a
    candidate whose reflection shares no pixel with the visible surface has no
    evidence of being behind it, and an unevaluated depth term must never read
    as a pass.
    """
    uv, ok = gate.project(mirrored, intrinsics, shape)
    vis = depth[uv[ok, 1], uv[ok, 0]]
    shared = np.isfinite(vis)
    if not shared.any():
        return 0.0, 0
    return float(np.mean(mirrored[ok, 2][shared] - vis[shared])), int(shared.sum())


def solve_from_view(points: np.ndarray, mask: np.ndarray, intrinsics: np.ndarray,
                    refine_affine: bool = False, init_scale: float = 1.0,
                    init_shift: float = 0.0) -> Symmetry:
    """Solve the symmetry plane of a ONE-VIEW cloud, against its own silhouette.

    `solve` minimises chamfer between the cloud and its reflection, which is
    provably the wrong objective for a single view: a one-view point map is a
    front-surface SHEET, and the reflection that best matches a sheet is the one
    that folds the sheet onto itself. The true bilateral plane scores WORSE,
    because it maps visible points onto the occluded half where there is nothing
    to match. Measured on this file's one-view fixture, `solve` returns a plane
    82.6 degrees from the truth at chamfer 0.0048 where the true plane measures
    0.0592 -- an order of magnitude worse cost at the right answer.

    So this scores against the stage 1 matte instead: it maximises silhouette
    agreement subject to the mirrored half sitting BEHIND the visible surface and
    NEAR it. The depth floor excludes the fold; the proximity ceiling excludes a
    plane that simply pushes the sheet clear of the hull, which is the degeneracy
    silhouette agreement has on its own -- it reaches exactly 1.0000 there, above
    the true plane. See the comment on `_PROXIMITY_CEILING` above for the
    measurements behind both, for the cheaper substitutes that were measured to
    fail, and for the obliquity range in which this holds.

    `solve` is KEPT for two-sided input (the synthetic ground-truth chain), where
    self-chamfer is correct and has an exact zero at the answer.

    `refine_affine` is accepted for signature parity with `solve` and REFUSED.
    The affine solve belongs here in principle -- reprojection under perspective
    is sensitive to a z-translation where self-chamfer is provably not (see
    `_apply_affine`), so this is the only objective that can identify `shift` at
    all. But it cannot be calibrated on top of a plane search that does not yet
    find the plane: a shift-recovery measurement would be measuring the plane
    error, not the shift. Implementing it against an objective that is being
    replaced would also mean writing the expensive part twice -- the affine moves
    the visible surface, so every evaluation has to rebuild the depth map, which
    is ~20x the cost of a fixed-affine evaluation. Deferred with the plane search.
    """
    if refine_affine:
        raise NotImplementedError(
            "solve_from_view cannot solve the affine yet: the shift solve needs a "
            "plane search that finds the plane, and this one does not (see "
            "test_solve_from_view_recovers_the_plane_a_self_chamfer_solve_folds). "
            "Use solve(refine_affine=True) on two-sided input, where the scale is "
            "identifiable and the shift provably is not.")
    shape = mask.shape
    rng = np.random.default_rng(0)
    idx = rng.choice(len(points), min(_SUBSAMPLE, len(points)), replace=False)
    sample = points[idx]
    extent = float(np.linalg.norm(sample.max(axis=0) - sample.min(axis=0)))
    z_extent = float(points[:, 2].max() - points[:, 2].min())
    floor = gate.MIN_DEPTH_SEPARATION_FRACTION * z_extent

    # The depth map is built from a denser subsample than the plane search
    # scores against: a mean-z map over only _SUBSAMPLE points covers ~4000
    # pixels of a ~26000-pixel silhouette, so most mirrored points would land on
    # an empty pixel and the shared-pixel count -- and with it the separation --
    # would be a sampling artifact. At _DEPTH_SUBSAMPLE the map is dense enough
    # that the count reflects geometry (measured 19651 shared pixels for the true
    # plane on the one-view fixture, against ~2900 from a 4000-point map).
    depth_points = points[rng.choice(len(points), min(_DEPTH_SUBSAMPLE, len(points)),
                                     replace=False)]
    depth = _visible_depth(depth_points, intrinsics, shape)
    tree = cKDTree(sample)

    def score(theta, phi, off, proximity=True):
        """(feasible, inside_fraction, depth_separation, proximity) for one plane.

        `feasible` means the depth separation clears the floor AND the mirrored
        half lands within `_PROXIMITY_CEILING` of the visible surface.

        `proximity=False` skips the nearest-neighbour query and reports proximity
        as 0.0 -- i.e. treats that constraint as satisfied. Used by the coarse
        pass only, where the query would dominate the cost 50:1 and where the
        depth term alone is enough to rank the basins (measured: the true basin
        is the best of 3000 coarse candidates on the depth-only cost at every
        view angle swept). Every result that can WIN is re-scored with proximity
        on.
        """
        n = _sph(theta, phi)
        mirrored = reflect(sample, n, off)
        sep, _ = _depth_separation(depth, mirrored, intrinsics, shape)
        near = 0.0
        if proximity:
            probe = mirrored[:: max(1, len(mirrored) // _PROXIMITY_SAMPLE)]
            near = float(tree.query(probe)[0].mean()) / max(extent, 1e-9)
        feasible = sep >= floor and near <= _PROXIMITY_CEILING
        return feasible, gate.inside_fraction(mirrored, intrinsics, mask), sep, near

    def cost(x, proximity=True):
        """Lexicographic barrier: no infeasible point can beat a feasible one.

        Feasible costs live in [-1, 0] (minus the silhouette agreement);
        infeasible ones start at 1.0 and grow with the constraint violation, so
        the optimiser is pushed back toward feasibility rather than wandering a
        flat penalty, while never being able to buy a better cost with more depth
        separation or a tighter chamfer.
        """
        feasible, frac, sep, near = score(*x[:3], proximity=proximity)
        if not feasible:
            return (1.0 + max(0.0, floor - sep) / max(floor, 1e-12)
                    + max(0.0, near - _PROXIMITY_CEILING) / _PROXIMITY_CEILING)
        return -frac

    # Coarse pass: every hemisphere direction, and for each of them offsets
    # across the cloud's own span along it, widened 10% because the true plane
    # can sit just outside the VISIBLE span at near-broadside views (measured
    # q = -0.05 of the span at 89 degrees, where only one flank is visible).
    coarse = []
    for n in _hemisphere(_SEARCH_NORMALS):
        theta = np.arccos(np.clip(n[2], -1.0, 1.0))
        phi = np.arctan2(n[1], n[0])
        proj = sample @ n
        pad = 0.1 * (proj.max() - proj.min())
        for off in np.linspace(proj.min() - pad, proj.max() + pad, _SEARCH_OFFSETS):
            x = [theta, phi, float(off)]
            coarse.append((cost(x, proximity=False), n, x))
    coarse.sort(key=lambda row: row[0])

    # One seed per BASIN, not the top N rows: the top rows of a grid this fine
    # are all neighbours of one peak, and the point of multi-starting is to visit
    # the competing peaks.
    seeds = []
    for row in coarse:
        if all(_angle_deg(row[1], kept[1]) > _BASIN_SEPARATION_DEG for kept in seeds):
            seeds.append(row)
        if len(seeds) == _REFINE_BASINS:
            break

    results = []
    for _, _, (theta, phi, off) in seeds:
        res = minimize(cost, [theta, phi, off],
                       method="Nelder-Mead",
                       options={"xatol": 1e-4, "fatol": 1e-6, "maxiter": 400,
                                "adaptive": True})
        results.append((res.fun, res.x))

    _, x = min(results, key=lambda r: r[0])
    feasible, frac, sep, near = score(*x[:3])
    chamfer = _chamfer(sample, reflect(sample, _sph(x[0], x[1]), x[2])) / max(extent, 1e-9)

    failure = None
    if not feasible:
        failure = (f"no candidate plane satisfied both constraints: best has "
                   f"depth_separation {sep:.4f} against a floor of "
                   f"{gate.MIN_DEPTH_SEPARATION_FRACTION} * z_extent = {floor:.4f} "
                   f"and mirrored-to-visible distance {near:.4f} of extent against "
                   f"a ceiling of {_PROXIMITY_CEILING}, at inside_fraction "
                   f"{frac:.4f} and obliquity {abs(_sph(x[0], x[1])[2]):.3f}. A "
                   f"depth failure means every candidate is a fold, which is what a "
                   f"view too close to bow-on looks like; a proximity failure means "
                   f"the view is too close to broadside for either bound to "
                   f"separate a real occluded half from a receded one")
    elif frac < gate.MIR_FLOOR:
        # A plane satisfying both constraints whose mirrored half still does not
        # land in the silhouette is not a symmetry plane. Checked once, on the
        # answer; it is not what the search maximises against.
        failure = (f"inside_fraction={frac:.4f} is below gate.MIR_FLOOR="
                   f"{gate.MIR_FLOOR}: the mirrored half does not reproject into "
                   f"the silhouette, so this plane is not a symmetry plane however "
                   f"well it satisfies the other two constraints")

    # `residual` stays the chamfer, normalised the same way `solve` normalises
    # it, so the two solvers' residuals remain comparable and RESIDUAL_CEILING
    # keeps one meaning. Chamfer is still the right measure of HOW SYMMETRIC the
    # hull is once the plane is known; it is only the search it cannot drive.
    return Symmetry(normal=_sph(x[0], x[1]), offset=float(x[2]),
                    scale=float(init_scale), shift=float(init_shift),
                    residual=float(chamfer), depth_separation=float(sep),
                    failure=failure)


def complete(points: np.ndarray, sym: Symmetry) -> np.ndarray:
    pts = _apply_affine(points, sym.scale, sym.shift)
    return np.vstack([pts, reflect(pts, sym.normal, sym.offset)])


def run(ship_id: str, cloud: pointmap.Cloud, mask: np.ndarray | None = None,
        refine_affine: bool = False) -> Symmetry:
    """Solve, write `cloud_resolved.npz`, and record the diagnostics.

    `mask` routes the solve: with a matte (every real ship) this is a one-view
    cloud and goes to `solve_from_view`; without one (the synthetic two-sided
    ground-truth chain) it goes to `solve`. There is no default that is right for
    both -- a one-view cloud handed to `solve` folds, and a two-sided cloud has
    no silhouette to score against.

    `obliquity`, `depth_separation` and `failure` land in quality.json here
    rather than in `gate.run`, which cannot see them: `gate.run` merges into the
    same file, so both stages' keys survive. Without this the failure a
    `Symmetry` reports would exist only in memory.
    """
    sym = (solve(cloud.points, refine_affine=refine_affine) if mask is None else
           solve_from_view(cloud.points, mask, cloud.intrinsics, refine_affine=refine_affine))
    np.savez_compressed(paths.artifact_dir(ship_id) / "cloud_resolved.npz",
                        points=complete(cloud.points, sym),
                        normal=sym.normal, offset=sym.offset,
                        scale=sym.scale, shift=sym.shift, residual=sym.residual,
                        depth_separation=sym.depth_separation,
                        failure=sym.failure if sym.failure is not None else "")

    p = paths.artifact_dir(ship_id) / "quality.json"
    data = json.loads(p.read_text()) if p.exists() else {}
    data.update({"symmetry_residual": sym.residual,
                 "depth_separation": sym.depth_separation,
                 "obliquity": sym.obliquity,
                 "symmetry_failure": sym.failure})
    p.write_text(json.dumps(data, indent=2))
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
                   residual=float(z["residual"]),
                   depth_separation=float(z["depth_separation"]),
                   failure=str(z["failure"]) or None)
    return sym, z["points"]
