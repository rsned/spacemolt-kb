# Voidborn Animation Effects v3 — Design

**Date:** 2026-06-14
**Status:** Approved, ready for implementation plan
**Builds on:** v2 (`2026-06-13-voidborn-animation-v2-design.md`) and the polytope overlay. Refines two shipped effects after visual review.

## Context

Visual review of the v2 clips produced concrete feedback:

**`disintegrate` (h3_phase_dissolve):**
- Only part of the head dissolves; the dissolve boundary "catches on cheekbones, eye sockets, jawline." **Root cause (diagnosed):** `subject_mask` is a luminance threshold + largest connected component with **no hole-filling**, so the mid-tone face interior (eye socket, cheek shadow, lips, jawline) falls below threshold and becomes interior holes. Only the bright rim/hair/halo is masked, so only that dissolves; the unmasked face stays solid and the dust edges trace the feature-shaped holes.
- Needs to **complete the fade-out** (figure fully gone at the peak) and run **a few frames longer**. The current model leaves residual `dust = body·noise` at the peak, so it never fully clears.
- The dissolved area should **reveal the background, not force black**. User chose **both**: inpaint the keyframe's own void for the gallery mp4 **and** emit a true-alpha version for future compositing.

**`hyperspace-streak` (abstract_phase_c):**
- Streaks are **barely visible** — want them **brighter** and **faster**, to highlight stars **approaching the curve**.
- Synthetic streaks should **match the keyframe's existing star-streaks** (long, thin, radial — visible in the corners of `abstract_phase_c`) and there should be **more of them**.

Environment confirmed: bundled ffmpeg has `libvpx-vp9` and `yuva420p` — alpha WebM is viable.

## Goals

1. Fill mask holes so the **entire figure** dissolves uniformly (also benefits `hyper-warp` protect).
2. Rewrite `disintegrate` so it **fully clears** at the peak and dissolves into a **reconstructed background**.
3. Emit an **alpha** version of disintegrate clips (transparent where dissolved) alongside the mp4.
4. Make `hyperspace-streak` brighter, faster, denser, and matched to the keyframe's radial streak look.

## Non-goals

- No ML/generative model (pure numpy/Pillow + imageio-ffmpeg).
- No change to the other effects (chromatic-split, crossfade-drift, hyper-warp behavior, polytope-overlay) beyond the shared mask improvement.
- No new gallery features for alpha (the gallery keeps showing the mp4; the webm is a compositing asset). The gallery builder is unchanged.

## Architecture

All changes in `tools/animate.py`; tests in `tools/test_animate.py`. Effects keep the `fn(imgs, params, t) -> uint8 (H,W,3)` contract for the mp4 path. The alpha output is a parallel, opt-in render path that does not change that contract.

### 1. `_fill_holes(mask_bool) -> bool`

Fill interior holes of a boolean mask: flood-fill the **background** inward from the image border (on `~mask`), using the same pure-numpy label-propagation style as `_largest_component` (or an iterative binary dilation of a border seed, intersected with `~mask`). Any background pixel **not** reachable from the border is an enclosed hole → set to foreground. Returns the filled mask.

### 2. `subject_mask` — apply hole-fill

After the largest-connected-component step and before the morphological close/feather, call `_fill_holes` so the returned mask is a **solid filled silhouette**. Everything else (threshold, downscale-for-CC, close, feather, `_MASK_CACHE`, override path) is unchanged. Result: the figure's interior is fully covered, so feature-shaped holes vanish.

### 3. `_inpaint_background(img, mask) -> (H,W,3)`

Reconstruct the scene behind the figure by diffusion inpainting:
- Start with `img`; set masked pixels to "unknown."
- Iterate: Gaussian-blur the image, then overwrite the **known** (unmasked) pixels back with their originals; repeat ~N times (e.g. 30–60) so background color/gradient diffuses into the masked hole.
- Returns an RGB plate where the masked region is filled with the surrounding void/star gradient.
- t-independent → cache in a module dict (`_BG_CACHE`, keyed `(id(img), img.shape, mask params)`), cleared in `render` like the others.

### 4. `disintegrate` — rewritten advected-alpha model

Compute a per-pixel **alpha/presence** `A` and dust color, then output for the mp4; the same `A` feeds the alpha path.

- `m = subject_mask(img, params)` (now hole-filled).
- `noise = _value_noise(h,w,grain,seed)` normalized to `[0, 1-band]` (so the peak fully clears). `band = float(params.get("fade_band", 0.25))`.
- `alpha_t = 0.5*(1-cos(2*pi*t))` — ping-pong 0→1→0 (seamless).
- **Static presence:** `presence = clip((noise + band - alpha_t)/band, 0, 1) * m` — a pixel is fully present until `alpha_t` reaches its `noise`, then fades to 0 over `band`. Because `noise ≤ 1-band`, at `alpha_t=1` every body pixel has `presence=0`.
- **Dissolve amount:** `diss = clip((alpha_t - noise)/band, 0, 1)` (= `1 - presence/m` within the band).
- **Wind advection (inverse remap):** `off = (wind_px*[cosθ,sinθ] + turb) * diss`, `turb = (noise-0.5)*turbulence`; sample dust from where it came:
  - `A = remap(presence[...,None], xs-off_x, ys-off_y)[...,0]`
  - `dust = remap(img * m[...,None], xs-off_x, ys-off_y)`
- **Inpainted background:** `bg = _inpaint_background(img, m)`.
- **mp4 output (RGB):** `out = bg*(1-A[...,None]) + dust*A[...,None]` → `to_uint8`. Endpoints: at `t=0`/`t=1`, `alpha_t=0` ⇒ `presence=m`, `off=0` ⇒ `A=m`, `out = bg*(1-m)+ (img·m)·m ≈ img` (original); at `t=0.5`, `A≈0` ⇒ `out ≈ bg` (fully dissolved). Background (outside `m`) is `bg=img` there, untouched.
- **alpha (RGBA) frame** (for the alpha path): `rgba = concat(dust, A)` — color = dust, alpha = `A`; transparent where dissolved.

Params: `wind_angle` (0.3), `wind_px` (70), `turbulence` (8), `grain` (3), `seed` (0), `fade_band` (0.25), plus mask params. 1 input.

To expose the RGBA without breaking the effect contract, factor the core into `_disintegrate_fields(img, params, t) -> (rgb_uint8, rgba_float)`; `disintegrate(imgs, params, t)` returns the `rgb_uint8`. The alpha render path calls `_disintegrate_fields` for the RGBA frames.

### 5. Alpha output path

- `encode_webm_alpha(frames_rgba, out, fps, size)` — VP9/`yuva420p` via `imageio_ffmpeg.write_frames(out, size, pix_fmt_in="rgba", pix_fmt_out="yuva420p", codec="libvpx-vp9", fps=fps, macro_block_size=1)`. Frames are `(H,W,4)` uint8. Fallback (only if VP9-alpha fails at runtime): write a PNG sequence to `<out_without_ext>_frames/####.png`.
- `render` gains awareness: when the item requests alpha (see wiring) and the effect is `disintegrate`, after writing the mp4 it also produces `<out with .webm>` by generating RGBA frames via `_disintegrate_fields` and calling `encode_webm_alpha`. Implement as a small, explicit branch keyed on a new `alpha` argument to `render` (default false); `run_batch` passes `item.get("alpha", False)`.
- The `.webm` lives in `voidborn-concepts/` (gitignored, like the mp4s).

### 6. `hyperspace-streak` — brighter, faster, denser, matched

- **Brighter:** composite synthetic stars by **screen** (`1-(1-a)(1-b)`) rather than `max`, lift per-star brightness, and whiten the default tint toward `[0.85, 0.92, 1.0]`. Add an `intensity` param (default 1.6) multiplying star brightness before clip.
- **Faster:** `speed` param (default 2.0) multiplies `max_travel` so stars cross more distance per loop.
- **Denser:** default `n_stars` 240 → 700.
- **Match keyframe streaks:** longer, thinner radial streaks — increase default `streak_len` (24 → 40) and keep 1px-wide trails; stars flow along the curve-directed field (already radial).
- **Approach emphasis:** ramp each star's brightness up as it nears the curve — weight the per-tap brightness by proximity to the curve (e.g. multiply by a factor that increases as the star's flow position approaches a bright-curve pixel), so arrivals "flare."
- The painted-streak (keyframe motion-blur) layer and curve compositing are unchanged; only the synthetic-star layer and tint/intensity change. Loop-seamlessness preserved (synthetic stars still use `(phase+t) mod 1` with the sine envelope; `_STREAK_CACHE` key unchanged).

### 7. Wiring (`anims.json`)

- `anim_dissolve_phase`: `duration` 5 → 7; add `"alpha": true`; params keep wind defaults (tune after review).
- `anim_dissolve_abstract` (hyperspace): bump params — `streak_len: 40, n_stars: 700, speed: 2.0, intensity: 1.6, toward: true`.
- All other items unchanged. Clip count stays 14 (the disintegrate clip additionally emits a sibling `.webm`).

## Error handling

- `_fill_holes` on an all-false or all-true mask returns it unchanged (no enclosed holes).
- Empty mask → disintegrate dissolves nothing; `_inpaint_background` returns `img` (no masked region) — graceful.
- VP9-alpha encode failure → PNG-sequence fallback (logged).
- Projection/encode/odd-dims unchanged.

## Testing (extend `tools/test_animate.py`)

- **`_fill_holes`:** a filled square with a punched interior hole → hole filled; a mask with no enclosed hole is unchanged; all-zero stays all-zero.
- **`subject_mask` coverage rises:** on a synthetic blob with a dark interior notch, the filled mask covers the notch (coverage strictly greater than without fill / the notch pixel becomes ≥0.5).
- **`disintegrate` completes:** at `t=0.5`, inside the body region the output ≈ the inpainted background (mean abs diff small) and the RGBA alpha mean over the body ≈ 0; at `t=0`/`t=1` output ≈ source (≤ small tol); background (corner) pixels unchanged across t; loop-seamless.
- **`_disintegrate_fields` RGBA:** returns `(H,W,4)`; alpha in [0,255]; alpha at t=0 ≈ mask, at t=0.5 ≈ 0.
- **`encode_webm_alpha`:** writes a nonzero `.webm` from a few RGBA frames; readable back (frame count matches). (If VP9 unavailable in CI-less local run, assert the PNG-seq fallback produced files.)
- **`hyperspace_streak` brighter:** with the new defaults, mean brightness over a dark base exceeds the v2 defaults' mean (or simply exceeds a threshold); loop-seamless; curve preserved.

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`.

## Verification (whole feature)

- All tests pass.
- `./tools/animate --batch voidborn-concepts/anims.json` → 14 clips, `0 skipped`; `anim_dissolve_phase.webm` also produced (alpha).
- Visual review: disintegrate fully clears into the void and the figure dissolves evenly (no feature-catching); the `.webm` is transparent where dissolved; hyperspace streaks are bright, fast, dense, radial.
- `python3 tools/build_voidborn_gallery.py` → 14 `<video>` tiles (mp4s).
- `git status`: tracked changes only `tools/animate.py`, `tools/test_animate.py`, `voidborn-concepts/anims.json`; `.mp4`/`.webm`/`index.html` remain gitignored.

## Notes

- The disintegrate rewrite is the substantive change; the mask hole-fill is the root-cause fix that makes it (and `hyper-warp` protect) behave. The alpha path reuses the same `A`.
- Expect a possible second tuning pass on `hyperspace-streak` brightness/speed and disintegrate `wind_px`/`fade_band` via `anims.json` after viewing.
