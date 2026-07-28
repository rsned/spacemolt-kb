package shipglyph

// Outline assembles the closed hull loop in glyph space. The loop runs along
// the starboard side from nose to tail, then back along the port side from
// tail to nose. The caller is responsible for closing the path.
func Outline(d Descriptor, st Style, seed uint64) []Point {
	star := sampleProfile(d, st, seed, 1)
	port := sampleProfile(d, st, seed, -1)

	loop := make([]Point, 0, len(star)+len(port))
	loop = append(loop, star...)
	for i := len(port) - 1; i >= 0; i-- {
		loop = append(loop, port[i])
	}

	if st.Chamfer > 0 {
		loop = chamfer(loop, st.Chamfer)
	}
	if st.Smooth {
		loop = smooth(loop)
	}
	return loop
}

// chamfer replaces each vertex with two points cut back along its adjacent
// edges by fraction f, producing the angular faceted look. f is clamped to
// (0, 0.5] so adjacent chamfers cannot overlap.
func chamfer(loop []Point, f float64) []Point {
	n := len(loop)
	if n < 3 {
		return loop
	}
	if f > 0.5 {
		f = 0.5
	}
	out := make([]Point, 0, n*2)
	for i := range n {
		prev := loop[(i-1+n)%n]
		cur := loop[i]
		next := loop[(i+1)%n]
		out = append(out,
			Point{X: cur.X + (prev.X-cur.X)*f, Y: cur.Y + (prev.Y-cur.Y)*f},
			Point{X: cur.X + (next.X-cur.X)*f, Y: cur.Y + (next.Y-cur.Y)*f},
		)
	}
	return out
}

// smooth applies one pass of Chaikin corner cutting, which converges toward a
// quadratic B-spline. Used for the flowing Nebula and Voidborn hulls.
func smooth(loop []Point) []Point {
	n := len(loop)
	if n < 3 {
		return loop
	}
	out := make([]Point, 0, n*2)
	for i := range n {
		a := loop[i]
		b := loop[(i+1)%n]
		out = append(out,
			Point{X: 0.75*a.X + 0.25*b.X, Y: 0.75*a.Y + 0.25*b.Y},
			Point{X: 0.25*a.X + 0.75*b.X, Y: 0.25*a.Y + 0.75*b.Y},
		)
	}
	return out
}
