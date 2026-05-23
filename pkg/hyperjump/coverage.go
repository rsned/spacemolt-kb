package hyperjump

import (
	"math"
	"sort"
)

// Gap is an uncovered range of headings from an origin — a "void escape" window
// where a hyper-jump intersects no system and never lands.
type Gap struct {
	StartDeg  float64
	EndDeg    float64
	WidthDeg  float64
	CenterDeg float64
}

// interval is a half-open heading range on [0,360).
type interval struct{ lo, hi float64 }

// Coverage returns the fraction of headings [0,360) from origin that are blocked
// by some system (the ray passes within margin GU of it), and the list of
// uncovered gaps ordered widest-first. An empty gap list means every direction
// eventually lands somewhere.
func Coverage(origin System, systems []System, margin float64) (pct float64, gaps []Gap) {
	var ivs []interval
	for _, s := range systems {
		if s.ID == origin.ID {
			continue
		}
		rel := Vec{s.Pos.X - origin.Pos.X, s.Pos.Y - origin.Pos.Y}
		r := math.Hypot(rel.X, rel.Y)
		if r == 0 {
			continue
		}
		phi := Bearing(origin.Pos, s.Pos)
		half := ArcHalfWidth(r, margin)
		ivs = append(ivs, splitWrap(phi-half, phi+half)...)
	}

	merged := mergeIntervals(ivs)

	covered := 0.0
	for _, iv := range merged {
		covered += iv.hi - iv.lo
	}
	pct = covered / 360

	gaps = findGaps(merged)
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].WidthDeg != gaps[j].WidthDeg {
			return gaps[i].WidthDeg > gaps[j].WidthDeg
		}
		return gaps[i].StartDeg < gaps[j].StartDeg
	})
	return pct, gaps
}

// splitWrap normalizes an arc [lo,hi] onto [0,360), splitting it where it
// crosses the 0/360 boundary.
func splitWrap(lo, hi float64) []interval {
	if hi-lo >= 360 {
		return []interval{{0, 360}}
	}
	lo = math.Mod(math.Mod(lo, 360)+360, 360)
	hi = math.Mod(math.Mod(hi, 360)+360, 360)
	if lo <= hi {
		return []interval{{lo, hi}}
	}
	return []interval{{lo, 360}, {0, hi}}
}

// mergeIntervals sorts and unions overlapping/touching intervals.
func mergeIntervals(ivs []interval) []interval {
	if len(ivs) == 0 {
		return nil
	}
	sort.Slice(ivs, func(i, j int) bool { return ivs[i].lo < ivs[j].lo })
	const touch = 1e-9
	merged := []interval{ivs[0]}
	for _, iv := range ivs[1:] {
		last := &merged[len(merged)-1]
		if iv.lo <= last.hi+touch {
			if iv.hi > last.hi {
				last.hi = iv.hi
			}
		} else {
			merged = append(merged, iv)
		}
	}
	return merged
}

// findGaps returns the uncovered spans of [0,360) given merged coverage. A gap
// adjacent to both 0 and 360 (coverage touches both ends) is a single wrap gap.
func findGaps(merged []interval) []Gap {
	if len(merged) == 0 {
		return []Gap{{StartDeg: 0, EndDeg: 360, WidthDeg: 360, CenterDeg: 180}}
	}
	const touch = 1e-9
	var gaps []Gap
	prev := 0.0
	for _, iv := range merged {
		if iv.lo > prev+touch {
			gaps = append(gaps, makeGap(prev, iv.lo))
		}
		if iv.hi > prev {
			prev = iv.hi
		}
	}
	if prev < 360-touch {
		gaps = append(gaps, makeGap(prev, 360))
	}
	return gaps
}

func makeGap(lo, hi float64) Gap {
	return Gap{StartDeg: lo, EndDeg: hi, WidthDeg: hi - lo, CenterDeg: (lo + hi) / 2}
}
