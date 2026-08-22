#!/usr/bin/env python3
"""Emblem exploration lab: 12 variations per empire + 9 pirate clan icons.

Variation index = temperature: v1-v4 are clean canonical takes, the
middle band loosens, v9-v12 go wild (heavier jitter, odder geometry).
Design language is fed by each empire's ship-name lore:

  solarian   reason/archives (Axiom, Cogito, Archimedes) -> sun, geometry,
             instruments of measurement
  crimson    medieval brutality (Bonesaw, Bastille, Cleaver) -> weapons,
             fortresses, wounds
  outerrim   junkyard improvisation (Baling Wire, Chop Shop, Bad Idea) ->
             tools, patches, hazards
  voidborn   metaphysics/entropy (Apeiron, Aporia, Apotheosis) -> void,
             warped geometry, transcendence
  nebula     COMMERCE (Arbitrage, Consortium, Collateral, Caravan) ->
             scales, coins, ledgers, caravans

Pirate clan icons are keyed to their stronghold stars: Algol (demon
star / eclipsing binary), Alhena (brand mark), Barnard 44 (runaway
star), Bellatrix (warrior), GSC-0008 (catalog ghost), Gliese 581 (red
dwarf + planet chain), Sheratan (ram horns), Xamidimura (scorpion),
Zaniah (the corner).

    python3 emblem_lab.py     (writes ../../mesh_bakeoff/emblem_lab.html)
"""
import math
import random
from pathlib import Path

SW = 2.4
_J = {"amp": 0.0, "rng": random.Random(0)}   # temperature jitter state


def _jit(x, y):
    a = _J["amp"]
    if not a:
        return x, y
    return x + _J["rng"].uniform(-a, a), y + _J["rng"].uniform(-a, a)


def _fmt(seq):
    return " ".join("{:.1f},{:.1f}".format(*_jit(x, y)) for x, y in seq)


def P(seq, closed=True, w=SW, dash=""):
    d = f' stroke-dasharray="{dash}"' if dash else ""
    return (f'<polyline points="{_fmt(list(seq) + ([list(seq)[0]] if closed else []))}" '
            f'fill="none" stroke="currentColor" stroke-width="{w}" '
            f'stroke-linejoin="round" stroke-linecap="round"{d}/>')


def ring(r, cx=50, cy=50, w=SW, a0=0.0, a1=360.0, dash="", n=48):
    pts = [(cx + r * math.cos(math.radians(a)),
            cy + r * math.sin(math.radians(a)))
           for a in [a0 + (a1 - a0) * i / n for i in range(n + 1)]]
    return P(pts, closed=abs(a1 - a0) >= 360, w=w, dash=dash)


def dot(cx, cy, r=2.6):
    x, y = _jit(cx, cy)
    return f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{r}" fill="currentColor"/>'


def gon(n, r, cx=50, cy=50, rot=0.0, w=SW):
    return P([(cx + r * math.cos(math.radians(rot + 360 * i / n)),
               cy + r * math.sin(math.radians(rot + 360 * i / n)))
              for i in range(n)], w=w)


def rays(n, r0, r1, cx=50, cy=50, rot=0.0, alt=None, w=SW):
    out = []
    for i in range(n):
        a = math.radians(rot + 360 * i / n)
        rr = r1 if (alt is None or i % 2 == 0) else alt
        out.append(P([(cx + r0 * math.cos(a), cy + r0 * math.sin(a)),
                      (cx + rr * math.cos(a), cy + rr * math.sin(a))],
                     closed=False, w=w))
    return "".join(out)


def spiral(turns, r0, r1, cx=50, cy=50, n=260, w=SW, rot=0.0):
    pts = []
    for i in range(n + 1):
        t = i / n
        th = rot + t * turns * 2 * math.pi
        r = r0 + (r1 - r0) * t
        pts.append((cx + r * math.cos(th), cy + r * math.sin(th)))
    return P(pts, closed=False, w=w)


def star4(cx, cy, s, w=SW * 0.8):
    return P([(cx - s, cy), (cx, cy - s), (cx + s, cy), (cx, cy + s)], w=w)


# ------------------------------------------------------------------ solarian
def sol_v(i):
    if i == 0:
        return ring(14) + ring(45, w=1.1, dash="3 4") + rays(12, 20, 38, alt=29)
    if i == 1:
        return (ring(10) + ring(22, w=1.2) + ring(32, w=1.2) + ring(42, w=1.2)
                + dot(50 + 22, 50, 3) + dot(50 - 16, 50 - 22, 3)
                + dot(50 + 12, 50 + 40, 3))
    if i == 2:
        return (ring(16, cy=58, a0=180, a1=360)
                + P([(12, 58), (88, 58)], closed=False)
                + rays(7, 22, 36, cy=58, rot=187.5 + 12)
                + P([(20, 70), (80, 70)], closed=False, w=1.2, dash="4 4"))
    if i == 3:
        pts = []
        for k in range(16):
            a = math.radians(360 * k / 16)
            r = 40 if k % 2 == 0 else 17
            pts.append((50 + r * math.cos(a), 50 + r * math.sin(a)))
        return P(pts) + ring(8)
    if i == 4:
        return (gon(3, 40, rot=-90) + ring(11, cy=55)
                + rays(8, 15, 22, cy=55))
    if i == 5:
        out, r = [], 42.0
        for _ in range(5):
            out.append(gon(4, r, rot=45))
            r *= 0.62
        return "".join(out) + dot(50, 50, 3)
    if i == 6:
        return (ring(20) + ring(20, cx=62, w=1.4)
                + rays(10, 26, 34, rot=8))
    if i == 7:
        return (gon(3, 34, rot=-90) + rays(9, 38, 46, rot=200)
                + ring(46, a0=195, a1=345, w=1.2, dash="2 3"))
    if i == 8:
        return (gon(6, 42) + gon(5, 30, rot=-90) + gon(4, 19, rot=45)
                + gon(3, 10, rot=-90) + dot(50, 50, 2))
    if i == 9:
        out, x, y, r = [], 50.0, 50.0, 34.0
        for k in range(6):
            a0 = 90 * k
            out.append(ring(r, cx=x, cy=y, a0=a0, a1=a0 + 90, w=SW - 0.2 * k))
            r *= 0.62
        return "".join(out) + dot(50, 50, 2.4)
    if i == 10:
        e = "".join(
            P([(50 + 40 * math.cos(math.radians(a)) * 0.38 * math.cos(t)
                - 40 * math.sin(math.radians(a)) * math.sin(t),
                50 + 40 * 0.38 * math.cos(math.radians(a)) * math.sin(t)
                + 40 * math.sin(math.radians(a)) * math.cos(t))
               for a in range(0, 361, 8)], w=1.3)
            for t in (math.radians(30),))
        return ring(42) + e + P([(50, 8), (50, 92)], closed=False, w=1.1) + \
            P([(8, 50), (92, 50)], closed=False, w=1.1) + ring(5)
    return (rays(14, 16, 44, rot=3) + ring(12)
            + "".join(dot(50 + rr * math.cos(math.radians(aa)),
                          50 + rr * math.sin(math.radians(aa)), 1.8)
                      for rr, aa in ((47, 20), (44, 130), (49, 250))))


# ------------------------------------------------------------------- crimson
def cri_v(i):
    if i == 0:
        return "".join(P([(18, yb), (50, ya), (82, yb)], closed=False,
                         w=SW + (2 - k) * 0.8)
                       for k, (yb, ya) in enumerate([(46, 24), (64, 42),
                                                     (82, 60)])) + \
            P([(50, 12), (57, 19), (50, 26), (43, 19)])
    if i == 1:
        ax = []
        for sx in (-1, 1):
            ax.append(P([(50 - sx * 30, 22), (50 + sx * 26, 78)],
                        closed=False, w=SW + 0.6))
            ax.append(P([(50 - sx * 30, 22), (50 - sx * 14, 16),
                         (50 - sx * 10, 34), (50 - sx * 30, 22)]))
        return "".join(ax)
    if i == 2:
        return (P([(30, 20), (70, 20), (70, 44), (30, 44)])
                + P([(50, 44), (50, 84)], closed=False, w=SW + 1)
                + P([(30, 32), (18, 32)], closed=False)
                + P([(70, 32), (82, 32)], closed=False))
    if i == 3:
        sh = P([(28, 22), (72, 22), (72, 52), (50, 82), (28, 52)])
        return sh + P([(34, 28), (66, 70)], closed=False, w=SW + 0.8) + \
            P([(66, 28), (34, 70)], closed=False, w=SW + 0.8)
    if i == 4:
        ball = gon(10, 15, cx=62, cy=62)
        spikes = rays(8, 15, 24, cx=62, cy=62, rot=10)
        chain = "".join(ring(4, cx=62 - 14 - 9 * k, cy=62 - 22 - 7 * k, w=1.6)
                        for k in range(4))
        return ball + spikes + chain
    if i == 5:
        teeth = []
        n = 14
        for k in range(n + 1):
            a = math.pi * (1 + k / n)
            r = 36 if k % 2 == 0 else 29
            teeth.append((50 + r * math.cos(a), 58 + r * math.sin(a)))
        return P(teeth + [(86, 58), (14, 58)]) + ring(7, cy=52)
    if i == 6:
        bars = "".join(P([(x, 24), (x, 76)], closed=False, w=2.0)
                       for x in range(26, 75, 12)) + \
            "".join(P([(24, y), (76, y)], closed=False, w=2.0)
                    for y in (34, 50, 66))
        tips = "".join(P([(x, 76), (x - 3, 84), (x + 3, 84), (x, 76)], w=1.4)
                       for x in range(26, 75, 12))
        return bars + tips
    if i == 7:
        return (gon(5, 40, rot=-90, w=SW + 0.4) + gon(5, 22, rot=90)
                + dot(50, 50, 3))
    if i == 8:
        return (P([(22, 66), (34, 66), (34, 56), (28, 50), (40, 38),
                   (52, 38), (58, 44), (78, 44), (78, 56), (66, 66),
                   (78, 66), (78, 74), (22, 74)])
                + "".join(dot(30 + 8 * k, 30 - 4 * k, 1.6) for k in range(4)))
    if i == 9:
        w = []
        for k, x in enumerate((30, 50, 70)):
            w.append(P([(x - 8, 18), (x + 4, 46), (x - 4, 46), (x + 8, 82)],
                       closed=False, w=SW + 0.8 - 0.3 * k))
        return "".join(w)
    if i == 10:
        return (P([(20, 30), (34, 16), (48, 30), (62, 16), (76, 30),
                   (70, 44), (26, 44)])
                + P([(26, 44), (22, 84)], closed=False, w=SW + 0.6)
                + P([(70, 44), (74, 84)], closed=False, w=SW + 0.6)
                + P([(22, 84), (74, 84)], closed=False, w=SW + 0.6)
                + gon(4, 7, cx=48, cy=62, rot=45))
    out = []
    rng = random.Random("wound")
    for k in range(3):
        x = 30 + 14 * k
        pts = [(x + rng.uniform(-3, 3) + 8 * t / 4,
                14 + 18 * t + rng.uniform(-2, 2)) for t in range(5)]
        out.append(P(pts, closed=False, w=SW + 1.2 - 0.4 * k))
    return "".join(out)


# ------------------------------------------------------------------ outerrim
def out_v(i):
    if i == 0:
        teeth = []
        n = 8
        for k in range(n):
            a0 = 2 * math.pi * k / n
            for da, r in ((0.0, 40), (0.16, 40), (0.22, 31), (0.40, 31),
                          (0.46, 40)):
                a = a0 + da * 2 * math.pi / n
                teeth.append((50 + r * math.cos(a), 50 + r * math.sin(a)))
        return P(teeth) + gon(6, 16, rot=30) + ring(8)
    if i == 1:
        def wrench(rot):
            c, s = math.cos(math.radians(rot)), math.sin(math.radians(rot))
            def T(x, y):
                return (50 + (x - 50) * c - (y - 50) * s,
                        50 + (x - 50) * s + (y - 50) * c)
            return P([T(*p) for p in
                      [(50, 20), (44, 14), (46, 8), (56, 8), (58, 14),
                       (52, 20), (52, 80), (58, 86), (56, 92), (46, 92),
                       (44, 86), (50, 80)]], w=1.8)
        return wrench(45) + wrench(-45)
    if i == 2:
        return (P([(26, 40), (74, 60), (74, 74), (26, 54)])
                + P([(26, 60), (74, 40), (74, 26), (26, 46)])
                + "".join(dot(32 + 9 * k, 44 + 4 * k, 1.4) for k in range(5)))
    if i == 3:
        return (P([(30, 18), (70, 18), (70, 82), (30, 82)])
                + "".join(P([(30, y), (70, y)], closed=False, w=1.6)
                          for y in (30, 50, 70))
                + P([(30, 18), (26, 14)], closed=False)
                + P([(70, 18), (74, 14)], closed=False))
    if i == 4:
        return ring(40) + "".join(
            P([(50 + 40 * math.cos(math.radians(a)),
                50 + 40 * math.sin(math.radians(a))),
               (50 + 40 * math.cos(math.radians(a + 22)),
                50 + 40 * math.sin(math.radians(a + 22)))], closed=False,
              w=5.5) for a in range(0, 360, 45))
    if i == 5:
        return (gon(6, 13, cx=46, cy=40, rot=12)
                + ring(20, cx=46, cy=40, w=1.6)
                + ring(26, cx=52, cy=52, w=1.3)
                + P([(46, 53), (46, 84)], closed=False, w=3.2)
                + P([(40, 84), (52, 84)], closed=False, w=2.2))
    if i == 6:
        return (P([(50, 12), (50, 46)], closed=False, w=3.0)
                + ring(16, cy=60, a0=-70, a1=180, w=3.0)
                + P([(34, 62), (30, 74)], closed=False, w=3.0)
                + ring(5, cx=50, cy=12, w=1.8))
    if i == 7:
        big, small = [], []
        for k in range(9):
            a0 = 2 * math.pi * k / 9
            if k == 4:
                continue
            for da, r in ((0.0, 26), (0.2, 26), (0.26, 20), (0.44, 20),
                          (0.5, 26)):
                a = a0 + da * 2 * math.pi / 9
                big.append((38 + r * math.cos(a), 44 + r * math.sin(a)))
        for k in range(6):
            a0 = 2 * math.pi * k / 6
            for da, r in ((0.0, 14), (0.2, 14), (0.28, 10), (0.42, 10),
                          (0.5, 14)):
                a = a0 + da * 2 * math.pi / 6
                small.append((72 + r * math.cos(a), 72 + r * math.sin(a)))
        return P(big) + P(small) + ring(6, cx=38, cy=44) + ring(3.4, cx=72, cy=72)
    if i == 8:
        return (P([(30, 30), (70, 30), (74, 84), (26, 84)])
                + P([(38, 30), (38, 22), (58, 22), (58, 30)], closed=False)
                + P([(32, 42), (68, 42)], closed=False, w=1.4)
                + P([(45, 55), (55, 62), (45, 69)], closed=False, w=1.8))
    if i == 9:
        return (gon(3, 42, rot=-90, w=SW + 0.6)
                + P([(50, 36), (50, 58)], closed=False, w=3.4)
                + dot(50, 68, 2.6))
    if i == 10:
        rng = random.Random("rivets")
        plate = P([(22, 26), (78, 22), (80, 78), (24, 80)])
        seam = P([(22, 52), (34, 46), (44, 56), (56, 46), (66, 56), (78, 50)],
                 closed=False, w=1.6)
        riv = "".join(dot(x, y, 1.5) for x, y in
                      [(28 + rng.uniform(-2, 2), 32 + rng.uniform(-2, 2))
                       for _ in range(3)] + [(70, 30), (74, 72), (30, 74)])
        return plate + seam + riv
    rng = random.Random("wirenest")
    out = []
    for _ in range(7):
        cx, cy = rng.uniform(38, 62), rng.uniform(38, 62)
        r = rng.uniform(10, 26)
        a0 = rng.uniform(0, 360)
        out.append(ring(r, cx=cx, cy=cy, a0=a0, a1=a0 + rng.uniform(190, 330),
                        w=1.3))
    return "".join(out) + gon(6, 9, rot=17, w=2.0)


# ------------------------------------------------------------------ voidborn
def voi_v(i):
    if i == 0:
        return spiral(3.1, 2.5, 40.5) + dot(50, 50, 2.4)
    if i == 1:
        return (ring(18, w=SW + 0.6)
                + "".join(ring(30, a0=a, a1=a + 55, w=1.6)
                          for a in (0, 120, 240))
                + "".join(ring(42, a0=a, a1=a + 35, w=1.1)
                          for a in (30, 150, 270)))
    if i == 2:
        return (spiral(1.6, 3, 38, rot=0) + spiral(1.6, 3, 38, rot=math.pi)
                + dot(50, 50, 2.2))
    if i == 3:
        a = ring(26, cx=41, w=1.8)
        b = ring(26, cx=59, w=1.8)
        return a + b + dot(50, 50, 2.6) + \
            "".join(P([(50, 50 + yy), (50, 54 + yy)], closed=False, w=1.2)
                    for yy in (-34, 30))
    if i == 4:
        out = []
        for k, r in enumerate((12, 20, 28, 36, 44)):
            g = 40 + 22 * k
            out.append(ring(r, a0=g, a1=g + 300 - 24 * k, w=SW - 0.3 * k))
        return "".join(out)
    if i == 5:
        return (gon(4, 40, rot=45, w=1.6) + gon(4, 22, rot=45, w=1.6)
                + "".join(P([(50 + 40 * math.cos(math.radians(a)),
                              50 + 40 * math.sin(math.radians(a))),
                             (50 + 22 * math.cos(math.radians(a)),
                              50 + 22 * math.sin(math.radians(a)))],
                            closed=False, w=1.1)
                          for a in (45, 135, 225, 315)))
    if i == 6:
        def branch(x, y, a, l, d, out):
            if d == 0 or l < 4:
                out.append(dot(x, y, 1.4))
                return
            x2, y2 = x + l * math.cos(a), y + l * math.sin(a)
            out.append(P([(x, y), (x2, y2)], closed=False,
                         w=1.0 + 0.5 * d))
            for da in (-0.62, 0.62):
                branch(x2, y2, a + da, l * 0.62, d - 1, out)
        out = []
        for a0 in (90, 210, 330):
            branch(50, 50, math.radians(a0), 20, 3, out)
        return "".join(out)
    if i == 7:
        return (ring(22, cx=42, cy=46, w=1.7) + ring(22, cx=58, cy=54, w=1.7)
                + "".join(P([(30 + 4 * k, 78), (36 + 4 * k, 84)],
                            closed=False, w=1.2) for k in range(9)))
    if i == 8:
        s = 34
        pts = [(50 + s * math.cos(math.radians(-90 + 120 * k)),
                54 + s * math.sin(math.radians(-90 + 120 * k)))
               for k in range(3)]
        out = []
        for k in range(3):
            a, b = pts[k], pts[(k + 1) % 3]
            m = (a[0] + (b[0] - a[0]) * 0.72, a[1] + (b[1] - a[1]) * 0.72)
            out.append(P([a, m], closed=False, w=SW + 0.5))
            out.append(P([m, b], closed=False, w=1.1))
        return "".join(out) + gon(3, 12, cy=54, rot=-90, w=1.4)
    if i == 9:
        rng = random.Random("entropy")
        out = []
        for gx in range(4):
            for gy in range(4):
                x0, y0 = 26 + gx * 10, 20 + gy * 10
                decay = (gx + gy) / 6
                x = x0 + rng.uniform(-12, 12) * decay + 14 * decay
                y = y0 + rng.uniform(-12, 12) * decay + 18 * decay
                out.append(dot(x, y, 2.0 - 0.9 * decay))
        return "".join(out) + P([(22, 16), (22, 56)], closed=False, w=1.2) + \
            P([(22, 16), (58, 16)], closed=False, w=1.2)
    if i == 10:
        out = []
        for k in range(6):
            ry = 26 - 4 * k
            rx = 34 - 5.2 * k
            y = 26 + 10 * k
            out.append(P([(50 + rx * math.cos(math.radians(a)),
                           y + ry * 0.34 * math.sin(math.radians(a)))
                          for a in range(0, 361, 12)], w=1.9 - 0.2 * k))
        return "".join(out) + dot(50, 84, 2.2)
    pts = []
    for t in range(0, 361, 3):
        th = math.radians(t)
        r = 30 + 11 * math.sin(3 * th)
        pts.append((50 + r * math.cos(th), 50 + r * math.sin(th)))
    return P(pts, w=1.8) + gon(3, 8, rot=90, w=1.4)


# -------------------------------------------------------------------- nebula
def neb_v(i):
    if i == 0:
        beam = P([(20, 40), (80, 40)], closed=False, w=2.2)
        return (beam + P([(50, 24), (50, 40)], closed=False, w=2.2)
                + ring(9, cx=26, cy=56, a0=-30, a1=210, w=1.8)
                + ring(9, cx=74, cy=56, a0=-30, a1=210, w=1.8)
                + P([(20, 40), (26, 56)], closed=False, w=1.2)
                + P([(32, 40), (26, 56)], closed=False, w=1.2)
                + P([(68, 40), (74, 56)], closed=False, w=1.2)
                + P([(80, 40), (74, 56)], closed=False, w=1.2)
                + P([(38, 82), (62, 82)], closed=False, w=2.0)
                + P([(50, 40), (50, 76)], closed=False, w=1.2, dash="3 3"))
    if i == 1:
        c = ring(19, cx=50, cy=44) + gon(8, 11, cx=50, cy=44, rot=22.5, w=1.4)
        return c + ring(19, cx=50, cy=44, a0=210, a1=330, w=1.0) + \
            P([(24, 78), (76, 78)], closed=False, w=1.6) + \
            P([(30, 70), (70, 70)], closed=False, w=1.6) + \
            P([(36, 62), (64, 62)], closed=False, w=1.6)
    if i == 2:
        out = []
        for k in range(4):
            y = 30 + k * 13
            out.append(P([(24 + 6 * k, y), (60 + 5 * k, y)], closed=False,
                         w=1.8))
            out.append(dot(66 + 5 * k, y, 1.6))
        return "".join(out) + P([(24, 24), (24, 76)], closed=False, w=2.4)
    if i == 3:
        chain = "".join(dot(24 + 13 * k, 62 - 7 * k + (3 if k % 2 else 0), 2.6)
                        for k in range(5))
        return chain + P([(18, 74), (86, 40)], closed=False, w=1.1,
                         dash="2 4") + star4(78, 26, 5)
    if i == 4:
        coins = "".join(
            P([(34, y), (66, y), (66, y + 7), (34, y + 7)], w=1.6)
            for y in (60, 50, 40))
        return coins + ring(13, cx=50, cy=24, w=1.8) + \
            P([(45, 24), (55, 24)], closed=False, w=1.4) + \
            P([(50, 19), (50, 29)], closed=False, w=1.4)
    if i == 5:
        return (P([(22, 76), (40, 58), (52, 66), (78, 30)], closed=False,
                  w=2.6)
                + P([(66, 30), (78, 30), (78, 42)], closed=False, w=2.6)
                + "".join(P([(x, 80), (x, 84)], closed=False, w=1.6)
                          for x in range(24, 81, 8)))
    if i == 6:
        return (ring(11, cx=38, cy=38, w=2.0) + ring(11, cx=62, cy=38, w=2.0)
                + ring(11, cx=50, cy=58, w=2.0)
                + P([(30, 80), (70, 80)], closed=False, w=1.4, dash="5 3"))
    if i == 7:
        return (ring(12, cx=34, cy=40, w=2.2)
                + "".join(ring(rr, cx=34, cy=40, a0=-38, a1=38, w=1.4)
                          for rr in (20, 27, 34))
                + "".join(dot(34 + rr * math.cos(math.radians(aa)),
                              40 + rr * math.sin(math.radians(aa)), 2.0)
                          for rr, aa in ((20, 12), (27, -20), (34, 26))))
    if i == 8:
        out = []
        rng = random.Random("ledger")
        for col in range(5):
            x = 26 + col * 12
            h = rng.choice((18, 30, 24, 40, 34))
            out.append(P([(x, 78), (x, 78 - h)], closed=False, w=3.4))
        return "".join(out) + P([(20, 78), (84, 78)], closed=False, w=1.6) + \
            P([(24, 44), (48, 30), (82, 22)], closed=False, w=1.2, dash="3 3")
    if i == 9:
        return (spiral(1.9, 4, 34, n=200, w=1.9)
                + "".join(dot(50 + r * math.cos(a), 50 + r * math.sin(a), 1.7)
                          for r, a in ((40, 0.6), (44, 2.4), (38, 4.1),
                                       (46, 5.3))))
    if i == 10:
        out = []
        for k, (x0, top) in enumerate(((30, 34), (50, 22), (70, 40))):
            out.append(P([(x0 - 6, 82), (x0 - 3, top), (x0 + 3, top + 6),
                          (x0 + 6, 82)], closed=False, w=1.9))
            out.append(star4(x0, top - 8, 3.4))
        return "".join(out)
    rng = random.Random("mist")
    out = [star4(50, 50, 4)]
    for a in range(0, 360, 15):
        r0 = rng.uniform(14, 26)
        r1 = r0 + rng.uniform(6, 20)
        th = math.radians(a + rng.uniform(-4, 4))
        out.append(P([(50 + r0 * math.cos(th), 50 + r0 * math.sin(th)),
                      (50 + r1 * math.cos(th), 50 + r1 * math.sin(th))],
                     closed=False, w=1.2))
    return "".join(out)


# ------------------------------------------------------------ pirate clans
def clan_algol():
    return (P([(14, 50), (50, 30), (86, 50), (50, 70)])
            + ring(11, cy=50) + dot(50, 50, 3.4) + dot(76, 30, 2.4)
            + ring(5, cx=76, cy=30, w=1.0))


def clan_alhena():
    return (ring(16, cx=44, cy=40) + P([(44, 32), (44, 48)], closed=False,
                                       w=2.8)
            + P([(36, 40), (52, 40)], closed=False, w=2.8)
            + P([(56, 52), (76, 72), (70, 78), (50, 58)])
            + P([(70, 78), (78, 86)], closed=False, w=1.6))


def clan_barnard():
    s = star4(62, 40, 12, w=2.2)
    return s + "".join(P([(18, y), (44 - k * 4, y)], closed=False, w=1.6)
                       for k, y in enumerate((30, 40, 50)))


def clan_bellatrix():
    dg = []
    for sx in (-1, 1):
        dg.append(P([(50 - sx * 26, 24), (50 + sx * 18, 68)], closed=False,
                    w=2.6))
        dg.append(P([(50 - sx * 30, 34), (50 - sx * 18, 30)], closed=False,
                    w=2.0))
    return "".join(dg) + ring(26, cy=44, a0=200, a1=340, w=1.4) + \
        dot(50, 78, 2.6)


def clan_gsc():
    return (ring(24) + P([(50, 14), (50, 34)], closed=False, w=1.6)
            + P([(50, 66), (50, 86)], closed=False, w=1.6)
            + P([(14, 50), (34, 50)], closed=False, w=1.6)
            + P([(66, 50), (86, 50)], closed=False, w=1.6)
            + "".join(P([(x, 76), (x, 88)], closed=False, w=wd)
                      for x, wd in ((30, 2.8), (35, 1.2), (40, 2.0),
                                    (60, 1.2), (65, 2.8), (70, 1.6))))


def clan_gliese():
    return (ring(9, cx=30, cy=64) + dot(30, 64, 3)
            + P([(20, 74), (82, 22)], closed=False, w=1.1, dash="2 3")
            + dot(48, 50, 2.6) + dot(62, 38, 2.2) + dot(74, 28, 1.8))


def clan_sheratan():
    return (spiral(1.4, 3, 16, cx=32, cy=44, rot=math.pi, w=2.2)
            + spiral(1.4, 3, 16, cx=68, cy=44, rot=0, w=2.2)
            + ring(12, cx=50, cy=58, a0=180, a1=360, w=2.2)
            + P([(38, 58), (38, 72)], closed=False, w=2.2)
            + P([(62, 58), (62, 72)], closed=False, w=2.2))


def clan_xamidimura():
    seg = []
    pts = [(24, 70), (36, 74), (48, 72), (58, 64), (64, 52), (64, 40),
           (58, 30)]
    seg.append(P(pts, closed=False, w=2.8))
    for x, y in pts[1:-1]:
        seg.append(dot(x, y, 1.8))
    seg.append(P([(58, 30), (48, 26), (52, 38)], closed=False, w=2.2))
    return "".join(seg)


def clan_zaniah():
    out = []
    for k, s in enumerate((40, 28, 16)):
        out.append(P([(50 - s, 30 + k * 6), (50 - s, 30 + k * 6 + s),
                      (50 - s + s, 30 + k * 6 + s)], closed=False,
                    w=SW - 0.3 * k))
    return "".join(out) + dot(58, 42, 2.4)


EMPIRES = {"solarian": sol_v, "crimson": cri_v, "outerrim": out_v,
           "voidborn": voi_v, "nebula": neb_v}
CLANS = {"Algol": clan_algol, "Alhena": clan_alhena,
         "Barnard 44": clan_barnard, "Bellatrix": clan_bellatrix,
         "GSC-0008": clan_gsc, "Gliese 581": clan_gliese,
         "Sheratan": clan_sheratan, "Xamidimura": clan_xamidimura,
         "Zaniah": clan_zaniah}

# temperature -> jitter amplitude per variant index
AMPS = [0, 0, 0, 0, 0.4, 0.5, 0.6, 0.8, 1.0, 1.3, 1.7, 2.2]


def render(fn, idx=None, seed=""):
    _J["amp"] = AMPS[idx] if idx is not None else 0.0
    _J["rng"] = random.Random(f"{seed}/{idx}")
    return fn(idx) if idx is not None else fn()


def main() -> int:
    out = Path(__file__).resolve().parent.parent.parent / "mesh_bakeoff" \
        / "emblem_lab.html"
    secs = []
    for name, fn in EMPIRES.items():
        cells = "".join(
            f'<div class="cell"><svg viewBox="0 0 100 100">'
            f'{render(fn, i, name)}</svg><span>v{i + 1} · t{AMPS[i]}</span></div>'
            for i in range(12))
        secs.append(f'<h2>{name.upper()}</h2><div class="row">{cells}</div>')
    cells = "".join(
        f'<div class="cell"><svg viewBox="0 0 100 100">{render(fn)}</svg>'
        f'<span>{nm.upper()}</span></div>' for nm, fn in CLANS.items())
    secs.append(f'<h2>PIRATE CLANS · STRONGHOLDS</h2><div class="row">{cells}</div>')
    out.write_text("""<!doctype html><meta charset="utf-8">
<title>emblem lab</title>
<style>
  body { background:#123d75; margin:0; padding:28px 40px; color:#eaf2ff;
         font:13px 'Courier New', monospace; letter-spacing:.14em }
  h2 { font-size:14px; margin:26px 0 10px; border-bottom:1px solid #eaf2ff55;
       padding-bottom:6px }
  .row { display:flex; gap:18px; flex-wrap:wrap }
  .cell { text-align:center }
  .cell svg { width:132px; height:132px; color:#eaf2ff; display:block;
              border:1.2px solid #eaf2ff; margin-bottom:6px }
  .cell span { font-size:10px; color:#bcd0e8 }
</style>
""" + "".join(secs))
    print(f"{out}\nserve:  http://localhost:8478/emblem_lab.html")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
