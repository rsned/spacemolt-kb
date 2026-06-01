package main

import (
	"sort"
	"strings"
)

// groupSightings collapses raw sightings by (system, poi, ship), keeping the
// latest LastSeenUTC and OR-ing the InCombat flag. Result is sorted by
// LastSeenUTC descending (string compare is valid for RFC3339 UTC).
func groupSightings(in []Sighting) []Sighting {
	type key struct{ sys, poi, ship string }
	byKey := map[key]*Sighting{}
	order := []key{}
	for i := range in {
		s := in[i]
		k := key{s.SystemID, s.POIID, s.ShipClass}
		g, ok := byKey[k]
		if !ok {
			cp := s
			byKey[k] = &cp
			order = append(order, k)
			continue
		}
		g.InCombat = g.InCombat || s.InCombat
		if s.LastSeenUTC > g.LastSeenUTC {
			g.LastSeenUTC = s.LastSeenUTC
		}
		if s.SystemSlug != "" && g.SystemSlug == "" {
			g.SystemSlug = s.SystemSlug
		}
	}
	out := make([]Sighting, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastSeenUTC != out[j].LastSeenUTC {
			return out[i].LastSeenUTC > out[j].LastSeenUTC
		}
		return strings.Compare(out[i].SystemID, out[j].SystemID) < 0
	})
	return out
}
