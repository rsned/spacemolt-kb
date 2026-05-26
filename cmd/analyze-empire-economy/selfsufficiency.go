package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// soloByEmpire is empire -> set of product IDs that empire can solo-build.
type soloByEmpireMap map[string]map[string]struct{}

// soloProducts is product ID -> set of empires that can solo-build it.
type soloProductsMap map[string]map[string]struct{}

// writeSelfSufficiency emits Section 2 (presence-based self-sufficiency).
// It returns the two derived maps so later sections (ease, SSI) can reuse them.
func writeSelfSufficiency(w *strings.Builder, products []*Product, empMap map[string]*EmpireResources) (soloByEmpireMap, soloProductsMap) {
	soloEmp := make(soloByEmpireMap, len(empires))
	for _, e := range empires {
		soloEmp[e] = make(map[string]struct{})
	}
	soloProd := make(soloProductsMap, len(products))

	for _, p := range products {
		soloProd[p.ID] = make(map[string]struct{})
		for _, e := range empires {
			er := empMap[e]
			if canSoloBuild(p, er) {
				soloEmp[e][p.ID] = struct{}{}
				soloProd[p.ID][e] = struct{}{}
			}
		}
	}

	fmt.Fprintln(w, "## 2. Empire Self-Sufficiency Distribution")
	fmt.Fprintln(w)
	writeDistribution(w, products, soloProd)
	writePerEmpire(w, products, soloEmp, soloProd)
	writeDependencyMatrix(w, products, empMap, soloProd)
	return soloEmp, soloProd
}

// canSoloBuild returns true iff every base material in p is present in the
// empire's resource set with non-zero richness.
func canSoloBuild(p *Product, er *EmpireResources) bool {
	for _, m := range p.BaseMaterials {
		if _, ok := er.Present[m.ItemID]; !ok {
			return false
		}
	}
	return true
}

// writeDistribution shows the headline 5/4/3/2/1/0 table.
func writeDistribution(w *strings.Builder, products []*Product, soloProd soloProductsMap) {
	type bucket struct {
		items    []*Product
		ships    []*Product
		examples []string
	}
	buckets := make(map[int]*bucket)
	for i := 0; i <= len(empires); i++ {
		buckets[i] = &bucket{}
	}
	for _, p := range products {
		n := len(soloProd[p.ID])
		b := buckets[n]
		switch p.Kind {
		case "ship":
			b.ships = append(b.ships, p)
		default:
			b.items = append(b.items, p)
		}
	}
	for _, b := range buckets {
		var pool []*Product
		pool = append(pool, b.items...)
		pool = append(pool, b.ships...)
		slices.SortFunc(pool, func(a, c *Product) int { return cmp.Compare(a.Name, c.Name) })
		for _, p := range pool {
			if len(b.examples) >= 4 {
				break
			}
			b.examples = append(b.examples, p.Name)
		}
	}

	fmt.Fprintln(w, "### Headline Distribution")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Empires Able to Solo-Build | # Items | # Ships | Examples |")
	fmt.Fprintln(w, "|----------------------------|--------:|--------:|----------|")
	for n := len(empires); n >= 0; n-- {
		b := buckets[n]
		label := fmt.Sprintf("%d", n)
		switch n {
		case len(empires):
			label = fmt.Sprintf("%d (all empires)", n)
		case 1:
			label = "1 (single empire only)"
		case 0:
			label = "0 (no empire alone)"
		}
		ex := strings.Join(b.examples, ", ")
		if ex == "" {
			ex = "_(none)_"
		}
		fmt.Fprintf(w, "| %s | %d | %d | %s |\n", label, len(b.items), len(b.ships), ex)
	}
	fmt.Fprintln(w)
}

// writePerEmpire renders the per-empire row count + count of exclusive
// products (products only that empire can build).
func writePerEmpire(w *strings.Builder, products []*Product, soloEmp soloByEmpireMap, soloProd soloProductsMap) {
	type row struct {
		Empire    string
		ItemsSolo int
		ShipsSolo int
		ExclItems int
		ExclShips int
	}
	kindByID := make(map[string]string, len(products))
	for _, p := range products {
		kindByID[p.ID] = p.Kind
	}

	rows := make([]row, 0, len(empires))
	for _, e := range empires {
		r := row{Empire: e}
		for pid := range soloEmp[e] {
			emps := soloProd[pid]
			if kindByID[pid] == "ship" {
				r.ShipsSolo++
				if len(emps) == 1 {
					r.ExclShips++
				}
			} else {
				r.ItemsSolo++
				if len(emps) == 1 {
					r.ExclItems++
				}
			}
		}
		rows = append(rows, r)
	}
	slices.SortFunc(rows, func(a, b row) int { return cmp.Compare(a.Empire, b.Empire) })

	fmt.Fprintln(w, "### Per-Empire Breakdown")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Empire | Items Solo | Ships Solo | Exclusive Items | Exclusive Ships |")
	fmt.Fprintln(w, "|--------|-----------:|-----------:|----------------:|----------------:|")
	for _, r := range rows {
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d |\n", titleEmpire(r.Empire), r.ItemsSolo, r.ShipsSolo, r.ExclItems, r.ExclShips)
	}
	fmt.Fprintln(w)
}

// writeDependencyMatrix shows, for each empire pair (A, B), how many extra
// products A would gain access to if it could trade base materials with B.
func writeDependencyMatrix(w *strings.Builder, products []*Product, empMap map[string]*EmpireResources, soloProd soloProductsMap) {
	// matrix[A][B] = number of products A *cannot* solo-build but A∪B *can*.
	matrix := make(map[string]map[string]int, len(empires))
	for _, a := range empires {
		matrix[a] = make(map[string]int, len(empires))
	}

	for _, p := range products {
		built := soloProd[p.ID]
		for _, a := range empires {
			if _, can := built[a]; can {
				continue
			}
			erA := empMap[a]
			for _, b := range empires {
				if a == b {
					continue
				}
				erB := empMap[b]
				if canSoloBuildUnion(p, erA, erB) {
					matrix[a][b]++
				}
			}
		}
	}

	fmt.Fprintln(w, "### Cross-Empire Dependency Matrix")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Cell `[row=A][col=B]` is the number of products `A` cannot solo-build but `A` + `B` together can. Higher = `B` is a more valuable trading partner for `A`.")
	fmt.Fprintln(w)
	fmt.Fprint(w, "| From \\ With |")
	for _, b := range empires {
		fmt.Fprintf(w, " %s |", titleEmpire(b))
	}
	fmt.Fprintln(w)
	fmt.Fprint(w, "|---|")
	for range empires {
		fmt.Fprint(w, "---:|")
	}
	fmt.Fprintln(w)
	for _, a := range empires {
		fmt.Fprintf(w, "| **%s** |", titleEmpire(a))
		for _, b := range empires {
			if a == b {
				fmt.Fprint(w, " — |")
				continue
			}
			fmt.Fprintf(w, " %d |", matrix[a][b])
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

// canSoloBuildUnion is canSoloBuild against the union of two empires'
// presence sets.
func canSoloBuildUnion(p *Product, a, b *EmpireResources) bool {
	for _, m := range p.BaseMaterials {
		_, inA := a.Present[m.ItemID]
		_, inB := b.Present[m.ItemID]
		if !inA && !inB {
			return false
		}
	}
	return true
}

// summarizeSelfSufficiency builds the executive-summary bullet for Section 2.
func summarizeSelfSufficiency(products []*Product, soloEmp soloByEmpireMap) string {
	// Count how many products no empire can solo-build vs. how many all can.
	zero := 0
	all := 0
	totalProducts := 0
	for _, p := range products {
		built := 0
		for _, e := range empires {
			if _, ok := soloEmp[e][p.ID]; ok {
				built++
			}
		}
		switch built {
		case 0:
			zero++
		case len(empires):
			all++
		}
		totalProducts++
	}
	if totalProducts == 0 {
		return ""
	}
	pctAll := float64(all) / float64(totalProducts) * 100
	pctZero := float64(zero) / float64(totalProducts) * 100
	return fmt.Sprintf("Of %d analyzed products, %d (%.1f%%) can be solo-built by every empire and %d (%.1f%%) cannot be solo-built by any single empire — those force interregional trade.",
		totalProducts, all, pctAll, zero, pctZero)
}

func titleEmpire(e string) string {
	if e == "" {
		return ""
	}
	return strings.ToUpper(e[:1]) + e[1:]
}
