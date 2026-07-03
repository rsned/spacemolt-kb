# Planet Generation — Phase 7 Design (Tier B foundations)

**Date:** 2026-05-02
**Branch:** `phase-0/cube-map` (worktree `/home/robert/spacemolt/kb-phase-0-cube-map`)
**Predecessors:** Phase 6 (per-stage debug pipeline) on the same branch.
**Master plan reference:** `docs/plans/2026-04-25-planet-gen-master-plan.md` §7, items 14, 19, 20.

## 1. Scope

Phase 7 lands the foundational data structures of Tier B. It produces no production-rendered change from plates themselves — plates are computed, tested, and exposed as debug stages but do not yet drive heightmap, biome, or color. The only production-visible diff in Phase 7 comes from item 20 (Voronoi cell-coordinate jitter), which breaks repetition in the Detail control field and Whittaker biome jitter.

**Items shipped:**

| Item | Subpackage | Production-visible? | LOC |
|---|---|---|---|
| 14 — Voronoi tectonic plates | `pkg/planetgen/field/plates.go` | No (debug stages only) | ~400 |
| 19 — Edge-blending QA / seam tests | `pkg/planetgen/cubemap/seamtest/` | No (test infra) | ~80 |
| 20 — Voronoi cell-coordinate jitter | `pkg/planetgen/noise/jitter.go` | Yes | ~120 |

**Out of scope (slips to Phase 8 or later):**

- Item 6 retroactive rewire (ridged mountain mask switching from `Continentalness` to plate convergent-distance). Master plan §2 marked this as a Phase 3 deliverable; Phase 7 explicitly defers it so plate code can be reviewed in isolation and tuned in the explorer before any consumer depends on its output.
- Item 10 Voronoi continents seeding from continental-plate centroids — also Phase 8.
- Items 15 (D8 rivers), 16 (rain shadow), 17 (clouds), 18 (civilization signs) — Phases 8 and 9.

## 2. Tier B sub-phase plan (context for Phase 7)

Tier B (master-plan items 14–20) is split into three sub-phases on the existing `phase-0/cube-map` branch, mirroring how Tier A shipped as Phases 4/5/6:

| Sub-phase | Items | Theme |
|---|---|---|
| **Phase 7 (this doc)** | 14, 19, 20 | Foundations — plate data, seam-QA, repetition-breaking jitter |
| Phase 8 | 15, 16 + item 6/10 rewire | Geology — D8 rivers, rain shadow, plates wired into mountain mask and continents |
| Phase 9 | 17, 18 | Overlays — clouds, civilization signs |

Each sub-phase ships a complete set of valid planet PNGs and gets its own goldens re-bake.

## 3. Design

### 3.1 Pipeline placement

Plates compute *after* control fields and *before* heightmap finalization, even though Phase 7 has no consumers yet — the slot is reserved for Phase 8 wiring. Updated stage order on the rocky path:

1. 5 control fields (Detail field samples now route through jitter when `JitterEnabled`)
2. Sum control fields → heightmap
3. Ridged
4. Provinces
5. Continents/Voronoi
6. **Plates** (NEW — plate-id, oceanic flag, three boundary SDFs; data-only in Phase 7)
7. HeightSmooth
8. Normalize
9. Coastal noise
10. Erosion
11. Craters
12. Colorize (biome jitter sample now routes through jitter when `JitterEnabled`)

Gas-giant pipeline: jovian/ice_giant/unknown have `PlateCount=0` and `JitterEnabled=false`, so the new stages no-op.

### 3.2 Plate algorithm (item 14)

**Subpackage:** `pkg/planetgen/field/plates.go`.

**Data structures:**

```go
type Plate struct {
    ID        int
    Seed      [3]float64   // unit vector — Voronoi seed direction
    RotAxis   [3]float64   // unit vector — rotation axis on sphere
    AngSpeed  float64      // [0,1]
    IsOceanic bool         // weighted by archetype OceanicPlateFraction
}

type PlateField struct {
    Size       int
    Plates     []Plate
    PlateID    [6][]int16        // per-pixel plate id, cube faces
    Convergent [6][]float64      // SDF in km; positive = away from convergent boundary
    Divergent  [6][]float64
    Transform  [6][]float64
}
```

**Generation pipeline:**

1. **Seed plates.** `N = profile.PlateCount`. Fibonacci-spiral N seed directions on the unit sphere; per-seed jitter from `seed.Domain(master, "plates.seeds")`.
2. **Per-plate attributes.** Random rot-axis (uniform unit vector) + angular speed `[0,1]` from `plates.motion`. Oceanic flag drawn from `Bernoulli(profile.OceanicPlateFraction)` via `plates.oceanic`.
3. **Random flood-fill** (not BFS) across cube faces with cross-face neighbor walk:
   - Start from each of the N plate seeds (mark seed pixel with its plate-id).
   - Maintain a frontier set of (assigned, unassigned-neighbor) pairs.
   - At each step, pick a uniformly random pair from the frontier and assign the unassigned pixel the same plate-id.
   - Cross-face neighbors discovered via the existing `cubemap.FacePixelToDir` math + nearest-pixel snap on the adjacent face (same primitive used by JFA in Tier A).
   - Termination: every pop assigns exactly one previously-unassigned pixel; loop runs exactly `6·Size² − N` iterations and then halts. Random draw breaks BFS-symmetry and produces jagged boundaries naturally.
4. **Boundary classification.** For each pixel whose 4-neighbor has a different plate-id, compute relative velocity at that point on the sphere `p`:
   - `v_rel = (ω_a × p) − (ω_b × p)` where `ω = AngSpeed · RotAxis`.
   - Boundary normal `n` = unit vector from this pixel toward the differing neighbor (in tangent plane).
   - `v_rel · n > +T` → **divergent**; `< −T` → **convergent**; `|·| < T` → **transform**. `T = profile.PlateConvergentT`, default `0.75`.
5. **Three SDFs** via three independent JFA passes — one per boundary type. Each starts with the boundary pixels of that type as seeds and propagates squared distance across faces (~11 passes). Final distance scaled by `profile.RadiusKm` to produce km units.

**Per-archetype defaults (`profile.go`):**

| Archetype | PlateCount | OceanicPlateFraction |
|---|---|---|
| terran, super_terran | 12 | 0.7 |
| oceanic | 12 | 0.9 |
| tundra | 8 | 0.5 |
| arid | 6 | 0.3 |
| glacial | 6 | 0.4 |
| scorched, lava_world | 4 | 0.2 |
| ice_world, jovian, ice_giant, unknown | 0 | n/a |

**Seed namespaces (per master plan §3.3):** `plates.seeds`, `plates.motion`, `plates.oceanic`, `plates.fill.random`, plus `plates.sdf.convergent`, `plates.sdf.divergent`, `plates.sdf.transform` if any future SDF parameterization needs sub-seeds. Each derived via `seed.Domain(master, name)` so adding new sub-seeds never shifts existing fields.

**Cube-seam correctness:** the random flood-fill is the riskiest primitive in this phase. Single-plate test (PlateCount=1) must produce identical plate-id at every seam pixel pair. Four-plate test (PlateCount=4 with seeds at cube-face centers) must produce stable boundaries that match across seams within ±1 pixel. These are unit tests, not goldens.

### 3.3 Voronoi cell-coordinate jitter (item 20)

**Subpackage:** `pkg/planetgen/noise/jitter.go`.

**Data structures:**

```go
type JitterCell struct {
    Center   [3]float64  // unit vector
    RotAxis  [3]float64  // unit vector
    RotAngle float64     // [-π/4, +π/4]
    Offset   [3]float64  // per-axis in [-0.1, +0.1]
}

type JitterField struct {
    Size      int
    CellCount int
    Cells     []JitterCell
    PerPixel  [6][]int16   // cell id per pixel
}
```

**Generation:**

1. Fibonacci-spiral `JitterCellCount` (default 120) cell centers on the sphere via `seed.Domain(master, "jitter.cells")`.
2. Per-cell `RotAxis` (random unit vector), `RotAngle` ∈ `[-JitterRotMax, +JitterRotMax]`, `Offset` (each component ∈ `[-JitterOffsetMax, +JitterOffsetMax]`). Sub-seeds: `jitter.rot`, `jitter.offset`.
3. Per-pixel cell-id assignment: nearest-center search across all 120 cells (single pass; cells convex on the sphere within tolerance, no flood-fill needed).

**Application points** (each ~10 LOC at the call site):

1. **Detail control field.** Inside the Detail-field fbm sample at direction `p`:
   ```
   cell = jitter.At(p)
   p′ = rotateAroundAxis(p − cell.Center, cell.RotAxis, cell.RotAngle) + cell.Center + cell.Offset
   value = fbm(p′)
   ```
   Adjacent cells sample different rotated views → repetition broken at cell boundaries. Discontinuities at boundaries are small (≤ 0.1 sample-space, ≤ π/4 rotation around cell center) and read as natural variation.
2. **Whittaker biome jitter** — the existing ±3-channel-units high-frequency noise from Tier S. Same transform applied to the jitter-noise lookup direction.

**No cell-boundary smoothing.** A discontinuous jump in sample-coords across a cell boundary is acceptable because the underlying fbm is continuous and the jumps are small. Smoothing would defeat the purpose (the jump is what breaks the repeat) and add complexity.

**Profile knobs (added to `PlanetProfile`):**

```go
JitterEnabled    bool     // per-archetype; false for jovian, ice_giant, unknown
JitterCellCount  int      // default 120
JitterRotMax     float64  // default π/4
JitterOffsetMax  float64  // default 0.1
```

### 3.4 Seam-QA test infrastructure (item 19)

**Subpackage:** `pkg/planetgen/cubemap/seamtest/seamtest.go`. Test-only helper, importable from any per-package test file. Not a runtime component.

**Core helper:**

```go
// WalkSeams visits every pixel pair on every cube-face edge.
// For each edge pixel on face F, finds the matched-pair pixel on the adjacent face F'
// using cubemap.FacePixelToDir and nearest-pixel snap, and calls cb with both values.
func WalkSeams[T any](
    faces [6][]T,
    size int,
    cb func(face Face, edge Edge, idx int, here, there T),
)
```

`Edge` enumerates the 12 cube edges (Top/Bottom/Left/Right of each of the 6 faces, dedup'd to 12).

**Continuous-field assertion:**

```go
func AssertSeamContinuity(t *testing.T, name string, f *cubemap.CubeMapF, tolPct float64) {
    fmin, fmax := minMaxAcrossFaces(f)
    rng := fmax - fmin
    if rng == 0 { return } // constant field — vacuously continuous
    var maxDelta float64
    var worstFace Face
    var worstEdge Edge
    var worstIdx int
    WalkSeams(f.Faces, f.Size, func(face Face, edge Edge, idx int, a, b float64) {
        d := math.Abs(a - b)
        if d > maxDelta {
            maxDelta = d; worstFace = face; worstEdge = edge; worstIdx = idx
        }
    })
    pct := maxDelta / rng
    if pct > tolPct {
        t.Errorf("%s: seam delta %.4f (%.2f%% of range %.4f) exceeds %.2f%% — worst at face=%d edge=%v idx=%d",
            name, maxDelta, 100*pct, rng, 100*tolPct, worstFace, worstEdge, worstIdx)
    }
}
```

**Categorical-field assertion (plate-id):**

```go
func AssertSeamMatch[T comparable](t *testing.T, name string, faces [6][]T, size int) {
    var mismatches int
    WalkSeams(faces, size, func(_ Face, _ Edge, _ int, a, b T) {
        if a != b { mismatches++ }
    })
    if mismatches > 0 { t.Errorf("%s: %d seam pixels mismatched", name, mismatches) }
}
```

**What gets tested per planet type** (curated 13-planet set, same as Phase 0 goldens):

| Field | Assertion | Threshold |
|---|---|---|
| 5 control fields (Continentalness, Detail, PeaksValleys, Temperature, Humidity) | continuity | 1% of range |
| Heightmap (final, post-erosion) | continuity | 1% of range |
| Plate-id | exact match | 0 mismatches |
| Convergent / Divergent / Transform SDF | continuity | 2% of range |
| Jittered Detail field (post-jitter) | continuity | 2% of range |

**Test placement:** colocated with the producing package — `field/plates_seam_test.go`, `noise/jitter_seam_test.go`, `render/rocky_seam_test.go`. The `seamtest` package is just a helper.

**Threshold rationale:** 1% permits floating-point drift across face transforms while catching real flood-fill bugs (typical magnitude 5–20%). SDFs and post-jitter fields get 2% to allow the known small discontinuities (±0.1 sample-space jitter offset; JFA's pixel-resolution distance limit at face boundaries where the nearest seed is on the adjacent face). Thresholds can be tuned post-implementation.

### 3.5 Profile changes

**`PlanetProfile` (`pkg/planetgen/profile.go`) additions:**

```go
// Phase 7 — Tier B foundations
PlateCount           int      // 0 disables plates
OceanicPlateFraction float64  // [0,1]
PlateConvergentT     float64  // default 0.75

JitterEnabled        bool     // per-archetype
JitterCellCount      int      // default 120
JitterRotMax         float64  // default π/4
JitterOffsetMax      float64  // default 0.1
```

Per-archetype `Profiles[type]` entries are filled for all 13 types (gas giants and ice_world get `PlateCount=0` and `JitterEnabled=false`).

**JSON serialization:** all 7 fields serialize via the existing struct-tag machinery. Default values omitted from JSON via `omitempty` only for `JitterEnabled` (a `false` is meaningful for non-default disable).

### 3.6 Debug-stage integration

Six new debug stages registered via the Phase 6 `DebugStage` mechanism:

| Stage ID | Source | Visualization |
|---|---|---|
| `plates.id` | `PlateField.PlateID` | Categorical color-by-id (golden-ratio hue stepping) |
| `plates.oceanic` | `PlateField.Plates[id].IsOceanic` per pixel | Two-tone (blue=oceanic, brown=continental) |
| `plates.convergent` | `PlateField.Convergent` | Heatmap (close=red, far=black) |
| `plates.divergent` | `PlateField.Divergent` | Heatmap (close=green, far=black) |
| `plates.transform` | `PlateField.Transform` | Heatmap (close=yellow, far=black) |
| `jitter.cells` | `JitterField.PerPixel` | Categorical color-by-cell-id |

Each follows the existing `DebugFrame/Stage/Bypass` pattern from Phase 6 — no new infrastructure, just new stage IDs.

**Planet-explorer:** debug-stage dropdown gets six new entries. Plate stages require full cube path (sphere-global flood-fill); swatch-mode (single-face flat render) gates them off and shows hint "switch to cube mode to view plates." Jitter cells work in swatch mode.

**LRU cache invalidation (`flat_cache.go`):** the four jitter knobs enter the cache key. Plate knobs do not enter the flat-path cache key because plates aren't computed on the flat path.

## 4. Testing

### 4.1 Unit tests

- `field/plates_test.go`:
  - Flood-fill correctness — 1-plate covers all 6 faces; k-plate boundary count matches expected from cell area.
  - Boundary classification — hand-built `(ω_a, ω_b, p, n)` fixtures hit each of convergent / divergent / transform.
  - SDF correctness — known plate layout produces distance values matching manhattan-distance sanity check at sample points.
  - Determinism — same seed → same `PlateField` (byte-equal `PlateID` arrays).
- `noise/jitter_test.go`:
  - Fibonacci spiral covers the sphere (max angular gap < expected).
  - Per-pixel cell-id is single-valued and stable.
  - Rotation+offset transform is involutive when `RotAngle=0, Offset=0` (identity).
- `cubemap/seamtest/seamtest_test.go`:
  - `WalkSeams` visits every edge pixel exactly once.
  - Matched pairs round-trip through `FacePixelToDir`.

### 4.2 Statistical invariants (CI gate, curated 13-planet set)

- All existing Tier-S/A invariants continue to pass.
- For non-zero-plate-count planets:
  - Number of distinct plate-ids equals `PlateCount`.
  - Number of oceanic-flagged plates (not pixels) within a Wilson-score 95% CI of `PlateCount · OceanicPlateFraction`. (Per-pixel fraction is too noisy at low PlateCount because plate sizes vary; per-plate fraction tests the Bernoulli draw directly.)
  - Every plate has at least 1 pixel on at least one face.
- Jitter (where enabled): all `JitterCellCount` distinct cell-ids visited; no cell empty.

### 4.3 Seam-QA tests

The item 19 deliverable. Run as ordinary `go test`. Files: `field/plates_seam_test.go`, `noise/jitter_seam_test.go`, `render/rocky_seam_test.go`. Thresholds per §3.4 table.

### 4.4 Goldens

Single full re-bake at end of phase via `go test -run TestGolden -update ./cmd/generate-planet-maps/`. No-update second pass confirms determinism.

Visual diff via `cmd/tools/planet-image-diff`. Expected ΔE2000 mean across the 13-planet set: 1.5–4.0 (jitter breaks repetition while preserving tuned character).

### 4.5 Performance

- Plates: O(faces × pixels) flood-fill + 3 × JFA (~11 passes each). At face size 1024, ~3–5 s per planet.
- Jitter cell-id assignment: O(pixels × cells) with cells = 120 → ~1.5 s per planet.
- Combined Phase 7 overhead: ~5–7 s per planet; full batch ~10% slower than current.

Acceptable; matches Tier A's per-phase budget.

## 5. Acceptance gates

1. `go build ./...` clean.
2. `golangci-lint` clean.
3. `GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/...` clean.
4. All unit tests pass.
5. Statistical invariants pass on curated 13-planet set.
6. Seam-QA tests pass with default thresholds (1% continuous / exact-match plate-id / 2% SDF & post-jitter).
7. Goldens regenerated; second-pass `go test` with no `-update` produces no diff (determinism check).
8. Planet-explorer debug panel renders all 6 new stages without crashing on every planet type, including PlateCount=0 cases (graceful "no plates" placeholder).
9. Profile JSON serialization round-trip passes for the 7 new fields.
10. `MEMORY.md` / `project_planet_gen.md` updated with Phase 7 status, debug-stage list, and seam-QA conventions.

## 6. Risks

- **Cube-seam flood-fill bugs.** Highest risk in this phase. Mitigation: dedicated unit tests at PlateCount=1 and PlateCount=4 with seam-pair assertions; seam-QA categorical match assertion as CI gate.
- **Boundary classification threshold misjudgment.** `T = 0.75` from master plan is unverified for our motion-vector distribution. Mitigation: tuneable via `PlateConvergentT` profile knob; debug stages let us inspect distribution in the explorer and adjust before Phase 8 commits to it.
- **Jitter goldens drift.** Jitter is the only production-visible change; goldens must be re-baked. Mitigation: standard goldens workflow; expected ΔE2000 1.5–4.0 is in line with Tier A re-bakes.
- **Wasm bundle size.** Plates add ~400 LOC plus three SDFs in memory. Mitigation: `-ldflags="-s -w"` already applied; revisit if explorer load time regresses.

## 7. Out-of-scope follow-ups (for Phase 8)

- Item 6 retroactive rewire — ridged mountain mask switching from `Continentalness` to `smoothstep(0.5, 0.7, distToConvergentPlate)`.
- Item 10 rewire — Voronoi continents seeded from continental-plate centroids.
- Item 15 — D8 flow accumulation + rivers, with river headwaters biased by plate-divergent-distance.
- Item 16 — wind-driven rain shadow, using plate-convergent-distance as a proxy for mountain-belt orientation until full Phase 8 wiring lands.
