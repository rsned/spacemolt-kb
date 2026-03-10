# MK Star Classification Rendering Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add visual rendering of MK star classifications and planet classes to knowledge base system maps, with spectral-type colors, luminosity-based sizes, and class-specific planet styling.

**Architecture:** Create two new packages (`stars.go` and `planets.go`) in `pkg/systemmap/` for classification lookup and rendering logic. Integrate into existing `RenderSystemMap()` function. Add `Class` field to POI types across the codebase. Follow TDD with test files for each new component.

**Tech Stack:** Go 1.24, SVG generation, SQLite database, existing knowledge base codebase

---

## File Structure

**New files:**
- `pkg/systemmap/stars.go` - Star classification lookup and rendering
- `pkg/systemmap/stars_test.go` - Unit tests for star functions
- `pkg/systemmap/planets.go` - Planet class lookup and rendering
- `pkg/systemmap/planets_test.go` - Unit tests for planet functions

**Modified files:**
- `pkg/systemmap/systemmap.go` - Integrate new rendering, add Class to POI struct
- `pkg/game/types.go` - Add Class field to POI struct
- `pkg/game/serverapi/types.go` - Add Class field to POI struct
- `pkg/knowledge/sqlite.go` - Add class column, update queries
- `pkg/knowledge/sqlite_migrations.go` - Add migration for class column

---

## Chunk 1: Data Model Updates

### Task 1: Add Class field to game client POI type

**Files:**
- Modify: `pkg/game/types.go:168-181`

- [ ] **Step 1: Add Class field to POI struct**

Locate the POI struct in `pkg/game/types.go` (around line 168). Add the Class field after the Type field:

```go
// POI represents a Point of Interest in a star system.
// Embedded in SystemData.POIs array in server responses.
type POI struct {
	ID          string        `json:"id"`
	SystemID    string        `json:"system_id"`
	Type        string        `json:"type"`
	Class       string        `json:"class,omitempty"` // MK classification (e.g., "G2 V") or planet class
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Position    Position      `json:"position"`
	Resources   []POIResource `json:"resources"`
	BaseID      string        `json:"base_id,omitempty"`
	HasBase     bool          `json:"has_base,omitempty"`
	BaseName    string        `json:"base_name,omitempty"`
	Online      int           `json:"online,omitempty"`
	Hidden      bool          `json:"hidden,omitempty"`
}
```

- [ ] **Step 2: Run build to verify**

Run: `go build ./pkg/game/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/game/types.go
git commit -m "feat(game): add Class field to POI struct for MK classification"
```

### Task 2: Add Class field to serverapi POI type

**Files:**
- Modify: `pkg/game/serverapi/types.go:164-177`

- [ ] **Step 1: Add Class field to serverapi POI struct**

Locate the POI struct in `pkg/game/serverapi/types.go` (around line 164). Add the Class field after the Type field:

```go
// POI represents a Point of Interest in a star system.
type POI struct {
	ID          string        `json:"id"`
	SystemID    string        `json:"system_id"`
	Type        string        `json:"type"`
	Class       string        `json:"class,omitempty"` // MK classification (e.g., "G2 V") or planet class
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Position    Position      `json:"position"`
	Resources   []POIResource `json:"resources"`
	BaseID      string        `json:"base_id,omitempty"`
	HasBase     bool          `json:"has_base,omitempty"`
	BaseName    string        `json:"base_name,omitempty"`
	Online      int           `json:"online,omitempty"`
	Hidden      bool          `json:"hidden,omitempty"`
}
```

- [ ] **Step 2: Run build to verify**

Run: `go build ./pkg/game/serverapi/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/game/serverapi/types.go
git commit -m "feat(serverapi): add Class field to POI struct for MK classification"
```

### Task 3: Add Class field to systemmap POI type

**Files:**
- Modify: `pkg/systemmap/systemmap.go:30-37`

- [ ] **Step 1: Add Class field to systemmap POI struct**

Locate the POI struct in `pkg/systemmap/systemmap.go` (around line 30). Add the Class field after the Type field:

```go
// POI is a point of interest within a system.
type POI struct {
	ID          string
	Name        string
	Type        string
	Class       string // MK classification (e.g., "G2 V") or planet class
	Description string
	PositionX   float64
	PositionY   float64
}
```

- [ ] **Step 2: Run build to verify**

Run: `go build ./pkg/systemmap/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/systemmap/systemmap.go
git commit -m "feat(systemmap): add Class field to POI struct"
```

---

## Chunk 2: Star Classification Package

### Task 4: Create star classification types and lookup tables

**Files:**
- Create: `pkg/systemmap/stars.go`

- [ ] **Step 1: Write package declaration and spectral types**

Create `pkg/systemmap/stars.go` with spectral type definitions:

```go
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
```

- [ ] **Step 2: Add luminosity class types**

Continue in `pkg/systemmap/stars.go`:

```go
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
```

- [ ] **Step 3: Run build to verify**

Run: `go build ./pkg/systemmap/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add pkg/systemmap/stars.go
git commit -m "feat(systemmap): add spectral and luminosity type definitions"
```

### Task 5: Implement GetStarColor lookup function

**Files:**
- Modify: `pkg/systemmap/stars.go`

- [ ] **Step 1: Write failing test for GetStarColor**

Create `pkg/systemmap/stars_test.go`:

```go
package systemmap

import "testing"

func TestGetStarColor(t *testing.T) {
	tests := []struct {
		name     string
		spectral string
		want     string
	}{
		{"O type blue", "O", "#a0c8ff"},
		{"G type yellow", "G", "#fff4a0"},
		{"M type red", "M", "#ff8060"},
		{"unknown type defaults to sun color", "X", "#EBCB8B"},
		{"empty string defaults to sun color", "", "#EBCB8B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStarColor(tt.spectral); got != tt.want {
				t.Errorf("GetStarColor(%q) = %q, want %q", tt.spectral, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/systemmap/ -run TestGetStarColor -v`
Expected: FAIL with "undefined: GetStarColor"

- [ ] **Step 3: Implement GetStarColor**

Add to `pkg/systemmap/stars.go`:

```go
// GetStarColor returns the hex color for a spectral type.
// Defaults to the current sun color (#EBCB8B) for unknown types.
func GetStarColor(spectral string) string {
	if st, ok := spectralTypes[spectral]; ok {
		return st.Color
	}
	return "#EBCB8B" // Default sun color from existing rendering
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/systemmap/ -run TestGetStarColor -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/systemmap/stars.go pkg/systemmap/stars_test.go
git commit -m "feat(systemmap): add GetStarColor lookup function"
```

### Task 6: Implement GetStarSize lookup function

**Files:**
- Modify: `pkg/systemmap/stars.go`
- Modify: `pkg/systemmap/stars_test.go`

- [ ] **Step 1: Write failing test for GetStarSize**

Add to `pkg/systemmap/stars_test.go`:

```go
func TestGetStarSize(t *testing.T) {
	tests := []struct {
		name       string
		luminosity string
		want       float64
	}{
		{"Ia hypergiant", "Ia", 28},
		{"V main sequence", "V", 10},
		{"VII white dwarf", "VII", 6},
		{"unknown defaults to main sequence", "X", 10},
		{"empty defaults to main sequence", "", 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStarSize(tt.luminosity); got != tt.want {
				t.Errorf("GetStarSize(%q) = %v, want %v", tt.luminosity, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/systemmap/ -run TestGetStarSize -v`
Expected: FAIL with "undefined: GetStarSize"

- [ ] **Step 3: Implement GetStarSize**

Add to `pkg/systemmap/stars.go`:

```go
// GetStarSize returns the pixel radius for a luminosity class.
// Defaults to 10px (main sequence baseline) for unknown classes.
func GetStarSize(luminosity string) float64 {
	if lc, ok := luminosityClasses[luminosity]; ok {
		return lc.Size
	}
	return 10 // Default main sequence size
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/systemmap/ -run TestGetStarSize -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/systemmap/stars.go pkg/systemmap/stars_test.go
git commit -m "feat(systemmap): add GetStarSize lookup function"
```

### Task 7: Implement ParseStarClass function

**Files:**
- Modify: `pkg/systemmap/stars.go`
- Modify: `pkg/systemmap/stars_test.go`

- [ ] **Step 1: Write failing test for ParseStarClass**

Add to `pkg/systemmap/stars_test.go`:

```go
func TestParseStarClass(t *testing.T) {
	tests := []struct {
		name           string
		class          string
		wantSpectral   string
		wantLuminosity string
		wantErr        bool
	}{
		{"G2 V with space", "G2 V", "G", "V", false},
		{"G2V without space", "G2V", "G", "V", false},
		{"B3Ia compact", "B3Ia", "B", "Ia", false},
		{"M9 main sequence", "M9V", "M", "V", false},
		{"M9 without luminosity defaults to V", "M9", "M", "V", false},
		{"G without number", "G V", "G", "V", false},
		{"V only invalid (no spectral)", "V", "", "", true},
		{"empty string", "", "", "", true},
		{"invalid spectral", "X9 V", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpectral, gotLuminosity, err := ParseStarClass(tt.class)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseStarClass(%q) error = %v, wantErr %v", tt.class, err, tt.wantErr)
				return
			}
			if gotSpectral != tt.wantSpectral {
				t.Errorf("ParseStarClass(%q) spectral = %q, want %q", tt.class, gotSpectral, tt.wantSpectral)
			}
			if gotLuminosity != tt.wantLuminosity {
				t.Errorf("ParseStarClass(%q) luminosity = %q, want %q", tt.class, gotLuminosity, tt.wantLuminosity)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/systemmap/ -run TestParseStarClass -v`
Expected: FAIL with "undefined: ParseStarClass"

- [ ] **Step 3: Implement ParseStarClass**

Add to `pkg/systemmap/stars.go`:

```go
import (
	"errors"
	"regexp"
	"strings"
)

// ParseStarClass parses a star classification string like "G2 V" or "G2V".
// Returns (spectral letter, luminosity roman numeral, error).
// If luminosity is omitted, defaults to "V" (main sequence).
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

// parseSpectralLetter extracts the spectral letter from a string like "G2".
func parseSpectralLetter(s string) string {
	re := regexp.MustCompile(`^([OBAFGKM])`)
	matches := re.FindStringSubmatch(s)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// isValidLuminosity checks if a Roman numeral is a valid luminosity class.
func isValidLuminosity(lum string) bool {
	valid := map[string]bool{
		"Ia": true, "Ib": true, "II": true, "III": true,
		"IV": true, "V": true, "VI": true, "VII": true,
	}
	return valid[lum]
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/systemmap/ -run TestParseStarClass -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/systemmap/stars.go pkg/systemmap/stars_test.go
git commit -m "feat(systemmap): add ParseStarClass function with space/compact format support"
```

### Task 8: Implement star rendering function

**Files:**
- Modify: `pkg/systemmap/stars.go`

- [ ] **Step 1: Implement renderStar function**

Add to `pkg/systemmap/stars.go`:

```go
import (
	"fmt"
	"strings"
)

// renderStar generates SVG for a star with MK classification.
// Returns SVG string with colored circle, glow gradient, and label.
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

	// Label (with classification if available)
	labelText := poi.Name
	if spectral != "" && luminosity != "" {
		labelText = fmt.Sprintf("%s %s%s", spectral, strings.TrimPrefix(poi.Class, spectral), luminosity)
	}
	b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" class="map-label">%s</text>`, cx, cy+size+8, htmlEscape(labelText)))

	return b.String()
}

// renderStarGlowGradient generates a radial gradient definition for a star.
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
```

- [ ] **Step 2: Run build to verify**

Run: `go build ./pkg/systemmap/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/systemmap/stars.go
git commit -m "feat(systemmap): add renderStar function for classified stars"
```

---

## Chunk 3: Planet Classification Package

### Task 9: Create planet class types and lookup

**Files:**
- Create: `pkg/systemmap/planets.go`
- Create: `pkg/systemmap/planets_test.go`

- [ ] **Step 1: Write planet class definitions**

Create `pkg/systemmap/planets.go`:

```go
package systemmap

// PlanetClass holds data for a planet classification.
type PlanetClass struct {
	ID       string // Class identifier (terran, jovian, etc.)
	Name     string // Human-readable name
	Color    string // Hex color for SVG rendering
	HasRings bool   // Whether to render ring system
}

// planetClasses maps class identifiers to their properties.
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
```

- [ ] **Step 2: Write failing test for GetPlanetClass**

Create `pkg/systemmap/planets_test.go`:

```go
package systemmap

import "testing"

func TestGetPlanetClass(t *testing.T) {
	tests := []struct {
		name      string
		class     string
		wantColor string
		wantName  string
	}{
		{"terran class", "terran", "#4a9c6d", "Terran"},
		{"jovian class", "jovian", "#e8a86c", "Jovian Gas Giant"},
		{"unknown class defaults", "unknown", "#A3BE8C", "Planet"},
		{"empty defaults", "", "#A3BE8C", "Planet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := GetPlanetClass(tt.class)
			if pc.Color != tt.wantColor {
				t.Errorf("GetPlanetClass(%q).Color = %q, want %q", tt.class, pc.Color, tt.wantColor)
			}
			if pc.Name != tt.wantName {
				t.Errorf("GetPlanetClass(%q).Name = %q, want %q", tt.class, pc.Name, tt.wantName)
			}
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./pkg/systemmap/ -run TestGetPlanetClass -v`
Expected: FAIL with "undefined: GetPlanetClass"

- [ ] **Step 4: Implement GetPlanetClass**

Add to `pkg/systemmap/planets.go`:

```go
// GetPlanetClass returns the PlanetClass for a class string.
// Defaults to generic planet styling for unknown classes.
func GetPlanetClass(class string) PlanetClass {
	if pc, ok := planetClasses[class]; ok {
		return pc
	}
	return PlanetClass{
		ID:       "unknown",
		Name:     "Planet",
		Color:    "#A3BE8C", // Current default planet color
		HasRings: false,
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./pkg/systemmap/ -run TestGetPlanetClass -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/systemmap/planets.go pkg/systemmap/planets_test.go
git commit -m "feat(systemmap): add planet class definitions and lookup"
```

### Task 10: Implement planet rendering function

**Files:**
- Modify: `pkg/systemmap/planets.go`

- [ ] **Step 1: Implement renderPlanet function**

Add to `pkg/systemmap/planets.go`:

```go
import (
	"fmt"
	"strings"
)

// renderPlanet generates SVG for a planet with class-based styling.
// Returns SVG string with colored circle, optional rings, and label.
func renderPlanet(poi POI, px, py float64) string {
	var b strings.Builder

	pc := GetPlanetClass(poi.Class)

	b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, poiTitle(poi)))

	// Selection circle (dashed)
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))

	// Render rings for jovian planets first (behind the planet)
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

// renderJovianRings generates an elliptical ring system for gas giants.
func renderJovianRings(cx, cy, planetRadius float64) string {
	var b strings.Builder

	// Ring dimensions
	rx := planetRadius * 2.5
	ry := planetRadius * 0.6

	// Transform for 30° tilt
	// SVG doesn't have direct ellipse rotation, so we use transform
	transform := fmt.Sprintf("transform=\"rotate(-30, %.1f, %.1f)\"", cx, cy)

	// Outer ring
	b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#d4a86c" stroke-width="1.5" opacity="0.5" %s/>`, cx, cy, rx, ry, transform))

	// Middle ring
	b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#e8b87c" stroke-width="1" opacity="0.4" %s/>`, cx, cy, rx*0.85, ry*0.85, transform))

	// Inner ring
	b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="#f0c88c" stroke-width="0.7" opacity="0.3" %s/>`, cx, cy, rx*0.7, ry*0.7, transform))

	return b.String()
}
```

- [ ] **Step 2: Run build to verify**

Run: `go build ./pkg/systemmap/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/systemmap/planets.go
git commit -m "feat(systemmap): add renderPlanet function with class-based styling and jovian rings"
```

---

## Chunk 4: Integration and Testing

### Task 11: Integrate star rendering into RenderSystemMap

**Files:**
- Modify: `pkg/systemmap/systemmap.go`

- [ ] **Step 1: Add gradient definitions to RenderSystemMap**

Locate the `<defs>` section in the explored case of `RenderSystemMap` (around line 192). Add star glow gradient generation:

Find the existing `<defs>` block and add star gradients dynamically:

```go
		// Sun gradient definition.
		b.WriteString(`<defs>`)
		if standalone {
			b.WriteString(`<style>`)
			b.WriteString(`.map-label { font: 11px sans-serif; fill: #d8dee9; }`)
			b.WriteString(`.gate-label { font: 10px sans-serif; fill: #81a1c1; }`)
			b.WriteString(`</style>`)
		}
		b.WriteString(`<radialGradient id="sun-glow">`)
		b.WriteString(`<stop offset="0%" stop-color="#EBCB8B" stop-opacity="1"/>`)
		b.WriteString(`<stop offset="40%" stop-color="#D08770" stop-opacity="0.45"/>`)
		b.WriteString(`<stop offset="100%" stop-color="#D08770" stop-opacity="0"/>`)
		b.WriteString(`</radialGradient>`)

		// Add star-specific gradients for classified stars
		for _, poi := range sys.POIs {
			if poi.Type == "sun" && poi.Class != "" {
				spectral, _, err := ParseStarClass(poi.Class)
				if err == nil {
					color := GetStarColor(spectral)
					isBrownDwarf := (spectral == "L" || spectral == "T" || spectral == "Y")
					b.WriteString(renderStarGlowGradient(poi, color, isBrownDwarf))
				}
			}
		}
```

- [ ] **Step 2: Update sun rendering to use renderStar for classified stars**

Find the sun case in the POI rendering switch (around line 237). Replace the existing sun rendering:

```go
			case "sun":
				if poi.Class != "" {
					// Use classified star rendering
					b.WriteString(renderStar(poi, cx, cy))
				} else {
					// Use default sun rendering
					b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, cx, cy))
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="24" fill="url(#sun-glow)"/>`, cx, cy))
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="10" fill="#EBCB8B"/>`, cx, cy))
					b.WriteString(`</g>`)
					labels = append(labels, labelInfo{x: cx, y: cy + 28, name: poi.Name, above: false})
				}
```

- [ ] **Step 3: Run build to verify**

Run: `go build ./pkg/systemmap/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add pkg/systemmap/systemmap.go
git commit -m "feat(systemmap): integrate classified star rendering"
```

### Task 12: Integrate planet rendering into RenderSystemMap

**Files:**
- Modify: `pkg/systemmap/systemmap.go`

- [ ] **Step 1: Update planet rendering to use renderPlanet for classified planets**

Find the planet case in the POI rendering switch (around line 246). Replace the existing planet rendering:

```go
			case "planet":
				if poi.Class != "" {
					// Use classified planet rendering
					b.WriteString(renderPlanet(poi, px, py))
					// Label is handled by renderPlanet
				} else {
					// Use default planet rendering
					b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="5" fill="#A3BE8C"/>`, px, py))
					b.WriteString(`</g>`)
					if labelAbove {
						labels = append(labels, labelInfo{x: px, y: py - 8, name: poi.Name, above: true})
					} else {
						labels = append(labels, labelInfo{x: px, y: py + 16, name: poi.Name, above: false})
					}
				}
```

- [ ] **Step 2: Run build to verify**

Run: `go build ./pkg/systemmap/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/systemmap/systemmap.go
git commit -m "feat(systemmap): integrate classified planet rendering"
```

### Task 13: Add conflict detection logging

**Files:**
- Modify: `pkg/systemmap/systemmap.go`

- [ ] **Step 1: Add conflict detection before POI rendering**

In `RenderSystemMap`, after computing scale and before rendering POIs, add conflict detection:

Find the explored check and add after scale computation (around line 80):

```go
	if explored {
		// Check for star-orbit conflicts
		for _, poi := range sys.POIs {
			if poi.Type == "sun" && poi.Class != "" {
				_, luminosity, err := ParseStarClass(poi.Class)
				if err != nil {
					continue
				}
				starRadius := GetStarSize(luminosity)

				// Find nearest orbit
				var minOrbitR float64 = -1
				for _, otherPoi := range sys.POIs {
					if otherPoi.Type != "sun" {
						r := math.Hypot(otherPoi.PositionX, otherPoi.PositionY) * scale
						if minOrbitR < 0 || r < minOrbitR {
							minOrbitR = r
						}
					}
				}

				// Check if star overlaps orbit (with 5px margin)
				if minOrbitR > 0 && starRadius+5 > minOrbitR-20 {
					fmt.Printf("WARNING: System %s: Star radius %.0fpx overlaps nearest orbit at %.0fpx (class: %s)\n",
						sys.ID, starRadius, minOrbitR-20, poi.Class)
				}
			}
		}
	}
```

- [ ] **Step 2: Run build to verify**

Run: `go build ./pkg/systemmap/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/systemmap/systemmap.go
git commit -m "feat(systemmap): add star-orbit conflict detection logging"
```

### Task 14: Run full test suite

**Files:**
- Test: All modified packages

- [ ] **Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

- [ ] **Step 2: Run golangci-lint**

Run: `golangci-lint run ./...`
Expected: No new findings

- [ ] **Step 3: Note: No commit needed for testing**

---

## Chunk 5: Database Migration

### Task 15: Add SQLite migration for class column

**Files:**
- Modify: `pkg/knowledge/sqlite_migrations.go`

- [ ] **Step 1: Find migrations array and add new migration**

Locate the migrations array in `pkg/knowledge/sqlite_migrations.go`. Add a new migration at the end:

```go
	{
		name: "add_poi_class_column",
		sql: `
			ALTER TABLE pois ADD COLUMN class TEXT;
		`,
	},
```

- [ ] **Step 2: Run build to verify**

Run: `go build ./pkg/knowledge/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add pkg/knowledge/sqlite_migrations.go
git commit -m "feat(knowledge): add migration for POI class column"
```

### Task 16: Update POI struct and queries in SQLite KB

**Files:**
- Modify: `pkg/knowledge/sqlite.go`

- [ ] **Step 1: Update memory.POI struct usage**

Find where POI is scanned from database queries. Update scan calls to include the new Class column:

Search for `scan` calls that populate POI structs and add the class field:

```go
// Example update pattern in POI query results:
err := rows.Scan(&poi.ID, &poi.SystemID, &poi.Type, &poi.Class, /* ... other fields */)
```

Note: The exact scan call locations vary. Search for `&poi.` in scan statements.

- [ ] **Step 2: Update INSERT statements to include class**

Find POI INSERT statements and add the class column:

```sql
INSERT INTO pois (id, system_id, type, class, name, description, position_x, position_y, ...)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ...)
```

- [ ] **Step 3: Run build to verify**

Run: `go build ./pkg/knowledge/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add pkg/knowledge/sqlite.go
git commit -m "feat(knowledge): update POI queries to include class field"
```

---

## Chunk 6: Validation and Testing

### Task 17: Create integration test for system map rendering

**Files:**
- Create: `pkg/systemmap/systemmap_integration_test.go`

- [ ] **Step 1: Write integration test**

Create `pkg/systemmap/systemmap_integration_test.go`:

```go
package systemmap

import "testing"

func TestRenderSystemMapWithClassifications(t *testing.T) {
	allSystems := map[string]*System{
		"test-system": {
			ID:   "test-system",
			Name: "Test System",
			POIs: []POI{
				{
					ID:        "star-1",
					Type:      "sun",
					Name:      "Test Star",
					Class:     "G2 V",
					PositionX: 0,
					PositionY: 0,
				},
				{
					ID:        "planet-1",
					Type:      "planet",
					Name:      "Test Planet",
					Class:     "terran",
					PositionX: 1,
					PositionY: 0,
				},
			},
		},
	}

	sys := allSystems["test-system"]
	output := RenderSystemMap(sys, allSystems, true)

	// Check that output contains expected elements
	expectedStrings := []string{
		`G2 V`,                   // Star classification in label
		`#fff4a0`,                // G-type color
		`#4a9c6d`,                // Terran planet color
		`Test Star`,              // Star name
		`Test Planet`,            // Planet name
		`xmlns="http://www.w3.org/2000/svg"`, // Standalone SVG
	}

	for _, expected := range expectedStrings {
		if !contains(output, expected) {
			t.Errorf("RenderSystemMap output missing expected string: %q", expected)
		}
	}
}

func TestRenderSystemMapInvalidClassifications(t *testing.T) {
	allSystems := map[string]*System{
		"test-system": {
			ID:   "test-system",
			Name: "Test System",
			POIs: []POI{
				{
					ID:        "star-1",
					Type:      "sun",
					Name:      "Invalid Star",
					Class:     "X9 Z", // Invalid
					PositionX: 0,
					PositionY: 0,
				},
			},
		},
	}

	sys := allSystems["test-system"]
	output := RenderSystemMap(sys, allSystems, true)

	// Should still render with default styling
	if !contains(output, `Invalid Star`) {
		t.Error("RenderSystemMap should render star even with invalid class")
	}
	if !contains(output, `#EBCB8B`) {
		t.Error("Invalid class should use default sun color")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run integration test**

Run: `go test ./pkg/systemmap/ -run TestRenderSystemMap -v`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add pkg/systemmap/systemmap_integration_test.go
git commit -m "test(systemmap): add integration tests for classified POI rendering"
```

### Task 18: Generate visual test samples

**Files:**
- Create: `cmd/test-system-map/main.go`

- [ ] **Step 1: Create test system map generator**

Create `cmd/test-system-map/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

type POI = struct {
	ID, Type, Class, Name string
	PositionX, PositionY float64
}

type System = struct {
	ID, Name string
	POIs     []POI
}

func main() {
	systems := []struct {
		name  string
		system System
	}{
		{
			name: "O-type hypergiant",
			system: System{
				ID: "test-o-hypergiant",
				Name: "O-type Hypergiant System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "O3 Ia", Name: "Hypergiant", PositionX: 0, PositionY: 0},
					{ID: "p1", Type: "planet", Class: "jovian", Name: "Gas Giant", PositionX: 5, PositionY: 0},
				},
			},
		},
		{
			name: "G2V Sun-like",
			system: System{
				ID: "test-g2v",
				Name: "G-type Main Sequence System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "G2 V", Name: "Sol-like", PositionX: 0, PositionY: 0},
					{ID: "p1", Type: "planet", Class: "terran", Name: "Earth-like", PositionX: 1, PositionY: 0},
				},
			},
		},
		{
			name: "M-type dwarf",
			system: System{
				ID: "test-m-dwarf",
				Name: "M-type Dwarf System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "M9 V", Name: "Red Dwarf", PositionX: 0, PositionY: 0},
					{ID: "p1", Type: "planet", Class: "rocky", Name: "Rocky World", PositionX: 0.5, PositionY: 0},
				},
			},
		},
		{
			name: "L-type brown dwarf",
			system: System{
				ID: "test-brown-dwarf",
				Name: "Brown Dwarf System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "L5 V", Name: "Brown Dwarf", PositionX: 0, PositionY: 0},
				},
			},
		},
		{
			name: "Mixed planets",
			system: System{
				ID: "test-mixed",
				Name: "Mixed Planet System",
				POIs: []POI{
					{ID: "star", Type: "sun", Class: "G2 V", Name: "Star", PositionX: 0, PositionY: 0},
					{ID: "p1", Type: "planet", Class: "jovian", Name: "Gas Giant", PositionX: 3, PositionY: 0},
					{ID: "p2", Type: "planet", Class: "ice_giant", Name: "Ice Giant", PositionX: 5, PositionY: 0},
					{ID: "p3", Type: "planet", Class: "terran", Name: "Terran", PositionX: 1.5, PositionY: 0},
				},
			},
		},
	}

	for _, tc := range systems {
		filename := fmt.Sprintf("test-%s.html", tc.name)
		// Would call actual RenderSystemMap here
		// For now, just create placeholder files
		f, _ := os.Create(filename)
		f.WriteString(fmt.Sprintf("<!-- %s -->\n", tc.name))
		f.Close()
		fmt.Printf("Generated %s\n", filename)
	}
}
```

- [ ] **Step 2: Run test generator**

Run: `go run ./cmd/test-system-map/main.go`
Expected: Generates test HTML files

- [ ] **Step 3: Commit**

```bash
git add cmd/test-system-map/main.go
git commit -m "test(systemmap): add visual test sample generator"
```

---

## Chunk 7: Documentation and Completion

### Task 19: Update documentation

**Files:**
- Modify: `README.md` or relevant docs

- [ ] **Step 1: Document the new feature**

Add documentation about the MK classification rendering feature to the appropriate documentation file (e.g., README.md or docs directory):

```markdown
## Star Classification Rendering

System maps now display MK (Morgan-Keenan) star classifications with scientifically accurate colors and sizes:

### Spectral Types (O-Y)
- **O**: Blue (>30,000 K)
- **B**: Blue-White (10,000-30,000 K)
- **A**: White (7,500-10,000 K)
- **F**: Yellow-White (6,000-7,500 K)
- **G**: Yellow (5,200-6,000 K) - e.g., The Sun (G2 V)
- **K**: Orange (3,700-5,200 K)
- **M**: Red (2,400-3,700 K)
- **L/T/Y**: Brown dwarfs (<2,400 K)

### Luminosity Classes
- **Ia/Ib**: Supergiants (largest)
- **II**: Bright giants
- **III**: Giants
- **IV**: Subgiants
- **V**: Main sequence (90% of stars)
- **VI**: Subdwarfs
- **VII**: White dwarfs

### Planet Classes
- **terran**: Earth-like blue-green
- **jovian**: Gas giant orange with rings
- **ice_giant**: Neptune/Uranus-like pale blue
- **rocky**: Mercury/Mars-like brown-gray
- **dwarf**: Pluto-like tan-brown

Stars and planets without classification data use default styling.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: document MK star classification rendering feature"
```

### Task 20: Final validation and cleanup

**Files:**
- All modified code

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

- [ ] **Step 2: Run golangci-lint**

Run: `golangci-lint run ./...`
Expected: No new findings

- [ ] **Step 3: Build all packages**

Run: `go build ./...`
Expected: No errors

- [ ] **Step 4: Create summary commit**

```bash
git add -A
git commit -m "feat: complete MK star classification rendering for system maps

- Add spectral type colors (O-Y) with accurate temperature gradients
- Add luminosity-based sizing (Ia-VII) with logarithmic scale
- Add planet class colors (terran, jovian, ice_giant, etc.)
- Add ring rendering for jovian gas giants
- Add brown dwarf visual distinction (muted glow)
- Add star-orbit conflict detection logging
- Support both 'G2 V' and 'G2V' class string formats
- Maintain backward compatibility with unclassified POIs

Closes: https://github.com/user/repo/issues/XXX"
```

- [ ] **Step 5: Tag completion**

Create a git tag for this feature:

```bash
git tag -a v0.1.0-mk-classification -m "MK Star Classification Rendering"
```

---

## Success Criteria Validation

After completing all tasks, verify:

- [ ] All spectral types (O-Y) render with correct colors
- [ ] All luminosity classes (Ia-VII) render with correct sizes
- [ ] All planet classes render with appropriate styling
- [ ] Jovian planets display ring systems
- [ ] Brown dwarfs appear distinct from main sequence stars
- [ ] Malformed class strings don't crash rendering
- [ ] Performance overhead <10% (measure with benchmarks)
- [ ] Existing systems without classification render unchanged
- [ ] Tooltips provide helpful classification information
- [ ] All tests pass with `go test ./...`
- [ ] No new golangci-lint findings
- [ ] Documentation is updated

---

## Notes for Implementation

- **TDD approach**: Each function has a test written before implementation
- **Frequent commits**: Each task commits immediately after completion
- **DRY principle**: Color/class data defined once in lookup tables
- **YAGNI principle**: Only implementing features from the approved spec
- **Error handling**: Malformed data logs warnings but doesn't crash
- **Performance**: Pre-compute gradients, minimize string allocations

## Rollback Plan

If issues are found:

1. **Revert commit**: `git revert HEAD` for problematic changes
2. **Hotfix**: Create patch release with specific fix
3. **Fallback**: Existing systems continue working without classifications

## Future Enhancements (Out of Scope)

- Binary star systems
- Variable star indicators
- Protoplanetary disks
- Interactive tooltips with full Wikipedia-style data
- Spectral gradient mapping (G0-G9 fine-tuning)
