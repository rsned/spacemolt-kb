#!/usr/bin/env python3
"""Local-only hangar detail comparison: the same ships at several face counts,
side by side, captioned "<Name> · <N>k faces". Opens at ships/hangar.html?lod.

Copies the variants into kb/ships/hangar/lod/ (gitignored) from the cinema-hull
export (full 40k meshes and make_lod_variants.py's <id>__lod<N>k files); the 6k
level is the hangar's own model. Stdlib only:

    python3 make_hangar_lod.py [--ships a,b] [--levels 40000,16000,8000,6000,4000,2000]
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


def build_lod_lineup(base, ships, levels):
    by_id = {s["id"]: s for s in base["ships"]}
    rows = []
    for sid in ships:
        ship = by_id[sid]                                   # KeyError for unknown ships
        for faces in levels:
            tag = level_tag(faces)
            model = ship["model"] if faces == HANGAR_FACES else f"lod/{sid}__{tag}.glb"
            rows.append({**ship, "id": f"{sid}__{tag}", "name": f"{ship['name']} · {tag} faces", "model": model})
    return {"version": 1, "ships": rows}


def source_glb(sid, faces):
    return HULLS / (f"{sid}.glb" if faces == FULL else f"{sid}__lod{level_tag(faces)}.glb")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--ships", default=",".join(DEFAULT_SHIPS))
    parser.add_argument("--levels", default=",".join(map(str, DEFAULT_LEVELS)))
    args = parser.parse_args()
    ships = [s for s in args.ships.split(",") if s]
    levels = [int(n) for n in args.levels.split(",") if n]
    lineup = build_lod_lineup(json.loads((HANGAR / "lineup.json").read_text()), ships, levels)
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
    (out / "lineup.json").write_text(json.dumps(lineup, separators=(",", ":")))
    print(f"{len(lineup['ships'])} variants -> {out}" + (f"; missing (run make_lod_variants.py): {missing}" if missing else ""))


if __name__ == "__main__":
    main()
