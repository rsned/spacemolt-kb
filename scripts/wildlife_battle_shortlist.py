#!/usr/bin/env python3
"""Pick per-species battle ids to export for wildlife stat harvesting.

Reads monthly battle shards from the bulk data feed (local .ndjson.gz
paths) and, for each wildlife species, shortlists up to two battles the
players won and up to two the wildlife won — the contrast shows the
creature both dealing and receiving damage. Battles are ranked to favor:

  1. clean 1v1s (participant_count == 2) with a single species,
  2. short fights (2..20 ticks; at least 2 so multiple volleys land),
  3. recent fights (newer server versions log more fields).

Output (default data/wildlife/battle_shortlist.json) maps species name to
{"player_win": [...], "wildlife_win": [...]}, each entry carrying
battle_id, system, duration_ticks, participant_count, and ended_at.

Usage:
    python3 scripts/wildlife_battle_shortlist.py shard1.ndjson.gz [...] \
        [-o data/wildlife/battle_shortlist.json] [--per-side 2]
"""

import argparse
import collections
import gzip
import json
import sys


def rank(b: dict) -> tuple:
    d = b["duration_ticks"]
    return (
        b["participant_count"] != 2,  # 1v1 first
        not (2 <= d <= 20),  # then short-but-not-instant
        d,  # then shortest
        b["ended_at"],  # newest last...
    )


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("shards", nargs="+")
    ap.add_argument("-o", "--out", default="data/wildlife/battle_shortlist.json")
    ap.add_argument("--per-side", type=int, default=2)
    ap.add_argument(
        "--since",
        default="",
        help="only consider battles with ended_at >= this ISO timestamp",
    )
    args = ap.parse_args()

    candidates: dict[str, dict[str, list]] = collections.defaultdict(
        lambda: {"player_win": [], "wildlife_win": []}
    )
    for path in args.shards:
        with gzip.open(path, "rt") as fh:
            for line in fh:
                b = json.loads(line)
                if b.get("category") != "wildlife" or b.get("outcome") != "victory":
                    continue
                if args.since and b.get("ended_at", "") < args.since:
                    continue
                players = set(b.get("player_names") or [])
                creature_sides = {}
                for side in b["sides"]:
                    creatures = [p for p in side["participants"] if p not in players]
                    if creatures:
                        creature_sides[side["side_id"]] = creatures
                species = {sp for crs in creature_sides.values() for sp in crs}
                if len(species) != 1:  # single-species battles only
                    continue
                sp = species.pop()
                bucket = (
                    "wildlife_win"
                    if b.get("winning_side") in creature_sides
                    else "player_win"
                )
                candidates[sp][bucket].append(
                    {
                        "battle_id": b["battle_id"],
                        "system": b.get("system_id", ""),
                        "duration_ticks": b.get("duration_ticks", 0),
                        "participant_count": b.get("participant_count", 0),
                        "ended_at": b.get("ended_at", ""),
                    }
                )

    out = {}
    n_battles = 0
    for sp in sorted(candidates):
        picks = {}
        for bucket, rows in candidates[sp].items():
            # Rank, then prefer the most recent among equally-ranked picks.
            rows.sort(key=lambda b: (rank(b)[:3], b["ended_at"]))
            best = sorted(
                rows[: args.per_side * 4], key=rank
            )[: args.per_side]
            picks[bucket] = best
            n_battles += len(best)
        out[sp] = picks

    with open(args.out, "w") as fh:
        json.dump(out, fh, indent=1, sort_keys=True)
        fh.write("\n")
    both = sum(1 for p in out.values() if p["player_win"] and p["wildlife_win"])
    print(
        f"wrote {args.out}: {len(out)} species, {n_battles} battles shortlisted "
        f"({both} species have both a player win and a wildlife win)",
        file=sys.stderr,
    )


if __name__ == "__main__":
    main()
