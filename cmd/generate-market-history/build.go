package main

import (
	"fmt"
	"html/template"
	"sort"
)

// WindowSummary is the whole-window rollup of one item's daily candles.
type WindowSummary struct {
	ItemID      string
	FirstOpen   float64
	LastClose   float64
	High        float64
	Low         float64
	TotalVolume float64
	Days        int
	ChangePct   float64
}

// summarize rolls a day-ordered candle slice into a WindowSummary. cs must be
// non-empty.
func summarize(cs []DailyCandle) WindowSummary {
	s := WindowSummary{
		FirstOpen: cs[0].Open,
		High:      cs[0].High,
		Low:       cs[0].Low,
		Days:      len(cs),
	}
	for _, c := range cs {
		if c.High > s.High {
			s.High = c.High
		}
		if c.Low < s.Low {
			s.Low = c.Low
		}
		s.TotalVolume += c.Volume
	}
	s.LastClose = cs[len(cs)-1].Close
	if s.Days > 1 && s.FirstOpen != 0 {
		s.ChangePct = (s.LastClose - s.FirstOpen) / s.FirstOpen * 100
	}
	return s
}

// fmtStat renders the one-line stat shown under an item heading.
func fmtStat(s WindowSummary) string {
	return fmt.Sprintf("last %s · %s · H %s / L %s · vol %s · %dd",
		fmtCompact(s.LastClose), fmtPct(s.ChangePct),
		fmtCompact(s.High), fmtCompact(s.Low), fmtCompact(s.TotalVolume), s.Days)
}

// ItemVM is one item's view model on a category page.
type ItemVM struct {
	ID       string
	Name     string
	Category string
	ItemHref string
	Stat     string
	Candles  []DailyCandle
	Chart    template.HTML // filled at render time
	Summary  WindowSummary
}

// CategoryPage is one category's page: its items sorted most-traded first.
type CategoryPage struct {
	Category string
	Items    []ItemVM
}

// CategoryCard is one landing-page card.
type CategoryCard struct {
	Category string
	Href     string
	VolStr   string
	Count    int
}

// buildPages groups items by category, sorts items within each category by
// total window volume (descending, ties by id), and builds the landing cards.
// Items with no candles are skipped; items with an empty category bucket under
// "other"; missing names fall back to the item id. Chart HTML is left empty
// (filled during rendering).
func buildPages(candles map[string][]DailyCandle, names, categories map[string]string) ([]CategoryPage, []CategoryCard) {
	byCat := map[string][]ItemVM{}
	for id, cs := range candles {
		if len(cs) == 0 {
			continue
		}
		cat := categories[id]
		if cat == "" {
			cat = "other"
		}
		name := names[id]
		if name == "" {
			name = id
		}
		s := summarize(cs)
		s.ItemID = id
		byCat[cat] = append(byCat[cat], ItemVM{
			ID:       id,
			Name:     name,
			Category: cat,
			ItemHref: fmt.Sprintf("../../items/%s/%s.html", cat, id),
			Stat:     fmtStat(s),
			Candles:  cs,
			Summary:  s,
		})
	}

	cats := make([]string, 0, len(byCat))
	for cat := range byCat {
		cats = append(cats, cat)
	}
	// Sort with "other" first, then alphabetically
	sort.Slice(cats, func(i, j int) bool {
		if cats[i] == "other" {
			return true
		}
		if cats[j] == "other" {
			return false
		}
		return cats[i] < cats[j]
	})

	var pages []CategoryPage
	var cards []CategoryCard
	for _, cat := range cats {
		items := byCat[cat]
		sort.Slice(items, func(i, j int) bool {
			if items[i].Summary.TotalVolume != items[j].Summary.TotalVolume {
				return items[i].Summary.TotalVolume > items[j].Summary.TotalVolume
			}
			return items[i].ID < items[j].ID
		})
		var tot float64
		for _, it := range items {
			tot += it.Summary.TotalVolume
		}
		pages = append(pages, CategoryPage{Category: cat, Items: items})
		cards = append(cards, CategoryCard{
			Category: cat,
			Href:     cat + "/",
			VolStr:   fmtCompact(tot),
			Count:    len(items),
		})
	}
	return pages, cards
}
