#!/usr/bin/env python3
"""Turn cockpit-window measurements into per-ship meter dimensions.

The anchor convention (user's): a 2 m pilot implies ~1 m window panes, so
one measured window height in hero pixels gives a px-per-meter rate for
that hero, and the hull's pixel span gives L.O.A. in meters. `scale`/tier
in the game data is a complexity band, NOT a size law — measured figures
stand as-is (a 40 m tug and a 110 m hauler can share a class), and
unmeasured ships inherit the median of measured ships in their
(scale, role-group), falling back to the per-scale geometric mean, then a
log-linear fit across scale classes.

Inputs (this directory unless noted):
  window_px.json      hand clicks from measure_windows.html (art-stem keyed)
  hull_spans.json     cached hero pixel spans (regenerated from the
                      chromakeys drop via data/mesh_bakeoff/chromakeys
                      when a measured stem is missing from the cache)
  axis_overrides.json optional {stem: factor} foreshortening overrides
  ../../mesh_bakeoff/ship_id_map.json   art stem -> KB ship id
  ../hy3d-svg/<id>.svg                  data-aspect -> beam
  ../../../spacemolt-knowledge.db       ships.scale/class/name

Output: ship_scale_est.json  {ship_id: {loa_m, beam_m, source, ...}}

    python3 compute_scale.py
"""
import json
import math
import re
import sqlite3
from pathlib import Path

HERE = Path(__file__).resolve().parent
FOOT = HERE.parent
REPO = FOOT.parent.parent
HEROES = REPO / "data" / "mesh_bakeoff" / "chromakeys"
KB = REPO / "spacemolt-knowledge.db"

WINDOW_M = 1.0        # one pane == 1 m (2 m pilot convention)
AXIS_FACTOR = 0.87    # default hull-axis foreshortening at hero angles
                      # (~30 deg off broadside, ~15 deg elevation)
OUTLIER_X = 3.0       # flag measured ships this far off their group median

# ships.class keyword -> role group (first match wins, checked lowercase)
ROLE_RULES = [
    ("combat", ["fighter", "frigate", "destroyer", "corvette", "dread",
                "gunship", "assault", "interceptor", "raider", "bomber",
                "cruiser", "battleship", "carrier", "warship", "attack"]),
    ("hauler", ["freighter", "hauler", "tanker", "barge", "transport",
                "cargo", "courier", "logistics", "trader", "merchant"]),
    ("industrial", ["refinery", "miner", "mining", "smelter", "foundry",
                    "rig", "harvester", "extractor", "excavat", "salvage",
                    "crusher", "drill", "processor", "collector"]),
    ("support", ["repair", "medical", "tender", "science", "survey",
                 "explorer", "scout", "recon", "research", "utility"]),
]


def role_group(cls: str) -> str:
    c = (cls or "").lower()
    for group, kws in ROLE_RULES:
        if any(k in c for k in kws):
            return group
    return "other"


def hull_axis(stem: str):
    """Principal axis of the hero silhouette: (span_px, project_fn).

    project_fn maps an image-space (x, y) to a 0..1 fraction along the
    hull axis measured from the image-left end — used to place the
    bridge on the deck plan at the surveyed window's station.
    """
    import numpy as np
    from PIL import Image
    rgb = np.asarray(Image.open(HEROES / f"{stem}.png").convert("RGB"),
                     np.float32)
    key = np.array([255, 0, 255], np.float32)
    fg = np.sqrt(((rgb - key) ** 2).sum(2)) > 120
    ys, xs = np.nonzero(fg)
    pts = np.column_stack([xs, ys]).astype(np.float32)
    mean = pts.mean(0)
    _, _, vt = np.linalg.svd(pts - mean, full_matrices=False)
    ax = vt[0] if vt[0][0] >= 0 else -vt[0]     # left-to-right convention
    proj = (pts - mean) @ ax
    lo, hi = float(proj.min()), float(proj.max())

    def frac(x, y):
        t = ((np.array([x, y], np.float32) - mean) @ ax - lo) / (hi - lo)
        return float(np.clip(t, 0.0, 1.0))
    return hi - lo, frac


def geomean(xs):
    return math.exp(sum(math.log(x) for x in xs) / len(xs))


def median(xs):
    xs = sorted(xs)
    n = len(xs)
    return xs[n // 2] if n % 2 else (xs[n // 2 - 1] + xs[n // 2]) / 2


def load_aspect(ship_id: str):
    p = FOOT / "hy3d-svg" / f"{ship_id}.svg"
    if not p.exists():
        return None
    m = re.search(r'data-aspect="([\d.]+)"', p.read_text())
    return float(m.group(1)) if m else None


def main() -> int:
    window = json.loads((HERE / "window_px.json").read_text())
    # curated entries survive tool re-exports (the tool emits raw browser
    # localStorage, which would resurrect superseded clicks — e.g. prayer's
    # pilot reference); overrides always win over the raw export
    if (HERE / "window_px_overrides.json").exists():
        window.update(json.loads(
            (HERE / "window_px_overrides.json").read_text()))
    id_map = json.loads(
        (REPO / "data/mesh_bakeoff/ship_id_map.json").read_text())["mapping"]
    overrides = {}
    if (HERE / "axis_overrides.json").exists():
        overrides = json.loads((HERE / "axis_overrides.json").read_text())

    spans_path = HERE / "hull_spans.json"
    spans = json.loads(spans_path.read_text()) if spans_path.exists() else {}
    # bridge stations: 0..1 fraction along the hull axis of each window
    # click (from image-left); cached so regens survive without the heroes
    bt_path = HERE / "bridge_t.json"
    bridge_t = json.loads(bt_path.read_text()) if bt_path.exists() else {}

    db = sqlite3.connect(KB)
    ships = {r[0]: {"name": r[1], "cls": r[2] or "", "scale": r[3]}
             for r in db.execute("select id,name,class,scale from ships")}

    # -- measured ships --------------------------------------------------
    measured = {}      # ship_id -> record
    for stem, w in window.items():
        if w.get("flag") == "none" or not w.get("h"):
            continue
        m = id_map.get(stem)
        if not m or m["id"] not in ships:
            continue
        if stem not in spans or (stem not in bridge_t
                                 and (HEROES / f"{stem}.png").exists()):
            span, frac = hull_axis(stem)
            spans[stem] = round(span, 1)
            if "x" in w:
                bridge_t[stem] = round(frac(w["x"], (w["y0"] + w["y1"]) / 2), 3)
        axis = overrides.get(stem, AXIS_FACTOR)
        # ref_m: meters of real height the clicked feature represents.
        # Default is the 1 m window pane; windowless ships can use any
        # human-scaled feature instead (e.g. prayer's seated 2 m pilot,
        # crown-to-seat = 0.52 x stature = 1.04 m) with a "ref" note.
        ref_m = w.get("ref_m", WINDOW_M)
        loa = spans[stem] / w["h"] * ref_m / axis
        sid = m["id"]
        measured[sid] = {
            "loa_m": loa, "source": "window", "stem": stem,
            "window_px": w["h"], "ref_m": ref_m, "span_px": spans[stem],
            "axis": axis, "ref": w.get("ref", "window"),
            "confirmed": bool(w.get("confirmed")),
            "window_t": bridge_t.get(stem),
        }
    spans_path.write_text(json.dumps(spans, indent=1, sort_keys=True))
    bt_path.write_text(json.dumps(bridge_t, indent=1, sort_keys=True))

    # -- ladders ----------------------------------------------------------
    by_group, by_scale = {}, {}
    for sid, r in measured.items():
        s = ships[sid]
        by_group.setdefault((s["scale"], role_group(s["cls"])),
                            []).append(r["loa_m"])
        by_scale.setdefault(s["scale"], []).append(r["loa_m"])
    group_med = {k: median(v) for k, v in by_group.items()}
    scale_gm = {k: geomean(v) for k, v in by_scale.items()}

    # log-linear loa ~ scale over measured ships, for classes with no data
    fit = None
    if len(scale_gm) >= 2:
        pts = [(k, math.log(v)) for k, v in scale_gm.items()]
        n = len(pts)
        mx = sum(p[0] for p in pts) / n
        my = sum(p[1] for p in pts) / n
        den = sum((p[0] - mx) ** 2 for p in pts)
        b = sum((p[0] - mx) * (p[1] - my) for p in pts) / den if den else 0
        fit = (my - b * mx, b)

    def ladder(scale, group):
        if (scale, group) in group_med:
            return group_med[(scale, group)], f"ladder:{group}"
        if scale in scale_gm:
            return scale_gm[scale], "ladder:scale"
        if fit:
            return math.exp(fit[0] + fit[1] * scale), "ladder:fit"
        return None, None

    # -- emit every catalog ship that has a footprint ----------------------
    out, flags = {}, []
    for sid, s in sorted(ships.items()):
        aspect = load_aspect(sid)
        if aspect is None:
            continue
        if sid in measured:
            r = measured[sid]
            loa, source = r["loa_m"], "window"
            anchor = ladder(s["scale"], role_group(s["cls"]))[0]
            # "confirmed" entries have been re-checked by hand: an honest
            # outlier (prayer really is ~6 m) stops re-flagging forever
            if (anchor and not r["confirmed"]
                    and abs(math.log(loa / anchor)) > math.log(OUTLIER_X)):
                flags.append({"ship": sid, "loa_m": round(loa, 1),
                              "group_anchor_m": round(anchor, 1),
                              "note": "recheck window measurement"})
            extra = {k: r[k] for k in
                     ("stem", "window_px", "ref_m", "ref", "span_px", "axis",
                      "window_t")}
        else:
            loa, source = ladder(s["scale"], role_group(s["cls"]))
            if loa is None:
                continue
            extra = {}
        out[sid] = {"loa_m": round(loa, 1),
                    "beam_m": round(loa / aspect, 1),
                    "source": source,
                    "scale": s["scale"], "group": role_group(s["cls"]),
                    **extra}

    result = {
        "convention": "window pane = 1.0 m (2 m pilot); axis factor 0.87 "
                      "unless overridden; scale/tier is a complexity band, "
                      "not a size law",
        "n_measured": len(measured), "n_total": len(out),
        "ladder_group_median": {f"{k[0]}/{k[1]}": round(v, 1)
                                for k, v in sorted(group_med.items())},
        "ladder_scale_geomean": {k: round(v, 1)
                                 for k, v in sorted(scale_gm.items())},
        "recheck": flags,
        "ships": out,
    }
    (HERE / "ship_scale_est.json").write_text(
        json.dumps(result, indent=1, sort_keys=False))
    print(f"{len(measured)} measured -> {len(out)} ships sized; "
          f"{len(flags)} flagged for recheck")
    for k, v in sorted(scale_gm.items()):
        print(f"  scale {k}: {v:.0f} m geomean "
              f"({len(by_scale[k])} measured)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
