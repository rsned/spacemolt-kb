package shipglyph

import "math"

// profileSamples is how many points are taken along each side of the hull.
// High enough for smooth curves at glyph size, low enough to keep SVGs small.
const profileSamples = 96

// partHalfWidth returns the hull half-width contributed by part p at spine
// position t. It returns 0 when t falls outside the part's span.
func partHalfWidth(p HullPart, t float64) float64 {
	lo, hi := p.Span[0], p.Span[1]
	if hi <= lo || t < lo || t > hi {
		return 0
	}
	// u is the position within the part, 0 at its start and 1 at its end.
	u := (t - lo) / (hi - lo)

	switch p.Kind {
	case "beam":
		return beamHalfWidth(p.Points, t)
	case "box":
		return p.Half
	case "container_stack":
		across := p.Grid[0]
		if across < 1 {
			across = 1
		}
		return 0.11 * float64(across)
	case "bay_rack":
		return 0.30
	case "engine_cone":
		// Widest where it meets the hull, tapering to the nozzle throat.
		return 0.20 - 0.09*u
	case "open_frame":
		if p.Seat {
			return 0.13
		}
		return 0.09
	case "drum":
		// Cylindrical body with rounded ends.
		return p.Radius * math.Sqrt(math.Max(0, 1-math.Pow(2*u-1, 6)))
	case "disc":
		v := 2*u - 1
		return p.Radius * math.Sqrt(math.Max(0, 1-v*v))
	case "lobe_cluster":
		n := p.Lobes
		if n < 1 {
			n = 1
		}
		// Sum of evenly spaced bulges along the span.
		var w float64
		for i := range n {
			c := (float64(i) + 0.5) / float64(n)
			d := (u - c) * float64(n) * 1.6
			w += 0.22 * math.Exp(-d*d)
		}
		return w
	default:
		return 0.15
	}
}

// beamHalfWidth linearly interpolates [t, halfwidth] control points. Points
// must be sorted by t; values outside the range clamp to the end points.
func beamHalfWidth(pts [][2]float64, t float64) float64 {
	if len(pts) == 0 {
		return 0
	}
	if t <= pts[0][0] {
		return pts[0][1]
	}
	last := pts[len(pts)-1]
	if t >= last[0] {
		return last[1]
	}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		if t <= b[0] {
			span := b[0] - a[0]
			if span <= 0 {
				return b[1]
			}
			f := (t - a[0]) / span
			return a[1] + f*(b[1]-a[1])
		}
	}
	return last[1]
}

// hullHalfWidth returns the widest contribution of any part at t.
func hullHalfWidth(d Descriptor, t float64) float64 {
	var w float64
	for _, p := range d.Hull {
		if h := partHalfWidth(p, t); h > w {
			w = h
		}
	}
	return w
}

// sampleProfile walks the spine from nose to tail and returns one side of the
// hull outline. side is +1 for starboard (positive Y) or -1 for port.
// Style-driven jitter is seeded per side, so asymmetric factions differ left to
// right while symmetric ones mirror exactly.
func sampleProfile(d Descriptor, st Style, seed uint64, side int) []Point {
	// Port gets a distinct sub-seed so its jitter is independent. Symmetric
	// styles have Jitter == 0 and are therefore unaffected.
	sub := seed
	if side < 0 {
		sub = seed ^ 0x5bf03635
	}
	r := newRNG(sub)

	out := make([]Point, profileSamples)
	for i := range profileSamples {
		t := float64(i) / float64(profileSamples-1)
		w := hullHalfWidth(d, t)

		if st.Lobed {
			// Organic swelling: a slow wave that never narrows the hull.
			w *= 1 + 0.18*math.Sin(t*math.Pi*3.0)
		}
		if st.Flute {
			// Regular perpendicular notches.
			w *= 1 - 0.06*math.Abs(math.Sin(t*math.Pi*9))
		}
		if st.Jitter > 0 {
			w *= 1 + st.Jitter*(r.next()*2-1)
		}
		if w < 0 {
			w = 0
		}
		out[i] = Point{X: t, Y: w * float64(side)}
	}
	return out
}
