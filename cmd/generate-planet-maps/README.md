# generate-planet-maps

Batch generator that produces planet surface imagery for every planet
in the kb knowledge database. Output is a 4×3 horizontal cross
cube-map PNG (the canonical storage format) and a 2000×1000
equirectangular PNG (the kb-page thumbnail) per planet.

## What it does

For each `pois` row with `type='planet'` and a non-empty `class`,
the generator looks up a per-type `PlanetProfile`, derives a
deterministic seed from the planet's name (`fnv64a`), invokes the
matching renderer (`rocky` or `gas_giant`), and writes two PNGs.
Both are produced in parallel (8 workers by default) using
`pkg/planetgen`.

## Inputs

- **Knowledge database** (`-db`, default `../spacemolt-knowledge.db`):
  reads `pois.name`, `pois.class`, `systems.name` from `pois` joined
  with `systems` filtered to `type='planet' AND class != ''`.
- **Per-type `PlanetProfile`** in `pkg/planetgen/profile.go`:
  palette, noise, crater, ocean, polar-cap, and band parameters.
- **Seed**: planet name → fnv64a → int64. Same name always produces
  the same image.

## Outputs

For every planet, two files in `-outdir` (default `kb/images/planets`):

| File | Format | Size | Role |
|---|---|---|---|
| `<sys>_<planet>.cube.png` | 4×3 horizontal cross PNG | 4096×3072 (S=1024) | Canonical storage, sphere-native, GL-compatible |
| `<sys>_<planet>.png` | Equirectangular PNG | 2000×1000 | kb-page thumbnail, baked from the cube map |

The cube-map cross layout (face cells, top-left origin):

```
[empty][ +Y  ][empty][empty]
[ -X  ][ +Z  ][ +X  ][ -Z  ]
[empty][ -Y  ][empty][empty]
```

Empty cells are transparent (alpha=0). Face order matches OpenGL
`GL_TEXTURE_CUBE_MAP_POSITIVE_X` so the PNG can be uploaded to a
WebGL cube texture without remapping.

## Running the tool

Single-image mode (testing):

```bash
go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/earth.png
# Produces /tmp/earth.png and /tmp/earth.cube.png.
```

Batch mode (default):

```bash
go run ./cmd/generate-planet-maps
# Reads ../spacemolt-knowledge.db, writes to kb/images/planets/.
```

Single-system mode (subset of batch — re-render every planet in one system):

```bash
go run ./cmd/generate-planet-maps -system "Sol" -outdir /tmp/sol-test
# Filters batch to pois rows whose system matches; useful for previewing
# a tuning change against a familiar handful of planets.
```

All flags:

| Flag | Default | Meaning |
|---|---|---|
| `-type` | "" | single planet type (must be paired with `-seed`) |
| `-seed` | "" | planet name (single mode only) |
| `-out` | "" | equirect output path (single mode); defaults to `<type>_<seed>.png` |
| `-db` | `../spacemolt-knowledge.db` | knowledge database path |
| `-outdir` | `kb/images/planets` | batch output directory |
| `-system` | "" | restrict batch mode to one system by name (exact match) |
| `-face` | 1024 | cube-map face size in pixels |
| `-width` | 2000 | equirect bake width |
| `-height` | 1000 | equirect bake height |
| `-workers` | 8 | parallel worker count (batch mode) |

## Architecture

The generator is layered across `pkg/planetgen` subpackages:

| Subpackage | Role |
|---|---|
| `pkg/planetgen` (root) | `Generate`, `GenerateEquirect`, `Profiles`, `GetProfile` |
| `pkg/planetgen/cubemap` | `CubeMap`, `CubeMapF`, GL-convention sample/cross/bake |
| `pkg/planetgen/noise` | `Generator`, FBM, future warp/ridged/curl |
| `pkg/planetgen/color` | `ColorStop`, `SampleGradient`, `Blend`, `Lerp`, `Brighten`; future OkLab/LUT |
| `pkg/planetgen/feature` | `Crater`, `GenerateCraters`, `ApplyCraters`; future civ/dunes/clouds |
| `pkg/planetgen/render` | `RenderRocky`, `RenderGasGiant` (orchestration) |
| `pkg/planetgen/types` | `PlanetProfile` struct (leaf package; breaks root↔render import cycle) |
| `pkg/planetgen/field` | `GenerateControlFields` (5-fbm with named-domain seeds) |
| `pkg/planetgen/biome` | `GenerateClimateFields`, `LookupColor` (Whittaker T+M) |
| `pkg/planetgen/seed` | `Hash`, `Domain` (orthogonal sub-seed mixing) |
| `pkg/planetgen/stats` | per-planet physical-property metadata generator |

## Phase 1

The render pipeline gained five Tier-S algorithms:

- **OkLab biome blending** (`pkg/planetgen/color/oklab.go`) — `BlendOkLab` /
  `SampleGradientOkLab` replace the RGB-space lerp in both renderers.
  Produces cleaner color transitions, especially across saturated palette
  stops. The legacy `Blend` / `SampleGradient` remain for explicit RGB ops
  (e.g., `Lerp`, `Brighten`).
- **Multi-noise control fields with monotone cubic splines**
  (`pkg/planetgen/field/control.go`, `pkg/planetgen/color/spline.go`).
  Five 3D-fBm fields (Continentalness, Detail, PeaksValleys, Temperature,
  Humidity), each seeded by `seed.Domain(master, "control.<name>")` so
  adding a new field never shifts existing field outputs. Each
  `ControlField` carries its own Fritsch-Carlson monotone-cubic
  `Spline`. The heightmap is the sum of the first three fields'
  spline outputs (Continentalness + Detail + PeaksValleys);
  Temperature and Humidity are climate fields consumed by the
  Whittaker biome lookup downstream and do not contribute to
  elevation. `RenderRocky` falls back to the legacy single-fBm path
  when `ControlConfig` is zero or no field has spline knots.
- **Domain warping** (`pkg/planetgen/noise/warp.go`) — Quilez per-axis
  fBm warp applied at every per-pixel sphere lookup in both renderers.
  `Warp.Amp == 0` short-circuits to identity; non-zero produces curling,
  non-axis-aligned features.
- **Whittaker T+M biome lookup** (`pkg/planetgen/biome/`) — climate
  fields driven by Temperature noise + cos(latitude) bias and Humidity
  noise. A profile's `BiomeTable` (a 2D grid of 2-stop palettes indexed
  by T and M) is bilinearly OkLab-blended per pixel. Profiles without a
  `BiomeTable` continue to use the legacy `Palette` / `EquatorialPalette`
  / `PolarPalette` path.
- **Per-archetype color LUT** (`pkg/planetgen/color/lut.go`,
  `pkg/planetgen/color/luts/*.cube`) — every planet type ships a 16³
  Resolve-format LUT loaded via `//go:embed`. Applied as the final
  per-pixel grade in both renderers; subtle 5–10 % hue/sat/value shifts
  unify the look across an archetype. Bypass at runtime by setting
  `profile.LUT = nil`.

The interactive slider tool at `cmd/planet-explorer/` is the canonical
workflow for tuning these parameters live in a browser.

## Phase 4

Four Tier-A items shipped in this phase, plus a clarifying rename:

- **JFA distance-to-coast** (`pkg/planetgen/field/jfa.go`) — jump-flooding
  distance field over the cube-sphere; reusable primitive consumed by
  coastal noise (Phase 4) and erosion (Phase 5).
- **Coastal noise enhancement** (`pkg/planetgen/noise/coastal.go`) — three
  high-frequency fBm bands modulated by distance-to-coast for crinklier
  shorelines: `e_coast = e + α·(1 − e⁴)·(n4 + n5/2 + n6/4)·falloff`.
  Gated on `Coastal.Amp > 0`; dormant by default.
- **Voronoi continents** (`pkg/planetgen/field/continents.go`) —
  Fibonacci-spiral seed points on the unit sphere with low-frequency
  warp; per-pixel nearest seed becomes a base-height contribution that
  the existing fBm noise then varies on top. Gated on `Continents.Seeds > 0`.
- **Curl-noise gas-giant advection** (`pkg/planetgen/noise/curl.go`) —
  semi-Lagrangian backward-trace using the curl of an fBm vector field
  plus a sinusoidal zonal jet. Produces ribbon-flow appearance on jovian
  and ice_giant. Gated on `Curl.Amp > 0 || Curl.JetAmp > 0`.
- **Cassini Jupiter ramp + StormBand** (`pkg/planetgen/color/jupiter_ramp.go`)
  — embedded 256×1 latitude-driven base palette for gas giants (OkLab-blended
  between four hand-picked stops) plus a `StormBands []StormBand` slice on
  the profile for hand-authored colored ovals (Great Red Spot, polar collars).

The Phase-1 `Erosion` control field was renamed to `Detail` to free the
name for Phase-5 flow-based erosion. Existing JSON dumps with the old key
still load via a custom `UnmarshalJSON` shim on `ControlConfig`.

## Phase 6 — Pipeline debug view (current)

The slider tool's debug panel (collapsible at the bottom of the page)
shows each rocky-pipeline stage as a row of half-size equirect
thumbnails:

- **raw** — the stage's signed scalar contribution as a cube map.
  Negative pixels render in red (subtractive layers like crater bowls
  and coastal noise are visible at a glance).
- **input bands / output bands** — for stages with splines (the five
  control fields), pixels classified by which knot interval they fall
  into, on the input axis or the output axis.
- **sum after** — the running heightmap after this stage applied.

Color stages (Palette, Ocean, Snow, Polar Caps, Ejecta, LUT) appear
below the heightmap stages, each as a single thumbnail of the cube map
*after* that stage applied. The LUT row is intentionally bypassable —
toggling it lets the underlying palette/biome color come through
unmodified.

Each row has a "bypass" checkbox. Bypassing a stage skips its
contribution while keeping the rest of the pipeline noise streams
unchanged (seeds and rng draws stay deterministic), so the operator
can isolate the effect of any single stage without zeroing parameters.
The panel auto-refreshes after every main render when it is open;
collapse it to skip the expensive PNG-encoding pass.

Implemented under `pkg/planetgen/render/debug.go` and
`pkg/planetgen/render/debug_palette.go`; surfaced via the
`planetExplorerGenerateDebug` wasm export and rendered into a
`<details id="debug-panel">` block in the explorer UI.

## Phase 5 — Particle hydraulic erosion

Master-plan item 9. Droplets walk the cube-sphere in 3D unit-sphere
coordinates, sampling height + gradient via seam-aware
`cubemap.CubeMapF.Sample` so cross-face propagation is implicit. Each
droplet carries water + sediment; capacity is `slope · speed · water ·
capacity`; depositing happens when sediment exceeds capacity or the
next step is uphill, eroding otherwise. Output is a strongly-subtractive
heightmap delta with dendritic channels and alluvial fans.

Implemented in `pkg/planetgen/field/erosion.go`. Wired into
`RenderRocky` between Coastal and Craters. Profile knob
`Erosion.Droplets` is the canonical count at face=1024; the renderer
auto-scales to face area, with a 5000-droplet floor so face=64 previews
still communicate the look.

### Ocean awareness
- Droplets that spawn below `OceanLevel` are skipped.
- Droplets flowing into ocean carve a "river-mouth notch" at the coast
  (`oceanLevel - 0.01`) instead of depositing — this is what makes river
  outlets visibly cut into the shoreline rather than disappearing.
- Erosion never carves below `oceanLevel - 0.01` anywhere else.

### Tuning recipe for visible rivers

Three knobs together produce the dendritic look:
- **`HeightSmoothRadius` 2-4** — disc blur applied to the heightmap
  before erosion, suppresses fbm popcorn so droplets see continental-
  scale slope instead of pinning at every per-pixel local minimum.
- **`Erosion.MaxStepsPerDrop` 100-250** — longer paths let each droplet
  cover more terrain before terminating, increasing chances of multiple
  droplets sharing routes (which reinforces channels into trunks).
- **`Erosion.BrushFalloff` 4-8** — exponent in the `1/(1+r)^k` brush
  weights. k=1 is diffuse 3-pixel-wide channels; k=4 concentrates ~73%
  of mass on the center pixel; k=8 is near-single-pixel rivers. Tune up
  for narrower, more drawn-looking channels.

Plus optionally:
- **`Erosion.Inertia` 0.10-0.15** — moderate momentum so droplets don't
  pinball locally but still respond to terrain.
- **`Erosion.MinSlope` 0.1** — floors capacity on flat terrain so
  channels keep carving across plains.

### Tuned defaults

- **terran, super_terran, tundra, oceanic** — full tuned recipe
  (`Droplets=250000, BrushFalloff=7, MaxStepsPerDrop=250,
  HeightSmoothRadius=4`)
- **arid** — earlier conservative tuning (Capacity=6, Deposition=0.5,
  HeightSmoothRadius=2)
- **glacial, ice_world** — lighter erosion (~50k droplets,
  HeightSmoothRadius=2)
- **scorched, hothouse, lava_world, unknown, jovian, ice_giant** —
  disabled

### Other Phase 5 follow-ups

- **Heightmap disc-blur stage** (`HeightSmoothRadius`) sits between
  Continents and Normalize. Per-face `1/(1+r)` falloff blur with
  per-pixel re-normalization at edges. Suppresses high-frequency fbm
  popcorn so erosion can form coherent channels.
- **Flat 2D render path** in `pkg/planetgen/render/flat.go` provides a
  fast iteration preview that bypasses the cube-sphere. Used by the
  planet-explorer's "swatch" view. Mirrors the cube colorize stage-by-
  stage but skips Continents/Voronoi/Craters/PolarCaps. Includes
  per-pipeline LRU cache keyed by upstream-affecting params so
  downstream tweaks are nearly free.

Tier A (master plan items 6–13) is now complete. Tier B (items 14–20)
and Tier C (items 21–25) remain.

## Testing and the golden-diff workflow

Three layers of tests:

1. **Math primitives** in each subpackage — direction round-trip,
   cube-map sample at flat colour, cross PNG round-trip, bake
   identity. Pure-math, deterministic, fast.

2. **Statistical invariants** in `cmd/generate-planet-maps/invariants_test.go`:
   for each of 13 planet types and a fixed seed, assert no NaN/Inf,
   alpha=255 everywhere, ≥4 distinct luminance buckets, terran
   ocean/land ratio in `[0.30, 0.85]`, sphere-continuity at the +X axis.

3. **Golden images** in `cmd/generate-planet-maps/testdata/golden/`:
   one 400×200 equirect PNG per planet type, generated from a fixed
   seed (`GoldenScorched`, `GoldenArid`, …). The
   `TestGolden` harness regenerates each image and asserts mean
   ΔE2000 vs. the committed golden is < 1.5.

### Running the diff locally

```bash
cd /home/robert/spacemolt/kb
go test ./cmd/generate-planet-maps/... -run TestGolden
```

Expected output: `--- PASS: TestGolden/<type>` for each of 13 types.
On failure, the test prints the mean ΔE2000 it observed.

### Reviewing a golden diff

When a PR changes a golden image, the reviewer should:

1. Pull the branch.
2. Run `go test ./cmd/generate-planet-maps/... -run TestGolden -v`.
3. For any sub-test that fails (i.e. ΔE2000 above the gate),
   regenerate the diff PNG locally:
   ```bash
   go run ./cmd/generate-planet-maps -type <type> -seed Golden<TitleCase> -out /tmp/new.png
   go run ./cmd/tools/planet-image-diff /tmp/new.png \
       cmd/generate-planet-maps/testdata/golden/<type>.png \
       /tmp/<type>-diff.png
   ```
4. Open `/tmp/<type>-diff.png` (left=new, middle=old, right=4×
   amplified per-pixel difference). Confirm the visible change
   matches the PR description (e.g. "OkLab makes the polar-cap
   edge less muddy — yes that's the change"). Verify the printed
   `mean ΔE2000` is in the ballpark expected for the change
   described.
5. If the change is intended, the PR author runs:
   ```bash
   go test ./cmd/generate-planet-maps/... -run TestGolden -update
   git add cmd/generate-planet-maps/testdata/golden/
   git commit -m "Update goldens for <feature>"
   ```
6. If the change is unintended, fix the bug instead.

**Rule:** never run `-update` without a PR description that
explains *why* the goldens changed.

## Lint and CI gates

`pkg/planetgen` and `cmd/planet-explorer` (added Phase 1) must
remain Wasm-compatible. Two CI gates enforce this:

1. **`golangci-lint` with `depguard`** denies `import "C"` (cgo)
   and any blacklisted native-binding deps (see `.golangci.yml`).
   Catches imports at lint time.
2. **`GOOS=js GOARCH=wasm go build ./pkg/planetgen/...`**
   in CI catches non-Wasm-compatible runtime behaviour beyond what
   `depguard` sees.

Both gates run on every PR (see `.github/workflows/build-test.yml`).

## Slider tool

The web-based parameter explorer at `cmd/planet-explorer/` (Phase 1)
will be the canonical workflow for tuning `PlanetProfile` parameters
interactively. Compiles `pkg/planetgen` to Wasm and renders live in
a browser canvas.
