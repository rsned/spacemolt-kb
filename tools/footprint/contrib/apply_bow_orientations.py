#!/usr/bin/env python3
"""Resolve each profile's bow end from the user's clock annotations and store
profiles NOSE-FIRST (t=0 = bow), with provenance.

Chain (replicates the production stage 6-7 frame exactly):
  sym, full <- mirror.load          (solved plane + completed cloud)
  up        <- ground.up_vector     (same call _process_phase_b makes)
  sym_xy    <- ground.project(sym.normal)
  axis2     <- (-sym_xy[1], sym_xy[0])   # canonicalise's +X == increasing t
The user's clock angle (deg CW from screen-up) gives an image-space bow
direction d = (sin a, -cos a). The solved longitudinal axis is signed by
matching its image projection to d, projected to the ground basis, and
compared against axis2: dot > 0 means the bow sits at t=1, so w/concave are
reversed to put the nose at t=0. orientation: "unknown" -> "bow_t0".

Skips (reported, untouched): no profile, no annotation, longitudinal nearly
perpendicular to the image plane, or weak screen alignment (<0.3).
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib
import sys

import numpy as np

sys.path.insert(0, _REPO)
from tools.footprint import camera, ground, mirror, pointmap  # noqa: E402

FOOT = pathlib.Path(_REPO + "/data/footprints")
ANN = json.loads((FOOT / "user_annotations_2026-08-01.json").read_text())
BOWS = ANN["bow_directions_deg_cw_from_screen_up"]
ALIGN_FLOOR = 0.3

report_path = FOOT / "report.json"
report = json.loads(report_path.read_text())
by_id = {r["id"]: r for r in report["results"]}

done, skipped = [], []
for sid, angle in sorted(BOWS.items()):
    rec = by_id.get(sid)
    pjp = FOOT / sid / "profile.json"
    if rec is None or not pjp.exists():
        skipped.append((sid, "no profile"))
        continue
    pj = json.loads(pjp.read_text())
    if pj.get("orientation") not in (None, "unknown"):
        skipped.append((sid, f"already {pj['orientation']}"))
        continue
    try:
        sym, full = mirror.load(sid)
        cloud = pointmap.load(sid)
        cj = json.loads((FOOT / sid / "camera.json").read_text())
        fit = camera.Fit(R=np.array(cj["R"]), focal=cj["focal"],
                         principal=tuple(cj["principal"]), confidence=cj["confidence"],
                         source=cj["source"], n_segments=cj["n_segments"],
                         inliers=tuple(cj["inliers"]), ortho=cj["ortho"])
        up = ground.up_vector(sym, full, cloud.normals, fit=fit)

        lateral = np.asarray(sym.normal, float)
        lateral /= np.linalg.norm(lateral)
        centred = full - full.mean(axis=0)
        in_plane = centred - np.outer(centred @ lateral, lateral)
        lon = np.linalg.svd(in_plane[:: max(1, len(in_plane) // 20000)],
                            full_matrices=False)[2][0]
        lon /= np.linalg.norm(lon)

        a = np.radians(angle)
        d_img = np.array([np.sin(a), -np.cos(a)])       # camera x right, y down
        l_img = lon[:2]
        if np.linalg.norm(l_img) < 1e-6:
            skipped.append((sid, "longitudinal ⟂ image plane"))
            continue
        align = float(l_img / np.linalg.norm(l_img) @ d_img)
        if abs(align) < ALIGN_FLOOR:
            skipped.append((sid, f"weak alignment {align:+.2f} (clock {angle})"))
            continue
        bow3d = lon if align > 0 else -lon

        sym_xy = ground.project(sym.normal[None, :], up)[0]
        n2 = sym_xy / np.linalg.norm(sym_xy)
        axis2 = np.array([-n2[1], n2[0]])               # canonicalise's +X
        bow_xy = ground.project(bow3d[None, :], up)[0]
        bow_at_t1 = float(bow_xy @ axis2) > 0

        if bow_at_t1:
            pj["w"] = pj["w"][::-1]
            pj["concave"] = pj["concave"][::-1]
        pj["orientation"] = "bow_t0"
        pj["orientation_source"] = {
            "method": "user_bow_annotation",
            "clock_angle_deg_cw_from_screen_up": angle,
            "screen_alignment": round(abs(align), 3),
            "flipped_from_stored": bool(bow_at_t1),
            "date": "2026-08-01",
        }
        pjp.write_text(json.dumps(pj, indent=2))
        rec.update({k: pj[k] for k in ("w", "concave", "orientation")})
        rec["orientation_source"] = pj["orientation_source"]
        done.append((sid, angle, abs(align), bow_at_t1))
    except Exception as e:  # noqa: BLE001
        skipped.append((sid, f"{type(e).__name__}: {e}"))

report_path.write_text(json.dumps(report, indent=2))

print(f"oriented {len(done)} profiles nose-first; {len(skipped)} skipped")
flips = sum(1 for *_, f in done if f)
print(f"flipped {flips}/{len(done)} (stored bow was at t=1)")
aligns = [al for _, _, al, _ in done]
print(f"screen alignment: median {np.median(aligns):.2f}, min {min(aligns):.2f}")
print("\nskips:")
for sid, why in skipped:
    print(f"  {sid:24s} {why}")
