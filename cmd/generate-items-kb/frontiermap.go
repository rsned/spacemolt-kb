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

// Frontier systems lie in separate regions thousands of units apart, so one
// fitted mini-map leaves them as two cramped specks. The Systems index draws
// them instead as a small charted-galaxy overview with one zoomed callout per
// region, joined to its spot on the overview by a leader line.

const (
	regionLink = 1000.0 // systems closer than this share a region
	oldRimAU   = 7579.0 // farthest charted system from the centre
	rimShare   = 0.85   // regions this far out are "rim" regions
)

// frontierRegion is one connected group of frontier systems.
type frontierRegion struct {
	Name    string
	Systems []*System
}

// frontierRegions groups the positioned uncharted systems by single-link
// distance and names each group by where it sits (in-game orientation: +y is
// down). Larger regions come first.
func frontierRegions(all []*System) []frontierRegion {
	var fs []*System
	for _, s := range all {
		if s.Uncharted && !s.PositionUnknown {
			fs = append(fs, s)
		}
	}
	parent := make([]int, len(fs))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			i = parent[i]
		}
		return i
	}
	for i := range fs {
		for j := i + 1; j < len(fs); j++ {
			if math.Hypot(fs[i].PositionX-fs[j].PositionX, fs[i].PositionY-fs[j].PositionY) < regionLink {
				parent[find(i)] = find(j)
			}
		}
	}
	groups := map[int][]*System{}
	for i, s := range fs {
		groups[find(i)] = append(groups[find(i)], s)
	}
	var out []frontierRegion
	used := map[string]int{}
	for _, g := range groups {
		slices.SortFunc(g, func(a, b *System) int { return cmp.Compare(a.Name, b.Name) })
		name := regionName(g, all)
		used[name]++
		if used[name] > 1 {
			name = fmt.Sprintf("%s %d", name, used[name])
		}
		out = append(out, frontierRegion{Name: name, Systems: g})
	}
	slices.SortFunc(out, func(a, b frontierRegion) int {
		return cmp.Or(cmp.Compare(len(b.Systems), len(a.Systems)), cmp.Compare(a.Name, b.Name))
	})
	return out
}

// regionName is "<compass> rim" for a region out near the rim, or
// "Inner <empire> space" (after the nearest charted system's empire) inside.
func regionName(g []*System, all []*System) string {
	cx, cy := centroid(g)
	if math.Hypot(cx, cy) >= rimShare*oldRimAU {
		dirs := []string{"East", "North-east", "North", "North-west", "West", "South-west", "South", "South-east"}
		a := math.Mod(math.Atan2(-cy, cx)*180/math.Pi+360+22.5, 360)
		return dirs[int(a/45)] + " rim"
	}
	var near *System
	best := math.Inf(1)
	for _, s := range all {
		if s.Uncharted || s.Empire == "" {
			continue
		}
		if d := math.Hypot(s.PositionX-cx, s.PositionY-cy); d < best {
			best, near = d, s
		}
	}
	if near == nil {
		return "Inner space"
	}
	return "Inner " + titleCase(near.Empire) + " space"
}

func centroid(g []*System) (float64, float64) {
	var x, y float64
	for _, s := range g {
		x += s.PositionX
		y += s.PositionY
	}
	return x / float64(len(g)), y / float64(len(g))
}

// renderFrontierCallouts draws the overview with a zoomed callout per region.
func renderFrontierCallouts(all []*System) string {
	regions := frontierRegions(all)
	if len(regions) == 0 {
		return ""
	}
	const (
		w       = 1000.0
		panelW  = 300.0
		gap     = 20.0
		ovR     = 150.0 // overview radius, px, for the old rim
		ovScale = ovR / oldRimAU
	)
	// Panels go on the side their region sits, stacked top to bottom.
	type slot struct {
		r        frontierRegion
		left     bool
		h, x, y  float64
		cyRegion float64
	}
	var left, right []*slot
	for _, r := range regions {
		cx, cy := centroid(r.Systems)
		h := 260.0
		if len(r.Systems) == 1 {
			h = 130
		}
		s := &slot{r: r, left: cx < 0, h: h, cyRegion: cy}
		if s.left {
			left = append(left, s)
		} else {
			right = append(right, s)
		}
	}
	byY := func(a, b *slot) int { return cmp.Compare(a.cyRegion, b.cyRegion) }
	slices.SortFunc(left, byY)
	slices.SortFunc(right, byY)
	stack := func(col []*slot, x float64) float64 {
		y := gap
		for _, s := range col {
			s.x, s.y = x, y
			y += s.h + gap
		}
		return y
	}
	h := max(stack(left, gap), stack(right, w-panelW-gap), 2*ovR+80)
	gx, gy := w/2, h/2
	ov := func(x, y float64) (float64, float64) { return gx + x*ovScale, gy + y*ovScale }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="fc-map" viewBox="0 0 %.0f %.0f" role="img" aria-label="Frontier regions on the galaxy overview">`, w, h)
	b.WriteString(`<style>.fc-map{width:100%;height:auto;background:#0b0f17;border-radius:6px}.fc-map a:hover text{fill:#fff}</style>`)
	fmt.Fprintf(&b, `<circle cx="%.0f" cy="%.0f" r="%.0f" fill="none" stroke="#ffffff26" stroke-dasharray="3 4"/>`, gx, gy, ovR)
	for _, s := range all {
		if s.Uncharted || s.PositionUnknown {
			continue
		}
		x, y := ov(s.PositionX, s.PositionY)
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="1.1" fill="#8b95ab" opacity=".55"/>`, x, y)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="#9fb0c2" font-size="11" text-anchor="middle">charted galaxy · dashed ring = old rim</text>`, gx, gy+ovR+22)

	for _, s := range append(left, right...) {
		var xs, ys []float64
		for _, sys := range s.r.Systems {
			xs = append(xs, sys.PositionX)
			ys = append(ys, sys.PositionY)
		}
		const pad = 250.0
		x0, x1 := slices.Min(xs)-pad, slices.Max(xs)+pad
		y0, y1 := slices.Min(ys)-pad, slices.Max(ys)+pad
		bx0, by0 := ov(x0, y0)
		bx1, by1 := ov(x1, y1)
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="%s" stroke-width="1.2"/>`,
			bx0, by0, math.Max(bx1-bx0, 3), math.Max(by1-by0, 3), frontier.Color)
		// Leader line from the box's near edge to the panel's facing edge.
		lx, px := bx0, s.x+panelW
		if !s.left {
			lx, px = bx1, s.x
		}
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1" opacity=".7"/>`,
			lx, (by0+by1)/2, px, s.y+s.h/2, frontier.Color)

		fmt.Fprintf(&b, `<g class="fc-panel"><rect x="%.0f" y="%.0f" width="%.0f" height="%.0f" rx="6" fill="#111827" stroke="%s99"/>`,
			s.x, s.y, panelW, s.h, frontier.Color)
		n := len(s.r.Systems)
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" fill="%s" font-size="12" font-weight="700">%s <tspan fill="#9fb0c2" font-weight="400">· %d system%s</tspan></text>`,
			s.x+10, s.y+18, frontier.Color, htmltpl.HTMLEscapeString(s.r.Name), n, map[bool]string{true: "", false: "s"}[n == 1])
		sc := 0.0
		if n > 1 {
			sc = math.Min((panelW-90)/(x1-x0), (s.h-50)/(y1-y0))
		}
		ccx, ccy := (x0+x1)/2, (y0+y1)/2
		pp := func(sys *System) (float64, float64) {
			return s.x + panelW/2 + (sys.PositionX-ccx)*sc, s.y + s.h/2 + 10 + (sys.PositionY-ccy)*sc
		}
		in := map[string]*System{}
		for _, sys := range s.r.Systems {
			in[sys.ID] = sys
		}
		for _, sys := range s.r.Systems {
			for _, c := range sys.Connections {
				if to, ok := in[c.SystemID]; ok && sys.ID < c.SystemID {
					ax, ay := pp(sys)
					bx, by := pp(to)
					fmt.Fprintf(&b, `<line class="fc-lane" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1" opacity=".55"/>`, ax, ay, bx, by, frontier.Color)
				}
			}
		}
		// Labels: nudge down any that would sit on top of an earlier one.
		var placed [][2]float64
		for _, sys := range s.r.Systems {
			x, y := pp(sys)
			ly := y + 3.5
			for _, p := range placed {
				if math.Abs(p[0]-x) < 70 && math.Abs(p[1]-ly) < 11 {
					ly = p[1] + 11
				}
			}
			placed = append(placed, [2]float64{x, ly})
			fmt.Fprintf(&b, `<a href="%s/"><circle cx="%.1f" cy="%.1f" r="3.5" fill="%s"><title>%s</title></circle><text x="%.1f" y="%.1f" fill="#d8dee9" font-size="10">%s</text></a>`,
				sys.ID, x, y, frontier.Color, htmltpl.HTMLEscapeString(sys.Name), x+6, ly, htmltpl.HTMLEscapeString(sys.Name))
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
}
