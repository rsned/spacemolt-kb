#!/usr/bin/env python3
"""Extract Gorgonia (the unique boss of The Maw, Alzirr) from its scene art
onto flat magenta, pipeline-ready. Reproduces heroes/gorgonia_scene.png from
~/Downloads/gargantua.webp — the creature interpenetrates a black hole,
accretion disk, starfield, and a ship, so no single matte works. Three masks
composited:

  1. BiRefNet (rembg) — semantic cut: removes black hole, ship, most disk.
  2. Border flood-fill over sky-coloured pixels — the frond crown's rim
     highlight stops the flood, giving a crisper lattice boundary than
     BiRefNet (the user's suggestion; star specks absorbed as islands).
  3. Ink-density classifier — thins the accretion-disk lava swirl: creature
     regions are threaded with near-black filament, the swirl is not.

Final = intersection, morphologically closed, largest connected component.
The lava skirt where the roots grip the disk survives by design — that
boundary is artistic judgment, and Meshy read it as a root splay (correctly).

    ~/sf3d-venv/bin/python extract_gorgonia_scene.py \
        [src=~/Downloads/gargantua.webp] [out=heroes/gorgonia_scene.png]
"""

import sys
from pathlib import Path

import numpy as np
import rembg
from PIL import Image
from scipy.ndimage import binary_closing, binary_propagation, label, uniform_filter

MAG = np.array([255, 0, 255], dtype=np.uint8)


def largest_component(mask: np.ndarray) -> np.ndarray:
    lab, _ = label(mask)
    sizes = np.bincount(lab.ravel())
    sizes[0] = 0
    return lab == sizes.argmax()


def main() -> int:
    src_path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path.home() / "Downloads/gargantua.webp"
    out_path = Path(sys.argv[2]) if len(sys.argv) > 2 else Path(__file__).parent / "heroes/gorgonia_scene.png"
    src = np.asarray(Image.open(src_path).convert("RGB")).astype(np.float32)
    r, g, b = src[..., 0], src[..., 1], src[..., 2]

    # 1. semantic matte
    birefnet = rembg.remove(Image.fromarray(src.astype(np.uint8)),
                            session=rembg.new_session("birefnet-general"))
    semantic = np.asarray(birefnet)[..., 3] > 25

    # 2. sky flood: dark-to-mid, cold/neutral pixels connected to top/side
    # borders (bottom border is disk, not sky); rim highlights block it
    lum = src.max(axis=2)
    cand = (lum < 115) & (b >= r - 12)
    seed = np.zeros(cand.shape, bool)
    seed[0, :] = seed[:, 0] = seed[:, -1] = True
    sky = binary_propagation(seed & cand, mask=cand)
    small = np.bincount(label(~sky)[0].ravel()) < 400  # absorb star specks
    sky |= small[label(~sky)[0]] & ~sky

    # 3. lava-swirl thinning by local filament-ink density
    ink = (src.mean(axis=2) < 55) & semantic
    inkfrac = uniform_filter(ink.astype(np.float32), size=61)
    lava = semantic & (inkfrac < 0.22) & (src.mean(axis=2) > 45)

    keep = semantic & ~sky & ~lava
    keep = binary_closing(keep, np.ones((3, 3)))
    keep = largest_component(keep)

    out = src.astype(np.uint8).copy()
    out[~keep] = MAG
    out_path.parent.mkdir(parents=True, exist_ok=True)
    Image.fromarray(out).save(out_path)
    print(f"kept {keep.mean()*100:.0f}% of frame -> {out_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
