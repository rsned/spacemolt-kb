#!/bin/bash
# Gorgonia — unique boss of The Maw (Alzirr). "A vast, black coral fan rooted
# in the accretion disk... darksteel filaments and raw fury-crystal woven into
# a skeleton that drinks the radiation." Gorgonia IS the sea-fan genus: the
# body is a FLAT planar lattice, not a tree crown — seeds 9610-12 (v1 prompt
# said "fractal tree") came out as winter oaks and were rejected for it.
# Fan-language prompt, seeds 9613-9615.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw

STYLE="single creature centered, floating in empty space, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, sci-fi videogame concept art, dramatic rim lighting, crisp silhouette, high detail"

P="a vast flat sea-fan coral creature, one single broad plane of jet-black lattice, thousands of fine darksteel filaments branching and fusing back together into a reticulated lace mesh like a gorgonian sea fan, shards of glowing red-orange fury-crystal embedded throughout the weave so the whole fan smolders from within, the fan rising from a gnarled holdfast root mass of braided black metal, a faint ghostly second image of the fan offset slightly out of phase, seen at a slight three-quarter angle so the fan's broad face and thin edge both show, $STYLE"

for seed in 9613 9614 9615; do
  f="$OUT/form_gorgonia_s${seed}.png"
  [ -f "$f" ] && { echo "skip $f"; continue; }
  PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "$P" || echo "FAILED gorgonia $seed"
done
echo DONE
