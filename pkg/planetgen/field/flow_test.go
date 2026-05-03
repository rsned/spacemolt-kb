package field

import (
	"math"
	"reflect"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// terranLikeHeightmap synthesizes a roughly-realistic terran heightmap
// at face size S using GenerateControlFields. It avoids importing the
// render package (which would be a dependency cycle: render → field).
//
// The composition: Continentalness contributes the broad landmass
// plus a continental-shelf spline so values fall in [0,1]; PeaksValleys
// adds a smaller mountainous overlay. The output is clamped to [0,1].
func terranLikeHeightmap(seed int64, S int) *cubemap.CubeMapF {
	cfg := types.ControlConfig{
		Continentalness: types.ControlField{
			Amp: 1.0, Freq: 2.0, Octaves: 5, Lacunarity: 2.0, Persistence: 0.5,
		},
		Detail: types.ControlField{
			Amp: 0.2, Freq: 4.0, Octaves: 3, Lacunarity: 2.0, Persistence: 0.5,
		},
		PeaksValleys: types.ControlField{
			Amp: 0.3, Freq: 2.5, Octaves: 4, Lacunarity: 2.0, Persistence: 0.5,
		},
	}
	fields := GenerateControlFields(seed, cfg, S, nil)
	hm := cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for i := range hm.Faces[face] {
			h := 0.6*fields[0].Faces[face][i] +
				0.3*fields[2].Faces[face][i] +
				0.1*fields[1].Faces[face][i]
			if h < 0 {
				h = 0
			} else if h > 1 {
				h = 1
			}
			hm.Faces[face][i] = h
		}
	}
	return hm
}

func TestFlowDeterministic(t *testing.T) {
	S := 64
	h1 := terranLikeHeightmap(42, S)
	h2 := terranLikeHeightmap(42, S)
	cfg := types.FlowConfig{RiverThreshold: 200, RiverDepth: 0.02}

	ff1 := GenerateFlow(h1, cfg)
	ff2 := GenerateFlow(h2, cfg)
	if ff1 == nil || ff2 == nil {
		t.Fatalf("GenerateFlow returned nil for enabled cfg")
	}
	for f := range ff1.D8 {
		if !reflect.DeepEqual(ff1.D8[f], ff2.D8[f]) {
			t.Errorf("D8 face %d differs", f)
		}
		if !reflect.DeepEqual(ff1.Accum[f], ff2.Accum[f]) {
			t.Errorf("Accum face %d differs", f)
		}
		if !reflect.DeepEqual(ff1.Rivers[f], ff2.Rivers[f]) {
			t.Errorf("Rivers face %d differs", f)
		}
	}
}

func TestFlowDisabledByZeroThreshold(t *testing.T) {
	S := 32
	h := terranLikeHeightmap(7, S)
	if ff := GenerateFlow(h, types.FlowConfig{}); ff != nil {
		t.Errorf("GenerateFlow returned non-nil for zero-threshold cfg")
	}
	if ff := GenerateFlow(h, types.FlowConfig{RiverThreshold: -1}); ff != nil {
		t.Errorf("GenerateFlow returned non-nil for negative threshold")
	}
}

// TestFlowRiverPixelsFormConnectedComponents flood-fills the river
// mask through 8-connected face-aware neighbors and asserts that the
// resulting connected-component count is small (1..50). This catches
// the failure mode "every pixel is a river" (no D8 chain ever
// terminates because the steepest-descent search returned bogus
// pointers) and "no rivers at all" (threshold misapplied).
func TestFlowRiverPixelsFormConnectedComponents(t *testing.T) {
	S := 64
	h := terranLikeHeightmap(42, S)
	cfg := types.FlowConfig{RiverThreshold: 200, RiverDepth: 0.02}
	ff := GenerateFlow(h, cfg)
	if ff == nil {
		t.Fatal("GenerateFlow returned nil")
	}

	visited := [cubemap.NumFaces][]bool{}
	for f := range visited {
		visited[f] = make([]bool, S*S)
	}
	components := 0
	totalRivers := 0
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				if !ff.Rivers[face][py*S+px] {
					continue
				}
				totalRivers++
				if visited[face][py*S+px] {
					continue
				}
				components++
				// BFS through 8-connected face-aware neighbors.
				stack := []cubemap.PixelAddr{{Face: face, PX: px, PY: py}}
				for len(stack) > 0 {
					p := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					if visited[p.Face][p.PY*S+p.PX] {
						continue
					}
					if !ff.Rivers[p.Face][p.PY*S+p.PX] {
						continue
					}
					visited[p.Face][p.PY*S+p.PX] = true
					nbrs := cubemap.FacePixelNeighbors8(p.Face, p.PX, p.PY, S)
					for _, n := range nbrs {
						if !visited[n.Face][n.PY*S+n.PX] && ff.Rivers[n.Face][n.PY*S+n.PX] {
							stack = append(stack, n)
						}
					}
				}
			}
		}
	}
	if totalRivers == 0 {
		t.Fatalf("no river pixels — threshold %v is wrong or D8 broken", cfg.RiverThreshold)
	}
	if components < 1 || components > 50 {
		t.Errorf("connected components %d out of expected range [1,50]; total river pixels=%d",
			components, totalRivers)
	}
	t.Logf("rivers: %d pixels in %d connected components", totalRivers, components)
}

// TestFlowSeamContinuity asserts that the Accum field is seam-
// continuous: looking up a pixel adjacent to the cube seam from each
// side should yield similar accumulation values. The threshold is
// 5% of the field's range because rivers are inherently spike-y;
// fields like Distance-to-Coast use 1%, which is too tight here.
func TestFlowSeamContinuity(t *testing.T) {
	S := 64
	h := terranLikeHeightmap(42, S)
	cfg := types.FlowConfig{RiverThreshold: 200, RiverDepth: 0.02}
	ff := GenerateFlow(h, cfg)
	if ff == nil {
		t.Fatal("GenerateFlow returned nil")
	}

	// Compute the field range to scale the tolerance.
	minA, maxA := math.Inf(1), math.Inf(-1)
	for f := range ff.Accum {
		for _, v := range ff.Accum[f] {
			if v < minA {
				minA = v
			}
			if v > maxA {
				maxA = v
			}
		}
	}
	rng := maxA - minA
	if rng <= 0 {
		t.Fatalf("Accum range is zero (max=%f, min=%f)", maxA, minA)
	}
	tolerance := 0.05 * rng

	// Walk every edge pixel; compare Accum against its mapped twin
	// across the seam (the +x neighbor when at px==S-1, etc.).
	maxDelta := 0.0
	violations := 0
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				if px > 0 && px < S-1 && py > 0 && py < S-1 {
					continue
				}
				here := ff.Accum[face][py*S+px]
				nbrs := cubemap.FacePixelNeighbors8(face, px, py, S)
				for _, n := range nbrs {
					if n.Face == face {
						continue
					}
					there := ff.Accum[n.Face][n.PY*S+n.PX]
					d := math.Abs(here - there)
					if d > maxDelta {
						maxDelta = d
					}
					if d > tolerance {
						violations++
					}
				}
			}
		}
	}
	// Count actual cross-face neighbor comparisons made.
	totalCrossPairs := 0
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				if px > 0 && px < S-1 && py > 0 && py < S-1 {
					continue
				}
				for _, n := range cubemap.FacePixelNeighbors8(face, px, py, S) {
					if n.Face != face {
						totalCrossPairs++
					}
				}
			}
		}
	}
	t.Logf("seam: max-delta=%f tolerance=%f range=%f violations=%d/%d (%.2f%%)",
		maxDelta, tolerance, rng, violations, totalCrossPairs,
		100.0*float64(violations)/float64(totalCrossPairs))

	// Rivers are spike-y — accumulation can grow by an order of
	// magnitude as a chain crosses the seam. We allow up to 10% of
	// cross-face pairs to exceed the 5%-of-range tolerance; anything
	// larger likely indicates per-face D8 pointers (the seam-bug
	// signature).
	if violations*10 > totalCrossPairs {
		t.Errorf("too many seam violations: %d / %d (>10%%)",
			violations, totalCrossPairs)
	}
}

// TestPlanchonDarbouxRaisesLocalMinima constructs a synthetic cube
// map with a 5×5 closed pit in the centre of FacePosX (elevation 0.3
// inside a plateau at 0.5). The cube also has a single deep sea-level
// pixel on FaceNegX at elevation 0.0, which serves as the global
// minimum (drained seed) — the pit on FacePosX is therefore a true
// closed depression and must be raised to the saddle elevation 0.5.
//
// After fill: every pit pixel equals 0.5 (lowest-saddle); the seed
// pixel stays at 0.0; no pixel anywhere is lowered below the original.
func TestPlanchonDarbouxRaisesLocalMinima(t *testing.T) {
	S := 16
	hm := cubemap.NewF(S)
	// Default plateau at 0.5 everywhere.
	for f := range hm.Faces {
		for i := range hm.Faces[f] {
			hm.Faces[f][i] = 0.5
		}
	}
	// Single drained-seed pixel at sea level on FaceNegX.
	hm.Set(cubemap.FaceNegX, S/2, S/2, 0.0)
	// Carve a 5×5 pit in the centre of FacePosX at elevation 0.3.
	// Surrounding plateau is 0.5 — this is the lowest-saddle.
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			hm.Set(cubemap.FacePosX, S/2+dx, S/2+dy, 0.3)
		}
	}

	filled := planchonDarbouxFill(hm)

	// No pixel should have been lowered below its original elevation.
	for f := range filled.Faces {
		for i, v := range filled.Faces[f] {
			if v < hm.Faces[f][i]-1e-12 {
				t.Errorf("pixel face=%d idx=%d lowered: original=%f filled=%f",
					f, i, hm.Faces[f][i], v)
			}
		}
	}

	// Seed pixel stays at 0.0.
	if got := filled.Get(cubemap.FaceNegX, S/2, S/2); math.Abs(got) > 1e-12 {
		t.Errorf("seed pixel: got %f, want 0.0", got)
	}

	// The entire 5×5 pit interior should have been raised to 0.5
	// (the saddle elevation around the closed depression).
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			got := filled.Get(cubemap.FacePosX, S/2+dx, S/2+dy)
			if math.Abs(got-0.5) > 1e-9 {
				t.Errorf("pit pixel (%d,%d): got %f, want 0.5",
					S/2+dx, S/2+dy, got)
			}
		}
	}
}
