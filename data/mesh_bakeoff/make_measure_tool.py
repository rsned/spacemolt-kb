#!/usr/bin/env python3
"""Build measure_windows.html — the cockpit-window measuring tool.

The blueprint scale model (data/footprints/scale/) needs a real-world
anchor per ship: the height in hero-image pixels of ONE window pane,
assumed to be 1 m tall (2 m pilot, half-height window — the sci-fi
convention). This bakes the roster (art stems from ship_id_map.json that
have a hero in the chromakeys drop and map to a KB ship) into a static
page served off the local 8478 server. Clicks persist to localStorage;
"export" downloads window_px.json for data/footprints/scale/.

    python3 make_measure_tool.py        -> measure_windows.html
"""
import json
import sqlite3
from pathlib import Path

HERE = Path(__file__).resolve().parent
KB = HERE.parent.parent / "spacemolt-knowledge.db"
HEROES = HERE / "chromakeys"


def roster():
    id_map = json.loads((HERE / "ship_id_map.json").read_text())["mapping"]
    db = sqlite3.connect(KB)
    ships = {r[0]: {"name": r[1], "cls": r[2] or "", "scale": r[3]}
             for r in db.execute("select id,name,class,scale from ships")}
    rows = []
    for stem, m in sorted(id_map.items(),
                          key=lambda kv: (kv[1]["faction"], kv[0])):
        s = ships.get(m["id"])
        if s is None or not (HEROES / f"{stem}.png").exists():
            continue
        rows.append({"stem": stem, "id": m["id"], "faction": m["faction"],
                     "name": s["name"], "cls": s["cls"], "scale": s["scale"]})
    return rows


def main():
    rows = roster()
    html = TEMPLATE.replace("__ROSTER__", json.dumps(rows))
    out = HERE / "measure_windows.html"
    out.write_text(html)
    print(f"{out}  {len(rows)} ships")


TEMPLATE = r"""<!doctype html><meta charset="utf-8">
<title>window measure — blueprint scale anchors</title>
<style>
  body { background:#0c0e12; color:#9aa1ab; font:13px system-ui; margin:0;
         display:flex; flex-direction:column; height:100vh; }
  #bar { display:flex; gap:14px; align-items:baseline; padding:8px 14px;
         background:#12151b; border-bottom:1px solid #232833; flex-wrap:wrap }
  #bar b { color:#e8ecf2; font-size:15px }
  #bar .dim { color:#5f6a78 }
  #view { flex:1; overflow:hidden; position:relative; cursor:crosshair;
          background:repeating-conic-gradient(#101318 0 25%,#0c0e12 0 50%) 0 0/24px 24px }
  #world { position:absolute; transform-origin:0 0 }
  #world img { display:block; image-rendering:pixelated; user-select:none;
               -webkit-user-drag:none }
  .mark { position:absolute; left:0; width:100%; height:0;
          border-top:1.6px solid #ff5b8d; pointer-events:none }
  .mark.b { border-color:#3dd6ff }
  #help { padding:6px 14px; color:#5f6a78; background:#12151b;
          border-top:1px solid #232833 }
  button { background:#1c2028; color:#9aa1ab; border:1px solid #333a46;
           border-radius:4px; padding:4px 10px; cursor:pointer }
  .done { color:#7fd98f } .skip { color:#c9a44d }
</style>
<div id="bar">
  <b id="who"></b><span id="meta" class="dim"></span>
  <span id="state"></span>
  <span id="prog" class="dim"></span>
  <span style="flex:1"></span>
  <button id="prev">&larr; p</button><button id="next">n &rarr;</button>
  <button id="none">no window (x)</button><button id="undo">undo (u)</button>
  <button id="export">export json (e)</button>
</div>
<div id="view"><div id="world"><img id="img" draggable="false"></div></div>
<div id="help">click the TOP of one window pane, then the BOTTOM (~1 m of real
height — one pane, not a stacked strip) · wheel = zoom · drag = pan ·
n/p next/prev unmeasured · x = no usable window · u = clear · e = export</div>
<script>
const ROSTER = __ROSTER__;
const LS = s => "smkb_win_" + s;
let cur = 0, scale0 = 1;
let tx = 0, ty = 0, zoom = 1;
const view = document.getElementById("view"), world = document.getElementById("world");
const img = document.getElementById("img");

function rec(stem) {
  const v = localStorage.getItem(LS(stem));
  return v ? JSON.parse(v) : null;
}
function save(stem, r) { localStorage.setItem(LS(stem), JSON.stringify(r)); ui(); }

function apply() { world.style.transform = `translate(${tx}px,${ty}px) scale(${zoom})`; }

function show(i, dir) {
  cur = (i + ROSTER.length) % ROSTER.length;
  const s = ROSTER[cur];
  img.src = "chromakeys/" + s.stem + ".png";
  img.onload = () => {
    const fit = Math.min(view.clientWidth / img.naturalWidth,
                         view.clientHeight / img.naturalHeight);
    zoom = fit; tx = (view.clientWidth - img.naturalWidth * fit) / 2;
    ty = (view.clientHeight - img.naturalHeight * fit) / 2; apply();
  };
  document.getElementById("who").textContent = s.stem;
  document.getElementById("meta").textContent =
    ` ${s.name} · ${s.cls} · ${s.faction} · scale ${s.scale}`;
  ui();
}
function ui() {
  const s = ROSTER[cur], r = rec(s.stem);
  const el = document.getElementById("state");
  world.querySelectorAll(".mark").forEach(m => m.remove());
  if (r && r.flag === "none") { el.textContent = "marked: no window"; el.className = "skip"; }
  else if (r && r.h) {
    el.textContent = `window = ${r.h.toFixed(1)} px`; el.className = "done";
    for (const [y, c] of [[r.y0, ""], [r.y1, "b"]]) {
      const m = document.createElement("div");
      m.className = "mark " + c; m.style.top = y + "px"; world.appendChild(m);
    }
  } else if (pending !== null) { el.textContent = "…now click the BOTTOM"; el.className = ""; }
  else { el.textContent = "unmeasured"; el.className = "dim"; }
  const done = ROSTER.filter(s => rec(s.stem)).length;
  document.getElementById("prog").textContent =
    ` ${done}/${ROSTER.length} measured (${cur + 1} shown)`;
}

// pan/zoom -----------------------------------------------------------
let drag = null, moved = 0, pending = null;
view.addEventListener("pointerdown", e => {
  drag = { x: e.clientX, y: e.clientY }; moved = 0;
  view.setPointerCapture(e.pointerId);
});
view.addEventListener("pointermove", e => {
  if (!drag) return;
  const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
  moved += Math.abs(dx) + Math.abs(dy);
  tx += dx; ty += dy; drag = { x: e.clientX, y: e.clientY }; apply();
});
view.addEventListener("pointerup", e => {
  drag = null;
  if (moved > 5) return;                     // was a pan, not a click
  const r = view.getBoundingClientRect();
  const yi = (e.clientY - r.top - ty) / zoom, xi = (e.clientX - r.left - tx) / zoom;
  const s = ROSTER[cur];
  if (pending === null) {
    localStorage.removeItem(LS(s.stem));
    pending = { y0: yi, x: xi }; ui();
    const m = document.createElement("div");
    m.className = "mark"; m.style.top = yi + "px"; world.appendChild(m);
  } else {
    save(s.stem, { y0: +pending.y0.toFixed(1), y1: +yi.toFixed(1),
                   x: +pending.x.toFixed(1),
                   h: +Math.abs(yi - pending.y0).toFixed(1) });
    pending = null;
    setTimeout(() => nextUnmeasured(1), 450);
  }
});
view.addEventListener("wheel", e => {
  e.preventDefault();
  const f = e.deltaY < 0 ? 1.18 : 1 / 1.18;
  const r = view.getBoundingClientRect();
  const px = e.clientX - r.left, py = e.clientY - r.top;
  tx = px - (px - tx) * f; ty = py - (py - ty) * f; zoom *= f; apply();
}, { passive: false });

function nextUnmeasured(dir) {
  for (let k = 1; k <= ROSTER.length; k++) {
    const i = (cur + dir * k + ROSTER.length) % ROSTER.length;
    if (!rec(ROSTER[i].stem)) { show(i); return; }
  }
  show(cur + dir);                            // all measured: plain step
}

document.getElementById("next").onclick = () => { pending = null; nextUnmeasured(1); };
document.getElementById("prev").onclick = () => { pending = null; nextUnmeasured(-1); };
document.getElementById("none").onclick = () =>
  { pending = null; save(ROSTER[cur].stem, { flag: "none" }); setTimeout(() => nextUnmeasured(1), 300); };
document.getElementById("undo").onclick = () =>
  { pending = null; localStorage.removeItem(LS(ROSTER[cur].stem)); ui(); };
document.getElementById("export").onclick = () => {
  const out = {};
  for (const s of ROSTER) { const r = rec(s.stem); if (r) out[s.stem] = r; }
  const blob = new Blob([JSON.stringify(out, null, 1)], { type: "application/json" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob); a.download = "window_px.json"; a.click();
};
addEventListener("keydown", e => {
  if (e.key === "n") document.getElementById("next").onclick();
  else if (e.key === "p") document.getElementById("prev").onclick();
  else if (e.key === "x") document.getElementById("none").onclick();
  else if (e.key === "u") document.getElementById("undo").onclick();
  else if (e.key === "e") document.getElementById("export").onclick();
});

// seed from any committed window_px.json so prior sessions carry over
fetch("window_px.json").then(r => r.ok ? r.json() : {}).then(seed => {
  for (const [stem, r] of Object.entries(seed))
    if (!localStorage.getItem(LS(stem))) localStorage.setItem(LS(stem), JSON.stringify(r));
}).catch(() => {}).finally(() => show(0));
</script>
"""

if __name__ == "__main__":
    main()
