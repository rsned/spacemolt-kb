package main

import (
	"database/sql"
	"encoding/json"
	"errors"
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
