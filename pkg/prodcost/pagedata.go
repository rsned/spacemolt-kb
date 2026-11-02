package prodcost

import (
	"maps"
	"slices"
)

// maxPageAsks is how many of an item's cheapest ask levels the page carries.
const maxPageAsks = 10

// Page is the private page's data: everything prodcost.js needs to redo
// CostUnder and Opportunities in the browser under other settings.
type Page struct {
	Home        string  `json:"home"`
	JumpTicks   float64 `json:"jumpTicks"`
	Hold        float64 `json:"hold"`
	TickSeconds float64 `json:"tickSeconds"`
	// Items holds every costed item and every item one of their routes
	// consumes.
	Items map[string]PageItem `json:"items"`
	// Bids is every Station Manager bid level of an item with a usable
	// route, in Opportunities' input order (item id, then book order).
	Bids []PageBid `json:"bids"`
	// Hops is system -> system -> jumps among home, the route run systems and
	// the bidder systems, stored once per pair with the lower id first.
	Hops map[string]map[string]int `json:"hops"`
}

// PageItem is one item. Raw items and unpriced items have no routes.
type PageItem struct {
	Name string    `json:"name"`
	Size float64   `json:"size"`
	Raw  bool      `json:"raw,omitempty"`
	Asks []PageAsk `json:"asks,omitempty"`
	// Routes is ItemCost.Routes in order; Cheapest and Fastest index it, -1
	// when there is no usable route.
	Routes   []PageRoute `json:"routes,omitempty"`
	Cheapest int         `json:"cheapest"`
	Fastest  int         `json:"fastest"`
}

// PageAsk is one ask level; Hops is its station's jumps to home, -1 when
// unknown.
type PageAsk struct {
	Station string  `json:"station"`
	Price   float64 `json:"price"`
	Qty     float64 `json:"qty"`
	Hops    int     `json:"hops"`
}

// PageRoute is one route's per-run values plus the engine's tree totals.
type PageRoute struct {
	Label  string  `json:"label"`
	Recipe string  `json:"recipe"`
	Kind   string  `json:"kind"`
	Usable bool    `json:"usable"`
	Labor  float64 `json:"labor"`
	Fee    float64 `json:"fee"`
	Ticks  float64 `json:"ticks"` // per run
	Out    float64 `json:"out"`   // per run
	// Sys is the system the runs happen in (see Opportunities); "" unknown.
	Sys    string      `json:"sys"`
	Inputs []PageInput `json:"inputs"`
	// CreditsPerUnit, TicksPerUnit and Partial are the RouteCost values.
	CreditsPerUnit float64 `json:"creditsPerUnit"`
	TicksPerUnit   float64 `json:"ticksPerUnit"`
	Partial        bool    `json:"partial,omitempty"`
}

// PageInput is one input, in units per output unit.
type PageInput struct {
	Item string  `json:"item"`
	Qty  float64 `json:"qty"`
}

// PageBid is one Station Manager bid level; Sys is "" when unknown.
type PageBid struct {
	Item    string  `json:"item"`
	Station string  `json:"station"`
	Sys     string  `json:"sys"`
	Price   float64 `json:"price"`
	Qty     float64 `json:"qty"`
}

// PageData builds the private page's data from Compute's output. Items
// absent from costs (such as Options.SkipInputs items) appear only where a
// costed route consumes them.
func PageData(w World, costs map[string]*ItemCost, m Market, hp HaulParams) Page {
	p := Page{Home: hp.Home, JumpTicks: hp.JumpTicks, Hold: hp.Hold, TickSeconds: tickSeconds,
		Items: map[string]PageItem{}, Hops: map[string]map[string]int{}}
	systems := map[string]bool{hp.Home: true}
	item := func(id string) PageItem {
		it := w.Items[id]
		pi := PageItem{Name: id, Size: it.Size, Raw: len(it.ExtractedBy) > 0, Cheapest: -1, Fastest: -1}
		if it.Name != "" {
			pi.Name = it.Name
		}
		for _, l := range m.Asks[id][:min(maxPageAsks, len(m.Asks[id]))] {
			hops := -1
			if d, ok := hp.hops(m.StationSystem[l.StationID], hp.Home); ok {
				hops = d
			}
			pi.Asks = append(pi.Asks, PageAsk{Station: l.StationID, Price: l.Price, Qty: l.Qty, Hops: hops})
		}
		return pi
	}

	for _, id := range slices.Sorted(maps.Keys(costs)) {
		ic := costs[id]
		pi := item(id)
		pi.Cheapest, pi.Fastest = ic.Cheapest, ic.Fastest
		for _, rc := range ic.Routes {
			pr := PageRoute{Label: rc.Label(), Recipe: rc.RecipeID, Kind: string(rc.Kind), Usable: rc.Usable,
				Labor: rc.Labor, Fee: rc.Fee, Ticks: rc.TicksPerRun, Out: rc.OutPerRun,
				Sys: runSystem(rc.Route, w, m, hp), Inputs: []PageInput{},
				CreditsPerUnit: rc.CreditsPerUnit, TicksPerUnit: rc.TicksPerUnit, Partial: rc.Partial || rc.Cycle}
			if rc.Usable && pr.Sys != "" {
				systems[pr.Sys] = true
			}
			for _, in := range rc.Inputs {
				pr.Inputs = append(pr.Inputs, PageInput{Item: in.ItemID, Qty: in.QtyPerUnit})
				if _, ok := p.Items[in.ItemID]; !ok && costs[in.ItemID] == nil {
					p.Items[in.ItemID] = item(in.ItemID)
				}
			}
			pi.Routes = append(pi.Routes, pr)
		}
		p.Items[id] = pi

		if ic.NoUsableRoute {
			continue
		}
		for _, b := range m.Bids[id] {
			sys := m.StationSystem[b.StationID]
			if sys != "" {
				systems[sys] = true
			}
			p.Bids = append(p.Bids, PageBid{Item: id, Station: b.StationID, Sys: sys, Price: b.Price, Qty: b.Qty})
		}
	}

	sys := slices.Sorted(maps.Keys(systems))
	for i, a := range sys {
		for _, b := range sys[i+1:] {
			if d, ok := hp.hops(a, b); ok {
				if p.Hops[a] == nil {
					p.Hops[a] = map[string]int{}
				}
				p.Hops[a][b] = d
			}
		}
	}
	return p
}
