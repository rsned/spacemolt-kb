package planetgen

import (
	"math"
	"math/rand/v2"
	"sort"
)

// Crater represents a single crater on the planet surface.
type Crater struct {
	// Spherical coordinates of center
	Lat, Lon float64
	// Angular radius on the sphere
	Radius float64
}

// GenerateCraters creates a list of craters distributed on a sphere.
func GenerateCraters(rng *rand.Rand, count int, minRadius, maxRadius float64) []Crater {
	craters := make([]Crater, count)
	for i := range count {
		// Uniform distribution on sphere
		lat := math.Asin(2*rng.Float64() - 1)
		lon := rng.Float64() * 2 * math.Pi

		// Power-law size distribution: many small, few large
		t := rng.Float64()
		radius := minRadius + (maxRadius-minRadius)*t*t

		craters[i] = Crater{
			Lat:    lat,
			Lon:    lon,
			Radius: radius,
		}
	}

	// Sort largest first so small craters overlay large ones
	sort.Slice(craters, func(i, j int) bool {
		return craters[i].Radius > craters[j].Radius
	})

	return craters
}

// ApplyCraters modifies a heightmap by stamping craters onto it.
// The heightmap is indexed as [y][x] with values in [0, 1].
func ApplyCraters(heightmap [][]float64, craters []Crater, width, height int, depth float64) {
	for _, c := range craters {
		// Convert crater center to 3D
		cx := math.Cos(c.Lat) * math.Cos(c.Lon)
		cy := math.Sin(c.Lat)
		cz := math.Cos(c.Lat) * math.Sin(c.Lon)

		// Only process pixels that could be within range
		// Convert angular radius to approximate pixel range
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
				// Wrap x coordinate
				wpx := ((px % width) + width) % width

				// Get 3D position of this pixel
				// Inline SphericalCoords
				crater_lon := float64(wpx) / float64(width) * 2 * math.Pi
				crater_lat := math.Pi/2 - float64(py)/float64(height)*math.Pi
				sx := math.Cos(crater_lat) * math.Cos(crater_lon)
				sy := math.Sin(crater_lat)
				sz := math.Cos(crater_lat) * math.Sin(crater_lon)

				// Angular distance on sphere
				dot := sx*cx + sy*cy + sz*cz
				dot = math.Max(-1, math.Min(1, dot))
				dist := math.Acos(dot)

				if dist < c.Radius {
					// Crater profile: bowl shape with rim
					t := dist / c.Radius // 0 at center, 1 at edge
					var mod float64
					if t < 0.8 {
						// Bowl: parabolic depression
						mod = -depth * (1 - (t/0.8)*(t/0.8))
					} else {
						// Rim: slight rise at edge
						rimT := (t - 0.8) / 0.2
						mod = depth * 0.15 * math.Sin(rimT*math.Pi)
					}
					heightmap[py][wpx] += mod
				}
			}
		}
	}

	// Clamp heightmap to [0, 1]
	for py := range height {
		for px := range width {
			heightmap[py][px] = math.Max(0, math.Min(1, heightmap[py][px]))
		}
	}
}
