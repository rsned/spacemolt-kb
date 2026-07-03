#!/usr/bin/env python3
"""Build the combined archetype contact sheet (voidborn-concepts/combined.html).

One wide table laying every archetype's looks side by side: 22 rows (one per
role archetype, alphabetical) x 10 columns — the four empires at male+female
(Crimson, Nebula, Outer Rim, Solarian) plus the two humanoid Voidborn body-forms
(Chassis, Two-frames). Pure composition of the already-rendered cells in the
five *_matrix.json manifests; no images are generated here.

    python3 tools/build_archetype_combined.py [out.html]

Run from the kb repo root after build_archetype_matrices.py has populated the
manifests (which also invokes this).
"""

import html
import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
CONCEPTS = ROOT / "voidborn-concepts"
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
from build_voidborn_archetype_grid import CSS, cell_html  # noqa: E402

OUT = CONCEPTS / "combined.html"

# (manifest stem, column slug, group label, sub-label) in display order.
COLUMNS = [
    ("crimson", "m", "Crimson", "M"),
    ("crimson", "f", "Crimson", "F"),
    ("nebula", "m", "Nebula", "M"),
    ("nebula", "f", "Nebula", "F"),
    ("outerrim", "m", "Outer Rim", "M"),
    ("outerrim", "f", "Outer Rim", "F"),
    ("solarian", "m", "Solarian", "M"),
    ("solarian", "f", "Solarian", "F"),
    ("archetype", "chassis", "Voidborn", "Chassis"),
    ("archetype", "twoframes", "Voidborn", "Two-frames"),
]

# Scope-narrower figures + a sticky role column so a 10-wide table stays usable.
EXTRA_CSS = """
  figure { width:150px; }
  .missing { width:150px; }
  td.role, th.role { position:sticky; left:0; background:#0a0c10; z-index:2; }
  th.group { border-bottom:1px solid #1d2530; font-size:.95em; color:#cdd6e0; }
"""


def manifest_path(stem):
    return CONCEPTS / ("archetype_matrix.json" if stem == "archetype" else f"{stem}_matrix.json")


def build(out_path):
    data = {stem: json.loads(manifest_path(stem).read_text())
            for stem in {c[0] for c in COLUMNS}}
    # Rows = the full archetype set (the voidborn matrix carries all 22),
    # displayed alphabetically by label.
    archetypes = sorted(data["archetype"]["archetypes"], key=lambda a: a["label"].lower())

    n = sum(1 for stem, slug, _, _ in COLUMNS for a in archetypes
            if data[stem]["cells"].get(f'{a["key"]}__{slug}'))

    # Grouped top header: merge consecutive columns that share a group label.
    group_cells = []
    i = 0
    while i < len(COLUMNS):
        label = COLUMNS[i][2]
        span = 1
        while i + span < len(COLUMNS) and COLUMNS[i + span][2] == label:
            span += 1
        group_cells.append(f'<th class="group" colspan="{span}">{html.escape(label)}</th>')
        i += span

    parts = [
        "<!DOCTYPE html>",
        '<html lang="en"><head><meta charset="utf-8">',
        "<title>archetype matrix — all empires + Voidborn</title>",
        f"<style>{CSS}{EXTRA_CSS}</style></head><body>",
        "<h1>Archetypes across all empires + Voidborn</h1>",
        f'<div class="sub">FLUX.1-schnell, 1024px · {n} portraits · rows = role archetype, '
        "columns = empire (male/female) + humanoid Voidborn body-form · click an image for full "
        'size, "prompt" for the recipe · <a href="index.html">← back to the main gallery</a></div>',
        "<table>",
        '<tr><th class="role" rowspan="2"></th>' + "".join(group_cells) + "</tr>",
        "<tr>" + "".join(f'<th>{html.escape(sub)}</th>' for _, _, _, sub in COLUMNS) + "</tr>",
    ]
    for a in archetypes:
        row = [f'<td class="role">{html.escape(a["label"])}</td>']
        for stem, slug, _, _ in COLUMNS:
            row.append(cell_html(data[stem]["cells"].get(f'{a["key"]}__{slug}')))
        parts.append("<tr>" + "".join(row) + "</tr>")
    parts.append("</table></body></html>")
    out_path.write_text("\n".join(parts))
    print(f"wrote {out_path} ({len(archetypes)} archetypes x {len(COLUMNS)} cols, {n} cells)")


def main(argv=None):
    argv = argv if argv is not None else sys.argv[1:]
    out = pathlib.Path(argv[0]) if argv else OUT
    build(out)


if __name__ == "__main__":
    main()
