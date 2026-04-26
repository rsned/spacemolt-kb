package planetgen

import (
	"image"
	"image/color"
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
			// Inline SphericalCoords
			lon := float64(x) / float64(width) * 2 * math.Pi
			lat := math.Pi/2 - float64(y)/float64(height)*math.Pi
			sx := math.Cos(lat) * math.Cos(lon)
			sy := math.Sin(lat)
			sz := math.Cos(lat) * math.Sin(lon)
			normalizedLat := (lat + math.Pi/2) / math.Pi // [0, 1]

			// Turbulence: distort the latitude to warp bands
			turbulence := turbNg.FractalNoise3D(sx*3, sy*3, sz*3, 4, 2.0, 0.5, 2.0)
			distortedLat := normalizedLat + (turbulence-0.5)*profile.TurbulenceAmp*2

			// Additional horizontal flow distortion
			flowDistort := turbNg.FractalNoise3D(sx*5, sy*1.5, sz*5, 3, 2.0, 0.5, 3.0)
			distortedLat += (flowDistort - 0.5) * profile.TurbulenceAmp

			// Get band color
			bandColor := getBandColor(bands, distortedLat, profile.BandBlendWidth)

			// Fine detail: cloud texture within bands
			detail := detailNg.FractalNoise3D(sx*8, sy*2, sz*8, 5, 2.0, 0.5, 4.0)
			detailFactor := 0.85 + detail*0.3
			c := brighten(bandColor.Color, detailFactor)

			// Storm spots
			for _, storm := range storms {
				dist := stormDistance(lat, float64(x)/float64(width)*2*math.Pi, storm)
				if dist < 1.5 {
					// Swirling flow around storm (1.0-1.5 range)
					if dist >= 1.0 {
						outerT := (1.5 - dist) / 0.5
						outerT *= outerT
						// Deflect surrounding band colors slightly
						deflect := ng.FractalNoise3D(sx*6, sy*6, sz*6, 3, 2.0, 0.5, 4.0)
						deflected := brighten(c, 0.9+deflect*0.2)
						c = blendColor(c, deflected, outerT*0.3)
					} else {
						// Inside storm: swirling vortex
						// Concentric swirl pattern
						angle := math.Atan2(lat-storm.Lat, (float64(x)/float64(width)*2*math.Pi-storm.Lon)*math.Cos(lat))
						swirl := ng.FractalNoise3D(sx*10+math.Cos(angle)*dist*3, sy*10+math.Sin(angle)*dist*3, sz*10, 5, 2.0, 0.5, 5.0)
						stormColor := brighten(storm.Color.Color, 0.8+swirl*0.4)

						// Darker rim, brighter center
						rimFactor := 1.0 - dist*0.25
						stormColor = brighten(stormColor, rimFactor)

						// Full opacity at center, smooth falloff to edge
						blend := (1.0 - dist)
						blend = blend * blend * (3 - 2*blend) // smoothstep
						c = blendColor(c, stormColor, blend)
					}
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

// generateBands creates the horizontal band structure with alternating
// zone (light) and belt (dark) colors, like real gas giants.
// Band widths vary significantly — some are wide prominent features,
// others are thin streaks.
func generateBands(rng *rand.Rand, count int, palette []ColorStop) []band {
	// Generate random widths with high variance
	widths := make([]float64, count)
	total := 0.0
	for i := range count {
		// Use exponential-ish distribution for more variety:
		// some very thin bands, some wide ones
		w := 0.3 + rng.Float64()*rng.Float64()*3.0
		widths[i] = w
		total += w
	}

	// Normalize to sum to 1.0
	bands := make([]band, count)
	pos := 0.0
	for i := range count {
		w := widths[i] / total

		// Alternate between zone (light) and belt (dark) colors
		var colorIdx float64
		if i%2 == 0 {
			colorIdx = 0.55 + rng.Float64()*0.45
		} else {
			colorIdx = rng.Float64() * 0.45
		}

		bands[i] = band{
			LatStart: pos,
			LatEnd:   pos + w,
			Color:    ColorStop{Position: colorIdx, Color: sampleGradient(palette, colorIdx)},
		}
		pos += w
	}
	return bands
}

// getBandColor returns the smoothly interpolated color at the given latitude.
// Near band borders, colors blend smoothly between adjacent bands.
// blendFraction controls how much of each band's width is used for blending (0 = default 0.2).
func getBandColor(bands []band, lat float64, blendFraction float64) ColorStop {
	lat = math.Max(0, math.Min(1, lat))
	if blendFraction == 0 {
		blendFraction = 0.2
	}

	// Find which band we're in
	idx := len(bands) - 1
	for i, b := range bands {
		if lat < b.LatEnd {
			idx = i
			break
		}
	}

	b := bands[idx]
	bandWidth := b.LatEnd - b.LatStart

	// Blend zone: blendFraction of band width at each edge
	blendSize := bandWidth * blendFraction
	if blendSize < 0.003 {
		blendSize = 0.003
	}

	// Distance from band edges
	distFromStart := lat - b.LatStart
	distFromEnd := b.LatEnd - lat

	// Blend with previous band at start edge
	if distFromStart < blendSize && idx > 0 {
		t := distFromStart / blendSize
		t = t * t * (3 - 2*t) // smoothstep
		prev := bands[idx-1]
		blended := lerpColor(prev.Color.Color, b.Color.Color, t)
		return ColorStop{Color: blended}
	}

	// Blend with next band at end edge
	if distFromEnd < blendSize && idx < len(bands)-1 {
		t := distFromEnd / blendSize
		t = t * t * (3 - 2*t) // smoothstep
		next := bands[idx+1]
		blended := lerpColor(next.Color.Color, b.Color.Color, t)
		return ColorStop{Color: blended}
	}

	return b.Color
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
// The first storm is always the largest (Great Red Spot equivalent).
func generateStorms(rng *rand.Rand, count int, size float64) []storm {
	storms := make([]storm, count)
	for i := range count {
		// Storms tend to appear at mid-latitudes
		lat := (rng.Float64() - 0.5) * math.Pi * 0.5
		lon := rng.Float64() * 2 * math.Pi

		var sizeX, sizeY float64
		var col color.RGBA
		if i == 0 {
			// Primary storm: large "Great Red Spot" — saturated red-orange
			sizeX = size * (2.5 + rng.Float64()*1.0)
			sizeY = size * (1.2 + rng.Float64()*0.6)
			col = rgba(
				uint8(200+rng.IntN(30)),
				uint8(80+rng.IntN(30)),
				uint8(50+rng.IntN(20)),
			)
		} else {
			// Secondary storms: smaller, lighter
			sizeX = size * (1.0 + rng.Float64()*0.8)
			sizeY = size * (0.5 + rng.Float64()*0.4)
			col = rgba(
				uint8(190+rng.IntN(40)),
				uint8(150+rng.IntN(50)),
				uint8(110+rng.IntN(40)),
			)
		}

		storms[i] = storm{
			Lat:   lat,
			Lon:   lon,
			SizeX: sizeX,
			SizeY: sizeY,
			Color: ColorStop{Color: col},
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
