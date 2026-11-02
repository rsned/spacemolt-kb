// Package prodcost models the production cost of catalog items.
package prodcost

// RouteKind names how a recipe run is performed.
type RouteKind string

// Route kinds.
const (
	KindHand    RouteKind = "hand"
	KindOwn     RouteKind = "own"
	KindEmpire  RouteKind = "empire"
	KindFaction RouteKind = "faction"
	KindCRFT    RouteKind = "crft"
)

// Line is one captured public_facilities or CRFT faction_facilities row.
type Line struct {
	StationID, StationName, FactionID, FactionTag, RecipeID string

	Level int
	// Fee is rental_fee_per_run; 0 for crft.
	Fee float64
	// TicksPerRun is the captured value; 0 means unknown, so it is computed
	// from the level.
	TicksPerRun float64
	// Empire is true when owner_faction is empty.
	Empire bool
	// Kind is set by the loader to KindFaction or KindCRFT for non-empire lines.
	Kind RouteKind
}

// Route is one way to produce a recipe's output.
type Route struct {
	RecipeID                           string
	Kind                               RouteKind
	Level                              int
	Labor, Fee                         float64
	TicksPerRun, OutPerRun             float64
	StationID, StationName, FactionTag string // empty for hand/own/empire
	Usable                             bool
	Tag                                string // "pirate base only" or ""
}
