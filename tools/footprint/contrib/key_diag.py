#!/usr/bin/env python3
"""Diagnose the 35 failed_unkeyable images: what does the border actually look
like in hue space? Is 'magenta-ish hue anywhere in the image' a safe key?"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib
import sys

import cv2
import numpy as np

sys.path.insert(0, _REPO)
from tools.footprint import matte, run as fprun  # noqa: E402

CHROMA = pathlib.Path.home() / "Downloads" / "chromakeys"
REPORT = pathlib.Path(_REPO + "/data/footprints/report.json")

report = json.load(open(REPORT))
unk = sorted(x["id"] for x in report["results"] if x["status"] == "failed_unkeyable")

heroes = fprun.resolve_heroes()  # id -> image path

print(f"{'ship':22s} {'spread':>7s} {'bstd':>6s} {'bord_hue':>8s} {'hue_iqr':>7s} "
      f"{'bord_sat':>8s} {'ship_magfrac':>12s}")
rows = []
for sid in unk:
    p = heroes.get(sid)
    if p is None:
        print(f"{sid:22s} NO IMAGE")
        continue
    bgr = cv2.imread(str(p), cv2.IMREAD_COLOR)
    rgb = cv2.cvtColor(bgr, cv2.COLOR_BGR2RGB)
    _, spread, bstd = matte.keyability(rgb)

    hsv = cv2.cvtColor(rgb, cv2.COLOR_RGB2HSV).astype(np.float64)  # H in [0,180)
    h, w = rgb.shape[:2]
    border = np.concatenate([hsv[0], hsv[-1], hsv[:, 0], hsv[:, -1]], axis=0)
    bh = border[:, 0]
    # circular-safe: magenta sits ~150 in cv2 half-degrees, far from wrap at 0/180? magenta ~300deg -> 150
    bord_hue = float(np.median(bh))
    hue_iqr = float(np.percentile(bh, 95) - np.percentile(bh, 5))
    bord_sat = float(np.median(border[:, 1]))

    # how much of the CURRENT matte's foreground is magenta-hued (would a hue
    # key eat ship pixels?)
    mask, frac = matte.extract(rgb)
    fg = hsv[mask.astype(bool)]
    if len(fg):
        mag = np.abs(fg[:, 0] - bord_hue) < 12
        magsat = mag & (fg[:, 1] > 80)
        ship_magfrac = float(magsat.mean())
    else:
        ship_magfrac = float("nan")
    print(f"{sid:22s} {spread:7.1f} {bstd:6.1f} {bord_hue:8.1f} {hue_iqr:7.1f} "
          f"{bord_sat:8.1f} {ship_magfrac:12.4f}")
    rows.append((sid, spread, bstd, bord_hue, hue_iqr, bord_sat, ship_magfrac))

json.dump(rows, open(pathlib.Path.cwd() / "key_diag.json", "w"), indent=1)
