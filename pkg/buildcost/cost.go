package buildcost

// Requirement is a quantity of a material needed to build a target.
type Requirement struct {
	ItemID string
	Qty    float64
}

// Shortfall records an unmet material quantity.
type Shortfall struct {
	ItemID string
	Short  float64
}

// ModeResult is a per-station build cost for one mode (BoM or Recipe).
// Cost is the total, or the partial cost of covered materials when infeasible.
// Covered/Total count materials fully satisfiable from depth. RecipeID names the
// chosen recipe (Recipe mode only). NA marks a mode that does not apply to this
// target (e.g. ship Recipe mode); NAReason explains why.
type ModeResult struct {
	Cost       float64
	Covered    int
	Total      int
	Feasible   bool
	Shortfalls []Shortfall
	RecipeID   string
	NA         bool
	NAReason   string
}

// PriceRequirements walks each requirement against the book and aggregates the
// cost, coverage, and shortfalls. Feasible is true only if every requirement is
// fully covered.
func (b *Book) PriceRequirements(reqs []Requirement) ModeResult {
	res := ModeResult{Total: len(reqs), Feasible: true}
	for _, r := range reqs {
		w := b.Walk(r.ItemID, r.Qty)
		res.Cost += w.Cost
		if w.Shortfall <= 0 {
			res.Covered++
		} else {
			res.Feasible = false
			res.Shortfalls = append(res.Shortfalls, Shortfall{ItemID: r.ItemID, Short: w.Shortfall})
		}
	}
	return res
}

// Recipe is one way to produce a target: its direct inputs and how many units
// of the target it yields (OutputQty, at least 1).
type Recipe struct {
	ID        string
	OutputQty float64
	Inputs    []Requirement
}

// CheapestRecipe prices every recipe and returns the cheapest feasible one
// (per-unit cost = total input cost / OutputQty), tagging RecipeID. When no
// recipe is feasible, it returns the lowest partial-cost result (still
// infeasible). An empty recipe list yields an NA result.
func (b *Book) CheapestRecipe(recipes []Recipe) ModeResult {
	if len(recipes) == 0 {
		return ModeResult{NA: true, NAReason: "no recipe"}
	}
	var best ModeResult
	var haveBest bool
	for _, rec := range recipes {
		r := b.PriceRequirements(rec.Inputs)
		out := rec.OutputQty
		if out < 1 {
			out = 1
		}
		r.Cost /= out
		r.RecipeID = rec.ID
		if !haveBest || better(r, best) {
			best, haveBest = r, true
		}
	}
	return best
}

// better reports whether candidate c should replace the current best: a feasible
// result always beats an infeasible one; among same feasibility, lower cost wins.
func better(c, best ModeResult) bool {
	if c.Feasible != best.Feasible {
		return c.Feasible
	}
	return c.Cost < best.Cost
}
