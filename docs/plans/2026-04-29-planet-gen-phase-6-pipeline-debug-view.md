# Planet Generation Phase 6: Pipeline Debug View

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax for tracking. Phase 6 follows Phase 4 and precedes Phase 5 (per-pipeline visibility before the heaviest subtractive stage lands).

**Goal:** Build an inline debug panel at the bottom of the planet-explorer page that shows every heightmap-pipeline stage as a thumbnail row. Each row exposes: the raw layer contribution (or fbm), spline-input-band classification, spline-output-band classification (where applicable), and the running heightmap sum after that stage. Per-stage "skip in sum" toggles let the operator dry-run individual stages without zeroing their parameters.

**Architecture:** A new `DebugFrame` struct accumulates per-stage cube maps as the renderer walks the pipeline. The renderer accepts an optional `*DebugFrame` and an optional `DebugBypass` set; when non-nil, each stage's contribution gets cloned into the frame and any stage whose name is in the bypass set is dry-run (skipped without changing rng/seed consumption — important so the rest of the pipeline produces the same noise streams). The wasm bridge exposes a new `planetExplorerGenerateDebug` function returning a `{ stages: [{name, raw_png, ...}] }` JSON blob with base64-encoded equirect PNGs. The frontend renders a collapsible `<details>` panel of canvases plus per-stage bypass checkboxes.

**Tech Stack:** Existing `pkg/planetgen` Go primitives, `cubemap.BakeEquirect` for thumbnail rendering, `image/png` for in-Go encoding, base64 + JSON for wasm-to-JS transport, plain JS canvas rendering for the panel.

---

## Pre-flight notes

**Stage inventory.** As of Phase 4 the rocky heightmap pipeline runs:

| # | Stage | Has spline? | Contribution sign |
|---|---|---|---|
| 1 | Continentalness fbm + spline | yes | + |
| 2 | Detail fbm + spline | yes | + |
| 3 | PeaksValleys fbm + spline | yes | + |
| 4 | Temperature fbm + spline | yes | + (informational; doesn't affect rocky height in current pipeline) |
| 5 | Humidity fbm + spline | yes | + (informational; same) |
| 6 | Ridged multifractal | no (mask gates it) | + |
| 7 | Provinces (rampMod, freqMod) | no | × (multiplicative) |
| 8 | Continents (Fibonacci-Voronoi base) | no | max |
| 9 | Normalize | no | rescale |
| 10 | Coastal noise (Phase 4) | no | ± |
| 11 | Craters (bowl + rim) | no | − (bowl) / + (rim) |

Each row in the debug view corresponds to one of the above. Temperature/Humidity get full thumbnails too even though they don't change the heightmap, because Phase-3+ biome lookup does consume them and the operator wants to see them.

**Bypass semantics.** A bypassed stage is *skipped* but the renderer still consumes the same rng draws to keep downstream noise streams identical. Practically this means: for control fields, generate the fbm anyway and just don't add its spline output to the running sum; for ridged, generate the noise and skip the addition; for provinces, skip the mod multiplication; for continents, skip the max; for coastal, return base unchanged from `ApplyCoastal`; for craters, skip the bowl stamp.

**What "spline-input bands" and "spline-output bands" mean.**
- *Input bands*: each pixel's raw fbm value is a number in roughly `[0, 1]`. The spline has N knots, hence N−1 input intervals `[Knot[i].Input, Knot[i+1].Input)`. Color each pixel by which interval its fbm value lands in.
- *Output bands*: each pixel's spline output is a number in roughly `[0, max(Output)]`. Same N−1 intervals, but on the *output* axis: `[Knot[i].Output, Knot[i+1].Output)`. Color by which output interval the pixel produces. Useful for spotting where the spline's plateau regions show up on the planet.

A fixed 5-color palette per band index is sufficient (cyan / blue / green / yellow / red, low → high).

**"Subtractive in red."** Any layer that produces a negative contribution at any pixel renders that pixel in a red palette (intensity ∝ |value|). The two layers that satisfy this today are Craters (bowl interior) and Coastal (the `n4 + n5/2 + n6/4` term can go either direction). All other layers render in grayscale.

**Wasm size impact.** The debug renderer adds another `RenderRockyDebug` path that allocates ~10–15 cube-map clones at face size S=256 (default explorer face). At 4 bytes/float × 6 faces × 256² × 12 cube maps ≈ 18 MB. PNG encoding is the slow part (~1–2 s); make the debug regen explicitly opt-in (clicking "Update debug view") rather than firing on every regen.

---

## File structure

**New files:**

| Path | Role |
|---|---|
| `pkg/planetgen/render/debug.go` | `DebugFrame`, `DebugStage`, `DebugBypass` types + `RenderRockyDebug` |
| `pkg/planetgen/render/debug_test.go` | Stage-list correctness + bypass parity tests |
| `pkg/planetgen/render/debug_palette.go` | Spline-band color classifier + red-palette helper |
| `pkg/planetgen/render/debug_palette_test.go` | Classifier boundary tests |
| `cmd/planet-explorer/web/debug.css` | Debug-panel layout styles |

**Modified files:**

| Path | Reason |
|---|---|
| `pkg/planetgen/render/rocky.go` | Optional `*DebugFrame` parameter threaded through `generateRockyHeightmap` |
| `cmd/planet-explorer/wasm/main.go` | New `planetExplorerGenerateDebug` export |
| `cmd/planet-explorer/web/index.html` | Add `<details id="debug-panel">` block at the bottom |
| `cmd/planet-explorer/web/app.js` | Fetch debug frame on demand, render canvas grid + bypass toggles |

---

## Task 1: `DebugFrame` types + spline-band classifier

**Files:**
- Create: `pkg/planetgen/render/debug.go`
- Create: `pkg/planetgen/render/debug_palette.go`
- Create: `pkg/planetgen/render/debug_palette_test.go`

- [ ] **Step 1: Define `DebugFrame`**

```go
// pkg/planetgen/render/debug.go
package render

import (
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// DebugBypass is the set of stage names the debug renderer should
// dry-run. The renderer still consumes any rng/noise draws the
// stage would have made (so downstream noise streams stay stable),
// but skips the stage's contribution to the heightmap.
type DebugBypass map[string]bool

// DebugFrame collects per-stage cube maps so the slider tool can
// show the operator each step of the rocky pipeline. Populated by
// RenderRockyDebug.
type DebugFrame struct {
    Stages []DebugStage
}

// DebugStage is one row in the pipeline visualization. RawFbm is the
// per-stage scalar contribution (signed; negative cells render red in
// the UI). InputBands and OutputBands are nil for stages that don't
// have splines. SumAfter is the running heightmap after this stage.
type DebugStage struct {
    Name        string
    RawFbm      *cubemap.CubeMapF
    InputBands  *cubemap.CubeMap // RGBA, optional
    OutputBands *cubemap.CubeMap // RGBA, optional
    SumAfter    *cubemap.CubeMapF
    Skipped     bool
}
```

- [ ] **Step 2: Spline-band classifier**

```go
// pkg/planetgen/render/debug_palette.go
package render

import (
    "image/color"

    planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// bandPalette is the fixed five-color palette for spline-band
// classification. Index 0 is the lowest band; out-of-range pixels
// render black.
var bandPalette = [...]color.RGBA{
    {R: 70, G: 200, B: 220, A: 255},  // cyan
    {R: 60, G: 110, B: 230, A: 255},  // blue
    {R: 80, G: 200, B: 80, A: 255},   // green
    {R: 240, G: 220, B: 60, A: 255},  // yellow
    {R: 230, G: 80, B: 60, A: 255},   // red
}

// ClassifySplineInputBands returns a cube map whose pixels are colored
// by which input-axis knot interval the corresponding scalar value
// lands in. For a spline with N knots, intervals are
// [Knot[0].Input, Knot[1].Input), …, [Knot[N-2].Input, Knot[N-1].Input].
func ClassifySplineInputBands(values *cubemap.CubeMapF, spline planetcolor.Spline, S int) *cubemap.CubeMap {
    out := cubemap.New(S)
    knots := spline.Knots
    if len(knots) < 2 {
        return out
    }
    intervals := len(knots) - 1
    for face := range cubemap.Face(cubemap.NumFaces) {
        for py := range S {
            for px := range S {
                v := values.Get(face, px, py)
                idx := -1
                for i := 0; i < intervals; i++ {
                    if v >= knots[i].Input && v <= knots[i+1].Input {
                        idx = i
                        break
                    }
                }
                if idx < 0 {
                    out.Set(face, px, py, color.RGBA{A: 255})
                    continue
                }
                out.Set(face, px, py, bandPalette[idx%len(bandPalette)])
            }
        }
    }
    return out
}

// ClassifySplineOutputBands is the same as the input variant but
// classifies on the spline's output axis instead.
func ClassifySplineOutputBands(values *cubemap.CubeMapF, spline planetcolor.Spline, S int) *cubemap.CubeMap {
    out := cubemap.New(S)
    knots := spline.Knots
    if len(knots) < 2 {
        return out
    }
    intervals := len(knots) - 1
    // Output intervals: pair (knots[i].Output, knots[i+1].Output);
    // could be non-monotone in pathological cases. We classify by
    // which input interval the pixel evaluated through, which is
    // equivalent for monotone splines and well-defined otherwise.
    for face := range cubemap.Face(cubemap.NumFaces) {
        for py := range S {
            for px := range S {
                outVal := planetcolor.EvalSpline(spline, values.Get(face, px, py))
                idx := -1
                for i := 0; i < intervals; i++ {
                    lo, hi := knots[i].Output, knots[i+1].Output
                    if hi < lo {
                        lo, hi = hi, lo
                    }
                    if outVal >= lo && outVal <= hi {
                        idx = i
                        break
                    }
                }
                if idx < 0 {
                    out.Set(face, px, py, color.RGBA{A: 255})
                    continue
                }
                out.Set(face, px, py, bandPalette[idx%len(bandPalette)])
            }
        }
    }
    return out
}

// SignedToRGBA renders a signed scalar field with grayscale for
// non-negative values and a red-intensity ramp for negative values.
// hi is the absolute scaling factor (values divided by hi before
// mapping; clamped to [-1, 1]).
func SignedToRGBA(field *cubemap.CubeMapF, S int, hi float64) *cubemap.CubeMap {
    if hi <= 0 {
        hi = 1
    }
    out := cubemap.New(S)
    for face := range cubemap.Face(cubemap.NumFaces) {
        for py := range S {
            for px := range S {
                v := field.Get(face, px, py) / hi
                var c color.RGBA
                if v >= 0 {
                    if v > 1 {
                        v = 1
                    }
                    g := uint8(v * 255)
                    c = color.RGBA{R: g, G: g, B: g, A: 255}
                } else {
                    if v < -1 {
                        v = -1
                    }
                    r := uint8(-v * 255)
                    c = color.RGBA{R: r, G: 0, B: 0, A: 255}
                }
                out.Set(face, px, py, c)
            }
        }
    }
    return out
}
```

- [ ] **Step 3: Test the classifier**

```go
// pkg/planetgen/render/debug_palette_test.go
package render

import (
    "testing"

    planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestClassifyInputBandsBoundaries(t *testing.T) {
    sp := planetcolor.Spline{Knots: []planetcolor.SplineKnot{
        {Input: 0, Output: 0},
        {Input: 0.5, Output: 0.3},
        {Input: 1, Output: 0.6},
    }}
    field := cubemap.NewF(4)
    field.Set(cubemap.FacePosX, 0, 0, 0.25) // band 0
    field.Set(cubemap.FacePosX, 1, 0, 0.75) // band 1
    out := ClassifySplineInputBands(field, sp, 4)
    if out.Get(cubemap.FacePosX, 0, 0) == out.Get(cubemap.FacePosX, 1, 0) {
        t.Error("two bands should differ")
    }
}

func TestSignedToRGBARed(t *testing.T) {
    field := cubemap.NewF(4)
    field.Set(cubemap.FacePosX, 0, 0, -0.5)
    out := SignedToRGBA(field, 4, 1)
    px := out.Get(cubemap.FacePosX, 0, 0)
    if px.R == 0 {
        t.Error("negative pixel should produce red component")
    }
}
```

Run: `go test ./pkg/planetgen/render/ -run "TestClassify|TestSignedToRGBA" -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/planetgen/render/debug.go pkg/planetgen/render/debug_palette.go pkg/planetgen/render/debug_palette_test.go
git commit -m "P6 T1: DebugFrame types + spline-band classifier"
```

---

## Task 2: Refactor `generateRockyHeightmap` to populate `*DebugFrame`

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`

Add an optional `*DebugFrame` parameter and `DebugBypass` set. When the frame is non-nil, after each stage clone the relevant cube maps into the frame. When a stage's name is in the bypass set, skip its contribution while still consuming any rng draws.

- [ ] **Step 1: Change signature**

```go
// pkg/planetgen/render/rocky.go
func generateRockyHeightmap(profile *types.PlanetProfile, seed int64, S int) (*cubemap.CubeMapF, []feature.Crater) {
    return generateRockyHeightmapDebug(profile, seed, S, nil, nil)
}

func generateRockyHeightmapDebug(profile *types.PlanetProfile, seed int64, S int,
    frame *DebugFrame, bypass DebugBypass) (*cubemap.CubeMapF, []feature.Crater) {
    // existing body, with per-stage hooks
}
```

The existing public `RenderRocky` keeps the no-debug fast path; the new code uses the same body but checks for `frame != nil` at each stage.

- [ ] **Step 2: Add per-stage hooks**

For each control field (Continentalness, Detail, PeaksValleys, Temperature, Humidity):

```go
// inside the per-field loop, accumulate the field's fbm into a cube map
// raw[face][px][py] = fbm sample for this stage
// then, when frame != nil, clone raw + spline bands + sum-after into frame
isBypassed := bypass[name]
if !isBypassed {
    height += splineOutput
}
if frame != nil {
    frame.Stages = append(frame.Stages, DebugStage{
        Name:        name,
        RawFbm:      raw.Clone(),
        InputBands:  ClassifySplineInputBands(raw, fc.Spline, S),
        OutputBands: ClassifySplineOutputBands(raw, fc.Spline, S),
        SumAfter:    heightmap.Clone(),
        Skipped:     isBypassed,
    })
}
```

(Add `Clone()` to `cubemap.CubeMapF` and `cubemap.CubeMap` if missing — copy of `Faces`.)

Repeat the pattern for: Ridged, Provinces, Continents, Normalize, Coastal, Craters. For non-spline stages set `InputBands` and `OutputBands` to nil.

- [ ] **Step 3: Implement `Clone` if missing**

In `pkg/planetgen/cubemap`:

```go
func (c *CubeMapF) Clone() *CubeMapF {
    out := NewF(c.Size)
    for face := range Face(NumFaces) {
        copy(out.Faces[face], c.Faces[face])
    }
    return out
}

func (c *CubeMap) Clone() *CubeMap {
    out := New(c.Size)
    for face := range Face(NumFaces) {
        copy(out.Faces[face], c.Faces[face])
    }
    return out
}
```

- [ ] **Step 4: Tests**

```go
// pkg/planetgen/render/debug_test.go
package render

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestDebugFrameStageOrder(t *testing.T) {
    prof := types.PlanetProfile{
        Type:     "test",
        Renderer: "rocky",
        ControlConfig: types.ControlConfig{
            Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5,
                Spline: planetcolor.Spline{Knots: []planetcolor.SplineKnot{{0, 0}, {1, 0.5}}}},
        },
    }
    frame := &DebugFrame{}
    _, _ = generateRockyHeightmapDebug(&prof, 42, 32, frame, nil)
    if len(frame.Stages) == 0 {
        t.Fatal("expected at least one stage")
    }
    if frame.Stages[0].Name != "Continentalness" {
        t.Errorf("first stage should be Continentalness; got %s", frame.Stages[0].Name)
    }
}

func TestDebugFrameBypassParity(t *testing.T) {
    // With every stage bypassed, the heightmap should equal an empty
    // pre-normalize sum (all zeros pre-normalize → all 0 post-normalize
    // when min==max).
    prof := types.PlanetProfile{
        Type: "test", Renderer: "rocky",
        ControlConfig: types.ControlConfig{
            Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5,
                Spline: planetcolor.Spline{Knots: []planetcolor.SplineKnot{{0, 0}, {1, 0.5}}}},
        },
    }
    bypass := DebugBypass{"Continentalness": true}
    hm, _ := generateRockyHeightmapDebug(&prof, 42, 16, nil, bypass)
    for face := range hm.Faces {
        for _, v := range hm.Faces[face] {
            if v != 0 {
                t.Fatalf("with all stages bypassed, heightmap should be flat 0; got %f", v)
            }
        }
    }
}
```

Run: `go test ./pkg/planetgen/render/ -run TestDebugFrame -v`
Expected: PASS.

- [ ] **Step 5: Run all goldens (must stay green)**

```bash
go test -timeout 25m ./...
```

Expected: PASS — the public `RenderRocky` path is unchanged; only the internal helper signature grew.

- [ ] **Step 6: Commit**

```bash
git add pkg/planetgen/cubemap/*.go pkg/planetgen/render/rocky.go pkg/planetgen/render/debug_test.go
git commit -m "P6 T2: thread DebugFrame through generateRockyHeightmap"
```

---

## Task 3: `RenderRockyDebug` exported function

**Files:**
- Modify: `pkg/planetgen/render/debug.go`

Public entry point that runs the debug-aware pipeline and returns the populated `DebugFrame`. Used by the wasm bridge.

- [ ] **Step 1: Add the export**

```go
// pkg/planetgen/render/debug.go (append)

// RenderRockyDebug runs the rocky heightmap pipeline with intermediate
// stages captured into a DebugFrame. bypass is a set of stage names
// to dry-run; pass nil to run all stages normally.
func RenderRockyDebug(profile *types.PlanetProfile, seed int64, S int, bypass DebugBypass) *DebugFrame {
    frame := &DebugFrame{}
    _, _ = generateRockyHeightmapDebug(profile, seed, S, frame, bypass)
    return frame
}
```

- [ ] **Step 2: Test**

```go
// pkg/planetgen/render/debug_test.go (append)

func TestRenderRockyDebugAllStagesPresent(t *testing.T) {
    prof := types.PlanetProfile{
        Type: "test", Renderer: "rocky",
        ControlConfig: types.ControlConfig{
            Continentalness: types.ControlField{Amp: 1, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5,
                Spline: planetcolor.Spline{Knots: []planetcolor.SplineKnot{{0, 0}, {1, 0.5}}}},
            Detail: types.ControlField{Amp: 0.5, Freq: 2, Octaves: 3, Lacunarity: 2, Persistence: 0.5,
                Spline: planetcolor.Spline{Knots: []planetcolor.SplineKnot{{0, 0}, {1, 0.3}}}},
        },
        Ridged: types.RidgedConfig{Amp: 0.2, Freq: 2, Octaves: 3, Lacunarity: 2, Gain: 0.5, Offset: 1, MaskLow: 0.3, MaskHigh: 0.7},
    }
    frame := RenderRockyDebug(&prof, 7, 32, nil)
    names := map[string]bool{}
    for _, s := range frame.Stages {
        names[s.Name] = true
    }
    for _, want := range []string{"Continentalness", "Detail", "PeaksValleys", "Ridged", "Normalize"} {
        if !names[want] {
            t.Errorf("missing stage %q", want)
        }
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add pkg/planetgen/render/debug.go pkg/planetgen/render/debug_test.go
git commit -m "P6 T3: RenderRockyDebug exported entry point"
```

---

## Task 4: Wasm export + PNG serialization

**Files:**
- Modify: `cmd/planet-explorer/wasm/main.go`

New export `planetExplorerGenerateDebug(profileJSON, seedString, faceSize, bypassJSON)` returning a JSON payload of base64-encoded equirect PNG thumbnails.

- [ ] **Step 1: Add the export**

```go
// cmd/planet-explorer/wasm/main.go

func generateDebug(this js.Value, args []js.Value) any {
    if len(args) < 4 {
        return js.ValueOf(map[string]any{"error": "want 4 args"})
    }
    var prof types.PlanetProfile
    if err := json.Unmarshal([]byte(args[0].String()), &prof); err != nil {
        return js.ValueOf(map[string]any{"error": "profile parse: " + err.Error()})
    }
    s := planetgen.HashSeed(args[1].String())
    face := args[2].Int()
    var bypass render.DebugBypass
    if args[3].Type() == js.TypeString && args[3].String() != "" {
        var arr []string
        if err := json.Unmarshal([]byte(args[3].String()), &arr); err == nil {
            bypass = make(render.DebugBypass, len(arr))
            for _, name := range arr {
                bypass[name] = true
            }
        }
    }
    frame := render.RenderRockyDebug(&prof, s, face, bypass)
    eqW, eqH := face*2, face

    encode := func(cm *cubemap.CubeMap) string {
        if cm == nil {
            return ""
        }
        img := cubemap.BakeEquirectFromRGBA(cm, eqW, eqH)
        buf := bytes.Buffer{}
        _ = png.Encode(&buf, img)
        return base64.StdEncoding.EncodeToString(buf.Bytes())
    }
    encodeF := func(cmF *cubemap.CubeMapF, signed bool) string {
        if cmF == nil {
            return ""
        }
        var cm *cubemap.CubeMap
        if signed {
            cm = render.SignedToRGBA(cmF, cmF.Size, 1.0)
        } else {
            cm = cubemap.GrayscaleFromF(cmF)
        }
        return encode(cm)
    }

    stages := make([]map[string]any, 0, len(frame.Stages))
    for _, s := range frame.Stages {
        stages = append(stages, map[string]any{
            "name":         s.Name,
            "skipped":      s.Skipped,
            "raw":          encodeF(s.RawFbm, true),
            "input_bands":  encode(s.InputBands),
            "output_bands": encode(s.OutputBands),
            "sum_after":    encodeF(s.SumAfter, false),
        })
    }
    out, _ := json.Marshal(map[string]any{"stages": stages})
    return js.ValueOf(string(out))
}

// in main()
js.Global().Set("planetExplorerGenerateDebug", js.FuncOf(generateDebug))
```

- [ ] **Step 2: Add `cubemap.GrayscaleFromF` + `BakeEquirectFromRGBA`**

In `pkg/planetgen/cubemap/`:

```go
// GrayscaleFromF returns a CubeMap rendering of cmF with each pixel's
// scalar mapped to grayscale via clamp(0,1).
func GrayscaleFromF(cmF *CubeMapF) *CubeMap {
    out := New(cmF.Size)
    for face := range Face(NumFaces) {
        for i := range cmF.Faces[face] {
            v := cmF.Faces[face][i]
            if v < 0 {
                v = 0
            }
            if v > 1 {
                v = 1
            }
            g := uint8(v * 255)
            // CubeMap.Faces is RGBA bytes
            out.Faces[face][i*4] = g
            out.Faces[face][i*4+1] = g
            out.Faces[face][i*4+2] = g
            out.Faces[face][i*4+3] = 255
        }
    }
    return out
}
```

`BakeEquirectFromRGBA` is `BakeEquirect` if it already exists; if not, add it as a thin wrapper that takes a `*CubeMap` rather than `*CubeMapF`.

- [ ] **Step 3: Rebuild wasm**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
```

Verify the binary grows a couple hundred kilobytes (encoder + base64 + new types).

- [ ] **Step 4: Commit**

```bash
git add cmd/planet-explorer/wasm/main.go pkg/planetgen/cubemap/*.go
git commit -m "P6 T4: wasm export for debug frame with PNG-encoded thumbnails"
```

---

## Task 5: HTML + CSS for the debug panel

**Files:**
- Modify: `cmd/planet-explorer/web/index.html`
- Create: `cmd/planet-explorer/web/debug.css`

- [ ] **Step 1: Add the panel container**

At the bottom of `<body>` in `index.html`, before the closing tag and any other footer content:

```html
<details id="debug-panel" class="debug-panel">
  <summary>Pipeline debug view <small>(click to expand)</small></summary>
  <div class="debug-controls">
    <button id="debug-refresh">Update debug view</button>
    <span class="debug-hint">Half-size equirects per stage. Bypass toggles dry-run a stage without zeroing values.</span>
  </div>
  <div id="debug-grid"></div>
</details>
<link rel="stylesheet" href="debug.css">
```

- [ ] **Step 2: CSS**

```css
/* cmd/planet-explorer/web/debug.css */
.debug-panel {
  border: 1px solid #444;
  margin: 24px 0;
  padding: 8px 12px;
  background: #1a1a1a;
  color: #ddd;
}
.debug-panel summary { cursor: pointer; padding: 4px 0; font-weight: 600; }
.debug-controls { margin: 8px 0; display: flex; align-items: center; gap: 12px; }
.debug-hint { color: #888; font-size: 0.9em; }
#debug-grid { display: flex; flex-direction: column; gap: 16px; margin-top: 12px; }
.debug-row {
  display: grid;
  grid-template-columns: 140px repeat(4, 1fr);
  gap: 8px;
  align-items: center;
  padding: 8px;
  background: #222;
  border: 1px solid #333;
}
.debug-row.skipped { opacity: 0.45; }
.debug-row .label { font-weight: 600; }
.debug-row canvas { width: 100%; max-width: 200px; height: auto; border: 1px solid #333; }
.debug-row .bypass-toggle { display: flex; align-items: center; gap: 4px; font-size: 0.85em; }
.debug-row .placeholder { color: #555; font-style: italic; text-align: center; }
```

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/index.html cmd/planet-explorer/web/debug.css
git commit -m "P6 T5: HTML + CSS scaffolding for debug panel"
```

---

## Task 6: JS — fetch frame, render thumbnails, bypass toggles

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Add the debug-render entry point**

```js
// cmd/planet-explorer/web/app.js (append; module-scope state for bypass set)
const debugBypass = new Set();

function refreshDebugView() {
  if (!window.planetExplorerGenerateDebug) {
    console.warn('debug API not available; rebuild wasm');
    return;
  }
  const profileJSON = JSON.stringify(profile);
  const bypassJSON = JSON.stringify([...debugBypass]);
  const result = window.planetExplorerGenerateDebug(profileJSON, currentSeed, currentFaceSize, bypassJSON);
  let parsed;
  try { parsed = JSON.parse(result); }
  catch (e) { console.error('debug parse', e); return; }
  if (parsed.error) { console.error(parsed.error); return; }
  renderDebugGrid(parsed.stages);
}

function renderDebugGrid(stages) {
  const grid = document.getElementById('debug-grid');
  grid.innerHTML = '';
  for (const s of stages) {
    const row = document.createElement('div');
    row.className = 'debug-row' + (s.skipped ? ' skipped' : '');

    const label = document.createElement('div');
    label.className = 'label';
    label.appendChild(document.createTextNode(s.name));
    const toggle = document.createElement('label');
    toggle.className = 'bypass-toggle';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = debugBypass.has(s.name);
    cb.addEventListener('change', () => {
      if (cb.checked) debugBypass.add(s.name); else debugBypass.delete(s.name);
      refreshDebugView();
    });
    toggle.appendChild(cb);
    toggle.appendChild(document.createTextNode(' bypass'));
    label.appendChild(toggle);
    row.appendChild(label);

    for (const key of ['raw', 'input_bands', 'output_bands', 'sum_after']) {
      if (!s[key]) {
        const ph = document.createElement('div');
        ph.className = 'placeholder';
        ph.textContent = '—';
        row.appendChild(ph);
        continue;
      }
      const img = new Image();
      img.src = 'data:image/png;base64,' + s[key];
      const cv = document.createElement('canvas');
      img.onload = () => {
        cv.width = img.width;
        cv.height = img.height;
        cv.getContext('2d').drawImage(img, 0, 0);
      };
      row.appendChild(cv);
    }
    grid.appendChild(row);
  }
}

document.getElementById('debug-refresh').addEventListener('click', refreshDebugView);
```

- [ ] **Step 2: Smoke-test**

Rebuild wasm if you didn't already, run `go run ./cmd/planet-explorer/`, open the browser, expand the panel, click "Update debug view". Verify rows appear for each stage with thumbnails. Toggle a bypass on Continentalness and re-update — thumbnails should re-render and the row should fade.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "P6 T6: debug panel JS — fetch frame, render thumbnails, bypass toggles"
```

---

## Task 7: Auto-refresh policy + acceptance

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`
- Modify: `cmd/generate-planet-maps/README.md`

- [ ] **Step 1: Auto-refresh on regen (optional)**

If the panel is open, refresh debug after each main-render commit:

```js
// in commitProfile or wherever the main render is invoked
if (document.getElementById('debug-panel').open) {
  refreshDebugView();
}
```

The `<details open>` check skips the expensive debug pass when collapsed.

- [ ] **Step 2: README**

In `cmd/generate-planet-maps/README.md`, add a Phase-6 section:

```markdown
## Phase 6 — Pipeline debug view

The slider tool's debug panel (collapsible at the bottom of the page)
shows each rocky-pipeline stage as a row of half-size equirects:

- **raw** — the stage's signed scalar contribution. Negative pixels
  render in red.
- **input bands / output bands** — for stages with splines, pixels
  classified by which knot interval they fall into (input axis or
  output axis).
- **sum after** — the running heightmap after this stage applied.

Each row also has a "bypass" checkbox. Bypassing a stage skips its
contribution while keeping the rest of the noise pipeline unchanged
(seeds and rng draws stay stable), so the operator can isolate the
effect of any single stage without zeroing parameters.

Implemented under `pkg/planetgen/render/debug.go`; surfaced via the
`planetExplorerGenerateDebug` wasm export.
```

- [ ] **Step 3: Phase 6 acceptance commit**

```bash
go test -timeout 25m ./...
golangci-lint run
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm

git commit --allow-empty -m "Phase 6 (pipeline debug view) accepted"
```

Body should list T1–T7 status, gates, and which Tier-A items remain (item 9 erosion → Phase 5).

---

## Self-review notes

**Spec coverage.** User asked for: per-layer fbm thumbnails, spline input-band view, spline output-band view, running sum after each stage, half-size equirects, red palette for subtractive contributions, per-stage bypass toggles, "lots of thumbnails for now". All covered: T1 (palette + classifier), T2 (per-stage hooks in renderer), T3 (public entry), T4 (wasm + PNG transport), T5 (HTML/CSS), T6 (JS rendering + toggles), T7 (auto-refresh + README).

**Placeholder scan.** No "TODO" or "TBD". The Cassini ramp asset stand-in lives in Phase 4 T8, not here. Bypass semantics are explicitly documented in pre-flight notes.

**Type consistency.** `DebugFrame`, `DebugStage`, `DebugBypass` defined in T1 and used through T2/T3/T4. `ClassifySplineInputBands` / `OutputBands` / `SignedToRGBA` defined in T1, consumed by T2 (renderer hooks) and T4 (wasm encoding). `Clone` added to `cubemap.CubeMapF` and `cubemap.CubeMap` in T2. `GrayscaleFromF` added in T4.

**Performance.** Debug regen at face=256 allocates ~12 cube-map clones (~18 MB) and PNG-encodes ~36 thumbnails. Estimated 1–3 s wall time per refresh, dominated by PNG encoding. Auto-refresh is gated on the panel being open (T7 step 1). Manual button is the canonical refresh path.

**Phase-4 layer compatibility.** Coastal and Continents both run after the control-fields stage but modify the heightmap in place / via max-merge. The renderer hooks need to capture (heightmap before, contribution-as-delta, heightmap after) for these layers. Practical approach: clone heightmap before the stage, then `delta = heightmap_after - heightmap_before` produces the "raw" thumbnail. This handles signed contributions correctly (e.g. Coastal's `n4 + n5/2 + n6/4` term).

**Phase-5 forward compat.** When particle erosion lands in Phase 5, it slots in as another stage with a strongly-subtractive contribution; the existing red-palette renderer handles it without changes. The bypass toggle for erosion will be especially valuable since erosion is the slowest pass.

**Untested but plausible failure modes.** (a) PNG encoding at S=512 may exceed wasm memory limits — mitigation: cap debug face size to 256, log a warning if face > 256. (b) Older browsers may rate-limit large data-URLs — render path uses `Image.onload` so this should be fine. (c) Spline output bands with non-monotone splines fall through to the lo/hi-swap branch in `ClassifySplineOutputBands`; classification will misorder visually but still produce well-defined colors.
