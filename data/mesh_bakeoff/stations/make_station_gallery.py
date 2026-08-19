#!/usr/bin/env python3
"""Review gallery for the empire station style exploration.

One section per empire (+ pirates): rows = the ten shared silhouette
archetypes, columns = moderate | capital. Missing renders show as empty
cells so the page is buildable mid-sweep.

    python3 make_station_gallery.py    # -> gallery.html (beside renders/)
"""

from pathlib import Path

from gen_stations import ARCHETYPES, EMPIRES, PIRATES, SIZES, STRONGHOLDS

HERE = Path(__file__).resolve().parent
RENDERS = HERE / "renders"

HEAD = """<!doctype html><meta charset="utf-8"><title>Empire station styles</title>
<style>
body { background:#181a1f; color:#cfd3da; font:13px system-ui,sans-serif; margin:20px; }
h1 { font-size:18px; } h2 { font-size:15px; margin:28px 0 4px; color:#9fd9ff; }
.style { color:#8a919c; max-width:900px; margin-bottom:10px; }
table { border-collapse:collapse; }
td, th { padding:6px; border-bottom:1px solid #2a2e36; vertical-align:top; text-align:left; }
th { color:#8a919c; font-weight:600; }
img { height:220px; background:#20242b; border-radius:4px; display:block; }
.arch { font-weight:600; width:110px; } .miss { color:#5f6a78; }
</style>
<h1>Empire station styles — exploration gallery</h1>
<div class="style">10 shared silhouette archetypes &times; 2 scales per empire; pirate
identities get one signature archetype each. FLUX seeds are deterministic
(gen_stations.py) — name a cell to re-roll or promote it to the Hy3D pipeline.</div>
"""


def cell(name: str, seed: int) -> str:
    f = RENDERS / f"{name}_s{seed}.png"
    if not f.exists():
        return '<td class="miss">—</td>'
    return (f'<td><a href="renders/{f.name}" target="_blank">'
            f'<img loading="lazy" src="renders/{f.name}"></a></td>')


def section(title: str, style: str, rows: list[str], headers=None) -> str:
    return (f"<h2>{title}</h2><div class='style'>{style}</div>"
            "<table><tr><th>archetype</th>"
            + "".join(f"<th>{s}</th>" for s in (headers or SIZES)) + "</tr>"
            + "".join(rows) + "</table>")


def main() -> int:
    parts = [HEAD]
    for ei, (emp, estyle) in enumerate(EMPIRES.items()):
        rows = []
        for ai, arch in enumerate(ARCHETYPES):
            cells = "".join(
                cell(f"{emp}__{arch}__{size}", 5000 + ei * 100 + ai * 10 + si)
                for si, size in enumerate(SIZES))
            rows.append(f'<tr><td class="arch">{arch}</td>{cells}</tr>')
        parts.append(section(emp, estyle, rows))
    rows = []
    for pi, (pid, (pstyle, arch)) in enumerate(PIRATES.items()):
        cells = "".join(
            cell(f"pirate_{pid}__{arch}__{size}", 5900 + pi * 10 + si)
            for si, size in enumerate(SIZES))
        rows.append(f'<tr><td class="arch">{pid}<br><span class="miss">{arch}</span></td>{cells}</tr>')
    parts.append(section("pirate factions",
                         "identities attested in KB lore + the freeport mix", rows))
    rows = []
    for pi, (pid, (where, pstyle, arch)) in enumerate(STRONGHOLDS.items()):
        cells = "".join(
            cell(f"stronghold_{pid}__{arch}__v{i}", 6000 + pi * 10 + i)
            for i in range(3))
        rows.append(f'<tr><td class="arch">{pid}<br><span class="miss">{where}<br>{arch}</span></td>{cells}</tr>')
    parts.append(section(
        "the nine pirate strongholds",
        "canonical bases from the KB (systems.is_stronghold=1); styling derived "
        "from each lord's lore line; three seed variations each",
        rows, headers=["v0", "v1", "v2"]))
    (HERE / "gallery.html").write_text("\n".join(parts))
    n = len(list(RENDERS.glob("*.png")))
    print(f"gallery.html ({n} renders present)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
