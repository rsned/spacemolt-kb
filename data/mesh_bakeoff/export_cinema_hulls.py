#!/usr/bin/env python3
"""Export Hy3D hulls for the battle cinematic (SpaceMolt/www fork).

    ~/sf3d-venv/bin/python export_cinema_hulls.py [--only dirk,axiom] [--out DIR]

Writes <kb_id>.glb + <kb_id>.json sidecar + manifest.json. See
docs/superpowers/specs/2026-09-24-cinematic-hy3d-hulls-design.md.
"""
import argparse
import contextlib
import json
import sqlite3
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

import cinema_frame as cf   # noqa: E402
import hardpoints as hp     # noqa: E402

DEFAULT_OUT = Path.home() / "spacemolt-www" / "public" / "cinema-hulls"
DEFAULT_DB = Path.home() / "spacemolt" / "spacemolt-knowledge.db"


def build_sidecar(kb_id, verts, faces, weapon_slots):
    return {"version": 1, "id": kb_id, "source": "auto",
            "engines": hp.guess_engines(verts, faces),
            "mounts": hp.guess_mounts(verts, faces),
            "weaponSlots": int(weapon_slots)}


def merge_manifest(existing: dict, exported: dict, only: bool) -> dict:
    """Combine this run's exported entries into a manifest dict.

    With --only, entries are merged into the existing manifest (other ids
    are kept, so successive --only runs accumulate). Without --only this is
    a full export of every mapped id, so the manifest is rebuilt from
    scratch and ids that were not (re)exported this run are pruned.
    """
    ships = dict(existing.get("ships", {})) if only else {}
    ships.update(exported)
    return {"version": 1, "ships": dict(sorted(ships.items()))}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", type=Path, default=DEFAULT_OUT)
    ap.add_argument("--db", type=Path, default=DEFAULT_DB)
    ap.add_argument("--only", default="", help="comma-separated KB ship ids")
    args = ap.parse_args()

    if not args.db.exists():
        print(f"--db not found: {args.db}", file=sys.stderr)
        return 1

    mapping = json.loads((HERE / "ship_id_map.json").read_text())["mapping"]
    adjustments = json.loads((HERE / "adjustments-final.json").read_text())
    stem_of = {m["id"]: stem for stem, m in mapping.items()}
    only = bool(args.only)
    wanted = [s for s in args.only.split(",") if s] or sorted(stem_of)
    with contextlib.closing(sqlite3.connect(args.db)) as conn:
        slots = dict(conn.execute("SELECT id, weapon_slots FROM ships").fetchall())

    args.out.mkdir(parents=True, exist_ok=True)
    manifest_path = args.out / "manifest.json"
    existing_manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else {"version": 1, "ships": {}}

    failures = 0
    exported = {}
    for kb_id in wanted:
        stem = stem_of.get(kb_id)
        if not stem or not (cf.SWEEP / stem / "mesh.obj").exists():
            print(f"{kb_id:32} SKIP (no mesh)")
            continue
        try:
            adj = adjustments.get(stem, {})
            v, f, flipped = cf.load_stem(stem, adj)
            p, f2 = cf.orient(v, f, adj, flipped)
            (args.out / f"{kb_id}.glb").write_bytes(cf.glb_bytes(p, cf.vertex_normals(p, f2), f2))
            sidecar = build_sidecar(kb_id, p, f2, slots.get(kb_id) or 0)
            (args.out / f"{kb_id}.json").write_text(json.dumps(sidecar, indent=1))
            exported[kb_id] = {"glb": f"{kb_id}.glb", "sidecar": f"{kb_id}.json"}
            ext = np.ptp(p, axis=0)
            print(f"{kb_id:32} ok  beam={ext[2]:.3f} height={ext[1]:.3f} "
                  f"engines={len(sidecar['engines'])} mounts={len(sidecar['mounts'])} slots={sidecar['weaponSlots']}")
        except Exception as exc:   # one bad hull must not stop the fleet
            failures += 1
            print(f"{kb_id:32} FAIL {exc!r}")
    manifest = merge_manifest(existing_manifest, exported, only)
    manifest_path.write_text(json.dumps(manifest, indent=1))
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
