#!/usr/bin/env python3
"""Demo: 'squaring' transform for TripoSR profiles — flatten rounded wide
bumps toward their plateau max so blocky engine sections read as blocks.

Method: work on the 0.67-narrowed mesh w(t). Find the wide region (stations
with w >= WIDE_FRAC * max(w)); within each contiguous wide run, snap stations
to the run's max where they already reach SNAP_FRAC of it — turning a rounded
bump into a flat-topped block with steep shoulders — and leave everything
else untouched. Renders before/after pairs for eyeball judgement.
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib

import numpy as np

SCRATCH = pathlib.Path.cwd()
BAKEOFF = pathlib.Path(_REPO + "/data/mesh_bakeoff/out-full")
OUT = pathlib.Path.cwd() / "square_demo.html"
BOX = 190
WIDE_FRAC = 0.80    # a station is "wide" if within 80% of the global max beam
SNAP_FRAC = 0.85    # inside a wide run, snap anything >= 85% of run max up to it

SHIPS = ["ledger", "magnate", "crimson_war_wagon", "outerrim_yard_sale",
         "nebula_bonanza", "voidborn_paradox", "solarian_principia", "datum"]


import sys
sys.path.insert(0, _REPO)
from tools.footprint import profile as fpp  # noqa: E402


def square(w):
    # the prototype graduated: this now IS profile.snap_plateaus
    return fpp.snap_plateaus(w, wide_frac=WIDE_FRAC, snap_frac=SNAP_FRAC)


def path(w, flip):
    w = np.asarray(w, dtype=float)
    if flip:
        w = w[::-1]
    t = np.linspace(0, 1, len(w))
    L = BOX * 0.85
    y0 = (BOX - L) / 2
    xs = BOX / 2 + np.concatenate([w, -w[::-1]]) * L
    ys = y0 + np.concatenate([t, t[::-1]]) * L
    return "M " + " L ".join(f"{x:.1f} {y:.1f}" for x, y in zip(xs, ys)) + " Z"


cards = []
for stem in SHIPS:
    pj = BAKEOFF / stem / "profile.json"
    if not pj.exists():
        continue
    w = np.array(json.loads(pj.read_text())["w"], dtype=float) * 0.67
    half = len(w) // 2
    flip = w[:half].mean() > w[half:].mean()   # wider end at bottom, like the sheet
    sq = square(w)
    cards.append(f"""
<div class='card'><h3>{stem}</h3><div class='row'>
<figure><svg viewBox='0 0 {BOX} {BOX}'><path d='{path(w, flip)}' fill='#c9a86a'
 fill-opacity='0.25' stroke='#c9a86a' stroke-width='1.2'/></svg>
 <figcaption>mesh &times;0.67 (current)</figcaption></figure>
<figure><svg viewBox='0 0 {BOX} {BOX}'><path d='{path(sq, flip)}' fill='#8fd0a0'
 fill-opacity='0.25' stroke='#8fd0a0' stroke-width='1.2'/></svg>
 <figcaption>squared</figcaption></figure>
</div></div>""")

OUT.write_text(f"""<!doctype html><meta charset="utf-8"><title>Squaring demo</title>
<style>body{{font:14px system-ui;background:#14161a;color:#dde3ea;margin:24px}}
.card{{display:inline-block;border:1px solid #2a2f38;border-radius:10px;
padding:10px 14px;margin:8px;background:#1a1e24}}
.row{{display:flex;gap:10px}} figure{{margin:0;text-align:center}}
svg{{width:{BOX}px;height:{BOX}px;background:#11141a;border-radius:6px}}
figcaption{{font-size:11px;color:#8b96a5;margin-top:4px}}</style>
<h1>Mesh profile squaring — before / after</h1>
<p>WIDE_FRAC={WIDE_FRAC}, SNAP_FRAC={SNAP_FRAC}: rounded wide bumps snap to
flat-topped blocks; narrow sections untouched.</p>
{''.join(cards)}""")
print(f"wrote {OUT} ({len(cards)} ships)")
