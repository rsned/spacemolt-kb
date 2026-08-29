#!/bin/bash
# Round 3b: organic re-take of the grazers that round 3 rendered as robots.
# "Armored quadruped ... overlapping plates" plus the videogame-concept-art
# style line read as mecha to FLUX (panel seams, bolted plating). These
# prompts describe the same animals as LIVING bodies -- hide, chitin, keratin,
# muscle, shell grown not built -- under a natural-history style line. FLUX at
# guidance 0 ignores negatives, so it is all positive wording.
#
# Seeds: the species' round-3 block +3..+5, so the round-3 originals stay on
# the sheet for side-by-side comparison.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw
mkdir -p "$OUT"

STYLE="a single living animal, biological creature with organic anatomy, natural hide and grown shell, no machinery, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, body level, xenobiology creature design, natural history illustration, soft studio lighting, crisp silhouette, high detail"

declare -A P
P[belt_grazer]="a placid cow-sized space grazer, a stocky quadruped beast with thick leathery grey-brown hide, a broad back covered in a natural chitinous carapace of overlapping scutes grown like a tortoise shell with a molting sheen, heavy hooves, small lidded eyes narrowed to slits, a long rasping tongue, gentle herbivore"
P[patina_grazer]="a placid cow-sized space grazer, a stocky quadruped beast whose leathery hide and natural shell scutes are furred over with a symbiotic blue-green lichen like verdigris on old bronze, mossy crust, heavy hooves, small lidded eyes, gentle herbivore"
P[soot_grazer]="a placid cow-sized space grazer, a stocky quadruped beast matte black from head to hoof, velvety radiation-drinking black hide, natural chitin scutes along the back, heavy hooves, small lidded eyes, fine ash drifting from its flanks, gentle herbivore"
P[rime_grazer]="a hardy cow-sized space grazer, a stocky shaggy quadruped beast with thick white fur crusted in rime frost, ice crystals hanging from its coat, a natural horn-keratin shell over the shoulders, heavy hooves, breath of freezing mist, gentle herbivore"
P[rivetshell]="a stout dome-shelled space grazer like a giant armadillo, a natural rust-brown keratin shell studded with rows of round bony knobs like rivets, wrinkled leathery hide, short thick legs, small blunt head, slow herbivore"
P[bullionaut]="a heavy beetle-like space grazer with a natural chitin shell gleaming metallic gold like a jewel beetle, faceted golden wing-cases, thick segmented legs, a crushing ore-grinding mouth, dense and slow"

declare -A BASE=( [belt_grazer]=9713 [patina_grazer]=9913 [soot_grazer]=10003 [rime_grazer]=9953 [rivetshell]=9963 [bullionaut]=9723 )

for sp in belt_grazer patina_grazer soot_grazer rime_grazer rivetshell bullionaut; do
  for j in 0 1 2; do
    seed=$((BASE[$sp] + j))
    f="$OUT/${sp}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "${P[$sp]}, $STYLE" || echo "FAILED $sp $seed"
  done
done
echo DONE
