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
import http.client
import json
import os
import sqlite3
import sys
import time
import urllib.error
import urllib.request

OLLAMA_URL = "http://localhost:11434/api/generate"
DEFAULT_MODEL = "gemma4:latest"
CACHE = "overlays/generated/archetypes.json"

# Set SMKB_NUM_GPU=0 to force CPU inference (used when the GPU runner is broken).
# Unset -> let Ollama decide (normal GPU path).
_NUM_GPU = os.environ.get("SMKB_NUM_GPU")

# Fixed taxonomy. Keep in sync with archetypeGarment in cmd/generate-factions-kb/prompt.go.
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
    "logistician",
    "engineer",
    "scientist",
    "retailer",
    "cook",
    "educator",
    "sanitation",
    "jurist",
    "journalist",
    "diplomat",
    "spacer",
]

PROMPT = """You classify a science-fiction passenger into exactly ONE role archetype from their biography.
Respond with ONLY the single lowercase archetype keyword and nothing else.

Archetypes:
- laborer: heavy manual or industrial worker (miner, refinery hand, hydroponic farmer, foundry or assembly worker)
- officer: military or security commander; soldier, marine, armored officer, guard, enforcer
- merchant: trader, broker, financier, dealer, wholesale or market negotiator
- official: bureaucrat, inspector, regulator, customs or compliance agent, administrative or records clerk, notary
- technician: mechanic, repair or maintenance worker, machinist, tinkerer, restorer, hands-on systems operator
- pilot: ship pilot, navigator, captain, freighter or shuttle operator
- medic: doctor, physician, nurse, field medic, surgeon
- outlaw: raider, pirate, smuggler, fugitive, black-market or ransom operator
- spiritual: mystic, monk, priest, ascetic, void-cultist, prophet
- aristocrat: owner, magnate, noble, executive, wealthy patron or heir
- performer: entertainer, stage musician, celebrity, showman, idol
- logistician: freight, cargo, and shipping coordination; cargo handler, freight or logistics coordinator, supply-chain planner, dispatcher, courier organizer
- engineer: professional engineer, architect, structural or ship surveyor, design consultant, fabricator, metalsmith
- scientist: researcher, scientist, scholar, academic, archivist, analyst (astrophysicist, xenobiologist, geologist, historian)
- retailer: shopkeeper, retail or commissary clerk, vendor, stock and inventory worker, salesfloor staff
- cook: galley cook, chef, food-service or hospitality worker, caterer, bartender, mess steward
- educator: teacher, tutor, instructor, professor, lecturer, education coordinator
- sanitation: sanitation, recycling, or waste-systems worker, janitorial or hygiene crew
- jurist: lawyer, legal counsel, contract arbitrator, insurance assessor or adjuster, claims agent
- journalist: correspondent, reporter, journalist, writer, author, press or news worker
- diplomat: diplomat, envoy, liaison, mediator, ambassador, attache, outreach or community-relations worker
- spacer: a generic traveler that fits none of the above

Biography:
{bio}

Archetype:"""


def classify(bio, model):
    options = {"temperature": 0, "num_predict": 8, "num_ctx": 2048}
    if _NUM_GPU is not None:
        options["num_gpu"] = int(_NUM_GPU)
    body = json.dumps(
        {
            "model": model,
            "prompt": PROMPT.format(bio=(bio or "").strip()),
            "stream": False,
            "keep_alive": "10m",
            "options": options,
        }
    ).encode()
    req = urllib.request.Request(OLLAMA_URL, data=body, headers={"Content-Type": "application/json"})
    out = ""
    for attempt in range(5):
        try:
            with urllib.request.urlopen(req, timeout=120) as resp:
                out = json.loads(resp.read())["response"]
            break
        except (urllib.error.URLError, http.client.HTTPException, ConnectionError, OSError, TimeoutError) as e:
            if attempt == 4:
                raise
            print(f"  ollama transient error ({e}); retry {attempt + 1}/4 in 5s", file=sys.stderr)
            time.sleep(5)
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

    os.makedirs(os.path.dirname(CACHE), exist_ok=True)

    def flush():
        tmp = CACHE + ".tmp"
        with open(tmp, "w") as f:
            json.dump(cache, f, indent=2, sort_keys=True)
        os.replace(tmp, CACHE)

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
            flush()  # checkpoint so an Ollama crash never loses queried work
            print(f"  {i}/{len(rows)} ({requeried} queried)", file=sys.stderr)

    flush()
    print(f"classified {len(rows)} passengers ({requeried} newly queried) -> {CACHE}")
    print("distribution:", dict(sorted(counts.items(), key=lambda kv: -kv[1])))


if __name__ == "__main__":
    main()
