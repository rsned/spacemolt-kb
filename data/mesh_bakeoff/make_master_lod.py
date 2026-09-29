#!/usr/bin/env python3
"""Dense-master LOD ladder for the hangar detail comparison.

Takes the run_hy3d.py master bakes (--octree 448 --max-faces 500000 --mc-algo dmc,
in out-hy3d-master/<stem>/mesh.obj), puts each into the cinema frame exactly as
export_cinema_hulls.py does for the 40k sweep (same adjustments, same bow-right
verdict: the seeded latent is shared, only the extraction grid differs), then
quadric-decimates straight from the master to each level. Writes
kb/ships/hangar/lod/<id>__m<N>k.glb (gitignored) plus the master itself as
<id>__m<faces>k.glb; make_hangar_lod.py --master lays them out beside the
original ladder. Two passes, because orienting needs shapely (sf3d-venv) and
decimating needs pymeshlab (hy3d-venv); they hand off through the master GLB:

    ~/sf3d-venv/bin/python make_master_lod.py --orient [--ships a,b]
    ~/hy3d-venv/bin/python make_master_lod.py [--ships a,b] [--levels 40000,...]
"""
import argparse
import json
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import cinema_frame as cf  # noqa: E402
from make_hangar_lod import DEFAULT_LEVELS, DEFAULT_SHIPS, HANGAR, level_tag  # noqa: E402

MASTER_DIR = HERE / "out-hy3d-master"
MASTER_FACES = 500000


def master_glb(sid, faces):
    return f"{sid}__m{level_tag(faces)}.glb"


def load_master(stem, adj):
    """Master mesh (solo applied) + the sweep mesh's committed bow-right verdict."""
    import trimesh
    import apply_adjustments as aa
    from make_svg_footprints import bow_flip, rings_of
    mesh = trimesh.load(MASTER_DIR / stem / "mesh.obj", force="mesh", process=False)
    if adj.get("solo"):
        mesh = aa.solo_hull(mesh)
    fp = json.loads((cf.SWEEP / stem / "footprint.json").read_text())
    flipped = bow_flip(rings_of(fp["polygon"]), bool(adj.get("flip")))
    return np.asarray(mesh.vertices, float), np.asarray(mesh.faces, np.int64), flipped


def write(path, verts, faces):
    verts = np.asarray(verts, np.float32)
    path.write_bytes(cf.glb_bytes(verts, cf.vertex_normals(verts, faces), np.asarray(faces, np.uint32)))
    return path.stat().st_size


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--ships", default=",".join(DEFAULT_SHIPS))
    parser.add_argument("--levels", default=",".join(map(str, DEFAULT_LEVELS)))
    parser.add_argument("--orient", action="store_true", help="pass 1: orient the raw masters (sf3d-venv)")
    parser.add_argument("--master-faces", type=int, default=MASTER_FACES, help="the bake's --max-faces; names the master file")
    args = parser.parse_args()
    mapping = json.loads((HERE / "ship_id_map.json").read_text())["mapping"]
    adjustments = json.loads((HERE / "adjustments-final.json").read_text())
    stem_of = {m["id"]: stem for stem, m in mapping.items()}
    out = HANGAR / "lod"
    out.mkdir(exist_ok=True)
    for sid in [s for s in args.ships.split(",") if s]:
        master = out / master_glb(sid, args.master_faces)
        if args.orient:
            stem = stem_of[sid]
            adj = adjustments.get(stem, {})
            v, f, flipped = load_master(stem, adj)
            p, f2 = cf.orient(v, f, adj, flipped)
            size = write(master, p, f2)
            print(f"{sid:18} master {len(f2):7d} faces {size // 1024:6d} KB")
            continue
        from make_lod_variants import decimate
        p, f2 = cf.read_glb(master.read_bytes())
        for target in (int(n) for n in args.levels.split(",") if n):
            dv, df = decimate(p, f2, target)
            if cf._signed_volume(dv, df) < 0:
                df = df[:, ::-1]
            size = write(out / master_glb(sid, target), dv, df)
            print(f"{sid:18} {level_tag(target):>6} {len(df):7d} faces {size // 1024:6d} KB")


if __name__ == "__main__":
    main()
