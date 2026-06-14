#!/usr/bin/env python3
"""Build the Voidborn concept gallery (voidborn-concepts/index.html) from
voidborn-concepts/prompts.json.

Each image is shown with its seed and a collapsible full prompt, so the exact
recipe for any render is one click away. Re-run after adding images/prompts:

    python3 tools/build_voidborn_gallery.py

Local-only review artifact (the voidborn-concepts/ dir is not a KB asset).
"""
import html
import json
import pathlib

ROOT = pathlib.Path(__file__).resolve().parent.parent
CONCEPTS = ROOT / "voidborn-concepts"
DATA = CONCEPTS / "prompts.json"
OUT = CONCEPTS / "index.html"
ANIMS = CONCEPTS / "anims.json"

CSS = """
  body { background:#0a0c10; color:#cdd6e0; font-family:system-ui,sans-serif; margin:0; padding:32px; }
  h1 { font-weight:500; letter-spacing:.04em; margin-bottom:4px; }
  h2 { font-weight:500; color:#9fb3cc; border-bottom:1px solid #1d2530; padding-bottom:8px; margin-top:40px; }
  h2 small { color:#6b7990; font-weight:400; }
  .sub { color:#7c8aa0; margin-bottom:8px; }
  .grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(250px,1fr)); gap:18px; margin-top:16px; }
  figure { margin:0; background:#11151c; border:1px solid #1d2530; border-radius:10px; overflow:hidden; display:flex; flex-direction:column; }
  figure.fav { border-color:#3a6ea5; box-shadow:0 0 0 1px #3a6ea5 inset; }
  figure img, figure video { width:100%; display:block; }
  figcaption { padding:11px 13px; }
  figcaption b { color:#e8eef6; }
  .seed { color:#7c8aa0; font-size:.84em; font-family:ui-monospace,monospace; }
  details { margin-top:8px; }
  details summary { cursor:pointer; color:#6f9bd1; font-size:.84em; outline:none; }
  details p { color:#9aa7b8; font-size:.82em; line-height:1.45; margin:6px 0 0; white-space:pre-wrap; }
  .topnav { position:sticky; top:0; z-index:10; margin:16px -32px 0; padding:10px 32px;
            background:rgba(10,12,16,.92); backdrop-filter:blur(6px);
            border-bottom:1px solid #1d2530; display:flex; flex-wrap:wrap; gap:6px; }
  .topnav a { color:#9fb3cc; font-size:.8em; text-decoration:none; padding:3px 9px;
              border:1px solid #1d2530; border-radius:999px; white-space:nowrap; }
  .topnav a:hover { color:#e8eef6; border-color:#3a6ea5; }
  h2 { scroll-margin-top:54px; }
"""


def short_label(title: str) -> str:
    """Strip a leading 'Voidborn · ' / 'Animations · ' prefix for compact nav chips."""
    return title.split(" · ", 1)[-1] if " · " in title else title


def figure_html(item: dict) -> str:
    fav = " fav" if item.get("fav") else ""
    f = html.escape(item["file"])
    label = html.escape(item["label"])
    seed = item["seed"]
    prompt = html.escape(item["prompt"])
    star = " ★" if item.get("fav") else ""
    return (
        f'    <figure class="card{fav}">'
        f'<a href="{f}"><img src="{f}" loading="lazy"></a>'
        f'<figcaption><b>{label}{star}</b><br>'
        f'<span class="seed">seed {seed}</span>'
        f'<details><summary>prompt</summary><p>{prompt}</p></details>'
        f'</figcaption></figure>'
    )


def video_figure_html(item: dict) -> str:
    fav = " fav" if item.get("fav") else ""
    f = html.escape(item["file"])
    label = html.escape(item["label"])
    effect = html.escape(item.get("effect", ""))
    params = html.escape(json.dumps(item.get("params", {})))
    star = " ★" if item.get("fav") else ""
    return (
        f'    <figure class="card{fav}">'
        f'<video src="{f}" autoplay loop muted playsinline></video>'
        f'<figcaption><b>{label}{star}</b><br>'
        f'<span class="seed">{effect}</span>'
        f'<details><summary>params</summary><p>{params}</p></details>'
        f'</figcaption></figure>'
    )


def main() -> None:
    data = json.loads(DATA.read_text())
    anim_data = json.loads(ANIMS.read_text()) if ANIMS.exists() else {"groups": []}
    n = sum(len(g["items"]) for g in data["groups"])
    n_clips = sum(len(g["items"]) for g in anim_data["groups"])
    parts = [
        "<!DOCTYPE html>",
        '<html lang="en"><head><meta charset="utf-8">',
        "<title>Voidborn species concepts</title>",
        f"<style>{CSS}</style></head><body>",
        "<h1>Voidborn — species concept exploration</h1>",
        f'<div class="sub">FLUX.1-schnell, 1024px · {n} renders · '
        f"{n_clips} animations · blue-outlined ★ = favorites · click an image "
        'for full size, "prompt"/"params" for the exact recipe.</div>',
    ]
    nav = [
        f'<a href="#{html.escape(g["id"])}">{html.escape(short_label(g["title"]))}</a>'
        for g in data["groups"] + anim_data["groups"]
    ]
    parts.append('<nav class="topnav">' + "".join(nav) + "</nav>")
    for g in data["groups"]:
        sub = f' <small>— {html.escape(g["subtitle"])}</small>' if g.get("subtitle") else ""
        parts.append(f'<h2 id="{html.escape(g["id"])}">{html.escape(g["title"])}{sub}</h2>')
        parts.append('<div class="grid">')
        parts.extend(figure_html(it) for it in g["items"])
        parts.append("</div>")
    for g in anim_data["groups"]:
        sub = f' <small>— {html.escape(g["subtitle"])}</small>' if g.get("subtitle") else ""
        parts.append(f'<h2 id="{html.escape(g["id"])}">{html.escape(g["title"])}{sub}</h2>')
        parts.append('<div class="grid">')
        parts.extend(video_figure_html(it) for it in g["items"])
        parts.append("</div>")
    parts.append("</body></html>")
    OUT.write_text("\n".join(parts))
    print(
        f"wrote {OUT} ({n} images, {n_clips} clips across "
        f"{len(data['groups']) + len(anim_data['groups'])} groups)"
    )


if __name__ == "__main__":
    main()
