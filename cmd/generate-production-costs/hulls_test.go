package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/prodcost"
)

func fptr(v float64) *float64 { return &v }
func iptr(v int) *int         { return &v }

func hullFixture() ([]prodcost.HullCost, map[string]catalog.Item) {
	items := map[string]catalog.Item{
		"steel_plate":    {ID: "steel_plate", Name: "Steel Plate"},
		"energy_crystal": {ID: "energy_crystal", Name: "Energy Crystal"},
		"salvage_part":   {ID: "salvage_part", Name: "Salvage Part"},
	}
	hulls := []prodcost.HullCost{
		{
			Ship: catalog.Ship{ID: "congregation", Name: "Congregation", Class: "Bulk Hauler", Tier: 2, Faction: "solarian", CargoCapacity: 120},
			Materials: []prodcost.HullMaterial{
				{ItemID: "steel_plate", Qty: 10, Route: "refine_steel", CreditsPerUnit: 98.4, Credits: 984, AskPrice: 120, AskStation: "near"},
				{ItemID: "energy_crystal", Qty: 2, Route: "raw", Flags: []prodcost.HullFlag{prodcost.FlagCrystal}, AskPrice: 70, AskStation: "far"},
			},
			MaterialCredits: 984, Labor: fptr(1000), LaborNote: "quoted", Royalty: 98.4, QuotedRoyalty: iptr(723),
			MakeTotal: 2082.4, License: prodcost.LicenseOwned, MarketCredits: 1340, Listing: &prodcost.ListingMin{Price: 1000000, Count: 2, StationID: "yard"},
			Flags: []prodcost.HullFlag{prodcost.FlagCrystal}, Best: true,
		},
		{
			Ship: catalog.Ship{ID: "wreck_runner", Name: "Wreck Runner", Class: "Bulk Hauler", Tier: 3},
			Materials: []prodcost.HullMaterial{
				{ItemID: "salvage_part", Qty: 1, Route: "unpriced", Flags: []prodcost.HullFlag{prodcost.FlagSalvage}},
			},
			Partial: true, MaterialCredits: 500, Labor: fptr(100), Royalty: 50, MakeTotal: 650,
			License: prodcost.LicenseNotOwned, Flags: []prodcost.HullFlag{prodcost.FlagSalvage},
		},
		{
			Ship:            catalog.Ship{ID: "mystery", Name: "Mystery", Class: "Bulk Hauler", Tier: 1},
			MaterialCredits: 300, Royalty: 30, MakeTotal: 330, License: prodcost.LicenseNotOwned,
			Flags: []prodcost.HullFlag{prodcost.FlagLaborUnknown},
		},
	}
	return hulls, items
}

func renderHullsString(t *testing.T) string {
	t.Helper()
	hulls, items := hullFixture()
	dir := t.TempDir()
	if err := renderHulls(dir, hulls, items, DataAges{Catalog: "20261101", Facilities: "f", Market: "m", Ships: "s", Listings: "l"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "hulls.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func rowOf(t *testing.T, out, id string) string {
	t.Helper()
	i := strings.Index(out, `data-hull="`+id+`"`)
	if i < 0 {
		t.Fatalf("no row for %s", id)
	}
	rest := out[i:]
	if j := strings.Index(rest[1:], "data-hull="); j >= 0 {
		return rest[:j+1]
	}
	return rest
}

func TestRenderHullsRowsAndFlags(t *testing.T) {
	out := renderHullsString(t)
	for _, want := range []string{"congregation", "984", "2,082", "1,000,000", "owned", "needs salvage", "server charged 723", `data-sort="98.4">98`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !strings.Contains(rowOf(t, out, "wreck_runner"), "≥") {
		t.Error("salvage row lacks ≥")
	}
	if !strings.Contains(rowOf(t, out, "mystery"), "?") {
		t.Error("unknown-labor row lacks ?")
	}
	if strings.Contains(rowOf(t, out, "congregation"), "≥") {
		t.Error("complete row has ≥")
	}
}

func TestRenderHullsBestBox(t *testing.T) {
	out := renderHullsString(t)
	if n := strings.Count(out, `class="n hl-cheap"`); n != 1 {
		t.Fatalf("best boxes = %d, want 1", n)
	}
	if !strings.Contains(rowOf(t, out, "congregation"), "hl-cheap") {
		t.Error("congregation not boxed")
	}
}

func TestRenderHullsMaterialLinksPublic(t *testing.T) {
	out := renderHullsString(t)
	for _, id := range []string{"energy_crystal", "steel_plate"} {
		if !strings.Contains(out, `href="`+privateBase+`production-costs/`+id+`.html"`) {
			t.Errorf("no public link for %s", id)
		}
	}
}

func TestRenderHullsNotPublished(t *testing.T) {
	w := steelWorld()
	out := renderTo(t, w)
	if strings.Contains(out, "hulls.html") || strings.Contains(out, "data-hull") {
		t.Error("public output mentions hulls")
	}
}

func TestMoney(t *testing.T) {
	for in, want := range map[float64]string{2082.4: "2,082", 984: "984", 1000000: "1,000,000", 98.4: "98", 0: "0", -1500.6: "-1,501"} {
		if got := money(in); got != want {
			t.Errorf("money(%v)=%q want %q", in, got, want)
		}
	}
}

func TestRenderHullsSortKeysNumeric(t *testing.T) {
	hulls, items := hullFixture()
	hulls[0].MaterialCredits, hulls[0].Royalty, hulls[0].MakeTotal = 12345, 1234.5, 13579.5
	dir := t.TempDir()
	if err := renderHulls(dir, hulls, items, DataAges{}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "hulls.html"))
	row := rowOf(t, string(b), "congregation")
	if !strings.Contains(row, `data-sort="1234.5"`) {
		t.Error("royalty sort key not plain numeric")
	}
	for _, m := range regexp.MustCompile(`data-sort="(\d[^"]*)"`).FindAllStringSubmatch(row, -1) {
		if strings.Contains(m[1], ",") {
			t.Errorf("comma in sort key %q", m[1])
		}
	}
}
