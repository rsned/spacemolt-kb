#!/usr/bin/env python3
"""Hy3D mesh -> the battle cinematic's hull frame.

Cinema frame (SpaceMolt/www src/lib/cinema/ships.ts): +X bow, +Y dorsal,
+Z starboard (right-handed), bbox-centred, bow-to-stern length 1.

Uses the same footprint frame, stretch and bow verdict as the shipped
SVG footprints / side views (make_views.make_one), so every exported hull
points the way the KB drawings do.
"""
import json
import struct
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

SWEEP = HERE / "out-hy3d-full"


def _signed_volume(v: np.ndarray, f: np.ndarray) -> float:
    return float(np.einsum("ij,ij->i", v[f[:, 0]], np.cross(v[f[:, 1]], v[f[:, 2]])).sum() / 6)


def orient(verts: np.ndarray, faces: np.ndarray, adj: dict, bow_flipped: bool):
    """Pure transform into the cinema frame. Returns (float32 verts, uint32 faces)."""
    from make_views import frame_for   # heavy (shapely): only the orient path needs it
    verts = np.asarray(verts, dtype=float)
    faces = np.asarray(faces, dtype=np.int64)
    centroid = verts.mean(axis=0)
    _lateral, longitudinal, up = frame_for(verts, adj)
    c = verts - centroid
    stretch = float(adj.get("stretch", 1.0))
    if abs(stretch - 1.0) > 1e-3:
        c = c + (stretch - 1.0) * np.outer(c @ longitudinal, longitudinal)
    xdir = -longitudinal if bow_flipped else longitudinal
    ydir = -up if adj.get("vflip") else up
    zdir = np.cross(xdir, ydir)          # proper rotation: keeps true chirality
    if adj.get("mirror"):
        zdir = -zdir                     # deliberate port/starboard swap
    p = np.column_stack([c @ xdir, c @ ydir, c @ zdir])
    lo, hi = p.min(axis=0), p.max(axis=0)
    p = (p - (lo + hi) / 2) / (hi[0] - lo[0])
    if _signed_volume(p, faces) < 0:     # mirror (or an inside-out source) flips winding
        faces = faces[:, ::-1]
    return p.astype(np.float32), np.ascontiguousarray(faces, dtype=np.uint32)


def vertex_normals(verts: np.ndarray, faces: np.ndarray) -> np.ndarray:
    """Area-weighted smooth vertex normals (numpy only: no trimesh .ptp paths)."""
    v = np.asarray(verts, dtype=float)
    f = np.asarray(faces, dtype=np.int64)
    fn = np.cross(v[f[:, 1]] - v[f[:, 0]], v[f[:, 2]] - v[f[:, 0]])
    n = np.zeros_like(v)
    for k in range(3):
        np.add.at(n, f[:, k], fn)
    n /= np.maximum(np.linalg.norm(n, axis=1, keepdims=True), 1e-12)
    return n.astype(np.float32)


def glb_bytes(verts: np.ndarray, normals: np.ndarray, faces: np.ndarray) -> bytes:
    """Minimal glTF 2.0 binary: one mesh, POSITION + NORMAL + uint32 indices."""
    v = np.ascontiguousarray(verts, dtype="<f4")
    n = np.ascontiguousarray(normals, dtype="<f4")
    idx = np.ascontiguousarray(faces, dtype="<u4").ravel()
    blob = v.tobytes() + n.tobytes() + idx.tobytes()
    blob += b"\0" * (-len(blob) % 4)
    lv, ln = v.nbytes, n.nbytes
    gltf = {
        "asset": {"version": "2.0", "generator": "kb export_cinema_hulls"},
        "scene": 0, "scenes": [{"nodes": [0]}], "nodes": [{"mesh": 0, "name": "hull"}],
        "meshes": [{"primitives": [{"attributes": {"POSITION": 0, "NORMAL": 1}, "indices": 2}]}],
        "buffers": [{"byteLength": len(blob)}],
        "bufferViews": [
            {"buffer": 0, "byteOffset": 0, "byteLength": lv, "target": 34962},
            {"buffer": 0, "byteOffset": lv, "byteLength": ln, "target": 34962},
            {"buffer": 0, "byteOffset": lv + ln, "byteLength": idx.nbytes, "target": 34963},
        ],
        "accessors": [
            {"bufferView": 0, "componentType": 5126, "count": len(v), "type": "VEC3",
             "min": v.min(axis=0).tolist(), "max": v.max(axis=0).tolist()},
            {"bufferView": 1, "componentType": 5126, "count": len(n), "type": "VEC3"},
            {"bufferView": 2, "componentType": 5125, "count": int(idx.size), "type": "SCALAR"},
        ],
    }
    js = json.dumps(gltf, separators=(",", ":")).encode()
    js += b" " * (-len(js) % 4)
    total = 12 + 8 + len(js) + 8 + len(blob)
    return (struct.pack("<III", 0x46546C67, 2, total)
            + struct.pack("<II", len(js), 0x4E4F534A) + js
            + struct.pack("<II", len(blob), 0x004E4942) + blob)


def read_glb(data: bytes):
    """Inverse of glb_bytes (our minimal layout): -> (float verts N x 3, int faces M x 3)."""
    jlen, _ = struct.unpack_from("<II", data, 12)
    gltf = json.loads(data[20:20 + jlen])
    bin_start = 20 + jlen + 8
    views, accessors = gltf["bufferViews"], gltf["accessors"]
    prim = gltf["meshes"][0]["primitives"][0]

    def accessor(index, dtype, width):
        acc = accessors[index]
        view = views[acc["bufferView"]]
        start = bin_start + view.get("byteOffset", 0)
        count = acc["count"] * width
        return np.frombuffer(data, dtype=dtype, count=count, offset=start).reshape(-1, width) if width > 1 \
            else np.frombuffer(data, dtype=dtype, count=count, offset=start)
    verts = accessor(prim["attributes"]["POSITION"], "<f4", 3).astype(float)
    index_dtype = "<u4" if accessors[prim["indices"]]["componentType"] == 5125 else "<u2"
    faces = accessor(prim["indices"], index_dtype, 1).astype(np.int64).reshape(-1, 3)
    return verts, faces


def load_stem(stem: str, adj: dict):
    """Raw Hy3D mesh (solo applied) + the committed bow-right verdict."""
    import trimesh
    import apply_adjustments as aa
    from make_svg_footprints import bow_flip, rings_of
    d = SWEEP / stem
    mesh = trimesh.load(d / "mesh.obj", force="mesh", process=False)
    if adj.get("solo"):
        mesh = aa.solo_hull(mesh)
    fp = json.loads((d / "footprint.json").read_text())
    flipped = bow_flip(rings_of(fp["polygon"]), bool(adj.get("flip")))
    return np.asarray(mesh.vertices, dtype=float), np.asarray(mesh.faces, dtype=np.int64), flipped
