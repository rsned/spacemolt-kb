package main

import (
	"cmp"
	"slices"
)

// RadiusRow describes the galaxy when stronghold reach is drawn out to
// Radius jumps.
type RadiusRow struct {
	Radius  int
	Systems int
	Percent float64
	Blobs   int
	// Merged is true when this row has strictly fewer blobs than the
	// previous one, i.e. territories joined at this radius.
	Merged bool
}

// TerritoryRow is one row of the nearest-stronghold table.
type TerritoryRow struct {
	SystemID string
	Name     string
	Systems  int
}

// componentCount returns the number of connected components formed by the
// members of inSet, counting a member with no in-set edge as its own
// component.
func componentCount(edges []Edge, inSet map[string]bool) int {
	parent := make(map[string]string, len(inSet))
	for id := range inSet {
		parent[id] = id
	}
	var find func(string) string //nolint:staticcheck // S1021 false positive: self-referential closure needs the two-step declare-then-assign
	find = func(x string) string {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}

	n := len(inSet)
	for _, e := range edges {
		if !inSet[e.A] || !inSet[e.B] {
			continue
		}
		ra, rb := find(e.A), find(e.B)
		if ra != rb {
			parent[ra] = rb
			n--
		}
	}
	return n
}

// RadiusRows builds one row per radius from 1 to maxRadius inclusive.
func RadiusRows(r Reach, edges []Edge, total, maxRadius int) []RadiusRow {
	rows := make([]RadiusRow, 0, max(0, maxRadius))
	prevBlobs := -1
	for radius := 1; radius <= maxRadius; radius++ {
		inSet := make(map[string]bool)
		for id, d := range r.Dist {
			if d <= radius {
				inSet[id] = true
			}
		}
		pct := 0.0
		if total > 0 {
			pct = 100.0 * float64(len(inSet)) / float64(total)
		}
		blobs := componentCount(edges, inSet)
		rows = append(rows, RadiusRow{
			Radius:  radius,
			Systems: len(inSet),
			Percent: pct,
			Blobs:   blobs,
			Merged:  prevBlobs >= 0 && blobs < prevBlobs,
		})
		prevBlobs = blobs
	}
	return rows
}

// TerritoryRows counts how many systems each source is nearest to, largest
// first, ties broken by name.
func TerritoryRows(r Reach, names map[string]string) []TerritoryRow {
	counts := make(map[string]int)
	for _, owner := range r.Owner {
		counts[owner]++
	}

	rows := make([]TerritoryRow, 0, len(counts))
	for id, n := range counts {
		name := names[id]
		if name == "" {
			name = id
		}
		rows = append(rows, TerritoryRow{SystemID: id, Name: name, Systems: n})
	}
	slices.SortFunc(rows, func(a, b TerritoryRow) int {
		if c := cmp.Compare(b.Systems, a.Systems); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.SystemID, b.SystemID)
	})
	return rows
}

// JumpEntry is one system in the by-jumps index.
type JumpEntry struct {
	SystemID string
	Name     string
}

// JumpSection lists every system sitting at exactly Radius jumps from the
// nearest stronghold.
type JumpSection struct {
	Radius  int
	Systems []JumpEntry
}

// JumpSections groups reachable systems by their exact distance, one section
// per radius from 0 to maxRadius inclusive. Radii with no systems at that
// exact distance are omitted rather than emitted empty.
//
// Entries are ordered by system ID, which is the token the rendered page is
// searched by; names are looked up for display and fall back to the ID.
func JumpSections(r Reach, names map[string]string, maxRadius int) []JumpSection {
	byRadius := make(map[int][]JumpEntry)
	for id, d := range r.Dist {
		if d > maxRadius {
			continue
		}
		name := names[id]
		if name == "" {
			name = id
		}
		byRadius[d] = append(byRadius[d], JumpEntry{SystemID: id, Name: name})
	}

	sections := make([]JumpSection, 0, len(byRadius))
	for radius := 0; radius <= maxRadius; radius++ {
		entries := byRadius[radius]
		if len(entries) == 0 {
			continue
		}
		slices.SortFunc(entries, func(a, b JumpEntry) int {
			return cmp.Compare(a.SystemID, b.SystemID)
		})
		sections = append(sections, JumpSection{Radius: radius, Systems: entries})
	}
	return sections
}
