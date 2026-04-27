package render

import (
	"image/color"
	"math"
	"math/rand/v2"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// RenderGasGiant generates a gas giant planet cube map.
func RenderGasGiant(profile *types.PlanetProfile, seed int64, S int) *cubemap.CubeMap {
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed*31+7)))
	ng := noise.New(seed)
	detailNg := noise.New(seed + 100)
	turbNg := noise.New(seed + 200)
	warper := noise.NewWarper(seed, profile.Warp)

	// Step 1: Generate band boundaries
	bands := generateGasBands(rng, profile.BandCount, profile.Palette)

	// Step 2+3: Generate storms if any
	storms := generateGasStorms(rng, profile.StormCount, profile.StormSize)

	out := cubemap.New(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				sx, sy, sz := cubemap.FacePixelToDir(face, px, py, S)
				sx, sy, sz = warper.Warp(sx, sy, sz)
				lat := math.Asin(sy)
				lonForX := math.Atan2(sz, sx)
				if lonForX < 0 {
					lonForX += 2 * math.Pi
				}
				normalizedLat := (lat + math.Pi/2) / math.Pi // [0, 1]

				// Turbulence: distort the latitude to warp bands
				turbulence := turbNg.FractalNoise3D(sx*3, sy*3, sz*3, 4, 2.0, 0.5, 2.0)
				distortedLat := normalizedLat + (turbulence-0.5)*profile.TurbulenceAmp*2

				// Additional horizontal flow distortion
				flowDistort := turbNg.FractalNoise3D(sx*5, sy*1.5, sz*5, 3, 2.0, 0.5, 3.0)
				distortedLat += (flowDistort - 0.5) * profile.TurbulenceAmp

				// Get band color
				bc := getGasBandColor(bands, distortedLat, profile.BandBlendWidth)

				// Fine detail: cloud texture within bands
				detail := detailNg.FractalNoise3D(sx*8, sy*2, sz*8, 5, 2.0, 0.5, 4.0)
				detailFactor := 0.85 + detail*0.3
				c := planetcolor.Brighten(bc.Color, detailFactor)

				// Storm spots
				for _, storm := range storms {
					dist := gasStormDistance(lat, lonForX, storm)
					if dist < 1.5 {
						// Swirling flow around storm (1.0-1.5 range)
						if dist >= 1.0 {
							outerT := (1.5 - dist) / 0.5
							outerT *= outerT
							// Deflect surrounding band colors slightly
							deflect := ng.FractalNoise3D(sx*6, sy*6, sz*6, 3, 2.0, 0.5, 4.0)
							deflected := planetcolor.Brighten(c, 0.9+deflect*0.2)
							c = planetcolor.BlendOkLab(c, deflected, outerT*0.3)
						} else {
							// Inside storm: swirling vortex
							// Concentric swirl pattern
							angle := math.Atan2(lat-storm.lat, (lonForX-storm.lon)*math.Cos(lat))
							swirl := ng.FractalNoise3D(sx*10+math.Cos(angle)*dist*3, sy*10+math.Sin(angle)*dist*3, sz*10, 5, 2.0, 0.5, 5.0)
							stormColor := planetcolor.Brighten(storm.color.Color, 0.8+swirl*0.4)

							// Darker rim, brighter center
							rimFactor := 1.0 - dist*0.25
							stormColor = planetcolor.Brighten(stormColor, rimFactor)

							// Full opacity at center, smooth falloff to edge
							blend := (1.0 - dist)
							blend = blend * blend * (3 - 2*blend) // smoothstep
							c = planetcolor.BlendOkLab(c, stormColor, blend)
						}
					}
				}

				// Tier-S Phase 1 Task 26: Apply per-archetype LUT as final color grade
				if name := profile.LUT; name != "" {
					if lut := planetcolor.LookupLUT(name); lut != nil {
						c = planetcolor.ApplyLUT(*lut, c)
					}
				}

				out.Set(face, px, py, c)
			}
		}
	}

	return out
}

// gasBand represents a single atmospheric band.
type gasBand struct {
	LatStart float64 // normalized latitude [0, 1]
	LatEnd   float64
	Color    planetcolor.ColorStop
}

// generateGasBands creates the horizontal band structure with alternating
// zone (light) and belt (dark) colors, like real gas giants.
// Band widths vary significantly — some are wide prominent features,
// others are thin streaks.
func generateGasBands(rng *rand.Rand, count int, palette []planetcolor.ColorStop) []gasBand {
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
	bands := make([]gasBand, count)
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

		bands[i] = gasBand{
			LatStart: pos,
			LatEnd:   pos + w,
			Color:    planetcolor.ColorStop{Position: colorIdx, Color: planetcolor.SampleGradientOkLab(palette, colorIdx)},
		}
		pos += w
	}
	return bands
}

// getGasBandColor returns the smoothly interpolated color at the given latitude.
// Near band borders, colors blend smoothly between adjacent bands.
// blendFraction controls how much of each band's width is used for blending (0 = default 0.2).
func getGasBandColor(bands []gasBand, lat float64, blendFraction float64) planetcolor.ColorStop {
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
		blended := planetcolor.Lerp(prev.Color.Color, b.Color.Color, t)
		return planetcolor.ColorStop{Color: blended}
	}

	// Blend with next band at end edge
	if distFromEnd < blendSize && idx < len(bands)-1 {
		t := distFromEnd / blendSize
		t = t * t * (3 - 2*t) // smoothstep
		next := bands[idx+1]
		blended := planetcolor.Lerp(next.Color.Color, b.Color.Color, t)
		return planetcolor.ColorStop{Color: blended}
	}

	return b.Color
}

// gasStormEntry represents an atmospheric storm feature.
type gasStormEntry struct {
	lat   float64 // latitude in radians
	lon   float64 // longitude in radians
	sizeX float64 // angular radius (wider)
	sizeY float64 // angular radius (taller, typically smaller)
	color planetcolor.ColorStop
}

// generateGasStorms creates storm features at random locations.
// The first storm is always the largest (Great Red Spot equivalent).
func generateGasStorms(rng *rand.Rand, count int, size float64) []gasStormEntry {
	storms := make([]gasStormEntry, count)
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
			col = color.RGBA{
				R: uint8(200 + rng.IntN(30)),
				G: uint8(80 + rng.IntN(30)),
				B: uint8(50 + rng.IntN(20)),
				A: 255,
			}
		} else {
			// Secondary storms: smaller, lighter
			sizeX = size * (1.0 + rng.Float64()*0.8)
			sizeY = size * (0.5 + rng.Float64()*0.4)
			col = color.RGBA{
				R: uint8(190 + rng.IntN(40)),
				G: uint8(150 + rng.IntN(50)),
				B: uint8(110 + rng.IntN(40)),
				A: 255,
			}
		}

		storms[i] = gasStormEntry{
			lat:   lat,
			lon:   lon,
			sizeX: sizeX,
			sizeY: sizeY,
			color: planetcolor.ColorStop{Color: col},
		}
	}
	return storms
}

// gasStormDistance returns the normalized distance from a point to a storm center.
// Returns < 1.0 if inside the storm ellipse.
func gasStormDistance(lat, lon float64, s gasStormEntry) float64 {
	dLat := lat - s.lat
	dLon := lon - s.lon
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
	dx := dLon / s.sizeX
	dy := dLat / s.sizeY
	return math.Sqrt(dx*dx + dy*dy)
}
