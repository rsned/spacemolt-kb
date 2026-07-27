package main

import (
	"cmp"
	"database/sql"
	"slices"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// loadGalaxy reads every system, its jump-gate connections, and the
// stronghold set from the knowledge database.
//
// It returns the systems in name order, the undirected edge list, and the
// IDs of systems flagged is_stronghold.
func loadGalaxy(db *sql.DB) ([]*galaxymap.System, []Edge, []string, error) {
	rows, err := db.Query(`SELECT id, name, position_x, position_y, police_level,
		COALESCE(empire,''), is_stronghold, last_updated_tick
		FROM systems ORDER BY name`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	byID := make(map[string]*galaxymap.System)
	var systems []*galaxymap.System
	var sources []string
	for rows.Next() {
		var s galaxymap.System
		if err := rows.Scan(&s.ID, &s.Name, &s.PositionX, &s.PositionY,
			&s.PoliceLevel, &s.Empire, &s.IsStronghold, &s.LastUpdatedTick); err != nil {
			return nil, nil, nil, err
		}
		if s.ID == "" {
			continue
		}
		byID[s.ID] = &s
		systems = append(systems, &s)
		if s.IsStronghold {
			sources = append(sources, s.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}

	connRows, err := db.Query(`SELECT from_system, to_system, distance
		FROM connections ORDER BY from_system, to_system`)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = connRows.Close() }()

	var edges []Edge
	seen := make(map[string]bool)
	for connRows.Next() {
		var fromID, toID string
		var distance int
		if err := connRows.Scan(&fromID, &toID, &distance); err != nil {
			return nil, nil, nil, err
		}
		from, okFrom := byID[fromID]
		to, okTo := byID[toID]
		if !okFrom || !okTo {
			continue
		}
		from.Connections = append(from.Connections, galaxymap.Connection{
			SystemID: toID, Name: to.Name, Distance: distance,
		})
		// Collapse the directed rows into one undirected edge.
		key := fromID + "|" + toID
		rev := toID + "|" + fromID
		if !seen[key] && !seen[rev] {
			seen[key] = true
			edges = append(edges, Edge{A: fromID, B: toID})
		}
	}
	if err := connRows.Err(); err != nil {
		return nil, nil, nil, err
	}

	for _, s := range systems {
		slices.SortFunc(s.Connections, func(a, b galaxymap.Connection) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}
	slices.Sort(sources)

	return systems, edges, sources, nil
}
