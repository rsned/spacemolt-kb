#!/usr/bin/env python3
"""Aggregate open-vocab role labels vs the fixed 12-way archetype assignments to
find taxonomy gaps.

Reads:
  overlays/generated/role_freeform.json   (citizen_id -> {role})         [open vocab]
  overlays/generated/archetypes.json      (citizen_id -> {archetype})    [fixed 12]
  spacemolt-knowledge.db                   passengers(citizen_id, bio)

Emits a markdown report on stdout:
  - distinct role phrases by frequency, each annotated with the 12-way bucket it
    was forced into (reveals forced fits and the spacer pile)
  - a normalized keyword-group rollup highlighting roles with no clean home

Run after role_freeform.py has populated its cache:
    python3 tools/analyze_roles.py [knowledge.db] > /tmp/role_report.md
"""

import json
import os
import re
import sqlite3
import sys
from collections import Counter, defaultdict

ROLE_CACHE = "overlays/generated/role_freeform.json"
ARCH_CACHE = "overlays/generated/archetypes.json"

# The 12 fixed archetypes, with keyword cues used ONLY to flag roles that look
# like they have no clean home in the current taxonomy (a heuristic prompt for
# human review, not an authoritative mapping).
COVERED = {
    "laborer": "miner refinery farm farmer dock machinist factory foundry hauler welder rigger labor worker hand",
    "officer": "officer soldier marine guard enforcer commander sergeant captain(mil) security marshal trooper",
    "merchant": "trader merchant broker dealer financier banker vendor salesman negotiator trade",
    "official": "bureaucrat inspector regulator customs clerk notary diplomat administrator official agent auditor magistrate",
    "technician": "engineer mechanic technician scientist researcher tinkerer restorer specialist analyst programmer",
    "pilot": "pilot navigator captain freighter shuttle courier flyer aviator",
    "medic": "doctor physician nurse medic surgeon paramedic orderly",
    "outlaw": "raider pirate smuggler fugitive bandit thief assassin mercenary bounty",
    "spiritual": "mystic monk priest ascetic cultist prophet acolyte shaman preacher",
    "aristocrat": "magnate noble executive heir patron aristocrat baron tycoon",
    "performer": "entertainer musician singer celebrity showman idol actor dancer artist performer",
    "spacer": "traveler spacer drifter wanderer passenger nomad",
}


def keyword_home(role):
    """Return set of archetypes whose cues appear in the role phrase."""
    homes = set()
    words = set(re.findall(r"[a-z]+", role.lower()))
    for arch, cues in COVERED.items():
        cueset = set(re.findall(r"[a-z]+", cues))
        if words & cueset:
            homes.add(arch)
    return homes


def main():
    db = sys.argv[1] if len(sys.argv) > 1 else "spacemolt-knowledge.db"
    if not os.path.exists(ROLE_CACHE):
        sys.exit(f"no role cache yet at {ROLE_CACHE}")
    roles = {k: v["role"] for k, v in json.load(open(ROLE_CACHE)).items()}
    arch = {}
    if os.path.exists(ARCH_CACHE):
        arch = {k: v["archetype"] for k, v in json.load(open(ARCH_CACHE)).items()}
    con = sqlite3.connect(db)
    bios = dict(con.execute("SELECT citizen_id, bio FROM passengers").fetchall())
    con.close()

    n = len(roles)
    role_count = Counter(roles.values())
    # role -> Counter of 12-way archetypes it was assigned
    role_to_arch = defaultdict(Counter)
    for cid, r in roles.items():
        role_to_arch[r][arch.get(cid, "?")] += 1

    print(f"# Open-vocabulary role analysis ({n} labeled passengers)\n")
    print(f"Distinct role phrases: **{len(role_count)}**\n")

    # --- roles with no obvious keyword home ---
    uncovered = Counter()
    uncovered_examples = defaultdict(list)
    for r, c in role_count.items():
        if not keyword_home(r):
            uncovered[r] = c
            ex = next((cid for cid, rr in roles.items() if rr == r), None)
            if ex:
                uncovered_examples[r] = (bios.get(ex, "") or "")[:110]
    print(f"## Roles with NO clean keyword home in the 12 ({sum(uncovered.values())} passengers, {len(uncovered)} phrases)\n")
    print("| count | role phrase | forced into (12-way) | example bio |")
    print("|------:|-------------|----------------------|-------------|")
    for r, c in uncovered.most_common(60):
        forced = ", ".join(f"{a}:{n}" for a, n in role_to_arch[r].most_common())
        print(f"| {c} | {r} | {forced} | {uncovered_examples[r]} |")

    # --- top roles overall with their forced bucket ---
    print(f"\n## Top 50 role phrases overall (with 12-way bucket)\n")
    print("| count | role phrase | 12-way bucket(s) |")
    print("|------:|-------------|------------------|")
    for r, c in role_count.most_common(50):
        forced = ", ".join(f"{a}:{n}" for a, n in role_to_arch[r].most_common())
        print(f"| {c} | {r} | {forced} |")

    # --- what landed in spacer (explicit misfits) ---
    spacer_roles = Counter()
    for cid, a in arch.items():
        if a == "spacer" and cid in roles:
            spacer_roles[roles[cid]] += 1
    print(f"\n## What the 12-way classifier dumped into `spacer` ({sum(spacer_roles.values())} passengers)\n")
    print("| count | open-vocab role |")
    print("|------:|-----------------|")
    for r, c in spacer_roles.most_common(40):
        print(f"| {c} | {r} |")


if __name__ == "__main__":
    main()
