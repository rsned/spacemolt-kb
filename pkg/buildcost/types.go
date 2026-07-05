// Package buildcost computes the live-market cost, feasibility, and margin of
// building an item or ship at a station, walking order-book depth. It is pure:
// callers supply in-memory order books; the package touches no database.
package buildcost

// Order is one resting order at a price level.
type Order struct {
	Price float64
	Qty   float64
}

// Ladder is the price-sorted resting depth for one (station, item) sell side.
// Sell ladders are sorted ascending by Price (cheapest first).
type Ladder []Order

// Book is the current order book at a single station.
// Sell maps item_id to its ascending sell ladder.
// BestBuy maps item_id to the highest resting buy price (0 if none).
type Book struct {
	Sell    map[string]Ladder
	BestBuy map[string]float64
}

// WalkResult is the outcome of covering a required quantity from a sell ladder.
// Cost and Covered describe what was actually purchasable; Shortfall is the
// unmet quantity (0 when fully covered).
type WalkResult struct {
	Cost      float64
	Covered   float64
	Shortfall float64
}
