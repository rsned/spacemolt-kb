#!/bin/bash
# Habitat scene renders: each prototype species in each of the three habitat
# POI types (asteroid_belt, gas_cloud, ice_field) -- "in their native habitat"
# concept art, like the ships' hangar renders. Scene shots, NOT chromakey:
# these are gallery/KB art, they never feed Hy3D.
# Seeds: 93<species><habitat> so every cell is reproducible.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/habitats
mkdir -p "$OUT"

STYLE="cinematic wide shot, deep space nature documentary framing, volumetric light, sci-fi videogame concept art, high detail"

declare -A CREATURE
CREATURE[pilot_whale]="a colossal spacefaring pilot whale, sleek charcoal-blue hide flecked with silver, an armored mineral-crusted forehead, rows of glowing cyan gas-siphon vents along its jaw, a small pod of its kin trailing behind it in the distance"
CREATURE[bell_jelly]="a drifting bloom of bell-shaped space jellyfish, glassy translucent teal bells each with a softly glowing amber core, delicate crystalline tendrils trailing beneath, one large jelly in the foreground"
CREATURE[tempest_eel]="a monstrous serpentine space eel predator, storm-grey segmented armor plates, crackling blue-white static arcs along its spined dorsal ridge, body in a long hunting S-curve, stalking prey"
CREATURE[drift_ray]="a graceful manta-ray-like space grazer gliding with broad silver-blue wings spread, rows of faint bioluminescent teal spots, a long tail streamer, two more rays gliding far behind it"
CREATURE[rainbow_leviathan]="a colossal armored space-lobster leviathan, the apex predator, segmented prismatic carapace fracturing light into shifting rainbow bands, massive serrated hunting claws, long armored tail, dwarfing everything around it"

declare -A HABITAT
HABITAT[asteroid_belt]="deep inside a dense asteroid belt, tumbling cratered rocks with glinting ore veins, drifting dust and pebble fields catching harsh sunlight from a distant star"
HABITAT[gas_cloud]="inside a billowing luminous nebular gas cloud, towering banks of glowing argon and neon vapor in teal and violet, soft hazy light scattering through the murk"
HABITAT[ice_field]="among a glacial ice field in space, drifting bergs and shards of blue-white ice, glittering frost crystals, cold pale light, freezing mist"

declare -A SIDX=( [pilot_whale]=1 [bell_jelly]=2 [tempest_eel]=3 [drift_ray]=4 [rainbow_leviathan]=5 )
declare -A HIDX=( [asteroid_belt]=1 [gas_cloud]=2 [ice_field]=3 )

for sp in pilot_whale bell_jelly tempest_eel drift_ray rainbow_leviathan; do
  for hab in asteroid_belt gas_cloud ice_field; do
    seed=$((9300 + 10 * SIDX[$sp] + HIDX[$hab]))
    f="$OUT/${sp}__${hab}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" \
      "${CREATURE[$sp]}, ${HABITAT[$hab]}, $STYLE" || echo "FAILED $sp $hab"
  done
done
echo DONE
