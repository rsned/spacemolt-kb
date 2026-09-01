#!/usr/bin/env python3
"""Harvest per-species combat stats from exported wildlife battle logs.

Reads every *.raw.json in data/battles/wildlife/ (raw get_battle_log pages
from bin/battle-export --out-dir) and aggregates, per creature species:

  - natural weapon(s): damage type, base damage range, shots observed
  - observed max_hull / max_shield ranges across individuals
  - the hit-chance envelope of the creature's own attacks

Creatures are snapshots with kind == "creature"; their username is the
species display name and their id carries a crt_ prefix. base_damage is
the weapon's pre-crit, pre-skill catalog value, so it is stable across
balance patches that only touch modules.

Usage:
    python3 scripts/wildlife_combat_stats.py [-d data/battles/wildlife] \
        [-o data/wildlife/combat_stats.json]
"""

import argparse
import glob
import json
import os
import sys


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("-d", "--dir", default="data/battles/wildlife")
    ap.add_argument("-o", "--out", default="data/wildlife/combat_stats.json")
    args = ap.parse_args()

    species: dict[str, dict] = {}

    def rec(name: str) -> dict:
        return species.setdefault(
            name,
            {
                "battles": 0,
                "hull_min": None, "hull_max": None,
                "shield_min": None, "shield_max": None,
                "hit_min": None, "hit_max": None,
                "weapons": {},
            },
        )

    def span(d: dict, key: str, v) -> None:
        lo, hi = d[key + "_min"], d[key + "_max"]
        d[key + "_min"] = v if lo is None else min(lo, v)
        d[key + "_max"] = v if hi is None else max(hi, v)

    files = sorted(glob.glob(os.path.join(args.dir, "*.raw.json")))
    for path in files:
        pages = json.load(open(path))
        creatures = {}  # id -> species name
        seen_here = set()
        for page in pages:
            for e in page.get("entries") or []:
                for sn in e.get("snapshots") or []:
                    if sn.get("kind") == "creature":
                        creatures[sn["player_id"]] = sn["username"]
                        r = rec(sn["username"])
                        span(r, "hull", sn.get("max_hull", 0))
                        span(r, "shield", sn.get("max_shield", 0))
                        seen_here.add(sn["username"])
                for a in e.get("attacks") or []:
                    name = creatures.get(a.get("attacker_id"))
                    if name is None:
                        continue
                    r = rec(name)
                    if "hit_chance" in a:
                        span(r, "hit", round(a["hit_chance"], 4))
                    for w in a.get("weapons") or []:
                        wr = r["weapons"].setdefault(
                            w["name"],
                            {
                                "damage_type": w.get("damage_type", ""),
                                "base_min": None, "base_max": None,
                                "shots": 0,
                            },
                        )
                        span(wr, "base", w.get("base_damage", 0))
                        wr["shots"] += 1
        for name in seen_here:
            species[name]["battles"] += 1

    out = {
        "source": f"{len(files)} exported battle logs in {args.dir}",
        "species": dict(sorted(species.items())),
    }
    with open(args.out, "w") as fh:
        json.dump(out, fh, indent=1, sort_keys=True)
        fh.write("\n")
    armed = sum(1 for r in species.values() if r["weapons"])
    print(
        f"wrote {args.out}: {len(species)} species from {len(files)} battles; "
        f"{armed} with observed attacks",
        file=sys.stderr,
    )


if __name__ == "__main__":
    main()
