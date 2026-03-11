package systemmap

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
	// White dwarf spectral types (use same spectral scale for temperature)
	"DA": {Letter: "DA", Color: "#f0f8ff", Name: "White Dwarf (H lines)", TempRange: "4,000–150,000 K"},
	"DB": {Letter: "DB", Color: "#e8eeff", Name: "White Dwarf (He I)", TempRange: "11,000–40,000 K"},
	"DC": {Letter: "DC", Color: "#e8e8e8", Name: "White Dwarf (featureless)", TempRange: "<5,000 K"},
	"DO": {Letter: "DO", Color: "#d8e8ff", Name: "White Dwarf (He II)", TempRange: "45,000–150,000 K"},
	"DQ": {Letter: "DQ", Color: "#f0e8d8", Name: "White Dwarf (carbon)", TempRange: "8,000–18,000 K"},
	"DZ": {Letter: "DZ", Color: "#e8f0f8", Name: "White Dwarf (metal)", TempRange: "5,000–15,000 K"},
	"DX": {Letter: "DX", Color: "#e8e8e8", Name: "White Dwarf (unclassifiable)", TempRange: "unknown"},
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

// spectralOrder defines the temperature sequence from hottest to coolest.
var spectralOrder = []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}

// spectralIndex maps spectral letter to its position in the temperature sequence.
var spectralIndex = map[string]int{
	"O": 0, "B": 1, "A": 2, "F": 3, "G": 4, "K": 5, "M": 6, "L": 7, "T": 8, "Y": 9,
}

// GetStarColor returns the hex color for a spectral type (OBAFGKMLTY) or white dwarf type.
// For white dwarfs with secondary features (e.g., "DAP", "DAV"), extracts the base type (e.g., "DA").
// Returns default sun color (#EBCB8B) for unknown types.
func GetStarColor(spectral string) string {
	// Check for white dwarf with secondary features (e.g., "DAP", "DAV", "DBQ")
	if len(spectral) == 3 && strings.HasPrefix(spectral, "D") {
		// Extract base white dwarf type (first 2 characters: DA, DB, DC, DO, DQ, DZ, DX)
		baseType := spectral[:2]
		if st, ok := spectralTypes[baseType]; ok {
			return st.Color
		}
	}

	if st, ok := spectralTypes[spectral]; ok {
		return st.Color
	}
	return "#EBCB8B" // Default sun color
}

// GetStarColorRefined returns the hex color for a spectral type with subtype refinement.
// The subtype (0-9) interpolates between spectral types for smooth temperature gradients.
// For example, G9 blends toward K, while G0 blends toward F.
func GetStarColorRefined(spectral string, subtype int) string {
	// If no subtype or out of range, use base color
	if subtype < 0 || subtype > 9 {
		return GetStarColor(spectral)
	}

	// For brown dwarfs (L, T, Y), don't blend - use their distinctive colors
	if spectral == "L" || spectral == "T" || spectral == "Y" {
		return GetStarColor(spectral)
	}

	// Get the base spectral color
	baseColor := GetStarColor(spectral)
	if baseColor == "#EBCB8B" { // Unknown spectral type
		return baseColor
	}

	// Subtype 5 is the "pure" spectral type - no blending needed
	if subtype == 5 {
		return baseColor
	}

	// Determine which direction to blend
	idx, exists := spectralIndex[spectral]
	if !exists {
		return baseColor
	}

	var blendColor string
	var blendFactor float64

	if subtype < 5 {
		// Blend toward hotter (previous) spectral type
		// G0 (40% toward F) to G4 (10% toward F)
		if idx == 0 {
			// O-type has nothing hotter to blend toward
			return baseColor
		}
		hotterSpectral := spectralOrder[idx-1]
		blendColor = GetStarColor(hotterSpectral)
		blendFactor = 1.0 - (float64(subtype) / 5.0) // 0→1.0, 4→0.2
	} else {
		// Blend toward cooler (next) spectral type
		// G6 (10% toward K) to G9 (40% toward K)
		if idx == len(spectralOrder)-1 {
			// Y-type has nothing cooler to blend toward
			return baseColor
		}
		coolerSpectral := spectralOrder[idx+1]
		blendColor = GetStarColor(coolerSpectral)
		blendFactor = (float64(subtype) - 5.0) / 5.0 // 6→0.2, 9→1.0
	}

	return blendHexColors(baseColor, blendColor, blendFactor)
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
//   - Without subtype (defaults to 5): "G V"
//   - White dwarf types: "DA", "DB", "DC", "DO", "DQ", "DZ", "DX", "DAP", "DAV", etc.
// Returns spectral type (OBAFGKMLTY or white dwarf type), subtype (0-9, -1 if unknown), luminosity class, and error.
func ParseStarClass(class string) (string, int, string, error) {
	class = strings.TrimSpace(class)
	if class == "" {
		return "", -1, "", errors.New("empty class string")
	}

	// Check for white dwarf classification first (starts with D followed by spectral type)
	// White dwarfs: DA, DB, DC, DO, DQ, DZ, DX, with optional secondary features (P, H, E, V)
	reWD := regexp.MustCompile(`^(D[ABCQOXZ])([PHEV])?$`)
	matchesWD := reWD.FindStringSubmatch(class)
	if len(matchesWD) > 0 {
		// White dwarf - use the specific type (e.g., "DA") as the spectral type
		wdType := matchesWD[1]
		secondary := matchesWD[2]
		if secondary != "" {
			wdType = wdType + secondary // e.g., "DA" + "P" = "DAP"
		}
		return wdType, -1, "VII", nil // White dwarfs are always luminosity class VII
	}

	// Try splitting on space first
	parts := strings.Fields(class)
	if len(parts) == 2 {
		spectral, subtype := parseSpectralWithSubtype(parts[0])
		luminosity := parts[1]
		if spectral == "" || !isValidLuminosity(luminosity) {
			return "", -1, "", errors.New("invalid class format")
		}
		return spectral, subtype, luminosity, nil
	}

	// Try compact format (e.g., "G2V", "B3Ia", "L5V")
	re := regexp.MustCompile(`^([OBAFGKMLTY])([0-9]?)((?:Ia|Ib|I{1,3}|IV|V|VI|VII))?$`)
	matches := re.FindStringSubmatch(class)
	if len(matches) > 0 {
		spectral := matches[1]
		subtype := parseSubtype(matches[2])
		luminosity := matches[3]
		if luminosity == "" {
			luminosity = "V" // Default to main sequence
		}
		return spectral, subtype, luminosity, nil
	}

	// Try just spectral type with number (e.g., "G2", "M9", "L5")
	re2 := regexp.MustCompile(`^([OBAFGKMLTY])([0-9]?)$`)
	matches2 := re2.FindStringSubmatch(class)
	if len(matches2) > 0 {
		spectral := matches2[1]
		subtype := parseSubtype(matches2[2])
		return spectral, subtype, "V", nil
	}

	return "", -1, "", errors.New("invalid class format")
}

// parseSpectralWithSubtype extracts both the spectral letter and subtype number.
func parseSpectralWithSubtype(s string) (string, int) {
	re := regexp.MustCompile(`^([OBAFGKMLTY])([0-9]?)$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) < 2 {
		return "", -1
	}
	spectral := matches[1]
	subtype := parseSubtype(matches[2])
	return spectral, subtype
}

// parseSubtype converts a string subtype to int, defaulting to 5 (middle of range).
func parseSubtype(s string) int {
	if s == "" {
		return 5 // Default to subtype 5 if not specified
	}
	val, err := strconv.Atoi(s)
	if err != nil || val < 0 || val > 9 {
		return 5 // Invalid subtype defaults to 5
	}
	return val
}

// isValidLuminosity checks if a string is a valid Yerkes luminosity class.
func isValidLuminosity(lum string) bool {
	valid := map[string]bool{
		"Ia": true, "Ib": true, "II": true, "III": true,
		"IV": true, "V": true, "VI": true, "VII": true,
	}
	return valid[lum]
}

// blendHexColors blends two hex colors by a given factor (0.0 to 1.0).
// Factor 0.0 returns color1, factor 1.0 returns color2, intermediate values blend.
func blendHexColors(color1, color2 string, factor float64) string {
	// Parse hex colors
	r1, g1, b1 := parseHexColor(color1)
	r2, g2, b2 := parseHexColor(color2)

	// Blend
	r := r1 + (r2-r1)*factor
	g := g1 + (g2-g1)*factor
	b := b1 + (b2-b1)*factor

	return fmt.Sprintf("#%02x%02x%02x", uint8(r), uint8(g), uint8(b))
}

// parseHexColor parses a hex color string (#RRGGBB) into RGB components.
func parseHexColor(hex string) (r, g, b float64) {
	// Remove # if present
	hex = strings.TrimPrefix(hex, "#")

	// Parse as integer
	val, err := strconv.ParseInt(hex, 16, 32)
	if err != nil {
		return 235, 203, 139 // Default to #EBCB8B
	}

	r = float64((val>>16)&0xFF) / 255.0
	g = float64((val>>8)&0xFF) / 255.0
	b = float64(val&0xFF) / 255.0

	// Convert to 0-255 range
	return r * 255, g * 255, b * 255
}

// renderStar generates SVG for a star POI with color and size based on classification.
func renderStar(poi POI, cx, cy float64) string {
	var b strings.Builder

	// Parse classification
	spectral, subtype, luminosity, err := ParseStarClass(poi.Class)
	var color string
	var size float64

	if err != nil {
		// Malformed class, use default rendering
		color = "#EBCB8B"
		size = 10
	} else {
		// Check if this is a white dwarf (starts with "D")
		isWhiteDwarf := strings.HasPrefix(spectral, "D")

		if isWhiteDwarf {
			// White dwarfs use their specific color and fixed size
			color = GetStarColor(spectral) // Use base color, no subtype refinement for white dwarfs
			size = 6 // White dwarfs are always luminosity VII size
		} else {
			color = GetStarColorRefined(spectral, subtype)
			size = GetStarSize(luminosity)

			// Brown dwarfs (L/T/Y) are always small, regardless of luminosity
			if spectral == "L" || spectral == "T" || spectral == "Y" {
				size = 8
			}
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
