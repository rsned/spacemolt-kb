package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderFacilitiesIndex(t *testing.T) {
	dir := t.TempDir()
	summaries := []FacilityGroupSummary{
		{Group: "weapon", Href: "weapon/", Count: 2},
		{Group: "service", Href: "service/", Count: 1},
	}
	stats := []CategoryStat{
		{Group: "weapon", Count: 2, Levels: []LevelStat{
			{Level: 1, Count: 1, BoM: "4.1M ± 2.0M", Recipe: "900K", Buildable: 1},
			{Level: 2, Count: 1, BoM: "—", Recipe: "1.2M", Buildable: 0},
		}},
	}
	if err := renderFacilitiesIndex(dir, summaries, stats); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"Facility Build Costs", `href="weapon/"`, "weapon", "service", ">2<",
		"How to read the two prices", "MKT-AVG", "Galaxy", "N/M covered",
		"Category stats", "Buildable now",
		"4.1M ± 2.0M", "900K", // stat cells rendered
		`class="dash"`,        // the em-dash BoM cell gets the muted class
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("index missing %q", want)
		}
	}
}

func TestRenderFacilityGroup(t *testing.T) {
	dir := t.TempDir()
	page := FacilityGroupPage{
		Group: "weapon", Heading: "weapon",
		TOC: []FacilityTOCEntry{
			{Group: "service", Href: "../service/", Count: 1},
			{Group: "weapon", Href: "../weapon/", Count: 1, Active: true},
		},
		Facilities: []FacilityEntryVM{{
			ID: "a_forge", Name: "A Forge", Href: "../../../facilities/production/a_forge.html", Level: 1, Produces: "Railgun",
			BoM: ViewVM{Title: "BoM (ore)", MktBuildCost: "40.00", GalBuildCost: "36.00", Components: []ComponentVM{
				{Name: "Iron Ore", Href: "../../../items/ore/iron_ore.html", Qty: "2", MktUnit: "5.00", MktTotal: "10.00", GalUnit: "4.00", GalTotal: "8.00"},
			}},
			Recipe: ViewVM{Title: "Recipe (components)", MktBuildCost: "10.00", MktNote: "(1/2 priced)", GalBuildCost: "1/2 covered", GalInfeasible: true, Components: []ComponentVM{
				{Name: "Iron", Qty: "1", MktUnit: "—", MktTotal: "—", GalUnit: "—", GalTotal: "—", GalInfeasible: true},
			}},
		}},
	}
	if err := renderFacilityGroup(dir, page); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "weapon", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`id="a_forge"`,
		`href="../../../facilities/production/a_forge.html">A Forge`,
		"produces Railgun",
		"BoM (ore)", "Recipe (components)",
		`href="../../../items/ore/iron_ore.html">Iron Ore`,
		"(1/2 priced)", "1/2 covered",
		`href="../service/"`, // TOC sibling link
		"class=\"active\"",   // active TOC entry
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("group page missing %q", want)
		}
	}
}
