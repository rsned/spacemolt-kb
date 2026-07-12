# Planet Explorer - User Guide

## Overview

Planet Explorer is a web-based interactive tool for exploring and tuning procedural planet generation. It compiles the Go planet generation pipeline to WebAssembly and renders live in your browser, allowing real-time experimentation with planet parameters.

**What it does:**
- Generates procedural planet textures using fractal Brownian motion (fBm) noise and advanced techniques
- Renders both cube-map cross (6 faces) and equirectangular projections
- Provides interactive sliders and panels for tuning all generation parameters
- Exports tuned profiles as JSON for use in production code

**Key Concepts from the Compass Artifact:**
This tool implements **Tier S** and **Tier A** enhancements from the procedural generation guide:

**Tier S (massive visual change, hours-to-days of work each):**
- Multi-noise control fields (Minecraft 1.18 approach)
- Domain warping (Quilez)
- Whittaker biome classification with OkLab color blending
- Per-archetype color LUTs
- Slope-based shading

**Tier A (high visual change, days of work):**
- Ridged multifractal mountains (masked by Continentalness)
- Province/roughness modulation (Voronoi regional variety)
- Particle-based hydraulic erosion (realistic alluvial fans, rivers)
- Coastal-noise enhancement (archipelagos, fjords near coastlines)
- Curl-noise turbulent advection for gas giants (realistic banding)
- Continent Voronoi masks (polygonal continental silhouettes)

---

## Quick Start

### Building and Running

```bash
# Build the WebAssembly binary
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm

# Run the dev server
go run ./cmd/planet-explorer
# Serves on http://localhost:8080 by default
```

**Optional flags:**
- `-addr :8080` — change listen address
- `-web cmd/planet-explorer/web` — change static file directory
- `-wasm cmd/planet-explorer/web/planet-explorer.wasm` — change WASM binary path

### Basic Workflow

1. **Select a planet type** from the dropdown (loads default profile)
2. **Adjust parameters** using the slider panels in the left sidebar
3. **Click "Regenerate"** to render (or press Enter with focus on sliders)
4. **View results** — rotating sphere preview + hidden cube-map/equirect canvases
5. **Export JSON** to copy tuned profile to clipboard for use in `pkg/planetgen/profile.go`

---

## UI Reference

### Header Controls

#### Type Selector
Choose from 13 planet archetypes:
- **Rocky planets:** scorched, arid, terran, tundra, glacial, ice_world, super_terran, hothouse, lava_world, oceanic
- **Gas giants:** jovian, ice_giant
- **Unknown** — generic fallback

Each type has carefully tuned default parameters in `pkg/planetgen/profile.go`.

#### Seed Input
Text string for deterministic noise generation. Same seed + same profile = identical planet. Try:
- `Earth` — default
- Planet names: `Mars`, `Venus`, `Kepler-22b`
- Random strings: `X7J9-K2M4`, `proctime-2025`

#### Face Size
Render resolution (per cube face). Higher = slower but more detail:
- `64` — fastest preview, ~200ms render time
- `128` — quick iteration
- `256` — default, good balance (~1s)
- `512` — high detail (~5-10s)
- `1024` — production quality (~10-20s)

#### View Mode
- **Color** — final shaded planet with biomes, oceans, ice caps
- **Heightmap** — grayscale elevation only (useful for debugging terrain)

#### Regenerate Button
Re-renders the planet at current settings. Also triggered by Enter key when editing sliders.

#### Jitter: ON/OFF Button
Phase 7 Tier B toggle that flips `JitterEnabled` in the profile JSON and re-renders. When on, the Detail control field samples a Voronoi cell-coordinate jitter (via `JitterCellCount`, `JitterRotMax`, `JitterOffsetMax`) so the same noise looks subtly different per region — breaks up the "stamped fBm" appearance without changing macro shape. The button label reflects current state.

#### Export JSON Button
Copies the current profile JSON to clipboard. Paste into `pkg/planetgen/profile.go` to commit tuned defaults.

---

## Parameter Panels

### Control Fields Panel

The heart of the new pipeline. Five independent 3D fBm noise fields that replace the single monolithic noise approach.

**Why this matters:** Following Minecraft 1.18's design, orthogonal control fields combined via splines produce "designed-feeling" planets instead of generic noise textures.

#### How Control Fields Combine

The combination method is **pure additive summation**, not multiplication or complex mixing:

1. **Per-field processing** (each of the 5 fields independently):
   - Sample fBm noise at pixel → raw value in [0, Amp]
   - Multiply by province frequency modifier (if provinces enabled)
   - Evaluate the spline: `EvalSpline(field.Spline, modifiedValue)` → height contribution
   - Multiply by province amplitude modifier (if provinces enabled)

2. **Sum all 5 contributions** → raw heightmap value

3. **Add ridged mountains** (if enabled, masked by Continentalness spline output)

4. **Normalize entire heightmap to [0,1]** — find global min/max, remap linearly

5. **Carve craters** by subtracting depth from normalized heightmap

**Key insight:** The splines do the heavy lifting of shaping each field's contribution before summation. This is why Continentalness might have a sharp mid-range S-curve (for coastlines) while Erosion has a negative curve (to subtract from highlands). Summing these shaped contributions creates the final terrain character.

**Province modulation** (if enabled) scales each field's contribution per-region, making some areas more mountainous, others smoother — this is applied *before* summation via the `freqMod` and `rampMod` multipliers.

#### Continentalness
**Purpose:** Macro land/ocean separation — the base shape of continents
**Effect on height:** Spline output adds to height
**Typical curve:** Steep in the middle for sharp coastlines
**Use when:** You want to control continent size and distribution

**Parameters:**
- `Amp` (0.5–2.0) — Output multiplier; determines continental contrast
- `Freq` (0.5–5.0) — Spatial frequency; higher = more, smaller continents
- `Octaves` (3–5) — Detail layers
- `Lacunarity` (~2.0) — Frequency multiplier per octave
- `Persistence` (~0.5) — Amplitude decay per octave

**Knots:** Spline maps [0, Amp] noise output to height contribution
```json
[{"Input":0,"Output":0},{"Input":0.4,"Output":0.05},{"Input":1,"Output":0.6}]
```

#### Detail (formerly "Erosion")
**Purpose:** High-frequency detail-noise layer that adds bumpy variation on top of the macro shape
**Effect on height:** Spline output adds to height (pure additive — *not* the negative-output "carve" curve the legacy name suggested)
**Use when:** You want surface texture that's independent of continent shape

**Parameters:** Same as Continentalness. Typical spline is a gentle positive ramp:
```json
[{"Input":0,"Output":0},{"Input":1,"Output":0.15}]
```

**Backward compatibility:** Profile JSON that uses the old `"Erosion"` key still loads — `ControlConfig.UnmarshalJSON` (`pkg/planetgen/types/types.go:360`) maps it to `Detail` when `Detail` is absent. Hydraulic erosion (Phase 5 droplets) lives in its own *Erosion Panel* further down.

#### PeaksValleys
**Purpose:** High-frequency mountain roughness on top of macro shape
**Effect on height:** Adds small-scale detail
**Use when:** You want jagged peaks and textured terrain

**Parameters:** Higher freq (2–8), higher octaves (4–6) for fine detail

#### Temperature
**Purpose:** Drives biome row selection in Whittaker table
**Effect on height:** None directly (used for biome lookup)
**Use when:** You want realistic biome distribution

**Special behavior:** Combined with `cos(latitude)` so poles are colder than equator

#### Humidity
**Purpose:** Drives biome column selection in Whittaker table
**Effect on height:** None directly (used for biome lookup)
**Use when:** You want wet/dry biome variation

**Special behavior:** No latitude bias — purely from this noise field

---

### Domain Warp Panel

**Purpose:** Displaces sphere coordinates before sampling noise (Quilez domain warp)
**Effect:** Features bend, curl, and swirl instead of being axis-aligned
**Why it's Tier S:** Single highest-ROI technique — one warp pass doubles perceived "organic-ness"

**Parameters:**
- `Amp` (0–0.4) — Displacement magnitude. 0 = off, >0 = warped
- `Freq` (0.5–4.0) — Warp noise frequency
- `Octaves` (1–3) — Warp detail layers
- `Lacunarity` (~2.0) — Standard fBm
- `Persistence` (~0.5) — Standard fBm

**Visual effect:**
- `Amp = 0` — straight latitude/longitude features
- `Amp = 0.03` — gentle organic curves (default for terran)
- `Amp = 0.1+` — dramatic swirling, fjord-like coastlines

**Tip:** Small values (0.02–0.05) go a long way. High warp (>0.15) gets chaotic fast.

---

### Ridged Mountains Panel

**Purpose:** Ridged-multifractal mountain belts (Musgrave/libnoise technique)
**Effect:** Adds sharp, crested mountain ranges
**Why it matters:** Prevents "spaghetti mountains" by masking via Continentalness

**Parameters:**
- `Amp` (0.05–0.3) — Mountain height contribution. 0 = disabled
- `Freq` (0.5–5.0) — Ridge density
- `Octaves` (4–6) — Mountain detail
- `Lacunarity` (~2.0) — Standard fBm
- `Gain` (~0.5) — Per-octave weight (higher = sharper)
- `Offset` (0.8–1.2) — Ridge sharpness
- `MaskLow` (0.3–0.5) — Mask input ≤ this = no ridges
- `MaskHigh` (0.6–0.8) — Mask input ≥ this = full ridges
- `PlateConvergentScaleKm` — *(JSON-only — not exposed in the UI panel.)* Switches the mask source. `0` = legacy Continentalness mask. `>0` = plate-convergent SDF mask (see below).

**How masking works:**
- **Legacy mode** (`PlateConvergentScaleKm = 0`): mask = `smoothstep(MaskLow, MaskHigh, Continentalness)`. Mountains follow continent interiors regardless of geology.
- **Plate-convergent mode** (`PlateConvergentScaleKm > 0`): mask = `smoothstep(MaskLow, MaskHigh, 1 - clamp(plates.Convergent[px] / PlateConvergentScaleKm, 0, 1))`. Mountain belts trace plate convergent boundaries — the geologically correct placement (Andes, Himalayas, Cascades). Falls back to the legacy path automatically if the planet has `PlateCount = 0`.
- `MaskLow = 0.4, MaskHigh = 0.7` is a reasonable starting point in either mode.

**Visual effect:**
- Creates sharp, jagged peaks like the Andes or Himalayas
- Contrasts with smoother fBm hills

---

### Provinces Panel

**Purpose:** Voronoi cells over the sphere, each with per-cell roughness variation
**Effect:** Regional variety so planet doesn't look like uniform noise
**Why it's Tier A:** Reproduces Mars dichotomy (smooth north, cratered south) with one extra noise

**Parameters:**
- `Count` (8–40) — Number of Voronoi cells. 0 = disabled
- `Jitter` (0–0.5) — Per-cell scalar variation. 0 = uniform; 0.5 = high variety
- `WarpAmp` (0–0.15) — Sphere warp before Voronoi lookup. 0 = clean cells; >0 = curvy boundaries

**How it works:**
1. Generate Voronoi cells over sphere
2. Each cell gets random amp/freq multiplier for control fields
3. Result: neighboring regions have different terrain character

**Visual effect:**
- `Count = 0` — uniform texture across planet
- `Count = 16, Jitter = 0.3` — distinct regions with different roughness
- `WarpAmp = 0.1` — organic, non-polygonal region boundaries

**Tip:** Start with `Count = 12–20` for subtle variety. Higher counts (>30) get busy.

---

### Plate Tectonics *(JSON-only — no UI panel yet)*

**Purpose:** Voronoi tectonic plates with classified boundaries (convergent/divergent/transform). Drives both the ridged-mountain mask (when `Ridged.PlateConvergentScaleKm > 0`) and the macro continent layout via the rewired item-10 Voronoi continents seeded from plate centroids.
**Renderer:** Rocky only.
**Phase:** Phase 7 Tier B.

Plates are toggled via top-level fields on `PlanetProfile` rather than a config struct. Edit them in the Profile JSON textarea:

| Field | Range | Effect |
|-------|-------|--------|
| `PlateCount` | 0 disables; 8–20 typical | Number of Voronoi plate centroids over the sphere |
| `OceanicPlateFraction` | 0–1 | Fraction of plates flagged oceanic (lower base height + thinner crust) |
| `PlateConvergentT` | 0–1 | Threshold above which a plate-pair velocity dot product counts as "convergent" (drives mountain belt placement) |
| `RadiusKm` | 0 = 6371 (Earth) | Planet radius in km, used by the JFA pass to scale angular distances into kilometers for `PlateConvergentScaleKm` lookups |

**How it works:**
1. `PlateCount` Voronoi cells with random rotation axes (plate motion vectors)
2. JFA computes a per-pixel signed distance to the nearest plate boundary, and classifies each boundary segment as convergent / divergent / transform from the relative motion of the two adjacent plates
3. The convergent-distance field is then available as a mask source for ridged mountains and as a continent seeder

**Tip:** Plates are silent unless something downstream reads them. Without `Ridged.PlateConvergentScaleKm > 0` *or* a Voronoi-continent setup tied to plate centroids, raising `PlateCount` will not visibly change the render.

---

### Tectonics (Crust Rafts) Panel

**Purpose:** Phase 12 crust-raft continents — plates carry cratons of continental crust that *decide* land vs ocean, replacing noise-threshold continents.
**Renderer:** Rocky only.
**Phase:** Phase 12.

When `majorPlates > 0` the whole pipeline switches to the crust path: two-tier plate seeding (major + minor filler plates with growth-weighted flood fill), craton placement on the non-oceanic plates, a `ContinentalMask` + `BaseHeight` that initialize the heightmap, the Tectonic FX boundary pass (below), and a sea level *derived* from the target land fraction by histogram quantile. Legacy Ridged, Basin, and Continents passes are skipped; the Continentalness control field no longer drives height (Detail and PeaksValleys still add relief on top).

**Note the JSON keys:** unlike the older capitalized profile fields, the crust config lives under lowerCamel keys — `crust.majorPlates`, `crust.assembly`, etc.

**Sample sentinels:** `assembly`, `targetLandFraction`, and `tectonicAge` accept **-1**, meaning "sample deterministically per planet from the configured range/weights". Any other value pins the parameter (0 is a valid pin — e.g. `assembly: 0` forces a supercontinent).

| Field | Range | Effect |
|-------|-------|--------|
| `majorPlates` | 0 disables; 6–8 typical | Number of major plates (two-tier seeding) |
| `minorPlates` | 0–8 | Gap-filler plates between the majors |
| `majorGrowthBias` | 1–8 | Flood-fill growth weight of majors vs minors (area ratio) |
| `oceanicFraction` | 0–1 | Fraction of plates carrying no craton (pure ocean floor) |
| `assembly` | -1 samples; 0–1 pins | 0 = supercontinent … 1 = fragmented small continents |
| `assemblyWeights` | 3 weights | Sampling weights for super / dispersed / fragmented bands |
| `targetLandFraction` | -1 samples; 0–1 pins | Exact land fraction the derived sea level enforces |
| `landFracLo` / `landFracHi` | 0–1 | Sample range when `targetLandFraction = -1` |
| `tectonicAge` | -1 samples; 0–1 pins | The "millions of years" slider: 0 young/sharp (Himalayas), 1 old/soft (Appalachians) |
| `ageLo` / `ageHi` | 0–1 | Sample range when `tectonicAge = -1` |
| `cratonsMax` | 2–14 | Craton cap; actual count grows with assembly |
| `shelfWidthRad` | 0.01–0.2 | Continental-shelf falloff half-width (radians) |
| `edgeNoiseAmp` / `edgeNoiseFreq` / `edgeNoiseOctaves` | — | Craton coastline fractality |
| `platformHeight` / `oceanFloorHeight` | 0–1 | Continental platform and abyssal base heights |

**Worked examples:**
- `assembly: 0` — one merged landmass with an interior mountain seam where its cratons abut.
- `assembly: 1` — scattered smaller continents, more coastline, more island arcs.
- Drag `tectonicAge` 0 → 1 — the same boundaries re-render softer and wider (amplitude × 0.45–1.0, width × 1.0–1.8).

**Off switch:** the Clear button (or `majorPlates: 0`) restores the full legacy pipeline — the pre-Phase-12 look.

**Debug stages:** `Crust` (continental mask + base height) and `TectonicFX` (boundary height delta) appear in the pipeline debug grid.

---

### Tectonic FX Panel

**Purpose:** Crust-aware boundary height effects along classified plate boundaries. Runs only on the crust path (`crust.majorPlates > 0`).
**Renderer:** Rocky only.
**Phase:** Phase 12.

Boundary class comes from pairing the crust type on each side (continental vs oceanic) with the boundary kind (convergent / divergent / transform):

| Effect | Boundary | Knobs |
|--------|----------|-------|
| Collision belt (Himalayas) | cont-cont convergent | `beltAmp`, `beltWidthKm`, `beltFreq`, `beltOctaves` |
| Coastal cordillera (Andes) | ocean-cont convergent, land side | `cordAmp`, `cordWidthKm` |
| Subduction trench | ocean-cont convergent, ocean side | `trenchDepth`, `trenchWidthKm` |
| Island arc (Japan) | oce-oce convergent | `arcAmp`, `arcWidthKm` |
| Mid-ocean ridge | oceanic divergent | `ridgeAmp`, `ridgeWidthKm` |
| Rift valley | continental divergent | `riftDepth`, `riftWidthKm`, `riftShoulder` |
| Transform fault roughness | transform | `transformAmp`, `transformWidthKm` |

Widths are Gaussian sigmas in km. Each effect disables individually at zero amplitude. `activityFreq` varies energy along boundaries with low-frequency noise so not every collision looks equally violent. `tectonicAge` (from the Tectonics panel) globally scales all effects: young = tall and narrow, old = low and wide.

---

### Shading Panel

**Purpose:** Slope-based Lambertian diffuse lighting
**Effect:** Makes mountains look 3D, not flat
**Phase:** Phase 3 polish

**Parameters:**
- `ShadingStrength` (0–1) — Lighting intensity. 0 = flat; 0.5 = reasonable; 1.0 = full
- `ShadingExaggeration` (0–50) — Heightmap gradient multiplier. 0 = default 8.0. Higher = more dramatic relief

**How it works:**
1. Compute surface normal from heightmap gradient
2. Calculate dot product with light direction
3. Modulate color by lighting

**Visual effect:**
- `Strength = 0` — flat, cartoony look
- `Strength = 0.5` — subtle depth
- `Strength = 1.0, Exaggeration = 20` — dramatic, exaggerated relief

**Tip:** Use heightmap view mode to see pure shading without color interference.

---

### Ocean Panel

**Purpose:** Configure sea level and ocean color
**Effect:** Pixels below OceanLevel get depth-shaded ocean color

**Parameters:**
- `OceanLevel` (0–1) — Normalized sea level. Pixels with height < this are ocean
- `OceanColor` — Base ocean color (RGB picker)

**How depth shading works:**
- Deeper pixels (lower height) get darker version of OceanColor
- Creates realistic depth gradient from coast to abyss

**Visual effect:**
- `OceanLevel = 0` — no oceans, all land
- `OceanLevel = 0.5` — Earth-like (half land, half sea)
- `OceanLevel = 0.8` — water world

**Tip:** OceanLevel interacts with Continentalness. Tune together for desired coastlines.

---

### Cryosphere Panel

**Purpose:** Polar ice caps and mountain snow lines
**Effect:** White-tinted overlays at high latitudes and elevations

**Parameters:**
- `HasPolarCaps` (checkbox) — Master toggle for polar caps
- `PolarCapSize` (0–0.5) — Latitude fraction covered. 0.15 = Earth-like; 0.4 = Hoth-like
- `PolarCapNoise` (0–0.5) — Edge roughness. 0 = smooth circle; 0.5 = jagged
- `SnowLine` (0–1) — Elevation above which pixels get snow tint. 0 = disabled

**Visual effect:**
- Polar caps appear as white overlays at poles
- Snow line adds white to peaks above threshold
- Combined with Temperature field for realistic snow distribution

**Tip:** Use `PolarCapNoise = 0.1–0.2` for natural-looking irregular edges.

---

### Height Smoothing Panel

**Purpose:** Per-face disc blur applied to the heightmap *after* control-field summation but *before* normalization
**Effect:** Smooths fBm popcorn so the erosion pass can carve coherent channels instead of biting into noisy single-pixel highs
**Renderer:** Only shown when profile's `Renderer = "rocky"`

**Parameters:**
- `Radius` (0–8 px) — Blur radius. 0 disables; 2–3 typical for terran/super_terran; up to 5 for very smooth worlds.

**Why it matters:**
- Without smoothing, hydraulic erosion droplets get trapped on individual fBm peaks
- With `Radius=2–3`, droplets walk smooth slopes and form proper drainage networks
- Larger values produce broader, gentler terrain at the cost of surface texture

**Tip:** Tune in tandem with `Erosion.Droplets`. If rivers look like scattered specks rather than channels, raise `Radius` before adding more droplets.

---

### Craters Panel

**Purpose:** Stamp circular depressions on the heightmap
**Effect:** Impact craters for barren bodies (Mercury, Moon, Mars)
**Renderer:** Only shown when profile's `Renderer = "rocky"`

**Parameters:**
- `CraterCount` (0–200) — Number of craters. 0 = none; 200 = Mercury-dense
- `CraterMinRadius` (0.001–0.02) — Smallest crater radius (radians on unit sphere)
- `CraterMaxRadius` (0.01–0.1) — Largest crater radius. Should be > MinRadius
- `CraterDepth` (0.02–0.3) — How deep craters cut into heightmap

**How it works:**
1. Random crater centers via Poisson-disc sampling
2. Radii drawn from uniform distribution between Min/Max
3. Each crater carves depth into heightmap

**Visual effect:**
- `Count = 0` — No craters (Earth-like)
- `Count = 50, Depth = 0.1` — Light cratering (Mars)
- `Count = 200, Depth = 0.25` — Heavy cratering (Mercury/Moon)

**Tip:** For realistic power-law size distribution, you'd need the full crater system (see Compass Artifact §1). Current implementation is simplified.

---

### Erosion Panel

**Purpose:** Particle-based hydraulic erosion — droplets walk the heightmap carving channels and depositing sediment
**Effect:** Realistic alluvial fans, river valleys, and worn-down terrain
**Renderer:** Only shown when profile's `Renderer = "rocky"`

**Parameters:**
- `Droplets` (0–500,000) — Canonical droplet count at face=1024. Auto-scaled by face area; floor 5000 so face=64 previews still show channels. 0 disables erosion.
- `Inertia` (0–1) — How much velocity carries between steps. 0.05 default. Higher = straighter channels.
- `Capacity` (0–20) — Sediment capacity multiplier. 4 default. Higher = droplets carry more before depositing.
- `ErosionRate` (0–1) — Fraction of "missing capacity" carved per step. 0.3 default.
- `Deposition` (0–1) — Fraction of "excess sediment" dropped per step. 0.3 default.
- `Evaporation` (0–0.1) — Water lost per step. 0.01 default.
- `MinSlope` (0–0.5) — Floor on slope used in capacity calc to avoid 0. 0.01 default.
- `MaxStepsPerDrop` (0–200) — Hard cap on steps per droplet. 50 default.
- `Gravity` (0–20) — Speed gain from -Δh per step. 4 default.
- `BrushFalloff` (0–16) — Brush sharpness exponent in `1/(1+r)^k`. 0/missing = 1.0 (3-pixel wide channels). 4–8 = near-single-pixel for narrow rivers.

**How it works:**
1. Spawn N droplets at random positions on the sphere
2. Each droplet walks downhill, carrying capacity
3. Carve erosion when moving uphill (capacity check)
4. Deposit sediment when capacity exceeded
5. Evaporate water per step
6. Apply changes to heightmap via Gaussian brush with falloff

**Visual effect:**
- `Droplets = 0` — No erosion (pure noise terrain)
- `Droplets = 50,000` — Light channeling (early drainage networks)
- `Droplets = 150,000` — Moderate erosion (Mars-like)
- `Droplets = 300,000+` — Heavy erosion (Earth-like with mature river systems)

**Why this matters:**
- Self-reinforcing channels emerge naturally
- Sediment fills receivers, creates basins
- Distinguishes plausible planets from pure-noise ones
- Realistic alluvial fans and river valleys

**Tip:** Start with `Droplets = 50,000` at face=256 for quick iteration. Increase to 150,000+ for final renders. Higher `BrushFalloff` creates narrower, more realistic rivers.

---

### Rivers (Flow) Panel

**Purpose:** D8 flow accumulation + Planchon-Darboux pit fill, then carve channels where flow exceeds a threshold
**Effect:** Mature river networks that actually drain to the ocean — distinct from particle-erosion droplets
**Renderer:** Rocky only.
**Phase:** Phase 8 item 15.

Profile field: `Flow` (JSON key `"flow"`). `RiverThreshold == 0` disables the entire pass.

| Field | Range | Effect |
|-------|-------|--------|
| `RiverThreshold` | 0–1 | Accumulation cutoff. Pixels with normalized flow accumulation ≥ this are carved. 0 disables. |
| `RiverDepth` | typical 0.005–0.03 | Height units carved into the river bed. |

**How it works:**
1. **Pit fill** — Planchon-Darboux ensures every cell drains downhill (no closed basins)
2. **D8 flow direction** — each cell points to its lowest neighbor
3. **Flow accumulation** — count upstream cells per pixel
4. **Carve** — subtract `RiverDepth` from cells whose accumulation exceeds `RiverThreshold`

**Difference vs hydraulic Erosion:**
- Erosion (droplets) is stochastic and produces alluvial fans, terraces, valley widening — texture-rich but no guaranteed connectivity
- Flow is deterministic and produces topologically correct river networks — every river reaches an ocean — but doesn't add fan/terrace texture
- Use both: Flow first for connectivity, then Erosion for texture

**Tip:** `RiverThreshold` is normalized to the per-face accumulation max, so the same value works across face sizes. Start at `0.001` for dense networks, raise toward `0.01` for sparse trunk rivers only.

---

### Rain Shadow (Wind) Panel

**Purpose:** Wind-tangent moisture advection — upwind sides of mountain ranges get more rain, lee sides get less
**Effect:** Modulates the Whittaker M (humidity) input so deserts naturally appear behind mountain belts (rain shadow), and rainforests appear on windward slopes
**Renderer:** Rocky only.
**Phase:** Phase 8 item 16.

Profile field: `RainShadow` (JSON key `"rainShadow"`). `WalkSteps == 0` disables the entire pass.

| Field | Typical | Effect |
|-------|---------|--------|
| `WalkSteps` | 0 disables; 8–24 typical | Number of upwind walk steps per pixel |
| `StepArcRad` | ≈ 0.087 (5°) | Arc length per step in radians |
| `MountainCutoff` | 0.55–0.75 | Height threshold above which a step counts as orographic uplift |
| `WindRainBoost` | 0.3–1.5 | Upwind multiplier minus 1 (e.g., 0.5 = +50% M on windward) |
| `LeeFactor` | ≈ 0.15 | Lee-side residual fraction (0.15 = 85% drying) |

**How it works:**
1. For each pixel, walk `WalkSteps` upwind along the per-cell wind tangent
2. If the path crosses a peak above `MountainCutoff`, mark this pixel as in the rain shadow
3. Multiply Whittaker M by `LeeFactor` (rain-shadow side) or `1 + WindRainBoost` (windward side)

**Visual effect:**
- Patagonia behind the Andes (cold desert)
- Atacama on the lee of the Altiplano
- Rainforests packed against windward mountain faces

**Tip:** Wind cell boundaries were smoothed in commit `4a596b9e` to remove obvious step-discontinuities at the cell joins. If you see banding, raise `WalkSteps` rather than lowering `StepArcRad`.

---

### Coastal Panel

**Purpose:** Localized roughening of pixels near coastlines via distance-to-coast modulation
**Effect:** Archipelagos, fjords, and coastal detail at every zoom level
**Formula:** `e_coast = e + α·(1 − e⁴)·(n4 + n5/2 + n6/4)·falloff`
**Renderer:** Only shown when profile's `Renderer = "rocky"` and requires `OceanLevel > 0`

**Parameters:**
- `Amp` (0–0.5) — Master strength. 0 disables. Useful 0.05–0.2.
- `Threshold` (0–1) — Distance-to-coast cutoff. Effect dies off above this.
- `Freq` (0–20) — Base fBm frequency for the n4 octave (n5/n6 derive from it).

**How it works:**
1. Compute distance-to-coast for each pixel (from `OceanLevel` height)
2. Apply bell-shaped multiplier: `(1 - e⁴)` peaks at sea level, decays inland/offshore
3. Multiply by high-frequency fBm (`n4 + n5/2 + n6/4`) for detail
4. Multiply by falloff that smoothly tapers from 1 at coast to 0 at `Threshold` distance
5. Add result to base heightmap

**Visual effect:**
- `Amp = 0` — No coastal enhancement (smooth coastlines)
- `Amp = 0.05–0.1` — Subtle coastal roughness
- `Amp = 0.15–0.2` — Dramatic archipelagos, fjords
- `Threshold = 0.05–0.2` — How far inland effect reaches
- `Freq = 1–5` — Coastal feature scale

**Why this matters:**
- Archipelagos and atolls appear at every zoom level for free
- Realistic fjord-like coastlines instead of smooth curves
- The `(1 - e⁴)` bell concentrates detail at sea level naturally

**Tip:** Combine with `Domain Warp` for even more organic coastal shapes. Use `Amp = 0.08–0.12` for realistic-looking coasts.

---

### Continents Panel

**Purpose:** Voronoi continents seeded by Fibonacci-spiral points on the unit sphere, each with a per-seed random base height
**Effect:** Establishes a continental "floor" — the heightmap is raised to that floor inside each cell so distinct continents emerge with their own characteristic elevation
**Renderer:** Only shown when profile's `Renderer = "rocky"`

This is *not* the same as Provinces:
- **Continents** assign a **base height** per cell (macro continental landmasses)
- **Provinces** modulate **noise amp/freq** per cell (regional roughness variety)
- They can coexist; Continents shape *where* land is, Provinces shape *what it looks like*

**Parameters:**
- `Seeds` (0–50) — Number of continent seeds. 0 disables. 10–50 typical.
- `WarpAmp` (0–0.3) — Sphere-warp displacement amplitude on seed positions. 0 = clean Voronoi cells.
- `WarpFreq` (0–10) — fBm frequency for the sphere-warp noise.
- `HeightLo` (0–1) — Per-continent base height lower bound. Default 0.3.
- `HeightHi` (0–1) — Per-continent base height upper bound. Default 0.7.

**How it works:**
1. Generate `Seeds` Fibonacci-spiral points on the unit sphere
2. Optionally warp each seed by low-frequency fBm (if `WarpAmp > 0`)
3. For each pixel, find nearest seed → that continent's random base height in `[HeightLo, HeightHi]`
4. Raise the heightmap to that floor inside the cell

**Visual effect:**
- `Seeds = 0` — disabled; heightmap unchanged
- `Seeds = 12, HeightLo = 0.4, HeightHi = 0.6` — Earth-like with distinct continents
- `Seeds = 30, HeightLo = 0.3, HeightHi = 0.8` — Pangaea-style supercontinent next to deep oceans
- `WarpAmp = 0.1` — organic, non-polygonal continent boundaries

**Tip:** Use sparingly with low `OceanLevel`. If `HeightLo` is below `OceanLevel`, that continent will be largely submerged.

---

### Curl Advection Panel

**Purpose:** Semi-Lagrangian backward-trace with curl-of-fBm tangent field for gas giants
**Effect:** Realistic banding, eddies, and fluid-dynamics character
**Renderer:** Only shown when profile's `Renderer = "gas_giant"`

**Parameters:**
- `Amp` (0–1) — Displacement strength per iteration. 0 disables (with `JetAmp = 0`). Useful 0.1–0.5.
- `Iterations` (0–24) — Backward-trace step count. 4–16 typical.
- `DT` (0–0.5) — Step size per iteration. 0.05–0.2 typical.
- `Freq` (0–10) — Curl-noise base frequency.
- `JetAmp` (0–1) — Zonal-jet contribution per latitude band. 0 disables.

**How it works:**
1. Each pixel: backward-trace `p_traced = p − dt·(zonal_jet(lat) + curlNoise(p_traced))`
2. Repeat for N iterations — each step compounds the displacement
3. Apply to texture sampling coordinates, not color directly
4. Result: color appears to have been advected/swirled by the velocity field

**Visual effect:**
- `Amp = 0, JetAmp = 0` — No curl advection (static bands)
- `Amp = 0.1–0.3, Iterations = 4–8` — Gentle eddies at band boundaries
- `Amp = 0.4–0.6, Iterations = 12–16` — Dramatic swirling, Kelvin-Helmholtz wisps
- `JetAmp = 0.2–0.5` — Strong zonal jet banding
- `Iterations` = 16, DT = 0.1` — Long coherent streaks

**Why this matters:**
- Single most important gas-giant change
- Matches `gaseous-giganticus` quality (open-source Jupiter renderer)
- Eliminates "stretched noise" look, produces true fluid dynamics
- Eddies, ribbons, and shears appear naturally

**Tip:** Start with `Iterations = 4–8` and `Amp = 0.2` for gentle enhancement. Increase to `Iterations = 12–16` for dramatic swirling. `JetAmp > 0.3` gives strong banding like real Jupiter.

---

### Storm Bands Panel

**Purpose:** Hand-authored oval storm overlays on gas giants (Great Red Spot, polar storms, Saturn hexagons)
**Effect:** Tints a latitude band by mixing in a chosen color over the Jupiter-ramp base palette
**Renderer:** Only shown when profile's `Renderer = "gas_giant"`

**Parameters per band:**
- `Lat` (-π/2 … π/2) — Center latitude in radians (≈ -1.57 south pole, 0 equator, +1.57 north pole)
- `HalfWidth` — Angular half-width in radians (e.g., 0.1 ≈ 5.7° tall band)
- `Color` — RGB tint
- `Strength` (0–1) — Mix amount over the underlying band palette

**UI controls:**
- **Add band** — Append a new band with default values (Lat=0, HalfWidth=0.1, red color, Strength=0.5)
- **Delete** (per row) — Remove that band

**Visual effect:**
- Empty list (or absent) — pure Jupiter-ramp banding from the base palette
- One band at `Lat = -0.4, HalfWidth = 0.05, red` — Great Red Spot analog
- Two bands near the poles — polar storm caps

**Tip:** Combine with `Curl Advection` so the bands themselves get advected into eddies and ribbons rather than sitting as flat discs.

---

### Clouds *(JSON-only — no UI panel yet)*

**Purpose:** Separate cloud-cover cube-map output (independent of the surface render), with banded coverage, domain warp, Rankine-vortex storms, and fake self-shadow from a sun direction
**Effect:** A second cube-map you can composite over the surface for atmospheric realism
**Renderer:** Rocky and rocky-like only — gas giants are excluded because their main render already produces cloud bands. Vacuum / ice-only worlds set `Coverage = 0`.
**Phase:** Phase 9a item 17.

Profile field: `Cloud` (JSON key `"cloud"`). `Coverage == 0` disables the entire pass.

| Field | Typical | Effect |
|-------|---------|--------|
| `coverage` | 0–1 | Mean cloud fraction at the equator. 0 disables. 0.4–0.6 Earth-like. |
| `bandLatRad` | ≈ π/12 | Half-width of latitude bands (Hadley cell analogue) in radians |
| `freq` | ≈ 4 | Base fBm frequency for cloud noise |
| `octaves` | ≈ 4 | fBm octave count |
| `warpAmp` | ≈ 0.4 | Domain-warp amplitude on cloud noise — produces curling fronts |
| `stormCount` | 3–8 | Number of Rankine-vortex storm centers (cyclones) |
| `stormRadiusRad` | ≈ π/16 | Storm angular radius in radians |
| `sunDir` | `[1, 0.3, 0]` | Unit vector of sun direction for fake self-shadow |
| `shadowGain` | ≈ 0.5 | Multiplier on density-gradient self-shadow term |

**How it works:**
1. Generate a domain-warped fBm cloud-density field over the cube map
2. Modulate by latitude bands of half-width `bandLatRad`
3. Add `stormCount` Rankine-vortex storms (rigid-body inner core, 1/r outer)
4. Apply fake self-shadow: brighter on the sun-facing slope of each cloud mass

**Wasm entry point:** `RenderCloudCubeMap` — separate JS API call from the main surface render. The dev server bakes both into the cube-map cross.

**Tip:** Couple `sunDir` with the same vector you intend to use for the sphere preview light so cloud shadows match terrain shading. Start with `coverage = 0.4`, `stormCount = 4` for an Earth-like atmosphere.

---

### Civilization Panel

**Purpose:** Multi-pass civilization-sign overlay — habitability scalar, Bridson-disc site placement, Zipfian population assignment, road generation (Delaunay + MST + A\*), agriculture patches, and nightside lights
**Effect:** A populated, "lived-in" planet with cities along coasts and rivers, road networks linking them, farmland around population centers, and city lights visible on the night side
**Renderer:** Rocky only — gas giants and ice worlds set `Tier = 0`.
**Phase:** Phase 9b item 18.

Profile field: `Civ` (JSON key `"civ"`). `Tier == 0` disables the entire civ pipeline.

| Field | Typical | Effect |
|-------|---------|--------|
| `tier` | 0–1 | Master strength. 0 disables. 0.3 = sparse, 0.7 = pre-industrial Earth, 1.0 = densely populated |
| `siteMinDistRad` | radians | Bridson-disc minimum site separation on the unit sphere |
| `siteMaxDistRad` | radians | Bridson-disc maximum site separation |
| `maxPopulation` | e.g. 1e7 | Population cap for the largest site (Zipfian distribution downward) |
| `nightLightHue` | 0–1 | Hue of night-side city lights (0 = warm sodium, 0.6 = cool LED) |
| `agricultureRatio` | 0–1 | Farmland-area-to-city-area ratio. 0.7 ≈ pre-industrial, 0.3 ≈ post-industrial |

**Pipeline (commit `2511b013` and predecessors):**
1. **Habitability scalar** — combines biome, slope, sea-level proximity, river distance into a per-pixel score (Task 1)
2. **Site placement** — Bridson Poisson-disc on the sphere, weighted by habitability; ocean-aware so cities don't spawn at sea (Tasks 2 + 6/4)
3. **Population assignment** — Zipfian distribution; largest city = `maxPopulation`, rank-N city = `maxPopulation/N` (Task 3a)
4. **Road generation** — Delaunay triangulation → MST backbone → A\* shortest paths between hub cities; prefers low-slope, river-following routes (Task 3b)
5. **Render passes:**
   - **City patches** — orange palette dots scaled by population
   - **Agriculture** — farmland patches around cities, sized by `agricultureRatio`
   - **Nightside** — city lights tinted by `nightLightHue`, blended via the sun-direction terminator (commit `50e91182`)

**Debug:** The Pipeline Debug View (see Render Outputs) exposes habitability, sites, roads, agriculture, and night layers as separate stage thumbnails.

**Visual effect:**
- `Tier = 0` — pristine, untouched planet
- `Tier = 0.5, agricultureRatio = 0.7` — Earth circa 1700; cities cluster along rivers and coasts, farmland visible
- `Tier = 1.0, nightLightHue = 0.55` — modern Earth with sprawling networks visible at night

**Tip:** Set `Civ.Tier > 0` and the wasm `RenderNightCubeMap` entry point will auto-bake a night-side cube map you can composite alongside the day side. Without civ enabled, no night cube is produced.

---

### Biome Table Panel

**Purpose:** Whittaker biome classification grid
**Effect:** Colors terrain based on Temperature + Humidity + height
**Only shown:** When profile has `BiomeTable` defined

**How it works:**
- Rows = Temperature buckets (0 = cold, last = hot)
- Columns = Humidity buckets (0 = dry, last = wet)
- Each cell has 2-stop gradient (Low for height=0, High for height=1)
- Bilinear OkLab interpolation between cells for smooth transitions

**Visual display:**
- Grid of gradient swatches
- Hover shows exact RGB values
- Legend: `↓ T=cold→hot (rows) → M=dry→wet (cols)`

**Why OkLab matters:**
- RGB interpolation goes through muddy gray
- OkLab/OkLCh gradients stay perceptually clean
- Desert → tundra stays vibrant, not brownish-gray

**Tip:** Edit biome colors in the Profile JSON textarea. Click "Apply & render" after changes.

---

### Color LUT Panel

**Purpose:** Per-archetype color grading for final "look unification"
**Effect:** Subtle hue/sat/value shift matching Stellaris aesthetic
**Phase:** Tier S Task 26

**How it works:**
- Each archetype has a 16³ 3D LUT (stored as `.cube` files)
- LUT applied as final pass over rendered planet
- Bypass to compare graded vs un-graded

**Controls:**
- Shows active LUT name (e.g., "terran", "arid", "jovian")
- "Bypass LUT" button disables grading temporarily
- "Restore LUT" re-enables it

**Visual effect:**
- Subtle color cast unifying all planets of same type
- "terran" LUT adds slight warm shift
- "jovian" LUT enhances band contrast

**Tip:** Toggle bypass on/off to see the difference. LUT effects are subtle but important for consistency.

---

### Palette Swatches

**Purpose:** Read-only display of legacy gradient palettes
**Shown:** For Palette, EquatorialPalette, PolarPalette (if defined)

**How they work:**
- Linear gradient of color stops at defined positions
- Used as base color when no BiomeTable is set
- Latitude-blended: Equatorial at equator, Polar at poles, mixed in between

**Visual display:**
- Horizontal gradient strip
- Hover shows position/color values

**Note:** In new pipeline, these are secondary to biome table. Terran uses both (biomes + latitude palettes).

---

## Profile JSON Section

### Purpose
Raw JSON editor for direct profile manipulation. Useful for:
- Editing biome colors
- Copying profiles between sessions
- Importing/exporting tuned presets
- Manual edits not exposed in UI

### Controls

**Apply & render** — Re-parse JSON and re-render (same as Regenerate, but ensures JSON is valid first)

**Import file** — Load a profile JSON file from disk

### Editing Guidelines

**Valid JSON required:** Syntax errors prevent rendering. Status message will show error location.

**Common manual edits:**
- Biome cell colors: `"Cells": [[{"Low":{"R":30,"G":90,"B":25},"High":{"R":90,"G":130,"B":80}},...]]`
- Palette colors: `"Palette": [{"Position":0.0,"Color":{"R":40,"G":40,"B":40,"A":255}},...]`
- Adding/removing control fields

**Format tips:**
- Use 2-space indentation (default)
- Colors as `{"R":r,"G":g,"B":b,"A":255}` or `{"R":r,"G":g,"B":b}`
- Numbers can be integers or floats

---

## Render Outputs

### Rotating Sphere Preview

**What you see:** 3D rotating planet with lighting
**Texture source:** Samples from the equirect canvas
**Interaction:**
- Click and drag to rotate manually
- Auto-rotates after 3 seconds idle
- Full 360° rotation

**Lighting model:**
- Directional light from upper-left
- Diffuse + rim lighting for depth
- Fresnel edge glow

**Use case:** Quick visual check without opening equirect

### Cube Map Cross (Hidden)

**What you see:** 6 cube faces arranged in cross pattern
**Faces:** +X, -X, +Y, -Y, +Z, -Z (cube-map convention)
**Resolution:** FaceSize × 4 wide × FaceSize × 3 tall
**Hidden by default:** View by browser dev tools or unhide in HTML

**Use case:**
- Cube-map textures for 3D engines
- Source for equirect bake
- Debug individual face issues

**Access:**
```javascript
// In browser console
document.getElementById('cube-canvas').hidden = false;
```

### Equirectangular Bake (Hidden)

**What you see:** Standard latitude/longitude projection
**Resolution:** 800×400 (configurable in app.js)
**Format:** 2:1 aspect ratio (360° longitude × 180° latitude)
**Hidden by default:** Sphere preview samples from this

**Use case:**
- Standard equirect textures
- Sphere preview texture source
- Compatible with most tools

**Projection:**
- `u = (lon / 2π) + 0.5`
- `v = 0.5 - (lat / π)`

### Pipeline Debug View

**What you see:** Collapsible panel below the viewport showing half-size equirect thumbnails for every intermediate stage of the render pipeline (heightmap, control fields, biome inputs, color stages, civ overlays, etc.)
**Use case:** See where a problem enters the pipeline — e.g., is the muddy color coming from the Whittaker lookup or the post-process LUT?

**Controls:**
- **Update debug view** — Re-bake all stage thumbnails from the current render (this is a separate, deliberate pass — main render does not produce them)
- **Clear all bypasses** — Re-enable every stage that was bypassed
- Each stage has its own bypass toggle that dry-runs the stage without zeroing its inputs (useful for A/B comparisons without losing tuning)

**Supported renderers:** Both `rocky` (flat path) and `gas_giant` / others (cube path).

**Tip:** Click a stage's bypass toggle, click "Update debug view", and compare side-by-side with the previous render. Combined with the `heightmap` view mode in the header, this is the fastest way to attribute a visual change to a single pipeline stage.

---

## Advanced Topics

### The New Pipeline Activation

The multi-noise control-field pipeline only activates when **both**:
1. At least one ControlField has non-zero Amp/Freq/etc.
2. At least one ControlField's Spline has non-empty Knots array

Otherwise, falls back to legacy single-fBm path (backward compatibility).

**Checking active pipeline:**
- Look for "Control fields" panel with sub-panels
- Check if Spline knots textareas have arrays like `[{"Input":0,"Output":0},{"Input":1,"Output":0.5}]`
- Empty `[]` knots = that field doesn't contribute to height

### Render Flow (Control Fields)

1. **Generate 5 fBm fields** — Continentalness, Erosion, PeaksValleys, Temperature, Humidity
2. **Apply splines** — Each field's Spline maps [0, Amp] output to height contribution
3. **Sum contributions** → normalize to [0,1] heightmap
4. **Apply craters** — Stamp circular depressions (if Count > 0)
5. **Color surface** — Use Palette or BiomeTable
6. **If BiomeTable set:** Lookup color from (Temperature, Humidity, height) with bilinear OkLab blend
7. **Apply overlays** — Ocean, snow line, polar caps
8. **Domain warp** — If Warp.Amp > 0, displace sphere coords before sampling (creates swirly features)

### FBM Parameters Reference

**Standard fBm:**
- `Lacunarity = 2.0` — Frequency doubles each octave
- `Persistence = 0.5` — Amplitude halves each octave
- `Octaves = 3–5` — Balance detail vs performance

**Variations:**
- Higher Persistence (0.6–0.8) — Noisier, more high-freq
- Lower Persistence (0.3–0.4) — Smoother, more blurred
- Higher Lacunarity (2.5–3.0) — Wider scale separation
- Lower Freq (0.5–1.0) — Larger features

**Control field ranges:**
- Amp: 0.5–2.0 (continentalness), 0.5–1.5 (others)
- Freq: 0.5–6.0 (higher = smaller features)
- Octaves: 2–6 (more = more detail, slower)

### Spline Tuning Guidelines

**Purpose:** Map raw noise output [0, Amp] to desired height contribution

**Fritsch-Carlson monotone cubic spline:**
- Guaranteed monotonic if inputs are monotonic
- No overshoot/oscillation
- Sorted by Input ascending

**Typical curves:**

**Continentalness (sharp coastlines):**
```json
[{"Input":0,"Output":0},{"Input":0.45,"Output":0.05},{"Input":0.55,"Output":0.6},{"Input":1,"Output":0.7}]
```
- Flat ocean (0–0.45)
- Steep coastline (0.45–0.55)
- Plateau interior (0.55–1.0)

**Erosion (smooth basins):**
```json
[{"Input":0,"Output":0},{"Input":0.3,"Output":-0.2},{"Input":0.7,"Output":-0.4},{"Input":1,"Output":-0.1}]
```
- Negative output subtracts from height
- Maximum erosion at mid-range

**PeaksValleys (add detail):**
```json
[{"Input":0,"Output":0},{"Input":1,"Output":0.3}]
```
- Simple linear ramp adds roughness uniformly

### Province Modulation Deep Dive

**Concept:** Instead of uniform noise across the whole planet, divide it into regions with different character.

**Implementation:**
1. Generate Voronoi cells (Count seeds via Fibonacci spiral on sphere)
2. Warp sphere coordinates by WarpAmp before nearest-seed lookup
3. Each cell gets random amp/freq multiplier sampled from [1-Jitter, 1+Jitter]
4. Apply multipliers to underlying control fields

**Effect on control fields:**
- Continentalness — Some regions more continuous, others more fragmented
- Erosion — Variable smoothing across regions
- PeaksValleys — Some areas rougher, others smoother

**Real-world analog:**
- Mars dichotomy (smooth northern plains, cratered southern highlands)
- Earth's cratons vs. active mountain belts

**Parameter interaction:**
- `Count = 0` — Disabled
- `Count = 8–12, Jitter = 0.2` — Subtle regional variation
- `Count = 20–30, Jitter = 0.4` — Dramatic regional differences
- `WarpAmp = 0` — Sharp polygonal boundaries
- `WarpAmp = 0.1` — Organic, natural boundaries

### Domain Warp Deep Dive

**Concept:** Before sampling noise, displace the sample position by a vector noise. This makes features "flow" instead of following straight lines.

**Quilez warp (IGR 2002):**
```
q(p) = vec3(noise(p + offset1), noise(p + offset2), noise(p + offset3))
warped_p = p + Amp · q(p)
output = fbm(warped_p)
```

**Why it works:**
- Prevents axis-aligned features
- Creates organic, swirling patterns
- Mimics fluid flow, geological processes

**Visual effects:**
- Coastlines become fjord-like instead of straight
- Mountain ranges curve and meander
- Biome boundaries look natural, not latitudinal

**Parameter guide:**
- `Amp = 0` — No warp (straight features)
- `Amp = 0.02–0.05` — Subtle organic feel (terran default)
- `Amp = 0.1–0.2` — Dramatic swirling
- `Amp > 0.3` — Chaotic, may break features

**Interaction with other systems:**
- Warp applied BEFORE sampling control fields, ridged, craters
- Provinces WarpAmp is separate — warps Voronoi lookup only
- Can use both for compound organic effect

### Biome System Deep Dive

**Whittaker classification (real ecology):**
- Biomes determined by temperature + moisture
- 2D lookup: cold/dry = tundra, hot/wet = rainforest
- Used by Minecraft 1.18, Dwarf Fortress, Azgaar, etc.

**Implementation:**
1. Temperature field combined with `cos(latitude)` — poles colder
2. Humidity field sampled directly — no latitude bias
3. (T, M) sampled from [0,1]²
4. Map to cell indices: `t_row = floor(T · TBuckets)`, `m_col = floor(M · MBuckets)`
5. Get cell's Low/High colors
6. Sample height from [0,1], interpolate between Low/High
7. Bilinear blend with neighboring cells for smooth transitions
8. Convert RGB → OkLab → interpolate → convert back (prevents muddy grays)

**Why OkLab matters (color science):**
- RGB is perceptually non-uniform
- Straight RGB lines go through gray/brown
- OkLab is perceptually uniform
- OkLab gradients stay vibrant

**Example color journey:**
- Desert (hot/dry) → Forest (warm/wet)
- RGB: goes through brownish-gray
- OkLab: stays vibrant, moves through clean greens/golds

**Biome table structure:**
```json
{
  "TBuckets": 4,
  "MBuckets": 4,
  "Cells": [[
    {"Low":{"R":200,"G":210,"B":220},"High":{"R":240,"G":245,"B":250}}, // T=0,M=0 cold/dry
    ...
  ], ...]
}
```

**Visual result:**
- Deserts and rainforests at same latitude (realistic)
- Smooth transitions, no hard boundaries
- Height variation within each biome (Low/High gradient)

### Shading System Deep Dive

**Lambertian diffuse reflectance:**
- Light intensity = `max(0, dot(N, L))`
- N = surface normal, L = light direction
- Classic physically-based lighting

**Implementation:**
1. Compute heightmap gradient via finite difference
2. Normalize gradient to get surface normal
3. Exaggerate gradient by ShadingExaggeration (optional)
4. Calculate dot product with light direction
5. Modulate color by lighting

**Why exaggeration matters:**
- Subtle height variations = subtle normals = flat lighting
- Exaggeration amplifies small gradients
- Makes low-relief terrain visible

**Visual effects:**
- `Strength = 0` — Flat, cartoony (no 3D cue)
- `Strength = 0.5` — Gentle depth
- `Strength = 1.0, Exaggeration = 20` — Dramatic relief
- Use heightmap view to see pure shading

**Light direction:**
- Fixed at `(-0.85, 0.30, 0.45)` in the sphere preview (`cmd/planet-explorer/web/app.js`)
- Phase 9b shifted this from the older `(-0.4, 0.5, 0.7)` value so the terminator centers on the visible disc at default rotation, making the day/night blend (and night-side city lights, when civ is enabled) read clearly
- Same for all renders; for civ night-side baking, the wasm side derives a matching sun direction so terminator alignment is consistent

### Crater System Deep Dive

**Simplified implementation (current):**
- Poisson-disc sampling for non-overlapping centers
- Uniform radius distribution between Min/Max
- Fixed depth for all craters

**Real-world physics (not implemented):**
- Power-law size distribution (Hartmann/Neukum production functions)
- `N(>D) ∝ D^(-α)` where α ≈ -2 to -3.5
- More small craters than large ones

**Enhanced crater system (future, see Compass §1):**
- **Age per crater** — Older craters more eroded (amplitude decay)
- **Ejecta rays** — Albedo features, not topography
- **Secondary craters** — Clusters around primaries
- **Floor types** — Bowl → Flat → CentralPeak → PeakRing → MareFlooded
- **Spatial density mask** — Maria vs highlands (lunar), dichotomy (Mars)

**Current use:**
- Barren bodies (Mercury, Moon, Mars, Callisto)
- Set Count=0 for lifebearing worlds (Earth, super_terran)

**Parameter guide:**
- `Count = 0` — No craters
- `Count = 10–30` — Light cratering (early Mars)
- `Count = 50–100` — Moderate cratering (Moon)
- `Count = 150–200` — Heavy cratering (Mercury, Callisto)
- `Depth = 0.02–0.1` — Shallow craters
- `Depth = 0.2–0.3` — Deep craters

---

## Tips and Tricks

### Performance Tuning

**Fast iteration (tuning phase):**
- Face size 128 or 256
- Octaves 3–4
- Few provinces (Count < 12)
- Disable craters temporarily

**Final render:**
- Face size 512 or 1024
- Full octaves (5–8)
- Full province count
- All features enabled

**Render time budget:**
- 64 face: ~200ms
- 256 face: ~1s
- 512 face: ~5-10s
- 1024 face: ~10-20s

### Planet Type Tuning

**Earth-like (terran):**
- `OceanLevel = 0.5`
- `HasPolarCaps = true`, `PolarCapSize = 0.15`
- `SnowLine = 0.78`
- Moderate warp (`Amp = 0.03`)
- Biome table with 4×4 or 5×5 cells

**Mars-like:**
- `OceanLevel = 0` or very low (0.1)
- `HasPolarCaps = true`, `PolarCapSize = 0.2`
- `CraterCount = 50–80`
- Red/orange palette
- Provinces for dichotomy (Count = 12, Jitter = 0.4)

**Ice world (Europa/Enceladus):**
- `OceanLevel = 0.8` (subsurface ocean)
- High `SnowLine` (0.3–0.5)
- Blue/white palette
- Low `CraterCount` (young surface)

**Lava world:**
- `OceanLevel = 0`
- Black/red/orange palette
- High `PeaksValleys` freq for cracked look
- No snow, no ice caps

**Jovian (gas giant with curl advection):**
- `Curl.Amp = 0.2–0.4` for realistic eddies
- `Curl.Iterations = 8–12` for gentle swirling
- `Curl.JetAmp = 0.3–0.5` for strong banding
- Combine with `BandCount = 6–8` and `StormBands` for authentic Jupiter look

### Common Patterns

**Sharp coastlines:**
- Continentalness spline with steep middle section
- `{"Input":0.45,"Output":0.05},{"Input":0.55,"Output":0.6}`
- Combine with low warp for defined continents

**Natural-looking mountains:**
- Moderate PeaksValleys (Freq 2–3, Octaves 5–6)
- Add Ridged (Amp 0.1–0.2) for sharp peaks
- Mask ridges via Continentalness (MaskLow 0.4, MaskHigh 0.7)

**Regional variety:**
- Provinces with Count = 12–20
- Jitter = 0.3–0.4 for noticeable difference
- WarpAmp = 0.05–0.1 for organic boundaries

**Swirly organic features:**
- Warp Amp = 0.05–0.1
- Apply to control fields for curled coastlines
- Higher warp (>0.15) for dramatic effect

**Realistic river channels (Tier A erosion):**
- Enable erosion with `Droplets = 100,000` (face=256)
- Set `BrushFalloff = 3–5` for narrow, realistic rivers
- Combine with `OceanLevel = 0.4–0.6` for coastal drainage

**Archipelago coastlines (Tier A coastal):**
- Enable coastal with `Amp = 0.1–0.15`
- Set `Threshold = 0.1–0.2` for moderate reach
- Combine with `OceanLevel = 0.5` for sea level reference
- Use `Freq = 2–4` for archipelago scale

### Debugging

**View heightmap mode:**
- Pure elevation without color
- See underlying noise structure
- Check spline effects

**Disable features incrementally:**
- Set `CraterCount = 0` to see terrain without craters
- Set `Warp.Amp = 0` to see axis-aligned baseline
- Set `Provinces.Count = 0` to check uniform noise
- Bypass LUT to see ungraded colors

**Check parameter values:**
- Hover any slider/row for inline help
- Read the Help / Parameter reference panel
- Export JSON and inspect values directly

### Common Issues

**Planet looks like noise soup:**
- Reduce Persistence (try 0.4–0.5)
- Reduce Octaves (try 3–4)
- Check if multiple features are stacking too high

**Coastlines are too straight:**
- Add warp (start with Amp = 0.03)
- Increase Continentalness Freq (try 2–4)
- Add more spline knots for nuanced curve

**Mountains are everywhere:**
- Add Ridged masking (MaskLow/High)
- Reduce PeaksValleys Amp
- Adjust Continentalness spline to flatten mid-range

**Biome colors are muddy:**
- Check if OkLab interpolation is active (it should be)
- Reduce biome table size (try 3×3 instead of 5×5)
- Adjust cell colors for better contrast

**Performance is slow:**
- Reduce Face Size (try 256 or 128)
- Reduce Octaves (try 3–4)
- Reduce Provinces Count
- Disable craters temporarily

---

## Export and Import

### Exporting Profiles

**Workflow:**
1. Tune planet to desired look
2. Click "Export JSON"
3. Paste into `pkg/planetgen/profile.go`
4. Add to Profiles map with unique key
5. Rebuild and test

**Format:**
```go
"my_planet": {
    Type:     "my_planet",
    Renderer: "rocky",
    Palette: []planetcolor.ColorStop{...},
    ControlConfig: types.ControlConfig{...},
    // ... other fields
},
```

### Importing Profiles

**Via file:**
1. Click "Import file" button
2. Select `.json` file
3. Profile loads into textarea
4. Panels rebuild to show imported parameters

**Via paste:**
1. Paste JSON into Profile JSON textarea
2. Click "Apply & render"
3. Validates and renders

**Validation:**
- Invalid JSON shows error message
- Missing fields use defaults
- Unknown fields are ignored

---

## Keyboard Shortcuts

- **Enter** — Regenerate (when focused on sliders/inputs)
- **Tab** — Navigate between inputs
- **Space** — Toggle checkboxes (when focused)

---

## Browser Compatibility

**Tested on:**
- Chrome/Edge 90+
- Firefox 88+
- Safari 14+

**Requirements:**
- WebAssembly support
- Canvas 2D context
- ES6 JavaScript

**Performance notes:**
- Chrome fastest (V8 engine)
- Firefox slightly slower
- Safari slowest but functional

---

## Troubleshooting

### Wasm fails to load

**Symptoms:** "Loading wasm…" hangs, "Error: WebAssembly.compile" 
**Solutions:**
- Check that `.wasm` file exists in `web/` directory
- Rebuild wasm: `GOOS=js GOARCH=wasm go build ...`
- Check browser console for specific errors
- Try different browser

### Render errors

**Symptoms:** Status shows "Error: ..." after render
**Solutions:**
- Check Profile JSON textarea for syntax errors
- Validate JSON at https://jsonlint.com
- Check that required fields exist (ControlConfig, etc.)
- Try clicking "Apply & render" instead of "Regenerate"

### Missing panels

**Symptoms:** Some panels don't appear in sidebar
**Solutions:**
- Check planet type — some panels are renderer-specific (e.g., Craters only for "rocky")
- Check that profile has required fields (BiomeTable, Provinces, etc.)
- Toggle panels open — they may be collapsed

### Performance issues

**Symptoms:** Renders take >30 seconds

The Wasm pipeline runs in a dedicated Web Worker (Phase 14), not the
main thread, so a slow render no longer freezes the tab or triggers a
"Page Unresponsive" dialog — the busy overlay stays interactive and
you can **Cancel** at any time. If a render is simply taking too long:
**Solutions:**
- Reduce Face Size to 256 or 128
- Reduce Octaves to 3–4
- Disable Provinces (Count = 0)
- Disable Craters (Count = 0)
- Click **Cancel** and lower parameters before retrying

---

## Further Reading

### Compass Artifact
The comprehensive procedural generation guide that inspired these enhancements:
- `docs/compass_artifact_wf-5839e7e9-3eea-43f4-a351-b47c001ca782_text_markdown.md`
- Covers all techniques: domain warping, craters, civilization signs, clouds, gas giants, etc.
- Explains the "why" behind each parameter

### Key References

**Minecraft 1.18 worldgen:**
- Henrik Kniberg JFokus 2022 talk
- Multi-noise control fields approach
- Nearest-bucket biome lookup

**Red Blob Games:**
- Introduction to procedural generation
- Voronoi diagrams, noise, erosion
- Map generation recipes

**Domain warping:**
- Inigo Quilez IGR 2002
- "Domain warping" for organic features

**Ridged multifractal:**
- Musgrave, libnoise
- Sebastian Lague implementation

**Whittaker biomes:**
- Real ecology classification
- Used by Dwarf Fortress, Azgaar, mapgen4

### Code Locations

**Wasm API:** `cmd/planet-explorer/wasm/main.go`
**UI logic:** `cmd/planet-explorer/web/app.js`
**Profile definitions:** `pkg/planetgen/profile.go`
**Type definitions:** `pkg/planetgen/types/types.go`
**Rocky renderer:** `pkg/planetgen/render/rocky.go`
**Gas giant renderer:** `pkg/planetgen/render/gas_giant.go`
**Cloud generation:** `pkg/planetgen/feature/clouds.go` (`GenerateClouds`)
**Cloud cube-map entry:** `pkg/planetgen/render/rocky.go:123` (`RenderCloudCubeMap`)
**Night cube-map entry:** `pkg/planetgen/render/rocky.go:83` (`RenderNightCubeMap`)
**Civ pipeline:** `pkg/planetgen/feature/civ.go`, `habitability.go`, `roads.go`, `poisson_sphere.go`
**Plates / SDF:** `pkg/planetgen/field/plates.go`, `jfa.go`
**Flow / rivers (D8 + Planchon-Darboux):** `pkg/planetgen/field/flow.go`
**Rain shadow:** `pkg/planetgen/biome/rainshadow.go` (`GenerateRainShadow`)
**Continents:** `pkg/planetgen/field/continents.go`
**Height smoothing:** `pkg/planetgen/field/smooth.go`
**Debug stages:** `pkg/planetgen/render/debug.go` (`RenderRockyDebug`)

---

## Appendix: Parameter Quick Reference

### Control Fields (fBm)
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Amp | 0–2 | 0.5–1.5 | Output magnitude |
| Freq | 0.5–8 | 0.5–5 | Feature size |
| Octaves | 1–8 | 3–5 | Detail layers |
| Lacunarity | 1.5–3 | 2.0 | Freq multiplier |
| Persistence | 0.2–0.8 | 0.5 | Amp decay |

### Domain Warp
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Amp | 0–0.4 | 0.03 | Warp strength |
| Freq | 0.5–4 | 0.5–5 | Warp scale |
| Octaves | 1–3 | 1–3 | Warp detail |

### Ridged Mountains
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Amp | 0–0.3 | 0.1–0.2 | Mountain height |
| Freq | 0.5–5 | 0.5–5 | Ridge density |
| Octaves | 4–6 | 4–6 | Detail |
| Gain | 0.4–0.6 | 0.5 | Sharpness |
| Offset | 0.8–1.2 | 1.0 | Peak sharpness |

### Provinces
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Count | 0–64 | 8–32 | Cell count |
| Jitter | 0–0.5 | 0.1–0.4 | Variety |
| WarpAmp | 0–0.3 | 0–0.15 | Boundary curve |

### Shading
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Strength | 0–1 | 0–0.5 | Light intensity |
| Exaggeration | 0–50 | 0–20 | Relief drama |

### Ocean
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| OceanLevel | 0–1 | 0–0.8 | Sea level |
| OceanColor | RGB picker | blue | Water color |

### Cryosphere
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| PolarCapSize | 0–0.5 | 0.1–0.3 | Cap extent |
| PolarCapNoise | 0–0.5 | 0–0.2 | Edge roughness |
| SnowLine | 0–1 | 0.7–0.9 | Snow elevation |

### Erosion
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Droplets | 0–500000 | 50000–150000 | Particle count (auto-scaled by face area) |
| Inertia | 0–1 | 0.05 | Velocity preservation (higher = straighter channels) |
| Capacity | 0–20 | 4 | Sediment capacity (higher = more carried) |
| ErosionRate | 0–1 | 0.3 | Carving fraction per step |
| Deposition | 0–1 | 0.3 | Sediment drop fraction per step |
| Evaporation | 0–0.1 | 0.01 | Water loss per step |
| MinSlope | 0–0.5 | 0.01 | Capacity floor to avoid zero |
| MaxStepsPerDrop | 0–200 | 50 | Hard cap on steps per droplet |
| Gravity | 0–20 | 4 | Speed gain from downhill slope |
| BrushFalloff | 0–16 | 0–2 | Brush sharpness (higher = narrower rivers) |

### Coastal
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Amp | 0–0.5 | 0.05–0.2 | Roughening strength |
| Threshold | 0–1 | 0.05–0.2 | Distance-to-coast cutoff |
| Freq | 0–20 | 1–5 | fBm base frequency |

### Craters
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Count | 0–200 | 0–100 | Crater density |
| MinRadius | 0–0.05 | 0.001–0.01 | Smallest size |
| MaxRadius | 0–0.2 | 0.01–0.1 | Largest size |
| Depth | 0–0.5 | 0.02–0.3 | Crater depth |

### Continents
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| Seeds | 0–50 | 0 | Continent count (0 disables) |
| WarpAmp | 0–0.3 | 0 | Sphere-warp on seed positions |
| WarpFreq | 0–10 | 0 | Sphere-warp fBm frequency |
| HeightLo | 0–1 | 0.3 | Per-continent base height min |
| HeightHi | 0–1 | 0.7 | Per-continent base height max |

### Height Smoothing
| Param | Range | Default | Effect |
|-------|-------|---------|--------|
| HeightSmoothRadius | 0–8 | 0 | Disc-blur radius pre-normalize |

### Storm Bands (gas giant)
| Param | Range | Effect |
|-------|-------|--------|
| Lat | -π/2 … π/2 | Center latitude (radians) |
| HalfWidth | radians | Band angular half-width |
| Color | RGB | Band tint |
| Strength | 0–1 | Mix amount over base palette |

### Plate Tectonics *(top-level fields)*
| Field | Default | Effect |
|-------|---------|--------|
| PlateCount | 0 | Number of Voronoi plates (0 disables) |
| OceanicPlateFraction | 0 | Fraction of plates flagged oceanic |
| PlateConvergentT | 0 | Velocity-dot threshold for convergent boundary |
| RadiusKm | 0 = 6371 | Planet radius for SDF scaling |

### Jitter (top-level fields)
| Field | Default | Effect |
|-------|---------|--------|
| JitterEnabled | false | Toggle Voronoi cell jitter on Detail field |
| JitterCellCount | 0 | Number of jitter cells |
| JitterRotMax | 0 | Max rotation per cell (radians) |
| JitterOffsetMax | 0 | Max coordinate offset per cell |

### Flow (Rivers)
| Param | Range | Effect |
|-------|-------|--------|
| RiverThreshold | 0–1 | Accum cutoff (0 disables) |
| RiverDepth | typical 0.005–0.03 | Carved depth |

### Rain Shadow
| Param | Typical | Effect |
|-------|---------|--------|
| WalkSteps | 0 disables; 8–24 | Upwind walk count |
| StepArcRad | ≈ 0.087 (5°) | Arc per step (radians) |
| MountainCutoff | 0.55–0.75 | Height above which uplift counts |
| WindRainBoost | 0.3–1.5 | Upwind multiplier minus 1 |
| LeeFactor | ≈ 0.15 | Lee residual fraction |

### Cloud
| Param | Typical | Effect |
|-------|---------|--------|
| coverage | 0–1 (0 disables) | Mean equatorial cloud fraction |
| bandLatRad | π/12 | Band half-width (radians) |
| freq | 4 | Base fBm frequency |
| octaves | 4 | fBm octaves |
| warpAmp | 0.4 | Domain-warp amplitude |
| stormCount | 3–8 | Rankine-vortex storm count |
| stormRadiusRad | π/16 | Storm angular radius |
| sunDir | (1, 0.3, 0) | Sun direction unit vector |
| shadowGain | 0.5 | Self-shadow density multiplier |

### Civ
| Param | Typical | Effect |
|-------|---------|--------|
| tier | 0–1 (0 disables) | Master strength |
| siteMinDistRad | radians | Bridson minimum site separation |
| siteMaxDistRad | radians | Bridson maximum site separation |
| maxPopulation | e.g. 1e7 | Cap for largest site (Zipfian) |
| nightLightHue | 0–1 | Night-light hue |
| agricultureRatio | 0–1 | Farmland-to-city area ratio |

---

**Version:** 1.2  
**Last updated:** 2026-05-06  
**For Planet Explorer:** Phase 0 Cube Map implementation
