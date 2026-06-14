# Voidborn Animation Effects v3 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the disintegrate effect (fill mask holes so the whole figure dissolves; rewrite the dissolve so it fully clears into a reconstructed background; emit an alpha WebM), and make hyperspace-streak brighter / faster / denser / matched to the keyframe's radial streaks.

**Architecture:** Extends `tools/animate.py`. Adds `_fill_holes`, `_inpaint_background` (+ `_BG_CACHE`), `_disintegrate_fields`, `encode_webm_alpha`; rewrites `disintegrate`; updates `_draw_streaking_stars` + `hyperspace_streak`; extends `render`/`run_batch` with an opt-in `alpha` path. The `fn(imgs, params, t) -> uint8 (H,W,3)` contract is unchanged; alpha is a parallel render path. Spec: `docs/superpowers/specs/2026-06-14-voidborn-animation-v3-design.md`.

**Tech Stack:** Python 3, numpy, Pillow, imageio-ffmpeg (bundled ffmpeg has `libvpx-vp9` + `yuva420p`, verified), pytest. Runs under `~/sd-venv`. Tests: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`.

---

## File Structure

- **Modify** `tools/animate.py`:
  - Add `_fill_holes` (after `_largest_component`); call it inside `subject_mask` and stop AND-ing with the bright threshold (use the filled silhouette).
  - Add `_blur_rgb`, `_inpaint_background`, and `_BG_CACHE = {}` (near the other caches).
  - Add `_disintegrate_fields`; rewrite `disintegrate` to delegate to it.
  - Add `encode_webm_alpha` and `_write_png_sequence`.
  - Extend `render(..., alpha=False)` to clear `_BG_CACHE` and emit a sibling `.webm` for disintegrate when `alpha`; `run_batch` passes `item.get("alpha", False)`.
  - Update `_draw_streaking_stars` (speed/intensity/approach-flare, whiter tint) and `hyperspace_streak` (new defaults, screen-blend).
- **Modify** `tools/test_animate.py`: tests per task.
- **Modify** `voidborn-concepts/anims.json`: disintegrate clip duration 7 + `alpha:true`; hyperspace clip brighter/faster/denser params.

---

## Task 1: `_fill_holes` + solid filled `subject_mask`

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def test_fill_holes_fills_interior_and_leaves_open_unchanged():
    a = np.zeros((20, 20), dtype=bool)
    a[4:16, 4:16] = True
    a[8:12, 8:12] = False           # punched interior hole
    filled = animate._fill_holes(a)
    assert filled[10, 10]           # hole is filled
    assert filled[4:16, 4:16].all()
    # a mask with no enclosed hole is unchanged
    b = np.zeros((10, 10), dtype=bool)
    b[2:5, 2:5] = True
    assert np.array_equal(animate._fill_holes(b), b)
    # all-zero stays all-zero
    z = np.zeros((6, 6), dtype=bool)
    assert not animate._fill_holes(z).any()


def _blob_notched(h=80, w=80):
    img = _blob(h, w)
    # carve a dark interior notch (like an eye socket) inside the bright disk
    img[36:44, 36:44] = 0.0
    return img


def test_subject_mask_fills_interior_notch():
    m = animate.subject_mask(_blob_notched(), {})
    assert m[40, 40] > 0.8          # the dark interior notch is now masked
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "fill_holes or interior_notch" -v`
Expected: FAIL — `AttributeError: ... '_fill_holes'` (and the notch test fails: center is a hole).

- [ ] **Step 3: Add `_fill_holes` and use it in `subject_mask`**

Insert AFTER `_largest_component` (which ends with `return labels == uniq[...]`) and BEFORE `def subject_mask`:
```python
def _fill_holes(mask_bool):
    """Fill enclosed interior holes of a boolean mask.

    Flood the background inward from the image border on ~mask; any background
    pixel NOT reachable from the border is an enclosed hole -> set foreground.
    """
    free = ~mask_bool
    reached = np.zeros_like(mask_bool)
    reached[0, :] |= free[0, :]
    reached[-1, :] |= free[-1, :]
    reached[:, 0] |= free[:, 0]
    reached[:, -1] |= free[:, -1]
    while True:
        prev = reached
        cur = reached.copy()
        cur[1:, :] |= reached[:-1, :] & free[1:, :]
        cur[:-1, :] |= reached[1:, :] & free[:-1, :]
        cur[:, 1:] |= reached[:, :-1] & free[:, 1:]
        cur[:, :-1] |= reached[:, 1:] & free[:, :-1]
        reached = cur
        if np.array_equal(reached, prev):
            break
    return mask_bool | (free & ~reached)
```

In `subject_mask`, find:
```python
    comp_small = _largest_component(binar[::scale, ::scale])
    comp = np.asarray(
        Image.fromarray((comp_small.astype(np.uint8) * 255)).resize(
            (w, h), Image.NEAREST),
        dtype=np.uint8) > 0
    keep = (binar & comp).astype(np.uint8) * 255
```
Replace with:
```python
    comp_small = _fill_holes(_largest_component(binar[::scale, ::scale]))
    comp = np.asarray(
        Image.fromarray((comp_small.astype(np.uint8) * 255)).resize(
            (w, h), Image.NEAREST),
        dtype=np.uint8) > 0
    keep = comp.astype(np.uint8) * 255   # solid filled silhouette (holes filled)
```
(The `_fill_holes` runs on the reduced-res component so it is cheap; `keep` is now the filled silhouette rather than only the bright pixels, so interior features are covered.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "fill_holes or interior_notch or subject_mask or largest_component" -v`
Expected: PASS (the prior subject_mask tests still pass; new ones pass).

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass (35 prior + 2 new = 37).

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): fill subject_mask interior holes (solid silhouette)"
```

---

## Task 2: `_inpaint_background` (diffusion fill of the void)

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

Append to `tools/test_animate.py`:
```python
def test_inpaint_background_fills_masked_region_from_surroundings():
    # vertical gradient background with a bright square "figure" in the middle
    h, w = 80, 80
    grad = np.linspace(0.1, 0.6, h, dtype=np.float32)[:, None, None]
    img = np.repeat(np.repeat(grad, w, axis=1), 3, axis=2).copy()
    img[30:50, 30:50] = 1.0                       # figure to remove
    mask = np.zeros((h, w), dtype=np.float32)
    mask[30:50, 30:50] = 1.0
    bg = animate._inpaint_background(img, mask)
    # the inpainted center should look like the surrounding gradient, not white
    assert bg[40, 40, 0] < 0.8
    # unmasked pixels are preserved
    assert np.allclose(bg[5, 5], img[5, 5], atol=0.05)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k inpaint -v`
Expected: FAIL — `AttributeError: ... '_inpaint_background'`.

- [ ] **Step 3: Add `_BG_CACHE`, `_blur_rgb`, `_inpaint_background`**

Find the cache declarations:
```python
_MASK_CACHE = {}
_STREAK_CACHE = {}
```
Replace with:
```python
_MASK_CACHE = {}
_STREAK_CACHE = {}
_BG_CACHE = {}
```

Insert AFTER `subject_mask` (before `def hyper_warp`):
```python
def _blur_rgb(arr, radius):
    """Gaussian-blur an (H,W,3) float image in [0,1]."""
    return np.asarray(
        Image.fromarray(to_uint8(arr)).filter(ImageFilter.GaussianBlur(radius)),
        dtype=np.float32) / 255.0


def _inpaint_background(img, mask):
    """Reconstruct the scene behind the masked figure by diffusion inpaint.

    Works at reduced resolution (the void is low-frequency): zero the masked
    region, then repeatedly blur and re-insert the known background so it
    diffuses into the hole; upscale and keep the original where unmasked.
    Memoized in _BG_CACHE (t-independent).
    """
    h, w = img.shape[:2]
    key = (id(img), img.shape, round(float(mask.sum()), 1))
    cached = _BG_CACHE.get(key)
    if cached is not None:
        return cached
    scale = max(1, int(max(h, w) / 256))
    sw, sh = max(1, w // scale), max(1, h // scale)
    small = np.asarray(
        Image.fromarray(to_uint8(img)).resize((sw, sh), Image.BILINEAR),
        dtype=np.float32) / 255.0
    msmall = np.asarray(
        Image.fromarray((mask * 255).astype(np.uint8)).resize((sw, sh), Image.BILINEAR),
        dtype=np.float32) / 255.0
    known = (msmall < 0.5)[..., None]
    cur = small * known
    for _ in range(60):
        cur = np.where(known, small, _blur_rgb(cur, 4.0))
    big = np.asarray(
        Image.fromarray(to_uint8(cur)).resize((w, h), Image.BILINEAR),
        dtype=np.float32) / 255.0
    out = np.where((mask < 0.5)[..., None], img, big)
    _BG_CACHE[key] = out
    return out
```
(The cache key uses `id(img)`, `img.shape`, and `mask.sum()` to distinguish masks; `render` clears `_BG_CACHE` per clip so ids never go stale.)

- [ ] **Step 4: Run test to verify it passes**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k inpaint -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): _inpaint_background diffusion void reconstruction"
```

---

## Task 3: Rewrite `disintegrate` (full clear + RGBA fields)

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def test_disintegrate_fully_clears_at_peak():
    img = _blob()
    bg = animate._inpaint_background(img, animate.subject_mask(img, {}))
    mid = animate.disintegrate([img], {"seed": 2}, 0.5).astype(np.int16)
    # inside the body, the peak frame matches the inpainted background (cleared)
    cy, cx = 40, 40
    assert abs(int(mid[cy, cx, 0]) - int(animate.to_uint8(bg)[cy, cx, 0])) < 25


def test_disintegrate_endpoints_and_loop():
    img = _blob()
    src = animate.to_uint8(img)
    f0 = animate.disintegrate([img], {"seed": 2}, 0.0).astype(np.int16)
    # endpoints reform to ~original inside the body
    assert abs(int(f0[40, 40, 0]) - int(src[40, 40, 0])) <= 6
    assert _loops(animate.disintegrate, [img], {"seed": 2}) <= 6


def test_disintegrate_fields_rgba_alpha():
    img = _blob()
    _, rgba0 = animate._disintegrate_fields(img, {"seed": 2}, 0.0)
    _, rgbamid = animate._disintegrate_fields(img, {"seed": 2}, 0.5)
    assert rgba0.shape == (img.shape[0], img.shape[1], 4)
    assert rgba0.dtype == np.uint8
    m = animate.subject_mask(img, {})
    body = m > 0.5
    assert rgba0[..., 3][body].mean() > 180      # opaque body at t=0
    assert rgbamid[..., 3][body].mean() < 40      # transparent at peak
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "disintegrate_fully or disintegrate_endpoints or disintegrate_fields" -v`
Expected: FAIL — `_disintegrate_fields` missing, and the old `disintegrate` does not clear to background at the peak.

- [ ] **Step 3: Replace `disintegrate` with the rewritten version**

Find the entire existing `disintegrate` function (from `def disintegrate(imgs, params, t):` through its `return to_uint8(out)`) and replace it with:
```python
def _disintegrate_fields(img, params, t):
    """Core of the disintegrate effect.

    Returns (rgb_uint8, rgba_uint8): the dust composited over the inpainted
    void (for mp4), and dust-color + presence-alpha (for an alpha clip).
    A per-pixel presence fades 1->0 as the dissolve front (alpha_t vs noise)
    passes it; presence and dust are advected along the wind. Noise is scaled
    to [0,1-band] so at the peak (alpha_t=1) every body pixel reaches 0 ->
    fully cleared. Ping-pong alpha_t makes t=0 and t=1 the intact keyframe.
    """
    wind_angle = float(params.get("wind_angle", 0.3))
    wind_px = float(params.get("wind_px", 70.0))
    turbulence = float(params.get("turbulence", 8.0))
    grain = int(params.get("grain", 3))
    seed = int(params.get("seed", 0))
    band = float(params.get("fade_band", 0.25))
    h, w = img.shape[:2]

    m = subject_mask(img, params)
    bg = _inpaint_background(img, m)
    noise = _value_noise(h, w, grain, seed) * (1.0 - band)   # in [0, 1-band]
    alpha_t = 0.5 * (1.0 - np.cos(2.0 * np.pi * t))
    presence = np.clip((noise + band - alpha_t) / band, 0.0, 1.0) * m
    diss = np.clip((alpha_t - noise) / band, 0.0, 1.0)

    ys, xs = np.meshgrid(
        np.arange(h, dtype=np.float32),
        np.arange(w, dtype=np.float32),
        indexing="ij",
    )
    wx, wy = np.cos(wind_angle), np.sin(wind_angle)
    turb = (noise - 0.5) * turbulence
    off_x = (wind_px * wx + turb) * diss
    off_y = (wind_px * wy + turb) * diss

    a = remap(presence[..., None], xs - off_x, ys - off_y)[..., 0]
    dust = remap(img * m[..., None], xs - off_x, ys - off_y)
    a3 = a[..., None]
    rgb = to_uint8(bg * (1.0 - a3) + dust * a3)
    rgba = np.concatenate([to_uint8(dust), to_uint8(a)[..., None]], axis=-1)
    return rgb, rgba


def disintegrate(imgs, params, t):
    """Erode the masked figure into wind-blown dust over the reconstructed
    void, fully clearing at the peak and reforming (ping-pong, seamless loop).
    """
    rgb, _ = _disintegrate_fields(imgs[0], params, t)
    return rgb
```

(The `EFFECTS` entry `"disintegrate": (1, disintegrate)` already exists and still points to the right function — no registry change needed.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "disintegrate" -v`
Expected: PASS (the rewritten endpoints/clear/loop/rgba tests; the older `test_disintegrate_*` from v2 that asserted black-vacate may need to be superseded — see note).

> **Note:** the v2 tests `test_disintegrate_midpoint_erodes_body` and `test_disintegrate_leaves_background_untouched` should still hold (midpoint differs from source; background corner unchanged). If `test_disintegrate_leaves_background_untouched` fails because the corner now equals the inpainted bg (which equals the source there anyway), confirm by inspection that the corner value is unchanged; it should still pass since `_inpaint_background` keeps unmasked pixels equal to the source. Do not weaken assertions — if a v2 test genuinely conflicts with the new (correct) behavior, report it for the controller to adjudicate rather than editing it silently.

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): rewrite disintegrate (full clear into inpainted void + RGBA)"
```

---

## Task 4: Alpha output — `encode_webm_alpha` + `render(alpha=...)`

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def test_encode_webm_alpha_writes_readable_file(tmp_path):
    frames = [
        np.dstack([
            np.full((64, 64), 200, np.uint8),
            np.full((64, 64), 100, np.uint8),
            np.full((64, 64), 50, np.uint8),
            np.full((64, 64), i * 80, np.uint8),   # varying alpha
        ]) for i in range(3)
    ]
    out = tmp_path / "a.webm"
    animate.encode_webm_alpha(iter(frames), str(out), 8, (64, 64))
    assert out.exists() and out.stat().st_size > 0


def test_render_alpha_emits_webm(tmp_path):
    src = tmp_path / "k.png"
    Image.fromarray(animate.to_uint8(_blob())).save(src)
    out = tmp_path / "o.mp4"
    animate.render([str(src)], "disintegrate", {"seed": 2}, 1.0, 6, str(out), alpha=True)
    assert out.exists()
    assert (tmp_path / "o.webm").exists()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "webm_alpha or render_alpha" -v`
Expected: FAIL — `encode_webm_alpha` missing / `render` has no `alpha` kwarg.

- [ ] **Step 3: Add `encode_webm_alpha`, `_write_png_sequence`; extend `render` and `run_batch`**

Insert AFTER `encode_mp4` (before `def render`):
```python
def encode_webm_alpha(frames, out, fps, size):
    """Encode (H,W,4) uint8 RGBA frames to a VP9 WebM with alpha (yuva420p)."""
    import imageio_ffmpeg

    w, h = size
    writer = imageio_ffmpeg.write_frames(
        out,
        (w, h),
        pix_fmt_in="rgba",
        pix_fmt_out="yuva420p",
        codec="libvpx-vp9",
        fps=fps,
        macro_block_size=1,
    )
    writer.send(None)
    for fr in frames:
        writer.send(np.ascontiguousarray(fr, dtype=np.uint8).tobytes())
    writer.close()


def _write_png_sequence(frames, out_dir):
    """Fallback: write RGBA frames as numbered PNGs in out_dir/."""
    os.makedirs(out_dir, exist_ok=True)
    for i, fr in enumerate(frames):
        Image.fromarray(np.ascontiguousarray(fr, dtype=np.uint8), "RGBA").save(
            os.path.join(out_dir, f"{i:04d}.png"))
```

In `render`, change the signature line:
```python
def render(srcs, effect, params, duration, fps, out):
```
to:
```python
def render(srcs, effect, params, duration, fps, out, alpha=False):
```

In `render`, find:
```python
    _MASK_CACHE.clear()
    _STREAK_CACHE.clear()
```
Replace with:
```python
    _MASK_CACHE.clear()
    _STREAK_CACHE.clear()
    _BG_CACHE.clear()
```

In `render`, find the tail:
```python
    n = max(1, round(duration * fps))
    frames = (fn(imgs, params, i / n) for i in range(n))
    encode_mp4(frames, out, fps, (w, h))
    return out, n
```
Replace with:
```python
    n = max(1, round(duration * fps))
    frames = (fn(imgs, params, i / n) for i in range(n))
    encode_mp4(frames, out, fps, (w, h))
    if alpha and effect == "disintegrate":
        webm = os.path.splitext(out)[0] + ".webm"
        rgba = (_disintegrate_fields(imgs[0], params, i / n)[1] for i in range(n))
        try:
            encode_webm_alpha(rgba, webm, fps, (w, h))
        except Exception as e:  # noqa: BLE001 — fall back to a PNG sequence
            seq = (_disintegrate_fields(imgs[0], params, i / n)[1] for i in range(n))
            _write_png_sequence(seq, os.path.splitext(out)[0] + "_frames")
            print(f"vp9-alpha failed ({e}); wrote PNG sequence", file=sys.stderr)
    return out, n
```

In `run_batch`, find the `render(` call:
```python
                render(
                    srcs,
                    item["effect"],
                    item.get("params", {}),
                    item.get("duration", 4.0),
                    item.get("fps", 24),
                    out,
                )
```
Replace with:
```python
                render(
                    srcs,
                    item["effect"],
                    item.get("params", {}),
                    item.get("duration", 4.0),
                    item.get("fps", 24),
                    out,
                    alpha=item.get("alpha", False),
                )
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "webm_alpha or render_alpha" -v`
Expected: PASS. (If VP9-alpha is unavailable, `test_render_alpha_emits_webm` would fail because no `.webm` is produced; the bundled ffmpeg has `libvpx-vp9`, so it should write the webm. If it genuinely cannot, report it — do not weaken the test.)

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): alpha WebM output path for disintegrate (VP9 yuva420p)"
```

---

## Task 5: `hyperspace-streak` brighter / faster / denser / matched

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def test_hyperspace_brighter_with_intensity():
    img = _point_with_curve()
    bright = animate.hyperspace_streak(
        [img], {"seed": 1, "n_stars": 700, "intensity": 2.0, "speed": 2.0}, 0.4)
    dim = animate.hyperspace_streak(
        [img], {"seed": 1, "n_stars": 20, "intensity": 0.2, "speed": 2.0}, 0.4)
    assert int(bright.sum()) > int(dim.sum())


def test_hyperspace_still_loops_and_keeps_curve():
    img = _point_with_curve()
    p = {"seed": 1, "n_stars": 200, "intensity": 1.6, "speed": 2.0}
    assert _loops(animate.hyperspace_streak, [img], p) <= 1
    frame = animate.hyperspace_streak([img], p, 0.4)
    assert frame[:, 40].max() > 180          # fixed curve preserved
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "hyperspace_brighter or still_loops" -v`
Expected: FAIL — `intensity`/`speed` not honored (brightness scaling absent), so `bright.sum() > dim.sum()` may not hold and/or signature mismatch.

- [ ] **Step 3: Update `_draw_streaking_stars` and `hyperspace_streak`**

Replace the entire existing `_draw_streaking_stars` function with:
```python
def _draw_streaking_stars(h, w, flow_x, flow_y, n_stars, streak_len, seed, t,
                          speed=2.0, intensity=1.6):
    """Synthetic stars streaming along the flow toward the curve, looping via
    (phase+t) mod 1. A sine envelope hides the wrap; brightness flares as the
    star approaches the curve. Returns (H,W,3) float. Long thin radial streaks.
    """
    rng = np.random.default_rng(seed)
    base = rng.random(n_stars)
    px = rng.integers(0, w, n_stars)
    py = rng.integers(0, h, n_stars)
    bri = rng.uniform(0.5, 1.0, n_stars)
    tint = np.array([0.85, 0.92, 1.0], dtype=np.float32)   # near-white cool
    out = np.zeros((h, w, 3), dtype=np.float32)
    max_travel = streak_len * speed
    for i in range(n_stars):
        phase = (base[i] + t) % 1.0
        env = np.sin(np.pi * phase)        # 0 at wrap, 1 mid-travel
        if env <= 0:
            continue
        flare = 0.4 + 0.6 * phase          # brighter as it nears the curve
        dist = phase * max_travel
        fx = float(flow_x[py[i], px[i]])
        fy = float(flow_y[py[i], px[i]])
        amp = bri[i] * intensity * env * flare
        for k in range(streak_len):
            x = int(px[i] + fx * (dist - k))
            y = int(py[i] + fy * (dist - k))
            if 0 <= x < w and 0 <= y < h:
                out[y, x] += amp * (1.0 - k / streak_len) * tint
    return np.clip(out, 0.0, 1.0)
```

In `hyperspace_streak`, find the param reads:
```python
    streak_len = int(params.get("streak_len", 24))
    n_stars = int(params.get("n_stars", 240))
    toward = bool(params.get("toward", True))
    curve_threshold = float(params.get("curve_threshold", 0.5))
    seed = int(params.get("seed", 0))
```
Replace with:
```python
    streak_len = int(params.get("streak_len", 40))
    n_stars = int(params.get("n_stars", 700))
    toward = bool(params.get("toward", True))
    curve_threshold = float(params.get("curve_threshold", 0.5))
    seed = int(params.get("seed", 0))
    speed = float(params.get("speed", 2.0))
    intensity = float(params.get("intensity", 1.6))
```

In `hyperspace_streak`, find the compositing tail:
```python
    stars = _draw_streaking_stars(h, w, fx, fy, n_stars, streak_len, seed, t)
    field = np.maximum(streaked, stars)
    return to_uint8(np.maximum(field, curve_rgb))
```
Replace with:
```python
    stars = _draw_streaking_stars(
        h, w, fx, fy, n_stars, streak_len, seed, t, speed, intensity)
    field = 1.0 - (1.0 - streaked) * (1.0 - stars)   # screen-blend the stars
    return to_uint8(np.maximum(field, curve_rgb))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "hyperspace" -v`
Expected: PASS (new brightness/loop/curve tests and the prior hyperspace tests).

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): hyperspace-streak brighter/faster/denser with approach flare"
```

---

## Task 6: Rewire `anims.json`, render (with alpha), rebuild gallery

**Files:** Modify `voidborn-concepts/anims.json`

- [ ] **Step 1: Update the two clip items**

In `voidborn-concepts/anims.json`, find the disintegrate item:
```json
        {"file": "anim_dissolve_phase.mp4", "src": ["h3_phase_dissolve.png"],
         "effect": "disintegrate", "params": {"wind_angle": 0.3, "wind_px": 70, "turbulence": 8, "grain": 3, "seed": 1},
         "duration": 5, "fps": 24, "label": "H3 phase — disintegrate", "fav": false},
```
Replace with:
```json
        {"file": "anim_dissolve_phase.mp4", "src": ["h3_phase_dissolve.png"],
         "effect": "disintegrate", "params": {"wind_angle": 0.3, "wind_px": 70, "turbulence": 8, "grain": 3, "seed": 1, "fade_band": 0.25},
         "duration": 7, "fps": 24, "alpha": true, "label": "H3 phase — disintegrate", "fav": false},
```

Find the hyperspace item:
```json
        {"file": "anim_dissolve_abstract.mp4", "src": ["abstract_phase_c.png"],
         "effect": "hyperspace-streak", "params": {"streak_len": 28, "n_stars": 300, "toward": true, "seed": 2},
         "duration": 5, "fps": 24, "label": "Abstract phase — probability field", "fav": false},
```
Replace with:
```json
        {"file": "anim_dissolve_abstract.mp4", "src": ["abstract_phase_c.png"],
         "effect": "hyperspace-streak", "params": {"streak_len": 40, "n_stars": 700, "speed": 2.0, "intensity": 1.6, "toward": true, "seed": 2},
         "duration": 6, "fps": 24, "label": "Abstract phase — probability field", "fav": false},
```

Validate: `~/sd-venv/bin/python -c "import json; json.load(open('voidborn-concepts/anims.json')); print('valid')"` → `valid`.

- [ ] **Step 2: Render the batch**

Run: `./tools/animate --batch voidborn-concepts/anims.json`
Expected: `batch: 14 ok, 0 skipped`. Then confirm the alpha sibling exists:
`ls -l voidborn-concepts/anim_dissolve_phase.webm` (nonzero). If any clip SKIPPED, report the exact error.

- [ ] **Step 3: Verify the disintegrate clip clears and the webm has alpha**

Run:
```bash
~/sd-venv/bin/python - <<'PY'
import imageio_ffmpeg as f
for c in ("anim_dissolve_phase.mp4","anim_dissolve_phase.webm","anim_dissolve_abstract.mp4"):
    r=f.read_frames(f"voidborn-concepts/{c}"); m=next(r); n=sum(1 for _ in r)
    print(c, "frames=",n,"dur=",round(m.get("duration") or 0,2))
PY
```
Expected: `anim_dissolve_phase.mp4` ~168 frames (7s·24); `.webm` similar; `anim_dissolve_abstract.mp4` ~144.

- [ ] **Step 4: Rebuild the gallery**

Run: `python3 tools/build_voidborn_gallery.py`
Expected: `... (NN images, 14 clips ...)`; `grep -c "<video" voidborn-concepts/index.html` == 14.

- [ ] **Step 5: Commit the recipe (force-add past gitignore)**

```bash
git add -f voidborn-concepts/anims.json
git commit -m "feat(kb): v3 anims wiring (disintegrate 7s+alpha, brighter hyperspace)"
```

Then `git status --short` — confirm no `.mp4`/`.webm`/`index.html` tracked (only anims.json).

---

## Verification (whole feature)

- [ ] `~/sd-venv/bin/python -m pytest tools/test_animate.py -v` — all tests pass.
- [ ] `./tools/animate --batch voidborn-concepts/anims.json` — `14 ok, 0 skipped`; `anim_dissolve_phase.webm` produced.
- [ ] `python3 tools/build_voidborn_gallery.py` — 14 `<video>` tiles.
- [ ] `git status` — tracked changes only `tools/animate.py`, `tools/test_animate.py`, `voidborn-concepts/anims.json`.
- [ ] **Visual review (manual):** the dissolve now consumes the whole figure (face included) and fully clears into the void; the `.webm` is transparent where dissolved; hyperspace streaks are bright/fast/dense/radial. Expect possible second tuning of `wind_px`/`fade_band` and hyperspace `intensity`/`speed` via `anims.json`.

---

## Notes for the implementer

- **Root cause first:** Task 1 (mask hole-fill) is what makes the figure dissolve evenly; the Task 3 rewrite is what makes it complete and reveal the void. They depend on each other — keep the order.
- **Caching:** `_BG_CACHE` joins `_MASK_CACHE`/`_STREAK_CACHE`; all three are cleared at the start of `render`. The inpaint and mask run once per clip; per-frame cost is a couple of `remap`s.
- **Loop invariants unchanged:** disintegrate ping-pong (`0.5*(1-cos2πt)`, all displacement ∝ `diss` which is 0 at the endpoints); hyperspace stars `(phase+t)%1` with sine envelope. Tolerances ≤6 for disintegrate (resample), ≤1 for hyperspace.
- **Do not weaken v2 tests.** If a v2 disintegrate assertion conflicts with the corrected behavior, surface it to the controller rather than editing it silently.
- **Alpha format:** VP9 `yuva420p` WebM (bundled ffmpeg supports it); PNG-sequence fallback only on encode failure.
