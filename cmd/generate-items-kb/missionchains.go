// Command generate-items-kb mission chain grouping.
package main

import (
	"cmp"
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
	Mission  *Mission
	Foreign  bool
	Href     string
	TypeIcon string
	TypeName string
}

// MissionChain is a run of missions linked by chain_next, in chain order.
type MissionChain struct {
	Steps []MissionChainStep
}

// Head is the first mission in the chain; chains are sorted by its difficulty.
func (c MissionChain) Head() *Mission { return c.Steps[0].Mission }

// HeadDifficulty exposes the sort key to templates.
func (c MissionChain) HeadDifficulty() int { return c.Head().Difficulty }

// Length exposes the step count to templates.
func (c MissionChain) Length() int { return len(c.Steps) }

// buildMissionChains walks chain_next from every head — a mission nothing else
// chains into — and returns each resulting run. Only runs of two or more known
// missions count: a head whose continuation has not been scraped yet has
// nothing to draw, and belongs with the standalone missions.
//
// Chains are a DAG, not a forest: two routes can converge on a shared tail
// (both Nebula consultations run into "Nebula Impossible List"). Each route is
// emitted complete from its own head, so a shared tail appears in both.
func buildMissionChains(missions []*Mission) []MissionChain {
	byID := make(map[string]*Mission, len(missions))
	for _, m := range missions {
		byID[m.ID] = m
	}
	linkedInto := make(map[string]bool, len(missions))
	for _, m := range missions {
		if m.ChainNext != "" && byID[m.ChainNext] != nil {
			linkedInto[m.ChainNext] = true
		}
	}

	var chains []MissionChain
	for _, m := range missions {
		if linkedInto[m.ID] {
			continue // not a head
		}
		var steps []MissionChainStep
		seen := make(map[string]bool)
		for cur := m; cur != nil && !seen[cur.ID]; cur = byID[cur.ChainNext] {
			seen[cur.ID] = true
			steps = append(steps, MissionChainStep{Mission: cur})
		}
		if len(steps) < 2 {
			continue
		}
		chains = append(chains, MissionChain{Steps: steps})
	}
	slices.SortFunc(chains, compareChains)
	return chains
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
		for _, s := range c.Steps {
			members[s.Mission.ID] = true
		}
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
		for _, s := range c.Steps {
			if s.Mission.Type == typ {
				touches = true
				break
			}
		}
		if !touches {
			continue
		}
		steps := make([]MissionChainStep, len(c.Steps))
		for i, s := range c.Steps {
			m := s.Mission
			steps[i] = MissionChainStep{
				Mission:  m,
				Foreign:  m.Type != typ,
				Href:     chainHref(typ, m.Type, m.ID),
				TypeIcon: missionTypeIcons[m.Type],
				TypeName: m.Type,
			}
		}
		out = append(out, MissionChain{Steps: steps})
	}
	slices.SortFunc(out, compareChains)
	return out
}

// partitionMissions splits a category's missions into the hand-authored ones
// that are not part of any chain and the procedurally generated bulk, each
// sorted by difficulty then title.
func partitionMissions(missions []*Mission, chained map[string]bool) (standalone, procedural []*Mission) {
	for _, m := range missions {
		switch {
		case m.Procedural:
			procedural = append(procedural, m)
		case chained[m.ID]:
			// rendered in the chain section
		default:
			standalone = append(standalone, m)
		}
	}
	slices.SortFunc(standalone, compareMissions)
	slices.SortFunc(procedural, compareMissions)
	return standalone, procedural
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
