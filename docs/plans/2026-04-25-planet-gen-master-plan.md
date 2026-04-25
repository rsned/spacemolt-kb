# Planet Generation Master Plan

**Date:** 2026-04-25
**Tool:** `cmd/generate-planet-maps` and supporting `pkg/planetgen`
**Source ideas:** `docs/compass_artifact_wf-5839e7e9-3eea-43f4-a351-b47c001ca782_text_markdown.md` (primary), `docs/Enhancing Procedural Planet Generation.md` (complementary), `docs/spacemolt-planet-techniques.html` (JS reference impl).

## 1. Goals and scope

**Goal.** Move `cmd/generate-planet-maps` from the v1 simple-noise pipeline to a designed-feeling, parameter-rich, sphere-native generator that implements all 25 items in the prioritized list of `compass_artifact_*.md`, plus cube-map storage and an interactive web parameter explorer. End state: every planet type rendered with multi-field control, OkLab biome blending, domain warping, plate-aware mountains, JFA-derived continentality, civilization signs on terran/super-terran, curl-noise gas giants, optional clouds, per-archetype color LUTs — stored as 4×3 cube-map crosses with a 2000×1000 equirect thumbnail baked as a post-pass, and per-planet parameter JSONs committed to the repo.

**Scope.** All 25 items in the prioritized list, the cube-map storage migration, the slider tool, and the per-planet JSON profile system. Six phases (0–5), each independently shippable.

**Non-goals.**
- No real-time rendering; slider tool re-renders on slider release at reduced face size, not per-frame.
- No GPU shader path. Pure Go on CPU. Wasm-compatible.
- No cgo dependencies anywhere in `pkg/planetgen` (slider tool runs as Wasm).
- No new planet *types* added; existing 13 stay, but each gains richer parameters.
- No backwards-compat with current PNGs; outputs change freely until each phase locks in.
- No refactor of `pkg/planetgen`'s consumers in this round (kb image references are by filename, format-agnostic).

**Defaults.**
- Cube-map face size **S = 1024**; cross is 4096×3072.
- Equirect thumbnail bake stays at **2000×1000** (matches current consumers).
- Slider tool at `cmd/planet-explorer/`.

## 2. Phase overview and sequencing

Five algorithm phases plus a profile-storage closer. Each phase is independently shippable: at the end of every phase, `cmd/generate-planet-maps` produces a complete set of valid planet PNGs.

| Phase | Theme | Items | Rough effort |
|---|---|---|---|
| 0 | Cube-map storage refactor (no visual change) | infra | 1–2 weeks |
| 1 | Tier S algorithms + slider tool | items 1–5 + tooling | 3–4 weeks |
| 2 | Tier A algorithms | items 6–13 | 4–6 weeks |
| 3 | Tier B algorithms | items 14–20 | 4–6 weeks |
| 4 | Tier C polish | items 21–25 | 2–3 weeks |
| 5 | Per-planet profile JSON storage | profile system | 1 week |

Estimates are speculative; phases ship when they ship.

**Inter-phase dependencies.** Phase 0 unblocks all. Phase 1's multi-noise control fields are an input to most Phase 2/3 items (ridged-mountain mask, rain shadow, civilization). JFA from Phase 2 is a primitive reused by Phase 3 civilization, rain shadow, and clouds. Plates from Phase 3 retroactively replace the Phase 2 placeholder continentality used by item 6.

## 3. Foundational decisions

These are locked design choices that constrain every later phase.

### 3.1 Storage migration first (Phase 0)
Cube-map refactor is a no-visual-change phase that lands before any algorithmic work. Every later item is written once on the new substrate. The refactor PR is small (~80 LOC of structural code), reviewable as image-diff-comparable, and trivially revertable.

### 3.2 Output stability
Algorithmic changes are allowed to alter every planet's appearance. Each merge re-renders all planets in batch. The only invariant preserved is **seed → planet identity** (same planet name keeps looking like a desert / oceanic / jovian, etc.). Once all phases land, the output is fully deterministic per seed: re-running with the same name produces the same image.

### 3.3 Per-subsystem named seeds
Every subsystem (warp, plates, biome T, biome M, craters, clouds, civ, ...) derives its own sub-seed from the master planet seed via a named-domain mix:
```go
subSeed := masterSeed ^ fnv64a("domain_warp")
```
Adding a new subsystem never shifts existing ones. This is enforced by code review, not lint.

### 3.4 Slider tool: web (Wasm) only
`pkg/planetgen` compiles to Wasm. UI is plain HTML + minimal JS (no framework). The Go core is the single source of truth; both batch generator and slider tool import it. No native binary in this round. Implication: **no cgo anywhere in `pkg/planetgen`**.

### 3.5 Plan structure
Master plan today (this doc) covers all phases. Detailed implementation plan written at the start of each phase under `docs/plans/YYYY-MM-DD-planet-gen-phase-N-<topic>.md`. Phases 0 and 1 get high detail in this master plan; later phases get paragraph-per-item sketches that earn full plans when started.

### 3.6 Package decomposition
`pkg/planetgen` splits into subpackages by *concern*, not by renderer:

```
pkg/planetgen/
├── (root: planetgen.go, profile.go — Generate, PlanetProfile, GetProfile)
├── cubemap/   (CubeMap, CubeMapF, sample, cross, bake)
├── noise/     (opensimplex, fbm, warp, ridged, curl, worley, jitter)
├── color/     (ColorStop, palette, oklab, lut, spline)
├── field/     (jfa, voronoi, plates, flow, control fields, province, continents, erosion)
├── biome/     (whittaker, climate, rainshadow)
├── feature/   (craters, civ, dunes, clouds, polish, ice, lava, ripples)
├── render/    (rocky, gasgiant — orchestration)
└── stats/     (pipeline timing, per-stage cost)
```

### 3.7 Testing strategy
Hybrid:
- **Math primitive unit tests** in each subpackage (deterministic, cheap).
- **Statistical invariants** in CI as hard gate (no NaN/Inf, alpha=255, ocean/land ratios in expected range, histogram non-degenerate, cross-seam continuity).
- **Golden cube-map crosses** for a curated 13-planet set (one per type) as soft gate. PR diffs surfaced via `cmd/tools/planet-image-diff` (side-by-side PNG + ΔE2000 mean). Reviewer regenerates with `go test -update`.
- **Slider tool** is the development feedback loop, not a test.

### 3.8 Lint and CI gates
- `.golangci.yml` enables `depguard` with deny list `["C", <known-cgo-pkgs>...]` scoped to `pkg/planetgen/...` and `cmd/planet-explorer/...`. Catches `import "C"` and any blacklisted native-binding deps at lint time.
- CI matrix adds `GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/...`. Catches non-Wasm-compatible behavior beyond what depguard sees.
- Both gates documented in `cmd/generate-planet-maps/README.md`.
- `golangci-lint` clean is a hard merge gate (per `CLAUDE.md`).

## 4. Phase 0 — cube-map storage refactor (high detail)

No visual change. Ports the existing equirect generator to a cube-map intermediate, sets up the package skeleton, lands testing infrastructure, lands lint and Wasm-build CI gates.

### 4.1 Cube-map data structures (`pkg/planetgen/cubemap/`)

```go
// Face index: 0=+X, 1=-X, 2=+Y, 3=-Y, 4=+Z, 5=-Z (matches GL_TEXTURE_CUBE_MAP_POSITIVE_X order)
type Face int

type CubeMap struct {
    Size  int
    Faces [6][]color.RGBA // row-major, len = Size*Size each
}

type CubeMapF struct {
    Size  int
    Faces [6][]float64
}
```

### 4.2 Direction primitives (`pkg/planetgen/cubemap/sample.go`)

```go
func DirToFaceUV(x, y, z float64) (face Face, u, v float64)
func FaceUVToDir(face Face, u, v float64) (x, y, z float64)
func FacePixelToDir(face Face, px, py, S int) (x, y, z float64)
func (cm *CubeMap)  Sample(x, y, z float64) color.RGBA
func (cmf *CubeMapF) Sample(x, y, z float64) float64
```

Convention matches GL's so the same PNG can be uploaded as a WebGL `TEXTURE_CUBE_MAP` in the slider tool with no remapping. Bilinear sample at seams pulls from the adjacent face.

### 4.3 4×3 cross I/O (`pkg/planetgen/cubemap/cross.go`)

Layout:
```
[ . ][+Y ][ . ][ . ]
[-X ][+Z ][+X ][-Z ]
[ . ][-Y ][ . ][ . ]
```
Empty cells are transparent (PNG). For S=1024, image is 4096×3072. Functions: `WriteCrossPNG(cm, path)`, `ReadCrossPNG(path) (*CubeMap, error)`.

### 4.4 Equirect bake (`pkg/planetgen/cubemap/bake.go`)

```go
func BakeEquirect(cm *CubeMap, width, height int) *image.RGBA
```
For each equirect pixel, `lat,lon → dir → DirToFaceUV → bilinear sample`. Default 2000×1000 thumbnail.

### 4.5 File migration

| Current file | Phase 0 destination | Change |
|---|---|---|
| `planetgen.go` | `pkg/planetgen/planetgen.go` | `Generate` returns `*cubemap.CubeMap`; new `GenerateEquirect` wrapper bakes 2000×1000 |
| `profile.go` | `pkg/planetgen/profile.go` | unchanged |
| `noise.go` | `pkg/planetgen/noise/noise.go` | `SphericalCoords` removed; replaced by `cubemap.FacePixelToDir` |
| `color.go` | `pkg/planetgen/color/color.go` | unchanged |
| `crater.go` | `pkg/planetgen/feature/crater.go` | unchanged math; loop iterates cube-map faces |
| `rocky.go` | `pkg/planetgen/render/rocky.go` | outer loop iterates faces |
| `gasgiant.go` | `pkg/planetgen/render/gasgiant.go` | same; lat extracted from `dir.y` |
| `stats.go` | `pkg/planetgen/stats/stats.go` | unchanged |

Caller change: `cmd/generate-planet-maps/main.go` writes both `<name>.cube.png` (cross) and `<name>.png` (equirect bake) for every planet. Both kept on disk.

### 4.6 Tests

Unit:
- `TestDirFaceUVRoundtrip`: 10 000 random unit dirs, distance < 1e-12.
- `TestFacePixelToDirCoverage`: every pixel maps to a unit-length direction; max gap < 2× pixel solid angle.
- `TestCrossRoundtrip`: random `CubeMap` → write → read → byte-identical.
- `TestSeamSample`: at every face edge, sampling exactly on the seam from either side returns the same RGBA within 1 LSB.
- `TestBakeIdentity`: face-index-painted cube-map bakes to the right lat/lon zones.

Statistical: per planet type, 100 random seeds → no NaN/Inf, alpha=255 for rocky outputs, ocean/land for `terran` in [0.55, 0.75], ≥4 distinct histogram buckets, lon-seam delta < 8/255.

Goldens: 13-planet curated set under `cmd/generate-planet-maps/testdata/golden/`. Diff CLI at `cmd/tools/planet-image-diff` produces side-by-side PNG + ΔE2000 mean.

### 4.7 Documentation

`cmd/generate-planet-maps/README.md` (lands in Phase 0, updated each phase). Sections:
1. What it does
2. Inputs (DB schema, seed derivation)
3. Outputs (cube-map cross, equirect thumbnail, face-arrangement ASCII diagram, file naming)
4. Running the tool (every flag, single mode, batch mode)
5. Architecture (subpackage map with one-line role of each)
6. Testing and golden-diff workflow:
   - what statistical invariants are auto-checked
   - where the curated 13-planet golden set lives
   - how to run diffs locally: `go test ./cmd/generate-planet-maps/... -run TestGolden`
   - what a reviewer sees (side-by-side diff PNG + ΔE2000)
   - how to manually validate a diff (open the diff PNG, confirm changes are intended, check ΔE2000 in expected ballpark)
   - how to regenerate goldens: `go test ./cmd/generate-planet-maps/... -run TestGolden -update`
   - rule: never run `-update` without a PR description explaining why
7. Slider tool pointer (Phase 1)
8. Lint and Wasm-build CI gates explained

Each subpackage gets a `doc.go` with a 1–2 paragraph role description.

### 4.8 Acceptance

1. All unit tests pass; `golangci-lint` clean.
2. Statistical invariants pass on the curated 13-planet set.
3. Golden equirect images for the 13 planets have **mean ΔE2000 < 1.5** vs Phase-0 baseline (small seam-handling deltas allowed).
4. `go build ./...` and `go test ./...` green.
5. Wasm build job green for `pkg/planetgen/...`.
6. Batch render of 700+ planets in ≤ 1.5× current wall time at 8 workers.
7. Both `<name>.cube.png` and `<name>.png` produced for every planet.

## 5. Phase 1 — Tier S algorithms + slider tool (high detail)

Five algorithm items + the web parameter explorer. Each algorithm lands in its subpackage with unit tests, then is wired into the rocky/gasgiant pipelines. Slider tool grows alongside so each item is tunable the day it lands.

### 5.1 OkLab biome blending (`pkg/planetgen/color/oklab.go`)

Add `lucasb-eyer/go-colorful`. Replace `blendColor` (RGB lerp) with `BlendOkLab(a, b color.RGBA, t float64)`. Add `SampleGradientOkLab(stops, h)`. Migrate every blend call site in `render/rocky.go` and `render/gasgiant.go`. ~50 LOC.

### 5.2 Multi-noise control fields with monotone cubic splines

Five 3D fBm fields per planet — `Continentalness`, `Erosion`, `PeaksValleys`, `Temperature`, `Humidity` — each with its own named-domain seed. Profile gains `ControlFields ControlConfig` (per-field amp/freq/octaves) and `Splines [5]Spline` (Fritsch-Carlson monotone cubic in `pkg/planetgen/color/spline.go`). Final height = sum of spline outputs (not multiplied). 13 existing planet types get hand-tuned defaults inferred from current behavior. ~350 LOC.

### 5.3 Domain warping (`pkg/planetgen/noise/warp.go`)

Single Quilez warp pass: `q = p + amp · vec3(noise(p+a), noise(p+b), noise(p+c))`. Three new sub-seeded generators per planet. Applied to *every* sample in heightmap, biome, cloud, civ lookups. Profile: `WarpAmp`, `WarpFreq`, `WarpOctaves`. ~70 LOC.

### 5.4 Whittaker T+M biome lookup (`pkg/planetgen/biome/whittaker.go`)

Drives biome from temperature + moisture instead of latitude. T = `cos(lat)·tempField + lapseRate·elevation`. M = `humidityField` (Phase 2 JFA continentality and Phase 3 rain shadow modulate this later). Lookup table: 2D grid of biome IDs with per-cell palette stops; bilinear color interpolation between neighbor cells in OkLab. Per-pixel jitter (±3 channel units) from a high-frequency noise. Profile: `BiomeTable`. Replaces latitude-band logic in `render/rocky.go`. ~250 LOC.

### 5.5 Per-archetype color-correction LUT (`pkg/planetgen/color/lut.go`)

16³ or 32³ 3D color cube applied as final grade. Stored as `[]color.RGBA` with trilinear sample. Per-archetype LUT files at `pkg/planetgen/color/luts/<archetype>.cube` (Resolve `.cube` format). Profile: `LUT *LUT`. ~80 LOC + LUT assets.

### 5.6 Slider tool (`cmd/planet-explorer/`)

```
cmd/planet-explorer/
├── main.go                  (Go HTTP server for dev)
├── wasm/
│   └── main.go              (Wasm entry: Generate, BakeEquirect, DefaultProfile via syscall/js)
└── web/
    ├── index.html           (slider UI, canvas viewport)
    ├── app.js
    ├── style.css
    └── wasm_exec.js
```

Wasm exports: `Generate(profileJSON, seedStr, faceSize) → cube-map PNG bytes`; `BakeEquirect(cubePNG, w, h) → equirect PNG`; `DefaultProfile(planetType) → JSON`.

Profile JSON: `PlanetProfile` round-trips via `encoding/json`. Slider tool reads/writes the same JSON the Go code uses.

UI layout: left panel collapsible groups by subpackage (Noise / Domain Warp / Control Fields / Biome / Color / Craters or Bands). Right panel: cube-map preview (cross), equirect bake below, regenerate button, planet-type dropdown, seed input, face-size dropdown (256 / 512 / 1024). Re-render on slider release.

Performance target: face size 256 + full Tier-S pipeline finishes < 1 s on a laptop. 1024 (production) ~16 s for "render full quality."

### 5.7 Acceptance

1. All five Tier-S items land with unit tests + statistical invariants passing.
2. Goldens regenerated; reviewer-signed PR.
3. Slider tool serves at `localhost:8080` via `go run ./cmd/planet-explorer`; renders all 13 planet types; all profile parameters tunable.
4. Tuned-default `PlanetProfile`s for all 13 types committed back to `profile.go`.
5. `cmd/planet-explorer/README.md` covering build (`GOOS=js GOARCH=wasm`), dev server, exporting tuned values back to Go.

## 6. Phase 2 sketches (Tier A)

Detailed plan written at start of Phase 2.

**6. Ridged multifractal mountains masked by continentality (`pkg/planetgen/noise/ridged.go` + `render/`).** `RidgedMulti(p, octaves, gain, offset)` returning `(offset − |noise|)²`. Mask by `smoothstep(0.5, 0.7, distToPlateBoundary)` from item 14 (or by item-2 `Continentalness` until plates land). Anisotropic stretch along gradient of a low-freq belt-direction field so ranges form belts. Profile: `RidgedAmp`, `RidgedFreq`, `RidgedOctaves`, `MountainBeltAniso`. ~120 LOC.

**7. Province / roughness modulation map (`pkg/planetgen/field/province.go`).** Warped Voronoi 8–40 cells via Fibonacci spiral; cell membership becomes integer field. Three low-freq scalars modulate per-cell `R_amp`, `R_freq`, `R_type`. Drives Mars-style dichotomy and regional roughness variety. Profile: `ProvinceCount`, `ProvinceJitter`, `ProvinceWarpAmp`. ~200 LOC.

**8. JFA distance-to-coast (`pkg/planetgen/field/jfa.go`).** Jump Flooding Algorithm on cube-map with cross-face propagation, ~11 passes. Output `CubeMapF` distance in km using planet-radius profile param. Reused by items 9, 16, 18, 22. ~150 LOC.

**9. Particle hydraulic erosion (`pkg/planetgen/field/erosion.go` via `setanarut/rainfall`).** 70k–200k droplets; cube-map seam crossings need angular-velocity continuation. Run after item 6, before color/biome. Profile: `ErosionDroplets`, `ErosionInertia`, `ErosionCapacity`. ~230 LOC.

**10. Coastal-noise enhancement + Voronoi continents (`pkg/planetgen/noise/coastal.go` + `field/continents.go`).** Coastal: `e_coast = e + α·(1 − e⁴)·(n4 + n5/2 + n6/4)`. Continents: 10–50 Fibonacci-spiral seeds with warped-Voronoi base height. Profile: `CoastalAmp`, `ContinentSeeds`, `ContinentWarpAmp`. ~180 LOC.

**11. Crater system rebuild (`pkg/planetgen/feature/crater.go`).** Truncated power-law SFD; density mask × `MariaDensityFactor`; per-crater age (beta-distributed, biased by `SurfaceAge`); ejecta rays as albedo (angular sin × radial falloff × age²); secondaries; catena; multi-ring basins; diameter-branched floor types. ~250 LOC.

**12. Curl-noise turbulent advection for gas giants (`pkg/planetgen/noise/curl.go` + `render/gasgiant.go`).** Backward-trace 4–16 semi-Lagrangian iterations: `p_traced = p − dt·(zonal_jet(lat) + curlNoise(p_traced)·amp)`. Profile: `CurlAmp`, `CurlIterations`, `ZonalJetProfile`. ~120 LOC.

**13. Cassini Jupiter ramp + storm latitude bands (`pkg/planetgen/color/jupiter_ramp.png` + `profile.go`).** 1D color slice asset. Profile gains `[]StormBand` so jovian/ice-giant get hand-authored placements. ~50 LOC + asset.

## 7. Phase 3 sketches (Tier B)

**14. Voronoi tectonic plates (`pkg/planetgen/field/plates.go`).** Random flood-fill (not BFS) for jagged boundaries; per-plate rotation axis + angular speed; oceanic/continental flag. Three boundary distance fields by type (convergent / divergent / transform). Critical: `Δdistance·n < −0.75` threshold. ~400 LOC.

**15. D8 flow accumulation + rivers (`pkg/planetgen/field/flow.go`).** Planchon-Darboux fill, D8 downhill, accumulation; rivers above threshold; Gaussian moisture boost feeds back into Whittaker M. Cube-map seam-aware D8 neighbors. ~250 LOC.

**16. Wind-driven rain shadow (`pkg/planetgen/biome/rainshadow.go`).** Latitude-dependent prevailing wind; advect moisture parcels along great-circle wind tangents on the sphere; deposit on upwind mountain side; lee `M *= 0.15`. ~150 LOC.

**17. Cloud-layer overlay (`pkg/planetgen/feature/clouds.go`).** Separate alpha cube-map. Latitude-banded coverage × domain-warped fBm × Rankine-vortex storms. Fake self-shadowing via density-gradient·sun-direction. Output `<name>.clouds.cube.png` for terran/super-terran/oceanic. ~350 LOC.

**18. Civilization signs (`pkg/planetgen/feature/civ.go`).** Habitability scoring + Bridson Poisson-disc on sphere + Zipfian populations + nightside Black Marble splats + dayside city patches + agriculture grids + Delaunay+MST roads with A* terrain cost. Single `civTier ∈ [0,1]` profile param gates everything. Outputs include `<name>.night.cube.png`. ~600 LOC.

**19. Edge-blending QA.** Phase 0 already moved generation to cube-sphere. This slot becomes a quality pass: explicit seam-continuity tests for every Tier-A/B field that crosses face boundaries. ~80 LOC of test infrastructure.

**20. Voronoi cell-coordinate jitter (`pkg/planetgen/noise/jitter.go`).** Per-cell rotation/offset of detail-noise lookup coords to break visible texture repetition. ~40 LOC.

## 8. Phase 4 sketches (Tier C polish)

**21. Surface polish bundle (`pkg/planetgen/feature/polish.go`).** Wind streaks behind craters, frost-vs-snow lines, polygonal-cracked playas, height-independent albedo blotches, rim discoloration, sediment fill. Each gated by per-type profile flag. ~250 LOC total.

**22. Anisotropic dune fields (`pkg/planetgen/feature/dunes.go`).** Curl-of-fBm wind tangent field; anisotropic noise sampled in stretched frame produces transverse dunes. `DuneTypeMix` blends transverse / barchan / longitudinal / star. Applied where Whittaker = desert AND rain shadow = arid. ~200 LOC.

**23. Chaos terrain + linea + crevasses (`pkg/planetgen/feature/ice.go`).** Domain-warped Voronoi cells with random per-cell tilt; great-circle linea arcs; high-freq anisotropic crevasse ridges aligned to glacial flow gradient. ~250 LOC.

**24. Lava lobes + emission channel (`pkg/planetgen/feature/lava.go`).** Poisson-disc volcano centers; MrLavaLoba probabilistic flow paths; output `<name>.emission.cube.png` single-channel cube-map. Pyroclastic darkening + crusted/molten Voronoi pattern. ~300 LOC.

**25. Reaction-diffusion ripples (`pkg/planetgen/feature/ripples.go` + `cmd/tools/gen-ripples`).** Gray-Scott offline once, low-res, wind-direction forcing → tileable ripple texture committed as asset. Sampled with item-22 wind-tangent rotation per dune cell. ~180 LOC.

## 9. Phase 5 — per-planet profile JSON

Every planet gets a self-contained JSON profile committed to the repo. Generator reads it if present; otherwise falls back to per-type defaults. Slider tool can open, edit, and save back via the dev server.

### 9.1 Storage

Files at `kb/data/planet-profiles/<system>_<planet>.json` (lower-cased, sanitized — same convention as PNG filenames). One file per planet. Self-contained; `git diff` on one file shows exactly what changed for that planet.

### 9.2 Schema

```go
type PlanetProfileV1 struct {
    SchemaVersion string `json:"schemaVersion"` // "1"
    Type          string `json:"type"`
    Seed          string `json:"seed"`          // canonical seed source (planet name)
    Renderer      string `json:"renderer"`
    HandTuned     bool   `json:"handTuned"`
    // ...all current PlanetProfile fields embedded inline...
    // ...all Phase 1+ additions (warp, splines, biome table, LUT name, plates, ...)...
}
```

`Migrate(jsonBytes []byte) ([]byte, error)` walks old schema versions forward.

### 9.3 Generator behavior

`Generate(planetType, planetName)` checks for `kb/data/planet-profiles/<sys>_<planet>.json`. If present → load + use. If absent → derive from `Profiles[planetType]` defaults. Determinism preserved: seed derived from `Seed` field.

### 9.4 Initial population

`cmd/tools/seed-planet-profiles` walks the kb DB, writes one JSON per planet using current per-type defaults. Commit the lot. From then on, hand-tuning a planet = editing one JSON file (manually or via slider).

### 9.5 Slider tool integration

- **Planet picker dropdown** populated from `kb/data/planet-profiles/*.json` (loaded via dev server `GET /profiles`).
- **Open** loads JSON into slider state.
- **Save** POSTs to dev server `PUT /profiles/<sys>_<planet>.json`; server writes the file. Production/static hosting disables save (read-only).
- **Save as new** prompts for `<sys>_<planet>` and creates a new file.
- **Diff against type default** shows which parameters differ from `Profiles[type]`.

### 9.6 Acceptance

1. `seed-planet-profiles` produces ~700 JSON files; baking from JSON vs in-code default produces byte-identical PNG.
2. Slider tool round-trip: open → tweak → save → re-open → state matches.
3. Generator dispatch correctly prefers JSON over in-code default.
4. Migration test: a fixture v1 file loads via `Migrate` and renders identically.
5. CI test: for any planet whose JSON has `"handTuned": false`, regenerating from per-type defaults produces JSON byte-equal to committed (catches silent default drift).
6. README updated documenting the JSON workflow.

## 10. Cross-cutting

### 10.1 New Go dependencies (all pure Go, Wasm-compatible)

- `github.com/lucasb-eyer/go-colorful` — Phase 1 (OkLab, ΔE2000)
- `github.com/setanarut/rainfall` — Phase 2 (particle erosion)
- `github.com/fogleman/delaunay` — Phase 3 (civ road graph)
- `github.com/fogleman/poissondisc` — Phases 2/3 (continents, civ, storms)
- `github.com/kelindar/noise` — optional Phase 1 (faster simplex)

### 10.2 Risks

- **Wasm bundle size.** Tier S adds 5 noise generators + go-colorful; Tier B adds plates + civ. *Mitigation*: `-ldflags="-s -w"` + `wasm-opt`; lazy-load rare modules if needed.
- **Cube-map seam quality.** Cross-face filtering at edges is the most error-prone primitive. *Mitigation*: explicit unit tests at every edge in Phase 0; statistical seam-continuity invariants run as CI gate.
- **Erosion runtime on cube-map.** 200k droplets × 6 faces is heavy. *Mitigation*: parallelize per-droplet across goroutines; benchmark in Phase 2; drop to 70k droplets default if needed.
- **Profile JSON drift vs in-code defaults.** JSON could silently diverge from `Profiles` map. *Mitigation*: CI test in 10.5 — for `handTuned: false` planets, regenerated JSON must match committed.
- **Time estimates.** All phase estimates speculative; phases ship when they ship.

### 10.3 Final repo layout

```
cmd/
├── generate-planet-maps/             (batch generator, README, golden testdata)
├── planet-explorer/                  (slider tool: Go server + Wasm + web/)
└── tools/
    ├── seed-planet-profiles/         (one-time JSON populator)
    ├── planet-image-diff/            (golden diff CLI)
    └── gen-ripples/                  (Phase 4 reaction-diffusion bake)

pkg/planetgen/
├── planetgen.go                      (Generate, Migrate, profile load)
├── profile.go                        (PlanetProfileV1)
├── cubemap/                          (Phase 0)
├── noise/                            (Phase 0; warp/ridged/curl/jitter added P1–P3)
├── color/                            (Phase 0; oklab/lut/spline added P1)
├── field/                            (Phases 2–3: jfa, voronoi, plates, flow, control, province, continents, erosion)
├── biome/                            (Phase 1: whittaker; Phase 3: rainshadow)
├── feature/                          (Phase 0 craters; P3 clouds/civ; P4 polish/dunes/ice/lava/ripples)
├── render/                           (rocky, gasgiant — orchestration)
└── stats/

kb/data/planet-profiles/              (Phase 5: ~700 JSON files, one per planet)

docs/plans/
├── 2026-04-25-planet-gen-master-plan.md   (this document)
└── 2026-XX-XX-planet-gen-phase-N-*.md     (per-phase implementation plans)
```

### 10.4 Final acceptance

Phase 5 closes the loop: every planet has a tunable, git-trackable, slider-editable profile; every algorithm in the prioritized list has been implemented in its assigned subpackage; statistical invariants + golden diffs gate every PR; the slider tool is the canonical tuning workflow; lint and Wasm-build CI gates enforce the no-cgo invariant.
