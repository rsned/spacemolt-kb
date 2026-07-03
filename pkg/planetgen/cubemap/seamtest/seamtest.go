// Package seamtest provides helpers for unit-testing seam continuity
// across cube-map face boundaries. Used only by tests; not imported
// by production code.
package seamtest

import (
	"fmt"
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Edge identifies a face's edge.
type Edge int

// Edge identifiers for the four sides of a cube-map face.
const (
	EdgeTop Edge = iota
	EdgeBottom
	EdgeLeft
	EdgeRight
)

// TB is the subset of testing.TB this package needs. testing.T
// satisfies it; tests of seamtest itself use a mock.
type TB interface {
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Helper()
}

// WalkSeams visits every pixel pair on every cube face edge. For each
// pixel on edge E of face F, it finds the matched-pair pixel on the
// adjacent face via cubemap.FaceUVToDir + DirToFacePixel and calls
// cb with both values.
//
// The callback is called once per edge pixel, with here being the
// value on the current face and there being the value on the
// adjacent face. The 12 cube edges are walked in order
// (FacePosX:Top, FacePosX:Bottom, ...).
func WalkSeams[T any](faces [cubemap.NumFaces][]T, S int, cb func(face cubemap.Face, edge Edge, idx int, here, there T)) {
	edges := [4]Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight}
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		for _, e := range edges {
			for i := 0; i < S; i++ {
				px, py := edgePixel(e, i, S)
				here := faces[f][py*S+px]
				tFace, tPx, tPy := adjacentEdgePixel(f, e, i, S)
				there := faces[tFace][tPy*S+tPx]
				cb(f, e, i, here, there)
			}
		}
	}
}

func edgePixel(e Edge, i, S int) (int, int) {
	switch e {
	case EdgeTop:
		return i, 0
	case EdgeBottom:
		return i, S - 1
	case EdgeLeft:
		return 0, i
	case EdgeRight:
		return S - 1, i
	}
	return 0, 0
}

// adjacentEdgePixel returns the (face, px, py) of the matched-pair
// pixel on the neighbor face — the pixel on the other side of the
// shared cube edge that points at the same 3D direction as edge
// pixel i on edge e of face f.
//
// We project the edge pixel's along-edge UV to the edge itself
// (u=0 or u=1 or v=0 or v=1), then step an infinitesimal eps past
// the edge so FaceUVToDir's face-classification flips to the neighbor
// face, then snap to the nearest pixel on that face.
// Stepping one full pixel beyond the edge (the legacy semantics)
// landed a row inside the neighbor face — fine for low-gradient SDFs
// but spurious for high-frequency fields.
func adjacentEdgePixel(f cubemap.Face, e Edge, i, S int) (cubemap.Face, int, int) {
	// eps must survive direction-vector normalization without being
	// rounded back to the source face at corner pixels (where two
	// face axes have nearly equal magnitude). 1e-6 is well above
	// float64 normalize-roundoff (~1e-15) and well below one pixel
	// at any practical face size (1/S ≥ 1e-3 for S ≤ 1024).
	const eps = 1e-6
	along := (float64(i) + 0.5) / float64(S)
	var u, v float64
	switch e {
	case EdgeTop:
		u, v = along, -eps
	case EdgeBottom:
		u, v = along, 1+eps
	case EdgeLeft:
		u, v = -eps, along
	case EdgeRight:
		u, v = 1+eps, along
	}
	dx, dy, dz := cubemap.FaceUVToDir(f, u, v)
	mag := math.Sqrt(dx*dx + dy*dy + dz*dz)
	return cubemap.DirToFacePixel(dx/mag, dy/mag, dz/mag, S)
}

// AssertSeamMatch fails t if any seam pixel pair has different values
// (categorical assertion for plate ids and similar).
func AssertSeamMatch[T comparable](t TB, name string, faces [cubemap.NumFaces][]T, S int) {
	t.Helper()
	var mismatches int
	var firstFace cubemap.Face
	var firstEdge Edge
	var firstIdx int
	var firstHere, firstThere T
	WalkSeams(faces, S, func(face cubemap.Face, edge Edge, idx int, here, there T) {
		if here != there {
			if mismatches == 0 {
				firstFace, firstEdge, firstIdx, firstHere, firstThere = face, edge, idx, here, there
			}
			mismatches++
		}
	})
	if mismatches > 0 {
		t.Errorf("%s: %d seam pixels mismatched; first at face=%v edge=%v idx=%d here=%v there=%v",
			name, mismatches, firstFace, firstEdge, firstIdx, firstHere, firstThere)
	}
}

// NeighborFace returns the face on the other side of edge e of face f.
// Useful when writing custom seam-walk tests that need to look up the
// neighbor face directly without computing a full matched pixel.
func NeighborFace(f cubemap.Face, e Edge) cubemap.Face {
	// 0 is a safe representative i: the neighbor identity depends only
	// on (f, e), not on the position along the edge.
	nf, _, _ := adjacentEdgePixel(f, e, 0, 1)
	return nf
}

// AssertSeamContinuityBilinear is the matched-pair-by-direction
// equivalent of AssertSeamContinuity. For each cube-edge pixel it
// bilinearly samples f at a direction stepped infinitesimally across
// the shared edge — recovering the field's value at the source
// direction on the neighbor face, rather than snapping to the
// nearest neighbor pixel.
//
// This is appropriate for production fields generated as smooth
// functions of direction (TransformDir-based fbm). The nearest-pixel
// AssertSeamContinuity is too strict for high-frequency content
// because sub-pixel direction quantization can swing a high-octave
// value by full amplitude.
func AssertSeamContinuityBilinear(t TB, name string, f *cubemap.CubeMapF, tolPct float64) {
	t.Helper()
	var fmin, fmax = math.Inf(1), math.Inf(-1)
	for face := range f.Faces {
		for _, v := range f.Faces[face] {
			if v < fmin {
				fmin = v
			}
			if v > fmax {
				fmax = v
			}
		}
	}
	rng := fmax - fmin
	if rng == 0 {
		return
	}
	S := f.Size
	edges := [4]Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight}
	var maxDelta float64
	var worstFace cubemap.Face
	var worstEdge Edge
	var worstIdx int
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		for _, e := range edges {
			for i := 0; i < S; i++ {
				px, py := edgePixel(e, i, S)
				here := f.Faces[face][py*S+px]
				// Sample the neighbor face's bilinear value at the
				// source pixel's exact direction. Using ForceFaceUV
				// (rather than f.Sample, which would classify back to
				// the source face) avoids the first-order half-pixel
				// error of edge-direction sampling.
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				nFace, _, _ := adjacentEdgePixel(face, e, i, S)
				nu, nv := cubemap.ForceFaceUV(nFace, dx, dy, dz)
				there := f.SampleFaceUV(nFace, nu, nv)
				if d := math.Abs(here - there); d > maxDelta {
					maxDelta = d
					worstFace, worstEdge, worstIdx = face, e, i
				}
			}
		}
	}
	pct := maxDelta / rng
	if pct > tolPct {
		msg := fmt.Sprintf("%s: bilinear seam delta %.4f (%.2f%% of range %.4f) exceeds %.2f%% — worst at face=%v edge=%v idx=%d",
			name, maxDelta, 100*pct, rng, 100*tolPct, worstFace, worstEdge, worstIdx)
		t.Errorf("%s", msg)
	}
}

// AssertSeamContinuity fails t if any seam pixel pair on f differs by
// more than tolPct (as fraction of f's value range) of the field's
// range. tolPct should be in [0,1] (e.g. 0.01 for 1%).
func AssertSeamContinuity(t TB, name string, f *cubemap.CubeMapF, tolPct float64) {
	t.Helper()
	var fmin, fmax = math.Inf(1), math.Inf(-1)
	for face := range f.Faces {
		for _, v := range f.Faces[face] {
			if v < fmin {
				fmin = v
			}
			if v > fmax {
				fmax = v
			}
		}
	}
	rng := fmax - fmin
	if rng == 0 {
		return // constant — vacuously continuous
	}
	var maxDelta float64
	var worstFace cubemap.Face
	var worstEdge Edge
	var worstIdx int
	WalkSeams(f.Faces, f.Size, func(face cubemap.Face, edge Edge, idx int, a, b float64) {
		d := math.Abs(a - b)
		if d > maxDelta {
			maxDelta = d
			worstFace, worstEdge, worstIdx = face, edge, idx
		}
	})
	pct := maxDelta / rng
	if pct > tolPct {
		msg := fmt.Sprintf("%s: seam delta %.4f (%.2f%% of range %.4f) exceeds %.2f%% — worst at face=%v edge=%v idx=%d",
			name, maxDelta, 100*pct, rng, 100*tolPct, worstFace, worstEdge, worstIdx)
		t.Errorf("%s", msg)
	}
}
