package prodcost

import (
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// levelTicks is the ticks per run of a recipe at a facility level.
func levelTicks(craftingTime float64, level int) float64 {
	return craftingTime / (5 * math.Pow(3, float64(level-1)))
}

// num formats a number without a trailing ".0", with up to 2 decimals.
func num(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// Label renders the route as shown in cost tables.
func (r Route) Label() string {
	var s string
	switch r.Kind {
	case KindHand:
		return "hand"
	case KindOwn:
		s = fmt.Sprintf("own L%d (labor %s)", r.Level, num(r.Labor))
	case KindEmpire:
		s = fmt.Sprintf("empire L%d (labor %s + fee %s)", r.Level, num(r.Labor), num(r.Fee))
	case KindCRFT:
		s = fmt.Sprintf("%s L%d @ %s (labor %s)", r.FactionTag, r.Level, r.StationName, num(r.Labor))
	default:
		s = fmt.Sprintf("%s L%d @ %s (labor %s + fee %s)", r.FactionTag, r.Level, r.StationName, num(r.Labor), num(r.Fee))
	}
	if r.Tag != "" {
		s += " · " + r.Tag
	}
	return s
}

// Routes lists every way to produce rec, given the facility catalog and the
// captured lines (only lines for rec are used).
func Routes(rec catalog.Recipe, facs map[string]catalog.Facility, lines []Line) []Route {
	var out []Route
	var outPerRun float64
	if len(rec.Outputs) > 0 {
		outPerRun = rec.Outputs[0].Quantity
	}
	if rec.HandCraftable {
		out = append(out, Route{RecipeID: rec.ID, Kind: KindHand, Usable: true,
			TicksPerRun: rec.CraftingTime, OutPerRun: outPerRun})
	}

	// Producing facilities that exist in the catalog.
	var producers []catalog.Facility
	for _, id := range rec.ProducedBy {
		if f, ok := facs[id]; ok {
			producers = append(producers, f)
		}
	}

	// laborAt is the labor of the producer whose level equals lvl; failing
	// that the nearest lower level's, else 0.
	laborAt := func(lvl int) float64 {
		best, found := -1, false
		var labor float64
		for _, f := range producers {
			if f.Level <= lvl && f.Level > best {
				best, labor, found = f.Level, float64(f.LaborCost), true
			}
		}
		if !found {
			return 0
		}
		return labor
	}

	for _, f := range producers {
		if f.Empire != "" {
			continue
		}
		r := Route{RecipeID: rec.ID, Kind: KindOwn, Level: f.Level, Labor: float64(f.LaborCost),
			TicksPerRun: levelTicks(rec.CraftingTime, f.Level), OutPerRun: outPerRun}
		if f.PirateBaseOnly {
			r.Tag = "pirate base only"
		}
		out = append(out, r)
	}

	empireFee := map[int]float64{}
	var empireLevels []int
	for _, l := range lines {
		if l.RecipeID != rec.ID {
			continue
		}
		if l.Empire {
			if fee, ok := empireFee[l.Level]; !ok {
				empireFee[l.Level] = l.Fee
				empireLevels = append(empireLevels, l.Level)
			} else {
				empireFee[l.Level] = math.Min(fee, l.Fee)
			}
			continue
		}
		ticks := l.TicksPerRun
		if ticks == 0 {
			ticks = levelTicks(rec.CraftingTime, l.Level)
		}
		kind := l.Kind
		if kind != KindCRFT {
			kind = KindFaction
		}
		r := Route{RecipeID: rec.ID, Kind: kind, Level: l.Level, Labor: laborAt(l.Level),
			TicksPerRun: ticks, OutPerRun: outPerRun, Usable: true,
			StationID: l.StationID, StationName: l.StationName, FactionTag: l.FactionTag}
		if kind == KindFaction {
			r.Fee = l.Fee
		}
		out = append(out, r)
	}

	slices.Sort(empireLevels)
	for _, lvl := range empireLevels {
		out = append(out, Route{RecipeID: rec.ID, Kind: KindEmpire, Level: lvl, Labor: laborAt(lvl),
			Fee: empireFee[lvl], TicksPerRun: levelTicks(rec.CraftingTime, lvl), OutPerRun: outPerRun, Usable: true})
	}
	return out
}
