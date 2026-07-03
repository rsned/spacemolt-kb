package main

import (
	"cmp"
	"fmt"
	"strings"
	"slices"
)

// popularityEntry is one row of either the direct or transitive ranking.
type popularityEntry struct {
	ItemID   string
	Name     string
	Category string
	Count    int
}

// contrastRow pairs an intermediate's direct rank with its transitive rank
// for the "where the rankings disagree" callout.
type contrastRow struct {
	ID     string
	Name   string
	Dir    int // 1-indexed direct rank
	Tra    int // 1-indexed transitive rank
	Spread int // Tra - Dir; positive => shallow workhorse, negative => deep bottleneck
}

// writePopularity emits the "Component Popularity" section and returns the
// summary bullet for the executive summary at the top of the report.
func writePopularity(w *strings.Builder, items map[string]*Item, recipes map[string]*Recipe, products []*Product) string {
	intermediates := identifyIntermediates(items, recipes)

	direct := computeDirectDemand(recipes, intermediates, items)
	transitive := computeTransitiveDemand(recipes, products, intermediates, items)

	fmt.Fprintln(w, "## 1. Component Popularity")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "_%d intermediate components identified (items that are both crafted and consumed in further recipes)._\n\n", len(intermediates))

	const topN = 20
	writeRanking(w, "Top 20 Intermediates by Direct Recipe Demand", "Recipes Using", topN, direct)
	writeRanking(w, "Top 20 Intermediates by Transitive Product Demand", "Products Needing", topN, transitive)
	writeStockpiling(w, direct, transitive)
	writeContrast(w, direct, transitive)

	if len(direct) == 0 || len(transitive) == 0 {
		return ""
	}
	return fmt.Sprintf("**%s** is the most directly demanded intermediate (%d recipes consume it); **%s** threads through the most BoM trees (%d products need it).",
		direct[0].Name, direct[0].Count, transitive[0].Name, transitive[0].Count)
}

// identifyIntermediates returns item IDs that are both produced and consumed
// by at least one recipe. Raw ores/materials are excluded.
func identifyIntermediates(items map[string]*Item, recipes map[string]*Recipe) map[string]struct{} {
	produced := make(map[string]struct{})
	consumed := make(map[string]struct{})
	for _, r := range recipes {
		for _, o := range r.Outputs {
			produced[o.ItemID] = struct{}{}
		}
		for _, in := range r.Inputs {
			consumed[in.ItemID] = struct{}{}
		}
	}
	out := make(map[string]struct{})
	for id := range produced {
		if _, also := consumed[id]; !also {
			continue
		}
		if it, ok := items[id]; ok && it.IsBase() {
			continue
		}
		out[id] = struct{}{}
	}
	return out
}

// computeDirectDemand counts, per intermediate, how many distinct recipes
// list it as an input.
func computeDirectDemand(recipes map[string]*Recipe, intermediates map[string]struct{}, items map[string]*Item) []popularityEntry {
	counts := make(map[string]int)
	for _, r := range recipes {
		seen := make(map[string]struct{})
		for _, in := range r.Inputs {
			if _, ok := intermediates[in.ItemID]; !ok {
				continue
			}
			if _, dup := seen[in.ItemID]; dup {
				continue
			}
			seen[in.ItemID] = struct{}{}
			counts[in.ItemID]++
		}
	}
	return makeRanking(counts, items)
}

// computeTransitiveDemand counts, per intermediate, how many final products
// contain that intermediate anywhere in their recipe expansion.
//
// For each product we DFS into its recipe inputs, recording every
// intermediate visited in a per-product set; then we increment the
// intermediate's count by 1 per product whose set contains it.
//
// Ships have no top-level recipe, so we treat each of their build-material
// item IDs as a root for the walk.
func computeTransitiveDemand(recipes map[string]*Recipe, products []*Product, intermediates map[string]struct{}, items map[string]*Item) []popularityEntry {
	itemToRecipe := selectFirstRecipePerItem(recipes)
	counts := make(map[string]int)

	for _, p := range products {
		visited := make(map[string]struct{})
		stack := make(map[string]bool)
		var walk func(id string)
		walk = func(id string) {
			if _, done := visited[id]; done {
				return
			}
			if stack[id] {
				return
			}
			stack[id] = true
			defer func() { delete(stack, id); visited[id] = struct{}{} }()

			// Intermediate hits get counted in the post-walk visited loop;
			// no need to track them here.

			recipe, ok := itemToRecipe[id]
			if !ok {
				return
			}
			for _, in := range recipe.Inputs {
				if it, okIt := items[in.ItemID]; okIt && it.IsBase() {
					continue
				}
				walk(in.ItemID)
			}
		}

		switch p.Kind {
		case "item":
			walk(p.ID)
		case "ship":
			// Ships have no top-level recipe; their direct inputs ARE the
			// build materials, each of which we treat as a walk root so its
			// own intermediates get credited.
			for _, in := range p.DirectInputs {
				if it, okIt := items[in.ItemID]; okIt && it.IsBase() {
					continue
				}
				walk(in.ItemID)
			}
		}

		for id := range visited {
			if _, isInter := intermediates[id]; isInter {
				counts[id]++
			}
		}
	}
	return makeRanking(counts, items)
}

// selectFirstRecipePerItem maps each produced item to one canonical recipe
// so the transitive walk is deterministic.
func selectFirstRecipePerItem(recipes map[string]*Recipe) map[string]*Recipe {
	out := make(map[string]*Recipe)
	ids := make([]string, 0, len(recipes))
	for id := range recipes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		r := recipes[id]
		for _, o := range r.Outputs {
			if _, claimed := out[o.ItemID]; !claimed {
				out[o.ItemID] = r
			}
		}
	}
	return out
}

func makeRanking(counts map[string]int, items map[string]*Item) []popularityEntry {
	out := make([]popularityEntry, 0, len(counts))
	for id, n := range counts {
		name, cat := id, ""
		if it, ok := items[id]; ok {
			name = it.Name
			cat = it.Category
		}
		out = append(out, popularityEntry{ItemID: id, Name: name, Category: cat, Count: n})
	}
	slices.SortFunc(out, func(a, b popularityEntry) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return out
}

func writeRanking(w *strings.Builder, title, countLabel string, n int, entries []popularityEntry) {
	fmt.Fprintf(w, "### %s\n\n", title)
	if len(entries) == 0 {
		fmt.Fprintln(w, "_(no data)_")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "| Rank | Component | %s | Category |\n", countLabel)
	fmt.Fprintln(w, "|------|-----------|---:|----------|")
	limit := min(n, len(entries))
	for i := range limit {
		e := entries[i]
		fmt.Fprintf(w, "| %d | %s | %d | %s |\n", i+1, e.Name, e.Count, e.Category)
	}
	fmt.Fprintln(w)
}

// writeStockpiling lists intermediates that rank in the top 10 of both
// rankings — the workhorses of the crafting tree.
func writeStockpiling(w *strings.Builder, direct, transitive []popularityEntry) {
	const topN = 10
	dir := topSet(direct, topN)
	tra := topSet(transitive, topN)
	var both []popularityEntry
	for _, e := range direct {
		if _, inDir := dir[e.ItemID]; !inDir {
			continue
		}
		if _, inTra := tra[e.ItemID]; !inTra {
			continue
		}
		both = append(both, e)
	}
	fmt.Fprintln(w, "### Worth Stockpiling (Top 10 in Both Rankings)")
	fmt.Fprintln(w)
	if len(both) == 0 {
		fmt.Fprintln(w, "_No components appear in the top 10 of both lists._")
		fmt.Fprintln(w)
		return
	}
	dirRank := rankIndex(direct)
	traRank := rankIndex(transitive)
	fmt.Fprintln(w, "| Component | Direct | Transitive |")
	fmt.Fprintln(w, "|-----------|-------:|-----------:|")
	for _, e := range both {
		fmt.Fprintf(w, "| %s | #%d | #%d |\n", e.Name, dirRank[e.ItemID]+1, traRank[e.ItemID]+1)
	}
	fmt.Fprintln(w)
}

// writeContrast highlights items with the biggest split between their direct
// and transitive ranks.
func writeContrast(w *strings.Builder, direct, transitive []popularityEntry) {
	dirRank := rankIndex(direct)
	traRank := rankIndex(transitive)

	var rows []contrastRow
	for _, e := range direct {
		dr, ok := dirRank[e.ItemID]
		if !ok {
			continue
		}
		tr, ok := traRank[e.ItemID]
		if !ok {
			continue
		}
		if dr >= 50 && tr >= 50 {
			continue
		}
		rows = append(rows, contrastRow{
			ID:     e.ItemID,
			Name:   e.Name,
			Dir:    dr + 1,
			Tra:    tr + 1,
			Spread: tr - dr,
		})
	}

	shallow := pickContrast(rows, true, 5)
	deep := pickContrast(rows, false, 5)

	fmt.Fprintln(w, "### Where the Two Rankings Disagree")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Shallow workhorses** (top of direct, lower in transitive — used in many recipes, but downstream of fewer build chains):")
	fmt.Fprintln(w)
	if len(shallow) == 0 {
		fmt.Fprintln(w, "_(none stand out)_")
	} else {
		for _, r := range shallow {
			fmt.Fprintf(w, "- %s — direct #%d / transitive #%d\n", r.Name, r.Dir, r.Tra)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "**Deep bottlenecks** (top of transitive, lower in direct — used in few recipes directly but those recipes feed many downstream builds):")
	fmt.Fprintln(w)
	if len(deep) == 0 {
		fmt.Fprintln(w, "_(none stand out)_")
	} else {
		for _, r := range deep {
			fmt.Fprintf(w, "- %s — transitive #%d / direct #%d\n", r.Name, r.Tra, r.Dir)
		}
	}
	fmt.Fprintln(w)
}

func pickContrast(rows []contrastRow, shallow bool, n int) []contrastRow {
	sorted := slices.Clone(rows)
	slices.SortFunc(sorted, func(a, b contrastRow) int {
		if shallow {
			return cmp.Compare(b.Spread, a.Spread)
		}
		return cmp.Compare(a.Spread, b.Spread)
	})
	return sorted[:min(n, len(sorted))]
}

func topSet(entries []popularityEntry, n int) map[string]struct{} {
	out := make(map[string]struct{})
	limit := min(n, len(entries))
	for i := range limit {
		out[entries[i].ItemID] = struct{}{}
	}
	return out
}

func rankIndex(entries []popularityEntry) map[string]int {
	out := make(map[string]int, len(entries))
	for i, e := range entries {
		out[e.ItemID] = i
	}
	return out
}
