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
import json
import re
import sys

import cv2
import numpy as np

from . import camera, gate, ground, matte, mirror, paths, pointmap, profile

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
    """
    ids = {s["id"] for s in json.load(open(CATALOG))["items"]}
    out = {}
    for p in sorted(paths.HERO_DIR.glob(paths.HERO_GLOB)):
        key = p.stem
        if key not in ids:
            key = _FACTION_PREFIX.sub("", key)
        if key in ids:
            out[key] = p
    return out


def _stage_1_to_4(ship_id, image_path, background):
    img = cv2.cvtColor(cv2.imread(str(image_path)), cv2.COLOR_BGR2RGB)
    mask, frac = matte.extract(img)

    clicks_path = paths.artifact_dir(ship_id) / "clicks.json"
    clicks = json.loads(clicks_path.read_text()) if clicks_path.exists() else None
    fit = camera.run(ship_id, img, mask, clicks=clicks)

    cloud = pointmap.run(ship_id, img, mask, background=background)
    # Real clouds are one-sided, so this routes to solve_from_view (Task 6b),
    # which needs the matte and the intrinsics to score silhouette agreement
    # and depth separation.
    sym = mirror.run(ship_id, cloud, mask)
    return img, mask, frac, fit, cloud, sym


def process(ship_id: str, image_path, alpha: float, background: str) -> dict:
    img, mask, frac, fit, cloud, sym = _stage_1_to_4(ship_id, image_path, background)

    full = mirror.complete(cloud.points, sym)
    # mirror.complete returns np.vstack([affine(points), reflect(affine(points))])
    # (see mirror.py's `complete`), so the first len(cloud.points) rows are the
    # visible half and the rest are the mirrored half -- verified by reading
    # complete()'s body, not assumed from call-site convention.
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
               "camera_confidence": fit.confidence, "camera_source": fit.source,
               "foreground_fraction": frac, "alpha": alpha}

    if not gate_result["mirrored_pass"]:
        return {"id": ship_id, "status": "failed_silhouette_gate", "quality": quality,
                "reason": gate_result.get("mirrored_pass_reason")}

    # No needs_clicks branch: stage 2 is a cross-check, not a gate (REVISED
    # 2026-07-29 — see Global Constraints). The frame comes from the recovered
    # geometry, so a low camera confidence no longer excludes a ship. Where the
    # fit IS confident, up_vector logs the agreement; it never overrides.
    up = ground.up_vector(sym, full, cloud.normals, fit=fit)
    poly = ground.run(ship_id, full, up, alpha)
    sym_xy = ground.project(sym.normal[None, :], up)[0]

    # depth_extent is in hull-length units — the units profile.implausible's
    # own tests use (beam comes from a footprint canonicalised to length == 1),
    # so the raw up-axis extent must be divided by the footprint's raw length.
    # Length is the POLYGON's extent along the longitudinal axis — the same
    # measure canonicalise normalises by — not the raw point extent:
    # ground.hull drops speck parts, and a dropped speck must not change the
    # denominator. (AMENDED 2026-07-31: the previous draft never passed
    # depth_extent at all, which left dimensional_pass True for every real
    # ship — the exact silent publish Task 8 says this check exists to
    # prevent — and never read dimensional_pass into status.)
    axis = np.array([-sym_xy[1], sym_xy[0]])
    axis /= np.linalg.norm(axis)
    geoms = poly.geoms if hasattr(poly, "geoms") else [poly]
    along = np.concatenate([np.array(g.exterior.coords) @ axis for g in geoms])
    depth_extent = float(np.ptp(full @ up)) / float(along.max() - along.min())
    data = profile.run(ship_id, poly, sym_xy, quality, depth_extent=depth_extent)
    data["status"] = "ok"
    if not data["dimensional_pass"]:
        data["status"] = "failed_dimensional_check"
    elif sym.residual > mirror.RESIDUAL_CEILING:
        data["status"] = "ok_asymmetric"
    return data


def _pick_alpha(heroes, background):
    """One alpha for the batch, from the clouds themselves."""
    clouds = []
    for ship_id, path in heroes.items():
        _, _, _, fit, cloud, sym = _stage_1_to_4(ship_id, path, background)
        full = mirror.complete(cloud.points, sym)
        up = ground.up_vector(sym, full, cloud.normals, fit=fit)
        clouds.append(ground.project(full, up))
    return ground.sweep_alpha(clouds)


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

    alpha = args.alpha if args.alpha else _pick_alpha(heroes, args.background)
    print(f"batch alpha = {alpha}")

    results = [process(i, p, alpha, args.background) for i, p in heroes.items()]
    ok = [r for r in results if r["status"].startswith("ok")]
    print(f"\nrecovered {len(ok)} / {len(results)}")
    for r in results:
        if not r["status"].startswith("ok"):
            print(f"  {r['id']:22s} {r['status']:26s} "
                  f"iou={r['quality']['silhouette_iou']:.2f} "
                  f"conf={r['quality']['camera_confidence']:.2f}")

    with open(args.report, "w") as f:
        json.dump({"alpha": alpha, "background": args.background,
                   "results": results}, f, indent=2)
    return 0


if __name__ == "__main__":
    sys.exit(main())
