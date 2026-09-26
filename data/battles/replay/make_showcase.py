#!/usr/bin/env python3
"""Synthetic fleet review: one of every ship class that has a Hy3D model, sides
by empire, so the cinematic shows the whole modeled fleet together.

Each tick a handful of ships land a one-point shield scratch on another side
(the cinematic only films encounters with a recorded hit); the
director frames whoever acts, so the camera tours the fleet ship by ship.
Nobody takes damage. Deliberately invented data (outcome "synthetic").

    python3 make_showcase.py [--per-tick 8]
    -> data/battles/cccccccccccccccccccccccccccccccc.json (replay_server serves it)
    python3 make_showcase.py --lod axiom,close_enough,hells_bells,absolute_entropy,opus_magna
    -> data/battles/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.json: one side per level of detail
"""
import argparse
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
BATTLES = HERE.parent
SHOWCASE_ID = "c" * 32
LOD_REVIEW_ID = "b" * 32
MANIFEST = Path.home() / "spacemolt-www" / "public" / "cinema-hulls" / "manifest.json"
ID_MAP = HERE.parent.parent / "mesh_bakeoff" / "ship_id_map.json"
EMPIRES = ["solarian", "crimson", "nebula", "outerrim", "voidborn", "pirate", ""]
KIND = {"pirate": "pirate"}
START_TICK = 2_000_000


def build(ship_factions, per_tick=8):
    """ship_factions: {ship_class: faction ('' = unaffiliated)} -> one side per empire."""
    groups = [((f or "independent").upper(), KIND.get(f, "player"), sorted(c for c, v in ship_factions.items() if v == f))
              for f in EMPIRES if any(v == f for v in ship_factions.values())]
    return build_groups(groups, SHOWCASE_ID, "FLEET REVIEW", per_tick)


def build_lod_review(ships, levels, per_tick=8):
    """One side per level of detail ("FULL 40K" = the exported original), same ships in each."""
    groups = [("FULL 40K", "player", list(ships))] + \
             [(level.upper(), "player", [f"{ship}__lod{level}" for ship in ships]) for level in levels]
    return build_groups(groups, LOD_REVIEW_ID, "LOD REVIEW", per_tick)


def build_groups(groups, battle_id, system_name, per_tick=8):
    """groups: [(side tag, kind, [ship_class, ...])] -> a synthetic, filmable battle."""
    parts = []
    for side, (tag, kind, classes) in enumerate(groups, start=1):
        for n, ship_class in enumerate(classes):
            label = ship_class.replace("__lod", " @ ").replace("_", " ").title()
            parts.append({"player_id": f"s{side}_{n}", "username": label,
                          "kind": kind, "side_id": side, "ship_class": ship_class,
                          "max_hull": 100, "max_shield": 100, "max_fuel": 100,
                          "modules": [{"name": "Pulse Laser I", "category": "module"}],
                          "first_tick": START_TICK, "destroyed_at_tick": 0, "killed_by": ""})
    order = [p for p in parts]
    ticks = -(-len(order) // per_tick) + 2          # a quiet establishing tick and a closing one
    frames = []
    for t in range(ticks):
        volley = order[(t - 1) * per_tick:t * per_tick] if 0 < t <= ticks - 2 else []
        shots = []
        for i, p in enumerate(volley):
            others = [q for q in parts if q["side_id"] != p["side_id"]]
            target = others[(t * 7 + i * 13) % len(others)]
            shots.append({"from_id": p["player_id"], "to_id": target["player_id"], "kind": "beam",
                          "weapon_name": "Pulse Laser I", "damage_type": "energy", "hit": True,
                          "damage": 1, "shield_damage": 1, "weapon_damage": 10, "zone_distance": 1})
        frames.append({"tick": START_TICK + t, "shots": shots, "kills": [], "moves": [], "chatter": [], "repairs": [],
                       "ships": [{"player_id": p["player_id"], "x": 0, "y": 0, "zone": "mid", "hull": 100, "shield": 100,
                                  "fuel": 100, "stance": "fire", "target_id": "", "auto_pilot": True} for p in parts]})
    for p in parts:
        p["last_tick"] = START_TICK + ticks - 1
    return {"schema": 1, "battle_id": battle_id, "system_id": "sol", "system_name": system_name,
            "status": "complete", "outcome": "synthetic", "winning_side": 0, "has_station": False,
            "start_tick": START_TICK, "end_tick": START_TICK + ticks - 1, "tick_count": ticks, "total_ticks": ticks,
            "total_damage": len(order), "zones": ["outer", "mid", "inner", "engaged"],
            "sides": [{"side_id": side, "faction_tag": tag, "count": len(classes)}
                      for side, (tag, _kind, classes) in enumerate(groups, start=1)],
            "participants": parts, "frames": frames}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--per-tick", type=int, default=8)
    parser.add_argument("--lod", default="", help="ships for a level-of-detail review (bbbb...bbbb.json), e.g. axiom,opus_magna")
    parser.add_argument("--levels", default="16k,8k,4k,2k")
    args = parser.parse_args()
    modeled = json.loads(MANIFEST.read_text())["ships"]
    if args.lod:
        battle = build_lod_review([s for s in args.lod.split(",") if s], [l for l in args.levels.split(",") if l], args.per_tick)
        missing = [p["ship_class"] for p in battle["participants"] if p["ship_class"] not in modeled]
        if missing:
            raise SystemExit(f"no exported model for {missing}: run make_lod_variants.py first")
    else:
        faction = {m["id"]: m.get("faction") or "" for m in json.loads(ID_MAP.read_text())["mapping"].values()}
        battle = build({ship: faction.get(ship, "") for ship in modeled if "__lod" not in ship}, args.per_tick)
    out = BATTLES / f"{battle['battle_id']}.json"
    out.write_text(json.dumps(battle))
    print(f"{out.name}: {len(battle['participants'])} ships in {len(battle['sides'])} sides, {len(battle['frames'])} ticks")


if __name__ == "__main__":
    main()
