package main

import "testing"

func TestSummarize(t *testing.T) {
	cs := []DailyCandle{
		{Day: "2026-06-21", Open: 10, High: 15, Low: 9, Close: 14, Volume: 150},
		{Day: "2026-06-22", Open: 14, High: 16, Low: 13, Close: 15, Volume: 80},
	}
	s := summarize(cs)
	if s.FirstOpen != 10 || s.LastClose != 15 || s.High != 16 || s.Low != 9 {
		t.Errorf("OHLC = %+v", s)
	}
	if s.TotalVolume != 230 || s.Days != 2 {
		t.Errorf("volume/days = %+v", s)
	}
	if s.ChangePct != 50 {
		t.Errorf("changePct = %v, want 50", s.ChangePct)
	}
}

func TestSummarize_SingleDayZeroChange(t *testing.T) {
	cs := []DailyCandle{{Day: "2026-06-21", Open: 10, High: 12, Low: 9, Close: 12, Volume: 40}}
	s := summarize(cs)
	if s.Days != 1 || s.ChangePct != 0 {
		t.Errorf("single-day summary = %+v, want Days=1 ChangePct=0", s)
	}
}

func TestBuildPages_GroupSortAndCards(t *testing.T) {
	candles := map[string][]DailyCandle{
		// ore: two items, different total volume.
		"ore_iron": {{Day: "2026-06-21", Open: 10, High: 12, Low: 9, Close: 11, Volume: 100}},
		"ore_gold": {{Day: "2026-06-21", Open: 50, High: 55, Low: 48, Close: 52, Volume: 300}},
		// weapon: one item.
		"laser": {{Day: "2026-06-21", Open: 5, High: 6, Low: 5, Close: 6, Volume: 20}},
		// no category -> "other".
		"mystery": {{Day: "2026-06-21", Open: 1, High: 2, Low: 1, Close: 2, Volume: 5}},
		// empty candle slice must be skipped.
		"ghost": {},
	}
	names := map[string]string{"ore_iron": "Iron Ore", "ore_gold": "Gold Ore", "laser": "Laser"}
	categories := map[string]string{"ore_iron": "ore", "ore_gold": "ore", "laser": "weapon"}

	pages, cards := buildPages(candles, names, categories)

	// Cards sorted by category name: other, ore, weapon.
	if len(cards) != 3 {
		t.Fatalf("want 3 cards, got %d: %+v", len(cards), cards)
	}
	if cards[0].Category != "other" || cards[1].Category != "ore" || cards[2].Category != "weapon" {
		t.Fatalf("card order = %v/%v/%v", cards[0].Category, cards[1].Category, cards[2].Category)
	}
	if cards[1].Count != 2 || cards[1].VolStr != "400" {
		t.Errorf("ore card = %+v, want Count=2 VolStr=400", cards[1])
	}

	// Find the ore page; items sorted by total volume desc -> gold(300) before iron(100).
	var ore CategoryPage
	for _, p := range pages {
		if p.Category == "ore" {
			ore = p
		}
	}
	if len(ore.Items) != 2 || ore.Items[0].ID != "ore_gold" || ore.Items[1].ID != "ore_iron" {
		t.Fatalf("ore items order = %+v", ore.Items)
	}
	// Name falls back to id when missing; href points at the items page.
	if ore.Items[0].Name != "Gold Ore" || ore.Items[0].ItemHref != "../../items/ore/ore_gold.html" {
		t.Errorf("gold item = %+v", ore.Items[0])
	}
	// Unknown category item lands under "other" with id as name.
	var other CategoryPage
	for _, p := range pages {
		if p.Category == "other" {
			other = p
		}
	}
	if len(other.Items) != 1 || other.Items[0].Name != "mystery" {
		t.Errorf("other page = %+v", other.Items)
	}
}

func TestFmtStat(t *testing.T) {
	s := summarize([]DailyCandle{
		{Day: "2026-06-21", Open: 10, High: 15, Low: 9, Close: 14, Volume: 150},
		{Day: "2026-06-22", Open: 14, High: 16, Low: 13, Close: 15, Volume: 80},
	})
	got := fmtStat(s)
	want := "last 15 · +50.0% · H 16 / L 9 · vol 230 · 2d"
	if got != want {
		t.Errorf("fmtStat = %q, want %q", got, want)
	}
}
