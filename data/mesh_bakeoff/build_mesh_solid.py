#!/usr/bin/env python3
"""Assemble mesh_solid.html — the solid-shaded companion to the point-cloud
holo page, for judging how smooth/choppy the raw Hy3D meshes actually are.

Same five ships and the same fitted hero cameras as pointcloud_holo.html
(both are read from that page so the two stay in sync), same quantisation:
vertices centred on their mean and scaled to int16 max-abs, so the solid
view and the cloud view occupy identical screen space. Each vertex also
carries an RGB sampled from the hero image at the fitted camera (the
colorize_cloud.py projection re-applied to mesh vertices — no UV unwrap
or texture bake needed; the Hy3D meshes have no UVs anyway, and at their
vertex density per-vertex colour is effectively a texture). three.js (UMD
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
from PIL import Image
from scipy.spatial import cKDTree

import colorize_cloud

HERE = Path(__file__).resolve().parent
SWEEP = HERE / "out-hy3d-full"
HOLO = HERE / "pointcloud_holo.html"
THREE_URL = "https://unpkg.com/three@0.147.0/build/three.min.js"
VENDOR = HERE / "vendor" / "three-0.147.0.min.js"

# two per empire, then the two independents (user's pick, 2026-08-18)
SHIPS = [
    "outerrim_prayer", "outerrim_money_pit",
    "crimson_falchion", "crimson_shiv",
    "nebula_gas_tanker", "nebula_hegemon",
    "solarian_opus_magna", "solarian_fractional_distillery",
    "voidborn_membrane", "voidborn_superposition",
    "specter", "ironjaw",
]
CAMS_CACHE = HERE / "mesh_solid_cams.json"

# polygon-budget comparison: 40k is the sweep's FaceReducer default; 20k is
# a pymeshlab quadric decimation of it; 80k is a seeded re-run of the
# generator with --max-faces 80000 (same latents, denser reduction target)
LODS = {"20k": HERE / "out-hy3d-20k", "40k": SWEEP, "80k": HERE / "out-hy3d-80k"}


def holo_ships() -> list[tuple[str, dict]]:
    txt = HOLO.read_text()
    pairs = re.findall(
        r'"?([a-z_0-9]+)"?:\s*\{n:\d+,\s*cam:\{elev:(-?[\d.]+),azim:(-?[\d.]+)\}', txt)
    if not pairs:
        raise SystemExit("no ships found in pointcloud_holo.html")
    return [(stem, {"elev": float(e), "azim": float(a)}) for stem, e, a in pairs]


def cameras() -> dict[str, dict]:
    """Hero camera per ship: the holo page's fitted cams where available
    (keeps the two pages in sync), otherwise colorize_cloud.fit_camera on a
    fresh surface sample, cached in mesh_solid_cams.json."""
    cams = dict(holo_ships())
    cache = json.loads(CAMS_CACHE.read_text()) if CAMS_CACHE.exists() else {}
    changed = False
    for stem in SHIPS:
        if stem in cams:
            continue
        if stem in cache:
            cams[stem] = cache[stem]
            continue
        mesh = trimesh.load(SWEEP / stem / "mesh.obj", force="mesh")
        pts, _ = trimesh.sample.sample_surface(mesh, 20000)
        pts = np.asarray(pts) - np.asarray(pts).mean(axis=0)
        img = np.asarray(
            Image.open(SWEEP / stem / "keyed.png").convert("RGBA")).astype(float)
        score, elev, azim = colorize_cloud.fit_camera(pts, img[..., 3] / 255.0)
        print(f"  fit {stem}: elev {elev} azim {azim}  (IoU {score:.3f})")
        cams[stem] = cache[stem] = {"elev": float(elev), "azim": float(azim)}
        changed = True
    if changed:
        CAMS_CACHE.write_text(json.dumps(cache, indent=1) + "\n")
    return cams


VIS_RES = 512
RADIAL_FILL = False


def colorize(v: np.ndarray, stem: str, cam: dict) -> np.ndarray:
    """Per-vertex RGB from the hero image — colorize_cloud.py's projection
    verbatim, applied to mesh vertices instead of surface samples. The
    fitted camera is reused (not re-fit); occluded vertices borrow the
    colour of their nearest visible neighbour. No UVs, no texture bake:
    at Hy3D vertex density the vertex colours ARE the texture."""
    img = np.asarray(
        Image.open(SWEEP / stem / "keyed.png").convert("RGBA")).astype(float)
    alpha = img[..., 3] / 255.0
    e, a = np.radians(cam["elev"]), np.radians(cam["azim"])
    fwd = np.array([np.cos(e) * np.sin(a), np.sin(e), np.cos(e) * np.cos(a)])
    right = np.cross([0.0, 1.0, 0.0], fwd)
    n = np.linalg.norm(right)
    right = np.array([1.0, 0.0, 0.0]) if n < 1e-6 else right / n
    up = np.cross(fwd, right)

    xy = np.column_stack([v @ right, -(v @ up)])  # image y grows downward
    depth = v @ fwd
    ys, xs = np.nonzero(alpha > 0.5)
    ix0, ix1, iy0, iy1 = xs.min(), xs.max(), ys.min(), ys.max()
    lo, hi = xy.min(axis=0), xy.max(axis=0)
    span = hi - lo
    s = min((ix1 - ix0) / max(span[0], 1e-9), (iy1 - iy0) / max(span[1], 1e-9))
    px = (xy - (lo + hi) / 2) * s + [(ix0 + ix1) / 2, (iy0 + iy1) / 2]
    pxi = np.clip(px.astype(int), [0, 0], [img.shape[1] - 1, img.shape[0] - 1])

    gx = (px[:, 0] * VIS_RES / img.shape[1]).astype(int).clip(0, VIS_RES - 1)
    gy = (px[:, 1] * VIS_RES / img.shape[0]).astype(int).clip(0, VIS_RES - 1)
    cell = gy * VIS_RES + gx
    zbuf = np.full(VIS_RES * VIS_RES, -np.inf)
    np.maximum.at(zbuf, cell, depth)
    eps = 0.03 * (depth.max() - depth.min())
    visible = depth >= zbuf[cell] - eps
    visible &= alpha[pxi[:, 1], pxi[:, 0]] > 0.5

    rgb = np.zeros((len(v), 3))
    rgb[visible] = img[pxi[visible, 1], pxi[visible, 0], :3]
    if visible.any() and (~visible).any():
        radial = RADIAL_FILL == "all" or (
            RADIAL_FILL and any(s in stem for s in RADIAL_FILL.split(",")))
        if radial:
            # radially symmetric bodies (stations): fill occluded vertices
            # from the nearest painted vertex at the same (radius, height) —
            # spins the visible band's paint around the axis, so each ring
            # keeps its own true colour instead of smearing across the seam
            feat = np.column_stack([np.hypot(v[:, 0], v[:, 2]), v[:, 1]])
        else:
            feat = v
        _, nn = cKDTree(feat[visible]).query(feat[~visible])
        rgb[~visible] = rgb[visible][nn]
    print(f"    colour: visible {visible.mean() * 100:.0f}%, "
          f"borrowed {(~visible).mean() * 100:.0f}%")
    return rgb.clip(0, 255).astype(np.uint8)


def pack(stem: str, cam: dict, lod_dir: Path) -> tuple[str, dict]:
    mesh = trimesh.load(lod_dir / stem / "mesh.obj", force="mesh", process=False)
    v = np.asarray(mesh.vertices, dtype=np.float64)
    f = np.asarray(mesh.faces)
    v -= v.mean(axis=0)                      # same centring as colorize_cloud
    q = (v / np.abs(v).max() * 32767).astype(np.int16)
    idx16 = len(v) < 65536
    idx = f.astype(np.uint16 if idx16 else np.uint32)
    c8 = colorize(v, stem, cam)
    blob = q.tobytes() + idx.tobytes() + c8.tobytes()
    return base64.b64encode(blob).decode(), {
        "nv": len(v), "nf": len(f), "idx16": idx16}


def main() -> int:
    import argparse
    ap = argparse.ArgumentParser()
    ap.add_argument("--lods", default=",".join(LODS),
                    help="comma-separated LOD subset (e.g. '40k' for the "
                         "lean deployed build; default: all)")
    ap.add_argument("--out", default=str(HERE / "mesh_solid.html"))
    ap.add_argument("--sweep", default=None,
                    help="alternate sweep dir (e.g. out-stations); implies "
                         "--lods 40k and cameras fitted on demand")
    ap.add_argument("--ships", default=None,
                    help="comma-separated stems overriding the built-in roster")
    ap.add_argument("--radial-fill", nargs="?", const="all", default=None,
                    help="fill occluded vertex colours by (radius, height) "
                         "instead of 3D nearness — for radially symmetric "
                         "stations, spins the hero paint around the axis. "
                         "Bare flag: every ship; or a comma list of stem "
                         "substrings (e.g. 'saucer,ring,pagoda')")
    ap.add_argument("--lodmap", default=None,
                    help="explicit label=dir LOD mapping, e.g. "
                         "'40k=out-stations,250k=out-stations-hr250k,1m=out-stations-hr1m'; "
                         "overrides --lods/--sweep LOD selection (missing dirs skip)")
    args = ap.parse_args()
    global SWEEP, SHIPS, RADIAL_FILL
    RADIAL_FILL = args.radial_fill
    if args.sweep:
        SWEEP = HERE / args.sweep
        LODS.clear()
        LODS["40k"] = SWEEP
        args.lods = "40k"
    if args.ships:
        SHIPS = args.ships.split(",")
    if args.lodmap:
        lod_dirs = {}
        for pair in args.lodmap.split(","):
            label, d = pair.split("=", 1)
            lod_dirs[label] = HERE / d
        SWEEP = next(iter(lod_dirs.values()))  # keyed.png/cam-fit source
    else:
        lod_dirs = {k: LODS[k] for k in args.lods.split(",")}
    if not VENDOR.exists():
        VENDOR.parent.mkdir(exist_ok=True)
        print(f"downloading {THREE_URL}")
        VENDOR.write_bytes(urllib.request.urlopen(THREE_URL, timeout=60).read())
    three = VENDOR.read_text()

    cams = cameras()
    entries = []
    for stem in SHIPS:
        lods = []
        for lod, lod_dir in lod_dirs.items():
            if not (lod_dir / stem / "mesh.obj").exists():
                print(f"  {stem}: no {lod} mesh yet, skipping that LOD")
                continue
            b64, meta = pack(stem, cams[stem], lod_dir)
            lods.append(
                f'"{lod}":{{nv:{meta["nv"]},nf:{meta["nf"]},'
                f'idx16:{str(meta["idx16"]).lower()},b64:"{b64}"}}')
            print(f"  {stem:30} {lod:>4} {meta['nv']:>6} verts {meta['nf']:>6} faces")
        entries.append(
            f'{stem}: {{cam:{json.dumps(cams[stem])},lods:{{{",".join(lods)}}}}}')

    page = (HERE / "mesh_solid_template.html").read_text()
    page = page.replace("__THREE_JS__", three)
    page = page.replace("__MESH_DATA__", "const MESHES = {\n" + ",\n".join(entries) + "\n};")
    out = Path(args.out)
    out.write_text(page)
    print(f"{out} ({out.stat().st_size // 1024}KB)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
