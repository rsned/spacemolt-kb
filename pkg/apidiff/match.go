package apidiff

import (
	"sort"
	"strings"
)

// Pair is one struct across two snapshots. A nil Old is a new struct; a nil
// New is a removed one. A Merged pair is an inline copy that was folded into
// a struct another pair already reports, so it gets no card of its own.
type Pair struct {
	Old, New *Struct
	Merged   bool
}

// nearMatch is the Jaccard similarity of field-name sets at or above which
// two differently named structs are treated as the same struct.
const nearMatch = 0.8

// Match pairs old and new structs: by ID first, then by field-name set, so a
// struct that was renamed, extracted from an inline variant or turned from a
// nested object into a named schema is not reported as new. Pairs come back
// sorted by New.ID, else Old.ID.
func Match(old, new map[string]*Struct) []Pair {
	var pairs []Pair
	var oldLeft, newLeft []*Struct
	for id, n := range new {
		if o, ok := old[id]; ok && !positional(id) {
			pairs = append(pairs, Pair{Old: o, New: n})
		} else {
			newLeft = append(newLeft, n)
		}
	}
	for id, o := range old {
		if _, ok := new[id]; !ok || positional(id) {
			oldLeft = append(oldLeft, o)
		}
	}
	type cand struct {
		score float64
		o, n  *Struct
	}
	var cands []cand
	for _, n := range newLeft {
		for _, o := range oldLeft {
			if s := jaccard(o, n); s >= nearMatch {
				cands = append(cands, cand{s, o, n})
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if x, y := a.o.ID == a.n.ID, b.o.ID == b.n.ID; x != y {
			return x
		}
		if x, y := parentOf(a.o.ID) == parentOf(a.n.ID), parentOf(b.o.ID) == parentOf(b.n.ID); x != y {
			return x
		}
		if a.n.ID != b.n.ID {
			return a.n.ID < b.n.ID
		}
		return a.o.ID < b.o.ID
	})
	usedO, usedN := map[*Struct]bool{}, map[*Struct]bool{}
	for _, c := range cands {
		if usedO[c.o] || usedN[c.n] {
			continue
		}
		usedO[c.o], usedN[c.n] = true, true
		pairs = append(pairs, Pair{Old: c.o, New: c.n})
	}
	// A variant that changed too much to reach nearMatch is still the same
	// variant when a leftover under the same parent has the same title or
	// shares any fields.
	var vc []cand
	for _, n := range newLeft {
		for _, o := range oldLeft {
			if usedN[n] || usedO[o] || !positional(n.ID) || !positional(o.ID) || parentOf(n.ID) != parentOf(o.ID) {
				continue
			}
			s := jaccard(o, n)
			if o.Display == n.Display {
				s++
			}
			if s > 0 {
				vc = append(vc, cand{s, o, n})
			}
		}
	}
	sort.Slice(vc, func(i, j int) bool {
		a, b := vc[i], vc[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.n.ID != b.n.ID {
			return a.n.ID < b.n.ID
		}
		return a.o.ID < b.o.ID
	})
	for _, c := range vc {
		if usedO[c.o] || usedN[c.n] {
			continue
		}
		usedO[c.o], usedN[c.n] = true, true
		pairs = append(pairs, Pair{Old: c.o, New: c.n})
	}
	for _, n := range newLeft {
		if !usedN[n] {
			pairs = append(pairs, Pair{New: n})
		}
	}
	for _, o := range oldLeft {
		if usedO[o] {
			continue
		}
		// Several identical inline objects can be extracted into one named
		// schema; every copy after the first folds into it.
		if inline(o.ID) {
			if n := bestMatch(o, new); n != nil {
				pairs = append(pairs, Pair{Old: o, New: n, Merged: true})
				continue
			}
		}
		pairs = append(pairs, Pair{Old: o})
	}
	sort.Slice(pairs, func(i, j int) bool {
		a, b := pairs[i], pairs[j]
		if pairKey(a) != pairKey(b) {
			return pairKey(a) < pairKey(b)
		}
		return pairRank(a) < pairRank(b)
	})
	return pairs
}

// positional reports whether an ID depends on a variant's position, which
// shifts between snapshots; those structs match by fields only.
func positional(id string) bool { return strings.Contains(id, "#v") }

// parentOf is the ID a variant or nested object hangs off: "P" for "P#v3",
// "P.items[]" for "P.items[]#v1", "P" for "P.x".
func parentOf(id string) string {
	if i := strings.LastIndex(id, "#v"); i >= 0 {
		return id[:i]
	}
	if i := strings.LastIndex(id, "."); i >= 0 {
		return id[:i]
	}
	return id
}

// inline reports whether an ID names an inline object rather than a schema.
func inline(id string) bool { return strings.ContainsAny(id, ".#") }

// bestMatch finds the new struct whose fields best match o, at nearMatch or
// above, ties broken by ID.
func bestMatch(o *Struct, new map[string]*Struct) *Struct {
	var best *Struct
	bestScore := 0.0
	for _, n := range new {
		s := jaccard(o, n)
		if s < nearMatch {
			continue
		}
		if best == nil || s > bestScore || (s == bestScore && n.ID < best.ID) {
			best, bestScore = n, s
		}
	}
	return best
}

// pairRank orders pairs that share a key: removed, then matched, then new,
// then merged copies by old ID. Every pair has a distinct rank, so the order
// never depends on map iteration.
func pairRank(p Pair) string {
	switch {
	case p.Merged:
		return "3" + p.Old.ID
	case p.New == nil:
		return "0"
	case p.Old == nil:
		return "2"
	}
	return "1"
}

func pairKey(p Pair) string {
	if p.New != nil {
		return p.New.ID
	}
	return p.Old.ID
}

// jaccard compares field-name sets. Structs without fields (pure unions)
// only ever match by ID.
func jaccard(a, b *Struct) float64 {
	if len(a.Fields) == 0 || len(b.Fields) == 0 {
		return 0
	}
	set := map[string]int{}
	for _, f := range a.Fields {
		set[f.Name] |= 1
	}
	for _, f := range b.Fields {
		set[f.Name] |= 2
	}
	both := 0
	for _, v := range set {
		if v == 3 {
			both++
		}
	}
	return float64(both) / float64(len(set))
}
