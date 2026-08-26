#!/usr/bin/env python3
"""Build the KB site search index: kb/search-index.json.

Walks the *built* kb/ tree and records one entry per page: its path and its
title, with the " - Spacemolt KB" suffix and any trailing " - <Section>" removed
(both are already implied by the section the entry sits under). Runs after the
page generators because it reads their output, not their inputs.

The index is grouped by top-level section so the section name is stored once
rather than 15,000 times, and so the client can weight catalog sections above
bulk records without a per-entry rank field.

Measured on the current site: 15,649 pages, 1.46 MB raw, ~230 KB gzipped, which
GitHub Pages serves compressed. The output is gitignored and rebuilt at deploy
time by scripts/build-search-index.sh -- the same contract as
build-fitting-plans.sh, so a regenerated site does not churn a 1.5 MB blob
through every diff.

    python3 scripts/build_search_index.py [--out kb/search-index.json]
"""
import argparse
import json
import os
import re
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent
KB = REPO / "kb"

TITLE_RE = re.compile(r"<title>(.*?)</title>", re.S | re.I)
# Only the head matters and some pages are megabytes; stop after this much.
HEAD_BYTES = 4096

DASH = r"[-\u2010-\u2015|]"          # hyphen, the unicode dash block, or a pipe
SITE_SUFFIX = re.compile(r"\s*" + DASH + r"\s*Spacemolt\s*KB\s*$", re.I)
WS = re.compile(r"\s+")

# Sections whose pages are catalog reference material. Everything else (missions,
# passengers, players, build-costs, market, diffs) is bulk: still searchable, but
# ranked below these so 4,318 trade runs cannot bury the item catalog.
CATALOG = {
    "items", "ships", "systems", "recipes", "skills",
    "facilities", "factions", "resources",
}

# Pages that are noise in a search box: fragments and machine-readable payloads.
SKIP_DIRS = {"images"}


def page_title(path: Path, fallback: str) -> str:
    """The page's <title>, trimmed of the parts the index already encodes."""
    try:
        with path.open(encoding="utf-8", errors="ignore") as fh:
            head = fh.read(HEAD_BYTES)
    except OSError:
        return fallback
    m = TITLE_RE.search(head)
    if not m:
        return fallback
    title = WS.sub(" ", m.group(1)).strip()
    title = SITE_SUFFIX.sub("", title)
    return title or fallback


def strip_section(title: str, section: str) -> str:
    """Drop a trailing " - Systems" when the entry already sits under systems."""
    if not section:
        return title
    tail = re.compile(r"\s*" + DASH + r"\s*" + re.escape(section) + r"\s*$", re.I)
    stripped = tail.sub("", title)
    return stripped or title


def build(kb: Path):
    sections, pages = {}, 0
    for dirpath, dirnames, filenames in os.walk(kb):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for name in filenames:
            if not name.endswith(".html"):
                continue
            path = Path(dirpath) / name
            rel = path.relative_to(kb).as_posix()
            parts = rel.split("/")
            section = parts[0] if len(parts) > 1 else ""
            # Section index pages are reachable from the nav already.
            title = page_title(path, name[:-5].replace("_", " "))
            title = strip_section(title, section.replace("-", " "))
            sections.setdefault(section, []).append([rel, title])
            pages += 1
    for entries in sections.values():
        entries.sort(key=lambda e: e[1].lower())
    return sections, pages


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(KB / "search-index.json"))
    args = ap.parse_args()

    if not KB.is_dir():
        sys.exit(f"missing {KB} — run the page generators first")

    sections, pages = build(KB)
    doc = {
        "v": 1,
        # Sections the client ranks first; everything else sorts below.
        "catalog": sorted(CATALOG),
        "s": sections,
    }
    out = Path(args.out)
    out.write_text(json.dumps(doc, separators=(",", ":")))
    size = out.stat().st_size
    top = sorted(((len(v), k) for k, v in sections.items()), reverse=True)[:5]
    print(f"build-search-index: {pages:,} pages across {len(sections)} sections "
          f"-> {out.relative_to(REPO)} ({size/1e6:.2f} MB)")
    print("  largest: " + ", ".join(f"{k or '/'}={n:,}" for n, k in top))


if __name__ == "__main__":
    main()
