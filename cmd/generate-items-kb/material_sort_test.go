package main

import (
	"strings"
	"testing"
)

func TestSortMaterialRefsByName(t *testing.T) {
	refs := []MaterialRef{
		{ItemID: "titanium_alloy", Name: "Titanium Alloy"},
		{ItemID: "fuel_tank", Name: "fuel Tank"},
		{ItemID: "b", Name: "Hull Plating"},
		{ItemID: "a", Name: "Hull Plating"},
		{ItemID: "cargo_container", Name: "Cargo Container"},
	}
	sortMaterialRefsByName(refs)
	var got []string
	for _, r := range refs {
		got = append(got, r.ItemID)
	}
	if want := "cargo_container,fuel_tank,a,b,titanium_alloy"; strings.Join(got, ",") != want {
		t.Errorf("order = %s, want %s (case-insensitive name, id breaks ties)", strings.Join(got, ","), want)
	}
}
