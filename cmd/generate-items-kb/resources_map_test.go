package main

import (
	"reflect"
	"testing"
)

func TestResourceSlugMatchesAnchorFormat(t *testing.T) {
	cases := map[string]string{
		"Iron Ore":        "iron-ore",
		"Water Ice":       "water-ice",
		"Miner's Delight": "miners-delight",
		"Copper":          "copper",
	}
	for in, want := range cases {
		if got := resourceSlug(in); got != want {
			t.Errorf("resourceSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSystemResourceClassesGroupsBySystem(t *testing.T) {
	groups := []ResourceGroup{
		{
			ResourceName: "Iron Ore", ResourceID: "iron_ore",
			Entries: []ResourceEntry{
				{SystemID: "sol"}, {SystemID: "vega"}, {SystemID: "sol"},
			},
		},
		{
			ResourceName: "Water Ice", ResourceID: "water_ice",
			Entries: []ResourceEntry{{SystemID: "sol"}},
		},
	}

	got := systemResourceClasses(groups)

	want := map[string][]string{
		"sol":  {"r-iron-ore", "r-water-ice"},
		"vega": {"r-iron-ore"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSystemResourceClassesDedupesRepeatedSystem(t *testing.T) {
	// Two POIs in one system bearing the same resource must yield one class.
	groups := []ResourceGroup{
		{
			ResourceName: "Iron Ore", ResourceID: "iron_ore",
			Entries: []ResourceEntry{{SystemID: "sol"}, {SystemID: "sol"}},
		},
	}

	got := systemResourceClasses(groups)

	if n := len(got["sol"]); n != 1 {
		t.Errorf("got %d classes for sol, want 1: %v", n, got["sol"])
	}
}

func TestSystemResourceClassesSkipsUndiscovered(t *testing.T) {
	groups := []ResourceGroup{
		{ResourceName: "Unobtainium", ResourceID: "unobtainium", Entries: []ResourceEntry{}},
	}

	got := systemResourceClasses(groups)

	if len(got) != 0 {
		t.Errorf("undiscovered resource produced classes: %v", got)
	}
}
