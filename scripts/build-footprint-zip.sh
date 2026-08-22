#!/usr/bin/env bash
# Build the ship footprint SVG bundle into the kb/ output tree before publishing.
#
# The 825 per-view SVGs (data/footprints/<id>_{top,side,front}.svg) are the
# committed source of truth; the .zip is a derived artifact and is gitignored,
# so it is absent on a fresh CI checkout. This rebuilds it from those SVGs and
# drops it beside the blueprint gallery, which links to it.
#
# Same shape as inject-overlay-images.sh: restore a gitignored kb/ file from
# its committed source at deploy time, so the repo carries one copy of the data
# and no duplicated binary.
#
# The archive is deterministic (sorted entries, fixed timestamps), so an
# unchanged fleet produces a byte-identical download. Safe to re-run.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="kb/ships/blueprints/ship_footprint_svg.zip"

if ! compgen -G "data/footprints/*_top.svg" >/dev/null; then
	echo "build-footprint-zip: no view SVGs in data/footprints/, nothing to bundle" >&2
	exit 1
fi

python3 data/mesh_bakeoff/export_view_svgs.py --zip-only --zip-out "$OUT"
