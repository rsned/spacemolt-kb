#!/bin/bash
# Round 4: the 33 non-prototype species again, this time under the free-fall
# design rules now written into FORMS_LORE.md -- no ground, no gravity
# orientation; belt/ice species grasp and clamp (no hooves, no walking);
# cloud-only species filter/inhale gas and carry limbs only for prey capture;
# no lift-wings (big membranes are sails, collectors, or slow ray-strokes
# through the thin medium); serpentine bodies cruise slowly. Living bodies,
# organic wording, natural-history style (round 3 read as mecha).
#
# Held objects are deliberately kept OUT of the hero shot -- a clutched rock
# would become part of the chroma-keyed silhouette that feeds Hy3D. Grasping
# limbs are shown curled in their clamp posture instead; "holding the rock"
# belongs to the habitat scene pass.
#
# Seeds: 10100 + 10*i for species i in alphabetical order (10100..10420).
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw
mkdir -p "$OUT"

STYLE="a single living animal floating weightless in open space with no ground beneath it, biological creature with organic anatomy, natural hide and grown shell, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, xenobiology creature design, natural history illustration, soft studio lighting, crisp silhouette, high detail"

declare -A P
# --- asteroid belt: clingers and clampers
P[ash_scarab]="a hand-sized isopod-like scavenger, matte ash-grey chitin, six hooked grasping legs curled ready to latch onto drifting carrion, carcass-stripping mandibles, wing-cases fused into radiator plates, drifting slowly"
P[belt_grazer]="a placid cow-sized grazer built like a manatee, a rotund bulbous blunt-headed body in thick leathery grey-brown hide, a natural chitinous carapace of overlapping scutes grown over the back, a broad flat paddle tail, two strong grasping fore-flippers ending in clamp-like magnetized claws curled beneath it, a rasping mouth, small lidded slit eyes, gentle herbivore"
P[patina_grazer]="a placid cow-sized grazer built like a manatee, a rotund bulbous blunt-headed body and broad paddle tail, its leathery hide and natural shell scutes furred over with symbiotic blue-green lichen like verdigris on old bronze, two grasping fore-flippers ending in clamp-like claws curled beneath it, a rasping mouth, small lidded eyes, gentle herbivore"
P[soot_grazer]="a placid cow-sized grazer built like a manatee, a rotund bulbous blunt-headed body and broad paddle tail, matte black from head to tail in velvety radiation-drinking hide, natural chitin scutes along the back, two grasping fore-flippers ending in clamp-like claws curled beneath it, fine ash drifting off its flanks, gentle herbivore"
P[bullionaut]="a heavy diving-beetle-like grazer with a natural chitin shell gleaming metallic gold like a jewel beetle, faceted golden wing-cases, paddle-fringed hind legs and four thick grasping forelegs curled beneath as if wrapped around a boulder, a crushing ore-grinding mouth, dense and slow"
P[carrion_moth]="a pale scavenger drifting weightless, broad tattered dust-grey sensory vanes spread flat like a sail rather than wings, feathery antennae, a long coiled proboscis, two small hooked forelimbs for gripping carrion, ragged and sombre"
P[cauldronback]="a hulking beast whose back is a basin of fused black slag holding a glowing permanent melt-pool, basalt-hard grasping pillar-legs curled beneath in a clamping posture, caldera glow spilling over the shell rim"
P[coronid]="a ring of arcing golden plasma loops orbiting a fist of dark degenerate matter, no flesh at all, erupting and rejoining loops of fire like solar prominences, a living storm drifting in space"
P[crusher_mantis]="a massive mantis-shrimp-like predator with thick ore-grey chitin, a segmented body with swimmerets along the tail, hooked anchoring legs curled beneath to clamp onto rock, enormous spiked crushing raptorial forelimbs cocked to strike, stalked compound eyes, braced and coiled"
P[facet_druse]="a crystalline grazer whose whole body is a cluster of quartz-like druse facets grown like a geode, geometric mineral grasping limbs curled beneath in a clamping posture, glittering internal light, angular and slow"
P[glintfin]="a small darting belt creature, streamlined body with mirror-bright metallic fins held like sails, a rear jet vent, tiny hooked claws tucked beneath for pushing off pebbles, flashing chrome scales"
P[glitterback_crab]="a dog-sized crab with a shell crusted thick in undigested glittering crystals, one heavy crusher claw and one fine manipulator claw, eight legs curled beneath in a gripping posture, camouflaged as a mineral vein"
P[inkwyrm]="a shuttle-sized boneless worm with a light-eating vantablack filament pelt, the darkest surface imaginable, body coiled in a loose spiral as if boring into rock, a single chrome rosette of rotary teeth at the head, its only shine"
P[rivetshell]="a stout dome-shelled grazer like a giant chiton or horseshoe crab, a natural rust-brown keratin shell of overlapping plates studded with rows of round bony knobs like rivets, many short gripping legs curled beneath the shell rim in a clamping posture, a small blunt head, slow herbivore"
P[silica_borer]="a tunnelling grub-like borer, a glassy silica drill head, segmented translucent body packed with ground rock, short hooked legs along the body for hugging stone, curled slightly"
P[slag_tortoise]="a hill-sized tortoise-like grazer with a single foamed-slag dome shell meters thick, layered like poured lava, an under-slung furnace mouth glowing orange, thick clamping limbs half drawn in"
# --- ice field: clingers and lickers
P[deuteron_slug]="a fat glossy slug grazer, frost-blue translucent flesh with a faint inner glow, ice crystals rimming its mantle, a broad muscular foot curled as if clamped to ice, a rasping mouth, trailing a frozen slime sheen"
P[frost_lurker]="a large pale pike-like ambush predator, long low body in rime-white hide with a natural frost-crusted ridge along the spine, two hooked clawed forelimbs curled to cling to ice, a powerful finned tail coiled to launch, glowing blue eyes, needle teeth"
P[frost_moth]="a pale ice-field grazer with broad frost-white sail-vanes spread flat to catch sunlight, not wings, delicate grasping legs curled beneath for clamping to ice, a long proboscis for licking meltwater, fine antennae"
P[hollow_pilgrim]="a gaunt hooded creature hauling itself through space hand-over-hand, long many-jointed grasping limbs reaching forward, a hollow shell body like a cowl of pale bone, an empty dark interior, slow and eerie"
P[pressblister]="a squat sea-cucumber-like ice-field grazer covered in pressure-swollen blister domes, thick bulging hide, rows of stubby tube-feet along its underside curled for clamping to ice, a wide licking mouth ringed with short feeding tentacles, taut and lumpy"
P[rime_grazer]="a hardy walrus-like grazer, a rotund bulbous blubbery body in thick white fur crusted with rime frost, ice crystals hanging from its whiskered muzzle, two curved tusks for hooking into ice, two strong clawed fore-flippers curled beneath it for clamping, a broad paddle tail, breath of freezing mist, gentle herbivore"
# --- gas cloud and nebula: filters, siphons, slow strokes
P[chlorophage]="a pale green membranous balloon-bodied grazer, chlorophyll-green veins, a wide intake mouth flared open inhaling a wisp of glowing vapor, trailing filter fronds, no limbs, small rear jet vents"
P[cinder_sylph]="a slender ember-glowing drifter, a flaring intake siphon at the head, long trailing filter-fronds glowing like embers in place of wings, thin capture-tendrils, no legs, riding a puff of vented gas"
P[etchmaw]="a squat gas-cloud grazer, a wide corrosive maw gaping open inhaling a stream of vapor, acid-pitted lips, mottled green-grey hide, no legs, broad paddle fins and rear jet vents, chemical fumes wisping"
P[halo_drifter]="a luminous ring-shaped drifter, a glowing rim with fine trailing filter filaments hanging inward, translucent, serene, no limbs"
P[nullbubble]="a near-transparent spherical creature, a faint dark void at its core, a shimmering iridescent membrane, tiny drifting cilia, no limbs"
P[pall_jelly]="a shrouded dark grey jellyfish-like scavenger, a drooping funeral-veil bell, long pale capture-tendrils trailing, pulsing slowly, dim and mournful"
P[phase_lurker]="a half-phased translucent serpentine drifter, long body in a slow open S-wave, flesh flickering between solid violet and ghostly transparency, shimmering unstable edges, faint afterimages, no legs"
P[prism_drifter]="a floating grazer with a faceted crystal body refracting rainbow light, slender light-beam emitter fins, intake vents drawing in glowing vapor, no limbs, glowing from within"
P[sift_ray]="a soft ray-shaped grazer, broad dusky violet wing membranes caught mid slow-stroke, a wide sieve-like mesh mouth straining dust from the murk, a slender tail, gentle"
P[veil_ray]="a gossamer ray, veil-like translucent wing membranes caught mid slow-stroke, long diaphanous streamers trailing, lit from within by nebula light, ethereal"
P[voltmanta]="a manta-shaped grazer, dark slate wing membranes caught mid slow-stroke, electric blue arcs dancing along its wing edges, a wide intake mouth inhaling gas, crackling with stored charge"

SPECIES=$(printf '%s\n' "${!P[@]}" | sort)
i=0
for sp in $SPECIES; do
  base=$((10100 + 10*i)); i=$((i+1))
  for j in 0 1 2; do
    seed=$((base + j))
    f="$OUT/${sp}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -s "$seed" -o "$f" "${P[$sp]}, $STYLE" || echo "FAILED $sp $seed"
  done
done
echo DONE
