# Voidborn Polytope Overlay — Design

**Date:** 2026-06-13
**Status:** Approved, ready for implementation plan
**Builds on:** `2026-06-13-voidborn-animation-v2-design.md` (effects v2). Adds the deferred "future enhancement #2" — a true rotating 4D-polytope wireframe — as its own effect.

## Context

In v2, `hyper-warp` (4D-rotation displacement of the keyframe) read as a soft organic swirl on the abstract fold tunnel rather than a crisp "rotating tesseract." Visual review confirmed: `disintegrate` and `hyperspace-streak` landed well, but the 4D warp did not produce the literal higher-dimensional-geometry look the user wants. The chosen fix is to render an **actual** rotating 4D polytope (real 4D→3D→2D projection) as a glowing wireframe composited over the crisp, unwarped keyframe.

Decisions from brainstorming:
- **Base layer:** the crisp original keyframe (not the warped one, not black).
- **Shapes:** tesseract (8-cell), 5-cell (4-simplex), 16-cell (cross-polytope).
- **Wiring:** add new clips; keep every existing clip. `hyper-warp` stays as-is on the H3 character folds.

## Goals

1. A `polytope-overlay` effect that draws a genuine rotating 4D polytope wireframe over a keyframe.
2. Three polytopes (tesseract, 5-cell, 16-cell) via a `shape` param.
3. Seamless loop (exact: integer `turns` ⇒ frame 0 == frame N), pure CPU.
4. Add three new clips to `anims.json` without removing or altering existing ones.

## Non-goals

- No ML/generative model (pure numpy/Pillow).
- No change to `hyper-warp`, the other effects, or the gallery builder (the gallery already renders any `anims.json` clip as a `<video>` tile).
- Not the hybrid warp+overlay (enhancement #3) — base is the crisp keyframe only.

## Architecture

All additions live in `tools/animate.py`; tests extend `tools/test_animate.py`. The effect obeys the existing `fn(imgs, params, t) -> uint8 (H,W,3)` contract and registers in `EFFECTS`.

### `_polytope(shape) -> (verts, edges)`

Returns `verts` as a float32 array shape `(N, 4)` and `edges` as a list of `(i, j)` index pairs.

- **`"tesseract"`** — 16 vertices = the Cartesian product `{-1,+1}^4`; edges connect every pair of vertices differing in exactly one coordinate (Hamming distance 1) → 32 edges.
- **`"16-cell"`** — 8 vertices = `±1` along each of the 4 axes (i.e. `(±1,0,0,0)`, `(0,±1,0,0)`, `(0,0,±1,0)`, `(0,0,0,±1)`); edges connect every pair EXCEPT antipodal pairs (same axis, opposite sign) → 24 edges.
- **`"5-cell"`** — 5 vertices of a regular 4-simplex, built deterministically: take the 5 standard basis vectors of R⁵, subtract the centroid (`np.eye(5) - 1/5`), and project the centered rows onto a 4D orthonormal basis of the sum-zero hyperplane via `np.linalg.svd` (the first 4 right-singular vectors). Edges = all 10 vertex pairs.
- Unknown `shape` → `ValueError` listing valid names.

### `polytope_overlay(imgs, params, t)` → key `"polytope-overlay"`, 1 input

Per-frame:
1. `verts, edges = _polytope(shape)`; normalize verts so `max(|coord|) == 1` (divide by the max absolute coordinate over all verts/dims) — guarantees `|w|,|z| ≤ 1` for safe projection.
2. **4D rotation** by `theta = 2*pi*t*turns` (integer `turns`). Rotate through the xw, yw, and zw planes (each by `theta`) — the same rotation family `hyper_warp` uses, applied to the small vertex set. A plane-(a,b) rotation by θ: `a' = a*cosθ - b*sinθ; b' = a*sinθ + b*cosθ`.
3. **Project 4D→3D:** `f4 = d4 / (d4 - w)`, then `x,y,z *= f4`. `d4 = 2.5`.
4. **Project 3D→2D:** `f3 = d3 / (d3 - z)`, then `x,y *= f3`. `d3 = 3.0`. (Both denominators are safe since `|w|,|z| ≤ 1 < d`.)
5. **To pixels:** `px = cx + x * rad`, `py = cy + y * rad`, where `cx,cy` is the image center and `rad = size * min(h,w) / 2`.
6. **Draw edges** into an intensity buffer:
   - Render on a 2× supersampled single-channel (`"L"`) canvas with `ImageDraw.line([...], fill=255, width=max(1, width*2))` for each edge, then downsample to (w,h) with `Image.LANCZOS` → anti-aliased lines. Call this `sharp` (float [0,1]).
   - `glowed = GaussianBlur(sharp_uint8, glow)` (float [0,1]).
   - `intensity = clip(sharp + 0.6 * glowed, 0, 1)`.
7. **Composite:** `overlay = intensity[...,None] * color` (color is an RGB triple in [0,1]); screen-blend over the base keyframe: `out = 1 - (1 - base) * (1 - overlay)`; return `to_uint8(out)`.

**Loop:** at `t=0` and `t=1`, `theta` differs by `2*pi*turns` (a whole number of turns) ⇒ identical vertex positions ⇒ identical projected lines ⇒ identical frame. The pipeline has no per-frame randomness, so the loop is exact (test tolerance ≤1 for safety against float line-raster differences).

**Params:** `shape` (default `"tesseract"`), `turns` (int, default 1), `size` (default 0.7), `width` (px, default 2), `glow` (blur radius, default 6.0), `color` (RGB list, default `[0.6, 0.8, 1.0]`), `d4` (default 2.5), `d3` (default 3.0). `imgs` length 1. No caching (per-frame cost is ~10–32 line draws).

### Wiring (`anims.json`)

Add a new group AFTER the existing groups; do not modify existing items. `abstract_fold_a.png` and `abstract_fold_b.png` are confirmed present.

```
"Animations · 4D polytopes" — true rotating 4D wireframes over the fold concepts
  anim_poly_tesseract.mp4  src abstract_fold_a.png  shape tesseract  size 0.75
  anim_poly_16cell.mp4     src abstract_fold_b.png  shape 16-cell    size 0.75
  anim_poly_5cell.mp4      src abstract_fold_a.png  shape 5-cell     size 0.8
```
All `duration 6, fps 24, turns 1`. 11 existing clips → 14 total.

## Error handling

- Unknown `shape` → `ValueError` (listing valid shapes) from `_polytope`.
- Unknown effect / wrong input count → unchanged (`render` validates).
- Projection denominators are safe by construction (normalized verts, `d4=2.5`, `d3=3.0`); no special clamp required, but keep the normalization step so this invariant holds.
- Odd dims / encoding → unchanged (`ensure_even`, `macro_block_size=1`).

## Testing (extend `tools/test_animate.py`)

- **`_polytope` counts/validity:** tesseract → 16 verts, 32 edges; 16-cell → 8 verts, 24 edges; 5-cell → 5 verts, 10 edges. Every edge index is in `range(len(verts))`. Verts shape `(N,4)`. Unknown shape raises `ValueError`.
- **`polytope_overlay` shape/dtype:** returns uint8 `(H,W,3)`.
- **loop-seamless:** `_loops(polytope_overlay, [img], {"seed":...}) <= 1` (use a synthetic base; deterministic, expect 0–1).
- **brightens over base:** on a mostly-dark base, the output has more total brightness than the base (the wireframe adds light) — `out.sum() > to_uint8(base).sum()`.
- **screen preserves base:** a bright base pixel far from any line stays ≥ its original value (screen never darkens).
- **render smoke:** render `polytope-overlay` through `render()` from a real (or synthetic) keyframe to a short MP4; assert frame count and nonzero file.

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`.

## Verification (whole feature)

- All tests pass.
- `./tools/animate --batch voidborn-concepts/anims.json` → all clips render, `0 skipped`, count = 14.
- `python3 tools/build_voidborn_gallery.py` → 14 clips; `<video>` tiles present.
- Visual review of the three polytope clips (expect possible tuning of `size`/`width`/`glow`/`color` via `anims.json`).
- `git status`: tracked changes only `tools/animate.py`, `tools/test_animate.py`, `voidborn-concepts/anims.json`; `.mp4`/`index.html` remain gitignored.
