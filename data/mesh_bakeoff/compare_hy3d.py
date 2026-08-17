#!/usr/bin/env python3
"""Score round 2 (Hunyuan3D-2) against round 1 (TripoSR) on two-view consistency.

The question is narrow: when the same ship is photographed from two angles,
does the reconstructed top-down footprint come out the same? Round 1 failed
this (report.md) because TripoSR's aspect tracked each photograph's apparent
foreshortening rather than the hull. Everything here is a paired comparison
on identical inputs and identical downstream extraction (mesh_footprint.py),
so any difference is the generative model.

    python3 compare_hy3d.py
"""

import json
from pathlib import Path

import numpy as np

HERE = Path(__file__).resolve().parent
ROUND1 = HERE / "out-full"     # TripoSR
ROUND2 = HERE / "out-hy3d"     # Hunyuan3D-2
FUSED = HERE.parent / "footprints" / "fused"

PAIRS = [
    ("warranty", "nebula_warranty"),
    ("precept", "solarian_precept"),
    ("excessive_force", "outerrim_excessive_force"),
    ("capacity", "solarian_capacity"),
    ("promenade", "solarian_promenade"),
    ("ordinance", "crimson_ordinance"),
    ("archive", "solarian_archive"),
]
PREFIXES = ("nebula_", "solarian_", "outerrim_", "crimson_", "voidborn_")


def ship_id(stem: str) -> str:
    for p in PREFIXES:
        if stem.startswith(p):
            return stem[len(p):]
    return stem


def load_profile(root: Path, stem: str) -> dict | None:
    f = root / stem / "profile.json"
    if not f.exists():
        return None
    return json.loads(f.read_text())


def spread(a: float, b: float) -> float:
    """Disagreement as a fraction of the smaller value -- the report's units."""
    lo, hi = min(a, b), max(a, b)
    return (hi - lo) / lo if lo > 0 else float("nan")


def profile_r(wa: list, wb: list) -> float:
    """Best correlation of two 96-station width profiles over both fore/aft
    orientations. Direction along the hull axis is not resolved by either
    reconstruction, so an anti-correlated pair usually means 'same shape,
    nose swapped', not 'different shape' -- taking the max keeps the metric
    measuring shape agreement rather than an unrelated frame convention."""
    a = np.asarray(wa, dtype=float)
    b = np.asarray(wb, dtype=float)
    if a.std() == 0 or b.std() == 0:
        return float("nan")
    return max(float(np.corrcoef(a, b)[0, 1]),
               float(np.corrcoef(a, b[::-1])[0, 1]))


def row(round_dir: Path, sa: str, sb: str) -> dict | None:
    pa, pb = load_profile(round_dir, sa), load_profile(round_dir, sb)
    if pa is None or pb is None:
        return None
    return {
        "aspect_a": pa["aspect"],
        "aspect_b": pb["aspect"],
        "spread": spread(pa["aspect"], pb["aspect"]),
        "r": profile_r(pa["w"], pb["w"]),
        # A pair where either view failed to resolve a lateral-vs-vertical
        # frame is measuring aspect on a possibly-wrong plane, so it can
        # inflate the spread for reasons that have nothing to do with the
        # generative model. Tracked so the headline can be re-checked with
        # those pairs dropped.
        "clean_frame": not (pa["frame_ambiguous"] or pb["frame_ambiguous"]),
    }


def main() -> int:
    have_r2 = ROUND2.exists()
    print(f"{'pair':26} {'TripoSR spread':>14} {'r':>6}   "
          f"{'Hy3D spread':>12} {'r':>6}   {'fused aspect':>12}")
    print("-" * 92)

    agg = {"r1": [], "r2": []}
    for sa, sb in PAIRS:
        r1 = row(ROUND1, sa, sb)
        r2 = row(ROUND2, sa, sb) if have_r2 else None
        fused_f = FUSED / f"{ship_id(sa)}.json"
        fused_a = json.loads(fused_f.read_text())["aspect"] if fused_f.exists() else float("nan")

        def fmt(r):
            if r is None:
                return f"{'--':>12} {'--':>6}"
            return f"{r['spread']*100:11.0f}% {r['r']:6.2f}"

        if r1:
            agg["r1"].append(r1)
        if r2:
            agg["r2"].append(r2)
        print(f"{ship_id(sa):26} {fmt(r1)}   {fmt(r2)}   {fused_a:12.3f}")

    print("-" * 92)
    for key, label in (("r1", "TripoSR"), ("r2", "Hunyuan3D-2")):
        rows = agg[key]
        if not rows:
            print(f"{label:12} (no results yet)")
            continue
        spreads = np.array([r["spread"] for r in rows])
        rs = np.array([r["r"] for r in rows])
        print(f"{label:12} median spread {np.median(spreads)*100:5.1f}%   "
              f"max {spreads.max()*100:5.1f}%   "
              f"pairs within 10% {int((spreads <= 0.10).sum())}/{len(spreads)}   "
              f"median r {np.median(rs):.2f}")
        clean = np.array([r["spread"] for r in rows if r["clean_frame"]])
        if len(clean) and len(clean) != len(spreads):
            print(f"{'':12}   clean-frame pairs only ({len(clean)}/{len(rows)}): "
                  f"median spread {np.median(clean)*100:5.1f}%   "
                  f"within 10% {int((clean <= 0.10).sum())}/{len(clean)}")

    if not have_r2:
        print(f"\n(round 2 not run yet -- {ROUND2} missing)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
