package main

import (
	"math"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// RowCell is the render-ready per-station result for one target.
type RowCell struct {
	BoMCost          float64
	BoMFeasible      bool
	BoMCovered       int
	BoMTotal         int
	RecipeCost       float64
	RecipeFeasible   bool
	RecipeCovered    int
	RecipeTotal      int
	RecipeNA         bool
	RecipeID         string
	SavingsBoM       float64
	HasSavings       bool
	ProfitBoM        float64
	HasProfit        bool
	SavingsRecipe    float64
	HasSavingsRecipe bool
	ProfitRecipe     float64
	HasProfitRecipe  bool
}

// MatrixRow is one item/ship across all stations, plus summary columns.
type MatrixRow struct {
	ID                  string
	Name                string
	Kind                string
	Category            string
	Cells               map[string]RowCell
	CheapestStation     string
	CheapestCost        float64
	FeasibleCount       int // stations where BoM is feasible
	RecipeFeasibleCount int // stations where Recipe is feasible (and not NA)
}

// Matrix is the full render model: station columns and target rows.
type Matrix struct {
	Stations []StationMeta
	Rows     []MatrixRow
}

// Viability summarizes, over all targets, how many are buildable at one or more
// stations as BoM, as Recipe, or by either mode (within the matrix's radius).
type Viability struct {
	Total  int
	BoM    int
	Recipe int
	Either int
}

// Pct returns n as a percentage of the total, rounded to a whole number (0 if empty).
func (v Viability) Pct(n int) int {
	if v.Total == 0 {
		return 0
	}
	return int(math.Round(100 * float64(n) / float64(v.Total)))
}

// matrixViability counts targets makeable at >=1 station in each mode.
func matrixViability(m Matrix) Viability {
	v := Viability{Total: len(m.Rows)}
	for _, r := range m.Rows {
		bom := r.FeasibleCount > 0
		rec := r.RecipeFeasibleCount > 0
		if bom {
			v.BoM++
		}
		if rec {
			v.Recipe++
		}
		if bom || rec {
			v.Either++
		}
	}
	return v
}

// BuildMatrix computes every target×station cell and the per-row summaries.
func BuildMatrix(targets []buildcost.Target, costBooks, marginBooks map[string]*buildcost.Book, stations []StationMeta,
	names, categories map[string]string, listings map[string]map[string]float64, catalogPrice map[string]int) Matrix {
	var active []StationMeta
	for _, st := range stations {
		if costBooks[st.ID] != nil {
			active = append(active, st)
		}
	}
	m := Matrix{Stations: active}
	for _, t := range targets {
		row := MatrixRow{ID: t.ID, Name: names[t.ID], Kind: t.Kind, Category: categories[t.ID], Cells: map[string]RowCell{}}
		if row.Name == "" {
			row.Name = t.ID
		}
		haveCheapest := false
		for _, st := range active {
			book := costBooks[st.ID]
			if book == nil {
				continue
			}
			var margin buildcost.Margin
			if t.Kind == "ship" {
				margin = shipMargin(listings, catalogPrice, t.ID, st.ID)
			} else {
				margin = itemMargin(marginBooks[st.ID], t.ID)
			}
			c := buildcost.BuildCell(t, st.ID, book, margin)
			rc := RowCell{
				BoMCost: c.BoM.Cost, BoMFeasible: c.BoM.Feasible,
				BoMCovered: c.BoM.Covered, BoMTotal: c.BoM.Total,
				RecipeCost: c.Recipe.Cost, RecipeFeasible: c.Recipe.Feasible,
				RecipeCovered: c.Recipe.Covered, RecipeTotal: c.Recipe.Total,
				RecipeNA: c.Recipe.NA, RecipeID: c.Recipe.RecipeID,
			}
			// BoM-mode margins: only meaningful when the BoM build actually completes.
			// (Infeasible BoM cost is only a partial sum, so a margin off it would mislead.)
			if c.BoM.Feasible {
				if s, ok := c.Margin.SavingsVsAsk(c.BoM.Cost); ok {
					rc.SavingsBoM, rc.HasSavings = s, true
				}
				if p, ok := c.Margin.ProfitVsBid(c.BoM.Cost); ok {
					rc.ProfitBoM, rc.HasProfit = p, true
				}
			}
			// Recipe-mode margins: only when the recipe build completes and applies.
			if c.Recipe.Feasible && !c.Recipe.NA {
				if s, ok := c.Margin.SavingsVsAsk(c.Recipe.Cost); ok {
					rc.SavingsRecipe, rc.HasSavingsRecipe = s, true
				}
				if p, ok := c.Margin.ProfitVsBid(c.Recipe.Cost); ok {
					rc.ProfitRecipe, rc.HasProfitRecipe = p, true
				}
			}
			row.Cells[st.ID] = rc
			if c.BoM.Feasible {
				row.FeasibleCount++
				if !haveCheapest || c.BoM.Cost < row.CheapestCost {
					row.CheapestStation, row.CheapestCost, haveCheapest = st.ID, c.BoM.Cost, true
				}
			}
			if c.Recipe.Feasible && !c.Recipe.NA {
				row.RecipeFeasibleCount++
			}
		}
		m.Rows = append(m.Rows, row)
	}
	sort.Slice(m.Rows, func(i, j int) bool {
		if m.Rows[i].Name != m.Rows[j].Name {
			return m.Rows[i].Name < m.Rows[j].Name
		}
		return m.Rows[i].ID < m.Rows[j].ID
	})
	return m
}
