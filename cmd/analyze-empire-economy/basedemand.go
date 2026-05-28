package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// baseDemandRow captures per-base-material demand across all analyzed products.
type baseDemandRow struct {
	ItemID         string
	Name           string
	ItemConsumers  int
	ShipConsumers  int
	TotalConsumers int
	// Consumers is a sorted list of consumer product names. Always populated
	// (the size of low-demand consumer lists is the whole point of the section).
	Consumers []string
}

// writeBaseDemand emits Section 2 — per-base-material demand across products.
// It returns the executive-summary bullet describing the high/low ends.
func writeBaseDemand(w *strings.Builder, items map[string]*Item, products []*Product) string {
	rows := computeBaseDemand(items, products)
	if len(rows) == 0 {
		return ""
	}

	fmt.Fprintln(w, "## 2. Base Material Demand")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "_For each base material (ore / exotic material), how many of the %d analyzed products transitively need it._\n\n", len(products))

	writeBaseDemandFull(w, rows)
	writeBaseDemandLowEnd(w, rows)
	writeBaseDemandFactionFlagship(w, rows)

	top := rows[0]
	// Lowest non-zero demand row — anything literally unused doesn't surprise.
	var bottom *baseDemandRow
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].TotalConsumers > 0 {
			bottom = &rows[i]
			break
		}
	}
	if bottom == nil {
		return ""
	}
	ratio := float64(top.TotalConsumers) / float64(bottom.TotalConsumers)
	return fmt.Sprintf("Base-material demand spans **%.0fx**: **%s** feeds %d products at the top vs. **%s** at just %d at the bottom.",
		ratio, top.Name, top.TotalConsumers, bottom.Name, bottom.TotalConsumers)
}

// computeBaseDemand builds the per-base-material rows. Sorted descending by
// total consumer count.
func computeBaseDemand(items map[string]*Item, products []*Product) []baseDemandRow {
	// consumerNames[baseID] = set of product names that need this base.
	consumerNames := make(map[string]map[string]struct{})
	consumerKinds := make(map[string]map[string]int) // baseID -> kind -> count

	for _, p := range products {
		for _, m := range p.BaseMaterials {
			if consumerNames[m.ItemID] == nil {
				consumerNames[m.ItemID] = make(map[string]struct{})
				consumerKinds[m.ItemID] = make(map[string]int)
			}
			consumerNames[m.ItemID][p.Name] = struct{}{}
			consumerKinds[m.ItemID][p.Kind]++
		}
	}

	// Include every base material that appears in any product's BoM. Missing
	// bases (e.g. exotic_crystal nobody uses) are dropped — they'd show as 0
	// and clutter the list.
	rows := make([]baseDemandRow, 0, len(consumerNames))
	for id, names := range consumerNames {
		name := id
		if it, ok := items[id]; ok {
			name = it.Name
		}
		nameList := make([]string, 0, len(names))
		for n := range names {
			nameList = append(nameList, n)
		}
		slices.Sort(nameList)
		rows = append(rows, baseDemandRow{
			ItemID:         id,
			Name:           name,
			ItemConsumers:  consumerKinds[id]["item"],
			ShipConsumers:  consumerKinds[id]["ship"],
			TotalConsumers: len(names),
			Consumers:      nameList,
		})
	}
	slices.SortFunc(rows, func(a, b baseDemandRow) int {
		if c := cmp.Compare(b.TotalConsumers, a.TotalConsumers); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return rows
}

// writeBaseDemandFull renders the headline ranked table — all base materials,
// sorted by total consumer count desc. This is the canonical demand map.
func writeBaseDemandFull(w *strings.Builder, rows []baseDemandRow) {
	fmt.Fprintln(w, "### Demand Ranking (All Base Materials)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Rank | Material | Total | Items | Ships |")
	fmt.Fprintln(w, "|-----:|----------|------:|------:|------:|")
	for i, r := range rows {
		fmt.Fprintf(w, "| %d | %s | %d | %d | %d |\n", i+1, r.Name, r.TotalConsumers, r.ItemConsumers, r.ShipConsumers)
	}
	fmt.Fprintln(w)
}

// writeBaseDemandLowEnd lists base materials with the fewest consumers and
// names each consumer. The threshold (≤8) is tuned to cover the genuinely
// niche cases without dragging in mid-tier materials.
func writeBaseDemandLowEnd(w *strings.Builder, rows []baseDemandRow) {
	const threshold = 8

	var low []baseDemandRow
	for _, r := range rows {
		if r.TotalConsumers > 0 && r.TotalConsumers <= threshold {
			low = append(low, r)
		}
	}
	if len(low) == 0 {
		return
	}
	slices.SortFunc(low, func(a, b baseDemandRow) int {
		if c := cmp.Compare(a.TotalConsumers, b.TotalConsumers); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	fmt.Fprintf(w, "### Narrowest Demand (≤%d consumers — every consumer listed)\n\n", threshold)
	fmt.Fprintln(w, "Base materials whose entire demand is concentrated in a handful of products. Useful for spotting near-vestigial resources and identifying which specific products gate each one.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Material | Consumers | Products |")
	fmt.Fprintln(w, "|----------|----------:|----------|")
	for _, r := range low {
		fmt.Fprintf(w, "| %s | %d | %s |\n", r.Name, r.TotalConsumers, strings.Join(r.Consumers, ", "))
	}
	fmt.Fprintln(w)
}

// writeBaseDemandFactionFlagship surfaces materials whose ship-count vastly
// outweighs their item-count. These look low-demand on a flat ranking but
// gate faction-flagship ship lines — the "monopoly leverage" materials.
func writeBaseDemandFactionFlagship(w *strings.Builder, rows []baseDemandRow) {
	// Heuristic: narrow item footprint (≤15) but meaningful ship reach (≥15),
	// AND ship-count outweighs item-count by ≥3:1. Catches monopoly-leverage
	// materials without sweeping in broadly-used resources that happen to
	// skew ship-heavy.
	const maxItems = 15
	const minShips = 15
	const ratio = 3

	var flagship []baseDemandRow
	for _, r := range rows {
		if r.ItemConsumers > maxItems {
			continue
		}
		if r.ShipConsumers < minShips {
			continue
		}
		if r.ItemConsumers == 0 || r.ShipConsumers >= ratio*r.ItemConsumers {
			flagship = append(flagship, r)
		}
	}
	if len(flagship) == 0 {
		return
	}
	slices.SortFunc(flagship, func(a, b baseDemandRow) int { return cmp.Compare(b.ShipConsumers, a.ShipConsumers) })

	fmt.Fprintln(w, "### Faction-Flagship Materials (Ship-Heavy Demand)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Base materials whose ship-consumers outnumber their item-consumers by at least 3:1 — they gate faction-flagship ship lines rather than general crafting. These look underused on a flat count but carry monopoly leverage.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Material | Items | Ships | Ratio |")
	fmt.Fprintln(w, "|----------|------:|------:|------:|")
	for _, r := range flagship {
		var ratioStr string
		if r.ItemConsumers == 0 {
			ratioStr = "ships only"
		} else {
			ratioStr = fmt.Sprintf("%.1fx", float64(r.ShipConsumers)/float64(r.ItemConsumers))
		}
		fmt.Fprintf(w, "| %s | %d | %d | %s |\n", r.Name, r.ItemConsumers, r.ShipConsumers, ratioStr)
	}
	fmt.Fprintln(w)
}
