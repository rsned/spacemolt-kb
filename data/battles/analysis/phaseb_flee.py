#!/usr/bin/env python3
"""
Measure flee mechanics: counter ticks, required value, escape success.

Scenario S6 (flee series).
Prints one row per flee event with counter, required, escaped, pilot, hull, base_speed, zone.
Summary table: required values grouped by (ship_class, tactics_level).
"""
import glob
import json
import argparse
import sys
from collections import defaultdict


def entries(dirpath="data/battles/duels"):
    """Yield (battle_id_prefix, entry) tuples from all raw.json files in dirpath."""
    for f in sorted(glob.glob(f"{dirpath}/*.raw.json")):
        try:
            for page in json.load(open(f)):
                for e in page.get("entries") or []:
                    yield f.rsplit("/", 1)[1][:8], e
        except (json.JSONDecodeError, IOError):
            pass


def load_ship_speeds(catalog_path="data/combat-sim/catalog/catalog_ships.json"):
    """Load ship base_speed from catalog."""
    speeds = {}
    try:
        with open(catalog_path) as f:
            data = json.load(f)
            for item in data.get("items", []):
                speeds[item["id"]] = item.get("base_speed", 1)
    except (json.JSONDecodeError, IOError, KeyError):
        pass
    return speeds


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-d", "--dir", default="data/battles/duels",
                        help="Directory of raw battle logs (default: data/battles/duels)")
    args = parser.parse_args()

    ship_speeds = load_ship_speeds()

    # Tactics levels (hardcoded per scenario; bots are 0, craftsman-1 is 2)
    TACTICS = {
        "Arthur 'Artificer' Artis": 2,
        "craftsman-1": 2
    }

    flee_events = []

    entry_count = 0
    for battle_id_prefix, entry in entries(args.dir):
        entry_count += 1
        battle_id = entry.get("battle_id", "unknown")
        tick = entry.get("tick", -1)
        snapshots = entry.get("snapshots", [])
        flee = entry.get("flee", [])
        zone_moves = entry.get("zone_moves", [])

        if not flee or not snapshots:
            continue

        # Build snap map for current tick
        snap_map = {}
        for snap in snapshots:
            player_id = snap.get("player_id")
            if player_id:
                snap_map[player_id] = snap

        # Build zone map (for same-tick zone)
        zone_map = {}
        if zone_moves:
            for zm in zone_moves:
                player_id = zm.get("player_id")
                new_zone = zm.get("new_zone")
                if player_id and new_zone:
                    zone_map[player_id] = new_zone

        # Process flee events
        for flee_event in flee:
            player_id = flee_event.get("player_id")
            counter = flee_event.get("flee_counter", 0)
            required = flee_event.get("flee_required", 0)
            escaped = flee_event.get("escaped", False)

            snap = snap_map.get(player_id)
            if not snap:
                continue

            username = snap.get("username", "unknown")
            ship_class = snap.get("ship_class", "unknown")
            base_speed = ship_speeds.get(ship_class, 1)
            zone = zone_map.get(player_id, snap.get("zone", "unknown"))

            tactics_level = TACTICS.get(username, 0)

            flee_events.append({
                "battle_id": battle_id,
                "tick": tick,
                "player_id": player_id,
                "username": username,
                "ship_class": ship_class,
                "base_speed": base_speed,
                "counter": counter,
                "required": required,
                "escaped": escaped,
                "zone": zone,
                "tactics_level": tactics_level
            })

    if entry_count == 0:
        print("No logs found in", args.dir)
        sys.exit(0)

    # Print flee events
    print("=== Flee Events ===\n")
    if flee_events:
        print("battle_id | tick | pilot | ship_class | base_speed | counter | required | escaped | zone | tactics")
        for evt in flee_events:
            print(f"{evt['battle_id']} | {evt['tick']} | {evt['username']:<20} | "
                  f"{evt['ship_class']:<15} | {evt['base_speed']} | {evt['counter']} | "
                  f"{evt['required']} | {evt['escaped']} | {evt['zone']:<8} | {evt['tactics_level']}")
    else:
        print("(No flee events found)")

    # Summary table: required grouped by (ship_class, tactics_level)
    print("\n=== Flee Required Summary ===\n")
    required_by_class_tactics = defaultdict(list)
    for evt in flee_events:
        key = (evt['ship_class'], evt['tactics_level'])
        required_by_class_tactics[key].append(evt['required'])

    if required_by_class_tactics:
        print("ship_class | tactics | count | avg_required | distinct_required")
        for (ship_class, tactics), required_vals in sorted(required_by_class_tactics.items()):
            avg_req = sum(required_vals) / len(required_vals)
            distinct = len(set(required_vals))
            print(f"{ship_class:<15} | {tactics} | {len(required_vals)} | {avg_req:.1f} | {sorted(set(required_vals))}")
    else:
        print("(No flee summary data)")


if __name__ == "__main__":
    main()
