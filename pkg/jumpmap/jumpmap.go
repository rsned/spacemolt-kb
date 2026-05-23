// Package jumpmap renders SVG visualizations of Pathfinder Drive (direct
// hyper-jump) analysis for a single origin system: a starburst of headings to
// station destinations and a 360-degree coverage/void wheel.
//
// Both use the engine convention: 0 degrees points along +X (right) and angle
// increases counter-clockwise. SVG's y axis is flipped so +Y points up.
package jumpmap

import "math"

// polar converts a center, radius, and heading (degrees, engine convention) into
// SVG coordinates, flipping y so +Y points up.
func polar(cx, cy, r, deg float64) (x, y float64) {
	rad := deg * math.Pi / 180
	return cx + r*math.Cos(rad), cy - r*math.Sin(rad)
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
