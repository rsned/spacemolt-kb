#!/usr/bin/env python3
"""Build the Voidborn archetype x body-form exploration grid
(voidborn-concepts/archetypes.html) from voidborn-concepts/archetype_matrix.json.

A matrix: one row per role archetype, one column per Voidborn body-form, each
cell a sample passenger portrait with its seed and a collapsible full prompt.
Re-run after (re)rendering the matrix:

    python3 tools/build_voidborn_archetype_grid.py

Local-only review artifact (voidborn-concepts/ is not a KB asset).
"""
import html
import json
import pathlib

ROOT = pathlib.Path(__file__).resolve().parent.parent
CONCEPTS = ROOT / "voidborn-concepts"
DATA = CONCEPTS / "archetype_matrix.json"
OUT = CONCEPTS / "archetypes.html"

CSS = """
  body { background:#0a0c10; color:#cdd6e0; font-family:system-ui,sans-serif; margin:0; padding:32px; }
  h1 { font-weight:500; letter-spacing:.04em; margin-bottom:4px; }
  .sub { color:#7c8aa0; margin-bottom:18px; }
  .sub a { color:#6f9bd1; }
  table { border-collapse:separate; border-spacing:10px; }
  th { color:#9fb3cc; font-weight:500; font-size:.9em; text-align:center; padding:6px 4px; position:sticky; top:0;
       background:#0a0c10; }
  th.role { text-align:right; min-width:120px; }
  td.role { color:#e8eef6; font-weight:500; text-align:right; vertical-align:middle; white-space:nowrap;
            padding-right:6px; }
  figure { margin:0; width:200px; background:#11151c; border:1px solid #1d2530; border-radius:9px; overflow:hidden; }
  figure img { width:100%; display:block; }
  figcaption { padding:7px 9px; }
  .seed { color:#7c8aa0; font-size:.78em; font-family:ui-monospace,monospace; }
  details summary { cursor:pointer; color:#6f9bd1; font-size:.78em; outline:none; }
  details p { color:#9aa7b8; font-size:.76em; line-height:1.4; margin:5px 0 0; white-space:pre-wrap; }
  .missing { width:200px; height:120px; display:flex; align-items:center; justify-content:center;
             color:#55617a; border:1px dashed #1d2530; border-radius:9px; font-size:.8em; }
"""


def cell_html(cell: dict) -> str:
    if not cell:
        return '<td><div class="missing">—</div></td>'
    f = html.escape(cell["file"])
    seed = cell["seed"]
    prompt = html.escape(cell["prompt"])
    return (
        f'<td><figure><a href="{f}"><img src="{f}" loading="lazy"></a>'
        f'<figcaption><span class="seed">seed {seed}</span>'
        f'<details><summary>prompt</summary><p>{prompt}</p></details>'
        f'</figcaption></figure></td>'
    )


def build(manifest_path: pathlib.Path, out_path: pathlib.Path) -> None:
    """Render a row(archetype) x column matrix page from a manifest.

    Manifest fields: `columns` (or legacy `forms`) = [{slug,label}], `archetypes`
    = [{key,label}], `cells` = {"<arch>__<colslug>": {file,seed,prompt}}, and the
    optional page-text fields `heading`, `col_label`, `title`.
    """
    data = json.loads(manifest_path.read_text())
    cols = data.get("columns") or data["forms"]   # [{slug,label}, ...]
    archetypes = data["archetypes"]               # [{key,label}, ...]
    cells = data["cells"]                          # {"<arch>__<colslug>": {...}}
    heading = data.get("heading", "Voidborn — archetype × body-form exploration")
    col_label = data.get("col_label", "Voidborn body-form")
    title = data.get("title", "archetype matrix")
    n = sum(1 for k in cells if cells[k])
    parts = [
        "<!DOCTYPE html>",
        '<html lang="en"><head><meta charset="utf-8">',
        f"<title>{html.escape(title)}</title>",
        f"<style>{CSS}</style></head><body>",
        f"<h1>{html.escape(heading)}</h1>",
        f'<div class="sub">FLUX.1-schnell, 1024px · {n} sample passenger portraits · '
        f"rows = role archetype, columns = {html.escape(col_label)} · click an image for full "
        'size, "prompt" for the recipe · <a href="index.html">← back to the main gallery</a></div>',
        "<table>",
    ]
    parts.append('<tr><th class="role"></th>'
                 + "".join(f'<th>{html.escape(c["label"])}</th>' for c in cols)
                 + "</tr>")
    for ar in archetypes:
        row = [f'<td class="role">{html.escape(ar["label"])}</td>']
        for c in cols:
            row.append(cell_html(cells.get(f'{ar["key"]}__{c["slug"]}')))
        parts.append("<tr>" + "".join(row) + "</tr>")
    parts.append("</table></body></html>")
    out_path.write_text("\n".join(parts))
    print(f"wrote {out_path} ({len(archetypes)} archetypes x {len(cols)} cols, {n} cells)")


def main(argv=None) -> None:
    import sys
    argv = argv if argv is not None else sys.argv[1:]
    manifest = pathlib.Path(argv[0]) if len(argv) > 0 else DATA
    out = pathlib.Path(argv[1]) if len(argv) > 1 else OUT
    build(manifest, out)


if __name__ == "__main__":
    main()
