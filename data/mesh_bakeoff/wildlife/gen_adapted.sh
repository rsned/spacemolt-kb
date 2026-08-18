#!/bin/bash
# Habitat-ADAPTED variant matrix: same body plan per species, but the feeding
# anatomy changes with the habitat's diet (user's direction: "one with baleen
# to inhale gas, vs teeth to eat asteroid/ice"). Docs agree — each habitat
# hosts distinct species eating its specific resource — so each cell reads as
# a sibling species of the family, shown actively feeding/hunting.
# Seeds: 94<species><habitat>, sibling block of gen_habitats.sh's 93xx.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/habitats
mkdir -p "$OUT"

STYLE="cinematic wide shot, deep space nature documentary framing, volumetric light, sci-fi videogame concept art, high detail"

declare -A HABITAT
HABITAT[asteroid_belt]="deep inside a dense asteroid belt, tumbling cratered rocks with glinting ore veins, drifting dust and pebble fields catching harsh sunlight from a distant star"
HABITAT[gas_cloud]="inside a billowing luminous nebular gas cloud, towering banks of glowing argon and neon vapor in teal and violet, soft hazy light scattering through the murk"
HABITAT[ice_field]="among a glacial ice field in space, drifting bergs and shards of blue-white ice, glittering frost crystals, cold pale light, freezing mist"

# CREATURE[species__habitat] = body plan + habitat-specific feeding anatomy, feeding
declare -A CREATURE
CREATURE[pilot_whale__gas_cloud]="a colossal spacefaring pilot whale, sleek charcoal-blue hide, rows of glowing cyan baleen siphon vents flared wide along its jaw, inhaling a swirling plume of luminous vapor into its mouth"
CREATURE[pilot_whale__asteroid_belt]="a colossal spacefaring whale with a massive rock-crusher jaw, heavy grinding tusks and a gravel-scarred armored snout, crunching through an ore-veined boulder in a spray of glittering rubble"
CREATURE[pilot_whale__ice_field]="a colossal spacefaring whale with an armored icebreaker prow of a skull and serrated shearing teeth, biting a drifting iceberg in half amid a burst of frost and ice shards"
CREATURE[bell_jelly__gas_cloud]="a bell-shaped translucent teal space jellyfish with a glowing amber core, its fine crystalline tendrils fanned into a wide drift-net, straining glowing vapor streams that spiral up into its bell"
CREATURE[bell_jelly__asteroid_belt]="a bell-shaped translucent space jellyfish with a glowing amber core, its thick muscular tendrils wrapped around a small ore asteroid, acid-etched glowing channels dissolving the rock where they grip"
CREATURE[bell_jelly__ice_field]="a bell-shaped translucent space jellyfish with a glowing amber core, its tendrils fused into faceted crystalline drills bored deep into a blue ice shard, drinking glowing meltwater up through them"
CREATURE[tempest_eel__gas_cloud]="a monstrous serpentine space eel wreathed in its own storm, static arcs leaping from its spined dorsal ridge into the charged vapor around it, jaws open as it lunges at a small glowing jellyfish"
CREATURE[tempest_eel__asteroid_belt]="a monstrous serpentine space eel ambush predator coiled through the crevices of a large cratered asteroid, only its head and crackling spines emerging from shadow, eyes fixed on prey"
CREATURE[tempest_eel__ice_field]="a monstrous serpentine space eel coiled around a drifting iceberg, static arcs grounding into the ice in glowing fractures, stalking a small pale creature sheltering inside"
CREATURE[drift_ray__gas_cloud]="a graceful manta-ray-like space grazer with broad silver-blue wings, its wide mouth scoop flared open, skimming along a bank of glowing vapor and swallowing the stream"
CREATURE[drift_ray__asteroid_belt]="a manta-ray-like space grazer hugging the surface of a large asteroid, its rasping underside mouth grinding glittering ore dust off the rock as it glides low like a remora"
CREATURE[drift_ray__ice_field]="a manta-ray-like space grazer with frost-white wing edges, gliding along the face of an iceberg, scraping and licking up sparkling frost crystals with a brush-tongued mouth"
CREATURE[rainbow_leviathan__gas_cloud]="a colossal prismatic-shelled space-lobster leviathan bursting out of a glowing vapor bank, rainbow carapace scattering the nebula light, serrated claws snapping shut around a fleeing glowing jellyfish"
CREATURE[rainbow_leviathan__asteroid_belt]="a colossal prismatic-shelled space-lobster leviathan striding across a large asteroid, rainbow carapace glinting in harsh sunlight, claws prying a smaller shelled grazer out of a crevice"
CREATURE[rainbow_leviathan__ice_field]="a colossal prismatic-shelled space-lobster leviathan smashing through a glacial berg, ice shards exploding off its rainbow carapace, claws lunging after a pale fleeing whale calf"

declare -A SIDX=( [pilot_whale]=1 [bell_jelly]=2 [tempest_eel]=3 [drift_ray]=4 [rainbow_leviathan]=5 )
declare -A HIDX=( [asteroid_belt]=1 [gas_cloud]=2 [ice_field]=3 )

for sp in pilot_whale bell_jelly tempest_eel drift_ray rainbow_leviathan; do
  for hab in asteroid_belt gas_cloud ice_field; do
    seed=$((9400 + 10 * SIDX[$sp] + HIDX[$hab]))
    f="$OUT/${sp}__${hab}__adapted.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" \
      "${CREATURE[${sp}__${hab}]}, ${HABITAT[$hab]}, $STYLE" || echo "FAILED $sp $hab"
  done
done
echo DONE
