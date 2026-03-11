package systemmap

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
)

// PlanetClass holds data for a planet classification.
type PlanetClass struct {
	ID       string
	Name     string
	Color    string
	Size     float64 // radius in SVG pixels (relative to base 5px)
	HasRings bool
}

// planetClasses maps planet class identifiers to their properties.
// Based on actual game data with occurrence counts: scorched(351), arid(249), terran(147), tundra(66), glacial(37), jovian(23), ice_world(12), ice_giant(2), super_terran(2), hothouse(1), lava_world(1), oceanic(1)
// Sizes are relative to base radius of 5px (terran)
var planetClasses = map[string]PlanetClass{
	"arid":        {ID: "arid", Name: "Arid", Color: "#d4b596", Size: 4.5, HasRings: false},      // dry, tan-brown, 0.9x, 249 occurrences
	"glacial":     {ID: "glacial", Name: "Glacial", Color: "#b0e0e6", Size: 5.0, HasRings: false},  // icy, blue-white, 1x, 37 occurrences
	"hothouse":    {ID: "hothouse", Name: "Hothouse", Color: "#58c868", Size: 5.0, HasRings: false}, // extreme greenhouse, bright green, 1x, 1 occurrence
	"ice_giant":   {ID: "ice_giant", Name: "Ice Giant", Color: "#9fc5e8", Size: 7.0, HasRings: true},  // ice giant, pale blue, 1.4x, 2 occurrences (has rings)
	"ice_world":   {ID: "ice_world", Name: "Ice World", Color: "#a0d0f0", Size: 5.5, HasRings: false}, // frozen, pale blue, 1.1x, 12 occurrences
	"jovian":      {ID: "jovian", Name: "Jovian", Color: "#e8a86c", Size: 10.0, HasRings: true},      // gas giant, orange with rings, 2x, 23 occurrences
	"lava_world":  {ID: "lava_world", Name: "Lava World", Color: "#ff4500", Size: 4.0, HasRings: false}, // molten, bright red-orange, 0.8x, 1 occurrence
	"oceanic":     {ID: "oceanic", Name: "Oceanic", Color: "#4a90d9", Size: 5.0, HasRings: false},   // water world, deep blue, 1x, 1 occurrence
	"scorched":    {ID: "scorched", Name: "Scorched", Color: "#d46a4e", Size: 3.0, HasRings: false}, // burnt, dark red-orange, 0.6x, 351 occurrences
	"super_terran": {ID: "super_terran", Name: "Super Terran", Color: "#3d8b5d", Size: 6.25, HasRings: false}, // large earth-like, deep green, 1.25x, 2 occurrences
	"terran":      {ID: "terran", Name: "Terran", Color: "#4a9c6d", Size: 5.0, HasRings: false},     // earth-like, blue-green, 1x (base), 147 occurrences
	"tundra":      {ID: "tundra", Name: "Tundra", Color: "#9cad8e", Size: 4.5, HasRings: false},     // cold, grayish-green, 0.9x, 66 occurrences
}

// GetPlanetClass returns the PlanetClass for a given class identifier.
// Returns a default "unknown" class if not found.
func GetPlanetClass(class string) PlanetClass {
	if pc, ok := planetClasses[class]; ok {
		return pc
	}
	return PlanetClass{
		ID:       "unknown",
		Name:     "Planet",
		Color:    "#A3BE8C",
		HasRings: false,
	}
}

// renderPlanet generates SVG for a planet POI with color and size based on class.
func renderPlanet(poi POI, px, py float64) string {
	var b strings.Builder

	pc := GetPlanetClass(poi.Class)

	b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, poiTitle(poi)))

	// Selection circle (scaled based on planet size)
	selectionRadius := 20.0
	if pc.Size > 5 {
		selectionRadius = 20.0 + (pc.Size-5.0)*2
	}
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py, selectionRadius))

	// Render rings for jovian and ice_giant planets first (behind)
	if pc.HasRings {
		b.WriteString(renderPlanetRings(px, py, pc.Size, pc.ID))
	}

	// Planet with bands for gas giants
	if pc.ID == "jovian" || pc.ID == "ice_giant" {
		// Create clip path for the planet circle
		clipID := fmt.Sprintf("planet-clip-%s", poi.ID)
		b.WriteString(fmt.Sprintf(`<defs><clipPath id="%s"><circle cx="%.1f" cy="%.1f" r="%.2f"/></clipPath></defs>`, clipID, px, py, pc.Size))

		// Base planet circle
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.2f" fill="%s"/>`, px, py, pc.Size, pc.Color))

		// Add bands clipped to planet
		b.WriteString(renderPlanetBands(px, py, pc.Size, pc.ID, clipID))
	} else if pc.ID == "lava_world" {
		// Lava world with red lava and dark brown land
		clipID := fmt.Sprintf("planet-clip-%s", poi.ID)
		b.WriteString(fmt.Sprintf(`<defs><clipPath id="%s"><circle cx="%.1f" cy="%.1f" r="%.2f"/></clipPath></defs>`, clipID, px, py, pc.Size))

		// Base (terran brown land - 60%)
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.2f" fill="#8B7355"/>`, px, py, pc.Size))

		// Add lava (red splotches, 40% coverage)
		b.WriteString(renderPlanetLava(px, py, pc.Size, poi.ID, clipID))
	} else if pc.ID == "terran" || pc.ID == "super_terran" || pc.ID == "oceanic" {
		// Earth-like planets with continents and clouds
		clipID := fmt.Sprintf("planet-clip-%s", poi.ID)
		b.WriteString(fmt.Sprintf(`<defs><clipPath id="%s"><circle cx="%.1f" cy="%.1f" r="%.2f"/></clipPath></defs>`, clipID, px, py, pc.Size))

		// Ocean base (blue)
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.2f" fill="#4a90d9"/>`, px, py, pc.Size))

		// Add continents (green splotches)
		landRatio := 0.66 // Default for terran/super_terran
		if pc.ID == "oceanic" {
			landRatio = 0.10 // 10% land for oceanic
		}
		b.WriteString(renderPlanetContinents(px, py, pc.Size, poi.ID, clipID, landRatio))

		// Add clouds on top (not clipped)
		b.WriteString(renderPlanetClouds(px, py, pc.Size, poi.ID))
	} else {
		// Other rocky planets with color variations to break up uniformity
		clipID := fmt.Sprintf("planet-clip-%s", poi.ID)
		b.WriteString(fmt.Sprintf(`<defs><clipPath id="%s"><circle cx="%.1f" cy="%.1f" r="%.2f"/></clipPath></defs>`, clipID, px, py, pc.Size))

		// Base planet circle
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.2f" fill="%s"/>`, px, py, pc.Size, pc.Color))

		// Add splotches of adjacent colors
		b.WriteString(renderPlanetSplotches(px, py, pc.Size, pc.ID, clipID, pc.Color))
	}

	b.WriteString(`</g>`)

	// Label (adjusted based on planet size)
	labelOffset := 8.0
	if pc.Size > 5 {
		labelOffset = 8.0 + (pc.Size-5.0)*0.5
	}
	labelAbove := poi.PositionY >= 0
	if labelAbove {
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" class="map-label">%s</text>`, px, py-labelOffset, htmlEscape(poi.Name)))
	} else {
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" class="map-label">%s</text>`, px, py+labelOffset+8, htmlEscape(poi.Name)))
	}

	return b.String()
}

// renderPlanetRings generates SVG ellipses for rings around a planet.
// For jovian: 3 rings with warm colors. For ice_giant: 2 rings with cool colors.
func renderPlanetRings(cx, cy, planetRadius float64, planetClass string) string {
	var b strings.Builder

	rx := planetRadius * 2.5
	ry := planetRadius * 0.6
	transform := fmt.Sprintf("transform=\"rotate(-30, %.1f, %.1f)\"", cx, cy)

	if planetClass == "ice_giant" {
		// Ice giant rings: 2 rings with cool colors (pale blue/white)
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#d8e8f8" stroke-width="1.5" opacity="0.5" %s/>`, cx, cy, rx, ry, transform))
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#e8f0f8" stroke-width="1" opacity="0.4" %s/>`, cx, cy, rx*0.8, ry*0.8, transform))
	} else {
		// Jovian rings: 3 rings with warm colors
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#d4a86c" stroke-width="1.5" opacity="0.5" %s/>`, cx, cy, rx, ry, transform))
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#e8b87c" stroke-width="1" opacity="0.4" %s/>`, cx, cy, rx*0.85, ry*0.85, transform))
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#f0c88c" stroke-width="0.7" opacity="0.3" %s/>`, cx, cy, rx*0.7, ry*0.7, transform))
	}

	return b.String()
}

// renderPlanetBands generates curved bands on gas giant planets for spherical effect.
func renderPlanetBands(cx, cy, planetRadius float64, planetClass, clipID string) string {
	var b strings.Builder

	// Band colors: similar/adjacent to main planet color
	var bandColors []string
	if planetClass == "jovian" {
		// Jovian is orange (#e8a86c) - use warm adjacent colors
		bandColors = []string{
			"#d49a5c", // Darker orange
			"#e8b87c", // Light orange
			"#c48a4c", // Brown-orange
			"#f0c88c", // Pale orange-yellow
		}
	} else {
		// Ice giant is pale blue (#9fc5e8) - use cool adjacent colors
		bandColors = []string{
			"#8fb5d8", // Darker blue
			"#afd5f0", // Lighter blue
			"#7fa5c8", // Deep blue
			"#bfe5f8", // Pale cyan
		}
	}

	// Draw 5-7 thin curved bands
	// Use ellipses with varying radii to create curved bands
	numBands := 5 + len(bandColors)%3 // Varies between 5-7 bands

	for i := 0; i < numBands; i++ {
		// Calculate band position (spread across planet)
		// Use a sine wave distribution for natural spacing
		bandY := cy + (float64(i)-float64(numBands)/2.0) * (planetRadius * 0.35)

		// Band width gets thinner toward edges for spherical effect
		normalizedPos := math.Abs(float64(i)-float64(numBands)/2.0) / (float64(numBands) / 2.0)
		bandHeight := (planetRadius * 0.15) * (1.0 - normalizedPos*0.4)

		// Band width (horizontal) gets narrower toward edges
		bandWidth := planetRadius * 1.6 * (1.0 - normalizedPos*0.3)

		// Select color with some variation
		color := bandColors[i%len(bandColors)]
		opacity := 0.3 + float64(i%3)*0.1 // Varying opacity for depth

		// Use ellipse for curved band, clipped to planet circle
		// The aspect ratio (ry vs rx) creates the curved appearance
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="%s" opacity="%.2f" clip-path="url(#%s)"/>`,
			cx, bandY, bandWidth, bandHeight, color, opacity, clipID))
	}

	return b.String()
}

// renderPlanetContinents generates green continent splotches on earth-like planets.
func renderPlanetContinents(cx, cy, planetRadius float64, planetID, clipID string, landRatio float64) string {
	var b strings.Builder

	// Number of continents based on land ratio
	numContinents := 3
	if landRatio > 0.5 {
		numContinents = 5 // More land for terran/super_terran
	}

	// Use planet ID as seed for consistent continent positions
	seed := simpleHash(planetID)
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x636f6e74))

	// Green shades for vegetation
	landColors := []string{
		"#3d8b5d", // Base green
		"#4a9c6d", // Lighter green
		"#2d7b4d", // Darker green
		"#5aac7d", // Forest green
	}

	for i := 0; i < numContinents; i++ {
		// Random position on planet
		angle := rng.Float64() * 2 * math.Pi
		distance := rng.Float64() * planetRadius * 0.7
		contX := cx + math.Cos(angle)*distance
		contY := cy + math.Sin(angle)*distance

		// Continent size varies
		baseSize := planetRadius * (0.3 + rng.Float64()*0.4)
		if landRatio < 0.2 {
			baseSize *= 0.6 // Smaller continents for oceanic
		}

		// Create irregular continent shape using multiple overlapping ellipses
		numSplotches := 3 + rng.IntN(3)
		for j := 0; j < numSplotches; j++ {
			offsetX := (rng.Float64() - 0.5) * baseSize
			offsetY := (rng.Float64() - 0.5) * baseSize
			splotchX := contX + offsetX
			splotchY := contY + offsetY

			rx := baseSize * (0.6 + rng.Float64()*0.4)
			ry := baseSize * (0.4 + rng.Float64()*0.4)
			rotation := rng.Float64() * 360

			color := landColors[rng.IntN(len(landColors))]
			opacity := 0.7 + rng.Float64()*0.2

			b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="%s" opacity="%.2f" transform="rotate(%.0f,%.1f,%.1f)" clip-path="url(#%s)"/>`,
				splotchX, splotchY, rx, ry, color, opacity, rotation, splotchX, splotchY, clipID))
		}
	}

	return b.String()
}

// renderPlanetClouds adds white cloud layer on top of planets.
func renderPlanetClouds(cx, cy, planetRadius float64, planetID string) string {
	var b strings.Builder

	// Use planet ID as seed for consistent cloud positions
	seed := simpleHash(planetID + "clouds")
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x636c6f64))

	// Number of clouds based on planet size
	numClouds := 4 + int(planetRadius)

	for i := 0; i < numClouds; i++ {
		// Random position, but biased toward visible upper hemisphere
		angle := rng.Float64() * 2 * math.Pi
		distance := rng.Float64() * planetRadius * 0.8
		cloudX := cx + math.Cos(angle)*distance
		cloudY := cy + math.Sin(angle)*distance

		// Cloud size
		cloudWidth := planetRadius * (0.15 + rng.Float64()*0.25)
		cloudHeight := cloudWidth * 0.4

		// Create fluffy cloud using multiple overlapping ellipses
		numPuffs := 3 + rng.IntN(3)
		for j := 0; j < numPuffs; j++ {
			puffOffsetX := (rng.Float64() - 0.5) * cloudWidth * 0.8
			puffOffsetY := (rng.Float64() - 0.5) * cloudHeight * 0.5

			puffX := cloudX + puffOffsetX
			puffY := cloudY + puffOffsetY
			puffWidth := cloudWidth * (0.4 + rng.Float64()*0.3)
			puffHeight := cloudHeight * (0.6 + rng.Float64()*0.4)

			opacity := 0.6 + rng.Float64()*0.25

			b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="#FFFFFF" opacity="%.2f"/>`,
				puffX, puffY, puffWidth, puffHeight, opacity))
		}
	}

	return b.String()
}

// renderPlanetLava generates red lava splotches on lava worlds (50% coverage).
func renderPlanetLava(cx, cy, planetRadius float64, planetID, clipID string) string {
	var b strings.Builder

	// Use planet ID as seed for consistent lava positions
	seed := simpleHash(planetID)
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x6c617661))

	// Red/orange shades for lava
	lavaColors := []string{
		"#FF4500", // Bright orange-red
		"#FF5722", // Red-orange
		"#FF6B35", // Medium orange
		"#E64A19", // Deep red-orange
		"#FF7043", // Light orange-red
	}

	// 40% coverage with lava pools
	numLavaPools := 3 + rng.IntN(3) // 3-5 pools

	for i := 0; i < numLavaPools; i++ {
		// Random position on planet
		angle := rng.Float64() * 2 * math.Pi
		distance := rng.Float64() * planetRadius * 0.7
		lavaX := cx + math.Cos(angle)*distance
		lavaY := cy + math.Sin(angle)*distance

		// Medium lava pools (smaller for 40% coverage)
		baseSize := planetRadius * (0.3 + rng.Float64()*0.4)

		// Create irregular lava shape using multiple overlapping ellipses
		numSplotches := 3 + rng.IntN(3) // 3-5 splotches per pool
		for j := 0; j < numSplotches; j++ {
			offsetX := (rng.Float64() - 0.5) * baseSize
			offsetY := (rng.Float64() - 0.5) * baseSize
			splotchX := lavaX + offsetX
			splotchY := lavaY + offsetY

			rx := baseSize * (0.5 + rng.Float64()*0.5)
			ry := baseSize * (0.3 + rng.Float64()*0.4)
			rotation := rng.Float64() * 360

			color := lavaColors[rng.IntN(len(lavaColors))]
			opacity := 0.8 + rng.Float64()*0.2

			b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="%s" opacity="%.2f" transform="rotate(%.0f,%.1f,%.1f)" clip-path="url(#%s)"/>`,
				splotchX, splotchY, rx, ry, color, opacity, rotation, splotchX, splotchY, clipID))
		}
	}

	return b.String()
}

// renderPlanetSplotches adds color variation splotches to break up planet uniformity.
func renderPlanetSplotches(cx, cy, planetRadius float64, planetID, clipID, baseColor string) string {
	var b strings.Builder

	// Define adjacent colors for each planet type
	splotchColors := map[string][]string{
		"arid":     {"#c4a576", "#b49566", "#d4b586", "#a48556"}, // Tan/brown variations
		"scorched": {"#c45a3e", "#d46a4e", "#b44a2e", "#e47a5e"}, // Red-orange variations
		"tundra":   {"#8cb87e", "#9cad8e", "#7ca86e", "#acbd9e"}, // Gray-green variations
		"glacial":  {"#c0f0ff", "#b0e0f0", "#d0ffff", "#a0d0e0"}, // Ice blue variations
		"hothouse": {"#48b858", "#58c868", "#38a848", "#68d878"}, // Bright green variations
		"ice_world": {"#b0e0ff", "#c0f0ff", "#a0d0f0", "#d0ffff"}, // Pale blue variations
	}

	colors, ok := splotchColors[planetID]
	if !ok {
		// Generate adjacent colors by adjusting lightness
		colors = []string{
			baseColor,
			adjustBrightness(baseColor, 20),
			adjustBrightness(baseColor, -20),
			adjustBrightness(baseColor, 10),
		}
	}

	// Use planet ID as seed for consistent splotch positions
	seed := simpleHash(planetID)
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x73706c6f))

	// Add 8-12 splotches depending on planet size
	numSplotches := 6 + int(planetRadius) + rng.IntN(4)

	for i := 0; i < numSplotches; i++ {
		// Random position on planet
		angle := rng.Float64() * 2 * math.Pi
		distance := rng.Float64() * planetRadius * 0.8
		splotchX := cx + math.Cos(angle)*distance
		splotchY := cy + math.Sin(angle)*distance

		// Splotch size varies
		baseSize := planetRadius * (0.15 + rng.Float64()*0.35)

		rx := baseSize * (0.6 + rng.Float64()*0.4)
		ry := baseSize * (0.4 + rng.Float64()*0.4)
		rotation := rng.Float64() * 360

		color := colors[rng.IntN(len(colors))]
		opacity := 0.4 + rng.Float64()*0.4

		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="%s" opacity="%.2f" transform="rotate(%.0f,%.1f,%.1f)" clip-path="url(#%s)"/>`,
			splotchX, splotchY, rx, ry, color, opacity, rotation, splotchX, splotchY, clipID))
	}

	return b.String()
}

// adjustBrightness lightens or darkens a hex color by a percentage.
func adjustBrightness(hex string, percent int) string {
	// Remove # if present
	hex = strings.TrimPrefix(hex, "#")

	// Parse hex to RGB
	r, err := strconv.ParseInt(hex[0:2], 16, 32)
	if err != nil {
		return hex
	}
	g, err := strconv.ParseInt(hex[2:4], 16, 32)
	if err != nil {
		return hex
	}
	b, err := strconv.ParseInt(hex[4:6], 16, 32)
	if err != nil {
		return hex
	}

	// Adjust brightness
	adjust := func(c int64) int64 {
		result := c + int64(float64(c)*float64(percent)/100.0)
		if result < 0 {
			result = 0
		}
		if result > 255 {
			result = 255
		}
		return result
	}

	r = adjust(r)
	g = adjust(g)
	b = adjust(b)

	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// simpleHash creates a simple hash from a string for seeding.
func simpleHash(s string) int32 {
	h := int32(0)
	for i := 0; i < len(s); i++ {
		h = h*31 + int32(s[i])
	}
	return h
}
