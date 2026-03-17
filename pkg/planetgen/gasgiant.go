package planetgen

import (
	"image"
	"math"
	"math/rand/v2"
)

// RenderGasGiant generates a gas giant planet surface map.
func RenderGasGiant(profile *PlanetProfile, seed int64, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed*31+7)))
	ng := NewNoiseGenerator(seed)
	detailNg := NewNoiseGenerator(seed + 100)
	turbNg := NewNoiseGenerator(seed + 200)

	// Step 1: Generate band boundaries
	bands := generateBands(rng, profile.BandCount, profile.Palette)

	// Step 2+3: Generate storms if any
	storms := generateStorms(rng, profile.StormCount, profile.StormSize)

	for y := range height {
		for x := range width {
			sx, sy, sz := SphericalCoords(x, y, width, height)
			lat := math.Pi/2 - float64(y)/float64(height)*math.Pi
			normalizedLat := (lat + math.Pi/2) / math.Pi // [0, 1]

			// Turbulence: distort the latitude to warp bands
			turbulence := turbNg.FractalNoise3D(sx*3, sy*3, sz*3, 4, 2.0, 0.5, 2.0)
			distortedLat := normalizedLat + (turbulence-0.5)*profile.TurbulenceAmp*2

			// Additional horizontal flow distortion
			flowDistort := turbNg.FractalNoise3D(sx*5, sy*1.5, sz*5, 3, 2.0, 0.5, 3.0)
			distortedLat += (flowDistort - 0.5) * profile.TurbulenceAmp

			// Get band color
			bandColor := getBandColor(bands, distortedLat)

			// Fine detail: cloud texture within bands
			detail := detailNg.FractalNoise3D(sx*8, sy*2, sz*8, 5, 2.0, 0.5, 4.0)
			detailFactor := 0.85 + detail*0.3
			c := brighten(bandColor.Color, detailFactor)

			// Storm spots
			for _, storm := range storms {
				dist := stormDistance(lat, float64(x)/float64(width)*2*math.Pi, storm)
				if dist < 1.0 {
					// Inside storm: swirl color
					swirl := ng.FractalNoise3D(sx*12, sy*12, sz*12, 4, 2.0, 0.5, 6.0)
					stormColor := brighten(storm.Color.Color, 0.8+swirl*0.4)
					blend := (1.0 - dist) * (1.0 - dist) // Smooth falloff
					c = blendColor(c, stormColor, blend*0.7)
				}
			}

			img.SetRGBA(x, y, c)
		}
	}

	return img
}

// band represents a single atmospheric band.
type band struct {
	LatStart float64 // normalized latitude [0, 1]
	LatEnd   float64
	Color    ColorStop
}

// generateBands creates the horizontal band structure.
func generateBands(rng *rand.Rand, count int, palette []ColorStop) []band {
	bands := make([]band, count)
	pos := 0.0
	for i := range count {
		width := 1.0/float64(count) + (rng.Float64()-0.5)*0.03
		if i == count-1 {
			width = 1.0 - pos
		}
		// Alternate between palette entries for zone/belt pattern
		colorIdx := float64(i) / float64(count-1)
		bands[i] = band{
			LatStart: pos,
			LatEnd:   pos + width,
			Color:    ColorStop{Position: colorIdx, Color: sampleGradient(palette, colorIdx)},
		}
		pos += width
	}
	return bands
}

// getBandColor returns the interpolated color at the given latitude.
func getBandColor(bands []band, lat float64) ColorStop {
	lat = math.Max(0, math.Min(1, lat))
	for _, b := range bands {
		if lat >= b.LatStart && lat < b.LatEnd {
			return b.Color
		}
	}
	return bands[len(bands)-1].Color
}

// storm represents an atmospheric storm feature.
type storm struct {
	Lat   float64 // latitude in radians
	Lon   float64 // longitude in radians
	SizeX float64 // angular radius (wider)
	SizeY float64 // angular radius (taller, typically smaller)
	Color ColorStop
}

// generateStorms creates storm features at random locations.
func generateStorms(rng *rand.Rand, count int, size float64) []storm {
	storms := make([]storm, count)
	for i := range count {
		// Storms tend to appear at mid-latitudes
		lat := (rng.Float64() - 0.5) * math.Pi * 0.6
		lon := rng.Float64() * 2 * math.Pi
		storms[i] = storm{
			Lat:   lat,
			Lon:   lon,
			SizeX: size * (1.5 + rng.Float64()),
			SizeY: size * (0.6 + rng.Float64()*0.4),
			Color: ColorStop{Color: rgba(
				uint8(180+rng.IntN(40)),
				uint8(100+rng.IntN(60)),
				uint8(60+rng.IntN(40)),
			)},
		}
	}
	return storms
}

// stormDistance returns the normalized distance from a point to a storm center.
// Returns < 1.0 if inside the storm ellipse.
func stormDistance(lat, lon float64, s storm) float64 {
	dLat := lat - s.Lat
	dLon := lon - s.Lon
	// Wrap longitude
	if dLon > math.Pi {
		dLon -= 2 * math.Pi
	}
	if dLon < -math.Pi {
		dLon += 2 * math.Pi
	}
	// Scale by cos(lat) for longitude
	dLon *= math.Cos(lat)
	// Elliptical distance
	dx := dLon / s.SizeX
	dy := dLat / s.SizeY
	return math.Sqrt(dx*dx + dy*dy)
}
