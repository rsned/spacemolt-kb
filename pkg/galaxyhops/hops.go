// Package galaxyhops resolves system ids and counts jumps over the galaxy's
// connection graph.
package galaxyhops

import (
	"database/sql"
	"slices"
)

// SystemResolver maps a system_id — which may be a systems.id slug OR a
// systems.name display value — to the canonical systems.id used by the
// connections graph.
type SystemResolver struct {
	ids    map[string]bool
	byName map[string]string
}

// NewSystemResolver builds a resolver from systems.id -> systems.name.
func NewSystemResolver(names map[string]string) *SystemResolver {
	r := &SystemResolver{ids: map[string]bool{}, byName: map[string]string{}}
	for id, name := range names {
		r.ids[id] = true
		r.byName[name] = id
	}
	return r
}

// Canon resolves a raw system_id to a canonical systems.id: it prefers a direct
// id match, then falls back to a systems.name match. ok is false if neither hits.
func (r *SystemResolver) Canon(systemID string) (string, bool) {
	if r.ids[systemID] {
		return systemID, true
	}
	if id, ok := r.byName[systemID]; ok {
		return id, true
	}
	return "", false
}

// LoadSystemResolver builds a resolver from the knowledge DB systems table.
func LoadSystemResolver(knowledgeDB *sql.DB) (*SystemResolver, error) {
	rows, err := knowledgeDB.Query(`SELECT id, name FROM systems`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	names := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewSystemResolver(names), nil
}

// LoadConnections returns a symmetric adjacency map (systems.id -> sorted
// neighbor ids) from the knowledge DB connections table.
func LoadConnections(knowledgeDB *sql.DB) (map[string][]string, error) {
	set := map[string]map[string]bool{}
	add := func(a, b string) {
		if set[a] == nil {
			set[a] = map[string]bool{}
		}
		set[a][b] = true
	}
	rows, err := knowledgeDB.Query(`SELECT from_system, to_system FROM connections`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		add(a, b)
		add(b, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	adj := make(map[string][]string, len(set))
	for a, nbrs := range set {
		list := make([]string, 0, len(nbrs))
		for b := range nbrs {
			list = append(list, b)
		}
		slices.Sort(list)
		adj[a] = list
	}
	return adj, nil
}

// BFSHops returns the jump distance from src to every reachable system.
func BFSHops(adj map[string][]string, src string) map[string]int {
	dist := map[string]int{src: 0}
	queue := []string{src}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, v := range adj[u] {
			if _, seen := dist[v]; !seen {
				dist[v] = dist[u] + 1
				queue = append(queue, v)
			}
		}
	}
	return dist
}
