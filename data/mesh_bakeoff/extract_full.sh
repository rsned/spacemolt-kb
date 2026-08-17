#!/bin/bash
# Serial footprint extraction over a sweep output directory.
#
# Strictly one mesh at a time, each inside a systemd memory cap: the 2026-08-13
# workstation lockup was caused by running a big shapely union concurrently
# with the generation sweep on a 31GB/2GB-swap box. Hy3D sweep meshes are
# ~40k faces so 8G is generous; a mesh that still blows the cap gets killed
# by the kernel and logged as FAILED instead of taking the desktop down.
#
#     ./extract_full.sh [out-hy3d-full]
set -u
cd "$(dirname "$0")"
DIR="${1:-out-hy3d-full}"

n=0
fail=0
for obj in "$DIR"/*/mesh.obj; do
  d=$(dirname "$obj")
  stem=$(basename "$d")
  [ -f "$d/profile.json" ] && continue
  n=$((n + 1))
  echo "== $stem"
  if ! systemd-run --user --scope -q -p MemoryMax=8G -p MemorySwapMax=0 \
      ~/sf3d-venv/bin/python mesh_footprint.py "$obj" "$stem" "$d" \
      >"$d/extract.log" 2>"$d/extract.err"; then
    fail=$((fail + 1))
    echo "   FAILED $stem (see $d/extract.err)"
  fi
done
echo "extracted $((n - fail)) ok, $fail failed"
