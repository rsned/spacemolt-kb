"""Stage the station triage sheet for the KB as a self-contained web page.

triage.html points at 411 MB of PNGs under renders/ and ../out-stations/, which
cannot ship: the .gitignore here keeps image binaries out of the repo, and a
Pages runner has no GPU to regenerate them. This writes a lean WebP mirror
instead -- hero, chroma-keyed source and the three-quarter mesh view per cell,
~86 KB a cell -- plus a copy of the sheet whose <img> srcs point at it.

The side and top-down mesh views are dropped; run the local triage.html when a
bake needs those (ghost planes, elevation-sign checks).

    python3 data/mesh_bakeoff/stations/build_triage_web.py
"""

from __future__ import annotations

import io
import re
import sys
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[3]
STATIONS = ROOT / "data" / "mesh_bakeoff" / "stations"
BAKES = ROOT / "data" / "mesh_bakeoff" / "out-stations"
OUT_HTML = ROOT / "kb" / "did_you_know" / "station_triage.html"
OUT_IMG = ROOT / "kb" / "did_you_know" / "station_triage"

# (max dimension, quality, keep alpha) -- the keyed source is cut out, so it
# needs its transparency; the renders are opaque frames.
SPEC = {"hero": (640, 75, False), "keyed": (640, 75, True), "tq": (480, 75, False)}

NOTE = (
    '<div class="miss">Static snapshot of the local triage sheet — stars and notes '
    "live in this browser's localStorage, so they do not follow you here from a "
    "local run, and nothing you star here is written back to the repo. Side and "
    "top-down mesh views are omitted; use the local sheet for those.</div>"
)


def encode(src: Path, dst: Path, kind: str) -> int:
    maxdim, quality, alpha = SPEC[kind]
    im = Image.open(src)
    im = im.convert("RGBA" if alpha else "RGB")
    if max(im.size) > maxdim:
        im.thumbnail((maxdim, maxdim), Image.LANCZOS)
    buf = io.BytesIO()
    im.save(buf, "WEBP", quality=quality, method=6)
    dst.write_bytes(buf.getvalue())
    return len(buf.getvalue())


def main() -> int:
    src_html = STATIONS / "triage.html"
    if not src_html.exists():
        print("no triage.html -- run make_station_triage.py first", file=sys.stderr)
        return 1
    html = src_html.read_text()
    OUT_IMG.mkdir(parents=True, exist_ok=True)

    # Drop the two view columns, header and cells alike, before rewriting srcs.
    html = html.replace("<th>side</th><th>top-down</th>", "")
    html = re.sub(r'<td><img loading="lazy" src="[^"]*view_(?:side|td)\.png"></td>', "", html)

    total = 0
    missing: list[str] = []
    seen: set[tuple[str, str]] = set()

    def swap(m: re.Match[str]) -> str:
        nonlocal total
        src = m.group(1)
        if src.startswith("renders/"):
            cell, kind, path = Path(src).stem, "hero", STATIONS / src
        else:
            parts = src.split("/")
            cell, kind = parts[-2], "keyed" if parts[-1] == "keyed.png" else "tq"
            path = BAKES / cell / parts[-1]
        dst = OUT_IMG / f"{cell}_{kind}.webp"
        if (cell, kind) not in seen:
            seen.add((cell, kind))
            if path.exists():
                total += encode(path, dst, kind)
            else:
                missing.append(str(path))
        return f'<img loading="lazy" src="station_triage/{dst.name}">'

    html = re.sub(r'<img loading="lazy" src="([^"]+)">', swap, html)
    html = html.replace("Re-run make_station_triage.py to pull in newly baked meshes.", "")
    html = html.replace("</div>\n", "</div>\n" + NOTE + "\n", 1)
    OUT_HTML.write_text(html)

    print(f"wrote {OUT_HTML.relative_to(ROOT)}  {len(html) / 1024:.0f} KB")
    print(f"wrote {len(seen)} images to {OUT_IMG.relative_to(ROOT)}/  {total / 1024 / 1024:.1f} MB")
    if missing:
        print(f"MISSING {len(missing)} source images, e.g. {missing[0]}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
