#!/usr/bin/env python3
"""Fetch-on-demand viewer assets: one packed .bin per station + manifest.

The prototype for the KB-scale viewer architecture: instead of base64-ing
every mesh into one page (30MB+ and growing), stations_viewer.html loads a
small manifest and fetches each model's binary only when selected.

Per model: <stem>.bin = int16 xyz ×3nv | (uint16|uint32) index ×3nf |
uint8 rgb ×3nv — the same layout the embedded pages use, minus base64.
manifest.json maps stem -> {nv, nf, idx16, cam} and is rewritten after
every model, so the viewer works mid-build. Cameras come from the shared
mesh_solid_cams.json cache (fitted on demand via colorize_cloud); hero
colours use build_mesh_solid.colorize with radial fill for the radially
symmetric archetypes.

    ~/sf3d-venv/bin/python build_station_web.py   # all baked stations
"""

import json
import sys
from pathlib import Path

import numpy as np
import trimesh
from PIL import Image

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import build_mesh_solid as bms  # noqa: E402
import colorize_cloud  # noqa: E402

SWEEP = HERE / "out-stations"
OUT = HERE / "out-stations-web"
CAMS = HERE / "mesh_solid_cams.json"

bms.SWEEP = SWEEP
bms.RADIAL_FILL = "saucer,ring,wheel,pagoda,spindle,pylon_toroid,drum"


def fit_cam(stem: str) -> dict:
    mesh = trimesh.load(SWEEP / stem / "mesh.obj", force="mesh")
    pts, _ = trimesh.sample.sample_surface(mesh, 20000)
    pts = np.asarray(pts) - np.asarray(pts).mean(axis=0)
    img = np.asarray(
        Image.open(SWEEP / stem / "keyed.png").convert("RGBA")).astype(float)
    score, elev, azim = colorize_cloud.fit_camera(pts, img[..., 3] / 255.0)
    print(f"  fit {stem}: elev {elev} azim {azim}  (IoU {score:.3f})")
    return {"elev": float(elev), "azim": float(azim)}


def main() -> int:
    OUT.mkdir(exist_ok=True)
    cams = json.loads(CAMS.read_text()) if CAMS.exists() else {}
    manifest_path = OUT / "manifest.json"
    manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else {}
    stems = sorted(d.name for d in SWEEP.iterdir() if (d / "mesh.obj").exists())
    for i, stem in enumerate(stems):
        if stem in manifest and (OUT / f"{stem}.bin").exists():
            continue
        if stem not in cams:
            cams[stem] = fit_cam(stem)
            CAMS.write_text(json.dumps(cams, indent=1) + "\n")
        mesh = trimesh.load(SWEEP / stem / "mesh.obj", force="mesh", process=False)
        v = np.asarray(mesh.vertices, dtype=np.float64)
        f = np.asarray(mesh.faces)
        v -= v.mean(axis=0)
        q = (v / np.abs(v).max() * 32767).astype(np.int16)
        idx16 = len(v) < 65536
        idx = f.astype(np.uint16 if idx16 else np.uint32)
        c8 = bms.colorize(v, stem, cams[stem])
        (OUT / f"{stem}.bin").write_bytes(q.tobytes() + idx.tobytes() + c8.tobytes())
        manifest[stem] = {"nv": len(v), "nf": len(f), "idx16": idx16,
                          "cam": cams[stem]}
        manifest_path.write_text(json.dumps(manifest) + "\n")
        print(f"[{i + 1}/{len(stems)}] {stem}", flush=True)
    print(f"{len(manifest)} models -> {OUT}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
