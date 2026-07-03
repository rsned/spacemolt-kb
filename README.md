# Spacemolt Knowledge Base

Static site generator and SVG/PNG rendering pipeline for the [Spacemolt](https://spacemolt.com) space game. It reads the game's SQLite databases and daily JSON catalog snapshots and produces a browsable HTML knowledge base — systems, items, recipes, skills, ships, facilities, missions, resources, factions, players, and passengers — together with procedurally rendered star-system maps, planet surfaces, a galaxy map, hyper-jump navigation charts, and day-over-day change reports.

The live site is published at [https://rsned.github.io/spacemolt-kb/](https://rsned.github.io/spacemolt-kb/).

## Contents

- [Data inputs](#data-inputs)
- [Generators (`cmd/`)](#generators-cmd)
- [Packages (`pkg/`, `internal/`)](#packages-pkg-internal)
- [Overlays — manual content enrichment](#overlays--manual-content-enrichment)
- [Python tooling (`tools/`)](#python-tooling-tools)
- [Generated site structure](#generated-site-structure)
- [Tech stack](#tech-stack)
- [Building](#building)

## Data inputs

| Input | Path | Description |
|-------|------|-------------|
| Crafting DB | `crafting.db` | SQLite database of recipes, bill-of-materials, and market price tables (symlink to the crafting server's DB). |
| Knowledge DB | `spacemolt-knowledge.db` | SQLite database of systems, POIs, catalogs, market intelligence, players, and passengers (symlink to the game data DB). |
| Snapshots | `data/snapshots/YYYYMMDD/` | Daily caches of the game API catalogs (`catalog_items.json`, `catalog_ships.json`, `catalog_skills.json`, `catalog_recipes.json`, `get_map.json`). `latest/` and `previous/` symlinks drive diff reporting. |
| Explorer notes | `data/explorer-notes.json` | LLM-generated discovery prose attached to POIs. |
| Planet textures | `data/planet-textures/` | Real Sol-system planet textures used by the planet renderer. |
| Overlays | `overlays/` | Hand-authored and machine-generated enrichment for factions, players, and passengers (see below). |

The two `*.db` files are symlinks to their authoritative sources; the KB is read-only against them. KB-derived data that must survive a regeneration (planet profiles, star assignments, etc.) is stored in dedicated tables managed by `pkg/kbdb`.

## Generators (`cmd/`)

| Tool | What it does | Key flags |
|------|--------------|-----------|
| `generate-items-kb` | **Main site generator.** Builds the items, recipes, systems, ships, skills, facilities, missions, and resources sections from the crafting + knowledge DBs, including BoM calculations and market data. | `-system` (regenerate one system), `-systems-only` |
| `generate-factions-kb` | Generates the factions, players, and passengers pages, merging in overlay data and (optionally) AI portraits via `$SMKB_PORTRAIT_CMD`. | `-portraits`, `-portrait-limit`, `-passengers`, `-agent-portraits`, `-portraits-only`, `-agents-dir`, `-daily-summary` |
| `generate-galaxy-map` | Renders the full galaxy map page: every explored system as an empire-colored node with connection links. | _(hardcoded paths)_ |
| `generate-diffs` | Compares fresh catalogs against the previous snapshot, writes per-day HTML change reports, and rotates the snapshot symlinks. | `-input`, `-snapshots`, `-output`, `-date` |
| `generate-explorer-notes` | Generates per-POI discovery journal prose using the Claude API; supports incremental and dry-run modes. | `-db`, `-out`, `-type`, `-poi`, `-limit`, `-model`, `-dry-run` |
| `generate-planet-maps` | Renders procedural planet surface PNGs from the DB, with configurable size and parallel workers. | `-type`, `-seed`, `-out`, `-db`, `-outdir`, `-width`, `-height`, `-workers` |
| `analyze-empire-economy` | Produces a Markdown report on component popularity, per-empire self-sufficiency, and resource scarcity. | `-crafting-db`, `-knowledge-db`, `-catalog`, `-out` |
| `hyperjump-analyze` | Computes Pathfinder Drive hyper-jump reachability: bearings, blocking systems, heading margins, and void-escape directions. | `-db`, `-margin`, `-out`, `-out-stations`, `-system` |
| `seed-overlays` | Seeds overlay stub files (`profile.md`) for players and factions from agent `personality.json` configs. | `-agents`, `-db`, `-overlays`, `-dry-run` |
| `system-map` | Standalone SVG renderer for a single star system, from game API JSON or the knowledge DB. | `-json`, `-map`, `-db`, `-system`, `-o` |
| `test-system-map` | Generates 60+ HTML test pages exercising every rendering path (all spectral types O–Y, luminosity classes Ia–VII, white-dwarf variants, planet classes, POI subtypes, black holes, multi-star systems, animated traffic). | _(hardcoded)_ |
| `planet-land-ratio-test` | Test grid of planet types across a range of land/liquid ratios, plus an HTML preview. | _(hardcoded, writes to `/tmp/land-ratio-test`)_ |

### Example: regenerate the core item/recipe/system site

```bash
go run ./cmd/generate-items-kb              # full rebuild
go run ./cmd/generate-items-kb -systems-only
go run ./cmd/generate-items-kb -system sol
```

### Example: render a single system map

```bash
# From game API JSON
go run ./cmd/system-map -json get_system.json [-map get_map.json] [-o output.svg]

# From the knowledge database
go run ./cmd/system-map -db spacemolt-knowledge.db -system sol [-o output.svg]
```

## Packages (`pkg/`, `internal/`)

| Package | Purpose | Key exports |
|---------|---------|-------------|
| `pkg/systemmap` | SVG star-system renderer with MK stellar classification, procedural planet surfaces, POI subtypes, black holes, jump gates, and ambient ship traffic. | `RenderSystemMap`, `ParseStarClass`, `GetStarSize`, `GetPlanetClass` |
| `pkg/planetgen` | Deterministic procedural planet surfaces (rocky and gas giant) from seeded simplex noise + crater algorithms. | `Generate`, `RenderRocky`, `RenderGasGiant`, `PlanetProfile`, `GetProfile` |
| `pkg/bom` | Bill-of-materials computation: resolves a target item to its base material requirements through the recipe chain. | `Calculator`, `BoMResult`, `NewCalculator`, `WriteBoM` |
| `pkg/gamediff` | Diffs two game-API catalog snapshots and reports structural changes. | `CatalogDiff`, `DayReport`, `DiffCatalog`, `DiffMap` |
| `pkg/hyperjump` | Pathfinder Drive jump geometry: reachable destinations, angular margins, interrupting systems, and coverage gaps. | `System`, `AngularMargin`, `Clearance`, `Interrupters`, `Coverage`, `Summarize` |
| `pkg/jumpmap` | SVG visualizations of hyper-jump analysis: destination starbursts and a 360° coverage dial. | `RenderArrows`, `RenderCoverageDial`, `DisplayBlockedPct` |
| `pkg/tierchart` | Groups tiered module families (e.g. `pulse_laser I–III`) and selects the relevant stat columns for comparison tables. | `TierStats`, `TierFamily`, `BuildFamilies`, `ColumnLabel` |
| `pkg/kbdb` | Manages KB-specific metadata tables in the knowledge DB so derived data persists across regenerations. | `Schema`, `Migrate` |
| `internal/kbnav` | Single source of truth for the site header and top navigation bar, shared by all generators. | `Items`, `Header` |

## Overlays — manual content enrichment

`overlays/` is a contributor-editable layer that augments machine-generated pages:

- `overlays/factions/{id}/`, `overlays/players/{id}/`, `overlays/passengers/{id}/` — hand-authored `profile.md`, portraits/logos, and override metadata.
- `overlays/generated/` — machine caches that are **not** hand-edited: `archetypes.json` (passengers classified into archetypes), `empire_guess.json` (empire inferred via n-gram Naive Bayes), `role_freeform.json`, and cached AI portraits keyed by prompt hash so they only regenerate when the prompt changes.
- `overlays/README.md` — the full contribution guide (image/markdown schema, precedence rules, override format).

`scripts/inject-overlay-images.sh` copies overlay imagery into the generated HTML during a build.

## Python tooling (`tools/`)

Auxiliary pipeline for portraits, classification, and concept art (run alongside the Go generators):

- **Portraits & classification** — `gen_portrait.py` (Stable Diffusion / SDXL-Turbo via `$SMKB_PORTRAIT_CMD`), `build_portrait_gallery.py`, `check_portrait_gender.py`, `infer_empire.py` (+ tests).
- **Archetype & role classification** — `classify_archetypes.py`, `role_freeform.py`, `analyze_roles.py`.
- **Voidborn species visualization** — `build_voidborn_gallery.py`, `build_voidborn_archetype_grid.py`, `animate.py` (image-to-video / GIF pipeline). Rendered assets live in `voidborn-concepts/`.
- **Agent galleries** — `build_agent_gallery.py`, `build_archetype_matrices.py`, `build_archetype_combined.py`.

Aggregated gallery pages: `portrait-gallery.html` (passengers), `agent-portrait-gallery.html` (agents).

## Generated site structure

Output is written under `kb/`. Top navigation (from `internal/kbnav`): **Systems · Items · Recipes · Skills · Ships · Facilities · Resources · Missions · Factions · Players · Passengers**.

```
kb/
├── index.html                  # Landing page
├── smui.css / system.css       # Dark-theme stylesheets
├── warp.js                     # Interactive map JS
├── galaxy-map.html             # Galaxy-wide network map
├── systems/                    # Per-empire network maps + system detail pages (SVG maps, POIs, connections)
├── items/                      # Item categories + detail pages with crafting chains and ship usage
├── recipes/                    # Recipe categories + detail pages (inputs/outputs, skills, facility gates)
├── skills/                     # Graphviz skill trees, XP curves, bonus tables
├── ships/                      # Ship stats by faction/class with passive recipe badges
├── facilities/                 # Station/base facilities and services
├── missions/                   # Mission templates and objectives
├── resources/                  # Resource POI index
├── factions/                   # Faction profiles (overlay logos/biographies)
├── players/                    # Player profiles (overlay biographies, affiliations)
├── passengers/                 # NPC profiles with portraits and archetype badges
├── diffs/                      # Per-day change reports comparing snapshots
├── did_you_know/               # Procedural trivia pages
└── images/                     # Copied portraits, logos, planet/system renders
```

## Tech stack

- **Go 1.25+**, SQLite via both `modernc.org/sqlite` and `mattn/go-sqlite3`
- Hand-crafted SVG generation (no external rendering libraries)
- `ojrac/opensimplex-go` for procedural planet/terrain noise; `golang.org/x/image` for PNG composition
- Graphviz (`dot`) for skill-tree diagrams
- `anthropics/anthropic-sdk-go` for LLM-generated explorer notes
- `yuin/goldmark` for rendering overlay Markdown
- Python + Stable Diffusion (SDXL-Turbo) for portrait and concept-art generation
- Dark-themed responsive HTML with sortable tables and seeded, deterministic procedural visuals

## Building

```bash
go build ./...
go test ./...
```
