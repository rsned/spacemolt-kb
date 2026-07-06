package main

import (
	"database/sql"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// systemResolver maps a market station's system_id — which may be a systems.id
// slug OR a systems.name display value — to the canonical systems.id used by the
// connections graph.
type systemResolver struct {
	ids    map[string]bool
	byName map[string]string
}

// canon resolves a raw system_id to a canonical systems.id: it prefers a direct
// id match, then falls back to a systems.name match. ok is false if neither hits.
func (r *systemResolver) canon(systemID string) (string, bool) {
	if r.ids[systemID] {
		return systemID, true
	}
	if id, ok := r.byName[systemID]; ok {
		return id, true
	}
	return "", false
}

// loadSystemResolver builds a resolver from the knowledge DB systems table.
func loadSystemResolver(knowledgeDB *sql.DB) (*systemResolver, error) {
	r := &systemResolver{ids: map[string]bool{}, byName: map[string]string{}}
	rows, err := knowledgeDB.Query(`SELECT id, name FROM systems`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		r.ids[id] = true
		r.byName[name] = id
	}
	return r, rows.Err()
}

// loadConnections returns a symmetric adjacency map (systems.id -> neighbor ids)
// from the knowledge DB connections table.
func loadConnections(knowledgeDB *sql.DB) (map[string][]string, error) {
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
		sort.Strings(list)
		adj[a] = list
	}
	return adj, nil
}

// stationSystems maps each station id to its canonical system id, and returns
// the ids of any stations whose system could not be resolved.
func stationSystems(stations []StationMeta, r *systemResolver) (map[string]string, []string) {
	out := map[string]string{}
	var unresolved []string
	for _, s := range stations {
		if id, ok := r.canon(s.System); ok {
			out[s.ID] = id
		} else {
			unresolved = append(unresolved, s.ID)
		}
	}
	return out, unresolved
}

// bfsHops returns the jump distance from src to every reachable system.
func bfsHops(adj map[string][]string, src string) map[string]int {
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

// stationHopDist returns hopDist[home][other] = jump distance between the two
// stations' systems. Unreachable pairs are absent. Every station maps to itself
// at distance 0 (even if its system has no connections).
func stationHopDist(adj map[string][]string, stationSys map[string]string) map[string]map[string]int {
	// group stations by their system to translate system distances to stations.
	out := map[string]map[string]int{}
	for home, homeSys := range stationSys {
		sysDist := bfsHops(adj, homeSys)
		row := map[string]int{}
		for other, otherSys := range stationSys {
			if home == other {
				row[other] = 0
				continue
			}
			if d, ok := sysDist[otherSys]; ok {
				row[other] = d
			}
		}
		out[home] = row
	}
	return out
}

// poolMembers returns the station ids within radius jumps of home (including
// home), sorted for deterministic output.
func poolMembers(hopDist map[string]map[string]int, home string, radius int) []string {
	members := []string{home}
	for other, d := range hopDist[home] {
		if other == home {
			continue
		}
		if d <= radius {
			members = append(members, other)
		}
	}
	sort.Strings(members)
	return members
}

// pooledBook merges the sell ladders of the member stations into a single Book
// with each item's ladder re-sorted ascending by price. BestBuy is left empty:
// margins use the home station's own book, never the pool.
func pooledBook(books map[string]*buildcost.Book, members []string) *buildcost.Book {
	pb := &buildcost.Book{Sell: map[string]buildcost.Ladder{}, BestBuy: map[string]float64{}}
	for _, id := range members {
		b := books[id]
		if b == nil {
			continue
		}
		for item, ladder := range b.Sell {
			pb.Sell[item] = append(pb.Sell[item], ladder...)
		}
	}
	for item, ladder := range pb.Sell {
		sort.Slice(ladder, func(i, j int) bool { return ladder[i].Price < ladder[j].Price })
		pb.Sell[item] = ladder
	}
	return pb
}

// pooledBooksForRadius returns, for each station that has a local book, a pooled
// book combining every station within radius jumps. A station's pool always
// includes itself, so the active-station set is identical across radii.
func pooledBooksForRadius(books map[string]*buildcost.Book, hopDist map[string]map[string]int, stations []StationMeta, radius int) map[string]*buildcost.Book {
	out := make(map[string]*buildcost.Book, len(stations))
	for _, s := range stations {
		if books[s.ID] == nil {
			continue
		}
		out[s.ID] = pooledBook(books, poolMembers(hopDist, s.ID, radius))
	}
	return out
}
