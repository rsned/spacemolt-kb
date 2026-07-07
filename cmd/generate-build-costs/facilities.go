// Package-level file for the facility build-cost section.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// galaxyBook pools every station's sell ladder into a single ascending order
// book — the "cheapest sourcing anywhere" reference the galaxy price walks. It
// reuses pooledBook with the full station set. BestBuy is irrelevant here.
func galaxyBook(books map[string]*buildcost.Book) *buildcost.Book {
	ids := make([]string, 0, len(books))
	for id := range books {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return pooledBook(books, ids)
}

// fmtMoney formats a value with thousands separators and two decimals
// (e.g. 28762.9 → "28,762.90"). Reuses commaInt for the integer part.
func fmtMoney(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := math.Floor(v)
	frac := int64(math.Round((v - whole) * 100))
	if frac >= 100 {
		whole++
		frac -= 100
	}
	s := fmt.Sprintf("%s.%02d", commaInt(whole), frac)
	if neg {
		return "-" + s
	}
	return s
}

// FacilityRec is the minimal facility shape the build-cost pages need: identity,
// category, level, its production recipe id (used only for grouping), and the
// direct build_materials that construct it (the Recipe view).
type FacilityRec struct {
	ID       string
	Name     string
	Category string
	Level    int
	RecipeID string
	Build    []buildcost.Requirement
}

// facilityCatDoc / facilityCatItem mirror the fields of catalog_facilities.json
// that the build-cost pages consume.
type facilityCatDoc struct {
	Items []facilityCatItem `json:"items"`
}

type facilityCatItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Level          int    `json:"level"`
	RecipeID       string `json:"recipe_id"`
	BuildMaterials []struct {
		ItemID   string  `json:"item_id"`
		Quantity float64 `json:"quantity"`
	} `json:"build_materials"`
}

// loadFacilityCatalog reads catalog_facilities.json from the newest snapshot dir
// under root and returns the trimmed facility records.
func loadFacilityCatalog(root string) ([]FacilityRec, error) {
	dir, err := findLatestCatalogDir(root)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "catalog_facilities.json"))
	if err != nil {
		return nil, err
	}
	var doc facilityCatDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make([]FacilityRec, 0, len(doc.Items))
	for _, it := range doc.Items {
		rec := FacilityRec{ID: it.ID, Name: it.Name, Category: it.Category, Level: it.Level, RecipeID: it.RecipeID}
		for _, m := range it.BuildMaterials {
			rec.Build = append(rec.Build, buildcost.Requirement{ItemID: m.ItemID, Qty: m.Quantity})
		}
		out = append(out, rec)
	}
	return out, nil
}

// loadFacilityBoM returns facility id -> flattened base-material requirements,
// from bill_of_materials rows with target_type='facility'.
func loadFacilityBoM(craftDB *sql.DB) (map[string][]buildcost.Requirement, error) {
	rows, err := craftDB.Query(`SELECT target_id, base_item_id, quantity
	                            FROM bill_of_materials WHERE target_type='facility'
	                            ORDER BY target_id, base_item_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]buildcost.Requirement{}
	for rows.Next() {
		var id, base string
		var qty float64
		if err := rows.Scan(&id, &base, &qty); err != nil {
			return nil, err
		}
		out[id] = append(out[id], buildcost.Requirement{ItemID: base, Qty: qty})
	}
	return out, rows.Err()
}

// loadRecipeOutputItem returns recipe id -> its first output item id (ordered),
// used to resolve what a production facility makes for grouping.
func loadRecipeOutputItem(craftDB *sql.DB) (map[string]string, error) {
	rows, err := craftDB.Query(`SELECT recipe_id, item_id FROM recipe_outputs ORDER BY recipe_id, item_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var rid, iid string
		if err := rows.Scan(&rid, &iid); err != nil {
			return nil, err
		}
		if _, seen := out[rid]; !seen {
			out[rid] = iid
		}
	}
	return out, rows.Err()
}

// facilityGroup returns the navigation group for a facility: non-production
// facilities group by their own category; production facilities group by the
// market category of the item their recipe produces, falling back to "other"
// when the produced item is unknown or uncategorized.
func facilityGroup(f FacilityRec, recipeOut, itemCat map[string]string) string {
	if f.Category != "production" {
		return f.Category
	}
	out := recipeOut[f.RecipeID]
	if out == "" {
		return "other"
	}
	if cat := itemCat[out]; cat != "" {
		return cat
	}
	return "other"
}

// FacilityComponentCost is one component within a cost view, priced two ways.
type FacilityComponentCost struct {
	ItemID  string
	Name    string
	Href    string // items page link, or "" when uncategorized
	Qty     float64
	MktUnit float64 // sell VWAP per unit
	HasMkt  bool
	GalUnit float64 // average per-unit fill price from the galaxy depth walk
	GalFull bool    // galaxy depth fully covers the required qty
}

// FacilityView is one costing view (BoM or Recipe) of constructing a facility.
type FacilityView struct {
	Components  []FacilityComponentCost
	MktTotal    float64 // sum over priced components
	MktPriced   int     // components with a sell VWAP
	MktCount    int     // total components
	GalTotal    float64 // pooled galaxy cost (partial when infeasible)
	GalFeasible bool    // every component fully covered by galaxy depth
	GalCovered  int     // components fully covered
}

// facItemHref returns the relative link to an item's KB catalog page from a
// facility group page (kb/build-costs/facilities/<group>/index.html → three
// levels up), or "" when the item's category is unknown.
func facItemHref(id string, cats map[string]string) string {
	cat := cats[id]
	if cat == "" {
		return ""
	}
	return "../../../items/" + cat + "/" + id + ".html"
}

// compName returns an item's display name, falling back to its id.
func compName(id string, names map[string]string) string {
	if n := names[id]; n != "" {
		return n
	}
	return id
}

// buildFacilityView prices a requirement set two ways: MKT-AVG (sell VWAP per
// unit, summed over components that have a price) and Galaxy (pooled sell-order
// depth walked cheapest-first). Coverage is reported when depth is short.
func buildFacilityView(reqs []buildcost.Requirement, sellVWAP map[string]float64, galaxy *buildcost.Book, names, cats map[string]string) FacilityView {
	v := FacilityView{MktCount: len(reqs)}
	for _, r := range reqs {
		c := FacilityComponentCost{ItemID: r.ItemID, Qty: r.Qty, Name: compName(r.ItemID, names), Href: facItemHref(r.ItemID, cats)}
		if u, ok := sellVWAP[r.ItemID]; ok && u > 0 {
			c.MktUnit, c.HasMkt = u, true
			v.MktTotal += u * r.Qty
			v.MktPriced++
		}
		w := galaxy.Walk(r.ItemID, r.Qty)
		if w.Shortfall <= 0 && w.Covered > 0 {
			c.GalFull = true
			c.GalUnit = w.Cost / w.Covered
		}
		v.Components = append(v.Components, c)
	}
	gr := galaxy.PriceRequirements(reqs)
	v.GalTotal, v.GalFeasible, v.GalCovered = gr.Cost, gr.Feasible, gr.Covered
	return v
}

// ComponentVM is a rendered component row.
type ComponentVM struct {
	Name, Href, Qty   string
	MktUnit, MktTotal string
	GalUnit, GalTotal string
	GalInfeasible     bool
}

// ViewVM is a rendered cost view (BoM or Recipe).
type ViewVM struct {
	Title         string
	Components    []ComponentVM
	MktBuildCost  string
	MktNote       string // "(k/N priced)" when some components lack a price
	GalBuildCost  string // money when feasible, else "N/M covered"
	GalInfeasible bool
	Empty         bool
}

// FacilityEntryVM is one facility section on a group page.
type FacilityEntryVM struct {
	ID, Name, Href, Produces string
	Level                    int
	BoM, Recipe              ViewVM
}

// FacilityTOCEntry is one entry in the horizontal cross-group TOC.
type FacilityTOCEntry struct {
	Group, Href string
	Count       int
	Active      bool
}

// FacilityGroupPage is a rendered per-group page.
type FacilityGroupPage struct {
	Group, Heading string
	Facilities     []FacilityEntryVM
	TOC            []FacilityTOCEntry
}

// FacilityGroupSummary is a landing-page card.
type FacilityGroupSummary struct {
	Group, Href string
	Count       int
}

// facilityViewVM converts a numeric view into rendered strings, applying the
// em-dash for unpriced/uncovered cells, a "k/N priced" note when MKT-AVG is
// partial, and an "N/M covered" galaxy total when depth is short.
func facilityViewVM(title string, v FacilityView) ViewVM {
	vm := ViewVM{Title: title, Empty: len(v.Components) == 0}
	for _, c := range v.Components {
		cvm := ComponentVM{Name: c.Name, Href: c.Href, Qty: qtyStr(c.Qty), MktUnit: emDash, MktTotal: emDash, GalUnit: emDash, GalTotal: emDash}
		if c.HasMkt {
			cvm.MktUnit = fmtMoney(c.MktUnit)
			cvm.MktTotal = fmtMoney(c.MktUnit * c.Qty)
		}
		if c.GalFull {
			cvm.GalUnit = fmtMoney(c.GalUnit)
			cvm.GalTotal = fmtMoney(c.GalUnit * c.Qty)
		} else {
			cvm.GalInfeasible = true
		}
		vm.Components = append(vm.Components, cvm)
	}
	vm.MktBuildCost = fmtMoney(v.MktTotal)
	if v.MktPriced < v.MktCount {
		vm.MktNote = fmt.Sprintf("(%d/%d priced)", v.MktPriced, v.MktCount)
	}
	if v.GalFeasible {
		vm.GalBuildCost = fmtMoney(v.GalTotal)
	} else {
		vm.GalBuildCost = fmt.Sprintf("%d/%d covered", v.GalCovered, v.MktCount)
		vm.GalInfeasible = true
	}
	return vm
}

// facDetailHref links to a facility's existing KB detail page from a group page
// (three levels up to kb/, then facilities/<category>/<id>.html).
func facDetailHref(f FacilityRec) string {
	return "../../../facilities/" + f.Category + "/" + f.ID + ".html"
}

// buildFacilityPages groups the facilities, builds each facility's two cost
// views, and assembles the per-group pages (facilities alphabetical within a
// group) plus the landing summaries. Both outputs are group-name sorted; every
// page carries the full cross-group TOC with its own group flagged active.
func buildFacilityPages(recs []FacilityRec, facBoM map[string][]buildcost.Requirement, recipeOut, names, cats map[string]string, sellVWAP map[string]float64, galaxy *buildcost.Book) ([]FacilityGroupPage, []FacilityGroupSummary) {
	grouped := map[string][]FacilityEntryVM{}
	for _, f := range recs {
		g := facilityGroup(f, recipeOut, cats)
		entry := FacilityEntryVM{ID: f.ID, Name: f.Name, Href: facDetailHref(f), Level: f.Level}
		if out := recipeOut[f.RecipeID]; out != "" && f.Category == "production" {
			entry.Produces = compName(out, names)
		}
		entry.BoM = facilityViewVM("BoM (ore)", buildFacilityView(facBoM[f.ID], sellVWAP, galaxy, names, cats))
		entry.Recipe = facilityViewVM("Recipe (components)", buildFacilityView(f.Build, sellVWAP, galaxy, names, cats))
		grouped[g] = append(grouped[g], entry)
	}

	groupNames := make([]string, 0, len(grouped))
	for g := range grouped {
		groupNames = append(groupNames, g)
	}
	sort.Strings(groupNames)

	summaries := make([]FacilityGroupSummary, 0, len(groupNames))
	for _, g := range groupNames {
		summaries = append(summaries, FacilityGroupSummary{Group: g, Href: g + "/", Count: len(grouped[g])})
	}

	pages := make([]FacilityGroupPage, 0, len(groupNames))
	for _, g := range groupNames {
		facs := grouped[g]
		sort.Slice(facs, func(i, j int) bool { return facs[i].Name < facs[j].Name })
		toc := make([]FacilityTOCEntry, 0, len(groupNames))
		for _, other := range groupNames {
			toc = append(toc, FacilityTOCEntry{Group: other, Href: "../" + other + "/", Count: len(grouped[other]), Active: other == g})
		}
		pages = append(pages, FacilityGroupPage{Group: g, Heading: g, Facilities: facs, TOC: toc})
	}
	return pages, summaries
}
