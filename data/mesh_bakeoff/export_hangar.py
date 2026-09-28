#!/usr/bin/env python3
"""Export the KB ship hangar: kb/ships/hangar/lineup.json + decimated models.

    ~/hy3d-venv/bin/python export_hangar.py [--faces 6000]
See docs/superpowers/specs/2026-09-26-ship-hangar-lineup-design.md.
"""
import argparse
import json
import sqlite3
import statistics
import sys
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
sys.path.insert(0, str(HERE.parent / "footprints" / "scale"))
import cinema_frame as cf                    # noqa: E402
from compute_scale import role_group         # noqa: E402

KB_DB = Path.home() / "spacemolt" / "spacemolt-knowledge.db"
HULLS = Path.home() / "spacemolt-www" / "public" / "cinema-hulls"
SCALE = HERE.parent / "footprints" / "scale" / "ship_scale_est.json"
OUT = HERE.parent.parent / "kb" / "ships" / "hangar"
HANGAR_FACES = 6000


def EMPIRE_OF(faction):
    return faction if faction and faction != "legacy" else "independent"


def build_lineup(catalog, estimates, ladder, aspects, modeled, pages=None):
    pages = pages or {}
    group_of = {sid: role_group(c.get("class")) for sid, c in catalog.items()}
    by_group = {}
    for sid, a in aspects.items():
        by_group.setdefault(group_of.get(sid, "other"), []).append(a)
    every = list(aspects.values()) or [{"beam": .35, "height": .22}]

    def typical(sid, key):
        pool = by_group.get(group_of[sid]) or every
        return statistics.median(a[key] for a in pool)

    ships = []
    for sid, c in catalog.items():
        est = estimates.get(sid)
        if est and sid in modeled:
            length, source = float(est["loa_m"]), "window" if est.get("source") == "window" else "estimate"
            beam = float(est.get("beam_m") or length * aspects[sid]["beam"])
            height = length * aspects[sid]["height"]
        else:
            key = f"{c.get('scale')}/{group_of[sid]}"
            length = ladder["ladder_group_median"].get(key) or ladder["ladder_scale_geomean"].get(str(c.get("scale"))) or 30.0
            length, source = float(length), "estimate"
            beam, height = length * typical(sid, "beam"), length * typical(sid, "height")
        ships.append({"id": sid, "name": c["name"], "empire": EMPIRE_OF(c.get("faction")), "tier": c.get("tier") or 0,
                      "category": c.get("category") or "", "lengthM": round(length, 1), "lengthSource": source,
                      "beamM": round(beam, 1), "heightM": round(height, 1),
                      "model": f"models/{sid}.glb" if sid in modeled else None,
                      "page": pages.get(sid)})
    ships.sort(key=lambda s: (s["lengthM"], s["name"]))
    return {"version": 1, "ships": ships}


def load_catalog(db_path):
    db = sqlite3.connect(db_path)
    try:
        rows = db.execute("SELECT id, name, faction, tier, scale, category, class FROM ships").fetchall()
    finally:
        db.close()
    return {r[0]: {"name": r[1], "faction": r[2], "tier": r[3], "scale": r[4], "category": r[5], "class": r[6]} for r in rows}


def write_models(ids, faces_budget, out_dir):
    from make_lod_variants import decimate
    out_dir.mkdir(parents=True, exist_ok=True)
    aspects = {}
    for sid in ids:
        verts, faces = cf.read_glb((HULLS / f"{sid}.glb").read_bytes())
        if len(faces) > faces_budget:
            verts, faces = decimate(verts, faces, faces_budget)
        ext = np.ptp(verts, axis=0)
        aspects[sid] = {"beam": float(ext[2] / ext[0]), "height": float(ext[1] / ext[0])}
        (out_dir / f"{sid}.glb").write_bytes(cf.glb_bytes(verts.astype(np.float32), cf.vertex_normals(verts, faces), faces.astype(np.uint32)))
    return aspects


def resolve_page_dir(dirs, category):
    """Pick one directory name out of several that all hold a same-named page.

    A ship can have a page under more than one folder (a live category dir plus a
    stale Discontinued/ copy from before a reclassification). Deterministic pick:
    prefer the dir matching the ship's catalog `category`, else prefer a dir that
    isn't "Discontinued", else the first dir in sorted order.
    """
    dirs = sorted(set(dirs))
    if category in dirs:
        return category
    non_discontinued = [d for d in dirs if d != "Discontinued"]
    return non_discontinued[0] if non_discontinued else dirs[0]


def load_pages(ships_dir, categories):
    """{ship_id: "Category/id.html"} for every catalog id with a page under kb/ships/*/<id>.html.

    `categories` is {ship_id: catalog_category}; only its keys (real catalog ids) are
    considered, so stray pages like index.html are ignored. Duplicate candidates for
    the same id are resolved deterministically via `resolve_page_dir`.
    """
    candidates = {}
    for html in ships_dir.glob("*/*.html"):
        if html.stem in categories:
            candidates.setdefault(html.stem, []).append(html.parent.name)
    return {sid: f"{resolve_page_dir(dirs, categories.get(sid))}/{sid}.html" for sid, dirs in candidates.items()}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--faces", type=int, default=HANGAR_FACES)
    args = parser.parse_args()
    catalog = load_catalog(KB_DB)
    scale = json.loads(SCALE.read_text())
    modeled = {sid for sid in json.loads((HULLS / "manifest.json").read_text())["ships"] if "__lod" not in sid} & set(catalog)
    aspects = write_models(sorted(modeled), args.faces, OUT / "models")
    categories = {sid: c.get("category") or "" for sid, c in catalog.items()}
    pages = load_pages(OUT.parent, categories)
    lineup = build_lineup(catalog, scale["ships"], scale, aspects, modeled, pages)
    (OUT / "lineup.json").write_text(json.dumps(lineup, separators=(",", ":")))
    size = sum(p.stat().st_size for p in (OUT / "models").glob("*.glb"))
    print(f"{len(lineup['ships'])} ships ({len(modeled)} modeled), models {size / 1e6:.1f} MB -> {OUT}")


if __name__ == "__main__":
    main()
