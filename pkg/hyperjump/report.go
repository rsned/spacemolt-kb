package hyperjump

// Pair is the analysis of a direct hyper-jump from one system to another.
type Pair struct {
	From           string   `json:"from"`
	To             string   `json:"to"`
	Bearing        float64  `json:"bearing"`                // Q1: heading in degrees [0,360)
	Distance       float64  `json:"distance"`               // GU to the target
	Reachable      bool     `json:"reachable"`              // Q2: true if no interrupter
	LandsAt        string   `json:"landsAt"`                // actual landing system
	Interrupters   []string `json:"interrupters,omitempty"` // stealing systems, nearest-first
	AngularMargin  float64  `json:"angularMargin"`          // Q3: binding deviation, degrees (0 if blocked)
	MarginLeft     float64  `json:"marginLeft"`             // clockwise limit, degrees
	MarginRight    float64  `json:"marginRight"`            // counter-clockwise limit, degrees
	Clearance      float64  `json:"clearance"`              // nearest non-target perpendicular, GU
	DestHasStation bool     `json:"destHasStation"`         // destination system contains a station
}

// OriginReport collects every direct-jump pair from one origin system plus its
// void-coverage summary.
type OriginReport struct {
	System      string  `json:"system"`
	Pairs       []Pair  `json:"pairs"`
	CoveragePct float64 `json:"coveragePct"`
	Gaps        []Gap   `json:"gaps,omitempty"`
}

// Analyze runs the full pairwise hyper-jump analysis for every system, returning
// one OriginReport per origin (in input order).
func Analyze(systems []System, margin float64) []OriginReport {
	reports := make([]OriginReport, 0, len(systems))
	for _, origin := range systems {
		pairs := make([]Pair, 0, len(systems)-1)
		for _, dest := range systems {
			if dest.ID == origin.ID {
				continue
			}
			pairs = append(pairs, analyzePair(origin, dest, systems, margin))
		}
		pct, gaps := Coverage(origin, systems, margin)
		reports = append(reports, OriginReport{
			System:      origin.ID,
			Pairs:       pairs,
			CoveragePct: pct,
			Gaps:        gaps,
		})
	}
	return reports
}

// FilterStationDestinations returns a copy of reports keeping only the pairs
// whose destination has a station, and dropping origins left with no such pairs.
// Each retained report keeps its origin's coverage/gaps for context.
func FilterStationDestinations(reports []OriginReport) []OriginReport {
	out := make([]OriginReport, 0, len(reports))
	for _, r := range reports {
		pairs := make([]Pair, 0)
		for _, p := range r.Pairs {
			if p.DestHasStation {
				pairs = append(pairs, p)
			}
		}
		if len(pairs) == 0 {
			continue
		}
		r.Pairs = pairs
		out = append(out, r)
	}
	return out
}

// analyzePair builds the Pair record for a single origin->dest jump.
func analyzePair(origin, dest System, systems []System, margin float64) Pair {
	p := Pair{
		From:           origin.ID,
		To:             dest.ID,
		Bearing:        Bearing(origin.Pos, dest.Pos),
		Distance:       dist(origin.Pos, dest.Pos),
		Clearance:      Clearance(origin, dest, systems),
		DestHasStation: dest.HasStation,
	}
	p.Interrupters = Interrupters(origin, dest, systems, margin)
	p.Reachable = len(p.Interrupters) == 0
	if p.Reachable {
		p.LandsAt = dest.ID
		p.AngularMargin, p.MarginLeft, p.MarginRight = AngularMargin(origin, dest, systems, margin)
	} else {
		p.LandsAt = p.Interrupters[0]
	}
	return p
}
