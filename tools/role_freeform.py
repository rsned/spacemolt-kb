#!/usr/bin/env python3
"""Open-vocabulary role labeler for taxonomy discovery.

Unlike classify_archetypes.py (which forces each bio into one of 12 fixed
archetypes), this asks the model for the passenger's actual primary role as a
free noun phrase. Aggregating these labels reveals roles the fixed taxonomy
either lacks entirely or forces into a poor-fitting bucket.

This is an ANALYSIS tool: it writes to a separate cache and never touches the
canonical overlays/generated/archetypes.json.

Run from the kb repo root (Ollama must be up on localhost:11434):
    python3 tools/role_freeform.py [knowledge.db] [--sample N] [--model M]
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
CACHE = "overlays/generated/role_freeform.json"

# Set SMKB_NUM_GPU=0 to force CPU inference (used when the GPU runner is broken).
_NUM_GPU = os.environ.get("SMKB_NUM_GPU")

PROMPT = """You read a science-fiction passenger biography and output their single \
primary occupation or social role as a short lowercase noun phrase of 1-3 words.
Be specific and literal to the bio. Examples of the FORM expected: "refinery worker", \
"smuggler", "customs inspector", "stage musician", "war correspondent", "bounty hunter", \
"galley cook", "xenobiologist", "courier pilot", "cult acolyte".
Output ONLY the phrase, nothing else. No explanation, no punctuation.

Biography:
{bio}

Primary role:"""


def label(bio, model):
    # small context: bios are 1-3 sentences, so 2k covers prompt+bio and
    # keeps the KV cache tiny; temperature 0 for deterministic labels.
    options = {"temperature": 0, "num_predict": 12, "num_ctx": 2048}
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
    # first line, lowercased, stripped of surrounding punctuation/quotes
    phrase = (out or "").strip().splitlines()[0].strip() if out.strip() else ""
    return phrase.lower().strip(".,:;\"'`*-•» ")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("db", nargs="?", default="spacemolt-knowledge.db")
    ap.add_argument("--sample", type=int, default=0, help="label only N random bios; print, don't cache")
    ap.add_argument("--model", default=DEFAULT_MODEL)
    args = ap.parse_args()

    con = sqlite3.connect(args.db)
    q = "SELECT citizen_id, bio FROM passengers"
    if args.sample:
        q += f" ORDER BY RANDOM() LIMIT {args.sample}"
    rows = con.execute(q).fetchall()
    con.close()

    if args.sample:
        for cid, bio in rows:
            r = label(bio, args.model)
            print(f"  {r:24} {cid}: {(bio or '')[:80]}")
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

    requeried = 0
    for i, (cid, bio) in enumerate(rows, 1):
        sha = hashlib.sha256((bio or "").encode()).hexdigest()
        cur = cache.get(cid)
        if not (cur and cur.get("bio_sha256") == sha):
            r = label(bio, args.model)
            cache[cid] = {"role": r, "bio_sha256": sha}
            requeried += 1
        if i % 25 == 0:
            flush()  # checkpoint so an Ollama crash never loses queried work
            print(f"  {i}/{len(rows)} ({requeried} queried)", file=sys.stderr)

    flush()
    print(f"labeled {len(rows)} passengers ({requeried} newly queried) -> {CACHE}")


if __name__ == "__main__":
    main()
