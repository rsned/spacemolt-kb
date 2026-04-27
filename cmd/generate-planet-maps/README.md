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

All flags:

| Flag | Default | Meaning |
|---|---|---|
| `-type` | "" | single planet type (must be paired with `-seed`) |
| `-seed` | "" | planet name (single mode only) |
| `-out` | "" | equirect output path (single mode); defaults to `<type>_<seed>.png` |
| `-db` | `../spacemolt-knowledge.db` | knowledge database path |
| `-outdir` | `kb/images/planets` | batch output directory |
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

## Phase 1 (current)

The render pipeline gained five Tier-S algorithms:

- **OkLab biome blending** (`pkg/planetgen/color/oklab.go`) — `BlendOkLab` /
  `SampleGradientOkLab` replace the RGB-space lerp in both renderers.
  Produces cleaner color transitions, especially across saturated palette
  stops. The legacy `Blend` / `SampleGradient` remain for explicit RGB ops
  (e.g., `Lerp`, `Brighten`).
- **Multi-noise control fields with monotone cubic splines**
  (`pkg/planetgen/field/control.go`, `pkg/planetgen/color/spline.go`).
  Five 3D-fBm fields (Continentalness, Erosion, PeaksValleys, Temperature,
  Humidity), each seeded by `seed.Domain(master, "control.<name>")` so
  adding a new field never shifts existing field outputs. Each field
  passes through a Fritsch-Carlson monotone-cubic spline; the heightmap
  is the sum of the five spline outputs. `RenderRocky` falls back to the
  legacy single-fBm path when `ControlConfig` and `Splines` are unset.
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
