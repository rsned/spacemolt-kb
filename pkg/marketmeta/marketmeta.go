// Package marketmeta provides shared freshness metadata for market-derived KB
// pages, so every page's "last updated" footer reports the same instant.
package marketmeta

import (
	"database/sql"
	"time"
)

// LatestCapture returns a human-readable UTC timestamp of the most recent
// order-book capture in the market DB (MAX(captured_at) over market_orders),
// formatted "2006-01-02 15:04 UTC". Returns "" when the market has no orders.
func LatestCapture(db *sql.DB) (string, error) {
	var raw sql.NullString
	if err := db.QueryRow(`SELECT MAX(captured_at) FROM market_orders`).Scan(&raw); err != nil {
		return "", err
	}
	if !raw.Valid || raw.String == "" {
		return "", nil
	}
	if ts, err := time.Parse(time.RFC3339, raw.String); err == nil {
		return ts.UTC().Format("2006-01-02 15:04 UTC"), nil
	}
	return raw.String, nil
}
