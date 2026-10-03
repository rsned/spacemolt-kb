# Tectonics-First Planet Generator — Checkpoint 1 Design

Date: 2026-10-02
Status: approved in conversation, awaiting written review

## Why a restart

The existing generator (`pkg/planetgen`, ~18–22k lines across `main` and
`phase-0/cube-map`) grew into an 11-stage pipeline plus plates, crust, flow,
erosion, clouds and civilisation layers. Every refresh runs every layer, the
fBM base still reads as white noise, and attempts to cut it back landed as
more branches on top. This spec starts a new, empty working space whose first
stage is the one the old pipeline bolted on last: tectonic plates that form
from the crust and drift over geologic time.

The old generator is untouched and keeps producing the live KB textures until
the new one reaches parity.

## Goals

1. From a planet's game id (seed) and archetype, produce 5–10 major and 10–20
   minor plates whose boundaries follow the thinnest crust, not Voronoi
   cells grown from random points.
2. Drive those plates forward over geologic time as rigid caps on the sphere,
   with divergent seams, subduction trenches, collision belts and transform
   faults emerging from the motion.
3. Show the result in a local dev tool with a time slider, so the drift and
   the ageing of mountain belts can be judged by eye.
4. Fix a bundle contract so every later layer (accretion, erosion, biomes)
   runs against the same slider.

## Non-goals (this checkpoint)

Deposition/accretion, erosion, colour and biomes, KB page integration,
replacing the live planet textures, gas giants.

## Decisions taken in brainstorming

| Question | Decision |
| --- | --- |
| Where it lives | New package `pkg/tectonics` + new `cmd/tectonics-lab`, on a new branch off `main`. Imports only `pkg/planetgen/cubemap` and `pkg/planetgen/seed`. |
| Plate construction | Watershed on the thickness field (not a thin-path walker, not Voronoi). |
| Motion | Rigid drift on the sphere: every plate rotates about its own Euler pole. |
| Compute | Timeline precomputed in Go into a keyframe bundle; the browser only scrubs. |
| Archetype role | Small parameter set per archetype (plate count ranges, activity, crust contrast, timeline length). Looks come later. |

## Section 1 — Thickness field and plates

### 1a. Crust thickness field

- Grid: the existing cube-sphere (`cubemap.CubeMapF`) at face size `S`
  (default 256, ≈393k pixels).
- Value: thickness in `[0,1]`, 0 thin, 1 thick.
- Generation: two or three octaves of low-frequency value noise sampled by
  3D direction (`cubemap.FacePixelToDir`) so there are no seams. Seed
  domains `thickness.base` and `thickness.warp` via `seed.Domain`.
- Relaxation: iterate until no pixel differs from any 4-neighbour
  (`cubemap.FacePixelNeighbors4`) by more than `MaxNeighborDelta`
  (default 0.05). Then normalise to `[0,1]`.

### 1b. Plates by watershed

1. Find local maxima of thickness (seeded tie order).
2. Priority-flood from them, highest thickness first, so every pixel joins
   the catchment of the thick high it drains to. Divides fall on the thin
   paths. Expect a few hundred micro-basins.
3. For every adjacent basin pair record the divide thickness: the thickest
   pixel along their shared boundary.
4. Merge, repeatedly, the pair whose divide is thickest (least seam-like),
   until the seeded target count is reached. Targets are drawn from the
   archetype ranges: majors (default 5–10) + minors (default 10–20).
5. Plates below `MinPlateArea` (fraction of the sphere, default 0.002) merge
   into their largest neighbour.
6. The N largest plates by area are majors; the rest are minors.

### 1c. Per-plate motion seeds

- Angular speed drawn from the archetype activity range, in cm/yr,
  converted to degrees per step with `RadiusKm` (default 6371).
- Push direction: the plate's centroid velocity points away from its
  thinnest boundary stretch (where the mantle pushes up), perturbed by seed.
- Euler pole derived from that velocity. Seed domain `plates.motion`.

### Stage outputs

Thickness grid, plate-id grid, plate table (id, area, major/minor, Euler
pole, speed, mean thickness). Deterministic from seed + archetype.

## Section 2 — Time simulation

### 2a. Clock and motion

- `TimelineMyr` from the archetype (terran 800), split into `Steps`
  (default 150): ≈5 Myr per step, one to three pixels of drift per step at
  face 256.
- Each step every plate rotates rigidly by `speed × dt` about its pole.
- Every `RepoleEvery` steps (default 25) each plate re-aims: recompute the
  push direction from its current thinnest boundary stretch and blend with
  the old direction, so locked plates gradually turn away from collisions.

### 2b. Per-step resolution

Inverse-map every grid pixel through every plate's rotation (bounding-cap
early-out) to find the claimants and the thickness each brings.

| Claimants | Rule |
| --- | --- |
| None | Divergent gap. New crust at `RidgeThickness` (default 0.15) + seeded jitter, age 0, assigned to the nearer retreating plate. |
| One | Carried over; age += dt. Young thin crust thickens ∝ √age (`OceanicThickening`). |
| Two+ | Take the two leading plates' relative velocity at the pixel; split into normal and tangential parts. |
| … tangential/normal > `TransformRatio` (default 2) | Transform fault: thicker plate keeps the pixel, shallow scar cut, fault age stamped. |
| … both ≥ `ContinentalThreshold` (default 0.5) | Collision: thicker plate keeps it; thickness += `CollisionUplift` per overlapping step, capped at 1; belt age stamped. |
| … otherwise | Subduction: thinner plate loses the pixel; trench carved on its side (`TrenchDepth`, decaying over a few pixels); small `ArcUplift` a few pixels inland on the overriding side. |

- A plate shrinking below `MinPlateArea` is retired; its pixels keep their
  thickness and join the overriding neighbour.
- A light diffusion pass (`Relax`) each step keeps the field within the
  neighbour-delta rule and removes single-pixel spikes.

### 2c. Keyframe bundle

Every `KeyframeEvery` steps (default 1) write a frame directory containing:

- `thickness.png` — cube-cross, 8-bit grey.
- `plate.png` — cube-cross, plate id.
- `feature.png` — cube-cross, one byte per pixel: boundary type in the top 2 bits (0 none, 1 divergent, 2 convergent, 3 transform), belt/fault age in the low 6 bits (steps since stamped, saturating at 63).
- one JSON row per plate: centroid, pole, speed, area.

`manifest.json` at the bundle root lists seed, archetype, every knob, step
count, Myr per step and the frame paths. Rough size: 150 frames × ≈250 KB ≈
35 MB per planet; `KeyframeEvery 2` halves it.

## Section 3 — Code, viewer, testing

### 3a. Layout

`pkg/tectonics/` (flat files by stage):

| File | Responsibility |
| --- | --- |
| `thickness.go` | field generation + relaxation |
| `watershed.go` | basins, divides, merging, plate table |
| `motion.go` | Euler poles, rotation, re-aim |
| `step.go` | per-step resolution rules |
| `sim.go` | run loop, frame callback |
| `params.go` | `Params` struct, archetype table, planet-id → seed |
| `bundle.go` | manifest + frame write/read |

`cmd/tectonics-lab/`:

- `run`: compute a bundle for `-planet <id>` or `-seed <n>` and
  `-archetype <name>` into `data/tectonics/<slug>/`; any knob via
  `-set name=value`.
- `serve`: host the viewer, list bundles, start a run on request, polled
  status endpoint.
- Binary in `bin/`; bundles git-ignored (`data/tectonics/`).

### 3b. Viewer (`cmd/tectonics-lab/web/`)

New small page, raw WebGL. Cube-map textures take the six faces straight from
the cross PNG (no equirect bake). Controls: planet/seed box, archetype select,
time slider with play/pause and Myr readout, layer toggles (plate colours by
id; thickness grey or hypsometric; feature overlay — convergent red,
divergent blue, transform yellow, belts fading bright→dull with age),
per-plate velocity arrows, flat cube-cross beside the sphere. Frames decode
lazily and cache.

### 3c. Archetype table (first cut, to be tuned)

| Archetype | Majors | Minors | Speed cm/yr | Timeline Myr | Crust |
| --- | --- | --- | --- | --- | --- |
| terran, super_terran | 6–9 | 10–16 | 3–8 | 800 | normal |
| oceanic | 6–9 | 10–16 | 3–8 | 800 | thinner mean |
| arid, tundra, glacial | 4–7 | 8–12 | 2–5 | 600 | normal |
| scorched, lava_world | 8–12 | 14–20 | 6–12 | 600 | thin, hot |
| ice_world | 2–4 | 2–6 | 0.2–1 | 600 | frozen shell |
| jovian, ice_giant | — | — | — | — | refused |

### 3d. Testing

Fast unit tests at face 32–64:

- relaxation honours `MaxNeighborDelta` on every 4-neighbour pair;
- watershed covers every pixel; plate counts inside the archetype range;
  mean boundary thickness < mean field thickness;
- plate id and thickness agree across every cube seam;
- rigid rotation preserves plate area within tolerance;
- gap pixels occur only at divergent boundaries;
- collisions raise thickness and stamp belt age;
- same seed → byte-identical bundle.

A committed golden of per-frame hashes at face 64 is the regression gate.
Face 256 runs stay out of CI; a face 128 timing test keeps one run under a
stated budget. Go 1.24 idioms (range over int, `b.Loop()`), clean
`golangci-lint`.

### 3e. Later-layer contract

Every later layer reads a bundle and writes a derived bundle with the same
frame indexing and manifest shape, so one slider and one viewer serve all
layers. Nothing beyond the contract is built now.

## Reused from the old tree

- `pkg/planetgen/cubemap`: faces, cross PNG read/write, direction↔pixel,
  seam-aware 4/8 neighbours, `CubeMapF`.
- `pkg/planetgen/seed`: `Domain(master, name)` for per-stage seeds.
