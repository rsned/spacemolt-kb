#!/usr/bin/env python3
"""Aggregate wildlife battle records from the SpaceMolt bulk data feed.

The feed (https://spacemolt.com/data) publishes every completed battle as a
summary row in gzipped NDJSON, one file per month on a stable URL:

    https://assets.spacemolt.com/public/v1/battles/YYYY-MM.ndjson.gz

This script reads one or more of those shards (local paths) and writes
data/wildlife/battle_stats.json: per species, how many wildlife-category
battles it appeared in and how many the wildlife side won outright.
Stalemates and mutual destructions are draws and count for neither side.

A creature is any participant whose name is not in the row's player_names.
Species display names match wildlife_species.name in the knowledge DB.

Usage:
    python3 scripts/wildlife_battle_stats.py shard1.ndjson.gz [shard2 ...] \
        [-o data/wildlife/battle_stats.json]
"""

import argparse
import collections
import gzip
import json
import re
import sys


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("shards", nargs="+", help="monthly battles .ndjson.gz files")
    ap.add_argument("-o", "--out", default="data/wildlife/battle_stats.json")
    ap.add_argument(
        "--since",
        default="",
        help="only count battles with ended_at >= this ISO timestamp "
        "(e.g. 2026-08-28T00:00:00Z, the first full day after the "
        "v0.566.0 balance patch)",
    )
    args = ap.parse_args()

    wins: collections.Counter[str] = collections.Counter()
    total: collections.Counter[str] = collections.Counter()
    months: set[str] = set()

    for path in args.shards:
        m = re.search(r"(\d{4}-\d{2})", path)
        if m:
            months.add(m.group(1))
        with gzip.open(path, "rt") as fh:
            for line in fh:
                b = json.loads(line)
                if b.get("category") != "wildlife":
                    continue
                if args.since and b.get("ended_at", "") < args.since:
                    continue
                players = set(b.get("player_names") or [])
                creature_sides = {}
                for side in b["sides"]:
                    creatures = [p for p in side["participants"] if p not in players]
                    if creatures:
                        creature_sides[side["side_id"]] = creatures
                if not creature_sides:
                    continue
                for sp in {sp for crs in creature_sides.values() for sp in crs}:
                    total[sp] += 1
                if b.get("outcome") == "victory" and b.get("winning_side") in creature_sides:
                    for sp in set(creature_sides[b["winning_side"]]):
                        wins[sp] += 1

    out = {
        "since": args.since,
        "source": "https://assets.spacemolt.com/public/v1/battles/ (bulk data feed)",
        "months": sorted(months),
        "note": "wildlife_wins = outcome 'victory' with winning_side the creature's "
        "side; stalemate/mutual_destruction are draws and count for neither",
        "species": {
            sp: {"battles": total[sp], "wildlife_wins": wins[sp]}
            for sp in sorted(total)
        },
    }
    with open(args.out, "w") as fh:
        json.dump(out, fh, indent=1, sort_keys=True)
        fh.write("\n")
    grand = sum(total.values())
    grand_w = sum(wins.values())
    print(
        f"wrote {args.out}: {len(total)} species, {grand:,} battles, "
        f"{grand_w:,} wildlife wins ({100 * grand_w / grand:.1f}%)",
        file=sys.stderr,
    )


if __name__ == "__main__":
    main()
