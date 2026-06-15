#!/usr/bin/env python3
"""Classify each passenger bio into one high-level role archetype via local Ollama.

The archetype drives garment/role styling in the portrait prompt so that, e.g.,
not every crimson citizen reads as a dress-uniform officer — a crimson refinery
supervisor, farmer, or trade clerk gets role-appropriate clothing while keeping
the empire's palette/cut sensibility.

Results are cached to overlays/generated/archetypes.json keyed by citizen_id,
with the bio's SHA-256 so a passenger is only re-queried when its bio changes.
The Go KB generator reads this file at portrait-build time.

Run from the kb repo root (Ollama must be up on localhost:11434):
    python3 tools/classify_archetypes.py [knowledge.db] [--sample N] [--model M]
"""

import argparse
import hashlib
import json
import os
import sqlite3
import sys
import urllib.request

OLLAMA_URL = "http://localhost:11434/api/generate"
DEFAULT_MODEL = "gemma4:latest"
CACHE = "overlays/generated/archetypes.json"

# Fixed taxonomy. Keep in sync with archetypeAesthetic in cmd/generate-factions-kb/prompt.go.
ARCHETYPES = [
    "laborer",
    "officer",
    "merchant",
    "official",
    "technician",
    "pilot",
    "medic",
    "outlaw",
    "spiritual",
    "aristocrat",
    "performer",
    "spacer",
]

PROMPT = """You classify a science-fiction passenger into exactly ONE role archetype from their biography.
Respond with ONLY the single lowercase archetype keyword and nothing else.

Archetypes:
- laborer: manual or industrial worker (miner, refinery hand, hydroponic farmer, dock worker, machinist)
- officer: military or security commander; soldier, marine, armored officer, guard, enforcer
- merchant: trader, broker, financier, dealer, market or sales representative, negotiator
- official: bureaucrat, inspector, regulator, customs/logistics/administrative agent, clerk, notary, diplomat
- technician: engineer, mechanic, scientist, tinkerer, restorer, specialist craftsperson
- pilot: ship pilot, navigator, captain, freighter or shuttle operator
- medic: doctor, physician, nurse, field medic, surgeon
- outlaw: raider, pirate, smuggler, fugitive, black-market or ransom operator
- spiritual: mystic, monk, priest, ascetic, void-cultist, prophet
- aristocrat: owner, magnate, noble, executive, wealthy patron or heir
- performer: entertainer, stage musician, celebrity, showman, idol
- spacer: a generic traveler that fits none of the above

Biography:
{bio}

Archetype:"""


def classify(bio, model):
    body = json.dumps(
        {
            "model": model,
            "prompt": PROMPT.format(bio=(bio or "").strip()),
            "stream": False,
            "options": {"temperature": 0, "num_predict": 8},
        }
    ).encode()
    req = urllib.request.Request(OLLAMA_URL, data=body, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as resp:
        out = json.loads(resp.read())["response"]
    word = out.strip().lower().split()[0].strip(".,:;\"'`*-") if out.strip() else ""
    return word if word in ARCHETYPES else "spacer"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("db", nargs="?", default="/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db")
    ap.add_argument("--sample", type=int, default=0, help="classify only N random bios; print, don't cache")
    ap.add_argument("--model", default=DEFAULT_MODEL)
    args = ap.parse_args()

    con = sqlite3.connect(args.db)
    q = "SELECT citizen_id, bio FROM passengers"
    if args.sample:
        q += f" ORDER BY RANDOM() LIMIT {args.sample}"
    rows = con.execute(q).fetchall()
    con.close()

    if args.sample:
        counts = {}
        for cid, bio in rows:
            a = classify(bio, args.model)
            counts[a] = counts.get(a, 0) + 1
            print(f"  {a:11} {cid}: {(bio or '')[:90]}")
        print("\ndistribution:", dict(sorted(counts.items(), key=lambda kv: -kv[1])))
        return

    cache = {}
    if os.path.exists(CACHE):
        with open(CACHE) as f:
            cache = json.load(f)

    counts = {}
    requeried = 0
    for i, (cid, bio) in enumerate(rows, 1):
        sha = hashlib.sha256((bio or "").encode()).hexdigest()
        cur = cache.get(cid)
        if not (cur and cur.get("bio_sha256") == sha):
            a = classify(bio, args.model)
            cache[cid] = {"archetype": a, "bio_sha256": sha}
            requeried += 1
        counts[cache[cid]["archetype"]] = counts.get(cache[cid]["archetype"], 0) + 1
        if i % 25 == 0:
            print(f"  {i}/{len(rows)} ({requeried} queried)", file=sys.stderr)

    os.makedirs(os.path.dirname(CACHE), exist_ok=True)
    with open(CACHE, "w") as f:
        json.dump(cache, f, indent=2, sort_keys=True)
    print(f"classified {len(rows)} passengers ({requeried} newly queried) -> {CACHE}")
    print("distribution:", dict(sorted(counts.items(), key=lambda kv: -kv[1])))


if __name__ == "__main__":
    main()
