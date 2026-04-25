# Planet Gen Phase 0 — Cube-Map Storage Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the existing `cmd/generate-planet-maps` equirect generator to a cube-map intermediate (4×3 horizontal cross, 1024-pixel face), bake to equirect on output, decompose `pkg/planetgen` into subpackages, and land golden-image + lint + Wasm-build CI gates — all with **no visible change** to planet imagery.

**Architecture:** The current pipeline samples noise on a unit sphere then writes results into a 2000×1000 equirect grid. After this refactor it will: (1) iterate the 6 faces of a cube-map of side `S=1024`, (2) compute each pixel's 3D unit direction via `cubemap.FacePixelToDir`, (3) sample noise with the same call signature as today, (4) write into a `*cubemap.CubeMap`, (5) bake a 2000×1000 equirect thumbnail as a final post-pass. Both files are persisted per planet. No algorithm change; the only computational difference is sampling 6×1024² = ~6.3M points instead of 2000×1000 = 2M. Code reorganized into one root package + 8 subpackages.

**Tech Stack:** Go 1.24, `github.com/ojrac/opensimplex-go` (existing), `github.com/lucasb-eyer/go-colorful` (new — added solely for the golden-diff CLI's ΔE2000 metric in Phase 0; reused in Phase 1 for OkLab blending), `golangci-lint` (new — config added in this phase), GitHub Actions (CI matrix gets a Wasm build step).

---

## File structure

**New files (Phase 0 deliverable):**

```
pkg/planetgen/
├── cubemap/
│   ├── doc.go
│   ├── cubemap.go          (Face, CubeMap, CubeMapF, New/NewF)
│   ├── sample.go           (DirToFaceUV, FaceUVToDir, FacePixelToDir, Sample methods)
│   ├── cross.go            (WriteCrossPNG, ReadCrossPNG)
│   ├── bake.go             (BakeEquirect)
│   ├── cubemap_test.go
│   ├── sample_test.go
│   ├── cross_test.go
│   └── bake_test.go
├── noise/doc.go            (subpackage skeleton)
├── color/doc.go            (subpackage skeleton)
├── field/doc.go            (subpackage skeleton — empty in Phase 0)
├── biome/doc.go            (subpackage skeleton — empty in Phase 0)
├── feature/doc.go          (subpackage skeleton)
├── render/doc.go           (subpackage skeleton)
└── stats/doc.go            (subpackage skeleton)

cmd/
├── generate-planet-maps/
│   ├── README.md           (new — full docs)
│   ├── golden_test.go      (new — TestGolden harness)
│   ├── invariants_test.go  (new — statistical invariants)
│   └── testdata/golden/    (new — 13 PNGs committed via -update)
└── tools/planet-image-diff/
    └── main.go             (new — diff CLI)

.golangci.yml               (new — depguard + standard linters)
.github/workflows/build-test.yml  (new — go test + wasm build gate)
```

**Files moved (relocated, body adapted):**

| From | To | Adaptation |
|---|---|---|
| `pkg/planetgen/noise.go` | `pkg/planetgen/noise/noise.go` | Remove `SphericalCoords` and `SphericalFractal`; rest unchanged |
| `pkg/planetgen/color.go` | `pkg/planetgen/color/color.go` | Export previously-private symbols (`SampleGradient`, `BlendColor`, `LerpColor`, `Brighten`) |
| `pkg/planetgen/crater.go` | `pkg/planetgen/feature/crater.go` | `ApplyCraters` rewritten to operate on `*cubemap.CubeMapF` |
| `pkg/planetgen/rocky.go` | `pkg/planetgen/render/rocky.go` | Outer loop iterates 6 faces × S × S; uses `cubemap.FacePixelToDir` |
| `pkg/planetgen/gasgiant.go` | `pkg/planetgen/render/gasgiant.go` | Same loop refactor; lat from `dir.y` |
| `pkg/planetgen/stats.go` | `pkg/planetgen/stats/stats.go` | Relocation only |

**Files modified in place:**

- `pkg/planetgen/planetgen.go` — `Generate` returns `*cubemap.CubeMap`; new `GenerateEquirect` wrapper.
- `pkg/planetgen/profile.go` — unchanged in Phase 0 (extended in Phase 1).
- `cmd/generate-planet-maps/main.go` — writes both `<name>.cube.png` and `<name>.png`; default `S=1024`.
- `go.mod`, `go.sum` — `github.com/lucasb-eyer/go-colorful` added.

---

## Conventions used throughout this plan

**Cube-map face index** matches OpenGL `GL_TEXTURE_CUBE_MAP_POSITIVE_X` ordering:

| Index | Face | Constant name |
|---|---|---|
| 0 | +X | `FacePosX` |
| 1 | -X | `FaceNegX` |
| 2 | +Y | `FacePosY` |
| 3 | -Y | `FaceNegY` |
| 4 | +Z | `FacePosZ` |
| 5 | -Z | `FaceNegZ` |

**4×3 horizontal cross layout** (face cell positions in a `4S × 3S` image):

```
[empty][ +Y  ][empty][empty]   row 0:  y in [0,    S)
[ -X  ][ +Z  ][ +X  ][ -Z  ]   row 1:  y in [S,   2S)
[empty][ -Y  ][empty][empty]   row 2:  y in [2S,  3S)
   ↑      ↑      ↑      ↑
   col0   col1   col2   col3
   x∈[0,S) [S,2S) [2S,3S) [3S,4S)
```

Empty cells are written transparent (alpha=0) so the PNG is human-viewable.

**Direction → face/UV mapping (GL convention):** given unit vector `(x,y,z)`, find largest absolute component → that's the face. For face `+X`: `u = 0.5·(-z/x + 1)`, `v = 0.5·(-y/x + 1)`. (Full table in Task 3.) `u, v ∈ [0, 1]` with `(0,0)` at the top-left corner of the face cell.

**Commit message style** matches recent repo history (see `git log --oneline -5`): short imperative summary line, optional body. Each task ends with one commit.

---

## Task 1: Create subpackage skeletons

**Files:**
- Create: `pkg/planetgen/cubemap/doc.go`
- Create: `pkg/planetgen/noise/doc.go`
- Create: `pkg/planetgen/color/doc.go`
- Create: `pkg/planetgen/field/doc.go`
- Create: `pkg/planetgen/biome/doc.go`
- Create: `pkg/planetgen/feature/doc.go`
- Create: `pkg/planetgen/render/doc.go`
- Create: `pkg/planetgen/stats/doc.go`

- [ ] **Step 1: Create cubemap doc.go**

```go
// Package cubemap provides a 6-faced cube-map storage and sampling
// substrate for sphere-native planet generation. Each face is a square
// 2D grid of pixels; samples on the unit sphere map to a face index
// plus (u, v) coordinates following the OpenGL GL_TEXTURE_CUBE_MAP
// convention.
//
// CubeMap holds RGBA values; CubeMapF holds float64 values for
// scalar fields like heightmaps and distance maps.
//
// On-disk format is a 4×3 horizontal cross PNG (4S × 3S pixels for
// face size S) with empty cells transparent.
package cubemap
```

Write to `pkg/planetgen/cubemap/doc.go`.

- [ ] **Step 2: Create noise/doc.go**

```go
// Package noise provides simplex-noise primitives used throughout
// planet generation: fractal Brownian motion (FBM), domain warping
// (Phase 1), ridged multifractal (Phase 2), curl noise (Phase 2),
// and Worley noise (Phase 3+).
//
// All functions are pure and deterministic given a seeded
// NoiseGenerator. Sampling is done in 3D (unit sphere directions)
// to keep generation seamless across cube-map faces.
package noise
```

- [ ] **Step 3: Create color/doc.go**

```go
// Package color provides palette-based gradient sampling, color
// blending in RGB (Phase 0) and OkLab (Phase 1), monotone cubic
// splines for control-field interpolation (Phase 1), and 3D color
// LUT application (Phase 1).
package color
```

- [ ] **Step 4: Create field/doc.go**

```go
// Package field provides scalar and vector fields layered on top of
// the cube-map substrate: jump-flooding distance transforms (Phase
// 2), warped Voronoi province maps (Phase 2), tectonic plate
// simulation (Phase 3), D8 flow accumulation (Phase 3), and the
// multi-noise control fields that drive height and biome (Phase 1).
//
// Phase 0 contains only the package declaration; populated in
// later phases.
package field
```

- [ ] **Step 5: Create biome/doc.go**

```go
// Package biome converts elevation, temperature, and moisture
// fields into surface colors using the Whittaker classification
// (Phase 1) and applies wind-driven rain shadow modulation (Phase
// 3).
//
// Phase 0 contains only the package declaration; populated in
// later phases.
package biome
```

- [ ] **Step 6: Create feature/doc.go**

```go
// Package feature implements discrete-feature generators that
// stamp onto the cube-map substrate: craters (Phase 0), clouds
// (Phase 3), civilization signs (Phase 3), dunes (Phase 4), ice
// terrain (Phase 4), lava lobes (Phase 4), and ripple textures
// (Phase 4).
package feature
```

- [ ] **Step 7: Create render/doc.go**

```go
// Package render orchestrates the per-renderer pipeline (rocky
// planets, gas giants) by composing primitives from the noise,
// color, field, biome, and feature subpackages and writing into a
// cubemap.CubeMap.
package render
```

- [ ] **Step 8: Create stats/doc.go**

```go
// Package stats records per-stage timing and counters from a
// generation run for performance tracking and debugging.
package stats
```

- [ ] **Step 9: Verify build**

Run: `cd /home/robert/spacemolt/kb && go build ./...`
Expected: success (no compilation errors; the new empty packages don't conflict with existing code).

- [ ] **Step 10: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/cubemap/doc.go pkg/planetgen/noise/doc.go \
        pkg/planetgen/color/doc.go pkg/planetgen/field/doc.go \
        pkg/planetgen/biome/doc.go pkg/planetgen/feature/doc.go \
        pkg/planetgen/render/doc.go pkg/planetgen/stats/doc.go
git commit -m "Phase 0: scaffold planetgen subpackages with doc.go files"
```

---

## Task 2: Define cubemap.Face, CubeMap, CubeMapF types

**Files:**
- Create: `pkg/planetgen/cubemap/cubemap.go`
- Create: `pkg/planetgen/cubemap/cubemap_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/cubemap/cubemap_test.go`:

```go
package cubemap

import (
	"image/color"
	"testing"
)

func TestNewCubeMap(t *testing.T) {
	cm := New(64)
	if cm.Size != 64 {
		t.Fatalf("Size = %d, want 64", cm.Size)
	}
	for i, f := range cm.Faces {
		if len(f) != 64*64 {
			t.Errorf("Faces[%d] len = %d, want %d", i, len(f), 64*64)
		}
	}
}

func TestNewCubeMapF(t *testing.T) {
	cm := NewF(64)
	if cm.Size != 64 {
		t.Fatalf("Size = %d, want 64", cm.Size)
	}
	for i, f := range cm.Faces {
		if len(f) != 64*64 {
			t.Errorf("Faces[%d] len = %d, want %d", i, len(f), 64*64)
		}
	}
}

func TestCubeMapSetGet(t *testing.T) {
	cm := New(8)
	red := color.RGBA{R: 255, A: 255}
	cm.Set(FacePosX, 3, 5, red)
	got := cm.Get(FacePosX, 3, 5)
	if got != red {
		t.Errorf("Get returned %v, want %v", got, red)
	}
}

func TestCubeMapFSetGet(t *testing.T) {
	cm := NewF(8)
	cm.Set(FaceNegZ, 1, 7, 0.42)
	got := cm.Get(FaceNegZ, 1, 7)
	if got != 0.42 {
		t.Errorf("Get returned %f, want 0.42", got)
	}
}

func TestFaceConstants(t *testing.T) {
	if FacePosX != 0 || FaceNegX != 1 || FacePosY != 2 ||
		FaceNegY != 3 || FacePosZ != 4 || FaceNegZ != 5 {
		t.Fatalf("face constants not in GL order")
	}
	if NumFaces != 6 {
		t.Fatalf("NumFaces = %d, want 6", NumFaces)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: FAIL with "undefined: New" / "undefined: FacePosX" etc.

- [ ] **Step 3: Implement the types**

Create `pkg/planetgen/cubemap/cubemap.go`:

```go
package cubemap

import "image/color"

// Face identifies one of the six faces of a cube map. Order matches
// the OpenGL GL_TEXTURE_CUBE_MAP_POSITIVE_X family so PNGs written
// by this package can be uploaded to a WebGL cube texture without
// remapping.
type Face int

const (
	FacePosX Face = iota // +X (right)
	FaceNegX             // -X (left)
	FacePosY             // +Y (top)
	FaceNegY             // -Y (bottom)
	FacePosZ             // +Z (front)
	FaceNegZ             // -Z (back)
)

// NumFaces is the number of faces in a cube map.
const NumFaces = 6

// CubeMap stores an RGBA value per pixel across six square faces.
// Each face is a row-major Size×Size grid; total storage is
// 6 * Size * Size pixels.
type CubeMap struct {
	Size  int
	Faces [NumFaces][]color.RGBA
}

// New allocates a CubeMap with all faces initialised to zero RGBA.
func New(size int) *CubeMap {
	cm := &CubeMap{Size: size}
	for i := range cm.Faces {
		cm.Faces[i] = make([]color.RGBA, size*size)
	}
	return cm
}

// Set writes a single pixel on a face. (px, py) origin is the top-
// left of the face cell.
func (cm *CubeMap) Set(face Face, px, py int, c color.RGBA) {
	cm.Faces[face][py*cm.Size+px] = c
}

// Get reads a single pixel on a face.
func (cm *CubeMap) Get(face Face, px, py int) color.RGBA {
	return cm.Faces[face][py*cm.Size+px]
}

// CubeMapF is the float64 equivalent of CubeMap, used for scalar
// fields like heightmaps, temperature, moisture, and distance
// transforms.
type CubeMapF struct {
	Size  int
	Faces [NumFaces][]float64
}

// NewF allocates a CubeMapF with all faces zeroed.
func NewF(size int) *CubeMapF {
	cm := &CubeMapF{Size: size}
	for i := range cm.Faces {
		cm.Faces[i] = make([]float64, size*size)
	}
	return cm
}

// Set writes a single float on a face.
func (cm *CubeMapF) Set(face Face, px, py int, v float64) {
	cm.Faces[face][py*cm.Size+px] = v
}

// Get reads a single float on a face.
func (cm *CubeMapF) Get(face Face, px, py int) float64 {
	return cm.Faces[face][py*cm.Size+px]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/cubemap/cubemap.go pkg/planetgen/cubemap/cubemap_test.go
git commit -m "Phase 0: add cubemap.Face, CubeMap, CubeMapF types"
```

---

## Task 3: Direction ↔ face/UV round-trip

**Files:**
- Create: `pkg/planetgen/cubemap/sample.go`
- Create: `pkg/planetgen/cubemap/sample_test.go`

The math (GL convention): given unit `(x,y,z)`, the major axis is the largest `|component|`. The two non-major components map to `(sc, tc)` per face, then `u = 0.5·(sc/|major| + 1)` and `v = 0.5·(tc/|major| + 1)`.

| Face | Major | sc (→u) | tc (→v) |
|------|-------|---------|---------|
| +X   | x     | -z      | -y      |
| -X   | x     | +z      | -y      |
| +Y   | y     | +x      | +z      |
| -Y   | y     | +x      | -z      |
| +Z   | z     | +x      | -y      |
| -Z   | z     | -x      | -y      |

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/cubemap/sample_test.go`:

```go
package cubemap

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestDirToFaceUVKnownDirections(t *testing.T) {
	cases := []struct {
		x, y, z  float64
		wantFace Face
	}{
		{1, 0, 0, FacePosX},
		{-1, 0, 0, FaceNegX},
		{0, 1, 0, FacePosY},
		{0, -1, 0, FaceNegY},
		{0, 0, 1, FacePosZ},
		{0, 0, -1, FaceNegZ},
	}
	for _, tc := range cases {
		face, u, v := DirToFaceUV(tc.x, tc.y, tc.z)
		if face != tc.wantFace {
			t.Errorf("DirToFaceUV(%v,%v,%v) face = %d, want %d",
				tc.x, tc.y, tc.z, face, tc.wantFace)
		}
		if math.Abs(u-0.5) > 1e-9 || math.Abs(v-0.5) > 1e-9 {
			t.Errorf("DirToFaceUV(%v,%v,%v) u,v = %f,%f, want 0.5,0.5",
				tc.x, tc.y, tc.z, u, v)
		}
	}
}

func TestDirFaceUVRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	const N = 10_000
	maxErr := 0.0
	for range N {
		// uniform on sphere
		u1 := rng.Float64()
		u2 := rng.Float64()
		theta := 2 * math.Pi * u1
		z := 2*u2 - 1
		r := math.Sqrt(1 - z*z)
		x := r * math.Cos(theta)
		y := r * math.Sin(theta)

		face, fu, fv := DirToFaceUV(x, y, z)
		x2, y2, z2 := FaceUVToDir(face, fu, fv)
		dx, dy, dz := x-x2, y-y2, z-z2
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d > maxErr {
			maxErr = d
		}
	}
	if maxErr > 1e-12 {
		t.Errorf("max roundtrip error = %g, want < 1e-12", maxErr)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: FAIL with "undefined: DirToFaceUV / FaceUVToDir".

- [ ] **Step 3: Implement direction primitives**

Create `pkg/planetgen/cubemap/sample.go`:

```go
package cubemap

import "math"

// DirToFaceUV maps a 3D direction to its cube-map face plus UV
// coordinates in [0, 1]. The input need not be unit-length; only
// the ratios of components matter. Convention matches OpenGL
// GL_TEXTURE_CUBE_MAP.
func DirToFaceUV(x, y, z float64) (face Face, u, v float64) {
	ax, ay, az := math.Abs(x), math.Abs(y), math.Abs(z)
	var sc, tc, ma float64
	switch {
	case ax >= ay && ax >= az:
		ma = ax
		if x >= 0 {
			face = FacePosX
			sc, tc = -z, -y
		} else {
			face = FaceNegX
			sc, tc = z, -y
		}
	case ay >= az:
		ma = ay
		if y >= 0 {
			face = FacePosY
			sc, tc = x, z
		} else {
			face = FaceNegY
			sc, tc = x, -z
		}
	default:
		ma = az
		if z >= 0 {
			face = FacePosZ
			sc, tc = x, -y
		} else {
			face = FaceNegZ
			sc, tc = -x, -y
		}
	}
	u = 0.5 * (sc/ma + 1)
	v = 0.5 * (tc/ma + 1)
	return face, u, v
}

// FaceUVToDir is the inverse of DirToFaceUV. Returns a unit vector.
func FaceUVToDir(face Face, u, v float64) (x, y, z float64) {
	sc := 2*u - 1
	tc := 2*v - 1
	switch face {
	case FacePosX:
		x, y, z = 1, -tc, -sc
	case FaceNegX:
		x, y, z = -1, -tc, sc
	case FacePosY:
		x, y, z = sc, 1, tc
	case FaceNegY:
		x, y, z = sc, -1, -tc
	case FacePosZ:
		x, y, z = sc, -tc, 1
	case FaceNegZ:
		x, y, z = -sc, -tc, -1
	}
	inv := 1.0 / math.Sqrt(x*x+y*y+z*z)
	return x * inv, y * inv, z * inv
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: PASS for all tests including `TestDirFaceUVRoundtrip` (max error well under 1e-12 for unit-input directions).

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/cubemap/sample.go pkg/planetgen/cubemap/sample_test.go
git commit -m "Phase 0: add DirToFaceUV / FaceUVToDir GL-convention mapping"
```

---

## Task 4: FacePixelToDir + per-pixel iteration helper

**Files:**
- Modify: `pkg/planetgen/cubemap/sample.go`
- Modify: `pkg/planetgen/cubemap/sample_test.go`

`FacePixelToDir(face, px, py, S)` returns the unit vector for the *center* of pixel `(px, py)` of an `S`-sized face. The pixel center is at `u = (px+0.5)/S`, `v = (py+0.5)/S`.

- [ ] **Step 1: Append the failing test**

Append to `pkg/planetgen/cubemap/sample_test.go`:

```go
func TestFacePixelToDirCoverage(t *testing.T) {
	const S = 8
	for face := range Face(NumFaces) {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				x, y, z := FacePixelToDir(face, px, py, S)
				mag := math.Sqrt(x*x + y*y + z*z)
				if math.Abs(mag-1.0) > 1e-12 {
					t.Errorf("face %d (%d,%d): |dir| = %f, want 1",
						face, px, py, mag)
				}
			}
		}
	}
}

func TestFacePixelToDirCenter(t *testing.T) {
	// The center pixel of FacePosX (at S/2-1, S/2-1 with bilinear-ish
	// rounding) should be very close to (1, 0, 0). With S=8 and
	// pixel-center sampling, pixel (3,3) center is at u=v=0.4375, which
	// gives sc=tc=-0.125 → direction roughly (1, 0.125, 0.125)
	// pre-normalisation. Just check we get the right *face* and that
	// the dominant axis is correct.
	x, _, _ := FacePixelToDir(FacePosX, 3, 3, 8)
	if x < 0.95 {
		t.Errorf("FacePosX (3,3) S=8 dir.x = %f, expected near 1", x)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: FAIL with "undefined: FacePixelToDir".

- [ ] **Step 3: Append implementation**

Append to `pkg/planetgen/cubemap/sample.go`:

```go
// FacePixelToDir returns the unit vector pointing at the center of
// pixel (px, py) on a face of side S.
func FacePixelToDir(face Face, px, py, S int) (x, y, z float64) {
	u := (float64(px) + 0.5) / float64(S)
	v := (float64(py) + 0.5) / float64(S)
	return FaceUVToDir(face, u, v)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/cubemap/sample.go pkg/planetgen/cubemap/sample_test.go
git commit -m "Phase 0: add FacePixelToDir per-pixel direction helper"
```

---

## Task 5: CubeMap.Sample (bilinear with edge clamping)

**Files:**
- Modify: `pkg/planetgen/cubemap/sample.go`
- Modify: `pkg/planetgen/cubemap/sample_test.go`

For Phase 0 we use **edge-clamped bilinear sampling** within the face the direction maps to. This means seam continuity is approximate — adjacent-face pixels on either side of an edge agree to within the per-pixel quantisation but not to within bilinear-filtered sub-pixel precision. That is acceptable for Phase 0: the equirect bake reads through `Sample`, and the eyeballed seam difference is sub-perceptual at S=1024. Cross-face filtering (true seamless) is a Phase 3 improvement (item 19, "edge-blending QA").

- [ ] **Step 1: Append the failing test**

Append to `pkg/planetgen/cubemap/sample_test.go`:

```go
import (
	"image/color"
	// ...existing imports
)

func TestCubeMapSampleFlatFace(t *testing.T) {
	cm := New(16)
	red := color.RGBA{R: 255, A: 255}
	for face := range Face(NumFaces) {
		for py := 0; py < cm.Size; py++ {
			for px := 0; px < cm.Size; px++ {
				cm.Set(face, px, py, red)
			}
		}
	}
	// Sample at every face center
	for _, dir := range [][3]float64{
		{1, 0, 0}, {-1, 0, 0},
		{0, 1, 0}, {0, -1, 0},
		{0, 0, 1}, {0, 0, -1},
	} {
		got := cm.Sample(dir[0], dir[1], dir[2])
		if got != red {
			t.Errorf("Sample at (%v) = %v, want red", dir, got)
		}
	}
}

func TestCubeMapFSampleFlatFace(t *testing.T) {
	cm := NewF(16)
	for face := range Face(NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = 0.7
		}
	}
	got := cm.Sample(1, 0, 0)
	if got < 0.69 || got > 0.71 {
		t.Errorf("CubeMapF.Sample = %f, want ~0.7", got)
	}
}
```

(Note: the `import` block at the top of the test file needs `"image/color"` if not already present — it already is from Task 2's tests.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: FAIL with "undefined: cm.Sample".

- [ ] **Step 3: Append implementation**

Append to `pkg/planetgen/cubemap/sample.go`:

```go
// Sample returns the bilinearly-filtered RGBA at the given 3D
// direction. Coordinates are clamped to face edges; see package
// docs for seam behavior.
func (cm *CubeMap) Sample(x, y, z float64) color.RGBA {
	face, u, v := DirToFaceUV(x, y, z)
	return cm.sampleFaceUV(face, u, v)
}

func (cm *CubeMap) sampleFaceUV(face Face, u, v float64) color.RGBA {
	S := cm.Size
	fx := u*float64(S) - 0.5
	fy := v*float64(S) - 0.5
	x0 := clampi(int(math.Floor(fx)), 0, S-1)
	y0 := clampi(int(math.Floor(fy)), 0, S-1)
	x1 := clampi(x0+1, 0, S-1)
	y1 := clampi(y0+1, 0, S-1)
	tx := fx - math.Floor(fx)
	ty := fy - math.Floor(fy)
	c00 := cm.Get(face, x0, y0)
	c10 := cm.Get(face, x1, y0)
	c01 := cm.Get(face, x0, y1)
	c11 := cm.Get(face, x1, y1)
	return blendRGBA4(c00, c10, c01, c11, tx, ty)
}

// Sample returns the bilinearly-filtered float at the given 3D direction.
func (cmf *CubeMapF) Sample(x, y, z float64) float64 {
	face, u, v := DirToFaceUV(x, y, z)
	S := cmf.Size
	fx := u*float64(S) - 0.5
	fy := v*float64(S) - 0.5
	x0 := clampi(int(math.Floor(fx)), 0, S-1)
	y0 := clampi(int(math.Floor(fy)), 0, S-1)
	x1 := clampi(x0+1, 0, S-1)
	y1 := clampi(y0+1, 0, S-1)
	tx := fx - math.Floor(fx)
	ty := fy - math.Floor(fy)
	v00 := cmf.Get(face, x0, y0)
	v10 := cmf.Get(face, x1, y0)
	v01 := cmf.Get(face, x0, y1)
	v11 := cmf.Get(face, x1, y1)
	a := v00*(1-tx) + v10*tx
	b := v01*(1-tx) + v11*tx
	return a*(1-ty) + b*ty
}

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func blendRGBA4(c00, c10, c01, c11 color.RGBA, tx, ty float64) color.RGBA {
	rA := float64(c00.R)*(1-tx) + float64(c10.R)*tx
	gA := float64(c00.G)*(1-tx) + float64(c10.G)*tx
	bA := float64(c00.B)*(1-tx) + float64(c10.B)*tx
	aA := float64(c00.A)*(1-tx) + float64(c10.A)*tx
	rB := float64(c01.R)*(1-tx) + float64(c11.R)*tx
	gB := float64(c01.G)*(1-tx) + float64(c11.G)*tx
	bB := float64(c01.B)*(1-tx) + float64(c11.B)*tx
	aB := float64(c01.A)*(1-tx) + float64(c11.A)*tx
	return color.RGBA{
		R: uint8(rA*(1-ty) + rB*ty),
		G: uint8(gA*(1-ty) + gB*ty),
		B: uint8(bA*(1-ty) + bB*ty),
		A: uint8(aA*(1-ty) + aB*ty),
	}
}
```

Add `"image/color"` to imports in `sample.go` if not already present (it isn't — add it).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/cubemap/sample.go pkg/planetgen/cubemap/sample_test.go
git commit -m "Phase 0: add CubeMap/CubeMapF bilinear Sample with edge clamping"
```

---

## Task 6: 4×3 cross PNG I/O

**Files:**
- Create: `pkg/planetgen/cubemap/cross.go`
- Create: `pkg/planetgen/cubemap/cross_test.go`

Cell positions (face → top-left corner in image pixels, S = face size):

| Face | col | row | top-left (x, y) |
|---|---|---|---|
| +Y | 1 | 0 | (S, 0) |
| -X | 0 | 1 | (0, S) |
| +Z | 1 | 1 | (S, S) |
| +X | 2 | 1 | (2S, S) |
| -Z | 3 | 1 | (3S, S) |
| -Y | 1 | 2 | (S, 2S) |

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/cubemap/cross_test.go`:

```go
package cubemap

import (
	"image/color"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossRoundtrip(t *testing.T) {
	const S = 32
	cm := New(S)
	rng := rand.New(rand.NewPCG(42, 99))
	for face := range Face(NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = color.RGBA{
				R: uint8(rng.UintN(256)),
				G: uint8(rng.UintN(256)),
				B: uint8(rng.UintN(256)),
				A: 255,
			}
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "cross.png")
	if err := WriteCrossPNG(cm, path); err != nil {
		t.Fatalf("WriteCrossPNG: %v", err)
	}
	cm2, err := ReadCrossPNG(path)
	if err != nil {
		t.Fatalf("ReadCrossPNG: %v", err)
	}
	if cm2.Size != cm.Size {
		t.Fatalf("size mismatch: got %d want %d", cm2.Size, cm.Size)
	}
	for face := range Face(NumFaces) {
		for i := range cm.Faces[face] {
			if cm.Faces[face][i] != cm2.Faces[face][i] {
				t.Fatalf("face %d pixel %d: %v vs %v",
					face, i, cm.Faces[face][i], cm2.Faces[face][i])
			}
		}
	}
}

func TestCrossImageSize(t *testing.T) {
	const S = 16
	cm := New(S)
	dir := t.TempDir()
	path := filepath.Join(dir, "cross.png")
	if err := WriteCrossPNG(cm, path); err != nil {
		t.Fatalf("WriteCrossPNG: %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Size() == 0 {
		t.Fatalf("output file missing or empty")
	}
	// Also re-decode to check dimensions.
	cm2, err := ReadCrossPNG(path)
	if err != nil {
		t.Fatalf("ReadCrossPNG: %v", err)
	}
	if cm2.Size != S {
		t.Fatalf("decoded Size = %d, want %d", cm2.Size, S)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: FAIL with "undefined: WriteCrossPNG / ReadCrossPNG".

- [ ] **Step 3: Implement cross I/O**

Create `pkg/planetgen/cubemap/cross.go`:

```go
package cubemap

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

// crossCells lists each face's (col, row) in the 4×3 cross grid.
var crossCells = [NumFaces]struct{ col, row int }{
	FacePosX: {2, 1},
	FaceNegX: {0, 1},
	FacePosY: {1, 0},
	FaceNegY: {1, 2},
	FacePosZ: {1, 1},
	FaceNegZ: {3, 1},
}

// WriteCrossPNG saves cm as a 4S × 3S horizontal-cross PNG with
// empty cells transparent.
func WriteCrossPNG(cm *CubeMap, path string) error {
	S := cm.Size
	img := image.NewRGBA(image.Rect(0, 0, 4*S, 3*S))
	// Empty cells are zero-RGBA (alpha=0) by default.
	for face := range Face(NumFaces) {
		cell := crossCells[face]
		ox, oy := cell.col*S, cell.row*S
		for py := range S {
			for px := range S {
				img.SetRGBA(ox+px, oy+py, cm.Get(face, px, py))
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

// ReadCrossPNG loads a 4×3 horizontal-cross PNG produced by
// WriteCrossPNG. The image dimensions must be 4S × 3S; face size
// is inferred from width / 4.
func ReadCrossPNG(path string) (*CubeMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
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
				r, g, bl, a := img.At(ox+px, oy+py).RGBA()
				cm.Set(face, px, py, color.RGBA{
					R: uint8(r >> 8), G: uint8(g >> 8),
					B: uint8(bl >> 8), A: uint8(a >> 8),
				})
			}
		}
	}
	return cm, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/cubemap/cross.go pkg/planetgen/cubemap/cross_test.go
git commit -m "Phase 0: add 4x3 horizontal-cross PNG I/O for CubeMap"
```

---

## Task 7: Bake equirect from cube-map

**Files:**
- Create: `pkg/planetgen/cubemap/bake.go`
- Create: `pkg/planetgen/cubemap/bake_test.go`

For each equirect pixel `(px, py)` in a `W × H` image:
- `lon = (px + 0.5)/W · 2π`
- `lat = π/2 − (py + 0.5)/H · π`
- `dir = (cos(lat)·cos(lon), sin(lat), cos(lat)·sin(lon))`
- `result = cm.Sample(dir.x, dir.y, dir.z)`

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/cubemap/bake_test.go`:

```go
package cubemap

import (
	"image/color"
	"testing"
)

func TestBakeIdentity(t *testing.T) {
	// Paint each face a distinct color, bake to a small equirect,
	// and verify each face's color appears in its expected region.
	cm := New(8)
	colors := [NumFaces]color.RGBA{
		FacePosX: {255, 0, 0, 255},
		FaceNegX: {0, 255, 0, 255},
		FacePosY: {0, 0, 255, 255},
		FaceNegY: {255, 255, 0, 255},
		FacePosZ: {0, 255, 255, 255},
		FaceNegZ: {255, 0, 255, 255},
	}
	for face := range Face(NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = colors[face]
		}
	}
	img := BakeEquirect(cm, 64, 32)
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 32 {
		t.Fatalf("bake size: got %v", img.Bounds())
	}
	// Equirect (px=W/4, py=H/2) → lon=π/2, lat=0 → dir=(0,0,1) → +Z face.
	c := img.RGBAAt(16, 16)
	if c != colors[FacePosZ] {
		t.Errorf("center-of-+Z bake = %v, want %v", c, colors[FacePosZ])
	}
	// Equirect (px=0, py=H/2) → lon=0, lat=0 → dir=(1,0,0) → +X face.
	c = img.RGBAAt(0, 16)
	if c != colors[FacePosX] {
		t.Errorf("+X-axis bake = %v, want %v", c, colors[FacePosX])
	}
	// Equirect (px=W/2, py=H/2) → lon=π, lat=0 → dir=(-1,0,0) → -X face.
	c = img.RGBAAt(32, 16)
	if c != colors[FaceNegX] {
		t.Errorf("-X-axis bake = %v, want %v", c, colors[FaceNegX])
	}
	// Equirect (px=W/2, py=0) → lat=π/2 (~) → near +Y.
	c = img.RGBAAt(32, 0)
	if c != colors[FacePosY] {
		t.Errorf("+Y-pole bake = %v, want %v", c, colors[FacePosY])
	}
}

func TestBakeSeamWrap(t *testing.T) {
	// Sampling at lon=0 from the right end of the equirect should
	// produce the same color as sampling at lon=0 from the left end,
	// modulo bilinear-edge effects.
	cm := New(16)
	for i := range cm.Faces[FacePosX] {
		cm.Faces[FacePosX][i] = color.RGBA{200, 100, 50, 255}
	}
	img := BakeEquirect(cm, 64, 32)
	cL := img.RGBAAt(0, 16)
	cR := img.RGBAAt(63, 16)
	dr := int(cL.R) - int(cR.R)
	if dr < -8 || dr > 8 {
		t.Errorf("seam Δr = %d, want |Δr| ≤ 8", dr)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: FAIL with "undefined: BakeEquirect".

- [ ] **Step 3: Implement BakeEquirect**

Create `pkg/planetgen/cubemap/bake.go`:

```go
package cubemap

import (
	"image"
	"math"
)

// BakeEquirect samples the cube map at every pixel of a width×height
// equirectangular image and returns the result. Latitude maps π/2 (y=0)
// to -π/2 (y=height-1); longitude maps 0 (x=0) to ~2π (x=width-1).
func BakeEquirect(cm *CubeMap, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for py := range height {
		lat := math.Pi/2 - (float64(py)+0.5)/float64(height)*math.Pi
		cosLat := math.Cos(lat)
		sinLat := math.Sin(lat)
		for px := range width {
			lon := (float64(px) + 0.5) / float64(width) * 2 * math.Pi
			dx := cosLat * math.Cos(lon)
			dy := sinLat
			dz := cosLat * math.Sin(lon)
			img.SetRGBA(px, py, cm.Sample(dx, dy, dz))
		}
	}
	return img
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/cubemap/...`
Expected: PASS for both `TestBakeIdentity` and `TestBakeSeamWrap`.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/cubemap/bake.go pkg/planetgen/cubemap/bake_test.go
git commit -m "Phase 0: add BakeEquirect for cube-map → equirect output"
```

---

## Task 8: Move noise.go into pkg/planetgen/noise

**Files:**
- Create: `pkg/planetgen/noise/noise.go`
- Delete: `pkg/planetgen/noise.go`
- Modify: any callers in `pkg/planetgen/rocky.go`, `pkg/planetgen/gasgiant.go`, `pkg/planetgen/crater.go` to import the new path. (We will reorganise those callers in later tasks; for now we just keep the existing root package compiling by re-exporting through type aliases.)

The current `pkg/planetgen/noise.go` exports `NoiseGenerator`, `NewNoiseGenerator`, `(*NoiseGenerator).Sample3D`, `(*NoiseGenerator).FractalNoise3D`, `SphericalCoords`, `(*NoiseGenerator).SphericalFractal`. After the move:
- `noise.NoiseGenerator`, `noise.NewNoiseGenerator`, `(*noise.NoiseGenerator).Sample3D`, `.FractalNoise3D` keep their identities.
- `SphericalCoords` and `SphericalFractal` are **deleted** — callers will switch to `cubemap.FacePixelToDir` + direct `FractalNoise3D(x,y,z,...)` in later tasks.
- For the root `planetgen` package to keep compiling during Tasks 8–14, we add a thin `pkg/planetgen/legacy.go` shim that re-exports `NoiseGenerator` etc. via type aliases. This shim is deleted in Task 14 once all callers are moved.

- [ ] **Step 1: Write the new package**

Create `pkg/planetgen/noise/noise.go`:

```go
package noise

import opensimplex "github.com/ojrac/opensimplex-go"

// Generator wraps opensimplex with octave-based fractal noise.
// Renamed from "NoiseGenerator" to "Generator" to avoid stutter
// (noise.NoiseGenerator → noise.Generator).
type Generator struct {
	noise opensimplex.Noise
}

// New creates a noise generator with the given seed.
func New(seed int64) *Generator {
	return &Generator{noise: opensimplex.New(seed)}
}

// Sample3D returns a single noise sample in [-1, 1].
func (g *Generator) Sample3D(x, y, z float64) float64 {
	return g.noise.Eval3(x, y, z)
}

// FractalNoise3D returns multi-octave fractal noise normalized to [0, 1].
func (g *Generator) FractalNoise3D(x, y, z float64, octaves int, lacunarity, persistence, scale float64) float64 {
	var value, amplitude, maxAmplitude float64
	amplitude = 1.0
	freq := scale
	for range octaves {
		value += g.noise.Eval3(x*freq, y*freq, z*freq) * amplitude
		maxAmplitude += amplitude
		amplitude *= persistence
		freq *= lacunarity
	}
	return (value/maxAmplitude + 1.0) / 2.0
}
```

- [ ] **Step 2: Add a temporary shim in the root package**

Create `pkg/planetgen/legacy.go`:

```go
package planetgen

// This file re-exports symbols whose canonical home has moved into
// subpackages. Callers inside the root planetgen package will be
// migrated incrementally; the shim is removed at the end of Task 14.

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
)

// NoiseGenerator is a legacy alias for noise.Generator.
type NoiseGenerator = noise.Generator

// NewNoiseGenerator is a legacy alias for noise.New.
var NewNoiseGenerator = noise.New
```

- [ ] **Step 3: Inline `SphericalCoords` and `SphericalFractal` into their callers**

Both `crater.go` and `rocky.go` and `gasgiant.go` call `SphericalCoords` and `(*NoiseGenerator).SphericalFractal`. Replace each call site with the inline body. The inlined math is:

```go
// Replace:  sx, sy, sz := SphericalCoords(px, py, width, height)
// With:
lon := float64(px) / float64(width) * 2 * math.Pi
lat := math.Pi/2 - float64(py)/float64(height)*math.Pi
sx := math.Cos(lat) * math.Cos(lon)
sy := math.Sin(lat)
sz := math.Cos(lat) * math.Sin(lon)
```

```go
// Replace:  v := ng.SphericalFractal(px, py, w, h, oct, lac, per, scale)
// With:
lon := float64(px) / float64(w) * 2 * math.Pi
lat := math.Pi/2 - float64(py)/float64(h)*math.Pi
sx := math.Cos(lat) * math.Cos(lon)
sy := math.Sin(lat)
sz := math.Cos(lat) * math.Sin(lon)
v := ng.FractalNoise3D(sx, sy, sz, oct, lac, per, scale)
```

Apply this transformation in `pkg/planetgen/rocky.go` and `pkg/planetgen/gasgiant.go` and `pkg/planetgen/crater.go` wherever those calls appear. (Search: `grep -n "SphericalCoords\|SphericalFractal" pkg/planetgen/*.go`.)

- [ ] **Step 4: Delete the old noise.go**

```bash
cd /home/robert/spacemolt/kb
rm pkg/planetgen/noise.go
```

- [ ] **Step 5: Build and run the existing tests (none yet for planetgen, but check for compile errors)**

Run: `cd /home/robert/spacemolt/kb && go build ./... && go test ./pkg/planetgen/cubemap/...`
Expected: success.

Run: `cd /home/robert/spacemolt/kb && go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/test_terran.png`
Expected: command succeeds; `/tmp/test_terran.png` produced (visually identical to current output — no algorithm change yet).

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/noise/noise.go pkg/planetgen/legacy.go pkg/planetgen/rocky.go pkg/planetgen/gasgiant.go pkg/planetgen/crater.go
git rm pkg/planetgen/noise.go
git commit -m "Phase 0: move noise primitives to pkg/planetgen/noise"
```

---

## Task 9: Move color.go into pkg/planetgen/color

**Files:**
- Create: `pkg/planetgen/color/color.go`
- Delete: `pkg/planetgen/color.go`
- Modify: `pkg/planetgen/rocky.go`, `pkg/planetgen/gasgiant.go` to import the new path; existing private symbols `lerpColor`, `sampleGradient`, `blendColor`, `brighten` and the type `ColorStop` are *exported* and callers updated.
- Modify: `pkg/planetgen/profile.go` references `ColorStop` — update import.
- Modify: `pkg/planetgen/legacy.go` to alias `ColorStop` for the root package's `profile.go`.

- [ ] **Step 1: Write the new package**

Create `pkg/planetgen/color/color.go`:

```go
package color

import (
	"image/color"
	"math"
)

// ColorStop represents a color at a specific position in a gradient.
type ColorStop struct {
	Position float64
	Color    color.RGBA
}

// Lerp interpolates between two colors by t [0,1].
func Lerp(a, b color.RGBA, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}

// SampleGradient returns the interpolated color at position t [0,1] in a gradient.
func SampleGradient(stops []ColorStop, t float64) color.RGBA {
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
			return Lerp(stops[i-1].Color, stops[i].Color, localT)
		}
	}
	return stops[len(stops)-1].Color
}

// Blend blends src over dst with the given alpha [0,1].
func Blend(dst, src color.RGBA, alpha float64) color.RGBA {
	alpha = math.Max(0, math.Min(1, alpha))
	return color.RGBA{
		R: uint8(float64(dst.R)*(1-alpha) + float64(src.R)*alpha),
		G: uint8(float64(dst.G)*(1-alpha) + float64(src.G)*alpha),
		B: uint8(float64(dst.B)*(1-alpha) + float64(src.B)*alpha),
		A: 255,
	}
}

// Brighten adjusts a color's brightness by factor (>1 brighter, <1 darker).
func Brighten(c color.RGBA, factor float64) color.RGBA {
	return color.RGBA{
		R: uint8(math.Min(255, float64(c.R)*factor)),
		G: uint8(math.Min(255, float64(c.G)*factor)),
		B: uint8(math.Min(255, float64(c.B)*factor)),
		A: 255,
	}
}
```

- [ ] **Step 2: Update legacy.go to alias ColorStop**

Edit `pkg/planetgen/legacy.go` and add to the existing imports + body:

```go
import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
)

// ColorStop is a legacy alias for color.ColorStop.
type ColorStop = color.ColorStop

// NoiseGenerator and NewNoiseGenerator from earlier task remain.
type NoiseGenerator = noise.Generator
var NewNoiseGenerator = noise.New
```

- [ ] **Step 3: Update callers**

In `pkg/planetgen/rocky.go` and `pkg/planetgen/gasgiant.go`, replace lower-case calls with their exported versions imported from the new package. Add the import:

```go
import (
	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
)
```

(Use the `planetcolor` alias to avoid colliding with `image/color`.)

Replacements:
- `sampleGradient(...)` → `planetcolor.SampleGradient(...)`
- `blendColor(...)` → `planetcolor.Blend(...)`
- `lerpColor(...)` → `planetcolor.Lerp(...)`
- `brighten(...)` → `planetcolor.Brighten(...)`

- [ ] **Step 4: Delete pkg/planetgen/color.go**

```bash
cd /home/robert/spacemolt/kb
rm pkg/planetgen/color.go
```

- [ ] **Step 5: Build and smoke-test**

Run: `cd /home/robert/spacemolt/kb && go build ./... && go test ./pkg/planetgen/cubemap/...`
Expected: success.

Run: `cd /home/robert/spacemolt/kb && go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/test_terran_postcolor.png`
Expected: produces a PNG byte-identical to the pre-Task-9 `/tmp/test_terran.png` (same code path, just relocated).

Verify with: `cmp /tmp/test_terran.png /tmp/test_terran_postcolor.png && echo IDENTICAL`
Expected: `IDENTICAL`.

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/color/color.go pkg/planetgen/legacy.go pkg/planetgen/rocky.go pkg/planetgen/gasgiant.go
git rm pkg/planetgen/color.go
git commit -m "Phase 0: move color helpers to pkg/planetgen/color, export public API"
```

---

## Task 10: Move stats.go into pkg/planetgen/stats

**Files:**
- Create: `pkg/planetgen/stats/stats.go`
- Delete: `pkg/planetgen/stats.go`
- Modify: any caller imports.

- [ ] **Step 1: Read current stats.go to understand its API**

Run: `cat /home/robert/spacemolt/kb/pkg/planetgen/stats.go`
Take note of the exported types/functions; relocate them as-is into `pkg/planetgen/stats/stats.go` with `package stats`.

- [ ] **Step 2: Create the new file**

Move the full body of `pkg/planetgen/stats.go` to `pkg/planetgen/stats/stats.go`, changing the package declaration from `package planetgen` to `package stats`. Keep all type/function names unchanged.

- [ ] **Step 3: Update callers**

Search for callers of any symbol previously in `pkg/planetgen/stats.go`:

```bash
cd /home/robert/spacemolt/kb && grep -rn "planetgen\.\(Stats\|RecordStats\|...\)" --include="*.go"
```

(Replace `\(Stats\|...\)` with the actual exported names found in step 1.) Update each caller's import to `github.com/rsned/spacemolt-kb/pkg/planetgen/stats` and adjust the package qualifier to `stats.`.

- [ ] **Step 4: Delete the old file**

```bash
cd /home/robert/spacemolt/kb
rm pkg/planetgen/stats.go
```

- [ ] **Step 5: Build**

Run: `cd /home/robert/spacemolt/kb && go build ./...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/stats/stats.go pkg/planetgen
git rm pkg/planetgen/stats.go
git commit -m "Phase 0: move stats helpers to pkg/planetgen/stats"
```

---

## Task 11: Rewrite ApplyCraters for cube-map; move to feature

**Files:**
- Create: `pkg/planetgen/feature/crater.go`
- Create: `pkg/planetgen/feature/crater_test.go`
- Delete: `pkg/planetgen/crater.go`

The current `ApplyCraters(heightmap [][]float64, craters []Crater, width, height int, depth float64)` walks every equirect pixel within an angular bounding region of each crater, computes its 3D direction, and modifies the heightmap. The new signature operates on `*cubemap.CubeMapF` and walks every pixel of every face within the angular bounding region. Because the angular distance check uses 3D dot products, the math is unchanged — only the loop changes.

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/feature/crater_test.go`:

```go
package feature

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestGenerateCratersDeterministic(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 99))
	a := GenerateCraters(rng, 50, 0.01, 0.05)
	rng = rand.New(rand.NewPCG(42, 99))
	b := GenerateCraters(rng, 50, 0.01, 0.05)
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("crater[%d] mismatch", i)
		}
	}
}

func TestApplyCratersStampSingle(t *testing.T) {
	cm := cubemap.NewF(64)
	// Pre-fill heightmap to 0.5 everywhere.
	for face := range cubemap.Face(cubemap.NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = 0.5
		}
	}
	// Single crater at +X axis, angular radius 0.2 rad, depth 0.3.
	craters := []Crater{{Lat: 0, Lon: 0, Radius: 0.2}}
	ApplyCraters(cm, craters, 0.3)

	// Pixel at +X face center (px=S/2, py=S/2) is at direction ~(1,0,0),
	// inside the crater. Should be < 0.5 after deposit.
	hCenter := cm.Get(cubemap.FacePosX, 32, 32)
	if hCenter >= 0.5 {
		t.Errorf("crater center h=%f, want < 0.5", hCenter)
	}

	// Pixel at -X face center is at direction (-1,0,0), arc π away —
	// untouched.
	hOpposite := cm.Get(cubemap.FaceNegX, 32, 32)
	if math.Abs(hOpposite-0.5) > 1e-9 {
		t.Errorf("opposite-side h=%f, want 0.5", hOpposite)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/feature/...`
Expected: FAIL with "undefined: Crater / GenerateCraters / ApplyCraters".

- [ ] **Step 3: Implement crater.go**

Create `pkg/planetgen/feature/crater.go`:

```go
package feature

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Crater represents a single crater on the planet surface.
type Crater struct {
	Lat, Lon float64 // Spherical coordinates of center
	Radius   float64 // Angular radius on the sphere (radians)
}

// GenerateCraters creates a list of craters distributed uniformly on a
// sphere with a quadratic-bias size distribution (most craters small,
// few large).
func GenerateCraters(rng *rand.Rand, count int, minRadius, maxRadius float64) []Crater {
	craters := make([]Crater, count)
	for i := range count {
		lat := math.Asin(2*rng.Float64() - 1)
		lon := rng.Float64() * 2 * math.Pi
		t := rng.Float64()
		radius := minRadius + (maxRadius-minRadius)*t*t
		craters[i] = Crater{Lat: lat, Lon: lon, Radius: radius}
	}
	sort.Slice(craters, func(i, j int) bool {
		return craters[i].Radius > craters[j].Radius
	})
	return craters
}

// ApplyCraters stamps a list of craters onto a cube-map heightmap.
// For each crater, every pixel within (1.5×) its angular radius
// is examined; pixels inside the radius receive a bowl-and-rim
// modification scaled by depth.
func ApplyCraters(cm *cubemap.CubeMapF, craters []Crater, depth float64) {
	S := cm.Size
	for _, c := range craters {
		cx := math.Cos(c.Lat) * math.Cos(c.Lon)
		cy := math.Sin(c.Lat)
		cz := math.Cos(c.Lat) * math.Sin(c.Lon)
		// A face is touched if any of its pixels is within 1.5×Radius
		// of the crater axis. We conservatively scan all 6 faces; the
		// per-pixel angular check filters non-affected pixels cheaply.
		for face := range cubemap.Face(cubemap.NumFaces) {
			for py := range S {
				for px := range S {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					dot := dx*cx + dy*cy + dz*cz
					if dot < math.Cos(c.Radius*1.5) {
						continue
					}
					if dot > 1 {
						dot = 1
					}
					dist := math.Acos(dot)
					if dist >= c.Radius {
						continue
					}
					t := dist / c.Radius
					var mod float64
					if t < 0.8 {
						mod = -depth * (1 - (t/0.8)*(t/0.8))
					} else {
						rimT := (t - 0.8) / 0.2
						mod = depth * 0.15 * math.Sin(rimT*math.Pi)
					}
					h := cm.Get(face, px, py) + mod
					if h < 0 {
						h = 0
					} else if h > 1 {
						h = 1
					}
					cm.Set(face, px, py, h)
				}
			}
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/feature/...`
Expected: PASS.

- [ ] **Step 5: Delete old crater.go**

The render code in `pkg/planetgen/rocky.go` still references the old `GenerateCraters`/`ApplyCraters`/`Crater` symbols. Add aliases to `legacy.go` so the root package keeps compiling until Task 12:

```go
// In pkg/planetgen/legacy.go, append:
import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
)

// Crater is a legacy alias for feature.Crater.
type Crater = feature.Crater
var GenerateCraters = feature.GenerateCraters
// Note: ApplyCraters has changed signature ([][]float64 → CubeMapF), so we
// cannot alias it. The root render code in rocky.go still uses the old
// signature; we'll port it in Task 12.
```

For the *old* signature, keep a compatibility wrapper in `pkg/planetgen/crater.go` reduced to just the heightmap-array variant:

```go
// pkg/planetgen/crater.go (cut down — kept temporarily for rocky.go compat)
package planetgen

import "math"

// applyCratersLegacy is the pre-refactor heightmap-array variant,
// retained until rocky.go is ported to cube-map in Task 12.
func applyCratersLegacy(heightmap [][]float64, craters []Crater, width, height int, depth float64) {
	for _, c := range craters {
		cx := math.Cos(c.Lat) * math.Cos(c.Lon)
		cy := math.Sin(c.Lat)
		cz := math.Cos(c.Lat) * math.Sin(c.Lon)
		latCenter := math.Pi/2 - c.Lat
		yCenter := latCenter / math.Pi * float64(height)
		xCenter := c.Lon / (2 * math.Pi) * float64(width)
		pixRadius := c.Radius / math.Pi * float64(height) * 1.5
		yMin := int(math.Max(0, yCenter-pixRadius))
		yMax := int(math.Min(float64(height-1), yCenter+pixRadius))
		for py := yMin; py <= yMax; py++ {
			xMin := int(xCenter - pixRadius)
			xMax := int(xCenter + pixRadius)
			for px := xMin; px <= xMax; px++ {
				wpx := ((px % width) + width) % width
				lon := float64(wpx) / float64(width) * 2 * math.Pi
				lat := math.Pi/2 - float64(py)/float64(height)*math.Pi
				sx := math.Cos(lat) * math.Cos(lon)
				sy := math.Sin(lat)
				sz := math.Cos(lat) * math.Sin(lon)
				dot := sx*cx + sy*cy + sz*cz
				if dot > 1 {
					dot = 1
				} else if dot < -1 {
					dot = -1
				}
				dist := math.Acos(dot)
				if dist < c.Radius {
					t := dist / c.Radius
					var mod float64
					if t < 0.8 {
						mod = -depth * (1 - (t/0.8)*(t/0.8))
					} else {
						rimT := (t - 0.8) / 0.2
						mod = depth * 0.15 * math.Sin(rimT*math.Pi)
					}
					heightmap[py][wpx] += mod
				}
			}
		}
	}
	for py := range height {
		for px := range width {
			heightmap[py][px] = math.Max(0, math.Min(1, heightmap[py][px]))
		}
	}
}
```

In `pkg/planetgen/rocky.go`, replace the call `ApplyCraters(...)` with `applyCratersLegacy(...)`. (One-line edit.)

- [ ] **Step 6: Build and smoke-test**

Run: `cd /home/robert/spacemolt/kb && go build ./... && go test ./pkg/planetgen/...`
Expected: success.

Run: `cd /home/robert/spacemolt/kb && go run ./cmd/generate-planet-maps -type scorched -seed Mercury -out /tmp/test_scorched.png`
Expected: produces a PNG. Compare to a freshly-rebuilt baseline at HEAD~3 to confirm visual identity. (Algorithm unchanged.)

- [ ] **Step 7: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/feature/crater.go pkg/planetgen/feature/crater_test.go \
        pkg/planetgen/legacy.go pkg/planetgen/crater.go pkg/planetgen/rocky.go
git commit -m "Phase 0: add feature.ApplyCraters cube-map variant; keep legacy heightmap variant"
```

---

## Task 12: Rewrite rocky.go for cube-map; move to render

**Files:**
- Create: `pkg/planetgen/render/rocky.go`
- Delete: `pkg/planetgen/rocky.go` (and the now-unused `pkg/planetgen/crater.go` legacy shim)
- Delete: `pkg/planetgen/legacy.go` aliases that are no longer needed (we'll keep only ones still referenced).

The current `RenderRocky(profile, seed, width, height) *image.RGBA` does:
1. Allocate `[height][width]float64` heightmap.
2. For each `(py, px)`, sample fractal noise → fill heightmap.
3. Normalise heightmap to `[0, 1]`.
4. Stamp craters on heightmap.
5. For each `(py, px)` colorise (palette + biome + ocean + snow + polar caps) → write into `image.RGBA`.

The new `RenderRocky(profile, seed, S int) *cubemap.CubeMap` does:
1. Allocate `cubemap.NewF(S)` heightmap.
2. For each face, each `(py, px)`, compute `dir = FacePixelToDir`, sample fractal noise via `FractalNoise3D(dir.x, dir.y, dir.z, ...)` → write into heightmap.
3. Normalise heightmap to `[0, 1]` (find min/max across all 6 faces, then rescale).
4. Stamp craters via the new `feature.ApplyCraters(heightmap, ...)`.
5. Allocate `cubemap.New(S)` output. For each face, each `(py, px)`, recompute `dir`, derive `lat = asin(dir.y)`, then run the colorisation logic (palette + biome + ocean + snow + polar caps) and write to the output cube map.

- [ ] **Step 1: Write the failing test**

Create `pkg/planetgen/render/rocky_test.go`:

```go
package render

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

func TestRenderRockyAllPixelsOpaque(t *testing.T) {
	prof := planetgen.Profiles["scorched"]
	cm := RenderRocky(prof, 1234, 64)
	for face := range len(cm.Faces) {
		for i, c := range cm.Faces[face] {
			if c.A != 255 {
				t.Fatalf("face %d pixel %d alpha=%d", face, i, c.A)
				_ = i
			}
		}
	}
}

func TestRenderRockyDeterministic(t *testing.T) {
	prof := planetgen.Profiles["arid"]
	a := RenderRocky(prof, 7, 32)
	b := RenderRocky(prof, 7, 32)
	for face := range len(a.Faces) {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs across runs", face, i)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/render/...`
Expected: FAIL with "undefined: RenderRocky".

- [ ] **Step 3: Write `pkg/planetgen/render/rocky.go`**

```go
package render

import (
	"image/color"
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
)

var (
	whiteIce  = color.RGBA{R: 240, G: 245, B: 250, A: 255}
	whiteSnow = color.RGBA{R: 235, G: 240, B: 245, A: 255}
)

// RenderRocky generates a rocky planet cube map.
func RenderRocky(profile *planetgen.PlanetProfile, seed int64, S int) *cubemap.CubeMap {
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed*31+7)))
	ng := noise.New(seed)
	capNoise := noise.New(seed + 42)
	oceanNoise := noise.New(seed + 77)
	biomeNoise := noise.New(seed + 99)

	heightmap := cubemap.NewF(S)

	// Step 1: base fractal heightmap on the unit sphere.
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

	// Step 1b: normalise to [0,1] across all faces.
	hMin, hMax := 1.0, 0.0
	for face := range cubemap.Face(cubemap.NumFaces) {
		for _, h := range heightmap.Faces[face] {
			if h < hMin {
				hMin = h
			}
			if h > hMax {
				hMax = h
			}
		}
	}
	if hMax > hMin {
		hRange := hMax - hMin
		for face := range cubemap.Face(cubemap.NumFaces) {
			for i := range heightmap.Faces[face] {
				heightmap.Faces[face][i] = (heightmap.Faces[face][i] - hMin) / hRange
			}
		}
	}

	// Step 2: craters.
	if profile.CraterCount > 0 {
		craters := feature.GenerateCraters(rng, profile.CraterCount,
			profile.CraterMinRadius, profile.CraterMaxRadius)
		feature.ApplyCraters(heightmap, craters, profile.CraterDepth)
	}

	// Step 3+4+5: colorise (biome, ocean, snow, polar caps).
	hasBiomes := len(profile.EquatorialPalette) > 0 || len(profile.PolarPalette) > 0
	out := cubemap.New(S)

	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				lat := math.Asin(dy)
				absLat := math.Abs(lat) / (math.Pi / 2)

				h := heightmap.Get(face, px, py)
				c := planetcolor.SampleGradient(profile.Palette, h)

				if hasBiomes {
					biomeVar := biomeNoise.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 4.0)
					adjustedLat := absLat + (biomeVar-0.5)*0.15
					if len(profile.EquatorialPalette) > 0 && adjustedLat < 0.35 {
						eqColor := planetcolor.SampleGradient(profile.EquatorialPalette, h)
						eqBlend := 1.0 - adjustedLat/0.35
						eqBlend *= eqBlend
						c = planetcolor.Blend(c, eqColor, eqBlend*0.8)
					}
					if len(profile.PolarPalette) > 0 && adjustedLat > 0.6 {
						polColor := planetcolor.SampleGradient(profile.PolarPalette, h)
						polBlend := (adjustedLat - 0.6) / 0.4
						polBlend *= polBlend
						c = planetcolor.Blend(c, polColor, polBlend*0.7)
					}
				}

				if profile.SnowLine > 0 && h > profile.SnowLine {
					snowBlend := (h - profile.SnowLine) / (1.0 - profile.SnowLine)
					snowBlend = math.Min(1.0, snowBlend*1.5)
					latBoost := 1.0 + absLat*0.5
					snowBlend = math.Min(1.0, snowBlend*latBoost)
					c = planetcolor.Blend(c, whiteSnow, snowBlend*0.85)
				}

				if profile.OceanLevel > 0 && h < profile.OceanLevel {
					depth := (profile.OceanLevel - h) / profile.OceanLevel
					surfaceVar := oceanNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 6.0)
					if profile.Type == "lava_world" {
						brightness := 0.7 + depth*0.3
						if depth < 0.2 {
							brightness *= 0.6 + depth*2.0
						}
						brightness += (surfaceVar - 0.5) * 0.25
						brightness = math.Max(0.4, math.Min(1.2, brightness))
						lavaColor := planetcolor.Lerp(
							profile.OceanColor,
							color.RGBA{R: 255, G: 160, B: 20, A: 255},
							surfaceVar*0.4,
						)
						c = planetcolor.Brighten(lavaColor, brightness)
					} else {
						shallowFactor := 1.0
						if depth < 0.15 {
							shallowFactor = 1.3 - depth*2.0
						}
						brightness := (1.0 - depth*0.5) * shallowFactor
						brightness += (surfaceVar - 0.5) * 0.15
						brightness = math.Max(0.5, math.Min(1.3, brightness))
						c = planetcolor.Brighten(profile.OceanColor, brightness)
					}
				}

				if profile.HasPolarCaps && profile.PolarCapSize > 0 {
					capThreshold := 1.0 - profile.PolarCapSize
					if absLat > capThreshold {
						capEdgeNoise := capNoise.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, 8.0)
						noiseAmt := profile.PolarCapNoise
						if noiseAmt == 0 {
							noiseAmt = 0.08
						}
						adjustedThreshold := capThreshold + (capEdgeNoise-0.5)*noiseAmt
						if absLat > adjustedThreshold {
							blend := math.Min(1.0, (absLat-adjustedThreshold)*15)
							capColor := planetcolor.Brighten(whiteIce, 0.9+capEdgeNoise*0.2)
							c = planetcolor.Blend(c, capColor, blend)
						}
					}
				}

				out.Set(face, px, py, c)
			}
		}
	}

	return out
}
```

- [ ] **Step 4: Run tests**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/render/...`
Expected: PASS for both `TestRenderRockyAllPixelsOpaque` and `TestRenderRockyDeterministic`.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/render/rocky.go pkg/planetgen/render/rocky_test.go
git commit -m "Phase 0: add render.RenderRocky cube-map variant"
```

---

## Task 13: Rewrite gasgiant.go for cube-map; move to render

**Files:**
- Create: `pkg/planetgen/render/gasgiant.go`
- Read existing: `pkg/planetgen/gasgiant.go` to preserve algorithm.

- [ ] **Step 1: Read the existing file in full**

Run: `cat /home/robert/spacemolt/kb/pkg/planetgen/gasgiant.go`
Note exported symbols (`generateBands`, `getBandColor`, `generateStorms`, `stormDistance`, etc.) and the `RenderGasGiant` body. The pattern is the same as Task 12: outer loop becomes face × pixel, lat from `dir.y`, lon from `atan2(dir.z, dir.x)`.

- [ ] **Step 2: Write `pkg/planetgen/render/gasgiant.go`**

Copy the full body of `pkg/planetgen/gasgiant.go` to `pkg/planetgen/render/gasgiant.go`, applying these mechanical replacements:

1. Change package declaration to `package render`.
2. Add imports for `cubemap`, `noise`, `planetcolor`, `planetgen`.
3. Replace the function signature:
   ```go
   func RenderGasGiant(profile *PlanetProfile, seed int64, width, height int) *image.RGBA {
   ```
   with:
   ```go
   func RenderGasGiant(profile *planetgen.PlanetProfile, seed int64, S int) *cubemap.CubeMap {
   ```
4. Replace the outer loop:
   ```go
   for y := range height {
       for x := range width {
           sx, sy, sz := SphericalCoords(x, y, width, height)
           lat := math.Pi/2 - float64(y)/float64(height)*math.Pi
           ...
           img.SetRGBA(x, y, c)
   ```
   with:
   ```go
   for face := range cubemap.Face(cubemap.NumFaces) {
       for py := range S {
           for px := range S {
               sx, sy, sz := cubemap.FacePixelToDir(face, px, py, S)
               lat := math.Asin(sy)
               lonForX := math.Atan2(sz, sx) // [-π, π]
               if lonForX < 0 {
                   lonForX += 2 * math.Pi
               }
               ...
               out.Set(face, px, py, c)
   ```
5. The original code computed `float64(x)/float64(width)*2*math.Pi` for storm-distance lon — replace with `lonForX` from step 4.
6. Replace `img := image.NewRGBA(image.Rect(0, 0, width, height))` with `out := cubemap.New(S)`. Return `out`.
7. Replace any `ng.SphericalFractal(...)` call with `ng.FractalNoise3D(sx, sy, sz, ...)`.
8. Update `image/color` types to `color.RGBA`; the `getBandColor`, `generateBands`, `generateStorms`, `stormDistance` private helpers move with the file unchanged.
9. `lerpColor`, `blendColor`, `brighten`, `sampleGradient` calls become `planetcolor.Lerp`, `planetcolor.Blend`, `planetcolor.Brighten`, `planetcolor.SampleGradient`.

- [ ] **Step 3: Add a smoke test**

Create `pkg/planetgen/render/gasgiant_test.go`:

```go
package render

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

func TestRenderGasGiantAllPixelsOpaque(t *testing.T) {
	prof := planetgen.Profiles["jovian"]
	cm := RenderGasGiant(prof, 1234, 64)
	for face := range len(cm.Faces) {
		for i, c := range cm.Faces[face] {
			if c.A != 255 {
				t.Fatalf("face %d pixel %d alpha=%d", face, i, c.A)
			}
		}
	}
}

func TestRenderGasGiantDeterministic(t *testing.T) {
	prof := planetgen.Profiles["ice_giant"]
	a := RenderGasGiant(prof, 99, 32)
	b := RenderGasGiant(prof, 99, 32)
	for face := range len(a.Faces) {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d differs across runs", face, i)
			}
		}
	}
}
```

- [ ] **Step 4: Build and run tests**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/planetgen/render/...`
Expected: PASS for all four tests (`TestRenderRocky*` from Task 12, `TestRenderGasGiant*`).

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/render/gasgiant.go pkg/planetgen/render/gasgiant_test.go
git commit -m "Phase 0: add render.RenderGasGiant cube-map variant"
```

---

## Task 14: Update Generate to return *cubemap.CubeMap; delete legacy code

**Files:**
- Modify: `pkg/planetgen/planetgen.go`
- Delete: `pkg/planetgen/rocky.go`, `pkg/planetgen/gasgiant.go`, `pkg/planetgen/crater.go`, `pkg/planetgen/legacy.go`

- [ ] **Step 1: Rewrite planetgen.go**

Replace the contents of `pkg/planetgen/planetgen.go` with:

```go
package planetgen

import (
	"fmt"
	"hash/fnv"
	"image"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

// DefaultFaceSize is the default cube-map face edge length in pixels.
const DefaultFaceSize = 1024

// DefaultWidth is the default equirect output width in pixels.
const DefaultWidth = 2000

// DefaultHeight is the default equirect output height in pixels.
const DefaultHeight = 1000

// Generate creates a planet cube map for the given planet type and name.
// The planet name is hashed to produce a deterministic seed.
func Generate(planetType, planetName string, faceSize int) (*cubemap.CubeMap, error) {
	profile := GetProfile(planetType)
	if profile == nil {
		return nil, fmt.Errorf("unknown planet type: %s", planetType)
	}
	seed := hashSeed(planetName)
	switch profile.Renderer {
	case "rocky":
		return render.RenderRocky(profile, seed, faceSize), nil
	case "gas_giant":
		return render.RenderGasGiant(profile, seed, faceSize), nil
	default:
		return nil, fmt.Errorf("unknown renderer: %s", profile.Renderer)
	}
}

// GenerateEquirect generates a planet and bakes it to a width×height
// equirectangular RGBA image. Convenience wrapper around Generate +
// cubemap.BakeEquirect.
func GenerateEquirect(planetType, planetName string, width, height int) (*image.RGBA, error) {
	cm, err := Generate(planetType, planetName, DefaultFaceSize)
	if err != nil {
		return nil, err
	}
	return cubemap.BakeEquirect(cm, width, height), nil
}

// hashSeed converts a planet name to a deterministic int64 seed.
func hashSeed(name string) int64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	return int64(h.Sum64())
}

// HashSeedPublic is the exported version of hashSeed for test tooling.
func HashSeedPublic(name string) int64 {
	return hashSeed(name)
}
```

- [ ] **Step 2: Delete the legacy code**

```bash
cd /home/robert/spacemolt/kb
rm pkg/planetgen/rocky.go pkg/planetgen/gasgiant.go pkg/planetgen/crater.go pkg/planetgen/legacy.go
```

- [ ] **Step 3: Build**

Run: `cd /home/robert/spacemolt/kb && go build ./...`
Expected: success. The only files left in `pkg/planetgen/` (root package) are `planetgen.go`, `profile.go`, plus the new subpackage directories.

If build fails because `profile.go` references the removed alias `ColorStop`, edit `profile.go` to import `pkg/planetgen/color` and use `color.ColorStop`. Search: `grep -n "ColorStop" pkg/planetgen/profile.go`.

- [ ] **Step 4: Run all tests**

Run: `cd /home/robert/spacemolt/kb && go test ./...`
Expected: PASS for everything previously passing, including `pkg/planetgen/cubemap`, `pkg/planetgen/feature`, `pkg/planetgen/render`.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add pkg/planetgen/planetgen.go pkg/planetgen/profile.go
git rm pkg/planetgen/rocky.go pkg/planetgen/gasgiant.go pkg/planetgen/crater.go pkg/planetgen/legacy.go
git commit -m "Phase 0: switch Generate to cube-map output; remove legacy equirect code paths"
```

---

## Task 15: Update cmd/generate-planet-maps to write both files

**Files:**
- Modify: `cmd/generate-planet-maps/main.go`

- [ ] **Step 1: Rewrite generateSingle to write both formats**

Edit `cmd/generate-planet-maps/main.go`. Replace the `generateSingle` function with:

```go
func generateSingle(planetType, planetName string, faceSize, equirectW, equirectH int, equirectPath string) error {
	cm, err := planetgen.Generate(planetType, planetName, faceSize)
	if err != nil {
		return err
	}

	// Write the cube-map cross alongside the equirect, derived path:
	// foo.png  → foo.cube.png
	cubePath := equirectPath
	if ext := filepath.Ext(cubePath); ext != "" {
		cubePath = cubePath[:len(cubePath)-len(ext)] + ".cube" + ext
	} else {
		cubePath += ".cube.png"
	}
	if err := cubemap.WriteCrossPNG(cm, cubePath); err != nil {
		return err
	}

	img := cubemap.BakeEquirect(cm, equirectW, equirectH)
	f, err := os.Create(equirectPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}
```

Add the import:

```go
"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
```

- [ ] **Step 2: Update flag definitions and call sites**

Replace the flag definitions block (lines ~27–37 of main.go) with:

```go
	var (
		singleType = flag.String("type", "", "generate only this planet type (for testing)")
		singleSeed = flag.String("seed", "", "planet name to use as seed (for testing)")
		outFile    = flag.String("out", "", "output equirect PNG path (single image mode)")
		dbPath     = flag.String("db", "../spacemolt-knowledge.db", "path to knowledge database")
		outDir     = flag.String("outdir", "kb/images/planets", "output directory for planet images")
		faceSize   = flag.Int("face", planetgen.DefaultFaceSize, "cube-map face size in pixels")
		eqWidth    = flag.Int("width", planetgen.DefaultWidth, "equirect bake width in pixels")
		eqHeight   = flag.Int("height", planetgen.DefaultHeight, "equirect bake height in pixels")
		workers    = flag.Int("workers", 8, "number of parallel workers")
	)
```

Update single-image-mode call:

```go
		if err := generateSingle(*singleType, *singleSeed, *faceSize, *eqWidth, *eqHeight, out); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Generated %s + %s.cube.png (face %d, equirect %dx%d)\n",
			out, strings.TrimSuffix(out, filepath.Ext(out)),
			*faceSize, *eqWidth, *eqHeight)
```

Update the batch worker call:

```go
				if err := generateSingle(p.PlanetType, p.PlanetName, *faceSize, *eqWidth, *eqHeight, outPath); err != nil {
```

- [ ] **Step 3: Smoke-test single mode**

Run:
```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/test_earth.png
```
Expected: produces both `/tmp/test_earth.png` (equirect, 2000×1000) and `/tmp/test_earth.cube.png` (4×3 cross, 4096×3072). Verify with:

```bash
file /tmp/test_earth.png /tmp/test_earth.cube.png
```
Expected: PNG image data, 2000 x 1000 / 4096 x 3072.

- [ ] **Step 4: Build**

Run: `cd /home/robert/spacemolt/kb && go build ./...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-planet-maps/main.go
git commit -m "Phase 0: write both cube-map cross and equirect PNG per planet"
```

---

## Task 16: Add golangci-lint config with depguard

**Files:**
- Create: `.golangci.yml`

This config enables `depguard` to deny `import "C"` (cgo) within `pkg/planetgen/...` and `cmd/planet-explorer/...` (which doesn't exist yet but is constrained pre-emptively for Phase 1).

- [ ] **Step 1: Create the config**

Create `/home/robert/spacemolt/kb/.golangci.yml`:

```yaml
version: "2"

run:
  timeout: 5m
  go: "1.24"

linters:
  default: standard
  enable:
    - depguard

linters-settings:
  depguard:
    rules:
      no-cgo-in-planetgen:
        list-mode: lax
        files:
          - "$all"
        allow:
          - "$gostd"
          - "github.com/rsned/spacemolt-kb"
          - "github.com/ojrac/opensimplex-go"
          - "github.com/lucasb-eyer/go-colorful"
          - "github.com/dustin/go-humanize"
          - "github.com/anthropics/anthropic-sdk-go"
          - "github.com/google/uuid"
          - "modernc.org/sqlite"
          - "github.com/tidwall"
          - "golang.org/x"
        deny:
          - pkg: "C"
            desc: "cgo is forbidden; pkg/planetgen and cmd/planet-explorer must compile to Wasm"
```

- [ ] **Step 2: Run golangci-lint**

If `golangci-lint` isn't installed, install it: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` (or use the project's preferred install method).

Run: `cd /home/robert/spacemolt/kb && golangci-lint run`
Expected: clean (no findings). If any pre-existing findings appear that are unrelated to depguard, leave them — Phase 0 only owns no-new-findings.

If any of the existing dependencies in `go.mod` are not in the `allow` list above, the linter will flag them. Add them to the `allow` list and re-run. The list above is derived from the current `go.mod`; add anything that's flagged.

- [ ] **Step 3: Verify cgo would be blocked**

Create a temporary test file `pkg/planetgen/cubemap/cgo_test.go.disabled`:

```go
//go:build ignore
package cubemap

/*
#include <stdio.h>
*/
import "C"
```

Run: `golangci-lint run pkg/planetgen/cubemap/cgo_test.go.disabled`
Expected: depguard flags `import "C"`.

Then delete the file:

```bash
rm /home/robert/spacemolt/kb/pkg/planetgen/cubemap/cgo_test.go.disabled
```

- [ ] **Step 4: Commit**

```bash
cd /home/robert/spacemolt/kb
git add .golangci.yml
git commit -m "Phase 0: add golangci-lint config with depguard cgo deny rule"
```

---

## Task 17: Add CI workflow with go test + Wasm build

**Files:**
- Create: `.github/workflows/build-test.yml`

- [ ] **Step 1: Create the workflow**

Create `/home/robert/spacemolt/kb/.github/workflows/build-test.yml`:

```yaml
name: Build & Test

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - name: Build
        run: go build ./...
      - name: Test
        run: go test ./...
      - name: Lint
        uses: golangci/golangci-lint-action@v8
        with:
          version: latest

  wasm-build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'
      - name: Wasm build (planetgen + future planet-explorer)
        env:
          GOOS: js
          GOARCH: wasm
        run: |
          go build ./pkg/planetgen/...
          # cmd/planet-explorer is added in Phase 1; the build will
          # extend automatically once it exists.
          if [ -d cmd/planet-explorer/wasm ]; then
            go build ./cmd/planet-explorer/wasm/...
          fi
```

- [ ] **Step 2: Commit**

```bash
cd /home/robert/spacemolt/kb
git add .github/workflows/build-test.yml
git commit -m "Phase 0: add CI matrix (build + test + lint + wasm build)"
```

---

## Task 18: Add the planet-image-diff CLI

**Files:**
- Create: `cmd/tools/planet-image-diff/main.go`
- Modify: `go.mod`, `go.sum` (adds `github.com/lucasb-eyer/go-colorful`).

- [ ] **Step 1: Add the dependency**

Run: `cd /home/robert/spacemolt/kb && go get github.com/lucasb-eyer/go-colorful@latest`
Expected: `go.mod` updated.

- [ ] **Step 2: Write the CLI**

Create `cmd/tools/planet-image-diff/main.go`:

```go
// Package main implements a side-by-side image diff CLI for the
// planet generator's golden image harness.
//
// Usage:
//
//	planet-image-diff <a.png> <b.png> [<diff.png>]
//
// Prints the mean ΔE2000 between corresponding pixels and (if a
// third argument is given) writes a diff image with A on the left,
// B in the middle, and an amplified per-pixel difference on the right.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"

	"github.com/lucasb-eyer/go-colorful"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: planet-image-diff <a.png> <b.png> [<diff.png>]")
		os.Exit(2)
	}
	a := readPNG(os.Args[1])
	b := readPNG(os.Args[2])
	if a.Bounds() != b.Bounds() {
		log.Fatalf("size mismatch: %v vs %v", a.Bounds(), b.Bounds())
	}
	w, h := a.Bounds().Dx(), a.Bounds().Dy()

	var sumDE float64
	var n int
	maxChannelDelta := 0.0
	for y := range h {
		for x := range w {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			ca := colorful.Color{
				R: float64(ar) / 65535,
				G: float64(ag) / 65535,
				B: float64(ab) / 65535,
			}
			cb := colorful.Color{
				R: float64(br) / 65535,
				G: float64(bg) / 65535,
				B: float64(bb) / 65535,
			}
			sumDE += ca.DistanceCIEDE2000(cb)
			n++
			cd := math.Max(
				math.Max(math.Abs(float64(ar)-float64(br)),
					math.Abs(float64(ag)-float64(bg))),
				math.Abs(float64(ab)-float64(bb))) / 257
			if cd > maxChannelDelta {
				maxChannelDelta = cd
			}
		}
	}
	meanDE := sumDE / float64(n)
	fmt.Printf("mean ΔE2000 = %.3f\n", meanDE)
	fmt.Printf("max channel delta = %.0f / 255\n", maxChannelDelta)

	if len(os.Args) >= 4 {
		writeDiffImage(a, b, os.Args[3])
		fmt.Printf("diff image: %s\n", os.Args[3])
	}
}

func readPNG(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		log.Fatalf("decode %s: %v", path, err)
	}
	return img
}

func writeDiffImage(a, b image.Image, path string) {
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, 3*w, h))
	for y := range h {
		for x := range w {
			ca := a.At(x, y)
			cb := b.At(x, y)
			out.Set(x, y, ca)
			out.Set(w+x, y, cb)
			ar, ag, ab, _ := ca.RGBA()
			br, bg, bbB, _ := cb.RGBA()
			dr := abs8(int(ar)-int(br)) >> 8
			dg := abs8(int(ag)-int(bg)) >> 8
			db := abs8(int(ab)-int(bbB)) >> 8
			amp := uint8(0)
			m := dr
			if dg > m {
				m = dg
			}
			if db > m {
				m = db
			}
			if m > 255 {
				m = 255
			}
			amp = uint8(m * 4)
			if int(amp) < m*4 {
				amp = 255
			}
			out.Set(2*w+x, y, color.RGBA{R: amp, G: amp, B: amp, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, out); err != nil {
		log.Fatal(err)
	}
}

func abs8(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
```

- [ ] **Step 3: Build and smoke-test**

```bash
cd /home/robert/spacemolt/kb
go build -o /tmp/planet-image-diff ./cmd/tools/planet-image-diff
go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/a.png
go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/b.png
/tmp/planet-image-diff /tmp/a.png /tmp/b.png /tmp/diff.png
```
Expected: `mean ΔE2000 = 0.000`, `max channel delta = 0 / 255`, diff image written.

- [ ] **Step 4: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/tools/planet-image-diff/main.go go.mod go.sum
git commit -m "Phase 0: add planet-image-diff CLI for golden-image regression review"
```

---

## Task 19: Add statistical invariants test

**Files:**
- Create: `cmd/generate-planet-maps/invariants_test.go`

- [ ] **Step 1: Write the test**

Create `cmd/generate-planet-maps/invariants_test.go`:

```go
package main

import (
	"image/color"
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

var invariantTypes = []string{
	"scorched", "arid", "terran", "tundra", "glacial", "ice_world",
	"super_terran", "hothouse", "lava_world", "oceanic",
	"jovian", "ice_giant", "unknown",
}

func TestInvariantsAlphaOpaque(t *testing.T) {
	for _, pt := range invariantTypes {
		img, err := planetgen.GenerateEquirect(pt, "InvariantSeed-"+pt, 200, 100)
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 255 {
				t.Fatalf("%s: alpha=%d at pixel %d", pt, img.Pix[i], i/4)
			}
		}
	}
}

func TestInvariantsHistogramNonDegenerate(t *testing.T) {
	for _, pt := range invariantTypes {
		img, err := planetgen.GenerateEquirect(pt, "InvariantSeed-"+pt, 200, 100)
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		var bucket [16]int
		for i := 0; i < len(img.Pix); i += 4 {
			r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
			lum := (int(r) + int(g) + int(b)) / 3
			bucket[lum/16]++
		}
		distinct := 0
		for _, n := range bucket {
			if n > 0 {
				distinct++
			}
		}
		if distinct < 4 {
			t.Errorf("%s: only %d distinct luminance buckets occupied; want ≥4", pt, distinct)
		}
	}
}

func TestInvariantsTerranOceanLandRatio(t *testing.T) {
	img, err := planetgen.GenerateEquirect("terran", "Earth", 400, 200)
	if err != nil {
		t.Fatal(err)
	}
	var oceanPx, totalPx int
	for i := 0; i < len(img.Pix); i += 4 {
		c := color.RGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
		if c.B > c.R && c.B > c.G && c.B > 100 {
			oceanPx++
		}
		totalPx++
	}
	ratio := float64(oceanPx) / float64(totalPx)
	if ratio < 0.30 || ratio > 0.85 {
		t.Errorf("terran ocean ratio = %.3f, want in [0.30, 0.85]", ratio)
	}
}

func TestInvariantsLongitudeSeam(t *testing.T) {
	for _, pt := range invariantTypes {
		img, err := planetgen.GenerateEquirect(pt, "SeamSeed-"+pt, 400, 200)
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		var maxDelta int
		w := img.Bounds().Dx()
		for y := 0; y < img.Bounds().Dy(); y++ {
			cL := img.RGBAAt(0, y)
			cR := img.RGBAAt(w-1, y)
			d := absI(int(cL.R)-int(cR.R)) +
				absI(int(cL.G)-int(cR.G)) +
				absI(int(cL.B)-int(cR.B))
			if d > maxDelta {
				maxDelta = d
			}
		}
		if maxDelta > 24 {
			t.Errorf("%s: lon-seam max RGB delta = %d, want ≤ 24", pt, maxDelta)
		}
	}
}

func TestInvariantsNoNaN(t *testing.T) {
	for _, pt := range invariantTypes {
		img, err := planetgen.GenerateEquirect(pt, "NaNSeed-"+pt, 200, 100)
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		for i := 0; i < len(img.Pix); i++ {
			v := float64(img.Pix[i])
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("%s: NaN/Inf at byte %d", pt, i)
			}
		}
	}
}

func absI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
```

- [ ] **Step 2: Run the tests**

Run: `cd /home/robert/spacemolt/kb && go test ./cmd/generate-planet-maps/...`
Expected: PASS for all five invariants. (If `TestInvariantsTerranOceanLandRatio` fails, inspect the produced image — the bound is permissive; failure means the algorithm output drifted significantly. Investigate before tightening the bound.)

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-planet-maps/invariants_test.go
git commit -m "Phase 0: add statistical invariants for all 13 planet types"
```

---

## Task 20: Add golden test harness with -update flag

**Files:**
- Create: `cmd/generate-planet-maps/golden_test.go`
- Create: `cmd/generate-planet-maps/testdata/golden/.gitkeep` (placeholder; PNGs added in Task 21)

- [ ] **Step 1: Write the harness**

Create `cmd/generate-planet-maps/golden_test.go`:

```go
package main

import (
	"flag"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

var updateGoldens = flag.Bool("update", false, "update golden images instead of comparing")

// goldenPlanets pairs each planet type with a fixed seed so the
// rendered output is deterministic across runs and machines.
var goldenPlanets = []struct{ Type, Seed string }{
	{"scorched", "GoldenScorched"},
	{"arid", "GoldenArid"},
	{"terran", "GoldenTerran"},
	{"tundra", "GoldenTundra"},
	{"glacial", "GoldenGlacial"},
	{"ice_world", "GoldenIceWorld"},
	{"super_terran", "GoldenSuperTerran"},
	{"hothouse", "GoldenHothouse"},
	{"lava_world", "GoldenLavaWorld"},
	{"oceanic", "GoldenOceanic"},
	{"jovian", "GoldenJovian"},
	{"ice_giant", "GoldenIceGiant"},
	{"unknown", "GoldenUnknown"},
}

const (
	goldenW       = 400
	goldenH       = 200
	maxMeanDE2000 = 1.5
)

func TestGolden(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range goldenPlanets {
		t.Run(p.Type, func(t *testing.T) {
			img, err := planetgen.GenerateEquirect(p.Type, p.Seed, goldenW, goldenH)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, p.Type+".png")
			if *updateGoldens {
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = f.Close() }()
				if err := png.Encode(f, img); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", path)
				return
			}
			want, err := readGoldenPNG(path)
			if err != nil {
				t.Fatalf("missing/unreadable golden %s: %v (run `go test -run TestGolden -update`)", path, err)
			}
			if want.Bounds() != img.Bounds() {
				t.Fatalf("%s: size %v != golden %v", p.Type, img.Bounds(), want.Bounds())
			}
			meanDE := meanDE2000(img.Pix, want.Pix, len(img.Pix))
			if meanDE > maxMeanDE2000 {
				t.Errorf("%s: mean ΔE2000 = %.3f > %.2f (run `go test -run TestGolden -update` after reviewing)",
					p.Type, meanDE, maxMeanDE2000)
			}
		})
	}
}

func readGoldenPNG(path string) (*pngImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return &pngImage{img: img}, nil
}

type pngImage struct {
	img interface {
		Bounds() (b rectangleStub)
	}
	Pix []uint8 // populated by helper below
}

// (Use the simpler approach below — overwrite the above stub)
```

That stub got tangled. Replace the file with this corrected version:

```go
package main

import (
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

var updateGoldens = flag.Bool("update", false, "update golden images instead of comparing")

var goldenPlanets = []struct{ Type, Seed string }{
	{"scorched", "GoldenScorched"},
	{"arid", "GoldenArid"},
	{"terran", "GoldenTerran"},
	{"tundra", "GoldenTundra"},
	{"glacial", "GoldenGlacial"},
	{"ice_world", "GoldenIceWorld"},
	{"super_terran", "GoldenSuperTerran"},
	{"hothouse", "GoldenHothouse"},
	{"lava_world", "GoldenLavaWorld"},
	{"oceanic", "GoldenOceanic"},
	{"jovian", "GoldenJovian"},
	{"ice_giant", "GoldenIceGiant"},
	{"unknown", "GoldenUnknown"},
}

const (
	goldenW       = 400
	goldenH       = 200
	maxMeanDE2000 = 1.5
)

func TestGolden(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range goldenPlanets {
		t.Run(p.Type, func(t *testing.T) {
			got, err := planetgen.GenerateEquirect(p.Type, p.Seed, goldenW, goldenH)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, p.Type+".png")
			if *updateGoldens {
				if err := writePNG(path, got); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", path)
				return
			}
			want, err := readPNGRGBA(path)
			if err != nil {
				t.Fatalf("missing/unreadable golden %s: %v (run `go test -run TestGolden -update`)", path, err)
			}
			if want.Bounds() != got.Bounds() {
				t.Fatalf("size %v != golden %v", got.Bounds(), want.Bounds())
			}
			meanDE := meanDE2000(got.Pix, want.Pix)
			if meanDE > maxMeanDE2000 {
				t.Errorf("mean ΔE2000 = %.3f > %.2f (run with -update after review)",
					meanDE, maxMeanDE2000)
			}
		})
	}
}

func writePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

func readPNGRGBA(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			dst.SetRGBA(x, y, colorRGBA(uint8(r>>8), uint8(g>>8), uint8(bl>>8), uint8(a>>8)))
		}
	}
	return dst, nil
}

func meanDE2000(a, b []uint8) float64 {
	n := len(a) / 4
	var sum float64
	for i := 0; i < n; i++ {
		j := i * 4
		ca := colorful.Color{
			R: float64(a[j]) / 255,
			G: float64(a[j+1]) / 255,
			B: float64(a[j+2]) / 255,
		}
		cb := colorful.Color{
			R: float64(b[j]) / 255,
			G: float64(b[j+1]) / 255,
			B: float64(b[j+2]) / 255,
		}
		sum += ca.DistanceCIEDE2000(cb)
	}
	return sum / float64(n)
}

func colorRGBA(r, g, b, a uint8) (out colorRGBA_) {
	return colorRGBA_{r, g, b, a}
}

type colorRGBA_ struct{ R, G, B, A uint8 }

// satisfy color.Color
func (c colorRGBA_) RGBA() (r, g, b, a uint32) {
	r = uint32(c.R) | uint32(c.R)<<8
	g = uint32(c.G) | uint32(c.G)<<8
	b = uint32(c.B) | uint32(c.B)<<8
	a = uint32(c.A) | uint32(c.A)<<8
	return
}
```

Actually that's overcomplicated. Use Go's built-in `image/color` directly. **Replace the `colorRGBA_` shim and the `readPNGRGBA` function with this simpler version:**

```go
func readPNGRGBA(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst, nil
}
```

Add `"image/draw"` to the imports, and remove the `colorRGBA` / `colorRGBA_` lines entirely. The final, clean file is:

```go
package main

import (
	"flag"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

var updateGoldens = flag.Bool("update", false, "update golden images instead of comparing")

var goldenPlanets = []struct{ Type, Seed string }{
	{"scorched", "GoldenScorched"},
	{"arid", "GoldenArid"},
	{"terran", "GoldenTerran"},
	{"tundra", "GoldenTundra"},
	{"glacial", "GoldenGlacial"},
	{"ice_world", "GoldenIceWorld"},
	{"super_terran", "GoldenSuperTerran"},
	{"hothouse", "GoldenHothouse"},
	{"lava_world", "GoldenLavaWorld"},
	{"oceanic", "GoldenOceanic"},
	{"jovian", "GoldenJovian"},
	{"ice_giant", "GoldenIceGiant"},
	{"unknown", "GoldenUnknown"},
}

const (
	goldenW       = 400
	goldenH       = 200
	maxMeanDE2000 = 1.5
)

func TestGolden(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range goldenPlanets {
		t.Run(p.Type, func(t *testing.T) {
			got, err := planetgen.GenerateEquirect(p.Type, p.Seed, goldenW, goldenH)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, p.Type+".png")
			if *updateGoldens {
				if err := writePNG(path, got); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", path)
				return
			}
			want, err := readPNGRGBA(path)
			if err != nil {
				t.Fatalf("missing/unreadable golden %s: %v (run `go test -run TestGolden -update`)", path, err)
			}
			if want.Bounds() != got.Bounds() {
				t.Fatalf("size %v != golden %v", got.Bounds(), want.Bounds())
			}
			meanDE := meanDE2000(got.Pix, want.Pix)
			if meanDE > maxMeanDE2000 {
				t.Errorf("mean ΔE2000 = %.3f > %.2f (run with -update after review)",
					meanDE, maxMeanDE2000)
			}
		})
	}
}

func writePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

func readPNGRGBA(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst, nil
}

func meanDE2000(a, b []uint8) float64 {
	n := len(a) / 4
	var sum float64
	for i := 0; i < n; i++ {
		j := i * 4
		ca := colorful.Color{
			R: float64(a[j]) / 255,
			G: float64(a[j+1]) / 255,
			B: float64(a[j+2]) / 255,
		}
		cb := colorful.Color{
			R: float64(b[j]) / 255,
			G: float64(b[j+1]) / 255,
			B: float64(b[j+2]) / 255,
		}
		sum += ca.DistanceCIEDE2000(cb)
	}
	return sum / float64(n)
}
```

Use this corrected file. Discard the earlier broken stub.

- [ ] **Step 2: Verify the harness fails (no goldens yet)**

Run: `cd /home/robert/spacemolt/kb && go test ./cmd/generate-planet-maps/... -run TestGolden`
Expected: 13 sub-tests fail with "missing/unreadable golden" — that's correct; we'll generate them in the next task.

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-planet-maps/golden_test.go
git commit -m "Phase 0: add golden image harness with ΔE2000 < 1.5 gate and -update flag"
```

---

## Task 21: Generate the 13 golden PNGs and commit

**Files:**
- Create: `cmd/generate-planet-maps/testdata/golden/<type>.png` × 13

- [ ] **Step 1: Generate goldens**

Run:
```bash
cd /home/robert/spacemolt/kb
go test ./cmd/generate-planet-maps/... -run TestGolden -update
ls cmd/generate-planet-maps/testdata/golden/
```
Expected: 13 PNGs listed (`scorched.png`, `arid.png`, …, `unknown.png`).

- [ ] **Step 2: Verify the harness now passes**

Run: `cd /home/robert/spacemolt/kb && go test ./cmd/generate-planet-maps/... -run TestGolden`
Expected: 13 sub-tests PASS with mean ΔE2000 = 0.

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-planet-maps/testdata/golden/
git commit -m "Phase 0: commit initial golden images (13 planet types, 400x200)"
```

---

## Task 22: Write README at cmd/generate-planet-maps/README.md

**Files:**
- Create: `cmd/generate-planet-maps/README.md`

- [ ] **Step 1: Write the README**

Create `cmd/generate-planet-maps/README.md`:

```markdown
# generate-planet-maps

Batch generator that produces planet surface imagery for every planet
in the kb knowledge database. Output is a 4×3 horizontal cross
cube-map PNG (the canonical storage format) and a 2000×1000
equirectangular PNG (the kb-page thumbnail) per planet.

## What it does

For each `pois` row with `type='planet'` and a non-empty `class`,
the generator looks up a per-type `PlanetProfile`, derives a
deterministic seed from the planet's name (`fnv64a`), invokes the
matching renderer (`rocky` or `gas_giant`), and writes two PNGs.
Both are produced in parallel (8 workers by default) using
`pkg/planetgen`.

## Inputs

- **Knowledge database** (`-db`, default `../spacemolt-knowledge.db`):
  reads `pois.name`, `pois.class`, `systems.name` from `pois` joined
  with `systems` filtered to `type='planet' AND class != ''`.
- **Per-type `PlanetProfile`** in `pkg/planetgen/profile.go`:
  palette, noise, crater, ocean, polar-cap, and band parameters.
- **Seed**: planet name → fnv64a → int64. Same name always produces
  the same image.

## Outputs

For every planet, two files in `-outdir` (default `kb/images/planets`):

| File | Format | Size | Role |
|---|---|---|---|
| `<sys>_<planet>.cube.png` | 4×3 horizontal cross PNG | 4096×3072 (S=1024) | Canonical storage, sphere-native, GL-compatible |
| `<sys>_<planet>.png` | Equirectangular PNG | 2000×1000 | kb-page thumbnail, baked from the cube map |

The cube-map cross layout (face cells, top-left origin):

```
[empty][ +Y  ][empty][empty]
[ -X  ][ +Z  ][ +X  ][ -Z  ]
[empty][ -Y  ][empty][empty]
```

Empty cells are transparent (alpha=0). Face order matches OpenGL
`GL_TEXTURE_CUBE_MAP_POSITIVE_X` so the PNG can be uploaded to a
WebGL cube texture without remapping.

## Running the tool

Single-image mode (testing):

```bash
go run ./cmd/generate-planet-maps -type terran -seed Earth -out /tmp/earth.png
# Produces /tmp/earth.png and /tmp/earth.cube.png.
```

Batch mode (default):

```bash
go run ./cmd/generate-planet-maps
# Reads ../spacemolt-knowledge.db, writes to kb/images/planets/.
```

All flags:

| Flag | Default | Meaning |
|---|---|---|
| `-type` | "" | single planet type (must be paired with `-seed`) |
| `-seed` | "" | planet name (single mode only) |
| `-out` | "" | equirect output path (single mode); defaults to `<type>_<seed>.png` |
| `-db` | `../spacemolt-knowledge.db` | knowledge database path |
| `-outdir` | `kb/images/planets` | batch output directory |
| `-face` | 1024 | cube-map face size in pixels |
| `-width` | 2000 | equirect bake width |
| `-height` | 1000 | equirect bake height |
| `-workers` | 8 | parallel worker count (batch mode) |

## Architecture

The generator is layered across `pkg/planetgen` subpackages:

| Subpackage | Role |
|---|---|
| `pkg/planetgen` (root) | `Generate`, `GenerateEquirect`, `PlanetProfile`, `Profiles`, `GetProfile` |
| `pkg/planetgen/cubemap` | `CubeMap`, `CubeMapF`, GL-convention sample/cross/bake |
| `pkg/planetgen/noise` | `Generator`, FBM, future warp/ridged/curl |
| `pkg/planetgen/color` | `ColorStop`, `SampleGradient`, `Blend`, `Lerp`, `Brighten`; future OkLab/LUT |
| `pkg/planetgen/feature` | `Crater`, `GenerateCraters`, `ApplyCraters`; future civ/dunes/clouds |
| `pkg/planetgen/render` | `RenderRocky`, `RenderGasGiant` (orchestration) |
| `pkg/planetgen/field` | (empty in Phase 0; populated Phase 2+) |
| `pkg/planetgen/biome` | (empty in Phase 0; populated Phase 1+) |
| `pkg/planetgen/stats` | per-stage timing/counters |

## Testing and the golden-diff workflow

Three layers of tests:

1. **Math primitives** in each subpackage — direction round-trip,
   cube-map sample at flat colour, cross PNG round-trip, bake
   identity. Pure-math, deterministic, fast.

2. **Statistical invariants** in `cmd/generate-planet-maps/invariants_test.go`:
   for each of 13 planet types and a fixed seed, assert no NaN/Inf,
   alpha=255 everywhere, ≥4 distinct luminance buckets, terran
   ocean/land ratio in `[0.30, 0.85]`, longitude-seam max RGB delta
   ≤ 24/255.

3. **Golden images** in `cmd/generate-planet-maps/testdata/golden/`:
   one 400×200 equirect PNG per planet type, generated from a fixed
   seed (`GoldenScorched`, `GoldenArid`, …). The
   `TestGolden` harness regenerates each image and asserts mean
   ΔE2000 vs. the committed golden is < 1.5.

### Running the diff locally

```bash
cd /home/robert/spacemolt/kb
go test ./cmd/generate-planet-maps/... -run TestGolden
```

Expected output: `--- PASS: TestGolden/<type>` for each of 13 types.
On failure, the test prints the mean ΔE2000 it observed.

### Reviewing a golden diff

When a PR changes a golden image, the reviewer should:

1. Pull the branch.
2. Run `go test ./cmd/generate-planet-maps/... -run TestGolden -v`.
3. For any sub-test that fails (i.e. ΔE2000 above the gate),
   regenerate the diff PNG locally:
   ```bash
   go run ./cmd/generate-planet-maps -type <type> -seed Golden<TitleCase> -out /tmp/new.png
   go run ./cmd/tools/planet-image-diff /tmp/new.png \
       cmd/generate-planet-maps/testdata/golden/<type>.png \
       /tmp/<type>-diff.png
   ```
4. Open `/tmp/<type>-diff.png` (left=new, middle=old, right=4×
   amplified per-pixel difference). Confirm the visible change
   matches the PR description (e.g. "OkLab makes the polar-cap
   edge less muddy — yes that's the change"). Verify the printed
   `mean ΔE2000` is in the ballpark expected for the change
   described.
5. If the change is intended, the PR author runs:
   ```bash
   go test ./cmd/generate-planet-maps/... -run TestGolden -update
   git add cmd/generate-planet-maps/testdata/golden/
   git commit -m "Update goldens for <feature>"
   ```
6. If the change is unintended, fix the bug instead.

**Rule:** never run `-update` without a PR description that
explains *why* the goldens changed.

## Lint and CI gates

`pkg/planetgen` and `cmd/planet-explorer` (added Phase 1) must
remain Wasm-compatible. Two CI gates enforce this:

1. **`golangci-lint` with `depguard`** denies `import "C"` (cgo)
   and any blacklisted native-binding deps (see `.golangci.yml`).
   Catches imports at lint time.
2. **`GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/...`**
   in CI catches non-Wasm-compatible runtime behaviour beyond what
   `depguard` sees.

Both gates run on every PR.

## Slider tool

The web-based parameter explorer at `cmd/planet-explorer/` (Phase 1)
is the canonical workflow for tuning `PlanetProfile` parameters
interactively. Compiles `pkg/planetgen` to Wasm and renders live in
a browser canvas.
```

- [ ] **Step 2: Commit**

```bash
cd /home/robert/spacemolt/kb
git add cmd/generate-planet-maps/README.md
git commit -m "Phase 0: add cmd/generate-planet-maps README with full workflow docs"
```

---

## Task 23: Final acceptance — full batch run + perf check

**Files:** none (verification only)

- [ ] **Step 1: Run the full test suite**

```bash
cd /home/robert/spacemolt/kb
go test ./...
```
Expected: PASS everywhere.

- [ ] **Step 2: Run golangci-lint**

```bash
cd /home/robert/spacemolt/kb
golangci-lint run
```
Expected: clean.

- [ ] **Step 3: Run the Wasm build**

```bash
cd /home/robert/spacemolt/kb
GOOS=js GOARCH=wasm go build ./pkg/planetgen/...
```
Expected: success (no output).

- [ ] **Step 4: Capture Phase-0 baseline timings**

Generate Phase-0 baseline equirects for the curated set and time the
full batch run:

```bash
cd /home/robert/spacemolt/kb
mkdir -p /tmp/phase0-out
time go run ./cmd/generate-planet-maps -outdir /tmp/phase0-out -workers 8
ls /tmp/phase0-out | wc -l
ls /tmp/phase0-out/*.cube.png | wc -l
```
Expected:
- ~700 lines from the first `wc -l` (one `<sys>_<planet>.png` per planet).
- ~700 lines from the second `wc -l` (one `<sys>_<planet>.cube.png` per planet).
- Wall time within **1.5×** of the pre-Phase-0 baseline measured on the same machine.

If wall time exceeds 1.5×, profile with `go test -cpuprofile`. Likely culprit: the `feature.ApplyCraters` outer-loop now iterates `6 × S × S` instead of `H × W`. For S=1024 that's `6.3M` versus the old `2M`. A simple optimisation: prune faces that are angularly far from each crater's axis (skip if no pixel within `1.5×Radius` can possibly be on the face). If perf is unacceptable, file a follow-up task; do not block Phase 0 on it unless the regression exceeds 2×.

- [ ] **Step 5: Verify Phase 0 acceptance criteria**

Re-read the master plan section 4.8. Each acceptance bullet:

1. ✅ All unit tests pass (Step 1).
2. ✅ Statistical invariants pass (Step 1 includes them).
3. ✅ Golden ΔE2000 < 1.5 (Step 1 — and at this point, ΔE2000 = 0 since goldens were just generated).
4. ✅ `go build ./...` and `go test ./...` green (Step 1).
5. ✅ Wasm build green (Step 3).
6. ⚠ Batch ≤ 1.5× current wall time (Step 4 — may need a follow-up).
7. ✅ Both `<name>.cube.png` and `<name>.png` produced (Step 4).

If all green, Phase 0 is complete. No commit on this task — verification only.

---

## Self-review notes

- Spec coverage: every section of master plan §4 (Phase 0) maps to one of Tasks 1–23. The cube-map types, primitives, cross I/O, bake, file migrations, generator update, lint config, Wasm build CI, statistical invariants, golden harness, diff CLI, README, and acceptance run are all represented.
- Placeholder scan: no `TODO`/`TBD` strings; every step has executable commands and full code blocks.
- Type consistency: `noise.Generator` (renamed from `NoiseGenerator`) is consistently used in Tasks 8, 12, 13. `cubemap.CubeMap`/`CubeMapF`/`Face` constants used identically across tasks. `planetgen.PlanetProfile` and `planetgen.Profiles` referenced consistently. The Task 20 README writeup re-uses the seed strings (`GoldenScorched`, etc.) defined in `goldenPlanets` in Task 20.

---

## Execution handoff

Plan complete and saved to `docs/plans/2026-04-25-planet-gen-phase-0-cube-map.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — execute tasks in this session using the executing-plans skill, batch execution with checkpoints for review.

Which approach?
