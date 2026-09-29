#!/usr/bin/env python3
"""Local-only hangar detail comparison: the same ships at several face counts,
side by side, captioned "<Name> · <N>k faces". Opens at ships/hangar.html?lod.

Copies the variants into kb/ships/hangar/lod/ (gitignored) from the cinema-hull
export (full 40k meshes and make_lod_variants.py's <id>__lod<N>k files); the 6k
level is the hangar's own model. Stdlib only:

    python3 make_hangar_lod.py [--ships a,b] [--levels 40000,16000,8000,6000,4000,2000] [--master [500000]]
"""
import argparse
import json
import shutil
from pathlib import Path

HERE = Path(__file__).resolve().parent
HANGAR = HERE.parent.parent / "kb" / "ships" / "hangar"
HULLS = Path.home() / "spacemolt-www" / "public" / "cinema-hulls"
DEFAULT_SHIPS = ["axiom", "close_enough", "hells_bells", "absolute_entropy", "opus_magna"]
DEFAULT_LEVELS = [40000, 16000, 8000, 6000, 4000, 2000]
FULL, HANGAR_FACES = 40000, 6000


def level_tag(faces):
    return f"{faces // 1000}k"


def build_lod_lineup(base, ships, levels, master=None):
    """With master (the dense bake's face count), each ship leads with its master and
    every level pairs the original ladder with make_master_lod.py's decimation of it."""
    by_id = {s["id"]: s for s in base["ships"]}
    rows = []
    for sid in ships:
        ship = by_id[sid]                                   # KeyError for unknown ships

        def row(rid, label, model):
            rows.append({**ship, "id": rid, "name": f"{ship['name']} · {label}", "model": model})

        if master:
            row(f"{sid}__m{level_tag(master)}", f"{level_tag(master)} master", f"lod/{sid}__m{level_tag(master)}.glb")
        for faces in levels:
            tag = level_tag(faces)
            row(f"{sid}__{tag}", f"{tag} orig" if master else f"{tag} faces",
                ship["model"] if faces == HANGAR_FACES else f"lod/{sid}__{tag}.glb")
            if master:
                row(f"{sid}__m{tag}", f"{tag} from master", f"lod/{sid}__m{tag}.glb")
    return {"version": 1, "ships": rows}


def source_glb(sid, faces):
    return HULLS / (f"{sid}.glb" if faces == FULL else f"{sid}__lod{level_tag(faces)}.glb")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--ships", default=",".join(DEFAULT_SHIPS))
    parser.add_argument("--levels", default=",".join(map(str, DEFAULT_LEVELS)))
    parser.add_argument("--master", type=int, nargs="?", const=500000, default=None,
                        help="interleave make_master_lod.py's dense-master ladder (bake face count, default 500000)")
    args = parser.parse_args()
    ships = [s for s in args.ships.split(",") if s]
    levels = [int(n) for n in args.levels.split(",") if n]
    lineup = build_lod_lineup(json.loads((HANGAR / "lineup.json").read_text()), ships, levels, args.master)
    out = HANGAR / "lod"
    out.mkdir(exist_ok=True)
    missing = []
    for sid in ships:
        for faces in levels:
            if faces == HANGAR_FACES:
                continue
            src = source_glb(sid, faces)
            if src.exists():
                shutil.copyfile(src, out / f"{sid}__{level_tag(faces)}.glb")
            else:
                missing.append(src.name)
    missing += [Path(r["model"]).name for r in lineup["ships"]
                if "__m" in r["id"] and not (HANGAR / r["model"]).exists()]
    (out / "lineup.json").write_text(json.dumps(lineup, separators=(",", ":")))
    print(f"{len(lineup['ships'])} variants -> {out}" + (f"; missing (run make_lod_variants.py / make_master_lod.py): {missing}" if missing else ""))


if __name__ == "__main__":
    main()
