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
	HasRings bool
}

// planetClasses maps planet class identifiers to their properties.
var planetClasses = map[string]PlanetClass{
	"terran":    {ID: "terran", Name: "Terran", Color: "#4a9c6d", HasRings: false},
	"jovian":    {ID: "jovian", Name: "Jovian Gas Giant", Color: "#e8a86c", HasRings: true},
	"ice_giant": {ID: "ice_giant", Name: "Ice Giant", Color: "#9fc5e8", HasRings: false},
	"gas_dwarf": {ID: "gas_dwarf", Name: "Gas Dwarf", Color: "#d4b596", HasRings: false},
	"rocky":     {ID: "rocky", Name: "Rocky", Color: "#8b7355", HasRings: false},
	"dwarf":     {ID: "dwarf", Name: "Dwarf", Color: "#c4a484", HasRings: false},
	"proto":     {ID: "proto", Name: "Protoplanet", Color: "#cc6666", HasRings: false},
	"captured":  {ID: "captured", Name: "Captured Body", Color: "#b0a0c0", HasRings: false},
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

// renderPlanet generates SVG for a planet POI with color based on class.
func renderPlanet(poi POI, px, py float64) string {
	var b strings.Builder

	pc := GetPlanetClass(poi.Class)

	b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, poiTitle(poi)))

	// Selection circle
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))

	// Render rings for jovian planets first (behind)
	if pc.HasRings {
		b.WriteString(renderJovianRings(px, py, 5))
	}

	// Planet circle
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="5" fill="%s"/>`, px, py, pc.Color))

	b.WriteString(`</g>`)

	// Label
	labelAbove := poi.PositionY >= 0
	if labelAbove {
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" class="map-label">%s</text>`, px, py-8, htmlEscape(poi.Name)))
	} else {
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" class="map-label">%s</text>`, px, py+16, htmlEscape(poi.Name)))
	}

	return b.String()
}

// renderJovianRings generates SVG ellipses for Saturn-style rings around a planet.
func renderJovianRings(cx, cy, planetRadius float64) string {
	var b strings.Builder

	rx := planetRadius * 2.5
	ry := planetRadius * 0.6
	transform := fmt.Sprintf("transform=\"rotate(-30, %.1f, %.1f)\"", cx, cy)

	b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#d4a86c" stroke-width="1.5" opacity="0.5" %s/>`, cx, cy, rx, ry, transform))
	b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#e8b87c" stroke-width="1" opacity="0.4" %s/>`, cx, cy, rx*0.85, ry*0.85, transform))
	b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#f0c88c" stroke-width="0.7" opacity="0.3" %s/>`, cx, cy, rx*0.7, ry*0.7, transform))

	return b.String()
}
