#!/usr/bin/env python3
"""Procedural deck-plan linework for the blueprint sheets.

Everything is derived, deterministic, and data-driven — no RNG:
  bridge     at the surveyed window's station along the hull
             (window_t from compute_scale; default 0.72 when unsurveyed)
  corridor   twin lines along the hull's column-wise midline
  inner hull dashed offset of the outline (structural wall)
  engines    bulkhead + thruster arcs in the stern lobes (stern = left,
             matching the footprint bow-right convention)
  tanks      near-circular arcs on the outline completed as dashed
             full circles (Kasa least-squares fits over boundary windows)
  cargo hold area fraction from the ship's cargo_capacity (log-normalized
             over the fleet's 10..3000 range), hatched, aft of midships
  cabins     partition walls off the corridor, spaced by an id-hash

Needs skimage (run make_blueprints.py under ~/sf3d-venv); without it the
sheet silently renders exterior-only.

Coordinates are footprint viewBox units; the caller embeds the fragment
inside the hull transform and passes its scale so strokes/text render at
constant screen size.
"""
import hashlib
import re

import numpy as np

CARGO_LO, CARGO_HI = 10.0, 3000.0


def _h(seed: str, mod: int) -> int:
    return int(hashlib.sha1(seed.encode()).hexdigest(), 16) % mod


def parse_subpaths(d: str):
    subs = []
    for chunk in re.findall(r"M[^MZ]*Z?", d):
        pts = np.array(re.findall(r"(-?[\d.]+)[ ,](-?[\d.]+)", chunk),
                       dtype=np.float32)
        if len(pts) >= 3:
            subs.append(pts)
    return subs


def rasterize(subs, w, h):
    """Even-odd fill = XOR of per-subpath fills."""
    from PIL import Image, ImageDraw
    mask = np.zeros((int(h), int(w)), bool)
    for pts in subs:
        im = Image.new("1", (int(w), int(h)))
        ImageDraw.Draw(im).polygon([tuple(p) for p in pts], fill=1)
        mask ^= np.array(im, bool)
    return mask


def _poly(pts, cls, closed=True):
    d = "M" + "L".join(f"{x:.1f} {y:.1f}" for x, y in pts) + ("Z" if closed else "")
    return f'<path d="{d}" class="{cls}"/>'


def _contour_paths(mask, cls, tol=1.5):
    from skimage import measure
    out = []
    for c in measure.find_contours(mask.astype(float), 0.5):
        if len(c) < 40:
            continue
        c = measure.approximate_polygon(c, tolerance=tol)
        out.append(_poly(c[:, ::-1], cls))
    return out


def _kasa(pts):
    """Least-squares circle fit -> (cx, cy, r, rms)."""
    x, y = pts[:, 0], pts[:, 1]
    A = np.column_stack([x, y, np.ones(len(x))])
    b = x * x + y * y
    try:
        (a1, a2, a3), *_ = np.linalg.lstsq(A, b, rcond=None)
    except np.linalg.LinAlgError:
        return None
    cx, cy = a1 / 2, a2 / 2
    r = np.sqrt(a3 + cx * cx + cy * cy)
    rms = np.sqrt(np.mean((np.hypot(x - cx, y - cy) - r) ** 2))
    return cx, cy, r, rms


def find_tanks(mask, w):
    """Complete near-circular outline arcs as full circles."""
    from skimage import measure
    circles = []
    for c in measure.find_contours(mask.astype(float), 0.5):
        if len(c) < 60:
            continue
        pts = c[:: max(1, len(c) // 400), ::-1]      # (x, y), decimated
        n = len(pts)
        # several window sizes: a tank sphere's exposed arc can be much
        # shorter than 1/14 of the perimeter (capacity's six side spheres)
        for win in sorted({max(10, n // 30), max(12, n // 20), max(14, n // 14)}):
            for i in range(0, n - win, max(3, win // 3)):
                seg = pts[i:i + win]
                fit = _kasa(seg)
                if fit is None:
                    continue
                cx, cy, r, rms = fit
                if not (0.025 * w < r < 0.30 * w) or rms > 0.036 * r:
                    continue
                # a tank bulge curves AROUND hull interior; a concave
                # fillet's fitted center lands outside the hull — reject
                iy, ix = int(round(cy)), int(round(cx))
                if not (0 <= iy < mask.shape[0] and 0 <= ix < mask.shape[1]
                        and mask[iy, ix]):
                    continue
                ang = np.unwrap(np.arctan2(seg[:, 1] - cy, seg[:, 0] - cx))
                if abs(ang[-1] - ang[0]) < np.deg2rad(75):
                    continue
                for c2 in circles:                    # merge duplicates
                    if np.hypot(c2[0] - cx, c2[1] - cy) < 0.7 * max(r, c2[2]):
                        break
                else:
                    circles.append((cx, cy, r))
    return circles


def _core_band(tops, bots, inset=2.0, min_h=6.0):
    """Vertical band of a 'mostly rectangular' bay inside per-column
    interval samples: near-intersection percentiles, robust to a few
    tapered columns. Returns (ylo, yhi) or None."""
    if len(tops) < 5:
        return None
    ylo = float(np.percentile(tops, 85)) + inset
    yhi = float(np.percentile(bots, 15)) - inset
    if yhi - ylo < min_h:
        return None
    return ylo, yhi


def _hold_rect(cid, x, y, w, h, step):
    """Rounded-rect cargo bay outline + 45-degree hatch."""
    r = min(0.35 * h, 0.25 * w, 18.0)
    rect = (f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" '
            f'height="{h:.1f}" rx="{r:.1f}"')
    hatch = "".join(
        f'<line x1="{hx - h:.0f}" y1="{y + h:.0f}" x2="{hx:.0f}" y2="{y:.0f}"/>'
        for hx in range(int(x), int(x + w + h) + 1, step))
    return (rect + ' class="deck-ln"/>'
            f'<clipPath id="{cid}">{rect}/></clipPath>'
            f'<g class="deck-ht" clip-path="url(#{cid})">{hatch}</g>')


def _col_runs(inner, xa, xb_):
    """Largest interior run per column over [xa, xb_): (tops, bots)."""
    from scipy.ndimage import label
    tops, bots = [], []
    for x in range(int(xa), int(xb_)):
        col = inner[:, x]
        if not col.any():
            continue
        runs, n = label(col)
        best = max(range(1, n + 1), key=lambda k: (runs == k).sum())
        ys = np.nonzero(runs == best)[0]
        tops.append(ys[0])
        bots.append(ys[-1])
    return np.array(tops, float), np.array(bots, float)


def deck_plan(ship_id, d, vb_w, vb_h, cargo_capacity, window_t, s,
              line_color="#eaf2ff", ext_pilot=False, bridge_pos=None):
    try:
        from scipy.ndimage import binary_erosion, uniform_filter1d, label
    except ImportError:
        return ""
    try:
        import skimage  # noqa: F401
    except ImportError:
        return ""

    subs = parse_subpaths(d)
    if not subs:
        return ""
    W, H = int(vb_w), int(vb_h)
    mask = rasterize(subs, W, H)
    wt = max(3, int(0.011 * W))
    inner = binary_erosion(mask, iterations=wt)
    if inner.sum() < 400:
        return ""

    frags = list(_contour_paths(inner, "in-hull"))

    # column profile of the main hull body ---------------------------------
    # Runs are chosen for CONTINUITY, seeded at the tallest column and
    # scanned outward both ways: picking the largest run per column
    # independently made the centerline flip between stern lobes (the
    # user's "weird zigzag").
    cands = [None] * W
    for x in range(W):
        col = inner[:, x]
        if not col.any():
            continue
        runs, n = label(col)
        cc = []
        for i in range(1, n + 1):
            ys = np.nonzero(runs == i)[0]
            cc.append(((ys[0] + ys[-1]) / 2, float(ys[-1] - ys[0])))
        cands[x] = cc
    cy = np.full(W, np.nan)
    ch = np.zeros(W)
    have = [x for x in range(W) if cands[x]]
    if len(have) < 30:
        return "".join(frags)
    xseed = max(have, key=lambda x: max(c[1] for c in cands[x]))
    for sweep in (range(xseed, W), range(xseed, -1, -1)):
        prev = None
        for x in sweep:
            if not cands[x]:
                continue
            c = (max(cands[x], key=lambda c: c[1]) if prev is None
                 else min(cands[x], key=lambda c: abs(c[0] - prev)))
            cy[x], ch[x] = c
            prev = c[0]
    xs = np.nonzero(~np.isnan(cy))[0]
    x0, x1 = int(xs[0]), int(xs[-1])
    span = x1 - x0
    cy[np.isnan(cy)] = np.interp(np.nonzero(np.isnan(cy))[0], xs, cy[xs])
    cy = uniform_filter1d(cy, size=max(9, span // 20))

    hw = float(np.clip(0.011 * W, 2.5, 8.0))    # corridor drawn after cargo

    # bridge/cockpit station -----------------------------------------------
    # a curated placement (bridge_positions.json, from the interactive
    # placer: viewBox fractions, bow right) wins over the surveyed window
    # station; the hero click fraction alone is direction-ambiguous
    # (heroes come bow-left AND bow-right) so it is only a fallback.
    kind = (bridge_pos or {}).get("kind", "bridge")
    bl = 0.036 * span
    if bridge_pos and bridge_pos.get("tx") is not None:
        xb = float(bridge_pos["tx"]) * W
    else:
        t = window_t if window_t is not None else 0.72
        xb = x0 + t * span
    xb = float(np.clip(xb, x0 + bl + 2, x1 - bl - 2))
    # always on the bow-stern axis: snap to the interior centerline at the
    # chosen station (placer drags are horizontal-only by design)
    ybc = float(cy[int(xb)])
    bw = float(min(3.2 * hw, 0.42 * max(ch[int(xb)], 4 * hw)))
    if ext_pilot:
        # prayer-class junkers: no bridge — the pilot is strapped to the
        # bow face. Mark the external seat instead; corridor and cabins
        # are suppressed below (there is no walkable interior).
        xp = x1 - 2
        yp = float(cy[xp])
        rp = float(min(1.6 * hw, 0.35 * max(ch[xp], 3 * hw)))
        frags.append(f'<circle cx="{xp:.1f}" cy="{yp:.1f}" r="{rp:.1f}" '
                     f'class="deck-rm"/>')
        frags.append(f'<text x="{xp - rp:.1f}" y="{yp - rp - 6 / s:.1f}" '
                     f'text-anchor="end" class="deck-lb">PILOT</text>')
        xb, bl = float(x1 - 1), 0.0             # hold logic: bow is "bridge"
    else:
        if kind == "cockpit":                   # 1-2 seat canopy: rounder,
            bl = min(bl, 1.4 * bw)              # stubbier than a bridge room
        rx = (0.9 if kind == "cockpit" else 0.4) * bw
        frags.append(f'<rect x="{xb - bl:.1f}" y="{ybc - bw:.1f}" width="{2 * bl:.1f}" '
                     f'height="{2 * bw:.1f}" rx="{rx:.1f}" class="deck-rm"/>')
        xcon = xb + bl * 0.45                   # console arc, bow side
        frags.append(f'<path d="M{xcon:.1f} {ybc - 0.55 * bw:.1f} '
                     f'A{0.7 * bw:.1f} {0.7 * bw:.1f} 0 0 1 {xcon:.1f} {ybc + 0.55 * bw:.1f}" '
                     f'class="deck-ln"/>')
        lb_txt = "COCKPIT" if kind == "cockpit" else "BRIDGE"
        frags.append(f'<text x="{xb:.1f}" y="{ybc - bw - 6 / s:.1f}" '
                     f'text-anchor="middle" class="deck-lb">{lb_txt}</text>')

    # engineering: stern bulkhead + thruster arcs --------------------------
    xe = x0 + 0.13 * span
    col = inner[:, int(xe)]
    runs, n = label(col)
    for i in range(1, n + 1):
        ys = np.nonzero(runs == i)[0]
        if len(ys) < 3:
            continue
        frags.append(f'<line x1="{xe:.1f}" y1="{ys[0]}" x2="{xe:.1f}" '
                     f'y2="{ys[-1]}" class="deck-ln"/>')
        yc = (ys[0] + ys[-1]) / 2
        rr = min((ys[-1] - ys[0]) / 2, 0.06 * span)   # pods, not hull-wide
        for k in (0.55, 0.8):
            frags.append(f'<path d="M{xe - 4:.1f} {yc - rr * k:.1f} '
                         f'A{rr * k:.1f} {rr * k:.1f} 0 0 0 '
                         f'{xe - 4:.1f} {yc + rr * k:.1f}" class="deck-ln"/>')
    frags.append(f'<text x="{x0 + 0.065 * span:.1f}" y="{cy[int(x0 + 0.065 * span)] - 8 / s:.1f}" '
                 f'text-anchor="middle" class="deck-lb">ENG</text>')

    # cargo hold sized by capacity: a mostly-rectangular bay confined to
    # the MAIN BODY run (never spanning wings or engine nacelles -- the
    # per-column continuity run cy/ch is exactly the central core)
    xc0 = xc1 = None
    hold = None
    if cargo_capacity and cargo_capacity > 0:
        f = np.clip((np.log(cargo_capacity) - np.log(CARGO_LO))
                    / (np.log(CARGO_HI) - np.log(CARGO_LO)), 0, 1)
        target = (0.10 + 0.30 * f) * float(ch[x0:x1 + 1].sum())
        xc0 = xe + 2
        if xb - bl < xc0 + 0.1 * span:          # bridge sits aft: hold fwd
            xc0 = xb + bl + 2
        acc, xc1 = 0.0, xc0
        while xc1 < x1 - 0.05 * span and acc < target:
            acc += ch[int(xc1)]
            xc1 += 1
        if xb - bl > xc0:                        # never swallow the bridge
            xc1 = min(xc1, xb - bl - 2)
        if xc1 - xc0 > 0.06 * span:
            sel = ch[int(xc0):int(xc1)] > 0
            band = _core_band((cy - ch / 2)[int(xc0):int(xc1)][sel],
                              (cy + ch / 2)[int(xc0):int(xc1)][sel],
                              min_h=3 * hw)
            if band:
                hold = (xc0, xc1)
                ylo, yhi = band
                step = max(9, int(0.016 * W))
                frags.append(_hold_rect(f"cargo_{ship_id}", xc0, ylo,
                                        xc1 - xc0, yhi - ylo, step))
                frags.append(f'<text x="{(xc0 + xc1) / 2:.1f}" '
                             f'y="{(ylo + yhi) / 2 + 3.5 / s:.1f}" '
                             f'text-anchor="middle" class="deck-lb">HOLD</text>')

    # corridor: runs bow-ward of the engine bulkhead, skips the hold -------
    allowed = np.zeros(W, bool)
    allowed[int(xe) + 2:x1 - max(2, int(0.02 * span))] = True
    if hold:
        allowed[int(hold[0]):int(hold[1])] = False
    if ext_pilot:
        allowed[:] = False                       # no walkable interior
    ok = ch > 4 * hw
    seg = []
    for x in range(x0, x1 + 1):
        good = ok[x] and allowed[x]
        if good:
            seg.append(x)
        if (not good or x == x1) and len(seg) > 0.05 * span:
            for off in (-hw, hw):
                frags.append(_poly([(x_, cy[x_] + off) for x_ in seg[::3]],
                                   "deck-ln", closed=False))
            seg = []
        elif not good:
            seg = []

    # cabins: partition walls off the corridor -----------------------------
    lo = max(xe + 2, (xc1 or xe) + 2)
    hi = xb - bl - 2 if xb > lo else x1 - 0.05 * span
    if hi - lo > 0.12 * span and not ext_pilot and kind != "cockpit":
        dw = max(0.045 * span, 3 * hw)
        i, xw = 0, lo + dw
        while xw < hi:
            jitter = (_h(f"{ship_id}:{i}", 1000) / 1000 - 0.5) * 0.4 * dw
            xj = xw + jitter
            colx = inner[:, int(xj)]
            runs, n = label(colx)
            if n:
                best = max(range(1, n + 1), key=lambda k: (runs == k).sum())
                ys = np.nonzero(runs == best)[0]
                side = _h(f"{ship_id}~{i}", 2)
                ya = cy[int(xj)] + (hw if side else -hw)
                yb_ = ys[-1] if side else ys[0]
                if abs(yb_ - ya) > 2 * hw:
                    frags.append(f'<line x1="{xj:.1f}" y1="{ya:.1f}" '
                                 f'x2="{xj:.1f}" y2="{yb_:.1f}" class="deck-ln"/>')
            i, xw = i + 1, xw + dw

    # tanks: completed circles ---------------------------------------------
    for cx_, cy_, r in find_tanks(mask, W):
        frags.append(f'<circle cx="{cx_:.1f}" cy="{cy_:.1f}" r="{r:.1f}" class="deck-ck"/>')
        frags.append(f'<line x1="{cx_ - 5:.1f}" y1="{cy_:.1f}" x2="{cx_ + 5:.1f}" '
                     f'y2="{cy_:.1f}" class="deck-ln"/>')
        frags.append(f'<line x1="{cx_:.1f}" y1="{cy_ - 5:.1f}" x2="{cx_:.1f}" '
                     f'y2="{cy_ + 5:.1f}" class="deck-ln"/>')

    return (f'<g class="deck" stroke="{line_color}" opacity="0.62">'
            f'{_style(s)}{"".join(frags)}</g>'), {"hold": hold}


def _style(s: float) -> str:
    return (f'<style>'
            f'.in-hull{{fill:none;stroke-width:{1.0 / s:.2f};stroke-dasharray:{7 / s:.1f} {4 / s:.1f}}}'
            f'.deck-ln{{fill:none;stroke-width:{0.9 / s:.2f}}}'
            f'.deck-rm{{fill:none;stroke-width:{1.3 / s:.2f}}}'
            f'.deck-ck{{fill:none;stroke-width:{1.0 / s:.2f};stroke-dasharray:{5 / s:.1f} {4 / s:.1f}}}'
            f'.deck-ht line{{stroke-width:{0.6 / s:.2f};stroke-opacity:.45}}'
            f'.deck-lb{{font-size:{10.5 / s:.1f}px;letter-spacing:.18em}}'
            f'</style>')


def side_overlay(side_d, sw, sh, hold, deck_step, s, ship_id,
                 line_color="#eaf2ff", front_core=False):
    """Interior linework for an ELEVATION view: the dashed inner-hull
    offset (structural wall, matching the plan's), and for the side view
    the cargo hold band at the same stations as the plan's hold (floor to
    ceiling, hatched) plus deck lines every ~3 m fore of it with
    staircases. Coordinates are the view's own corner-origin units; the
    caller embeds this in the view's transform. `hold` is the (x0, x1)
    interval in those units or None; the front elevation passes
    hold=None, deck_step=None and gets just the inner hull."""
    try:
        from scipy.ndimage import binary_erosion
    except ImportError:
        return ""
    try:
        import skimage  # noqa: F401
    except ImportError:
        return ""
    subs = parse_subpaths(side_d)
    if not subs:
        return ""
    W, H = int(np.ceil(sw)), int(np.ceil(sh))
    if W < 40 or H < 12:
        return ""
    mask = rasterize(subs, W, H)
    # same wall thickness as the plan (its viewbox is always ~1020 units,
    # so 0.011*W there is ~11) for a consistent read across the views
    inner = binary_erosion(mask, iterations=11)
    if inner.sum() < 200:
        return ""

    frags = list(_contour_paths(inner, "in-hull"))
    step = max(9, int(0.016 * W))
    hx1 = None
    if hold:
        hx0, hx1 = max(0, int(hold[0])), min(W, int(hold[1]))
        if hx1 - hx0 > 8:
            # bay confined to the hull's core: the per-column main run's
            # near-intersection band keeps it out of masts / dishes /
            # ventral gear that the full profile would sweep in
            band = _core_band(*_col_runs(inner, hx0, hx1), min_h=6.0)
            if band:
                ylo, yhi = band
                frags.append(_hold_rect(f"cargo_side_{ship_id}", hx0, ylo,
                                        hx1 - hx0, yhi - ylo, step))
                frags.append(
                    f'<text x="{(hx0 + hx1) / 2:.1f}" '
                    f'y="{(ylo + yhi) / 2 + 3.5 / s:.1f}" '
                    f'text-anchor="middle" class="deck-lb">HOLD</text>')

    if front_core:
        # front elevation: the hold in cross-section -- the contiguous
        # central column region around the tallest station, cut off where
        # the profile drops toward wings/nacelles
        from scipy.ndimage import uniform_filter1d
        tops = np.full(W, np.nan)
        bots = np.full(W, np.nan)
        for x in range(W):
            t, b = _col_runs(inner, x, x + 1)
            if len(t):
                tops[x], bots[x] = t[0], b[0]
        h_x = np.where(np.isnan(tops), 0.0, bots - tops)
        hs = uniform_filter1d(h_x, size=max(5, W // 30))
        xpk = int(np.argmax(hs))
        thr = 0.55 * hs[xpk]
        a = xpk
        while a > 0 and hs[a - 1] >= thr:
            a -= 1
        b = xpk
        while b < W - 1 and hs[b + 1] >= thr:
            b += 1
        if b - a > 10:
            sel = h_x[a:b] > 0
            band = _core_band(tops[a:b][sel], bots[a:b][sel], min_h=6.0)
            if band:
                ylo, yhi = band
                frags.append(_hold_rect(f"cargo_front_{ship_id}", a + 2, ylo,
                                        b - a - 4, yhi - ylo, step))

    if deck_step and deck_step > 6:
        fore = inner.copy()
        fore[:, :int(hx1 if hx1 is not None else 0.15 * W)] = False
        if fore.sum() > 100:
            cps = _contour_paths(fore, "x")
            cid = f"decks_{ship_id}"
            clip = "".join(p.replace('class="x"', "") for p in cps)
            frags.append(f'<clipPath id="{cid}">{clip}</clipPath>')
            decks = list(np.arange(H - deck_step, 0, -deck_step))
            # double lines: the deck slab has thickness (~2.5px on screen)
            gap = 2.5 / s
            lines = "".join(
                f'<line x1="0" y1="{y:.1f}" x2="{W}" y2="{y:.1f}" '
                f'class="deck-ln"/>'
                f'<line x1="0" y1="{y - gap:.1f}" x2="{W}" y2="{y - gap:.1f}" '
                f'class="deck-ln"/>' for y in decks)
            frags.append(f'<g clip-path="url(#{cid})">{lines}</g>')

            # staircases: step-profiles between adjacent decks, id-hash
            # placed among the columns whose interior spans both levels
            # (never poking through the hull) — one set per deck for every
            # 30 m of non-hold span (deck_step units = 3 m)
            levels = [float(H)] + [float(y) for y in decks]
            n_steps = 4
            for i in range(len(levels) - 1):
                y_lo, y_hi = levels[i], levels[i + 1]
                run_w = 1.1 * (y_lo - y_hi)
                ok = fore[int(y_hi):int(min(y_lo, H - 1)) + 1, :].all(axis=0)
                starts = [x for x in range(W - int(run_w) - 2)
                          if ok[x:x + int(run_w) + 2].all()]
                if not starts:
                    continue
                span_m = (starts[-1] - starts[0] + run_w) * 3.0 / deck_step
                n_stairs = max(1, int(span_m // 30))
                tw, rh = run_w / n_steps, (y_lo - y_hi) / n_steps
                for k in range(n_stairs):
                    lo_i = k * len(starts) // n_stairs
                    hi_i = (k + 1) * len(starts) // n_stairs
                    bucket = starts[lo_i:hi_i]
                    if not bucket:
                        continue
                    sx = bucket[_h(f"{ship_id}/stair{i}.{k}", len(bucket))]
                    dpath = f"M{sx:.1f} {y_lo:.1f}" + "".join(
                        f"h{tw:.1f}v{-rh:.1f}" for _ in range(n_steps))
                    frags.append(f'<path d="{dpath}" class="deck-ln"/>')

    if not frags:
        return ""
    return (f'<g class="deck" stroke="{line_color}" opacity="0.62">'
            f'{_style(s)}{"".join(frags)}</g>')
