package main

import (
	"cmp"
	"slices"
	"strings"
)

// Limits tune how sightings are judged.
type Limits struct {
	Tol       float64 // degrees: farthest a match may sit from the sighting
	ClearErr  float64 // degrees: a match farther off than this is doubtful
	ClearDist float64 // map units: a match to an ordinary star farther than this is doubtful (0: off)
	DwarfDist float64 // map units: the same for a white dwarf (class D…) (0: off)
	FitErr    float64 // degrees: how far a reading may miss a fitted star and still count
	Sigma     float64 // degrees: assumed bearing error, for the fit's error ellipse
	MinCross  float64 // degrees: two lines crossing at less than this place nothing
}

// Reading is one sighting with what it was matched to.
type Reading struct {
	Obs    string
	From   Point
	S      Sighting
	Status string // "match", "doubtful" or "unknown"
	Match  *Match
}

// analyze matches every sighting and places the unknown stars the unclear
// readings agree on. Observations
// from a system with no known position are skipped.
func analyze(obs []Observation, pos map[string]Point, stars []Star, lim Limits) ([]Reading, []Fit) {
	rs := matchAll(obs, pos, stars, lim)
	// The telescope reads each star at one fixed temperature (Menkib is
	// always 36,000 K). Lock a star to its reading once two clear matches
	// agree on it, then match again so other temperatures cannot claim it.
	if locked := lockTemps(rs); len(locked) > 0 {
		ls := slices.Clone(stars)
		for i := range ls {
			ls[i].Locked = locked[ls[i].key()]
		}
		rs = matchAll(obs, pos, ls, lim)
	}
	return rs, groupStars(rs, lim)
}

// lockTemps returns, for each clearly matched star, the temperature most of
// its clear matches agree on: at least two of them, and more than all other
// temperatures together (a stray reading of some other star at its bearing
// must not block the lock).
func lockTemps(rs []Reading) map[string]float64 {
	counts := map[string]map[float64]int{}
	for _, r := range rs {
		if r.Status != "match" {
			continue
		}
		sys := r.Match.Star.key()
		if counts[sys] == nil {
			counts[sys] = map[float64]int{}
		}
		counts[sys][r.S.Temperature]++
	}
	out := map[string]float64{}
	for sys, c := range counts {
		total := 0
		for _, n := range c {
			total += n
		}
		for t, n := range c {
			if n >= 2 && 2*n > total {
				out[sys] = t
			}
		}
	}
	return out
}

// matchAll matches every sighting of every observation, one star per
// sighting within a reading.
func matchAll(obs []Observation, pos map[string]Point, stars []Star, lim Limits) []Reading {
	var rs []Reading
	for _, o := range obs {
		at, ok := pos[o.System]
		if !ok {
			continue
		}
		// One star can explain at most one sighting in a reading. Assign
		// plausible candidates first (a cool star too far away to be seen is
		// a last resort), then the closest bearing fits, then nearer stars.
		type cand struct {
			i int
			m Match
		}
		var cs []cand
		for i, s := range o.Stars {
			for _, m := range candidates(at, o.System, s, stars, lim.Tol) {
				cs = append(cs, cand{i, m})
			}
		}
		slices.SortStableFunc(cs, func(a, b cand) int {
			return cmp.Or(cmp.Compare(b2i(lim.tooFar(a.m)), b2i(lim.tooFar(b.m))),
				cmp.Compare(a.m.Err, b.m.Err), cmp.Compare(a.m.Distance, b.m.Distance))
		})
		got := make([]*Match, len(o.Stars))
		used := map[string]bool{}
		for _, c := range cs {
			if got[c.i] == nil && !used[c.m.Star.key()] {
				m := c.m
				got[c.i], used[c.m.Star.key()] = &m, true
			}
		}
		for i, s := range o.Stars {
			r := Reading{Obs: o.System, From: at, S: s, Status: "unknown", Match: got[i]}
			if m := got[i]; m != nil {
				r.Status = "match"
				if m.Err > lim.ClearErr || lim.tooFar(*m) {
					r.Status = "doubtful"
				}
			}
			rs = append(rs, r)
		}
	}
	return rs
}

// tooFar reports a match too far away for the telescope to have seen it.
// Wolf-Rayet and O-class stars have been read from 6,000 units away; a white
// dwarf only to about 1,500, and other stars to about 1,500.
func (lim Limits) tooFar(m Match) bool {
	switch c := m.Star.Class; {
	case strings.HasPrefix(c, "WR"), strings.HasPrefix(c, "O"):
		return false
	case strings.HasPrefix(c, "D"):
		return lim.DwarfDist > 0 && m.Distance > lim.DwarfDist
	default:
		return lim.ClearDist > 0 && m.Distance > lim.ClearDist
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
