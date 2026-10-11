# Tectonics — Land-mass layer (elevation, sea level, erosion) Design

Date: 2026-10-10
Status: approved in conversation, awaiting written review
Extends: `2026-10-02-tectonics-first-planet-gen-design.md` (checkpoint 1),
`2026-10-03-tectonics-rifting-design.md` (rifting)
Sub-project 1 of the land-mass roadmap (then: age parameterisation, detail
pass, hydrology).

## Why

Checkpoint 1 plus rifting gives plausible plate histories, but nothing in
the bundle says where the coast is. This layer turns the crust the
simulation already carries into elevation and a sea level on every
keyframe, adds erosion to the step loop so mountains wear down, and makes
the viewer's hypsometric layer draw real coastlines. It is the layer that
lets a planet be judged by eye, which the parked tuning questions (rift
chance, giant plates) and the next sub-projects all wait on.

## Decisions taken in brainstorming

| Question | Decision |
| --- | --- |
| Start state | Seed worlds start mature: continents are already in the step-0 thickness map; the run is a recent-past window, not a planet grown from a uniform crust. |
| Resolution split (roadmap, global) | Geology at face 128/256 (~78/39 km per pixel at a face centre on an Earth-sized world). Detail and hydrology later at 512/1024 on one frame. |
| Sea level | Water budget: a seeded water volume fixed per planet; each step the sea rises until that volume fills the hollows. Coastlines react to tectonics (collisions drop the sea, rifts flood). |
| Erosion | Slope-driven diffusion with sediment transfer, inside the step loop, land only. No flow routing (hydrology's job later). |
| State | Option A: thickness stays the only state. Elevation is derived from thickness and crust age at every keyframe; erosion acts on thickness; nothing new is advected. |
| Cross-boundary continents at step 0 | Not forced. Boundaries fall on thin crust by construction; land crosses a boundary only where a divide clears the land cut (merged plates, high land share). Rifting and collisions create the rest during the run. |
| Test visuals | Unit tests render before/after isometric panels on request (`-vis`), written to an ignored directory with a contact sheet. |

## Knobs (added to `Params`)

Defaults are starting values; the terran set is tuned by a seed sweep (see
Tuning) before merge, not from one seed.

| Knob | Default | Meaning |
| --- | --- | --- |
| `LandShareT0` | archetype: terran 0.30, super_terran 0.35, oceanic 0.15, arid 0.45, tundra 0.30, glacial 0.30, scorched 0.40, lava_world 0.40, ice_world 0.30 | share of the sphere remapped above the continental threshold at step 0 |
| `LandBandMin` | 0.55 | lowest thickness in the continent band after the remap |
| `OceanBandMax` | 0.45 | highest thickness in the ocean band after the remap |
| `LandRankPow` | 3 | rank exponent inside the continent band (higher = most continent is low) |
| `OceanRankPow` | 1 | rank exponent inside the ocean band |
| `ContinentAgeMyr` | 1000 | initial crust age of continental pixels |
| `OceanAgeCapMyr` | 180 | cap on the initial ocean age inferred from thickness |
| `ElevShelfKm` | −0.2 | continental elevation at the continental threshold |
| `ElevPeakKm` | 8 | continental elevation at thickness 1 |
| `ElevPow` | 1.6 | shape of the continental curve |
| `RidgeDepthKm` | −2.5 | ocean floor depth at age 0 |
| `SubsidenceKm` | 0.35 | ocean floor sinks this many km per √Myr |
| `AbyssKm` | −6 | floor of the subsidence |
| `OceanReliefKm` | 20 | km per unit of thickness deviation from the age-predicted ocean thickness (deviation clamped to ±0.1) |
| `ElevBlend` | 0.05 | half-width in thickness of the ocean/continent blend around `ContinentalThreshold` |
| `WaterFactorMin`, `WaterFactorMax` | archetype: terran 0.9–1.1, super_terran 0.9–1.1, oceanic 1.3–1.6, arid 0.5–0.7, tundra 0.8–1.0, glacial 0.8–1.0, scorched 0.3–0.6, lava_world 0.2–0.5, ice_world 0.8–1.0 | seeded multiplier on the step-0 hollow volume that sets the planet's water |
| `ErosionRate` | 0.01 | fraction of the elevation difference (over `ElevPeakKm`) moved per Myr |
| `ErosionMaxPerStep` | 0.2 | cap on the total fraction a pixel gives away in one step (stability) |
| `SedimentOceanFactor` | 0.5 | share of a slice kept when the receiving pixel is deep ocean |

`Validate`: `LandShareT0` in (0, 0.9); `OceanBandMax < ContinentalThreshold < LandBandMin`;
rank powers > 0; `ContinentAgeMyr`, `OceanAgeCapMyr` ≥ 0; `ElevShelfKm < ElevPeakKm`;
`ElevPow > 0`; `AbyssKm ≤ RidgeDepthKm`; `SubsidenceKm`, `OceanReliefKm` ≥ 0;
`ElevBlend` in (0, 0.2); `0 < WaterFactorMin ≤ WaterFactorMax`;
`ErosionRate ≥ 0`; `ErosionMaxPerStep` in (0, 0.25]; `SedimentOceanFactor` in [0, 1].

Seeds: the water factor uses domain `water.factor` (`drawWaterFactor(p, master)`).
Nothing else in this layer draws random numbers.

## 1. Elevation from crust

`Elevation(th, age *Grid, p Params) *Grid[float64]` returns height in km
for every pixel and is a pure function of the step's thickness and crust
age. `ageMyr = age × MyrPerStep`.

- **Continental branch** (`th ≥ T`, `T = ContinentalThreshold`):
  `h_c = ElevShelfKm + (ElevPeakKm − ElevShelfKm) × ((th − T)/(1 − T))^ElevPow`.
- **Oceanic branch** (`th < T`):
  `h_o = max(AbyssKm, RidgeDepthKm − SubsidenceKm × √ageMyr) + OceanReliefKm × clamp(th − thExpected(age), −0.1, 0.1)`
  where `thExpected(age) = RidgeThickness + OceanicThickening × √ageSteps` is the
  existing thickening law. Trenches are thinner than expected and sink; arcs
  are thicker and rise; an arc that keeps growing crosses `T` and becomes land.
- **Blend**: `w = smoothstep((th − (T − ElevBlend)) / (2 × ElevBlend))`,
  `h = (1 − w) × h_o + w × h_c`, so the two branches meet continuously.

Both branches are evaluated for every pixel (the ocean branch at `th ≥ T`
and the continental branch at `th < T` are just the blend's inputs).

### Step-0 remap (continents)

In `GenerateThickness`, after `normalize` and before the relax loop:
`remapBands(th, p)` sorts pixel indices by thickness (ties by index), takes
the top `LandShareT0` of the sphere **by area weight** as continent, and
maps rank `r ∈ [0, 1]` within each band:

- continent: `th = LandBandMin + (1 − LandBandMin) × r^LandRankPow`
- ocean: `th = RidgeThickness + (OceanBandMax − RidgeThickness) × r^OceanRankPow`

The remap is monotone, so the watershed's basins and divides are unchanged
in order; the relax loop then smooths the coast step to the 0.05 per pixel
limit. `CrustBias` stays (it still shapes which regions are thick).

### Initial crust age

`InitialAges(th *Grid[float64], p Params) *Grid[uint16]`, called by
`NewState` (which today zeroes ages):

- `th ≥ T`: `ContinentAgeMyr / MyrPerStep` steps (clamped to uint16).
- `th < T`: invert the thickening law, `steps = ((th − RidgeThickness)/OceanicThickening)²`
  (0 when `th ≤ RidgeThickness`), capped at `OceanAgeCapMyr / MyrPerStep`.

So ocean thicker than the cap explains starts as a submerged plateau
(positive deviation) rather than as impossibly old floor.

## 2. Sea level from a water budget

- `AreaWeights(S) []float64` (cached like `Dirs`): the solid angle of each
  cube-face pixel, from the exact corner formula
  `Ω = Σ_corners ± atan(u v / √(1 + u² + v²))` on the face's tangent plane
  (u, v ∈ [−1, 1]); the six faces sum to 4π. Volumes are
  `Σ depth_i × Ω_i × RadiusKm²` in km³.
- **Budget** (once, in `Run` after `NewState`): `elev0 = Elevation(th0, age0)`,
  `hollow = Σ max(0, 0 − elev0_i) × Ω_i × R²`, `s.Water = hollow × drawWaterFactor(p, master)`.
- **Sea level** (`SeaLevelFor(elev, water, S, p) float64`, every step and
  at step 0): the height `L` with `Σ max(0, L − elev_i) × Ω_i × R² = water`.
  Elevations are binned at 1 m over [−32 km, +32 km] (65,536 bins); a
  single pass over the bins accumulates area and volume and interpolates
  `L` inside the bin where the volume is reached. `L` is clamped to the bin
  range.
- **Water mask**: every pixel with `elev < L` is water. Connectivity is
  ignored (an interior basin below `L` is an inland sea); lakes versus sea
  is hydrology's job later.
- `State` gains `Water float64` (km³) and `SeaLevel float64` (km), updated
  at the end of every `Step` (after erosion and relax). `Snapshot` fills
  `Frame.Elev` (km), `Frame.SeaLevelKm` and `Frame.LandShare` (area-weighted
  share with `elev ≥ L`).

## 3. Erosion in the step loop

`erode(th, age, labels, seaLevel, p, nb)` runs inside `Step` as a new step 7
after ageing (step 6) and before the relax pass; it reads an `Elevation`
grid computed from the step's thickness and age.

For each pixel `i` with `h_i ≥ L`:

- for each 4-neighbour `j` with `h_j < h_i`: `f_ij = ErosionRate × MyrPerStep × (h_i − h_j) / ElevPeakKm`;
- scale all `f_ij` of `i` down so `Σ_j f_ij ≤ ErosionMaxPerStep`;
- move `Δ_ij = f_ij × max(0, th_i − th_j)` thickness from `i` to `j`. If
  `th_j < T` and `h_j < L` (deep ocean receiver) `j` gains only
  `SedimentOceanFactor × Δ_ij`; the rest is lost.

Moves are computed from the step's starting thickness and applied in one
pass (Jacobi style), so order does not matter and the result is
deterministic. Pixels with `h_i < L` never erode. Plate labels are
ignored for deposition. No new feature type, no sediment field.

Thickness is clamped to [0, 1] after the pass, as elsewhere.

## 4. Bundle, viewer, lab, stats

- **Bundle version 2.** `Frame` gains `Elev *Grid[float64]`, `SeaLevelKm`,
  `LandShare`. `WriteFrame` adds `elevation.png`: a cube-cross PNG like the
  others, `v = clamp(round(h_km × 1000) + 32768, 0, 65535)`, red byte
  `v >> 8`, green byte `v & 255`, blue 0, alpha 255 (range ±32.768 km, 1 m
  steps). `FrameInfo` gains `sea_level_km` and `land_share`; `Manifest`
  gains `water_km3` and `Version` becomes 2. `ReadFrame` decodes
  `elevation.png` when present. Plate, thickness and feature images are
  unchanged.
- **Viewer** (`cmd/tectonics-lab/web`): the hypsometric layer samples the
  elevation texture (`h = (r × 255 × 256 + g × 255 − 32768) / 1000` km in
  GLSL) and the frame's sea level: water below `L` shaded by depth (two
  blues), land shaded by height above `L` (green → brown → white), a
  one-pixel coast line where a NEAREST-sampled pixel is land and a
  4-neighbour is water. The status line shows `sea level X km · land Y %`
  after the Myr. A version 1 bundle (no `elevation.png`) keeps today's
  thickness-based hypsometric shading. Plates, thickness and features
  layers are untouched.
- **Lab.** No new commands or flags; new knobs go through `-set` and the
  run form's Sets field.
- **Earth exporter** (`scripts/earth_plates_export.py`): writes version 2
  with `elevation.png` = 0.8 km inside continents and −3.7 km elsewhere,
  `sea_level_km` 0, `land_share` from the mask, `water_km3` 0. Regenerating
  `earth-nnr` is optional.
- **Stats** (`scripts/tectonics_stats.py`): new lines and CSV fields
  `land_mean`, `land_min`, `land_max` (land share over frames),
  `land_elev_mean` (area-weighted mean elevation above sea level, km),
  `elev_max` (km), `ocean_depth_mean` (km). Bundles without
  `elevation.png` print `—`. The sweep driver (`tectonics_seed_sweep.py`)
  adds these to its METRICS table with an outlier band of 0.15–0.5 on
  `land_mean`.
- **Golden** (`cmd/tectonics-lab/testdata/golden_face64.json`): recipe
  `terran/2026/64/20/v10`; the hash adds the `Elev` float bits and each
  frame's `SeaLevelKm` and `LandShare` bits.

## 5. Test visualisations

`vis_test.go` in `pkg/tectonics` (test-only code, no new dependencies):

- `renderIso(h *Grid[float64], face cubemap.Face, seaLevel, exaggeration float64) *image.RGBA`
  draws one cube face as a 3/4 isometric height field: each pixel a shaded
  column (height × exaggeration), land coloured by height above sea level
  (green → brown → white), a translucent water plane at `seaLevel`, water
  depth tinted below it. Painter's order back to front.
- `writeVis(t, name string, panels ...*image.RGBA)` tiles the panels side
  by side with a label strip and writes `testdata/vis/<name>.png`, then
  regenerates `testdata/vis/index.html` listing every PNG present. Both are
  no-ops unless `go test -vis` is passed (flag registered like `-update`).
- A difference panel helper `diffPanel(before, after *Grid[float64], face, scale)`
  colours loss red and gain blue, used by the erosion tests.
- `testdata/vis/` is added to `.gitignore`.

Each test in section 6 that changes a field renders before/after (or a
strip for multi-step tests) through these helpers.

## 6. Tests

- `TestElevationContinentalMonotone`: `h_c` strictly increases with `th` on `[T, 1]`.
- `TestElevationOceanSinksWithAge`: at fixed thickness-deviation 0, `h_o` decreases with age toward `AbyssKm` and never below it.
- `TestElevationBranchesMeet`: `|h(T − ε) − h(T + ε)| < 0.3 km` for ε = 1e−3 at age 0 and at the cap.
- `TestElevationTrenchBelowAgeDepth`: a pixel `TrenchDepth` thinner than expected sits below a pixel at the expected thickness with the same age.
- `TestRemapLandShare`: after `remapBands` on an S=32 noise field the area-weighted share with `th ≥ T` is within 0.01 of `LandShareT0`, no ocean pixel exceeds `OceanBandMax`, no continent pixel is below `LandBandMin`; after the relax loop the gradient cap holds.
- `TestInitialAgesRoundTrip`: `InitialAges` returns N for a pixel whose thickness is `thExpected(N)` for N in {0, 10, 50}; continent pixels get `ContinentAgeMyr / MyrPerStep`; the cap is honoured.
- `TestAreaWeightsSumToSphere`: `Σ AreaWeights(S) = 4π` within 1e−9 for S in {8, 32}.
- `TestSeaLevelAtDatum`: on an S=32 remapped field, `SeaLevelFor(elev0, hollow × 1.0)` is within 0.002 km of 0; factor 1.2 gives a higher level; the filled volume at the returned level matches the budget within 0.1 %.
- `TestErosionPeakSlumps`: one tall column on a flat continent above sea level: peak thickness decreases every step, the four neighbours gain, total thickness is conserved to 1e−12 (no ocean receiver), and no pixel goes negative at `ErosionMaxPerStep` = 0.25 with a 50 Myr step. Renders a 6-panel strip.
- `TestErosionSubmarineUntouched`: pixels below sea level never change.
- `TestErosionOceanLoss`: a coast pixel next to deep ocean loses `Δ` while the ocean pixel gains `SedimentOceanFactor × Δ`.
- `TestLandmassDeterminism`: two `Run`s at S=32, 10 steps produce identical `Elev`, `SeaLevelKm`, `LandShare`.
- `TestLandmassIntegration`: `Run` at S=64, 20 steps, terran: land share in [0.15, 0.5] on every frame, sea levels finite and within [−5, 5] km.
- Golden rebaked as v10; `TestTimingBudgetFace128` unchanged (10 s).
- Manual: `tectonics_stats.py` on `earth-test` (regenerated as v2) and on a seed bundle prints the new lines; the sweep smoke at face 64 writes the new columns.

## Tuning (before merge)

A seed sweep at face 256, terran, 400 steps (the existing driver), targets:

- `land_mean` 0.25–0.35, moving by a few points over a run but not drifting;
- `land_elev_mean` 0.5–1.2 km (Earth 0.84), `ocean_depth_mean` near −3.7 km, `elev_max` 5–9 km;
- a fresh ~6 km collision belt with no further uplift wears to ~2 km in ~250 Myr (checked on a single-seed `-set CollisionUplift=0 ArcUplift=0` run from a mid-run frame).

Ship the defaults that meet these across the sweep, as with rifting.

## Out of scope

Frozen water and uplift decaying with age (age sub-project); lakes versus
sea connectivity, rivers, valleys (hydrology); surface noise (detail pass);
a separate sediment field; and the existing quirk that ocean crust thickens
with age so old ocean subducts less readily than young (the reverse of
Earth) — noted, not fixed; ocean depth here comes from age, not thickness,
so it does not affect this layer.
