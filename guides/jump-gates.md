---
title: Jump Gates
summary: What a jump gate is, how the recipes are earned, what the parts take, where the rare inputs come from, and how a gate site is founded. Much is still unmeasured.
intro: |
  A **jump gate** is a fold in space held open between two systems: a **Lane Anchor** at each
  end holds the fold, a lot of **Fold Lattice** gives it something to stand on, and **Gate
  Regulators** keep it from drifting. Players build them. The first gate in the galaxy was
  built by another player (*player-reported*), and more are under construction now.

  The recipes are not in the public catalog yet. The numbers below come from our own recipe
  capture after finishing the chain that grants them. As elsewhere in the KB, our own
  observations are labelled *measured*, other players' reports *player-reported*, and the
  official docs *doc-quoted*. This guide is young: everything we have not measured is listed
  as such at the bottom, and the guide grows as we learn.

  > ⚠ **Monoliths.** The rare inputs come from Frontier **Repeaters**. The largest kind, the
  > **Repeater Monolith**, cannot be beaten today: every kill spawns two more, and a fight
  > with them cannot be fled. One pilot fought Monoliths for eight hours, destroyed 354 of
  > them, and was lost (*player-reported*). Stay out of any system where you have seen one.

  > ⚠ **A gate costs money for as long as it stands.** Building it pays labor from your
  > faction treasury every facility cycle, and an open gate needs upkeep at both ends. Budget
  > for it the way you would for a station (see the [station guide](player-stations.html)).
steps:
  - title: Earn the recipes
    facts:
      - {label: Given by, value: "Sinter, Starfall Salvage Station", basis: "measured"}
      - {label: Chain ends, value: "Taking the Machine Apart → Bench Test → The Method", basis: "measured"}
      - {label: Unlocks, value: "Lane Anchor, Lane Anchor Prototype, Fold Lattice, Gate Regulator, Fold Shim", basis: "measured 2026-11-21 (recipe capture); time per run is skill-adjusted"}
    notes: |
      The recipes are granted when you complete **The Method**, the last mission of Sinter's
      chain. Its completion text sums the build up: "A Lane Anchor at each end, holding the
      fold. Fold Lattice, a lot of it, to give the fold something to stand on. Gate Regulators
      to keep it from drifting. The fold wears through its shims, and the shims are seam glass,
      so whoever builds a gate had better plan to keep feeding it. Every empire's materials and
      a lot of the frontier's."

      Earlier in the chain, *Taking the Machine Apart* is where the first [[fold_knot]] turns
      up; the fold knot is the hinge every Lane Anchor is built around.
  - title: The parts
    facts:
      - {label: Lane Anchor, value: "50 seam glass · 1 fold knot · 10 mirror facet · 40 darksteel plating · 8.62 ticks", basis: "measured 2026-11-21 (recipe capture); time per run is skill-adjusted"}
      - {label: Lane Anchor Prototype, value: "10 seam glass · 1 fold knot · 2 mirror facet · 10 darksteel plating · 1.72 ticks", basis: "measured 2026-11-21 (recipe capture); time per run is skill-adjusted"}
      - {label: Fold Lattice, value: "10 seam glass · 2 echo cell · 10 phase matrix · 2.59 ticks", basis: "measured 2026-11-21 (recipe capture); time per run is skill-adjusted"}
      - {label: Gate Regulator, value: "4 echo cell · 10 solarian composite · 5 prismatic lens · 3.45 ticks", basis: "measured 2026-11-21 (recipe capture); time per run is skill-adjusted"}
      - {label: Fold Shim, value: "2 annealed seam glass · 2 circuit board · 2.59 ticks", basis: "measured 2026-11-21 (recipe capture); time per run is skill-adjusted"}
    notes: |
      All five are hand-crafted in a station workshop. Hand crafting charges no labor fee. The
      times above are ticks per run for the crafter who captured them, already sped up by their
      skills; yours will differ.

      - **Lane Anchor:** one at each end of the gate, so two per gate. Each takes a whole
        [[fold_knot]], which is the scarcest input in the whole build. Per the recipe note,
        dismantling a gate returns each fully built end's Lane Anchor.
      - **Lane Anchor Prototype:** a test build with a fifth of the [[seam_glass]] and mirror
        facets and a quarter of the [[darksteel_plating]]. It still needs a full fold knot.
      - **Fold Lattice:** "a lot of it". How many a gate takes is not yet measured. Each one
        uses [[seam_glass]], echo cells and [[phase_matrix]].
      - **Gate Regulator:** keeps the fold from drifting. Uses echo cells,
        [[solarian_composite]] and [[prismatic_lens]].
      - **Fold Shim:** the wear part. The fold wears through its shims, so a standing gate needs
        a steady supply of [[annealed_seam_glass]] and [[circuit_board]].

      The common inputs ([[darksteel_plating]], [[phase_matrix]], [[solarian_composite]],
      [[prismatic_lens]], [[circuit_board]]) are ordinary refined goods. The hard ones are
      [[seam_glass]], echo cells, mirror facets and above all the [[fold_knot]]: see the next
      step.
  - title: The rare inputs
    facts:
      - {label: Repeater Sliver, value: "100 hull / 150 shield · drops 1 mirror facet (plus seam glass or trade crystal, by site)", basis: "measured (wreck reads)"}
      - {label: Repeater Pane, value: "400 / 600 · drops 1 echo cell + 2 mirror facets", basis: "measured (wreck reads)"}
      - {label: Repeater Spire, value: "1,600 / 2,400 · drops 2 echo cells + 4 mirror facets, sometimes a fold knot", basis: "measured (wreck reads); too few kills to give a rate"}
      - {label: Seam glass, value: "about 17,700 across 10 systems", basis: "player-reported"}
    notes: |
      **Repeaters** are Frontier hostiles, crystalline machines found in small groups out past
      charted space. Echo cells, mirror facets and fold knots all come from their wrecks: loot
      the wreck after each kill. The [[fold_knot]] is an uncommon **spire** drop (too few kills
      yet to give a rate), so a gate's two anchors mean a lot of spires.

      Repeaters **reflect damage**. Every hit on a pane sends back a flat amount (about 15)
      as EM damage; a spire sends back a share of the *energy* damage it takes. Kinetic weapons
      are the better choice against spires, and the reflected damage counts against your ship
      even though the repeater never "fires".

      **[[seam_glass]]** is mined, not crafted. It sits in **nebula shells guarded by
      repeaters**: in the deposits we know, every shell of any size had a group sitting on
      it. Deposit sizes vary widely. We are not publishing where they are; look for nebulae with repeaters on them in frontier systems.

      **Working a guarded shell.** Repeaters respawn, so clearing a shell once is not enough.
      The approach that fits is a **heavily tanked miner** that can sit on the shell and take the
      reflected and direct damage, plus an **escort** that kills each repeater as it respawns.
      Each round of kills also pays out echo cells and mirror facets, so the mining trip and the
      repeater hunt are the same trip. Frontier systems are mostly off the lanes, so both ships
      usually need a pathfinder drive.
  - title: Monoliths
    facts:
      - {label: Repeater Monolith, value: "6,400 hull / 9,600 shield", basis: "measured (sighting)"}
      - {label: On a kill, value: "spawns two more", basis: "player-reported"}
      - {label: Escape, value: "none: the fight cannot be fled", basis: "player-reported"}
    notes: |
      Monoliths are not a harder spire: today they are **undefeatable**. Every kill spawns two
      more, so the fight only grows, and a ship in that fight cannot flee it. One player fought
      Monoliths continuously for eight hours, destroyed 354, and was lost (*player-reported*).

      Treat any system where a Monolith has been seen as closed, even if it holds seam glass.
      Only slivers, panes and spires simply respawn; only Monoliths multiply.
  - title: Found the site
    facts:
      - {label: Command, value: "build_gate_site {\"name\": \"…\", \"far_system\": \"<system id>\"}", basis: "measured"}
      - {label: Founding fee, value: "2,500,000 cr", credits: 2500000, basis: "measured"}
      - {label: Public access, value: "off at founding", basis: "measured"}
      - {label: Site defenses, value: "an outpost: 12,000 hull · 0 shield · no weapons, at founding", basis: "measured (get_base)"}
      - {label: Achievement, value: "The Gatekeeper (industry, 40 points): found one end of a jump gate", basis: "measured"}
    notes: |
      Found one end from the system it will stand in, naming the system the **other** end
      will stand in as `far_system`. The field is `far_system`: `system_id` is rejected with
      `invalid_payload` (*measured*). The site anchors at **its own new point of interest**
      and your ship is docked there.

      From the server's reply (*measured*):

      > Deposit construction materials into faction storage here; each facility cycle builds a
      > share of the current phase and pays labor from your faction treasury. Use 'station' info
      > to see the phase and what it still needs. Found the far end the same way, naming this
      > system as its far_system. The gate opens when both ends are built and both hold upkeep.

      So a gate is two construction sites, one at each end, each fed from its own faction
      storage, each paying labor from the treasury while it builds. Which phases a site goes
      through and what each needs is not yet measured.
  - title: Running a gate
    notes: |
      **Shims.** The fold wears through its Fold Shims, so a gate keeps eating
      [[annealed_seam_glass]] and [[circuit_board]] for as long as it stands (from the recipe
      and mission text; the rate is not yet measured).

      **Entanglers.** [[quantum_entangler]]s sit in a gate's storage as upkeep (*player-reported*;
      the mission text agrees). Sometimes one
      goes wrong: the logs call it **incoherent**. [[incoherent_entangler]]s have been found in
      gate storage, including one in an unfinished gate's storage on its inaccessible side
      (*player-reported*). First Step Memorial Station's mission *It Hums* asks for one, and
      its text now explains how they come about.
---

## Not yet measured

- How many Fold Lattice and Gate Regulators one gate takes.
- The construction phases of a gate site and what each phase needs.
- Labor per cycle while a site builds, and how long a site takes to finish.
- Upkeep for an open gate: entanglers, shims, anything else, and how often.
- What crossing a gate costs, who may cross (the public access setting), and how fast it is.
- How often an entangler in gate storage turns incoherent.
- Whether any repeater besides the spire drops a fold knot.
