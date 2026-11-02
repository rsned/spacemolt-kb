package prodcost

import (
	"math"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

func steel() (catalog.Recipe, map[string]catalog.Facility) {
	rec := catalog.Recipe{ID: "refine_steel", CraftingTime: 0.54,
		Outputs:    []catalog.Material{{ItemID: "steel_plate", Quantity: 2}},
		ProducedBy: []string{"iron_refinery", "industrial_forge"}}
	facs := map[string]catalog.Facility{
		"iron_refinery":    {ID: "iron_refinery", Level: 1, LaborCost: 10},
		"industrial_forge": {ID: "industrial_forge", Level: 2, LaborCost: 20},
	}
	return rec, facs
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func find(rs []Route, k RouteKind, lvl int) (Route, bool) {
	for _, r := range rs {
		if r.Kind == k && r.Level == lvl {
			return r, true
		}
	}
	return Route{}, false
}

func TestRoutesHandAndEmpire(t *testing.T) {
	rec := catalog.Recipe{ID: "basic_iron_smelting", CraftingTime: 0.27, HandCraftable: true,
		Outputs: []catalog.Material{{ItemID: "iron_plate", Quantity: 1}}}
	rs := Routes(rec, nil, nil)
	if len(rs) != 1 || rs[0].Kind != KindHand || rs[0].Labor != 0 || rs[0].TicksPerRun != 0.27 || rs[0].OutPerRun != 1 || !rs[0].Usable {
		t.Fatalf("got %+v", rs)
	}
	if rs[0].Label() != "hand" {
		t.Errorf("label %q", rs[0].Label())
	}
}

func TestRoutesEmpireLabel(t *testing.T) {
	rec, facs := steel()
	rs := Routes(rec, facs, []Line{{RecipeID: "refine_steel", Level: 1, Fee: 9, Empire: true}, {RecipeID: "other", Level: 3, Fee: 1, Empire: true}})
	r, ok := find(rs, KindEmpire, 1)
	if !ok || len(rs) != 3 {
		t.Fatalf("got %+v", rs)
	}
	if got := r.Label(); got != "empire L1 (labor 10 + fee 9)" {
		t.Errorf("label %q", got)
	}
	if !near(r.TicksPerRun, 0.108) || !r.Usable {
		t.Errorf("ticks %v usable %v", r.TicksPerRun, r.Usable)
	}
}

func TestRoutesOwnLevelSpeed(t *testing.T) {
	rec, facs := steel()
	r, ok := find(Routes(rec, facs, nil), KindOwn, 2)
	if !ok || !near(r.TicksPerRun, 0.54/15) || r.Usable || r.Label() != "own L2 (labor 20)" {
		t.Fatalf("got %+v label %q", r, r.Label())
	}
}

func TestRoutesEmpireFacilityHasNoOwn(t *testing.T) {
	rec, facs := steel()
	f := facs["iron_refinery"]
	f.Empire = "haven"
	facs["iron_refinery"] = f
	if _, ok := find(Routes(rec, facs, nil), KindOwn, 1); ok {
		t.Error("empire facility produced own route")
	}
}

func TestRoutesPirateTag(t *testing.T) {
	rec, facs := steel()
	f := facs["industrial_forge"]
	f.PirateBaseOnly = true
	facs["industrial_forge"] = f
	r, _ := find(Routes(rec, facs, nil), KindOwn, 2)
	if !strings.HasSuffix(r.Label(), " · pirate base only") {
		t.Errorf("label %q", r.Label())
	}
}

func TestRoutesFactionUsesCapturedTicks(t *testing.T) {
	rec, facs := steel()
	rs := Routes(rec, facs, []Line{{RecipeID: "refine_steel", Level: 2, Fee: 4, TicksPerRun: 0.083, FactionTag: "ABC", StationName: "Dock", Kind: KindFaction}})
	r, ok := find(rs, KindFaction, 2)
	if !ok || r.TicksPerRun != 0.083 || r.Fee != 4 || r.Labor != 20 {
		t.Fatalf("got %+v", r)
	}
	if got := r.Label(); got != "ABC L2 @ Dock (labor 20 + fee 4)" {
		t.Errorf("label %q", got)
	}
}

func TestRoutesCRFTAndFallbacks(t *testing.T) {
	rec, facs := steel()
	rs := Routes(rec, facs, []Line{
		{RecipeID: "refine_steel", Level: 1, FactionTag: "CRFT", StationName: "CJ", Kind: KindCRFT},
		{RecipeID: "refine_steel", Level: 3, Fee: 2.5, FactionTag: "XY", StationName: "S", Kind: KindFaction},
		{RecipeID: "refine_steel", Level: 1, Fee: 9, Empire: true},
		{RecipeID: "refine_steel", Level: 1, Fee: 7.25, Empire: true},
	})
	c, _ := find(rs, KindCRFT, 1)
	if c.Fee != 0 || c.Label() != "CRFT L1 @ CJ (labor 10)" || !near(c.TicksPerRun, 0.108) {
		t.Errorf("crft %+v %q", c, c.Label())
	}
	f, _ := find(rs, KindFaction, 3) // no L3 facility: nearest lower is L2, labor 20
	if f.Labor != 20 || f.Label() != "XY L3 @ S (labor 20 + fee 2.5)" {
		t.Errorf("faction %+v %q", f, f.Label())
	}
	e, _ := find(rs, KindEmpire, 1)
	if e.Fee != 7.25 {
		t.Errorf("empire fee %v", e.Fee)
	}
}
