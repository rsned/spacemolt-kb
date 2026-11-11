// Command generate-items-kb mission chain grouping.
package main

import (
	"cmp"
	"maps"
	"slices"
	"strings"
)

// missionTypeIcons gives each mission type a glyph, used to flag chain steps
// that belong to another category than the page showing them.
var missionTypeIcons = map[string]string{
	"combat":           "\u2694\ufe0f",     // crossed swords
	"crafting":         "\U0001f528",       // hammer
	"delivery":         "\U0001f4e6",       // package
	"equipment":        "\U0001f6e0\ufe0f", // hammer and wrench
	"exploration":      "\U0001f9ed",       // compass
	"mining":           "\u26cf\ufe0f",     // pick
	"salvage_recovery": "\u267b\ufe0f",     // recycling
	"smuggling":        "\U0001f576\ufe0f", // dark glasses
	"trading":          "\U0001f4b1",       // currency exchange
}

// MissionChainStep is one mission as rendered inside a chain on a category
// page. Foreign marks a step belonging to a different type than the page,
// which chains routinely do — the mining opener "First Cargo" runs into
// exploration missions, for example.
type MissionChainStep struct {
	Mission *Mission
	// Undiscovered marks a placeholder for a chain_next mission that has not
	// been scraped yet: Mission carries only its id and a title rebuilt from
	// it, and the row is rendered without a link or details.
	Undiscovered bool
	Foreign      bool
	Href         string
	TypeIcon     string
	TypeName     string
}

// MissionChain is a run of missions in chain order. Where a mission unlocks
// more than one next mission the run stops and continues in Branches, each a
// MissionChain of its own; branches can branch again.
type MissionChain struct {
	// Start is how many legs precede Steps[0]: 0 for a whole chain, the stem's
	// length for a branch, so leg numbers carry on into branches.
	Start    int
	Steps    []MissionChainStep
	Branches []MissionChain
}

// LegNumber is the 1-based leg number of Steps[i].
func (c MissionChain) LegNumber(i int) int { return c.Start + i + 1 }

// legCount is the number of legs in the chain's longest path.
func (c MissionChain) legCount() int {
	longest := 0
	for _, b := range c.Branches {
		longest = max(longest, b.legCount())
	}
	return len(c.Steps) + longest
}

// walk visits every step in the chain and its branches, depth first.
func (c MissionChain) walk(visit func(MissionChainStep)) {
	for _, s := range c.Steps {
		visit(s)
	}
	for _, b := range c.Branches {
		b.walk(visit)
	}
}

// Head is the first mission in the chain; chains are sorted by its difficulty.
func (c MissionChain) Head() *Mission { return c.Steps[0].Mission }

// HeadDifficulty exposes the sort key to templates.
func (c MissionChain) HeadDifficulty() int { return c.Head().Difficulty }

// Length exposes the step count to templates.
func (c MissionChain) Length() int { return len(c.Steps) }

// nextLegs returns the missions that follow m: its chain_next first, then every
// mission it unlocks (by title), without duplicates. One that has not been
// scraped yet comes back as an Undiscovered placeholder.
func nextLegs(m *Mission, byID map[string]*Mission) []MissionChainStep {
	var out []MissionChainStep
	seen := map[string]bool{}
	add := func(id, title string) {
		if id == "" || seen[id] || id == m.ID {
			return
		}
		seen[id] = true
		if next, ok := byID[id]; ok {
			out = append(out, MissionChainStep{Mission: next})
			return
		}
		if title == "" {
			title = missionTitleFromID(id)
		}
		out = append(out, MissionChainStep{Mission: &Mission{ID: id, Title: title, Type: m.Type}, Undiscovered: true})
	}
	add(m.ChainNext, "")
	for _, u := range m.Unlocks {
		add(u.ID, u.Title)
	}
	return out
}

// buildMissionChains walks from every head -- a mission nothing chains into or
// unlocks -- along chain_next and unlock edges, and returns each resulting chain
// of two or more legs. A mission with one next leg continues the run; one with
// several (The Listening Foundry opens both It Hums and The Foundry Points
// Somewhere) ends it, and each next leg starts a branch. A next leg not scraped
// yet becomes an Undiscovered placeholder, so a head whose continuation is
// unknown still renders as a chain opener rather than a one-off.
//
// Chains are a DAG: two routes can also converge on a shared tail (both Nebula
// consultations run into "Nebula Impossible List"). Each route is emitted
// complete from its own head, so a shared tail appears in both.
func buildMissionChains(missions []*Mission) []MissionChain {
	byID := make(map[string]*Mission, len(missions))
	for _, m := range missions {
		byID[m.ID] = m
	}
	linkedInto := make(map[string]bool, len(missions))
	for _, m := range missions {
		for _, n := range nextLegs(m, byID) {
			if !n.Undiscovered {
				linkedInto[n.Mission.ID] = true
			}
		}
	}

	var chains []MissionChain
	for _, m := range missions {
		if linkedInto[m.ID] {
			continue // not a head
		}
		c := walkChain(m, byID, map[string]bool{}, 0)
		if c.legCount() < 2 {
			continue
		}
		chains = append(chains, c)
	}
	slices.SortFunc(chains, compareChains)
	return chains
}

// walkChain builds the chain starting at m, start legs in. path holds the
// missions already on this route, so a cycle in the data ends the walk.
func walkChain(m *Mission, byID map[string]*Mission, path map[string]bool, start int) MissionChain {
	c := MissionChain{Start: start}
	for cur := m; cur != nil && !path[cur.ID]; {
		path[cur.ID] = true
		c.Steps = append(c.Steps, MissionChainStep{Mission: cur})
		next := nextLegs(cur, byID)
		switch {
		case len(next) == 0:
			return c
		case len(next) == 1:
			if next[0].Undiscovered {
				c.Steps = append(c.Steps, next[0])
				return c
			}
			cur = next[0].Mission
		default:
			at := start + len(c.Steps)
			slices.SortFunc(next, func(a, b MissionChainStep) int {
				return cmp.Or(cmp.Compare(a.Mission.Title, b.Mission.Title), cmp.Compare(a.Mission.ID, b.Mission.ID))
			})
			for _, n := range next {
				if n.Undiscovered {
					c.Branches = append(c.Branches, MissionChain{Start: at, Steps: []MissionChainStep{n}})
					continue
				}
				c.Branches = append(c.Branches, walkChain(n.Mission, byID, maps.Clone(path), at))
			}
			return c
		}
	}
	return c
}

// compareChains orders chains by head difficulty, then head title, so the
// ordering is stable across regenerations.
func compareChains(a, b MissionChain) int {
	if c := cmp.Compare(a.Head().Difficulty, b.Head().Difficulty); c != 0 {
		return c
	}
	return cmp.Compare(a.Head().Title, b.Head().Title)
}

// chainMemberIDs is the set of missions that appear in any chain.
func chainMemberIDs(chains []MissionChain) map[string]bool {
	members := map[string]bool{}
	for _, c := range chains {
		c.walk(func(s MissionChainStep) { members[s.Mission.ID] = true })
	}
	return members
}

// chainsForType returns the chains touching typ, with each step resolved
// relative to that type's directory: local steps link within the directory,
// foreign steps link across and carry their own type's icon.
func chainsForType(chains []MissionChain, typ string) []MissionChain {
	var out []MissionChain
	for _, c := range chains {
		touches := false
		c.walk(func(s MissionChainStep) { touches = touches || s.Mission.Type == typ })
		if !touches {
			continue
		}
		out = append(out, resolveChainForType(c, typ))
	}
	slices.SortFunc(out, compareChains)
	return out
}

// resolveChainForType copies c with every step's link and icon resolved
// relative to typ's directory, branches included.
func resolveChainForType(c MissionChain, typ string) MissionChain {
	r := MissionChain{Start: c.Start, Steps: make([]MissionChainStep, len(c.Steps))}
	for i, s := range c.Steps {
		m := s.Mission
		r.Steps[i] = MissionChainStep{
			Mission:      m,
			Undiscovered: s.Undiscovered,
			Foreign:      m.Type != typ,
			TypeIcon:     missionTypeIcons[m.Type],
			TypeName:     m.Type,
		}
		if !s.Undiscovered {
			r.Steps[i].Href = chainHref(typ, m.Type, m.ID)
		}
	}
	for _, b := range c.Branches {
		r.Branches = append(r.Branches, resolveChainForType(b, typ))
	}
	return r
}

// compareMissions orders missions by difficulty, then title.
func compareMissions(a, b *Mission) int {
	if c := cmp.Compare(a.Difficulty, b.Difficulty); c != 0 {
		return c
	}
	return cmp.Compare(a.Title, b.Title)
}

// missionChainSmallWords stay lowercase inside a reconstructed title.
var missionChainSmallWords = map[string]bool{
	"a": true, "an": true, "and": true, "at": true, "by": true, "for": true,
	"from": true, "in": true, "of": true, "on": true, "or": true, "the": true,
	"to": true, "with": true,
}

// missionTitleFromID reconstructs a readable title from a mission ID. Mission
// IDs are the slugified title (verified: 129 of 131 story missions match
// exactly), so this renders a chain continuation we have not scraped yet as a
// name rather than a raw slug. It is a display convenience only — the real
// title arrives with the mission itself.
func missionTitleFromID(id string) string {
	if id == "" {
		return ""
	}
	words := strings.Split(id, "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		if i > 0 && missionChainSmallWords[w] {
			words[i] = w
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
