package buildcost

// Target is a buildable item or ship: its fully-decomposed BoM requirements and
// (for items) its candidate recipes. RecipeNA, when non-empty, forces Recipe
// mode to NA with that reason (ships, whose sub-assemblies are not market-traded).
type Target struct {
	ID       string
	Kind     string // "item" or "ship"
	BoM      []Requirement
	Recipes  []Recipe
	RecipeNA string
}

// Cell is the computed build cost of one target at one station.
type Cell struct {
	TargetID  string
	StationID string
	BoM       ModeResult
	Recipe    ModeResult
	Margin    Margin
}

// BuildCell computes BoM and Recipe results for target t at a station whose order
// book is b and whose finished-good margin is m.
func BuildCell(t Target, stationID string, b *Book, m Margin) Cell {
	c := Cell{TargetID: t.ID, StationID: stationID, Margin: m}
	c.BoM = b.PriceRequirements(t.BoM)
	if t.RecipeNA != "" {
		c.Recipe = ModeResult{NA: true, NAReason: t.RecipeNA}
	} else {
		c.Recipe = b.CheapestRecipe(t.Recipes)
	}
	return c
}
