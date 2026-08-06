#!/usr/bin/env python3
"""Throwaway grading preview: recovered profile vs procedural glyph, per ship.

For each recovered ship: parse the glyph SVG's path coordinates, build a
96-station half-width profile of the drawn hull (nose-up, length vertical),
normalise to hull-length units, and compare against the recovered profile.json
w(t) — Pearson r over both orientations (bow/stern is unknown in the recovery,
so the better flip wins and is reported).
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib
import re

import numpy as np

WT = pathlib.Path(_REPO)
FOOT = WT / "data/footprints"
GLYPHS = pathlib.Path(_REPO + "/kb/ships/glyphs")
STATIONS = 96
COORD = re.compile(r"[-+]?\d*\.?\d+")


def glyph_profile(svg_text):
    """96-station half-widths of the drawn glyph, in hull-length units."""
    pts = []
    for d in re.findall(r'\bd="([^"]+)"', svg_text) + re.findall(r"\bd='([^']+)'", svg_text):
        nums = [float(x) for x in COORD.findall(d)]
        pts.extend(zip(nums[0::2], nums[1::2]))
    pts = np.array(pts)
    ymin, ymax = pts[:, 1].min(), pts[:, 1].max()
    length = ymax - ymin
    cx = (pts[:, 0].min() + pts[:, 0].max()) / 2.0
    t = (pts[:, 1] - ymin) / length
    half = np.abs(pts[:, 0] - cx) / length
    w = np.zeros(STATIONS)
    idx = np.clip((t * (STATIONS - 1)).round().astype(int), 0, STATIONS - 1)
    for i, h in zip(idx, half):
        w[i] = max(w[i], h)
    # fill empty stations by linear interpolation over occupied ones
    occ = w > 0
    if occ.sum() >= 2:
        w[~occ] = np.interp(np.flatnonzero(~occ), np.flatnonzero(occ), w[occ])
    return w


def pearson(a, b):
    a, b = np.asarray(a), np.asarray(b)
    if a.std() == 0 or b.std() == 0:
        return float("nan")
    return float(np.corrcoef(a, b)[0, 1])


report = json.loads((FOOT / "report.json").read_text())
rows = []
for r in sorted(report["results"], key=lambda x: x["id"]):
    if not r["status"].startswith("ok"):
        continue
    sid = r["id"]
    pj = json.loads((FOOT / sid / "profile.json").read_text())
    w_rec = np.array(pj["w"], dtype=float)
    w_gly = glyph_profile((GLYPHS / f"{sid}.svg").read_text())

    gly_aspect = 1.0 / (2.0 * w_gly.max()) if w_gly.max() > 0 else float("nan")
    r_fwd = pearson(w_rec, w_gly)
    r_rev = pearson(w_rec[::-1], w_gly)
    best, orient = (r_fwd, "fwd") if r_fwd >= r_rev else (r_rev, "rev")
    rows.append((sid, pj["aspect"], gly_aspect, best, orient, r_fwd, r_rev))

print(f"{'ship':22s} {'rec_aspect':>10s} {'gly_aspect':>10s} {'ratio':>6s} "
      f"{'best_r':>7s} {'orient':>6s} {'r_fwd':>7s} {'r_rev':>7s}")
for sid, ra, ga, br, orient, rf, rr in sorted(rows, key=lambda x: -x[3]):
    print(f"{sid:22s} {ra:10.2f} {ga:10.2f} {ra/ga:6.2f} "
          f"{br:7.3f} {orient:>6s} {rf:7.3f} {rr:7.3f}")

aspects_rec = np.array([x[1] for x in rows])
aspects_gly = np.array([x[2] for x in rows])
print(f"\nships: {len(rows)}")
print(f"aspect correlation (recovered vs glyph, across ships): "
      f"{pearson(aspects_rec, aspects_gly):.3f}")
print(f"median per-ship profile r (best orientation): "
      f"{np.median([x[3] for x in rows]):.3f}")
print(f"orientation flips chosen: {sum(1 for x in rows if x[4] == 'rev')}/{len(rows)}")
