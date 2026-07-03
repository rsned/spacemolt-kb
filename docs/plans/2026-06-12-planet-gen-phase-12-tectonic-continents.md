# Planet Gen Phase 12 — Tectonic Continents (Crust Rafts) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make plates *cause* continents: land/ocean comes from a new crust-raft stage (cratons riding plates → ContinentalMask + BaseHeight), mountain belts / trenches / arcs / ridges / rifts come from crust-aware boundary effects scaled by a TectonicAge knob, and sea level is derived from a TargetLandFraction quantile.

**Architecture:** A new `field.CrustField` is generated after `field.GeneratePlates` and before the control-field height sum. When `profile.Crust.MajorPlates > 0` ("crust path"): plates use two-tier seeding (major + minor filler) with growth-weighted flood fill; the heightmap is initialized from crust BaseHeight; the Continentalness spline contribution, legacy Ridged, Basin, and Continents passes are skipped; a new TectonicFX pass adds the six boundary effects; after Normalize the ocean level is derived by histogram quantile and threaded to all downstream consumers. Zero-value `CrustConfig` keeps every existing planet byte-identical.

**Tech Stack:** Go 1.24, existing `pkg/planetgen` subpackages (cubemap, noise, field, render, types, seed, profilejson), existing JFA (`JumpFloodFromMaskWithValue`), planet-explorer plain-JS UI. No new dependencies. All code must pass `golangci-lint` and build for `GOOS=js GOARCH=wasm`.

**Spec:** `docs/superpowers/specs/2026-06-12-tectonic-continents-design.md`

**Worktree:** all paths are relative to `/home/robert/spacemolt/kb-phase-12-tectonic-continents` (branch `phase-12/tectonic-continents`, branched from `phase-0/cube-map`). Run all commands from that directory.

---

## File structure

**New files:**

```
pkg/planetgen/field/
├── crust.go              (Craton, CrustField, ResolveCrustParams, PlaceCratons, GenerateCrust, math helpers)
├── crust_test.go
├── crust_seam_test.go    (seam continuity for mask/base via seamtest)
├── tectonicfx.go         (TectonicFXField, ClassifyTectonics, ApplyTectonicFX)
├── tectonicfx_test.go
├── sealevel.go           (QuantileSeaLevel)
└── sealevel_test.go
```

**Modified files:**

| File | Change |
|---|---|
| `pkg/planetgen/types/types.go` | add `CrustConfig`, `TectonicFXConfig`; add `Crust`, `TectonicFX` fields to `PlanetProfile` |
| `pkg/planetgen/field/plates.go` | `Plate.Weight`; `seedPlatesTwoTier`; weighted flood fill; `BoundaryPixel` lists on `PlateField`; `balanceMomentum` helper |
| `pkg/planetgen/render/rocky.go` | crust path in `generateRockyHeightmapDebug`; derived ocean level; signature change (returns ocean level); caller updates |
| `pkg/planetgen/render/debug.go` | caller update + profile copy with derived ocean level |
| `pkg/planetgen/profile.go` | crust + FX defaults for terran, super_terran, oceanic, tundra, glacial, arid |
| `cmd/generate-planet-maps/invariants_test.go` | Phase 12 invariants (land fraction, largest landmass, island ceiling) |
| `cmd/planet-explorer/web/app.js` | `renderTectonicsPanel`, `renderTectonicFXPanel` |
| `cmd/generate-planet-maps/README.md`, `cmd/planet-explorer/USER_GUIDE.md` | docs |
| `cmd/generate-planet-maps/testdata/golden/*` | regenerated goldens for the six types |

**Deliberately NOT changed:** `pkg/planetgen/profilejson/envelope.go`. The envelope doc-comment states the schema version bumps only when the *envelope wire format* changes; the inner PlanetProfile evolves via struct-tag additions. Older v1 JSONs without the `crust`/`tectonicFX` keys still load and render identically (zero-value = legacy path). Serialization, however, did change: the three non-`omitempty` sentinel fields mean a zero-value `CrustConfig` still emits `assembly`/`assemblyWeights`/`targetLandFraction`/`tectonicAge` at zero, so reseeded `handTuned: false` fixtures gain the `crust`/`tectonicFX` keys at zero values (the drift test enforces this). The three default fixtures were reseeded in Task 1 rather than Task 14. This deviates from the spec's "schema version bump" line but satisfies its intent (hand-tuned planets unchanged until opted in).

## Conventions

- **Named seed domains** (master plan §3.3): every new RNG/noise stream uses `seed.Domain(master, "<name>")`. New domains in this phase: `crust.params`, `crust.params.stream`, `crust.cratons`, `crust.cratons.stream`, `crust.edge`, `tectonicfx.activity`, `tectonicfx.belt`, `plates.seeds.minor` (jitter only).
- **Disable idiom:** `Crust.MajorPlates == 0` disables the whole phase (like `Cloud.Coverage == 0`).
- **Sentinels:** `Crust.Assembly`, `Crust.TargetLandFraction`, `Crust.TectonicAge` use `-1` = "sample deterministically from the configured range/weights"; an explicit value in range pins it. These three fields are serialized WITHOUT `omitempty` so both `-1` and `0` round-trip.
- **TDD:** every task writes the failing test first. Run `go build ./... && go test ./pkg/planetgen/...` before each commit; run `golangci-lint run ./pkg/planetgen/... ./cmd/...` after each series of changes.
- Each task ends with one commit. Commit style: `P12: <imperative summary>` (matches `P9b: ...` precedent in git log).

---

## Task 1: CrustConfig + TectonicFXConfig types

**Files:**
- Modify: `pkg/planetgen/types/types.go`
- Test: `pkg/planetgen/types/types_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/planetgen/types/types_test.go`:

```go
func TestCrustConfigJSONRoundTrip(t *testing.T) {
	in := CrustConfig{
		MajorPlates: 7, MinorPlates: 4, MajorGrowthBias: 4,
		OceanicFraction: 0.45,
		Assembly:        -1, AssemblyWeights: [3]float64{25, 65, 10},
		TargetLandFraction: -1, LandFracLo: 0.22, LandFracHi: 0.38,
		TectonicAge: -1, AgeLo: 0.25, AgeHi: 0.75,
		CratonsMax: 8, ShelfWidthRad: 0.05,
		EdgeNoiseAmp: 0.45, EdgeNoiseFreq: 2.2, EdgeNoiseOctaves: 4,
		PlatformHeight: 0.62, OceanFloorHeight: 0.25,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out CrustConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestCrustSentinelsSurviveZeroAndNegOne(t *testing.T) {
	// Assembly / TargetLandFraction / TectonicAge must round-trip both
	// 0 (pinned supercontinent) and -1 (sample) — they are not omitempty.
	for _, v := range []float64{0, -1} {
		in := CrustConfig{MajorPlates: 5, Assembly: v, TargetLandFraction: v, TectonicAge: v}
		data, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out CrustConfig
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.Assembly != v || out.TargetLandFraction != v || out.TectonicAge != v {
			t.Errorf("sentinel %v did not round-trip: %+v", v, out)
		}
	}
}

func TestTectonicFXConfigJSONRoundTrip(t *testing.T) {
	in := TectonicFXConfig{
		BeltAmp: 0.3, BeltWidthKm: 900, BeltFreq: 3.2, BeltOctaves: 5,
		CordAmp: 0.22, CordWidthKm: 450,
		TrenchDepth: 0.12, TrenchWidthKm: 220,
		ArcAmp: 0.25, ArcWidthKm: 260,
		RidgeAmp: 0.06, RidgeWidthKm: 700,
		RiftDepth: 0.1, RiftWidthKm: 280, RiftShoulder: 0.35,
		TransformAmp: 0.03, TransformWidthKm: 150,
		ActivityFreq: 1.5,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out TectonicFXConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}
```

If `types_test.go` does not already import `encoding/json`, add it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/types/...`
Expected: FAIL with `undefined: CrustConfig` / `undefined: TectonicFXConfig`.

- [ ] **Step 3: Add the types**

In `pkg/planetgen/types/types.go`, append after the `RainShadowConfig` declaration:

```go
// CrustConfig (Phase 12) parameterizes the crust-raft stage: two-tier
// plate seeding, craton placement, and the continental mask + base
// height it produces. MajorPlates == 0 disables the entire crust
// pipeline (zero-value = legacy path: PlateCount, Continents, Basin,
// and Ridged behave exactly as before).
//
// Assembly, TargetLandFraction, and TectonicAge use a -1 sentinel
// meaning "sample deterministically from the configured range/weights
// using the master seed"; any in-range value pins the parameter. They
// are serialized without omitempty so 0 and -1 both round-trip.
type CrustConfig struct {
	MajorPlates     int     `json:"majorPlates,omitempty"`
	MinorPlates     int     `json:"minorPlates,omitempty"`
	MajorGrowthBias float64 `json:"majorGrowthBias,omitempty"` // flood-fill growth weight for majors; default 4
	OceanicFraction float64 `json:"oceanicFraction,omitempty"` // fraction of plates carrying no craton

	Assembly        float64    `json:"assembly"`                  // -1 = sample; 0 supercontinent … 1 fragmented
	AssemblyWeights [3]float64 `json:"assemblyWeights,omitempty"` // sampling weights: super / dispersed / fragmented

	TargetLandFraction float64 `json:"targetLandFraction"` // -1 = sample uniform [LandFracLo, LandFracHi]
	LandFracLo         float64 `json:"landFracLo,omitempty"`
	LandFracHi         float64 `json:"landFracHi,omitempty"`

	TectonicAge float64 `json:"tectonicAge"` // -1 = sample uniform [AgeLo, AgeHi]; 0 young/sharp … 1 old/soft
	AgeLo       float64 `json:"ageLo,omitempty"`
	AgeHi       float64 `json:"ageHi,omitempty"`

	CratonsMax       int     `json:"cratonsMax,omitempty"`       // total craton cap; default 8
	ShelfWidthRad    float64 `json:"shelfWidthRad,omitempty"`    // continental-shelf falloff half-width (radians); default 0.05
	EdgeNoiseAmp     float64 `json:"edgeNoiseAmp,omitempty"`     // craton-edge radius modulation fraction; default 0.45
	EdgeNoiseFreq    float64 `json:"edgeNoiseFreq,omitempty"`    // default 2.2
	EdgeNoiseOctaves int     `json:"edgeNoiseOctaves,omitempty"` // default 4
	PlatformHeight   float64 `json:"platformHeight,omitempty"`   // continental platform base height; default 0.62
	OceanFloorHeight float64 `json:"oceanFloorHeight,omitempty"` // abyssal base height; default 0.25
}

// TectonicFXConfig (Phase 12) parameterizes the crust-aware boundary
// height effects. Each effect disables individually at zero amplitude;
// the whole pass runs only on the crust path (Crust.MajorPlates > 0).
// Widths are envelope length scales in km (Gaussian sigma).
type TectonicFXConfig struct {
	BeltAmp     float64 `json:"beltAmp,omitempty"` // cont-cont collision belt (Himalayas)
	BeltWidthKm float64 `json:"beltWidthKm,omitempty"`
	BeltFreq    float64 `json:"beltFreq,omitempty"` // ridged noise frequency inside the belt
	BeltOctaves int     `json:"beltOctaves,omitempty"`

	CordAmp     float64 `json:"cordAmp,omitempty"` // ocean-cont coastal cordillera (Andes)
	CordWidthKm float64 `json:"cordWidthKm,omitempty"`

	TrenchDepth   float64 `json:"trenchDepth,omitempty"` // subduction trench (ocean side)
	TrenchWidthKm float64 `json:"trenchWidthKm,omitempty"`

	ArcAmp     float64 `json:"arcAmp,omitempty"` // oce-oce volcanic island arc (Japan)
	ArcWidthKm float64 `json:"arcWidthKm,omitempty"`

	RidgeAmp     float64 `json:"ridgeAmp,omitempty"` // mid-ocean ridge bathymetric rise
	RidgeWidthKm float64 `json:"ridgeWidthKm,omitempty"`

	RiftDepth    float64 `json:"riftDepth,omitempty"` // continental rift valley floor
	RiftWidthKm  float64 `json:"riftWidthKm,omitempty"`
	RiftShoulder float64 `json:"riftShoulder,omitempty"` // shoulder uplift as fraction of RiftDepth

	TransformAmp     float64 `json:"transformAmp,omitempty"` // fault-zone roughness
	TransformWidthKm float64 `json:"transformWidthKm,omitempty"`

	ActivityFreq float64 `json:"activityFreq,omitempty"` // per-boundary activity noise frequency; default 1.5
}
```

Then add two fields to `PlanetProfile`, after the `Civ CivConfig` field:

```go
	// Phase 12: crust-raft tectonic continents. MajorPlates == 0
	// disables (legacy land/ocean from Continentalness noise).
	Crust CrustConfig `json:"crust,omitempty"`

	// Phase 12: crust-aware boundary height effects (belts, trenches,
	// arcs, ridges, rifts, faults). Only applied on the crust path.
	TectonicFX TectonicFXConfig `json:"tectonicFX,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/planetgen/types/...`
Expected: PASS.

- [ ] **Step 5: Build + lint + commit**

```bash
go build ./... && go test ./pkg/planetgen/... && golangci-lint run ./pkg/planetgen/types/...
git add pkg/planetgen/types/types.go pkg/planetgen/types/types_test.go
git commit -m "P12: add CrustConfig + TectonicFXConfig profile types"
```

---

## Task 2: Crust parameter resolution (sampling with sentinels)

**Files:**
- Create: `pkg/planetgen/field/crust.go`
- Create: `pkg/planetgen/field/crust_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/field/crust_test.go`:

```go
package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func terranCrustCfg() types.CrustConfig {
	return types.CrustConfig{
		MajorPlates: 7, MinorPlates: 4, MajorGrowthBias: 4,
		OceanicFraction: 0.45,
		Assembly:        -1, AssemblyWeights: [3]float64{25, 65, 10},
		TargetLandFraction: -1, LandFracLo: 0.22, LandFracHi: 0.38,
		TectonicAge: -1, AgeLo: 0.25, AgeHi: 0.75,
		CratonsMax: 8, ShelfWidthRad: 0.05,
		EdgeNoiseAmp: 0.45, EdgeNoiseFreq: 2.2, EdgeNoiseOctaves: 4,
		PlatformHeight: 0.62, OceanFloorHeight: 0.25,
	}
}

func TestResolveCrustParamsDeterministic(t *testing.T) {
	cfg := terranCrustCfg()
	a1, l1, g1 := ResolveCrustParams(cfg, 12345)
	a2, l2, g2 := ResolveCrustParams(cfg, 12345)
	if a1 != a2 || l1 != l2 || g1 != g2 {
		t.Fatalf("not deterministic: (%v,%v,%v) vs (%v,%v,%v)", a1, l1, g1, a2, l2, g2)
	}
}

func TestResolveCrustParamsRanges(t *testing.T) {
	cfg := terranCrustCfg()
	for master := int64(0); master < 200; master++ {
		a, l, g := ResolveCrustParams(cfg, master)
		if a < 0 || a > 1 {
			t.Fatalf("seed %d: assembly %v out of [0,1]", master, a)
		}
		if l < cfg.LandFracLo || l > cfg.LandFracHi {
			t.Fatalf("seed %d: landFrac %v out of [%v,%v]", master, l, cfg.LandFracLo, cfg.LandFracHi)
		}
		if g < cfg.AgeLo || g > cfg.AgeHi {
			t.Fatalf("seed %d: age %v out of [%v,%v]", master, g, cfg.AgeLo, cfg.AgeHi)
		}
	}
}

func TestResolveCrustParamsPinned(t *testing.T) {
	cfg := terranCrustCfg()
	cfg.Assembly = 0.0 // pinned supercontinent — 0 is a valid pin, not "unset"
	cfg.TargetLandFraction = 0.3
	cfg.TectonicAge = 0.9
	a, l, g := ResolveCrustParams(cfg, 777)
	if a != 0.0 || l != 0.3 || g != 0.9 {
		t.Fatalf("pin ignored: got (%v,%v,%v)", a, l, g)
	}
}

func TestResolveCrustParamsAssemblyDistribution(t *testing.T) {
	// Weights 25/65/10 → over many seeds the band shares should be
	// roughly proportional (loose tolerance: ±10 points).
	cfg := terranCrustCfg()
	var bands [3]int
	const n = 2000
	for master := int64(0); master < n; master++ {
		a, _, _ := ResolveCrustParams(cfg, master)
		switch {
		case a < 0.33:
			bands[0]++
		case a < 0.67:
			bands[1]++
		default:
			bands[2]++
		}
	}
	want := [3]float64{0.25, 0.65, 0.10}
	for i := range bands {
		got := float64(bands[i]) / n
		if math.Abs(got-want[i]) > 0.10 {
			t.Errorf("band %d share %v, want %v ± 0.10", i, got, want[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/field/ -run TestResolveCrustParams`
Expected: FAIL with `undefined: ResolveCrustParams`.

- [ ] **Step 3: Implement**

Create `pkg/planetgen/field/crust.go`:

```go
// Crust-raft stage (Phase 12): cratons of continental crust riding on
// tectonic plates produce a ContinentalMask and BaseHeight that decide
// land vs ocean, replacing noise-threshold continents on the crust path.
package field

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Craton is one raft of continental crust riding a plate.
type Craton struct {
	Center  [3]float64 // unit direction
	Radius  float64    // angular radius (radians) before edge noise
	PlateID int
}

// CrustField is the per-pixel output of the crust stage plus the
// resolved per-planet tectonic parameters.
type CrustField struct {
	Size            int
	ContinentalMask *cubemap.CubeMapF // 0..1 continental-crust fraction
	BaseHeight      *cubemap.CubeMapF // isostatic base height
	Cratons         []Craton
	Assembly        float64
	LandFraction    float64
	TectonicAge     float64
}

// ResolveCrustParams resolves the three sampled-or-pinned tectonic
// parameters. A -1 sentinel samples deterministically from the
// configured range/weights using named seed domains; any other value
// pins. All draws happen on the "crust.params" domain, in a fixed
// order (assembly, landFrac, age), and pinned parameters still consume
// their draws so pinning one parameter never shifts the others.
func ResolveCrustParams(cfg types.CrustConfig, master int64) (assembly, landFrac, age float64) {
	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "crust.params")),        //nolint:gosec
		uint64(seed.Domain(master, "crust.params.stream")), //nolint:gosec
	))

	// Assembly: weighted band, then uniform within the band.
	w := cfg.AssemblyWeights
	if w[0]+w[1]+w[2] <= 0 {
		w = [3]float64{25, 65, 10}
	}
	u := rng.Float64() * (w[0] + w[1] + w[2])
	v := rng.Float64()
	sampledAssembly := 0.67 + v*0.33
	switch {
	case u < w[0]:
		sampledAssembly = v * 0.33
	case u < w[0]+w[1]:
		sampledAssembly = 0.33 + v*0.34
	}
	assembly = cfg.Assembly
	if assembly < 0 {
		assembly = sampledAssembly
	}

	lo, hi := cfg.LandFracLo, cfg.LandFracHi
	if hi <= lo {
		lo, hi = 0.22, 0.38
	}
	sampledLand := lo + rng.Float64()*(hi-lo)
	landFrac = cfg.TargetLandFraction
	if landFrac < 0 {
		landFrac = sampledLand
	}

	alo, ahi := cfg.AgeLo, cfg.AgeHi
	if ahi <= alo {
		alo, ahi = 0.2, 0.8
	}
	sampledAge := alo + rng.Float64()*(ahi-alo)
	age = cfg.TectonicAge
	if age < 0 {
		age = sampledAge
	}
	return assembly, landFrac, age
}

func dot3(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func norm3(a [3]float64) [3]float64 {
	m := math.Sqrt(dot3(a, a))
	if m < 1e-12 {
		return [3]float64{1, 0, 0}
	}
	return [3]float64{a[0] / m, a[1] / m, a[2] / m}
}

// slerp3 spherically interpolates between unit vectors a and b.
func slerp3(a, b [3]float64, t float64) [3]float64 {
	d := dot3(a, b)
	if d > 1 {
		d = 1
	}
	if d < -1 {
		d = -1
	}
	th := math.Acos(d)
	if th < 1e-9 {
		return a
	}
	sa := math.Sin((1-t)*th) / math.Sin(th)
	sb := math.Sin(t*th) / math.Sin(th)
	return norm3([3]float64{
		a[0]*sa + b[0]*sb,
		a[1]*sa + b[1]*sb,
		a[2]*sa + b[2]*sb,
	})
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func smoothstep(lo, hi, x float64) float64 {
	if hi <= lo {
		if x < lo {
			return 0
		}
		return 1
	}
	t := clamp01((x - lo) / (hi - lo))
	return t * t * (3 - 2*t)
}
```

Note: `noise` is imported but unused until Task 5 — to keep this task compiling, omit the `noise` import here and add it in Task 5. The import block for this task is:

```go
import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)
```

(`cubemap` IS used — by the `CrustField` struct fields.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/planetgen/field/ -run TestResolveCrustParams`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
go build ./... && golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/crust.go pkg/planetgen/field/crust_test.go
git commit -m "P12: crust param resolution with -1 sample sentinels"
```

---

## Task 3: Two-tier plate seeding + weighted flood fill

**Files:**
- Modify: `pkg/planetgen/field/plates.go`
- Modify: `pkg/planetgen/field/plates_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/planetgen/field/plates_test.go`:

```go
func crustProfile() *types.PlanetProfile {
	return &types.PlanetProfile{
		PlateConvergentT: 0.75,
		Crust: types.CrustConfig{
			MajorPlates: 6, MinorPlates: 4, MajorGrowthBias: 4,
			OceanicFraction: 0.45,
		},
	}
}

func TestTwoTierPlateCountAndWeights(t *testing.T) {
	pf := GeneratePlates(crustProfile(), 42, 64)
	if pf == nil {
		t.Fatal("nil PlateField")
	}
	if len(pf.Plates) != 10 {
		t.Fatalf("got %d plates, want 10 (6 major + 4 minor)", len(pf.Plates))
	}
	for i, p := range pf.Plates {
		want := 4.0
		if i >= 6 {
			want = 1.0
		}
		if p.Weight != want {
			t.Errorf("plate %d weight %v, want %v", i, p.Weight, want)
		}
	}
}

func TestTwoTierMajorsClaimMoreArea(t *testing.T) {
	const S = 64
	pf := GeneratePlates(crustProfile(), 42, S)
	areas := make([]int, len(pf.Plates))
	for f := range pf.PlateID {
		for _, id := range pf.PlateID[f] {
			areas[id]++
		}
	}
	var majorSum, minorSum int
	for i, a := range areas {
		if i < 6 {
			majorSum += a
		} else {
			minorSum += a
		}
	}
	majorMean := float64(majorSum) / 6
	minorMean := float64(minorSum) / 4
	if majorMean < 2*minorMean {
		t.Errorf("major mean area %v not ≥ 2× minor mean %v", majorMean, minorMean)
	}
}

func TestTwoTierEveryPixelAssigned(t *testing.T) {
	const S = 64
	pf := GeneratePlates(crustProfile(), 7, S)
	for f := range pf.PlateID {
		for i, id := range pf.PlateID[f] {
			if id < 0 || int(id) >= len(pf.Plates) {
				t.Fatalf("face %d pixel %d has invalid plate id %d", f, i, id)
			}
		}
	}
}

func TestLegacyPathUnchangedByCrustCode(t *testing.T) {
	// A profile with PlateCount but zero Crust must produce the same
	// plates as before this phase: weights all 1, count == PlateCount.
	p := &types.PlanetProfile{PlateCount: 8, OceanicPlateFraction: 0.5, PlateConvergentT: 0.75}
	pf := GeneratePlates(p, 99, 64)
	if len(pf.Plates) != 8 {
		t.Fatalf("legacy plate count %d, want 8", len(pf.Plates))
	}
	for i, pl := range pf.Plates {
		if pl.Weight != 1 {
			t.Errorf("legacy plate %d weight %v, want 1", i, pl.Weight)
		}
	}
}
```

(If `plates_test.go` lacks the `types` import, add it; it almost certainly has it already.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/field/ -run 'TestTwoTier|TestLegacyPathUnchangedByCrustCode'`
Expected: FAIL — `Plate` has no `Weight` field, and a Crust-only profile (PlateCount=0) returns nil.

- [ ] **Step 3: Implement**

In `pkg/planetgen/field/plates.go`:

**(a)** Add `Weight` to `Plate` (after `IsOceanic bool`):

```go
	// Weight is the flood-fill growth bias (Phase 12 two-tier seeding):
	// frontier candidates from a plate are pushed Weight times, so a
	// weight-4 major plate expands ~4× faster than a weight-1 minor.
	// Legacy single-tier seeding sets Weight = 1 on every plate.
	Weight float64
```

**(b)** In `seedPlates`, set `plates[i].Weight = 1` right after `plates[i].ID = i`.

**(c)** Extract the existing sum-to-zero block at the end of `seedPlates` (from `// Sum-to-zero angular momentum` through the final `for` loop) into a new function, and call it from `seedPlates`:

```go
// balanceMomentum subtracts the mean angular-velocity vector from every
// plate so the lithosphere has no net spin (see seedPlates for why).
func balanceMomentum(plates []Plate) {
	n := len(plates)
	var mx, my, mz float64
	for i := range plates {
		mx += plates[i].RotAxis[0] * plates[i].AngSpeed
		my += plates[i].RotAxis[1] * plates[i].AngSpeed
		mz += plates[i].RotAxis[2] * plates[i].AngSpeed
	}
	mx /= float64(n)
	my /= float64(n)
	mz /= float64(n)
	for i := range plates {
		wx := plates[i].RotAxis[0]*plates[i].AngSpeed - mx
		wy := plates[i].RotAxis[1]*plates[i].AngSpeed - my
		wz := plates[i].RotAxis[2]*plates[i].AngSpeed - mz
		mag := math.Sqrt(wx*wx + wy*wy + wz*wz)
		if mag < 1e-12 {
			plates[i].AngSpeed = 0
			continue
		}
		plates[i].RotAxis = [3]float64{wx / mag, wy / mag, wz / mag}
		plates[i].AngSpeed = mag
	}
}
```

The body of `seedPlates` after the per-plate loop becomes just `balanceMomentum(plates)` followed by `return plates`. This is a pure extraction — legacy byte-identity is verified by the existing plate tests and the golden suite.

**(d)** Add the two-tier seeder:

```go
// seedPlatesTwoTier (Phase 12) seeds MajorPlates spiral seeds with
// growth weight MajorGrowthBias plus MinorPlates gap-filler seeds with
// weight 1, placed by farthest-point sampling over a 64-candidate
// Fibonacci pool. Motion and momentum balancing match seedPlates.
// Oceanic flags use Crust.OceanicFraction ("carries no craton").
func seedPlatesTwoTier(profile *types.PlanetProfile, master int64) []Plate {
	nMaj := profile.Crust.MajorPlates
	nMin := profile.Crust.MinorPlates
	if nMaj <= 0 {
		return nil
	}
	bias := profile.Crust.MajorGrowthBias
	if bias <= 0 {
		bias = 4
	}
	total := nMaj + nMin
	plates := make([]Plate, 0, total)

	rngSeed := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.seeds")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.seeds.stream")), //nolint:gosec
	))
	rngMotion := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.motion")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.motion.stream")), //nolint:gosec
	))
	rngOceanic := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.oceanic")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.oceanic.stream")), //nolint:gosec
	))

	goldenAngle := math.Pi * (3.0 - math.Sqrt(5))
	spiral := func(i, n int) [3]float64 {
		y := 1.0 - 2.0*(float64(i)+0.5)/float64(n)
		radius := math.Sqrt(1.0 - y*y)
		theta := goldenAngle * float64(i)
		jitter := (rngSeed.Float64() - 0.5) * (math.Pi / 30)
		return [3]float64{math.Cos(theta+jitter) * radius, y, math.Sin(theta+jitter) * radius}
	}

	// Majors: spiral over nMaj.
	for i := range nMaj {
		plates = append(plates, Plate{ID: i, Seed: spiral(i, nMaj), Weight: bias})
	}

	// Minors: farthest-point picks from a 64-candidate spiral pool.
	const poolN = 64
	pool := make([][3]float64, poolN)
	for i := range poolN {
		pool[i] = spiral(i, poolN)
	}
	for m := range nMin {
		bestIdx, bestScore := 0, -2.0
		for ci, c := range pool {
			worst := 2.0 // min dot to any existing seed = farthest candidate
			for _, p := range plates {
				if d := dot3(p.Seed, c); d < worst {
					worst = d
				}
			}
			// Lower min-dot = farther from everything = better. Negate.
			if score := -worst; score > bestScore {
				bestScore = score
				bestIdx = ci
			}
		}
		plates = append(plates, Plate{ID: nMaj + m, Seed: pool[bestIdx], Weight: 1})
		// Remove the chosen candidate so the next minor picks elsewhere.
		pool[bestIdx] = pool[len(pool)-1]
		pool = pool[:len(pool)-1]
	}

	// Motion + oceanic flags, in plate order (matches seedPlates idiom).
	for i := range plates {
		for {
			a := 2*rngMotion.Float64() - 1
			b := 2*rngMotion.Float64() - 1
			s := a*a + b*b
			if s >= 1 {
				continue
			}
			f := 2 * math.Sqrt(1-s)
			plates[i].RotAxis = [3]float64{a * f, b * f, 1 - 2*s}
			break
		}
		plates[i].AngSpeed = rngMotion.Float64()
		plates[i].IsOceanic = rngOceanic.Float64() < profile.Crust.OceanicFraction
	}
	balanceMomentum(plates)
	return plates
}
```

**(e)** Dispatch in `GeneratePlates` — replace the opening of the function:

```go
func GeneratePlates(profile *types.PlanetProfile, master int64, S int) *PlateField {
	var plates []Plate
	if profile.Crust.MajorPlates > 0 {
		plates = seedPlatesTwoTier(profile, master)
	} else {
		if profile.PlateCount > math.MaxInt16 {
			panic("planetgen/field: PlateCount exceeds int16 max (32767)")
		}
		plates = seedPlates(profile, master)
	}
	if len(plates) == 0 {
		return nil
	}
	// ... rest unchanged (PlateID init, floodFillPlates, computeSDFs)
```

**(f)** Weighted flood fill — in `floodFillPlates`, replace the body of `pushNeighbors`:

```go
	pushNeighbors := func(face cubemap.Face, px, py int, id int16) {
		repeat := int(pf.Plates[id].Weight + 0.5)
		if repeat < 1 {
			repeat = 1
		}
		nbrs := cubemap.FacePixelNeighbors4(face, px, py, S)
		for _, n := range nbrs {
			if pf.PlateID[n.Face][n.PY*S+n.PX] == -1 {
				for range repeat {
					frontier = append(frontier, frontierItem{Addr: n, ID: id})
				}
			}
		}
	}
```

(Duplicates are harmless: the pop loop already skips already-filled pixels. With all weights 1 — the legacy path — `repeat` is 1 and the push sequence is byte-identical to before, so legacy rng-draw order is preserved.)

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/field/ -run 'TestTwoTier|TestLegacy|TestPlate|TestGeneratePlates|TestSeedPlates|TestFloodFill|TestExtract'`
Expected: PASS, including all pre-existing plate tests (legacy unchanged).

- [ ] **Step 5: Commit**

```bash
go build ./... && go test ./pkg/planetgen/... && golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/plates.go pkg/planetgen/field/plates_test.go
git commit -m "P12: two-tier plate seeding with growth-weighted flood fill"
```

---

## Task 4: Craton placement

**Files:**
- Modify: `pkg/planetgen/field/crust.go`
- Modify: `pkg/planetgen/field/crust_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/planetgen/field/crust_test.go`:

```go
func crustTestProfile() *types.PlanetProfile {
	return &types.PlanetProfile{
		PlateConvergentT: 0.75,
		Crust:            terranCrustCfg(),
	}
}

func TestPlaceCratonsOnCarrierPlates(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	cratons := PlaceCratons(p.Crust, 42, pf, 0.5, 0.3, S)
	if len(cratons) < 2 {
		t.Fatalf("got %d cratons, want ≥ 2", len(cratons))
	}
	for i, c := range cratons {
		f, px, py := cubemap.DirToFacePixel(c.Center[0], c.Center[1], c.Center[2], S)
		if got := int(pf.PlateID[f][py*S+px]); got != c.PlateID {
			t.Errorf("craton %d center sits on plate %d, recorded PlateID %d", i, got, c.PlateID)
		}
		if pf.Plates[c.PlateID].IsOceanic {
			t.Errorf("craton %d placed on oceanic plate %d", i, c.PlateID)
		}
		if c.Radius <= 0 || c.Radius > 1.3 {
			t.Errorf("craton %d radius %v out of (0, 1.3]", i, c.Radius)
		}
	}
}

func TestPlaceCratonsAreaBudget(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	for _, landFrac := range []float64{0.1, 0.3, 0.5} {
		cratons := PlaceCratons(p.Crust, 42, pf, 0.5, landFrac, S)
		var capSum float64 // Σ cap areas as fraction of sphere: (1-cos r)/2 each
		for _, c := range cratons {
			capSum += (1 - math.Cos(c.Radius)) / 2
		}
		// Budget includes a 1.15 overlap fudge; allow generous bounds —
		// exactness comes from the sea-level quantile, not from here.
		if capSum < landFrac*0.8 || capSum > landFrac*1.8 {
			t.Errorf("landFrac %v: cap-area sum %v outside [%v, %v]",
				landFrac, capSum, landFrac*0.8, landFrac*1.8)
		}
	}
}

func TestPlaceCratonsAssemblyClusters(t *testing.T) {
	// Supercontinent (assembly 0) cratons must be mutually closer than
	// fragmented (assembly 1) cratons for the same seed.
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	meanPairDot := func(cs []Craton) float64 {
		var sum float64
		var n int
		for i := range cs {
			for j := i + 1; j < len(cs); j++ {
				sum += dot3(cs[i].Center, cs[j].Center)
				n++
			}
		}
		if n == 0 {
			return 1
		}
		return sum / float64(n)
	}
	clustered := meanPairDot(PlaceCratons(p.Crust, 42, pf, 0.0, 0.3, S))
	scattered := meanPairDot(PlaceCratons(p.Crust, 42, pf, 1.0, 0.3, S))
	if clustered <= scattered {
		t.Errorf("assembly=0 mean pair dot %v not greater than assembly=1 %v", clustered, scattered)
	}
}
```

Add `"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/field/ -run TestPlaceCratons`
Expected: FAIL with `undefined: PlaceCratons`.

- [ ] **Step 3: Implement**

Append to `pkg/planetgen/field/crust.go`:

```go
// PlaceCratons places continental-crust rafts on the carrier
// (non-oceanic) plates. Craton count grows with assembly; centers are
// pulled toward a deterministic "assembly focus" (the midpoint of the
// two closest carrier-plate seeds) by (1−assembly)·0.75 so low
// assembly forms one merged landmass abutting at a shared boundary.
// Radii are budgeted so total cap area ≈ landFrac · sphere · 1.15
// (overlap fudge); the sea-level quantile trues up exactness later.
func PlaceCratons(cfg types.CrustConfig, master int64, pf *PlateField, assembly, landFrac float64, S int) []Craton {
	var carriers []int
	for i, p := range pf.Plates {
		if !p.IsOceanic {
			carriers = append(carriers, i)
		}
	}
	if len(carriers) == 0 {
		carriers = []int{0} // degenerate config: force one carrier
	}

	maxC := cfg.CratonsMax
	if maxC < 2 {
		maxC = 8
	}
	k := 2 + int(math.Round(assembly*float64(maxC-2)))

	// Assembly focus: midpoint of the two closest carrier seeds (or the
	// single carrier's seed). Deterministic — no rng.
	focus := pf.Plates[carriers[0]].Seed
	if len(carriers) >= 2 {
		bi, bj, best := 0, 1, -2.0
		for i := range carriers {
			for j := i + 1; j < len(carriers); j++ {
				d := dot3(pf.Plates[carriers[i]].Seed, pf.Plates[carriers[j]].Seed)
				if d > best {
					best, bi, bj = d, i, j
				}
			}
		}
		focus = norm3([3]float64{
			pf.Plates[carriers[bi]].Seed[0] + pf.Plates[carriers[bj]].Seed[0],
			pf.Plates[carriers[bi]].Seed[1] + pf.Plates[carriers[bj]].Seed[1],
			pf.Plates[carriers[bi]].Seed[2] + pf.Plates[carriers[bj]].Seed[2],
		})
	}
	pull := (1 - assembly) * 0.75

	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "crust.cratons")),        //nolint:gosec
		uint64(seed.Domain(master, "crust.cratons.stream")), //nolint:gosec
	))

	// Radius budget: per-craton weight 1/(1+i) (first craton biggest);
	// cap-area fraction f_i = landFrac·1.15·w_i/Σw; r = acos(1 − 2·f).
	weights := make([]float64, k)
	var wSum float64
	for i := range k {
		weights[i] = 1.0 / float64(1+i)
		wSum += weights[i]
	}

	cratons := make([]Craton, 0, k)
	for i := range k {
		plateIdx := carriers[i%len(carriers)]
		base := pf.Plates[plateIdx].Seed
		// Small jitter off the plate seed so repeat cratons on the same
		// plate don't stack exactly.
		j := [3]float64{
			base[0] + (rng.Float64()-0.5)*0.3,
			base[1] + (rng.Float64()-0.5)*0.3,
			base[2] + (rng.Float64()-0.5)*0.3,
		}
		base = norm3(j)
		// Pull toward focus, shrinking t until the center stays on its
		// home plate (cratons never straddle boundaries).
		t := pull
		center := base
		for range 8 {
			c := slerp3(base, focus, t)
			f, px, py := cubemap.DirToFacePixel(c[0], c[1], c[2], S)
			if int(pf.PlateID[f][py*S+px]) == plateIdx {
				center = c
				break
			}
			t *= 0.5
		}
		frac := landFrac * 1.15 * weights[i] / wSum
		if frac > 0.45 {
			frac = 0.45
		}
		r := math.Acos(1 - 2*frac)
		cratons = append(cratons, Craton{Center: center, Radius: r, PlateID: plateIdx})
	}
	return cratons
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/planetgen/field/ -run TestPlaceCratons`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go build ./... && golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/crust.go pkg/planetgen/field/crust_test.go
git commit -m "P12: assembly-driven craton placement with area budget"
```

---

## Task 5: ContinentalMask + BaseHeight (GenerateCrust)

**Files:**
- Modify: `pkg/planetgen/field/crust.go`
- Modify: `pkg/planetgen/field/crust_test.go`
- Create: `pkg/planetgen/field/crust_seam_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `pkg/planetgen/field/crust_test.go`:

```go
func TestGenerateCrustBasics(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	if crust == nil {
		t.Fatal("nil CrustField")
	}
	if crust.ContinentalMask == nil || crust.BaseHeight == nil {
		t.Fatal("nil mask or base height")
	}
	for f := range crust.ContinentalMask.Faces {
		for i, m := range crust.ContinentalMask.Faces[f] {
			if m < 0 || m > 1 || math.IsNaN(m) {
				t.Fatalf("mask face %d idx %d = %v out of [0,1]", f, i, m)
			}
			h := crust.BaseHeight.Faces[f][i]
			if h < 0 || h > 1 || math.IsNaN(h) {
				t.Fatalf("base height face %d idx %d = %v out of [0,1]", f, i, h)
			}
		}
	}
	if crust.LandFraction < 0.22 || crust.LandFraction > 0.38 {
		t.Errorf("resolved land fraction %v outside terran range", crust.LandFraction)
	}
}

func TestGenerateCrustLandAreaNearBudget(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	p.Crust.TargetLandFraction = 0.3 // pin for a deterministic assertion
	p.Crust.Assembly = 0.5
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	var land, total int
	for f := range crust.ContinentalMask.Faces {
		for _, m := range crust.ContinentalMask.Faces[f] {
			if m > 0.5 {
				land++
			}
			total++
		}
	}
	frac := float64(land) / float64(total)
	// Pre-sea-level mask area is approximate (overlap, edge noise);
	// the quantile stage enforces exactness. ±0.12 here.
	if math.Abs(frac-0.3) > 0.12 {
		t.Errorf("mask land fraction %v, want 0.3 ± 0.12", frac)
	}
}

func TestGenerateCrustDeterministic(t *testing.T) {
	const S = 32
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	a := GenerateCrust(p, 42, S, pf)
	b := GenerateCrust(p, 42, S, pf)
	for f := range a.BaseHeight.Faces {
		for i := range a.BaseHeight.Faces[f] {
			if a.BaseHeight.Faces[f][i] != b.BaseHeight.Faces[f][i] {
				t.Fatalf("nondeterministic at face %d idx %d", f, i)
			}
		}
	}
}
```

Create `pkg/planetgen/field/crust_seam_test.go`:

```go
package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
)

func TestCrustSeamContinuity(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	seamtest.AssertSeamContinuity(t, "ContinentalMask", crust.ContinentalMask, 2.0)
	seamtest.AssertSeamContinuity(t, "BaseHeight", crust.BaseHeight, 2.0)
}
```

(Check the tolerance argument convention against an existing caller, e.g. `pkg/planetgen/field/control_seam_test.go`, and match its typical value if 2.0 is out of family.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/planetgen/field/ -run 'TestGenerateCrust|TestCrustSeam'`
Expected: FAIL with `undefined: GenerateCrust`.

- [ ] **Step 3: Implement**

Append to `pkg/planetgen/field/crust.go` (and add `"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"` to imports):

```go
// GenerateCrust runs the full crust stage: resolve sampled params,
// place cratons, and rasterize the ContinentalMask + BaseHeight cube
// maps. Each craton's edge radius is modulated by a shared fBm sampled
// at a per-craton offset, so coastlines are fractal but each landmass
// stays one coherent body. Returns nil when the crust stage is
// disabled (Crust.MajorPlates == 0) or pf is nil.
func GenerateCrust(profile *types.PlanetProfile, master int64, S int, pf *PlateField) *CrustField {
	cfg := profile.Crust
	if cfg.MajorPlates <= 0 || pf == nil {
		return nil
	}
	assembly, landFrac, age := ResolveCrustParams(cfg, master)
	cratons := PlaceCratons(cfg, master, pf, assembly, landFrac, S)

	shelf := cfg.ShelfWidthRad
	if shelf <= 0 {
		shelf = 0.05
	}
	edgeAmp := cfg.EdgeNoiseAmp
	if edgeAmp <= 0 {
		edgeAmp = 0.45
	}
	edgeFreq := cfg.EdgeNoiseFreq
	if edgeFreq <= 0 {
		edgeFreq = 2.2
	}
	edgeOct := cfg.EdgeNoiseOctaves
	if edgeOct <= 0 {
		edgeOct = 4
	}
	platform := cfg.PlatformHeight
	if platform <= 0 {
		platform = 0.62
	}
	floor := cfg.OceanFloorHeight
	if floor <= 0 {
		floor = 0.25
	}

	gen := noise.New(seed.Domain(master, "crust.edge"))
	mask := cubemap.NewF(S)
	base := cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				m := 0.0
				for ci, c := range cratons {
					d := math.Acos(clampDot(dx*c.Center[0] + dy*c.Center[1] + dz*c.Center[2]))
					// Per-craton noise offset: same generator, shifted
					// input domain, so edges differ between cratons.
					off := 7.3 * float64(ci+1)
					e := (gen.FractalNoise3D(dx+off, dy+off*0.7, dz+off*1.3,
						edgeOct, 2.0, 0.5, edgeFreq) - 0.5) * 2 * edgeAmp
					rEff := c.Radius * (1 + e)
					mi := 1 - smoothstep(rEff-shelf, rEff+shelf, d)
					if mi > m {
						m = mi
					}
				}
				mask.Set(face, px, py, m)
				base.Set(face, px, py, floor+(platform-floor)*m)
			}
		}
	}
	return &CrustField{
		Size:            S,
		ContinentalMask: mask,
		BaseHeight:      base,
		Cratons:         cratons,
		Assembly:        assembly,
		LandFraction:    landFrac,
		TectonicAge:     age,
	}
}

func clampDot(d float64) float64 {
	if d > 1 {
		return 1
	}
	if d < -1 {
		return -1
	}
	return d
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/field/ -run 'TestGenerateCrust|TestCrustSeam'`
Expected: PASS. If `TestGenerateCrustLandAreaNearBudget` misses the ±0.12 window, tune the 1.15 fudge in `PlaceCratons` (raise if land is short, lower if over) — do NOT widen the test tolerance past 0.12.

- [ ] **Step 5: Commit**

```bash
go build ./... && go test ./pkg/planetgen/... && golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/crust.go pkg/planetgen/field/crust_test.go pkg/planetgen/field/crust_seam_test.go
git commit -m "P12: GenerateCrust — continental mask + base height from cratons"
```

---

## Task 6: Boundary pixel lists on PlateField

`ClassifyTectonics` (Task 7) needs every convergent/divergent boundary pixel with its smoothed normal and magnitude. `extractBoundaries` computes all of this and throws the per-pixel normals away; retain them in sparse lists.

**Files:**
- Modify: `pkg/planetgen/field/plates.go`
- Modify: `pkg/planetgen/field/plates_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/planetgen/field/plates_test.go`:

```go
func TestBoundaryPixelListsPopulated(t *testing.T) {
	const S = 64
	pf := GeneratePlates(crustProfile(), 42, S)
	if len(pf.ConvPixels)+len(pf.DivPixels) == 0 {
		t.Fatal("no boundary pixels collected")
	}
	for _, bp := range pf.ConvPixels {
		mag := math.Sqrt(bp.N[0]*bp.N[0] + bp.N[1]*bp.N[1] + bp.N[2]*bp.N[2])
		if math.Abs(mag-1) > 1e-6 {
			t.Fatalf("conv pixel normal not unit: %v", bp.N)
		}
		if bp.Mag < 0 || bp.Mag > 1 {
			t.Fatalf("conv pixel mag %v out of [0,1]", bp.Mag)
		}
		// The list entry must match the dense mask-derived data: this
		// pixel's ConvergentMag should equal bp.Mag and distance 0.
		if pf.ConvergentMag[bp.Face][bp.Idx] != bp.Mag {
			t.Fatalf("conv list mag %v != dense mag %v", bp.Mag, pf.ConvergentMag[bp.Face][bp.Idx])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/field/ -run TestBoundaryPixelLists`
Expected: FAIL — `pf.ConvPixels` undefined.

- [ ] **Step 3: Implement**

In `pkg/planetgen/field/plates.go`:

**(a)** Add the type and fields:

```go
// BoundaryPixel is one boundary source pixel retained for crust-aware
// classification (Phase 12): its location, smoothed tangent normal
// pointing toward the foreign plate, and clamped velocity magnitude.
type BoundaryPixel struct {
	Face cubemap.Face
	Idx  int // py*S + px
	N    [3]float64
	Mag  float64
}
```

Add to `PlateField` (after `DivergentMag`):

```go
	// ConvPixels / DivPixels are the sparse boundary source pixels of
	// each kind, with smoothed normals — consumed by ClassifyTectonics.
	ConvPixels []BoundaryPixel
	DivPixels  []BoundaryPixel
```

**(b)** Change `extractBoundaries`'s signature to also return the lists:

```go
func extractBoundaries(pf *PlateField, T float64) (
	conv, div, trans [cubemap.NumFaces][]bool,
	convMag, divMag [cubemap.NumFaces][]float64,
	convPix, divPix []BoundaryPixel,
) {
```

In the classification `switch` at the bottom of the pixel loop, append the list entries:

```go
				switch {
				case proj > +T:
					conv[face][idx] = true
					convMag[face][idx] = magnitudeFromProj(proj, T)
					convPix = append(convPix, BoundaryPixel{
						Face: cubemap.Face(face), Idx: idx,
						N: [3]float64{nx, ny, nz}, Mag: convMag[face][idx],
					})
				case proj < -T:
					div[face][idx] = true
					divMag[face][idx] = magnitudeFromProj(-proj, T)
					divPix = append(divPix, BoundaryPixel{
						Face: cubemap.Face(face), Idx: idx,
						N: [3]float64{nx, ny, nz}, Mag: divMag[face][idx],
					})
				default:
					trans[face][idx] = true
				}
```

**(c)** Update `computeSDFs` to store them:

```go
	conv, div, trans, convMag, divMag, convPix, divPix := extractBoundaries(pf, profile.PlateConvergentT)
	pf.ConvPixels = convPix
	pf.DivPixels = divPix
```

**(d)** Fix any other `extractBoundaries` callers (tests use it — search `grep -n 'extractBoundaries(' pkg/planetgen/field/*_test.go` and add `, _, _` to their assignment lists).

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/field/`
Expected: PASS (new test + all pre-existing plate/seam tests).

- [ ] **Step 5: Commit**

```bash
go build ./... && golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/plates.go pkg/planetgen/field/plates_test.go
git commit -m "P12: retain sparse boundary pixel lists with smoothed normals"
```

---

## Task 7: ClassifyTectonics — crust pairing + five effect fields

**Files:**
- Create: `pkg/planetgen/field/tectonicfx.go`
- Create: `pkg/planetgen/field/tectonicfx_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/field/tectonicfx_test.go`:

```go
package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestClassifyTectonicsPartitionsBoundaries(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	fx := ClassifyTectonics(pf, crust, 6371)
	if fx == nil {
		t.Fatal("nil TectonicFXField")
	}
	// Every convergent boundary pixel must be claimed by exactly one of
	// belt / subduction / arc (distance 0 in exactly one field).
	const eps = 1e-9
	for _, bp := range pf.ConvPixels {
		claimed := 0
		for _, f := range []*cubemap.CubeMapF{fx.BeltDist, fx.SubdDist, fx.ArcDist} {
			if f.Faces[bp.Face][bp.Idx] < eps {
				claimed++
			}
		}
		if claimed != 1 {
			t.Fatalf("conv pixel face %d idx %d claimed by %d classes, want 1", bp.Face, bp.Idx, claimed)
		}
	}
	for _, bp := range pf.DivPixels {
		claimed := 0
		for _, f := range []*cubemap.CubeMapF{fx.RidgeDist, fx.RiftDist} {
			if f.Faces[bp.Face][bp.Idx] < eps {
				claimed++
			}
		}
		if claimed != 1 {
			t.Fatalf("div pixel face %d idx %d claimed by %d classes, want 1", bp.Face, bp.Idx, claimed)
		}
	}
}

func TestClassifyTectonicsBeltsTouchContinents(t *testing.T) {
	// Belt source pixels (dist 0) must sit where the continental mask is
	// high on BOTH sides — sample the mask at the pixel itself and assert
	// it is at least moderately continental.
	const S = 64
	p := crustTestProfile()
	p.Crust.Assembly = 0 // supercontinent maximizes cont-cont collisions
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	fx := ClassifyTectonics(pf, crust, 6371)
	const eps = 1e-9
	checked := 0
	for _, bp := range pf.ConvPixels {
		if fx.BeltDist.Faces[bp.Face][bp.Idx] >= eps {
			continue
		}
		if m := crust.ContinentalMask.Faces[bp.Face][bp.Idx]; m < 0.25 {
			t.Errorf("belt pixel face %d idx %d sits on mask %v < 0.25", bp.Face, bp.Idx, m)
		}
		checked++
	}
	t.Logf("checked %d belt pixels", checked)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/field/ -run TestClassifyTectonics`
Expected: FAIL with `undefined: ClassifyTectonics`.

- [ ] **Step 3: Implement**

Create `pkg/planetgen/field/tectonicfx.go`:

```go
// Crust-aware boundary effects (Phase 12): classify each plate-boundary
// pixel by the continental crust on its two sides, then JFA-propagate
// per-class distance + magnitude fields consumed by ApplyTectonicFX.
package field

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// TectonicFXField holds five (distance-km, magnitude) field pairs, one
// per crust-aware boundary class. Distances follow the PlateField
// convention: geodesic km, half-circumference where a face has no
// source pixel of that class.
type TectonicFXField struct {
	Size      int
	BeltDist  *cubemap.CubeMapF // convergent cont-cont
	BeltMag   *cubemap.CubeMapF
	SubdDist  *cubemap.CubeMapF // convergent ocean-cont (trench + cordillera)
	SubdMag   *cubemap.CubeMapF
	ArcDist   *cubemap.CubeMapF // convergent oce-oce (trench + island arc)
	ArcMag    *cubemap.CubeMapF
	RidgeDist *cubemap.CubeMapF // divergent in ocean (mid-ocean ridge)
	RidgeMag  *cubemap.CubeMapF
	RiftDist  *cubemap.CubeMapF // divergent under/between cratons
	RiftMag   *cubemap.CubeMapF
}

// crustPairSampleOffset is the sampling offset (in direction-space
// units, ~4 pixels at the working resolution) used to look at the
// crust on each side of a boundary pixel.
func crustPairSampleOffset(S int) float64 { return 4.0 / float64(S) }

// ClassifyTectonics splits the convergent boundary pixels into belt /
// subduction / arc and the divergent ones into rift / ridge by
// sampling the continental mask a few pixels to each side along the
// smoothed boundary normal, then JFA-propagates each class.
func ClassifyTectonics(pf *PlateField, crust *CrustField, radiusKm float64) *TectonicFXField {
	if pf == nil || crust == nil {
		return nil
	}
	S := pf.Size
	if radiusKm == 0 {
		radiusKm = 6371
	}
	delta := crustPairSampleOffset(S)

	var beltM, subdM, arcM, ridgeM, riftM [cubemap.NumFaces][]bool
	var beltV, subdV, arcV, ridgeV, riftV [cubemap.NumFaces][]float64
	for f := range beltM {
		beltM[f] = make([]bool, S*S)
		subdM[f] = make([]bool, S*S)
		arcM[f] = make([]bool, S*S)
		ridgeM[f] = make([]bool, S*S)
		riftM[f] = make([]bool, S*S)
		beltV[f] = make([]float64, S*S)
		subdV[f] = make([]float64, S*S)
		arcV[f] = make([]float64, S*S)
		ridgeV[f] = make([]float64, S*S)
		riftV[f] = make([]float64, S*S)
	}

	sideMasks := func(bp BoundaryPixel) (here, there float64) {
		px := bp.Idx % S
		py := bp.Idx / S
		dx, dy, dz := cubemap.FacePixelToDir(bp.Face, px, py, S)
		here = crust.ContinentalMask.Sample(dx-bp.N[0]*delta, dy-bp.N[1]*delta, dz-bp.N[2]*delta)
		there = crust.ContinentalMask.Sample(dx+bp.N[0]*delta, dy+bp.N[1]*delta, dz+bp.N[2]*delta)
		return here, there
	}

	const contThresh = 0.5
	for _, bp := range pf.ConvPixels {
		a, b := sideMasks(bp)
		switch {
		case a > contThresh && b > contThresh:
			beltM[bp.Face][bp.Idx] = true
			beltV[bp.Face][bp.Idx] = bp.Mag
		case a > contThresh || b > contThresh:
			subdM[bp.Face][bp.Idx] = true
			subdV[bp.Face][bp.Idx] = bp.Mag
		default:
			arcM[bp.Face][bp.Idx] = true
			arcV[bp.Face][bp.Idx] = bp.Mag
		}
	}
	const riftThresh = 0.35
	for _, bp := range pf.DivPixels {
		a, b := sideMasks(bp)
		if math.Max(a, b) > riftThresh {
			riftM[bp.Face][bp.Idx] = true
			riftV[bp.Face][bp.Idx] = bp.Mag
		} else {
			ridgeM[bp.Face][bp.Idx] = true
			ridgeV[bp.Face][bp.Idx] = bp.Mag
		}
	}

	factor := math.Pi * radiusKm
	scaleKm := func(f *cubemap.CubeMapF) *cubemap.CubeMapF {
		for i := range f.Faces {
			for j := range f.Faces[i] {
				f.Faces[i][j] *= factor
			}
		}
		return f
	}
	fx := &TectonicFXField{Size: S}
	var d, m *cubemap.CubeMapF
	d, m = JumpFloodFromMaskWithValue(beltM, beltV, S)
	fx.BeltDist, fx.BeltMag = scaleKm(d), m
	d, m = JumpFloodFromMaskWithValue(subdM, subdV, S)
	fx.SubdDist, fx.SubdMag = scaleKm(d), m
	d, m = JumpFloodFromMaskWithValue(arcM, arcV, S)
	fx.ArcDist, fx.ArcMag = scaleKm(d), m
	d, m = JumpFloodFromMaskWithValue(ridgeM, ridgeV, S)
	fx.RidgeDist, fx.RidgeMag = scaleKm(d), m
	d, m = JumpFloodFromMaskWithValue(riftM, riftV, S)
	fx.RiftDist, fx.RiftMag = scaleKm(d), m
	return fx
}
```

(`noise`, `seed`, and `types` imports are used by `ApplyTectonicFX` in Task 8 — include them only when Task 8 lands; for this task the imports are `math`, `cubemap`.)

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/field/ -run TestClassifyTectonics`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go build ./... && golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/tectonicfx.go pkg/planetgen/field/tectonicfx_test.go
git commit -m "P12: crust-pairing boundary classification + per-class JFA fields"
```

---

## Task 8: ApplyTectonicFX — the six height effects × TectonicAge

**Files:**
- Modify: `pkg/planetgen/field/tectonicfx.go`
- Modify: `pkg/planetgen/field/tectonicfx_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/planetgen/field/tectonicfx_test.go`:

```go
func defaultFXCfg() types.TectonicFXConfig {
	return types.TectonicFXConfig{
		BeltAmp: 0.30, BeltWidthKm: 900, BeltFreq: 3.2, BeltOctaves: 5,
		CordAmp: 0.22, CordWidthKm: 450,
		TrenchDepth: 0.12, TrenchWidthKm: 220,
		ArcAmp: 0.25, ArcWidthKm: 260,
		RidgeAmp: 0.06, RidgeWidthKm: 700,
		RiftDepth: 0.10, RiftWidthKm: 280, RiftShoulder: 0.35,
		TransformAmp: 0.03, TransformWidthKm: 150,
		ActivityFreq: 1.5,
	}
}

func TestApplyTectonicFXRaisesBelts(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	p.Crust.Assembly = 0
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	fx := ClassifyTectonics(pf, crust, 6371)
	hm := cubemap.NewF(S) // flat zero heightmap isolates the FX deltas
	ApplyTectonicFX(hm, fx, crust, pf, defaultFXCfg(), 42, S)

	var beltSum float64
	var beltN int
	var farSum float64
	var farN int
	for f := range hm.Faces {
		for i, h := range hm.Faces[f] {
			switch {
			case fx.BeltDist.Faces[f][i] < 300:
				beltSum += h
				beltN++
			case fx.BeltDist.Faces[f][i] > 4000 && fx.RiftDist.Faces[f][i] > 4000 &&
				fx.SubdDist.Faces[f][i] > 4000 && fx.ArcDist.Faces[f][i] > 4000 &&
				fx.RidgeDist.Faces[f][i] > 4000:
				farSum += h
				farN++
			}
		}
	}
	if beltN == 0 || farN == 0 {
		t.Skip("seed produced no belt pixels at this size; acceptable for a single fixed seed")
	}
	beltMean := beltSum / float64(beltN)
	farMean := farSum / float64(farN)
	if beltMean <= farMean+0.02 {
		t.Errorf("belt mean Δh %v not above far-field %v + 0.02", beltMean, farMean)
	}
}

func TestApplyTectonicFXAgeSoftens(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	p.Crust.Assembly = 0
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	fx := ClassifyTectonics(pf, crust, 6371)

	maxAbs := func(age float64) float64 {
		c := *crust
		c.TectonicAge = age
		hm := cubemap.NewF(S)
		ApplyTectonicFX(hm, fx, &c, pf, defaultFXCfg(), 42, S)
		mx := 0.0
		for f := range hm.Faces {
			for _, h := range hm.Faces[f] {
				if a := math.Abs(h); a > mx {
					mx = a
				}
			}
		}
		return mx
	}
	young := maxAbs(0.0)
	old := maxAbs(1.0)
	if old >= young {
		t.Errorf("old-planet max relief %v not below young %v", old, young)
	}
}
```

Add `"math"` and `"github.com/rsned/spacemolt-kb/pkg/planetgen/types"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/field/ -run TestApplyTectonicFX`
Expected: FAIL with `undefined: ApplyTectonicFX`.

- [ ] **Step 3: Implement**

Append to `pkg/planetgen/field/tectonicfx.go` (now add the `noise`, `seed`, `types` imports):

```go
// ApplyTectonicFX adds the six crust-aware boundary effects to the
// heightmap in place. TectonicAge (from crust) scales amplitude down
// and width up so old planets read wide-and-rounded (Appalachians)
// while young ones read tall-and-sharp (Himalayas). A low-frequency
// activity noise varies energy between boundaries so they don't all
// look equally violent. Deterministic; no internal state survives.
func ApplyTectonicFX(hm *cubemap.CubeMapF, fx *TectonicFXField, crust *CrustField,
	pf *PlateField, cfg types.TectonicFXConfig, master int64, S int) {
	if hm == nil || fx == nil || crust == nil || pf == nil {
		return
	}
	age := crust.TectonicAge
	hs := 1.0 - 0.55*age // height scale: 1.0 young → 0.45 old
	ws := 1.0 + 0.8*age  // width scale:  1.0 young → 1.8 old

	actFreq := cfg.ActivityFreq
	if actFreq <= 0 {
		actFreq = 1.5
	}
	actGen := noise.New(seed.Domain(master, "tectonicfx.activity"))
	beltGen := noise.New(seed.Domain(master, "tectonicfx.belt"))

	// Gaussian envelope, cut at 3 sigma to skip most pixels cheaply.
	env := func(distKm, widthKm float64) float64 {
		if widthKm <= 0 || distKm > 3*widthKm {
			return 0
		}
		x := distKm / widthKm
		return math.Exp(-x * x)
	}

	beltOct := cfg.BeltOctaves
	if beltOct <= 0 {
		beltOct = 5
	}

	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				i := py*S + px
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				activity := 0.5 + 0.5*actGen.FractalNoise3D(dx, dy, dz, 2, 2.0, 0.5, actFreq)
				contHere := crust.ContinentalMask.Faces[face][i]
				var dh float64

				// 1. Collision belt (cont-cont): ridged relief inside a
				// wide envelope straddling the suture.
				if cfg.BeltAmp > 0 {
					if e := env(fx.BeltDist.Faces[face][i], cfg.BeltWidthKm*ws); e > 0 {
						r := beltGen.RidgedFractal3D(
							dx*cfg.BeltFreq, dy*cfg.BeltFreq, dz*cfg.BeltFreq,
							beltOct, 2.0, 0.5, 1.0)
						mag := 0.4 + 0.6*fx.BeltMag.Faces[face][i]
						dh += cfg.BeltAmp * hs * e * mag * activity * r
					}
				}

				// 2+3. Subduction (ocean-cont): cordillera on the
				// continent side, trench on the ocean side.
				if cfg.CordAmp > 0 || cfg.TrenchDepth > 0 {
					d := fx.SubdDist.Faces[face][i]
					mag := fx.SubdMag.Faces[face][i]
					if contHere > 0.5 {
						if e := env(d, cfg.CordWidthKm*ws); e > 0 {
							r := beltGen.RidgedFractal3D(
								dx*cfg.BeltFreq*1.4, dy*cfg.BeltFreq*1.4, dz*cfg.BeltFreq*1.4,
								beltOct, 2.0, 0.5, 1.0)
							dh += cfg.CordAmp * hs * e * (0.4 + 0.6*mag) * activity * r
						}
					} else {
						if e := env(d, cfg.TrenchWidthKm); e > 0 {
							dh -= cfg.TrenchDepth * e * (0.4 + 0.6*mag)
						}
					}
				}

				// 4. Island arc (oce-oce): trench-adjacent dotted islands
				// gated by a mid-frequency noise so the arc is a chain,
				// not a wall.
				if cfg.ArcAmp > 0 && contHere < 0.5 {
					if e := env(fx.ArcDist.Faces[face][i], cfg.ArcWidthKm); e > 0 {
						islands := smoothstep(0.55, 0.72,
							actGen.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 8.0))
						mag := 0.4 + 0.6*fx.ArcMag.Faces[face][i]
						dh += cfg.ArcAmp * hs * e * mag * islands
					}
				}

				// 5a. Mid-ocean ridge: gentle bathymetric rise.
				if cfg.RidgeAmp > 0 && contHere < 0.5 {
					if e := env(fx.RidgeDist.Faces[face][i], cfg.RidgeWidthKm); e > 0 {
						dh += cfg.RidgeAmp * e * (0.4 + 0.6*fx.RidgeMag.Faces[face][i])
					}
				}

				// 5b. Continental rift: floor depression + shoulder uplift.
				if cfg.RiftDepth > 0 {
					d := fx.RiftDist.Faces[face][i]
					w := cfg.RiftWidthKm * ws
					mag := 0.4 + 0.6*fx.RiftMag.Faces[face][i]
					if e := env(d, w); e > 0 {
						// Age deepens rifts (mature rift → Red Sea), unlike
						// belts which age erodes: scale by (0.5 + 0.5·age).
						dh -= cfg.RiftDepth * (0.5 + 0.5*age) * e * mag
					}
					if cfg.RiftShoulder > 0 {
						sd := d - 1.6*w
						if e := env(math.Abs(sd), w*0.7); e > 0 {
							dh += cfg.RiftDepth * cfg.RiftShoulder * e * mag
						}
					}
				}

				// 6. Transform faults: small-scale roughness near the
				// existing transform SDF.
				if cfg.TransformAmp > 0 {
					if e := env(pf.Transform[face][i], cfg.TransformWidthKm); e > 0 {
						n := actGen.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 12.0) - 0.5
						dh += cfg.TransformAmp * e * n
					}
				}

				if dh != 0 {
					hm.Faces[face][i] += dh
				}
			}
		}
	}
}
```

Verify `RidgedFractal3D`'s parameter order against `pkg/planetgen/noise/ridged.go` (call sites in `render/rocky.go` pass `octaves, lacunarity, gain, offset`) — match exactly.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/field/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go build ./... && golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/tectonicfx.go pkg/planetgen/field/tectonicfx_test.go
git commit -m "P12: ApplyTectonicFX — belts/trenches/arcs/ridges/rifts × TectonicAge"
```

---

## Task 9: QuantileSeaLevel

**Files:**
- Create: `pkg/planetgen/field/sealevel.go`
- Create: `pkg/planetgen/field/sealevel_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/field/sealevel_test.go`:

```go
package field

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestQuantileSeaLevelUniform(t *testing.T) {
	const S = 64
	hm := cubemap.NewF(S)
	rng := rand.New(rand.NewPCG(1, 2))
	for f := range hm.Faces {
		for i := range hm.Faces[f] {
			hm.Faces[f][i] = rng.Float64()
		}
	}
	for _, oceanFrac := range []float64{0.3, 0.5, 0.7, 0.9} {
		lvl := QuantileSeaLevel(hm, oceanFrac)
		var below, total int
		for f := range hm.Faces {
			for _, h := range hm.Faces[f] {
				if h < lvl {
					below++
				}
				total++
			}
		}
		got := float64(below) / float64(total)
		if math.Abs(got-oceanFrac) > 0.01 {
			t.Errorf("oceanFrac %v: %v of pixels below level %v", oceanFrac, got, lvl)
		}
	}
}

func TestQuantileSeaLevelEdgeCases(t *testing.T) {
	const S = 8
	hm := cubemap.NewF(S)
	if lvl := QuantileSeaLevel(hm, 0.5); lvl < 0 || lvl > 1 {
		t.Errorf("flat heightmap level %v out of [0,1]", lvl)
	}
	if lvl := QuantileSeaLevel(hm, 0); lvl != 0 {
		t.Errorf("oceanFrac 0 → level %v, want 0", lvl)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/field/ -run TestQuantileSeaLevel`
Expected: FAIL with `undefined: QuantileSeaLevel`.

- [ ] **Step 3: Implement**

Create `pkg/planetgen/field/sealevel.go`:

```go
package field

import "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"

// QuantileSeaLevel returns the height threshold such that oceanFrac of
// the heightmap's pixels fall below it, computed from a 4096-bucket
// histogram over [0,1] with linear interpolation inside the crossing
// bucket. Heights outside [0,1] are clamped into the histogram range
// (the rocky pipeline normalizes before calling this). oceanFrac ≤ 0
// returns 0 (no ocean); oceanFrac ≥ 1 returns 1.
func QuantileSeaLevel(hm *cubemap.CubeMapF, oceanFrac float64) float64 {
	if oceanFrac <= 0 {
		return 0
	}
	if oceanFrac >= 1 {
		return 1
	}
	const buckets = 4096
	var hist [buckets]int
	total := 0
	for f := range hm.Faces {
		for _, h := range hm.Faces[f] {
			b := int(h * buckets)
			if b < 0 {
				b = 0
			}
			if b >= buckets {
				b = buckets - 1
			}
			hist[b]++
			total++
		}
	}
	target := oceanFrac * float64(total)
	cum := 0.0
	for b := range buckets {
		next := cum + float64(hist[b])
		if next >= target {
			frac := 0.0
			if hist[b] > 0 {
				frac = (target - cum) / float64(hist[b])
			}
			return (float64(b) + frac) / buckets
		}
		cum = next
	}
	return 1
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/field/ -run TestQuantileSeaLevel`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go build ./... && golangci-lint run ./pkg/planetgen/field/...
git add pkg/planetgen/field/sealevel.go pkg/planetgen/field/sealevel_test.go
git commit -m "P12: histogram-quantile sea level from target land fraction"
```

---

## Task 10: Rocky pipeline integration

This is the riskiest task; it changes `generateRockyHeightmapDebug`'s behavior on the crust path and its signature everywhere.

**Files:**
- Modify: `pkg/planetgen/render/rocky.go`
- Modify: `pkg/planetgen/render/debug.go`
- Create: `pkg/planetgen/render/rocky_crust_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/render/rocky_crust_test.go`:

```go
package render

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// crustTerran returns the terran default profile with the Phase 12
// crust stage force-enabled and pinned parameters for assertions.
func crustTerran(t *testing.T) *types.PlanetProfile {
	t.Helper()
	p := profileForTest(t, "terran") // see Step 1a below
	p.Crust = types.CrustConfig{
		MajorPlates: 7, MinorPlates: 4, MajorGrowthBias: 4,
		OceanicFraction: 0.45,
		Assembly:        0.5, AssemblyWeights: [3]float64{25, 65, 10},
		TargetLandFraction: 0.3, LandFracLo: 0.22, LandFracHi: 0.38,
		TectonicAge: 0.5, AgeLo: 0.25, AgeHi: 0.75,
		CratonsMax: 8, ShelfWidthRad: 0.05,
		EdgeNoiseAmp: 0.45, EdgeNoiseFreq: 2.2, EdgeNoiseOctaves: 4,
		PlatformHeight: 0.62, OceanFloorHeight: 0.25,
	}
	p.TectonicFX = types.TectonicFXConfig{
		BeltAmp: 0.30, BeltWidthKm: 900, BeltFreq: 3.2, BeltOctaves: 5,
		CordAmp: 0.22, CordWidthKm: 450,
		TrenchDepth: 0.12, TrenchWidthKm: 220,
		ArcAmp: 0.25, ArcWidthKm: 260,
		RidgeAmp: 0.06, RidgeWidthKm: 700,
		RiftDepth: 0.10, RiftWidthKm: 280, RiftShoulder: 0.35,
		TransformAmp: 0.03, TransformWidthKm: 150,
		ActivityFreq: 1.5,
	}
	return p
}

func TestCrustPathLandFractionMatchesTarget(t *testing.T) {
	const S = 128
	p := crustTerran(t)
	for _, master := range []int64{1, 42, 31337} {
		hm, _, lvl := renderHeightmapForTest(p, master, S) // see Step 1a
		var below, total int
		for f := range hm.Faces {
			for _, h := range hm.Faces[f] {
				if h < lvl {
					below++
				}
				total++
			}
		}
		ocean := float64(below) / float64(total)
		if math.Abs(ocean-0.7) > 0.03 {
			t.Errorf("seed %d: ocean fraction %v, want 0.70 ± 0.03", master, ocean)
		}
	}
}

func TestLegacyPathOceanLevelPassthrough(t *testing.T) {
	const S = 64
	p := profileForTest(t, "terran") // Crust zero-value → legacy
	_, _, lvl := renderHeightmapForTest(p, 42, S)
	if lvl != p.OceanLevel {
		t.Errorf("legacy ocean level %v, want profile value %v", lvl, p.OceanLevel)
	}
}
```

**Step 1a:** the test needs two small exported-for-test helpers. Add to the bottom of `rocky_crust_test.go` (same package, so unexported access works):

```go
// profileForTest deep-copies a per-type default so tests can mutate it.
func profileForTest(t *testing.T, typ string) *types.PlanetProfile {
	t.Helper()
	p, ok := planetgenProfiles(typ)
	if !ok {
		t.Fatalf("no default profile for %q", typ)
	}
	return p
}
```

The render package cannot import the root `planetgen` package (it would be an import cycle: planetgen → render). Check how existing render tests obtain profiles — `pkg/planetgen/render/rocky_test.go` and `rocky_ridged_plate_mask_test.go` construct profiles inline or via a helper; mirror that pattern. If they build profiles inline, replace `profileForTest` with an inline terran-like literal (copy the `ControlConfig`, `Ridged`, `OceanLevel: 0.5`, `Coastal`, `HeightSmoothRadius: 4` fields from `pkg/planetgen/profile.go`'s terran entry, skipping palettes/biome — heights don't need colors). `renderHeightmapForTest` wraps the new three-value internal call:

```go
func renderHeightmapForTest(p *types.PlanetProfile, master int64, S int) (*cubemap.CubeMapF, []feature.Crater, float64) {
	jitter := jitterForProfile(p, master, S) // reuse however RenderRocky builds it (see rocky.go:24)
	plates := field.GeneratePlates(p, master, S)
	return generateRockyHeightmapWithJitter(p, master, S, jitter, plates)
}
```

Match the jitter-construction call exactly to what `RenderRocky` does at `rocky.go:24-25` (read it; it is one or two lines).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/render/ -run 'TestCrustPath|TestLegacyPathOceanLevel'`
Expected: FAIL — `generateRockyHeightmapWithJitter` returns two values, not three.

- [ ] **Step 3: Implement the pipeline changes**

All edits in `pkg/planetgen/render/rocky.go` unless noted.

**(a) Signature change.** `generateRockyHeightmapWithJitter` and `generateRockyHeightmapDebug` now return `(*cubemap.CubeMapF, []feature.Crater, float64)` where the float is the effective ocean level.

**(b) Crust setup.** In `generateRockyHeightmapDebug`, right after the `warper` setup, add:

```go
	crustOn := plates != nil && profile.Crust.MajorPlates > 0
	var crust *field.CrustField
	if crustOn {
		crust = field.GenerateCrust(profile, seed, S, plates)
		for face := range cubemap.Face(cubemap.NumFaces) {
			copy(heightmap.Faces[face], crust.BaseHeight.Faces[face])
		}
		if frame != nil {
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "Crust",
				Kind:     "height",
				RawFbm:   crust.ContinentalMask.Clone(),
				SumAfter: heightmap.Clone(),
				Skipped:  bypass["Crust"],
			})
		}
	}
```

(Move this AFTER `heightmap := cubemap.NewF(S)`. Crust has no bypass-skip of the base height itself — bypassing "Crust" in the debug grid only makes sense as "show me the world without the crust stage", which would cascade into every later stage; instead the `Skipped` flag is cosmetic here, and the panel-level bypass is wired in Task 13 by zeroing `MajorPlates` in the explorer profile, which is the honest off-switch.)

**(c) Demote Continentalness.** In the debug-path control-field loop, change the contribution condition from:

```go
				if !bypassed && i < heightContributingFields {
```

to:

```go
				if !bypassed && i < heightContributingFields && !(crustOn && i == 0) {
```

In the fast-path loop, change:

```go
						for i := range 3 {
```

to:

```go
						for i := range 3 {
							if crustOn && i == 0 {
								continue // crust path: Continentalness no longer drives height
							}
```

**(d) Skip legacy Ridged on the crust path.** The ridged stage in both debug and fast paths is guarded by `if ridgedGen != nil`; create the generator only off the crust path:

```go
		var ridgedGen *noise.Generator
		if !crustOn && profile.Ridged.Amp > 0 && profile.Ridged.Freq > 0 && profile.Ridged.Octaves > 0 {
			ridgedGen = noise.New(pgseed.Domain(seed, "ridged"))
		}
```

(Apply to both occurrences — the `useControl` branch and the legacy-noise branch.)

**(e) TectonicFX pass.** Insert immediately BEFORE the Basin block:

```go
	// Phase 12: crust-aware boundary effects (belt/cordillera/trench/
	// arc/ridge/rift/fault), replacing legacy Ridged+Basin on the
	// crust path.
	if crustOn {
		bypassed := bypass["TectonicFX"]
		var hmBefore *cubemap.CubeMapF
		if frame != nil {
			hmBefore = heightmap.Clone()
		}
		if !bypassed {
			fx := field.ClassifyTectonics(plates, crust, profile.RadiusKm)
			field.ApplyTectonicFX(heightmap, fx, crust, plates, profile.TectonicFX, seed, S)
		}
		if frame != nil {
			delta := cubemap.NewF(S)
			for face := range cubemap.Face(cubemap.NumFaces) {
				for i := range heightmap.Faces[face] {
					delta.Faces[face][i] = heightmap.Faces[face][i] - hmBefore.Faces[face][i]
				}
			}
			frame.Stages = append(frame.Stages, DebugStage{
				Name:     "TectonicFX",
				Kind:     "height",
				RawFbm:   delta,
				SumAfter: heightmap.Clone(),
				Skipped:  bypassed,
			})
		}
	}
```

**(f) Gate Basin and Continents off the crust path.** Change their conditions:

```go
	if plates != nil && !crustOn && profile.Basin.Depth > 0 && profile.Basin.PlateDivergentScaleKm > 0 {
```

```go
	if profile.Continents.Seeds > 0 && !crustOn {
```

**(g) Derived ocean level.** Introduce `oceanLevel := profile.OceanLevel` immediately before the Normalize block. After the Normalize block (and its debug stage), add:

```go
	if crustOn {
		oceanLevel = field.QuantileSeaLevel(heightmap, 1-crust.LandFraction)
	}
```

Then replace every later use of `profile.OceanLevel` INSIDE this function with `oceanLevel`. There are exactly three: the Coastal `active` check + `DistanceToCoast` call, and the `field.Erode(..., profile.OceanLevel, S)` argument. (Verify with `grep -n 'profile.OceanLevel' pkg/planetgen/render/rocky.go` — only those inside `generateRockyHeightmapDebug` change.)

Finally, AFTER the Flow block (the last height-mutating stage before craters), recompute once so the returned level reflects the final height distribution — erosion and river carving shift the histogram slightly, and the Task 12 land-fraction invariant (±0.03) measures the final heightmap:

```go
	if crustOn {
		oceanLevel = field.QuantileSeaLevel(heightmap, 1-crust.LandFraction)
	}
```

(The earlier computation still matters: Coastal and Erode consume it mid-pipeline. Two histogram passes cost ~one heightmap scan each — negligible.)

**(h) Return it.** Both functions return `heightmap, craters, oceanLevel`.

**(i) Caller updates** (use `grep -n 'generateRockyHeightmap' pkg/planetgen/render/*.go`):

`RenderRocky` (rocky.go:26):

```go
	heightmap, craters, oceanLevel := generateRockyHeightmapWithJitter(profile, seed, S, jitter, plates)
	if oceanLevel != profile.OceanLevel {
		prof := *profile
		prof.OceanLevel = oceanLevel
		profile = &prof
	}
```

(The copy keeps the caller's shared default profile immutable; everything below — flow gen and `colorizeRocky` — reads the patched copy.)

`RenderRockyHeightmap` (rocky.go:44): `heightmap, _, _ := ...`.

`RenderRockyWithCiv` (rocky.go:89): same copy-on-differ pattern as `RenderRocky`, BEFORE the climate/rainshadow/civ calls so civ habitability sees the derived level.

`render/debug.go:105`: same copy-on-differ pattern; the colorize stages below it must use the patched profile.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/render/ ./pkg/planetgen/field/`
Expected: PASS — new tests AND all existing render tests (legacy profiles take `crustOn == false` everywhere, so outputs are bit-identical; if any existing rocky test fails, the legacy path was disturbed — fix before proceeding).

- [ ] **Step 5: Commit**

```bash
go build ./... && go test ./pkg/planetgen/... && golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/render/rocky.go pkg/planetgen/render/debug.go pkg/planetgen/render/rocky_crust_test.go
git commit -m "P12: crust path in rocky pipeline + derived quantile sea level"
```

---

## Task 11: Per-type defaults for the six crust-enabled types

**Files:**
- Modify: `pkg/planetgen/profile.go`
- Modify: `pkg/planetgen/planetgen_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/planetgen/planetgen_test.go`:

```go
func TestCrustEnabledTypes(t *testing.T) {
	enabled := []string{"terran", "super_terran", "oceanic", "tundra", "glacial", "arid"}
	for _, typ := range enabled {
		p, ok := Profiles[typ]
		if !ok {
			t.Fatalf("missing profile %q", typ)
		}
		if p.Crust.MajorPlates == 0 {
			t.Errorf("%s: crust not enabled", typ)
		}
		if p.Crust.Assembly != -1 || p.Crust.TargetLandFraction != -1 || p.Crust.TectonicAge != -1 {
			t.Errorf("%s: sampled params must default to -1 sentinels", typ)
		}
		if p.Crust.LandFracHi <= p.Crust.LandFracLo {
			t.Errorf("%s: land fraction range empty", typ)
		}
		if p.TectonicFX.BeltAmp <= 0 {
			t.Errorf("%s: TectonicFX not configured", typ)
		}
	}
	disabled := []string{"scorched", "hothouse", "lava_world", "ice_world", "jovian", "ice_giant", "unknown"}
	for _, typ := range disabled {
		p, ok := Profiles[typ]
		if !ok {
			continue // not all type names guaranteed; skip absent
		}
		if p.Crust.MajorPlates != 0 {
			t.Errorf("%s: crust must stay disabled this phase", typ)
		}
	}
}
```

(Adjust the `Profiles` map identifier if the actual exported name differs — check `pkg/planetgen/profile.go` for the map declaration, e.g. `var Profiles = map[string]*types.PlanetProfile{...}` or a `GetProfile` accessor, and write the test against what exists.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/ -run TestCrustEnabledTypes`
Expected: FAIL — crust not enabled on any type.

- [ ] **Step 3: Add defaults**

In `pkg/planetgen/profile.go`, add to the `"terran"` entry (after the `RainShadow` field):

```go
		// Phase 12: crust-raft tectonic continents.
		Crust: types.CrustConfig{
			MajorPlates: 7, MinorPlates: 4, MajorGrowthBias: 4,
			OceanicFraction: 0.45,
			Assembly:        -1, AssemblyWeights: [3]float64{25, 65, 10},
			TargetLandFraction: -1, LandFracLo: 0.22, LandFracHi: 0.38,
			TectonicAge: -1, AgeLo: 0.25, AgeHi: 0.75,
			CratonsMax: 8, ShelfWidthRad: 0.05,
			EdgeNoiseAmp: 0.45, EdgeNoiseFreq: 2.2, EdgeNoiseOctaves: 4,
			PlatformHeight: 0.62, OceanFloorHeight: 0.25,
		},
		TectonicFX: types.TectonicFXConfig{
			BeltAmp: 0.30, BeltWidthKm: 900, BeltFreq: 3.2, BeltOctaves: 5,
			CordAmp: 0.22, CordWidthKm: 450,
			TrenchDepth: 0.12, TrenchWidthKm: 220,
			ArcAmp: 0.25, ArcWidthKm: 260,
			RidgeAmp: 0.06, RidgeWidthKm: 700,
			RiftDepth: 0.10, RiftWidthKm: 280, RiftShoulder: 0.35,
			TransformAmp: 0.03, TransformWidthKm: 150,
			ActivityFreq: 1.5,
		},
```

Then the other five, varying only these fields from the terran block (copy the rest verbatim):

| Type | Crust deltas | TectonicFX deltas |
|---|---|---|
| `super_terran` | `MajorPlates: 8`, `LandFracLo: 0.30, LandFracHi: 0.50`, `AssemblyWeights: [3]float64{35, 55, 10}` | `BeltAmp: 0.36` (bigger world, bigger relief) |
| `oceanic` | `MajorPlates: 8, MinorPlates: 5`, `OceanicFraction: 0.8`, `LandFracLo: 0.03, LandFracHi: 0.12`, `AssemblyWeights: [3]float64{5, 25, 70}`, `CratonsMax: 10` | `ArcAmp: 0.34` (arc islands carry the look) |
| `tundra` | same as terran, `LandFracLo: 0.25, LandFracHi: 0.45` | same as terran |
| `glacial` | same as tundra | same as terran |
| `arid` | `LandFracLo: 0.55, LandFracHi: 0.80`, `AssemblyWeights: [3]float64{40, 55, 5}`, `OceanFloorHeight: 0.30` | `RiftDepth: 0.14` (dry rift valleys read well) |

Existing `PlateCount`, `OceanicPlateFraction`, `Ridged`, `Basin`, `Continents` fields on these six profiles stay as-is — the crust path ignores them at runtime (Task 10's gates), and they remain live for anyone who zeroes `Crust.MajorPlates` in the explorer.

- [ ] **Step 4: Run tests + render a smoke planet**

Run: `go test ./pkg/planetgen/...`
Expected: PASS.

Run: `go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/p12_terran.png`
(Verify the exact flag names against `cmd/generate-planet-maps/main.go` / its README first; use the documented single-planet invocation.) Open `/tmp/p12_terran.png` and eyeball: a few sizeable continents, mountain belts along some interior seams, no island spray.

- [ ] **Step 5: Commit**

```bash
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/profile.go pkg/planetgen/planetgen_test.go
git commit -m "P12: crust + tectonic FX defaults for the six water-world types"
```

---

## Task 12: Statistical invariants

**Files:**
- Modify: `cmd/generate-planet-maps/invariants_test.go`

- [ ] **Step 1: Write the tests** (these are CI gates encoding the original complaint; they go straight in — the "failing first" check here is running them against a deliberately broken constant, see Step 2)

Append to `cmd/generate-planet-maps/invariants_test.go`, following the existing file's conventions for rendering test planets (it already renders via the public planetgen API at a small face size — match the established helper / face size, typically S=128 across several seeds):

```go
// landMask returns a per-face boolean land mask for a crust-path
// terran heightmap plus its derived ocean level, at face size S.
// Rendering goes through the same public entry the other invariants
// use; adjust the call to match this file's existing render helper.
func phase12LandStats(t *testing.T, master int64, S int) (landFrac float64, largestShare float64, smallIslands int) {
	t.Helper()
	p := planetgen.GetProfile("terran") // match the accessor used elsewhere in this file
	prof := *p
	prof.Crust.TargetLandFraction = 0.30 // pin: invariant needs a known target
	prof.Crust.Assembly = 0.5

	hm, lvl := render.RenderRockyHeightmapWithOceanLevel(&prof, master, S) // hook added below
	land := make([][]bool, cubemap.NumFaces)
	total, landN := 0, 0
	for f := range hm.Faces {
		land[f] = make([]bool, S*S)
		for i, h := range hm.Faces[f] {
			if h >= lvl {
				land[f][i] = true
				landN++
			}
			total++
		}
	}
	landFrac = float64(landN) / float64(total)

	// Connected components over the land mask with cross-face neighbors.
	visited := make([][]bool, cubemap.NumFaces)
	for f := range visited {
		visited[f] = make([]bool, S*S)
	}
	var sizes []int
	for f := range land {
		for i := range land[f] {
			if !land[f][i] || visited[f][i] {
				continue
			}
			size := 0
			stack := []cubemap.PixelAddr{{Face: cubemap.Face(f), PX: i % S, PY: i / S}}
			visited[f][i] = true
			for len(stack) > 0 {
				cur := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				size++
				for _, nb := range cubemap.FacePixelNeighbors4(cur.Face, cur.PX, cur.PY, S) {
					idx := nb.PY*S + nb.PX
					if land[nb.Face][idx] && !visited[nb.Face][idx] {
						visited[nb.Face][idx] = true
						stack = append(stack, nb)
					}
				}
			}
			sizes = append(sizes, size)
		}
	}
	largest := 0
	for _, s := range sizes {
		if s > largest {
			largest = s
		}
		if s < (S*S)/512 { // tiny blob ≈ < 0.2% of a face
			smallIslands++
		}
	}
	if landN > 0 {
		largestShare = float64(largest) / float64(landN)
	}
	return landFrac, largestShare, smallIslands
}

func TestPhase12LandFractionWithinTolerance(t *testing.T) {
	const S = 128
	for _, master := range []int64{1, 42, 31337, 777, 2026} {
		frac, _, _ := phase12LandStats(t, master, S)
		if math.Abs(frac-0.30) > 0.03 {
			t.Errorf("seed %d: land fraction %v, want 0.30 ± 0.03", master, frac)
		}
	}
}

func TestPhase12LargestLandmassDominates(t *testing.T) {
	const S = 128
	for _, master := range []int64{1, 42, 31337, 777, 2026} {
		_, share, _ := phase12LandStats(t, master, S)
		if share < 0.40 {
			t.Errorf("seed %d: largest landmass holds %v of land, want ≥ 0.40 (assembly 0.5)", master, share)
		}
	}
}

func TestPhase12SmallIslandCeiling(t *testing.T) {
	const S = 128
	for _, master := range []int64{1, 42, 31337, 777, 2026} {
		_, _, islands := phase12LandStats(t, master, S)
		if islands > 40 {
			t.Errorf("seed %d: %d small islands, want ≤ 40", master, islands)
		}
	}
}
```

**Note on `GenerateHeightmapForTest`:** the invariants file lives in `cmd/generate-planet-maps` and uses the public planetgen API. The render-internal heightmap+level pair needs one small exported hook: add to `pkg/planetgen/render/rocky.go`:

```go
// RenderRockyHeightmapWithOceanLevel exposes the normalized heightmap
// plus the effective (possibly quantile-derived) ocean level for
// statistical invariants and tools. Mirrors RenderRockyHeightmap.
func RenderRockyHeightmapWithOceanLevel(profile *types.PlanetProfile, seed int64, S int) (*cubemap.CubeMapF, float64) {
	jitter := jitterForProfile(profile, seed, S) // same construction as RenderRocky
	plates := field.GeneratePlates(profile, seed, S)
	hm, _, lvl := generateRockyHeightmapWithJitter(profile, seed, S, jitter, plates)
	return hm, lvl
}
```

and have the invariants call `render.RenderRockyHeightmapWithOceanLevel` (the cmd test file may already import the render package — check its imports; if it only imports root `planetgen`, add a forwarding func there following however `RenderRockyHeightmap` is currently surfaced — `grep -rn 'RenderRockyHeightmap' pkg/planetgen/planetgen.go cmd/`).

- [ ] **Step 2: Verify the tests actually bite**

Temporarily change `0.30` to `0.10` in `phase12LandStats`'s pin and run:
`go test ./cmd/generate-planet-maps/ -run TestPhase12LandFraction`
Expected: FAIL (fraction ≈ 0.10 vs want 0.30). Revert the sabotage; run again: PASS.

If `TestPhase12LargestLandmassDominates` or the island ceiling fail legitimately, tune in this order: raise `MajorGrowthBias`, lower `EdgeNoiseAmp`, raise `ShelfWidthRad` — re-render and re-run. Do not loosen thresholds; they encode the user's complaint (sizeable continents, few islands).

- [ ] **Step 3: Full test pass + commit**

```bash
go build ./... && go test ./... && golangci-lint run ./...
git add cmd/generate-planet-maps/invariants_test.go pkg/planetgen/render/rocky.go
git commit -m "P12: invariants — land fraction, landmass dominance, island ceiling"
```

---

## Task 13: Explorer Tectonics + Tectonic FX panels

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

No Go tests; verify interactively (Step 3). Follow the `renderBasinPanel` idiom exactly (`makePanel(title, help, bypassStage)`, `panelControls`, `makeAuxBtn`, `makeNumberRow(label, help, value, min, max, step, onCommit)`, `commitProfile`, `renderPanels`, `round2`).

- [ ] **Step 1: Add `renderTectonicsPanel`**

Insert after `renderBasinPanel`'s function definition:

```js
function renderTectonicsPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  if (!profile.crust) {
    profile.crust = { majorPlates: 0, assembly: -1, targetLandFraction: -1, tectonicAge: -1 };
  }
  const c = profile.crust;
  const panel = makePanel('Tectonics (crust rafts)',
    'Phase 12: plates carry cratons of continental crust that decide land vs ocean. ' +
    'majorPlates=0 disables (legacy noise continents). assembly/targetLandFraction/tectonicAge: ' +
    '-1 samples per planet from the configured range; any other value pins it. ' +
    'tectonicAge is the "millions of years" slider: 0 young/sharp, 1 old/soft.',
    'Crust');

  const reset = () => {
    profile.crust = originalProfile && originalProfile.crust
      ? JSON.parse(JSON.stringify(originalProfile.crust))
      : { majorPlates: 0, assembly: -1, targetLandFraction: -1, tectonicAge: -1 };
    commitProfile(profile); renderPanels();
  };
  const clear = () => {
    profile.crust = { majorPlates: 0, assembly: -1, targetLandFraction: -1, tectonicAge: -1 };
    commitProfile(profile); renderPanels();
  };
  const randomize = () => {
    profile.crust = Object.assign({}, c, {
      majorPlates:     5 + Math.floor(Math.random() * 5),
      minorPlates:     2 + Math.floor(Math.random() * 5),
      majorGrowthBias: 4,
      oceanicFraction: round2(0.3 + Math.random() * 0.5),
      assembly:        round2(Math.random()),
      targetLandFraction: round2(0.1 + Math.random() * 0.5),
      tectonicAge:     round2(Math.random()),
    });
    commitProfile(profile); renderPanels();
  };

  const controls = panelControls(panel);
  controls.appendChild(makeAuxBtn('Randomize', 'Roll random in-range tectonics', randomize));
  controls.appendChild(makeAuxBtn('Reset', 'Restore loaded values', reset));
  controls.appendChild(makeAuxBtn('Clear', 'Disable crust stage', clear));

  const num = (label, help, key, min, max, step) =>
    panel.appendChild(makeNumberRow(label, help, c[key] ?? 0, min, max, step, v => {
      c[key] = v; commitProfile(profile);
    }));

  num('Major plates', '0 disables the crust stage entirely', 'majorPlates', 0, 12, 1);
  num('Minor plates', 'gap-filler plates between the majors', 'minorPlates', 0, 8, 1);
  num('Growth bias', 'flood-fill weight of majors vs minors', 'majorGrowthBias', 1, 8, 0.5);
  num('Oceanic fraction', 'fraction of plates carrying no craton', 'oceanicFraction', 0, 1, 0.05);
  num('Assembly', '-1 samples; 0 supercontinent … 1 fragmented', 'assembly', -1, 1, 0.05);
  num('Target land fraction', '-1 samples from [lo, hi]', 'targetLandFraction', -1, 1, 0.01);
  num('Land frac lo', 'sample range lower bound', 'landFracLo', 0, 1, 0.01);
  num('Land frac hi', 'sample range upper bound', 'landFracHi', 0, 1, 0.01);
  num('Tectonic age', '-1 samples; 0 young/sharp … 1 old/soft', 'tectonicAge', -1, 1, 0.05);
  num('Cratons max', 'total craton cap (count grows with assembly)', 'cratonsMax', 2, 14, 1);
  num('Shelf width (rad)', 'continental-shelf falloff half-width', 'shelfWidthRad', 0.01, 0.2, 0.005);
  num('Edge noise amp', 'craton coastline fractality', 'edgeNoiseAmp', 0, 1, 0.05);
  num('Edge noise freq', '', 'edgeNoiseFreq', 0.5, 8, 0.1);
  num('Platform height', 'continental base height', 'platformHeight', 0.3, 0.9, 0.01);
  num('Ocean floor height', 'abyssal base height', 'oceanFloorHeight', 0.05, 0.5, 0.01);

  panels.appendChild(panel);
}

function renderTectonicFXPanel(profile, panels) {
  if (profile.Renderer !== 'rocky') return;
  if (!profile.tectonicFX) profile.tectonicFX = {};
  const c = profile.tectonicFX;
  const panel = makePanel('Tectonic FX (boundary effects)',
    'Crust-aware boundary height effects: collision belts (cont-cont), ' +
    'cordillera+trench (ocean-cont), island arcs (oce-oce), mid-ocean ridges, ' +
    'continental rifts, transform faults. Active only when Tectonics is enabled.',
    'TectonicFX');

  const reset = () => {
    profile.tectonicFX = originalProfile && originalProfile.tectonicFX
      ? JSON.parse(JSON.stringify(originalProfile.tectonicFX)) : {};
    commitProfile(profile); renderPanels();
  };
  const controls = panelControls(panel);
  controls.appendChild(makeAuxBtn('Reset', 'Restore loaded values', reset));

  const num = (label, help, key, min, max, step) =>
    panel.appendChild(makeNumberRow(label, help, c[key] ?? 0, min, max, step, v => {
      c[key] = v; commitProfile(profile);
    }));

  num('Belt amp', 'cont-cont collision belt height', 'beltAmp', 0, 1, 0.02);
  num('Belt width (km)', '', 'beltWidthKm', 100, 3000, 50);
  num('Belt freq', 'ridged noise frequency in belts', 'beltFreq', 0.5, 8, 0.1);
  num('Belt octaves', '', 'beltOctaves', 1, 8, 1);
  num('Cordillera amp', 'coastal mountains on subduction coasts', 'cordAmp', 0, 1, 0.02);
  num('Cordillera width (km)', '', 'cordWidthKm', 100, 1500, 25);
  num('Trench depth', 'offshore subduction trench', 'trenchDepth', 0, 0.5, 0.01);
  num('Trench width (km)', '', 'trenchWidthKm', 50, 800, 10);
  num('Arc amp', 'oce-oce island arc height', 'arcAmp', 0, 1, 0.02);
  num('Arc width (km)', '', 'arcWidthKm', 50, 1000, 10);
  num('Ridge amp', 'mid-ocean ridge rise', 'ridgeAmp', 0, 0.3, 0.01);
  num('Ridge width (km)', '', 'ridgeWidthKm', 100, 2000, 50);
  num('Rift depth', 'continental rift valley', 'riftDepth', 0, 0.4, 0.01);
  num('Rift width (km)', '', 'riftWidthKm', 50, 1000, 10);
  num('Rift shoulder', 'shoulder uplift fraction', 'riftShoulder', 0, 1, 0.05);
  num('Transform amp', 'fault-zone roughness', 'transformAmp', 0, 0.2, 0.005);
  num('Transform width (km)', '', 'transformWidthKm', 50, 600, 10);
  num('Activity freq', 'per-boundary energy variation', 'activityFreq', 0.2, 5, 0.1);

  panels.appendChild(panel);
}
```

**JSON key caution:** the Go fields use lowercase JSON tags (`crust`, `majorPlates`, `tectonicFX`, `beltAmp`, …) per Task 1. Verify against an actual profile dump: load the explorer, dump `JSON.stringify(profile)` in the console (or `go run` a one-liner marshalling the terran default) and confirm key casing before wiring the panel. Note the no-omitempty trio (`assembly`, `targetLandFraction`, `tectonicAge`) is always present in dumps.

- [ ] **Step 2: Register the panels**

In the `renderPanels` panel list (after `renderBasinPanel(profile, panels);`):

```js
  renderTectonicsPanel(profile, panels);
  renderTectonicFXPanel(profile, panels);
```

- [ ] **Step 3: Verify interactively**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm
go run ./cmd/planet-explorer
```

(Verify build/serve commands against `cmd/planet-explorer/README.md` and use its documented invocation.) Open the explorer, pick terran:
1. Both new panels render with terran defaults.
2. Dragging Tectonic age re-renders with softer/sharper relief.
3. Assembly 0 → one merged landmass with an interior mountain seam; assembly 1 → scattered smaller continents.
4. Clear on Tectonics → legacy render (the pre-Phase-12 look).
5. The "TectonicFX" debug stage appears in the debug grid, and "Crust" shows the continental mask.

- [ ] **Step 4: Commit**

```bash
golangci-lint run ./... 2>/dev/null || true   # JS not linted by golangci; ensure Go untouched
git add cmd/planet-explorer/web/app.js
git commit -m "P12: explorer Tectonics + Tectonic FX panels"
```

---

## Task 14: Re-seed profiles, goldens, docs, perf check

**Files:**
- Modify: `cmd/generate-planet-maps/README.md`, `cmd/planet-explorer/USER_GUIDE.md`
- Regenerate: `cmd/generate-planet-maps/testdata/golden/*.png`, `kb/data/planet-profiles/*.json` (or wherever the seeder writes — check `cmd/tools/seed-planet-profiles`)

- [ ] **Step 1: Regenerate goldens**

```bash
go test ./cmd/generate-planet-maps/... -run TestGolden -update
go test ./cmd/generate-planet-maps/...
```

Open the regenerated goldens for terran / super_terran / oceanic / tundra / glacial / arid side-by-side with `git diff --stat` confirming only those six (plus their cloud/night variants if the heightmap feeds them) changed. Use `go run ./cmd/tools/planet-image-diff` per its README for before/after sheets. **Manual gate:** continents sizeable, belts along collision seams, trench-darkened subduction coasts, oceanic type = arcs + microcontinents.

- [ ] **Step 2: Re-seed the committed profile JSONs**

Run the seeder exactly as its README documents (`cmd/tools/seed-planet-profiles/README.md` exists per the Phase 5 commits):

```bash
go run ./cmd/tools/seed-planet-profiles   # plus whatever flags its README requires
go test ./pkg/planetgen/profilejson/...   # drift guard must pass
```

`handTuned: true` files must NOT change — verify with `git diff --name-only | xargs grep -l '"handTuned": true'` returning empty.

- [ ] **Step 3: Perf check (≤ 2× budget)**

The render package has a perf test pattern (`rocky_phase8_perf_test.go`). Time a terran render before/after at S=1024:

```bash
git stash && go test ./pkg/planetgen/render/ -run TestPhase8 -v 2>&1 | tail -5 && git stash pop
go test ./pkg/planetgen/render/ -run TestPhase8 -v 2>&1 | tail -5
```

Or, simpler and more direct: `time go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/p12_perf.png` on `main..HEAD^` vs `HEAD`. If crust-path wall time exceeds 2× the legacy path, the JFA count is the lever: merge the five FX classifications into two `JumpFloodFromMaskWithValue` calls by packing class id into the value channel (class = floor(value)/8, mag = frac) — only do this if the budget is actually blown.

- [ ] **Step 4: Docs**

- `cmd/generate-planet-maps/README.md`: add a "Phase 12 — tectonic continents" section: what the crust path is, the `MajorPlates == 0` off switch, the `-1` sample sentinels, the derived sea level, which six types enable it.
- `cmd/planet-explorer/USER_GUIDE.md`: document both panels, the tectonicAge slider semantics, and the assembly axis with the three worked examples from Task 13 Step 3.

- [ ] **Step 5: Full gate + commit**

```bash
go build ./... && go test ./... && golangci-lint run ./...
GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/...
git add -A
git commit -m "P12: goldens + reseeded profiles + docs + perf validation"
```

---

## Self-review checklist (run after Task 14)

1. **Spec coverage:** §3 pipeline (Task 10), §4 plates/cratons (Tasks 3–5), §5 boundary FX + age (Tasks 7–8), §6 sea level + defaults (Tasks 9, 11), §7 explorer/tests/migration (Tasks 12–14). Migration deviation (no schema bump) documented in File structure.
2. **Legacy byte-identity:** all pre-existing tests green at every task boundary; goldens for the seven non-crust types unchanged in Task 14's diff.
3. **Wasm + lint gates:** green at Task 14.
4. **The user's complaint, mechanically encoded:** Task 12's three invariants are the acceptance criteria — land fraction hits target, one dominant landmass at mid assembly, island count capped.
