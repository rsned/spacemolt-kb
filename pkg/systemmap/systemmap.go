// Package systemmap provides SVG rendering for system maps.
package systemmap

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"strings"
)

// System holds the data needed to render a system map.
type System struct {
	ID          string
	Name        string
	PositionX   float64
	PositionY   float64
	Connections []Connection
	POIs        []POI
}

// Connection is a jump gate connection to another system.
type Connection struct {
	SystemID string
	Name     string
	Distance int
}

// POI is a point of interest within a system.
type POI struct {
	ID          string
	Name        string
	Type        string
	Class       string // Star class (e.g., "G2 V") or planet class (e.g., "terran")
	Description string
	PositionX   float64
	PositionY   float64
}

// RenderSystemMap generates the complete SVG system map HTML for a system page.
//
// When standalone is false, the SVG is wrapped in a <div class="sys-map"> and
// relies on external CSS for text styling.
//
// When standalone is true, the SVG is emitted bare (no wrapper div) with an
// xmlns attribute and an embedded <style> block so it is self-contained.
func RenderSystemMap(sys *System, allSystems map[string]*System, standalone bool) string {
	const (
		cx     = 400.0
		cy     = 300.0
		maxSVG = 250.0 // max radius in SVG pixels for outermost POI orbit
	)

	var b strings.Builder

	if !standalone {
		b.WriteString(`<div class="sys-map">`)
	}

	// Determine if explored (has POI data).
	explored := len(sys.POIs) > 0

	// Compute scale and gate radius.
	var scale, gateRadius float64
	if explored {
		var maxR float64
		for _, poi := range sys.POIs {
			r := math.Hypot(poi.PositionX, poi.PositionY)
			if r > maxR {
				maxR = r
			}
		}
		if maxR < 1 {
			maxR = 1 // avoid division by zero for systems with only a sun at origin
		}
		scale = maxSVG / maxR
		gateRadius = maxSVG + 30
	} else {
		scale = 100 // arbitrary; fog covers everything
		gateRadius = 230
	}

	// Check for star-orbit conflicts when explored
	if explored {
		for _, poi := range sys.POIs {
			if poi.Type == "sun" && poi.Class != "" {
				_, _, luminosity, err := ParseStarClass(poi.Class)
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

	// Compute viewBox so outermost orbit fills 80% of width, with cropping.
	maxOrbitR := gateRadius + 5 // include gate ring
	if explored {
		for _, poi := range sys.POIs {
			if poi.Type == "sun" {
				continue
			}
			r := math.Hypot(poi.PositionX, poi.PositionY) * scale
			if r > maxOrbitR {
				maxOrbitR = r
			}
		}
	}
	fillRatio := 0.8 // orbit fills 80% of viewBox (10% padding per side)
	if standalone {
		fillRatio = 0.9 // orbit fills 90% of viewBox (5% padding per side)
	}
	vbSize := 2 * maxOrbitR / fillRatio
	vbX := cx - vbSize/2
	vbY := cy - vbSize/2

	aspectRatio := "xMidYMid slice"
	if standalone {
		aspectRatio = "xMidYMid meet"
	}
	b.WriteString(fmt.Sprintf(`<svg preserveAspectRatio="%s" viewBox="%.1f %.1f %.1f %.1f" xmlns="http://www.w3.org/2000/svg">`, aspectRatio, vbX, vbY, vbSize, vbSize))

	if standalone {
		b.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#000"/>`, vbX, vbY, vbSize, vbSize))
	}

	// Compute visible range for gate clamping.
	visTop := vbY + 25
	visBottom := vbY + vbSize - 25

	if explored {
		// Axis crosshairs (span full viewBox).
		b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#8b95ab" stroke-width="0.5" opacity="0.9"/>`, vbX, cy, vbX+vbSize, cy))
		b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#8b95ab" stroke-width="0.5" opacity="0.9"/>`, cx, vbY, cx, vbY+vbSize))

		// Tick marks at whole-number game coordinate intervals.
		tickLen := 4.0 // half-length of each tick mark
		minGame := int(math.Floor((vbX - cx) / scale))
		maxGame := int(math.Ceil((vbX + vbSize - cx) / scale))
		for n := minGame; n <= maxGame; n++ {
			if n == 0 {
				continue
			}
			// X-axis tick (vertical line at game X = n).
			sx := cx + float64(n)*scale
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#8b95ab" stroke-width="0.5" opacity="0.9"/>`, sx, cy-tickLen, sx, cy+tickLen))
			// Y-axis tick (horizontal line at game Y = n; SVG Y is inverted).
			sy := cy - float64(n)*scale
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#8b95ab" stroke-width="0.5" opacity="0.9"/>`, cx-tickLen, sy, cx+tickLen, sy))
		}

		// Scale indicator in top-left corner: "1 AU" bar.
		scaleX := vbX + 20
		scaleY := vbY + 25
		b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#8b95ab" stroke-width="1" opacity="0.9"/>`, scaleX, scaleY, scaleX+scale, scaleY))
		// End caps.
		b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#8b95ab" stroke-width="1" opacity="0.9"/>`, scaleX, scaleY-3, scaleX, scaleY+3))
		b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#8b95ab" stroke-width="1" opacity="0.9"/>`, scaleX+scale, scaleY-3, scaleX+scale, scaleY+3))
		// Label.
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" fill="#8b95ab" font-size="10" font-family="sans-serif">1 AU</text>`, scaleX+scale/2, scaleY-6))

		// Orbital rings for each non-sun POI.
		for _, poi := range sys.POIs {
			if poi.Type == "sun" {
				continue
			}
			r := math.Hypot(poi.PositionX, poi.PositionY) * scale
			if r < 1 {
				continue
			}
			b.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="#8b95ab" stroke-width="0.7" opacity="0.9" stroke-dasharray="4,4"/>`, cx, cy, r))
		}
	}

	// Jump gate ring (dashed, just inside the gate markers).
	b.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.6" stroke-dasharray="8,6"/>`, cx, cy, gateRadius+5))

	// Jump gate markers.
	for _, conn := range sys.Connections {
		angle := computeGateAngle(sys, conn.SystemID, allSystems)
		gx := cx + gateRadius*math.Cos(angle)
		gy := cy - gateRadius*math.Sin(angle) // Y inverted
		// Clamp to visible area.
		gx = math.Max(vbX+30, math.Min(vbX+vbSize-30, gx))
		gy = math.Max(visTop, math.Min(visBottom, gy))

		// Dashed line from gate toward center.
		lineEndX := cx + 20*math.Cos(angle)
		lineEndY := cy - 20*math.Sin(angle)
		b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#81a1c1" stroke-width="0.5" opacity="0.45" stroke-dasharray="6,4"/>`,
			gx, gy, lineEndX, lineEndY))

		// Gate marker: circle + crosshair + label, wrapped in <a>.
		b.WriteString(fmt.Sprintf(`<a href="%s.html" class="gate-link">`, conn.SystemID))
		b.WriteString(`<g class="poi-marker">`)
		b.WriteString(fmt.Sprintf(`<title>Jump Gate: %s</title>`, htmlEscape(conn.Name)))
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="8" fill="none" stroke="#81a1c1" stroke-width="1.5"/>`, gx, gy))
		b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#81a1c1" stroke-width="1"/>`, gx, gy-8, gx, gy+8))
		b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#81a1c1" stroke-width="1"/>`, gx-8, gy, gx+8, gy))
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" class="gate-label" fill="#81a1c1">%s</text>`, gx, gy+21, htmlEscape(conn.Name)))
		b.WriteString(`</g></a>`)
	}

	// Pre-compute black hole → star stream data for gradient definitions.
	type bhStreamEntry struct {
		stream *blackHoleStreamInfo
		bx, by float64 // BH SVG coordinates
	}
	bhStreams := make(map[string]*bhStreamEntry)
	if explored {
		for _, poi := range sys.POIs {
			if poi.Type != "black_hole" {
				continue
			}
			bhSX := cx + poi.PositionX*scale
			bhSY := cy - poi.PositionY*scale

			// Find the nearest star to draw the accretion stream.
			var nearest *POI
			var minDist float64 = -1
			for i, other := range sys.POIs {
				if other.Type != "sun" {
					continue
				}
				d := math.Hypot(poi.PositionX-other.PositionX, poi.PositionY-other.PositionY)
				if minDist < 0 || d < minDist {
					minDist = d
					nearest = &sys.POIs[i]
				}
			}

			entry := &bhStreamEntry{bx: bhSX, by: bhSY}
			if nearest != nil {
				spectral, subtype, luminosity, err := ParseStarClass(nearest.Class)
				starColor := "#EBCB8B"
				starSize := 10.0
				if err == nil {
					starColor = GetStarColorRefined(spectral, subtype)
					starSize = GetStarSize(luminosity)
				}
				entry.stream = &blackHoleStreamInfo{
					sx:        cx + nearest.PositionX*scale,
					sy:        cy - nearest.PositionY*scale,
					starColor: starColor,
					starSize:  starSize,
				}
			}
			bhStreams[poi.ID] = entry
		}
	}

	if explored {
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
		// Wormhole glow gradient (orange-red).
		b.WriteString(`<radialGradient id="wormhole-glow">`)
		b.WriteString(`<stop offset="0%" stop-color="#1a0a00" stop-opacity="1"/>`)
		b.WriteString(`<stop offset="35%" stop-color="#BF360C" stop-opacity="0.9"/>`)
		b.WriteString(`<stop offset="60%" stop-color="#D84315" stop-opacity="0.5"/>`)
		b.WriteString(`<stop offset="100%" stop-color="#D08770" stop-opacity="0"/>`)
		b.WriteString(`</radialGradient>`)
		// Collapsed wormhole glow gradient (yellow).
		b.WriteString(`<radialGradient id="collapsed-wormhole-glow">`)
		b.WriteString(`<stop offset="0%" stop-color="#1a1a00" stop-opacity="1"/>`)
		b.WriteString(`<stop offset="35%" stop-color="#F9A825" stop-opacity="0.7"/>`)
		b.WriteString(`<stop offset="60%" stop-color="#FDD835" stop-opacity="0.35"/>`)
		b.WriteString(`<stop offset="100%" stop-color="#FDD835" stop-opacity="0"/>`)
		b.WriteString(`</radialGradient>`)
		// Add star-specific gradients for classified stars
		for _, poi := range sys.POIs {
			if poi.Type == "sun" && poi.Class != "" {
				spectral, subtype, _, err := ParseStarClass(poi.Class)
				if err == nil {
					color := GetStarColorRefined(spectral, subtype)
					isBrownDwarf := (spectral == "L" || spectral == "T" || spectral == "Y")
					b.WriteString(renderStarGlowGradient(poi, color, isBrownDwarf))
				}
			}
		}
		// Add black hole gradients (glow + accretion stream).
		for _, poi := range sys.POIs {
			if poi.Type == "black_hole" {
				entry := bhStreams[poi.ID]
				if entry != nil {
					b.WriteString(renderBlackHoleGradients(poi, entry.stream, entry.bx, entry.by))
				}
			}
		}
		b.WriteString(`</defs>`)

		// Render POIs by type; collect label info for de-overlap.
		type labelInfo struct {
			x, y  float64
			name  string
			above bool // true = label above POI
		}
		var labels []labelInfo

		for _, poi := range sys.POIs {
			px := cx + poi.PositionX*scale
			py := cy - poi.PositionY*scale
			r := math.Hypot(poi.PositionX, poi.PositionY) * scale

			// Label direction: game y >= 0 (SVG y <= center) -> above; game y < 0 -> below.
			labelAbove := poi.PositionY >= 0

			switch poi.Type {
			case "sun":
				if poi.Class != "" {
					// Use classified star rendering
					b.WriteString(renderStar(poi, px, py))
					// Label is handled by renderStar
				} else {
					// Use default sun rendering
					b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, cx, cy))
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="24" fill="url(#sun-glow)"/>`, cx, cy))
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="10" fill="#EBCB8B"/>`, cx, cy))
					b.WriteString(`</g>`)
					labels = append(labels, labelInfo{x: cx, y: cy + 28, name: poi.Name, above: false})
				}

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

			case "station":
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
				b.WriteString(renderHexagon(px, py, 6))
				b.WriteString(`</g>`)
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 8, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 20, name: poi.Name, above: false})
				}

			case "asteroid_belt":
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.0f" fill="none" stroke="#8b95ab" stroke-width="0.7" opacity="0.9" stroke-dasharray="4,4"/>`, cx, cy, r))
				b.WriteString(generateAsteroidParticles(cx, cy, r, poiSeed(poi.ID)))
				b.WriteString(`</g>`)
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 12, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 18, name: poi.Name, above: false})
				}

			case "ice_field":
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.0f" fill="none" stroke="#8b95ab" stroke-width="0.7" opacity="0.9" stroke-dasharray="4,4"/>`, cx, cy, r))
				b.WriteString(generateIceParticles(cx, cy, r, poiSeed(poi.ID)))
				b.WriteString(`</g>`)
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 12, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 18, name: poi.Name, above: false})
				}

			case "gas_cloud":
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="5" fill="#B48EAD" opacity="0.35"/>`, px-4, py+2))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="3" fill="#B48EAD" opacity="0.45"/>`, px+5, py-3))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4" fill="#B48EAD" opacity="0.40"/>`, px+1, py+4))
				// 3 extra random small bubbles.
				gcRng := rand.New(rand.NewPCG(poiSeed(poi.ID), poiSeed(poi.ID)^0xbeef))
				for range 3 {
					bx := px + (gcRng.Float64()-0.5)*16
					by := py + (gcRng.Float64()-0.5)*16
					br := 2.0 + gcRng.Float64()*3.0
					bo := 0.25 + gcRng.Float64()*0.2
					b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#B48EAD" opacity="%.2f"/>`, bx, by, br, bo))
				}
				b.WriteString(`</g>`)
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 12, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 18, name: poi.Name, above: false})
				}

			case "relic":
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
				b.WriteString(renderCompassRose(px, py, 10))
				b.WriteString(`</g>`)
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 12, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 18, name: poi.Name, above: false})
				}

			case "wormhole":
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
				b.WriteString(renderWormhole(px, py, 10, false))
				b.WriteString(`</g>`)
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 14, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 20, name: poi.Name, above: false})
				}

			case "collapsed_wormhole":
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
				b.WriteString(renderWormhole(px, py, 10, true))
				b.WriteString(`</g>`)
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 14, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 20, name: poi.Name, above: false})
				}

			case "black_hole":
				entry := bhStreams[poi.ID]
				var stream *blackHoleStreamInfo
				if entry != nil {
					stream = entry.stream
				}
				b.WriteString(renderBlackHole(poi, px, py, stream))
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 22, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 26, name: poi.Name, above: false})
				}
			}
		}

		// De-overlap labels: push overlapping labels in their stacking direction.
		const labelH = 12.0
		for i := range labels {
			for j := range labels {
				if i == j {
					continue
				}
				if math.Abs(labels[i].x-labels[j].x) > 50 {
					continue
				}
				if math.Abs(labels[i].y-labels[j].y) < labelH {
					if labels[i].above {
						labels[i].y = labels[j].y - labelH
					} else {
						labels[i].y = labels[j].y + labelH
					}
				}
			}
		}

		// Render labels.
		for _, lbl := range labels {
			b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" class="map-label">%s</text>`, lbl.x, lbl.y, htmlEscape(lbl.name)))
		}

		// Legend: collect unique POI types present in this system.
		b.WriteString(renderLegend(sys.POIs, vbX, vbY, vbSize))
	} else {
		// Unexplored: star + fog of war.
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
		b.WriteString(`<radialGradient id="fog">`)
		b.WriteString(`<stop offset="0%" stop-color="#8b95ab" stop-opacity="0"/>`)
		b.WriteString(`<stop offset="30%" stop-color="#8b95ab" stop-opacity="0"/>`)
		b.WriteString(`<stop offset="60%" stop-color="#8b95ab" stop-opacity="0.15"/>`)
		b.WriteString(`<stop offset="100%" stop-color="#8b95ab" stop-opacity="0.25"/>`)
		b.WriteString(`</radialGradient>`)
		b.WriteString(`</defs>`)

		// Star.
		b.WriteString(`<g class="poi-marker">`)
		b.WriteString(fmt.Sprintf(`<title>%s</title>`, htmlEscape(sys.Name)))
		b.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="24" fill="url(#sun-glow)"/>`, cx, cy))
		b.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="10" fill="#EBCB8B"/>`, cx, cy))
		b.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.0f" text-anchor="middle" class="map-label">%s</text>`, cx, cy+28, htmlEscape(sys.Name)))
		b.WriteString(`</g>`)

		// Fog circle.
		b.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="200" fill="url(#fog)"/>`, cx, cy))
	}

	b.WriteString(`</svg>`)
	if !standalone {
		b.WriteString(`</div>`)
	}
	return b.String()
}

// computeGateAngle returns the angle (radians) from sys to the connected system
// based on galaxy-map coordinates.
func computeGateAngle(sys *System, targetID string, allSystems map[string]*System) float64 {
	target, ok := allSystems[targetID]
	if !ok {
		return 0
	}
	dx := target.PositionX - sys.PositionX
	// Galaxy map Y-axis is inverted relative to the official game map.
	dy := -(target.PositionY - sys.PositionY)
	return math.Atan2(dy, dx)
}

// poiSeed returns a deterministic seed from a POI ID.
func poiSeed(id string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return h.Sum64()
}

// poiTitle builds a tooltip string for a POI.
func poiTitle(poi POI) string {
	if poi.Description != "" {
		return poi.Name + " \u2014 " + poi.Description
	}
	return poi.Name
}

// htmlEscape escapes a string for safe inclusion in SVG/HTML attributes.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// renderHexagon returns an SVG polygon for a hexagon centered at (cx, cy) with the given radius.
func renderHexagon(hx, hy, r float64) string {
	var pts []string
	for i := range 6 {
		angle := math.Pi/6 + float64(i)*math.Pi/3
		px := hx + r*math.Cos(angle)
		py := hy + r*math.Sin(angle)
		pts = append(pts, fmt.Sprintf("%.0f,%.0f", px, py))
	}
	return fmt.Sprintf(`<polygon points="%s" fill="none" stroke="#81a1c1" stroke-width="1.5"/>`, strings.Join(pts, " "))
}

// renderCompassRose returns an SVG compass rose centered at (cx, cy) with the given outer radius.
// It draws four elongated diamond points on the cardinal axes with a small circle at the center.
func renderCompassRose(cx, cy, r float64) string {
	var b strings.Builder
	// Each cardinal point is a thin diamond: tip at r, base at r*0.3, width r*0.2.
	half := r * 0.2 // half-width of each diamond
	base := r * 0.3 // distance from center to the inner base of each point
	// N point (up in SVG = -Y).
	b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#EBCB8B" opacity="0.9"/>`,
		cx, cy-r, cx+half, cy-base, cx, cy, cx-half, cy-base))
	// S point.
	b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#EBCB8B" opacity="0.9"/>`,
		cx, cy+r, cx-half, cy+base, cx, cy, cx+half, cy+base))
	// E point.
	b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#EBCB8B" opacity="0.9"/>`,
		cx+r, cy, cx+base, cy-half, cx, cy, cx+base, cy+half))
	// W point.
	b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#EBCB8B" opacity="0.9"/>`,
		cx-r, cy, cx-base, cy+half, cx, cy, cx-base, cy-half))
	// Center dot.
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#EBCB8B" opacity="0.9"/>`, cx, cy, r*0.15))
	return b.String()
}

// renderWormhole returns an SVG black-hole-style marker centered at (cx, cy) with the given radius.
// When collapsed is true, the accretion disk is fragmented and colored yellow instead of orange-red.
func renderWormhole(cx, cy, r float64, collapsed bool) string {
	var b strings.Builder
	gradientID := "wormhole-glow"
	diskColor := "#D84315"
	diskColorOuter := "#BF360C"
	rimColor := "#D08770"
	centerColor := "#0a0000"
	if collapsed {
		gradientID = "collapsed-wormhole-glow"
		diskColor = "#FDD835"
		diskColorOuter = "#F9A825"
		rimColor = "#FFEE58"
		centerColor = "#0a0a00"
	}

	// Outer glow.
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="url(#%s)"/>`,
		cx, cy, r*1.8, gradientID))

	// Tilted accretion disk (ellipse, ~30° tilt via scaleY).
	diskRX := r * 1.3
	diskRY := r * 0.45

	if collapsed {
		// Fragmented arcs for collapsed wormhole.
		for i, arc := range []struct {
			start, sweep float64
			opacity      float64
		}{
			{20, 70, 0.7},
			{120, 50, 0.5},
			{210, 80, 0.6},
			{330, 40, 0.4},
		} {
			_ = i
			startRad := arc.start * math.Pi / 180
			endRad := (arc.start + arc.sweep) * math.Pi / 180
			x1 := cx + diskRX*math.Cos(startRad)
			y1 := cy + diskRY*math.Sin(startRad)
			x2 := cx + diskRX*math.Cos(endRad)
			y2 := cy + diskRY*math.Sin(endRad)
			largeArc := 0
			if arc.sweep > 180 {
				largeArc = 1
			}
			b.WriteString(fmt.Sprintf(`<path d="M%.1f,%.1f A%.1f,%.1f 0 %d,1 %.1f,%.1f" fill="none" stroke="%s" stroke-width="2" opacity="%.2f"/>`,
				x1, y1, diskRX, diskRY, largeArc, x2, y2, diskColor, arc.opacity))
		}
		// Faint outer rim fragments.
		for _, arc := range []struct {
			start, sweep float64
		}{
			{40, 50},
			{180, 60},
		} {
			startRad := arc.start * math.Pi / 180
			endRad := (arc.start + arc.sweep) * math.Pi / 180
			outerRX := diskRX * 1.15
			outerRY := diskRY * 1.15
			x1 := cx + outerRX*math.Cos(startRad)
			y1 := cy + outerRY*math.Sin(startRad)
			x2 := cx + outerRX*math.Cos(endRad)
			y2 := cy + outerRY*math.Sin(endRad)
			b.WriteString(fmt.Sprintf(`<path d="M%.1f,%.1f A%.1f,%.1f 0 0,1 %.1f,%.1f" fill="none" stroke="%s" stroke-width="1" opacity="0.3"/>`,
				x1, y1, outerRX, outerRY, x2, y2, rimColor))
		}
	} else {
		// Solid accretion disk for active wormhole.
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="%s" stroke-width="2.5" opacity="0.85"/>`,
			cx, cy, diskRX, diskRY, diskColor))
		// Outer rim glow.
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="%s" stroke-width="1.5" opacity="0.5"/>`,
			cx, cy, diskRX*1.15, diskRY*1.15, diskColorOuter))
		// Inner bright rim.
		b.WriteString(fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="none" stroke="%s" stroke-width="1" opacity="0.6"/>`,
			cx, cy, diskRX*0.85, diskRY*0.85, rimColor))
	}

	// Dark center void.
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`,
		cx, cy, r*0.55, centerColor))

	return b.String()
}

// generateAsteroidParticles creates ~100 small triangle particles scattered along an orbital ring.
func generateAsteroidParticles(orbitCX, orbitCY, radius float64, seed uint64) string {
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
		opacity := 0.3 + rng.Float64()*0.4
		rotation := rng.Float64() * 360

		// Triangle: three points centered around (px, py).
		h := size * 0.866 // sqrt(3)/2
		p1x := px - size/2
		p1y := py + h/2
		p2x := px
		p2y := py - h/2
		p3x := px + size/2
		p3y := py + h/2
		b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#D08770" opacity="%.2f" transform="rotate(%.0f,%.1f,%.1f)"/>`,
			p1x, p1y, p2x, p2y, p3x, p3y, opacity, rotation, px, py))
	}
	return b.String()
}

// generateIceParticles creates ~80 small diamond particles scattered along an orbital ring.
func generateIceParticles(orbitCX, orbitCY, radius float64, seed uint64) string {
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
		opacity := 0.3 + rng.Float64()*0.3

		// Diamond: four points.
		b.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#88C0D0" opacity="%.2f"/>`,
			px, py-size, px+size*0.6, py, px, py+size, px-size*0.6, py, opacity))
	}
	return b.String()
}

// legendEntry defines a POI type for the map legend.
type legendEntry struct {
	poiType string
	label   string
}

// legendTypes lists all POI types in display order.
var legendTypes = []legendEntry{
	{"sun", "Star"},
	{"planet", "Planet"},
	{"station", "Station"},
	{"asteroid_belt", "Asteroid Belt"},
	{"ice_field", "Ice Field"},
	{"gas_cloud", "Gas Cloud"},
	{"relic", "Relic"},
	{"wormhole", "Wormhole"},
	{"collapsed_wormhole", "Collapsed Wormhole"},
	{"black_hole", "Black Hole"},
}

// renderLegend draws a legend box in the bottom-right corner of the map showing
// only the POI types present in the current system.
func renderLegend(pois []POI, vbX, vbY, vbSize float64) string {
	// Collect present types.
	present := make(map[string]bool)
	for _, poi := range pois {
		present[poi.Type] = true
	}

	// Build ordered list of entries to show.
	var entries []legendEntry
	for _, lt := range legendTypes {
		if present[lt.poiType] {
			entries = append(entries, lt)
		}
	}
	if len(entries) == 0 {
		return ""
	}

	const (
		rowH    = 16.0  // height per row
		padX    = 8.0   // horizontal padding
		padY    = 8.0   // vertical padding
		swatchW = 16.0  // swatch area width
		gap     = 6.0   // gap between swatch and label
		marginR = 10.0  // margin from viewBox right edge
		marginB = 10.0  // margin from viewBox bottom edge
		charW   = 5.0   // approximate character width at 9px font
	)

	// Compute box dimensions.
	maxLabelW := 0.0
	for _, e := range entries {
		w := float64(len(e.label)) * charW
		if w > maxLabelW {
			maxLabelW = w
		}
	}
	boxW := padX + swatchW + gap + maxLabelW + padX
	boxH := padY + float64(len(entries))*rowH + padY - (rowH - 12) // tighten bottom

	// Position: bottom-right of viewBox.
	boxX := vbX + vbSize - marginR - boxW
	boxY := vbY + vbSize - marginB - boxH

	var b strings.Builder

	// Background.
	b.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="4" fill="#000" fill-opacity="0.6" stroke="#8b95ab" stroke-width="0.5" stroke-opacity="0.5"/>`,
		boxX, boxY, boxW, boxH))

	// Rows.
	for i, e := range entries {
		rowY := boxY + padY + float64(i)*rowH
		swatchCX := boxX + padX + swatchW/2
		swatchCY := rowY + 6 // vertical center of swatch
		labelX := boxX + padX + swatchW + gap
		labelY := rowY + 10 // text baseline

		// Render mini swatch per type.
		b.WriteString(renderLegendSwatch(e.poiType, swatchCX, swatchCY))

		// Label text.
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" font-size="9" font-family="sans-serif" fill="#d8dee9">%s</text>`,
			labelX, labelY, e.label))
	}

	return b.String()
}

// renderLegendSwatch draws a tiny version of the POI marker for the legend.
func renderLegendSwatch(poiType string, cx, cy float64) string {
	switch poiType {
	case "sun":
		return fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="5" fill="#EBCB8B"/>`, cx, cy)
	case "planet":
		return fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4" fill="#A3BE8C"/>`, cx, cy)
	case "station":
		return renderHexagon(cx, cy, 5)
	case "asteroid_belt":
		// Small triangle.
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#D08770" opacity="0.8"/>`,
			cx-4, cy+3, cx, cy-4, cx+4, cy+3)
	case "ice_field":
		// Small diamond.
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="#88C0D0" opacity="0.8"/>`,
			cx, cy-4, cx+3, cy, cx, cy+4, cx-3, cy)
	case "gas_cloud":
		// Cluster of small circles.
		return fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="3" fill="#B48EAD" opacity="0.5"/>`+
			`<circle cx="%.1f" cy="%.1f" r="2" fill="#B48EAD" opacity="0.6"/>`,
			cx-2, cy+1, cx+3, cy-1)
	case "relic":
		return renderCompassRose(cx, cy, 5)
	case "wormhole":
		// Mini black hole: dark center + orange ring.
		return fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="6" ry="2.5" fill="none" stroke="#D84315" stroke-width="1.5" opacity="0.85"/>`+
			`<circle cx="%.1f" cy="%.1f" r="2.5" fill="#0a0000"/>`,
			cx, cy, cx, cy)
	case "collapsed_wormhole":
		// Mini broken hole: dark center + yellow dashed ring.
		return fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="6" ry="2.5" fill="none" stroke="#FDD835" stroke-width="1.5" opacity="0.7" stroke-dasharray="3,2"/>`+
			`<circle cx="%.1f" cy="%.1f" r="2.5" fill="#0a0a00"/>`,
			cx, cy, cx, cy)
	case "black_hole":
		// Mini black hole: dark center + orange spiral hint.
		return fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="5" fill="#0a0010"/>`+
			`<circle cx="%.1f" cy="%.1f" r="5" fill="none" stroke="#D84315" stroke-width="1" opacity="0.7"/>`+
			`<circle cx="%.1f" cy="%.1f" r="2" fill="#020005"/>`,
			cx, cy, cx, cy, cx, cy)
	}
	return ""
}
