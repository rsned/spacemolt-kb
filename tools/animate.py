#!/usr/bin/env python3
"""animate — procedurally animate Voidborn keyframes into looping MP4 clips.

Pure-CPU numpy/Pillow compositing (no generative model). Each effect is a
function of a loop phase t in [0,1) so frame 0 and frame N match: the MP4
loops seamlessly with <video loop>. See
docs/superpowers/specs/2026-06-13-voidborn-procedural-animation-design.md.
"""
import argparse
import json
import os
import sys
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFilter


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
    # 1.5 over-drives the mean-color tint so the noise reads as luminous
    # substance (bright cells clip to white); deliberate, not a bug.
    noise_rgb = field[..., None] * palette[None, None, :] * 1.5
    alpha = max_noise * 0.5 * (1.0 - np.cos(2.0 * np.pi * t))
    return to_uint8((1.0 - alpha) * img + alpha * noise_rgb)


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


# t-independent results memoized per render (cleared at the start of render()).
_MASK_CACHE = {}
_STREAK_CACHE = {}
_BG_CACHE = {}
_FLOW_CACHE = {}


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
    key = (id(img), img.shape, mask_path, round(thr, 4), round(feather, 4))
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
    comp_small = _fill_holes(_largest_component(binar[::scale, ::scale]))
    comp = np.asarray(
        Image.fromarray((comp_small.astype(np.uint8) * 255)).resize(
            (w, h), Image.NEAREST),
        dtype=np.uint8) > 0
    keep = comp.astype(np.uint8) * 255   # solid filled silhouette (holes filled)
    pil = Image.fromarray(keep)
    r = max(1, int(round(feather / 2.0)))
    k = 2 * r + 1
    pil = pil.filter(ImageFilter.MaxFilter(k)).filter(ImageFilter.MinFilter(k))
    pil = pil.filter(ImageFilter.GaussianBlur(feather))
    m = np.asarray(pil, dtype=np.float32) / 255.0
    _MASK_CACHE[key] = m
    return m


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


def hyperspace_streak(imgs, params, t):
    """Stargate 'probability field': fixed bright curve, stars streaking past.

    The painted texture is motion-blurred along a flow toward/away from the
    curve (t-independent, cached); synthetic stars stream along the same flow
    and loop; the detected bright curve is composited back unmoved on top.
    """
    img = imgs[0]
    streak_len = int(params.get("streak_len", 40))
    n_stars = int(params.get("n_stars", 700))
    toward = bool(params.get("toward", True))
    curve_threshold = float(params.get("curve_threshold", 0.5))
    seed = int(params.get("seed", 0))
    speed = float(params.get("speed", 2.0))
    intensity = float(params.get("intensity", 1.6))
    h, w = img.shape[:2]

    key = (id(img), img.shape, streak_len, round(curve_threshold, 4), toward)
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

    stars = _draw_streaking_stars(
        h, w, fx, fy, n_stars, streak_len, seed, t, speed, intensity)
    field = 1.0 - (1.0 - streaked) * (1.0 - stars)   # screen-blend the stars
    return to_uint8(np.maximum(field, curve_rgb))


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


# Effect registry: name -> (required_input_count, frame_function).
EFFECTS = {
    "fold-churn": (1, fold_churn),
    "chromatic-split": (1, chromatic_split),
    "noise-dissolve": (1, noise_dissolve),
    "crossfade-drift": (2, crossfade_drift),
    "hyper-warp": (1, hyper_warp),
    "hyperspace-streak": (1, hyperspace_streak),
    "disintegrate": (1, disintegrate),
    "polytope-overlay": (1, polytope_overlay),
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


def render(srcs, effect, params, duration, fps, out, alpha=False):
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
    _MASK_CACHE.clear()
    _STREAK_CACHE.clear()
    _BG_CACHE.clear()
    _FLOW_CACHE.clear()
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
                    alpha=item.get("alpha", False),
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
