package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
)

//go:embed templates/*.tmpl
var tmplFS embed.FS

// clientCell is the compact per-station cell in the embedded JSON.
type clientCell struct {
	BC  float64 `json:"bc"`  // BoM cost
	BF  bool    `json:"bf"`  // BoM feasible
	RC  float64 `json:"rc"`  // Recipe cost
	RF  bool    `json:"rf"`  // Recipe feasible
	RNA bool    `json:"rna"` // Recipe NA
	SV  float64 `json:"sv"`  // savings (BoM)
	HS  bool    `json:"hs"`  // has savings
	PF  float64 `json:"pf"`  // profit (BoM)
	HP  bool    `json:"hp"`  // has profit
}

type clientRow struct {
	ID    string                `json:"id"`
	Name  string                `json:"name"`
	Kind  string                `json:"kind"`
	Cat   string                `json:"cat"`
	CS    string                `json:"cs"` // cheapest station id
	CC    float64               `json:"cc"` // cheapest cost
	FC    int                   `json:"fc"` // feasible count
	Cells map[string]clientCell `json:"cells"`
}

type clientStation struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Empire string `json:"empire"`
}

type clientModel struct {
	Stations []clientStation `json:"stations"`
	Rows     []clientRow     `json:"rows"`
}

func toClientModel(m Matrix) clientModel {
	cm := clientModel{}
	for _, s := range m.Stations {
		cm.Stations = append(cm.Stations, clientStation{ID: s.ID, Name: s.Name, Empire: s.Empire})
	}
	for _, r := range m.Rows {
		cr := clientRow{ID: r.ID, Name: r.Name, Kind: r.Kind, Cat: r.Category, CS: r.CheapestStation, CC: r.CheapestCost, FC: r.FeasibleCount, Cells: map[string]clientCell{}}
		for st, c := range r.Cells {
			cr.Cells[st] = clientCell{BC: c.BoMCost, BF: c.BoMFeasible, RC: c.RecipeCost, RF: c.RecipeFeasible, RNA: c.RecipeNA, SV: c.SavingsBoM, HS: c.HasSavings, PF: c.ProfitBoM, HP: c.HasProfit}
		}
		cm.Rows = append(cm.Rows, cr)
	}
	return cm
}

// matrixJSON serializes the compact client model.
func matrixJSON(m Matrix) (string, error) {
	b, err := json.Marshal(toClientModel(m))
	return string(b), err
}

// renderIndex writes the landing matrix page.
func renderIndex(outDir string, m Matrix) error {
	js, err := matrixJSON(m)
	if err != nil {
		return err
	}
	t, err := template.ParseFS(tmplFS, "templates/index.html.tmpl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, map[string]any{"JSON": template.JS(js)})
}
