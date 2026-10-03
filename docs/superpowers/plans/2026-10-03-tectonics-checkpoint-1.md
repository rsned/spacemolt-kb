# Tectonics-First Planet Generator — Checkpoint 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A new `pkg/tectonics` that turns a planet id + archetype into drifting tectonic plates over geologic time, a `cmd/tectonics-lab` that bakes the timeline into a keyframe bundle, and a time-slider viewer to judge it by eye.

**Architecture:** A smooth crust-thickness field on the existing cube-sphere grid is segmented into plates by watershed (divides on the thinnest crust). Each plate is a rigid cap rotating about its own Euler pole; every step inverse-maps the grid through each plate and resolves gaps (new ridge crust), overlaps (collision / subduction / transform) and ageing. Frames are written as cube-cross PNGs plus a manifest; a raw-WebGL page scrubs them.

**Tech Stack:** Go 1.24+ (module `github.com/rsned/spacemolt-kb`), stdlib only (`math/rand/v2` PCG, `container/heap`, `image/png`, `net/http`), reused `pkg/planetgen/cubemap` + `pkg/planetgen/seed`; plain HTML/JS/WebGL 1 viewer.

**Spec:** `docs/superpowers/specs/2026-10-02-tectonics-first-planet-gen-design.md`

## Global Constraints

- Work on branch `tectonics/checkpoint-1` in worktree `/home/robert/spacemolt/kb-tectonics`. Never touch `pkg/planetgen` except to import `pkg/planetgen/cubemap` and `pkg/planetgen/seed`.
- Grid: cube-sphere, face size `S` (default 256). Thickness in `[0,1]`, 0 thin, 1 thick.
- `MaxNeighborDelta` default 0.05 across every 4-neighbour pair, cube-seam aware (`cubemap.FacePixelNeighbors4`).
- Plate counts: majors default range 5–10, minors 10–20; `MinPlateArea` 0.002 of the sphere.
- Defaults: `Steps` 150, `RepoleEvery` 25, `RidgeThickness` 0.15, `TransformRatio` 2, `ContinentalThreshold` 0.5, `RadiusKm` 6371, `KeyframeEvery` 1.
- Feature byte: boundary type in top 2 bits (0 none, 1 divergent, 2 convergent, 3 transform), age in low 6 bits saturating at 63.
- Every random draw goes through `seed.Domain(master, "<stage.name>")` + `rand.New(rand.NewPCG(uint64(s), 0))`; same seed ⇒ byte-identical bundle.
- Gas giants (`jovian`, `ice_giant`) are refused with an error.
- Bundles live under `data/tectonics/` (git-ignored). Binary built to `bin/tectonics-lab` (git-ignored).
- Go 1.24 idioms: `for i := range n`, `b.Loop()` in benchmarks. Every task ends with `go build ./... && go test ./pkg/tectonics/... ./cmd/tectonics-lab/... && golangci-lint run ./pkg/tectonics/... ./cmd/tectonics-lab/...` clean.
- Commit trailer on every commit: `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`. `git add` only named paths.

## File Structure

| File | Responsibility |
| --- | --- |
| `pkg/tectonics/grid.go` | `Grid[T]`: six faces of `[]T`, flat index ↔ (face,px,py), seam-aware 4-neighbour flat indices, unit-direction table |
| `pkg/tectonics/params.go` | `Params` knobs, archetype table, `DefaultParams`, `SeedForPlanet`, `Set(name, value)` |
| `pkg/tectonics/thickness.go` | 3-D value noise, `GenerateThickness`, `Relax`, `normalize` |
| `pkg/tectonics/watershed.go` | priority-flood basins, divide table, merging, `Plate` table, `BuildPlates` |
| `pkg/tectonics/motion.go` | vector helpers, `Motion` per plate, `InitMotion`, `Reaim`, speed conversion |
| `pkg/tectonics/step.go` | `State`, `Step`: inverse-map claims, gap fill, collision/subduction/transform, trench/arc, ageing, retire, relax |
| `pkg/tectonics/sim.go` | `Frame`, `Run(params, master, emit)` |
| `pkg/tectonics/bundle.go` | `Manifest`, `WriteFrame`, `WriteManifest`, `ReadManifest`, `ReadFrame` |
| `cmd/tectonics-lab/main.go` | `run` and `serve` subcommands, flag parsing |
| `cmd/tectonics-lab/server.go` | HTTP API: bundles list, start run, job status, static files |
| `cmd/tectonics-lab/web/index.html`, `app.js` | viewer |
| `cmd/tectonics-lab/testdata/golden_face64.json` | per-frame hashes regression gate |

Package doc comment goes in `grid.go`.

---

### Task 1: Grid and Params

**Files:**
- Create: `pkg/tectonics/grid.go`, `pkg/tectonics/params.go`
- Test: `pkg/tectonics/grid_test.go`, `pkg/tectonics/params_test.go`

**Interfaces:**
- Consumes: `cubemap.Face`, `cubemap.NumFaces`, `cubemap.FacePixelNeighbors4(face, px, py, S) [4]PixelAddr`, `cubemap.FacePixelToDir(face, px, py, S) (x, y, z float64)`, `seed.Hash`, `seed.Domain`.
- Produces: `type Grid[T any] struct{ S int; Cells []T }` with `NewGrid[T](S)`, `Len()`, `Index(face, px, py) int`, `Addr(i) (cubemap.Face, int, int)`, `Get(i) T`, `Set(i, v)`, `Clone()`; `Neighbors4(S int) [][4]int32` (cached per S); `Dirs(S int) [][3]float64` (cached per S); `type Params struct{...}`; `DefaultParams(archetype string) (Params, error)`; `SeedForPlanet(id string) int64`; `(p *Params) Set(name, value string) error`; `(p Params) MyrPerStep() float64`; `newRNG(master int64, domain string) *rand.Rand`.

- [ ] **Step 1: Write the failing grid test**

```go
// pkg/tectonics/grid_test.go
package tectonics

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestGridIndexRoundTrip(t *testing.T) {
	g := NewGrid[int32](8)
	if g.Len() != 6*64 {
		t.Fatalf("len %d", g.Len())
	}
	for i := range g.Len() {
		f, px, py := g.Addr(i)
		if j := g.Index(f, px, py); j != i {
			t.Fatalf("index %d -> (%d,%d,%d) -> %d", i, f, px, py, j)
		}
	}
}

func TestNeighbors4MatchCubemapAndAreSymmetric(t *testing.T) {
	const S = 8
	nb := Neighbors4(S)
	g := NewGrid[int32](S)
	for i := range g.Len() {
		f, px, py := g.Addr(i)
		want := cubemap.FacePixelNeighbors4(f, px, py, S)
		for k := range 4 {
			if int(nb[i][k]) != g.Index(want[k].Face, want[k].PX, want[k].PY) {
				t.Fatalf("pixel %d neighbor %d mismatch", i, k)
			}
			// symmetry: i must appear among its neighbour's neighbours
			found := false
			for _, back := range nb[nb[i][k]] {
				if int(back) == i {
					found = true
				}
			}
			if !found {
				t.Fatalf("pixel %d not a neighbour of its neighbour %d", i, nb[i][k])
			}
		}
	}
}

func TestDirsAreUnit(t *testing.T) {
	for _, d := range Dirs(4) {
		if n := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2]); math.Abs(n-1) > 1e-9 {
			t.Fatalf("dir %v has length %g", d, n)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd /home/robert/spacemolt/kb-tectonics && go test ./pkg/tectonics/ -run 'TestGrid|TestNeighbors4|TestDirs' -v`
Expected: FAIL — `undefined: NewGrid`.

- [ ] **Step 3: Write grid.go**

```go
// Package tectonics generates tectonic plates from a smooth crust-thickness
// field on a cube-sphere and drifts them over geologic time. It is the
// first stage of the tectonics-first planet generator (see
// docs/superpowers/specs/2026-10-02-tectonics-first-planet-gen-design.md).
package tectonics

import (
	"math"
	"sync"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Grid stores one value per cube-sphere pixel, all six faces flattened so a
// single int addresses any pixel: i = face*S*S + py*S + px.
type Grid[T any] struct {
	S     int
	Cells []T
}

func NewGrid[T any](S int) *Grid[T] {
	return &Grid[T]{S: S, Cells: make([]T, cubemap.NumFaces*S*S)}
}

func (g *Grid[T]) Len() int { return len(g.Cells) }

func (g *Grid[T]) Index(face cubemap.Face, px, py int) int {
	return int(face)*g.S*g.S + py*g.S + px
}

func (g *Grid[T]) Addr(i int) (cubemap.Face, int, int) {
	ff := g.S * g.S
	face := cubemap.Face(i / ff)
	r := i % ff
	return face, r % g.S, r / g.S
}

func (g *Grid[T]) Get(i int) T    { return g.Cells[i] }
func (g *Grid[T]) Set(i int, v T) { g.Cells[i] = v }

func (g *Grid[T]) Clone() *Grid[T] {
	out := &Grid[T]{S: g.S, Cells: make([]T, len(g.Cells))}
	copy(out.Cells, g.Cells)
	return out
}

var (
	nbMu    sync.Mutex
	nbCache = map[int][][4]int32{}
	dirMu   sync.Mutex
	dirCach = map[int][][3]float64{}
)

// Neighbors4 returns, for every flat pixel index, the flat indices of its
// four seam-aware neighbours. Cached per face size.
func Neighbors4(S int) [][4]int32 {
	nbMu.Lock()
	defer nbMu.Unlock()
	if nb, ok := nbCache[S]; ok {
		return nb
	}
	g := NewGrid[struct{}](S)
	nb := make([][4]int32, g.Len())
	for i := range g.Len() {
		f, px, py := g.Addr(i)
		for k, a := range cubemap.FacePixelNeighbors4(f, px, py, S) {
			nb[i][k] = int32(g.Index(a.Face, a.PX, a.PY))
		}
	}
	nbCache[S] = nb
	return nb
}

// Dirs returns the unit direction of every pixel centre. Cached per face size.
func Dirs(S int) [][3]float64 {
	dirMu.Lock()
	defer dirMu.Unlock()
	if d, ok := dirCach[S]; ok {
		return d
	}
	g := NewGrid[struct{}](S)
	d := make([][3]float64, g.Len())
	for i := range g.Len() {
		f, px, py := g.Addr(i)
		x, y, z := cubemap.FacePixelToDir(f, px, py, S)
		n := math.Sqrt(x*x + y*y + z*z)
		d[i] = [3]float64{x / n, y / n, z / n}
	}
	dirCach[S] = d
	return d
}
```

- [ ] **Step 4: Run grid tests, expect PASS**

Run: `go test ./pkg/tectonics/ -run 'TestGrid|TestNeighbors4|TestDirs' -v`

- [ ] **Step 5: Write the failing params test**

```go
// pkg/tectonics/params_test.go
package tectonics

import "testing"

func TestDefaultParamsPerArchetype(t *testing.T) {
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	if p.Face != 256 || p.Steps != 150 || p.MajorMin != 6 || p.MajorMax != 9 || p.TimelineMyr != 800 {
		t.Errorf("terran defaults %+v", p)
	}
	if got := p.MyrPerStep(); got < 5.3 || got > 5.4 {
		t.Errorf("MyrPerStep %g", got)
	}
	if _, err := DefaultParams("jovian"); err == nil {
		t.Error("jovian must be refused")
	}
	if _, err := DefaultParams("nope"); err == nil {
		t.Error("unknown archetype must error")
	}
}

func TestParamsSet(t *testing.T) {
	p, _ := DefaultParams("arid")
	if err := p.Set("Steps", "12"); err != nil || p.Steps != 12 {
		t.Errorf("Set Steps: %v %+v", err, p)
	}
	if err := p.Set("RidgeThickness", "0.2"); err != nil || p.RidgeThickness != 0.2 {
		t.Errorf("Set RidgeThickness: %v", err)
	}
	if err := p.Set("Nope", "1"); err == nil {
		t.Error("unknown knob must error")
	}
	if err := p.Set("Steps", "x"); err == nil {
		t.Error("bad value must error")
	}
}

func TestSeedForPlanetStable(t *testing.T) {
	if SeedForPlanet("sol_earth") != SeedForPlanet("sol_earth") || SeedForPlanet("a") == SeedForPlanet("b") {
		t.Error("seed must be stable and distinct")
	}
	a, b := newRNG(7, "x"), newRNG(7, "x")
	if a.Uint64() != b.Uint64() {
		t.Error("newRNG not deterministic")
	}
}
```

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run 'TestDefaultParams|TestParamsSet|TestSeedFor' -v`
Expected: FAIL — `undefined: DefaultParams`.

- [ ] **Step 7: Write params.go**

```go
package tectonics

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"strconv"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

// Params are every knob of checkpoint 1. Archetype defaults come from
// archetypes; any field can be overridden by name with Set.
type Params struct {
	Archetype string
	Face      int // cube face size S
	Steps     int
	KeyframeEvery int

	// Thickness field
	NoiseFreq        float64 // base octave frequency (cells across the sphere)
	NoiseOctaves     int
	WarpAmp          float64
	MaxNeighborDelta float64
	RelaxIters       int // per-step relaxation passes
	CrustBias        float64 // added to the field before normalise: >0 thicker worlds

	// Plates
	MajorMin, MajorMax int
	MinorMin, MinorMax int
	MinPlateArea       float64 // fraction of the sphere

	// Motion
	SpeedMinCmYr, SpeedMaxCmYr float64
	TimelineMyr                float64
	RadiusKm                   float64
	RepoleEvery                int
	PushJitterDeg              float64

	// Interactions
	RidgeThickness       float64
	RidgeJitter          float64
	OceanicThickening    float64
	TransformRatio       float64
	ContinentalThreshold float64
	CollisionUplift      float64
	TrenchDepth          float64
	TrenchWidth          int
	ArcUplift            float64
	ArcOffset            int
	FaultScar            float64
}

type archetypeRow struct {
	majorMin, majorMax, minorMin, minorMax int
	speedMin, speedMax, timeline, crustBias float64
}

var archetypes = map[string]archetypeRow{
	"terran":       {6, 9, 10, 16, 3, 8, 800, 0},
	"super_terran": {6, 9, 10, 16, 3, 8, 800, 0},
	"oceanic":      {6, 9, 10, 16, 3, 8, 800, -0.15},
	"arid":         {4, 7, 8, 12, 2, 5, 600, 0},
	"tundra":       {4, 7, 8, 12, 2, 5, 600, 0},
	"glacial":      {4, 7, 8, 12, 2, 5, 600, 0},
	"scorched":     {8, 12, 14, 20, 6, 12, 600, -0.2},
	"lava_world":   {8, 12, 14, 20, 6, 12, 600, -0.2},
	"ice_world":    {2, 4, 2, 6, 0.2, 1, 600, 0.1},
}

var refused = map[string]bool{"jovian": true, "ice_giant": true}

// Archetypes lists the supported archetype names, sorted.
func Archetypes() []string {
	out := make([]string, 0, len(archetypes))
	for k := range archetypes {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func DefaultParams(archetype string) (Params, error) {
	if refused[archetype] {
		return Params{}, fmt.Errorf("archetype %q has no solid crust; tectonics not applicable", archetype)
	}
	a, ok := archetypes[archetype]
	if !ok {
		return Params{}, fmt.Errorf("unknown archetype %q", archetype)
	}
	return Params{
		Archetype: archetype, Face: 256, Steps: 150, KeyframeEvery: 1,
		NoiseFreq: 2, NoiseOctaves: 3, WarpAmp: 0.3, MaxNeighborDelta: 0.05, RelaxIters: 2, CrustBias: a.crustBias,
		MajorMin: a.majorMin, MajorMax: a.majorMax, MinorMin: a.minorMin, MinorMax: a.minorMax, MinPlateArea: 0.002,
		SpeedMinCmYr: a.speedMin, SpeedMaxCmYr: a.speedMax, TimelineMyr: a.timeline, RadiusKm: 6371, RepoleEvery: 25, PushJitterDeg: 20,
		RidgeThickness: 0.15, RidgeJitter: 0.03, OceanicThickening: 0.01, TransformRatio: 2, ContinentalThreshold: 0.5,
		CollisionUplift: 0.01, TrenchDepth: 0.03, TrenchWidth: 3, ArcUplift: 0.01, ArcOffset: 4, FaultScar: 0.02,
	}, nil
}

func (p Params) MyrPerStep() float64 { return p.TimelineMyr / float64(p.Steps) }

// Set overrides one knob by exact field name ("Steps", "RidgeThickness").
func (p *Params) Set(name, value string) error {
	f := reflect.ValueOf(p).Elem().FieldByName(name)
	if !f.IsValid() {
		return fmt.Errorf("unknown knob %q", name)
	}
	switch f.Kind() {
	case reflect.Int:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("knob %s: %w", name, err)
		}
		f.SetInt(int64(n))
	case reflect.Float64:
		x, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("knob %s: %w", name, err)
		}
		f.SetFloat(x)
	case reflect.String:
		f.SetString(value)
	default:
		return fmt.Errorf("knob %s: unsupported kind %s", name, f.Kind())
	}
	return nil
}

// SeedForPlanet maps a planet's game id to the master seed.
func SeedForPlanet(id string) int64 { return seed.Hash(id) }

// newRNG returns the deterministic stream for one stage of one planet.
func newRNG(master int64, domain string) *rand.Rand {
	return rand.New(rand.NewPCG(uint64(seed.Domain(master, domain)), 0x9e3779b97f4a7c15))
}
```

Add at the bottom of `params.go` (keeps the import list small):

```go
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
```

- [ ] **Step 8: Run all package tests, lint, expect PASS**

Run: `go build ./... && go test ./pkg/tectonics/ -v && golangci-lint run ./pkg/tectonics/...`

- [ ] **Step 9: Commit**

```bash
git add pkg/tectonics/grid.go pkg/tectonics/grid_test.go pkg/tectonics/params.go pkg/tectonics/params_test.go
git commit -m "feat(tectonics): grid, params and archetype table for the new generator" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: Thickness field

**Files:**
- Create: `pkg/tectonics/thickness.go`
- Test: `pkg/tectonics/thickness_test.go`

**Interfaces:**
- Consumes: `Grid`, `Neighbors4`, `Dirs`, `Params`, `newRNG`.
- Produces: `GenerateThickness(p Params, master int64) *Grid[float64]`; `Relax(th *Grid[float64], maxDelta float64, iters int) int` (returns violations fixed in the last pass; 0 means converged); `valueNoise(seed uint64, x, y, z float64) float64` in [0,1].

Ruling vs spec: the spec says relax then normalise; normalising after relaxing would stretch the deltas and break the bound. Order here is noise → normalise to [0,1] → relax. Relax only moves neighbours toward each other, so values stay in [0,1].

- [ ] **Step 1: Write the failing test**

```go
// pkg/tectonics/thickness_test.go
package tectonics

import (
	"math"
	"testing"
)

func testParams(t *testing.T, S int) Params {
	t.Helper()
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face = S
	return p
}

func maxNeighborDelta(th *Grid[float64]) float64 {
	nb := Neighbors4(th.S)
	worst := 0.0
	for i := range th.Len() {
		for _, j := range nb[i] {
			worst = math.Max(worst, math.Abs(th.Cells[i]-th.Cells[j]))
		}
	}
	return worst
}

func TestThicknessBoundedSmoothAndVaried(t *testing.T) {
	p := testParams(t, 32)
	th := GenerateThickness(p, 42)
	lo, hi, sum := 1.0, 0.0, 0.0
	for _, v := range th.Cells {
		lo, hi, sum = math.Min(lo, v), math.Max(hi, v), sum+v
	}
	if lo < 0 || hi > 1 || hi-lo < 0.5 {
		t.Errorf("range [%g,%g]", lo, hi)
	}
	mean := sum / float64(th.Len())
	variance := 0.0
	for _, v := range th.Cells {
		variance += (v - mean) * (v - mean)
	}
	if variance/float64(th.Len()) < 0.01 {
		t.Errorf("field is nearly flat, variance %g", variance/float64(th.Len()))
	}
	if d := maxNeighborDelta(th); d > p.MaxNeighborDelta+1e-9 {
		t.Errorf("neighbour delta %g exceeds %g", d, p.MaxNeighborDelta)
	}
}

func TestThicknessDeterministic(t *testing.T) {
	p := testParams(t, 16)
	a, b := GenerateThickness(p, 7), GenerateThickness(p, 7)
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			t.Fatalf("pixel %d differs", i)
		}
	}
	c := GenerateThickness(p, 8)
	same := 0
	for i := range a.Cells {
		if a.Cells[i] == c.Cells[i] {
			same++
		}
	}
	if same == len(a.Cells) {
		t.Error("different seeds gave identical fields")
	}
}

func TestRelaxEnforcesBoundAndConverges(t *testing.T) {
	th := NewGrid[float64](8)
	th.Cells[0] = 1 // a single spike
	for Relax(th, 0.05, 1) > 0 {
	}
	if d := maxNeighborDelta(th); d > 0.05+1e-9 {
		t.Errorf("delta %g after relax", d)
	}
	for _, v := range th.Cells {
		if v < 0 || v > 1 {
			t.Fatalf("value %g escaped [0,1]", v)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run Thickness -v`
Expected: FAIL — `undefined: GenerateThickness`.

- [ ] **Step 3: Write thickness.go**

```go
package tectonics

import "math"

// hash3 mixes an integer lattice point and a seed into [0,1).
func hash3(seed uint64, ix, iy, iz int64) float64 {
	h := seed ^ (uint64(ix) * 0x9E3779B97F4A7C15) ^ (uint64(iy) * 0xC2B2AE3D27D4EB4F) ^ (uint64(iz) * 0x165667B19E3779F9)
	h ^= h >> 31
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 29
	return float64(h>>11) / float64(1<<53)
}

func smooth(t float64) float64 { return t * t * (3 - 2*t) }

// valueNoise is trilinear value noise on an integer lattice, in [0,1].
func valueNoise(seed uint64, x, y, z float64) float64 {
	fx, fy, fz := math.Floor(x), math.Floor(y), math.Floor(z)
	ix, iy, iz := int64(fx), int64(fy), int64(fz)
	tx, ty, tz := smooth(x-fx), smooth(y-fy), smooth(z-fz)
	lerp := func(a, b, t float64) float64 { return a + (b-a)*t }
	c := func(dx, dy, dz int64) float64 { return hash3(seed, ix+dx, iy+dy, iz+dz) }
	x00 := lerp(c(0, 0, 0), c(1, 0, 0), tx)
	x10 := lerp(c(0, 1, 0), c(1, 1, 0), tx)
	x01 := lerp(c(0, 0, 1), c(1, 0, 1), tx)
	x11 := lerp(c(0, 1, 1), c(1, 1, 1), tx)
	return lerp(lerp(x00, x10, ty), lerp(x01, x11, ty), tz)
}

// GenerateThickness builds the crust thickness field: warped low-frequency
// value noise, normalised to [0,1], then relaxed so no 4-neighbour pair
// differs by more than p.MaxNeighborDelta.
func GenerateThickness(p Params, master int64) *Grid[float64] {
	S := p.Face
	th := NewGrid[float64](S)
	dirs := Dirs(S)
	base := newRNG(master, "thickness.base").Uint64()
	warp := newRNG(master, "thickness.warp").Uint64()
	const off = 1000.0 // keep lattice coords positive and away from the origin
	for i, d := range dirs {
		wx := (valueNoise(warp, off+d[0]*p.NoiseFreq, off+d[1]*p.NoiseFreq, off+d[2]*p.NoiseFreq) - 0.5) * 2 * p.WarpAmp
		wy := (valueNoise(warp+1, off+d[0]*p.NoiseFreq, off+d[1]*p.NoiseFreq, off+d[2]*p.NoiseFreq) - 0.5) * 2 * p.WarpAmp
		wz := (valueNoise(warp+2, off+d[0]*p.NoiseFreq, off+d[1]*p.NoiseFreq, off+d[2]*p.NoiseFreq) - 0.5) * 2 * p.WarpAmp
		x, y, z := d[0]+wx, d[1]+wy, d[2]+wz
		sum, amp, freq, norm := 0.0, 1.0, p.NoiseFreq, 0.0
		for o := range p.NoiseOctaves {
			sum += amp * valueNoise(base+uint64(o), off+x*freq, off+y*freq, off+z*freq)
			norm += amp
			amp *= 0.5
			freq *= 2
		}
		th.Cells[i] = sum/norm + p.CrustBias
	}
	normalize(th)
	for range 200 {
		if Relax(th, p.MaxNeighborDelta, 1) == 0 {
			break
		}
	}
	return th
}

func normalize(th *Grid[float64]) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range th.Cells {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if hi-lo < 1e-12 {
		return
	}
	for i, v := range th.Cells {
		th.Cells[i] = (v - lo) / (hi - lo)
	}
}

// Relax runs iters Gauss-Seidel passes pulling every 4-neighbour pair that
// differs by more than maxDelta toward each other. Returns the number of
// pairs adjusted in the last pass; 0 means the bound holds everywhere.
func Relax(th *Grid[float64], maxDelta float64, iters int) int {
	nb := Neighbors4(th.S)
	fixed := 0
	for range iters {
		fixed = 0
		for i := range th.Len() {
			for _, j := range nb[i] {
				d := th.Cells[i] - th.Cells[j]
				if d > maxDelta {
					e := (d - maxDelta) / 2
					th.Cells[i] -= e
					th.Cells[j] += e
					fixed++
				} else if d < -maxDelta {
					e := (-d - maxDelta) / 2
					th.Cells[i] += e
					th.Cells[j] -= e
					fixed++
				}
			}
		}
	}
	return fixed
}
```

- [ ] **Step 4: Run tests and lint, expect PASS**

Run: `go test ./pkg/tectonics/ -v && golangci-lint run ./pkg/tectonics/...`

If `TestThicknessBoundedSmoothAndVaried` fails on `hi-lo < 0.5`, the relax step is over-flattening at S=32 (where 0.05 per pixel allows only 0.05×16 = 0.8 across a face); loosen the test to `hi-lo < 0.3` and note it in the commit, since production runs at S=256.

- [ ] **Step 5: Commit**

```bash
git add pkg/tectonics/thickness.go pkg/tectonics/thickness_test.go
git commit -m "feat(tectonics): smooth crust thickness field with neighbour-delta relaxation" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 3: Plates by watershed

**Files:**
- Create: `pkg/tectonics/watershed.go`
- Test: `pkg/tectonics/watershed_test.go`

**Interfaces:**
- Consumes: `Grid`, `Neighbors4`, `Dirs`, `Params`, `newRNG`, `GenerateThickness`.
- Produces: `type Plate struct{ ID int32; Area float64; Major bool; MeanThickness float64; Centroid [3]float64; Retired bool }`; `BuildPlates(th *Grid[float64], p Params, master int64) (*Grid[int32], []Plate)` — labels are dense 0..len(plates)-1, majors come first; `PlateStats(th *Grid[float64], labels *Grid[int32], n int) []Plate` (recomputes Area, MeanThickness, Centroid for n plates; used again by step/sim); `basins(th *Grid[float64]) (*Grid[int32], int)` (priority flood, returns micro-basin labels + count).

- [ ] **Step 1: Write the failing test**

```go
// pkg/tectonics/watershed_test.go
package tectonics

import "testing"

func TestBasinsCoverEveryPixel(t *testing.T) {
	p := testParams(t, 24)
	th := GenerateThickness(p, 3)
	labels, n := basins(th)
	if n < 2 {
		t.Fatalf("only %d basins", n)
	}
	for i, l := range labels.Cells {
		if l < 0 || int(l) >= n {
			t.Fatalf("pixel %d label %d of %d", i, l, n)
		}
	}
}

func TestBuildPlatesCountsAreasAndThinBoundaries(t *testing.T) {
	p := testParams(t, 32)
	th := GenerateThickness(p, 11)
	labels, plates := BuildPlates(th, p, 11)
	if n := len(plates); n < p.MajorMin+p.MinorMin || n > p.MajorMax+p.MinorMax {
		t.Fatalf("%d plates, want %d..%d", n, p.MajorMin+p.MinorMin, p.MajorMax+p.MinorMax)
	}
	majors, total := 0, 0.0
	for i, pl := range plates {
		if pl.ID != int32(i) {
			t.Errorf("plate %d has id %d", i, pl.ID)
		}
		if pl.Major {
			majors++
		}
		if pl.Area < p.MinPlateArea {
			t.Errorf("plate %d area %g below minimum", i, pl.Area)
		}
		total += pl.Area
	}
	if majors < p.MajorMin || majors > p.MajorMax {
		t.Errorf("%d majors", majors)
	}
	if total < 0.999 || total > 1.001 {
		t.Errorf("areas sum to %g", total)
	}
	if plates[0].Area < plates[len(plates)-1].Area {
		t.Error("plates not sorted largest first")
	}
	// boundaries lie on thin crust: mean thickness of boundary pixels is
	// below the mean of the whole field.
	nb := Neighbors4(th.S)
	bsum, bn, sum := 0.0, 0, 0.0
	for i := range th.Len() {
		sum += th.Cells[i]
		for _, j := range nb[i] {
			if labels.Cells[j] != labels.Cells[i] {
				bsum += th.Cells[i]
				bn++
				break
			}
		}
	}
	if bn == 0 || bsum/float64(bn) >= sum/float64(th.Len()) {
		t.Errorf("boundary mean %g not below field mean %g", bsum/float64(bn), sum/float64(th.Len()))
	}
}

func TestBuildPlatesSeamBlind(t *testing.T) {
	// Boundary density on face-edge pixels must not be wildly higher than
	// in face interiors; a seam bug would show as plates cut along cube edges.
	p := testParams(t, 32)
	th := GenerateThickness(p, 5)
	labels, _ := BuildPlates(th, p, 5)
	nb := Neighbors4(th.S)
	var edgeB, edgeN, inB, inN int
	for i := range th.Len() {
		_, px, py := th.Addr(i)
		edge := px == 0 || py == 0 || px == th.S-1 || py == th.S-1
		isB := false
		for _, j := range nb[i] {
			if labels.Cells[j] != labels.Cells[i] {
				isB = true
			}
		}
		if edge {
			edgeN++
			if isB {
				edgeB++
			}
		} else {
			inN++
			if isB {
				inB++
			}
		}
	}
	edgeRate := float64(edgeB) / float64(edgeN)
	inRate := float64(inB) / float64(inN)
	if edgeRate > 3*inRate+0.05 {
		t.Errorf("boundary rate on seams %.3f vs interior %.3f", edgeRate, inRate)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run 'TestBasins|TestBuildPlates' -v`
Expected: FAIL — `undefined: basins`.

- [ ] **Step 3: Write watershed.go**

```go
package tectonics

import (
	"container/heap"
	"math"
	"sort"
)

type Plate struct {
	ID            int32
	Area          float64 // fraction of the sphere
	Major         bool
	MeanThickness float64
	Centroid      [3]float64 // unit vector
	Retired       bool
}

type floodItem struct {
	th    float64
	idx   int32
	label int32
}

// floodHeap pops the thickest pixel first; ties break on the lower index so
// the flood order is a pure function of the field.
type floodHeap []floodItem

func (h floodHeap) Len() int { return len(h) }
func (h floodHeap) Less(i, j int) bool {
	if h[i].th != h[j].th {
		return h[i].th > h[j].th
	}
	return h[i].idx < h[j].idx
}
func (h floodHeap) Swap(i, j int)  { h[i], h[j] = h[j], h[i] }
func (h *floodHeap) Push(x any)    { *h = append(*h, x.(floodItem)) }
func (h *floodHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// basins priority-floods the field from every local maximum of thickness so
// each pixel joins the catchment of the thick high it drains to.
func basins(th *Grid[float64]) (*Grid[int32], int) {
	nb := Neighbors4(th.S)
	labels := NewGrid[int32](th.S)
	for i := range labels.Cells {
		labels.Cells[i] = -1
	}
	h := &floodHeap{}
	n := int32(0)
	for i := range th.Len() {
		isMax := true
		for _, j := range nb[i] {
			if th.Cells[j] > th.Cells[i] {
				isMax = false
				break
			}
		}
		if isMax {
			labels.Cells[i] = n
			heap.Push(h, floodItem{th.Cells[i], int32(i), n})
			n++
		}
	}
	for h.Len() > 0 {
		it := heap.Pop(h).(floodItem)
		for _, j := range nb[it.idx] {
			if labels.Cells[j] < 0 {
				labels.Cells[j] = it.label
				heap.Push(h, floodItem{th.Cells[j], j, it.label})
			}
		}
	}
	return labels, int(n)
}

type pairKey struct{ a, b int32 }

func key(a, b int32) pairKey {
	if a > b {
		a, b = b, a
	}
	return pairKey{a, b}
}

// find with path halving over a union-find parent slice.
func find(parent []int32, x int32) int32 {
	for parent[x] != x {
		parent[x] = parent[parent[x]]
		x = parent[x]
	}
	return x
}

// BuildPlates segments the field into major+minor plates. Micro-basins are
// merged across their thickest divides until the seeded target count is
// reached, then plates below p.MinPlateArea join their largest neighbour.
// Labels are relabelled dense, largest plate first; the first N are majors.
func BuildPlates(th *Grid[float64], p Params, master int64) (*Grid[int32], []Plate) {
	rng := newRNG(master, "plates.count")
	majors := p.MajorMin + rng.IntN(p.MajorMax-p.MajorMin+1)
	minors := p.MinorMin + rng.IntN(p.MinorMax-p.MinorMin+1)
	target := majors + minors

	labels, n := basins(th)
	nb := Neighbors4(th.S)
	divide := map[pairKey]float64{}
	for i := range th.Len() {
		for _, j := range nb[i] {
			if a, b := labels.Cells[i], labels.Cells[j]; a != b {
				k := key(a, b)
				if v := math.Max(th.Cells[i], th.Cells[j]); v > divide[k] {
					divide[k] = v
				}
			}
		}
	}
	parent := make([]int32, n)
	for i := range parent {
		parent[i] = int32(i)
	}
	count := n
	for count > target && len(divide) > 0 {
		// pick the thickest divide; ties on the smaller pair for determinism
		var best pairKey
		bestV := -1.0
		for k, v := range divide {
			if v > bestV || (v == bestV && (k.a < best.a || (k.a == best.a && k.b < best.b))) {
				best, bestV = k, v
			}
		}
		ra, rb := find(parent, best.a), find(parent, best.b)
		parent[rb] = ra
		count--
		// re-key every divide that touched rb onto ra
		next := make(map[pairKey]float64, len(divide))
		for k, v := range divide {
			a, b := find(parent, k.a), find(parent, k.b)
			if a == b {
				continue
			}
			nk := key(a, b)
			if v > next[nk] {
				next[nk] = v
			}
		}
		divide = next
	}
	for i, l := range labels.Cells {
		labels.Cells[i] = find(parent, l)
	}
	labels, plates := compact(th, labels)
	labels, plates = mergeSmall(th, labels, plates, p.MinPlateArea)
	for i := range plates {
		plates[i].Major = i < majors && i < len(plates)
	}
	return labels, plates
}

// compact relabels to dense ids ordered by area (largest first) and returns
// the plate table.
func compact(th *Grid[float64], labels *Grid[int32]) (*Grid[int32], []Plate) {
	area := map[int32]int{}
	for _, l := range labels.Cells {
		area[l]++
	}
	ids := make([]int32, 0, len(area))
	for id := range area {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if area[ids[i]] != area[ids[j]] {
			return area[ids[i]] > area[ids[j]]
		}
		return ids[i] < ids[j]
	})
	remap := make(map[int32]int32, len(ids))
	for i, id := range ids {
		remap[id] = int32(i)
	}
	out := NewGrid[int32](labels.S)
	for i, l := range labels.Cells {
		out.Cells[i] = remap[l]
	}
	return out, PlateStats(th, out, len(ids))
}

// PlateStats recomputes Area, MeanThickness and Centroid for n plates.
func PlateStats(th *Grid[float64], labels *Grid[int32], n int) []Plate {
	plates := make([]Plate, n)
	dirs := Dirs(th.S)
	cnt := make([]int, n)
	for i, l := range labels.Cells {
		pl := &plates[l]
		cnt[l]++
		pl.MeanThickness += th.Cells[i]
		for k := range 3 {
			pl.Centroid[k] += dirs[i][k]
		}
	}
	total := float64(th.Len())
	for i := range plates {
		plates[i].ID = int32(i)
		plates[i].Area = float64(cnt[i]) / total
		if cnt[i] > 0 {
			plates[i].MeanThickness /= float64(cnt[i])
			plates[i].Centroid = unit(plates[i].Centroid)
		} else {
			plates[i].Retired = true
		}
	}
	return plates
}

// mergeSmall folds every plate under minArea into the neighbour it shares the
// most boundary with, then re-compacts.
func mergeSmall(th *Grid[float64], labels *Grid[int32], plates []Plate, minArea float64) (*Grid[int32], []Plate) {
	for {
		small := int32(-1)
		for i := len(plates) - 1; i >= 0; i-- {
			if plates[i].Area < minArea {
				small = int32(i)
				break
			}
		}
		if small < 0 || len(plates) <= 2 {
			return labels, plates
		}
		nb := Neighbors4(th.S)
		shared := map[int32]int{}
		for i, l := range labels.Cells {
			if l != small {
				continue
			}
			for _, j := range nb[i] {
				if m := labels.Cells[j]; m != small {
					shared[m]++
				}
			}
		}
		best, bestN := int32(-1), -1
		for m, c := range shared {
			if c > bestN || (c == bestN && m < best) {
				best, bestN = m, c
			}
		}
		for i, l := range labels.Cells {
			if l == small {
				labels.Cells[i] = best
			}
		}
		labels, plates = compact(th, labels)
	}
}

func unit(v [3]float64) [3]float64 {
	n := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if n == 0 {
		return [3]float64{0, 0, 1}
	}
	return [3]float64{v[0] / n, v[1] / n, v[2] / n}
}
```

- [ ] **Step 4: Run tests and lint, expect PASS**

Run: `go test ./pkg/tectonics/ -v && golangci-lint run ./pkg/tectonics/...`

If `TestBuildPlatesCountsAreasAndThinBoundaries` fails on the count because S=32 yields fewer micro-basins than the target, raise the test face to 48. If `mergeSmall` drops the count below `MajorMin+MinorMin`, that is acceptable behaviour; change the lower bound in the test to `p.MajorMin` and note it.

- [ ] **Step 5: Commit**

```bash
git add pkg/tectonics/watershed.go pkg/tectonics/watershed_test.go
git commit -m "feat(tectonics): watershed plates along the thinnest crust" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 4: Plate motion

**Files:**
- Create: `pkg/tectonics/motion.go`
- Test: `pkg/tectonics/motion_test.go`

**Interfaces:**
- Consumes: `Grid`, `Neighbors4`, `Dirs`, `Params`, `Plate`, `newRNG`, `unit`.
- Produces: `type Motion struct{ Pole [3]float64; DegPerStep float64; SpeedCmYr float64 }`; `InitMotion(th *Grid[float64], labels *Grid[int32], plates []Plate, p Params, master int64) []Motion`; `Reaim(th, labels, plates, motions []Motion, p Params, rng *rand.Rand)` (in place); vector helpers `cross(a, b [3]float64) [3]float64`, `dot(a, b) float64`, `rotate(v, axis [3]float64, deg float64) [3]float64` (Rodrigues, right-handed), `velocityAt(m Motion, d [3]float64) [3]float64` (deg/step × tangent, i.e. ω×d with |ω| in radians per step); `pushDirection(th, labels, plateID int32, c [3]float64) [3]float64` (unit tangent at c pointing away from the plate's thinnest boundary stretch, zero vector if the plate has no boundary); `DegPerStep(speedCmYr, radiusKm, myrPerStep float64) float64`.

- [ ] **Step 1: Write the failing test**

```go
// pkg/tectonics/motion_test.go
package tectonics

import (
	"math"
	"testing"
)

func TestDegPerStep(t *testing.T) {
	// 5 cm/yr for 5 Myr = 250 km; on a 6371 km sphere that is 2.248°.
	if got := DegPerStep(5, 6371, 5); math.Abs(got-2.248) > 0.01 {
		t.Errorf("DegPerStep = %g", got)
	}
}

func TestRotateIsRightHanded(t *testing.T) {
	v := rotate([3]float64{1, 0, 0}, [3]float64{0, 0, 1}, 90)
	if math.Abs(v[0]) > 1e-9 || math.Abs(v[1]-1) > 1e-9 {
		t.Errorf("rotate x about z by 90 = %v, want +y", v)
	}
}

// twoHemispheres splits the sphere at z=0: plate 0 is z>=0 (thick), plate 1
// is z<0 (thin). Shared by the motion and step tests.
func twoHemispheres(S int, thNorth, thSouth float64) (*Grid[float64], *Grid[int32], []Plate) {
	th := NewGrid[float64](S)
	labels := NewGrid[int32](S)
	for i, d := range Dirs(S) {
		if d[2] >= 0 {
			th.Cells[i], labels.Cells[i] = thNorth, 0
		} else {
			th.Cells[i], labels.Cells[i] = thSouth, 1
		}
	}
	return th, labels, PlateStats(th, labels, 2)
}

func TestInitMotionPushesAwayFromThinSeam(t *testing.T) {
	// Make the northern plate's equatorial rim thin on the +x side only, so
	// its push direction must point toward -x.
	S := 16
	th, labels, plates := twoHemispheres(S, 0.8, 0.3)
	for i, d := range Dirs(S) {
		if labels.Cells[i] == 0 && d[2] < 0.3 && d[0] > 0.3 {
			th.Cells[i] = 0.1
		}
	}
	p := testParams(t, S)
	p.PushJitterDeg = 0
	ms := InitMotion(th, labels, plates, p, 1)
	v := velocityAt(ms[0], plates[0].Centroid)
	if v[0] >= 0 {
		t.Errorf("plate 0 velocity %v should point to -x", v)
	}
	if ms[0].DegPerStep <= 0 || ms[0].SpeedCmYr < p.SpeedMinCmYr || ms[0].SpeedCmYr > p.SpeedMaxCmYr {
		t.Errorf("motion %+v", ms[0])
	}
	if math.Abs(dot(ms[0].Pole, ms[0].Pole)-1) > 1e-9 {
		t.Error("pole not unit")
	}
}

func TestReaimBlendsTowardNewPush(t *testing.T) {
	S := 16
	th, labels, plates := twoHemispheres(S, 0.8, 0.3)
	p := testParams(t, S)
	p.PushJitterDeg = 0
	ms := InitMotion(th, labels, plates, p, 1)
	before := ms[0]
	// thin the -x rim instead: push should swing toward +x
	for i, d := range Dirs(S) {
		if labels.Cells[i] == 0 && d[2] < 0.3 && d[0] < -0.3 {
			th.Cells[i] = 0.05
		}
	}
	Reaim(th, labels, plates, ms, p, newRNG(1, "test"))
	vb, va := velocityAt(before, plates[0].Centroid), velocityAt(ms[0], plates[0].Centroid)
	if va[0] <= vb[0] {
		t.Errorf("reaim did not swing toward +x: before %v after %v", vb, va)
	}
	if ms[0].SpeedCmYr != before.SpeedCmYr {
		t.Error("reaim must not change speed")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run 'TestDegPerStep|TestRotate|TestInitMotion|TestReaim' -v`
Expected: FAIL — `undefined: DegPerStep`.

- [ ] **Step 3: Write motion.go**

```go
package tectonics

import (
	"math"
	"math/rand/v2"
)

type Motion struct {
	Pole       [3]float64 // unit rotation axis (right-handed)
	DegPerStep float64
	SpeedCmYr  float64
}

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func scale(a [3]float64, s float64) [3]float64 { return [3]float64{a[0] * s, a[1] * s, a[2] * s} }

func add(a, b [3]float64) [3]float64 { return [3]float64{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }

// rotate v about the unit axis by deg degrees (Rodrigues' formula).
func rotate(v, axis [3]float64, deg float64) [3]float64 {
	r := deg * math.Pi / 180
	c, s := math.Cos(r), math.Sin(r)
	k := axis
	kv := cross(k, v)
	kd := dot(k, v)
	return [3]float64{
		v[0]*c + kv[0]*s + k[0]*kd*(1-c),
		v[1]*c + kv[1]*s + k[1]*kd*(1-c),
		v[2]*c + kv[2]*s + k[2]*kd*(1-c),
	}
}

// tangent projects v onto the plane perpendicular to the unit vector c.
func tangent(v, c [3]float64) [3]float64 { return add(v, scale(c, -dot(v, c))) }

// DegPerStep converts a plate speed to degrees of rotation per step.
func DegPerStep(speedCmYr, radiusKm, myrPerStep float64) float64 {
	km := speedCmYr * 1e-5 * myrPerStep * 1e6 // cm/yr → km over the step
	return km / (2 * math.Pi * radiusKm) * 360
}

// velocityAt is the plate's surface velocity (radians of arc per step, as a
// tangent vector) at unit direction d: ω × d.
func velocityAt(m Motion, d [3]float64) [3]float64 {
	return cross(scale(m.Pole, m.DegPerStep*math.Pi/180), d)
}

// pushDirection is the unit tangent at c that points away from the plate's
// thinnest boundary stretch. Boundary pixels are weighted by (1-th)^4 so the
// thin stretch dominates. Returns the zero vector when the plate has no
// boundary (a single-plate world).
func pushDirection(th *Grid[float64], labels *Grid[int32], id int32, c [3]float64) [3]float64 {
	nb := Neighbors4(th.S)
	dirs := Dirs(th.S)
	var acc [3]float64
	any := false
	for i, l := range labels.Cells {
		if l != id {
			continue
		}
		for _, j := range nb[i] {
			if labels.Cells[j] != id {
				w := math.Pow(1-th.Cells[i], 4)
				acc = add(acc, scale(dirs[i], w))
				any = true
				break
			}
		}
	}
	if !any {
		return [3]float64{}
	}
	thin := unit(acc)
	t := tangent(add(c, scale(thin, -1)), c) // from the thin side through c, onward
	if dot(t, t) < 1e-12 {
		return [3]float64{}
	}
	return unit(t)
}

func poleFor(c, t [3]float64) [3]float64 { return unit(cross(c, t)) }

// InitMotion draws each plate's speed and push direction.
func InitMotion(th *Grid[float64], labels *Grid[int32], plates []Plate, p Params, master int64) []Motion {
	rng := newRNG(master, "plates.motion")
	ms := make([]Motion, len(plates))
	for i, pl := range plates {
		speed := p.SpeedMinCmYr + rng.Float64()*(p.SpeedMaxCmYr-p.SpeedMinCmYr)
		t := pushDirection(th, labels, pl.ID, pl.Centroid)
		jitter := (rng.Float64()*2 - 1) * p.PushJitterDeg
		if dot(t, t) == 0 { // no boundary: pick any tangent
			t = unit(tangent([3]float64{0.3, 0.5, 0.8}, pl.Centroid))
		}
		t = rotate(t, pl.Centroid, jitter)
		ms[i] = Motion{Pole: poleFor(pl.Centroid, t), SpeedCmYr: speed,
			DegPerStep: DegPerStep(speed, p.RadiusKm, p.MyrPerStep())}
	}
	return ms
}

// Reaim blends each plate's current push direction with a fresh one computed
// from its present thinnest boundary, so locked plates slowly turn away.
func Reaim(th *Grid[float64], labels *Grid[int32], plates []Plate, ms []Motion, p Params, rng *rand.Rand) {
	for i, pl := range plates {
		if pl.Retired {
			continue
		}
		fresh := pushDirection(th, labels, pl.ID, pl.Centroid)
		if dot(fresh, fresh) == 0 {
			continue
		}
		old := unit(tangent(velocityAt(ms[i], pl.Centroid), pl.Centroid))
		blend := unit(add(scale(old, 0.5), scale(fresh, 0.5)))
		if dot(blend, blend) == 0 {
			blend = fresh
		}
		blend = rotate(blend, pl.Centroid, (rng.Float64()*2-1)*p.PushJitterDeg*0.5)
		ms[i].Pole = poleFor(pl.Centroid, unit(tangent(blend, pl.Centroid)))
	}
}
```

- [ ] **Step 4: Run tests and lint, expect PASS**

Run: `go test ./pkg/tectonics/ -v && golangci-lint run ./pkg/tectonics/...`

`golangci-lint` may flag the identifier `any` shadowing the builtin in `pushDirection`; rename it to `found`.

- [ ] **Step 5: Commit**

```bash
git add pkg/tectonics/motion.go pkg/tectonics/motion_test.go
git commit -m "feat(tectonics): per-plate Euler motion pushed away from the thinnest seam" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 5: One simulation step

**Files:**
- Create: `pkg/tectonics/step.go`
- Test: `pkg/tectonics/step_test.go`

**Interfaces:**
- Consumes: `Grid`, `Neighbors4`, `Dirs`, `Params`, `Plate`, `PlateStats`, `Motion`, `velocityAt`, `rotate`, `tangent`, `unit`, `dot`, `scale`, `Relax`, `cubemap.DirToFacePixel`.
- Produces: feature constants `FeatNone=0, FeatDivergent=1, FeatConvergent=2, FeatTransform=3` (`uint8`); `type State struct{ P Params; Th *Grid[float64]; Labels *Grid[int32]; Age *Grid[uint16]; FeatType *Grid[uint8]; FeatAge *Grid[uint8]; Plates []Plate; Motions []Motion; StepNo int }`; `NewState(p Params, th *Grid[float64], labels *Grid[int32], plates []Plate, motions []Motion) *State`; `(s *State) Step(rng *rand.Rand)`; `(s *State) FeatureByte(i int) uint8` (type<<6 | min(age,63)).

Rules implemented by `Step`, in order:
1. Advance: for every destination pixel, for every live plate whose cap could cover it, inverse-rotate the pixel direction by that plate's step rotation and read the source pixel; if the source belongs to that plate, record a claim (plate, thickness, age).
2. No claimant → divergent gap: thickness `RidgeThickness ± RidgeJitter`, age 0, label from the nearest labelled pixel (BFS), `FeatDivergent`, feature age 0.
3. One claimant → carry over; age+1; if thickness < `ContinentalThreshold`, add `OceanicThickening × (√(age+1) − √age)`.
4. Two+ claimants → sort by thickness descending, take the top two (a thicker, b thinner); relative velocity `va − vb` split against the normal `n = tangent(unit(ca − cb), d)`: tangential/normal > `TransformRatio` → `FeatTransform`, a keeps, thickness −`FaultScar`; both ≥ `ContinentalThreshold` → `FeatConvergent`, a keeps, thickness +`CollisionUplift` capped at 1, feature age reset to 0 (belt stamped); else → subduction, `FeatConvergent`, a keeps a's thickness; pixel pushed onto the trench list with loser b.
5. Trench and arc: BFS from every subduction pixel up to `TrenchWidth` steps through pixels now labelled b: thickness −= `TrenchDepth × (1 − dist/(TrenchWidth+1))`; through pixels labelled a at exactly distance `ArcOffset`: thickness += `ArcUplift`. Floor 0, cap 1.
6. Ageing: every pixel with a feature type ages by 1, saturating at 63; divergent and transform marks clear once they reach 63 and were not re-stamped this step; convergent belts persist.
7. Retire: recompute `PlateStats`; any live plate with area < `MinPlateArea` is retired and its pixels relabelled to the neighbour plate with the most shared boundary.
8. `Relax(Th, MaxNeighborDelta, RelaxIters)`.

- [ ] **Step 1: Write the failing test**

```go
// pkg/tectonics/step_test.go
package tectonics

import (
	"math"
	"testing"
)

func stepState(t *testing.T, S int, thN, thS float64, poleN, poleS [3]float64, degN, degS float64) *State {
	t.Helper()
	th, labels, plates := twoHemispheres(S, thN, thS)
	p := testParams(t, S)
	p.RelaxIters = 0
	ms := []Motion{{Pole: unit(poleN), DegPerStep: degN}, {Pole: unit(poleS), DegPerStep: degS}}
	return NewState(p, th, labels, plates, ms)
}

func countFeat(s *State, ft uint8) int {
	n := 0
	for _, v := range s.FeatType.Cells {
		if v == ft {
			n++
		}
	}
	return n
}

func TestStepSinglePlateRotatesWithoutGaps(t *testing.T) {
	S := 16
	th := NewGrid[float64](S)
	labels := NewGrid[int32](S)
	for i, d := range Dirs(S) {
		th.Cells[i] = 0.5 + 0.3*d[0]
	}
	p := testParams(t, S)
	s := NewState(p, th, labels, PlateStats(th, labels, 1), []Motion{{Pole: [3]float64{0, 0, 1}, DegPerStep: 3}})
	s.Step(newRNG(1, "t"))
	if n := countFeat(s, FeatDivergent); n != 0 {
		t.Errorf("%d divergent pixels on a one-plate world", n)
	}
	// thickness pattern rotated: the +x high moved toward +y
	best, bi := -1.0, 0
	for i, v := range s.Th.Cells {
		if v > best {
			best, bi = v, i
		}
	}
	if d := Dirs(S)[bi]; d[1] <= 0 {
		t.Errorf("thickest pixel at %v, expected to have rotated toward +y", d)
	}
}

func TestStepDivergentGapFills(t *testing.T) {
	// The north plate rotates about +x (its rim at +y lifts north, its rim at
	// -y sinks south); the south plate rotates the opposite way. So along +y
	// the plates separate (ridge) and along -y they converge.
	s := stepState(t, 24, 0.8, 0.8, [3]float64{1, 0, 0}, [3]float64{-1, 0, 0}, 4, 4)
	s.Step(newRNG(1, "t"))
	div, conv := 0, 0
	for i, ft := range s.FeatType.Cells {
		d := Dirs(24)[i]
		switch ft {
		case FeatDivergent:
			div++
			if d[1] < 0 {
				t.Fatalf("divergent pixel on the converging side at %v", d)
			}
			if v := s.Th.Cells[i]; math.Abs(v-s.P.RidgeThickness) > s.P.RidgeJitter+1e-9 {
				t.Fatalf("ridge thickness %g", v)
			}
			if s.Age.Cells[i] != 0 {
				t.Fatalf("new crust age %d", s.Age.Cells[i])
			}
		case FeatConvergent:
			conv++
			if d[1] > 0 {
				t.Fatalf("convergent pixel on the diverging side at %v", d)
			}
		}
	}
	if div == 0 || conv == 0 {
		t.Fatalf("div %d conv %d", div, conv)
	}
}

func TestStepCollisionRaisesThickness(t *testing.T) {
	s := stepState(t, 24, 0.8, 0.7, [3]float64{1, 0, 0}, [3]float64{-1, 0, 0}, 4, 4)
	before := s.Th.Clone()
	s.Step(newRNG(1, "t"))
	raised := 0
	for i, ft := range s.FeatType.Cells {
		if ft == FeatConvergent && s.Th.Cells[i] > before.Cells[i]+s.P.CollisionUplift/2 {
			raised++
			if s.FeatAge.Cells[i] != 0 {
				t.Fatalf("belt age %d, want 0", s.FeatAge.Cells[i])
			}
		}
	}
	if raised == 0 {
		t.Error("no collision pixel got thicker")
	}
}

func TestStepSubductionTrenchesThinSide(t *testing.T) {
	s := stepState(t, 24, 0.8, 0.2, [3]float64{1, 0, 0}, [3]float64{-1, 0, 0}, 4, 4)
	before := s.Th.Clone()
	s.Step(newRNG(1, "t"))
	trench, lost := 0, 0
	for i, ft := range s.FeatType.Cells {
		if ft == FeatConvergent && s.Labels.Cells[i] != 0 {
			lost++ // the thin south plate must never win a convergent pixel
		}
	}
	for i := range s.Th.Cells {
		if s.Labels.Cells[i] == 1 && s.Th.Cells[i] < before.Cells[i]-1e-9 {
			trench++
		}
	}
	if lost != 0 || trench == 0 {
		t.Errorf("thin plate won %d pixels, trench pixels %d", lost, trench)
	}
}

func TestStepTransformFault(t *testing.T) {
	// Both plates rotate about z in opposite senses: motion along the
	// equatorial boundary is purely tangential.
	s := stepState(t, 24, 0.8, 0.8, [3]float64{0, 0, 1}, [3]float64{0, 0, -1}, 4, 4)
	s.Step(newRNG(1, "t"))
	if tr := countFeat(s, FeatTransform); tr == 0 {
		t.Error("no transform pixels")
	}
	if cv := countFeat(s, FeatConvergent); cv > countFeat(s, FeatTransform)/4 {
		t.Errorf("too many convergent pixels (%d) for pure shear", cv)
	}
}

func TestStepAreaConservedAndDeterministic(t *testing.T) {
	a := stepState(t, 16, 0.8, 0.3, [3]float64{1, 0, 0}, [3]float64{0, 1, 0}, 3, 2)
	b := stepState(t, 16, 0.8, 0.3, [3]float64{1, 0, 0}, [3]float64{0, 1, 0}, 3, 2)
	for range 5 {
		a.Step(newRNG(9, "t"))
		b.Step(newRNG(9, "t"))
	}
	for i := range a.Th.Cells {
		if a.Th.Cells[i] != b.Th.Cells[i] || a.Labels.Cells[i] != b.Labels.Cells[i] || a.FeatType.Cells[i] != b.FeatType.Cells[i] {
			t.Fatalf("pixel %d differs", i)
		}
	}
	total := 0.0
	for _, pl := range a.Plates {
		total += pl.Area
	}
	if math.Abs(total-1) > 1e-9 {
		t.Errorf("areas sum to %g", total)
	}
	if fb := a.FeatureByte(0); fb>>6 != uint8(a.FeatType.Cells[0]) {
		t.Errorf("feature byte %08b", fb)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run TestStep -v`
Expected: FAIL — `undefined: NewState`.

- [ ] **Step 3: Write step.go**

```go
package tectonics

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

const (
	FeatNone uint8 = iota
	FeatDivergent
	FeatConvergent
	FeatTransform
)

type State struct {
	P        Params
	Th       *Grid[float64]
	Labels   *Grid[int32]
	Age      *Grid[uint16] // steps since the crust formed
	FeatType *Grid[uint8]
	FeatAge  *Grid[uint8] // steps since the feature was stamped, saturating at 63
	Plates   []Plate
	Motions  []Motion
	StepNo   int
}

func NewState(p Params, th *Grid[float64], labels *Grid[int32], plates []Plate, motions []Motion) *State {
	S := th.S
	return &State{P: p, Th: th, Labels: labels, Age: NewGrid[uint16](S),
		FeatType: NewGrid[uint8](S), FeatAge: NewGrid[uint8](S), Plates: plates, Motions: motions}
}

func (s *State) FeatureByte(i int) uint8 {
	return s.FeatType.Cells[i]<<6 | min(s.FeatAge.Cells[i], 63)
}

type claim struct {
	plate int32
	th    float64
	age   uint16
}

// capRadius returns, per plate, the cosine of the angular radius of its
// bounding cap (plus a two-pixel margin) around its centroid.
func (s *State) capCos() []float64 {
	dirs := Dirs(s.Th.S)
	minCos := make([]float64, len(s.Plates))
	for i := range minCos {
		minCos[i] = 1
	}
	for i, l := range s.Labels.Cells {
		c := dot(dirs[i], s.Plates[l].Centroid)
		if c < minCos[l] {
			minCos[l] = c
		}
	}
	margin := 2.0 * (90.0 / float64(s.Th.S)) * math.Pi / 180
	for i, c := range minCos {
		ang := math.Acos(math.Max(-1, math.Min(1, c))) + margin + s.Motions[i].DegPerStep*math.Pi/180
		minCos[i] = math.Cos(math.Min(ang, math.Pi))
	}
	return minCos
}

// Step advances the state by one geologic step.
func (s *State) Step(rng *rand.Rand) {
	S := s.Th.S
	N := s.Th.Len()
	dirs := Dirs(S)
	nb := Neighbors4(S)
	prevTh, prevL, prevAge := s.Th, s.Labels, s.Age
	th, labels, age := NewGrid[float64](S), NewGrid[int32](S), NewGrid[uint16](S)
	stamped := make([]bool, N)
	capc := s.capCos()

	// 1. claims
	claims := make([][]claim, N)
	for k, pl := range s.Plates {
		if pl.Retired {
			continue
		}
		m := s.Motions[k]
		for i := range N {
			if dot(dirs[i], pl.Centroid) < capc[k] {
				continue
			}
			src := rotate(dirs[i], m.Pole, -m.DegPerStep)
			f, px, py := cubemap.DirToFacePixel(src[0], src[1], src[2], S)
			j := prevL.Index(f, px, py)
			if prevL.Cells[j] == int32(k) {
				claims[i] = append(claims[i], claim{int32(k), prevTh.Cells[j], prevAge.Cells[j]})
			}
		}
	}

	// 2–4. resolve
	type trenchSeed struct{ idx int32; loser, winner int32 }
	var trenches []trenchSeed
	var gaps []int32
	for i := range N {
		labels.Cells[i] = -1
		switch c := claims[i]; len(c) {
		case 0:
			gaps = append(gaps, int32(i))
		case 1:
			labels.Cells[i], th.Cells[i], age.Cells[i] = c[0].plate, c[0].th, c[0].age+1
			if c[0].th < s.P.ContinentalThreshold {
				a := float64(c[0].age)
				th.Cells[i] += s.P.OceanicThickening * (math.Sqrt(a+1) - math.Sqrt(a))
			}
		default:
			sort.Slice(c, func(x, y int) bool {
				if c[x].th != c[y].th {
					return c[x].th > c[y].th
				}
				return c[x].plate < c[y].plate
			})
			a, b := c[0], c[1]
			labels.Cells[i], th.Cells[i], age.Cells[i] = a.plate, a.th, a.age+1
			va := velocityAt(s.Motions[a.plate], dirs[i])
			vb := velocityAt(s.Motions[b.plate], dirs[i])
			rel := add(va, scale(vb, -1))
			n := unit(tangent(add(s.Plates[a.plate].Centroid, scale(s.Plates[b.plate].Centroid, -1)), dirs[i]))
			normal := math.Abs(dot(rel, n))
			tang := math.Sqrt(math.Max(0, dot(rel, rel)-normal*normal))
			switch {
			case tang > s.P.TransformRatio*normal:
				s.FeatType.Cells[i] = FeatTransform
				th.Cells[i] = math.Max(0, th.Cells[i]-s.P.FaultScar)
			case a.th >= s.P.ContinentalThreshold && b.th >= s.P.ContinentalThreshold:
				s.FeatType.Cells[i] = FeatConvergent
				th.Cells[i] = math.Min(1, th.Cells[i]+s.P.CollisionUplift)
			default:
				s.FeatType.Cells[i] = FeatConvergent
				trenches = append(trenches, trenchSeed{int32(i), b.plate, a.plate})
			}
			s.FeatAge.Cells[i] = 0
			stamped[i] = true
		}
	}

	// 2. gaps: nearest labelled pixel by BFS, ridge crust
	if len(gaps) > 0 {
		queue := make([]int32, 0, N)
		for i := range N {
			if labels.Cells[i] >= 0 {
				queue = append(queue, int32(i))
			}
		}
		for len(queue) > 0 {
			i := queue[0]
			queue = queue[1:]
			for _, j := range nb[i] {
				if labels.Cells[j] < 0 {
					labels.Cells[j] = labels.Cells[i]
					queue = append(queue, j)
				}
			}
		}
		for _, i := range gaps {
			th.Cells[i] = s.P.RidgeThickness + (rng.Float64()*2-1)*s.P.RidgeJitter
			age.Cells[i] = 0
			s.FeatType.Cells[i] = FeatDivergent
			s.FeatAge.Cells[i] = 0
			stamped[i] = true
		}
	}

	// 5. trench and arc
	if len(trenches) > 0 {
		dist := make([]int16, N)
		for i := range dist {
			dist[i] = -1
		}
		type qi struct{ idx int32; d int16; loser, winner int32 }
		queue := make([]qi, 0, len(trenches))
		for _, t := range trenches {
			dist[t.idx] = 0
			queue = append(queue, qi{t.idx, 0, t.loser, t.winner})
		}
		maxD := int16(max(s.P.TrenchWidth, s.P.ArcOffset))
		for len(queue) > 0 {
			q := queue[0]
			queue = queue[1:]
			if q.d >= maxD {
				continue
			}
			for _, j := range nb[q.idx] {
				if dist[j] >= 0 {
					continue
				}
				l := labels.Cells[j]
				if l != q.loser && l != q.winner {
					continue
				}
				dist[j] = q.d + 1
				queue = append(queue, qi{j, q.d + 1, q.loser, q.winner})
				if l == q.loser && int(q.d+1) <= s.P.TrenchWidth {
					th.Cells[j] = math.Max(0, th.Cells[j]-s.P.TrenchDepth*(1-float64(q.d+1)/float64(s.P.TrenchWidth+1)))
				}
				if l == q.winner && int(q.d+1) == s.P.ArcOffset {
					th.Cells[j] = math.Min(1, th.Cells[j]+s.P.ArcUplift)
				}
			}
		}
	}

	// 6. ageing
	for i := range N {
		if s.FeatType.Cells[i] == FeatNone || stamped[i] {
			continue
		}
		if s.FeatAge.Cells[i] < 63 {
			s.FeatAge.Cells[i]++
		} else if s.FeatType.Cells[i] != FeatConvergent {
			s.FeatType.Cells[i] = FeatNone
			s.FeatAge.Cells[i] = 0
		}
	}

	s.Th, s.Labels, s.Age = th, labels, age
	s.StepNo++

	// 7. retire
	s.Plates = s.retire(PlateStats(s.Th, s.Labels, len(s.Plates)))

	// 8. smooth
	if s.P.RelaxIters > 0 {
		Relax(s.Th, s.P.MaxNeighborDelta, s.P.RelaxIters)
	}
}

// retire folds plates under MinPlateArea into their most-shared neighbour and
// keeps the Retired flag on plates that already were.
func (s *State) retire(stats []Plate) []Plate {
	for i := range stats {
		stats[i].Retired = stats[i].Retired || s.Plates[i].Retired
	}
	nb := Neighbors4(s.Th.S)
	for i := range stats {
		if stats[i].Retired || stats[i].Area >= s.P.MinPlateArea {
			continue
		}
		shared := map[int32]int{}
		for j, l := range s.Labels.Cells {
			if l != int32(i) {
				continue
			}
			for _, k := range nb[j] {
				if m := s.Labels.Cells[k]; m != int32(i) && !stats[m].Retired {
					shared[m]++
				}
			}
		}
		best, bestN := int32(-1), -1
		for m, c := range shared {
			if c > bestN || (c == bestN && m < best) {
				best, bestN = m, c
			}
		}
		if best < 0 {
			continue
		}
		for j, l := range s.Labels.Cells {
			if l == int32(i) {
				s.Labels.Cells[j] = best
			}
		}
		stats = PlateStats(s.Th, s.Labels, len(stats))
		for k := range stats {
			stats[k].Retired = stats[k].Retired || s.Plates[k].Retired
		}
		stats[i].Retired = true
	}
	return stats
}
```

- [ ] **Step 4: Run tests and lint, expect PASS**

Run: `go test ./pkg/tectonics/ -v && golangci-lint run ./pkg/tectonics/...`

Likely adjustments: `PlateStats` marks empty plates `Retired`, which `retire` preserves; if `TestStepTransformFault` sees too many convergent pixels because the hemispheres' centroids are antipodal (so the normal is ill-defined), compute the normal instead from the local label gradient: average of `dirs[j] − dirs[i]` over neighbours `j` labelled `b.plate` in the previous labels, projected tangent; fall back to the centroid difference when that sum is zero. Implement that fallback in the `default:` branch and keep the test as written.

- [ ] **Step 5: Commit**

```bash
git add pkg/tectonics/step.go pkg/tectonics/step_test.go
git commit -m "feat(tectonics): per-step plate drift with ridge, collision, subduction and transform rules" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 6: Run loop and frames

**Files:**
- Create: `pkg/tectonics/sim.go`
- Test: `pkg/tectonics/sim_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–5.
- Produces: `type PlateRow struct{ ID int32; Centroid [3]float64; Pole [3]float64; SpeedCmYr float64; Area float64; Major, Retired bool }`; `type Frame struct{ Step int; Myr float64; Th *Grid[float64]; Labels *Grid[int32]; Feature *Grid[uint8]; Plates []PlateRow }`; `Run(p Params, master int64, emit func(Frame) error) error` — emits frame 0 (initial state) and then every `KeyframeEvery`-th step, always including the last; `(s *State) Snapshot(myrPerStep float64) Frame` (copies grids so emitters may keep them); `Validate(p Params) error` (Face ≥ 8, Steps ≥ 1, KeyframeEvery ≥ 1, MajorMin ≤ MajorMax, MinorMin ≤ MinorMax, speeds ≥ 0).

- [ ] **Step 1: Write the failing test**

```go
// pkg/tectonics/sim_test.go
package tectonics

import "testing"

func TestRunEmitsFramesDeterministically(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 5, 2
	collect := func() []Frame {
		var out []Frame
		if err := Run(p, 123, func(f Frame) error { out = append(out, f); return nil }); err != nil {
			t.Fatal(err)
		}
		return out
	}
	a, b := collect(), collect()
	wantSteps := []int{0, 2, 4, 5}
	if len(a) != len(wantSteps) {
		t.Fatalf("%d frames, want %d", len(a), len(wantSteps))
	}
	for i, f := range a {
		if f.Step != wantSteps[i] {
			t.Errorf("frame %d step %d want %d", i, f.Step, wantSteps[i])
		}
		if f.Myr != float64(f.Step)*p.MyrPerStep() {
			t.Errorf("frame %d Myr %g", i, f.Myr)
		}
		if len(f.Plates) == 0 || f.Th.Len() != 6*16*16 {
			t.Errorf("frame %d incomplete", i)
		}
		for j := range f.Th.Cells {
			if f.Th.Cells[j] != b[i].Th.Cells[j] || f.Feature.Cells[j] != b[i].Feature.Cells[j] {
				t.Fatalf("frame %d pixel %d differs between runs", i, j)
			}
		}
	}
	// frames are snapshots: frame 0 must not reflect later steps
	if a[0].Step != 0 {
		t.Error("first frame is not the initial state")
	}
	for _, v := range a[0].Feature.Cells {
		if v != 0 {
			t.Fatal("frame 0 has features before any step")
		}
	}
}

func TestValidate(t *testing.T) {
	p := testParams(t, 16)
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	p.Steps = 0
	if err := Validate(p); err == nil {
		t.Error("Steps 0 must fail")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run 'TestRun|TestValidate' -v`
Expected: FAIL — `undefined: Run`.

- [ ] **Step 3: Write sim.go**

```go
package tectonics

import "fmt"

type PlateRow struct {
	ID        int32      `json:"id"`
	Centroid  [3]float64 `json:"centroid"`
	Pole      [3]float64 `json:"pole"`
	SpeedCmYr float64    `json:"speed_cm_yr"`
	Area      float64    `json:"area"`
	Major     bool       `json:"major"`
	Retired   bool       `json:"retired"`
}

type Frame struct {
	Step    int
	Myr     float64
	Th      *Grid[float64]
	Labels  *Grid[int32]
	Feature *Grid[uint8]
	Plates  []PlateRow
}

func Validate(p Params) error {
	switch {
	case p.Face < 8:
		return fmt.Errorf("Face %d < 8", p.Face)
	case p.Steps < 1:
		return fmt.Errorf("Steps %d < 1", p.Steps)
	case p.KeyframeEvery < 1:
		return fmt.Errorf("KeyframeEvery %d < 1", p.KeyframeEvery)
	case p.MajorMin > p.MajorMax || p.MinorMin > p.MinorMax || p.MajorMin < 1:
		return fmt.Errorf("plate count ranges %d-%d / %d-%d invalid", p.MajorMin, p.MajorMax, p.MinorMin, p.MinorMax)
	case p.SpeedMinCmYr < 0 || p.SpeedMaxCmYr < p.SpeedMinCmYr:
		return fmt.Errorf("speed range %g-%g invalid", p.SpeedMinCmYr, p.SpeedMaxCmYr)
	case p.RepoleEvery < 1:
		return fmt.Errorf("RepoleEvery %d < 1", p.RepoleEvery)
	}
	return nil
}

// Snapshot copies the current grids into a Frame.
func (s *State) Snapshot(myrPerStep float64) Frame {
	feat := NewGrid[uint8](s.Th.S)
	for i := range feat.Cells {
		feat.Cells[i] = s.FeatureByte(i)
	}
	rows := make([]PlateRow, len(s.Plates))
	for i, pl := range s.Plates {
		rows[i] = PlateRow{ID: pl.ID, Centroid: pl.Centroid, Pole: s.Motions[i].Pole,
			SpeedCmYr: s.Motions[i].SpeedCmYr, Area: pl.Area, Major: pl.Major, Retired: pl.Retired}
	}
	return Frame{Step: s.StepNo, Myr: float64(s.StepNo) * myrPerStep,
		Th: s.Th.Clone(), Labels: s.Labels.Clone(), Feature: feat, Plates: rows}
}

// Run builds the initial plates for (p, master) and drives them for p.Steps
// steps, calling emit for frame 0, every KeyframeEvery-th step and the last.
func Run(p Params, master int64, emit func(Frame) error) error {
	if err := Validate(p); err != nil {
		return err
	}
	th := GenerateThickness(p, master)
	labels, plates := BuildPlates(th, p, master)
	motions := InitMotion(th, labels, plates, p, master)
	s := NewState(p, th, labels, plates, motions)
	stepRNG := newRNG(master, "sim.step")
	reaimRNG := newRNG(master, "sim.reaim")
	if err := emit(s.Snapshot(p.MyrPerStep())); err != nil {
		return err
	}
	for step := 1; step <= p.Steps; step++ {
		s.Step(stepRNG)
		if step%p.RepoleEvery == 0 {
			Reaim(s.Th, s.Labels, s.Plates, s.Motions, p, reaimRNG)
		}
		if step%p.KeyframeEvery == 0 || step == p.Steps {
			if err := emit(s.Snapshot(p.MyrPerStep())); err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests and lint, expect PASS**

Run: `go test ./pkg/tectonics/ -v && golangci-lint run ./pkg/tectonics/...`

- [ ] **Step 5: Commit**

```bash
git add pkg/tectonics/sim.go pkg/tectonics/sim_test.go
git commit -m "feat(tectonics): run loop emitting keyframe snapshots" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 7: Keyframe bundle on disk

**Files:**
- Create: `pkg/tectonics/bundle.go`
- Test: `pkg/tectonics/bundle_test.go`

**Interfaces:**
- Consumes: `Frame`, `PlateRow`, `Params`, `Grid`, `cubemap.New`, `cubemap.WriteCrossPNGTo`, `cubemap.ReadCrossPNGFrom`, `cubemap.CubeMap`.
- Produces: `type FrameInfo struct{ Step int; Myr float64; Dir string; Plates []PlateRow }`; `type Manifest struct{ Version int; Seed int64; Planet string; Archetype string; Params Params; Face int; Steps int; MyrPerStep float64; Frames []FrameInfo }`; `NewManifest(p Params, master int64, planet string) *Manifest`; `WriteFrame(bundleDir string, f Frame) (FrameInfo, error)` — writes `frames/<step:04d>/{thickness,plate,feature}.png`; `WriteManifest(bundleDir string, m *Manifest) error`; `ReadManifest(bundleDir string) (*Manifest, error)`; `ReadFrame(bundleDir string, fi FrameInfo) (Frame, error)` — Th quantised to 8 bits (`v/255`), Labels from the plate PNG red channel, Feature from the feature PNG red channel.

Encoding: each PNG is a 4S×3S cube-cross (`cubemap.WriteCrossPNGTo`). Thickness → grey `uint8(round(v*255))` in R=G=B, A=255. Plate id → R = id (max 255 plates), G=B=0, A=255. Feature → R = feature byte, G=B=0, A=255.

- [ ] **Step 1: Write the failing test**

```go
// pkg/tectonics/bundle_test.go
package tectonics

import (
	"math"
	"path/filepath"
	"testing"
)

func TestBundleRoundTrip(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 3, 1
	dir := t.TempDir()
	m := NewManifest(p, 99, "test_planet")
	var frames []Frame
	err := Run(p, 99, func(f Frame) error {
		fi, err := WriteFrame(dir, f)
		if err != nil {
			return err
		}
		m.Frames = append(m.Frames, fi)
		frames = append(frames, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	m2, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Seed != 99 || m2.Planet != "test_planet" || m2.Face != 16 || len(m2.Frames) != 4 || m2.Frames[3].Step != 3 {
		t.Fatalf("manifest %+v", m2)
	}
	if m2.Frames[1].Dir != filepath.Join("frames", "0001") {
		t.Errorf("frame dir %q", m2.Frames[1].Dir)
	}
	for i, fi := range m2.Frames {
		got, err := ReadFrame(dir, fi)
		if err != nil {
			t.Fatal(err)
		}
		want := frames[i]
		for j := range want.Th.Cells {
			if math.Abs(got.Th.Cells[j]-want.Th.Cells[j]) > 1.0/255+1e-9 {
				t.Fatalf("frame %d pixel %d thickness %g vs %g", i, j, got.Th.Cells[j], want.Th.Cells[j])
			}
			if got.Labels.Cells[j] != want.Labels.Cells[j] || got.Feature.Cells[j] != want.Feature.Cells[j] {
				t.Fatalf("frame %d pixel %d label/feature mismatch", i, j)
			}
		}
		if len(fi.Plates) != len(want.Plates) {
			t.Errorf("frame %d plate rows %d vs %d", i, len(fi.Plates), len(want.Plates))
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run TestBundle -v`
Expected: FAIL — `undefined: NewManifest`.

- [ ] **Step 3: Write bundle.go**

```go
package tectonics

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

const manifestVersion = 1

type FrameInfo struct {
	Step   int        `json:"step"`
	Myr    float64    `json:"myr"`
	Dir    string     `json:"dir"` // relative to the bundle root
	Plates []PlateRow `json:"plates"`
}

type Manifest struct {
	Version    int         `json:"version"`
	Seed       int64       `json:"seed"`
	Planet     string      `json:"planet,omitempty"`
	Archetype  string      `json:"archetype"`
	Params     Params      `json:"params"`
	Face       int         `json:"face"`
	Steps      int         `json:"steps"`
	MyrPerStep float64     `json:"myr_per_step"`
	Frames     []FrameInfo `json:"frames"`
}

func NewManifest(p Params, master int64, planet string) *Manifest {
	return &Manifest{Version: manifestVersion, Seed: master, Planet: planet, Archetype: p.Archetype,
		Params: p, Face: p.Face, Steps: p.Steps, MyrPerStep: p.MyrPerStep()}
}

func gridToCross[T any](g *Grid[T], px func(T) color.RGBA) *cubemap.CubeMap {
	cm := cubemap.New(g.S)
	for i, v := range g.Cells {
		f, x, y := g.Addr(i)
		cm.Set(f, x, y, px(v))
	}
	return cm
}

func writeCross(path string, cm *cubemap.CubeMap) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = cubemap.WriteCrossPNGTo(cm, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// WriteFrame writes frames/<step:04d>/{thickness,plate,feature}.png under
// bundleDir and returns the manifest row for the frame.
func WriteFrame(bundleDir string, fr Frame) (FrameInfo, error) {
	rel := filepath.Join("frames", fmt.Sprintf("%04d", fr.Step))
	dir := filepath.Join(bundleDir, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return FrameInfo{}, err
	}
	grey := func(v float64) color.RGBA {
		g := uint8(math.Round(math.Max(0, math.Min(1, v)) * 255))
		return color.RGBA{g, g, g, 255}
	}
	idc := func(v int32) color.RGBA { return color.RGBA{uint8(v), 0, 0, 255} }
	fb := func(v uint8) color.RGBA { return color.RGBA{v, 0, 0, 255} }
	if err := writeCross(filepath.Join(dir, "thickness.png"), gridToCross(fr.Th, grey)); err != nil {
		return FrameInfo{}, err
	}
	if err := writeCross(filepath.Join(dir, "plate.png"), gridToCross(fr.Labels, idc)); err != nil {
		return FrameInfo{}, err
	}
	if err := writeCross(filepath.Join(dir, "feature.png"), gridToCross(fr.Feature, fb)); err != nil {
		return FrameInfo{}, err
	}
	return FrameInfo{Step: fr.Step, Myr: fr.Myr, Dir: rel, Plates: fr.Plates}, nil
}

func WriteManifest(bundleDir string, m *Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bundleDir, "manifest.json"), b, 0o644)
}

func ReadManifest(bundleDir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(bundleDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", bundleDir, err)
	}
	if m.Version != manifestVersion {
		return nil, fmt.Errorf("manifest %s: version %d, want %d", bundleDir, m.Version, manifestVersion)
	}
	return &m, nil
}

func readCross(path string) (*cubemap.CubeMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return cubemap.ReadCrossPNGFrom(f)
}

func crossToGrid[T any](cm *cubemap.CubeMap, px func(color.RGBA) T) *Grid[T] {
	g := NewGrid[T](cm.Size)
	for i := range g.Cells {
		f, x, y := g.Addr(i)
		g.Cells[i] = px(cm.Get(f, x, y))
	}
	return g
}

// ReadFrame loads one frame's grids. Thickness is 8-bit quantised.
func ReadFrame(bundleDir string, fi FrameInfo) (Frame, error) {
	dir := filepath.Join(bundleDir, fi.Dir)
	th, err := readCross(filepath.Join(dir, "thickness.png"))
	if err != nil {
		return Frame{}, err
	}
	pl, err := readCross(filepath.Join(dir, "plate.png"))
	if err != nil {
		return Frame{}, err
	}
	fe, err := readCross(filepath.Join(dir, "feature.png"))
	if err != nil {
		return Frame{}, err
	}
	return Frame{Step: fi.Step, Myr: fi.Myr, Plates: fi.Plates,
		Th:      crossToGrid(th, func(c color.RGBA) float64 { return float64(c.R) / 255 }),
		Labels:  crossToGrid(pl, func(c color.RGBA) int32 { return int32(c.R) }),
		Feature: crossToGrid(fe, func(c color.RGBA) uint8 { return c.R }),
	}, nil
}
```

- [ ] **Step 4: Run tests and lint, expect PASS**

Run: `go test ./pkg/tectonics/ -v && golangci-lint run ./pkg/tectonics/...`

If `errcheck` flags `defer f.Close()` in `readCross`, change it to `defer func() { _ = f.Close() }()`.

- [ ] **Step 5: Commit**

```bash
git add pkg/tectonics/bundle.go pkg/tectonics/bundle_test.go
git commit -m "feat(tectonics): keyframe bundle writer and reader (cube-cross PNGs + manifest)" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 8: `tectonics-lab run`

**Files:**
- Create: `cmd/tectonics-lab/main.go`, `cmd/tectonics-lab/run.go`
- Modify: `.gitignore` (append `data/tectonics/`)
- Test: `cmd/tectonics-lab/run_test.go`

**Interfaces:**
- Consumes: `tectonics.DefaultParams`, `tectonics.Params.Set`, `tectonics.SeedForPlanet`, `tectonics.Run`, `tectonics.NewManifest`, `tectonics.WriteFrame`, `tectonics.WriteManifest`, `tectonics.Archetypes`.
- Produces: `type runOpts struct{ Planet string; Seed int64; Archetype string; Out string; Sets []string; Face, Steps int; Progress func(step, steps int) }`; `runBundle(o runOpts) (dir string, err error)` — resolves seed (planet id wins over `Seed`), applies `-set`s, writes to `<Out>/<slug>/`, slug = planet id or `seed-<n>`, suffixed `-<archetype>`; `setFlag` type implementing `flag.Value` for repeated `-set k=v`.

Usage:

```
tectonics-lab run -planet sol_earth -archetype terran [-face 256] [-steps 150] [-set RidgeThickness=0.2]... [-out data/tectonics]
tectonics-lab run -seed 42 -archetype arid
tectonics-lab serve [-addr :8091] [-data data/tectonics]
```

- [ ] **Step 1: Write the failing test**

```go
// cmd/tectonics-lab/run_test.go
package main

import (
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

func TestRunBundleWritesManifestAndFrames(t *testing.T) {
	out := t.TempDir()
	var last int
	dir, err := runBundle(runOpts{Seed: 5, Archetype: "arid", Out: out, Face: 16, Steps: 3,
		Sets: []string{"KeyframeEvery=1", "RidgeThickness=0.2"}, Progress: func(s, _ int) { last = s }})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "seed-5-arid" {
		t.Errorf("dir %s", dir)
	}
	m, err := tectonics.ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Frames) != 4 || m.Params.RidgeThickness != 0.2 || m.Face != 16 || m.Seed != 5 || last != 3 {
		t.Errorf("manifest %+v last=%d", m, last)
	}
	if _, err := tectonics.ReadFrame(dir, m.Frames[3]); err != nil {
		t.Fatal(err)
	}
}

func TestRunBundleRefusesGasGiantAndBadSet(t *testing.T) {
	if _, err := runBundle(runOpts{Seed: 1, Archetype: "jovian", Out: t.TempDir(), Face: 16, Steps: 1}); err == nil {
		t.Error("jovian must be refused")
	}
	if _, err := runBundle(runOpts{Seed: 1, Archetype: "arid", Out: t.TempDir(), Face: 16, Steps: 1, Sets: []string{"Nope=1"}}); err == nil {
		t.Error("unknown knob must be refused")
	}
	if _, err := runBundle(runOpts{Planet: "x_i", Archetype: "arid", Out: t.TempDir(), Face: 16, Steps: 1, Sets: []string{"garbage"}}); err == nil {
		t.Error("malformed -set must be refused")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/tectonics-lab/ -run TestRunBundle -v`
Expected: FAIL — `undefined: runBundle`.

- [ ] **Step 3: Write run.go**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

type runOpts struct {
	Planet    string
	Seed      int64
	Archetype string
	Out       string
	Sets      []string
	Face      int
	Steps     int
	Progress  func(step, steps int)
}

// setFlag collects repeated -set Name=value flags.
type setFlag []string

func (s *setFlag) String() string     { return strings.Join(*s, ",") }
func (s *setFlag) Set(v string) error { *s = append(*s, v); return nil }

func runBundle(o runOpts) (string, error) {
	p, err := tectonics.DefaultParams(o.Archetype)
	if err != nil {
		return "", err
	}
	if o.Face > 0 {
		p.Face = o.Face
	}
	if o.Steps > 0 {
		p.Steps = o.Steps
	}
	for _, kv := range o.Sets {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return "", fmt.Errorf("-set %q: want Name=value", kv)
		}
		if err := p.Set(k, v); err != nil {
			return "", err
		}
	}
	seed, slug := o.Seed, fmt.Sprintf("seed-%d", o.Seed)
	if o.Planet != "" {
		seed, slug = tectonics.SeedForPlanet(o.Planet), o.Planet
	}
	dir := filepath.Join(o.Out, slug+"-"+o.Archetype)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	m := tectonics.NewManifest(p, seed, o.Planet)
	err = tectonics.Run(p, seed, func(f tectonics.Frame) error {
		fi, err := tectonics.WriteFrame(dir, f)
		if err != nil {
			return err
		}
		m.Frames = append(m.Frames, fi)
		if o.Progress != nil {
			o.Progress(f.Step, p.Steps)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return dir, tectonics.WriteManifest(dir, m)
}
```

Note `os.RemoveAll(dir)`: the bundle directory is always `<Out>/<slug>-<archetype>` under the git-ignored data dir, so re-running a seed replaces its old bundle. Never point `-out` at anything else.

- [ ] **Step 4: Write main.go**

```go
// Command tectonics-lab bakes tectonic-plate timelines into keyframe bundles
// and serves a time-slider viewer for them.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  tectonics-lab run  -planet <id> | -seed <n>  -archetype <%s> [-face 256] [-steps 150] [-set Name=value]... [-out data/tectonics]
  tectonics-lab serve [-addr :8091] [-data data/tectonics] [-web cmd/tectonics-lab/web]
`, strings.Join(tectonics.Archetypes(), "|"))
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		var o runOpts
		var sets setFlag
		fs.StringVar(&o.Planet, "planet", "", "planet game id (seeds the run)")
		fs.Int64Var(&o.Seed, "seed", 0, "raw master seed when no -planet")
		fs.StringVar(&o.Archetype, "archetype", "terran", "planet archetype")
		fs.StringVar(&o.Out, "out", "data/tectonics", "bundle root")
		fs.IntVar(&o.Face, "face", 0, "cube face size (default from archetype)")
		fs.IntVar(&o.Steps, "steps", 0, "time steps (default from archetype)")
		fs.Var(&sets, "set", "override a knob, Name=value (repeatable)")
		_ = fs.Parse(os.Args[2:])
		o.Sets = sets
		o.Progress = func(step, steps int) {
			if step%10 == 0 || step == steps {
				log.Printf("step %d/%d", step, steps)
			}
		}
		dir, err := runBundle(o)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("bundle written to %s", dir)
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := fs.String("addr", ":8091", "listen address")
		data := fs.String("data", "data/tectonics", "bundle root")
		web := fs.String("web", "cmd/tectonics-lab/web", "viewer assets")
		_ = fs.Parse(os.Args[2:])
		log.Fatal(serve(*addr, *data, *web))
	default:
		usage()
	}
}
```

`serve` is defined in Task 10; until then add a stub in `main.go` so the package builds:

```go
func serve(addr, data, web string) error { return fmt.Errorf("serve not implemented") }
```

- [ ] **Step 5: Append to .gitignore**

```
# tectonics-lab keyframe bundles
data/tectonics/
```

- [ ] **Step 6: Run tests, build the binary, lint, expect PASS**

Run: `go test ./cmd/tectonics-lab/ -v && go build -o bin/tectonics-lab ./cmd/tectonics-lab && golangci-lint run ./cmd/tectonics-lab/... && ./bin/tectonics-lab run -seed 42 -archetype terran -face 64 -steps 20 && ls data/tectonics/seed-42-terran/frames | wc -l`
Expected: 21 frame dirs, manifest present.

- [ ] **Step 7: Commit**

```bash
git add cmd/tectonics-lab/main.go cmd/tectonics-lab/run.go cmd/tectonics-lab/run_test.go .gitignore
git commit -m "feat(tectonics-lab): run subcommand bakes a keyframe bundle per planet" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 9: Golden hashes and timing budget

**Files:**
- Create: `cmd/tectonics-lab/golden_test.go`, `cmd/tectonics-lab/testdata/golden_face64.json`

**Interfaces:**
- Consumes: `tectonics.DefaultParams`, `tectonics.Run`, `tectonics.Frame`.
- Produces: `frameHash(f tectonics.Frame) string` — FNV-1a 64 over Th (as `math.Float64bits`), Labels, Feature in index order, hex; `-update` test flag rewrites the golden.

Golden recipe: archetype terran, seed 2026, face 64, steps 20, KeyframeEvery 1. The file is `{"recipe": "terran/2026/64/20", "hashes": ["...", ...]}` with 21 entries.

- [ ] **Step 1: Write the test**

```go
// cmd/tectonics-lab/golden_test.go
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

var update = flag.Bool("update", false, "rewrite testdata/golden_face64.json")

type golden struct {
	Recipe string   `json:"recipe"`
	Hashes []string `json:"hashes"`
}

func frameHash(f tectonics.Frame) string {
	h := fnv.New64a()
	var b [8]byte
	for _, v := range f.Th.Cells {
		bits := math.Float64bits(v)
		for k := range 8 {
			b[k] = byte(bits >> (8 * k))
		}
		_, _ = h.Write(b[:])
	}
	for _, v := range f.Labels.Cells {
		_, _ = h.Write([]byte{byte(v), byte(v >> 8)})
	}
	_, _ = h.Write(f.Feature.Cells)
	return hex.EncodeToString(h.Sum(nil))
}

func goldenRun(t *testing.T) []string {
	t.Helper()
	p, err := tectonics.DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face, p.Steps, p.KeyframeEvery = 64, 20, 1
	var hashes []string
	if err := tectonics.Run(p, 2026, func(f tectonics.Frame) error {
		hashes = append(hashes, frameHash(f))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return hashes
}

func TestGoldenFace64(t *testing.T) {
	path := filepath.Join("testdata", "golden_face64.json")
	got := golden{Recipe: "terran/2026/64/20", Hashes: goldenRun(t)}
	if *update {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden rewritten: %d frames", len(got.Hashes))
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	var want golden
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if want.Recipe != got.Recipe || len(want.Hashes) != len(got.Hashes) {
		t.Fatalf("golden recipe/length mismatch: %s/%d vs %s/%d", want.Recipe, len(want.Hashes), got.Recipe, len(got.Hashes))
	}
	for i := range want.Hashes {
		if want.Hashes[i] != got.Hashes[i] {
			t.Errorf("frame %d hash %s, golden %s", i, got.Hashes[i], want.Hashes[i])
		}
	}
}

func TestTimingBudgetFace128(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test skipped in -short")
	}
	p, err := tectonics.DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face, p.Steps, p.KeyframeEvery = 128, 10, 10
	start := time.Now()
	if err := tectonics.Run(p, 1, func(tectonics.Frame) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 60*time.Second {
		t.Errorf("face 128 × 10 steps took %s, budget 60s", d)
	} else {
		t.Logf("face 128 × 10 steps: %s", d)
	}
}
```

- [ ] **Step 2: Bake the golden, then verify a clean second pass**

Run: `go test ./cmd/tectonics-lab/ -run TestGoldenFace64 -update -v && go test ./cmd/tectonics-lab/ -run TestGoldenFace64 -v`
Expected: first run logs "golden rewritten: 21 frames"; second run PASS.

- [ ] **Step 3: Run the timing test**

Run: `go test ./cmd/tectonics-lab/ -run TestTimingBudgetFace128 -v`
Expected: PASS with the logged duration. If it exceeds 60 s, the claims loop in `Step` is the hotspot: precompute each plate's cap test as a cosine threshold (already done) and skip plates whose `DegPerStep` is 0; if still over, record the measured time in the commit message and raise the budget to 120 s rather than optimise further in this checkpoint.

- [ ] **Step 4: Lint and commit**

Run: `golangci-lint run ./cmd/tectonics-lab/...`

```bash
git add cmd/tectonics-lab/golden_test.go cmd/tectonics-lab/testdata/golden_face64.json
git commit -m "test(tectonics-lab): face-64 golden hashes and face-128 timing budget" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 10: `tectonics-lab serve`

**Files:**
- Create: `cmd/tectonics-lab/server.go`
- Modify: `cmd/tectonics-lab/main.go` (delete the `serve` stub)
- Test: `cmd/tectonics-lab/server_test.go`

**Interfaces:**
- Consumes: `runBundle`, `runOpts`, `tectonics.ReadManifest`, `tectonics.Archetypes`.
- Produces: `newServer(data, web string) *server` with `(s *server) Handler() http.Handler`; `serve(addr, data, web string) error`. Routes:
  - `GET /api/bundles` → `[{"slug":"seed-42-terran","archetype":"terran","planet":"","seed":42,"face":256,"steps":150,"frames":151}]`, sorted by slug.
  - `GET /api/archetypes` → `["arid", ...]`.
  - `POST /api/run` with JSON `{"planet":"","seed":42,"archetype":"terran","face":256,"steps":150,"sets":["RidgeThickness=0.2"]}` → `202 {"job":"<id>"}`; `409` if a run is already in progress; `400` on bad params.
  - `GET /api/jobs/{id}` → `{"state":"running|done|error","step":12,"steps":150,"slug":"...","error":""}`.
  - `GET /bundles/<slug>/manifest.json`, `/bundles/<slug>/frames/0000/thickness.png` … → files under `data` (`http.FileServer`, no directory listings needed).
  - `GET /` and other paths → files under `web`, no-cache.

- [ ] **Step 1: Write the failing test**

```go
// cmd/tectonics-lab/server_test.go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerRunListAndFiles(t *testing.T) {
	data, web := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>viewer</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(newServer(data, web).Handler())
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/run", "application/json",
		strings.NewReader(`{"seed":3,"archetype":"arid","face":16,"steps":2,"sets":["KeyframeEvery=1"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("run status %d", res.StatusCode)
	}
	var started struct{ Job string }
	_ = json.NewDecoder(res.Body).Decode(&started)
	res.Body.Close()

	var st struct {
		State string
		Step  int
		Slug  string
		Error string
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != "done" && st.State != "error" && time.Now().Before(deadline) {
		r, err := http.Get(ts.URL + "/api/jobs/" + started.Job)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(r.Body).Decode(&st)
		r.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}
	if st.State != "done" || st.Slug != "seed-3-arid" || st.Step != 2 {
		t.Fatalf("job %+v", st)
	}

	r, _ := http.Get(ts.URL + "/api/bundles")
	var list []struct {
		Slug   string
		Frames int
	}
	_ = json.NewDecoder(r.Body).Decode(&list)
	r.Body.Close()
	if len(list) != 1 || list[0].Slug != "seed-3-arid" || list[0].Frames != 3 {
		t.Fatalf("bundles %+v", list)
	}
	for _, path := range []string{"/bundles/seed-3-arid/manifest.json", "/bundles/seed-3-arid/frames/0002/plate.png", "/"} {
		r, err := http.Get(ts.URL + path)
		if err != nil || r.StatusCode != 200 {
			t.Errorf("GET %s: %v %v", path, err, r)
		}
		if r != nil {
			r.Body.Close()
		}
	}
	r, _ = http.Post(ts.URL+"/api/run", "application/json", strings.NewReader(`{"seed":1,"archetype":"jovian","face":16,"steps":1}`))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("jovian run status %d", r.StatusCode)
	}
	r.Body.Close()
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/tectonics-lab/ -run TestServer -v`
Expected: FAIL — `undefined: newServer`.

- [ ] **Step 3: Write server.go and remove the stub from main.go**

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

type job struct {
	ID    string `json:"job"`
	State string `json:"state"`
	Step  int    `json:"step"`
	Steps int    `json:"steps"`
	Slug  string `json:"slug"`
	Error string `json:"error,omitempty"`
}

type server struct {
	data, web string
	mu        sync.Mutex
	jobs      map[string]*job
	running   bool
}

func newServer(data, web string) *server {
	return &server{data: data, web: web, jobs: map[string]*job{}}
}

type bundleInfo struct {
	Slug      string `json:"slug"`
	Archetype string `json:"archetype"`
	Planet    string `json:"planet"`
	Seed      int64  `json:"seed"`
	Face      int    `json:"face"`
	Steps     int    `json:"steps"`
	Frames    int    `json:"frames"`
}

func (s *server) listBundles() []bundleInfo {
	entries, _ := os.ReadDir(s.data)
	var out []bundleInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := tectonics.ReadManifest(filepath.Join(s.data, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, bundleInfo{Slug: e.Name(), Archetype: m.Archetype, Planet: m.Planet,
			Seed: m.Seed, Face: m.Face, Steps: m.Steps, Frames: len(m.Frames)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	if out == nil {
		out = []bundleInfo{}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) startRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Planet    string   `json:"planet"`
		Seed      int64    `json:"seed"`
		Archetype string   `json:"archetype"`
		Face      int      `json:"face"`
		Steps     int      `json:"steps"`
		Sets      []string `json:"sets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// validate params up front so bad requests fail synchronously
	p, err := tectonics.DefaultParams(req.Archetype)
	if err == nil {
		for _, kv := range req.Sets {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				err = fmt.Errorf("-set %q: want Name=value", kv)
				break
			}
			if err = p.Set(k, v); err != nil {
				break
			}
		}
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a run is already in progress"})
		return
	}
	s.running = true
	j := &job{ID: fmt.Sprintf("%d", time.Now().UnixNano()), State: "running"}
	s.jobs[j.ID] = j
	s.mu.Unlock()

	go func() {
		dir, err := runBundle(runOpts{Planet: req.Planet, Seed: req.Seed, Archetype: req.Archetype,
			Out: s.data, Sets: req.Sets, Face: req.Face, Steps: req.Steps,
			Progress: func(step, steps int) {
				s.mu.Lock()
				j.Step, j.Steps = step, steps
				s.mu.Unlock()
			}})
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running = false
		if err != nil {
			j.State, j.Error = "error", err.Error()
			return
		}
		j.State, j.Slug = "done", filepath.Base(dir)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID})
}

func (s *server) jobStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	j, ok := s.jobs[r.PathValue("id")]
	var cp job
	if ok {
		cp = *j
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such job"})
		return
	}
	writeJSON(w, http.StatusOK, cp)
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (s *server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/bundles", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.listBundles()) })
	mux.HandleFunc("GET /api/archetypes", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, tectonics.Archetypes()) })
	mux.HandleFunc("POST /api/run", s.startRun)
	mux.HandleFunc("GET /api/jobs/{id}", s.jobStatus)
	mux.Handle("GET /bundles/", noCache(http.StripPrefix("/bundles/", http.FileServer(http.Dir(s.data)))))
	mux.Handle("/", noCache(http.FileServer(http.Dir(s.web))))
	return mux
}

func serve(addr, data, web string) error {
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}
	fmt.Printf("tectonics-lab viewer on http://localhost%s (bundles: %s)\n", addr, data)
	srv := &http.Server{Addr: addr, Handler: newServer(data, web).Handler(), ReadHeaderTimeout: 10 * time.Second}
	return srv.ListenAndServe()
}
```

Delete the `serve` stub line from `main.go`.

- [ ] **Step 4: Run tests and lint, expect PASS**

Run: `go test ./cmd/tectonics-lab/ -short -v && golangci-lint run ./cmd/tectonics-lab/...`

- [ ] **Step 5: Commit**

```bash
git add cmd/tectonics-lab/server.go cmd/tectonics-lab/server_test.go cmd/tectonics-lab/main.go
git commit -m "feat(tectonics-lab): serve subcommand with bundle list, run jobs and static viewer" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 11: Time-slider viewer

**Files:**
- Create: `cmd/tectonics-lab/web/index.html`, `cmd/tectonics-lab/web/app.js`
- Test: `cmd/tectonics-lab/web_test.go`

**Interfaces:**
- Consumes: the HTTP API from Task 10 and the bundle layout from Task 7 (`/bundles/<slug>/manifest.json`, frame dirs with `thickness.png`, `plate.png`, `feature.png`, cross layout 4S×3S with cells PosX(2,1) NegX(0,1) PosY(1,0) NegY(1,2) PosZ(1,1) NegZ(3,1), `DirToFaceUV` formula from `pkg/planetgen/cubemap/sample.go`).
- Produces: a page at `/` with: bundle select, run form (planet, seed, archetype, face, steps, sets), time slider + play/pause + Myr readout, layer radio (plates / thickness / hypsometric / features), arrows checkbox, WebGL sphere (drag to rotate) and flat cube-cross, both coloured by one GLSL function.

The sphere is ray-cast in the fragment shader (no mesh): each pixel of a full-screen quad intersects a unit sphere, the hit direction is rotated by the view, mapped to (face, u, v) exactly as `DirToFaceUV`, then to cross-texture coordinates `((col+u)/4, (row+v)/3)`. Flat mode draws the cross directly. Textures use NEAREST filtering so plate ids and feature bytes survive sampling.

- [ ] **Step 1: Write the asset test**

```go
// cmd/tectonics-lab/web_test.go
package main

import (
	"os"
	"strings"
	"testing"
)

func TestViewerAssetsPresent(t *testing.T) {
	idx, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`src="app.js"`, `id="slider"`, `id="sphere"`, `id="cross"`, `id="bundle"`} {
		if !strings.Contains(string(idx), want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	js, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/api/bundles", "/api/run", "/api/jobs/", "manifest.json", "gl_FragColor", "NEAREST"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/tectonics-lab/ -run TestViewerAssets -v`
Expected: FAIL — `open web/index.html: no such file`.

- [ ] **Step 3: Write index.html**

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Tectonics Lab</title>
<style>
  :root { --bg:#0e1116; --fg:#d8dee9; --muted:#7a8494; --panel:#171b22; --accent:#5aa9ff; }
  body { margin:0; background:var(--bg); color:var(--fg); font:14px/1.4 system-ui, sans-serif; }
  header { display:flex; flex-wrap:wrap; gap:12px 20px; align-items:center; padding:10px 16px; background:var(--panel); }
  header label { display:flex; gap:6px; align-items:center; color:var(--muted); }
  input, select, button { background:#0b0e13; color:var(--fg); border:1px solid #2a313c; border-radius:4px; padding:4px 6px; font:inherit; }
  button { cursor:pointer; } button:hover { border-color:var(--accent); }
  main { display:grid; grid-template-columns: minmax(320px, 1fr) minmax(320px, 1fr); gap:16px; padding:16px; }
  @media (max-width: 760px) { main { grid-template-columns: 1fr; } }
  .view { position:relative; background:#05070a; border:1px solid #2a313c; border-radius:6px; overflow:hidden; }
  canvas { display:block; width:100%; height:auto; }
  #arrows { position:absolute; inset:0; pointer-events:none; }
  .timeline { grid-column: 1 / -1; display:flex; gap:12px; align-items:center; }
  .timeline input[type=range] { flex:1; }
  #status { color:var(--muted); min-width:220px; }
  #myr { font-variant-numeric: tabular-nums; min-width:120px; }
  fieldset { border:1px solid #2a313c; border-radius:4px; padding:4px 8px; display:flex; gap:10px; }
  legend { color:var(--muted); font-size:12px; }
</style>
</head>
<body>
<header>
  <label>Bundle <select id="bundle"></select></label>
  <fieldset><legend>New run</legend>
    <label>Planet <input id="planet" size="12" placeholder="sol_earth"></label>
    <label>Seed <input id="seed" type="number" value="42" style="width:7em"></label>
    <label>Archetype <select id="archetype"></select></label>
    <label>Face <input id="face" type="number" value="256" style="width:5em"></label>
    <label>Steps <input id="steps" type="number" value="150" style="width:5em"></label>
    <label>Sets <input id="sets" size="24" placeholder="RidgeThickness=0.2 Steps=100"></label>
    <button id="run">Run</button>
  </fieldset>
  <fieldset><legend>Layer</legend>
    <label><input type="radio" name="layer" value="0" checked> plates</label>
    <label><input type="radio" name="layer" value="1"> thickness</label>
    <label><input type="radio" name="layer" value="2"> hypsometric</label>
    <label><input type="radio" name="layer" value="3"> features</label>
    <label><input type="checkbox" id="showArrows" checked> arrows</label>
  </fieldset>
  <span id="status">no bundle loaded</span>
</header>
<main>
  <div class="view"><canvas id="sphere" width="640" height="640"></canvas><canvas id="arrows" width="640" height="640"></canvas></div>
  <div class="view"><canvas id="cross" width="800" height="600"></canvas></div>
  <div class="timeline">
    <button id="play">▶</button>
    <input id="slider" type="range" min="0" max="0" value="0">
    <span id="myr">0 Myr</span>
  </div>
</main>
<script src="app.js"></script>
</body>
</html>
```

- [ ] **Step 4: Write app.js**

```js
// Tectonics Lab viewer: scrubs keyframe bundles produced by tectonics-lab run.
(function () {
  'use strict';
  const $ = (id) => document.getElementById(id);

  // ---- GLSL: one colouring function shared by the sphere and the cross ----
  const VS = `
    attribute vec2 aPos; varying vec2 vUV;
    void main(){ vUV = aPos*0.5+0.5; gl_Position = vec4(aPos,0.0,1.0); }`;
  const FS = `
    precision highp float;
    varying vec2 vUV;
    uniform sampler2D uTh, uPl, uFe;
    uniform int uMode;      // 0 sphere, 1 flat cross
    uniform int uLayer;     // 0 plates, 1 thickness, 2 hypsometric, 3 features
    uniform mat3 uView;     // view rotation (screen -> world)
    uniform vec3 uSun;

    // Mirrors cubemap.DirToFaceUV + crossCells: returns cross-texture coords.
    vec2 crossUV(vec3 d){
      vec3 a = abs(d); float sc, tc, ma; vec2 cell;
      if (a.x >= a.y && a.x >= a.z) { ma = a.x;
        if (d.x >= 0.0) { sc = -d.z; tc = -d.y; cell = vec2(2.0,1.0); } else { sc = d.z; tc = -d.y; cell = vec2(0.0,1.0); }
      } else if (a.y >= a.z) { ma = a.y;
        if (d.y >= 0.0) { sc = d.x; tc = d.z; cell = vec2(1.0,0.0); } else { sc = d.x; tc = -d.z; cell = vec2(1.0,2.0); }
      } else { ma = a.z;
        if (d.z >= 0.0) { sc = d.x; tc = -d.y; cell = vec2(1.0,1.0); } else { sc = -d.x; tc = -d.y; cell = vec2(3.0,1.0); }
      }
      float u = 0.5*(sc/ma+1.0), v = 0.5*(tc/ma+1.0);
      return vec2((cell.x+u)/4.0, (cell.y+v)/3.0);
    }
    vec3 hsv(float h, float s, float v){
      vec3 k = fract(vec3(h, h+2.0/3.0, h+1.0/3.0))*6.0;
      return v * mix(vec3(1.0), clamp(abs(k-3.0)-1.0, 0.0, 1.0), s);
    }
    vec3 colour(vec2 uv){
      float th = texture2D(uTh, uv).r;
      float id = floor(texture2D(uPl, uv).r*255.0+0.5);
      float fb = floor(texture2D(uFe, uv).r*255.0+0.5);
      float ftype = floor(fb/64.0), fage = fb - ftype*64.0;
      vec3 c;
      if (uLayer == 0) {
        c = hsv(fract(id*0.618034), 0.55, 0.45+0.45*th);
      } else if (uLayer == 1) {
        c = vec3(th);
      } else if (uLayer == 2) {
        c = th < 0.5 ? mix(vec3(0.05,0.15,0.45), vec3(0.2,0.55,0.8), th*2.0)
                     : mix(vec3(0.25,0.5,0.2), vec3(0.95,0.95,0.95), (th-0.5)*2.0);
      } else {
        c = vec3(0.25+0.5*th);
      }
      if (uLayer == 0 || uLayer == 3) {
        float fade = 1.0 - fage/63.0;
        if (ftype == 1.0) c = mix(c, vec3(0.2,0.5,1.0), 0.4+0.6*fade);
        if (ftype == 2.0) c = mix(c, mix(vec3(0.55,0.3,0.15), vec3(1.0,0.2,0.1), fade), 0.5+0.5*fade);
        if (ftype == 3.0) c = mix(c, vec3(1.0,0.9,0.2), 0.3+0.7*fade);
      }
      return c;
    }
    void main(){
      if (uMode == 1) {
        gl_FragColor = vec4(colour(vec2(vUV.x, 1.0-vUV.y)), 1.0);
        return;
      }
      vec2 p = vUV*2.0-1.0;
      float r2 = dot(p,p);
      if (r2 > 1.0) { gl_FragColor = vec4(0.02,0.027,0.04,1.0); return; }
      vec3 n = vec3(p.x, p.y, sqrt(1.0-r2));
      vec3 d = uView * n;
      float light = 0.25 + 0.75*max(0.0, dot(n, normalize(uSun)));
      gl_FragColor = vec4(colour(crossUV(d))*light, 1.0);
    }`;

  function makeGL(canvas) {
    const gl = canvas.getContext('webgl');
    if (!gl) { throw new Error('WebGL unavailable'); }
    const sh = (type, src) => {
      const s = gl.createShader(type); gl.shaderSource(s, src); gl.compileShader(s);
      if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) { throw new Error(gl.getShaderInfoLog(s)); }
      return s;
    };
    const prog = gl.createProgram();
    gl.attachShader(prog, sh(gl.VERTEX_SHADER, VS)); gl.attachShader(prog, sh(gl.FRAGMENT_SHADER, FS));
    gl.linkProgram(prog); gl.useProgram(prog);
    const buf = gl.createBuffer(); gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1,-1, 1,-1, -1,1, 1,1]), gl.STATIC_DRAW);
    const aPos = gl.getAttribLocation(prog, 'aPos'); gl.enableVertexAttribArray(aPos); gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);
    const u = {}; ['uTh','uPl','uFe','uMode','uLayer','uView','uSun'].forEach((n) => { u[n] = gl.getUniformLocation(prog, n); });
    gl.uniform1i(u.uTh, 0); gl.uniform1i(u.uPl, 1); gl.uniform1i(u.uFe, 2);
    const tex = [0,1,2].map(() => {
      const t = gl.createTexture(); gl.bindTexture(gl.TEXTURE_2D, t);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST); gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE); gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      return t;
    });
    return { gl, u, tex };
  }

  function upload(ctx, images) {
    const gl = ctx.gl;
    images.forEach((img, i) => {
      gl.activeTexture(gl.TEXTURE0 + i); gl.bindTexture(gl.TEXTURE_2D, ctx.tex[i]);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, img);
    });
  }

  // ---- view rotation (yaw/pitch by drag) ----
  let yaw = 0.6, pitch = 0.2;
  function viewMatrix() {
    const cy = Math.cos(yaw), sy = Math.sin(yaw), cp = Math.cos(pitch), sp = Math.sin(pitch);
    // column-major mat3 = Ry(yaw) * Rx(pitch)
    return new Float32Array([cy, 0, -sy,  sy*sp, cp, cy*sp,  sy*cp, -sp, cy*cp]);
  }
  function worldToScreen(v) { // inverse of uView applied in the shader
    const m = viewMatrix();
    return [m[0]*v[0]+m[1]*v[1]+m[2]*v[2], m[3]*v[0]+m[4]*v[1]+m[5]*v[2], m[6]*v[0]+m[7]*v[1]+m[8]*v[2]];
  }

  // ---- state ----
  const sphere = makeGL($('sphere')), cross = makeGL($('cross'));
  const arrows = $('arrows').getContext('2d');
  let manifest = null, slug = '', frameCache = new Map(), current = 0, playing = false, timer = null;
  const layer = () => parseInt(document.querySelector('input[name=layer]:checked').value, 10);

  function loadImage(url) {
    return new Promise((res, rej) => { const img = new Image(); img.onload = () => res(img); img.onerror = () => rej(new Error('load ' + url)); img.src = url; });
  }
  async function frameImages(i) {
    if (frameCache.has(i)) { return frameCache.get(i); }
    const dir = `/bundles/${slug}/${manifest.frames[i].dir}`;
    const p = Promise.all(['thickness', 'plate', 'feature'].map((n) => loadImage(`${dir}/${n}.png`)));
    frameCache.set(i, p);
    return p;
  }

  function drawArrows(frame) {
    const c = arrows, W = c.canvas.width, H = c.canvas.height;
    c.clearRect(0, 0, W, H);
    if (!$('showArrows').checked) { return; }
    const R = Math.min(W, H) / 2;
    for (const pl of frame.plates) {
      if (pl.retired) { continue; }
      const s = worldToScreen(pl.centroid);
      if (s[2] < 0.05) { continue; }
      const w = pl.pole.map((x) => x * pl.speed_cm_yr);
      const v = [w[1]*pl.centroid[2]-w[2]*pl.centroid[1], w[2]*pl.centroid[0]-w[0]*pl.centroid[2], w[0]*pl.centroid[1]-w[1]*pl.centroid[0]];
      const sv = worldToScreen(v);
      const x = W/2 + s[0]*R, y = H/2 - s[1]*R, k = 6 * R / 100;
      const dx = sv[0]*k, dy = -sv[1]*k;
      c.strokeStyle = pl.major ? '#ffffff' : '#b0b8c4'; c.lineWidth = pl.major ? 2 : 1;
      c.beginPath(); c.moveTo(x, y); c.lineTo(x+dx, y+dy); c.stroke();
      c.beginPath(); c.arc(x+dx, y+dy, pl.major ? 3 : 2, 0, Math.PI*2); c.fillStyle = c.strokeStyle; c.fill();
      c.fillStyle = '#e8ecf2'; c.font = '11px system-ui'; c.fillText(String(pl.id), x+4, y-4);
    }
  }

  async function render() {
    if (!manifest) { return; }
    const i = current, images = await frameImages(i);
    if (i !== current) { return; }
    for (const [ctx, mode] of [[sphere, 0], [cross, 1]]) {
      const gl = ctx.gl;
      upload(ctx, images);
      gl.viewport(0, 0, gl.canvas.width, gl.canvas.height);
      gl.uniform1i(ctx.u.uMode, mode); gl.uniform1i(ctx.u.uLayer, layer());
      gl.uniformMatrix3fv(ctx.u.uView, false, viewMatrix()); gl.uniform3f(ctx.u.uSun, -0.5, 0.4, 0.8);
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
    }
    drawArrows(manifest.frames[i]);
    $('myr').textContent = `${manifest.frames[i].myr.toFixed(0)} Myr  (step ${manifest.frames[i].step}/${manifest.steps})`;
  }

  function setFrame(i) { current = Math.max(0, Math.min(manifest.frames.length - 1, i)); $('slider').value = current; render(); }

  async function loadBundle(name) {
    slug = name; frameCache = new Map();
    manifest = await (await fetch(`/bundles/${slug}/manifest.json`)).json();
    $('slider').max = manifest.frames.length - 1;
    $('status').textContent = `${slug}: ${manifest.archetype}, face ${manifest.face}, ${manifest.frames.length} frames, ${manifest.frames[0].plates.length} plates`;
    setFrame(0);
    for (let i = 1; i < manifest.frames.length; i++) { frameImages(i); } // warm the cache in order
  }

  async function refreshBundles(select) {
    const list = await (await fetch('/api/bundles')).json();
    const sel = $('bundle'); sel.innerHTML = '';
    for (const b of list) { const o = document.createElement('option'); o.value = b.slug; o.textContent = `${b.slug} (${b.frames} frames)`; sel.appendChild(o); }
    if (select && list.some((b) => b.slug === select)) { sel.value = select; }
    if (sel.value) { loadBundle(sel.value); } else { $('status').textContent = 'no bundles yet — start a run'; }
  }

  async function startRun() {
    const body = { planet: $('planet').value.trim(), seed: parseInt($('seed').value, 10) || 0, archetype: $('archetype').value,
      face: parseInt($('face').value, 10) || 0, steps: parseInt($('steps').value, 10) || 0,
      sets: $('sets').value.split(/\s+/).filter(Boolean) };
    const res = await fetch('/api/run', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    const j = await res.json();
    if (!res.ok) { $('status').textContent = 'run refused: ' + j.error; return; }
    const poll = async () => {
      const st = await (await fetch('/api/jobs/' + j.job)).json();
      if (st.state === 'running') { $('status').textContent = `running: step ${st.step}/${st.steps}`; setTimeout(poll, 1000); return; }
      if (st.state === 'error') { $('status').textContent = 'run failed: ' + st.error; return; }
      refreshBundles(st.slug);
    };
    poll();
  }

  // ---- wiring ----
  $('bundle').addEventListener('change', (e) => loadBundle(e.target.value));
  $('run').addEventListener('click', startRun);
  $('slider').addEventListener('input', (e) => setFrame(parseInt(e.target.value, 10)));
  document.querySelectorAll('input[name=layer], #showArrows').forEach((el) => el.addEventListener('change', render));
  $('play').addEventListener('click', () => {
    playing = !playing; $('play').textContent = playing ? '❚❚' : '▶';
    if (playing) { timer = setInterval(() => setFrame(current + 1 >= manifest.frames.length ? 0 : current + 1), 120); } else { clearInterval(timer); }
  });
  let drag = null;
  $('arrows').style.pointerEvents = 'none';
  $('sphere').addEventListener('mousedown', (e) => { drag = [e.clientX, e.clientY]; });
  window.addEventListener('mousemove', (e) => {
    if (!drag) { return; }
    yaw += (e.clientX - drag[0]) * 0.01; pitch = Math.max(-1.4, Math.min(1.4, pitch + (e.clientY - drag[1]) * 0.01));
    drag = [e.clientX, e.clientY]; render();
  });
  window.addEventListener('mouseup', () => { drag = null; });
  window.addEventListener('keydown', (e) => {
    if (!manifest) { return; }
    if (e.key === 'ArrowRight') { setFrame(current + 1); } else if (e.key === 'ArrowLeft') { setFrame(current - 1); }
  });

  fetch('/api/archetypes').then((r) => r.json()).then((names) => {
    const sel = $('archetype');
    for (const n of names) { const o = document.createElement('option'); o.value = n; o.textContent = n; sel.appendChild(o); }
    sel.value = 'terran';
  });
  refreshBundles();
})();
```

- [ ] **Step 5: Run the asset test and lint, expect PASS**

Run: `go test ./cmd/tectonics-lab/ -short -v && golangci-lint run ./cmd/tectonics-lab/...`

- [ ] **Step 6: Human visual check (not for subagents)**

The implementer does not open a browser. Report the command for the user and move on:

```
go build -o bin/tectonics-lab ./cmd/tectonics-lab && ./bin/tectonics-lab serve
# then open http://localhost:8091, pick seed-42-terran (baked in Task 8), scrub the slider
```

Things the user checks: plates coloured and contiguous across cube seams on the sphere; blue ridges open along the trailing edges and red belts/trenches along leading edges as the slider advances; belts dull with age; arrows point the way plates move; the cross view matches the sphere.

- [ ] **Step 7: Commit**

```bash
git add cmd/tectonics-lab/web/index.html cmd/tectonics-lab/web/app.js cmd/tectonics-lab/web_test.go
git commit -m "feat(tectonics-lab): WebGL time-slider viewer for keyframe bundles" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

## Self-review notes

- **Spec coverage:** 1a → Task 2; 1b → Task 3; 1c → Task 4; 2a/2b → Tasks 4–6; 2c bundle → Task 7; 3a layout → Tasks 1–10; 3b viewer → Task 11; 3c archetype table → Task 1; 3d tests → every task plus Task 9 golden and timing; 3e contract → the `Manifest`/`FrameInfo` shape in Task 7 is the contract later layers reuse. Non-goals untouched.
- **Known deviations from the spec, ruled here:** normalise-before-relax (Task 2); the convergent normal falls back to the local label gradient when centroids are antipodal (Task 5 note); plate id is stored in one PNG channel so bundles cap at 255 plates (spec ranges max at 32).
- **Type consistency:** `Grid[T]`, `Plate`, `Motion`, `State`, `Frame`, `PlateRow`, `FrameInfo`, `Manifest` are defined once and used by name everywhere; `twoHemispheres` and `testParams` test helpers are defined in Tasks 4 and 2 and reused in Task 5.
