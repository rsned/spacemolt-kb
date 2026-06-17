#!/usr/bin/env python3
"""Build a standalone local contact-sheet of EVERY passenger.

Reads passenger metadata from the knowledge DB and the generated portrait cache
under overlays/generated/passengers/<id>/portrait.png, then writes a single
self-contained HTML file with a responsive grid (4 columns on a wide screen).
Every passenger gets a card: the rendered portrait where present, a "pending"
placeholder otherwise (useful while a regen is mid-flight). For passengers
imported without a citizenship, the name-inferred empire (overlays/generated/
empire_guess.json) is shown with a trailing "?".

Run from the kb repo root:
    python3 tools/build_portrait_gallery.py [knowledge.db] [out.html]
Defaults: /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db  ->  portrait-gallery.html
"""

import html
import json
import os
import sqlite3
import sys

DB = sys.argv[1] if len(sys.argv) > 1 else "/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db"
OUT = sys.argv[2] if len(sys.argv) > 2 else "portrait-gallery.html"
CACHE = "overlays/generated/passengers"
GUESS = "overlays/generated/empire_guess.json"

con = sqlite3.connect(DB)
con.row_factory = sqlite3.Row
rows = con.execute(
    "SELECT citizen_id, name, citizenship, class FROM passengers"
).fetchall()
con.close()

guesses = {}
if os.path.exists(GUESS):
    with open(GUESS) as f:
        guesses = json.load(f)

# Every passenger gets a card; img may be absent (still rendering / not yet run).
cards = []
for r in rows:
    cid = r["citizen_id"]
    img = os.path.join(CACHE, cid, "portrait.png")
    cit = (r["citizenship"] or "").strip()
    empire = cit.title() if cit else (
        f"{guesses[cid]['empire'].title()} ?" if cid in guesses else "")
    cards.append((r["name"] or cid, (r["class"] or ""), empire, cid, img, os.path.exists(img)))

cards.sort(key=lambda c: (c[0].lower(), c[3]))
rendered = sum(1 for c in cards if c[5])

cells = []
for name, cls, empire, cid, img, has_img in cards:
    meta = " · ".join(p for p in (cls.title(), empire) if p)
    media = (f'<img loading="lazy" src="{html.escape(img)}" alt="{html.escape(name)}">'
             if has_img else '<div class="ph">pending</div>')
    cells.append(f"""    <a class="cell{'' if has_img else ' empty'}" href="kb/passengers/{html.escape(cid)}/" title="{html.escape(name)}">
      {media}
      <div class="cap"><span class="nm">{html.escape(name)}</span><span class="mt">{html.escape(meta)}</span></div>
    </a>""")

doc = f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Passenger Portraits — {rendered}/{len(cards)}</title>
<style>
  :root {{ color-scheme: dark; }}
  body {{ margin: 0; background:#0d0f14; color:#e8eaf0;
         font:14px/1.4 system-ui,-apple-system,Segoe UI,Roboto,sans-serif; }}
  header {{ padding:18px 20px; border-bottom:1px solid #232634; position:sticky; top:0;
            background:#0d0f14ee; backdrop-filter:blur(6px); }}
  h1 {{ margin:0; font-size:17px; font-weight:600; }}
  header .sub {{ color:#8b90a3; font-size:12px; margin-top:3px; }}
  .grid {{ display:grid; grid-template-columns:repeat(4, 1fr); gap:14px; padding:20px; }}
  @media (max-width:1100px) {{ .grid {{ grid-template-columns:repeat(3,1fr); }} }}
  @media (max-width:760px)  {{ .grid {{ grid-template-columns:repeat(2,1fr); }} }}
  .cell {{ display:block; text-decoration:none; color:inherit; background:#161922;
           border:1px solid #232634; border-radius:10px; overflow:hidden;
           transition:transform .12s ease, border-color .12s ease; }}
  .cell:hover {{ transform:translateY(-3px); border-color:#3a4055; }}
  .cell.empty {{ opacity:.55; }}
  .cell img {{ display:block; width:100%; aspect-ratio:1/1; object-fit:cover; }}
  .ph {{ display:flex; align-items:center; justify-content:center; width:100%; aspect-ratio:1/1;
         background:repeating-linear-gradient(45deg,#161922,#161922 10px,#1b1f2a 10px,#1b1f2a 20px);
         color:#5a6072; font-size:12px; letter-spacing:.08em; text-transform:uppercase; }}
  .cap {{ padding:8px 10px; }}
  .nm {{ display:block; font-weight:600; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }}
  .mt {{ display:block; color:#8b90a3; font-size:12px; margin-top:2px; }}
</style>
</head>
<body>
<header>
  <h1>Passenger Portraits</h1>
  <div class="sub">{rendered} of {len(cards)} rendered · FLUX.1-schnell · "?" = name-inferred empire · click a card for the passenger page</div>
</header>
<div class="grid">
{os.linesep.join(cells)}
</div>
</body>
</html>
"""

with open(OUT, "w", encoding="utf-8") as f:
    f.write(doc)

print(f"wrote {OUT} — {rendered}/{len(cards)} rendered ({len(cards) - rendered} pending)")
