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
    evade_observations = []  # (battle_id, tick, evader_id, hit_chance, fuel, evader_attacks)
    brace_timelines = defaultdict(lambda: defaultdict(list))  # battle_id -> braced_id -> [(tick, shield, max_shield)]
    regen_zero_timelines = defaultdict(lambda: defaultdict(list))  # battle_id -> target_id -> [(tick, shield)]

    # Track attacks per tick and battle
    attacks_by_battle_tick = defaultdict(lambda: defaultdict(list))  # battle_id -> tick -> [attacks]

    # Track server-reported regen per tick and battle (RegenLogEntry.shield_regen)
    regen_by_battle_tick = defaultdict(lambda: defaultdict(dict))  # battle_id -> tick -> player_id -> shield_regen

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

        # Track attacks for this tick
        if attacks:
            attacks_by_battle_tick[battle_id][tick] = attacks

        # Track server-reported shield regen for this tick, keyed by player_id
        # (RegenLogEntry: player_id, shield_regen, ...; matched into the brace
        # table below alongside the inferred-delta candidates).
        for r in (regen or []):
            player_id = r.get("player_id")
            if player_id:
                regen_by_battle_tick[battle_id][tick][player_id] = r.get("shield_regen")

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
            # S4: brace scenario - collect timeline data
            for snap in snapshots:
                stance = snap.get("stance")
                if stance == "brace":
                    braced_id = snap.get("player_id")
                    shield = snap.get("shield", 0)
                    max_shield = snap.get("max_shield", 1)

                    brace_timelines[battle_id][braced_id].append({
                        "tick": tick,
                        "shield": shield,
                        "max_shield": max_shield
                    })

        # S5 detection: collect timeline when shield transitions or stays at 0
        for snap in snapshots:
            shield_val = snap.get("shield", 0)
            target_id = snap.get("player_id")
            # Track all shield values for later regen-zero detection
            regen_zero_timelines[battle_id][target_id].append({
                "tick": tick,
                "shield": shield_val
            })

    if entry_count == 0:
        print("No logs found in", args.dir)
        sys.exit(0)

    # Print results
    print("=== Evade Scenario (S3) ===\n")
    if evade_observations:
        print("battle_id | tick | hit_chance | fuel | attacker_attacks")
        for obs in evade_observations[:20]:  # Print first 20
            hit_chance_str = f"{obs['hit_chance']:.2f}" if obs['hit_chance'] is not None else "N/A"
            print(f"{obs['battle_id']} | {obs['tick']} | "
                  f"{hit_chance_str} | "
                  f"{obs['fuel']} | {obs['evader_attacks']}")
    else:
        print("(No evade observations found)")

    # S4: Brace Scenario with shield delta and regen candidates
    print("\n=== Brace Scenario (S4) ===\n")
    if brace_timelines:
        for battle_id in sorted(brace_timelines.keys()):
            for braced_id in sorted(brace_timelines[battle_id].keys()):
                timeline = sorted(brace_timelines[battle_id][braced_id], key=lambda x: x["tick"])

                if not timeline:
                    continue

                print(f"Battle {battle_id}, braced player {braced_id}:")
                print("tick | shield | shield_delta | was_hit | regen_obs | candidates: recharge, 2*recharge, floor(2*recharge/3), 2*floor(recharge/3)")

                # Infer recharge from un-hit-tick deltas
                un_hit_deltas = []
                for i, obs in enumerate(timeline):
                    tick = obs["tick"]
                    # Check if this player was hit this tick
                    attacks_this_tick = attacks_by_battle_tick.get(battle_id, {}).get(tick, [])
                    was_hit = any(a.get("target_id") == braced_id for a in attacks_this_tick)

                    if not was_hit and i > 0:
                        delta = obs["shield"] - timeline[i-1]["shield"]
                        if delta > 0:
                            un_hit_deltas.append(delta)

                # Mode of positive deltas (most common un-hit regen)
                if un_hit_deltas:
                    recharge = max(set(un_hit_deltas), key=un_hit_deltas.count)
                else:
                    recharge = 0

                print(f"Inferred recharge: {recharge}")

                # Print per-tick data
                for i, obs in enumerate(timeline):
                    tick = obs["tick"]
                    shield = obs["shield"]
                    delta = shield - timeline[i-1]["shield"] if i > 0 else 0

                    # Check if hit this tick
                    attacks_this_tick = attacks_by_battle_tick.get(battle_id, {}).get(tick, [])
                    was_hit = any(a.get("target_id") == braced_id for a in attacks_this_tick)

                    # Regen candidates
                    if recharge > 0:
                        cand_recharge = recharge
                        cand_2x = 2 * recharge
                        cand_floor_2div3 = int(2 * recharge / 3)
                        cand_2floor_div3 = 2 * int(recharge / 3)
                    else:
                        cand_recharge = cand_2x = cand_floor_2div3 = cand_2floor_div3 = 0

                    # Mark which candidate matches
                    matches = []
                    if delta == cand_recharge:
                        matches.append("recharge")
                    if delta == cand_2x:
                        matches.append("2*recharge")
                    if delta == cand_floor_2div3:
                        matches.append("floor(2*recharge/3)")
                    if delta == cand_2floor_div3:
                        matches.append("2*floor(recharge/3)")

                    match_str = ",".join(matches) if matches else "—"
                    hit_str = "✓" if was_hit else "—"

                    regen_obs = regen_by_battle_tick.get(battle_id, {}).get(tick, {}).get(braced_id)
                    regen_obs_str = regen_obs if regen_obs is not None else "N/A"

                    print(f"{tick} | {shield} | {delta:+3d} | {hit_str} | {regen_obs_str} | "
                          f"{cand_recharge}, {cand_2x}, {cand_floor_2div3}, {cand_2floor_div3} [{match_str}]")
                print()
    else:
        print("(No brace observations found)")

    # S5: Regen-Zero Scenario with timeline
    print("\n=== Regen-Zero Scenario (S5) ===\n")
    found_zero = False
    for battle_id in sorted(regen_zero_timelines.keys()):
        for target_id in sorted(regen_zero_timelines[battle_id].keys()):
            timeline = sorted(regen_zero_timelines[battle_id][target_id], key=lambda x: x["tick"])

            # Find first tick where shield is 0
            zero_idx = None
            for i, obs in enumerate(timeline):
                if obs["shield"] == 0:
                    zero_idx = i
                    break

            if zero_idx is not None:
                found_zero = True
                print(f"Battle {battle_id}, target {target_id} (shield hits 0 at tick {timeline[zero_idx]['tick']}):")
                print("tick | shield | was_hit")

                # Print from zero tick onwards, up to 10+ ticks or end of log
                for i in range(zero_idx, min(zero_idx + 12, len(timeline))):
                    obs = timeline[i]
                    tick = obs["tick"]
                    shield = obs["shield"]

                    # Check if hit this tick
                    attacks_this_tick = attacks_by_battle_tick.get(battle_id, {}).get(tick, [])
                    was_hit = any(a.get("target_id") == target_id for a in attacks_this_tick)
                    hit_str = "✓" if was_hit else "—"

                    print(f"{tick} | {shield} | {hit_str}")
                print()

    if not found_zero:
        print("(No regen-zero observations found)")


if __name__ == "__main__":
    main()
