#!/usr/bin/env python3
"""Build the keep-vs-Meshy triage sheet for a Hunyuan3D sweep.

One row per ship: hero image, two shaded renders of the generated mesh,
the KB's fused footprint outline, and the footprint extracted from the
generated mesh -- side by side so the fused column can be compared against
(or chosen over) the generated one. Rows are auto-flagged when the mesh is
a likely failure (ambiguous extraction frame, no fused entry to compare,
or aspect far from the fused value); flagged rows start pre-checked, and
the checked set is the Meshy paid-fallback queue.

Renders reuse render_mesh.py's splatter, so this must run in the venv that
has trimesh (~/hy3d-venv). Rough by design -- it is a working sheet, not a
KB page.

    ~/hy3d-venv/bin/python make_triage_sheet.py [--dir out-hy3d-full] [--limit N]
"""

import argparse
import json
from pathlib import Path

import numpy as np
import trimesh
from PIL import Image, ImageDraw

from render_mesh import look_at, render

HERE = Path(__file__).resolve().parent
FUSED = HERE.parent / "footprints" / "fused"
PREFIXES = ("nebula_", "solarian_", "outerrim_", "crimson_", "voidborn_")

OUTLINE_W, OUTLINE_H = 360, 220
OUTLINE_BG, OUTLINE_FG = 28, 225
ASPECT_FLAG = 0.25  # |hy3d - fused| / fused above this flags the row


def ship_id(stem: str) -> str:
    for p in PREFIXES:
        if stem.startswith(p):
            return stem[len(p):]
    return stem


def load_fused(stem: str) -> dict | None:
    for cand in (ship_id(stem), stem):
        f = FUSED / f"{cand}.json"
        if f.exists():
            return json.loads(f.read_text())
    return None


def polygon_rings(geo) -> list[tuple[np.ndarray, bool]]:
    """(ring, is_hole) pairs from either polygon encoding in play: the
    extraction's GeoJSON Polygon/MultiPolygon dict, or the fused files'
    bare list-of-rings. Ring 0 of each polygon is its exterior, the rest
    are holes. Keeping the role explicit matters for MultiPolygon
    footprints: treating every ring after the first as a hole erased
    disconnected parts (nebula_benefit's front outrigger)."""
    if isinstance(geo, dict):
        polys = [geo["coordinates"]] if geo["type"] == "Polygon" else geo["coordinates"]
    else:
        polys = [geo]
    return [(np.asarray(ring, dtype=float), i > 0)
            for poly in polys for i, ring in enumerate(poly)]


def profile_rings(fused: dict) -> list[np.ndarray]:
    """Reconstruct a symmetric outline from the 96-station width profile.
    w is normalised against unit hull length, so the real width scale is
    (1/aspect) at the widest station -- without the rescale the outline
    draws at the wrong aspect (that bug shipped in the first superposition
    comparison image)."""
    x = np.asarray(fused["w"], dtype=float)
    st = np.linspace(0.0, 1.0, len(x))
    half = x * ((1.0 / fused["aspect"]) / x.max()) / 2.0
    top = np.column_stack([st, half])
    bot = np.column_stack([st[::-1], -half[::-1]])
    return [(np.vstack([top, bot]), False)]


def draw_outline(rings: list[tuple[np.ndarray, bool]], out_path: Path,
                 frame: str = "xlong") -> None:
    """Long-axis-horizontal, aspect-preserving fill on a dark card,
    bow (probably) to the right.

    frame: 'xlong' -- coordinates already have the hull long axis on x
    (fused polygons, profile reconstructions); 'latlong' -- extraction
    frame with x=lateral, y=longitudinal (hy3d footprint.json), so swap.
    Trusting the stored frame instead of re-running PCA matters on squat
    hulls: with aspect ~1 the principal axes are degenerate and vertex
    density (greebled edges carry more points) tilted crimson_levy ~15
    degrees off the bow-stern axis."""
    arrs = [r for r, _ in rings]
    holes = [h for _, h in rings]
    pts = np.vstack(arrs)
    ctr = pts.mean(axis=0)
    if frame == "latlong":
        rot = [(r - ctr)[:, ::-1] for r in arrs]
    else:
        rot = [r - ctr for r in arrs]
    allp = np.vstack(rot)
    # bow heuristic: hulls taper toward the nose while engines/cargo widen
    # the stern, so the narrower outer-20% end is called the bow and drawn
    # on the right. PCA's sign is arbitrary, so this also makes the fused
    # and hy3d columns face the same way for any wedge-shaped hull.
    x, y = allp[:, 0], allp[:, 1]
    lo, hi = x.min(), x.max()
    end = 0.2 * (hi - lo)
    left_w = np.abs(y[x < lo + end]).max() if (x < lo + end).any() else 0
    right_w = np.abs(y[x > hi - end]).max() if (x > hi - end).any() else 0
    if left_w < right_w:  # narrow end sits left -> flip bow to the right
        rot = [r * np.array([-1.0, 1.0]) for r in rot]
        allp = np.vstack(rot)
    span = allp.max(axis=0) - allp.min(axis=0)
    scale = min((OUTLINE_W - 20) / max(span[0], 1e-9),
                (OUTLINE_H - 20) / max(span[1], 1e-9))

    img = Image.new("L", (OUTLINE_W, OUTLINE_H), OUTLINE_BG)
    dr = ImageDraw.Draw(img)
    # centre on the bbox midpoint, NOT the vertex mean: a needle-nosed hull
    # has almost no vertices near the tip, so the mean sits far aft and the
    # nose rendered off-canvas (crimson_shiv's clipped point)
    mid = (allp.min(axis=0) + allp.max(axis=0)) / 2
    # all exteriors first, then all holes, so one polygon's hole can never
    # erase another polygon's body
    order = [i for i, h in enumerate(holes) if not h] + \
            [i for i, h in enumerate(holes) if h]
    for i in order:
        px = (rot[i] - mid) * scale + [OUTLINE_W / 2, OUTLINE_H / 2]
        fill = OUTLINE_BG if holes[i] else OUTLINE_FG
        dr.polygon([tuple(p) for p in px], fill=fill)
    img.save(out_path)


def render_views(mesh_path: Path, out_dir: Path) -> bool:
    try:
        mesh = trimesh.load(mesh_path, force="mesh")
        pts, fi = trimesh.sample.sample_surface(mesh, 300_000)
        nrm = mesh.face_normals[fi]
        pts = np.asarray(pts) - np.asarray(pts).mean(axis=0)
        render(pts, nrm, look_at(18, 35)).save(out_dir / "render_tq.png")
        # the three canonical orthographic projections are the rotation
        # snapper's candidates: Hy3D emits meshes in a sensible canonical
        # frame, and when the extractor's PCA rolls the footprint around
        # the bow-stern axis (flat-winged or side-heavy hulls), the true
        # top-down is almost always one of these
        render(pts, nrm, look_at(89.9, 0)).save(out_dir / "render_td.png")
        render(pts, nrm, look_at(0, 90)).save(out_dir / "render_side.png")
        render(pts, nrm, look_at(0, 0)).save(out_dir / "render_front.png")
        return True
    except Exception as exc:
        print(f"  render failed: {exc}")
        return False


def build_row(d: Path) -> dict:
    stem = d.name
    row = {"stem": stem, "id": ship_id(stem), "flags": []}

    prof_f = d / "profile.json"
    fp_f = d / "footprint.json"
    if prof_f.exists():
        prof = json.loads(prof_f.read_text())
        row["hy3d_aspect"] = prof["aspect"]
        if prof["frame_ambiguous"]:
            row["flags"].append("frame_ambiguous")
    else:
        row["flags"].append("no_extraction")

    fused = load_fused(stem)
    if fused is None:
        row["flags"].append("no_fused_entry")
    elif not fused.get("aspect"):
        # a fused record can carry a shape with no resolved aspect; there is
        # nothing to compare against, which is itself a reason to review
        row["flags"].append("fused_no_aspect")
    else:
        row["fused_aspect"] = fused["aspect"]
        if "hy3d_aspect" in row:
            delta = abs(row["hy3d_aspect"] - fused["aspect"]) / fused["aspect"]
            row["aspect_delta"] = delta
            if delta > ASPECT_FLAG:
                row["flags"].append(f"aspect_off_{delta * 100:.0f}pct")

    # keyed on the newest view file so older partial render sets refresh
    if not (d / "render_front.png").exists():
        if not render_views(d / "mesh.obj", d):
            row["flags"].append("render_failed")

    # profile-only fused shapes need an aspect to reconstruct an outline;
    # with neither polygon nor aspect there is nothing drawable
    drawable = fused is not None and (fused.get("polygon") or fused.get("aspect"))
    if drawable and not (d / "outline_fused.png").exists():
        try:
            rings = (polygon_rings(fused["polygon"]) if fused.get("polygon")
                     else profile_rings(fused))
            draw_outline(rings, d / "outline_fused.png")
        except Exception as exc:
            print(f"  fused outline failed ({stem}): {exc}")
    if fused is not None:
        row["fused_source"] = fused.get("shape_source", "?")

    if fp_f.exists() and not (d / "outline_hy3d.png").exists():
        try:
            fp = json.loads(fp_f.read_text())
            draw_outline(polygon_rings(fp["polygon"]), d / "outline_hy3d.png",
                         frame="latlong")
        except Exception as exc:
            print(f"  hy3d outline failed: {exc}")

    return row


def cell_img(d: Path, name: str, cls: str) -> str:
    f = d / name
    return (f'<img class="{cls}" loading="lazy" src="{d.name}/{name}">'
            if f.exists() else '<div class="missing">—</div>')


def radio_cell(d: Path, stem: str, view: str) -> str:
    img = cell_img(d, f"render_{view}.png", "rend")
    # 'td' is the default frame: the generator's canonical top-down is
    # right for essentially every upright hero, so extraction now defaults
    # to it and the radios mark exceptions
    checked = " checked" if view == "td" else ""
    return (f'<td class="cand">{img}<label><input type="radio" '
            f'name="top_{stem}" value="{view}"{checked}> top</label></td>')


def html_row(row: dict, d: Path) -> str:
    flagged = bool(row["flags"])
    checked = " checked" if flagged else ""
    fa = row.get("fused_aspect")
    ha = row.get("hy3d_aspect")
    delta = row.get("aspect_delta")
    stats = (f'hy3d {ha:.2f}' if ha is not None else 'hy3d —')
    stats += (f' / fused {fa:.2f}' if fa is not None else ' / fused —')
    if delta is not None:
        stats += f' <b>Δ{delta * 100:.0f}%</b>'
    flags = " ".join(f'<span class="flag">{f}</span>' for f in row["flags"])
    src = row.get("fused_source", "")
    stem = row["stem"]
    # suggested bow-stern stretch = what would reconcile the mesh with the
    # pipeline's aspect; shown as a hint, applied only if typed into the box
    suggest = (f' <span class="hint">(fused suggests ×{fa / ha:.2f})</span>'
               if fa and ha else "")
    return f"""<tr class="{'flagged' if flagged else 'ok'}" data-stem="{stem}">
<td><input type="checkbox" class="pick"{checked}></td>
<td class="name">{stem}<div class="stats">{stats}</div><div class="flags">{flags}</div>
<div class="adj">stretch <input type="number" class="stretch" value="1.00" min="0.5" max="2.0" step="0.05">{suggest}
<label><input type="checkbox" class="sym"> sym</label>
<label><input type="radio" name="top_{stem}" value="pca"> extractor</label></div></td>
<td><a href="file:///home/robert/Downloads/chromakeys/{stem}.png" target="_blank" title="open source hero art">{cell_img(d, 'keyed.png', 'hero')}</a></td>
<td>{cell_img(d, 'render_tq.png', 'rend')}</td>
{radio_cell(d, stem, 'td')}
{radio_cell(d, stem, 'side')}
{radio_cell(d, stem, 'front')}
<td>{cell_img(d, 'outline_fused.png', 'outl')}<div class="src">{src}</div></td>
<td>{cell_img(d, 'outline_hy3d.png', 'outl')}</td>
</tr>"""


HTML_HEAD = """<!doctype html><meta charset="utf-8"><title>Hy3D sweep triage</title>
<style>
body { background:#181a1f; color:#cfd3da; font:14px/1.4 system-ui, sans-serif; margin:16px; }
table { border-collapse:collapse; }
td { border-bottom:1px solid #2a2e36; padding:6px 8px; vertical-align:middle; }
tr.flagged { background:#241c1c; }
.name { min-width:220px; font-weight:600; }
.stats { font-weight:400; color:#9aa1ab; }
.flag { background:#7a3030; color:#f0d0d0; border-radius:3px; padding:1px 5px; margin-right:4px; font-size:11px; font-weight:400; }
.src { color:#6a7280; font-size:11px; }
.adj { margin-top:6px; font-weight:400; color:#9aa1ab; font-size:12px; }
.adj input.stretch { width:60px; background:#111; color:#cfd3da; border:1px solid #444; border-radius:3px; }
.hint { color:#6a7280; }
.cand { text-align:center; }
.cand label { display:block; font-size:11px; color:#9aa1ab; cursor:pointer; }
img.hero { max-height:110px; max-width:160px; }
img.rend { height:110px; }
img.outl { width:200px; cursor:pointer; }
img.outl.flip { transform:scaleX(-1); }
.missing { color:#555; text-align:center; width:120px; }
#bar { position:sticky; top:0; background:#181a1f; padding:8px 0; z-index:2; }
button { background:#2c313a; color:#cfd3da; border:1px solid #444; border-radius:4px; padding:5px 12px; margin-right:8px; cursor:pointer; }
#queue { width:100%; height:70px; background:#111; color:#9fd49f; border:1px solid #333; margin-top:8px; }
th { text-align:left; padding:4px 8px; color:#8a919c; position:sticky; top:44px; background:#181a1f; }
</style>
<div id="bar">
<button onclick="filter('all')">all</button>
<button onclick="filter('flagged')">flagged only</button>
<button onclick="filter('checked')">checked only</button>
<button onclick="queue()">build Meshy queue from checked</button>
<button onclick="adjustments()">export adjustments</button>
<button onclick="importAdj()">import adjustments from box</button>
<span id="count"></span>
<textarea id="queue" placeholder="checked stems / adjustment JSON appear here"></textarea>
</div>
<table>
<tr><th></th><th>ship</th><th>hero</th><th>hy3d ¾</th><th>view A</th><th>view B</th><th>view C</th><th>KB fused outline</th><th>hy3d footprint</th></tr>
"""

HTML_TAIL = """</table>
<script>
function rows(){ return [...document.querySelectorAll('tr[data-stem]')]; }
function filter(mode){
  rows().forEach(r=>{
    const f=r.classList.contains('flagged'), c=r.querySelector('.pick').checked;
    r.style.display = (mode==='all'||(mode==='flagged'&&f)||(mode==='checked'&&c))?'':'none';
  }); count();
}
function queue(){
  const q=rows().filter(r=>r.querySelector('.pick').checked).map(r=>r.dataset.stem);
  document.getElementById('queue').value=q.join('\\n');
  document.getElementById('queue').select(); document.execCommand('copy');
}
function adjustments(){
  const out={};
  rows().forEach(r=>{
    const stem=r.dataset.stem;
    const top=r.querySelector('input[name="top_'+stem+'"]:checked');
    const st=parseFloat(r.querySelector('.stretch').value);
    const adj={};
    // 'td' is the extraction default now; export only deviations from it
    if(top && top.value!=='td') adj.top_view=top.value;
    if(st && Math.abs(st-1.0)>0.001) adj.stretch=st;
    if(r.querySelector('.sym').checked) adj.sym=true;
    const outl=[...r.querySelectorAll('img.outl')];
    if(outl.length&&outl[outl.length-1].classList.contains('flip')) adj.flip=true;
    if(Object.keys(adj).length) out[stem]=adj;
  });
  document.getElementById('queue').value=JSON.stringify(out,null,1);
  document.getElementById('queue').select(); document.execCommand('copy');
}
function count(){
  const v=rows().filter(r=>r.style.display!=='none');
  document.getElementById('count').textContent=
    v.length+' shown / '+rows().filter(r=>r.querySelector('.pick').checked).length+' checked';
}
rows().forEach(r=>r.querySelector('.pick').addEventListener('change',count));

// ---- persistence: every change autosaves to localStorage (keyed by ship),
// restored on load, so refreshes and sheet rebuilds keep the triage state
const STORE_KEY='hy3d_triage_v1';
let store=JSON.parse(localStorage.getItem(STORE_KEY)||'{}');
function saveRow(r){
  const stem=r.dataset.stem;
  const top=r.querySelector('input[name="top_'+stem+'"]:checked');
  const outl=[...r.querySelectorAll('img.outl')];
  store[stem]={p:r.querySelector('.pick').checked?1:0,
               s:r.querySelector('.stretch').value,
               y:r.querySelector('.sym').checked?1:0,
               t:top?top.value:'pca',
               f:outl.map(im=>im.classList.contains('flip')?1:0)};
  localStorage.setItem(STORE_KEY,JSON.stringify(store));
}
function applyRow(r,d){
  r.querySelector('.pick').checked=!!d.p;
  if(d.s) r.querySelector('.stretch').value=d.s;
  r.querySelector('.sym').checked=!!d.y;
  const t=r.querySelector('input[name="top_'+r.dataset.stem+'"][value="'+(d.t||'pca')+'"]');
  if(t) t.checked=true;
  if(d.f) [...r.querySelectorAll('img.outl')].forEach((im,i)=>
    im.classList.toggle('flip',!!d.f[i]));
}
function importAdj(){
  let adj;
  try{ adj=JSON.parse(document.getElementById('queue').value); }
  catch(e){ alert('paste exported adjustments JSON into the box first'); return; }
  rows().forEach(r=>{
    const a=adj[r.dataset.stem]; if(!a) return;
    const d={p:1,s:a.stretch||'1.00',y:a.sym?1:0,t:a.top_view||'pca'};
    applyRow(r,d); saveRow(r);
  });
  count();
}
rows().forEach(r=>{
  const d=store[r.dataset.stem];
  if(d) applyRow(r,d);
  r.addEventListener('change',()=>saveRow(r));
  r.querySelector('.stretch').addEventListener('input',()=>saveRow(r));
  r.querySelectorAll('img.outl').forEach(im=>im.addEventListener('click',()=>{
    im.classList.toggle('flip'); saveRow(r);
  }));
});
count();
</script>
"""


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", default="out-hy3d-full")
    ap.add_argument("--limit", type=int, default=0,
                    help="only process the first N ships (for a preview run)")
    args = ap.parse_args()

    root = HERE / args.dir
    dirs = sorted(d for d in root.iterdir() if (d / "mesh.obj").exists())
    if args.limit:
        dirs = dirs[:args.limit]

    body = []
    n_flagged = 0
    for i, d in enumerate(dirs):
        row = build_row(d)
        n_flagged += bool(row["flags"])
        body.append(html_row(row, d))
        if (i + 1) % 25 == 0:
            print(f"{i + 1}/{len(dirs)}")

    out = root / "triage.html"
    out.write_text(HTML_HEAD + "\n".join(body) + HTML_TAIL)
    print(f"{len(dirs)} ships, {n_flagged} auto-flagged -> {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
