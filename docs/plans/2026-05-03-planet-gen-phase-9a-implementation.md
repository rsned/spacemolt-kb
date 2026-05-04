# Planet Generation Phase 9a Implementation Plan: Cloud Layer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a separate cloud-cover overlay produced as an independent alpha cube-map, output as `<name>.clouds.cube.png` for atmospheric archetypes (terran, super_terran, oceanic, hothouse). Phase 9a does NOT modify the existing colored cube-map output.

**Date:** 2026-05-03
**Branch:** `phase-0/cube-map` (worktree `/home/robert/spacemolt/kb-phase-0-cube-map`)
**Predecessors:** Phase 8 (commits `6d221b0a` … `9af6a40c`).
**Master plan reference:** `docs/plans/2026-04-25-planet-gen-master-plan.md` §7, item 17.

**Architecture:** Cloud generation is a pure function of (seed, profile, S). Latitude-banded coverage × domain-warped fBm × Rankine-vortex storms produces a per-pixel alpha + density value; fake self-shadowing modulates brightness based on the density gradient projected onto a fixed sun direction. Output is a separate cube-map (CubeMapF for density, with optional CubeMap for the rendered RGBA preview). The main color cube-map output is unchanged; the cloud cube-map is an additional artifact at `cmd/generate-planet-maps/testdata/golden/<name>.clouds.cube.png`.

**Tech Stack:** Go 1.24+, `pkg/planetgen/feature/` package (alongside crater.go), wasm via planet-explorer cloud debug stage, golangci-lint hard gate, golden-image regression test.

**Phase 9 split:**

| Sub-phase | Items | Status |
|---|---|---|
| **Phase 9a (this plan — clouds)** | 17 | this doc |
| Phase 9b (civilization signs) | 18 | follow-on |

---

## File Structure

**Created:**
- `pkg/planetgen/feature/clouds.go` — cloud cube-map generation (~250 LOC)
- `pkg/planetgen/feature/clouds_test.go` — unit + invariants
- `cmd/generate-planet-maps/testdata/golden/<name>.clouds.cube.png` — 4 new golden PNGs (terran, super_terran, oceanic, hothouse)

**Modified:**
- `pkg/planetgen/types/types.go` — add `CloudConfig`, `Cloud CloudConfig` field on `PlanetProfile`
- `pkg/planetgen/profile.go` — per-archetype `Cloud:` defaults
- `pkg/planetgen/render/rocky.go` — invoke cloud generation for atmospheric archetypes; expose via new `RenderCloudCubeMap` entry point
- `pkg/planetgen/render/debug.go` — register `Cloud: density`, `Cloud: alpha`, `Cloud: shaded` debug stages
- `cmd/generate-planet-maps/main.go` — write `<name>.clouds.cube.png` next to the existing `<name>.cube.png`
- `cmd/generate-planet-maps/golden_test.go` — extend `TestGolden` to compare cloud goldens (or add a parallel `TestGoldenClouds`)
- `cmd/generate-planet-maps/invariants_test.go` — `TestPhase9aCloudInvariants`

---

## Task 1: Cloud profile knob

**Files:**
- Modify: `pkg/planetgen/types/types.go` — add `CloudConfig` and `Cloud CloudConfig` field
- Modify: `pkg/planetgen/profile.go` — per-archetype defaults

- [ ] **Step 1: Add the config**

```go
// CloudConfig parameterizes the separate cloud-cover cube-map.
// Coverage == 0 disables cloud generation entirely.
type CloudConfig struct {
    Coverage     float64 `json:"coverage,omitempty"`     // [0,1] mean cloud fraction at the equator
    BandLatRad   float64 `json:"bandLatRad,omitempty"`   // band half-width in radians; typical π/12
    Freq         float64 `json:"freq,omitempty"`         // base fbm frequency; typical 4
    Octaves      int     `json:"octaves,omitempty"`      // typical 4
    WarpAmp      float64 `json:"warpAmp,omitempty"`      // domain-warp amplitude; typical 0.4
    StormCount   int     `json:"stormCount,omitempty"`   // Rankine-vortex storms; typical 3-8
    StormRadiusRad float64 `json:"stormRadiusRad,omitempty"` // storm radius; typical π/16
    SunDir       [3]float64 `json:"sunDir,omitempty"`    // unit vector for fake self-shadow; default (1,0.3,0)
    ShadowGain   float64 `json:"shadowGain,omitempty"`   // multiplier on density gradient; typical 0.5
}
```

Add `Cloud CloudConfig` to `PlanetProfile` (append at end to avoid conflict with any future struct edits).

- [ ] **Step 2: Per-archetype defaults**

| archetype | Coverage | BandLat | StormCount | StormRadius | SunDir |
|---|---|---|---|---|---|
| terran | 0.45 | 0.26 | 5 | 0.20 | (1, 0.3, 0) |
| super_terran | 0.50 | 0.26 | 6 | 0.18 | (1, 0.3, 0) |
| oceanic | 0.65 | 0.30 | 8 | 0.22 | (1, 0.3, 0) |
| hothouse | 0.85 | 0.40 | 4 | 0.30 | (1, 0.3, 0) (Venus-like opaque cover) |
| jovian | 0.95 | 0.18 | 12 | 0.10 | (1, 0.3, 0) (gas-giant banding) |
| ice_giant | 0.85 | 0.20 | 8 | 0.12 | (1, 0.3, 0) |
| all rocky non-atmospheric (scorched, lava_world, ice_world, glacial, tundra, arid, unknown) | 0 | n/a | n/a | n/a | n/a |

For each `Profiles["..."]` entry, add a `Cloud:` block with these values. Use Go zero-value (`Cloud: types.CloudConfig{}` or omit) for disabled archetypes.

- [ ] **Step 3: Tests, lint, commit**

```
go test ./pkg/planetgen/types/... -count=1
go test ./pkg/planetgen/... -count=1
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/types/types.go pkg/planetgen/profile.go
git commit -m "P9a: CloudConfig profile knob + per-archetype defaults (item 17)"
```

---

## Task 2: Implement `clouds.go`

**Files:**
- Create: `pkg/planetgen/feature/clouds.go`

**Algorithm.**

1. **Latitude-banded coverage**: at sphere direction `(dx, dy, dz)`, latitude `lat = asin(dy)`. Coverage envelope = `Coverage * cos(lat)^k` (k=2 typical) plus narrow ITCZ-style bands at midlatitudes. Simple baseline: `envelope = Coverage * cos(lat)`.
2. **Domain-warped fBm**: `dir' = dir + WarpAmp * vec_fbm(dir, Freq, 2)`; `density = fbm(dir', Freq, Octaves)`.
3. **Rankine vortex storms**: generate `StormCount` Fibonacci-spiral centers at random latitudes (drawn from a band), each with a rotation sense and angular radius `StormRadiusRad`. For each pixel, sample the worst-case overlap with any storm: `r = angular_distance(dir, storm_center)`. If `r < StormRadius`, density gets a Rankine-vortex spiral profile multiplied in (or added).
4. **Combine**: `density_final = clamp(envelope + 2*density - 1 + storm_contribution, 0, 1)`.
5. **Fake self-shadow**: compute the gradient of density in tangent space at each pixel; project onto `SunDir` to get a directional brightness multiplier. `lit_density = density_final * (1 + ShadowGain * (gradient · sun_tangent))`.

**API:**

```go
// CloudField holds per-pixel cloud density (post-shadow) on a cube-map.
// Density is in [0, 1]; alpha for the rendered PNG is density itself.
type CloudField struct {
    Size    int
    Density [cubemap.NumFaces][]float64 // post-shadow brightness; 0=clear, 1=opaque-bright
    Alpha   [cubemap.NumFaces][]float64 // pre-shadow density; the actual cloud fraction
}

// GenerateClouds returns a CloudField for the given profile + seed.
// Returns nil when cfg.Coverage == 0.
func GenerateClouds(profile *types.PlanetProfile, seed int64, S int) *CloudField
```

**Seed namespaces:** `clouds.warp`, `clouds.density`, `clouds.storm.centers`, `clouds.storm.rotation`. Each via `seed.Domain(master, name)`.

**Cube-seam continuity:** because cloud generation is a pure function of sphere direction (no per-face indexing), seams are continuous by construction. Add a `seamtest.AssertSeamContinuity` test at 1% of range threshold (per the Phase 7 convention for direction-pure fields).

- [ ] **Step 1: Write a failing test**

`pkg/planetgen/feature/clouds_test.go`:

```go
package feature

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
)

func TestGenerateCloudsDeterministic(t *testing.T) {
    p := planetgen.Profiles["terran"]
    a := GenerateClouds(p, 42, 64)
    b := GenerateClouds(p, 42, 64)
    if a == nil || b == nil { t.Fatal("expected non-nil") }
    for f := range a.Density {
        for i := range a.Density[f] {
            if a.Density[f][i] != b.Density[f][i] {
                t.Fatalf("non-deterministic: face %d pixel %d", f, i)
            }
        }
    }
}

func TestGenerateCloudsDisabledByZeroCoverage(t *testing.T) {
    p := planetgen.Profiles["scorched"]
    if cf := GenerateClouds(p, 42, 64); cf != nil {
        t.Errorf("zero-coverage archetype returned non-nil")
    }
}

func TestCloudsCoverageInRange(t *testing.T) {
    p := planetgen.Profiles["terran"]
    cf := GenerateClouds(p, 42, 64)
    var sum float64
    var n int
    for f := range cf.Alpha {
        for _, v := range cf.Alpha[f] {
            if v < 0 || v > 1 { t.Fatalf("alpha out of [0,1]: %v", v) }
            sum += v
            n++
        }
    }
    mean := sum / float64(n)
    // Expected mean alpha is roughly Coverage * mean(cos(lat)) ≈ Coverage * 0.64.
    // Allow ±20% tolerance on the actual Coverage.
    expectedMin := p.Cloud.Coverage * 0.64 * 0.8
    expectedMax := p.Cloud.Coverage * 0.64 * 1.2
    if mean < expectedMin || mean > expectedMax {
        t.Errorf("terran mean alpha %.3f outside [%.3f, %.3f]", mean, expectedMin, expectedMax)
    }
}

func TestCloudsSeamContinuity(t *testing.T) {
    p := planetgen.Profiles["terran"]
    cf := GenerateClouds(p, 42, 64)
    cm := &cubemap.CubeMapF{Size: 64}
    for i := range cm.Faces {
        cm.Faces[i] = cf.Alpha[i]
    }
    // Direction-pure field; tight tolerance.
    seamtest.AssertSeamContinuity(t, "terran:Alpha", cm, 0.05)
    // 5% absorbs the seamtest pixel-snap floor (per P8 Task 4 lessons).
}
```

Run, expect FAIL — `clouds.go` doesn't exist yet.

- [ ] **Step 2: Implement `GenerateClouds`**

```go
package feature

import (
    "math"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func GenerateClouds(profile *types.PlanetProfile, master int64, S int) *CloudField {
    cfg := profile.Cloud
    if cfg.Coverage <= 0 {
        return nil
    }

    cf := &CloudField{Size: S}
    for f := range cf.Density {
        cf.Density[f] = make([]float64, S*S)
        cf.Alpha[f] = make([]float64, S*S)
    }

    warpGen := noise.New(seed.Domain(master, "clouds.warp"))
    densGen := noise.New(seed.Domain(master, "clouds.density"))
    storms := generateStorms(cfg, master)

    sun := cfg.SunDir
    if sun[0] == 0 && sun[1] == 0 && sun[2] == 0 {
        sun = [3]float64{1, 0.3, 0}
    }
    sun = normalize(sun)

    // First pass: alpha (raw cloud density).
    for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
        for py := 0; py < S; py++ {
            for px := 0; px < S; px++ {
                dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
                lat := math.Asin(dy)

                // 1. Latitude band.
                envelope := cfg.Coverage * math.Cos(lat)

                // 2. Domain warp + fbm density.
                wx := warpGen.FractalNoise3D(dx*cfg.Freq, dy*cfg.Freq, dz*cfg.Freq, 2, 2.0, 0.5)
                wy := warpGen.FractalNoise3D(dy*cfg.Freq, dz*cfg.Freq, dx*cfg.Freq, 2, 2.0, 0.5)
                wz := warpGen.FractalNoise3D(dz*cfg.Freq, dx*cfg.Freq, dy*cfg.Freq, 2, 2.0, 0.5)
                wdx, wdy, wdz := dx+cfg.WarpAmp*wx, dy+cfg.WarpAmp*wy, dz+cfg.WarpAmp*wz
                density := densGen.FractalNoise3D(wdx*cfg.Freq, wdy*cfg.Freq, wdz*cfg.Freq, cfg.Octaves, 2.0, 0.5)
                density = (density + 1) * 0.5 // [-1,1] → [0,1]

                // 3. Storm contribution.
                stormBoost := stormContribution(storms, dx, dy, dz, cfg)

                // 4. Combine.
                alpha := envelope + 0.6*(2*density-1) + stormBoost
                if alpha < 0 { alpha = 0 }
                if alpha > 1 { alpha = 1 }
                cf.Alpha[face][py*S+px] = alpha
            }
        }
    }

    // Second pass: shadow.
    for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
        for py := 0; py < S; py++ {
            for px := 0; px < S; px++ {
                a := cf.Alpha[face][py*S+px]
                grad := approximateAlphaGradient(cf, face, px, py, S)
                shade := dot(grad, sun) * cfg.ShadowGain
                lit := a * (1 + shade)
                if lit < 0 { lit = 0 }
                if lit > 1 { lit = 1 }
                cf.Density[face][py*S+px] = lit
            }
        }
    }

    return cf
}
```

Helpers:
- `generateStorms(cfg, master) []Storm` — Fibonacci-spiral `cfg.StormCount` directions plus a tiny RNG-jittered offset, each with a rotation sense `±1`.
- `stormContribution(storms, dx,dy,dz, cfg) float64` — sum over storms of `max(0, 1 - r/cfg.StormRadiusRad)` × Rankine-vortex spiral angle factor. Cap at +0.5 to avoid totally clobbering the rest.
- `approximateAlphaGradient(cf, face, px, py, S) [3]float64` — finite difference of the Alpha field via `cubemap.FacePixelNeighbors4` (cross-face-aware). Returns a vector in 3-space (gradient projected from spherical tangent plane to ambient).

- [ ] **Step 3: Run tests, lint, build**

```
go test ./pkg/planetgen/feature/ -count=1
go test ./pkg/planetgen/... -count=1
golangci-lint run ./pkg/planetgen/...
go build ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/
```

- [ ] **Step 4: Commit**

```
git add pkg/planetgen/feature/clouds.go pkg/planetgen/feature/clouds_test.go
git commit -m "P9a: cloud cube-map generator (item 17)"
```

---

## Task 3: Wire clouds into the rocky pipeline + expose new entry point

**Files:**
- Modify: `pkg/planetgen/render/rocky.go` — add `RenderCloudCubeMap` orchestrator
- Modify: `pkg/planetgen/render/debug.go` — register cloud debug stages
- Modify: `pkg/planetgen/render/debug_palette.go` — add helpers if needed

- [ ] **Step 1: Add `RenderCloudCubeMap`**

```go
// RenderCloudCubeMap returns the cloud overlay as a separate cube-map.
// Returns nil when profile.Cloud.Coverage == 0 (cloud-less archetype).
// Renders Density (post-shadow) into RGBA: each channel = uint8(255 *
// density), alpha = uint8(255 * alpha) so the PNG is a partially-
// transparent overlay over the planet color cube-map.
func RenderCloudCubeMap(profile *types.PlanetProfile, seed int64, S int) *cubemap.CubeMap
```

Implementation: `cf := feature.GenerateClouds(profile, seed, S)`; if nil → nil; else allocate a `*cubemap.CubeMap` and fill RGBA per pixel.

The main color cube-map output (RenderRocky / RenderGasGiant) is unchanged. Clouds are an *additional* artifact alongside.

- [ ] **Step 2: Cloud debug stages**

In `pkg/planetgen/render/debug.go`, after the existing flow + rainshadow stages:

```go
if cf := feature.GenerateClouds(profile, seed, S); cf != nil {
    frame.Stages = append(frame.Stages,
        DebugStage{Name: "Cloud: alpha", Kind: "field", ScalarAfter: scalarFromCubeFaces(cf.Alpha, S)},
        DebugStage{Name: "Cloud: density", Kind: "field", ScalarAfter: scalarFromCubeFaces(cf.Density, S)},
        DebugStage{Name: "Cloud: shaded", Kind: "color", ColorAfter: paintCloudShaded(cf, S)},
    )
}
```

`scalarFromCubeFaces` likely already exists — match the helper used by Phase 8 Task 9. If not, add it.

`paintCloudShaded` renders the cloud Density as grayscale RGBA (no transparency in debug stage).

- [ ] **Step 3: Test the entry point**

```go
func TestRenderCloudCubeMapTerranNonNil(t *testing.T) {
    cm := render.RenderCloudCubeMap(planetgen.Profiles["terran"], 42, 64)
    if cm == nil { t.Fatal("expected non-nil for terran") }
}

func TestRenderCloudCubeMapDisabledForScorched(t *testing.T) {
    cm := render.RenderCloudCubeMap(planetgen.Profiles["scorched"], 42, 64)
    if cm != nil { t.Errorf("expected nil for scorched") }
}

func TestRenderRockyDebugRegistersCloudStages(t *testing.T) {
    frame := render.RenderRockyDebug(planetgen.Profiles["terran"], 42, 64, nil)
    var foundAlpha, foundDensity, foundShaded bool
    for _, st := range frame.Stages {
        switch st.Name {
        case "Cloud: alpha":   foundAlpha = true
        case "Cloud: density": foundDensity = true
        case "Cloud: shaded":  foundShaded = true
        }
    }
    if !foundAlpha   { t.Errorf("missing Cloud: alpha") }
    if !foundDensity { t.Errorf("missing Cloud: density") }
    if !foundShaded  { t.Errorf("missing Cloud: shaded") }
}
```

- [ ] **Step 4: Commit**

```
git add pkg/planetgen/render/rocky.go pkg/planetgen/render/debug.go pkg/planetgen/render/debug_palette.go pkg/planetgen/render/cloud_render_test.go
git commit -m "P9a: RenderCloudCubeMap entry point + debug stages"
```

---

## Task 4: Output cloud PNG from `cmd/generate-planet-maps`

**Files:**
- Modify: `cmd/generate-planet-maps/main.go` — write `<name>.clouds.cube.png` next to `<name>.cube.png`
- Modify: `cmd/generate-planet-maps/golden_test.go` — extend `TestGolden` (or add `TestGoldenClouds`)

- [ ] **Step 1: Add cloud PNG output**

In the per-planet rendering loop in `cmd/generate-planet-maps/main.go`, after the existing color cube-map is written:

```go
if cm := render.RenderCloudCubeMap(profile, seedHash, faceSize); cm != nil {
    cloudPath := strings.TrimSuffix(outPath, ".cube.png") + ".clouds.cube.png"
    if err := writeCubeCrossPNG(cloudPath, cm); err != nil {
        log.Fatalf("write clouds: %v", err)
    }
}
```

- [ ] **Step 2: Extend goldens**

`TestGolden` walks the curated 13-planet set and compares each PNG to the committed golden. Add cloud golden comparison: for archetypes with `Cloud.Coverage > 0` (terran, super_terran, oceanic, hothouse, jovian, ice_giant), compare `<name>.clouds.cube.png` to its committed golden. ΔE2000 threshold same as the main golden (e.g. 1.0).

If the structure of `golden_test.go` is per-planet, add one more file to compare per planet. Read the existing test to match its style.

- [ ] **Step 3: Statistical invariants**

`cmd/generate-planet-maps/invariants_test.go::TestPhase9aCloudInvariants`:

```go
func TestPhase9aCloudInvariants(t *testing.T) {
    archetypes := []string{"terran", "super_terran", "oceanic", "hothouse"}
    S := 128
    for _, name := range archetypes {
        t.Run(name, func(t *testing.T) {
            p := planetgen.Profiles[name]
            cf := feature.GenerateClouds(p, 42, S)
            // Mean alpha matches Coverage * cos(lat) average within ±20%.
            // Variance > 0.005 (the field is non-trivial).
            // Storm centers visibly contribute (variance with storms > variance without by 20%+).
            ...
        })
    }
}
```

The 3-test pattern (mean, variance, storm contribution) ensures the algorithm parts are all firing.

- [ ] **Step 4: Run all + commit (no goldens yet)**

```
go test ./pkg/planetgen/... ./cmd/generate-planet-maps/... -count=1 -run "^(?!TestGolden)"
golangci-lint run ./...
git add cmd/generate-planet-maps/main.go cmd/generate-planet-maps/golden_test.go cmd/generate-planet-maps/invariants_test.go
git commit -m "P9a: cmd output + statistical invariants for cloud overlay"
```

---

## Task 5: Bake cloud goldens

**Files:**
- New: 6 PNGs at `cmd/generate-planet-maps/testdata/golden/<name>.clouds.cube.png` (terran, super_terran, oceanic, hothouse, jovian, ice_giant)

- [ ] **Step 1: Bake**

```
go test -timeout 60m ./cmd/generate-planet-maps/ -run TestGolden -update
```

Extends the existing 60 min Phase 8 bake by ~5-10 min for the 4-6 cloud renders.

- [ ] **Step 2: Determinism check**

```
go test -timeout 60m ./cmd/generate-planet-maps/ -run TestGolden
```

Expected: PASS with zero diff.

- [ ] **Step 3: Visual inspection**

```
go run ./cmd/tools/planet-image-diff -- terran.clouds
```

Confirm: clouds are recognizable atmospheric overlays — bands at midlatitudes, storms visible as spirals, land/ocean texture beneath unaffected.

- [ ] **Step 4: Commit**

```
git add cmd/generate-planet-maps/testdata/golden/
git commit -m "P9a: cloud goldens — first bake (item 17)"
```

---

## Task 6: README + memory note

- [ ] **Step 1: Append to `cmd/generate-planet-maps/README.md`**

```markdown
## Phase 9a — Cloud overlay (master-plan item 17)

**Clouds** (`pkg/planetgen/feature/clouds.go`). Separate cube-map output: latitude-banded coverage × domain-warped fBm × Rankine-vortex storms; fake self-shadow via density-gradient · sun-direction. Output as `<name>.clouds.cube.png` for atmospheric archetypes (terran, super_terran, oceanic, hothouse) and gas giants (jovian, ice_giant).

Profile knobs: `Cloud.Coverage` (0 disables), `BandLatRad`, `Freq`, `Octaves`, `WarpAmp`, `StormCount`, `StormRadiusRad`, `SunDir`, `ShadowGain`.

The main color cube-map output is unchanged; clouds are an additional artifact alongside.
```

- [ ] **Step 2: Memory note**

Update `/home/robert/.claude/projects/-home-robert-spacemolt-kb/memory/project_planet_gen.md` with a `## Phase 9a status` section before "Future work":

```markdown
## Phase 9a status

**Tier B Phase 9a complete** as of <date>:
- Item 17: cloud-cover overlay at `pkg/planetgen/feature/clouds.go`. Output as `<name>.clouds.cube.png` for atmospheric archetypes + gas giants.

**Phase 9a deferred to Phase 9b**: item 18 (civilization signs).

**Phase 9a commits**: TBD list.
```

- [ ] **Step 3: Commit**

```
git add cmd/generate-planet-maps/README.md
git commit -m "P9a: README + memory note"
```

---

## Acceptance gates (run all after Task 6)

```
go build ./...
go test ./pkg/planetgen/... ./cmd/generate-planet-maps/...
golangci-lint run ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/
go test -run TestGolden ./cmd/generate-planet-maps/   # determinism: no -update
```

All green.

1. Phase 8 acceptance gates still pass.
2. `TestPhase9aCloudInvariants` passes.
3. `TestRockyHeightmapSeamContinuity` and the other previously-skipped tests stay in their current skip state — Phase 9a does not touch them.
4. Cloud goldens committed; second-pass `go test` no `-update` produces no diff.
5. Planet-explorer renders 3 cloud debug stages on every atmospheric archetype.
6. Profile JSON serialization round-trip passes for the 9 new `Cloud.*` fields.
7. README and memory note updated.

---

## Risks

- **Cloud generation cost.** Domain-warped fBm + storm contribution at S=1024 ~6M pixels × ~10 fbm taps × 6 faces ≈ 360M noise calls per planet. Estimate: ~30s per atmospheric planet at S=1024. Acceptable; matches the existing rocky pipeline's per-pixel cost.
- **Cloud-on-gas-giant ambiguity.** The "color cube-map" for jovian/ice_giant is ALREADY a cloud-band rendering (`render.RenderGasGiant`). Adding a separate `.clouds.cube.png` for gas giants doesn't make sense as an *overlay* — they don't have a separate land surface beneath. Choose: (a) skip cloud output for gas giants and only enable for atmospheric *rocky* archetypes; (b) output the cloud cube-map for gas giants but document that it's redundant with the main render. The plan above defaults to (a) — `Cloud:` blocks for jovian/ice_giant exist in profile.go but the goldens-write logic at Task 4 Step 1 should skip gas giants. Decide at implementation time.
- **Wasm bundle size.** Clouds add ~250 LOC + new fbm generators. Mitigation: `-ldflags="-s -w"` already applied; revisit if explorer load regresses.
- **Storm centers may cluster at poles** under naive Fibonacci-spiral with latitude restriction. Mitigation: visual inspection in Task 5 Step 3.

---

## Out-of-scope follow-ups (Phase 9b)

- Item 18 — Civilization signs (habitability scoring + Bridson Poisson + Black Marble nightside).
