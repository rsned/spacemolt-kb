package resourcediff

import (
	"strings"
	"testing"
)

func baseSnapshot() *Snapshot {
	return &Snapshot{
		Date:          "2026-08-29",
		ServerVersion: "0.566.2",
		Summary:       Summary{Types: 3, Deposits: 2, Systems: 505, Explored: 505},
		Types: []ResourceType{
			{ID: "iron_ore", Name: "Iron Ore", Category: "ore"},
			{ID: "old_ore", Name: "Old Ore", Category: "ore"},
			{ID: "zinc_ore", Name: "Zinc Ore", Category: "ore"},
		},
		Deposits: []Deposit{
			{SystemID: "sol", SystemName: "Sol", POIID: "sol_belt", POIName: "Belt", Station: true, ResourceID: "iron_ore", Richness: 10, Remaining: 500, LastTick: 100},
			{SystemID: "sol", SystemName: "Sol", POIID: "sol_belt", POIName: "Belt", Station: true, ResourceID: "old_ore", Richness: 1, Remaining: 5, LastTick: 100},
		},
	}
}

func TestDiff_NoChanges(t *testing.T) {
	c := Diff(baseSnapshot(), baseSnapshot())
	if c.HasChanges() {
		t.Errorf("identical snapshots should not report changes: %+v", c)
	}
}

func TestDiff_Scenarios(t *testing.T) {
	old := baseSnapshot()
	cur := baseSnapshot()
	cur.Date = "2026-09-01"
	// A new catalog type, and old_ore withdrawn.
	cur.Types = []ResourceType{
		{ID: "iron_ore", Name: "Iron Ore", Category: "ore"},
		{ID: "nebulium", Name: "Nebulium", Category: "ore"},
		{ID: "zinc_ore", Name: "Zinc Ore", Category: "ore"},
	}
	cur.Deposits = []Deposit{
		// Re-surveyed: remaining dropped, tick moved (tick alone is not a change).
		{SystemID: "sol", SystemName: "Sol", POIID: "sol_belt", POIName: "Belt", Station: true, ResourceID: "iron_ore", Richness: 10, Remaining: 420, LastTick: 200},
		// Zinc discovered (0 -> 1) at a brand-new hidden POI in a brand-new system.
		{SystemID: "haven", SystemName: "Haven", POIID: "haven_zinc", POIName: "Zinc Field", Hidden: true, ResourceID: "zinc_ore", Richness: 3, Remaining: 90, LastTick: 200},
		// Another iron deposit at a new POI in an already-known system.
		{SystemID: "sol", SystemName: "Sol", POIID: "sol_rock", POIName: "Rock", Station: true, ResourceID: "iron_ore", Richness: 2, Remaining: 30, LastTick: 200},
	}
	cur.Summary = Summary{Types: 3, Deposits: 3, Systems: 505, Explored: 505}

	c := Diff(old, cur)
	if !c.HasChanges() {
		t.Fatal("want changes")
	}
	if len(c.NewTypes) != 1 || c.NewTypes[0].ID != "nebulium" {
		t.Errorf("NewTypes = %+v", c.NewTypes)
	}
	if len(c.RemovedTypes) != 1 || c.RemovedTypes[0].ID != "old_ore" {
		t.Errorf("RemovedTypes = %+v", c.RemovedTypes)
	}
	if len(c.Discovered) != 1 || c.Discovered[0].ID != "zinc_ore" || c.Discovered[0].Deposits != 1 {
		t.Errorf("Discovered = %+v", c.Discovered)
	}
	if len(c.NewDeposits) != 2 {
		t.Fatalf("NewDeposits groups = %+v", c.NewDeposits)
	}
	// Groups sorted by resource name: Iron Ore, Zinc Ore.
	if c.NewDeposits[0].Name != "Iron Ore" || len(c.NewDeposits[0].Rows) != 1 || c.NewDeposits[0].Rows[0].POIID != "sol_rock" {
		t.Errorf("NewDeposits[0] = %+v", c.NewDeposits[0])
	}
	if c.NewDeposits[1].Name != "Zinc Ore" || c.NewDeposits[1].Rows[0].POIID != "haven_zinc" {
		t.Errorf("NewDeposits[1] = %+v", c.NewDeposits[1])
	}
	if n := c.NewDepositCount(); n != 2 {
		t.Errorf("NewDepositCount = %d", n)
	}
	if len(c.RemovedDeposits) != 1 || c.RemovedDeposits[0].ResourceID != "old_ore" {
		t.Errorf("RemovedDeposits = %+v", c.RemovedDeposits)
	}
	if len(c.Changed) != 1 || c.Changed[0].Old.Remaining != 500 || c.Changed[0].New.Remaining != 420 || c.Changed[0].ResourceName != "Iron Ore" {
		t.Errorf("Changed = %+v", c.Changed)
	}
	if len(c.NewSystems) != 1 || c.NewSystems[0].ID != "haven" || c.NewSystems[0].Deposits != 1 {
		t.Errorf("NewSystems = %+v", c.NewSystems)
	}
	if len(c.NewPOIs) != 2 {
		t.Fatalf("NewPOIs = %+v", c.NewPOIs)
	}
	// Sorted by system name then POI name: Haven/Zinc Field, Sol/Rock.
	if c.NewPOIs[0].ID != "haven_zinc" || !c.NewPOIs[0].Hidden || c.NewPOIs[1].ID != "sol_rock" || c.NewPOIs[1].Hidden {
		t.Errorf("NewPOIs = %+v", c.NewPOIs)
	}
	if c.NewHiddenPOIs() != 1 {
		t.Errorf("NewHiddenPOIs = %d", c.NewHiddenPOIs())
	}
	if c.Old.Deposits != 2 || c.New.Deposits != 3 {
		t.Errorf("summaries not carried: %+v %+v", c.Old, c.New)
	}
}

func TestDiff_HiddenFlagChangeIsAChange(t *testing.T) {
	old := baseSnapshot()
	cur := baseSnapshot()
	cur.Deposits[0].Hidden = true
	c := Diff(old, cur)
	if len(c.Changed) != 1 || !c.Changed[0].New.Hidden {
		t.Errorf("Changed = %+v", c.Changed)
	}
}

func TestRenderReport(t *testing.T) {
	old := baseSnapshot()
	cur := baseSnapshot()
	cur.Date = "2026-09-01"
	cur.Deposits = append(cur.Deposits, Deposit{SystemID: "haven", SystemName: "Haven", POIID: "haven_zinc", POIName: "Zinc Field", Hidden: true, ResourceID: "zinc_ore", Richness: 3, Remaining: 90})
	vsPrev := Diff(old, cur)
	rep := DayReport{
		Snapshot:           cur,
		PrevDate:           "2026-08-29",
		VsPrevious:         vsPrev,
		VsBaseline:         Diff(old, cur),
		BaselineDate:       "2026-08-29",
		ContentVersion:     "0.566.0",
		ContentReleaseDate: "2026-08-27",
		ContentNotes:       []string{"11 newly available mineable resources"},
		CatalogDiffURL:     "../../diffs/2026-08-27.html",
	}
	html, err := RenderDayReport(rep)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<title>Resource Survey Changes`,
		`href="../../resources/index.html"`, // kbnav header at depth 2
		"Since the last regen",
		"Since the last server content update",
		"Zinc Ore",
		`../../systems/haven/index.html`,
		`../../items/ore/zinc_ore.html`,
		"Newly discovered",
		">Power</th>",
		`<td class="num">4</td>`, // zinc: floor(90/20)
		"patch v0.566.0",
		"11 newly available mineable resources",
		`href="../../diffs/2026-08-27.html"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}

	idx, err := RenderIndex([]IndexEntry{{Date: "2026-09-01", ServerVersion: "0.566.2", VsPrevious: "1 new deposit", VsBaseline: "1 new deposit"}, {Date: "2026-08-29", ServerVersion: "0.566.2", IsBaseline: true, ContentVersion: "0.566.0", VsPrevious: "First snapshot"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="2026-09-01.html"`, "baseline for v0.566.0", "v0.566.2"} {
		if !strings.Contains(idx, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestSummaryLine(t *testing.T) {
	old := baseSnapshot()
	cur := baseSnapshot()
	if got := Diff(old, cur).SummaryLine(); got != "No changes" {
		t.Errorf("got %q", got)
	}
	cur.Types = append(cur.Types, ResourceType{ID: "nebulium", Name: "Nebulium", Category: "ore"})
	cur.Deposits = append(cur.Deposits, Deposit{SystemID: "haven", SystemName: "Haven", POIID: "h1", POIName: "P", Hidden: true, ResourceID: "zinc_ore", Richness: 1, Remaining: 1})
	got := Diff(old, cur).SummaryLine()
	for _, want := range []string{"1 new type", "1 discovered", "1 new deposit", "1 new POI", "1 hidden"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
}

func TestPerResource(t *testing.T) {
	old := baseSnapshot()
	cur := baseSnapshot()
	cur.Types = append(cur.Types, ResourceType{ID: "nebulium", Name: "Nebulium", Category: "ore"})
	cur.Deposits = []Deposit{
		cur.Deposits[0], // iron_ore at sol_belt kept
		// old_ore at sol_belt removed
		{SystemID: "sol", SystemName: "Sol", POIID: "sol_rock", POIName: "Rock", ResourceID: "iron_ore", Richness: 2, Remaining: 30},
		{SystemID: "sol", SystemName: "Sol", POIID: "sol_far", POIName: "Far", ResourceID: "iron_ore", Richness: 2, Remaining: 30},
		{SystemID: "haven", SystemName: "Haven", POIID: "h1", POIName: "P", ResourceID: "zinc_ore", Richness: 1, Remaining: 1},
	}
	got := Diff(old, cur).PerResource()
	if d := got["iron_ore"]; d.Added != 2 || d.Removed != 0 || d.Net() != 2 || d.Discovered {
		t.Errorf("iron_ore = %+v", d)
	}
	if d := got["old_ore"]; d.Removed != 1 || d.Net() != -1 {
		t.Errorf("old_ore = %+v", d)
	}
	if d := got["zinc_ore"]; d.Added != 1 || !d.Discovered {
		t.Errorf("zinc_ore = %+v", d)
	}
	if d := got["nebulium"]; !d.NewType || d.Added != 0 {
		t.Errorf("nebulium = %+v", d)
	}
}
