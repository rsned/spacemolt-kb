package main

import (
	"cmp"
	"fmt"
	"strings"
	"slices"
)

// writeSSI emits Section 4 (Composite Self-Sufficiency Index) and returns
// the summary bullet for the executive summary.
//
// SSI = 0.5 * breadth(%) + 0.5 * mean_ease(%)
//   breadth   = solo-buildable products / total products
//   mean_ease = mean (over solo-buildable products) of the ease score (0..1)
func writeSSI(w *strings.Builder, products []*Product, soloEmp soloByEmpireMap, soloProd soloProductsMap, ease easeByEmpireProductMap) string {
	type row struct {
		Empire    string
		SSI       float64
		Breadth   float64
		MeanEase  float64
		ItemsSolo int
		ShipsSolo int
		Exclusive int
	}

	kindByID := make(map[string]string, len(products))
	for _, p := range products {
		kindByID[p.ID] = p.Kind
	}

	total := len(products)
	rows := make([]row, 0, len(empires))
	for _, e := range empires {
		soloSet := soloEmp[e]
		breadth := 0.0
		if total > 0 {
			breadth = float64(len(soloSet)) / float64(total)
		}
		var sumEase float64
		var items, ships, excl int
		for pid := range soloSet {
			sumEase += ease[e][pid]
			if kindByID[pid] == "ship" {
				ships++
			} else {
				items++
			}
			if len(soloProd[pid]) == 1 {
				excl++
			}
		}
		meanEase := 0.0
		if len(soloSet) > 0 {
			meanEase = sumEase / float64(len(soloSet))
		}
		ssi := 0.5*breadth*100 + 0.5*meanEase*100
		rows = append(rows, row{
			Empire:    e,
			SSI:       ssi,
			Breadth:   breadth,
			MeanEase:  meanEase,
			ItemsSolo: items,
			ShipsSolo: ships,
			Exclusive: excl,
		})
	}
	slices.SortFunc(rows, func(a, b row) int { return cmp.Compare(b.SSI, a.SSI) })

	fmt.Fprintln(w, "## 4. Composite Self-Sufficiency Index")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SSI is a 0–100 score: half breadth (how much of the catalog the empire can solo-build) and half comfort (how rich the bottleneck inputs are, averaged across those products).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Rank | Empire | SSI | Breadth | Mean Ease | Items Solo | Ships Solo | Exclusive |")
	fmt.Fprintln(w, "|-----:|--------|----:|--------:|----------:|-----------:|-----------:|----------:|")
	for i, r := range rows {
		fmt.Fprintf(w, "| %d | %s | %.1f | %s | %s | %d | %d | %d |\n",
			i+1, titleEmpire(r.Empire), r.SSI, formatShare(r.Breadth), formatShare(r.MeanEase),
			r.ItemsSolo, r.ShipsSolo, r.Exclusive)
	}
	fmt.Fprintln(w)

	if len(rows) == 0 {
		return ""
	}
	return fmt.Sprintf("Composite SSI leader: **%s** (%.1f). Trailing: **%s** (%.1f).",
		titleEmpire(rows[0].Empire), rows[0].SSI,
		titleEmpire(rows[len(rows)-1].Empire), rows[len(rows)-1].SSI)
}
