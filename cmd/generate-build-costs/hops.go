package main

import (
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
	"github.com/rsned/spacemolt-kb/pkg/galaxyhops"
)

// stationSystems maps each station id to its canonical system id, and returns
// the ids of any stations whose system could not be resolved.
func stationSystems(stations []StationMeta, r *galaxyhops.SystemResolver) (map[string]string, []string) {
	out := map[string]string{}
	var unresolved []string
	for _, s := range stations {
		if id, ok := r.Canon(s.System); ok {
			out[s.ID] = id
		} else {
			unresolved = append(unresolved, s.ID)
		}
	}
	return out, unresolved
}

// stationHopDist returns hopDist[home][other] = jump distance between the two
// stations' systems. Unreachable pairs are absent. Every station maps to itself
// at distance 0 (even if its system has no connections).
func stationHopDist(adj map[string][]string, stationSys map[string]string) map[string]map[string]int {
	// group stations by their system to translate system distances to stations.
	out := map[string]map[string]int{}
	for home, homeSys := range stationSys {
		sysDist := galaxyhops.BFSHops(adj, homeSys)
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
