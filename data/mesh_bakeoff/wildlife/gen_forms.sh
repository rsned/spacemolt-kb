#!/bin/bash
# "Beyond earth-equivalents" creature-form exploration — the wildlife
# analogue of the Voidborn beyond-the-human-form round. 10 body-plan
# hypotheses x 3 seeds, rendered as pipeline-ready heroes (flat magenta,
# level pose) so any winner can go straight through Hy3D.
#
# Three forms are grounded in real scraped species names (wildlife_species
# table: cauldronback, coronid, inkwyrm); seven are novel plans with no
# earth ancestor. FLUX ignores negation — every prompt steers by positive
# description only, never "not an animal / no legs".
# Seeds: form slug N gets 95N0+{0,1,2}.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw

STYLE="single creature centered, full body in three-quarter side profile, body level, floating in empty space, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, sci-fi videogame concept art, dramatic rim lighting, crisp silhouette, high detail"

declare -A FORMS
# -- grounded in scraped species names -------------------------------------
FORMS[cauldronback]="a walking crucible creature, a dome of fused slag rock with a glowing molten caldera lake set into its back, dribbles of magma running down its flanks, four stumpy basalt pillar legs, ember light from within, $STYLE"
FORMS[coronid]="a creature of pure stellar plasma, a floating halo ring of arcing golden fire surrounding a small dark stellar core, prominence loops erupting and rejoining like slow limbs, corona streamers trailing, $STYLE"
FORMS[inkwyrm]="a segmented burrowing wyrm whose long body is absolute ink-black void that swallows all light, a chrome-mirrored rotary drill maw at the front the only reflective part, faint dust spiraling into it, $STYLE"
# -- novel body plans, no earth ancestor -----------------------------------
FORMS[tessellate]="a flying carpet of interlocking hexagonal chitin tiles, a flat mosaic-sheet creature rippling as its tiles re-tessellate mid-glide, glowing seams between plates, grazing surface-first over rock, $STYLE"
FORMS[orrery]="a living orrery, concentric rings of polished stone organs orbiting slowly around a glowing amber nucleus, each ring turning at its own speed, connected by nothing but gravity, $STYLE"
FORMS[chandelier]="an inverted crystal chandelier organism, a buoyant iridescent gas bladder crown above branching mineral stalactite arms that fork downward into hundreds of glowing prism tips, $STYLE"
FORMS[mobius]="a creature that is a single twisted ribbon of banded muscle and shell looping back into itself, a rolling half-twisted band with cilia fringing both edges, tumbling gracefully end over end, $STYLE"
FORMS[barnacle_moon]="a boulder-sized armored sphere creature disguised as a cratered asteroid, its rock shell split open into radial petal jaws revealing a glowing feeding throat at the center, $STYLE"
FORMS[filament]="an immensely long drift-line creature, a single glowing neural thread strung with periodic bead organs like pearls, ends frayed into fine sail fibers that ride charged dust, gently bowed, $STYLE"
FORMS[dustveil]="a swarm-body creature, thousands of magnetized dust motes flying as one coordinated translucent veil membrane, curling like silk in vacuum, a denser glowing knot of core motes at its heart, $STYLE"

declare -A FIDX=( [cauldronback]=1 [coronid]=2 [inkwyrm]=3 [tessellate]=4 [orrery]=5 [chandelier]=6 [mobius]=7 [barnacle_moon]=8 [filament]=9 [dustveil]=10 )

for form in cauldronback coronid inkwyrm tessellate orrery chandelier mobius barnacle_moon filament dustveil; do
  base=$((9500 + 10 * FIDX[$form]))
  for i in 0 1 2; do
    seed=$((base + i))
    f="$OUT/form_${form}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "${FORMS[$form]}" || echo "FAILED $form $seed"
  done
done
echo DONE
