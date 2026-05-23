package jumpmap

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/hyperjump"
)

// Arrow-map geometry (SVG pixels).
const (
	arrowSize    = 480.0
	arrowCenter  = arrowSize / 2
	arrowInnerR  = 30.0  // arrows start outside the sun glyph
	arrowOuterR  = 175.0 // arrow tip radius
	labelBaseR   = 185.0 // first label ring radius
	labelRingGap = 15.0  // radius added per de-collision ring
	labelMinSep  = 6.0   // degrees; closer labels fan out to outer rings
)

// stationArrow is one resolved station-destination arrow.
type stationArrow struct {
	name     string
	bearing  float64
	distance float64
	direct   bool
}

// RenderStationArrows draws the Sun-centered starburst of headings to every
// station destination. White arrows are direct (reachable) jumps; gray arrows
// are interrupted. names maps system id to display name.
func RenderStationArrows(r hyperjump.OriginReport, names map[string]string) string {
	var arrows []stationArrow
	for _, p := range r.Pairs {
		if !p.DestHasStation {
			continue
		}
		name := names[p.To]
		if name == "" {
			name = p.To
		}
		arrows = append(arrows, stationArrow{name, p.Bearing, p.Distance, p.Reachable})
	}
	sort.Slice(arrows, func(i, j int) bool { return arrows[i].bearing < arrows[j].bearing })

	degs := make([]float64, len(arrows))
	for i, a := range arrows {
		degs[i] = a.bearing
	}
	rings := assignLabelRings(degs, labelMinSep)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="jumpmap-arrows" viewBox="0 0 %g %g" xmlns="http://www.w3.org/2000/svg">`,
		arrowSize, arrowSize)
	b.WriteString(`<defs>` +
		`<marker id="ah-direct" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto"><path d="M0,0 L6,3 L0,6 Z" fill="#f5f7fa"/></marker>` +
		`<marker id="ah-blocked" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto"><path d="M0,0 L6,3 L0,6 Z" fill="#7a8290"/></marker>` +
		`</defs>`)

	// Faint compass ticks.
	for _, t := range []float64{0, 90, 180, 270} {
		x1, y1 := polar(arrowCenter, arrowCenter, arrowOuterR+4, t)
		x2, y2 := polar(arrowCenter, arrowCenter, arrowOuterR+10, t)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#33384a" stroke-width="1"/>`, x1, y1, x2, y2)
	}

	for i, a := range arrows {
		cls, color, marker := "direct", "#f5f7fa", "ah-direct"
		if !a.direct {
			cls, color, marker = "blocked", "#7a8290", "ah-blocked"
		}
		x1, y1 := polar(arrowCenter, arrowCenter, arrowInnerR, a.bearing)
		x2, y2 := polar(arrowCenter, arrowCenter, arrowOuterR, a.bearing)
		fmt.Fprintf(&b, `<line class="jump-arrow %s" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%s" marker-end="url(#%s)"><title>%s — %.1f° — %.0f GU</title></line>`,
			cls, x1, y1, x2, y2, color, arrowStrokeWidth(a.direct), marker, html.EscapeString(a.name), a.bearing, a.distance)

		lr := labelBaseR + float64(rings[i])*labelRingGap
		lx, ly := polar(arrowCenter, arrowCenter, lr, a.bearing)
		anchor := "start"
		if lx < arrowCenter {
			anchor = "end"
		}
		// Leader line for labels pushed to an outer ring.
		if rings[i] > 0 {
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#33384a" stroke-width="0.5"/>`, x2, y2, lx, ly)
		}
		fmt.Fprintf(&b, `<text class="jump-label %s" x="%.1f" y="%.1f" text-anchor="%s" dominant-baseline="middle" fill="%s">%s %.1f°</text>`,
			cls, lx, ly, anchor, color, html.EscapeString(a.name), a.bearing)
	}

	// Sun glyph at center.
	fmt.Fprintf(&b, `<circle cx="%g" cy="%g" r="10" fill="#ffd27f"/><circle cx="%g" cy="%g" r="16" fill="none" stroke="#ffd27f" stroke-opacity="0.4"/>`,
		arrowCenter, arrowCenter, arrowCenter, arrowCenter)

	b.WriteString(`</svg>`)
	return b.String()
}

func arrowStrokeWidth(direct bool) string {
	if direct {
		return "2"
	}
	return "1"
}
