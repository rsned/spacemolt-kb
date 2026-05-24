package jumpmap

import (
	"fmt"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/hyperjump"
)

// Coverage-wheel geometry (SVG pixels).
const (
	wheelSize    = 480.0
	wheelCenter  = wheelSize / 2
	wheelR       = 175.0 // outer radius of the disk
	wheelLabelR  = 192.0 // radius for boundary heading labels
	maxGapLabels = 12    // label every gap edge only when gaps are few
	minLabelGap  = 1.0   // otherwise label only gaps at least this wide (deg)
)

// RenderCoverageWheel draws the 360-degree heading wheel for an origin. The disk
// is "covered" (headings that intersect some system) with void-escape gaps
// overlaid in a bright color and their boundary headings labeled.
func RenderCoverageWheel(r hyperjump.OriginReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="jumpmap-wheel" viewBox="0 0 %g %g" xmlns="http://www.w3.org/2000/svg">`,
		wheelSize, wheelSize)

	// Covered disk; gap wedges are drawn over it next.
	fmt.Fprintf(&b, `<circle class="wheel-covered" cx="%g" cy="%g" r="%g" fill="#2b3856"/>`,
		wheelCenter, wheelCenter, wheelR)

	// Gap wedges (void escape directions).
	labelEvery := len(r.Gaps) <= maxGapLabels
	for _, g := range r.Gaps {
		fmt.Fprintf(&b, `<polygon class="wheel-gap" points="%s" fill="#ffcf4d"/>`,
			wedgePoints(wheelCenter, wheelCenter, wheelR, g.StartDeg, g.EndDeg))
		if labelEvery || g.WidthDeg >= minLabelGap {
			wheelEdgeLabel(&b, g.StartDeg)
			wheelEdgeLabel(&b, g.EndDeg)
		}
	}

	// Faint compass ticks.
	for _, t := range []float64{0, 90, 180, 270} {
		x1, y1 := polar(wheelCenter, wheelCenter, wheelR, t)
		x2, y2 := polar(wheelCenter, wheelCenter, wheelR+8, t)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#55607a" stroke-width="1"/>`, x1, y1, x2, y2)
	}

	// Center summary text. Cap the blocked figure so a system with a (possibly
	// sub-0.1%) gap never reads "100.0% blocked"; derive open from it so they agree.
	blocked := DisplayBlockedPct(r.CoveragePct*100, len(r.Gaps))
	open := 100 - blocked
	fmt.Fprintf(&b, `<text class="wheel-center" x="%g" y="%g" text-anchor="middle" dominant-baseline="middle" fill="#cdd3e0" font-size="18">%.1f%% blocked</text>`,
		wheelCenter, wheelCenter-8, blocked)
	if len(r.Gaps) == 0 {
		fmt.Fprintf(&b, `<text class="wheel-sub" x="%g" y="%g" text-anchor="middle" dominant-baseline="middle" fill="#ff8a8a" font-size="13">No escape</text>`,
			wheelCenter, wheelCenter+14)
	} else {
		fmt.Fprintf(&b, `<text class="wheel-sub" x="%g" y="%g" text-anchor="middle" dominant-baseline="middle" fill="#ffcf4d" font-size="13">%.1f%% open · %d gaps</text>`,
			wheelCenter, wheelCenter+14, open, len(r.Gaps))
	}

	b.WriteString(`</svg>`)
	return b.String()
}

// wheelEdgeLabel writes a heading label just outside the wheel at the given angle.
func wheelEdgeLabel(b *strings.Builder, deg float64) {
	x, y := polar(wheelCenter, wheelCenter, wheelLabelR, deg)
	anchor := "start"
	if x < wheelCenter {
		anchor = "end"
	}
	fmt.Fprintf(b, `<text class="wheel-label" x="%.1f" y="%.1f" text-anchor="%s" dominant-baseline="middle" fill="#9aa3b8" font-size="10">%.2f°</text>`,
		x, y, anchor, deg)
}

// wedgePoints returns the polygon point list for a pie wedge from startDeg to
// endDeg (counter-clockwise), approximated in 1-degree steps.
func wedgePoints(cx, cy, r, startDeg, endDeg float64) string {
	var pts strings.Builder
	fmt.Fprintf(&pts, "%.1f,%.1f ", cx, cy)
	for d := startDeg; d < endDeg; d += 1 {
		x, y := polar(cx, cy, r, d)
		fmt.Fprintf(&pts, "%.1f,%.1f ", x, y)
	}
	x, y := polar(cx, cy, r, endDeg)
	fmt.Fprintf(&pts, "%.1f,%.1f", x, y)
	return pts.String()
}
