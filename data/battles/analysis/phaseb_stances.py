#!/usr/bin/env python3
"""
Measure stance effects: evade (-0.20 hit, 0.5x damage, 5 fuel/tick), brace (2x regen, 0.25x damage), regen_from_zero.

Scenarios S3 (evade), S4 (brace), S5 (regen_from_zero).
Detects scenarios by observed stances in snapshots.
Prints raw observations: evader fuel per tick, damage ratio, attacker hit_chance delta.
For brace: per-tick shield delta vs regen candidates.
For regen-zero: shield timeline after it hits zero.
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


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("-d", "--dir", default="data/battles/duels",
                        help="Directory of raw battle logs (default: data/battles/duels)")
    args = parser.parse_args()

    # Collect observations by scenario type
    evade_observations = []  # (battle_id, tick, evader_id, hit_chance, hit_delta, damage_ratio, fuel_per_tick, attacker_attacks)
    brace_observations = []  # (battle_id, tick, braced_id, shield, max_shield, shield_delta, regen_observed)
    regen_zero_observations = []  # (battle_id, tick_from_zero, target_id, shield_value)

    entry_count = 0
    for battle_id_prefix, entry in entries(args.dir):
        entry_count += 1
        battle_id = entry.get("battle_id", "unknown")
        tick = entry.get("tick", -1)
        snapshots = entry.get("snapshots", [])
        attacks = entry.get("attacks", [])
        regen = entry.get("regen", [])

        if not snapshots:
            continue

        # Build snap map
        snap_map = {}
        for snap in snapshots:
            player_id = snap.get("player_id")
            if player_id:
                snap_map[player_id] = snap

        # Detect scenario by stances
        has_evade = any(s.get("stance") == "evade" for s in snapshots)
        has_brace = any(s.get("stance") == "brace" for s in snapshots)

        if has_evade:
            # S3: evade scenario
            for snap in snapshots:
                stance = snap.get("stance")
                if stance == "evade":
                    evader_id = snap.get("player_id")
                    evader_fuel = snap.get("fuel", 0)
                    evader_max_fuel = snap.get("max_fuel", 1)

                    # Count attacks on evader in this tick
                    evader_attacks_count = sum(1 for a in (attacks or [])
                                               if a.get("attacker_id") == evader_id)

                    # Get attacker snap
                    target_id = snap.get("target_id")
                    if target_id:
                        attacker_snap = snap_map.get(target_id)
                        if attacker_snap:
                            # Get hit_chance from attacker's attack on evader
                            hit_chance = None
                            for a in (attacks or []):
                                if (a.get("attacker_id") == target_id and
                                        a.get("target_id") == evader_id):
                                    hit_chance = a.get("hit_chance")
                                    break

                            evade_observations.append({
                                "battle_id": battle_id,
                                "tick": tick,
                                "evader_id": evader_id,
                                "hit_chance": hit_chance,
                                "fuel": evader_fuel,
                                "evader_attacks": evader_attacks_count
                            })

        elif has_brace:
            # S4: brace scenario
            for snap in snapshots:
                stance = snap.get("stance")
                if stance == "brace":
                    braced_id = snap.get("player_id")
                    shield = snap.get("shield", 0)
                    max_shield = snap.get("max_shield", 1)

                    brace_observations.append({
                        "battle_id": battle_id,
                        "tick": tick,
                        "braced_id": braced_id,
                        "shield": shield,
                        "max_shield": max_shield
                    })

            # Collect regen data
            if regen:
                for reg in regen:
                    if reg.get("player_id") in snap_map:
                        snap = snap_map[reg.get("player_id")]
                        if snap.get("stance") == "brace":
                            regen_value = reg.get("amount", 0)
                            print(f"Brace regen observation: battle={battle_id}, tick={tick}, "
                                  f"regen={regen_value}, shield={snap.get('shield')}/{snap.get('max_shield')}")

        # S5 detection: shield at 0 and braced attacker
        for snap in snapshots:
            if snap.get("shield", 0) == 0 and snap.get("stance") == "brace":
                # This might be regen_from_zero scenario
                target_id = snap.get("player_id")
                regen_zero_observations.append({
                    "battle_id": battle_id,
                    "tick": tick,
                    "target_id": target_id,
                    "shield": 0
                })

    if entry_count == 0:
        print("No logs found in", args.dir)
        sys.exit(0)

    # Print results
    print("=== Evade Scenario (S3) ===\n")
    if evade_observations:
        print("battle_id | tick | hit_chance | fuel | attacker_attacks")
        for obs in evade_observations[:20]:  # Print first 20
            print(f"{obs['battle_id']} | {obs['tick']} | "
                  f"{obs['hit_chance']:.2f if obs['hit_chance'] else 'N/A'} | "
                  f"{obs['fuel']} | {obs['evader_attacks']}")
    else:
        print("(No evade observations found)")

    print("\n=== Brace Scenario (S4) ===\n")
    if brace_observations:
        print("battle_id | tick | shield | max_shield")
        for obs in brace_observations[:20]:
            print(f"{obs['battle_id']} | {obs['tick']} | {obs['shield']} | {obs['max_shield']}")
    else:
        print("(No brace observations found)")

    print("\n=== Regen-Zero Scenario (S5) ===\n")
    if regen_zero_observations:
        print(f"Found {len(regen_zero_observations)} shield-at-zero ticks")
    else:
        print("(No regen-zero observations found)")


if __name__ == "__main__":
    main()
