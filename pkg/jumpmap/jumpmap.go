// Package jumpmap renders SVG visualizations of Pathfinder Drive (direct
// hyper-jump) analysis for a single origin system: a starburst of headings to
// station destinations and a 360-degree coverage/void wheel.
//
// Both use the server convention: 0 degrees points along +X (right) and +Y is
// rendered DOWN the page (matching the galaxy/empire maps and the game client),
// so on screen the angle appears to increase clockwise.
package jumpmap

import "math"

// polar converts a center, radius, and heading (degrees) into SVG coordinates.
// +Y renders down the page, so the SVG y term adds r*sin(angle).
func polar(cx, cy, r, deg float64) (x, y float64) {
	rad := deg * math.Pi / 180
	return cx + r*math.Cos(rad), cy + r*math.Sin(rad)
}

// assignLabelRings places each heading (given in ascending order) on the lowest
// concentric label ring whose previous label on that ring is at least minSep
// degrees away, so crowded labels fan out instead of overlapping.
func assignLabelRings(degsAsc []float64, minSep float64) []int {
	rings := make([]int, len(degsAsc))
	var lastOnRing []float64 // last heading placed on each ring
	for i, d := range degsAsc {
		placed := false
		for ring := range lastOnRing {
			if d-lastOnRing[ring] >= minSep {
				rings[i] = ring
				lastOnRing[ring] = d
				placed = true
				break
			}
		}
		if !placed {
			rings[i] = len(lastOnRing)
			lastOnRing = append(lastOnRing, d)
		}
	}
	return rings
}
