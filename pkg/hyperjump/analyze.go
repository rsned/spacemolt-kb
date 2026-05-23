package hyperjump

import (
	"math"
	"sort"
)

// System is a galactic system with an identity and position.
type System struct {
	ID   string
	Name string
	Pos  Vec
}

// dist returns the Euclidean distance between two points.
func dist(a, b Vec) float64 { return math.Hypot(b.X-a.X, b.Y-a.Y) }

// angDiff returns a-b normalized to (-180, 180] degrees.
func angDiff(a, b float64) float64 {
	d := math.Mod(a-b, 360)
	if d <= -180 {
		d += 360
	} else if d > 180 {
		d -= 360
	}
	return d
}

// AngularMargin returns, for a clean direct jump from origin to dest, how far
// the heading may deviate (in degrees) before dest is no longer the landing
// system. It returns the binding margin (min of the two sides) plus the
// clockwise (left) and counter-clockwise (right) limits separately.
//
// On each side the limit is the nearest of: dest leaving its own corridor, or a
// nearer system (proj < distance-to-dest) entering its corridor. Systems farther
// than dest at the aimed heading are ignored — within these small windows they
// cannot overtake dest in projection.
func AngularMargin(origin, dest System, systems []System, margin float64) (m, left, right float64) {
	heading := Bearing(origin.Pos, dest.Pos)
	target := dist(origin.Pos, dest.Pos)

	// Destination's own corridor limit bounds both sides initially.
	destLimit := ArcHalfWidth(target, margin)
	left, right = destLimit, destLimit

	for _, s := range systems {
		if s.ID == origin.ID || s.ID == dest.ID {
			continue
		}
		rel := Vec{s.Pos.X - origin.Pos.X, s.Pos.Y - origin.Pos.Y}
		proj := Proj(rel, heading)
		if proj <= 0 || proj >= target { // behind, or not closer than dest
			continue
		}
		r := math.Hypot(rel.X, rel.Y)
		half := ArcHalfWidth(r, margin)
		d := angDiff(Bearing(origin.Pos, s.Pos), heading) // CCW offset of s from heading
		lo, hi := d-half, d+half                       // s's corridor relative to heading

		if hi > 0 { // corridor reaches the CCW side
			onset := math.Max(lo, 0)
			right = math.Min(right, onset)
		}
		if lo < 0 { // corridor reaches the CW side
			onset := math.Max(-hi, 0)
			left = math.Min(left, onset)
		}
	}
	return math.Min(left, right), left, right
}

// Clearance returns the smallest perpendicular distance (GU) from any system
// other than origin and dest to the origin->dest bearing ray — how close the
// aimed jump comes to an accidental interruption.
func Clearance(origin, dest System, systems []System) float64 {
	heading := Bearing(origin.Pos, dest.Pos)
	best := math.Inf(1)
	for _, s := range systems {
		if s.ID == origin.ID || s.ID == dest.ID {
			continue
		}
		rel := Vec{s.Pos.X - origin.Pos.X, s.Pos.Y - origin.Pos.Y}
		if p := math.Abs(SignedPerp(rel, heading)); p < best {
			best = p
		}
	}
	return best
}

// Interrupters returns the systems that steal a direct hyper-jump from origin
// aimed at dest: those lying in front (0 < proj < distance-to-dest) and within
// margin GU of the bearing ray. The result is ordered nearest-first (ascending
// proj) — the first entry is where the ship actually lands.
func Interrupters(origin, dest System, systems []System, margin float64) []string {
	heading := Bearing(origin.Pos, dest.Pos)
	target := dist(origin.Pos, dest.Pos)

	type hit struct {
		id   string
		proj float64
	}
	var hits []hit
	for _, s := range systems {
		if s.ID == origin.ID || s.ID == dest.ID {
			continue
		}
		rel := Vec{s.Pos.X - origin.Pos.X, s.Pos.Y - origin.Pos.Y}
		proj := Proj(rel, heading)
		if proj <= 0 || proj >= target {
			continue
		}
		if math.Abs(SignedPerp(rel, heading)) > margin {
			continue
		}
		hits = append(hits, hit{s.ID, proj})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].proj < hits[j].proj })

	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.id
	}
	return ids
}
