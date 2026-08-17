#!/usr/bin/env python3
"""Export the triaged Hy3D footprints as consumable SVGs + a gallery page.

One SVG per ship from its (adjusted) footprint.json polygon, presented the
same way the triage sheet draws outlines: canonical frame (x = bow-stern),
bow to the RIGHT via the wedge heuristic, then the user's recorded `flip`
verdicts from adjustments-final.json applied on top -- so the SVGs are the
human-approved orientation, not just the heuristic's.

Consumer contract (attributes on <svg>):
    data-ship             sweep stem (faction_shipid, anomalies as-is)
    data-aspect           length/width from the stage-7 profile sampler
    data-frame-ambiguous  extraction confidence flag
    data-adjustments      JSON of the applied human corrections ({} if none)
Geometry: single <path> with fill-rule="evenodd" (holes are real: lattice
hulls, tendril gaps). Coordinates: hull length normalised to 1000 units,
centred; bow at +x; y grows downward (SVG convention), starboard up.
Scale is RELATIVE -- absolute ship size is not known to this pipeline.

    ~/hy3d-venv/bin/python make_svg_footprints.py [--dir out-hy3d-full] [--out ../footprints/hy3d-svg]
"""

import argparse
import json
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent

LENGTH = 1000.0  # normalised hull length in SVG units
MARGIN = 10.0


def rings_of(geo) -> list[tuple[np.ndarray, bool]]:
    polys = [geo["coordinates"]] if geo["type"] == "Polygon" else geo["coordinates"]
    return [(np.asarray(r, dtype=float), i > 0)
            for poly in polys for i, r in enumerate(poly)]


def orient(rings, user_flip: bool):
    """Extraction frame (x=lat, y=long) -> presentation frame: bow right,
    wedge heuristic first, then the human flip verdict."""
    arrs = [r[:, ::-1] for r, _ in rings]          # swap -> x = bow-stern
    allp = np.vstack(arrs)
    ctr = (allp.min(axis=0) + allp.max(axis=0)) / 2
    arrs = [r - ctr for r in arrs]
    allp = np.vstack(arrs)
    x, y = allp[:, 0], allp[:, 1]
    lo, hi = x.min(), x.max()
    end = 0.2 * (hi - lo)
    left_w = np.abs(y[x < lo + end]).max() if (x < lo + end).any() else 0
    right_w = np.abs(y[x > hi - end]).max() if (x > hi - end).any() else 0
    flip = left_w < right_w                        # narrow end left -> flip
    if user_flip:
        flip = not flip
    if flip:
        arrs = [r * np.array([-1.0, 1.0]) for r in arrs]
    return arrs, [h for _, h in rings]


def svg_for(stem: str, fp: dict, prof: dict, adj: dict) -> str:
    arrs, holes = orient(rings_of(fp["polygon"]), bool(adj.get("flip")))
    allp = np.vstack(arrs)
    span = allp.max(axis=0) - allp.min(axis=0)
    s = LENGTH / span[0]
    w = LENGTH + 2 * MARGIN
    h = span[1] * s + 2 * MARGIN
    off = np.array([w / 2, h / 2])

    parts = []
    for r in arrs:
        px = r * s + off
        d = "M" + "L".join(f"{p[0]:.1f} {p[1]:.1f}" for p in px) + "Z"
        parts.append(d)
    path = "".join(parts)

    meta = {k: v for k, v in adj.items()}
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w:.0f} {h:.0f}"\n'
        f'  data-ship="{stem}" data-aspect="{prof["aspect"]:.4f}"\n'
        f'  data-frame-ambiguous="{str(prof["frame_ambiguous"]).lower()}"\n'
        f"  data-adjustments='{json.dumps(meta)}'>\n"
        f'<title>{stem}</title>\n'
        f'<path d="{path}" fill-rule="evenodd" fill="#d5d8dd"/>\n'
        f'</svg>\n'
    )


GALLERY_HEAD = """<!doctype html><meta charset="utf-8"><title>Hy3D footprint gallery</title>
<style>
body { background:#181a1f; color:#cfd3da; font:13px system-ui,sans-serif; margin:20px; }
h1 { font-size:18px; } .sub { color:#8a919c; margin-bottom:14px; }
#q { background:#111; color:#cfd3da; border:1px solid #444; border-radius:4px; padding:5px 10px; width:280px; margin-bottom:14px; }
.grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(230px,1fr)); gap:10px; }
.card { background:#20242b; border-radius:6px; padding:8px; }
.card img { width:100%; height:110px; object-fit:contain; background:#181a1f; border-radius:4px; }
.name { font-weight:600; margin-top:6px; word-break:break-all; }
.meta { color:#8a919c; font-size:11px; }
</style>
<h1>Hy3D footprint gallery</h1>
<div class="sub">%COUNT% ships — top-down hull footprints from the triaged Hunyuan3D sweep.
Bow right, hull length normalised to 1000 units, fill-rule evenodd (holes are real).
Aspect + adjustment provenance are data- attributes on each SVG.</div>
<input id="q" placeholder="filter by name...">
<div class="grid">
"""

GALLERY_TAIL = """</div>
<script>
document.getElementById('q').addEventListener('input', e => {
  const v = e.target.value.toLowerCase();
  document.querySelectorAll('.card').forEach(c =>
    c.style.display = c.dataset.name.includes(v) ? '' : 'none');
});
</script>
"""


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", default="out-hy3d-full")
    ap.add_argument("--out", default=str(HERE.parent / "footprints" / "hy3d-svg"))
    args = ap.parse_args()

    adjustments = json.loads((HERE / "adjustments-final.json").read_text())
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)

    cards = []
    n = 0
    for d in sorted((HERE / args.dir).iterdir()):
        fp_f, prof_f = d / "footprint.json", d / "profile.json"
        if not (fp_f.exists() and prof_f.exists()):
            continue
        stem = d.name
        fp = json.loads(fp_f.read_text())
        prof = json.loads(prof_f.read_text())
        adj = adjustments.get(stem, {})
        try:
            (out / f"{stem}.svg").write_text(svg_for(stem, fp, prof, adj))
        except Exception as exc:
            print(f"{stem:32} FAILED {type(exc).__name__}: {exc}")
            continue
        n += 1
        adj_txt = " ".join(f"{k}={v}" for k, v in adj.items()) or "—"
        cards.append(
            f'<div class="card" data-name="{stem}"><img loading="lazy" src="{stem}.svg">'
            f'<div class="name">{stem}</div>'
            f'<div class="meta">aspect {prof["aspect"]:.2f} · {adj_txt}</div></div>')

    (out / "gallery.html").write_text(
        GALLERY_HEAD.replace("%COUNT%", str(n)) + "\n".join(cards) + GALLERY_TAIL)
    print(f"{n} SVGs + gallery.html -> {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
