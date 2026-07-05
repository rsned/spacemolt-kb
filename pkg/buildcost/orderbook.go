package buildcost

// Walk covers qty of itemID by consuming the sell ladder cheapest-first.
// It returns the cost of what was covered and any shortfall.
func (b *Book) Walk(itemID string, qty float64) WalkResult {
	remaining := qty
	var cost float64
	for _, o := range b.Sell[itemID] {
		if remaining <= 0 {
			break
		}
		take := o.Qty
		if take > remaining {
			take = remaining
		}
		cost += take * o.Price
		remaining -= take
	}
	covered := qty - remaining
	if remaining < 0 {
		remaining = 0
	}
	return WalkResult{Cost: cost, Covered: covered, Shortfall: remaining}
}
