package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	htmltpl "html/template"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
	"github.com/rsned/spacemolt-kb/pkg/prodcost"
)

//go:embed assets/prodcost.js
var prodcostJS []byte

// privateBase is the KB root as seen from the private output directory: the
// default private/production-costs sits beside kb/ in the KB repo.
const privateBase = "../../kb/"

type oppView struct {
	Name, Station, Bid, Depth, Cost, Margin, Total, Hours, PPH, Route, Policy string
	Partial                                                                   bool
}

type baseRow struct {
	Name                                      string
	Recipe, Out, Credits, Time, Ask, BestBid string
	Route                                     htmltpl.HTML
	BuyCheaper, NoPrice, NoUsable, Partial   bool
}

type treeNode struct {
	Qty, Name string
	ItemID    string
	Default   string // the select's only option until prodcost.js renders the tree
	Above     bool
	Children  []treeNode
}

type detailView struct {
	ID, Name, Total string
	Tree            treeNode
}

type privatePage struct {
	Title, Base string
	Header      htmltpl.HTML
	Ages        DataAges
	Home        string
	JumpTicks   string
	Hold        string
	Uncosted    int
	Opps        []oppView
	Baseline    []baseRow
	Details     []detailView
	Data        htmltpl.JS
}

// renderPrivate writes the private page (index.html with its data inlined,
// data.json and prodcost.js) to outDir. Market prices, opportunities and CRFT
// lines appear only here.
func renderPrivate(outDir string, w prodcost.World, costs map[string]*prodcost.ItemCost, m prodcost.Market,
	hp prodcost.HaulParams, opps []prodcost.Opportunity, ages DataAges) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	page := prodcost.PageData(w, costs, m, hp)
	data, err := json.Marshal(page)
	if err != nil {
		return fmt.Errorf("marshal page data: %w", err)
	}
	pretty, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal page data: %w", err)
	}

	pp := privatePage{Title: "Production Costs (private)", Base: privateBase,
		Header: htmltpl.HTML(kbnav.Header(privateBase)), //nolint:gosec // site header, generated internally
		Ages:   ages, Home: hp.Home, JumpTicks: num(hp.JumpTicks), Hold: num(hp.Hold),
		Uncosted: prodcost.UncostedBids(costs, m),
		Data:     htmltpl.JS(data), //nolint:gosec // json.Marshal escapes <, > and &
	}
	pp.Opps = oppViews(page, opps)
	ids := slices.SortedFunc(maps.Keys(costs), func(a, b string) int {
		return strings.Compare(strings.ToLower(page.Items[a].Name), strings.ToLower(page.Items[b].Name))
	})
	for _, id := range ids {
		ic := costs[id]
		if len(ic.Routes) == 0 {
			continue
		}
		pp.Baseline = append(pp.Baseline, baselineRow(page, ic, m))
		top := ic.Routes[max(ic.Cheapest, 0)]
		total := num(top.CreditsPerUnit) + " cr/u · " + num(top.TicksPerUnit) + " t/u"
		pp.Details = append(pp.Details, detailView{ID: id, Name: page.Items[id].Name, Total: total,
			Tree: tree(page, id, "", map[string]bool{})})
	}

	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, "private.html.tmpl", pp); err != nil {
		return fmt.Errorf("render private: %w", err)
	}
	files := map[string][]byte{"index.html": []byte(buf.String()), "data.json": append(pretty, '\n'), "prodcost.js": prodcostJS}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(outDir, name), b, 0o644); err != nil { //nolint:gosec // local page output
			return err
		}
	}
	return nil
}

// oppViews formats opps in the page's default order (prodcost.js
// compareRows): reachable, then complete before partial, then profit/hr.
func oppViews(page prodcost.Page, opps []prodcost.Opportunity) []oppView {
	opps = slices.Clone(opps)
	slices.SortStableFunc(opps, func(a, b prodcost.Opportunity) int {
		switch {
		case a.Unreachable != b.Unreachable:
			return boolOrder(a.Unreachable)
		case a.Unreachable:
			return cmpDesc(a.Total, b.Total)
		case a.Partial != b.Partial:
			return boolOrder(a.Partial)
		}
		return cmpDesc(a.ProfitPerHour, b.ProfitPerHour)
	})
	out := make([]oppView, 0, len(opps))
	for _, o := range opps {
		it, ok := page.Items[o.ItemID]
		if !ok {
			continue // not costed on the page (a skipped item)
		}
		v := oppView{Name: it.Name, Station: o.StationID, Bid: num(o.Bid), Depth: num(o.Depth),
			Cost: num(o.CostPerUnit), Margin: num(o.MarginPerUnit), Total: num(o.Total),
			Hours: num(o.Hours), PPH: num(o.ProfitPerHour), Route: o.RouteLabel, Policy: string(o.Policy),
			Partial: o.Partial}
		if o.Unreachable {
			v.Hours, v.PPH = "unreachable", "unreachable"
		}
		out = append(out, v)
	}
	return out
}

func boolOrder(first bool) int {
	if first {
		return 1
	}
	return -1
}

func cmpDesc(a, b float64) int {
	switch {
	case a > b:
		return -1
	case a < b:
		return 1
	}
	return 0
}

func baselineRow(page prodcost.Page, ic *prodcost.ItemCost, m prodcost.Market) baseRow {
	top := ic.Routes[max(ic.Cheapest, 0)]
	r := baseRow{Name: page.Items[ic.ItemID].Name, Recipe: top.RecipeID, Route: escLabel(top.Label()),
		Out: num(top.OutPerRun), Credits: num(top.CreditsPerUnit), Time: sig3(top.TicksPerUnit),
		Ask: "—", BestBid: "—", NoUsable: ic.NoUsableRoute, Partial: top.Partial}
	asks, bids := m.Asks[ic.ItemID], m.Bids[ic.ItemID]
	if len(asks) > 0 {
		r.Ask = num(asks[0].Price)
		r.BuyCheaper = !ic.NoUsableRoute && asks[0].Price < top.CreditsPerUnit
	}
	if len(bids) > 0 {
		r.BestBid = num(bids[0].Price)
	}
	r.NoPrice = len(asks) == 0 && len(bids) == 0
	return r
}

// tree mirrors treeHTML in prodcost.js at default choices: each step on its
// cheapest route, each item expanded once. Its selects hold only the default
// option; prodcost.js renders the full tree when a detail is opened, which
// keeps the page a few MB instead of ~15.
func tree(page prodcost.Page, id, qty string, seen map[string]bool) treeNode {
	it := page.Items[id]
	n := treeNode{Qty: qty, Name: it.Name, ItemID: id, Default: "cheapest"}
	switch {
	case it.Raw:
		n.Default = "raw: per settings"
	case len(it.Routes) == 0:
		n.Default = "unpriced (0)"
	}
	if it.Raw || it.Cheapest < 0 || len(it.Routes[it.Cheapest].Inputs) == 0 {
		return n
	}
	if seen[id] {
		n.Above = true
		return n
	}
	seen[id] = true
	for _, in := range it.Routes[it.Cheapest].Inputs {
		n.Children = append(n.Children, tree(page, in.Item, num(in.Qty), seen))
	}
	return n
}
