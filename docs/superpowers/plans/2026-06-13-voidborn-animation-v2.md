# Voidborn Animation Effects v2 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three new procedural animation effects to `tools/animate.py` — `hyper-warp` (4D-rotation displacement, optional subject protection), `hyperspace-streak` (Stargate "probability field"), `disintegrate` (body dust) — plus a reusable `subject_mask` helper, then rewire `anims.json` to use them.

**Architecture:** Extends the existing `tools/animate.py`. New effects obey the v1 `fn(imgs, params, t) -> uint8 (H,W,3)` contract and register in `EFFECTS`. Because `subject_mask` (connected-components) and the streak's painted layer are t-independent but expensive and effects run per-frame, both are memoized in module-level caches cleared at the start of each `render()`. Spec: `docs/superpowers/specs/2026-06-13-voidborn-animation-v2-design.md`.

**Tech Stack:** Python 3, numpy, Pillow (incl. `ImageFilter`), imageio-ffmpeg, pytest. Runs under `~/sd-venv`. **No scipy** (not installed) — connected components is pure numpy. Run tests: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`.

---

## File Structure

- **Modify** `tools/animate.py`:
  - Add imports: `import os`; change `from PIL import Image` → `from PIL import Image, ImageFilter`.
  - Add module caches `_MASK_CACHE = {}` and `_STREAK_CACHE = {}`.
  - Add helpers `_largest_component`, `subject_mask`, `_flow_to_mask`, `_draw_streaking_stars`.
  - Add effects `hyper_warp`, `hyperspace_streak`, `disintegrate`; register all three in `EFFECTS`.
  - Modify `render` to clear caches and inject `mask_path` from `<src>.mask.png`.
- **Modify** `tools/test_animate.py`: add tests for each new unit.
- **Modify** `voidborn-concepts/anims.json`: rewire fold/dissolve clips to the new effects; add `anim_fold_abstract_b.mp4`.

**Insertion convention:** new helper/effect functions go AFTER `crossfade_drift` and BEFORE the `# Effect registry: name -> ...` comment line, in the order the tasks introduce them (so each is defined before `EFFECTS` references it).

---

## Task 1: Imports, caches, `_largest_component`, `subject_mask`

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def _blob(h=80, w=80):
    """A bright disk on black — a stand-in 'subject' for mask tests."""
    img = np.zeros((h, w, 3), dtype=np.float32)
    yy, xx = np.mgrid[0:h, 0:w]
    cy, cx = h / 2.0, w / 2.0
    disk = (yy - cy) ** 2 + (xx - cx) ** 2 < (min(h, w) * 0.25) ** 2
    img[disk] = 0.9
    return img


def test_subject_mask_high_inside_low_outside():
    m = animate.subject_mask(_blob(), {})
    assert m.dtype == np.float32
    assert m.shape == (80, 80)
    assert m[40, 40] > 0.8          # center of the blob
    assert m[2, 2] < 0.2            # corner (background)


def test_subject_mask_empty_on_black():
    m = animate.subject_mask(np.zeros((40, 40, 3), dtype=np.float32), {})
    assert float(m.max()) == 0.0    # no bright blob -> all-zero mask


def test_subject_mask_uses_override_path(tmp_path):
    # a half-white grayscale override should win over auto-detection
    mp = tmp_path / "m.png"
    arr = np.zeros((40, 40), dtype=np.uint8)
    arr[:, :20] = 255
    Image.fromarray(arr).save(mp)
    m = animate.subject_mask(_blob(40, 40), {"mask_path": str(mp)})
    assert m[20, 5] > 0.9 and m[20, 35] < 0.1


def test_largest_component_picks_biggest_blob():
    a = np.zeros((20, 20), dtype=bool)
    a[2:5, 2:5] = True          # small blob (9 px)
    a[10:18, 10:18] = True      # big blob (64 px)
    big = animate._largest_component(a)
    assert big[14, 14] and not big[3, 3]
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "subject_mask or largest_component" -v`
Expected: FAIL — `AttributeError: module 'animate' has no attribute 'subject_mask'`.

- [ ] **Step 3: Edit imports and add caches + helpers**

In `tools/animate.py`, change the top imports. Find:
```python
import numpy as np
from PIL import Image
```
Replace with:
```python
import os

import numpy as np
from PIL import Image, ImageFilter
```
(Keep the existing `import argparse`, `import json`, `import sys`, `from pathlib import Path` lines as they are; add `import os` among the stdlib imports.)

Then insert AFTER `crossfade_drift` and BEFORE the `# Effect registry` comment:
```python
# t-independent results memoized per render (cleared at the start of render()).
_MASK_CACHE = {}
_STREAK_CACHE = {}


def _largest_component(mask_bool):
    """Return the largest 4-connected True component of a boolean array.

    Pure-numpy label propagation (no scipy): each True pixel starts with a
    unique id; ids spread the minimum to 4-neighbors until stable; the most
    frequent surviving id is the largest component.
    """
    h, w = mask_bool.shape
    labels = np.where(mask_bool, np.arange(h * w).reshape(h, w), -1)
    while True:
        prev = labels
        cur = labels.copy()
        cur[1:, :] = np.where(
            mask_bool[1:, :] & mask_bool[:-1, :],
            np.minimum(cur[1:, :], labels[:-1, :]), cur[1:, :])
        cur[:-1, :] = np.where(
            mask_bool[:-1, :] & mask_bool[1:, :],
            np.minimum(cur[:-1, :], labels[1:, :]), cur[:-1, :])
        cur[:, 1:] = np.where(
            mask_bool[:, 1:] & mask_bool[:, :-1],
            np.minimum(cur[:, 1:], labels[:, :-1]), cur[:, 1:])
        cur[:, :-1] = np.where(
            mask_bool[:, :-1] & mask_bool[:, 1:],
            np.minimum(cur[:, :-1], labels[:, 1:]), cur[:, :-1])
        labels = cur
        if np.array_equal(labels, prev):
            break
    vals = labels[mask_bool]
    if vals.size == 0:
        return np.zeros((h, w), dtype=bool)
    uniq, counts = np.unique(vals, return_counts=True)
    return labels == uniq[int(np.argmax(counts))]


def subject_mask(img, params):
    """Soft [0,1] mask separating a bright figure from a dark background.

    Override: if params['mask_path'] points to an existing image, use it
    (grayscale, resized). Auto: luminance threshold -> largest connected
    component (computed at reduced resolution for speed) -> morphological
    close -> Gaussian feather. Memoized in _MASK_CACHE (t-independent).
    """
    mask_path = params.get("mask_path")
    thr = float(params.get("mask_threshold", 0.35))
    feather = float(params.get("mask_feather", 6.0))
    h, w = img.shape[:2]
    key = (id(img), mask_path, round(thr, 4), round(feather, 4))
    cached = _MASK_CACHE.get(key)
    if cached is not None:
        return cached

    if mask_path and os.path.exists(mask_path):
        pil = Image.open(mask_path).convert("L").resize((w, h), Image.BILINEAR)
        m = np.asarray(pil, dtype=np.float32) / 255.0
        _MASK_CACHE[key] = m
        return m

    lum = img @ np.array([0.299, 0.587, 0.114], dtype=np.float32)
    binar = lum > thr
    # largest connected component at reduced res, then upsample + AND
    scale = max(1, int(max(h, w) / 256))
    comp_small = _largest_component(binar[::scale, ::scale])
    comp = np.asarray(
        Image.fromarray((comp_small.astype(np.uint8) * 255)).resize(
            (w, h), Image.NEAREST),
        dtype=np.uint8) > 0
    keep = (binar & comp).astype(np.uint8) * 255
    pil = Image.fromarray(keep)
    r = max(1, int(round(feather / 2.0)))
    k = 2 * r + 1
    pil = pil.filter(ImageFilter.MaxFilter(k)).filter(ImageFilter.MinFilter(k))
    pil = pil.filter(ImageFilter.GaussianBlur(feather))
    m = np.asarray(pil, dtype=np.float32) / 255.0
    _MASK_CACHE[key] = m
    return m
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "subject_mask or largest_component" -v`
Expected: PASS (all four).

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass (15 prior + 4 new = 19).

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): subject_mask + pure-numpy largest-component helper"
```

---

## Task 2: `hyper-warp` effect (4D-rotation displacement)

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def test_hyper_warp_shape_and_loop():
    img = _synth()
    frame = animate.hyper_warp([img], {}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.hyper_warp, [img]) <= 1


def test_hyper_warp_identity_at_t0():
    img = _synth()
    f0 = animate.hyper_warp([img], {"amp": 0.4}, 0.0)
    assert np.abs(f0.astype(np.int16) - animate.to_uint8(img).astype(np.int16)).max() <= 1


def test_hyper_warp_protect_keeps_subject_closer():
    img = _blob()
    protected = animate.hyper_warp([img], {"protect_subject": True, "amp": 0.6}, 0.5)
    warped = animate.hyper_warp([img], {"protect_subject": False, "amp": 0.6}, 0.5)
    src = animate.to_uint8(img)
    # inside the blob, the protected frame is closer to the original than the
    # warp-everything frame
    cy, cx = 40, 40
    d_prot = abs(int(protected[cy, cx, 0]) - int(src[cy, cx, 0]))
    d_warp = abs(int(warped[cy, cx, 0]) - int(src[cy, cx, 0]))
    assert d_prot <= d_warp
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k hyper_warp -v`
Expected: FAIL — `AttributeError: ... 'hyper_warp'`.

- [ ] **Step 3: Add the effect and register it**

Insert AFTER `subject_mask` (before the `# Effect registry` comment):
```python
def hyper_warp(imgs, params, t):
    """Displace the keyframe by a real 4D rotation projected back to 2D.

    Each pixel (u,v) in [-1,1] is embedded in 4D with radius-seeded z,w coords,
    rotated through the xw/yw/zw planes (the rotations with no 3D analog) by
    theta = 2*pi*t*turns, and perspective-projected back. Displacement is taken
    RELATIVE to the rest (theta=0) projection, so t=0 and t=1 (integer turns)
    are the undistorted keyframe -> seamless loop with crisp endpoints.
    With protect_subject, the feathered subject is composited back unwarped.
    """
    img = imgs[0]
    amp = float(params.get("amp", 0.35))
    turns = int(params.get("turns", 1))
    w_dist = float(params.get("w_dist", 2.5))
    protect = bool(params.get("protect_subject", False))
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    u = 2.0 * xs / (w - 1) - 1.0
    v = 2.0 * ys / (h - 1) - 1.0
    r = np.sqrt(u * u + v * v)
    z = r * np.cos(np.pi * r)   # hidden dims carry radial structure
    wc = r * np.sin(np.pi * r)

    def project(theta):
        c, s = np.cos(theta), np.sin(theta)
        px, py, pz, pw = u.copy(), v.copy(), z.copy(), wc.copy()
        px, pw = px * c - pw * s, px * s + pw * c   # xw plane
        py, pw = py * c - pw * s, py * s + pw * c   # yw plane
        pz, pw = pz * c - pw * s, pz * s + pw * c   # zw plane
        denom = w_dist - pw
        denom = np.where(np.abs(denom) < 1e-3, 1e-3, denom)
        proj = w_dist / denom
        return px * proj, py * proj

    u_now, v_now = project(2.0 * np.pi * t * turns)
    u_rest, v_rest = project(0.0)
    scale = min(h, w) / 2.0
    dx = amp * (u_now - u_rest) * scale
    dy = amp * (v_now - v_rest) * scale
    warped = remap(img, xs + dx, ys + dy)
    if protect:
        m = subject_mask(img, params)[..., None]
        return to_uint8(warped * (1.0 - m) + img * m)
    return to_uint8(warped)
```

Add to the `EFFECTS` dict (before its closing `}`):
```python
    "hyper-warp": (1, hyper_warp),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k hyper_warp -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): hyper-warp 4D-rotation displacement effect"
```

---

## Task 3: `hyperspace-streak` effect (Stargate probability field)

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def _point_with_curve(h=80, w=80):
    """A single bright star plus a bright vertical 'curve' down the middle."""
    img = np.zeros((h, w, 3), dtype=np.float32)
    img[:, w // 2 - 1:w // 2 + 1] = 0.9   # the fixed curve
    img[20, 20] = 1.0                      # a lone star to streak
    return img


def test_hyperspace_streak_shape_and_loop():
    img = _point_with_curve()
    frame = animate.hyperspace_streak([img], {"seed": 1}, 0.4)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.hyperspace_streak, [img], {"seed": 1}) <= 1


def test_hyperspace_streak_elongates_a_point():
    img = _point_with_curve()
    frame = animate.hyperspace_streak([img], {"seed": 1, "streak_len": 20}, 0.4)
    # the lone star should smear into more than one bright pixel along the flow
    bright = (frame.max(axis=2) > 120)
    assert bright.sum() > (img.max(axis=2) > 0.5).sum()


def test_hyperspace_streak_keeps_curve():
    img = _point_with_curve()
    frame = animate.hyperspace_streak([img], {"seed": 1}, 0.4)
    # the fixed curve column stays bright
    assert frame[:, 40].max() > 180
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k hyperspace -v`
Expected: FAIL — `AttributeError: ... 'hyperspace_streak'`.

- [ ] **Step 3: Add helpers + effect and register**

Insert AFTER `hyper_warp` (before the `# Effect registry` comment):
```python
def _flow_to_mask(bright, toward):
    """Unit flow vectors pointing toward (or away from) the bright curve.

    Heavily blur the bright mask into a smooth scalar field that rises toward
    the curve; its gradient points uphill (toward the curve). Returns
    (flow_x, flow_y) each (H,W).
    """
    field = np.asarray(
        Image.fromarray((bright.astype(np.uint8) * 255)).filter(
            ImageFilter.GaussianBlur(40)),
        dtype=np.float32)
    gy, gx = np.gradient(field)
    mag = np.sqrt(gx * gx + gy * gy) + 1e-6
    fx, fy = gx / mag, gy / mag
    if not toward:
        fx, fy = -fx, -fy
    return fx, fy


def _draw_streaking_stars(h, w, flow_x, flow_y, n_stars, streak_len, seed, t):
    """Synthetic stars that stream along the flow, looping via (phase+t) mod 1.

    A per-star sine envelope makes brightness zero at the wrap, so the
    position discontinuity at the loop is invisible. Returns (H,W,3) float.
    """
    rng = np.random.default_rng(seed)
    base = rng.random(n_stars)
    px = rng.integers(0, w, n_stars)
    py = rng.integers(0, h, n_stars)
    bri = rng.uniform(0.5, 1.0, n_stars)
    tint = np.array([0.7, 0.85, 1.0], dtype=np.float32)   # cool Voidborn blue
    out = np.zeros((h, w, 3), dtype=np.float32)
    max_travel = streak_len * 2.0
    for i in range(n_stars):
        phase = (base[i] + t) % 1.0
        env = np.sin(np.pi * phase)        # 0 at wrap, 1 mid-travel
        if env <= 0:
            continue
        dist = phase * max_travel
        fx = float(flow_x[py[i], px[i]])
        fy = float(flow_y[py[i], px[i]])
        for k in range(streak_len):
            x = int(px[i] + fx * (dist - k))
            y = int(py[i] + fy * (dist - k))
            if 0 <= x < w and 0 <= y < h:
                out[y, x] += bri[i] * env * (1.0 - k / streak_len) * tint
    return np.clip(out, 0.0, 1.0)


def hyperspace_streak(imgs, params, t):
    """Stargate 'probability field': fixed bright curve, stars streaking past.

    The painted texture is motion-blurred along a flow toward/away from the
    curve (t-independent, cached); synthetic stars stream along the same flow
    and loop; the detected bright curve is composited back unmoved on top.
    """
    img = imgs[0]
    streak_len = int(params.get("streak_len", 24))
    n_stars = int(params.get("n_stars", 240))
    toward = bool(params.get("toward", True))
    curve_threshold = float(params.get("curve_threshold", 0.5))
    seed = int(params.get("seed", 0))
    h, w = img.shape[:2]

    key = (id(img), streak_len, round(curve_threshold, 4), toward)
    cached = _STREAK_CACHE.get(key)
    if cached is None:
        lum = img @ np.array([0.299, 0.587, 0.114], dtype=np.float32)
        bright = lum > curve_threshold
        fx, fy = _flow_to_mask(bright, toward)
        ys, xs = np.meshgrid(
            np.arange(h, dtype=np.float32),
            np.arange(w, dtype=np.float32),
            indexing="ij",
        )
        acc = np.zeros_like(img)
        wsum = 0.0
        for k in range(streak_len):
            wgt = 0.85 ** k
            acc += wgt * remap(img, xs + fx * k, ys + fy * k)
            wsum += wgt
        streaked = acc / wsum
        curve_rgb = img * bright[..., None]
        cached = (fx, fy, streaked, curve_rgb)
        _STREAK_CACHE[key] = cached
    fx, fy, streaked, curve_rgb = cached

    stars = _draw_streaking_stars(h, w, fx, fy, n_stars, streak_len, seed, t)
    field = np.maximum(streaked, stars)
    return to_uint8(np.maximum(field, curve_rgb))
```

Add to `EFFECTS`:
```python
    "hyperspace-streak": (1, hyperspace_streak),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k hyperspace -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): hyperspace-streak Stargate probability-field effect"
```

---

## Task 4: `disintegrate` effect (body dust)

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def test_disintegrate_shape_and_loop():
    img = _blob()
    frame = animate.disintegrate([img], {"seed": 2}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.disintegrate, [img], {"seed": 2}) <= 1


def test_disintegrate_midpoint_erodes_body():
    img = _blob()
    mid = animate.disintegrate([img], {"seed": 2}, 0.5)
    src = animate.to_uint8(img)
    # inside the body, the midpoint differs substantially from the intact source
    cy, cx = 40, 40
    assert abs(int(mid[cy, cx, 0]) - int(src[cy, cx, 0])) > 30


def test_disintegrate_leaves_background_untouched():
    img = _blob()
    src = animate.to_uint8(img)
    for tt in (0.0, 0.5, 1.0):
        frame = animate.disintegrate([img], {"seed": 2}, tt)
        # a far corner is background (mask ~0) and must never change
        assert np.array_equal(frame[2, 2], src[2, 2])
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k disintegrate -v`
Expected: FAIL — `AttributeError: ... 'disintegrate'`.

- [ ] **Step 3: Add the effect and register**

Insert AFTER `hyperspace_streak` (before the `# Effect registry` comment):
```python
def disintegrate(imgs, params, t):
    """Erode the masked body into wind-blown dust, then reform (ping-pong loop).

    A per-pixel value-noise threshold vs alpha(t)=0.5*(1-cos(2*pi*t)) drives a
    progressive dissolve front over the body only; dusted pixels are vacated to
    void and a wind-displaced, fading copy of the body drifts on top. All
    displacement scales with per-pixel progress, which is 0 at alpha=0, so t=0
    and t=1 are the intact keyframe. Background (outside the mask) is untouched.
    """
    img = imgs[0]
    wind_angle = float(params.get("wind_angle", 0.3))
    wind_px = float(params.get("wind_px", 60.0))
    turbulence = float(params.get("turbulence", 8.0))
    grain = int(params.get("grain", 3))
    seed = int(params.get("seed", 0))
    h, w = img.shape[:2]

    m = subject_mask(img, params)
    noise = _value_noise(h, w, grain, seed)
    alpha = 0.5 * (1.0 - np.cos(2.0 * np.pi * t))
    dust_sel = (m > 0.5) & (noise < alpha)
    prog = np.clip((alpha - noise) / max(alpha, 1e-3), 0.0, 1.0)

    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    wx, wy = np.cos(wind_angle), np.sin(wind_angle)
    turb = (noise - 0.5) * turbulence
    off_x = (wind_px * wx + turb) * prog
    off_y = (wind_px * wy + turb) * prog

    out = img.copy()
    out = out * np.where(dust_sel[..., None], 0.0, 1.0)        # vacate dust to void
    drift = remap(img * m[..., None], xs - off_x, ys - off_y)  # drifting dust copy
    out = np.maximum(out, drift * (1.0 - prog)[..., None])     # fades as it travels
    return to_uint8(out)
```

Add to `EFFECTS`:
```python
    "disintegrate": (1, disintegrate),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k disintegrate -v`
Expected: PASS (all three).

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass (19 + 3 + 3 + 3 = 28).

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): disintegrate body-dust effect"
```

---

## Task 5: `render` clears caches and injects `mask_path`

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
def test_render_injects_mask_path(tmp_path, monkeypatch):
    # write a keyframe and a sibling .mask.png; render() should pick up the mask
    src = tmp_path / "k.png"
    Image.fromarray(animate.to_uint8(_blob())).save(src)
    mask = tmp_path / "k.png.mask.png"
    Image.fromarray(np.full((80, 80), 200, dtype=np.uint8)).save(mask)

    seen = {}
    real = animate.subject_mask

    def spy(img, params):
        seen["mask_path"] = params.get("mask_path")
        return real(img, params)

    monkeypatch.setattr(animate, "subject_mask", spy)
    out = tmp_path / "o.mp4"
    animate.render([str(src)], "disintegrate", {}, 1.0, 6, str(out))
    assert seen["mask_path"] == str(src) + ".mask.png"
    assert out.exists()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k render_injects_mask_path -v`
Expected: FAIL — `seen["mask_path"]` is `None` (not yet injected), so the assert fails (KeyError or None mismatch).

- [ ] **Step 3: Modify `render`**

In `tools/animate.py`, find the body of `render`:
```python
    imgs = [ensure_even(load_image(s)) for s in srcs]
    h, w = imgs[0].shape[:2]
    imgs = [_resize_to(img, h, w) for img in imgs]
    n = max(1, round(duration * fps))
    frames = (fn(imgs, params, i / n) for i in range(n))
```
Replace with:
```python
    _MASK_CACHE.clear()
    _STREAK_CACHE.clear()
    imgs = [ensure_even(load_image(s)) for s in srcs]
    h, w = imgs[0].shape[:2]
    imgs = [_resize_to(img, h, w) for img in imgs]
    # auto-pick a sibling <src>.mask.png override for mask-using effects
    params = dict(params)
    if "mask_path" not in params:
        cand = srcs[0] + ".mask.png"
        if os.path.exists(cand):
            params["mask_path"] = cand
    n = max(1, round(duration * fps))
    frames = (fn(imgs, params, i / n) for i in range(n))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k render_injects_mask_path -v`
Expected: PASS.

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass (29).

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): render clears effect caches and injects .mask.png override"
```

---

## Task 6: Rewire `anims.json` to the new effects + render + rebuild gallery

**Files:** Modify `voidborn-concepts/anims.json`

- [ ] **Step 1: Confirm keyframes exist**

Run:
```bash
ls voidborn-concepts/{h3_phase_fold,h3_v_fold2,h3_v_fold3,abstract_fold_a,abstract_fold_b,abstract_phase_c,h3_phase_dissolve,h3_v_double2,fusion_amber_double,fusion_sapphire_double,fusion_amber_fold,fusion_sapphire_fold}.png
```
Expected: all listed. (If `abstract_fold_b.png` is missing, omit the `anim_fold_abstract_b` item in Step 2.)

- [ ] **Step 2: Rewrite `voidborn-concepts/anims.json`**

Replace the file with EXACTLY this content:
```json
{
  "groups": [
    {
      "id": "anim_fold",
      "title": "Animations · 4D fold warp",
      "subtitle": "geometry tumbling through a 4D rotation (xw/yw/zw planes); figures held solid",
      "items": [
        {"file": "anim_fold_phase.mp4", "src": ["h3_phase_fold.png"],
         "effect": "hyper-warp", "params": {"protect_subject": true, "amp": 0.4, "turns": 1},
         "duration": 5, "fps": 24, "label": "H3 phase fold — 4D (body solid)", "fav": false},
        {"file": "anim_fold_v2.mp4", "src": ["h3_v_fold2.png"],
         "effect": "hyper-warp", "params": {"protect_subject": true, "amp": 0.4, "turns": 1},
         "duration": 5, "fps": 24, "label": "H3 fold v2 — 4D (body solid)", "fav": false},
        {"file": "anim_fold_v3.mp4", "src": ["h3_v_fold3.png"],
         "effect": "hyper-warp", "params": {"protect_subject": true, "amp": 0.4, "turns": 1},
         "duration": 5, "fps": 24, "label": "H3 fold v3 — 4D (body solid)", "fav": false},
        {"file": "anim_fold_abstract.mp4", "src": ["abstract_fold_a.png"],
         "effect": "hyper-warp", "params": {"amp": 0.5, "turns": 1},
         "duration": 5, "fps": 24, "label": "Abstract fold tunnel — 4D", "fav": false},
        {"file": "anim_fold_abstract_b.mp4", "src": ["abstract_fold_b.png"],
         "effect": "hyper-warp", "params": {"amp": 0.5, "turns": 1},
         "duration": 5, "fps": 24, "label": "Abstract fold b — 4D", "fav": false}
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
      "title": "Animations · disintegrate & probability field",
      "subtitle": "body blown to dust (disintegrate); Stargate star-streak field",
      "items": [
        {"file": "anim_dissolve_phase.mp4", "src": ["h3_phase_dissolve.png"],
         "effect": "disintegrate", "params": {"wind_angle": 0.3, "wind_px": 70, "turbulence": 8, "grain": 3, "seed": 1},
         "duration": 5, "fps": 24, "label": "H3 phase — disintegrate", "fav": false},
        {"file": "anim_dissolve_abstract.mp4", "src": ["abstract_phase_c.png"],
         "effect": "hyperspace-streak", "params": {"streak_len": 28, "n_stars": 300, "toward": true, "seed": 2},
         "duration": 5, "fps": 24, "label": "Abstract phase — probability field", "fav": false}
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

- [ ] **Step 3: Render the batch**

Run: `./tools/animate --batch voidborn-concepts/anims.json`
Expected: a `wrote ...` line per clip ending `batch: 11 ok, 0 skipped` (11 = 5 fold + 3 chroma + 2 dissolve + 1 crossfade). Confirm with `ls -l voidborn-concepts/anim_*.mp4` (11 nonzero files). If `abstract_fold_b` was omitted in Step 2, expect 10.

- [ ] **Step 4: Rebuild the gallery**

Run: `python3 tools/build_voidborn_gallery.py`
Expected: prints `... (NN images, 11 clips across MM groups)` — clip count 11 (or 10).
Then: `grep -c "<video" voidborn-concepts/index.html` == that clip count.

- [ ] **Step 5: Commit the rewired recipe (force-add past gitignore)**

```bash
git add -f voidborn-concepts/anims.json
git commit -m "feat(kb): rewire animation batch to 4D-warp/streak/disintegrate effects"
```

Then `git status --short` — confirm no `.mp4` or `index.html` is tracked.

---

## Verification (whole feature)

- [ ] `~/sd-venv/bin/python -m pytest tools/test_animate.py -v` — all 29 tests pass.
- [ ] `./tools/animate --batch voidborn-concepts/anims.json` — `11 ok, 0 skipped` (or 10 if `abstract_fold_b` absent).
- [ ] `python3 tools/build_voidborn_gallery.py` — clip count matches; `grep -c "<video"` matches.
- [ ] `git status` — tracked changes only: `tools/animate.py`, `tools/test_animate.py`, `voidborn-concepts/anims.json`. The `.mp4`/`index.html` remain gitignored.
- [ ] **Visual review (manual):** `xdg-open voidborn-concepts/index.html`. Expect a tuning pass on `hyperspace-streak` (streak_len / n_stars / curve_threshold) and possibly `disintegrate` (wind_px / grain) and `hyper-warp amp` — all adjustable in `anims.json`, then re-run the batch.

---

## Notes for the implementer

- **Loop invariant:** `hyper-warp` displacement is taken relative to the rest projection and uses integer `turns` (t=0 and t=1 both identity); `hyperspace-streak` synthetic stars use `(phase+t) mod 1` with a sine envelope that zeroes brightness at the wrap; `disintegrate` uses `alpha=0.5(1-cos 2πt)` with all displacement scaled by per-pixel progress (zero at the endpoints). Tests assert frame0≈frameN with ≤1 tolerance.
- **Caching is correctness-critical for speed, not just optimization:** `subject_mask` (connected components) and the streak's painted layer are recomputed per frame without the `_MASK_CACHE` / `_STREAK_CACHE` memoization — full-res clips would take ~a minute each. Keys use `id(img)`; `render` clears both caches at the start so ids never go stale across clips.
- **No scipy:** `_largest_component` is pure-numpy label propagation; `subject_mask` runs it on a downscaled mask (~256px) for speed, then upsamples and ANDs with the full-res threshold.
- **Expect visual iteration** on `hyperspace-streak` especially (same as v1 `noise-dissolve`). Don't hand-tune in code — adjust `anims.json` params and re-render.
- **v1 effects** `fold-churn` / `noise-dissolve` stay registered and tested; they're just no longer referenced by `anims.json`.
