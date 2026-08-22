#!/usr/bin/env python3
"""Procedural empire emblems for the blueprint title-block logo square.

Pure line-work SVG fragments in a 100x100 viewBox, stroke-only in
currentColor so the sheet can tint them cyanotype-white. One mark per
ship-design faction:

  solarian   sunburst: disc + alternating rays + thin orbit ring
  crimson    crossed swords in front of a heraldic shield
  outerrim   scrapyard gear: 8-tooth ring on a hex nut
  voidborn   entropy spiral: archimedean coil into a point
  nebula     merchant market arrow rising over a tick baseline

    python3 emblems.py           (writes ../../mesh_bakeoff/emblem_review.html)
"""
import math
from pathlib import Path

SW = 2.4          # stroke width in the 100-unit frame


def _pts(seq):
    return " ".join(f"{x:.1f},{y:.1f}" for x, y in seq)


def _poly(seq, closed=True, w=SW):
    return (f'<polyline points="{_pts(seq + ([seq[0]] if closed else []))}" '
            f'fill="none" stroke="currentColor" stroke-width="{w}" '
            f'stroke-linejoin="round" stroke-linecap="round"/>')


def _circle(r, cx=50, cy=50, w=SW, dash=""):
    d = f' stroke-dasharray="{dash}"' if dash else ""
    return (f'<circle cx="{cx}" cy="{cy}" r="{r}" fill="none" '
            f'stroke="currentColor" stroke-width="{w}"{d}/>')


def solarian():
    parts = [_circle(14), _circle(45, w=1.1, dash="3 4")]
    for i in range(12):
        a = math.radians(i * 30)
        r0, r1 = 20, (38 if i % 2 == 0 else 29)
        parts.append(_poly([(50 + r0 * math.cos(a), 50 + r0 * math.sin(a)),
                            (50 + r1 * math.cos(a), 50 + r1 * math.sin(a))],
                           closed=False))
    parts.append(f'<circle cx="{50 + 45 * math.cos(0.6):.1f}" '
                 f'cy="{50 - 45 * math.sin(0.6):.1f}" r="3.2" '
                 f'fill="currentColor"/>')
    return "".join(parts)


def crimson():
    """Crossed swords in front of a heraldic shield (user pick, lab v4
    reworked: the plain saltire read as an X, not as swords)."""
    parts = [_poly([(31, 24), (69, 24), (69, 52), (50, 78), (31, 52)])]
    for sx in (-1, 1):
        ax, ay = 50 - sx * 26, 12                    # blade tip, top
        bx, by = 50 + sx * 26, 88                    # pommel, bottom
        dx, dy = bx - ax, by - ay
        ln = math.hypot(dx, dy)
        ux, uy = dx / ln, dy / ln
        px, py = -uy, ux                             # perpendicular
        gx_, gy_ = ax + dx * 0.62, ay + dy * 0.62    # crossguard station
        w = 2.6
        parts.append(_poly([(ax, ay), (gx_ + px * w, gy_ + py * w),
                            (gx_ - px * w, gy_ - py * w)]))
        parts.append(_poly([(gx_ + px * 9, gy_ + py * 9),
                            (gx_ - px * 9, gy_ - py * 9)], closed=False,
                           w=SW + 0.6))
        parts.append(_poly([(gx_, gy_), (bx - ux * 5, by - uy * 5)],
                           closed=False, w=SW))
        parts.append(_circle(2.8, cx=bx, cy=by, w=1.8))
    return "".join(parts)


def outerrim():
    teeth = []
    n = 8
    for i in range(n):
        a0 = 2 * math.pi * i / n
        for da, r in ((0.00, 40), (0.16, 40), (0.22, 31), (0.40, 31),
                      (0.46, 40)):
            a = a0 + da * 2 * math.pi / n
            teeth.append((50 + r * math.cos(a), 50 + r * math.sin(a)))
    hexpts = [(50 + 16 * math.cos(math.radians(60 * i + 30)),
               50 + 16 * math.sin(math.radians(60 * i + 30)))
              for i in range(6)]
    return _poly(teeth) + _poly(hexpts) + _circle(8)


def voidborn():
    pts = []
    for t in range(0, 320):
        th = t / 320 * 3.1 * 2 * math.pi
        r = 2.5 + 38 * t / 320
        pts.append((50 + r * math.cos(th), 50 + r * math.sin(th)))
    return (_poly(pts[::-1], closed=False) +
            '<circle cx="50" cy="50" r="2.4" fill="currentColor"/>')


def nebula():
    """Market arrow over a tick baseline (user pick, lab v6): the
    merchant empire's rising trade line."""
    return (_poly([(22, 76), (40, 58), (52, 66), (78, 30)], closed=False,
                  w=2.6)
            + _poly([(66, 30), (78, 30), (78, 42)], closed=False, w=2.6)
            + "".join(_poly([(x, 80), (x, 84)], closed=False, w=1.6)
                      for x in range(24, 81, 8)))


def pirate():
    """GSC-0008 stronghold mark (catalog-ghost reticle over a barcode),
    standing in for ALL pirate clans until ships carry a clan."""
    return (_circle(24)
            + _poly([(50, 14), (50, 34)], closed=False, w=1.6)
            + _poly([(50, 66), (50, 86)], closed=False, w=1.6)
            + _poly([(14, 50), (34, 50)], closed=False, w=1.6)
            + _poly([(66, 50), (86, 50)], closed=False, w=1.6)
            + "".join(_poly([(x, 76), (x, 88)], closed=False, w=wd)
                      for x, wd in ((30, 2.8), (35, 1.2), (40, 2.0),
                                    (60, 1.2), (65, 2.8), (70, 1.6))))


MARKS = {"solarian": solarian, "crimson": crimson, "outerrim": outerrim,
         "voidborn": voidborn, "nebula": nebula, "pirate": pirate}


def emblem(faction: str) -> str:
    """Inner-SVG fragment (100x100 frame) or '' for unmarked factions."""
    fn = MARKS.get(faction)
    return fn() if fn else ""


def main() -> int:
    out = Path(__file__).resolve().parent.parent.parent / "mesh_bakeoff" \
        / "emblem_review.html"
    cells = "".join(
        f'<div class="cell"><svg viewBox="0 0 100 100">{fn()}</svg>'
        f'<span>{name.upper()}</span></div>'
        for name, fn in MARKS.items())
    out.write_text(f"""<!doctype html><meta charset="utf-8">
<title>empire emblems</title>
<style>
  body {{ background:#123d75; margin:0; padding:40px; font:13px 'Courier New',
         monospace; letter-spacing:.14em; color:#eaf2ff;
         display:flex; gap:36px; flex-wrap:wrap }}
  .cell {{ text-align:center }}
  .cell svg {{ width:220px; height:220px; color:#eaf2ff; display:block;
              border:1.4px solid #eaf2ff; background:#123d75;
              margin-bottom:10px }}
</style>
{cells}""")
    print(f"{out}\nserve:  http://localhost:8478/emblem_review.html")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
