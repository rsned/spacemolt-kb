#!/bin/bash
# Seed-sweep FLUX hero renders for the wildlife prototype (4 gas-cloud species).
# Positive-only prompts (FLUX ignores negation); flat magenta backdrop so the
# ship pipeline's chroma_key consumes these unchanged. 3 seeds per species.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw
mkdir -p "$OUT"

STYLE="single creature centered, full body in three-quarter side profile, floating in empty space, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, sci-fi videogame concept art, dramatic rim lighting, crisp silhouette, high detail"

declare -A PROMPTS
PROMPTS[pilot_whale]="a colossal spacefaring pilot whale creature, sleek charcoal-blue hide flecked with silver, a broad armored forehead crusted with mineral plating, rows of glowing cyan gas-siphon vents along the jaw, small trailing fins and a powerful fluked tail, gentle grazer of argon gas clouds, $STYLE"
PROMPTS[bell_jelly]="a small bell-shaped space jellyfish, glassy translucent teal bell with a softly glowing amber core visible inside, delicate crystalline tendrils trailing beneath, faint shimmering membrane, serene drifting grazer, $STYLE"
PROMPTS[tempest_eel]="a monstrous serpentine space eel predator, long muscular body armored in storm-grey segmented plates, crackling blue-white static arcs dancing along a spined dorsal fin, jaws open showing needle teeth, body coiled in an S-curve mid-strike, $STYLE"
PROMPTS[drift_ray]="a graceful manta-ray-like space grazer, broad flat silver-blue wing membranes spread wide, rows of faint bioluminescent teal spots across its back, a long thin tail streamer, gliding serenely, $STYLE"

declare -A SEEDBASE=( [pilot_whale]=9210 [bell_jelly]=9220 [tempest_eel]=9230 [drift_ray]=9240 )

for sp in pilot_whale bell_jelly tempest_eel drift_ray; do
  base=${SEEDBASE[$sp]}
  for i in 0 1 2; do
    seed=$((base + i))
    f="$OUT/${sp}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "${PROMPTS[$sp]}" || echo "FAILED $sp $seed"
  done
done
echo DONE
ls -la "$OUT"
