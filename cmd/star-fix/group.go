package main

import (
	"cmp"
	"math"
	"slices"
)

// hotK is the reading at and above which an unknown star may be very far
// away: Wolf-Rayet, O-class and B-class stars have been read from thousands
// of units. Cooler readings have only ever been ordinary nearby stars.
const hotK = 20000

// Fit is one unknown star placed from several unclear readings that agree.
type Fit struct {
	Temp    float64
	Members []Reading // at most one per observing system
	At      Point
	MaxErr  float64 // degrees: the worst member bearing against At
	// Major and Minor are the 1-sigma error ellipse half-axes (map units) for
	// Sigma degrees of bearing error; Angle is the major axis direction
	// (radians, map orientation). Nearly parallel lines give a long ellipse:
	// the direction is known, the distance is not.
	Major, Minor, Angle float64
}

// groupStars places unknown stars. It repeatedly takes the largest set of
// unclear readings (same temperature, one per system) whose bearings all
// pass within FitErr of one least-squares point ahead of every observer,
// then removes them and looks again. Two-reading sets need lines crossing at
// MinCross degrees or more, or the point is meaningless.
func groupStars(rs []Reading, lim Limits) []Fit {
	var pool []Reading
	for _, r := range rs {
		if r.Status != "match" {
			pool = append(pool, r)
		}
	}
	var fits []Fit
	for {
		var best *Fit
		for i := range pool {
			for j := i + 1; j < len(pool); j++ {
				a, b := pool[i], pool[j]
				if a.Obs == b.Obs || a.S.Temperature != b.S.Temperature {
					continue
				}
				seed, ok := intersect(a.From, a.S.Bearing, b.From, b.S.Bearing)
				if !ok {
					continue
				}
				f, ok := grow(seed, a.S.Temperature, pool, lim)
				if !ok {
					continue
				}
				if best == nil || len(f.Members) > len(best.Members) ||
					(len(f.Members) == len(best.Members) && f.MaxErr < best.MaxErr) {
					best = &f
				}
			}
		}
		if best == nil {
			break
		}
		fits = append(fits, *best)
		pool = slices.DeleteFunc(pool, func(r Reading) bool {
			return slices.ContainsFunc(best.Members, func(m Reading) bool { return m.Obs == r.Obs && m.S == r.S })
		})
	}
	slices.SortStableFunc(fits, func(a, b Fit) int { return cmp.Compare(len(b.Members), len(a.Members)) })
	return fits
}

// grow gathers the readings that agree with a point, refits, and repeats a
// few times so the members and the point settle together.
func grow(p Point, temp float64, pool []Reading, lim Limits) (Fit, bool) {
	var f Fit
	for range 4 {
		members := agreeing(p, temp, pool, lim)
		if len(members) < 2 {
			return Fit{}, false
		}
		q, ok := lsq(members, p, lim.Sigma)
		if !ok {
			return Fit{}, false
		}
		f = Fit{Temp: temp, Members: members}
		p = q
	}
	f.Members = agreeing(p, temp, pool, lim)
	if len(f.Members) < 2 {
		return Fit{}, false
	}
	if len(f.Members) == 2 && angleDiff(f.Members[0].S.Bearing, f.Members[1].S.Bearing) < lim.MinCross {
		return Fit{}, false
	}
	f.At = p
	for _, m := range f.Members {
		f.MaxErr = math.Max(f.MaxErr, angleDiff(bearing(m.From, p), m.S.Bearing))
	}
	f.Major, f.Minor, f.Angle = ellipse(f.Members, p, lim.Sigma)
	return f, true
}

// minStarDist is how close a fitted star may sit to a system that saw it:
// closer, and the "star" is that system's own.
const minStarDist = 100

// agreeing picks, per observing system, the reading of this temperature
// whose bearing passes closest to p, if within FitErr degrees, the point is
// not the system itself, and a cool star (under hotK) is close enough to
// have been seen (ClearDist).
func agreeing(p Point, temp float64, pool []Reading, lim Limits) []Reading {
	best := map[string]Reading{}
	var order []string
	for _, r := range pool {
		d := math.Hypot(p.X-r.From.X, p.Y-r.From.Y)
		if r.S.Temperature != temp || d < minStarDist || (lim.ClearDist > 0 && temp < hotK && d > lim.ClearDist) {
			continue
		}
		e := angleDiff(bearing(r.From, p), r.S.Bearing)
		if e > lim.FitErr {
			continue
		}
		cur, ok := best[r.Obs]
		if !ok {
			order = append(order, r.Obs)
		}
		if !ok || e < angleDiff(bearing(cur.From, p), cur.S.Bearing) {
			best[r.Obs] = r
		}
	}
	out := make([]Reading, 0, len(order))
	for _, o := range order {
		out = append(out, best[o])
	}
	return out
}

// normalInfo sums each bearing line's normal outer product, weighted by the
// line's perpendicular error at p (distance × Sigma), and the matching
// right-hand side for a least-squares point.
func normalInfo(rs []Reading, p Point, sigma float64) (a [2][2]float64, bx, by float64) {
	for _, r := range rs {
		t := r.S.Bearing * math.Pi / 180
		nx, ny := -math.Sin(t), math.Cos(t)
		d := math.Max(math.Hypot(p.X-r.From.X, p.Y-r.From.Y), 1)
		s := d * sigma * math.Pi / 180
		w := 1 / (s * s)
		c := nx*r.From.X + ny*r.From.Y
		a[0][0] += w * nx * nx
		a[0][1] += w * nx * ny
		a[1][1] += w * ny * ny
		bx += w * nx * c
		by += w * ny * c
	}
	a[1][0] = a[0][1]
	return a, bx, by
}

// lsq is the weighted least-squares point nearest every bearing line; it
// fails if the point is behind any observer or the lines are parallel.
func lsq(rs []Reading, p Point, sigma float64) (Point, bool) {
	a, bx, by := normalInfo(rs, p, sigma)
	det := a[0][0]*a[1][1] - a[0][1]*a[1][0]
	if math.Abs(det) < 1e-18 {
		return Point{}, false
	}
	q := Point{(a[1][1]*bx - a[0][1]*by) / det, (a[0][0]*by - a[1][0]*bx) / det}
	for _, r := range rs {
		t := r.S.Bearing * math.Pi / 180
		if (q.X-r.From.X)*math.Cos(t)+(q.Y-r.From.Y)*math.Sin(t) <= 0 {
			return Point{}, false
		}
	}
	return q, true
}

// ellipse is the 1-sigma error ellipse of the fit: the inverse of the
// weighted normal matrix, as half-axes and the major axis direction.
func ellipse(rs []Reading, p Point, sigma float64) (major, minor, angle float64) {
	a, _, _ := normalInfo(rs, p, sigma)
	det := a[0][0]*a[1][1] - a[0][1]*a[1][0]
	if det <= 0 {
		return math.Inf(1), math.Inf(1), 0
	}
	cxx, cxy, cyy := a[1][1]/det, -a[0][1]/det, a[0][0]/det
	tr, dd := cxx+cyy, cxx*cyy-cxy*cxy
	disc := math.Sqrt(math.Max(tr*tr/4-dd, 0))
	l1, l2 := tr/2+disc, math.Max(tr/2-disc, 0)
	return math.Sqrt(l1), math.Sqrt(l2), 0.5 * math.Atan2(2*cxy, cxx-cyy)
}
