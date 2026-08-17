#!/usr/bin/env python3
"""Apply triage-sheet adjustments (rotation snap + bow-stern stretch).

Input is the JSON the sheet's "export adjustments" button produces:

    { "voidborn_dissolution": {"top_view": "side", "stretch": 1.15}, ... }

`top_view` names which rendered candidate is the TRUE top-down (td / side /
front, i.e. which world axis of the generator's canonical frame is "up");
omitted means keep the extractor's own PCA/chamfer frame. `stretch` scales
the hull along the bow-stern axis -- the standing finding is that Hy3D
recovers silhouette character but compresses proportion, so this is the
knob that reconciles a good mesh with the pipeline's aspect.

Re-extracts footprint.json / profile.json through the same stage-7 sampler
as mesh_footprint.py (so numbers stay comparable), redraws outline_hy3d.png,
and writes mesh_adjusted.obj when a stretch is applied. Renders are NOT
re-shot -- they still show the unstretched mesh. Re-run make_triage_sheet.py
afterwards to refresh the sheet's stats from the new profiles.

    ~/sf3d-venv/bin/python apply_adjustments.py adjustments.json [--dir out-hy3d-full]
"""

import argparse
import json
import sys
from pathlib import Path

import numpy as np
import trimesh
from shapely.geometry import mapping

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(Path("/home/robert/spacemolt/kb")))
from tools.footprint import profile  # noqa: E402

import mesh_footprint  # noqa: E402  (pca_frame, find_lateral_axis, build_footprint)
from make_triage_sheet import draw_outline, polygon_rings  # noqa: E402

# which world axis the chosen candidate view looks along (= "up" for the
# footprint plane); must match render_mesh.py's look_at() conventions
VIEW_UP_AXIS = {"td": 1, "side": 0, "front": 2}


def forced_frame(verts: np.ndarray, up_idx: int):
    """Footprint frame from a user-chosen up axis: the two in-plane world
    axes, longitudinal = the one with the larger extent."""
    axes = [i for i in range(3) if i != up_idx]
    spans = [np.ptp(verts[:, i]) for i in axes]
    long_i = axes[int(np.argmax(spans))]
    lat_i = axes[int(np.argmin(spans))] if spans[0] != spans[1] else axes[1 - int(np.argmax(spans))]
    basis = np.eye(3)
    return basis[:, lat_i], basis[:, long_i], basis[:, up_idx]


def apply_one(d: Path, adj: dict) -> dict:
    mesh = trimesh.load(d / "mesh.obj", force="mesh", process=False)
    if adj.get("solo"):
        # keep only the main hull: companion objects in the hero (swarm's
        # launching drones) are faithfully reconstructed as separate bodies,
        # but they are not hull and must not count toward the footprint.
        # (Hand-rolled components: this venv's old trimesh calls the
        # numpy-2.x-removed ndarray.ptp() inside mesh.split().)
        import scipy.sparse as sp
        from scipy.sparse.csgraph import connected_components
        v = np.asarray(mesh.vertices)
        f = np.asarray(mesh.faces)
        e = np.vstack([f[:, [0, 1]], f[:, [1, 2]], f[:, [2, 0]]])
        g = sp.coo_matrix((np.ones(len(e)), (e[:, 0], e[:, 1])),
                          shape=(len(v), len(v)))
        _, lab = connected_components(g, directed=False)
        flab = lab[f[:, 0]]
        keep = flab == np.bincount(flab).argmax()
        mesh = trimesh.Trimesh(vertices=v, faces=f[keep], process=False)
    verts = np.asarray(mesh.vertices, dtype=float)
    faces = np.asarray(mesh.faces, dtype=int)
    centroid = verts.mean(axis=0)

    # default frame is the canonical top-down ('td') -- the triage pass
    # showed it is right for essentially every upright hero. 'pca' opts
    # back into the extractor's discovered frame for the exceptions.
    view = adj.get("top_view", "td")
    if view != "pca":
        lateral, longitudinal, up = forced_frame(verts, VIEW_UP_AXIS[view])
        frame_ambiguous = False
    else:
        centroid, axes, _ = mesh_footprint.pca_frame(verts)
        longitudinal = axes[:, 0]
        lateral, up, _, frame_ambiguous = mesh_footprint.find_lateral_axis(
            verts, centroid, longitudinal, [axes[:, 1], axes[:, 2]],
            np.random.default_rng(0))

    if adj.get("rot90"):
        # near-square hulls (aspect ~1) make the longer-axis-is-longitudinal
        # tie-break a coin flip; this swaps the in-plane roles when it lands
        # wrong (nebula_portfolio: wingspan == hull length)
        lateral, longitudinal = longitudinal, lateral

    stretch = float(adj.get("stretch", 1.0))
    if abs(stretch - 1.0) > 1e-3:
        centered = verts - centroid
        verts = centroid + centered + (stretch - 1.0) * np.outer(
            centered @ longitudinal, longitudinal)
        trimesh.Trimesh(vertices=verts, faces=faces).export(d / "mesh_adjusted.obj")

    centered = verts - centroid
    verts_2d = np.column_stack([centered @ lateral, centered @ longitudinal])
    footprint = mesh_footprint.build_footprint(verts_2d, faces)
    if adj.get("sym"):
        # the ship is known-symmetric but the reconstruction is lopsided
        # (occluded side under-built); union with the mirror across the
        # centreline so the fuller side fills in the starved one. The
        # centreline is NOT x=0 -- the centroid is dragged toward the fat
        # side -- so search the mirror axis for the most compact union.
        from shapely.affinity import scale as shp_scale, translate
        from shapely.ops import unary_union
        minx, miny, maxx, maxy = footprint.bounds
        best = None
        for xc in np.linspace(minx * 0.4 + maxx * 0.6,
                              minx * 0.6 + maxx * 0.4, 25):
            mirrored = shp_scale(footprint, xfact=-1, origin=(xc, 0))
            u = unary_union([footprint, mirrored]).buffer(0)
            if best is None or u.area < best.area:
                best = u
        footprint = best
    canon = profile.canonicalise(footprint, np.array([1.0, 0.0]))
    w, concave = profile.sample(canon)
    aspect_val = profile.aspect(w)

    up_proj = centered @ up
    (d / "footprint.json").write_text(json.dumps({
        "id": d.name, "polygon": mapping(footprint),
        "frame_ambiguous": frame_ambiguous, "depth_extent": float(np.ptp(up_proj)),
        "adjustment": adj,
    }, indent=2))
    (d / "profile.json").write_text(json.dumps({
        "schema": profile.SCHEMA_VERSION, "id": d.name,
        "stations": profile.STATIONS, "w": w.tolist(), "concave": concave.tolist(),
        "aspect": aspect_val, "orientation": "unknown",
        "frame_ambiguous": frame_ambiguous, "adjustment": adj,
    }, indent=2))
    draw_outline(polygon_rings(mapping(footprint)), d / "outline_hy3d.png",
                 frame="latlong")
    return {"aspect": aspect_val}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("adjustments")
    ap.add_argument("--dir", default="out-hy3d-full")
    args = ap.parse_args()

    adjustments = json.loads(Path(args.adjustments).read_text())
    root = HERE / args.dir
    for stem, adj in adjustments.items():
        d = root / stem
        if not (d / "mesh.obj").exists():
            print(f"{stem:34} SKIP (no mesh)")
            continue
        try:
            r = apply_one(d, adj)
            print(f"{stem:34} {adj}  -> aspect {r['aspect']:.2f}")
        except Exception as exc:
            print(f"{stem:34} FAILED  {type(exc).__name__}: {exc}")
    print("\nnow re-run make_triage_sheet.py to refresh the sheet's stats")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
