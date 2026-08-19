#!/usr/bin/env python3
"""4x Real-ESRGAN upscale for starred hero images (display/Meshy tier).

The mesh pipeline conditions at 512 and never benefits from more pixels —
this is for the OTHER consumers: KB display art, zoomable heroes, and
Meshy uploads. Composition is preserved exactly (unlike re-generating at a
higher resolution, which re-rolls the diffusion composition), silhouette
stays chroma-key compatible. Tiled inference, so VRAM stays ~2GB and it
can run alongside a bake.

    ~/sd-venv/bin/python upscale_hero.py <hero.png> [...] [-o outdir]

Weights: ~/models/esrgan/RealESRGAN_x4plus.pth (spandrel loader).
"""

import sys
from pathlib import Path

import numpy as np
import torch
from PIL import Image
from spandrel import ImageModelDescriptor, ModelLoader

WEIGHTS = Path.home() / "models/esrgan/RealESRGAN_x4plus.pth"
TILE, OVERLAP = 512, 32


def upscale(model, img: np.ndarray) -> np.ndarray:
    h, w, _ = img.shape
    s = model.scale
    out = np.zeros((h * s, w * s, 3), np.float32)
    weight = np.zeros((h * s, w * s, 1), np.float32)
    step = TILE - 2 * OVERLAP
    for y0 in range(0, h, step):
        for x0 in range(0, w, step):
            ya, xa = max(0, y0 - OVERLAP), max(0, x0 - OVERLAP)
            yb, xb = min(h, y0 + step + OVERLAP), min(w, x0 + step + OVERLAP)
            tile = torch.from_numpy(img[ya:yb, xa:xb].transpose(2, 0, 1))[None]
            tile = tile.cuda().float() / 255.0
            with torch.no_grad():
                up = model(tile)[0].clamp(0, 1).cpu().numpy().transpose(1, 2, 0)
            out[ya * s:yb * s, xa * s:xb * s] += up
            weight[ya * s:yb * s, xa * s:xb * s] += 1
    return (out / weight * 255).clip(0, 255).astype(np.uint8)


def main() -> int:
    args = [a for a in sys.argv[1:] if a != "-o"]
    outdir = None
    if "-o" in sys.argv:
        outdir = Path(sys.argv[sys.argv.index("-o") + 1])
        args.remove(str(outdir))
    model = ModelLoader().load_from_file(WEIGHTS)
    assert isinstance(model, ImageModelDescriptor)
    model.cuda().eval()
    for a in args:
        p = Path(a)
        img = np.asarray(Image.open(p).convert("RGB"))
        up = upscale(model, img)
        out = (outdir or p.parent) / f"{p.stem}_4k.png"
        out.parent.mkdir(parents=True, exist_ok=True)
        Image.fromarray(up).save(out)
        print(f"{p.name}: {img.shape[1]}x{img.shape[0]} -> {up.shape[1]}x{up.shape[0]} {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
