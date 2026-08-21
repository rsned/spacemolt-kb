#!/usr/bin/env python3
"""Triage page for scale-survey recheck flags.

For every ship in ship_scale_est.json's `recheck` list: the hero with
the recorded window click marked (magenta/cyan lines), the blueprint
sheet, and the numbers — so each flag can be called as "re-measure" or
"confirmed honest outlier" (confirmed ones go in
data/footprints/scale/window_px_overrides.json with confirmed: true).

    python3 make_flag_triage.py   ->  flag_triage.html + flag_triage/*.png
"""
import json
from pathlib import Path

from PIL import Image, ImageDraw

HERE = Path(__file__).resolve().parent
SCALE = HERE.parent / "footprints" / "scale"
OUT = HERE / "flag_triage"


def mark_hero(stem, w):
    img = Image.open(HERE / "chromakeys" / f"{stem}.png").convert("RGB")
    d = ImageDraw.Draw(img)
    x = w.get("x", img.width / 2)
    for y, c in ((w["y0"], (255, 60, 130)), (w["y1"], (60, 210, 255))):
        d.line([(0, y), (img.width, y)], fill=c, width=2)
    d.ellipse([x - 6, w["y0"] - 6, x + 6, w["y1"] + 6], outline=(255, 230, 60),
              width=3)
    OUT.mkdir(exist_ok=True)
    img.save(OUT / f"{stem}.png")


def main():
    est = json.loads((SCALE / "ship_scale_est.json").read_text())
    window = json.loads((SCALE / "window_px.json").read_text())
    rows = []
    for f in est["recheck"]:
        sid = f["ship"]
        r = est["ships"][sid]
        stem = r["stem"]
        w = window.get(stem)
        if w and w.get("h"):
            mark_hero(stem, w)
        rows.append(f"""
<h3>{sid} — measured {f['loa_m']} m vs group anchor {f['group_anchor_m']} m</h3>
<p class="dim">stem {stem} · window {r.get('window_px')} px · hull span
{r.get('span_px')} px · scale class {r['scale']} / {r['group']}</p>
<div class="pair">
  <img src="flag_triage/{stem}.png">
  <object data="bp/{sid}.svg" type="image/svg+xml"></object>
</div>""")
    page = f"""<!doctype html><meta charset="utf-8">
<title>scale survey — recheck flags</title>
<style>
 body {{ background:#0c0e12; color:#9aa1ab; font:14px system-ui;
        max-width:1400px; margin:24px auto; }}
 h3 {{ color:#e8ecf2; font-weight:500; margin:34px 0 4px }}
 .dim {{ color:#5f6a78; margin:0 0 8px }}
 .pair {{ display:grid; grid-template-columns:1fr 1fr; gap:12px }}
 .pair img, .pair object {{ width:100%; border-radius:4px }}
</style>
<h2 style="color:#e8ecf2">Recheck flags — {len(rows)} ships</h2>
<p class="dim">pink line = clicked window top, cyan = bottom, yellow ring =
click site. Verdicts: re-measure in the tool (u, re-click, export), or
declare an honest outlier by adding the stem to
data/footprints/scale/window_px_overrides.json with "confirmed": true.</p>
{"".join(rows)}"""
    (HERE / "flag_triage.html").write_text(page)
    print(HERE / "flag_triage.html", len(rows), "flags")


if __name__ == "__main__":
    main()
