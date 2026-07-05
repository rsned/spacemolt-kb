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
