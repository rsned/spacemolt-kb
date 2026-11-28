# Jump Gates player guide — design (small change)

## Intent

A new KB guide on jump gates, next to "Building a Player Station": what a gate is, how
the recipes are earned, what the parts take, how the hard inputs are gathered, how a site
is founded, and what is still unknown. The site is public, and other players are building
gates too (the first gate was built by another player).

## Decisions (operator, 2026-11-27)

- **Recipes:** cite our own capture (`data/recipes_unlisted.json`, craftsman-1, 2026-11-21)
  until the game publishes them; then switch the basis to the catalog.
- **Gathering side: hints only.** No system names, POI ids, or exact deposit amounts for
  seam glass; no per-site guard lists. The JoiFu-style level: seam glass sits in
  repeater-guarded nebula shells, 10 systems, ~17,700 in total; repeaters drop the rare inputs,
  spires the rarest. Monolith warning is in full (it is a safety warning, not an edge).
- **Approach 1:** hand-written `guides/jump-gates.md` rendered by the existing
  `generate-guides`, steps with `facts` + `notes` and no `facility` (as "Found the station"
  does). No renderer changes.

## Approaches considered

1. Hand-written guide on the existing renderer — CHOSEN. Same look/nav as the station guide;
   recipe numbers typed in with a basis line; a five-line edit if recipes change.
2. A `recipe:` step type that reads recipes from the unlisted file / catalog — keeps numbers
   in sync, but a code change with tests for 5 rarely-changing recipes. Revisit if more
   recipe-heavy guides follow.

## Outline (my call; correct freely)

Intro: what a gate is (a fold held open between two systems by a Lane Anchor at each end),
why it matters, the label convention (*measured* / *player-reported* / *doc-quoted*), and two
warnings: Monoliths, and that a gate eats upkeep for as long as it stands.

Steps:
1. **Earn the recipes** — Sinter's chain at Starfall Salvage Station ending in *The Method*
   (Taking the Machine Apart → Bench Test → The Method); what The Method's completion text says.
2. **The parts** — Lane Anchor (×2, one per end), Lane Anchor Prototype (test build), Fold
   Lattice ("a lot"), Gate Regulator, Fold Shim (the wear part). Each: inputs, hand-craft
   time, measured basis. Counts per gate: *not yet measured*.
3. **The rare inputs** — echo cells, mirror facets, fold knots from repeaters (sliver / pane
   / spire; fold knot is a rare spire drop); seam glass mined from guarded nebula shells (hint
   level only). Repeaters reflect damage (pane flat, spire % of energy) → kinetic for spires.
   How to work a guarded shell: a heavily tanked miner plus an escort that kills respawns.
4. **Monoliths** — undefeatable today; each kill spawns two more; a fight cannot be fled
   (player-reported: 8 hours, 354 kills, then lost). Stay out of any system holding one.
5. **Found the site** — `build_gate_site` with `{"name", "far_system"}` (`system_id` is
   rejected, *measured*). Founding fee 2,500,000 cr (*measured* 2026-11-28). The site gets
   its own new POI and you are docked there. Construction: deposit materials into faction
   storage **at the site**; each facility cycle builds a share of the current phase and pays
   labor from the faction treasury; `station` shows the phase and what it still needs. The far
   end is founded the same way from the other system. The gate opens when both ends are built
   and both hold upkeep. (Server text, *measured*.) Phase list and per-phase needs: *to be
   filled from our first site* (no system names in the public guide).
6. **Running a gate** — Fold Shims wear out (seam-glass upkeep); quantum entanglers sit in
   gate storage for upkeep and some turn **incoherent** (wanted by First Step's *It Hums*).
   Upkeep amounts, labor, crossing cost: *not yet measured*.

Closing: "Not yet measured" list, so the guide grows as we learn.

## Non-goals

Locations or maps of seam glass / repeater groups; our stock levels; any renderer change.

## Done when

`go run ./cmd/generate-guides` renders `kb/guides/jump-gates.html`, it appears in the
Guides index, and `[[item]]` links resolve (lane_anchor etc. may not have item pages yet;
unresolved ones render as plain text — check and note).
