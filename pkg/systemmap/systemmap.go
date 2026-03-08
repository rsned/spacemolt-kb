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
		b.WriteString(`</radialGradient></defs>`)

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
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, cx, cy))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="24" fill="url(#sun-glow)"/>`, cx, cy))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="10" fill="#EBCB8B"/>`, cx, cy))
				b.WriteString(`</g>`)
				// Sun label always below.
				labels = append(labels, labelInfo{x: cx, y: cy + 28, name: poi.Name, above: false})

			case "planet":
				b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="20" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`, px, py))
				b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="5" fill="#A3BE8C"/>`, px, py))
				b.WriteString(`</g>`)
				if labelAbove {
					labels = append(labels, labelInfo{x: px, y: py - 8, name: poi.Name, above: true})
				} else {
					labels = append(labels, labelInfo{x: px, y: py + 16, name: poi.Name, above: false})
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
	dy := target.PositionY - sys.PositionY
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
