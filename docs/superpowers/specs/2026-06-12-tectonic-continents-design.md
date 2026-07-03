# Tectonic Continents — Crust-Raft Design

**Date:** 2026-06-12
**Scope:** `pkg/planetgen` (field, render, types, profilejson), `cmd/planet-explorer`, `cmd/generate-planet-maps`
**Branch:** phase-0/cube-map (continues the planet-gen phase series)

## 1. Problem

Terran, super_terran, and oceanic planets render as island spray instead of
sizeable continents, and the landmasses bear no causal relationship to the
tectonic plate layer. Root cause: the `Continentalness` control field — an
independent fBm noise — decides land vs. ocean by threshold. Plates exist
(Phase 7+) but only modulate the ridged-mountain mask and seed the Voronoi
`Continents` cells; they never determine *where land is*. Mountain belts,
ocean basins, and coastline shapes are therefore uncorrelated with plate
boundaries.

## 2. Goal

Invert the hierarchy: **land/ocean comes from crust, not from noise.**
Continents are rafts of continental crust (cratons) riding on plates.
Mountain belts form where crust collides, trenches and arcs where ocean
crust subducts, ridges where ocean plates diverge, rifts where continents
tear. Downstream layers (biomes, rivers, rain shadow, erosion, civ, clouds)
consume the resulting heightmap unchanged.

**Non-goals (this round):**
- No time-stepped plate simulation. The crust stage is an analytic function
  of (plates, motions, TectonicAge). Its input/output seam — plates in,
  crust mask + base height out — is the explicit slot where a future
  iterative sim replaces it without touching downstream stages.
- No changes to gas giants, scorched, hothouse, lava_world, ice_world,
  unknown. (Plates-without-ocean for hothouse/lava is a possible follow-up;
  nothing in this design assumes water exists.)
- No new output files or KB page changes; same PNG set per planet.

**Decisions locked during brainstorming:**
- Tectonics-first round; other plan-review findings deferred.
- Believable end-state with slider-ready architecture (option A over a real
  simulation).
- Per-planet generation budget ≤ ~2× current (soft; production regens may
  spend more if needed).
- Continental assembly is a continuous per-seed axis with per-type weights
  (option A over uniform distributions).

## 3. Pipeline architecture

Current rocky pipeline (simplified):

```
plates → [Continentalness+Detail+PeaksValleys splines] → ridged(plate-masked)
       → basin → continents(Voronoi) → coastal → erosion → sea = fixed OceanLevel
```

New rocky pipeline:

```
plates (two-tier: major + minor filler)
  → crust        (NEW: cratons → ContinentalMask + BaseHeight)
  → boundary FX  (NEW: crust-aware belts/trenches/arcs/ridges/rifts × TectonicAge)
  → control fields (Detail + PeaksValleys unchanged; Continentalness demoted
                    to craton-edge shaping + interior variation)
  → coastal / erosion / smoothing / craters (unchanged)
  → sea level    (NEW: histogram quantile hitting TargetLandFraction)
```

**New type `field.CrustField`** (sibling of `PlateField`):
- `ContinentalMask *cubemap.CubeMapF` — 0..1 continental-crust fraction.
- `BaseHeight *cubemap.CubeMapF` — isostatic base: continental platform
  high, ocean floor low, smooth continental-shelf falloff between.
- Computed at coarse resolution (S/4) and upsampled with domain-warped
  lookups so coastlines stay fractal at full output resolution.

**Absorbed/replaced passes:**
- Voronoi `Continents` pass → superseded by cratons.
- `Basin` divergent depression → folds into the boundary-FX divergent case
  (inverted into a mid-ocean ridge with depth increasing away from it).
- Ridged plate-mask path → becomes the cont-cont collision case.

**Backward compatibility:** zero-value `CrustConfig` selects the legacy
path, byte-identical output (the established `Coverage==0 → off` idiom).

## 4. Plates and cratons

**Two-tier plate seeding.** Crust-enabled planets use `MajorPlates`
(4–8) and `MinorPlates` (3–6); the existing `PlateCount` remains honored
on the legacy (zero `CrustConfig`) path and is ignored when crust is on. Major seeds keep the Fibonacci-spiral
spread; minor seeds are placed by farthest-point sampling after the majors.
The random-walk flood fill gains a per-plate growth weight so majors claim
large territories and minors fill the gaps between them. Plate motion is
unchanged (per-plate rotation axes, sum-to-zero).

**Cratons.** `OceanicPlateFraction` is redefined crisply: that fraction of
plates carry no craton (pure ocean plates). Carrier plates receive 1–3
craton blobs: a center direction + radius, blob = warped radial falloff
whose edge is shaped by the demoted Continentalness noise. Coastlines are
fractal but each landmass is one coherent body. Cratons never straddle a
plate boundary; merged-looking landmasses arise only by abutting across a
convergent boundary.

**Assembly axis.** `Assembly ∈ [0,1]` sampled per seed from type-weighted
distributions:
- Low (supercontinent): few cratons placed on adjacent plates hugging their
  shared convergent boundary → one landmass with an emergent Himalaya-style
  suture belt through its middle.
- Mid (dispersed/Earth-like): cratons spread mid-plate → most coasts are
  passive margins; cordilleras only where a craton edge faces a convergent
  boundary.
- High (fragmented): more, smaller cratons + arc-heavy oceans; default
  flavor for oceanic type.

**Land budgeting.** Craton radii are scaled up front so total continental
area ≈ `TargetLandFraction`; the sea-level quantile (Section 6) trues it
up exactly.

## 5. Boundary effects × TectonicAge

One new lookup on top of the existing convergent/divergent/transform SDFs
and magnitudes: crust type on each side of the boundary. Dispatch:

| Boundary | Crust pairing | Effect |
|---|---|---|
| Convergent | cont ↔ cont | Collision belt: wide tall ridged band straddling the suture (Himalayas). |
| Convergent | ocean ↔ cont | Trench offshore (ocean side) + narrower coastal cordillera (Andes). |
| Convergent | ocean ↔ ocean | Trench + volcanic island-arc chain on the overriding side (Japan). |
| Divergent | ocean ↔ ocean | Mid-ocean ridge: bathymetric rise, floor deepens away from ridge. |
| Divergent | under/between cratons | Rift valley: floor depression + shoulder uplift; at high age×magnitude the floor drops below sea level (Red Sea splitting two abutting cratons). |
| Transform | any | Fault texture: small scarp roughness, no net elevation. |

All effects are envelope functions of boundary distance in km (via
`RadiusKm`), scaled by per-pixel boundary magnitude. A low-frequency
per-boundary activity noise varies energy between boundaries.

**`TectonicAge ∈ [0,1]`** scales uplift height, belt width, rift depth, and
a softening term (old belts wide + rounded like the Appalachians; young
ones sharp like the Himalayas). It is the v1 "millions of years" explorer
slider and complements the existing crater `SurfaceAge`.

## 6. Sea level and per-type defaults

For crust-enabled planets, sea level is **derived, not configured**: the
histogram quantile of the finished heightmap such that ocean covers
`1 − TargetLandFraction`. The derived level feeds every consumer of
`profile.OceanLevel` (coastal JFA, erosion, depth shading, rivers). Legacy
profiles (zero `CrustConfig`) keep the fixed threshold.

`TargetLandFraction` sampled per seed from per-type ranges:

| Type | Land fraction | Assembly weights (super/dispersed/fragmented) |
|---|---|---|
| terran | 0.22–0.38 | 25 / 65 / 10 |
| super_terran | 0.30–0.50 | 35 / 55 / 10 |
| oceanic | 0.03–0.12 | 5 / 25 / 70 |
| tundra, glacial | 0.25–0.45 | 25 / 65 / 10 |
| arid | 0.55–0.80 | 40 / 55 / 5 |

Crust is enabled by default for these six types only.

**Sampling vs. override semantics:** `Assembly` and `TargetLandFraction`
are profile fields. When a profile leaves them unset (zero), the generator
samples them deterministically from the master seed within the per-type
range/weights above — so two terrans differ, but the same planet always
renders the same. An explicit value in the profile (set via explorer or
JSON edit) pins it. The same idiom applies to `TectonicAge`.

## 7. Explorer, debug, testing, migration

**Explorer.** New "Tectonics" collapsible panel with bypass checkbox:
MajorPlates, MinorPlates, OceanicPlateFraction, Assembly, TargetLandFraction,
TectonicAge, PlateConvergentT. Debug-grid stages added: crust mask, base
height, boundary-effect classification (colored by effect), pre/post sea
level. Existing plate-motion arrow overlay unchanged.

**Testing.**
- Unit: craton area budgeting, quantile sea-level selection, crust-pairing
  classification, two-tier flood-fill weighting.
- Statistical invariants (CI, across seeds): land fraction within
  `TargetLandFraction ± 0.03`; largest connected landmass ≥ ~40% of land
  when Assembly < 0.5; small-island count below ceiling for non-fragmented
  worlds; mean elevation inside cont-cont convergent envelopes above planet
  mean; seam continuity for all new fields via the `seamtest` harness.
- Goldens regenerated for the six affected types; normal diff workflow.

**Migration.** `profilejson` schema version bump; `Migrate` fills
`CrustConfig` zero-valued so `handTuned: true` planets render byte-identically
until opted in. `handTuned: false` profiles re-seeded from new defaults via
the existing seeder; drift CI guards the result.

**Performance.** Crust + boundary fields at S/4, upsampled with warped
lookups; benchmark against the ≤2× per-planet budget using the existing
perf-test pattern.

## 8. Future work (explicitly out of scope)

- Time-stepped crust simulation replacing the analytic crust stage; the
  explorer slider then scrubs sim steps instead of re-rendering on age.
- Plates-without-ocean for hothouse/lava/scorched belt structure.
- Drift artifacts (matching coastlines across young oceans) — requires the
  simulation.
