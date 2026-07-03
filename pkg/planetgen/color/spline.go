package color

// EvalSpline evaluates a Fritsch-Carlson monotone-cubic spline at x.
// Empty knot lists return 0. x outside [first.Input, last.Input] is
// clamped to the boundary knot's Output. Knots must be sorted by Input
// ascending.
func EvalSpline(s Spline, x float64) float64 {
	n := len(s.Knots)
	if n == 0 {
		return 0
	}
	if n == 1 || x <= s.Knots[0].Input {
		return s.Knots[0].Output
	}
	if x >= s.Knots[n-1].Input {
		return s.Knots[n-1].Output
	}

	// Find the segment.
	i := 0
	for i < n-1 && x > s.Knots[i+1].Input {
		i++
	}
	x0, y0 := s.Knots[i].Input, s.Knots[i].Output
	x1, y1 := s.Knots[i+1].Input, s.Knots[i+1].Output
	h := x1 - x0
	if h == 0 {
		return y0
	}

	// Fritsch-Carlson tangents using harmonic-mean rule.
	d := splineSlope(s.Knots, i)   // tangent at i
	dn := splineSlope(s.Knots, i+1) // tangent at i+1

	t := (x - x0) / h
	t2 := t * t
	t3 := t2 * t
	h00 := 2*t3 - 3*t2 + 1
	h10 := t3 - 2*t2 + t
	h01 := -2*t3 + 3*t2
	h11 := t3 - t2
	return h00*y0 + h10*h*d + h01*y1 + h11*h*dn
}

// splineSlope computes the Fritsch-Carlson tangent at knot i using
// harmonic-mean rule for interior knots and one-sided differences at
// endpoints.
func splineSlope(ks []SplineKnot, i int) float64 {
	n := len(ks)
	if i == 0 {
		return (ks[1].Output - ks[0].Output) / (ks[1].Input - ks[0].Input)
	}
	if i == n-1 {
		return (ks[n-1].Output - ks[n-2].Output) / (ks[n-1].Input - ks[n-2].Input)
	}
	// Secant slopes on either side.
	dl := (ks[i].Output - ks[i-1].Output) / (ks[i].Input - ks[i-1].Input)
	dr := (ks[i+1].Output - ks[i].Output) / (ks[i+1].Input - ks[i].Input)
	if dl*dr <= 0 {
		// Sign change → flat tangent to preserve monotonicity.
		return 0
	}
	// Harmonic mean weighted by interval lengths.
	hl := ks[i].Input - ks[i-1].Input
	hr := ks[i+1].Input - ks[i].Input
	return (3 * (hl + hr)) / ((2*hl+hr)/dr + (hl+2*hr)/dl)
}
