# Planet Generation Phase 8 Implementation Plan: Tier B Geology

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land Tier B geology — fix the three Phase 7 seam-continuity bugs, retroactively rewire items 6 (ridged mountain mask) and 10 (Voronoi continents) onto Phase 7 plate data, and add items 15 (D8 rivers) and 16 (rain shadow).

**Date:** 2026-05-03
**Branch:** `phase-0/cube-map` (worktree `/home/robert/spacemolt/kb-phase-0-cube-map`)
**Predecessors:** Phase 7 — Tier B foundations (commits `63efaede` … `34140778`).
**Master plan reference:** `docs/plans/2026-04-25-planet-gen-master-plan.md` §7, items 6, 10, 15, 16.
**Phase 7 design reference:** `docs/plans/2026-05-02-planet-gen-phase-7-tier-b-foundations.md` §7 (out-of-scope follow-ups).
**Seam-bug worklist:** `docs/plans/phase-8-seam-bugs.md`.

**Architecture:** Phase 7 produces plate, jitter, and SDF data but does not consume it in production. Phase 8 closes the loop: the existing ridged-mountain mask and Voronoi-continents generators read plate data instead of fbm-of-Continentalness; new D8 flow + rain shadow stages read the heightmap and plate convergent SDF; all of this requires the Phase 7 seam-continuity bugs to be fixed first so plate-derived inputs don't carry per-face seam errors into every downstream consumer.

**Tech Stack:** Go 1.24+, `pkg/planetgen` packages (field, noise, render, biome, cubemap), wasm via planet-explorer, golangci-lint hard gate, golden-image regression test, ΔE2000 visual-diff threshold.

**Sub-phase position in Tier B:**

| Sub-phase | Items | Status |
|---|---|---|
| Phase 7 (foundations) | 14, 19, 20 | ✅ landed |
| **Phase 8 (this plan — geology)** | 15, 16 + item 6/10 retroactive rewires + Phase 7 seam-bug fixes | this doc |
| Phase 9 (overlays) | 17 (clouds), 18 (civilization signs) | not started |

---

## File Structure

**Created:**
- `pkg/planetgen/field/flow.go` — D8 flow accumulation, Planchon-Darboux fill, river mask (item 15, ~250 LOC)
- `pkg/planetgen/field/flow_test.go`
- `pkg/planetgen/biome/rainshadow.go` — wind-tangent moisture advection (item 16, ~150 LOC)
- `pkg/planetgen/biome/rainshadow_test.go`

**Modified:**
- `pkg/planetgen/field/plates.go` — fix flood-fill seam bug (Task 1) and JFA seam bug (Task 2)
- `pkg/planetgen/noise/jitter.go` — make cell-id lookup direction-based (Task 3)
- `pkg/planetgen/render/rocky.go` — rewire ridged mask (Task 5), wire river + rain shadow stages (Task 9)
- `pkg/planetgen/field/continents.go` — accept plate centroids as seed source (Task 6)
- `pkg/planetgen/types/types.go` — add new profile knobs (FlowConfig, RainShadowConfig, plate-mask threshold knobs)
- `pkg/planetgen/profile.go` — per-archetype defaults for new knobs
- `pkg/planetgen/render/debug.go` — register flow + rainshadow debug stages
- `pkg/planetgen/field/plates_seam_test.go`, `noise/jitter_seam_test.go`, `render/rocky_seam_test.go` — remove `t.Skip` (Task 4)
- `cmd/generate-planet-maps/invariants_test.go` — invariants for items 6/10/15/16 (Task 10)
- `cmd/generate-planet-maps/testdata/golden/*.png` — single re-bake at end (Task 11)
- `cmd/generate-planet-maps/README.md` — Phase 8 section (Task 12)

**Stage order on the rocky pipeline after Phase 8:**

1. 5 control fields (Detail uses jitter)
2. Sum control fields → heightmap
3. **Continents** (NEW: seeded from plate centroids when plates exist)
4. **Ridged** (rewired: mask = `smoothstep(low, high, dist_convergent)` instead of fbm-of-Continentalness)
5. Provinces
6. Plates (data, classification, three SDFs — same as Phase 7)
7. HeightSmooth
8. Normalize
9. Coastal noise
10. **Rivers** (NEW: D8 flow accumulation, river mask written into heightmap as carved channels)
11. Erosion
12. Craters
13. **Rain shadow** (NEW: moisture multiplier into Whittaker M)
14. Colorize (biome jitter still uses jitter)

Items 6/10 are reordered into earlier slots because they now feed the heightmap rather than colour. River carving sits after erosion smoothing, before craters, so erosion doesn't re-fill carved channels.

---

## Task 1: Fix cross-face flood-fill seam bug

**Files:**
- Modify: `pkg/planetgen/field/plates.go:89-138` (`floodFillPlates`)
- Test gate: `pkg/planetgen/field/plates_seam_test.go::TestPlateFieldSeamMatch` (currently `t.Skip`-marked)

**Diagnosis (do not assume the worklist hypothesis is correct).** The doc `phase-8-seam-bugs.md` claims "the BFS frontier is per-face; queue does not push across face boundaries". Inspecting the code shows the frontier *does* use `cubemap.FacePixelNeighbors4`, which already maps off-edge neighbors to the adjacent face. So that hypothesis is wrong. The real bug is one of:

(a) **Asymmetric neighbor map.** `FacePixelNeighbors4` projects an off-edge neighbor via `FaceUVToDir` → `DirToFacePixel`. The inverse — going from the adjacent face's edge pixel back — may snap to a *different* pixel on the original face, so flood propagation from F1→F2 can reach F2 pixels that, from F2's perspective, would have been claimed by a different chain. Two seam pixels that *should* read as a "matched pair" then end up with different plate ids.

(b) **Race in the random-pop loop.** Multiple frontier entries for the same unfilled pixel exist (one from each direction). The first popped wins; the rest are skipped via the `!= -1` guard. If two neighbors-on-different-faces both push the same matched-pair pixel with different ids, whichever is popped first wins on each face independently.

- [ ] **Step 1: Write a failing test that pinpoints the bug**

`pkg/planetgen/field/plates_floodfill_diag_test.go`:

```go
package field

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
)

func TestFloodFillSeamMatchTerran(t *testing.T) {
    pf := GeneratePlates(planetgen.Profiles["terran"], 1, 64)
    if pf == nil {
        t.Fatal("PlateCount=0 unexpected for terran")
    }
    seamtest.AssertSeamMatch(t, "terran", pf.PlateID, 64)
    _ = cubemap.NumFaces
}
```

Expected: FAIL with mismatches. Capture the count and a representative `(face, edge, idx, here, there)` for the first mismatch.

- [ ] **Step 2: Identify the actual bug**

Add temporary instrumentation to `floodFillPlates` (or a one-off `TestNeighborSymmetry`) that, for the recorded mismatch coordinates, prints:

1. The frontier order at the time the two pixels were assigned.
2. Whether `FacePixelNeighbors4(F1, x1, y1, S)` includes `(F2, x2, y2)` AND whether `FacePixelNeighbors4(F2, x2, y2, S)` includes `(F1, x1, y1)`.

If asymmetry is the cause (case a), proceed to Step 3a. If race is the cause (case b), Step 3b.

- [ ] **Step 3a (asymmetry fix): symmetrize the cross-face neighbor walk**

In `pkg/planetgen/cubemap/neighbors.go`, audit `FacePixelNeighbors4`. The current implementation projects `(px+0.5+dx, py+0.5+dy)/S → dir → (face,px,py)`. To make it symmetric, post-snap by also walking the *returned* pixel's neighbor list and choosing the entry that maps back to `(face, px, py)` if multiple candidates differ by ±1.

Simpler fix: switch to **direction-based neighbor walk** at the seam. Compute `dir = FacePixelToDir(face, px, py, S)`, rotate `dir` by one pixel in cube-tangent space, snap to nearest pixel via `DirToFacePixel`. This is what FacePixelNeighbors4 already does; the asymmetry comes from the half-pixel offset in the `(px+0.5+dx)` projection. Try removing the half-pixel offset for off-edge case only, or use a single canonical direction-based snap.

Add `pkg/planetgen/cubemap/neighbors_symmetry_test.go`:

```go
func TestFacePixelNeighbors4Symmetric(t *testing.T) {
    S := 64
    for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
        for py := 0; py < S; py++ {
            for px := 0; px < S; px++ {
                if px > 0 && px < S-1 && py > 0 && py < S-1 {
                    continue // interior — symmetry trivially holds
                }
                for _, n := range cubemap.FacePixelNeighbors4(f, px, py, S) {
                    if n.Face == f {
                        continue
                    }
                    back := cubemap.FacePixelNeighbors4(n.Face, n.PX, n.PY, S)
                    found := false
                    for _, b := range back {
                        if b.Face == f && b.PX == px && b.PY == py {
                            found = true
                            break
                        }
                    }
                    if !found {
                        t.Errorf("(%v,%d,%d) → (%v,%d,%d) not symmetric", f, px, py, n.Face, n.PX, n.PY)
                    }
                }
            }
        }
    }
}
```

This is the actual contract the flood-fill needs. If it fails, fix `FacePixelNeighbors4` (the cleanest place — also fixes any other downstream consumer of the helper). If it passes, the bug is elsewhere → Step 3b.

- [ ] **Step 3b (race fix): seam-pin pass after flood-fill**

If the neighbor walk is already symmetric, the bug is the random-order race. Add a deterministic seam-pin pass at the end of `floodFillPlates`:

```go
// Seam-pin pass: for each seam pixel pair, if the two assigned plate
// ids disagree, take the lexicographically lower (face, py, px). This
// guarantees seam-categorical equality without altering the
// statistical distribution of plate sizes appreciably (≤ 12·S pixels
// reassigned out of 6·S²).
for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
    for py := 0; py < S; py++ {
        for px := 0; px < S; px++ {
            if px > 0 && px < S-1 && py > 0 && py < S-1 {
                continue
            }
            for _, n := range cubemap.FacePixelNeighbors4(f, px, py, S) {
                if n.Face == f {
                    continue
                }
                hereID := pf.PlateID[f][py*S+px]
                thereID := pf.PlateID[n.Face][n.PY*S+n.PX]
                if hereID == thereID {
                    continue
                }
                // Lexicographic tiebreak: lower (face, py, px) wins.
                if n.Face < f || (n.Face == f && (n.PY < py || (n.PY == py && n.PX < px))) {
                    pf.PlateID[f][py*S+px] = thereID
                } else {
                    pf.PlateID[n.Face][n.PY*S+n.PX] = hereID
                }
            }
        }
    }
}
```

Note: the seam-pin pass runs *both* directions per pair, so it must be idempotent. The lexicographic-lower rule guarantees idempotence.

- [ ] **Step 4: Verify**

```
go test ./pkg/planetgen/field/ -run TestFloodFillSeamMatchTerran -v
```

Expected: PASS, 0 mismatches.

- [ ] **Step 5: Re-run determinism + invariants**

```
go test ./pkg/planetgen/field/ -v
go test ./cmd/generate-planet-maps/ -run TestPhase7PlateInvariants -v
```

Expected: PASS. Plate counts unchanged (TestPhase7PlateInvariants asserts `len(seen) == profile.PlateCount` — must still hold).

- [ ] **Step 6: Lint, build, commit**

```
golangci-lint run ./pkg/planetgen/...
go build ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/...
git add pkg/planetgen/field/plates.go pkg/planetgen/cubemap/neighbors.go pkg/planetgen/cubemap/neighbors_symmetry_test.go pkg/planetgen/field/plates_floodfill_diag_test.go
git commit -m "P8: cross-face flood-fill seam fix (item 19 carry-over)"
```

Delete the diagnostic test (`plates_floodfill_diag_test.go`) only if the seam-pair coverage is already adequately captured by the re-enabled `TestPlateFieldSeamMatch` from Task 4. Otherwise keep it as a tighter regression test.

---

## Task 2: Fix per-face JFA seam bug in `computeSDFs`

**Files:**
- Modify: `pkg/planetgen/field/plates.go:348-...` (`computeSDFs`)
- Test gate: `pkg/planetgen/field/plates_seam_test.go::TestPlateFieldSeamMatch` (the SDF subtests)

**Diagnosis.** JFA seeds boundary pixels (one per boundary type) and propagates squared distance via 8-direction passes per power-of-two step. Per `phase-8-seam-bugs.md`, the seam test reports up to 100% of range — exactly consistent with one face having no boundary seeds (defaults to `math.Pi*RadiusKm` sentinel) while the adjacent face has ~0 distance. The fix is making the JFA propagation cross-face-aware.

- [ ] **Step 1: Inspect the current JFA**

Read `computeSDFs` and the per-pass loop. Note whether each pass uses `FacePixelNeighbors4` (for cross-face) or `FacePixelNeighbors8` / a hand-rolled in-face loop. If the latter, the propagation is per-face which is the bug.

- [ ] **Step 2: Write a failing direct test**

`pkg/planetgen/field/plates_sdf_seam_test.go`:

```go
package field

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
)

func TestPlateSDFsContinuousAcrossSeams(t *testing.T) {
    pf := GeneratePlates(planetgen.Profiles["terran"], 1, 64)
    if pf == nil {
        t.Fatal("plates required")
    }
    for name, slc := range map[string][cubemap.NumFaces][]float64{
        "convergent": pf.Convergent,
        "divergent":  pf.Divergent,
        "transform":  pf.Transform,
    } {
        cm := &cubemap.CubeMapF{Size: 64}
        for i := range cm.Faces {
            cm.Faces[i] = slc[i]
        }
        seamtest.AssertSeamContinuity(t, name, cm, 0.02)
    }
}
```

Expected: FAIL.

- [ ] **Step 3: Make JFA cross-face-aware**

In each JFA pass, when sampling a candidate neighbor at offset `(dx, dy)` step `k`, use:

```go
// candidate neighbor pixel at face-aware offset
candX, candY := px+dx*k, py+dy*k
var cFace cubemap.Face
if candX >= 0 && candX < S && candY >= 0 && candY < S {
    cFace = face
} else {
    // project beyond the edge through cube-tangent space, snap on the adjacent face
    cFace, candX, candY = cubemap.OffsetPixel(face, px, py, dx*k, dy*k, S)
}
// read seedDir at (cFace, candX, candY), compute distance, compare
```

If `cubemap.OffsetPixel` doesn't exist, add it to `pkg/planetgen/cubemap/neighbors.go`:

```go
// OffsetPixel returns the pixel at (px+dx, py+dy) on face, projecting
// across face boundaries when the offset leaves the face bounds. Uses
// half-pixel-centered direction snap. dx and dy can be any int.
func OffsetPixel(face Face, px, py, dx, dy, S int) (Face, int, int) {
    nx, ny := px+dx, py+dy
    if nx >= 0 && nx < S && ny >= 0 && ny < S {
        return face, nx, ny
    }
    u := (float64(px) + 0.5 + float64(dx)) / float64(S)
    v := (float64(py) + 0.5 + float64(dy)) / float64(S)
    x, y, z := FaceUVToDir(face, u, v)
    return DirToFacePixel(x, y, z, S)
}
```

JFA stores per-pixel "nearest seed pixel" addresses, not just distances. Make sure the seed-address record holds `(face, px, py)` not just `(px, py)` — otherwise distance is computed on the wrong face. If the existing implementation stores only `(px, py)`, this is a structural change that touches the seed-record type.

- [ ] **Step 4: Verify**

```
go test ./pkg/planetgen/field/ -run TestPlateSDFsContinuousAcrossSeams -v
```

Expected: PASS at 2% tolerance. Also re-run `TestPlateFieldSeamMatch` from Phase 7 once Task 4 unskips it.

- [ ] **Step 5: Sanity-check the values**

```
go test ./pkg/planetgen/field/ -run TestPlateSDFKmRange -v
```

Add a quick sanity test that asserts: max `Convergent` SDF on a multi-plate planet is well below `math.Pi * profile.RadiusKm` (i.e. nearly all pixels found a seed). Pre-fix, ~1/6 of pixels held the sentinel; post-fix, ≤ 1% should.

- [ ] **Step 6: Lint, build, commit**

```
git add pkg/planetgen/field/plates.go pkg/planetgen/cubemap/neighbors.go pkg/planetgen/field/plates_sdf_seam_test.go
git commit -m "P8: cross-face JFA seam fix in computeSDFs (item 19 carry-over)"
```

---

## Task 3: Make `JitterField.TransformPixel` seam-symmetric

**Files:**
- Modify: `pkg/planetgen/noise/jitter.go:165-180` (`TransformPixel`) and possibly `:80-115` (`PerPixel` raster generation)
- Test gate: `pkg/planetgen/noise/jitter_seam_test.go::TestJitteredDetailFieldSeamContinuity`

**Diagnosis.** `TransformPixel(face, px, py, ...)` does an O(1) raster lookup `jf.PerPixel[face][py*S+px]` to find the cell. The raster is generated per-face, so neighboring pixels on adjacent faces may map to different cells (the cell-boundary cuts a face seam). Cell-id discontinuities at seams produce jitter-direction discontinuities → fbm sample-point jumps → 8-47% Detail-field discontinuity per the Phase 7 seam-QA report.

The fix is to compute cell-id from the underlying *direction* rather than the per-face raster. Direction is continuous across cube faces, so cell-id is deterministic regardless of which face you query from.

- [ ] **Step 1: Add a direction-based jitter API**

In `pkg/planetgen/noise/jitter.go`:

```go
// TransformDir is the same as Transform but computes cell-id from the
// query direction directly. Use this anywhere seam continuity matters.
// The PerPixel raster is retained for the "Jitter: cells" debug stage
// only; production sampling should call TransformDir.
func (jf *JitterField) TransformDir(qx, qy, qz, tx, ty, tz float64) (float64, float64, float64) {
    return jf.transformByCell(jf.At(qx, qy, qz), tx, ty, tz)
}
```

`At(dx,dy,dz) *JitterCell` already exists at `jitter.go:121` and uses the direction-based nearest-cell search. So `TransformDir` is a thin renaming that makes the seam-correct path the obvious one to call.

- [ ] **Step 2: Update consumers to call `TransformDir`**

In `pkg/planetgen/field/control.go:GenerateControlFields` (the Detail-field jitter call site), replace:

```go
dx, dy, dz = jitter.TransformPixel(face, px, py, dx, dy, dz)
```

with:

```go
qx, qy, qz := cubemap.FacePixelToDir(face, px, py, S)
dx, dy, dz = jitter.TransformDir(qx, qy, qz, dx, dy, dz)
```

Same in the Whittaker-biome jitter call site (`pkg/planetgen/render/colorize.go` or wherever `jitter.TransformPixel` is called for biome jitter — grep for the second call site).

- [ ] **Step 3: Deprecate `TransformPixel`**

Mark `TransformPixel` with a deprecation comment but keep it in case any future flat-mode resurrection needs the per-face raster. (We deleted the flat path in Phase 7; if it stays gone forever, delete `TransformPixel` instead.)

```go
// Deprecated: TransformPixel uses the per-face PerPixel raster, which
// is discontinuous at cube seams. Use TransformDir for production
// sampling.
```

If golangci-lint flags the deprecation as unused-when-removed, remove it instead.

- [ ] **Step 4: Verify**

```
go test ./pkg/planetgen/noise/ -run TestJitteredDetailFieldSeam -v
```

Re-enable the test by removing the `t.Skip` line in Task 4.

After unskipping, all 8 archetypes should pass at 2% tolerance. If `oceanic` or `arid` is borderline (say 2.0-2.5%), the threshold can be widened to 3% with a comment explaining the small residual fbm-amplitude at the seam.

- [ ] **Step 5: Lint, build, commit**

```
git add pkg/planetgen/noise/jitter.go pkg/planetgen/field/control.go pkg/planetgen/render/colorize.go
git commit -m "P8: direction-based jitter cell lookup (item 19 carry-over)"
```

---

## Task 4: Re-enable Phase 7 seam-QA tests

**Files:**
- Modify: `pkg/planetgen/field/plates_seam_test.go` — remove `t.Skip` line at top of `TestPlateFieldSeamMatch`
- Modify: `pkg/planetgen/noise/jitter_seam_test.go` — remove `t.Skip` line at top of `TestJitteredDetailFieldSeamContinuity`
- Modify: `pkg/planetgen/render/rocky_seam_test.go` — remove `t.Skip` line at top of `TestRockyHeightmapSeamContinuity`

- [ ] **Step 1: Remove all three skips**

Each file has a single `t.Skip(...)` call as the first statement of the test function. Delete it (and the trailing blank line if it leaves one).

- [ ] **Step 2: Run all three**

```
go test ./pkg/planetgen/field/ -run TestPlateFieldSeam -v
go test ./pkg/planetgen/noise/ -run TestJitteredDetail -v
go test ./pkg/planetgen/render/ -run TestRockyHeightmapSeam -v
```

Expected: PASS at default thresholds (1% / 0 / 2%) after Tasks 1-3 land.

If `TestRockyHeightmapSeamContinuity` still fails after Tasks 1-3 because some non-plate, non-jitter stage (e.g. craters, erosion) has its own seam issue, it gets its own task in this plan or `t.Skip` with a precise per-stage diagnosis added to `phase-8-seam-bugs.md`. Do not relax the threshold without filing the bug.

- [ ] **Step 3: Lint, commit**

```
git add pkg/planetgen/field/plates_seam_test.go pkg/planetgen/noise/jitter_seam_test.go pkg/planetgen/render/rocky_seam_test.go
git commit -m "P8: re-enable Phase 7 seam-QA tests (item 19 fully gated)"
```

---

## Task 5: Item 6 rewire — ridged mountain mask via plate convergent SDF

**Files:**
- Modify: `pkg/planetgen/render/rocky.go:143-200` (the ridged-application loop)
- Modify: `pkg/planetgen/types/types.go` — add convergent-SDF mask knobs to `RidgedConfig` (or a new `PlateMaskConfig`)
- Test: `pkg/planetgen/render/rocky_ridged_plate_mask_test.go`

**Design.** Today the ridged-mountain mask reads `cont = continentalness sample at (face,px,py)` and applies `mask = smoothstep(MaskLow, MaskHigh, cont)`. Phase 8 switches to `mask = smoothstep(MaskLow, MaskHigh, plates.Convergent[face][py*S+px] / RadiusKm * scale)`, where the input now ranges over normalized convergent-distance. The intent: mountains form along convergent boundaries, not along arbitrary continental interior.

The exact mapping: `dist_norm = 1 - clamp(plates.Convergent / threshold_km, 0, 1)`, so `dist_norm = 1` exactly at the boundary and decays to 0 at `threshold_km` away. Then `mask = smoothstep(MaskLow, MaskHigh, dist_norm)` — same thresholds as before, just on a different input.

- [ ] **Step 1: Add profile knob**

```go
// in types.go RidgedConfig
PlateConvergentScaleKm float64 `json:"plateConvergentScaleKm,omitempty"`
// 0 → use legacy Continentalness mask (back-compat for archetypes
// without plates: hothouse, ice_world, gas giants).
// >0 → mask is built from plates.Convergent SDF; smoothstep input is
// 1 - clamp(dist/PlateConvergentScaleKm, 0, 1).
```

Per-archetype defaults in `pkg/planetgen/profile.go`:
- terran/super_terran: 800 km (typical mountain-belt half-width)
- oceanic: 600 km
- tundra: 700 km
- arid/glacial: 600 km
- scorched/lava_world: 400 km
- ice_world/jovian/ice_giant/unknown: 0 (no plates → keep continentalness path)

- [ ] **Step 2: Plumb plates into the ridged stage**

`RenderRocky` already calls `field.GeneratePlates(...)` and discards the result (`_ = plates`). Pass `plates` into the ridged stage. If `plates == nil` or `RidgedConfig.PlateConvergentScaleKm == 0`, fall back to the existing Continentalness mask. Otherwise:

```go
if plates != nil && profile.Ridged.PlateConvergentScaleKm > 0 {
    distKm := plates.Convergent[face][py*S+px]
    distNorm := 1.0 - clamp01(distKm / profile.Ridged.PlateConvergentScaleKm)
    mask := smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, distNorm)
    // ... ridged contribution as before
}
```

- [ ] **Step 3: Test**

```go
func TestRidgedMaskUsesPlateConvergent(t *testing.T) {
    profile := *planetgen.Profiles["terran"]
    profile.Ridged.PlateConvergentScaleKm = 800
    cm := render.RenderRocky(&profile, 42, 64)
    // Heuristic: pixels within ~100 km of a convergent boundary should
    // have substantially higher heightmap variance than pixels >2000 km
    // away. Compare the histogram of heights for the two pixel sets.
    // (Concrete assertion: variance ratio > 1.5)
    ...
}
```

- [ ] **Step 4: Lint, commit**

```
git add pkg/planetgen/types/types.go pkg/planetgen/profile.go pkg/planetgen/render/rocky.go pkg/planetgen/render/rocky_ridged_plate_mask_test.go
git commit -m "P8: ridged mountain mask via plate convergent SDF (item 6 rewire)"
```

---

## Task 6: Item 10 rewire — Voronoi continents from plate centroids

**Files:**
- Modify: `pkg/planetgen/field/continents.go` — accept an optional seed-direction list
- Modify: `pkg/planetgen/render/rocky.go:285-300` (the continents call site) — pass plate centroids
- Test: `pkg/planetgen/field/continents_plate_seed_test.go`

**Design.** Today `GenerateContinents` picks `Continents.Seeds` random unit vectors. Phase 8 lets the caller supply seeds from `PlateField`: for each plate where `IsOceanic == false`, take the plate `Seed` direction (Voronoi center on the sphere). This means continents align with continental-plate cores, which is geologically correct.

Number of continental-plate seeds varies per archetype (terran has ~3-4 of 12 plates continental, arid has ~4-5 of 6, oceanic has ~1 of 12). If the count is below `profile.Continents.Seeds`, top up with the existing fbm-of-Continentalness seeding for the residual (so a non-zero `Continents.Seeds` always produces at least N seeds).

- [ ] **Step 1: Extend `GenerateContinents` API**

```go
// GenerateContinentsFromSeeds takes an explicit seed-direction list.
// Use this when plate centroids drive continent placement (Phase 8).
// Pass nil seeds to fall back to the legacy random-seed flow.
func GenerateContinentsFromSeeds(masterSeed int64, cfg types.ContinentConfig, S int, seeds [][3]float64) *cubemap.CubeMapF
```

Existing `GenerateContinents(...)` becomes a thin wrapper that calls `GenerateContinentsFromSeeds(..., nil)`.

- [ ] **Step 2: Wire plate centroids in `RenderRocky`**

```go
var contSeeds [][3]float64
if plates != nil {
    for _, p := range plates.Plates {
        if !p.IsOceanic {
            contSeeds = append(contSeeds, p.Seed)
        }
    }
}
cont := field.GenerateContinentsFromSeeds(seed, profile.Continents, S, contSeeds)
```

- [ ] **Step 3: Test**

```go
func TestContinentsAlignWithContinentalPlates(t *testing.T) {
    profile := planetgen.Profiles["terran"]
    pf := field.GeneratePlates(profile, 42, 64)
    cont := field.GenerateContinentsFromSeeds(42, profile.Continents, 64, plateContinentalSeeds(pf))
    // For each continental plate seed dir, the Continents field at that
    // dir should be a "land" voronoi-region (above 0.5 threshold).
    for _, p := range pf.Plates {
        if p.IsOceanic { continue }
        f, px, py := cubemap.DirToFacePixel(p.Seed[0], p.Seed[1], p.Seed[2], 64)
        if cont.Faces[f][py*64+px] < 0.5 {
            t.Errorf("continental plate seed not in land region")
        }
    }
}
```

- [ ] **Step 4: Lint, commit**

```
git add pkg/planetgen/field/continents.go pkg/planetgen/render/rocky.go pkg/planetgen/field/continents_plate_seed_test.go
git commit -m "P8: Voronoi continents seeded from plate centroids (item 10 rewire)"
```

---

## Task 7: D8 flow accumulation + rivers (item 15)

**Files:**
- Create: `pkg/planetgen/field/flow.go`
- Create: `pkg/planetgen/field/flow_test.go`
- Modify: `pkg/planetgen/types/types.go` — `FlowConfig`
- Modify: `pkg/planetgen/profile.go` — per-archetype defaults

**Design.** Standard D8: at each pixel, find the 8-connected neighbor with the steepest descent; accumulate upstream flow following pointer chains. River mask = pixels where accumulated flow > threshold. Heightmap is carved (multiplied by `1 - riverDepth * riverMask`) along river pixels.

**Cube-map seam handling.** D8 neighbors are 8-connected. Use a new helper `cubemap.FacePixelNeighbors8` (extend the existing 4-neighbor helper) so off-edge neighbors map to the adjacent face. Required because river paths cross cube seams routinely. The Phase 7 seam fixes from Tasks 1-2 are direct prerequisites: D8 propagation across a seam relies on the same `FacePixelNeighbors4`/`OffsetPixel` symmetry.

**Planchon-Darboux fill** runs first to remove pit artefacts that would otherwise sink rivers into local minima. Standard sequence:

1. Initialize a "filled" heightmap to `+∞` everywhere except the open boundary (here: every pixel is interior on a sphere — there is no boundary, so seed `filled = original` at all pixels and let the sphere drain to itself; the Planchon-Darboux phase becomes "raise local minima to the lowest-saddle elevation" which is computed via successive relaxation passes).
2. Sweep until convergence: for each pixel, `filled[p] = max(original[p], min over 8-neighbors of filled[n])`. Iterate until no pixel changes.

This is O(passes × pixels). Typical convergence in 4-8 passes at S=512.

- [ ] **Step 1: Add 8-neighbor helper**

`pkg/planetgen/cubemap/neighbors.go`:

```go
// FacePixelNeighbors8 returns the eight 8-connected neighbors of
// (face, px, py) at face size S, including diagonals. Off-edge
// neighbors are mapped to the adjacent face via OffsetPixel.
func FacePixelNeighbors8(face Face, px, py, S int) [8]PixelAddr {
    deltas := [8][2]int{
        {1, 0}, {-1, 0}, {0, 1}, {0, -1},
        {1, 1}, {-1, 1}, {1, -1}, {-1, -1},
    }
    var out [8]PixelAddr
    for i, d := range deltas {
        f, nx, ny := OffsetPixel(face, px, py, d[0], d[1], S)
        out[i] = PixelAddr{Face: f, PX: nx, PY: ny}
    }
    return out
}
```

- [ ] **Step 2: Implement `flow.go`**

Sketch:

```go
package field

import (
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// FlowField holds per-pixel D8 pointer + accumulated upstream flow.
// River mask is derived from accumulation > FlowConfig.RiverThreshold.
type FlowField struct {
    Size       int
    D8         [cubemap.NumFaces][]int8       // 0..7 neighbor index, -1 = sink
    Accum      [cubemap.NumFaces][]float64    // upstream flow count
    Rivers     [cubemap.NumFaces][]bool       // accum > threshold
}

// GenerateFlow computes Planchon-Darboux fill, D8 pointers,
// accumulation, and the river mask from a heightmap. Returns nil if
// FlowConfig.RiverThreshold == 0 (rivers disabled).
func GenerateFlow(heightmap *cubemap.CubeMapF, cfg types.FlowConfig) *FlowField {
    if cfg.RiverThreshold <= 0 {
        return nil
    }
    S := heightmap.Size
    filled := planchonDarbouxFill(heightmap)
    ff := &FlowField{Size: S}
    for f := range ff.D8 {
        ff.D8[f] = make([]int8, S*S)
        ff.Accum[f] = make([]float64, S*S)
        ff.Rivers[f] = make([]bool, S*S)
    }
    computeD8Pointers(filled, ff)
    computeAccumulation(ff)
    for f := range ff.Rivers {
        for i, a := range ff.Accum[f] {
            ff.Rivers[f][i] = a >= cfg.RiverThreshold
        }
    }
    return ff
}

// CarveRivers writes river channels into the heightmap by subtracting
// cfg.RiverDepth where ff.Rivers is set. Operates in place.
func CarveRivers(heightmap *cubemap.CubeMapF, ff *FlowField, cfg types.FlowConfig) {
    if ff == nil { return }
    S := heightmap.Size
    for f := range heightmap.Faces {
        for i := range heightmap.Faces[f] {
            if ff.Rivers[f][i] {
                heightmap.Faces[f][i] -= cfg.RiverDepth
            }
            _ = S
        }
    }
}
```

Inner functions (`planchonDarbouxFill`, `computeD8Pointers`, `computeAccumulation`) follow standard textbook implementations; the only project-specific bit is using `FacePixelNeighbors8` for cube-map seam-aware neighbor walks.

`computeAccumulation` uses topological sort by elevation (descending), so an upstream pixel is processed before its downstream neighbor's accumulation is finalized. With ~6·S² pixels at S=512 this is a sort of ~1.6M elements; manageable.

- [ ] **Step 3: `FlowConfig` profile knobs**

```go
type FlowConfig struct {
    RiverThreshold float64 `json:"riverThreshold,omitempty"` // accum cutoff for "is river"
    RiverDepth     float64 `json:"riverDepth,omitempty"`     // height units carved
}
```

Defaults:
- terran: `{Threshold: 200, Depth: 0.02}`
- super_terran: `{Threshold: 250, Depth: 0.025}`
- tundra: `{Threshold: 300, Depth: 0.015}` (sparser drainage)
- arid: `{Threshold: 1500, Depth: 0.005}` (very sparse, dry valleys)
- glacial: `{Threshold: 400, Depth: 0.018}`
- scorched/lava_world/ice_world/oceanic/hothouse/jovian/ice_giant/unknown: `{Threshold: 0}` (disabled)

- [ ] **Step 4: Tests**

```go
func TestFlowDeterministic(t *testing.T) {
    h1 := someHeightmap(42, 64)
    h2 := someHeightmap(42, 64)
    ff1 := field.GenerateFlow(h1, cfg)
    ff2 := field.GenerateFlow(h2, cfg)
    // FlowField byte-equal between runs
}

func TestFlowRiverPixelsFormConnectedComponents(t *testing.T) {
    // For terran heightmap at S=64: river mask, when traversed via
    // 8-neighbors with face hops, should form O(N continents) connected
    // components (i.e. each continent has one or more river systems).
}

func TestFlowSeamContinuity(t *testing.T) {
    // The Accum field must be seam-continuous within 5% of range
    // (rivers are spike-y so the threshold is wider than other fields).
}
```

- [ ] **Step 5: Lint, build, commit**

```
git add pkg/planetgen/field/flow.go pkg/planetgen/field/flow_test.go pkg/planetgen/cubemap/neighbors.go pkg/planetgen/types/types.go pkg/planetgen/profile.go
git commit -m "P8: D8 flow + Planchon-Darboux + rivers (item 15)"
```

---

## Task 8: Wind-driven rain shadow (item 16)

**Files:**
- Create: `pkg/planetgen/biome/rainshadow.go`
- Create: `pkg/planetgen/biome/rainshadow_test.go`
- Modify: `pkg/planetgen/types/types.go` — `RainShadowConfig`

**Design.** For each pixel, compute the prevailing-wind tangent vector at that latitude (e.g. easterlies in tropics, westerlies in mid-latitudes; one full Hadley/Ferrel/Polar cell pair per hemisphere). Walk N steps along the wind tangent on the great-circle path; at each step, sample the heightmap. If you encounter a peak above `MountainCutoff`, record the upwind-uplift event. After the walk, compute moisture multiplier:

- Upwind of peak: `M *= 1 + windRain` (orographic precipitation enhancement)
- Lee of peak: `M *= 0.15` (rain shadow)
- Otherwise: unchanged

Output is a per-pixel multiplier on Whittaker M (the humidity dimension of the biome lookup).

- [ ] **Step 1: Implement `rainshadow.go`**

```go
package biome

import (
    "math"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// RainShadowField holds a per-pixel multiplier for the humidity input
// to the Whittaker biome lookup.
type RainShadowField struct {
    Size       int
    Multiplier [cubemap.NumFaces][]float64
}

func GenerateRainShadow(heightmap *cubemap.CubeMapF, cfg types.RainShadowConfig) *RainShadowField {
    if cfg.WalkSteps == 0 { return nil }
    S := heightmap.Size
    out := &RainShadowField{Size: S}
    for f := range out.Multiplier {
        out.Multiplier[f] = make([]float64, S*S)
        for i := range out.Multiplier[f] { out.Multiplier[f][i] = 1.0 }
    }
    for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
        for py := 0; py < S; py++ {
            for px := 0; px < S; px++ {
                dx, dy, dz := cubemap.FacePixelToDir(f, px, py, S)
                lat := math.Asin(dy)
                wind := prevailingWindTangent(dx, dy, dz, lat)
                m := walkUpwindForOrography(heightmap, dx, dy, dz, wind, cfg, S)
                out.Multiplier[f][py*S+px] = m
            }
        }
    }
    return out
}
```

`prevailingWindTangent(dir, lat)` returns a unit tangent vector based on a piecewise function of latitude (trade winds 0–30°N/S, westerlies 30–60°, polar easterlies 60–90°). `walkUpwindForOrography` steps N times along the great-circle in the wind direction, sampling the heightmap, and returns the moisture multiplier per the rule above.

Compute great-circle steps via spherical lerp at angular step `cfg.StepArcRad ≈ π/(180·5)` (5°/step ≈ 555 km on Earth scale).

- [ ] **Step 2: Profile knobs**

```go
type RainShadowConfig struct {
    WalkSteps      int     `json:"walkSteps,omitempty"`      // 0 = disabled
    StepArcRad     float64 `json:"stepArcRad,omitempty"`     // typical 0.087 (5°)
    MountainCutoff float64 `json:"mountainCutoff,omitempty"` // height threshold for orography
    WindRainBoost  float64 `json:"windRainBoost,omitempty"`  // upwind multiplier minus 1
    LeeFactor      float64 `json:"leeFactor,omitempty"`      // typical 0.15
}
```

Defaults:
- terran/super_terran: `{Steps: 12, StepArc: 0.087, Cutoff: 0.65, Boost: 0.4, Lee: 0.15}`
- arid: `{Steps: 12, Cutoff: 0.55, Boost: 0.2, Lee: 0.05}`
- tundra: `{Steps: 8, Cutoff: 0.7, Boost: 0.3, Lee: 0.2}`
- glacial: `{Steps: 6, Cutoff: 0.7, Boost: 0.25, Lee: 0.25}`
- oceanic: `{Steps: 0}` (disabled — too few mountains to matter)
- scorched/lava_world/ice_world/hothouse/jovian/ice_giant/unknown: disabled

- [ ] **Step 3: Tests**

```go
func TestRainShadowProducesAsymmetry(t *testing.T) {
    // Construct a synthetic heightmap with a single ridge running N-S
    // at 0° longitude. With easterly winds at the equator, the rain
    // shadow should appear west of the ridge.
    // Assert: Multiplier[westside] < 0.3, Multiplier[eastside] > 1.2.
}

func TestRainShadowDeterministic(t *testing.T) { /* byte-equal */ }

func TestRainShadowSeamContinuity(t *testing.T) {
    // 5% threshold (rain shadow is spatially noisy by nature)
}
```

- [ ] **Step 4: Lint, commit**

```
git add pkg/planetgen/biome/rainshadow.go pkg/planetgen/biome/rainshadow_test.go pkg/planetgen/types/types.go pkg/planetgen/profile.go
git commit -m "P8: wind-driven rain shadow on Whittaker M (item 16)"
```

---

## Task 9: Wire flow + rain shadow into rocky pipeline

**Files:**
- Modify: `pkg/planetgen/render/rocky.go` — add carve-rivers stage between erosion and craters; apply rain-shadow multiplier in the colorize/biome lookup.
- Modify: `pkg/planetgen/render/debug.go` — register `Flow: rivers`, `Flow: accum`, `RainShadow: multiplier` debug stages.

- [ ] **Step 1: Insert flow stage**

After erosion smoothing, before crater stamping, in `RenderRocky`:

```go
ff := field.GenerateFlow(heightmap, profile.Flow)
field.CarveRivers(heightmap, ff, profile.Flow)
```

- [ ] **Step 2: Insert rain-shadow stage**

After heightmap is finalized, before colorize:

```go
rs := biome.GenerateRainShadow(heightmap, profile.RainShadow)
// In the colorize loop:
m := humidityAt(face, px, py)
if rs != nil { m *= rs.Multiplier[face][py*S+px] }
// then look up biome with adjusted m
```

- [ ] **Step 3: Debug stages**

Mirror the Phase 7 plate debug stages:

```go
if ff != nil {
    frame.Stages = append(frame.Stages,
        DebugStage{Name: "Flow: rivers", Kind: "field", BooleanAfter: paintBooleanCubeMap(ff.Rivers, S)},
        DebugStage{Name: "Flow: accum", Kind: "field", ScalarAfter: scalarFromAccum(ff.Accum, S)},
    )
}
if rs != nil {
    frame.Stages = append(frame.Stages,
        DebugStage{Name: "RainShadow: multiplier", Kind: "field", ScalarAfter: scalarFromMultiplier(rs.Multiplier, S)},
    )
}
```

- [ ] **Step 4: Smoke test in planet-explorer**

Build wasm, render terran, switch to each new debug stage. Confirm rivers visibly trace continental drainage; rain-shadow multiplier shows asymmetry at ridges.

- [ ] **Step 5: Lint, build, commit**

```
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/
git add pkg/planetgen/render/rocky.go pkg/planetgen/render/debug.go
git commit -m "P8: wire D8 rivers + rain shadow into rocky pipeline"
```

---

## Task 10: Statistical invariants for items 6/10/15/16

**File:** `cmd/generate-planet-maps/invariants_test.go`

- [ ] **Step 1: Write invariants**

```go
func TestPhase8RidgedMaskTracksConvergent(t *testing.T) {
    // For terran/super_terran: pixels close to a convergent boundary
    // (Convergent < 200 km) have mean heightmap value > pixels far
    // from any convergent boundary (Convergent > 2000 km). Variance
    // ratio assertion as in Task 5.
}

func TestPhase8ContinentsCenteredOnContinentalPlates(t *testing.T) {
    // For terran: every continental-plate seed direction lands inside
    // a "land" voronoi region (Continents value > 0.5).
}

func TestPhase8RiverMaskCoverage(t *testing.T) {
    // For each archetype with FlowConfig.RiverThreshold > 0: river
    // pixels are 0.5%–5% of total surface area. Below 0.5% = threshold
    // too high; above 5% = threshold too low or carving runaway.
}

func TestPhase8RainShadowAsymmetry(t *testing.T) {
    // For terran: variance of Multiplier across pixels > 0.05 (i.e.
    // the field is non-trivial — at least some lee/wind asymmetry
    // present).
}
```

- [ ] **Step 2: Run, lint, commit**

```
go test ./cmd/generate-planet-maps/ -run TestPhase8 -v
git add cmd/generate-planet-maps/invariants_test.go
git commit -m "P8: statistical invariants for items 6/10/15/16"
```

---

## Task 11: Goldens re-bake

Single re-bake of the curated 13-planet golden set. Phase 8 changes ridged-mask source, continent placement, adds rivers, and adds rain shadow → all 10 rocky archetypes will diff. ΔE2000 expected 4.0–8.0 (larger than Phase 7's 1.5–4.0 because the changes are structural, not just texture).

- [ ] **Step 1: Confirm clean state**

```
go build ./...
go test ./pkg/planetgen/...
golangci-lint run ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/
```

- [ ] **Step 2: Re-bake**

```
go test -timeout 60m ./cmd/generate-planet-maps/ -run TestGolden -update
```

(Allow 60 min; rivers add a Planchon-Darboux fill at face=1024.)

- [ ] **Step 3: Determinism check**

```
go test -timeout 60m ./cmd/generate-planet-maps/ -run TestGolden
```

Expected: PASS with zero diff.

- [ ] **Step 4: Visual review**

```
go run ./cmd/tools/planet-image-diff -- terran
go run ./cmd/tools/planet-image-diff -- arid
```

Confirm: mountain belts now align with plate convergent zones; rivers visible on terran/tundra/super_terran; arid shows rain-shadow color shifts.

- [ ] **Step 5: Commit goldens**

```
git add cmd/generate-planet-maps/testdata/golden/
git commit -m "P8: goldens re-bake — items 6/10/15/16"
```

---

## Task 12: README + memory note

- [ ] **Step 1: Append to `cmd/generate-planet-maps/README.md`**

```markdown
## Phase 8 — Tier B geology (master-plan items 6 rewire, 10 rewire, 15, 16)

**Rivers** (`pkg/planetgen/field/flow.go`). Planchon-Darboux fill + D8 flow accumulation + per-archetype river-threshold mask. Carved into heightmap as channels (`RiverDepth` height units). Cube-seam-aware via `cubemap.FacePixelNeighbors8`.

Profile knobs: `Flow.RiverThreshold` (accumulation cutoff; 0 disables), `Flow.RiverDepth` (carved depth in normalized height units).

**Rain shadow** (`pkg/planetgen/biome/rainshadow.go`). Latitude-banded prevailing wind + N-step great-circle walk on the heightmap + orographic boost upwind / lee multiplier downwind of mountains. Output is a per-pixel multiplier on Whittaker humidity.

Profile knobs: `RainShadow.WalkSteps` (0 disables), `StepArcRad`, `MountainCutoff`, `WindRainBoost`, `LeeFactor`.

**Item 6 rewire.** Ridged mountain mask now reads `plates.Convergent` SDF instead of fbm-of-Continentalness. New knob `Ridged.PlateConvergentScaleKm` (0 keeps legacy mask).

**Item 10 rewire.** `field.GenerateContinentsFromSeeds` accepts a list of seed directions; rocky pipeline passes plate centroids of continental plates. Falls back to legacy random seeds when plates absent.

**Phase 7 seam-bug fixes (items 19 carry-over).** All three skipped seam-QA tests now pass:
- `floodFillPlates` — symmetric cross-face neighbor walk + lex-tiebreak seam-pin pass.
- `computeSDFs` — JFA propagates across face seams via `cubemap.OffsetPixel`.
- `JitterField` — production sampling uses direction-based `TransformDir` instead of per-face raster.
```

- [ ] **Step 2: Update memory note**

`/home/robert/.claude/projects/-home-robert-spacemolt-kb/memory/project_planet_gen.md` — append a `## Phase 8 status` section before "Future work":

```markdown
## Phase 8 status

**Tier B Phase 8 complete** as of <date>:
- Items 6 + 10 (retroactive rewires): ridged mask reads plate convergent SDF; continents seeded from continental-plate centroids.
- Item 15: D8 flow + Planchon-Darboux + rivers (`pkg/planetgen/field/flow.go`). Cube-seam-aware via new `cubemap.FacePixelNeighbors8` + `OffsetPixel`.
- Item 16: wind-driven rain shadow (`pkg/planetgen/biome/rainshadow.go`).
- Item 19 carry-over: Phase 7 seam-QA tests fully passing — all three bugs from `phase-8-seam-bugs.md` fixed.

**Phase 8 deferred to Phase 9**: items 17 (clouds) + 18 (civilization signs).

**New per-archetype defaults**: see `pkg/planetgen/profile.go` for `Flow`, `RainShadow`, `Ridged.PlateConvergentScaleKm` rows.
```

- [ ] **Step 3: Commit**

```
git add cmd/generate-planet-maps/README.md
git commit -m "P8: README + memory note"
```

---

## Acceptance gates (run all after Task 12)

```
go build ./...
go test ./pkg/planetgen/... ./cmd/generate-planet-maps/...
golangci-lint run ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/
go test -run TestGolden ./cmd/generate-planet-maps/   # determinism: must pass without -update
```

All green. Then:

1. Phase 7 seam-QA tests pass at default thresholds (1% / 0 / 2%).
2. Phase 8 statistical invariants pass.
3. Goldens re-baked; second-pass `go test` with no `-update` produces no diff.
4. Planet-explorer renders the 3 new debug stages (`Flow: rivers`, `Flow: accum`, `RainShadow: multiplier`) on every rocky archetype with the stage enabled, and gracefully skips them on archetypes with the relevant config disabled.
5. Profile JSON serialization round-trip passes for the new knobs (`Flow`, `RainShadow`, `Ridged.PlateConvergentScaleKm`).
6. README and memory note updated.
7. Branch ready to push.

---

## Risks

- **Seam-bug fixes change plate-id distribution.** Task 1's seam-pin pass reassigns ≤ 12·S pixels (1.5% at S=64, 0.4% at S=512). `TestPhase7PlateInvariants` only checks count and nonempty-per-plate — should still pass. If it doesn't, adjust the tiebreak to favor the larger plate to preserve count.
- **D8 with 8-neighbor cross-face walk is the most complex new code.** Determinism risk via map iteration order in topological-sort step. Mitigation: sort by elevation-then-pixel-address, deterministic.
- **Rain shadow walk is O(pixels × WalkSteps)** — at S=512, WalkSteps=12, that's 19M heightmap samples per planet. Profile early; if too slow, downsample the heightmap to S=256 for the rain-shadow walk and upsample the multiplier (acceptable because rain shadow is naturally smooth on the multi-100-km scale).
- **Item 6 rewire risks regression on archetypes without plates** (hothouse, ice_world, gas giants). The fallback to legacy Continentalness mask kicks in via `PlateConvergentScaleKm == 0`; verify in Task 5 that those archetypes produce unchanged heightmaps in the goldens-diff stage.
- **Goldens diff scope.** Item 6/10 alone re-shape every rocky planet's mountain layout. Visual review in Task 11 should focus on "is the new shape geologically plausible" rather than "does it match the old one" — they should not.

---

## Out-of-scope follow-ups (Phase 9)

- Item 17 — Cloud-layer overlay (separate alpha cube-map).
- Item 18 — Civilization signs (habitability scoring + Bridson Poisson + Black Marble nightside).
- Tier C polish (items 21–25).
