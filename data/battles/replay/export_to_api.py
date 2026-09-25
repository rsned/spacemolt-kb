#!/usr/bin/env python3
"""Rebuild the game API's battle summary + log entries from our battle export.

Our exports (data/battles/*.json, schema 1) are a per-frame reshaping of the
API's /api/battle/log: ships->snapshots, shots->attacks (one row per gun),
moves->zone_moves, chatter->autopilot, repairs->regen, kills->kills. This
reverses that so the spacemolt.com battle cinematic can replay battles the
server has since pruned (see replay_server.py).

Lossy by construction: per-gun rolls, crit chances and pre-defence damage
stages were dropped at export. The cinematic only needs what survives.
"""
import hashlib

CATEGORY_BY_KIND = (("creature", "wildlife"), ("police", "police"), ("pirate", "pirate"))


def _category(participants):
    kinds = {p.get("kind") for p in participants}
    for kind, category in CATEGORY_BY_KIND:
        if kind in kinds:
            return category
    return "pvp"


def _instance_id(attacker_id, name, occurrence):
    """Stable stand-in for the dropped per-gun instance id."""
    return hashlib.md5(f"{attacker_id}|{name}|{occurrence}".encode()).hexdigest()


def _attacks(shots):
    """Consecutive per-gun rows of one attacker->target become one attack."""
    attacks = []
    for shot in shots or []:
        head = attacks[-1] if attacks else None
        if not head or head["attacker_id"] != shot["from_id"] or head["target_id"] != shot["to_id"]:
            head = {
                "attacker_id": shot["from_id"], "target_id": shot["to_id"],
                "zone_distance": shot.get("zone_distance", 0),
                "hit_success": bool(shot.get("hit")), "hit_chance": 0,
                "damage_type": shot.get("damage_type", ""),
                "raw_damage": 0, "weapon_skill_pct": 0,
                "shield_damage": shot.get("shield_damage", 0),
                "hull_damage": shot.get("hull_damage", 0),
                "final_damage": shot.get("damage", 0),
                "weapons": [],
            }
            attacks.append(head)
        name = shot.get("weapon_name", "")
        occurrence = sum(1 for w in head["weapons"] if w["name"] == name)
        weapon = {
            "instance_id": _instance_id(shot["from_id"], name, occurrence), "name": name,
            "damage_type": shot.get("damage_type", ""),
            "base_damage": shot.get("weapon_damage", 0), "damage": shot.get("weapon_damage", 0),
            "after_disruption": shot.get("weapon_damage", 0), "type_bonus_pct": 0,
            "crit_chance": 0, "crit_roll": 1, "crit_fired": bool(shot.get("crit")),
        }
        if shot.get("ammo"):
            weapon["ammo_used"] = shot["ammo"]
        head["weapons"].append(weapon)
        head["raw_damage"] += weapon["damage"]
    return attacks


def convert(export, ended_at=None):
    """-> (summary, entries) in the shapes of /api/battle/summary and the log.

    ended_at (ISO 8601) is not in the export; the cinematic needs a past one to
    treat a finished battle as settled without waiting on further polls."""
    parts = export["participants"]
    by_id = {p["player_id"]: p for p in parts}
    static = ("username", "kind", "side_id", "faction_id", "ship_class", "max_hull", "max_shield", "max_fuel", "modules")
    battle_id, system_id = export["battle_id"], export["system_id"]
    category = _category(parts)
    destroyed = [p for p in parts if p.get("destroyed_at_tick") is not None]
    frames = export["frames"]
    finished = export.get("status") in ("completed", "complete")

    entries = []
    for index, frame in enumerate(frames):
        snapshots = []
        for ship in frame.get("ships", []):
            base = {k: by_id[ship["player_id"]][k] for k in static if k in by_id.get(ship["player_id"], {})}
            snapshots.append({"flee_counter": 0, "damage_dealt": 0, "damage_taken": 0, "kill_count": 0, **base, **ship})
        shields = {s["player_id"]: s for s in snapshots}
        entry = {
            "battle_id": battle_id, "system_id": system_id, "tick": frame["tick"], "snapshots": snapshots,
            "attacks": _attacks(frame.get("shots")) or None,
            "zone_moves": [{"player_id": m["player_id"], "old_zone": m.get("from", ""), "new_zone": m.get("to", ""),
                            "reason": m.get("reason", "")} for m in frame.get("moves", [])] or None,
            "autopilot": [dict(c) for c in frame.get("chatter", [])] or None,
            "regen": [{
                "player_id": r["player_id"], "shield_regen": r.get("shield_regen", 0), "armor_repair": 0,
                "shield_after": shields.get(r["player_id"], {}).get("shield", 0),
                "shield_before": shields.get(r["player_id"], {}).get("shield", 0) - r.get("shield_regen", 0),
                "hull_after": shields.get(r["player_id"], {}).get("hull", 0),
                "hull_before": shields.get(r["player_id"], {}).get("hull", 0),
            } for r in frame.get("repairs", [])] or None,
            "kills": [{"killer_id": k["killer_id"], "victim_id": k["victim_id"],
                       "killer_username": by_id.get(k["killer_id"], {}).get("username", ""),
                       "victim_username": by_id.get(k["victim_id"], {}).get("username", "")}
                      for k in frame.get("kills", [])] or None,
        }
        if index == len(frames) - 1 and finished:
            entry["battle_ended"] = {
                "outcome": export.get("outcome", ""), "winning_side": export.get("winning_side", 0),
                "duration": export.get("tick_count", len(frames)), "total_damage": export.get("total_damage", 0),
                "ships_destroyed": len(destroyed), "category": category,
                "participant_names": [p.get("username", "").lower() for p in parts],
                "participants": [{"player_id": p["player_id"], "username": p.get("username", ""),
                                  "side_id": p.get("side_id", 0)} for p in parts],
            }
        entries.append(entry)

    sides = []
    for side in export.get("sides", []):
        out = {"side_id": side["side_id"], "participants": [p.get("username", "") for p in parts if p.get("side_id") == side["side_id"]]}
        for key in ("faction_id", "faction_tag"):
            if side.get(key):
                out[key] = side[key]
        sides.append(out)
    summary = {
        "battle_id": battle_id, "system_id": system_id, "system_name": export.get("system_name", system_id),
        "status": "completed" if finished else "active", "category": category,
        "has_station": bool(export.get("has_station")), "start_tick": export.get("start_tick", frames[0]["tick"] if frames else 0),
        "duration_ticks": export.get("tick_count", len(frames)), "participant_count": len(parts), "sides": sides,
        "total_damage": export.get("total_damage", 0), "ships_destroyed": len(destroyed),
        "destroyed_names": [p.get("username", "") for p in destroyed],
        "player_names": [p.get("username", "") for p in parts if p.get("kind") == "player"],
        "outcome": export.get("outcome", ""), "winning_side": export.get("winning_side", 0),
    }
    if ended_at:
        summary["ended_at"] = ended_at
    return summary, entries


def log_page(entries, battle_id, tick_start, limit):
    """/api/battle/log pagination: entries at or after tick_start, `limit` per page."""
    rest = [e for e in entries if e["tick"] >= tick_start]
    page = rest[:limit]
    return {"battle_id": battle_id, "status": "completed", "entries": page,
            "total_ticks": len(entries), "has_more": len(rest) > limit}
