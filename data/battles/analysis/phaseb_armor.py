#!/usr/bin/env python3
"""
Measure armor damage law for low armor values (0-30).

Scenario S7 (armor ladder).
For every hull-landing volley, prints armor total (inferred from fitted modules),
pre_hit damage, observed hull damage, and predictions for two laws:
  - flat: max(1, pre - floor(c)) where c = 0.75*armor (energy) or 1.5*armor (kinetic)
  - saturating: max(1, floor(pre * (1 - c/(c+150)))) where c = 0.75*armor or 1.5*armor
Ends with per-law exact-match counts per armor value.
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


def load_ship_base_armor(catalog_path="data/combat-sim/catalog/catalog_ships.json"):
    """Load base_armor per ship from catalog."""
    armor = {}
    try:
        with open(catalog_path) as f:
            data = json.load(f)
            for item in data.get("items", []):
                armor[item["id"]] = item.get("base_armor", 0)
    except (json.JSONDecodeError, IOError, KeyError):
        pass
    return armor


def infer_armor(snap, base_armor_map):
    """Infer total armor from snapshot modules and base_armor."""
    ship_class = snap.get("ship_class", "unknown")
    base = base_armor_map.get(ship_class, 0)

    # Count Armor Plate I modules (each gives +5)
    modules = snap.get("modules", [])
    armor_plate_count = 0
    for mod in modules:
        mod_name = mod.get("name", "")
        if "Armor Plate I" in mod_name:
            armor_plate_count += 1

    total_armor = base + armor_plate_count * 5
    return total_armor, armor_plate_count, base


def flat_law(pre_hit, armor, is_kinetic=False):
    """Flat armor law: max(1, pre - floor(c)) where c = 0.75*armor or 1.5*armor."""
    if is_kinetic:
        c = 1.5 * armor
    else:
        c = 0.75 * armor
    return max(1, int(pre_hit - int(c)))


def saturating_law(pre_hit, armor, is_kinetic=False):
    """Saturating armor law: max(1, floor(pre * (1 - c/(c+150))))."""
    if is_kinetic:
        c = 1.5 * armor
    else:
        c = 0.75 * armor
    if c + 150 == 0:
        return max(1, int(pre_hit))
    multiplier = 1.0 - c / (c + 150)
    return max(1, int(pre_hit * multiplier))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-d", "--dir", default="data/battles/duels",
                        help="Directory of raw battle logs (default: data/battles/duels)")
    args = parser.parse_args()

    base_armor_map = load_ship_base_armor()

    # Collect hull-landing volleys
    volleys = []

    entry_count = 0
    for battle_id_prefix, entry in entries(args.dir):
        entry_count += 1
        battle_id = entry.get("battle_id", "unknown")
        snapshots = entry.get("snapshots", [])
        attacks = entry.get("attacks", [])

        if not attacks or not snapshots:
            continue

        # Build snap map
        snap_map = {}
        for snap in snapshots:
            player_id = snap.get("player_id")
            if player_id:
                snap_map[player_id] = snap

        # Process attacks
        for attack in attacks:
            target_id = attack.get("target_id")
            hull_damage = attack.get("hull_damage", 0)
            pre_hit_damage = attack.get("pre_hit_damage", 0)
            damage_type = attack.get("damage_type", "energy")

            # Only hull-landing volleys
            if hull_damage <= 0:
                continue

            target_snap = snap_map.get(target_id)
            if not target_snap:
                continue

            # Infer armor
            total_armor, plate_count, base = infer_armor(target_snap, base_armor_map)
            is_kinetic = damage_type == "kinetic"

            # Compute predictions
            flat_pred = flat_law(pre_hit_damage, total_armor, is_kinetic)
            sat_pred = saturating_law(pre_hit_damage, total_armor, is_kinetic)

            volleys.append({
                "battle_id": battle_id,
                "target_id": target_id,
                "ship_class": target_snap.get("ship_class", "unknown"),
                "total_armor": total_armor,
                "plate_count": plate_count,
                "base_armor": base,
                "damage_type": damage_type,
                "pre_hit": pre_hit_damage,
                "observed_hull": hull_damage,
                "flat_pred": flat_pred,
                "sat_pred": sat_pred
            })

    if entry_count == 0:
        print("No logs found in", args.dir)
        sys.exit(0)

    # Print volleys grouped by armor
    print("=== Hull-Landing Volleys ===\n")
    volleys_by_armor = defaultdict(list)
    for v in volleys:
        volleys_by_armor[v["total_armor"]].append(v)

    for armor in sorted(volleys_by_armor.keys()):
        print(f"\n--- Armor {armor} (base={volleys_by_armor[armor][0]['base_armor']}, "
              f"plates={volleys_by_armor[armor][0]['plate_count']}) ---")
        print("pre_hit | obs_hull | damage_type | flat_pred | sat_pred | flat_match | sat_match")
        for v in volleys_by_armor[armor][:30]:  # Print first 30 per armor
            flat_match = "✓" if v["flat_pred"] == v["observed_hull"] else "✗"
            sat_match = "✓" if v["sat_pred"] == v["observed_hull"] else "✗"
            print(f"{v['pre_hit']:6d} | {v['observed_hull']:8d} | {v['damage_type']:<11} | "
                  f"{v['flat_pred']:9d} | {v['sat_pred']:8d} | {flat_match:<10} | {sat_match}")

    # Summary: exact-match counts per law per armor
    print("\n=== Per-Law Exact-Match Counts ===\n")
    flat_matches_by_armor = defaultdict(int)
    sat_matches_by_armor = defaultdict(int)
    total_by_armor = defaultdict(int)

    for v in volleys:
        armor = v["total_armor"]
        total_by_armor[armor] += 1
        if v["flat_pred"] == v["observed_hull"]:
            flat_matches_by_armor[armor] += 1
        if v["sat_pred"] == v["observed_hull"]:
            sat_matches_by_armor[armor] += 1

    print("armor | total | flat_matches (%) | sat_matches (%)")
    for armor in sorted(total_by_armor.keys()):
        total = total_by_armor[armor]
        flat_m = flat_matches_by_armor[armor]
        sat_m = sat_matches_by_armor[armor]
        flat_pct = 100.0 * flat_m / total if total > 0 else 0
        sat_pct = 100.0 * sat_m / total if total > 0 else 0
        print(f"{armor:5d} | {total:5d} | {flat_m:4d} ({flat_pct:5.1f}%) | {sat_m:4d} ({sat_pct:5.1f}%)")


if __name__ == "__main__":
    main()
