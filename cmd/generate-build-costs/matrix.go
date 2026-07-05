package main

import (
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// RowCell is the render-ready per-station result for one target.
type RowCell struct {
	BoMCost        float64
	BoMFeasible    bool
	BoMCovered     int
	BoMTotal       int
	RecipeCost     float64
	RecipeFeasible bool
	RecipeNA       bool
	RecipeID       string
	SavingsBoM     float64
	HasSavings     bool
	ProfitBoM      float64
	HasProfit      bool
}

// MatrixRow is one item/ship across all stations, plus summary columns.
type MatrixRow struct {
	ID              string
	Name            string
	Kind            string
	Category        string
	Cells           map[string]RowCell
	CheapestStation string
	CheapestCost    float64
	FeasibleCount   int
}

// Matrix is the full render model: station columns and target rows.
type Matrix struct {
	Stations []StationMeta
	Rows     []MatrixRow
}

// BuildMatrix computes every target×station cell and the per-row summaries.
func BuildMatrix(targets []buildcost.Target, books map[string]*buildcost.Book, stations []StationMeta,
	names, categories map[string]string, listings map[string]map[string]float64, catalogPrice map[string]int) Matrix {
	m := Matrix{Stations: stations}
	for _, t := range targets {
		row := MatrixRow{ID: t.ID, Name: names[t.ID], Kind: t.Kind, Category: categories[t.ID], Cells: map[string]RowCell{}}
		if row.Name == "" {
			row.Name = t.ID
		}
		haveCheapest := false
		for _, st := range stations {
			book := books[st.ID]
			if book == nil {
				continue
			}
			var margin buildcost.Margin
			if t.Kind == "ship" {
				margin = shipMargin(listings, catalogPrice, t.ID, st.ID)
			} else {
				margin = itemMargin(book, t.ID)
			}
			c := buildcost.BuildCell(t, st.ID, book, margin)
			rc := RowCell{
				BoMCost: c.BoM.Cost, BoMFeasible: c.BoM.Feasible,
				BoMCovered: c.BoM.Covered, BoMTotal: c.BoM.Total,
				RecipeCost: c.Recipe.Cost, RecipeFeasible: c.Recipe.Feasible,
				RecipeNA: c.Recipe.NA, RecipeID: c.Recipe.RecipeID,
			}
			if s, ok := c.Margin.SavingsVsAsk(c.BoM.Cost); ok {
				rc.SavingsBoM, rc.HasSavings = s, true
			}
			if p, ok := c.Margin.ProfitVsBid(c.BoM.Cost); ok {
				rc.ProfitBoM, rc.HasProfit = p, true
			}
			row.Cells[st.ID] = rc
			if c.BoM.Feasible {
				row.FeasibleCount++
				if !haveCheapest || c.BoM.Cost < row.CheapestCost {
					row.CheapestStation, row.CheapestCost, haveCheapest = st.ID, c.BoM.Cost, true
				}
			}
		}
		m.Rows = append(m.Rows, row)
	}
	sort.Slice(m.Rows, func(i, j int) bool { return m.Rows[i].Name < m.Rows[j].Name })
	return m
}
