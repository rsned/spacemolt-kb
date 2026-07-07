package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderCategoryPage(t *testing.T) {
	dir := t.TempDir()
	page := CategoryPage{
		Category: "ore",
		Items: []ItemVM{{
			ID:       "ore_iron",
			Name:     "Iron Ore",
			Category: "ore",
			ItemHref: "../../items/ore/ore_iron.html",
			Stat:     "last 11 · +0.0% · H 12 / L 9 · vol 100 · 1d",
			Candles:  []DailyCandle{{Day: "2026-06-21", Open: 10, High: 12, Low: 9, Close: 11, Volume: 100}},
		}},
	}
	if err := renderCategoryPage(dir, page); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ore", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	if !strings.Contains(html, `id="ore_iron"`) {
		t.Error("missing item anchor")
	}
	if !strings.Contains(html, `../../items/ore/ore_iron.html`) {
		t.Error("missing item-page link")
	}
	if !strings.Contains(html, "last 11 ·") {
		t.Error("missing stat line")
	}
	if !strings.Contains(html, "<svg") {
		t.Error("missing chart svg")
	}
}

func TestRenderMarketIndex(t *testing.T) {
	dir := t.TempDir()
	cards := []CategoryCard{
		{Category: "ore", Href: "ore/", VolStr: "400", Count: 2},
		{Category: "weapon", Href: "weapon/", VolStr: "20", Count: 1},
	}
	if err := renderMarketIndex(dir, cards); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	if !strings.Contains(html, `href="ore/"`) || !strings.Contains(html, `href="weapon/"`) {
		t.Error("missing category cards")
	}
	if !strings.Contains(html, "sell-side") {
		t.Error("missing legend text")
	}
}
