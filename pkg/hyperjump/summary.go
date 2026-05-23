package hyperjump

import "sort"

// Summary is a galaxy-wide rollup of a hyper-jump analysis.
type Summary struct {
	Systems           int     `json:"systems"`
	DirectedPairs     int     `json:"directedPairs"`
	Reachable         int     `json:"reachable"`
	Blocked           int     `json:"blocked"`
	SystemsWithEscape int     `json:"systemsWithEscape"` // origins with >=1 void gap
	MinMargin         float64 `json:"minMargin"`         // degrees, over reachable pairs
	MedianMargin      float64 `json:"medianMargin"`
	MaxMargin         float64 `json:"maxMargin"`
}

// Summarize aggregates per-origin reports into galaxy-wide statistics. Margin
// statistics are taken over reachable pairs only.
func Summarize(reports []OriginReport) Summary {
	s := Summary{Systems: len(reports)}
	var margins []float64
	for _, r := range reports {
		s.DirectedPairs += len(r.Pairs)
		for _, p := range r.Pairs {
			if p.Reachable {
				s.Reachable++
				margins = append(margins, p.AngularMargin)
			} else {
				s.Blocked++
			}
		}
		if len(r.Gaps) > 0 {
			s.SystemsWithEscape++
		}
	}
	if len(margins) > 0 {
		sort.Float64s(margins)
		s.MinMargin = margins[0]
		s.MaxMargin = margins[len(margins)-1]
		s.MedianMargin = median(margins)
	}
	return s
}

// median returns the median of a sorted slice.
func median(sorted []float64) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
