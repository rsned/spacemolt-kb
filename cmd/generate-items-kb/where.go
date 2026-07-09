package main

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
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
// name; lines within a group sort by station name. Facilities whose recipe is
// absent from the crafting DB are dropped -- every ID resolves today, and a
// group with no name or category would render as a dead link.
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
			return cmp.Compare(a.FacilityID, b.FacilityID)
		})
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
