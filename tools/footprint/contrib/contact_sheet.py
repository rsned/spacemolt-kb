#!/usr/bin/env python3
"""Throwaway contact sheet: recovered footprint vs procedural glyph per ship.

Three columns per ship: the existing pkg/shipglyph SVG, the recovered
ground-plane footprint polygon (PCA-rotated so the long axis is vertical,
bow direction arbitrary), and the canonical profile silhouette rebuilt from
profile.json's 96 half-widths. Failures are shown with their recorded reason.
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import html
import json
import pathlib
import sys

import numpy as np

WT = pathlib.Path(_REPO)
sys.path.insert(0, str(WT))
from tools.footprint import profile as fpp  # noqa: E402
FOOT = WT / "data/footprints"
GLYPHS = pathlib.Path(_REPO + "/kb/ships/glyphs")
OUT = pathlib.Path.cwd() / "footprint_contact_sheet.html"

BOX = 190  # drawing box per cell, px


def polygon_paths(coords_sets):
    """SVG path strings for a (Multi)Polygon's rings, long axis vertical.

    True bow/stern is unknown ("orientation": "unknown" in the schema), and
    both this SVD's sign and canonicalise's axis sign are arbitrary — so pin a
    DISPLAY convention instead: the wider end renders at the bottom. The
    profile panel applies the same rule, so the two panels always agree.
    """
    pts = np.concatenate([np.asarray(ring) for part in coords_sets for ring in part])
    ctr = pts.mean(axis=0)
    # PCA: rotate the dominant axis onto +Y for a top-down, nose-up look.
    u, s, vt = np.linalg.svd(pts - ctr, full_matrices=False)
    R = np.array([vt[1], vt[0]])  # minor axis -> x, major axis -> y
    q0 = (pts - ctr) @ R.T
    lo = np.abs(q0[q0[:, 1] < 0, 0]).max(initial=0)   # widest |x| in lower half
    hi = np.abs(q0[q0[:, 1] > 0, 0]).max(initial=0)   # widest |x| in upper half
    if hi > lo:          # wider end currently up (pre-SVG-flip down): flip axis
        R = -R
    span = np.abs((pts - ctr) @ R.T).max() * 2.1
    scale = BOX / span

    def tx(ring):
        q = (np.asarray(ring) - ctr) @ R.T * scale
        q[:, 1] *= -1  # SVG y grows downward
        q += BOX / 2
        return "M " + " L ".join(f"{x:.1f} {y:.1f}" for x, y in q) + " Z"

    return [tx(ring) for part in coords_sets for ring in part]


def profile_path(w, size=None, oriented=False):
    """Symmetric silhouette from 96 half-widths, length normalised to BOX.

    Display convention: wider end at the bottom for UNRESOLVED profiles (t=0
    order hemisphere-arbitrary). `oriented=True` means the profile is stored
    nose-first ("bow_t0", from the user's bow annotations) — draw it as-is,
    nose at the top, no heuristic flip.

    `size`: optional relative-size multiplier (tier scaling). None keeps the
    fill-the-box normalisation. With a size, the drawn length is
    BOX*0.92*size and the cell itself GROWS for size>1 (tier 4/5 cells are
    1.5x/2x tall+wide; tiers 0-3 fit the normal box) — the row is the ruler.
    """
    w = np.asarray(w, dtype=float)
    half = len(w) // 2
    if not oriented and w[:half].mean() > w[half:].mean():  # wider end down
        w = w[::-1]
    t = np.linspace(0, 1, len(w))
    if size is None:
        box_px, length_px = BOX, BOX * 0.85
    else:
        box_px, length_px = tier_box(size), BOX * 0.92 * size
    y0 = (box_px - length_px) / 2.0
    xs = box_px / 2 + np.concatenate([w, -w[::-1]]) * length_px
    ys = y0 + np.concatenate([t, t[::-1]]) * length_px
    return ("M " + " L ".join(f"{x:.1f} {y:.1f}" for x, y in zip(xs, ys)) + " Z")


def tier_box(mult):
    """Cell edge for a tier-scaled panel: normal box up to x1, grows beyond."""
    return int(round(BOX * max(1.0, mult)))


TIER_MULT = {0: 0.5, 1: 0.5, 2: 0.75, 3: 1.0, 4: 1.5, 5: 2.0}  # tier 0 = starter, tier-1 sized
CATALOG = pathlib.Path("/home/robert/spacemolt/spacemolt/data/game-api/latest/catalog_ships.json")
TIERS = {s["id"]: s["tier"] for s in json.loads(CATALOG.read_text())["items"]}


def hero_thumbs():
    """ship_id -> base64 JPEG thumbnail of the source hero image."""
    import base64
    import re
    import cv2
    hero_dirs = [pathlib.Path("/home/robert/Downloads/chromakeys"), pathlib.Path("/home/robert/Downloads")]
    prefix = re.compile(r"^(outerrim|solarian|voidborn|crimson|nebula|pirate)_")
    ids = {r["id"] for r in json.loads((FOOT / "report.json").read_text())["results"]}
    out = {}
    paths = [p for d in hero_dirs for pat in ("*.png", "*.webp") for p in sorted(d.glob(pat))]
    # Exact-stem pass first, matching resolve_heroes' preference — otherwise a
    # duplicate-pair render that sorts earlier (crimson_ordinance.png) shows as
    # the thumbnail for a ship the pipeline solved from its exact-stem file
    # (ordinance.png), and eyeball verdicts get graded against the wrong image.
    for p in [q for q in paths if q.stem in ids] + paths:
        key = p.stem if p.stem in ids else prefix.sub("", p.stem)
        if key in ids and key not in out:
            img = cv2.imread(str(p))
            if img is None:
                continue
            h, w = img.shape[:2]
            s = BOX / max(h, w)
            img = cv2.resize(img, (max(1, int(w * s)), max(1, int(h * s))))
            ok_enc, buf = cv2.imencode(".jpg", img, [cv2.IMWRITE_JPEG_QUALITY, 82])
            if ok_enc:
                out[key] = base64.b64encode(buf).decode()
    return out


THUMBS = hero_thumbs()

import re  # noqa: E402

BAKEOFF = pathlib.Path(_REPO + "/data/mesh_bakeoff/out-full")
_PREFIX = re.compile(r"^(outerrim|solarian|voidborn|crimson|nebula)_")


def mesh_profiles():
    """ship_id -> (w array, aspect) from the TripoSR bake-off, exact stem preferred."""
    out = {}
    if not BAKEOFF.exists():
        return out
    stems = sorted(p.name for p in BAKEOFF.iterdir() if (p / "profile.json").is_file())
    for exact_pass in (True, False):
        for stem in stems:
            sid = stem if exact_pass else _PREFIX.sub("", stem)
            if (exact_pass or sid != stem) and sid not in out:
                d = json.loads((BAKEOFF / stem / "profile.json").read_text())
                out[sid] = (np.array(d["w"], dtype=float), d.get("aspect"))
    return out


MESH = mesh_profiles()


def view_triad_svg(sid):
    """Small gizmo: hull X/Y/Z axes as seen from the detected camera pose.

    X = width (lateral, the symmetry normal), Y = main beam axis
    (longitudinal), Z = up — all recovered in CAMERA coordinates from
    cloud_resolved.npz, so drawing their (x, y) components directly gives the
    triad rotated exactly as the pipeline believes the hero shot is posed.
    Axes pointing away from the camera (z > 0) render dashed. Signs are
    display-canonicalised (up toward screen-top, beam toward screen-right;
    bow/stern remains unknowable) and the frame is kept right-handed.
    """
    npz = FOOT / sid / "cloud_resolved.npz"
    if not npz.exists():
        return None
    d = np.load(npz)
    if "normal" not in d.files or "points" not in d.files:
        return None
    lateral = d["normal"].astype(float)
    lateral /= np.linalg.norm(lateral)
    pts = d["points"].astype(float)
    inplane = pts - np.outer(pts @ lateral, lateral)
    inplane -= inplane.mean(axis=0)
    _, _, vt = np.linalg.svd(inplane[:: max(1, len(inplane) // 20000)], full_matrices=False)
    longitudinal = vt[0] / np.linalg.norm(vt[0])
    up = np.cross(lateral, longitudinal)

    # Display sign conventions (camera frame: x right, y down, z away).
    if abs(up[1]) > 0.2:
        if up[1] > 0:
            up = -up
    elif up[2] > 0:          # near top-down shot: resolve toward the viewer
        up = -up
    if longitudinal[0] > 0:  # user-observed art convention: bow faces screen-LEFT
        longitudinal = -longitudinal
    lateral = np.cross(longitudinal, up)  # re-derive: right-handed after flips

    # Plane-swap suspicion (user-confirmed on symposium): hero art keeps ships
    # visually upright, so a solved up-vector lying screen-horizontal in a
    # non-top-down shot suggests the solver locked the horizontal dorsal plane
    # instead of the bilateral one (frame rolled 90deg about Y; the recovered
    # "footprint" is then a side profile).
    tilt = np.degrees(np.arctan2(abs(up[0]), abs(up[1])))
    elev = np.degrees(np.arcsin(np.clip(abs(up[2]), 0, 1)))
    suspect = tilt > 50 and elev < 55

    C, R = BOX / 2, BOX * 0.36
    axes = [("X", lateral, "#ff7a7a"), ("Y", longitudinal, "#78dd96"), ("Z", up, "#7aa8ff")]
    parts = []
    for name, v, color in sorted(axes, key=lambda a: -a[1][2]):  # far first
        x, y = C + v[0] * R, C + v[1] * R
        away = v[2] > 0
        dash = " stroke-dasharray='4 3'" if away else ""
        op = 0.55 if away else 1.0
        parts.append(
            f"<g stroke='{color}' fill='{color}' opacity='{op}'>"
            f"<line x1='{C:.1f}' y1='{C:.1f}' x2='{x:.1f}' y2='{y:.1f}' stroke-width='2'{dash}/>"
            f"<circle cx='{x:.1f}' cy='{y:.1f}' r='3'/>"
            f"<text x='{C + v[0] * (R + 16):.1f}' y='{C + v[1] * (R + 16) + 4:.1f}' "
            f"font-size='13' text-anchor='middle' stroke='none'>{name}</text></g>")
    warn = (f"<text x='{BOX - 6}' y='16' font-size='12' text-anchor='end' "
            f"fill='#e8c66a'>&#9888; plane?</text>" if suspect else "")
    return (f"<svg viewBox='0 0 {BOX} {BOX}' class='shape triad'>"
            f"<circle cx='{C}' cy='{C}' r='{R:.0f}' fill='none' stroke='#2a2f38'/>"
            + "".join(parts) + warn + "</svg>")


def best_r(a, b):
    """Pearson r over both orientations (bow/stern unknown on both sides)."""
    a, b = np.asarray(a, float), np.asarray(b, float)
    if a.std() == 0 or b.std() == 0:
        return float("nan")
    return max(float(np.corrcoef(a, b)[0, 1]), float(np.corrcoef(a[::-1], b)[0, 1]))

report = json.loads((FOOT / "report.json").read_text())
order = sorted(report["results"], key=lambda r: (not r["status"].startswith("ok"), r["id"]))

cells = []
for r in order:
    sid, status = r["id"], r["status"]
    ok = status.startswith("ok")
    reason = r.get("reason") or r.get("dimensional_reason") or ""
    q = r.get("quality", {})
    meta = []
    if q.get("silhouette_iou") is not None:
        meta.append(f"IoU {q['silhouette_iou']:.3f}")
    if r.get("aspect") is not None:
        meta.append(f"aspect {r['aspect']:.2f}")

    glyph = (GLYPHS / f"{sid}.svg").read_text()

    # A ship only reaches stage 6 if it recovered or failed the (stage-7)
    # dimensional check; earlier failures may leave a STALE footprint.json
    # from a previous batch on disk — never render those.
    reached_stage6 = ok or status == "failed_dimensional_check"
    fp_svg = "<div class='missing'>no footprint<br>(failed before stage 6)</div>"
    fpj = FOOT / sid / "footprint.json"
    if reached_stage6 and fpj.exists():
        poly = json.loads(fpj.read_text())["polygon"]
        parts = [poly["coordinates"]] if poly["type"] == "Polygon" else poly["coordinates"]
        paths = "".join(f"<path d='{d}'/>" for d in polygon_paths(parts))
        fp_svg = (f"<svg viewBox='0 0 {BOX} {BOX}' class='shape'>"
                  f"<g fill='none' stroke='currentColor' stroke-width='1.4' "
                  f"stroke-linejoin='round'>{paths}</g></svg>")

    pr_svg = "<div class='missing'>no profile</div>"
    prj = FOOT / sid / "profile.json"
    oriented = False
    if reached_stage6 and prj.exists():
        pj = json.loads(prj.read_text())
        oriented = pj.get("orientation") == "bow_t0"
        # Amber ticks on stations whose cut is SPLIT (concave[i]): the envelope
        # w(t) has blobbed an out-rigger pod onto the body there — the gap
        # exists in the footprint polygon but is unrepresentable in w(t).
        w_arr = np.asarray(pj["w"], dtype=float)
        conc = np.asarray(pj.get("concave", []), dtype=bool)
        flip = (not oriented
                and w_arr[: len(w_arr) // 2].mean() > w_arr[len(w_arr) // 2:].mean())
        ticks = ""
        if conc.any():
            wf, cf = (w_arr[::-1], conc[::-1]) if flip else (w_arr, conc)
            t = np.linspace(0, 1, len(wf))
            y0, L = (BOX - BOX * 0.85) / 2.0, BOX * 0.85
            ticks = "".join(
                f"<line x1='{BOX/2 - wf[i]*L:.1f}' y1='{y0 + t[i]*L:.1f}' "
                f"x2='{BOX/2 + wf[i]*L:.1f}' y2='{y0 + t[i]*L:.1f}' "
                f"stroke='#e8c66a' stroke-width='1' opacity='0.5'/>"
                for i in range(len(wf)) if cf[i])
        # Nose marker for oriented profiles: small triangle at the top (t=0).
        nose = ("<path d='M {c} 6 l 5 8 l -10 0 Z' fill='#63d68b'/>"
                .format(c=BOX / 2) if oriented else "")
        pr_svg = (f"<svg viewBox='0 0 {BOX} {BOX}' class='shape'>"
                  f"<path d='{profile_path(pj['w'], oriented=oriented)}' fill='currentColor' "
                  f"fill-opacity='0.25' stroke='currentColor' stroke-width='1.2'/>"
                  f"{ticks}{nose}</svg>")
        if pj.get("aspect") is not None and "aspect" not in " ".join(meta):
            meta.append(f"aspect {pj['aspect']:.2f}")

    mesh_svg = "<div class='missing'>no mesh</div>"
    slim_svg = "<div class='missing'>no mesh</div>"
    sq_svg = "<div class='missing'>no mesh</div>"
    if sid in MESH:
        mw, masp = MESH[sid]
        mesh_svg = (f"<svg viewBox='0 0 {BOX} {BOX}' class='shape mesh'>"
                    f"<path d='{profile_path(mw)}' fill='currentColor' "
                    f"fill-opacity='0.25' stroke='currentColor' stroke-width='1.2'/></svg>")
        # Foreshortening correction candidate: TripoSR systematically over-widens
        # (user-measured "needs 1.5-2x stretch"); 33% beam narrowing = aspect x1.49.
        slim_svg = (f"<svg viewBox='0 0 {BOX} {BOX}' class='shape mesh'>"
                    f"<path d='{profile_path(mw * 0.67)}' fill='currentColor' "
                    f"fill-opacity='0.25' stroke='currentColor' stroke-width='1.2'/></svg>")
        # Plateau-snap squares TripoSR's rounded engine blocks — a PER-SHIP call
        # (good on blocky sterns, harmful on e.g. postulate), so both variants
        # are separate pickable panels and the user's click decides.
        sq_svg = (f"<svg viewBox='0 0 {BOX} {BOX}' class='shape mesh'>"
                  f"<path d='{profile_path(fpp.snap_plateaus(mw * 0.67))}' fill='currentColor' "
                  f"fill-opacity='0.25' stroke='currentColor' stroke-width='1.2'/></svg>")
        if masp is not None:
            meta.append(f"mesh aspect {masp:.2f} (adj {masp / 0.67:.2f})")
        if reached_stage6 and prj.exists():
            meta.append(f"mesh r {best_r(mw, np.array(pj['w'], dtype=float)):.2f}")

    # Tier-scaled panels: same profiles, drawn at relative size so scale
    # differences across ships are visible (tier 5 fills the box).
    tier = TIERS.get(sid)
    mult = TIER_MULT.get(tier, 1.0)
    D = tier_box(mult)
    tier_pipe_svg = "<div class='missing'>no profile</div>"
    if reached_stage6 and prj.exists():
        tier_pipe_svg = (f"<svg viewBox='0 0 {D} {D}' class='tierbox' style='width:{D}px;height:{D}px'>"
                         f"<path d='{profile_path(np.array(pj['w'], dtype=float), size=mult, oriented=oriented)}' "
                         f"fill='currentColor' fill-opacity='0.25' stroke='currentColor' stroke-width='1.1'/></svg>")
    tier_mesh_svg = "<div class='missing'>no mesh</div>"
    if sid in MESH:
        tier_mesh_svg = (f"<svg viewBox='0 0 {D} {D}' class='tierbox mesh' style='width:{D}px;height:{D}px'>"
                         f"<path d='{profile_path(MESH[sid][0] * 0.67, size=mult)}' "
                         f"fill='currentColor' fill-opacity='0.25' stroke='currentColor' stroke-width='1.1'/></svg>")
    tier_badge = f"<span class='tier'>T{tier} &times;{mult:g}</span>" if tier is not None else ""

    triad = view_triad_svg(sid)
    # 12-way bow-direction picker: clock positions (30-deg steps) around the
    # triad. Angle convention: degrees clockwise from screen-up, so
    # 12 o'clock = 0, 3 o'clock = 90, 8 o'clock = 240 (the most common bow
    # direction per the user).
    bow_dots = "".join(
        f"<button class='bowbtn' data-angle='{ang}' "
        f"title='bow at {(ang // 30) or 12} o&#39;clock' "
        f"style='left:{50 + 48 * np.sin(np.radians(ang)):.1f}%;"
        f"top:{50 - 48 * np.cos(np.radians(ang)):.1f}%'></button>"
        for ang in range(0, 360, 30))
    triad_fig = (f"<figure class='triadfig'>"
                 f"<div class='triadwrap'>{triad}{bow_dots}</div>"
                 f"<figcaption>view axes · click dot = bow dir</figcaption></figure>"
                 if triad else
                 "<figure><div class='missing'>no view<br>(failed before stage 4)</div>"
                 "<figcaption>view axes</figcaption></figure>")

    cells.append(f"""
<div class="card {'ok' if ok else 'fail'}" data-ship="{html.escape(sid)}">
  <h3>{html.escape(sid)} {tier_badge}<span class="st">{html.escape(status)}</span></h3>
  <div class="row">
    <figure>{f"<img class='hero' src='data:image/jpeg;base64,{THUMBS[sid]}'>" if sid in THUMBS else "<div class='missing'>hero image<br>not found</div>"}<figcaption>hero art</figcaption></figure>
    {triad_fig}
    <figure class="sel" data-panel="glyph">{glyph}<figcaption>glyph</figcaption></figure>
    <figure class="sel" data-panel="footprint">{fp_svg}<figcaption>recovered footprint</figcaption></figure>
    <figure class="sel" data-panel="moge">{pr_svg}<figcaption>profile w(t){' · nose ▲' if oriented else ''}</figcaption></figure>
    <figure class="sel" data-panel="mesh">{mesh_svg}<figcaption>mesh w(t) (TripoSR)</figcaption></figure>
    <figure class="sel" data-panel="mesh067">{slim_svg}<figcaption>mesh &times;0.67 beam</figcaption></figure>
    <figure class="sel" data-panel="mesh067sq">{sq_svg}<figcaption>mesh &times;0.67 squared</figcaption></figure>
    <figure>{tier_pipe_svg}<figcaption>moge &times; tier</figcaption></figure>
    <figure>{tier_mesh_svg}<figcaption>mesh 0.67 &times; tier</figcaption></figure>
  </div>
  <p class="meta">{html.escape(' · '.join(meta))}</p>
  {f'<p class="reason">{html.escape(str(reason))}</p>' if reason else ''}
</div>""")

n_ok = sum(1 for r in order if r["status"].startswith("ok"))
OUT.write_text(f"""<!doctype html><meta charset="utf-8">
<title>Footprint recovery — contact sheet</title>
<style>
 body {{ font: 14px/1.4 system-ui, sans-serif; margin: 24px; background: #14161a; color: #dde3ea; }}
 h1 {{ font-size: 20px; }} .sub {{ color: #8b96a5; margin-bottom: 20px; }}
 .grid {{ display: grid; grid-template-columns: repeat(auto-fill, minmax(2060px, 1fr)); gap: 16px; }}
 .card {{ border: 1px solid #2a2f38; border-radius: 10px; padding: 12px 16px; background: #1a1e24; }}
 .card.fail {{ border-color: #5a3038; background: #201a1d; }}
 .card h3 {{ margin: 0 0 8px; font-size: 15px; }}
 .st {{ font-weight: normal; font-size: 12px; color: #8b96a5; margin-left: 8px; }}
 .tier {{ font-weight: 600; font-size: 12px; color: #e8c66a; border: 1px solid #4a4230; border-radius: 4px; padding: 1px 6px; margin-right: 6px; }}
 .card.fail .st {{ color: #d98a94; }}
 .row {{ display: flex; gap: 12px; align-items: flex-start; }}
 figure {{ margin: 0; text-align: center; }}
 figure svg {{ width: {BOX}px; height: {BOX}px; color: #9fd0ff; background: #11141a; border-radius: 6px; }}
 img.hero {{ max-width: {BOX}px; max-height: {BOX}px; border-radius: 6px; background: #11141a; }}
 .card.fail figure svg {{ color: #d0a0a8; }}
 figure svg.mesh {{ color: #c9a86a; }}
 .card.fail figure svg.mesh {{ color: #c9a86a; }}
 figcaption {{ font-size: 11px; color: #8b96a5; margin-top: 4px; }}
 .missing {{ width: {BOX}px; height: {BOX}px; display: flex; align-items: center; justify-content: center;
            font-size: 12px; color: #6b7480; background: #11141a; border-radius: 6px; text-align: center; }}
 .meta {{ font-size: 12px; color: #8b96a5; margin: 8px 0 0; }}
 .reason {{ font-size: 12px; color: #d98a94; margin: 4px 0 0; }}
 figure svg.triad {{ background: #11141a; }}
 figure.sel {{ cursor: pointer; }}
 figure.sel:hover svg, figure.sel:hover .missing {{ box-shadow: 0 0 0 2px #4a5568; }}
 figure.sel.selected svg, figure.sel.selected .missing {{ box-shadow: 0 0 0 3px #63d68b; }}
 figure.sel.selected figcaption {{ color: #63d68b; font-weight: 600; }}
 #seltools {{ position: fixed; top: 12px; right: 16px; z-index: 10; background: #1a1e24;
   border: 1px solid #2a2f38; border-radius: 8px; padding: 8px 12px; font-size: 13px;
   display: flex; gap: 10px; align-items: center; }}
 #seltools button {{ background: #2a3444; color: #dde3ea; border: 1px solid #3a4557;
   border-radius: 6px; padding: 4px 10px; cursor: pointer; font-size: 13px; }}
 #seltools button:hover {{ background: #35415a; }}
 .triadwrap {{ position: relative; width: {BOX}px; height: {BOX}px; margin: 0 auto; }}
 .bowbtn {{ position: absolute; width: 16px; height: 16px; border-radius: 50%;
   border: 1px solid #4a5568; background: #222a35; cursor: pointer; padding: 0;
   transform: translate(-50%, -50%); }}
 .bowbtn:hover {{ background: #35415a; border-color: #7aa8ff; }}
 .bowbtn.chosen {{ background: #63d68b; border-color: #63d68b; }}
</style>
<div id="seltools"><span id="selcount">0 picked</span>
  <button id="selexport">Export picks</button></div>
<h1>Footprint recovery — contact sheet</h1>
<p class="sub">alpha {report['alpha']} · background {report['background']} · {n_ok}/19 recovered ·
footprint orientation is PCA-derived (bow direction arbitrary); glyph and recovery need not agree on nose-up.
Click a panel to pick the best result per ship (click again to unpick) — picks persist in this browser; Export downloads them as JSON.</p>
<div class="grid">{''.join(cells)}</div>
<script>
const KEY = 'footprint_best_picks';
const BOWKEY = 'footprint_bow_dirs';
const load = () => JSON.parse(localStorage.getItem(KEY) || '{{}}');
const save = s => localStorage.setItem(KEY, JSON.stringify(s));
const loadBow = () => JSON.parse(localStorage.getItem(BOWKEY) || '{{}}');
const saveBow = s => localStorage.setItem(BOWKEY, JSON.stringify(s));
const updateCount = () =>
  document.getElementById('selcount').textContent =
    Object.keys(load()).length + ' picked · ' + Object.keys(loadBow()).length + ' bows';

document.querySelectorAll('figure.sel').forEach(f => f.addEventListener('click', () => {{
  const card = f.closest('.card'), sid = card.dataset.ship, store = load();
  const was = f.classList.contains('selected');
  card.querySelectorAll('figure.sel.selected').forEach(x => x.classList.remove('selected'));
  if (was) delete store[sid];
  else {{ f.classList.add('selected'); store[sid] = f.dataset.panel; }}
  save(store); updateCount();
}}));

// restore picks on load
{{
  const store = load();
  for (const [sid, panel] of Object.entries(store)) {{
    const f = document.querySelector(
      `.card[data-ship="${{CSS.escape(sid)}}"] figure.sel[data-panel="${{CSS.escape(panel)}}"]`);
    if (f) f.classList.add('selected');
  }}
  updateCount();
}}

document.querySelectorAll('.bowbtn').forEach(b => b.addEventListener('click', () => {{
  const card = b.closest('.card'), sid = card.dataset.ship, store = loadBow();
  const was = b.classList.contains('chosen');
  card.querySelectorAll('.bowbtn.chosen').forEach(x => x.classList.remove('chosen'));
  if (was) delete store[sid];
  else {{ b.classList.add('chosen'); store[sid] = parseInt(b.dataset.angle, 10); }}
  saveBow(store); updateCount();
}}));

{{
  const store = loadBow();
  for (const [sid, ang] of Object.entries(store)) {{
    const b = document.querySelector(
      `.card[data-ship="${{CSS.escape(sid)}}"] .bowbtn[data-angle="${{ang}}"]`);
    if (b) b.classList.add('chosen');
  }}
  updateCount();
}}

document.getElementById('selexport').addEventListener('click', () => {{
  const payload = {{best_picks: load(), bow_directions_deg_cw_from_screen_up: loadBow()}};
  const blob = new Blob([JSON.stringify(payload, null, 2)], {{type: 'application/json'}});
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = 'footprint_annotations.json';
  a.click();
  URL.revokeObjectURL(a.href);
}});
</script>
""")
print(f"wrote {OUT} ({OUT.stat().st_size} bytes, {len(cells)} ships)")
