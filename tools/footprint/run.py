#!/usr/bin/env python3
"""Batch driver for footprint recovery.

Runs all seven stages over every hero image that maps to a catalog ship, pins
one alpha for the batch, and writes a report separating recovered ships from
reconstruction failures. Failures are listed, never averaged in.

    ~/moge-venv/bin/python tools/footprint/run.py --all
    ~/moge-venv/bin/python tools/footprint/run.py --ship prayer
    SMKB_HERO_DIR=/path/to/drop ~/moge-venv/bin/python tools/footprint/run.py --all
"""

import argparse
import dataclasses
import json
import logging
import re
import sys

import cv2
import numpy as np

from . import camera, gate, ground, matte, mirror, paths, pointmap, profile

logger = logging.getLogger(__name__)

CATALOG = "/home/robert/spacemolt/spacemolt/data/game-api/latest/catalog_ships.json"
_FACTION_PREFIX = re.compile(r"^(crimson|nebula|solarian|outerrim|voidborn|pirate)_")


def resolve_heroes() -> dict:
    """Map ship ID to hero image path, for images whose name matches a ship.

    The raw stem is tried first, and only then the faction-prefix-stripped
    form. Order matters: five current ship IDs legitimately begin with a
    faction name (crimson_devastator, crimson_stiletto, nebula_tender,
    solarian_foundation, voidborn_event_horizon), so stripping first would
    turn crimson_devastator.webp into "devastator", match nothing, and drop
    the image silently. The stripped form is still needed because catalog IDs
    carried a faction prefix before ~March 2026 and some art is named for the
    old scheme — outerrim_prayer.webp is the ship now called "prayer". No ID
    collides under stripping, so this order is unambiguous.

    `paths.HERO_GLOBS` globs both `.webp` (the original 19-ship drop) and
    `.png` (the 402-image chromakey drop) and merges them into a single
    sorted union, so the same directory can hold both without either being
    ignored. Two files can legitimately resolve to the same ship id (a
    leftover .webp beside its .png replacement, or two files that both need
    faction-prefix stripping), and that must never mean silently double-
    processing the ship or silently dropping one file with no record of it:
    the exact-stem match wins over one that needed stripping, and among two
    matches of the same kind the first in sorted order wins -- both cases log
    a warning naming the files involved so a stale leftover is discoverable,
    not just invisible.
    """
    ids = {s["id"] for s in json.load(open(CATALOG))["items"]}
    files = sorted({p for pattern in paths.HERO_GLOBS for p in paths.HERO_DIR.glob(pattern)})

    out = {}         # ship id -> winning path
    exact = {}        # ship id -> whether the winner's stem matched with no stripping
    for p in files:
        stem = p.stem
        is_exact = stem in ids
        key = stem if is_exact else _FACTION_PREFIX.sub("", stem)
        if key not in ids:
            continue

        if key not in out:
            out[key] = p
            exact[key] = is_exact
        elif is_exact and not exact[key]:
            # The incumbent needed prefix-stripping to match; this file
            # didn't, so it wins regardless of sort order.
            logger.warning("ship %s resolves from multiple hero images (%s and %s); "
                           "using the exact-stem match %s", key, out[key], p, p)
            out[key] = p
            exact[key] = True
        else:
            # Either both exact or both stripped: `files` is already sorted,
            # so the incumbent -- the earlier file -- is the preferred one.
            logger.warning("ship %s resolves from multiple hero images (%s and %s); "
                           "using %s (first in sorted order)", key, out[key], p, out[key])
    return out


def _stage_1(ship_id, image_path):
    """Matte + keyability. Cheap and CPU-only, so this runs -- and can reject
    an image -- before any MoGe inference is spent on it (task-9 final
    review C1).

    `matte.run` recomputes `extract()`/`keyability()` a second time
    internally to do its own file-writing (`matte.png`, `keyability.json`);
    that's cheap enough here (no GPU, no MoGe) that it isn't worth threading
    those internals back out of `matte.run` just to avoid the duplicate
    call.
    """
    img = cv2.cvtColor(cv2.imread(str(image_path)), cv2.COLOR_BGR2RGB)
    mask, frac = matte.extract(img)
    is_keyable, corner_spread, border_std = matte.keyability(img)
    matte.run(ship_id, img)
    return img, mask, frac, is_keyable, corner_spread, border_std


def _stage_2_to_4(ship_id, img, mask, background):
    clicks_path = paths.artifact_dir(ship_id) / "clicks.json"
    clicks = json.loads(clicks_path.read_text()) if clicks_path.exists() else None
    fit = camera.run(ship_id, img, mask, clicks=clicks)

    cloud = pointmap.run(ship_id, img, mask, background=background)
    # Real clouds are one-sided, so this routes to solve_from_view (Task 6b),
    # which needs the matte and the intrinsics to score silhouette agreement
    # and depth separation.
    sym = mirror.run(ship_id, cloud, mask)
    return fit, cloud, sym


def _fail(ship_id, status, quality, reason):
    """Build a failure record, and clear any stale profile.json.

    A ship that used to reach stage 7 and now fails earlier (a stricter gate,
    a worse solve on a re-run) must not leave behind a profile.json this run
    never wrote -- see task-9 final review I3. `missing_ok=True`: most
    failing ships never had one.
    """
    (paths.artifact_dir(ship_id) / "profile.json").unlink(missing_ok=True)
    return {"id": ship_id, "status": status, "quality": quality, "reason": reason}


def _depth_extent(full, up, poly, sym_xy) -> float:
    """The completed cloud's up-axis extent, in HULL-LENGTH units.

    Hull-length units because that is what `profile.implausible` and its own
    tests use (beam comes from a footprint canonicalised to length == 1), so
    the raw up-axis extent must be divided by the footprint's raw length.
    Length is the POLYGON's extent along the longitudinal axis -- the same
    measure `canonicalise` normalises by -- not the raw point extent:
    `ground.hull` drops speck parts, and a dropped speck must not change the
    denominator. (AMENDED 2026-07-31: an earlier draft never passed
    depth_extent at all, which left `dimensional_pass` True for every real
    ship -- the exact silent publish Task 8 says this check exists to
    prevent.)

    Factored out of `process()` (task-9 final review I1) so it has its own
    seam to test with known synthetic geometry -- this expression is the
    ONLY place a scale or depth error can still be caught (see profile.py's
    comment on `ASPECT_BOUNDS`), so it needs its own coverage, not just a
    real-batch smoke test.
    """
    axis = np.array([-sym_xy[1], sym_xy[0]])
    axis /= np.linalg.norm(axis)
    geoms = poly.geoms if hasattr(poly, "geoms") else [poly]
    along = np.concatenate([np.array(g.exterior.coords) @ axis for g in geoms])
    return float(np.ptp(full @ up)) / float(along.max() - along.min())


@dataclasses.dataclass
class _Stage4Result:
    """What Phase A (stages 1-4) hands to Phase B (stages 5-7).

    `frac`, `fit`, `cloud`, `sym` and `mask` are exactly `process()`'s own
    local variables at the point stages 1-4 finish successfully, bundled so
    Phase A and Phase B can be two separately callable halves of what used to
    be one function (Item 2's stages-1-4-once restructure) without changing
    either half's behaviour.

    `full` is `None` from `_process_phase_a` -- Phase A does not itself need
    the completed (mirrored) cloud, only `sym`, so it leaves that to whichever
    half of the pipeline needs it: `process()`'s own single-ship path
    recomputes it from `cloud`/`sym` exactly as the old inline code did,
    while the batch path (`_run_all`/`_reload_for_phase_b`) reloads it
    straight from `cloud_resolved.npz` (`mirror.load` already wrote the
    completed cloud there, so recomputing it a second time would be exactly
    the kind of redundant stage-4-adjacent work Item 2 exists to remove).
    """
    frac: float
    fit: camera.Fit
    cloud: pointmap.Cloud
    sym: mirror.Symmetry
    mask: np.ndarray
    full: np.ndarray | None = None


def _process_phase_a(ship_id: str, image_path, background: str):
    """Stages 1-4: matte/keyability, camera fit, MoGe cloud, symmetry solve.

    Returns a `_Stage4Result` on success, or a failure dict (from `_fail`) the
    moment an unkeyable image, a self-reported symmetry-solve failure, or a
    too-close-to-bow-on view rules the ship out. Factored out of `process()`
    (Item 2) so `process()`'s single-ship path and the batch driver's Phase A
    pass share one copy of these gates instead of each re-implementing them --
    see this module's docstring on the stages-1-4-once restructure.
    """
    img, mask, frac, is_keyable, corner_spread, border_std = _stage_1(ship_id, image_path)
    if not is_keyable:
        # Environmental scene renders (a ship in a hall, a cavern, a hangar)
        # still yield a plausible-looking foreground fraction from a colour-
        # distance threshold -- matte.extract segments scenery, not the
        # subject -- so every downstream stage would run on garbage. Caught
        # here, before MoGe, rather than downstream where it's expensive and
        # indirect (task-9 final review C1).
        return _fail(ship_id, "failed_unkeyable",
                     {"foreground_fraction": frac, "corner_spread": corner_spread,
                      "border_std": border_std},
                     f"corner_spread={corner_spread:.2f} (floor "
                     f"{matte.CORNER_SPREAD_THRESHOLD}) / border_std={border_std:.2f} "
                     f"(floor {matte.BORDER_STD_THRESHOLD}): background is not a flat "
                     "chroma key, this looks like an environmental scene render")

    fit, cloud, sym = _stage_2_to_4(ship_id, img, mask, background)

    if sym.failure is not None:
        # `Symmetry.failure`: "Non-None means DO NOT TRUST this result... a
        # field rather than an exception because the measured numbers are
        # worth recording for the failing ship" -- mirror.py's own words.
        # Never read before this fix (task-9 final review C3): gate.run has
        # no proximity check of its own, so a proximity-infeasible solve
        # (the receded-plane degeneracy `mirror._PROXIMITY_CEILING` exists to
        # reject) could clear `mirrored_pass` anyway and publish as ok.
        return _fail(ship_id, "failed_symmetry_solve",
                     {"foreground_fraction": frac, "camera_confidence": fit.confidence,
                      "camera_source": fit.source, "mirror_residual": sym.residual,
                      "obliquity": sym.obliquity},
                     sym.failure)

    if sym.obliquity < mirror.OBLIQUITY_FLOOR:
        # A near-bow-on view can still clear every check solve_from_view runs
        # on itself while sitting in a regime those checks are documented not
        # to discriminate in -- see mirror.OBLIQUITY_FLOOR's comment (task-9
        # final review C2).
        return _fail(ship_id, "failed_low_obliquity",
                     {"foreground_fraction": frac, "camera_confidence": fit.confidence,
                      "camera_source": fit.source, "mirror_residual": sym.residual,
                      "obliquity": sym.obliquity},
                     f"obliquity={sym.obliquity:.3f} is below "
                     f"mirror.OBLIQUITY_FLOOR={mirror.OBLIQUITY_FLOOR}: near bow-on, "
                     "where the depth-separation term cannot reliably distinguish a "
                     "fold from a genuine occluded half")

    return _Stage4Result(frac=frac, fit=fit, cloud=cloud, sym=sym, mask=mask)


def _process_phase_b(ship_id: str, stage4: _Stage4Result, alpha: float) -> dict:
    """Stages 5-7: silhouette gate, ground projection, profile.

    Takes the `_Stage4Result` a successful `_process_phase_a` produced --
    either straight from Phase A (process()'s single-ship path) or
    reconstructed from disk (`_reload_for_phase_b`, the batch path) -- and
    never touches stages 1-4 itself.
    """
    cloud, sym, mask, fit, frac = (stage4.cloud, stage4.sym, stage4.mask,
                                   stage4.fit, stage4.frac)
    full = stage4.full if stage4.full is not None else mirror.complete(cloud.points, sym)
    # mirror.complete returns np.vstack([affine(points), reflect(affine(points))])
    # (see mirror.py's `complete`), so the first len(cloud.points) rows are the
    # visible half and the rest are the mirrored half -- verified by reading
    # complete()'s body, not assumed from call-site convention. Holds equally
    # for a reloaded `full`: `mirror.run` writes `cloud_resolved.npz` from
    # this exact same `complete()` call, so `mirror.load`'s `full` has the
    # same layout.
    n = len(cloud.points)
    assert len(full) == 2 * n, (len(full), n)
    mirrored = full[n:]

    # gate.run's mirrored_pass is the stage-5 verdict (union IoU was demoted to
    # diagnostic-only in Task 6's fix rounds: `uv = K.p / p_z` is invariant
    # under any positive per-point depth scale, so IoU cannot see a folded or
    # rescaled reconstruction -- see gate.IOU_FLOOR's comment). z_extent is the
    # RAW ONE-VIEW visible cloud's z-range, matching what mirror.solve_from_view
    # itself used internally to derive sym.depth_separation, and what
    # test_gate.py's own fixtures use.
    z_extent = float(cloud.points[:, 2].max() - cloud.points[:, 2].min())
    gate_result = gate.run(ship_id, cloud.points, mirrored, cloud.intrinsics, mask,
                           depth_separation=sym.depth_separation, z_extent=z_extent)
    quality = {"silhouette_iou": gate_result["silhouette_iou"],
               "mirrored_fraction": gate_result["mirrored_fraction"],
               "mirror_residual": sym.residual,
               "obliquity": sym.obliquity,
               "camera_confidence": fit.confidence, "camera_source": fit.source,
               "foreground_fraction": frac, "alpha": alpha}

    if not gate_result["mirrored_pass"]:
        return _fail(ship_id, "failed_silhouette_gate", quality,
                     gate_result.get("mirrored_pass_reason"))

    # No needs_clicks branch: stage 2 is a cross-check, not a gate (REVISED
    # 2026-07-29 — see Global Constraints). The frame comes from the recovered
    # geometry, so a low camera confidence no longer excludes a ship. Where the
    # fit IS confident, up_vector logs the agreement; it never overrides.
    up = ground.up_vector(sym, full, cloud.normals, fit=fit)
    poly = ground.run(ship_id, full, up, alpha)
    sym_xy = ground.project(sym.normal[None, :], up)[0]

    depth_extent = _depth_extent(full, up, poly, sym_xy)
    data = profile.run(ship_id, poly, sym_xy, quality, depth_extent=depth_extent)
    data["status"] = "ok"
    if not data["dimensional_pass"]:
        data["status"] = "failed_dimensional_check"
    elif sym.residual > mirror.RESIDUAL_CEILING:
        data["status"] = "ok_asymmetric"
    # profile.run already wrote profile.json, but WITHOUT `status` -- it is
    # computed here, after profile.run returns, from fields profile.run does
    # not itself know the meaning of (RESIDUAL_CEILING is mirror's). Rewrite
    # the file so a consumer reading profile.json alone (not report.json)
    # still sees the verdict (task-9 final review I3).
    (paths.artifact_dir(ship_id) / "profile.json").write_text(json.dumps(data, indent=2))
    return data


def process(ship_id: str, image_path, alpha: float, background: str) -> dict:
    """Run every stage for one ship. Unchanged behaviour and statuses (Item
    2's brief) -- only the body is now `_process_phase_a` followed by
    `_process_phase_b`, so the batch driver (`_run_all`) can reuse the exact
    same gates without paying for stages 1-4 twice.
    """
    stage4 = _process_phase_a(ship_id, image_path, background)
    if isinstance(stage4, dict):
        return stage4
    return _process_phase_b(ship_id, stage4, alpha)


def _reload_for_phase_b(ship_id: str, fit: camera.Fit) -> _Stage4Result:
    """Reconstruct one ship's Phase A result from its artifacts, for Phase B.

    `_run_all` does not keep Phase A's in-memory result alive for the whole
    batch -- a completed cloud is ~19MB per ship (Item 2's brief), and 402 of
    those does not fit in memory at once -- so Phase B reloads what stages
    6-7 need from what Phase A already wrote to disk instead: the matte
    (`matte.png`, stage 1), the MoGe cloud (`cloud.npz`, stage 3, via
    `pointmap.load`) and the solved symmetry plus completed cloud
    (`cloud_resolved.npz`, stage 4, via `mirror.load` -- both previously had
    no caller). `foreground_fraction` is recomputed as `mask.mean()` rather
    than persisted anywhere new, since it's one float and matches exactly what
    `_stage_1` computed it as originally (`matte.extract`'s own return value).

    `fit` is the one piece the caller must supply rather than this function
    reloading it: a `camera.Fit` is a handful of scalars and a 3x3 rotation,
    not an array that scales with point count, so `_run_all` just keeps it in
    memory across the batch instead of adding a reader to `camera.json` --
    `camera.py` is outside this restructure's file scope, and `camera.json`
    already has no loader to begin with.
    """
    matte_path = paths.artifact_dir(ship_id) / "matte.png"
    mask = (cv2.imread(str(matte_path), cv2.IMREAD_GRAYSCALE) > 0).astype(np.uint8)
    cloud = pointmap.load(ship_id)
    sym, full = mirror.load(ship_id)
    return _Stage4Result(frac=float(mask.mean()), fit=fit, cloud=cloud, sym=sym,
                         mask=mask, full=full)


def _run_all(heroes: dict, background: str, alpha: float | None = None):
    """The production `--all`/`--ship` driver: stages 1-4 run once per ship.

    Phase A runs stages 1-4 for every ship via `_process_phase_a`. A ship
    that fails one of its gates (or raises) gets its final result right here
    and never reaches Phase B. A ship that clears them contributes its
    subsampled ground xy to the alpha sweep (task-9 final review C4's
    subsampling requirement) -- computed inline, once, right after Phase A
    returns. Only that small array and the ship's tiny `camera.Fit` are kept
    past this loop -- not the cloud, the mask, or the completed points -- so
    memory stays bounded across a batch of hundreds of ships (see
    `_reload_for_phase_b`).

    Alpha is picked once from every surviving ship's xy, unless the caller
    already pins one (`--alpha` on the command line). Phase B then reloads
    each surviving ship's stage 1-4 artifacts from disk and runs stages 5-7 --
    so stages 1-4 never run twice for any ship. This replaces an earlier
    `_pick_alpha` (alpha sweep, via a `_ship_ground_xy` helper) composed with
    `_run_batch` (calling `process()` per ship) that ran stages 1-4 TWICE per
    ship -- once in each half of that composition -- for the same heroes
    dict (Item 2's brief). Those three functions and the isolation tests that
    exercised them directly were retired once their properties -- per-ship
    exception isolation in both phases, KeyboardInterrupt/SystemExit
    propagation, and the all-ships-failed RuntimeError -- were ported onto
    `_run_all` itself below (fix round 1: dead code protected by tests of
    dead code protects nothing).

    Per-ship isolation: an exception from `_process_phase_a` (Phase A) or
    from `_reload_for_phase_b`/`_process_phase_b` (Phase B) is caught,
    recorded as `failed_exception`, and does not stop the rest of the batch
    -- see task-9 fix round 1's original rationale, which still applies
    here. Only `KeyboardInterrupt`/`SystemExit` propagate.

    Returns `(alpha, results)`, with `results` in the same order as `heroes`
    regardless of which ships dropped out in Phase A vs. Phase B.
    """
    fits = {}
    xys = []
    results = {}
    for ship_id, path in heroes.items():
        try:
            outcome = _process_phase_a(ship_id, path, background)
        except (KeyboardInterrupt, SystemExit):
            raise
        except Exception as e:
            logger.warning("ship %s raised during stages 1-4: %s: %s",
                           ship_id, type(e).__name__, e)
            (paths.artifact_dir(ship_id) / "profile.json").unlink(missing_ok=True)
            results[ship_id] = {"id": ship_id, "status": "failed_exception", "quality": {},
                                "reason": f"{type(e).__name__}: {e}"}
            continue

        if isinstance(outcome, dict):
            results[ship_id] = outcome
            continue

        full = mirror.complete(outcome.cloud.points, outcome.sym)
        up = ground.up_vector(outcome.sym, full, outcome.cloud.normals, fit=outcome.fit)
        xys.append((ship_id, ground.subsample(ground.project(full, up))))
        fits[ship_id] = outcome.fit

    if alpha is None:
        if not xys:
            raise RuntimeError(
                "every ship failed stages 1-4; cannot pick a batch alpha from zero clouds")
        alpha = ground.sweep_alpha([xy for _, xy in xys])

    for ship_id, _ in xys:
        try:
            stage4 = _reload_for_phase_b(ship_id, fits[ship_id])
            results[ship_id] = _process_phase_b(ship_id, stage4, alpha)
        except (KeyboardInterrupt, SystemExit):
            raise
        except Exception as e:
            logger.warning("ship %s raised during processing: %s: %s",
                           ship_id, type(e).__name__, e)
            (paths.artifact_dir(ship_id) / "profile.json").unlink(missing_ok=True)
            results[ship_id] = {"id": ship_id, "status": "failed_exception", "quality": {},
                                "reason": f"{type(e).__name__}: {e}"}

    return alpha, [results[ship_id] for ship_id in heroes if ship_id in results]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--ship")
    ap.add_argument("--background", choices=["raw", "neutral"], default="neutral")
    ap.add_argument("--alpha", type=float)
    ap.add_argument("--report", default="data/footprints/report.json")
    args = ap.parse_args()

    heroes = resolve_heroes()
    if args.ship:
        heroes = {args.ship: heroes[args.ship]}
    if not heroes:
        print("no hero images matched a catalog ship", file=sys.stderr)
        return 1

    # `args.alpha`, when given, is passed straight through and skips the
    # sweep entirely -- see `_run_all`'s own docstring for what this replaced.
    alpha, results = _run_all(heroes, args.background, alpha=args.alpha)
    print(f"batch alpha = {alpha}")

    ok = [r for r in results if r["status"].startswith("ok")]
    print(f"\nrecovered {len(ok)} / {len(results)}")
    for r in results:
        if not r["status"].startswith("ok"):
            # .get(), not r["quality"][...]: a failed_exception record's
            # `quality` is empty (the stage that would have filled it in is
            # exactly what raised), so this must not itself raise while
            # reporting a failure.
            q = r.get("quality", {})
            iou = q.get("silhouette_iou")
            conf = q.get("camera_confidence")
            print(f"  {r['id']:22s} {r['status']:26s} "
                  f"iou={'n/a' if iou is None else f'{iou:.2f}'} "
                  f"conf={'n/a' if conf is None else f'{conf:.2f}'}")

    with open(args.report, "w") as f:
        json.dump({"alpha": alpha, "background": args.background,
                   "results": results}, f, indent=2)
    return 0


if __name__ == "__main__":
    sys.exit(main())
