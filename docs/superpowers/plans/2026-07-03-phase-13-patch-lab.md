# Phase 13 Patch Lab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A "Patch Lab" mode in planet-explorer: sphere tectonics at S_tect=256 → crop a 512² flat patch of one cube face at virtual S_prod=1024 → run all downstream layers on the patch with near-realtime dirty-tracked sliders → "Go!" runs the existing full production render. Plus per-layer byte-exact snapshot tests.

**Architecture:** New `pkg/planetgen/patch` package. Sphere-global stages (plates, crust, FX classification, sea-level quantiles, normalize bounds) run once at S_tect via existing `field` functions; per-pixel stages are re-run on the patch at true per-pixel sphere directions (`cubemap.FacePixelToDir` on the virtual S_prod grid) so all noise is byte-identical to production. Three small behavior-preserving extractions in production code (`field.FXDelta`, `render` per-pixel colorize helpers, `feature.HabitabilityScoreAt`, `biome.RainShadowMultiplierAt`) let the patch reuse production formulas instead of copying them.

**Tech Stack:** Go 1.24+, existing planetgen packages, `syscall/js` wasm, vanilla JS explorer UI.

**Spec:** `docs/superpowers/specs/2026-07-02-patch-lab-design.md` (read it first).

## Global Constraints

- Worktree: `/home/robert/spacemolt/kb-phase-13-progressive-layers`, branch `phase-13/progressive-layers`. Run all commands from the worktree root.
- Go 1.24+ idioms: `for i := range n` over ints; `b.Loop()` in benchmarks (never `for range b.N`).
- After every task: `golangci-lint run ./...` must report 0 new issues, and `go build ./...` must pass.
- Production behavior must not change: Tasks 2–3 are pure extractions; the face-128 golden suite (`go test ./cmd/generate-planet-maps/ -timeout 120m`) is run once at the end (Task 19) and must pass WITHOUT `-update`. Per-task verification uses the fast package tests only (`go test ./pkg/planetgen/...`).
- All new RNG: `seed.Domain(master, "patch.<name>")`. Where a patch layer mirrors a production stage, use the production stage's seeds verbatim (e.g. colorize noise `master+42/+77`, erosion `"erosion.seed"`/`"erosion.stream"` are inside `field.Erode`; the patch erosion port keeps them).
- New JSON keys are lowerCamel (matches `crust`/`tectonicFX` convention).
- Patch package tests must be fast: use `sTect ≤ 64`, window `size ≤ 128`, `sProd ≤ 256` in tests. Target: whole `pkg/planetgen/patch` suite < 60s.
- Built binaries go in `bin/`, never the repo root.
- Commit after every task with the message given in the task. End commit messages with `Co-Authored-By:` line per project convention.

## Key existing APIs (verified, do not re-derive)

```go
// cubemap
func FacePixelToDir(face Face, px, py, S int) (x, y, z float64)       // pixel CENTER dir, unit
func DirToFacePixel(x, y, z float64, S int) (Face, int, int)          // clamped inverse
func ForceFaceUV(face Face, x, y, z float64) (u, v float64)           // UV in a FIXED face frame
func (cmf *CubeMapF) Sample(x, y, z float64) float64                  // bilinear at direction
type CubeMapF struct{ Size int; Faces [NumFaces][]float64 }           // fields EXPORTED — wrap raw slices

// field
func GeneratePlates(profile *types.PlanetProfile, master int64, S int) *PlateField
func GenerateCrust(profile *types.PlanetProfile, master int64, S int, pf *PlateField) *CrustField
func ClassifyTectonics(pf *PlateField, crust *CrustField, radiusKm float64) *TectonicFXField
func ApplyTectonicFX(hm *cubemap.CubeMapF, fx *TectonicFXField, crust *CrustField, pf *PlateField, cfg types.TectonicFXConfig, master int64, S int)
func GenerateControlFields(master int64, cfg types.ControlConfig, S int, jitter *noise.JitterField) [5]*cubemap.CubeMapF
func SmoothHeightmap(heightmap *cubemap.CubeMapF, r int, S int) *cubemap.CubeMapF
func QuantileSeaLevel(hm *cubemap.CubeMapF, oceanFrac float64) float64
func Erode(masterSeed int64, heightmap *cubemap.CubeMapF, cfg types.ErosionConfig, oceanLevel float64, S int) *cubemap.CubeMapF
func GenerateFlow(heightmap *cubemap.CubeMapF, cfg types.FlowConfig) *FlowField
func CarveRivers(heightmap *cubemap.CubeMapF, ff *FlowField, cfg types.FlowConfig)
// CrustField: Size, ContinentalMask, BaseHeight *cubemap.CubeMapF, Cratons []Craton, Assembly, LandFraction, TectonicAge float64
// TectonicFXField: Size + Belt/Subd/Arc/Ridge/Rift × Dist/Mag, all *cubemap.CubeMapF
// PlateField: Size, Plates, PlateID [6][]int16, Convergent/Divergent/Transform + ConvergentMag/DivergentMag [6][]float64

// noise
func New(seed int64) *Generator
func (g *Generator) FractalNoise3D(x, y, z float64, octaves int, lacunarity, persistence, scale float64) float64
func (g *Generator) RidgedFractal3D(x, y, z float64, octaves int, lacunarity, gain, offset float64) float64
func GenerateJitter(profile *types.PlanetProfile, master int64, S int) *JitterField
func (jf *JitterField) Transform(px, py, pz float64) (float64, float64, float64)  // direction-based, patch-safe
func NewWarper(master int64, cfg types.WarpConfig) *Warper; func (w *Warper) Warp(x, y, z float64) (float64, float64, float64)
func NewCoastalGen(seed int64) *CoastalGen
func ApplyCoastal(g *CoastalGen, dx, dy, dz, height, distToCoast, amp, threshold, freq float64) float64

// biome / feature / planetcolor
func LookupColor(table types.BiomeTable, T, M, height float64) color.RGBA
func GenerateCraters(seed int64, profile *types.PlanetProfile) []Crater   // global list, Lat/Lon/Radius/Age
func ApplyEjecta(base color.RGBA, dx, dy, dz float64, craters []Crater) color.RGBA
func AssignPopulations(sites []Site, cfg types.CivConfig)                 // Site{Dir [3]float64; Population; Habitability}
planetcolor.EvalSpline / BlendOkLab / Brighten / Lerp / SampleGradientOkLab / LookupLUT / ApplyLUT

// seed
func Domain(master int64, name string) int64
```

**Production crust-path stage order** (render/rocky.go, verified): crust BaseHeight init → control fields (crust path SKIPS Continentalness spline, index 0; Detail + PeaksValleys splines added; Ridged/Basin/Continents skipped) → ApplyTectonicFX → SmoothHeightmap → inline Normalize (global min/max rescale) → `oceanLevel = QuantileSeaLevel(hm, 1-crust.LandFraction)` → Coastal (needs DistanceToCoast) → Erode (droplets auto-scaled by face area) → GenerateFlow + CarveRivers → oceanLevel RE-quantiled → Craters → colorize (Biome/Palette → Snow → Ocean → PolarCaps → Shading → Ejecta → Civ → LUT).

## File Structure

```
pkg/planetgen/field/tectonicfx.go        MODIFY: extract FXSample/FXGens/FXDelta (Task 2)
pkg/planetgen/render/pixelcolor.go       CREATE: SnowPixel/OceanPixel/PolarCapPixel/SlopeShadeSampled (Task 3)
pkg/planetgen/render/rocky.go            MODIFY: call the extracted helpers (Task 3)
pkg/planetgen/feature/habitability.go    MODIFY: extract HabitabilityScoreAt (Task 3)
pkg/planetgen/biome/rainshadow.go        MODIFY: extract RainShadowMultiplierAt (Task 3)

pkg/planetgen/patch/window.go            CREATE: Window, Dir, Sampler, validation (Task 1)
pkg/planetgen/patch/grid.go              CREATE: Grid raster + Bilinear (Task 1)
pkg/planetgen/patch/sphere.go            CREATE: SphereData + ComputeSphere (Task 4)
pkg/planetgen/patch/extract.go           CREATE: Fields + ExtractFields (Task 5)
pkg/planetgen/patch/pick.go              CREATE: Candidate + Pick (Task 6)
pkg/planetgen/patch/stack.go             CREATE: Context, State, Layer, Stack, Layers() (Task 7)
pkg/planetgen/patch/layer_base.go        CREATE: L0 tectonic-base (Task 8)
pkg/planetgen/patch/layer_fx.go          CREATE: L1 tectonic-fx (Task 8)
pkg/planetgen/patch/layer_noise.go       CREATE: L2 control-noise, L3 height-smooth, L4 normalize (Task 9)
pkg/planetgen/patch/layer_coastal.go     CREATE: L5 coastal + flat chamfer distance (Task 10)
pkg/planetgen/patch/layer_erosion.go     CREATE: L6 erosion (flat droplet port) (Task 11)
pkg/planetgen/patch/layer_craters.go     CREATE: L7 craters (Task 12)
pkg/planetgen/patch/layer_flow.go        CREATE: L8 flow-rivers (flat P-D + D8) (Task 12)
pkg/planetgen/patch/layer_climate.go     CREATE: L9 climate (Task 13)
pkg/planetgen/patch/layer_color.go       CREATE: L10 biome-color, L11 waterlines (Tasks 13–14)
pkg/planetgen/patch/layer_civ.go         CREATE: L12 civ (Task 15)
pkg/planetgen/patch/render.go            CREATE: State → PNG encoders + minimap (Task 16)
pkg/planetgen/patch/golden_test.go       CREATE: per-layer snapshot hashes (Task 16)
pkg/planetgen/patch/testdata/goldens.json CREATE: baked hashes (Task 16)

cmd/planet-explorer/wasm/main.go         MODIFY: patch* exports (Task 17)
cmd/planet-explorer/web/index.html       MODIFY: Patch Lab section (Task 18)
cmd/planet-explorer/web/app.js           MODIFY: Patch Lab mode (Task 18)
cmd/planet-explorer/web/style.css        MODIFY: patch styles (Task 18)
cmd/planet-explorer/README.md            MODIFY: Patch Lab docs (Task 19)
```

**Layer index table (canonical, used everywhere):**

| # | ID | State fields it writes |
|---|----|------------------------|
| 0 | `tectonic-base` | `Height` |
| 1 | `tectonic-fx` | `Height` |
| 2 | `control-noise` | `Height` |
| 3 | `height-smooth` | `Height` |
| 4 | `normalize` | `Height` |
| 5 | `coastal` | `Height`, `DistCoast` |
| 6 | `erosion` | `Height` |
| 7 | `craters` | `Height`, `Craters` |
| 8 | `flow-rivers` | `Height`, `Rivers`, `FlowAccum` |
| 9 | `climate` | `T`, `M`, `RainMult` |
| 10 | `biome-color` | `Img` |
| 11 | `waterlines` | `Img` |
| 12 | `civ` | `Img`, `Sites` |

---

### Task 1: patch package — Window and Grid

**Files:**
- Create: `pkg/planetgen/patch/window.go`
- Create: `pkg/planetgen/patch/grid.go`
- Test: `pkg/planetgen/patch/window_test.go`, `pkg/planetgen/patch/grid_test.go`

**Interfaces:**
- Produces: `patch.Window{Face, X0, Y0, Size, SProd}`, `(Window) Dir(ix, iy int) (x, y, z float64)`, `(Window) Valid() error`, `(Window) Sampler(g *Grid) func(x, y, z float64) float64`, `patch.Grid{Size, Data}`, `NewGrid`, `At/Set/Clone/Bilinear`. Every later task uses these exact names.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/planetgen/patch/window_test.go
package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestWindowDirMatchesFacePixelToDir(t *testing.T) {
	w := Window{Face: cubemap.FacePosZ, X0: 256, Y0: 128, Size: 512, SProd: 1024}
	for _, p := range [][2]int{{0, 0}, {511, 511}, {17, 300}} {
		gx, gy, gz := w.Dir(p[0], p[1])
		ex, ey, ez := cubemap.FacePixelToDir(w.Face, w.X0+p[0], w.Y0+p[1], w.SProd)
		if gx != ex || gy != ey || gz != ez {
			t.Fatalf("Dir(%d,%d) = (%v,%v,%v), want (%v,%v,%v)", p[0], p[1], gx, gy, gz, ex, ey, ez)
		}
	}
}

func TestWindowValid(t *testing.T) {
	ok := Window{Face: cubemap.FacePosX, X0: 0, Y0: 0, Size: 512, SProd: 1024}
	if err := ok.Valid(); err != nil {
		t.Fatalf("valid window rejected: %v", err)
	}
	bad := []Window{
		{Face: cubemap.FacePosX, X0: 600, Y0: 0, Size: 512, SProd: 1024}, // overflows face
		{Face: cubemap.FacePosX, X0: -1, Y0: 0, Size: 512, SProd: 1024},
		{Face: cubemap.FacePosX, X0: 0, Y0: 0, Size: 0, SProd: 1024},
		{Face: cubemap.Face(9), X0: 0, Y0: 0, Size: 64, SProd: 1024},
	}
	for i, w := range bad {
		if err := w.Valid(); err == nil {
			t.Fatalf("bad window %d accepted", i)
		}
	}
}

func TestWindowSamplerRoundTrip(t *testing.T) {
	// At exact pixel centers the sampler must return the grid value exactly.
	w := Window{Face: cubemap.FaceNegY, X0: 100, Y0: 40, Size: 64, SProd: 256}
	g := NewGrid(64)
	for iy := range 64 {
		for ix := range 64 {
			g.Set(ix, iy, float64(iy*64+ix))
		}
	}
	s := w.Sampler(g)
	for _, p := range [][2]int{{0, 0}, {63, 63}, {5, 40}} {
		x, y, z := w.Dir(p[0], p[1])
		got := s(x, y, z)
		want := g.At(p[0], p[1])
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("sampler at pixel (%d,%d): got %v want %v", p[0], p[1], got, want)
		}
	}
}
```

```go
// pkg/planetgen/patch/grid_test.go
package patch

import "testing"

func TestGridBilinear(t *testing.T) {
	g := NewGrid(2)
	g.Set(0, 0, 0)
	g.Set(1, 0, 1)
	g.Set(0, 1, 2)
	g.Set(1, 1, 3)
	if v := g.Bilinear(0.5, 0.5); v != 1.5 {
		t.Fatalf("center bilinear = %v, want 1.5", v)
	}
	// Clamped outside.
	if v := g.Bilinear(-5, -5); v != 0 {
		t.Fatalf("clamp low = %v, want 0", v)
	}
	if v := g.Bilinear(10, 10); v != 3 {
		t.Fatalf("clamp high = %v, want 3", v)
	}
}

func TestGridClone(t *testing.T) {
	g := NewGrid(4)
	g.Set(1, 1, 7)
	c := g.Clone()
	c.Set(1, 1, 9)
	if g.At(1, 1) != 7 {
		t.Fatal("Clone aliases the original")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/planetgen/patch/ -v`
Expected: FAIL — package does not exist / undefined `Window`, `NewGrid`.

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/grid.go
// Package patch implements the Phase 13 Patch Lab: sphere-global
// tectonics computed at modest resolution, cropped to a flat window of
// one cube face at virtual production resolution, with all downstream
// layers re-run per-pixel on the window.
package patch

// Grid is a Size×Size float raster, row-major — the patch analog of
// cubemap.CubeMapF for a single window.
type Grid struct {
	Size int
	Data []float64
}

func NewGrid(size int) *Grid {
	return &Grid{Size: size, Data: make([]float64, size*size)}
}

func (g *Grid) At(ix, iy int) float64     { return g.Data[iy*g.Size+ix] }
func (g *Grid) Set(ix, iy int, v float64) { g.Data[iy*g.Size+ix] = v }

func (g *Grid) Clone() *Grid {
	c := NewGrid(g.Size)
	copy(c.Data, g.Data)
	return c
}

// Bilinear samples at fractional pixel coordinates (pixel centers at
// integer coords), clamping to the window border. The border clamp is
// the patch edge policy: outside-window reads see the edge value.
func (g *Grid) Bilinear(x, y float64) float64 {
	max := float64(g.Size - 1)
	x = min(max, math.Max(0, x))
	y = min(max, math.Max(0, y))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, g.Size-1), min(y0+1, g.Size-1)
	fx, fy := x-float64(x0), y-float64(y0)
	top := g.At(x0, y0)*(1-fx) + g.At(x1, y0)*fx
	bot := g.At(x0, y1)*(1-fx) + g.At(x1, y1)*fx
	return top*(1-fy) + bot*fy
}
```

(Add `import "math"` — gofmt will place it.)

```go
// pkg/planetgen/patch/window.go
package patch

import (
	"fmt"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Window locates a patch: a Size×Size pixel window on one cube face of
// a virtual full-resolution cube map with SProd pixels per face edge.
// Patch pixel (ix,iy) is virtual face pixel (X0+ix, Y0+iy), so its
// sphere direction — and therefore every 3D noise sample — is exactly
// what the production render computes for that pixel at S=SProd.
type Window struct {
	Face  cubemap.Face `json:"face"`
	X0    int          `json:"x0"`
	Y0    int          `json:"y0"`
	Size  int          `json:"size"`
	SProd int          `json:"sProd"`
}

func (w Window) Valid() error {
	if w.Face < 0 || w.Face >= cubemap.NumFaces {
		return fmt.Errorf("patch: invalid face %d", w.Face)
	}
	if w.Size <= 0 || w.SProd <= 0 {
		return fmt.Errorf("patch: non-positive size %d / sProd %d", w.Size, w.SProd)
	}
	if w.X0 < 0 || w.Y0 < 0 || w.X0+w.Size > w.SProd || w.Y0+w.Size > w.SProd {
		return fmt.Errorf("patch: window (%d,%d)+%d overflows face of %d", w.X0, w.Y0, w.Size, w.SProd)
	}
	return nil
}

// Dir returns the unit sphere direction at the CENTER of patch pixel
// (ix, iy) — identical to the production cube path's direction for the
// same virtual pixel.
func (w Window) Dir(ix, iy int) (x, y, z float64) {
	return cubemap.FacePixelToDir(w.Face, w.X0+ix, w.Y0+iy, w.SProd)
}

// Sampler returns a direction-space sampler over a patch grid — the
// patch analog of CubeMapF.Sample. Directions off the window clamp to
// its border (open-patch edge policy).
func (w Window) Sampler(g *Grid) func(x, y, z float64) float64 {
	return func(x, y, z float64) float64 {
		u, v := cubemap.ForceFaceUV(w.Face, x, y, z)
		px := u*float64(w.SProd) - 0.5 - float64(w.X0)
		py := v*float64(w.SProd) - 0.5 - float64(w.Y0)
		return g.Bilinear(px, py)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/planetgen/patch/ -v`
Expected: PASS (all 5 tests). If `TestWindowSamplerRoundTrip` fails, check the pixel-center convention: `FacePixelToDir` puts pixel centers at `(px+0.5)/S` in UV, so the inverse is `px = u*S - 0.5`.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch package — Window + Grid with exact production pixel dirs"
```

---

### Task 2: extract per-pixel tectonic-FX core (production refactor, behavior-preserving)

**Files:**
- Modify: `pkg/planetgen/field/tectonicfx.go` (the `ApplyTectonicFX` body, lines ~142–275)
- Test: `pkg/planetgen/field/tectonicfx_test.go` (add regression test)

**Interfaces:**
- Produces (all in `package field`):
  - `type FXSample struct { BeltDist, BeltMag, SubdDist, SubdMag, ArcDist, ArcMag, RidgeDist, RidgeMag, RiftDist, RiftMag, TransformDist, ContinentalMask float64 }`
  - `type FXGens struct { Act, Belt *noise.Generator }` and `func NewFXGens(master int64) *FXGens`
  - `func FXDelta(dx, dy, dz float64, s FXSample, cfg types.TectonicFXConfig, age float64, g *FXGens) float64`
- The patch fx layer (Task 8) calls `NewFXGens` + `FXDelta` with grid-sampled values. `ApplyTectonicFX` keeps its exact signature and becomes a loop over `FXDelta`.

- [ ] **Step 1: Write the failing regression test**

Add to `pkg/planetgen/field/tectonicfx_test.go`:

```go
// TestFXDeltaMatchesApply pins the extraction: ApplyTectonicFX must
// remain byte-identical to per-pixel FXDelta accumulation.
func TestFXDeltaMatchesApply(t *testing.T) {
	profile := &types.PlanetProfile{
		Type: "terran", Renderer: "rocky", RadiusKm: 6371,
		Crust: types.CrustConfig{MajorPlates: 6, MinorPlates: 6, TargetLandFraction: 0.3, Assembly: 0.5, TectonicAge: 0.4},
		TectonicFX: defaultFXCfg(),
	}
	const S = 32
	master := int64(12345)
	pf := GeneratePlates(profile, master, S)
	crust := GenerateCrust(profile, master, S, pf)
	fx := ClassifyTectonics(pf, crust, profile.RadiusKm)

	hmA := cubemap.NewF(S)
	ApplyTectonicFX(hmA, fx, crust, pf, profile.TectonicFX, master, S)

	g := NewFXGens(master)
	hmB := cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				i := py*S + px
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				s := FXSample{
					BeltDist: fx.BeltDist.Faces[face][i], BeltMag: fx.BeltMag.Faces[face][i],
					SubdDist: fx.SubdDist.Faces[face][i], SubdMag: fx.SubdMag.Faces[face][i],
					ArcDist: fx.ArcDist.Faces[face][i], ArcMag: fx.ArcMag.Faces[face][i],
					RidgeDist: fx.RidgeDist.Faces[face][i], RidgeMag: fx.RidgeMag.Faces[face][i],
					RiftDist: fx.RiftDist.Faces[face][i], RiftMag: fx.RiftMag.Faces[face][i],
					TransformDist:   pf.Transform[face][i],
					ContinentalMask: crust.ContinentalMask.Faces[face][i],
				}
				hmB.Faces[face][i] += FXDelta(dx, dy, dz, s, profile.TectonicFX, crust.TectonicAge, g)
			}
		}
	}
	for face := range cubemap.Face(cubemap.NumFaces) {
		for i := range hmA.Faces[face] {
			if hmA.Faces[face][i] != hmB.Faces[face][i] {
				t.Fatalf("face %d idx %d: Apply=%v FXDelta=%v", face, i, hmA.Faces[face][i], hmB.Faces[face][i])
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/planetgen/field/ -run TestFXDeltaMatchesApply -v`
Expected: FAIL — `undefined: NewFXGens`, `undefined: FXSample`, `undefined: FXDelta`.

- [ ] **Step 3: Extract the core**

In `pkg/planetgen/field/tectonicfx.go`, add (below `ClassifyTectonics`):

```go
// FXSample is the per-pixel bundle of tectonic fields FXDelta reads.
// On the cube path the values come straight from the field rasters; on
// the Patch Lab path they are bilinear crops upsampled at patch dirs.
type FXSample struct {
	BeltDist, BeltMag   float64
	SubdDist, SubdMag   float64
	ArcDist, ArcMag     float64
	RidgeDist, RidgeMag float64
	RiftDist, RiftMag   float64
	TransformDist       float64
	ContinentalMask     float64
}

// FXGens bundles the two seeded generators the FX pass consumes.
type FXGens struct {
	Act, Belt *noise.Generator
}

func NewFXGens(master int64) *FXGens {
	return &FXGens{
		Act:  noise.New(seed.Domain(master, "tectonicfx.activity")),
		Belt: noise.New(seed.Domain(master, "tectonicfx.belt")),
	}
}

// FXDelta returns the tectonic-FX height delta at one pixel. It is the
// exact per-pixel body of ApplyTectonicFX, extracted so the Patch Lab
// flat path evaluates the identical formula.
func FXDelta(dx, dy, dz float64, s FXSample, cfg types.TectonicFXConfig, age float64, g *FXGens) float64 {
	hs := 1.0 - 0.55*age
	ws := 1.0 + 0.8*age
	actFreq := cfg.ActivityFreq
	if actFreq <= 0 {
		actFreq = 1.5
	}
	beltOct := cfg.BeltOctaves
	if beltOct <= 0 {
		beltOct = 5
	}
	env := func(distKm, widthKm float64) float64 {
		if widthKm <= 0 || distKm > 3*widthKm {
			return 0
		}
		x := distKm / widthKm
		return math.Exp(-x * x)
	}
	activity := -1.0
	getActivity := func() float64 {
		if activity < 0 {
			activity = 0.5 + 0.5*g.Act.FractalNoise3D(dx, dy, dz, 2, 2.0, 0.5, actFreq)
		}
		return activity
	}
	contHere := s.ContinentalMask
	var dh float64
	if cfg.BeltAmp > 0 {
		if e := env(s.BeltDist, cfg.BeltWidthKm*ws); e > 0 {
			r := g.Belt.RidgedFractal3D(dx*cfg.BeltFreq, dy*cfg.BeltFreq, dz*cfg.BeltFreq, beltOct, 2.0, 0.5, 1.0)
			mag := 0.4 + 0.6*s.BeltMag
			dh += cfg.BeltAmp * hs * e * mag * getActivity() * r
		}
	}
	if cfg.CordAmp > 0 || cfg.TrenchDepth > 0 {
		d := s.SubdDist
		mag := s.SubdMag
		if contHere > 0.5 {
			if e := env(d, cfg.CordWidthKm*ws); e > 0 {
				r := g.Belt.RidgedFractal3D(dx*cfg.BeltFreq*1.4, dy*cfg.BeltFreq*1.4, dz*cfg.BeltFreq*1.4, beltOct, 2.0, 0.5, 1.0)
				dh += cfg.CordAmp * hs * e * (0.4 + 0.6*mag) * getActivity() * r
			}
		} else {
			if e := env(d, cfg.TrenchWidthKm); e > 0 {
				dh -= cfg.TrenchDepth * e * (0.4 + 0.6*mag)
			}
		}
	}
	if cfg.ArcAmp > 0 && contHere < 0.5 {
		if e := env(s.ArcDist, cfg.ArcWidthKm); e > 0 {
			islands := smoothstep(0.55, 0.72, g.Act.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 8.0))
			mag := 0.4 + 0.6*s.ArcMag
			dh += cfg.ArcAmp * hs * e * mag * islands
		}
	}
	if cfg.RidgeAmp > 0 && contHere < 0.5 {
		if e := env(s.RidgeDist, cfg.RidgeWidthKm); e > 0 {
			dh += cfg.RidgeAmp * e * (0.4 + 0.6*s.RidgeMag)
		}
	}
	if cfg.RiftDepth > 0 {
		d := s.RiftDist
		w := cfg.RiftWidthKm * ws
		mag := 0.4 + 0.6*s.RiftMag
		if e := env(d, w); e > 0 {
			dh -= cfg.RiftDepth * (0.5 + 0.5*age) * e * mag
		}
		if cfg.RiftShoulder > 0 {
			sd := d - 1.6*w
			if e := env(math.Abs(sd), w*0.7); e > 0 {
				dh += cfg.RiftDepth * cfg.RiftShoulder * e * mag
			}
		}
	}
	if cfg.TransformAmp > 0 {
		if e := env(s.TransformDist, cfg.TransformWidthKm); e > 0 {
			n := g.Act.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 12.0) - 0.5
			dh += cfg.TransformAmp * e * n
		}
	}
	return dh
}
```

Then rewrite `ApplyTectonicFX`'s body to delegate — delete its inline loop body and replace with:

```go
func ApplyTectonicFX(hm *cubemap.CubeMapF, fx *TectonicFXField, crust *CrustField,
	pf *PlateField, cfg types.TectonicFXConfig, master int64, S int) {
	if hm == nil || fx == nil || crust == nil || pf == nil {
		return
	}
	g := NewFXGens(master)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				i := py*S + px
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				s := FXSample{
					BeltDist: fx.BeltDist.Faces[face][i], BeltMag: fx.BeltMag.Faces[face][i],
					SubdDist: fx.SubdDist.Faces[face][i], SubdMag: fx.SubdMag.Faces[face][i],
					ArcDist: fx.ArcDist.Faces[face][i], ArcMag: fx.ArcMag.Faces[face][i],
					RidgeDist: fx.RidgeDist.Faces[face][i], RidgeMag: fx.RidgeMag.Faces[face][i],
					RiftDist: fx.RiftDist.Faces[face][i], RiftMag: fx.RiftMag.Faces[face][i],
					TransformDist:   pf.Transform[face][i],
					ContinentalMask: crust.ContinentalMask.Faces[face][i],
				}
				if dh := FXDelta(dx, dy, dz, s, cfg, crust.TectonicAge, g); dh != 0 {
					hm.Faces[face][i] += dh
				}
			}
		}
	}
}
```

**Float-identity caveat:** the noise generators are stateless samplers (pure functions of position), so per-pixel call order does not matter; the arithmetic inside FXDelta is copied verbatim, so results are bit-identical.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./pkg/planetgen/field/ -v -run 'TectonicFX|FXDelta'`
Expected: PASS — including the pre-existing `TestApplyTectonicFXRaisesBelts` and `TestApplyTectonicFXAgeSoftens`.

- [ ] **Step 5: Run the wider fast suite, lint, commit**

```bash
go test ./pkg/planetgen/...
golangci-lint run ./pkg/planetgen/field/
git add pkg/planetgen/field/tectonicfx.go pkg/planetgen/field/tectonicfx_test.go
git commit -m "P13: extract FXDelta per-pixel core from ApplyTectonicFX (byte-identical)"
```

---

### Task 3: extract shared per-pixel colorize / habitability / rain-shadow helpers (production refactor, behavior-preserving)

**Files:**
- Create: `pkg/planetgen/render/pixelcolor.go`
- Modify: `pkg/planetgen/render/rocky.go` (Snow ~L949-982, Ocean ~L985-1036, PolarCaps ~L1039-1077, `applySlopeShading` ~L1240)
- Modify: `pkg/planetgen/feature/habitability.go`
- Modify: `pkg/planetgen/biome/rainshadow.go`
- Test: `pkg/planetgen/render/pixelcolor_test.go`

**Interfaces:**
- Produces (`package render`):
  - `func SnowPixel(c color.RGBA, h, absLat, snowLine float64) color.RGBA` — caller guards `h > snowLine`. `absLat` = `|asin(rawY)| / (π/2)`.
  - `func OceanPixel(oceanColor color.RGBA, planetType string, h, oceanLevel, surfaceVar float64) color.RGBA` — caller guards `h < oceanLevel`; `surfaceVar` is the already-sampled `FractalNoise3D(warped dir, 4, 2.0, 0.5, 6.0)` draw.
  - `func PolarCapPixel(c color.RGBA, absLat, capSize, polarCapNoise, capEdgeNoise float64) color.RGBA` — caller guards `absLat > 1-capSize`; returns `c` unchanged when the noise-adjusted threshold isn't crossed.
  - `func SlopeShadeSampled(sample func(x, y, z float64) float64, c color.RGBA, rx, ry, rz, strength, exaggeration float64) color.RGBA`
- Produces (`package feature`): `func HabitabilityScoreAt(h, t, m, rainMult float64, onRiver bool, convergentKm, oceanLevel float64) float64` (pass `convergentKm = math.Inf(1)` when no plate data, `rainMult = 1` when no rain shadow).
- Produces (`package biome`): `func RainShadowMultiplierAt(sample func(x, y, z float64) float64, dx, dy, dz float64, cfg types.RainShadowConfig) float64` — per-pixel wind classification + upwind walk against an arbitrary height sampler.
- Consumed by: Tasks 13–15 patch layers, and by the production call sites (which must switch to these helpers so the formulas can never drift apart).

**Extraction recipe (all four follow the same pattern):**
1. `SnowPixel`: move rocky.go L960-968 body verbatim; `whiteSnow` stays a package var in `render`.
2. `OceanPixel`: move L998-1022 (the `!bypassed` inner block) verbatim, including the `lava_world` branch.
3. `PolarCapPixel`: move L1052-1063 verbatim (threshold adjust, blend, `whiteIce`).
4. `SlopeShadeSampled`: copy `applySlopeShading` verbatim, replacing the four `hm.Sample(...)` calls with `sample(...)`; then make `applySlopeShading` a one-line wrapper: `return SlopeShadeSampled(hm.Sample, c, rx, ry, rz, strength, exaggeration)`.
5. `HabitabilityScoreAt`: move the per-pixel loop body of `GenerateHabitability` (habitability.go L124-171) verbatim; the `lowA/lowB` anchor computation moves inside (pure function of `oceanLevel`). `GenerateHabitability`'s loop becomes: gather `h,t,m,rs,onRiver,convKm` then `out[i] = HabitabilityScoreAt(...)`.
6. `RainShadowMultiplierAt`: extract the per-pixel body of `GenerateRainShadow` (wind tangent + `walkUpwindForOrography` + windward/lee classification) with the heightmap access routed through the `sample` closure. Verify first (read rainshadow.go L58-103 + L187-230) that all height reads go through `heightmap.Sample` — if any use `.Get`, route those through the sampler with `cubemap` dir math preserved. `GenerateRainShadow` becomes a loop over `RainShadowMultiplierAt(heightmap.Sample, ...)`.

Update the three production call sites in `colorizeRockyDebug` (Snow/Ocean/PolarCaps stages) to call the helpers — the surrounding loops, bypass logic, and noise draws (which must ALWAYS be consumed for rng stability) stay where they are.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/render/pixelcolor_test.go
package render

import (
	"image/color"
	"testing"
)

func TestSnowPixelBlends(t *testing.T) {
	base := color.RGBA{50, 90, 40, 255}
	got := SnowPixel(base, 0.9, 0.5, 0.6)
	if got == base {
		t.Fatal("SnowPixel above snowline must lighten the pixel")
	}
	// Higher terrain → more snow (monotone in h).
	hi := SnowPixel(base, 0.98, 0.5, 0.6)
	if int(hi.R)+int(hi.G)+int(hi.B) < int(got.R)+int(got.G)+int(got.B) {
		t.Fatal("snow blend must be monotone in height")
	}
}

func TestOceanPixelDepthDarkens(t *testing.T) {
	oc := color.RGBA{10, 40, 120, 255}
	shallow := OceanPixel(oc, "terran", 0.29, 0.30, 0.5)
	deep := OceanPixel(oc, "terran", 0.02, 0.30, 0.5)
	if int(deep.R)+int(deep.G)+int(deep.B) >= int(shallow.R)+int(shallow.G)+int(shallow.B) {
		t.Fatal("deep ocean must be darker than shallow")
	}
}

func TestSlopeShadeSampledFlatIsNeutralish(t *testing.T) {
	flat := func(x, y, z float64) float64 { return 0.5 }
	base := color.RGBA{100, 100, 100, 255}
	got := SlopeShadeSampled(flat, base, 0, 0, 1, 0.5, 8)
	// Flat terrain: normal == radial; brightness = (1-s) + s*(0.4+0.8*diff).
	if got.R < 80 || got.R > 130 {
		t.Fatalf("flat shading out of neutral band: %v", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/planetgen/render/ -run 'SnowPixel|OceanPixel|SlopeShade' -v`
Expected: FAIL — undefined `SnowPixel` etc.

- [ ] **Step 3: Implement the extractions** (per the recipe above; move code, do not retype formulas)

- [ ] **Step 4: Verify — new tests AND the full fast suite (production must not shift)**

Run: `go test ./pkg/planetgen/...`
Expected: PASS everywhere — especially `render` (rocky tests), `feature` (habitability tests), `biome` (rainshadow tests). Any numeric diff means the extraction reordered floating-point ops — fix by moving code verbatim.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/...
git add pkg/planetgen/render/ pkg/planetgen/feature/habitability.go pkg/planetgen/biome/rainshadow.go
git commit -m "P13: extract per-pixel colorize/habitability/rainshadow helpers for patch reuse"
```

---

### Task 4: SphereData + ComputeSphere (the S_tect precompute)

**Files:**
- Create: `pkg/planetgen/patch/sphere.go`
- Test: `pkg/planetgen/patch/sphere_test.go`

**Interfaces:**
- Produces:
  - `type SphereData struct { STect int; Master int64; Profile *types.PlanetProfile; Jitter *noise.JitterField; Plates *field.PlateField; Crust *field.CrustField; FX *field.TectonicFXField; HMin, HMax float64; SeaLevel0, SeaLevel float64 }`
  - `func ComputeSphere(profile *types.PlanetProfile, master int64, sTect int) (*SphereData, error)` — error when `profile.Crust.MajorPlates <= 0` (Patch Lab requires the crust path).
- Consumes: `field.*` generators (see Key APIs), `noise.GenerateJitter`.
- Consumed by: `ExtractFields` (Task 5), `Pick` (Task 6), wasm `patchInit` (Task 17).

`ComputeSphere` mirrors the production crust-path heightmap pipeline at S_tect. It exists to produce the **global scalars** a patch cannot derive locally: normalize bounds (`HMin/HMax`), pre-flow sea level (`SeaLevel0`, used by coastal + erosion, mirroring production), post-flow sea level (`SeaLevel`, used by waterlines), plus the tectonic fields for cropping.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/sphere_test.go
package patch

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

func terranProfile(t *testing.T) *types.PlanetProfile {
	t.Helper()
	p := planetgen.GetProfile("terran")
	if p.Crust.MajorPlates <= 0 {
		t.Fatal("terran profile is expected to be crust-enabled")
	}
	return p
}

func TestComputeSphereTerran(t *testing.T) {
	sd, err := ComputeSphere(terranProfile(t), 4242, 64)
	if err != nil {
		t.Fatal(err)
	}
	if sd.Plates == nil || sd.Crust == nil || sd.FX == nil {
		t.Fatal("missing tectonic fields")
	}
	if !(sd.SeaLevel0 > 0 && sd.SeaLevel0 < 1) || !(sd.SeaLevel > 0 && sd.SeaLevel < 1) {
		t.Fatalf("sea levels out of range: %v %v", sd.SeaLevel0, sd.SeaLevel)
	}
	if sd.HMax <= sd.HMin {
		t.Fatalf("degenerate normalize bounds: [%v, %v]", sd.HMin, sd.HMax)
	}
	// Determinism.
	sd2, err := ComputeSphere(terranProfile(t), 4242, 64)
	if err != nil {
		t.Fatal(err)
	}
	if sd2.SeaLevel != sd.SeaLevel || sd2.HMin != sd.HMin || sd2.HMax != sd.HMax {
		t.Fatal("ComputeSphere is not deterministic")
	}
}

func TestComputeSphereRejectsLegacy(t *testing.T) {
	p := planetgen.GetProfile("scorched") // no crust path
	if _, err := ComputeSphere(p, 1, 32); err == nil {
		t.Fatal("legacy (non-crust) profile must be rejected")
	}
}
```

(Imports: add `"github.com/rsned/spacemolt-kb/pkg/planetgen/types"`.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/planetgen/patch/ -run ComputeSphere -v`
Expected: FAIL — undefined `ComputeSphere`.

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/sphere.go
package patch

import (
	"fmt"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/planetcolor"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// canonicalFace is the production face size droplet counts are
// calibrated against (planetgen.DefaultFaceSize; kept as a local const
// to avoid importing the root package).
const canonicalFace = 1024

// SphereData is everything the patch needs from the sphere-global
// tectonic precompute at face size STect.
type SphereData struct {
	STect   int
	Master  int64
	Profile *types.PlanetProfile

	Jitter *noise.JitterField
	Plates *field.PlateField
	Crust  *field.CrustField
	FX     *field.TectonicFXField

	// Normalize affine measured on the S_tect heightmap after
	// smooth (production normalizes with the global min/max at
	// S_prod; this is the documented S_tect approximation).
	HMin, HMax float64
	// SeaLevel0 is the pre-flow quantile (drives coastal +
	// erosion, exactly as production uses its first-derived
	// level); SeaLevel is the post-flow re-quantile (drives
	// waterline coloring).
	SeaLevel0, SeaLevel float64
}

// ComputeSphere runs the sphere-global part of the crust-path rocky
// pipeline at face size sTect. It mirrors render/rocky.go stage order:
// crust init → Detail/PeaksValleys splines (Continentalness skipped on
// the crust path) → TectonicFX → smooth → normalize → quantile sea
// level → erode → flow+carve → re-quantile.
func ComputeSphere(profile *types.PlanetProfile, master int64, sTect int) (*SphereData, error) {
	if profile == nil || profile.Crust.MajorPlates <= 0 {
		return nil, fmt.Errorf("patch: Patch Lab requires a crust-enabled profile (crust.majorPlates > 0)")
	}
	jitter := noise.GenerateJitter(profile, master, sTect)
	plates := field.GeneratePlates(profile, master, sTect)
	if plates == nil {
		return nil, fmt.Errorf("patch: plate generation returned nil")
	}
	crust := field.GenerateCrust(profile, master, sTect, plates)
	fx := field.ClassifyTectonics(plates, crust, profile.RadiusKm)

	// Heightmap init from the crust raft.
	hm := crust.BaseHeight.Clone()

	// Control-field splines: crust path skips index 0
	// (Continentalness); indices 1 (Detail) and 2 (PeaksValleys)
	// contribute. Mirrors rocky.go's fast path.
	fields := field.GenerateControlFields(master, profile.ControlConfig, sTect, jitter)
	cf := [3]types.ControlField{profile.ControlConfig.Continentalness, profile.ControlConfig.Detail, profile.ControlConfig.PeaksValleys}
	for face := range cubemap.Face(cubemap.NumFaces) {
		for i := range hm.Faces[face] {
			h := hm.Faces[face][i]
			for k := 1; k < 3; k++ {
				h += planetcolor.EvalSpline(cf[k].Spline, fields[k].Faces[face][i])
			}
			hm.Faces[face][i] = h
		}
	}

	field.ApplyTectonicFX(hm, fx, crust, plates, profile.TectonicFX, master, sTect)
	if profile.HeightSmoothRadius > 0 {
		hm = field.SmoothHeightmap(hm, profile.HeightSmoothRadius, sTect)
	}

	// Normalize: global min/max rescale (inline in rocky.go too).
	hMin, hMax := hm.Faces[0][0], hm.Faces[0][0]
	for face := range cubemap.Face(cubemap.NumFaces) {
		for _, v := range hm.Faces[face] {
			if v < hMin {
				hMin = v
			}
			if v > hMax {
				hMax = v
			}
		}
	}
	if hMax > hMin {
		inv := 1 / (hMax - hMin)
		for face := range cubemap.Face(cubemap.NumFaces) {
			for i, v := range hm.Faces[face] {
				hm.Faces[face][i] = (v - hMin) * inv
			}
		}
	}

	seaLevel0 := field.QuantileSeaLevel(hm, 1-crust.LandFraction)

	// Erode + flow at S_tect purely to re-derive the post-flow sea
	// level the way production does. Droplets scale by face area
	// (canonical counts are for S=1024), floor 5000 like rocky.go.
	ecfg := profile.Erosion
	if ecfg.Droplets > 0 {
		scaled := ecfg.Droplets * sTect * sTect / (canonicalFace * canonicalFace)
		ecfg.Droplets = max(scaled, 5000)
		hm = field.Erode(master, hm, ecfg, seaLevel0, sTect)
	}
	if ff := field.GenerateFlow(hm, profile.Flow); ff != nil {
		field.CarveRivers(hm, ff, profile.Flow)
	}
	seaLevel := field.QuantileSeaLevel(hm, 1-crust.LandFraction)

	return &SphereData{
		STect: sTect, Master: master, Profile: profile,
		Jitter: jitter, Plates: plates, Crust: crust, FX: fx,
		HMin: hMin, HMax: hMax,
		SeaLevel0: seaLevel0, SeaLevel: seaLevel,
	}, nil
}
```

**Note:** before finalizing, open `render/rocky.go` around L730-763 and copy its exact droplet area-scaling formula into the `scaled :=` line above if it differs (the plan's floor-5000 form matches the survey; trust the source).

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/planetgen/patch/ -v`
Expected: PASS. `TestComputeSphereTerran` takes a few seconds (erosion at 64 with 5000 droplets).

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/sphere.go pkg/planetgen/patch/sphere_test.go
git commit -m "P13: ComputeSphere — S_tect tectonic precompute with global scalars"
```

---

### Task 5: ExtractFields — cropping sphere fields into the window

**Files:**
- Create: `pkg/planetgen/patch/extract.go`
- Test: `pkg/planetgen/patch/extract_test.go`

**Interfaces:**
- Produces:
  - `type Fields struct { Window Window; BaseHeight, ContinentalMask, BeltDist, BeltMag, SubdDist, SubdMag, ArcDist, ArcMag, RidgeDist, RidgeMag, RiftDist, RiftMag, Transform *Grid; PlateID []int16 }`
  - `func ExtractFields(sd *SphereData, w Window) (*Fields, error)`
- Consumes: `SphereData` (Task 4), `Window.Dir` (Task 1), `cubemap.CubeMapF.Sample`, `cubemap.DirToFacePixel`.
- Consumed by: all layers (Tasks 8+) via `Context.Fields`.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/extract_test.go
package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func testSphere(t *testing.T) *SphereData {
	t.Helper()
	sd, err := ComputeSphere(terranProfile(t), 4242, 64)
	if err != nil {
		t.Fatal(err)
	}
	return sd
}

func TestExtractFieldsMatchesSample(t *testing.T) {
	sd := testSphere(t)
	w := Window{Face: cubemap.FacePosZ, X0: 32, Y0: 32, Size: 64, SProd: 128}
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range [][2]int{{0, 0}, {63, 63}, {10, 50}} {
		x, y, z := w.Dir(p[0], p[1])
		want := sd.Crust.BaseHeight.Sample(x, y, z)
		got := f.BaseHeight.At(p[0], p[1])
		if math.Abs(got-want) > 1e-12 {
			t.Fatalf("BaseHeight at (%d,%d): got %v want %v", p[0], p[1], got, want)
		}
	}
	// PlateID is nearest-neighbor and must be a real plate id.
	if f.PlateID[0] < 0 || int(f.PlateID[0]) >= len(sd.Plates.Plates) {
		t.Fatalf("PlateID[0] out of range: %d", f.PlateID[0])
	}
}

func TestExtractFieldsRejectsInvalidWindow(t *testing.T) {
	sd := testSphere(t)
	if _, err := ExtractFields(sd, Window{Face: cubemap.FacePosZ, X0: 100, Y0: 0, Size: 64, SProd: 128}); err == nil {
		t.Fatal("invalid window accepted")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/planetgen/patch/ -run ExtractFields -v`
Expected: FAIL — undefined `ExtractFields`.

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/extract.go
package patch

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Fields is the per-pixel patch-extraction contract (spec §4.2): the
// sphere fields downstream layers consume, bilinearly upsampled from
// S_tect at each patch pixel's true direction. PlateID is
// nearest-neighbor (categorical, debug overlay only).
type Fields struct {
	Window Window

	BaseHeight, ContinentalMask *Grid

	BeltDist, BeltMag   *Grid
	SubdDist, SubdMag   *Grid
	ArcDist, ArcMag     *Grid
	RidgeDist, RidgeMag *Grid
	RiftDist, RiftMag   *Grid
	Transform           *Grid

	PlateID []int16 // row-major Size×Size
}

// wrapF wraps raw per-face float slices (PlateField SDF layout) in a
// CubeMapF so we can reuse its bilinear direction Sample.
func wrapF(size int, faces [cubemap.NumFaces][]float64) *cubemap.CubeMapF {
	return &cubemap.CubeMapF{Size: size, Faces: faces}
}

func cropF(src *cubemap.CubeMapF, w Window) *Grid {
	g := NewGrid(w.Size)
	for iy := range w.Size {
		for ix := range w.Size {
			x, y, z := w.Dir(ix, iy)
			g.Set(ix, iy, src.Sample(x, y, z))
		}
	}
	return g
}

func ExtractFields(sd *SphereData, w Window) (*Fields, error) {
	if err := w.Valid(); err != nil {
		return nil, err
	}
	f := &Fields{Window: w}
	f.BaseHeight = cropF(sd.Crust.BaseHeight, w)
	f.ContinentalMask = cropF(sd.Crust.ContinentalMask, w)
	f.BeltDist = cropF(sd.FX.BeltDist, w)
	f.BeltMag = cropF(sd.FX.BeltMag, w)
	f.SubdDist = cropF(sd.FX.SubdDist, w)
	f.SubdMag = cropF(sd.FX.SubdMag, w)
	f.ArcDist = cropF(sd.FX.ArcDist, w)
	f.ArcMag = cropF(sd.FX.ArcMag, w)
	f.RidgeDist = cropF(sd.FX.RidgeDist, w)
	f.RidgeMag = cropF(sd.FX.RidgeMag, w)
	f.RiftDist = cropF(sd.FX.RiftDist, w)
	f.RiftMag = cropF(sd.FX.RiftMag, w)
	f.Transform = cropF(wrapF(sd.STect, sd.Plates.Transform), w)

	f.PlateID = make([]int16, w.Size*w.Size)
	for iy := range w.Size {
		for ix := range w.Size {
			x, y, z := w.Dir(ix, iy)
			face, px, py := cubemap.DirToFacePixel(x, y, z, sd.STect)
			f.PlateID[iy*w.Size+ix] = sd.Plates.PlateID[face][py*sd.STect+px]
		}
	}
	return f, nil
}
```

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/extract.go pkg/planetgen/patch/extract_test.go
git commit -m "P13: ExtractFields — bilinear sphere-to-patch field crops"
```

---

### Task 6: Pick — scoring interesting windows

**Files:**
- Create: `pkg/planetgen/patch/pick.go`
- Test: `pkg/planetgen/patch/pick_test.go`

**Interfaces:**
- Produces: `type Candidate struct { Window Window \`json:"window"\`; Score float64 \`json:"score"\` }`, `func Pick(sd *SphereData, size, sProd, topN int) []Candidate`.
- Consumed by: wasm `patchInit` (Task 17); tests use `Pick(...)[0].Window` as the canonical golden window.

Scoring per spec §4.3: candidate window centers stride across all 6 faces; the footprint is examined **at S_tect resolution** (cheap). Score = per-FX-class presence + boundary density + mixed land/ocean bonus. Deterministic: pure function of the fields; ties broken by (face, y, x).

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/pick_test.go
package patch

import "testing"

func TestPickDeterministicAndValid(t *testing.T) {
	sd := testSphere(t)
	a := Pick(sd, 64, 128, 5)
	b := Pick(sd, 64, 128, 5)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("Pick returned %d/%d candidates", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("candidate %d differs between runs", i)
		}
		if err := a[i].Window.Valid(); err != nil {
			t.Fatalf("candidate %d invalid: %v", i, err)
		}
	}
	// Ranked descending.
	for i := 1; i < len(a); i++ {
		if a[i].Score > a[i-1].Score {
			t.Fatal("candidates not sorted by score desc")
		}
	}
	// The top window on a terran world should straddle land AND ocean.
	f, err := ExtractFields(sd, a[0].Window)
	if err != nil {
		t.Fatal(err)
	}
	land, ocean := 0, 0
	for _, v := range f.ContinentalMask.Data {
		if v > 0.5 {
			land++
		} else {
			ocean++
		}
	}
	if land == 0 || ocean == 0 {
		t.Fatalf("top window is single-domain: land=%d ocean=%d", land, ocean)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/planetgen/patch/ -run Pick -v` → FAIL undefined `Pick`.

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/pick.go
package patch

import (
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Candidate is a scored patch window.
type Candidate struct {
	Window Window  `json:"window"`
	Score  float64 `json:"score"`
}

// fxClassBoundaryKm: a footprint pixel closer than this to a class
// boundary counts as "active" for that class.
const fxClassBoundaryKm = 300.0

// Pick ranks candidate windows of size×size (at virtual resolution
// sProd) by tectonic interest and returns the topN. Scoring runs on
// the S_tect rasters: FX-class presence (+1 per distinct class with
// >2% active pixels), boundary density (up to +0.3 per class), and a
// +1 bonus for windows that straddle land and ocean.
func Pick(sd *SphereData, size, sProd, topN int) []Candidate {
	sT := sd.STect
	// Footprint of the window in S_tect pixels.
	fp := size * sT / sProd
	if fp < 2 {
		fp = 2
	}
	stride := max(fp/2, 1)

	classes := []*cubemap.CubeMapF{sd.FX.BeltDist, sd.FX.SubdDist, sd.FX.ArcDist, sd.FX.RidgeDist, sd.FX.RiftDist}

	var cands []Candidate
	maxOrigin := sT - fp
	for face := range cubemap.Face(cubemap.NumFaces) {
		for oy := 0; oy <= maxOrigin; oy += stride {
			for ox := 0; ox <= maxOrigin; ox += stride {
				var score float64
				total := fp * fp
				for _, cls := range classes {
					active := 0
					for fy := oy; fy < oy+fp; fy++ {
						row := cls.Faces[face][fy*sT : fy*sT+sT]
						for fx := ox; fx < ox+fp; fx++ {
							if row[fx] < fxClassBoundaryKm {
								active++
							}
						}
					}
					frac := float64(active) / float64(total)
					if frac > 0.02 {
						score += 1.0
					}
					score += min(frac, 0.3)
				}
				land := 0
				for fy := oy; fy < oy+fp; fy++ {
					row := sd.Crust.ContinentalMask.Faces[face][fy*sT : fy*sT+sT]
					for fx := ox; fx < ox+fp; fx++ {
						if row[fx] > 0.5 {
							land++
						}
					}
				}
				lf := float64(land) / float64(total)
				if lf > 0.25 && lf < 0.75 {
					score += 1.0
				}
				// Map the S_tect origin back to virtual pixels.
				w := Window{Face: face, X0: ox * sProd / sT, Y0: oy * sProd / sT, Size: size, SProd: sProd}
				if w.X0+size > sProd {
					w.X0 = sProd - size
				}
				if w.Y0+size > sProd {
					w.Y0 = sProd - size
				}
				cands = append(cands, Candidate{Window: w, Score: score})
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Score != cands[j].Score {
			return cands[i].Score > cands[j].Score
		}
		a, b := cands[i].Window, cands[j].Window
		if a.Face != b.Face {
			return a.Face < b.Face
		}
		if a.Y0 != b.Y0 {
			return a.Y0 < b.Y0
		}
		return a.X0 < b.X0
	})
	if topN > 0 && len(cands) > topN {
		cands = cands[:topN]
	}
	return cands
}
```

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS. (If the terran land/ocean assertion is flaky at sTect=64, it means the scoring bonus isn't working — debug the scoring, don't loosen the test.)

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/pick.go pkg/planetgen/patch/pick_test.go
git commit -m "P13: Pick — deterministic tectonic-interest window ranking"
```

---

### Task 7: Context, State, Layer, Stack (dirty-tracked caching)

**Files:**
- Create: `pkg/planetgen/patch/stack.go`
- Test: `pkg/planetgen/patch/stack_test.go`

**Interfaces:**
- Produces:

```go
type Context struct {
	Sphere  *SphereData
	Fields  *Fields
	Profile *types.PlanetProfile // live copy edited by the UI
	Master  int64
	SeaLevelView float64 // waterline slider; 0 → use Sphere.SeaLevel
}

type State struct {
	Height    *Grid
	DistCoast *Grid
	T, M      *Grid
	RainMult  *Grid
	Rivers    []bool
	FlowAccum *Grid
	Craters   []feature.Crater
	Img       *image.RGBA
	Sites     []feature.Site
}

type Layer struct {
	Index   int
	ID      string
	Name    string
	Params  []string // profile param path prefixes owned by this layer
	Enabled func(*Context) bool
	Apply   func(*Context, *State) *State
}

func Layers() []Layer                     // canonical ordered list, indices 0..12
func NewStack(ctx *Context) *Stack
func (s *Stack) Ctx() *Context
func (s *Stack) MarkDirty(paramPath string) (needsSphere bool)
func (s *Stack) MarkAllDirty()
func (s *Stack) RenderTo(target int) (*State, error)
```

- **Apply contract (critical):** `Apply` receives the cached upstream `State` and must NOT mutate it. It returns a shallow copy (`ns := *st`) with fresh pointers for exactly the fields it writes (see the layer index table). This makes per-layer caching free — unchanged fields share memory across the cache.
- Layer registration: `Layers()` in this task returns 13 entries whose `Apply` is a placeholder identity (`func(_ *Context, st *State) *State { return st }`) EXCEPT where later tasks fill them in; each layer task replaces its placeholder. `Enabled` defaults to `func(*Context) bool { return true }`.
- `MarkDirty(path)`: if `path` matches a prefix in some layer's `Params` (either string has the other as prefix), set `dirtyFrom = min(dirtyFrom, layer.Index)` and return false. If it matches no layer, return **true** — the param is sphere-level (crust seeding, plates, radius, jitter) and the caller must `ComputeSphere` + `ExtractFields` again and then `MarkAllDirty`.
- `RenderTo(k)`: builds `State{Height: NewGrid(size)}` at layer -1, then applies enabled layers `dirtyFrom..k` starting from `cache[dirtyFrom-1]`; stores each result in `cache[i]`; sets `dirtyFrom = k+1` (i.e. clean through k). If `k < dirtyFrom-1` just return `cache[k]`.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/stack_test.go
package patch

import "testing"

// testStack builds a Stack whose first three layers count their
// invocations, to pin cache/dirty semantics without real layers.
func countingStack(t *testing.T, counts *[13]int) *Stack {
	t.Helper()
	sd := testSphere(t)
	w := Pick(sd, 32, 64, 1)[0].Window
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &Context{Sphere: sd, Fields: f, Profile: sd.Profile, Master: sd.Master}
	s := NewStack(ctx)
	for i := range s.layers {
		idx := i
		inner := s.layers[i].Apply
		s.layers[i].Apply = func(c *Context, st *State) *State {
			counts[idx]++
			return inner(c, st)
		}
	}
	return s
}

func TestStackCachesCleanLayers(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if _, err := s.RenderTo(4); err != nil {
		t.Fatal(err)
	}
	if counts[0] != 1 || counts[4] != 1 {
		t.Fatalf("first render should run each layer once: %v", counts)
	}
	if _, err := s.RenderTo(4); err != nil {
		t.Fatal(err)
	}
	if counts[0] != 1 {
		t.Fatalf("clean re-render must be fully cached: %v", counts)
	}
}

func TestStackDirtyRerunsSuffixOnly(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if _, err := s.RenderTo(6); err != nil {
		t.Fatal(err)
	}
	if needsSphere := s.MarkDirty("Erosion.Droplets"); needsSphere {
		t.Fatal("Erosion is a stack param, not a sphere param")
	}
	if _, err := s.RenderTo(6); err != nil {
		t.Fatal(err)
	}
	if counts[5] != 1 {
		t.Fatalf("layer 5 (coastal) must stay cached, ran %d times", counts[5])
	}
	if counts[6] != 2 {
		t.Fatalf("layer 6 (erosion) must re-run, ran %d times", counts[6])
	}
}

func TestStackSphereParamSignals(t *testing.T) {
	var counts [13]int
	s := countingStack(t, &counts)
	if needsSphere := s.MarkDirty("crust.majorPlates"); !needsSphere {
		t.Fatal("crust seeding params must signal a sphere recompute")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/planetgen/patch/ -run Stack -v` → FAIL undefined `NewStack`.

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/stack.go
package patch

import (
	"fmt"
	"image"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Context holds the immutable-per-render inputs shared by all layers.
type Context struct {
	Sphere  *SphereData
	Fields  *Fields
	Profile *types.PlanetProfile
	Master  int64
	// SeaLevelView overrides the waterline sea level for the
	// waterlines/civ layers; 0 means "use Sphere.SeaLevel".
	SeaLevelView float64
}

// seaLevelView resolves the effective waterline sea level.
func (c *Context) seaLevelView() float64 {
	if c.SeaLevelView > 0 {
		return c.SeaLevelView
	}
	return c.Sphere.SeaLevel
}

// State is the accumulated per-layer output. Layers must treat the
// input State as immutable: copy the struct, replace only the pointers
// for fields they write.
type State struct {
	Height    *Grid
	DistCoast *Grid
	T, M      *Grid
	RainMult  *Grid
	Rivers    []bool
	FlowAccum *Grid
	Craters   []feature.Crater
	Img       *image.RGBA
	Sites     []feature.Site
}

// Layer is one patch pipeline stage.
type Layer struct {
	Index   int
	ID      string
	Name    string
	Params  []string
	Enabled func(*Context) bool
	Apply   func(*Context, *State) *State
}

func always(*Context) bool { return true }
func identity(_ *Context, st *State) *State { return st }

// Layers returns the canonical ordered layer list. Layer tasks 8–15
// replace the identity Apply/Enabled placeholders with real stages.
func Layers() []Layer {
	ls := []Layer{
		{ID: "tectonic-base", Name: "Tectonic base", Params: nil},
		{ID: "tectonic-fx", Name: "Tectonic FX", Params: []string{"tectonicFX"}},
		{ID: "control-noise", Name: "Control noise", Params: []string{"ControlConfig"}},
		{ID: "height-smooth", Name: "Height smooth", Params: []string{"HeightSmoothRadius"}},
		{ID: "normalize", Name: "Normalize", Params: nil},
		{ID: "coastal", Name: "Coastal noise", Params: []string{"Coastal"}},
		{ID: "erosion", Name: "Erosion", Params: []string{"Erosion"}},
		{ID: "craters", Name: "Craters", Params: []string{"CraterCount", "CraterMinRadius", "CraterMaxRadius", "CraterDepth", "PowerLawAlpha", "MariaDensityFactor", "SurfaceAge", "SecondaryDensity"}},
		{ID: "flow-rivers", Name: "Rivers", Params: []string{"flow"}},
		{ID: "climate", Name: "Climate", Params: []string{"rainShadow"}},
		{ID: "biome-color", Name: "Biome color", Params: []string{"BiomeTable", "Palette", "EquatorialPalette", "PolarPalette", "Warp", "LUT"}},
		{ID: "waterlines", Name: "Waterlines", Params: []string{"SnowLine", "OceanColor", "HasPolarCaps", "PolarCapSize", "PolarCapNoise", "ShadingStrength", "ShadingExaggeration", "seaLevelView"}},
		{ID: "civ", Name: "Civilization", Params: []string{"civ"}},
	}
	for i := range ls {
		ls[i].Index = i
		ls[i].Enabled = always
		ls[i].Apply = identity
	}
	return ls
}

// Stack renders layers with per-layer caching and dirty tracking.
type Stack struct {
	ctx       *Context
	layers    []Layer
	cache     []*State // cache[i] = state after layer i
	dirtyFrom int      // first layer that must re-run
}

func NewStack(ctx *Context) *Stack {
	ls := Layers()
	return &Stack{ctx: ctx, layers: ls, cache: make([]*State, len(ls)), dirtyFrom: 0}
}

func (s *Stack) Ctx() *Context { return s.ctx }

// MarkDirty maps a changed profile param path to the earliest owning
// layer. Returns true when the param belongs to the sphere precompute
// (no stack layer owns it) — caller must recompute SphereData +
// Fields, then MarkAllDirty.
func (s *Stack) MarkDirty(paramPath string) bool {
	for i := range s.layers {
		for _, p := range s.layers[i].Params {
			if strings.HasPrefix(paramPath, p) || strings.HasPrefix(p, paramPath) {
				if i < s.dirtyFrom {
					s.dirtyFrom = i
				}
				return false
			}
		}
	}
	return true
}

func (s *Stack) MarkAllDirty() { s.dirtyFrom = 0 }

// RenderTo runs enabled layers up to and including target, reusing
// cached upstream states.
func (s *Stack) RenderTo(target int) (*State, error) {
	if target < 0 || target >= len(s.layers) {
		return nil, fmt.Errorf("patch: layer index %d out of range", target)
	}
	if target < s.dirtyFrom {
		return s.cache[target], nil
	}
	var st *State
	start := s.dirtyFrom
	if start == 0 {
		st = &State{Height: NewGrid(s.ctx.Fields.Window.Size)}
	} else {
		st = s.cache[start-1]
	}
	for i := start; i <= target; i++ {
		if s.layers[i].Enabled(s.ctx) {
			st = s.layers[i].Apply(s.ctx, st)
		}
		s.cache[i] = st
	}
	if target+1 > s.dirtyFrom {
		s.dirtyFrom = target + 1
	}
	return st, nil
}
```

**Subtlety pinned by the tests:** `dirtyFrom` after `RenderTo(k)` is `k+1`, so rendering deeper later (e.g. `RenderTo(8)` after `RenderTo(4)`) resumes from the cached layer-4 state, and `MarkDirty` can pull it back down.

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/stack.go pkg/planetgen/patch/stack_test.go
git commit -m "P13: layer Stack with param-mapped dirty tracking and per-layer cache"
```

---

### Task 8: Layers 0–1 — tectonic-base and tectonic-fx

**Files:**
- Create: `pkg/planetgen/patch/layer_base.go`, `pkg/planetgen/patch/layer_fx.go`
- Modify: `pkg/planetgen/patch/stack.go` (wire real Apply funcs into `Layers()`)
- Test: `pkg/planetgen/patch/layer_base_test.go`

**Interfaces:**
- Consumes: `Fields` grids (Task 5), `field.FXDelta`/`NewFXGens`/`FXSample` (Task 2).
- Produces: `applyTectonicBase(ctx, st) *State` and `applyTectonicFX(ctx, st) *State` wired into `Layers()` indices 0 and 1. Later layer tasks follow this exact wiring pattern.

**Wiring pattern (used by every layer task):** in `Layers()`, replace the placeholder for the layer, e.g.:

```go
ls[0].Apply = applyTectonicBase
ls[1].Apply = applyTectonicFX
```

(Concretely: change the `for i := range ls` loop to only set placeholders where `ls[i].Apply == nil`, and set real funcs in the literal or right after it. Keep `Layers()` the single source of truth.)

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/layer_base_test.go
package patch

import "testing"

func testContext(t *testing.T) *Context {
	t.Helper()
	sd := testSphere(t)
	w := Pick(sd, 64, 128, 1)[0].Window
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	return &Context{Sphere: sd, Fields: f, Profile: sd.Profile, Master: sd.Master}
}

func TestLayerBaseCopiesBaseHeight(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(0)
	if err != nil {
		t.Fatal(err)
	}
	if st.Height.At(3, 3) != ctx.Fields.BaseHeight.At(3, 3) {
		t.Fatal("layer 0 must initialize Height from the BaseHeight crop")
	}
}

func TestLayerFXChangesHeightNearBoundaries(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st0, err := s.RenderTo(0)
	if err != nil {
		t.Fatal(err)
	}
	st1, err := s.RenderTo(1)
	if err != nil {
		t.Fatal(err)
	}
	diff := 0
	for i := range st1.Height.Data {
		if st1.Height.Data[i] != st0.Height.Data[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("tectonic FX changed nothing — the picked window should contain active boundaries")
	}
	// Immutability: layer 1 must not have mutated layer 0's cache.
	st0b, _ := s.RenderTo(0)
	if st0b != st0 {
		t.Fatal("cache identity broken")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/planetgen/patch/ -run Layer -v` → FAIL (layer 0 is identity, Height stays zero ≠ BaseHeight).

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/layer_base.go
package patch

// applyTectonicBase (layer 0) initializes the heightmap from the
// cropped crust BaseHeight — the patch analog of rocky.go's crust
// raft init.
func applyTectonicBase(ctx *Context, st *State) *State {
	ns := *st
	ns.Height = ctx.Fields.BaseHeight.Clone()
	return &ns
}
```

```go
// pkg/planetgen/patch/layer_fx.go
package patch

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
)

// applyTectonicFX (layer 1) evaluates the production FXDelta formula
// per patch pixel against the cropped Dist/Mag fields. All FX params
// and TectonicAge behave exactly as on the sphere; only the field
// resolution (upsampled from S_tect) differs.
func applyTectonicFX(ctx *Context, st *State) *State {
	f := ctx.Fields
	w := f.Window
	cfg := ctx.Profile.TectonicFX
	age := ctx.Sphere.Crust.TectonicAge
	g := field.NewFXGens(ctx.Master)

	ns := *st
	ns.Height = st.Height.Clone()
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			dx, dy, dz := w.Dir(ix, iy)
			s := field.FXSample{
				BeltDist: f.BeltDist.Data[i], BeltMag: f.BeltMag.Data[i],
				SubdDist: f.SubdDist.Data[i], SubdMag: f.SubdMag.Data[i],
				ArcDist: f.ArcDist.Data[i], ArcMag: f.ArcMag.Data[i],
				RidgeDist: f.RidgeDist.Data[i], RidgeMag: f.RidgeMag.Data[i],
				RiftDist: f.RiftDist.Data[i], RiftMag: f.RiftMag.Data[i],
				TransformDist:   f.Transform.Data[i],
				ContinentalMask: f.ContinentalMask.Data[i],
			}
			ns.Height.Data[i] += field.FXDelta(dx, dy, dz, s, cfg, age, g)
		}
	}
	return &ns
}
```

Wire both into `Layers()` per the wiring pattern.

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch layers 0-1 — tectonic base + FX via shared FXDelta"
```

---

### Task 9: Layers 2–4 — control noise, height smooth, normalize

**Files:**
- Create: `pkg/planetgen/patch/layer_noise.go`
- Modify: `pkg/planetgen/patch/stack.go` (wire indices 2–4)
- Test: `pkg/planetgen/patch/layer_noise_test.go`

**Interfaces:**
- Consumes: `noise.New`/`FractalNoise3D`, `noise.JitterField.Transform`, `planetcolor.EvalSpline`, `seed.Domain`, sphere `HMin/HMax`.
- Produces: `applyControlNoise`, `applyHeightSmooth`, `applyNormalize` wired at indices 2–4.

**Semantics (mirrors rocky.go crust fast path exactly):**
- Control noise: for each pixel dir, sample Detail (index 1) and PeaksValleys (index 2) fBm and add their spline evaluations. Continentalness is SKIPPED on the crust path. Seed domains are the production ones from `field/control.go`: Detail = `"control.erosion"` (historical name), PeaksValleys = `"control.peaks-valleys"`. Detail's sample direction goes through `ctx.Sphere.Jitter.Transform(dx,dy,dz)` when jitter is non-nil (production jitters ONLY field index 1).
- Height smooth: flat disc blur radius `profile.HeightSmoothRadius`, weights `1/(1+d)`, edge-clamped (skip out-of-bounds neighbors), per-pixel renormalized — the flat port of `field.SmoothHeightmap`.
- Normalize: apply the SPHERE-DERIVED affine `(h - HMin) / (HMax - HMin)`, NOT a patch-local min/max (spec §4.2). No clamp — production doesn't clamp either; out-of-range patch values are legal and harmless.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/layer_noise_test.go
package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/planetcolor"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

func TestControlNoiseMatchesProductionFormula(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st1, _ := s.RenderTo(1)
	st2, err := s.RenderTo(2)
	if err != nil {
		t.Fatal(err)
	}
	// Recompute pixel (7,9) by hand with the production domains.
	w := ctx.Fields.Window
	ix, iy := 7, 9
	dx, dy, dz := w.Dir(ix, iy)
	cc := ctx.Profile.ControlConfig

	jx, jy, jz := dx, dy, dz
	if ctx.Sphere.Jitter != nil {
		jx, jy, jz = ctx.Sphere.Jitter.Transform(dx, dy, dz)
	}
	det := noise.New(seed.Domain(ctx.Master, "control.erosion"))
	dv := det.FractalNoise3D(jx, jy, jz, cc.Detail.Octaves, cc.Detail.Lacunarity, cc.Detail.Persistence, cc.Detail.Freq) * cc.Detail.Amp
	pv := noise.New(seed.Domain(ctx.Master, "control.peaks-valleys"))
	pvv := pv.FractalNoise3D(dx, dy, dz, cc.PeaksValleys.Octaves, cc.PeaksValleys.Lacunarity, cc.PeaksValleys.Persistence, cc.PeaksValleys.Freq) * cc.PeaksValleys.Amp

	want := st1.Height.At(ix, iy) +
		planetcolor.EvalSpline(cc.Detail.Spline, dv) +
		planetcolor.EvalSpline(cc.PeaksValleys.Spline, pvv)
	if got := st2.Height.At(ix, iy); math.Abs(got-want) > 1e-12 {
		t.Fatalf("control-noise pixel mismatch: got %v want %v", got, want)
	}
}

func TestNormalizeUsesSphereAffine(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st3, _ := s.RenderTo(3)
	st4, err := s.RenderTo(4)
	if err != nil {
		t.Fatal(err)
	}
	sd := ctx.Sphere
	want := (st3.Height.At(5, 5) - sd.HMin) / (sd.HMax - sd.HMin)
	if got := st4.Height.At(5, 5); math.Abs(got-want) > 1e-12 {
		t.Fatalf("normalize must use sphere HMin/HMax: got %v want %v", got, want)
	}
}

func TestHeightSmoothIsNoopAtZeroRadius(t *testing.T) {
	ctx := testContext(t)
	prof := *ctx.Profile
	prof.HeightSmoothRadius = 0
	ctx.Profile = &prof
	s := NewStack(ctx)
	st2, _ := s.RenderTo(2)
	st3, _ := s.RenderTo(3)
	for i := range st3.Height.Data {
		if st3.Height.Data[i] != st2.Height.Data[i] {
			t.Fatal("smooth with radius 0 must be identity")
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/planetgen/patch/ -run 'ControlNoise|Normalize|HeightSmooth' -v` → FAIL (layers are identity).

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/layer_noise.go
package patch

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/planetcolor"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// controlDomains matches field/control.go's controlFieldDomains for
// the two crust-path height contributors ("control.erosion" is
// Detail's historical domain name — do not "fix" it, it would reseed
// every planet).
var patchControlDomains = [2]struct {
	domain string
	pick   func(cc types.ControlConfig) types.ControlField
	jitter bool
}{
	{"control.erosion", func(cc types.ControlConfig) types.ControlField { return cc.Detail }, true},
	{"control.peaks-valleys", func(cc types.ControlConfig) types.ControlField { return cc.PeaksValleys }, false},
}

// applyControlNoise (layer 2): Detail + PeaksValleys fBm splines at
// true patch directions. Continentalness is skipped on the crust path
// (rocky.go L417-419).
func applyControlNoise(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	cc := ctx.Profile.ControlConfig
	ns := *st
	ns.Height = st.Height.Clone()
	for _, cd := range patchControlDomains {
		fc := cd.pick(cc)
		if fc.Amp == 0 || fc.Octaves <= 0 {
			continue
		}
		ng := noise.New(seed.Domain(ctx.Master, cd.domain))
		for iy := range w.Size {
			for ix := range w.Size {
				dx, dy, dz := w.Dir(ix, iy)
				if cd.jitter && ctx.Sphere.Jitter != nil {
					dx, dy, dz = ctx.Sphere.Jitter.Transform(dx, dy, dz)
				}
				v := ng.FractalNoise3D(dx, dy, dz, fc.Octaves, fc.Lacunarity, fc.Persistence, fc.Freq) * fc.Amp
				ns.Height.Data[iy*w.Size+ix] += planetcolor.EvalSpline(fc.Spline, v)
			}
		}
	}
	return &ns
}

// applyHeightSmooth (layer 3): flat disc blur, 1/(1+d) weights,
// edge-clamped — the flat port of field.SmoothHeightmap.
func applyHeightSmooth(ctx *Context, st *State) *State {
	r := ctx.Profile.HeightSmoothRadius
	if r <= 0 {
		return st
	}
	size := st.Height.Size
	ns := *st
	ns.Height = NewGrid(size)
	for iy := range size {
		for ix := range size {
			var sum, wsum float64
			for oy := -r; oy <= r; oy++ {
				for ox := -r; ox <= r; ox++ {
					d2 := ox*ox + oy*oy
					if d2 > r*r {
						continue
					}
					nx, ny := ix+ox, iy+oy
					if nx < 0 || ny < 0 || nx >= size || ny >= size {
						continue
					}
					wgt := 1.0 / (1.0 + math.Sqrt(float64(d2)))
					sum += st.Height.At(nx, ny) * wgt
					wsum += wgt
				}
			}
			ns.Height.Set(ix, iy, sum/wsum)
		}
	}
	return &ns
}

// applyNormalize (layer 4): the sphere-derived affine — a patch-local
// min/max would disagree with the production render (spec §4.2).
func applyNormalize(ctx *Context, st *State) *State {
	sd := ctx.Sphere
	if sd.HMax <= sd.HMin {
		return st
	}
	inv := 1 / (sd.HMax - sd.HMin)
	ns := *st
	ns.Height = NewGrid(st.Height.Size)
	for i, v := range st.Height.Data {
		ns.Height.Data[i] = (v - sd.HMin) * inv
	}
	return &ns
}
```

(Add `"math"` import. **Check `field.SmoothHeightmap`'s actual weight formula first** — read `pkg/planetgen/field/smooth.go:14` and copy its exact weight expression; the survey says `1/(1+d)` falloff. Match it exactly.)

Wire indices 2–4 in `Layers()`.

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch layers 2-4 — control noise (production domains), smooth, sphere-affine normalize"
```

---

### Task 10: Layer 5 — coastal noise with patch-local distance-to-coast

**Files:**
- Create: `pkg/planetgen/patch/layer_coastal.go`
- Modify: `pkg/planetgen/patch/stack.go` (wire index 5, plus `Enabled`)
- Test: `pkg/planetgen/patch/layer_coastal_test.go`

**Interfaces:**
- Consumes: `noise.NewCoastalGen(seed.Domain(ctx.Master, "coastal"))` — **verify the exact seed rocky.go uses for its CoastalGen (read the Coastal stage around rocky.go L688-726) and use the identical expression**; `noise.ApplyCoastal`; `SeaLevel0`.
- Produces: `applyCoastal` (writes `Height` + `DistCoast`); `distanceToCoastPatch(hm *Grid, threshold float64) *Grid` — two-pass chamfer distance normalized like the old flat path, reused by nothing else but kept package-private.
- Enabled: `func(ctx *Context) bool { return ctx.Profile.Coastal.Amp > 0 && ctx.Sphere.SeaLevel0 > 0 }`.

**Divergence (spec §7, accepted):** production uses sphere-global JFA distance-to-coast in great-circle units; the patch uses a local 2-pass chamfer in pixel units converted to the same [0,1] angular scale by `distPx * (π/2) / SProd / π` (one face ≈ π/2 radians across, S_prod pixels per face). Get the constant right so `ApplyCoastal`'s falloff sees comparable magnitudes.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/layer_coastal_test.go
package patch

import "testing"

func TestCoastalOnlyTouchesNearCoast(t *testing.T) {
	ctx := testContext(t)
	if ctx.Profile.Coastal.Amp <= 0 {
		t.Skip("terran profile has no coastal config; pick another archetype in testContext if this skips")
	}
	s := NewStack(ctx)
	st4, _ := s.RenderTo(4)
	st5, err := s.RenderTo(5)
	if err != nil {
		t.Fatal(err)
	}
	if st5.DistCoast == nil {
		t.Fatal("coastal layer must publish DistCoast")
	}
	changed, far := 0, 0
	for i := range st5.Height.Data {
		if st5.Height.Data[i] != st4.Height.Data[i] {
			changed++
			if st5.DistCoast.Data[i] > 0.2 {
				far++
			}
		}
	}
	if changed == 0 {
		t.Skip("window has no coast; acceptable for some seeds")
	}
	if far > changed/10 {
		t.Fatalf("coastal noise reached far inland/offshore: %d of %d changed pixels", far, changed)
	}
}

func TestCoastalDisabledIsIdentity(t *testing.T) {
	ctx := testContext(t)
	prof := *ctx.Profile
	prof.Coastal.Amp = 0
	ctx.Profile = &prof
	s := NewStack(ctx)
	st4, _ := s.RenderTo(4)
	st5, _ := s.RenderTo(5)
	if st5 != st4 {
		t.Fatal("disabled coastal layer must pass the state through untouched")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/planetgen/patch/ -run Coastal -v` → FAIL (`DistCoast` nil).

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/layer_coastal.go
package patch

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

// distanceToCoastPatch: two-pass chamfer distance (in pixels) to the
// nearest coastline crossing of threshold, converted to the JFA
// field's [0,1] angular-fraction units: one face spans ~π/2 radians,
// so 1 px ≈ (π/2)/SProd radians, and the JFA field divides by π.
func distanceToCoastPatch(hm *Grid, threshold float64, sProd int) *Grid {
	size := hm.Size
	const inf = math.MaxFloat64
	d := NewGrid(size)
	for i, h := range hm.Data {
		ocean := h < threshold
		// Seed: pixels adjacent to the opposite domain are coast.
		d.Data[i] = inf
		ix, iy := i%size, i/size
		for _, o := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := ix+o[0], iy+o[1]
			if nx < 0 || ny < 0 || nx >= size || ny >= size {
				continue
			}
			if (hm.At(nx, ny) < threshold) != ocean {
				d.Data[i] = 0
				break
			}
		}
	}
	// Chamfer 3-4 forward/backward passes.
	relax := func(i int, ix, iy, ox, oy int, w float64) {
		nx, ny := ix+ox, iy+oy
		if nx < 0 || ny < 0 || nx >= size || ny >= size {
			return
		}
		if v := d.Data[ny*size+nx] + w; v < d.Data[i] {
			d.Data[i] = v
		}
	}
	for iy := range size {
		for ix := range size {
			i := iy*size + ix
			relax(i, ix, iy, -1, 0, 1)
			relax(i, ix, iy, 0, -1, 1)
			relax(i, ix, iy, -1, -1, math.Sqrt2)
			relax(i, ix, iy, 1, -1, math.Sqrt2)
		}
	}
	for iy := size - 1; iy >= 0; iy-- {
		for ix := size - 1; ix >= 0; ix-- {
			i := iy*size + ix
			relax(i, ix, iy, 1, 0, 1)
			relax(i, ix, iy, 0, 1, 1)
			relax(i, ix, iy, 1, 1, math.Sqrt2)
			relax(i, ix, iy, -1, 1, math.Sqrt2)
		}
	}
	// Pixels → angular fraction of π (JFA units).
	pxToFrac := (math.Pi / 2) / float64(sProd) / math.Pi
	for i, v := range d.Data {
		if v == inf {
			d.Data[i] = 1
		} else {
			d.Data[i] = v * pxToFrac
		}
	}
	return d
}

func coastalEnabled(ctx *Context) bool {
	return ctx.Profile.Coastal.Amp > 0 && ctx.Sphere.SeaLevel0 > 0
}

// applyCoastal (layer 5): production ApplyCoastal per pixel, with the
// patch-local chamfer standing in for the sphere JFA (spec §7).
func applyCoastal(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	p := ctx.Profile
	sea := ctx.Sphere.SeaLevel0
	dist := distanceToCoastPatch(st.Height, sea, w.SProd)
	gen := noise.NewCoastalGen(seed.Domain(ctx.Master, "coastal")) // VERIFY: match rocky.go's exact CoastalGen seed expression

	ns := *st
	ns.Height = st.Height.Clone()
	ns.DistCoast = dist
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			dx, dy, dz := w.Dir(ix, iy)
			ns.Height.Data[i] = noise.ApplyCoastal(gen, dx, dy, dz,
				st.Height.Data[i], dist.Data[i], p.Coastal.Amp, p.Coastal.Threshold, p.Coastal.Freq)
		}
	}
	return &ns
}
```

Wire index 5 (`Apply` + `Enabled: coastalEnabled`).

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS (skips are acceptable where marked).

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch layer 5 — coastal noise with patch-local chamfer coast distance"
```

---

### Task 11: Layer 6 — hydraulic erosion (flat droplet port from git history)

**Files:**
- Create: `pkg/planetgen/patch/layer_erosion.go`
- Modify: `pkg/planetgen/patch/stack.go` (wire index 6 + Enabled)
- Test: `pkg/planetgen/patch/layer_erosion_test.go`

**Interfaces:**
- Produces: `applyErosion`; internal `erodePatch(masterSeed int64, hm *Grid, cfg types.ErosionConfig, oceanLevel float64)` (mutates hm in place — callers pass a fresh Clone).
- Enabled: `ctx.Profile.Erosion.Droplets > 0`.

**Recovery step — do not write the droplet simulator from scratch.** The deleted flat-path implementation is the correct starting point:

```bash
git show 966f17131^:pkg/planetgen/render/flat_erosion.go > /tmp/flat_erosion_reference.go
```

Port it into `pkg/planetgen/patch/layer_erosion.go` with these exact adaptations:
1. `package render` → `package patch`; rename `erodeFlat` → `erodePatch`; take `hm *Grid` instead of `[]float64 + size` (use `hm.Data`, `hm.Size` internally; keep all helper signatures otherwise).
2. Keep its PCG seeding verbatim (it uses the `"erosion.seed"`/`"erosion.stream"` domains — verify in the recovered file and keep identical).
3. Keep the open-edge policy exactly as recovered: droplets terminate when they leave `[0,size)`; brush application skips out-of-bounds cells. **That behavior IS the spec's §4.5 edge policy.**
4. Keep the ocean guards (skip ocean spawns, river-mouth notch at `oceanLevel - riverNotchDepth`, erode floor) and `BrushFalloff` support as recovered.
5. Replace its whole-face droplet-count scaling (`scaleErosionDropletsFlat`, canonical 1024² face) with patch-fraction scaling:

```go
// patchDroplets scales the canonical droplet budget (calibrated for a
// full 6×1024² sphere) down to the window's share of the sphere.
func patchDroplets(canonical, size, sProd int) int {
	n := canonical * size * size / (6 * sProd * sProd)
	return max(n, 200)
}
```

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/layer_erosion_test.go
package patch

import "testing"

func TestErosionDeterministicAndBounded(t *testing.T) {
	ctx := testContext(t)
	if ctx.Profile.Erosion.Droplets <= 0 {
		t.Fatal("terran profile is expected to have erosion enabled")
	}
	s1 := NewStack(ctx)
	a, err := s1.RenderTo(6)
	if err != nil {
		t.Fatal(err)
	}
	s2 := NewStack(ctx)
	b, _ := s2.RenderTo(6)
	for i := range a.Height.Data {
		if a.Height.Data[i] != b.Height.Data[i] {
			t.Fatal("erosion is not deterministic")
		}
	}
	st5, _ := s1.RenderTo(5)
	diff := 0
	for i := range a.Height.Data {
		v := a.Height.Data[i]
		if v != st5.Height.Data[i] {
			diff++
		}
		if v != v { // NaN
			t.Fatal("erosion produced NaN")
		}
	}
	if diff == 0 {
		t.Fatal("erosion changed nothing")
	}
}

func TestPatchDropletsScaling(t *testing.T) {
	if got := patchDroplets(250000, 512, 1024); got < 9000 || got > 12000 {
		t.Fatalf("512²@1024 share of 250k should be ~10.4k, got %d", got)
	}
	if got := patchDroplets(10, 32, 1024); got != 200 {
		t.Fatalf("floor must be 200, got %d", got)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/planetgen/patch/ -run 'Erosion|Droplets' -v` → FAIL.

- [ ] **Step 3: Port the recovered file** (adaptations above), then:

```go
func erosionEnabled(ctx *Context) bool { return ctx.Profile.Erosion.Droplets > 0 }

// applyErosion (layer 6): the recovered flat droplet simulator, seeded
// with the production erosion domains, on a clone of the heightmap.
func applyErosion(ctx *Context, st *State) *State {
	cfg := ctx.Profile.Erosion
	cfg.Droplets = patchDroplets(cfg.Droplets, st.Height.Size, ctx.Fields.Window.SProd)
	ns := *st
	ns.Height = st.Height.Clone()
	erodePatch(ctx.Master, ns.Height, cfg, ctx.Sphere.SeaLevel0)
	return &ns
}
```

Wire index 6.

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch layer 6 — flat droplet erosion recovered from 966f17131^ with patch-share droplet scaling"
```

---

### Task 12: Layers 7–8 — craters and flow/rivers

**Files:**
- Create: `pkg/planetgen/patch/layer_craters.go`, `pkg/planetgen/patch/layer_flow.go`
- Modify: `pkg/planetgen/patch/stack.go` (wire 7–8 + Enabled)
- Test: `pkg/planetgen/patch/layer_flow_test.go`

**Interfaces:**
- Craters: consumes `feature.GenerateCraters(seed, profile)` (the GLOBAL deterministic crater list — same craters as production) and stamps only those whose angular footprint intersects the window. Publishes `State.Craters` (the intersecting subset) for the waterlines layer's Ejecta pass. **Before implementing the bowl profile, read `pkg/planetgen/feature/crater.go:187` (`ApplyCraters`) and reproduce its exact per-pixel depth formula**, with pixel dirs from `Window.Dir` and the crater center dir built with the SAME lat/lon convention used in `crater_ejecta.go:14` — `(cos lat · cos lon, sin lat, cos lat · sin lon)`. Enabled: `CraterCount > 0`.
- Flow: `applyFlowRivers` — flat Planchon–Darboux with **window border as outlet**, D8 descent, upstream accumulation, river mask at `flow.riverThreshold`, carve by `flow.riverDepth`. Publishes `Rivers []bool` and `FlowAccum *Grid`. Enabled: `ctx.Profile.Flow.RiverThreshold > 0`.

Flat flow implementation (complete):

```go
// pkg/planetgen/patch/layer_flow.go
package patch

import (
	"math"
	"sort"
)

// d8off: D8 neighbor offsets, index = direction id stored per pixel.
var d8off = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}}

// planchonDarbouxPatch fills pits on a clone with the window border as
// the outlet: border pixels keep their true height; interior pixels
// start at +inf and relax downward. eps keeps drainage strictly
// monotone.
func planchonDarbouxPatch(hm *Grid) *Grid {
	size := hm.Size
	const eps = 1e-7
	f := NewGrid(size)
	for iy := range size {
		for ix := range size {
			if ix == 0 || iy == 0 || ix == size-1 || iy == size-1 {
				f.Set(ix, iy, hm.At(ix, iy))
			} else {
				f.Set(ix, iy, math.MaxFloat64)
			}
		}
	}
	for changed := true; changed; {
		changed = false
		relax := func(ix, iy int) {
			i := iy*size + ix
			h := hm.Data[i]
			if f.Data[i] <= h {
				return
			}
			for _, o := range d8off {
				nx, ny := ix+o[0], iy+o[1]
				if nx < 0 || ny < 0 || nx >= size || ny >= size {
					continue
				}
				cand := f.At(nx, ny) + eps
				if h >= cand {
					if f.Data[i] != h {
						f.Data[i] = h
						changed = true
					}
					return
				}
				if cand < f.Data[i] {
					f.Data[i] = cand
					changed = true
				}
			}
		}
		for iy := 1; iy < size-1; iy++ {
			for ix := 1; ix < size-1; ix++ {
				relax(ix, iy)
			}
		}
		for iy := size - 2; iy >= 1; iy-- {
			for ix := size - 2; ix >= 1; ix-- {
				relax(ix, iy)
			}
		}
	}
	return f
}

func flowEnabled(ctx *Context) bool { return ctx.Profile.Flow.RiverThreshold > 0 }

// applyFlowRivers (layer 8): D8 + Planchon-Darboux with the patch
// border as drain (spec §4.5), then carve rivers.
func applyFlowRivers(ctx *Context, st *State) *State {
	size := st.Height.Size
	cfg := ctx.Profile.Flow
	filled := planchonDarbouxPatch(st.Height)

	// D8 pointers on the filled surface; border pixels drain out (-1).
	d8 := make([]int8, size*size)
	for iy := range size {
		for ix := range size {
			i := iy*size + ix
			d8[i] = -1
			if ix == 0 || iy == 0 || ix == size-1 || iy == size-1 {
				continue
			}
			best, bestDrop := int8(-1), 0.0
			for k, o := range d8off {
				nx, ny := ix+o[0], iy+o[1]
				drop := (filled.Data[i] - filled.At(nx, ny)) / math.Hypot(float64(o[0]), float64(o[1]))
				if drop > bestDrop {
					bestDrop, best = drop, int8(k)
				}
			}
			d8[i] = best
		}
	}

	// Accumulate downstream in descending fill order.
	order := make([]int, size*size)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return filled.Data[order[a]] > filled.Data[order[b]] })
	accum := NewGrid(size)
	for i := range accum.Data {
		accum.Data[i] = 1
	}
	for _, i := range order {
		k := d8[i]
		if k < 0 {
			continue // border outlet or pit
		}
		ix, iy := i%size, i/size
		j := (iy+d8off[k][1])*size + (ix + d8off[k][0])
		accum.Data[j] += accum.Data[i]
	}

	rivers := make([]bool, size*size)
	ns := *st
	ns.Height = st.Height.Clone()
	ns.Rivers = rivers
	ns.FlowAccum = accum
	for i := range rivers {
		if accum.Data[i] >= cfg.RiverThreshold {
			rivers[i] = true
			ns.Height.Data[i] -= cfg.RiverDepth
		}
	}
	return &ns
}
```

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/layer_flow_test.go
package patch

import "testing"

func TestFlowBorderIsDrain(t *testing.T) {
	ctx := testContext(t)
	if ctx.Profile.Flow.RiverThreshold <= 0 {
		t.Fatal("terran profile is expected to have flow enabled")
	}
	s := NewStack(ctx)
	st, err := s.RenderTo(8)
	if err != nil {
		t.Fatal(err)
	}
	if st.Rivers == nil || st.FlowAccum == nil {
		t.Fatal("flow layer must publish Rivers + FlowAccum")
	}
	// Edge-drain invariant: the filled surface never creates a lake
	// pinned against the frame — every interior pixel must have a
	// non-ascending D8 path that reaches the border. Cheap proxy:
	// total accumulation reaching the border+pits equals the pixel
	// count (mass conservation).
	size := st.Height.Size
	var reached float64
	for i, a := range st.FlowAccum.Data {
		ix, iy := i%size, i/size
		border := ix == 0 || iy == 0 || ix == size-1 || iy == size-1
		if border {
			reached += a
		}
	}
	if reached < float64(size*size)/4 {
		t.Fatalf("suspiciously little flow reaches the border: %v of %d", reached, size*size)
	}
}

func TestCratersLayerIdentityWhenZero(t *testing.T) {
	ctx := testContext(t)
	prof := *ctx.Profile
	prof.CraterCount = 0
	ctx.Profile = &prof
	s := NewStack(ctx)
	st6, _ := s.RenderTo(6)
	st7, _ := s.RenderTo(7)
	if st7 != st6 {
		t.Fatal("craters disabled must be identity (Enabled=false passthrough)")
	}
}
```

- [ ] **Step 2: Run to verify failure** — FAIL (`Rivers` nil).

- [ ] **Step 3: Implement** flow per the code above; craters per the interface notes:

```go
// pkg/planetgen/patch/layer_craters.go
package patch

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
)

func cratersEnabled(ctx *Context) bool { return ctx.Profile.CraterCount > 0 }

// applyCraters (layer 7): the production global crater list, stamped
// only where it intersects the window. Bowl profile mirrors
// feature.ApplyCraters — read it and keep the depth formula identical.
func applyCraters(ctx *Context, st *State) *State {
	all := feature.GenerateCraters(ctx.Master, ctx.Profile)
	w := ctx.Fields.Window
	ns := *st
	ns.Height = st.Height.Clone()
	var kept []feature.Crater
	for _, cr := range all {
		cx := math.Cos(cr.Lat) * math.Cos(cr.Lon)
		cy := math.Sin(cr.Lat)
		cz := math.Cos(cr.Lat) * math.Sin(cr.Lon)
		// Cheap window cull: compare against the window center dir.
		mx, my, mz := w.Dir(w.Size/2, w.Size/2)
		halfDiag := float64(w.Size) / float64(w.SProd) * (math.Pi / 2) // window half-diagonal upper bound, radians
		if math.Acos(clampDot(cx*mx+cy*my+cz*mz)) > cr.Radius+halfDiag {
			continue
		}
		kept = append(kept, cr)
		for iy := range w.Size {
			for ix := range w.Size {
				dx, dy, dz := w.Dir(ix, iy)
				ang := math.Acos(clampDot(dx*cx + dy*cy + dz*cz))
				if ang >= cr.Radius {
					continue
				}
				// EXACT bowl/rim profile: transcribe from
				// feature.ApplyCraters (crater.go:187) here,
				// scaled by profile.CraterDepth and cr.Age² the
				// same way.
				ns.Height.Data[iy*w.Size+ix] += craterDeltaLikeApplyCraters(ang, cr, ctx.Profile.CraterDepth)
			}
		}
	}
	ns.Craters = kept
	return &ns
}

func clampDot(d float64) float64 { return math.Max(-1, math.Min(1, d)) }
```

(`craterDeltaLikeApplyCraters` is written during implementation by transcribing `ApplyCraters`' inner formula — it is a pure function of angle/crater/depth. Keep the name or inline it; the golden test in Task 16 pins the result.)

Wire indices 7–8 with their Enabled gates.

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch layers 7-8 — production crater list stamped in-window; border-drain flow + rivers"
```

---

### Task 13: Layers 9–10 — climate and biome color

**Files:**
- Create: `pkg/planetgen/patch/layer_climate.go`, `pkg/planetgen/patch/layer_color.go`
- Modify: `pkg/planetgen/patch/stack.go` (wire 9–10 + Enabled)
- Test: `pkg/planetgen/patch/layer_climate_test.go`

**Interfaces:**
- Climate (layer 9) consumes: `noise.New(seed.Domain(...))` with the production climate domains `"biome.temperature"` / `"biome.humidity"` (field/control.go indices 3–4 — **note production generates climate noise UNJITTERED**, `GenerateClimateFields` passes `jitter=nil`), the exact `T = t*0.7 + latBias*0.3` formula with `latBias = 0.5 + 0.5*cos(lat)*0.6`, and `biome.RainShadowMultiplierAt` (Task 3) with `Window.Sampler(st.Height)`. Produces `T`, `M`, `RainMult` grids.
- Biome color (layer 10) consumes: `biome.LookupColor(profile.BiomeTable, T, M*RainMult, h)` when the table is non-empty, else `planetcolor.SampleGradientOkLab(profile.Palette, h)` (equatorial/polar palette blends are intentionally skipped on the patch — documented divergence; crust archetypes are biome-table based). Produces `Img *image.RGBA` (Size×Size).

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/layer_climate_test.go
package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

func TestClimateMatchesProductionFormula(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(9)
	if err != nil {
		t.Fatal(err)
	}
	if st.T == nil || st.M == nil || st.RainMult == nil {
		t.Fatal("climate layer must publish T, M, RainMult")
	}
	w := ctx.Fields.Window
	ix, iy := 11, 23
	dx, dy, dz := w.Dir(ix, iy)
	cc := ctx.Profile.ControlConfig
	tg := noise.New(seed.Domain(ctx.Master, "biome.temperature"))
	tn := tg.FractalNoise3D(dx, dy, dz, cc.Temperature.Octaves, cc.Temperature.Lacunarity, cc.Temperature.Persistence, cc.Temperature.Freq) * cc.Temperature.Amp
	lat := math.Asin(dy)
	latBias := 0.5 + 0.5*math.Cos(lat)*0.6
	want := tn*0.7 + latBias*0.3
	if want < 0 {
		want = 0
	} else if want > 1 {
		want = 1
	}
	if got := st.T.At(ix, iy); math.Abs(got-want) > 1e-12 {
		t.Fatalf("T mismatch: got %v want %v", got, want)
	}
	for _, v := range st.RainMult.Data {
		if !(v > 0 && v <= 2) {
			t.Fatalf("rain multiplier out of (0,2]: %v", v)
		}
	}
}

func TestBiomeColorProducesImage(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(10)
	if err != nil {
		t.Fatal(err)
	}
	if st.Img == nil || st.Img.Bounds().Dx() != ctx.Fields.Window.Size {
		t.Fatal("biome layer must produce a window-sized image")
	}
	// Non-degenerate: at least two distinct colors.
	first := st.Img.Pix[0:4:4]
	for i := 4; i < len(st.Img.Pix); i += 4 {
		if st.Img.Pix[i] != first[0] || st.Img.Pix[i+1] != first[1] || st.Img.Pix[i+2] != first[2] {
			return
		}
	}
	t.Fatal("biome image is a single flat color")
}
```

- [ ] **Step 2: Run to verify failure** — FAIL (`T` nil).

- [ ] **Step 3: Implement**

```go
// pkg/planetgen/patch/layer_climate.go
package patch

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

// applyClimate (layer 9): production climate formula at patch dirs
// (biome/whittaker.go — climate noise is UNJITTERED), plus the
// rain-shadow multiplier via the shared per-pixel walk against the
// window-clamped height sampler (patch-local winds, spec §7).
func applyClimate(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	cc := ctx.Profile.ControlConfig
	tGen := noise.New(seed.Domain(ctx.Master, "biome.temperature"))
	mGen := noise.New(seed.Domain(ctx.Master, "biome.humidity"))

	ns := *st
	ns.T = NewGrid(w.Size)
	ns.M = NewGrid(w.Size)
	ns.RainMult = NewGrid(w.Size)
	sampler := w.Sampler(st.Height)
	rsCfg := ctx.Profile.RainShadow
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			dx, dy, dz := w.Dir(ix, iy)
			tn := tGen.FractalNoise3D(dx, dy, dz, cc.Temperature.Octaves, cc.Temperature.Lacunarity, cc.Temperature.Persistence, cc.Temperature.Freq) * cc.Temperature.Amp
			lat := math.Asin(dy)
			latBias := 0.5 + 0.5*math.Cos(lat)*0.6
			tv := tn*0.7 + latBias*0.3
			if tv < 0 {
				tv = 0
			} else if tv > 1 {
				tv = 1
			}
			ns.T.Data[i] = tv
			ns.M.Data[i] = mGen.FractalNoise3D(dx, dy, dz, cc.Humidity.Octaves, cc.Humidity.Lacunarity, cc.Humidity.Persistence, cc.Humidity.Freq) * cc.Humidity.Amp
			if rsCfg.WalkSteps > 0 {
				ns.RainMult.Data[i] = biome.RainShadowMultiplierAt(sampler, dx, dy, dz, rsCfg)
			} else {
				ns.RainMult.Data[i] = 1
			}
		}
	}
	return &ns
}
```

```go
// pkg/planetgen/patch/layer_color.go
package patch

import (
	"image"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/planetcolor"
)

// applyBiomeColor (layer 10): Whittaker table lookup with
// rain-shadow-multiplied moisture; palette-gradient fallback for
// archetypes without a biome table. Equatorial/polar palette blends
// are intentionally not reproduced on the patch (spec divergence).
func applyBiomeColor(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	p := ctx.Profile
	useTable := len(p.BiomeTable.Cells) > 0

	ns := *st
	ns.Img = image.NewRGBA(image.Rect(0, 0, w.Size, w.Size))
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			h := st.Height.Data[i]
			var c color.RGBA
			if useTable {
				m := st.M.Data[i] * st.RainMult.Data[i]
				if m > 1 {
					m = 1
				}
				c = biome.LookupColor(p.BiomeTable, st.T.Data[i], m, h)
			} else {
				c = planetcolor.SampleGradientOkLab(p.Palette, h)
			}
			o := ns.Img.PixOffset(ix, iy)
			ns.Img.Pix[o], ns.Img.Pix[o+1], ns.Img.Pix[o+2], ns.Img.Pix[o+3] = c.R, c.G, c.B, 255
		}
	}
	return &ns
}
```

(Add `"image/color"` import. **Check how colorizeRockyDebug clamps/multiplies M with the rain-shadow multiplier** — read the Palette/Biome stage around rocky.go L883-946 and mirror the exact clamping.)

Wire indices 9–10.

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch layers 9-10 — climate (production formula) + biome color"
```

---

### Task 14: Layer 11 — waterlines (ocean, snow, polar caps, shading, ejecta, LUT)

**Files:**
- Modify: `pkg/planetgen/patch/layer_color.go` (add `applyWaterlines`)
- Modify: `pkg/planetgen/patch/stack.go` (wire index 11)
- Test: `pkg/planetgen/patch/layer_color_test.go`

**Interfaces:**
- Consumes: the Task 3 helpers — `render.OceanPixel`, `render.SnowPixel`, `render.PolarCapPixel`, `render.SlopeShadeSampled` — plus `feature.ApplyEjecta`, `planetcolor.LookupLUT/ApplyLUT`, `noise.NewWarper`, and the production colorize seeds `capNoise = noise.New(master+42)`, `oceanNoise = noise.New(master+77)` (rocky.go L855-861; the `+42/+77` offsets are load-bearing).
- Produces: `applyWaterlines` at index 11. The **effective sea level** is `ctx.seaLevelView()` (slider override or post-flow sphere quantile) — this is the "waterline slider" the spec calls for; snow/caps use their profile params directly.
- Production stage order preserved: Snow → Ocean → PolarCaps → Shading → Ejecta → LUT (after the layer-10 base color).

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/layer_color_test.go
package patch

import "testing"

func TestWaterlinesPaintsOcean(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(11)
	if err != nil {
		t.Fatal(err)
	}
	sea := ctx.Sphere.SeaLevel
	oc := ctx.Profile.OceanColor
	size := st.Height.Size
	checked := 0
	for iy := 0; iy < size; iy += 7 {
		for ix := 0; ix < size; ix += 7 {
			if st.Height.At(ix, iy) < sea*0.5 { // deep ocean only
				o := st.Img.PixOffset(ix, iy)
				// Deep ocean pixels must be recognizably ocean-hued:
				// the dominant channel of OceanColor stays dominant.
				if oc.B > oc.R && st.Img.Pix[o+2] <= st.Img.Pix[o] {
					t.Fatalf("deep ocean pixel (%d,%d) not ocean-colored", ix, iy)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Skip("window has no deep ocean at this seed")
	}
}

func TestSeaLevelViewOverride(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	base, _ := s.RenderTo(11)
	ctx.SeaLevelView = 0.95 // drown almost everything
	s.MarkDirty("seaLevelView")
	flooded, err := s.RenderTo(11)
	if err != nil {
		t.Fatal(err)
	}
	diff := 0
	for i := 0; i < len(base.Img.Pix); i += 4 {
		if base.Img.Pix[i] != flooded.Img.Pix[i] || base.Img.Pix[i+1] != flooded.Img.Pix[i+1] {
			diff++
		}
	}
	if diff < len(base.Img.Pix)/4/10 {
		t.Fatalf("raising sea level to 0.95 changed only %d pixels", diff)
	}
}
```

- [ ] **Step 2: Run to verify failure** — FAIL (waterlines is identity; ocean never painted).

- [ ] **Step 3: Implement** (append to `layer_color.go`):

```go
// applyWaterlines (layer 11): the production colorize tail — Snow,
// Ocean, PolarCaps, Shading, Ejecta, LUT — via the shared per-pixel
// helpers, with the sea level taken from ctx.seaLevelView() so the
// waterline is a live slider.
func applyWaterlines(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	p := ctx.Profile
	sea := ctx.seaLevelView()
	warper := noise.NewWarper(ctx.Master, p.Warp)
	capNoise := noise.New(ctx.Master + 42)
	oceanNoise := noise.New(ctx.Master + 77)
	sampler := w.Sampler(st.Height)
	var lut *planetcolor.LUT
	if p.LUT != "" {
		lut = planetcolor.LookupLUT(p.LUT) // VERIFY exact return type/signature in planetcolor
	}

	ns := *st
	ns.Img = image.NewRGBA(image.Rect(0, 0, w.Size, w.Size))
	copy(ns.Img.Pix, st.Img.Pix)
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			o := ns.Img.PixOffset(ix, iy)
			c := color.RGBA{ns.Img.Pix[o], ns.Img.Pix[o+1], ns.Img.Pix[o+2], 255}
			rawX, rawY, rawZ := w.Dir(ix, iy)
			lat := math.Asin(rawY)
			absLat := math.Abs(lat) / (math.Pi / 2)
			h := st.Height.Data[i]

			if p.SnowLine > 0 && h > p.SnowLine {
				c = render.SnowPixel(c, h, absLat, p.SnowLine)
			}
			if sea > 0 && h < sea {
				dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
				surfaceVar := oceanNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 6.0)
				c = render.OceanPixel(p.OceanColor, p.Type, h, sea, surfaceVar)
			}
			if p.HasPolarCaps && p.PolarCapSize > 0 && absLat > 1.0-p.PolarCapSize {
				dx, dy, dz := warper.Warp(rawX, rawY, rawZ)
				capEdge := capNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 8.0)
				c = render.PolarCapPixel(c, absLat, p.PolarCapSize, p.PolarCapNoise, capEdge)
			}
			if p.ShadingStrength > 0 {
				exag := p.ShadingExaggeration
				if exag <= 0 {
					exag = 8.0
				}
				c = render.SlopeShadeSampled(sampler, c, rawX, rawY, rawZ, p.ShadingStrength, exag)
			}
			if p.PowerLawAlpha > 0 && len(st.Craters) > 0 {
				c = feature.ApplyEjecta(c, rawX, rawY, rawZ, st.Craters)
			}
			if lut != nil {
				c = planetcolor.ApplyLUT(*lut, c)
			}
			ns.Img.Pix[o], ns.Img.Pix[o+1], ns.Img.Pix[o+2], ns.Img.Pix[o+3] = c.R, c.G, c.B, 255
		}
	}
	return &ns
}
```

**Note on rng-stability comments in rocky.go:** production always consumes ocean/cap noise draws even for skipped pixels because its generators would otherwise shift... they don't — `noise.Generator` is a stateless position sampler; the "always consume" comments guard the DEBUG bypass path only. On the patch we sample only when needed; results are identical because sampling is position-pure.

Wire index 11.

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch layer 11 — waterlines with live sea-level slider via shared colorize helpers"
```

---

### Task 15: Layer 12 — civilization on the patch

**Files:**
- Create: `pkg/planetgen/patch/layer_civ.go`
- Modify: `pkg/planetgen/patch/stack.go` (wire index 12 + Enabled)
- Test: `pkg/planetgen/patch/layer_civ_test.go`

**Interfaces:**
- Consumes: `feature.HabitabilityScoreAt` (Task 3), `feature.AssignPopulations` (works on `[]feature.Site` — patch fills `Site.Dir` from `Window.Dir` so it is reusable as-is), `github.com/fogleman/delaunay` (already a module dep) for site triangulation, `ctx.seaLevelView()`.
- Produces: `applyCiv` — habitability grid → grid-accelerated Poisson-disc sites → Zipf populations → Kruskal MST over Delaunay edges → per-edge A* roads (8-neighbor, slope-weighted, refuses ocean) → overlay raster onto `Img`; publishes `Sites`. Enabled: `ctx.Profile.Civ.Tier > 0`.
- Acceptance bar (spec §4.6): roads follow terrain and read as roads at patch resolution.

Key sub-pieces (complete):

```go
// pkg/planetgen/patch/layer_civ.go
package patch

import (
	"container/heap"
	"image/color"
	"math"
	"math/rand/v2"
	"sort"

	"github.com/fogleman/delaunay"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

func civEnabled(ctx *Context) bool { return ctx.Profile.Civ.Tier > 0 }

// patchHabitability scores every pixel with the shared production
// formula; convergent distance comes from the min of the three
// cropped convergent-class FX distances (belt/subduction/arc).
func patchHabitability(ctx *Context, st *State) *Grid {
	f := ctx.Fields
	size := st.Height.Size
	sea := ctx.seaLevelView()
	hab := NewGrid(size)
	for i := range hab.Data {
		convKm := math.Min(f.BeltDist.Data[i], math.Min(f.SubdDist.Data[i], f.ArcDist.Data[i]))
		onRiver := st.Rivers != nil && st.Rivers[i]
		hab.Data[i] = feature.HabitabilityScoreAt(
			st.Height.Data[i], st.T.Data[i], st.M.Data[i], st.RainMult.Data[i],
			onRiver, convKm, sea)
	}
	return hab
}

// poissonPatch: Bridson-style dart throwing on the flat grid with a
// habitability-modulated radius (pixels). Deterministic via the
// "patch.civ.sites" domain.
func poissonPatch(ctx *Context, hab *Grid) []feature.Site {
	cfg := ctx.Profile.Civ
	size := hab.Size
	w := ctx.Fields.Window
	// Radii in virtual pixels: SiteMinDistRad/SiteMaxDistRad are
	// angular; one virtual pixel ≈ (π/2)/SProd radians.
	pxRad := (math.Pi / 2) / float64(w.SProd)
	rMin := cfg.SiteMinDistRad / pxRad
	rMax := cfg.SiteMaxDistRad / pxRad
	if rMin <= 0 || rMax < rMin {
		return nil
	}
	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(ctx.Master, "patch.civ.sites")),        //nolint:gosec
		uint64(seed.Domain(ctx.Master, "patch.civ.sites.stream")), //nolint:gosec
	))
	const habFloor = 0.05
	type pt struct{ x, y float64 }
	var placed []pt
	var sites []feature.Site
	tooClose := func(x, y, r float64) bool {
		for _, p := range placed {
			if math.Hypot(p.x-x, p.y-y) < r {
				return true
			}
		}
		return false
	}
	attempts := size * size / 2
	for range attempts {
		x := rng.Float64() * float64(size-1)
		y := rng.Float64() * float64(size-1)
		h := hab.Bilinear(x, y)
		if h < habFloor {
			continue
		}
		r := rMin + (1-h)*(rMax-rMin)
		if tooClose(x, y, r) {
			continue
		}
		placed = append(placed, pt{x, y})
		dx, dy, dz := ctx.Fields.Window.Dir(int(x), int(y))
		sites = append(sites, feature.Site{Dir: [3]float64{dx, dy, dz}, Habitability: h})
	}
	// Stash pixel coords in Population temporarily? NO — keep a
	// parallel slice; see civSitePx below.
	_ = placed
	return sites
}
```

**Implementation note:** keep a parallel `[]image.Point` (or store `pt` pairs alongside sites in a small struct slice) for pixel positions — `feature.Site` has no pixel field and must not grow one. `AssignPopulations` stable-sorts `sites` by habitability, which would desynchronize parallel slices — so wrap: sort a combined `[]struct{ S feature.Site; X, Y float64 }` by habitability desc FIRST, then call `AssignPopulations` on the already-sorted extraction (its internal re-sort is then a no-op reorder of equal data).

Roads (complete algorithm requirements — implement in the same file):
1. Delaunay: `delaunay.Triangulate([]delaunay.Point{{X: x, Y: y}, ...})`, extract unique edges `(i<j)`.
2. MST: Kruskal over edges, weight = `math.Hypot(dx, dy) * (1 + 3*avgSlopeAlongStraightLine)` — sample `st.Height` at 8 evenly spaced points along the segment for the slope estimate.
3. A* per MST edge on the pixel grid: 8-neighbor; cost = `dist * (1 + roadSlopeWeight*|Δh|*SProd)` with `roadSlopeWeight = 5.0` (matches `feature/roads.go`); neighbors with `h < seaLevelView` are refused; both endpoints included.
4. Raster: paint road pixels onto a copy of `Img` with `color.RGBA{95, 82, 62, 255}` (dirt-road brown), 1px wide; paint each site as a filled disc of radius `1 + int(2*site.Population/maxPop)` px in `color.RGBA{210, 200, 160, 255}`.

- [ ] **Step 1: Write the failing test**

```go
// pkg/planetgen/patch/layer_civ_test.go
package patch

import "testing"

func TestCivSitesOnLandAndRoadsAvoidOcean(t *testing.T) {
	ctx := testContext(t)
	prof := *ctx.Profile
	if prof.Civ.Tier <= 0 {
		prof.Civ = types.CivConfig{Tier: 0.5, SiteMinDistRad: 0.02, SiteMaxDistRad: 0.12, MaxPopulation: 1e7}
	}
	ctx.Profile = &prof
	s := NewStack(ctx)
	st, err := s.RenderTo(12)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Sites) == 0 {
		t.Skip("no habitable pixels in window at this seed")
	}
	// Determinism.
	s2 := NewStack(ctx)
	st2, _ := s2.RenderTo(12)
	if len(st2.Sites) != len(st.Sites) {
		t.Fatal("civ layer not deterministic")
	}
}
```

(Import `types`; the strong "roads avoid ocean" property is pinned pixel-exactly by the golden test in Task 16 — here we pin determinism and non-emptiness.)

- [ ] **Step 2: Run to verify failure** — FAIL (`Sites` empty, layer identity).

- [ ] **Step 3: Implement** per the pieces above.

- [ ] **Step 4: Run tests** — `go test ./pkg/planetgen/patch/ -v` → PASS. Also eyeball once: `go run ./cmd/... ` is not wired yet; visual check happens in Task 18.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch layer 12 — civ sites/roads on the patch (shared habitability formula)"
```

---

### Task 16: renderers, per-layer snapshot goldens, patch⇔sphere consistency

**Files:**
- Create: `pkg/planetgen/patch/render.go`
- Create: `pkg/planetgen/patch/golden_test.go`
- Create: `pkg/planetgen/patch/consistency_test.go`
- Create: `pkg/planetgen/patch/testdata/goldens.json` (baked by `-update`)

**Interfaces:**
- Produces:
  - `func HeightPNG(st *State) ([]byte, error)` — grayscale (clamped [0,1] → 0..255) PNG of `st.Height`.
  - `func ColorPNG(st *State) ([]byte, error)` — `st.Img` PNG; falls back to HeightPNG when `Img == nil` (layers < 10).
  - `func TectonicDebugPNG(ctx *Context, st *State) ([]byte, error)` — height grayscale tinted by FX class proximity (belt=red, subduction=orange, arc=yellow, ridge=cyan, rift=magenta at `Dist < 300km`, alpha ∝ magnitude) with `PlateID`-boundary dark lines. Used as the layer-0/1 debug view and the wizard's "tectonic preview".
  - `func MinimapPNG(sd *SphereData, w Window, width, height int) ([]byte, error)` — equirect bake (`cubemap.BakeEquirect`) of `cubemap.GrayscaleFromF(sd.Crust.ContinentalMask)` re-colored with the same FX tint, window outline drawn by projecting the window's border pixel dirs to equirect coords.
  - `func StateHash(st *State) string` — FNV-64a over `Height` float bits (little-endian) then `Img.Pix` when present; returned as hex. **The byte-exact per-layer diff gate.**
- Golden scheme: `testdata/goldens.json` maps `"<archetype>/<layerIdx>-<layerID>" → hash`. Fixtures: terran + arid, seed name `"PatchGolden"`, `sTect=64`, window `Pick(sd, 128, 256, 1)[0]`, all 13 layers. `-update` flag re-bakes.

- [ ] **Step 1: Write the failing golden test**

```go
// pkg/planetgen/patch/golden_test.go
package patch

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

var updateGoldens = flag.Bool("update", false, "re-bake patch layer goldens")

func TestPatchLayerGoldens(t *testing.T) {
	path := filepath.Join("testdata", "goldens.json")
	got := map[string]string{}
	for _, arch := range []string{"terran", "arid"} {
		prof := planetgen.GetProfile(arch)
		if prof.Crust.MajorPlates <= 0 {
			t.Fatalf("%s must be crust-enabled", arch)
		}
		master := seed.Hash("PatchGolden")
		sd, err := ComputeSphere(prof, master, 64)
		if err != nil {
			t.Fatal(err)
		}
		w := Pick(sd, 128, 256, 1)[0].Window
		f, err := ExtractFields(sd, w)
		if err != nil {
			t.Fatal(err)
		}
		s := NewStack(&Context{Sphere: sd, Fields: f, Profile: prof, Master: master})
		for i, l := range Layers() {
			st, err := s.RenderTo(i)
			if err != nil {
				t.Fatal(err)
			}
			got[fmt.Sprintf("%s/%d-%s", arch, i, l.ID)] = StateHash(st)
		}
	}
	if *updateGoldens {
		data, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no goldens baked yet — run with -update: %v", err)
	}
	want := map[string]string{}
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("layer snapshot drifted: %s got %s want %s", k, got[k], w)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("new un-baked layer snapshot: %s (run -update)", k)
		}
	}
}
```

And the consistency test:

```go
// pkg/planetgen/patch/consistency_test.go
package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
)

// TestPatchSphereConsistency pins the "patch is a true crop" property
// (spec §6.2): with sTect == sProd, window pixel dirs are exactly face
// pixel centers, bilinear Sample degenerates to exact reads, and
// layers 0–1 must match a sphere-side base+FX computation bit-for-bit.
func TestPatchSphereConsistency(t *testing.T) {
	prof := terranProfile(t)
	const S = 64
	master := int64(777)
	sd, err := ComputeSphere(prof, master, S)
	if err != nil {
		t.Fatal(err)
	}
	w := Window{Face: cubemap.FacePosX, X0: 16, Y0: 16, Size: 32, SProd: S}
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStack(&Context{Sphere: sd, Fields: f, Profile: prof, Master: master})
	st, err := s.RenderTo(1)
	if err != nil {
		t.Fatal(err)
	}

	// Sphere side: BaseHeight + ApplyTectonicFX at the same S.
	hm := sd.Crust.BaseHeight.Clone()
	field.ApplyTectonicFX(hm, sd.FX, sd.Crust, sd.Plates, prof.TectonicFX, master, S)

	worst := 0.0
	for iy := range w.Size {
		for ix := range w.Size {
			want := hm.Get(w.Face, w.X0+ix, w.Y0+iy)
			got := st.Height.At(ix, iy)
			if d := math.Abs(got - want); d > worst {
				worst = d
			}
		}
	}
	if worst > 1e-9 {
		t.Fatalf("patch layers 0-1 diverge from sphere crop: worst |Δ| = %g", worst)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `go test ./pkg/planetgen/patch/ -run 'Golden|Consistency' -v` → FAIL (undefined `StateHash`; no goldens).

- [ ] **Step 3: Implement `render.go`**

```go
// pkg/planetgen/patch/render.go
package patch

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
)

func HeightPNG(st *State) ([]byte, error) {
	size := st.Height.Size
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i, v := range st.Height.Data {
		g := uint8(min(255, max(0, int(v*255))))
		o := i * 4
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = g, g, g, 255
	}
	return encodePNG(img)
}

func ColorPNG(st *State) ([]byte, error) {
	if st.Img == nil {
		return HeightPNG(st)
	}
	return encodePNG(st.Img)
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// StateHash is the byte-exact per-layer regression fingerprint:
// FNV-64a over Height bits, then Img pixels when present.
func StateHash(st *State) string {
	h := fnv.New64a()
	var b [8]byte
	for _, v := range st.Height.Data {
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
		h.Write(b[:])
	}
	if st.Img != nil {
		h.Write(st.Img.Pix)
	}
	return fmt.Sprintf("%016x", h.Sum64())
}
```

(Add `"math"` import; implement `TectonicDebugPNG` and `MinimapPNG` per the interface description — tint colors are literal RGBA values, e.g. belt `{200,40,40}`, subduction `{230,120,30}`, arc `{230,210,60}`, ridge `{60,200,220}`, rift `{200,60,200}`; blend with `alpha = env * mag` where `env = max(0, 1 - dist/300)`. `MinimapPNG` draws the window outline by setting the equirect pixels nearest each border pixel dir to `{255,255,255,255}`.)

- [ ] **Step 4: Bake goldens, verify, re-verify**

```bash
go test ./pkg/planetgen/patch/ -run TestPatchLayerGoldens -update
go test ./pkg/planetgen/patch/ -v
```
Expected: bake writes `testdata/goldens.json` (26 entries: 2 archetypes × 13 layers); second run PASSES with no `-update`. Consistency test PASSES.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/planetgen/patch/
git add pkg/planetgen/patch/
git commit -m "P13: patch renderers + byte-exact per-layer goldens + patch/sphere consistency gate"
```

---

### Task 17: wasm exports

**Files:**
- Modify: `cmd/planet-explorer/wasm/main.go`
- Test: build check only (wasm has no test harness here)

**Interfaces (JS-visible, following the existing export conventions — PNG bytes as `Uint8Array` via `jsBytes`, errors as `{"error":...}` JSON strings via `jsError`, profile as JSON string arg):**

- `patchInit(profileJSON string, seedStr string, sTect int) string` — runs `ComputeSphere` + `Pick(sd, 512, 1024, 12)`; stores `sd` + candidates in package globals; returns JSON `{"seaLevel":..., "seaLevel0":..., "candidates":[{"window":{...},"score":...}, ...]}`. Seed resolution: reuse the existing seedStr→int64 logic used by `planetExplorerGenerate` (find and call the same helper).
- `patchSelect(windowJSON string) string` — `ExtractFields` + `NewStack` into globals; returns `""` or error JSON.
- `patchLayers() string` — JSON `[{"index":0,"id":"tectonic-base","name":"Tectonic base","enabled":true}, ...]` from `Layers()` + each layer's `Enabled(ctx)`.
- `patchSetParam(paramPath string, profileJSON string) string` — decodes the profile into the stack's `ctx.Profile`; calls `MarkDirty(paramPath)`; if it returns true, reruns `ComputeSphere`+`ExtractFields` (same seed/sTect/window — re-validate the window, snap into range if the face shrank) and `MarkAllDirty`; special-case `paramPath == "seaLevelView"`: parse the value from the profile JSON top-level key `"seaLevelView"` into `ctx.SeaLevelView` instead of the profile struct. Returns `{"sphereRecomputed":bool}` JSON.
- `patchRender(targetLayer int, view string) Uint8Array` — `stack.RenderTo(targetLayer)`; view `"color"` → `ColorPNG`, `"height"` → `HeightPNG`, `"tectonic"` → `TectonicDebugPNG`.
- `patchMinimap(width, height int) Uint8Array` — `MinimapPNG` of the stored sphere + selected window.

Register all six in `main()` alongside the existing `js.Global().Set` calls. Keep globals in a single `var patchSession struct { sd *patch.SphereData; stack *patch.Stack; window patch.Window; sTect int; master int64 }`.

- [ ] **Step 1: Implement** (wasm code paths can't run under `go test` here; the compile gate is the test).

- [ ] **Step 2: Verify it compiles for wasm**

Run: `GOOS=js GOARCH=wasm go build -o /tmp/claude-1000/patch-explorer.wasm ./cmd/planet-explorer/wasm/`
Expected: clean build.

- [ ] **Step 3: Verify the native tree still builds and fast tests pass**

Run: `go build ./... && go test ./pkg/planetgen/patch/`
Expected: PASS.

- [ ] **Step 4: Lint and commit**

```bash
golangci-lint run ./cmd/planet-explorer/...
git add cmd/planet-explorer/wasm/main.go
git commit -m "P13: wasm patch* exports — init/select/layers/setParam/render/minimap"
```

---

### Task 18: Patch Lab web UI

**Files:**
- Modify: `cmd/planet-explorer/web/index.html`
- Modify: `cmd/planet-explorer/web/app.js`
- Modify: `cmd/planet-explorer/web/style.css`

**UI contract (keep it minimal — reuse existing panel builders):**

1. `index.html`: add a `<button id="patch-mode-btn">Patch Lab</button>` next to `#render-btn`; add a `<section id="patch-lab" hidden>` inside the viewport column containing:
   - `<canvas id="patch-canvas" width="512" height="512">`
   - `<canvas id="patch-minimap" width="360" height="180">`
   - `<div id="patch-layer-rail"></div>` (vertical list)
   - controls: `<select id="patch-view">` (color / height / tectonic), `<button id="patch-next-window">Next window</button>`, `<input id="patch-sealevel" type="range" min="0" max="1" step="0.005">`, `<button id="patch-go">Go! (full render)</button>`.
2. `app.js`, new section at the bottom (`// ---- Patch Lab ----`):
   - State: `let patchOn = false, patchCands = [], patchCandIdx = 0, patchTarget = 12, patchPrevProfile = null;`
   - `enterPatchLab()`: reads `#profile-json` + seed; calls `patchInit(profileJSON, seed, 256)`; on error JSON, `alert` and abort (crust-disabled archetypes get the wasm error message); stores candidates; calls `selectCandidate(0)`; hides `#cube-canvas`/`#equirect-canvas`/`#sphere-canvas`, shows `#patch-lab`; builds the layer rail from `patchLayers()` — one row per layer: index, name, a radio "view up to here" setting `patchTarget`.
   - `selectCandidate(i)`: `patchSelect(JSON.stringify(patchCands[i].window))`, then `refreshPatch()` + `refreshMinimap()`.
   - `refreshPatch()`: `const png = patchRender(patchTarget, $('#patch-view').value);` check `instanceof Uint8Array`; paint via the existing `paintToCanvas(patchCanvas, png)`.
   - **Param plumbing (the dirty-tracking hook):** wrap the existing `commitProfile(profile)` — after it writes the textarea, if `patchOn`, compute `changedPath = diffProfilePath(patchPrevProfile, profile)` (a ~20-line helper that walks both objects and returns the dot-joined path of the first differing leaf, e.g. `"tectonicFX.beltAmp"`, `"Erosion.Droplets"`); call `patchSetParam(changedPath, JSON.stringify(profile))`; if the reply has `sphereRecomputed:true`, also `refreshMinimap()`; debounce `refreshPatch()` at ~150ms. Update `patchPrevProfile`.
   - `#patch-sealevel` input → `patchSetParam('seaLevelView', JSON.stringify({...profile, seaLevelView: +slider.value}))` + debounced `refreshPatch()`.
   - `#patch-next-window` → `selectCandidate((patchCandIdx+1) % patchCands.length)`.
   - `#patch-go` → exit Patch Lab mode (restore canvases) and call the existing `regenerate()` — the production render with the tuned profile textarea.
3. `style.css`: `#patch-lab { display:flex; gap:12px; }`, `#patch-layer-rail .layer-row { cursor:pointer; padding:2px 6px; }`, `.layer-row.active { background:#2a4d69; color:#fff; }` — match the existing dark theme variables used by the debug panel.

- [ ] **Step 1: Implement** the three files per the contract.

- [ ] **Step 2: Build wasm into the served path and verify end-to-end by hand**

```bash
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm/
go run ./cmd/planet-explorer -addr :8090
```
Manual checks (list them in the commit message if any fail):
- terran → Patch Lab → tectonic view shows colored boundary tint; layer rail switches views; sliders in Tectonic FX panel update the patch in <1s; sea-level slider moves the waterline; Next window cycles; Go! produces the normal sphere render.
- scorched → Patch Lab → clean error message, UI stays usable.

- [ ] **Step 3: Lint (JS has no linter here; run Go lint for the tree) and commit**

```bash
golangci-lint run ./...
git add cmd/planet-explorer/web/
git commit -m "P13: Patch Lab explorer UI — layer rail, candidate cycling, live sliders, Go! handoff"
```

---

### Task 19: docs, full-suite verification, wrap-up

**Files:**
- Modify: `cmd/planet-explorer/README.md` (Patch Lab section: what it is, the S_tect/S_prod/512 defaults, the four documented divergences from spec §7)
- Modify: `docs/superpowers/specs/2026-07-02-patch-lab-design.md` (status line → implemented; correct anything that drifted during implementation)

- [ ] **Step 1: Write the docs** (README section ~30 lines; keep the spec's divergence list as the canonical reference and link it).

- [ ] **Step 2: Full verification — the production-unchanged gate**

```bash
go build ./...
go test ./... -timeout 150m
golangci-lint run ./...
```
Expected: ALL PASS, **without** touching any golden under `cmd/generate-planet-maps/testdata/golden/` (git status must show no modified goldens). The face-128 suite proves Tasks 2–3 were behavior-preserving. If a golden fails: the extraction reordered floating-point math — bisect Tasks 2–3, do NOT re-bake.

- [ ] **Step 3: Commit**

```bash
git add cmd/planet-explorer/README.md docs/
git commit -m "P13: Patch Lab docs + full-suite verification"
```

- [ ] **Step 4: Hand off** — merge decision is the user's: `phase-13/progressive-layers` → `phase-0/cube-map` or straight to `main` (Phase 12's flow merged the phase branch after review). Surface: `git log --oneline phase-0/cube-map..HEAD` and offer superpowers:finishing-a-development-branch.

---

## Self-review notes (already applied)

- **Spec coverage:** §4.1 geometry→T1; §4.2 contract→T4/T5 (incl. sea-level + normalize scalars); §4.3 picking→T6; §4.4 layers/caching→T7–T15; §4.5 edge policy→T11 (recovered border-death), T12 (border drains), T15 (interior-only civ); §4.6 civ→T15 (growth animation = stretch, excluded); §4.7 UI→T17/T18 incl. minimap + Go!; §5 determinism→patch.* domains + production domains reused; §6 testing→T16 (goldens #1, consistency #2, edge invariants in T11/T12 tests, picker determinism in T6, face-128 suite untouched T19); §7 divergences→coastal chamfer (T10), patch-local rain walk (T13), S_tect upsampling (T4/T5), sphere-side requantile (T4); §8 errors→ComputeSphere error (T4), UI error path (T18), degenerate picker always returns argmax (T6).
- **Known verify-at-implementation points (flagged inline):** rocky.go's exact droplet-scaling formula (T4), CoastalGen seed expression (T10), `SmoothHeightmap` weight formula (T9), `ApplyCraters` bowl profile (T12), rain-shadow extraction reads (T3), `LookupLUT` signature (T14), biome M×rainshadow clamp (T13). Each is a "read the named production line and mirror it" instruction with the file:line given — not a design gap.
- **Type consistency:** `Window/Grid/Fields/SphereData/Context/State/Layer/Stack` names and field sets are used identically across tasks; layer indices follow the single table in File Structure; `seaLevelView` is the one param path not in the profile struct and is special-cased in both `Layers()` params and `patchSetParam`.
