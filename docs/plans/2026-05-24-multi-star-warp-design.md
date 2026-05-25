# Multi-Star Systems in the Hyperspace Warp

**Date:** 2026-05-24
**Status:** Approved

## Problem

The galaxy has multi-star systems, but the warp visualization collapses each
system to a single representative star. Alzirr — a trinary of Alzirr A (K2III),
a blank-class companion, and The Maw (a black hole) — renders as just the black
hole, hiding its two suns. We want the first-person warp flythrough to render
all component stars of a system.

## Data findings

- Every star is a `type='sun'` POI; the black hole is a sun with `class='BH'`
  (exactly one in the galaxy — The Maw, in Alzirr).
- Multi-star systems: **8 total** — 4 binary, 4 trinary. The rest are single;
  ~131 systems have no star data yet.
- Component stars are co-located at the system's galaxy coordinate; their
  separation is in AU (sub-pixel at galaxy GU scale), and two of Alzirr's three
  share the exact AU position (0,0). So real positions cannot separate them.

## Key constraint

Jump capture geometry (what blocks a route, where you drop out) is defined by
the **system's** single 100 GU bubble, not the individual stars. Therefore the
route/collision math stays one-entry-per-system, unchanged. Multiple stars are
a **visual decoration** layered on top of that entry.

## Design

### Data (`cmd/generate-items-kb/stars.go`)

`StarRecord` gains an optional `Suns []SunComp` (`json:"suns,omitempty"`), where
`SunComp{Name, Class string}`. Emitted only for multi-star systems (8 of 505),
so `stars.js` barely grows. The top-level `Class` stays the BH-preferred
representative (`sunClass`) for the PIP insets and the single-star fallback.

`sunComponents(pois)` returns the sun POIs ordered **headline-first** (any black
hole first, then the rest in catalog order), or `nil` when there is one sun or
none.

### Rendering (`kb/warp.js`)

Only the star-drawing and label passes change. When a scene star has
`suns.length > 1`, draw each component instead of one star:

- **Arrangement** is a deterministic view-space rosette inside the 100 GU
  bubble (~55 GU radius), computed from the system id (stable per frame):
  - If a black hole is the headline (`suns[0].class === 'BH'`), it sits at the
    center and the companions spread evenly on the ring.
  - Otherwise (e.g. binaries) all components spread evenly on the ring, giving a
    symmetric pair.
- Each component is colored/sized by its **own** class, so The Maw renders with
  the black-hole horizon + accretion treatment while Alzirr A is an orange
  K-giant and the blank companion a default star.
- Each component gets its own label at its drawn position, reusing the existing
  distance/offset fade. Fallback if cluttered: label only the headline.

Ambient field, bubbles, collision, arrival flash, and the PIP insets are
untouched — keyed to the system center.

### Scope

PIP insets (full-route map, radar) keep one marker per system using the
representative class. A 55 GU rosette is sub-pixel on the full-route map and a
tiny clump on the radar, so splitting it would only add noise. Multi-star
rendering is a first-person flythrough feature only.

## Testing

- TDD the Go data layer: `sunComponents` ordering (BH first), `nil` for
  single/no sun, and `StarRecord.Suns` populated only for multi-star systems
  with `omitempty` honored in JSON.
- Canvas rendering verified visually, consistent with the rest of `warp.js`.
