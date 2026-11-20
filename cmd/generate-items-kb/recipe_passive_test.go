package main

import "testing"

// v0.729.0: a recipe is hand-craftable at the Station Workshop only when
// facility_only is false AND its category is not "Ship Passive".
func TestRecipeHandCraftable(t *testing.T) {
	cases := []struct {
		name string
		r    Recipe
		want bool
	}{
		{"plain", Recipe{Category: "Smelting"}, true},
		{"facility only", Recipe{Category: "Smelting", FacilityOnly: true}, false},
		{"ship passive", Recipe{Category: "Ship Passive"}, false},
	}
	for _, c := range cases {
		if got := c.r.HandCraftable(); got != c.want {
			t.Errorf("%s: HandCraftable() = %v, want %v", c.name, got, c.want)
		}
	}
}

// Ship Passive recipes can't be crafted anywhere, so the where.html
// "no facility needed" table must not list them as craftable at any station.
func TestSplitNoFacilityRecipesSkipsShipPassive(t *testing.T) {
	recipes := map[string]*Recipe{
		"onboard_x": {ID: "onboard_x", Name: "Onboard X", Category: "Ship Passive"},
		"hand_y":    {ID: "hand_y", Name: "Hand Y", Category: "Smelting"},
	}
	fo, nf := splitNoFacilityRecipes(recipes, map[string]bool{})
	if len(fo) != 0 || len(nf) != 1 || nf[0].ID != "hand_y" {
		t.Fatalf("got facilityOnly=%v noFacilityNeeded=%v, want only hand_y in noFacilityNeeded", fo, nf)
	}
}
