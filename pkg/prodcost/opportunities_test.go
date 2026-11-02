package prodcost

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/galaxyhops"
)

// allHops builds a system-to-system hop table from an undirected edge list.
func allHops(edges ...[2]string) map[string]map[string]int {
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e[0]] = append(adj[e[0]], e[1])
		adj[e[1]] = append(adj[e[1]], e[0])
	}
	out := map[string]map[string]int{}
	for s := range adj {
		out[s] = galaxyhops.BFSHops(adj, s)
	}
	return out
}

// widgetWorld has widget from 1 raw ore: by hand in handTicks ticks, and on a
// fab L1 (labor 10, crafting time handTicks).
func widgetWorld(handTicks float64, lines ...Line) World {
	return World{
		Items: map[string]catalog.Item{
			"ore":    {ID: "ore", Category: "ore", ExtractedBy: []string{"mining"}, Size: 1},
			"widget": {ID: "widget", Size: 1},
		},
		Recipes: []catalog.Recipe{{ID: "make_widget", CraftingTime: handTicks, HandCraftable: true,
			ProducedBy: []string{"fab"},
			Inputs:     []catalog.Material{mat("ore", 1)}, Outputs: []catalog.Material{mat("widget", 1)}}},
		Facilities: map[string]catalog.Facility{"fab": {ID: "fab", Level: 1, LaborCost: 10, Empire: "solarian"}},
		Lines:      lines,
	}
}

func TestOpportunityMarginAndHours(t *testing.T) {
	w := widgetWorld(0.5)
	m := Market{
		Bids:          map[string][]Level{"widget": {{StationID: "bidder", Price: 2230, Qty: 12}}},
		StationSystem: map[string]string{"bidder": "s3"},
	}
	hp := HaulParams{Home: "home", JumpTicks: 4, Hold: 2000,
		Hops: allHops([2]string{"home", "s1"}, [2]string{"s1", "s2"}, [2]string{"s2", "s3"})}
	ops := Opportunities(Compute(w, Options{}), w, m, hp)
	if len(ops) != 1 {
		t.Fatalf("got %d rows: %+v", len(ops), ops)
	}
	o := ops[0]
	if o.ItemID != "widget" || o.StationID != "bidder" || o.Bid != 2230 || o.Depth != 12 || o.CostPerUnit != 0 {
		t.Errorf("row %+v", o)
	}
	if o.Total != 26760 || o.MarginPerUnit != 2230 {
		t.Errorf("Total %v margin %v, want 26760 / 2230", o.Total, o.MarginPerUnit)
	}
	if !near(o.MakeHours, 12*0.5*10/3600.0) || !near(o.HaulHours, 1*6*4*10/3600.0) {
		t.Errorf("MakeHours %v HaulHours %v", o.MakeHours, o.HaulHours)
	}
	if !near(o.Hours, o.MakeHours+o.HaulHours) || !near(o.ProfitPerHour, o.Total/o.Hours) {
		t.Errorf("Hours %v PPH %v", o.Hours, o.ProfitPerHour)
	}
	if o.Unreachable || o.RouteLabel != "hand" || o.Partial {
		t.Errorf("row %+v", o)
	}
}

func TestOpportunitiesPicksBestProfitPerHour(t *testing.T) {
	// Hand: free but 100 ticks/u. Empire L1: 15 credits/u, 20 ticks/u. The
	// nearer empire station (emp_near, 1 hop from home) is F.
	lines := []Line{
		{StationID: "emp_far", RecipeID: "make_widget", Level: 1, Fee: 5, Empire: true},
		{StationID: "emp_near", RecipeID: "make_widget", Level: 1, Fee: 5, Empire: true},
	}
	w := widgetWorld(100, lines...)
	m := Market{
		Bids:          map[string][]Level{"widget": {{StationID: "bidder", Price: 1000, Qty: 10}}},
		StationSystem: map[string]string{"bidder": "sb", "emp_near": "sn", "emp_far": "sf"},
	}
	// home - sn - sb ; home - x - sf
	hp := HaulParams{Home: "home", JumpTicks: 4, Hold: 2000,
		Hops: allHops([2]string{"home", "sn"}, [2]string{"sn", "sb"}, [2]string{"home", "x"}, [2]string{"x", "sf"})}
	ops := Opportunities(Compute(w, Options{}), w, m, hp)
	if len(ops) != 1 {
		t.Fatalf("got %d rows", len(ops))
	}
	o := ops[0]
	if o.RouteLabel != "empire L1 (labor 10 + fee 5)" {
		t.Fatalf("RouteLabel %q, want the empire route: %+v", o.RouteLabel, o)
	}
	// F = emp_near: hops(home,sn)+hops(sn,sb)+hops(sb,home) = 1+1+2.
	if !near(o.HaulHours, 1*4*4*10/3600.0) || !near(o.MakeHours, 10*20*10/3600.0) {
		t.Errorf("HaulHours %v MakeHours %v", o.HaulHours, o.MakeHours)
	}
	if o.CostPerUnit != 15 || o.Total != 9850 {
		t.Errorf("cost %v total %v", o.CostPerUnit, o.Total)
	}
	hand := 10000 / (10*100*10/3600.0 + 2*2*4*10/3600.0)
	if o.ProfitPerHour <= hand {
		t.Errorf("PPH %v not above the hand route's %v", o.ProfitPerHour, hand)
	}

	// Make the hand route fast: it wins on profit/hour.
	w = widgetWorld(0.01, lines...)
	ops = Opportunities(Compute(w, Options{}), w, m, hp)
	if ops[0].RouteLabel != "hand" {
		t.Errorf("fast hand: RouteLabel %q", ops[0].RouteLabel)
	}
}

func TestOpportunitiesUnreachableStation(t *testing.T) {
	w := widgetWorld(0.5)
	m := Market{
		Bids: map[string][]Level{"widget": {
			{StationID: "island", Price: 9000, Qty: 5},
			{StationID: "near", Price: 10, Qty: 1},
			{StationID: "near", Price: 20, Qty: 1},
			{StationID: "nosystem", Price: 50, Qty: 1},
		}},
		StationSystem: map[string]string{"island": "isolated", "near": "s1"},
	}
	hp := HaulParams{Home: "home", JumpTicks: 4, Hold: 2000, Hops: allHops([2]string{"home", "s1"})}
	ops := Opportunities(Compute(w, Options{}), w, m, hp)
	if len(ops) != 4 {
		t.Fatalf("got %d rows", len(ops))
	}
	for i, o := range ops[:2] {
		if o.Unreachable || o.StationID != "near" || o.Hours <= 0 {
			t.Errorf("row %d %+v", i, o)
		}
	}
	if ops[0].Bid != 20 {
		t.Errorf("reachable rows not sorted by profit/hour: %+v", ops[:2])
	}
	for i, o := range ops[2:] {
		if !o.Unreachable || o.Hours != 0 || o.ProfitPerHour != 0 || o.Total != o.Bid*o.Depth {
			t.Errorf("unreachable row %d %+v", i, o)
		}
	}
}

func TestOpportunitiesSkipsUncostedItems(t *testing.T) {
	w := widgetWorld(0.5)
	m := Market{Bids: map[string][]Level{
		"widget": {{StationID: "near", Price: 10, Qty: 1}},
		"ore":    {{StationID: "near", Price: 10, Qty: 1}},
		"relic":  {{StationID: "near", Price: 10, Qty: 1}, {StationID: "near", Price: 9, Qty: 1}},
	}, StationSystem: map[string]string{"near": "home"}}
	costs := Compute(w, Options{})
	hp := HaulParams{Home: "home", JumpTicks: 4, Hold: 2000, Hops: map[string]map[string]int{}}
	ops := Opportunities(costs, w, m, hp)
	if len(ops) != 1 || ops[0].ItemID != "widget" || ops[0].Unreachable {
		t.Errorf("ops %+v", ops)
	}
	if n := UncostedBids(costs, m); n != 3 {
		t.Errorf("UncostedBids = %d, want 3", n)
	}
	// Bidder at home, hand route, 0 make time: Hours 0 never divides by zero.
	w = widgetWorld(0)
	costs = Compute(w, Options{})
	ops = Opportunities(costs, w, m, hp)
	if o := ops[0]; o.Hours != 0 || o.ProfitPerHour != o.Total/zeroHoursFloor {
		t.Errorf("0-hour row %+v", o)
	}
}

func TestCostUnderPolicies(t *testing.T) {
	// beam = 2 steel_plate by hand in 1 tick. steel_plate is cheapest by hand
	// (0 cr, 0.27 t/u) and fastest on the ABC L2 line (12 cr/u, 0.0415 t/u).
	w := steelWorld()
	w.Items["beam"] = catalog.Item{ID: "beam"}
	w.Recipes = append(w.Recipes, catalog.Recipe{ID: "make_beam", CraftingTime: 1, HandCraftable: true,
		Inputs: []catalog.Material{mat("steel_plate", 2)}, Outputs: []catalog.Material{mat("beam", 1)}})
	costs := Compute(w, Options{})
	top := costs["beam"].Routes[0].Route
	c, tk, partial := CostUnder(costs, w, "beam", top, PolicyCheapest)
	if c != 0 || !near(tk, 1.54) || partial {
		t.Errorf("cheapest = (%v, %v, %v)", c, tk, partial)
	}
	c, tk, partial = CostUnder(costs, w, "beam", top, PolicyFastest)
	if !near(c, 24) || !near(tk, 1.083) || partial {
		t.Errorf("fastest = (%v, %v, %v)", c, tk, partial)
	}
}

func TestCostUnderUnpricedIsPartial(t *testing.T) {
	w := widgetWorld(1)
	w.Recipes[0].Inputs = append(w.Recipes[0].Inputs, mat("relic", 1))
	costs := Compute(w, Options{})
	if _, _, partial := CostUnder(costs, w, "widget", costs["widget"].Routes[0].Route, PolicyCheapest); !partial {
		t.Error("an unpriced input must mark the result Partial")
	}
}
