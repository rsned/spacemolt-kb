# Voidborn Procedural Animation Pipeline — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Animate existing Voidborn FLUX keyframes into seamless looping H.264 MP4 clips using a pure-CPU numpy/Pillow + imageio-ffmpeg pipeline (no generative model), with a CLI + data-driven batch, surfaced as `<video>` tiles in the existing gallery.

**Architecture:** A single core module `tools/animate.py` exposes four stateless per-frame effect functions (each a function of phase `t ∈ [0,1)` so frame 0 == frame N → seamless loop), a frame-count-driven renderer, and an `imageio-ffmpeg` MP4 encoder. A thin `tools/animate` shell wrapper runs it via `~/sd-venv`. `voidborn-concepts/anims.json` holds reproducible batch recipes. `tools/build_voidborn_gallery.py` is extended to render video tiles from `anims.json`.

**Tech Stack:** Python 3, numpy, Pillow, imageio-ffmpeg (bundles ffmpeg), pytest. Runs under `~/sd-venv`.

---

## File Structure

- **Create** `tools/animate.py` — core library + CLI. Image I/O, bilinear `remap`, four effect functions, `EFFECTS` registry, `render`, `encode_mp4`, `run_batch`, argparse CLI.
- **Create** `tools/test_animate.py` — pytest unit/smoke tests.
- **Create** `tools/animate` — shell wrapper (mirrors `tools/portrait`).
- **Create** `voidborn-concepts/anims.json` — batch clip recipes (gitignored dir, but this file is small — committed; see Task 13 note).
- **Modify** `tools/build_voidborn_gallery.py` — load `anims.json`, render `<video>` tiles, add video CSS, update count line.

All paths relative to repo root `/home/robert/spacemolt/kb`.

**Run tests with:** `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`

---

## Task 1: Environment setup

**Files:** none (venv only)

- [ ] **Step 1: Install imageio-ffmpeg and pytest into the venv**

Run:
```bash
~/sd-venv/bin/pip install imageio-ffmpeg pytest
```
Expected: successful install (imageio-ffmpeg downloads a bundled ffmpeg binary on first use).

- [ ] **Step 2: Verify the bundled ffmpeg is reachable**

Run:
```bash
~/sd-venv/bin/python -c "import imageio_ffmpeg; print(imageio_ffmpeg.get_ffmpeg_exe())"
```
Expected: prints a path to an `ffmpeg-*` binary (may download it on this first call). No error.

- [ ] **Step 3: Commit nothing** — this task changes only the venv (outside the repo). Proceed.

---

## Task 2: Core image helpers (`load_image`, `to_uint8`, `ensure_even`)

**Files:**
- Create: `tools/animate.py`
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Create `tools/test_animate.py`:
```python
import os
import sys

import numpy as np

sys.path.insert(0, os.path.dirname(__file__))
import animate  # noqa: E402


def _synth(h=64, w=48):
    rng = np.random.default_rng(7)
    return rng.random((h, w, 3), dtype=np.float32)


def test_to_uint8_clamps_and_rounds():
    arr = np.array([[[-0.5, 0.0, 1.0]]], dtype=np.float32)
    out = animate.to_uint8(arr)
    assert out.dtype == np.uint8
    assert out.tolist() == [[[0, 0, 255]]]


def test_ensure_even_pads_odd_dims():
    odd = np.zeros((63, 47, 3), dtype=np.float32)
    even = animate.ensure_even(odd)
    h, w = even.shape[:2]
    assert h % 2 == 0 and w % 2 == 0
    assert (h, w) == (64, 48)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'animate'`.

- [ ] **Step 3: Write minimal implementation**

Create `tools/animate.py`:
```python
#!/usr/bin/env python3
"""animate — procedurally animate Voidborn keyframes into looping MP4 clips.

Pure-CPU numpy/Pillow compositing (no generative model). Each effect is a
function of a loop phase t in [0,1) so frame 0 and frame N match: the MP4
loops seamlessly with <video loop>. See
docs/superpowers/specs/2026-06-13-voidborn-procedural-animation-design.md.
"""
import numpy as np
from PIL import Image


def load_image(path):
    """Load an image as float32 RGB array in [0,1], shape (H, W, 3)."""
    img = Image.open(path).convert("RGB")
    return np.asarray(img, dtype=np.float32) / 255.0


def to_uint8(arr):
    """Clamp a float array to [0,1] and convert to uint8 with rounding."""
    return (np.clip(arr, 0.0, 1.0) * 255.0 + 0.5).astype(np.uint8)


def ensure_even(arr):
    """Pad bottom/right by one pixel (edge replicate) so H and W are even.

    yuv420p requires even dimensions; we pad ourselves and use
    macro_block_size=1 so imageio-ffmpeg never silently resizes.
    """
    h, w = arr.shape[:2]
    ph, pw = h % 2, w % 2
    if ph or pw:
        arr = np.pad(arr, ((0, ph), (0, pw), (0, 0)), mode="edge")
    return arr
```

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): animate.py image helpers (load/to_uint8/ensure_even)"
```

---

## Task 3: Bilinear `remap` helper

**Files:**
- Modify: `tools/animate.py`
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
def test_remap_identity_returns_same_image():
    img = _synth()
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    out = animate.remap(img, xs, ys)
    assert np.allclose(out, img, atol=1e-4)


def test_remap_integer_shift():
    img = _synth()
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    # sample from one column to the right -> output shifted left by 1
    out = animate.remap(img, xs + 1.0, ys)
    assert np.allclose(out[:, :-1], img[:, 1:], atol=1e-4)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k remap -v`
Expected: FAIL — `AttributeError: module 'animate' has no attribute 'remap'`.

- [ ] **Step 3: Write minimal implementation**

Add to `tools/animate.py` (after `ensure_even`):
```python
def remap(img, map_x, map_y):
    """Bilinear-sample img at floating source coords (map_x, map_y).

    map_x/map_y are (H, W) arrays giving, for each output pixel, the source
    column/row to read. Coordinates are clamped to the image edges.
    """
    h, w = img.shape[:2]
    x = np.clip(map_x, 0, w - 1)
    y = np.clip(map_y, 0, h - 1)
    x0 = np.floor(x).astype(np.int64)
    y0 = np.floor(y).astype(np.int64)
    x1 = np.minimum(x0 + 1, w - 1)
    y1 = np.minimum(y0 + 1, h - 1)
    wx = (x - x0)[..., None]
    wy = (y - y0)[..., None]
    ia = img[y0, x0]
    ib = img[y0, x1]
    ic = img[y1, x0]
    idd = img[y1, x1]
    top = ia * (1 - wx) + ib * wx
    bot = ic * (1 - wx) + idd * wx
    return top * (1 - wy) + bot * wy
```

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k remap -v`
Expected: PASS (both remap tests).

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): bilinear remap helper for displacement effects"
```

---

## Task 4: `fold-churn` effect

**Files:**
- Modify: `tools/animate.py`
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
def _loops(fn, imgs, params=None):
    """A frame fn loops if its t=0 and t=1 frames match within rounding."""
    params = params or {}
    f0 = fn(imgs, params, 0.0).astype(np.int16)
    f1 = fn(imgs, params, 1.0).astype(np.int16)
    return int(np.abs(f0 - f1).max())


def test_fold_churn_shape_and_loop():
    img = _synth()
    frame = animate.fold_churn([img], {}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.fold_churn, [img]) <= 1
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k fold_churn -v`
Expected: FAIL — `AttributeError: module 'animate' has no attribute 'fold_churn'`.

- [ ] **Step 3: Write minimal implementation**

Add to `tools/animate.py`:
```python
def fold_churn(imgs, params, t):
    """Animated sinusoidal displacement field — non-euclidean folds churning.

    The field phase advances by 2*pi*cycles over the loop; with integer
    `cycles` the t=0 and t=1 fields coincide, so the clip loops seamlessly.
    """
    img = imgs[0]
    amp = float(params.get("amp_px", 12.0))
    freq = float(params.get("freq", 2.0))
    cycles = float(params.get("cycles", 1.0))
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    phase = 2.0 * np.pi * t * cycles
    kx = 2.0 * np.pi * freq / w
    ky = 2.0 * np.pi * freq / h
    dx = amp * np.sin(ky * ys + phase)
    dy = amp * np.cos(kx * xs + phase)
    return to_uint8(remap(img, xs + dx, ys + dy))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k fold_churn -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): fold-churn displacement effect"
```

---

## Task 5: `chromatic-split` effect

**Files:**
- Modify: `tools/animate.py`
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
def test_chromatic_split_shape_and_loop():
    img = _synth()
    frame = animate.chromatic_split([img], {}, 0.5)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.chromatic_split, [img]) <= 1
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k chromatic -v`
Expected: FAIL — `AttributeError: ... 'chromatic_split'`.

- [ ] **Step 3: Write minimal implementation**

Add to `tools/animate.py`:
```python
def chromatic_split(imgs, params, t):
    """RGB channels drift apart off-axis then re-merge (unify -> split -> unify).

    Split distance d(t) = max_px * 0.5*(1-cos(2*pi*t)) goes 0 -> max -> 0, and a
    shared whole-frame off-axis wobble uses sin(2*pi*t); both vanish at t=0 and
    t=1, so the loop is seamless.
    """
    img = imgs[0]
    max_px = float(params.get("max_px", 18.0))
    angle = np.deg2rad(float(params.get("angle_deg", 20.0)))
    drift_px = float(params.get("drift_px", 6.0))
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    d = max_px * 0.5 * (1.0 - np.cos(2.0 * np.pi * t))
    drift = drift_px * np.sin(2.0 * np.pi * t)
    ox, oy = np.cos(angle), np.sin(angle)
    r = remap(img, xs + d * ox + drift * oy, ys + d * oy - drift * ox)[..., 0]
    g = remap(img, xs + drift * oy, ys - drift * ox)[..., 1]
    b = remap(img, xs - d * ox + drift * oy, ys - d * oy - drift * ox)[..., 2]
    return to_uint8(np.stack([r, g, b], axis=-1))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k chromatic -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): chromatic-split channel-drift effect"
```

---

## Task 6: `noise-dissolve` effect

**Files:**
- Modify: `tools/animate.py`
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
def test_noise_dissolve_shape_and_loop():
    img = _synth()
    frame = animate.noise_dissolve([img], {"seed": 3}, 0.5)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.noise_dissolve, [img], {"seed": 3}) <= 1


def test_noise_dissolve_midpoint_differs_from_source():
    img = _synth()
    f_mid = animate.noise_dissolve([img], {"seed": 3, "max_noise": 0.85}, 0.5)
    f_start = animate.noise_dissolve([img], {"seed": 3}, 0.0)
    # at the dissolve peak the frame should differ substantially from the source
    assert np.abs(f_mid.astype(np.int16) - f_start.astype(np.int16)).mean() > 5
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k noise -v`
Expected: FAIL — `AttributeError: ... 'noise_dissolve'`.

- [ ] **Step 3: Write minimal implementation**

Add to `tools/animate.py`:
```python
def _value_noise(h, w, grain, seed):
    """Low-frequency value noise in [0,1], upsampled from a coarse grid.

    Reads as structured 'probability noise', not white static. Deterministic
    in (h, w, grain, seed) so the field is identical across frames (only the
    dissolve alpha varies with t), keeping the loop seamless.
    """
    rng = np.random.default_rng(seed)
    lh = max(1, h // max(1, grain))
    lw = max(1, w // max(1, grain))
    low = (rng.random((lh, lw), dtype=np.float32) * 255.0).astype(np.uint8)
    up = Image.fromarray(low).resize((w, h), Image.BILINEAR)
    return np.asarray(up, dtype=np.float32) / 255.0


def noise_dissolve(imgs, params, t):
    """Dissolve the image into palette-tinted probability noise and back.

    alpha(t) = max_noise * 0.5*(1-cos(2*pi*t)) goes 0 -> max -> 0 (seamless).
    The noise is tinted by the frame's mean color so it reads as the image
    coming apart into its own substance rather than TV static.
    """
    img = imgs[0]
    max_noise = float(params.get("max_noise", 0.85))
    grain = int(params.get("grain", 4))
    seed = int(params.get("seed", 0))
    h, w = img.shape[:2]
    field = _value_noise(h, w, grain, seed)
    palette = img.reshape(-1, 3).mean(axis=0)
    noise_rgb = field[..., None] * palette[None, None, :] * 1.5
    alpha = max_noise * 0.5 * (1.0 - np.cos(2.0 * np.pi * t))
    return to_uint8((1.0 - alpha) * img + alpha * noise_rgb)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k noise -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): noise-dissolve effect (palette-tinted value noise)"
```

---

## Task 7: `crossfade-drift` effect (two inputs)

**Files:**
- Modify: `tools/animate.py`
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
def test_crossfade_drift_shape_and_loop():
    a = _synth()
    b = _synth() * 0.5  # a visibly different second frame
    frame = animate.crossfade_drift([a, b], {}, 0.5)
    assert frame.shape == a.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.crossfade_drift, [a, b]) <= 1


def test_crossfade_midpoint_is_mostly_b():
    a = np.zeros((64, 48, 3), dtype=np.float32)
    b = np.ones((64, 48, 3), dtype=np.float32)
    mid = animate.crossfade_drift([a, b], {"drift_px": 0.0}, 0.5)
    # at t=0.5 alpha=1 -> should be image b (white)
    assert mid.min() >= 254
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k crossfade -v`
Expected: FAIL — `AttributeError: ... 'crossfade_drift'`.

- [ ] **Step 3: Write minimal implementation**

Add to `tools/animate.py`:
```python
def crossfade_drift(imgs, params, t):
    """Off-axis parallax crossfade between two keyframes: A -> B -> A.

    The two images pan in opposite directions by drift_px*sin(2*pi*t) while
    alpha = 0.5*(1-cos(2*pi*t)) blends A->B->A. Both vanish at t=0 and t=1.
    """
    a, b = imgs[0], imgs[1]
    drift_px = float(params.get("drift_px", 10.0))
    angle = np.deg2rad(float(params.get("angle_deg", 8.0)))
    h, w = a.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    ox, oy = np.cos(angle), np.sin(angle)
    s = drift_px * np.sin(2.0 * np.pi * t)
    a_s = remap(a, xs + s * ox, ys + s * oy)
    b_s = remap(b, xs - s * ox, ys - s * oy)
    alpha = 0.5 * (1.0 - np.cos(2.0 * np.pi * t))
    return to_uint8((1.0 - alpha) * a_s + alpha * b_s)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k crossfade -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): crossfade-drift two-keyframe parallax effect"
```

---

## Task 8: Effect registry, `render`, and `encode_mp4`

**Files:**
- Modify: `tools/animate.py`
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
import imageio_ffmpeg  # noqa: E402


def _count_mp4_frames(path):
    reader = imageio_ffmpeg.read_frames(path)
    next(reader)  # first item is the meta dict
    return sum(1 for _ in reader)


def test_render_writes_mp4_with_expected_frame_count(tmp_path):
    src = tmp_path / "src.png"
    Image.fromarray(animate.to_uint8(_synth(66, 50))).save(src)  # odd dims
    out = tmp_path / "clip.mp4"
    written, n = animate.render(
        [str(src)], "fold-churn", {}, duration=1.0, fps=12, out=str(out)
    )
    assert os.path.exists(written)
    assert os.path.getsize(written) > 0
    assert n == 12
    assert _count_mp4_frames(str(out)) == 12


def test_render_rejects_unknown_effect(tmp_path):
    src = tmp_path / "src.png"
    Image.fromarray(animate.to_uint8(_synth())).save(src)
    try:
        animate.render([str(src)], "nope", {}, 1.0, 12, str(tmp_path / "x.mp4"))
        assert False, "expected ValueError"
    except ValueError as e:
        assert "nope" in str(e)


def test_render_rejects_wrong_input_count(tmp_path):
    src = tmp_path / "src.png"
    Image.fromarray(animate.to_uint8(_synth())).save(src)
    try:
        animate.render(
            [str(src)], "crossfade-drift", {}, 1.0, 12, str(tmp_path / "x.mp4")
        )
        assert False, "expected ValueError"
    except ValueError as e:
        assert "2" in str(e)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k render -v`
Expected: FAIL — `AttributeError: module 'animate' has no attribute 'render'`.

- [ ] **Step 3: Write minimal implementation**

Add to `tools/animate.py` (after the effect functions):
```python
# Effect registry: name -> (required_input_count, frame_function).
EFFECTS = {
    "fold-churn": (1, fold_churn),
    "chromatic-split": (1, chromatic_split),
    "noise-dissolve": (1, noise_dissolve),
    "crossfade-drift": (2, crossfade_drift),
}


def _resize_to(img, h, w):
    if img.shape[:2] == (h, w):
        return img
    pil = Image.fromarray(to_uint8(img)).resize((w, h), Image.BILINEAR)
    return np.asarray(pil, dtype=np.float32) / 255.0


def encode_mp4(frames, out, fps, size):
    """Encode an iterable of (H,W,3) uint8 frames to a looping H.264 MP4."""
    import imageio_ffmpeg

    w, h = size
    writer = imageio_ffmpeg.write_frames(
        out,
        (w, h),
        pix_fmt_in="rgb24",
        pix_fmt_out="yuv420p",
        fps=fps,
        macro_block_size=1,
        output_params=["-movflags", "+faststart"],
    )
    writer.send(None)  # seed the generator
    for fr in frames:
        writer.send(np.ascontiguousarray(fr, dtype=np.uint8).tobytes())
    writer.close()


def render(srcs, effect, params, duration, fps, out):
    """Render `effect` over `srcs` to a looping MP4 at `out`.

    Returns (out_path, frame_count). Raises ValueError on unknown effect or
    wrong number of source images.
    """
    if effect not in EFFECTS:
        valid = ", ".join(sorted(EFFECTS))
        raise ValueError(f"unknown effect '{effect}'; valid: {valid}")
    n_inputs, fn = EFFECTS[effect]
    if len(srcs) != n_inputs:
        raise ValueError(
            f"effect '{effect}' needs {n_inputs} source image(s), got {len(srcs)}"
        )
    imgs = [ensure_even(load_image(s)) for s in srcs]
    h, w = imgs[0].shape[:2]
    imgs = [_resize_to(img, h, w) for img in imgs]
    n = max(1, round(duration * fps))
    frames = (fn(imgs, params, i / n) for i in range(n))
    encode_mp4(frames, out, fps, (w, h))
    return out, n
```

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k render -v`
Expected: PASS (all three render tests).

- [ ] **Step 5: Run the full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): effect registry, render, and mp4 encoder"
```

---

## Task 9: CLI (single-clip) + `run_batch`

**Files:**
- Modify: `tools/animate.py`
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
import json  # noqa: E402


def test_parse_params_typed():
    params = animate.parse_params(["amp_px=12", "freq=2.5", "seed=3"])
    assert params == {"amp_px": 12, "freq": 2.5, "seed": 3}


def test_run_batch_renders_all_and_skips_bad(tmp_path, capsys):
    good = tmp_path / "k.png"
    Image.fromarray(animate.to_uint8(_synth())).save(good)
    spec = {
        "groups": [
            {
                "title": "t",
                "items": [
                    {
                        "file": "ok.mp4",
                        "src": ["k.png"],
                        "effect": "fold-churn",
                        "params": {},
                        "duration": 1.0,
                        "fps": 8,
                    },
                    {
                        "file": "bad.mp4",
                        "src": ["missing.png"],
                        "effect": "fold-churn",
                        "duration": 1.0,
                        "fps": 8,
                    },
                ],
            }
        ]
    }
    spec_path = tmp_path / "anims.json"
    spec_path.write_text(json.dumps(spec))
    ok, fail = animate.run_batch(str(spec_path))
    assert ok == 1 and fail == 1
    assert (tmp_path / "ok.mp4").exists()
    assert not (tmp_path / "bad.mp4").exists()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "parse_params or run_batch" -v`
Expected: FAIL — `AttributeError: ... 'parse_params'`.

- [ ] **Step 3: Write minimal implementation**

Add to `tools/animate.py`:
```python
import argparse
import json
import sys
from pathlib import Path


def parse_params(pairs):
    """Parse ['name=value', ...] into a dict, typing values via JSON.

    'amp_px=12' -> {'amp_px': 12}; 'mode=soft' -> {'mode': 'soft'}.
    """
    params = {}
    for pair in pairs:
        if "=" not in pair:
            raise ValueError(f"bad --param '{pair}', expected NAME=VALUE")
        name, _, raw = pair.partition("=")
        try:
            value = json.loads(raw)
        except json.JSONDecodeError:
            value = raw
        params[name.strip()] = value
    return params


def run_batch(spec_path):
    """Render every clip in an anims.json file. Returns (ok_count, fail_count).

    Source and output paths resolve relative to the spec file's directory. A
    failing item is logged and skipped; the batch continues.
    """
    base = Path(spec_path).parent
    data = json.loads(Path(spec_path).read_text())
    ok = fail = 0
    for group in data["groups"]:
        for item in group["items"]:
            out = str(base / item["file"])
            srcs = [str(base / s) for s in item["src"]]
            try:
                render(
                    srcs,
                    item["effect"],
                    item.get("params", {}),
                    item.get("duration", 4.0),
                    item.get("fps", 24),
                    out,
                )
                ok += 1
                print(f"wrote {out}")
            except Exception as e:  # noqa: BLE001 — batch should not abort
                fail += 1
                print(f"SKIP {item.get('file')}: {e}", file=sys.stderr)
    print(f"batch: {ok} ok, {fail} skipped")
    return ok, fail


def main(argv=None):
    p = argparse.ArgumentParser(
        description="Animate Voidborn keyframes into looping MP4 clips."
    )
    p.add_argument("srcs", nargs="*", help="1-2 source keyframe image paths")
    p.add_argument("--effect", choices=sorted(EFFECTS), help="effect to apply")
    p.add_argument("--duration", type=float, default=4.0, help="seconds")
    p.add_argument("--fps", type=int, default=24)
    p.add_argument("-o", "--out", help="output .mp4 path")
    p.add_argument(
        "-p", "--param", action="append", default=[],
        metavar="NAME=VALUE", help="effect parameter (repeatable)",
    )
    p.add_argument("--batch", help="render all clips in an anims.json file")
    args = p.parse_args(argv)

    if args.batch:
        ok, fail = run_batch(args.batch)
        return 1 if fail and not ok else 0

    if not args.srcs or not args.effect or not args.out:
        p.error("single-clip mode needs SRC(s), --effect, and --out")
    params = parse_params(args.param)
    out, n = render(args.srcs, args.effect, params, args.duration, args.fps, args.out)
    print(f"wrote {out} ({n} frames)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

Move the `import argparse/json/sys` and `from pathlib import Path` lines to the top of the file with the other imports if you prefer; functionally either placement works.

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "parse_params or run_batch" -v`
Expected: PASS (both).

- [ ] **Step 5: Run the full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): animate CLI + anims.json batch runner"
```

---

## Task 10: `tools/animate` shell wrapper

**Files:**
- Create: `tools/animate`

- [ ] **Step 1: Write the wrapper**

Create `tools/animate`:
```bash
#!/usr/bin/env bash
#
# animate — procedurally animate a Voidborn keyframe into a looping MP4.
#
# A thin convenience wrapper around tools/animate.py, run via ~/sd-venv so you
# don't have to activate the venv. Pure CPU (numpy/Pillow + imageio-ffmpeg);
# no model, no GPU. See
# docs/superpowers/specs/2026-06-13-voidborn-procedural-animation-design.md.
#
#   ./tools/animate voidborn-concepts/h3_phase_fold.png \
#       --effect fold-churn --duration 4 -o voidborn-concepts/anim_fold.mp4
#   ./tools/animate a.png b.png --effect crossfade-drift -o out.mp4
#   ./tools/animate --param amp_px=18 --param freq=2.5 ... (repeatable)
#   ./tools/animate --batch voidborn-concepts/anims.json
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
gen="$here/animate.py"
py="${SD_VENV:-$HOME/sd-venv}/bin/python"

[[ -x "$py" ]]  || { echo "animate: python not found at $py (set \$SD_VENV)" >&2; exit 1; }
[[ -f "$gen" ]] || { echo "animate: animate.py missing at $gen" >&2; exit 1; }

exec "$py" "$gen" "$@"
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x tools/animate`

- [ ] **Step 3: Verify it renders a real keyframe**

Run:
```bash
./tools/animate voidborn-concepts/h3_phase_fold.png \
    --effect fold-churn --duration 2 --fps 12 -o /tmp/_animtest.mp4
```
Expected: prints `wrote /tmp/_animtest.mp4 (24 frames)`; file exists and is nonzero (`ls -l /tmp/_animtest.mp4`).

- [ ] **Step 4: Commit**

```bash
git add tools/animate
git commit -m "feat(kb): tools/animate shell wrapper"
```

---

## Task 11: Author the initial `anims.json` and render the batch

**Files:**
- Create: `voidborn-concepts/anims.json`

> **Note on gitignore:** `voidborn-concepts/` is gitignored (PNG/MP4 review artifacts). `anims.json` is a small recipe file worth tracking, like the build script. After creating it, force-add with `git add -f` (Step 4). The `.mp4` outputs stay ignored.

- [ ] **Step 1: Confirm the keyframes exist**

Run:
```bash
ls voidborn-concepts/{h3_phase_fold,h3_v_fold2,h3_v_fold3,abstract_fold_a,h3_v_double2,fusion_amber_double,fusion_sapphire_double,h3_phase_dissolve,abstract_phase_c,fusion_amber_fold,fusion_sapphire_fold}.png
```
Expected: all 11 files listed, no "No such file" errors. (If any are missing, drop the corresponding item from the spec below.)

- [ ] **Step 2: Write `voidborn-concepts/anims.json`**

Create `voidborn-concepts/anims.json`:
```json
{
  "groups": [
    {
      "id": "anim_fold",
      "title": "Animations · fold churn",
      "subtitle": "non-euclidean folds breathing (displacement field)",
      "items": [
        {"file": "anim_fold_phase.mp4", "src": ["h3_phase_fold.png"],
         "effect": "fold-churn", "params": {"amp_px": 12, "freq": 2.0},
         "duration": 4, "fps": 24, "label": "H3 phase fold churn", "fav": false},
        {"file": "anim_fold_v2.mp4", "src": ["h3_v_fold2.png"],
         "effect": "fold-churn", "params": {"amp_px": 14, "freq": 2.5},
         "duration": 4, "fps": 24, "label": "H3 fold v2 churn", "fav": false},
        {"file": "anim_fold_v3.mp4", "src": ["h3_v_fold3.png"],
         "effect": "fold-churn", "params": {"amp_px": 14, "freq": 2.5},
         "duration": 4, "fps": 24, "label": "H3 fold v3 churn", "fav": false},
        {"file": "anim_fold_abstract.mp4", "src": ["abstract_fold_a.png"],
         "effect": "fold-churn", "params": {"amp_px": 10, "freq": 3.0},
         "duration": 4, "fps": 24, "label": "Abstract fold tunnel", "fav": false}
      ]
    },
    {
      "id": "anim_chroma",
      "title": "Animations · chromatic split",
      "subtitle": "reference frames unify then chromatically split + drift",
      "items": [
        {"file": "anim_chroma_double2.mp4", "src": ["h3_v_double2.png"],
         "effect": "chromatic-split", "params": {"max_px": 18, "angle_deg": 20, "drift_px": 6},
         "duration": 4, "fps": 24, "label": "H3 double — split", "fav": false},
        {"file": "anim_chroma_amber.mp4", "src": ["fusion_amber_double.png"],
         "effect": "chromatic-split", "params": {"max_px": 16, "angle_deg": 15, "drift_px": 5},
         "duration": 4, "fps": 24, "label": "Fusion amber — split", "fav": false},
        {"file": "anim_chroma_sapphire.mp4", "src": ["fusion_sapphire_double.png"],
         "effect": "chromatic-split", "params": {"max_px": 16, "angle_deg": 15, "drift_px": 5},
         "duration": 4, "fps": 24, "label": "Fusion sapphire — split", "fav": false}
      ]
    },
    {
      "id": "anim_dissolve",
      "title": "Animations · noise dissolve",
      "subtitle": "dissolve into palette-tinted probability noise and back",
      "items": [
        {"file": "anim_dissolve_phase.mp4", "src": ["h3_phase_dissolve.png"],
         "effect": "noise-dissolve", "params": {"max_noise": 0.85, "grain": 4, "seed": 1},
         "duration": 5, "fps": 24, "label": "H3 phase dissolve", "fav": false},
        {"file": "anim_dissolve_abstract.mp4", "src": ["abstract_phase_c.png"],
         "effect": "noise-dissolve", "params": {"max_noise": 0.8, "grain": 5, "seed": 2},
         "duration": 5, "fps": 24, "label": "Abstract phase dissolve", "fav": false}
      ]
    },
    {
      "id": "anim_crossfade",
      "title": "Animations · crossfade drift",
      "subtitle": "off-axis parallax crossfade between two keyframes",
      "items": [
        {"file": "anim_crossfade_fusion.mp4",
         "src": ["fusion_amber_fold.png", "fusion_sapphire_fold.png"],
         "effect": "crossfade-drift", "params": {"drift_px": 10, "angle_deg": 8},
         "duration": 5, "fps": 24, "label": "Fusion amber ↔ sapphire", "fav": false}
      ]
    }
  ]
}
```

- [ ] **Step 3: Render the whole batch**

Run: `./tools/animate --batch voidborn-concepts/anims.json`
Expected: `wrote ...` for each of the 10 clips, ending `batch: 10 ok, 0 skipped`. Confirm with `ls -l voidborn-concepts/anim_*.mp4` (10 nonzero files).

- [ ] **Step 4: Commit the recipe (force-add past gitignore)**

```bash
git add -f voidborn-concepts/anims.json
git commit -m "feat(kb): initial Voidborn animation batch recipes"
```

---

## Task 12: Gallery integration — render `<video>` tiles

**Files:**
- Modify: `tools/build_voidborn_gallery.py`

- [ ] **Step 1: Add video CSS**

In `tools/build_voidborn_gallery.py`, find the `CSS` string and the line:
```python
  figure img { width:100%; display:block; }
```
Change it to:
```python
  figure img, figure video { width:100%; display:block; }
```

- [ ] **Step 2: Add the `ANIMS` path constant**

Find:
```python
DATA = CONCEPTS / "prompts.json"
OUT = CONCEPTS / "index.html"
```
Add a line after it:
```python
ANIMS = CONCEPTS / "anims.json"
```

- [ ] **Step 3: Add a `video_figure_html` function**

Immediately after the existing `figure_html` function, add:
```python
def video_figure_html(item: dict) -> str:
    fav = " fav" if item.get("fav") else ""
    f = html.escape(item["file"])
    label = html.escape(item["label"])
    effect = html.escape(item.get("effect", ""))
    params = html.escape(json.dumps(item.get("params", {})))
    star = " ★" if item.get("fav") else ""
    return (
        f'    <figure class="card{fav}">'
        f'<video src="{f}" autoplay loop muted playsinline></video>'
        f'<figcaption><b>{label}{star}</b><br>'
        f'<span class="seed">{effect}</span>'
        f'<details><summary>params</summary><p>{params}</p></details>'
        f'</figcaption></figure>'
    )
```

- [ ] **Step 4: Render the anim groups in `main`**

In `main()`, find this block:
```python
    data = json.loads(DATA.read_text())
    n = sum(len(g["items"]) for g in data["groups"])
```
Replace it with:
```python
    data = json.loads(DATA.read_text())
    anim_data = json.loads(ANIMS.read_text()) if ANIMS.exists() else {"groups": []}
    n = sum(len(g["items"]) for g in data["groups"])
    n_clips = sum(len(g["items"]) for g in anim_data["groups"])
```

Then find the sub-line that builds the header description:
```python
        f'<div class="sub">FLUX.1-schnell, 1024px · {n} renders · '
        "blue-outlined ★ = favorites · click an image for full size, "
        '"prompt" for the exact recipe.</div>',
```
Replace it with:
```python
        f'<div class="sub">FLUX.1-schnell, 1024px · {n} renders · '
        f"{n_clips} animations · blue-outlined ★ = favorites · click an image "
        'for full size, "prompt"/"params" for the exact recipe.</div>',
```

Then find the groups loop:
```python
    for g in data["groups"]:
        sub = f' <small>— {html.escape(g["subtitle"])}</small>' if g.get("subtitle") else ""
        parts.append(f'<h2>{html.escape(g["title"])}{sub}</h2>')
        parts.append('<div class="grid">')
        parts.extend(figure_html(it) for it in g["items"])
        parts.append("</div>")
    parts.append("</body></html>")
```
Replace it with:
```python
    for g in data["groups"]:
        sub = f' <small>— {html.escape(g["subtitle"])}</small>' if g.get("subtitle") else ""
        parts.append(f'<h2>{html.escape(g["title"])}{sub}</h2>')
        parts.append('<div class="grid">')
        parts.extend(figure_html(it) for it in g["items"])
        parts.append("</div>")
    for g in anim_data["groups"]:
        sub = f' <small>— {html.escape(g["subtitle"])}</small>' if g.get("subtitle") else ""
        parts.append(f'<h2>{html.escape(g["title"])}{sub}</h2>')
        parts.append('<div class="grid">')
        parts.extend(video_figure_html(it) for it in g["items"])
        parts.append("</div>")
    parts.append("</body></html>")
```

Then find the final print line:
```python
    print(f"wrote {OUT} ({n} images across {len(data['groups'])} groups)")
```
Replace it with:
```python
    print(
        f"wrote {OUT} ({n} images, {n_clips} clips across "
        f"{len(data['groups']) + len(anim_data['groups'])} groups)"
    )
```

- [ ] **Step 5: Rebuild the gallery and verify**

Run: `python3 tools/build_voidborn_gallery.py`
Expected: prints `wrote .../index.html (72 images, 10 clips across N groups)` (image count may differ if the stills set changed).

Run: `grep -c "<video" voidborn-concepts/index.html`
Expected: `10` (one per clip).

- [ ] **Step 6: Open and eyeball (optional, manual)**

Run: `xdg-open voidborn-concepts/index.html` and confirm the four animation sections autoplay-loop and the fold/dissolve/split effects read correctly. (Per the spec, harsher static for `noise-dissolve` is a possible later tweak — `grain`/`max_noise` in `anims.json`.)

- [ ] **Step 7: Commit**

```bash
git add tools/build_voidborn_gallery.py
git commit -m "feat(kb): gallery renders animation <video> tiles from anims.json"
```

---

## Verification (whole pipeline)

- [ ] `~/sd-venv/bin/python -m pytest tools/test_animate.py -v` — all tests pass.
- [ ] `./tools/animate --batch voidborn-concepts/anims.json` — `10 ok, 0 skipped`.
- [ ] `python3 tools/build_voidborn_gallery.py` — reports 10 clips; `grep -c "<video" voidborn-concepts/index.html` == 10.
- [ ] `git status` — tracked changes are only: `tools/animate.py`, `tools/test_animate.py`, `tools/animate`, `tools/build_voidborn_gallery.py`, `voidborn-concepts/anims.json`. The `.mp4`/`index.html` artifacts remain gitignored.

---

## Notes for the implementer

- **Loop invariant:** every effect is a function `fn(imgs, params, t)`. The renderer emits frames at `t = i/n` for `i in 0..n-1`; because each effect's time terms use `sin/cos(2*pi*t)` (or integer `cycles`), the frame at `t=1` equals `t=0`, so the MP4 loops with no seam. Tests assert this with a ≤1 uint8 tolerance (floating-point `sin(2*pi) ≈ 0`).
- **No GPU/model** — this is pure CPU. Renders are short; no warm daemon needed (unlike `tools/portrait`).
- **Harsher static** is a deferred tweak: bump `max_noise` toward 1.0 and/or lower `grain` in `anims.json`, or swap `_value_noise` for per-pixel white noise. Don't build it now.
- **imageio-ffmpeg `write_frames`** takes `size=(width, height)`; we pad to even dims ourselves and pass `macro_block_size=1` so it never silently resizes.
