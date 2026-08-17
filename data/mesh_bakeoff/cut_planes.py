#!/usr/bin/env python3
"""Surgically remove backdrop-plane faces from a mesh.

Last resort for ships whose hero image makes Hunyuan3D invent a backdrop
sheet on EVERY seed (clean chroma key notwithstanding). Iteratively finds
the dominant coplanar face cluster meeting the detector's plane criteria,
deletes those faces, then drops the debris: disconnected components that
are tiny, or big-but-paper-thin (plane fragments the cut orphaned).

The result is no longer watertight -- fine for footprint extraction and
the point-cloud viewer, NOT for printing. The original mesh is kept as
mesh_orig.obj; print such ships from Meshy output instead.

    ~/hy3d-venv/bin/python cut_planes.py <sweep_dir> <stem> [...]
"""

import sys
from pathlib import Path

import numpy as np
import trimesh

AREA_FRAC = 0.08
SPAN_FRAC = 0.80
COS_TOL = 0.996
OFF_TOL = 0.01
MAX_CUTS = 4


def find_plane_faces(mesh: trimesh.Trimesh):
    """Face mask of the dominant qualifying coplanar cluster, or None."""
    n = mesh.face_normals
    ctr = mesh.triangles_center
    area = mesh.area_faces.copy()
    total = area.sum()
    dims = np.sort(mesh.extents)[::-1]

    for _ in range(6):
        seed_i = int(np.argmax(area))
        if area[seed_i] <= 0:
            break
        d = n[seed_i]
        aligned = np.abs(n @ d) > COS_TOL
        off = ctr @ d
        band = aligned & (np.abs(off - off[seed_i]) < OFF_TOL)
        frac = mesh.area_faces[band].sum() / total
        pts = ctr[band]
        u = np.cross(d, [0, 0, 1.0])
        if np.linalg.norm(u) < 1e-6:
            u = np.cross(d, [0, 1.0, 0])
        u /= np.linalg.norm(u)
        v = np.cross(d, u)
        span = max(np.ptp(pts @ u) / dims[0], np.ptp(pts @ v) / dims[1])
        if frac >= AREA_FRAC and span >= SPAN_FRAC:
            # widen the offset band slightly to catch the sheet's rim faces
            return aligned & (np.abs(off - off[seed_i]) < OFF_TOL * 3)
        area[aligned] = 0
    return None


def clean_debris(mesh: trimesh.Trimesh) -> trimesh.Trimesh:
    comps = mesh.split(only_watertight=False)
    if len(comps) <= 1:
        return mesh
    total = sum(len(c.faces) for c in comps)
    keep = []
    for c in comps:
        if len(c.faces) < 0.02 * total:
            continue  # crumbs
        ext = np.sort(c.extents)[::-1]
        if ext[2] < 0.02 * ext[0] and len(c.faces) < 0.25 * total:
            continue  # orphaned paper-thin sheet fragment
        keep.append(c)
    return trimesh.util.concatenate(keep) if keep else mesh


def main() -> int:
    sweep = Path(sys.argv[1])
    for stem in sys.argv[2:]:
        d = sweep / stem
        mesh = trimesh.load(d / "mesh.obj", force="mesh")
        orig_faces = len(mesh.faces)
        cut = 0
        for _ in range(MAX_CUTS):
            band = find_plane_faces(mesh)
            if band is None:
                break
            mesh.update_faces(~band)
            mesh.remove_unreferenced_vertices()
            cut += 1
        mesh = clean_debris(mesh)
        if cut == 0:
            print(f"{stem:28} no qualifying plane found, untouched")
            continue
        if not (d / "mesh_orig.obj").exists():
            (d / "mesh.obj").rename(d / "mesh_orig.obj")
        mesh.export(d / "mesh.obj")
        mesh.export(d / "mesh.glb")
        mesh.export(d / "mesh.stl")
        print(f"{stem:28} cut {cut} plane(s): {orig_faces} -> {len(mesh.faces)} faces, "
              f"watertight={mesh.is_watertight}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
