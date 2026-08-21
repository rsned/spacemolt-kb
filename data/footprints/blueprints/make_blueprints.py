#!/usr/bin/env python3
"""Generate cyanotype blueprint sheets for the KB ship pages.

One SVG "registry sheet" per catalog ship that has a footprint: blue
field, drafting grid, heavy double border, the top-view silhouette in
white line-work with L.O.A./beam dimension callouts (meters from
../scale/ship_scale_est.json, labeled by source), a duotone keyed-hero
perspective vignette, and a title block. Ships without a scale estimate
yet render with "DIMENSIONS PENDING SURVEY".

Outputs:
  kb/ships/blueprints/<id>.svg      committed (pure text)
  kb/ships/blueprints/art/<id>.png  gitignored raster vignette, regenerated
                                    from the chromakeys hero drop; the sheet
                                    degrades gracefully when it is absent

Stage 2 hook: if ../views/<id>.json exists (side/front silhouette paths
from the Hy3D mesh), the sheet becomes a full three-view. Not built yet.

    python3 make_blueprints.py [ship_id ...]
"""
import json
import re
import sys
from pathlib import Path

import numpy as np
from PIL import Image

try:
    import interiors            # deck-plan linework; needs skimage
except ImportError:             # pragma: no cover
    interiors = None

HERE = Path(__file__).resolve().parent
FOOT = HERE.parent
REPO = FOOT.parent.parent
SVGDIR = FOOT / "hy3d-svg"
SCALE = FOOT / "scale" / "ship_scale_est.json"
HEROES = REPO / "data" / "mesh_bakeoff" / "chromakeys"
OUT = REPO / "kb" / "ships" / "blueprints"

BLUE, LINE = "#123d75", "#eaf2ff"   # classic cyanotype (user pick, variant A)
GRID_A = 0.16

SW = 1200                            # sheet width; height depends on hull


def load_footprint(ship_id):
    p = SVGDIR / f"{ship_id}.svg"
    if not p.exists():
        return None
    txt = p.read_text()
    vb = re.search(r'viewBox="0 0 ([\d.]+) ([\d.]+)"', txt)
    d = re.search(r'<path d="([^"]+)"', txt).group(1)
    stem = re.search(r'data-art-stem="([^"]+)"', txt)
    return (float(vb.group(1)), float(vb.group(2)), d,
            stem.group(1) if stem else ship_id)


def duotone_hero(stem, ship_id):
    """Keyed hero -> cyanotype duotone PNG vignette (gitignored raster)."""
    src = HEROES / f"{stem}.png"
    if not src.exists():
        return False
    rgb = np.asarray(Image.open(src).convert("RGB"), np.float32)
    key = np.array([255, 0, 255], np.float32)
    alpha = np.clip((np.sqrt(((rgb - key) ** 2).sum(2)) - 60) / 60, 0, 1)
    lum = (rgb @ [0.299, 0.587, 0.114]) / 255.0
    t = (lum ** 0.75)[..., None] * alpha[..., None]
    blue = np.array([int(BLUE[i:i + 2], 16) for i in (1, 3, 5)], np.float32)
    white = np.array([int(LINE[i:i + 2], 16) for i in (1, 3, 5)], np.float32)
    out = blue + (white - blue) * t
    img = Image.fromarray(out.clip(0, 255).astype(np.uint8))
    img.thumbnail((640, 640))
    (OUT / "art").mkdir(parents=True, exist_ok=True)
    img.save(OUT / "art" / f"{ship_id}.png", optimize=True)
    return True


def dim_h(x0, x1, y, label):
    return f'''<g class="dim">
  <line x1="{x0:.0f}" y1="{y:.0f}" x2="{x1:.0f}" y2="{y:.0f}"/>
  <line x1="{x0:.0f}" y1="{y - 6:.0f}" x2="{x0:.0f}" y2="{y + 6:.0f}"/>
  <line x1="{x1:.0f}" y1="{y - 6:.0f}" x2="{x1:.0f}" y2="{y + 6:.0f}"/>
  <path d="M{x0:.0f} {y:.0f} l 10 -3.5 v 7 z" class="arr"/>
  <path d="M{x1:.0f} {y:.0f} l -10 -3.5 v 7 z" class="arr"/>
  <text x="{(x0 + x1) / 2:.0f}" y="{y + 16:.0f}" text-anchor="middle">{label}</text>
</g>'''


def dim_v(x, y0, y1, label):
    return f'''<g class="dim">
  <line x1="{x:.0f}" y1="{y0:.0f}" x2="{x:.0f}" y2="{y1:.0f}"/>
  <line x1="{x - 6:.0f}" y1="{y0:.0f}" x2="{x + 6:.0f}" y2="{y0:.0f}"/>
  <line x1="{x - 6:.0f}" y1="{y1:.0f}" x2="{x + 6:.0f}" y2="{y1:.0f}"/>
  <path d="M{x:.0f} {y0:.0f} l -3.5 10 h 7 z" class="arr"/>
  <path d="M{x:.0f} {y1:.0f} l -3.5 -10 h 7 z" class="arr"/>
  <text x="{x + 10:.0f}" y="{(y0 + y1) / 2 - 8:.0f}">{label}</text>
</g>'''


def source_label(src, ref=""):
    if src == "window":
        return "PILOT SURVEY" if "pilot" in (ref or "") else "WINDOW SURVEY"
    if src and src.startswith("ladder"):
        return "REGISTRY EST."
    return ""


def sheet(ship_id, ship, est):
    fp = load_footprint(ship_id)
    if fp is None:
        return None
    w, h, d, stem = fp
    has_art = duotone_hero(stem, ship_id)
    deck = ""

    # geometry: hull box on the left, vignette + title block column right
    col_x, col_w = 830, 330
    pad_l, pad_t = 96, 90
    cw = col_x - 40 - pad_l          # hull content width
    s = cw / w
    if interiors is not None:
        deck = interiors.deck_plan(
            ship_id, d, w, h, ship.get("cargo", 0),
            (est or {}).get("window_t"), s, LINE)
    ch = h * s
    hull_ch = max(ch, 300)
    SH = max(hull_ch + pad_t + 150, 690)   # floor fits art+stats+title column
    y0 = pad_t + (SH - 150 - pad_t - ch) / 2   # vertically center the hull
    x0, x1, y1 = pad_l, pad_l + cw, None
    y1 = y0 + ch
    gy, gx = y1 + 40, x0 - 42

    grid = "".join(
        f'<line x1="{gx0}" y1="16" x2="{gx0}" y2="{SH - 16:.0f}" class="g{1 if i % 5 else 0}"/>'
        for i, gx0 in enumerate(range(40, SW - 16, 24))) + "".join(
        f'<line x1="16" y1="{gy0}" x2="{SW - 16}" y2="{gy0}" class="g{1 if i % 5 else 0}"/>'
        for i, gy0 in enumerate(range(40, int(SH) - 16, 24)))

    if est:
        loa, beam = est["loa_m"], est["beam_m"]
        tag = source_label(est.get("source"), est.get("ref"))
        dims = dim_h(x0, x1, gy, f"L.O.A. {loa:.0f} m") + \
            dim_v(gx, y0, y1, f"{beam:.0f} m")
        dim_line = f"L.O.A. {loa:.0f} m · BEAM {beam:.0f} m · {tag}"
    else:
        dims = ""
        dim_line = "DIMENSIONS PENDING SURVEY"

    # perspective vignette panel
    art_y, art_h = 60, 300
    if has_art:
        art = (f'<rect x="{col_x}" y="{art_y}" width="{col_w}" height="{art_h}" class="frame" stroke-width="1.2"/>'
               f'<image href="art/{ship_id}.png" x="{col_x + 8}" y="{art_y + 8}" '
               f'width="{col_w - 16}" height="{art_h - 36}" preserveAspectRatio="xMidYMid meet"/>'
               f'<text x="{col_x + 12}" y="{art_y + art_h - 12}">PERSPECTIVE VIEW — REGISTRY ARCHIVE</text>')
    else:
        art = ""

    # stats box between the vignette and the title block
    st_y = art_y + art_h + 16
    rows = [("SHIELDS", f'{ship["shield"]}'), ("ARMOR", f'{ship["armor"]}'),
            ("HULL", f'{ship["hull"]}'), ("FUEL", f'{ship["fuel"]}'),
            ("CARGO", f'{ship["cargo"]}'),
            ("SPEED", f'{ship["speed"]} AU/T'),
            ("SLOTS W/D/U", "{}/{}/{}".format(*ship["slots"]))]
    rh, st_h = 18, 26 + 18 * len(rows) + 8
    stats = [f'<rect x="{col_x}" y="{st_y}" width="{col_w}" height="{st_h}" class="frame" stroke-width="1.2"/>',
             f'<line x1="{col_x}" y1="{st_y + 22}" x2="{col_x + col_w}" y2="{st_y + 22}" stroke="{LINE}" stroke-width="1"/>',
             f'<text x="{col_x + 12}" y="{st_y + 16}">REGISTRY STATS</text>']
    for i, (k, v) in enumerate(rows):
        ry = st_y + 38 + i * rh
        stats.append(f'<text x="{col_x + 12}" y="{ry}">{k}</text>')
        stats.append(f'<text x="{col_x + col_w - 12}" y="{ry}" text-anchor="end">{v}</text>')
    stats_box = "".join(stats)

    tb_y = SH - 116
    name = ship["name"].upper()
    # explicit width/height: <object> embeds need the intrinsic size to
    # scale responsively (viewBox alone yields the 300x150 default)
    return f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {SW} {SH:.0f}" width="{SW}" height="{SH:.0f}" style="background:{BLUE}" role="img" aria-label="{name} blueprint">
<style>
  text {{ font: 11px 'Courier New', monospace; fill: {LINE}; letter-spacing: .12em; }}
  .g0 {{ stroke: {LINE}; stroke-opacity: {GRID_A}; stroke-width:.7 }}
  .g1 {{ stroke: {LINE}; stroke-opacity: {GRID_A * 0.45}; stroke-width:.5 }}
  .hull {{ fill: {LINE}; fill-opacity: .10; stroke: {LINE};
           stroke-width: 2.4; stroke-linejoin: round; }}
  .dim line {{ stroke: {LINE}; stroke-width: 1; }}
  .dim .arr {{ fill: {LINE}; stroke: none; }}
  .cl {{ stroke: {LINE}; stroke-width: .8; stroke-dasharray: 22 5 3 5;
         stroke-opacity: .7 }}
  .frame {{ fill: {BLUE}; stroke: {LINE}; }}
  .big {{ font-size: 16px; font-weight: bold; }}
  .tb text {{ letter-spacing: .05em }}
</style>
<rect x="0" y="0" width="{SW}" height="{SH:.0f}" fill="{BLUE}"/>
{grid}
<rect x="8" y="8" width="{SW - 16}" height="{SH - 16:.0f}" fill="none" stroke="{LINE}" stroke-width="5"/>
<rect x="18" y="18" width="{SW - 36}" height="{SH - 36:.0f}" fill="none" stroke="{LINE}" stroke-width="1.2"/>
<line x1="{x0 - 22}" y1="{(y0 + y1) / 2:.0f}" x2="{x1 + 22}" y2="{(y0 + y1) / 2:.0f}" class="cl"/>
<g transform="translate({x0} {y0:.1f}) scale({s:.5f})">
  <path d="{d}" fill-rule="evenodd" class="hull" style="stroke-width:{2.4 / s:.2f}"/>
  {deck}
</g>
<text x="{x0}" y="{pad_t - 26}">TOP VIEW</text>
{dims}
{art}
<g class="tb">{stats_box}</g>
<g class="tb">
  <rect x="{col_x}" y="{tb_y:.0f}" width="{col_w}" height="80" class="frame" stroke-width="1.4"/>
  <line x1="{col_x}" y1="{tb_y + 28:.0f}" x2="{col_x + col_w}" y2="{tb_y + 28:.0f}" stroke="{LINE}" stroke-width="1"/>
  <text x="{col_x + 12}" y="{tb_y + 20:.0f}" class="big">{name}</text>
  <text x="{col_x + 12}" y="{tb_y + 46:.0f}">{(ship["cls"] or "").upper()} · TIER {ship["tier"]} · SCALE CLASS {ship["scale"]}</text>
  <text x="{col_x + 12}" y="{tb_y + 64:.0f}">{dim_line}</text>
</g>
<text x="36" y="{SH - 30:.0f}">SPACEMOLT NAVAL REGISTRY — {ship_id.replace("_", "-").upper()} — WINDOW CONVENTION: 1 PANE = 1 m</text>
</svg>'''


def main() -> int:
    import sqlite3
    db = sqlite3.connect(REPO / "spacemolt-knowledge.db")
    ships = {r[0]: {"name": r[1], "cls": r[2] or "", "scale": r[3],
                    "tier": r[4], "cargo": r[5] or 0,
                    "shield": r[6] or 0, "armor": r[7] or 0,
                    "hull": r[8] or 0, "fuel": r[9] or 0, "speed": r[10] or 0,
                    "slots": (r[11] or 0, r[12] or 0, r[13] or 0)}
             for r in db.execute(
                 "select id,name,class,scale,tier,cargo_capacity,"
                 "base_shield,base_armor,base_hull,base_fuel,base_speed,"
                 "weapon_slots,defense_slots,utility_slots from ships")}
    est = json.loads(SCALE.read_text())["ships"] if SCALE.exists() else {}

    only = set(sys.argv[1:])
    OUT.mkdir(parents=True, exist_ok=True)
    n = 0
    for sid, ship in sorted(ships.items()):
        if only and sid not in only:
            continue
        svg = sheet(sid, ship, est.get(sid))
        if svg is None:
            continue
        (OUT / f"{sid}.svg").write_text(svg)
        n += 1
    print(f"{n} blueprint sheets -> {OUT}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
