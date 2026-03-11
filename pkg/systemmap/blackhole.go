package systemmap

import (
	"fmt"
	"math"
	"strings"
)

// blackHoleStreamInfo holds pre-computed data about a star being consumed by a
// black hole, used to render the accretion stream between them.
type blackHoleStreamInfo struct {
	sx, sy    float64 // star SVG coordinates
	starColor string  // star hex color
	starSize  float64 // star radius in SVG pixels
}

// renderBlackHole generates animated SVG for a black hole with a spinning
// multi-arm spiral accretion disk and an optional accretion stream from a
// nearby star being consumed.
func renderBlackHole(poi POI, bx, by float64, stream *blackHoleStreamInfo) string {
	var b strings.Builder

	r := 6.0
	gradientID := fmt.Sprintf("bh-glow-%s", poi.ID)

	b.WriteString(fmt.Sprintf(`<g class="poi-marker"><title>%s</title>`, htmlEscape(poiTitle(poi))))

	// Selection circle.
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#8b95ab" stroke-width="0.5" opacity="0.75" stroke-dasharray="3,3"/>`,
		bx, by, r+8))

	// Dark gravitational glow.
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="url(#%s)"/>`,
		bx, by, r*3.0, gradientID))

	// Accretion stream from star (rendered behind the spiral disk).
	if stream != nil {
		b.WriteString(renderAccretionStream(poi.ID, bx, by, r, stream))
	}

	// Rotating multi-arm spiral accretion disk.
	b.WriteString(renderSpiralDisk(bx, by, r))

	// Event horizon — deep void.
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#020005"/>`,
		bx, by, r*0.45))

	// Photon ring (pulsing bright ring at event horizon edge).
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#E64A19" stroke-width="1.2" opacity="0.8">`,
		bx, by, r*0.5))
	b.WriteString(`<animate attributeName="opacity" values="0.7;0.9;0.7" dur="2s" repeatCount="indefinite"/>`)
	b.WriteString(`</circle>`)

	// Faint outer photon ring.
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#D08770" stroke-width="0.5" opacity="0.3"/>`,
		bx, by, r*0.58))

	b.WriteString(`</g>`)

	return b.String()
}

// renderSpiralDisk draws 4 Archimedean spiral arms rotating slowly clockwise.
func renderSpiralDisk(cx, cy, r float64) string {
	var b strings.Builder

	b.WriteString(`<g>`)
	b.WriteString(fmt.Sprintf(
		`<animateTransform attributeName="transform" type="rotate" from="0 %.1f %.1f" to="360 %.1f %.1f" dur="25s" repeatCount="indefinite"/>`,
		cx, cy, cx, cy))

	numArms := 4
	armColors := []string{"#c0c0c0", "#b0b0b0", "#d0d0d0", "#a8a8a8"}
	armOpacities := []float64{0.70, 0.50, 0.60, 0.45}

	for arm := range numArms {
		offset := float64(arm) * 2.0 * math.Pi / float64(numArms)
		b.WriteString(renderOneSpiralArm(cx, cy, r, offset, armColors[arm], armOpacities[arm]))
	}

	b.WriteString(`</g>`)
	return b.String()
}

// renderOneSpiralArm draws a single spiral arm from near the event horizon
// outward, with a brighter inner-core overlay for visual depth.
func renderOneSpiralArm(cx, cy, r, angleOffset float64, color string, opacity float64) string {
	minR := r * 0.55
	maxR := r * 2.2
	steps := 50
	totalAngle := 2.5 * math.Pi // ~1.25 full turns

	// Build main spiral path.
	var mainPath strings.Builder
	for i := range steps {
		t := float64(i) / float64(steps-1)
		theta := t * totalAngle
		sr := minR + (maxR-minR)*t
		angle := theta + angleOffset
		px := cx + sr*math.Cos(angle)
		py := cy + sr*math.Sin(angle)
		if i == 0 {
			mainPath.WriteString(fmt.Sprintf("M%.1f,%.1f", px, py))
		} else {
			mainPath.WriteString(fmt.Sprintf(" L%.1f,%.1f", px, py))
		}
	}

	var b strings.Builder

	// Main arm stroke.
	b.WriteString(fmt.Sprintf(`<path d="%s" fill="none" stroke="%s" stroke-width="3" opacity="%.2f" stroke-linecap="round"/>`,
		mainPath.String(), color, opacity))

	// Bright inner core (first ~40% of arm).
	innerSteps := steps * 2 / 5
	var innerPath strings.Builder
	for i := range innerSteps {
		t := float64(i) / float64(steps-1)
		theta := t * totalAngle
		sr := minR + (maxR-minR)*t
		angle := theta + angleOffset
		px := cx + sr*math.Cos(angle)
		py := cy + sr*math.Sin(angle)
		if i == 0 {
			innerPath.WriteString(fmt.Sprintf("M%.1f,%.1f", px, py))
		} else {
			innerPath.WriteString(fmt.Sprintf(" L%.1f,%.1f", px, py))
		}
	}
	b.WriteString(fmt.Sprintf(`<path d="%s" fill="none" stroke="#e0e0e0" stroke-width="1.5" opacity="%.2f" stroke-linecap="round"/>`,
		innerPath.String(), opacity*0.6))

	return b.String()
}

// renderAccretionStream draws a curved, tapered gas stream being pulled from a
// star into the black hole. The stream is wide and star-colored at the source,
// narrowing and shifting to orange-red as it approaches the event horizon.
// Animated flowing particles ride along the center of the stream.
func renderAccretionStream(bhID string, bx, by, bhR float64, star *blackHoleStreamInfo) string {
	var b strings.Builder

	sx, sy := star.sx, star.sy

	dx := bx - sx
	dy := by - sy
	dist := math.Hypot(dx, dy)
	if dist < 30 {
		return ""
	}

	// Normalized direction and perpendicular.
	nx := dx / dist
	ny := dy / dist
	perpX := -ny
	perpY := nx

	// Stream start (just outside star surface) and end (near BH edge).
	startX := sx + nx*(star.starSize+3)
	startY := sy + ny*(star.starSize+3)
	endX := bx - nx*(bhR*1.2)
	endY := by - ny*(bhR*1.2)

	// Curve control point — perpendicular offset for a graceful arc.
	curveMag := dist * 0.3
	midX := (startX+endX)/2 + perpX*curveMag
	midY := (startY+endY)/2 + perpY*curveMag

	// Stream tapers: wide at star, narrow at BH.
	wStart := 10.0
	wMid := 5.0
	wEnd := 2.0

	// Upper edge points (offset perpendicular to stream).
	s1x := startX + perpX*wStart/2
	s1y := startY + perpY*wStart/2
	m1x := midX + perpX*wMid/2
	m1y := midY + perpY*wMid/2
	e1x := endX + perpX*wEnd/2
	e1y := endY + perpY*wEnd/2

	// Lower edge points.
	s2x := startX - perpX*wStart/2
	s2y := startY - perpY*wStart/2
	m2x := midX - perpX*wMid/2
	m2y := midY - perpY*wMid/2
	e2x := endX - perpX*wEnd/2
	e2y := endY - perpY*wEnd/2

	gradID := fmt.Sprintf("stream-%s", bhID)

	// Main stream body — filled tapered shape with pulsing opacity.
	b.WriteString(fmt.Sprintf(
		`<path d="M%.1f,%.1f Q%.1f,%.1f %.1f,%.1f L%.1f,%.1f Q%.1f,%.1f %.1f,%.1f Z" fill="url(#%s)" opacity="0.55">`,
		s1x, s1y, m1x, m1y, e1x, e1y,
		e2x, e2y, m2x, m2y, s2x, s2y,
		gradID))
	b.WriteString(`<animate attributeName="opacity" values="0.45;0.65;0.45" dur="4s" repeatCount="indefinite"/>`)
	b.WriteString(`</path>`)

	// Bright center line along the stream.
	b.WriteString(fmt.Sprintf(
		`<path d="M%.1f,%.1f Q%.1f,%.1f %.1f,%.1f" fill="none" stroke="%s" stroke-width="1.5" opacity="0.4">`,
		startX, startY, midX, midY, endX, endY, star.starColor))
	b.WriteString(`<animate attributeName="opacity" values="0.3;0.55;0.3" dur="3s" repeatCount="indefinite"/>`)
	b.WriteString(`</path>`)

	// Flowing particles — animated dash pattern moving toward BH.
	b.WriteString(fmt.Sprintf(
		`<path d="M%.1f,%.1f Q%.1f,%.1f %.1f,%.1f" fill="none" stroke="%s" stroke-width="2" opacity="0.35" stroke-dasharray="3,8">`,
		startX, startY, midX, midY, endX, endY, star.starColor))
	b.WriteString(`<animate attributeName="stroke-dashoffset" from="0" to="-44" dur="2s" repeatCount="indefinite"/>`)
	b.WriteString(`</path>`)

	return b.String()
}

// renderBlackHoleGradients returns SVG gradient definitions needed by a black
// hole POI: a dark radial glow and (when a star is present) a linear gradient
// for the accretion stream that transitions from star color to deep orange-red.
func renderBlackHoleGradients(poi POI, stream *blackHoleStreamInfo, bx, by float64) string {
	var b strings.Builder

	// Dark glow gradient.
	glowID := fmt.Sprintf("bh-glow-%s", poi.ID)
	b.WriteString(fmt.Sprintf(`<radialGradient id="%s">`, glowID))
	b.WriteString(`<stop offset="0%" stop-color="#0a0010" stop-opacity="1"/>`)
	b.WriteString(`<stop offset="20%" stop-color="#1a0020" stop-opacity="0.9"/>`)
	b.WriteString(`<stop offset="45%" stop-color="#BF360C" stop-opacity="0.4"/>`)
	b.WriteString(`<stop offset="70%" stop-color="#D84315" stop-opacity="0.15"/>`)
	b.WriteString(`<stop offset="100%" stop-color="#D08770" stop-opacity="0"/>`)
	b.WriteString(`</radialGradient>`)

	// Accretion stream gradient (star color → orange-red), aligned to
	// the star→BH direction using userSpaceOnUse coordinates.
	if stream != nil {
		streamID := fmt.Sprintf("stream-%s", poi.ID)
		b.WriteString(fmt.Sprintf(
			`<linearGradient id="%s" gradientUnits="userSpaceOnUse" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f">`,
			streamID, stream.sx, stream.sy, bx, by))
		b.WriteString(fmt.Sprintf(`<stop offset="0%%" stop-color="%s" stop-opacity="0.8"/>`, stream.starColor))
		b.WriteString(`<stop offset="40%" stop-color="#E64A19" stop-opacity="0.7"/>`)
		b.WriteString(`<stop offset="100%" stop-color="#BF360C" stop-opacity="0.9"/>`)
		b.WriteString(`</linearGradient>`)
	}

	return b.String()
}
