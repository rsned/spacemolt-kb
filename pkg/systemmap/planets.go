package systemmap

import (
	"fmt"
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

	// Planet circle (size based on class)
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.2f" fill="%s"/>`, px, py, pc.Size, pc.Color))

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
