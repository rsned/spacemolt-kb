#!/usr/bin/env python3
"""Re-run the 35 previously-unkeyable ships through the full pipeline with the
new shadow-tolerant hue key, alpha pinned to the committed batch's 8.0, and
merge the fresh records into data/footprints/report.json (backing it up first).
"""
import pathlib as _pathlib
_REPO = str(_pathlib.Path(__file__).resolve().parents[3])
import json
import pathlib
import shutil
import sys

sys.path.insert(0, _REPO)
from tools.footprint import run as fprun  # noqa: E402

REPORT = pathlib.Path(_REPO + "/data/footprints/report.json")
ALPHA = 8.0
TARGET_STATUS = sys.argv[1] if len(sys.argv) > 1 else "failed_unkeyable"

report = json.loads(REPORT.read_text())
unk = sorted(x["id"] for x in report["results"] if x["status"] == TARGET_STATUS)
print(f"re-running {len(unk)} ships with alpha={ALPHA}", flush=True)

heroes = fprun.resolve_heroes()
subset = {sid: heroes[sid] for sid in unk if sid in heroes}
missing = [s for s in unk if s not in heroes]
if missing:
    print("WARNING no hero image for:", missing, flush=True)

alpha, results = fprun._run_all(subset, "neutral", alpha=ALPHA)
by_id = {r["id"]: r for r in results}

shutil.copy(REPORT, REPORT.with_suffix(".json.bak"))
merged = []
for rec in report["results"]:
    merged.append(by_id.get(rec["id"], rec))
report["results"] = merged
REPORT.write_text(json.dumps(report, indent=2))

from collections import Counter
print("\nnew statuses for the 35:")
for sid in unk:
    r = by_id.get(sid)
    print(f"  {sid:22s} {r['status'] if r else 'NOT RUN'}")
print(Counter(r["status"] for r in by_id.values()))
print("\nfull-batch statuses now:")
print(Counter(r["status"] for r in merged))
