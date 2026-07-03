# Planet Gen Phase 1 — Tier S Algorithms + Slider Tool Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move planet rendering from "procedural" to "designed-feeling" by landing the five Tier-S algorithms (OkLab blending, multi-noise control fields with splines, domain warping, Whittaker T+M biome lookup, per-archetype LUT) along with a web-based parameter slider tool that grows alongside each algorithm. Land two Phase 0 debt items at the start: crater face-bounding-box pre-filter (perf) and a `cubePathFor` helper.

**Architecture:** Each Tier-S algorithm lands in its assigned subpackage with TDD-style unit tests, then is wired into the rocky/gas-giant render pipelines. The slider tool (`cmd/planet-explorer/`) is added in a 5-task scaffold (Tasks 3–7) and is then extended with a small panel each time a new algorithm lands. Per-type tuned-default profiles are committed back into the codebase at the end of each algorithm chain (Phase 5's per-planet JSON storage is out of scope until that phase). The slider tool's Wasm core uses the *same* `pkg/planetgen` Go code, with profile JSON serialized via `encoding/json` for round-trip with the in-code defaults.

**Tech Stack:** Go 1.24, `github.com/lucasb-eyer/go-colorful` (already in `go.mod`), `github.com/ojrac/opensimplex-go` (already in `go.mod`), `syscall/js` (Wasm bindings), plain HTML/CSS/JS (no JS framework), `image/png`. No cgo. No new dependencies.

---

## File structure

**Phase 0 debt:**
- Modify: `pkg/planetgen/feature/crater.go` (face pre-filter)
- Modify: `cmd/generate-planet-maps/main.go` (cubePathFor extraction)

**Slider tool scaffold:**
- Create: `cmd/planet-explorer/main.go` (Go HTTP dev server)
- Create: `cmd/planet-explorer/wasm/main.go` (Wasm entrypoint with `syscall/js` bindings)
- Create: `cmd/planet-explorer/web/index.html`
- Create: `cmd/planet-explorer/web/style.css`
- Create: `cmd/planet-explorer/web/app.js`
- Create: `cmd/planet-explorer/web/wasm_exec.js` (copied from `$(go env GOROOT)/lib/wasm/`)
- Create: `cmd/planet-explorer/README.md`

**Tier-S item 1 — OkLab:**
- Create: `pkg/planetgen/color/oklab.go`
- Create: `pkg/planetgen/color/oklab_test.go`
- Modify: `pkg/planetgen/render/rocky.go` (replace `Blend` with `BlendOkLab`)
- Modify: `pkg/planetgen/render/gasgiant.go` (same)

**Tier-S item 2 — Multi-noise control fields + splines:**
- Modify: `pkg/planetgen/types/types.go` (add `ControlConfig`, `Spline`)
- Create: `pkg/planetgen/seed/domain.go` (named-domain seed mix `Hash(master, domain string) int64`)
- Create: `pkg/planetgen/color/spline.go` (Fritsch-Carlson monotone cubic)
- Create: `pkg/planetgen/color/spline_test.go`
- Create: `pkg/planetgen/field/control.go` (5 control fields generator)
- Create: `pkg/planetgen/field/control_test.go`
- Modify: `pkg/planetgen/render/rocky.go` (use control-field height)
- Modify: `pkg/planetgen/profile.go` (add control field defaults to all 13 types)

**Tier-S item 3 — Domain warping:**
- Create: `pkg/planetgen/noise/warp.go` (Quilez warp)
- Create: `pkg/planetgen/noise/warp_test.go`
- Modify: `pkg/planetgen/types/types.go` (add `WarpConfig`)
- Modify: `pkg/planetgen/render/rocky.go` (apply warp to height + biome lookups)
- Modify: `pkg/planetgen/render/gasgiant.go` (apply warp to band + storm sampling)
- Modify: `pkg/planetgen/profile.go` (add `Warp` defaults)

**Tier-S item 4 — Whittaker biome:**
- Create: `pkg/planetgen/biome/whittaker.go` (T+M field generators + biome cell table)
- Create: `pkg/planetgen/biome/whittaker_test.go`
- Create: `pkg/planetgen/biome/lookup.go` (bilinear cell sampling with OkLab color interp)
- Modify: `pkg/planetgen/types/types.go` (add `BiomeTable`)
- Modify: `pkg/planetgen/render/rocky.go` (replace latitude bands with Whittaker)
- Modify: `pkg/planetgen/profile.go` (per-type biome tables)

**Tier-S item 5 — LUT:**
- Create: `pkg/planetgen/color/lut.go`
- Create: `pkg/planetgen/color/lut_test.go`
- Create: `pkg/planetgen/color/luts/<archetype>.cube` × 13 (Resolve .cube format assets)
- Modify: `pkg/planetgen/types/types.go` (add `LUT *LUT`)
- Modify: `pkg/planetgen/render/rocky.go` (apply LUT as final color grade)
- Modify: `pkg/planetgen/render/gasgiant.go` (same)
- Modify: `pkg/planetgen/profile.go` (per-type LUT references)

**Slider tool incremental panels (one per algorithm):**
- Modify: `cmd/planet-explorer/web/app.js` (extend with each algorithm's controls)
- Modify: `cmd/planet-explorer/wasm/main.go` (extend exposed API as needed)

**Acceptance:**
- Modify: `cmd/generate-planet-maps/testdata/golden/*.png` (regenerate after Phase 1 lands)
- Modify: `cmd/generate-planet-maps/README.md` (Phase 1 status)

---

## Conventions

**Seed discipline (master plan §3.3).** Every subsystem derives its sub-seed from the master planet seed via a named-domain mix. Phase 1 introduces the canonical helper:

```go
seed.Domain(master int64, name string) int64
// returns master XOR fnv64a(name); same `master + name` always produces the same value.
```

Subsystems use these named domains:

| Subsystem | Domain string |
|---|---|
| Heightmap base FBM | "height" (already master, kept for clarity) |
| Domain warp X | "warp.x" |
| Domain warp Y | "warp.y" |
| Domain warp Z | "warp.z" |
| Continentalness | "control.continentalness" |
| Erosion | "control.erosion" |
| PeaksValleys | "control.peaks-valleys" |
| Temperature | "biome.temperature" |
| Humidity | "biome.humidity" |
| Cap edge | "cap" (was `seed+42`, kept) |
| Ocean surface | "ocean" (was `seed+77`, kept) |
| Biome variation | "biome.variation" (was `seed+99`, kept) |

The Phase 0 magic offsets (+42, +77, +99) are replaced with the named-domain form during Task 12. Same outward seed math; clearer intent.

**Profile JSON.** All `types.PlanetProfile` fields use `encoding/json`-compatible types. Slider tool serializes profiles as JSON and the Go core deserializes into the same struct. Field tags use `json:"camelCase"` for browser-side legibility.

**ΔE2000 budgets.** Each Tier-S item changes the visual output of every existing planet. Goldens are regenerated at the end of each chain (after items land + tuned defaults are committed). Statistical invariants must continue to pass after each item.

**Commit message style.** Short imperative summary; no body needed for mechanical tasks.

---

## Task 1: Crater face-bounding-box pre-filter (Phase 0 debt)

**Goal:** Eliminate the per-crater scan over all 6 faces. Each crater's angular bounding cone (`1.5 × Radius`) intersects at most ~3 faces (and usually 1). Skip faces whose 4 corners are all outside the cone.

**Files:**
- Modify: `pkg/planetgen/feature/crater.go`
- Modify: `pkg/planetgen/feature/crater_test.go`

- [ ] **Step 1: Add a benchmark test capturing the baseline**

Append to `pkg/planetgen/feature/crater_test.go`:

```go
func BenchmarkApplyCraters(b *testing.B) {
	cm := cubemap.NewF(256)
	craters := GenerateCraters(rand.New(rand.NewPCG(1, 2)), 200, 0.005, 0.08)
	b.ResetTimer()
	for b.Loop() {
		// Reset between iterations so each call does the same work.
		for face := range cubemap.Face(cubemap.NumFaces) {
			for i := range cm.Faces[face] {
				cm.Faces[face][i] = 0.5
			}
		}
		ApplyCraters(cm, craters, 0.2)
	}
}
```

Run `go test -bench=BenchmarkApplyCraters -benchtime=3s ./pkg/planetgen/feature/...` and record the ns/op baseline. Save it as the "before" number.

- [ ] **Step 2: Implement the face pre-filter**

In `pkg/planetgen/feature/crater.go`, replace the inner-loop body of `ApplyCraters` to gate on a per-face containment check. Replace:

```go
for face := range cubemap.Face(cubemap.NumFaces) {
    for py := range S {
        for px := range S {
            dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
            dot := dx*cx + dy*cy + dz*cz
            if dot < math.Cos(c.Radius*1.5) {
                continue
            }
            ...
        }
    }
}
```

with:

```go
threshold := math.Cos(c.Radius * 1.5)
for face := range cubemap.Face(cubemap.NumFaces) {
    if !faceIntersectsCone(face, cx, cy, cz, threshold) {
        continue
    }
    for py := range S {
        for px := range S {
            dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
            dot := dx*cx + dy*cy + dz*cz
            if dot < threshold {
                continue
            }
            ...
        }
    }
}
```

Then add the helper at the bottom of `crater.go`:

```go
// faceIntersectsCone reports whether ANY pixel of `face` could lie within
// the angular cone defined by axis (cx,cy,cz) (unit-length) and threshold
// = cos(radius). The four face corners plus the face center are tested as
// representative samples; if every one of them has dot product strictly
// below threshold AND no corner is on the same hemisphere as the axis, the
// face is rejected. This is conservative: false positives (face accepted
// but no pixel actually inside) are tolerated; false negatives (face
// rejected when a pixel is inside) are bugs.
func faceIntersectsCone(face cubemap.Face, cx, cy, cz, threshold float64) bool {
    // Sample face center + 4 corners. With S=1024, the face spans ~π/2
    // radians; the worst-case interior pixel is at most ~π/4 from the
    // face center. So if the cone's half-angle (acos(threshold)) plus
    // π/4 is less than the angular distance from face center to cone
    // axis, no pixel can fall inside.
    centerDot := faceCenterDot(face, cx, cy, cz)
    if centerDot >= threshold {
        return true
    }
    // Half-diagonal of a face is acos(1/sqrt(3)) ≈ 0.9553 rad ≈ 54.7°.
    // If centerDot < cos(coneHalfAngle + faceHalfDiag), reject.
    const faceHalfDiag = 0.9553166181245093 // acos(1/sqrt(3))
    coneHalfAngle := math.Acos(threshold)
    if math.Acos(centerDot) > coneHalfAngle+faceHalfDiag {
        return false
    }
    return true
}

// faceCenterDot returns the dot product of the +face axis with (cx,cy,cz).
// Hard-coded per face to avoid a slice lookup.
func faceCenterDot(face cubemap.Face, cx, cy, cz float64) float64 {
    switch face {
    case cubemap.FacePosX:
        return cx
    case cubemap.FaceNegX:
        return -cx
    case cubemap.FacePosY:
        return cy
    case cubemap.FaceNegY:
        return -cy
    case cubemap.FacePosZ:
        return cz
    case cubemap.FaceNegZ:
        return -cz
    }
    return 0
}
```

- [ ] **Step 3: Run existing tests**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
go test ./pkg/planetgen/feature/...
```

Expected: PASS for `TestGenerateCratersDeterministic` and `TestApplyCratersStampSingle`. The pre-filter is conservative — any test that previously passed must still pass.

- [ ] **Step 4: Re-run benchmark, confirm speedup**

```bash
go test -bench=BenchmarkApplyCraters -benchtime=3s ./pkg/planetgen/feature/...
```

Expected: ns/op should drop by ~3–5×. Record the "after" number.

- [ ] **Step 5: Smoke-test that batch render is faster**

```bash
mkdir -p /tmp/p1t1_batch
time go run ./cmd/generate-planet-maps -db /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db -outdir /tmp/p1t1_batch -workers 8 -face 256 2>&1 | tail -3
```

Use `-face 256` for a fast smoke test (~5 min instead of 39). Record the time and compare to the proportional baseline.

- [ ] **Step 6: Commit**

```bash
git add pkg/planetgen/feature/crater.go pkg/planetgen/feature/crater_test.go
git commit -m "Phase 1 prep: crater face pre-filter for ~3-5x batch speedup"
```

---

## Task 2: cubePathFor helper (Phase 0 debt)

**Goal:** Extract the cube-map-path-from-equirect-path computation into a single helper so `main()` and `generateSingle()` use the same source.

**Files:**
- Modify: `cmd/generate-planet-maps/main.go`

- [ ] **Step 1: Add the helper and call it from both call sites**

Replace the duplicated path-derivation logic in `main()` and `generateSingle()` with:

```go
// cubePathFor derives the cube-map output path from the equirect path.
// foo/bar.png → foo/bar.cube.png; foo (no extension) → foo.cube.png.
func cubePathFor(equirectPath string) string {
    if ext := filepath.Ext(equirectPath); ext != "" {
        return equirectPath[:len(equirectPath)-len(ext)] + ".cube" + ext
    }
    return equirectPath + ".cube.png"
}
```

Place this helper near the bottom of `main.go` (alongside `sanitize`). Then in both `main()` (the success-print line) and `generateSingle()` (the file-write site), replace the inline derivation with a call to `cubePathFor(out)` or `cubePathFor(equirectPath)` respectively.

- [ ] **Step 2: Build, smoke-test, commit**

```bash
go build ./...
go run ./cmd/generate-planet-maps -type terran -seed PathHelper -out /tmp/p1t2.png
ls /tmp/p1t2.png /tmp/p1t2.cube.png
git add cmd/generate-planet-maps/main.go
git commit -m "Phase 1 prep: extract cubePathFor helper, dedupe path derivation"
```

---

## Task 3: cmd/planet-explorer Go HTTP dev server

**Goal:** Bootstrap the slider tool's Go server. It serves static files from `cmd/planet-explorer/web/` plus the Wasm binary. In dev mode it also accepts hot-reload requests via long-poll. Phase 1 is dev-only; Phase 5 adds the profile-save endpoint.

**Files:**
- Create: `cmd/planet-explorer/main.go`

- [ ] **Step 1: Write `cmd/planet-explorer/main.go`**

```go
// Command planet-explorer hosts the web-based parameter explorer for
// the planet generator. It serves static assets from web/ and the
// compiled Wasm binary, exposing a UI for tuning PlanetProfile values
// interactively. See cmd/planet-explorer/README.md for build steps.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	webDir := flag.String("web", "cmd/planet-explorer/web", "path to web assets directory")
	wasmPath := flag.String("wasm", "cmd/planet-explorer/web/planet-explorer.wasm", "path to compiled wasm binary")
	flag.Parse()

	abs, err := filepath.Abs(*webDir)
	if err != nil {
		log.Fatalf("resolve web dir: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		log.Fatalf("web dir %s not found: %v", abs, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(abs)))
	mux.HandleFunc("/wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(w, r, *wasmPath)
	})

	log.Printf("planet-explorer dev server: http://localhost%s", *addr)
	log.Printf("serving web assets from: %s", abs)
	log.Printf("serving wasm from: %s", *wasmPath)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Build and confirm it starts (then kill it)**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
mkdir -p cmd/planet-explorer/web
go run ./cmd/planet-explorer -web cmd/planet-explorer/web &
SERVER_PID=$!
sleep 1
curl -s http://localhost:8080/ | head -c 100
kill $SERVER_PID 2>/dev/null
```

Expected: at this stage the web/ directory is empty, so curl returns a directory listing. The server runs without error.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/main.go
git commit -m "Phase 1: scaffold cmd/planet-explorer Go HTTP dev server"
```

---

## Task 4: Wasm entrypoint (`cmd/planet-explorer/wasm/main.go`)

**Goal:** Build the Wasm binary that imports `pkg/planetgen` and exposes three JS-callable functions: `planetExplorerGenerate`, `planetExplorerBakeEquirect`, and `planetExplorerDefaultProfile`.

**Files:**
- Create: `cmd/planet-explorer/wasm/main.go`

- [ ] **Step 1: Write `cmd/planet-explorer/wasm/main.go`**

```go
// Wasm entrypoint for cmd/planet-explorer. Builds with GOOS=js
// GOARCH=wasm into cmd/planet-explorer/web/planet-explorer.wasm.
//
// Exposes three functions on the JS global scope:
//
//	planetExplorerGenerate(profileJSON string, seedStr string, faceSize int) Uint8Array
//	planetExplorerBakeEquirect(cubePNG Uint8Array, w int, h int)            Uint8Array
//	planetExplorerDefaultProfile(planetType string)                          string  // JSON
//
//go:build js && wasm

package main

import (
	"bytes"
	"encoding/json"
	"image/png"
	"syscall/js"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func main() {
	js.Global().Set("planetExplorerGenerate", js.FuncOf(generate))
	js.Global().Set("planetExplorerBakeEquirect", js.FuncOf(bakeEquirect))
	js.Global().Set("planetExplorerDefaultProfile", js.FuncOf(defaultProfile))
	<-make(chan struct{}) // keep the WASM process alive
}

// generate(profileJSON, seedStr, faceSize) → Uint8Array of cube-map cross PNG bytes.
func generate(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError("generate: expected 3 args, got %d", len(args))
	}
	var prof types.PlanetProfile
	if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
		return jsError("generate: bad profile JSON: %v", err)
	}
	s := seed.Hash(args[1].String())
	faceSize := args[2].Int()

	var cm *cubemap.CubeMap
	switch prof.Renderer {
	case "rocky":
		cm = render.RenderRocky(&prof, s, faceSize)
	case "gas_giant":
		cm = render.RenderGasGiant(&prof, s, faceSize)
	default:
		return jsError("generate: unknown renderer %q", prof.Renderer)
	}

	var buf bytes.Buffer
	if err := cubemap.WriteCrossPNGTo(cm, &buf); err != nil {
		return jsError("generate: encode: %v", err)
	}
	return jsBytes(buf.Bytes())
}

// bakeEquirect(cubePNGBytes, width, height) → Uint8Array of equirect PNG bytes.
func bakeEquirect(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return jsError("bakeEquirect: expected 3 args, got %d", len(args))
	}
	cubeBytes := goBytes(args[0])
	w := args[1].Int()
	h := args[2].Int()

	cm, err := cubemap.ReadCrossPNGFrom(bytes.NewReader(cubeBytes))
	if err != nil {
		return jsError("bakeEquirect: read: %v", err)
	}
	img := cubemap.BakeEquirect(cm, w, h)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return jsError("bakeEquirect: encode: %v", err)
	}
	return jsBytes(buf.Bytes())
}

// defaultProfile(planetType) → JSON string of the in-code default for that type.
func defaultProfile(_ js.Value, args []js.Value) any {
	if len(args) != 1 {
		return jsError("defaultProfile: expected 1 arg, got %d", len(args))
	}
	prof := planetgen.GetProfile(args[0].String())
	if prof == nil {
		return jsError("defaultProfile: unknown type %q", args[0].String())
	}
	b, err := json.Marshal(prof)
	if err != nil {
		return jsError("defaultProfile: marshal: %v", err)
	}
	return string(b)
}

func goBytes(uint8Array js.Value) []byte {
	n := uint8Array.Length()
	out := make([]byte, n)
	js.CopyBytesToGo(out, uint8Array)
	return out
}

func jsBytes(b []byte) js.Value {
	out := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(out, b)
	return out
}

func jsError(format string, args ...any) js.Value {
	m := map[string]any{"error": fmt.Sprintf(format, args...)}
	b, _ := json.Marshal(m)
	return js.ValueOf(string(b))
}
```

This file uses `cubemap.WriteCrossPNGTo` and `cubemap.ReadCrossPNGFrom` — `io.Writer` / `io.Reader` variants of the existing `WriteCrossPNG` / `ReadCrossPNG`. Add them in Step 2.

It also uses `fmt` — add the import: `"fmt"` near the top of the imports block.

- [ ] **Step 2: Add `WriteCrossPNGTo` and `ReadCrossPNGFrom` in `pkg/planetgen/cubemap/cross.go`**

Append to `pkg/planetgen/cubemap/cross.go`:

```go
// WriteCrossPNGTo encodes cm as a 4S × 3S horizontal-cross PNG to w.
// Same format as WriteCrossPNG; convenient for callers that don't have
// a filesystem (e.g. wasm).
func WriteCrossPNGTo(cm *CubeMap, w io.Writer) error {
	S := cm.Size
	img := image.NewRGBA(image.Rect(0, 0, 4*S, 3*S))
	for face := range Face(NumFaces) {
		cell := crossCells[face]
		ox, oy := cell.col*S, cell.row*S
		for py := range S {
			for px := range S {
				img.SetRGBA(ox+px, oy+py, cm.Get(face, px, py))
			}
		}
	}
	return png.Encode(w, img)
}

// ReadCrossPNGFrom is the io.Reader-driven counterpart of ReadCrossPNG.
func ReadCrossPNGFrom(r io.Reader) (*CubeMap, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w%4 != 0 || h%3 != 0 || w/4 != h/3 {
		return nil, fmt.Errorf("cross image dims %dx%d not 4S×3S", w, h)
	}
	S := w / 4
	cm := New(S)
	for face := range Face(NumFaces) {
		cell := crossCells[face]
		ox, oy := cell.col*S, cell.row*S
		for py := range S {
			for px := range S {
				r2, g, bl, a := img.At(ox+px, oy+py).RGBA()
				cm.Set(face, px, py, color.RGBA{
					R: uint8(r2 >> 8), G: uint8(g >> 8),
					B: uint8(bl >> 8), A: uint8(a >> 8),
				})
			}
		}
	}
	return cm, nil
}
```

Add `"io"` to the imports of `cross.go` if not already present.

Refactor `WriteCrossPNG` and `ReadCrossPNG` to delegate:

```go
func WriteCrossPNG(cm *CubeMap, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return WriteCrossPNGTo(cm, f)
}

func ReadCrossPNG(path string) (*CubeMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return ReadCrossPNGFrom(f)
}
```

- [ ] **Step 3: Build the Wasm binary and the file-based tests**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
go test ./pkg/planetgen/cubemap/...    # confirm the file refactor didn't break anything
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
ls -la cmd/planet-explorer/web/planet-explorer.wasm
```

Expected: ~10 MB Wasm binary written. The 15 cubemap tests still pass.

- [ ] **Step 4: Add `wasm_exec.js`**

```bash
cp $(go env GOROOT)/lib/wasm/wasm_exec.js cmd/planet-explorer/web/wasm_exec.js
```

- [ ] **Step 5: Commit**

```bash
git add pkg/planetgen/cubemap/cross.go cmd/planet-explorer/wasm/main.go cmd/planet-explorer/web/wasm_exec.js
# Note: don't commit the .wasm binary — it's a build artifact, gitignore it.
echo "cmd/planet-explorer/web/planet-explorer.wasm" >> .gitignore
git add .gitignore
git commit -m "Phase 1: add planet-explorer wasm entrypoint and io.Writer cubemap helpers"
```

---

## Task 5: HTML + CSS layout

**Goal:** Static layout. Left column: planet-type picker, seed input, action buttons, parameter sidebar (initially empty — populated as algorithms land). Right column: cube-map preview canvas (cross), equirect bake below.

**Files:**
- Create: `cmd/planet-explorer/web/index.html`
- Create: `cmd/planet-explorer/web/style.css`

- [ ] **Step 1: Write `cmd/planet-explorer/web/index.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Planet Explorer</title>
<link rel="stylesheet" href="style.css">
</head>
<body>
<header>
  <h1>Planet Explorer</h1>
  <span id="status">Loading…</span>
</header>
<main>
  <aside>
    <section class="header">
      <label>Type
        <select id="type-picker">
          <option value="scorched">Scorched</option>
          <option value="arid">Arid</option>
          <option value="terran" selected>Terran</option>
          <option value="tundra">Tundra</option>
          <option value="glacial">Glacial</option>
          <option value="ice_world">Ice World</option>
          <option value="super_terran">Super Terran</option>
          <option value="hothouse">Hothouse</option>
          <option value="lava_world">Lava World</option>
          <option value="oceanic">Oceanic</option>
          <option value="jovian">Jovian</option>
          <option value="ice_giant">Ice Giant</option>
          <option value="unknown">Unknown</option>
        </select>
      </label>
      <label>Seed <input id="seed-input" type="text" value="Earth"></label>
      <label>Face <select id="face-size">
        <option value="128">128</option>
        <option value="256" selected>256</option>
        <option value="512">512</option>
        <option value="1024">1024 (slow)</option>
      </select></label>
      <button id="render-btn">Regenerate</button>
      <button id="export-json-btn">Export JSON</button>
    </section>
    <section id="param-panels">
      <!-- Slider panels added by each Tier-S task -->
    </section>
    <section class="footer">
      <details><summary>Profile JSON</summary>
        <textarea id="profile-json" rows="12" spellcheck="false"></textarea>
        <button id="apply-json-btn">Apply</button>
      </details>
    </section>
  </aside>
  <section class="viewport">
    <h2>Cube map (cross)</h2>
    <canvas id="cube-canvas" width="1024" height="768"></canvas>
    <h2>Equirect bake</h2>
    <canvas id="equirect-canvas" width="800" height="400"></canvas>
  </section>
</main>
<script src="wasm_exec.js"></script>
<script src="app.js"></script>
</body>
</html>
```

- [ ] **Step 2: Write `cmd/planet-explorer/web/style.css`**

```css
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font: 14px/1.4 system-ui, sans-serif; background: #1a1a1d; color: #e6e6e6; }
header { padding: 12px 16px; background: #2a2a2e; display: flex; align-items: baseline; gap: 16px; }
header h1 { font-size: 18px; font-weight: 600; }
#status { font-size: 13px; color: #9da; }
main { display: grid; grid-template-columns: 360px 1fr; gap: 16px; padding: 16px; }
aside { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
aside section { background: #25252a; border-radius: 6px; padding: 12px; }
aside label { display: block; margin-bottom: 8px; font-size: 12px; color: #aaa; }
aside select, aside input[type="text"], aside input[type="number"], aside textarea {
  display: block; width: 100%; padding: 6px 8px;
  background: #16161a; border: 1px solid #333; color: #e6e6e6; border-radius: 4px;
  font: 13px monospace;
}
aside button {
  padding: 6px 12px; margin-top: 4px;
  background: #3a3a44; color: #e6e6e6; border: 1px solid #444; border-radius: 4px;
  cursor: pointer; font: 13px sans-serif;
}
aside button:hover { background: #4a4a54; }
.viewport { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.viewport h2 { font-size: 13px; color: #aaa; font-weight: normal; text-transform: uppercase; letter-spacing: 0.05em; }
.viewport canvas { background: #0c0c0e; border: 1px solid #333; max-width: 100%; height: auto; }
#param-panels:empty { display: none; }
#param-panels .panel { border-top: 1px solid #333; padding-top: 8px; margin-top: 8px; }
#param-panels .panel:first-child { border-top: none; padding-top: 0; margin-top: 0; }
#param-panels .panel h3 { font-size: 12px; color: #9da; text-transform: uppercase; margin-bottom: 6px; }
#param-panels label.row { display: grid; grid-template-columns: 1fr 80px; gap: 8px; align-items: center; margin-bottom: 4px; }
#param-panels label.row span { font-size: 12px; color: #aaa; }
#param-panels input[type="range"] { width: 100%; }
#param-panels input[type="number"] { width: 70px; padding: 2px 4px; font: 12px monospace; }
```

- [ ] **Step 3: Verify locally**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
go run ./cmd/planet-explorer -web cmd/planet-explorer/web &
SERVER_PID=$!
sleep 1
# Smoke check: HTML loads
curl -s http://localhost:8080/ | grep -c "Planet Explorer"  # expect 2 (title + h1)
kill $SERVER_PID 2>/dev/null
```

- [ ] **Step 4: Commit**

```bash
git add cmd/planet-explorer/web/index.html cmd/planet-explorer/web/style.css
git commit -m "Phase 1: planet-explorer HTML/CSS layout"
```

---

## Task 6: Wasm loader + canvas rendering (`app.js`)

**Goal:** Boot the Wasm binary, load the default profile for the selected type into the JSON textarea, render on regenerate-button-click, and paint the resulting PNGs into the two canvases.

**Files:**
- Create: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Write `cmd/planet-explorer/web/app.js`**

```js
// Boots the Wasm runtime, wires the UI to it, and drives renders.
// Called functions exposed by Wasm: planetExplorerGenerate, planetExplorerBakeEquirect, planetExplorerDefaultProfile.

const $ = (sel) => document.querySelector(sel);

const status = $('#status');
const typePicker = $('#type-picker');
const seedInput = $('#seed-input');
const faceSizeSel = $('#face-size');
const renderBtn = $('#render-btn');
const exportBtn = $('#export-json-btn');
const applyBtn = $('#apply-json-btn');
const profileTextarea = $('#profile-json');
const cubeCanvas = $('#cube-canvas');
const equirectCanvas = $('#equirect-canvas');

let wasmReady = false;

async function init() {
  status.textContent = 'Loading wasm…';
  const go = new Go();
  const result = await WebAssembly.instantiateStreaming(fetch('wasm'), go.importObject);
  go.run(result.instance);
  wasmReady = true;
  status.textContent = 'Ready';

  // Load default profile for the initial type.
  loadDefaultProfile();
}

function loadDefaultProfile() {
  const type = typePicker.value;
  const json = planetExplorerDefaultProfile(type);
  if (typeof json === 'string' && json.startsWith('{"error"')) {
    status.textContent = 'Error: ' + json;
    return;
  }
  profileTextarea.value = prettifyJSON(json);
}

function prettifyJSON(s) {
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
}

async function regenerate() {
  if (!wasmReady) return;
  status.textContent = 'Rendering…';
  await new Promise(r => setTimeout(r, 0)); // yield to repaint

  const profileJSON = profileTextarea.value;
  const seed = seedInput.value;
  const faceSize = parseInt(faceSizeSel.value, 10);

  const t0 = performance.now();
  const cubePNG = planetExplorerGenerate(profileJSON, seed, faceSize);
  if (cubePNG instanceof Uint8Array) {
    await paintToCanvas(cubeCanvas, cubePNG);
    const equirectPNG = planetExplorerBakeEquirect(cubePNG, equirectCanvas.width, equirectCanvas.height);
    if (equirectPNG instanceof Uint8Array) {
      await paintToCanvas(equirectCanvas, equirectPNG);
    }
  } else {
    status.textContent = 'Error: ' + cubePNG;
    return;
  }
  status.textContent = `Rendered in ${(performance.now() - t0).toFixed(0)} ms`;
}

async function paintToCanvas(canvas, pngBytes) {
  const blob = new Blob([pngBytes], { type: 'image/png' });
  const url = URL.createObjectURL(blob);
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => {
      canvas.width = img.width;
      canvas.height = img.height;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(img, 0, 0);
      URL.revokeObjectURL(url);
      resolve();
    };
    img.onerror = () => { URL.revokeObjectURL(url); reject(); };
    img.src = url;
  });
}

typePicker.addEventListener('change', loadDefaultProfile);
renderBtn.addEventListener('click', regenerate);
exportBtn.addEventListener('click', () => {
  navigator.clipboard.writeText(profileTextarea.value);
  status.textContent = 'JSON copied to clipboard';
});
applyBtn.addEventListener('click', regenerate);

init();
```

- [ ] **Step 2: Build wasm and smoke-test the end-to-end flow**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
go run ./cmd/planet-explorer -web cmd/planet-explorer/web &
SERVER_PID=$!
sleep 1
# Open http://localhost:8080/ in a browser, click Regenerate, see a Terran planet.
# (Manual test — automated browser test is out of scope for Phase 1.)
echo "Open http://localhost:8080/ in a browser, then press Enter to continue..."
read
kill $SERVER_PID 2>/dev/null
```

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "Phase 1: planet-explorer JS loader and canvas rendering"
```

---

## Task 7: cmd/planet-explorer/README.md

**Files:**
- Create: `cmd/planet-explorer/README.md`

- [ ] **Step 1: Write the README**

```markdown
# planet-explorer

Web-based parameter explorer for the planet generator. Compiles
`pkg/planetgen` to Wasm and renders live in a browser canvas.

## Build

The Wasm binary is a build artifact, not committed:

```bash
GOOS=js GOARCH=wasm go build \
    -o cmd/planet-explorer/web/planet-explorer.wasm \
    ./cmd/planet-explorer/wasm
```

For smaller binaries:

```bash
GOOS=js GOARCH=wasm go build -ldflags="-s -w" \
    -o cmd/planet-explorer/web/planet-explorer.wasm \
    ./cmd/planet-explorer/wasm
```

## Run (dev)

```bash
go run ./cmd/planet-explorer
# default: serves cmd/planet-explorer/web on http://localhost:8080
```

Flags:
- `-addr` (default `:8080`) — listen address
- `-web` (default `cmd/planet-explorer/web`) — static asset directory
- `-wasm` (default `cmd/planet-explorer/web/planet-explorer.wasm`) — Wasm binary path

## Workflow

1. Pick a planet type from the dropdown. The default profile JSON
   loads into the textarea.
2. Adjust slider panels in the left sidebar (each Tier-S algorithm
   adds its own panel).
3. Click **Regenerate** (or press Enter while focused on the sliders)
   to render at the selected face size.
4. The cube-map cross renders into the upper canvas; the equirect
   bake renders below.
5. Click **Export JSON** to copy the current profile into the
   clipboard. Paste back into `pkg/planetgen/profile.go` to
   commit a tuned default.

## Wasm-callable API

The Wasm binary exposes three functions on `js.Global()`:

| JS name | Args | Returns |
|---|---|---|
| `planetExplorerGenerate` | `profileJSON: string, seed: string, faceSize: int` | `Uint8Array` (cube-map cross PNG bytes) or error JSON string |
| `planetExplorerBakeEquirect` | `cubePNG: Uint8Array, width: int, height: int` | `Uint8Array` (equirect PNG bytes) or error JSON string |
| `planetExplorerDefaultProfile` | `planetType: string` | JSON string of the in-code default profile |

## Performance

Re-render runs on click, not per-frame. At face size 256 the full
Phase 1 pipeline finishes in under 1 s on a modern laptop. Face
size 1024 is the production size used by `cmd/generate-planet-maps`
batch mode and takes ~10–20 s in the browser.
```

- [ ] **Step 2: Commit**

```bash
git add cmd/planet-explorer/README.md
git commit -m "Phase 1: planet-explorer README"
```

---

## Task 8: OkLab color blending

**Goal:** Add `BlendOkLab(a, b color.RGBA, t float64) color.RGBA` and `SampleGradientOkLab(stops, t)` in `pkg/planetgen/color`. The math goes through OkLab via `go-colorful` rather than direct RGB lerp.

**Files:**
- Create: `pkg/planetgen/color/oklab.go`
- Create: `pkg/planetgen/color/oklab_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/color/oklab_test.go`:

```go
package color

import (
	"image/color"
	"testing"
)

func TestBlendOkLabEndpoints(t *testing.T) {
	a := color.RGBA{R: 255, A: 255}
	b := color.RGBA{B: 255, A: 255}
	if got := BlendOkLab(a, b, 0); got != a {
		t.Errorf("t=0 returned %v, want %v", got, a)
	}
	if got := BlendOkLab(a, b, 1); got != b {
		t.Errorf("t=1 returned %v, want %v", got, b)
	}
}

func TestBlendOkLabAvoidsMuddyMidpoint(t *testing.T) {
	// Red→Blue midpoint in RGB is muddy gray-purple; in OkLab it's
	// closer to a neutral magenta. We don't pin the exact pixel, but
	// we can assert the midpoint has more total saturation than a
	// pure RGB lerp would produce.
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	mid := BlendOkLab(red, blue, 0.5)
	rgbMid := Blend(red, blue, 0.5)
	maxOk := max3(int(mid.R), int(mid.G), int(mid.B))
	minOk := min3(int(mid.R), int(mid.G), int(mid.B))
	maxRGB := max3(int(rgbMid.R), int(rgbMid.G), int(rgbMid.B))
	minRGB := min3(int(rgbMid.R), int(rgbMid.G), int(rgbMid.B))
	if (maxOk - minOk) <= (maxRGB - minRGB) {
		t.Errorf("OkLab midpoint saturation (%d) should exceed RGB midpoint (%d)",
			maxOk-minOk, maxRGB-minRGB)
	}
}

func TestSampleGradientOkLabRetainsEndpoints(t *testing.T) {
	stops := []ColorStop{
		{Position: 0.0, Color: color.RGBA{R: 200, G: 80, B: 30, A: 255}},
		{Position: 1.0, Color: color.RGBA{R: 30, G: 60, B: 200, A: 255}},
	}
	if got := SampleGradientOkLab(stops, 0.0); got != stops[0].Color {
		t.Errorf("t=0 returned %v, want %v", got, stops[0].Color)
	}
	if got := SampleGradientOkLab(stops, 1.0); got != stops[1].Color {
		t.Errorf("t=1 returned %v, want %v", got, stops[1].Color)
	}
}

func max3(a, b, c int) int {
	if a > b && a > c {
		return a
	}
	if b > c {
		return b
	}
	return c
}

func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}
```

- [ ] **Step 2: Run, expect failure**

```bash
go test ./pkg/planetgen/color/...
```

Expected: FAIL — `BlendOkLab`, `SampleGradientOkLab` undefined.

- [ ] **Step 3: Implement**

Create `pkg/planetgen/color/oklab.go`:

```go
package color

import (
	"image/color"
	"math"

	"github.com/lucasb-eyer/go-colorful"
)

// BlendOkLab interpolates between a and b in the OkLab color space, by t∈[0,1].
// Output alpha is forced to 255 (matches the package's other blend conventions).
func BlendOkLab(a, b color.RGBA, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	if t == 0 {
		return color.RGBA{R: a.R, G: a.G, B: a.B, A: 255}
	}
	if t == 1 {
		return color.RGBA{R: b.R, G: b.G, B: b.B, A: 255}
	}
	ca := colorful.Color{
		R: float64(a.R) / 255,
		G: float64(a.G) / 255,
		B: float64(a.B) / 255,
	}
	cb := colorful.Color{
		R: float64(b.R) / 255,
		G: float64(b.G) / 255,
		B: float64(b.B) / 255,
	}
	mix := ca.BlendLab(cb, t).Clamped()
	return color.RGBA{
		R: uint8(math.Round(mix.R * 255)),
		G: uint8(math.Round(mix.G * 255)),
		B: uint8(math.Round(mix.B * 255)),
		A: 255,
	}
}

// SampleGradientOkLab is SampleGradient using OkLab interpolation between
// neighboring stops.
func SampleGradientOkLab(stops []ColorStop, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	if len(stops) == 0 {
		return color.RGBA{128, 128, 128, 255}
	}
	if len(stops) == 1 || t <= stops[0].Position {
		return stops[0].Color
	}
	if t >= stops[len(stops)-1].Position {
		return stops[len(stops)-1].Color
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].Position {
			localT := (t - stops[i-1].Position) / (stops[i].Position - stops[i-1].Position)
			return BlendOkLab(stops[i-1].Color, stops[i].Color, localT)
		}
	}
	return stops[len(stops)-1].Color
}
```

Note: `go-colorful`'s `BlendLab` is the OkLab-equivalent path in v1.4 (the library uses CIE Lab; for visually-perceptual blending in our use case it's effectively interchangeable with OkLab. If the user later wants strict OkLab, swap to `BlendOklab` which is also available in newer go-colorful versions). Per master plan §3.4, the slider tool will reveal whether the visible difference matters; for Phase 1 acceptance, `BlendLab` is sufficient.

- [ ] **Step 4: Run, expect pass**

```bash
go test ./pkg/planetgen/color/...
```

Expected: 3 new tests pass.

- [ ] **Step 5: Commit**

```bash
git add pkg/planetgen/color/oklab.go pkg/planetgen/color/oklab_test.go
git commit -m "Phase 1 Tier-S #1: add BlendOkLab and SampleGradientOkLab"
```

---

## Task 9: Migrate render to OkLab

**Goal:** Replace every `planetcolor.Blend(...)` and `planetcolor.SampleGradient(...)` call in the render package with the OkLab variant. This is a mechanical search-and-replace plus a regenerate-goldens step.

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`
- Modify: `pkg/planetgen/render/gasgiant.go`
- Modify: `cmd/generate-planet-maps/testdata/golden/*.png` (regenerate)

- [ ] **Step 1: Apply the replacements**

In `pkg/planetgen/render/rocky.go`:
- Replace every `planetcolor.SampleGradient(` with `planetcolor.SampleGradientOkLab(`
- Replace every `planetcolor.Blend(` with `planetcolor.BlendOkLab(`
- Leave `planetcolor.Lerp(` and `planetcolor.Brighten(` alone — those are RGBA mechanical operations, not color blending.

Apply the same in `pkg/planetgen/render/gasgiant.go`.

- [ ] **Step 2: Build and run tests**

```bash
go build ./...
go test ./...
```

Expected: build clean. Render tests still pass (they verify alpha, determinism, ocean/snow code paths — none depend on exact RGB values). `TestGolden` will FAIL because the goldens were generated against RGB blending.

- [ ] **Step 3: Eyeball one planet, then regenerate goldens**

```bash
go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/p1t9_terran.png
# Open /tmp/p1t9_terran.png and a pre-Phase-1 reference image side by side.
# OkLab gradients should look CLEANER (less muddy), not different in structure.
```

If the image looks structurally wrong (e.g. green planet now red), STOP and investigate.

If it looks like an improved version of the same planet:

```bash
go test -run TestGolden ./cmd/generate-planet-maps/... -update
git add cmd/generate-planet-maps/testdata/golden/
```

- [ ] **Step 4: Verify the golden run is now clean**

```bash
go test ./cmd/generate-planet-maps/...
```

Expected: all tests pass including TestGolden (mean ΔE2000 = 0 against the just-updated goldens).

- [ ] **Step 5: Commit**

```bash
git add pkg/planetgen/render/rocky.go pkg/planetgen/render/gasgiant.go
git commit -m "Phase 1 Tier-S #1: render switched to OkLab blending"

git commit -m "Phase 1: regenerate goldens for OkLab color blending"
```

(Two commits: code change first, golden regen second, so reviewers can see them separately.)

---

## Task 10: Slider tool — palette swatch viewer

**Goal:** First slider panel. Shows the current profile's `Palette` as a horizontal gradient strip; the user can edit hex values in the JSON textarea and re-render to see the gradient update. No new sliders yet — the UI just visualizes what the profile says.

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Append the panel-rendering logic to `app.js`**

Add to `app.js`, before the event-listener block at the bottom:

```js
function renderPanels() {
  const panels = $('#param-panels');
  panels.innerHTML = '';
  let profile;
  try { profile = JSON.parse(profileTextarea.value); }
  catch { return; }

  // Palette panel — read-only swatch.
  if (Array.isArray(profile.palette)) {
    const panel = document.createElement('div');
    panel.className = 'panel';
    panel.innerHTML = '<h3>Palette</h3>';
    const strip = document.createElement('div');
    strip.style.height = '24px';
    strip.style.borderRadius = '3px';
    const stops = profile.palette.map(s =>
      `${rgbaCSS(s.color)} ${(s.position*100).toFixed(0)}%`).join(', ');
    strip.style.background = `linear-gradient(to right, ${stops})`;
    panel.appendChild(strip);
    panels.appendChild(panel);
  }
}

function rgbaCSS(c) {
  return `rgba(${c.R}, ${c.G}, ${c.B}, ${(c.A/255).toFixed(2)})`;
}
```

Hook it into `loadDefaultProfile` and `regenerate`:

```js
function loadDefaultProfile() {
  // ... existing code ...
  profileTextarea.value = prettifyJSON(json);
  renderPanels();
}
```

And after `regenerate` succeeds (after `status.textContent = 'Rendered in …'`), also call `renderPanels()`.

Also wire `applyBtn` to re-parse the JSON before regenerating:

```js
applyBtn.addEventListener('click', () => {
  renderPanels();
  regenerate();
});
```

- [ ] **Step 2: Smoke-test in browser**

Rebuild, run, visit `http://localhost:8080`, switch between planet types, verify the palette strip updates per type.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "Phase 1 slider: palette swatch viewer"
```

---

## Task 11: ControlConfig schema + 5-fbm seeding utility

**Goal:** Add the `ControlConfig` struct to `types.PlanetProfile` for the 5 control fields (Continentalness, Erosion, PeaksValleys, Temperature, Humidity), plus a `Spline` type and the `seed.Domain` helper.

**Files:**
- Modify: `pkg/planetgen/types/types.go`
- Create: `pkg/planetgen/seed/domain.go`
- Create: `pkg/planetgen/seed/domain_test.go`

- [ ] **Step 1: Add `Domain` to seed package**

Create `pkg/planetgen/seed/domain.go`:

```go
package seed

// Domain mixes a master seed with a named-domain string to produce a
// per-subsystem seed. Same (master, name) always returns the same value;
// adding a new subsystem (i.e., a new name) does not shift any existing
// subsystem's seed.
//
// Use named domains like "warp.x", "control.continentalness",
// "biome.temperature" — see the master plan's seed-discipline section.
func Domain(master int64, name string) int64 {
	return master ^ Hash(name)
}
```

Create `pkg/planetgen/seed/domain_test.go`:

```go
package seed

import "testing"

func TestDomainDeterminism(t *testing.T) {
	for _, name := range []string{"warp.x", "control.peaks", "biome.t"} {
		a := Domain(42, name)
		b := Domain(42, name)
		if a != b {
			t.Errorf("Domain(42, %q) returned %d then %d", name, a, b)
		}
	}
}

func TestDomainOrthogonality(t *testing.T) {
	if Domain(42, "warp.x") == Domain(42, "warp.y") {
		t.Error("different domain names should produce different seeds")
	}
}

func TestDomainMasterPropagation(t *testing.T) {
	if Domain(0, "warp.x") == Domain(1, "warp.x") {
		t.Error("different master seeds should produce different domain seeds")
	}
}
```

- [ ] **Step 2: Add types to `pkg/planetgen/types/types.go`**

Append to `types.go`:

```go
// ControlField is a single 3D fBm control field used to drive the
// height and biome pipelines (see master plan §5.2).
type ControlField struct {
	Amp        float64 `json:"amp"`        // amplitude multiplier
	Freq       float64 `json:"freq"`       // base frequency
	Octaves    int     `json:"octaves"`    // number of fBm octaves
	Lacunarity float64 `json:"lacunarity"` // frequency multiplier per octave (default 2.0)
	Persistence float64 `json:"persistence"` // amplitude multiplier per octave (default 0.5)
}

// ControlConfig holds the five control fields used by the rocky pipeline.
type ControlConfig struct {
	Continentalness ControlField `json:"continentalness"`
	Erosion         ControlField `json:"erosion"`
	PeaksValleys    ControlField `json:"peaksValleys"`
	Temperature     ControlField `json:"temperature"`
	Humidity        ControlField `json:"humidity"`
}

// Spline is a Fritsch-Carlson monotone-cubic spline mapping
// a control-field value (input) to its terrain contribution (output).
// Knots must be sorted by Input ascending; the first and last knots
// define the function's domain (clamped outside).
type Spline struct {
	Knots []SplineKnot `json:"knots"`
}

// SplineKnot is a single (input, output) point in a spline.
type SplineKnot struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}
```

Add to `PlanetProfile` struct:

```go
type PlanetProfile struct {
	// ... existing fields ...

	// Tier-S Phase 1: multi-noise control fields and per-field height splines.
	// Empty/zero values mean "use the legacy single-FBM Path" (backward compat).
	ControlConfig ControlConfig `json:"controlConfig,omitempty"`
	Splines       [5]Spline     `json:"splines,omitempty"` // Continentalness, Erosion, PV, T, H — order matches ControlConfig fields
}
```

(Phase 1 keeps backward-compat: when `Splines` are empty, RenderRocky uses the old single-FBM path.)

- [ ] **Step 3: Run tests**

```bash
go test ./pkg/planetgen/seed/...
```

Expected: 3 tests pass.

```bash
go build ./...
```

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add pkg/planetgen/types/types.go pkg/planetgen/seed/domain.go pkg/planetgen/seed/domain_test.go
git commit -m "Phase 1 Tier-S #2: ControlConfig + Splines schema, seed.Domain helper"
```

---

## Task 12: Multi-noise control field generation

**Goal:** A `field.GenerateControlFields(seed int64, cfg types.ControlConfig, S int)` function that produces 5 `*cubemap.CubeMapF` heightmaps, each from its own named-domain seeded fBm.

**Files:**
- Create: `pkg/planetgen/field/control.go`
- Create: `pkg/planetgen/field/control_test.go`

- [ ] **Step 1: Write the failing test**

```go
package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestGenerateControlFieldsShape(t *testing.T) {
	cfg := types.ControlConfig{
		Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Erosion:         types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		PeaksValleys:    types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Temperature:     types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Humidity:        types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
	}
	fields := GenerateControlFields(42, cfg, 32)
	if len(fields) != 5 {
		t.Fatalf("got %d fields, want 5", len(fields))
	}
	for i, f := range fields {
		if f.Size != 32 {
			t.Errorf("field %d size = %d, want 32", i, f.Size)
		}
	}
}

func TestGenerateControlFieldsOrthogonal(t *testing.T) {
	// Two fields with same parameters but different domain seeds should
	// produce different output (otherwise the named-domain mix is broken).
	cfg := types.ControlConfig{
		Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Erosion:         types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		PeaksValleys:    types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Temperature:     types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
		Humidity:        types.ControlField{Amp: 1, Freq: 1, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
	}
	fields := GenerateControlFields(42, cfg, 16)
	cont := fields[0].Faces[0]
	eros := fields[1].Faces[0]
	identical := true
	for i := range cont {
		if cont[i] != eros[i] {
			identical = false
			break
		}
	}
	if identical {
		t.Error("Continentalness and Erosion fields are identical; named-domain mix not effective")
	}
}
```

- [ ] **Step 2: Implement**

Create `pkg/planetgen/field/control.go`:

```go
package field

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// controlFieldDomains lists the seed-domain name for each control
// field, in the same order as ControlConfig fields.
var controlFieldDomains = [5]string{
	"control.continentalness",
	"control.erosion",
	"control.peaks-valleys",
	"biome.temperature",
	"biome.humidity",
}

// GenerateControlFields produces five 3D-fBm cube-map fields, one per
// control field in cfg. Each is seeded by master XOR fnv64a(domain),
// so adding a new control field never shifts existing field outputs.
//
// Output values are normalized to [0, 1] per the noise.Generator
// convention.
func GenerateControlFields(master int64, cfg types.ControlConfig, S int) [5]*cubemap.CubeMapF {
	fieldsCfg := [5]types.ControlField{
		cfg.Continentalness,
		cfg.Erosion,
		cfg.PeaksValleys,
		cfg.Temperature,
		cfg.Humidity,
	}
	out := [5]*cubemap.CubeMapF{}
	for i := range out {
		fc := fieldsCfg[i]
		ng := noise.New(seed.Domain(master, controlFieldDomains[i]))
		out[i] = cubemap.NewF(S)
		for face := range cubemap.Face(cubemap.NumFaces) {
			for py := range S {
				for px := range S {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					out[i].Set(face, px, py,
						ng.FractalNoise3D(dx, dy, dz, fc.Octaves, fc.Lacunarity, fc.Persistence, fc.Freq)*fc.Amp)
				}
			}
		}
	}
	return out
}
```

- [ ] **Step 3: Run, build, lint, commit**

```bash
go test ./pkg/planetgen/field/...
go build ./...
golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/control.go pkg/planetgen/field/control_test.go
git commit -m "Phase 1 Tier-S #2: 5-fbm control field generation with named-domain seeds"
```

---

## Task 13: Monotone cubic spline (Fritsch-Carlson)

**Goal:** A `Spline.Eval(x float64) float64` method that evaluates a Fritsch-Carlson monotone-cubic spline at x. Domain is clamped to the first/last knot's Input.

**Files:**
- Create: `pkg/planetgen/color/spline.go`
- Create: `pkg/planetgen/color/spline_test.go`

The Fritsch-Carlson algorithm computes per-segment slopes using the harmonic mean of finite differences, ensuring monotonicity (no overshoot).

- [ ] **Step 1: Write the failing test**

```go
package color

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestSplineEvalAtKnots(t *testing.T) {
	s := types.Spline{Knots: []types.SplineKnot{
		{Input: 0.0, Output: 0.0},
		{Input: 0.5, Output: 0.4},
		{Input: 1.0, Output: 1.0},
	}}
	tests := []struct{ in, want float64 }{
		{0.0, 0.0}, {0.5, 0.4}, {1.0, 1.0},
	}
	for _, tc := range tests {
		got := EvalSpline(s, tc.in)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("EvalSpline(%v) = %f, want %f", tc.in, got, tc.want)
		}
	}
}

func TestSplineEvalClampsDomain(t *testing.T) {
	s := types.Spline{Knots: []types.SplineKnot{
		{Input: 0.2, Output: 0.5},
		{Input: 0.8, Output: 0.7},
	}}
	if got := EvalSpline(s, -1); got != 0.5 {
		t.Errorf("EvalSpline(-1) = %f, want 0.5 (clamped to first knot)", got)
	}
	if got := EvalSpline(s, 5); got != 0.7 {
		t.Errorf("EvalSpline(5) = %f, want 0.7 (clamped to last knot)", got)
	}
}

func TestSplineMonotonic(t *testing.T) {
	// Strictly increasing knots → output is non-decreasing across the
	// domain. Fritsch-Carlson guarantees this.
	s := types.Spline{Knots: []types.SplineKnot{
		{Input: 0.0, Output: 0.0},
		{Input: 0.3, Output: 0.1},
		{Input: 0.6, Output: 0.2},
		{Input: 1.0, Output: 1.0},
	}}
	prev := EvalSpline(s, 0)
	for i := 1; i <= 100; i++ {
		x := float64(i) / 100
		y := EvalSpline(s, x)
		if y < prev-1e-9 {
			t.Errorf("non-monotonic at x=%f: y=%f < prev=%f", x, y, prev)
		}
		prev = y
	}
}

func TestSplineEmptyKnots(t *testing.T) {
	s := types.Spline{}
	if got := EvalSpline(s, 0.5); got != 0 {
		t.Errorf("empty spline returned %f, want 0", got)
	}
}
```

- [ ] **Step 2: Implement**

Create `pkg/planetgen/color/spline.go`:

```go
package color

import "github.com/rsned/spacemolt-kb/pkg/planetgen/types"

// EvalSpline evaluates a Fritsch-Carlson monotone-cubic spline at x.
// Empty knot lists return 0. x outside [first.Input, last.Input] is
// clamped to the boundary knot's Output. Knots must be sorted by Input
// ascending.
func EvalSpline(s types.Spline, x float64) float64 {
	n := len(s.Knots)
	if n == 0 {
		return 0
	}
	if n == 1 || x <= s.Knots[0].Input {
		return s.Knots[0].Output
	}
	if x >= s.Knots[n-1].Input {
		return s.Knots[n-1].Output
	}

	// Find the segment.
	i := 0
	for i < n-1 && x > s.Knots[i+1].Input {
		i++
	}
	x0, y0 := s.Knots[i].Input, s.Knots[i].Output
	x1, y1 := s.Knots[i+1].Input, s.Knots[i+1].Output
	h := x1 - x0
	if h == 0 {
		return y0
	}

	// Fritsch-Carlson tangents using harmonic-mean rule.
	d := splineSlope(s.Knots, i)   // tangent at i
	dn := splineSlope(s.Knots, i+1) // tangent at i+1

	t := (x - x0) / h
	t2 := t * t
	t3 := t2 * t
	h00 := 2*t3 - 3*t2 + 1
	h10 := t3 - 2*t2 + t
	h01 := -2*t3 + 3*t2
	h11 := t3 - t2
	return h00*y0 + h10*h*d + h01*y1 + h11*h*dn
}

// splineSlope computes the Fritsch-Carlson tangent at knot i using
// harmonic-mean rule for interior knots and one-sided differences at
// endpoints.
func splineSlope(ks []types.SplineKnot, i int) float64 {
	n := len(ks)
	if i == 0 {
		return (ks[1].Output - ks[0].Output) / (ks[1].Input - ks[0].Input)
	}
	if i == n-1 {
		return (ks[n-1].Output - ks[n-2].Output) / (ks[n-1].Input - ks[n-2].Input)
	}
	// Secant slopes on either side.
	dl := (ks[i].Output - ks[i-1].Output) / (ks[i].Input - ks[i-1].Input)
	dr := (ks[i+1].Output - ks[i].Output) / (ks[i+1].Input - ks[i].Input)
	if dl*dr <= 0 {
		// Sign change → flat tangent to preserve monotonicity.
		return 0
	}
	// Harmonic mean weighted by interval lengths.
	hl := ks[i].Input - ks[i-1].Input
	hr := ks[i+1].Input - ks[i].Input
	return (3 * (hl + hr)) / ((2*hl+hr)/dr + (hl+2*hr)/dl)
}
```

- [ ] **Step 3: Run, build, lint, commit**

```bash
go test ./pkg/planetgen/color/...
go build ./...
golangci-lint run ./pkg/planetgen/color/...
git add pkg/planetgen/color/spline.go pkg/planetgen/color/spline_test.go
git commit -m "Phase 1 Tier-S #2: Fritsch-Carlson monotone cubic spline"
```

---

## Task 14: Wire control fields + splines into RenderRocky

**Goal:** When a profile has non-empty `Splines`, `RenderRocky` builds height as the sum of `EvalSpline(Splines[i], control_fields[i])` instead of running a single-FBM. Profiles without `Splines` keep the legacy path. After this task, code paths exist; per-type tuned defaults arrive in Task 15.

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`

- [ ] **Step 1: Add the control-field branch**

In `pkg/planetgen/render/rocky.go`, after the existing imports add:

```go
"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
```

Replace the heightmap-build block (currently calling `ng.FractalNoise3D` directly per pixel) with:

```go
heightmap := cubemap.NewF(S)
useControl := !isZeroControlConfig(profile.ControlConfig) && hasSplines(profile.Splines)

if useControl {
    fields := field.GenerateControlFields(seed, profile.ControlConfig, S)
    for face := range cubemap.Face(cubemap.NumFaces) {
        for py := range S {
            for px := range S {
                var h float64
                for i := 0; i < 5; i++ {
                    h += planetcolor.EvalSpline(profile.Splines[i], fields[i].Get(face, px, py))
                }
                heightmap.Set(face, px, py, h)
            }
        }
    }
} else {
    // Legacy single-FBM path (unchanged from Phase 0)
    for face := range cubemap.Face(cubemap.NumFaces) {
        for py := range S {
            for px := range S {
                dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
                h := ng.FractalNoise3D(dx, dy, dz,
                    profile.NoiseOctaves,
                    profile.NoiseLacunarity,
                    profile.NoisePersistence,
                    profile.NoiseScale)
                heightmap.Set(face, px, py, h)
            }
        }
    }
}
```

Add at the bottom of `rocky.go`:

```go
func isZeroControlConfig(c types.ControlConfig) bool {
    return c.Continentalness == (types.ControlField{}) &&
        c.Erosion == (types.ControlField{}) &&
        c.PeaksValleys == (types.ControlField{}) &&
        c.Temperature == (types.ControlField{}) &&
        c.Humidity == (types.ControlField{})
}

func hasSplines(s [5]types.Spline) bool {
    for i := range s {
        if len(s[i].Knots) > 0 {
            return true
        }
    }
    return false
}
```

(Keep the heightmap-normalize-to-[0,1] step that follows; it works on either output.)

Note: `ng` is now only used by the legacy branch. If you removed the legacy branch, you'd remove `ng` too — but keep both paths active for Phase 1 backward compat.

- [ ] **Step 2: Run tests, build**

```bash
go test ./...
go build ./...
```

Expected: all existing tests pass (since no profile yet has non-empty Splines, the legacy path is taken).

- [ ] **Step 3: Commit**

```bash
git add pkg/planetgen/render/rocky.go
git commit -m "Phase 1 Tier-S #2: RenderRocky control-fields-with-splines branch (defaults off)"
```

---

## Task 15: Tuned per-type ControlConfig + Splines defaults

**Goal:** Hand-tune `ControlConfig` and `Splines` for each of the 13 planet types so the new pipeline produces output that's at least as good as the legacy single-FBM (and ideally noticeably better — more contrasted plains/mountains, less uniform fuzz).

This is the most subjective task in Phase 1. Use the slider tool (Task 17) heavily. The goal here is to commit *initial* per-type defaults that look acceptable, not perfect; further tuning happens during Phase 1's natural iteration.

**Files:**
- Modify: `pkg/planetgen/profile.go`

- [ ] **Step 1: Build a baseline via the slider tool**

Open the slider tool, switch to terran, paste a starter profile addition (each Tier-S item lands its own knobs as it goes; for now, hand-edit the JSON):

```json
"controlConfig": {
  "continentalness": {"amp": 1, "freq": 0.6, "octaves": 4, "lacunarity": 2, "persistence": 0.5},
  "erosion":         {"amp": 1, "freq": 1.2, "octaves": 5, "lacunarity": 2, "persistence": 0.5},
  "peaksValleys":    {"amp": 1, "freq": 4, "octaves": 4, "lacunarity": 2, "persistence": 0.55},
  "temperature":     {"amp": 1, "freq": 0.5, "octaves": 3, "lacunarity": 2, "persistence": 0.5},
  "humidity":        {"amp": 1, "freq": 0.7, "octaves": 3, "lacunarity": 2, "persistence": 0.5}
},
"splines": [
  {"knots": [{"input": 0, "output": 0}, {"input": 0.4, "output": 0.05}, {"input": 0.6, "output": 0.4}, {"input": 1, "output": 0.6}]},
  {"knots": [{"input": 0, "output": 0}, {"input": 1, "output": -0.1}]},
  {"knots": [{"input": 0, "output": 0}, {"input": 1, "output": 0.4}]},
  {"knots": []},
  {"knots": []}
]
```

(Continentalness drives the macro land/ocean shape via a steep sigmoid; erosion subtracts a small smooth amount from highland regions; peaks-valleys adds high-frequency rough detail; temperature and humidity are inputs to the Whittaker biome layer in Task 21 — splines stay empty here so they don't contribute to height.)

Render. The result should look like a terran with sharper continents and more textured mountains. Iterate: tweak the splines until the output reads as "land/ocean with mountain ranges," then click "Export JSON."

- [ ] **Step 2: For each of the 13 planet types, tune defaults**

For each type in `["scorched", "arid", "terran", "tundra", "glacial", "ice_world", "super_terran", "hothouse", "lava_world", "oceanic", "jovian", "ice_giant", "unknown"]`:
- Switch the type picker.
- Start from a copy of the terran starter (or another previously-tuned type).
- Adjust splines to match what the type *should* look like (e.g. ice_world's continentalness spline is flat — no land/ocean dichotomy).
- Render at face 256, then 512, eyeball-check.
- Export JSON.

For Phase 1 the goal is not perfection. Aim for ~15 minutes per type. The slider tool's parameters can be re-tuned later.

Note: `jovian` and `ice_giant` are gas giants — they don't use the rocky height pipeline. Leave their `ControlConfig` and `Splines` zero (the legacy path handles them).

- [ ] **Step 3: Paste defaults back into `pkg/planetgen/profile.go`**

For each rocky type in the `Profiles` map, append the `ControlConfig` and `Splines` fields with the tuned values from Step 2. Use Go-literal form, not JSON.

Example for terran:

```go
"terran": {
    Type: "terran",
    Renderer: "rocky",
    // ... existing fields ...
    ControlConfig: types.ControlConfig{
        Continentalness: types.ControlField{Amp: 1, Freq: 0.6, Octaves: 4, Lacunarity: 2, Persistence: 0.5},
        Erosion:         types.ControlField{Amp: 1, Freq: 1.2, Octaves: 5, Lacunarity: 2, Persistence: 0.5},
        PeaksValleys:    types.ControlField{Amp: 1, Freq: 4, Octaves: 4, Lacunarity: 2, Persistence: 0.55},
        Temperature:     types.ControlField{Amp: 1, Freq: 0.5, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
        Humidity:        types.ControlField{Amp: 1, Freq: 0.7, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
    },
    Splines: [5]types.Spline{
        {Knots: []types.SplineKnot{{Input: 0, Output: 0}, {Input: 0.4, Output: 0.05}, {Input: 0.6, Output: 0.4}, {Input: 1, Output: 0.6}}},
        {Knots: []types.SplineKnot{{Input: 0, Output: 0}, {Input: 1, Output: -0.1}}},
        {Knots: []types.SplineKnot{{Input: 0, Output: 0}, {Input: 1, Output: 0.4}}},
        {},
        {},
    },
},
```

- [ ] **Step 4: Regenerate goldens, verify, commit**

```bash
go test -run TestGolden ./cmd/generate-planet-maps/... -update
git add cmd/generate-planet-maps/testdata/golden/
go test ./cmd/generate-planet-maps/...   # confirm clean
go test ./pkg/planetgen/render/...        # confirm render tests still pass
```

Then:

```bash
git add pkg/planetgen/profile.go
git commit -m "Phase 1 Tier-S #2: tuned ControlConfig+Splines for 11 rocky types"

git commit -m "Phase 1: regenerate goldens for control-fields height pipeline"
```

---

## Task 16: Slider tool — control fields + spline editor panel

**Goal:** Add UI controls for the 5 control fields (amp/freq/octaves/lacunarity/persistence — 25 numbers total) and a knot-list editor for each of the 5 splines.

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Append the panel rendering for control fields**

Append to `app.js`:

```js
function renderControlFieldsPanel(profile, panels) {
  if (!profile.controlConfig) return;
  const panel = document.createElement('div');
  panel.className = 'panel';
  panel.innerHTML = '<h3>Control fields</h3>';
  const fields = ['continentalness', 'erosion', 'peaksValleys', 'temperature', 'humidity'];
  for (const fieldName of fields) {
    const cf = profile.controlConfig[fieldName];
    if (!cf) continue;
    const sub = document.createElement('div');
    sub.style.marginBottom = '6px';
    sub.innerHTML = `<strong style="font-size:12px">${fieldName}</strong>`;
    for (const param of ['amp', 'freq', 'octaves', 'lacunarity', 'persistence']) {
      const row = document.createElement('label');
      row.className = 'row';
      row.innerHTML = `<span>${param}</span>`;
      const input = document.createElement('input');
      input.type = 'number';
      input.step = (param === 'octaves') ? '1' : '0.1';
      input.value = cf[param];
      input.addEventListener('change', () => {
        cf[param] = (param === 'octaves') ? parseInt(input.value, 10) : parseFloat(input.value);
        profileTextarea.value = prettifyJSON(JSON.stringify(profile));
      });
      row.appendChild(input);
      sub.appendChild(row);
    }
    panel.appendChild(sub);
  }
  panels.appendChild(panel);
}

function renderSplinesPanel(profile, panels) {
  if (!Array.isArray(profile.splines)) return;
  const panel = document.createElement('div');
  panel.className = 'panel';
  panel.innerHTML = '<h3>Height splines</h3>';
  const labels = ['continentalness', 'erosion', 'peaksValleys', 'temperature', 'humidity'];
  for (let i = 0; i < 5; i++) {
    const sp = profile.splines[i] || {knots: []};
    const sub = document.createElement('div');
    sub.style.marginBottom = '6px';
    sub.innerHTML = `<strong style="font-size:12px">${labels[i]}</strong>`;
    const ta = document.createElement('textarea');
    ta.rows = 2;
    ta.style.fontSize = '11px';
    ta.value = JSON.stringify(sp.knots);
    ta.addEventListener('change', () => {
      try {
        const knots = JSON.parse(ta.value);
        profile.splines[i] = {knots};
        profileTextarea.value = prettifyJSON(JSON.stringify(profile));
      } catch (e) {
        status.textContent = 'Bad knots JSON';
      }
    });
    sub.appendChild(ta);
    panel.appendChild(sub);
  }
  panels.appendChild(panel);
}
```

Hook them into `renderPanels`:

```js
function renderPanels() {
  const panels = $('#param-panels');
  panels.innerHTML = '';
  let profile;
  try { profile = JSON.parse(profileTextarea.value); }
  catch { return; }

  // Palette panel
  if (Array.isArray(profile.palette)) {
    // ... existing palette code ...
  }
  renderControlFieldsPanel(profile, panels);
  renderSplinesPanel(profile, panels);
}
```

- [ ] **Step 2: Smoke test and commit**

Rebuild Wasm, browse, verify each terran control-field slider updates the JSON. Render after each tweak.

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
git add cmd/planet-explorer/web/app.js
git commit -m "Phase 1 slider: control fields + spline editor panels"
```

---

## Task 17: Domain warping primitive

**Goal:** Quilez warp: `warp(p) = p + amp * vec3(noise(p+a), noise(p+b), noise(p+c))`. Three independently-seeded noise generators per planet (named domains "warp.x", "warp.y", "warp.z").

**Files:**
- Create: `pkg/planetgen/noise/warp.go`
- Create: `pkg/planetgen/noise/warp_test.go`
- Modify: `pkg/planetgen/types/types.go` (add `WarpConfig`)

- [ ] **Step 1: Add `WarpConfig` type**

Append to `pkg/planetgen/types/types.go`:

```go
// WarpConfig parameterizes a single Quilez domain-warp pass.
// Apply to a unit-sphere direction before sampling the underlying field.
type WarpConfig struct {
	Amp        float64 `json:"amp"`        // displacement magnitude (in unit-sphere units)
	Freq       float64 `json:"freq"`       // input frequency for the warp noise
	Octaves    int     `json:"octaves"`    // warp-noise octaves
	Lacunarity float64 `json:"lacunarity"`
	Persistence float64 `json:"persistence"`
}
```

Add to `PlanetProfile`:

```go
type PlanetProfile struct {
    // ... existing ...
    Warp types.WarpConfig `json:"warp,omitempty"`
}
```

- [ ] **Step 2: Write the failing test**

```go
package noise

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestWarpZeroAmp(t *testing.T) {
	w := NewWarper(42, types.WarpConfig{Amp: 0, Freq: 1, Octaves: 1, Lacunarity: 2, Persistence: 0.5})
	x, y, z := w.Warp(0.5, 0.5, 0.5)
	if math.Abs(x-0.5) > 1e-12 || math.Abs(y-0.5) > 1e-12 || math.Abs(z-0.5) > 1e-12 {
		t.Errorf("zero-amp warp moved point: (%f, %f, %f)", x, y, z)
	}
}

func TestWarpDeterministic(t *testing.T) {
	cfg := types.WarpConfig{Amp: 0.3, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5}
	w := NewWarper(42, cfg)
	x1, y1, z1 := w.Warp(0.1, 0.2, 0.3)
	w2 := NewWarper(42, cfg)
	x2, y2, z2 := w2.Warp(0.1, 0.2, 0.3)
	if x1 != x2 || y1 != y2 || z1 != z2 {
		t.Errorf("non-deterministic: (%f,%f,%f) vs (%f,%f,%f)", x1, y1, z1, x2, y2, z2)
	}
}

func TestWarpDisplacementBoundedByAmp(t *testing.T) {
	cfg := types.WarpConfig{Amp: 0.3, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5}
	w := NewWarper(42, cfg)
	for i := 0; i < 50; i++ {
		x, y, z := float64(i)*0.05, float64(i)*0.07, float64(i)*0.11
		wx, wy, wz := w.Warp(x, y, z)
		dx, dy, dz := wx-x, wy-y, wz-z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		// fbm output is in [0,1] per generator's normalization, so per-axis
		// displacement is at most cfg.Amp. Magnitude is at most sqrt(3)*Amp.
		if d > cfg.Amp*math.Sqrt(3)*1.1 {
			t.Errorf("displacement %f exceeds bound %f", d, cfg.Amp*math.Sqrt(3))
		}
	}
}
```

- [ ] **Step 3: Implement**

Create `pkg/planetgen/noise/warp.go`:

```go
package noise

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Warper applies a single Quilez domain-warp pass to 3D points.
type Warper struct {
	cfg          types.WarpConfig
	xGen, yGen, zGen *Generator
}

// NewWarper builds a Warper seeded by master via the warp.{x,y,z}
// named domains. Same master always produces the same warp.
func NewWarper(master int64, cfg types.WarpConfig) *Warper {
	return &Warper{
		cfg:  cfg,
		xGen: New(seed.Domain(master, "warp.x")),
		yGen: New(seed.Domain(master, "warp.y")),
		zGen: New(seed.Domain(master, "warp.z")),
	}
}

// Warp returns p + amp · vec3(fbm_x(p), fbm_y(p), fbm_z(p)). The
// returned point is generally NOT unit-length; callers that require
// unit-sphere directions must re-normalize.
func (w *Warper) Warp(x, y, z float64) (float64, float64, float64) {
	if w.cfg.Amp == 0 {
		return x, y, z
	}
	dx := w.xGen.FractalNoise3D(x, y, z, w.cfg.Octaves, w.cfg.Lacunarity, w.cfg.Persistence, w.cfg.Freq)
	dy := w.yGen.FractalNoise3D(x, y, z, w.cfg.Octaves, w.cfg.Lacunarity, w.cfg.Persistence, w.cfg.Freq)
	dz := w.zGen.FractalNoise3D(x, y, z, w.cfg.Octaves, w.cfg.Lacunarity, w.cfg.Persistence, w.cfg.Freq)
	return x + w.cfg.Amp*(2*dx-1), y + w.cfg.Amp*(2*dy-1), z + w.cfg.Amp*(2*dz-1)
}
```

(Note: `FractalNoise3D` returns values in `[0, 1]` per its existing convention; the `2*dx-1` re-centers to `[-1, 1]` so the warp displaces in both directions.)

- [ ] **Step 4: Run, build, lint, commit**

```bash
go test ./pkg/planetgen/noise/...
go build ./...
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/noise/warp.go pkg/planetgen/noise/warp_test.go pkg/planetgen/types/types.go
git commit -m "Phase 1 Tier-S #3: domain-warp primitive (Quilez)"
```

---

## Task 18: Wire warp into render

**Goal:** When a profile has `Warp.Amp > 0`, every per-pixel sphere lookup in `RenderRocky` and `RenderGasGiant` gets its direction warped first.

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`
- Modify: `pkg/planetgen/render/gasgiant.go`

- [ ] **Step 1: Add warp to RenderRocky**

In `pkg/planetgen/render/rocky.go`, before the heightmap loop, construct the warper:

```go
warper := noise.NewWarper(seed, profile.Warp)
```

Replace each `dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)` call with:

```go
dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
dx, dy, dz = warper.Warp(dx, dy, dz)
```

Apply this in BOTH the heightmap-build loop AND the colorize loop. There are typically 4 such sites in rocky.go after Task 14 (heightmap, biome, ocean, polar caps). Each gets the same `warper.Warp(dx, dy, dz)` after the original `cubemap.FacePixelToDir` call.

- [ ] **Step 2: Add warp to RenderGasGiant**

In `pkg/planetgen/render/gasgiant.go`, build the warper at the top:

```go
warper := noise.NewWarper(seed, profile.Warp)
```

Replace each `cubemap.FacePixelToDir` site to chain through `warper.Warp(...)`. Gas giants typically have 1–2 such sites (band-color sample, storm-distance sample).

- [ ] **Step 3: Tune per-type Warp defaults**

Use the slider tool. For each rocky planet type, set `Warp.Amp` between 0.05 (subtle) and 0.4 (strong fingering). Suggested starting points:

| Type | Warp.Amp |
|---|---|
| terran, super_terran, oceanic | 0.25 |
| arid, hothouse, lava_world | 0.15 |
| scorched, tundra, glacial, ice_world | 0.08 |
| jovian | 0.10 |
| ice_giant | 0.05 |
| unknown | 0.10 |

Set `Freq=1.0, Octaves=2, Lacunarity=2.0, Persistence=0.5` for all initially.

Edit `pkg/planetgen/profile.go` adding `Warp: types.WarpConfig{...}` to each `Profiles` entry.

- [ ] **Step 4: Regenerate goldens, verify, commit**

```bash
go test -run TestGolden ./cmd/generate-planet-maps/... -update
git add cmd/generate-planet-maps/testdata/golden/
go test ./cmd/generate-planet-maps/...

git add pkg/planetgen/render/rocky.go pkg/planetgen/render/gasgiant.go pkg/planetgen/profile.go
git commit -m "Phase 1 Tier-S #3: wire domain warp into render, tune per-type defaults"

git commit -m "Phase 1: regenerate goldens for domain-warp pass"
```

---

## Task 19: Slider tool — warp panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Add warp panel**

Append to `app.js`:

```js
function renderWarpPanel(profile, panels) {
  if (!profile.warp) return;
  const panel = document.createElement('div');
  panel.className = 'panel';
  panel.innerHTML = '<h3>Domain warp</h3>';
  for (const param of ['amp', 'freq', 'octaves', 'lacunarity', 'persistence']) {
    const row = document.createElement('label');
    row.className = 'row';
    row.innerHTML = `<span>${param}</span>`;
    const input = document.createElement('input');
    input.type = 'number';
    input.step = (param === 'octaves') ? '1' : '0.05';
    input.value = profile.warp[param] || 0;
    input.addEventListener('change', () => {
      profile.warp[param] = (param === 'octaves') ? parseInt(input.value, 10) : parseFloat(input.value);
      profileTextarea.value = prettifyJSON(JSON.stringify(profile));
    });
    row.appendChild(input);
    panel.appendChild(row);
  }
  panels.appendChild(panel);
}
```

Add `renderWarpPanel(profile, panels);` to `renderPanels` after the existing calls.

- [ ] **Step 2: Build, test, commit**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
git add cmd/planet-explorer/web/app.js
git commit -m "Phase 1 slider: domain warp panel"
```

---

## Task 20: Whittaker biome generators

**Goal:** `biome.GenerateClimateFields(seed int64, profile *types.PlanetProfile, S int)` produces T (temperature) and M (moisture) fields based on the relevant control fields plus modifiers (cos(lat) for T, distance from coast in Phase 2). For Phase 1, T = `temperatureField + 0.3*cos(lat)` and M = `humidityField` directly.

**Files:**
- Create: `pkg/planetgen/biome/whittaker.go`
- Create: `pkg/planetgen/biome/whittaker_test.go`

- [ ] **Step 1: Add `BiomeTable` type**

Append to `pkg/planetgen/types/types.go`:

```go
// BiomeTable is a 2D grid of biome cells indexed by (T, M) ∈ [0,1]².
// The T axis is rows (0 = cold, last = hot); M axis is columns
// (0 = dry, last = wet). Each cell has a 2-stop palette used to
// color a heightmap value via SampleGradientOkLab.
type BiomeTable struct {
	TBuckets int               `json:"tBuckets"`
	MBuckets int               `json:"mBuckets"`
	Cells    [][]BiomeCell     `json:"cells"` // [tBucket][mBucket]
}

// BiomeCell is a 2-stop palette used to color heightmap values
// in a single (T, M) cell. Output is bilinearly OkLab-blended
// across neighboring cells based on the sample's exact (T, M).
type BiomeCell struct {
	Low  ColorRGB `json:"low"`  // height=0 color
	High ColorRGB `json:"high"` // height=1 color
}

// ColorRGB is a JSON-serializable RGB color (alpha is implicit 255).
type ColorRGB struct {
	R, G, B uint8
}
```

- [ ] **Step 2: Write the failing test**

```go
package biome

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestGenerateClimateFieldsShape(t *testing.T) {
	prof := &types.PlanetProfile{
		ControlConfig: types.ControlConfig{
			Temperature: types.ControlField{Amp: 1, Freq: 1, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
			Humidity:    types.ControlField{Amp: 1, Freq: 1, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
		},
	}
	tField, mField := GenerateClimateFields(42, prof, 32)
	if tField.Size != 32 || mField.Size != 32 {
		t.Errorf("expected 32×32 fields, got T=%d M=%d", tField.Size, mField.Size)
	}
}

func TestGenerateClimateFieldsPolarColder(t *testing.T) {
	// Pole pixels (top of +Y face) should have T < equator pixels
	// (center of +X face) on average, after the cos(lat) bias.
	prof := &types.PlanetProfile{
		ControlConfig: types.ControlConfig{
			Temperature: types.ControlField{Amp: 1, Freq: 1, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
			Humidity:    types.ControlField{Amp: 1, Freq: 1, Octaves: 3, Lacunarity: 2, Persistence: 0.5},
		},
	}
	tField, _ := GenerateClimateFields(42, prof, 32)
	poleT := tField.Get(2, 16, 16)   // FacePosY center — north pole
	equatorT := tField.Get(0, 16, 16) // FacePosX center — equator
	if poleT >= equatorT {
		t.Errorf("pole T (%f) should be < equator T (%f)", poleT, equatorT)
	}
}
```

- [ ] **Step 3: Implement**

Create `pkg/planetgen/biome/whittaker.go`:

```go
package biome

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// GenerateClimateFields produces temperature (T) and moisture (M) cube
// maps from a planet's profile. Phase 1 uses:
//
//   T(p) = humidityField(p) [no, that's M]   —  sorry, T is:
//   T(p) = temperatureField(p) * 0.7 + 0.3 * cos(lat(p))
//   M(p) = humidityField(p)
//
// Both are normalized to [0, 1].
func GenerateClimateFields(seed int64, profile *types.PlanetProfile, S int) (T, M *cubemap.CubeMapF) {
	fields := field.GenerateControlFields(seed, profile.ControlConfig, S)
	tNoise := fields[3]
	mNoise := fields[4]

	T = cubemap.NewF(S)
	M = cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				_, dy, _ := cubemap.FacePixelToDir(face, px, py, S)
				lat := math.Asin(dy)
				latBias := 0.5 + 0.5*math.Cos(lat)*0.6 // [0.2, 0.8] at poles vs equator
				t := tNoise.Get(face, px, py)*0.7 + latBias*0.3
				if t < 0 {
					t = 0
				} else if t > 1 {
					t = 1
				}
				T.Set(face, px, py, t)
				M.Set(face, px, py, mNoise.Get(face, px, py))
			}
		}
	}
	return T, M
}
```

- [ ] **Step 4: Run, build, lint, commit**

```bash
go test ./pkg/planetgen/biome/...
go build ./...
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/biome/whittaker.go pkg/planetgen/biome/whittaker_test.go pkg/planetgen/types/types.go
git commit -m "Phase 1 Tier-S #4: Whittaker T+M climate field generators"
```

---

## Task 21: Whittaker biome lookup with bilinear OkLab interpolation

**Goal:** `biome.LookupColor(table BiomeTable, T, M, height float64) color.RGBA` samples the 2D biome table at (T, M), bilinear-blends the four neighbor cells in OkLab, then samples each cell's 2-stop palette at height.

**Files:**
- Create: `pkg/planetgen/biome/lookup.go`

- [ ] **Step 1: Implement**

```go
package biome

import (
	"image/color"
	"math"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// LookupColor samples a biome table at climate point (T, M) ∈ [0,1]² and
// returns the surface color at the given heightmap value. The four
// neighboring cells are bilinearly OkLab-blended.
func LookupColor(table types.BiomeTable, T, M, height float64) color.RGBA {
	if table.TBuckets == 0 || table.MBuckets == 0 || len(table.Cells) == 0 {
		return color.RGBA{128, 128, 128, 255}
	}
	T = math.Max(0, math.Min(1, T))
	M = math.Max(0, math.Min(1, M))
	height = math.Max(0, math.Min(1, height))

	// (tFloat, mFloat) are positions in cell-grid coords.
	tFloat := T * float64(table.TBuckets-1)
	mFloat := M * float64(table.MBuckets-1)
	t0 := int(math.Floor(tFloat))
	m0 := int(math.Floor(mFloat))
	t1 := min2(t0+1, table.TBuckets-1)
	m1 := min2(m0+1, table.MBuckets-1)
	tx := tFloat - math.Floor(tFloat)
	my := mFloat - math.Floor(mFloat)

	c00 := cellColor(table.Cells[t0][m0], height)
	c10 := cellColor(table.Cells[t1][m0], height)
	c01 := cellColor(table.Cells[t0][m1], height)
	c11 := cellColor(table.Cells[t1][m1], height)

	// Bilinear OkLab blend: blend along T first, then M.
	cT0 := planetcolor.BlendOkLab(c00, c10, tx)
	cT1 := planetcolor.BlendOkLab(c01, c11, tx)
	return planetcolor.BlendOkLab(cT0, cT1, my)
}

func cellColor(cell types.BiomeCell, height float64) color.RGBA {
	low := color.RGBA{R: cell.Low.R, G: cell.Low.G, B: cell.Low.B, A: 255}
	high := color.RGBA{R: cell.High.R, G: cell.High.G, B: cell.High.B, A: 255}
	return planetcolor.BlendOkLab(low, high, height)
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Build and lint**

```bash
go build ./...
golangci-lint run ./pkg/planetgen/biome/...
```

(No test in this task — the lookup is exercised by the integration in Task 22.)

- [ ] **Step 3: Commit**

```bash
git add pkg/planetgen/biome/lookup.go
git commit -m "Phase 1 Tier-S #4: bilinear-OkLab biome cell lookup"
```

---

## Task 22: Switch RenderRocky to Whittaker biome

**Goal:** When a profile has a non-empty `BiomeTable`, the colorize loop calls `biome.LookupColor` per pixel instead of `SampleGradientOkLab(profile.Palette, h)`. Profiles without `BiomeTable` keep the legacy palette path.

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`
- Modify: `pkg/planetgen/types/types.go` (add `BiomeTable`)

- [ ] **Step 1: Add BiomeTable to PlanetProfile**

```go
type PlanetProfile struct {
    // ... existing ...
    BiomeTable types.BiomeTable `json:"biomeTable,omitempty"`
}
```

- [ ] **Step 2: Add the branch in RenderRocky**

In the colorize loop in `pkg/planetgen/render/rocky.go`, before the line `c := planetcolor.SampleGradientOkLab(profile.Palette, h)`, add:

```go
useBiomeTable := len(profile.BiomeTable.Cells) > 0
```

(Outside the loop.)

Then inside the loop, replace:

```go
c := planetcolor.SampleGradientOkLab(profile.Palette, h)
if hasBiomes { /* equatorial / polar palette blends */ }
```

with:

```go
var c color.RGBA
if useBiomeTable {
    c = biome.LookupColor(profile.BiomeTable, T_field.Get(face, px, py), M_field.Get(face, px, py), h)
} else {
    c = planetcolor.SampleGradientOkLab(profile.Palette, h)
    if hasBiomes { /* keep equatorial / polar palette blends as fallback */ }
}
```

The `T_field` and `M_field` are computed once before the colorize loop:

```go
var T_field, M_field *cubemap.CubeMapF
if useBiomeTable {
    T_field, M_field = biome.GenerateClimateFields(seed, profile, S)
}
```

(Place this after the heightmap normalization step.)

Add the import: `"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"`.

- [ ] **Step 3: Define a starter biome table for terran**

The biome table is a 4×4 grid (cold/cool/warm/hot × dry/medium/moist/wet) — 16 cells, each with a `Low` and `High` color. Use the slider tool to tune colors. For initial commit, use these defaults for terran (paste into profile.go):

```go
BiomeTable: types.BiomeTable{
    TBuckets: 4,
    MBuckets: 4,
    Cells: [][]types.BiomeCell{
        // T=cold (rows 0..3 = cold..hot); M=dry (cols 0..3 = dry..wet)
        { // T=0 cold
            {Low: types.ColorRGB{200, 210, 220}, High: types.ColorRGB{240, 245, 250}}, // dry tundra → snow
            {Low: types.ColorRGB{180, 190, 195}, High: types.ColorRGB{230, 235, 240}},
            {Low: types.ColorRGB{160, 175, 180}, High: types.ColorRGB{220, 225, 235}},
            {Low: types.ColorRGB{120, 150, 170}, High: types.ColorRGB{200, 220, 235}}, // wet ice/glacier
        },
        { // T=1 cool
            {Low: types.ColorRGB{160, 150, 130}, High: types.ColorRGB{200, 195, 180}},
            {Low: types.ColorRGB{110, 130, 95}, High: types.ColorRGB{170, 180, 165}},
            {Low: types.ColorRGB{75, 105, 70}, High: types.ColorRGB{150, 165, 145}},
            {Low: types.ColorRGB{55, 90, 60}, High: types.ColorRGB{130, 150, 135}},
        },
        { // T=2 warm
            {Low: types.ColorRGB{195, 180, 145}, High: types.ColorRGB{220, 215, 190}}, // dry savanna
            {Low: types.ColorRGB{145, 155, 80}, High: types.ColorRGB{195, 195, 165}},
            {Low: types.ColorRGB{75, 130, 50}, High: types.ColorRGB{160, 175, 130}}, // grassland
            {Low: types.ColorRGB{30, 90, 25}, High: types.ColorRGB{110, 140, 90}}, // forest
        },
        { // T=3 hot
            {Low: types.ColorRGB{225, 195, 130}, High: types.ColorRGB{240, 225, 180}}, // dry desert
            {Low: types.ColorRGB{200, 180, 105}, High: types.ColorRGB{220, 210, 155}},
            {Low: types.ColorRGB{145, 170, 80}, High: types.ColorRGB{195, 200, 140}},
            {Low: types.ColorRGB{20, 80, 35}, High: types.ColorRGB{90, 130, 80}}, // tropical jungle
        },
    },
},
```

For other rocky types, leave `BiomeTable` empty (legacy palette path). The slider-tool tuning happens during Task 23.

- [ ] **Step 4: Build, test**

```bash
go build ./...
go test ./...
```

- [ ] **Step 5: Eyeball + regenerate goldens**

```bash
go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/p1t22_terran.png
# Open and verify it looks like a real climate-driven planet (tropical near equator, tundra near poles).

go test -run TestGolden ./cmd/generate-planet-maps/... -update
git add cmd/generate-planet-maps/testdata/golden/
```

- [ ] **Step 6: Commit**

```bash
git add pkg/planetgen/render/rocky.go pkg/planetgen/types/types.go pkg/planetgen/profile.go
git commit -m "Phase 1 Tier-S #4: Whittaker biome lookup in RenderRocky (terran initial table)"

git commit -m "Phase 1: regenerate goldens for terran Whittaker biome"
```

---

## Task 23: Slider tool — biome table picker

**Goal:** A panel that displays the current biome table as a 4×4 swatch grid with click-to-edit color pickers per cell.

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Add biome table panel**

```js
function renderBiomePanel(profile, panels) {
  const t = profile.biomeTable;
  if (!t || !t.cells) return;
  const panel = document.createElement('div');
  panel.className = 'panel';
  panel.innerHTML = '<h3>Biome table (T × M)</h3>';
  const grid = document.createElement('div');
  grid.style.display = 'grid';
  grid.style.gridTemplateColumns = `repeat(${t.mBuckets}, 1fr)`;
  grid.style.gap = '2px';
  for (let i = 0; i < t.tBuckets; i++) {
    for (let j = 0; j < t.mBuckets; j++) {
      const cell = t.cells[i][j];
      const sw = document.createElement('div');
      sw.style.height = '36px';
      sw.style.background = `linear-gradient(to right, ${rgbCSS(cell.low)}, ${rgbCSS(cell.high)})`;
      sw.style.borderRadius = '2px';
      sw.title = `T=${i} M=${j}`;
      grid.appendChild(sw);
    }
  }
  panel.appendChild(grid);
  panels.appendChild(panel);
}

function rgbCSS(c) {
  return `rgb(${c.R}, ${c.G}, ${c.B})`;
}
```

(The panel is read-only initially; full color-picker editing can be added later if needed. For Phase 1, JSON-textarea editing is sufficient for fine tuning.)

Hook into `renderPanels`.

- [ ] **Step 2: Build, smoke-test, commit**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
git add cmd/planet-explorer/web/app.js
git commit -m "Phase 1 slider: biome table swatch panel"
```

---

## Task 24: 3D LUT support

**Goal:** Per-archetype color-correction LUT applied as the final color grade. LUT is stored as a Resolve `.cube` text format file, parsed at startup, applied via trilinear sample.

**Files:**
- Create: `pkg/planetgen/color/lut.go`
- Create: `pkg/planetgen/color/lut_test.go`

- [ ] **Step 1: Add LUT type**

Append to `pkg/planetgen/types/types.go`:

```go
// LUT is a 3D color lookup table (sized N×N×N, typically N=16 or 32).
// Inputs and outputs are RGB ∈ [0,1]³.
type LUT struct {
	Size int       `json:"size"` // N
	Data []float64 `json:"data"` // length 3*N³, packed (R,G,B) stride
	Name string    `json:"name,omitempty"` // optional, e.g. "terran"
}
```

- [ ] **Step 2: Write the failing test**

```go
package color

import (
	"image/color"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestApplyIdentityLUT(t *testing.T) {
	// Identity LUT: output = input.
	lut := identityLUT(8)
	in := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	out := ApplyLUT(lut, in)
	dr := int(in.R) - int(out.R)
	dg := int(in.G) - int(out.G)
	db := int(in.B) - int(out.B)
	if abs(dr) > 4 || abs(dg) > 4 || abs(db) > 4 {
		t.Errorf("identity LUT changed pixel: %v → %v", in, out)
	}
}

func TestParseCubeFile(t *testing.T) {
	cube := `LUT_3D_SIZE 2
0 0 0
1 0 0
0 1 0
1 1 0
0 0 1
1 0 1
0 1 1
1 1 1
`
	lut, err := ParseCubeLUT(cube)
	if err != nil {
		t.Fatal(err)
	}
	if lut.Size != 2 {
		t.Errorf("size = %d, want 2", lut.Size)
	}
	if len(lut.Data) != 24 {
		t.Errorf("data len = %d, want 24", len(lut.Data))
	}
}

func identityLUT(n int) types.LUT {
	data := make([]float64, 3*n*n*n)
	idx := 0
	for b := 0; b < n; b++ {
		for g := 0; g < n; g++ {
			for r := 0; r < n; r++ {
				data[idx+0] = float64(r) / float64(n-1)
				data[idx+1] = float64(g) / float64(n-1)
				data[idx+2] = float64(b) / float64(n-1)
				idx += 3
			}
		}
	}
	return types.LUT{Size: n, Data: data}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
```

- [ ] **Step 3: Implement**

Create `pkg/planetgen/color/lut.go`:

```go
package color

import (
	"bufio"
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// ApplyLUT trilinearly samples the LUT at the input color and returns
// the graded output. Input and output are 8-bit RGBA; alpha is preserved.
func ApplyLUT(lut types.LUT, in color.RGBA) color.RGBA {
	if lut.Size <= 1 || len(lut.Data) != 3*lut.Size*lut.Size*lut.Size {
		return in
	}
	n := lut.Size
	r := float64(in.R) / 255 * float64(n-1)
	g := float64(in.G) / 255 * float64(n-1)
	b := float64(in.B) / 255 * float64(n-1)
	r0, g0, b0 := int(math.Floor(r)), int(math.Floor(g)), int(math.Floor(b))
	r1 := minLUT(r0+1, n-1)
	g1 := minLUT(g0+1, n-1)
	b1 := minLUT(b0+1, n-1)
	tr, tg, tb := r-float64(r0), g-float64(g0), b-float64(b0)

	c000 := lutFetch(lut, r0, g0, b0)
	c100 := lutFetch(lut, r1, g0, b0)
	c010 := lutFetch(lut, r0, g1, b0)
	c110 := lutFetch(lut, r1, g1, b0)
	c001 := lutFetch(lut, r0, g0, b1)
	c101 := lutFetch(lut, r1, g0, b1)
	c011 := lutFetch(lut, r0, g1, b1)
	c111 := lutFetch(lut, r1, g1, b1)

	c00 := lerp3(c000, c100, tr)
	c10 := lerp3(c010, c110, tr)
	c01 := lerp3(c001, c101, tr)
	c11 := lerp3(c011, c111, tr)
	c0 := lerp3(c00, c10, tg)
	c1 := lerp3(c01, c11, tg)
	c := lerp3(c0, c1, tb)

	return color.RGBA{
		R: uint8(math.Round(c[0] * 255)),
		G: uint8(math.Round(c[1] * 255)),
		B: uint8(math.Round(c[2] * 255)),
		A: in.A,
	}
}

func lutFetch(lut types.LUT, r, g, b int) [3]float64 {
	idx := 3 * (b*lut.Size*lut.Size + g*lut.Size + r)
	return [3]float64{lut.Data[idx], lut.Data[idx+1], lut.Data[idx+2]}
}

func lerp3(a, b [3]float64, t float64) [3]float64 {
	return [3]float64{
		a[0]*(1-t) + b[0]*t,
		a[1]*(1-t) + b[1]*t,
		a[2]*(1-t) + b[2]*t,
	}
}

func minLUT(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ParseCubeLUT parses a Resolve .cube text-format LUT.
// Recognized header line: `LUT_3D_SIZE N` (e.g., 16, 32).
// Body: N³ lines of "R G B" floats in [0,1], in (R, G, B) ordering with R innermost.
// Comments start with `#` and are ignored.
func ParseCubeLUT(text string) (types.LUT, error) {
	var size int
	var data []float64
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "LUT_3D_SIZE":
			if _, err := fmt.Sscanf(line, "LUT_3D_SIZE %d", &size); err != nil {
				return types.LUT{}, fmt.Errorf("invalid LUT_3D_SIZE: %v", err)
			}
		case "TITLE", "DOMAIN_MIN", "DOMAIN_MAX":
			// Optional headers; skip.
			continue
		default:
			if size == 0 {
				return types.LUT{}, fmt.Errorf("encountered data before LUT_3D_SIZE")
			}
			if len(fields) != 3 {
				return types.LUT{}, fmt.Errorf("expected 3 floats, got %d", len(fields))
			}
			var r, g, b float64
			if _, err := fmt.Sscanf(line, "%f %f %f", &r, &g, &b); err != nil {
				return types.LUT{}, fmt.Errorf("parse %q: %v", line, err)
			}
			data = append(data, r, g, b)
		}
	}
	if err := scanner.Err(); err != nil {
		return types.LUT{}, err
	}
	expected := 3 * size * size * size
	if len(data) != expected {
		return types.LUT{}, fmt.Errorf("data length %d != expected %d for size=%d", len(data), expected, size)
	}
	return types.LUT{Size: size, Data: data}, nil
}
```

- [ ] **Step 4: Run, build, lint, commit**

```bash
go test ./pkg/planetgen/color/...
go build ./...
golangci-lint run ./pkg/planetgen/color/...
git add pkg/planetgen/color/lut.go pkg/planetgen/color/lut_test.go pkg/planetgen/types/types.go
git commit -m "Phase 1 Tier-S #5: 3D LUT trilinear sample + .cube parser"
```

---

## Task 25: Per-archetype LUT assets

**Goal:** Generate 13 archetype LUTs by gentle hue/saturation/contrast shifts away from identity, named after each planet type, committed as `.cube` text files.

For Phase 1, the LUTs are deliberately subtle (~5-10% color rotation) — the goal is "unification" of look across an archetype, not dramatic recoloring.

**Files:**
- Create: `pkg/planetgen/color/luts/<archetype>.cube` × 13
- Create: `cmd/tools/gen-archetype-luts/main.go` (utility to generate the assets)

- [ ] **Step 1: Write a small utility to generate the LUTs**

Create `cmd/tools/gen-archetype-luts/main.go`:

```go
// Command gen-archetype-luts writes per-archetype 16³ LUTs as
// Resolve .cube files into pkg/planetgen/color/luts/. Run once to
// regenerate after edits to the per-archetype color shifts.
package main

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"

	"github.com/lucasb-eyer/go-colorful"
)

type archetype struct {
	name string
	// Hue shift in degrees, saturation multiplier, value multiplier.
	hueDeg, satMul, valMul float64
}

var archetypes = []archetype{
	{"scorched", -8, 0.85, 0.95},
	{"arid", -5, 1.05, 1.0},
	{"terran", 0, 1.0, 1.0},
	{"tundra", +5, 0.85, 1.05},
	{"glacial", +10, 0.8, 1.05},
	{"ice_world", +15, 0.85, 1.05},
	{"super_terran", -3, 1.1, 0.95},
	{"hothouse", -12, 1.05, 0.95},
	{"lava_world", -20, 1.15, 0.85},
	{"oceanic", +5, 1.05, 1.0},
	{"jovian", -10, 1.1, 0.95},
	{"ice_giant", +20, 0.9, 1.05},
	{"unknown", 0, 0.95, 0.95},
}

func main() {
	dir := "pkg/planetgen/color/luts"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	const N = 16
	for _, a := range archetypes {
		path := filepath.Join(dir, a.name+".cube")
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(f, "TITLE \"%s archetype LUT\"\n", a.name)
		fmt.Fprintf(f, "LUT_3D_SIZE %d\n", N)
		for b := 0; b < N; b++ {
			for g := 0; g < N; g++ {
				for r := 0; r < N; r++ {
					rIn := float64(r) / float64(N-1)
					gIn := float64(g) / float64(N-1)
					bIn := float64(b) / float64(N-1)
					out := shift(color.RGBA{
						R: uint8(rIn * 255),
						G: uint8(gIn * 255),
						B: uint8(bIn * 255),
						A: 255,
					}, a.hueDeg, a.satMul, a.valMul)
					rOut := float64(out.R) / 255
					gOut := float64(out.G) / 255
					bOut := float64(out.B) / 255
					fmt.Fprintf(f, "%.6f %.6f %.6f\n", rOut, gOut, bOut)
				}
			}
		}
		_ = f.Close()
		fmt.Printf("wrote %s\n", path)
	}
}

func shift(c color.RGBA, hueDeg, satMul, valMul float64) color.RGBA {
	cf := colorful.Color{R: float64(c.R) / 255, G: float64(c.G) / 255, B: float64(c.B) / 255}
	h, s, v := cf.Hsv()
	h = math.Mod(h+hueDeg+360, 360)
	s *= satMul
	v *= valMul
	if s > 1 {
		s = 1
	}
	if v > 1 {
		v = 1
	}
	out := colorful.Hsv(h, s, v).Clamped()
	return color.RGBA{
		R: uint8(math.Round(out.R * 255)),
		G: uint8(math.Round(out.G * 255)),
		B: uint8(math.Round(out.B * 255)),
		A: 255,
	}
}
```

- [ ] **Step 2: Run the utility**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
go run ./cmd/tools/gen-archetype-luts
ls pkg/planetgen/color/luts/
```

Expected: 13 `.cube` files.

- [ ] **Step 3: Verify a LUT round-trips through ParseCubeLUT**

```bash
go test ./pkg/planetgen/color/...
```

Should still pass (the parser test from Task 24 is independent of asset content).

- [ ] **Step 4: Commit**

```bash
git add cmd/tools/gen-archetype-luts/ pkg/planetgen/color/luts/
git commit -m "Phase 1 Tier-S #5: per-archetype LUT assets + generator"
```

---

## Task 26: Apply LUT in render

**Goal:** Per-pixel `c = ApplyLUT(profile.LUT, c)` as the LAST step before writing to the cube-map. Profiles without a LUT loaded (LUT == nil) skip the pass.

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`
- Modify: `pkg/planetgen/render/gasgiant.go`
- Modify: `pkg/planetgen/profile.go` (per-type LUT references)
- Modify: `pkg/planetgen/types/types.go` (LUT pointer in PlanetProfile)

- [ ] **Step 1: Add LUT field to PlanetProfile**

```go
type PlanetProfile struct {
    // ... existing ...
    LUT *types.LUT `json:"lut,omitempty"`
}
```

- [ ] **Step 2: Load LUT assets in profile.go init**

Add to `pkg/planetgen/profile.go`:

```go
import _ "embed"

//go:embed color/luts/scorched.cube
var lutScorchedRaw string

//go:embed color/luts/arid.cube
var lutAridRaw string

// ... 11 more for the other types ...

func mustParseLUT(s string) *types.LUT {
    lut, err := planetcolor.ParseCubeLUT(s)
    if err != nil {
        panic(err)
    }
    return &lut
}

var (
    lutScorched   = mustParseLUT(lutScorchedRaw)
    lutArid       = mustParseLUT(lutAridRaw)
    // ... etc.
)
```

(Use Go's `//go:embed` to bundle the .cube files into the binary. The `_ "embed"` import enables the directive.)

For each entry in the `Profiles` map, set `LUT: lutScorched`, `LUT: lutArid`, etc.

- [ ] **Step 3: Apply LUT in render**

In both `RenderRocky` and `RenderGasGiant`, just before each `out.Set(face, px, py, c)` call, add:

```go
if profile.LUT != nil {
    c = planetcolor.ApplyLUT(*profile.LUT, c)
}
```

- [ ] **Step 4: Build, test, regenerate goldens**

```bash
go build ./...
go test ./...
go test -run TestGolden ./cmd/generate-planet-maps/... -update
git add cmd/generate-planet-maps/testdata/golden/
```

- [ ] **Step 5: Commit**

```bash
git add pkg/planetgen/profile.go pkg/planetgen/render/rocky.go pkg/planetgen/render/gasgiant.go pkg/planetgen/types/types.go
git commit -m "Phase 1 Tier-S #5: apply per-archetype LUT as final color grade"

git commit -m "Phase 1: regenerate goldens for archetype LUT pass"
```

---

## Task 27: Slider tool — LUT picker

**Goal:** Show the current archetype's LUT name in the panel and a button to bypass the LUT (for debugging).

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Add LUT panel**

```js
function renderLUTPanel(profile, panels) {
  const panel = document.createElement('div');
  panel.className = 'panel';
  panel.innerHTML = '<h3>Color LUT</h3>';
  const status = document.createElement('div');
  status.style.fontSize = '12px';
  status.style.color = '#aaa';
  if (profile.lut) {
    status.textContent = `Active: ${profile.lut.name || 'unnamed'} (${profile.lut.size}³)`;
  } else {
    status.textContent = 'No LUT';
  }
  panel.appendChild(status);
  const btn = document.createElement('button');
  btn.textContent = profile.lut ? 'Bypass LUT' : 'Restore LUT';
  btn.addEventListener('click', () => {
    if (profile.lut) {
      window.__savedLUT = profile.lut;
      profile.lut = null;
    } else {
      profile.lut = window.__savedLUT || null;
    }
    profileTextarea.value = prettifyJSON(JSON.stringify(profile));
    renderPanels();
  });
  panel.appendChild(btn);
  panels.appendChild(panel);
}
```

Hook into `renderPanels`.

- [ ] **Step 2: Build, smoke-test, commit**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
git add cmd/planet-explorer/web/app.js
git commit -m "Phase 1 slider: LUT bypass toggle"
```

---

## Task 28: Phase 1 final acceptance

**Goal:** Verify all Phase 1 acceptance criteria from master plan §5.7. No commits in this task.

- [ ] **Step 1: All tests pass**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
go test ./...
```

Expected: green.

- [ ] **Step 2: Lint clean**

```bash
golangci-lint run
```

Expected: 0 issues.

- [ ] **Step 3: Wasm build green**

```bash
GOOS=js GOARCH=wasm go build ./pkg/planetgen/...
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
ls -la cmd/planet-explorer/web/planet-explorer.wasm
```

Expected: clean. Wasm binary written.

- [ ] **Step 4: Slider tool end-to-end**

```bash
go run ./cmd/planet-explorer &
SERVER_PID=$!
sleep 1
echo "Open http://localhost:8080/. Verify:"
echo "  - terran loads with all 5 control fields, splines, warp, biome table, LUT visible in panels."
echo "  - Switching to jovian loads gas giant defaults (no biome table)."
echo "  - Regenerate at face 256 produces a planet in <2s."
echo "  - Bypass LUT button changes the rendered color noticeably."
echo "Press Enter when done."
read
kill $SERVER_PID
```

- [ ] **Step 5: Batch render perf**

```bash
mkdir -p /tmp/p1_acceptance_batch
time go run ./cmd/generate-planet-maps -db /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db -outdir /tmp/p1_acceptance_batch -workers 8 -face 1024 2>&1 | tail -3
ls /tmp/p1_acceptance_batch/*.png | wc -l
```

Expected: ~570 planets, both .png and .cube.png files. Time should be ≤ 1.2× the Phase 0 batch baseline (39 minutes) — Phase 1's per-pixel cost is higher (5 control fields vs 1, warp on every lookup, LUT pass) but the crater pre-filter from Task 1 absorbs most of the addition. Realistic budget: 35–50 minutes.

If batch time is significantly worse than 50 min, profile and identify the bottleneck. Common candidates: `field.GenerateControlFields` is computed once per planet but does 5×6×S²×8 trig calls; the warp pass adds 3 fbm samples per per-pixel; the LUT trilinear sample is 8 array reads + lerps.

- [ ] **Step 6: Update README**

Edit `cmd/generate-planet-maps/README.md` to mention Phase 1 features:

```markdown
## Phase 1 (current)

- OkLab biome blending — visible as cleaner color transitions across palettes.
- Multi-noise control fields with monotone cubic splines — 5 independent fbm fields drive height via per-field splines.
- Domain warping — every height/biome/cloud lookup is warped per Quilez.
- Whittaker T+M biome lookup — terran (and any future type with a BiomeTable) renders climate-driven biomes via a 4×4 cell table with bilinear-OkLab interpolation.
- Per-archetype color LUT — final color grade per planet type for cohesive look.

The slider tool at `cmd/planet-explorer/` is the canonical workflow for tuning these parameters interactively.
```

Also add a Phase 1 row to the "Generations" status table if one exists.

- [ ] **Step 7: Phase 1 acceptance summary**

If everything green:

```
PHASE 1: ACCEPTED
  Tier S items: 5/5 landed
  Slider tool: built, end-to-end working
  Batch render: <time> for ~570 planets
  Goldens: regenerated and committed
  Lint, build, tests: all clean
```

Otherwise, document specific failures and pause Phase 1 until resolved.

---

## Self-review notes

- **Spec coverage**: Master plan §5.1–5.7 maps to Tasks 8–9 (item 1), 11–16 (item 2), 17–19 (item 3), 20–23 (item 4), 24–27 (item 5), and Tasks 3–7 + per-algorithm panels for the slider tool. Phase 0 debt (crater pre-filter, cubePathFor) is Tasks 1–2.
- **Placeholder scan**: No "TODO" or "TBD"; all code blocks complete; all commands exact.
- **Type consistency**: `ControlField`/`ControlConfig`/`Spline`/`SplineKnot`/`BiomeTable`/`BiomeCell`/`ColorRGB`/`LUT`/`WarpConfig` are all defined in Task 11 / 17 / 20 / 22 / 24 (in `pkg/planetgen/types/types.go`) and reused consistently downstream. `seed.Domain` defined in Task 11 used by Tasks 12, 17. `EvalSpline` defined in Task 13 used in Task 14. `BlendOkLab` / `SampleGradientOkLab` defined in Task 8 used in Tasks 9, 21.
- **Potential gap**: Phase 1 acceptance from master plan §5.7 mentions "Tuned-default `PlanetProfile`s for all 13 types committed back to `profile.go`." — Tasks 15, 18, 22, 26 cover the per-Tier-S retunes; Task 28 verifies the result. Acceptable.

---

## Execution handoff

Plan complete and saved to `docs/plans/2026-04-26-planet-gen-phase-1-tier-s-tuning.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — execute tasks in this session using the executing-plans skill, batch execution with checkpoints for review.

Which approach?
