#!/usr/bin/env python3
"""Contact sheet for the wildlife hero seed sweeps: one row per species,
one cell per seed, with the chroma-keyed matte preview beside each render
so a bad key (magenta-tinted creature, busy backdrop) is visible before
anything reaches Hy3D.

    python3 make_seed_sheet.py            # -> heroes-raw/seed_sheet.html
"""

import json
import re
import sys
from collections import defaultdict
from pathlib import Path

HERE = Path(__file__).resolve().parent
RAW = HERE / "heroes-raw"

sys.path.insert(0, str(HERE.parent))
from PIL import Image  # noqa: E402

from run_hy3d import chroma_key  # noqa: E402

HEAD = """<!doctype html><meta charset="utf-8"><title>Wildlife hero seed sweep</title>
<style>
body { background:#181a1f; color:#cfd3da; font:13px system-ui,sans-serif; margin:20px; }
h1 { font-size:18px; } h2 { font-size:15px; margin:22px 0 8px; }
.row { display:flex; gap:10px; flex-wrap:wrap; }
.cell { background:#20242b; border-radius:6px; padding:6px; width:300px; }
.cell img { width:100%; border-radius:4px; background:#181a1f; display:block; }
.cell img + img { margin-top:4px; }
.tag { color:#8a919c; font-size:11px; margin-top:4px; }
.cell.pick { outline:2px solid #ffc832; }
.pick .tag { color:#ffc832; }
.round { color:#8a919c; font-weight:normal; font-size:12px; margin-left:8px; }
.cell img { cursor: zoom-in; }
.pickbtn { float:right; font-size:11px; padding:1px 6px; margin-top:2px; cursor:pointer; }
#lb { position:fixed; inset:0; background:rgba(0,0,0,.92); display:none; align-items:center; justify-content:center; gap:16px; z-index:10; cursor:zoom-out; }
#lb.on { display:flex; }
#lb img { max-height:92vh; max-width:46vw; object-fit:contain; background:#181a1f; border-radius:6px; }
#lb .cap { position:absolute; bottom:12px; left:0; right:0; text-align:center; color:#cfd3da; font-size:13px; }
</style>
<h1>Wildlife hero seed sweep</h1>
<p style="color:#8a919c">Top image = raw FLUX render, bottom = chroma-keyed matte
(what Hy3D would actually see, on checker = transparent). Newest round first
within each species; the current pick (picks-round4.json) is outlined.
<b>Click an image to see it large</b>; press <b>pick</b> on a cell to make it
the species' pick (one per species; picks live in this browser until you
export), then <button id="copy">copy picks JSON</button>
<button id="reset">reset to committed picks</button>
<span id="status"></span></p>
"""

# Click-to-pick: the outline moves, choices persist in localStorage, and the
# copy button puts a picks-round4.json-shaped document on the clipboard.
SCRIPT = """
<div id="lb"><img id="lb-raw" alt=""><img id="lb-key" alt=""><div class="cap" id="lb-cap"></div></div>
<script>
(function () {
  var lb = document.getElementById('lb');
  function show(cell) {
    var imgs = cell.querySelectorAll('img');
    document.getElementById('lb-raw').src = imgs[0].src;
    document.getElementById('lb-key').src = imgs[1].src;
    document.getElementById('lb-cap').textContent = cell.dataset.species + ' \u00b7 ' + cell.dataset.base + ' (raw | keyed)';
    lb.classList.add('on');
  }
  lb.addEventListener('click', function () { lb.classList.remove('on'); });
  document.addEventListener('keydown', function (e) { if (e.key === 'Escape') lb.classList.remove('on'); });
  document.querySelectorAll('.cell img').forEach(function (img) {
    img.addEventListener('click', function (e) { e.stopPropagation(); show(img.closest('.cell')); });
  });

  var KEY = 'wildlife-seed-picks';
  var committed = %s;
  var picks;
  try { picks = JSON.parse(localStorage.getItem(KEY)) || {}; } catch (e) { picks = {}; }
  function current() { var out = Object.assign({}, committed); for (var k in picks) out[k] = picks[k]; return out; }
  function paint() {
    var cur = current(), changed = 0;
    document.querySelectorAll('.cell').forEach(function (c) {
      var on = cur[c.dataset.species] === +c.dataset.seed;
      c.classList.toggle('pick', on);
      c.querySelector('.tag').textContent = c.dataset.base + (on ? ' \u00b7 PICK' : '');
    });
    for (var k in picks) if (picks[k] !== committed[k]) changed++;
    document.getElementById('status').textContent = changed ? changed + ' pick(s) differ from the committed set' : 'matches the committed picks';
  }
  document.querySelectorAll('.cell').forEach(function (c) {
    var b = document.createElement('button'); b.className = 'pickbtn'; b.textContent = 'pick';
    c.querySelector('.tag').before(b);
    b.addEventListener('click', function (e) { e.stopPropagation(); picks[c.dataset.species] = +c.dataset.seed; localStorage.setItem(KEY, JSON.stringify(picks)); paint(); });
  });
  document.getElementById('reset').addEventListener('click', function () { picks = {}; localStorage.removeItem(KEY); paint(); });
  document.getElementById('copy').addEventListener('click', function () {
    var doc = { _note: 'Round 4/4b hero picks (free-fall rules). heroes/<species>.png is a copy of heroes-raw/<species>_s<seed>.png; regenerate with gen_heroes_round4.sh + gen_heroes_round4b.sh.', picks: current() };
    var text = JSON.stringify(doc, null, 1);
    (navigator.clipboard ? navigator.clipboard.writeText(text) : Promise.reject()).then(
      function () { document.getElementById('status').textContent = 'picks JSON copied to clipboard'; },
      function () { window.prompt('copy this JSON', text); });
  });
  paint();
})();
</script>
"""

# Seed blocks per generation round, so a cell can say where it came from.
ROUNDS = [
    (12000, 12499, "turnaround sheets — side-quest, one generation holding four views"),
    (10500, 10999, "round 5 — leading three-quarter overhead view instruction"),
    (11000, 11499, "babies — infant versions of the accepted adults"),
    (10100, 10499, "round 4/4b/4c/4d — free-fall rules"),
    (9700, 10099, "round 3/3b — first full roster"),
    (9500, 9699, "forms — exotic hypotheses"),
    (9200, 9299, "rounds 1-2 — prototypes"),
]


def round_of(seed: int) -> str:
    for lo, hi, name in ROUNDS:
        if lo <= seed <= hi:
            return name
    return "other"


def load_picks() -> dict:
    """species -> picked seed, from every picks-*.json beside this script."""
    picks = {}
    for f in sorted(HERE.glob("picks-*.json")):
        picks.update(json.loads(f.read_text()).get("picks", {}))
    return picks


def checker_composite(rgba: Image.Image) -> Image.Image:
    w, h = rgba.size
    bg = Image.new("RGB", (w, h))
    px = bg.load()
    for y in range(0, h, 32):
        for x in range(0, w, 32):
            c = (44, 46, 52) if (x // 32 + y // 32) % 2 else (58, 60, 68)
            for yy in range(y, min(y + 32, h)):
                for xx in range(x, min(x + 32, w)):
                    px[xx, yy] = c
    bg.paste(rgba, mask=rgba.split()[-1])
    return bg


def main() -> int:
    by_species = defaultdict(list)
    for p in sorted(RAW.glob("*_s*.png")):
        if "_keyed" in p.stem:
            continue
        m = re.match(r"(.+)_s(\d+)$", p.stem)
        if m:
            by_species[m.group(1)].append((int(m.group(2)), p))

    picks = load_picks()
    parts = [HEAD]
    parts.append("<p>" + " · ".join(f"<a href='#{sp}' style='color:#cfd3da'>{sp}</a>" for sp in sorted(by_species)) + "</p>")
    for sp, entries in sorted(by_species.items()):
        parts.append(f"<h2 id='{sp}'>{sp}</h2>")
        current = None
        for seed, p in sorted(entries, reverse=True):
            rnd = round_of(seed)
            if rnd != current:
                if current is not None:
                    parts.append("</div>")
                parts.append(f"<div class='round'>{rnd}</div><div class='row'>")
                current = rnd
            keyed_png = RAW / f"{p.stem}_keyed.png"
            rgba = chroma_key(Image.open(p))
            cov = (rgba.split()[-1].getextrema(), )
            import numpy as np
            a = np.asarray(rgba)[..., 3]
            coverage = (a > 25).mean() * 100
            if not keyed_png.exists() or keyed_png.stat().st_mtime < p.stat().st_mtime:
                checker_composite(rgba).save(keyed_png)
            is_pick = picks.get(sp) == seed
            base_tag = f"seed {seed} · keyed coverage {coverage:.0f}%"
            parts.append(
                f"<div class='cell{' pick' if is_pick else ''}' data-species='{sp}' data-seed='{seed}' data-base='{base_tag}'>"
                f"<img loading='lazy' src='{p.name}'>"
                f"<img loading='lazy' src='{keyed_png.name}'>"
                f"<div class='tag'>{base_tag}{' · PICK' if is_pick else ''}</div></div>")
        if current is not None:
            parts.append("</div>")
    parts.append(SCRIPT % json.dumps(picks))
    (RAW / "seed_sheet.html").write_text("\n".join(parts))
    print(f"seed_sheet.html -> {RAW}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
