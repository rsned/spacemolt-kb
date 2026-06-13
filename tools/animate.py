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
from PIL import Image, ImageFilter


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
