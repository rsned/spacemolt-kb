# KB Guides Section — Design

Date: 2026-10-25 · Status: draft for approval

## Intent

The KB is entirely **generated reference** (facilities, items, recipes, systems…).
It has nowhere for **long-form, hand-written guidance**: the "how do I actually do
this" knowledge the official spacemolt.com guides leave thin. The first need is a
**Player Station guide**, written while our faction (Crafting Collective) founds and
runs **Conjunction Junction** in Cloverfield. Founding it showed the official docs
are wrong or silent on what a station ships with, maintenance costs, and build order.

Readers: **anyone holding a Station Core.** That's us when we found station #2,
or another faction's leader. The guide is public on the KB site.

## What gets built

1. **A new `kb/guides/` section**: a Guides index page plus one page per guide,
   linked from the KB home page (`kb/index.html`) the same way the other sections are.
2. **A new generator `cmd/generate-guides`**, in the style of the other
   `cmd/generate-*` tools, that renders `guides/*.md` (hand-written markdown in this
   repo, rendered with goldmark, already a dependency) into `kb/guides/`.
3. **The first guide: `guides/player-stations.md`**, "Building a Player Station".

## The Player Station guide page (user-decided)

Top to bottom:

1. **Intro.** Briefly what a player station is and why we're writing this: a home
   of your own instead of rent at NPC stations, and a record of what the docs
   leave out.
2. **Annotated step-by-step build order.** Step 0 Found (`build_base`), then
   Locker Bay → Faction Workshop → Atmospheric Recycler → power → fuel.
   Each step shows:
   - what it gives you;
   - **build cost**: credits, materials, build time;
   - **running cost**: upkeep per cycle and per day (×86.4 cycles/day). This is
     **the headline** of each step, because facilities going offline from missing
     upkeep is the main risk a station owner faces;
   - notes and warnings from what we actually hit (e.g. "fresh station has no personal
     storage", "Workshop upkeep consumes focused crystals → ~346 energy crystals/day");
   - **every named facility and item cross-links to its KB page.**
3. Below the build order: reference material (e.g. the `station` command table, measured
   findings, "where the official docs are wrong").

**Layout: option A (user-chosen from a mockup).** Each build step is a **card**, with a
**sticky sidebar** that shows the **cumulative station upkeep per day** after the
steps so far (one row per item, summed across facilities), plus total credits spent
on builds.

## How it's made (Claude's calls, user said "a good start")

- **Guide source = markdown + a step list.** `guides/player-stations.md` holds the
  prose. Its build steps are declared as a small structured list (frontmatter or
  an equivalent block; builder's choice of syntax), each entry giving a **facility id**
  plus its notes/warnings markdown. **Authors never type numbers.**
- **Numbers come from the facility catalog at generation time**
  (`data/snapshots/latest/catalog_facilities.json`: `build_cost`, `build_materials`,
  `build_time`, `maintenance_inputs`, `maintenance_fuel`, `power_supply`,
  `life_support_supply`, `power_draw`, `life_support_draw`, `service_type`). So when
  the game changes a cost, the guide follows on the next KB build. `pkg/catalog.Facility`
  currently carries only id/name/category/level/recipe_id/build_materials and will
  need the extra fields.
- **Cross-links are automatic.** A facility id → `kb/facilities/<category>/<id>.html`;
  an item id → `kb/items/<category>/<id>.html` (category from `catalog_items.json`).
  Prose can link an item or facility inline with `[[id]]`. An id that matches nothing
  renders as plain text and is reported at build time; it never produces a broken link.
- **Measured vs catalog.** A step may carry measured values (e.g. maintenance observed
  at Conjunction Junction). Where they differ from the catalog, show both, labelled
  *catalog* and *measured (date)*.
- **Reusable core for the future planner.** Keep the catalog lookup, the per-day upkeep
  maths, and the cumulative totals in a small package (not buried in the generator), so
  the planner below can reuse them.

## Decisions from the builder check (Claude's calls)

- **Steps can offer alternatives.** A step lists one or more facility ids with one
  marked **default**; the sidebar totals use the default, and the card shows the
  alternatives compactly (build cost, upkeep/day, supply). The **power** step defaults to
  `stellar_collector_grid` (Cloverfield has a star; upkeep is only 2 circuit_board/cycle)
  with `fuel_cell_generator`, `bunker_fed_reactor`, `scrap_reactor` as alternatives.
  The **fuel** step stays a prose-only placeholder ("not yet charted") until we learn
  which facility "start making fuel" means; no guessed id.
- **Step 0 (founding) is a non-facility step** with a small hand-kept facts block (fee
  5,000,000; requires Station Core; ships with 500k faction storage, 0 power, 0 life
  support, no personal storage). Each fact carries its basis tag and date. This is the one
  allowed place for hand-typed figures, because the catalog doesn't have them. The founding
  fee counts toward the sidebar's credits total.
- **Sidebar totals use catalog values.** Measured values are shown alongside on the
  step card only. Measured format: per-cycle `item_id → qty` with a date and source.
- **Power & life-support balance is in scope.** The sidebar shows running supply vs draw
  for power and life support, and flags a step whose cumulative draw exceeds supply. The
  core maths and tests cover it. (A fresh station starts at 0/0.)
- **`maintenance_fuel` is its own row** in the upkeep totals (keyed as `fuel`, linked
  to the fuel item page if one exists), so totals stay keyed by item id.
- **Build totals include materials, not just credits.** The core package returns a
  cumulative bill of materials (credits + items) for the steps so far; the sidebar shows
  credits with the material total collapsible. The future planner needs exactly this.
- **This build includes writing the first guide's content**, not just a skeleton:
  intro, Step 0, Locker Bay, Faction Workshop, Atmospheric Recycler, Power (with
  alternatives), the Fuel placeholder, and the `station` command reference, written from
  `findings.md`. Sections we haven't measured yet (Defense, measured upkeep) say so
  plainly instead of being left out.

## Goals

- A readable, linkable guide page that a new station owner can follow from Station Core
  to a running station.
- Running costs impossible to miss.
- Numbers that never go stale relative to the catalog.
- Adding a second guide is just a new markdown file.

## Non-goals (this build)

- **Station planner** *(user's later feature)*: a checklist of facilities with a
  cumulative build bill of materials (like the BOM explorer, `cmd/generate-bom-explorer`)
  and cumulative upkeep tables. It's out of scope now, but the design must not block it.
- Automatically importing or syncing `spacemolt/data/stations/cloverfield/findings.md`.
  Guide prose is written by hand from those findings; nothing is written back to the
  spacemolt repo.
- Guides other than Player Stations (the section supports them; none are written now).
- Live game data (fuel prices, current storage levels). The guide is static.

## Anti-goals

- Guesses presented as fact. Every hand-written claim carries its basis (measured /
  catalog / doc-quoted / player-reported, with date), as in our findings file.
- Running costs buried in prose.
- Broken cross-links.
- Hand-typed numbers that drift from the catalog.

## Constraints

- Go 1.24+; follow existing generator conventions (flags for snapshot/output dirs,
  `internal/kbnav` for shared navigation, existing site CSS look).
- `golangci-lint` clean; tests for the catalog lookup, upkeep maths, and link resolution.
- Committed site output policy: follow whatever the repo already does for `kb/`.

## Content for the first guide (for the author, not the generator)

Source material lives in `~/spacemolt/spacemolt/data/stations/cloverfield/findings.md`:
founding facts (`build_base`, 5M fee, own new POI, auto-dock; ships with 500k faction
storage, 0 power, 0 life support, no personal storage), the player-reported build order
(Locker Bay → Faction Workshop → Atmospheric Recycler → then fuel), the catalog
maintenance table, and the full `station` command reference.

## Left to the builder

- Step-list syntax and markdown extension details.
- CSS for cards and sidebar (sticky on desktop; on narrow screens, collapse the sidebar
  to a summary above the steps).
- How the guides index is laid out.
