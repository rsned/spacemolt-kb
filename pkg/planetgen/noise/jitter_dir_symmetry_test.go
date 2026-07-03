package noise_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// TestJitterTransformDirectionSymmetry demonstrates that a synthetic
// low-freq fbm sampled through JitterField.Transform produces a
// matched-pair-by-direction seam-continuous cube map, except at
// pixels whose bilinear neighborhood on the neighbor face straddles
// a Voronoi cell boundary. The cell-boundary discontinuity is
// intentional (Phase 7 §3.3 explicitly rejects cell-boundary
// smoothing).
//
// This is an existence proof for the claim in the skipped
// production-frequency test jitter_seam_test.go: the underlying
// pipeline IS direction-only-continuous, and the large deltas at
// production frequencies come from cell-boundary discontinuity plus
// fbm Nyquist aliasing, not from a code bug in the sampling path.
//
// Caveat: this test does NOT strongly distinguish Transform from the
// deprecated TransformPixel — both agree on cell membership at most
// directions, differing only when sub-pixel direction offset crosses
// a cell boundary between adjacent faces (rare at 8 cells). It guards
// against a gross regression (per-face raster sampling that wildly
// mis-classifies cells across faces) via the 25% per-pair ceiling.
func TestJitterTransformDirectionSymmetry(t *testing.T) {
	S := 128
	profile := &types.PlanetProfile{
		JitterEnabled:   true,
		JitterCellCount: 8,
		JitterRotMax:    0.2,
		JitterOffsetMax: 0.05,
	}
	jf := noise.GenerateJitter(profile, 42, S)
	if jf == nil {
		t.Fatal("expected non-nil JitterField for JitterEnabled=true")
	}

	cf := types.ControlField{Amp: 1.0, Freq: 1, Octaves: 1, Lacunarity: 2, Persistence: 0.5}
	out := cubemap.NewF(S)
	g := noise.New(99)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				jx, jy, jz := jf.Transform(dx, dy, dz)
				out.Set(face, px, py,
					g.FractalNoise3D(jx, jy, jz, cf.Octaves, cf.Lacunarity, cf.Persistence, cf.Freq)*cf.Amp)
			}
		}
	}

	// What we're guarding against: a regression to per-face raster
	// sampling (the deprecated TransformPixel path). That regression
	// would mis-classify cells differently on each face → ~30-80% of
	// every seam edge would mismatch. With the correct direction-only
	// Transform, only seam pairs whose 4 bilinear-neighborhood pixels
	// span a cell boundary on the neighbor face show non-trivial
	// delta. Those pairs cluster near Voronoi cell boundaries and
	// scale with cell count.
	//
	// Bound: at most 15% of seam pairs may exceed the 3% "smooth-fbm"
	// floor (these are the cell-boundary-adjacent pairs at the
	// bilinear scale), AND no seam pair may exceed 25% (a per-face
	// raster regression would put many pairs above 50%).
	S2 := out.Size
	edges := [4]seamtest.Edge{seamtest.EdgeTop, seamtest.EdgeBottom, seamtest.EdgeLeft, seamtest.EdgeRight}
	totalPairs := 6 * 4 * S2
	boundaryPairs := 0
	maxDelta := 0.0
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		for _, e := range edges {
			for i := 0; i < S2; i++ {
				here, there := samplePairBilinear(out, face, e, i)
				delta := absf(here - there)
				if delta > maxDelta {
					maxDelta = delta
				}
				if delta > 0.03 {
					boundaryPairs++
				}
			}
		}
	}
	if frac := float64(boundaryPairs) / float64(totalPairs); frac > 0.15 {
		t.Errorf("seam pairs over 3%% delta = %.2f%% (>15%%) — direction-only sampling may have regressed",
			100*frac)
	}
	if maxDelta > 0.25 {
		t.Errorf("worst seam delta %.2f%% exceeds 25%% ceiling — per-face raster regression suspected",
			100*maxDelta)
	}
}

// samplePairBilinear returns (here, there) for one seam pair: here is
// the source pixel's value, there is the neighbor face's bilinear value
// at the source pixel's exact direction. Matched-pair-by-direction.
func samplePairBilinear(f *cubemap.CubeMapF, face cubemap.Face, e seamtest.Edge, i int) (float64, float64) {
	S := f.Size
	var px, py int
	switch e {
	case seamtest.EdgeTop:
		px, py = i, 0
	case seamtest.EdgeBottom:
		px, py = i, S-1
	case seamtest.EdgeLeft:
		px, py = 0, i
	case seamtest.EdgeRight:
		px, py = S-1, i
	}
	here := f.Faces[face][py*S+px]
	dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
	nFace := seamtest.NeighborFace(face, e)
	nu, nv := cubemap.ForceFaceUV(nFace, dx, dy, dz)
	return here, f.SampleFaceUV(nFace, nu, nv)
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
