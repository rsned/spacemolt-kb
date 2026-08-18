#!/usr/bin/env python3
"""Prototype results sheet for the wildlife sweep: one row per species —
hero render, chroma-keyed matte, the three candidate mesh views, and the
extracted top-down outline with its profile stats. View-only (the full
triage sheet takes over if/when the fleet-scale wildlife pass happens).

    python3 make_proto_sheet.py       # -> proto_sheet.html (beside out-wildlife)
"""

import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
OUT_DIR = HERE.parent / "out-wildlife"

SPECIES = {
    "pilot_whale": ("Pilot-Whale", "grazer", "hull 220"),
    "bell_jelly": ("Bell-Jelly", "grazer", "hull 45"),
    "tempest_eel": ("Tempest-Eel", "predator", "hull 280"),
    "drift_ray": ("Drift-Ray", "grazer", "hull 45"),
    "rainbow_leviathan": ("Rainbow Leviathan", "predator", "apex · cruiser-scale"),
    "gorgonia": ("Gorgonia", "predator", "unique boss · The Maw, Alzirr"),
    # Meshy premium mesh from the scene extraction; glyph = fan-face
    # silhouette (top_view 'front'), drawn centred like a station in the
    # battle viewer, at a scale that dwarfs tier-5 hulls
    "gorgonia_meshy": ("Gorgonia (Meshy boss mesh)", "predator",
                       "unique boss · station-style glyph"),
}

HEAD = """<!doctype html><meta charset="utf-8"><title>Wildlife footprint prototype</title>
<style>
body { background:#181a1f; color:#cfd3da; font:13px system-ui,sans-serif; margin:20px; }
h1 { font-size:18px; } .sub { color:#8a919c; margin-bottom:16px; }
table { border-collapse:collapse; }
td, th { padding:6px; border-bottom:1px solid #2a2e36; vertical-align:top; text-align:left; }
img { height:150px; background:#20242b; border-radius:4px; display:block; }
.name { font-weight:600; } .meta { color:#8a919c; font-size:11px; margin-top:4px; }
.role-predator { color:#c07070; } .role-grazer { color:#7ea87e; }
</style>
<h1>Wildlife footprint prototype — gas-cloud fauna</h1>
<div class="sub">FLUX hero &rarr; chroma key &rarr; Hunyuan3D-2 &rarr; footprint, the ship
pipeline unchanged. Outline is the td-frame (world-Y up) extraction.</div>
<table>
<tr><th>species</th><th>hero (FLUX)</th><th>keyed</th><th>&frac34;</th><th>side</th><th>top-down</th><th>footprint</th></tr>
"""


def main() -> int:
    rows = []
    for sp, (name, role, hull) in SPECIES.items():
        d = OUT_DIR / sp
        if not d.exists():
            continue
        rel = f"../out-wildlife/{sp}"
        prof = {}
        pf = d / "profile.json"
        if pf.exists():
            prof = json.loads(pf.read_text())
        aspect = prof.get("aspect")
        meta = (f"aspect {aspect:.2f}" if aspect else "no profile") + \
            (" · frame ambiguous" if prof.get("frame_ambiguous") else "")
        rows.append(f"""<tr>
<td><div class="name">{name}</div>
    <div class="meta role-{role}">{role} · {hull}</div>
    <div class="meta">{meta}</div></td>
<td><a href="heroes/{sp}.png" target="_blank"><img loading="lazy" src="heroes/{sp}.png"></a></td>
<td><img loading="lazy" src="{rel}/keyed.png"></td>
<td><img loading="lazy" src="{rel}/render_threequarter.png"></td>
<td><img loading="lazy" src="{rel}/render_side.png"></td>
<td><img loading="lazy" src="{rel}/render_topdown.png"></td>
<td><img loading="lazy" src="{rel}/outline_hy3d.png"></td>
</tr>""")
    (HERE / "proto_sheet.html").write_text(HEAD + "\n".join(rows) + "</table>\n")
    print(f"proto_sheet.html -> {HERE}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
