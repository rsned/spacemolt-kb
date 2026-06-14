# Voidborn Polytope Overlay — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `polytope-overlay` effect to `tools/animate.py` that draws a genuine rotating 4D polytope (tesseract / 5-cell / 16-cell) — real 4D→3D→2D projection — as a glowing wireframe screen-blended over a crisp keyframe, then add three clips to `anims.json`.

**Architecture:** Extends `tools/animate.py`. A pure-data `_polytope(shape)` returns vertices/edges; `polytope_overlay(imgs, params, t)` rotates them in 4D, projects to 2D, draws anti-aliased glowing lines with Pillow, and screen-blends over the base. Obeys the existing `fn(imgs, params, t) -> uint8 (H,W,3)` contract and registers in `EFFECTS`. Spec: `docs/superpowers/specs/2026-06-13-voidborn-polytope-overlay-design.md`.

**Tech Stack:** Python 3, numpy, Pillow (`Image`, `ImageDraw`, `ImageFilter` — all already imported), imageio-ffmpeg, pytest. Runs under `~/sd-venv`. Run tests: `~/sd-venv/bin/python -m pytest tools/test_animate.py -v`.

---

## File Structure

- **Modify** `tools/animate.py`:
  - Add `from PIL import Image, ImageDraw, ImageFilter` (currently `Image, ImageFilter` — add `ImageDraw`).
  - Add `_polytope(shape)` and `polytope_overlay(imgs, params, t)` AFTER `disintegrate` and BEFORE the `# Effect registry: name -> ...` comment line.
  - Register `"polytope-overlay": (1, polytope_overlay)` in `EFFECTS`.
- **Modify** `tools/test_animate.py`: add tests for `_polytope` and `polytope_overlay`.
- **Modify** `voidborn-concepts/anims.json`: add a new "4D polytopes" group with three clips; leave existing groups untouched.

**Insertion convention:** the new functions go after `disintegrate` and before the `# Effect registry` comment so they are defined before `EFFECTS` references them.

---

## Task 1: `_polytope(shape)` — vertices & edges

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def test_polytope_counts():
    v, e = animate._polytope("tesseract")
    assert v.shape == (16, 4) and len(e) == 32
    v, e = animate._polytope("16-cell")
    assert v.shape == (8, 4) and len(e) == 24
    v, e = animate._polytope("5-cell")
    assert v.shape == (5, 4) and len(e) == 10


def test_polytope_edges_in_range():
    for shape in ("tesseract", "16-cell", "5-cell"):
        v, e = animate._polytope(shape)
        n = v.shape[0]
        for (i, j) in e:
            assert 0 <= i < n and 0 <= j < n and i != j


def test_polytope_unknown_shape_raises():
    try:
        animate._polytope("dodecaplex")
        assert False, "expected ValueError"
    except ValueError as ex:
        assert "dodecaplex" in str(ex)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k polytope -v`
Expected: FAIL — `AttributeError: module 'animate' has no attribute '_polytope'`.

- [ ] **Step 3: Add `ImageDraw` import and `_polytope`**

In `tools/animate.py`, change:
```python
from PIL import Image, ImageFilter
```
to:
```python
from PIL import Image, ImageDraw, ImageFilter
```

Insert AFTER `disintegrate` and BEFORE the `# Effect registry` comment:
```python
def _polytope(shape):
    """Return (verts (N,4) float32, edges [(i,j), ...]) for a 4D polytope."""
    if shape == "tesseract":
        verts = np.array(
            [[(1.0 if (b >> k) & 1 else -1.0) for k in range(4)] for b in range(16)],
            dtype=np.float32,
        )
        edges = [
            (a, b)
            for a in range(16)
            for b in range(a + 1, 16)
            if bin(a ^ b).count("1") == 1   # differ in exactly one coordinate
        ]
        return verts, edges
    if shape == "16-cell":
        verts = []
        for axis in range(4):
            for sign in (1.0, -1.0):
                p = [0.0, 0.0, 0.0, 0.0]
                p[axis] = sign
                verts.append(p)
        verts = np.array(verts, dtype=np.float32)   # axis*2 + (0 for +, 1 for -)
        edges = [
            (a, b)
            for a in range(8)
            for b in range(a + 1, 8)
            if a // 2 != b // 2          # skip the antipodal pair on the same axis
        ]
        return verts, edges
    if shape == "5-cell":
        # regular 4-simplex: center the R^5 basis, project onto the 4D
        # sum-zero hyperplane via SVD (first 4 right-singular vectors).
        centered = np.eye(5, dtype=np.float64) - 1.0 / 5.0
        _, _, vt = np.linalg.svd(centered)
        verts = (centered @ vt[:4].T).astype(np.float32)   # (5,4)
        edges = [(a, b) for a in range(5) for b in range(a + 1, 5)]
        return verts, edges
    raise ValueError(
        f"unknown shape '{shape}'; valid: 16-cell, 5-cell, tesseract"
    )
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k polytope -v`
Expected: PASS (all three).

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass (29 prior + 3 new = 32).

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): _polytope vertex/edge tables (tesseract, 5-cell, 16-cell)"
```

---

## Task 2: `polytope_overlay` effect

**Files:** Modify `tools/animate.py`, `tools/test_animate.py`

- [ ] **Step 1: Write the failing tests**

Append to `tools/test_animate.py`:
```python
def test_polytope_overlay_shape_and_loop():
    img = _synth()
    frame = animate.polytope_overlay([img], {"shape": "tesseract"}, 0.3)
    assert frame.shape == img.shape
    assert frame.dtype == np.uint8
    assert _loops(animate.polytope_overlay, [img], {"shape": "tesseract"}) <= 1


def test_polytope_overlay_brightens_dark_base():
    base = np.zeros((120, 120, 3), dtype=np.float32)   # dark base
    frame = animate.polytope_overlay([base], {"shape": "tesseract", "size": 0.8}, 0.25)
    # the wireframe adds light over a dark base
    assert int(frame.sum()) > 0


def test_polytope_overlay_screen_never_darkens():
    base = np.full((120, 120, 3), 0.5, dtype=np.float32)
    src = animate.to_uint8(base)
    frame = animate.polytope_overlay([base], {"shape": "5-cell"}, 0.4)
    # screen blend never reduces a pixel below the base value
    assert int(frame.min()) >= int(src.min()) - 1
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k polytope_overlay -v`
Expected: FAIL — `AttributeError: ... 'polytope_overlay'`.

- [ ] **Step 3: Add the effect and register it**

Insert AFTER `_polytope` and BEFORE the `# Effect registry` comment:
```python
def polytope_overlay(imgs, params, t):
    """Screen-blend a rotating 4D polytope wireframe over the keyframe.

    Vertices are rotated in 4D (xw/yw/zw planes) by theta=2*pi*t*turns, then
    projected 4D->3D->2D by perspective. Edges are drawn anti-aliased (2x
    supersample) with a Gaussian glow, tinted by `color`, and screen-blended
    so the painting is preserved and only lit. Integer turns -> exact loop.
    """
    img = imgs[0]
    shape = params.get("shape", "tesseract")
    turns = int(params.get("turns", 1))
    size = float(params.get("size", 0.7))
    width = int(params.get("width", 2))
    glow = float(params.get("glow", 6.0))
    color = np.array(params.get("color", [0.6, 0.8, 1.0]), dtype=np.float32)
    d4 = float(params.get("d4", 2.5))
    d3 = float(params.get("d3", 3.0))
    h, w = img.shape[:2]

    verts, edges = _polytope(shape)
    verts = verts / np.max(np.abs(verts))          # |coord| <= 1 -> safe projection
    theta = 2.0 * np.pi * t * turns
    c, s = np.cos(theta), np.sin(theta)
    x, y, z, wv = verts[:, 0], verts[:, 1], verts[:, 2], verts[:, 3]
    x, wv = x * c - wv * s, x * s + wv * c          # xw plane
    y, wv = y * c - wv * s, y * s + wv * c          # yw plane
    z, wv = z * c - wv * s, z * s + wv * c          # zw plane
    f4 = d4 / (d4 - wv)
    x, y, z = x * f4, y * f4, z * f4
    f3 = d3 / (d3 - z)
    x, y = x * f3, y * f3
    rad = size * min(h, w) / 2.0
    cx, cy = (w - 1) / 2.0, (h - 1) / 2.0
    px = cx + x * rad
    py = cy + y * rad

    # draw edges anti-aliased: 2x supersample -> LANCZOS downsample
    ss = 2
    canvas = Image.new("L", (w * ss, h * ss), 0)
    draw = ImageDraw.Draw(canvas)
    lw = max(1, width * ss)
    for (i, j) in edges:
        draw.line(
            [(px[i] * ss, py[i] * ss), (px[j] * ss, py[j] * ss)],
            fill=255, width=lw,
        )
    sharp = np.asarray(
        canvas.resize((w, h), Image.LANCZOS), dtype=np.float32) / 255.0
    glowed = np.asarray(
        Image.fromarray((sharp * 255).astype(np.uint8)).filter(
            ImageFilter.GaussianBlur(glow)),
        dtype=np.float32) / 255.0
    intensity = np.clip(sharp + 0.6 * glowed, 0.0, 1.0)

    overlay = intensity[..., None] * color[None, None, :]
    out = 1.0 - (1.0 - img) * (1.0 - overlay)
    return to_uint8(out)
```

Add to the `EFFECTS` dict (before its closing `}`):
```python
    "polytope-overlay": (1, polytope_overlay),
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k polytope_overlay -v`
Expected: PASS (all three).

- [ ] **Step 5: Run full suite**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass (32 prior + 3 new = 35).

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): polytope-overlay rotating 4D wireframe effect"
```

---

## Task 3: Add the three polytope clips to `anims.json` + render + rebuild gallery

**Files:** Modify `voidborn-concepts/anims.json`

- [ ] **Step 1: Confirm keyframes**

Run: `ls voidborn-concepts/{abstract_fold_a,abstract_fold_b}.png`
Expected: both listed.

- [ ] **Step 2: Add the new group to `anims.json`**

Open `voidborn-concepts/anims.json`. It is a JSON object `{ "groups": [ ... ] }`. Add the following group object as the LAST element of the `groups` array (i.e. after the existing `anim_crossfade` group, before the closing `]`). Do NOT modify any existing group. Remember to add a comma after the previous group's closing `}`.

```json
    {
      "id": "anim_poly",
      "title": "Animations · 4D polytopes",
      "subtitle": "true rotating 4D wireframes (real 4D→3D→2D projection) over the fold concepts",
      "items": [
        {"file": "anim_poly_tesseract.mp4", "src": ["abstract_fold_a.png"],
         "effect": "polytope-overlay", "params": {"shape": "tesseract", "size": 0.75, "turns": 1},
         "duration": 6, "fps": 24, "label": "Tesseract over fold tunnel", "fav": false},
        {"file": "anim_poly_16cell.mp4", "src": ["abstract_fold_b.png"],
         "effect": "polytope-overlay", "params": {"shape": "16-cell", "size": 0.75, "turns": 1},
         "duration": 6, "fps": 24, "label": "16-cell over fold b", "fav": false},
        {"file": "anim_poly_5cell.mp4", "src": ["abstract_fold_a.png"],
         "effect": "polytope-overlay", "params": {"shape": "5-cell", "size": 0.8, "turns": 1},
         "duration": 6, "fps": 24, "label": "5-cell over fold tunnel", "fav": false}
      ]
    }
```

After editing, validate the JSON: `~/sd-venv/bin/python -c "import json; json.load(open('voidborn-concepts/anims.json')); print('valid')"`
Expected: `valid`.

- [ ] **Step 3: Render the batch**

Run: `./tools/animate --batch voidborn-concepts/anims.json`
Expected: a `wrote ...` line per clip ending `batch: 14 ok, 0 skipped`. Confirm the three new files: `ls -l voidborn-concepts/anim_poly_*.mp4` (3 nonzero files). If any clip is SKIPPED, report the exact error (do not accept silently).

- [ ] **Step 4: Rebuild the gallery**

Run: `python3 tools/build_voidborn_gallery.py`
Expected: prints `... (NN images, 14 clips across MM groups)`. Then `grep -c "<video" voidborn-concepts/index.html` == 14.

- [ ] **Step 5: Commit the recipe (force-add past gitignore)**

```bash
git add -f voidborn-concepts/anims.json
git commit -m "feat(kb): add 4D polytope-overlay clips (tesseract, 5-cell, 16-cell)"
```

Then `git status --short` — confirm no `.mp4` or `index.html` is tracked.

---

## Verification (whole feature)

- [ ] `~/sd-venv/bin/python -m pytest tools/test_animate.py -v` — all 35 tests pass.
- [ ] `./tools/animate --batch voidborn-concepts/anims.json` — `14 ok, 0 skipped`.
- [ ] `python3 tools/build_voidborn_gallery.py` — 14 clips; `grep -c "<video"` == 14.
- [ ] `git status` — tracked changes only `tools/animate.py`, `tools/test_animate.py`, `voidborn-concepts/anims.json`; `.mp4`/`index.html` remain gitignored.
- [ ] **Visual review (manual):** `xdg-open voidborn-concepts/index.html` → the "4D polytopes" group. Expect possible tuning of `size`/`width`/`glow`/`color` in `anims.json`, then re-render.

---

## Notes for the implementer

- **Loop invariant:** the wireframe uses no per-frame randomness; integer `turns` makes the t=0 and t=1 vertex sets identical, so the frame is identical (test tolerance ≤1 guards against line-raster float noise).
- **Projection safety:** verts are normalized so `|w|,|z| ≤ 1`; with `d4=2.5`, `d3=3.0` the denominators `d - w`, `d - z` are always positive — no clamp needed, but keep the normalization line.
- **Anti-aliasing:** PIL `ImageDraw.line` is not AA; draw at 2× and downsample with `Image.LANCZOS`. Glow is a Gaussian-blurred copy added at 0.6 weight.
- **Screen blend** (`1-(1-base)(1-overlay)`) only lightens, so the crisp keyframe is preserved under the wireframe.
- This effect needs no caching (a handful of line draws per frame).
- Existing effects and clips are untouched; this is purely additive.
