package main

import (
	"fmt"
	htmltpl "html/template"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/prodcost"
)

// money formats credits as a rounded integer with thousands separators.
func money(v float64) string {
	n := int64(math.Round(v))
	s := strconv.FormatInt(max(n, -n), 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if n < 0 {
		return "-" + b.String()
	}
	return b.String()
}

type hullMatView struct {
	Name, Href, Qty, Route, Unit, Total, Flags, Ask string
}

type hullView struct {
	ID, Class, Name, Empire         string
	Tier, Cargo                     int
	Materials, MaterialsSort        string
	Labor, LaborSort, LaborNote     string
	Royalty, RoyaltyNote            string
	Total, TotalSort                string
	License, Market, MarketSort     string
	Listing, ListingSort, FlagsText string
	Best, Partial, LaborUnknown     bool
	Mats                            []hullMatView
}

type hullsPage struct {
	Title, Base string
	Header      htmltpl.HTML
	Ages        DataAges
	Hulls       []hullView
}

func flagsText(fs []prodcost.HullFlag) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = string(f)
	}
	return strings.Join(parts, ", ")
}

func hullViews(hulls []prodcost.HullCost, items map[string]catalog.Item) []hullView {
	out := make([]hullView, 0, len(hulls))
	for _, h := range hulls {
		s := h.Ship
		v := hullView{ID: s.ID, Class: s.Class, Name: s.Name, Empire: s.Faction, Tier: s.Tier, Cargo: s.CargoCapacity,
			Materials: money(h.MaterialCredits), MaterialsSort: num(h.MaterialCredits),
			Royalty: money(h.Royalty), Total: money(h.MakeTotal), TotalSort: num(h.MakeTotal),
			License: string(h.License), Market: "—", MarketSort: "-1", Listing: "—", ListingSort: "-1",
			Best: h.Best, Partial: h.Partial, LaborSort: "-1", FlagsText: flagsText(h.Flags)}
		if h.Partial {
			v.Materials = "≥" + v.Materials
		}
		if h.Labor != nil {
			v.Labor, v.LaborSort, v.LaborNote = money(*h.Labor), num(*h.Labor), h.LaborNote
		} else {
			v.Labor, v.LaborUnknown = "?", true
		}
		if h.QuotedRoyalty != nil && math.Round(float64(*h.QuotedRoyalty)) != math.Round(h.Royalty) {
			v.RoyaltyNote = fmt.Sprintf("server charged %d", *h.QuotedRoyalty)
		}
		if h.MarketCredits > 0 {
			v.Market, v.MarketSort = money(h.MarketCredits), num(h.MarketCredits)
			if h.MarketShort {
				v.Market += " (short)"
			}
		}
		if h.Listing != nil {
			v.Listing, v.ListingSort = money(float64(h.Listing.Price)), strconv.Itoa(h.Listing.Price)
			if h.Listing.StationID != "" {
				v.Listing += " @ " + h.Listing.StationID
			}
		}
		for _, m := range h.Materials {
			name := m.ItemID
			if it, ok := items[m.ItemID]; ok && it.Name != "" {
				name = it.Name
			}
			mv := hullMatView{Name: name, Href: privateBase + "production-costs/" + m.ItemID + ".html",
				Qty: num(m.Qty), Route: m.Route, Unit: num(m.CreditsPerUnit), Total: money(m.Credits),
				Flags: flagsText(m.Flags), Ask: "—"}
			if m.AskPrice > 0 {
				mv.Ask = num(m.AskPrice)
				if m.AskStation != "" {
					mv.Ask += " @ " + m.AskStation
				}
			}
			v.Mats = append(v.Mats, mv)
		}
		out = append(out, v)
	}
	return out
}

// renderHulls writes outDir/hulls.html, the private make-versus-market page
// for ship hulls.
func renderHulls(outDir string, hulls []prodcost.HullCost, items map[string]catalog.Item, ages DataAges) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	hp := hullsPage{Title: "Hull Costs (private)", Base: privateBase,
		Header: htmltpl.HTML(kbnav.Header(privateBase)), //nolint:gosec // site header, generated internally
		Ages:   ages, Hulls: hullViews(hulls, items)}
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "hulls.html.tmpl", hp); err != nil {
		return fmt.Errorf("render hulls: %w", err)
	}
	return os.WriteFile(filepath.Join(outDir, "hulls.html"), []byte(buf.String()), 0o644) //nolint:gosec // local page output
}
