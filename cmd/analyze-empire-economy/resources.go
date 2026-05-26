package main

import (
	"cmp"
	"fmt"
	"strings"
	"slices"
)

// easeByEmpireProduct is empire -> product ID -> ease score (0..1).
// Only contains entries for (empire, product) pairs the empire can solo-build.
type easeByEmpireProductMap map[string]map[string]float64

// writeResourceSheets emits Section 3 and returns the per-empire/per-product
// ease scores so the SSI section can reuse them.
func writeResourceSheets(w *strings.Builder, items map[string]*Item, products []*Product, empMap map[string]*EmpireResources, galaxy map[string]float64) easeByEmpireProductMap {
	fmt.Fprintln(w, "## 3. Per-Empire Resource Sheets")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "For each empire, a resource sheet shows galaxy market share per base material (sum of POI richness in that empire ÷ sum across all named-empire POIs), and the top-10 products with the *worst* ease score (the empire's hardest end-products to source comfortably).")
	fmt.Fprintln(w)

	// Gather the union of all base material IDs that any analyzed product
	// actually needs. Resources never referenced by a recipe are uninteresting.
	usedBases := make(map[string]struct{})
	for _, p := range products {
		for _, m := range p.BaseMaterials {
			usedBases[m.ItemID] = struct{}{}
		}
	}

	ease := make(easeByEmpireProductMap, len(empires))
	for _, e := range empires {
		ease[e] = make(map[string]float64, len(products))
	}

	for _, e := range empires {
		er := empMap[e]
		writeOneEmpireSheet(w, e, er, galaxy, usedBases, items, products, ease[e])
	}

	writeGalaxyHeadlines(w, empMap, galaxy, usedBases, items)

	return ease
}

// resourceRow is one line of the per-empire resource table.
type resourceRow struct {
	ItemID   string
	Name     string
	Richness float64
	Share    float64
	POIs     int
	Verdict  string
}

func writeOneEmpireSheet(
	w *strings.Builder,
	empire string,
	er *EmpireResources,
	galaxy map[string]float64,
	usedBases map[string]struct{},
	items map[string]*Item,
	products []*Product,
	easeOut map[string]float64,
) {
	rows := make([]resourceRow, 0, len(usedBases))
	for id := range usedBases {
		gal := galaxy[id]
		emp := er.Richness[id]
		var share float64
		if gal > 0 {
			share = emp / gal
		}
		name := id
		if it, ok := items[id]; ok {
			name = it.Name
		}
		rows = append(rows, resourceRow{
			ItemID:   id,
			Name:     name,
			Richness: emp,
			Share:    share,
			POIs:     er.POICount[id],
			Verdict:  verdictFor(share),
		})
	}
	slices.SortFunc(rows, func(a, b resourceRow) int {
		if c := cmp.Compare(b.Share, a.Share); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	fmt.Fprintf(w, "### %s Empire\n\n", titleEmpire(empire))
	fmt.Fprintln(w, "**Resource Position**")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Resource | Richness | Galaxy Share | POIs | Verdict |")
	fmt.Fprintln(w, "|----------|---------:|-------------:|-----:|---------|")
	for _, r := range rows {
		fmt.Fprintf(w, "| %s | %s | %s | %d | %s |\n",
			r.Name, formatRichness(r.Richness), formatShare(r.Share), r.POIs, r.Verdict)
	}
	fmt.Fprintln(w)

	// Compute and stash ease scores per product for downstream SSI use; render
	// the worst-10 here.
	type easeRow struct {
		ProductName string
		Kind        string
		Ease        float64
		Bottleneck  string
	}
	var easeRows []easeRow
	shareLookup := make(map[string]float64, len(rows))
	for _, r := range rows {
		shareLookup[r.ItemID] = r.Share
	}
	for _, p := range products {
		if !canSoloBuild(p, er) {
			continue
		}
		minShare := 1.0
		bottleneck := ""
		for _, m := range p.BaseMaterials {
			s := shareLookup[m.ItemID]
			if s < minShare {
				minShare = s
				bottleneck = m.ItemID
				if it, ok := items[m.ItemID]; ok {
					bottleneck = it.Name
				}
			}
		}
		easeOut[p.ID] = minShare
		easeRows = append(easeRows, easeRow{
			ProductName: p.Name,
			Kind:        p.Kind,
			Ease:        minShare,
			Bottleneck:  bottleneck,
		})
	}
	if len(easeRows) > 0 {
		slices.SortFunc(easeRows, func(a, b easeRow) int {
			if c := cmp.Compare(a.Ease, b.Ease); c != 0 {
				return c
			}
			return cmp.Compare(a.ProductName, b.ProductName)
		})
		fmt.Fprintln(w, "**Top 10 Hardest Solo-Buildable Products** (lowest ease — empire can build but is thin on a key input):")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| Product | Kind | Ease | Bottleneck Resource |")
		fmt.Fprintln(w, "|---------|------|-----:|---------------------|")
		limit := min(10, len(easeRows))
		for i := range limit {
			r := easeRows[i]
			fmt.Fprintf(w, "| %s | %s | %s | %s |\n", r.ProductName, r.Kind, formatShare(r.Ease), r.Bottleneck)
		}
		fmt.Fprintln(w)
	}
}

// writeGalaxyHeadlines covers galaxy-level findings: monopolies and contested
// resources.
func writeGalaxyHeadlines(w *strings.Builder, empMap map[string]*EmpireResources, galaxy map[string]float64, usedBases map[string]struct{}, items map[string]*Item) {
	type galaxyRow struct {
		ItemID    string
		Name      string
		Total     float64
		TopEmp    string
		TopShare  float64
		Empires   int
		Contested bool
	}
	rows := make([]galaxyRow, 0, len(usedBases))
	for id := range usedBases {
		total := galaxy[id]
		if total <= 0 {
			continue
		}
		var topShare float64
		var topEmp string
		empires2 := 0
		for _, e := range empires {
			r := empMap[e].Richness[id]
			if r > 0 {
				empires2++
				share := r / total
				if share > topShare {
					topShare = share
					topEmp = e
				}
			}
		}
		name := id
		if it, ok := items[id]; ok {
			name = it.Name
		}
		rows = append(rows, galaxyRow{
			ItemID:   id,
			Name:     name,
			Total:    total,
			TopEmp:   topEmp,
			TopShare: topShare,
			Empires:  empires2,
		})
	}

	// Monopolies: a single empire holds ≥80% of richness.
	monopolies := slices.Clone(rows)
	monopolies = slices.DeleteFunc(monopolies, func(g galaxyRow) bool { return g.TopShare < 0.80 })
	slices.SortFunc(monopolies, func(a, b galaxyRow) int { return cmp.Compare(b.TopShare, a.TopShare) })

	// Contested: present in many empires but no one dominates (top share <40%).
	contested := slices.Clone(rows)
	contested = slices.DeleteFunc(contested, func(g galaxyRow) bool { return !(g.Empires >= 3 && g.TopShare < 0.40) })
	slices.SortFunc(contested, func(a, b galaxyRow) int { return cmp.Compare(a.TopShare, b.TopShare) })

	fmt.Fprintln(w, "### Galaxy Headlines")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Empire Monopolies** (one empire holds ≥80% of galaxy richness):")
	fmt.Fprintln(w)
	if len(monopolies) == 0 {
		fmt.Fprintln(w, "_None._")
	} else {
		fmt.Fprintln(w, "| Resource | Empire | Share |")
		fmt.Fprintln(w, "|----------|--------|------:|")
		for _, g := range monopolies {
			fmt.Fprintf(w, "| %s | %s | %s |\n", g.Name, titleEmpire(g.TopEmp), formatShare(g.TopShare))
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Contested Resources** (present in ≥3 empires, no empire holds ≥40%):")
	fmt.Fprintln(w)
	if len(contested) == 0 {
		fmt.Fprintln(w, "_None._")
	} else {
		fmt.Fprintln(w, "| Resource | Empires Present | Largest Holder Share |")
		fmt.Fprintln(w, "|----------|---------------:|---------------------:|")
		for _, g := range contested {
			fmt.Fprintf(w, "| %s | %d | %s |\n", g.Name, g.Empires, formatShare(g.TopShare))
		}
	}
	fmt.Fprintln(w)
}

// summarizeResources builds the executive-summary bullet for Section 3.
// Counts how many used-by-recipe base resources each empire is missing.
func summarizeResources(empMap map[string]*EmpireResources, galaxy map[string]float64) string {
	type row struct {
		Empire  string
		Missing int
	}
	rows := make([]row, 0, len(empires))
	usedSet := make(map[string]struct{}, len(galaxy))
	for id, total := range galaxy {
		if total > 0 {
			usedSet[id] = struct{}{}
		}
	}
	for _, e := range empires {
		missing := 0
		er := empMap[e]
		for id := range usedSet {
			if _, ok := er.Present[id]; !ok {
				missing++
			}
		}
		rows = append(rows, row{Empire: e, Missing: missing})
	}
	slices.SortFunc(rows, func(a, b row) int { return cmp.Compare(b.Missing, a.Missing) })
	if len(rows) == 0 {
		return ""
	}
	return fmt.Sprintf("**%s** is the most resource-poor empire, missing %d of %d galaxy base materials; **%s** is missing the fewest (%d).",
		titleEmpire(rows[0].Empire), rows[0].Missing, len(usedSet),
		titleEmpire(rows[len(rows)-1].Empire), rows[len(rows)-1].Missing)
}

// verdictFor maps a galaxy share to the named band.
func verdictFor(share float64) string {
	switch {
	case share >= 0.40:
		return "Dominant"
	case share >= 0.15:
		return "Sufficient"
	case share > 0:
		return "Scarce"
	default:
		return "Missing"
	}
}

func formatShare(s float64) string {
	if s <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", s*100)
}

func formatRichness(r float64) string {
	if r <= 0 {
		return "—"
	}
	if r >= 100 {
		return fmt.Sprintf("%.0f", r)
	}
	return fmt.Sprintf("%.1f", r)
}
