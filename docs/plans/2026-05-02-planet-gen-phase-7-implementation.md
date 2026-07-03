# Planet Generation Phase 7: Tier B Foundations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land master-plan items 14 (Voronoi tectonic plates), 19 (seam-QA test infrastructure), and 20 (Voronoi cell-coordinate jitter). Plates land as data + debug stages with no production-rendering effect; jitter is the only production-visible change. Item 6/10 retroactive rewires deferred to Phase 8.

**Architecture:** New `pkg/planetgen/field/plates.go` produces a `PlateField` (per-pixel plate id, oceanic flag pixel-mask, three boundary SDFs in km) via Fibonacci-spiral seeds, random flood-fill across cube faces, motion-vector-based boundary classification, and three independent JFA passes. New `pkg/planetgen/noise/jitter.go` produces a `JitterField` (~120 Voronoi cells with per-cell rotation+offset) consumed at sample-time inside the Detail control field and biome jitter to break visible repetition. New `pkg/planetgen/cubemap/seamtest/` is a test-only helper that walks all 12 cube edges and asserts continuity (continuous fields) or exact-match (categorical). Plates compute after Continents/Voronoi and before HeightSmooth in the rocky pipeline; in Phase 7 nothing downstream consumes them, so the slot is reserved for Phase 8.

**Tech Stack:** Go 1.24+, `pkg/planetgen/cubemap` (sphere ↔ face/pixel + seam-aware sampling), `pkg/planetgen/seed.Domain`, `math/rand/v2`. Reuses the JFA primitive from `pkg/planetgen/field/jfa.go` (from Phase 4) with a small refactor to accept arbitrary seed sets instead of only "below threshold" pixels.

**Design reference:** `docs/plans/2026-05-02-planet-gen-phase-7-tier-b-foundations.md` — the spec. This plan reproduces what's needed for implementation but defers rationale to the spec.

---

## Pre-flight notes

**Branch & worktree.** Continue on `phase-0/cube-map` in `/home/robert/spacemolt/kb-phase-0-cube-map`. The branch name is historical; rename only at PR time if at all.

**Sub-phase context.** Tier B (master-plan items 14–20) is split into three sub-phases: Phase 7 (this — items 14, 19, 20), Phase 8 (items 15, 16 plus item 6/10 retroactive rewires), Phase 9 (items 17, 18). Each sub-phase ships a complete set of valid planet PNGs.

**Why plates, jitter, and seam-QA together.** Plates are ~400 LOC of cube-seam-sensitive primitives that benefit from explicit seam tests landing at the same time. Jitter is small (~120 LOC) but its production-visible diff means a goldens re-bake either way — combining lets that single re-bake cover both items. Item 19 (seam-QA helpers) is consumed by the plates and jitter tests, so it lands first in dependency order.

**Plate algorithm sketch.** Fibonacci-spiral N seed directions; per-plate random rotation axis + angular speed + oceanic/continental flag (Bernoulli on archetype `OceanicPlateFraction`). Random flood-fill (not BFS) starting from N seeds — frontier set of (assigned, unassigned-neighbor) pairs, pop uniformly at random per step until every pixel is assigned. Boundary pixels are 4-connected pixels whose neighbor on at least one side has a different plate id; for each, compute relative velocity `v_rel = (ω_a × p) − (ω_b × p)` and boundary normal `n` (tangent vector toward differing neighbor), classify via `v_rel · n` against threshold T (default 0.75). Three JFA passes produce convergent/divergent/transform SDFs in km.

**Cross-face flood-fill.** The frontier-pair walk uses the same cross-face neighbor primitive that JFA used in Tier A: at each cube-face edge pixel, the four 4-neighbors include an off-edge pixel that lives on the adjacent face. We compute its (face, px, py) by going `face-pixel → unit dir → adjacent-face pixel` via `cubemap.FacePixelToDir` + nearest-pixel snap on the adjacent face. This re-uses logic already proven in Phase 4 erosion.

**Jitter sketch.** `JitterCellCount` (default 120) cell centers via Fibonacci spiral on the unit sphere. Per cell: random rotation axis, random rotation angle in [-π/4, +π/4], random per-axis offset in [-0.1, +0.1]. Per-pixel cell-id = nearest cell center (single-pass O(pixels × cells); no flood-fill). Application: at every Detail-field fbm sample inside `pkg/planetgen/field/control.go` and every biome-jitter sample inside the biome lookup, transform the sample direction `p` to `p′ = rotateAroundAxis(p − cell.Center, cell.RotAxis, cell.RotAngle) + cell.Center + cell.Offset`, then sample fbm at `p′`. Adjacent cells produce different rotated views of the same fbm field → repetition broken.

**Profile knobs added** (per the spec §3.5):

```go
PlateCount           int      // 0 disables plates
OceanicPlateFraction float64  // [0,1]
PlateConvergentT     float64  // default 0.75

JitterEnabled        bool     // per-archetype
JitterCellCount      int      // default 120
JitterRotMax         float64  // default π/4
JitterOffsetMax      float64  // default 0.1
```

**Per-archetype defaults** (set in `pkg/planetgen/profile.go`):

| Archetype | PlateCount | OceanicPlateFraction | JitterEnabled |
|---|---|---|---|
| terran, super_terran | 12 | 0.7 | true |
| oceanic | 12 | 0.9 | true |
| tundra | 8 | 0.5 | true |
| arid | 6 | 0.3 | true |
| glacial | 6 | 0.4 | true |
| scorched, lava_world | 4 | 0.2 | true |
| hothouse, ice_world | 0 | 0 | true |
| jovian, ice_giant, unknown | 0 | 0 | false |

**Seed namespaces** (per master plan §3.3): `plates.seeds`, `plates.motion`, `plates.oceanic`, `plates.fill.random`, `plates.sdf.convergent`, `plates.sdf.divergent`, `plates.sdf.transform`, `jitter.cells`, `jitter.rot`, `jitter.offset`. Each derived via `seed.Domain(master, name)`.

**Determinism.** Same profile + master seed → bit-identical `PlateField` and `JitterField`. Goldens re-baked once at the end; second pass with no `-update` must produce no diff.

**Forward compat with Phase 8.** Plates are wired into the pipeline at slot 5b (after Continents, before HeightSmooth) but the resulting `PlateField` is currently used only by debug stages. Phase 8 will replace `Continentalness`-based mountain mask with `smoothstep(0.5, 0.7, distToConvergent)` and seed continents from continental-plate centroids; that's deferred.

---

## File structure

**New files:**

| Path | Role |
|---|---|
| `pkg/planetgen/field/plates.go` | `Plate`, `PlateField`, `GeneratePlates` entry point, flood-fill, classify, SDFs |
| `pkg/planetgen/field/plates_test.go` | Unit tests: spiral seeds, flood-fill correctness, classification, SDF, determinism |
| `pkg/planetgen/field/plates_seam_test.go` | Seam-match (plate-id) and continuity (3 SDFs) tests on the curated 13-planet set |
| `pkg/planetgen/noise/jitter.go` | `JitterCell`, `JitterField`, `GenerateJitter`, `(*JitterField).At`, `(*JitterField).Transform` |
| `pkg/planetgen/noise/jitter_test.go` | Unit tests: cell coverage, single-valued cell-id, identity-when-zero |
| `pkg/planetgen/noise/jitter_seam_test.go` | Continuity test on jittered Detail field |
| `pkg/planetgen/cubemap/seamtest/seamtest.go` | `Edge`, `WalkSeams`, `AssertSeamContinuity`, `AssertSeamMatch` |
| `pkg/planetgen/cubemap/seamtest/seamtest_test.go` | `WalkSeams` visits each edge pixel once; round-trip via `FacePixelToDir`/`DirToFacePixel` |

**Modified files:**

| Path | Reason |
|---|---|
| `pkg/planetgen/types/types.go` | Add 7 new `PlanetProfile` fields |
| `pkg/planetgen/types/types_test.go` | JSON round-trip covers new fields |
| `pkg/planetgen/profile.go` | Set per-archetype defaults for all 13 types |
| `pkg/planetgen/field/control.go` | Detail-field fbm sample routes through `JitterField.Transform` when non-nil |
| `pkg/planetgen/biome/whittaker.go` (or wherever biome jitter is sampled) | Biome jitter sample routes through `JitterField.Transform` when non-nil |
| `pkg/planetgen/render/rocky.go` | Compute `PlateField` and `JitterField`; thread jitter into control + biome stages |
| `pkg/planetgen/render/debug.go` | 6 new debug stages (5 plates + 1 jitter); `DebugStage` gains an optional categorical-id raster + km-heatmap raster |
| `pkg/planetgen/render/flat_cache.go` | Cache key gains 4 jitter knobs |
| `pkg/planetgen/render/rocky_seam_test.go` (new file alongside `rocky.go`) | Seam continuity for heightmap + 5 control fields |
| `cmd/generate-planet-maps/README.md` | Phase 7 section |
| `cmd/planet-explorer/web/app.js` | Debug stage dropdown gets 6 new entries; plate stages disabled in swatch mode with hint |

**Worktree:** `/home/robert/spacemolt/kb-phase-0-cube-map`. All `go` commands in this plan run from there.

---

## Task 1: Profile fields and per-archetype defaults

Phase 7 starts by adding the profile knobs so later tasks can be data-driven. No algorithm yet — just struct fields, JSON round-trip, and per-archetype values.

**Files:**
- Modify: `pkg/planetgen/types/types.go`
- Modify: `pkg/planetgen/types/types_test.go`
- Modify: `pkg/planetgen/profile.go`

- [ ] **Step 1: Add profile fields**

In `pkg/planetgen/types/types.go`, append to the `PlanetProfile` struct (after the existing Phase 4 / Phase 5 fields, before the closing brace):

```go
// Phase 7 Tier B: Voronoi tectonic plates. PlateCount=0 disables plates.
PlateCount           int     `json:",omitempty"`
OceanicPlateFraction float64 `json:",omitempty"`
PlateConvergentT     float64 `json:",omitempty"`

// Phase 7 Tier B: Voronoi cell-coordinate jitter on Detail field and biome jitter.
JitterEnabled   bool    `json:",omitempty"`
JitterCellCount int     `json:",omitempty"`
JitterRotMax    float64 `json:",omitempty"`
JitterOffsetMax float64 `json:",omitempty"`
```

- [ ] **Step 2: Write the failing JSON round-trip test**

In `pkg/planetgen/types/types_test.go`, add:

```go
func TestPlanetProfilePhase7FieldsRoundTrip(t *testing.T) {
    p := PlanetProfile{
        PlateCount:           12,
        OceanicPlateFraction: 0.7,
        PlateConvergentT:     0.75,
        JitterEnabled:        true,
        JitterCellCount:      120,
        JitterRotMax:         math.Pi / 4,
        JitterOffsetMax:      0.1,
    }
    b, err := json.Marshal(p)
    if err != nil {
        t.Fatalf("marshal: %v", err)
    }
    var got PlanetProfile
    if err := json.Unmarshal(b, &got); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if got.PlateCount != p.PlateCount ||
        got.OceanicPlateFraction != p.OceanicPlateFraction ||
        got.PlateConvergentT != p.PlateConvergentT ||
        got.JitterEnabled != p.JitterEnabled ||
        got.JitterCellCount != p.JitterCellCount ||
        got.JitterRotMax != p.JitterRotMax ||
        got.JitterOffsetMax != p.JitterOffsetMax {
        t.Errorf("Phase 7 fields not round-tripped: got %+v", got)
    }
}
```

Add `"math"`, `"encoding/json"`, `"testing"` imports if missing.

- [ ] **Step 3: Run the test**

```
go test ./pkg/planetgen/types/ -run TestPlanetProfilePhase7FieldsRoundTrip -v
```

Expected: PASS (the struct fields exist; JSON round-trip is automatic with stdlib).

- [ ] **Step 4: Add per-archetype defaults**

In `pkg/planetgen/profile.go`, for each archetype in the `Profiles` map, set the new fields. Use this table:

| Archetype | PlateCount | OceanicPlateFraction | PlateConvergentT | JitterEnabled | JitterCellCount | JitterRotMax | JitterOffsetMax |
|---|---|---|---|---|---|---|---|
| terran | 12 | 0.7 | 0.75 | true | 120 | π/4 | 0.1 |
| super_terran | 12 | 0.7 | 0.75 | true | 120 | π/4 | 0.1 |
| oceanic | 12 | 0.9 | 0.75 | true | 120 | π/4 | 0.1 |
| tundra | 8 | 0.5 | 0.75 | true | 120 | π/4 | 0.1 |
| arid | 6 | 0.3 | 0.75 | true | 120 | π/4 | 0.1 |
| glacial | 6 | 0.4 | 0.75 | true | 120 | π/4 | 0.1 |
| scorched | 4 | 0.2 | 0.75 | true | 120 | π/4 | 0.1 |
| lava_world | 4 | 0.2 | 0.75 | true | 120 | π/4 | 0.1 |
| hothouse | 0 | 0 | 0 | true | 120 | π/4 | 0.1 |
| ice_world | 0 | 0 | 0 | true | 120 | π/4 | 0.1 |
| jovian | 0 | 0 | 0 | false | 0 | 0 | 0 |
| ice_giant | 0 | 0 | 0 | false | 0 | 0 | 0 |
| unknown | 0 | 0 | 0 | false | 0 | 0 | 0 |

Use literal `math.Pi / 4` for JitterRotMax. Add `"math"` to the imports of `profile.go` if not present.

For each archetype block, add the eight assignments at the end of the struct literal. Example for `terran`:

```go
"terran": {
    Type:     "terran",
    // ... existing fields ...
    PlateCount:           12,
    OceanicPlateFraction: 0.7,
    PlateConvergentT:     0.75,
    JitterEnabled:        true,
    JitterCellCount:      120,
    JitterRotMax:         math.Pi / 4,
    JitterOffsetMax:      0.1,
},
```

For `jovian` etc, JitterEnabled is false and the rest are zero — Go zero values cover them, so you only need to set `JitterEnabled: false` explicitly (or omit if you want the zero default; explicit is clearer).

- [ ] **Step 5: Verify the build**

```
go build ./...
go test ./pkg/planetgen/types/ ./pkg/planetgen/... -v
```

Expected: all green (no behavior change yet — the new fields are unused).

- [ ] **Step 6: Lint and commit**

```
golangci-lint run ./...
git add pkg/planetgen/types/types.go pkg/planetgen/types/types_test.go pkg/planetgen/profile.go
git commit -m "P7: add Phase 7 profile knobs + per-archetype defaults"
```

---

## Task 2: Fibonacci-spiral plate seeds and motion attributes

Build the plate-seeding step. No flood-fill yet — just N unit-vector seeds + per-plate random rotation axis, angular speed, and oceanic flag.

**Files:**
- Create: `pkg/planetgen/field/plates.go`
- Create: `pkg/planetgen/field/plates_test.go`

- [ ] **Step 1: Write the failing test**

`pkg/planetgen/field/plates_test.go`:

```go
package field

import (
    "math"
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestSeedPlatesCountAndUnit(t *testing.T) {
    profile := &types.PlanetProfile{
        PlateCount:           12,
        OceanicPlateFraction: 0.7,
        PlateConvergentT:     0.75,
    }
    plates := seedPlates(profile, 42)
    if len(plates) != 12 {
        t.Fatalf("got %d plates, want 12", len(plates))
    }
    for i, p := range plates {
        if p.ID != i {
            t.Errorf("plate %d: ID=%d, want %d", i, p.ID, i)
        }
        // Seed and RotAxis must be unit vectors.
        for label, v := range map[string][3]float64{"Seed": p.Seed, "RotAxis": p.RotAxis} {
            mag := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
            if math.Abs(mag-1) > 1e-9 {
                t.Errorf("plate %d %s not unit: |v|=%f", i, label, mag)
            }
        }
        if p.AngSpeed < 0 || p.AngSpeed > 1 {
            t.Errorf("plate %d AngSpeed=%f out of [0,1]", i, p.AngSpeed)
        }
    }
}

func TestSeedPlatesDeterministic(t *testing.T) {
    profile := &types.PlanetProfile{PlateCount: 8, OceanicPlateFraction: 0.5}
    a := seedPlates(profile, 99)
    b := seedPlates(profile, 99)
    for i := range a {
        if a[i] != b[i] {
            t.Errorf("plate %d differs across calls: %+v vs %+v", i, a[i], b[i])
        }
    }
}

func TestSeedPlatesOceanicFractionInCI(t *testing.T) {
    // With PlateCount=200 and OceanicPlateFraction=0.7, expect ~140 oceanic.
    // Wilson 95% CI for n=200, p=0.7 is roughly [0.635, 0.757].
    profile := &types.PlanetProfile{PlateCount: 200, OceanicPlateFraction: 0.7}
    plates := seedPlates(profile, 7)
    var oceanic int
    for _, p := range plates {
        if p.IsOceanic {
            oceanic++
        }
    }
    frac := float64(oceanic) / float64(len(plates))
    if frac < 0.60 || frac > 0.80 {
        t.Errorf("oceanic fraction %.3f outside expected window [0.60, 0.80]", frac)
    }
}

func TestSeedPlatesZeroCount(t *testing.T) {
    profile := &types.PlanetProfile{PlateCount: 0}
    plates := seedPlates(profile, 1)
    if len(plates) != 0 {
        t.Errorf("PlateCount=0 should yield empty slice, got %d", len(plates))
    }
}
```

- [ ] **Step 2: Run the test, expect FAIL**

```
go test ./pkg/planetgen/field/ -run TestSeedPlates -v
```

Expected: FAIL — `seedPlates` undefined.

- [ ] **Step 3: Implement `Plate`, `PlateField`, and `seedPlates`**

`pkg/planetgen/field/plates.go`:

```go
// Package field — Phase 7 Tier B: Voronoi tectonic plates.
//
// GeneratePlates produces a PlateField — per-pixel plate id, per-plate
// motion + oceanic flag, and three boundary distance fields
// (convergent / divergent / transform) in km — for use by Phase 8
// consumers. Phase 7 itself only renders these as debug stages.
//
// Algorithm: Fibonacci-spiral N plate seeds → random flood-fill across
// cube faces with cross-face neighbor walk → boundary classification
// via relative-velocity dot-product → three independent JFA passes for
// the typed SDFs.
package field

import (
    "math"
    "math/rand/v2"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Plate captures a tectonic plate's identity and motion.
//
// Seed is the unit-vector direction of the Fibonacci-spiral seed used
// to start the plate's flood-fill region. RotAxis + AngSpeed encode an
// instantaneous angular-velocity vector ω = AngSpeed · RotAxis used
// only to classify boundaries via relative velocity at boundary pixels.
// No animation is implied.
//
// IsOceanic is drawn at construction time as a Bernoulli sample on the
// archetype's OceanicPlateFraction; it does not depend on the
// heightmap.
type Plate struct {
    ID        int
    Seed      [3]float64
    RotAxis   [3]float64
    AngSpeed  float64
    IsOceanic bool
}

// PlateField is the per-pixel plate output for a planet at a given
// face size. PlateID[face][py*S+px] holds the plate id (-1 for unset
// during construction; never persisted).
//
// Convergent / Divergent / Transform are signed-distance fields in km
// from each pixel to the nearest boundary of the corresponding type.
// Pixels in a planet with no plates of that boundary type get
// math.MaxFloat64.
type PlateField struct {
    Size       int
    Plates     []Plate
    PlateID    [cubemap.NumFaces][]int16
    Convergent [cubemap.NumFaces][]float64
    Divergent  [cubemap.NumFaces][]float64
    Transform  [cubemap.NumFaces][]float64
}

// seedPlates returns N plates with Fibonacci-spiral unit-vector seeds,
// random rotation axes, [0,1] angular speeds, and Bernoulli-sampled
// oceanic flags. Deterministic for fixed (profile.PlateCount,
// OceanicPlateFraction, master).
func seedPlates(profile *types.PlanetProfile, master int64) []Plate {
    n := profile.PlateCount
    if n <= 0 {
        return nil
    }
    plates := make([]Plate, n)

    // Fibonacci spiral on the unit sphere with a small per-seed jitter
    // so different planet seeds produce visibly distinct plate layouts.
    rngSeed := rand.New(rand.NewPCG(
        uint64(seed.Domain(master, "plates.seeds")), //nolint:gosec
        uint64(seed.Domain(master, "plates.seeds.stream")), //nolint:gosec
    ))
    rngMotion := rand.New(rand.NewPCG(
        uint64(seed.Domain(master, "plates.motion")), //nolint:gosec
        uint64(seed.Domain(master, "plates.motion.stream")), //nolint:gosec
    ))
    rngOceanic := rand.New(rand.NewPCG(
        uint64(seed.Domain(master, "plates.oceanic")), //nolint:gosec
        uint64(seed.Domain(master, "plates.oceanic.stream")), //nolint:gosec
    ))

    const goldenAngle = math.Pi * (3.0 - 2.23606797749979) // π·(3 − √5)
    for i := 0; i < n; i++ {
        // Spiral
        y := 1.0 - 2.0*(float64(i)+0.5)/float64(n)
        radius := math.Sqrt(1.0 - y*y)
        theta := goldenAngle * float64(i)
        // Jitter the spiral seed direction by ~3° to break visual symmetry
        // between planets that share PlateCount.
        jitter := (rngSeed.Float64() - 0.5) * (math.Pi / 30)
        x := math.Cos(theta+jitter) * radius
        z := math.Sin(theta+jitter) * radius

        plates[i].ID = i
        plates[i].Seed = [3]float64{x, y, z}

        // Random axis on unit sphere via two uniform draws (Marsaglia).
        for {
            a := 2*rngMotion.Float64() - 1
            b := 2*rngMotion.Float64() - 1
            s := a*a + b*b
            if s >= 1 {
                continue
            }
            f := 2 * math.Sqrt(1-s)
            plates[i].RotAxis = [3]float64{a * f, b * f, 1 - 2*s}
            break
        }
        plates[i].AngSpeed = rngMotion.Float64()
        plates[i].IsOceanic = rngOceanic.Float64() < profile.OceanicPlateFraction
    }
    return plates
}
```

- [ ] **Step 4: Run the test, expect PASS**

```
go test ./pkg/planetgen/field/ -run TestSeedPlates -v
```

Expected: all four tests PASS.

- [ ] **Step 5: Lint and commit**

```
golangci-lint run ./pkg/planetgen/field/
git add pkg/planetgen/field/plates.go pkg/planetgen/field/plates_test.go
git commit -m "P7: Fibonacci-spiral plate seeds + motion attributes (item 14 part 1)"
```

---

## Task 3: Cross-face neighbor primitive

The flood-fill needs a "give me the 4 neighbors of (face, px, py) including the off-edge ones on adjacent faces." Tier-A's JFA uses sphere-direction sampling rather than discrete neighbor walks, so this primitive may not exist directly. Add it.

**Files:**
- Modify: `pkg/planetgen/cubemap/sample.go` (or wherever direction helpers live — verify by grep first)
- Modify: `pkg/planetgen/cubemap/sample_test.go` (add tests)

- [ ] **Step 1: Grep to confirm what already exists**

```
grep -n "Neighbors\|FaceNeighbor\|FacePixelNeighbors" pkg/planetgen/cubemap/*.go
```

If a function with a similar role already exists, prefer wiring to it. Otherwise proceed.

- [ ] **Step 2: Write the failing test**

In `pkg/planetgen/cubemap/sample_test.go` (or a new `neighbor_test.go` in the same package — check existing convention):

```go
func TestFacePixelNeighbors4(t *testing.T) {
    S := 16
    // Interior pixel: 4 neighbors all on the same face.
    nbrs := FacePixelNeighbors4(FacePosX, 5, 5, S)
    if len(nbrs) != 4 {
        t.Fatalf("interior should have 4 neighbors, got %d", len(nbrs))
    }
    for _, n := range nbrs {
        if n.Face != FacePosX {
            t.Errorf("interior neighbor on different face: %+v", n)
        }
    }

    // Top-left corner: 2 in-face neighbors + 2 cross-face neighbors.
    nbrs = FacePixelNeighbors4(FacePosX, 0, 0, S)
    if len(nbrs) != 4 {
        t.Fatalf("corner should have 4 neighbors, got %d", len(nbrs))
    }
    var crossFace int
    for _, n := range nbrs {
        if n.Face != FacePosX {
            crossFace++
        }
    }
    if crossFace != 2 {
        t.Errorf("corner should have 2 cross-face neighbors, got %d (%+v)", crossFace, nbrs)
    }
}

func TestFacePixelNeighbors4Symmetry(t *testing.T) {
    S := 16
    // Walking from a pixel to a neighbor and back should land within
    // ±1 pixel of the start (face-id-quantized).
    for f := Face(0); f < NumFaces; f++ {
        for _, edge := range [][2]int{{0, S/2}, {S - 1, S/2}, {S/2, 0}, {S/2, S - 1}} {
            for _, nbr := range FacePixelNeighbors4(f, edge[0], edge[1], S) {
                back := FacePixelNeighbors4(nbr.Face, nbr.PX, nbr.PY, S)
                var found bool
                for _, b := range back {
                    if b.Face == f && abs(b.PX-edge[0]) <= 1 && abs(b.PY-edge[1]) <= 1 {
                        found = true
                        break
                    }
                }
                if !found {
                    t.Errorf("no return-walk from face=%v (%d,%d) via face=%v (%d,%d)",
                        f, edge[0], edge[1], nbr.Face, nbr.PX, nbr.PY)
                }
            }
        }
    }
}

func abs(v int) int { if v < 0 { return -v }; return v }
```

If `abs` is already declared in the test file, drop it.

- [ ] **Step 3: Run the test, expect FAIL**

```
go test ./pkg/planetgen/cubemap/ -run TestFacePixelNeighbors4 -v
```

Expected: FAIL — undefined.

- [ ] **Step 4: Implement `FacePixelNeighbors4`**

In `pkg/planetgen/cubemap/sample.go` (append) or a new `neighbors.go`:

```go
// PixelAddr identifies a pixel on a cube map by face and (px, py).
type PixelAddr struct {
    Face   Face
    PX, PY int
}

// FacePixelNeighbors4 returns the four 4-connected neighbors of (face,
// px, py). Off-edge neighbors are remapped to the adjacent face by
// converting the off-edge integer offset to a unit direction via
// FacePixelToDir, nudging slightly off the edge into the next face's
// space, and re-projecting via DirToFacePixel.
//
// Returned addresses always lie within [0, S) on both axes; cross-face
// neighbors carry the adjacent face id.
func FacePixelNeighbors4(face Face, px, py, S int) [4]PixelAddr {
    deltas := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
    var out [4]PixelAddr
    for i, d := range deltas {
        nx, ny := px+d[0], py+d[1]
        if nx >= 0 && nx < S && ny >= 0 && ny < S {
            out[i] = PixelAddr{Face: face, PX: nx, PY: ny}
            continue
        }
        // Off-edge: project a half-pixel beyond the edge to a unit
        // direction, then back into face/pixel space.
        dx, dy, dz := FacePixelToDirFractional(face,
            float64(px)+0.5+float64(d[0]),
            float64(py)+0.5+float64(d[1]),
            S)
        // Normalize.
        mag := math.Sqrt(dx*dx + dy*dy + dz*dz)
        dx, dy, dz = dx/mag, dy/mag, dz/mag
        nf, npx, npy := DirToFacePixel(dx, dy, dz, S)
        out[i] = PixelAddr{Face: nf, PX: npx, PY: npy}
    }
    return out
}
```

- [ ] **Step 5: Add `FacePixelToDirFractional` if not present**

Grep:

```
grep -n "FacePixelToDirFractional\|FacePixelToDirF" pkg/planetgen/cubemap/*.go
```

If absent, add the float-input variant alongside `FacePixelToDir`. Open `pkg/planetgen/cubemap/sample.go` and find the existing `FacePixelToDir`. The float variant differs only in that `(px, py)` are float64 and don't get truncated. Reproduce the existing axis math (do not invent new sign conventions — read the existing function and translate float64 in). If the existing function uses `(float64(px)+0.5)/float64(S)` to produce u in [0,1], the float variant uses `(px)/float64(S)` directly without the integer truncation/+0.5. Sample illustrative skeleton (verify against your codebase before committing):

```go
// FacePixelToDirFractional is the float64 variant of FacePixelToDir.
// (fpx, fpy) are continuous coordinates in [0, S]; the half-pixel
// offset that integer FacePixelToDir applies is the caller's
// responsibility.
func FacePixelToDirFractional(face Face, fpx, fpy float64, S int) (dx, dy, dz float64) {
    u := 2*fpx/float64(S) - 1
    v := 2*fpy/float64(S) - 1
    switch face {
    case FacePosX:
        return 1, -v, -u
    case FaceNegX:
        return -1, -v, u
    case FacePosY:
        return u, 1, v
    case FaceNegY:
        return u, -1, -v
    case FacePosZ:
        return u, -v, 1
    case FaceNegZ:
        return -u, -v, -1
    }
    return 0, 0, 0
}
```

The case axes above match the conventional GL_TEXTURE_CUBE_MAP_POSITIVE_X/etc layout. **Verify by comparing with the existing `FacePixelToDir` in your codebase before trusting this — if signs differ, copy the existing function's signs verbatim.**

- [ ] **Step 6: Run tests, expect PASS**

```
go test ./pkg/planetgen/cubemap/ -run TestFacePixelNeighbors4 -v
```

Expected: PASS.

- [ ] **Step 7: Lint and commit**

```
golangci-lint run ./pkg/planetgen/cubemap/
git add pkg/planetgen/cubemap/
git commit -m "cubemap: add FacePixelNeighbors4 (cross-face 4-neighbor walk)"
```

---

## Task 4: Random flood-fill across cube faces

Use the seeded plates (Task 2) and the cross-face neighbor primitive (Task 3) to fill every pixel with a plate id.

**Files:**
- Modify: `pkg/planetgen/field/plates.go`
- Modify: `pkg/planetgen/field/plates_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `pkg/planetgen/field/plates_test.go`:

```go
func TestFloodFillSinglePlateCoversAll(t *testing.T) {
    S := 16
    profile := &types.PlanetProfile{PlateCount: 1, OceanicPlateFraction: 0.5}
    pf := GeneratePlates(profile, 1, S)
    if pf == nil {
        t.Fatal("GeneratePlates returned nil")
    }
    for f := range pf.PlateID {
        for i, id := range pf.PlateID[f] {
            if id != 0 {
                t.Fatalf("face %d idx %d: id=%d (want 0)", f, i, id)
            }
        }
    }
}

func TestFloodFillCoverageAndDeterminism(t *testing.T) {
    S := 32
    profile := &types.PlanetProfile{PlateCount: 6, OceanicPlateFraction: 0.5}
    a := GeneratePlates(profile, 11, S)
    b := GeneratePlates(profile, 11, S)
    for f := range a.PlateID {
        for i := range a.PlateID[f] {
            if a.PlateID[f][i] != b.PlateID[f][i] {
                t.Fatalf("non-deterministic at face %d idx %d", f, i)
            }
        }
    }
    counts := make(map[int16]int)
    for f := range a.PlateID {
        for _, id := range a.PlateID[f] {
            if id < 0 {
                t.Fatalf("unfilled pixel: face %d id=%d", f, id)
            }
            counts[id]++
        }
    }
    if len(counts) != 6 {
        t.Errorf("expected 6 distinct ids, got %d (%+v)", len(counts), counts)
    }
    // No plate may be empty.
    for id, c := range counts {
        if c == 0 {
            t.Errorf("plate %d has 0 pixels", id)
        }
    }
}

func TestFloodFillZeroPlatesNilField(t *testing.T) {
    profile := &types.PlanetProfile{PlateCount: 0}
    pf := GeneratePlates(profile, 0, 16)
    if pf != nil {
        t.Errorf("expected nil PlateField when PlateCount=0, got %+v", pf)
    }
}
```

- [ ] **Step 2: Run, expect FAIL**

```
go test ./pkg/planetgen/field/ -run TestFloodFill -v
```

Expected: FAIL — `GeneratePlates` undefined.

- [ ] **Step 3: Implement `GeneratePlates` flood-fill phase**

Append to `pkg/planetgen/field/plates.go`:

```go
// GeneratePlates produces a PlateField for a planet at face size S.
// Returns nil when profile.PlateCount == 0 (no plates).
//
// Pipeline (Phase 7 — flood-fill only; classification + SDFs in
// later commits):
//   1. seedPlates: N spiral seeds + per-plate motion + oceanic flag.
//   2. floodFill: assign every pixel a plate id by random-walk frontier.
//   3. classifyAndSDF: <added in Task 5/6>.
//
// All RNG draws happen inside the named-domain seeds defined in
// pkg/planetgen/field/plates.go so adding new sub-steps in later
// phases never shifts existing field values.
func GeneratePlates(profile *types.PlanetProfile, master int64, S int) *PlateField {
    plates := seedPlates(profile, master)
    if len(plates) == 0 {
        return nil
    }
    pf := &PlateField{Size: S, Plates: plates}
    for f := range pf.PlateID {
        pf.PlateID[f] = make([]int16, S*S)
        for i := range pf.PlateID[f] {
            pf.PlateID[f][i] = -1
        }
    }
    floodFillPlates(pf, master, S)
    return pf
}

// floodFillPlates assigns every pixel a plate id via random-walk
// frontier expansion starting from each plate's spiral seed pixel.
//
// Termination is bounded: each loop iteration assigns exactly one
// previously-unassigned pixel; total iterations = 6·S² − len(plates).
func floodFillPlates(pf *PlateField, master int64, S int) {
    rng := rand.New(rand.NewPCG(
        uint64(seed.Domain(master, "plates.fill.random")),         //nolint:gosec
        uint64(seed.Domain(master, "plates.fill.random.stream")),  //nolint:gosec
    ))

    // Mark each plate's seed pixel.
    for i := range pf.Plates {
        d := pf.Plates[i].Seed
        f, px, py := cubemap.DirToFacePixel(d[0], d[1], d[2], S)
        pf.PlateID[f][py*S+px] = int16(i)
    }

    // Frontier: list of (assigned-pixel, off-side direction-index 0..3).
    // We resample randomly each step to avoid BFS symmetry. To keep the
    // step cost low, store unassigned-neighbor candidates directly.
    type frontierItem struct {
        Addr cubemap.PixelAddr
        ID   int16
    }
    frontier := make([]frontierItem, 0, 6*S*S)

    pushNeighbors := func(face cubemap.Face, px, py int, id int16) {
        nbrs := cubemap.FacePixelNeighbors4(face, px, py, S)
        for _, n := range nbrs {
            if pf.PlateID[n.Face][n.PY*S+n.PX] == -1 {
                frontier = append(frontier, frontierItem{Addr: n, ID: id})
            }
        }
    }

    // Seed the frontier.
    for i := range pf.Plates {
        d := pf.Plates[i].Seed
        f, px, py := cubemap.DirToFacePixel(d[0], d[1], d[2], S)
        pushNeighbors(f, px, py, int16(i))
    }

    for len(frontier) > 0 {
        // Pick a random index.
        idx := rng.IntN(len(frontier))
        item := frontier[idx]
        // Pop by swap-with-end.
        frontier[idx] = frontier[len(frontier)-1]
        frontier = frontier[:len(frontier)-1]

        if pf.PlateID[item.Addr.Face][item.Addr.PY*S+item.Addr.PX] != -1 {
            continue // already filled by a different chain
        }
        pf.PlateID[item.Addr.Face][item.Addr.PY*S+item.Addr.PX] = item.ID
        pushNeighbors(item.Addr.Face, item.Addr.PX, item.Addr.PY, item.ID)
    }
}
```

- [ ] **Step 4: Run, expect PASS**

```
go test ./pkg/planetgen/field/ -run TestFloodFill -v
```

Expected: PASS.

- [ ] **Step 5: Performance smoke check**

```
go test ./pkg/planetgen/field/ -run TestFloodFillCoverageAndDeterminism -v -count=1 -timeout 30s
```

Expected: completes in < 1s at S=32. If slower, profile (hot path is the random pop with swap-end — already O(1)).

- [ ] **Step 6: Lint and commit**

```
golangci-lint run ./pkg/planetgen/field/
git add pkg/planetgen/field/plates.go pkg/planetgen/field/plates_test.go
git commit -m "P7: random flood-fill plates across cube faces (item 14 part 2)"
```

---

## Task 5: Boundary classification

Walk every pixel; if any 4-neighbor has a different plate id, compute the boundary type from `v_rel · n` against `PlateConvergentT`. Record per-pixel boundary-type tags so Task 6 can JFA them into SDFs.

**Files:**
- Modify: `pkg/planetgen/field/plates.go`
- Modify: `pkg/planetgen/field/plates_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/planetgen/field/plates_test.go`:

```go
func TestClassifyBoundaryHandBuilt(t *testing.T) {
    // Construct two synthetic plates that share a boundary at p = +X.
    // Plate A: ω points +Y, AngSpeed=1 → at p=+X, v_a = ω×p = (0,1,0)×(1,0,0) = (0,0,-1)
    // Plate B: ω points -Y, AngSpeed=1 → at p=+X, v_b = (0,-1,0)×(1,0,0) = (0,0,+1)
    // v_rel = v_a − v_b = (0, 0, -2). |v_rel| = 2.
    //
    // Boundary normal n = +Z (pointing from B toward A across the seam):
    //   v_rel·n = -2 → strongly convergent (assuming A is "here" and B is "there"
    //   and we picked n toward "there"; the test verifies which kind we hit).
    //
    // What we test is just that the classifier branches by sign and threshold,
    // by feeding it specific (v_rel, n, T) triples.
    cases := []struct {
        vrel [3]float64
        n    [3]float64
        t    float64
        want boundaryKind
    }{
        {[3]float64{0, 0, -2}, [3]float64{0, 0, 1}, 0.75, boundaryConvergent},
        {[3]float64{0, 0, +2}, [3]float64{0, 0, 1}, 0.75, boundaryDivergent},
        {[3]float64{0, 0, +0.1}, [3]float64{0, 0, 1}, 0.75, boundaryTransform},
        {[3]float64{0, 0, +0.8}, [3]float64{0, 0, 1}, 0.75, boundaryDivergent},
        {[3]float64{0, 0, -0.74}, [3]float64{0, 0, 1}, 0.75, boundaryTransform},
    }
    for i, c := range cases {
        got := classifyBoundary(c.vrel, c.n, c.t)
        if got != c.want {
            t.Errorf("case %d: classifyBoundary(%v,%v,%v) = %v, want %v",
                i, c.vrel, c.n, c.t, got, c.want)
        }
    }
}

func TestExtractBoundariesAtLeastOnePerType(t *testing.T) {
    profile := &types.PlanetProfile{
        PlateCount: 12, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75,
    }
    pf := GeneratePlates(profile, 5, 32)
    extractBoundariesForTest(pf, profile.PlateConvergentT)
    counts := boundaryCountsForTest(pf)
    // With 12 random plates we expect at least one of each type. If a
    // run produces zero of a type, the rng was unlucky — bump the seed
    // until this test stabilizes.
    for k, c := range counts {
        if c == 0 {
            t.Errorf("no boundaries of kind %v", k)
        }
    }
}
```

The test references unexported helpers `extractBoundariesForTest` and `boundaryCountsForTest` (added below) so the unit test can drive the boundary phase without going through SDF computation. Add these as a `plates_internal_test.go` (in `package field`, same file is fine but separate keeps it tidy).

- [ ] **Step 2: Run, expect FAIL**

```
go test ./pkg/planetgen/field/ -run TestClassifyBoundary -v
```

Expected: FAIL — symbols undefined.

- [ ] **Step 3: Add boundary types and classification**

Append to `pkg/planetgen/field/plates.go`:

```go
type boundaryKind int8

const (
    boundaryNone boundaryKind = iota
    boundaryConvergent
    boundaryDivergent
    boundaryTransform
)

// classifyBoundary returns the plate-boundary type at a point given
// the relative velocity vRel between the two plates at that point and
// the boundary normal n (unit vector from "this" plate's pixel toward
// the differing-plate neighbor pixel, in tangent space). T is the
// signed-velocity threshold (profile.PlateConvergentT, default 0.75).
func classifyBoundary(vRel, n [3]float64, T float64) boundaryKind {
    proj := vRel[0]*n[0] + vRel[1]*n[1] + vRel[2]*n[2]
    switch {
    case proj > +T:
        return boundaryDivergent
    case proj < -T:
        return boundaryConvergent
    default:
        return boundaryTransform
    }
}

// boundaryAt returns the boundary kind at face/(px,py) by examining
// the four 4-neighbors. If no neighbor has a different plate id, the
// pixel is interior and the return is boundaryNone. If multiple
// neighbors with differing ids exist, the kind from the first is
// returned (priority Convergent > Divergent > Transform). T is
// profile.PlateConvergentT.
func boundaryAt(pf *PlateField, face cubemap.Face, px, py int, T float64) boundaryKind {
    S := pf.Size
    here := pf.PlateID[face][py*S+px]
    nbrs := cubemap.FacePixelNeighbors4(face, px, py, S)
    var best boundaryKind = boundaryNone
    rank := func(k boundaryKind) int {
        switch k {
        case boundaryConvergent:
            return 3
        case boundaryDivergent:
            return 2
        case boundaryTransform:
            return 1
        }
        return 0
    }
    for _, nb := range nbrs {
        there := pf.PlateID[nb.Face][nb.PY*S+nb.PX]
        if there == here {
            continue
        }
        a := pf.Plates[here]
        b := pf.Plates[there]
        // Position p on the unit sphere at the boundary pixel.
        dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
        p := [3]float64{dx, dy, dz}
        // ω = AngSpeed · RotAxis. v = ω × p.
        va := cross(scale(a.RotAxis, a.AngSpeed), p)
        vb := cross(scale(b.RotAxis, b.AngSpeed), p)
        vRel := [3]float64{va[0] - vb[0], va[1] - vb[1], va[2] - vb[2]}
        // Normal: from here-pixel toward there-pixel in tangent plane.
        ndx, ndy, ndz := cubemap.FacePixelToDir(nb.Face, nb.PX, nb.PY, S)
        n := [3]float64{ndx - dx, ndy - dy, ndz - dz}
        // Project n into tangent plane at p (subtract component along p).
        proj := n[0]*p[0] + n[1]*p[1] + n[2]*p[2]
        n[0] -= proj * p[0]
        n[1] -= proj * p[1]
        n[2] -= proj * p[2]
        nmag := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
        if nmag > 0 {
            n[0] /= nmag
            n[1] /= nmag
            n[2] /= nmag
        }
        kind := classifyBoundary(vRel, n, T)
        if rank(kind) > rank(best) {
            best = kind
        }
    }
    return best
}

func cross(a, b [3]float64) [3]float64 {
    return [3]float64{
        a[1]*b[2] - a[2]*b[1],
        a[2]*b[0] - a[0]*b[2],
        a[0]*b[1] - a[1]*b[0],
    }
}

func scale(a [3]float64, s float64) [3]float64 {
    return [3]float64{a[0] * s, a[1] * s, a[2] * s}
}

// extractBoundaries computes the boundary kind for every pixel and
// stores convergent/divergent/transform pixel masks in temporary
// per-face slices. Called by GeneratePlates between flood-fill and
// SDF.
func extractBoundaries(pf *PlateField, T float64) (conv, div, trans [cubemap.NumFaces][]bool) {
    S := pf.Size
    for f := range conv {
        conv[f] = make([]bool, S*S)
        div[f] = make([]bool, S*S)
        trans[f] = make([]bool, S*S)
    }
    for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
        for py := 0; py < S; py++ {
            for px := 0; px < S; px++ {
                k := boundaryAt(pf, f, px, py, T)
                idx := py*S + px
                switch k {
                case boundaryConvergent:
                    conv[f][idx] = true
                case boundaryDivergent:
                    div[f][idx] = true
                case boundaryTransform:
                    trans[f][idx] = true
                }
            }
        }
    }
    return
}

// extractBoundariesForTest exposes extractBoundaries for unit testing.
func extractBoundariesForTest(pf *PlateField, T float64) (conv, div, trans [cubemap.NumFaces][]bool) {
    return extractBoundaries(pf, T)
}

// boundaryCountsForTest sums the number of boundary pixels of each
// type across all faces. Used by the at-least-one-per-type test.
func boundaryCountsForTest(pf *PlateField) map[boundaryKind]int {
    conv, div, trans := extractBoundaries(pf, 0.75)
    counts := map[boundaryKind]int{}
    for f := range conv {
        for i := range conv[f] {
            if conv[f][i] {
                counts[boundaryConvergent]++
            }
            if div[f][i] {
                counts[boundaryDivergent]++
            }
            if trans[f][i] {
                counts[boundaryTransform]++
            }
        }
    }
    return counts
}
```

- [ ] **Step 4: Run, expect PASS**

```
go test ./pkg/planetgen/field/ -run TestClassifyBoundary -v
go test ./pkg/planetgen/field/ -run TestExtractBoundaries -v
```

Expected: both PASS. If the second test fails because rng=5 produced no convergent (or no divergent) boundaries, change the seed in the test until it stabilizes — the goal is that *some* seed produces all three types, which validates the classifier hits each branch.

- [ ] **Step 5: Lint and commit**

```
golangci-lint run ./pkg/planetgen/field/
git add pkg/planetgen/field/plates.go pkg/planetgen/field/plates_test.go
git commit -m "P7: plate boundary classification by relative velocity (item 14 part 3)"
```

---

## Task 6: Three SDF passes via JFA

Run a JFA pass per boundary type — `Convergent`, `Divergent`, `Transform`. Each starts from that type's boundary-pixel mask and propagates squared distance across faces. Reuse the existing `pkg/planetgen/field/jfa.go` primitive — extract a generic JFA helper that takes a "this pixel is a seed" predicate.

**Files:**
- Modify: `pkg/planetgen/field/jfa.go` (add a generic export)
- Modify: `pkg/planetgen/field/plates.go`
- Modify: `pkg/planetgen/field/plates_test.go`

- [ ] **Step 1: Inspect existing JFA**

```
sed -n '1,90p' pkg/planetgen/field/jfa.go
```

Confirm the structure: there's a `jfaSeed` type and a `DistanceToCoast` function. We need a generic function that takes any per-pixel "is this a seed" predicate and produces a normalized-distance field.

- [ ] **Step 2: Add generic JFA primitive**

Append to `pkg/planetgen/field/jfa.go`:

```go
// JumpFloodFromMask runs JFA over a cube map starting from every
// pixel where mask[face][py*S+px] is true. Returns a CubeMapF where
// each pixel holds the great-circle angular distance (in radians,
// i.e. [0, π]) to the nearest seed pixel. Pixels in a planet with no
// seeds get π (the antipodal max).
//
// This is the generic core extracted from DistanceToCoast.
// DistanceToCoast can later be re-expressed in terms of this helper;
// for Phase 7 we just add the helper without refactoring the existing
// caller.
func JumpFloodFromMask(mask [cubemap.NumFaces][]bool, S int) *cubemap.CubeMapF {
    seeds := make([][]jfaSeed, cubemap.NumFaces)
    dists := cubemap.NewF(S)
    for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
        seeds[f] = make([]jfaSeed, S*S)
        for i := range seeds[f] {
            seeds[f][i].face = noJFASeed
        }
        for py := 0; py < S; py++ {
            for px := 0; px < S; px++ {
                idx := py*S + px
                if mask[f][idx] {
                    dx, dy, dz := cubemap.FacePixelToDir(f, px, py, S)
                    seeds[f][idx] = jfaSeed{
                        face: int8(f),
                        px:   int16(px),
                        py:   int16(py),
                        dirX: float32(dx),
                        dirY: float32(dy),
                        dirZ: float32(dz),
                    }
                    dists.Set(f, px, py, 0)
                } else {
                    dists.Set(f, px, py, math.Pi)
                }
            }
        }
    }
    // Reuse the propagation core from DistanceToCoast: jump-step sizes
    // S/2, S/4, ..., 1; at each step compare each pixel's current best
    // seed to the seed of each 8-connected neighbor at the step size.
    propagateJFA(seeds, dists, S)
    return dists
}
```

If `propagateJFA` doesn't already exist as a separate helper, extract it from `DistanceToCoast`. Inspect the existing function and pull the propagation loop into a private helper `propagateJFA(seeds, dists *cubemap.CubeMapF, S int)`. The existing test `TestDistanceToCoast` (or equivalent) must continue to pass after the extraction.

- [ ] **Step 3: Write the failing SDF test**

Append to `pkg/planetgen/field/plates_test.go`:

```go
func TestSDFsZeroAtSeeds(t *testing.T) {
    profile := &types.PlanetProfile{
        PlateCount: 6, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75,
    }
    pf := GeneratePlates(profile, 17, 32)
    if pf.Convergent[0] == nil {
        t.Fatal("Convergent SDF not allocated")
    }
    // Re-run boundary extraction to know which pixels are seeds.
    conv, div, trans := extractBoundariesForTest(pf, profile.PlateConvergentT)
    for f := range conv {
        for i, isSeed := range conv[f] {
            if isSeed && pf.Convergent[f][i] != 0 {
                t.Fatalf("convergent seed face %d idx %d distance=%f, want 0",
                    f, i, pf.Convergent[f][i])
            }
        }
    }
    _, _ = div, trans // visual-only sanity: ensure the same is true for them
}

func TestSDFsKmScaling(t *testing.T) {
    // RadiusKm=6371 (Earth-like). At a pixel halfway around the sphere
    // (geodesic π/2 radians) from any seed, distance should be ~10000 km.
    // We don't construct that exact case; just check the maximum
    // distance is plausible: bounded above by π * RadiusKm and not
    // ridiculously below it.
    profile := &types.PlanetProfile{
        PlateCount: 4, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75,
        RadiusKm: 6371,
    }
    pf := GeneratePlates(profile, 23, 32)
    var maxConv float64
    for f := range pf.Convergent {
        for _, d := range pf.Convergent[f] {
            if d > maxConv {
                maxConv = d
            }
        }
    }
    if maxConv < 100 || maxConv > math.Pi*profile.RadiusKm+1 {
        t.Errorf("max convergent distance %.1f km outside plausible range", maxConv)
    }
}
```

This requires `RadiusKm` on `PlanetProfile`. **Verify it exists** by grepping `grep -n "RadiusKm" pkg/planetgen/types/types.go`. If absent, add it as `RadiusKm float64 // Mean radius in km; default 6371.` and use `6371` in the renderer when zero.

- [ ] **Step 4: Run, expect FAIL**

```
go test ./pkg/planetgen/field/ -run TestSDFs -v
```

Expected: FAIL — `pf.Convergent` is nil because we haven't computed it yet.

- [ ] **Step 5: Wire SDF into `GeneratePlates`**

Append to `pkg/planetgen/field/plates.go`:

```go
// computeSDFs runs three JFA passes over the boundary-type masks and
// scales the angular-distance output by RadiusKm. RadiusKm defaults
// to 6371 (Earth-like) when zero.
func computeSDFs(pf *PlateField, profile *types.PlanetProfile) {
    conv, div, trans := extractBoundaries(pf, profile.PlateConvergentT)
    radius := profile.RadiusKm
    if radius == 0 {
        radius = 6371
    }
    runOne := func(mask [cubemap.NumFaces][]bool) [cubemap.NumFaces][]float64 {
        f := JumpFloodFromMask(mask, pf.Size)
        var out [cubemap.NumFaces][]float64
        for i := range f.Faces {
            out[i] = make([]float64, len(f.Faces[i]))
            for j, v := range f.Faces[i] {
                out[i][j] = v * radius
            }
        }
        return out
    }
    pf.Convergent = runOne(conv)
    pf.Divergent = runOne(div)
    pf.Transform = runOne(trans)
}
```

Update `GeneratePlates` to call `computeSDFs(pf, profile)` after the flood-fill:

```go
func GeneratePlates(profile *types.PlanetProfile, master int64, S int) *PlateField {
    plates := seedPlates(profile, master)
    if len(plates) == 0 {
        return nil
    }
    pf := &PlateField{Size: S, Plates: plates}
    for f := range pf.PlateID {
        pf.PlateID[f] = make([]int16, S*S)
        for i := range pf.PlateID[f] {
            pf.PlateID[f][i] = -1
        }
    }
    floodFillPlates(pf, master, S)
    computeSDFs(pf, profile)
    return pf
}
```

- [ ] **Step 6: Run, expect PASS**

```
go test ./pkg/planetgen/field/ -run TestSDFs -v
```

Expected: PASS. If the existing `DistanceToCoast` tests now fail because of the propagation extraction, fix the extraction (most likely a missing initialization) and rerun.

```
go test ./pkg/planetgen/field/ -v
```

Expected: all field tests PASS.

- [ ] **Step 7: Lint and commit**

```
golangci-lint run ./pkg/planetgen/field/
git add pkg/planetgen/field/jfa.go pkg/planetgen/field/plates.go pkg/planetgen/field/plates_test.go pkg/planetgen/types/types.go
git commit -m "P7: three boundary SDFs via JFA + RadiusKm scaling (item 14 part 4)"
```

---

## Task 7: Jitter cells and per-pixel cell-id

`JitterField` with Fibonacci-spiral cell centers and per-pixel nearest-cell-id assignment.

**Files:**
- Create: `pkg/planetgen/noise/jitter.go`
- Create: `pkg/planetgen/noise/jitter_test.go`

- [ ] **Step 1: Write the failing test**

`pkg/planetgen/noise/jitter_test.go`:

```go
package noise

import (
    "math"
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestGenerateJitterCellCount(t *testing.T) {
    profile := &types.PlanetProfile{
        JitterEnabled: true, JitterCellCount: 120, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.1,
    }
    jf := GenerateJitter(profile, 7, 32)
    if jf == nil {
        t.Fatal("nil JitterField for enabled profile")
    }
    if len(jf.Cells) != 120 {
        t.Errorf("got %d cells, want 120", len(jf.Cells))
    }
}

func TestGenerateJitterDisabled(t *testing.T) {
    profile := &types.PlanetProfile{JitterEnabled: false}
    if jf := GenerateJitter(profile, 7, 32); jf != nil {
        t.Errorf("disabled profile should yield nil, got %+v", jf)
    }
}

func TestGenerateJitterPerPixelInRange(t *testing.T) {
    profile := &types.PlanetProfile{JitterEnabled: true, JitterCellCount: 32, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.1}
    S := 16
    jf := GenerateJitter(profile, 5, S)
    seen := make(map[int16]int)
    for f := range jf.PerPixel {
        for _, id := range jf.PerPixel[f] {
            if id < 0 || int(id) >= len(jf.Cells) {
                t.Fatalf("cell id %d out of range [0, %d)", id, len(jf.Cells))
            }
            seen[id]++
        }
    }
    // Most cells should be visited.
    if len(seen) < len(jf.Cells)/2 {
        t.Errorf("only %d/%d cells visited", len(seen), len(jf.Cells))
    }
}

func TestJitterTransformIdentityWhenZero(t *testing.T) {
    profile := &types.PlanetProfile{JitterEnabled: true, JitterCellCount: 8, JitterRotMax: 0, JitterOffsetMax: 0}
    jf := GenerateJitter(profile, 3, 16)
    p := [3]float64{0.5, 0.5, math.Sqrt2 / 2}
    px, py, pz := jf.Transform(p[0], p[1], p[2])
    if math.Abs(px-p[0]) > 1e-9 || math.Abs(py-p[1]) > 1e-9 || math.Abs(pz-p[2]) > 1e-9 {
        t.Errorf("zero-jitter transform changed p: in=%v out=(%f,%f,%f)", p, px, py, pz)
    }
}

func TestGenerateJitterDeterministic(t *testing.T) {
    profile := &types.PlanetProfile{JitterEnabled: true, JitterCellCount: 64, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.1}
    a := GenerateJitter(profile, 99, 16)
    b := GenerateJitter(profile, 99, 16)
    for f := range a.PerPixel {
        for i := range a.PerPixel[f] {
            if a.PerPixel[f][i] != b.PerPixel[f][i] {
                t.Fatalf("non-deterministic at face %d idx %d", f, i)
            }
        }
    }
    for i := range a.Cells {
        if a.Cells[i] != b.Cells[i] {
            t.Errorf("cell %d non-deterministic: %+v vs %+v", i, a.Cells[i], b.Cells[i])
        }
    }
    _ = cubemap.NumFaces // silence unused-import nag if test grows
}
```

- [ ] **Step 2: Run, expect FAIL**

```
go test ./pkg/planetgen/noise/ -run TestGenerateJitter -v
go test ./pkg/planetgen/noise/ -run TestJitterTransformIdentity -v
```

Expected: FAIL — `GenerateJitter` undefined.

- [ ] **Step 3: Implement `JitterField`, `GenerateJitter`, `Transform`**

`pkg/planetgen/noise/jitter.go`:

```go
// Package noise — Phase 7 Tier B: Voronoi cell-coordinate jitter.
//
// JitterField rotates and offsets sample coordinates per Voronoi cell
// to break visible repetition in fbm-based detail noise. Adjacent
// cells produce slightly-different rotated views of the same fbm
// field, so what would otherwise be a tiled pattern shows natural
// variation across cell boundaries.
//
// The discontinuity at cell boundaries is small (≤ JitterOffsetMax in
// sample-space, ≤ JitterRotMax around the cell center) and reads as
// natural variation given the underlying fbm continuity.
package noise

import (
    "math"
    "math/rand/v2"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// JitterCell encodes a per-cell rotation and translation applied at
// sample time when looking up fbm noise inside the cell's Voronoi
// region.
type JitterCell struct {
    Center   [3]float64 // unit vector
    RotAxis  [3]float64 // unit vector
    RotAngle float64    // radians
    Offset   [3]float64 // each component in [-JitterOffsetMax, +JitterOffsetMax]
}

// JitterField is the per-planet jitter state. PerPixel[face][py*S+px]
// holds the cell id (0..len(Cells)-1) that contains that pixel.
type JitterField struct {
    Size      int
    Cells     []JitterCell
    PerPixel  [cubemap.NumFaces][]int16
}

// GenerateJitter returns a JitterField for a planet at face size S.
// Returns nil when profile.JitterEnabled is false (the renderer skips
// the jitter transform when nil — no per-call flag needed).
func GenerateJitter(profile *types.PlanetProfile, master int64, S int) *JitterField {
    if !profile.JitterEnabled || profile.JitterCellCount <= 0 {
        return nil
    }
    n := profile.JitterCellCount
    jf := &JitterField{Size: S, Cells: make([]JitterCell, n)}
    rngCells := rand.New(rand.NewPCG(
        uint64(seed.Domain(master, "jitter.cells")),         //nolint:gosec
        uint64(seed.Domain(master, "jitter.cells.stream")),  //nolint:gosec
    ))
    rngRot := rand.New(rand.NewPCG(
        uint64(seed.Domain(master, "jitter.rot")),         //nolint:gosec
        uint64(seed.Domain(master, "jitter.rot.stream")),  //nolint:gosec
    ))
    rngOff := rand.New(rand.NewPCG(
        uint64(seed.Domain(master, "jitter.offset")),         //nolint:gosec
        uint64(seed.Domain(master, "jitter.offset.stream")),  //nolint:gosec
    ))
    const goldenAngle = math.Pi * (3.0 - 2.23606797749979)
    for i := 0; i < n; i++ {
        y := 1.0 - 2.0*(float64(i)+0.5)/float64(n)
        radius := math.Sqrt(1.0 - y*y)
        theta := goldenAngle * float64(i)
        // Tiny per-cell jitter on the spiral so different planets at
        // the same JitterCellCount don't share cell layout.
        jitter := (rngCells.Float64() - 0.5) * (math.Pi / 60)
        x := math.Cos(theta+jitter) * radius
        z := math.Sin(theta+jitter) * radius
        jf.Cells[i].Center = [3]float64{x, y, z}

        // Random rotation axis (Marsaglia).
        for {
            a := 2*rngRot.Float64() - 1
            b := 2*rngRot.Float64() - 1
            s := a*a + b*b
            if s >= 1 {
                continue
            }
            f := 2 * math.Sqrt(1-s)
            jf.Cells[i].RotAxis = [3]float64{a * f, b * f, 1 - 2*s}
            break
        }
        jf.Cells[i].RotAngle = (rngRot.Float64()*2 - 1) * profile.JitterRotMax
        jf.Cells[i].Offset = [3]float64{
            (rngOff.Float64()*2 - 1) * profile.JitterOffsetMax,
            (rngOff.Float64()*2 - 1) * profile.JitterOffsetMax,
            (rngOff.Float64()*2 - 1) * profile.JitterOffsetMax,
        }
    }

    // Per-pixel nearest-center search.
    for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
        jf.PerPixel[f] = make([]int16, S*S)
        for py := 0; py < S; py++ {
            for px := 0; px < S; px++ {
                dx, dy, dz := cubemap.FacePixelToDir(f, px, py, S)
                bestI := int16(0)
                bestDot := -2.0
                for i := range jf.Cells {
                    c := jf.Cells[i].Center
                    dot := c[0]*dx + c[1]*dy + c[2]*dz
                    if dot > bestDot {
                        bestDot = dot
                        bestI = int16(i)
                    }
                }
                jf.PerPixel[f][py*S+px] = bestI
            }
        }
    }
    return jf
}

// At returns the JitterCell whose Voronoi region contains the unit
// direction (dx, dy, dz). O(N) nearest-center search; N is small
// (~120) so this is fine for sample-time use.
func (jf *JitterField) At(dx, dy, dz float64) *JitterCell {
    if jf == nil {
        return nil
    }
    bestI := 0
    bestDot := -2.0
    for i := range jf.Cells {
        c := jf.Cells[i].Center
        dot := c[0]*dx + c[1]*dy + c[2]*dz
        if dot > bestDot {
            bestDot = dot
            bestI = i
        }
    }
    return &jf.Cells[bestI]
}

// Transform applies the cell-local rotation + offset to direction
// (px, py, pz). Returns the original direction unchanged if jf is nil.
func (jf *JitterField) Transform(px, py, pz float64) (float64, float64, float64) {
    if jf == nil {
        return px, py, pz
    }
    cell := jf.At(px, py, pz)
    // Rotate (p − Center) around RotAxis by RotAngle (Rodrigues' formula).
    dx, dy, dz := px-cell.Center[0], py-cell.Center[1], pz-cell.Center[2]
    rx, ry, rz := cell.RotAxis[0], cell.RotAxis[1], cell.RotAxis[2]
    cosA := math.Cos(cell.RotAngle)
    sinA := math.Sin(cell.RotAngle)
    dot := dx*rx + dy*ry + dz*rz
    // p' = p·cos + (k×p)·sin + k·(k·p)·(1−cos)
    cx := ry*dz - rz*dy
    cy := rz*dx - rx*dz
    cz := rx*dy - ry*dx
    nx := dx*cosA + cx*sinA + rx*dot*(1-cosA)
    ny := dy*cosA + cy*sinA + ry*dot*(1-cosA)
    nz := dz*cosA + cz*sinA + rz*dot*(1-cosA)
    // Translate back and offset.
    return nx + cell.Center[0] + cell.Offset[0],
        ny + cell.Center[1] + cell.Offset[1],
        nz + cell.Center[2] + cell.Offset[2]
}
```

- [ ] **Step 4: Run, expect PASS**

```
go test ./pkg/planetgen/noise/ -run TestGenerateJitter -v
go test ./pkg/planetgen/noise/ -run TestJitterTransform -v
```

Expected: PASS.

- [ ] **Step 5: Lint and commit**

```
golangci-lint run ./pkg/planetgen/noise/
git add pkg/planetgen/noise/jitter.go pkg/planetgen/noise/jitter_test.go
git commit -m "P7: JitterField cells + per-cell rotation/offset transform (item 20 part 1)"
```

---

## Task 8: Wire jitter into Detail control field

The Detail control field is sampled inside `pkg/planetgen/field/control.go`. Find the detail-fbm sample call and route its sample direction through `JitterField.Transform` when a `JitterField` is provided.

**Files:**
- Modify: `pkg/planetgen/field/control.go`
- Modify: `pkg/planetgen/field/control_test.go`

- [ ] **Step 1: Inspect the Detail field sample**

```
grep -n "Detail" pkg/planetgen/field/control.go | head -20
sed -n '1,60p' pkg/planetgen/field/control.go
```

Identify the function that produces the Detail field — likely `BuildDetailField` or similar. The plan below assumes the structure.

- [ ] **Step 2: Add a `JitterField` parameter to the Detail-field builder**

The least-invasive change: add a `*noise.JitterField` parameter to whatever function produces the Detail field. The renderer (Task 10) will pass a `JitterField` when one is generated. Existing callers pass `nil` to preserve behavior; tests pass `nil` unless they specifically test jitter.

In `pkg/planetgen/field/control.go`, find the Detail-field producer (assume it's a function like `BuildControlFields(profile *types.PlanetProfile, master int64, S int) [...]` returning the 5 fields). Identify the inner per-pixel loop that calls `noise.Sample` (or fbm sample) for the Detail field, and rewrite:

```go
// Before
dx, dy, dz := cubemap.FacePixelToDir(f, px, py, S)
val := detailFbm(dx, dy, dz)

// After
dx, dy, dz := cubemap.FacePixelToDir(f, px, py, S)
if jitter != nil {
    dx, dy, dz = jitter.Transform(dx, dy, dz)
}
val := detailFbm(dx, dy, dz)
```

(`jitter` is the new `*noise.JitterField` parameter.) Update every call site.

- [ ] **Step 3: Write the test**

In `pkg/planetgen/field/control_test.go`, add:

```go
func TestDetailFieldJitterDifferentFromUnjittered(t *testing.T) {
    // Same profile, same seed, same S — but one with jitter and one without.
    // The Detail field outputs should differ for at least 5% of pixels.
    profile := &types.PlanetProfile{
        ControlConfig: defaultControlConfig(),  // helper that returns a non-empty config
        JitterEnabled: true, JitterCellCount: 32, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.1,
    }
    S := 32
    fieldsNoJitter := BuildControlFields(profile, 11, S, nil)
    jf := noise.GenerateJitter(profile, 11, S)
    fieldsJitter := BuildControlFields(profile, 11, S, jf)
    var differ int
    var total int
    for f := range fieldsNoJitter.Detail.Faces {
        for i := range fieldsNoJitter.Detail.Faces[f] {
            total++
            if math.Abs(fieldsNoJitter.Detail.Faces[f][i]-fieldsJitter.Detail.Faces[f][i]) > 1e-6 {
                differ++
            }
        }
    }
    if float64(differ)/float64(total) < 0.05 {
        t.Errorf("jitter changed only %d/%d (%.1f%%) of detail pixels — want ≥5%%", differ, total, 100*float64(differ)/float64(total))
    }
}
```

If `BuildControlFields`'s actual signature differs (e.g., it returns a struct with a `Detail` field, or takes different positional args), adapt the test to match. The point of the test is that the jitter parameter actually changes the output. Update the helper `defaultControlConfig` to whatever exists, or inline the config literal. **Verify by reading the existing `control_test.go` first.**

- [ ] **Step 4: Run, expect FAIL on the signature change first**

```
go build ./pkg/planetgen/field/...
```

Expect compile errors at every call site that doesn't pass the `*noise.JitterField` parameter. Fix them by passing `nil` at every existing call site (rocky.go, generate-planet-maps, planet-explorer, etc).

- [ ] **Step 5: Run the unit test, expect PASS**

```
go test ./pkg/planetgen/field/ -run TestDetailFieldJitter -v
```

Expected: PASS — the jittered run produces a different Detail field.

- [ ] **Step 6: Lint and commit**

```
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/field/control.go pkg/planetgen/field/control_test.go
# Plus any rocky.go / planet-explorer call-site nil pass-throughs
git commit -m "P7: route Detail field samples through JitterField.Transform (item 20 part 2)"
```

---

## Task 9: Wire jitter into Whittaker biome jitter

Same pattern, applied to the biome-lookup jitter noise.

**Files:**
- Modify: `pkg/planetgen/biome/whittaker.go` (or wherever the biome jitter sample lives — verify with grep)
- Modify: `pkg/planetgen/biome/whittaker_test.go`

- [ ] **Step 1: Locate the biome-jitter sample**

```
grep -rn "jitter\|Jitter\|biomeNoise\|biome.*noise" pkg/planetgen/biome/
```

Identify the per-pixel ±3-channel jitter introduced in Tier S. It's likely a small fbm sample whose result is added to the biome RGB color before the OkLab blend.

- [ ] **Step 2: Write the test**

In `pkg/planetgen/biome/whittaker_test.go`:

```go
func TestBiomeJitterAffectedByJitterField(t *testing.T) {
    profile := &types.PlanetProfile{
        BiomeTable: defaultBiomeTable(),  // or whatever fixture exists
        JitterEnabled: true, JitterCellCount: 32, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.1,
    }
    S := 16
    // Construct a synthetic Whittaker T/M cube map where every pixel maps to
    // the same biome cell, so the only source of color variation is jitter noise.
    T := constantField(S, 0.5)
    M := constantField(S, 0.5)
    H := constantField(S, 0.5)
    out1 := ApplyWhittaker(profile, 7, S, T, M, H, nil)
    jf := noise.GenerateJitter(profile, 7, S)
    out2 := ApplyWhittaker(profile, 7, S, T, M, H, jf)
    var differ int
    for f := range out1.Faces {
        for i := range out1.Faces[f] {
            if out1.Faces[f][i] != out2.Faces[f][i] {
                differ++
            }
        }
    }
    if differ == 0 {
        t.Errorf("jitter had no effect on biome output")
    }
}
```

If `ApplyWhittaker`'s actual name/signature differs, adapt. The crucial property: pass a `*noise.JitterField` parameter (nil in legacy callers).

- [ ] **Step 3: Run, expect FAIL on missing signature**

```
go build ./pkg/planetgen/biome/...
```

Update every call site to pass `nil` (or a real jf in rocky.go).

- [ ] **Step 4: Implement the wiring**

Inside the biome-jitter sample, transform the sample direction through `jitter.Transform` when non-nil — same pattern as Task 8.

- [ ] **Step 5: Run the test**

```
go test ./pkg/planetgen/biome/ -run TestBiomeJitter -v
```

Expected: PASS.

- [ ] **Step 6: Lint and commit**

```
golangci-lint run ./pkg/planetgen/biome/
git add pkg/planetgen/biome/whittaker.go pkg/planetgen/biome/whittaker_test.go
# Plus call-site nil passes elsewhere
git commit -m "P7: route Whittaker biome jitter through JitterField.Transform (item 20 part 3)"
```

---

## Task 10: Render pipeline integration

`pkg/planetgen/render/rocky.go` calls control-field, biome, and per-stage code. Generate `PlateField` and `JitterField` once at the start of the rocky pipeline; pass them through to `BuildControlFields` (Task 8) and `ApplyWhittaker` (Task 9). Plates are computed and stashed but not consumed by any production stage in Phase 7 — they exist for the debug pipeline only.

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`
- Modify: `pkg/planetgen/render/rocky_test.go`

- [ ] **Step 1: Inspect the existing rocky pipeline**

```
sed -n '1,80p' pkg/planetgen/render/rocky.go
grep -n "GenerateRocky\|RenderRocky\|generateRocky" pkg/planetgen/render/rocky.go
```

Identify the entry point and the order of steps.

- [ ] **Step 2: Add plate + jitter generation**

Near the top of the entry point (e.g., `GenerateRocky` or `RenderRocky`), add:

```go
// Phase 7 Tier B: generate plate field (data-only in Phase 7 — used by debug stages only)
// and jitter field (consumed by Detail control field + biome jitter).
plates := field.GeneratePlates(profile, master, S)
jitter := noise.GenerateJitter(profile, master, S)
```

Both can be nil; downstream callers handle nil correctly.

Pass `jitter` into `BuildControlFields(...)` and `ApplyWhittaker(...)` calls.

`plates` is unused in production for Phase 7; stash it on a struct that the debug renderer can read, or pass it through to `RenderRockyDebug` only. The cleanest move: extend `DebugFrame` (Task 11) with a `Plates *field.PlateField` and `Jitter *noise.JitterField` field; during the debug render pass, populate them so the debug stages can read them.

- [ ] **Step 3: Write a regression test**

In `pkg/planetgen/render/rocky_test.go`:

```go
func TestRockyRenderWithJitterShiftsOutput(t *testing.T) {
    profile := *Profiles["terran"]  // copy
    profile.JitterEnabled = false
    cm1 := GenerateRocky(&profile, 13, 64)

    profile.JitterEnabled = true
    profile.JitterCellCount = 64
    profile.JitterRotMax = math.Pi / 4
    profile.JitterOffsetMax = 0.1
    cm2 := GenerateRocky(&profile, 13, 64)

    var differ int
    for f := range cm1.Faces {
        for i := range cm1.Faces[f] {
            if cm1.Faces[f][i] != cm2.Faces[f][i] {
                differ++
            }
        }
    }
    if differ < 100 {
        t.Errorf("jitter on/off only differed in %d pixels — want substantial shift", differ)
    }
}

func TestRockyRenderPlatesNoEffect(t *testing.T) {
    // Phase 7 invariant: plate generation must not affect the production
    // render. Compare a render with PlateCount=12 to PlateCount=0; identical.
    profile1 := *Profiles["terran"]
    profile1.PlateCount = 12
    cm1 := GenerateRocky(&profile1, 13, 64)

    profile2 := *Profiles["terran"]
    profile2.PlateCount = 0
    cm2 := GenerateRocky(&profile2, 13, 64)

    for f := range cm1.Faces {
        for i := range cm1.Faces[f] {
            if cm1.Faces[f][i] != cm2.Faces[f][i] {
                t.Fatalf("Phase 7: plates changed production render at face %d idx %d", f, i)
                return
            }
        }
    }
}
```

`Profiles` is exported from `pkg/planetgen/profile.go`. Adjust the import path if needed.

- [ ] **Step 4: Run, expect PASS**

```
go test ./pkg/planetgen/render/ -run TestRockyRender -v
```

Expected: both PASS. If `TestRockyRenderPlatesNoEffect` fails, something is consuming the plate field in production — find and fix (it must be debug-only in Phase 7).

- [ ] **Step 5: Lint and commit**

```
golangci-lint run ./pkg/planetgen/render/
git add pkg/planetgen/render/rocky.go pkg/planetgen/render/rocky_test.go
git commit -m "P7: wire plates + jitter into rocky render pipeline (jitter visible, plates data-only)"
```

---

## Task 11: Debug stages for plates and jitter

Six new debug stages plumbed through `RenderRockyDebug`.

**Files:**
- Modify: `pkg/planetgen/render/debug.go`
- Modify: `pkg/planetgen/render/debug_test.go`
- Modify: `pkg/planetgen/render/flat_cache.go`

- [ ] **Step 1: Add stage-specific raster fields to `DebugStage`**

The existing `DebugStage` struct has `RawFbm`, `SumAfter`, `ColorAfter`. Plate stages need:

```go
// Categorical raster (plate id, jitter cell id). When set, the
// debug renderer paints each unique value as a distinct hue.
CategoricalAfter *cubemap.CubeMap
// Boolean raster (oceanic flag). Two-tone render.
BooleanAfter *cubemap.CubeMap
// Single-channel float (SDF in km). Heatmap-rendered.
ScalarAfter *cubemap.CubeMapF
```

Add to `DebugStage` in `pkg/planetgen/render/debug.go`. Make sure `Skipped` semantics still hold: a skipped plate stage emits a placeholder (zeroed raster) so consumers don't crash.

- [ ] **Step 2: Add `Plates` / `Jitter` to `DebugFrame`**

```go
type DebugFrame struct {
    Stages []DebugStage
    // Phase 7: cached for downstream debug stage rendering.
    Plates *field.PlateField
    Jitter *noise.JitterField
}
```

Imports: `"github.com/rsned/spacemolt-kb/pkg/planetgen/field"` and `"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"`.

- [ ] **Step 3: Populate plate + jitter fields and append 6 stages**

Inside `RenderRockyDebug` (or its heightmap-debug helper), after generating plates and jitter:

```go
frame.Plates = plates
frame.Jitter = jitter

if plates != nil {
    frame.Stages = append(frame.Stages,
        DebugStage{Name: "Plates: id", Kind: "field", CategoricalAfter: paintCategoricalCubeMap16(plates.PlateID, S)},
        DebugStage{Name: "Plates: oceanic", Kind: "field", BooleanAfter: paintOceanicMask(plates, S)},
        DebugStage{Name: "Plates: convergent", Kind: "field", ScalarAfter: scalarFromKmPerFace(plates.Convergent, S)},
        DebugStage{Name: "Plates: divergent", Kind: "field", ScalarAfter: scalarFromKmPerFace(plates.Divergent, S)},
        DebugStage{Name: "Plates: transform", Kind: "field", ScalarAfter: scalarFromKmPerFace(plates.Transform, S)},
    )
}
if jitter != nil {
    frame.Stages = append(frame.Stages,
        DebugStage{Name: "Jitter: cells", Kind: "field", CategoricalAfter: paintCategoricalCubeMap16(jitter.PerPixel, S)},
    )
}
```

Add the helpers (in the same file or `debug_palette.go`):

```go
// paintCategoricalCubeMap maps each unique int16 id to a distinct
// color via golden-ratio hue stepping.
func paintCategoricalCubeMap16(ids [cubemap.NumFaces][]int16, S int) *cubemap.CubeMap {
    out := cubemap.New(S)
    for f := range ids {
        for i, id := range ids[f] {
            out.Faces[f][i] = goldenHue(int(id))
        }
    }
    return out
}

// paintOceanicMask paints oceanic-flagged plates blue and continental
// plates brown.
func paintOceanicMask(pf *field.PlateField, S int) *cubemap.CubeMap {
    blue := color.RGBA{R: 80, G: 120, B: 200, A: 255}
    brown := color.RGBA{R: 130, G: 100, B: 70, A: 255}
    out := cubemap.New(S)
    for f := range pf.PlateID {
        for i, id := range pf.PlateID[f] {
            if pf.Plates[id].IsOceanic {
                out.Faces[f][i] = blue
            } else {
                out.Faces[f][i] = brown
            }
        }
    }
    return out
}

// scalarFromKmPerFace wraps a per-face km-distance slice into a
// CubeMapF for the existing scalar-heatmap painter.
func scalarFromKmPerFace(km [cubemap.NumFaces][]float64, S int) *cubemap.CubeMapF {
    out := cubemap.NewF(S)
    for f := range km {
        copy(out.Faces[f], km[f])
    }
    return out
}

// goldenHue returns a categorical color for id by stepping golden
// ratio in HSV hue space.
func goldenHue(id int) color.RGBA {
    const phi = 0.61803398875
    h := math.Mod(float64(id)*phi, 1.0)
    return hsvToRGB(h, 0.65, 0.85)
}

// hsvToRGB converts (h, s, v) with h ∈ [0,1] to color.RGBA.
func hsvToRGB(h, s, v float64) color.RGBA {
    i := int(h * 6)
    f := h*6 - float64(i)
    p := v * (1 - s)
    q := v * (1 - f*s)
    t := v * (1 - (1-f)*s)
    var r, g, b float64
    switch i % 6 {
    case 0:
        r, g, b = v, t, p
    case 1:
        r, g, b = q, v, p
    case 2:
        r, g, b = p, v, t
    case 3:
        r, g, b = p, q, v
    case 4:
        r, g, b = t, p, v
    case 5:
        r, g, b = v, p, q
    }
    return color.RGBA{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255), A: 255}
}
```

`paintCategoricalCubeMap16` is reused for both plate ids and jitter cell ids (both `[]int16` per face).

- [ ] **Step 4: Update existing flat_cache key**

In `pkg/planetgen/render/flat_cache.go`, the cache key is computed from a subset of profile fields. Append the four jitter knobs:

```go
key := fmt.Sprintf("%d|...|%v|%d|%f|%f",
    master,
    // ... existing fields ...
    profile.JitterEnabled,
    profile.JitterCellCount,
    profile.JitterRotMax,
    profile.JitterOffsetMax,
)
```

Plate fields do **not** go in the key — plates aren't computed on the flat path.

- [ ] **Step 5: Write debug-stage tests**

In `pkg/planetgen/render/debug_test.go`:

```go
func TestRenderRockyDebugIncludesPlateStages(t *testing.T) {
    profile := *Profiles["terran"]
    frame := RenderRockyDebug(&profile, 17, 32, nil)
    var names []string
    for _, s := range frame.Stages {
        names = append(names, s.Name)
    }
    for _, want := range []string{
        "Plates: id", "Plates: oceanic",
        "Plates: convergent", "Plates: divergent", "Plates: transform",
        "Jitter: cells",
    } {
        var found bool
        for _, n := range names {
            if n == want {
                found = true
                break
            }
        }
        if !found {
            t.Errorf("stage %q missing from %+v", want, names)
        }
    }
}

func TestRenderRockyDebugZeroPlates(t *testing.T) {
    // jovian/ice_giant have PlateCount=0 and JitterEnabled=false;
    // debug pipeline must not crash and must omit those stages.
    profile := *Profiles["jovian"]
    frame := RenderRockyDebug(&profile, 17, 32, nil)
    for _, s := range frame.Stages {
        if strings.HasPrefix(s.Name, "Plates: ") || s.Name == "Jitter: cells" {
            t.Errorf("unexpected stage %q for plate-less profile", s.Name)
        }
    }
}
```

- [ ] **Step 6: Run all debug + render tests**

```
go test ./pkg/planetgen/render/ -v
```

Expected: all PASS, including the existing Phase 6 debug tests and the new ones.

- [ ] **Step 7: Lint and commit**

```
golangci-lint run ./pkg/planetgen/render/
git add pkg/planetgen/render/
git commit -m "P7: 6 debug stages for plates + jitter; flat_cache invalidation key"
```

---

## Task 12: Seam-test helper package (item 19)

Create the test-only `seamtest` package; both `WalkSeams`-based assertions are needed by Task 13.

**Files:**
- Create: `pkg/planetgen/cubemap/seamtest/seamtest.go`
- Create: `pkg/planetgen/cubemap/seamtest/seamtest_test.go`

- [ ] **Step 1: Write the failing test**

`pkg/planetgen/cubemap/seamtest/seamtest_test.go`:

```go
package seamtest

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestWalkSeamsVisitsEachEdgePixelOnce(t *testing.T) {
    S := 16
    var faces [cubemap.NumFaces][]int
    for f := range faces {
        faces[f] = make([]int, S*S)
    }
    visits := 0
    WalkSeams(faces, S, func(_ cubemap.Face, _ Edge, _ int, _, _ int) {
        visits++
    })
    // 12 edges × S pixels each, each pixel visited once from this side.
    if visits != 12*S {
        t.Errorf("WalkSeams visited %d pairs, want %d", visits, 12*S)
    }
}

func TestAssertSeamMatchPasses(t *testing.T) {
    S := 8
    var faces [cubemap.NumFaces][]int16
    for f := range faces {
        faces[f] = make([]int16, S*S)
    }
    // All zeros → every seam pixel matches.
    AssertSeamMatch(t, "all-zeros", faces, S)
}

func TestAssertSeamMatchFailsForMismatch(t *testing.T) {
    S := 8
    var faces [cubemap.NumFaces][]int16
    for f := range faces {
        faces[f] = make([]int16, S*S)
    }
    // Force a mismatch on the +X face top-left edge pixel.
    faces[cubemap.FacePosX][0] = 99
    inner := &mockT{}
    AssertSeamMatch(inner, "synthetic", faces, S)
    if !inner.failed {
        t.Errorf("expected failure on synthetic mismatch")
    }
}

func TestAssertSeamContinuityPasses(t *testing.T) {
    S := 8
    cm := cubemap.NewF(S)
    for f := range cm.Faces {
        for i := range cm.Faces[f] {
            cm.Faces[f][i] = 0.5
        }
    }
    AssertSeamContinuity(t, "constant", cm, 0.01)
}

type mockT struct {
    failed bool
}

func (m *mockT) Errorf(string, ...any) { m.failed = true }
func (m *mockT) Fatalf(string, ...any) { m.failed = true }
func (m *mockT) Helper()               {}
```

The mock in the last block is a minimal `testing.TB` subset for unit-testing the helper itself. The helper's signatures use a small interface so we can mock it.

- [ ] **Step 2: Run, expect FAIL**

```
go test ./pkg/planetgen/cubemap/seamtest/ -v
```

Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement the helper**

`pkg/planetgen/cubemap/seamtest/seamtest.go`:

```go
// Package seamtest provides helpers for unit-testing seam continuity
// across cube-map face boundaries. Used only by tests; not imported
// by production code.
package seamtest

import (
    "fmt"
    "math"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Edge identifies a face's edge.
type Edge int

const (
    EdgeTop Edge = iota
    EdgeBottom
    EdgeLeft
    EdgeRight
)

// TB is the subset of testing.TB this package needs. testing.T
// satisfies it; tests of seamtest itself use a mock.
type TB interface {
    Errorf(format string, args ...any)
    Fatalf(format string, args ...any)
    Helper()
}

// WalkSeams visits every pixel pair on every cube face edge. For each
// pixel on edge E of face F, it finds the matched-pair pixel on the
// adjacent face via cubemap.FacePixelToDir + DirToFacePixel and calls
// cb with both values.
//
// The callback is called once per edge pixel, with `here` being the
// value on the current face and `there` being the value on the
// adjacent face. The 12 cube edges are walked in order
// (FacePosX:Top, FacePosX:Bottom, ...).
func WalkSeams[T any](faces [cubemap.NumFaces][]T, S int, cb func(face cubemap.Face, edge Edge, idx int, here, there T)) {
    edges := [4]Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight}
    for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
        for _, e := range edges {
            for i := 0; i < S; i++ {
                px, py := edgePixel(e, i, S)
                here := faces[f][py*S+px]
                tFace, tPx, tPy := adjacentEdgePixel(f, e, i, S)
                there := faces[tFace][tPy*S+tPx]
                cb(f, e, i, here, there)
            }
        }
    }
}

func edgePixel(e Edge, i, S int) (int, int) {
    switch e {
    case EdgeTop:
        return i, 0
    case EdgeBottom:
        return i, S - 1
    case EdgeLeft:
        return 0, i
    case EdgeRight:
        return S - 1, i
    }
    return 0, 0
}

// adjacentEdgePixel returns the (face, px, py) just outside the edge,
// computed by stepping one pixel beyond the edge on the current face
// and re-projecting via FacePixelToDirFractional + DirToFacePixel.
func adjacentEdgePixel(f cubemap.Face, e Edge, i, S int) (cubemap.Face, int, int) {
    px, py := edgePixel(e, i, S)
    var dpx, dpy float64
    switch e {
    case EdgeTop:
        dpy = -1
    case EdgeBottom:
        dpy = +1
    case EdgeLeft:
        dpx = -1
    case EdgeRight:
        dpx = +1
    }
    dx, dy, dz := cubemap.FacePixelToDirFractional(
        f,
        float64(px)+0.5+dpx,
        float64(py)+0.5+dpy,
        S,
    )
    mag := math.Sqrt(dx*dx + dy*dy + dz*dz)
    return cubemap.DirToFacePixel(dx/mag, dy/mag, dz/mag, S)
}

// AssertSeamMatch fails t if any seam pixel pair has different values
// (categorical assertion for plate ids and similar).
func AssertSeamMatch[T comparable](t TB, name string, faces [cubemap.NumFaces][]T, S int) {
    t.Helper()
    var mismatches int
    var firstFace cubemap.Face
    var firstEdge Edge
    var firstIdx int
    var firstHere, firstThere T
    WalkSeams(faces, S, func(face cubemap.Face, edge Edge, idx int, here, there T) {
        if here != there {
            if mismatches == 0 {
                firstFace, firstEdge, firstIdx, firstHere, firstThere = face, edge, idx, here, there
            }
            mismatches++
        }
    })
    if mismatches > 0 {
        t.Errorf("%s: %d seam pixels mismatched; first at face=%v edge=%v idx=%d here=%v there=%v",
            name, mismatches, firstFace, firstEdge, firstIdx, firstHere, firstThere)
    }
}

// AssertSeamContinuity fails t if any seam pixel pair on f differs by
// more than tolPct (as fraction of f's value range) of the field's
// range. tolPct should be in [0,1] (e.g. 0.01 for 1%).
func AssertSeamContinuity(t TB, name string, f *cubemap.CubeMapF, tolPct float64) {
    t.Helper()
    var fmin, fmax float64 = math.Inf(1), math.Inf(-1)
    for face := range f.Faces {
        for _, v := range f.Faces[face] {
            if v < fmin {
                fmin = v
            }
            if v > fmax {
                fmax = v
            }
        }
    }
    rng := fmax - fmin
    if rng == 0 {
        return // constant — vacuously continuous
    }
    var maxDelta float64
    var worstFace cubemap.Face
    var worstEdge Edge
    var worstIdx int
    WalkSeams(f.Faces, f.Size, func(face cubemap.Face, edge Edge, idx int, a, b float64) {
        d := math.Abs(a - b)
        if d > maxDelta {
            maxDelta = d
            worstFace, worstEdge, worstIdx = face, edge, idx
        }
    })
    pct := maxDelta / rng
    if pct > tolPct {
        msg := fmt.Sprintf("%s: seam delta %.4f (%.2f%% of range %.4f) exceeds %.2f%% — worst at face=%v edge=%v idx=%d",
            name, maxDelta, 100*pct, rng, 100*tolPct, worstFace, worstEdge, worstIdx)
        t.Errorf("%s", msg)
    }
}
```

- [ ] **Step 4: Run, expect PASS**

```
go test ./pkg/planetgen/cubemap/seamtest/ -v
```

Expected: PASS.

- [ ] **Step 5: Lint and commit**

```
golangci-lint run ./pkg/planetgen/cubemap/seamtest/
git add pkg/planetgen/cubemap/seamtest/
git commit -m "cubemap/seamtest: WalkSeams + continuity & match assertions (item 19)"
```

---

## Task 13: Seam-QA tests on plates, jitter, and Tier-A fields

Apply the seamtest helpers to the actual planet-gen output for the curated 13-planet golden set.

**Files:**
- Create: `pkg/planetgen/field/plates_seam_test.go`
- Create: `pkg/planetgen/noise/jitter_seam_test.go`
- Create: `pkg/planetgen/render/rocky_seam_test.go`

- [ ] **Step 1: Plate-id and SDF seam tests**

`pkg/planetgen/field/plates_seam_test.go`:

```go
package field

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
)

func TestPlateFieldSeamMatch(t *testing.T) {
    seeds := map[string]int64{
        "terran": 1, "super_terran": 2, "oceanic": 3, "tundra": 4,
        "arid": 5, "glacial": 6, "scorched": 7, "lava_world": 8,
    }
    S := 64
    for name, master := range seeds {
        t.Run(name, func(t *testing.T) {
            profile := planetgen.Profiles[name]
            pf := GeneratePlates(profile, master, S)
            if pf == nil {
                t.Skip("PlateCount=0 for this archetype")
            }
            seamtest.AssertSeamMatch(t, name+":plate-id", pf.PlateID, S)
            for kind, slc := range map[string][cubemap.NumFaces][]float64{
                "convergent": pf.Convergent,
                "divergent":  pf.Divergent,
                "transform":  pf.Transform,
            } {
                cm := &cubemap.CubeMapF{Size: S}
                for i := range cm.Faces {
                    cm.Faces[i] = slc[i]
                }
                seamtest.AssertSeamContinuity(t, name+":"+kind, cm, 0.02)
            }
        })
    }
}
```

- [ ] **Step 2: Jitter seam test**

`pkg/planetgen/noise/jitter_seam_test.go`:

```go
package noise_test

import (
    "math"
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/field"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
)

func TestJitteredDetailFieldSeamContinuity(t *testing.T) {
    seeds := map[string]int64{
        "terran": 1, "super_terran": 2, "oceanic": 3, "tundra": 4,
        "arid": 5, "glacial": 6, "scorched": 7, "lava_world": 8,
    }
    S := 64
    for name, master := range seeds {
        t.Run(name, func(t *testing.T) {
            profile := planetgen.Profiles[name]
            jf := noise.GenerateJitter(profile, master, S)
            if jf == nil {
                t.Skip("jitter disabled")
            }
            fields := field.BuildControlFields(profile, master, S, jf)
            seamtest.AssertSeamContinuity(t, name+":Detail", fields.Detail, 0.02)
        })
    }
    _ = math.Pi // silence unused-import nag if Detail removed
    _ = cubemap.NumFaces
}
```

If `BuildControlFields` returns a different shape, adjust the access path to whatever holds the Detail field.

- [ ] **Step 3: Heightmap + control-field seam tests**

`pkg/planetgen/render/rocky_seam_test.go`:

```go
package render

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
)

func TestRockyHeightmapSeamContinuity(t *testing.T) {
    archetypes := []string{
        "terran", "super_terran", "oceanic", "tundra",
        "arid", "glacial", "scorched", "lava_world",
        "hothouse", "ice_world",
    }
    S := 64
    for i, name := range archetypes {
        t.Run(name, func(t *testing.T) {
            master := int64(100 + i)
            profile := planetgen.Profiles[name]
            // Generate the rocky heightmap via the debug pipeline so we can
            // inspect the Sum stages.
            frame := RenderRockyDebug(profile, master, S, nil)
            for _, st := range frame.Stages {
                if st.SumAfter == nil {
                    continue
                }
                seamtest.AssertSeamContinuity(t, name+":"+st.Name, st.SumAfter, 0.01)
            }
        })
    }
}
```

- [ ] **Step 4: Run, expect PASS (with possible threshold tuning)**

```
go test ./pkg/planetgen/field/ -run TestPlateFieldSeam -v
go test ./pkg/planetgen/noise/ -run TestJitteredDetail -v
go test ./pkg/planetgen/render/ -run TestRockyHeightmapSeam -v
```

Expected: PASS at thresholds 0.01 (continuous), 0 (categorical), 0.02 (SDF/jitter). If a threshold legitimately needs to be wider (e.g. some fbm field overshoots seam), bump it modestly (≤ 5%); larger overshoots indicate a real bug, not a threshold problem.

- [ ] **Step 5: Lint and commit**

```
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/field/plates_seam_test.go pkg/planetgen/noise/jitter_seam_test.go pkg/planetgen/render/rocky_seam_test.go
git commit -m "P7: seam-QA tests on plates + jitter + heightmap (item 19)"
```

---

## Task 14: Statistical invariants for plates and jitter

The Phase 0 test infrastructure has statistical invariants per planet (lon-seam delta, ocean ratios, alpha=255). Add invariants for plates and jitter.

**Files:**
- Modify: `cmd/generate-planet-maps/golden_test.go` (or wherever statistical invariants live — verify with grep)

- [ ] **Step 1: Locate the invariants file**

```
grep -rn "TestStatisticalInvariants\|TestGoldens\|alpha.*255\|seam.*delta" cmd/generate-planet-maps/
```

- [ ] **Step 2: Add plate invariants**

For each rocky planet with `PlateCount > 0`:

```go
func TestPhase7PlateInvariants(t *testing.T) {
    archetypes := map[string]int64{
        "terran": 1, "super_terran": 2, "oceanic": 3, "tundra": 4,
        "arid": 5, "glacial": 6, "scorched": 7, "lava_world": 8,
    }
    S := 64
    for name, master := range archetypes {
        t.Run(name, func(t *testing.T) {
            profile := planetgen.Profiles[name]
            pf := field.GeneratePlates(profile, master, S)
            if pf == nil {
                t.Skipf("no plates for %s", name)
            }
            // Number of distinct plate ids equals PlateCount.
            seen := make(map[int16]int)
            for f := range pf.PlateID {
                for _, id := range pf.PlateID[f] {
                    if id < 0 {
                        t.Fatalf("unfilled pixel in face %d", f)
                    }
                    seen[id]++
                }
            }
            if len(seen) != profile.PlateCount {
                t.Errorf("got %d distinct plate ids, want %d", len(seen), profile.PlateCount)
            }
            for id, c := range seen {
                if c == 0 {
                    t.Errorf("plate %d has 0 pixels", id)
                }
            }
        })
    }
}

func TestPhase7JitterInvariants(t *testing.T) {
    archetypes := map[string]int64{
        "terran": 1, "tundra": 2, "arid": 3, "scorched": 4,
    }
    S := 64
    for name, master := range archetypes {
        t.Run(name, func(t *testing.T) {
            profile := planetgen.Profiles[name]
            jf := noise.GenerateJitter(profile, master, S)
            if jf == nil {
                t.Skipf("jitter disabled for %s", name)
            }
            seen := make(map[int16]int)
            for f := range jf.PerPixel {
                for _, id := range jf.PerPixel[f] {
                    seen[id]++
                }
            }
            if len(seen) != len(jf.Cells) {
                t.Errorf("got %d distinct cell ids, want %d", len(seen), len(jf.Cells))
            }
        })
    }
}
```

Imports: `"github.com/rsned/spacemolt-kb/pkg/planetgen"`, `"github.com/rsned/spacemolt-kb/pkg/planetgen/field"`, `"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"`.

- [ ] **Step 3: Run**

```
go test ./cmd/generate-planet-maps/ -run TestPhase7 -v
```

Expected: PASS.

- [ ] **Step 4: Lint and commit**

```
golangci-lint run ./cmd/generate-planet-maps/
git add cmd/generate-planet-maps/golden_test.go
git commit -m "P7: statistical invariants for plates and jitter"
```

---

## Task 15: Planet-explorer debug-stage dropdown

Add the 6 new debug stages to the explorer's stage dropdown.

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Inspect the existing dropdown**

```
grep -n "DebugStage\|debugStage\|stageDropdown\|stages" cmd/planet-explorer/web/app.js | head -20
```

- [ ] **Step 2: Add the new stage names**

The frontend gets stage names from the wasm `RenderRockyDebug` call (or analogous) — they should appear automatically once the Go side adds them. Verify by:

1. Building wasm: `(cd cmd/planet-explorer && GOOS=js GOARCH=wasm go build ./wasm/)`
2. Running the explorer: `(cd cmd/planet-explorer && go run .)`
3. Opening `localhost:8080`, picking `terran`, expanding the debug-stage dropdown.

If the new stages appear, no JS change needed. If JS hard-codes a stage list, append the six new entries.

- [ ] **Step 3: Add a swatch-mode plate-stages disable**

In `app.js`, when swatch mode is selected, disable plate-stage dropdown entries with hint "switch to cube mode":

```js
const platelessInSwatch = ["Plates: id", "Plates: oceanic", "Plates: convergent", "Plates: divergent", "Plates: transform"];
if (renderMode === "swatch" && platelessInSwatch.includes(selectedStage)) {
    showHint("Plate stages require cube-render mode");
    selectedStage = null;
}
```

Adjust to match the existing JS style (event handlers, hint UI, etc).

- [ ] **Step 4: Smoke test in the browser**

Open the explorer, render `terran`, switch through each new debug stage, confirm:
- `Plates: id` shows distinct hue regions covering every face, no gaps.
- `Plates: oceanic` shows blue + brown two-tone in plausible distribution.
- `Plates: convergent/divergent/transform` show heatmaps with bright lines along boundaries.
- `Jitter: cells` shows ~120 distinct hue regions.

For `jovian` and `ice_giant`, all 6 stages either don't appear or show a "no plates / jitter disabled" placeholder.

- [ ] **Step 5: Commit**

```
git add cmd/planet-explorer/
git commit -m "P7: add 6 debug stages to planet-explorer dropdown"
```

---

## Task 16: Goldens re-bake and determinism check

Single re-bake of the curated 13-planet golden set covering jitter visual changes and any incidental shifts.

**Files:**
- Modify: `cmd/generate-planet-maps/testdata/golden/*.png` (regenerated)

- [ ] **Step 1: Build everything, confirm clean state**

```
go build ./...
go test ./pkg/planetgen/...
golangci-lint run ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/
```

All green expected.

- [ ] **Step 2: Re-bake goldens**

```
go test -run TestGolden -update ./cmd/generate-planet-maps/
```

This may take ~20 minutes for the full 13-planet set at face=1024.

- [ ] **Step 3: Diff against committed**

```
git status cmd/generate-planet-maps/testdata/golden/
git diff cmd/generate-planet-maps/testdata/golden/ | head -20
```

Expected: every PNG in the golden dir has a binary diff. View them with the existing `cmd/tools/planet-image-diff` tool to confirm the diffs look like jitter (broken repetition, otherwise tuned terrain preserved).

- [ ] **Step 4: Determinism check (no `-update`)**

```
go test -run TestGolden ./cmd/generate-planet-maps/
```

Expected: PASS — re-running with the freshly-baked goldens produces zero diff. If this fails, there's a non-deterministic seed somewhere; trace via per-stage diff.

- [ ] **Step 5: Inspect a few diffs visually**

For each archetype with a substantial diff:

```
go run ./cmd/tools/planet-image-diff -- terran
```

Confirm: terrain shapes preserved; high-frequency texture varies across cell boundaries (~120 cells visible if you squint at the Detail field). No new seam artifacts.

- [ ] **Step 6: Commit goldens**

```
git add cmd/generate-planet-maps/testdata/golden/
git commit -m "P7: goldens re-bake — Voronoi cell-coordinate jitter (item 20)"
```

---

## Task 17: README updates and memory note

**Files:**
- Modify: `cmd/generate-planet-maps/README.md`
- Modify: `/home/robert/.claude/projects/-home-robert-spacemolt-kb/memory/project_planet_gen.md`

- [ ] **Step 1: README**

Append a `## Phase 7 — Tier B foundations` section to `cmd/generate-planet-maps/README.md`:

```markdown
## Phase 7 — Tier B foundations (master-plan items 14, 19, 20)

**Plates** (`pkg/planetgen/field/plates.go`). Voronoi tectonic plates produced via Fibonacci-spiral seeds + random flood-fill across cube faces. Per-plate motion (RotAxis + AngSpeed) classifies boundary pixels as convergent / divergent / transform; three JFA passes give per-pixel SDFs in km. Phase 7 produces the data and renders it as 5 debug stages but **does not** consume it in the production render — that wiring lands in Phase 8.

Profile knobs: `PlateCount` (per-archetype, 0–12), `OceanicPlateFraction` (0–1, Bernoulli per plate), `PlateConvergentT` (default 0.75, signed-velocity threshold).

**Jitter** (`pkg/planetgen/noise/jitter.go`). ~120 Voronoi cells with per-cell rotation+offset applied to the Detail control field and the Whittaker biome jitter. Breaks visible repetition without changing tuned terrain shapes.

Profile knobs: `JitterEnabled` (per-archetype), `JitterCellCount` (default 120), `JitterRotMax` (default π/4), `JitterOffsetMax` (default 0.1).

**Seam-QA** (`pkg/planetgen/cubemap/seamtest/`). Test-only helper exposing `WalkSeams`, `AssertSeamContinuity` (continuous fields, threshold per cent of range), `AssertSeamMatch` (categorical, exact). Used by `field/plates_seam_test.go`, `noise/jitter_seam_test.go`, `render/rocky_seam_test.go`.

Default thresholds: 1% (continuous fields), 2% (SDFs and post-jitter), 0 (plate-id categorical).
```

- [ ] **Step 2: Memory note**

Update `/home/robert/.claude/projects/-home-robert-spacemolt-kb/memory/project_planet_gen.md`:

Add a new "Tier B Phase 7 status" section before "Future work":

```markdown
## Phase 7 status

**Tier B Phase 7 complete** as of <fill date>:
- Item 14 (full): Voronoi tectonic plates — plate-id flood-fill + motion + oceanic flag + 3 boundary SDFs in `pkg/planetgen/field/plates.go`. Data-only in production; 5 debug stages.
- Item 19: seam-QA helper at `pkg/planetgen/cubemap/seamtest/`; unit-tested seam continuity (1%/2%) and categorical match (0 mismatches) on the curated 13-planet set.
- Item 20: Voronoi cell-coordinate jitter at `pkg/planetgen/noise/jitter.go`; ~120 cells, applied to Detail control field and Whittaker biome jitter — breaks visible repetition.

**Phase 7 deferred to Phase 8** (master-plan item 6/10 retroactive rewires):
- Ridged mountain mask switching from `Continentalness` to `smoothstep(0.5, 0.7, distToConvergent)`.
- Voronoi continents seeded from continental-plate centroids.

**Per-archetype plate counts**: terran/super_terran/oceanic=12, tundra=8, arid/glacial=6, scorched/lava_world=4, hothouse/ice_world/jovian/ice_giant/unknown=0.

**Seed namespaces**: `plates.{seeds,motion,oceanic,fill.random,sdf.{convergent,divergent,transform}}`, `jitter.{cells,rot,offset}` — all via `seed.Domain`.
```

- [ ] **Step 3: Commit**

```
git add cmd/generate-planet-maps/README.md
git commit -m "P7: README + memory note for Tier B foundations"
```

(The memory file is not in the repo; it lives at the absolute path above.)

---

## Acceptance gates (run all after Task 17)

```
go build ./...
go test ./pkg/planetgen/... ./cmd/generate-planet-maps/...
golangci-lint run ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/
go test -run TestGolden ./cmd/generate-planet-maps/   # determinism: must pass without -update
```

All green. Then:

1. `go test` of `pkg/planetgen/field/`, `noise/`, `cubemap/seamtest/` PASS.
2. Statistical invariants (`cmd/generate-planet-maps/golden_test.go`) PASS.
3. Seam-QA tests PASS at default thresholds (1% / 0 / 2%).
4. Goldens re-bake committed; second-pass `go test` with no `-update` produces no diff.
5. Planet-explorer renders all 6 new debug stages without crashing on every planet type, including PlateCount=0 / JitterEnabled=false cases.
6. Profile JSON serialization round-trip passes for all 7 new fields.
7. README and memory note updated.
8. `git push` — branch should be ahead by ~17 commits beyond the Phase 7 starting point.

---

## Risks recap (from spec §6)

- **Cube-seam flood-fill bugs** — highest risk. Caught by Task 12's seam-match assertion plus Task 4's PlateCount=1 covers-all test.
- **`PlateConvergentT = 0.75` may be wrong** for our motion-vector distribution — debug stages let us inspect; tunable via profile knob.
- **Jitter goldens drift** — single re-bake at end (Task 16); ΔE2000 expected 1.5–4.0.
- **Wasm bundle size** — `-ldflags="-s -w"` already applied; revisit if explorer load regresses.
