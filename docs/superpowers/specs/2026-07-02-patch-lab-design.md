# Phase 13 — Patch Lab: Sphere Tectonics → Flat-Patch Interactive Dev View

**Date:** 2026-07-02 (implemented 2026-07-04)
**Status:** Implemented — Tasks 1-18 built `pkg/planetgen/patch/`, wasm exports, and the
Patch Lab explorer UI against this spec; key decisions confirmed by user 2026-07-03
(merge-not-supersede, civ-growth-animation-as-stretch, S_tect=256 / S_prod=1024 / patch 512²)
**Branch:** phase-13/progressive-layers (needs re-point onto phase-0/cube-map, see §10)
**Supersedes/absorbs:** `2026-06-14-progressive-layers-spec.md` (whole-sphere wizard) — see §2

## 1. Problem

Tuning the rocky pipeline requires full-sphere renders: minutes per render at S=1024,
~7 min for the face-128 test suite. Downstream layers (erosion, rivers, waterlines,
civ) cannot be tuned interactively. The old flat/swatch path was removed
(commit `966f17131`, 2026-05-03) because it kept re-rendering every phase and layer
at full size, making the explorer page unusable — and it also bypassed sphere-global
stages (plates, cratons, JFA coastal distance), so what it did show wasn't faithful.
There is currently no fast-but-faithful iteration loop. Patch Lab addresses both
failure modes directly: renders are bounded to a 512² patch, and per-layer caching
with dirty tracking means a slider change re-renders only the affected layer suffix,
never the whole stack at full size.

## 2. Goal

A **Patch Lab** mode in planet-explorer:

1. Compute tectonics (plates + cratons + FX classification) **on the sphere** at
   modest face size (S_tect, default 256).
2. Extract a **512×512 flat patch** — a window of one cube face at virtual
   production resolution (S_prod, default 1024) — either at 0°/0° or smart-picked
   to maximize plate/craton boundary interactions.
3. Run all downstream layers **on the patch, layer by layer**, with near-realtime
   sliders: tectonic FX, fBm control fields, a couple of craters, dozens of erosion
   droplets, rivers, cryosphere/snowline/ocean waterlines, then civ (prove roads
   read as roads).
4. A final **"Go!"** button runs the full sphere at production resolution with the
   tuned profile (the existing render path, unchanged).
5. Side goal: fixed seed + per-layer patch snapshots = cheap deterministic
   per-stage diff tests (byte-exact, sharper than the ΔE2000 golden gate).

**Relation to the 2026-06-14 spec** (confirmed — "merge: patch-first,
layers as substrate"): the layer decomposition, dependency/dirty-tracking model,
and per-planet-type template ideas from the June spec are reused, retargeted at
the patch. The whole-sphere wizard walkthrough UX is dropped for this phase; the
sphere path stays the monolithic production render behind "Go!". The June PoC code
(`pkg/planetgen/layers/`, `render/tectonic_preview.go`, commit `ec4b28257`) predates
Phase 12's real APIs and is treated as reference only, not a base.

**Non-goals:** whole-sphere wizard; cross-face patches; undo/redo; per-layer
save/load; time-stepped tectonic simulation.

## 3. Approaches considered

- **A. Patch-first, layer substrate (chosen):** new `pkg/planetgen/patch` package;
  sphere computes only what is truly global; everything downstream runs on a flat
  512² grid with per-pixel sphere directions so noise sampling is identical to
  production. Near-realtime sliders; the patch is a true crop of the final planet
  (modulo documented approximations, §7).
- **B. Whole-sphere walkthrough at low face size (June spec as-is):** no new patch
  machinery, but face 128 is too coarse for the stated goals (roads readable,
  single-pixel rivers) and face 512+ is not near-realtime. Rejected.
- **C. Standalone patch CLI (no explorer work):** cheapest, but no interactive
  sliders — misses the point of the phase. Rejected.

## 4. Architecture

```
            sphere (S_tect=256)                    patch (512² @ virtual S_prod=1024)
  GeneratePlates ─► GenerateCrust ─► ClassifyTectonics
        │                │                 │
        ▼                ▼                 ▼
   PlateField       CrustField       TectonicFXField          ┌─ per-layer cache
        └────────────┬───┴──────────────┘                     │
                     ▼                                        ▼
              patch.Extract(...)  ──────────────►  patch.Stack.RenderTo(layerK)
              (bilinear crop at patch dirs)                   │
                     ▲                                        ▼
              patch.Pick(...) (window scoring)         explorer Patch Lab UI
                                                       (wasm exports, sliders)
                     "Go!" ─► existing planetgen.Generate at S_prod (unchanged)
```

### 4.1 Patch geometry

- A patch is `(face, cx, cy, size=512, sProd=1024)`: an axis-aligned window on one
  cube face at virtual resolution S_prod. The window must fit inside the face
  (centers constrained to the interior margin; with 6 faces this leaves plenty of
  candidates).
- Per-pixel unit sphere directions are computed exactly as the cube render path
  computes them for face pixels at S_prod. All 3D noise (control fields, coastal
  noise, jitter, crater placement hashing) samples at these directions —
  **byte-identical inputs to the production render**.

### 4.2 Patch extraction contract (what crosses sphere → patch)

Scalars / metadata:
- Master seed, profile snapshot, RadiusKm.
- Resolved crust params: Assembly, LandFraction, TectonicAge (`ResolveCrustParams`).
- `[]Craton` (centers/radii/plate ids — for debug overlay and any craton-aware FX).
- **Sea level**: derived on the sphere by histogram quantile from
  TargetLandFraction at S_tect and passed in as a scalar. A patch-local quantile
  would be wrong (the patch is not a representative sample of the globe).
- Normalize mapping: the global min/max (affine) from the sphere-side heightmap
  normalization at S_tect, applied as-is on the patch (a patch-local normalize
  would also be wrong).

Per-pixel fields, sampled at patch dirs from the S_tect sphere fields:
- `CrustField.ContinentalMask`, `CrustField.BaseHeight` — bilinear.
- `TectonicFXField` — all five Dist/Mag pairs (Belt, Subd, Arc, Ridge, Rift) — bilinear
  (SDFs and JFA-propagated magnitudes are smooth; bilinear upsampling is safe).
- `PlateField.PlateID` — nearest-neighbor (categorical), debug overlay only.
- `PlateField` Convergent/Divergent/Transform SDFs + Mags — bilinear (debug +
  any mask consumer).

### 4.3 Smart patch picking

`patch.Pick(pf, crust, fx, sTect)` scores candidate window centers (grid stride
across all 6 faces, window footprint projected to S_tect):

```
score = Σ_class present(class)·w_class      # distinct FX classes in window
      + boundary-pixel count in window       # lots of active boundary
      + craton-edge presence                 # continental margin interest
      + has both land and ocean              # waterline layers need a coast
```

Deterministic given seed. UI offers: smart-pick (default), "next best" cycling
through the top-N ranked windows, 0°/0° preset, and manual pin (face + center).
Fully-oceanic degenerate cases still return the argmax (score is always defined).

### 4.4 Layer stack on the patch

Adapted from the June spec: same registry/dirty-tracking idea, but layers operate
on a flat `patch.Field` (512² float64) + a `patch.Context` holding the extracted
sphere fields. Changing a param marks its owning layer dirty; re-render runs from
the dirtiest layer to the current view layer, using cached upstream results.
At 512², every layer except erosion is single-digit-ms; erosion with dozens–low
thousands of droplets is comfortably interactive.

Patch layer order (mirrors the production rocky crust-path stage order):

| # | Layer | Source | Notes |
|---|-------|--------|-------|
| 0 | tectonic-base | crop | BaseHeight init + ContinentalMask; debug: plate/craton overlay |
| 1 | tectonic-fx | patch | `ApplyTectonicFX` on patch using cropped Dist/Mag; all FX params + TectonicAge are sliders |
| 2 | control-noise | patch | Detail + PeaksValleys fBm at patch dirs, splines, summed into heightmap |
| 3 | height-smooth | patch | box blur, square kernel (edge-clamped); matches `field.SmoothHeightmap` exactly — not a disc |
| 4 | normalize | crop | applies the sphere-derived affine, not patch-local min/max |
| 5 | coastal | patch | coastal noise; distance-to-coast computed patch-locally (§7) |
| 6 | erosion | patch | droplet erosion, open-edge policy (§4.5) |
| 7 | craters | patch | a couple of craters placed within the patch |
| 8 | flow-rivers | patch | D8 + Planchon–Darboux with patch edges as drains; river mask |
| 9 | climate | patch | Temperature/Humidity fBm at dirs + rain shadow (patch-local walk, §7) |
| 10 | biome-color | patch | Whittaker lookup + palette |
| 11 | waterlines | patch | ocean level, snowline/cryosphere, polar caps — pure recolor sliders |
| 12 | civ | patch | habitability → sites → roads on the patch (§4.6) |

Sea level is a context scalar (from the sphere), re-derivable post-flow the same
way production does (recompute quantile on the S_tect sphere, not the patch).

### 4.5 Edge policy (open patch boundaries)

- Erosion droplets that step outside the window terminate (deposit nothing outside).
- Planchon–Darboux treats the window border as an outlet (edge pixels are drains) —
  water always has somewhere to go, no artificial lakes against the frame.
- Disc blur / JFA-ish local passes clamp at edges.
- A* roads and civ sites are confined to the window interior.

### 4.6 Civ on the patch

Reuse the existing pipeline components (habitability scoring, site placement,
road A*) on the patch grid at patch resolution, with the Civ params as sliders.
The stated acceptance bar: **roads visibly follow terrain (valleys, coasts) and
read as roads; water does not route through rectangular bricks**. Confirmed
decision: an animated "tribes creep inland over time" growth simulation is a
stretch goal, not core scope — the core is interactive tuning of the existing civ
stages at readable resolution.

### 4.7 Explorer UI ("Patch Lab" mode)

- Mode toggle in planet-explorer (crust-enabled archetypes only; others get an
  explanatory message).
- **Patch canvas** at 1:1 pixels; view selector to show the current layer's
  output. **As built**: three views (Color / Height / Tectonic) rather than the
  five originally envisioned — river mask and civ overlay are visible via the
  Color view once those layers are selected in the rail, not as separate view
  modes.
- **Layer rail**: ordered list; clicking a row renders the stack up to (and
  including) that layer, using cached upstream output.
- **Param panels**: reuse existing explorer panels; every panel's slider drives
  the patch view while Patch Lab is open, debounced auto-render on input.
- **Context minimap**: small sphere-tectonics view with the patch window
  outlined — orientation + picker feedback.
- **Picker controls**: seed (from the header) + smart-pick-by-default with a
  **Next window** button that cycles through the ranked candidate list. **As
  built**: there is no separate 0°/0° preset button or manual face/center pin
  control — cycling through candidates is the only re-pick affordance.
- **Go!**: runs the existing full production render with the tuned profile and
  shows the standard sphere/equirect views.
- Wasm exports (as built): `patchInit`, `patchSelect`, `patchLayers`,
  `patchSetParam`, `patchRender`, `patchMinimap` (no `patchGo`/`patchDebugView` —
  "Go!" reuses the existing `planetExplorerGenerate`/`planetExplorerBakeEquirect`
  exports with the tuned profile).
- Sphere-stage params (MajorPlates, Assembly, CratonsMax, TargetLandFraction, …)
  are also editable in Patch Lab; changing one triggers sphere recompute at S_tect
  (a few seconds), re-pick (unless pinned), re-crop, full patch re-render.

## 5. Determinism

- All new RNG uses `seed.Domain` namespaces under `patch.*`; layers that reuse
  production stages keep the production domains so identical inputs give
  identical outputs.
- Given (seed, profile, patch window), every layer output is byte-deterministic.

## 6. Testing

1. **Per-layer snapshot goldens** (new, the phase's side goal): at fixed seed +
   pinned profile, bake each layer's patch output (hash or small PNG) for 2–3
   archetypes (terran, arid). Byte-exact comparison — any stage drift is caught at
   the exact layer that drifted, unlike the ΔE sphere goldens. Runtime: seconds.
2. **Patch⇔sphere consistency invariant**: render layers 0–1 on the patch and
   compare against the same window cropped from a real sphere render at S_prod
   (small tolerance for S_tect→S_prod upsampling). Pins the "patch is a true
   crop" property.
3. **Edge-policy invariants**: no droplet deposition outside window; no
   Planchon–Darboux lake touching the frame; roads/sites strictly interior.
4. **Picker determinism**: same seed → same ranked window list.
5. Existing face-128 golden suite unchanged (`-face` flag convention preserved).

## 7. Known, accepted divergences from the production render

Documented, not hidden — the "Go!" render is the source of truth:

- **Distance-to-coast** for coastal noise is computed patch-locally (production
  uses sphere-global JFA). Coast cells near the window edge see slightly
  different distances.
- **Rain shadow** wind walks clamp at the window edge (production walks great
  circles across the sphere); upwind terrain outside the patch is invisible.
- **S_tect→S_prod upsampling** of tectonic fields smooths boundary detail
  slightly relative to computing tectonics natively at S_prod.
- Post-flow **sea-level requantile** uses the S_tect sphere, not the patch. As
  implemented (`Context.seaLevelView()`, layer 11), absent a slider override this
  value is provably identical to production's own post-flow requantiled
  `OceanLevel` (both are `field.QuantileSeaLevel` over the same land-fraction
  target) — the only real divergence from production is the resolution the
  quantile is computed at (S_tect vs. S_prod), not the mechanism, and the
  slider override is a deliberate Patch Lab addition, not an accidental one.

## 8. Error handling

- Crust-disabled profile → Patch Lab disabled with message (crust path required).
- Picker on degenerate worlds (all ocean / no boundaries) → still returns argmax.
- Sphere recompute failures / invalid params → surface the error in the UI,
  keep the previous patch state (never blank the canvas).

## 9. New/changed code (survey)

- `pkg/planetgen/patch/` — Patch/Context/Field types, Extract, Pick, layer
  registry + stack (adapted from June PoC, rewritten against Phase 12 APIs).
- `pkg/planetgen/render` — small exports where stage internals need reuse on a
  flat grid (erosion, flow, FX apply are already field-level and mostly reusable;
  rocky.go colorize helpers may need extraction).
- `cmd/planet-explorer/wasm` + `web/` — Patch Lab mode, exports, UI.
- `cmd/generate-planet-maps` or `pkg/planetgen/patch` tests — per-layer snapshot
  goldens + invariants.

## 10. Branch housekeeping (approved 2026-07-03)

- `phase-0/cube-map` merged into `main` via PR (main had diverged; conflicts in
  .gitignore / generate-planet-maps main.go / go.mod / go.sum resolved by union +
  adopting main's db default path).
- `phase-13/progressive-layers` had forked from main-era history (159 ahead /
  289 behind `phase-0/cube-map`; only `ec4b28257` — the June spec + PoC — was
  unique). Re-pointed: reset to the merged base + cherry-pick `ec4b28257`. The
  June PoC Go code is reference only; if it does not compile against Phase 12
  APIs it is removed from the branch (it stays reachable at `ec4b28257`).
