#!/usr/bin/env python3
"""Decimated level-of-detail variants of exported cinema hulls, for choosing the
polygon budget of the compressed production models.

Reads <out>/<id>.glb (already in the cinema frame), writes <id>__lod<N>.glb at
each target face count plus a copied sidecar, and adds them to the manifest.
Quadric edge-collapse via pymeshlab, so run it with the venv that has it:

    ~/hy3d-venv/bin/python make_lod_variants.py [--ships a,b] [--levels 16000,8000,4000,2000]
"""
import argparse
import json
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import cinema_frame as cf  # noqa: E402

DEFAULT_OUT = Path.home() / "spacemolt-www" / "public" / "cinema-hulls"
DEFAULT_SHIPS = ["axiom", "close_enough", "hells_bells", "absolute_entropy", "opus_magna"]
DEFAULT_LEVELS = [16000, 8000, 4000, 2000]


def level_name(faces):
    return f"{faces // 1000}k" if faces % 1000 == 0 else str(faces)


def decimate(verts, faces, target):
    import pymeshlab
    ms = pymeshlab.MeshSet()
    ms.add_mesh(pymeshlab.Mesh(vertex_matrix=np.asarray(verts, float), face_matrix=np.asarray(faces, np.int32)))
    ms.meshing_decimation_quadric_edge_collapse(targetfacenum=int(target), preservenormal=True,
                                                preservetopology=True, qualitythr=.5)
    mesh = ms.current_mesh()
    return mesh.vertex_matrix().astype(float), mesh.face_matrix().astype(np.int64)


def make_variants(out, ships, levels):
    manifest_path = out / "manifest.json"
    manifest = json.loads(manifest_path.read_text())
    written = []
    for ship in ships:
        verts, faces = cf.read_glb((out / f"{ship}.glb").read_bytes())
        sidecar = json.loads((out / f"{ship}.json").read_text())
        for target in levels:
            vid = f"{ship}__lod{level_name(target)}"
            dv, df = decimate(verts, faces, target)
            (out / f"{vid}.glb").write_bytes(cf.glb_bytes(dv.astype(np.float32), cf.vertex_normals(dv, df), df.astype(np.uint32)))
            (out / f"{vid}.json").write_text(json.dumps({**sidecar, "id": vid}, indent=1))
            manifest["ships"][vid] = {"glb": f"{vid}.glb", "sidecar": f"{vid}.json"}
            written.append(vid)
            print(f"{vid:36} {len(df):6d} faces {len(dv):6d} verts {(out / f'{vid}.glb').stat().st_size // 1024:5d} KB")
    manifest["ships"] = dict(sorted(manifest["ships"].items()))
    manifest_path.write_text(json.dumps(manifest, indent=1))
    return written


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    parser.add_argument("--ships", default=",".join(DEFAULT_SHIPS))
    parser.add_argument("--levels", default=",".join(map(str, DEFAULT_LEVELS)))
    args = parser.parse_args()
    make_variants(args.out, [s for s in args.ships.split(",") if s], [int(n) for n in args.levels.split(",") if n])


if __name__ == "__main__":
    main()
