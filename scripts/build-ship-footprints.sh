#!/usr/bin/env bash
# Stage the hy3d footprint outlines the ship pages reference.
#
# Same contract as build-fitting-plans.sh: the SVGs are already committed under
# data/footprints/hy3d-svg/, so kb/ships/footprints/ is a gitignored copy rebuilt
# at deploy time rather than a second checked-in copy of 3.9 MB of art.
#
# Deliberately a plain copy with no id mapping. Pages reference a footprint by
# its SOURCE stem, not by the ship's page id, because 32 discontinued hulls have
# their art filed under a retired id (Benefit's outline is nebula_benefit.svg).
# Resolving that lives in exactly one place -- shipFootprint() in
# cmd/generate-items-kb/ships.go -- so this script never has to know about it.
set -euo pipefail

SRC="data/footprints/hy3d-svg"
OUT="kb/ships/footprints"

if [ ! -d "$SRC" ]; then
	echo "build-ship-footprints: no $SRC, nothing to stage" >&2
	exit 1
fi

mkdir -p "$OUT"
n=0
for f in "$SRC"/*.svg; do
	cp -p "$f" "$OUT/$(basename "$f")"
	n=$((n + 1))
done
echo "build-ship-footprints: staged $n footprint outlines into $OUT"
