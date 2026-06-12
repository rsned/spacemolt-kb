#!/usr/bin/env python3
"""Temporary contact-sheet of the generated agent portraits (the user's own bot
fleet, sighted-player subset).

Resolves agent_id -> server player_id from daily-summary.db (status_raw.player.id
in the latest snapshot), reads each agent's personality.json for name/role/empire,
and shows the generated portrait under overlays/generated/players/<player_id>/.
Each card shows the prompt (from the prompt.txt sidecar) on hover/expand.

Run from the kb repo root:
    python3 tools/build_agent_gallery.py
Defaults:
    agents dir     ../spacemolt/data/agents
    daily-summary  ../spacemolt/data/daily-summary.db
    out            agent-portrait-gallery.html
"""

import html
import json
import os
import sqlite3
import sys

AGENTS = sys.argv[1] if len(sys.argv) > 1 else "../spacemolt/data/agents"
DAILY = sys.argv[2] if len(sys.argv) > 2 else "../spacemolt/data/daily-summary.db"
OUT = sys.argv[3] if len(sys.argv) > 3 else "agent-portrait-gallery.html"
CACHE = "overlays/generated/players"

# agent_id -> player_id from the latest snapshot.
con = sqlite3.connect(DAILY)
idmap = {}
for aid, sj in con.execute(
    "SELECT agent_id, state_json FROM snapshots "
    "GROUP BY agent_id HAVING captured_at = MAX(captured_at)"
):
    try:
        pid = json.loads(sj).get("status_raw", {}).get("player", {}).get("id")
    except Exception:
        pid = None
    if pid:
        idmap[aid] = pid
con.close()

cards = []
for aid, pid in idmap.items():
    img = os.path.join(CACHE, pid, "portrait.png")
    if not os.path.exists(img):
        continue
    pj = os.path.join(AGENTS, aid, "personality.json")
    name, role, empire = aid, "", ""
    if os.path.exists(pj):
        p = json.load(open(pj))
        name = p.get("name") or aid
        role = p.get("role") or ""
        empire = p.get("empire") or ""
    prompt = ""
    sidecar = os.path.join(CACHE, pid, "prompt.txt")
    if os.path.exists(sidecar):
        lines = open(sidecar, encoding="utf-8").read().split("\n", 1)
        prompt = lines[1].strip() if len(lines) > 1 else ""
    cards.append((name, aid, role, empire, pid, img, prompt))

cards.sort(key=lambda c: (c[1].lower(),))

cells = []
for name, aid, role, empire, pid, img, prompt in cards:
    meta = " · ".join(x for x in (aid, role, empire.title()) if x)
    cells.append(
        f"""    <figure class="cell">
      <img loading="lazy" src="{html.escape(img)}" alt="{html.escape(name)}">
      <figcaption class="cap">
        <span class="nm">{html.escape(name)}</span>
        <span class="mt">{html.escape(meta)}</span>
        <details><summary>prompt</summary><p class="pr">{html.escape(prompt)}</p></details>
      </figcaption>
    </figure>"""
    )

doc = f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Agent Portraits — {len(cards)} images</title>
<style>
  :root {{ color-scheme: dark; }}
  body {{ margin:0; background:#0d0f14; color:#e8eaf0;
         font:14px/1.4 system-ui,-apple-system,Segoe UI,Roboto,sans-serif; }}
  header {{ padding:18px 20px; border-bottom:1px solid #232634; position:sticky; top:0;
            background:#0d0f14ee; backdrop-filter:blur(6px); }}
  h1 {{ margin:0; font-size:17px; font-weight:600; }}
  header .sub {{ color:#8b90a3; font-size:12px; margin-top:3px; }}
  .grid {{ display:grid; grid-template-columns:repeat(4,1fr); gap:14px; padding:20px; }}
  @media (max-width:1100px) {{ .grid {{ grid-template-columns:repeat(3,1fr); }} }}
  @media (max-width:760px)  {{ .grid {{ grid-template-columns:repeat(2,1fr); }} }}
  .cell {{ margin:0; background:#161922; border:1px solid #232634; border-radius:10px;
           overflow:hidden; }}
  .cell img {{ display:block; width:100%; aspect-ratio:1/1; object-fit:cover; }}
  .cap {{ padding:8px 10px; }}
  .nm {{ display:block; font-weight:600; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }}
  .mt {{ display:block; color:#8b90a3; font-size:12px; margin-top:2px; }}
  details {{ margin-top:6px; }}
  summary {{ cursor:pointer; color:#6f9; font-size:12px; }}
  .pr {{ color:#aab; font-size:11px; margin:6px 0 0; white-space:pre-wrap; }}
</style>
</head>
<body>
<header>
  <h1>Agent Portraits (temporary preview)</h1>
  <div class="sub">{len(cards)} generated images · sighted-player subset · SDXL-Turbo</div>
</header>
<div class="grid">
{os.linesep.join(cells)}
</div>
</body>
</html>
"""

with open(OUT, "w", encoding="utf-8") as f:
    f.write(doc)

print(f"wrote {OUT} with {len(cards)} portraits")
