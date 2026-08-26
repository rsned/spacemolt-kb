#!/usr/bin/env bash
# Build the site search index into the kb/ output tree before publishing.
#
# kb/search-index.json is derived from the generated pages (it reads their
# <title>), so it is a build artifact rather than source: gitignored here and
# rebuilt at deploy time, the same contract as build-fitting-plans.sh and
# build-footprint-zip.sh. That keeps a 1.5 MB blob out of every regeneration
# diff while guaranteeing the deployed index matches the deployed pages.
#
# Safe to re-run.
set -euo pipefail
cd "$(dirname "$0")/.."

if ! compgen -G "kb/*.html" >/dev/null; then
	echo "build-search-index: no pages in kb/, nothing to index" >&2
	exit 1
fi

python3 scripts/build_search_index.py
