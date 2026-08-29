package resourcediff

import (
	"database/sql"
	"fmt"
	"math"
)

// FromDB builds a snapshot from the knowledge database using the same joins
// as the Resources page generator, rounding values the way the page prints
// them. The caller sets Date and ServerVersion.
func FromDB(db *sql.DB) (*Snapshot, error) {
	snap := &Snapshot{Source: SourceDB}

	rows, err := db.Query(`
		SELECT s.id, s.name, p.id, p.name, p.hidden,
		       EXISTS(SELECT 1 FROM pois sp WHERE sp.system_id = s.id AND sp.type = 'station'),
		       pr.resource_id, pr.richness, pr.remaining, pr.max_remaining, pr.last_updated_tick
		FROM poi_resources pr
		JOIN pois p ON pr.poi_id = p.id
		JOIN systems s ON p.system_id = s.id`)
	if err != nil {
		return nil, fmt.Errorf("query deposits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	seen := make(map[string]bool)
	for rows.Next() {
		var d Deposit
		var richness, remaining, maxRemaining float64
		if err := rows.Scan(&d.SystemID, &d.SystemName, &d.POIID, &d.POIName, &d.Hidden, &d.Station,
			&d.ResourceID, &richness, &remaining, &maxRemaining, &d.LastTick); err != nil {
			return nil, err
		}
		d.Richness = int(math.Round(richness))
		d.Remaining = int(remaining)
		d.MaxRemaining = int(maxRemaining)
		snap.Deposits = append(snap.Deposits, d)
		seen[d.ResourceID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Every ore/material in the catalog is a listed type, discovered or not,
	// plus anything a deposit references that the catalog lacks (the page
	// shows those under their raw ID).
	trows, err := db.Query(`SELECT id, COALESCE(name, id), COALESCE(category, '') FROM items WHERE category IN ('ore', 'material')`)
	if err != nil {
		return nil, fmt.Errorf("query resource items: %w", err)
	}
	defer func() { _ = trows.Close() }()
	declared := make(map[string]bool)
	for trows.Next() {
		var t ResourceType
		if err := trows.Scan(&t.ID, &t.Name, &t.Category); err != nil {
			return nil, err
		}
		declared[t.ID] = true
		snap.Types = append(snap.Types, t)
	}
	if err := trows.Err(); err != nil {
		return nil, err
	}
	for id := range seen {
		if declared[id] {
			continue
		}
		t := ResourceType{ID: id, Name: id}
		// A deposit resource outside ore/material still resolves its name
		// and category from the items table when present.
		_ = db.QueryRow(`SELECT COALESCE(name, id), COALESCE(category, '') FROM items WHERE id = ?`, id).Scan(&t.Name, &t.Category)
		snap.Types = append(snap.Types, t)
	}

	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(last_updated_tick > 0), 0) FROM systems`).
		Scan(&snap.Summary.Systems, &snap.Summary.Explored); err != nil {
		return nil, fmt.Errorf("query systems: %w", err)
	}
	snap.Summary.Types = len(snap.Types)
	snap.Summary.Deposits = len(snap.Deposits)
	snap.normalize()
	return snap, nil
}
