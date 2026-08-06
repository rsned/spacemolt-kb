#!/usr/bin/env python3
"""Re-solve every ship's mirror plane with the upright-art prior, then re-run
stages 5-7, all from the on-disk stage 1-3 artifacts (mattes unchanged since
the hue-key batch, so cloud.npz is still exact — no MoGe needed).

Replicates _process_phase_a's post-solve gates (symmetry failure, obliquity
floor) verbatim, then hands a _Stage4Result to the production _process_phase_b.
Merges results into report.json (previous version saved alongside as
report.json.pre_upright). Parallel across ships (CPU-only solve).
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib
import shutil
import sys
import traceback
from concurrent.futures import ProcessPoolExecutor

WORKERS = 6
ALPHA = 8.0
REPO = _REPO
FOOT = pathlib.Path(REPO) / "data/footprints"


def solve_one(sid):
    import cv2
    import numpy as np
    sys.path.insert(0, REPO)
    from tools.footprint import camera, mirror, pointmap, run as fprun

    try:
        art = FOOT / sid
        if not (art / "cloud.npz").exists() or not (art / "matte.png").exists():
            return {"id": sid, "status": "failed_exception", "quality": {},
                    "reason": "missing stage 1-3 artifacts for re-solve"}
        mask = (cv2.imread(str(art / "matte.png"), cv2.IMREAD_GRAYSCALE) > 0).astype(np.uint8)
        cloud = pointmap.load(sid)
        cj = json.loads((art / "camera.json").read_text())
        fit = camera.Fit(R=np.array(cj["R"]), focal=cj["focal"],
                         principal=tuple(cj["principal"]), confidence=cj["confidence"],
                         source=cj["source"], n_segments=cj["n_segments"],
                         inliers=tuple(cj["inliers"]), ortho=cj["ortho"])

        sym = mirror.run(sid, cloud, mask)

        # _process_phase_a's post-solve gates, verbatim statuses and reasons.
        if sym.failure is not None:
            return fprun._fail(sid, "failed_symmetry_solve",
                               {"obliquity": sym.obliquity,
                                "depth_separation": sym.depth_separation,
                                "mirror_residual": sym.residual},
                               sym.failure)
        if sym.obliquity < mirror.OBLIQUITY_FLOOR:
            return fprun._fail(sid, "failed_low_obliquity",
                               {"obliquity": sym.obliquity,
                                "depth_separation": sym.depth_separation,
                                "mirror_residual": sym.residual},
                               f"obliquity={sym.obliquity:.3f} is below "
                               f"mirror.OBLIQUITY_FLOOR={mirror.OBLIQUITY_FLOOR}: near "
                               "bow-on, where the depth-separation term cannot reliably "
                               "distinguish a fold from a genuine occluded half")

        stage4 = fprun._Stage4Result(frac=float(mask.mean()), fit=fit, cloud=cloud,
                                     sym=sym, mask=mask)
        return fprun._process_phase_b(sid, stage4, ALPHA)
    except Exception as e:  # noqa: BLE001 - batch isolation, like _run_all
        (FOOT / sid / "profile.json").unlink(missing_ok=True)
        return {"id": sid, "status": "failed_exception", "quality": {},
                "reason": f"{type(e).__name__}: {e} | {traceback.format_exc(limit=2)}"}


def main():
    report_path = FOOT / "report.json"
    report = json.loads(report_path.read_text())
    ids = [r["id"] for r in report["results"]]
    print(f"re-solving {len(ids)} ships with the upright prior "
          f"({WORKERS} workers, alpha={ALPHA})", flush=True)

    results = {}
    done = 0
    with ProcessPoolExecutor(max_workers=WORKERS) as ex:
        for rec in ex.map(solve_one, ids):
            results[rec["id"]] = rec
            done += 1
            if done % 20 == 0 or rec["status"].startswith("failed"):
                print(f"[{done}/{len(ids)}] {rec['id']}: {rec['status']}", flush=True)

    shutil.copy(report_path, report_path.with_suffix(".json.pre_upright"))
    report["results"] = [results.get(r["id"], r) for r in report["results"]]
    report_path.write_text(json.dumps(report, indent=2))

    from collections import Counter
    old = json.loads((report_path.with_suffix(".json.pre_upright")).read_text())
    old_by = {r["id"]: r["status"] for r in old["results"]}
    changed = [(sid, old_by[sid], r["status"]) for sid, r in results.items()
               if old_by.get(sid) != r["status"]]
    print("\nstatus changes:")
    for sid, a, b in sorted(changed):
        print(f"  {sid:24s} {a} -> {b}")
    print("\nfinal:", Counter(r["status"] for r in results.values()))


if __name__ == "__main__":
    main()
