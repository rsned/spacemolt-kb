package shipglyph

import (
	"fmt"
	"math"
)

// AppendageShape is a closed polygon for one hull appendage, on one side.
type AppendageShape struct {
	// ID is the stable SVG element ID, e.g. "ap-wing-1p".
	ID string
	// Kind mirrors the descriptor's Appendage.Kind.
	Kind string
	// Poly is the closed outline in glyph space.
	Poly []Point
}

// AppendageShapes builds polygons for every appendage in the descriptor. A
// "both" appendage yields two shapes, one per side.
func AppendageShapes(d Descriptor) []AppendageShape {
	var out []AppendageShape
	for i, a := range d.Appendages {
		for _, side := range sidesOf(a.Side) {
			suffix := "s"
			if side < 0 {
				suffix = "p"
			}
			out = append(out, AppendageShape{
				ID:   fmt.Sprintf("ap-%s-%d%s", a.Kind, i+1, suffix),
				Kind: a.Kind,
				Poly: appendagePoly(d, a, side),
			})
		}
	}
	return out
}

// sidesOf expands an Appendage.Side value into the sides it occupies.
func sidesOf(side string) []float64 {
	switch side {
	case "port":
		return []float64{-1}
	case "starboard":
		return []float64{1}
	default:
		return []float64{1, -1}
	}
}

// appendagePoly returns the closed outline of one appendage on one side. Every
// kind is a quadrilateral rooted on the hull edge; only the outboard corners
// differ, which keeps the shapes readable at glyph size.
func appendagePoly(d Descriptor, a Appendage, side float64) []Point {
	root := hullHalfWidth(d, a.At)
	span := a.Span
	if span <= 0 {
		span = 0.2
	}

	// Sweep converts to how far aft the outboard edge trails the root.
	trail := span * math.Tan(a.Sweep*math.Pi/180)

	var chordFwd, chordAft, tipFwd, tipAft float64
	switch a.Kind {
	case "wing":
		chordFwd, chordAft = 0.06, 0.14
		tipFwd, tipAft = 0.02, 0.05
	case "nacelle":
		chordFwd, chordAft = 0.10, 0.10
		tipFwd, tipAft = 0.08, 0.08
	case "sponson":
		chordFwd, chordAft = 0.05, 0.05
		tipFwd, tipAft = 0.04, 0.04
	case "drone_rack":
		chordFwd, chordAft = 0.16, 0.16
		tipFwd, tipAft = 0.14, 0.14
	case "tow_arm", "boom", "outrigger":
		chordFwd, chordAft = 0.03, 0.03
		tipFwd, tipAft = 0.02, 0.02
	case "antenna_mast":
		chordFwd, chordAft = 0.02, 0.02
		tipFwd, tipAft = 0.005, 0.005
	default:
		chordFwd, chordAft = 0.06, 0.06
		tipFwd, tipAft = 0.04, 0.04
	}

	inner := root * 0.9 * side
	outer := (root + span) * side

	return []Point{
		{X: a.At - chordFwd, Y: inner},
		{X: a.At + chordAft, Y: inner},
		{X: a.At + trail + tipAft, Y: outer},
		{X: a.At + trail - tipFwd, Y: outer},
	}
}
