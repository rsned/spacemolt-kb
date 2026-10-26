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

// TickSeconds is the length of one game tick in real time.
const TickSeconds = 10

// DefaultMaintenanceCycleTicks is how often a station's facilities consume
// their maintenance inputs. Player-reported (2026-10-26) as ~3,000 ticks and
// not yet measured by us; it is NOT the 100-tick facility rent cycle. Guides
// can override it per file once a measured value exists.
const DefaultMaintenanceCycleTicks = 3000

// CyclesPerDay is how many cycles of cycleTicks run in a real-time day.
func CyclesPerDay(cycleTicks int) float64 {
	return 86400 / float64(cycleTicks*TickSeconds)
}

// PerDay converts a per-cycle quantity to a per-day quantity for a cycle of
// cycleTicks. It is not rounded: at ~2.9 cycles a day, rounding hides most of
// the value, so callers format it.
func PerDay(perCycle, cycleTicks int) float64 {
	return float64(perCycle) * CyclesPerDay(cycleTicks)
}

// FuelItemID is the key under which a facility's maintenance_fuel is counted,
// so upkeep totals stay keyed by item id.
const FuelItemID = "fuel"

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

// Contribution returns what one step adds on its own. Accumulate is the
// running sum of these, so a client that lets the reader swap a step's
// facility can re-add contributions and reach the same totals. An unknown
// facility id is an error.
func Contribution(facilities map[string]catalog.Facility, s StepInput) (Totals, error) {
	c := Totals{Credits: s.ExtraCredits, Materials: map[string]int{}, UpkeepPerCycle: map[string]int{}}
	if s.FacilityID == "" {
		return c, nil
	}
	f, ok := facilities[s.FacilityID]
	if !ok {
		return c, fmt.Errorf("unknown facility %q", s.FacilityID)
	}
	c.Credits += f.BuildCost
	for _, mat := range f.BuildMaterials {
		c.Materials[mat.ItemID] += qty(mat)
	}
	for _, mat := range f.MaintenanceInputs {
		c.UpkeepPerCycle[mat.ItemID] += qty(mat)
	}
	if f.MaintenanceFuel > 0 {
		c.UpkeepPerCycle[FuelItemID] += f.MaintenanceFuel
	}
	c.PowerSupply = f.PowerSupply
	c.PowerDraw = f.PowerDraw
	c.LifeSupportSupply = f.LifeSupportSupply
	c.LifeSupportDraw = f.LifeSupportDraw
	return c, nil
}

// Plus returns t with c added. The result owns its maps.
func (t Totals) Plus(c Totals) Totals {
	out := t
	out.Materials = maps.Clone(t.Materials)
	out.UpkeepPerCycle = maps.Clone(t.UpkeepPerCycle)
	if out.Materials == nil {
		out.Materials = map[string]int{}
	}
	if out.UpkeepPerCycle == nil {
		out.UpkeepPerCycle = map[string]int{}
	}
	out.Credits += c.Credits
	for id, q := range c.Materials {
		out.Materials[id] += q
	}
	for id, q := range c.UpkeepPerCycle {
		out.UpkeepPerCycle[id] += q
	}
	out.PowerSupply += c.PowerSupply
	out.PowerDraw += c.PowerDraw
	out.LifeSupportSupply += c.LifeSupportSupply
	out.LifeSupportDraw += c.LifeSupportDraw
	return out
}

// Accumulate returns the cumulative Totals after each step, in order. Each
// returned Totals owns its maps. An unknown facility id is an error.
func Accumulate(facilities map[string]catalog.Facility, steps []StepInput) ([]Totals, error) {
	cur := Totals{Materials: map[string]int{}, UpkeepPerCycle: map[string]int{}}
	out := make([]Totals, 0, len(steps))
	for i, s := range steps {
		c, err := Contribution(facilities, s)
		if err != nil {
			return nil, fmt.Errorf("step %d: %w", i, err)
		}
		cur = cur.Plus(c)
		out = append(out, cur)
	}
	return out, nil
}

// qty converts a catalog quantity (a float in the source JSON) to a count.
func qty(m catalog.Material) int { return int(math.Round(m.Quantity)) }
