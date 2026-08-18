#!/usr/bin/env python3
"""Contact sheet for the wildlife hero seed sweeps: one row per species,
one cell per seed, with the chroma-keyed matte preview beside each render
so a bad key (magenta-tinted creature, busy backdrop) is visible before
anything reaches Hy3D.

    python3 make_seed_sheet.py            # -> heroes-raw/seed_sheet.html
"""

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
</style>
<h1>Wildlife hero seed sweep</h1>
<p style="color:#8a919c">Top image = raw FLUX render, bottom = chroma-keyed matte
(what Hy3D would actually see, on checker = transparent).</p>
"""


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

    parts = [HEAD]
    for sp, entries in sorted(by_species.items()):
        parts.append(f"<h2>{sp}</h2><div class='row'>")
        for seed, p in sorted(entries):
            keyed_png = RAW / f"{p.stem}_keyed.png"
            rgba = chroma_key(Image.open(p))
            cov = (rgba.split()[-1].getextrema(), )
            import numpy as np
            a = np.asarray(rgba)[..., 3]
            coverage = (a > 25).mean() * 100
            checker_composite(rgba).save(keyed_png)
            parts.append(
                f"<div class='cell'><img loading='lazy' src='{p.name}'>"
                f"<img loading='lazy' src='{keyed_png.name}'>"
                f"<div class='tag'>seed {seed} · keyed coverage {coverage:.0f}%</div></div>")
        parts.append("</div>")
    (RAW / "seed_sheet.html").write_text("\n".join(parts))
    print(f"seed_sheet.html -> {RAW}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
