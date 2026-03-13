package gamediff

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDayReport_ContainsDate(t *testing.T) {
	day := DayReport{
		Date:     time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
		PrevDate: "2026-03-11",
		NextDate: "",
		Catalogs: []CatalogDiff{
			{Name: "Recipes", File: "catalog_recipes.json"},
		},
	}
	html, err := RenderDayReport(day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "March 12, 2026") {
		t.Error("report should contain formatted date")
	}
	if !strings.Contains(html, "2026-03-11") {
		t.Error("report should contain prev link")
	}
	if !strings.Contains(html, "No changes") {
		t.Error("empty catalog should show 'No changes'")
	}
}

func TestRenderDayReport_ShowsAdditions(t *testing.T) {
	day := DayReport{
		Date: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
		Catalogs: []CatalogDiff{
			{
				Name:      "Items",
				Additions: []Entry{{Name: "Copper Ore", ID: "copper_ore"}},
			},
		},
	}
	html, err := RenderDayReport(day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Copper Ore") {
		t.Error("report should contain addition name")
	}
	if !strings.Contains(html, "+1") {
		t.Error("report should contain addition count")
	}
}

func TestRenderDayReport_ShowsDeletions(t *testing.T) {
	day := DayReport{
		Date: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
		Catalogs: []CatalogDiff{
			{
				Name:      "Ships",
				Deletions: []Entry{{Name: "Old Frigate", ID: "old_frigate"}},
			},
		},
	}
	html, err := RenderDayReport(day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Old Frigate") {
		t.Error("report should contain deletion name")
	}
}

func TestRenderDayReport_ShowsModifications(t *testing.T) {
	day := DayReport{
		Date: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
		Catalogs: []CatalogDiff{
			{
				Name: "Recipes",
				Changes: []Change{
					{ID: "smelt_copper", Field: "outputs", OldVal: "3", NewVal: "1"},
				},
			},
		},
	}
	html, err := RenderDayReport(day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "smelt_copper") {
		t.Error("report should contain modified item ID")
	}
	if !strings.Contains(html, "outputs") {
		t.Error("report should contain modified field name")
	}
}

func TestRenderIndex_ListsDays(t *testing.T) {
	days := []IndexEntry{
		{Date: "2026-03-12", Summary: "19 additions, 33 deletions"},
		{Date: "2026-03-11", Summary: "No changes"},
	}
	html, err := RenderIndex(days)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "2026-03-12") {
		t.Error("index should contain day link")
	}
	if !strings.Contains(html, "No changes") {
		t.Error("index should contain summary text")
	}
	if !strings.Contains(html, "Game Data Change Log") {
		t.Error("index should contain title")
	}
}

func TestRenderDayReport_SummaryLine(t *testing.T) {
	day := DayReport{
		Date: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
		Catalogs: []CatalogDiff{
			{
				Name:      "Recipes",
				Additions: []Entry{{Name: "A", ID: "a"}, {Name: "B", ID: "b"}},
				Deletions: []Entry{{Name: "C", ID: "c"}},
			},
			{Name: "Items"},
		},
	}
	html, err := RenderDayReport(day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "2 additions") {
		t.Error("summary should show '2 additions'")
	}
	if !strings.Contains(html, "1 deletion") {
		t.Error("summary should show '1 deletion' (singular)")
	}
	if !strings.Contains(html, "across 1 catalog") {
		t.Error("summary should show '1 catalog' (singular)")
	}
}
