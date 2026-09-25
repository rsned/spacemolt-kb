#!/usr/bin/env python3
"""Synthetic four-way barrage for cinematic screenshots and browser load tests.

Takes the 420-ship stress test (ffff...ffff.json: fleets and movement only, no
fire), arms every ship with a per-side weapon theme so the four sides read as
four colours, and makes every ship fire every tick at an enemy on another side.
~8% of ships die in the last third so there are explosions. Deliberately
invented data: outcome "synthetic", never mixed into real-battle stats.

    python3 make_barrage.py [--ticks 120] [--seed 7]
    -> data/battles/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee.json (replay_server serves it)
    python3 make_barrage.py --alpha --ticks 8
    -> data/battles/dddddddddddddddddddddddddddddddd.json: one full alpha strike, then quiet
"""
import argparse
import copy
import json
import random
from pathlib import Path

BATTLES = Path(__file__).resolve().parent.parent
SOURCE = BATTLES / ("f" * 32 + ".json")
BARRAGE_ID = "e" * 32
ALPHA_ID = "d" * 32

# (name, damage_type, weapon_damage) — real catalog weapons, one theme per side.
THEMES = {
    1: [("Pulse Laser III", "energy", 28), ("Focused Beam II", "energy", 25), ("Solar Lance", "energy", 65)],
    2: [("Railgun I", "kinetic", 45), ("Autocannon I", "kinetic", 10), ("Flak Cannon II", "kinetic", 28)],
    3: [("Plasma Cannon III", "thermal", 58), ("Missile Launcher I", "explosive", 20), ("Heavy Torpedo", "explosive", 80)],
    4: [("Phase Disruptor", "void", 48), ("Void Lance I", "void", 55), ("Ion Blaster III", "em", 22)],
}
RETARGET_EVERY = 12
KILL_FRACTION = .08


def build(source, ticks, seed):
    rng = random.Random(seed)
    battle = copy.deepcopy(source)
    frames = battle["frames"][:ticks]
    parts = battle["participants"]
    by_side = {}
    for p in parts:
        by_side.setdefault(p["side_id"], []).append(p["player_id"])
        theme = THEMES[p["side_id"]]
        guns = 6 if p["kind"] == "station" else rng.randint(2, 4)
        p["modules"] = [{"name": rng.choice(theme)[0], "category": "module"} for _ in range(guns)]
        p["destroyed_at_tick"], p["killed_by"] = 0, ""
    weapons = {p["player_id"]: [next(w for w in THEMES[p["side_id"]] if w[0] == m["name"]) for m in p["modules"]] for p in parts}
    side_of = {p["player_id"]: p["side_id"] for p in parts}
    ships = [p["player_id"] for p in parts if p["kind"] != "station"]

    # Victims die in the last third, killed by whoever is firing at them then.
    doomed = set(rng.sample(ships, int(len(ships) * KILL_FRACTION)))
    death_tick = {pid: rng.randrange(ticks * 2 // 3, ticks) for pid in doomed}
    dead, targets = set(), {}
    for index, frame in enumerate(frames):
        alive = [s for s in frame["ships"] if s["player_id"] not in dead]
        alive_ids = {s["player_id"] for s in alive}
        shots, kills = [], []
        for ship in alive:
            pid = ship["player_id"]
            if index % RETARGET_EVERY == 0 or targets.get(pid) not in alive_ids:
                enemies = [e for e in alive_ids if side_of[e] != side_of[pid]]
                targets[pid] = rng.choice(enemies) if enemies else None
            target = targets[pid]
            ship["target_id"] = target or ""
            ship["stance"] = "fire"
            if not target:
                continue
            hit = rng.random() < .8
            for name, damage_type, damage in weapons[pid]:
                shots.append({"from_id": pid, "to_id": target, "kind": "beam", "weapon_name": name,
                              "damage_type": damage_type, "hit": hit, "damage": damage if hit else 0,
                              "shield_damage": damage if hit else 0, "weapon_damage": damage, "zone_distance": 1})
        for victim, tick_index in death_tick.items():
            if tick_index == index and victim in alive_ids:
                killer = next((s["player_id"] for s in alive if targets.get(s["player_id"]) == victim), None) \
                    or next(e for e in alive_ids if side_of[e] != side_of[victim])
                kills.append({"killer_id": killer, "victim_id": victim})
                dead.add(victim)
                part = next(p for p in parts if p["player_id"] == victim)
                part["destroyed_at_tick"], part["killed_by"] = frame["tick"], killer
                for s in alive:
                    if s["player_id"] == victim:
                        s["hull"], s["shield"] = 0, 0
        frame["ships"] = [s for s in frame["ships"] if s["player_id"] not in dead or s["player_id"] in {k["victim_id"] for k in kills}]
        frame["shots"], frame["kills"] = shots, kills

    battle.update({"battle_id": BARRAGE_ID, "system_name": "SYNTHETIC BARRAGE", "frames": frames,
                   "status": "complete", "outcome": "synthetic", "tick_count": len(frames), "total_ticks": len(frames),
                   "end_tick": frames[-1]["tick"],
                   "total_damage": sum(s["damage"] for f in frames for s in f["shots"])})
    return battle


def build_alpha(source, ticks, seed):
    """Every ship fires every gun once, at an enemy, on the first tick; then quiet."""
    battle = build(source, ticks, seed)
    first, rest = battle["frames"][0], battle["frames"][1:]
    first["shots"] = [dict(s, hit=True, damage=s["weapon_damage"], shield_damage=s["weapon_damage"]) for s in first["shots"]]
    for frame in rest:
        frame["shots"], frame["kills"] = [], []
    # Nobody dies in an alpha strike; restore anyone the barrage schedule removed.
    for p in battle["participants"]:
        p["destroyed_at_tick"], p["killed_by"] = 0, ""
    # Every frame keeps the full fleet at source health, aimed where the volley went.
    targets = {s["from_id"]: s["to_id"] for s in first["shots"]}
    for frame, original in zip(battle["frames"], source["frames"][:ticks]):
        frame["ships"] = [dict(s, stance="fire", target_id=targets.get(s["player_id"], s.get("target_id", "")))
                          for s in original["ships"]]
    battle.update({"battle_id": ALPHA_ID, "system_name": "ALPHA STRIKE",
                   "total_damage": sum(s["damage"] for s in first["shots"])})
    return battle


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--ticks", type=int, default=120)
    parser.add_argument("--seed", type=int, default=7)
    parser.add_argument("--alpha", action="store_true", help="one volley from every ship on the first tick (dddd...dddd.json)")
    args = parser.parse_args()
    source = json.loads(SOURCE.read_text())
    battle = build_alpha(source, args.ticks, args.seed) if args.alpha else build(source, args.ticks, args.seed)
    out = BATTLES / f"{battle['battle_id']}.json"
    out.write_text(json.dumps(battle))
    shots = sum(len(f["shots"]) for f in battle["frames"])
    kills = sum(len(f["kills"]) for f in battle["frames"])
    print(f"{out.name}: {len(battle['participants'])} ships, {len(battle['frames'])} ticks, {shots} gun shots, {kills} kills")


if __name__ == "__main__":
    main()
