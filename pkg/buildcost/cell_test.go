package buildcost

import "testing"

func TestBuildCell_Item_BothModes(t *testing.T) {
	b := bookFixture()
	tgt := Target{
		ID: "widget", Kind: "item",
		BoM:     []Requirement{{"iron", 2}, {"copper", 4}},          // 40, feasible
		Recipes: []Recipe{{ID: "r", OutputQty: 1, Inputs: []Requirement{{"copper", 4}}}}, // 20
	}
	m := Margin{FinishedAsk: 100, HasAsk: true}
	c := BuildCell(tgt, "st1", b, m)
	if c.TargetID != "widget" || c.StationID != "st1" {
		t.Fatalf("ids: got %+v", c)
	}
	if !c.BoM.Feasible || !approx(c.BoM.Cost, 40) {
		t.Fatalf("bom: got %+v", c.BoM)
	}
	if c.Recipe.NA || !c.Recipe.Feasible || c.Recipe.RecipeID != "r" || !approx(c.Recipe.Cost, 20) {
		t.Fatalf("recipe: got %+v", c.Recipe)
	}
}

func TestBuildCell_Ship_RecipeNA(t *testing.T) {
	b := bookFixture()
	tgt := Target{
		ID: "frigate", Kind: "ship",
		BoM:      []Requirement{{"iron", 2}},
		RecipeNA: "sub-assemblies not market-traded",
	}
	c := BuildCell(tgt, "st1", b, Margin{})
	if !c.Recipe.NA || c.Recipe.NAReason != "sub-assemblies not market-traded" {
		t.Fatalf("ship recipe should be NA: got %+v", c.Recipe)
	}
	if !c.BoM.Feasible || !approx(c.BoM.Cost, 20) {
		t.Fatalf("ship bom: got %+v", c.BoM)
	}
}
