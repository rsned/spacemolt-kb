package systemmap

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
