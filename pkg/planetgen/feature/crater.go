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
