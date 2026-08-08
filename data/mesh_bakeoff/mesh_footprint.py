#!/usr/bin/env python3
"""Extract a top-down footprint + 96-station profile from an image-to-3D mesh.

Bake-off spike only -- see bakeoff-brief.md. Reuses the existing footprint
pipeline's stage-7 sampler (tools.footprint.profile.canonicalise + .sample)
so the comparison against the pipeline's own recovered profiles is apples to
apples; everything upstream of that (mesh load, frame discovery, footprint
polygon) is new code written for this spike.

    ~/sf3d-venv/bin/python mesh_footprint.py <mesh_path> <ship_id> <out_dir>
"""

import json
import sys
import time
from pathlib import Path

import numpy as np
import trimesh
from scipy.spatial import cKDTree
from shapely.geometry import Polygon
from shapely.ops import unary_union

REPO_ROOT = Path("/home/robert/spacemolt/kb")
sys.path.insert(0, str(REPO_ROOT))
from tools.footprint import profile  # noqa: E402

CHAMFER_SUBSAMPLE = 5000
AMBIGUOUS_THRESHOLD = 0.10  # candidates within 10% of each other -> ambiguous


def pca_frame(verts: np.ndarray):
    """Return (centroid, axes) where axes columns are PCA directions, sorted
    by extent (max-min projection along that direction) descending."""
    centroid = verts.mean(axis=0)
    centered = verts - centroid
    cov = np.cov(centered.T)
    eigvals, eigvecs = np.linalg.eigh(cov)  # ascending eigenvalue order
    # Re-rank by actual extent along each axis, not eigenvalue -- the brief
    # asks for "largest-extent axis", which for a roughly-uniform-density
    # point cloud tracks eigenvalue closely but isn't identical (e.g. long
    # thin spikes vs a bulky hull can invert the ranking).
    extents = np.array([
        centered @ eigvecs[:, i] for i in range(3)
    ])
    extents = extents.max(axis=1) - extents.min(axis=1)
    order = np.argsort(extents)[::-1]
    return centroid, eigvecs[:, order], extents[order]


def chamfer(a: np.ndarray, b: np.ndarray) -> float:
    tree_a = cKDTree(a)
    tree_b = cKDTree(b)
    d_ab, _ = tree_b.query(a)
    d_ba, _ = tree_a.query(b)
    return float((d_ab.mean() + d_ba.mean()) / 2.0)


def reflect(points: np.ndarray, centroid: np.ndarray, normal: np.ndarray) -> np.ndarray:
    n = normal / np.linalg.norm(normal)
    v = points - centroid
    d = v @ n
    return points - 2.0 * np.outer(d, n)


def find_lateral_axis(verts: np.ndarray, centroid: np.ndarray, longitudinal: np.ndarray,
                       candidates: list[np.ndarray], rng: np.random.Generator):
    """Pick LATERAL from `candidates` (the two non-longitudinal PCA axes) by
    which reflection plane (through centroid, normal = candidate axis) best
    preserves the shape -- ships are bilaterally symmetric about lateral, not
    about up. Returns (lateral_axis, up_axis, chamfers, frame_ambiguous)."""
    n = verts.shape[0]
    idx = rng.choice(n, size=min(CHAMFER_SUBSAMPLE, n), replace=False)
    sub = verts[idx]

    chamfers = []
    for axis in candidates:
        reflected = reflect(sub, centroid, axis)
        chamfers.append(chamfer(sub, reflected))

    lo, hi = min(chamfers), max(chamfers)
    frame_ambiguous = bool(hi > 0 and (hi - lo) / hi < AMBIGUOUS_THRESHOLD)
    lateral_i = int(np.argmin(chamfers))
    up_i = 1 - lateral_i
    return candidates[lateral_i], candidates[up_i], chamfers, frame_ambiguous


def build_footprint(verts_2d: np.ndarray, faces: np.ndarray):
    """Union of the mesh's triangles projected onto the footprint plane."""
    tris = []
    for f in faces:
        pts = verts_2d[f]
        poly = Polygon(pts)
        if poly.is_empty or poly.area == 0:
            continue
        if not poly.is_valid:
            poly = poly.buffer(0)
            if poly.is_empty:
                continue
        tris.append(poly)
    footprint = unary_union(tris).buffer(0)
    return footprint


def run(mesh_path: str, ship_id: str, out_dir: Path, seed: int = 0) -> dict:
    t0 = time.time()
    out_dir.mkdir(parents=True, exist_ok=True)
    rng = np.random.default_rng(seed)

    mesh = trimesh.load(mesh_path, force="mesh", process=False)
    verts = np.asarray(mesh.vertices, dtype=float)
    faces = np.asarray(mesh.faces, dtype=int)

    centroid, axes, extents = pca_frame(verts)
    longitudinal = axes[:, 0]
    candidates = [axes[:, 1], axes[:, 2]]

    lateral, up, chamfers, frame_ambiguous = find_lateral_axis(
        verts, centroid, longitudinal, candidates, rng)

    # Project onto (lateral, longitudinal) -- footprint plane, matching
    # profile.canonicalise's expectation of a 2D polygon with an X-ish
    # longitudinal axis and a signed lateral Y.
    centered = verts - centroid
    x = centered @ lateral
    y = centered @ longitudinal
    verts_2d = np.column_stack([x, y])

    footprint = build_footprint(verts_2d, faces)
    up_proj = centered @ up
    depth_extent = float(up_proj.max() - up_proj.min())

    # profile.canonicalise rotates using sym_normal_xy = the symmetry
    # plane's normal *in this (lateral, longitudinal) 2D frame*. Since we
    # already projected onto (lateral, longitudinal) with lateral as the
    # x-axis, the symmetry-plane normal in that 2D frame is simply +X.
    sym_normal_xy = np.array([1.0, 0.0])

    canon = profile.canonicalise(footprint, sym_normal_xy)
    w, concave = profile.sample(canon)
    aspect_val = profile.aspect(w)

    from shapely.geometry import mapping
    footprint_data = {
        "id": ship_id,
        "polygon": mapping(footprint),
        "frame_ambiguous": frame_ambiguous,
        "chamfer_lateral": chamfers[int(np.argmin(chamfers))],
        "chamfer_up_candidate": chamfers[int(np.argmax(chamfers))],
        "chamfer_ratio": (min(chamfers) / max(chamfers)) if max(chamfers) > 0 else None,
        "depth_extent": depth_extent,
    }
    (out_dir / "footprint.json").write_text(json.dumps(footprint_data, indent=2))

    profile_data = {
        "schema": profile.SCHEMA_VERSION,
        "id": ship_id,
        "stations": profile.STATIONS,
        "w": w.tolist(),
        "concave": concave.tolist(),
        "aspect": aspect_val,
        "orientation": "unknown",
        "frame_ambiguous": frame_ambiguous,
    }
    (out_dir / "profile.json").write_text(json.dumps(profile_data, indent=2))

    elapsed = time.time() - t0
    return {
        "id": ship_id,
        "footprint_json": str(out_dir / "footprint.json"),
        "profile_json": str(out_dir / "profile.json"),
        "frame_ambiguous": frame_ambiguous,
        "aspect": aspect_val,
        "footprint_extract_seconds": elapsed,
        "n_verts": int(verts.shape[0]),
        "n_faces": int(faces.shape[0]),
    }


if __name__ == "__main__":
    mesh_path, ship_id, out_dir = sys.argv[1], sys.argv[2], Path(sys.argv[3])
    result = run(mesh_path, ship_id, out_dir)
    print(json.dumps(result, indent=2))
