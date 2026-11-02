package prodcost

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// LoadWorld reads the catalogs under catalogRoot and the captured production
// lines from kdb. A non-empty ourTag also loads that faction's own production
// facilities as KindCRFT lines; pass "" for the public build.
func LoadWorld(catalogRoot string, kdb *sql.DB, ourTag string) (World, error) {
	items, err := catalog.LoadItems(catalogRoot)
	if err != nil {
		return World{}, fmt.Errorf("load items: %w", err)
	}
	recipes, err := catalog.LoadRecipes(catalogRoot)
	if err != nil {
		return World{}, fmt.Errorf("load recipes: %w", err)
	}
	facs, err := catalog.LoadFacilities(catalogRoot)
	if err != nil {
		return World{}, fmt.Errorf("load facilities: %w", err)
	}
	w := World{
		Items:      make(map[string]catalog.Item, len(items)),
		Recipes:    recipes,
		Facilities: make(map[string]catalog.Facility, len(facs)),
	}
	for _, it := range items {
		w.Items[it.ID] = it
	}
	for _, f := range facs {
		w.Facilities[f.ID] = f
	}

	var ourID string
	if ourTag != "" {
		err := kdb.QueryRow(`SELECT faction_id FROM factions WHERE tag = ? LIMIT 1`, ourTag).Scan(&ourID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return World{}, fmt.Errorf("find faction %q: %w", ourTag, err)
		}
	}

	pub, err := loadPublicLines(kdb, ourID)
	if err != nil {
		return World{}, err
	}
	w.Lines = append(w.Lines, pub...)
	if ourID != "" {
		own, err := loadCRFTLines(kdb, ourID, ourTag)
		if err != nil {
			return World{}, err
		}
		w.Lines = append(w.Lines, own...)
	}
	return w, nil
}

func tagOrPrefix(tag, id string) string {
	if tag != "" {
		return tag
	}
	return id[:min(6, len(id))]
}

// loadPublicLines reads public_facilities. Lines owned by ourID are skipped
// (they are loaded as crft lines) when ourID is set.
func loadPublicLines(kdb *sql.DB, ourID string) ([]Line, error) {
	rows, err := kdb.Query(`
SELECT pf.station_id, COALESCE(b.name, ''), pf.recipe_id, COALESCE(pf.level, 1),
       COALESCE(pf.rental_fee_per_run, 0), COALESCE(pf.owner_faction, ''),
       COALESCE(f.tag, ''),
       CASE WHEN json_valid(pf.details_json) THEN COALESCE(json_extract(pf.details_json, '$.production.ticks_per_run'), 0) ELSE 0 END
FROM public_facilities pf
LEFT JOIN bases b ON b.id = pf.station_id
LEFT JOIN factions f ON f.faction_id = pf.owner_faction
WHERE pf.recipe_id <> ''
ORDER BY pf.station_id, pf.facility_id`)
	if err != nil {
		return nil, fmt.Errorf("query public_facilities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Line
	for rows.Next() {
		var l Line
		var name, tag string
		if err := rows.Scan(&l.StationID, &name, &l.RecipeID, &l.Level, &l.Fee,
			&l.FactionID, &tag, &l.TicksPerRun); err != nil {
			return nil, fmt.Errorf("scan public_facilities: %w", err)
		}
		if ourID != "" && l.FactionID == ourID {
			continue
		}
		l.StationName = name
		if name == "" {
			l.StationName = l.StationID
		}
		l.Empire = l.FactionID == ""
		if !l.Empire {
			l.Kind = KindFaction
			l.FactionTag = tagOrPrefix(tag, l.FactionID)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// loadCRFTLines reads the production facilities of faction ourID.
func loadCRFTLines(kdb *sql.DB, ourID, ourTag string) ([]Line, error) {
	rows, err := kdb.Query(`
SELECT ff.base_id, COALESCE(b.name, ''), ff.recipe_id, ff.level
FROM faction_facilities ff
LEFT JOIN bases b ON b.id = ff.base_id
WHERE ff.faction_id = ? AND ff.category = 'production' AND COALESCE(ff.recipe_id, '') <> ''
ORDER BY ff.base_id, ff.facility_id`, ourID)
	if err != nil {
		return nil, fmt.Errorf("query faction_facilities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Line
	for rows.Next() {
		l := Line{FactionID: ourID, FactionTag: ourTag, Kind: KindCRFT}
		var name string
		if err := rows.Scan(&l.StationID, &name, &l.RecipeID, &l.Level); err != nil {
			return nil, fmt.Errorf("scan faction_facilities: %w", err)
		}
		l.StationName = name
		if name == "" {
			l.StationName = l.StationID
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
