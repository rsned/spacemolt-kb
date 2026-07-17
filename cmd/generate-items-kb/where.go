package main

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	htmltpl "html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	humanize "github.com/dustin/go-humanize"
)

// errNoPublicFacilities is returned when the knowledge DB predates the
// public_facilities table. Callers treat this as "skip the page", not "fail
// the build" -- the table is new and older snapshots will not have it.
var errNoPublicFacilities = errors.New("public_facilities table not present")

// PublicFacility is one public production line at one station, joined to its
// station, system, and owning faction.
type PublicFacility struct {
	StationID   string
	StationName string
	SystemID    string
	SystemName  string

	FacilityID   string
	FacilityName string
	FacilityType string // details_json.type; links to kb/facilities/production/<type>.html

	RecipeID string
	Level    int

	FeePerRun    int
	QtyPerRun    int // details_json.production.output_per_run
	ItemsPerHour int // details_json.production.items_per_hour

	OwnerID   string // raw faction hash; always set
	OwnerName string // empty when the faction does not resolve
	OwnerTag  string

	LastSeenTick int

	// PriceRank is priceBest, priceWorst, or empty, set by markPriceExtremes
	// relative to the other lines running the same recipe in the same table.
	// It is a rendering hint, not a property of the facility: the same line is
	// ranked separately in the by-recipe and by-station views, whose comparison
	// sets differ.
	PriceRank string
}

// Price ranks, used as CSS class suffixes (fee-best / fee-worst).
const (
	priceBest  = "best"
	priceWorst = "worst"
)

// perUnitFee is what one unit of output costs to rent, the only figure that
// compares two lines fairly: a line charging twice as much per run but yielding
// ten times the goods is the cheaper line.
//
// Reports false when the line yields nothing, which cannot be ranked.
func perUnitFee(f PublicFacility) (float64, bool) {
	if f.QtyPerRun <= 0 {
		return 0, false
	}
	return float64(f.FeePerRun) / float64(f.QtyPerRun), true
}

// markPriceExtremes tags the cheapest and costliest lines of one comparison set
// -- lines running the same recipe, which is the only set where price is
// comparable. Callers pass pointers into the slice they are about to render.
//
// Fewer than two priced lines, or every line priced alike, leaves the whole set
// plain: a lone row is neither the best nor the worst deal, and marking a
// uniformly-priced set both best and worst says nothing. Tied extremes all
// colour, since they are equally the best (or worst) available.
func markPriceExtremes(rows []*PublicFacility) {
	var lo, hi float64
	priced := 0
	for _, r := range rows {
		v, ok := perUnitFee(*r)
		if !ok {
			continue
		}
		if priced == 0 || v < lo {
			lo = v
		}
		if priced == 0 || v > hi {
			hi = v
		}
		priced++
	}
	if priced < 2 || lo == hi {
		return
	}
	for _, r := range rows {
		v, ok := perUnitFee(*r)
		if !ok {
			continue
		}
		switch v {
		case lo:
			r.PriceRank = priceBest
		case hi:
			r.PriceRank = priceWorst
		}
	}
}

// facilityDetails is the narrow slice of details_json we consume. The three
// values here are the only ones not available as table columns.
//
// The numbers are decoded as float64 rather than int: the server has emitted
// fractional throughput before (ticks_per_run is fractional), and a float in
// items_per_hour would otherwise fail the whole row's unmarshal.
type facilityDetails struct {
	Type       string `json:"type"`
	Production struct {
		ItemsPerHour float64 `json:"items_per_hour"`
		OutputPerRun float64 `json:"output_per_run"`
	} `json:"production"`
}

// hasPublicFacilities reports whether the knowledge DB has the table at all.
func hasPublicFacilities(db *sql.DB) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'public_facilities'`,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// loadPublicFacilities reads every public production line, resolving each to
// its station, system, and owning faction.
//
// station_id is a bases.id for some stations and a pois.id for others, so the
// POI join goes through COALESCE(b.poi_id, pf.station_id) to accept either.
// The faction join is a LEFT JOIN because roughly a third of owner hashes do
// not resolve; those render as a bare hash rather than a broken link.
func loadPublicFacilities(db *sql.DB) ([]PublicFacility, error) {
	ok, err := hasPublicFacilities(db)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNoPublicFacilities
	}

	rows, err := db.Query(`
		SELECT pf.station_id, pf.facility_id, pf.recipe_id, pf.facility_name,
		       pf.level, pf.rental_fee_per_run, pf.owner_faction,
		       pf.details_json, pf.last_seen_tick,
		       COALESCE(b.name, ''), COALESCE(p.name, ''),
		       COALESCE(s.id, ''), COALESCE(s.name, ''),
		       COALESCE(f.name, ''), COALESCE(f.tag, '')
		FROM public_facilities pf
		LEFT JOIN bases    b ON b.id = pf.station_id
		LEFT JOIN pois     p ON p.id = COALESCE(b.poi_id, pf.station_id)
		LEFT JOIN systems  s ON s.id = p.system_id
		LEFT JOIN factions f ON f.faction_id = pf.owner_faction
		WHERE pf.public = 1
		ORDER BY pf.station_id, pf.facility_id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []PublicFacility
	for rows.Next() {
		var f PublicFacility
		var detailsJSON string
		var baseName string
		var poiName string
		if err := rows.Scan(
			&f.StationID, &f.FacilityID, &f.RecipeID, &f.FacilityName,
			&f.Level, &f.FeePerRun, &f.OwnerID,
			&detailsJSON, &f.LastSeenTick,
			&baseName, &poiName,
			&f.SystemID, &f.SystemName,
			&f.OwnerName, &f.OwnerTag,
		); err != nil {
			return nil, err
		}

		// Station display name: bases.name -> pois.name -> raw station_id.
		switch {
		case baseName != "":
			f.StationName = baseName
		case poiName != "":
			f.StationName = poiName
		default:
			f.StationName = f.StationID
		}

		// A malformed details_json degrades this row's throughput cells to
		// blank. It never fails the page.
		var d facilityDetails
		if err := json.Unmarshal([]byte(detailsJSON), &d); err == nil {
			f.FacilityType = d.Type
			f.ItemsPerHour = int(d.Production.ItemsPerHour)
			f.QtyPerRun = int(d.Production.OutputPerRun)
		}

		out = append(out, f)
	}
	return out, rows.Err()
}

// WhereRecipeGroup is one recipe and every public line that runs it.
type WhereRecipeGroup struct {
	RecipeID       string
	RecipeName     string
	RecipeCategory string
	RecipeDirName  string // category with spaces -> underscores, for the URL path
	FacilityOnly   bool
	Outputs        []RecipeItem
	Facilities     []PublicFacility
}

// WhereStationFacility is a public line with its recipe metadata attached, for
// rendering inside a station's section.
type WhereStationFacility struct {
	PublicFacility
	RecipeName    string
	RecipeDirName string
	Outputs       []RecipeItem
}

// WhereStationCategory is one recipe-category block within a station.
type WhereStationCategory struct {
	Category   string
	Facilities []WhereStationFacility
}

// WhereStationGroup is one station and everything craftable there, bucketed by
// recipe category.
type WhereStationGroup struct {
	StationID   string
	StationName string
	SystemID    string
	SystemName  string
	Count       int
	FeeMin      int
	FeeMax      int
	Categories  []WhereStationCategory
}

// WhereOwnerSummary is one row of the owner rollup above the tabs: a faction
// (or the stations themselves) and how much public industry it runs.
type WhereOwnerSummary struct {
	OwnerID   string
	OwnerName string // empty when the faction does not resolve
	OwnerTag  string

	// StationOwned marks the row for lines the stations run themselves. Those
	// carry no faction_id, so they aggregate into a single row rather than
	// silently sharing the empty-ID faction bucket with nothing to name it.
	StationOwned bool

	Facilities int
	Stations   int
	Recipes    int
	FeeMin     int
	FeeMax     int
}

// summarizeOwners rolls facilities up by owning faction, with station-owned
// lines gathered into one row. Sorted by facility count descending; OwnerID
// closes the sort so equal counts cannot reorder between regenerations.
func summarizeOwners(facs []PublicFacility) []WhereOwnerSummary {
	type acc struct {
		row      WhereOwnerSummary
		stations map[string]bool
		recipes  map[string]bool
	}
	byOwner := make(map[string]*acc)

	for _, f := range facs {
		a, ok := byOwner[f.OwnerID]
		if !ok {
			a = &acc{
				row: WhereOwnerSummary{
					OwnerID:      f.OwnerID,
					OwnerName:    f.OwnerName,
					OwnerTag:     f.OwnerTag,
					StationOwned: f.OwnerID == "",
					FeeMin:       f.FeePerRun,
					FeeMax:       f.FeePerRun,
				},
				stations: make(map[string]bool),
				recipes:  make(map[string]bool),
			}
			byOwner[f.OwnerID] = a
		}
		a.row.Facilities++
		a.row.FeeMin = min(a.row.FeeMin, f.FeePerRun)
		a.row.FeeMax = max(a.row.FeeMax, f.FeePerRun)
		a.stations[f.StationID] = true
		a.recipes[f.RecipeID] = true
	}

	out := make([]WhereOwnerSummary, 0, len(byOwner))
	for _, a := range byOwner {
		a.row.Stations = len(a.stations)
		a.row.Recipes = len(a.recipes)
		out = append(out, a.row)
	}
	slices.SortFunc(out, func(a, b WhereOwnerSummary) int {
		if c := cmp.Compare(b.Facilities, a.Facilities); c != 0 {
			return c // most facilities first
		}
		if c := cmp.Compare(a.OwnerName, b.OwnerName); c != 0 {
			return c
		}
		return cmp.Compare(a.OwnerID, b.OwnerID)
	})
	return out
}

// NoFacilityRecipe is a recipe with no known public line, for the two dense
// tables at the bottom of the by-recipe tab.
type NoFacilityRecipe struct {
	ID             string
	Name           string
	Category       string
	DirName        string
	OutputID       string
	OutputName     string
	OutputCategory string
	OutputQty      int
	CraftingTime   float64
}

// groupByRecipe buckets public lines by the recipe they run, sorted by recipe
// name. Facilities whose recipe is absent from the crafting DB are dropped --
// every ID resolves today, and a group with no name or category would render as
// a dead link.
//
// Lines within a group answer "where should I go to make this", so they sort by
// station name, then cheapest fee, then highest throughput, then faction name.
// StationID and FacilityID close the sort: neither station name nor faction
// name is a key, and slices.SortFunc is not stable, so without a unique final
// comparison the committed HTML could reorder between regenerations.
func groupByRecipe(facs []PublicFacility, recipes map[string]*Recipe) []WhereRecipeGroup {
	byRecipe := make(map[string][]PublicFacility)
	for _, f := range facs {
		if _, ok := recipes[f.RecipeID]; !ok {
			continue
		}
		byRecipe[f.RecipeID] = append(byRecipe[f.RecipeID], f)
	}

	groups := make([]WhereRecipeGroup, 0, len(byRecipe))
	for id, lines := range byRecipe {
		r := recipes[id]
		slices.SortFunc(lines, func(a, b PublicFacility) int {
			if c := cmp.Compare(a.StationName, b.StationName); c != 0 {
				return c
			}
			if c := cmp.Compare(a.FeePerRun, b.FeePerRun); c != 0 {
				return c // cheapest first
			}
			if c := cmp.Compare(b.ItemsPerHour, a.ItemsPerHour); c != 0 {
				return c // fastest first
			}
			if c := cmp.Compare(a.OwnerName, b.OwnerName); c != 0 {
				return c
			}
			if c := cmp.Compare(a.StationID, b.StationID); c != 0 {
				return c
			}
			return cmp.Compare(a.FacilityID, b.FacilityID)
		})
		// Every line here runs this recipe, so the section is one comparison set.
		ptrs := make([]*PublicFacility, len(lines))
		for i := range lines {
			ptrs[i] = &lines[i]
		}
		markPriceExtremes(ptrs)
		groups = append(groups, WhereRecipeGroup{
			RecipeID:       r.ID,
			RecipeName:     r.Name,
			RecipeCategory: r.Category,
			RecipeDirName:  dirName(r.Category),
			FacilityOnly:   r.FacilityOnly,
			Outputs:        r.Outputs,
			Facilities:     lines,
		})
	}
	slices.SortFunc(groups, func(a, b WhereRecipeGroup) int {
		if c := cmp.Compare(a.RecipeName, b.RecipeName); c != 0 {
			return c
		}
		return cmp.Compare(a.RecipeID, b.RecipeID)
	})
	return groups
}

// groupByStation buckets public lines by station (count descending, then name)
// and, within each station, by recipe category (name ascending). The category
// grouping is what keeps a 219-line station scannable.
func groupByStation(facs []PublicFacility, recipes map[string]*Recipe) []WhereStationGroup {
	type stationAcc struct {
		g     WhereStationGroup
		byCat map[string][]WhereStationFacility
	}
	acc := make(map[string]*stationAcc)

	for _, f := range facs {
		r, ok := recipes[f.RecipeID]
		if !ok {
			continue
		}
		a, ok := acc[f.StationID]
		if !ok {
			a = &stationAcc{
				g: WhereStationGroup{
					StationID:   f.StationID,
					StationName: f.StationName,
					SystemID:    f.SystemID,
					SystemName:  f.SystemName,
					FeeMin:      f.FeePerRun,
					FeeMax:      f.FeePerRun,
				},
				byCat: make(map[string][]WhereStationFacility),
			}
			acc[f.StationID] = a
		}
		a.g.Count++
		a.g.FeeMin = min(a.g.FeeMin, f.FeePerRun)
		a.g.FeeMax = max(a.g.FeeMax, f.FeePerRun)
		a.byCat[r.Category] = append(a.byCat[r.Category], WhereStationFacility{
			PublicFacility: f,
			RecipeName:     r.Name,
			RecipeDirName:  dirName(r.Category),
			Outputs:        r.Outputs,
		})
	}

	stations := make([]WhereStationGroup, 0, len(acc))
	for _, a := range acc {
		cats := make([]WhereStationCategory, 0, len(a.byCat))
		for name, lines := range a.byCat {
			slices.SortFunc(lines, func(x, y WhereStationFacility) int {
				if c := cmp.Compare(x.RecipeName, y.RecipeName); c != 0 {
					return c
				}
				return cmp.Compare(x.FacilityID, y.FacilityID)
			})
			// A block lists many recipes, so each recipe is its own comparison
			// set -- a station often runs the same recipe on several lines at
			// different fees, and those are what a reader is choosing between.
			byRecipe := make(map[string][]*PublicFacility)
			for i := range lines {
				id := lines[i].RecipeID
				byRecipe[id] = append(byRecipe[id], &lines[i].PublicFacility)
			}
			for _, set := range byRecipe {
				markPriceExtremes(set)
			}
			cats = append(cats, WhereStationCategory{Category: name, Facilities: lines})
		}
		slices.SortFunc(cats, func(x, y WhereStationCategory) int {
			return cmp.Compare(x.Category, y.Category)
		})
		a.g.Categories = cats
		stations = append(stations, a.g)
	}

	slices.SortFunc(stations, func(a, b WhereStationGroup) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 { // count descending
			return c
		}
		if c := cmp.Compare(a.StationName, b.StationName); c != 0 {
			return c
		}
		return cmp.Compare(a.StationID, b.StationID)
	})
	return stations
}

// splitNoFacilityRecipes partitions the recipes with no known public line into
// the two dense tables.
//
// The split is on facility_only, and the distinction is the whole point: a
// facility_only recipe with no public line genuinely cannot be crafted at a
// bare station, while a non-facility_only one can be crafted anywhere, so the
// absence of a public line barely matters. Recipes that DO have a public line
// appear in neither table.
func splitNoFacilityRecipes(recipes map[string]*Recipe, covered map[string]bool) (facilityOnly, noFacilityNeeded []NoFacilityRecipe) {
	for id, r := range recipes {
		if covered[id] {
			continue
		}
		e := NoFacilityRecipe{
			ID:           r.ID,
			Name:         r.Name,
			Category:     r.Category,
			DirName:      dirName(r.Category),
			CraftingTime: r.CraftingTime,
		}
		if len(r.Outputs) > 0 {
			o := r.Outputs[0]
			e.OutputID, e.OutputName, e.OutputCategory, e.OutputQty =
				o.ItemID, o.ItemName, o.ItemCategory, o.Quantity
		}
		if r.FacilityOnly {
			facilityOnly = append(facilityOnly, e)
		} else {
			noFacilityNeeded = append(noFacilityNeeded, e)
		}
	}

	byName := func(a, b NoFacilityRecipe) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	}
	slices.SortFunc(facilityOnly, byName)
	slices.SortFunc(noFacilityNeeded, byName)
	return facilityOnly, noFacilityNeeded
}

// wherePageData is the root template context for where.html.
type wherePageData struct {
	StationCount     int
	FacilityCount    int
	RecipesCovered   int
	FacilityOnlyGap  int
	LastSeenTick     int
	Owners           []WhereOwnerSummary
	RecipeGroups     []WhereRecipeGroup
	StationGroups    []WhereStationGroup
	FacilityOnlyNone []NoFacilityRecipe
	NoFacilityNeeded []NoFacilityRecipe
	LastUpdated      string
}

// writeWherePage renders kb/recipes/where.html.
//
// MUST be called after writeRecipePages: that function calls
// cleanGeneratedFiles on the same directory, which deletes every .html in it.
func writeWherePage(outDir string, knowledgeDB *sql.DB, recipes map[string]*Recipe) error {
	facs, err := loadPublicFacilities(knowledgeDB)
	if err != nil {
		return fmt.Errorf("load public facilities: %w", err)
	}

	covered := make(map[string]bool, len(facs))
	maxTick := 0
	for _, f := range facs {
		covered[f.RecipeID] = true
		maxTick = max(maxTick, f.LastSeenTick)
	}

	recipeGroups := groupByRecipe(facs, recipes)
	stationGroups := groupByStation(facs, recipes)
	facilityOnlyNone, noFacilityNeeded := splitNoFacilityRecipes(recipes, covered)

	data := wherePageData{
		StationCount:     len(stationGroups),
		FacilityCount:    len(facs),
		RecipesCovered:   len(recipeGroups),
		FacilityOnlyGap:  len(facilityOnlyNone),
		LastSeenTick:     maxTick,
		Owners:           summarizeOwners(facs),
		RecipeGroups:     recipeGroups,
		StationGroups:    stationGroups,
		FacilityOnlyNone: facilityOnlyNone,
		NoFacilityNeeded: noFacilityNeeded,
		LastUpdated:      lastMarketUpdate,
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	outPath := filepath.Join(outDir, "where.html")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if err := renderWherePage(f, data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	log.Printf("Where-To-Craft: %d public lines, %d recipes covered, %d stations, %d facility-only gaps",
		len(facs), len(recipeGroups), len(stationGroups), len(facilityOnlyNone))
	return nil
}

// renderWherePage writes the page HTML for data. It is separate from
// writeWherePage so the template can be exercised against hand-built rows —
// notably station-owned facilities, which do not yet exist in the knowledge DB.
func renderWherePage(w io.Writer, data wherePageData) error {
	funcs := htmltpl.FuncMap{
		"comma": func(n int) string { return humanize.Comma(int64(n)) },
		"lower": strings.ToLower, // faction dirs are the lowercased tag: kb/factions/hexc/
		"fmtTime": func(f float64) string {
			if f == 0 {
				return "-"
			}
			return fmt.Sprintf("%.1fs", f)
		},
		"itemURL": func(category, id string) string {
			if category == "" {
				return ""
			}
			return fmt.Sprintf("../items/%s/%s.html", category, id)
		},
		"shortHash": func(s string) string {
			if len(s) > 8 {
				return s[:8]
			}
			return s
		},
	}

	tmpl, err := htmltpl.New("where").Funcs(funcs).Parse(whereTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, data)
}

// whereTabScript selects the tab from the URL hash: #s-<station> opens the
// by-station tab, anything else opens by-recipe, so external deep links land
// on the right view and scroll to their anchor.
var whereTabScript = `    <script>
    (function() {
      var buttons = document.querySelectorAll(".tab-btn");
      function show(id) {
        document.querySelectorAll(".tab-panel").forEach(function(p) { p.hidden = (p.id !== id); });
        buttons.forEach(function(b) { b.classList.toggle("active", b.dataset.tab === id); });
      }
      var hash = location.hash.slice(1);
      var initial = (hash === "by-station" || hash.indexOf("s-") === 0) ? "by-station" : "by-recipe";
      show(initial);
      if (hash && hash !== "by-recipe" && hash !== "by-station") {
        var el = document.getElementById(hash);
        if (el) el.scrollIntoView();
      }
      buttons.forEach(function(b) {
        b.addEventListener("click", function() {
          show(b.dataset.tab);
          history.replaceState(null, "", "#" + b.dataset.tab);
        });
      });
    })();
    </script>`

// denseTableTemplate renders one of the two dense gap tables (facility-only
// with no public line, and no-facility-required). The brief's original
// template repeated this markup twice, differing only in the range variable;
// it is defined once here and invoked twice from whereTemplate so the two
// tables can never drift out of sync with each other.
var denseTableTemplate = `{{define "denseTable"}}
            <table class="dense sortable">
                <thead>
                    <tr>
                        <th class="sortable">Recipe</th>
                        <th class="sortable">Category</th>
                        <th class="sortable">Output</th>
                        <th class="sortable">Qty</th>
                        <th class="sortable">Craft Time</th>
                    </tr>
                </thead>
                <tbody>
{{- range .}}
                    <tr>
                        <td><a href="{{.DirName}}/{{.ID}}.html">{{.Name}}</a></td>
                        <td><a href="{{.DirName}}/">{{.Category}}</a></td>
                        <td>{{if .OutputCategory}}<a href="{{itemURL .OutputCategory .OutputID}}">{{.OutputName}}</a>{{else}}{{.OutputName}}{{end}}</td>
                        <td class="num-cell" data-sort="{{.OutputQty}}">{{.OutputQty}}</td>
                        <td class="num-cell" data-sort="{{.CraftingTime}}">{{fmtTime .CraftingTime}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
{{end}}`

var whereTemplate = denseTableTemplate + `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Where Can I Make This - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
    <style>
        .owners { margin: 20px 0 8px; border: 1px solid var(--border); border-radius: 6px; padding: 10px 14px; background: var(--bg-card); }
        .owners > summary { cursor: pointer; font-weight: 600; }
        .owners table { margin-top: 10px; }
        .tabs { display: flex; gap: 4px; margin: 20px 0 8px; border-bottom: 2px solid var(--border); }
        .tab-btn { background: none; border: none; border-bottom: 2px solid transparent; margin-bottom: -2px;
                   padding: 10px 18px; font-size: 1em; cursor: pointer; color: var(--text-muted); }
        .tab-btn:hover { color: var(--link); }
        .tab-btn.active { color: var(--link); border-bottom-color: var(--link); font-weight: 600; }
        .summary-cards { display: flex; gap: 16px; margin: 16px 0; flex-wrap: wrap; }
        .summary-card { background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; padding: 12px 20px; text-align: center; }
        .summary-card .num { font-size: 1.8em; font-weight: 700; }
        .summary-card .label { font-size: 0.8em; color: var(--text-muted); text-transform: uppercase; }
        .freshness { font-size: 0.85em; color: var(--text-muted); margin-bottom: 8px; }
        .toc { columns: 3; column-gap: 24px; margin: 16px 0 32px; }
        .toc a { display: block; padding: 2px 0; color: var(--link); text-decoration: none; font-size: 0.95em; }
        .toc a:hover { text-decoration: underline; }
        .where-section { margin-top: 32px; scroll-margin-top: 16px; }
        .where-section h3 { margin-bottom: 8px; border-bottom: 1px solid var(--border); padding-bottom: 4px; }
        .where-section table { width: 100%; font-size: 0.9em; }
        .where-section th { text-align: left; cursor: pointer; user-select: none; white-space: nowrap; }
        .where-section th:hover { color: var(--link); }
        .where-section td { padding: 4px 8px; }
        .where-section tr:hover { background: var(--bg-hover, rgba(128,128,128,0.08)); }
        .cat-block { margin: 16px 0 24px; }
        .cat-block h4 { margin: 0 0 4px; font-size: 0.95em; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.04em; }
        .dense { font-size: 0.85em; width: 100%; }
        .dense td, .dense th { padding: 2px 8px; }
        .callout { border: 1px solid var(--border); border-left: 4px solid #999; padding: 12px 16px; margin: 24px 0 8px; border-radius: 4px; background: var(--bg-card); }
        .callout.warn { border-left-color: #d08040; }
        .callout h3 { margin: 0 0 4px; }
        .callout p { margin: 0; color: var(--text-muted); font-size: 0.9em; }
        .back-top { font-size: 0.8em; margin-left: 8px; color: var(--text-muted); }
        .num-cell { text-align: right; font-variant-numeric: tabular-nums; }
        .fee-best  { color: hsl(var(--smui-green)); font-weight: 600; }
        .fee-worst { color: hsl(var(--smui-red));   font-weight: 600; }
        @media (max-width: 768px) { .toc { columns: 2; } }
        @media (max-width: 480px) { .toc { columns: 1; } }
    </style>
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Where Can I Make This</h2>
        <p>Public production facilities across the galaxy: which stations rent a line for a given recipe, and what each station can produce.</p>

        <div class="summary-cards">
            <div class="summary-card"><div class="num">{{.StationCount}}</div><div class="label">Stations With Public Lines</div></div>
            <div class="summary-card"><div class="num">{{.FacilityCount}}</div><div class="label">Public Facilities</div></div>
            <div class="summary-card"><div class="num">{{.RecipesCovered}}</div><div class="label">Recipes Covered</div></div>
            <div class="summary-card"><div class="num">{{.FacilityOnlyGap}}</div><div class="label">Facility-Only, No Public Line</div></div>
        </div>
        <p class="freshness">Facility data as of tick {{comma .LastSeenTick}}. Station survey bots report roughly hourly. Private and faction-owned facilities are not listed here.</p>

        <details class="owners" open>
            <summary>Facility Owners ({{len .Owners}})</summary>
            <table class="dense sortable">
                <thead>
                    <tr>
                        <th class="sortable">Source</th>
                        <th class="sortable">Facilities</th>
                        <th class="sortable">Stations</th>
                        <th class="sortable">Recipes</th>
                        <th class="sortable">Fee/run range</th>
                    </tr>
                </thead>
                <tbody>
{{- range .Owners}}
                    <tr>
                        <td>{{if .OwnerName}}<a href="../factions/{{lower .OwnerTag}}/index.html">{{.OwnerName}}</a>{{else if .OwnerID}}<code title="{{.OwnerID}}">{{shortHash .OwnerID}}</code>{{else}}<span class="badge">Station Facility</span>{{end}}</td>
                        <td class="num-cell" data-sort="{{.Facilities}}">{{comma .Facilities}}</td>
                        <td class="num-cell" data-sort="{{.Stations}}">{{comma .Stations}}</td>
                        <td class="num-cell" data-sort="{{.Recipes}}">{{comma .Recipes}}</td>
                        <td class="num-cell" data-sort="{{.FeeMin}}">{{if eq .FeeMin .FeeMax}}{{comma .FeeMin}}{{else}}{{comma .FeeMin}}&ndash;{{comma .FeeMax}}{{end}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
        </details>

        <div class="tabs">
            <button class="tab-btn" data-tab="by-recipe">By Recipe</button>
            <button class="tab-btn" data-tab="by-station">By Station</button>
        </div>

        <section class="tab-panel" id="by-recipe">
            <div class="card" style="padding: 12px 16px">
                <div class="section-label">Jump To Recipe</div>
                <div class="toc">
{{- range .RecipeGroups}}
                    <a href="#r-{{.RecipeID}}">{{.RecipeName}} ({{len .Facilities}})</a>
{{- end}}
                </div>
            </div>

{{- range .RecipeGroups}}
            <div id="r-{{.RecipeID}}" class="where-section">
                <h3>
                    <a href="{{.RecipeDirName}}/{{.RecipeID}}.html">{{.RecipeName}}</a>
                    <span class="badge" style="font-size:0.7em; vertical-align:middle;">{{len .Facilities}} station{{if ne (len .Facilities) 1}}s{{end}}</span>
{{- if .FacilityOnly}}
                    <span class="badge badge-frost" style="font-size:0.7em; vertical-align:middle;" title="Requires a production facility">Facility Only</span>
{{- end}}
{{- range .Outputs}}
                    <small style="font-size:0.75em; font-weight:normal;">&rarr; {{if .ItemCategory}}<a href="{{itemURL .ItemCategory .ItemID}}">{{.ItemName}}</a>{{else}}{{.ItemName}}{{end}} &times;{{.Quantity}}</small>
{{- end}}
                    <a href="#" class="back-top">[top]</a>
                </h3>
                <table class="sortable">
                    <thead>
                        <tr>
                            <th class="sortable">Station</th>
                            <th class="sortable">System</th>
                            <th class="sortable">Facility</th>
                            <th class="sortable">Level</th>
                            <th class="sortable">Fee/run</th>
                            <th class="sortable">Qty/run</th>
                            <th class="sortable">Items/hr</th>
                            <th class="sortable">Source</th>
                        </tr>
                    </thead>
                    <tbody>
{{- range .Facilities}}
                        <tr>
                            <td><a href="#s-{{.StationID}}">{{.StationName}}</a></td>
                            <td>{{if .SystemID}}<a href="../systems/{{.SystemID}}/index.html">{{.SystemName}}</a>{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                            <td>{{if .FacilityType}}<a href="../facilities/production/{{.FacilityType}}.html">{{.FacilityName}}</a>{{else}}{{.FacilityName}}{{end}}</td>
                            <td class="num-cell" data-sort="{{.Level}}">{{.Level}}</td>
                            <td class="num-cell{{if .PriceRank}} fee-{{.PriceRank}}{{end}}" data-sort="{{.FeePerRun}}"{{if .PriceRank}} title="{{if eq .PriceRank "best"}}Cheapest{{else}}Costliest{{end}} per unit of output among the lines running this recipe here"{{end}}>{{comma .FeePerRun}}</td>
                            <td class="num-cell" data-sort="{{.QtyPerRun}}">{{if .QtyPerRun}}{{.QtyPerRun}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                            <td class="num-cell" data-sort="{{.ItemsPerHour}}">{{if .ItemsPerHour}}{{comma .ItemsPerHour}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                            <td>{{if .OwnerName}}<a href="../factions/{{lower .OwnerTag}}/index.html">{{.OwnerName}}</a>{{else if .OwnerID}}<code title="{{.OwnerID}}">{{shortHash .OwnerID}}</code>{{else}}<span class="badge">Station Facility</span>{{end}}</td>
                        </tr>
{{- end}}
                    </tbody>
                </table>
            </div>
{{- end}}

            <div class="callout warn">
                <h3>Facility-Only &mdash; No Known Public Line ({{len .FacilityOnlyNone}})</h3>
                <p>These recipes require a production facility, and no public line for them has been surveyed. They cannot be crafted at a bare station. A private or faction-owned facility may still run them &mdash; those never appear in this data.</p>
            </div>
{{template "denseTable" .FacilityOnlyNone}}

            <div class="callout">
                <h3>No Facility Required ({{len .NoFacilityNeeded}})</h3>
                <p>No public line has been surveyed for these, but none is needed &mdash; they can be crafted at any station. A public facility would only add throughput.</p>
            </div>
{{template "denseTable" .NoFacilityNeeded}}
        </section>

        <section class="tab-panel" id="by-station" hidden>
            <div class="card" style="padding: 12px 16px">
                <div class="section-label">Jump To Station</div>
                <div class="toc">
{{- range .StationGroups}}
                    <a href="#s-{{.StationID}}">{{.StationName}} ({{.Count}})</a>
{{- end}}
                </div>
            </div>

{{- range .StationGroups}}
            <div id="s-{{.StationID}}" class="where-section">
                <h3>
                    {{.StationName}}
                    <span class="badge" style="font-size:0.7em; vertical-align:middle;">{{.Count}} facilit{{if eq .Count 1}}y{{else}}ies{{end}}</span>
                    {{if .SystemID}}<small style="font-size:0.75em; font-weight:normal;">in <a href="../systems/{{.SystemID}}/index.html">{{.SystemName}}</a></small>{{end}}
                    <small style="font-size:0.75em; font-weight:normal;" class="text-muted">fees {{comma .FeeMin}}&ndash;{{comma .FeeMax}}/run</small>
                    <a href="#" class="back-top">[top]</a>
                </h3>
{{- range .Categories}}
                <div class="cat-block">
                    <h4>{{.Category}}</h4>
                    <table class="sortable">
                        <thead>
                            <tr>
                                <th class="sortable">Recipe</th>
                                <th class="sortable">Output</th>
                                <th class="sortable">Facility</th>
                                <th class="sortable">Level</th>
                                <th class="sortable">Fee/run</th>
                                <th class="sortable">Qty/run</th>
                                <th class="sortable">Items/hr</th>
                                <th class="sortable">Source</th>
                            </tr>
                        </thead>
                        <tbody>
{{- range .Facilities}}
                            <tr>
                                <td><a href="{{.RecipeDirName}}/{{.RecipeID}}.html">{{.RecipeName}}</a></td>
                                <td>{{range .Outputs}}{{if .ItemCategory}}<a href="{{itemURL .ItemCategory .ItemID}}">{{.ItemName}}</a>{{else}}{{.ItemName}}{{end}} &times;{{.Quantity}} {{end}}</td>
                                <td>{{if .FacilityType}}<a href="../facilities/production/{{.FacilityType}}.html">{{.FacilityName}}</a>{{else}}{{.FacilityName}}{{end}}</td>
                                <td class="num-cell" data-sort="{{.Level}}">{{.Level}}</td>
                                <td class="num-cell{{if .PriceRank}} fee-{{.PriceRank}}{{end}}" data-sort="{{.FeePerRun}}"{{if .PriceRank}} title="{{if eq .PriceRank "best"}}Cheapest{{else}}Costliest{{end}} per unit of output among the lines running this recipe here"{{end}}>{{comma .FeePerRun}}</td>
                                <td class="num-cell" data-sort="{{.QtyPerRun}}">{{if .QtyPerRun}}{{.QtyPerRun}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                                <td class="num-cell" data-sort="{{.ItemsPerHour}}">{{if .ItemsPerHour}}{{comma .ItemsPerHour}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                                <td>{{if .OwnerName}}<a href="../factions/{{lower .OwnerTag}}/index.html">{{.OwnerName}}</a>{{else if .OwnerID}}<code title="{{.OwnerID}}">{{shortHash .OwnerID}}</code>{{else}}<span class="badge">Station Facility</span>{{end}}</td>
                            </tr>
{{- end}}
                        </tbody>
                    </table>
                </div>
{{- end}}
            </div>
{{- end}}
        </section>
    </main>
` + sortScript + `
` + whereTabScript + `
` + themeScript + `
{{if .LastUpdated}}<footer style="margin-top:2rem;padding-top:1rem;border-top:1px solid #333;color:#888;font-size:0.85rem;text-align:center">Market data last updated: {{.LastUpdated}}</footer>{{end}}
</body>
</html>
`
