// Package galaxymap renders galaxy maps as inline SVG.
//
// It is the reusable core extracted from cmd/generate-galaxy-map, so other
// pages (such as a resource-highlighted galaxy map) can render the same
// visualization with a lighter feature set via Options.
package galaxymap

import (
	"fmt"
	"strings"
)

// System holds galaxy map data for a single system.
type System struct {
	ID              string
	Name            string
	PositionX       float64
	PositionY       float64
	PoliceLevel     int
	Empire          string
	IsStronghold    bool
	LastUpdatedTick int
	Connections     []Connection
}

// Connection is a jump gate connection to another system.
type Connection struct {
	SystemID string
	Name     string
	Distance int
}

// Options controls which visual elements Render includes and how system
// dots are linked and classed.
type Options struct {
	// ShowEmpireBlobs draws the metaball-style territory blob behind
	// explored systems and their connections.
	ShowEmpireBlobs bool
	// ShowConnections draws the visible connection lines between
	// systems, both explored-to-explored and to unexplored systems.
	ShowConnections bool
	// HighlightClasses, if set, is called per system ID to obtain extra
	// CSS classes appended to that system's dot.
	HighlightClasses func(systemID string) []string
	// LinkPrefix is prepended to the "systems/<id>/" href on each dot.
	LinkPrefix string
}

// Render generates a galaxy SVG map (or a placeholder string if there are
// no explored systems) for the given explored and unexplored systems.
func Render(explored, unexplored []*System, systemMap map[string]*System, opt Options) string {
	if len(explored) == 0 {
		return `<p style="padding:20px;text-align:center">No explored systems to display.</p>`
	}

	// Build explored set for fast lookup.
	exploredSet := make(map[string]bool, len(explored))
	for _, s := range explored {
		exploredSet[s.ID] = true
	}

	// Compute bounding box of explored systems.
	minX, minY := explored[0].PositionX, explored[0].PositionY
	maxX, maxY := minX, minY
	for _, s := range explored[1:] {
		if s.PositionX < minX {
			minX = s.PositionX
		}
		if s.PositionX > maxX {
			maxX = s.PositionX
		}
		if s.PositionY < minY {
			minY = s.PositionY
		}
		if s.PositionY > maxY {
			maxY = s.PositionY
		}
	}

	// Add padding.
	padX := (maxX - minX) * 0.10
	padY := (maxY - minY) * 0.10
	if padX < 50 {
		padX = 50
	}
	if padY < 50 {
		padY = 50
	}
	minX -= padX
	minY -= padY
	maxX += padX
	maxY += padY

	rangeX := maxX - minX
	rangeY := maxY - minY
	if rangeX < 1 {
		rangeX = 1
	}
	if rangeY < 1 {
		rangeY = 1
	}

	// Use 2000x2000 canvas as requested.
	const svgSize = 2000.0
	scale := svgSize / max(rangeX, rangeY)

	// Center the explored region in the canvas.
	offsetX := (svgSize - rangeX*scale) / 2
	offsetY := (svgSize - rangeY*scale) / 2

	// Transform galaxy coords to SVG coords.
	tx := func(x float64) float64 {
		return (x-minX)*scale + offsetX
	}
	ty := func(y float64) float64 {
		return (y-minY)*scale + offsetY
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg" class="galaxy-map-svg">`, svgSize, svgSize))

	// Dark background for space.
	b.WriteString(`<rect width="100%" height="100%" fill="#0a0e1a"/>`)

	// blobColor is hoisted above the ShowEmpireBlobs guard: it is also
	// read later by the dot-color default and the unexplored-dot loop.
	const blobColor = "#E8E8E8" // Light white/grey

	if opt.ShowEmpireBlobs {
		// Metaball filter for explored territory blob.
		b.WriteString(`<defs><filter id="goo-galaxy" x="-20%" y="-20%" width="140%" height="140%" colorInterpolationFilters="sRGB">`)
		b.WriteString(`<feGaussianBlur in="SourceGraphic" stdDeviation="18" result="blur"/>`)
		b.WriteString(`<feColorMatrix in="blur" type="matrix" values="1 0 0 0 0  0 1 0 0 0  0 0 1 0 0  0 0 0 30 -12" result="blob"/>`)
		b.WriteString(`<feComponentTransfer in="blob" result="fill"><feFuncA type="linear" slope="0.25" intercept="0"/></feComponentTransfer>`)
		b.WriteString(`</filter></defs>`)

		// Territory blob - only for explored systems and their connections.
		blobR := 28.0
		b.WriteString(`<g filter="url(#goo-galaxy)">`)

		// Thick connection lines between explored systems.
		drawnBlob := make(map[string]bool)
		for _, s := range explored {
			for _, conn := range s.Connections {
				if !exploredSet[conn.SystemID] {
					continue // Skip connections to unexplored systems
				}
				key := s.ID + "|" + conn.SystemID
				rev := conn.SystemID + "|" + s.ID
				if drawnBlob[key] || drawnBlob[rev] {
					continue
				}
				drawnBlob[key] = true
				target := systemMap[conn.SystemID]
				if target == nil {
					continue
				}
				b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.0f"/>`,
					tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY), blobColor, blobR*1.2))
			}
		}

		// Circles at each explored system position.
		for _, s := range explored {
			b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.0f" fill="%s"/>`, tx(s.PositionX), ty(s.PositionY), blobR, blobColor))
		}
		b.WriteString(`</g>`)
	}

	if opt.ShowConnections {
		// Visible connection lines between explored systems (on top of blob).
		b.WriteString(`<g stroke="#63b3ed" stroke-width="2" opacity="0.6">`)
		drawn := make(map[string]bool)
		for _, s := range explored {
			for _, conn := range s.Connections {
				if !exploredSet[conn.SystemID] {
					continue
				}
				key := s.ID + "|" + conn.SystemID
				rev := conn.SystemID + "|" + s.ID
				if drawn[key] || drawn[rev] {
					continue
				}
				drawn[key] = true
				target := systemMap[conn.SystemID]
				if target == nil {
					continue
				}
				b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
					tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY)))
			}
		}
		b.WriteString(`</g>`)

		// Outgoing connections to unexplored systems (dashed, brighter).
		b.WriteString(`<g stroke="#a0aec0" stroke-width="2" opacity="0.8" stroke-dasharray="4,4">`)
		drawnUnexplored := make(map[string]bool)

		// Connections from explored to unexplored
		for _, s := range explored {
			for _, conn := range s.Connections {
				if exploredSet[conn.SystemID] {
					continue
				}
				target := systemMap[conn.SystemID]
				if target == nil {
					continue
				}
				key := s.ID + "|" + conn.SystemID
				drawnUnexplored[key] = true
				b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
					tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY)))
			}
		}

		// Connections between unexplored systems
		for _, s := range unexplored {
			for _, conn := range s.Connections {
				if exploredSet[conn.SystemID] {
					continue
				}
				target := systemMap[conn.SystemID]
				if target == nil {
					continue
				}
				key := s.ID + "|" + conn.SystemID
				rev := conn.SystemID + "|" + s.ID
				if drawnUnexplored[key] || drawnUnexplored[rev] {
					continue
				}
				drawnUnexplored[key] = true
				b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
					tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY)))
			}
		}
		b.WriteString(`</g>`)
	}

	// Explored system dots.
	for _, s := range explored {
		sx, sy := tx(s.PositionX), ty(s.PositionY)

		// Dot color based on empire.
		dotColor := blobColor
		if s.Empire != "" {
			switch s.Empire {
			case "solarian":
				dotColor = "#FFD700"
			case "voidborn":
				dotColor = "#9932CC"
			case "crimson":
				dotColor = "#DC143C"
			case "nebula":
				dotColor = "#00CED1"
			case "outerrim":
				dotColor = "#2E8B57"
			}
		}

		if s.IsStronghold {
			dotColor = "#FF0000"
		}

		classes := "galaxy-sys-dot"
		if opt.HighlightClasses != nil {
			if extra := opt.HighlightClasses(s.ID); len(extra) > 0 {
				classes = classes + " " + strings.Join(extra, " ")
			}
		}
		b.WriteString(fmt.Sprintf(`<a href="%ssystems/%s/"><circle cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="#000" stroke-width="0.5" class="%s"><title>%s</title></circle>`,
			opt.LinkPrefix, s.ID, sx, sy, dotColor, classes, s.Name))

		// Label for major systems (capitals or strongholds).
		if s.IsStronghold || isCapital(s.ID) {
			b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="galaxy-sys-label" fill="#d8dee9" font-size="12" font-weight="bold">%s</text></a>`,
				sx+8, sy+4, s.Name))
		}
		b.WriteString(`</a>`)
	}

	// Unexplored system dots (same style as explored non-empire stars).
	for _, s := range unexplored {
		sx, sy := tx(s.PositionX), ty(s.PositionY)
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="#000" stroke-width="0.5" opacity="0.7"><title>%s (Unexplored)</title></circle>`,
			sx, sy, blobColor, s.Name))
	}

	b.WriteString(`</svg>`)
	return b.String()
}

// isCapital returns true if the system is an empire capital.
func isCapital(systemID string) bool {
	capitals := map[string]bool{
		"sol":         true, // Solarian
		"nexus_prime": true, // Voidborn
		"krynn":       true, // Crimson
		"haven":       true, // Nebula
		"frontier":    true, // Outerrim
	}
	return capitals[systemID]
}
