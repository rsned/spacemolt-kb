#!/usr/bin/env python3
"""One-command hero-image -> blueprint pipeline for one-off ships.

The bulk sweep is done; this packages the whole chain for the one-offs
that follow (new ship launched, hero regenerated, other assets wanting
the same treatment). Two phases with a human triage gate between them,
handed off through an exported verdict file so either half can be rerun
independently:

PHASE A -- bake to triage:
    python3 shipwright.py new <hero.png or stem> [--stem NAME] [--rebake]
  * stages the hero into the chromakeys drop (magenta or any flat backdrop)
  * Hunyuan3D bake with the production sweep recipe (hy3d-venv, GPU:
    steps 50, octree 320, guidance 5.5, seed 1234, mc, 40k faces)
  * default "td" extraction + renders, then builds the per-ship triage
    page (make_ship_triage.py) and prints its :8478 URL
  The user reviews it, sets top-view / rot90 / flip / mirror / vflip /
  sym / solo / stretch, clicks the cockpit window, and exports
  shipwright_<stem>.json.

PHASE B -- verdict to deliverables:
    python3 shipwright.py finish <shipwright_STEM.json> [--id KB_ID]
  * merges the verdict into adjustments-final.json + the window survey
    (window_px_overrides.json still wins if the stem is curated there)
  * maps the stem to its KB ship id (ship_id_map.json; --id to add a
    mapping for a new stem)
  * replays the chain: apply_adjustments (memory-capped) ->
    make_svg_footprints -> compute_scale -> make_views ->
    make_blueprints (sheet + gallery) -> make_views_gallery
  * collects deliverables/<id>/: footprint SVG, side/front views JSON,
    blueprint sheet SVG, mesh as OBJ + STL (adjusted mesh when stretched)

deliverables/ is gitignored -- moving files to their final home is a
follow-on task. A catalog ship must exist in spacemolt-knowledge.db for
the blueprint sheet (scale ladder + registry stats); art-only stems stop
after the footprint/views steps with a warning.

Run under system python3; each step shells into its own venv.
"""
import argparse
import json
import shutil
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
FOOT = HERE.parent / "footprints"
CHROMAKEYS = Path.home() / "Downloads" / "chromakeys"
HY3D_PY = str(Path.home() / "hy3d-venv" / "bin" / "python")
SF3D_PY = str(Path.home() / "sf3d-venv" / "bin" / "python")
MEMCAP = ["systemd-run", "--user", "--scope", "-q",
          "-p", "MemoryMax=8G", "-p", "MemorySwapMax=0"]


def run(label: str, cmd: list[str], **kw) -> None:
    print(f"\n=== {label}\n    $ {' '.join(str(c) for c in cmd)}")
    r = subprocess.run([str(c) for c in cmd], cwd=str(HERE), **kw)
    if r.returncode != 0:
        sys.exit(f"FAILED: {label} (exit {r.returncode})")


def cmd_new(args) -> None:
    src = Path(args.hero)
    if src.exists() and src.suffix.lower() == ".png":
        stem = args.stem or src.stem
        dst = CHROMAKEYS / f"{stem}.png"
        if src.resolve() != dst.resolve():
            shutil.copy(src, dst)
            print(f"staged hero -> {dst}")
    else:
        stem = args.hero          # already-staged stem
        if not (CHROMAKEYS / f"{stem}.png").exists():
            sys.exit(f"no hero: {args.hero} is neither a png path nor a "
                     f"stem in {CHROMAKEYS}")

    d = HERE / args.dir / stem
    if args.rebake or not (d / "mesh.obj").exists():
        run("Hunyuan3D bake (GPU)",
            [HY3D_PY, "run_hy3d.py", stem, "--out", args.dir])
        if not (d / "mesh.obj").exists():
            sys.exit("bake produced no mesh — see run output")
    else:
        print(f"mesh exists, skipping bake (--rebake to force): {d}")

    mini = d / "shipwright_extract.json"
    prior = json.loads((HERE / "adjustments-final.json").read_text()
                       ).get(stem, {})
    mini.write_text(json.dumps({stem: prior}))
    run("default extraction (memory-capped)",
        MEMCAP + [SF3D_PY, "apply_adjustments.py", str(mini),
                  "--dir", args.dir])
    run("triage page",
        [HY3D_PY, "make_ship_triage.py", stem, "--dir", args.dir])
    print(f"\nnext: open the URL above (server: python3 -m http.server 8478 "
          f"in {HERE}), set the verdicts, export, then\n"
          f"    python3 shipwright.py finish shipwright_{stem}.json")


def cmd_finish(args) -> None:
    verdict = json.loads(Path(args.verdict).read_text())
    stem = verdict["stem"]
    adj = verdict.get("adjustments") or {}
    d = HERE / args.dir / stem
    if not (d / "mesh.obj").exists():
        sys.exit(f"no mesh at {d} — run `shipwright.py new` first")

    # 1. adjustments-final.json — the canonical per-stem corrections
    adj_f = HERE / "adjustments-final.json"
    all_adj = json.loads(adj_f.read_text())
    if adj:
        all_adj[stem] = adj
    else:
        all_adj.pop(stem, None)
    adj_f.write_text(json.dumps(all_adj, indent=1))
    print(f"adjustments-final.json[{stem}] = {adj or '(default)'}")

    # 2. window survey
    w = verdict.get("window")
    if w:
        wp_f = FOOT / "scale" / "window_px.json"
        wp = json.loads(wp_f.read_text())
        wp[stem] = w
        wp_f.write_text(json.dumps(wp, indent=1))
        ovr = json.loads((FOOT / "scale" / "window_px_overrides.json")
                         .read_text())
        if stem in ovr:
            print(f"NOTE: {stem} is curated in window_px_overrides.json — "
                  f"the override still wins; edit it there if this new "
                  f"measurement should replace it")
        print(f"window_px.json[{stem}] = {w}")

    # 3. KB ship mapping
    map_f = HERE / "ship_id_map.json"
    idmap = json.loads(map_f.read_text())
    mapped = idmap["mapping"].get(stem)
    if mapped is None and args.id:
        faction = next((p[:-1] for p in ("nebula_", "solarian_", "outerrim_",
                                         "crimson_", "voidborn_")
                        if stem.startswith(p)), "")
        idmap["mapping"][stem] = {"id": args.id, "faction": faction,
                                  "match": "manual"}
        map_f.write_text(json.dumps(idmap, indent=1))
        mapped = idmap["mapping"][stem]
        print(f"ship_id_map.json: {stem} -> {args.id} (manual)")
    ship_id = mapped["id"] if mapped else stem
    if mapped is None:
        print(f"WARNING: {stem} has no KB ship mapping (pass --id to add "
              f"one); footprint/views proceed under the stem name, the "
              f"blueprint sheet is skipped")

    # 4. replay the chain
    mini = d / "shipwright_extract.json"
    mini.write_text(json.dumps({stem: adj}))
    run("re-extract with verdicts (memory-capped)",
        MEMCAP + [SF3D_PY, "apply_adjustments.py", str(mini),
                  "--dir", args.dir])
    run("footprint SVGs", [HY3D_PY, "make_svg_footprints.py",
                           "--dir", args.dir])
    run("scale survey", [sys.executable, str(FOOT / "scale" /
                                             "compute_scale.py")])
    run("side/front views", [SF3D_PY, "make_views.py", stem])
    if mapped is not None:
        run("blueprint sheet + gallery",
            [SF3D_PY, str(FOOT / "blueprints" / "make_blueprints.py"),
             ship_id])
    run("views triage gallery", [sys.executable, "make_views_gallery.py"])

    # 5. deliverables
    out = HERE / "deliverables" / ship_id
    out.mkdir(parents=True, exist_ok=True)
    mesh = d / ("mesh_adjusted.obj" if (d / "mesh_adjusted.obj").exists()
                else "mesh.obj")
    pairs = [
        (FOOT / "hy3d-svg" / f"{ship_id}.svg", f"{ship_id}_footprint.svg"),
        (FOOT / "views" / f"{ship_id}.json", f"{ship_id}_views.json"),
        (HERE.parent.parent / "kb" / "ships" / "blueprints" /
         f"{ship_id}.svg", f"{ship_id}_blueprint.svg"),
        (mesh, f"{ship_id}_mesh.obj"),
    ]
    for src, name in pairs:
        if src.exists():
            shutil.copy(src, out / name)
        else:
            print(f"  (no {name} — source missing: {src})")
    run("mesh -> STL",
        [SF3D_PY, "-c",
         "import sys, trimesh; trimesh.load(sys.argv[1], force='mesh', "
         "process=False).export(sys.argv[2])",
         str(mesh), str(out / f"{ship_id}_mesh.stl")])
    print(f"\ndeliverables -> {out}")
    for f in sorted(out.iterdir()):
        print(f"  {f.name}  ({f.stat().st_size // 1024} KB)")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    a = sub.add_parser("new", help="bake a hero image up to the triage gate")
    a.add_argument("hero", help="path to a hero PNG, or an already-staged "
                               "chromakeys stem")
    a.add_argument("--stem", help="art stem to file it under (default: "
                                  "the PNG's filename stem)")
    a.add_argument("--dir", default="out-hy3d-full")
    a.add_argument("--rebake", action="store_true",
                   help="re-run the GPU bake even if a mesh exists")
    a.set_defaults(fn=cmd_new)
    b = sub.add_parser("finish", help="apply an exported verdict and build "
                                      "all deliverables")
    b.add_argument("verdict", help="shipwright_<stem>.json from the triage "
                                   "page export")
    b.add_argument("--id", help="KB catalog ship id, to map a stem that "
                                "ship_id_map.json does not know yet")
    b.add_argument("--dir", default="out-hy3d-full")
    b.set_defaults(fn=cmd_finish)
    args = ap.parse_args()
    args.fn(args)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
