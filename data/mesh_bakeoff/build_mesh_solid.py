#!/usr/bin/env python3
"""Assemble mesh_solid.html — the solid-shaded companion to the point-cloud
holo page, for judging how smooth/choppy the raw Hy3D meshes actually are.

Same five ships and the same fitted hero cameras as pointcloud_holo.html
(both are read from that page so the two stay in sync), same quantisation:
vertices centred on their mean and scaled to int16 max-abs, so the solid
view and the cloud view occupy identical screen space. three.js (UMD
r147, the last single-file build) is downloaded once into vendor/ and
inlined, keeping the page fully self-contained like the holo one.

    ~/sf3d-venv/bin/python build_mesh_solid.py
"""

import base64
import json
import re
import urllib.request
from pathlib import Path

import numpy as np
import trimesh

HERE = Path(__file__).resolve().parent
SWEEP = HERE / "out-hy3d-full"
HOLO = HERE / "pointcloud_holo.html"
THREE_URL = "https://unpkg.com/three@0.147.0/build/three.min.js"
VENDOR = HERE / "vendor" / "three-0.147.0.min.js"


def holo_ships() -> list[tuple[str, dict]]:
    txt = HOLO.read_text()
    pairs = re.findall(
        r'"?([a-z_0-9]+)"?:\s*\{n:\d+,\s*cam:\{elev:(-?[\d.]+),azim:(-?[\d.]+)\}', txt)
    if not pairs:
        raise SystemExit("no ships found in pointcloud_holo.html")
    return [(stem, {"elev": float(e), "azim": float(a)}) for stem, e, a in pairs]


def pack(stem: str) -> tuple[str, dict]:
    mesh = trimesh.load(SWEEP / stem / "mesh.obj", force="mesh", process=False)
    v = np.asarray(mesh.vertices, dtype=np.float64)
    f = np.asarray(mesh.faces)
    v -= v.mean(axis=0)                      # same centring as colorize_cloud
    q = (v / np.abs(v).max() * 32767).astype(np.int16)
    idx16 = len(v) < 65536
    idx = f.astype(np.uint16 if idx16 else np.uint32)
    blob = q.tobytes() + idx.tobytes()
    return base64.b64encode(blob).decode(), {
        "nv": len(v), "nf": len(f), "idx16": idx16}


def main() -> int:
    if not VENDOR.exists():
        VENDOR.parent.mkdir(exist_ok=True)
        print(f"downloading {THREE_URL}")
        VENDOR.write_bytes(urllib.request.urlopen(THREE_URL, timeout=60).read())
    three = VENDOR.read_text()

    entries = []
    for stem, cam in holo_ships():
        b64, meta = pack(stem)
        entries.append(
            f'{stem}: {{nv:{meta["nv"]},nf:{meta["nf"]},'
            f'idx16:{str(meta["idx16"]).lower()},cam:{json.dumps(cam)},'
            f'b64:"{b64}"}}')
        print(f"  {stem:26} {meta['nv']:>6} verts {meta['nf']:>6} faces")

    page = (HERE / "mesh_solid_template.html").read_text()
    page = page.replace("__THREE_JS__", three)
    page = page.replace("__MESH_DATA__", "const MESHES = {\n" + ",\n".join(entries) + "\n};")
    out = HERE / "mesh_solid.html"
    out.write_text(page)
    print(f"{out} ({out.stat().st_size // 1024}KB)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
