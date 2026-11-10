// Command generate-items-kb mission template families.
//
// Most missions are generated from a handful of server formulas ("Trade Run:
// X to Y" alone has ~19,000 variants). Category pages render each formula as
// one family card instead of listing every permutation; hand-written missions
// that belong to no chain stay as one-offs.
package main

import (
	"cmp"
	"slices"
	"strings"
)

// familyMinVariants is the smallest group that renders as a family; smaller
// groups are listed as one-offs whatever their procedural flag says.
const familyMinVariants = 3

// oddJobsFamilyName names the family of procedural missions that carry
// whimsical, prefix-less titles ("Please hold", "The chair inspection").
const oddJobsFamilyName = "Odd jobs"

// MissionFamily is one server formula and every mission generated from it.
type MissionFamily struct {
	Key      string // type + "|" + Name
	Name     string // title prefix, e.g. "Contraband Run"
	Type     string
	Variants []*Mission
}

// familyKey groups a mission by the title text before its first colon. A
// prefix-less procedural mission takes oddJobsName; a prefix-less hand-written
// one keys on its exact title.
func familyKey(m *Mission, oddJobsName string) (key, name string) {
	if before, _, ok := strings.Cut(m.Title, ":"); ok && strings.TrimSpace(before) != "" {
		name = strings.TrimSpace(before)
	} else if m.Procedural {
		name = oddJobsName
	} else {
		name = m.Title
	}
	return m.Type + "|" + name, name
}

// hasTitlePrefix reports whether a title carries a "Prefix: ..." formula name.
func hasTitlePrefix(title string) bool {
	before, _, ok := strings.Cut(title, ":")
	return ok && strings.TrimSpace(before) != ""
}

// partitionFamilies drops chain members, groups the rest into families of at
// least familyMinVariants, and returns everything left over as one-offs —
// player-faction posts (ids starting "faction_") separately from the server's.
func partitionFamilies(missions []*Mission, chained map[string]bool) (families []*MissionFamily, oneOffs, factionOneOffs []*Mission) {
	var pool []*Mission
	for _, m := range missions {
		if !chained[m.ID] {
			pool = append(pool, m)
		}
	}

	// Prefix-less procedural missions of a type form one family: named after
	// their title when they all share it, otherwise "Odd jobs".
	oddTitles := map[string]map[string]bool{}
	for _, m := range pool {
		if m.Procedural && !hasTitlePrefix(m.Title) {
			if oddTitles[m.Type] == nil {
				oddTitles[m.Type] = map[string]bool{}
			}
			oddTitles[m.Type][m.Title] = true
		}
	}
	oddName := func(typ string) string {
		if t := oddTitles[typ]; len(t) == 1 {
			for title := range t {
				return title
			}
		}
		return oddJobsFamilyName
	}

	groups := map[string]*MissionFamily{}
	var order []string
	for _, m := range pool {
		key, name := familyKey(m, oddName(m.Type))
		g, ok := groups[key]
		if !ok {
			g = &MissionFamily{Key: key, Name: name, Type: m.Type}
			groups[key] = g
			order = append(order, key)
		}
		g.Variants = append(g.Variants, m)
	}

	for _, key := range order {
		g := groups[key]
		if len(g.Variants) >= familyMinVariants {
			families = append(families, g)
			continue
		}
		for _, m := range g.Variants {
			if strings.HasPrefix(m.ID, "faction_") {
				factionOneOffs = append(factionOneOffs, m)
			} else {
				oneOffs = append(oneOffs, m)
			}
		}
	}

	slices.SortFunc(families, func(a, b *MissionFamily) int {
		if c := cmp.Compare(len(b.Variants), len(a.Variants)); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	slices.SortFunc(oneOffs, compareMissions)
	slices.SortFunc(factionOneOffs, compareMissions)
	return families, oneOffs, factionOneOffs
}
