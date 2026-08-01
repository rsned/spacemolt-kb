"""End-to-end checks for the footprint pipeline.

Stages 4-7 are asserted exactly against ground truth. The MoGe stage is
reported, not asserted: it is a network being shown a synthetic render, and a
hard assertion there would fail for reasons unrelated to this code.

Run: ~/moge-venv/bin/python -m pytest tools/footprint/test_endtoend.py -v -s
"""

import importlib
import json
import pathlib
import sys
import types

import numpy as np
import pytest
from shapely.geometry import Polygon

def _load(name):
    """Import a pipeline module through the package.

    The modules use `from . import paths`, so they must be imported as package
    members. Loading them as standalone files raises ImportError: attempted
    relative import with no known parent package.
    """
    sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[2]))
    return importlib.import_module(f"tools.footprint.{name}")

synth = _load("synth")
mirror = _load("mirror")
ground = _load("ground")
profile = _load("profile")
run = _load("run")

# Match test_pointmap.py exactly. Do NOT write the guard as
# `pytest.mark.skipif(not __import__("importlib").util.find_spec("torch"), ...)`:
# `importlib.util` is a SUBMODULE and is not bound by `import importlib`, so that
# expression raises `AttributeError: module 'importlib' has no attribute 'util'`
# in plain Python. Verified — it only appears to work under pytest, because
# pytest imports importlib.util itself as a side effect. Relying on another
# library's imports to make your guard evaluable is a latent collection failure.
#
# Also per-function marks, never a module-level `pytestmark`: pytest applies a
# bare `pytestmark` to every test in the module regardless of where the line sits
# in the file, which would skip the two CPU-only ground-truth tests on a box with
# no GPU — and those are the tests that assert our own arithmetic.
torch = pytest.importorskip("torch")
needs_cuda = pytest.mark.skipif(not torch.cuda.is_available(), reason="needs CUDA")


def _ground_truth_chain(scene):
    """Stages 4-7 on a perfect cloud: our arithmetic, no network involved."""
    cam = scene.points @ scene.R.T + scene.t
    # `solve`, not `solve_from_view`: scene.points is a TWO-SIDED volume sampled
    # over the whole mesh, which is the regime self-chamfer handles correctly.
    # This chain exists to validate our arithmetic, so it must not depend on the
    # silhouette machinery the real path uses.
    sym = mirror.solve(cam)
    full = mirror.complete(cam, sym)
    up = scene.up @ scene.R.T
    xy = ground.project(full, up)
    poly = ground.hull(xy, alpha=3.0)
    sym_xy = ground.project(sym.normal[None, :], up)[0]
    return profile.sample(profile.canonicalise(poly, sym_xy))


def test_box_recovers_a_rectangular_profile():
    s = synth.box_scene(length=4.0, width=2.0, height=1.5,
                        azimuth_deg=35, elevation_deg=30)
    w, concave = _ground_truth_chain(s)

    interior = w[8:-8]
    assert interior.std() / interior.mean() < 0.10, "a box must have constant beam"
    assert abs(profile.aspect(w) - 2.0) / 2.0 < 0.15, profile.aspect(w)
    assert not concave.any()


def test_vertical_cylinder_recovers_a_circular_profile():
    s = synth.cylinder_scene(radius=1.0, height=3.0,
                             azimuth_deg=20, elevation_deg=35)
    w, _ = _ground_truth_chain(s)

    # A circle of unit length has half-width sqrt(t - t^2) / 2 ... check the
    # midships beam equals the length, i.e. aspect 1.
    assert abs(profile.aspect(w) - 1.0) < 0.20, profile.aspect(w)
    mid, near_end = profile.STATIONS // 2, profile.STATIONS // 12
    assert w[mid] > w[near_end] and w[mid] > w[-near_end], \
        "a circle must be widest amidships"


@needs_cuda
def test_moge_chain_is_reported_not_asserted(capsys):
    pointmap = _load("pointmap")
    matte = _load("matte")
    s = synth.box_scene(4.0, 2.0, 1.5, azimuth_deg=35, elevation_deg=30)
    mask, _ = matte.extract(s.image)
    cloud = pointmap.infer(s.image, mask)
    # A MoGe cloud is one-sided even on a synthetic render, so this is the
    # view-based solve, not the two-sided one.
    sym = mirror.solve_from_view(cloud.points, mask, cloud.intrinsics)
    with capsys.disabled():
        print(f"\n  MoGe on synthetic box: n={len(cloud.points)} "
              f"mirror residual={sym.residual:.4f} "
              f"(ceiling {mirror.RESIDUAL_CEILING})")
    assert len(cloud.points) > 0


@needs_cuda
def test_moge_clears_a_floor_on_real_hero_art():
    """The gate on what we actually depend on.

    The synthetic check above is reporting-only because a flat-shaded render is
    out of distribution for MoGe. A hero image is not: it is a rendered object
    on a plain ground, which is what the model was trained on. So this one
    asserts. Skips when the art is absent rather than failing in a clean
    checkout.
    """
    import cv2
    pointmap = _load("pointmap")
    matte = _load("matte")
    paths = _load("paths")

    hero = paths.HERO_DIR / "outerrim_prayer.webp"
    if not hero.exists():
        pytest.skip(f"no hero art at {hero}")

    img = cv2.cvtColor(cv2.imread(str(hero)), cv2.COLOR_BGR2RGB)
    mask, _ = matte.extract(img)
    cloud = pointmap.infer(img, mask)

    assert len(cloud.points) > 10_000, "point map is implausibly sparse"
    assert np.isfinite(cloud.points).all()
    # Documented invariant, NOT a test of our code: MoGe emits camera-space
    # points with strictly positive depth by construction (measured z in
    # [23.9, 137] even on pure random-noise input), so this cannot fail unless
    # a later change starts transforming the cloud. It is here because
    # gate.reproject relies on it when it filters points[:, 2] > 1e-6.
    assert (cloud.points[:, 2] > 0).all(), "points behind the camera"

    # A hull seen in 3/4 must have real depth relief. A collapsed depth range
    # means the model read the image as a flat card, which would make every
    # downstream footprint a silhouette rather than a shape.
    d = cloud.points[:, 2]
    assert d.std() / d.mean() > 0.02, f"depth relief {d.std() / d.mean():.4f} too flat"

    sym = mirror.solve_from_view(cloud.points, mask, cloud.intrinsics)
    assert sym.residual < 0.25, f"Prayer is a boxy hull; residual {sym.residual:.3f}"
    # The fold check, which is the failure this stage actually had: a folded
    # plane leaves the completed cloud barely larger than the visible one
    # (measured 1.035x with the superseded self-chamfer solve). A genuine
    # occluded half must add real width.
    full = mirror.complete(cloud.points, sym)
    ext = float(np.linalg.norm(cloud.points.max(0) - cloud.points.min(0)))
    gain = float(np.linalg.norm(full.max(0) - full.min(0))) / ext
    assert gain > 1.15, f"completion added almost nothing ({gain:.3f}x): plane folded"


# --- Task 9 fix round 1: a single ship must not be able to kill the batch ---
#
# `run.process` can raise -- most concretely `profile.canonicalise`'s
# ValueError("degenerate footprint...") on a real reconstruction whose hull
# axis collapses, a plausible outcome scaling from 19 to 335 ships -- and
# `main()`'s original list comprehension over every ship had no exception
# handling at all, so one bad ship on the tail end of a long batch would
# lose every earlier ship's already-computed result (report.json is written
# once, from the full list, at the very end). These tests exercise the seam
# directly (`_run_batch` / `_pick_alpha`'s per-ship helper `_ship_ground_xy`)
# rather than the real pipeline, so they need neither CUDA nor real hero art.

def test_batch_isolates_a_per_ship_exception(monkeypatch):
    def fake_process(ship_id, image_path, alpha, background):
        if ship_id == "bad_ship":
            raise ValueError("degenerate footprint: zero length along the hull axis")
        return {"id": ship_id, "status": "ok", "quality": {}}

    monkeypatch.setattr(run, "process", fake_process)
    heroes = {"good_one": pathlib.Path("a.webp"), "bad_ship": pathlib.Path("b.webp"),
              "good_two": pathlib.Path("c.webp")}

    results = run._run_batch(heroes, alpha=3.0, background="neutral")

    by_id = {r["id"]: r for r in results}
    assert len(results) == 3, "the two good ships' results must survive the bad one"
    assert by_id["good_one"]["status"] == "ok"
    assert by_id["good_two"]["status"] == "ok"
    assert by_id["bad_ship"]["status"] == "failed_exception"
    assert "degenerate footprint" in by_id["bad_ship"]["reason"]


def test_batch_does_not_swallow_keyboard_interrupt(monkeypatch):
    def fake_process(ship_id, image_path, alpha, background):
        raise KeyboardInterrupt

    monkeypatch.setattr(run, "process", fake_process)
    with pytest.raises(KeyboardInterrupt):
        run._run_batch({"any_ship": pathlib.Path("a.webp")}, alpha=3.0, background="neutral")


def test_pick_alpha_skips_a_ship_whose_stages_raise(monkeypatch):
    def fake_ground_xy(ship_id, path, background):
        if ship_id == "bad_ship":
            raise ValueError("boom")
        return np.zeros((10, 2))

    seen = []
    monkeypatch.setattr(run, "_ship_ground_xy", fake_ground_xy)
    monkeypatch.setattr(run.ground, "sweep_alpha", lambda clouds: seen.append(len(clouds)) or 7.0)

    heroes = {"good_one": pathlib.Path("a.webp"), "bad_ship": pathlib.Path("b.webp"),
              "good_two": pathlib.Path("c.webp")}
    alpha = run._pick_alpha(heroes, "neutral")

    assert alpha == 7.0
    assert seen == [2], "sweep_alpha must only see the two ships that didn't raise"


def test_pick_alpha_raises_when_every_ship_fails(monkeypatch):
    def always_raises(ship_id, path, background):
        raise ValueError("boom")

    monkeypatch.setattr(run, "_ship_ground_xy", always_raises)
    with pytest.raises(RuntimeError, match="every ship failed"):
        run._pick_alpha({"bad_one": pathlib.Path("a.webp")}, "neutral")


# --- Task 9 final review I1: run._depth_extent was untested -------------
#
# run.process's hull-length-unit conversion is the pipeline's ONLY check on
# scale/depth error (profile.py's own comment on ASPECT_BOUNDS: silhouette
# IoU is provably invariant to it). It had no coverage of its own until now
# -- exercised only indirectly by the real batch, where an earlier draft
# never passed depth_extent at all and nothing caught it (see run.py's
# AMENDED comment on _depth_extent). These construct known synthetic
# geometry through the same axis/along/ptp expression `_depth_extent` uses,
# then check `profile.implausible` sees the units it expects on both sides.

def test_depth_extent_bridges_to_profile_implausible_on_a_healthy_hull():
    # length 4.0 along x, beam 0.4 (half-width 0.2) -- not length 1.0, so the
    # "divide by the polygon's raw length" step in _depth_extent is actually
    # exercised rather than accidentally a no-op.
    poly = Polygon([(0, -0.2), (4, -0.2), (4, 0.2), (0, 0.2)])
    sym_xy = np.array([0.0, 1.0])          # axis = [-1, 0]: poly's x-extent is the length
    up = np.array([0.0, 0.0, 1.0])
    # Raw z-range 0.5 over raw length 4.0 -> depth_extent 0.125; canonicalised
    # beam is 0.2/4.0 = 0.05, so half-beam-doubled = 0.1 and depth/beam =
    # 1.25, comfortably above profile.MIN_DEPTH_TO_BEAM (0.15).
    full = np.array([[0.0, 0.0, 0.0], [4.0, 0.0, 0.5]])

    depth_extent = run._depth_extent(full, up, poly, sym_xy)
    assert abs(depth_extent - 0.125) < 1e-9, depth_extent

    canon = profile.canonicalise(poly, sym_xy)
    w, _ = profile.sample(canon)
    assert profile.implausible(w, depth_extent) is None


def test_depth_extent_bridges_to_profile_implausible_on_a_flat_card():
    # Same footprint, but a cloud flattened to 1% of its former z-range --
    # the case silhouette IoU is provably blind to and depth_extent alone
    # exists to catch.
    poly = Polygon([(0, -0.2), (4, -0.2), (4, 0.2), (0, 0.2)])
    sym_xy = np.array([0.0, 1.0])
    up = np.array([0.0, 0.0, 1.0])
    full = np.array([[0.0, 0.0, 0.0], [4.0, 0.0, 0.005]])

    depth_extent = run._depth_extent(full, up, poly, sym_xy)
    assert abs(depth_extent - 0.00125) < 1e-9, depth_extent

    canon = profile.canonicalise(poly, sym_xy)
    w, _ = profile.sample(canon)
    reason = profile.implausible(w, depth_extent)
    assert reason is not None and "flat card" in reason, reason


# --- Task 9 final review C1/C2/C3: new gates in run.process --------------
#
# Beyond I1's explicit test requirement, these three cover the new early-exit
# branches directly (the real batch re-run also exercises them on real art,
# but a monkeypatched unit test pins the routing logic itself rather than
# relying on which real ships happen to trip each gate this time).

def test_process_skips_moge_for_an_unkeyable_image(monkeypatch, tmp_path):
    monkeypatch.setattr(run.paths, "FOOTPRINT_ROOT", tmp_path)
    monkeypatch.setattr(run, "_stage_1",
                        lambda ship_id, image_path: ("img", "mask", 0.3, False, 50.9, 67.2))

    def fail_if_called(*a, **k):
        raise AssertionError("_stage_2_to_4 (camera/MoGe/mirror) must not run "
                             "on an unkeyable image")
    monkeypatch.setattr(run, "_stage_2_to_4", fail_if_called)

    result = run.process("scene_ship", pathlib.Path("a.webp"), alpha=3.0, background="neutral")
    assert result["status"] == "failed_unkeyable"
    assert result["quality"]["corner_spread"] == 50.9
    assert "environmental scene render" in result["reason"]


def test_process_reports_failed_symmetry_solve_when_sym_failure_is_set(monkeypatch, tmp_path):
    monkeypatch.setattr(run.paths, "FOOTPRINT_ROOT", tmp_path)
    monkeypatch.setattr(run, "_stage_1",
                        lambda ship_id, image_path: ("img", "mask", 0.3, True, 1.0, 1.0))
    fit = types.SimpleNamespace(confidence=0.02, source="auto")
    sym = mirror.Symmetry(normal=np.array([0.0, 0.0, 1.0]), offset=0.0, scale=1.0, shift=0.0,
                          residual=0.05, depth_separation=0.1,
                          failure="no candidate plane satisfied both constraints")
    monkeypatch.setattr(run, "_stage_2_to_4", lambda *a: (fit, None, sym))

    result = run.process("bad_solve_ship", pathlib.Path("a.webp"), alpha=3.0, background="neutral")
    assert result["status"] == "failed_symmetry_solve"
    assert result["reason"] == sym.failure
    assert result["quality"]["obliquity"] == sym.obliquity


def test_process_reports_failed_low_obliquity(monkeypatch, tmp_path):
    monkeypatch.setattr(run.paths, "FOOTPRINT_ROOT", tmp_path)
    monkeypatch.setattr(run, "_stage_1",
                        lambda ship_id, image_path: ("img", "mask", 0.3, True, 1.0, 1.0))
    fit = types.SimpleNamespace(confidence=0.02, source="auto")
    # normal mostly in-plane (x), tiny z component -> low obliquity (near bow-on).
    sym = mirror.Symmetry(normal=np.array([1.0, 0.0, 0.05]), offset=0.0, scale=1.0, shift=0.0,
                          residual=0.05, depth_separation=0.1, failure=None)
    assert sym.obliquity < mirror.OBLIQUITY_FLOOR, sym.obliquity
    monkeypatch.setattr(run, "_stage_2_to_4", lambda *a: (fit, None, sym))

    result = run.process("bow_on_ship", pathlib.Path("a.webp"), alpha=3.0, background="neutral")
    assert result["status"] == "failed_low_obliquity"
    assert "obliquity=" in result["reason"]


# --- Followup Item 1: resolve_heroes must accept .png, deterministically ---
#
# `paths.HERO_GLOB` was a single hardcoded "*.webp" pattern; the 402-image
# chromakey drop is .png. `paths.HERO_GLOBS` now globs both and
# `resolve_heroes` merges them into a sorted union, with an explicit
# preference rule for the case where the same ship id resolves from two
# files (a leftover .webp beside its .png replacement, most concretely).

def test_resolve_heroes_globs_both_extensions_and_prefers_exact_stem(monkeypatch, tmp_path, caplog):
    monkeypatch.setattr(run.paths, "HERO_DIR", tmp_path)
    monkeypatch.setattr(run, "CATALOG", str(tmp_path / "catalog.json"))
    (tmp_path / "catalog.json").write_text(json.dumps({"items": [
        {"id": "alpha"}, {"id": "bravo"}, {"id": "gamma"}, {"id": "prayer"},
    ]}))

    # Distinct ids across the two extensions: both must resolve, and neither
    # extension is preferred over the other in this case.
    (tmp_path / "alpha.webp").touch()
    (tmp_path / "bravo.png").touch()

    # Same id from both extensions, both exact-stem matches: the tie-break is
    # "first in sorted order", and "gamma.png" sorts before "gamma.webp".
    (tmp_path / "gamma.webp").touch()
    (tmp_path / "gamma.png").touch()

    # An exact-stem match must win over one that only matches after
    # faction-prefix stripping, even though the stripped file
    # ("outerrim_prayer.webp") sorts BEFORE the exact one ("prayer.png") --
    # proving this is a real preference rule, not just sort order.
    (tmp_path / "outerrim_prayer.webp").touch()
    (tmp_path / "prayer.png").touch()

    with caplog.at_level("WARNING"):
        heroes = run.resolve_heroes()

    assert heroes["alpha"] == tmp_path / "alpha.webp"
    assert heroes["bravo"] == tmp_path / "bravo.png"
    assert heroes["gamma"] == tmp_path / "gamma.png"
    assert heroes["prayer"] == tmp_path / "prayer.png"

    # Never silently double-process: exactly two ships resolved from more
    # than one file, and both were logged by name.
    assert "gamma.webp" in caplog.text and "gamma.png" in caplog.text
    assert "outerrim_prayer.webp" in caplog.text and "prayer.png" in caplog.text


def test_resolve_heroes_hero_glob_override_pins_a_single_pattern(monkeypatch, tmp_path):
    """SMKB_HERO_GLOB (paths.HERO_GLOBS, once set) narrows resolve_heroes to
    one pattern -- useful for pointing at a directory that mixes the new png
    drop with unrelated leftover webp files without picking either up.
    """
    monkeypatch.setattr(run.paths, "HERO_DIR", tmp_path)
    monkeypatch.setattr(run.paths, "HERO_GLOBS", ("*.png",))
    monkeypatch.setattr(run, "CATALOG", str(tmp_path / "catalog.json"))
    (tmp_path / "catalog.json").write_text(json.dumps({"items": [{"id": "alpha"}]}))
    (tmp_path / "alpha.webp").touch()
    (tmp_path / "alpha.png").touch()

    assert run.resolve_heroes() == {"alpha": tmp_path / "alpha.png"}

