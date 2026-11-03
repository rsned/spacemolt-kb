package prodcost

import (
	"cmp"
	"math"
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
	// SkipInputs leaves out every recipe with one of these items as an input
	// or as outputs[0], so the items themselves are never costed either.
	SkipInputs map[string]bool
	// UnbuiltL1 makes facility-only items costable: an item with no usable
	// route at all gets, for each of its recipes, the own routes at that
	// recipe's lowest own level promoted to Usable, as if that facility were
	// built at its lowest level. The promotion is per item, so an item that
	// already has any usable route is left alone. Consumers tell a promoted
	// step by Kind == KindOwn && Usable.
	UnbuiltL1 bool
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
	onStack  map[string]int // item -> stack depth
	// complete is each item's cheapest complete route, from settle.
	complete map[string]RouteCost
	// settling makes route treat every input without a complete cost as a
	// cycle instead of searching it.
	settling bool
	// unbuilt is Options.UnbuiltL1: routes promotes unbuilt own routes.
	unbuilt bool
}

// noDep is the "depends on no ancestor" depth returned by item and route.
const noDep = math.MaxInt

// Compute costs every item that is outputs[0] of at least one recipe not
// skipped by opt. Raw and unpriced items get no entry.
//
// It runs in two passes. settle first finds every item's cheapest complete
// route as a fixed point, so an input with any complete route is always
// costed on it, however the recipes loop. A memoised depth-first search then
// costs every route, recursing only into inputs with no complete route. An
// input already on that search's stack marks the route Cycle and contributes
// nothing; a result that hit an ancestor still on the stack is used only by
// that ancestor's search and never memoised. So a route is Cycle only when it
// cannot be completed without the loop.
func Compute(w World, opt Options) map[string]*ItemCost {
	e := &engine{w: w, byOutput: map[string][]catalog.Recipe{},
		memo: map[string]*ItemCost{}, onStack: map[string]int{}, complete: map[string]RouteCost{}, unbuilt: opt.UnbuiltL1}
	var order []string
	for _, rec := range w.Recipes {
		if len(rec.Outputs) == 0 || opt.SkipInputs[rec.Outputs[0].ItemID] || slices.ContainsFunc(rec.Inputs, func(m catalog.Material) bool { return opt.SkipInputs[m.ItemID] }) {
			continue
		}
		id := rec.Outputs[0].ItemID
		if _, ok := e.byOutput[id]; !ok {
			order = append(order, id)
		}
		e.byOutput[id] = append(e.byOutput[id], rec)
	}
	e.settle(order)
	for _, id := range order {
		e.item(id) // at the root, so always memoised
	}
	return e.memo
}

func (e *engine) raw(id string) bool {
	it, ok := e.w.Items[id]
	return ok && len(it.ExtractedBy) > 0
}

// settle fills e.complete by relaxing every item's routes until no cheapest
// complete route changes. Costs only fall once set; the round cap stops a
// loop whose output per unit of input exceeds 1 from shrinking forever.
func (e *engine) settle(order []string) {
	e.settling = true
	defer func() { e.settling = false }()
	for range len(order) + 1 {
		changed := false
		for _, id := range order {
			var rs []RouteCost
			for _, rr := range e.routes(id) {
				if rc, _ := e.route(rr.rec, rr.r); rc.complete() {
					rs = append(rs, rc)
				}
			}
			sortRoutes(rs)
			i := best(rs, cheaper)
			if i < 0 {
				continue
			}
			old, ok := e.complete[id]
			if !ok || old.Route != rs[i].Route || old.CreditsPerUnit != rs[i].CreditsPerUnit ||
				old.TicksPerUnit != rs[i].TicksPerUnit || old.RawPerUnit != rs[i].RawPerUnit {
				e.complete[id] = rs[i]
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

// recRoute is one route of one recipe.
type recRoute struct {
	rec catalog.Recipe
	r   Route
}

// routes is every route of every recipe producing id. When UnbuiltL1 is set
// and no recipe of id has a usable route, each recipe's lowest-level own
// routes are promoted to Usable (per item, not per recipe).
func (e *engine) routes(id string) []recRoute {
	var out []recRoute
	anyUsable := false
	for _, rec := range e.byOutput[id] {
		for _, r := range Routes(rec, e.w.Facilities, e.w.Lines) {
			anyUsable = anyUsable || r.Usable
			out = append(out, recRoute{rec, r})
		}
	}
	if !e.unbuilt || anyUsable {
		return out
	}
	lowest := map[string]int{}
	for _, rr := range out {
		if rr.r.Kind == KindOwn {
			if l, ok := lowest[rr.rec.ID]; !ok || rr.r.Level < l {
				lowest[rr.rec.ID] = rr.r.Level
			}
		}
	}
	for i, rr := range out {
		if l, ok := lowest[rr.rec.ID]; ok && rr.r.Kind == KindOwn && rr.r.Level == l {
			out[i].r.Usable = true
		}
	}
	return out
}

// sortRoutes orders routes credits asc, then ticks asc, stably.
func sortRoutes(rs []RouteCost) {
	slices.SortStableFunc(rs, func(a, b RouteCost) int {
		return cmp.Or(cmp.Compare(a.CreditsPerUnit, b.CreditsPerUnit), cmp.Compare(a.TicksPerUnit, b.TicksPerUnit))
	})
}

// item costs id, which must have at least one recipe in byOutput. dep is
// the smallest stack depth of an ancestor the result looped back onto, or
// noDep; only a noDep result is memoised.
func (e *engine) item(id string) (ic *ItemCost, dep int) {
	if ic, ok := e.memo[id]; ok {
		return ic, noDep
	}
	depth := len(e.onStack)
	e.onStack[id] = depth
	ic, dep = &ItemCost{ItemID: id}, noDep
	for _, rr := range e.routes(id) {
		rc, d := e.route(rr.rec, rr.r)
		ic.Routes = append(ic.Routes, rc)
		dep = min(dep, d)
	}
	sortRoutes(ic.Routes)
	ic.Cheapest, ic.Fastest = best(ic.Routes, cheaper), best(ic.Routes, faster)
	ic.NoUsableRoute = ic.Cheapest < 0
	delete(e.onStack, id)
	if dep >= depth {
		// Looped back only onto id itself: complete as seen from the root.
		e.memo[id] = ic
		dep = noDep
	}
	return ic, dep
}

// route costs one route of rec over its input tree.
// dep is as for item.
func (e *engine) route(rec catalog.Recipe, r Route) (rc RouteCost, dep int) {
	rc, dep = RouteCost{Route: r}, noDep
	if r.OutPerRun <= 0 {
		rc.Partial = true
		return rc, dep
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
		stackDepth, looped := e.onStack[in.ItemID]
		done, isComplete := e.complete[in.ItemID]
		switch {
		case e.raw(in.ItemID):
			line.Via = "raw"
			rc.RawPerUnit += line.QtyPerUnit
		case len(e.byOutput[in.ItemID]) == 0:
			line.Via = "unpriced"
			addUnpriced(in.ItemID)
		case isComplete:
			addInput(&rc, &line, done)
		case looped || e.settling:
			line.Via = "cycle"
			rc.Cycle = true
			if looped {
				dep = min(dep, stackDepth)
			}
		default:
			sub, d := e.item(in.ItemID)
			dep = min(dep, d)
			if sub.NoUsableRoute {
				line.Via = "unpriced"
				addUnpriced(in.ItemID)
				break
			}
			c := sub.Routes[sub.Cheapest]
			addInput(&rc, &line, c)
			if c.Partial {
				addUnpriced(c.Unpriced...)
			}
			rc.Cycle = rc.Cycle || c.Cycle
		}
		rc.Inputs = append(rc.Inputs, line)
	}
	return rc, dep
}

// addInput adds line's input, made on route c, to rc.
func addInput(rc *RouteCost, line *InputLine, c RouteCost) {
	line.Via = c.Label()
	rc.CreditsPerUnit += line.QtyPerUnit * c.CreditsPerUnit
	rc.TicksPerUnit += line.QtyPerUnit * c.TicksPerUnit
	rc.RawPerUnit += line.QtyPerUnit * c.RawPerUnit
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
