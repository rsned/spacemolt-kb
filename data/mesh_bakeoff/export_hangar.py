#!/usr/bin/env python3
"""Export the KB ship hangar: kb/ships/hangar/lineup.json + decimated models.

    ~/hy3d-venv/bin/python export_hangar.py [--faces 6000] [--db PATH] [--hulls DIR] [--out DIR]
See docs/superpowers/specs/2026-09-26-ship-hangar-lineup-design.md.
"""
import argparse
import datetime as dt
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
        page = pages.get(sid)
        # No DB category: fall back to the directory the ship's page lives in.
        category = c.get("category") or (page.split("/")[0] if page else "")
        ships.append({"id": sid, "name": c["name"], "empire": EMPIRE_OF(c.get("faction")), "tier": c.get("tier") or 0,
                      "category": category, "lengthM": round(length, 1), "lengthSource": source,
                      "beamM": round(beam, 1), "heightM": round(height, 1),
                      "model": f"models/{sid}.glb" if sid in modeled else None,
                      "page": page})
    ships.sort(key=lambda s: (s["lengthM"], s["name"], s["id"]))
    return {"version": 1, "ships": ships}


def load_catalog(db_path):
    """Catalog ships keyed by id; faction 'legacy' rows are old-id duplicates and are dropped."""
    db = sqlite3.connect(db_path)
    try:
        rows = db.execute("SELECT id, name, faction, tier, scale, category, class FROM ships").fetchall()
    finally:
        db.close()
    return {r[0]: {"name": r[1], "faction": r[2], "tier": r[3], "scale": r[4], "category": r[5], "class": r[6]}
            for r in rows if r[2] != "legacy"}


def hull_ids(manifest_path):
    """Hull ids in the cinema-hulls manifest, without the __lod variants."""
    return [sid for sid in json.loads(Path(manifest_path).read_text())["ships"] if "__lod" not in sid]


def _shown(path):
    """Path as text, with the home directory shown as ~ (the KB is published)."""
    home, text = str(Path.home()), str(path)
    return "~" + text[len(home):] if text.startswith(home + "/") else text


def source_info(faces, manifest_path, db_path, now=None):
    """Provenance block for lineup.json: what inputs produced this export."""
    now = now or dt.datetime.now(dt.timezone.utc)
    mtime = dt.datetime.fromtimestamp(Path(db_path).stat().st_mtime, dt.timezone.utc)
    return {"faces": faces, "generated": now.strftime("%Y-%m-%d"),
            "hulls": {"manifest": _shown(manifest_path), "ships": len(hull_ids(manifest_path))},
            "db": {"path": _shown(db_path), "mtime": mtime.strftime("%Y-%m-%dT%H:%M:%SZ")}}


def prune_models(models_dir, modeled):
    """Delete models/*.glb whose id is not in the modeled set; returns the removed file names."""
    removed = sorted(p.name for p in Path(models_dir).glob("*.glb") if p.stem not in modeled)
    for name in removed:
        (Path(models_dir) / name).unlink()
    return removed


def write_models(ids, faces_budget, out_dir, hulls=HULLS):
    from make_lod_variants import decimate
    out_dir.mkdir(parents=True, exist_ok=True)
    aspects = {}
    for sid in ids:
        verts, faces = cf.read_glb((hulls / f"{sid}.glb").read_bytes())
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


def parse_args(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("--faces", type=int, default=HANGAR_FACES)
    parser.add_argument("--db", type=Path, default=KB_DB, help="knowledge DB with the ships table")
    parser.add_argument("--hulls", type=Path, default=HULLS, help="cinema-hulls dir (manifest.json + <id>.glb)")
    parser.add_argument("--out", type=Path, default=OUT, help="hangar output dir (lineup.json + models/)")
    return parser.parse_args(argv)


def main():
    args = parse_args()
    catalog = load_catalog(args.db)
    scale = json.loads(SCALE.read_text())
    manifest = args.hulls / "manifest.json"
    modeled = set(hull_ids(manifest)) & set(catalog)
    aspects = write_models(sorted(modeled), args.faces, args.out / "models", args.hulls)
    removed = prune_models(args.out / "models", modeled)
    categories = {sid: c.get("category") or "" for sid, c in catalog.items()}
    pages = load_pages(args.out.parent, categories)
    lineup = build_lineup(catalog, scale["ships"], scale, aspects, modeled, pages)
    lineup["source"] = source_info(args.faces, manifest, args.db)
    (args.out / "lineup.json").write_text(json.dumps(lineup, separators=(",", ":")))
    size = sum(p.stat().st_size for p in (args.out / "models").glob("*.glb"))
    print(f"{len(lineup['ships'])} ships ({len(modeled)} modeled), models {size / 1e6:.1f} MB, "
          f"removed {len(removed)} stale GLBs {removed} -> {args.out}")


if __name__ == "__main__":
    main()
