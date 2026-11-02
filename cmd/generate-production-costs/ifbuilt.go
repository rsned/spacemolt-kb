package main

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/prodcost"
)

// unitCost is item's cheapest usable production cost per unit; raw resources
// cost 0. ok is false when the item has no complete usable cost.
func unitCost(w prodcost.World, costs map[string]*prodcost.ItemCost, item string) (credits float64, ok bool) {
	if it, found := w.Items[item]; found && len(it.ExtractedBy) > 0 {
		return 0, true
	}
	ic := costs[item]
	if ic == nil || ic.Cheapest < 0 {
		return 0, false
	}
	r := ic.Routes[ic.Cheapest]
	return r.CreditsPerUnit, !r.Partial && !r.Cycle
}

// buildCost is what building facID costs from nothing: its build credits and
// materials plus those of every level it upgrades from, with materials valued
// at their cheapest usable production cost. partial is true when a material
// or a level could not be costed.
func buildCost(w prodcost.World, costs map[string]*prodcost.ItemCost, facID string) (credits float64, partial bool) {
	seen := map[string]bool{}
	for id := facID; id != "" && !seen[id]; {
		seen[id] = true
		f, ok := w.Facilities[id]
		if !ok {
			return credits, true
		}
		credits += float64(f.BuildCost)
		for _, m := range f.BuildMaterials {
			c, ok := unitCost(w, costs, m.ItemID)
			partial = partial || !ok
			credits += m.Quantity * c
		}
		id = f.UpgradesFrom
	}
	return credits, partial
}

// ifBuilt picks the unbuilt own routes worth flagging: the cheapest one when
// it beats the cheapest usable route on credits, and the fastest one when it
// beats the fastest usable route on time. When nothing is usable, the best
// own routes are flagged outright. -1 means none.
func ifBuilt(ic *prodcost.ItemCost) (cheap, fast int) {
	cheap, fast = -1, -1
	for i, r := range ic.Routes {
		if r.Kind != prodcost.KindOwn || r.Partial || r.Cycle {
			continue
		}
		if (ic.Cheapest < 0 || r.CreditsPerUnit < ic.Routes[ic.Cheapest].CreditsPerUnit) &&
			(cheap < 0 || r.CreditsPerUnit < ic.Routes[cheap].CreditsPerUnit) {
			cheap = i
		}
		if (ic.Fastest < 0 || r.TicksPerUnit < ic.Routes[ic.Fastest].TicksPerUnit) &&
			(fast < 0 || r.TicksPerUnit < ic.Routes[fast].TicksPerUnit) {
			fast = i
		}
	}
	return cheap, fast
}

// payback is the number of units an own route must make before its saving
// over the cheapest usable route covers its build cost; ok is false when
// there is no usable route to save against or no saving.
func payback(ic *prodcost.ItemCost, r prodcost.RouteCost, build float64) (units float64, ok bool) {
	if ic.Cheapest < 0 {
		return 0, false
	}
	saving := ic.Routes[ic.Cheapest].CreditsPerUnit - r.CreditsPerUnit
	if saving <= 0 {
		return 0, false
	}
	return math.Ceil(build / saving), true
}
