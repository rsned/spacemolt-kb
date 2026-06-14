# Voidborn Animation Effects v2 — Design

**Date:** 2026-06-13
**Status:** Approved, ready for implementation plan
**Builds on:** `2026-06-13-voidborn-procedural-animation-design.md` (the v1 pipeline: `tools/animate.py`, the loop-seamless `fn(imgs, params, t)` contract, `render`/`encode_mp4`/`run_batch`, the gallery `<video>` integration).

## Context

The v1 procedural pipeline shipped four effects. After visual review the user kept two as-is (`chromatic-split` — "nice otherworldly sense"; `crossfade-drift` — "works as described") and asked for a more ambitious treatment of the H3 / abstract concepts:

- **fold-churn** read as a flat 2D domain warp. The user wants the *character body to stay solid* while the surrounding geometry (the glass hex ring in `h3_phase_fold`, the pentagon tunnel in `abstract_fold_a`) moves in a genuinely *mathematical* way — "like watching a rotating tesseract," which "would require actual higher dimensional math."
- **noise-dissolve** read as a flat fade. The user wants two distinct things instead: (a) for the abstract "probability field" (`abstract_phase_c`) a **Stargate-style** look — a fixed bright curve where two halves meet, with stars warping toward/away from it and self-similar 2nd/3rd-order curves behind it; (b) for `h3_phase_dissolve` a **body-shaped disintegration** ("turned to dust and the wind poofs them away"), not a whole-image fade.

Keyframe inspection confirmed feasibility: the H3 figures are bright/pale on a dark void (luminance-separable), `abstract_phase_c` already contains the primary curve + self-similar secondary curves + faint radial star texture, and `abstract_fold_a` is a nested-pentagon tunnel well suited to a rotating-polytope warp.

This spec adds three new effects plus a shared subject-mask helper. It does not remove the v1 effects.

## Goals

1. A reusable **subject mask** (auto from luminance, optional hand-painted override) separating figure from surroundings.
2. **`hyper-warp`** — displacement driven by a real 4D rotation projected back to 2D, applied to the existing keyframe; optional subject protection (body stays solid).
3. **`hyperspace-streak`** — Stargate "probability field": fixed curve, motion-blurred star streaks flowing toward/away, boosted with synthetic stars.
4. **`disintegrate`** — body-masked dust dissolve with wind, ping-pong so it reforms (seamless loop).
5. Rewire `anims.json` to use the new effects on the right keyframes; keep the v1 effects in the registry.
6. All effects obey the v1 loop-seamless contract (`fn(imgs, params, t)`, frame 0 == frame N).

## Non-goals

- No generative/ML model (still pure CPU numpy/Pillow + imageio-ffmpeg).
- No synthetic projected-polytope wireframe overlay (future enhancement, see below).
- No removal of v1 `fold-churn` / `noise-dissolve` (kept as generic tools, just unreferenced by default).
- No new output formats or gallery changes (the v1 gallery already renders any `anims.json` clip as a `<video>` tile; new effects' params show in the existing `<details>`).

## Architecture

All additions live in the existing `tools/animate.py`; tests extend `tools/test_animate.py`. The `fn(imgs, params, t) -> uint8 (H,W,3)` contract, `EFFECTS` registry, `render`, and `run_batch` are unchanged — new effects register the same way.

### Shared helper: `subject_mask(img, params) -> (H,W) float32 in [0,1]`

`img` is a float32 RGB array in [0,1]. `params` may carry a resolved `mask_path` (see below).

- **Override:** if `params.get("mask_path")` is set and the file exists, load it as grayscale, resize to `img`'s (H,W), return in [0,1]. (The batch/CLI layer resolves `<src>.mask.png` next to the keyframe and injects `mask_path` — `subject_mask` itself only checks `params`, so it stays pure/testable.)
- **Auto:** luminance `L = 0.299 R + 0.587 G + 0.114 B`; binary `L > mask_threshold`; keep the **largest connected component** (4-connectivity flood via a small numpy/SciPy-free label, or `scipy.ndimage.label` only if scipy is already importable — DO NOT add scipy as a dep; implement a simple iterative/stack flood fill on the boolean array); morphological **close** (max-then-min via `PIL.ImageFilter.MaxFilter`/`MinFilter` on the uint8 mask) to fill speckle; Gaussian **feather** (`PIL.ImageFilter.GaussianBlur(mask_feather)`); return float32 in [0,1].
- Params: `mask_threshold` (default 0.35), `mask_feather` (default 6.0).
- If the auto mask is empty (no bright blob), return an all-zeros mask — consumers must treat "no subject" gracefully (e.g. `hyper-warp protect_subject` then simply warps everything; `disintegrate` dissolves nothing, returns the source — acceptable, the keyframe just isn't a candidate).

### New effect: `hyper_warp(imgs, params, t)` → key `"hyper-warp"`, 1 input

Genuine 4D-rotation displacement of the existing keyframe.

1. Build normalized coords `u,v ∈ [-1,1]` over the image (u = 2x/(w-1) - 1, v = 2y/(h-1) - 1).
2. Embed each pixel into 4D: `p = (u, v, z0, w0)` where `z0 = r*cos(k*theta_polar)` and `w0 = r*sin(k*theta_polar)` with `r = sqrt(u^2+v^2)` (so the hidden z,w dimensions carry real radial structure rather than being constant — a constant would make the xw/yw rotation a trivial scale). Use `k = 1`.
3. Rotation angle `theta = 2*pi * t * turns` with integer `turns` (default 1) → at t=1 the rotation is a whole multiple of 2π → identity → seamless loop.
4. Apply 4D rotation composing the **xw, yw, and zw** plane rotations (the three involving the 4th axis — the ones with no 3D analog) each by `theta` (optionally scaled per-plane by fixed weights so motion isn't degenerate). A 4D rotation in plane (a,b) by θ rotates only coords a,b: `a' = a cosθ - b sinθ; b' = a sinθ + b cosθ`.
5. Perspective-project 4D→2D: `s = w_dist / (w_dist - p_w)` (guard `w_dist - p_w` away from 0), `u_proj = p_x * s`, `v_proj = p_y * s`.
6. Displacement (in pixels) = `amp * ((u_proj - u), (v_proj - v))` mapped back to pixel scale; `remap` the source image by it.
7. If `protect_subject` is truthy: compute `m = subject_mask(img, params)`; `out = warped*(1-m) + img*m` (feathered edge blends). The body stays put; surroundings tumble in 4D.

Params: `amp` (default 0.35, in normalized units → scaled by min(h,w)/2), `turns` (int, default 1), `w_dist` (default 2.5), `protect_subject` (default false), plus mask params when protecting. `imgs` length 1.

### New effect: `hyperspace_streak(imgs, params, t)` → key `"hyperspace-streak"`, 1 input

Stargate "probability field": fixed curve, star streaks flowing toward/away, boosted with synthetic stars.

1. **Curve detection (fixed):** `bright = L > curve_threshold`; this is the keyframe's painted curve(s) and stays put — it is composited back at full strength at the end so it never moves.
2. **Flow field:** for each pixel, a unit flow direction. Compute it as the direction toward the nearest bright-curve pixel (via a coarse distance-transform gradient — downsample the bright mask, compute nearest-curve direction on the small grid using an iterative approximation or a simple per-pixel gradient of a blurred distance proxy, upsample). `toward=true` flows inward toward the curve; `toward=false` flips sign (outward). Keep this affordable: operate at, say, 1/4 resolution for the flow then upsample with bilinear.
3. **Streak the keyframe:** accumulate the source sampled at increasing offsets along the flow with geometrically decaying weights (a directional motion blur of `streak_len` taps), producing star streaks from the painted texture.
4. **Boosted synthetic stars:** seed `n_stars` bright points (deterministic from `seed`) positioned near the curve; each lives at a parametric position along its flow line that advances by `t` (mod 1) so it streams and returns at t=1; draw each as a short motion-blur streak of length `streak_len`. Add (screen-blend / max) onto the streaked field.
5. Composite the **fixed bright curve** back on top (`max` with the original curve pixels) so the seam where the halves meet stays sharp and stationary.
6. Loop: the painted-streak layer is t-independent (constant directional blur); only the synthetic stars move, and their `(pos + t) mod 1` parametrization makes frame 0 == frame N.

Params: `streak_len` (default 24 taps), `n_stars` (default 240), `toward` (default true), `curve_threshold` (default 0.5), `seed` (default 0). `imgs` length 1.

**Risk note:** this is the effect whose first render is most likely to need visual tuning (streak length/decay, star count/brightness, flow quality). The plan should produce a first render and explicitly expect a tuning iteration via `anims.json` params — like v1 `noise-dissolve` did. The loop-seamlessness invariant is still unit-tested.

### New effect: `disintegrate(imgs, params, t)` → key `"disintegrate"`, 1 input

Body-shaped dust dissolve with wind; ping-pong so it reforms.

1. `m = subject_mask(img, params)` — the body region.
2. Dissolve front: per-pixel value-noise field `n ∈ [0,1]` (reuse `_value_noise(h,w,grain,seed)`), deterministic. `alpha = 0.5*(1-cos(2*pi*t))` (0→1→0, ping-pong, seamless). A masked pixel is "dust" where `n < alpha` (so the body erodes progressively and reforms).
3. Wind advection: dust pixels are displaced along `wind = wind_px * (cos(wind_angle), sin(wind_angle))` plus per-pixel `turbulence` jitter (from the noise field gradient), and faded toward 0 as they travel (so they thin out like blown dust). Implement as: build a displaced+faded copy of the masked body (`remap` with the wind offset; multiply alpha by a falloff), then where a pixel has become dust, replace the intact body contribution with the drifting/fading dust contribution.
4. Compose: `out = background_where_not_body + intact_body_where_not_dust + drifting_dust`. Background (non-masked) is untouched throughout.
5. Endpoints: at t=0 and t=1, `alpha=0` → no dust → original image. Seamless.

Params: `wind_angle` (radians, default 0.3), `wind_px` (default 60), `turbulence` (default 8), `grain` (default 3), `seed` (default 0), plus mask params. `imgs` length 1.

### `mask_path` resolution (CLI + batch layer)

So `subject_mask` stays pure, the layer that knows file paths resolves the override:
- In `render(srcs, effect, params, ...)`: before invoking the effect, if `effect` is one that uses a mask (`hyper-warp` with `protect_subject`, or `disintegrate`) and `params` has no `mask_path`, check for `<srcs[0]> + ".mask.png"` (i.e. `foo.png.mask.png`) — if it exists, inject `params["mask_path"]`. Keep this a small, explicit helper; do not over-generalize.
- Equivalent resolution applies in `run_batch` (paths already resolved relative to the spec dir before calling `render`, so `render`'s own check covers it).

### Effect → keyframe wiring (`anims.json`)

Rewrite the relevant items; leave chroma/crossfade untouched:

| clip file | src | effect | key params |
|---|---|---|---|
| anim_fold_phase.mp4 | h3_phase_fold.png | hyper-warp | `protect_subject:true, amp:0.4, turns:1` |
| anim_fold_v2.mp4 | h3_v_fold2.png | hyper-warp | `protect_subject:true, amp:0.4, turns:1` |
| anim_fold_v3.mp4 | h3_v_fold3.png | hyper-warp | `protect_subject:true, amp:0.4, turns:1` |
| anim_fold_abstract.mp4 | abstract_fold_a.png | hyper-warp | `amp:0.5, turns:1` |
| anim_fold_abstract_b.mp4 (NEW) | abstract_fold_b.png | hyper-warp | `amp:0.5, turns:1` |
| anim_dissolve_abstract.mp4 | abstract_phase_c.png | hyperspace-streak | `streak_len:28, n_stars:300, toward:true` |
| anim_dissolve_phase.mp4 | h3_phase_dissolve.png | disintegrate | `wind_angle:0.3, wind_px:70` |
| anim_chroma_* (3) | unchanged | chromatic-split | unchanged |
| anim_crossfade_fusion.mp4 | unchanged | crossfade-drift | unchanged |

(Group titles/subtitles in `anims.json` updated to describe the new effects. `abstract_fold_b.png` existence to be confirmed at plan time; if absent, drop that row.)

### Future enhancements (noted, not built)

- **4D approach (2):** render a true rotating tesseract / 5-cell wireframe (real 4D→3D→2D projection) as a synthetic overlay.
- **4D approach (3):** hybrid — `hyper-warp` the keyframe AND overlay the projected polytope locked to the same rotation.
- A `boosted-only` synthetic Stargate field (discard keyframe) if `hyperspace-streak` on the painting proves too faint.

## Error handling

- Unknown effect / wrong input count: unchanged from v1 (`render` validates).
- Empty subject mask: graceful (warp-all / dissolve-nothing), as specified above — no error.
- Missing `<src>.mask.png`: not an error; fall back to auto mask.
- `w_dist - p_w` near zero in `hyper-warp`: clamp the denominator to avoid divide-by-zero / extreme displacement.
- Odd dims, encoding: unchanged (`ensure_even`, `macro_block_size=1`).

## Testing

Per new unit (extend `tools/test_animate.py`):

- **subject_mask:** on a synthetic image with a bright centered blob on black, the mask is ~1 inside the blob, ~0 in the corners, dtype float32, shape (H,W). On an all-black image, returns all-zeros. With an injected `mask_path` to a written grayscale PNG, returns that mask (resized).
- **hyper-warp:** shape/dtype; loop-seamless (`_loops <= tolerance`; integer `turns` ⇒ t=0 and t=1 displacement fields coincide — assert ≤1, or assert the displacement field equality directly to avoid resample rounding); with `protect_subject:true` on a bright-blob image, the blob region is closer to the original than the warped-everywhere result (mask actually protects).
- **hyperspace-streak:** shape/dtype; loop-seamless (t=0 vs t=1 within a small tolerance); a single bright point near a curve produces an elongated bright region along the flow (streak actually elongates); the detected fixed curve pixels remain present at full brightness across t.
- **disintegrate:** shape/dtype; loop-seamless (alpha=0 endpoints ⇒ t=0 and t=1 equal the source within ≤1); midpoint (t=0.5) differs substantially from the source inside the body; pixels OUTSIDE the body mask are unchanged at all t (background untouched).
- **render smoke:** render each new effect through `render()` from a real keyframe (or synthetic if keyframe absent) to a short MP4; assert frame count and nonzero file.

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`.

## Verification (whole feature)

- All tests pass.
- `./tools/animate --batch voidborn-concepts/anims.json` renders all clips (count matches the rewired set), `0 skipped`.
- `python3 tools/build_voidborn_gallery.py` regenerates `index.html`; clip count matches; `<video>` tiles present.
- Visual review of the new effects (expect a tuning pass on `hyperspace-streak`, possibly `disintegrate` wind).
- `git status`: tracked changes only `tools/animate.py`, `tools/test_animate.py`, `voidborn-concepts/anims.json` (force-added); `.mp4`/`index.html` remain gitignored.
