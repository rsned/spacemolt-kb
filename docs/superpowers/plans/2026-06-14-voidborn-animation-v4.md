# Voidborn Animation v4 (shell-growth + gas-swirl) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two loop-seamless procedural effects — `shell-growth` (rotating mandala core + rotating growth-wave shell) and `gas-swirl` (curl-noise + vortex advection with a cycling containment cage) — to `tools/animate.py`, and wire two new form-animation clips into the gallery.

**Architecture:** Both effects keep the existing `fn(imgs, params, t) -> uint8 (H,W,3)` contract and register in `EFFECTS`. They reuse `remap` (bilinear inverse sampling), `_value_noise` (scalar field), and the `polytope_overlay` AA-line+glow rasterizer. Loop-seamlessness is guaranteed structurally: rigid rotation by integer `turns` (period 1 in t) plus any deformation whose magnitude is modulated by `0.5*(1-cos(2*pi*t))` (zero at t=0 and t=1), and angular waves of the form `frac(theta - t)`.

**Tech Stack:** Python 3, numpy, Pillow (`ImageDraw`/`ImageFilter`), pytest. Run interpreter: `~/sd-venv/bin/python`.

**Design refinements vs the spec** (intentional, for provable looping): the curl flow is *static* (cached) and its displacement *magnitude* breathes via `0.5*(1-cos(2*pi*t))` rather than a continuously-advancing phase; the hurricane spin is a *rigid* rotation by integer `turns` (true differential rotation would wind up and never loop); `vortex` is a dimensionless fraction (tangential bias added to the curl flow), not an absolute speed. These match the spec's intent (curl-noise + vortex advection, semi-Lagrangian backward-trace, loop-seamless) while making the loop exact.

**File structure:**
- `tools/animate.py` — add `_FLOW_CACHE`, `_curl_flow`, `_advect`, `gas_swirl`, `_cage_polygon`, `_draw_cycling_cage`, `shell_growth`; register two effects; clear `_FLOW_CACHE` in `render`.
- `tools/test_animate.py` — add tests for each new helper/effect.
- `voidborn-concepts/anims.json` — add the `anim_forms` group with two clips (tracked via `git add -f`).

---

### Task 1: Curl-noise flow field + cache

**Files:**
- Modify: `tools/animate.py` (add `_FLOW_CACHE` near the other caches ~line 177; add `_curl_flow` after `_value_noise` ~line 130; add `_FLOW_CACHE.clear()` in `render` ~line 707)
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

```python
def test_curl_flow_is_divergence_free():
    fx, fy = animate._curl_flow(64, 64, freq=4.0, seed=1)
    assert fx.shape == (64, 64) and fy.shape == (64, 64)
    # divergence d(fx)/dx + d(fy)/dy ~ 0 for a curl-of-scalar field
    div = np.gradient(fx, axis=1) + np.gradient(fy, axis=0)
    field_mag = np.sqrt(fx**2 + fy**2).mean()
    assert np.abs(div).mean() < 0.2 * field_mag


def test_curl_flow_varies_with_seed_and_is_cached():
    a = animate._curl_flow(48, 48, 4.0, 1)
    b = animate._curl_flow(48, 48, 4.0, 2)
    assert not np.allclose(a[0], b[0])           # different seed -> different field
    assert animate._curl_flow(48, 48, 4.0, 1)[0] is a[0]   # cached identity
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py::test_curl_flow_is_divergence_free -v`
Expected: FAIL with `AttributeError: module 'animate' has no attribute '_curl_flow'`

- [ ] **Step 3: Add the cache dict next to the other module caches**

Find the existing cache block (around line 177):

```python
# t-independent results memoized per render (cleared at the start of render()).
_MASK_CACHE = {}
```

Add below the other cache declarations in that block:

```python
_FLOW_CACHE = {}
```

- [ ] **Step 4: Implement `_curl_flow` (place right after `_value_noise`)**

```python
def _curl_flow(h, w, freq, seed):
    """Static 2D divergence-free flow field from the curl of a value-noise potential.

    The 2D curl of a scalar potential Phi is (dPhi/dy, -dPhi/dx), which is
    divergence-free by construction (mixed partials cancel). Cached per
    (h, w, freq, seed); cleared in render(). Returns (fx, fy), each (H, W)
    float32 normalised so the mean vector magnitude is ~1.
    """
    key = (h, w, round(float(freq), 4), int(seed))
    if key in _FLOW_CACHE:
        return _FLOW_CACHE[key]
    grain = max(1, int(min(h, w) / max(1e-6, float(freq))))
    phi = _value_noise(h, w, grain, seed)        # smooth scalar potential in [0,1]
    gy = np.gradient(phi, axis=0)                 # dPhi/dy (rows)
    gx = np.gradient(phi, axis=1)                 # dPhi/dx (cols)
    fx = gy.astype(np.float32)                    # 2D curl: ( dPhi/dy, -dPhi/dx )
    fy = (-gx).astype(np.float32)
    scale = float(np.sqrt(fx**2 + fy**2).mean()) + 1e-9
    fx, fy = fx / scale, fy / scale
    _FLOW_CACHE[key] = (fx, fy)
    return fx, fy
```

- [ ] **Step 5: Clear the cache in `render`**

Find the cache-clearing block in `render` (around line 705):

```python
    _MASK_CACHE.clear()
    _STREAK_CACHE.clear()
    _BG_CACHE.clear()
```

Add:

```python
    _FLOW_CACHE.clear()
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k curl_flow -v`
Expected: PASS (2 tests)

- [ ] **Step 7: Lint and commit**

```bash
~/sd-venv/bin/python -m pyflakes tools/animate.py || true
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): 2D curl-noise flow field for gas advection"
```

---

### Task 2: Semi-Lagrangian backward-trace advection

**Files:**
- Modify: `tools/animate.py` (add `_advect` after `_curl_flow`)
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

```python
def test_advect_zero_magnitude_is_identity():
    img = _synth(40, 40)
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(np.arange(h, dtype=np.float32),
                         np.arange(w, dtype=np.float32), indexing="ij")
    fx, fy = animate._curl_flow(h, w, 4.0, 0)
    out = animate._advect(img, xs.copy(), ys.copy(), fx, fy, mag=0.0, iters=3)
    assert np.allclose(out, img, atol=1e-4)


def test_advect_nonzero_magnitude_moves_pixels():
    img = _synth(40, 40)
    h, w = img.shape[:2]
    ys, xs = np.meshgrid(np.arange(h, dtype=np.float32),
                         np.arange(w, dtype=np.float32), indexing="ij")
    fx, fy = animate._curl_flow(h, w, 4.0, 0)
    out = animate._advect(img, xs.copy(), ys.copy(), fx, fy, mag=6.0, iters=3)
    assert not np.allclose(out, img, atol=1e-3)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k advect -v`
Expected: FAIL with `AttributeError: module 'animate' has no attribute '_advect'`

- [ ] **Step 3: Implement `_advect`**

```python
def _advect(img, px, py, fx, fy, mag, iters):
    """Semi-Lagrangian backward-trace: step each pixel back along (fx,fy)*mag
    over `iters` sub-steps, then bilinear-sample img at the traced position.

    px, py are (H,W) float start coordinates (already include any rigid
    rotation). 2D port of the planetgen BackwardTrace technique. mag is the
    total displacement in pixels; with mag=0 this is the identity.
    """
    px = px.copy()
    py = py.copy()
    step = float(mag) / max(1, int(iters))
    for _ in range(int(iters)):
        u = remap(fx[..., None], px, py)[..., 0]
        v = remap(fy[..., None], px, py)[..., 0]
        px = px - step * u
        py = py - step * v
    return remap(img, px, py)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k advect -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): semi-Lagrangian backward-trace advection helper"
```

---

### Task 3: `gas-swirl` effect (rotation + curl + vortex, no cage yet)

**Files:**
- Modify: `tools/animate.py` (add `gas_swirl` after `_advect`; register in `EFFECTS` ~line 624)
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

```python
def test_gas_swirl_shape_dtype_and_loops():
    img = _synth(64, 64)
    out = animate.gas_swirl([img], {"turns": 1}, 0.25)
    assert out.shape == (64, 64, 3) and out.dtype == np.uint8
    assert animate._loops(animate.gas_swirl, [img], {"turns": 1}) <= 1


def test_gas_swirl_moves_pixels_midloop():
    img = _synth(64, 64)
    f0 = animate.gas_swirl([img], {"turns": 1, "amp": 16}, 0.0).astype(np.int16)
    fm = animate.gas_swirl([img], {"turns": 1, "amp": 16}, 0.25).astype(np.int16)
    assert np.abs(f0 - fm).mean() > 1.0


def test_gas_swirl_registered():
    assert "gas-swirl" in animate.EFFECTS
    assert animate.EFFECTS["gas-swirl"][0] == 1
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k gas_swirl -v`
Expected: FAIL with `AttributeError: module 'animate' has no attribute 'gas_swirl'`

- [ ] **Step 3: Implement `gas_swirl` (cage branch is a stub flag for now)**

```python
def gas_swirl(imgs, params, t):
    """Curl-noise + vortex advection of a gas image — a top-down hurricane
    approximation. A rigid rotation by integer `turns` gives the seamless spin;
    a static curl flow (plus a tangential `vortex` bias) deforms the gas with a
    magnitude that breathes 0 -> amp -> 0 via 0.5*(1-cos(2*pi*t)), so frame 0
    equals frame N. NOT a fluid sim (see the v4 spec).
    """
    img = imgs[0]
    amp = float(params.get("amp", 16.0))
    vortex = float(params.get("vortex", 0.5))     # dimensionless tangential bias
    turns = int(params.get("turns", 1))
    freq = float(params.get("freq", 3.0))
    iters = int(params.get("iters", 3))
    seed = int(params.get("seed", 0))
    h, w = img.shape[:2]
    cx, cy = (w - 1) / 2.0, (h - 1) / 2.0
    ys, xs = np.meshgrid(np.arange(h, dtype=np.float32),
                         np.arange(w, dtype=np.float32), indexing="ij")

    # rigid rotation: sample source at angle theta -> seamless for integer turns
    ang = 2.0 * np.pi * turns * t
    ca, sa = np.cos(-ang), np.sin(-ang)
    px = cx + (xs - cx) * ca - (ys - cy) * sa
    py = cy + (xs - cx) * sa + (ys - cy) * ca

    # static curl flow + tangential vortex bias
    fx, fy = _curl_flow(h, w, freq, seed)
    dxc, dyc = xs - cx, ys - cy
    r = np.sqrt(dxc**2 + dyc**2) + 1e-6
    fx = fx + vortex * (-dyc / r)
    fy = fy + vortex * (dxc / r)

    mag = amp * 0.5 * (1.0 - np.cos(2.0 * np.pi * t))   # 0 at t=0 and t=1
    out = _advect(img, px, py, fx, fy, mag, iters)
    return to_uint8(out)
```

- [ ] **Step 4: Register the effect**

Find the `EFFECTS` registry (around line 624) and add the `gas-swirl` line:

```python
    "polytope-overlay": (1, polytope_overlay),
    "gas-swirl": (1, gas_swirl),
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k gas_swirl -v`
Expected: PASS (3 tests)

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): gas-swirl effect (rotation + curl/vortex advection)"
```

---

### Task 4: Cycling polygon-cage geometry

**Files:**
- Modify: `tools/animate.py` (add `_cage_polygon` after `gas_swirl`)
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

```python
def _corner_count(pts):
    """Count vertices where the path turns appreciably (a test helper)."""
    m = len(pts)
    corners = 0
    for i in range(m):
        a = pts[i] - pts[(i - 1) % m]
        b = pts[(i + 1) % m] - pts[i]
        na, nb = np.linalg.norm(a), np.linalg.norm(b)
        if na < 1e-6 or nb < 1e-6:
            continue
        cosang = np.dot(a, b) / (na * nb)
        if cosang < 0.999:                # not collinear -> a real corner
            corners += 1
    return corners


def test_cage_polygon_cycles_side_count():
    pts0, _ = animate._cage_polygon(6, 10, 0.0, 1.0, 0.0, 0.0)
    ptsm, _ = animate._cage_polygon(6, 10, 0.5, 1.0, 0.0, 0.0)
    pts1, _ = animate._cage_polygon(6, 10, 1.0, 1.0, 0.0, 0.0)
    assert _corner_count(pts0) == 6
    assert _corner_count(ptsm) == 10
    assert _corner_count(pts1) == 6          # seamless: back to n_min
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k cage_polygon -v`
Expected: FAIL with `AttributeError: module 'animate' has no attribute '_cage_polygon'`

- [ ] **Step 3: Implement `_cage_polygon`**

```python
def _poly_vertices(n, radius, cx, cy):
    """Regular n-gon vertices (n,2), first vertex at the top (-pi/2)."""
    a = -np.pi / 2.0 + 2.0 * np.pi * np.arange(n) / n
    return np.stack([cx + radius * np.cos(a), cy + radius * np.sin(a)], axis=1)


def _resample_closed(verts, m):
    """Resample a closed polygon to m points evenly by arc length."""
    pts = np.vstack([verts, verts[:1]])
    seg = np.linalg.norm(np.diff(pts, axis=0), axis=1)
    cum = np.concatenate([[0.0], np.cumsum(seg)])
    total = cum[-1] + 1e-12
    targets = total * np.arange(m) / m
    out = np.empty((m, 2), dtype=np.float64)
    for i, d in enumerate(targets):
        k = int(np.clip(np.searchsorted(cum, d) - 1, 0, len(seg) - 1))
        f = (d - cum[k]) / max(seg[k], 1e-9)
        out[i] = pts[k] * (1.0 - f) + pts[k + 1] * f
    return out


def _cage_polygon(n_min, n_max, t, radius, cx, cy):
    """Vertices+edges of a regular polygon whose side count cycles
    n_min -> n_max -> n_min over t in [0,1] (triangle wave, seamless).

    Smooth side emergence: a fractional count n is rendered by blending an
    n_base-gon and an (n_base+1)-gon, each resampled to (n_base+1) points, by
    the fractional part. At frac=0 the extra point is collinear (reads as
    n_base sides); at frac=1 it is a full vertex (n_base+1 sides).
    Returns (pts (m,2) float, edges [(i,j),...] closed loop).
    """
    tri = 1.0 - abs(2.0 * t - 1.0)                  # 0 -> 1 -> 0 over t
    n_float = n_min + (n_max - n_min) * tri
    n_base = int(np.floor(n_float))
    n_base = max(n_min, min(n_base, n_max))
    frac = float(np.clip(n_float - n_base, 0.0, 1.0))
    m = n_base + 1
    a = _resample_closed(_poly_vertices(n_base, radius, cx, cy), m)
    b = _resample_closed(_poly_vertices(n_base + 1, radius, cx, cy), m)
    pts = a * (1.0 - frac) + b * frac
    edges = [(i, (i + 1) % m) for i in range(m)]
    return pts, edges
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k cage_polygon -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): cycling polygon-cage geometry (fractional side morph)"
```

---

### Task 5: Cage rasterizer + integrate into `gas-swirl`

**Files:**
- Modify: `tools/animate.py` (add `_draw_cycling_cage` after `_cage_polygon`; add the cage branch in `gas_swirl`)
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

```python
def test_draw_cycling_cage_returns_additive_layer():
    layer = animate._draw_cycling_cage(64, 64, 8, 10, 0.5, 0.8,
                                       np.array([0.6, 1.0, 0.7], np.float32), 3, 8.0)
    assert layer.shape == (64, 64, 3)
    assert layer.min() >= 0.0                 # additive (non-negative)
    assert layer.max() > 0.05                 # something was drawn


def test_gas_swirl_cage_changes_output():
    img = _synth(64, 64)
    no_cage = animate.gas_swirl([img], {"cage": False}, 0.5)
    with_cage = animate.gas_swirl([img], {"cage": True, "cage_min": 8, "cage_max": 10}, 0.5)
    assert not np.array_equal(no_cage, with_cage)
    # still seamless with the cage on
    assert animate._loops(animate.gas_swirl, [img],
                          {"cage": True, "cage_min": 8, "cage_max": 10}) <= 1
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "cycling_cage or gas_swirl_cage" -v`
Expected: FAIL with `AttributeError: module 'animate' has no attribute '_draw_cycling_cage'`

- [ ] **Step 3: Implement `_draw_cycling_cage` (mirrors the polytope AA-line+glow rasterizer)**

```python
def _draw_cycling_cage(h, w, n_min, n_max, t, size, color, width, glow):
    """Additive RGB layer: a regular polygon cage centred in the frame whose
    side count cycles n_min -> n_max -> n_min (seamless). Anti-aliased via a
    2x supersample + LANCZOS downsample, with a Gaussian glow, tinted by color.
    """
    cx, cy = (w - 1) / 2.0, (h - 1) / 2.0
    radius = float(size) * min(h, w) / 2.0
    pts, edges = _cage_polygon(int(n_min), int(n_max), t, radius, cx, cy)
    ss = 2
    canvas = Image.new("L", (w * ss, h * ss), 0)
    draw = ImageDraw.Draw(canvas)
    lw = max(1, int(width) * ss)
    for (i, j) in edges:
        draw.line([(pts[i][0] * ss, pts[i][1] * ss),
                   (pts[j][0] * ss, pts[j][1] * ss)], fill=255, width=lw)
    sharp = np.asarray(canvas.resize((w, h), Image.LANCZOS), dtype=np.float32) / 255.0
    glowed = np.asarray(
        Image.fromarray((sharp * 255).astype(np.uint8)).filter(
            ImageFilter.GaussianBlur(float(glow))), dtype=np.float32) / 255.0
    intensity = np.clip(sharp + 0.6 * glowed, 0.0, 1.0)
    return intensity[..., None] * np.asarray(color, dtype=np.float32)[None, None, :]
```

- [ ] **Step 4: Add the cage branch to `gas_swirl`**

In `gas_swirl`, replace the final two lines:

```python
    mag = amp * 0.5 * (1.0 - np.cos(2.0 * np.pi * t))   # 0 at t=0 and t=1
    out = _advect(img, px, py, fx, fy, mag, iters)
    return to_uint8(out)
```

with:

```python
    mag = amp * 0.5 * (1.0 - np.cos(2.0 * np.pi * t))   # 0 at t=0 and t=1
    out = _advect(img, px, py, fx, fy, mag, iters)
    if params.get("cage"):
        cage = _draw_cycling_cage(
            h, w, int(params.get("cage_min", 8)), int(params.get("cage_max", 10)),
            t, float(params.get("cage_size", 0.78)),
            np.asarray(params.get("cage_color", [0.6, 1.0, 0.7]), dtype=np.float32),
            int(params.get("cage_width", 3)), float(params.get("cage_glow", 8.0)))
        out = 1.0 - (1.0 - out) * (1.0 - cage)          # screen blend
    return to_uint8(out)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k "cycling_cage or gas_swirl" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): cage rasterizer + wire cycling cage into gas-swirl"
```

---

### Task 6: `shell-growth` effect

**Files:**
- Modify: `tools/animate.py` (add `shell_growth` after `_draw_cycling_cage`; register in `EFFECTS`)
- Test: `tools/test_animate.py`

- [ ] **Step 1: Write the failing test**

```python
def _angular_synth(h=72, w=72):
    """A test image with clear angular + radial structure (for rotation/shell)."""
    ys, xs = np.meshgrid(np.linspace(-1, 1, h, dtype=np.float32),
                         np.linspace(-1, 1, w, dtype=np.float32), indexing="ij")
    ang = np.arctan2(ys, xs)
    base = 0.5 + 0.5 * np.sin(3 * ang)
    return np.stack([base, base * 0.6, np.sqrt(xs**2 + ys**2)], axis=-1).astype(np.float32)


def test_shell_growth_shape_dtype_and_loops():
    img = _angular_synth()
    out = animate.shell_growth([img], {"turns": 1}, 0.3)
    assert out.shape == img.shape and out.dtype == np.uint8
    assert animate._loops(animate.shell_growth, [img], {"turns": 1}) <= 1


def test_shell_growth_core_rotates():
    img = _angular_synth()
    f0 = animate.shell_growth([img], {"turns": 1, "coverage": 0.0}, 0.0).astype(np.int16)
    fq = animate.shell_growth([img], {"turns": 1, "coverage": 0.0}, 0.25).astype(np.int16)
    h, w = img.shape[:2]
    cy, cx = h // 2, w // 2
    core = (slice(cy - 6, cy + 6), slice(cx - 6, cx + 6))
    assert np.abs(f0[core] - fq[core]).mean() > 1.0     # core changed under rotation


def test_shell_growth_coverage_varies_and_returns():
    img = _angular_synth()
    def pinkness(t):
        f = animate.shell_growth([img], {"coverage": 0.5, "turns": 1}, t).astype(np.float32)
        return float((f[..., 0] - f[..., 1]).mean())     # tint is pink (R>G)
    p0, pm, p1 = pinkness(0.0), pinkness(0.5), pinkness(1.0)
    assert abs(pm - p0) > 0.5                             # the wave moved
    assert abs(p1 - p0) < 1.0                             # seamless return
```

- [ ] **Step 2: Run test to verify it fails**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k shell_growth -v`
Expected: FAIL with `AttributeError: module 'animate' has no attribute 'shell_growth'`

- [ ] **Step 3: Implement `shell_growth`**

```python
def shell_growth(imgs, params, t):
    """Rotating mandala core + a rotating growth-wave shell with a bright
    fractal leading edge. The core (radius core_r) rotates by 2*pi*turns*t
    (seamless for integer turns); the pink shell layer is present where the
    angular wave frac(theta - t) is within `coverage`, so the arc rotates once
    per loop. Value noise wobbles the front for a fractal edge.
    """
    img = imgs[0]
    turns = int(params.get("turns", 1))
    coverage = float(params.get("coverage", 0.55))
    tint = np.asarray(params.get("tint", [0.95, 0.45, 0.7]), dtype=np.float32)
    edge_glow = float(params.get("edge_glow", 1.4))
    core_r = float(params.get("core_r", 0.32))
    softness = float(params.get("softness", 0.06))
    grain = int(params.get("grain", 3))
    seed = int(params.get("seed", 0))
    h, w = img.shape[:2]
    cx, cy = (w - 1) / 2.0, (h - 1) / 2.0
    ys, xs = np.meshgrid(np.arange(h, dtype=np.float32),
                         np.arange(w, dtype=np.float32), indexing="ij")
    dx = (xs - cx) / (w / 2.0)
    dy = (ys - cy) / (h / 2.0)
    r = np.sqrt(dx**2 + dy**2)
    theta = (np.arctan2(dy, dx) / (2.0 * np.pi)) % 1.0      # [0,1)

    # rotating mandala core
    ang = 2.0 * np.pi * turns * t
    ca, sa = np.cos(-ang), np.sin(-ang)
    rx = cx + (xs - cx) * ca - (ys - cy) * sa
    ry = cy + (xs - cx) * sa + (ys - cy) * ca
    rotated = remap(img, rx, ry)
    core_m = np.clip((core_r - r) / max(softness, 1e-6) + 0.5, 0.0, 1.0)[..., None]
    base = rotated * core_m + img * (1.0 - core_m)

    # rotating growth-wave shell (fractal front via noise wobble)
    nz = _value_noise(h, w, grain, seed) - 0.5
    phase = (theta - t + 0.12 * nz) % 1.0
    rise = np.clip(phase / max(softness, 1e-6), 0.0, 1.0)
    fall = np.clip((coverage - phase) / max(softness, 1e-6), 0.0, 1.0)
    fill = rise * fall                                       # 1 across the arc, 0 outside
    shell_band = (np.clip((r - core_r) / max(softness, 1e-6), 0.0, 1.0)
                  * np.clip((1.15 - r) / 0.1, 0.0, 1.0))
    a = (fill * shell_band)[..., None]
    out = base * (1.0 - a) + tint[None, None, :] * a

    # bright leading edge: glow near the front (phase ~ 0), within the band
    edge = np.exp(-(phase / max(0.5 * softness, 1e-6)) ** 2) * shell_band
    out = out + (edge[..., None] * edge_glow) * tint[None, None, :]
    return to_uint8(np.clip(out, 0.0, 1.0))
```

- [ ] **Step 4: Register the effect**

In `EFFECTS` (around line 624), add the `shell-growth` line below `gas-swirl`:

```python
    "gas-swirl": (1, gas_swirl),
    "shell-growth": (1, shell_growth),
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -k shell_growth -v`
Expected: PASS (3 tests)

- [ ] **Step 6: Run the whole suite + lint, then commit**

Run: `~/sd-venv/bin/python -m pytest tools/test_animate.py -q`
Expected: all pass

```bash
~/sd-venv/bin/python -m pyflakes tools/animate.py || true
git add tools/animate.py tools/test_animate.py
git commit -m "feat(kb): shell-growth effect (rotating core + growth-wave shell)"
```

---

### Task 7: Wire the two clips into `anims.json` + render + gallery

**Files:**
- Modify: `voidborn-concepts/anims.json` (add the `anim_forms` group)
- Verify: `voidborn-concepts/index.html` (rebuilt artifact, gitignored)

- [ ] **Step 1: Add the `anim_forms` group to `anims.json`**

Insert a new group object as the last entry in the `groups` array (after the `anim_poly` group, before the closing `]`):

```json
    {
      "id": "anim_forms",
      "title": "Animations · Beyond the Human Form",
      "subtitle": "non-humanoid form concepts animated: growth-wave shell + gaseous vortex",
      "items": [
        {"file": "anim_form_projection.mp4", "src": ["form_projection_c.png"],
         "effect": "shell-growth",
         "params": {"turns": 1, "coverage": 0.55, "edge_glow": 1.4},
         "duration": 6, "fps": 24, "label": "Dimensional projection — shell growth", "fav": false},
        {"file": "anim_form_gas.mp4", "src": ["form_gas_c.png"],
         "effect": "gas-swirl",
         "params": {"amp": 16, "vortex": 0.7, "turns": 1, "freq": 3.0, "iters": 3,
                    "cage": true, "cage_min": 8, "cage_max": 10},
         "duration": 6, "fps": 24, "label": "Null-matter wisp — vortex + cage", "fav": false}
      ]
    }
```

- [ ] **Step 2: Validate the JSON**

Run: `~/sd-venv/bin/python -c "import json; json.load(open('voidborn-concepts/anims.json')); print('anims.json valid')"`
Expected: `anims.json valid`

- [ ] **Step 3: Render the two new clips (single-clip mode, fast)**

```bash
cd /home/robert/spacemolt/kb
./tools/animate voidborn-concepts/form_projection_c.png --effect shell-growth \
  -p turns=1 -p coverage=0.55 -p edge_glow=1.4 --duration 6 --fps 24 \
  -o voidborn-concepts/anim_form_projection.mp4
./tools/animate voidborn-concepts/form_gas_c.png --effect gas-swirl \
  -p amp=16 -p vortex=0.7 -p turns=1 -p cage=true -p cage_min=8 -p cage_max=10 \
  --duration 6 --fps 24 -o voidborn-concepts/anim_form_gas.mp4
```
Expected: each prints `wrote voidborn-concepts/anim_form_*.mp4 (144 frames)`

- [ ] **Step 4: Rebuild the gallery**

Run: `python3 tools/build_voidborn_gallery.py`
Expected: `wrote .../index.html (102 images, 16 clips across 21 groups)`

- [ ] **Step 5: Commit (force-add the gitignored anims.json)**

```bash
git add -f voidborn-concepts/anims.json
git commit -m "feat(kb): wire form-animation clips (shell-growth, gas-swirl) into gallery"
```

---

## Self-Review

**1. Spec coverage:**
- `_curl_flow` → Task 1. `_advect` (backward-trace) → Task 2. `gas_swirl` → Tasks 3+5. `_draw_cycling_cage`/`_cage_polygon` → Tasks 4+5. `shell_growth` → Task 6. Wiring + new group → Task 7. Gallery auto-includes the group (no builder change) → Task 7 Step 4. Cache cleared in `render` → Task 1 Step 5. Tests for every helper/effect → each task. All spec sections covered.
- Spec deviations (static flow + cos-modulated magnitude + rigid rotation + dimensionless `vortex`) are documented in the plan header and the docstrings; they preserve the spec's intent and make looping exact.

**2. Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step has complete code and exact run/expected lines.

**3. Type consistency:** `_curl_flow(h,w,freq,seed) -> (fx,fy)`; `_advect(img,px,py,fx,fy,mag,iters) -> (H,W,3)`; `_cage_polygon(n_min,n_max,t,radius,cx,cy) -> (pts,edges)`; `_draw_cycling_cage(h,w,n_min,n_max,t,size,color,width,glow) -> (H,W,3)`; `gas_swirl`/`shell_growth([img],params,t) -> uint8`. Names/signatures are consistent across the tasks that call them. `EFFECTS` keys `gas-swirl`/`shell-growth` match the registry and `anims.json` `effect` fields.
