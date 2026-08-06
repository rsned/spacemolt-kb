#!/usr/bin/env python3
"""Per-ship estimated viewing direction, in the hull's own frame.

Rebuilds the geometric frame the pipeline itself uses (lateral = symmetry
normal, longitudinal = principal axis within the plane, up = lateral x long)
from cloud_resolved.npz, then decomposes the camera view axis (+Z) in it:

  yaw    0   = bow/stern-on      90 = full broadside
  elev   0   = level side shot   90 = straight top-down
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib

import numpy as np

WT = pathlib.Path(_REPO)
FOOT = WT / "data/footprints"

report = json.loads((FOOT / "report.json").read_text())
print(f"{'ship':22s} {'status':26s} {'yaw':>5s} {'elev':>5s} {'obliquity':>9s}")
for r in sorted(report["results"], key=lambda x: x["id"]):
    npz = FOOT / r["id"] / "cloud_resolved.npz"
    if not npz.exists():
        continue
    d = np.load(npz)
    pts, lateral = d["points"], d["normal"] / np.linalg.norm(d["normal"])
    inplane = pts - np.outer(pts @ lateral, lateral)
    inplane -= inplane.mean(axis=0)
    _, _, vt = np.linalg.svd(inplane[:: max(1, len(inplane) // 20000)], full_matrices=False)
    longitudinal = vt[0] / np.linalg.norm(vt[0])
    up = np.cross(lateral, longitudinal)

    view = np.array([0.0, 0.0, 1.0])  # camera looks down +Z
    lat, lon, u = (abs(view @ lateral), abs(view @ longitudinal), abs(view @ up))
    yaw = np.degrees(np.arctan2(lat, lon))
    elev = np.degrees(np.arcsin(np.clip(u, 0, 1)))
    q = r.get("quality", {}) or {}
    ob = q.get("obliquity")
    if ob is None:
        qj = FOOT / r["id"] / "quality.json"
        if qj.exists():
            ob = json.loads(qj.read_text()).get("obliquity")
    print(f"{r['id']:22s} {r['status']:26s} {yaw:5.0f} {elev:5.0f} "
          f"{ob if ob is None else f'{ob:9.3f}'}")
