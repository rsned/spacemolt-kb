#!/usr/bin/env bash
# Stage the top-view plan silhouettes the fitting sheet fetches on demand.
#
# data/footprints/<id>_top.svg is the committed source of truth (275 ships,
# 2.6 MB). kb/ships/fitting.html fetches plans/<id>.svg per ship rather than
# baking every path into the page, so this copies them into the kb/ output tree
# at deploy time and leaves the repo carrying one copy of the data.
#
# Same shape as build-footprint-zip.sh and inject-overlay-images.sh. The output
# directory is gitignored. Safe to re-run.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="kb/ships/plans"

if ! compgen -G "data/footprints/*_top.svg" >/dev/null; then
	echo "build-fitting-plans: no top-view SVGs in data/footprints/, nothing to stage" >&2
	exit 1
fi

mkdir -p "$OUT"
n=0
for f in data/footprints/*_top.svg; do
	base="$(basename "$f")"
	cp -p "$f" "$OUT/${base%_top.svg}.svg"
	n=$((n + 1))
done
echo "build-fitting-plans: staged $n plan views into $OUT"
