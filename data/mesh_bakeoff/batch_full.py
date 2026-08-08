#!/usr/bin/env python3
"""Full-drop bake-off batch: ALL PNGs in ~/Downloads/chromakeys, chunked.

Per team-lead direction after the 32-image spike graduated: same model
(TripoSR) and same mesh_footprint.py extraction, but run over the entire
402-image chromakeys drop instead of the curated 32. Chunked into ~50-image
TripoSR subprocess calls so a single bad image or a chunk-level crash loses
that chunk, not the whole run -- each chunk's outcome (including failure) is
recorded in results-full.json rather than raising.

DO NOT RUN until explicitly told GO -- the GPU is occupied by the pipeline's
own 402-ship batch. Verify safely first:

    ~/sf3d-venv/bin/python batch_full.py --dry-run

which builds every subprocess command, chunk boundary, and output path
without importing torch, touching the GPU, or invoking TripoSR.

    ~/sf3d-venv/bin/python batch_full.py

runs for real once cleared.
"""

import argparse
import json
import re
import subprocess
import sys
import time
from pathlib import Path

BAKEOFF = Path("/tmp/claude-1000/-home-robert-spacemolt-kb/f10074c6-5202-4215-8d92-b487ae17d8de/scratchpad/bakeoff")
OUT = BAKEOFF / "out-full"
TRIPOSR_DIR = Path.home() / "sf3d-venv" / "src" / "TripoSR"
PYTHON = Path.home() / "sf3d-venv" / "bin" / "python"
CHROMAKEYS = Path.home() / "Downloads" / "chromakeys"

MODEL_NAME = "TripoSR (stabilityai/TripoSR)"
CHUNK_SIZE = 50


def discover_images() -> list[tuple[str, Path]]:
    """All PNGs in chromakeys, stem = filename stem, no filtering. Sorted
    for a deterministic chunk assignment (so a re-run with --dry-run always
    reports the same plan as the real run)."""
    paths = sorted(CHROMAKEYS.glob("*.png"))
    return [(p.stem, p) for p in paths]


def chunk(images: list[tuple[str, Path]], size: int) -> list[list[tuple[str, Path]]]:
    return [images[i:i + size] for i in range(0, len(images), size)]


def plan(images: list[tuple[str, Path]]) -> list[dict]:
    """Build the full per-chunk execution plan (commands + output dirs)
    without running anything -- shared by --dry-run and the real run so the
    dry-run actually validates what will execute."""
    chunks = chunk(images, CHUNK_SIZE)
    out_plan = []
    for ci, imgs in enumerate(chunks):
        tsr_out = OUT / "_tsr" / f"chunk_{ci:03d}"
        cmd = [str(PYTHON), "run.py"] + [str(p) for _, p in imgs] + [
            "--output-dir", str(tsr_out),
            "--model-save-format", "obj",
        ]
        out_plan.append({
            "chunk_index": ci,
            "n_images": len(imgs),
            "stems": [s for s, _ in imgs],
            "tsr_out_dir": str(tsr_out),
            "cmd": cmd,
            "log_path": str(OUT / f"_tsr_chunk_{ci:03d}.log"),
        })
    return out_plan


def run_chunk(chunk_plan: dict, images: list[tuple[str, Path]]) -> tuple[dict, float]:
    """Run one TripoSR subprocess for a chunk. Returns ({stem: mesh_path or
    None}, elapsed_seconds). Never raises on subprocess failure -- caller
    records it as a chunk-level failure for every image in the chunk."""
    import os
    tsr_out = Path(chunk_plan["tsr_out_dir"])
    tsr_out.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env["HF_HUB_DISABLE_XET"] = "1"

    t0 = time.time()
    proc = subprocess.run(chunk_plan["cmd"], cwd=str(TRIPOSR_DIR), env=env,
                           capture_output=True, text=True)
    elapsed = time.time() - t0

    Path(chunk_plan["log_path"]).write_text(proc.stdout + "\n----STDERR----\n" + proc.stderr)

    mesh_paths = {}
    if proc.returncode != 0:
        # Whole chunk failed (e.g. OOM, a corrupt image crashing the batch
        # before any mesh export) -- every image in it is unresolved, but
        # every OTHER chunk still runs. Caller records this per-image.
        for stem, _ in images:
            mesh_paths[stem] = None
        return mesh_paths, elapsed

    for i, (stem, _) in enumerate(images):
        p = tsr_out / str(i) / "mesh.obj"
        mesh_paths[stem] = p if p.exists() else None
    return mesh_paths, elapsed


def parse_per_image_timings(log_text: str) -> list[dict]:
    pattern = re.compile(r"(Running model|Extracting mesh|Exporting mesh) finished in ([\d.]+)ms")
    matches = pattern.findall(log_text)
    per_image = []
    cur = {}
    for name, ms in matches:
        cur[name] = float(ms) / 1000.0
        if name == "Exporting mesh":
            per_image.append(cur)
            cur = {}
    return per_image


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true",
                     help="Build commands/chunks/paths only. No torch import, "
                          "no GPU use, no TripoSR invocation.")
    args = ap.parse_args()

    images = discover_images()
    if not images:
        print(f"ERROR: no PNGs found under {CHROMAKEYS}", file=sys.stderr)
        sys.exit(1)

    chunk_plan = plan(images)

    if args.dry_run:
        print(f"Discovered {len(images)} PNGs under {CHROMAKEYS}")
        print(f"Chunked into {len(chunk_plan)} chunks of up to {CHUNK_SIZE} images each:")
        for cp in chunk_plan:
            print(f"  chunk {cp['chunk_index']:03d}: {cp['n_images']} images -> {cp['tsr_out_dir']}")
            print(f"    first: {cp['stems'][0]}  last: {cp['stems'][-1]}")
        total_planned = sum(cp["n_images"] for cp in chunk_plan)
        assert total_planned == len(images), "chunk plan lost or duplicated images"
        assert len(set(s for cp in chunk_plan for s in cp["stems"])) == len(images), \
            "duplicate stems across chunks"
        OUT.mkdir(parents=True, exist_ok=True)  # only side effect: the empty out-full/ dir
        print(f"\nOK: {total_planned} images across {len(chunk_plan)} chunks, "
              f"no duplicate/lost stems. Output root: {OUT}")
        print("Dry run only -- no torch import, no subprocess launched, no GPU touched.")
        return

    OUT.mkdir(parents=True, exist_ok=True)
    sys.path.insert(0, str(BAKEOFF))
    from mesh_footprint import run as extract_footprint

    images_by_stem = dict(images)
    results = []
    chunk_summaries = []
    for cp in chunk_plan:
        chunk_images = [(s, images_by_stem[s]) for s in cp["stems"]]
        print(f"[chunk {cp['chunk_index']+1}/{len(chunk_plan)}] "
              f"running {cp['n_images']} images...")
        try:
            mesh_paths, elapsed = run_chunk(cp, chunk_images)
            chunk_error = None
        except Exception as e:  # noqa: BLE001 - one chunk's crash must not kill the run
            mesh_paths = {s: None for s, _ in chunk_images}
            elapsed = 0.0
            chunk_error = f"{type(e).__name__}: {e}"

        log_text = Path(cp["log_path"]).read_text() if Path(cp["log_path"]).exists() else ""
        per_image_tsr = parse_per_image_timings(log_text)
        n_chunk_ok = 0

        for i, (stem, src_path) in enumerate(chunk_images):
            entry = {
                "image_stem": stem,
                "source_image": str(src_path),
                "model": MODEL_NAME,
                "chunk_index": cp["chunk_index"],
            }
            mesh_src = mesh_paths.get(stem)
            entry["triposr_seconds"] = per_image_tsr[i] if i < len(per_image_tsr) else {}

            if chunk_error is not None:
                entry["status"] = "failed"
                entry["error"] = f"chunk-level failure: {chunk_error}"
            elif mesh_src is None or not mesh_src.exists():
                entry["status"] = "failed"
                entry["error"] = "TripoSR produced no mesh.obj for this image"
            else:
                ship_out = OUT / stem
                ship_out.mkdir(parents=True, exist_ok=True)
                mesh_dst = ship_out / "mesh.obj"
                mesh_dst.write_bytes(mesh_src.read_bytes())
                try:
                    fp = extract_footprint(str(mesh_dst), stem, ship_out)
                    entry.update({
                        "status": "ok",
                        "mesh_path": str(mesh_dst),
                        "footprint_json": fp["footprint_json"],
                        "profile_json": fp["profile_json"],
                        "frame_ambiguous": fp["frame_ambiguous"],
                        "aspect": fp["aspect"],
                        "footprint_extract_seconds": fp["footprint_extract_seconds"],
                        "n_verts": fp["n_verts"],
                        "n_faces": fp["n_faces"],
                    })
                    n_chunk_ok += 1
                except Exception as e:  # noqa: BLE001 - one image must not kill the chunk
                    entry["status"] = "failed"
                    entry["error"] = f"footprint extraction: {type(e).__name__}: {e}"

            results.append(entry)

        chunk_summaries.append({
            "chunk_index": cp["chunk_index"],
            "n_images": cp["n_images"],
            "n_ok": n_chunk_ok,
            "n_failed": cp["n_images"] - n_chunk_ok,
            "chunk_error": chunk_error,
            "triposr_seconds": elapsed,
        })
        print(f"[chunk {cp['chunk_index']+1}/{len(chunk_plan)}] "
              f"{n_chunk_ok}/{cp['n_images']} ok"
              + (f" -- CHUNK FAILED: {chunk_error}" if chunk_error else ""))

    summary = {
        "model": MODEL_NAME,
        "n_images": len(images),
        "n_chunks": len(chunk_plan),
        "chunk_size": CHUNK_SIZE,
        "n_ok": sum(1 for r in results if r["status"] == "ok"),
        "n_failed": sum(1 for r in results if r["status"] == "failed"),
        "chunks": chunk_summaries,
        "results": results,
    }
    (BAKEOFF / "results-full.json").write_text(json.dumps(summary, indent=2))
    print(f"\nDone: {summary['n_ok']}/{summary['n_images']} ok, "
          f"{summary['n_failed']} failed across {summary['n_chunks']} chunks. "
          f"results-full.json written.")


if __name__ == "__main__":
    main()
