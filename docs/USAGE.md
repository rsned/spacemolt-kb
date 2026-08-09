# KB Generator Usage

Regeneration notes for the KB site generators. See the root `README.md` for
the full generator table; this file collects details on individual
generators as they're added.

## BoM Explorer

### BoM Explorer data

The interactive Bill of Materials explorer (`kb/build-costs/explorer.html`)
reads `kb/build-costs/recipe-graph.json`. Regenerate it after any crafting-DB
refresh:

    go run ./cmd/generate-bom-explorer

It needs only `crafting.db` and the newest `data/snapshots/<date>/` catalogs —
no market DB. `explorer.html` and `bom-explorer.js` are hand-maintained and are
not written by any generator.

Its default recipe choices deliberately diverge from the ones baked into the
committed `bill_of_materials` table for a handful of items (`gold_bar`,
`adamantite_bar`, `armor_plate`, `tungsten_rod`, `graphene_sheet`, and a few
others) — the static pages resolve ties with live market-availability data,
which can prefer a recipe that happens to route through a mob-drop input over
a mineable one. The explorer instead prefers structurally-obtainable inputs
(ore/material, or built from obtainable inputs) since its purpose is telling
you what to gather. Each run logs how many targets differ from the committed
table for this reason, and separately how many differ only because a
multi-yield recipe makes whole-batch rounding diverge from the table's
per-unit arithmetic — see `cmd/generate-bom-explorer/build.go`'s
`computeDefaults` doc comment and
`docs/superpowers/specs/2026-08-08-bom-explorer-design.md` for the full
reasoning.
