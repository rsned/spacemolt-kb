#!/usr/bin/env bash
# Extract top-down footprints from the round-2 (Hunyuan3D-2) meshes.
#
# Runs the SAME mesh_footprint.py that round 1 used, from the same sf3d-venv
# that has its trimesh/scipy/shapely stack -- the comparison is only
# meaningful if everything downstream of the generative model is identical.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PY="$HOME/sf3d-venv/bin/python"
OUT="$HERE/out-hy3d"

ok=0; fail=0
for d in "$OUT"/*/; do
    stem="$(basename "$d")"
    mesh="$d/mesh.obj"
    [ -f "$mesh" ] || { echo "  $stem: no mesh.obj, skipped"; continue; }
    if "$PY" "$HERE/mesh_footprint.py" "$mesh" "$stem" "$d" >/dev/null 2>"$d/extract.err"; then
        aspect=$(python3 -c "import json;print(f\"{json.load(open('$d/profile.json'))['aspect']:.3f}\")")
        printf "  %-30s aspect %s\n" "$stem" "$aspect"
        ok=$((ok+1))
    else
        printf "  %-30s FAILED (see %sextract.err)\n" "$stem" "$d"
        fail=$((fail+1))
    fi
done
echo "extracted ok=$ok failed=$fail"
