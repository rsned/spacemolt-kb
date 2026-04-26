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
		// of the crater axis. We use a per-face pre-filter to skip faces
		// that are far from the cone, then the per-pixel angular check
		// filters non-affected pixels cheaply.
		threshold := math.Cos(c.Radius * 1.5)
		for face := range cubemap.Face(cubemap.NumFaces) {
			if !faceIntersectsCone(face, cx, cy, cz, threshold) {
				continue
			}
			for py := range S {
				for px := range S {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					dot := dx*cx + dy*cy + dz*cz
					if dot < threshold {
						continue
					}
					// dot < -1 is unreachable: the early-exit above
					// guarantees dot >= cos(Radius*1.5) > -1.
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

// faceIntersectsCone reports whether ANY pixel of `face` could lie within
// the angular cone defined by axis (cx,cy,cz) (unit-length) and threshold
// = cos(coneHalfAngle). Conservative: false positives (face accepted but
// no pixel inside) are tolerated; false negatives (face rejected when a
// pixel IS inside) are bugs.
//
// Method: a face's most-distant interior point from the +face axis is a
// corner, at angular distance acos(1/sqrt(3)) ≈ 0.9553 rad ≈ 54.7° from
// the face axis. Any pixel of the face is within that distance of the
// face axis. So if angularDistance(faceAxis, coneAxis) > coneHalfAngle +
// faceHalfDiag, no pixel can fall inside the cone.
func faceIntersectsCone(face cubemap.Face, cx, cy, cz, threshold float64) bool {
	centerDot := faceCenterDot(face, cx, cy, cz)
	if centerDot >= threshold {
		return true
	}
	const faceHalfDiag = 0.9553166181245093 // acos(1/sqrt(3))
	coneHalfAngle := math.Acos(threshold)
	return math.Acos(centerDot) <= coneHalfAngle+faceHalfDiag
}

// faceCenterDot returns the dot product of the +face axis with (cx,cy,cz).
func faceCenterDot(face cubemap.Face, cx, cy, cz float64) float64 {
	switch face {
	case cubemap.FacePosX:
		return cx
	case cubemap.FaceNegX:
		return -cx
	case cubemap.FacePosY:
		return cy
	case cubemap.FaceNegY:
		return -cy
	case cubemap.FacePosZ:
		return cz
	case cubemap.FaceNegZ:
		return -cz
	}
	return 0
}
