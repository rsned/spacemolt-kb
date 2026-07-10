# Phase 14: Worker Responsiveness + Patch Lab Polish

**Date:** 2026-07-10
**Status:** Approved

## Goal

Planet-explorer computes (sphere generates above face 128, Patch Lab init/render)
block the browser main thread for their full duration. Chrome repaints nothing,
no spinner can animate, and after ~5s the "Page Unresponsive" Wait/Kill dialog
fires — 4–5 times per compute at face 256+. Phase 14 moves the wasm module into
a dedicated Web Worker so the tab stays responsive at any face size, adds real
progress reporting with a cancel button, and closes out the Phase 13 deferred
polish list.

## Root Cause (context)

`web/app.js:39` instantiates the wasm on the main thread via
`WebAssembly.instantiateStreaming`. `cmd/planet-explorer/wasm/main.go` exports
13 globals (`planetExplorerGenerate`, `planetExplorerGenerateNight`,
`planetExplorerGenerateHeightmap`, `planetExplorerBakeEquirect`,
`planetExplorerDefaultProfile`, `planetExplorerGenerateDebug`,
`planetExplorerGenerateWithBypass`, `patchInit`, `patchSelect`, `patchLayers`,
`patchSetParam`, `patchRender`, `patchMinimap`), all called synchronously from
`app.js`. Every long compute therefore blocks the event loop. A busy overlay
alone cannot fix this: the unresponsive dialog triggers on main-thread blockage
regardless of what is painted.

## Design

### 1. Web Worker architecture

- New `web/worker.js`: loads `wasm_exec.js` and the wasm module inside a
  dedicated Worker. The 13 Go-exported globals land in the worker's global
  scope (`js.Global()` resolves there); the Go export code is unchanged.
- `app.js` gains an RPC shim. Every direct `window.planetExplorer*` /
  `window.patch*` call becomes `await rpc('name', args)`:
  - each call gets a monotonically increasing id, posts `{id, name, args}`;
  - the worker invokes the matching global and posts `{id, result}` (or
    `{id, error}`);
  - the shim resolves/rejects the pending promise by id.
- Pixel buffers travel back as **Transferable ArrayBuffers** (zero-copy). The
  main thread wraps them in `ImageData` and draws to canvas exactly as today.
- The worker processes one request at a time; the shim queues requests in
  call order.
- **Worker-only. No main-thread fallback path.** One code path; every browser
  that runs Go wasm has Workers.

### 2. Progress, cancel, busy UI

- Go side: compute entry points (`generate`, `generateDebug`, `patchInit`,
  `patchRender`, and the sphere recompute behind the new button) invoke an
  optional progress callback between pipeline stages:
  `progress(stageKey string, i, n int)`. The callback is a nil-default field
  threaded through the compute context — **it must not perturb output bytes**
  (golden tests enforce this).
- Worker forwards progress as `{id, progress: {stage, i, n}}` messages.
- Main thread shows a busy overlay for any RPC still in flight after
  **~150 ms** (quick calls never flash a spinner): animated spinner, stage
  descriptor, honest `i/n` layer counter/bar, and a **Cancel** button.
- **Whimsical descriptors:** `app.js` maps each canonical stage key to a pool
  of flavor lines ("Smashing continental plates together", "Hiding dinosaur
  bones", "Making mountains out of molehills", …) and picks one per stage.
  Copywriting lives entirely in JS; the Go pipeline reports only canonical
  keys. The honest counter is always shown alongside the joke.
- **Cancel** = `worker.terminate()`, respawn a fresh worker, reject all
  in-flight RPCs, restore pre-compute UI state. Stated trade-off: the
  wasm-side patch session dies with the worker — cancel means abandon, and
  re-entering Patch Lab re-inits.

### 3. FX-slider latency under async RPC

Today's <1 s slider feedback relies on synchronous `patchSetParam` +
`patchRender`. Async RPC makes flooding possible (one drag emits dozens of
events), so the shim adds **latest-wins coalescing for render-class calls**:

- while a render is in flight, newer slider-triggered renders overwrite a
  single pending slot;
- when the in-flight render returns, only the newest pending request runs;
- cheap ordered calls (`patchSetParam`) are not coalesced, only renders.

Net: same or better perceived latency; a drag can never queue 30 stale renders.

### 4. Deferred-list polish (from Phase 13 final review)

- **Flow / Civ / RainShadow panels**: three new `render…Panel` functions in
  `app.js` following the existing panel pattern — 13 sliders total
  (Flow: RiverThreshold, RiverDepth; RainShadow: WalkSteps, StepArcRad,
  MountainCutoff, WindRainBoost, LeeFactor; Civ: Tier, SiteMinDistRad,
  SiteMaxDistRad, MaxPopulation, NightLightHue, AgricultureRatio). Each
  panel's enable toggle wires to its zero-value-disables param
  (`RiverThreshold == 0`, `WalkSteps == 0`, `Tier == 0`). Panels edit the
  profile through the same patch-aware Apply path as existing panels; the
  plan must verify the patch dirty-prefix mapping covers `flow.` / `civ.` /
  `rainShadow.` (patch layers already exist: `layer_flow.go`, `layer_civ.go`,
  `layer_climate.go`).
- **Recompute sphere button** (Patch Lab): re-runs sphere compute to resync
  the four stale scalars (HMin, HMax, SeaLevel0, SeaLevel) — the documented
  5th divergence in the Patch Lab spec §7. Newly reasonable because the
  worker gives it a progress bar and cancel instead of a frozen tab.
- **Layer-rail label refresh**: "(disabled)" labels recompute on param change.
- **Patch-mode canvas headers**: hide the leftover canvas `<h2>` headers
  while patch mode is active.
- **Sea-level hint**: visible "view only — not applied on Go!" note on the
  patch-mode sea-level slider.

### 5. Internals cleanup

- `Window.PxRad()` helper replaces the three duplicated `(π/2)/SProd`
  derivations in the patch package.
- FX tint table deduplicates to one shared definition (currently duplicated
  in `render.go`).
- **Invalidation narrowing**: ControlConfig.Temperature/Humidity edits
  currently re-run erosion though only the climate-consuming layer reads
  them — narrow the dirty mapping. Inert params (OceanLevel, Continents.*,
  Ridged.*, Basin.*) stop triggering no-op sphere recomputes.

## Testing & Verification

- **Go**: progress hooks tested for stage sequence AND golden byte-identity
  (nil vs non-nil callback produces identical output). Invalidation narrowing
  gets equality tests: edit Temperature via the narrowed path, assert the
  result byte-equals a full recompute. Existing production goldens
  (face-128) must remain untouched.
- **Wasm build gate**: `GOOS=js GOARCH=wasm go build ./cmd/planet-explorer/wasm`
  (regular `go test ./...` never builds this package).
- **JS** (no test harness in repo): an explicit, scripted browser
  click-through task in the plan — face-256 generate with zero unresponsive
  dialogs, spinner + stage text visible, slider-drag coalescing (no queued
  stale renders), cancel mid-compute → clean UI restore + Patch Lab re-init,
  whimsy descriptors rotating. Not left implicit as in Phase 13.
- `go build ./...`, `go test ./...`, `golangci-lint` clean.

## Out of Scope

- LOW hash-robustness notes (StateHash presence markers; Craters hash
  IsSecondary/ParentIdx) — still deferred, matter only for future features.
- OffscreenCanvas rendering in the worker (main-thread ImageData draw is fine).
- Multi-worker parallelism; main-thread fallback path.
- Any change to production generation output (byte-identity preserved).
