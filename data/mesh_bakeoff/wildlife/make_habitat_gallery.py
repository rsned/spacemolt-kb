#!/usr/bin/env python3
"""Habitat matrix gallery: rows = species, columns = the three habitat POI
types. Each cell holds up to two renders: the plain scene (creature dropped
into the habitat, seeds 93xx) and the diet-ADAPTED sibling-species variant
(feeding anatomy reshaped for that habitat's resource, seeds 94xx).

    python3 make_habitat_gallery.py     # -> habitats/index.html
"""

from pathlib import Path

HERE = Path(__file__).resolve().parent
HAB_DIR = HERE / "habitats"

SPECIES = ["pilot_whale", "bell_jelly", "tempest_eel", "drift_ray", "rainbow_leviathan"]
HABITATS = ["asteroid_belt", "gas_cloud", "ice_field"]
NATIVE = {sp: "gas_cloud" for sp in SPECIES}  # the prototype list came from a gas_cloud POI

HEAD = """<!doctype html><meta charset="utf-8"><title>Wildlife habitat matrix</title>
<style>
body { background:#181a1f; color:#cfd3da; font:13px system-ui,sans-serif; margin:20px; }
h1 { font-size:18px; } .sub { color:#8a919c; margin-bottom:16px; max-width:900px; }
table { border-collapse:collapse; }
td, th { padding:8px; border-bottom:1px solid #2a2e36; vertical-align:top; }
th { text-align:left; color:#aeb4bd; }
.sp { font-weight:600; white-space:nowrap; }
.native { color:#7ea87e; font-size:11px; }
img { width:230px; border-radius:5px; background:#20242b; display:block; }
img + img { margin-top:6px; }
.lbl { color:#8a919c; font-size:10px; margin:2px 0 6px; }
</style>
<h1>Wildlife habitat matrix</h1>
<div class="sub">Each cell: top = the species dropped into the habitat as-is,
bottom = the diet-<b>adapted</b> sibling species (feeding anatomy reshaped for the
habitat's resource — baleen siphons for gas, crusher jaws for ore, ice-breaker
prows for bergs). Click any image for full size.</div>
<table>
"""


def cell(sp: str, hab: str) -> str:
    parts = []
    for suffix, lbl in [("", "scene"), ("__adapted", "adapted sibling")]:
        f = HAB_DIR / f"{sp}__{hab}{suffix}.png"
        if f.exists():
            parts.append(f'<a href="{f.name}" target="_blank"><img loading="lazy" src="{f.name}"></a>'
                         f'<div class="lbl">{lbl}</div>')
    return "<td>" + "".join(parts or ["<div class='lbl'>—</div>"]) + "</td>"


def main() -> int:
    rows = ["<tr><th></th>" + "".join(f"<th>{h}</th>" for h in HABITATS) + "</tr>"]
    for sp in SPECIES:
        native = f'<div class="native">native: {NATIVE[sp]}</div>' if sp in NATIVE else ""
        rows.append(f'<tr><td class="sp">{sp}{native}</td>' +
                    "".join(cell(sp, h) for h in HABITATS) + "</tr>")
    (HAB_DIR / "index.html").write_text(HEAD + "\n".join(rows) + "</table>\n")
    print(f"index.html -> {HAB_DIR}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
