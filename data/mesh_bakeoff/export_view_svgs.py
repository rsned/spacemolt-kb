#!/usr/bin/env python3
"""Export the three orthographic ship footprints as standalone SVG files.

Writes <kb_ship_id>_{top,side,front}.svg into data/footprints/, plus a
deterministic ship_footprint_svg.zip holding the same set.

These are OUTLINES ONLY -- the bare hull silhouette, no blueprint furniture.
Deck lines, cargo holds, bridge/cockpit stations and hatching are drawn by
data/footprints/blueprints/interiors.py at sheet-render time and live solely
in kb/ships/blueprints/<id>.svg.

Sources, both already keyed by KB ship id:

  top    data/footprints/hy3d-svg/<id>.svg   copied verbatim, keeping its
         data-* provenance attributes
  side   data/footprints/views/<id>.json     "side" path, wrapped
  front  data/footprints/views/<id>.json     "front" path, wrapped

All three sources are stored bow-right and share that convention, so the
export applies no mirror or rotation (make_blueprints.py flips the hero
vignette TO match these, not the other way round).

    python3 export_view_svgs.py [--no-zip] [ship_id ...]
    python3 export_view_svgs.py --zip-only --zip-out PATH

The SVGs are committed; the zip is not (see .gitignore). The deploy workflow
rebuilds it from them into kb/ via scripts/build-footprint-zip.sh, the same
way inject-overlay-images.sh restores the gitignored portrait copies.
"""
import argparse
import json
import shutil
import sys
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
FOOT = HERE.parent / "footprints"
VIEWS = FOOT / "views"
TOPS = FOOT / "hy3d-svg"
ZIP_NAME = "ship_footprint_svg.zip"

# Matches the fill the hy3d-svg top views already use, so the three files
# render as siblings.
FILL = "#d5d8dd"

# Fixed timestamp for every zip entry: the archive must be byte-identical
# across runs when the SVGs have not changed.
ZIP_EPOCH = (1980, 1, 1, 0, 0, 0)

SUFFIXES = ("_top.svg", "_side.svg", "_front.svg")


def esc(s: str) -> str:
    return (s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")
            .replace('"', "&quot;"))


def wrap(path_d, width, height, ship, stem, view, extra=""):
    """One silhouette path as a standalone corner-origin SVG document."""
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" '
        f'viewBox="0 0 {width:.1f} {height:.1f}"\n'
        f'  data-ship="{esc(ship)}" data-art-stem="{esc(stem)}" '
        f'data-view="{view}"{extra}>\n'
        f"<title>{esc(ship)} {view}</title>\n"
        f'<path d="{path_d}" fill-rule="evenodd" fill="{FILL}"/>\n'
        f"</svg>\n"
    )


def export(ship_id):
    """Write the three SVGs for one ship. Returns the filenames written."""
    vp = VIEWS / f"{ship_id}.json"
    tp = TOPS / f"{ship_id}.svg"
    if not vp.exists() or not tp.exists():
        return []

    v = json.loads(vp.read_text())
    stem = v.get("stem", ship_id)
    written = []

    # Top: copy as-is so the provenance attributes (data-aspect,
    # data-adjustments, data-frame-ambiguous) survive.
    dst = FOOT / f"{ship_id}_top.svg"
    shutil.copyfile(tp, dst)
    written.append(dst.name)

    side = wrap(v["side"], v["len_units"], v["height_units"],
                ship_id, stem, "side")
    (FOOT / f"{ship_id}_side.svg").write_text(side)
    written.append(f"{ship_id}_side.svg")

    # The front view carries the hull's true bilateral centerline, which is
    # not the bbox middle on an asymmetric hull; downstream alignment needs it.
    fc = v.get("front_center_units")
    extra = f'\n  data-front-center="{fc:.1f}"' if fc is not None else ""
    front = wrap(v["front"], v["beam_units"], v["height_units"],
                 ship_id, stem, "front", extra)
    (FOOT / f"{ship_id}_front.svg").write_text(front)
    written.append(f"{ship_id}_front.svg")

    return written


def prune(keep):
    """Drop top-level view SVGs for ships that no longer have views."""
    gone = []
    for p in FOOT.glob("*.svg"):
        if p.name.endswith(SUFFIXES) and p.name not in keep:
            p.unlink()
            gone.append(p.name)
    return gone


def write_zip(names, out=None):
    """Deterministic archive: sorted entries, fixed timestamps."""
    out = Path(out) if out else FOOT / ZIP_NAME
    out.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for name in sorted(names):
            info = zipfile.ZipInfo(name, date_time=ZIP_EPOCH)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o644 << 16
            z.writestr(info, (FOOT / name).read_bytes())
    return out


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("ships", nargs="*", help="ship ids (default: all)")
    ap.add_argument("--no-zip", action="store_true",
                    help="skip building " + ZIP_NAME)
    ap.add_argument("--zip-only", action="store_true",
                    help="zip the SVGs already on disk, exporting none")
    ap.add_argument("--zip-out", metavar="PATH",
                    help=f"write the archive here instead of {ZIP_NAME}")
    args = ap.parse_args(argv)

    if args.zip_only:
        names = sorted(p.name for p in FOOT.glob("*.svg")
                       if p.name.endswith(SUFFIXES))
        if not names:
            print("error: no view SVGs found to zip", file=sys.stderr)
            return 1
        out = write_zip(names, args.zip_out)
        print(f"{len(names)} entries -> {out} "
              f"({out.stat().st_size / 1e6:.1f} MB)")
        return 0

    ids = args.ships or sorted(p.stem for p in VIEWS.glob("*.json"))

    written, missing = [], []
    for sid in ids:
        got = export(sid)
        if got:
            written.extend(got)
        else:
            missing.append(sid)

    for sid in missing:
        print(f"warning: no views/top pair for {sid}, skipped",
              file=sys.stderr)

    # Only a full run may prune; a targeted run must not delete its siblings.
    if not args.ships:
        for name in prune(set(written)):
            print(f"pruned stale {name}")

    print(f"{len(written)} SVGs for {len(written) // 3} ships -> {FOOT}")

    if not args.no_zip:
        # A targeted run still zips the complete set on disk, not just the
        # ships it rewrote.
        allnames = sorted(p.name for p in FOOT.glob("*.svg")
                          if p.name.endswith(SUFFIXES))
        out = write_zip(allnames, args.zip_out)
        print(f"{len(allnames)} entries -> {out} "
              f"({out.stat().st_size / 1e6:.1f} MB)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
