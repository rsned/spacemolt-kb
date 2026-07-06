package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
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
	SVR float64 `json:"svr"` // savings (Recipe)
	HSR bool    `json:"hsr"` // has savings (Recipe)
	PFR float64 `json:"pfr"` // profit (Recipe)
	HPR bool    `json:"hpr"` // has profit (Recipe)
}

type clientRow struct {
	ID    string                `json:"id"`
	Name  string                `json:"name"`
	Kind  string                `json:"kind"`
	Cat   string                `json:"cat"`
	CS    string                `json:"cs"`  // cheapest station id
	CC    float64               `json:"cc"`  // cheapest cost
	FC    int                   `json:"fc"`  // feasible count
	RFC   int                   `json:"rfc"` // recipe feasible count
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
		cr := clientRow{ID: r.ID, Name: r.Name, Kind: r.Kind, Cat: r.Category, CS: r.CheapestStation, CC: r.CheapestCost, FC: r.FeasibleCount, RFC: r.RecipeFeasibleCount, Cells: map[string]clientCell{}}
		for st, c := range r.Cells {
			cr.Cells[st] = clientCell{
				BC: c.BoMCost, BF: c.BoMFeasible, RC: c.RecipeCost, RF: c.RecipeFeasible, RNA: c.RecipeNA,
				SV: c.SavingsBoM, HS: c.HasSavings, PF: c.ProfitBoM, HP: c.HasProfit,
				SVR: c.SavingsRecipe, HSR: c.HasSavingsRecipe, PFR: c.ProfitRecipe, HPR: c.HasProfitRecipe,
			}
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

// detailLine is one station row in a per-target detail table.
type detailLine struct {
	StationName, Empire string

	BoMFeasible bool
	BoMCostStr  string
	BoMCovered  int
	BoMTotal    int

	RecipeNA          bool
	RecipeFeasible    bool
	RecipeCostStr     string
	RecipeFeasibleStr string
	RecipeID          string

	SavingsStr, SavingsClass string
	ProfitStr, ProfitClass   string
}

// commaInt formats v (rounded to the nearest integer) with thousands separators.
func commaInt(v float64) string {
	n := int64(math.Round(v))
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// money renders a value as a comma-formatted string, or an em dash when absent.
func money(v float64, ok bool) string {
	if !ok {
		return "—"
	}
	return commaInt(v)
}

// signClass returns the CSS class for a signed value, or "" when absent.
func signClass(v float64, ok bool) string {
	if !ok {
		return ""
	}
	if v >= 0 {
		return "pos"
	}
	return "neg"
}

// inputRow is one row in a BoM or recipe-input table on the detail page.
type inputRow struct {
	Name string
	Qty  string
	Href string // "" when the input has no KB catalog page
}

// recipeView is one candidate recipe's input table.
type recipeView struct {
	ID        string
	OutputQty string
	Inputs    []inputRow
}

// itemHref returns the relative URL to an item's KB catalog page, or "" when
// its category is unknown (e.g. a sub-assembly with no catalog entry).
func itemHref(id string, categories map[string]string) string {
	cat := categories[id]
	if cat == "" {
		return ""
	}
	return "../items/" + cat + "/" + id + ".html"
}

// qtyStr formats a quantity without trailing zeros (2, 2.5, 0.25).
func qtyStr(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// renderDetail writes the per-target station breakdown page.
func renderDetail(outDir string, row MatrixRow, stations []StationMeta, tgt buildcost.Target, names, categories map[string]string) error {
	t, err := template.ParseFS(tmplFS, "templates/detail.html.tmpl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	inputRowsFor := func(reqs []buildcost.Requirement) []inputRow {
		out := make([]inputRow, 0, len(reqs))
		for _, r := range reqs {
			n := names[r.ItemID]
			if n == "" {
				n = r.ItemID
			}
			out = append(out, inputRow{Name: n, Qty: qtyStr(r.Qty), Href: itemHref(r.ItemID, categories)})
		}
		return out
	}
	bom := inputRowsFor(tgt.BoM)
	var recipes []recipeView
	if tgt.RecipeNA == "" {
		for _, rec := range tgt.Recipes {
			recipes = append(recipes, recipeView{ID: rec.ID, OutputQty: qtyStr(rec.OutputQty), Inputs: inputRowsFor(rec.Inputs)})
		}
	}
	selfHref := itemHref(tgt.ID, categories)
	if row.Kind == "ship" {
		selfHref = "../ships/all.html"
	}

	var lines []detailLine
	for _, s := range stations {
		c, ok := row.Cells[s.ID]
		if !ok {
			continue
		}
		ln := detailLine{
			StationName: s.Name, Empire: s.Empire,
			BoMFeasible: c.BoMFeasible, BoMCostStr: commaInt(c.BoMCost),
			BoMCovered: c.BoMCovered, BoMTotal: c.BoMTotal,
			RecipeNA: c.RecipeNA, RecipeFeasible: c.RecipeFeasible, RecipeID: c.RecipeID,
			SavingsStr: money(c.SavingsBoM, c.HasSavings), SavingsClass: signClass(c.SavingsBoM, c.HasSavings),
			ProfitStr: money(c.ProfitBoM, c.HasProfit), ProfitClass: signClass(c.ProfitBoM, c.HasProfit),
		}
		if c.RecipeNA {
			ln.RecipeCostStr, ln.RecipeFeasibleStr = "n/a", "sub-assemblies not traded"
		} else {
			ln.RecipeCostStr = commaInt(c.RecipeCost)
			if c.RecipeFeasible {
				ln.RecipeFeasibleStr = "yes"
			} else {
				ln.RecipeFeasibleStr = "no"
			}
		}
		lines = append(lines, ln)
	}
	f, err := os.Create(filepath.Join(outDir, row.ID+".html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, map[string]any{
		"Name": row.Name, "Kind": row.Kind, "Lines": lines,
		"SelfHref": selfHref, "BoM": bom, "Recipes": recipes, "RecipeNA": tgt.RecipeNA,
	})
}
