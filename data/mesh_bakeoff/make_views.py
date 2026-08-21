#!/usr/bin/env python3
"""Project side + front elevation silhouettes for the 3-view blueprints.

Stage 2 of the registry-blueprint feature. For every mapped catalog ship
with a Hy3D mesh, union-project the mesh triangles onto the (longitudinal,
up) plane -> SIDE elevation and (lateral, up) plane -> FRONT elevation,
in EXACTLY the frame the shipped top-view footprint used:

  * frame + solo + rot90 + stretch replayed from adjustments-final.json
    via apply_adjustments.py's own functions (all 402 hulls were
    re-extracted with top_view "td", i.e. world axes, up = +Y);
  * bow-right flip replayed from make_svg_footprints.bow_flip on the
    committed footprint polygon, so the side view's bow matches the SVG;
  * `mirror` (port/starboard) flips the front view; a new `vflip`
    adjustment key flips dorsal/ventral if a hull comes out upside down.

Output (committed): ../footprints/views/<kb_ship_id>.json with SVG path
data (fill-rule evenodd, holes real) in corner-origin coordinates on the
footprint's scale: hull length = 1000 units, so height_m = loa_m *
height_units / 1000.

    ~/sf3d-venv/bin/python make_views.py [--jobs 8] [stem ...]
"""
import argparse
import json
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import apply_adjustments as aa          # noqa: E402  (forced_frame, solo_hull)
import mesh_footprint                   # noqa: E402  (build_footprint, pca_frame)
from make_svg_footprints import bow_flip, rings_of  # noqa: E402

OUTDIR = HERE.parent / "footprints" / "views"
LENGTH = 1000.0     # same normalised hull length as the footprint SVGs
SIMPLIFY = 1.2      # simplify tolerance in those units (0.12% of length)


def frame_for(verts: np.ndarray, adj: dict):
    """Replicate apply_adjustments.apply_one's frame selection."""
    view = adj.get("top_view", "td")
    if view != "pca":
        lateral, longitudinal, up = aa.forced_frame(verts, aa.VIEW_UP_AXIS[view])
    else:
        centroid, axes, _ = mesh_footprint.pca_frame(verts)
        longitudinal = axes[:, 0]
        lateral, up, _, _ = mesh_footprint.find_lateral_axis(
            verts, centroid, longitudinal, [axes[:, 1], axes[:, 2]],
            np.random.default_rng(0))
    if adj.get("rot90"):
        lateral, longitudinal = longitudinal, lateral
    return lateral, longitudinal, up


def poly_path(poly, s: float):
    """Shapely polygon -> corner-origin SVG path data at scale s.
    Returns (path_d, (width, height), origin) in scaled units."""
    from shapely.geometry import mapping
    poly = poly.simplify(SIMPLIFY / s, preserve_topology=True)
    geo = mapping(poly)
    rings = [np.asarray(r, dtype=float) for r, _ in rings_of(geo)]
    allp = np.vstack(rings)
    lo = allp.min(axis=0)
    parts = []
    for r in rings:
        px = (r - lo) * s
        parts.append("M" + "L".join(f"{p[0]:.1f} {p[1]:.1f}" for p in px) + "Z")
    dims = (allp.max(axis=0) - lo) * s
    return "".join(parts), (float(dims[0]), float(dims[1])), lo


def declutter(poly, eps_close: float, eps_open: float):
    """Silhouette cleanup. Closing (always, small) merges the doubled rim
    outlines that near-surface mesh folds project (crimson_bonesaw's front
    jaggies). Opening (opt-in via the per-stem "clean" adjustment) removes
    hairline slivers from internal sheets poking through the hull
    (bonesaw's transverse fin) -- opt-in because it would also erase
    genuine thin masts (absolute_zero)."""
    out = poly.buffer(eps_close).buffer(-eps_close)
    if eps_open > 0:
        out = out.buffer(-eps_open).buffer(eps_open)
    if out.geom_type == "MultiPolygon":
        # drop speck fragments (isolated projection noise); real detached
        # hull parts (catamaran pontoons) are orders of magnitude larger
        big = max(g.area for g in out.geoms)
        keep = [g for g in out.geoms if g.area >= 0.008 * big]
        if keep:
            from shapely.ops import unary_union
            out = unary_union(keep)
    return poly if out.is_empty else out.buffer(0)


def deroll(lat: np.ndarray, scr_y: np.ndarray, forced: float | None = None):
    """Estimate hull roll about the bow-stern axis + the symmetry axis.

    Hy3D reconstructions carry a degree or two of roll that the top-view
    footprint flattens invisibly but the front elevation exposes as a
    visible tilt (billhook). Ships are bilaterally symmetric, so the roll
    is the small rotation of the front-view point cloud that best mirrors
    it onto itself; the mirror line of the best fit is the centerline.
    `forced` (the per-stem "roll" adjustment, degrees) bypasses the fit
    for naturally lopsided hulls the symmetry search cannot judge
    (dosimetry's angled wings). Returns (rot2x2, xc) with xc in the
    ROTATED centered frame."""
    from scipy.spatial import cKDTree
    rng = np.random.default_rng(0)
    idx = rng.choice(lat.shape[0], size=min(4000, lat.shape[0]),
                     replace=False)
    P0 = np.column_stack([lat[idx], scr_y[idx]])
    ctr = P0.mean(axis=0)
    P0 = P0 - ctr

    def rot(deg):
        c, s = np.cos(np.radians(deg)), np.sin(np.radians(deg))
        return np.array([[c, -s], [s, c]])

    def score(deg):
        q = P0 @ rot(deg)
        d, _ = cKDTree(q).query(np.column_stack([-q[:, 0], q[:, 1]]))
        return d.mean()

    if forced is not None:
        best = float(forced)
    else:
        best = min(np.arange(-6.0, 6.01, 1.0), key=score)
        best = min(np.arange(best - 1, best + 1.01, 0.25), key=score)
        # accept only a CLEAR symmetry gain: on genuinely asymmetric hulls
        # (start_praying's flank blaster) the mirror-fit otherwise wanders
        # a few degrees chasing noise and tilts a level hull
        if best and score(best) > 0.85 * score(0.0):
            best = 0.0

    q = P0 @ rot(best)
    span = np.ptp(q[:, 0])
    tree = cKDTree(q)

    def xscore(xc):
        d, _ = tree.query(np.column_stack([2 * xc - q[:, 0], q[:, 1]]))
        return d.mean()

    xc = min(np.linspace(-0.12 * span, 0.12 * span, 25), key=xscore)
    return rot(best), ctr, float(xc), float(best)


def silhouette(x: np.ndarray, y: np.ndarray, faces: np.ndarray):
    """build_footprint with a snap-rounding fallback: a few meshes trip
    GEOS "ring edge missing" topology errors in the plain union."""
    try:
        return mesh_footprint.build_footprint(np.column_stack([x, y]), faces)
    except Exception:
        import shapely
        from shapely.geometry import Polygon
        pts2 = np.column_stack([x, y])
        span = max(np.ptp(x), np.ptp(y))
        tris = [p for f in faces
                if not (p := Polygon(pts2[f])).is_empty and p.area > 0]
        return shapely.union_all(tris, grid_size=span * 1e-6).buffer(0)


def make_one(job) -> str:
    stem, ship_id, adj = job
    import trimesh
    d = HERE / "out-hy3d-full" / stem
    mesh = trimesh.load(d / "mesh.obj", force="mesh", process=False)
    if adj.get("solo"):
        mesh = aa.solo_hull(mesh)
    verts = np.asarray(mesh.vertices, dtype=float)
    faces = np.asarray(mesh.faces, dtype=int)
    centroid = verts.mean(axis=0)
    lateral, longitudinal, up = frame_for(verts, adj)

    stretch = float(adj.get("stretch", 1.0))
    if abs(stretch - 1.0) > 1e-3:
        centered = verts - centroid
        verts = centroid + centered + (stretch - 1.0) * np.outer(
            centered @ longitudinal, longitudinal)

    centered = verts - centroid
    long_p = centered @ longitudinal
    lat_p = centered @ lateral
    up_p = centered @ up

    # bow-right exactly as the committed footprint SVG has it
    fp = json.loads((d / "footprint.json").read_text())
    flip = bow_flip(rings_of(fp["polygon"]), bool(adj.get("flip")))
    if flip:
        long_p = -long_p
    if adj.get("mirror"):
        lat_p = -lat_p
    # bow-on convention: the front elevation is viewed FROM AHEAD, so the
    # ship's port side sits on the viewer's right. Without this flip the
    # view reads as a stern view and every asymmetric feature (deeprock_
    # harvester's port derrick) appears mirrored.
    lat_p = -lat_p
    scr_y = up_p if adj.get("vflip") else -up_p   # dorsal up on screen

    # de-roll + true centerline: rotate out the reconstruction's slight
    # roll and put the bilateral symmetry axis at lat = 0
    forced = adj.get("roll")
    rot, ctr, xc, roll_deg = deroll(
        lat_p, scr_y, float(forced) if forced is not None else None)
    P = (np.column_stack([lat_p, scr_y]) - ctr) @ rot
    lat_p, scr_y = P[:, 0] - xc, P[:, 1]

    s = LENGTH / np.ptp(long_p)
    clean = float(adj.get("clean", 0))
    eps_close, eps_open = max(1.5, clean) / s, clean / s
    side = declutter(silhouette(long_p, scr_y, faces), eps_close, eps_open)
    front = declutter(silhouette(lat_p, scr_y, faces), eps_close, eps_open)
    if adj.get("sym"):
        # the top view is mirror-unioned for known-symmetric lopsided
        # reconstructions; give the front elevation the same treatment
        # (mirror across the centerline found above)
        from shapely.affinity import scale as shp_scale
        from shapely.ops import unary_union
        front = unary_union(
            [front, shp_scale(front, xfact=-1, origin=(0, 0))]).buffer(0)
    side_d, (side_w, side_h), _ = poly_path(side, s)
    front_d, (front_w, front_h), flo = poly_path(front, s)

    out = {
        "ship": ship_id, "stem": stem,
        "len_units": round(side_w, 1),          # ~= 1000 by construction
        "height_units": round(side_h, 1),
        "beam_units": round(front_w, 1),
        "front_center_units": round(-flo[0] * s, 1),   # centerline x
        "roll_deg": round(roll_deg, 2),
        "side": side_d, "front": front_d,
        "adjustments": adj,
    }
    (OUTDIR / f"{ship_id}.json").write_text(json.dumps(out))
    return f"{stem:34} -> {ship_id}  h={side_h:.0f}u beam={front_w:.0f}u"


def _safe_entry(job) -> str:      # top-level: must pickle for the Pool
    try:
        return make_one(job)
    except Exception as exc:                          # keep the batch alive
        return f"{job[0]:34} FAILED {type(exc).__name__}: {exc}"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--jobs", type=int, default=8)
    ap.add_argument("stems", nargs="*", help="limit to these art stems")
    args = ap.parse_args()

    adjustments = json.loads((HERE / "adjustments-final.json").read_text())
    idmap = json.loads((HERE / "ship_id_map.json").read_text())
    dup_stems = {d["stem"] for d in idmap["duplicates"]}
    OUTDIR.mkdir(parents=True, exist_ok=True)

    jobs = []
    for stem, m in sorted(idmap["mapping"].items()):
        if stem in dup_stems or (args.stems and stem not in args.stems):
            continue
        d = HERE / "out-hy3d-full" / stem
        if not (d / "mesh.obj").exists() or not (d / "footprint.json").exists():
            continue
        jobs.append((stem, m["id"], adjustments.get(stem, {})))

    if args.jobs > 1 and len(jobs) > 1:
        from multiprocessing import Pool
        with Pool(args.jobs) as pool:
            for line in pool.imap_unordered(_safe_entry, jobs):
                print(line, flush=True)
    else:
        for job in jobs:
            print(_safe_entry(job), flush=True)
    print(f"{len(jobs)} view files -> {OUTDIR}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
