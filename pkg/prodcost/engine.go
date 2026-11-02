package prodcost

import (
	"cmp"
	"slices"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// World is everything the costing engine reads.
type World struct {
	Items      map[string]catalog.Item
	Recipes    []catalog.Recipe
	Facilities map[string]catalog.Facility
	Lines      []Line
}

// Options tunes Compute.
type Options struct {
	// SkipInputs leaves out every recipe with one of these items as an input.
	SkipInputs map[string]bool
}

// InputLine is one input of a costed route.
type InputLine struct {
	ItemID     string
	QtyPerUnit float64 // input units per 1 output unit
	Via        string  // chosen route label for that input; "raw", "unpriced" or "cycle"
}

// RouteCost is a Route costed over its whole input tree.
type RouteCost struct {
	Route
	CreditsPerUnit, TicksPerUnit float64 // whole tree; usable inputs on their cheapest route
	RawPerUnit                   float64 // raw resource units per output unit (for mine-fuel)
	Partial                      bool
	Unpriced                     []string
	Cycle                        bool
	Inputs                       []InputLine
}

// complete reports whether the cost covers the whole tree.
func (r RouteCost) complete() bool { return !r.Partial && !r.Cycle }

// ItemCost holds every costed route of an item.
type ItemCost struct {
	ItemID string
	// Routes is every route of every recipe whose outputs[0] is the item,
	// sorted credits asc then ticks asc.
	Routes []RouteCost
	// Cheapest and Fastest index Routes among Usable routes; -1 when none.
	Cheapest, Fastest int
	NoUsableRoute     bool
}

// cheaper orders routes complete first, then credits, then ticks.
func cheaper(a, b RouteCost) int {
	if a.complete() != b.complete() {
		if a.complete() {
			return -1
		}
		return 1
	}
	return cmp.Or(cmp.Compare(a.CreditsPerUnit, b.CreditsPerUnit), cmp.Compare(a.TicksPerUnit, b.TicksPerUnit))
}

// faster orders routes complete first, then ticks, then credits.
func faster(a, b RouteCost) int {
	if a.complete() != b.complete() {
		if a.complete() {
			return -1
		}
		return 1
	}
	return cmp.Or(cmp.Compare(a.TicksPerUnit, b.TicksPerUnit), cmp.Compare(a.CreditsPerUnit, b.CreditsPerUnit))
}

// best returns the index of the usable route that sorts first under order,
// or -1 when no route is usable.
func best(rs []RouteCost, order func(a, b RouteCost) int) int {
	idx := -1
	for i, r := range rs {
		if r.Usable && (idx < 0 || order(r, rs[idx]) < 0) {
			idx = i
		}
	}
	return idx
}

type engine struct {
	w        World
	byOutput map[string][]catalog.Recipe
	memo     map[string]*ItemCost
	onStack  map[string]bool
}

// Compute costs every item that is outputs[0] of at least one recipe not
// skipped by opt. Raw and unpriced items get no entry.
//
// It is a memoised depth-first search. An input already on the stack marks
// the route Cycle and contributes nothing. Because an item first reached
// inside a cycle is memoised with that cycle broken where the search entered
// it, results around cycles depend on the order items are visited; Compute
// visits them in recipe order so the output is deterministic.
func Compute(w World, opt Options) map[string]*ItemCost {
	e := &engine{w: w, byOutput: map[string][]catalog.Recipe{},
		memo: map[string]*ItemCost{}, onStack: map[string]bool{}}
	var order []string
	for _, rec := range w.Recipes {
		if len(rec.Outputs) == 0 || slices.ContainsFunc(rec.Inputs, func(m catalog.Material) bool { return opt.SkipInputs[m.ItemID] }) {
			continue
		}
		id := rec.Outputs[0].ItemID
		if _, ok := e.byOutput[id]; !ok {
			order = append(order, id)
		}
		e.byOutput[id] = append(e.byOutput[id], rec)
	}
	for _, id := range order {
		e.item(id)
	}
	return e.memo
}

func (e *engine) raw(id string) bool {
	it, ok := e.w.Items[id]
	return ok && len(it.ExtractedBy) > 0
}

// item costs id, which must have at least one recipe in byOutput.
func (e *engine) item(id string) *ItemCost {
	if ic, ok := e.memo[id]; ok {
		return ic
	}
	e.onStack[id] = true
	ic := &ItemCost{ItemID: id}
	for _, rec := range e.byOutput[id] {
		for _, r := range Routes(rec, e.w.Facilities, e.w.Lines) {
			ic.Routes = append(ic.Routes, e.route(rec, r))
		}
	}
	slices.SortStableFunc(ic.Routes, func(a, b RouteCost) int {
		return cmp.Or(cmp.Compare(a.CreditsPerUnit, b.CreditsPerUnit), cmp.Compare(a.TicksPerUnit, b.TicksPerUnit))
	})
	ic.Cheapest, ic.Fastest = best(ic.Routes, cheaper), best(ic.Routes, faster)
	ic.NoUsableRoute = ic.Cheapest < 0
	delete(e.onStack, id)
	e.memo[id] = ic
	return ic
}

// route costs one route of rec over its input tree.
func (e *engine) route(rec catalog.Recipe, r Route) RouteCost {
	rc := RouteCost{Route: r}
	if r.OutPerRun <= 0 {
		rc.Partial = true
		return rc
	}
	rc.CreditsPerUnit = (r.Labor + r.Fee) / r.OutPerRun
	rc.TicksPerUnit = r.TicksPerRun / r.OutPerRun
	addUnpriced := func(ids ...string) {
		rc.Partial = true
		for _, id := range ids {
			if !slices.Contains(rc.Unpriced, id) {
				rc.Unpriced = append(rc.Unpriced, id)
			}
		}
	}
	for _, in := range rec.Inputs {
		line := InputLine{ItemID: in.ItemID, QtyPerUnit: in.Quantity / r.OutPerRun}
		switch {
		case e.raw(in.ItemID):
			line.Via = "raw"
			rc.RawPerUnit += line.QtyPerUnit
		case len(e.byOutput[in.ItemID]) == 0:
			line.Via = "unpriced"
			addUnpriced(in.ItemID)
		case e.onStack[in.ItemID]:
			line.Via = "cycle"
			rc.Cycle = true
		default:
			sub := e.item(in.ItemID)
			if sub.NoUsableRoute {
				line.Via = "unpriced"
				addUnpriced(in.ItemID)
				break
			}
			c := sub.Routes[sub.Cheapest]
			line.Via = c.Label()
			rc.CreditsPerUnit += line.QtyPerUnit * c.CreditsPerUnit
			rc.TicksPerUnit += line.QtyPerUnit * c.TicksPerUnit
			rc.RawPerUnit += line.QtyPerUnit * c.RawPerUnit
			if c.Partial {
				addUnpriced(c.Unpriced...)
			}
			rc.Cycle = rc.Cycle || c.Cycle
		}
		rc.Inputs = append(rc.Inputs, line)
	}
	return rc
}

// Policy chooses each input's route across a whole costing tree.
type Policy string

// Policies.
const (
	PolicyCheapest Policy = "cheapest"
	PolicyFastest  Policy = "fastest"
)

// CostUnder costs one unit of itemID made on top, with every input below it
// made on its own Cheapest (PolicyCheapest) or Fastest (PolicyFastest)
// route, recursively. Raw inputs cost 0. An input that is unpriced, has no
// usable route, or loops back onto an item already being costed adds 0 and
// sets partial. A top route absent from costs[itemID] yields only its own
// step, marked partial.
func CostUnder(costs map[string]*ItemCost, w World, itemID string, top Route, p Policy) (credits, ticks float64, partial bool) {
	return costUnder(costs, w, itemID, top, p, map[string]bool{})
}

func costUnder(costs map[string]*ItemCost, w World, itemID string, top Route, p Policy, stack map[string]bool) (credits, ticks float64, partial bool) {
	if top.OutPerRun <= 0 {
		return 0, 0, true
	}
	credits = (top.Labor + top.Fee) / top.OutPerRun
	ticks = top.TicksPerRun / top.OutPerRun
	ic := costs[itemID]
	if ic == nil {
		return credits, ticks, true
	}
	i := slices.IndexFunc(ic.Routes, func(rc RouteCost) bool { return rc.Route == top })
	if i < 0 {
		return credits, ticks, true
	}
	stack[itemID] = true
	defer delete(stack, itemID)
	for _, in := range ic.Routes[i].Inputs {
		if it, ok := w.Items[in.ItemID]; in.Via == "raw" || (ok && len(it.ExtractedBy) > 0) {
			continue
		}
		sub := costs[in.ItemID]
		if sub == nil || sub.NoUsableRoute || stack[in.ItemID] {
			partial = true
			continue
		}
		idx := sub.Cheapest
		if p == PolicyFastest {
			idx = sub.Fastest
		}
		c, t, pa := costUnder(costs, w, in.ItemID, sub.Routes[idx].Route, p, stack)
		credits += in.QtyPerUnit * c
		ticks += in.QtyPerUnit * t
		partial = partial || pa
	}
	return credits, ticks, partial
}
