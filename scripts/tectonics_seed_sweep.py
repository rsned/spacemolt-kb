#!/usr/bin/env python3
"""Bake many seeds with tectonics-lab, score each with tectonics_stats, keep only the numbers.

For every seed: `tectonics-lab run` into a work directory, summarise the
bundle with tectonics_stats.Bundle, append one CSV row, delete the bundle.
Seeds are drawn from a master seed so a sweep is reproducible and resumable:
seeds already present in sweep.csv are skipped on a rerun.

At the end (or with --summarize-only) a summary.md is written with the
median / p10 / p90 of every metric next to the Earth reference bundle, plus
the outlier seeds: no births, mean live plates outside 12–25, largest plate
above 50 %, or a failed run.

Usage:
  ~/gplates-venv/bin/python3 scripts/tectonics_seed_sweep.py --n 100 --jobs 3
  ~/gplates-venv/bin/python3 scripts/tectonics_seed_sweep.py --summarize-only
Needs numpy + Pillow (the gplates venv has both) and bin/tectonics-lab.
"""
import argparse
import concurrent.futures as cf
import csv
import json
import os
import random
import shutil
import statistics
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import tectonics_stats as ts  # noqa: E402

EXTRA_FIELDS = ["status", "wall_s", "error"]
HEADER = ["seed"] + ts.CSV_FIELDS + EXTRA_FIELDS

# metric -> (label, format, lower outlier bound, upper outlier bound); None = no bound
METRICS = [
    ("plates_mean", "live plates (mean)", "{:.1f}", 12, 25),
    ("plates_min", "live plates (min)", "{:.0f}", None, None),
    ("plates_max", "live plates (max)", "{:.0f}", None, None),
    ("largest", "largest plate share", "{:.0%}", None, 0.5),
    ("top3", "top-3 share", "{:.0%}", None, None),
    ("speed", "mean surface speed cm/yr", "{:.2f}", None, None),
    ("speed_p90", "plate speed p90 cm/yr", "{:.2f}", None, None),
    ("div", "active boundary: divergent", "{:.0%}", None, None),
    ("conv", "active boundary: convergent", "{:.0%}", None, None),
    ("trans", "active boundary: transform", "{:.0%}", None, None),
    ("lifetime_median", "median plate lifetime Myr", "{:.0f}", None, None),
    ("births", "plate births", "{:.0f}", 1, None),
    ("reaims_per_100myr", "re-aims (>30°) / plate / 100 Myr", "{:.2f}", None, None),
]


def draw_seeds(master, n):
    rng = random.Random(master)
    seeds, seen = [], set()
    while len(seeds) < n:
        s = rng.randrange(1, 1 << 31)
        if s not in seen:
            seen.add(s)
            seeds.append(s)
    return seeds


def read_rows(path):
    if not os.path.exists(path):
        return []
    with open(path, newline="") as f:
        return list(csv.DictReader(f))


def bake_and_score(args, seed):
    """Run one seed end to end; returns a dict keyed by HEADER."""
    t0 = time.time()
    row = {k: "" for k in HEADER}
    row["seed"] = str(seed)
    work = os.path.join(args.out, "work")
    bundle = os.path.join(work, f"seed-{seed}-{args.archetype}")
    cmd = [args.bin, "run", "-seed", str(seed), "-archetype", args.archetype,
           "-face", str(args.face), "-steps", str(args.steps), "-out", work]
    for kv in args.set:
        cmd += ["-set", kv]
    try:
        subprocess.run(cmd, check=True, capture_output=True, text=True, timeout=args.timeout)
        agg, _ = ts.Bundle(bundle).summary()
        for k in ts.CSV_FIELDS:
            v = agg[k]
            row[k] = str(v) if isinstance(v, int) else f"{v:.5g}"
        row["status"] = "ok"
    except subprocess.CalledProcessError as e:
        row["status"], row["error"] = "error", (e.stderr or str(e)).strip().replace("\n", " ")[-300:]
    except Exception as e:  # noqa: BLE001 — any failure is a data point, not a crash
        row["status"], row["error"] = "error", f"{type(e).__name__}: {e}"[:300]
    finally:
        shutil.rmtree(bundle, ignore_errors=True)
    row["wall_s"] = f"{time.time() - t0:.0f}"
    return row


def earth_row(args):
    """Earth reference metrics, computed once and cached beside the sweep."""
    cache = os.path.join(args.out, "earth.json")
    if os.path.exists(cache):
        return json.load(open(cache))
    if not args.earth or not os.path.isdir(args.earth):
        return None
    agg, _ = ts.Bundle(args.earth).summary()
    agg = {k: agg[k] for k in ts.CSV_FIELDS}
    json.dump(agg, open(cache, "w"), indent=1)
    return agg


def pct(values, p):
    values = sorted(values)
    if not values:
        return float("nan")
    k = (len(values) - 1) * p
    lo, hi = int(k), min(int(k) + 1, len(values) - 1)
    return values[lo] + (values[hi] - values[lo]) * (k - lo)


def summarize(args):
    rows = read_rows(os.path.join(args.out, "sweep.csv"))
    ok = [r for r in rows if r["status"] == "ok"]
    bad = [r for r in rows if r["status"] != "ok"]
    earth = earth_row(args)
    out = []
    out.append(f"# Seed sweep — {args.archetype}, face {args.face}, {args.steps} steps\n")
    out.append(f"{len(ok)} seeds scored, {len(bad)} failed. "
               f"Master seed {args.master}; defaults from the binary plus `{' '.join(args.set) or 'no -set overrides'}`.\n")
    out.append("| metric | Earth | median | p10 | p90 | min | max |")
    out.append("| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
    for key, label, fmt, _, _ in METRICS:
        vals = [float(r[key]) for r in ok]
        if not vals:
            continue
        e = fmt.format(earth[key]) if earth else "—"
        out.append(f"| {label} | {e} | {fmt.format(statistics.median(vals))} | {fmt.format(pct(vals, 0.1))} | "
                   f"{fmt.format(pct(vals, 0.9))} | {fmt.format(min(vals))} | {fmt.format(max(vals))} |")
    if ok:
        walls = [float(r["wall_s"]) for r in ok]
        out.append(f"\nWall time per seed: median {statistics.median(walls):.0f} s, max {max(walls):.0f} s "
                   f"(bake + score, {args.jobs} in parallel).\n")

    out.append("## Outliers\n")
    flagged = []
    for r in ok:
        why = []
        for key, label, fmt, lo, hi in METRICS:
            v = float(r[key])
            if lo is not None and v < lo:
                why.append(f"{label} {fmt.format(v)} < {lo}")
            if hi is not None and v > hi:
                why.append(f"{label} {fmt.format(v)} > {hi}")
        if why:
            flagged.append((r["seed"], "; ".join(why)))
    if not flagged and not bad:
        out.append("None.\n")
    for seed, why in flagged:
        out.append(f"- seed {seed}: {why}")
    for r in bad:
        out.append(f"- seed {r['seed']}: FAILED — {r['error']}")
    if flagged or bad:
        out.append("")
    out.append(f"Rebake one seed for the viewer with `bin/tectonics-lab run -seed <seed> -archetype {args.archetype} "
               f"-face {args.face} -steps {args.steps} -out data/tectonics`.")
    text = "\n".join(out) + "\n"
    with open(os.path.join(args.out, "summary.md"), "w") as f:
        f.write(text)
    print(text)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--n", type=int, default=100, help="number of seeds")
    ap.add_argument("--master", type=int, default=1, help="master seed the per-run seeds are drawn from")
    ap.add_argument("--archetype", default="terran")
    ap.add_argument("--face", type=int, default=256)
    ap.add_argument("--steps", type=int, default=400)
    ap.add_argument("--set", action="append", default=[], metavar="Name=value", help="forwarded to tectonics-lab run")
    ap.add_argument("--jobs", type=int, default=3, help="seeds baked in parallel")
    ap.add_argument("--timeout", type=int, default=1800, help="seconds allowed per bake")
    ap.add_argument("--bin", default="bin/tectonics-lab")
    ap.add_argument("--out", default="data/tectonics/sweep")
    ap.add_argument("--earth", default="data/tectonics/earth-nnr", help="Earth reference bundle ('' to skip)")
    ap.add_argument("--summarize-only", action="store_true", help="only rewrite summary.md from sweep.csv")
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)
    if args.summarize_only:
        summarize(args)
        return

    json.dump(vars(args), open(os.path.join(args.out, "sweep.json"), "w"), indent=1)
    csv_path = os.path.join(args.out, "sweep.csv")
    done = {r["seed"] for r in read_rows(csv_path)}
    todo = [s for s in draw_seeds(args.master, args.n) if str(s) not in done]
    print(f"{len(done)} seeds already scored, {len(todo)} to go, {args.jobs} at a time", flush=True)

    new_file = not os.path.exists(csv_path)
    t_start = time.time()
    with open(csv_path, "a", newline="") as f, cf.ThreadPoolExecutor(max_workers=args.jobs) as pool:
        w = csv.DictWriter(f, fieldnames=HEADER)
        if new_file:
            w.writeheader()
            f.flush()
        futures = {pool.submit(bake_and_score, args, s): s for s in todo}
        n = 0
        for fut in cf.as_completed(futures):
            row = fut.result()
            w.writerow(row)
            f.flush()
            n += 1
            elapsed = time.time() - t_start
            eta = elapsed / n * (len(todo) - n)
            print(f"[{n}/{len(todo)}] seed {row['seed']} {row['status']} "
                  f"plates {row['plates_mean'] or '-'} largest {row['largest'] or '-'} births {row['births'] or '-'} "
                  f"({row['wall_s']} s; ETA {eta / 60:.0f} min)", flush=True)
    summarize(args)


if __name__ == "__main__":
    main()
