#!/usr/bin/env python3
"""Detect backdrop-plane artifacts in sweep meshes.

Hunyuan3D sometimes hallucinates a large flat sheet (a backdrop or ground
plane) through the ship; marching cubes fuses it to the hull into one
watertight component, so FloaterRemover cannot help. Signature: a large
batch of faces that are (a) mutually coplanar within tight tolerance and
(b) span most of the bounding box in-plane. Real hull panels are coplanar
too, but never at backdrop scale -- the area and span thresholds separate
them (validated on crimson_flail / crimson_frostbite vs clean ships).

    ~/hy3d-venv/bin/python detect_planes.py out-hy3d-full          # sweep, report
    ~/hy3d-venv/bin/python detect_planes.py out-hy3d-full stem...  # verbose few
"""

import json
import sys
from pathlib import Path

import numpy as np
import trimesh

# Thresholds set against the full-sweep score distribution: clean ships top
# out ~4% coplanar area; backdrop planes and cyclorama frames (an L of floor
# and wall strips -- see outerrim_plausible_deniability, 10.4% area but only
# 0.71 span on one axis) sit >=8%. A frame strip is long in ONE in-plane
# axis, so only the longer span is required.
AREA_FRAC = 0.08   # coplanar cluster must hold >=8% of total surface area
SPAN_FRAC = 0.80   # ... and span >=80% of the bbox in its longer in-plane axis
COS_TOL = 0.996    # ~5 degrees normal alignment
OFF_TOL = 0.01     # plane-offset band, in mesh units (bbox ~2)


def plane_score(mesh: trimesh.Trimesh) -> dict:
    n = mesh.face_normals
    ctr = mesh.triangles_center
    area = mesh.area_faces
    total = area.sum()
    bbox = mesh.extents

    # candidate plane orientations: the area-weighted dominant normals.
    # Bucket normals (folding opposite directions together, since a thin
    # sheet contributes both sides) and test the biggest buckets.
    folded = n * np.sign(n[:, np.argmax(np.abs(n).mean(axis=0))])[:, None]
    best = {"area_frac": 0.0, "span_a": 0.0, "span_b": 0.0}
    for _ in range(6):
        seed_i = np.argmax(area)  # heaviest untested face
        d = folded[seed_i]
        aligned = np.abs(n @ d) > COS_TOL
        off = ctr @ d
        # densest offset band among aligned faces
        ref = off[seed_i]
        band = aligned & (np.abs(off - ref) < OFF_TOL)
        frac = area[band].sum() / total
        if frac > best["area_frac"]:
            pts = ctr[band]
            # in-plane axes
            u = np.cross(d, [0, 0, 1.0])
            if np.linalg.norm(u) < 1e-6:
                u = np.cross(d, [0, 1.0, 0])
            u /= np.linalg.norm(u)
            v = np.cross(d, u)
            su = np.ptp(pts @ u)
            sv = np.ptp(pts @ v)
            # compare spans against the two largest bbox dims
            dims = np.sort(bbox)[::-1]
            best = {"area_frac": float(frac),
                    "span_a": float(su / dims[0]), "span_b": float(sv / dims[1])}
        area = area.copy()
        area[aligned] = 0  # exhaust this orientation, try next-dominant
        if area.max() <= 0:
            break

    best["is_plane"] = (best["area_frac"] >= AREA_FRAC
                        and max(best["span_a"], best["span_b"]) >= SPAN_FRAC)
    return best


def main() -> int:
    root = Path(sys.argv[1])
    stems = sys.argv[2:] or sorted(d.name for d in root.iterdir()
                                   if (d / "mesh.obj").exists())
    verbose = bool(sys.argv[2:])
    hits = []
    for i, stem in enumerate(stems):
        mesh = trimesh.load(root / stem / "mesh.obj", force="mesh")
        s = plane_score(mesh)
        if verbose or s["is_plane"]:
            print(f"{stem:34} area {s['area_frac']*100:5.1f}%  "
                  f"span {s['span_a']:.2f}x{s['span_b']:.2f}  "
                  f"{'PLANE' if s['is_plane'] else 'ok'}")
        if s["is_plane"]:
            hits.append(stem)
        if not verbose and (i + 1) % 50 == 0:
            print(f"  ... {i + 1}/{len(stems)}")
    (root / "plane_hits.json").write_text(json.dumps(hits, indent=1))
    print(f"\n{len(hits)}/{len(stems)} flagged -> {root / 'plane_hits.json'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
