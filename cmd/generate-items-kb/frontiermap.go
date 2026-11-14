package main

import (
	"cmp"
	"fmt"
	htmltpl "html/template"
	"math"
	"slices"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/frontier"
)

// Frontier systems ring the charted galaxy, so one fitted mini-map crushes
// them and region callouts turn into long crowded arcs. The Systems index
// draws them instead as a 3×3 compass grid: the charted galaxy in the
// middle, with the inner frontier systems marked on it, and one zoomed
// panel per 45° wedge around it, each sitting on its own compass side.

const (
	oldRimAU = 7579.0 // farthest charted system from the centre
	rimShare = 0.85   // systems this far out belong to a rim wedge
)

// compassNames run counter-clockwise from east in in-game orientation
// (+y is down, so north is -y).
var compassNames = [8]string{"East", "North-east", "North", "North-west", "West", "South-west", "South", "South-east"}

// compassCell is each wedge's (column, row) in the 3×3 grid.
var compassCell = [8][2]int{{2, 1}, {2, 0}, {1, 0}, {0, 0}, {0, 1}, {0, 2}, {1, 2}, {2, 2}}

// compassWedge is the wedge index (into compassNames) a map point falls in.
func compassWedge(x, y float64) int {
	a := math.Mod(math.Atan2(-y, x)*180/math.Pi+360+22.5, 360)
	return int(a/45) % 8
}

// frontierWedges splits the positioned uncharted systems into the eight rim
// wedges and the inner systems (inside rimShare of the old rim), each list
// sorted by name.
func frontierWedges(all []*System) (wedges [8][]*System, inner []*System) {
	for _, s := range all {
		if !s.Uncharted || s.PositionUnknown {
			continue
		}
		if math.Hypot(s.PositionX, s.PositionY) < rimShare*oldRimAU {
			inner = append(inner, s)
			continue
		}
		w := compassWedge(s.PositionX, s.PositionY)
		wedges[w] = append(wedges[w], s)
	}
	byName := func(a, b *System) int { return cmp.Compare(a.Name, b.Name) }
	for _, w := range wedges {
		slices.SortFunc(w, byName)
	}
	slices.SortFunc(inner, byName)
	return wedges, inner
}

// writeFrontierDot draws one frontier system: a filled, linked dot, or for a
// system seen only on another player's map a hollow dot with a dimmer label
// and no link (it has no page).
func writeFrontierDot(b *strings.Builder, s *System, x, y, lx, ly float64, class string) {
	name := htmltpl.HTMLEscapeString(s.Name)
	if s.Sighted {
		fmt.Fprintf(b, `<g class="fc-sighted"><circle cx="%.1f" cy="%.1f" r="3" fill="none" stroke="%s" stroke-width="1.3"><title>%s (seen on another player's map, not yet scanned)</title></circle><text x="%.1f" y="%.1f" fill="#8b95ab" font-size="10" font-style="italic">%s</text></g>`,
			x, y, frontier.Color, name, lx, ly, name)
		return
	}
	cls := ""
	if class != "" {
		cls = ` class="` + class + `"`
	}
	fmt.Fprintf(b, `<a href="%s/"%s><circle cx="%.1f" cy="%.1f" r="3.5" fill="%s"><title>%s</title></circle><text x="%.1f" y="%.1f" fill="#d8dee9" font-size="10">%s</text></a>`,
		s.ID, cls, x, y, frontier.Color, name, lx, ly, name)
}

// renderFrontierCallouts draws the compass grid.
func renderFrontierCallouts(all []*System) string {
	wedges, inner := frontierWedges(all)
	if len(inner) == 0 && !slices.ContainsFunc(wedges[:], func(w []*System) bool { return len(w) > 0 }) {
		return ""
	}
	const (
		w     = 1000.0
		gap   = 16.0
		cellW = (w - 4*gap) / 3
		cellH = 280.0
	)
	h := 3*cellH + 4*gap
	cell := func(col, row int) (float64, float64) {
		return gap + float64(col)*(cellW+gap), gap + float64(row)*(cellH+gap)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="fc-map" viewBox="0 0 %.0f %.0f" role="img" aria-label="Frontier systems by compass direction around the charted galaxy">`, w, h)
	b.WriteString(`<style>.fc-map{width:100%;height:auto;background:#0b0f17;border-radius:6px}.fc-map a:hover text{fill:#fff}</style>`)

	// Centre: the charted galaxy, wedge boundaries, every frontier system
	// as a dot, and the inner ones labelled.
	cx0, cy0 := cell(1, 1)
	gx, gy := cx0+cellW/2, cy0+cellH/2-8
	ovR := math.Min(cellW, cellH)/2 - 22
	sc := ovR / oldRimAU
	ov := func(x, y float64) (float64, float64) { return gx + x*sc, gy + y*sc }
	for i := range 8 {
		a := (22.5 + 45*float64(i)) * math.Pi / 180
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#ffffff14"/>`,
			gx+0.25*ovR*math.Cos(a), gy-0.25*ovR*math.Sin(a), gx+1.12*ovR*math.Cos(a), gy-1.12*ovR*math.Sin(a))
	}
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="#ffffff26" stroke-dasharray="3 4"/>`, gx, gy, ovR)
	for _, s := range all {
		if s.PositionUnknown || (s.Uncharted && math.Hypot(s.PositionX, s.PositionY) < rimShare*oldRimAU) {
			continue
		}
		x, y := ov(s.PositionX, s.PositionY)
		if s.Uncharted {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="1.6" fill="%s"/>`, x, y, frontier.Color)
			continue
		}
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="1" fill="#8b95ab" opacity=".55"/>`, x, y)
	}
	for _, s := range inner {
		x, y := ov(s.PositionX, s.PositionY)
		writeFrontierDot(&b, s, x, y, x+6, y+3.5, "fc-inner")
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="#9fb0c2" font-size="11" text-anchor="middle">charted galaxy · dashed ring = old rim</text>`, gx, cy0+cellH-6)

	for i, sys := range wedges {
		px, py := cell(compassCell[i][0], compassCell[i][1])
		fmt.Fprintf(&b, `<g class="fc-panel"><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="6" fill="#111827" stroke="%s99"/>`,
			px, py, cellW, cellH, frontier.Color)
		n := len(sys)
		count := "no scans yet"
		if n > 0 {
			count = fmt.Sprintf("%d system%s", n, map[bool]string{true: "", false: "s"}[n == 1])
			if k := countSighted(sys); k > 0 {
				count += fmt.Sprintf(", %d seen on other maps", k)
			}
		}
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s" font-size="12" font-weight="700">%s <tspan fill="#9fb0c2" font-weight="400">· %s</tspan></text>`,
			px+10, py+18, frontier.Color, compassNames[i], count)
		if n == 0 {
			fmt.Fprintf(&b, `<text class="fc-empty" x="%.1f" y="%.1f" fill="#5b6678" font-size="11" text-anchor="middle">No scans yet</text></g>`,
				px+cellW/2, py+cellH/2+6)
			continue
		}
		writeWedgePanel(&b, sys, px, py, cellW, cellH)
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// writeWedgePanel draws one wedge's systems, fitted north-up into the panel
// below its title, with the lanes between them.
func writeWedgePanel(b *strings.Builder, sys []*System, px, py, pw, ph float64) {
	const (
		top    = 30.0 // title band
		padX   = 70.0 // room for labels on both sides
		padY   = 22.0
		labelH = 11.0
	)
	var xs, ys []float64
	for _, s := range sys {
		xs = append(xs, s.PositionX)
		ys = append(ys, s.PositionY)
	}
	x0, x1 := slices.Min(xs), slices.Max(xs)
	y0, y1 := slices.Min(ys), slices.Max(ys)
	aw, ah := pw-2*padX, ph-top-2*padY
	// Rim arcs are long and thin, so each axis is scaled to fill the panel,
	// stretched up to maxStretch over the other: a schematic, not a chart.
	const maxStretch = 3.0
	sx, sy := aw/math.Max(x1-x0, 1), ah/math.Max(y1-y0, 1)
	sx, sy = math.Min(sx, sy*maxStretch), math.Min(sy, sx*maxStretch)
	ccx, ccy := (x0+x1)/2, (y0+y1)/2
	mx, my := px+pw/2, py+top+padY+ah/2
	pp := func(s *System) (float64, float64) {
		return mx + (s.PositionX-ccx)*sx, my + (s.PositionY-ccy)*sy
	}
	in := map[string]*System{}
	for _, s := range sys {
		in[s.ID] = s
	}
	for _, s := range sys {
		for _, c := range s.Connections {
			if to, ok := in[c.SystemID]; ok && s.ID < c.SystemID {
				ax, ay := pp(s)
				bx, by := pp(to)
				fmt.Fprintf(b, `<line class="fc-lane" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1" opacity=".55"/>`, ax, ay, bx, by, frontier.Color)
			}
		}
	}
	// Each label takes the first spot, of right, left, above and below its
	// dot (the side facing the panel centre first), that stays inside the
	// panel and clears every dot and earlier label; failing that, it steps
	// down a line at a time on the inward side.
	type box struct{ x0, x1, y0, y1 float64 }
	const charW = 5.6 // approximate glyph width at font-size 10
	var taken []box
	for _, s := range sys {
		x, y := pp(s)
		taken = append(taken, box{x - 4, x + 4, y - 4, y + 4})
	}
	free := func(c box, own int) bool {
		if c.x0 < px+2 || c.x1 > px+pw-2 || c.y0 < py+top || c.y1 > py+ph-2 {
			return false
		}
		for k, t := range taken {
			if k != own && c.x0 < t.x1 && t.x0 < c.x1 && c.y0 < t.y1 && t.y0 < c.y1 {
				return false
			}
		}
		return true
	}
	order := slices.Clone(sys)
	slices.SortStableFunc(order, func(a, c *System) int { return cmp.Compare(a.PositionY, c.PositionY) })
	idx := map[string]int{}
	for k, s := range sys {
		idx[s.ID] = k
	}
	for _, s := range order {
		x, y := pp(s)
		w := charW * float64(len([]rune(s.Name)))
		// Candidate boxes, with the text baseline at y1-2.5.
		right := box{x + 6, x + 6 + w, y - 5, y + 6}
		left := box{x - 6 - w, x - 6, y - 5, y + 6}
		above := box{x - w/2, x + w/2, y - 16, y - 5}
		below := box{x - w/2, x + w/2, y + 5, y + 16}
		cands := []box{right, left, above, below}
		if x > mx {
			cands = []box{left, right, above, below}
		}
		l, ok := box{}, false
		for _, c := range cands {
			if free(c, idx[s.ID]) {
				l, ok = c, true
				break
			}
		}
		if !ok {
			l = cands[0]
			for !free(l, idx[s.ID]) && l.y1 < py+ph-2 {
				l.y0 += labelH
				l.y1 += labelH
			}
		}
		taken = append(taken, l)
		lx, ly := l.x0, l.y1-2.5
		writeFrontierDot(b, s, x, y, lx, ly, "")
	}
}

// countSighted counts the systems known only from other players' maps.
func countSighted(sys []*System) int {
	n := 0
	for _, s := range sys {
		if s.Sighted {
			n++
		}
	}
	return n
}
