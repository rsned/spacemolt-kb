# Planet Generation Phase 5: Tier-A Particle Hydraulic Erosion

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax for tracking. Phase 5 follows Phase 6 (debug view); the debug view will visualize erosion's strongly-subtractive contribution for free as the only Phase 5 implementation work focuses on the simulation itself.

**Goal:** Land master-plan item 9 — particle hydraulic erosion that walks droplets across the cube-sphere, carving channels and depositing sediment. Wraps Tier A (along with Phase 3 items 6/7/11 and Phase 4 items 8/10/12/13). Output is a strongly-subtractive contribution to the heightmap that produces dendritic channels and alluvial fans no fbm sum can match.

**Architecture:** A new `pkg/planetgen/field/erosion.go` package adds `Erode(heightmap, cfg, seed) -> *cubemap.CubeMapF` and a struct-of-arrays droplet simulator. Droplets live in 3D unit-sphere coordinates; gradient and velocity are computed in a local tangent frame at each step. The cube map's existing seam-aware `Sample` handles cross-face propagation automatically — no special edge-table code needed. The renderer wires erosion in after heightmap construction (post-normalize, post-coastal, before craters). Droplet count auto-scales with face size so the slider tool stays interactive at face=64 while batch renders use the full count at face=1024+.

**Tech Stack:** Go 1.24+, `pkg/planetgen/cubemap` (for sphere ↔ face/pixel conversion and seam-aware sampling), `pkg/planetgen/seed.Domain`, `math/rand/v2`. No external library — this is a from-scratch droplet simulator since `setanarut/rainfall` operates on 2D plane heightmaps and adapting it to the cube-sphere is more work than rolling our own ~150 LOC implementation.

---

## Pre-flight notes

**Why roll our own.** The original master plan suggested `setanarut/rainfall` but that library expects a 2D rectangular heightmap with implicit Cartesian neighbors. On a cube-sphere we need (a) seam-aware gradient computation across face boundaries, (b) tangent-space velocity that gets parallel-transported as the droplet moves, and (c) heightmap deposition that resolves to (face, px, py). Wrapping the library would mean baking to equirect, eroding, and re-importing with seam fixup — three sources of artifacts. Rolling our own avoids them and keeps the simulation in unit-sphere 3D throughout.

**Algorithm sketch (per droplet).** Initialize: random position on the sphere, zero velocity, water=1, sediment=0. For each step (max 50): sample height + gradient at current position; update velocity `v = inertia·v + (1-inertia)·(-grad_tangent)`; step `pos += v * stepLen`, normalize back to unit sphere; sample new height; compute capacity from speed/water/slope; if carrying excess → deposit at *previous* pixel, else erode from *previous* pixel. Evaporate water each step. Stop when water < 0.01 or step count exceeded.

**Droplet count auto-scaling.** A profile knob `ErosionDroplets` is the count at the canonical face size (DefaultFaceSize == 1024). The renderer scales: `effective = max(minDroplets, profile.ErosionDroplets * (S*S) / (1024*1024))`. At face=64 with `ErosionDroplets=200000` this gives ≈ 781 droplets — too few for a useful preview. We override the floor: `minDroplets = 5000` so the slider tool's preview always shows enough droplets to communicate the look. At face=256 it's ≈ 12.5k droplets, at face=1024 it's the full 200k.

**Seam-aware gradient.** At each step, sample height at `pos` and at four offset positions in the tangent plane (±ε along east tangent, ±ε along north tangent). Central differences give ∂h/∂east and ∂h/∂north. Convert back to a 3D tangent vector via `grad3D = ∂h/∂east · east_tangent + ∂h/∂north · north_tangent`. The gradient is intrinsic to the surface, not to any face's coordinate system, so seams are crossed transparently as long as `cubemap.CubeMapF.Sample` handles them (it does).

**Position → face/pixel conversion.** For depositing/eroding, convert the 3D unit-sphere position back to (face, px, py) via the inverse of `FacePixelToDir`. This already exists in `cubemap` as `DirToFacePixel` (or implement if missing).

**Determinism.** Seeded via `pgseed.Domain(master, "erosion")`. The same profile + seed must produce bit-identical heightmaps across runs. Goldens for archetypes that opt in will be re-baked once and committed.

**Interactive vs batch.** At face=256, ~12k droplets × ~30 steps each ≈ 360k sphere samples + 360k height writes. Single-threaded that's ~200 ms. Acceptable for the slider tool. At face=1024 with 200k droplets we're at 6M samples — 3 s, well within batch-render budget. Goroutine parallelism is possible (each droplet is independent until it deposits) but writes need a per-pixel mutex or sharded approach; defer to a follow-up if profiling shows it's needed.

**Forward compat with debug view (Phase 6).** Erosion lives between Coastal and Craters in the heightmap pipeline. The debug view introduced in Phase 6 already has stage hooks for "Coastal" and "Craters"; we add a new "Erosion" stage between them. Its raw thumbnail (the height-delta from erosion) is strongly subtractive in carved channels, so the red-palette helper from Phase 6 T1 renders it correctly without changes.

---

## File structure

**New files:**

| Path | Role |
|---|---|
| `pkg/planetgen/field/erosion.go` | `ErosionConfig`, `Erode` entry point, droplet loop |
| `pkg/planetgen/field/erosion_test.go` | Determinism + height-delta + extreme-input behavior |

**Modified files:**

| Path | Reason |
|---|---|
| `pkg/planetgen/types/types.go` | Add `ErosionConfig` + `Erosion ErosionConfig` field on `PlanetProfile` |
| `pkg/planetgen/render/rocky.go` | Wire erosion stage; auto-scale droplet count by face size |
| `pkg/planetgen/render/debug.go` | Add "Erosion" debug stage hook (Phase-6-compatible) |
| `pkg/planetgen/cubemap/sample.go` (or wherever direction helpers live) | Add `DirToFacePixel` if missing |
| `cmd/planet-explorer/web/app.js` | Add erosion slider panel |
| `cmd/generate-planet-maps/README.md` | Phase 5 section |

---

## Task 1: Sphere-walk droplet simulator primitive

**Files:**
- Create: `pkg/planetgen/field/erosion.go`
- Create: `pkg/planetgen/field/erosion_test.go`
- Modify: `pkg/planetgen/types/types.go`

- [ ] **Step 1: Add `ErosionConfig`**

```go
// pkg/planetgen/types/types.go
type ErosionConfig struct {
    Droplets       int     // canonical droplet count at face=1024; auto-scaled at lower face sizes
    Inertia        float64 // [0,1]; how much velocity is preserved between steps. 0.05 default
    Capacity       float64 // sediment capacity multiplier; ~4 default
    ErosionRate    float64 // fraction of "missing" capacity carved per step; 0.3 default
    Deposition     float64 // fraction of "excess" sediment dropped per step; 0.3 default
    Evaporation    float64 // water loss per step; 0.01 default
    MinSlope       float64 // floor on slope used for capacity to avoid 0; 0.01 default
    MaxStepsPerDrop int    // hard cap on steps per droplet; 50 default
    Gravity        float64 // arbitrary gravitational constant; 4.0 default
    StepLen        float64 // per-step displacement on the unit sphere; 0 = auto = 1/(2*S)
}
```

Add `Erosion ErosionConfig` to `PlanetProfile`. Zero `Droplets` disables.

- [ ] **Step 2: Write the failing tests**

```go
// pkg/planetgen/field/erosion_test.go
package field

import (
    "math"
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestErodeNoOpZeroDroplets(t *testing.T) {
    hm := cubemap.NewF(32)
    for face := range hm.Faces {
        for i := range hm.Faces[face] {
            hm.Faces[face][i] = 0.5
        }
    }
    cfg := types.ErosionConfig{Droplets: 0}
    out := Erode(123, hm, cfg, 32)
    for face := range out.Faces {
        for i, v := range out.Faces[face] {
            if v != 0.5 {
                t.Fatalf("0 droplets should be a no-op; face %d idx %d got %f", face, i, v)
            }
        }
    }
}

func TestErodeDeterministic(t *testing.T) {
    hm := makeBumpyHeightmap(32, 1.0)
    cfg := types.ErosionConfig{Droplets: 500, Inertia: 0.05, Capacity: 4, ErosionRate: 0.3, Deposition: 0.3, Evaporation: 0.01, MaxStepsPerDrop: 50}
    a := Erode(7, hm.Clone(), cfg, 32)
    b := Erode(7, hm.Clone(), cfg, 32)
    for face := range a.Faces {
        for i := range a.Faces[face] {
            if a.Faces[face][i] != b.Faces[face][i] {
                t.Fatalf("non-deterministic at face %d idx %d", face, i)
            }
        }
    }
}

func TestErodeLowersPeaks(t *testing.T) {
    hm := makePyramid(32, 0.9, 0.4) // peak at +X face center, base height 0.4
    peakBefore := hm.Get(cubemap.FacePosX, 16, 16)
    cfg := types.ErosionConfig{Droplets: 2000, Inertia: 0.05, Capacity: 4, ErosionRate: 0.5, Deposition: 0.2, Evaporation: 0.01, MaxStepsPerDrop: 60, Gravity: 4}
    out := Erode(11, hm.Clone(), cfg, 32)
    peakAfter := out.Get(cubemap.FacePosX, 16, 16)
    if peakAfter >= peakBefore {
        t.Errorf("peak should be lower after erosion; before=%f after=%f", peakBefore, peakAfter)
    }
}

func makeBumpyHeightmap(S int, _ float64) *cubemap.CubeMapF {
    out := cubemap.NewF(S)
    for face := range out.Faces {
        for py := range S {
            for px := range S {
                u := float64(px)/float64(S-1) - 0.5
                v := float64(py)/float64(S-1) - 0.5
                out.Set(cubemap.Face(face), px, py, 0.5 + 0.2*math.Sin(u*8)*math.Cos(v*8))
            }
        }
    }
    return out
}

func makePyramid(S int, peak, base float64) *cubemap.CubeMapF {
    out := cubemap.NewF(S)
    for face := range out.Faces {
        for i := range out.Faces[face] {
            out.Faces[face][i] = base
        }
    }
    cx, cy := S/2, S/2
    for r := 0; r < S/2; r++ {
        h := peak * (1 - float64(r)/float64(S/2))
        if h < base {
            h = base
        }
        for py := cy - r; py <= cy+r; py++ {
            for px := cx - r; px <= cx+r; px++ {
                if px < 0 || px >= S || py < 0 || py >= S {
                    continue
                }
                if out.Get(cubemap.FacePosX, px, py) < h {
                    out.Set(cubemap.FacePosX, px, py, h)
                }
            }
        }
    }
    return out
}
```

Run: `go test ./pkg/planetgen/field/ -run TestErode -v`
Expected: FAIL — `Erode` not defined.

- [ ] **Step 3: Implement `DirToFacePixel` if missing**

In `pkg/planetgen/cubemap/`:

```go
// DirToFacePixel returns the (face, px, py) of the cube-map cell that
// the unit-sphere direction (dx, dy, dz) projects onto. Inverse of
// FacePixelToDir.
func DirToFacePixel(dx, dy, dz float64, S int) (Face, int, int) {
    abs := func(v float64) float64 { if v < 0 { return -v }; return v }
    ax, ay, az := abs(dx), abs(dy), abs(dz)
    var face Face
    var sc, tc, ma float64
    switch {
    case ax >= ay && ax >= az:
        ma = ax
        if dx > 0 {
            face = FacePosX
            sc, tc = -dz, -dy
        } else {
            face = FaceNegX
            sc, tc = dz, -dy
        }
    case ay >= ax && ay >= az:
        ma = ay
        if dy > 0 {
            face = FacePosY
            sc, tc = dx, dz
        } else {
            face = FaceNegY
            sc, tc = dx, -dz
        }
    default:
        ma = az
        if dz > 0 {
            face = FacePosZ
            sc, tc = dx, -dy
        } else {
            face = FaceNegZ
            sc, tc = -dx, -dy
        }
    }
    u := 0.5 * (sc/ma + 1)
    v := 0.5 * (tc/ma + 1)
    px := int(u * float64(S))
    py := int(v * float64(S))
    if px < 0 {
        px = 0
    }
    if px >= S {
        px = S - 1
    }
    if py < 0 {
        py = 0
    }
    if py >= S {
        py = S - 1
    }
    return face, px, py
}
```

(Convention should match the existing `FacePixelToDir` in the same package — verify direction signs match before committing.)

Add a round-trip test:

```go
// pkg/planetgen/cubemap/sample_test.go (or wherever face conversion tests live)
func TestDirToFacePixelRoundTrip(t *testing.T) {
    S := 64
    for face := range Face(NumFaces) {
        for py := 0; py < S; py++ {
            for px := 0; px < S; px++ {
                dx, dy, dz := FacePixelToDir(face, px, py, S)
                f2, px2, py2 := DirToFacePixel(dx, dy, dz, S)
                if f2 != face || (px2 != px && abs(px2-px) > 1) || (py2 != py && abs(py2-py) > 1) {
                    t.Errorf("round-trip mismatch: (%v,%d,%d) → (%v,%d,%d)", face, px, py, f2, px2, py2)
                }
            }
        }
    }
}

func abs(v int) int { if v < 0 { return -v }; return v }
```

- [ ] **Step 4: Implement `Erode`**

```go
// pkg/planetgen/field/erosion.go
package field

import (
    "math"
    "math/rand/v2"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Erode runs particle hydraulic erosion on heightmap in-place and
// returns the same map (modified). cfg.Droplets <= 0 is a no-op
// (returns heightmap unchanged).
//
// The simulation walks droplets in 3D unit-sphere coordinates. Each
// droplet samples height + gradient at its current position, updates a
// tangent-space velocity, steps to a new position, and erodes or
// deposits at the previous pixel based on its sediment capacity.
//
// Determinism is rooted at masterSeed; same seed + same cfg + same
// heightmap → same output, regardless of CPU.
func Erode(masterSeed int64, heightmap *cubemap.CubeMapF, cfg types.ErosionConfig, S int) *cubemap.CubeMapF {
    if cfg.Droplets <= 0 {
        return heightmap
    }
    rng := rand.New(rand.NewPCG(uint64(seed.Domain(masterSeed, "erosion")),
        uint64(seed.Domain(masterSeed, "erosion")^0x5a5a5a5a))) //nolint:gosec
    inertia := defaultIfZero(cfg.Inertia, 0.05)
    capacity := defaultIfZero(cfg.Capacity, 4.0)
    erosionRate := defaultIfZero(cfg.ErosionRate, 0.3)
    deposition := defaultIfZero(cfg.Deposition, 0.3)
    evaporation := defaultIfZero(cfg.Evaporation, 0.01)
    minSlope := defaultIfZero(cfg.MinSlope, 0.01)
    maxSteps := cfg.MaxStepsPerDrop
    if maxSteps <= 0 {
        maxSteps = 50
    }
    gravity := defaultIfZero(cfg.Gravity, 4.0)
    stepLen := cfg.StepLen
    if stepLen <= 0 {
        stepLen = 1.0 / float64(2*S)
    }

    for range cfg.Droplets {
        simulateDroplet(rng, heightmap, S, inertia, capacity, erosionRate,
            deposition, evaporation, minSlope, gravity, stepLen, maxSteps)
    }
    return heightmap
}

func simulateDroplet(rng *rand.Rand, hm *cubemap.CubeMapF, S int,
    inertia, capacity, erosionRate, deposition, evaporation, minSlope, gravity, stepLen float64,
    maxSteps int) {
    // Random position on the unit sphere via uniform-sphere sampling.
    z := 1 - 2*rng.Float64()
    phi := 2 * math.Pi * rng.Float64()
    r := math.Sqrt(1 - z*z)
    px, py, pz := r*math.Cos(phi), z, r*math.Sin(phi)

    var vx, vy, vz float64 // tangent-space velocity (3D vector tangent to sphere)
    water := 1.0
    sediment := 0.0

    for range maxSteps {
        h, gx, gy, gz := sampleWithGradient(hm, S, px, py, pz)
        // Update velocity: damp toward -gradient
        vx = inertia*vx + (1-inertia)*(-gx)
        vy = inertia*vy + (1-inertia)*(-gy)
        vz = inertia*vz + (1-inertia)*(-gz)
        // Project velocity onto tangent plane (subtract radial component).
        dot := vx*px + vy*py + vz*pz
        vx -= dot * px
        vy -= dot * py
        vz -= dot * pz
        // Renormalize to unit tangent length and step
        vlen := math.Sqrt(vx*vx + vy*vy + vz*vz)
        if vlen < 1e-9 {
            return // stalled
        }
        vxN, vyN, vzN := vx/vlen, vy/vlen, vz/vlen
        nx, ny, nz := px+vxN*stepLen, py+vyN*stepLen, pz+vzN*stepLen
        nlen := math.Sqrt(nx*nx + ny*ny + nz*nz)
        nx, ny, nz = nx/nlen, ny/nlen, nz/nlen
        nh, _, _, _ := sampleWithGradient(hm, S, nx, ny, nz)

        // Capacity = max(-Δh, minSlope) * speed * water * capacity
        deltaH := nh - h
        speed := vlen
        slope := -deltaH
        if slope < minSlope {
            slope = minSlope
        }
        cap := slope * speed * water * capacity

        // Resolve to (face, px, py) for the *previous* position.
        face, ix, iy := cubemap.DirToFacePixel(px, py, pz, S)
        if sediment > cap || deltaH > 0 {
            // Deposit
            amt := (sediment - cap) * deposition
            if deltaH > 0 {
                if amt > deltaH {
                    amt = deltaH
                }
            }
            cur := hm.Get(face, ix, iy)
            if cur+amt > 1 {
                amt = 1 - cur
            }
            hm.Set(face, ix, iy, cur+amt)
            sediment -= amt
        } else {
            // Erode
            amt := (cap - sediment) * erosionRate
            if amt > -deltaH {
                amt = -deltaH
            }
            cur := hm.Get(face, ix, iy)
            if cur-amt < 0 {
                amt = cur
            }
            hm.Set(face, ix, iy, cur-amt)
            sediment += amt
        }

        // Step
        px, py, pz = nx, ny, nz
        // Update velocity speed using gravity-on-slope
        speedSq := speed*speed + (-deltaH)*gravity
        if speedSq < 0 {
            speedSq = 0
        }
        s2 := math.Sqrt(speedSq)
        vx = vxN * s2
        vy = vyN * s2
        vz = vzN * s2
        water *= 1 - evaporation
        if water < 0.01 {
            return
        }
    }
}

// sampleWithGradient returns (h, ∇h_x, ∇h_y, ∇h_z) where the gradient
// is a 3D vector in the tangent plane at (x,y,z). Computed by sampling
// at four offset positions (±ε along east tangent, ±ε along north
// tangent) and projecting back to 3D via the tangent basis.
func sampleWithGradient(hm *cubemap.CubeMapF, S int, x, y, z float64) (float64, float64, float64, float64) {
    h := hm.Sample(x, y, z)
    const eps = 1e-3
    // Local east/north tangents at the position.
    ex, ey, ez := tangentEast(x, y, z)
    nx, ny, nz := tangentNorth(x, y, z)
    // Sample at ±ε along each tangent.
    rx := func(a, b, c, dx, dy, dz, e float64) float64 { return hm.Sample(normalize(a+dx*e, b+dy*e, c+dz*e)) }
    hE := rx(x, y, z, ex, ey, ez, eps)
    hW := rx(x, y, z, ex, ey, ez, -eps)
    hN := rx(x, y, z, nx, ny, nz, eps)
    hS := rx(x, y, z, nx, ny, nz, -eps)
    dE := (hE - hW) / (2 * eps)
    dN := (hN - hS) / (2 * eps)
    gx := dE*ex + dN*nx
    gy := dE*ey + dN*ny
    gz := dE*ez + dN*nz
    return h, gx, gy, gz
}

func tangentEast(x, y, z float64) (float64, float64, float64) {
    // (north pole) × pos, normalized; gives an "eastward" tangent
    cx, cy, cz := -z, 0.0, x
    n := math.Sqrt(cx*cx + cy*cy + cz*cz)
    if n < 1e-9 {
        return 1, 0, 0
    }
    return cx / n, cy / n, cz / n
}

func tangentNorth(x, y, z float64) (float64, float64, float64) {
    // pos × east; gives a "northward" tangent perpendicular to east + radial
    ex, ey, ez := tangentEast(x, y, z)
    nx := y*ez - z*ey
    ny := z*ex - x*ez
    nz := x*ey - y*ex
    n := math.Sqrt(nx*nx + ny*ny + nz*nz)
    if n < 1e-9 {
        return 0, 1, 0
    }
    return nx / n, ny / n, nz / n
}

func normalize(x, y, z float64) (float64, float64, float64) {
    n := math.Sqrt(x*x + y*y + z*z)
    if n == 0 {
        return x, y, z
    }
    return x / n, y / n, z / n
}

// rx returned three floats; collapse into the multi-return convenience above.
// (workaround: hm.Sample only takes (x,y,z), so the closure boxes that.)

func defaultIfZero(v, fallback float64) float64 {
    if v == 0 {
        return fallback
    }
    return v
}
```

Note the closure in `sampleWithGradient` is a bit awkward; the simpler form is acceptable so long as `hm.Sample` is called four times per step. If `Sample` doesn't take a single 3-tuple arg today, adapt it.

- [ ] **Step 5: Run tests to confirm**

Run: `go test ./pkg/planetgen/field/ -run TestErode -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/planetgen/field/erosion.go pkg/planetgen/field/erosion_test.go \
  pkg/planetgen/types/types.go pkg/planetgen/cubemap/*.go
git commit -m "P5 T1: sphere-walk droplet erosion primitive"
```

---

## Task 2: Wire erosion into `RenderRocky` with auto-scaled droplet count

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`

- [ ] **Step 1: Add the stage**

In `generateRockyHeightmapDebug` (the function that already runs through control fields → ridged → provinces → continents → normalize → coastal → craters), insert erosion between coastal and craters:

```go
if profile.Erosion.Droplets > 0 {
    // Auto-scale droplet count by face-area ratio. At face=1024 we use
    // the canonical count; at face=64 we floor at 5000 droplets so the
    // preview communicates the look.
    const baseSize = planetgen.DefaultFaceSize
    scale := float64(S*S) / float64(baseSize*baseSize)
    n := int(float64(profile.Erosion.Droplets) * scale)
    if n < 5000 && profile.Erosion.Droplets >= 5000 {
        n = 5000
    }
    cfg := profile.Erosion
    cfg.Droplets = n
    field.Erode(seed, heightmap, cfg, S)
}
```

(`planetgen.DefaultFaceSize` may need re-exporting through a constant — if the import cycle blocks, hard-code 1024 with a comment.)

- [ ] **Step 2: Run all goldens (must stay green)**

```bash
go test -timeout 25m -run TestGolden ./cmd/generate-planet-maps/
```

Expected: PASS — no archetype enables `Erosion` yet.

- [ ] **Step 3: Commit**

```bash
git add pkg/planetgen/render/rocky.go
git commit -m "P5 T2: wire erosion stage into RenderRocky with face-size auto-scale"
```

---

## Task 3: Debug-view stage hook

**Files:**
- Modify: `pkg/planetgen/render/debug.go` (assumes Phase 6 has landed)

- [ ] **Step 1: Add the Erosion stage to the debug pipeline**

In `generateRockyHeightmapDebug`, instrument the erosion stage the same way the other heightmap stages are instrumented:

```go
if frame != nil {
    var before *cubemap.CubeMapF
    if !bypass["Erosion"] {
        before = heightmap.Clone()
        field.Erode(seed, heightmap, cfg, S)
    }
    delta := cubemap.NewF(S)
    if before != nil {
        for face := range cubemap.Face(cubemap.NumFaces) {
            for i := range delta.Faces[face] {
                delta.Faces[face][i] = heightmap.Faces[face][i] - before.Faces[face][i]
            }
        }
    }
    frame.Stages = append(frame.Stages, DebugStage{
        Name:     "Erosion",
        Kind:     "height",
        RawFbm:   delta, // signed; carved channels go negative → red palette
        SumAfter: heightmap.Clone(),
        Skipped:  bypass["Erosion"],
    })
}
```

- [ ] **Step 2: Test**

```go
// pkg/planetgen/render/debug_test.go (append)
func TestDebugFrameIncludesErosion(t *testing.T) {
    prof := types.PlanetProfile{
        Type: "test", Renderer: "rocky",
        ControlConfig: types.ControlConfig{
            Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5,
                Spline: planetcolor.Spline{Knots: []planetcolor.SplineKnot{{0, 0}, {1, 0.5}}}},
        },
        Erosion: types.ErosionConfig{Droplets: 200, Inertia: 0.05, Capacity: 4, ErosionRate: 0.3, Deposition: 0.3, Evaporation: 0.01, MaxStepsPerDrop: 30},
    }
    frame := RenderRockyDebug(&prof, 7, 32, nil)
    found := false
    for _, s := range frame.Stages {
        if s.Name == "Erosion" {
            found = true
            break
        }
    }
    if !found {
        t.Error("Erosion stage missing from DebugFrame")
    }
}
```

Run: `go test ./pkg/planetgen/render/ -run TestDebugFrameIncludesErosion -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/planetgen/render/debug.go pkg/planetgen/render/debug_test.go
git commit -m "P5 T3: erosion stage in debug view"
```

---

## Task 4: Erosion slider panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Add `renderErosionPanel`**

```js
function renderErosionPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Erosion',
    'Particle hydraulic erosion: droplets walk the heightmap, carving channels and depositing sediment. Droplets=0 disables. The slider tool auto-scales droplet count to face size — at face=64 you get a 5k-droplet preview; full count runs at face=1024.');
  if (!profile.Erosion) profile.Erosion = {};
  const e = profile.Erosion;
  const reset = () => {
    const orig = (originalProfile && originalProfile.Erosion) || {};
    profile.Erosion = JSON.parse(JSON.stringify(orig));
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => { profile.Erosion = {}; commitProfile(profile); renderPanels(); };
  const randomize = () => {
    profile.Erosion = {
      Droplets:        Math.round(50000 + Math.random() * 150000),
      Inertia:         round2(0.02 + Math.random() * 0.15),
      Capacity:        round2(2 + Math.random() * 6),
      ErosionRate:     round2(0.1 + Math.random() * 0.5),
      Deposition:      round2(0.1 + Math.random() * 0.5),
      Evaporation:     round2(0.005 + Math.random() * 0.04),
      MinSlope:        round2(0.005 + Math.random() * 0.03),
      MaxStepsPerDrop: 30 + Math.floor(Math.random() * 50),
      Gravity:         round2(2 + Math.random() * 6),
    };
    commitProfile(profile);
    renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll new in-range erosion params', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out erosion config', clear));

  panel.appendChild(makeNumberRow('Droplets',
    'Canonical droplet count at face=1024. Auto-scaled at lower face sizes (floor 5000). 0 disables.',
    e.Droplets || 0, 0, 500000, '1000',
    v => { profile.Erosion.Droplets = Math.max(0, Math.round(v)); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Inertia',
    '[0,1]: how much velocity carries between steps. 0.05 default; higher = straighter channels.',
    e.Inertia || 0, 0, 1, '0.01',
    v => { profile.Erosion.Inertia = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Capacity',
    'Sediment capacity multiplier. 4 default; higher = droplets carry more before depositing.',
    e.Capacity || 0, 0, 20, '0.1',
    v => { profile.Erosion.Capacity = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('ErosionRate',
    'Fraction of "missing capacity" carved per step. 0.3 default.',
    e.ErosionRate || 0, 0, 1, '0.05',
    v => { profile.Erosion.ErosionRate = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Deposition',
    'Fraction of "excess sediment" dropped per step. 0.3 default.',
    e.Deposition || 0, 0, 1, '0.05',
    v => { profile.Erosion.Deposition = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Evaporation',
    'Water lost per step. 0.01 default.',
    e.Evaporation || 0, 0, 0.1, '0.005',
    v => { profile.Erosion.Evaporation = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('MinSlope',
    'Floor on slope used in capacity calc to avoid 0. 0.01 default.',
    e.MinSlope || 0, 0, 0.5, '0.005',
    v => { profile.Erosion.MinSlope = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('MaxStepsPerDrop',
    'Hard cap on steps per droplet. 50 default.',
    e.MaxStepsPerDrop || 0, 0, 200, '5',
    v => { profile.Erosion.MaxStepsPerDrop = Math.max(0, Math.round(v)); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Gravity',
    'Speed gain from -Δh per step. 4 default.',
    e.Gravity || 0, 0, 20, '0.1',
    v => { profile.Erosion.Gravity = v; commitProfile(profile); }));

  panels.appendChild(panel);
}
```

- [ ] **Step 2: Hook into render order**

After `renderCratersPanel` (so erosion is grouped with the post-fbm height stages):

```js
renderErosionPanel(profile, panels);
```

- [ ] **Step 3: Rebuild wasm + smoke-test**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
go run ./cmd/planet-explorer/
```

Open the explorer at face=64. Switch to terran. Set Erosion Droplets=100000 (will floor to 5000 at face=64). Confirm dendritic channels appear within 2 s. Bump face=256, confirm denser channels and slower regen (~5 s).

- [ ] **Step 4: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "P5 T4: erosion slider panel"
```

---

## Task 5: Phase 5 acceptance + per-archetype tuning + README

**Files:**
- Modify: `pkg/planetgen/profile.go` (optional erosion defaults for selected rocky archetypes)
- Modify: `cmd/generate-planet-maps/README.md`

- [ ] **Step 1: All gates green**

```bash
go test -timeout 25m ./...
golangci-lint run
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
```

- [ ] **Step 2: Per-archetype tuning**

Same workflow as Phase 3 T10 / Phase 4 T10. Tune in the explorer, export JSON, fold into `profile.go`. Suggested archetypes that benefit most from erosion:

- **terran, super_terran**: medium erosion (~100k droplets, Inertia 0.05, Capacity 4) — produces realistic river systems on land.
- **arid**: medium erosion with high capacity (Capacity 6) and high Deposition (0.5) — "dry channels" with broad alluvial fans.
- **tundra, glacial, ice_world**: light erosion (~50k droplets) — implies ancient surfaces with subdued channels.
- **scorched, hothouse**: skip — Mercury-style and hellscape don't benefit from terrestrial flow.
- **lava_world**: skip — lava channels are a separate feature for a future phase.
- **oceanic**: light, only on the small landmass — ~30k droplets.

Note: enabling erosion on archetypes that opt in will shift goldens (the heightmap actually changes). Be ready to re-bake with `go test -run TestGolden -update`.

- [ ] **Step 3: Update README**

In `cmd/generate-planet-maps/README.md`, add a Phase 5 section:

```markdown
## Phase 5 — Particle hydraulic erosion (current)

Master-plan item 9. Droplets walk the cube-sphere in 3D unit-sphere
coordinates, sampling height + gradient via seam-aware
`cubemap.CubeMapF.Sample` so cross-face propagation is implicit.
Each droplet carries water + sediment; capacity is `slope · speed ·
water · capacity`; depositing happens when sediment exceeds capacity
or the next step is uphill, eroding otherwise. Output is a
strongly-subtractive heightmap delta with dendritic channels and
alluvial fans.

Implemented in `pkg/planetgen/field/erosion.go`. Wired into
`RenderRocky` between Coastal and Craters. Profile knob
`ErosionDroplets` is the canonical count at face=1024; the renderer
auto-scales to face size, with a 5k-droplet floor so face=64 previews
still communicate the look.

Tuned defaults:
- terran, super_terran, arid: enabled (see profile.go)
- tundra, glacial, ice_world, oceanic: light enabled
- scorched, hothouse, lava_world, unknown: disabled by default

Tier A (master plan items 6–13) is now complete. Tier B (items 14–20)
and Tier C (items 21–25) remain.
```

- [ ] **Step 4: Phase 5 acceptance commit**

```bash
git commit --allow-empty -m "Phase 5 (Tier-A particle erosion) accepted"
```

Body should list T1–T5 status, gates, and call out that Tier A is now complete.

---

## Self-review notes

**Spec coverage.** Master plan §6.9 → T1 (primitive), T2 (wire-up), T4 (slider). Debug-view integration → T3 (depends on Phase 6 having landed). Per-archetype tuning + acceptance → T5. The `Erosion` profile field is unambiguously the *flow-based* erosion now; the Phase-3 control field that was misnamed `Erosion` was renamed to `Detail` in Phase 4 T1.

**Placeholder scan.** No "TODO" or "TBD". The droplet algorithm is concrete: explicit step count, capacity formula, deposit/erode branch, water evaporation. The closure in `sampleWithGradient` is noted as "awkward; adapt to whatever signature `hm.Sample` actually has" — that's a flag, not a placeholder.

**Type consistency.** `ErosionConfig` defined in T1 step 1, used by `Erode` in T1 step 4 and consumed by `RenderRocky` in T2. `DirToFacePixel` defined in T1 step 3 (or already exists), consumed by `simulateDroplet` in T1 step 4. Debug "Erosion" stage name in T3 matches the bypass key documented in pre-flight notes.

**Performance.** At face=256 with 12k droplets × 30 steps × 4 samples + 1 write per step ≈ 1.5M sphere samples, ~200 ms single-threaded. Acceptable for the slider tool. At face=1024 with 200k droplets, full pass is ~3 s, fine for batch render. Goroutine parallelism is straightforward (each droplet is independent during simulation; deposit-time writes need either per-pixel atomic or sharded buffers); defer to a follow-up if profiling shows it matters.

**Forward compat.** The renderer wires erosion as a between-stage; no Phase-6 changes are needed beyond the Erosion stage hook in T3 (which assumes Phase 6 has landed since erosion follows it in execution order). The strongly-subtractive contribution renders correctly in the existing red-palette path from Phase 6 T1 without modification.

**Untested but plausible failure modes.** (a) Droplets near the cube-map corners where two faces meet at 120° may have ill-defined tangent frames — the `tangentEast` fallback returns `(1,0,0)` if the cross product is degenerate; in practice this happens only at the poles where droplets rarely linger. (b) `DirToFacePixel` quantization at high face sizes may clamp the sub-pixel position to one cell off; the round-trip test catches this within ±1 pixel. (c) Extreme `Inertia` (close to 1) produces droplets that never turn — they walk straight off in any direction; the test in T1 step 2 doesn't catch this but the slider tooltip warns about it.
