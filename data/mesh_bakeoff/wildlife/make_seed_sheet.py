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
</style>
<h1>Wildlife hero seed sweep</h1>
<p style="color:#8a919c">Top image = raw FLUX render, bottom = chroma-keyed matte
(what Hy3D would actually see, on checker = transparent). Newest round first
within each species; the current pick (picks-round4.json) is outlined.</p>
"""

# Seed blocks per generation round, so a cell can say where it came from.
ROUNDS = [
    (10100, 10499, "round 4/4b — free-fall rules"),
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
            parts.append(
                f"<div class='cell{' pick' if is_pick else ''}'><img loading='lazy' src='{p.name}'>"
                f"<img loading='lazy' src='{keyed_png.name}'>"
                f"<div class='tag'>seed {seed} · keyed coverage {coverage:.0f}%{' · PICK' if is_pick else ''}</div></div>")
        if current is not None:
            parts.append("</div>")
    (RAW / "seed_sheet.html").write_text("\n".join(parts))
    print(f"seed_sheet.html -> {RAW}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
