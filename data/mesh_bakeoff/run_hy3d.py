#!/usr/bin/env python3
"""Hunyuan3D-2 shape-only generation for the image-to-3D bake-off, round 2.

Round 1 (TripoSR, 2026-08-07, see report.md) died on the two-view
self-consistency test: 6 of 7 duplicate pairs disagreed 10-131% on aspect,
because TripoSR's reconstruction tracked each photograph's apparent
foreshortening. This re-runs exactly that test with a frontier-tier open
model to see whether the failure was TripoSR's capacity or is intrinsic to
single-image 3D.

Deliberately shape-only -- Hunyuan3D-Paint is never loaded. Footprints need
geometry, not texture, and skipping it keeps us inside 16GB with headroom.

Background removal is a hard chroma key, not rembg. The source drop is RGB
on pure magenta, so keying is exact; round 1's rembg matte produced magenta
edge-fringing that the report flagged as a scale risk.

    ~/hy3d-venv/bin/python run_hy3d.py --dry-run
    ~/hy3d-venv/bin/python run_hy3d.py [stem ...]
"""

import argparse
import json
import time
from pathlib import Path

import numpy as np
from PIL import Image
from scipy.ndimage import binary_propagation, label

CHROMAKEYS = Path.home() / "Downloads" / "chromakeys"
OUT = Path(__file__).resolve().parent / "out-hy3d"
MODEL_PATH = "tencent/Hunyuan3D-2"

# The 7 duplicate pairs from round 1: same ship, two camera angles. These are
# the whole point -- a model that reconstructs shape rather than perspective
# must land both views of a pair on the same aspect.
PAIRS = [
    ("warranty", "nebula_warranty"),
    ("precept", "solarian_precept"),
    ("excessive_force", "outerrim_excessive_force"),
    ("capacity", "solarian_capacity"),
    ("promenade", "solarian_promenade"),
    ("ordinance", "crimson_ordinance"),
    ("archive", "solarian_archive"),
]
# Plus the ship the user independently validated on meshy.ai, so we can hold
# our output next to a known-good commercial result on the same input.
EXTRA = ["outerrim_rapid_smelter"]

DEFAULT_STEMS = [s for pair in PAIRS for s in pair] + EXTRA

KEY_RGB = np.array([255, 0, 255], dtype=np.int16)
KEY_TOL = 60  # generous: the key is flat, the tolerance only has to beat JPEG-ish ringing


def chroma_key(img: Image.Image) -> Image.Image:
    """Magenta -> alpha, with a despill pass.

    Distance-to-key gives a soft matte at the boundary so we keep antialiased
    hull edges instead of stairstepping them. Despill then pulls the residual
    magenta out of the surviving fringe pixels: on a magenta key the spill is
    excess R and B relative to G, so clamping both toward G neutralises it
    without touching the ship's own warm browns (which have R > G > B, not
    R,B >> G).
    """
    # float32, not int16: squaring a channel delta reaches 255**2 = 65025,
    # which overflows int16 and silently yields NaN distances -> opaque matte.
    rgb = np.asarray(img.convert("RGB"), dtype=np.float32)

    # The drop is nominally magenta, but a few heroes arrive on a different
    # backdrop entirely (osprey: white). Key against whatever colour the
    # border actually is; fall back to magenta only if the border is
    # already close to it, so the despill/hue passes below stay valid.
    border = np.concatenate([rgb[0], rgb[-1], rgb[:, 0], rgb[:, -1]])
    key = np.median(border, axis=0)
    if np.linalg.norm(key - KEY_RGB) < 90:
        key = KEY_RGB.astype(np.float32)
    dist = np.sqrt(((rgb - key) ** 2).sum(axis=2))
    alpha = np.clip((dist - KEY_TOL) / KEY_TOL, 0.0, 1.0)

    # Shadow / tinted-backdrop removal. Drop shadows are darkened magenta:
    # far from the key by DISTANCE (so the matte keeps them -- crimson_
    # guillotine's puddle, voidborn_parallax's whole textured backdrop) but
    # still magenta by HUE (r ~ b, g suppressed). Hue alone would also eat
    # voidborn glow membranes, so only hue-matched pixels flood-connected to
    # the image border through background-or-shadow territory are removed;
    # interior glow stays. Costs a little genuine membrane fringe where it
    # fades into the backdrop -- acceptable vs modelling a shadow pancake.
    r, g_, b = rgb[..., 0], rgb[..., 1], rgb[..., 2]
    mx = np.maximum(r, b)
    maghue = (np.abs(r - b) < 0.25 * mx) & (g_ < 0.45 * mx) & (mx > 40)
    cand = (alpha <= 0) | maghue
    seed = np.zeros_like(cand)
    seed[0, :] = seed[-1, :] = seed[:, 0] = seed[:, -1] = True
    reach = binary_propagation(seed & cand, mask=cand)
    alpha[maghue & reach] = 0.0

    # Speckle removal: some heroes carry a ragged dark border (compression
    # junk at the art's edge) that is neither near the key colour nor
    # magenta-hued, so it survives to here and the model extrudes it into
    # dangling sheets (crimson_gas_cracker's strips). Any surviving blob
    # that is tiny relative to the biggest one is noise, not ship.
    fg, n_blobs = label(alpha > 0.1)
    if n_blobs > 1:
        sizes = np.bincount(fg.ravel())
        sizes[0] = 0
        alpha[sizes[fg] < 0.005 * sizes.max()] = 0.0

    # Despill only where the matte actually keeps the pixel. Applying it to
    # fully-keyed background too would just recolour pixels we are about to
    # make transparent, and it is what turned the failed first run's magenta
    # field into dark purple instead of revealing the bug.
    r, g, b = rgb[..., 0], rgb[..., 1], rgb[..., 2]
    spill = (r > g) & (b > g) & (alpha > 0)
    out = rgb.copy()
    cap = g + 12
    out[..., 0] = np.where(spill, np.minimum(r, cap), r)
    out[..., 2] = np.where(spill, np.minimum(b, cap), b)

    rgba = np.dstack([np.clip(out, 0, 255).astype(np.uint8),
                      (alpha * 255).astype(np.uint8)])
    return Image.fromarray(rgba, mode="RGBA")


def build_plan(stems: list[str], out: Path, src_dir: Path) -> list[dict]:
    plan = []
    for stem in stems:
        src = src_dir / f"{stem}.png"
        plan.append({
            "stem": stem,
            "source_image": str(src),
            "exists": src.exists(),
            "out_dir": str(out / stem),
        })
    return plan


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("stems", nargs="*", default=None)
    ap.add_argument("--all", action="store_true",
                    help="run every PNG in the chromakeys drop")
    ap.add_argument("--out", default=str(OUT),
                    help="output directory (use a fresh one for new sweeps; "
                         "out-hy3d holds the round-2 bake-off record)")
    ap.add_argument("--src", default=str(CHROMAKEYS),
                    help="source image directory (default: the ship "
                         "chromakeys drop; wildlife heroes live elsewhere)")
    ap.add_argument("--resume", action="store_true",
                    help="skip stems whose mesh.obj already exists in --out")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--steps", type=int, default=50)
    ap.add_argument("--octree", type=int, default=320,
                    help="marching-cubes resolution; 320 is the quality/VRAM knee on 16GB")
    ap.add_argument("--guidance", type=float, default=5.5)
    ap.add_argument("--seed", type=int, default=1234,
                    help="fixed so both views of a pair face identical sampling noise")
    args = ap.parse_args()

    out = Path(args.out)
    src_dir = Path(args.src)
    if args.all:
        stems = sorted(p.stem for p in src_dir.glob("*.png"))
    else:
        stems = args.stems or DEFAULT_STEMS
    plan = build_plan(stems, out, src_dir)

    missing = [p["stem"] for p in plan if not p["exists"]]
    if missing:
        print(f"MISSING source images: {missing}")
        return 1

    if args.resume:
        done = [p for p in plan if (Path(p["out_dir"]) / "mesh.obj").exists()]
        plan = [p for p in plan if p not in done]
        print(f"--resume: skipping {len(done)} already-generated stems")

    print(f"{len(plan)} images -> {out}")
    for p in plan:
        print(f"  {p['stem']:30} {p['source_image']}")
    if args.dry_run:
        print("\n--dry-run: no model loaded, no GPU touched.")
        return 0

    import torch
    from hy3dgen.shapegen import Hunyuan3DDiTFlowMatchingPipeline
    from hy3dgen.shapegen.postprocessors import (
        DegenerateFaceRemover,
        FaceReducer,
        FloaterRemover,
    )

    out.mkdir(parents=True, exist_ok=True)
    t_load = time.time()
    pipe = Hunyuan3DDiTFlowMatchingPipeline.from_pretrained(MODEL_PATH)
    load_seconds = time.time() - t_load
    print(f"pipeline loaded in {load_seconds:.1f}s")

    results = []
    for p in plan:
        stem = p["stem"]
        out_dir = Path(p["out_dir"])
        out_dir.mkdir(parents=True, exist_ok=True)
        rec = {"stem": stem, "source_image": p["source_image"], "model": MODEL_PATH,
               "steps": args.steps, "octree_resolution": args.octree,
               "guidance_scale": args.guidance, "seed": args.seed}
        try:
            img = chroma_key(Image.open(p["source_image"]))
            img.save(out_dir / "keyed.png")

            t0 = time.time()
            mesh = pipe(
                image=img,
                num_inference_steps=args.steps,
                octree_resolution=args.octree,
                guidance_scale=args.guidance,
                generator=torch.manual_seed(args.seed),
                mc_algo="mc",  # skimage marching cubes: no nvcc on this box for diso/dmc
            )[0]
            rec["generate_seconds"] = time.time() - t0

            t1 = time.time()
            mesh = FloaterRemover()(mesh)
            mesh = DegenerateFaceRemover()(mesh)
            mesh = FaceReducer()(mesh)
            rec["postprocess_seconds"] = time.time() - t1

            mesh.export(out_dir / "mesh.obj")
            mesh.export(out_dir / "mesh.glb")
            mesh.export(out_dir / "mesh.stl")  # for the 3D print
            rec["mesh_path"] = str(out_dir / "mesh.obj")
            rec["n_verts"] = int(len(mesh.vertices))
            rec["n_faces"] = int(len(mesh.faces))
            rec["watertight"] = bool(mesh.is_watertight)
            rec["peak_vram_gb"] = torch.cuda.max_memory_allocated() / 2**30
            rec["status"] = "ok"
            print(f"  {stem:30} ok  {rec['generate_seconds']:6.1f}s  "
                  f"{rec['n_faces']:>7} faces  watertight={rec['watertight']}  "
                  f"peak {rec['peak_vram_gb']:.1f}GB")
        except Exception as exc:  # one bad image must not lose the batch
            rec["status"] = "failed"
            rec["error"] = f"{type(exc).__name__}: {exc}"
            print(f"  {stem:30} FAILED  {rec['error']}")
        results.append(rec)
        (out / "results-hy3d.json").write_text(json.dumps(
            {"model": MODEL_PATH, "load_seconds": load_seconds,
             "n_ok": sum(r["status"] == "ok" for r in results),
             "n_failed": sum(r["status"] == "failed" for r in results),
             "results": results}, indent=1))

    print(f"\n{sum(r['status'] == 'ok' for r in results)}/{len(results)} ok "
          f"-> {out / 'results-hy3d.json'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
