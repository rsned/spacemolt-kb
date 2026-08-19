#!/usr/bin/env python3
"""Empire-themed station style exploration — FLUX seed grid.

6 empires x 10 shared archetypes x 2 sizes, plus 3 pirate identities x 2
sizes = 126 renders. Hero conventions throughout (flat magenta, complete
silhouette, three-quarter) so any winner can go straight into the Hy3D
footprint pipeline. Design DNA distilled from the user's station animation
mockup (tiered plating shelves, banded lit decks, greeble); empire styling
from the established passenger/ship style guides. Deterministic seeds:
5000 + empire*100 + archetype*10 + size. Skip-existing, so re-runs resume.

    python3 gen_stations.py            # all missing renders
    python3 gen_stations.py solarian   # one empire (or 'pirate')
"""

import os
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
OUT = HERE / "renders"
KB = HERE.parents[2]

BASE = ("colossal space station, centered, complete silhouette fully in frame, "
        "three-quarter view from slightly above, isolated on a completely flat "
        "solid magenta background, uniform bright magenta backdrop, sci-fi "
        "videogame concept art, dramatic key light, crisp silhouette, dense "
        "tiered hull plating in stepped shelves, bands of tiny lit deck "
        "windows, fine greebled panel detail, ultra high detail")

EMPIRES = {
    "solarian": ("Solarian Empire style: classic gleaming white and silver hull, "
                 "clean stately elegant lines, restrained gold trim accents, "
                 "polished ceramic panels, dignified flagship grandeur"),
    "outerrim": ("Outer Rim style: junkyard-chic scrap construction, mismatched "
                 "salvaged hull plates in clashing colours, duct-tape seams and "
                 "chicken-wire mesh guards, exposed rusty trusses, welded "
                 "patchwork, jury-rigged antenna clusters, oil streaks"),
    "nebula": ("Nebula Empire style: gauche opulent luxury, sleek mirror-polished "
               "gold and platinum surfaces, ostentatious filigree ribbing, "
               "jewel-toned glass inlays, extravagant yacht-like curves"),
    "crimson": ("Crimson Empire style: militaristic and utilitarian, dark "
                "gunmetal armour plating with deep crimson livery bands, angular "
                "bastion geometry, turret batteries and armoured shutters, "
                "zero ornament, fortress menace"),
    "voidborn": ("Voidborn style: exotic otherworldly grown structure, "
                 "iridescent bone-white lattice weave, organic curving spars, "
                 "translucent membrane panels with faint violet glow, "
                 "asymmetric alien elegance"),
    "player": ("player-built ModernSpaceIKEA style: modular flat-pack prefab "
               "sections in bright clean white and pale birch-tone panels, "
               "colour-coded connector frames, cheerful minimalist Scandinavian "
               "design, standardized snap-together modules, showroom clean"),
}

# same ten silhouette archetypes for every empire, classic-space-theme nods
ARCHETYPES = {
    "saucer": "a mega-saucer station, broad tiered flying-saucer disc with stepped deck shelves and a central hub spire",
    "ring": "a giant ring station, one vast slender annular ring with an open centre, habitat band on the inner face",
    "wheel": "a classic wheel-and-spoke station, rotating torus rim connected by radial spokes to a central docking hub",
    "drum": "an enormous spin-drum colony station, massive rotating cylinder hull with longitudinal ribs and end-cap docks",
    "pylon_toroid": "a toroidal docking station with tall curved docking pylons arcing above and below the ring plane",
    "terraces": "an organic-architecture station of broad cantilevered horizontal terraces stacked asymmetrically, long low prairie-style decks around a low central mass",
    "spindle": "a tall slender spindle station, thin central spine crowned with a hemispherical dome, mid-spine collar rings and protruding gantry arms",
    "rock": "a hollowed asteroid station, habitat towers and glowing docking bays embedded in a carved rocky shell",
    "gantry": "an open scaffold drydock station, exposed lattice gantry frames enclosing ship berths and hangar boxes",
    "pagoda": "a stacked-disc pagoda station, tiers of discs of shrinking diameter climbing a central column",
}

SIZES = {
    "moderate": "moderate outpost scale, compact tidy structure, a few docking arms",
    "capital": ("immense capital-class megastructure scale, hull crusted with a "
                "city skyline of towers, thousands of lit decks, sprawling "
                "docking arms"),
}

# pirate faction identities (kept alongside the strongholds): the two
# groups attested in the KB plus the freeport junkyard/IKEA mix
PIRATES = {
    "void_reapers": ("Void Reapers pirate clan station, a grim charnel fortress "
                     "welded from captured warship carcasses, scorched black "
                     "hulls, jagged reaper-blade pylons, trophy wreckage chained "
                     "to the hull, baleful red floodlights",
                     "rock"),
    "crimson_corsairs": ("Crimson Corsairs pirate den, sleek stolen military "
                         "hulls lashed into a raider haven, repainted "
                         "crimson-and-black corsair livery, emblem banners, "
                         "gun-bristled docking spars, fast launch rails",
                         "gantry"),
    "free_haven": ("freeport smuggler haven, chaotic half-junkyard "
                   "half-prefab-showroom station, rusty scrap trusses bolted to "
                   "clean white flat-pack modules, neon bazaar signage, "
                   "mismatched habitat pods, tangled fuel lines",
                   "pagoda"),
}

# the nine pirate strongholds — canonical base names + lore from the KB
# `bases` table (join to systems where is_stronghold=1); styling derives
# from each lord's one-line description. 3 seed variations each.
PIRATE_DNA = ("pirate stronghold construction, hull assembled from salvaged "
              "and captured ship sections, visible weld seams, mounted gun "
              "emplacements, trophy hull plates")
STRONGHOLDS = {
    "crix_stronghold": ("Bellatrix — Sovereign Crix",
                        "an unnervingly orderly pirate station, salvaged hull sections arranged in neat "
                        "symmetric rows with parade-ground discipline, matched paint over the welds, "
                        "clean docking queues, a tidy fortress built from stolen parts", "wheel"),
    "voss_redoubt": ("Alhena — Commandant Voss",
                     "a fortified redoubt whose armour walls are a quilt of hull plating from a dozen "
                     "different ship classes in clashing colours and profiles, layered bastion rings, "
                     "one heavily guarded entry corridor", "rock"),
    "kael_arsenal": ("Xamidimura — Admiral Kael",
                     "a weapons depot and fleet anchorage, open gantry berths crowded with warships, "
                     "ammunition silos and missile racks, fuel lines everywhere, scorched launch rails", "gantry"),
    "korr_fortress": ("Gliese 581 — Grand Marshal Korr",
                      "the oldest and largest pirate fortress, the archetype every other stronghold "
                      "copies, generations of accreted construction in concentric fortress layers, "
                      "ancient battle-scarred core hull inside newer armour shells", "pagoda"),
    "sable_port": ("Barnard 44 — Director Sable",
                   "a coldly professional trade port, sleek dark polished grey-black hull, corporate "
                   "minimalism with discreet gun batteries, orderly cargo terminals, no decoration, "
                   "everything profitable", "ring"),
    "thane_keep": ("Sheratan — Warlord Thane",
                   "a brutalist armoured keep, massive thick sloped armour slabs, minimal windows, "
                   "squat no-nonsense bunker geometry, overlapping point-defence turrets", "drum"),
    "mera_sanctum": ("Zaniah — Archon Mera",
                     "a concealed station built to hide in a sensor shadow, matte light-absorbing "
                     "black hull, low-emission slit windows, asymmetric shard-like silhouette, "
                     "quiet and dangerous", "terraces"),
    "dross_citadel": ("Algol — Imperator Dross",
                      "a cramped hyper-efficient citadel, machinery and habitat modules packed dense "
                      "with zero wasted volume, exposed ducting, purpose-built and unapologetic", "spindle"),
    "nyx_nexus": ("GSC-0008 — Overlord Nyx",
                  "a remote isolation stronghold, one austere spire with sparse docking, immense "
                  "long-range antenna arrays, dark hull with a few cold lights, deliberate solitude", "spindle"),
}


def render(name: str, seed: int, prompt: str) -> None:
    f = OUT / f"{name}_s{seed}.png"
    if f.exists():
        print(f"skip {f.name}")
        return
    r = subprocess.run([str(KB / "tools/portrait"), "-s", str(seed), "-o", str(f), prompt],
                       env={**os.environ, "PORTRAIT_SIZE": "1024"})
    print(("ok   " if r.returncode == 0 else "FAIL ") + f.name, flush=True)


def main() -> int:
    OUT.mkdir(exist_ok=True)
    only = sys.argv[1] if len(sys.argv) > 1 else None
    for ei, (emp, estyle) in enumerate(EMPIRES.items()):
        if only and only != emp:
            continue
        for ai, (arch, adesc) in enumerate(ARCHETYPES.items()):
            for si, (size, sdesc) in enumerate(SIZES.items()):
                seed = 5000 + ei * 100 + ai * 10 + si
                render(f"{emp}__{arch}__{size}", seed,
                       f"{adesc}, {estyle}, {sdesc}, {BASE}")
    if not only or only == "pirate":
        for pi, (pid, (pstyle, arch)) in enumerate(PIRATES.items()):
            for si, (size, sdesc) in enumerate(SIZES.items()):
                seed = 5900 + pi * 10 + si
                render(f"pirate_{pid}__{arch}__{size}", seed,
                       f"{ARCHETYPES[arch]}, {pstyle}, {sdesc}, {BASE}")
        for pi, (pid, (_, pstyle, arch)) in enumerate(STRONGHOLDS.items()):
            for i in range(3):
                seed = 6000 + pi * 10 + i
                render(f"stronghold_{pid}__{arch}__v{i}", seed,
                       f"{ARCHETYPES[arch]}, {pstyle}, {PIRATE_DNA}, {BASE}")
    print("DONE")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
