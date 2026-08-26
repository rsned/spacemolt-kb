#!/usr/bin/env python3
"""Derive data/legacy.json: catalog entries the game no longer sells.

spacemolt-knowledge.db accumulates. Nothing is ever deleted from it, and
`last_updated_tick` is 0 on every module, so the DB alone cannot say whether an
item still exists. The dated API snapshots can: an id present in the DB but
absent from the newest catalog_*.json left the buyable catalog, and walking the
snapshots backwards dates when.

"Legacy" here means *not purchasable*, NOT *not usable*. Players still fly and
fit these — a Deeprock Harvester is legacy and in active service, and the five
legacy modules all appear on ships in the August battle logs. Consumers should
mark them, not hide them.

Renames matter as much as removals: around 2026-03-20 several mining hulls
changed id (mining_cruiser -> deeprock_harvester), and the DB kept both sides.
Same-name ids are cross-linked as aliases so a lookup on either finds the other.

Sources (read-only):
  <game-api>/<YYYYMMDD>/catalog_ships.json   335 entries in the newest snapshot
  <game-api>/<YYYYMMDD>/catalog_items.json   746 entries
  spacemolt-knowledge.db                     338 ships, 210 fittable modules

catalog_modules.json is deliberately NOT used: it returns "showing all 0 items"
in recent snapshots (last good capture 2026-06-22), so module status is derived
from catalog_items.json instead.

    python3 scripts/build_legacy.py [--game-api DIR] [--out data/legacy.json]
"""
import argparse
import json
import os
import re
import sqlite3
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent
DB = REPO / "spacemolt-knowledge.db"
DEFAULT_API = Path("/home/robert/spacemolt/spacemolt/data/game-api")
SNAP_RE = re.compile(r"\d{8}")


def entries(path: Path):
    """The id set from a catalog_*.json, or None when the capture is empty."""
    try:
        doc = json.loads(path.read_text())
    except (OSError, ValueError):
        return None
    rows = doc if isinstance(doc, list) else None
    if rows is None and isinstance(doc, dict):
        for key in ("items", "ships", "modules", "data"):
            if isinstance(doc.get(key), list):
                rows = doc[key]
                break
    if not rows:
        return None
    return {(r.get("id") if isinstance(r, dict) else r) for r in rows}


def newest_with_data(api: Path, snaps, filename):
    """Newest snapshot whose catalog file actually has rows, and those rows."""
    for s in snaps:
        got = entries(api / s / filename)
        if got:
            return s, got
    return None, None


def last_seen(api, snaps, filename, wanted):
    """Map id -> newest snapshot containing it, for the ids we care about."""
    out, remaining = {}, set(wanted)
    for s in snaps:
        if not remaining:
            break
        got = entries(api / s / filename)
        if not got:
            continue
        for i in list(remaining):
            if i in got:
                out[i] = s
                remaining.discard(i)
    return out


def build(kind, db_rows, api, snaps, filename):
    snap, current = newest_with_data(api, snaps, filename)
    if not current:
        sys.exit(f"no usable {filename} in any snapshot")
    legacy = {i: n for i, n in db_rows.items() if i not in current}
    seen = last_seen(api, snaps, filename, legacy)

    # Same display name on two ids means a rename; link them both ways so a
    # lookup on the retired id finds the one people actually use.
    by_name = {}
    for i, n in db_rows.items():
        by_name.setdefault(n, []).append(i)

    out = {}
    for i, n in sorted(legacy.items()):
        rec = {"name": n, "last_in_catalog": seen.get(i)}
        alias = [o for o in by_name.get(n, []) if o != i]
        if alias:
            rec["aliases"] = sorted(alias)
        out[i] = rec
    return out, snap, len(current)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--game-api", default=str(DEFAULT_API))
    ap.add_argument("--out", default=str(REPO / "data" / "legacy.json"))
    args = ap.parse_args()

    api = Path(args.game_api)
    if not api.is_dir():
        sys.exit(f"missing snapshot directory {api}")
    snaps = sorted((d for d in os.listdir(api) if SNAP_RE.fullmatch(d)), reverse=True)
    if not snaps:
        sys.exit(f"no dated snapshots under {api}")

    con = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    ships = {r[0]: r[1] for r in con.execute("select id, name from ships")}
    items = {r[0]: r[1] for r in con.execute("select id, name from items")}
    modules = {r[0] for r in con.execute("select item_id from item_modules")}

    ship_legacy, ship_snap, n_ships = build("ships", ships, api, snaps, "catalog_ships.json")
    item_legacy, item_snap, n_items = build("items", items, api, snaps, "catalog_items.json")
    for i, rec in item_legacy.items():
        rec["fittable"] = i in modules

    doc = {
        "v": 1,
        "note": "Absent from the buyable catalog. Still usable in game — mark, do not hide.",
        "snapshots": {"ships": ship_snap, "items": item_snap, "count": len(snaps)},
        "ships": ship_legacy,
        "items": item_legacy,
    }
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(doc, indent=1, sort_keys=True) + "\n")

    fit = sum(1 for r in item_legacy.values() if r["fittable"])
    print(f"build-legacy: {len(snaps)} snapshots; catalog {n_ships} ships / {n_items} items")
    print(f"  legacy ships: {len(ship_legacy)}")
    print(f"  legacy items: {len(item_legacy)}  ({fit} fittable modules)")
    print(f"  -> {out.relative_to(REPO)}")


if __name__ == "__main__":
    main()
