package systemmap

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
)

// AsteroidBeltStyle defines colors for different asteroid belt subtypes.
type AsteroidBeltStyle struct {
	BaseColor string
	Opacity   float64
}

var asteroidBeltStyles = map[string]AsteroidBeltStyle{
	"metallic":     {BaseColor: "#C0C0C0", Opacity: 1.0},     // silver-gray, 199 occurrences (2x brighter)
	"mixed":        {BaseColor: "#D4A574", Opacity: 0.9},     // mixed brown-gray, 26 occurrences (2x brighter)
	"carbonaceous": {BaseColor: "#8B7355", Opacity: 0.75},    // dark brown, 6 occurrences (2x brighter)
	"icy":          {BaseColor: "#A0D8EF", Opacity: 0.95},    // icy blue-white, 1 occurrence (2x brighter)
	"":             {BaseColor: "#D08770", Opacity: 0.9},     // default orange-brown (2x brighter)
}

// GasCloudStyle defines colors for different gas cloud subtypes.
type GasCloudStyle struct {
	BaseColor string
	Density   float64
	Opacity   float64
}

var gasCloudStyles = map[string]GasCloudStyle{
	"molecular_cloud": {BaseColor: "#B48EAD", Density: 1.2, Opacity: 0.6},  // purple, dense, 136 occurrences (2x brighter)
	"emission":       {BaseColor: "#FF6B6B", Density: 0.8, Opacity: 0.7},  // reddish emission, 29 occurrences (2x brighter)
	"atmospheric":    {BaseColor: "#87CEEB", Density: 0.5, Opacity: 0.5},  // blue, thin, 5 occurrences (2x brighter)
	"":               {BaseColor: "#B48EAD", Density: 1.0, Opacity: 0.6},  // default (2x brighter)
}

// IceFieldStyle defines colors for different ice field subtypes.
type IceFieldStyle struct {
	BaseColor string
	Opacity   float64
}

var iceFieldStyles = map[string]IceFieldStyle{
	"kuiper":   {BaseColor: "#E0F7FA", Opacity: 0.8},  // pale cyan, 137 occurrences (2x brighter)
	"cometary": {BaseColor: "#B3E5FC", Opacity: 0.75}, // light blue, 39 occurrences (2x brighter)
	"":         {BaseColor: "#88C0D0", Opacity: 0.75}, // default blue (2x brighter)
}

// RelicStyle defines rendering styles for different relic subtypes.
type RelicStyle struct {
	BaseColor string
	Size      float64
	Symbol    string
}

var relicStyles = map[string]RelicStyle{
	"derelict":       {BaseColor: "#D4A574", Size: 10, Symbol: "hull"},    // brown, 6 occurrences
	"megastructure":  {BaseColor: "#FFD700", Size: 15, Symbol: "ring"},     // gold, 2 occurrences
	"alien_artifact": {BaseColor: "#00FF7F", Size: 12, Symbol: "crystal"},  // spring green, 1 occurrence
	"":               {BaseColor: "#EBCB8B", Size: 10, Symbol: "compass"}, // default gold, compass rose
}

// generateAsteroidParticlesWithSubtype creates asteroid particles with subtype-specific colors.
func generateAsteroidParticlesWithSubtype(orbitCX, orbitCY, radius float64, seed uint64, subtype string) string {
	style, ok := asteroidBeltStyles[subtype]
	if !ok {
		style = asteroidBeltStyles[""]
	}

	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeef))
	var b strings.Builder
	count := 150
	for range count {
		angle := rng.Float64() * 2 * math.Pi
		jitter := 1.0 + (rng.Float64()-0.5)*0.3 // +/-15%
		r := radius * jitter
		px := orbitCX + r*math.Cos(angle)
		py := orbitCY + r*math.Sin(angle)
		size := 2.0 + rng.Float64()*3.0
		opacity := style.Opacity * (0.7 + rng.Float64()*0.3)
		rotation := rng.Float64() * 360

		// Triangle: three points centered around (px, py).
		h := size * 0.866 // sqrt(3)/2
		p1x := px - size/2
		p1y := py + h/2
		p2x := px
		p2y := py - h/2
		p3x := px + size/2
		p3y := py + h/2
		b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="%s" opacity="%.2f" transform="rotate(%.0f,%.1f,%.1f)"/>`,
			p1x, p1y, p2x, p2y, p3x, p3y, style.BaseColor, opacity, rotation, px, py))
	}
	return b.String()
}

// generateIceParticlesWithSubtype creates ice field particles with subtype-specific colors.
func generateIceParticlesWithSubtype(orbitCX, orbitCY, radius float64, seed uint64, subtype string) string {
	style, ok := iceFieldStyles[subtype]
	if !ok {
		style = iceFieldStyles[""]
	}

	rng := rand.New(rand.NewPCG(seed, seed^0xcafebabe))
	var b strings.Builder
	count := 80
	for range count {
		angle := rng.Float64() * 2 * math.Pi
		// Cap spread at +/-15px from orbit ring.
		maxJitter := 15.0
		if radius > 0 {
			maxJitter = math.Min(15.0, radius*0.15)
		}
		r := radius + (rng.Float64()-0.5)*2*maxJitter
		px := orbitCX + r*math.Cos(angle)
		py := orbitCY + r*math.Sin(angle)
		size := 2.0 + rng.Float64()*3.0
		// Use style opacity with some variation
		opacity := style.Opacity + (rng.Float64()-0.5)*0.2
		if opacity < 0.2 {
			opacity = 0.2
		}
		if opacity > 1.0 {
			opacity = 1.0
		}

		// Diamond: four points.
		b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="%s" opacity="%.2f"/>`,
			px, py-size, px+size*0.6, py, px, py+size, px-size*0.6, py, style.BaseColor, opacity))
	}
	return b.String()
}

// generateGasCloudWithSubtype renders gas clouds with subtype-specific appearance.
func generateGasCloudWithSubtype(cx, cy float64, seed uint64, subtype string) string {
	style, ok := gasCloudStyles[subtype]
	if !ok {
		style = gasCloudStyles[""]
	}

	rng := rand.New(rand.NewPCG(seed, seed^0xbeef))
	var b strings.Builder

	// At least 12 bubbles of different sizes
	count := 12 + int(style.Density*6) // 12-18 bubbles based on density

	for range count {
		// Spread bubbles across a larger area
		spread := 20.0 + style.Density*5.0
		bx := cx + (rng.Float64()-0.5)*spread
		by := cy + (rng.Float64()-0.5)*spread
		// More varied sizes: 1.5 to 6.0
		br := 1.5 + rng.Float64()*4.5
		// Use style opacity with some variation
		bo := style.Opacity + (rng.Float64()-0.5)*0.2
		if bo < 0.1 {
			bo = 0.1
		}
		if bo > 1.0 {
			bo = 1.0
		}
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="%.2f"/>`, bx, by, br, style.BaseColor, bo))
	}

	return b.String()
}

// renderRelicWithSubtype renders relics with subtype-specific appearance.
func renderRelicWithSubtype(cx, cy float64, subtype string) string {
	style, ok := relicStyles[subtype]
	if !ok {
		style = relicStyles[""]
	}

	switch style.Symbol {
	case "hull":
		// Derelict ship hull - irregular polygon
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="%s" opacity="0.8"/>`,
			cx-style.Size, cy-style.Size*0.3,
			cx-style.Size*0.5, cy+style.Size,
			cx, cy+style.Size*0.7,
			cx+style.Size*0.5, cy+style.Size,
			cx+style.Size, cy-style.Size*0.3,
			style.BaseColor)
	case "ring":
		// Megastructure - large ring with center
		var b strings.Builder
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="%s" stroke-width="2" opacity="0.9"/>`,
			cx, cy, style.Size, style.BaseColor))
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="0.7"/>`,
			cx, cy, style.Size*0.3, style.BaseColor))
		return b.String()
	case "crystal":
		// Alien artifact - crystalline shape
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="%s" opacity="0.9"/>`,
			cx, cy-style.Size,
			cx+style.Size*0.4, cy-style.Size*0.3,
			cx+style.Size, cy,
			cx+style.Size*0.4, cy+style.Size*0.3,
			cx, cy+style.Size,
			cx-style.Size*0.3, cy,
			style.BaseColor)
	default:
		// Default compass rose
		return renderCompassRose(cx, cy, style.Size)
	}
}

// renderNebula renders a nebula POI (stellar_nursery) with organic, flowing shapes.
func renderNebula(cx, cy float64, seed uint64) string {
	rng := rand.New(rand.NewPCG(seed, seed^0xbabecafe))
	var b strings.Builder

	// Nebula colors - pinks, purples, blues for stellar nursery
	nebulaColors := []string{
		"#FF6B9D", "#FFB6C1", "#FFC0CB", "#FFE4E1", // Pinks
		"#B48EAD", "#DDA0DD", "#EE82EE", // Purples
		"#87CEEB", "#ADD8E6", // Light blues
		"#FFD700", "#FFA500", // Gold and orange accents
	}

	// Create organic, flowing shapes using rotated ellipses
	// Multiple layers for depth and soft edges
	for range 35 {
		color := nebulaColors[rng.IntN(len(nebulaColors))]

		// Irregular positioning for organic feel
		angle := rng.Float64() * 2 * math.Pi
		distance := 2.0 + rng.Float64()*14.0
		nx := cx + math.Cos(angle)*distance
		ny := cy + math.Sin(angle)*distance*0.7 // Slight vertical compression

		// Irregular elliptical shapes for wispy, flowing appearance
		rx := 6.0 + rng.Float64()*15.0 // Varying horizontal radius
		ry := 3.0 + rng.Float64()*10.0 // Varying vertical radius
		rotation := rng.Float64() * 360

		// Lower opacity for soft, diffused edges like real nebulae
		opacity := 0.08 + rng.Float64()*0.20

		// Use ellipse for organic shapes
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="%s" opacity="%.2f" transform="rotate(%.0f,%.1f,%.1f)"/>`,
			nx, ny, rx, ry, color, opacity, rotation, nx, ny))
	}

	// Add some very faint, large wisps for depth
	for range 8 {
		color := nebulaColors[rng.IntN(len(nebulaColors))]
		angle := rng.Float64() * 2 * math.Pi
		distance := 8.0 + rng.Float64()*10.0
		nx := cx + math.Cos(angle)*distance
		ny := cy + math.Sin(angle)*distance*0.7

		rx := 15.0 + rng.Float64()*10.0
		ry := 8.0 + rng.Float64()*6.0
		rotation := rng.Float64() * 360
		opacity := 0.03 + rng.Float64()*0.05 // Very faint for depth

		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="%s" opacity="%.2f" transform="rotate(%.0f,%.1f,%.1f)"/>`,
			nx, ny, rx, ry, color, opacity, rotation, nx, ny))
	}

	// Add bright stars forming with 4-point pointed highlights
	starCount := 4 + rng.IntN(3) // 4-6 stars
	for range starCount {
		sx := cx + (rng.Float64()-0.5)*18
		sy := cy + (rng.Float64()-0.5)*18
		sr := 1.0 + rng.Float64()*1.5

		// Draw 4-pointed star with pointed highlights
		// Inner bright circle
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#FFFFFF" opacity="0.95"/>`,
			sx, sy, sr*0.6))

		// Four pointed rays extending outward
		rayLength := sr * 3.5
		rayWidth := sr * 0.3

		// Horizontal ray
		b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#FFFFFF" opacity="0.7"/>`,
			sx-rayLength, sy-rayWidth,
			sx-rayWidth, sy,
			sx+rayWidth, sy,
			sx+rayLength, sy-rayWidth))

		// Vertical ray
		b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#FFFFFF" opacity="0.7"/>`,
			sx-rayWidth, sy-rayLength,
			sx, sy-rayWidth,
			sx, sy+rayWidth,
			sx-rayWidth, sy+rayLength))
	}

	return b.String()
}

