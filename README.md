# Spacemolt Knowledge Base

Static site generator and SVG rendering engine for the [Spacemolt](https://spacemolt.com) space game. Produces a browsable HTML knowledge base covering systems, items, recipes, skills, and ships, along with procedurally rendered star system maps.

## Tools

### `cmd/generate-items-kb`

Main static site generator. Reads from SQLite databases and JSON catalogs to produce the full KB site under `kb/`.

```bash
go run ./cmd/generate-items-kb \
  -crafting-db crafting.db \
  -knowledge-db spacemolt-knowledge.db \
  -catalog-items catalog_items.json \
  -catalog-ships catalog_ships.json \
  -catalog-skills catalog_skills.json \
  -catalog-recipes catalog_recipes.json
```

**Generates:**

| Section | Output | Content |
|---------|--------|---------|
| Systems | `kb/systems/` | Per-empire network maps, system detail pages with rendered SVG system maps, POIs, connections |
| Items | `kb/items/` | 14 categories (ore, weapon, drone, etc.), item detail pages with crafting chains and ship usage |
| Recipes | `kb/recipes/` | 20 categories (Refining, Shipbuilding, Weapons, etc.), inputs/outputs, skill requirements |
| Skills | `kb/skills/` | Graphviz-rendered skill trees, XP tables, bonus tables, empire restrictions |
| Ships | `kb/ships/` | Stats tables grouped by faction and class, passive recipe badges |

### `cmd/system-map`

Standalone SVG renderer for individual star system maps.

```bash
# From game API JSON
go run ./cmd/system-map -json get_system.json [-map get_map.json] [-o output.svg]

# From knowledge database
go run ./cmd/system-map -db spacemolt-knowledge.db -system sol [-o output.svg]
```

### `cmd/test-system-map`

Generates 50+ test HTML pages exercising every rendering path in the system map engine: all spectral types (O through Y), luminosity classes (Ia through VII), white dwarf variants, planet classes, POI subtypes (asteroid belts, gas clouds, ice fields, relics, nebulae), black holes, binary/triple star systems, and animated ship traffic.

## Package: `pkg/systemmap`

SVG rendering engine for star system maps with MK stellar classification support.

**Features:**
- Star rendering with spectral type color interpolation (O-Y + white dwarf subtypes DA-DZ)
- Luminosity-based size scaling (Ia hypergiant 28px down to VII white dwarf 6px)
- Procedural planet surfaces (continents, rings, bands, lava, clouds) for 12 planet classes
- POI subtype rendering (metallic/carbonaceous asteroids, molecular/emission gas clouds, derelict/megastructure relics)
- Black holes with accretion disks and animated particle streams
- Jump gate placement computed from galaxy-level coordinates
- Ambient animated ship traffic between gates (scaled by security level)
- Coordinate grid with AU-scale indicators
- Explored/unexplored fog of war

**Key functions:**
- `RenderSystemMap(sys, allSystems, standalone)` - main entry point
- `ParseStarClass(class)` - parses MK notation (e.g. "G2 V", "DAP")
- `GetStarColorRefined(spectral, subtype)` - subtype-interpolated star colors
- `GetPlanetClass(classID)` - planet rendering parameters

## Output Structure

```
kb/
├── index.html              # Main landing page
├── smui.css                # Shared dark-theme stylesheet
├── systems/
│   ├── index.html          # Empire-grouped system index with network maps
│   └── {system}.html       # System detail with SVG map, POIs, connections
├── items/
│   ├── index.html          # Category grid
│   ├── {category}/index.html
│   └── {item}.html         # Item detail with crafting chain
├── recipes/
│   ├── index.html
│   ├── {category}/index.html
│   └── {recipe}.html       # Inputs, outputs, skill requirements
├── skills/
│   ├── index.html          # Category grid
│   ├── cat_{category}.html # Skill tree SVG + table
│   └── {skill}.html        # XP curve, bonuses, prerequisites
└── ships/
    └── index.html          # All ships by faction/class with stats
```

## Tech Stack

- **Go 1.24+** with SQLite (modernc.org/sqlite)
- Hand-crafted SVG generation (no external rendering libraries)
- Graphviz (`dot`) for skill tree diagrams
- Dark-themed responsive HTML with sortable tables
- Seeded procedural generation for deterministic planet/asteroid visuals

## Building

```bash
go build ./...
go test ./...
```
