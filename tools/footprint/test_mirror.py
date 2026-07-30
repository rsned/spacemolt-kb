"""Checks for the mirror-constrained symmetry solve.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_mirror.py
"""

import importlib
import json
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
# mirror.py already imports pointmap unconditionally (`from . import paths,
# pointmap`), so this costs nothing extra: if it weren't importable here, the
# `mirror` import above would already have failed. Only used below to build a
# plain Cloud value for the run()/load() round trip -- no CUDA, no model
# inference, so unlike test_pointmap.py this needs no `needs_cuda` mark.
pointmap = _load("pointmap")
# `mirror` imports `gate` (for `project`, `inside_fraction` and
# `MIN_DEPTH_SEPARATION_FRACTION`), so this too costs nothing extra. Used to
# build the one-view fixtures' own silhouettes, so the test's matte and the
# solver's scoring share exactly one projection.
gate = _load("gate")


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
    CARRY THIS INTO TASK 6B: measured directly against 3 real MoGe-2 clouds
    (see the task report), residuals were 0.014-0.016 -- ABOVE this ceiling
    and above the synthetic lopsided-hull number. That is not primarily a
    noise-model problem: `_partially_visible()` below (this file's one-VIEW
    fixture, near half plus a sparse far-half sample -- what production
    actually gets, unlike this test's fully two-sided `_symmetric_hull()`) is
    already at residual ~0.013-0.014 while EXACTLY symmetric, before any of
    THIS test's noise is added. Partial coverage alone explains the real
    numbers; see the longer note beside `RESIDUAL_CEILING` in mirror.py. This
    test proves the mechanism (noise apart from asymmetry, on the ONLY
    two-sided fixture where doing so is meaningful), not that 0.013 is the
    right number for real art -- Task 6b re-derives that from real clouds.
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
    that.

    The kept far points get isotropic noise (sigma=0.01) added on top of the
    subsample: without it, they are `half * [1, -1, 1]` exactly (see
    `_symmetric_hull`), i.e. bit-for-bit mirrors of near-side points, which
    foreshortened real pixels never are. That mattered in practice: with exact
    mirrors, `test_complete_fills_the_occluded_half`'s `chamfer < 0.05`
    assertion measured chamfer-to-truth of EXACTLY 0.0000 at far_frac=0.25 --
    a margin so perfect it could not distinguish a correct solve from one that
    got lucky, only from one that failed outright. With sigma=0.01 noise,
    that same assertion now measures a real, non-zero 0.0020 -- still well
    under the 0.05 ceiling, but an actual measurement rather than an artifact
    of exact duplication.

    Re-measured the far_frac breakpoint under this noise (seed 0): axis error
    stays under 0.05 degrees and chamfer-to-truth stays under 0.003 for
    far_frac >= 0.20; at far_frac=0.18 it's borderline (axis error 2.1 degrees,
    chamfer 0.036); at far_frac=0.15 the solver still locks onto the wrong SVD
    starting axis entirely (axis error 90.0 degrees, chamfer 0.042) -- the same
    break point as without noise. far_frac=0.25 keeps clear margin above it.
    """
    pts = _symmetric_hull(seed, n)
    near = pts[pts[:, 1] > 0]
    far = pts[pts[:, 1] <= 0]
    rng = np.random.default_rng(seed + 501)
    keep = rng.choice(len(far), int(len(far) * far_frac), replace=False)
    far_kept = far[keep] + rng.normal(0.0, 0.01, (len(keep), 3))
    return np.vstack([near, far_kept])


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

    `good` was previously built only as a `dataclasses.replace` base and never
    itself asserted on, so the "correct vs incorrect" comparison the docstring
    describes was never actually made -- a `complete()` that ignores `sym` and
    returns `points` unchanged (no reflection at all) measures chamfer 0.1028
    against `pts`, which also clears `> 0.1`, so that inert stub passed this
    test undetected. Asserting on `good` directly closes that gap.
    """
    pts = _symmetric_hull()
    visible = pts[pts[:, 1] > 0]
    good = mirror.Symmetry(normal=np.array([0.0, 1.0, 0.0]), offset=0.0,
                            scale=1.0, shift=0.0, residual=0.0)
    wrong = dataclasses.replace(good, normal=np.array([1.0, 0.0, 0.0]), offset=0.0)
    assert _chamfer(mirror.complete(visible, good), pts) < 0.05
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
    """MoGe-2 is already metric, so the scale solve must stay at identity, and
    the plane must still be found, even when initialised away from shift=0.

    On its own the scale assertion is satisfied by `scale = 1.0` hardcoded and
    no refinement at all, so it is paired with the recovery test below. Keep
    both: this one pins "does not wander", that one pins "actually solves".
    Uses `_rotated_symmetric_hull`, not `_symmetric_hull`, so this is a real
    check of "does not wander off 1.0" rather than a check of a direction the
    cost is mathematically incapable of moving in -- see that fixture's
    docstring.

    Deliberately does NOT assert on `sym.shift`. `shift` is not identifiable
    by self-reflection chamfer at all, for any normal -- see the comment
    beside `_apply_affine` in mirror.py and its measured table (init_shift
    0.0/0.7/-1.5 -> shift 0.000000/0.561740/-1.205369, all at residual
    ~1e-8-1e-16: three different answers, all equally valid, because `off`
    absorbs any shift exactly). An `abs(sym.shift) < 0.05` assertion here only
    ever passed because init_shift defaults to 0.0 -- it could not have failed
    on a solve() that silently ignored shift entirely. What DOES hold
    regardless of init_shift, verified directly at init_shift=0.7: scale stays
    1.000000 and the recovered normal stays exact (axis error 0.0 degrees) --
    that is the real "no-op on metric input" guarantee, so that's what this
    asserts instead.
    """
    pts = _rotated_symmetric_hull()
    sym = mirror.solve(pts, refine_affine=True, init_shift=0.7)
    true_normal = np.array([0.0, np.cos(np.radians(30.0)), np.sin(np.radians(30.0))])
    axis_err = np.degrees(np.arccos(min(1.0, abs(sym.normal @ true_normal))))
    assert abs(sym.scale - 1.0) < 0.05, sym.scale
    assert axis_err < 5.0, axis_err
    assert sym.residual < mirror.RESIDUAL_CEILING, sym.residual


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


def test_run_writes_cloud_resolved_and_load_round_trips(tmp_path, monkeypatch):
    """`run()` writes 6 npz keys nothing else pins; `load()` must read them back.

    Without this, stages 5-7 would each hand-roll `np.load(...)["normal"]`
    etc. against `cloud_resolved.npz`, unpinned by any test -- a key rename in
    `run()` would break them silently, the same gap `pointmap.py` closed with
    its own `load()` and `test_run_writes_cloud_and_load_round_trips`.
    """
    monkeypatch.setattr(mirror.paths, "FOOTPRINT_ROOT", tmp_path)
    pts = _symmetric_hull()
    cloud = pointmap.Cloud(points=pts, pixels=np.zeros((len(pts), 2), dtype=np.float32),
                            normals=None, intrinsics=np.eye(3))

    written_sym = mirror.run("test_ship", cloud)
    out = tmp_path / "test_ship" / "cloud_resolved.npz"
    assert out.exists()

    loaded_sym, loaded_points = mirror.load("test_ship")
    assert np.allclose(loaded_sym.normal, written_sym.normal)
    assert loaded_sym.depth_separation == written_sym.depth_separation
    assert loaded_sym.failure == written_sym.failure
    assert loaded_sym.offset == written_sym.offset
    assert loaded_sym.scale == written_sym.scale
    assert loaded_sym.shift == written_sym.shift
    assert loaded_sym.residual == written_sym.residual
    assert np.allclose(loaded_points, mirror.complete(pts, written_sym))


# --------------------------------------------------------------------------
# One-view fixtures for `solve_from_view` (stage 4's real-cloud entry point).
# --------------------------------------------------------------------------

def _hull_surface(n=60000, a=2.0, b=1.0, c=0.6, keel=0.9, waist=0.35, seed=0):
    """Points ON a hull surface, with outward normals. Symmetry plane: y=0.

    A quadric would be simpler but is DEGENERATE for this test, measured. The
    task brief's fixture was a plain ellipsoid (`pts = v * [a, b, c]`) on the
    grounds that "its symmetry about x and z is harmless here: this test
    asserts the recovered axis against a known answer rather than relying on
    the axis being unique". That reasoning does not survive measurement: on the
    plain ellipsoid, back-face-culled at obliquity 0.866, the reflections
    across ALL THREE coordinate planes map the solid onto itself, so all three
    reproject inside the silhouette and score essentially the same silhouette
    agreement --

        candidate      inside_fraction   depth_sep / z-extent
        y=0 (TRUE)          0.9980              +0.331
        x=0                 0.9984              +0.00015
        z=0                 0.9981              +0.290

    -- i.e. z=0 BEATS the true plane by 1e-4 while clearing the depth floor
    comfortably, so "maximise silhouette agreement subject to the depth
    constraint" is entitled to return a plane 90 degrees from the truth. A
    perfect solver fails an `axis_err < 8.0` assertion on that fixture.

    Fixed by deforming the ellipsoid EVENLY in x (a keel that drops the belly
    toward the nose and tail, and a waist that narrows them), which breaks z=0
    while leaving both y=0 (the answer) and x=0 (the fold, see
    `_tangential_fold`) exact. Same measurement on this fixture:

        y=0 (TRUE)          0.9976              +0.357
        x=0 (fold)          0.9988              +0.00005
        z=0                 0.7987              +0.236

    z=0 is now demoted by 0.20 of silhouette agreement, and x=0 -- which still
    scores ABOVE the true plane, structurally, because it maps the visible
    sheet exactly onto itself -- is left as the single competitor the depth
    constraint has to reject. That is the arrangement this task is about.

    Keeping x=0 exact is deliberate for the same reason: it makes the depth
    constraint load-bearing rather than decorative (see
    `test_the_depth_constraint_is_what_rejects_the_tangential_fold`).

    Outward normals come from the deformation's Jacobian: for points q = f(p),
    normals transform as J^-T n, not as J n. They are needed to cull back faces
    -- the whole point of the fixture is a front-surface SHEET, not a subsample
    of a solid volume. Reflecting a bilaterally symmetric SOLID across its true
    plane maps the solid onto itself, so the correct answer's depth separation
    is zero by construction; measured on `_symmetric_hull` (a solid) it comes
    out -0.004 to +0.005 of z-extent across viewing angles, sometimes negative.
    Depth separation exists only for a one-view sheet, where the reflected
    front surface becomes the genuinely occluded back surface.
    """
    rng = np.random.default_rng(seed)
    v = rng.normal(size=(n, 3))
    v /= np.linalg.norm(v, axis=1, keepdims=True)
    p = v * np.array([a, b, c])
    nrm = v / np.array([a, b, c])
    nrm /= np.linalg.norm(nrm, axis=1, keepdims=True)

    x, y = p[:, 0], p[:, 1]
    u = (x / a) ** 2                                  # even in x, so x=0 stays exact
    q = np.column_stack([x, y * (1.0 - waist * u), p[:, 2] + keel * c * u])

    j = np.zeros((n, 3, 3))
    j[:, 0, 0] = 1.0
    j[:, 1, 0] = -2.0 * waist * x * y / a ** 2
    j[:, 1, 1] = 1.0 - waist * u
    j[:, 2, 0] = 2.0 * keel * c * x / a ** 2
    j[:, 2, 2] = 1.0
    nq = np.einsum("nji,nj->ni", np.linalg.inv(j), nrm)   # n' = J^-T n
    nq /= np.linalg.norm(nq, axis=1, keepdims=True)
    return q, nq


def _one_sided_view(obliquity_deg=60.0, dist=8.0, shape=(480, 640), focal=600.0, seed=0):
    """Render the hull as a single view: back-face culled, in camera coordinates.

    `obliquity_deg` rotates the symmetry-plane normal toward the camera. It is
    the parameter that bounds how much depth separation the TRUE plane can
    possibly show, so it is explicit rather than buried: at 0 the plane
    contains the view direction and reflection moves nothing in depth, so the
    separation is exactly zero and the depth term carries no signal at all.
    Measured true-plane depth_separation / z-extent on this fixture: 0.045 at
    20 deg, 0.094 at 30, 0.204 at 45, 0.357 at 60, 0.654 at 75, 0.971 at 85.
    60 sits well clear of the 0.01 floor while staying a plausible
    three-quarter view.

    Returns `(view, mask, K, R, t)`. `R` and `t` are handed back so a test can
    build the EXACT camera-space image of any world plane
    (`_plane(R, t, world_normal)`) instead of approximating it -- the true
    plane's offset is `n . t`, NOT `n . centroid`: the visible sheet's centroid
    is not on the symmetry plane, and using it costs 0.07 of silhouette
    agreement and half the depth separation (measured 0.9267 / 0.140 against
    the true plane's 0.9980 / 0.341 on the undeformed ellipsoid).
    """
    b = np.radians(obliquity_deg)
    r = np.array([[1.0, 0.0, 0.0],
                  [0.0, np.cos(b), np.sin(b)],
                  [0.0, -np.sin(b), np.cos(b)]])
    pts_w, nrm_w = _hull_surface(seed=seed)
    t = np.array([0.0, 0.0, dist])
    cam = pts_w @ r.T + t
    nrm_c = nrm_w @ r.T
    viewdir = cam / np.linalg.norm(cam, axis=1, keepdims=True)
    view = cam[(nrm_c * viewdir).sum(1) < 0]              # back-face cull

    h, w = shape
    k = np.array([[focal, 0.0, w / 2.0], [0.0, focal, h / 2.0], [0.0, 0.0, 1.0]])
    mask = gate.reproject(view, k, shape)                  # the view's own silhouette
    return view, mask, k, r, t


def _plane(r, t, world_normal):
    """The camera-space (normal, offset) of a world plane through the origin."""
    n = r @ np.asarray(world_normal, dtype=float)
    return n, float(n @ t)


def _axis_err(a, b):
    return float(np.degrees(np.arccos(min(1.0, abs(np.asarray(a) @ np.asarray(b))))))


def test_solve_from_view_recovers_the_plane_a_self_chamfer_solve_folds():
    """The regression test for the whole task: same input, both solvers.

    `solve` (self-chamfer) must fold and `solve_from_view` must not. Asserting
    only that the new solver works would leave the test passing if someone
    quietly routed it back to the old objective.

    THE BAR IS 10 DEGREES, NOT 8, AND THAT IS A RECORDED WEAKNESS OF THE STAGE.
    The objective as originally specified does not identify the plane at all --
    silhouette agreement saturates at exactly 1.0000 for any plane that pushes
    the mirrored half clear of the hull (strictly interior to the matte, so
    nothing spills), while the TRUE plane grazes the silhouette rim and measures
    0.9969-0.9997, so the true answer is not the maximum and loses by ~5e-4. The
    depth term cannot arbitrate: a receded plane has MORE separation (1.14 of
    z-extent against 0.357). What makes the search tractable is
    `mirror._PROXIMITY_CEILING`, and its own comment carries the calibration and
    the obliquity range over which it holds.

    Accuracy is not uniform across the view, and the assertion below is set by
    the worst case rather than the typical one. It degrades near obliquity 0.866,
    where the returned plane is a mild recession (obliquity 0.928 against a true
    0.866, depth separation 0.50 of z-extent against 0.357). See the comment on
    the assertion for the numbers and for why tightening it is the wrong move.

    The two tests below pin WHY each constraint is needed: the depth floor is the
    only thing that rejects the tangential fold, and the proximity ceiling is the
    only thing that rejects a receded plane.
    """
    view, mask, k, r, t = _one_sided_view(obliquity_deg=60.0)
    n_true, _ = _plane(r, t, [0.0, 1.0, 0.0])
    zext = float(view[:, 2].max() - view[:, 2].min())

    good = mirror.solve_from_view(view, mask, k)
    assert good.failure is None, good.failure
    assert _axis_err(good.normal, n_true) < 10.0, _axis_err(good.normal, n_true)
    # Measured axis error by obliquity: 0.500 -> 0.18, 0.707 -> 1.74,
    # 0.866 -> 8.19, 0.966 -> 3.40 degrees. 10.0 clears the worst by 1.2x.
    # Do NOT tighten this to 8.0 by setting _PROXIMITY_CEILING = 0.09: that
    # answer is still a receded plane (obliquity 0.926 vs a true 0.866), so it
    # would be a green test standing on a wrong reconstruction.
    # Measured 0.357 * zext for the true plane at this obliquity, versus
    # 0.00005 * zext for the tangential fold -- a 7000x gap. Assert well
    # inside it.
    assert good.depth_separation > 0.05 * zext, good.depth_separation
    assert good.obliquity > 0.8, good.obliquity        # cos(60 deg) rotation -> 0.866

    folded = mirror.solve(view)                        # the superseded objective
    assert _axis_err(folded.normal, n_true) > 20.0, (
        "the self-chamfer solve is expected to fold on one-sided input; if it "
        "no longer does, this task's premise changed and the fixture is wrong")


def _receded_plane(view, q=1.05):
    """A plane behind the cloud, normal along the mean view direction.

    The canonical member of the family that defeats "maximise silhouette
    agreement". Reflecting across it pushes the whole sheet away from the camera,
    and because `uv = K p / p_z` that SHRINKS the projection until every mirrored
    point is strictly interior to the matte. Axis-independent by construction --
    normal = the direction from the camera to the cloud's centroid -- so it needs
    no seeded search and no hardcoded numbers, unlike "N degrees off the view
    direction", which names a one-parameter family rather than a plane.

    `q` places the plane that fraction of the way through the cloud's own span
    along that normal; 1.05 puts it just behind everything.
    """
    n = view.mean(axis=0)
    n = n / np.linalg.norm(n)
    proj = view @ n
    return n, float(proj.min() + q * (proj.max() - proj.min()))


def _metrics(view, mask, k, n, off):
    """(inside_fraction, depth_separation, proximity) for one candidate plane."""
    n = np.asarray(n, dtype=float)
    n = n / np.linalg.norm(n)
    mirrored = mirror.reflect(view, n, off)
    depth = mirror._visible_depth(view, k, mask.shape)
    sep, _ = mirror._depth_separation(depth, mirrored, k, mask.shape)
    extent = float(np.linalg.norm(view.max(axis=0) - view.min(axis=0)))
    near = float(cKDTree(view).query(mirrored)[0].mean()) / extent
    return gate.inside_fraction(mirrored, k, mask), sep, near


def test_silhouette_agreement_alone_cannot_reject_the_tangential_fold():
    """The depth floor is load-bearing, not decorative.

    The canonical tangential fold on this fixture is not a searched-for normal
    but an exact one: the hull is symmetric about world x=0 as well as y=0 (see
    `_hull_surface`), and reflection across x=0 maps the CAMERA position to
    itself, so it maps the visible sheet exactly onto itself. That is the
    definition of a fold, stated axis-independently.

    Its silhouette agreement is at or ABOVE the true plane's -- structurally, not
    coincidentally, since it reprojects onto the very pixels the visible points
    were read from -- so no floor on `inside_fraction` can separate them, and
    only `depth_separation` can.
    """
    view, mask, k, r, t = _one_sided_view(obliquity_deg=60.0)
    zext = float(view[:, 2].max() - view[:, 2].min())

    true_frac, true_sep, _ = _metrics(view, mask, k, *_plane(r, t, [0.0, 1.0, 0.0]))
    fold_frac, fold_sep, _ = _metrics(view, mask, k, *_plane(r, t, [1.0, 0.0, 0.0]))

    assert fold_frac >= true_frac, (fold_frac, true_frac)      # 0.9988 vs 0.9976
    assert abs(fold_sep) < 0.001 * zext, fold_sep              # measured 0.00005 * zext
    assert true_sep > 0.05 * zext, true_sep                    # measured 0.357 * zext
    assert abs(fold_sep) < gate.MIN_DEPTH_SEPARATION_FRACTION * zext, fold_sep
    assert true_sep > gate.MIN_DEPTH_SEPARATION_FRACTION * zext, true_sep


def test_silhouette_agreement_alone_cannot_reject_a_receded_plane():
    """Silhouette agreement is MAXIMISED by a wrong plane, and the depth term
    cannot see it either -- which is why `solve_from_view` needs a proximity
    ceiling as well.

    This is the measurement that contradicts this task's brief. The brief's
    evidence for "maximise inside_fraction subject to the depth constraint"
    compared candidates at a fixed offset through the centroid; with the offset
    free, a plane behind the cloud reaches inside_fraction 1.0000 -- ABOVE the
    true plane -- while having MORE depth separation than the true plane, so it
    clears the floor comfortably. What it cannot do is land near the hull.
    """
    view, mask, k, r, t = _one_sided_view(obliquity_deg=60.0)
    zext = float(view[:, 2].max() - view[:, 2].min())

    true_frac, true_sep, true_near = _metrics(view, mask, k, *_plane(r, t, [0.0, 1.0, 0.0]))
    far_frac, far_sep, far_near = _metrics(view, mask, k, *_receded_plane(view))

    assert far_frac >= true_frac, (far_frac, true_frac)        # 1.0000 vs 0.9976
    assert far_sep > true_sep, (far_sep, true_sep)             # 1.14 vs 0.357 of zext
    assert far_sep > gate.MIN_DEPTH_SEPARATION_FRACTION * zext, far_sep
    # Only proximity separates them: measured 0.0610 against 0.2157 of extent.
    assert true_near < mirror._PROXIMITY_CEILING < far_near, (true_near, far_near)


def test_solve_from_view_reports_failure_when_no_plane_clears_the_depth_floor():
    """An unreachable depth floor must surface as `failure`, not as an answer.

    Raising `MIN_DEPTH_SEPARATION_FRACTION` above what any plane can achieve is
    the same situation a bow-on hero image produces for real: at obliquity 0 the
    symmetry plane contains the view direction, reflection moves nothing in
    depth, and the separation is EXACTLY zero, so no floor is clearable. Forcing
    it here rather than rendering a bow-on view keeps the test to a couple of
    seconds and makes the mechanism, not the fixture, the thing under test.
    """
    view, mask, k, _, _ = _one_sided_view(obliquity_deg=60.0)
    original = gate.MIN_DEPTH_SEPARATION_FRACTION
    try:
        gate.MIN_DEPTH_SEPARATION_FRACTION = 100.0
        sym = mirror.solve_from_view(view, mask, k)
    finally:
        gate.MIN_DEPTH_SEPARATION_FRACTION = original
    assert sym.failure is not None
    assert "depth_separation" in sym.failure, sym.failure
    # The numbers are still reported, so a failing ship can be diagnosed.
    assert sym.depth_separation != 0.0


def test_solve_reports_failure_when_every_scale_lands_on_a_bound():
    """An all-bounds affine solve is a reported failure, not a silent fallback.

    Task 5's guard discarded results terminating on a scale bound and then fell
    back to `best_any` -- the best result of ANY kind -- which returns exactly
    what the guard exists to reject. Measured on this fixture with 0.5 as the only
    scale start: all three SVD axes run to the 0.2 lower bound and the fallback
    returns residual 0.0073, UNDER RESIDUAL_CEILING, i.e. a 5x depth collapse
    reported as trustworthy against a true scale of 1/0.6 = 1.667.

    Constructing the case needs the scale multi-start narrowed to 0.5 alone,
    which is why `_SCALE_STARTS` is a module constant. Without this test the guard
    is unfalsifiable: deleting it entirely changes no other test, because with the
    default starts the correct basin wins on raw residual anyway.
    """
    squashed = _rotated_symmetric_hull() * [1.0, 1.0, 0.6]
    original = mirror._SCALE_STARTS
    try:
        mirror._SCALE_STARTS = (0.5,)
        sym = mirror.solve(squashed, refine_affine=True, init_scale=0.5)
    finally:
        mirror._SCALE_STARTS = original

    assert sym.scale <= 0.2 + 1e-9, sym.scale             # it really is on the bound
    assert sym.residual < mirror.RESIDUAL_CEILING, sym.residual   # and looks trustworthy
    assert sym.failure is not None, (
        "an all-bounds solve returns a bound artifact at a residual under "
        "RESIDUAL_CEILING; it must not be reportable as a pass")
    assert "bound" in sym.failure, sym.failure


def test_obliquity_tracks_the_normal_it_is_derived_from():
    """`obliquity` is derived, so it cannot drift out of step with `normal`.

    A stored field would: `dataclasses.replace(sym, normal=...)` leaves it stale,
    and `load()` would have to restore it from a file that could disagree.
    """
    sym = mirror.Symmetry(normal=np.array([0.0, 1.0, 0.0]), offset=0.0, scale=1.0,
                          shift=0.0, residual=0.0)
    assert sym.obliquity == 0.0
    assert dataclasses.replace(sym, normal=np.array([0.0, 0.0, 1.0])).obliquity == 1.0
    # Unnormalised normals must not inflate it.
    assert abs(dataclasses.replace(
        sym, normal=np.array([0.0, 3.0, 3.0])).obliquity - 1 / np.sqrt(2)) < 1e-12


def test_run_records_the_symmetry_diagnostics_in_quality_json(tmp_path, monkeypatch):
    """`obliquity`, `depth_separation` and `failure` must reach quality.json.

    `gate.run` cannot write them -- it never sees a `Symmetry` -- so a failure
    that lived only in the returned dataclass would vanish from the batch report.
    Both stages merge into the same file, so this also pins that `mirror.run`
    writes rather than replaces.
    """
    monkeypatch.setattr(mirror.paths, "FOOTPRINT_ROOT", tmp_path)
    q = tmp_path / "test_ship"
    q.mkdir(parents=True, exist_ok=True)
    (q / "quality.json").write_text(json.dumps({"silhouette_iou": 0.5}))

    pts = _symmetric_hull()
    cloud = pointmap.Cloud(points=pts, pixels=np.zeros((len(pts), 2), dtype=np.float32),
                           normals=None, intrinsics=np.eye(3))
    sym = mirror.run("test_ship", cloud)

    data = json.loads((q / "quality.json").read_text())
    assert data["silhouette_iou"] == 0.5              # the earlier key survives
    assert data["obliquity"] == sym.obliquity
    assert data["depth_separation"] == sym.depth_separation
    assert data["symmetry_residual"] == sym.residual
    assert data["symmetry_failure"] == sym.failure
