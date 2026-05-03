# Phase 8 — Seam-continuity bugs surfaced by Phase 7 seam-QA tests

The Task 13 seam-QA tests (`pkg/planetgen/{field,noise,render}/*_seam_test.go`) are
landed but `t.Skip`-marked. They start passing as the bugs below are fixed.

## 1. `field.floodFillPlates` — cross-face PlateID mismatches at seams
- **Symptom:** `TestPlateFieldSeamMatch` reports 20–72 categorical mismatches per archetype at S=64.
- **Hypothesis:** the BFS frontier is per-face; queue does not push across face boundaries, so neighbouring faces re-seed independently and pick different ids near the seam.
- **Fix sketch:** when a flood reaches an edge pixel, also enqueue the matched-pair pixel on the adjacent face via `cubemap.FacePixelToDir` → `DirToFacePixel`.

## 2. `field.computeSDFs` — JFA per-face produces 100%-of-range seam deltas
- **Symptom:** `convergent`/`divergent`/`transform` continuity reports up to 100.00% of range on terran/super_terran/oceanic.
- **Hypothesis:** JFA seeds are local to each face. Faces without an adjacency boundary fall back to the `math.Pi * RadiusKm` (~20015 km) sentinel while neighbouring faces have ~0 nearby distance.
- **Fix sketch:** seed JFA with cross-face neighbour pixels at iteration 0, or run a second pass that pulls in min-distance from `seamtest.WalkSeams` neighbours before final write-out.

## 3. `noise.JitterField.TransformPixel` — non-symmetric across seams
- **Symptom:** `TestJitteredDetailFieldSeamContinuity` reports 8.7–46.6% Detail-field discontinuity per archetype.
- **Hypothesis:** at edge pixel `e` on face F, `TransformPixel` returns `(dx,dy,dz)` that depends on F's cell-id raster; the matched pixel on the adjacent face uses a different cell, so the displaced direction differs slightly. Compounded over fBm sampling this becomes a visible seam.
- **Fix sketch:** sample jitter cell-id on the underlying *direction* (continuous on the sphere) rather than the per-face raster — i.e. compute `(face,px,py) → dir → cell-id` instead of `(face,px,py) → raster lookup`.

All three are caught by Task 12's `seamtest` helpers, so re-enabling the skipped tests is the regression gate for the fixes.
