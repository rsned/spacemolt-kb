package main

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// empireColors maps lowercase empire (citizenship) names to their theme color.
// Values mirror cmd/generate-items-kb/main.go:351.
var empireColors = map[string]string{
	"solarian": "#FFD700",
	"voidborn": "#9932CC",
	"crimson":  "#DC143C",
	"nebula":   "#00CED1",
	"outerrim": "#2E8B57",
}

// empireColor returns the theme color for a citizenship, or "" when unknown.
func empireColor(citizenship string) string {
	return empireColors[strings.ToLower(strings.TrimSpace(citizenship))]
}

// loadPassengers loads all rows of the passengers table, sorted by name
// (case-insensitive, ID as tiebreaker). A missing table surfaces as a query error.
func loadPassengers(db *sql.DB) ([]*Passenger, error) {
	rows, err := db.Query(`SELECT citizen_id, name, citizenship, bio, class,
	                              first_seen_utc, last_seen_utc, sighting_count
	                       FROM passengers`)
	if err != nil {
		return nil, fmt.Errorf("query passengers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*Passenger
	for rows.Next() {
		var id, name string
		var citizenship, bio, class, first, last sql.NullString
		var count sql.NullInt64
		if err := rows.Scan(&id, &name, &citizenship, &bio, &class, &first, &last, &count); err != nil {
			return nil, fmt.Errorf("scan passenger: %w", err)
		}
		out = append(out, &Passenger{
			ID:            id,
			Slug:          id,
			Name:          name,
			Citizenship:   citizenship.String,
			EmpireColor:   empireColor(citizenship.String),
			Bio:           bio.String,
			Class:         class.String,
			FirstSeenUTC:  first.String,
			LastSeenUTC:   last.String,
			SightingCount: int(count.Int64),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate passengers: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if li != lj {
			return li < lj
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
