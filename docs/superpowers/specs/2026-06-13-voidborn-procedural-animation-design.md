# Voidborn Procedural Animation Pipeline — Design

**Date:** 2026-06-13
**Status:** Approved, ready for implementation plan

## Context

The Voidborn concept-art exploration (`voidborn-concepts/`, see the project memory
`project_voidborn_concept_gen`) produced ~72 still keyframes with FLUX.1-schnell. The next
staged goal is generative image-to-video, but that lane is blocked on hardware (the 2 TB drive
is not yet installed; root is 93 % full at ~15 GB free).

This spec covers the **low-cost complement** that runs today with no model and no large
download: a procedural compositing pipeline that animates the existing keyframes into seamless
looping MP4 clips using numpy/Pillow + `imageio-ffmpeg`. It gives frame-precise control over the
specific H3 phase/dimensional effects called out in the plan. It deliberately does **not**
attempt anything that needs a generative model (new geometry, the H4 crowd shot) — those remain
deferred to the model lane (LTX-Video on NAS or the 2 TB drive).

## Environment (verified 2026-06-13)

- `~/sd-venv`: `diffusers 0.38.0`, `torch 2.5.1+cu121`, `numpy 2.4.6`, `pillow 12.2.0`,
  `transformers 5.11.0`, `accelerate 1.13.0`.
- **Missing:** `imageio-ffmpeg` (not installed); **no system ffmpeg**. One-time install of
  `imageio-ffmpeg` into the venv bundles a private ffmpeg binary (no sudo).
- GPU not required for this pipeline (pure CPU numpy/Pillow); RTX 2000 Ada is free if ever wanted.
- Keyframes confirmed present in `voidborn-concepts/`: `h3_phase_fold.png`, `h3_v_fold2.png`,
  `h3_v_fold3.png`, `h3_v_double2.png`, `h3_phase_dissolve.png`, `abstract_fold_a.png`,
  `abstract_phase_c.png`, `fusion_*_double.png`, `scene_monitored_*.png`, etc.

## Goals

1. Render seamless **looping** H.264 MP4 clips from existing keyframes — no model.
2. Four named effects with frame-precise, reproducible parameters.
3. Two ways to drive it: a CLI wrapper (one-off experiments) and a data-driven `anims.json`
   batch (reproducible recipes, mirrors the `prompts.json` gallery pattern).
4. Surface the clips in the existing gallery as `<video>` tiles alongside the stills.

## Non-goals

- No generative video model. No new geometry. No H4 crowd shot.
- No GIF output (poor for the crystalline gradients; MP4 only).
- No warm daemon (there is no model to keep resident; each render is a short CPU job).

## Architecture

Three tracked artifacts (the `voidborn-concepts/*.mp4` outputs themselves stay gitignored, like
the PNGs):

### 1. `tools/animate.py` — core library + CLI

- **Image I/O:** load a keyframe as a float32 numpy array in `[0,1]`, shape `(H, W, 3)`.
- **Frame generation:** each effect is a generator `frames(imgs, params, n) -> yields (H,W,3) uint8`,
  where `n = round(duration * fps)`.
- **Loop seamlessness (core invariant):** every effect is parameterized by a phase
  `t = i / n` for frame `i` in `0..n-1`, and every time-varying quantity is a function of
  `2*pi*t` (or a `t`↔`1-t` ping-pong). Therefore frame `n` would equal frame `0`; we emit frames
  `0..n-1` so the MP4 loops with no seam.
- **Encoding:** `imageio_ffmpeg.write_frames` (or `imageio.v3`/`imageio-ffmpeg` writer) to H.264,
  `pix_fmt=yuv420p`, `-movflags +faststart`; pad/crop to **even** width/height (yuv420p requires
  it). Default `fps=24`.

#### Effects

| effect key | inputs | description | default params |
|---|---|---|---|
| `fold-churn` | 1 keyframe | per-pixel sinusoidal **displacement field** sampled with bilinear remap; field phase advances with `t`, amplitude/spatial-frequency configurable → folds breathe/churn | `amp_px=12`, `freq=2.0`, `cycles=1` |
| `chromatic-split` | 1 keyframe | R/G/B channels offset along an axis by `±d(t)`, where `d(t)` goes `0→max→0` (ping-pong) → channels drift apart off-axis then re-merge; optional whole-frame off-axis drift | `max_px=18`, `angle_deg=20`, `drift_px=6` |
| `noise-dissolve` | 1 keyframe | alpha-blend between the image and a structured probability-noise field; `alpha(t)` ping-pongs `0→1→0` → dissolves into noise and reforms. Noise = low-freq value-noise tinted to the frame palette, not white static | `max_noise=0.85`, `grain=4` |
| `crossfade-drift` | 2 keyframes | slow off-axis parallax: each frame pans the two images in opposite directions by a few px while crossfading A→B→A | `drift_px=10`, `angle_deg=8` |

- Displacement remap uses a precomputed coordinate grid + bilinear sample (numpy gather or
  `PIL.Image.transform`/`scipy`-free manual bilinear) to avoid extra deps.

#### CLI

```
tools/animate <keyframe.png> [keyframe2.png] \
    --effect fold-churn --duration 4 --fps 24 \
    --amp 12 --freq 2.0 \
    -o voidborn-concepts/anim_fold_churn.mp4
```

- `--batch <anims.json>` renders every clip in the file instead of a single one.
- Per-effect params exposed as flags; sensible defaults from the table above.

### 2. `tools/animate` — shell wrapper

Mirrors `tools/portrait`: a thin script that execs `~/sd-venv/bin/python <repo>/tools/animate.py "$@"`
so it runs without manually activating the venv.

### 3. `voidborn-concepts/anims.json` — batch recipes

Mirrors `prompts.json`. A `groups` list; each item:

```json
{
  "file": "anim_h3_fold_churn.mp4",
  "src": ["h3_phase_fold.png"],
  "effect": "fold-churn",
  "params": { "amp_px": 12, "freq": 2.0 },
  "duration": 4, "fps": 24,
  "label": "H3 fold churn",
  "group": "Voidborn · animations",
  "fav": false
}
```

`src` is a list to accommodate two-keyframe effects (`crossfade-drift`). `tools/animate
--batch anims.json` reads this and writes each `file` into `voidborn-concepts/`.

### 4. Gallery integration — `tools/build_voidborn_gallery.py`

- Additionally load `anims.json` (if present) and render its items as **video tiles**:
  `<video src="..." autoplay loop muted playsinline>` reusing the existing `figure`/`figcaption`
  card styling.
- The collapsible `<details>` shows the effect name + params (the animation analogue of the
  still's seed+prompt) so each clip's recipe is one click away.
- Stills (`prompts.json`) and animations (`anims.json`) appear on the same `index.html` page.
- Total count line accounts for both stills and clips.

## Data flow

```
keyframe.png ──► animate.py (load → effect frames(t) → ffmpeg encode) ──► clip.mp4
anims.json ──► animate.py --batch ──► many clip.mp4 in voidborn-concepts/
prompts.json + anims.json ──► build_voidborn_gallery.py ──► index.html (img + video tiles)
```

## Error handling

- Missing keyframe / unreadable image → clear error naming the missing path; in `--batch`, skip
  that item with a warning and continue (don't abort the whole batch).
- Unknown `effect` key → error listing valid effect names.
- `crossfade-drift` given ≠2 sources, or a 1-input effect given 2 → explicit validation error.
- Odd dimensions handled automatically (pad to even) so yuv420p never fails.
- `imageio-ffmpeg` not installed → actionable message (`pip install imageio-ffmpeg` into the venv).

## Testing / verification

- Unit-level: for each effect, assert frame 0 and a synthesized frame `n` (phase `t=1`) are equal
  (loop-seamlessness invariant), and that output frame count == `round(duration*fps)` and dtype
  is uint8 with even dims.
- Smoke: render one short clip per effect from a real keyframe; verify the `.mp4` exists, is
  nonzero, and `imageio-ffmpeg` can read back the expected frame count.
- Gallery: run `build_voidborn_gallery.py`, confirm `index.html` contains `<video>` tiles for the
  `anims.json` items and still renders the existing stills.

## Initial clip set (first batch in anims.json)

From the plan's "target loops/clips":
1. `fold-churn` on `h3_phase_fold`, `h3_v_fold2`, `h3_v_fold3`, `abstract_fold_a`.
2. `chromatic-split` on `h3_v_double2`, `fusion_amber_double`, `fusion_sapphire_double`.
3. `noise-dissolve` on `h3_phase_dissolve`, `abstract_phase_c`.
4. `crossfade-drift` on a fusion pair (e.g. `fusion_amber_fold` ↔ `fusion_sapphire_fold`).

## Out of scope / deferred

- Generative image-to-video (LTX-Video etc.) on the NAS or 2 TB drive — separate lane.
- H4 networked-hive crowd shot (needs a model).
