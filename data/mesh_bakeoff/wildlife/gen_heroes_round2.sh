#!/bin/bash
# Round 2: rainbow_leviathan (new apex) + pose-corrected tempest_eel and
# drift_ray. The footprint is a top-down battle-view asset, so the pose must
# read from overhead: eel stretched into an open horizontal S-wave, ray flat
# and level with the full wing planform showing.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw

STYLE="single creature centered, floating in empty space, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, sci-fi videogame concept art, dramatic rim lighting, crisp silhouette, high detail"

LEV="the grandest of the void-leviathans, a colossal armored space-lobster apex predator, segmented prismatic carapace fracturing light into shifting rainbow bands, iridescent opalescent shell laced with a lattice of exotic metals, massive serrated hunting claws held forward, long armored tail extended straight behind, many articulated legs tucked beneath, body level and elongated in a hunting glide, full body in three-quarter side profile, $STYLE"

EEL="a monstrous serpentine space eel predator, its long body stretched out to full length in a wide open horizontal S-wave, undulating like a swimming serpent gliding forward, armored in storm-grey segmented plates, crackling blue-white static arcs dancing along a spined dorsal ridge, jaws open showing needle teeth, full body seen from a high three-quarter overhead angle showing the sinuous wave of its body, $STYLE"

RAY="a graceful manta-ray-like space grazer gliding perfectly level and flat, broad silver-blue wing membranes fully spread wide, rows of faint bioluminescent teal spots across its back, a long thin tail streamer trailing straight behind, full body seen from a high three-quarter overhead angle showing the complete wing planform, $STYLE"

gen() { # name prompt seeds...
  local name=$1 prompt=$2; shift 2
  for seed in "$@"; do
    f="$OUT/${name}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "$prompt" || echo "FAILED $name $seed"
  done
}

gen rainbow_leviathan "$LEV" 9250 9251 9252
gen tempest_eel "$EEL" 9233 9234 9235
gen drift_ray "$RAY" 9243 9244 9245
echo DONE
