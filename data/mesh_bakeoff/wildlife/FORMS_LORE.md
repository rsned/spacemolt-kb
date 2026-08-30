# Wildlife lore — bodies, feeding, defenses

Codex-voice entries for the fauna exploration. Every entry answers the
same three questions: **Changed** — what the body altered from its
familiar terrestrial baseline to survive vacuum, radiation, and thermal
whiplash; **Feeds** — the mechanism by which it consumes its specific
resource (ores, gases, ice, radiation, other fauna); **Defends** — what
it evolved instead of running home to a burrow. Voice matches the item
catalog: field-guide practical, a hunter's or rancher's view.

Part 1 covers the live roster (scraped `wildlife_species` + the
gas-cloud POI species). Part 2 covers the "beyond earth-equivalents"
exotic form hypotheses (`gen_forms.sh`).

Habitat and role tags in Part 1 are the scanned values, so they win over
the prose where the two ever drift apart. Since server v0.571.0 a creature
scan also returns the species' official codex entry (`description`); where
one has been read it is quoted at the top of the entry, in italics, and the
prose below follows it — the codex is canon, the field notes are ours. Where combat is described it
comes from `wildlife_attacks`, which is thin — five species have ever
been recorded swinging at anything. Two gaps worth knowing about:
**Tempest-Eel** is here on the strength of the gas-cloud POI listing
alone and has never appeared in `wildlife_species`; and eight scanned
species still have no entry — ash-scarab, carrion-moth, crusher-mantis,
frost-moth, hollow pilgrim, pressblister, prism-drifter and rime-grazer.

---

## Design rules — bodies in free fall

Everything on this roster lives, feeds and moves in open space. There is no
ground and no down: almost nothing here keeps a gravity orientation, and a
form should show how the animal gets around and what it holds onto.

- **Space is thin, not empty.** Broad membranes still propel, because they
  shove enough of the sparse medium past them — the rays fly, on wide slow
  strokes. Serpentine bodies work the same way and just as slowly: with
  little to push against, an eel's S-wave is a cruise, never a sprint.
- **Lift is pointless.** Nothing needs to fight gravity, so moth and
  butterfly planforms are unlikely. Where a species carries big flat
  surfaces they are collectors, radiators, sails or sensor vanes, not
  aerofoils.
- **Rock and ice are solid.** Belt and ice-field species have limbs that
  grasp, clamp and hold the boulder or berg they are eating; they travel by
  pushing off it or hauling hand-over-hand, and rest by clinging. Hooves
  and walking gaits are the wrong picture — a stock grazer is a beast
  latched onto a rock, not standing on one.
- **Gas clouds have no surface.** Cloud-only species have nothing to push
  against and nothing to hold, so limbs, where they exist at all, are for
  capturing prey. Feeding is filtering, inhaling, ingesting or straining the
  gas itself; travel is venting, jetting or undulating through the medium.
- **Think aquatic.** Any body plan that moves through water works in space
  at some scale — fish, rays, eels, jellies, squid, crabs, mantis shrimp,
  sea cows. Four-legged land forms do not: no cows, no hooves, no gaits.
  The stock grazers are manatees at heart — rotund, bulbous base forms
  with paddle tails and grasping fore-flippers, and grinding mouthparts —
  broad rasp-plated muzzles, never tusks, because rock and ice are ground
  down, not hooked. That is also the ranch
  roster: the scan flags the four manatee grazers (belt, patina, soot,
  rime), the two rays (drift, sift) and the facet-druse as ranchable stock.
- **Habitat makes the body.** A species scanned in `asteroid_belt`,
  `ice_field`, `gas_cloud` or `nebula` got its shape from eating what is
  there and moving how that place allows.

---

## Part 1 — the known roster

**Belt-Grazer** *(asteroid_belt · grazer · ranchable)*
The baseline stock animal of the belts, and the reason "grazer" means
anything out here: a placid, cow-sized armored grazer built like a
manatee — rotund, bulbous, blunt-headed, a broad paddle tail for slow strokes
and two grasping magnetized fore-flippers that latch onto asteroid
surfaces and hold the rock it eats.
- **Changed:** lungs traded for sealed fermentation stomachs; skin for
  a molting pressure-carapace (the shed plates are the familiar
  UV-hardened `creature_carapace` drop); eyes lidded down to
  radiation-slit pupils.
- **Feeds:** rasps oxide crusts and ice-bearing regolith straight off
  the rock with a diamond-file tongue, gut-smelting it — the iron-rich
  residue is why its milk tastes faintly of blood and sells anyway.
- **Defends:** nothing but armor and arithmetic. It clamps down,
  seals, and is a rock; herds scatter in all directions so a predator
  gets one, not five. Ranch brands don't change that math.

**Patina-Grazer** *(asteroid_belt · grazer · ranchable)*
*Codex (scanned, v0.571.0): "Green-crusted and unhurried, it combs copper
dust from the rubble with its filter-plates spread wide, laying the metal it
strains down over its own shell until the plating weathers to verdigris. The
crust is not for fighting — it is simply what happens when a creature filters
copper out of the rock for long enough. Where a belt's copper runs thin the
herd drifts on, and a worked-out field loses its grazers for good."*
A belt-grazer lineage gone verdigris: the same rotund manatee body, but its
feeding plates comb copper dust out of the rubble and the strained metal is
laid down over the shell, layer on layer, until the plating weathers green.
- **Changed:** the rasp-plated muzzle became a fan of filter-plates that
  spread wide to comb dust rather than grind rock; the shell became a
  slow-growing copper deposit, the animal plating itself with what it eats.
- **Feeds:** strains copper dust from worked rubble with the filter-plates
  spread, clamped to the rock by its fore-flippers, and drifts on with the
  herd when a belt's copper runs thin — a worked-out field loses its grazers
  for good.
- **Defends:** nothing on purpose. The verdigris crust is a by-product, not
  armour, and the herd's answer to a predator is the belt-grazer's: clamp
  down, seal, and be a rock.

**Soot-Grazer** *(asteroid_belt · grazer · ranchable)*
The carbon-lane cousin, matte black head to hoof.
- **Changed:** carapace loaded with radiation-drinking melanin — it
  grazes sun-blasted faces no other stock can stand, and its hide runs
  warm to the touch.
- **Feeds:** strips carbonaceous chondrite, spitting back a fine ash
  that marks its trails like inverse snow.
- **Defends:** vents that stored heat when threatened — a shimmering
  scald-halo that convinces most predators the herd is on fire.

**Glitterback Crab** *(asteroid_belt · grazer)*
A scuttling ore-sorter the size of a dog, shell crusted with the
crystals it couldn't digest.
- **Changed:** claws became assay tools — one crusher, one fine
  manipulator; book-lungs became sealed trachea running on stored
  oxygen it cracks from ice.
- **Feeds:** picks through rubble for mineral fines, tasting by
  conductivity, packing rejects onto its own back as it goes.
- **Defends:** the glitter is armor and camouflage in one — against
  rock it reads as a mineral vein, and the crystal crust spalls off in
  a blinding reflective cloud when struck, covering a surprisingly
  fast reverse scuttle.

**Slag-Tortoise** *(asteroid_belt · grazer)*
Half again a belt-grazer's bulk and a fraction of its hurry.
- **Changed:** shell fused into a single foamed-slag dome, meters
  thick in places — the animal secretes it molten and lets vacuum
  freeze it. There is no molt; it grows by pouring on another layer.
- **Feeds:** an under-slung furnace mouth that lips over ore seams and
  cooks them loose at contact temperature, sipping the melt.
- **Defends:** is a hill. Pulled limbs in, a mature slag-tortoise has
  survived deliberate mining charges; the recorded counter-attack is
  it sitting on the drill.

**Inkwyrm** *(asteroid_belt · grazer)*
The darkest surface ever recorded in the belts, met as perfectly round
holes long before you meet the animal.
- **Changed:** hide became a light-eating pelt of vantablack filaments
  that drinks the radiation bath as calories; the skeleton dissolved
  entirely for burrowing — it is a muscular hydrostat, a worm the size
  of a shuttle.
- **Feeds:** a chrome rosette of rotary teeth — the only shine on it —
  bores nesting spirals through carbonaceous rock, gut-sorting the
  spoil; the pelt handles dessert.
- **Defends:** disappears. In shadow an inkwyrm is a hole in the
  sensor return; in rock it is gone. The molted pelt ribbons still eat
  light, which is why sensor-baffle weavers pay by the meter.

**Cauldronback** *(asteroid_belt · grazer)*
The belt's answer to keeping warm: stop trying, become the furnace.
- **Changed:** back became a basin of fused slag holding a permanent
  melt-pool; legs became basalt-hard grasping pillars that clamp the rock
  and don't feel the spill.
  The caldera glow is digestion, visible for kilometers.
- **Feeds:** swallows thorium-rich gravel whole and lets the pool do
  the work — the animal is a walking crucible with a commute.
- **Defends:** flicks. A threatened cauldronback snaps its shell-rim
  and slings arcs of magma with shocking accuracy. Ranchers tap the
  melt for crucible flux from the SIDE, living — the pool scabs to
  worthless rock a day after death.

**Coronid** *(asteroid_belt · grazer)*
Scans as weather, not wildlife.
- **Changed:** everything — no tissue at all. A ring of arcing golden
  plasma around a fist of degenerate matter; the erupting, rejoining
  loops are limbs or organs or the animal, and taxonomy stopped
  voting.
- **Feeds:** grazes charge gradients off the stellar wind, fattening
  visibly before a flare.
- **Defends:** is un-hittable by anything that isn't a ground. Hull
  weapons pass through; what kills one is earthing it, and the drop is
  the nugget — still warm, still humming.

**Pilot-Whale** *(gas_cloud · grazer)*
The herd animal of the argon clouds, and the closest thing the deep
sky has to livestock you'd recognize.
- **Changed:** blowhole and lungs became a flow-through siphon gallery
  — the animal never holds breath because it never stops breathing
  cloud; blubber became a layered gas-bladder hull that is buoyancy,
  insulation, and armor at once.
- **Feeds:** rows of glowing electrostatic baleen vents along the jaw
  charge the vapor and comb the ionized fraction inward — it inhales
  its pasture, and the pod's wake is a clean lane through the cloud.
- **Defends:** the pod. Adults ring the calves and vent stored gas in
  a coordinated squall that blinds sensors and shoves small predators
  tumbling. Whale oil is rendered from the bladder walls, which is why
  the trade is regulated by every empire that borders a cloud.

**Bell-Jelly** *(gas_cloud · grazer)*
A drifting glass bell with an amber heart, harmless in every way that
matters and famous in every galley that matters.
- **Changed:** water-jet propulsion became gas-jet; the bell membrane
  became a one-molecule-thick pressure vessel that flexes instead of
  freezing; the sting was traded away entirely.
- **Feeds:** fans its crystalline tendrils into a charged drift-net
  that strains aerosol ice and organics from the murk, spiraling the
  catch up into the glowing core to be digested by slow light.
- **Defends:** transparency, numbers, and worthlessness — a predator
  that bites one gets a mouthful of cold gas. Blooms simply re-form
  around the loss. The amber cores, though, are why jelly-tenders
  exist.

**Tempest-Eel** *(gas_cloud · predator)*
The reason cloud-divers file flight plans.
- **Changed:** swim bladder became a capacitor chain running the length
  of the spine — the animal is a living transmission line; scales
  became storm-grey dielectric plates.
- **Feeds:** hunts by lightning. It grounds itself against whatever the
  prey floats near and discharges through them, then swallows the
  cooked result whole; the needle teeth are for gripping, not
  chewing.
- **Defends:** rarely needs to — but a cornered tempest-eel dumps its
  whole capacitor chain in one arc that can brown out a shuttle's
  avionics, and spends the next hour dangerous only by reputation.

**Drift-Ray** *(gas_cloud · grazer)*
The glider of the cloud margins, all wing and no hurry.
- **Changed:** cartilage became a rigid-light spar lattice; the wing
  became a solar-and-static membrane that trickle-charges the animal
  as it flies; gills became a wide intake scoop.
- **Feeds:** skims vapor banks with the scoop flared, electrostatic
  mesh across the mouth combing out ice and organics — a living
  ram-scoop on a lazy circuit.
- **Defends:** the bioluminescent spot-rows are a counting trick —
  they fire in patterns that make one ray read as three on a
  predator's motion sense. Failing that, it folds its wings and
  drops like a dead leaf, which in a cloud is functionally
  vanishing.

**Rainbow Leviathan** *(asteroid_belt · predator)*
The grandest of the void-leviathans, and the single rarest plate in
the galaxy. The scan files it as "predator" like any other, which
undersells it: 2,200 hull against 320 for the next largest thing in
the belts, and the only apex the roster has.
- **Changed:** the lobster plan scaled to cruiser size because nothing
  says it can't: molt-armor became the prismatic carapace, light-
  fracturing lattice laced with the exotic metals of a lifetime of
  hunting exotic-fed fauna.
- **Feeds:** ranges off its own belts after whatever concentrates rare
  compounds — prism-drifters in the gas clouds, frost-moths out on the
  ice fields — and prys, crushes and shears with serrated claws that
  work like hydraulic cutters. The shimmer in its flesh is its diet.
- **Defends:** attacks — but not with the claws, which are cutlery.
  It fires the carapace. The one engagement on record is two energy
  beams, both on target, 130 apiece: the same trick its prey uses,
  since a prism-drifter beams too, only feebly, run back through a
  cruiser's worth of light-fracturing lattice. The shimmer is the
  diet and the diet is the weapon. It carries no shields whatever, so
  it simply takes the hit and answers out of that enormous hull; the
  molted shell (`prismatic_carapace`) is the armour it no longer
  needs, shed and sold. "Attack on sight" in the survey notes is
  written from experience — six sightings, no kills.

---

## Part 2 — exotic form hypotheses

**Tessellate** *(regolith flats · grazer)*
A herd pretending to be a hide: hundreds of hexagonal chitin plates,
each a complete dinner-tray organism, flying as one sheet.
- **Changed:** the individual shrank and the colony became the body —
  there is no organ the sheet cannot lose.
- **Feeds:** ripples across sun-shadowed rock, each plate's
  grinding-foot stripping algal frost; the sheet digests communally
  through edge-contact.
- **Defends:** re-tessellates — holes close, edges curl, and the
  carpet folds into a faceted ball, every foot facing out. Kill-drops
  are plates; the sheet closes the gap and grazes on, slightly
  smaller.

**Orrery** *(deep gravity wells · grazer)*
The only known animal whose skeleton is orbital mechanics: polished
stone organs, none touching, circling an amber nucleus in nested
rings.
- **Changed:** connective tissue abandoned for gravitation — the
  nucleus's improbable pull is ligament, spine, and heartbeat.
- **Feeds:** shepherds dust into its lanes and mills it to powder
  between passing organs.
- **Defends:** hard to hurt something with no body to hit — but
  perturb its orbits and it dies by collision, which is why the
  recorded deaths are all careless ships. Hunters price the nucleus
  as a heartcore; the rings, unanchored, simply drift apart.

**Chandelier** *(gas_cloud upper layers · grazer)*
An inverted crystal chandelier hanging from its own buoyant crown.
- **Changed:** rooted plant logic in an animal — a neon-fractionating
  bladder for a head and mineral stalactite arms that grow downward,
  forking into hundreds of prism tips.
- **Feeds:** each tip condenses a different compound out of the murk;
  the animal is a living fractionating column and the glow along its
  arms is chemistry, not blood.
- **Defends:** cannot flee, doesn't need to — bite a chandelier and
  you get a mouthful of whatever it was distilling. Harvesters clip
  tips like fruit and let the crown regrow them.

**Möbius** *(dust lanes · grazer)*
One ribbon of banded muscle and shell, half-twisted, joined to itself,
rolling forever along its own single surface.
- **Changed:** front, back, mouth, and gut merged into topology — the
  twist IS the digestive fold.
- **Feeds:** edge-cilia rake dust onto the band as it tumbles; the
  band carries the meal through itself. Mouth, stomach, and wheel are
  the same organ.
- **Defends:** no front to attack, no back to flank, and a predator
  that swallows one whole discovers the roll does not stop.

**Barnacle-Moon** *(asteroid_belt · ambush grazer)*
Most rocks in a belt are rocks. This is the other kind.
- **Changed:** shell learned to accrete genuine local regolith until
  the animal is metallurgically identical to its neighborhood —
  camouflage that passes an assay.
- **Feeds:** holds station for years; when a mineral-rich fragment
  drifts close, the crust splits along hidden seams into radial petal
  jaws and takes it.
- **Defends:** patience and provenance. Its carapace plate sells
  precisely because it scans as ordinary rock — the defense and the
  market are the same trait.

**Filament** *(magnetic field lines · grazer)*
A kilometer of animal and a meter of body: one glowing neural thread
strung with bead-organs, ends frayed into field-gripping sail fibers.
- **Changed:** the body plan gave up bulk for length — it wears its
  planet's magnetosphere the way other animals wear skin.
- **Feeds:** harvests induced current from the field lines it rides,
  growing by adding beads.
- **Defends:** cut it and both halves live — ranchers exploit it, and
  it's why an old filament's beads don't match: some of it has been
  several animals. Charts mark the big ones like shoals.

**Dustveil** *(nebula margins · swarm grazer)*
A permanent sandstorm that learned to want things.
- **Changed:** the organism dissolved into thousands of magnetized
  motes flying as one translucent membrane, herded by a glowing knot
  of core-motes that carries whatever passes for its mind.
- **Feeds:** engulfs — wraps an ice shard or ore fleck and mills it to
  nothing between charged grains.
- **Defends:** weapons fire re-arranges it. The practical kill is the
  knot; the practical harvest is flying through slowly with charged
  plates and stealing the veil a gram at a time.

---

**Gorgonia** — unique boss, The Maw, Alzirr; canonical text kept in
the game's own words (black coral fan rooted in the accretion disk,
darksteel and fury-crystal skeleton, drinks the radiation, only one of
its kind — starve it by stripping the ring, or kill it and there is no
second). Heroes: `heroes-raw/form_gorgonia_*`.
