package buildcost

import "sort"

// StationDepth maps station id -> (item id -> total sell depth available there).
type StationDepth map[string]map[string]float64

// CoverResult is the outcome of a minimum-station cover search.
// Feasible is false when some input's total depth across all stations is below
// the required quantity; Missing then lists those inputs (sorted). When
// Feasible, Count is the number of stations in the returned cover (Stations,
// sorted) and Exact reports whether that count is a proven minimum (true) or a
// greedy upper bound (false, for covers deeper than exactK).
type CoverResult struct {
	Feasible bool
	Count    int
	Stations []string
	Exact    bool
	Missing  []string
}

// MinStationCover finds the fewest stations whose pooled sell depth covers every
// requirement. It proves the minimum for covers up to exactK stations by
// exhaustive search over station combinations (in lexicographic order of sorted
// ids), then falls back to a greedy upper bound. Deterministic.
func MinStationCover(reqs []Requirement, depth StationDepth, stationIDs []string, exactK int) CoverResult {
	// Aggregate required quantity per input (summing duplicates).
	need := map[string]float64{}
	for _, r := range reqs {
		need[r.ItemID] += r.Qty
	}
	if len(need) == 0 {
		return CoverResult{Feasible: true, Count: 0, Stations: []string{}, Exact: true}
	}

	ids := append([]string(nil), stationIDs...)
	sort.Strings(ids)

	// Feasibility gate: any input whose total depth < need is unmeetable.
	var missing []string
	for item, q := range need {
		var total float64
		for _, s := range ids {
			total += depth[s][item]
		}
		if total < q {
			missing = append(missing, item)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return CoverResult{Feasible: false, Missing: missing}
	}

	// covers reports whether the given station subset meets every need.
	covers := func(subset []string) bool {
		for item, q := range need {
			var have float64
			for _, s := range subset {
				have += depth[s][item]
			}
			if have < q {
				return false
			}
		}
		return true
	}

	// Exact search for k = 1..exactK over combinations in lexicographic order.
	for k := 1; k <= exactK && k <= len(ids); k++ {
		combo := make([]string, k)
		var found []string
		var rec func(start, filled int) bool
		rec = func(start, filled int) bool {
			if filled == k {
				if covers(combo) {
					found = append([]string(nil), combo...)
					return true
				}
				return false
			}
			for i := start; i <= len(ids)-(k-filled); i++ {
				combo[filled] = ids[i]
				if rec(i+1, filled+1) {
					return true
				}
			}
			return false
		}
		if rec(0, 0) {
			return CoverResult{Feasible: true, Count: k, Stations: found, Exact: true}
		}
	}

	// Greedy fallback: repeatedly add the station that covers the most remaining
	// shortfall; ties break by id (ids already sorted). Guaranteed to terminate
	// because the feasibility gate proved the full set covers.
	remaining := map[string]float64{}
	for item, q := range need {
		remaining[item] = q
	}
	chosen := map[string]bool{}
	var cover []string
	shortfallLeft := func() bool {
		for _, q := range remaining {
			if q > 0 {
				return true
			}
		}
		return false
	}
	for shortfallLeft() {
		bestID := ""
		bestGain := -1.0
		for _, s := range ids {
			if chosen[s] {
				continue
			}
			var gain float64
			for item, q := range remaining {
				if q <= 0 {
					continue
				}
				d := depth[s][item]
				if d > q {
					d = q
				}
				gain += d
			}
			if gain > bestGain {
				bestGain = gain
				bestID = s
			}
		}
		if bestID == "" { // safety; should not happen given the gate
			break
		}
		chosen[bestID] = true
		cover = append(cover, bestID)
		for item := range remaining {
			remaining[item] -= depth[bestID][item]
		}
	}
	sort.Strings(cover)
	return CoverResult{Feasible: true, Count: len(cover), Stations: cover, Exact: false}
}
