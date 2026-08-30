#!/bin/bash
# Round 4e: heroes re-rendered from OFFICIAL codex entries (v0.571.0 scans),
# where the canon changes the body -- belt_grazer (filter-plates combing
# ferrous dust, iron packed into the shell) and slag_tortoise (half-rust
# half-raw-metal dome, furnace mouth, slag trail). Rotund manatee / tortoise
# bases per the free-fall rules.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw
STYLE="a single living animal floating weightless in open space with no ground beneath it, biological creature with organic anatomy, natural hide and grown shell, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, xenobiology creature design, natural history illustration, soft studio lighting, crisp silhouette, high detail"
declare -A P
P[belt_grazer]="a placid cow-sized grazer built like a manatee, a rotund bulbous blunt-headed body with a broad paddle tail, a fan of wide flat filter-plates spread open around its mouth like baleen combs sifting fine iron dust, its back and shell packed with layers of dull grey iron plating it has grown from what it ate, methodical and unhurried, two grasping fore-flippers ending in clamp-like magnetized claws curled beneath it, small lidded slit eyes, gentle herbivore"
P[slag_tortoise]="a hill-sized tortoise-like grazer, a single massive dome shell half rust-red and half raw grey iron, foamed slag poured on in layers and frozen, an under-slung furnace mouth glowing orange as it swallows rock, a steady trail of glowing slag venting behind it, thick clamping limbs half drawn into the shell, relentlessly slow"
declare -A BASE=( [belt_grazer]=10116 [slag_tortoise]=10396 )
for sp in belt_grazer slag_tortoise; do
  for j in 0 1 2; do
    seed=$((BASE[$sp] + j)); f="$OUT/${sp}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "${P[$sp]}, $STYLE" || echo "FAILED $sp $seed"
  done
done
echo DONE
