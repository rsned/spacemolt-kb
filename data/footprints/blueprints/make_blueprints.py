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
  kb/ships/blueprints/index.html    committed gallery of every sheet
                                    (lazy-loaded, name/class filter)
  kb/ships/blueprints/art/<id>.png  gitignored raster vignette, regenerated
                                    from the chromakeys hero drop; the sheet
                                    degrades gracefully when it is absent

Three-view: if ../views/<id>.json exists (side/front silhouettes
projected from the Hy3D mesh by data/mesh_bakeoff/make_views.py), the
sheet becomes a full three-view -- side elevation above the plan sharing
its x-scale (stations line up vertically), front elevation in a middle
column sharing the side view's height rows, height dimension callout.
Ships without a views file keep the single-view layout.

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
try:
    import emblems              # empire marks for the title-block square
except ImportError:             # pragma: no cover
    emblems = None

HERE = Path(__file__).resolve().parent
FOOT = HERE.parent
REPO = FOOT.parent.parent
SVGDIR = FOOT / "hy3d-svg"
SCALE = FOOT / "scale" / "ship_scale_est.json"
HEROES = REPO / "data" / "mesh_bakeoff" / "chromakeys"
OUT = REPO / "kb" / "ships" / "blueprints"

BLUE, LINE = "#123d75", "#eaf2ff"   # classic cyanotype (user pick, variant A)
GRID_A = 0.16


def load_views(ship_id):
    p = FOOT / "views" / f"{ship_id}.json"
    return json.loads(p.read_text()) if p.exists() else None


# Discontinued hulls whose outline is filed under a retired id. The hy3d sweep
# ran before the 2026-03-03 faction-prefix rename, so Benefit's trace is
# nebula_benefit.svg. Same resolution as shipFootprint() in
# cmd/generate-items-kb/ships.go, from the same source of truth.
def _alias_map():
    p = REPO / "data" / "legacy.json"
    if not p.exists():
        return {}
    doc = json.loads(p.read_text())
    return {i: rec.get("aliases") or []
            for i, rec in (doc.get("ships") or {}).items()}


ALIASES = _alias_map()


def footprint_stem(ship_id):
    """The hy3d-svg stem for a ship, or None when nothing was ever traced."""
    if (SVGDIR / f"{ship_id}.svg").exists():
        return ship_id
    for alias in ALIASES.get(ship_id, []):
        if (SVGDIR / f"{alias}.svg").exists():
            return alias
    return None


def load_footprint(ship_id):
    stem = footprint_stem(ship_id)
    if stem is None:
        return None
    p = SVGDIR / f"{stem}.svg"
    txt = p.read_text()
    vb = re.search(r'viewBox="0 0 ([\d.]+) ([\d.]+)"', txt)
    d = re.search(r'<path d="([^"]+)"', txt).group(1)
    stem = re.search(r'data-art-stem="([^"]+)"', txt)
    return (float(vb.group(1)), float(vb.group(2)), d,
            stem.group(1) if stem else ship_id)


def duotone_hero(stem, ship_id, mirror=True):
    """Keyed hero -> cyanotype duotone PNG vignette (gitignored raster).

    Prefer the bake's keyed.png: run_hy3d's chroma_key removes the
    flood-connected drop shadows that a plain distance key keeps (they are
    darkened magenta -- far from the key by distance, magenta by hue), which
    otherwise show up as a light haze under the ship on the sheet."""
    keyed = REPO / "data" / "mesh_bakeoff" / "out-hy3d-full" / stem / "keyed.png"
    if keyed.exists():
        rgba = np.asarray(Image.open(keyed).convert("RGBA"), np.float32)
        rgb, alpha = rgba[..., :3], rgba[..., 3] / 255.0
    else:
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
    if mirror:
        # heroes are near-universally composed bow-left; mirror so the
        # vignette's bow points right like the ortho views. The rare
        # bow-right hero opts out via "hero_bow_right" in adjustments.
        img = img.transpose(Image.FLIP_LEFT_RIGHT)
    img.thumbnail((640, 640))
    (OUT / "art").mkdir(parents=True, exist_ok=True)
    # 128-color palette: the duotone gradient quantizes invisibly and
    # halves the size -- these ARE committed so the live site shows them
    img = img.convert("P", palette=Image.ADAPTIVE, colors=128)
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


def sheet(ship_id, ship, est, adj_map=None, bridge_pos=None):
    fp = load_footprint(ship_id)
    if fp is None:
        return None
    w, h, d, stem = fp
    flags = (adj_map or {}).get(stem, {})
    has_art = duotone_hero(stem, ship_id,
                           mirror=not flags.get("hero_bow_right"))
    deck = ""

    vw = load_views(ship_id)

    # Right-column stack, hoisted above the layout branch because the
    # single-view sheet has to be tall enough to hold it. Top to bottom:
    # vignette, gap, stats card, gap, title block, bottom margin.
    ART_Y, ART_H, TB_H, COL_GAP, BOT_M = 60, 300, 160, 16, 36
    stat_rows = [("SHIELDS", f'{ship["shield"]}'), ("ARMOR", f'{ship["armor"]}'),
                 ("HULL", f'{ship["hull"]}'), ("FUEL", f'{ship["fuel"]}'),
                 ("CARGO", f'{ship["cargo"]}'),
                 ("SPEED", f'{ship["speed"]} AU/T'),
                 ("SLOTS W/D/U", "{}/{}/{}".format(*ship["slots"]))]
    st_h = 26 + 18 * len(stat_rows) + 8
    col_stack = ART_Y + ART_H + COL_GAP + st_h + COL_GAP + TB_H + BOT_M

    # geometry: ortho views on the left, vignette + title block column right
    pad_l, pad_t = 96, 90
    cw = 830 - 40 - pad_l            # hull content width
    s = cw / w
    deck_meta = {}
    if interiors is not None:
        deck, deck_meta = interiors.deck_plan(
            ship_id, d, w, h, ship.get("cargo", 0),
            (est or {}).get("window_t"), s, LINE,
            ext_pilot=bool(flags.get("ext_pilot")), bridge_pos=bridge_pos)
    ch = h * s
    x0, x1 = pad_l, pad_l + cw
    gx = x0 - 42

    views = ""
    if vw:
        # side elevation above the plan at the SAME scale, x-aligned so
        # stations line up vertically; front elevation in a middle column
        # sharing the side view's height rows (fit-scaled only when a very
        # wide beam would blow the column)
        side_ch = vw["height_units"] * s
        y_s0 = pad_t
        y0 = y_s0 + side_ch + 80
        y1 = y0 + ch
        gy = y1 + 40
        mid_x = 830
        # place the front view so its true CENTERLINE (the hull's bilateral
        # symmetry axis from make_views' de-roll fit, not the bbox middle)
        # sits at the column center. ALL views share the same scale s — a
        # wide beam widens the sheet rather than shrinking the front view.
        fc = vw.get("front_center_units", vw["beam_units"] / 2)
        halfw = max(fc, vw["beam_units"] - fc)
        # column floor guarantees room for the perspective vignette frame
        mid_w = max(2 * halfw * s + 24, 474)
        sf = s
        fcx = mid_x + mid_w / 2
        fx = fcx - fc * sf
        fy = y_s0 + (side_ch - vw["height_units"] * sf) / 2
        col_x = round(mid_x + mid_w + 30)
        # sheet height also fits a full-size vignette frame in the middle
        # bay (short slim hulls would otherwise letterbox the hero)
        art_need = 0.75 * max(450.0, min(mid_w - 24, 690.0))
        SH = max(gy + 72, y_s0 + side_ch + 26 + art_need + 16 + 196, 690)
        # side-view interiors: the plan's hold interval (plan x minus the
        # 10-unit footprint margin = side x) and ~5 m deck spacing
        side_extra = front_extra = ""
        if interiors is not None:
            hold = deck_meta.get("hold")
            hold_side = (max(hold[0] - 10, 0),
                         min(hold[1] - 10, vw["len_units"])) if hold else None
            step = (3.0 / est["loa_m"] * 1000) if est else None
            side_extra = interiors.side_overlay(
                vw["side"], vw["len_units"], vw["height_units"],
                hold_side, step, s, ship_id, LINE)
            front_extra = interiors.side_overlay(
                vw["front"], vw["beam_units"], vw["height_units"],
                None, None, sf, f"{ship_id}_front", LINE,
                front_core=bool(hold))
        views = f'''<text x="{x0}" y="{pad_t - 26}">SIDE ELEVATION</text>
<text x="{mid_x:.0f}" y="{pad_t - 26}">FRONT ELEVATION</text>
<g transform="translate({x0 + 10 * s:.1f} {y_s0:.1f}) scale({s:.5f})">
  <path d="{vw["side"]}" fill-rule="evenodd" class="hull" style="stroke-width:{2.4 / s:.2f}"/>
  {side_extra}
</g>
<line x1="{fcx:.0f}" y1="{y_s0 - 14:.0f}" x2="{fcx:.0f}" y2="{y_s0 + side_ch + 14:.0f}" class="cl"/>
<g transform="translate({fx:.1f} {fy:.1f}) scale({sf:.5f})">
  <path d="{vw["front"]}" fill-rule="evenodd" class="hull" style="stroke-width:{2.4 / sf:.2f}"/>
  {front_extra}
</g>'''
    else:
        col_x = 830
        hull_ch = max(ch, 300)
        SH = max(hull_ch + pad_t + 150, col_stack)
        y0 = pad_t + (SH - 150 - pad_t - ch) / 2   # vertically center
        y1 = y0 + ch
        gy = y1 + 40
    col_w = 330
    SW = col_x + col_w + 40

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
        if vw:
            dims += dim_v(gx, y_s0, y_s0 + side_ch,
                          f"{loa * vw['height_units'] / 1000:.0f} m")
        dim_line = f"L.O.A. {loa:.0f} m · BEAM {beam:.0f} m · {tag}"
    else:
        dims = ""
        dim_line = "DIMENSIONS PENDING SURVEY"

    # perspective vignette panel: three-view sheets tuck it into the
    # empty middle bay between the front elevation and the plan; the
    # single-view layout keeps it at the top of the right column
    tb_h = TB_H
    tb_y = SH - tb_h - BOT_M
    if has_art and vw:
        # frameless: the duotone hero floats on the field with a plain
        # rule underneath (user mock), half again the old panel size
        aw = max(450.0, min(mid_w - 24, 690.0))
        ah = 0.75 * aw
        ay = max(y0 + (ch - ah) / 2, y_s0 + side_ch + 26)
        if ay + ah + 16 > tb_y:                 # keep clear of the title row
            ay = max(y_s0 + side_ch + 26, tb_y - 16 - ah)
            ah = min(ah, tb_y - 16 - ay)
        ax = mid_x + mid_w / 2 - aw / 2
        art = (f'<rect x="{ax:.0f}" y="{ay:.0f}" width="{aw:.0f}" height="{ah:.0f}" '
               f'class="frame" stroke-width="1.2"/>'
               f'<image href="art/{ship_id}.png" x="{ax + 8:.0f}" y="{ay + 8:.0f}" '
               f'width="{aw - 16:.0f}" height="{ah - 16:.0f}" preserveAspectRatio="xMidYMid meet"/>')
    elif has_art:
        art_y, art_h = ART_Y, ART_H
        art = (f'<rect x="{col_x}" y="{art_y}" width="{col_w}" height="{art_h}" class="frame" stroke-width="1.2"/>'
               f'<image href="art/{ship_id}.png" x="{col_x + 8}" y="{art_y + 8}" '
               f'width="{col_w - 16}" height="{art_h - 16}" preserveAspectRatio="xMidYMid meet"/>')
    else:
        art = ""

    # stats panel: a narrow card pinned to the upper-right corner on
    # three-view sheets; below the vignette on the single-view layout
    rows, rh = stat_rows, 18
    if vw:
        st_w = 250
        st_x, st_y = SW - 40 - st_w, 60
    else:
        st_w = col_w
        st_x, st_y = col_x, ART_Y + ART_H + COL_GAP
    stats = [f'<rect x="{st_x}" y="{st_y}" width="{st_w}" height="{st_h}" class="frame" stroke-width="1.2"/>',
             f'<line x1="{st_x}" y1="{st_y + 22}" x2="{st_x + st_w}" y2="{st_y + 22}" stroke="{LINE}" stroke-width="1"/>',
             f'<text x="{st_x + 12}" y="{st_y + 16}">REGISTRY STATS</text>']
    for i, (k, v) in enumerate(rows):
        ry = st_y + 38 + i * rh
        stats.append(f'<text x="{st_x + 12}" y="{ry}">{k}</text>')
        stats.append(f'<text x="{st_x + st_w - 12}" y="{ry}" text-anchor="end">{v}</text>')
    stats_box = "".join(stats)

    name = ship["name"].upper()

    def fit(txt, px):
        # squeeze over-long lines into the title box (monospace estimate)
        if len(txt) * px > col_w - 28:
            return f' textLength="{col_w - 28}" lengthAdjust="spacingAndGlyphs"'
        return ""

    name_fit = fit(name, 17.4)
    meta1 = (f'{(ship["cls"] or "").upper()} · TIER {ship["tier"]} · '
             f'SCALE CLASS {ship["scale"]}')
    m1_fit, m2_fit = fit(meta1, 9.4), fit(dim_line, 9.4)

    # empire mark in the logo square left of the title block
    mk = emblems.emblem(ship.get("faction") or "") if emblems else ""
    logo = (f'<g transform="translate({col_x - tb_h - 16 + 14} {tb_y + 14:.0f}) '
            f'scale({(tb_h - 28) / 100:.3f})" color="{LINE}" '
            f'opacity="0.92">{mk}</g>') if mk else ""
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
  .tt {{ font-size: 26px; font-weight: bold; }}
  .tm {{ font-size: 14px; }}
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
<text x="{x0}" y="{(y0 - 26) if vw else (pad_t - 26):.0f}">TOP VIEW</text>
{views}
{dims}
{art}
<g class="tb">{stats_box}</g>
<g class="tb">
  <rect x="{col_x - tb_h - 16}" y="{tb_y:.0f}" width="{tb_h}" height="{tb_h}" class="frame" stroke-width="1.4"/>
  {logo}
  <rect x="{col_x}" y="{tb_y:.0f}" width="{col_w}" height="{tb_h}" class="frame" stroke-width="1.4"/>
  <line x1="{col_x}" y1="{tb_y + 56:.0f}" x2="{col_x + col_w}" y2="{tb_y + 56:.0f}" stroke="{LINE}" stroke-width="1"/>
  <text x="{col_x + 14}" y="{tb_y + 38:.0f}" class="tt"{name_fit}>{name}</text>
  <text x="{col_x + 14}" y="{tb_y + 96:.0f}" class="tm"{m1_fit}>{meta1}</text>
  <text x="{col_x + 14}" y="{tb_y + 132:.0f}" class="tm"{m2_fit}>{dim_line}</text>
</g>
<text x="36" y="{SH - 30:.0f}">SPACEMOLT NAVAL REGISTRY — {ship_id.replace("_", "-").upper()} — WINDOW CONVENTION: 1 PANE = 1 m</text>
</svg>'''


GAL_HEAD = """<!doctype html><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Registry Blueprints</title>
<style>
body { background:#0c0e12; color:#9aa1ab; font:14px system-ui; margin:0 }
header { padding:18px 24px 6px } h1 { color:#e8ecf2; font-size:20px; margin:0 }
.sub { color:#5f6a78; margin:6px 0 10px }
#q { background:#111; color:#cfd3da; border:1px solid #333a46; border-radius:4px;
     padding:6px 12px; width:300px; margin:0 0 6px }
main { max-width:1180px; margin:0 auto; padding:0 24px 40px }
.card { margin:26px 0 }
.hd { display:flex; gap:14px; align-items:baseline; margin-bottom:6px }
.hd b { color:#e8ecf2; font-size:15px } .hd .m { color:#5f6a78; font-size:12px }
.hd a { color:#7aa7d8; margin-left:auto; font-size:12px; text-decoration:none }
.hd a:hover { text-decoration:underline }
.ph { width:100%; background:#123d75; border-radius:4px }
.ph object { display:block; width:100%; height:auto }
.bc { color:#5f6a78; font-size:12px; margin-bottom:8px }
.bc a { color:#7aa7d8; text-decoration:none }
.bc a:hover { text-decoration:underline }
.dl { color:#7aa7d8; text-decoration:none; border:1px solid #333a46;
      border-radius:4px; padding:6px 12px; margin-left:10px; font-size:12px }
.dl:hover { border-color:#7aa7d8; text-decoration:none }
</style>
<header>
<!-- This page carries its own chrome, not the generated KB site header, so
     it needs an explicit way back or it is a dead end from kb/ships/. Mirrors
     the breadcrumb the all-ships page uses (ships.go), restyled for this page's
     palette. -->
<div class="bc"><a href="../index.html">Ships</a> / Registry Blueprints</div>
<h1>Registry Blueprints</h1>
<div class="sub">%COUNT% sheets · three-view registry drawings from the hull
survey · window convention: 1 pane = 1 m</div>
<input id="q" placeholder="filter by name / class / category...">
<!-- Built into kb/ at deploy time by scripts/build-footprint-zip.sh; the
     .zip is gitignored, so this link 404s on a local checkout. -->
<a class="dl" href="ship_footprint_svg.zip" download>&#x2193; all footprint
SVGs (top / side / front, 275 ships)</a></header>
<main>
"""

GAL_TAIL = """</main>
<script>
const io = new IntersectionObserver(es => es.forEach(e => {
  if (!e.isIntersecting) return;
  const o = document.createElement('object');
  o.type = 'image/svg+xml'; o.data = e.target.dataset.src;
  e.target.replaceChildren(o); io.unobserve(e.target);
}), { rootMargin: '900px' });
document.querySelectorAll('.ph').forEach(p => io.observe(p));
document.getElementById('q').addEventListener('input', e => {
  const v = e.target.value.toLowerCase();
  document.querySelectorAll('.card').forEach(c =>
    c.style.display = c.dataset.name.includes(v) ? '' : 'none');
});
</script>
"""


def gallery(ships, est):
    """index.html over every sheet present in OUT (lazy-loaded)."""
    from urllib.parse import quote
    cards = []
    for sid, ship in sorted(ships.items(), key=lambda kv: kv[1]["name"]):
        p = OUT / f"{sid}.svg"
        if not p.exists():
            continue
        head = p.read_text()[:400]
        m = re.search(r'viewBox="0 0 (\d+) (\d+)', head)
        e = est.get(sid)
        meta = f'{ship["cls"]} · {ship["category"]} · TIER {ship["tier"]}'
        if e:
            meta += f' · L.O.A. {e["loa_m"]:.0f} m'
        page = f'../{quote(ship["category"])}/{sid}.html'
        cards.append(f'''<div class="card" data-name="{sid} {ship["name"].lower()} {ship["cls"].lower()} {ship["category"].lower()}">
<div class="hd"><b>{ship["name"].upper()}</b><span class="m">{meta.upper()}</span>
<a href="{page}">ship page ↗</a></div>
<div class="ph" data-src="{sid}.svg" style="aspect-ratio:{m.group(1)}/{m.group(2)}"></div>
</div>''')
    (OUT / "index.html").write_text(
        GAL_HEAD.replace("%COUNT%", str(len(cards))) + "\n".join(cards) + GAL_TAIL)
    return len(cards)


def main() -> int:
    import sqlite3
    db = sqlite3.connect(REPO / "spacemolt-knowledge.db")
    ships = {r[0]: {"name": r[1], "cls": r[2] or "", "scale": r[3],
                    "tier": r[4], "cargo": r[5] or 0,
                    "shield": r[6] or 0, "armor": r[7] or 0,
                    "hull": r[8] or 0, "fuel": r[9] or 0, "speed": r[10] or 0,
                    "slots": (r[11] or 0, r[12] or 0, r[13] or 0),
                    "category": r[14] or "Uncategorized",
                    "faction": r[15] or ""}
             for r in db.execute(
                 "select id,name,class,scale,tier,cargo_capacity,"
                 "base_shield,base_armor,base_hull,base_fuel,base_speed,"
                 "weapon_slots,defense_slots,utility_slots,category,faction"
                 " from ships")}

    # Retired hulls are not in the DB -- 45 of the 49 never got there, since the
    # DB only holds what agents met. Their catalog-shaped records come from the
    # same overlay the KB generators merge, so a discontinued ship with a traced
    # outline gets a real registry sheet instead of a bare silhouette.
    ov = REPO / "overlays" / "generated" / "legacy_ships.json"
    if ov.exists():
        for r in json.loads(ov.read_text()):
            ships.setdefault(r["id"], {
                "name": r.get("name") or r["id"], "cls": r.get("class") or "",
                "scale": r.get("scale"), "tier": r.get("tier") or 0,
                "cargo": r.get("cargo_capacity") or 0,
                "shield": r.get("base_shield") or 0,
                "armor": r.get("base_armor") or 0,
                "hull": r.get("base_hull") or 0,
                "fuel": r.get("base_fuel") or 0,
                "speed": r.get("base_speed") or 0,
                "slots": (r.get("weapon_slots") or 0, r.get("defense_slots") or 0,
                          r.get("utility_slots") or 0),
                "category": r.get("category") or "Uncategorized",
                "faction": r.get("faction") or "",
            })

    est = json.loads(SCALE.read_text())["ships"] if SCALE.exists() else {}
    adj_f = REPO / "data" / "mesh_bakeoff" / "adjustments-final.json"
    adj_map = json.loads(adj_f.read_text()) if adj_f.exists() else {}
    # curated bridge/cockpit stations from make_bridge_placer.py's export
    bp_f = HERE / "bridge_positions.json"
    bpos_map = json.loads(bp_f.read_text()) if bp_f.exists() else {}

    only = set(sys.argv[1:])
    OUT.mkdir(parents=True, exist_ok=True)
    n = 0
    for sid, ship in sorted(ships.items()):
        if only and sid not in only:
            continue
        svg = sheet(sid, ship, est.get(sid), adj_map, bpos_map.get(sid))
        if svg is None:
            continue
        (OUT / f"{sid}.svg").write_text(svg)
        n += 1
    g = gallery(ships, est)
    print(f"{n} blueprint sheets + gallery ({g} cards) -> {OUT}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
