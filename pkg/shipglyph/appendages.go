package shipglyph

import (
	"fmt"
	"math"
	"strings"
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

// safeKind reduces an appendage kind to characters valid in an SVG id and a
// CSS class. Kinds arrive from hand-authored overlay JSON, which is trusted to
// be well-intentioned but not to be syntactically careful, and the result is
// interpolated into attributes of a glyph embedded directly in KB pages.
func safeKind(kind string) string {
	var b strings.Builder
	for _, r := range kind {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

// AppendageShapes builds polygons for every appendage in the descriptor. A
// "both" appendage yields two shapes, one per side.
func AppendageShapes(d Descriptor) []AppendageShape {
	var out []AppendageShape
	for i, a := range d.Appendages {
		kind := safeKind(a.Kind)
		for _, side := range sidesOf(a.Side) {
			suffix := "s"
			if side < 0 {
				suffix = "p"
			}
			out = append(out, AppendageShape{
				ID:   fmt.Sprintf("ap-%s-%d%s", kind, i+1, suffix),
				Kind: kind,
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

	// Sweep arrives from externally-authored overlay JSON, so bound it before
	// it reaches tan(): angles near 90 degrees produce an enormous trail and
	// an appendage that extends far off the glyph. The trail is then capped to
	// half the hull length, which keeps even an extreme sweep attached to the
	// ship it belongs to.
	sweep := math.Min(85, math.Max(-85, a.Sweep))
	trail := span * math.Tan(sweep*math.Pi/180)
	trail = math.Min(0.5, math.Max(-0.5, trail))

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
