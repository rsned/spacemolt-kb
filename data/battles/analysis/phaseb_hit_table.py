#!/usr/bin/env python3
"""
Measure hit_chance by zone_distance and speed differential.

Scenarios S1 (hit table) and S2 (speed modifier) contribute.
Groups attacks by zone_distance, prints observed hit_chance stats per distance,
buckets by attacker base_speed - target base_speed using catalog_ships.json.
Proposes a hit_chance_by_distance array and fits base[d] + k*speed_diff.
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

    # Collect attacks by distance
    attacks_by_distance = defaultdict(list)  # distance -> list of (hit_chance, speed_diff)
    # Collect all hit_chance values per distance (for distinct/min/max)
    hit_chances_by_distance = defaultdict(list)

    entry_count = 0
    for battle_id_prefix, entry in entries(args.dir):
        entry_count += 1
        tick = entry.get("tick", -1)
        snapshots = entry.get("snapshots", [])
        attacks = entry.get("attacks", [])

        if not attacks or not snapshots:
            continue

        # Build snapshot map
        snap_map = {}
        for snap in snapshots:
            player_id = snap.get("player_id")
            if player_id:
                snap_map[player_id] = snap

        # Process each attack
        for attack in attacks:
            attacker_id = attack.get("attacker_id")
            target_id = attack.get("target_id")
            hit_chance = attack.get("hit_chance")
            zone_distance = attack.get("zone_distance")

            if hit_chance is None or zone_distance is None:
                continue

            attacker_snap = snap_map.get(attacker_id)
            target_snap = snap_map.get(target_id)

            if not attacker_snap or not target_snap:
                continue

            attacker_ship = attacker_snap.get("ship_class")
            target_ship = target_snap.get("ship_class")

            attacker_speed = ship_speeds.get(attacker_ship, 1)
            target_speed = ship_speeds.get(target_ship, 1)
            speed_diff = attacker_speed - target_speed

            hit_chances_by_distance[zone_distance].append(hit_chance)
            attacks_by_distance[zone_distance].append((hit_chance, speed_diff))

    if entry_count == 0:
        print("No logs found in", args.dir)
        sys.exit(0)

    # Print summary per distance
    print("=== Hit Chance by Zone Distance ===\n")
    distances = sorted(hit_chances_by_distance.keys())

    for dist in distances:
        values = hit_chances_by_distance[dist]
        distinct = len(set(values))
        print(f"Distance {dist}: n={len(values)}, distinct={distinct}, "
              f"min={min(values):.2f}, max={max(values):.2f}")

    print("\n=== Speed Differential Buckets (S2 if present) ===\n")

    # Bucket by speed_diff
    by_speed_diff = defaultdict(lambda: defaultdict(list))
    for dist, attack_list in attacks_by_distance.items():
        for hit_chance, speed_diff in attack_list:
            by_speed_diff[speed_diff][dist].append(hit_chance)

    for speed_diff in sorted(by_speed_diff.keys()):
        print(f"Speed diff {speed_diff:+d}:")
        for dist in sorted(by_speed_diff[speed_diff].keys()):
            values = by_speed_diff[speed_diff][dist]
            if values:
                print(f"  Distance {dist}: n={len(values)}, avg={sum(values)/len(values):.3f}")

    # Propose hit_chance_by_distance array (max per distance)
    print("\n=== Proposed hit_chance_by_distance ===\n")
    proposed = []
    for dist in range(7):  # distances 0-6
        if dist in hit_chances_by_distance:
            proposed.append(max(hit_chances_by_distance[dist]))
        else:
            proposed.append(None)
    print("[", end="")
    for i, val in enumerate(proposed):
        if val is not None:
            print(f"{val:.2f}" if i < len(proposed)-1 else f"{val:.2f}", end="")
        if i < len(proposed) - 1:
            print(", ", end="")
    print("]")

    # Simple least-squares fit: base[d] + k*speed_diff
    print("\n=== Least-Squares Fit: base[d] + k*speed_diff ===\n")
    all_observations = []
    for dist, attack_list in attacks_by_distance.items():
        for hit_chance, speed_diff in attack_list:
            all_observations.append((dist, speed_diff, hit_chance))

    if len(all_observations) >= 2:
        # Group by (distance, speed_diff) and average
        grouped = defaultdict(list)
        for dist, speed_diff, hit_chance in all_observations:
            grouped[(dist, speed_diff)].append(hit_chance)

        distinct_points = []
        for (dist, speed_diff), values in sorted(grouped.items()):
            avg_hit = sum(values) / len(values)
            distinct_points.append((dist, speed_diff, avg_hit))

        # Fit: assume base[d] is max(hit_chance at distance d with speed_diff=0)
        base_by_dist = {}
        for dist in distances:
            candidates = [h for d, sd, h in distinct_points if d == dist and sd == 0]
            if candidates:
                base_by_dist[dist] = max(candidates)

        # Compute residuals and estimate k
        residuals = []
        for dist, speed_diff, avg_hit in distinct_points:
            if dist in base_by_dist:
                base = base_by_dist[dist]
                predicted_delta = avg_hit - base
                residuals.append((speed_diff, predicted_delta))

        if residuals:
            print("Residuals (speed_diff, hit_chance_delta):")
            for sd, delta in sorted(residuals):
                print(f"  {sd:+d}: {delta:+.3f}")

            # Estimate k via least squares: k = sum(sd * delta) / sum(sd^2)
            numerator = sum(sd * delta for sd, delta in residuals)
            denominator = sum(sd * sd for sd, delta in residuals)
            if denominator > 0:
                k = numerator / denominator
                print(f"\nEstimated k (per speed_diff point): {k:+.4f}")


if __name__ == "__main__":
    main()
