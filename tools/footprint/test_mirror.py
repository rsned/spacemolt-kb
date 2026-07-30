"""Checks for the mirror-constrained symmetry solve.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_mirror.py
"""

import importlib
import pathlib
import sys

import dataclasses

import numpy as np
from scipy.spatial import cKDTree

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")


mirror = _load("mirror")


def _chamfer(a, b):
    """Symmetric mean nearest-neighbour distance between two clouds."""
    return 0.5 * (cKDTree(b).query(a)[0].mean() + cKDTree(a).query(b)[0].mean())


def _symmetric_hull(seed=0, n=4000):
    """A tapered, bilaterally symmetric hull: mirror plane normal +Y, offset 0.

    Deliberately NOT a uniform box. A uniform box is symmetric about all three
    coordinate planes, so the solver could return +X or +Z and be right. Worse,
    an "asymmetric" variant built by breaking only Y would leave Z an exact
    symmetry plane and the solver would correctly find it: measured on the box
    version, Z residual 0.069 vs Y 0.183 at n=4000, and the Z figure is pure
    sampling noise that falls monotonically with density (0.135 at n=500 ->
    0.025 at n=80000). A residual-above-ceiling assertion on that fixture would
    be testing cloud density, not asymmetry.

    The x-dependent taper breaks X symmetry and the x-dependent keel offset
    breaks Z symmetry, leaving Y the unique answer. Measured directly against
    `mirror.solve()`'s actual (extent-normalised) residual field at n=4000:
    Y=0.0000 exactly, X=0.025, Z=0.018, and Y stays exact as density changes.
    (An earlier draft of this fixture's numbers -- X=0.119, Z=0.117 -- was
    raw, un-normalised chamfer, off by the ~4.68 bounding-box-diagonal factor
    that `mirror.solve()`'s `cost()` divides by; every calibration number in
    this file was re-measured against the actual `Symmetry.residual` field,
    not a hand-rolled chamfer, after that mismatch surfaced as 4 failing
    tests against the reference implementation.)
    """
    rng = np.random.default_rng(seed)
    x = rng.uniform(-2.0, 2.0, n // 2)
    halfwidth = 0.15 + 0.85 * (1.0 - (x + 2.0) / 4.0)   # wide at the nose
    y = 0.05 + halfwidth * rng.uniform(0.0, 1.0, n // 2)
    z = rng.uniform(-0.5, 0.5, n // 2) + 0.4 * (x / 2.0) ** 2   # keel
    half = np.column_stack([x, y, z])
    return np.vstack([half, half * [1, -1, 1]])


def test_solve_finds_the_known_symmetry_plane():
    pts = _symmetric_hull()
    sym = mirror.solve(pts)
    axis_err = np.degrees(np.arccos(min(1.0, abs(sym.normal @ [0, 1, 0]))))
    assert axis_err < 5.0, axis_err
    assert abs(sym.offset) < 0.05, sym.offset
    assert sym.residual < mirror.RESIDUAL_CEILING, sym.residual


def _lopsided_hull(seed=0, n=4000):
    """The symmetric hull plus a sponson attached to the +Y side only.

    Do NOT build this by shoving one side along an axis: `lop[lop[:,1]>0, 0] +=
    1.2` leaves Z an exact symmetry plane, so the solver correctly returns Z
    with a LOW residual and the asymmetry assertion fails on correct code.
    Adding mass to one side leaves no candidate plane: measured against
    `mirror.solve()`'s own residual field (extent-normalised, not raw chamfer
    -- see the note in `_symmetric_hull`), the best plane found is 0.017 /
    0.017 / 0.017 at n = 2000 / 4000 / 20000 (seed 0), confirmed against a
    5200-plane brute-force grid search at n=4000 (best 0.0176, same axis) and
    stable across seeds 0-5 (0.016-0.017). That is driven by shape, not
    sampling density -- it does not fall as n grows, unlike the box-fixture Z
    artifact above -- but it sits much closer to the noisy-symmetric-hull
    ceiling than the first draft's numbers implied (0.094/0.094/0.085, which
    were the same raw-chamfer-units error described above); see
    RESIDUAL_CEILING in mirror.py for the corrected calibration.
    """
    pts = _symmetric_hull(seed, n)
    rng = np.random.default_rng(seed + 99)
    m = n // 8
    sponson = np.column_stack([rng.uniform(-0.5, 0.9, m),
                               rng.uniform(1.0, 1.8, m),
                               rng.uniform(-0.3, 0.3, m)])
    return np.vstack([pts, sponson])


def test_solve_reports_a_high_residual_on_an_asymmetric_hull():
    # Scrap-built hulls are deliberately lopsided; the residual is how we know
    # not to trust the mirrored half.
    sym = mirror.solve(_lopsided_hull())
    assert sym.residual > mirror.RESIDUAL_CEILING, sym.residual


def test_residual_ceiling_tolerates_a_noisy_symmetric_hull():
    """A symmetric hull with measurement noise must stay UNDER the ceiling.

    Without this, RESIDUAL_CEILING is only ever shown to separate 0.0 from
    0.017 — and the exact-zero case is an artifact of the fixture mirroring
    points exactly, which no real point map does. Measured against
    `mirror.solve()`'s actual residual field, Y residual on the symmetric hull
    at noise sigma 0.005 / 0.01 / 0.02 / 0.04 is 0.0024 / 0.0047 / 0.0080 /
    0.0110 (stable across 7 seeds at sigma=0.04: 0.0109-0.0111), against a
    lopsided hull's 0.016-0.017. RESIDUAL_CEILING=0.013 sits in that gap.
    (An earlier draft of this docstring reported 0.011 / 0.022 / 0.038 / 0.054
    against a lopsided hull's 0.094 and a ceiling of 0.06 -- those were raw,
    un-normalised chamfer numbers, ~4.68x the actual `Symmetry.residual`
    values on this fixture; see the note in `_symmetric_hull` for how the
    mismatch surfaced.)
    CARRY THIS INTO TASK 9: measured directly against 3 real MoGe-2 clouds
    (see the task report), residuals were 0.014-0.016 -- ABOVE this ceiling
    and above the synthetic lopsided-hull number. Real MoGe noise is
    evidently higher than this synthetic Gaussian model produces, so
    re-validate the ceiling against real clouds there. This test proves the
    mechanism (noise apart from asymmetry), not that 0.013 is the right
    number for real art.
    """
    pts = _symmetric_hull()
    noisy = pts + np.random.default_rng(7).normal(0, 0.02, pts.shape)
    sym = mirror.solve(noisy)
    assert sym.residual < mirror.RESIDUAL_CEILING, sym.residual


def test_reflect_is_an_involution():
    pts = _symmetric_hull()
    n, d = np.array([0.0, 1.0, 0.0]), 0.3
    once = mirror.reflect(pts, n, d)
    twice = mirror.reflect(once, n, d)
    assert np.allclose(twice, pts, atol=1e-9)


def _partially_visible(seed=0, n=4000, far_frac=0.25):
    """The near half plus a sparse sample of the far half, as a single view would.

    The brief's original fixture here was `pts[pts[:, 1] > 0]` -- a literal
    disjoint half-space cut with ZERO far-side points. That is information-
    theoretically unrecoverable by `solve()`'s self-reflection-chamfer method,
    not merely a hard case: reflecting the visible half across the TRUE plane
    (offset 0) produces zero overlap with itself (self-chamfer 0.095,
    extent-normalised), while folding that SAME half through its own interior
    midpoint produces near-perfect self-overlap (self-chamfer 0.025 at
    offset~0.29) -- a strictly LOWER cost at the WRONG plane. Verified by a
    60-point offset sweep along the y-axis alone (true offset=0 gives 0.095,
    the sweep's minimum is 0.025 at offset=0.29) and confirmed `solve()` does
    in fact return that wrong interior fold (residual 0.0126, chamfer-to-truth
    0.1126) on the literal cut. No implementation of "minimise self-reflection
    chamfer" can find the true plane there; the signal simply is not present
    in an exactly-one-sided cloud.

    A real photographed hull is not literally binary like that: foreshortening
    thins the far side's point density near the grazing viewing angle but does
    not erase it entirely. Keeping a random far_frac of the far half models
    that. Measured (seed 0): axis error stays 0.0 degrees and chamfer-to-truth
    stays 0.0000 for far_frac >= 0.25 (0.20 already gets close: axis error 1.9
    degrees, chamfer 0.031); at far_frac=0.15 the solver locks onto the wrong
    SVD starting axis entirely (axis error 90.0 degrees, chamfer 0.041).
    far_frac=0.25 keeps clear margin above that break point.
    """
    pts = _symmetric_hull(seed, n)
    near = pts[pts[:, 1] > 0]
    far = pts[pts[:, 1] <= 0]
    rng = np.random.default_rng(seed + 501)
    keep = rng.choice(len(far), int(len(far) * far_frac), replace=False)
    return np.vstack([near, far[keep]])


def test_complete_fills_the_occluded_half():
    """Keep one side, as a single view would; completion must restore the other.

    The assertion compares the completed cloud against the KNOWN full hull.
    Counting how many points land at y < 0 is not enough: `complete` could
    ignore `sym` entirely and return
    `np.vstack([visible, visible * [1, -1, 1]])`, which satisfies any count-
    based assertion while reflecting across a hardcoded plane rather than the
    solved one. Chamfer against ground truth fails for a wrong plane, so this
    test actually exercises `sym`.
    """
    pts = _symmetric_hull()
    visible = _partially_visible()
    sym = mirror.solve(visible)
    full = mirror.complete(visible, sym)
    assert (full[:, 1] < 0).sum() >= len(visible) - 1
    assert _chamfer(full, pts) < 0.05, _chamfer(full, pts)


def test_complete_uses_the_solved_plane_not_a_hardcoded_one():
    """A deliberately wrong plane must produce a visibly wrong completion.

    This is the red half of the test above: it pins down that `complete`
    reflects across `sym.normal`/`sym.offset`. If it did not, both this and the
    previous test would pass on an implementation that always mirrors in Y.

    Unlike the test above, this one does not need `solve()` to have found the
    plane -- it tests `complete()` in isolation, so it builds the KNOWN true
    Symmetry by hand and uses the literal single-sided `pts[pts[:, 1] > 0]`
    (no far-side leak) rather than `_partially_visible()`, to get the widest
    possible gap between a correct and an incorrect completion. Measured:
    with the true plane, chamfer-to-truth is exactly 0.0; with `wrong`
    (normal replaced by X, offset 0), it is 0.130 -- reflecting in X leaves
    `full[:, 1]` entirely positive, so the whole negative-y half of the hull
    has zero representation. (The brief's original threshold here was 0.2;
    that number is unreachable by ANY wrong-plane construction on this
    fixture family -- even this maximally-adversarial case tops out at 0.130,
    because `visible` is still an exact subset of `pts` and contributes 0
    distance in the full-to-pts direction regardless of which plane is used
    to complete it. 0.1 sits with real margin below the measured 0.130 and
    above the true plane's 0.0.)
    """
    pts = _symmetric_hull()
    visible = pts[pts[:, 1] > 0]
    good = mirror.Symmetry(normal=np.array([0.0, 1.0, 0.0]), offset=0.0,
                            scale=1.0, shift=0.0, residual=0.0)
    wrong = dataclasses.replace(good, normal=np.array([1.0, 0.0, 0.0]), offset=0.0)
    assert _chamfer(mirror.complete(visible, wrong), pts) > 0.1


def _rotated_symmetric_hull(seed=0, n=4000, angle_deg=30.0):
    """`_symmetric_hull` rotated about X, so the mirror normal has a Z component.

    The brief's original pair of affine tests both used the axis-aligned
    `_symmetric_hull()` directly and squashed only z: `pts * [1.0, 1.0, 0.6]`.
    That is a genuine degeneracy, not a hard case: the axis-aligned hull's
    mirror normal is exactly (0, 1, 0), which has zero z-component, and
    `reflect()` only uses `points @ normal` -- so a uniform rescale of z
    changes NOTHING about the y-normal candidate's self-reflection chamfer.
    Verified directly: `solve(pts * [1,1,0.6], refine_affine=True,
    init_scale=s)` returns `scale == s` UNCHANGED to machine precision (tried
    s in {1.0, 2.0, 0.3, 1/0.6}, residual ~1e-16 in every case) -- the
    optimizer has zero gradient in scale along that candidate, so it never
    moves off whatever it started at. No implementation, correct or not, can
    recover a specific scale from a cost surface that is exactly flat in that
    direction; the test as originally written could not fail on a stub that
    hardcodes `scale = init_scale` and ignores `refine_affine` entirely.

    Rotating about X by `angle_deg` gives normal (0, cos(a), sin(a)); at 30
    degrees the z-coefficient is 0.5, enough to give the optimizer a real
    gradient. Verified: on the rotated hull (unsquashed), refine_affine=True
    returns scale=1.0, shift=0.0, residual ~1e-15 (the no-op case still
    holds); on the same hull squashed by 0.6 in (now-rotated) z, it recovers
    scale=1.66667 against a target of 1/0.6=1.66667 (axis error 0.0 degrees,
    residual ~5e-9) -- proving the recovery is real arithmetic, not a fixture
    artifact.
    """
    pts = _symmetric_hull(seed, n)
    a = np.radians(angle_deg)
    c, s = np.cos(a), np.sin(a)
    r = np.array([[1.0, 0.0, 0.0], [0.0, c, -s], [0.0, s, c]])
    return pts @ r.T


def test_affine_refinement_is_a_noop_on_metric_input():
    """MoGe-2 is already metric, so the scale/shift solve must stay at identity.

    On its own this assertion is satisfied by `scale = 1.0` hardcoded and no
    refinement at all, so it is paired with the recovery test below. Keep both:
    this one pins "does not wander", that one pins "actually solves". Uses
    `_rotated_symmetric_hull`, not `_symmetric_hull`, so this is a real check
    of "does not wander off 1.0" rather than a check of a direction the cost
    is mathematically incapable of moving in -- see that fixture's docstring.
    """
    pts = _rotated_symmetric_hull()
    sym = mirror.solve(pts, refine_affine=True)
    assert abs(sym.scale - 1.0) < 0.05, sym.scale
    assert abs(sym.shift) < 0.05, sym.shift


def test_affine_refinement_recovers_a_known_depth_scaling():
    """Distort depth by a known factor; the refinement must undo it.

    This is the falsifiable half of the pair above. A stub that returns
    scale=1.0 passes the no-op test and fails this one, which is the whole
    point: the affine solve is what makes an affine-invariant point map usable,
    so it must be shown to do arithmetic rather than return its initial value.
    Uses `_rotated_symmetric_hull` for the same reason as the no-op test above:
    on the axis-aligned hull this assertion is unsatisfiable by ANY
    implementation (the cost is exactly flat in scale along the true y-normal,
    verified directly), not merely hard to satisfy correctly.
    """
    pts = _rotated_symmetric_hull()
    squashed = pts * [1.0, 1.0, 0.6]
    sym = mirror.solve(squashed, refine_affine=True)
    assert abs(sym.scale - 1.0 / 0.6) < 0.1 * (1.0 / 0.6), sym.scale
