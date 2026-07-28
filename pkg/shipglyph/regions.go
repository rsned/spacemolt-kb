package shipglyph

// RegionNames are the five damage-diagram regions, in the order the hull pip
// track threads through them.
var RegionNames = []string{"bow", "port", "star", "stern", "core"}

// Region boundaries along the spine.
const (
	bowEnd     = 0.25
	sternStart = 0.75
	coreInset  = 0.45 // core half-width as a fraction of the local hull
)

// Regions partitions the hull into the five diagram regions. Each region is a
// closed polygon in glyph space, suitable for emitting as its own <path> with
// a stable element ID.
func Regions(d Descriptor, st Style, seed uint64) map[string][]Point {
	star := sampleProfile(d, st, seed, 1)
	port := sampleProfile(d, st, seed, -1)

	out := map[string][]Point{
		"bow":   spanPolygon(star, port, 0, bowEnd),
		"stern": spanPolygon(star, port, sternStart, 1),
		"star":  sidePolygon(star, bowEnd, sternStart),
		"port":  sidePolygon(port, bowEnd, sternStart),
		"core":  corePolygon(d, star, bowEnd, sternStart),
	}
	return out
}

// spanPolygon returns the full-width slice of the hull between lo and hi:
// starboard edge forward to aft, then port edge back.
func spanPolygon(star, port []Point, lo, hi float64) []Point {
	var out []Point
	for _, p := range star {
		if p.X >= lo && p.X <= hi {
			out = append(out, p)
		}
	}
	for i := len(port) - 1; i >= 0; i-- {
		if port[i].X >= lo && port[i].X <= hi {
			out = append(out, port[i])
		}
	}
	return out
}

// sidePolygon returns one side's slice between lo and hi, closed along the
// centerline.
func sidePolygon(side []Point, lo, hi float64) []Point {
	var out []Point
	for _, p := range side {
		if p.X >= lo && p.X <= hi {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return out
	}
	// Close back along the centerline.
	out = append(out, Point{X: out[len(out)-1].X, Y: 0}, Point{X: out[0].X, Y: 0})
	return out
}

// corePolygon returns the narrow centerline spine between lo and hi.
func corePolygon(d Descriptor, star []Point, lo, hi float64) []Point {
	var upper, lower []Point
	for _, p := range star {
		if p.X < lo || p.X > hi {
			continue
		}
		w := hullHalfWidth(d, p.X) * coreInset
		upper = append(upper, Point{X: p.X, Y: w})
		lower = append(lower, Point{X: p.X, Y: -w})
	}
	out := upper
	for i := len(lower) - 1; i >= 0; i-- {
		out = append(out, lower[i])
	}
	return out
}
