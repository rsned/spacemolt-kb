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

// adjacentEdgePixel returns the (face, px, py) just outside the edge,
// computed by stepping one pixel beyond the edge on the current face
// and re-projecting via FaceUVToDir + DirToFacePixel.
func adjacentEdgePixel(f cubemap.Face, e Edge, i, S int) (cubemap.Face, int, int) {
	px, py := edgePixel(e, i, S)
	var dpx, dpy float64
	switch e {
	case EdgeTop:
		dpy = -1
	case EdgeBottom:
		dpy = +1
	case EdgeLeft:
		dpx = -1
	case EdgeRight:
		dpx = +1
	}
	dx, dy, dz := cubemap.FaceUVToDir(
		f,
		(float64(px)+0.5+dpx)/float64(S),
		(float64(py)+0.5+dpy)/float64(S),
	)
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
