# Tectonics Land-mass Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Derive elevation and a water-budget sea level from the crust the tectonics simulation already carries, erode land inside the step loop, and show real coastlines on the lab viewer's slider.

**Architecture:** Thickness stays the only simulated state. `Elevation` is a pure function of thickness and crust age evaluated per step and per keyframe; `SeaLevelFor` fills a fixed water volume into the derived hollows; `erode` moves thickness downhill from land pixels. The bundle gains a 16-bit elevation image plus sea level and land share per frame (version 2); the viewer's hypsometric layer reads them. Test-only isometric renders are produced on `go test -vis`.

**Tech Stack:** Go 1.24 (`pkg/tectonics`, `cmd/tectonics-lab`), raw WebGL viewer (`cmd/tectonics-lab/web`), Python 3 + numpy + Pillow for the stats and Earth exporter (`~/gplates-venv/bin/python3`).

**Spec:** `docs/superpowers/specs/2026-10-10-tectonics-landmass-design.md`

## Global Constraints

- Go 1.24+: range over integers, `b.Loop()` in benchmarks. Every Go change passes `go build ./...`, `go test ./pkg/tectonics/... ./cmd/tectonics-lab/...` and `golangci-lint run ./pkg/tectonics/... ./cmd/tectonics-lab/...` with no new findings.
- Thickness is the only simulated state (spec decision "State", option A). Elevation is never stored on `State`; it is derived.
- Resolution split (roadmap): geology runs at face 128/256; nothing in this layer assumes a larger face. `Validate` keeps `Face` in 8..1024.
- Bundle version 2: plate, thickness and feature images are unchanged; `elevation.png` packs `clamp(round(h_km × 1000) + 32768, 0, 65535)` as red = high byte, green = low byte, blue 0, alpha 255. `ReadManifest` accepts versions 1 and 2.
- Sea level is a global height; every pixel with `elev < L` is water; connectivity is ignored.
- Erosion: land pixels only, conservative except `SedimentOceanFactor` loss into deep ocean, Jacobi-style (computed from the step's starting thickness, applied in one pass), deterministic.
- Seeds: the water factor uses domain `water.factor`; nothing else in this layer draws random numbers.
- Test visuals are produced only with `go test -vis`, written to `pkg/tectonics/testdata/vis/` (git-ignored), no new dependencies.
- Commits: named paths only (`git add <paths>`), never `-A`. Trailer `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`. Built binaries go in `bin/`.
- Subagents never open browsers and never run `rm -rf`.
- Working tree: `/home/robert/spacemolt/kb-tectonics`, branch `tectonics/landmass`.

---

## File map

| File | Responsibility |
| --- | --- |
| `pkg/tectonics/params.go` | new knobs on `Params`, archetype columns (land share, water factor range), defaults |
| `pkg/tectonics/sim.go` | `Validate` rules for the knobs; `Run` sets the water budget; `Snapshot` fills elevation fields; `Frame` gains fields |
| `pkg/tectonics/elevation.go` (new) | `thExpected`, `elevationAt`, `Elevation`, `InitialAges` |
| `pkg/tectonics/grid.go` | `AreaWeights(S)` cached solid angles |
| `pkg/tectonics/thickness.go` | `remapBands` called from `GenerateThickness` |
| `pkg/tectonics/sealevel.go` (new) | `drawWaterFactor`, `HollowVolume`, `SeaLevelFor`, `LandShare` |
| `pkg/tectonics/step.go` | `State.Water`, `State.SeaLevel`; `NewState` calls `InitialAges` and sets the factor-1 budget; `Step` calls `erode` then refreshes `SeaLevel` |
| `pkg/tectonics/erosion.go` (new) | `(*State).erode` |
| `pkg/tectonics/vis_test.go` (new) | `-vis` flag, `renderIso`, `diffPanel`, `writeVis`, index.html |
| `pkg/tectonics/{elevation,sealevel,erosion}_test.go` (new), `thickness_test.go`, `sim_test.go` | tests |
| `pkg/tectonics/bundle.go` | version 2, `elevation.png`, `FrameInfo`/`Manifest` fields, `ReadManifest` range |
| `cmd/tectonics-lab/run.go` | copies `WaterKm3` into the manifest |
| `cmd/tectonics-lab/golden_test.go`, `testdata/golden_face64.json` | v10 recipe hashing elevation and sea level |
| `cmd/tectonics-lab/web/app.js`, `index.html` | elevation texture, hypsometric shading, coast, status |
| `cmd/tectonics-lab/web_test.go` | asserts the viewer mentions `elevation.png` |
| `scripts/earth_plates_export.py` | version 2 Earth bundle |
| `scripts/tectonics_stats.py`, `scripts/tectonics_seed_sweep.py` | land/elevation metrics |
| `.gitignore` | `pkg/tectonics/testdata/vis/` |

---

### Task 1: Knobs, archetype columns and validation

**Files:**
- Modify: `pkg/tectonics/params.go`
- Modify: `pkg/tectonics/sim.go:35-73` (`Validate`)
- Test: `pkg/tectonics/params_test.go` (create if absent; otherwise append)

**Interfaces:**
- Produces: the `Params` fields below, read by every later task by exact name: `LandShareT0, LandBandMin, OceanBandMax, LandRankPow, OceanRankPow, ContinentAgeMyr, OceanAgeCapMyr, ElevShelfKm, ElevPeakKm, ElevPow, RidgeDepthKm, SubsidenceKm, AbyssKm, OceanReliefKm, ElevBlend, WaterFactorMin, WaterFactorMax, ErosionRate, ErosionMaxPerStep, SedimentOceanFactor` (all `float64`).

- [ ] **Step 1: Write the failing test**

Create `pkg/tectonics/params_test.go` (or append if the file exists):

```go
package tectonics

import "testing"

func TestLandmassDefaultsAndValidation(t *testing.T) {
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	if p.LandShareT0 != 0.30 || p.WaterFactorMin != 0.9 || p.WaterFactorMax != 1.1 {
		t.Fatalf("terran land/water defaults: %g %g %g", p.LandShareT0, p.WaterFactorMin, p.WaterFactorMax)
	}
	if p.ElevPeakKm != 8 || p.RidgeDepthKm != -2.5 || p.ErosionRate != 0.01 {
		t.Fatalf("elevation/erosion defaults: %g %g %g", p.ElevPeakKm, p.RidgeDepthKm, p.ErosionRate)
	}
	if err := Validate(p); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	o, _ := DefaultParams("oceanic")
	if o.LandShareT0 != 0.15 || o.WaterFactorMin != 1.3 {
		t.Fatalf("oceanic columns: %g %g", o.LandShareT0, o.WaterFactorMin)
	}
	bad := []struct {
		name string
		mut  func(*Params)
	}{
		{"LandShareT0 0", func(q *Params) { q.LandShareT0 = 0 }},
		{"bands cross threshold", func(q *Params) { q.OceanBandMax = 0.6 }},
		{"LandBandMin below threshold", func(q *Params) { q.LandBandMin = 0.4 }},
		{"ElevPow 0", func(q *Params) { q.ElevPow = 0 }},
		{"shelf above peak", func(q *Params) { q.ElevShelfKm = 9 }},
		{"abyss above ridge", func(q *Params) { q.AbyssKm = -1 }},
		{"ElevBlend 0", func(q *Params) { q.ElevBlend = 0 }},
		{"water range reversed", func(q *Params) { q.WaterFactorMax = 0.5 }},
		{"water min 0", func(q *Params) { q.WaterFactorMin = 0 }},
		{"ErosionMaxPerStep 0.3", func(q *Params) { q.ErosionMaxPerStep = 0.3 }},
		{"SedimentOceanFactor 2", func(q *Params) { q.SedimentOceanFactor = 2 }},
		{"negative erosion", func(q *Params) { q.ErosionRate = -1 }},
	}
	for _, b := range bad {
		q := p
		b.mut(&q)
		if err := Validate(q); err == nil {
			t.Errorf("%s: Validate accepted an invalid value", b.name)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd /home/robert/spacemolt/kb-tectonics && go test ./pkg/tectonics/ -run TestLandmassDefaultsAndValidation`
Expected: compile error (`p.LandShareT0 undefined`).

- [ ] **Step 3: Add the knobs and archetype columns**

In `pkg/tectonics/params.go`, add to `Params` after the `RiftRetries` field:

```go
	// Land-mass layer, see docs/superpowers/specs/2026-10-10-tectonics-landmass-design.md
	LandShareT0                 float64 // share of the sphere remapped to continent at step 0 (archetype)
	LandBandMin, OceanBandMax   float64 // thickness bands after the step-0 remap
	LandRankPow, OceanRankPow   float64 // rank exponents inside the bands
	ContinentAgeMyr             float64 // initial crust age of continental pixels
	OceanAgeCapMyr              float64 // cap on the initial ocean age inferred from thickness
	ElevShelfKm, ElevPeakKm     float64 // continental elevation at the threshold and at thickness 1
	ElevPow                     float64 // shape of the continental curve
	RidgeDepthKm, SubsidenceKm  float64 // ocean floor at age 0; km sunk per sqrt(Myr)
	AbyssKm                     float64 // floor of the subsidence
	OceanReliefKm               float64 // km per unit of thickness deviation from the age-predicted ocean thickness
	ElevBlend                   float64 // half-width in thickness of the ocean/continent blend
	WaterFactorMin, WaterFactorMax float64 // seeded multiplier on the step-0 hollow volume (archetype)
	ErosionRate                 float64 // fraction of the elevation difference (over ElevPeakKm) moved per Myr
	ErosionMaxPerStep           float64 // cap on the fraction a pixel gives away per step
	SedimentOceanFactor         float64 // share kept when the receiver is deep ocean
```

Extend `archetypeRow` and the table:

```go
type archetypeRow struct {
	majorMin, majorMax, minorMin, minorMax  int
	speedMin, speedMax, timeline, crustBias float64
	landShare, waterMin, waterMax           float64
}

var archetypes = map[string]archetypeRow{
	"terran":       {6, 9, 10, 16, 3, 10, 800, 0, 0.30, 0.9, 1.1},
	"super_terran": {6, 9, 10, 16, 3, 10, 800, 0, 0.35, 0.9, 1.1},
	"oceanic":      {6, 9, 10, 16, 3, 10, 800, -0.15, 0.15, 1.3, 1.6},
	"arid":         {4, 7, 8, 12, 2, 5, 600, 0, 0.45, 0.5, 0.7},
	"tundra":       {4, 7, 8, 12, 2, 5, 600, 0, 0.30, 0.8, 1.0},
	"glacial":      {4, 7, 8, 12, 2, 5, 600, 0, 0.30, 0.8, 1.0},
	"scorched":     {8, 12, 14, 20, 6, 12, 600, -0.2, 0.40, 0.3, 0.6},
	"lava_world":   {8, 12, 14, 20, 6, 12, 600, -0.2, 0.40, 0.2, 0.5},
	"ice_world":    {2, 4, 2, 6, 0.2, 1, 600, 0.1, 0.30, 0.8, 1.0},
}
```

In `DefaultParams`, add to the returned literal after `RiftRetries: 3,`:

```go
		LandShareT0: a.landShare, LandBandMin: 0.55, OceanBandMax: 0.45, LandRankPow: 3, OceanRankPow: 1,
		ContinentAgeMyr: 1000, OceanAgeCapMyr: 180,
		ElevShelfKm: -0.2, ElevPeakKm: 8, ElevPow: 1.6,
		RidgeDepthKm: -2.5, SubsidenceKm: 0.35, AbyssKm: -6, OceanReliefKm: 20, ElevBlend: 0.05,
		WaterFactorMin: a.waterMin, WaterFactorMax: a.waterMax,
		ErosionRate: 0.0015, ErosionMaxPerStep: 0.2, SedimentOceanFactor: 0.5, // ErosionRate corrected by Task 7b
```

In `Validate` (`pkg/tectonics/sim.go`), add cases before the closing `}` of the switch:

```go
	case p.LandShareT0 <= 0 || p.LandShareT0 >= 0.9:
		return fmt.Errorf("LandShareT0 %g outside (0, 0.9)", p.LandShareT0)
	case !(p.OceanBandMax < p.ContinentalThreshold && p.ContinentalThreshold < p.LandBandMin):
		return fmt.Errorf("bands %g < threshold %g < %g violated", p.OceanBandMax, p.ContinentalThreshold, p.LandBandMin)
	case p.LandRankPow <= 0 || p.OceanRankPow <= 0:
		return fmt.Errorf("rank powers %g/%g must be positive", p.LandRankPow, p.OceanRankPow)
	case p.ContinentAgeMyr < 0 || p.OceanAgeCapMyr < 0:
		return fmt.Errorf("initial ages %g/%g must be non-negative", p.ContinentAgeMyr, p.OceanAgeCapMyr)
	case p.ElevShelfKm >= p.ElevPeakKm:
		return fmt.Errorf("ElevShelfKm %g must be below ElevPeakKm %g", p.ElevShelfKm, p.ElevPeakKm)
	case p.ElevPow <= 0:
		return fmt.Errorf("ElevPow %g must be positive", p.ElevPow)
	case p.AbyssKm > p.RidgeDepthKm:
		return fmt.Errorf("AbyssKm %g above RidgeDepthKm %g", p.AbyssKm, p.RidgeDepthKm)
	case p.SubsidenceKm < 0 || p.OceanReliefKm < 0:
		return fmt.Errorf("SubsidenceKm %g / OceanReliefKm %g must be non-negative", p.SubsidenceKm, p.OceanReliefKm)
	case p.ElevBlend <= 0 || p.ElevBlend >= 0.2:
		return fmt.Errorf("ElevBlend %g outside (0, 0.2)", p.ElevBlend)
	case p.WaterFactorMin <= 0 || p.WaterFactorMax < p.WaterFactorMin:
		return fmt.Errorf("water factor range %g-%g invalid", p.WaterFactorMin, p.WaterFactorMax)
	case p.ErosionRate < 0:
		return fmt.Errorf("ErosionRate %g must be non-negative", p.ErosionRate)
	case p.ErosionMaxPerStep <= 0 || p.ErosionMaxPerStep > 0.25:
		return fmt.Errorf("ErosionMaxPerStep %g outside (0, 0.25]", p.ErosionMaxPerStep)
	case p.SedimentOceanFactor < 0 || p.SedimentOceanFactor > 1:
		return fmt.Errorf("SedimentOceanFactor %g outside 0..1", p.SedimentOceanFactor)
```

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/tectonics/ && golangci-lint run ./pkg/tectonics/...`
Expected: PASS, no findings. (The golden test in `cmd/tectonics-lab` still passes: the knobs are unused so far.)

- [ ] **Step 5: Commit**

```bash
git add pkg/tectonics/params.go pkg/tectonics/sim.go pkg/tectonics/params_test.go
git commit -m "feat(tectonics): land-mass knobs, archetype land share and water factor, validation

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Elevation, initial ages and area weights

**Files:**
- Create: `pkg/tectonics/elevation.go`
- Modify: `pkg/tectonics/grid.go` (add `AreaWeights`)
- Modify: `pkg/tectonics/step.go:39-43` (`NewState` uses `InitialAges`)
- Test: `pkg/tectonics/elevation_test.go`

**Interfaces:**
- Consumes: Task 1 knobs.
- Produces: `func thExpected(p Params, ageSteps float64) float64`; `func elevationAt(p Params, th float64, ageSteps uint16) float64`; `func Elevation(th *Grid[float64], age *Grid[uint16], p Params) *Grid[float64]` (km); `func InitialAges(th *Grid[float64], p Params) *Grid[uint16]`; `func AreaWeights(S int) []float64` (steradians per pixel, cached, sums to 4π).

- [ ] **Step 1: Write the failing tests**

Create `pkg/tectonics/elevation_test.go`:

```go
package tectonics

import (
	"math"
	"testing"
)

func TestElevationContinentalMonotone(t *testing.T) {
	p := testParams(t, 16)
	prev := math.Inf(-1)
	for k := 0; k <= 100; k++ {
		th := p.ContinentalThreshold + (1-p.ContinentalThreshold)*float64(k)/100
		h := elevationAt(p, th, 0)
		if h <= prev {
			t.Fatalf("th %.3f: h %.4f not above %.4f", th, h, prev)
		}
		prev = h
	}
	if top := elevationAt(p, 1, 0); math.Abs(top-p.ElevPeakKm) > 1e-9 {
		t.Fatalf("thickness 1 gives %.4f km, want ElevPeakKm %g", top, p.ElevPeakKm)
	}
}

func TestElevationOceanSinksWithAge(t *testing.T) {
	p := testParams(t, 16)
	prev := math.Inf(1)
	for steps := uint16(0); steps <= 400; steps += 10 {
		th := thExpected(p, float64(steps)) // zero deviation
		h := elevationAt(p, th, steps)
		if h > prev {
			t.Fatalf("age %d steps: h %.4f rose above %.4f", steps, h, prev)
		}
		if h < p.AbyssKm-1e-9 {
			t.Fatalf("age %d steps: h %.4f below AbyssKm %g", steps, h, p.AbyssKm)
		}
		prev = h
	}
	if h0 := elevationAt(p, thExpected(p, 0), 0); math.Abs(h0-p.RidgeDepthKm) > 1e-9 {
		t.Fatalf("age 0 ridge sits at %.4f, want %g", h0, p.RidgeDepthKm)
	}
}

func TestElevationBranchesMeet(t *testing.T) {
	p := testParams(t, 16)
	T := p.ContinentalThreshold
	capSteps := uint16(p.OceanAgeCapMyr / p.MyrPerStep())
	for _, age := range []uint16{0, capSteps} {
		lo, hi := elevationAt(p, T-1e-3, age), elevationAt(p, T+1e-3, age)
		if math.Abs(hi-lo) > 0.3 {
			t.Fatalf("age %d: jump %.3f km across the threshold (%.3f vs %.3f)", age, hi-lo, lo, hi)
		}
	}
}

func TestElevationTrenchBelowAgeDepth(t *testing.T) {
	p := testParams(t, 16)
	const age = 50
	expected := thExpected(p, age)
	if elevationAt(p, expected-p.TrenchDepth, age) >= elevationAt(p, expected, age) {
		t.Fatal("a trench-thinned pixel must sit below the age-predicted depth")
	}
}

func TestInitialAgesRoundTrip(t *testing.T) {
	p := testParams(t, 8)
	th := NewGrid[float64](8)
	want := []uint16{0, 10, 50}
	for i := range th.Cells {
		th.Cells[i] = thExpected(p, float64(want[i%3]))
	}
	th.Cells[0] = 0.9 // continent
	th.Cells[1] = 0.1 // thinner than ridge crust
	th.Cells[2] = p.OceanBandMax // thicker than the cap explains
	age := InitialAges(th, p)
	for i := 3; i < th.Len(); i++ {
		if age.Cells[i] != want[i%3] {
			t.Fatalf("pixel %d: age %d, want %d", i, age.Cells[i], want[i%3])
		}
	}
	if cont := uint16(p.ContinentAgeMyr / p.MyrPerStep()); age.Cells[0] != cont {
		t.Fatalf("continent age %d, want %d", age.Cells[0], cont)
	}
	if age.Cells[1] != 0 {
		t.Fatalf("sub-ridge thickness age %d, want 0", age.Cells[1])
	}
	if capSteps := uint16(p.OceanAgeCapMyr / p.MyrPerStep()); age.Cells[2] != capSteps {
		t.Fatalf("thick ocean age %d, want cap %d", age.Cells[2], capSteps)
	}
}

func TestAreaWeightsSumToSphere(t *testing.T) {
	for _, S := range []int{8, 32} {
		sum := 0.0
		for _, w := range AreaWeights(S) {
			if w <= 0 {
				t.Fatalf("S=%d: non-positive weight %g", S, w)
			}
			sum += w
		}
		if math.Abs(sum-4*math.Pi) > 1e-9 {
			t.Fatalf("S=%d: weights sum to %.12f, want 4π", S, sum)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/tectonics/ -run 'TestElevation|TestInitialAges|TestAreaWeights'`
Expected: compile errors (`elevationAt`, `thExpected`, `InitialAges`, `AreaWeights` undefined).

- [ ] **Step 3: Implement elevation.go**

```go
package tectonics

import "math"

// thExpected is the thickness the oceanic thickening law in Step predicts
// after ageSteps steps of cooling from ridge crust.
func thExpected(p Params, ageSteps float64) float64 {
	return p.RidgeThickness + p.OceanicThickening*math.Sqrt(ageSteps)
}

func smoothstep(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return x * x * (3 - 2*x)
}

// elevationAt returns the height in km of one pixel from its thickness and
// crust age: a continental curve above ContinentalThreshold, age-driven
// subsidence plus thickness relief below it, blended across ElevBlend.
func elevationAt(p Params, th float64, ageSteps uint16) float64 {
	T := p.ContinentalThreshold
	x := math.Max(0, (th-T)/(1-T))
	hc := p.ElevShelfKm + (p.ElevPeakKm-p.ElevShelfKm)*math.Pow(x, p.ElevPow)

	ageMyr := float64(ageSteps) * p.MyrPerStep()
	base := math.Max(p.AbyssKm, p.RidgeDepthKm-p.SubsidenceKm*math.Sqrt(ageMyr))
	dev := math.Max(-0.1, math.Min(0.1, th-thExpected(p, float64(ageSteps))))
	ho := base + p.OceanReliefKm*dev

	w := smoothstep((th - (T - p.ElevBlend)) / (2 * p.ElevBlend))
	return (1-w)*ho + w*hc
}

// Elevation derives the height in km of every pixel. It is a pure function
// of thickness and crust age and is never stored on State.
func Elevation(th *Grid[float64], age *Grid[uint16], p Params) *Grid[float64] {
	h := NewGrid[float64](th.S)
	for i, v := range th.Cells {
		h.Cells[i] = elevationAt(p, v, age.Cells[i])
	}
	return h
}

func clampU16(x float64) uint16 {
	if x < 0 {
		return 0
	}
	if x > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(x)
}

// InitialAges gives step-0 crust ages: continents are ContinentAgeMyr old;
// ocean age is inferred by inverting the thickening law (thicker ocean is
// older), capped at OceanAgeCapMyr.
func InitialAges(th *Grid[float64], p Params) *Grid[uint16] {
	age := NewGrid[uint16](th.S)
	myr := p.MyrPerStep()
	cont := clampU16(math.Round(p.ContinentAgeMyr / myr))
	capSteps := p.OceanAgeCapMyr / myr
	for i, v := range th.Cells {
		if v >= p.ContinentalThreshold {
			age.Cells[i] = cont
			continue
		}
		steps := 0.0
		if v > p.RidgeThickness && p.OceanicThickening > 0 {
			d := (v - p.RidgeThickness) / p.OceanicThickening
			steps = d * d
		}
		age.Cells[i] = clampU16(math.Round(math.Min(steps, capSteps)))
	}
	return age
}
```

Note: the round-trip test relies on `math.Round` in `InitialAges` because `((th−R)/O)²` is not exactly integral in floating point.

- [ ] **Step 4: Add AreaWeights to grid.go**

Below the `Dirs` function in `pkg/tectonics/grid.go`:

```go
var (
	areaMu    sync.Mutex
	areaCache = map[int][]float64{}
)

// cornerAngle is the solid angle of the rectangle [0,u]×[0,v] on the plane
// z = 1 seen from the origin.
func cornerAngle(u, v float64) float64 { return math.Atan(u * v / math.Sqrt(1+u*u+v*v)) }

// rectSolidAngle is the solid angle of [u0,u1]×[v0,v1] on the plane z = 1.
func rectSolidAngle(u0, u1, v0, v1 float64) float64 {
	return cornerAngle(u1, v1) - cornerAngle(u0, v1) - cornerAngle(u1, v0) + cornerAngle(u0, v0)
}

// AreaWeights returns the solid angle in steradians of every pixel; the six
// faces sum to 4π. Cached per face size.
func AreaWeights(S int) []float64 {
	areaMu.Lock()
	defer areaMu.Unlock()
	if w, ok := areaCache[S]; ok {
		return w
	}
	g := NewGrid[struct{}](S)
	w := make([]float64, g.Len())
	for i := range g.Len() {
		_, px, py := g.Addr(i)
		u0, u1 := 2*float64(px)/float64(S)-1, 2*float64(px+1)/float64(S)-1
		v0, v1 := 2*float64(py)/float64(S)-1, 2*float64(py+1)/float64(S)-1
		w[i] = rectSolidAngle(u0, u1, v0, v1)
	}
	areaCache[S] = w

	return w
}
```

- [ ] **Step 5: Make NewState use InitialAges**

In `pkg/tectonics/step.go` replace `NewState`:

```go
// NewState wraps an initial thickness field, plate labelling and motions as
// step 0, with crust ages from InitialAges and no features.
func NewState(p Params, th *Grid[float64], labels *Grid[int32], plates []Plate, motions []Motion) *State {
	S := th.S
	return &State{P: p, Th: th, Labels: labels, Age: InitialAges(th, p),
		FeatType: NewGrid[uint8](S), FeatAge: NewGrid[uint8](S), Plates: plates, Motions: motions}
}
```

Also update the doc comment on `State.Age` to `// steps since the crust formed (InitialAges at step 0)`.

- [ ] **Step 6: Run the package tests and lint**

Run: `go test ./pkg/tectonics/ && golangci-lint run ./pkg/tectonics/...`
Expected: PASS. If an existing step test asserts on crust ages after `NewState` (ages were all zero before), update that assertion to the `InitialAges` value and say so in the report; do not weaken the test otherwise.

`cmd/tectonics-lab`'s golden test now fails (initial ages change the thickening). That is expected until Task 7 rebakes it; do not run `-update` here.

- [ ] **Step 7: Commit**

```bash
git add pkg/tectonics/elevation.go pkg/tectonics/elevation_test.go pkg/tectonics/grid.go pkg/tectonics/step.go
git commit -m "feat(tectonics): elevation from crust, initial crust ages, pixel area weights

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Step-0 continent remap

**Files:**
- Modify: `pkg/tectonics/thickness.go:33-63` (`GenerateThickness`), add `remapBands`
- Test: `pkg/tectonics/thickness_test.go`

**Interfaces:**
- Consumes: `AreaWeights` (Task 2), knobs `LandShareT0, LandBandMin, OceanBandMax, LandRankPow, OceanRankPow, RidgeThickness`.
- Produces: `func remapBands(th *Grid[float64], p Params)` (in place), called by `GenerateThickness` after `normalize` and before the relax loop.

- [ ] **Step 1: Write the failing test**

Append to `pkg/tectonics/thickness_test.go`:

```go
func landShareByArea(th *Grid[float64], threshold float64) float64 {
	w := AreaWeights(th.S)
	land := 0.0
	for i, v := range th.Cells {
		if v >= threshold {
			land += w[i]
		}
	}
	return land / (4 * math.Pi)
}

func TestRemapLandShare(t *testing.T) {
	p := testParams(t, 32)
	th := NewGrid[float64](32)
	rng := newRNG(7, "test.remap")
	for i := range th.Cells {
		th.Cells[i] = rng.Float64()
	}
	normalize(th)
	remapBands(th, p)
	if got := landShareByArea(th, p.ContinentalThreshold); math.Abs(got-p.LandShareT0) > 0.01 {
		t.Fatalf("land share %.3f, want %.2f ± 0.01", got, p.LandShareT0)
	}
	for i, v := range th.Cells {
		if v < p.ContinentalThreshold && v > p.OceanBandMax+1e-12 {
			t.Fatalf("pixel %d: ocean thickness %.4f above OceanBandMax", i, v)
		}
		if v >= p.ContinentalThreshold && v < p.LandBandMin-1e-12 {
			t.Fatalf("pixel %d: continent thickness %.4f below LandBandMin", i, v)
		}
		if v < p.RidgeThickness-1e-12 || v > 1+1e-12 {
			t.Fatalf("pixel %d: thickness %.4f outside [RidgeThickness, 1]", i, v)
		}
	}
	for range 200 {
		if Relax(th, p.MaxNeighborDelta, 1) == 0 {
			break
		}
	}
	if d := maxNeighborDelta(th); d > p.MaxNeighborDelta+1e-9 {
		t.Fatalf("gradient cap violated after relax: %.4f", d)
	}
}

func TestGenerateThicknessStartsWithContinents(t *testing.T) {
	p := testParams(t, 32)
	th := GenerateThickness(p, 2026)
	got := landShareByArea(th, p.ContinentalThreshold)
	if math.Abs(got-p.LandShareT0) > 0.05 {
		t.Fatalf("land share after remap+relax %.3f, want %.2f ± 0.05", got, p.LandShareT0)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run 'TestRemapLandShare|TestGenerateThicknessStartsWithContinents'`
Expected: compile error (`remapBands` undefined).

- [ ] **Step 3: Implement remapBands and call it**

Add to `pkg/tectonics/thickness.go` (imports gain `slices` and `cmp` if not present):

```go
// remapBands pushes the normalised field into an ocean band
// [RidgeThickness, OceanBandMax] and a continent band [LandBandMin, 1] by
// rank, so LandShareT0 of the sphere (by area) starts as continent. The map
// is monotone, so the watershed's basins and divides keep their order.
func remapBands(th *Grid[float64], p Params) {
	n := th.Len()
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	slices.SortFunc(idx, func(a, b int) int {
		if c := cmp.Compare(th.Cells[a], th.Cells[b]); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	w := AreaWeights(th.S)
	cut, cum := n, 0.0
	for k := n - 1; k >= 0; k-- {
		cum += w[idx[k]] / (4 * math.Pi)
		if cum >= p.LandShareT0 {
			cut = k
			break
		}
	}
	nOcean, nLand := float64(cut), float64(n-cut)
	for k, i := range idx {
		if k < cut {
			r := float64(k) / math.Max(1, nOcean-1)
			th.Cells[i] = p.RidgeThickness + (p.OceanBandMax-p.RidgeThickness)*math.Pow(r, p.OceanRankPow)
		} else {
			r := float64(k-cut) / math.Max(1, nLand-1)
			th.Cells[i] = p.LandBandMin + (1-p.LandBandMin)*math.Pow(r, p.LandRankPow)
		}
	}
}
```

In `GenerateThickness`, change

```go
	normalize(th)
	for range 200 {
```

to

```go
	normalize(th)
	remapBands(th, p)
	for range 200 {
```

and update its doc comment: `// ... normalised to [0,1], remapped into ocean and continent bands (remapBands), then relaxed ...`.

- [ ] **Step 4: Run the package tests and lint**

Run: `go test ./pkg/tectonics/ && golangci-lint run ./pkg/tectonics/...`
Expected: PASS. Plate-building tests that run on `GenerateThickness` output now see a banded field; if one asserts a plate count outside its archetype range, report it as a concern with the numbers rather than changing the assertion.

- [ ] **Step 5: Commit**

```bash
git add pkg/tectonics/thickness.go pkg/tectonics/thickness_test.go
git commit -m "feat(tectonics): remap the step-0 crust into ocean and continent bands by area share

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: Water budget and sea level

**Files:**
- Create: `pkg/tectonics/sealevel.go`
- Modify: `pkg/tectonics/step.go` (`State` fields, `NewState`, end of `Step`)
- Modify: `pkg/tectonics/sim.go` (`Frame` fields, `Snapshot`, `Run`)
- Test: `pkg/tectonics/sealevel_test.go`

**Interfaces:**
- Consumes: `Elevation`, `AreaWeights`, `InitialAges` (Task 2); knobs `WaterFactorMin/Max`, `RadiusKm`.
- Produces: `func drawWaterFactor(p Params, master int64) float64`; `func HollowVolume(elev *Grid[float64], datum float64, p Params) float64` (km³); `func SeaLevelFor(elev *Grid[float64], water float64, p Params) float64` (km); `func LandShare(elev *Grid[float64], seaLevel float64) float64`; `State.Water float64`, `State.SeaLevel float64`, `(*State).updateSeaLevel()`; `Frame.Elev *Grid[float64]`, `Frame.SeaLevelKm`, `Frame.LandShare`, `Frame.WaterKm3 float64`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/tectonics/sealevel_test.go`:

```go
package tectonics

import (
	"math"
	"testing"
)

func TestSeaLevelAtDatum(t *testing.T) {
	p := testParams(t, 32)
	th := GenerateThickness(p, 2026)
	elev := Elevation(th, InitialAges(th, p), p)
	hollow := HollowVolume(elev, 0, p)
	if hollow <= 0 {
		t.Fatal("a remapped world must have hollows below the datum")
	}
	L := SeaLevelFor(elev, hollow, p)
	if math.Abs(L) > 0.002 {
		t.Fatalf("sea level at factor 1 is %.4f km, want 0 ± 0.002", L)
	}
	L2 := SeaLevelFor(elev, hollow*1.2, p)
	if L2 <= L {
		t.Fatalf("more water must raise the sea: %.4f vs %.4f", L2, L)
	}
	if got := HollowVolume(elev, L2, p); math.Abs(got-hollow*1.2)/(hollow*1.2) > 0.001 {
		t.Fatalf("filled volume %.6g, want %.6g within 0.1%%", got, hollow*1.2)
	}
	if ls := LandShare(elev, L); ls <= 0 || ls >= 1 {
		t.Fatalf("land share %.3f out of range", ls)
	}
	if LandShare(elev, L2) >= LandShare(elev, L) {
		t.Fatal("a higher sea must leave less land")
	}
}

func TestSeaLevelZeroWaterIsLowestPixel(t *testing.T) {
	p := testParams(t, 8)
	elev := NewGrid[float64](8)
	for i := range elev.Cells {
		elev.Cells[i] = 1 + float64(i%5)
	}
	if L := SeaLevelFor(elev, 0, p); math.Abs(L-1) > 0.0011 {
		t.Fatalf("zero water: sea level %.4f, want the lowest pixel (1 km) within a bin", L)
	}
}

func TestRunCarriesWaterAndSeaLevel(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 4, 2
	var frames []Frame
	if err := Run(p, 11, func(f Frame) error { frames = append(frames, f); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, f := range frames {
		if f.Elev == nil || f.Elev.Len() != f.Th.Len() {
			t.Fatalf("frame %d: no elevation grid", f.Step)
		}
		if f.WaterKm3 <= 0 || math.IsNaN(f.SeaLevelKm) || f.LandShare <= 0 || f.LandShare >= 1 {
			t.Fatalf("frame %d: water %g sea %g land %g", f.Step, f.WaterKm3, f.SeaLevelKm, f.LandShare)
		}
		if math.Abs(f.LandShare-LandShare(f.Elev, f.SeaLevelKm)) > 1e-12 {
			t.Fatalf("frame %d: land share %g disagrees with its own grid", f.Step, f.LandShare)
		}
	}
	if frames[0].WaterKm3 != frames[len(frames)-1].WaterKm3 {
		t.Fatal("the water budget must not change during a run")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/tectonics/ -run 'TestSeaLevel|TestRunCarriesWater'`
Expected: compile errors.

- [ ] **Step 3: Implement sealevel.go**

```go
package tectonics

import "math"

const (
	seaMinKm = -32.0
	seaBins  = 65536 // 1 m bins over [-32 km, +32 km)
	seaBinKm = 0.001
)

// drawWaterFactor is the planet's seeded multiplier on its step-0 hollow volume.
func drawWaterFactor(p Params, master int64) float64 {
	rng := newRNG(master, "water.factor")
	return p.WaterFactorMin + rng.Float64()*(p.WaterFactorMax-p.WaterFactorMin)
}

// HollowVolume is the volume in km³ between the height datum (km) and every
// pixel below it, weighting pixels by their true area on the sphere.
func HollowVolume(elev *Grid[float64], datum float64, p Params) float64 {
	w := AreaWeights(elev.S)
	r2 := p.RadiusKm * p.RadiusKm
	v := 0.0
	for i, h := range elev.Cells {
		if h < datum {
			v += (datum - h) * w[i] * r2
		}
	}
	return v
}

func seaBin(h float64) int {
	b := int(math.Floor((h - seaMinKm) / seaBinKm))
	return max(0, min(seaBins-1, b))
}

// SeaLevelFor returns the height L in km at which the hollows of elev hold
// water km³. Heights are quantised to 1 m bins; the volume is linear in L
// inside a bin, so L is interpolated there. Zero water gives the lowest
// pixel; more water than the range can hold gives the top of the range.
func SeaLevelFor(elev *Grid[float64], water float64, p Params) float64 {
	w := AreaWeights(elev.S)
	r2 := p.RadiusKm * p.RadiusKm
	areaAt := make([]float64, seaBins)
	for i, h := range elev.Cells {
		areaAt[seaBin(h)] += w[i] * r2
	}
	area, volume := 0.0, 0.0
	for b := range seaBins {
		area += areaAt[b]
		if area > 0 && volume+area*seaBinKm >= water {
			return seaMinKm + float64(b)*seaBinKm + (water-volume)/area
		}
		volume += area * seaBinKm
	}
	return seaMinKm + seaBins*seaBinKm
}

// LandShare is the area-weighted share of the sphere at or above seaLevel.
func LandShare(elev *Grid[float64], seaLevel float64) float64 {
	w := AreaWeights(elev.S)
	land := 0.0
	for i, h := range elev.Cells {
		if h >= seaLevel {
			land += w[i]
		}
	}
	return land / (4 * math.Pi)
}
```

- [ ] **Step 4: Thread water and sea level through State, Frame and Run**

In `pkg/tectonics/step.go`, add to `State` after `StepNo int`:

```go
	Water    float64 // the planet's water in km³ (set once; factor 1 from NewState, seeded factor from Run)
	SeaLevel float64 // km, refreshed at the end of every Step
```

Replace `NewState` with:

```go
func NewState(p Params, th *Grid[float64], labels *Grid[int32], plates []Plate, motions []Motion) *State {
	S := th.S
	s := &State{P: p, Th: th, Labels: labels, Age: InitialAges(th, p),
		FeatType: NewGrid[uint8](S), FeatAge: NewGrid[uint8](S), Plates: plates, Motions: motions}
	s.Water = HollowVolume(Elevation(s.Th, s.Age, p), 0, p)
	s.updateSeaLevel()
	return s
}

// updateSeaLevel recomputes SeaLevel from the current crust and Water.
func (s *State) updateSeaLevel() {
	s.SeaLevel = SeaLevelFor(Elevation(s.Th, s.Age, s.P), s.Water, s.P)
}
```

At the end of `Step`, after the `// 8. smooth` block, add:

```go
	// 9. sea level
	s.updateSeaLevel()
```

In `pkg/tectonics/sim.go`, extend `Frame`:

```go
type Frame struct {
	Step       int
	Myr        float64
	Th         *Grid[float64]
	Labels     *Grid[int32]
	Feature    *Grid[uint8]
	Elev       *Grid[float64] // km, derived (nil for version 1 bundles read from disk)
	SeaLevelKm float64
	LandShare  float64
	WaterKm3   float64
	Plates     []PlateRow
}
```

In `Snapshot`, compute elevation and return it:

```go
	elev := Elevation(s.Th, s.Age, s.P)
	return Frame{Step: s.StepNo, Myr: float64(s.StepNo) * myrPerStep,
		Th: s.Th.Clone(), Labels: s.Labels.Clone(), Feature: feat, Plates: rows,
		Elev: elev, SeaLevelKm: s.SeaLevel, LandShare: LandShare(elev, s.SeaLevel), WaterKm3: s.Water}
```

In `Run`, right after `s := NewState(...)`:

```go
	s.Water *= drawWaterFactor(p, master)
	s.updateSeaLevel()
```

- [ ] **Step 5: Run the package tests and lint**

Run: `go test ./pkg/tectonics/ && golangci-lint run ./pkg/tectonics/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/tectonics/sealevel.go pkg/tectonics/sealevel_test.go pkg/tectonics/step.go pkg/tectonics/sim.go
git commit -m "feat(tectonics): water-budget sea level on State and Frame

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 5: Test visualisation helper

**Files:**
- Create: `pkg/tectonics/vis_test.go`
- Modify: `.gitignore`
- Modify: `pkg/tectonics/elevation_test.go`, `pkg/tectonics/thickness_test.go`, `pkg/tectonics/sealevel_test.go` (add renders)

**Interfaces:**
- Produces (test package only): `var vis = flag.Bool("vis", ...)`; `func renderIso(h *Grid[float64], face cubemap.Face, seaLevel, exag float64) *image.RGBA`; `func diffPanel(before, after *Grid[float64], face cubemap.Face, scale float64) *image.RGBA`; `func writeVis(t *testing.T, name string, panels ...*image.RGBA)`.

- [ ] **Step 1: Write the helper**

Create `pkg/tectonics/vis_test.go`:

```go
package tectonics

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// vis enables PNG renders of before/after fields under testdata/vis/.
var vis = flag.Bool("vis", false, "write testdata/vis/<test>.png isometric renders and index.html")

const visDir = "testdata/vis"

// fillQuad fills the convex quad with corners c (screen coords, in order).
func fillQuad(img *image.RGBA, c [4][2]float64, col color.RGBA) {
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, p := range c {
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	b := img.Bounds()
	for y := max(b.Min.Y, int(math.Floor(minY))); y <= min(b.Max.Y-1, int(math.Ceil(maxY))); y++ {
		fy := float64(y) + 0.5
		xl, xr := math.Inf(1), math.Inf(-1)
		for k := range 4 {
			p, q := c[k], c[(k+1)%4]
			if (p[1] <= fy) != (q[1] <= fy) {
				x := p[0] + (fy-p[1])*(q[0]-p[0])/(q[1]-p[1])
				xl, xr = math.Min(xl, x), math.Max(xr, x)
			}
		}
		if xl > xr {
			continue
		}
		for x := max(b.Min.X, int(math.Floor(xl))); x <= min(b.Max.X-1, int(math.Ceil(xr))); x++ {
			dst := img.RGBAAt(x, y)
			a := float64(col.A) / 255
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(float64(col.R)*a + float64(dst.R)*(1-a)),
				G: uint8(float64(col.G)*a + float64(dst.G)*(1-a)),
				B: uint8(float64(col.B)*a + float64(dst.B)*(1-a)), A: 255})
		}
	}
}

func shade(c color.RGBA, k float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), c.A}
}

func lerpRGB(a, b color.RGBA, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	return color.RGBA{uint8(float64(a.R) + t*(float64(b.R)-float64(a.R))),
		uint8(float64(a.G) + t*(float64(b.G)-float64(a.G))),
		uint8(float64(a.B) + t*(float64(b.B)-float64(a.B))), 255}
}

// landColour shades height above sea level green → brown → white.
func landColour(above, span float64) color.RGBA {
	t := above / math.Max(span, 1e-9)
	if t < 0.5 {
		return lerpRGB(color.RGBA{60, 140, 60, 255}, color.RGBA{140, 100, 60, 255}, t*2)
	}
	return lerpRGB(color.RGBA{140, 100, 60, 255}, color.RGBA{245, 245, 245, 255}, (t-0.5)*2)
}

// renderIso draws one cube face of h as a 3/4 isometric height field: each
// pixel a shaded column (height × exag screen pixels), land coloured by
// height above seaLevel, a translucent water plane at seaLevel.
func renderIso(h *Grid[float64], face cubemap.Face, seaLevel, exag float64) *image.RGBA {
	S := h.S
	const cw = 10.0 // screen width of one cell
	ch := cw / 2
	lo, hi := math.Inf(1), math.Inf(-1)
	for py := range S {
		for px := range S {
			v := h.Cells[h.Index(face, px, py)]
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	lo = math.Min(lo, seaLevel)
	hi = math.Max(hi, seaLevel+1e-9)
	z := func(v float64) float64 { return (v - lo) * exag }
	top := 10 + z(hi)
	W, H := int(cw*float64(S)+cw), int(top+ch*float64(S)+ch+10)
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{20, 22, 28, 255}}, image.Point{}, draw.Src)
	sx := func(x, y float64) float64 { return (x-y+float64(S))*cw/2 + cw/2 }
	sy := func(x, y, zz float64) float64 { return top + (x+y)*ch/2 - zz }
	column := func(x, y, zz float64, col color.RGBA) {
		// top, then right (x+1) and front (y+1) side faces down to the base plane
		fillQuad(img, [4][2]float64{{sx(x, y), sy(x, y, zz)}, {sx(x+1, y), sy(x+1, y, zz)},
			{sx(x+1, y+1), sy(x+1, y+1, zz)}, {sx(x, y+1), sy(x, y+1, zz)}}, col)
		fillQuad(img, [4][2]float64{{sx(x+1, y), sy(x+1, y, zz)}, {sx(x+1, y+1), sy(x+1, y+1, zz)},
			{sx(x+1, y+1), sy(x+1, y+1, 0)}, {sx(x+1, y), sy(x+1, y, 0)}}, shade(col, 0.75))
		fillQuad(img, [4][2]float64{{sx(x, y+1), sy(x, y+1, zz)}, {sx(x+1, y+1), sy(x+1, y+1, zz)},
			{sx(x+1, y+1), sy(x+1, y+1, 0)}, {sx(x, y+1), sy(x, y+1, 0)}}, shade(col, 0.55))
	}
	for sum := 0; sum <= 2*(S-1); sum++ { // painter's order: back (small px+py) to front
		for px := range S {
			py := sum - px
			if py < 0 || py >= S {
				continue
			}
			v := h.Cells[h.Index(face, px, py)]
			x, y := float64(px), float64(py)
			if v < seaLevel {
				depth := (seaLevel - v) / math.Max(seaLevel-lo, 1e-9)
				column(x, y, z(v), lerpRGB(color.RGBA{40, 90, 160, 255}, color.RGBA{10, 30, 90, 255}, depth))
				column(x, y, z(seaLevel), color.RGBA{60, 120, 220, 110})
			} else {
				column(x, y, z(v), landColour(v-seaLevel, hi-seaLevel))
			}
		}
	}
	return img
}

// diffPanel colours after−before: loss red, gain blue, scaled so |scale| is full intensity.
func diffPanel(before, after *Grid[float64], face cubemap.Face, scale float64) *image.RGBA {
	S := before.S
	const cell = 10
	img := image.NewRGBA(image.Rect(0, 0, S*cell, S*cell))
	for py := range S {
		for px := range S {
			d := (after.Cells[after.Index(face, px, py)] - before.Cells[before.Index(face, px, py)]) / scale
			d = math.Max(-1, math.Min(1, d))
			c := color.RGBA{40, 40, 44, 255}
			if d < 0 {
				c = lerpRGB(c, color.RGBA{230, 60, 50, 255}, -d)
			} else if d > 0 {
				c = lerpRGB(c, color.RGBA{60, 120, 230, 255}, d)
			}
			draw.Draw(img, image.Rect(px*cell, py*cell, (px+1)*cell, (py+1)*cell), &image.Uniform{c}, image.Point{}, draw.Src)
		}
	}
	return img
}

// writeVis tiles panels left to right into testdata/vis/<name>.png and
// regenerates index.html. A no-op unless -vis is set.
func writeVis(t *testing.T, name string, panels ...*image.RGBA) {
	t.Helper()
	if !*vis || len(panels) == 0 {
		return
	}
	const gap = 12
	W, H := gap, 0
	for _, p := range panels {
		W += p.Bounds().Dx() + gap
		H = max(H, p.Bounds().Dy())
	}
	out := image.NewRGBA(image.Rect(0, 0, W, H+2*gap))
	draw.Draw(out, out.Bounds(), &image.Uniform{color.RGBA{12, 13, 16, 255}}, image.Point{}, draw.Src)
	x := gap
	for _, p := range panels {
		draw.Draw(out, image.Rect(x, gap, x+p.Bounds().Dx(), gap+p.Bounds().Dy()), p, p.Bounds().Min, draw.Src)
		x += p.Bounds().Dx() + gap
	}
	if err := os.MkdirAll(visDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(visDir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, out); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	writeVisIndex(t)
}

func writeVisIndex(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(visDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".png") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=utf-8><title>tectonics test renders</title>" +
		"<style>body{background:#0c0d10;color:#ccc;font:14px system-ui;margin:16px}h2{margin:24px 0 6px}img{max-width:100%}</style>" +
		"<h1>tectonics test renders</h1><p>panels left to right as named in each test (typically before | after | diff).</p>")
	for _, n := range names {
		fmt.Fprintf(&b, "<h2>%s</h2><img src=%q>", strings.TrimSuffix(n, ".png"), n)
	}
	if err := os.WriteFile(filepath.Join(visDir, "index.html"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Ignore the output directory**

Append to `.gitignore` under the tectonics block:

```
# tectonics test renders (go test -vis)
pkg/tectonics/testdata/vis/
```

- [ ] **Step 3: Add renders to the existing tests**

In `pkg/tectonics/elevation_test.go`, `TestElevationOceanSinksWithAge`: after the loop, build a strip and write it:

```go
	// render: a 16×16 face of ridge crust ageing 0, 50, 200, 400 steps
	var panels []*image.RGBA
	for _, steps := range []uint16{0, 50, 200, 400} {
		th, age := NewGrid[float64](16), NewGrid[uint16](16)
		for i := range th.Cells {
			th.Cells[i], age.Cells[i] = thExpected(p, float64(steps)), steps
		}
		panels = append(panels, renderIso(Elevation(th, age, p), cubemap.FacePosZ, 0, 12))
	}
	writeVis(t, "elevation_ocean_sinks_with_age", panels...)
```

(add `"image"` and the cubemap import to that file.)

In `pkg/tectonics/thickness_test.go`, `TestGenerateThicknessStartsWithContinents`: after the share check:

```go
	elev := Elevation(th, InitialAges(th, p), p)
	writeVis(t, "thickness_remap_continents", renderIso(th, cubemap.FacePosZ, p.ContinentalThreshold, 60),
		renderIso(elev, cubemap.FacePosZ, 0, 6))
```

In `pkg/tectonics/sealevel_test.go`, `TestSeaLevelAtDatum`: after `L2` is checked:

```go
	writeVis(t, "sealevel_factor_1_vs_1.2", renderIso(elev, cubemap.FacePosZ, L, 6), renderIso(elev, cubemap.FacePosZ, L2, 6))
```

- [ ] **Step 4: Run with and without the flag, lint**

Run: `go test ./pkg/tectonics/ && go test ./pkg/tectonics/ -vis -run 'TestElevationOceanSinksWithAge|TestGenerateThicknessStartsWithContinents|TestSeaLevelAtDatum' && ls pkg/tectonics/testdata/vis/ && golangci-lint run ./pkg/tectonics/...`
Expected: PASS both ways; three PNGs plus `index.html` listed; `git status --short` shows no `testdata/vis` entries.

- [ ] **Step 5: Commit**

```bash
git add .gitignore pkg/tectonics/vis_test.go pkg/tectonics/elevation_test.go pkg/tectonics/thickness_test.go pkg/tectonics/sealevel_test.go
git commit -m "test(tectonics): -vis isometric before/after renders with a contact sheet

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 6: Erosion in the step loop

**Files:**
- Create: `pkg/tectonics/erosion.go`
- Modify: `pkg/tectonics/step.go` (call `erode` before the relax pass)
- Modify: `pkg/tectonics/thickness_test.go:8-16` (`testParams` sets `ErosionRate = 0`)
- Test: `pkg/tectonics/erosion_test.go`

**Interfaces:**
- Consumes: `Elevation`, `Neighbors4`, `State.SeaLevel` (Task 4), knobs `ErosionRate, ErosionMaxPerStep, SedimentOceanFactor, ElevPeakKm, ContinentalThreshold`.
- Produces: `func (s *State) erode(th *Grid[float64], age *Grid[uint16], seaLevel float64)`.

Ruling carried from the plan: existing step tests assert pre-erosion thickness behaviour, so `testParams` turns erosion off; `DefaultParams` keeps it on for `Run`, the golden and the integration test. Any test that builds `Params` through `DefaultParams` directly and breaks from erosion sets `p.ErosionRate = 0` explicitly with a one-line comment.

- [ ] **Step 1: Write the failing tests**

Create `pkg/tectonics/erosion_test.go`:

```go
package tectonics

import (
	"image"
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// plateauState is a one-plate world of uniform continental thickness.
func plateauState(t *testing.T, S int, th0 float64) *State {
	t.Helper()
	p := testParams(t, S)
	p.RelaxIters = 0
	th, labels := NewGrid[float64](S), NewGrid[int32](S)
	for i := range th.Cells {
		th.Cells[i] = th0
	}
	plates := PlateStats(th, labels, 1)
	return NewState(p, th, labels, plates, []Motion{{Pole: [3]float64{0, 0, 1}}})
}

func sum(g *Grid[float64]) float64 {
	s := 0.0
	for _, v := range g.Cells {
		s += v
	}
	return s
}

func TestErosionPeakSlumps(t *testing.T) {
	S := 16
	s := plateauState(t, S, 0.6)
	s.P.ErosionRate, s.P.ErosionMaxPerStep = 0.05, 0.25
	s.P.Steps, s.P.TimelineMyr = 16, 800 // 50 Myr per step
	peak := s.Th.Index(cubemap.FacePosZ, 8, 8)
	s.Th.Cells[peak] = 1
	nb := Neighbors4(S)[peak]
	total := sum(s.Th)
	before := s.Th.Clone()
	panels := []*image.RGBA{renderIso(Elevation(s.Th, s.Age, s.P), cubemap.FacePosZ, -10, 8)}
	prev := 1.0
	for step := range 6 {
		s.erode(s.Th, s.Age, -10) // everything is land
		if v := s.Th.Cells[peak]; v >= prev {
			t.Fatalf("step %d: peak %.4f did not fall from %.4f", step, v, prev)
		}
		prev = s.Th.Cells[peak]
		for _, j := range nb {
			if s.Th.Cells[j] <= 0.6 {
				t.Fatalf("step %d: neighbour %d did not gain (%.4f)", step, j, s.Th.Cells[j])
			}
		}
		if math.Abs(sum(s.Th)-total) > 1e-12 {
			t.Fatalf("step %d: thickness not conserved: %.12f vs %.12f", step, sum(s.Th), total)
		}
		for i, v := range s.Th.Cells {
			if v < 0 || v > 1 {
				t.Fatalf("step %d: pixel %d out of range %.4f", step, i, v)
			}
		}
		panels = append(panels, renderIso(Elevation(s.Th, s.Age, s.P), cubemap.FacePosZ, -10, 8))
	}
	panels = append(panels, diffPanel(before, s.Th, cubemap.FacePosZ, 0.05))
	writeVis(t, "erosion_peak_slumps", panels...)
}

func TestErosionSubmarineUntouched(t *testing.T) {
	s := plateauState(t, 8, 0.6)
	s.P.ErosionRate = 0.05
	for i := range s.Th.Cells {
		s.Th.Cells[i] = 0.3 + 0.1*float64(i%3) // ocean, bumpy
	}
	before := s.Th.Clone()
	s.erode(s.Th, s.Age, 100) // sea level above everything
	for i := range s.Th.Cells {
		if s.Th.Cells[i] != before.Cells[i] {
			t.Fatalf("submarine pixel %d changed", i)
		}
	}
}

func TestErosionOceanLoss(t *testing.T) {
	S := 8
	s := plateauState(t, S, 0.2) // deep ocean everywhere
	s.P.ErosionRate, s.P.ErosionMaxPerStep, s.P.SedimentOceanFactor = 0.05, 0.25, 0.5
	for i := range s.Age.Cells {
		s.Age.Cells[i] = 0
	}
	coast := s.Th.Index(cubemap.FacePosZ, 4, 4)
	s.Th.Cells[coast] = 0.9 // one land pixel ringed by ocean
	elev := Elevation(s.Th, s.Age, s.P)
	L := (elev.Cells[coast] + elev.Cells[Neighbors4(S)[coast][0]]) / 2 // between land and ocean
	lost := s.Th.Cells[coast]
	before := s.Th.Clone()
	s.erode(s.Th, s.Age, L)
	lost -= s.Th.Cells[coast]
	if lost <= 0 {
		t.Fatal("coast pixel did not erode")
	}
	gained := 0.0
	for _, j := range Neighbors4(S)[coast] {
		gained += s.Th.Cells[j] - before.Cells[j]
	}
	if math.Abs(gained-0.5*lost) > 1e-12 {
		t.Fatalf("ocean kept %.6f of %.6f, want half", gained, lost)
	}
	writeVis(t, "erosion_ocean_loss", renderIso(Elevation(before, s.Age, s.P), cubemap.FacePosZ, L, 8),
		renderIso(Elevation(s.Th, s.Age, s.P), cubemap.FacePosZ, L, 8), diffPanel(before, s.Th, cubemap.FacePosZ, 0.05))
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/tectonics/ -run TestErosion`
Expected: compile error (`erode` undefined).

- [ ] **Step 3: Implement erosion.go**

```go
package tectonics

import "math"

// erode is step 7 of Step: land pixels (elevation at or above seaLevel)
// give thickness to each lower 4-neighbour in proportion to the elevation
// drop, capped at ErosionMaxPerStep of their outflow fractions per step.
// Moves are computed from the step's starting thickness and applied in one
// pass, so the result is order-independent. Material is conserved except
// that a deep-ocean receiver keeps only SedimentOceanFactor of a slice.
func (s *State) erode(th *Grid[float64], age *Grid[uint16], seaLevel float64) {
	p := s.P
	if p.ErosionRate <= 0 {
		return
	}
	elev := Elevation(th, age, p)
	nb := Neighbors4(th.S)
	delta := make([]float64, th.Len())
	k := p.ErosionRate * p.MyrPerStep() / p.ElevPeakKm
	for i, hi := range elev.Cells {
		if hi < seaLevel {
			continue
		}
		var f [4]float64
		total := 0.0
		for n, j := range nb[i] {
			if hj := elev.Cells[j]; hj < hi {
				f[n] = k * (hi - hj)
				total += f[n]
			}
		}
		if total <= 0 {
			continue
		}
		scale := 1.0
		if total > p.ErosionMaxPerStep {
			scale = p.ErosionMaxPerStep / total
		}
		for n, j := range nb[i] {
			if f[n] == 0 {
				continue
			}
			d := f[n] * scale * math.Max(0, th.Cells[i]-th.Cells[j])
			if d == 0 {
				continue
			}
			delta[i] -= d
			if th.Cells[j] < p.ContinentalThreshold && elev.Cells[j] < seaLevel {
				d *= p.SedimentOceanFactor
			}
			delta[j] += d
		}
	}
	for i := range th.Cells {
		th.Cells[i] = math.Max(0, math.Min(1, th.Cells[i]+delta[i]))
	}
}
```

Stability note for the reviewer: a pixel's outflow is `Σ f·scale·max(0, th_i − th_j) ≤ ErosionMaxPerStep × th_i < th_i` since `ErosionMaxPerStep ≤ 0.25`, so no pixel goes negative without the clamp; the clamp only guards float noise.

- [ ] **Step 4: Call it from Step and turn it off in testParams**

In `pkg/tectonics/step.go`, between `// 7. retire` ... and `// 8. smooth`, renumber and insert so the tail reads:

```go
	// 7. retire
	s.Plates = s.retire(PlateStats(s.Th, s.Labels, len(s.Plates)))

	// 8. erosion (land only, from the sea level of the previous step)
	s.erode(s.Th, s.Age, s.SeaLevel)

	// 9. smooth
	if s.P.RelaxIters > 0 {
		Relax(s.Th, s.P.MaxNeighborDelta, s.P.RelaxIters)
	}

	// 10. sea level
	s.updateSeaLevel()
```

In `pkg/tectonics/thickness_test.go`, `testParams` gains, before `return p`:

```go
	p.ErosionRate = 0 // step tests assert pre-erosion behaviour; erosion has its own tests
```

- [ ] **Step 5: Run the package tests (with -vis once) and lint**

Run: `go test ./pkg/tectonics/ && go test ./pkg/tectonics/ -vis -run TestErosion && golangci-lint run ./pkg/tectonics/...`
Expected: PASS; `testdata/vis/erosion_peak_slumps.png` shows the peak column shrinking over six panels and a red centre with blue ring in the diff.

- [ ] **Step 6: Commit**

```bash
git add pkg/tectonics/erosion.go pkg/tectonics/erosion_test.go pkg/tectonics/step.go pkg/tectonics/thickness_test.go
git commit -m "feat(tectonics): slope-driven erosion with sediment transfer in the step loop

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Bundle version 2, integration test, golden v10

**Files:**
- Modify: `pkg/tectonics/bundle.go`
- Modify: `cmd/tectonics-lab/run.go` (manifest water)
- Modify: `cmd/tectonics-lab/golden_test.go`, `cmd/tectonics-lab/testdata/golden_face64.json`
- Test: `pkg/tectonics/bundle_test.go` (append), `pkg/tectonics/sim_test.go` (append)

**Interfaces:**
- Consumes: `Frame.Elev/SeaLevelKm/LandShare/WaterKm3` (Task 4).
- Produces: `FrameInfo.SeaLevelKm` (`json:"sea_level_km"`), `FrameInfo.LandShare` (`json:"land_share"`), `Manifest.WaterKm3` (`json:"water_km3"`), `manifestVersion = 2`, `elevation.png` per frame; `ReadManifest` accepts versions 1..2; `ReadFrame` sets `Elev` when the file exists (nil otherwise).

- [ ] **Step 1: Write the failing tests**

Append to `pkg/tectonics/bundle_test.go` (create with `package tectonics` and imports `os`, `path/filepath`, `math`, `testing` if the file does not exist):

```go
func TestBundleRoundTripsElevation(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 2, 2
	dir := t.TempDir()
	m := NewManifest(p, 5, "")
	var frames []Frame
	err := Run(p, 5, func(f Frame) error {
		fi, err := WriteFrame(dir, f)
		if err != nil {
			return err
		}
		m.Frames = append(m.Frames, fi)
		m.WaterKm3 = f.WaterKm3
		frames = append(frames, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	got, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || got.WaterKm3 != frames[0].WaterKm3 {
		t.Fatalf("manifest version %d water %g", got.Version, got.WaterKm3)
	}
	for k, fi := range got.Frames {
		if fi.SeaLevelKm != frames[k].SeaLevelKm || fi.LandShare != frames[k].LandShare {
			t.Fatalf("frame %d: sea/land rows %g/%g vs %g/%g", k, fi.SeaLevelKm, fi.LandShare, frames[k].SeaLevelKm, frames[k].LandShare)
		}
		if _, err := os.Stat(filepath.Join(dir, fi.Dir, "elevation.png")); err != nil {
			t.Fatal(err)
		}
		fr, err := ReadFrame(dir, fi)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range frames[k].Elev.Cells {
			if math.Abs(fr.Elev.Cells[i]-v) > 0.0006 {
				t.Fatalf("frame %d pixel %d: elevation %.4f read back as %.4f", k, i, v, fr.Elev.Cells[i])
			}
		}
	}
}

func TestReadManifestAcceptsVersion1(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":1,"archetype":"terran","face":8,"steps":1,"myr_per_step":1,"frames":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(dir); err != nil {
		t.Fatalf("version 1 must still load: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(dir); err == nil {
		t.Fatal("version 3 must be refused")
	}
}
```

Append to `pkg/tectonics/sim_test.go` (create with `package tectonics`, imports `math`, `testing`, `reflect` if absent):

```go
func TestLandmassIntegration(t *testing.T) {
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face, p.Steps, p.TimelineMyr, p.KeyframeEvery = 64, 20, 40, 5 // 20 steps × 2 Myr (Task 7b correction)
	var frames []Frame
	if err := Run(p, 2026, func(f Frame) error { frames = append(frames, f); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, f := range frames {
		if f.LandShare < 0.1 || f.LandShare > 0.5 {
			t.Fatalf("frame %d: land share %.3f outside [0.1, 0.5]", f.Step, f.LandShare)
		}
		if math.IsNaN(f.SeaLevelKm) || f.SeaLevelKm < -5 || f.SeaLevelKm > 5 {
			t.Fatalf("frame %d: sea level %.3f km", f.Step, f.SeaLevelKm)
		}
	}
}

func TestLandmassDeterminism(t *testing.T) {
	p := testParams(t, 32)
	p.Steps, p.KeyframeEvery, p.ErosionRate = 10, 5, 0.01
	run := func() []Frame {
		var out []Frame
		if err := Run(p, 99, func(f Frame) error { out = append(out, f); return nil }); err != nil {
			t.Fatal(err)
		}
		return out
	}
	a, b := run(), run()
	for k := range a {
		if a[k].SeaLevelKm != b[k].SeaLevelKm || a[k].LandShare != b[k].LandShare || !reflect.DeepEqual(a[k].Elev.Cells, b[k].Elev.Cells) {
			t.Fatalf("frame %d differs between runs", k)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/tectonics/ -run 'TestBundleRoundTripsElevation|TestReadManifestAcceptsVersion1|TestLandmass'`
Expected: compile errors (`WaterKm3`, `SeaLevelKm` on `FrameInfo`/`Manifest` undefined).

- [ ] **Step 3: Implement bundle version 2**

In `pkg/tectonics/bundle.go`:

```go
const manifestVersion = 2 // 1 = checkpoint 1 bundles (no elevation); 2 adds elevation.png, sea level, land share, water
```

`FrameInfo` gains, after `Dir`:

```go
	SeaLevelKm float64    `json:"sea_level_km"`
	LandShare  float64    `json:"land_share"`
```

`Manifest` gains, after `MyrPerStep`:

```go
	WaterKm3   float64     `json:"water_km3"`
```

In `WriteFrame`, after the feature image is written and before the return:

```go
	if fr.Elev != nil {
		elevPx := func(v float64) color.RGBA {
			m := math.Max(0, math.Min(65535, math.Round(v*1000)+32768))
			u := uint16(m)
			return color.RGBA{R: uint8(u >> 8), G: uint8(u), B: 0, A: 255}
		}
		if err := writeCross(filepath.Join(dir, "elevation.png"), gridToCross(fr.Elev, elevPx)); err != nil {
			return FrameInfo{}, err
		}
	}
	return FrameInfo{Step: fr.Step, Myr: fr.Myr, Dir: rel, Plates: fr.Plates,
		SeaLevelKm: fr.SeaLevelKm, LandShare: fr.LandShare}, nil
```

Update the `WriteFrame` doc comment to list `elevation.png`.

In `ReadManifest`, replace the version check:

```go
	if m.Version < 1 || m.Version > manifestVersion {
		return nil, fmt.Errorf("manifest %s: version %d, want 1..%d", bundleDir, m.Version, manifestVersion)
	}
```

In `ReadFrame`, before the return, read the elevation if present:

```go
	fr := Frame{Step: fi.Step, Myr: fi.Myr, Plates: fi.Plates, SeaLevelKm: fi.SeaLevelKm, LandShare: fi.LandShare,
		Th:      crossToGrid(th, func(c color.RGBA) float64 { return float64(c.R) / 255 }),
		Labels:  crossToGrid(pl, func(c color.RGBA) int32 { return int32(c.R) }),
		Feature: crossToGrid(fe, func(c color.RGBA) uint8 { return c.R }),
	}
	if el, err := readCross(filepath.Join(dir, "elevation.png")); err == nil {
		fr.Elev = crossToGrid(el, func(c color.RGBA) float64 {
			return (float64(uint16(c.R)<<8|uint16(c.G)) - 32768) / 1000
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return Frame{}, err
	}
	return fr, nil
```

(add `errors` to the imports.)

In `cmd/tectonics-lab/run.go`, inside the `Run` callback after `m.Frames = append(m.Frames, fi)`:

```go
		m.WaterKm3 = f.WaterKm3
```

- [ ] **Step 4: Extend the golden hash and rebake v10**

In `cmd/tectonics-lab/golden_test.go`, `frameHash`, after `_, _ = h.Write(f.Feature.Cells)`:

```go
	if f.Elev != nil {
		for _, v := range f.Elev.Cells {
			putF64(h, v)
		}
	}
	putF64(h, f.SeaLevelKm)
	putF64(h, f.LandShare)
```

Change the recipe string to `"terran/2026/64/20/v10"`.

Run: `go test ./cmd/tectonics-lab/ -run TestGoldenFace64 -update && go test ./cmd/tectonics-lab/`
Expected: "golden rewritten"; then PASS including `TestTimingBudgetFace128` (report its measured time).

- [ ] **Step 5: Full build, tests and lint**

Run: `go build ./... && go test ./pkg/tectonics/... ./cmd/tectonics-lab/... && golangci-lint run ./pkg/tectonics/... ./cmd/tectonics-lab/...`
Expected: PASS, no findings.

- [ ] **Step 6: Commit**

```bash
git add pkg/tectonics/bundle.go pkg/tectonics/bundle_test.go pkg/tectonics/sim_test.go cmd/tectonics-lab/run.go cmd/tectonics-lab/golden_test.go cmd/tectonics-lab/testdata/golden_face64.json
git commit -m "feat(tectonics): bundle v2 with elevation.png, sea level and land share; golden v10

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7b: Conservative advection, relax within regime, thickening cap, erosion rate (amendment)

Added during execution after Task 7's integration test exposed a pre-existing crust leak (see spec section 3b and the SDD ledger). The brief lives at `.superpowers/sdd/2026-10-10-tectonics-landmass/task-7b-brief.md` and is reproduced here in summary: classify gap regions in `Step` (ridge crust only where two plates open across the seam with `dot(rel, normal) > 0.3 |rel|`, otherwise heal from the BFS source pixel); `RelaxWithin(th, maxDelta, iters, threshold)` in `thickness.go`, used by `Step`; cap oceanic thickening at `OceanBandMax`; `ErosionRate` default 0.0015; tests `TestShearAlongJaggedBoundaryOpensNoCrust`, `TestDivergenceStillOpensRidgeCrust`, `TestOceanThickeningCappedAtBand`, `TestRelaxWithinKeepsCoastStep`, `TestContinentsSurviveAdvection` in `advection_test.go`; `TestLandmassIntegration` re-added with `p.TimelineMyr = 40` (the Task 7 version used 40 Myr steps by mistake); golden recipe v11.

---

### Task 8: Viewer hypsometric layer

**Files:**
- Modify: `cmd/tectonics-lab/web/app.js`
- Modify: `cmd/tectonics-lab/web/index.html` (status width only if needed)
- Test: `cmd/tectonics-lab/web_test.go`

**Interfaces:**
- Consumes: bundle v2 (`elevation.png`, `frames[i].sea_level_km`, `frames[i].land_share`).

- [ ] **Step 1: Write the failing test**

In `cmd/tectonics-lab/web_test.go`, add:

```go
func TestViewerReadsElevation(t *testing.T) {
	b, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"elevation", "uSea", "uHasEl", "sea_level_km", "land_share"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
}
```

(add `os` and `strings` imports if absent.)

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/tectonics-lab/ -run TestViewerReadsElevation`
Expected: FAIL listing the missing strings.

- [ ] **Step 3: Edit the shader and loader**

In `app.js` fragment shader, change the uniform line and add two uniforms:

```glsl
    uniform sampler2D uTh, uPl, uFe, uEl;
    uniform int uMode;      // 0 sphere, 1 flat cross
    uniform int uLayer;     // 0 plates, 1 thickness, 2 hypsometric, 3 features
    uniform int uHasEl;     // 1 when the bundle has elevation.png (version 2)
    uniform float uSea;     // sea level, km
    uniform vec2 uTexel;    // one texel of the cross texture
```

Add before `vec3 colour(vec2 uv)`:

```glsl
    // elevation in km from the packed 16-bit red/green bytes; a < 0.5 marks the cross gaps
    float elevAt(vec2 uv, out bool ok){
      vec4 e = texture2D(uEl, uv);
      ok = e.a > 0.5;
      return (floor(e.r*255.0+0.5)*256.0 + floor(e.g*255.0+0.5) - 32768.0)/1000.0;
    }
    vec3 hypso(vec2 uv, float th){
      bool ok; float h = elevAt(uv, ok);
      if (uHasEl == 0 || !ok) {
        return th < 0.5 ? mix(vec3(0.05,0.15,0.45), vec3(0.2,0.55,0.8), th*2.0)
                        : mix(vec3(0.25,0.5,0.2), vec3(0.95,0.95,0.95), (th-0.5)*2.0);
      }
      if (h < uSea) {
        float d = clamp((uSea - h)/6.0, 0.0, 1.0);
        return mix(vec3(0.22,0.45,0.72), vec3(0.03,0.10,0.32), d);
      }
      float a = clamp((h - uSea)/4.0, 0.0, 1.0);
      vec3 land = a < 0.5 ? mix(vec3(0.24,0.55,0.24), vec3(0.55,0.40,0.24), a*2.0)
                          : mix(vec3(0.55,0.40,0.24), vec3(0.95,0.95,0.95), (a-0.5)*2.0);
      // one-pixel coast: any valid 4-neighbour texel below sea level
      vec2 o[4]; o[0] = vec2(uTexel.x,0.0); o[1] = vec2(-uTexel.x,0.0); o[2] = vec2(0.0,uTexel.y); o[3] = vec2(0.0,-uTexel.y);
      for (int k = 0; k < 4; k++) { bool nok; float hn = elevAt(uv+o[k], nok); if (nok && hn < uSea) { return vec3(0.9,0.9,0.75); } }
      return land;
    }
```

Replace the `uLayer == 2` branch of `colour` with `c = hypso(uv, th);`.

In `makeGL`: uniform list becomes `['uTh','uPl','uFe','uEl','uMode','uLayer','uHasEl','uSea','uTexel','uView','uSun']`; after the existing `gl.uniform1i` line add `gl.uniform1i(u.uEl, 3);`; textures become `[0,1,2,3].map(...)`.

In `frameImages`, load the elevation image optionally:

```js
    const p = Promise.all([
      ...['thickness', 'plate', 'feature'].map((n) => loadImage(`${dir}/${n}.png`)),
      loadImage(`${dir}/elevation.png`).catch(() => null),
    ]);
```

In `upload`, skip null images: `images.forEach((img, i) => { if (!img) { return; } ... })`.

In `render`, after `gl.uniform1i(ctx.u.uLayer, layer());` add:

```js
      const f = manifest.frames[i];
      gl.uniform1i(ctx.u.uHasEl, images[3] ? 1 : 0);
      gl.uniform1f(ctx.u.uSea, f.sea_level_km || 0);
      gl.uniform2f(ctx.u.uTexel, 1 / (4 * manifest.face), 1 / (3 * manifest.face));
```

and change the Myr status line to:

```js
    const fr = manifest.frames[i];
    const sea = fr.sea_level_km === undefined ? '' : ` · sea ${fr.sea_level_km.toFixed(2)} km · land ${(100 * fr.land_share).toFixed(0)} %`;
    $('myr').textContent = `${fr.myr.toFixed(0)} Myr  (step ${fr.step}/${manifest.steps})${sea}`;
```

In `index.html`, widen `#myr` to `min-width:300px` so the longer status does not wrap.

- [ ] **Step 4: Check the shader compiles**

The implementer does not open a browser. Run the test and a syntax check:

```bash
go test ./cmd/tectonics-lab/ -run 'TestViewer' && node --check cmd/tectonics-lab/web/app.js
```

Expected: PASS and no syntax error (if `node` is absent, say so in the report; the controller verifies the page in the browser after review).

- [ ] **Step 5: Commit**

```bash
git add cmd/tectonics-lab/web/app.js cmd/tectonics-lab/web/index.html cmd/tectonics-lab/web_test.go
git commit -m "feat(tectonics-lab): hypsometric layer from elevation and sea level, coast line, land share status

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: Python — Earth exporter v2, stats and sweep metrics

**Files:**
- Modify: `scripts/earth_plates_export.py`
- Modify: `scripts/tectonics_stats.py`
- Modify: `scripts/tectonics_seed_sweep.py`

**Interfaces:**
- Consumes: bundle v2 layout (Task 7).
- Produces: stats fields `land_mean, land_min, land_max, land_elev_mean, elev_max, ocean_depth_mean` in the table and CSV; `area_weights(S)` in `tectonics_stats.py` reused by the exporter.

- [ ] **Step 1: Stats — area weights, elevation reader, new metrics**

In `scripts/tectonics_stats.py`, after `pixel_dirs`:

```python
def area_weights(S):
    """Solid angle of every pixel (steradians), 6*S*S, summing to 4π."""
    edges = 2 * np.arange(S + 1) / S - 1
    u0, u1 = np.meshgrid(edges[:-1], edges[:-1], indexing="xy")[0], np.meshgrid(edges[1:], edges[1:], indexing="xy")[0]
    v0, v1 = np.meshgrid(edges[:-1], edges[:-1], indexing="xy")[1], np.meshgrid(edges[1:], edges[1:], indexing="xy")[1]
    corner = lambda u, v: np.arctan(u * v / np.sqrt(1 + u * u + v * v))  # noqa: E731
    face = corner(u1, v1) - corner(u0, v1) - corner(u1, v0) + corner(u0, v0)
    return np.tile(face.ravel(), 6)


def read_elevation(path, S):
    """elevation.png → km: 16-bit height packed red (high) / green (low), offset 32768."""
    img = np.array(Image.open(path)).astype(np.int64)
    v = img[..., 0] * 256 + img[..., 1]
    out = np.empty(6 * S * S, dtype=np.float64)
    for face in range(6):
        col, row = CROSS_CELLS[face]
        out[face * S * S:(face + 1) * S * S] = v[row * S:(row + 1) * S, col * S:(col + 1) * S].ravel()
    return (out - 32768) / 1000.0
```

In `Bundle.__init__` add `self.w = area_weights(self.S)`. In `frame_stats`, before the `return`, add:

```python
        land = {"land": None, "land_elev": None, "elev_max": None, "ocean_depth": None}
        epath = os.path.join(d, "elevation.png")
        if os.path.exists(epath) and "sea_level_km" in f:
            h = read_elevation(epath, self.S)
            L = f["sea_level_km"]
            wet, dry = h < L, h >= L
            land["land"] = float(self.w[dry].sum() / (4 * math.pi))
            land["land_elev"] = float(((h[dry] - L) * self.w[dry]).sum() / self.w[dry].sum()) if dry.any() else 0.0
            land["elev_max"] = float((h - L).max())
            land["ocean_depth"] = float(((h[wet] - L) * self.w[wet]).sum() / self.w[wet].sum()) if wet.any() else 0.0
```

and include `**land` in the returned dict. In `summary`, after `"reaims_per_100myr"`, add:

```python
            "land_mean": statistics.mean(p["land"] for p in per) if per[0]["land"] is not None else None,
            "land_min": min(p["land"] for p in per) if per[0]["land"] is not None else None,
            "land_max": max(p["land"] for p in per) if per[0]["land"] is not None else None,
            "land_elev_mean": statistics.mean(p["land_elev"] for p in per) if per[0]["land"] is not None else None,
            "elev_max": max(p["elev_max"] for p in per) if per[0]["land"] is not None else None,
            "ocean_depth_mean": statistics.mean(p["ocean_depth"] for p in per) if per[0]["land"] is not None else None,
```

Extend `CSV_FIELDS` with `"land_mean", "land_min", "land_max", "land_elev_mean", "elev_max", "ocean_depth_mean"`. In `csv_row`, emit `""` for `None`. Add table lines after the re-aims line:

```python
        ("land share (mean, min–max)", lambda a: "—" if a["land_mean"] is None else f"{100*a['land_mean']:.0f}% ({100*a['land_min']:.0f}–{100*a['land_max']:.0f}%)"),
        ("mean land elevation km", lambda a: "—" if a["land_elev_mean"] is None else f"{a['land_elev_mean']:.2f}"),
        ("max elevation km", lambda a: "—" if a["elev_max"] is None else f"{a['elev_max']:.1f}"),
        ("mean ocean depth km", lambda a: "—" if a["ocean_depth_mean"] is None else f"{a['ocean_depth_mean']:.2f}"),
```

Update the module docstring's metric list accordingly.

- [ ] **Step 2: Sweep — new metrics, tolerate blanks**

In `scripts/tectonics_seed_sweep.py`, append to `METRICS`:

```python
    ("land_mean", "land share (mean)", "{:.0%}", 0.15, 0.5),
    ("land_elev_mean", "mean land elevation km", "{:.2f}", None, None),
    ("elev_max", "max elevation km", "{:.1f}", None, None),
    ("ocean_depth_mean", "mean ocean depth km", "{:.2f}", None, None),
```

In `summarize`, where metric values are collected, skip blanks: `vals = [float(r[key]) for r in ok if r.get(key) not in ("", None)]`, and in the outlier loop guard `if r.get(key) in ("", None): continue`. In `bake_and_score`, where CSV fields are copied, write `""` when the value is `None`. Also the `earth_row` cache: a cached `earth.json` without the new keys should be treated as missing for those keys (`earth.get(key)`; print `—` when absent).

- [ ] **Step 3: Earth exporter v2**

In `scripts/earth_plates_export.py`: `from tectonics_stats import area_weights` (same directory; add `sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))` before it). Add constants `LAND_ELEV_KM, OCEAN_ELEV_KM = 0.8, -3.7`. Add:

```python
def write_cross16(values_km, S):
    """elevation cross: 16-bit metres + 32768 packed red (high) / green (low)."""
    v = np.clip(np.round(values_km * 1000) + 32768, 0, 65535).astype(np.uint16)
    img = np.zeros((3 * S, 4 * S, 4), dtype=np.uint8)
    for face in range(6):
        col, row = CROSS_CELLS[face]
        cell = v[face * S * S:(face + 1) * S * S].reshape(S, S)
        img[row * S:(row + 1) * S, col * S:(col + 1) * S, 0] = cell >> 8
        img[row * S:(row + 1) * S, col * S:(col + 1) * S, 1] = cell & 255
        img[row * S:(row + 1) * S, col * S:(col + 1) * S, 3] = 255
    return img
```

Where the frame images are saved, add:

```python
        elev = np.where(cont_labels >= 0, LAND_ELEV_KM, OCEAN_ELEV_KM)
        save_png(os.path.join(fdir, "elevation.png"), write_cross16(elev, S))
        land_share = float(W[cont_labels >= 0].sum() / (4 * math.pi))
        frames.append({"step": step, "myr": args.start - t, "ma": t, "dir": rel, "plates": rows,
                       "sea_level_km": 0.0, "land_share": land_share})
```

with `W = area_weights(S)` computed once before the frame loop (and `import math` if absent). In the manifest set `"version": 2` and add `"water_km3": 0.0`. Update the module docstring's layout description (thickness line and the new elevation line).

- [ ] **Step 4: Verify on real bundles**

```bash
cd /home/robert/spacemolt/kb-tectonics && go build -o bin/tectonics-lab ./cmd/tectonics-lab \
 && bin/tectonics-lab run -seed 42 -archetype terran -face 64 -steps 20 -out /tmp/claude-1000/-home-robert-spacemolt-kb/2e334816-08d5-4e5f-88ff-dc8802aadfb1/scratchpad/lm \
 && ~/gplates-venv/bin/python3 scripts/tectonics_stats.py /tmp/claude-1000/-home-robert-spacemolt-kb/2e334816-08d5-4e5f-88ff-dc8802aadfb1/scratchpad/lm/seed-42-terran data/tectonics/earth-test \
 && ~/gplates-venv/bin/python3 scripts/tectonics_stats.py --csv /tmp/claude-1000/-home-robert-spacemolt-kb/2e334816-08d5-4e5f-88ff-dc8802aadfb1/scratchpad/lm/seed-42-terran \
 && ~/gplates-venv/bin/python3 scripts/earth_plates_export.py --help | head -3
```

Expected: the seed bundle prints land share, land elevation, max elevation and ocean depth lines; the version 1 `earth-test` bundle prints `—` for them; the CSV has the new columns; the exporter still parses. Then regenerate the small Earth test bundle as version 2 with the same arguments that made `data/tectonics/earth-test` (3 frames, face 64; see the exporter's `--help`), writing to a new directory `data/tectonics/earth-test-v2`, and rerun the stats on it: its land share should be about 0.30 and its elevation lines 0.80 / 0.8 / −3.70. Paste the two stats tables into the report.

Finally the sweep smoke: `~/gplates-venv/bin/python3 scripts/tectonics_seed_sweep.py --n 2 --face 64 --steps 20 --jobs 2 --out /tmp/claude-1000/-home-robert-spacemolt-kb/2e334816-08d5-4e5f-88ff-dc8802aadfb1/scratchpad/lm-sweep --earth ''` must print the four new metric rows.

- [ ] **Step 5: Commit**

```bash
git add scripts/earth_plates_export.py scripts/tectonics_stats.py scripts/tectonics_seed_sweep.py
git commit -m "feat(tectonics): land share and elevation metrics in stats and sweep; Earth exporter writes bundle v2

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 10: Face-256 bake and tuning sweep (controller-run)

This task produces numbers, not code, and is run by the controller after Task 9's review, not by an implementer. Its outcome decides the final knob values committed before merge.

**Files:**
- Possibly modify: `pkg/tectonics/params.go` (defaults), `cmd/tectonics-lab/testdata/golden_face64.json` (rebake if defaults change)

- [ ] **Step 1: Single seed for the viewer**

```bash
cd /home/robert/spacemolt/kb-tectonics && go build -o bin/tectonics-lab ./cmd/tectonics-lab \
 && bin/tectonics-lab run -seed 42 -archetype terran -out data/tectonics \
 && ~/gplates-venv/bin/python3 scripts/tectonics_stats.py data/tectonics/earth-nnr data/tectonics/seed-42-terran
```

Open the viewer (the serve process on :8092 picks the bundle up) and look at the hypsometric layer over the slider.

- [ ] **Step 2: Sweep**

```bash
systemd-run --user --unit tectonics-sweep-lm -p MemoryMax=8G -p Nice=10 -p IOSchedulingClass=best-effort -p IOSchedulingPriority=7 \
  -p WorkingDirectory=/home/robert/spacemolt/kb-tectonics \
  -p StandardOutput=append:/home/robert/spacemolt/kb-tectonics/data/tectonics/sweep-lm/sweep.log -p StandardError=inherit \
  /home/robert/gplates-venv/bin/python3 scripts/tectonics_seed_sweep.py --n 30 --jobs 3 --out data/tectonics/sweep-lm
```

(create `data/tectonics/sweep-lm` first.) Targets from the spec: `land_mean` 0.25–0.35 and stable over a run, `land_elev_mean` 0.5–1.2 km, `ocean_depth_mean` near −3.7 km, `elev_max` 5–9 km. If a target is missed, adjust `ErosionRate`, `LandRankPow`, `ElevPow` or `SubsidenceKm` through `--set` on a second sweep, then commit the chosen defaults with a rebaked golden (`go test ./cmd/tectonics-lab/ -run TestGoldenFace64 -update`).

- [ ] **Step 3: Erosion check**

From a mid-run frame of seed 42, rerun with uplift off to watch a belt wear down:

```bash
bin/tectonics-lab run -seed 42 -archetype terran -set CollisionUplift=0 -set ArcUplift=0 -out /tmp/claude-1000/-home-robert-spacemolt-kb/2e334816-08d5-4e5f-88ff-dc8802aadfb1/scratchpad/lm-erode \
 && ~/gplates-venv/bin/python3 scripts/tectonics_stats.py --series 100 /tmp/claude-1000/-home-robert-spacemolt-kb/2e334816-08d5-4e5f-88ff-dc8802aadfb1/scratchpad/lm-erode/seed-42-terran
```

The `elev_max` line over the run should fall from its early value toward a third of it by about 250 Myr later. Record the numbers in the final report.

- [ ] **Step 4: Commit any default change**

```bash
git add pkg/tectonics/params.go cmd/tectonics-lab/testdata/golden_face64.json
git commit -m "feat(tectonics): land-mass defaults from the face-256 sweep

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Self-review

- **Spec coverage.** Knobs and validation: Task 1. Elevation branches, blend, remap, initial ages: Tasks 2–3. Water budget, sea level, water mask, State/Frame fields: Task 4. Erosion rule, cap, ocean loss, Jacobi, placement before relax: Task 6. Bundle v2, viewer, lab (`-set` unchanged), Earth exporter, stats, sweep, golden v10: Tasks 7–9. Test visuals and gitignore: Task 5. Every named test in spec section 6 appears in Tasks 2, 3, 4, 6, 7; the timing budget test runs unchanged in Task 7. Tuning: Task 10.
- **Placeholders.** None; every code step carries its code. Task 10 is a controller-run procedure by design.
- **Type consistency.** `Elevation(th, age, p)`, `InitialAges(th, p)`, `AreaWeights(S)`, `HollowVolume(elev, datum, p)`, `SeaLevelFor(elev, water, p)`, `LandShare(elev, L)`, `(*State).erode(th, age, seaLevel)`, `(*State).updateSeaLevel()`, `Frame.{Elev, SeaLevelKm, LandShare, WaterKm3}`, `FrameInfo.{SeaLevelKm, LandShare}`, `Manifest.WaterKm3`, `renderIso(h, face, seaLevel, exag)`, `diffPanel(before, after, face, scale)`, `writeVis(t, name, panels...)` are used with the same names and argument orders in every task.
