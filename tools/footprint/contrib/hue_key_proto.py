#!/usr/bin/env python3
"""Prototype: hue+saturation chroma key vs the current RGB-distance key.

Background = pixels whose hue is within HUE_TOL of the border's median hue AND
saturation >= SAT_FLOOR (shadowed key keeps hue+sat, only value drops).
Keyability = border is overwhelmingly key-hued (fraction of border pixels
passing the same background test).

Runs on: all 35 failed_unkeyable + a regression sample of previously-ok ships
(new matte must agree with old matte, IoU). Writes side-by-side previews.
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib
import sys

import cv2
import numpy as np

sys.path.insert(0, _REPO)
from tools.footprint import matte, run as fprun  # noqa: E402

SCRATCH = pathlib.Path.cwd()
OUT = SCRATCH / "hue_key_preview"
OUT.mkdir(exist_ok=True)

HUE_TOL = 14.0      # cv2 half-degrees (=28 deg)
SAT_FLOOR = 60.0    # 0-255
BORDER_KEY_FRACTION = 0.98  # keyable iff >=98% of border pixels are key-hued


def hue_key(rgb):
    hsv = cv2.cvtColor(rgb, cv2.COLOR_RGB2HSV).astype(np.float32)
    border = np.concatenate([hsv[0], hsv[-1], hsv[:, 0], hsv[:, -1]], axis=0)
    key_hue = float(np.median(border[:, 0]))

    dh = np.abs(hsv[..., 0] - key_hue)
    dh = np.minimum(dh, 180.0 - dh)  # circular
    is_bg = (dh <= HUE_TOL) & (hsv[..., 1] >= SAT_FLOOR)

    bdh = np.abs(border[:, 0] - key_hue)
    bdh = np.minimum(bdh, 180.0 - bdh)
    border_ok = float(((bdh <= HUE_TOL) & (border[:, 1] >= SAT_FLOOR)).mean())
    is_keyable = border_ok >= BORDER_KEY_FRACTION

    mask = (~is_bg).astype(np.uint8)
    kernel = np.ones((5, 5), np.uint8)
    mask = cv2.morphologyEx(mask, cv2.MORPH_OPEN, kernel)
    mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)
    n, labels, stats, _ = cv2.connectedComponentsWithStats(mask, connectivity=8)
    if n > 1:
        areas = stats[1:, cv2.CC_STAT_AREA]
        mask = (labels == 1 + int(np.argmax(areas))).astype(np.uint8)
    filled = mask.copy()
    h, w = mask.shape
    flood = np.zeros((h + 2, w + 2), np.uint8)
    cv2.floodFill(filled, flood, (0, 0), 1)
    mask = (mask | (1 - filled)).astype(np.uint8)
    return mask, float(mask.mean()), is_keyable, border_ok, key_hue


def preview(rgb, old_mask, new_mask, path):
    h, w = rgb.shape[:2]
    def tint(m):
        vis = rgb.copy()
        vis[m == 0] = (vis[m == 0] * 0.25).astype(np.uint8)
        return vis
    row = np.concatenate([rgb, tint(old_mask), tint(new_mask)], axis=1)
    cv2.imwrite(str(path), cv2.cvtColor(row, cv2.COLOR_RGB2BGR))


report = json.load(open(_REPO + "/data/footprints/report.json"))
by_status = {}
for x in report["results"]:
    by_status.setdefault(x["status"], []).append(x["id"])

heroes = fprun.resolve_heroes()

print("=== 35 failed_unkeyable under the hue key ===")
print(f"{'ship':22s} {'keyable':>7s} {'bord_ok':>7s} {'frac':>6s} {'old_frac':>8s}")
recovered = 0
for sid in sorted(by_status.get("failed_unkeyable", [])):
    p = heroes.get(sid)
    rgb = cv2.cvtColor(cv2.imread(str(p)), cv2.COLOR_BGR2RGB)
    old_mask, old_frac = matte.extract(rgb)
    new_mask, frac, keyable, border_ok, key_hue = hue_key(rgb)
    recovered += keyable
    print(f"{sid:22s} {str(keyable):>7s} {border_ok:7.3f} {frac:6.3f} {old_frac:8.3f}")
    preview(rgb, old_mask, new_mask, OUT / f"unk_{sid}.png")
print(f"keyable now: {recovered}/35")

print("\n=== regression: previously-ok ships, new-vs-old matte IoU ===")
ok_ids = sorted(by_status.get("ok_asymmetric", []))
sample = ok_ids[:: max(1, len(ok_ids) // 20)][:20]
print(f"{'ship':22s} {'keyable':>7s} {'iou':>6s} {'frac':>6s} {'old_frac':>8s}")
worst = []
for sid in sample:
    p = heroes.get(sid)
    rgb = cv2.cvtColor(cv2.imread(str(p)), cv2.COLOR_BGR2RGB)
    old_mask, old_frac = matte.extract(rgb)
    new_mask, frac, keyable, border_ok, key_hue = hue_key(rgb)
    inter = float((old_mask & new_mask).sum())
    union = float((old_mask | new_mask).sum())
    iou = inter / union if union else float("nan")
    worst.append((iou, sid))
    print(f"{sid:22s} {str(keyable):>7s} {iou:6.3f} {frac:6.3f} {old_frac:8.3f}")
    if iou < 0.98:
        preview(rgb, old_mask, new_mask, OUT / f"reg_{sid}.png")
worst.sort()
print(f"min IoU: {worst[0][1]} {worst[0][0]:.3f}; median: {np.median([w[0] for w in worst]):.3f}")
