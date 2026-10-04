#!/usr/bin/env python3
"""Compare tectonics-lab keyframe bundles by plate statistics.

Works on any bundle in the tectonics-lab layout (seeded worlds from
`tectonics-lab run` and the Earth reference bundle from
earth_plates_export.py), so the same numbers can be put side by side:

  plates        live plates per frame (mean, min, max) and majors
  largest       largest plate's share of the sphere (mean)
  top3          share of the three largest plates (mean)
  speed         area-weighted mean surface speed, cm/yr (mean over frames)
  speed p90     90th percentile of per-plate surface speed
  boundary      share of active boundary pixels that are divergent /
                convergent / transform ("active" = feature age 0)
  lifetime      median plate lifetime in Myr (ids present in consecutive
                frames; censored at the run's ends)
  births        plate births (ids first seen after frame 0)
  re-aims       mean number of >30° pole-direction changes per plate per 100 Myr

Usage:
  python3 scripts/tectonics_stats.py data/tectonics/earth-nnr data/tectonics/seed-42-terran [...]
  python3 scripts/tectonics_stats.py --series 100 data/tectonics/earth-nnr   # per-100-Myr table
Needs numpy + Pillow (the gplates venv has both).
"""
import argparse
import json
import math
import os
import statistics

import numpy as np
from PIL import Image

RADIUS_KM = 6371.0
CROSS_CELLS = [(2, 1), (0, 1), (1, 0), (1, 2), (1, 1), (3, 1)]


def face_uv_to_dir(face, u, v):
    sc, tc = 2 * u - 1, 2 * v - 1
    one = np.ones_like(sc)
    x, y, z = {0: (one, -tc, -sc), 1: (-one, -tc, sc), 2: (sc, one, tc),
               3: (sc, -one, -tc), 4: (sc, -tc, one), 5: (-sc, -tc, -one)}[face]
    n = np.sqrt(x * x + y * y + z * z)
    return np.stack([x / n, y / n, z / n], axis=-1)


def pixel_dirs(S):
    px = (np.arange(S) + 0.5) / S
    u, v = np.meshgrid(px, px)
    return np.concatenate([face_uv_to_dir(f, u.ravel(), v.ravel()) for f in range(6)])


def read_cross(path, S):
    img = np.array(Image.open(path))[..., 0]
    out = np.empty(6 * S * S, dtype=np.int64)
    for face in range(6):
        col, row = CROSS_CELLS[face]
        out[face * S * S:(face + 1) * S * S] = img[row * S:(row + 1) * S, col * S:(col + 1) * S].ravel()
    return out


class Bundle:
    def __init__(self, path):
        self.path = path
        self.m = json.load(open(os.path.join(path, "manifest.json")))
        self.S = self.m["face"]
        self.dirs = pixel_dirs(self.S)
        self.frames = self.m["frames"]
        self.myr_per_frame = (self.frames[-1]["myr"] - self.frames[0]["myr"]) / max(1, len(self.frames) - 1)

    def frame_stats(self, fi):
        f = self.frames[fi]
        d = os.path.join(self.path, f["dir"])
        labels = read_cross(os.path.join(d, "plate.png"), self.S)
        feature = read_cross(os.path.join(d, "feature.png"), self.S)
        rows = [r for r in f["plates"] if not r.get("retired") and r["area"] > 0]
        N = len(labels)
        # per-pixel surface speed from each plate's pole and speed scalar
        speed_px = np.zeros(N)
        plate_speed = {}
        for r in rows:
            mask = labels == r["id"]
            if not mask.any():
                continue
            pole = np.array(r["pole"])
            v = np.cross(pole, self.dirs[mask])  # |ω×d| with |ω| = 1
            s = np.linalg.norm(v, axis=1) * r["speed_cm_yr"]
            speed_px[mask] = s
            plate_speed[r["id"]] = float(s.mean())
        areas = sorted((r["area"] for r in rows), reverse=True)
        ftype = feature >> 6
        fage = feature & 63
        active = (ftype > 0) & (fage == 0)
        n_active = int(active.sum())
        frac = {k: (int(((ftype == k) & active).sum()) / n_active if n_active else 0.0) for k in (1, 2, 3)}
        return {
            "plates": len(rows),
            "majors": sum(1 for r in rows if r.get("major")),
            "largest": areas[0] if areas else 0.0,
            "top3": sum(areas[:3]),
            "speed": float(speed_px.mean()),
            "speed_p90": float(np.percentile(list(plate_speed.values()), 90)) if plate_speed else 0.0,
            "div": frac[1], "conv": frac[2], "trans": frac[3],
            "ids": {r["id"]: np.array(r["pole"]) for r in rows},
            "myr": f["myr"],
        }

    def summary(self, series=0):
        per = [self.frame_stats(i) for i in range(len(self.frames))]
        # lifetimes and re-aims from id continuity
        first, last, poles = {}, {}, {}
        reaims = {}
        for i, p in enumerate(per):
            for pid, pole in p["ids"].items():
                first.setdefault(pid, i)
                last[pid] = i
                if pid in poles:
                    cosang = float(np.clip(np.dot(poles[pid], pole), -1, 1))
                    if math.degrees(math.acos(cosang)) > 30:
                        reaims[pid] = reaims.get(pid, 0) + 1
                poles[pid] = pole
        lifetimes = [(last[p] - first[p] + 1) * self.myr_per_frame for p in first]
        births = sum(1 for p in first if first[p] > 0)
        span = per[-1]["myr"] - per[0]["myr"]
        total_plate_myr = sum(lifetimes)
        agg = {
            "frames": len(per), "span_myr": span, "face": self.S,
            "plates_mean": statistics.mean(p["plates"] for p in per),
            "plates_min": min(p["plates"] for p in per), "plates_max": max(p["plates"] for p in per),
            "majors_mean": statistics.mean(p["majors"] for p in per),
            "largest": statistics.mean(p["largest"] for p in per),
            "top3": statistics.mean(p["top3"] for p in per),
            "speed": statistics.mean(p["speed"] for p in per),
            "speed_p90": statistics.mean(p["speed_p90"] for p in per),
            "div": statistics.mean(p["div"] for p in per),
            "conv": statistics.mean(p["conv"] for p in per),
            "trans": statistics.mean(p["trans"] for p in per),
            "lifetime_median": statistics.median(lifetimes) if lifetimes else 0.0,
            "births": births,
            "reaims_per_100myr": (sum(reaims.values()) / total_plate_myr * 100) if total_plate_myr else 0.0,
        }
        rows = []
        if series:
            cur, bucket = None, []
            for p in per:
                b = int(p["myr"] // series)
                if cur is None:
                    cur = b
                if b != cur:
                    rows.append((cur * series, bucket))
                    cur, bucket = b, []
                bucket.append(p)
            if bucket:
                rows.append((cur * series, bucket))
        return agg, rows


def fmt_pct(x):
    return f"{100 * x:4.0f}%"


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("bundles", nargs="+")
    ap.add_argument("--series", type=int, default=0, help="also print a per-N-Myr table")
    args = ap.parse_args()

    results = []
    for b in args.bundles:
        bundle = Bundle(b)
        agg, rows = bundle.summary(args.series)
        results.append((os.path.basename(b.rstrip("/")), agg, rows))

    names = [r[0] for r in results]
    print("| metric | " + " | ".join(names) + " |")
    print("| --- | " + " | ".join("---:" for _ in names) + " |")
    lines = [
        ("frames / span Myr / face", lambda a: f"{a['frames']} / {a['span_myr']:.0f} / {a['face']}"),
        ("live plates (mean, min–max)", lambda a: f"{a['plates_mean']:.1f} ({a['plates_min']}–{a['plates_max']})"),
        ("majors (mean)", lambda a: f"{a['majors_mean']:.1f}"),
        ("largest plate share", lambda a: fmt_pct(a['largest'])),
        ("top-3 share", lambda a: fmt_pct(a['top3'])),
        ("mean surface speed cm/yr", lambda a: f"{a['speed']:.2f}"),
        ("plate speed p90 cm/yr", lambda a: f"{a['speed_p90']:.2f}"),
        ("active boundary: divergent", lambda a: fmt_pct(a['div'])),
        ("active boundary: convergent", lambda a: fmt_pct(a['conv'])),
        ("active boundary: transform", lambda a: fmt_pct(a['trans'])),
        ("median plate lifetime Myr", lambda a: f"{a['lifetime_median']:.0f}"),
        ("plate births (ids first seen after frame 0)", lambda a: f"{a['births']}"),
        ("re-aims (>30°) per plate per 100 Myr", lambda a: f"{a['reaims_per_100myr']:.2f}"),
    ]
    for label, fn in lines:
        print(f"| {label} | " + " | ".join(fn(r[1]) for r in results) + " |")

    if args.series:
        for name, _, rows in results:
            print(f"\n### {name} per {args.series} Myr\n")
            print("| from Myr | plates | majors | largest | speed cm/yr | div | conv | trans |")
            print("| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
            for start, bucket in rows:
                print(f"| {start:.0f} | {statistics.mean(p['plates'] for p in bucket):.1f} | "
                      f"{statistics.mean(p['majors'] for p in bucket):.1f} | "
                      f"{fmt_pct(statistics.mean(p['largest'] for p in bucket))} | "
                      f"{statistics.mean(p['speed'] for p in bucket):.2f} | "
                      f"{fmt_pct(statistics.mean(p['div'] for p in bucket))} | "
                      f"{fmt_pct(statistics.mean(p['conv'] for p in bucket))} | "
                      f"{fmt_pct(statistics.mean(p['trans'] for p in bucket))} |")


if __name__ == "__main__":
    main()
