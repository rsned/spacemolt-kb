package buildcost

// Margin holds the finished good's own price at a station for build-vs-buy
// comparison. FinishedAsk is what you'd pay to acquire it; FinishedBid is what
// a buyer will pay you. Has* flags distinguish a genuine 0 from "unknown".
type Margin struct {
	FinishedAsk float64
	FinishedBid float64
	HasAsk      bool
	HasBid      bool
}

// SavingsVsAsk is finished ask minus build cost (positive = building is cheaper).
func (m Margin) SavingsVsAsk(cost float64) (float64, bool) {
	if !m.HasAsk {
		return 0, false
	}
	return m.FinishedAsk - cost, true
}

// ProfitVsBid is finished bid minus build cost (positive = craft-and-sell profit).
func (m Margin) ProfitVsBid(cost float64) (float64, bool) {
	if !m.HasBid {
		return 0, false
	}
	return m.FinishedBid - cost, true
}
