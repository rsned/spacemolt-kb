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
OVERLAY = REPO / "overlays" / "generated"

# Retired hulls have no category in the DB (that field is only set from the
# catalog they have left), and the ship pages are filed by category. Giving them
# their own bucket both files them and labels them.
LEGACY_CATEGORY = "Discontinued"

# Ship columns that map 1:1 onto the catalog_ships.json field names the KB's
# Ship struct unmarshals, so a DB row can stand in for a catalog entry.
SHIP_COLS = [
    "id", "name", "class", "category", "faction", "tier", "base_hull",
    "base_armor", "base_shield", "base_shield_recharge", "base_speed",
    "base_fuel", "cargo_capacity", "weapon_slots", "defense_slots",
    "utility_slots", "power_capacity", "cpu_capacity", "scale", "build_time",
    "price", "shipyard_tier", "starter_ship", "description", "lore",
    "based_on", "npc_role", "special", "piloting_required",
]
JSON_COLS = {"flavor_tags", "default_modules", "passive_recipes"}
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


def catalog_history(api, snaps, filename):
    """id -> (newest snapshot carrying it, its record there).

    The knowledge DB only holds what agents have actually met, so a hull retired
    before the DB ever saw it is invisible to a DB-vs-catalog diff -- Benefit
    left the catalog on 2026-03-30 and has no DB row at all. The dated snapshots
    are the real history and this reads all of them. snaps is newest-first, so
    the first sighting of an id is the last catalog that carried it.
    """
    hist = {}
    for s in snaps:
        try:
            doc = json.loads((api / s / filename).read_text())
        except (OSError, ValueError):
            continue
        rows = doc.get("items") if isinstance(doc, dict) else doc
        if not rows:
            continue
        for r in rows:
            if isinstance(r, dict) and r.get("id") and r["id"] not in hist:
                hist[r["id"]] = (s, r)
    return hist


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


def build(kind, db_rows, api, snaps, filename, history=None):
    """Retired ids for one catalog, from the DB and optionally the snapshots.

    The two sources are complementary, not redundant. The DB knows ids the live
    game uses that no catalog ever listed (deeprock_harvester, and prospector
    which predates every snapshot); the snapshots know ids retired before the DB
    met them (benefit, and 45 more). Neither alone is the answer.
    """
    snap, current = newest_with_data(api, snaps, filename)
    if not current:
        sys.exit(f"no usable {filename} in any snapshot")

    names = dict(db_rows)
    if history:
        for i, (_, rec) in history.items():
            names.setdefault(i, (rec.get("name") or "").strip())

    legacy = {i: n for i, n in names.items() if i not in current}

    # Most vanished ids are renames, not retirements: the 2026-03-03 faction
    # prefix drop alone moved 225 hulls (crimson_billhook -> billhook). If the
    # display name still ships under a current id the ship was never retired,
    # so it needs no legacy page -- it already has a live one.
    if history:
        live = {(history[i][1].get("name") or "").strip().lower()
                for i in current if i in history}
        legacy = {i: n for i, n in legacy.items()
                  if (n or "").strip().lower() not in live}

    seen = last_seen(api, snaps, filename, legacy)
    if history:
        for i in legacy:
            if i in history:
                seen[i] = history[i][0]

    # Same display name on two ids means a rename; link them both ways so a
    # lookup on the retired id finds the one people actually use.
    by_name = {}
    for i, n in names.items():
        by_name.setdefault(n, []).append(i)

    out = {}
    for i, n in sorted(legacy.items()):
        rec = {"name": n, "last_in_catalog": seen.get(i)}
        alias = [o for o in by_name.get(n, []) if o != i and o in legacy]
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

    ship_history = catalog_history(api, snaps, "catalog_ships.json")
    ship_legacy, ship_snap, n_ships = build(
        "ships", ships, api, snaps, "catalog_ships.json", history=ship_history)
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

    # Catalog-shaped records for the retired hulls, so generate-items-kb can
    # render pages for entries the catalog no longer carries. Same idea as the
    # other overlays/generated files: derived data a generator merges in.
    cols = ", ".join(SHIP_COLS + sorted(JSON_COLS))
    q = f"select {cols} from ships where id in ({','.join('?' * len(ship_legacy))})"
    names = SHIP_COLS + sorted(JSON_COLS)
    records, from_db = [], set()
    for row in con.execute(q, tuple(sorted(ship_legacy))):
        rec = dict(zip(names, row))
        for k in JSON_COLS:
            try:
                rec[k] = json.loads(rec[k]) if rec[k] else []
            except (ValueError, TypeError):
                rec[k] = []
        rec["starter_ship"] = bool(rec["starter_ship"])
        from_db.add(rec["id"])
        records.append(rec)

    # Hulls the DB never met come straight from the last snapshot that carried
    # them. That file IS catalog_ships.json, so the record already has exactly
    # the shape the KB's Ship struct unmarshals -- no column mapping needed.
    for sid in sorted(ship_legacy):
        if sid in from_db or sid not in ship_history:
            continue
        records.append(dict(ship_history[sid][1]))

    for rec in records:
        # Every emitted hull is retired, so they all file under one bucket —
        # otherwise a renamed pair lands in two different category directories.
        rec["category"] = LEGACY_CATEGORY
        rec["legacy"] = True
        rec["aliases"] = ship_legacy[rec["id"]].get("aliases", [])

    # A rename leaves two ids for one ship. Emitting both would publish the same
    # hull twice, so keep one page per name and fold the other ids into it as
    # aliases. The surviving id is the one that never appeared in the ship
    # catalog on its own: the catalog entry is what the rename retired, and the
    # replacement id was never separately listed for sale.
    def survives(sid):
        """Sort key for which id of a renamed pair keeps the page.

        Highest wins. An id with no catalog history at all is the newest thing
        we know: the catalog entry is what the rename retired, and the
        replacement was never separately listed (mining_cruiser ->
        deeprock_harvester). Otherwise the one the catalog carried most recently
        wins -- nebula_benefit last shipped 2026-02-27, benefit 2026-03-27, and
        Benefit is the name on the hull players are buying today.
        """
        lic = ship_legacy[sid].get("last_in_catalog")
        return (1, "") if not lic else (0, lic)

    chosen, dropped = {}, []
    for rec in sorted(records, key=lambda r: r["id"]):
        prev = chosen.get(rec["name"])
        if prev is None:
            chosen[rec["name"]] = rec
            continue
        keep, drop = ((rec, prev) if survives(rec["id"]) > survives(prev["id"])
                      else (prev, rec))
        merged = set(keep["aliases"]) | {drop["id"]} | set(drop["aliases"])
        keep["aliases"] = sorted(merged - {keep["id"]})   # never alias to itself
        chosen[rec["name"]] = keep
        dropped.append(drop["id"])
    records = sorted(chosen.values(), key=lambda r: r["name"])
    OVERLAY.mkdir(parents=True, exist_ok=True)
    (OVERLAY / "legacy_ships.json").write_text(
        json.dumps(records, indent=1, sort_keys=True) + "\n")

    # Same treatment for retired items. generate-items-kb builds its item map
    # from the crafting DB and then overlays module stats from catalog_items.json;
    # a retired item is in neither, so it is skipped entirely. This emits one
    # catalog_items-shaped record per retired item, carrying both the base fields
    # (so the generator can create the item) and the module stats (so the overlay
    # can dress it), rebuilt from the knowledge DB.
    ITEM_BASE = """select id, name, coalesce(description,''), coalesce(category,''),
             coalesce(rarity,''), size, base_value, stackable, tradeable,
             coalesce(power_bonus,0), hazardous, quest_item,
             coalesce(extracted_by,''), coalesce(required_skills,''),
             coalesce(region_lock,''), passenger_economy_berths,
             passenger_business_berths, passenger_first_berths
             from items where id = ?"""
    BASE_KEYS = ["id", "name", "description", "category", "rarity", "size",
                 "base_value", "stackable", "tradeable", "power_bonus",
                 "hazardous", "quest_item", "extracted_by", "required_skills",
                 "region_lock", "passenger_economy_berths",
                 "passenger_business_berths", "passenger_first_berths"]
    SUB = [
        ("item_modules", {"type": "type", "type_id": "type_id", "slot": "slot",
                          "special": "special", "cpu_usage": "cpu_usage",
                          "power_usage": "power_usage", "hidden": "hidden"}),
        ("item_weapons", {"damage": "damage", "damage_type": "damage_type",
                          "range": "range", "reach": "reach", "cooldown": "cooldown",
                          "ammo_type": "ammo_type", "magazine_size": "magazine_size",
                          "armor_bypass_bonus": "armor_bypass_bonus",
                          "shield_bypass_bonus": "shield_bypass_bonus"}),
        ("item_defenses", {"armor_bonus": "armor_bonus", "hull_bonus": "hull_bonus",
                           "shield_bonus": "shield_bonus",
                           "shield_recharge_bonus": "shield_recharge_bonus",
                           "armor_repair_rate": "armor_repair_rate",
                           "resistance_bonus": "resistance_bonus",
                           "damage_reduction": "damage_reduction",
                           "cloak_strength": "cloak_strength"}),
        ("item_utilities", {"speed_bonus": "speed_bonus", "cargo_bonus": "cargo_bonus",
                            "cloak_strength": "cloak_strength",
                            "scanner_power": "scanner_power",
                            "accuracy_bonus": "accuracy_bonus",
                            "tracking_bonus": "tracking_bonus",
                            "signature_bonus": "signature_bonus",
                            "fuel_efficiency": "fuel_efficiency",
                            "drone_bandwidth": "drone_bandwidth",
                            "drone_capacity": "drone_capacity",
                            "harvest_power": "mining_power",
                            "survey_power": "survey_power",
                            "survey_range": "survey_range",
                            "tow_speed_penalty": "tow_speed_penalty",
                            "cpu_bonus": "cpu_bonus", "max_fuel_bonus": "max_fuel_bonus",
                            "hull_penalty": "hull_penalty",
                            "speed_penalty": "speed_penalty"}),
    ]

    def table_cols(name):
        return {r[1] for r in con.execute(f"pragma table_info({name})")}

    item_records = []
    for iid in sorted(item_legacy):
        row = con.execute(ITEM_BASE, (iid,)).fetchone()
        if not row:
            continue
        rec = dict(zip(BASE_KEYS, row))
        rec["stackable"] = bool(rec["stackable"])
        rec["tradeable"] = bool(rec["tradeable"])
        rec["hazardous"] = bool(rec["hazardous"])
        rec["quest_item"] = bool(rec["quest_item"])
        for key in ("required_skills", "region_lock"):
            try:
                rec[key] = json.loads(rec[key]) if rec[key] else None
            except (ValueError, TypeError):
                rec[key] = None
        for table, mapping in SUB:
            have = table_cols(table)
            cols = [c for c in mapping if c in have]
            if not cols:
                continue
            r = con.execute(
                f"select {', '.join(cols)} from {table} where item_id = ?", (iid,)
            ).fetchone()
            if not r:
                continue
            for col, val in zip(cols, r):
                if val in (None, ""):
                    continue
                key = mapping[col]
                if key == "resistance_bonus":
                    try:
                        val = json.loads(val)
                    except (ValueError, TypeError):
                        continue
                if key == "hidden":
                    val = bool(val)
                rec[key] = val
        if not rec.get("category") and rec.get("slot"):
            rec["category"] = rec["slot"]
        rec["legacy"] = True
        item_records.append(rec)

    (OVERLAY / "legacy_items.json").write_text(
        json.dumps({"items": item_records}, indent=1, sort_keys=True) + "\n")

    fit = sum(1 for r in item_legacy.values() if r["fittable"])
    print(f"build-legacy: {len(snaps)} snapshots; catalog {n_ships} ships / {n_items} items")
    print(f"  legacy ships: {len(ship_legacy)}")
    print(f"  legacy items: {len(item_legacy)}  ({fit} fittable modules)")
    print(f"  -> {out.relative_to(REPO)}")
    print(f"  -> overlays/generated/legacy_items.json ({len(item_records)} catalog-shaped items)")
    print(f"  -> overlays/generated/legacy_ships.json ({len(records)} catalog-shaped hulls"
          + (f"; {len(dropped)} renamed ids folded in as aliases: {', '.join(dropped)}" if dropped else "")
          + ")")


if __name__ == "__main__":
    main()
