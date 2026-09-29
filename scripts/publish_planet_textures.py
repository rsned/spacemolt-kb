#!/usr/bin/env python3
"""Publish planet surface textures to the KB as WebP, overlays first.

generate-planet-maps renders equirect PNGs (~2 MB each); the site serves WebP
(~30-220 KB at q85). Any <name>.png under overlays/planets/ is an explicit
replacement (e.g. Sol's real NASA textures) and always wins over the render of
the same planet, whichever generator produced it. Old PNGs left in the output
directory are removed so pages can only reference the WebP.

    python3 scripts/publish_planet_textures.py [--src DIR] [--out kb/images/planets]
"""
import argparse
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parent.parent
QUALITY = 85


def to_webp(src, dest, quality):
    Image.open(src).convert("RGB").save(dest, "WEBP", quality=quality, method=6)


def publish(src, out, overlays, quality=QUALITY):
    out.mkdir(parents=True, exist_ok=True)
    overlay = {p.stem: p for p in sorted(overlays.glob("*.png"))} if overlays.is_dir() else {}
    rendered = 0
    for png in sorted(src.glob("*.png")):
        if png.stem not in overlay:
            to_webp(png, out / f"{png.stem}.webp", quality)
            rendered += 1
    for stem, png in overlay.items():
        to_webp(png, out / f"{stem}.webp", quality)
    removed = 0
    for png in sorted(out.glob("*.png")):
        png.unlink()
        removed += 1
    return {"rendered": rendered, "overlaid": len(overlay), "removed_png": removed}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--src", type=Path, default=ROOT / "kb" / "images" / "planets",
                        help="directory of rendered PNGs (generate-planet-maps -outdir)")
    parser.add_argument("--out", type=Path, default=ROOT / "kb" / "images" / "planets")
    parser.add_argument("--overlays", type=Path, default=ROOT / "overlays" / "planets")
    parser.add_argument("--quality", type=int, default=QUALITY)
    args = parser.parse_args()
    print(publish(args.src, args.out, args.overlays, args.quality))


if __name__ == "__main__":
    main()
