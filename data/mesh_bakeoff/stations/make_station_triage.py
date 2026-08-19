#!/usr/bin/env python3
"""Incremental triage sheet for the station Hy3D bake.

For every gallery cell whose mesh exists in ../out-stations/, render three
point-splat views (cached beside the mesh — no GL needed, safe to run while
the bake holds the GPU), then build triage.html: hero, keyed input, mesh
views, and a star + note per row persisted in localStorage with a JSON
export button. Re-run any time; it only renders views that are missing, so
the sheet grows as the bake progresses.

    ~/sf3d-venv/bin/python make_station_triage.py
"""

import json
from pathlib import Path

import numpy as np
import trimesh
from PIL import Image

from gen_stations import ARCHETYPES, EMPIRES, PIRATES, SIZES, STRONGHOLDS

HERE = Path(__file__).resolve().parent
OUT = HERE.parent / "out-stations"
RES, N_SAMPLES = 560, 300000
VIEWS = {"tq": (25, 35), "side": (0, 90), "td": (89, 0)}


def splat(pts, nrm, elev, azim):
    e, a = np.radians(elev), np.radians(azim)
    fwd = np.array([np.cos(e) * np.sin(a), np.sin(e), np.cos(e) * np.cos(a)])
    right = np.cross([0.0, 1.0, 0.0], fwd)
    n = np.linalg.norm(right)
    right = np.array([1.0, 0.0, 0.0]) if n < 1e-6 else right / n
    up = np.cross(fwd, right)
    xy = np.column_stack([pts @ right, -(pts @ up)])
    order = np.argsort(pts @ fwd)
    px = ((xy * 0.45 + 0.5) * RES).astype(int).clip(0, RES - 1)
    light = fwd * 0.8 + up * 0.5 + right * 0.3
    light /= np.linalg.norm(light)
    shade = (nrm @ light).clip(0, 1) * 0.75 + 0.2
    img = np.zeros((RES, RES), np.float32)
    img[px[order, 1], px[order, 0]] = shade[order]
    return np.maximum.reduce([np.roll(img, s, axis=(0, 1))
                              for s in ((0, 0), (1, 0), (0, 1), (1, 1))])


def ensure_views(stem: str) -> bool:
    d = OUT / stem
    if not (d / "mesh.obj").exists():
        return False
    if all((d / f"view_{v}.png").exists() for v in VIEWS):
        return True
    mesh = trimesh.load(d / "mesh.obj", force="mesh")
    pts, fi = trimesh.sample.sample_surface(mesh, N_SAMPLES)
    nrm = mesh.face_normals[fi]
    pts = np.asarray(pts) - np.asarray(pts).mean(axis=0)
    pts /= np.abs(pts).max()
    for v, (elev, azim) in VIEWS.items():
        g = splat(pts, nrm, elev, azim)
        Image.fromarray((g * 255).astype(np.uint8)).save(d / f"view_{v}.png")
    print(f"views {stem}")
    return True


def cells() -> list[tuple[str, str, str]]:
    """(section, label, stem) in gen_stations order."""
    out = []
    for ei, emp in enumerate(EMPIRES):
        for ai, arch in enumerate(ARCHETYPES):
            for si, size in enumerate(SIZES):
                seed = 5000 + ei * 100 + ai * 10 + si
                out.append((emp, f"{arch} · {size}", f"{emp}__{arch}__{size}_s{seed}"))
    for pi, (pid, (_, arch)) in enumerate(PIRATES.items()):
        for si, size in enumerate(SIZES):
            out.append(("pirate factions", f"{pid} · {size}",
                        f"pirate_{pid}__{arch}__{size}_s{5900 + pi * 10 + si}"))
    for pi, (pid, (where, _, arch)) in enumerate(STRONGHOLDS.items()):
        for i in range(3):
            out.append(("strongholds", f"{pid} ({where}) · v{i}",
                        f"stronghold_{pid}__{arch}__v{i}_s{6000 + pi * 10 + i}"))
    return out


HEAD = """<!doctype html><meta charset="utf-8"><title>Station bake triage</title>
<style>
body { background:#181a1f; color:#cfd3da; font:13px system-ui,sans-serif; margin:20px; }
h1 { font-size:18px; } h2 { font-size:15px; margin:26px 0 6px; color:#9fd9ff; }
table { border-collapse:collapse; }
td, th { padding:6px; border-bottom:1px solid #2a2e36; vertical-align:top; text-align:left; }
img { height:170px; background:#20242b; border-radius:4px; display:block; }
.lbl { font-weight:600; width:150px; } .miss { color:#5f6a78; }
input[type=text] { background:#1c2028; color:#cfd3da; border:1px solid #333a46;
  border-radius:4px; padding:4px 6px; width:140px; }
input[type=checkbox] { transform:scale(1.5); margin:8px; }
#export { position:fixed; top:12px; right:16px; background:#22384c; color:#9fd9ff;
  border:1px solid #3d6d94; border-radius:4px; padding:6px 12px; cursor:pointer; }
</style>
<button id="export">export stars</button>
<h1>Station bake triage</h1>
<div class="miss">star + note persist in localStorage; export dumps JSON.
Re-run make_station_triage.py to pull in newly baked meshes.</div>
"""

SCRIPT = """<script>
for (const el of document.querySelectorAll('[data-k]')) {
  const k = 'smkb_stn_' + el.dataset.k;
  if (el.type === 'checkbox') { el.checked = localStorage.getItem(k) === '1';
    el.onchange = () => localStorage.setItem(k, el.checked ? '1' : '0'); }
  else { el.value = localStorage.getItem(k) || '';
    el.oninput = () => localStorage.setItem(k, el.value); }
}
document.getElementById('export').onclick = () => {
  const out = {};
  for (const el of document.querySelectorAll('[data-k]')) {
    const v = el.type === 'checkbox' ? (el.checked ? 1 : 0) : el.value;
    if (v) out[el.dataset.k] = v;
  }
  navigator.clipboard.writeText(JSON.stringify(out, null, 1));
  alert('copied to clipboard');
};
</script>"""


def main() -> int:
    rows_by_section: dict[str, list[str]] = {}
    done = 0
    for sec, label, stem in cells():
        have = ensure_views(stem)
        done += have
        art = stem.rsplit("_s", 1)[0]
        imgs = (f'<td><img loading="lazy" src="renders/{stem}.png"></td>'
                f'<td><img loading="lazy" src="../out-stations/{stem}/keyed.png"></td>')
        if have:
            imgs += "".join(
                f'<td><img loading="lazy" src="../out-stations/{stem}/view_{v}.png"></td>'
                for v in VIEWS)
        else:
            imgs += '<td colspan="3" class="miss">not baked yet</td>'
        ctl = (f'<td><input type="checkbox" data-k="{stem}.star">'
               f'<input type="text" data-k="{stem}.note" placeholder="note"></td>')
        rows_by_section.setdefault(sec, []).append(
            f'<tr><td class="lbl">{label}</td>{imgs}{ctl}</tr>')
    # re-rolls: baked dirs that aren't grid cells (seed base+2.. convention)
    known = {stem for _, _, stem in cells()}
    for d in sorted(OUT.iterdir()):
        if not d.is_dir() or d.name in known or not (d / "mesh.obj").exists():
            continue
        stem = d.name
        ensure_views(stem)
        imgs = (f'<td><img loading="lazy" src="renders/{stem}.png"></td>'
                f'<td><img loading="lazy" src="../out-stations/{stem}/keyed.png"></td>'
                + "".join(
                    f'<td><img loading="lazy" src="../out-stations/{stem}/view_{v}.png"></td>'
                    for v in VIEWS))
        ctl = (f'<td><input type="checkbox" data-k="{stem}.star">'
               f'<input type="text" data-k="{stem}.note" placeholder="note"></td>')
        rows_by_section.setdefault("re-rolls", []).append(
            f'<tr><td class="lbl">{stem}</td>{imgs}{ctl}</tr>')
    parts = [HEAD]
    for sec, rows in rows_by_section.items():
        parts.append(f"<h2>{sec}</h2><table><tr><th></th><th>hero</th><th>keyed</th>"
                     "<th>&frac34;</th><th>side</th><th>top-down</th><th>star / note</th></tr>"
                     + "".join(rows) + "</table>")
    parts.append(SCRIPT)
    (HERE / "triage.html").write_text("\n".join(parts))
    print(f"triage.html — {done} baked of {len(cells())} cells")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
