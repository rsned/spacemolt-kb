# Planet Gen Phase 3 — Rocky Surface Character (Tier A bucket A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land three Tier-A algorithms that transform how rocky archetypes read at every zoom level: ridged-multifractal mountain belts masked by continentality, Voronoi province modulation that gives each planet regional roughness variety, and a properly designed crater system (power-law SFD, age, ejecta rays, secondaries). Each algorithm gets a slider panel in `cmd/planet-explorer/` so per-archetype tuning can happen iteratively the same way Phase 1/2 did.

**Architecture:** Three independent algorithms, each living in its assigned subpackage with TDD-style unit tests, then wired into `RenderRocky`. None depend on JFA (deferred to Phase 4 bucket B). Profile fields gate each algorithm — empty/zero values short-circuit to the legacy single-fBm path so the addition is opt-in per archetype until tuning lands. The existing simple Craters panel from Phase 2 is replaced by the richer crater system panel.

**Tech Stack:** Go 1.24, existing `opensimplex-go` (already in `go.mod`). No new dependencies. No cgo. Wasm-compatible.

---

## File structure

**Item 6 — Ridged multifractal mountains:**
- Create: `pkg/planetgen/noise/ridged.go`
- Create: `pkg/planetgen/noise/ridged_test.go`
- Modify: `pkg/planetgen/types/types.go` (add `RidgedConfig` + field on `PlanetProfile`)
- Modify: `pkg/planetgen/render/rocky.go` (apply ridged contribution after control fields, before craters)
- Modify: `cmd/planet-explorer/web/app.js` (`renderRidgedPanel`)

**Item 7 — Province / roughness modulation:**
- Create: `pkg/planetgen/field/province.go`
- Create: `pkg/planetgen/field/province_test.go`
- Modify: `pkg/planetgen/types/types.go` (add `ProvinceConfig`)
- Modify: `pkg/planetgen/render/rocky.go` (multiply control-field amplitudes by per-pixel province modulation)
- Modify: `cmd/planet-explorer/web/app.js` (`renderProvincePanel`)

**Item 11 — Crater system rebuild:**
- Modify: `pkg/planetgen/feature/crater.go` (replace the simple uniform-distribution generator with a power-law SFD + age + secondaries + maria mask)
- Create: `pkg/planetgen/feature/crater_ejecta.go` (ejecta-ray albedo overlay; height-only stays in `crater.go`)
- Create: `pkg/planetgen/feature/crater_rebuild_test.go`
- Modify: `pkg/planetgen/types/types.go` (extend crater profile fields)
- Modify: `pkg/planetgen/render/rocky.go` (wire ejecta albedo into the post-color pass)
- Modify: `cmd/planet-explorer/web/app.js` (replace `renderCratersPanel` with the richer version)

**Acceptance:**
- Modify: `cmd/generate-planet-maps/testdata/golden/*.png` if any goldens drift past ΔE2000 1.5 (only with `-update` and a per-archetype before/after diff PNG saved as commit evidence)

---

## Task 1: Ridged multifractal noise primitive

**Files:**
- Create: `pkg/planetgen/noise/ridged.go`
- Create: `pkg/planetgen/noise/ridged_test.go`

`RidgedMulti(p, octaves, lacunarity, gain, offset)` returns `Σᵢ wᵢ·(offset − |fbm_i(p)|)²` with weights `wᵢ = clamp(prev² · gain, 0, 1)`. Output range roughly [0, offset²·gainSum]. Adds a new `Generator` method `RidgedFractal3D`.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/noise/ridged_test.go
package noise

import (
	"math"
	"testing"
)

func TestRidgedRange(t *testing.T) {
	g := New(42)
	min, max := math.Inf(1), math.Inf(-1)
	N := 1024
	for i := 0; i < N; i++ {
		theta := float64(i) * 2 * math.Pi / float64(N)
		v := g.RidgedFractal3D(math.Cos(theta), math.Sin(theta), 0, 4, 2.0, 0.5, 1.0)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite ridged value: %v", v)
		}
		if v < min { min = v }
		if v > max { max = v }
	}
	if min < 0 {
		t.Errorf("ridged should be ≥0, got min=%v", min)
	}
	// 4-octave gain=0.5 offset=1: peak roughly Σ(0.5^i) ≈ 1.875.
	if max < 0.3 {
		t.Errorf("ridged peak suspiciously low: %v", max)
	}
}

func TestRidgedDeterministic(t *testing.T) {
	a := New(1).RidgedFractal3D(0.3, 0.7, 0.1, 5, 2.0, 0.5, 1.0)
	b := New(1).RidgedFractal3D(0.3, 0.7, 0.1, 5, 2.0, 0.5, 1.0)
	if a != b {
		t.Errorf("ridged not deterministic: %v vs %v", a, b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/noise/ -run TestRidged -v`
Expected: FAIL with `RidgedFractal3D undefined`.

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/noise/ridged.go
package noise

import "math"

// RidgedFractal3D evaluates a ridged-multifractal at point p.
// Each octave contributes (offset - |noise|)² weighted by the previous
// octave's signal² × gain (clamped to [0,1]). Produces sharp, ridge-like
// features good for mountain belts. Output is non-negative.
func (g *Generator) RidgedFractal3D(x, y, z float64, octaves int, lacunarity, gain, offset float64) float64 {
	if octaves <= 0 {
		return 0
	}
	freq := 1.0
	weight := 1.0
	sum := 0.0
	for i := 0; i < octaves; i++ {
		signal := offset - math.Abs(g.Noise3D(x*freq, y*freq, z*freq))
		signal *= signal
		signal *= weight
		sum += signal
		weight = signal * gain
		if weight > 1 { weight = 1 }
		if weight < 0 { weight = 0 }
		freq *= lacunarity
	}
	return sum
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/planetgen/noise/ -run TestRidged -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/planetgen/noise/ridged.go pkg/planetgen/noise/ridged_test.go
git commit -m "Add ridged multifractal noise primitive"
```

---

## Task 2: RidgedConfig profile field + render wire-up

**Files:**
- Modify: `pkg/planetgen/types/types.go` (add `RidgedConfig`)
- Modify: `pkg/planetgen/render/rocky.go` (apply ridged contribution, masked by Continentalness)

Each profile gets a `Ridged RidgedConfig`. When `Ridged.Amp > 0`, ridged output is added to the heightmap, masked by a smoothstep on the Continentalness control-field output (so ridges form on land, not under ocean). The result is normalized in the existing post-pass.

- [ ] **Step 1: Add the type**

```go
// pkg/planetgen/types/types.go (append after WarpConfig)

// RidgedConfig parameterizes a ridged-multifractal mountain pass.
// Apply after the control-field heightmap, masked by Continentalness
// so ridges only form on land. Amp=0 disables.
type RidgedConfig struct {
	Amp         float64 // overall mountain contribution (0 = disabled)
	Freq        float64 // base frequency
	Octaves     int     // octaves of ridged-fbm (typical 4–6)
	Lacunarity  float64 // freq multiplier per octave (default 2.0)
	Gain        float64 // per-octave weight gain (default 0.5)
	Offset      float64 // ridge sharpness (default 1.0; >1 sharper)
	MaskLow     float64 // Continentalness output below this = no ridges
	MaskHigh    float64 // Continentalness output above this = full ridges
}
```

Add field to `PlanetProfile`:

```go
	Ridged RidgedConfig // Tier-A: ridged mountain belts
```

- [ ] **Step 2: Wire into render**

Locate the heightmap loop in `pkg/planetgen/render/rocky.go` (the per-pixel control-field summation) and add a ridged contribution after the spline sum, before normalization:

```go
// inside the per-pixel heightmap loop, after `h = sum of spline outputs`
if profile.Ridged.Amp > 0 {
	cont := planetcolor.EvalSpline(profile.ControlConfig.Continentalness.Spline,
		fields[0].Get(face, px, py))
	mask := smoothstep(profile.Ridged.MaskLow, profile.Ridged.MaskHigh, cont)
	if mask > 0 {
		r := ridgedGen.RidgedFractal3D(dx*profile.Ridged.Freq, dy*profile.Ridged.Freq, dz*profile.Ridged.Freq,
			profile.Ridged.Octaves, profile.Ridged.Lacunarity,
			profile.Ridged.Gain, profile.Ridged.Offset)
		h += profile.Ridged.Amp * mask * r
	}
}

// add helper at file scope
func smoothstep(lo, hi, x float64) float64 {
	if hi <= lo { return 0 }
	t := (x - lo) / (hi - lo)
	if t < 0 { t = 0 }
	if t > 1 { t = 1 }
	return t * t * (3 - 2*t)
}
```

`ridgedGen` should be created once per `RenderRocky` call using a domain-mixed seed:
`ridgedGen := noise.New(seed.Domain(masterSeed, "ridged"))`

- [ ] **Step 3: Run goldens**

Run: `go test ./cmd/generate-planet-maps/... -run TestGolden -v`
Expected: All 13 PASS — every shipping profile has `Ridged.Amp == 0` so output is byte-identical.

- [ ] **Step 4: Commit**

```bash
git add pkg/planetgen/types/types.go pkg/planetgen/render/rocky.go
git commit -m "Wire ridged multifractal heightmap pass into RenderRocky"
```

---

## Task 3: Ridged slider panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (add `renderRidgedPanel`, register in `renderPanels`)

Mirrors the warp panel's structure: Randomize / Reset / Clear in the summary, numeric rows for each parameter. Hidden when `Renderer !== 'rocky'`.

- [ ] **Step 1: Add the panel**

```js
function renderRidgedPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  if (!profile.Ridged) profile.Ridged = {Amp:0, Freq:0, Octaves:0, Lacunarity:0, Gain:0, Offset:0, MaskLow:0, MaskHigh:0};
  const panel = makePanel('Ridged mountains',
    'Ridged-multifractal mountain belts. Masked by Continentalness so ridges only form on land. Amp=0 disables.');

  const reset = () => {
    if (originalProfile?.Ridged) {
      profile.Ridged = JSON.parse(JSON.stringify(originalProfile.Ridged));
    } else {
      profile.Ridged = {Amp:0, Freq:0, Octaves:0, Lacunarity:0, Gain:0, Offset:0, MaskLow:0, MaskHigh:0};
    }
    commitProfile(profile); renderPanels();
  };
  const clear = () => {
    profile.Ridged = {Amp:0, Freq:0, Octaves:0, Lacunarity:0, Gain:0, Offset:0, MaskLow:0, MaskHigh:0};
    commitProfile(profile); renderPanels();
  };
  const randomize = () => {
    profile.Ridged = {
      Amp: round2(0.05 + Math.random() * 0.25),
      Freq: round2(0.5 + Math.random() * 4),
      Octaves: 3 + Math.floor(Math.random() * 4),
      Lacunarity: round2(1.8 + Math.random() * 0.6),
      Gain: round2(0.4 + Math.random() * 0.4),
      Offset: round2(0.8 + Math.random() * 0.4),
      MaskLow: round2(0.3 + Math.random() * 0.2),
      MaskHigh: round2(0.6 + Math.random() * 0.2),
    };
    commitProfile(profile); renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll random in-range ridged params', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore Ridged to loaded JSON values', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Zero out all ridged params', clear));

  const help = {
    Amp: 'Overall mountain contribution to the heightmap. 0 = disabled. Useful 0.05–0.3.',
    Freq: 'Base spatial frequency of the ridges. Higher = more rugged. Useful 0.5–5.',
    Octaves: 'Stacked ridged-fbm layers. More = more detail. Typical 4–6.',
    Lacunarity: 'Frequency multiplier per octave. Standard = 2.0.',
    Gain: 'Per-octave weight gain. Standard = 0.5. Higher = sharper ridges.',
    Offset: 'Ridge sharpness. 1.0 default; values > 1 produce sharper peaks.',
    MaskLow: 'Continentalness-spline output below this = no ridges (deep ocean).',
    MaskHigh: 'Continentalness-spline output above this = full ridges (interior).',
  };
  for (const param of ['Amp','Freq','Octaves','Lacunarity','Gain','Offset','MaskLow','MaskHigh']) {
    const step = (param === 'Octaves') ? '1' : '0.01';
    panel.appendChild(makeNumberRow(param, help[param],
      profile.Ridged[param] || 0, 0, param === 'Octaves' ? 8 : 5, step,
      v => { profile.Ridged[param] = v; commitProfile(profile); }));
  }
  panels.appendChild(panel);
}
```

- [ ] **Step 2: Register in `renderPanels`**

Insert `renderRidgedPanel(profile, panels);` immediately after `renderWarpPanel(profile, panels);`.

- [ ] **Step 3: Manual smoke test**

`(cd cmd/planet-explorer && go run .)` — open browser. Type=terran. Click the new "Ridged mountains" panel.
- Click Randomize → values populate, click Regenerate → visible mountain belts appear in mid-elevation regions.
- Click Clear → ridges disappear on Regenerate.

- [ ] **Step 4: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "Add ridged-mountain editor panel"
```

---

## Task 4: Province field generator

**Files:**
- Create: `pkg/planetgen/field/province.go`
- Create: `pkg/planetgen/field/province_test.go`

Generates a low-frequency Voronoi field over the cube-map. Cell membership is a deterministic integer per pixel. Cells are seeded from a Fibonacci spiral on the unit sphere, plus warp jitter. Returns three `*cubemap.CubeMapF` outputs: cell-id (encoded as float), per-cell `R_amp` modulator [0.5, 1.5], per-cell `R_freq` modulator [0.5, 1.5].

- [ ] **Step 1: Write the test**

```go
// pkg/planetgen/field/province_test.go
package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestProvinceCellCountMatchesConfig(t *testing.T) {
	cfg := types.ProvinceConfig{Count: 12, Jitter: 0.1, WarpAmp: 0.05}
	id, _, _ := GenerateProvinces(42, cfg, 64)
	if id == nil { t.Fatal("nil cell-id field") }
	uniq := map[int]struct{}{}
	for face := 0; face < 6; face++ {
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				uniq[int(id.Get(face, x, y))] = struct{}{}
			}
		}
	}
	if len(uniq) < 6 || len(uniq) > 12 {
		t.Errorf("expected 6–12 cells visible at S=64, got %d", len(uniq))
	}
}

func TestProvinceModulatorRanges(t *testing.T) {
	cfg := types.ProvinceConfig{Count: 8, Jitter: 0.1}
	_, ramp, rfreq := GenerateProvinces(7, cfg, 32)
	for _, f := range []*FloatCubeMap{ramp, rfreq} {
		for face := 0; face < 6; face++ {
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					v := f.Get(face, x, y)
					if math.IsNaN(v) || v < 0.5 || v > 1.5 {
						t.Errorf("modulator out of [0.5,1.5]: %v", v)
					}
				}
			}
		}
	}
}
```

(`FloatCubeMap` here is a placeholder for whatever `cubemap.CubeMapF` type alias the package exposes — match the existing pattern.)

- [ ] **Step 2: Verify failure**

Run: `go test ./pkg/planetgen/field/ -run TestProvince -v`
Expected: FAIL with `GenerateProvinces undefined`.

- [ ] **Step 3: Add the type**

In `pkg/planetgen/types/types.go`:

```go
// ProvinceConfig parameterizes per-region roughness modulation
// (Voronoi cells over the sphere, each with jittered amp/freq scalars
// applied to the underlying control fields).
type ProvinceConfig struct {
	Count   int     // number of Voronoi cells (8–40 typical; 0 = disabled)
	Jitter  float64 // per-cell scalar jitter strength (0–0.5; default 0.2)
	WarpAmp float64 // sphere-warp displacement before nearest-cell lookup
}
```

Add `Provinces ProvinceConfig` field to `PlanetProfile`.

- [ ] **Step 4: Implement `GenerateProvinces`**

```go
// pkg/planetgen/field/province.go
package field

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// GenerateProvinces returns three cube-map fields:
//   id    — integer cell membership encoded as float (deterministic in [0, Count))
//   ramp  — per-cell amp modulator in [0.5, 1.5]
//   rfreq — per-cell freq modulator in [0.5, 1.5]
//
// Cells seeded from Fibonacci spiral on unit sphere, optional pre-lookup warp.
func GenerateProvinces(masterSeed int64, cfg types.ProvinceConfig, S int) (id, ramp, rfreq *cubemap.CubeMapF) {
	if cfg.Count <= 0 {
		return nil, nil, nil
	}
	// Seed cell centers with Fibonacci spiral.
	centers := make([][3]float64, cfg.Count)
	phi := math.Pi * (math.Sqrt(5) - 1)
	for i := 0; i < cfg.Count; i++ {
		y := 1 - float64(2*i)/float64(cfg.Count-1)
		r := math.Sqrt(1 - y*y)
		theta := float64(i) * phi
		centers[i] = [3]float64{math.Cos(theta) * r, y, math.Sin(theta) * r}
	}
	// Per-cell jitter (deterministic, seeded from master).
	jitterAmp := make([]float64, cfg.Count)
	jitterFreq := make([]float64, cfg.Count)
	jitterRng := noise.NewRand(seed.Domain(masterSeed, "province.jitter"))
	for i := 0; i < cfg.Count; i++ {
		j := cfg.Jitter
		jitterAmp[i] = 1.0 + (jitterRng.Float64()*2 - 1) * j
		jitterFreq[i] = 1.0 + (jitterRng.Float64()*2 - 1) * j
	}

	id = cubemap.NewCubeMapF(S)
	ramp = cubemap.NewCubeMapF(S)
	rfreq = cubemap.NewCubeMapF(S)

	warp := noise.New(seed.Domain(masterSeed, "province.warp"))
	for face := 0; face < 6; face++ {
		for y := 0; y < S; y++ {
			for x := 0; x < S; x++ {
				dx, dy, dz := cubemap.FacePixelToDir(face, x, y, S)
				if cfg.WarpAmp > 0 {
					wx := warp.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 1.5)
					wy := warp.FractalNoise3D(dx+11, dy+7, dz+13, 3, 2.0, 0.5, 1.5)
					wz := warp.FractalNoise3D(dx-9, dy-3, dz-5, 3, 2.0, 0.5, 1.5)
					dx += wx * cfg.WarpAmp
					dy += wy * cfg.WarpAmp
					dz += wz * cfg.WarpAmp
					n := math.Sqrt(dx*dx + dy*dy + dz*dz)
					dx, dy, dz = dx/n, dy/n, dz/n
				}
				// Nearest cell by 3D dot-product distance.
				bestI, bestD := 0, -2.0
				for i, c := range centers {
					d := dx*c[0] + dy*c[1] + dz*c[2]
					if d > bestD { bestD = d; bestI = i }
				}
				id.Set(face, x, y, float64(bestI))
				ramp.Set(face, x, y, jitterAmp[bestI])
				rfreq.Set(face, x, y, jitterFreq[bestI])
			}
		}
	}
	return id, ramp, rfreq
}
```

(If `noise.NewRand` doesn't exist yet, add a wrapper around `rand.New(rand.NewSource(seed))`. If `cubemap.NewCubeMapF` / `Set` aren't the exact names, mirror whatever the existing code uses.)

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/planetgen/field/ -run TestProvince -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/planetgen/field/province.go pkg/planetgen/field/province_test.go pkg/planetgen/types/types.go
git commit -m "Add Voronoi province modulation field"
```

---

## Task 5: Wire provinces into RenderRocky

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`

When `profile.Provinces.Count > 0`, generate the three province cube-maps once per render and multiply each control-field's per-pixel sample by the corresponding `(ramp, rfreq)` modulator before passing through the spline.

- [ ] **Step 1: Add the call**

Near the top of `RenderRocky`, after control-field generation:

```go
var provRamp, provRFreq *cubemap.CubeMapF
if profile.Provinces.Count > 0 {
	_, provRamp, provRFreq = field.GenerateProvinces(masterSeed, profile.Provinces, faceSize)
}
```

In the per-pixel heightmap loop, modify the spline contribution:

```go
for i := 0; i < 5; i++ {
	v := fields[i].Get(face, px, py)
	if provRamp != nil {
		// Scale "amp" effect by ramp, "freq" effect by rescaling input.
		// Simplest: multiply spline output by ramp, multiply spline input by rfreq.
		v *= provRFreq.Get(face, px, py)
	}
	contribution := planetcolor.EvalSpline(cfFields[i].Spline, v)
	if provRamp != nil {
		contribution *= provRamp.Get(face, px, py)
	}
	h += contribution
}
```

(The `freq` modulator goes on the spline input — equivalent to scaling the field's effective sampling frequency. The `amp` modulator goes on the output — equivalent to scaling each octave's contribution.)

- [ ] **Step 2: Goldens still green**

Run: `go test ./cmd/generate-planet-maps/... -run TestGolden -v`
Expected: All PASS — every shipping profile has `Provinces.Count == 0`.

- [ ] **Step 3: Commit**

```bash
git add pkg/planetgen/render/rocky.go
git commit -m "Wire province modulation into RenderRocky control-field sums"
```

---

## Task 6: Province slider panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (add `renderProvincePanel`, register)

- [ ] **Step 1: Add the panel**

```js
function renderProvincePanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  if (!profile.Provinces) profile.Provinces = {Count: 0, Jitter: 0, WarpAmp: 0};
  const panel = makePanel('Provinces',
    'Voronoi cells over the sphere, each with jittered amp/freq scalars applied to the control fields. Gives each archetype regional roughness variety. Count=0 disables.');

  const reset = () => {
    if (originalProfile?.Provinces) profile.Provinces = JSON.parse(JSON.stringify(originalProfile.Provinces));
    else profile.Provinces = {Count: 0, Jitter: 0, WarpAmp: 0};
    commitProfile(profile); renderPanels();
  };
  const clear = () => {
    profile.Provinces = {Count: 0, Jitter: 0, WarpAmp: 0};
    commitProfile(profile); renderPanels();
  };
  const randomize = () => {
    profile.Provinces = {
      Count: 8 + Math.floor(Math.random() * 24),  // 8–32
      Jitter: round2(0.1 + Math.random() * 0.3),
      WarpAmp: round2(Math.random() * 0.15),
    };
    commitProfile(profile); renderPanels();
  };

  const summary = panel.querySelector('summary');
  summary.appendChild(makeAuxBtn('Randomize', 'Roll random province params', randomize));
  summary.appendChild(makeAuxBtn('Reset', 'Restore Provinces to loaded JSON', reset));
  summary.appendChild(makeAuxBtn('Clear', 'Disable provinces', clear));

  panel.appendChild(makeNumberRow('Count',
    'Number of Voronoi cells (8–40 typical; 0 = disabled).',
    profile.Provinces.Count, 0, 64, '1',
    v => { profile.Provinces.Count = Math.max(0, Math.round(v)); commitProfile(profile); }));
  panel.appendChild(makeNumberRow('Jitter',
    'Per-cell scalar jitter strength. 0 = uniform; 0.5 = high variety.',
    profile.Provinces.Jitter, 0, 0.5, '0.01',
    v => { profile.Provinces.Jitter = v; commitProfile(profile); }));
  panel.appendChild(makeNumberRow('WarpAmp',
    'Sphere-warp displacement before nearest-cell lookup. 0 = clean Voronoi; >0 = curvy boundaries.',
    profile.Provinces.WarpAmp, 0, 0.3, '0.01',
    v => { profile.Provinces.WarpAmp = v; commitProfile(profile); }));
  panels.appendChild(panel);
}
```

- [ ] **Step 2: Register**

Insert `renderProvincePanel(profile, panels);` after `renderRidgedPanel`.

- [ ] **Step 3: Smoke test**

Type=terran. New "Provinces" panel. Click Randomize → Regenerate → visible regional variety in roughness. Click Clear → uniform terrain again.

- [ ] **Step 4: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "Add provinces editor panel"
```

---

## Task 7: Crater system rebuild — power-law SFD + age + maria

**Files:**
- Modify: `pkg/planetgen/feature/crater.go`
- Create: `pkg/planetgen/feature/crater_rebuild_test.go`
- Modify: `pkg/planetgen/types/types.go` (extend crater profile fields)

Replace the existing uniform-distribution generator with:
1. Power-law size-frequency distribution: `P(R) ∝ R^(-α)` with `α ≈ 2.0`.
2. Maria density mask: a low-freq fBm field reduces crater density where the mask is high (bright = young plains).
3. Per-crater age: beta-distributed in [0,1], biased by `SurfaceAge`.
4. Secondaries: each large crater (R > maxRadius/2) spawns 5–15 small craters within ~3R.

`Crater` struct gains: `Age float64`, `IsSecondary bool`, `ParentIdx int`. The render pass uses `Age` to fade depth (`depth *= age²` so young craters are sharper).

- [ ] **Step 1: Extend the type**

```go
// pkg/planetgen/types/types.go (extend PlanetProfile)
PowerLawAlpha       float64 // SFD slope (default 2.0; 0 = use uniform)
MariaDensityFactor  float64 // 0–1; reduce crater density in maria regions (0 = disabled)
SurfaceAge          float64 // 0–1; biases age distribution toward old (1) or young (0). Default 0.7.
SecondaryDensity    float64 // 0–1; density of secondaries per large crater. 0 = disabled.
```

- [ ] **Step 2: Write the test**

```go
// pkg/planetgen/feature/crater_rebuild_test.go
package feature

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestPowerLawSFD(t *testing.T) {
	prof := types.PlanetProfile{
		CraterCount: 1000,
		CraterMinRadius: 0.005, CraterMaxRadius: 0.05,
		PowerLawAlpha: 2.0,
	}
	craters := GenerateCraters(42, &prof)
	if len(craters) != 1000 {
		t.Fatalf("expected 1000 craters, got %d", len(craters))
	}
	// Power-law: many small, few large. Bin by radius and check ratio.
	smallCount, largeCount := 0, 0
	for _, c := range craters {
		if c.Radius < 0.015 { smallCount++ }
		if c.Radius > 0.04 { largeCount++ }
	}
	if smallCount < 5*largeCount {
		t.Errorf("expected smalls ≫ larges with α=2; small=%d large=%d", smallCount, largeCount)
	}
}

func TestSecondariesNearParent(t *testing.T) {
	prof := types.PlanetProfile{
		CraterCount: 50,
		CraterMinRadius: 0.005, CraterMaxRadius: 0.05,
		PowerLawAlpha: 2.0,
		SecondaryDensity: 0.5,
	}
	craters := GenerateCraters(7, &prof)
	hasSecondary := false
	for _, c := range craters {
		if c.IsSecondary {
			hasSecondary = true
			if c.ParentIdx < 0 || c.ParentIdx >= len(craters) {
				t.Errorf("secondary parent index out of range: %d", c.ParentIdx)
			}
		}
	}
	if !hasSecondary {
		t.Errorf("expected some secondaries with SecondaryDensity=0.5")
	}
}
```

- [ ] **Step 3: Implement**

Rewrite `GenerateCraters` (and extend the `Crater` struct in the same file) to honor the new fields. Sketch:

```go
type Crater struct {
	Lat, Lon    float64
	Radius      float64
	Depth       float64
	Age         float64
	IsSecondary bool
	ParentIdx   int
}

func GenerateCraters(seed int64, profile *types.PlanetProfile) []Crater {
	rng := noise.NewRand(seed)
	mariaNoise := noise.New(seed + 113)
	out := make([]Crater, 0, profile.CraterCount)
	alpha := profile.PowerLawAlpha
	if alpha <= 0 { alpha = 0 } // 0 falls back to uniform

	for i := 0; i < profile.CraterCount; i++ {
		lat, lon := uniformSphereLatLon(rng)
		// Maria mask: skip with probability MariaDensityFactor where mask > 0.
		if profile.MariaDensityFactor > 0 {
			x, y, z := latLonToXYZ(lat, lon)
			m := 0.5 + 0.5*mariaNoise.FractalNoise3D(x*1.5, y*1.5, z*1.5, 3, 2.0, 0.5, 1.0)
			if m > 1-profile.MariaDensityFactor && rng.Float64() < 0.7 { continue }
		}
		var r float64
		if alpha > 0 {
			// Inverse-CDF sampling of P(R) ∝ R^-α on [Rmin, Rmax].
			u := rng.Float64()
			rmin, rmax := profile.CraterMinRadius, profile.CraterMaxRadius
			if alpha == 1 {
				r = rmin * math.Pow(rmax/rmin, u)
			} else {
				one := 1 - alpha
				r = math.Pow(u*math.Pow(rmax, one)+(1-u)*math.Pow(rmin, one), 1/one)
			}
		} else {
			r = profile.CraterMinRadius + rng.Float64()*(profile.CraterMaxRadius-profile.CraterMinRadius)
		}
		age := betaSample(rng, profile.SurfaceAge)
		out = append(out, Crater{Lat: lat, Lon: lon, Radius: r, Depth: profile.CraterDepth, Age: age})
	}

	// Spawn secondaries.
	if profile.SecondaryDensity > 0 {
		nLarge := 0
		for i, c := range out {
			if c.Radius < profile.CraterMaxRadius*0.5 { continue }
			nLarge++
			n := 5 + int(profile.SecondaryDensity*10*rng.Float64())
			for j := 0; j < n; j++ {
				dLat := (rng.Float64()*2 - 1) * c.Radius * 3
				dLon := (rng.Float64()*2 - 1) * c.Radius * 3 / math.Cos(c.Lat)
				out = append(out, Crater{
					Lat: c.Lat + dLat, Lon: c.Lon + dLon,
					Radius: c.Radius * (0.05 + 0.15*rng.Float64()),
					Depth: c.Depth * 0.3,
					Age: c.Age * (0.6 + 0.4*rng.Float64()),
					IsSecondary: true, ParentIdx: i,
				})
			}
		}
	}
	return out
}

// helpers: uniformSphereLatLon, latLonToXYZ, betaSample
```

In `ApplyCraters`, scale depth by `Age * Age` so young craters look sharper than old ones:

```go
effectiveDepth := c.Depth * c.Age * c.Age
```

- [ ] **Step 4: Run tests + goldens**

Run: `go test ./pkg/planetgen/feature/ -v`
Expected: PASS.

Run: `go test ./cmd/generate-planet-maps/... -run TestGolden -v`
Expected: ΔE2000 will likely exceed 1.5 for `scorched` (the only archetype with high `CraterCount=200` shipped today). For each failure, regenerate the diff PNG, confirm the new craters look intentional (smaller-by-default, varied ages), then update the golden:

```bash
go test ./cmd/generate-planet-maps/... -run TestGolden -update
git diff cmd/generate-planet-maps/testdata/golden/  # confirm only expected files changed
```

- [ ] **Step 5: Commit**

```bash
git add pkg/planetgen/feature/crater.go pkg/planetgen/feature/crater_rebuild_test.go pkg/planetgen/types/types.go cmd/generate-planet-maps/testdata/golden/
git commit -m "Crater system rebuild: power-law SFD, age, maria mask, secondaries"
```

---

## Task 8: Ejecta-ray albedo overlay

**Files:**
- Create: `pkg/planetgen/feature/crater_ejecta.go`
- Modify: `pkg/planetgen/render/rocky.go` (apply ejecta in the post-color pass, before LUT)

Ejecta is a per-pixel albedo brightening near recent craters. Brightness = `(angularSin pattern) × radialFalloff(distFromRim) × age²`. Output is a lightening overlay (multiplied with the existing color) so it darkens with age.

- [ ] **Step 1: Implement**

```go
// pkg/planetgen/feature/crater_ejecta.go
package feature

import (
	"image/color"
	"math"
)

// ApplyEjecta brightens c near recent craters in proportion to age² with
// an angular sin pattern (rays). dx,dy,dz is the unit-sphere direction
// of the pixel; craters carry lat/lon already.
func ApplyEjecta(base color.RGBA, dx, dy, dz float64, craters []Crater) color.RGBA {
	bright := 0.0
	for _, cr := range craters {
		// only fresh, large enough craters cast visible rays
		if cr.IsSecondary || cr.Age < 0.3 || cr.Radius < 0.01 { continue }
		// great-circle angular distance
		cx := math.Cos(cr.Lat) * math.Cos(cr.Lon)
		cy := math.Sin(cr.Lat)
		cz := math.Cos(cr.Lat) * math.Sin(cr.Lon)
		dot := dx*cx + dy*cy + dz*cz
		if dot < 0 { continue }
		ang := math.Acos(dot)
		rOuter := cr.Radius * 6
		if ang > rOuter { continue }
		// radial falloff
		t := 1 - (ang-cr.Radius)/(rOuter-cr.Radius)
		if t <= 0 { continue }
		// angular ray pattern (8 rays)
		theta := math.Atan2(dy*cz-dz*cy, dx*cx+dy*cy+dz*cz)
		ray := 0.5 + 0.5*math.Cos(8*theta)
		bright += t * t * ray * cr.Age * cr.Age * 0.4
	}
	if bright <= 0 { return base }
	if bright > 1 { bright = 1 }
	r := uint8(math.Min(255, float64(base.R)+bright*60))
	g := uint8(math.Min(255, float64(base.G)+bright*60))
	b := uint8(math.Min(255, float64(base.B)+bright*60))
	return color.RGBA{R: r, G: g, B: b, A: base.A}
}
```

- [ ] **Step 2: Wire in**

In `RenderRocky`, after biome/palette coloring, before LUT:

```go
if len(craters) > 0 {
	c = feature.ApplyEjecta(c, dx, dy, dz, craters)
}
```

- [ ] **Step 3: Goldens**

Run: `go test ./cmd/generate-planet-maps/... -run TestGolden -v`
Expected: noticeable brightening on scorched / lava_world; update goldens with `-update` and inspect diffs.

- [ ] **Step 4: Commit**

```bash
git add pkg/planetgen/feature/crater_ejecta.go pkg/planetgen/render/rocky.go cmd/generate-planet-maps/testdata/golden/
git commit -m "Add ejecta-ray albedo overlay around fresh craters"
```

---

## Task 9: Crater system slider panel

**Files:**
- Modify: `cmd/planet-explorer/web/app.js` (extend the existing `renderCratersPanel`)

Add the four new fields (`PowerLawAlpha`, `MariaDensityFactor`, `SurfaceAge`, `SecondaryDensity`) to the existing Craters panel. Keep Randomize/Reset/Clear; expand the randomize ranges.

- [ ] **Step 1: Extend the panel**

After the existing four crater rows in `renderCratersPanel`, add:

```js
panel.appendChild(makeNumberRow('PowerLawAlpha',
  'SFD slope. 2.0 = realistic many-small/few-large; 0 = uniform distribution.',
  profile.PowerLawAlpha || 0, 0, 4, '0.1',
  v => { profile.PowerLawAlpha = v; commitProfile(profile); }));
panel.appendChild(makeNumberRow('MariaDensityFactor',
  'Strength of the maria mask (low-freq fBm regions where craters are suppressed). 0 = disabled; 1 = strong contrast.',
  profile.MariaDensityFactor || 0, 0, 1, '0.05',
  v => { profile.MariaDensityFactor = v; commitProfile(profile); }));
panel.appendChild(makeNumberRow('SurfaceAge',
  'Bias age distribution. 0 = all young (sharp); 1 = all old (faded). Default 0.7.',
  profile.SurfaceAge || 0, 0, 1, '0.05',
  v => { profile.SurfaceAge = v; commitProfile(profile); }));
panel.appendChild(makeNumberRow('SecondaryDensity',
  'Density of small secondary craters per large parent. 0 = disabled.',
  profile.SecondaryDensity || 0, 0, 1, '0.05',
  v => { profile.SecondaryDensity = v; commitProfile(profile); }));
```

Update Randomize:

```js
const randomize = () => {
  // ... existing four ...
  profile.PowerLawAlpha     = round2(1.5 + Math.random() * 1);   // 1.5–2.5
  profile.MariaDensityFactor = round2(Math.random() * 0.6);
  profile.SurfaceAge        = round2(0.3 + Math.random() * 0.6); // 0.3–0.9
  profile.SecondaryDensity  = round2(Math.random() * 0.5);
  commitProfile(profile);
  renderPanels();
};
```

Same for Clear (zero out all eight) and Reset (mirror originalProfile).

- [ ] **Step 2: Smoke test**

Type=scorched. Crank `PowerLawAlpha=2.5` → mostly small craters. Set `MariaDensityFactor=0.7` → smooth bands appear. Set `SurfaceAge=0.2` → craters look fresh, ejecta rays prominent. Set `SecondaryDensity=0.5` → clusters of small craters around large ones.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/web/app.js
git commit -m "Extend craters panel with power-law/maria/age/secondaries"
```

---

## Task 10: Acceptance + per-archetype defaults

**Files:**
- Modify: `pkg/planetgen/profile.go` (set sensible defaults for archetypes that need it)
- Modify: `cmd/generate-planet-maps/README.md` (Phase 3 section)

- [ ] **Step 1: Set archetype defaults**

For the rocky archetypes the user has tuned (terran, arid, scorched, lava_world, super_terran, oceanic), pick reasonable starting values:
- All: `PowerLawAlpha: 2.0`, `SurfaceAge: 0.7`.
- scorched: `MariaDensityFactor: 0.4`, `SecondaryDensity: 0.3` (mercurial).
- lava_world: `MariaDensityFactor: 0.6` (much lava-resurfaced), `SurfaceAge: 0.3` (young surface).
- terran/super_terran/oceanic: `MariaDensityFactor: 0`, `SurfaceAge: 0.7`.

Add to each profile entry:

```go
PowerLawAlpha:      2.0,
MariaDensityFactor: <archetype value>,
SurfaceAge:         <archetype value>,
SecondaryDensity:   <archetype value>,
```

- [ ] **Step 2: Goldens**

Run: `go test ./cmd/generate-planet-maps/... -run TestGolden -v`
Expected: ΔE2000 will likely exceed 1.5 for the rocky archetypes due to the new crater system. For each, inspect the diff and confirm the new look is intended, then `-update`.

- [ ] **Step 3: Wasm + lint gates**

Run:
- `(cd cmd/planet-explorer && GOOS=js GOARCH=wasm go build -o web/planet-explorer.wasm ./wasm)`
- `golangci-lint run ./pkg/planetgen/... ./cmd/planet-explorer/...`

Expected: 0 issues, ~5-6 MB wasm.

- [ ] **Step 4: Phase 3 README section**

Add a "Phase 3 (current)" section to `cmd/generate-planet-maps/README.md` listing the three new algorithms and their profile fields (mirror Phase 1's section style).

- [ ] **Step 5: Final commit**

```bash
git add pkg/planetgen/profile.go cmd/generate-planet-maps/README.md cmd/generate-planet-maps/testdata/golden/
git commit -m "Phase 3 defaults + README + golden refresh"
```

---

## Self-review notes

- All three algorithms gate on a profile field being non-zero — every existing archetype that hasn't been re-tuned yet renders byte-identical (until Task 10's defaults land).
- Province modulation deliberately multiplies *both* spline input and output for symmetry; if it ends up making the planet feel too patchwork, halve `Jitter` rather than restructure.
- Crater rebuild deliberately doesn't touch `ApplyCraters`'s height-stamping math beyond the `age²` depth scale — keeping the existing seam-aware crater stamp intact.
- JFA / hydraulic erosion / coastal noise are explicitly Phase 4 (bucket B). Multi-ring basins, catena, ejecta darkening below older craters, and rim discoloration are Phase 4+ polish.
- Expect goldens to need `-update` for at least scorched, lava_world, and super_terran. The `cmd/tools/planet-image-diff` tool is the canonical reviewer for those diffs.
