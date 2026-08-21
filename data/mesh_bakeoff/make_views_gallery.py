#!/usr/bin/env python3
"""Triage gallery for the stage-2 side/front view projections.

One row per ship: keyed hero, the projected SIDE and FRONT silhouettes
(inline SVG, dorsal should be UP and the bow RIGHT in the side view),
and the raw render_side.png as ground truth. Serve from the :8478
mesh_bakeoff server. Verdicts for upside-down hulls: add
`"vflip": true` to adjustments-final.json and re-run make_views.py for
that stem.

    python3 make_views_gallery.py   ->  views_gallery.html
"""
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
VIEWS = HERE.parent / "footprints" / "views"

HEAD = """<!doctype html><meta charset="utf-8"><title>side/front view triage</title>
<style>
body { background:#181a1f; color:#cfd3da; font:13px system-ui; margin:20px; }
h1 { font-size:18px }  .sub { color:#8a919c; margin-bottom:14px }
#q { background:#111; color:#cfd3da; border:1px solid #444; border-radius:4px;
     padding:5px 10px; width:280px; margin-bottom:14px }
.row { display:grid; grid-template-columns:220px 1fr 220px 220px; gap:10px;
       background:#20242b; border-radius:6px; padding:8px; margin-bottom:8px;
       align-items:center }
.row img { width:100%; max-height:130px; object-fit:contain;
           background:#181a1f; border-radius:4px }
.row svg { width:100%; max-height:130px; background:#123d75; border-radius:4px }
.name { font-weight:600 } .meta { color:#8a919c; font-size:11px }
.lbl { position:absolute; font-size:10px; color:#8a919c }
</style>
<h1>side/front projections — %COUNT% ships</h1>
<div class="sub">hero · SIDE (bow right, dorsal up) · FRONT · render_side ground
truth. Upside-down hull ⇒ "vflip": true in adjustments-final.json, re-run
make_views.py &lt;stem&gt; then make_blueprints.py &lt;id&gt;.</div>
<input id="q" placeholder="filter by name...">
"""

TAIL = """<script>
document.getElementById('q').addEventListener('input', e => {
  const v = e.target.value.toLowerCase();
  document.querySelectorAll('.row').forEach(r =>
    r.style.display = r.dataset.name.includes(v) ? '' : 'none');
});
</script>
"""


def svg(d, wu, hu):
    # non-scaling stroke: each SVG is fitted into a fixed-height cell, so
    # a unit-space width would render fat on flat hulls and hairline on
    # tall ones — constant screen px keeps the rows comparable
    return (f'<svg viewBox="-10 -10 {wu + 20:.0f} {hu + 20:.0f}" '
            f'preserveAspectRatio="xMidYMid meet">'
            f'<path d="{d}" fill-rule="evenodd" fill="none" stroke="#eaf2ff" '
            f'stroke-width="1.6" vector-effect="non-scaling-stroke"/></svg>')


def main():
    rows = []
    for f in sorted(VIEWS.glob("*.json")):
        v = json.loads(f.read_text())
        stem, sid = v["stem"], v["ship"]
        rows.append(f'''<div class="row" data-name="{sid} {stem}">
<div><div class="name">{sid}</div><div class="meta">{stem} ·
h {v["height_units"]:.0f}u · beam {v["beam_units"]:.0f}u ·
{v["adjustments"] or "no adj"}</div>
<img loading="lazy" src="chromakeys/{stem}.png"></div>
{svg(v["side"], v["len_units"], v["height_units"])}
{svg(v["front"], v["beam_units"], v["height_units"])}
<img loading="lazy" src="out-hy3d-full/{stem}/render_side.png">
</div>''')
    (HERE / "views_gallery.html").write_text(
        HEAD.replace("%COUNT%", str(len(rows))) + "\n".join(rows) + TAIL)
    print(HERE / "views_gallery.html", len(rows), "ships")


if __name__ == "__main__":
    main()
