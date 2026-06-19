#!/usr/bin/env bash
# Inject committed overlay images into the kb/ output tree before publishing.
#
# Portrait and contributor overlay images live exactly once in git, under
# overlays/. The kb/<tree>/<id>/ output copies the generated HTML references
# (via same-dir src="...") are gitignored, so they are absent on a fresh CI
# checkout. This script mirrors each source image into its matching kb/ output
# directory using the same filename the HTML expects.
#
# Convention (kept in sync with cmd/generate-factions-kb/main.go copy points):
#   overlays/generated/{passengers,players}/<id>/<img>  -> kb/<tree>/<id>/<img>
#   overlays/{passengers,players,factions}/<id>/<img>   -> kb/<tree>/<id>/<img>
# Contributor overlay images are copied after generated ones so they win on a
# filename collision (matching the generator's precedence).
#
# Safe to re-run; only copies into kb/<tree>/<id>/ directories that already
# exist (the committed HTML page). Run from anywhere.
set -euo pipefail
cd "$(dirname "$0")/.."

shopt -s nullglob
copied=0

copy_tree() {
	local src_base="$1" out_tree="$2"
	[ -d "$src_base" ] || return 0
	for src in "$src_base"/*/; do
		local id dest
		id=$(basename "$src")
		dest="kb/$out_tree/$id"
		[ -d "$dest" ] || continue
		for img in "$src"*.png "$src"*.jpg "$src"*.jpeg "$src"*.webp "$src"*.gif; do
			cp -f "$img" "$dest/"
			copied=$((copied + 1))
		done
	done
}

# Generated portraits first.
copy_tree overlays/generated/passengers passengers
copy_tree overlays/generated/players players

# Contributor overlay images second (override generated on name collision).
copy_tree overlays/passengers passengers
copy_tree overlays/players players
copy_tree overlays/factions factions

echo "inject-overlay-images: copied $copied image(s) into kb/"
