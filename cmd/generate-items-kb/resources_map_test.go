package main

import (
	"reflect"
	"strings"
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

// TestSystemResourceClassesReturnsSortedOrder guards the slices.Sort call in
// systemResourceClasses. It uses eight resources in one system, inserted in
// reverse-alphabetical order by slug, so that:
//   - random Go map iteration order coincides with sorted order only by
//     astronomically unlikely chance (1 in 8! if it were uniform), unlike a
//     2-element case where "sorted by luck" happens ~75% of the time; and
//   - a regression that merely preserves insertion order (rather than
//     sorting) also fails, since insertion order here is the exact reverse
//     of the expected sorted output.
func TestSystemResourceClassesReturnsSortedOrder(t *testing.T) {
	groups := []ResourceGroup{
		{ResourceName: "Zirconium Ore", ResourceID: "zirconium_ore", Entries: []ResourceEntry{{SystemID: "nova"}}},
		{ResourceName: "Yttrium Ore", ResourceID: "yttrium_ore", Entries: []ResourceEntry{{SystemID: "nova"}}},
		{ResourceName: "Xenon Gas", ResourceID: "xenon_gas", Entries: []ResourceEntry{{SystemID: "nova"}}},
		{ResourceName: "Water Ice", ResourceID: "water_ice", Entries: []ResourceEntry{{SystemID: "nova"}}},
		{ResourceName: "Vanadium Ore", ResourceID: "vanadium_ore", Entries: []ResourceEntry{{SystemID: "nova"}}},
		{ResourceName: "Uranium Ore", ResourceID: "uranium_ore", Entries: []ResourceEntry{{SystemID: "nova"}}},
		{ResourceName: "Tungsten Ore", ResourceID: "tungsten_ore", Entries: []ResourceEntry{{SystemID: "nova"}}},
		{ResourceName: "Silver Ore", ResourceID: "silver_ore", Entries: []ResourceEntry{{SystemID: "nova"}}},
	}

	got := systemResourceClasses(groups)

	want := []string{
		"r-silver-ore",
		"r-tungsten-ore",
		"r-uranium-ore",
		"r-vanadium-ore",
		"r-water-ice",
		"r-xenon-gas",
		"r-yttrium-ore",
		"r-zirconium-ore",
	}
	if !reflect.DeepEqual(got["nova"], want) {
		t.Errorf("got %v, want %v", got["nova"], want)
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

func TestResourceHighlightCSSOneRulePerDiscoveredResource(t *testing.T) {
	groups := []ResourceGroup{
		{ResourceName: "Iron Ore", Entries: []ResourceEntry{{SystemID: "sol"}}},
		{ResourceName: "Water Ice", Entries: []ResourceEntry{{SystemID: "vega"}}},
		{ResourceName: "Unobtainium", Entries: []ResourceEntry{}},
	}

	css := resourceHighlightCSS(groups)

	if !strings.Contains(css, `#res-map[data-active="iron-ore"] .r-iron-ore`) {
		t.Errorf("missing iron-ore rule:\n%s", css)
	}
	if !strings.Contains(css, `#res-map[data-active="water-ice"] .r-water-ice`) {
		t.Errorf("missing water-ice rule:\n%s", css)
	}
	if strings.Contains(css, "unobtainium") {
		t.Errorf("undiscovered resource must not get a rule:\n%s", css)
	}
	if n := strings.Count(css, "data-active="); n != 2 {
		t.Errorf("got %d rules, want 2", n)
	}
}
