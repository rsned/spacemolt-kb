#!/bin/bash
# Round 4b: the four round-4 misses. The two "moths" kept lift-wings however
# they were described, so they become sea-butterflies -- pteropods that swim
# on two fleshy lobes, an aquatic form the rules allow. Etchmaw kept frog
# legs in a gas cloud, so it is a legless grouper-like filter fish. Glintfin's
# "mirror-bright metallic" read as chrome machinery; it is a herring now.
# Coronid's "fist of degenerate matter" was drawn as a literal hand with
# magic swirling round it, so the core is a dark sphere.
# Seeds: round-4 block +3..+5.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw

STYLE="a single living animal floating weightless in open space with no ground beneath it, biological creature with organic anatomy, natural hide and grown shell, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, xenobiology creature design, natural history illustration, soft studio lighting, crisp silhouette, high detail"

declare -A P
P[carrion_moth]="a pale sea-butterfly-like scavenger, a pteropod sea snail with a soft translucent dust-grey body and two broad rounded fleshy swimming lobes spread wide like a sea angel, no insect wings, feathery sensory antennae, a long coiled proboscis, two small hooked forelimbs for gripping carrion, tattered and sombre"
P[frost_moth]="a pale sea-butterfly-like ice-field grazer, a pteropod sea snail with a translucent frost-white body and two broad rounded fleshy swimming lobes spread wide like a sea angel, no insect wings, rimed with ice crystals, delicate grasping tendrils beneath for clamping to ice, a long proboscis for licking meltwater"
P[etchmaw]="a squat grouper-like gas-cloud filter fish, a fat legless body with mottled green-grey hide, a wide corrosive maw gaping open inhaling a stream of vapor, acid-pitted lips, broad paddle fins and a stubby tail, rear jet vents, chemical fumes wisping, no legs at all"
P[coronid]="a living storm of plasma, a small dense dark sphere of degenerate matter like a black pearl at the center, ringed by arcing loops of golden plasma erupting and rejoining like solar prominences, coronal filaments, no flesh, no body parts, pure energy and one dark core"
P[glintfin]="a small darting herring-like belt fish, sleek organic body of bright silver scales that flash like polished metal, translucent sail-like fins held wide, a rear jet vent, tiny hooked pectoral claws tucked beneath for pushing off pebbles, living fish anatomy"

declare -A BASE=( [carrion_moth]=10133 [frost_moth]=10233 [etchmaw]=10203 [glintfin]=10243 [coronid]=10173 )
for sp in carrion_moth frost_moth etchmaw glintfin coronid; do
  for j in 0 1 2; do
    seed=$((BASE[$sp] + j)); f="$OUT/${sp}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "${P[$sp]}, $STYLE" || echo "FAILED $sp $seed"
  done
done
echo DONE
