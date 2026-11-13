package main

import (
	"cmp"
	"math"
	"slices"
)

// jumpMargin is the Pathfinder Drive landing tolerance: a jump stops at the
// first system ahead within this many units of its line (the KB's arc
// tables use the same rule).
const jumpMargin = 100.0

// pathLanding is where a Pathfinder jump from `from` on heading would stop:
// the nearest known system ahead within jumpMargin of the line, or "" for
// the void.
func pathLanding(from Point, fromID string, heading float64, systems []System) (string, float64) {
	t := heading * math.Pi / 180
	cx, cy := math.Cos(t), math.Sin(t)
	best, bestD := "", math.Inf(1)
	for _, s := range systems {
		if s.ID == fromID {
			continue
		}
		rx, ry := s.At.X-from.X, s.At.Y-from.Y
		proj := rx*cx + ry*cy
		if proj <= 0 || math.Abs(rx*cy-ry*cx) > jumpMargin {
			continue
		}
		if proj < bestD {
			best, bestD = s.ID, proj
		}
	}
	if best == "" {
		return "", 0
	}
	return best, bestD
}

// Jump is one Pathfinder heading toward a fitted star.
type Jump struct {
	From     string
	Heading  float64 // toward the fitted position, not the rounded sighting
	Tol      float64 // ± degrees that still pass within jumpMargin of the star
	Distance float64
	Blocker  string // known system the jump would stop at first ("" if none)
	BlockAt  float64
	Clear    bool // nothing known in the way: the jump should reach the star
}

// jumpChecks tests a Pathfinder jump from each system that saw the star,
// clear paths first, shortest first.
func jumpChecks(f Fit, systems []System) []Jump {
	var out []Jump
	for _, m := range f.Members {
		d := math.Hypot(f.At.X-m.From.X, f.At.Y-m.From.Y)
		j := Jump{From: m.Obs, Heading: bearing(m.From, f.At), Distance: d, Tol: math.Atan(jumpMargin/d) * 180 / math.Pi}
		id, at := pathLanding(m.From, m.Obs, j.Heading, systems)
		if id != "" && at < d-jumpMargin {
			j.Blocker, j.BlockAt = id, at
		} else {
			j.Clear = true
		}
		out = append(out, j)
	}
	slices.SortStableFunc(out, func(a, b Jump) int {
		if a.Clear != b.Clear {
			if a.Clear {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Distance, b.Distance)
	})
	return out
}
