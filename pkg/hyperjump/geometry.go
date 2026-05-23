// Package hyperjump computes Pathfinder Drive (direct hyper-jump) reachability
// between galactic systems: bearings, path interrupters, heading margins, and
// void escape directions. See docs/plans/2026-05-23-hyperjump-analyze-design.md.
package hyperjump

import "math"

// Vec is a 2D galactic coordinate.
type Vec struct{ X, Y float64 }

// Bearing returns the heading in degrees [0,360) from a toward b, where 0°
// points along +X and angle increases counter-clockwise (+Y at 90°).
func Bearing(a, b Vec) float64 {
	deg := math.Atan2(b.Y-a.Y, b.X-a.X) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	return deg
}

// Proj returns the forward projection of rel onto the heading direction.
// Negative means the point is behind the ship.
func Proj(rel Vec, headingDeg float64) float64 {
	rad := headingDeg * math.Pi / 180
	return rel.X*math.Cos(rad) + rel.Y*math.Sin(rad)
}

// SignedPerp returns the signed perpendicular distance of rel from the heading
// ray. Positive is counter-clockwise (left) of the heading.
func SignedPerp(rel Vec, headingDeg float64) float64 {
	rad := headingDeg * math.Pi / 180
	return rel.X*math.Sin(rad) - rel.Y*math.Cos(rad)
}

// ArcHalfWidth returns the half-angle (degrees) of headings that keep a system
// at radius r within margin GU of the ray. Systems within margin GU block a
// full 90° half-plane.
func ArcHalfWidth(r, margin float64) float64 {
	ratio := margin / r
	if ratio >= 1 {
		return 90
	}
	return math.Asin(ratio) * 180 / math.Pi
}
