package prodcost

import (
	"cmp"
	"maps"
	"math"
	"slices"
)

// tickSeconds is the length of one game tick.
const tickSeconds = 10

// zeroHoursFloor is the Hours value ProfitPerHour divides by when a row's
// Hours is 0 (no make time, no haul): one tick, so a free instant sale ranks
// above any slower one and nothing divides by zero.
const zeroHoursFloor = tickSeconds / 3600.0

// Opportunity is one Station Manager bid level filled by making the item.
type Opportunity struct {
	ItemID, StationID string

	Bid, Depth, CostPerUnit, MarginPerUnit, Total float64
	MakeHours, HaulHours, Hours, ProfitPerHour    float64
	// Unreachable is set when a needed hop is missing; such rows keep Margin
	// and Total but have Hours, HaulHours and ProfitPerHour 0, and sort last.
	Unreachable bool
	RouteLabel  string
	Policy      Policy
	Partial     bool
}

// HaulParams sets how hauls are timed. Home is a system id; Hops is
// system -> system -> jumps (missing pairs are unreachable; it is read
// symmetrically, and a system is 0 hops from itself).
type HaulParams struct {
	Home            string
	JumpTicks, Hold float64
	Hops            map[string]map[string]int
}

func (hp HaulParams) hops(a, b string) (int, bool) {
	if a == b {
		return 0, a != ""
	}
	if d, ok := hp.Hops[a][b]; ok {
		return d, true
	}
	d, ok := hp.Hops[b][a]
	return d, ok
}

// candidate is one usable top route costed under one policy.
type candidate struct {
	route          Route
	policy         Policy
	credits, ticks float64
	partial        bool
	fSys           string // system the run happens in; "" when unknown
}

// UncostedBids counts the bid levels Opportunities skips because their item
// has no ItemCost with a usable route.
func UncostedBids(costs map[string]*ItemCost, m Market) int {
	n := 0
	for item, ls := range m.Bids {
		if ic := costs[item]; ic == nil || ic.NoUsableRoute {
			n += len(ls)
		}
	}
	return n
}

// Opportunities returns one row per Station Manager bid level of every item
// with a usable route (see UncostedBids for the rest). Each row keeps the
// usable top route × policy with the best ProfitPerHour: reachable rows
// first, then rows whose cost is complete, then the higher ProfitPerHour.
// Rows sort by ProfitPerHour descending, unreachable rows last (by Total).
func Opportunities(costs map[string]*ItemCost, w World, m Market, hp HaulParams) []Opportunity {
	var out []Opportunity
	for _, item := range slices.Sorted(maps.Keys(m.Bids)) {
		ic := costs[item]
		if ic == nil || ic.NoUsableRoute {
			continue
		}
		cands := candidates(costs, w, m, hp, item, ic)
		size := w.Items[item].Size
		for _, bid := range m.Bids[item] {
			var best Opportunity
			for i, c := range cands {
				o := evaluate(item, bid, size, c, m, hp)
				if i == 0 || better(o, best) {
					best = o
				}
			}
			out = append(out, best)
		}
	}
	slices.SortStableFunc(out, func(a, b Opportunity) int {
		if a.Unreachable != b.Unreachable {
			if a.Unreachable {
				return 1
			}
			return -1
		}
		if a.Unreachable {
			return cmp.Compare(b.Total, a.Total)
		}
		return cmp.Compare(b.ProfitPerHour, a.ProfitPerHour)
	})
	return out
}

func candidates(costs map[string]*ItemCost, w World, m Market, hp HaulParams, item string, ic *ItemCost) []candidate {
	var out []candidate
	for _, rc := range ic.Routes {
		if !rc.Usable {
			continue
		}
		f := runSystem(rc.Route, w, m, hp)
		for _, p := range []Policy{PolicyCheapest, PolicyFastest} {
			c, t, partial := CostUnder(costs, w, item, rc.Route, p)
			out = append(out, candidate{route: rc.Route, policy: p, credits: c, ticks: t, partial: partial, fSys: f})
		}
	}
	return out
}

// runSystem is F, the system a route's runs happen in: home for hand, the
// line's station system for faction and crft lines, and for an empire route
// the system of the empire line (same recipe and level) nearest home, ties
// broken by station id. "" when unknown or unreachable.
func runSystem(r Route, w World, m Market, hp HaulParams) string {
	switch r.Kind {
	case KindHand:
		return hp.Home
	case KindEmpire:
		bestSys, bestSt, bestD := "", "", math.MaxInt
		for _, l := range w.Lines {
			if !l.Empire || l.RecipeID != r.RecipeID || l.Level != r.Level {
				continue
			}
			sys, ok := m.StationSystem[l.StationID]
			if !ok {
				continue
			}
			d, ok := hp.hops(hp.Home, sys)
			if ok && (d < bestD || (d == bestD && l.StationID < bestSt)) {
				bestSys, bestSt, bestD = sys, l.StationID, d
			}
		}
		return bestSys
	default:
		return m.StationSystem[r.StationID]
	}
}

func evaluate(item string, bid Level, size float64, c candidate, m Market, hp HaulParams) Opportunity {
	o := Opportunity{ItemID: item, StationID: bid.StationID, Bid: bid.Price, Depth: bid.Qty,
		CostPerUnit: c.credits, MarginPerUnit: bid.Price - c.credits,
		RouteLabel: c.route.Label(), Policy: c.policy, Partial: c.partial}
	o.Total = o.MarginPerUnit * o.Depth
	o.MakeHours = o.Depth * c.ticks * tickSeconds / 3600

	jumps, ok := haulJumps(c, m.StationSystem[bid.StationID], hp)
	if !ok {
		o.Unreachable = true
		return o
	}
	trips := 1.0
	if hp.Hold > 0 {
		trips = max(1, math.Ceil(o.Depth*size/hp.Hold))
	}
	o.HaulHours = trips * float64(jumps) * hp.JumpTicks * tickSeconds / 3600
	o.Hours = o.MakeHours + o.HaulHours
	o.ProfitPerHour = o.Total / max(o.Hours, zeroHoursFloor)
	return o
}

// haulJumps is the jumps per trip: 2 × hops(home, bidder) for hand, else
// hops(home, F) + hops(F, bidder) + hops(bidder, home).
func haulJumps(c candidate, bidSys string, hp HaulParams) (int, bool) {
	if bidSys == "" {
		return 0, false
	}
	if c.route.Kind == KindHand {
		d, ok := hp.hops(hp.Home, bidSys)
		return 2 * d, ok
	}
	if c.fSys == "" {
		return 0, false
	}
	a, ok1 := hp.hops(hp.Home, c.fSys)
	b, ok2 := hp.hops(c.fSys, bidSys)
	d, ok3 := hp.hops(bidSys, hp.Home)
	return a + b + d, ok1 && ok2 && ok3
}

// better reports whether a beats b for the same bid level.
func better(a, b Opportunity) bool {
	if a.Unreachable != b.Unreachable {
		return !a.Unreachable
	}
	if a.Partial != b.Partial {
		return !a.Partial
	}
	if a.Unreachable {
		return a.Total > b.Total
	}
	return a.ProfitPerHour > b.ProfitPerHour
}
