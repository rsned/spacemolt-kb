# Planet Detail Pages — Design

## Goal

Add detail pages for every planet in the KB, showing a 3D sphere with the
procedurally generated surface map, physical stats, and existing DB data
(description, resources). Reorganize system pages into per-system directories
to support future POI detail pages (stations, belts, etc.).

## Decisions

- **Directory structure:** One directory per system, planet pages inside
- **Page naming:** `planet_{sanitized_name}.html`
- **Visual style:** Match existing KB dark theme (option A)
- **Stats:** Procedurally generated from planet type + seed, ±20% variation
- **Sphere view:** Inline JS canvas renderer (from preview.html), no flat map
- **Baseline references:** Saturn for jovian, Neptune for ice giant

## Directory Structure

```
kb/systems/
  index.html                    # system index (updated links)
  {system_id}/
    index.html                  # system detail (moved from {id}.html)
    planet_{name}.html          # planet detail pages
```

## Planet Physical Stats

Baseline values per type, varied ±20% per planet using FNV hash of name:

| Type | Analogue | Radius (km) | Mass (M⊕) | Gravity (g) | Temp (K) |
|------|----------|-------------|-----------|-------------|----------|
| scorched | Mercury | 2,440 | 0.055 | 0.38 | 440 |
| arid | Mars | 3,390 | 0.107 | 0.38 | 210 |
| terran | Earth | 6,371 | 1.0 | 1.0 | 288 |
| tundra | — | 5,740 | 0.85 | 0.9 | 220 |
| glacial | — | 6,371 | 1.0 | 0.95 | 170 |
| ice_world | Europa-ish | 7,000 | 1.2 | 0.85 | 100 |
| super_terran | — | 7,960 | 2.5 | 1.25 | 280 |
| hothouse | Venus | 6,052 | 0.815 | 0.9 | 737 |
| lava_world | — | 5,100 | 0.7 | 0.85 | 1200 |
| oceanic | — | 6,371 | 1.0 | 0.95 | 295 |
| jovian | Saturn | 58,232 | 95 | 1.06 | 134 |
| ice_giant | Neptune | 24,622 | 17 | 1.14 | 72 |

Additional stats:
- **Orbital period**: Kepler's third law from orbital distance
- **Atmospheric pressure**: type-dependent constant
- **Day length**: random 8-100h rocky, 10-16h gas giant

## Planet Detail Page Layout

1. Breadcrumb: Systems / {System} / {Planet}
2. Header: planet name, type badge, system link
3. Sphere canvas (400x400, auto-rotating, draggable, rings for gas giants)
4. Physical stats card (two-column grid)
5. Description (from DB or "Unexplored")
6. Resources card (if any)

## Implementation

### Part 1 — Stats generator (`pkg/planetgen/stats.go`)

- `PlanetStats` struct
- `GenerateStats()` using baseline + seeded ±20% variation
- Orbital period from distance

### Part 2 — Page generator (extend `cmd/generate-items-kb/main.go`)

- Planet detail HTML template with inline sphere JS
- Generate surface map PNG per planet
- Write to `kb/systems/{system_id}/planet_{name}.html`

### Part 3 — Migrate system pages to subdirectories

- Move `{id}.html` → `{id}/index.html`
- Update all links (index, inter-system, CSS paths)
- Add planet links to POI table on system pages

## Future Work

- Store planet metadata in dedicated DB table
- Agent-generated survey notes and descriptions
- Station, belt, and other POI detail pages in same directory structure
