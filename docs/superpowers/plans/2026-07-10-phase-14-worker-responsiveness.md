# Phase 14: Worker Responsiveness + Patch Lab Polish — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the planet-explorer wasm module into a Web Worker so the browser tab stays responsive at any face size, add progress reporting + cancel + a busy overlay with whimsical stage descriptors, and close out the Phase 13 deferred polish list (Flow/RainShadow/Civ panels, Recompute-sphere button, MarkDirty narrowing, PxRad/tint dedup, UI polish trio).

**Architecture:** The wasm module (13 exported globals) moves from the main thread into `web/worker.js`; `app.js` talks to it through a promise-based RPC shim with Transferable pixel buffers. Go gains a nil-default progress hook in `pkg/planetgen/render` and `pkg/planetgen/patch` (set only by the wasm worker), which the worker forwards as `progress` messages driving a busy overlay with cancel (= worker terminate + respawn). Patch-package cleanups (MarkDirty specificity, `Window.PxRad()`, shared FX tint table) land first as pure-Go tasks.

**Tech Stack:** Go 1.24+ (`GOOS=js GOARCH=wasm` for the module), vanilla JS (no framework, no build step), Web Worker + postMessage + Transferable ArrayBuffers.

## Global Constraints

- **Byte-identity of generation output is inviolable.** Progress hooks and refactors must not perturb any rendered pixel or hash. `TestPatchLayerGoldens` (pkg/planetgen/patch/golden_test.go) and all existing render tests must pass unchanged — never re-bake a golden in this phase.
- **The wasm package builds only via cross-compile**: `GOOS=js GOARCH=wasm go build ./cmd/planet-explorer/wasm` — `go test ./...` never touches it. Run this gate after every task that edits `cmd/planet-explorer/wasm/main.go`, `pkg/planetgen/render`, or `pkg/planetgen/patch`.
- **Worker-only, no main-thread fallback.** After Task 6 there is exactly one wasm boot path (the worker). Do not keep dual paths.
- **Whimsical copy lives only in `app.js`.** Go reports canonical stage keys (`"sphere:plates"`, `"layer:erosion"`, `"Crust"`); the honest `i/n` counter is always displayed alongside any joke.
- **Progress hooks are nil-default package globals**, set once at wasm boot, never set in production/server/test code paths (tests may set them locally and must restore nil via `t.Cleanup`).
- **JSON profile keys for the new panels are lowercase**: `profile.flow`, `profile.rainShadow`, `profile.civ` (types.go tags `json:"flow,omitempty"` etc.). PascalCase (`profile.Flow`) silently creates a dead field.
- Go 1.24+ idioms (range-over-int, `b.Loop()` in benchmarks). `golangci-lint run` must report zero new findings after every task.
- JS files must pass `node --check <file>` (syntax gate; there is no JS test harness in this repo).
- Run `go build ./... && go test ./...` before every commit (project rule).

## Verification commands (used throughout)

```bash
cd /home/robert/spacemolt/kb
go build ./...
go test ./pkg/planetgen/... 2>&1 | tail -20
GOOS=js GOARCH=wasm go build -o /tmp/pe-wasm-check ./cmd/planet-explorer/wasm && rm /tmp/pe-wasm-check
node --check cmd/planet-explorer/web/app.js
node --check cmd/planet-explorer/web/worker.js
golangci-lint run ./pkg/planetgen/... ./cmd/planet-explorer/...
```

**IMPORTANT (wasm binary gotcha):** `GOOS=js GOARCH=wasm go build ./cmd/planet-explorer/wasm` with no `-o` drops a binary named `wasm` in the CWD. Always use `-o /tmp/pe-wasm-check` as shown.

**Rebuilding the shipped wasm** (only Task 6's manual smoke and later UI tasks need it):
```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
```
The rebuilt `planet-explorer.wasm` IS committed when a task changes wasm behavior (it is a checked-in web asset — verify with `git ls-files cmd/planet-explorer/web/planet-explorer.wasm`).

---

### Task 1: MarkDirty specificity, inert params, climate field ownership

Editing `ControlConfig.Temperature`/`.Humidity` currently re-runs the stack from layer 2 (control-noise) although only layer 9 (climate) reads them — erosion re-runs for nothing. And edits to params the crust path never reads (`OceanLevel`, `Continents.*`, `Ridged.*`, `Basin.*`) trigger a full multi-second sphere recompute that produces an identical `SphereData` (Patch Lab rejects non-crust profiles, so these are inert in every session).

**Files:**
- Modify: `pkg/planetgen/patch/stack.go` (MarkDirty ~L128-140; Layers() climate entry L74)
- Test: `pkg/planetgen/patch/stack_test.go`

**Interfaces:**
- Consumes: existing `Stack`, `Layers()`, `countingStack` test helper (stack_test.go L11), `StateHash` (render.go).
- Produces: `MarkDirty` semantics used verbatim by wasm `patchSetParam` (no wasm change needed); layer 9 Params `["rainShadow", "ControlConfig.Temperature", "ControlConfig.Humidity"]`.

- [ ] **Step 1: Write the failing tests** — append to `pkg/planetgen/patch/stack_test.go`:

```go
// dirtyFromOf exposes the private dirtyFrom for assertions without
// widening the API.
func dirtyFromOf(s *Stack) int { return s.dirtyFrom }

func TestMarkDirtyClimateFieldsNarrowToLayer9(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if _, err := s.RenderTo(12); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"ControlConfig.Temperature.Amp", "ControlConfig.Humidity.Freq"} {
		s.MarkAllDirty()
		if _, err := s.RenderTo(12); err != nil {
			t.Fatal(err)
		}
		if sphere := s.MarkDirty(path); sphere {
			t.Fatalf("MarkDirty(%q) = true, want false (climate layer owns it)", path)
		}
		if got := dirtyFromOf(s); got != 9 {
			t.Fatalf("MarkDirty(%q): dirtyFrom = %d, want 9 (climate)", path, got)
		}
	}
}

func TestMarkDirtyControlDetailStillLayer2(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if _, err := s.RenderTo(12); err != nil {
		t.Fatal(err)
	}
	if sphere := s.MarkDirty("ControlConfig.Detail.Amp"); sphere {
		t.Fatal("MarkDirty(ControlConfig.Detail.Amp) = true, want false")
	}
	if got := dirtyFromOf(s); got != 2 {
		t.Fatalf("dirtyFrom = %d, want 2 (control-noise)", got)
	}
}

func TestMarkDirtyBulkControlConfigDirtiesEarliestOwner(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if _, err := s.RenderTo(12); err != nil {
		t.Fatal(err)
	}
	// A whole-subtree edit ("ControlConfig") touches both layer 2's
	// pattern and layer 9's more-specific patterns; the earliest
	// consuming layer must re-run.
	if sphere := s.MarkDirty("ControlConfig"); sphere {
		t.Fatal("MarkDirty(ControlConfig) = true, want false")
	}
	if got := dirtyFromOf(s); got != 2 {
		t.Fatalf("dirtyFrom = %d, want 2", got)
	}
}

func TestMarkDirtyInertParamsAreNoOps(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if _, err := s.RenderTo(12); err != nil {
		t.Fatal(err)
	}
	clean := dirtyFromOf(s)
	for _, path := range []string{"OceanLevel", "Continents.Scale", "Ridged.Amp", "Basin.Freq"} {
		if sphere := s.MarkDirty(path); sphere {
			t.Fatalf("MarkDirty(%q) = true, want false (inert on crust path)", path)
		}
		if got := dirtyFromOf(s); got != clean {
			t.Fatalf("MarkDirty(%q) moved dirtyFrom %d -> %d, want unchanged", path, clean, got)
		}
	}
}

func TestMarkDirtySphereParamsStillRecompute(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	for _, path := range []string{"Crust.MajorPlates", "__fullRefresh", "JitterCellCount"} {
		if sphere := s.MarkDirty(path); !sphere {
			t.Fatalf("MarkDirty(%q) = false, want true (sphere-level)", path)
		}
	}
}

// TestClimateNarrowingEquality proves the narrowed path is not just
// cheaper but CORRECT: editing Temperature via MarkDirty then
// re-rendering equals a from-scratch stack built on the edited profile.
func TestClimateNarrowingEquality(t *testing.T) {
	sd := testSphere(t)
	w := Pick(sd, 32, 64, 1)[0].Window
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	edited := *sd.Profile
	edited.ControlConfig.Temperature.Amp += 0.25

	// Path A: live stack, original profile rendered, then edit +
	// MarkDirty + re-render (what patchSetParam does).
	profA := *sd.Profile
	ctxA := &Context{Sphere: sd, Fields: f, Profile: &profA, Master: sd.Master}
	sA := NewStack(ctxA)
	if _, err := sA.RenderTo(12); err != nil {
		t.Fatal(err)
	}
	ctxA.Profile = &edited
	if sphere := sA.MarkDirty("ControlConfig.Temperature.Amp"); sphere {
		t.Fatal("Temperature edit must not signal a sphere recompute")
	}
	stA, err := sA.RenderTo(12)
	if err != nil {
		t.Fatal(err)
	}

	// Path B: fresh stack on the edited profile.
	ctxB := &Context{Sphere: sd, Fields: f, Profile: &edited, Master: sd.Master}
	sB := NewStack(ctxB)
	stB, err := sB.RenderTo(12)
	if err != nil {
		t.Fatal(err)
	}

	if ha, hb := StateHash(stA), StateHash(stB); ha != hb {
		t.Fatalf("narrowed re-render diverged from fresh render: %s != %s", ha, hb)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/planetgen/patch/ -run 'TestMarkDirty|TestClimateNarrowing' -v`
Expected: `TestMarkDirtyClimateFieldsNarrowToLayer9` FAILS (dirtyFrom = 2, want 9), `TestMarkDirtyInertParamsAreNoOps` FAILS (sphere recompute signalled or dirtyFrom moved). The others may pass — that pins current behavior.

- [ ] **Step 3: Implement** — in `pkg/planetgen/patch/stack.go`:

3a. Change the climate layer entry in `Layers()` (L74):

```go
		{ID: "climate", Name: "Climate", Params: []string{"rainShadow", "ControlConfig.Temperature", "ControlConfig.Humidity"}},
```

3b. Replace `MarkDirty` (L124-140) with:

```go
// inertParams are profile params the crust-path pipeline never reads.
// Patch Lab sessions are always crust-path (ComputeSphere rejects
// crust-disabled profiles), so edits to these change nothing — mapping
// them to a sphere recompute would spend seconds producing an
// identical SphereData. Matched by path prefix, like Layer.Params.
var inertParams = []string{"OceanLevel", "Continents", "Ridged", "Basin"}

// MarkDirty maps a changed profile param path to the owning layer.
//
// Matching rules:
//  1. Inert params (crust path never reads them): no-op, returns false.
//  2. The edit sits at/under one or more layer patterns: the MOST
//     SPECIFIC (longest) pattern wins, so "ControlConfig.Temperature.Amp"
//     dirties climate (layer 9) while "ControlConfig.Detail.Amp" still
//     dirties control-noise (layer 2).
//  3. The edit is BROADER than some pattern (paramPath is a proper
//     prefix of it, e.g. a whole-"ControlConfig" replacement): several
//     layers may consume parts of the subtree — the EARLIEST matching
//     layer is dirtied.
//
// Returns true when no layer owns the param — the sphere precompute
// does — and the caller must recompute SphereData + Fields, then
// MarkAllDirty.
func (s *Stack) MarkDirty(paramPath string) bool {
	for _, p := range inertParams {
		if strings.HasPrefix(paramPath, p) {
			return false
		}
	}
	bestSpecific, bestLen := -1, -1 // longest pattern containing the edit
	broadEarliest := -1            // earliest layer whose pattern the edit contains
	for i := range s.layers {
		for _, p := range s.layers[i].Params {
			switch {
			case strings.HasPrefix(paramPath, p):
				if len(p) > bestLen {
					bestSpecific, bestLen = i, len(p)
				}
			case strings.HasPrefix(p, paramPath):
				if broadEarliest == -1 {
					broadEarliest = i
				}
			}
		}
	}
	target := bestSpecific
	if broadEarliest != -1 && (target == -1 || broadEarliest < target) {
		target = broadEarliest
	}
	if target == -1 {
		return true
	}
	if target < s.dirtyFrom {
		s.dirtyFrom = target
	}
	return false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/planetgen/patch/ -v 2>&1 | tail -30`
Expected: ALL patch tests PASS, including `TestPatchLayerGoldens` and the pre-existing `TestStackSphereParamSignals` (if that test asserted the old first-match behavior for a now-narrowed path, read it and update ONLY assertions that contradict the new documented semantics — flag this in the task report).

- [ ] **Step 5: Full gates + commit**

```bash
go build ./... && go test ./pkg/planetgen/... 2>&1 | tail -5
GOOS=js GOARCH=wasm go build -o /tmp/pe-wasm-check ./cmd/planet-explorer/wasm && rm /tmp/pe-wasm-check
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/patch/stack.go pkg/planetgen/patch/stack_test.go
git commit -m "feat(patch): narrow MarkDirty to most-specific layer, skip crust-inert params"
```

---

### Task 2: `Window.PxRad()` helper + shared FX tint table

Three sites independently derive "radians per virtual production pixel" as `(π/2)/SProd`; two functions duplicate the 5-entry tectonic FX tint table. Pure refactor — goldens prove identity.

**Files:**
- Modify: `pkg/planetgen/patch/window.go` (add method), `pkg/planetgen/patch/layer_civ.go:82`, `pkg/planetgen/patch/layer_craters.go:30`, `pkg/planetgen/patch/layer_coastal.go:63`, `pkg/planetgen/patch/render.go` (L75-81 and L119-128)
- Test: `pkg/planetgen/patch/window_test.go`, `pkg/planetgen/patch/render_test.go`

**Interfaces:**
- Produces: `func (w Window) PxRad() float64`; package var `fxTints [5]color.RGBA` in render.go.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/planetgen/patch/window_test.go`:

```go
func TestWindowPxRad(t *testing.T) {
	w := Window{SProd: 1024}
	want := (math.Pi / 2) / 1024.0
	if got := w.PxRad(); got != want {
		t.Fatalf("PxRad() = %v, want %v", got, want)
	}
}
```

(Add `"math"` to the test file's imports if absent.)

Append to `pkg/planetgen/patch/render_test.go`:

```go
// TestFxTintsPinned pins the five debug tints in canonical class order
// (belt, subduction, arc, ridge, rift) so the TectonicDebugPNG /
// MinimapPNG dedup cannot silently change a color.
func TestFxTintsPinned(t *testing.T) {
	want := [5]color.RGBA{
		{R: 200, G: 40, B: 40, A: 255},
		{R: 230, G: 120, B: 30, A: 255},
		{R: 230, G: 210, B: 60, A: 255},
		{R: 60, G: 200, B: 220, A: 255},
		{R: 200, G: 60, B: 200, A: 255},
	}
	if fxTints != want {
		t.Fatalf("fxTints = %v, want %v", fxTints, want)
	}
}
```

(Add `"image/color"` to render_test.go imports if absent.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/planetgen/patch/ -run 'TestWindowPxRad|TestFxTintsPinned' -v`
Expected: compile FAILURE — `w.PxRad undefined`, `undefined: fxTints`.

- [ ] **Step 3: Implement**

3a. `window.go` — add `"math"` import and the method after `Dir`:

```go
// PxRad is the angular size of one virtual production pixel in
// radians: a cube face spans π/2 radians across SProd pixels.
func (w Window) PxRad() float64 { return (math.Pi / 2) / float64(w.SProd) }
```

3b. `layer_civ.go:82` — replace `pxRad := (math.Pi / 2) / float64(w.SProd)` with `pxRad := w.PxRad()`. Remove the `math` import ONLY if now unused (check the rest of the file first).

3c. `layer_craters.go:30` — replace
`halfDiag := float64(w.Size) / float64(w.SProd) * (math.Pi / 2)` with
`halfDiag := float64(w.Size) * w.PxRad()` (keep the trailing comment).

3d. `layer_coastal.go` — `distanceToCoastPatch(hm *Grid, threshold float64, sProd int)` (L14) takes the face size only to derive radians-per-pixel at L63. Change the parameter to what it actually wants:

```go
func distanceToCoastPatch(hm *Grid, threshold float64, pxRad float64) *Grid {
```

L63 becomes `pxToFrac := pxRad / math.Pi` (keep the `// Pixels → angular fraction of π (JFA units).` comment). Update the single caller (`applyCoastal`, L84): `dist := distanceToCoastPatch(st.Height, sea, w.PxRad())`. Update the doc comment at L10-13 if it mentions sProd.

3e. `render.go` — add above `fxClass`:

```go
// fxTints are the five tectonic FX class debug tints in canonical
// class order (belt=red, subduction=orange, arc=yellow, ridge=cyan,
// rift=magenta). Shared by TectonicDebugPNG (patch-resolution grids)
// and MinimapPNG (sphere-resolution fields).
var fxTints = [5]color.RGBA{
	{R: 200, G: 40, B: 40, A: 255},
	{R: 230, G: 120, B: 30, A: 255},
	{R: 230, G: 210, B: 60, A: 255},
	{R: 60, G: 200, B: 220, A: 255},
	{R: 200, G: 60, B: 200, A: 255},
}
```

In `TectonicDebugPNG` replace the literal tints:

```go
	classes := []fxClass{
		{f.BeltDist, f.BeltMag, fxTints[0]},
		{f.SubdDist, f.SubdMag, fxTints[1]},
		{f.ArcDist, f.ArcMag, fxTints[2]},
		{f.RidgeDist, f.RidgeMag, fxTints[3]},
		{f.RiftDist, f.RiftMag, fxTints[4]},
	}
```

In `MinimapPNG` likewise:

```go
	classes := []struct {
		Dist, Mag *cubemap.CubeMapF
		Tint      color.RGBA
	}{
		{sd.FX.BeltDist, sd.FX.BeltMag, fxTints[0]},
		{sd.FX.SubdDist, sd.FX.SubdMag, fxTints[1]},
		{sd.FX.ArcDist, sd.FX.ArcMag, fxTints[2]},
		{sd.FX.RidgeDist, sd.FX.RidgeMag, fxTints[3]},
		{sd.FX.RiftDist, sd.FX.RiftMag, fxTints[4]},
	}
```

- [ ] **Step 4: Verify**

Run: `go test ./pkg/planetgen/patch/ 2>&1 | tail -5`
Expected: ok — all tests including `TestPatchLayerGoldens` pass (byte-identity of the refactor).

- [ ] **Step 5: Gates + commit**

```bash
go build ./... && GOOS=js GOARCH=wasm go build -o /tmp/pe-wasm-check ./cmd/planet-explorer/wasm && rm /tmp/pe-wasm-check
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/patch/window.go pkg/planetgen/patch/window_test.go pkg/planetgen/patch/render.go pkg/planetgen/patch/render_test.go pkg/planetgen/patch/layer_civ.go pkg/planetgen/patch/layer_craters.go pkg/planetgen/patch/layer_coastal.go
git commit -m "refactor(patch): Window.PxRad helper + shared fxTints table"
```

---

### Task 3: Progress hook in pkg/planetgen/patch

A nil-default package callback reporting pipeline stages, invoked by `ComputeSphere` (10 sphere stages) and `Stack.RenderTo` (per layer). Output bytes must be provably unaffected.

**Files:**
- Create: `pkg/planetgen/patch/progress.go`
- Modify: `pkg/planetgen/patch/sphere.go`, `pkg/planetgen/patch/stack.go` (RenderTo loop)
- Test: `pkg/planetgen/patch/progress_test.go`

**Interfaces:**
- Produces: `type ProgressFunc func(stage string, i, n int)`, `func SetProgressHook(fn ProgressFunc)` — consumed by Task 5 (wasm). Stage keys: `sphere:jitter|plates|crust|fx|splines|tectonic-fx|smooth|normalize|erode|flow` (i/n over 10) and `layer:<layer-ID>` (i = Index+1, n = 13). Conditional stages (smooth, erode) are skipped when disabled, so the counter may jump — the UI treats i/n as "position", not "everything runs".

- [ ] **Step 1: Write the failing test** — create `pkg/planetgen/patch/progress_test.go`:

```go
package patch

import (
	"slices"
	"strings"
	"testing"
)

// TestProgressHookSphereSequence asserts ComputeSphere announces its
// stages in pipeline order, and that setting the hook does not change
// any derived scalar (byte-level identity is separately pinned by
// TestPatchLayerGoldens running with a nil hook).
func TestProgressHookSphereSequence(t *testing.T) {
	base := testSphere(t) // nil-hook baseline

	var got []string
	SetProgressHook(func(stage string, i, n int) {
		got = append(got, stage)
		if n != 10 {
			t.Errorf("stage %q: n = %d, want 10", stage, n)
		}
	})
	t.Cleanup(func() { SetProgressHook(nil) })

	hooked, err := ComputeSphere(base.Profile, base.Master, base.STect)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := []string{"sphere:jitter", "sphere:plates", "sphere:crust", "sphere:fx", "sphere:splines", "sphere:tectonic-fx"}
	if len(got) < len(wantPrefix) || !slices.Equal(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("stage sequence = %v, want prefix %v", got, wantPrefix)
	}
	for _, s := range got {
		if !strings.HasPrefix(s, "sphere:") {
			t.Fatalf("unexpected stage key %q", s)
		}
	}
	if hooked.HMin != base.HMin || hooked.HMax != base.HMax ||
		hooked.SeaLevel0 != base.SeaLevel0 || hooked.SeaLevel != base.SeaLevel {
		t.Fatalf("hooked ComputeSphere diverged: %+v vs %+v",
			[4]float64{hooked.HMin, hooked.HMax, hooked.SeaLevel0, hooked.SeaLevel},
			[4]float64{base.HMin, base.HMax, base.SeaLevel0, base.SeaLevel})
	}
}

// TestProgressHookRenderToReportsRunLayersOnly asserts RenderTo
// announces exactly the layers it re-runs (cached layers are silent).
func TestProgressHookRenderToReportsRunLayersOnly(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)

	var got []string
	SetProgressHook(func(stage string, i, n int) {
		if strings.HasPrefix(stage, "layer:") {
			got = append(got, stage)
			if n != 13 {
				t.Errorf("stage %q: n = %d, want 13", stage, n)
			}
		}
	})
	t.Cleanup(func() { SetProgressHook(nil) })

	if _, err := s.RenderTo(3); err != nil {
		t.Fatal(err)
	}
	first := len(got)
	if first == 0 {
		t.Fatal("no layer progress reported on first render")
	}
	got = got[:0]
	if _, err := s.RenderTo(3); err != nil { // fully cached
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("cached re-render reported %v, want none", got)
	}
}
```

Note: `testSphere(t)` already exists (used by stack_test.go). If its profile disables smooth or erosion the sequence simply omits those keys — the test asserts only the unconditional prefix.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/planetgen/patch/ -run TestProgressHook -v`
Expected: compile FAILURE — `undefined: SetProgressHook`.

- [ ] **Step 3: Implement**

3a. Create `pkg/planetgen/patch/progress.go`:

```go
package patch

// ProgressFunc receives pipeline stage announcements. stage is a
// canonical key ("sphere:plates", "layer:erosion"); i/n give the
// stage's 1-based position and the stage count when known. Conditional
// stages are skipped when disabled, so i may jump — treat i/n as
// position, not a completion guarantee.
type ProgressFunc func(stage string, i, n int)

// progressHook is a nil-default package global, set exactly once at
// boot by single-threaded consumers (the planet-explorer wasm worker)
// and nil everywhere else. Do not set it concurrently with renders.
var progressHook ProgressFunc

// SetProgressHook installs fn as the pipeline progress callback.
// Pass nil to disable.
func SetProgressHook(fn ProgressFunc) { progressHook = fn }

func reportProgress(stage string, i, n int) {
	if progressHook != nil {
		progressHook(stage, i, n)
	}
}
```

3b. `sphere.go` — insert `reportProgress` calls immediately BEFORE each stage (n=10 throughout):

- before `noise.GenerateJitter(...)`: `reportProgress("sphere:jitter", 1, 10)`
- before `field.GeneratePlates(...)`: `reportProgress("sphere:plates", 2, 10)`
- before `field.GenerateCrust(...)`: `reportProgress("sphere:crust", 3, 10)`
- before `field.ClassifyTectonics(...)`: `reportProgress("sphere:fx", 4, 10)`
- before `field.GenerateControlFields(...)`: `reportProgress("sphere:splines", 5, 10)`
- before `field.ApplyTectonicFX(...)`: `reportProgress("sphere:tectonic-fx", 6, 10)`
- inside `if profile.HeightSmoothRadius > 0 {`, first line: `reportProgress("sphere:smooth", 7, 10)`
- before the normalize min/max block: `reportProgress("sphere:normalize", 8, 10)`
- inside `if ecfg.Droplets > 0 {`, first line: `reportProgress("sphere:erode", 9, 10)`
- before `if ff := field.GenerateFlow(...)`: `reportProgress("sphere:flow", 10, 10)`

3c. `stack.go` `RenderTo` — in the layer loop, report before Apply:

```go
	for i := start; i <= target; i++ {
		if s.layers[i].Enabled(s.ctx) {
			reportProgress("layer:"+s.layers[i].ID, i+1, len(s.layers))
			st = s.layers[i].Apply(s.ctx, st)
		}
		s.cache[i] = st
	}
```

- [ ] **Step 4: Verify**

Run: `go test ./pkg/planetgen/patch/ 2>&1 | tail -5`
Expected: ok — including `TestPatchLayerGoldens` (nil hook) and the two new tests.

- [ ] **Step 5: Gates + commit**

```bash
go build ./... && golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/patch/progress.go pkg/planetgen/patch/progress_test.go pkg/planetgen/patch/sphere.go pkg/planetgen/patch/stack.go
git commit -m "feat(patch): nil-default progress hook for sphere + layer pipeline"
```

---

### Task 4: Progress hook in pkg/planetgen/render

Same pattern for the production render path used by full generates. Coarse 5-step reporting in `RenderRocky`, plus fine-grained stage names inside the two debug-aware pipeline bodies at their existing stage boundaries.

**Files:**
- Create: `pkg/planetgen/render/progress.go`
- Modify: `pkg/planetgen/render/rocky.go`
- Test: `pkg/planetgen/render/progress_test.go`

**Interfaces:**
- Produces: `render.SetProgressHook(fn func(stage string, i, n int))` — consumed by Task 5. Coarse keys `render:jitter|plates|heightmap|flow|colorize` (i/n over 5); fine keys are the existing DebugStage names with (0,0): heightmap side `Crust, ControlFields, Ridged, TectonicFX, Basin, Continents, HeightSmooth, Normalize, Coastal, Erosion, Flow, Craters`; colorize side `Palette, Snow, Ocean, PolarCaps, Shading, Ejecta, Civ, LUT`.

- [ ] **Step 1: Write the failing test** — create `pkg/planetgen/render/progress_test.go` (external test package, matching rocky_test.go, using the `planetgen.Profiles["terran"]` fixture the other render tests use):

```go
package render_test

import (
	"bytes"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

// TestRenderRockyProgressHookIdentity proves the hook changes no output
// byte: same profile+seed rendered with a nil hook and with a hook set
// must produce identical cube maps, and the hook must actually fire.
func TestRenderRockyProgressHookIdentity(t *testing.T) {
	prof := *planetgen.Profiles["terran"]
	const seed int64 = 42
	const face = 32

	base := render.RenderRocky(&prof, seed, face)

	var stages []string
	render.SetProgressHook(func(stage string, i, n int) { stages = append(stages, stage) })
	t.Cleanup(func() { render.SetProgressHook(nil) })
	hooked := render.RenderRocky(&prof, seed, face)

	if len(stages) == 0 {
		t.Fatal("progress hook never fired")
	}
	var b1, b2 bytes.Buffer
	if err := cubemap.WriteCrossPNGTo(base, &b1); err != nil {
		t.Fatal(err)
	}
	if err := cubemap.WriteCrossPNGTo(hooked, &b2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
		t.Fatal("hooked render diverged from nil-hook render")
	}
	want := []string{"render:jitter", "render:plates", "render:heightmap"}
	for i, w := range want {
		if stages[i] != w {
			t.Fatalf("stages[%d] = %q, want %q (full: %v)", i, stages[i], w, stages)
		}
	}
}
```

(If `planetgen.Profiles` is not the exported map name, check how `rocky_test.go:17` accesses the terran profile and use that exact form.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/planetgen/render/ -run TestRenderRockyProgressHookIdentity -v`
Expected: compile FAILURE — `undefined: SetProgressHook`.

- [ ] **Step 3: Implement**

3a. Create `pkg/planetgen/render/progress.go` with the identical pattern as Task 3's `patch/progress.go` (same doc comments adapted, `package render`).

3b. `rocky.go` `RenderRocky` (L23-44) — insert coarse reports:

```go
func RenderRocky(profile *types.PlanetProfile, seed int64, S int) *cubemap.CubeMap {
	reportProgress("render:jitter", 1, 5)
	jitter := noise.GenerateJitter(profile, seed, S)
	reportProgress("render:plates", 2, 5)
	plates := field.GeneratePlates(profile, seed, S)
	reportProgress("render:heightmap", 3, 5)
	heightmap, craters, oceanLevel := generateRockyHeightmapWithJitter(profile, seed, S, jitter, plates)
	// ... existing oceanLevel copy-on-differ block unchanged ...
	var flow *field.FlowField
	if profile.Flow.RiverThreshold > 0 {
		reportProgress("render:flow", 4, 5)
		flow = field.GenerateFlow(heightmap, profile.Flow)
	}
	reportProgress("render:colorize", 5, 5)
	return colorizeRocky(profile, seed, S, heightmap, craters, jitter, plates, flow)
}
```

3c. `generateRockyHeightmapDebug` — at the START of each stage block (the block whose end contains the matching `frame.Stages = append(..., Name: "X", ...)`), insert `reportProgress("X", 0, 0)`. The stage blocks and their anchor lines: Crust (~L221), the control-fields block (~L241, report once as `"ControlFields"` before `field.GenerateControlFields`), Ridged (~L389 anchor), TectonicFX (~L518), Basin (~L561), Continents (~L615), HeightSmooth (~L643), Normalize (~L678), Coastal (~L719), Erosion (~L756), Flow (~L792), Craters (~L829). IMPORTANT: place the report where the stage's WORK begins (often 10-40 lines above the `frame.Stages` append), inside any enabling `if`, so disabled stages stay silent. Line numbers are pre-task anchors — re-locate by grepping `Name:     "X"`.

3d. `colorizeRockyDebug` — same treatment for: Palette (~L941), Snow (~L971), Ocean (~L1001), PolarCaps (~L1033), Shading (~L1062), Ejecta (~L1086), Civ (~L1135), LUT (~L1160).

3e. `RenderNightCubeMap`, `RenderRockyHeightmap`, `bakeEquirect` paths get NO hooks — the JS side labels those whole calls (Task 7's RPC_LABELS).

- [ ] **Step 4: Verify**

Run: `go test ./pkg/planetgen/render/ 2>&1 | tail -5`
Expected: ok — all existing render tests plus the new identity test pass.

- [ ] **Step 5: Gates + commit**

```bash
go build ./... && GOOS=js GOARCH=wasm go build -o /tmp/pe-wasm-check ./cmd/planet-explorer/wasm && rm /tmp/pe-wasm-check
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/render/progress.go pkg/planetgen/render/progress_test.go pkg/planetgen/render/rocky.go
git commit -m "feat(render): nil-default progress hook on the rocky pipeline"
```

---

### Task 5: wasm — progress forwarding + patchRecomputeSphere export

**Files:**
- Modify: `cmd/planet-explorer/wasm/main.go`

**Interfaces:**
- Consumes: `patch.SetProgressHook`, `render.SetProgressHook` (Tasks 3-4); existing `patchSession`, `clampWindow`, `jsError`.
- Produces: JS global `patchRecomputeSphere()` → JSON `{"seaLevel":..., "seaLevel0":...}` or `{"error":...}` (consumed by Task 10); Go→JS progress forwarding via optional worker global `__pxProgress(stage, i, n)` (consumed by Task 6's worker.js).

There is no Go test harness for js/wasm code in this repo — the gates are the cross-compile plus code review; the behavior is exercised in Task 6/10 browser verification.

- [ ] **Step 1: Add progress forwarding** — in `main()` (after `planetgen.SetProfileRoot("")`, before the export registrations) add a call to `registerProgressHooks()`, and define:

```go
// registerProgressHooks forwards Go pipeline progress to the JS global
// __pxProgress(stage, i, n) when the embedder (worker.js) defined it
// before booting the wasm. No-op in a context without the global.
func registerProgressHooks() {
	cb := js.Global().Get("__pxProgress")
	if cb.Type() != js.TypeFunction {
		return
	}
	fn := func(stage string, i, n int) { cb.Invoke(stage, i, n) }
	patch.SetProgressHook(fn)
	render.SetProgressHook(fn)
}
```

- [ ] **Step 2: Add the recompute export** — register `js.Global().Set("patchRecomputeSphere", js.FuncOf(patchRecomputeSphere))` alongside the other patch exports, add the signature line `patchRecomputeSphere() → JSON {"seaLevel":..., "seaLevel0":...}` to the file-top doc comment, and implement:

```go
// patchRecomputeSphere() → JSON {"seaLevel":..., "seaLevel0":...}.
// Re-runs the sphere-global precompute with the session's CURRENT
// profile and re-extracts the window's fields, resyncing the four
// sphere-derived scalars (HMin/HMax/SeaLevel0/SeaLevel) that heavy FX
// / control-noise / height-smooth retuning drifts away from a fresh
// compute — the stale-scalar divergence documented in the Patch Lab
// spec §7. Marks every layer dirty; the caller re-renders.
func patchRecomputeSphere(_ js.Value, _ []js.Value) any {
	if patchSession.stack == nil {
		return jsError("patchRecomputeSphere: call patchSelect first")
	}
	sd, err := patch.ComputeSphere(patchSession.profile, patchSession.master, patchSession.sTect)
	if err != nil {
		return jsError("patchRecomputeSphere: %v", err)
	}
	w := clampWindow(patchSession.window)
	fields, err := patch.ExtractFields(sd, w)
	if err != nil {
		return jsError("patchRecomputeSphere: extract fields: %v", err)
	}
	patchSession.sd = sd
	patchSession.window = w
	ctx := patchSession.stack.Ctx()
	ctx.Sphere = sd
	ctx.Fields = fields
	patchSession.stack.MarkAllDirty()
	out, err := json.Marshal(map[string]any{"seaLevel": sd.SeaLevel, "seaLevel0": sd.SeaLevel0})
	if err != nil {
		return jsError("patchRecomputeSphere: marshal: %v", err)
	}
	return string(out)
}
```

- [ ] **Step 3: Verify + commit**

```bash
GOOS=js GOARCH=wasm go build -o /tmp/pe-wasm-check ./cmd/planet-explorer/wasm && rm /tmp/pe-wasm-check
go build ./... && golangci-lint run ./cmd/planet-explorer/...
git add cmd/planet-explorer/wasm/main.go
git commit -m "feat(explorer): patchRecomputeSphere export + __pxProgress forwarding"
```

---

### Task 6: Web Worker migration (worker.js + RPC shim)

The core change: the wasm boots inside a dedicated Worker; every call becomes an async RPC. Behavior-preserving — no overlay/cancel yet (Task 7). After this task the tab never blocks.

**Files:**
- Create: `cmd/planet-explorer/web/worker.js`
- Modify: `cmd/planet-explorer/web/app.js` (init L36-46, regenerate L123-186, loadDefaultProfile L75-86, refreshDebugView ~L1912-1928, and the Patch Lab functions L2156-2414), `cmd/planet-explorer/web/index.html` (remove the `wasm_exec.js` script tag, L193)
- Rebuild + commit: `cmd/planet-explorer/web/planet-explorer.wasm` (picks up Task 5's exports)

**Interfaces:**
- Consumes: the 14 wasm globals (13 existing + `patchRecomputeSphere`), `__pxProgress` protocol from Task 5.
- Produces: `rpc(name, ...args) → Promise` and `bootWorker()` in app.js — every later task calls wasm ONLY through `rpc`. Worker protocol: main→worker `{id, name, args}`; worker→main `{type:'ready'}` once, `{type:'progress', id, stage, i, n}` during a call, `{type:'result', id, result}` / `{type:'result', id, error}` per call. Uint8Array results transfer their buffer.

- [ ] **Step 1: Create `cmd/planet-explorer/web/worker.js`:**

```js
// Dedicated Worker that hosts the planet-explorer wasm module so heavy
// computes never block the page's main thread. Protocol:
//   main -> worker: {id, name, args}   (name = a wasm-exported global)
//   worker -> main: {type:'ready'}                       once, after boot
//                   {type:'progress', id, stage, i, n}   during a call
//                   {type:'result', id, result}          per call (Uint8Array
//                                                        buffers transferred)
//                   {type:'result', id, error}           on thrown exception
// Calls run synchronously on this worker thread — messages arriving
// mid-compute queue in the worker's event loop, which serializes RPCs
// in send order for free.
importScripts('wasm_exec.js');

let currentId = 0;

// Defined BEFORE the wasm boots so main.go's registerProgressHooks
// finds it. Forwards Go pipeline progress tagged with the in-flight id.
self.__pxProgress = (stage, i, n) => {
  self.postMessage({ type: 'progress', id: currentId, stage, i, n });
};

const ready = (async () => {
  const go = new Go();
  const result = await WebAssembly.instantiateStreaming(fetch('wasm'), go.importObject);
  // go.run resolves only when the Go program exits (it never does —
  // main blocks on a channel). Exports are installed synchronously
  // before that first block, so we deliberately do not await it.
  go.run(result.instance);
  self.postMessage({ type: 'ready' });
})();

self.onmessage = async (e) => {
  const { id, name, args } = e.data;
  await ready;
  currentId = id;
  let result;
  try {
    result = self[name](...args);
  } catch (err) {
    self.postMessage({ type: 'result', id, error: String(err && err.stack || err) });
    return;
  }
  if (result instanceof Uint8Array) {
    self.postMessage({ type: 'result', id, result }, [result.buffer]);
  } else {
    self.postMessage({ type: 'result', id, result });
  }
};
```

- [ ] **Step 2: Replace the wasm boot in app.js** — replace `init()` (L36-46) and add the shim directly above it:

```js
// ---- Worker RPC ----
// The wasm module lives in a dedicated Worker (web/worker.js) so
// multi-second computes never freeze this thread. Every wasm call goes
// through rpc(); results resolve asynchronously in send order.
let worker = null;
const pendingRPCs = new Map(); // id -> {resolve, reject, name}
let nextRPCId = 1;

function bootWorker() {
  worker = new Worker('worker.js');
  worker.onmessage = (e) => {
    const m = e.data;
    if (m.type === 'ready') {
      wasmReady = true;
      return;
    }
    if (m.type === 'progress') {
      onWorkerProgress(m); // no-op until the busy overlay task
      return;
    }
    const p = pendingRPCs.get(m.id);
    if (!p) return;
    pendingRPCs.delete(m.id);
    if (m.error !== undefined) p.reject(new Error(m.error));
    else p.resolve(m.result);
  };
}

function onWorkerProgress(_m) {} // replaced by the busy-overlay task

function rpc(name, ...args) {
  return new Promise((resolve, reject) => {
    const id = nextRPCId++;
    pendingRPCs.set(id, { resolve, reject, name });
    worker.postMessage({ id, name, args });
  });
}

async function init() {
  status.textContent = 'Loading wasm…';
  bootWorker();
  await refreshPlanetPicker();
  await loadDefaultProfile();
  status.textContent = 'Ready';
}
```

- [ ] **Step 3: Migrate every wasm call site.** Grep first: `grep -n 'planetExplorer\|patchInit\|patchSelect\|patchLayers\|patchSetParam\|patchRender\|patchMinimap' cmd/planet-explorer/web/app.js`. Complete inventory and replacements (all `window.X` existence checks are removed — the wasm is ours, the export always exists):

| Site | Old | New |
|---|---|---|
| `loadDefaultProfile` L75 | `const json = planetExplorerDefaultProfile(type);` | `async function loadDefaultProfile()` … `const json = await rpc('planetExplorerDefaultProfile', type);` |
| `regenerate` L141 | `planetExplorerGenerateHeightmap(profileJSON, seed, size)` | `await rpc('planetExplorerGenerateHeightmap', profileJSON, seed, size)` |
| `regenerate` L142-143 | `debugBypass.size > 0 && window.planetExplorerGenerateWithBypass` … | `debugBypass.size > 0` → `await rpc('planetExplorerGenerateWithBypass', profileJSON, seed, size, JSON.stringify([...debugBypass]))` |
| `regenerate` L145 | `planetExplorerGenerate(profileJSON, seed, size)` | `await rpc('planetExplorerGenerate', profileJSON, seed, size)` |
| `regenerate` L152 | `planetExplorerBakeEquirect(cubePNG, w, h)` | `await rpc('planetExplorerBakeEquirect', cubePNG, w, h)` |
| `regenerate` L164-167 | `if (window.planetExplorerGenerateNight) { const nightCubePNG = planetExplorerGenerateNight(...)` | drop the `if`; `const nightCubePNG = await rpc('planetExplorerGenerateNight', profileJSON, seed, size);` (and `await rpc('planetExplorerBakeEquirect', ...)` inside) |
| `refreshDebugView` ~L1912-1928 | `if (!window.planetExplorerGenerateDebug) {...}` guard + `const result = window.planetExplorerGenerateDebug(...)` | make the function `async`, drop the guard, `const result = await rpc('planetExplorerGenerateDebug', profileJSON, seed, size, bypassJSON);` |
| `enterPatchLab` L2166 | `const initRaw = patchInit(JSON.stringify(profile), seedInput.value, 256);` | `const initRaw = await rpc('patchInit', JSON.stringify(profile), seedInput.value, 256);` |
| `buildLayerRail` L2209 | `const raw = patchLayers();` | `async function buildLayerRail()` … `const raw = await rpc('patchLayers');` |
| `selectCandidate` L2242 | `const err = patchSelect(JSON.stringify(...));` | `const err = await rpc('patchSelect', JSON.stringify(...));` |
| `refreshPatch` L2254 | `const png = patchRender(patchTarget, view);` | `const png = await rpc('patchRender', patchTarget, view);` |
| `refreshMinimap` L2264 | `const png = patchMinimap(w, h);` | `const png = await rpc('patchMinimap', w, h);` |
| `applyProfileToPatch` L2375 | `const raw = patchSetParam(path, JSON.stringify(profile));` | `async function applyProfileToPatch(profile)` … `const raw = await rpc('patchSetParam', path, JSON.stringify(profile));` |
| sea-level listener L2410 | `const raw = patchSetParam('seaLevelView', ...);` | make the listener callback `async`, `const raw = await rpc('patchSetParam', 'seaLevelView', ...);` |

Notes:
- `enterPatchLab` L2197: `buildLayerRail()` → `await buildLayerRail()`.
- Callers of the now-async `applyProfileToPatch` (toggleJitter L206, apply-btn L410, import L434, `patchAwareCommitProfile` L2394) stay fire-and-forget — RPC-queue ordering (`patchSetParam` before any later `patchRender`) preserves correctness. Do NOT convert `commitProfile` to async.
- Uint8Array results arrive reconstructed over the transferred buffer, so `png instanceof Uint8Array` checks and `paintToCanvas` work unchanged.
- The error convention is unchanged: wasm errors are `{"error":...}` STRING results (`isWasmError`), not RPC rejections; RPC rejections happen only on thrown exceptions/cancel.

- [ ] **Step 4: index.html** — delete `<script src="wasm_exec.js"></script>` (L193; the worker `importScripts` it instead).

- [ ] **Step 5: Rebuild the shipped wasm** (Task 5's exports must be in the committed artifact):

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
```

- [ ] **Step 6: Syntax gates**

```bash
node --check cmd/planet-explorer/web/app.js && node --check cmd/planet-explorer/web/worker.js
```
Expected: both silent (exit 0).

- [ ] **Step 7: Manual smoke test** — `go run ./cmd/planet-explorer -addr :8091`, then in a browser: (a) page loads to "Ready"; (b) Regenerate terran at face 256 completes with the tab responsive (select text / scroll during compute; no "Page Unresponsive" dialog); (c) Patch Lab opens, layer rail populates, slider tweak re-renders, Go! hands off; (d) scorched → Patch Lab shows the clean error alert. If the executing agent cannot drive a browser, run the server, `curl -s localhost:8091 | grep worker` sanity-check, and EXPLICITLY list the four browser checks as UNVERIFIED in the task report so the controller carries them to the final user click-through.

- [ ] **Step 8: Commit**

```bash
git add cmd/planet-explorer/web/worker.js cmd/planet-explorer/web/app.js cmd/planet-explorer/web/index.html cmd/planet-explorer/web/planet-explorer.wasm
git commit -m "feat(explorer): move wasm into a Web Worker with promise RPC"
```

---

### Task 7: Busy overlay — progress, whimsy, cancel

**Files:**
- Modify: `cmd/planet-explorer/web/index.html` (overlay markup before `</main>`), `cmd/planet-explorer/web/style.css`, `cmd/planet-explorer/web/app.js` (busy module + `onWorkerProgress` + cancel path)

**Interfaces:**
- Consumes: `rpc`/`pendingRPCs`/`bootWorker` (Task 6), progress messages, canonical stage keys (Tasks 3-4).
- Produces: `cancelCompute()`; drivers treat an RPC rejection whose message is `'cancelled'` as a quiet no-op.

- [ ] **Step 1: index.html** — insert before `</main>` (L183):

```html
<div id="busy-overlay" hidden>
  <div class="busy-box">
    <div class="busy-spinner" aria-hidden="true"></div>
    <div id="busy-whimsy">Working…</div>
    <div id="busy-stage"></div>
    <div class="busy-bar"><div id="busy-bar-fill"></div></div>
    <button id="busy-cancel" type="button" title="Terminate the compute. Cancelling mid-Patch-Lab resets the patch session.">Cancel</button>
  </div>
</div>
```

- [ ] **Step 2: style.css** — append:

```css
/* Busy overlay: shown when a worker RPC exceeds 150 ms. The spinner
   animates transform only, so it keeps spinning on the compositor
   even while this thread is briefly busy painting results. */
#busy-overlay {
  position: fixed; inset: 0; z-index: 1000;
  background: rgba(10, 12, 18, 0.55);
  display: flex; align-items: center; justify-content: center;
}
.busy-box {
  background: #1c2028; border: 1px solid #444; border-radius: 8px;
  padding: 24px 32px; text-align: center; min-width: 320px; color: #ddd;
}
.busy-spinner {
  width: 36px; height: 36px; margin: 0 auto 12px;
  border: 4px solid #333; border-top-color: #6cf; border-radius: 50%;
  animation: busy-spin 0.9s linear infinite;
}
@keyframes busy-spin { to { transform: rotate(360deg); } }
#busy-whimsy { font-size: 1.05em; margin-bottom: 4px; }
#busy-stage { font-size: 0.85em; color: #8a93a5; min-height: 1.2em; margin-bottom: 10px; }
.busy-bar { height: 6px; background: #333; border-radius: 3px; overflow: hidden; margin-bottom: 14px; }
#busy-bar-fill { height: 100%; width: 0%; background: #6cf; transition: width 0.2s; }
```

- [ ] **Step 3: app.js busy module** — add below the RPC shim; replace the `onWorkerProgress` stub; wire `rpc()`:

```js
// ---- Busy overlay ----
// Shown when any RPC is still pending after SHOW_DELAY_MS, so quick
// param sets never flash a spinner. Whimsy is cosmetic; the honest
// stage key and i/n counter always render beside it.
const BUSY_SHOW_DELAY_MS = 150;
const busyOverlay = $('#busy-overlay');
const busyWhimsy = $('#busy-whimsy');
const busyStage = $('#busy-stage');
const busyBarFill = $('#busy-bar-fill');
let busyTimer = null;
let lastWhimsyKey = '';

const RPC_LABELS = {
  planetExplorerGenerate: 'Rendering the planet',
  planetExplorerGenerateNight: 'Turning on the city lights',
  planetExplorerGenerateHeightmap: 'Measuring the mountains',
  planetExplorerGenerateWithBypass: 'Rendering the planet (with bypasses)',
  planetExplorerBakeEquirect: 'Flattening the globe',
  planetExplorerGenerateDebug: 'Rendering every pipeline stage',
  planetExplorerDefaultProfile: 'Fetching defaults',
  patchInit: 'Surveying the whole sphere',
  patchSelect: 'Cutting out your patch',
  patchLayers: 'Listing the layers',
  patchSetParam: 'Applying the tweak',
  patchRender: 'Painting the patch',
  patchMinimap: 'Drawing the minimap',
  patchRecomputeSphere: 'Recomputing the sphere',
};

const WHIMSY = {
  'sphere:jitter': ['Wobbling the crust (on purpose)'],
  'sphere:plates': ['Smashing continental plates together', 'Filing tectonic grievances'],
  'sphere:crust': ['Baking a fresh planetary crust', 'Arranging cratons like furniture'],
  'sphere:fx': ['Classifying mountain-making collisions'],
  'sphere:splines': ['Sculpting hills with cubic splines'],
  'sphere:tectonic-fx': ['Making mountains out of molehills'],
  'sphere:smooth': ['Sanding down the rough edges'],
  'sphere:normalize': ['Convincing the peaks to fit in [0,1]'],
  'sphere:erode': ['Raining on the mountains for a few eons', 'Hiding dinosaur bones'],
  'sphere:flow': ['Rerouting rivers for scenic value'],
  'layer:tectonic-base': ['Laying the tectonic foundation'],
  'layer:tectonic-fx': ['Crumpling the crust artistically'],
  'layer:control-noise': ['Seasoning with fractal noise'],
  'layer:height-smooth': ['Buffing out the pixel wrinkles'],
  'layer:normalize': ['Renormalizing with bureaucratic rigor'],
  'layer:coastal': ['Nibbling the coastlines'],
  'layer:erosion': ['Applying 10,000 years of drizzle', 'Hiding dinosaur bones'],
  'layer:craters': ['Throwing rocks from space'],
  'layer:flow-rivers': ['Teaching water to flow downhill'],
  'layer:climate': ['Negotiating with the rain shadow'],
  'layer:biome-color': ['Coloring inside the biome lines'],
  'layer:waterlines': ['Filling the oceans — do not disturb'],
  'layer:civ': ['Zoning land for tiny civilizations', 'Approving planning permission'],
  Crust: ['Baking a fresh planetary crust'],
  ControlFields: ['Seasoning with fractal noise'],
  Ridged: ['Extruding dramatic ridge lines'],
  TectonicFX: ['Making mountains out of molehills'],
  Basin: ['Digging decorative basins'],
  Continents: ['Rolling out the continents'],
  HeightSmooth: ['Sanding down the rough edges'],
  Normalize: ['Convincing the peaks to fit in [0,1]'],
  Coastal: ['Nibbling the coastlines'],
  Erosion: ['Applying 10,000 years of drizzle', 'Hiding dinosaur bones'],
  Flow: ['Teaching water to flow downhill'],
  Craters: ['Throwing rocks from space'],
  Palette: ['Mixing planetary paint'],
  Snow: ['Dusting the peaks with snow'],
  Ocean: ['Filling the oceans'],
  PolarCaps: ['Icing the poles'],
  Shading: ['Adding dramatic lighting'],
  Ejecta: ['Splattering crater ejecta tastefully'],
  Civ: ['Zoning land for tiny civilizations'],
  LUT: ['Applying the cinematic color grade'],
  'render:jitter': ['Wobbling the crust (on purpose)'],
  'render:plates': ['Smashing continental plates together'],
  'render:heightmap': ['Raising mountains, digging seas'],
  'render:flow': ['Rerouting rivers for scenic value'],
  'render:colorize': ['Arguing about paint colors'],
};

function busyMaybeShow() {
  if (busyTimer !== null || !busyOverlay.hidden) return;
  busyTimer = setTimeout(() => {
    busyTimer = null;
    if (pendingRPCs.size === 0) return;
    const first = pendingRPCs.values().next().value;
    busyWhimsy.textContent = RPC_LABELS[first.name] || 'Working…';
    busyStage.textContent = '';
    busyBarFill.style.width = '0%';
    lastWhimsyKey = '';
    busyOverlay.hidden = false;
  }, BUSY_SHOW_DELAY_MS);
}

function busyMaybeHide() {
  if (pendingRPCs.size > 0) return;
  if (busyTimer !== null) { clearTimeout(busyTimer); busyTimer = null; }
  busyOverlay.hidden = true;
}

function onWorkerProgress(m) {
  if (busyOverlay.hidden) return;
  if (m.stage !== lastWhimsyKey) {
    lastWhimsyKey = m.stage;
    const pool = WHIMSY[m.stage];
    if (pool) busyWhimsy.textContent = pool[Math.floor(Math.random() * pool.length)];
  }
  busyStage.textContent = m.n > 0 ? `${m.stage} (${m.i}/${m.n})` : m.stage;
  if (m.n > 0) busyBarFill.style.width = `${Math.round((m.i / m.n) * 100)}%`;
}
```

Wire the shim: in `rpc()` call `busyMaybeShow()` right after `pendingRPCs.set(...)`; in the `worker.onmessage` result branch call `busyMaybeHide()` right after resolving/rejecting.

- [ ] **Step 4: Cancel** — add below the busy module and register the button:

```js
// cancelCompute kills the worker mid-compute. All in-flight RPCs
// reject with 'cancelled'; the wasm-side patch session dies with the
// worker, so an open Patch Lab is exited (cancel means abandon).
function cancelCompute() {
  worker.terminate();
  for (const p of pendingRPCs.values()) p.reject(new Error('cancelled'));
  pendingRPCs.clear();
  wasmReady = false;
  busyMaybeHide();
  bootWorker();
  if (patchOn) {
    exitPatchLab();
    status.textContent = 'Cancelled — Patch Lab session reset';
  } else {
    status.textContent = 'Cancelled';
  }
}
const busyCancelBtn = $('#busy-cancel');
if (busyCancelBtn) busyCancelBtn.addEventListener('click', cancelCompute);
```

- [ ] **Step 5: Quiet-cancel the drivers.** Every async driver that awaits `rpc` directly (`regenerate`, `loadDefaultProfile`, `enterPatchLab`, `selectCandidate`, `refreshPatch`, `refreshMinimap`, `buildLayerRail`, `applyProfileToPatch`, `refreshDebugView`, the sea-level listener) gets its awaits wrapped so a cancel doesn't spray console errors. Add one helper and use it at each driver's top level:

```js
// quiet wraps a driver's RPC promise so it NEVER rejects (drivers are
// often fire-and-forget; a rejection would surface as an unhandled-
// rejection console error). A cancelCompute() rejection is expected
// and silent; anything else lands in the status line and the console.
// Callers must treat a null result as "stop this driver".
function quiet(p) {
  return p.catch((e) => {
    if (!(e && e.message === 'cancelled')) {
      status.textContent = 'Error: ' + (e && e.message || e);
      console.warn(e);
    }
    return null;
  });
}
```

Pattern per driver: `const cubePNG = await quiet(rpc('planetExplorerGenerate', ...)); if (cubePNG === null) return;`. Apply to every await site listed in Task 6's table.

- [ ] **Step 6: Gates + manual check + commit**

```bash
node --check cmd/planet-explorer/web/app.js
```
Manual (or report UNVERIFIED): face-512 generate shows the overlay with rotating whimsy + stage counter and no unresponsive dialog; Cancel mid-generate returns to a working UI; Cancel mid-Patch-Lab exits the lab with the reset message; a quick `patchSetParam`-only tweak shows no overlay flash.

```bash
git add cmd/planet-explorer/web/index.html cmd/planet-explorer/web/style.css cmd/planet-explorer/web/app.js
git commit -m "feat(explorer): busy overlay with progress, whimsy descriptors, and cancel"
```

---

### Task 8: Latest-wins render coalescing

Async RPC makes render flooding possible: a slider drag emits dozens of `input` events → dozens of queued `patchRender`s. Add an in-flight guard so at most one render is queued behind the running one, always with the newest state. Also guard `regenerate` against double-fire.

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (`refreshPatch` L~2251, `scheduleRefreshPatch` L~2277, `regenerate`)

**Interfaces:**
- Consumes: `rpc`, `quiet` (Task 7).
- Produces: `refreshPatch()` safe to call at any rate; `regenerate()` re-entrant-safe.

- [ ] **Step 1: Replace `refreshPatch` with the coalescing version:**

```js
// Latest-wins coalescing: while a patchRender RPC is in flight, any
// number of further refresh requests collapse into ONE queued flag;
// when the running render lands, exactly one more runs with the
// NEWEST target/view. A drag can never queue 30 stale renders.
let patchRenderInFlight = false;
let patchRenderQueued = false;

async function refreshPatch() {
  if (!patchOn) return;
  if (patchRenderInFlight) { patchRenderQueued = true; return; }
  patchRenderInFlight = true;
  try {
    do {
      patchRenderQueued = false;
      const view = patchViewSel ? patchViewSel.value : 'color';
      const png = await quiet(rpc('patchRender', patchTarget, view));
      if (png === null) return; // cancelled
      if (!(png instanceof Uint8Array)) {
        status.textContent = 'Patch Lab error: ' + wasmErrorMessage(png);
        return;
      }
      await paintToCanvas(patchCanvas, png);
    } while (patchRenderQueued && patchOn);
  } finally {
    patchRenderInFlight = false;
  }
}
```

Keep `scheduleRefreshPatch`'s 150 ms debounce as-is on top (it batches the *param* churn; the in-flight guard batches the *render* churn).

- [ ] **Step 2: Guard `regenerate`** — add at the top of `regenerate()`:

```js
  if (regenerate.inFlight) return;
  regenerate.inFlight = true;
  renderBtn.disabled = true;
```

and wrap the body so it always releases (make the existing body a `try` block with):

```js
  } finally {
    regenerate.inFlight = false;
    renderBtn.disabled = false;
  }
```

- [ ] **Step 3: Gates + manual check + commit**

```bash
node --check cmd/planet-explorer/web/app.js
```
Manual (or report UNVERIFIED): drag an FX slider rapidly in Patch Lab — the canvas updates smoothly and settles on the final value; the Network/console shows no runaway queue; double-clicking Regenerate does not double-render.

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "feat(explorer): latest-wins coalescing for patch renders, re-entrancy guard for regenerate"
```

---

### Task 9: Flow / RainShadow / Civ slider panels

Three new panels following the existing `renderCurlPanel` pattern (makePanel + panelControls + makeNumberRow + commitProfile). **JSON keys are lowercase** (`profile.flow`, `profile.rainShadow`, `profile.civ`) and `commitProfile` is already patch-aware, so paths like `flow.riverThreshold` hit the right layers (8, 9, 12) with zero extra wiring.

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (three new functions + three calls in `renderPanels()` after `renderErosionPanel(profile, panels);` L479)

**Interfaces:**
- Consumes: `makePanel`, `panelControls`, `makeAuxBtn`, `makeNumberRow`, `commitProfile`, `originalProfile`, `round2`.

- [ ] **Step 1: Add the three panel functions** (place after `renderErosionPanel`'s definition — find it with `grep -n 'function renderErosionPanel' app.js`):

```js
function renderFlowPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Rivers (Flow)',
    'Planchon-Darboux fill + D8 flow accumulation carves river channels into the heightmap. RiverThreshold = 0 disables the pass entirely. Profile JSON key: "flow".');
  if (!profile.flow) profile.flow = { riverThreshold: 0, riverDepth: 0 };

  const reset = () => {
    const orig = (originalProfile && originalProfile.flow) || {};
    profile.flow = { riverThreshold: orig.riverThreshold || 0, riverDepth: orig.riverDepth || 0 };
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => {
    profile.flow = { riverThreshold: 0, riverDepth: 0 };
    commitProfile(profile);
    renderPanels();
  };
  const controls = panelControls(panel);
  controls.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  controls.appendChild(makeAuxBtn('Clear', 'Disable river carving (RiverThreshold = 0)', clear));

  panel.appendChild(makeNumberRow('RiverThreshold',
    'Flow-accumulation cutoff above which a cell becomes river. 0 disables. Lower = more rivers (archetype defaults: 200–1500).',
    profile.flow.riverThreshold ?? 0, 0, 5000, '10',
    v => { profile.flow.riverThreshold = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('RiverDepth',
    'Height units carved along river channels (defaults: 0.005–0.025).',
    profile.flow.riverDepth ?? 0, 0, 0.1, '0.001',
    v => { profile.flow.riverDepth = v; commitProfile(profile); }));

  panels.appendChild(panel);
}

function renderRainShadowPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Rain Shadow',
    'Orographic rainfall: an upwind walk over the heightmap boosts windward rain and dries leeward slopes. WalkSteps = 0 disables the pass entirely. Profile JSON key: "rainShadow".');
  if (!profile.rainShadow) {
    profile.rainShadow = { walkSteps: 0, stepArcRad: 0, mountainCutoff: 0, windRainBoost: 0, leeFactor: 0 };
  }
  const rs = profile.rainShadow;

  const reset = () => {
    const orig = (originalProfile && originalProfile.rainShadow) || {};
    profile.rainShadow = {
      walkSteps: orig.walkSteps || 0, stepArcRad: orig.stepArcRad || 0,
      mountainCutoff: orig.mountainCutoff || 0, windRainBoost: orig.windRainBoost || 0,
      leeFactor: orig.leeFactor || 0,
    };
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => {
    profile.rainShadow = { walkSteps: 0, stepArcRad: 0, mountainCutoff: 0, windRainBoost: 0, leeFactor: 0 };
    commitProfile(profile);
    renderPanels();
  };
  const controls = panelControls(panel);
  controls.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  controls.appendChild(makeAuxBtn('Clear', 'Disable rain shadow (WalkSteps = 0)', clear));

  panel.appendChild(makeNumberRow('WalkSteps',
    'Upwind walk length in steps. 0 disables (default 12).',
    rs.walkSteps ?? 0, 0, 60, '1',
    v => { profile.rainShadow.walkSteps = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('StepArcRad',
    'Arc length of one walk step in radians (default 0.087 ≈ 5°).',
    rs.stepArcRad ?? 0, 0, 0.3, '0.001',
    v => { profile.rainShadow.stepArcRad = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('MountainCutoff',
    'Height threshold for orographic uplift (defaults 0.55–0.65).',
    rs.mountainCutoff ?? 0, 0, 1, '0.01',
    v => { profile.rainShadow.mountainCutoff = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('WindRainBoost',
    'Upwind rainfall multiplier minus 1 (defaults 0.2–0.4).',
    rs.windRainBoost ?? 0, 0, 2, '0.05',
    v => { profile.rainShadow.windRainBoost = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('LeeFactor',
    'Leeward drying strength (defaults 0.05–0.15).',
    rs.leeFactor ?? 0, 0, 1, '0.01',
    v => { profile.rainShadow.leeFactor = v; commitProfile(profile); }));

  panels.appendChild(panel);
}

function renderCivPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Civilization',
    'Settlement sites (Bridson-spaced), farmland, roads, and the Black-Marble nightside. Tier = 0 disables civ entirely. Profile JSON key: "civ".');
  if (!profile.civ) {
    profile.civ = { tier: 0, siteMinDistRad: 0, siteMaxDistRad: 0, maxPopulation: 0, nightLightHue: 0, agricultureRatio: 0 };
  }
  const cv = profile.civ;

  const reset = () => {
    const orig = (originalProfile && originalProfile.civ) || {};
    profile.civ = {
      tier: orig.tier || 0, siteMinDistRad: orig.siteMinDistRad || 0,
      siteMaxDistRad: orig.siteMaxDistRad || 0, maxPopulation: orig.maxPopulation || 0,
      nightLightHue: orig.nightLightHue || 0, agricultureRatio: orig.agricultureRatio || 0,
    };
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => {
    profile.civ = { tier: 0, siteMinDistRad: 0, siteMaxDistRad: 0, maxPopulation: 0, nightLightHue: 0, agricultureRatio: 0 };
    commitProfile(profile);
    renderPanels();
  };
  const controls = panelControls(panel);
  controls.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  controls.appendChild(makeAuxBtn('Clear', 'Disable civilization (Tier = 0)', clear));

  panel.appendChild(makeNumberRow('Tier',
    '0 = disabled; 1 = full civilization (terran default 0.5).',
    cv.tier ?? 0, 0, 1, '0.05',
    v => { profile.civ.tier = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('SiteMinDistRad',
    'Bridson minimum site separation, radians (default 0.0314).',
    cv.siteMinDistRad ?? 0, 0, 0.3, '0.001',
    v => { profile.civ.siteMinDistRad = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('SiteMaxDistRad',
    'Bridson maximum site separation, radians (default 0.1047).',
    cv.siteMaxDistRad ?? 0, 0, 0.5, '0.001',
    v => { profile.civ.siteMaxDistRad = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('MaxPopulation',
    'Population scale for the most populous site (default 1.0).',
    cv.maxPopulation ?? 0, 0, 10, '0.1',
    v => { profile.civ.maxPopulation = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('NightLightHue',
    'Hue (0..1) of nightside city lights; 0.12 ≈ sodium orange.',
    cv.nightLightHue ?? 0, 0, 1, '0.01',
    v => { profile.civ.nightLightHue = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('AgricultureRatio',
    'Farmland-to-city area ratio (default 0.4).',
    cv.agricultureRatio ?? 0, 0, 2, '0.05',
    v => { profile.civ.agricultureRatio = v; commitProfile(profile); }));

  panels.appendChild(panel);
}
```

- [ ] **Step 2: Register them** — in `renderPanels()` after `renderErosionPanel(profile, panels);` (L479) insert:

```js
  renderFlowPanel(profile, panels);
  renderRainShadowPanel(profile, panels);
  renderCivPanel(profile, panels);
```

- [ ] **Step 3: Gates + manual check + commit**

```bash
node --check cmd/planet-explorer/web/app.js
```
Manual (or report UNVERIFIED): terran shows all three panels with the archetype defaults populated; in Patch Lab, tweaking RiverThreshold re-renders from the Rivers layer (fast — layers 0-7 cached); tweaking a Civ param re-runs only the final layer; Clear on Flow disables rivers on the next full render.

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "feat(explorer): Flow, RainShadow, and Civ slider panels"
```

---

### Task 10: Recompute-sphere button

**Files:**
- Modify: `cmd/planet-explorer/web/index.html` (patch-lab-controls, L154-167), `cmd/planet-explorer/web/app.js`

**Interfaces:**
- Consumes: `patchRecomputeSphere` RPC (Task 5), `rpc`/`quiet`, `buildLayerRail`, `refreshMinimap`, `refreshPatch`.

- [ ] **Step 1: index.html** — inside `.patch-lab-controls`, after the "Next window" button:

```html
        <button id="patch-recompute" title="Re-run the sphere-global precompute with the current profile, resyncing HMin/HMax/SeaLevel0/SeaLevel after heavy FX tuning (the documented stale-scalar divergence). Slow — full progress bar applies.">Recompute sphere</button>
```

- [ ] **Step 2: app.js** — next to the other patch button listeners (~L2397):

```js
const patchRecomputeBtn = $('#patch-recompute');
if (patchRecomputeBtn) {
  patchRecomputeBtn.addEventListener('click', async () => {
    if (!patchOn) return;
    const raw = await quiet(rpc('patchRecomputeSphere'));
    if (raw === null) return; // cancelled
    if (isWasmError(raw)) {
      status.textContent = 'Recompute failed: ' + wasmErrorMessage(raw);
      return;
    }
    let data;
    try { data = JSON.parse(raw); } catch { data = {}; }
    if (patchSeaLevelInput && typeof data.seaLevel === 'number') {
      patchSeaLevelInput.value = data.seaLevel;
    }
    await buildLayerRail();
    await refreshMinimap();
    await refreshPatch();
    status.textContent = 'Sphere recomputed — scalars resynced';
  });
}
```

- [ ] **Step 3: Gates + manual check + commit**

```bash
node --check cmd/planet-explorer/web/app.js
```
Manual (or report UNVERIFIED): in Patch Lab, crank TectonicFX params, click Recompute sphere — progress overlay shows `sphere:*` stages, sea-level slider updates, patch re-renders; Cancel mid-recompute resets the session cleanly.

```bash
git add cmd/planet-explorer/web/index.html cmd/planet-explorer/web/app.js
git commit -m "feat(explorer): Recompute-sphere button resyncs stale sphere scalars"
```

---

### Task 11: UI polish trio — layer-rail refresh, patch-mode headers, sea-level hint

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (`scheduleRefreshPatch`, `enterPatchLab`, `exitPatchLab`), `cmd/planet-explorer/web/index.html` (sea-level label), `cmd/planet-explorer/web/style.css`

- [ ] **Step 1: Layer-rail label refresh** — a param change can flip a layer's `Enabled` gate (e.g. setting Civ Tier > 0), but the rail labels were built once. In `scheduleRefreshPatch`'s timeout callback, refresh the rail alongside the render:

```js
function scheduleRefreshPatch() {
  if (patchRefreshTimer) clearTimeout(patchRefreshTimer);
  patchRefreshTimer = setTimeout(() => {
    patchRefreshTimer = null;
    refreshPatch();
    buildLayerRail(); // "(disabled)" labels track live Enabled gates
  }, 150);
}
```

(`buildLayerRail` preserves the active selection via `patchTarget`, so the rebuild is invisible except for label changes.)

- [ ] **Step 2: Hide stale viewport chrome in patch mode** — the three canvas `<h2>`s, the sphere hint, and the (empty) canvas wrapper divs stay visible over blank space when Patch Lab opens. In `enterPatchLab()` (beside the `hidden` toggles) add `document.body.classList.add('patch-mode');` and in `exitPatchLab()` add `document.body.classList.remove('patch-mode');`. Then in style.css:

```css
/* Patch mode replaces the whole viewport; hide the normal-view chrome
   (headers, hint, canvas wrappers) instead of leaving orphaned
   headings above blank space. */
body.patch-mode .viewport > h2,
body.patch-mode .sphere-hint,
body.patch-mode .planet-sphere,
body.patch-mode .planet-cube,
body.patch-mode .planet-equirect {
  display: none;
}
```

- [ ] **Step 3: Sea-level view-only hint** — in index.html change the sea-level label (L163-165) to:

```html
        <label>Sea level
          <input id="patch-sealevel" type="range" min="0" max="1" step="0.005"
                 title="Preview-only waterline override. NOT written to the profile — Go! renders with the profile's own sea level.">
          <small class="sealevel-hint">view only — not applied on Go!</small>
        </label>
```

and in style.css:

```css
.sealevel-hint { color: #8a93a5; font-size: 0.75em; margin-left: 4px; }
```

- [ ] **Step 4: Gates + manual check + commit**

```bash
node --check cmd/planet-explorer/web/app.js
```
Manual (or report UNVERIFIED): entering Patch Lab hides the three headers/hint (no blank-space headings); exiting restores them; setting Civ Tier 0→0.5 updates the rail's "(disabled)" label within ~150 ms; the sea-level hint is visible.

```bash
git add cmd/planet-explorer/web/app.js cmd/planet-explorer/web/index.html cmd/planet-explorer/web/style.css
git commit -m "feat(explorer): layer-rail refresh, patch-mode chrome hiding, sea-level hint"
```

---

### Task 12: Docs + full gates + click-through checklist

**Files:**
- Modify: `cmd/planet-explorer/README.md` (Patch Lab section), `cmd/planet-explorer/USER_GUIDE.md` (if it describes the render flow)

- [ ] **Step 1: Update README** — in the Patch Lab section add a short "Architecture: Web Worker" paragraph covering: the wasm runs in `web/worker.js` (main thread never blocks); progress protocol (`__pxProgress` → `{type:'progress'}` messages → busy overlay); Cancel = worker terminate + respawn, which resets any open patch session; the Recompute-sphere button resolves the §7 stale-scalar divergence on demand (update the divergence text to say "resync on demand via Recompute sphere, or by re-entering Patch Lab"). Mention the three new panels (Flow / Rain Shadow / Civilization) in whatever panel list exists. Skim USER_GUIDE.md and update only statements the worker migration made false.

- [ ] **Step 2: Full verification battery**

```bash
go build ./...
go test ./... 2>&1 | tail -5
GOOS=js GOARCH=wasm go build -o /tmp/pe-wasm-check ./cmd/planet-explorer/wasm && rm /tmp/pe-wasm-check
node --check cmd/planet-explorer/web/app.js
node --check cmd/planet-explorer/web/worker.js
golangci-lint run
```
Expected: all clean. Also confirm the committed wasm is current: rebuild `cmd/planet-explorer/web/planet-explorer.wasm` and `git status` must show it unchanged (if it changed, a UI task forgot to rebuild — rebuild and include it here).

- [ ] **Step 3: Commit docs**

```bash
git add cmd/planet-explorer/README.md cmd/planet-explorer/USER_GUIDE.md
git commit -m "docs(explorer): worker architecture, cancel semantics, new panels"
```

- [ ] **Step 4: Produce the user click-through checklist** in the task report (the human runs it in a real browser; agents cannot):

1. `go run ./cmd/planet-explorer -addr :8090` (or the user's running :8080 instance after pull) — page reaches "Ready".
2. Terran, face 512, Regenerate: tab stays responsive (scroll/select during compute), busy overlay appears with whimsy + stage counter, **zero** "Page Unresponsive" dialogs.
3. Cancel mid-generate: overlay closes, status "Cancelled", next Regenerate works.
4. Patch Lab (terran): opens with rail + minimap; normal-view headers hidden; sea-level hint visible.
5. Drag an FX slider fast: smooth updates, settles on final value (<1 s feel), no queue buildup.
6. Set Civ Tier 0 → 0.5 in the new Civilization panel: rail's "civ (disabled)" label refreshes; civ layer renders sites.
7. New Rivers/Rain Shadow panels drive their layers (threshold change re-renders fast from layer 8).
8. Crank TectonicFX, click Recompute sphere: `sphere:*` progress stages stream, sea level updates, patch re-renders.
9. Cancel mid-recompute: Patch Lab exits with "session reset" message; re-entering works.
10. Scorched → Patch Lab: clean error alert, normal view intact.
11. Go! from a tuned patch: full-res render matches the tuned look.

---

## Task Dependency Notes

- Tasks 1-4 are pure Go and independent of each other except Task 3/4 sharing the hook pattern; execute in order anyway (SDD is sequential).
- Task 5 needs Tasks 3+4 (imports the hooks). Task 6 needs Task 5 (ships the rebuilt wasm with the new export). Tasks 7-11 need Task 6 (`rpc`). Task 10 needs Task 5's export. Task 12 is last.
- The shipped `web/planet-explorer.wasm` is rebuilt in Task 6 and verified current in Task 12; Go-only tasks (1-5) do NOT rebuild it (harmless: exports unused until Task 6 ships).
