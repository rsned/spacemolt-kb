#!/bin/bash
# Round 4c: silica_borer head fix. Owner liked round-3 seed 9982's body but
# the "glassy silica drill head" read as a machine part. Same seed, organic
# boring mouthparts instead (a shipworm's rasping shell-beak), in both the
# round-3 style line (10430) and the round-4 one (10431), plus a fresh seed.
# Also glintfin 10246-10248: the herring was "too real-fish, not space-y".
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw

BODY="a tunnelling grub-like borer, segmented translucent body packed with ground rock, short hooked legs along the body for hugging stone, curled slightly, its head a hardened glassy silica rasping beak ringed with crystalline teeth like a shipworm's shell, organic boring mouthparts"
STYLE3="single creature centered, floating in empty space, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, body level and elongated, sci-fi videogame concept art, dramatic rim lighting, crisp silhouette, high detail"
STYLE4="a single living animal floating weightless in open space with no ground beneath it, biological creature with organic anatomy, natural hide and grown shell, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, xenobiology creature design, natural history illustration, soft studio lighting, crisp silhouette, high detail"

gen() { # seed outseed style
  f="$OUT/silica_borer_s$2.png"; [ -f "$f" ] && { echo "skip $f"; return; }
  PORTRAIT_SIZE=1024 ./tools/portrait -s "$1" -o "$f" "$BODY, $3" || echo "FAILED silica_borer $2"
}
GLINT="an alien space fish, a streamlined darting body plated in living mirror-scales that flash chrome and rainbow, translucent fan-fins spread wide like solar sails with glowing cyan edges, bioluminescent stripes pulsing along the flanks, a glowing rear jet vent instead of a tail fin, a ring of small black eyes, an intake slit for a mouth, organic and strange"
gen9() { f="$OUT/glintfin_s$1.png"; [ -f "$f" ] && { echo "skip $f"; return; }; PORTRAIT_SIZE=1024 ./tools/portrait -s "$1" -o "$f" "$GLINT, $STYLE4" || echo "FAILED glintfin $1"; }
gen9 10246; gen9 10247; gen9 10248

gen 9982 10430 "$STYLE3"
gen 9982 10431 "$STYLE4"
gen 10432 10432 "$STYLE4"
echo DONE
