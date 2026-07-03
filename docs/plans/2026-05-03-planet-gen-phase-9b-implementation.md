# Planet Generation Phase 9b Implementation Plan: Civilization Signs

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add civilization-sign overlays — city patches, agriculture grids, Black Marble nightside lights, MST road networks. All gated by a single `civTier ∈ [0,1]` profile knob; outputs include the existing color cube-map (modified with daytime city patches + agriculture) and a new `<name>.night.cube.png`.

**Date:** 2026-05-03
**Branch:** `phase-0/cube-map` (worktree `/home/robert/spacemolt/kb-phase-0-cube-map`)
**Predecessors:** Phase 9a (clouds). Phase 8 (Tier B geology) is required for habitability scoring (uses plates, rivers, rain shadow as inputs).
**Master plan reference:** `docs/plans/2026-04-25-planet-gen-master-plan.md` §7, item 18.

**Architecture:** Civilization is a layered process driven by a habitability scalar field, then site placement (Bridson Poisson on the sphere, weighted by habitability), then population assignment (Zipfian over sites), then per-site rendering passes (city patches, agriculture, lights, roads). All enabled only when `profile.Civ.Tier > 0`. Outputs are the modified color cube-map (with daytime overlays) plus a new nightside cube-map (Black Marble lights).

**Tech Stack:** Go 1.24+, `pkg/planetgen/feature/civ.go`, two new pure-Go deps (`github.com/fogleman/poissondisc`, `github.com/fogleman/delaunay`), wasm via planet-explorer civ debug stages, golangci-lint hard gate, golden-image regression test, ΔE2000 visual-diff threshold.

**Phase 9 split:**

| Sub-phase | Items | Status |
|---|---|---|
| Phase 9a (clouds) | 17 | follow Phase 8 |
| **Phase 9b (this plan — civ)** | 18 | this doc |

---

## File Structure

**Created:**
- `pkg/planetgen/feature/civ.go` — civilization-sign generation (~600 LOC)
- `pkg/planetgen/feature/civ_test.go`
- `pkg/planetgen/feature/habitability.go` — habitability scalar field (~150 LOC)
- `pkg/planetgen/feature/habitability_test.go`
- `pkg/planetgen/feature/poisson_sphere.go` — Bridson Poisson-disc on the sphere (~100 LOC, may use external dep instead)
- `pkg/planetgen/feature/poisson_sphere_test.go`
- `pkg/planetgen/feature/roads.go` — Delaunay+MST+A* road generation (~200 LOC)
- `pkg/planetgen/feature/roads_test.go`
- `cmd/generate-planet-maps/testdata/golden/<name>.night.cube.png` — 3-5 new golden PNGs

**Modified:**
- `pkg/planetgen/types/types.go` — `CivConfig` with `Tier float64` and tunables
- `pkg/planetgen/profile.go` — per-archetype `Civ:` defaults (0 for most; 0.5 for terran goldens)
- `pkg/planetgen/render/rocky.go` — invoke civilization stages between Craters and Colorize; expose `RenderNightCubeMap`
- `pkg/planetgen/render/colorize.go` (or wherever the day-color blend happens) — apply city patches + agriculture overlays
- `pkg/planetgen/render/debug.go` — register civ debug stages (habitability, sites, populations, roads, lights)
- `cmd/generate-planet-maps/main.go` — write `<name>.night.cube.png` for civ-enabled archetypes
- `cmd/generate-planet-maps/golden_test.go` — extend to compare night goldens
- `cmd/generate-planet-maps/invariants_test.go` — `TestPhase9bCivInvariants`
- `go.mod` / `go.sum` — add new dependencies

---

## Task 1: Habitability scalar field

**Files:**
- Create: `pkg/planetgen/feature/habitability.go`
- Create: `pkg/planetgen/feature/habitability_test.go`

**Algorithm.** Per pixel, compute a habitability score in `[0, 1]`:

```
hab(p) =     w_h * smoothstep(0.30, 0.55, height)            # not too low (ocean), not too high (peaks)
           - w_l * smoothstep(0.55, 0.85, height)            # subtract for high elevation
           + w_t * gaussian(temp, μ=0.55, σ=0.15)            # temperate zones
           + w_m * gaussian(humid * rainshadow, μ=0.6, σ=0.20) # not too dry, not too wet
           + w_r * smoothstep(0, 0.05, riverDistRecip)       # bonus near rivers
           - w_p * (plates.Convergent < 100)                 # subtract along convergent boundaries (volcanism)
```

Sum, clamp to [0, 1]. Inputs are read from existing pipeline state: heightmap (after rivers carved), tField/mField (climate fields), `plates.Convergent`, `flow.Rivers`, `rainShadow.Multiplier`.

- [ ] **Step 1: Define `HabitabilityField` and writer**

```go
type HabitabilityField struct {
    Size  int
    Score [cubemap.NumFaces][]float64
}

// GenerateHabitability composes the habitability scalar field from
// the rocky-pipeline inputs.
func GenerateHabitability(
    heightmap *cubemap.CubeMapF,
    tField, mField *cubemap.CubeMapF,
    plates *field.PlateField,
    flow *field.FlowField,
    rainShadow *biome.RainShadowField,
    cfg types.CivConfig,
    S int,
) *HabitabilityField
```

Inputs `plates`, `flow`, `rainShadow` may be nil for archetypes without those features — habitability falls back to height + climate only.

- [ ] **Step 2: Tests**

```go
func TestHabitabilityZeroOnOcean(t *testing.T) // ocean pixels score < 0.1
func TestHabitabilityHighOnTemperateLowlands(t *testing.T) // hand-built fixtures
func TestHabitabilityDeterministic(t *testing.T)
func TestHabitabilitySeamContinuity(t *testing.T) // pure-direction; 5% threshold
```

- [ ] **Step 3: Lint, commit**

```
git add pkg/planetgen/feature/habitability.go pkg/planetgen/feature/habitability_test.go
git commit -m "P9b: habitability scalar field (item 18 part 1/4)"
```

---

## Task 2: Bridson Poisson-disc on the sphere + site placement

**Files:**
- Create: `pkg/planetgen/feature/poisson_sphere.go`
- Create: `pkg/planetgen/feature/poisson_sphere_test.go`

**Algorithm.** Bridson Poisson-disc adapted to the sphere: maintain an active list, for each active site sample candidates within a spherical-cap annulus `[r, 2r]` around it, accept those above a minimum angular distance from all existing sites. The minimum distance varies per pixel based on habitability — high-habitability cells get tighter packing (more cities), low-habitability cells get sparser placement.

The simplest implementation: pick an initial random unit-direction seed in the highest-habitability cell. Active list grows; after termination, total sites is roughly `4π / (πr²) = 4/r²` for uniform `r`. With habitability-modulated `r(p) = r_min + (1-hab(p)) * (r_max - r_min)`, expect more sites in habitable regions.

Existing dep `github.com/fogleman/poissondisc` is 2D. We need spherical. Either:
- (a) Implement directly in `poisson_sphere.go` (~80 LOC).
- (b) Project the sphere to a 2D parameterization (e.g. equal-area Lambert), use the 2D library, project sites back. Distortion at the poles.

Choose (a) — direct implementation is simpler than dealing with projection distortion.

- [ ] **Step 1: API**

```go
type Site struct {
    Dir        [3]float64 // unit
    Population float64    // [0, 1]; assigned in Task 3
    Habitability float64
}

// PoissonOnSphere returns Poisson-disc-distributed sites on the unit
// sphere with site density modulated by hf.Score.
func PoissonOnSphere(hf *HabitabilityField, cfg types.CivConfig, master int64) []Site
```

- [ ] **Step 2: Tests**

```go
func TestPoissonNoTwoSitesCloserThanMin(t *testing.T)
func TestPoissonDeterministic(t *testing.T)
func TestPoissonSitesClusterInHabitableRegions(t *testing.T) // mean habitability of all sites > 0.6
func TestPoissonNoSitesInOcean(t *testing.T) // score < 0.05 → no sites
func TestPoissonReturnsZeroSitesForCivTierZero(t *testing.T)
```

- [ ] **Step 3: Lint, commit**

```
git add pkg/planetgen/feature/poisson_sphere.go pkg/planetgen/feature/poisson_sphere_test.go
git commit -m "P9b: Bridson Poisson-disc on sphere + civ site placement (item 18 part 2/4)"
```

---

## Task 3: Population assignment (Zipfian)

**Files:**
- Modify: `pkg/planetgen/feature/civ.go` (new file; will fill out in Tasks 3-5)

**Algorithm.** Sort sites by habitability descending; assign `population = base * rank^(-zipfAlpha)` with `base = cfg.Tier`, `zipfAlpha = 1.07` (typical cities Zipf coefficient). This produces a few high-pop sites + many small ones.

- [ ] **Step 1: Population assignment as a stage**

```go
// AssignPopulations sorts sites by habitability and writes Zipfian
// populations to each. Mutates the slice in place.
func AssignPopulations(sites []Site, cfg types.CivConfig)
```

- [ ] **Step 2: Tests**

```go
func TestPopulationsZipfian(t *testing.T) // log-log slope of (rank, pop) ≈ -1.07
func TestPopulationsDeterministic(t *testing.T)
```

- [ ] **Step 3: Commit**

```
git add pkg/planetgen/feature/civ.go pkg/planetgen/feature/civ_test.go
git commit -m "P9b: Zipfian population assignment for civ sites (item 18 part 3a/4)"
```

---

## Task 4: Road network — Delaunay + MST + A*

**Files:**
- Create: `pkg/planetgen/feature/roads.go`
- Create: `pkg/planetgen/feature/roads_test.go`

**Algorithm.**
1. **Delaunay triangulation of sites** on the sphere. Project to a 2D space (e.g. stereographic from the antipode of the centroid), run `github.com/fogleman/delaunay`, project back. Preserve site ids.
2. **Minimum spanning tree** of the Delaunay edges weighted by `length × terrain_factor(midpoint)`. `terrain_factor = 1 + 5*max(0, height_at_midpoint - 0.7)` (mountains penalized).
3. **A* path** along the heightmap from each MST edge endpoint to endpoint, using 8-connected pixel grid with cost `1 + slope_penalty`. Project the resulting path back to a list of unit directions; rasterize as a 1-pixel-wide line with anti-aliased blend.

Roads written to a separate `[6][]float64` field (intensity per pixel) that the colorizer overlays at low alpha.

- [ ] **Step 1: Delaunay on sphere**

```go
// SphericalDelaunay returns the Delaunay triangulation of the sites
// projected via stereographic. Returns edges as (i, j) pairs.
func SphericalDelaunay(sites []Site) [][2]int
```

Use `github.com/fogleman/delaunay`. Add to `go.mod`:

```
go get github.com/fogleman/delaunay
```

- [ ] **Step 2: MST**

Standard Kruskal. Edge weight = great-circle distance × `1 + 5*max(0, midpoint_height - 0.7)`.

- [ ] **Step 3: A* path**

8-connected pixel grid. Cost per step = `1 + abs(height_neighbor - height_current) * slope_weight`. Endpoint conversions: site direction → cube-map pixel via `cubemap.DirToFacePixel`; reverse for path output.

A*'s closed/open sets handle face seams via `cubemap.FacePixelNeighbors8` (which Phase 8 added).

- [ ] **Step 4: Rasterize**

For each path, walk pixel-by-pixel and add to the road intensity field. Anti-aliasing: each path step contributes 1.0 to the center pixel and 0.3 to the 4-connected neighbors.

- [ ] **Step 5: Tests**

```go
func TestRoadsConnectAllSites(t *testing.T) // every pair of large sites is reachable via the road network
func TestRoadsAvoidPeaks(t *testing.T) // mean height along roads < mean height of map
func TestRoadsDeterministic(t *testing.T)
```

- [ ] **Step 6: Commit**

```
git add pkg/planetgen/feature/roads.go pkg/planetgen/feature/roads_test.go go.mod go.sum
git commit -m "P9b: road generation via Delaunay+MST+A* (item 18 part 3b/4)"
```

---

## Task 5: Render passes — city patches, agriculture, nightside lights

**Files:**
- Modify: `pkg/planetgen/feature/civ.go` — add `GenerateCiv` orchestrator + render fields
- Modify: `pkg/planetgen/render/rocky.go` — invoke civ before colorize; expose `RenderNightCubeMap`
- Modify: `pkg/planetgen/render/colorize.go` (or rocky.go) — apply daytime overlays

**Algorithm.**

- **Daytime city patches:** for each site, paint a circular patch of radius `r_city * sqrt(population)` (typical max 30 km) on the daytime color. Color = warm beige `(0.7, 0.6, 0.5)` blended at alpha `population`.
- **Agriculture grids:** for each site, paint regular rectangles in a band around the patch. Use a per-site rotation seed for variation. Green-yellow tone, alpha `0.5 * population`.
- **Nightside lights:** Black Marble splats — for each site, render to the night cube-map a Gaussian splat with intensity proportional to `population^1.5`. Sum over all sites.

```go
type CivField struct {
    Size       int
    Sites      []Site
    DayMask    [cubemap.NumFaces][]float64 // city + agriculture overlay alpha; per-pixel
    DayColor   *cubemap.CubeMap            // pre-rendered RGBA for the daytime overlay
    Roads      [cubemap.NumFaces][]float64 // road intensity from Task 4
    Night      *cubemap.CubeMap            // black-marble nightside RGBA
}

func GenerateCiv(
    heightmap *cubemap.CubeMapF,
    tField, mField *cubemap.CubeMapF,
    plates *field.PlateField,
    flow *field.FlowField,
    rainShadow *biome.RainShadowField,
    profile *types.PlanetProfile,
    master int64,
    S int,
) *CivField
```

Returns nil when `profile.Civ.Tier == 0`.

- [ ] **Step 1: City + agriculture patch rendering**

Add to `civ.go`. Patches written to `DayColor` and `DayMask`.

- [ ] **Step 2: Black Marble nightside**

Gaussian splat per site. Use `go-colorful` for warm tone (golden-orange).

- [ ] **Step 3: Roads woven in**

Roads from Task 4 paint into `DayColor` at low alpha (a darker beige line).

- [ ] **Step 4: Wire into rocky pipeline**

In `RenderRocky` (or wherever colorize lives), after habitability + civ generation:

```go
civ := feature.GenerateCiv(heightmap, tField, mField, plates, flow, rainShadow, profile, seed, S)
// In the colorize loop, for each pixel:
c := biome.LookupColor(...)
if civ != nil {
    overlayAlpha := civ.DayMask[face][py*S+px]
    if overlayAlpha > 0 {
        c = blend(c, civ.DayColor.Faces[face][py*S+px], overlayAlpha)
    }
    if r := civ.Roads[face][py*S+px]; r > 0 {
        c = blend(c, beigeRoadColor, r * 0.5)
    }
}
```

- [ ] **Step 5: Add `RenderNightCubeMap` entry point**

```go
// RenderNightCubeMap returns the Black Marble nightside as a separate
// cube-map. Returns nil when civ disabled.
func RenderNightCubeMap(profile *types.PlanetProfile, seed int64, S int) *cubemap.CubeMap
```

Internally calls `feature.GenerateCiv(...)` and returns `civ.Night`.

- [ ] **Step 6: Tests**

```go
func TestCivProducesDayOverlay(t *testing.T)
func TestCivProducesNightLights(t *testing.T)
func TestCivDisabledForCivTierZero(t *testing.T)
func TestRenderNightCubeMapTerranNonNil(t *testing.T)
```

- [ ] **Step 7: Commit**

```
git add pkg/planetgen/feature/civ.go pkg/planetgen/render/rocky.go pkg/planetgen/render/colorize.go pkg/planetgen/render/civ_render_test.go
git commit -m "P9b: civ render passes — patches + agriculture + nightside (item 18 part 4/4)"
```

---

## Task 6: Profile knobs + per-archetype defaults

**Files:**
- Modify: `pkg/planetgen/types/types.go` — `CivConfig`
- Modify: `pkg/planetgen/profile.go` — defaults

```go
type CivConfig struct {
    Tier             float64 `json:"tier,omitempty"`             // 0 disables
    SiteMinDistRad   float64 `json:"siteMinDistRad,omitempty"`   // typical π/100 ≈ 1.8°
    SiteMaxDistRad   float64 `json:"siteMaxDistRad,omitempty"`   // typical π/30 ≈ 6°
    MaxPopulation   float64 `json:"maxPopulation,omitempty"`     // [0,1] cap on largest city
    NightLightHue   float64 `json:"nightLightHue,omitempty"`     // OkLab hue [0, 360)
    AgricultureRatio float64 `json:"agricultureRatio,omitempty"` // [0, 1] fraction of patch area for ag
}
```

Defaults: `Tier: 0` everywhere except `terran: 0.5`, `super_terran: 0.3`. Other archetypes (oceanic, tundra, arid, glacial, scorched, lava_world, ice_world, hothouse, gas giants, unknown) have `Tier: 0`.

- [ ] **Step 1: Add config + defaults**

- [ ] **Step 2: JSON serialization round-trip test**

- [ ] **Step 3: Commit**

```
git add pkg/planetgen/types/types.go pkg/planetgen/profile.go
git commit -m "P9b: CivConfig profile knob + per-archetype defaults"
```

---

## Task 7: Civ debug stages

**Files:**
- Modify: `pkg/planetgen/render/debug.go`
- Modify: `pkg/planetgen/render/debug_palette.go`

Register five new debug stages (only when civ is non-nil):
- `Civ: habitability` — scalar field, heatmap colorization.
- `Civ: sites` — color-by-population on a clear background.
- `Civ: roads` — road intensity heatmap.
- `Civ: day overlay` — `civ.DayColor` rendered at full opacity.
- `Civ: night lights` — `civ.Night` cube-map.

- [ ] **Step 1: Add stages**
- [ ] **Step 2: Test `TestRenderRockyDebugRegistersCivStages`**
- [ ] **Step 3: Commit**

```
git commit -m "P9b: civ debug stages in planet-explorer"
```

---

## Task 8: cmd output + statistical invariants

**Files:**
- Modify: `cmd/generate-planet-maps/main.go`
- Modify: `cmd/generate-planet-maps/invariants_test.go`

- [ ] **Step 1: Add nightside PNG output**

```go
if cm := render.RenderNightCubeMap(profile, seedHash, faceSize); cm != nil {
    nightPath := strings.TrimSuffix(outPath, ".cube.png") + ".night.cube.png"
    writeCubeCrossPNG(nightPath, cm)
}
```

- [ ] **Step 2: Statistical invariants**

```go
func TestPhase9bCivInvariants(t *testing.T) {
    // For terran with Civ.Tier=0.5 at S=128:
    // - At least 30 sites generated
    // - Population follows Zipf within 20% slope tolerance
    // - All sites in habitable regions (habitability > 0.4)
    // - Roads cover < 0.5% of total surface (not runaway)
    // - Night cube-map is non-empty (>0.1% of pixels above intensity 0.3)
}
```

- [ ] **Step 3: Commit**

```
git commit -m "P9b: cmd output + statistical invariants for civ"
```

---

## Task 9: Bake civ goldens

**Files:**
- New: `<terran|super_terran>.cube.png` updated, `<terran|super_terran>.night.cube.png` new

The main color cube-map for terran/super_terran will diff because of daytime city patches + agriculture + roads. The new `.night.cube.png` files are entirely new.

- [ ] **Step 1: Bake**

```
go test -timeout 90m ./cmd/generate-planet-maps/ -run TestGolden -update
```

(90 min cap because civ adds Poisson sampling + Delaunay + A* — at S=1024 this can be slow.)

- [ ] **Step 2: Determinism check**

```
go test -timeout 90m ./cmd/generate-planet-maps/ -run TestGolden
```

Expected: zero diff.

- [ ] **Step 3: Visual review**

```
go run ./cmd/tools/planet-image-diff -- terran
go run ./cmd/tools/planet-image-diff -- terran.night
```

Confirm: city patches visible at habitable lowlands; nightside lights cluster in coastal/riverine regions.

- [ ] **Step 4: Commit**

```
git add cmd/generate-planet-maps/testdata/golden/
git commit -m "P9b: goldens — civ overlays + night cube-map (item 18)"
```

---

## Task 10: README + memory note

- [ ] **Step 1: README**

```markdown
## Phase 9b — Civilization signs (master-plan item 18)

**Civ pipeline.** Habitability scoring (`pkg/planetgen/feature/habitability.go`) → Bridson Poisson-disc on the sphere (`poisson_sphere.go`) → Zipfian population assignment → Delaunay+MST+A* road generation (`roads.go`) → daytime patch overlays + Black Marble nightside (`civ.go`).

Profile knobs: `Civ.Tier` (0 disables), `SiteMinDistRad`, `SiteMaxDistRad`, `MaxPopulation`, `NightLightHue`, `AgricultureRatio`.

Outputs: the existing `<name>.cube.png` is modified with daytime overlays; new `<name>.night.cube.png` is the Black Marble nightside. Both produced for archetypes with `Civ.Tier > 0` (terran, super_terran by default).

The `civTier ∈ [0,1]` knob is the single gate — gas giants, lava_world, ice_world, scorched all have `Tier=0` (no surface civilization).
```

- [ ] **Step 2: Memory note**

```markdown
## Phase 9b status

**Tier B Phase 9b complete** as of <date>:
- Item 18: civilization signs at `pkg/planetgen/feature/{civ,habitability,poisson_sphere,roads}.go`. Outputs daytime overlays + Black Marble nightside.

**Tier B fully complete.** Items 14, 15, 16, 17, 18, 19, 20 all landed. Item 6/10 retroactive rewires landed in Phase 8.

**Phase 9b commits**: TBD list.
```

- [ ] **Step 3: Commit**

```
git commit -m "P9b: README + memory note"
```

---

## Acceptance gates (run all after Task 10)

```
go build ./...
go test ./pkg/planetgen/... ./cmd/generate-planet-maps/...
golangci-lint run ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/
go test -run TestGolden ./cmd/generate-planet-maps/   # determinism: no -update
```

All green.

1. Phase 8 + 9a acceptance gates still pass.
2. `TestPhase9bCivInvariants` passes.
3. Civ goldens (`.cube.png` updated for terran/super_terran; `.night.cube.png` new) committed; no-update re-run passes.
4. Planet-explorer renders all 5 civ debug stages on terran without crashing.
5. Profile JSON serialization round-trip passes for the 6 new `Civ.*` fields.
6. README + memory note updated.

---

## Risks

- **Wasm bundle size.** Civ adds ~600 LOC + two new deps (`poissondisc`, `delaunay`). Estimate: +300 KB to wasm. Monitor in Task 7 — if explorer load >5s, consider lazy-loading civ via a separate wasm module.
- **A* on the cube-map at S=1024 with face hops.** Worst case: a continent-spanning road crosses 3+ faces at ~1000 pixels each → A* explores millions of cells. Mitigation: cap A* expansion at `~10x great-circle distance / pixel_size` and fall back to direct-line interpolation when the cap is hit. Reasonable terrain produces no fallback; the cap protects pathological inputs.
- **Determinism.** Poisson-disc + Delaunay + A* all have map iteration / unstable sort gotchas. Each step's tests must include a determinism check; the integration test re-runs the full civ pipeline twice and asserts byte-equal `*.cube.png`.
- **Per-archetype tuning.** With only `terran` and `super_terran` enabled by default, the visual review in Task 9 must scrutinize whether the Civ.Tier=0.3 vs 0.5 produces a recognizable difference. If they look identical, lower super_terran to 0.15 or change another knob.

---

## Out-of-scope follow-ups (post-Phase 9)

- Tier C polish (master-plan items 21-25): wind streaks, dunes, chaos terrain, lava lobes, reaction-diffusion ripples.
- Profile-driven civ aesthetics (e.g. coastal vs inland city distribution; medieval vs modern road style).
- Animated time-of-day cube-map (interpolating between day/night).
