#!/bin/bash
# Round 4f: the four ice-field species re-rendered from their OFFICIAL codex
# entries (v0.571.0). Ice fields are surfaces, so contact forms are canon
# here: the frost-moth skims helium ice on pale wings (our pteropod is
# retired), the hollow pilgrim walks ridges on hollow stilts (not hand-over-
# hand), the pressblister is a taut pale bladder that crawls, the rime-grazer
# a manatee buried in its own refrozen-nitrogen rime.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw
STYLE="a single living animal, biological creature with organic anatomy, natural hide and grown shell, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, xenobiology creature design, natural history illustration, soft studio lighting, crisp silhouette, high detail"
declare -A P
P[frost_moth]="a pale papery moth-like ice-field grazer with broad frost-white wings held flat and spread wide as skimming surfaces, a round abdomen swollen taut and translucent with superfluid helium glowing faintly blue, delicate legs folded beneath, fine antennae, dusted in ice crystals, resting weightless"
P[hollow_pilgrim]="a tall gaunt hooded creature walking on four long hollow stilt legs, a hollow shell body like a cowl of pale bone plates with an empty dark interior, a small clear water crystal forming at the tip of one stilt, slow purposeful eerie posture, mid-stride, floating in space"
P[pressblister]="a bloated bladder-like ice-field grazer, a taut pale translucent pressurized sac swollen with boiling nitrogen gas, domed and blistered hide stretched thin, rows of stubby crawling feet along its underside, a small licking mouth, looking as if it might pop, floating weightless"
P[rime_grazer]="a hardy manatee-like grazer, a rotund bulbous blubbery body buried under a thick crust of refrozen nitrogen rime, white frost plates and icicles caked over its back, a broad flat blunt grinding muzzle with thick leathery lips over horny rasp-plates, two strong clawed fore-flippers curled beneath it, a broad paddle tail, vents of freezing mist, no tusks, floating weightless"
declare -A BASE=( [frost_moth]=10236 [hollow_pilgrim]=10273 [pressblister]=10333 [rime_grazer]=10359 )
for sp in frost_moth hollow_pilgrim pressblister rime_grazer; do
  for j in 0 1 2; do
    seed=$((BASE[$sp] + j)); f="$OUT/${sp}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "${P[$sp]}, $STYLE" || echo "FAILED $sp $seed"
  done
done
echo DONE
