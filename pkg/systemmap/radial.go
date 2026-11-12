package systemmap

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// radial maps a distance from the star (AU) to pixels on a system map. Since
// v0.698 systems have real scale -- inner planets at hundredths of an AU,
// belts tens of AU out -- so a linear map crushes the inner system into the
// star. The log mapping r_px = k*log(1 + r/r0) is nearly linear inside r0 and
// gives each further power of ten about the same width, with angles kept true.
type radial struct {
	k, r0      float64
	lin        float64 // >0: plain linear pixels per AU (no bodies to fit)
	rmin, rmax float64 // innermost and outermost non-star bodies, AU
	// rulerLimit is how far (px from the star) the ruler may run before
	// leaving the drawing area; 0 means rulerExtend × the outermost body.
	rulerLimit float64
}

// newLinearRadial is the plain linear mapping (the alternate view): the
// outermost non-star body lands at maxPx.
func newLinearRadial(pois []POI, maxPx float64) radial {
	m := newRadial(pois, maxPx)
	if m.rmax > 0 {
		m.lin = maxPx / m.rmax
	}
	return m
}

// logR0Fraction places r0 relative to the innermost body, so the inner system
// keeps its true spacing and room to breathe.
const logR0Fraction = 0.6

// newRadial fits the mapping so the outermost non-star body lands at maxPx.
func newRadial(pois []POI, maxPx float64) radial {
	rmin, rmax := math.Inf(1), 0.0
	for _, p := range pois {
		if p.Type == "sun" {
			continue
		}
		if r := math.Hypot(p.PositionX, p.PositionY); r > 0 {
			rmin, rmax = math.Min(rmin, r), math.Max(rmax, r)
		}
	}
	if rmax == 0 {
		return radial{lin: maxPx} // only a star at the origin: 1 AU spans the map
	}
	r0 := math.Max(rmin*logR0Fraction, 1e-6)
	return radial{k: maxPx / math.Log1p(rmax/r0), r0: r0, rmin: rmin, rmax: rmax}
}

// Px is the pixel radius for a distance of r AU from the star.
func (m radial) Px(r float64) float64 {
	if m.lin > 0 {
		return r * m.lin
	}
	return m.k * math.Log1p(r/m.r0)
}

// Point places a body at game (x, y) around the map center: true angle,
// compressed distance. SVG y is inverted.
func (m radial) Point(cx, cy, x, y float64) (float64, float64) {
	r := math.Hypot(x, y)
	if r == 0 {
		return cx, cy
	}
	s := m.Px(r) / r
	return cx + x*s, cy - y*s
}

// rulerExtend is how far past the outermost body a log ruler may run to reach
// the next power of ten, as a multiple of its normal length.
const rulerExtend = 1.4

// rulerEnd is where the ruler stops, in AU: the outermost body, or on the log
// scale the next power of ten when that is only a little farther out (wide
// pages have room either side), so the long run past the last label ends on
// a labeled tick.
func (m radial) rulerEnd() float64 {
	if m.lin > 0 || m.rmax == 0 {
		return m.rmax
	}
	next := math.Pow(10, math.Ceil(math.Log10(m.rmax)))
	limit := m.rulerLimit
	if limit == 0 {
		limit = rulerExtend * m.Px(m.rmax)
	}
	if m.Px(next) <= limit {
		return next
	}
	return m.rmax
}

// rulerTick is one tick on the distance ruler.
type rulerTick struct {
	au    float64
	px    float64 // distance from the star in pixels
	major bool    // a power of ten
}

// ticks lists 1-9 x each power of ten from below the innermost body out to the
// outermost, skipping any that would sit on the star.
func (m radial) ticks() []rulerTick {
	if m.rmax == 0 {
		return nil
	}
	if m.lin > 0 {
		// Linear: evenly spaced round steps, every one labeled.
		step := niceStep(m.rmax)
		var out []rulerTick
		for i := 1; float64(i)*step <= m.rmax*1.0001; i++ {
			v := float64(i) * step
			out = append(out, rulerTick{au: v, px: m.Px(v), major: true})
		}
		return out
	}
	end := m.rulerEnd()
	var out []rulerTick
	for d := math.Pow(10, math.Floor(math.Log10(m.rmin))-1); d <= end*1.0001; d *= 10 {
		for i := 1; i <= 9; i++ {
			v := d * float64(i)
			if v > end*1.0001 {
				break
			}
			if px := m.Px(v); px >= 3 {
				out = append(out, rulerTick{au: v, px: px, major: i == 1})
			}
		}
	}
	return out
}

// niceStep picks a round tick spacing (1, 2 or 5 x a power of ten) giving
// about six to ten ticks out to rmax.
func niceStep(rmax float64) float64 {
	raw := rmax / 8
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5, 10} {
		if s := m * p; s >= raw*0.75 {
			return s
		}
	}
	return 10 * p
}

// fmtAUTick formats a ruler label: 0.01, 0.1, 1, 10, 100.
func fmtAUTick(v float64) string {
	return strconv.FormatFloat(v, 'g', 6, 64)
}

// rulerSVG draws the distance ruler under the map: 0 directly under the star,
// running out both ways to the outermost body through the same log mapping, so
// a tick's distance from 0 equals an orbit's radius. Powers of ten are gold
// and labeled; labels that would overlap are skipped.
func (m radial) rulerSVG(cx, starY, y float64) string {
	ts := m.ticks()
	if len(ts) == 0 {
		return ""
	}
	span := m.Px(m.rulerEnd())
	var b strings.Builder
	b.WriteString(`<g class="map-ruler">`)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#ebcb8b" stroke-width="0.6" opacity="0.35" stroke-dasharray="2,3"/>`, cx, starY, cx, y)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#cfd8e3" stroke-width="0.8"/>`, cx-span, y, cx+span, y)
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="2.5" fill="#ebcb8b"/>`, cx, y)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" class="map-ruler-label">0</text>`, cx, y+13)
	lastLabel := 0.0 // pixel offset of the last label drawn on each side
	for _, t := range ts {
		h, stroke, width := 2.5, "#8b95ab", 0.6
		if t.major {
			h, stroke, width = 5, "#ebcb8b", 1.2
		}
		for _, sgn := range []float64{1, -1} {
			x := cx + sgn*t.px
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f"/>`, x, y-h, x, y+h, stroke, width)
		}
		if t.major && t.px > 14 && t.px-lastLabel > 22 {
			for _, sgn := range []float64{1, -1} {
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" class="map-ruler-label">%s</text>`, cx+sgn*t.px, y+13, fmtAUTick(t.au))
			}
			lastLabel = t.px
		}
	}
	kind := "log"
	if m.lin > 0 {
		kind = "linear"
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="start" class="map-ruler-note">AU from the star (%s scale)</text>`, cx-span, y-8, kind)
	b.WriteString(`</g>`)
	return b.String()
}

var (
	svgIDRe   = regexp.MustCompile(`\bid="([^"]+)"`)
	svgURLRe  = regexp.MustCompile(`url\(#([^)]+)\)`)
	svgHrefRe = regexp.MustCompile(`href="#([^"]+)"`)
)

// PrefixSVGIDs renames every id in an SVG fragment, and every in-document
// reference to one, so two maps of the same system can share a page: a
// gradient defined inside a hidden copy does not paint the visible one.
func PrefixSVGIDs(svg, prefix string) string {
	svg = svgIDRe.ReplaceAllString(svg, `id="`+prefix+`$1"`)
	svg = svgURLRe.ReplaceAllString(svg, `url(#`+prefix+`$1)`)
	return svgHrefRe.ReplaceAllString(svg, `href="#`+prefix+`$1"`)
}
