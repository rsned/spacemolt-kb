package prodcost

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// listingWindow is how far back ship listings count as current.
const listingWindow = 72 * time.Hour

func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// LoadHullData reads shipyard quotes, CRFT licenses and recent ship listing
// minimums. Missing shipyard tables yield empty data, not an error.
func LoadHullData(kdb *sql.DB, ourTag string, tierOf map[string]int, now time.Time) (HullData, error) {
	hd := HullData{Licensed: map[string]bool{}, Listings: map[string]ListingMin{}}

	var ourID string
	if ourTag != "" {
		err := kdb.QueryRow(`SELECT faction_id FROM factions WHERE tag = ? LIMIT 1`, ourTag).Scan(&ourID)
		if errors.Is(err, sql.ErrNoRows) {
			return HullData{}, fmt.Errorf("find faction %q: no such tag in factions", ourTag)
		}
		if err != nil {
			return HullData{}, fmt.Errorf("find faction %q: %w", ourTag, err)
		}
	}

	if err := loadQuotes(kdb, tierOf, &hd); err != nil {
		return HullData{}, err
	}
	if ourID != "" {
		if err := loadLicenses(kdb, ourID, &hd); err != nil {
			return HullData{}, err
		}
	}
	if err := loadListingMins(kdb, now, &hd); err != nil {
		return HullData{}, err
	}
	return hd, nil
}

func loadQuotes(kdb *sql.DB, tierOf map[string]int, hd *HullData) error {
	rows, err := kdb.Query(`SELECT ship_class, station_id, mode, labor, royalty, tick FROM shipyard_quotes ORDER BY id`)
	if isNoSuchTable(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query shipyard_quotes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var q Quote
		var roy sql.NullInt64
		if err := rows.Scan(&q.ShipClass, &q.StationID, &q.Mode, &q.Labor, &roy, &q.Tick); err != nil {
			return fmt.Errorf("scan shipyard_quotes: %w", err)
		}
		if roy.Valid {
			v := int(roy.Int64)
			q.Royalty = &v
		}
		q.Tier = tierOf[q.ShipClass]
		hd.Quotes = append(hd.Quotes, q)
	}
	return rows.Err()
}

func loadLicenses(kdb *sql.DB, ourID string, hd *HullData) error {
	rows, err := kdb.Query(`SELECT ship_class FROM ship_licenses WHERE faction_id = ?`, ourID)
	if isNoSuchTable(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query ship_licenses: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return fmt.Errorf("scan ship_licenses: %w", err)
		}
		hd.Licensed[c] = true
	}
	return rows.Err()
}

func loadListingMins(kdb *sql.DB, now time.Time, hd *HullData) error {
	cutoff := now.UTC().Add(-listingWindow).Format(time.RFC3339)
	rows, err := kdb.Query(`
SELECT class_id, CAST(price AS INTEGER), COALESCE(station_id, '')
FROM ship_listings
WHERE captured_at > ? AND class_id IS NOT NULL AND price IS NOT NULL
ORDER BY class_id, price, station_id`, cutoff)
	if isNoSuchTable(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query ship_listings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c, st string
		var p int
		if err := rows.Scan(&c, &p, &st); err != nil {
			return fmt.Errorf("scan ship_listings: %w", err)
		}
		m, ok := hd.Listings[c]
		if !ok {
			m = ListingMin{Price: p, StationID: st}
		}
		m.Count++
		hd.Listings[c] = m
	}
	return rows.Err()
}
