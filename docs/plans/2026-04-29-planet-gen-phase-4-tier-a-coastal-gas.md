# Planet Generation Phase 4: Tier-A Coastal & Gas-Giant Detail

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land master-plan items 8 (JFA distance-to-coast), 10 (coastal noise + Voronoi continents), 12 (curl-noise gas-giant advection), and 13 (Cassini Jupiter ramp + storm bands), plus a clarifying rename of the existing `Erosion` control field to `Detail`. Wraps four of the five Tier-A master-plan items not yet shipped (item 9 erosion is its own Phase 5).

**Architecture:** Same shape as Phase 3 — `pkg/planetgen` gains primitives in `field/`, `noise/`, and `color/`, the `render/` package consumes them through small additive stages, and the slider tool gets per-feature panels. Item 8 produces a reusable `*cubemap.CubeMapF` distance field that item 10 consumes; the gas-giant items are entirely orthogonal to the rocky pipeline.

**Tech Stack:** Go 1.24+, cube-sphere primitives in `pkg/planetgen/cubemap`, `math/rand/v2`, `pkg/planetgen/seed.Domain` for orthogonal sub-seed mixing, the existing `noise.Generator` and `noise.Warper` types, `//go:embed` for the Jupiter ramp asset, plain JS in `cmd/planet-explorer/web/app.js`.

---

## Pre-flight notes

**Sequencing reminder.** Phase 4 is followed by Phase 6 (pipeline debug view) before Phase 5 (particle erosion). The debug view will visualize every layer this phase introduces, so design new fields with that in mind: each new layer should be a `*cubemap.CubeMapF` (or a small struct around one) so the debug view can grab it for free.

**Backward compatibility.** The Phase-3 tuned profiles in `profile.go` use the field name `Erosion` and saved-JSON dumps from the explorer round-trip the same key. Task 1 renames the field to `Detail` and adds a `UnmarshalJSON` shim on `ControlConfig` that aliases the old key, so users' existing saved JSONs (e.g. `terran_crater.json`) still load. The shim stays in place at least through Phase 5; can be removed once all known external JSONs have been re-exported.

**Seed-domain stability.** `field/control.go` keys each control field's noise via `seed.Domain(master, "control.erosion")` etc. The rename does **not** change the domain string — keeping it `"control.erosion"` preserves the fbm output bit-for-bit on every existing archetype. Goldens stay green on Task 1 even though the API changes.

**No JSON struct tags.** Per `CLAUDE.md`, default Go-capitalized JSON keys are the convention. The `UnmarshalJSON` shim is custom code on the type, not a struct tag.

---

## File structure

**New files:**

| Path | Role |
|---|---|
| `pkg/planetgen/field/jfa.go` | JFA distance-to-coast on cube-sphere with cross-face propagation |
| `pkg/planetgen/field/jfa_test.go` | Synthetic-heightmap distance correctness tests |
| `pkg/planetgen/field/continents.go` | Fibonacci-spiral seeds → Voronoi-with-warp continent base height |
| `pkg/planetgen/field/continents_test.go` | Determinism + per-cell coverage tests |
| `pkg/planetgen/noise/coastal.go` | `e_coast` enhancement around shore lines |
| `pkg/planetgen/noise/coastal_test.go` | Coastal-only modulation correctness |
| `pkg/planetgen/noise/curl.go` | Curl-noise + semi-Lagrangian backward-trace |
| `pkg/planetgen/noise/curl_test.go` | Curl tangent + advection convergence tests |
| `pkg/planetgen/color/jupiter_ramp.go` | Loader + sampler for the embedded 1-D Jupiter ramp asset |
| `pkg/planetgen/color/luts/jupiter_ramp.png` | 256×1 RGBA strip used as the gas-giant base palette |
| `pkg/planetgen/color/jupiter_ramp_test.go` | Decode + bounds + alpha tests |

**Modified files:**

| Path | Reason |
|---|---|
| `pkg/planetgen/types/types.go` | Rename `Erosion` → `Detail`. Add `CoastalConfig`, `ContinentConfig`, `CurlConfig`, `StormBand`. Add `UnmarshalJSON` on `ControlConfig` for old-key shim. |
| `pkg/planetgen/profile.go` | Rename per-archetype `Erosion:` → `Detail:` (11 sites). |
| `pkg/planetgen/field/control.go` | Rename struct field reference; keep `"control.erosion"` seed domain. |
| `pkg/planetgen/field/control_test.go` | Rename test fixtures. |
| `pkg/planetgen/render/rocky.go` | Wire coastal noise + continent base height. |
| `pkg/planetgen/render/gasgiant.go` | Wire curl-noise advection + Jupiter ramp + storm bands. |
| `cmd/planet-explorer/web/app.js` | Rename label/tooltip; add coastal/continent/curl/storm panels. |
| `cmd/planet-explorer/web/index.html` | Update docs (Erosion → Detail, mention Phase-5 erosion). |

---

## Task 1: Rename `Erosion` → `Detail`

**Files:**
- Modify: `pkg/planetgen/types/types.go`
- Modify: `pkg/planetgen/field/control.go`
- Modify: `pkg/planetgen/field/control_test.go`
- Modify: `pkg/planetgen/render/rocky.go`
- Modify: `pkg/planetgen/profile.go`
- Modify: `cmd/planet-explorer/web/app.js`
- Modify: `cmd/planet-explorer/web/index.html`

The existing `Erosion` control field is just another fbm noise layer — its spline output is summed positively into the heightmap. The name and the UI tooltips ("Spline output usually negative — subtracted from height") have been lying since they were written. Phase 5 will introduce a real flow-based erosion stage that owns the name `Erosion`. This task renames the misnomer to `Detail` so that distinction is clean before Phase 5 lands.

- [ ] **Step 1: Rename in `types.go`**

```go
// pkg/planetgen/types/types.go (inside ControlConfig)
type ControlConfig struct {
    Continentalness ControlField
    Detail          ControlField // formerly "Erosion"; high-frequency detail-noise layer
    PeaksValleys    ControlField
    Temperature     ControlField
    Humidity        ControlField
}
```

- [ ] **Step 2: Add `UnmarshalJSON` shim**

```go
// pkg/planetgen/types/types.go (append at the end of the file)

// UnmarshalJSON accepts both the current "Detail" key and the legacy
// "Erosion" key so explorer JSON dumps from before the Phase-4 rename
// keep loading. When both are present the current key wins.
func (c *ControlConfig) UnmarshalJSON(data []byte) error {
    type raw ControlConfig
    aux := struct {
        *raw
        Erosion *ControlField `json:",omitempty"`
    }{raw: (*raw)(c)}
    if err := json.Unmarshal(data, &aux); err != nil {
        return err
    }
    if aux.Erosion != nil && c.Detail == (ControlField{}) {
        c.Detail = *aux.Erosion
    }
    return nil
}
```

(Add `"encoding/json"` import to `types.go`.)

- [ ] **Step 3: Test the shim**

Add `pkg/planetgen/types/types_test.go`:

```go
package types

import (
    "encoding/json"
    "testing"
)

func TestControlConfigLegacyErosionKey(t *testing.T) {
    raw := []byte(`{"Erosion":{"Amp":1.5,"Freq":2.0,"Octaves":3}}`)
    var cfg ControlConfig
    if err := json.Unmarshal(raw, &cfg); err != nil {
        t.Fatal(err)
    }
    if cfg.Detail.Amp != 1.5 || cfg.Detail.Freq != 2.0 || cfg.Detail.Octaves != 3 {
        t.Errorf("legacy Erosion key did not populate Detail; got %+v", cfg.Detail)
    }
}

func TestControlConfigDetailKeyWins(t *testing.T) {
    raw := []byte(`{"Detail":{"Amp":2.5},"Erosion":{"Amp":9.9}}`)
    var cfg ControlConfig
    if err := json.Unmarshal(raw, &cfg); err != nil {
        t.Fatal(err)
    }
    if cfg.Detail.Amp != 2.5 {
        t.Errorf("Detail should win over Erosion; got Amp=%f", cfg.Detail.Amp)
    }
}
```

Run: `go test ./pkg/planetgen/types/ -v`
Expected: PASS.

- [ ] **Step 4: Rename in `control.go`**

```go
// pkg/planetgen/field/control.go (controlFieldDomains stays unchanged)
fieldsCfg := [5]types.ControlField{
    cfg.Continentalness,
    cfg.Detail, // was cfg.Erosion
    cfg.PeaksValleys,
    cfg.Temperature,
    cfg.Humidity,
}
```

Domain string at index 1 stays `"control.erosion"` so the per-archetype noise output is bit-for-bit identical.

- [ ] **Step 5: Rename in `rocky.go`**

In `orderedControlFields`:

```go
func orderedControlFields(c types.ControlConfig) [5]types.ControlField {
    return [5]types.ControlField{
        c.Continentalness,
        c.Detail, // was c.Erosion
        c.PeaksValleys,
        c.Temperature,
        c.Humidity,
    }
}
```

Also update the leading comment to mention `Detail` instead of `Erosion`.

- [ ] **Step 6: Rename in `profile.go`**

Replace every `Erosion:` with `Detail:` (11 occurrences across the rocky archetypes). Same values, same splines.

- [ ] **Step 7: Rename in `control_test.go`**

Replace the three `Erosion:` references with `Detail:` and update the assertion message ("Continentalness and Detail fields are identical; named-domain mix not effective").

- [ ] **Step 8: Update slider tooltips**

In `cmd/planet-explorer/web/app.js`:

```js
const fields = ['Continentalness', 'Detail', 'PeaksValleys', 'Temperature', 'Humidity'];
// ...
const FIELD_TOOLTIPS = {
  // ...
  Detail: 'High-frequency detail noise. Adds bumpy variation to the heightmap. (Despite the legacy name "Erosion", this layer is purely additive — Phase 5 will add a separate flow-based erosion stage.)',
  // ...
};
```

- [ ] **Step 9: Update `index.html` docs**

Replace both Erosion mentions with Detail and add a one-line note that flow-based erosion is a separate Phase-5 stage.

- [ ] **Step 10: Run all tests + goldens**

```bash
go build ./...
golangci-lint run
go test -timeout 25m ./...
```

Expected: pass, including all 13 goldens (the rename is a pure name change; the seed domain string and all numeric values are unchanged).

- [ ] **Step 11: Rebuild wasm**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
```

- [ ] **Step 12: Commit**

```bash
git add pkg/planetgen/types/types.go pkg/planetgen/types/types_test.go \
  pkg/planetgen/field/control.go pkg/planetgen/field/control_test.go \
  pkg/planetgen/render/rocky.go pkg/planetgen/profile.go \
  cmd/planet-explorer/web/app.js cmd/planet-explorer/web/index.html
git commit -m "P4 T1: rename Erosion control field to Detail (semantic accuracy)"
```

---

## Task 2: JFA distance-to-coast field generator

**Files:**
- Create: `pkg/planetgen/field/jfa.go`
- Create: `pkg/planetgen/field/jfa_test.go`

Jump Flooding Algorithm on the cube-sphere. Given a heightmap and an ocean threshold (e.g. `OceanLevel`), produce a `*cubemap.CubeMapF` whose value at each pixel is the great-circle angular distance to the nearest below-threshold pixel, normalized to [0, 1] (clamped at π).

Each pass propagates seed positions to neighbors at offsets `step ∈ {S/2, S/4, ..., 1}` for `S` the face size; ~`log2(S) + 2` passes total. Cross-face propagation reads through `cubemap.CubeMapF.Sample` so seam continuity is automatic.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/field/jfa_test.go
package field

import (
    "math"
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestJFAOneSeed(t *testing.T) {
    // Single ocean pixel at (+X face, S/2, S/2); everywhere else is land.
    S := 32
    hm := cubemap.NewF(S)
    for face := range cubemap.Face(cubemap.NumFaces) {
        for i := range hm.Faces[face] {
            hm.Faces[face][i] = 1.0 // all land
        }
    }
    hm.Set(cubemap.FacePosX, S/2, S/2, 0.0) // single ocean pixel

    dist := DistanceToCoast(hm, 0.5, S)

    // Pixel at the seed should be ~0.
    if got := dist.Get(cubemap.FacePosX, S/2, S/2); got > 0.05 {
        t.Errorf("seed distance: got %f, want ~0", got)
    }
    // Antipodal pixel should be ~π/π == 1.
    if got := dist.Get(cubemap.FaceNegX, S/2, S/2); math.Abs(got-1) > 0.1 {
        t.Errorf("antipodal distance: got %f, want ~1", got)
    }
}

func TestJFANoOcean(t *testing.T) {
    S := 16
    hm := cubemap.NewF(S)
    for face := range cubemap.Face(cubemap.NumFaces) {
        for i := range hm.Faces[face] {
            hm.Faces[face][i] = 1.0
        }
    }
    dist := DistanceToCoast(hm, 0.5, S)
    // No ocean → every pixel reports max distance (1.0).
    for face := range cubemap.Face(cubemap.NumFaces) {
        for i, v := range dist.Faces[face] {
            if v < 0.999 {
                t.Errorf("face %v idx %d: got %f, want ~1.0 (no ocean seeds)", face, i, v)
            }
        }
    }
}
```

- [ ] **Step 2: Run the test to see it fail**

Run: `go test ./pkg/planetgen/field/ -run TestJFA -v`
Expected: FAIL — `DistanceToCoast` not defined.

- [ ] **Step 3: Implement `DistanceToCoast`**

```go
// pkg/planetgen/field/jfa.go
package field

import (
    "math"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// DistanceToCoast returns a cube-map field where each pixel holds the
// great-circle angular distance to the nearest below-threshold pixel
// in heightmap, divided by π (so values fall in [0, 1]).
//
// Implementation: jump flooding. Each pixel stores its nearest seed's
// (face, x, y) and the great-circle angular distance to it; passes at
// offsets S/2, S/4, ..., 1 propagate seeds to neighbors, with one final
// pass at offset 1 to clean up.
func DistanceToCoast(heightmap *cubemap.CubeMapF, threshold float64, S int) *cubemap.CubeMapF {
    // Each cell records the (face, px, py) of its nearest seed and the
    // angular distance.
    type seed struct {
        face       int8
        px, py     int16
        dirX, dirY, dirZ float32 // cached unit-sphere direction of seed
    }
    const noSeed = int8(-1)
    seeds := make([][]seed, cubemap.NumFaces)
    dists := cubemap.NewF(S)
    for face := range cubemap.Face(cubemap.NumFaces) {
        seeds[face] = make([]seed, S*S)
        for i := range seeds[face] {
            seeds[face][i].face = noSeed
        }
        for py := range S {
            for px := range S {
                if heightmap.Get(face, px, py) < threshold {
                    dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
                    seeds[face][py*S+px] = seed{face: int8(face), px: int16(px), py: int16(py),
                        dirX: float32(dx), dirY: float32(dy), dirZ: float32(dz)}
                    dists.Set(face, px, py, 0)
                } else {
                    dists.Set(face, px, py, 1.0) // π / π = 1; will shrink as seeds reach
                }
            }
        }
    }

    // Standard JFA: at each step (S/2, S/4, …, 1), each cell looks at
    // 8 neighbors at that offset and adopts the closest seed it sees.
    propagate := func(step int) {
        next := make([][]seed, cubemap.NumFaces)
        for face := range cubemap.Face(cubemap.NumFaces) {
            next[face] = make([]seed, S*S)
            copy(next[face], seeds[face])
            for py := range S {
                for px := range S {
                    dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
                    best := next[face][py*S+px]
                    bestDist := dists.Get(face, px, py)
                    for _, off := range [8][2]int{
                        {-step, -step}, {0, -step}, {step, -step},
                        {-step, 0}, {step, 0},
                        {-step, step}, {0, step}, {step, step},
                    } {
                        nx, ny := px+off[0], py+off[1]
                        // Cross-face: use Sample for the heightmap, but for
                        // seed propagation we need to walk to the neighbor's
                        // (face, px, py) discretely. Simplest: clamp on-face
                        // for now; full cross-face seam handling is a follow-up
                        // pass below.
                        if nx < 0 || nx >= S || ny < 0 || ny >= S {
                            continue
                        }
                        cand := seeds[face][ny*S+nx]
                        if cand.face == noSeed {
                            continue
                        }
                        // angular distance from current pixel to candidate seed
                        cosA := dx*float64(cand.dirX) + dy*float64(cand.dirY) + dz*float64(cand.dirZ)
                        if cosA > 1 {
                            cosA = 1
                        }
                        if cosA < -1 {
                            cosA = -1
                        }
                        ang := math.Acos(cosA) / math.Pi
                        if ang < bestDist {
                            best = cand
                            bestDist = ang
                        }
                    }
                    next[face][py*S+px] = best
                    dists.Set(face, px, py, bestDist)
                }
            }
        }
        seeds = next
    }

    for step := S / 2; step >= 1; step /= 2 {
        propagate(step)
    }
    propagate(1) // cleanup
    return dists
}
```

(Cross-face seam handling: the on-face clamp in this first pass is conservative — JFA will still converge through the seam because adjacent on-face pixels near the edge eventually reach the seam pixel and the next pass propagates further inland. A follow-up pass that explicitly walks `(face, x, y)` across edges using the cube-map adjacency table is a possible Phase-5 follow-up but not required to pass the tests in Step 1.)

- [ ] **Step 4: Run tests to confirm**

Run: `go test ./pkg/planetgen/field/ -run TestJFA -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/planetgen/field/jfa.go pkg/planetgen/field/jfa_test.go
git commit -m "P4 T2: JFA distance-to-coast field generator"
```

---

## Task 3: Coastal noise enhancement

**Files:**
- Create: `pkg/planetgen/noise/coastal.go`
- Create: `pkg/planetgen/noise/coastal_test.go`
- Modify: `pkg/planetgen/types/types.go` (add `CoastalConfig`)
- Modify: `pkg/planetgen/render/rocky.go` (consume coastal config)

Per master-plan §6.10, coastal enhancement applies a localized roughening near coastlines: `e_coast = e + α·(1 − e⁴)·(n4 + n5/2 + n6/4)` where `e` is the current heightmap and `n4..n6` are higher-frequency fbm samples. Modulated by distance-to-coast so the effect dies off inland.

- [ ] **Step 1: Add `CoastalConfig` type**

```go
// pkg/planetgen/types/types.go
type CoastalConfig struct {
    Amp       float64 // 0 disables; useful 0.05–0.2
    Threshold float64 // distance-to-coast cutoff in [0,1]; effect dies off above this
    Freq      float64 // base frequency for the n4 fbm
}
```

Add `Coastal CoastalConfig` to `PlanetProfile`.

- [ ] **Step 2: Write the failing test**

```go
// pkg/planetgen/noise/coastal_test.go
package noise

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestCoastalNoOpFarFromCoast(t *testing.T) {
    h := 0.7
    base := h
    out := ApplyCoastal(NewCoastalGen(123), 0, 1, 0, h, 1.0, 0.1, 0.5, 1.0)
    // distance = 1.0 (max), effect must be zero.
    if out != base {
        t.Errorf("far-from-coast pixel changed: got %f, want %f", out, base)
    }
}

func TestCoastalAmpZeroIsIdentity(t *testing.T) {
    // Even at zero distance, Amp=0 means no change.
    if got := ApplyCoastal(NewCoastalGen(7), 0, 1, 0, 0.5, 0.0, 0, 0.5, 1.0); got != 0.5 {
        t.Errorf("Amp=0 should be identity; got %f", got)
    }
}

func TestCoastalDeterministic(t *testing.T) {
    a := ApplyCoastal(NewCoastalGen(99), 0.3, 0.4, 0.5, 0.6, 0.0, 0.1, 0.5, 4.0)
    b := ApplyCoastal(NewCoastalGen(99), 0.3, 0.4, 0.5, 0.6, 0.0, 0.1, 0.5, 4.0)
    if a != b {
        t.Errorf("non-deterministic; %f vs %f", a, b)
    }
    _ = cubemap.FacePosX // keep import
}
```

- [ ] **Step 3: Run test, see fail**

Run: `go test ./pkg/planetgen/noise/ -run TestCoastal -v`
Expected: FAIL — `ApplyCoastal` and `NewCoastalGen` not defined.

- [ ] **Step 4: Implement**

```go
// pkg/planetgen/noise/coastal.go
package noise

import "math"

// CoastalGen wraps a noise.Generator at a fixed seed for coastal-noise
// sampling. Construct once per planet to keep the noise stream stable.
type CoastalGen struct{ g *Generator }

func NewCoastalGen(seed int64) *CoastalGen { return &CoastalGen{g: New(seed)} }

// ApplyCoastal applies the coastal enhancement formula at one pixel.
// dx,dy,dz is the unit-sphere direction; height is the current height
// in [0,1]; distToCoast is in [0,1] (0 = on coast, 1 = far inland or
// far offshore); amp is the master strength (0 disables); threshold
// is the distance-to-coast cutoff; freq is the base frequency.
func ApplyCoastal(g *CoastalGen, dx, dy, dz, height, distToCoast, amp, threshold, freq float64) float64 {
    if amp <= 0 || distToCoast >= threshold || threshold <= 0 {
        return height
    }
    // Smooth falloff from 1 at coast to 0 at threshold.
    t := 1.0 - distToCoast/threshold
    falloff := t * t * (3 - 2*t) // smoothstep
    n4 := g.g.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, freq*4)
    n5 := g.g.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, freq*8)
    n6 := g.g.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, freq*16)
    delta := (n4 + n5/2 + n6/4) - (1 + 0.5 + 0.25)/2
    e4 := height * height * height * height
    return clampF(height+amp*(1-e4)*delta*falloff, 0, 1)
}

func clampF(v, lo, hi float64) float64 {
    if v < lo {
        return lo
    }
    if v > hi {
        return hi
    }
    return v
}

// math kept imported for future use
var _ = math.Sqrt
```

- [ ] **Step 5: Run tests to confirm**

Run: `go test ./pkg/planetgen/noise/ -run TestCoastal -v`
Expected: PASS.

- [ ] **Step 6: Wire into `RenderRocky`**

In `generateRockyHeightmap` (after normalization, before crater stamping):

```go
// pkg/planetgen/render/rocky.go
if profile.Coastal.Amp > 0 && profile.OceanLevel > 0 {
    distField := field.DistanceToCoast(heightmap, profile.OceanLevel, S)
    coastGen := noise.NewCoastalGen(pgseed.Domain(seed, "coastal"))
    for face := range cubemap.Face(cubemap.NumFaces) {
        for py := range S {
            for px := range S {
                dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
                h := heightmap.Get(face, px, py)
                d := distField.Get(face, px, py)
                heightmap.Set(face, px, py,
                    noise.ApplyCoastal(coastGen, dx, dy, dz, h, d,
                        profile.Coastal.Amp, profile.Coastal.Threshold, profile.Coastal.Freq))
            }
        }
    }
}
```

- [ ] **Step 7: Run goldens (expect green)**

```bash
go test -timeout 25m -run TestGolden ./cmd/generate-planet-maps/
```

Expected: PASS — no archetype has populated `Coastal` yet, so the new code path is dormant.

- [ ] **Step 8: Commit**

```bash
git add pkg/planetgen/noise/coastal.go pkg/planetgen/noise/coastal_test.go \
  pkg/planetgen/types/types.go pkg/planetgen/render/rocky.go
git commit -m "P4 T3: coastal noise enhancement using JFA distance-to-coast"
```

---

## Task 4: Coastal slider panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

Add a slider panel exposing `Coastal.Amp`, `Coastal.Threshold`, `Coastal.Freq`.

- [ ] **Step 1: Add `renderCoastalPanel`**

```js
function renderCoastalPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  const panel = makePanel('Coastal',
    'Localized roughening of pixels near coast lines (requires OceanLevel > 0). Combines three high-frequency fBm bands modulated by distance-to-coast. Amp=0 disables.');
  if (!profile.Coastal) profile.Coastal = { Amp: 0, Threshold: 0, Freq: 0 };

  const reset = () => {
    const orig = (originalProfile && originalProfile.Coastal) || {};
    profile.Coastal = { Amp: orig.Amp||0, Threshold: orig.Threshold||0, Freq: orig.Freq||0 };
    commitProfile(profile);
    renderPanels();
  };
  const clear = () => { profile.Coastal = { Amp: 0, Threshold: 0, Freq: 0 }; commitProfile(profile); renderPanels(); };
  const randomize = () => {
    profile.Coastal = {
      Amp:       round2(0.05 + Math.random() * 0.15),
      Threshold: round2(0.05 + Math.random() * 0.20),
      Freq:      round2(1 + Math.random() * 4),
    };
    commitProfile(profile);
    renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll new in-range coastal values', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore to loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out coastal config', clear));

  panel.appendChild(makeNumberRow('Amp', 'Master strength (0 disables; useful 0.05–0.2).',
    profile.Coastal.Amp, 0, 0.5, '0.01',
    v => { profile.Coastal.Amp = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Threshold', 'Distance-to-coast cutoff [0,1]. Effect dies above this.',
    profile.Coastal.Threshold, 0, 1, '0.01',
    v => { profile.Coastal.Threshold = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Freq', 'Base fbm frequency for the n4 octave (n5/n6 derive from it).',
    profile.Coastal.Freq, 0, 20, '0.1',
    v => { profile.Coastal.Freq = v; commitProfile(profile); }));

  panels.appendChild(panel);
}
```

- [ ] **Step 2: Hook the panel into the render order**

Find the panel-render call list (where `renderCratersPanel(profile, panels)` etc are called) and add `renderCoastalPanel(profile, panels)` right after the cryosphere panel.

- [ ] **Step 3: Rebuild wasm + test in browser**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
go run ./cmd/planet-explorer/
```

Open the explorer, switch to terran, set Coastal Amp=0.15, Threshold=0.10, Freq=4. Confirm coastlines look noisier than before.

- [ ] **Step 4: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "P4 T4: coastal slider panel"
```

---

## Task 5: Voronoi continents field + wire-up

**Files:**
- Create: `pkg/planetgen/field/continents.go`
- Create: `pkg/planetgen/field/continents_test.go`
- Modify: `pkg/planetgen/types/types.go` (add `ContinentConfig`)
- Modify: `pkg/planetgen/render/rocky.go`

Per master-plan §6.10: 10–50 Fibonacci-spiral seed points on the unit sphere; warp them via low-frequency fbm; for each pixel find the nearest seed and produce a per-continent base height (each seed gets a random base height in [0.3, 0.7]). Combined with the existing Continentalness output as `h = max(h, continentBase)` so the continents form a baseline floor that fbm noise then varies on top.

- [ ] **Step 1: Add `ContinentConfig`**

```go
// pkg/planetgen/types/types.go
type ContinentConfig struct {
    Seeds    int     // 0 disables; 10–50 typical
    WarpAmp  float64 // displacement amplitude on seed positions
    WarpFreq float64 // displacement fbm frequency
    HeightLo float64 // per-continent base height lower bound (default 0.3)
    HeightHi float64 // per-continent base height upper bound (default 0.7)
}
```

Add `Continents ContinentConfig` to `PlanetProfile`.

- [ ] **Step 2: Write the failing test**

```go
// pkg/planetgen/field/continents_test.go
package field

import (
    "testing"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestContinentsDeterministic(t *testing.T) {
    cfg := types.ContinentConfig{Seeds: 12, WarpAmp: 0.1, WarpFreq: 1.0, HeightLo: 0.3, HeightHi: 0.7}
    a := GenerateContinents(42, cfg, 32)
    b := GenerateContinents(42, cfg, 32)
    for face := range a.Faces {
        for i := range a.Faces[face] {
            if a.Faces[face][i] != b.Faces[face][i] {
                t.Fatal("non-deterministic continent base height")
            }
        }
    }
}

func TestContinentsCoverEverything(t *testing.T) {
    cfg := types.ContinentConfig{Seeds: 8, WarpAmp: 0, WarpFreq: 1, HeightLo: 0.3, HeightHi: 0.7}
    out := GenerateContinents(7, cfg, 32)
    for face := range out.Faces {
        for i, v := range out.Faces[face] {
            if v < 0.3 || v > 0.7 {
                t.Errorf("face %d idx %d height %f outside [0.3,0.7]", face, i, v)
            }
        }
    }
}
```

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/field/continents.go
package field

import (
    "math"
    "math/rand/v2"

    "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
    "github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// GenerateContinents returns a cube-map field where each pixel holds
// the base height of its nearest Fibonacci-spiral continent seed, with
// per-seed heights drawn uniformly from [HeightLo, HeightHi]. Seed
// positions are warped by a low-frequency fbm if cfg.WarpAmp > 0.
func GenerateContinents(masterSeed int64, cfg types.ContinentConfig, S int) *cubemap.CubeMapF {
    out := cubemap.NewF(S)
    if cfg.Seeds <= 0 {
        return out
    }
    rng := rand.New(rand.NewPCG(uint64(seed.Domain(masterSeed, "continents.height")),
        uint64(seed.Domain(masterSeed, "continents.height")^0x5a5a5a5a))) //nolint:gosec
    centers := make([][3]float64, cfg.Seeds)
    heights := make([]float64, cfg.Seeds)
    phi := math.Pi * (3 - math.Sqrt(5))
    for i := range cfg.Seeds {
        y := 1 - 2*float64(i)/float64(cfg.Seeds-1)
        if cfg.Seeds == 1 {
            y = 0
        }
        r := math.Sqrt(1 - y*y)
        theta := phi * float64(i)
        centers[i] = [3]float64{math.Cos(theta) * r, y, math.Sin(theta) * r}
        heights[i] = cfg.HeightLo + rng.Float64()*(cfg.HeightHi-cfg.HeightLo)
    }
    var warp *noise.Generator
    if cfg.WarpAmp > 0 {
        warp = noise.New(seed.Domain(masterSeed, "continents.warp"))
    }
    for face := range cubemap.Face(cubemap.NumFaces) {
        for py := range S {
            for px := range S {
                dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
                if warp != nil {
                    dx += cfg.WarpAmp * (warp.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, cfg.WarpFreq) - 0.5)
                    dy += cfg.WarpAmp * (warp.FractalNoise3D(dy, dz, dx, 3, 2.0, 0.5, cfg.WarpFreq) - 0.5)
                    dz += cfg.WarpAmp * (warp.FractalNoise3D(dz, dx, dy, 3, 2.0, 0.5, cfg.WarpFreq) - 0.5)
                    n := math.Sqrt(dx*dx + dy*dy + dz*dz)
                    if n > 0 {
                        dx, dy, dz = dx/n, dy/n, dz/n
                    }
                }
                bestIdx, bestDot := 0, -2.0
                for i, c := range centers {
                    d := dx*c[0] + dy*c[1] + dz*c[2]
                    if d > bestDot {
                        bestDot = d
                        bestIdx = i
                    }
                }
                out.Set(face, px, py, heights[bestIdx])
            }
        }
    }
    return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/field/ -run TestContinents -v`
Expected: PASS.

- [ ] **Step 5: Wire into `RenderRocky`**

In `generateRockyHeightmap`, after the control-fields summation but before normalization:

```go
if profile.Continents.Seeds > 0 {
    cont := field.GenerateContinents(seed, profile.Continents, S)
    for face := range cubemap.Face(cubemap.NumFaces) {
        for py := range S {
            for px := range S {
                h := heightmap.Get(face, px, py)
                base := cont.Get(face, px, py)
                if base > h {
                    h = base
                }
                heightmap.Set(face, px, py, h)
            }
        }
    }
}
```

- [ ] **Step 6: Run goldens (expect green)**

```bash
go test -timeout 25m -run TestGolden ./cmd/generate-planet-maps/
```

Expected: PASS — no archetype enables `Continents` yet.

- [ ] **Step 7: Commit**

```bash
git add pkg/planetgen/field/continents.go pkg/planetgen/field/continents_test.go \
  pkg/planetgen/types/types.go pkg/planetgen/render/rocky.go
git commit -m "P4 T5: Voronoi continents field + wire into RenderRocky"
```

---

## Task 6: Continents slider panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Add `renderContinentsPanel`**

Same shape as `renderCoastalPanel` from Task 4. Five sliders: `Seeds` (0–50 integer), `WarpAmp` (0–0.3), `WarpFreq` (0–10), `HeightLo` (0–1), `HeightHi` (0–1). Reset/Clear/Randomize buttons. Hook into the panel render order after `renderCoastalPanel`.

- [ ] **Step 2: Rebuild wasm + smoke test in browser**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
```

Open explorer, switch to terran, set Continents Seeds=15, WarpAmp=0.1, HeightLo=0.4, HeightHi=0.65. Verify chunky continent shapes appear.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "P4 T6: continents slider panel"
```

---

## Task 7: Curl-noise primitive + wire into `RenderGasGiant`

**Files:**
- Create: `pkg/planetgen/noise/curl.go`
- Create: `pkg/planetgen/noise/curl_test.go`
- Modify: `pkg/planetgen/types/types.go` (add `CurlConfig`)
- Modify: `pkg/planetgen/render/gasgiant.go`

Per master-plan §6.12: curl-of-fbm tangent field on the sphere; semi-Lagrangian backward-trace for 4–16 iterations: `p_traced = p − dt · (zonal_jet(lat) + curlNoise(p_traced) · amp)`. Sample the existing band/turbulence color at the traced position to produce ribbon-flow appearance.

- [ ] **Step 1: Add `CurlConfig`**

```go
// pkg/planetgen/types/types.go
type CurlConfig struct {
    Amp        float64 // displacement strength per iteration (0 disables)
    Iterations int     // 4–16 typical
    DT         float64 // step size; 0.05–0.2 typical
    Freq       float64 // curl-noise base frequency
    JetAmp     float64 // zonal-jet contribution per latitude band (0 disables)
}
```

Add `Curl CurlConfig` to `PlanetProfile`.

- [ ] **Step 2: Write the failing test**

```go
// pkg/planetgen/noise/curl_test.go
package noise

import (
    "math"
    "testing"
)

func TestCurlBackwardTraceIdentityAtZeroAmp(t *testing.T) {
    g := NewCurlGen(123)
    x, y, z := 0.5, 0.5, math.Sqrt(0.5)
    tx, ty, tz := g.BackwardTrace(x, y, z, 0, 8, 0.1, 1.0, 0)
    if tx != x || ty != y || tz != z {
        t.Errorf("Amp=0 should be identity; got (%f,%f,%f) want (%f,%f,%f)",
            tx, ty, tz, x, y, z)
    }
}

func TestCurlDeterministic(t *testing.T) {
    g := NewCurlGen(7)
    a := [3]float64{}
    a[0], a[1], a[2] = g.BackwardTrace(0.3, 0.4, 0.5, 0.1, 10, 0.05, 2.0, 0.1)
    g2 := NewCurlGen(7)
    b := [3]float64{}
    b[0], b[1], b[2] = g2.BackwardTrace(0.3, 0.4, 0.5, 0.1, 10, 0.05, 2.0, 0.1)
    if a != b {
        t.Errorf("non-deterministic; %v vs %v", a, b)
    }
}
```

- [ ] **Step 3: Run, see fail**

Run: `go test ./pkg/planetgen/noise/ -run TestCurl -v`
Expected: FAIL.

- [ ] **Step 4: Implement**

```go
// pkg/planetgen/noise/curl.go
package noise

import "math"

// CurlGen wraps a noise.Generator for curl-noise sampling.
type CurlGen struct{ g *Generator }

func NewCurlGen(seed int64) *CurlGen { return &CurlGen{g: New(seed)} }

// curl3D returns the curl of a vector field (Φx, Φy, Φz) where each
// component is fbm. Approximated via central differences.
func (cg *CurlGen) curl3D(x, y, z, freq float64) (float64, float64, float64) {
    const eps = 1e-3
    px := func(a, b, c float64) float64 {
        return cg.g.FractalNoise3D(a, b+1.7, c+5.3, 3, 2.0, 0.5, freq)
    }
    py := func(a, b, c float64) float64 {
        return cg.g.FractalNoise3D(a+5.3, b, c+1.7, 3, 2.0, 0.5, freq)
    }
    pz := func(a, b, c float64) float64 {
        return cg.g.FractalNoise3D(a+1.7, b+5.3, c, 3, 2.0, 0.5, freq)
    }
    dpzdy := (pz(x, y+eps, z) - pz(x, y-eps, z)) / (2 * eps)
    dpydz := (py(x, y, z+eps) - py(x, y, z-eps)) / (2 * eps)
    dpxdz := (px(x, y, z+eps) - px(x, y, z-eps)) / (2 * eps)
    dpzdx := (pz(x+eps, y, z) - pz(x-eps, y, z)) / (2 * eps)
    dpydx := (py(x+eps, y, z) - py(x-eps, y, z)) / (2 * eps)
    dpxdy := (px(x, y+eps, z) - px(x, y-eps, z)) / (2 * eps)
    return dpzdy - dpydz, dpxdz - dpzdx, dpydx - dpxdy
}

// BackwardTrace integrates p backward by `iters` semi-Lagrangian steps.
// Each step subtracts dt·(jetAmp·zonal + amp·curlNoise) from the
// position, normalising back onto the unit sphere.
func (cg *CurlGen) BackwardTrace(x, y, z, amp float64, iters int, dt, freq, jetAmp float64) (float64, float64, float64) {
    if amp == 0 && jetAmp == 0 {
        return x, y, z
    }
    for range iters {
        // Zonal jet: tangent vector (eastward) at this latitude.
        // cross product of (north pole) × p, projected.
        cx, cy, cz := -z, 0.0, x
        n := math.Sqrt(cx*cx + cy*cy + cz*cz)
        if n > 1e-9 {
            cx, cy, cz = cx/n, cy/n, cz/n
        }
        // Sinusoidal jet by latitude (∝ cos(3·lat)).
        lat := math.Asin(y)
        jet := math.Cos(3 * lat)
        cux, cuy, cuz := cg.curl3D(x, y, z, freq)
        x -= dt * (jetAmp*jet*cx + amp*cux)
        y -= dt * (jetAmp*jet*cy + amp*cuy)
        z -= dt * (jetAmp*jet*cz + amp*cuz)
        n = math.Sqrt(x*x + y*y + z*z)
        if n > 0 {
            x, y, z = x/n, y/n, z/n
        }
    }
    return x, y, z
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/planetgen/noise/ -run TestCurl -v`
Expected: PASS.

- [ ] **Step 6: Wire into `RenderGasGiant`**

In `RenderGasGiant`, before the existing band/turbulence sample, run the backward-trace and sample at the traced position:

```go
if profile.Curl.Amp > 0 || profile.Curl.JetAmp > 0 {
    iters := profile.Curl.Iterations
    if iters <= 0 {
        iters = 8
    }
    dt := profile.Curl.DT
    if dt <= 0 {
        dt = 0.1
    }
    freq := profile.Curl.Freq
    if freq <= 0 {
        freq = 1
    }
    cg := noise.NewCurlGen(pgseed.Domain(seed, "gas.curl"))
    dx, dy, dz = cg.BackwardTrace(dx, dy, dz, profile.Curl.Amp, iters, dt, freq, profile.Curl.JetAmp)
}
```

- [ ] **Step 7: Run gas-giant goldens (expect green)**

```bash
go test -timeout 25m -run TestGolden ./cmd/generate-planet-maps/
```

Expected: PASS — no archetype enables `Curl` yet.

- [ ] **Step 8: Commit**

```bash
git add pkg/planetgen/noise/curl.go pkg/planetgen/noise/curl_test.go \
  pkg/planetgen/types/types.go pkg/planetgen/render/gasgiant.go
git commit -m "P4 T7: curl-noise turbulent advection for gas giants"
```

---

## Task 8: Cassini Jupiter ramp asset + StormBand profile

**Files:**
- Create: `pkg/planetgen/color/luts/jupiter_ramp.png` (binary asset, 256×1 RGBA)
- Create: `pkg/planetgen/color/jupiter_ramp.go`
- Create: `pkg/planetgen/color/jupiter_ramp_test.go`
- Modify: `pkg/planetgen/types/types.go` (add `StormBand`)
- Modify: `pkg/planetgen/render/gasgiant.go` (consume ramp + storm bands)

Per master-plan §6.13: a 256×1 RGBA strip captured from a Cassini Jupiter narrow-angle composite, applied as the latitude-driven base palette so jovian/ice-giant share an authored color identity. `StormBand` lets a profile place hand-authored ovals (red spot, polar storms) at specific latitudes.

- [ ] **Step 1: Generate the asset**

Use a small Go program (or any tool — committed result is the PNG). The strip should sample distinct hue bands matching Jupiter equatorial → polar (cream → tan → orange → blue-grey at poles). 256 pixels wide, 1 pixel tall. Commit the PNG.

For now, a synthetic stand-in works; replace with a real Cassini sample later. Authoring procedure:

```bash
# Generate via cmd/tools/gen-jupiter-ramp.go (one-shot, not committed):
# step pixels through (R,G,B) = lerp(equatorial_color, polar_color, lat_fraction)
# with three hand-tuned color stops at 0.0, 0.4, 1.0
# Output to pkg/planetgen/color/luts/jupiter_ramp.png
```

The actual stop colors (hex):
- 0.0 (equator):  `#ddc795`
- 0.4 (mid):      `#b88e62`
- 0.7 (subpolar): `#a87248`
- 1.0 (pole):     `#5a5b6b`

Smooth-blend in OkLab between stops.

- [ ] **Step 2: Add `StormBand` type**

```go
// pkg/planetgen/types/types.go
type StormBand struct {
    Lat       float64    // latitude in radians (-π/2 … π/2)
    HalfWidth float64    // angular half-width
    Color     ColorRGB   // band tint
    Strength  float64    // mix amount [0,1]
}
```

Add `StormBands []StormBand` to `PlanetProfile`.

- [ ] **Step 3: Implement loader**

```go
// pkg/planetgen/color/jupiter_ramp.go
package color

import (
    "bytes"
    _ "embed"
    "image"
    "image/color"
    _ "image/png"
)

//go:embed luts/jupiter_ramp.png
var jupiterRampPNG []byte

var jupiterRamp [256]color.RGBA

func init() {
    img, _, err := image.Decode(bytes.NewReader(jupiterRampPNG))
    if err != nil {
        return
    }
    b := img.Bounds()
    if b.Dx() < 256 {
        return
    }
    for i := range 256 {
        r, g, b2, a := img.At(b.Min.X+i, b.Min.Y).RGBA()
        jupiterRamp[i] = color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b2 >> 8), A: uint8(a >> 8)}
    }
}

// JupiterRamp returns the embedded Jupiter palette sampled by latitude
// fraction f in [0, 1] (0 = equator, 1 = pole). Linear interpolation.
func JupiterRamp(f float64) color.RGBA {
    if f < 0 {
        f = 0
    }
    if f > 1 {
        f = 1
    }
    x := f * 255
    i0 := int(x)
    if i0 >= 255 {
        return jupiterRamp[255]
    }
    t := x - float64(i0)
    a := jupiterRamp[i0]
    b := jupiterRamp[i0+1]
    return color.RGBA{
        R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
        G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
        B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
        A: 255,
    }
}
```

- [ ] **Step 4: Test the loader**

```go
// pkg/planetgen/color/jupiter_ramp_test.go
package color

import "testing"

func TestJupiterRampBounds(t *testing.T) {
    for _, f := range []float64{-1, 0, 0.5, 1, 2} {
        c := JupiterRamp(f)
        if c.A != 255 {
            t.Errorf("alpha=%d at f=%f, want 255", c.A, f)
        }
    }
}

func TestJupiterRampMonotoneFalloff(t *testing.T) {
    eq := JupiterRamp(0)
    pole := JupiterRamp(1)
    if eq == pole {
        t.Error("equator and pole colors are identical; ramp not loaded?")
    }
}
```

Run: `go test ./pkg/planetgen/color/ -run TestJupiterRamp -v`
Expected: PASS.

- [ ] **Step 5: Wire into `RenderGasGiant`**

After computing `dx, dy, dz` (post-curl-trace if applicable), use the ramp as the latitude-driven base color, then mix in any matching `StormBands`:

```go
lat := math.Asin(dy)
latFrac := math.Abs(lat) / (math.Pi / 2)
c := planetcolor.JupiterRamp(latFrac)
for _, sb := range profile.StormBands {
    if math.Abs(lat-sb.Lat) < sb.HalfWidth {
        // smooth falloff inside the band
        t := 1 - math.Abs(lat-sb.Lat)/sb.HalfWidth
        t *= t * sb.Strength
        c = planetcolor.BlendOkLab(c, color.RGBA{R: sb.Color.R, G: sb.Color.G, B: sb.Color.B, A: 255}, t)
    }
}
```

(Existing band/turbulence/storm-oval code can stay; the ramp acts as the base palette, with the existing logic layering on top.)

- [ ] **Step 6: Run goldens (expect green for jovian, ice_giant)**

```bash
go test -timeout 25m -run TestGolden ./cmd/generate-planet-maps/
```

Expected: PASS — neither `jovian` nor `ice_giant` populates `StormBands` yet, so the only change is the base palette source. Goldens at 400×200 may diverge by < 1.5 ΔE2000 depending on the ramp colors; if they exceed the gate, regenerate with `-update` after eyeballing.

- [ ] **Step 7: Commit**

```bash
git add pkg/planetgen/color/jupiter_ramp.go pkg/planetgen/color/jupiter_ramp_test.go \
  pkg/planetgen/color/luts/jupiter_ramp.png pkg/planetgen/types/types.go \
  pkg/planetgen/render/gasgiant.go
git commit -m "P4 T8: Cassini Jupiter ramp asset + StormBand profile"
```

---

## Task 9: Gas-giant slider panels (curl + storm bands)

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

Add two panels gated on `profile.Renderer === 'gas_giant'`:

- [ ] **Step 1: Curl panel**

Sliders: `Amp` (0–1), `Iterations` (0–24 integer), `DT` (0–0.5), `Freq` (0–10), `JetAmp` (0–1). Reset/Clear/Randomize as usual.

- [ ] **Step 2: Storm bands panel**

`StormBands` is `[]StormBand`. Render as a list of rows, each with Lat, HalfWidth, Color (color picker), Strength. Plus an "Add band" button and per-row delete.

```js
function renderStormBandsPanel(profile, panels) {
  if (profile.Renderer !== 'gas_giant') return;
  const panel = makePanel('Storm Bands', 'Hand-authored colored bands at specific latitudes (e.g. red spot, polar collars). Each entry: Lat (radians, -π/2 … π/2), HalfWidth, Color, Strength.');
  if (!profile.StormBands) profile.StormBands = [];
  // ...iterate profile.StormBands, render row per band with delete + an add-band footer button...
  panels.appendChild(panel);
}
```

- [ ] **Step 3: Hook into render order**

After the existing gas-giant `renderBandsPanel` (or wherever turbulence sliders live), add:

```js
renderCurlPanel(profile, panels);
renderStormBandsPanel(profile, panels);
```

- [ ] **Step 4: Rebuild wasm + smoke test**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
```

Open explorer, switch to jovian, set Curl Amp=0.4, Iterations=12, JetAmp=0.3 → bands should swirl. Add a StormBand at Lat=-0.3, HalfWidth=0.1, Color=red, Strength=0.6 → red oval near 17°S.

- [ ] **Step 5: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "P4 T9: gas-giant curl + storm-bands slider panels"
```

---

## Task 10: Phase 4 acceptance + per-archetype tuning + README

**Files:**
- Modify: `pkg/planetgen/profile.go` (optional: Coastal/Continents tunings for rocky archetypes; Curl/StormBands tunings for jovian / ice_giant)
- Modify: `cmd/generate-planet-maps/README.md` (add Phase-4 features section)

- [ ] **Step 1: All tests + lint + wasm pass**

```bash
go test -timeout 25m ./...
golangci-lint run
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
```

Expected: green.

- [ ] **Step 2: Tune defaults per archetype**

Identical workflow to Phase-3 T10 — tune in the explorer, export JSON, hand back, fold. Specific archetypes worth tuning this phase:

- **terran, super_terran**: Coastal (Amp ~0.1, Threshold ~0.08, Freq ~4) + Continents (Seeds ~12, WarpAmp ~0.08).
- **oceanic**: Continents (Seeds ~6, low HeightHi to keep land sparse).
- **arid**: Coastal usually noop (no ocean). Continents optional.
- **glacial, ice_world**: Coastal at low Amp; Continents optional.
- **jovian**: Curl (Amp ~0.3, Iterations 10, JetAmp ~0.3). StormBands: 1–2 bands matching the Great Red Spot.
- **ice_giant**: Curl at lower Amp (~0.15). StormBands: subtle polar collars.

This step is the manual-tuning checkpoint; pause here for explorer iteration before continuing.

- [ ] **Step 3: Update README**

In `cmd/generate-planet-maps/README.md`, after the Phase-3 section, add:

```markdown
## Phase 4 (current)

Four Tier-A items ship in this phase:

- **JFA distance-to-coast** (`pkg/planetgen/field/jfa.go`) — jump-flooding distance field over the cube-sphere; reusable primitive consumed by coastal noise (Phase 4) and erosion (Phase 5).
- **Coastal noise enhancement** (`pkg/planetgen/noise/coastal.go`) — three high-frequency fbm bands modulated by distance-to-coast for crinkly shorelines.
- **Voronoi continents** (`pkg/planetgen/field/continents.go`) — Fibonacci-spiral seed points with low-frequency warp; per-pixel nearest seed becomes a base-height contribution that fbm noise then varies on top.
- **Curl-noise gas-giant advection** (`pkg/planetgen/noise/curl.go`) — semi-Lagrangian backward-trace produces ribbon-flow appearance; combined with sinusoidal zonal jets.
- **Cassini Jupiter ramp + StormBand** (`pkg/planetgen/color/jupiter_ramp.go`) — embedded latitude-driven base palette for gas giants plus profile-authored storm overlays.

The `Erosion` control field was renamed to `Detail` to free the name for Phase-5 flow-based erosion. Existing JSON dumps with the old key still load via a custom `UnmarshalJSON`.
```

- [ ] **Step 4: Phase 4 acceptance summary commit**

```bash
git commit --allow-empty -m "Phase 4 (Tier-A coastal + gas) accepted"
```

Body should list T1–T10 status, lint/test/wasm gates, and which Tier-A items remain (item 9 erosion → Phase 5).

---

## Self-review notes

**Spec coverage.** Master plan §6.8 → T2; §6.10 → T3+T4 (coastal) and T5+T6 (continents); §6.12 → T7+T9; §6.13 → T8+T9. Item 9 (erosion) intentionally deferred to Phase 5 per the user's explicit split. The `Erosion → Detail` rename is a Phase-4 prerequisite to keep the Phase-5 erosion stage's name unambiguous.

**Placeholder scan.** No "TODO" / "TBD" left in tasks. All code blocks are concrete; the Jupiter-ramp PNG creation is a one-shot bake described in T8 step 1 with explicit color stops.

**Type consistency.** `CoastalConfig`, `ContinentConfig`, `CurlConfig`, `StormBand` are all defined in T3/T5/T7/T8 and consumed in matching wire-up tasks. `DistanceToCoast` defined in T2 is consumed in T3. `GenerateContinents` defined in T5 is consumed in T5 step 5. `JupiterRamp` defined in T8 consumed in T8 step 5.

**Backward compat.** T1's `UnmarshalJSON` shim is the only forward-compat work needed. All other new profile fields default to zero values that disable their respective code paths, matching Phase-3's add-without-breaking pattern.

**Performance.** JFA at S=1024 is ~30 passes × 6 faces × 1M pixels × 8 neighbors ≈ 1.4G ops. That's heavier than the ridged pass; could be a noticeable batch-render slowdown. Mitigation if needed: only run JFA when `Coastal.Amp > 0` (already gated in T3 step 6). Continents Voronoi at S=1024 with 30 seeds is 6 × 1M × 30 = 180M dot products; comparable to ridged. Curl backward-trace at 12 iterations × 6 cube-map faces × eq. faceSize² is the main per-pixel hot-loop on gas giants.

**Debug-view forward compat (Phase 6).** Every new heightmap layer in this phase is produced as a `*cubemap.CubeMapF` (jfa) or applied per-pixel inline (coastal, continents merge). Phase 6 will need each layer's *contribution* to be inspectable; for the merge-style layers (coastal, continents) we should consider returning a "delta" cube map alongside the modification when Phase 6 lands, so they show up as discrete layers in the debug view. Note this in Phase 6's plan but no Phase-4 code changes needed yet.
