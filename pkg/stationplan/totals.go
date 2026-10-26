// Package stationplan does the arithmetic behind planning a player station:
// the running build bill of materials, upkeep per cycle and per day, and the
// power / life-support balance as facilities are added in order. The guides
// generator uses it for its build-step sidebar; a future station planner
// (tick facilities, see the totals) is meant to reuse it unchanged.
package stationplan

import (
	"fmt"
	"maps"
	"math"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// CyclesPerDay is how many upkeep cycles run in a real-time day: a cycle is
// 100 ticks and a tick is 10 seconds.
const CyclesPerDay = 86.4

// FuelItemID is the key under which a facility's maintenance_fuel is counted,
// so upkeep totals stay keyed by item id.
const FuelItemID = "fuel"

// PerDay converts a per-cycle quantity to a rounded per-day quantity.
func PerDay(perCycle int) int {
	return int(math.Round(float64(perCycle) * CyclesPerDay))
}

// StepInput is one build step. An empty FacilityID is a non-facility step
// (e.g. founding); ExtraCredits adds hand-stated costs such as the founding fee.
type StepInput struct {
	FacilityID   string
	ExtraCredits int
}

// Totals is the station's cumulative state after a step.
type Totals struct {
	Credits           int
	Materials         map[string]int // build materials spent, by item id
	UpkeepPerCycle    map[string]int // upkeep consumed per cycle, by item id (fuel as FuelItemID)
	PowerSupply       int
	PowerDraw         int
	LifeSupportSupply int
	LifeSupportDraw   int
}

// PowerDeficit reports whether the facilities so far draw more power than they supply.
func (t Totals) PowerDeficit() bool { return t.PowerDraw > t.PowerSupply }

// LifeSupportDeficit reports whether life-support draw exceeds supply.
func (t Totals) LifeSupportDeficit() bool { return t.LifeSupportDraw > t.LifeSupportSupply }

// Accumulate returns the cumulative Totals after each step, in order. Each
// returned Totals owns its maps. An unknown facility id is an error.
func Accumulate(facilities map[string]catalog.Facility, steps []StepInput) ([]Totals, error) {
	cur := Totals{Materials: map[string]int{}, UpkeepPerCycle: map[string]int{}}
	out := make([]Totals, 0, len(steps))
	for i, s := range steps {
		cur.Credits += s.ExtraCredits
		if s.FacilityID != "" {
			f, ok := facilities[s.FacilityID]
			if !ok {
				return nil, fmt.Errorf("step %d: unknown facility %q", i, s.FacilityID)
			}
			cur.Credits += f.BuildCost
			for _, mat := range f.BuildMaterials {
				cur.Materials[mat.ItemID] += qty(mat)
			}
			for _, mat := range f.MaintenanceInputs {
				cur.UpkeepPerCycle[mat.ItemID] += qty(mat)
			}
			if f.MaintenanceFuel > 0 {
				cur.UpkeepPerCycle[FuelItemID] += f.MaintenanceFuel
			}
			cur.PowerSupply += f.PowerSupply
			cur.PowerDraw += f.PowerDraw
			cur.LifeSupportSupply += f.LifeSupportSupply
			cur.LifeSupportDraw += f.LifeSupportDraw
		}
		snap := cur
		snap.Materials = maps.Clone(cur.Materials)
		snap.UpkeepPerCycle = maps.Clone(cur.UpkeepPerCycle)
		out = append(out, snap)
	}
	return out, nil
}

// qty converts a catalog quantity (a float in the source JSON) to a count.
func qty(m catalog.Material) int { return int(math.Round(m.Quantity)) }
