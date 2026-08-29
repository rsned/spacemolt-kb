#!/bin/bash
# Round 3: the rest of the scanned roster -- every wildlife_species entry that
# rounds 1-2 did not cover (33 species, 3 seeds each). Same contract as before:
# flat magenta backdrop so run_hy3d's chroma_key consumes these unchanged, and
# the round-2 pose rule (the footprint is a top-down battle asset, so the body
# must read from a high three-quarter overhead angle, level and elongated).
#
# Prompts: the eight species with FORMS_LORE.md entries follow their lore
# (belt/patina/soot grazer, glitterback crab, slag-tortoise, inkwyrm,
# cauldronback, coronid); the rest are built from the scan data alone --
# class, habitat, hull -- and the name. Lore for those is still to be written,
# so treat their looks as first proposals.
#
# The isolation cues lead the prompt and -c (compel) encodes past CLIP's
# 77-token cap, so the backdrop instruction is never truncated away.
#
# Seeds: 9700 + 10*i for species i in the alphabetical list below (9700..10020),
# clear of the 92xx/93xx/94xx/95xx/96xx blocks already used.
set -u
cd /home/robert/spacemolt/kb
OUT=data/mesh_bakeoff/wildlife/heroes-raw
mkdir -p "$OUT"

STYLE="single creature centered, floating in empty space, isolated on a completely flat solid magenta background, uniform bright magenta backdrop, full body seen from a high three-quarter overhead angle showing the complete body plan, body level and elongated, sci-fi videogame concept art, dramatic rim lighting, crisp silhouette, high detail"

declare -A P
P[ash_scarab]="a hand-sized armored scarab beetle scavenger, matte grey ash-dusted carapace, folded wing-cases, carcass-stripping mandibles, six barbed legs, low and compact"
P[belt_grazer]="a placid cow-sized armored quadruped stock animal, molting pressure-carapace of overlapping UV-hardened plates, magnetized hooves, lidded radiation-slit eyes, a diamond-file rasping tongue, hooves planted in empty space"
P[bullionaut]="a heavy armored beetle-tortoise grazer plated in faceted bullion-gold shell segments like stacked ingots, stubby digging limbs, a crushing ore mouth, dense and slow"
P[carrion_moth]="a large pale moth scavenger, tattered dust-grey wings dotted with dim eyespots, feathery antennae, a coiled proboscis, ragged and sombre"
P[cauldronback]="a hulking grazer whose back is a basin of fused black slag holding a glowing permanent melt-pool, basalt-pillar legs, caldera glow spilling over the shell rim, arcs of magma flicked from its edge"
P[chlorophage]="a pale green membranous gas-cloud grazer, translucent balloon body threaded with chlorophyll-green veins, trailing filter fronds, small light-drinking lobes, soft and buoyant"
P[cinder_sylph]="a slender ember-coloured winged sylph creature, delicate glowing wing membranes like drifting embers, thin trailing limbs, ash-flecked, faintly luminous"
P[coronid]="a ring of arcing golden plasma loops orbiting a fist of dark degenerate matter, no flesh at all, erupting and rejoining loops of fire like solar prominences, a living storm"
P[crusher_mantis]="a massive armored space mantis predator, thick ore-grey plate carapace, enormous spiked crushing raptorial forelimbs held ready, compound eyes, wings folded flat, braced to strike"
P[deuteron_slug]="a fat glossy slug grazer, frost-blue translucent flesh with a faint inner glow, ice crystals rimming its mantle, a rasping mouth, trailing a frozen slime sheen"
P[etchmaw]="a squat gas-cloud grazer with a wide corrosive maw ringed by acid-etched pitted plates, mottled green-grey hide, drifting fins, chemical fumes wisping from its jaws"
P[facet_druse]="a crystalline grazer whose whole body is a cluster of quartz-like druse facets, geometric mineral limbs, glittering internal light, angular and slow"
P[frost_lurker]="a large pale ice-field ambush predator, long low body armored in rime-white plates, glowing blue eyes, hooked claws, frost-camouflaged, coiled to pounce"
P[frost_moth]="a delicate moth grazer, frost-white feathered wings dusted in ice crystals, pale blue markings, fine antennae, wings spread flat"
P[glintfin]="a small darting fish-like belt grazer, mirror-bright metallic fins and scales that flash like polished chrome, streamlined, quick"
P[glitterback_crab]="a dog-sized scuttling crab, shell crusted thick with undigested glittering crystals, one heavy crusher claw and one fine manipulator claw, armored legs, camouflaged as a mineral vein"
P[halo_drifter]="a drifting ring-shaped gas-cloud grazer, a luminous halo body with a glowing rim and fine trailing filaments, serene and translucent"
P[hollow_pilgrim]="a gaunt hooded ice-field wanderer creature, a tall hollow shell body like a walking cowl of pale bone plates, slow stilt limbs, an empty dark interior"
P[inkwyrm]="a shuttle-sized boneless worm with a light-eating vantablack filament pelt, the darkest surface imaginable, a single chrome rosette of rotary teeth at the head, its only shine"
P[nullbubble]="a near-transparent spherical bubble creature, a faint dark void at its core, a shimmering iridescent membrane, tiny drifting cilia"
P[pall_jelly]="a shrouded dark grey scavenger jellyfish, drooping funeral-veil bell, long pale trailing tendrils, dim and mournful"
P[patina_grazer]="a cow-sized armored belt-grazer gone verdigris, its carapace plated in blue-green copper-fixing lichen like an old bronze statue, magnetized hooves, radiation-slit eyes"
P[phase_lurker]="a half-phased translucent nebula grazer, body flickering between solid violet flesh and ghostly transparency, shimmering unstable edges, faint afterimages"
P[pressblister]="a squat ice-field grazer covered in pressure-swollen blister domes, thick bulging hide, stubby limbs, taut and lumpy"
P[prism_drifter]="a floating prismatic gas-cloud grazer, faceted crystal body refracting rainbow light, slender light-beam emitter fins, glowing from within"
P[rime_grazer]="an armored quadruped stock animal plated in rime frost, ice-crusted carapace, magnetized hooves, breath of freezing mist, hardy and placid"
P[rivetshell]="a heavy belt grazer whose shell looks like riveted iron plate, bolt-like studs along every seam, industrial rust-brown armor, stubby legs"
P[sift_ray]="a soft ray-shaped grazer with a wide sieve-like mesh mouth, dusky violet wings spread flat, gentle and slow, straining dust from the murk"
P[silica_borer]="a tunneling grub-like borer, glassy silica drill head, segmented translucent body packed with ground rock, boring through stone"
P[slag_tortoise]="a hill-sized tortoise with a single foamed-slag dome shell meters thick, layered like poured lava, an under-slung furnace mouth glowing orange, limbs half drawn in"
P[soot_grazer]="a cow-sized armored belt-grazer matte black from head to hoof, radiation-drinking melanin carapace, hide that glows faintly warm, trailing fine ash"
P[veil_ray]="a gossamer nebula ray, veil-like translucent wings trailing long diaphanous streamers, lit from within by nebula light, ethereal"
P[voltmanta]="a manta-shaped gas-cloud grazer crackling with stored charge, electric blue arcs dancing along its wing edges, dark slate hide, wings spread flat"

SPECIES=$(printf '%s\n' "${!P[@]}" | sort)
i=0
for sp in $SPECIES; do
  base=$((9700 + 10*i)); i=$((i+1))
  for j in 0 1 2; do
    seed=$((base + j))
    f="$OUT/${sp}_s${seed}.png"
    [ -f "$f" ] && { echo "skip $f"; continue; }
    PORTRAIT_SIZE=1024 ./tools/portrait -c -s "$seed" -o "$f" "${P[$sp]}, $STYLE" || echo "FAILED $sp $seed"
  done
done
echo DONE
ls "$OUT" | grep -c '_s9[7-9][0-9][0-9]\.png\|_s100[0-2][0-9]\.png'
