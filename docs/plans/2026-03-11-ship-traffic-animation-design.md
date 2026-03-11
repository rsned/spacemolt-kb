# Ship Traffic Animation for System Maps

## Overview

Add ambient animated ship traffic to system maps. Ships travel back and forth
between POI pairs based on system composition and security level, using 6
distinct ship icon silhouettes.

## Ship Icons

| # | Name       | Shape                                    | Size | Route Context                         |
|---|------------|------------------------------------------|------|---------------------------------------|
| 1 | Shuttle    | Small wedge/arrow, minimal detail        | 10px | Gate↔Gate, general traffic            |
| 2 | Freighter  | Wide rectangular hull, stubby wings      | 16px | Station↔Resource (cargo runs)         |
| 3 | Hauler     | Long narrow body, boxy rear              | 14px | Station↔Resource                      |
| 4 | Corvette   | Sleek pointed nose, swept wings          | 12px | Station↔Gate (patrol/escort)          |
| 5 | Scow       | Squat trapezoid, asymmetric              | 14px | Gate↔Resource (independent miners)    |
| 6 | Raider     | Angular, aggressive wedge, notched wings | 12px | Gate↔Gate low-sec, Gate↔Sun pirates   |

All icons rendered in `#8b95ab` at ~0.6 opacity (ambient background, not
competing with POI markers). Oriented pointing right (0°) by default;
`rotate="auto"` on `<animateMotion>` handles facing.

## Route Generation Rules

Routes are determined by POI composition and system security level (0-100):

| Route Type        | Condition                | Security 0-30 | Security 31-70 | Security 71-100 |
|-------------------|--------------------------|----------------|----------------|-----------------|
| Gate↔Gate         | 2+ connections           | 1              | 1-2            | 2-3             |
| Station↔Gate      | Station exists           | 0              | 1              | 1-2             |
| Gate↔Resource     | Resource, no station     | 0              | 1              | 1-2             |
| Station↔Resource  | Both exist               | 0              | 1-2            | 2-4             |
| Gate↔Sun          | Dead-end pirate system   | 1              | 0              | 0               |

**Resource POI types:** `asteroid_belt`, `ice_field`, `gas_cloud`

**Dead-end pirate stronghold:** Single connection, lawless (security 0-30),
unexplored interior. Route is Gate↔Sun until POI data is available.

When multiple resources exist, routes are distributed across them via seeded
RNG from system ID.

## Animation Cycle

Each route runs a 60-second cycle:

```
[Ship A appears at start]
  → 20s travel to destination (fade in at start, fade out at end)
  → 10s pause (no ship visible)
[Ship B appears at destination]
  → 20s travel back to start (fade in at start, fade out at end)
  → 10s pause (no ship visible)
[Repeat]
```

- Total cycle: 60 seconds
- Each route gets a staggered `begin` delay (seeded) so ships don't depart
  simultaneously
- The return trip is a second `<g>` with reversed path, offset by 30s
- Ship icon is randomly selected per trip from the route type's allowed set
- Paths use quadratic bezier curves (slight perpendicular offset) so ships
  don't fly along axis lines; multiple routes between the same POI pair get
  different curve magnitudes

## SVG Implementation

- Ship silhouettes defined as `<symbol>` elements in `<defs>`
- Each animated ship: `<g>` containing `<use>` + `<animateMotion>` along path
- `rotate="auto"` for automatic heading
- Opacity animated via `<animate attributeName="opacity">` synced to motion

## File Changes

### New: `pkg/systemmap/ships.go`
- 6 ship icon `<symbol>` definitions
- `generateShipRoutes(sys *System, scale, cx, cy float64) string` — route
  generation + animation rendering
- Route rule logic, ship selection, path computation

### Modified: `pkg/systemmap/systemmap.go`
- Add `Security int` to `System` struct
- Emit ship `<symbol>` defs in `<defs>` block (when routes exist)
- Call `generateShipRoutes()` after POI rendering, before labels

### Modified: `cmd/test-system-map/main.go`
- Add `"ship-traffic"` test: station + asteroid belt + 2 gates, security 75
- Add `"ship-traffic-pirate"` test: dead-end, 1 gate, security 0, Gate↔Sun
