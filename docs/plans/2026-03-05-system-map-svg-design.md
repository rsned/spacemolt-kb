# System Map SVG Generation Design

## Overview

Add SVG system maps to all 505 system detail pages in the KB generator.
Two rendering modes: explored systems (29 with POI data) show orbital rings,
POI markers, and asteroid/ice particles; unexplored systems (476) show a
fog-of-war effect with the star visible through translucent cloud cover.

## Map Layout

- 800x600 viewBox, center at (400, 300)
- Dark background (#1a1e2a) via existing CSS
- Explored: axis crosshairs, orbital rings, POI markers, jump gates outside
- Unexplored: star, radial gradient fog (r=200px), jump gates at r=230px

## Coordinate Mapping (Explored)

Auto-scale per system: find max POI radius, scale so it maps to ~250px.
Jump gates placed at ~280px. POI positions: svgX = 400 + poi.X * scale,
svgY = 300 - poi.Y * scale (Y inverted).

## Jump Gate Positioning

Angle from current system to connected system using galaxy-map coordinates
(atan2(dy, dx)). Gate markers on a ring just outside outermost content.

## POI Types

- sun: radial gradient glow, #EBCB8B/#D08770, always at center
- planet: solid circle r=5, #A3BE8C
- station: hexagon outline, #81a1c1
- asteroid_belt: dashed ring + ~100 triangle particles, #D08770
- ice_field: dashed ring + ~80 diamond particles, #88C0D0
- gas_cloud: overlapping semi-transparent circles, #B48EAD
- relic: 4-pointed star, #EBCB8B

## Particles

Deterministic: seeded from POI ID hash. Triangles (asteroids) and diamonds
(ice) scattered along orbital ring with ±15% radial jitter. Varying size
(2-5px), opacity (0.3-0.7), rotation.

## Fog of War (Unexplored)

RadialGradient: transparent center, fog begins at ~60% radius (#8b95ab
at 0.15 opacity), densest at edge (0.25 opacity). No crosshairs or
orbital rings. Star glows through center.

## Implementation

- Add PositionX/PositionY to SystemPOI struct, load from DB
- Add renderSystemMap() function returning full SVG HTML string
- Template calls {{systemMap .}} between header and connections
- Particle generators seeded by POI ID hash for determinism
- System map keyed by ID passed via closure for galaxy-position lookups
