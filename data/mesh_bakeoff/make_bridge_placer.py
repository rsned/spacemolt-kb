#!/usr/bin/env python3
"""Interactive bridge/cockpit placement page for the blueprint deck plans.

One card per mapped ship: the keyed hero (with the window-survey click
lines overlaid, plus the click column) next to the top-view footprint
SVG with a draggable BRIDGE/COCKPIT node. Click or drag anywhere on the
footprint to place the node; a per-card button toggles bridge vs
cockpit. Placements autosave to localStorage; EXPORT downloads
bridge_positions.json for the whole fleet, which the user drops at

    data/footprints/blueprints/bridge_positions.json   (committed)

and reruns make_blueprints.py. The node stores only the STATION (tx,
fraction of the footprint viewBox width, bow right): bridges/cockpits
always sit on the bow-stern axis, so the sheet snaps the room to the
interior centerline at that station and vertical drag is ignored.

Prefill order: existing bridge_positions.json entry > cockpit-class
heuristic (nose) > fore-oriented window station max(t, 1-t) > 0.72.
The hero click fraction alone cannot be trusted for direction: heroes
come bow-left AND bow-right, and neither the Hy3D canonical frame nor
hero-silhouette IoU matching resolves the side better than ~70%.

    python3 make_bridge_placer.py        (writes bridge_placer.html)
"""
import json
import sqlite3
from pathlib import Path

from PIL import Image

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
BPOS = REPO / "data" / "footprints" / "blueprints" / "bridge_positions.json"
BT = REPO / "data" / "footprints" / "scale" / "bridge_t.json"

# small-crew hulls whose "bridge" is a 1-2 seat canopy at the nose
COCKPIT_CLASSES = {"Fighter", "Heavy Fighter", "Interceptor", "Shuttle",
                   "Scout", "Patrol", "Courier", "Raider", "Customs Patrol",
                   "Recon"}


def main() -> int:
    idmap = json.loads((HERE / "ship_id_map.json").read_text())["mapping"]
    dup = {d["stem"] for d in json.loads(
        (HERE / "ship_id_map.json").read_text())["duplicates"]}
    window = json.loads((HERE / "window_px.json").read_text())
    bt = json.loads(BT.read_text()) if BT.exists() else {}
    prev = json.loads(BPOS.read_text()) if BPOS.exists() else {}

    db = sqlite3.connect(REPO / "spacemolt-knowledge.db")
    ships = {r[0]: (r[1], r[2] or "", r[3] or 0) for r in db.execute(
        "select id,name,class,scale from ships")}

    cards = []
    for stem, m in sorted(idmap.items(), key=lambda kv: kv[1]["id"]):
        sid = m["id"]
        if stem in dup or sid not in ships:
            continue
        if not (REPO / "data" / "footprints" / "hy3d-svg" / f"{sid}.svg").exists():
            continue
        name, cls, scale = ships[sid]
        is_ck = cls in COCKPIT_CLASSES or scale <= 1

        if sid in prev:
            tx = prev[sid].get("tx", 0.72)
            kind = prev[sid].get("kind", "cockpit" if is_ck else "bridge")
        else:
            kind = "cockpit" if is_ck else "bridge"
            t = bt.get(stem)
            if kind == "cockpit":
                tx = 0.88
            elif t is not None:
                tx = round(min(max(max(t, 1.0 - t), 0.45), 0.90), 2)
            else:
                tx = 0.72

        # window click lines as % of the hero image
        marks = ""
        w = window.get(stem) or {}
        hero = HERE / "chromakeys" / f"{stem}.png"
        if hero.exists() and w.get("h"):
            iw, ih = Image.open(hero).size
            marks = (f'<i class="wl" style="top:{100*w["y0"]/ih:.1f}%"></i>'
                     f'<i class="wl b" style="top:{100*w["y1"]/ih:.1f}%"></i>'
                     f'<i class="wv" style="left:{100*w["x"]/iw:.1f}%"></i>')

        cards.append(f'''<div class="card" id="c_{sid}" data-name="{sid} {name.lower()} {cls.lower()}">
<div class="hd"><b>{name.upper()}</b><span class="m">{cls.upper()} · SCALE {scale}</span>
<span style="flex:1"></span><button class="kind"></button><button class="rst">reset</button></div>
<div class="row">
<div class="hero"><span class="hw"><img loading="lazy" src="chromakeys/{stem}.png">{marks}</span></div>
<div class="fp"><img loading="lazy" src="fpsvg/{sid}.svg"><div class="node"><span></span></div></div>
</div></div>''')

        cards.append(f'<script>DEF["{sid}"]={{tx:{tx},kind:"{kind}"}};</script>')

    html = (TEMPLATE
            .replace("__CARDS__", "\n".join(cards))
            .replace("__N__", str(sum(1 for c in cards if c.startswith("<div")))))
    out = HERE / "bridge_placer.html"
    out.write_text(html)
    print(f"{out}\nserve:  http://localhost:8478/bridge_placer.html")
    return 0


TEMPLATE = r"""<!doctype html><meta charset="utf-8">
<title>bridge placer</title>
<style>
  body { background:#0c0e12; color:#9aa1ab; font:13px system-ui; margin:0 }
  #bar { position:sticky; top:0; z-index:5; display:flex; gap:12px;
         align-items:center; padding:8px 14px; background:#12151b;
         border-bottom:1px solid #232833 }
  #bar b { color:#e8ecf2; font-size:15px }
  #bar input { background:#111; color:#cfd3da; border:1px solid #333a46;
               border-radius:4px; padding:4px 8px; width:220px }
  button { background:#1c2028; color:#cfd3da; border:1px solid #333a46;
           border-radius:4px; padding:4px 10px; cursor:pointer }
  #export { background:#24432b; border-color:#3c6b47; color:#c9e8cf }
  .card { margin:10px 14px; background:#12151b; border:1px solid #232833;
          border-radius:6px; overflow:hidden }
  .card.touched { border-color:#3c6b47 }
  .hd { display:flex; gap:10px; align-items:center; padding:6px 12px;
        border-bottom:1px solid #1a1e26 }
  .hd b { color:#e8ecf2 } .m { color:#5f6a78; letter-spacing:.08em }
  .row { display:flex; align-items:center; gap:14px; padding:10px 12px }
  .hero { flex:0 0 320px; text-align:center }
  .hw { position:relative; display:inline-block }
  .hw img { max-width:320px; max-height:150px; display:block }
  .wl { position:absolute; left:0; width:100%; height:0;
        border-top:1.4px solid #ff5b8d; pointer-events:none }
  .wl.b { border-color:#3dd6ff }
  .wv { position:absolute; top:0; height:100%; width:0;
        border-left:1.4px dashed #ffd75b; opacity:.7; pointer-events:none }
  .fp { position:relative; flex:0 0 460px; cursor:crosshair;
        background:#0e1a2e; border-radius:4px }
  .fp img { display:block; width:100%; user-select:none; -webkit-user-drag:none }
  .fp::after { content:""; position:absolute; left:0; top:50%; width:100%;
               height:0; border-top:1px dashed rgba(234,242,255,.25);
               pointer-events:none }
  .node { position:absolute; width:0; height:0; pointer-events:none }
  .node span { position:absolute; left:-9px; top:-9px; width:18px; height:18px;
               border-radius:50%; border:2.2px solid #ffb04d;
               background:rgba(255,176,77,.18); display:block }
  .node::after { content:attr(data-lb); position:absolute; left:12px; top:-7px;
                 font-size:10px; letter-spacing:.14em; color:#ffb04d;
                 white-space:nowrap }
  .node.ck span { border-color:#3dd6ff; background:rgba(61,214,255,.18) }
  .node.ck::after { color:#3dd6ff }
  .kind.ck { color:#3dd6ff; border-color:#28566b }
  .kind.br { color:#ffb04d; border-color:#6b5228 }
</style>
<div id="bar"><b>bridge placer</b><span id="prog"></span>
<input id="q" placeholder="filter name / class">
<span style="flex:1"></span>
<button id="clear">clear all placements</button>
<button id="export">export bridge_positions.json</button></div>
<script>const DEF={};</script>
__CARDS__
<script>
const LS="smkb_bridge_placer";
let st=JSON.parse(localStorage.getItem(LS)||"{}");
const cur=id=>st[id]||DEF[id];
function save(){ localStorage.setItem(LS,JSON.stringify(st)); prog(); }
function prog(){ document.getElementById("prog").textContent=
  `placed ${Object.keys(st).length}/__N__`; }

function paint(id){
  const c=document.getElementById("c_"+id), v=cur(id);
  const n=c.querySelector(".node"), k=c.querySelector(".kind");
  n.style.left=(v.tx*100)+"%"; n.style.top="50%";
  n.className="node"+(v.kind==="cockpit"?" ck":"");
  n.dataset.lb=v.kind==="cockpit"?"CKPT":"BRIDGE";
  k.textContent=v.kind.toUpperCase();
  k.className="kind "+(v.kind==="cockpit"?"ck":"br");
  c.classList.toggle("touched",id in st);
}

for (const id of Object.keys(DEF)){
  const c=document.getElementById("c_"+id);
  const fp=c.querySelector(".fp");
  let drag=false;
  const place=e=>{ const r=fp.getBoundingClientRect();
    const tx=Math.min(1,Math.max(0,(e.clientX-r.left)/r.width));
    st[id]={kind:cur(id).kind,tx:+tx.toFixed(3)}; save(); paint(id); };
  fp.addEventListener("pointerdown",e=>{ drag=true;
    fp.setPointerCapture(e.pointerId); place(e); });
  fp.addEventListener("pointermove",e=>{ if(drag) place(e); });
  fp.addEventListener("pointerup",()=>drag=false);
  c.querySelector(".kind").onclick=()=>{ const v=cur(id);
    st[id]={...v,kind:v.kind==="cockpit"?"bridge":"cockpit"}; save(); paint(id); };
  c.querySelector(".rst").onclick=()=>{ delete st[id]; save(); paint(id); };
  paint(id);
}
prog();

document.getElementById("q").oninput=e=>{
  const q=e.target.value.toLowerCase();
  document.querySelectorAll(".card").forEach(c=>
    c.style.display=c.dataset.name.includes(q)?"":"none");
};
document.getElementById("clear").onclick=()=>{
  if(Object.keys(st).length&&!confirm("drop ALL placements?"))return;
  st={}; save();
  for (const id of Object.keys(DEF)) paint(id); };
document.getElementById("export").onclick=()=>{
  const out={};
  for (const id of Object.keys(DEF)){ const v=cur(id);
    out[id]={tx:v.tx,kind:v.kind}; }
  const blob=new Blob([JSON.stringify(out,null,1)],{type:"application/json"});
  const a=document.createElement("a");
  a.href=URL.createObjectURL(blob); a.download="bridge_positions.json"; a.click();
};
</script>
"""

if __name__ == "__main__":
    raise SystemExit(main())
