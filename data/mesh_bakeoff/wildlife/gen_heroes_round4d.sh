#!/bin/bash
# Round 4d: patina_grazer from its OFFICIAL codex entry (v0.571.0 scan):
# filter-plates spread wide combing copper dust, strained metal laid over the
# shell until it weathers to verdigris. Rotund manatee base per the rules.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw
STYLE="a single living animal floating weightless in open space with no ground beneath it, biological creature with organic anatomy, natural hide and grown shell, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, xenobiology creature design, natural history illustration, soft studio lighting, crisp silhouette, high detail"
P="a placid cow-sized grazer built like a manatee, a rotund bulbous blunt-headed body with a broad paddle tail, a fan of wide flat filter-plates spread open around its mouth like baleen combs sifting fine copper dust, its back and shell crusted in layers of deposited copper weathered to blue-green verdigris, green-crusted and unhurried, two grasping fore-flippers ending in clamp-like claws curled beneath it, small lidded eyes, gentle herbivore"
for seed in 10316 10317 10318; do
  f="$OUT/patina_grazer_s${seed}.png"; [ -f "$f" ] && { echo "skip $f"; continue; }
  PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "$P, $STYLE" || echo "FAILED patina_grazer $seed"
done
echo DONE
