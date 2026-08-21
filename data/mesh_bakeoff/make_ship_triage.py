#!/usr/bin/env python3
"""Single-ship triage page for the shipwright one-off pipeline.

Written to <dir>/<stem>/triage.html (inside the gitignored sweep output),
served off the :8478 mesh_bakeoff server. One page = every human verdict
the pipeline needs, exported as ONE handoff file for `shipwright.py finish`:

  * top-view pick (td / side / front / pca candidates, from the renders)
  * rot90 / flip (bow direction) / mirror / vflip / sym / solo, stretch
  * cockpit-window click (top + bottom of ONE ~1 m pane on the hero; the
    click x doubles as the bridge station for the deck plan), or the
    "no usable window" flag

Missing renders are generated first (render_mesh splatter), so run in the
venv with trimesh:

    ~/hy3d-venv/bin/python make_ship_triage.py <stem> [--dir out-hy3d-full]
"""
import argparse
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("stem")
    ap.add_argument("--dir", default="out-hy3d-full")
    args = ap.parse_args()

    d = HERE / args.dir / args.stem
    if not (d / "mesh.obj").exists():
        print(f"no mesh at {d}/mesh.obj — run the bake first")
        return 1
    if not (d / "render_front.png").exists():
        from make_triage_sheet import render_views
        render_views(d / "mesh.obj", d)

    adj = json.loads((HERE / "adjustments-final.json").read_text()
                     ).get(args.stem, {})
    html = TEMPLATE.replace("__STEM__", args.stem) \
                   .replace("__ADJ__", json.dumps(adj))
    (d / "triage.html").write_text(html)
    rel = f"{args.dir}/{args.stem}/triage.html"
    print(f"{d / 'triage.html'}\nserve:  http://localhost:8478/{rel}")
    return 0


TEMPLATE = r"""<!doctype html><meta charset="utf-8">
<title>shipwright triage — __STEM__</title>
<style>
  body { background:#0c0e12; color:#9aa1ab; font:13px system-ui; margin:0;
         display:flex; flex-direction:column; height:100vh }
  #bar { display:flex; gap:12px; align-items:center; padding:8px 14px;
         background:#12151b; border-bottom:1px solid #232833; flex-wrap:wrap }
  #bar b { color:#e8ecf2; font-size:15px }
  main { flex:1; display:flex; min-height:0 }
  #view { flex:1; overflow:hidden; position:relative; cursor:crosshair;
          background:repeating-conic-gradient(#101318 0 25%,#0c0e12 0 50%) 0 0/24px 24px }
  #world { position:absolute; transform-origin:0 0 }
  #world img { display:block; image-rendering:pixelated; user-select:none;
               -webkit-user-drag:none }
  .mark { position:absolute; left:0; width:100%; height:0;
          border-top:1.6px solid #ff5b8d; pointer-events:none }
  .mark.b { border-color:#3dd6ff }
  aside { width:340px; overflow-y:auto; background:#12151b;
          border-left:1px solid #232833; padding:12px 14px }
  aside h3 { color:#e8ecf2; font-size:12px; text-transform:uppercase;
             letter-spacing:1px; margin:14px 0 6px }
  .cand { display:inline-block; text-align:center; margin:0 6px 6px 0;
          cursor:pointer }
  .cand img { width:96px; display:block; background:#181a1f; border-radius:4px;
              border:2px solid transparent }
  .cand input:checked + img { border-color:#7aa7d8 }
  .cand input { display:none }
  label.t { display:block; margin:4px 0; cursor:pointer }
  input[type=number] { width:64px; background:#111; color:#cfd3da;
                       border:1px solid #333a46; border-radius:3px }
  button { background:#1c2028; color:#cfd3da; border:1px solid #333a46;
           border-radius:4px; padding:6px 12px; cursor:pointer }
  #export { background:#24432b; border-color:#3c6b47; color:#c9e8cf }
  .done { color:#7fd98f } .dim { color:#5f6a78 }
  #help { padding:6px 14px; color:#5f6a78; background:#12151b;
          border-top:1px solid #232833 }
</style>
<div id="bar"><b>__STEM__</b><span id="wstate" class="dim">window: unmeasured</span>
<span style="flex:1"></span>
<button id="wnone">no usable window</button><button id="wundo">clear window</button>
<button id="export">export verdict json</button></div>
<main>
<div id="view"><div id="world"><img id="img" src="keyed.png" draggable="false"></div></div>
<aside>
  <h3>top view</h3>
  <div id="cands">
    <label class="cand"><input type="radio" name="top" value="td" checked>
      <img src="render_td.png"><span>td</span></label>
    <label class="cand"><input type="radio" name="top" value="side">
      <img src="render_side.png"><span>side</span></label>
    <label class="cand"><input type="radio" name="top" value="front">
      <img src="render_front.png"><span>front</span></label>
    <label class="cand"><input type="radio" name="top" value="pca">
      <img src="render_tq.png"><span>pca (extractor)</span></label>
  </div>
  <h3>hull toggles</h3>
  <label class="t"><input type="checkbox" id="rot90"> rot90 — swap length/width (near-square hulls)</label>
  <label class="t"><input type="checkbox" id="flip"> flip — bow front-to-back</label>
  <label class="t"><input type="checkbox" id="mirror"> mirror — port/starboard swap</label>
  <label class="t"><input type="checkbox" id="vflip"> vflip — dorsal/ventral swap (side view upside down)</label>
  <label class="t"><input type="checkbox" id="sym"> sym — mirror-union a lopsided reconstruction</label>
  <label class="t"><input type="checkbox" id="solo"> solo — keep main hull only (drop companion bodies)</label>
  <label class="t">stretch <input type="number" id="stretch" value="1.00"
    min="0.5" max="2.0" step="0.05"> bow-stern proportion fix</label>
  <label class="t">roll <input type="number" id="roll" value="0"
    min="-45" max="45" step="0.5">° manual de-roll for lopsided hulls
    (overrides the symmetry fit; + rotates the front view CCW)</label>
  <label class="t">clean <input type="number" id="clean" value="0"
    min="0" max="10" step="0.5"> sliver/noise removal dose (0 = off;
    erases features thinner than 2× this in hull-length ‰)</label>
  <h3>current footprint</h3>
  <img src="outline_hy3d.png" style="width:100%;border-radius:4px">
  <p class="dim">reflects the LAST extraction — toggles apply when
  `shipwright.py finish` re-extracts.</p>
</aside>
</main>
<div id="help">hero: click the TOP then the BOTTOM of one window pane (~1 m);
the click site doubles as the bridge station · wheel = zoom · drag = pan ·
verdicts autosave; export writes shipwright___STEM__.json for
`shipwright.py finish`</div>
<script>
const STEM="__STEM__", LS="smkb_shipwright_"+STEM;
const PRIOR_ADJ=__ADJ__;
let st = JSON.parse(localStorage.getItem(LS) || "null") ||
         { adj: PRIOR_ADJ, window: null };
const view=document.getElementById("view"), world=document.getElementById("world");
const img=document.getElementById("img");
let tx=0, ty=0, zoom=1, drag=null, moved=0, pending=null;

function save(){ localStorage.setItem(LS, JSON.stringify(st)); ui(); }
function apply(){ world.style.transform=`translate(${tx}px,${ty}px) scale(${zoom})`; }
img.onload=()=>{ const f=Math.min(view.clientWidth/img.naturalWidth,
  view.clientHeight/img.naturalHeight);
  zoom=f; tx=(view.clientWidth-img.naturalWidth*f)/2;
  ty=(view.clientHeight-img.naturalHeight*f)/2; apply(); };

function ui(){
  const a=st.adj||{};
  document.querySelector(`input[name=top][value=${a.top_view||"td"}]`).checked=true;
  for (const k of ["rot90","flip","mirror","vflip","sym","solo"])
    document.getElementById(k).checked=!!a[k];
  document.getElementById("stretch").value=(a.stretch||1).toFixed(2);
  document.getElementById("roll").value=a.roll||0;
  document.getElementById("clean").value=a.clean||0;
  world.querySelectorAll(".mark").forEach(m=>m.remove());
  const w=st.window, el=document.getElementById("wstate");
  if (w && w.flag==="none"){ el.textContent="window: none usable"; el.className=""; }
  else if (w && w.h){ el.textContent=`window: ${w.h.toFixed(1)} px`; el.className="done";
    for (const [y,c] of [[w.y0,""],[w.y1,"b"]]){
      const m=document.createElement("div");
      m.className="mark "+c; m.style.top=y+"px"; world.appendChild(m); } }
  else if (pending!==null){ el.textContent="…now click the BOTTOM"; el.className=""; }
  else { el.textContent="window: unmeasured"; el.className="dim"; }
}
function readAdj(){
  const a={};
  const top=document.querySelector("input[name=top]:checked").value;
  if (top!=="td") a.top_view=top;
  for (const k of ["rot90","flip","mirror","vflip","sym","solo"])
    if (document.getElementById(k).checked) a[k]=true;
  const s=parseFloat(document.getElementById("stretch").value);
  if (s && Math.abs(s-1)>0.001) a.stretch=s;
  const r=parseFloat(document.getElementById("roll").value);
  if (r) a.roll=r;
  const c=parseFloat(document.getElementById("clean").value);
  if (c>0) a.clean=c;
  st.adj=a; save();
}
document.querySelectorAll("aside input").forEach(i=>i.addEventListener("change",readAdj));

view.addEventListener("pointerdown",e=>{ drag={x:e.clientX,y:e.clientY}; moved=0;
  view.setPointerCapture(e.pointerId); });
view.addEventListener("pointermove",e=>{ if(!drag)return;
  const dx=e.clientX-drag.x, dy=e.clientY-drag.y; moved+=Math.abs(dx)+Math.abs(dy);
  tx+=dx; ty+=dy; drag={x:e.clientX,y:e.clientY}; apply(); });
view.addEventListener("pointerup",e=>{
  drag=null; if(moved>5)return;
  const r=view.getBoundingClientRect();
  const yi=(e.clientY-r.top-ty)/zoom, xi=(e.clientX-r.left-tx)/zoom;
  if (pending===null){ st.window=null; pending={y0:yi,x:xi}; ui();
    const m=document.createElement("div");
    m.className="mark"; m.style.top=yi+"px"; world.appendChild(m); }
  else { st.window={y0:+pending.y0.toFixed(1), y1:+yi.toFixed(1),
                    x:+pending.x.toFixed(1),
                    h:+Math.abs(yi-pending.y0).toFixed(1)};
         pending=null; save(); } });
view.addEventListener("wheel",e=>{ e.preventDefault();
  const f=e.deltaY<0?1.18:1/1.18, r=view.getBoundingClientRect();
  const px=e.clientX-r.left, py=e.clientY-r.top;
  tx=px-(px-tx)*f; ty=py-(py-ty)*f; zoom*=f; apply(); },{passive:false});

document.getElementById("wnone").onclick=()=>{ pending=null;
  st.window={flag:"none"}; save(); };
document.getElementById("wundo").onclick=()=>{ pending=null;
  st.window=null; save(); };
document.getElementById("export").onclick=()=>{
  readAdj();
  const out={stem:STEM, adjustments:st.adj, window:st.window};
  const blob=new Blob([JSON.stringify(out,null,1)],{type:"application/json"});
  const a=document.createElement("a");
  a.href=URL.createObjectURL(blob); a.download=`shipwright_${STEM}.json`; a.click();
};
ui();
</script>
"""

if __name__ == "__main__":
    raise SystemExit(main())
