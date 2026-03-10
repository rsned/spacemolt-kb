package systemmap

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// SpectralType holds data for a Harvard spectral class (OBAFGKMLTY).
type SpectralType struct {
	Letter    string
	Color     string // Hex color for SVG rendering
	Name      string // Human-readable color name
	TempRange string
}

// spectralTypes maps spectral letters to their properties.
var spectralTypes = map[string]SpectralType{
	"O": {Letter: "O", Color: "#a0c8ff", Name: "Blue", TempRange: ">30,000 K"},
	"B": {Letter: "B", Color: "#c8d8ff", Name: "Blue-White", TempRange: "10,000–30,000 K"},
	"A": {Letter: "A", Color: "#e8eeff", Name: "White", TempRange: "7,500–10,000 K"},
	"F": {Letter: "F", Color: "#fffff0", Name: "Yellow-White", TempRange: "6,000–7,500 K"},
	"G": {Letter: "G", Color: "#fff4a0", Name: "Yellow", TempRange: "5,200–6,000 K"},
	"K": {Letter: "K", Color: "#ffcf6e", Name: "Orange", TempRange: "3,700–5,200 K"},
	"M": {Letter: "M", Color: "#ff8060", Name: "Red", TempRange: "2,400–3,700 K"},
	"L": {Letter: "L", Color: "#cc4020", Name: "Dark Red", TempRange: "1,300–2,400 K"},
	"T": {Letter: "T", Color: "#882010", Name: "Infrared", TempRange: "700–1,300 K"},
	"Y": {Letter: "Y", Color: "#440a05", Name: "Near-IR", TempRange: "<700 K"},
}

// LuminosityClass holds data for a Yerkes luminosity class (Roman numerals).
type LuminosityClass struct {
	Roman      string  // Roman numeral (Ia, Ib, I, II, III, IV, V, VI, VII)
	Name       string  // Human-readable name
	Multiplier float64 // Size multiplier relative to baseline (V = 1.0)
	Size       float64 // Actual pixel radius for rendering
}

// luminosityClasses maps Roman numerals to their properties.
// Sizes are logarithmic: Ia=28px (2.8×), V=10px (1.0× baseline).
var luminosityClasses = map[string]LuminosityClass{
	"Ia":  {Roman: "Ia", Name: "Hypergiant", Multiplier: 2.8, Size: 28},
	"Ib":  {Roman: "Ib", Name: "Supergiant", Multiplier: 2.4, Size: 24},
	"II":  {Roman: "II", Name: "Bright Giant", Multiplier: 2.0, Size: 20},
	"III": {Roman: "III", Name: "Giant", Multiplier: 1.6, Size: 16},
	"IV":  {Roman: "IV", Name: "Subgiant", Multiplier: 1.4, Size: 14},
	"V":   {Roman: "V", Name: "Main Sequence", Multiplier: 1.0, Size: 10},
	"VI":  {Roman: "VI", Name: "Subdwarf", Multiplier: 0.8, Size: 8},
	"VII": {Roman: "VII", Name: "White Dwarf", Multiplier: 0.6, Size: 6},
}

// GetStarColor returns the hex color for a spectral type (OBAFGKMLTY).
// Returns default sun color (#EBCB8B) for unknown types.
func GetStarColor(spectral string) string {
	if st, ok := spectralTypes[spectral]; ok {
		return st.Color
	}
	return "#EBCB8B" // Default sun color
}

// GetStarSize returns the pixel radius for a luminosity class (Roman numerals).
// Returns default main sequence size (10) for unknown classes.
func GetStarSize(luminosity string) float64 {
	if lc, ok := luminosityClasses[luminosity]; ok {
		return lc.Size
	}
	return 10 // Default main sequence size
}

// ParseStarClass parses a star classification string in MK system notation.
// Supports formats:
//   - With space: "G2 V"
//   - Without space: "G2V"
//   - Compact luminosity: "B3Ia"
//   - Without luminosity (defaults to V): "M9"
// Returns spectral type (OBAFGKM), luminosity class, and error.
func ParseStarClass(class string) (string, string, error) {
	class = strings.TrimSpace(class)
	if class == "" {
		return "", "", errors.New("empty class string")
	}

	// Try splitting on space first
	parts := strings.Fields(class)
	if len(parts) == 2 {
		spectral := parseSpectralLetter(parts[0])
		luminosity := parts[1]
		if spectral == "" || !isValidLuminosity(luminosity) {
			return "", "", errors.New("invalid class format")
		}
		return spectral, luminosity, nil
	}

	// Try compact format (e.g., "G2V", "B3Ia")
	re := regexp.MustCompile(`^([OBAFGKM])([0-9]?)((?:Ia|Ib|I{1,3}|IV|V|VI|VII))?$`)
	matches := re.FindStringSubmatch(class)
	if len(matches) > 0 {
		spectral := matches[1]
		luminosity := matches[3]
		if luminosity == "" {
			luminosity = "V" // Default to main sequence
		}
		return spectral, luminosity, nil
	}

	// Try just spectral type with number (e.g., "G2", "M9")
	re2 := regexp.MustCompile(`^([OBAFGKM])([0-9]?)$`)
	matches2 := re2.FindStringSubmatch(class)
	if len(matches2) > 0 {
		return matches2[1], "V", nil
	}

	return "", "", errors.New("invalid class format")
}

// parseSpectralLetter extracts the spectral type letter from a string.
func parseSpectralLetter(s string) string {
	re := regexp.MustCompile(`^([OBAFGKM])`)
	matches := re.FindStringSubmatch(s)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// isValidLuminosity checks if a string is a valid Yerkes luminosity class.
func isValidLuminosity(lum string) bool {
	valid := map[string]bool{
		"Ia": true, "Ib": true, "II": true, "III": true,
		"IV": true, "V": true, "VI": true, "VII": true,
	}
	return valid[lum]
}

// renderStar generates SVG for a star POI with color and size based on classification.
func renderStar(poi POI, cx, cy float64) string {
	var b strings.Builder

	// Parse classification
	spectral, luminosity, err := ParseStarClass(poi.Class)
	var color string
	var size float64

	if err != nil {
		// Malformed class, use default rendering
		color = "#EBCB8B"
		size = 10
	} else {
		color = GetStarColor(spectral)
		size = GetStarSize(luminosity)

		// Brown dwarfs (L/T/Y) are always small, regardless of luminosity
		if spectral == "L" || spectral == "T" || spectral == "Y" {
			size = 8
		}
	}

	// Create gradient ID unique to this POI
	gradientID := fmt.Sprintf("star-glow-%s", poi.ID)

	b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, poiTitle(poi)))

	// Selection circle (dashed)
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, cx, cy, size+4))

	// Outer glow gradient
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="url(#%s)"/>`, cx, cy, size*2.4, gradientID))

	// Core star
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, cx, cy, size, color))

	b.WriteString(`</g>`)

	// Label with classification
	labelText := poi.Class
	b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" class="map-label">%s</text>`, cx, cy+size+8, htmlEscape(labelText)))

	return b.String()
}

// renderStarGlowGradient generates an SVG radial gradient definition for star glow.
func renderStarGlowGradient(poi POI, color string, isBrownDwarf bool) string {
	opacity := 0.45
	if isBrownDwarf {
		opacity = 0.15 // Muted glow for brown dwarfs
	}

	return fmt.Sprintf(`<radialGradient id="star-glow-%s">
<stop offset="0%%" stop-color="%s" stop-opacity="1"/>
<stop offset="40%%" stop-color="%s" stop-opacity="%.2f"/>
<stop offset="100%%" stop-color="%s" stop-opacity="0"/>
</radialGradient>`, poi.ID, color, color, opacity, color)
}
